package relayclient

import (
	"context"
	"fmt"

	"sdmm/internal/aphelion/collab/authority"
	"sdmm/internal/aphelion/collab/protocolv2"
)

type Owner struct {
	Transport *Client
	Authority *authority.OwnerSession
	GroupKey  protocolv2.GroupKey
}

func (owner *Owner) Serve(ctx context.Context) error {
	if owner == nil || owner.Transport == nil || owner.Authority == nil {
		return fmt.Errorf("owner relay adapter is incomplete")
	}
	for {
		sender, message, err := owner.Transport.ReadApplication(ctx, owner.GroupKey)
		if err != nil {
			return err
		}
		var response authority.Response
		if message.Type == protocolv2.ApplicationSyncHello {
			hello, ok := message.Payload.(*protocolv2.SyncHello)
			if !ok {
				return fmt.Errorf("sync hello payload is invalid")
			}
			response, err = owner.Authority.Sync(ctx, sender, *hello)
		} else {
			response, err = owner.Authority.Handle(ctx, sender, message)
		}
		if err != nil {
			return err
		}
		if response.Type == "" {
			continue
		}
		route := protocolv2.RouteActor
		recipient := response.Recipient
		if response.Broadcast {
			route = protocolv2.RouteRoom
			recipient = protocolv2.ActorKey{}
		} else if recipient == (protocolv2.ActorKey{}) {
			recipient = sender
		}
		if err := owner.Transport.SendApplication(ctx, owner.GroupKey, route, recipient, response.Type, response.Payload); err != nil {
			return err
		}
	}
}
