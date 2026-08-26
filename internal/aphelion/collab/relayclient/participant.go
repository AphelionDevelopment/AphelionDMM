package relayclient

import (
	"context"
	"fmt"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/replica"
)

type Participant struct {
	Transport   *Client
	Replica     *replica.Session
	GroupKey    protocolv2.GroupKey
	ActorID     model.ActorID
	DisplayName string
	Invitation  *protocolv2.Invitation
}

func (participant *Participant) Synchronize(ctx context.Context, environmentHash string, pending []model.OperationID) (protocolv2.SyncComplete, error) {
	if participant == nil || participant.Transport == nil || participant.Replica == nil {
		return protocolv2.SyncComplete{}, fmt.Errorf("participant relay adapter is incomplete")
	}
	local := participant.Replica.LocalSession()
	hello := protocolv2.SyncHello{
		ActorKey: participant.Transport.ActorKey(), ActorID: participant.ActorID, DisplayName: participant.DisplayName,
		Role: local.Role, Revision: local.AcknowledgedRevision, MapHash: local.AcknowledgedMapHash,
		EnvironmentHash: environmentHash, Pending: append([]model.OperationID(nil), pending...), Invitation: participant.Invitation,
	}
	if err := participant.Transport.SendApplication(ctx, participant.GroupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationSyncHello, hello); err != nil {
		return protocolv2.SyncComplete{}, err
	}
	sender, message, err := participant.Transport.ReadApplication(ctx, participant.GroupKey)
	if err != nil {
		return protocolv2.SyncComplete{}, err
	}
	switch message.Type {
	case protocolv2.ApplicationSyncReplay:
		return participant.Replica.InstallReplay(ctx, sender, *message.Payload.(*protocolv2.SyncReplay))
	case protocolv2.ApplicationSyncSnapshot:
		return participant.Replica.InstallSnapshot(ctx, sender, *message.Payload.(*protocolv2.SyncSnapshot))
	default:
		return protocolv2.SyncComplete{}, fmt.Errorf("owner returned unexpected sync message %q", message.Type)
	}
}

func (participant *Participant) Submit(ctx context.Context, operation model.Operation) error {
	return participant.Transport.SendApplication(ctx, participant.GroupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationOperationSubmit, protocolv2.OperationSubmit{Operation: operation})
}

func (participant *Participant) ReceiveAccepted(ctx context.Context) (protocolv2.OperationAccepted, error) {
	sender, message, err := participant.Transport.ReadApplication(ctx, participant.GroupKey)
	if err != nil {
		return protocolv2.OperationAccepted{}, err
	}
	if message.Type != protocolv2.ApplicationOperationAccepted {
		return protocolv2.OperationAccepted{}, fmt.Errorf("owner returned unexpected application message %q", message.Type)
	}
	accepted := *message.Payload.(*protocolv2.OperationAccepted)
	acknowledgement, err := participant.Replica.ApplyAccepted(ctx, sender, accepted)
	if err != nil {
		return protocolv2.OperationAccepted{}, err
	}
	if err := participant.Transport.SendApplication(ctx, participant.GroupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationRevisionAcknowledged, acknowledgement); err != nil {
		return protocolv2.OperationAccepted{}, err
	}
	return accepted, nil
}
