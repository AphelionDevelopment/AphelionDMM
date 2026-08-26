package testoidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html/template"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	fixtureKeyID     = "aphelion-local-fixture"
	maximumQuerySize = 8192
	maximumTokenBody = 16 << 10
)

type Config struct {
	Issuer                string
	ClientID              string
	ClientSecret          string
	RedirectURL           string
	Subject               string
	DisplayName           string
	InteractiveIdentities bool
	Now                   func() time.Time
}

var identityFormTemplate = template.Must(template.New("pilot-identity").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>AphelionDMM pilot sign-in</title></head>
<body><main><h1>AphelionDMM pilot sign-in</h1><p>This disposable local identity is only for multiplayer testing.</p>
<form method="get" action="/authorize">
<input type="hidden" name="client_id" value="{{.ClientID}}"><input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
<input type="hidden" name="response_type" value="{{.ResponseType}}"><input type="hidden" name="scope" value="{{.Scope}}">
<input type="hidden" name="state" value="{{.State}}"><input type="hidden" name="nonce" value="{{.Nonce}}">
<input type="hidden" name="code_challenge" value="{{.CodeChallenge}}"><input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
<label>Pilot identifier <input name="fixture_subject" required maxlength="128" autocomplete="username"></label><br>
<label>Pilot display name <input name="fixture_display_name" required maxlength="128" autocomplete="name"></label><br>
<button type="submit">Continue</button></form></main></body></html>`))

type identityFormData struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	Nonce               string
	CodeChallenge       string
	CodeChallengeMethod string
}

type authorizationCode struct {
	challenge   string
	nonce       string
	subject     string
	displayName string
	expiresAt   time.Time
}

type Fixture struct {
	config Config
	key    *rsa.PrivateKey
	mutex  sync.Mutex
	codes  map[[sha256.Size]byte]authorizationCode
	mux    *http.ServeMux
}

func New(config Config) (*Fixture, error) {
	issuer, err := url.Parse(config.Issuer)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.Path != "" || issuer.RawQuery != "" || issuer.Fragment != "" {
		return nil, fmt.Errorf("fixture issuer must be an HTTPS origin")
	}
	redirect, err := url.Parse(config.RedirectURL)
	if err != nil || redirect.Scheme != "https" || redirect.Host == "" || redirect.RawQuery != "" || redirect.Fragment != "" {
		return nil, fmt.Errorf("fixture redirect URL must use HTTPS")
	}
	if config.ClientID == "" || config.ClientSecret == "" || config.Subject == "" || config.DisplayName == "" {
		return nil, fmt.Errorf("fixture identity and client configuration are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate fixture signing key: %w", err)
	}
	fixture := &Fixture{config: config, key: key, codes: make(map[[sha256.Size]byte]authorizationCode), mux: http.NewServeMux()}
	fixture.mux.HandleFunc("GET /.well-known/openid-configuration", fixture.handleDiscovery)
	fixture.mux.HandleFunc("GET /authorize", fixture.handleAuthorize)
	fixture.mux.HandleFunc("POST /token", fixture.handleToken)
	fixture.mux.HandleFunc("GET /keys", fixture.handleKeys)
	fixture.mux.HandleFunc("GET /health", func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) })
	return fixture, nil
}

func (fixture *Fixture) Handler() http.Handler {
	return fixture.mux
}

func (fixture *Fixture) SetIdentity(subject, displayName string) error {
	if subject == "" || displayName == "" {
		return fmt.Errorf("fixture identity is required")
	}
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	fixture.config.Subject = subject
	fixture.config.DisplayName = displayName
	return nil
}

func (fixture *Fixture) handleDiscovery(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"issuer": fixture.config.Issuer, "authorization_endpoint": fixture.config.Issuer + "/authorize",
		"token_endpoint": fixture.config.Issuer + "/token", "jwks_uri": fixture.config.Issuer + "/keys",
		"response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"}, "code_challenge_methods_supported": []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
	})
}

func (fixture *Fixture) handleAuthorize(writer http.ResponseWriter, request *http.Request) {
	if len(request.URL.RawQuery) > maximumQuerySize {
		writeOAuthError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	query := request.URL.Query()
	if query.Get("client_id") != fixture.config.ClientID || query.Get("redirect_uri") != fixture.config.RedirectURL || query.Get("response_type") != "code" || query.Get("code_challenge_method") != "S256" || query.Get("state") == "" || query.Get("nonce") == "" || query.Get("code_challenge") == "" || !strings.Contains(" "+query.Get("scope")+" ", " openid ") {
		writeOAuthError(writer, http.StatusBadRequest, "invalid_request")
		return
	}
	subject := fixture.config.Subject
	displayName := fixture.config.DisplayName
	if fixture.config.InteractiveIdentities {
		subject = strings.TrimSpace(query.Get("fixture_subject"))
		displayName = strings.TrimSpace(query.Get("fixture_display_name"))
		if subject == "" && displayName == "" {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			writer.Header().Set("Cache-Control", "no-store")
			if err := identityFormTemplate.Execute(writer, identityFormData{
				ClientID: query.Get("client_id"), RedirectURI: query.Get("redirect_uri"), ResponseType: query.Get("response_type"), Scope: query.Get("scope"),
				State: query.Get("state"), Nonce: query.Get("nonce"), CodeChallenge: query.Get("code_challenge"), CodeChallengeMethod: query.Get("code_challenge_method"),
			}); err != nil {
				return
			}
			return
		}
		if subject == "" || displayName == "" || len(subject) > 128 || len(displayName) > 128 {
			writeOAuthError(writer, http.StatusBadRequest, "invalid_request")
			return
		}
	}
	code, err := randomCredential()
	if err != nil {
		writeOAuthError(writer, http.StatusInternalServerError, "server_error")
		return
	}
	fixture.mutex.Lock()
	fixture.cleanupCodes(fixture.config.Now())
	fixture.codes[sha256.Sum256([]byte(code))] = authorizationCode{
		challenge: query.Get("code_challenge"), nonce: query.Get("nonce"), subject: subject,
		displayName: displayName, expiresAt: fixture.config.Now().Add(5 * time.Minute),
	}
	fixture.mutex.Unlock()
	redirect, _ := url.Parse(fixture.config.RedirectURL)
	values := redirect.Query()
	values.Set("state", query.Get("state"))
	values.Set("code", code)
	redirect.RawQuery = values.Encode()
	http.Redirect(writer, request, redirect.String(), http.StatusFound)
}

func (fixture *Fixture) handleToken(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	request.Body = http.MaxBytesReader(writer, request.Body, maximumTokenBody)
	clientID, clientSecret, ok := request.BasicAuth()
	if !ok || subtle.ConstantTimeCompare([]byte(clientID), []byte(fixture.config.ClientID)) != 1 || subtle.ConstantTimeCompare([]byte(clientSecret), []byte(fixture.config.ClientSecret)) != 1 || request.ParseForm() != nil {
		writeOAuthError(writer, http.StatusUnauthorized, "invalid_client")
		return
	}
	code := request.Form.Get("code")
	verifier := request.Form.Get("code_verifier")
	codeHash := sha256.Sum256([]byte(code))
	fixture.mutex.Lock()
	stored, found := fixture.codes[codeHash]
	delete(fixture.codes, codeHash)
	fixture.mutex.Unlock()
	verifierDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(verifierDigest[:])
	if !found || code == "" || verifier == "" || request.Form.Get("grant_type") != "authorization_code" || request.Form.Get("redirect_uri") != fixture.config.RedirectURL || !fixture.config.Now().Before(stored.expiresAt) || subtle.ConstantTimeCompare([]byte(challenge), []byte(stored.challenge)) != 1 {
		writeOAuthError(writer, http.StatusBadRequest, "invalid_grant")
		return
	}
	accessToken, err := randomCredential()
	if err != nil {
		writeOAuthError(writer, http.StatusInternalServerError, "server_error")
		return
	}
	idToken, err := fixture.signIDToken(stored.nonce, stored.subject, stored.displayName)
	if err != nil {
		writeOAuthError(writer, http.StatusInternalServerError, "server_error")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"access_token": accessToken, "token_type": "Bearer", "expires_in": 3600, "id_token": idToken})
}

func (fixture *Fixture) handleKeys(writer http.ResponseWriter, _ *http.Request) {
	exponent := make([]byte, 4)
	binary.BigEndian.PutUint32(exponent, uint32(fixture.key.E))
	exponent = bytesWithoutLeadingZero(exponent)
	writeJSON(writer, http.StatusOK, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": fixtureKeyID,
		"n": base64.RawURLEncoding.EncodeToString(fixture.key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(exponent),
	}}})
}

func (fixture *Fixture) signIDToken(nonce, subject, displayName string) (string, error) {
	now := fixture.config.Now()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": fixtureKeyID, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": fixture.config.Issuer, "sub": subject, "aud": fixture.config.ClientID,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "nonce": nonce, "name": displayName,
	})
	signingInput := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, fixture.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (fixture *Fixture) cleanupCodes(now time.Time) {
	for key, code := range fixture.codes {
		if !now.Before(code.expiresAt) {
			delete(fixture.codes, key)
		}
	}
}

func GenerateLocalTLSCertificate(hosts []string, now time.Time) (tls.Certificate, []byte, error) {
	if len(hosts) == 0 {
		return tls.Certificate{}, nil, fmt.Errorf("at least one TLS host is required")
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	caTemplate := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "AphelionDMM local OIDC fixture CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	leafSerial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	leafTemplate := &x509.Certificate{SerialNumber: leafSerial, Subject: pkix.Name{CommonName: hosts[0]}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(12 * time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment}
	for _, host := range hosts {
		if ip := net.ParseIP(host); ip != nil {
			leafTemplate.IPAddresses = append(leafTemplate.IPAddresses, ip)
		} else {
			leafTemplate.DNSNames = append(leafTemplate.DNSNames, host)
		}
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)})
	certificate, err := tls.X509KeyPair(certificatePEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, nil, err
	}
	return certificate, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), nil
}

func randomCredential() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func bytesWithoutLeadingZero(value []byte) []byte {
	for len(value) > 1 && value[0] == 0 {
		value = value[1:]
	}
	return value
}

func writeOAuthError(writer http.ResponseWriter, status int, code string) {
	writeJSON(writer, status, map[string]string{"error": code})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
