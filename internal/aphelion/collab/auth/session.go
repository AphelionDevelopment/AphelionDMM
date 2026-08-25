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

	"golang.org/x/oauth2"

	"sdmm/internal/aphelion/collab/model"
)

var (
	ErrInvalidState     = errors.New("OIDC authorization state is invalid or expired")
	ErrInvalidSession   = errors.New("hosted authentication session is invalid or expired")
	ErrDisabledIdentity = errors.New("hosted identity is disabled")
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
	now              func() time.Time
	pending          map[[sha256.Size]byte]pendingAuthorization
	sessions         map[[sha256.Size]byte]Session
	actors           map[string]model.ActorID
}

func NewManager(flow AuthorizationFlow, directory Directory, config ManagerConfig) *Manager {
	if config.PendingTTL <= 0 {
		config.PendingTTL = 5 * time.Minute
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Manager{
		flow: flow, directory: directory, sessionDirectory: config.SessionDirectory, pendingTTL: config.PendingTTL, now: config.Now,
		pending: make(map[[sha256.Size]byte]pendingAuthorization), sessions: make(map[[sha256.Size]byte]Session), actors: make(map[string]model.ActorID),
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
	manager.mutex.Lock()
	actorKey := identity.Issuer + "\x00" + identity.Subject
	actorID := manager.actors[actorKey]
	if actorID == "" {
		actorID, err = model.NewActorID()
		if err == nil {
			manager.actors[actorKey] = actorID
		}
	}
	if err != nil {
		manager.mutex.Unlock()
		return Session{}, err
	}
	session := Session{ActorID: actorID, Issuer: identity.Issuer, Subject: identity.Subject, DisplayName: identity.DisplayName, Role: role, ExpiresAt: identity.ExpiresAt}
	manager.sessions[sha256.Sum256([]byte(token))] = session
	manager.mutex.Unlock()
	session.Token = token
	return session, nil
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
