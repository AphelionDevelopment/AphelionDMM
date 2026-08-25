package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/auth"
	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func TestHostedLifecycleBindsOIDCIdentityToPersistentInvitation(t *testing.T) {
	now := time.Unix(10_000, 0).UTC()
	ownerActor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	joinedActor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	backend := newFakeHostedBackend(map[string]auth.Session{
		"owner-auth":  {Token: "owner-auth", ActorID: ownerActor, Issuer: "https://issuer.example", Subject: "owner", DisplayName: "Owner", Role: auth.RoleViewer, ExpiresAt: now.Add(time.Hour)},
		"joined-auth": {Token: "joined-auth", ActorID: joinedActor, Issuer: "https://issuer.example", Subject: "joined", DisplayName: "Joined", Role: auth.RoleViewer, ExpiresAt: now.Add(time.Hour)},
	})
	service := NewService(ServiceConfig{HostedAuth: backend, HostedRegistry: backend, AllowedOrigins: []string{"https://maps.example"}, Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	testServer := httptest.NewServer(service.Handler())
	t.Cleanup(testServer.Close)

	snapshot := testSnapshot(t, 2)
	createBody, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	createdResponse := postJSON(t, testServer.URL+"/v1/hosted/sessions", "owner-auth", createBody)
	defer func() { _ = createdResponse.Body.Close() }()
	if createdResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create hosted session status = %d", createdResponse.StatusCode)
	}
	var created HostedSessionResponse
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.SessionID != string(snapshot.DocumentID) {
		t.Fatalf("created hosted session = %#v", created)
	}

	invitationBody := []byte(`{"role":"editor"}`)
	invitationResponse := postJSON(t, testServer.URL+"/v1/sessions/"+created.SessionID+"/hosted-invitations", "owner-auth", invitationBody)
	defer func() { _ = invitationResponse.Body.Close() }()
	if invitationResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create hosted invitation status = %d", invitationResponse.StatusCode)
	}
	var invitation struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(invitationResponse.Body).Decode(&invitation); err != nil {
		t.Fatal(err)
	}
	if invitation.Token == "" || !invitation.ExpiresAt.After(now) || backend.hasRawInvitation(invitation.Token) {
		t.Fatalf("hosted invitation was empty, expired, or stored raw")
	}

	redeemBody, _ := json.Marshal(map[string]string{"token": invitation.Token})
	redeemResponse := postJSON(t, testServer.URL+"/v1/sessions/"+created.SessionID+"/hosted-invitations/redeem", "joined-auth", redeemBody)
	_ = redeemResponse.Body.Close()
	if redeemResponse.StatusCode != http.StatusOK {
		t.Fatalf("redeem hosted invitation status = %d", redeemResponse.StatusCode)
	}
	reusedResponse := postJSON(t, testServer.URL+"/v1/sessions/"+created.SessionID+"/hosted-invitations/redeem", "joined-auth", redeemBody)
	_ = reusedResponse.Body.Close()
	if reusedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused hosted invitation status = %d", reusedResponse.StatusCode)
	}

	snapshotRequest, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, testServer.URL+"/v1/sessions/"+created.SessionID+"/snapshot", nil)
	snapshotRequest.Header.Set("Authorization", "Bearer joined-auth")
	snapshotResponse, err := http.DefaultClient.Do(snapshotRequest)
	if err != nil {
		t.Fatal(err)
	}
	_ = snapshotResponse.Body.Close()
	if snapshotResponse.StatusCode != http.StatusOK {
		t.Fatalf("joined snapshot status = %d", snapshotResponse.StatusCode)
	}
}

func TestHostedOwnerCannotMintEmbeddedJoinToken(t *testing.T) {
	now := time.Unix(15_000, 0).UTC()
	ownerActor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	backend := newFakeHostedBackend(map[string]auth.Session{
		"owner-auth": {Token: "owner-auth", ActorID: ownerActor, Issuer: "https://issuer.example", Subject: "owner", DisplayName: "Owner", Role: auth.RoleViewer, ExpiresAt: now.Add(time.Hour)},
	})
	service := NewService(ServiceConfig{HostedAuth: backend, HostedRegistry: backend, Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	testServer := httptest.NewServer(service.Handler())
	t.Cleanup(testServer.Close)

	snapshot := testSnapshot(t, 2)
	createBody, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	createdResponse := postJSON(t, testServer.URL+"/v1/hosted/sessions", "owner-auth", createBody)
	_ = createdResponse.Body.Close()
	if createdResponse.StatusCode != http.StatusCreated {
		t.Fatalf("create hosted session status = %d", createdResponse.StatusCode)
	}

	joinTokenResponse := postJSON(t, testServer.URL+"/v1/sessions/"+string(snapshot.DocumentID)+"/join-tokens", "owner-auth", []byte(`{"role":"editor","display_name":"Bypass"}`))
	_ = joinTokenResponse.Body.Close()
	if joinTokenResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("hosted embedded join-token status = %d, want %d", joinTokenResponse.StatusCode, http.StatusForbidden)
	}
}

func TestRecoverHostedSessionsRestoresDurableMembership(t *testing.T) {
	now := time.Unix(20_000, 0).UTC()
	ownerActor, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	backend := newFakeHostedBackend(map[string]auth.Session{
		"owner-auth": {Token: "owner-auth", ActorID: ownerActor, Issuer: "https://issuer.example", Subject: "owner", DisplayName: "Owner", Role: auth.RoleViewer, ExpiresAt: now.Add(time.Hour)},
	})
	durable := &nonClosingSessionStore{SessionStore: NewMemoryStore()}
	first := NewService(ServiceConfig{Store: durable, HostedAuth: backend, HostedRegistry: backend, Now: func() time.Time { return now }})
	snapshot := testSnapshot(t, 2)
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	firstServer := httptest.NewServer(first.Handler())
	created := postJSON(t, firstServer.URL+"/v1/hosted/sessions", "owner-auth", body)
	_ = created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create hosted session status = %d", created.StatusCode)
	}
	firstServer.Close()
	if err := first.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	second := NewService(ServiceConfig{Store: durable, HostedAuth: backend, HostedRegistry: backend, Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = second.Shutdown(context.Background()) })
	if err := second.RecoverHostedSessions(context.Background()); err != nil {
		t.Fatal(err)
	}
	secondServer := httptest.NewServer(second.Handler())
	t.Cleanup(secondServer.Close)
	request, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, secondServer.URL+"/v1/sessions/"+string(snapshot.DocumentID)+"/snapshot", nil)
	request.Header.Set("Authorization", "Bearer owner-auth")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("recovered hosted snapshot status = %d", response.StatusCode)
	}
}

func TestHostedAuthenticationEndpointsDoNotExposeStateSeparately(t *testing.T) {
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	login := &stubHostedLogin{session: auth.Session{Token: "session-secret", ActorID: actorID, DisplayName: "Mapper", ExpiresAt: time.Now().Add(time.Hour)}}
	service := NewService(ServiceConfig{HostedLogin: login})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	testServer := httptest.NewServer(service.Handler())
	t.Cleanup(testServer.Close)
	begin := postJSON(t, testServer.URL+"/v1/auth/begin", "", nil)
	var beginBody map[string]any
	if err := json.NewDecoder(begin.Body).Decode(&beginBody); err != nil {
		t.Fatal(err)
	}
	_ = begin.Body.Close()
	if begin.StatusCode != http.StatusOK || begin.Header.Get("Cache-Control") != "no-store" || beginBody["authorization_url"] == "" || beginBody["state"] != nil {
		t.Fatalf("begin response = status %d body %#v", begin.StatusCode, beginBody)
	}
	complete, err := http.Get(testServer.URL + "/v1/auth/complete?state=state-value&code=code-value")
	if err != nil {
		t.Fatal(err)
	}
	var completeBody map[string]any
	if err := json.NewDecoder(complete.Body).Decode(&completeBody); err != nil {
		t.Fatal(err)
	}
	_ = complete.Body.Close()
	if complete.StatusCode != http.StatusOK || complete.Header.Get("Cache-Control") != "no-store" || completeBody["token"] != "session-secret" {
		t.Fatalf("complete response = status %d body %#v", complete.StatusCode, completeBody)
	}
	logout := postJSON(t, testServer.URL+"/v1/auth/logout", "session-secret", nil)
	_ = logout.Body.Close()
	if logout.StatusCode != http.StatusNoContent || login.loggedOut != "session-secret" {
		t.Fatalf("logout status/token = %d/%q", logout.StatusCode, login.loggedOut)
	}
}

type stubHostedLogin struct {
	session   auth.Session
	loggedOut string
}

func (*stubHostedLogin) Begin(context.Context) (auth.BeginResult, error) {
	return auth.BeginResult{AuthorizationURL: "https://issuer.example/authorize?state=state-value", State: "state-value"}, nil
}

func (login *stubHostedLogin) Complete(_ context.Context, state, code string) (auth.Session, error) {
	if state != "state-value" || code != "code-value" {
		return auth.Session{}, auth.ErrInvalidState
	}
	return login.session, nil
}

func (login *stubHostedLogin) Logout(token string) { login.loggedOut = token }

type nonClosingSessionStore struct {
	SessionStore
}

func (*nonClosingSessionStore) Close() error { return nil }

type fakeHostedBackend struct {
	mutex       sync.Mutex
	auth        map[string]auth.Session
	sessions    map[string]collabstore.HostedSession
	members     map[string]map[string]collabstore.HostedMember
	invitations map[[sha256.Size]byte]collabstore.HostedInvitation
	redeemed    map[[sha256.Size]byte]bool
}

func newFakeHostedBackend(authentication map[string]auth.Session) *fakeHostedBackend {
	return &fakeHostedBackend{auth: authentication, sessions: make(map[string]collabstore.HostedSession), members: make(map[string]map[string]collabstore.HostedMember), invitations: make(map[[sha256.Size]byte]collabstore.HostedInvitation), redeemed: make(map[[sha256.Size]byte]bool)}
}

func (backend *fakeHostedBackend) Authorize(_ context.Context, token string) (auth.Session, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	session, found := backend.auth[token]
	if !found {
		return auth.Session{}, auth.ErrInvalidSession
	}
	return session, nil
}

func (backend *fakeHostedBackend) AuthorizeSession(ctx context.Context, token, sessionID string) (auth.Session, error) {
	session, err := backend.Authorize(ctx, token)
	if err != nil {
		return auth.Session{}, err
	}
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	member, found := backend.members[sessionID][session.Issuer+"\x00"+session.Subject]
	if !found || member.Disabled {
		return auth.Session{}, auth.ErrSessionAccessDenied
	}
	session.Role = auth.Role(member.Role)
	return session, nil
}

func (backend *fakeHostedBackend) CreateHostedSession(_ context.Context, session collabstore.HostedSession, owner collabstore.HostedMember) error {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	if _, exists := backend.sessions[session.SessionID]; exists {
		return collabstore.ErrHostedSessionExists
	}
	backend.sessions[session.SessionID] = session
	backend.members[session.SessionID] = map[string]collabstore.HostedMember{owner.Issuer + "\x00" + owner.Subject: owner}
	return nil
}

func (backend *fakeHostedBackend) ListHostedSessions(context.Context) ([]collabstore.HostedSession, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	result := make([]collabstore.HostedSession, 0, len(backend.sessions))
	for _, session := range backend.sessions {
		result = append(result, session)
	}
	return result, nil
}

func (backend *fakeHostedBackend) ListHostedMembers(_ context.Context, sessionID string) ([]collabstore.HostedMember, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	result := make([]collabstore.HostedMember, 0, len(backend.members[sessionID]))
	for _, member := range backend.members[sessionID] {
		result = append(result, member)
	}
	return result, nil
}

func (backend *fakeHostedBackend) ResolveHostedMember(_ context.Context, sessionID, issuer, subject string) (collabstore.HostedMember, bool, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	member, found := backend.members[sessionID][issuer+"\x00"+subject]
	return member, found, nil
}

func (backend *fakeHostedBackend) CreateHostedInvitation(_ context.Context, invitation collabstore.HostedInvitation) error {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	backend.invitations[invitation.TokenHash] = invitation
	return nil
}

func (backend *fakeHostedBackend) RedeemHostedInvitation(_ context.Context, expectedSessionID string, tokenHash [sha256.Size]byte, identity collabstore.HostedIdentity, now time.Time) (collabstore.HostedMember, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	invitation, found := backend.invitations[tokenHash]
	if !found || invitation.SessionID != expectedSessionID || backend.redeemed[tokenHash] || !now.Before(invitation.ExpiresAt) {
		return collabstore.HostedMember{}, collabstore.ErrHostedInvitationInvalid
	}
	member := collabstore.HostedMember{SessionID: invitation.SessionID, Issuer: identity.Issuer, Subject: identity.Subject, ActorID: identity.ActorID, DisplayName: identity.DisplayName, Role: invitation.Role}
	backend.members[invitation.SessionID][identity.Issuer+"\x00"+identity.Subject] = member
	backend.redeemed[tokenHash] = true
	return member, nil
}

func (backend *fakeHostedBackend) hasRawInvitation(token string) bool {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	for hash := range backend.invitations {
		if string(hash[:]) == token {
			return true
		}
	}
	return false
}

var _ HostedSessionAuthorizer = (*fakeHostedBackend)(nil)
var _ collabstore.HostedRegistry = (*fakeHostedBackend)(nil)
