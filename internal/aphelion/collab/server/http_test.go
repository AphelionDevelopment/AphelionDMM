package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sdmm/internal/aphelion/collab/auth"
	"sdmm/internal/aphelion/collab/compat"
	"sdmm/internal/aphelion/collab/model"
	"sdmm/internal/aphelion/collab/protocol"
)

func TestValidateListenAddressRequiresLoopback(t *testing.T) {
	t.Parallel()

	for _, address := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		if err := ValidateListenAddress(address, false); err != nil {
			t.Errorf("ValidateListenAddress(%q) error = %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:0", "[::]:0", "192.0.2.1:8080"} {
		if err := ValidateListenAddress(address, false); err == nil {
			t.Errorf("ValidateListenAddress(%q) error = nil", address)
		}
	}
}

func TestHTTPLaunchTokenIsSingleUseAndBodiesAreBounded(t *testing.T) {
	t.Parallel()

	service := NewService(ServiceConfig{AllowedOrigins: []string{"http://127.0.0.1"}})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	launchToken, err := service.NewLaunchToken()
	if err != nil {
		t.Fatal(err)
	}
	testServer := httptest.NewServer(service.Handler())
	defer testServer.Close()

	snapshot := testSnapshot(t, 1)
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	response := postJSON(t, testServer.URL+"/v1/sessions", launchToken, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", response.StatusCode, http.StatusCreated)
	}
	var created CreateSessionResponse
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.SessionID == "" || created.OwnerToken == "" {
		t.Fatalf("created session lacks identifiers: %#v", created)
	}

	second := postJSON(t, testServer.URL+"/v1/sessions", launchToken, body)
	defer func() { _ = second.Body.Close() }()
	if second.StatusCode != http.StatusUnauthorized {
		t.Fatalf("second token redemption status = %d, want %d", second.StatusCode, http.StatusUnauthorized)
	}

	oversized := bytes.Repeat([]byte("x"), MaxHTTPBodyBytes+1)
	tooLarge := postJSON(t, testServer.URL+"/v1/sessions", "invalid", oversized)
	defer func() { _ = tooLarge.Body.Close() }()
	if tooLarge.StatusCode != http.StatusRequestEntityTooLarge && tooLarge.StatusCode != http.StatusUnauthorized {
		t.Fatalf("oversized status = %d, want bounded rejection", tooLarge.StatusCode)
	}
}

func TestSessionCreationUsesSnapshotBodyLimit(t *testing.T) {
	t.Parallel()

	service := NewService(ServiceConfig{Limits: Limits{MaxHTTPBodyBytes: 64}})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	launchToken, err := service.NewLaunchToken()
	if err != nil {
		t.Fatal(err)
	}
	stableID, err := model.NewStableID()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot(t, 1)
	snapshot.Tiles = []model.Tile{{
		Coord: model.Coord{X: 1, Y: 1, Z: 1},
		State: model.TileState{Prefabs: []model.PrefabState{{
			StableID: stableID,
			Path:     "/turf/open/floor",
			Vars:     map[string]string{"large_fixture": strings.Repeat("x", MaxHTTPBodyBytes)},
		}}},
	}}
	testServer := httptest.NewServer(service.Handler())
	t.Cleanup(testServer.Close)
	body, err := json.Marshal(map[string]any{"snapshot": snapshot})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= MaxHTTPBodyBytes {
		t.Fatalf("session fixture is %d bytes, want more than %d-byte generic HTTP limit", len(body), MaxHTTPBodyBytes)
	}
	response := postJSON(t, testServer.URL+"/v1/sessions", launchToken, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", response.StatusCode, http.StatusCreated)
	}
}

func TestHTTPHealthAndVersion(t *testing.T) {
	t.Parallel()

	service := NewService(ServiceConfig{})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	testServer := httptest.NewServer(service.Handler())
	defer testServer.Close()
	for _, path := range []string{"/v1/health/live", "/v1/health/ready", "/v1/version"} {
		response, err := http.Get(testServer.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			t.Fatalf("GET %s status = %d, want 200", path, response.StatusCode)
		}
		_ = response.Body.Close()
	}
}

func TestReadyFailsWhenDurableStoreIsUnavailable(t *testing.T) {
	store := &failingReadinessStore{SessionStore: NewMemoryStore(), err: errors.New("database unavailable")}
	service := NewService(ServiceConfig{Store: store})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	request := httptest.NewRequest(http.MethodGet, "/v1/health/ready", nil)
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("readiness status = %d", response.Code)
	}
}

type failingReadinessStore struct {
	SessionStore
	err error
}

func (store *failingReadinessStore) Ready(context.Context) error { return store.err }

func TestHTTPVersionPublishesCompatibilityMatrix(t *testing.T) {
	t.Parallel()

	matrix := compat.Matrix{
		Releases: []compat.Release{
			{Name: "previous", ProtocolVersions: []uint16{1}, SchemaVersions: []uint16{1}},
			{Name: compat.CurrentRelease, ProtocolVersions: []uint16{1}, SchemaVersions: []uint16{1}},
		},
		RollingPairs: []compat.RollingPair{{From: "previous", To: compat.CurrentRelease}},
	}
	service := NewService(ServiceConfig{Compatibility: matrix})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	request := httptest.NewRequest(http.MethodGet, "/v1/version", nil)
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /v1/version status = %d", response.Code)
	}
	var body struct {
		Compatibility compat.Matrix `json:"compatibility"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Compatibility.Releases) != 2 || len(body.Compatibility.RollingPairs) != 1 {
		t.Fatalf("compatibility = %#v", body.Compatibility)
	}
}

func TestHTTPLaunchTokenExpires(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	service := NewService(ServiceConfig{LaunchTokenTTL: time.Minute, Now: func() time.Time { return now }})
	t.Cleanup(func() { _ = service.Shutdown(context.Background()) })
	token, err := service.NewLaunchToken()
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	testServer := httptest.NewServer(service.Handler())
	defer testServer.Close()
	body, err := json.Marshal(map[string]any{"snapshot": testSnapshot(t, 1)})
	if err != nil {
		t.Fatal(err)
	}
	response := postJSON(t, testServer.URL+"/v1/sessions", token, body)
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired launch token status = %d, want 401", response.StatusCode)
	}
}

func TestHostedOwnerOperationReauthorizesCurrentSessionRole(t *testing.T) {
	actorID, err := model.NewActorID()
	if err != nil {
		t.Fatal(err)
	}
	authorizer := &fakeHostedAuthorizer{session: auth.Session{
		ActorID: actorID, Issuer: "https://issuer.example", Subject: "owner", DisplayName: "Hosted Owner", Role: auth.RoleOwner, ExpiresAt: time.Now().Add(time.Hour),
	}}
	registry := newFakeHostedBackend(nil)
	_, created, testServer := startHTTPTestSessionWithConfig(t, ServiceConfig{AllowedOrigins: []string{"http://127.0.0.1"}, HostedAuth: authorizer, HostedRegistry: registry})
	body := []byte(`{"role":"viewer"}`)
	allowed := postJSON(t, testServer.URL+"/v1/sessions/"+created.SessionID+"/hosted-invitations", "hosted-token", body)
	_ = allowed.Body.Close()
	if allowed.StatusCode != http.StatusCreated {
		t.Fatalf("owner request status = %d", allowed.StatusCode)
	}
	authorizer.setRole(auth.RoleViewer)
	denied := postJSON(t, testServer.URL+"/v1/sessions/"+created.SessionID+"/hosted-invitations", "hosted-token", body)
	_ = denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("downgraded request status = %d", denied.StatusCode)
	}
}

func postJSON(t *testing.T, url, token string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestProtocolEnvelopeLimitMatchesHTTPBound(t *testing.T) {
	t.Parallel()
	if protocol.MaxMessageBytes > MaxHTTPBodyBytes {
		t.Fatalf("WebSocket limit %d exceeds HTTP limit %d", protocol.MaxMessageBytes, MaxHTTPBodyBytes)
	}
	if strings.TrimSpace(WebSocketSubprotocol) == "" {
		t.Fatal("WebSocket subprotocol is empty")
	}
}
