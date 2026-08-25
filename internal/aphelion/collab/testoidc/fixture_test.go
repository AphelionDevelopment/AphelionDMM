package testoidc

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFixtureCompletesOneUseConfidentialPKCEFlow(t *testing.T) {
	fixture, err := New(Config{
		Issuer: "https://issuer.example", ClientID: "client-id", ClientSecret: "client-secret",
		RedirectURL: "https://maps.example/v1/auth/complete", Subject: "subject-1", DisplayName: "Mapper",
		Now: func() time.Time { return time.Unix(10_000, 0).UTC() },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.SetIdentity("subject-2", "Second Mapper"); err != nil {
		t.Fatal(err)
	}
	verifier := strings.Repeat("v", 64)
	challengeDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	authorizeURL := "/authorize?" + url.Values{
		"client_id": {"client-id"}, "redirect_uri": {"https://maps.example/v1/auth/complete"},
		"response_type": {"code"}, "scope": {"openid profile"}, "state": {"state-value"},
		"nonce": {"nonce-value"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}.Encode()
	authorize := httptest.NewRecorder()
	fixture.Handler().ServeHTTP(authorize, httptest.NewRequest(http.MethodGet, authorizeURL, nil))
	if authorize.Code != http.StatusFound {
		t.Fatalf("authorize status = %d body=%s", authorize.Code, authorize.Body.String())
	}
	redirect, err := url.Parse(authorize.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	code := redirect.Query().Get("code")
	if code == "" || redirect.Query().Get("state") != "state-value" {
		t.Fatalf("authorization redirect = %s", redirect)
	}
	tokenForm := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {"https://maps.example/v1/auth/complete"}, "code_verifier": {verifier}}
	tokenRequest := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(tokenForm.Encode()))
	tokenRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRequest.SetBasicAuth("client-id", "client-secret")
	token := httptest.NewRecorder()
	fixture.Handler().ServeHTTP(token, tokenRequest)
	if token.Code != http.StatusOK {
		t.Fatalf("token status = %d body=%s", token.Code, token.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(token.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["access_token"] == "" || strings.Count(payload["id_token"].(string), ".") != 2 {
		t.Fatalf("token payload = %#v", payload)
	}
	parts := strings.Split(payload["id_token"].(string), ".")
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != "subject-2" || claims["name"] != "Second Mapper" {
		t.Fatalf("identity claims = %#v", claims)
	}
	reuseRequest := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(tokenForm.Encode()))
	reuseRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reuseRequest.SetBasicAuth("client-id", "client-secret")
	reuse := httptest.NewRecorder()
	fixture.Handler().ServeHTTP(reuse, reuseRequest)
	if reuse.Code != http.StatusBadRequest {
		t.Fatalf("reused code status = %d", reuse.Code)
	}
}

func TestLocalTLSCertificateTrustsDockerHostName(t *testing.T) {
	certificate, roots, err := GenerateLocalTLSCertificate([]string{"host.docker.internal", "localhost"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(certificate.Certificate) == 0 || len(roots) == 0 {
		t.Fatal("generated certificate or root PEM is empty")
	}
}
