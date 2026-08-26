package load_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/auth"
	loadscenario "sdmm/internal/aphelion/collab/load"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/server"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestRunnerUsesHostedInvitations(t *testing.T) {
	scenario, err := loadscenario.Generate(loadscenario.Config{Seed: 86, Clients: 2, Operations: 4, PresencePerClient: 1, MaxX: 2, MaxY: 2})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	ownerID, _ := model.NewActorID()
	firstID, _ := model.NewActorID()
	secondID, _ := model.NewActorID()
	backend := newLoadHostedBackend(map[string]auth.Session{
		"owner-secret":  {Token: "owner-secret", ActorID: ownerID, Issuer: "https://issuer.example", Subject: "owner", DisplayName: "Owner", Role: auth.RoleViewer, ExpiresAt: now.Add(time.Hour)},
		"first-secret":  {Token: "first-secret", ActorID: firstID, Issuer: "https://issuer.example", Subject: "first", DisplayName: "First", Role: auth.RoleViewer, ExpiresAt: now.Add(time.Hour)},
		"second-secret": {Token: "second-secret", ActorID: secondID, Issuer: "https://issuer.example", Subject: "second", DisplayName: "Second", Role: auth.RoleViewer, ExpiresAt: now.Add(time.Hour)},
	})
	service := server.NewService(server.ServiceConfig{HostedAuth: backend, HostedRegistry: backend, AllowedOrigins: []string{"http://127.0.0.1"}, Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	var joinTokenRequests atomic.Int32
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/sessions/"+string(scenario.Initial.DocumentID)+"/join-tokens" {
			joinTokenRequests.Add(1)
		}
		service.Handler().ServeHTTP(writer, request)
	})
	testServer := httptest.NewServer(handler)
	t.Cleanup(testServer.Close)

	body, _ := json.Marshal(map[string]any{"snapshot": scenario.Initial})
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, testServer.URL+"/v1/hosted/sessions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer owner-secret")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create hosted session status = %d", response.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := loadscenario.Run(ctx, loadscenario.RunConfig{
		Endpoint: testServer.URL, Origin: "http://127.0.0.1", SessionID: string(scenario.Initial.DocumentID), OwnerToken: "owner-secret", EditorTokens: []string{"first-secret", "second-secret"},
	}, scenario)
	if err != nil {
		t.Fatal(err)
	}
	if joinTokenRequests.Load() != 0 {
		t.Fatalf("embedded join-token requests = %d", joinTokenRequests.Load())
	}
	if !result.GatePassed {
		t.Fatalf("result = %#v", result)
	}
}

type loadHostedBackend struct {
	mu          sync.Mutex
	tokens      map[string]auth.Session
	sessions    map[string]collabstore.HostedSession
	members     map[string]map[string]collabstore.HostedMember
	invitations map[[sha256.Size]byte]collabstore.HostedInvitation
}

func newLoadHostedBackend(tokens map[string]auth.Session) *loadHostedBackend {
	return &loadHostedBackend{tokens: tokens, sessions: make(map[string]collabstore.HostedSession), members: make(map[string]map[string]collabstore.HostedMember), invitations: make(map[[sha256.Size]byte]collabstore.HostedInvitation)}
}

func (backend *loadHostedBackend) Authorize(_ context.Context, token string) (auth.Session, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	session, ok := backend.tokens[token]
	if !ok {
		return auth.Session{}, auth.ErrInvalidSession
	}
	return session, nil
}

func (backend *loadHostedBackend) AuthorizeSession(ctx context.Context, token, sessionID string) (auth.Session, error) {
	session, err := backend.Authorize(ctx, token)
	if err != nil {
		return auth.Session{}, err
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	member, found := backend.members[sessionID][loadIdentityKey(session.Issuer, session.Subject)]
	if !found || member.Disabled {
		return auth.Session{}, auth.ErrSessionAccessDenied
	}
	session.Role = auth.Role(member.Role)
	return session, nil
}

func (backend *loadHostedBackend) CreateHostedSession(_ context.Context, session collabstore.HostedSession, owner collabstore.HostedMember) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if _, exists := backend.sessions[session.SessionID]; exists {
		return collabstore.ErrHostedSessionExists
	}
	backend.sessions[session.SessionID] = session
	backend.members[session.SessionID] = map[string]collabstore.HostedMember{loadIdentityKey(owner.Issuer, owner.Subject): owner}
	return nil
}

func (backend *loadHostedBackend) ListHostedSessions(context.Context) ([]collabstore.HostedSession, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	result := make([]collabstore.HostedSession, 0, len(backend.sessions))
	for _, session := range backend.sessions {
		result = append(result, session)
	}
	return result, nil
}

func (backend *loadHostedBackend) ListHostedMembers(_ context.Context, sessionID string) ([]collabstore.HostedMember, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	result := make([]collabstore.HostedMember, 0, len(backend.members[sessionID]))
	for _, member := range backend.members[sessionID] {
		result = append(result, member)
	}
	return result, nil
}

func (backend *loadHostedBackend) ResolveHostedMember(_ context.Context, sessionID, issuer, subject string) (collabstore.HostedMember, bool, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	member, found := backend.members[sessionID][loadIdentityKey(issuer, subject)]
	return member, found, nil
}

func (backend *loadHostedBackend) UpdateHostedMemberDisplayName(_ context.Context, sessionID string, actorID model.ActorID, displayName string) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	for key, member := range backend.members[sessionID] {
		if member.ActorID == actorID {
			member.DisplayName = displayName
			backend.members[sessionID][key] = member
			return nil
		}
	}
	return collabstore.ErrHostedSessionMissing
}

func (backend *loadHostedBackend) CreateHostedInvitation(_ context.Context, invitation collabstore.HostedInvitation) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.invitations[invitation.TokenHash] = invitation
	return nil
}

func (backend *loadHostedBackend) RedeemHostedInvitation(_ context.Context, sessionID string, tokenHash [sha256.Size]byte, identity collabstore.HostedIdentity, now time.Time) (collabstore.HostedMember, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	invitation, found := backend.invitations[tokenHash]
	if !found || invitation.SessionID != sessionID || !now.Before(invitation.ExpiresAt) {
		return collabstore.HostedMember{}, collabstore.ErrHostedInvitationInvalid
	}
	delete(backend.invitations, tokenHash)
	key := loadIdentityKey(identity.Issuer, identity.Subject)
	if _, exists := backend.members[sessionID][key]; exists {
		return collabstore.HostedMember{}, collabstore.ErrHostedMemberExists
	}
	member := collabstore.HostedMember{SessionID: sessionID, Issuer: identity.Issuer, Subject: identity.Subject, ActorID: identity.ActorID, DisplayName: identity.DisplayName, Role: invitation.Role}
	backend.members[sessionID][key] = member
	return member, nil
}

func loadIdentityKey(issuer, subject string) string {
	return issuer + "\x00" + subject
}

var _ server.HostedSessionAuthorizer = (*loadHostedBackend)(nil)
var _ collabstore.HostedRegistry = (*loadHostedBackend)(nil)
