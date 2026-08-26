package relay

import (
	"fmt"
	"sort"

	"sdmm/internal/aphelion/collab/protocolv2"
)

type Router struct {
	registry *Registry
}

func NewRouter(registry *Registry) *Router {
	return &Router{registry: registry}
}

func (router *Router) Targets(envelope protocolv2.Envelope) ([]protocolv2.ActorKey, error) {
	if router == nil || router.registry == nil {
		return nil, fmt.Errorf("relay registry is unavailable")
	}
	if _, err := envelope.Header.AuthenticatedBytes(); err != nil {
		return nil, err
	}
	if envelope.Header.Kind != protocolv2.KindApplication {
		return nil, fmt.Errorf("relay router accepts only application envelopes")
	}
	room, exists := router.registry.Snapshot(envelope.Header.RoomID)
	if !exists {
		return nil, fmt.Errorf("room does not exist")
	}
	role, member := room.Actors[envelope.Header.Sender]
	if !member || !room.Connected[envelope.Header.Sender] {
		return nil, fmt.Errorf("sender is not connected to the room")
	}
	switch envelope.Header.Route {
	case protocolv2.RouteOwner:
		if !room.OwnerConnected {
			return nil, fmt.Errorf("owner_offline")
		}
		if envelope.Header.Sender == room.Owner {
			return nil, fmt.Errorf("owner cannot route to itself")
		}
		return []protocolv2.ActorKey{room.Owner}, nil
	case protocolv2.RouteActor:
		if role != protocolv2.RoleOwner || envelope.Header.Sender != room.Owner {
			return nil, fmt.Errorf("actor routing is owner-only")
		}
		if _, exists := room.Actors[envelope.Header.Recipient]; !exists || !room.Connected[envelope.Header.Recipient] {
			return nil, fmt.Errorf("recipient is not connected")
		}
		return []protocolv2.ActorKey{envelope.Header.Recipient}, nil
	case protocolv2.RouteRoom:
		if role != protocolv2.RoleOwner || envelope.Header.Sender != room.Owner {
			return nil, fmt.Errorf("room routing is owner-only")
		}
		return connectedExcept(room, envelope.Header.Sender), nil
	case protocolv2.RoutePresence:
		return connectedExcept(room, envelope.Header.Sender), nil
	default:
		return nil, fmt.Errorf("control route cannot carry application payloads")
	}
}

func connectedExcept(room RoomSnapshot, excluded protocolv2.ActorKey) []protocolv2.ActorKey {
	targets := make([]protocolv2.ActorKey, 0, len(room.Connected))
	for actor, connected := range room.Connected {
		if connected && actor != excluded {
			targets = append(targets, actor)
		}
	}
	sort.Slice(targets, func(left, right int) bool { return string(targets[left][:]) < string(targets[right][:]) })
	return targets
}
