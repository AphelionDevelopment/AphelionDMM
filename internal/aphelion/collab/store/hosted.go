package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"sdmm/internal/aphelion/collab/model"
)

type HostedRole string

const (
	HostedRoleViewer HostedRole = "viewer"
	HostedRoleEditor HostedRole = "editor"
	HostedRoleOwner  HostedRole = "owner"
)

var (
	ErrHostedSessionExists     = errors.New("hosted session already exists")
	ErrHostedSessionMissing    = errors.New("hosted session does not exist")
	ErrHostedMemberExists      = errors.New("hosted session member already exists")
	ErrHostedInvitationInvalid = errors.New("hosted invitation is invalid, expired, or redeemed")
)

type HostedSession struct {
	SessionID  string
	DocumentID model.DocumentID
	CreatedAt  time.Time
}

type HostedIdentity struct {
	Issuer      string
	Subject     string
	ActorID     model.ActorID
	DisplayName string
}

type HostedMember struct {
	SessionID   string
	Issuer      string
	Subject     string
	ActorID     model.ActorID
	DisplayName string
	Role        HostedRole
	Disabled    bool
}

type HostedInvitation struct {
	TokenHash        [sha256.Size]byte
	SessionID        string
	Role             HostedRole
	CreatedByActorID model.ActorID
	ExpiresAt        time.Time
}

type HostedRegistry interface {
	CreateHostedSession(context.Context, HostedSession, HostedMember) error
	ListHostedSessions(context.Context) ([]HostedSession, error)
	ListHostedMembers(context.Context, string) ([]HostedMember, error)
	ResolveHostedMember(context.Context, string, string, string) (HostedMember, bool, error)
	CreateHostedInvitation(context.Context, HostedInvitation) error
	RedeemHostedInvitation(context.Context, string, [sha256.Size]byte, HostedIdentity, time.Time) (HostedMember, error)
}

func ValidHostedRole(role HostedRole) bool {
	return role == HostedRoleViewer || role == HostedRoleEditor || role == HostedRoleOwner
}
