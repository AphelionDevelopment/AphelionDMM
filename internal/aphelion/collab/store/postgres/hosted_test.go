package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestHostedRegistryPersistsSessionMembershipAndOneUseInvitation(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	ownerActor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Unix(1000, 0).UTC()
	owner := collabstore.HostedMember{SessionID: "hosted-session", Issuer: "https://issuer.example", Subject: "owner", ActorID: ownerActor, DisplayName: "Owner", Role: collabstore.HostedRoleOwner}
	if err := value.CreateHostedSession(context.Background(), collabstore.HostedSession{SessionID: owner.SessionID, DocumentID: fixture.Initial.DocumentID, CreatedAt: createdAt}, owner); err != nil {
		t.Fatal(err)
	}
	sessions, err := value.ListHostedSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != owner.SessionID || sessions[0].DocumentID != fixture.Initial.DocumentID {
		t.Fatalf("hosted sessions = %#v", sessions)
	}
	loadedOwner, found, err := value.ResolveHostedMember(context.Background(), owner.SessionID, owner.Issuer, owner.Subject)
	if err != nil || !found || loadedOwner.ActorID != owner.ActorID || loadedOwner.Role != collabstore.HostedRoleOwner {
		t.Fatalf("resolved owner = %#v/%t/%v", loadedOwner, found, err)
	}

	tokenHash := sha256.Sum256([]byte("one-use-invitation"))
	invitation := collabstore.HostedInvitation{TokenHash: tokenHash, SessionID: owner.SessionID, Role: collabstore.HostedRoleEditor, CreatedByActorID: owner.ActorID, ExpiresAt: createdAt.Add(time.Minute)}
	if err := value.CreateHostedInvitation(context.Background(), invitation); err != nil {
		t.Fatal(err)
	}
	joinedActor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	joined, err := value.RedeemHostedInvitation(context.Background(), owner.SessionID, tokenHash, collabstore.HostedIdentity{Issuer: "https://issuer.example", Subject: "joined", ActorID: joinedActor, DisplayName: "Joined"}, createdAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if joined.Role != collabstore.HostedRoleEditor || joined.SessionID != owner.SessionID {
		t.Fatalf("joined member = %#v", joined)
	}
	if _, err := value.RedeemHostedInvitation(context.Background(), owner.SessionID, tokenHash, collabstore.HostedIdentity{Issuer: "https://issuer.example", Subject: "reused", ActorID: joinedActor, DisplayName: "Reused"}, createdAt.Add(2*time.Second)); !errors.Is(err, collabstore.ErrHostedInvitationInvalid) {
		t.Fatalf("reused invitation error = %v", err)
	}
}

func TestHostedRegistryRejectsExpiredInvitationWithoutCreatingMember(t *testing.T) {
	dsn, schema := isolatedSchema(t)
	fixture, err := collabstore.NewConformanceFixture()
	if err != nil {
		t.Fatal(err)
	}
	value, err := Open(context.Background(), Config{DSN: dsn, Schema: schema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = value.Close() })
	if err := value.Create(context.Background(), fixture.Initial); err != nil {
		t.Fatal(err)
	}
	actorID, _ := model.NewActorID()
	owner := collabstore.HostedMember{SessionID: "expired-session", Issuer: "https://issuer.example", Subject: "owner", ActorID: actorID, DisplayName: "Owner", Role: collabstore.HostedRoleOwner}
	if err := value.CreateHostedSession(context.Background(), collabstore.HostedSession{SessionID: owner.SessionID, DocumentID: fixture.Initial.DocumentID, CreatedAt: time.Unix(2000, 0).UTC()}, owner); err != nil {
		t.Fatal(err)
	}
	tokenHash := sha256.Sum256([]byte("expired"))
	if err := value.CreateHostedInvitation(context.Background(), collabstore.HostedInvitation{TokenHash: tokenHash, SessionID: owner.SessionID, Role: collabstore.HostedRoleViewer, CreatedByActorID: owner.ActorID, ExpiresAt: time.Unix(2001, 0).UTC()}); err != nil {
		t.Fatal(err)
	}
	joinedActor, _ := model.NewActorID()
	_, err = value.RedeemHostedInvitation(context.Background(), owner.SessionID, tokenHash, collabstore.HostedIdentity{Issuer: owner.Issuer, Subject: "late", ActorID: joinedActor, DisplayName: "Late"}, time.Unix(2002, 0).UTC())
	if !errors.Is(err, collabstore.ErrHostedInvitationInvalid) {
		t.Fatalf("expired invitation error = %v", err)
	}
	if _, found, err := value.ResolveHostedMember(context.Background(), owner.SessionID, owner.Issuer, "late"); err != nil || found {
		t.Fatalf("expired invitation member found = %t, error = %v", found, err)
	}
}
