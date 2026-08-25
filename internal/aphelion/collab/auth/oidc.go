package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type OIDCConfig struct {
	Issuer                string
	ClientID              string
	ClientSecret          string
	RedirectURL           string
	HTTPClient            *http.Client
	AllowInsecureLoopback bool
}

type OIDCFlow struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	client   *http.Client
}

func NewOIDCFlow(ctx context.Context, config OIDCConfig) (*OIDCFlow, error) {
	if config.Issuer == "" || config.ClientID == "" || config.RedirectURL == "" {
		return nil, fmt.Errorf("OIDC issuer, client ID, and redirect URL are required")
	}
	if err := validateOIDCEndpoint(config.Issuer, config.AllowInsecureLoopback); err != nil {
		return nil, fmt.Errorf("validate OIDC issuer: %w", err)
	}
	if err := validateOIDCEndpoint(config.RedirectURL, config.AllowInsecureLoopback); err != nil {
		return nil, fmt.Errorf("validate OIDC redirect URL: %w", err)
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	discoveryContext := oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(discoveryContext, strings.TrimRight(config.Issuer, "/"))
	if err != nil {
		return nil, fmt.Errorf("discover OIDC provider: %w", err)
	}
	return &OIDCFlow{
		oauth:    oauth2.Config{ClientID: config.ClientID, ClientSecret: config.ClientSecret, Endpoint: provider.Endpoint(), RedirectURL: config.RedirectURL, Scopes: []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}},
		verifier: provider.Verifier(&oidc.Config{ClientID: config.ClientID}),
		client:   client,
	}, nil
}

func (flow *OIDCFlow) AuthorizationURL(state, nonce, verifier string) string {
	return flow.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier))
}

func (flow *OIDCFlow) Exchange(ctx context.Context, code, verifier, nonce string) (Identity, error) {
	ctx = oidc.ClientContext(ctx, flow.client)
	token, err := flow.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("exchange OIDC authorization code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return Identity{}, fmt.Errorf("OIDC token response has no ID token")
	}
	idToken, err := flow.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, fmt.Errorf("verify OIDC ID token: %w", err)
	}
	var claims struct {
		Nonce             string `json:"nonce"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
		AccessTokenHash   string `json:"at_hash"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("decode OIDC ID token claims: %w", err)
	}
	if nonce == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(nonce)) != 1 {
		return Identity{}, fmt.Errorf("OIDC nonce does not match authorization request")
	}
	if claims.AccessTokenHash != "" {
		if err := idToken.VerifyAccessToken(token.AccessToken); err != nil {
			return Identity{}, fmt.Errorf("verify OIDC access token binding: %w", err)
		}
	}
	displayName := claims.Name
	if displayName == "" {
		displayName = claims.PreferredUsername
	}
	if displayName == "" {
		displayName = claims.Email
	}
	if idToken.Subject == "" || len(idToken.Subject) > 128 || displayName == "" || len(displayName) > 128 {
		return Identity{}, fmt.Errorf("OIDC identity claims are incomplete or oversized")
	}
	return Identity{Issuer: idToken.Issuer, Subject: idToken.Subject, DisplayName: displayName, ExpiresAt: idToken.Expiry}, nil
}

func validateOIDCEndpoint(value string, allowInsecureLoopback bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("URL is invalid or contains forbidden components")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	host := parsed.Hostname()
	if allowInsecureLoopback && parsed.Scheme == "http" && (strings.EqualFold(host, "localhost") || net.ParseIP(host).IsLoopback()) {
		return nil
	}
	return fmt.Errorf("URL must use HTTPS")
}
