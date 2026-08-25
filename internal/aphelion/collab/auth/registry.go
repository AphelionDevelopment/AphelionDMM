package auth

import (
	"context"
	"errors"
	"fmt"

	collabstore "sdmm/internal/aphelion/collab/store"
)

var ErrSessionAccessDenied = errors.New("hosted identity is not a member of the collaboration session")

type HostedMemberResolver interface {
	ResolveHostedMember(context.Context, string, string, string) (collabstore.HostedMember, bool, error)
}

type RegistryDirectory struct {
	resolver HostedMemberResolver
}

func NewRegistryDirectory(resolver HostedMemberResolver) *RegistryDirectory {
	return &RegistryDirectory{resolver: resolver}
}

func (directory *RegistryDirectory) Resolve(_ context.Context, identity Identity) (Role, bool, error) {
	if directory == nil || directory.resolver == nil || identity.Issuer == "" || identity.Subject == "" {
		return "", false, fmt.Errorf("hosted identity directory is unavailable")
	}
	return RoleViewer, false, nil
}

func (directory *RegistryDirectory) ResolveSession(ctx context.Context, identity Identity, sessionID string) (Role, bool, error) {
	if directory == nil || directory.resolver == nil {
		return "", false, fmt.Errorf("hosted identity directory is unavailable")
	}
	member, found, err := directory.resolver.ResolveHostedMember(ctx, sessionID, identity.Issuer, identity.Subject)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, ErrSessionAccessDenied
	}
	expectedActorID, err := actorIDForIdentity(identity)
	if err != nil {
		return "", false, err
	}
	if member.ActorID != expectedActorID {
		return "", false, fmt.Errorf("hosted member actor identity is inconsistent")
	}
	role := Role(member.Role)
	if !validRole(role) {
		return "", false, fmt.Errorf("hosted member role %q is invalid", member.Role)
	}
	return role, member.Disabled, nil
}

var _ Directory = (*RegistryDirectory)(nil)
var _ SessionDirectory = (*RegistryDirectory)(nil)
