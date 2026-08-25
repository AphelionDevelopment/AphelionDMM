package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"
)

func TestNewOIDCFlowRejectsIncompleteConfiguration(t *testing.T) {
	t.Parallel()

	for _, config := range []OIDCConfig{
		{},
		{Issuer: "https://issuer.example"},
		{Issuer: "https://issuer.example", ClientID: "client"},
	} {
		if _, err := NewOIDCFlow(context.Background(), config); err == nil {
			t.Fatalf("configuration %#v was accepted", config)
		}
	}
}

func TestOIDCFlowDiscoversUsesPKCEAndVerifiesIdentity(t *testing.T) {
	t.Parallel()

	issuer := newSignedIssuer(t)
	flow, err := NewOIDCFlow(context.Background(), OIDCConfig{
		Issuer: issuer.server.URL, ClientID: issuer.clientID, RedirectURL: "http://127.0.0.1/callback", AllowInsecureLoopback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	verifier := oauth2.GenerateVerifier()
	authorizationURL := flow.AuthorizationURL("state-value", "nonce-value", verifier)
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("state") != "state-value" || query.Get("nonce") != "nonce-value" || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(verifier) {
		t.Fatalf("authorization query = %v", query)
	}
	issuer.expect(verifier, "nonce-value")
	identity, err := flow.Exchange(context.Background(), "valid-code", verifier, "nonce-value")
	if err != nil {
		t.Fatal(err)
	}
	if identity.Issuer != issuer.server.URL || identity.Subject != "subject-1" || identity.DisplayName != "Mapper One" || !identity.ExpiresAt.After(time.Now()) {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestOIDCFlowRejectsNonceAudienceExpiryIssuerAndSignature(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*signedIssuer)
		nonce  string
	}{
		{name: "nonce", nonce: "different-nonce"},
		{name: "audience", mutate: func(issuer *signedIssuer) { issuer.audience = "different-client" }, nonce: "nonce-value"},
		{name: "expiry", mutate: func(issuer *signedIssuer) { issuer.expiry = time.Now().Add(-time.Minute) }, nonce: "nonce-value"},
		{name: "issuer", mutate: func(issuer *signedIssuer) { issuer.claimedIssuer = "https://different.example" }, nonce: "nonce-value"},
		{name: "signature", mutate: func(issuer *signedIssuer) { issuer.signWithUnpublishedKey(t) }, nonce: "nonce-value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issuer := newSignedIssuer(t)
			flow, err := NewOIDCFlow(context.Background(), OIDCConfig{Issuer: issuer.server.URL, ClientID: issuer.clientID, RedirectURL: "http://127.0.0.1/callback", AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			if test.mutate != nil {
				test.mutate(issuer)
			}
			verifier := oauth2.GenerateVerifier()
			issuer.expect(verifier, "nonce-value")
			if _, err := flow.Exchange(context.Background(), "valid-code", verifier, test.nonce); err == nil {
				t.Fatal("invalid token was accepted")
			}
		})
	}
}

func TestOIDCFlowRefreshesKeysAfterRotation(t *testing.T) {
	t.Parallel()

	issuer := newSignedIssuer(t)
	flow, err := NewOIDCFlow(context.Background(), OIDCConfig{Issuer: issuer.server.URL, ClientID: issuer.clientID, RedirectURL: "http://127.0.0.1/callback", AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	firstVerifier := oauth2.GenerateVerifier()
	issuer.expect(firstVerifier, "nonce-one")
	if _, err := flow.Exchange(context.Background(), "valid-code", firstVerifier, "nonce-one"); err != nil {
		t.Fatal(err)
	}
	issuer.rotate(t)
	secondVerifier := oauth2.GenerateVerifier()
	issuer.expect(secondVerifier, "nonce-two")
	if _, err := flow.Exchange(context.Background(), "valid-code", secondVerifier, "nonce-two"); err != nil {
		t.Fatalf("exchange after key rotation: %v", err)
	}
}

type signedIssuer struct {
	mutex            sync.Mutex
	server           *httptest.Server
	clientID         string
	key              *rsa.PrivateKey
	publishedKey     *rsa.PrivateKey
	kid              string
	expectedVerifier string
	nonce            string
	audience         string
	claimedIssuer    string
	expiry           time.Time
}

func newSignedIssuer(t *testing.T) *signedIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer := &signedIssuer{clientID: "aphelion-test-client", key: key, publishedKey: key, kid: "key-1", audience: "aphelion-test-client", expiry: time.Now().Add(time.Hour)}
	issuer.server = httptest.NewServer(http.HandlerFunc(issuer.serveHTTP))
	issuer.claimedIssuer = issuer.server.URL
	t.Cleanup(issuer.server.Close)
	return issuer
}

func (issuer *signedIssuer) expect(verifier, nonce string) {
	issuer.mutex.Lock()
	issuer.expectedVerifier = verifier
	issuer.nonce = nonce
	issuer.mutex.Unlock()
}

func (issuer *signedIssuer) rotate(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer.mutex.Lock()
	issuer.key = key
	issuer.publishedKey = key
	issuer.kid = "key-2"
	issuer.mutex.Unlock()
}

func (issuer *signedIssuer) signWithUnpublishedKey(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer.mutex.Lock()
	issuer.key = key
	issuer.kid = "unpublished"
	issuer.mutex.Unlock()
}

func (issuer *signedIssuer) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	issuer.mutex.Lock()
	defer issuer.mutex.Unlock()
	switch request.URL.Path {
	case "/.well-known/openid-configuration":
		writeIssuerJSON(writer, map[string]any{
			"issuer": issuer.server.URL, "authorization_endpoint": issuer.server.URL + "/authorize", "token_endpoint": issuer.server.URL + "/token", "jwks_uri": issuer.server.URL + "/keys",
			"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}, "token_endpoint_auth_methods_supported": []string{"none"},
		})
	case "/keys":
		writeIssuerJSON(writer, map[string]any{"keys": []jose.JSONWebKey{{Key: &issuer.publishedKey.PublicKey, KeyID: issuer.kid, Algorithm: string(jose.RS256), Use: "sig"}}})
	case "/token":
		if err := request.ParseForm(); err != nil || request.Form.Get("code") != "valid-code" || request.Form.Get("client_id") != issuer.clientID || request.Form.Get("code_verifier") != issuer.expectedVerifier {
			http.Error(writer, "invalid token request", http.StatusBadRequest)
			return
		}
		raw, err := issuer.signedToken()
		if err != nil {
			http.Error(writer, "sign token", http.StatusInternalServerError)
			return
		}
		writeIssuerJSON(writer, map[string]any{"access_token": "access-token", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
	default:
		http.NotFound(writer, request)
	}
}

func (issuer *signedIssuer) signedToken() (string, error) {
	claims, err := json.Marshal(map[string]any{
		"iss": issuer.claimedIssuer, "sub": "subject-1", "aud": issuer.audience, "exp": issuer.expiry.Unix(), "iat": time.Now().Add(-time.Minute).Unix(), "nonce": issuer.nonce, "name": "Mapper One",
	})
	if err != nil {
		return "", err
	}
	options := (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", issuer.kid)
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: issuer.key}, options)
	if err != nil {
		return "", err
	}
	signed, err := signer.Sign(claims)
	if err != nil {
		return "", err
	}
	return signed.CompactSerialize()
}

func writeIssuerJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
