package authority

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"

	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/store"
)

func TestOwnerSessionCommitsEditorOperationBeforeAcceptance(t *testing.T) {
	session, editor, fixture := newTestOwnerSession(t, store.NewMemoryStore())
	operation := fixture.First.Operation
	operation.ActorID = editor.ActorID

	response, err := session.Handle(context.Background(), editor.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationOperationSubmit, Payload: &protocolv2.OperationSubmit{Operation: operation}})
	require.NoError(t, err)
	require.Equal(t, protocolv2.ApplicationOperationAccepted, response.Type)
	accepted := response.Payload.(*protocolv2.OperationAccepted)
	require.Equal(t, operation.OperationID, accepted.Operation.OperationID)
	stored, found, err := session.documentStore.LookupOperation(context.Background(), operation.DocumentID, operation.OperationID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, stored, accepted.Operation)
}

func TestOwnerSessionRejectsViewerMutationWithoutCommit(t *testing.T) {
	session, editor, fixture := newTestOwnerSession(t, store.NewMemoryStore())
	session.manifest.Manifest.Members[1].Role = protocolv2.RoleViewer
	operation := fixture.First.Operation
	operation.ActorID = editor.ActorID

	response, err := session.Handle(context.Background(), editor.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationOperationSubmit, Payload: &protocolv2.OperationSubmit{Operation: operation}})
	require.NoError(t, err)
	require.Equal(t, protocolv2.ApplicationOperationRejected, response.Type)
	require.Equal(t, "role_forbidden", response.Payload.(*protocolv2.OperationRejected).Code)
	_, found, err := session.documentStore.LookupOperation(context.Background(), operation.DocumentID, operation.OperationID)
	require.NoError(t, err)
	require.False(t, found)
}

func TestOwnerSessionReturnsNoAcceptanceWhenLocalAppendFails(t *testing.T) {
	value := &appendFailingStore{SessionStore: store.NewMemoryStore()}
	session, editor, fixture := newTestOwnerSession(t, value)
	operation := fixture.First.Operation
	operation.ActorID = editor.ActorID

	response, err := session.Handle(context.Background(), editor.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationOperationSubmit, Payload: &protocolv2.OperationSubmit{Operation: operation}})
	require.ErrorIs(t, err, errInjectedAppend)
	require.Zero(t, response)
}

func TestOwnerSessionTurnsDeterministicValidationFailureIntoStableRejection(t *testing.T) {
	session, editor, fixture := newTestOwnerSession(t, store.NewMemoryStore())
	operation := fixture.First.Operation
	operation.ActorID = editor.ActorID
	operation.BaseMapHash = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	response, err := session.Handle(context.Background(), editor.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationOperationSubmit, Payload: &protocolv2.OperationSubmit{Operation: operation}})
	require.NoError(t, err)
	require.Equal(t, protocolv2.ApplicationOperationRejected, response.Type)
	require.Equal(t, "base_hash_mismatch", response.Payload.(*protocolv2.OperationRejected).Code)
}

func TestOwnerSessionAllowsOwnerAndParticipantRenameAfterAcknowledgement(t *testing.T) {
	session, editor, _ := newTestOwnerSession(t, store.NewMemoryStore())
	owner := session.manifest.Manifest.Members[0]

	response, err := session.Handle(context.Background(), owner.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationProfileUpdate, Payload: &protocolv2.ProfileUpdate{DisplayName: "New Test Owner", Sequence: 1}})
	require.NoError(t, err)
	require.Equal(t, protocolv2.ApplicationRoleManifest, response.Type)
	require.Equal(t, "New Test Owner", response.Payload.(*protocolv2.RoleManifest).Members[0].DisplayName)
	response, err = session.Handle(context.Background(), editor.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationProfileUpdate, Payload: &protocolv2.ProfileUpdate{DisplayName: "Test Editor", Sequence: 1}})
	require.NoError(t, err)
	require.Equal(t, "Test Editor", response.Payload.(*protocolv2.RoleManifest).Members[1].DisplayName)
	_, err = session.Handle(context.Background(), editor.ActorKey, protocolv2.DecodedApplication{Type: protocolv2.ApplicationProfileUpdate, Payload: &protocolv2.ProfileUpdate{DisplayName: "Stale", Sequence: 1}})
	require.ErrorContains(t, err, "profile sequence")
}

func TestOwnerSessionRequiresTargetSignatureForOwnershipTransfer(t *testing.T) {
	fixture, err := store.NewConformanceFixture()
	require.NoError(t, err)
	value := store.NewMemoryStore()
	document, err := StartDocument(context.Background(), fixture.Initial, value)
	require.NoError(t, err)
	t.Cleanup(func() { _ = document.Close(context.Background()) })
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	editorPublic, editorPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifest := testManifest(t, ownerPublic)
	var editorKey protocolv2.ActorKey
	copy(editorKey[:], editorPublic)
	manifest.Members = append(manifest.Members, protocolv2.Member{ActorKey: editorKey, ActorID: fixture.First.ActorID, Role: protocolv2.RoleEditor, DisplayName: "Test Editor"})
	signed, err := SignManifest(manifest, ownerPrivate)
	require.NoError(t, err)
	session, err := NewOwnerSession(document, value, ownerPrivate, signed)
	require.NoError(t, err)
	offer, err := session.OfferOwnership(editorKey)
	require.NoError(t, err)
	digest, err := protocolv2.DigestOwnershipOffer(offer)
	require.NoError(t, err)
	acceptance := protocolv2.OwnershipAccept{OfferSHA256: digest}
	copy(acceptance.NewOwnerKeySignature[:], ed25519.Sign(editorPrivate, digest[:]))

	transferred, err := session.AcceptOwnership(editorKey, acceptance)
	require.NoError(t, err)
	require.Equal(t, editorKey, transferred.Manifest.OwnerKey)
	require.Equal(t, protocolv2.RoleEditor, transferred.Manifest.Members[0].Role)
	require.Equal(t, protocolv2.RoleOwner, transferred.Manifest.Members[1].Role)
	require.Equal(t, digest, transferred.OfferSHA256)
	require.True(t, ed25519.Verify(ownerPublic, digest[:], transferred.OldOwnerSignature[:]))
	require.True(t, ed25519.Verify(editorPublic, digest[:], transferred.NewOwnerSignature[:]))
}

func newTestOwnerSession(t *testing.T, value store.SessionStore) (*OwnerSession, protocolv2.Member, store.ConformanceFixture) {
	t.Helper()
	fixture, err := store.NewConformanceFixture()
	require.NoError(t, err)
	document, err := StartDocument(context.Background(), fixture.Initial, value)
	require.NoError(t, err)
	t.Cleanup(func() { _ = document.Close(context.Background()) })
	ownerPublic, ownerPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	editorPublic, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	ownerManifest := testManifest(t, ownerPublic)
	require.NoError(t, fixture.First.ActorID.Validate())
	var editorKey protocolv2.ActorKey
	copy(editorKey[:], editorPublic)
	editor := protocolv2.Member{ActorKey: editorKey, ActorID: fixture.First.ActorID, Role: protocolv2.RoleEditor, DisplayName: "Initial Editor"}
	ownerManifest.Members = append(ownerManifest.Members, editor)
	signed, err := SignManifest(ownerManifest, ownerPrivate)
	require.NoError(t, err)
	session, err := NewOwnerSession(document, value, ownerPrivate, signed)
	require.NoError(t, err)
	return session, editor, fixture
}
