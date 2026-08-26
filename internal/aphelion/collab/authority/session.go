package authority

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"sdmm/internal/aphelion/collab/engine"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocolv2"
	"sdmm/internal/aphelion/collab/store"
)

type Response struct {
	Type      protocolv2.ApplicationType
	Payload   any
	Recipient protocolv2.ActorKey
	Broadcast bool
}

type OwnerSession struct {
	mutex            sync.Mutex
	document         *Document
	documentID       model.DocumentID
	documentStore    store.SessionStore
	signer           ed25519.PrivateKey
	manifest         SignedManifest
	profileSequence  map[protocolv2.ActorKey]uint64
	peerRevision     map[protocolv2.ActorKey]model.Revision
	pendingOwnership *protocolv2.OwnershipOffer
	invitations      map[[32]byte]registeredInvitation
}

type registeredInvitation struct {
	invitation protocolv2.Invitation
	boundActor protocolv2.ActorKey
}

func NewOwnerSession(document *Document, documentStore store.SessionStore, signer ed25519.PrivateKey, manifest SignedManifest) (*OwnerSession, error) {
	if document == nil || documentStore == nil {
		return nil, fmt.Errorf("owner session document or store is unavailable")
	}
	if err := VerifyManifest(manifest); err != nil {
		return nil, err
	}
	if len(signer) != ed25519.PrivateKeySize || !equalActorKey(manifest.Manifest.OwnerKey, signer.Public().(ed25519.PublicKey)) {
		return nil, fmt.Errorf("owner session signer does not match manifest owner")
	}
	snapshot, err := document.Snapshot(context.Background())
	if err != nil {
		return nil, fmt.Errorf("read owner document: %w", err)
	}
	return &OwnerSession{
		document:        document,
		documentID:      snapshot.DocumentID,
		documentStore:   documentStore,
		signer:          signer,
		manifest:        cloneSignedManifest(manifest),
		profileSequence: make(map[protocolv2.ActorKey]uint64),
		peerRevision:    make(map[protocolv2.ActorKey]model.Revision),
		invitations:     make(map[[32]byte]registeredInvitation),
	}, nil
}

func (session *OwnerSession) RegisterInvitation(invitation protocolv2.Invitation, now time.Time) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if err := protocolv2.VerifyInvitation(invitation, now); err != nil {
		return err
	}
	if invitation.RoomID != session.manifest.Manifest.RoomID || invitation.OwnerPublicKey != session.manifest.Manifest.OwnerKey {
		return fmt.Errorf("invitation does not belong to this owner session")
	}
	manifestDigest, err := DigestManifest(session.manifest.Manifest)
	if err != nil {
		return err
	}
	if invitation.ManifestSHA256 != manifestDigest {
		return fmt.Errorf("invitation manifest does not match this owner session")
	}
	digest := sha256.Sum256(invitation.Admission[:])
	if _, exists := session.invitations[digest]; exists {
		return fmt.Errorf("invitation admission is already registered")
	}
	session.invitations[digest] = registeredInvitation{invitation: invitation}
	return nil
}

func (session *OwnerSession) RelayAdmissions() []protocolv2.AdmissionControl {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	result := make([]protocolv2.AdmissionControl, 0, len(session.invitations))
	for digest, registered := range session.invitations {
		result = append(result, protocolv2.AdmissionControl{Digest: digest, Role: registered.invitation.Role, ExpiresAt: registered.invitation.ExpiresAt, BoundActor: registered.boundActor})
	}
	return result
}

func (session *OwnerSession) Manifest() protocolv2.RoleManifest {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	return cloneRoleManifest(session.manifest.Manifest)
}

func (session *OwnerSession) ExecuteLocal(ctx context.Context, sender protocolv2.ActorKey, operation model.Operation) (protocolv2.OperationAccepted, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	member, found := session.member(sender)
	if !found || member.Role != protocolv2.RoleOwner {
		return protocolv2.OperationAccepted{}, fmt.Errorf("local operation sender is not the owner")
	}
	response, err := session.handleOperation(ctx, member, operation)
	if err != nil {
		return protocolv2.OperationAccepted{}, err
	}
	if response.Type != protocolv2.ApplicationOperationAccepted {
		return protocolv2.OperationAccepted{}, fmt.Errorf("local owner operation was rejected")
	}
	return *response.Payload.(*protocolv2.OperationAccepted), nil
}

func (session *OwnerSession) BuildLocalInverse(ctx context.Context, sender protocolv2.ActorKey, targetID model.OperationID) (model.Operation, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	member, found := session.member(sender)
	if !found || member.Role != protocolv2.RoleOwner {
		return model.Operation{}, fmt.Errorf("local inverse sender is not the owner")
	}
	inverseID, err := model.NewOperationID()
	if err != nil {
		return model.Operation{}, err
	}
	return session.document.BuildInverse(ctx, member.ActorID, targetID, inverseID)
}

func (session *OwnerSession) Snapshot(ctx context.Context) (model.Snapshot, error) {
	return session.document.Snapshot(ctx)
}

func (session *OwnerSession) OfferOwnership(target protocolv2.ActorKey) (protocolv2.OwnershipOffer, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	member, found := session.member(target)
	if !found || member.Role != protocolv2.RoleEditor {
		return protocolv2.OwnershipOffer{}, fmt.Errorf("ownership target must be a current editor")
	}
	manifest := cloneRoleManifest(session.manifest.Manifest)
	manifest.Generation++
	manifest.OwnerKey = target
	for index := range manifest.Members {
		switch manifest.Members[index].ActorKey {
		case session.manifest.Manifest.OwnerKey:
			manifest.Members[index].Role = protocolv2.RoleEditor
		case target:
			manifest.Members[index].Role = protocolv2.RoleOwner
		}
	}
	offer := protocolv2.OwnershipOffer{TargetActor: target, NewOwnerKey: target, Generation: manifest.Generation, Manifest: manifest}
	session.pendingOwnership = &offer
	return offer, nil
}

func (session *OwnerSession) AcceptOwnership(sender protocolv2.ActorKey, acceptance protocolv2.OwnershipAccept) (protocolv2.OwnershipTransferred, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.pendingOwnership == nil || session.pendingOwnership.TargetActor != sender {
		return protocolv2.OwnershipTransferred{}, fmt.Errorf("ownership acceptance does not match a pending offer")
	}
	digest, err := protocolv2.DigestOwnershipOffer(*session.pendingOwnership)
	if err != nil {
		return protocolv2.OwnershipTransferred{}, err
	}
	if acceptance.OfferSHA256 != digest || !ed25519.Verify(ed25519.PublicKey(sender[:]), digest[:], acceptance.NewOwnerKeySignature[:]) {
		return protocolv2.OwnershipTransferred{}, fmt.Errorf("ownership acceptance signature is invalid")
	}
	manifest := cloneRoleManifest(session.pendingOwnership.Manifest)
	var oldSignature protocolv2.Signature
	copy(oldSignature[:], ed25519.Sign(session.signer, digest[:]))
	session.pendingOwnership = nil
	return protocolv2.OwnershipTransferred{Manifest: manifest, OfferSHA256: digest, OldOwnerSignature: oldSignature, NewOwnerSignature: acceptance.NewOwnerKeySignature}, nil
}

func (session *OwnerSession) Handle(ctx context.Context, sender protocolv2.ActorKey, message protocolv2.DecodedApplication) (Response, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	member, found := session.member(sender)
	if !found {
		return Response{}, fmt.Errorf("sender is not in the current manifest")
	}
	switch message.Type {
	case protocolv2.ApplicationOperationSubmit:
		payload, ok := message.Payload.(*protocolv2.OperationSubmit)
		if !ok {
			return Response{}, fmt.Errorf("operation submission payload is invalid")
		}
		return session.handleOperation(ctx, member, payload.Operation)
	case protocolv2.ApplicationInverseRequest:
		payload, ok := message.Payload.(*protocolv2.InverseRequest)
		if !ok {
			return Response{}, fmt.Errorf("inverse request payload is invalid")
		}
		return session.handleInverse(ctx, member, *payload)
	case protocolv2.ApplicationProfileUpdate:
		payload, ok := message.Payload.(*protocolv2.ProfileUpdate)
		if !ok {
			return Response{}, fmt.Errorf("profile update payload is invalid")
		}
		return session.handleProfile(sender, *payload)
	case protocolv2.ApplicationRevisionAcknowledged:
		payload, ok := message.Payload.(*protocolv2.RevisionAcknowledged)
		if !ok {
			return Response{}, fmt.Errorf("revision acknowledgement payload is invalid")
		}
		return session.handleRevisionAcknowledgement(ctx, sender, *payload)
	default:
		return Response{}, fmt.Errorf("owner session application type %q is unsupported", message.Type)
	}
}

func (session *OwnerSession) handleOperation(ctx context.Context, member protocolv2.Member, operation model.Operation) (Response, error) {
	if member.Role != protocolv2.RoleEditor && member.Role != protocolv2.RoleOwner {
		return rejection(member.ActorKey, operation.OperationID, "role_forbidden", "current role cannot edit"), nil
	}
	if operation.ActorID != member.ActorID {
		return rejection(member.ActorKey, operation.OperationID, "actor_mismatch", "operation actor does not match sender"), nil
	}
	accepted, _, err := session.document.Submit(ctx, operation)
	if err != nil {
		var rejected *engine.Rejection
		if errors.As(err, &rejected) {
			return Response{
				Type: protocolv2.ApplicationOperationRejected,
				Payload: &protocolv2.OperationRejected{
					OperationID: operation.OperationID,
					Code:        string(rejected.Code),
					Message:     "operation is not valid at the current revision",
					Revision:    rejected.CurrentRevision,
					MapHash:     rejected.CurrentMapHash,
				},
				Recipient: member.ActorKey,
			}, nil
		}
		return Response{}, err
	}
	snapshot, err := session.document.Snapshot(ctx)
	if err != nil {
		return Response{}, err
	}
	mapHash, err := snapshot.Hash()
	if err != nil {
		return Response{}, err
	}
	return Response{Type: protocolv2.ApplicationOperationAccepted, Payload: &protocolv2.OperationAccepted{Operation: accepted, MapHash: mapHash}, Broadcast: true}, nil
}

func (session *OwnerSession) handleInverse(ctx context.Context, member protocolv2.Member, request protocolv2.InverseRequest) (Response, error) {
	if member.Role != protocolv2.RoleEditor && member.Role != protocolv2.RoleOwner {
		return rejection(member.ActorKey, request.InverseID, "role_forbidden", "current role cannot edit"), nil
	}
	operation, err := session.document.BuildInverse(ctx, member.ActorID, request.OperationID, request.InverseID)
	if err != nil {
		return rejection(member.ActorKey, request.InverseID, "inverse_rejected", "inverse is not valid at the current revision"), nil
	}
	return session.handleOperation(ctx, member, operation)
}

func (session *OwnerSession) handleProfile(sender protocolv2.ActorKey, update protocolv2.ProfileUpdate) (Response, error) {
	name := strings.TrimSpace(update.DisplayName)
	if name == "" || !utf8.ValidString(name) || len(name) > MaxDisplayNameBytes {
		return Response{}, fmt.Errorf("display name is invalid")
	}
	if update.Sequence == 0 || update.Sequence <= session.profileSequence[sender] {
		return Response{}, fmt.Errorf("profile sequence is stale")
	}
	manifest := cloneRoleManifest(session.manifest.Manifest)
	for index := range manifest.Members {
		if manifest.Members[index].ActorKey == sender {
			manifest.Members[index].DisplayName = name
			manifest.Generation++
			signed, err := SignManifest(manifest, session.signer)
			if err != nil {
				return Response{}, err
			}
			session.manifest = signed
			session.profileSequence[sender] = update.Sequence
			published := cloneRoleManifest(signed.Manifest)
			return Response{Type: protocolv2.ApplicationRoleManifest, Payload: &published, Broadcast: true}, nil
		}
	}
	return Response{}, fmt.Errorf("profile sender is not in the manifest")
}

func (session *OwnerSession) handleRevisionAcknowledgement(ctx context.Context, sender protocolv2.ActorKey, acknowledgement protocolv2.RevisionAcknowledged) (Response, error) {
	hash, found, err := session.documentStore.RevisionHash(ctx, session.documentID, acknowledgement.Revision)
	if err != nil {
		return Response{}, err
	}
	if !found || hash != acknowledgement.MapHash {
		return Response{}, fmt.Errorf("revision acknowledgement does not match authoritative state")
	}
	if acknowledgement.Revision < session.peerRevision[sender] {
		return Response{}, fmt.Errorf("revision acknowledgement moves backwards")
	}
	session.peerRevision[sender] = acknowledgement.Revision
	return Response{}, nil
}

func (session *OwnerSession) member(actorKey protocolv2.ActorKey) (protocolv2.Member, bool) {
	for _, member := range session.manifest.Manifest.Members {
		if member.ActorKey == actorKey {
			return member, true
		}
	}
	return protocolv2.Member{}, false
}

func rejection(recipient protocolv2.ActorKey, operationID model.OperationID, code, message string) Response {
	return Response{
		Type:      protocolv2.ApplicationOperationRejected,
		Payload:   &protocolv2.OperationRejected{OperationID: operationID, Code: code, Message: message},
		Recipient: recipient,
	}
}

func cloneSignedManifest(manifest SignedManifest) SignedManifest {
	manifest.Manifest = cloneRoleManifest(manifest.Manifest)
	return manifest
}

func cloneRoleManifest(manifest protocolv2.RoleManifest) protocolv2.RoleManifest {
	manifest.Members = append([]protocolv2.Member(nil), manifest.Members...)
	return manifest
}
