package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestManagerBindsStateSubjectRoleChangeAndLogout(t *testing.T) {
	t.Parallel()

	now := time.Unix(1000, 0).UTC()
	flow := &fakeFlow{identity: Identity{Issuer: "https://issuer.example", Subject: "subject-1", DisplayName: "Mapper", ExpiresAt: now.Add(time.Hour)}}
	directory := &fakeDirectory{role: RoleEditor}
	manager := NewManager(flow, directory, ManagerConfig{Now: func() time.Time { return now }})
	begin, err := manager.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if begin.State == "" || begin.AuthorizationURL == "" || flow.state != begin.State || flow.nonce == "" || flow.verifier == "" {
		t.Fatalf("begin = %#v flow = %#v", begin, flow)
	}
	session, err := manager.Complete(context.Background(), begin.State, "authorization-code")
	if err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.ActorID == "" || session.Role != RoleEditor || session.Subject != flow.identity.Subject {
		t.Fatalf("session = %#v", session)
	}
	if _, err := manager.Complete(context.Background(), begin.State, "authorization-code"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("reused state error = %v, want invalid state", err)
	}
	directory.role = RoleViewer
	updated, err := manager.Authorize(context.Background(), session.Token)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != RoleViewer || updated.ActorID != session.ActorID {
		t.Fatalf("reauthorized session = %#v", updated)
	}
	manager.Logout(session.Token)
	if _, err := manager.Authorize(context.Background(), session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("logged-out authorization error = %v", err)
	}
}

func TestManagerRejectsExpiredStateDisabledUserAndExpiredToken(t *testing.T) {
	t.Parallel()

	now := time.Unix(2000, 0).UTC()
	flow := &fakeFlow{identity: Identity{Issuer: "https://issuer.example", Subject: "subject-2", DisplayName: "Mapper", ExpiresAt: now.Add(time.Hour)}}
	directory := &fakeDirectory{role: RoleOwner}
	manager := NewManager(flow, directory, ManagerConfig{Now: func() time.Time { return now }, PendingTTL: time.Minute})
	begin, err := manager.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := manager.Complete(context.Background(), begin.State, "code"); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("expired state error = %v", err)
	}

	now = time.Unix(3000, 0).UTC()
	begin, err = manager.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	directory.disabled = true
	if _, err := manager.Complete(context.Background(), begin.State, "code"); !errors.Is(err, ErrDisabledIdentity) {
		t.Fatalf("disabled completion error = %v", err)
	}
	directory.disabled = false
	begin, _ = manager.Begin(context.Background())
	session, err := manager.Complete(context.Background(), begin.State, "code")
	if err != nil {
		t.Fatal(err)
	}
	now = flow.identity.ExpiresAt.Add(time.Second)
	if _, err := manager.Authorize(context.Background(), session.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired session error = %v", err)
	}
}

func TestManagerReauthorizesRoleForSpecificCollaborationSession(t *testing.T) {
	t.Parallel()

	now := time.Unix(4000, 0).UTC()
	flow := &fakeFlow{identity: Identity{Issuer: "https://issuer.example", Subject: "subject-3", DisplayName: "Mapper", ExpiresAt: now.Add(time.Hour)}}
	directory := &fakeDirectory{role: RoleViewer}
	sessionDirectory := &fakeSessionDirectory{roles: map[string]Role{"session-a": RoleEditor, "session-b": RoleViewer}}
	manager := NewManager(flow, directory, ManagerConfig{Now: func() time.Time { return now }, SessionDirectory: sessionDirectory})
	begin, err := manager.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	authentication, err := manager.Complete(context.Background(), begin.State, "code")
	if err != nil {
		t.Fatal(err)
	}
	access, err := manager.AuthorizeSession(context.Background(), authentication.Token, "session-a")
	if err != nil {
		t.Fatal(err)
	}
	if access.Role != RoleEditor || access.ActorID != authentication.ActorID || sessionDirectory.sessionID != "session-a" {
		t.Fatalf("session access = %#v", access)
	}
	sessionDirectory.roles["session-a"] = RoleViewer
	access, err = manager.AuthorizeSession(context.Background(), authentication.Token, "session-a")
	if err != nil {
		t.Fatal(err)
	}
	if access.Role != RoleViewer {
		t.Fatalf("updated role = %q", access.Role)
	}
	sessionDirectory.disabled = true
	if _, err := manager.AuthorizeSession(context.Background(), authentication.Token, "session-a"); !errors.Is(err, ErrDisabledIdentity) {
		t.Fatalf("disabled session access error = %v", err)
	}
}

type fakeFlow struct {
	identity Identity
	state    string
	nonce    string
	verifier string
}

func (flow *fakeFlow) AuthorizationURL(state, nonce, verifier string) string {
	flow.state, flow.nonce, flow.verifier = state, nonce, verifier
	return "https://issuer.example/authorize?state=" + state
}

func (flow *fakeFlow) Exchange(context.Context, string, string, string) (Identity, error) {
	return flow.identity, nil
}

type fakeDirectory struct {
	role     Role
	disabled bool
}

func (directory *fakeDirectory) Resolve(context.Context, Identity) (Role, bool, error) {
	return directory.role, directory.disabled, nil
}

type fakeSessionDirectory struct {
	roles     map[string]Role
	disabled  bool
	sessionID string
}

func (directory *fakeSessionDirectory) ResolveSession(_ context.Context, _ Identity, sessionID string) (Role, bool, error) {
	directory.sessionID = sessionID
	return directory.roles[sessionID], directory.disabled, nil
}
