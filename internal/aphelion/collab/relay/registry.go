package relay

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"time"

	"sdmm/internal/aphelion/collab/protocolv2"
)

type Admission struct {
	Digest     [32]byte
	Role       protocolv2.Role
	ExpiresAt  time.Time
	BoundActor protocolv2.ActorKey
}

type Registry struct {
	mutex                 sync.RWMutex
	roomTTL               time.Duration
	maxRooms              int
	maxConnectionsPerRoom int
	rooms                 map[protocolv2.RoomID]*room
}

type room struct {
	owner               protocolv2.ActorKey
	ownerConnected      bool
	admissionGeneration uint64
	admissions          map[[32]byte]Admission
	actors              map[protocolv2.ActorKey]protocolv2.Role
	connected           map[protocolv2.ActorKey]bool
	lastActivity        time.Time
}

type RoomSnapshot struct {
	Owner               protocolv2.ActorKey
	OwnerConnected      bool
	AdmissionGeneration uint64
	Actors              map[protocolv2.ActorKey]protocolv2.Role
	Connected           map[protocolv2.ActorKey]bool
}

func NewRegistry(roomTTL time.Duration) *Registry {
	return NewRegistryWithLimits(roomTTL, 0, 0)
}

func NewRegistryWithLimits(roomTTL time.Duration, maxRooms, maxConnectionsPerRoom int) *Registry {
	return &Registry{roomTTL: roomTTL, maxRooms: maxRooms, maxConnectionsPerRoom: maxConnectionsPerRoom, rooms: make(map[protocolv2.RoomID]*room)}
}

func (registry *Registry) CreateRoom(roomID protocolv2.RoomID, owner protocolv2.ActorKey, now time.Time) error {
	if roomID == (protocolv2.RoomID{}) || owner == (protocolv2.ActorKey{}) || now.IsZero() {
		return fmt.Errorf("room registration is invalid")
	}
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	if current, exists := registry.rooms[roomID]; exists {
		if current.owner != owner {
			return fmt.Errorf("room is owned by another actor")
		}
		current.ownerConnected = true
		current.connected[owner] = true
		current.lastActivity = now
		return nil
	}
	if registry.maxRooms > 0 && len(registry.rooms) >= registry.maxRooms {
		return fmt.Errorf("relay room limit reached")
	}
	registry.rooms[roomID] = &room{
		owner:          owner,
		ownerConnected: true,
		admissions:     make(map[[32]byte]Admission),
		actors:         map[protocolv2.ActorKey]protocolv2.Role{owner: protocolv2.RoleOwner},
		connected:      map[protocolv2.ActorKey]bool{owner: true},
		lastActivity:   now,
	}
	return nil
}

func (registry *Registry) ReplaceAdmissions(roomID protocolv2.RoomID, owner protocolv2.ActorKey, generation uint64, admissions []Admission) error {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	current, exists := registry.rooms[roomID]
	if !exists {
		return fmt.Errorf("room does not exist")
	}
	if current.owner != owner {
		return fmt.Errorf("admission replacement sender is not the owner")
	}
	if generation == 0 || generation <= current.admissionGeneration {
		return fmt.Errorf("admission generation is stale")
	}
	replacement := make(map[[32]byte]Admission, len(admissions))
	for _, admission := range admissions {
		if admission.Digest == ([32]byte{}) || (admission.Role != protocolv2.RoleViewer && admission.Role != protocolv2.RoleEditor) || admission.ExpiresAt.IsZero() {
			return fmt.Errorf("admission is invalid")
		}
		if _, duplicate := replacement[admission.Digest]; duplicate {
			return fmt.Errorf("admission digest is duplicated")
		}
		replacement[admission.Digest] = admission
		if admission.BoundActor != (protocolv2.ActorKey{}) {
			current.actors[admission.BoundActor] = admission.Role
		}
	}
	current.admissions = replacement
	current.admissionGeneration = generation
	return nil
}

func (registry *Registry) Connect(roomID protocolv2.RoomID, actor protocolv2.ActorKey, capability []byte, now time.Time) (protocolv2.Role, error) {
	if actor == (protocolv2.ActorKey{}) || len(capability) == 0 {
		return "", fmt.Errorf("connection identity or capability is empty")
	}
	digest := sha256.Sum256(capability)
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	current, exists := registry.rooms[roomID]
	if !exists {
		return "", fmt.Errorf("room does not exist")
	}
	admission, exists := current.admissions[digest]
	if !exists {
		return "", fmt.Errorf("admission capability is unknown")
	}
	if !now.Before(admission.ExpiresAt) {
		return "", fmt.Errorf("admission capability is expired")
	}
	if admission.BoundActor != (protocolv2.ActorKey{}) && admission.BoundActor != actor {
		return "", fmt.Errorf("admission capability is bound to another actor")
	}
	if !current.connected[actor] && registry.maxConnectionsPerRoom > 0 {
		connectedCount := 0
		for _, connected := range current.connected {
			if connected {
				connectedCount++
			}
		}
		if connectedCount >= registry.maxConnectionsPerRoom {
			return "", fmt.Errorf("relay room connection limit reached")
		}
	}
	if admission.BoundActor == (protocolv2.ActorKey{}) {
		admission.BoundActor = actor
		current.admissions[digest] = admission
	}
	current.actors[actor] = admission.Role
	current.connected[actor] = true
	current.lastActivity = now
	return admission.Role, nil
}

func (registry *Registry) Disconnect(roomID protocolv2.RoomID, actor protocolv2.ActorKey, now time.Time) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	current, exists := registry.rooms[roomID]
	if !exists {
		return
	}
	current.connected[actor] = false
	if actor == current.owner {
		current.ownerConnected = false
	}
	current.lastActivity = now
}

func (registry *Registry) TransferOwner(roomID protocolv2.RoomID, oldOwner, newOwner protocolv2.ActorKey, now time.Time) error {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	current, exists := registry.rooms[roomID]
	if !exists {
		return fmt.Errorf("room does not exist")
	}
	if current.owner != oldOwner {
		return fmt.Errorf("ownership transfer sender is not the owner")
	}
	if !current.connected[oldOwner] || !current.connected[newOwner] {
		return fmt.Errorf("ownership transfer requires both actors to be connected")
	}
	if current.actors[newOwner] != protocolv2.RoleEditor {
		return fmt.Errorf("new owner is not a current editor")
	}
	current.owner = newOwner
	current.ownerConnected = true
	current.actors[oldOwner] = protocolv2.RoleEditor
	current.actors[newOwner] = protocolv2.RoleOwner
	current.lastActivity = now
	return nil
}

func (registry *Registry) Snapshot(roomID protocolv2.RoomID) (RoomSnapshot, bool) {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	current, exists := registry.rooms[roomID]
	if !exists {
		return RoomSnapshot{}, false
	}
	snapshot := RoomSnapshot{Owner: current.owner, OwnerConnected: current.ownerConnected, AdmissionGeneration: current.admissionGeneration, Actors: make(map[protocolv2.ActorKey]protocolv2.Role, len(current.actors)), Connected: make(map[protocolv2.ActorKey]bool, len(current.connected))}
	for actor, role := range current.actors {
		snapshot.Actors[actor] = role
	}
	for actor, connected := range current.connected {
		snapshot.Connected[actor] = connected
	}
	return snapshot, true
}

func (registry *Registry) RoomCount() int {
	registry.mutex.RLock()
	defer registry.mutex.RUnlock()
	return len(registry.rooms)
}

func (registry *Registry) Sweep(now time.Time) []protocolv2.RoomID {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	removed := make([]protocolv2.RoomID, 0)
	for roomID, current := range registry.rooms {
		if current.ownerConnected || now.Sub(current.lastActivity) <= registry.roomTTL {
			continue
		}
		delete(registry.rooms, roomID)
		removed = append(removed, roomID)
	}
	sort.Slice(removed, func(left, right int) bool { return string(removed[left][:]) < string(removed[right][:]) })
	return removed
}
