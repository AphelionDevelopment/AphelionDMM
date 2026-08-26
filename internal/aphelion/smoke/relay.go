package smoke

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/relayclient"
)

func RunRelayTransport(ctx context.Context, endpoint string) error {
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	_, editorPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	var ownerKey protocolv2.ActorKey
	copy(ownerKey[:], ownerPublic)
	var roomID protocolv2.RoomID
	if _, err := rand.Read(roomID[:]); err != nil {
		return err
	}
	owner, err := relayclient.ConnectOwner(ctx, endpoint, roomID, ownerPrivate)
	if err != nil {
		return fmt.Errorf("connect smoke owner: %w", err)
	}
	defer owner.Close()
	var capability [32]byte
	if _, err := rand.Read(capability[:]); err != nil {
		return err
	}
	if err := owner.ReplaceAdmissions(ctx, 1, []protocolv2.AdmissionControl{{Digest: sha256.Sum256(capability[:]), Role: protocolv2.RoleEditor, ExpiresAt: time.Now().Add(time.Minute)}}); err != nil {
		return fmt.Errorf("publish smoke admission: %w", err)
	}
	editor, err := relayclient.ConnectParticipant(ctx, endpoint, roomID, editorPrivate, capability)
	if err != nil {
		return fmt.Errorf("connect smoke editor: %w", err)
	}
	defer editor.Close()
	var groupKey protocolv2.GroupKey
	if _, err := rand.Read(groupKey[:]); err != nil {
		return err
	}
	expected := protocolv2.ProfileUpdate{DisplayName: "Relay Smoke Editor", Sequence: 1}
	if err := editor.SendApplication(ctx, groupKey, protocolv2.RouteOwner, protocolv2.ActorKey{}, protocolv2.ApplicationProfileUpdate, expected); err != nil {
		return fmt.Errorf("send smoke application: %w", err)
	}
	sender, received, err := owner.ReadApplication(ctx, groupKey)
	if err != nil {
		return fmt.Errorf("read smoke application: %w", err)
	}
	if sender != editor.ActorKey() || received.Type != protocolv2.ApplicationProfileUpdate || *received.Payload.(*protocolv2.ProfileUpdate) != expected || owner.ActorKey() != ownerKey {
		return fmt.Errorf("relay smoke application did not converge")
	}
	return nil
}
