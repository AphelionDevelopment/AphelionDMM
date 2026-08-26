package authority

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
)

func (session *OwnerSession) Sync(ctx context.Context, sender protocolv2.ActorKey, hello protocolv2.SyncHello) (Response, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	member, found := session.member(sender)
	if hello.ActorKey != sender {
		return Response{}, fmt.Errorf("sync actor does not match sender")
	}
	if !found {
		var err error
		member, err = session.admitMember(sender, hello, time.Now().UTC())
		if err != nil {
			return Response{}, err
		}
	}
	if hello.Role != member.Role {
		return Response{}, fmt.Errorf("sync role does not match current manifest")
	}
	current, err := session.document.Snapshot(ctx)
	if err != nil {
		return Response{}, err
	}
	if hello.EnvironmentHash != current.EnvironmentHash {
		return Response{}, fmt.Errorf("sync environment does not match owner document")
	}
	currentHash, err := current.Hash()
	if err != nil {
		return Response{}, err
	}
	base, retained, err := session.documentStore.Load(ctx, session.documentID)
	if err != nil {
		return Response{}, err
	}
	knownHash, known, err := session.documentStore.RevisionHash(ctx, session.documentID, hello.Revision)
	if err != nil {
		return Response{}, err
	}
	if known && knownHash == hello.MapHash && hello.Revision >= base.Revision && hello.Revision <= current.Revision {
		operations := make([]model.AcceptedOperation, 0)
		for _, accepted := range retained {
			if accepted.Revision > hello.Revision {
				operations = append(operations, model.CloneAcceptedOperation(accepted))
			}
		}
		return Response{
			Type:      protocolv2.ApplicationSyncReplay,
			Payload:   &protocolv2.SyncReplay{Operations: operations, Revision: current.Revision, MapHash: currentHash},
			Recipient: sender,
		}, nil
	}
	return Response{
		Type:      protocolv2.ApplicationSyncSnapshot,
		Payload:   &protocolv2.SyncSnapshot{Snapshot: current, MapHash: currentHash},
		Recipient: sender,
	}, nil
}

func (session *OwnerSession) admitMember(sender protocolv2.ActorKey, hello protocolv2.SyncHello, now time.Time) (protocolv2.Member, error) {
	if hello.Invitation == nil {
		return protocolv2.Member{}, fmt.Errorf("sync actor is not in the current manifest")
	}
	if err := protocolv2.VerifyInvitation(*hello.Invitation, now); err != nil {
		return protocolv2.Member{}, err
	}
	digest := sha256.Sum256(hello.Invitation.Admission[:])
	registered, exists := session.invitations[digest]
	if !exists || registered.invitation.OwnerSignature != hello.Invitation.OwnerSignature {
		return protocolv2.Member{}, fmt.Errorf("sync invitation is not registered")
	}
	if registered.boundActor != (protocolv2.ActorKey{}) && registered.boundActor != sender {
		return protocolv2.Member{}, fmt.Errorf("sync invitation is bound to another actor")
	}
	name := strings.TrimSpace(hello.DisplayName)
	if err := hello.ActorID.Validate(); err != nil || name == "" || !utf8.ValidString(name) || len(name) > MaxDisplayNameBytes || hello.Role != registered.invitation.Role {
		return protocolv2.Member{}, fmt.Errorf("sync invitation profile is invalid")
	}
	manifest := cloneRoleManifest(session.manifest.Manifest)
	manifest.Generation++
	member := protocolv2.Member{ActorKey: sender, ActorID: hello.ActorID, Role: hello.Role, DisplayName: name}
	manifest.Members = append(manifest.Members, member)
	signed, err := SignManifest(manifest, session.signer)
	if err != nil {
		return protocolv2.Member{}, err
	}
	registered.boundActor = sender
	session.invitations[digest] = registered
	session.manifest = signed
	return member, nil
}
