package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"

	"sdmm/internal/aphelion/collab/model"
)

var (
	ErrInvalidState           = errors.New("OIDC authorization state is invalid or expired")
	ErrInvalidSession         = errors.New("hosted authentication session is invalid or expired")
	ErrDisabledIdentity       = errors.New("hosted identity is disabled")
	ErrAuthenticationCapacity = errors.New("hosted authentication capacity is exhausted")
)

const (
	defaultMaxPending  = 4096
	defaultMaxSessions = 16384
)

type Role string

const (
	RoleViewer Role = "viewer"
	RoleEditor Role = "editor"
	RoleOwner  Role = "owner"
)

type Identity struct {
	Issuer      string
	Subject     string
	DisplayName string
	ExpiresAt   time.Time
}

type AuthorizationFlow interface {
	AuthorizationURL(state, nonce, verifier string) string
	Exchange(context.Context, string, string, string) (Identity, error)
}

type Directory interface {
	Resolve(context.Context, Identity) (Role, bool, error)
}

type SessionDirectory interface {
	ResolveSession(context.Context, Identity, string) (Role, bool, error)
}

type ManagerConfig struct {
	PendingTTL       time.Duration
	MaxPending       int
	MaxSessions      int
	Now              func() time.Time
	SessionDirectory SessionDirectory
}

type BeginResult struct {
	AuthorizationURL string
	State            string
}

type Session struct {
	Token       string
	ActorID     model.ActorID
	Issuer      string
	Subject     string
	DisplayName string
	Role        Role
	ExpiresAt   time.Time
}

type pendingAuthorization struct {
	nonce     string
	verifier  string
	expiresAt time.Time
}

type Manager struct {
	mutex            sync.Mutex
	flow             AuthorizationFlow
	directory        Directory
	sessionDirectory SessionDirectory
	pendingTTL       time.Duration
	maxPending       int
	maxSessions      int
	now              func() time.Time
	pending          map[[sha256.Size]byte]pendingAuthorization
	sessions         map[[sha256.Size]byte]Session
}

func NewManager(flow AuthorizationFlow, directory Directory, config ManagerConfig) *Manager {
	if config.PendingTTL <= 0 {
		config.PendingTTL = 5 * time.Minute
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.MaxPending <= 0 {
		config.MaxPending = defaultMaxPending
	}
	if config.MaxSessions <= 0 {
		config.MaxSessions = defaultMaxSessions
	}
	return &Manager{
		flow: flow, directory: directory, sessionDirectory: config.SessionDirectory, pendingTTL: config.PendingTTL, maxPending: config.MaxPending, maxSessions: config.MaxSessions, now: config.Now,
		pending: make(map[[sha256.Size]byte]pendingAuthorization), sessions: make(map[[sha256.Size]byte]Session),
	}
}

func (manager *Manager) Begin(context.Context) (BeginResult, error) {
	if manager.flow == nil || manager.directory == nil {
		return BeginResult{}, fmt.Errorf("hosted authentication is unavailable")
	}
	state, err := randomCredential()
	if err != nil {
		return BeginResult{}, err
	}
	nonce, err := randomCredential()
	if err != nil {
		return BeginResult{}, err
	}
	verifier := oauth2.GenerateVerifier()
	manager.mutex.Lock()
	manager.cleanupPending(manager.now())
	if len(manager.pending) >= manager.maxPending {
		manager.mutex.Unlock()
		return BeginResult{}, ErrAuthenticationCapacity
	}
	manager.pending[sha256.Sum256([]byte(state))] = pendingAuthorization{nonce: nonce, verifier: verifier, expiresAt: manager.now().Add(manager.pendingTTL)}
	manager.mutex.Unlock()
	return BeginResult{AuthorizationURL: manager.flow.AuthorizationURL(state, nonce, verifier), State: state}, nil
}

func (manager *Manager) Complete(ctx context.Context, state, code string) (Session, error) {
	stateHash := sha256.Sum256([]byte(state))
	manager.mutex.Lock()
	pending, found := manager.pending[stateHash]
	delete(manager.pending, stateHash)
	now := manager.now()
	manager.mutex.Unlock()
	if !found || state == "" || code == "" || !now.Before(pending.expiresAt) {
		return Session{}, ErrInvalidState
	}
	identity, err := manager.flow.Exchange(ctx, code, pending.verifier, pending.nonce)
	if err != nil {
		return Session{}, err
	}
	role, disabled, err := manager.directory.Resolve(ctx, identity)
	if err != nil {
		return Session{}, err
	}
	if disabled {
		return Session{}, ErrDisabledIdentity
	}
	if !validRole(role) {
		return Session{}, fmt.Errorf("hosted identity role %q is invalid", role)
	}
	token, err := randomCredential()
	if err != nil {
		return Session{}, err
	}
	actorID, err := actorIDForIdentity(identity)
	if err != nil {
		return Session{}, err
	}
	session := Session{ActorID: actorID, Issuer: identity.Issuer, Subject: identity.Subject, DisplayName: identity.DisplayName, Role: role, ExpiresAt: identity.ExpiresAt}
	manager.mutex.Lock()
	manager.cleanupSessions(manager.now())
	if len(manager.sessions) >= manager.maxSessions {
		manager.mutex.Unlock()
		return Session{}, ErrAuthenticationCapacity
	}
	manager.sessions[sha256.Sum256([]byte(token))] = session
	manager.mutex.Unlock()
	session.Token = token
	return session, nil
}

func actorIDForIdentity(identity Identity) (model.ActorID, error) {
	if identity.Issuer == "" || identity.Subject == "" {
		return "", fmt.Errorf("hosted identity issuer and subject are required")
	}
	digest := sha256.Sum256([]byte("apheliondmm-actor-v1\x00" + identity.Issuer + "\x00" + identity.Subject))
	var value uuid.UUID
	copy(value[:], digest[:len(value)])
	value[6] = value[6]&0x0f | 0x70
	value[8] = value[8]&0x3f | 0x80
	actorID := model.ActorID(value.String())
	if err := actorID.Validate(); err != nil {
		return "", err
	}
	return actorID, nil
}

func (manager *Manager) Authorize(ctx context.Context, token string) (Session, error) {
	tokenHash := sha256.Sum256([]byte(token))
	manager.mutex.Lock()
	session, found := manager.sessions[tokenHash]
	now := manager.now()
	if !found || token == "" || !now.Before(session.ExpiresAt) {
		delete(manager.sessions, tokenHash)
		manager.mutex.Unlock()
		return Session{}, ErrInvalidSession
	}
	manager.mutex.Unlock()
	identity := Identity{Issuer: session.Issuer, Subject: session.Subject, DisplayName: session.DisplayName, ExpiresAt: session.ExpiresAt}
	role, disabled, err := manager.directory.Resolve(ctx, identity)
	if err != nil {
		return Session{}, err
	}
	if disabled {
		manager.Logout(token)
		return Session{}, ErrDisabledIdentity
	}
	if !validRole(role) {
		return Session{}, fmt.Errorf("hosted identity role %q is invalid", role)
	}
	session.Role = role
	manager.mutex.Lock()
	if _, current := manager.sessions[tokenHash]; !current {
		manager.mutex.Unlock()
		return Session{}, ErrInvalidSession
	}
	manager.sessions[tokenHash] = session
	manager.mutex.Unlock()
	session.Token = token
	return session, nil
}

func (manager *Manager) AuthorizeSession(ctx context.Context, token, sessionID string) (Session, error) {
	if sessionID == "" || len(sessionID) > 128 {
		return Session{}, fmt.Errorf("collaboration session id length is %d, want 1..128", len(sessionID))
	}
	if manager.sessionDirectory == nil {
		return Session{}, fmt.Errorf("hosted collaboration session authorization is unavailable")
	}
	session, err := manager.Authorize(ctx, token)
	if err != nil {
		return Session{}, err
	}
	identity := Identity{Issuer: session.Issuer, Subject: session.Subject, DisplayName: session.DisplayName, ExpiresAt: session.ExpiresAt}
	role, disabled, err := manager.sessionDirectory.ResolveSession(ctx, identity, sessionID)
	if err != nil {
		return Session{}, err
	}
	if disabled {
		return Session{}, ErrDisabledIdentity
	}
	if !validRole(role) {
		return Session{}, fmt.Errorf("hosted collaboration session role %q is invalid", role)
	}
	session.Role = role
	return session, nil
}

func (manager *Manager) Logout(token string) {
	manager.mutex.Lock()
	delete(manager.sessions, sha256.Sum256([]byte(token)))
	manager.mutex.Unlock()
}

func (manager *Manager) cleanupPending(now time.Time) {
	for key, pending := range manager.pending {
		if !now.Before(pending.expiresAt) {
			delete(manager.pending, key)
		}
	}
}

func (manager *Manager) cleanupSessions(now time.Time) {
	for key, session := range manager.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(manager.sessions, key)
		}
	}
}

func validRole(role Role) bool {
	return role == RoleViewer || role == RoleEditor || role == RoleOwner
}

func randomCredential() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate hosted authentication credential: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
