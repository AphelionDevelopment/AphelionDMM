package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

const (
	MaxHTTPBodyBytes     = 2 << 20
	WebSocketSubprotocol = "apheliondmm.collaboration.v1"
)

type ServiceConfig struct {
	AllowedOrigins   []string
	Build            string
	Revision         string
	OnWebSocketError func(error)
	LaunchTokenTTL   time.Duration
	JoinTokenTTL     time.Duration
	PresenceTimeout  time.Duration
	Now              func() time.Time
}

type CreateSessionResponse struct {
	SessionID           string           `json:"session_id"`
	DocumentID          model.DocumentID `json:"document_id"`
	Revision            model.Revision   `json:"revision"`
	MapHash             string           `json:"map_hash"`
	OwnerToken          string           `json:"owner_token"`
	OwnerTokenExpiresAt time.Time        `json:"owner_token_expires_at"`
}

type tokenRecord struct {
	launch             bool
	sessionID          string
	principal          Principal
	expiresAt          time.Time
	expectedDocumentID model.DocumentID
	expectedMapHash    string
}

type sessionRecord struct {
	owner *DocumentOwner
}

type Service struct {
	config   ServiceConfig
	context  context.Context
	cancel   context.CancelFunc
	hub      *Hub
	store    *MemoryStore
	mutex    sync.RWMutex
	tokens   map[string]tokenRecord
	sessions map[string]sessionRecord
	server   *http.ServeMux
}

func NewService(config ServiceConfig) *Service {
	if config.LaunchTokenTTL <= 0 {
		config.LaunchTokenTTL = 2 * time.Minute
	}
	if config.JoinTokenTTL <= 0 {
		config.JoinTokenTTL = 15 * time.Minute
	}
	if config.PresenceTimeout <= 0 {
		config.PresenceTimeout = time.Minute
	}
	if config.Now == nil {
		config.Now = func() time.Time { return time.Now().UTC() }
	}
	serviceContext, cancel := context.WithCancel(context.Background())
	service := &Service{
		config:   config,
		context:  serviceContext,
		cancel:   cancel,
		hub:      NewHub(config.PresenceTimeout),
		store:    NewMemoryStore(),
		tokens:   make(map[string]tokenRecord),
		sessions: make(map[string]sessionRecord),
		server:   http.NewServeMux(),
	}
	service.routes()
	go service.expirePresence(config.PresenceTimeout / 2)
	return service
}

func (service *Service) expirePresence(interval time.Duration) {
	if interval <= 0 {
		interval = time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-service.context.Done():
			return
		case now := <-ticker.C:
			service.hub.ExpirePresence(now)
		}
	}
}

func (service *Service) Handler() http.Handler {
	return service.server
}

func (service *Service) Shutdown(ctx context.Context) error {
	service.cancel()
	service.mutex.RLock()
	owners := make([]*DocumentOwner, 0, len(service.sessions))
	for _, session := range service.sessions {
		owners = append(owners, session.owner)
	}
	service.mutex.RUnlock()
	for _, owner := range owners {
		if err := owner.Close(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) NewLaunchToken() (string, error) {
	return service.newLaunchToken(model.Snapshot{})
}

func (service *Service) NewLaunchTokenForSnapshot(snapshot model.Snapshot) (string, error) {
	if _, err := snapshot.Hash(); err != nil {
		return "", err
	}
	return service.newLaunchToken(snapshot)
}

func (service *Service) newLaunchToken(snapshot model.Snapshot) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	service.mutex.Lock()
	record := tokenRecord{launch: true, expiresAt: service.config.Now().Add(service.config.LaunchTokenTTL)}
	if snapshot.DocumentID != "" {
		record.expectedDocumentID = snapshot.DocumentID
		record.expectedMapHash, err = snapshot.Hash()
		if err != nil {
			return "", err
		}
	}
	service.tokens[token] = record
	service.mutex.Unlock()
	return token, nil
}

func ValidateListenAddress(address string, allowNetwork bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse listen address: %w", err)
	}
	if allowNetwork || strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen address %q is not loopback", address)
	}
	return nil
}

func (service *Service) routes() {
	service.server.HandleFunc("GET /v1/health/live", service.handleHealth)
	service.server.HandleFunc("GET /v1/health/ready", service.handleHealth)
	service.server.HandleFunc("GET /v1/version", service.handleVersion)
	service.server.HandleFunc("POST /v1/sessions", service.handleCreateSession)
	service.server.HandleFunc("GET /v1/sessions/{session_id}", service.handleGetSession)
	service.server.HandleFunc("GET /v1/sessions/{session_id}/snapshot", service.handleGetSnapshot)
	service.server.HandleFunc("POST /v1/sessions/{session_id}/join-tokens", service.handleCreateJoinToken)
	service.server.HandleFunc("POST /v1/sessions/{session_id}/exports", service.handleExport)
	service.server.HandleFunc("GET /v1/collaboration", service.handleWebSocket)
}

func (service *Service) handleHealth(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (service *Service) handleVersion(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{
		"build": service.config.Build, "revision": service.config.Revision,
		"protocol_versions": []uint16{model.ProtocolVersion}, "schema_versions": []uint16{model.SchemaVersion},
	})
}

func (service *Service) handleCreateSession(writer http.ResponseWriter, request *http.Request) {
	token := bearerToken(request)
	launch, exists := service.launchToken(token)
	if !exists {
		writeError(writer, http.StatusUnauthorized, "unauthorized", "invalid or redeemed launch token")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, MaxHTTPBodyBytes)
	var body struct {
		Snapshot model.Snapshot `json:"snapshot"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		status := http.StatusBadRequest
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(writer, status, "invalid_request", "invalid session request")
		return
	}
	mapHash, err := body.Snapshot.Hash()
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_snapshot", "snapshot hash failed")
		return
	}
	if launch.expectedDocumentID != "" && (body.Snapshot.DocumentID != launch.expectedDocumentID || mapHash != launch.expectedMapHash) {
		writeError(writer, http.StatusForbidden, "snapshot_mismatch", "snapshot does not match the embedded launch document")
		return
	}
	if !service.redeemLaunchToken(token) {
		writeError(writer, http.StatusUnauthorized, "unauthorized", "invalid or redeemed launch token")
		return
	}
	owner, err := StartDocument(service.context, body.Snapshot, service.store)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_snapshot", "snapshot is not valid")
		return
	}
	actorID, err := model.NewActorID()
	if err != nil {
		_ = owner.Close(request.Context())
		writeError(writer, http.StatusInternalServerError, "internal", "identity generation failed")
		return
	}
	principal, err := NewPrincipal("embedded-owner", actorID, "Owner", RoleOwner)
	if err != nil {
		_ = owner.Close(request.Context())
		writeError(writer, http.StatusInternalServerError, "internal", "identity initialization failed")
		return
	}
	sessionID, err := randomToken()
	if err != nil {
		_ = owner.Close(request.Context())
		writeError(writer, http.StatusInternalServerError, "internal", "session generation failed")
		return
	}
	ownerToken, ownerTokenExpiresAt, err := service.issueToken(sessionID, principal)
	if err != nil {
		_ = owner.Close(request.Context())
		writeError(writer, http.StatusInternalServerError, "internal", "token generation failed")
		return
	}
	if err := service.hub.Create(sessionID, owner, principal); err != nil {
		_ = owner.Close(request.Context())
		writeError(writer, http.StatusInternalServerError, "internal", "session registration failed")
		return
	}
	service.mutex.Lock()
	service.sessions[sessionID] = sessionRecord{owner: owner}
	service.mutex.Unlock()
	writeJSON(writer, http.StatusCreated, CreateSessionResponse{
		SessionID: sessionID, DocumentID: body.Snapshot.DocumentID, Revision: body.Snapshot.Revision, MapHash: mapHash, OwnerToken: ownerToken, OwnerTokenExpiresAt: ownerTokenExpiresAt,
	})
}

func (service *Service) launchToken(token string) (tokenRecord, bool) {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	record, exists := service.tokens[token]
	return record, exists && record.launch && service.config.Now().Before(record.expiresAt)
}

func (service *Service) handleGetSession(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("session_id")
	_, record, ok := service.authorize(request, sessionID)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "unauthorized", "session token is invalid")
		return
	}
	snapshot, err := record.owner.Snapshot(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "unavailable", "session is unavailable")
		return
	}
	hash, _ := snapshot.Hash()
	writeJSON(writer, http.StatusOK, map[string]any{"session_id": sessionID, "document_id": snapshot.DocumentID, "protocol_version": snapshot.ProtocolVersion, "schema_version": snapshot.SchemaVersion, "revision": snapshot.Revision, "map_hash": hash})
}

func (service *Service) handleGetSnapshot(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("session_id")
	_, record, ok := service.authorize(request, sessionID)
	if !ok {
		writeError(writer, http.StatusUnauthorized, "unauthorized", "session token is invalid")
		return
	}
	snapshot, err := record.owner.Snapshot(request.Context())
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "unavailable", "session is unavailable")
		return
	}
	hash, _ := snapshot.Hash()
	writer.Header().Set("ETag", `"`+hash+`"`)
	writeJSON(writer, http.StatusOK, snapshot)
}

func (service *Service) handleCreateJoinToken(writer http.ResponseWriter, request *http.Request) {
	sessionID := request.PathValue("session_id")
	principal, _, ok := service.authorize(request, sessionID)
	if !ok || !principal.CanAdminister() {
		writeError(writer, http.StatusForbidden, "forbidden", "owner role is required")
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, MaxHTTPBodyBytes)
	var body struct {
		Role        Role   `json:"role"`
		DisplayName string `json:"display_name"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", "invalid join-token request")
		return
	}
	actorID, err := model.NewActorID()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "internal", "identity generation failed")
		return
	}
	joined, err := NewPrincipal("join-"+string(actorID), actorID, body.DisplayName, body.Role)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_request", "invalid join identity")
		return
	}
	if err := service.hub.Join(sessionID, joined); err != nil {
		writeError(writer, http.StatusBadRequest, "join_failed", "could not authorize join")
		return
	}
	token, expiresAt, err := service.issueToken(sessionID, joined)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "internal", "token generation failed")
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"token": token, "actor_id": actorID, "role": body.Role, "expires_at": expiresAt})
}

func (service *Service) handleExport(writer http.ResponseWriter, _ *http.Request) {
	writeError(writer, http.StatusNotImplemented, "not_implemented", "export checkpoints are implemented in the durability phase")
}

func (service *Service) authorize(request *http.Request, sessionID string) (Principal, sessionRecord, bool) {
	service.mutex.RLock()
	defer service.mutex.RUnlock()
	token, tokenExists := service.tokens[bearerToken(request)]
	session, sessionExists := service.sessions[sessionID]
	return token.principal, session, tokenExists && !token.launch && token.sessionID == sessionID && sessionExists && service.config.Now().Before(token.expiresAt)
}

func (service *Service) redeemLaunchToken(token string) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	record, exists := service.tokens[token]
	if !exists || !record.launch || !service.config.Now().Before(record.expiresAt) {
		return false
	}
	delete(service.tokens, token)
	return true
}

func (service *Service) issueToken(sessionID string, principal Principal) (string, time.Time, error) {
	token, err := randomToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expiresAt := service.config.Now().Add(service.config.JoinTokenTTL)
	service.mutex.Lock()
	service.tokens[token] = tokenRecord{sessionID: sessionID, principal: principal, expiresAt: expiresAt}
	service.mutex.Unlock()
	return token, expiresAt, nil
}

func randomToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func bearerToken(request *http.Request) string {
	value := request.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(value, "Bearer ")
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, map[string]string{"code": code, "message": message})
}
