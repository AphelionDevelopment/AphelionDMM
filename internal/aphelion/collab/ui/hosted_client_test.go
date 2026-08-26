package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionClientHostedSignInRejectsMissingOrInsecureServiceOrigin(t *testing.T) {
	t.Parallel()

	client := NewSessionClient(SessionClientConfig{})
	for name, origin := range map[string]string{
		"missing":          "",
		"incomplete HTTPS": "https://",
		"cleartext remote": "http://maps.example.test",
		"credentials":      "https://user@maps.example.test",
		"path":             "https://maps.example.test/collaboration",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := client.BeginHostedSignIn(context.Background(), origin); err == nil {
				t.Fatalf("hosted service origin %q was accepted", origin)
			}
		})
	}
}

func TestSessionClientCanceledHostedSignInDoesNotRetainCredential(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/auth/desktop/begin":
			_ = json.NewEncoder(writer).Encode(map[string]string{"authorization_url": "https://issuer.example/authorize", "handoff_id": "canceled-handoff"})
		case "/v1/auth/desktop/exchange":
			writer.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(writer).Encode(map[string]string{"code": "unauthorized", "message": "desktop authentication handoff is invalid or expired"})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(testServer.Close)
	client := NewSessionClient(SessionClientConfig{})
	signIn, err := client.BeginHostedSignIn(context.Background(), testServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = client.CompleteHostedSignIn(context.Background(), signIn)
	if err == nil || errors.Is(err, ErrHostedSignInPending) {
		t.Fatalf("canceled hosted sign-in error = %v", err)
	}
	if client.HostedSignedIn() || client.hostedCredential != "" {
		t.Fatal("canceled hosted sign-in retained a credential")
	}
}

func TestSessionClientHostedDesktopSignInKeepsCredentialInMemory(t *testing.T) {
	t.Parallel()
	var challenge string
	var handoffID string
	var verifier string
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/auth/desktop/begin":
			var body struct {
				VerifierChallenge string `json:"verifier_challenge"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			challenge = body.VerifierChallenge
			handoffID = "handoff-id"
			_ = json.NewEncoder(writer).Encode(map[string]string{"authorization_url": "https://issuer.example/authorize", "handoff_id": handoffID})
		case "/v1/auth/desktop/exchange":
			var body struct {
				HandoffID string `json:"handoff_id"`
				Verifier  string `json:"verifier"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.HandoffID != handoffID {
				t.Fatalf("handoff id = %q, want %q", body.HandoffID, handoffID)
			}
			verifier = body.Verifier
			_ = json.NewEncoder(writer).Encode(map[string]any{"token": "hosted-secret", "display_name": "Mapper", "expires_at": time.Now().Add(time.Hour)})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(testServer.Close)
	client := NewSessionClient(SessionClientConfig{})
	signIn, err := client.BeginHostedSignIn(context.Background(), testServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	if signIn.AuthorizationURL != "https://issuer.example/authorize" || challenge == "" {
		t.Fatalf("sign in = %#v challenge present %t", signIn, challenge != "")
	}
	if err := client.CompleteHostedSignIn(context.Background(), signIn); err != nil {
		t.Fatal(err)
	}
	if verifier == "" || client.hostedCredential != "hosted-secret" || client.hostedDisplayName != "Mapper" {
		t.Fatalf("hosted sign in state = verifier present %t credential present %t name %q", verifier != "", client.hostedCredential != "", client.hostedDisplayName)
	}
}

func TestSessionClientCreatesAndRedeemsHostedSessions(t *testing.T) {
	t.Parallel()
	snapshot := controllerSnapshot(t)
	var redeemedSecret string
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer hosted-secret" {
			t.Fatalf("authorization header was not the in-memory hosted credential")
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/hosted/sessions":
			writer.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(writer).Encode(map[string]any{"session_id": "session-1", "document_id": snapshot.DocumentID, "revision": snapshot.Revision, "map_hash": mustSnapshotHash(t, snapshot)})
		case "/v1/sessions/session-1/hosted-invitations/redeem":
			var body map[string]string
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			redeemedSecret = body["token"]
			_ = json.NewEncoder(writer).Encode(map[string]any{"session_id": "session-1", "role": "editor"})
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(testServer.Close)
	client := NewSessionClient(SessionClientConfig{})
	client.hostedCredential = "hosted-secret"
	client.hostedCredentialExpires = time.Now().Add(time.Hour)
	client.hostedBaseURL = testServer.URL
	created, err := client.CreateHosted(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Hosted || created.Token != "hosted-secret" || created.SessionID != "session-1" {
		t.Fatalf("created hosted invitation = %#v", created)
	}
	join, err := client.RedeemHostedInvitation(context.Background(), Invitation{BaseURL: testServer.URL, Origin: testServer.URL, SessionID: "session-1", Token: "invite-secret", Hosted: true})
	if err != nil {
		t.Fatal(err)
	}
	if redeemedSecret != "invite-secret" || join.Token != "hosted-secret" || !join.Hosted {
		t.Fatalf("redeemed secret/join = %q/%#v", redeemedSecret, join)
	}
}

func TestSessionClientCreatesHostedInvitationForActiveOwner(t *testing.T) {
	t.Parallel()
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/sessions/session-1/hosted-invitations" || request.Header.Get("Authorization") != "Bearer hosted-secret" {
			t.Fatalf("hosted invitation request = %s authorization %q", request.URL.Path, request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(map[string]any{"token": "invite-secret", "role": "editor", "expires_at": time.Now().Add(time.Minute)})
	}))
	t.Cleanup(testServer.Close)
	client := NewSessionClient(SessionClientConfig{})
	client.transport = &capturingSessionTransport{}
	client.sessionID = "session-1"
	client.role = "owner"
	client.hostedSession = true
	client.hostedCredential = "hosted-secret"
	client.hostedCredentialExpires = time.Now().Add(time.Hour)
	client.hostedBaseURL = testServer.URL
	invitation, err := client.CreateInvitation(context.Background(), InvitationRoleEditor, "ignored for hosted identity")
	if err != nil {
		t.Fatal(err)
	}
	if !invitation.Hosted || invitation.Token != "invite-secret" || invitation.SessionID != "session-1" {
		t.Fatalf("hosted invitation = %#v", invitation)
	}
}

func TestSessionClientSignsOutHostedCredential(t *testing.T) {
	t.Parallel()
	loggedOut := false
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		loggedOut = request.URL.Path == "/v1/auth/logout" && request.Header.Get("Authorization") == "Bearer hosted-secret"
		writer.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(testServer.Close)
	client := NewSessionClient(SessionClientConfig{})
	client.hostedCredential = "hosted-secret"
	client.hostedCredentialExpires = time.Now().Add(time.Hour)
	client.hostedBaseURL = testServer.URL
	if err := client.SignOutHosted(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !loggedOut || client.hostedCredential != "" || client.HostedSignedIn() {
		t.Fatalf("sign out = logged out %t credential retained %t", loggedOut, client.hostedCredential != "")
	}
}
