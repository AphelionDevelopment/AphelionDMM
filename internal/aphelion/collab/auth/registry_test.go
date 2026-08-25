package auth

import (
	"context"
	"errors"
	"testing"

	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestRegistryDirectorySeparatesLoginFromSessionMembership(t *testing.T) {
	identity := Identity{Issuer: "https://issuer.example", Subject: "subject", DisplayName: "Mapper"}
	actorID, err := actorIDForIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &fakeHostedMemberResolver{found: true, member: collabstore.HostedMember{SessionID: "session", Issuer: identity.Issuer, Subject: identity.Subject, ActorID: actorID, DisplayName: identity.DisplayName, Role: collabstore.HostedRoleEditor}}
	directory := NewRegistryDirectory(resolver)
	loginRole, disabled, err := directory.Resolve(context.Background(), identity)
	if err != nil || disabled || loginRole != RoleViewer {
		t.Fatalf("login resolution = %q/%t/%v", loginRole, disabled, err)
	}
	sessionRole, disabled, err := directory.ResolveSession(context.Background(), identity, "session")
	if err != nil || disabled || sessionRole != RoleEditor {
		t.Fatalf("session resolution = %q/%t/%v", sessionRole, disabled, err)
	}
	resolver.found = false
	if _, _, err := directory.ResolveSession(context.Background(), identity, "other"); !errors.Is(err, ErrSessionAccessDenied) {
		t.Fatalf("missing membership error = %v", err)
	}
}

type fakeHostedMemberResolver struct {
	member collabstore.HostedMember
	found  bool
}

func (resolver *fakeHostedMemberResolver) ResolveHostedMember(context.Context, string, string, string) (collabstore.HostedMember, bool, error) {
	return resolver.member, resolver.found, nil
}
