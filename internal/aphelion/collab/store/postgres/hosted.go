package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"sdmm/internal/aphelion/collab/model"
	collabstore "sdmm/internal/aphelion/collab/store"
)

func (store *Store) CreateHostedSession(ctx context.Context, session collabstore.HostedSession, owner collabstore.HostedMember) error {
	if err := validateHostedSession(session, owner); err != nil {
		return err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin hosted session create: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	if _, err := transaction.Exec(ctx, `INSERT INTO collaboration_hosted_sessions(session_id, document_id, created_at) VALUES($1, $2, $3)`, session.SessionID, session.DocumentID, session.CreatedAt); err != nil {
		if postgresCode(err) == "23505" {
			return collabstore.ErrHostedSessionExists
		}
		return fmt.Errorf("insert hosted session: %w", err)
	}
	if err := insertHostedMember(ctx, transaction, owner); err != nil {
		return err
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit hosted session create: %w", err)
	}
	return nil
}

func (store *Store) ListHostedSessions(ctx context.Context) ([]collabstore.HostedSession, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return nil, collabstore.ErrStoreClosed
	}
	rows, err := store.pool.Query(ctx, `SELECT session_id, document_id, created_at FROM collaboration_hosted_sessions ORDER BY session_id`)
	if err != nil {
		return nil, fmt.Errorf("list hosted sessions: %w", err)
	}
	defer rows.Close()
	var sessions []collabstore.HostedSession
	for rows.Next() {
		var session collabstore.HostedSession
		if err := rows.Scan(&session.SessionID, &session.DocumentID, &session.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan hosted session: %w", err)
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (store *Store) ListHostedMembers(ctx context.Context, sessionID string) ([]collabstore.HostedMember, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return nil, collabstore.ErrStoreClosed
	}
	rows, err := store.pool.Query(ctx, `SELECT issuer, subject, actor_id, display_name, role, disabled FROM collaboration_hosted_members WHERE session_id = $1 ORDER BY actor_id`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list hosted members: %w", err)
	}
	defer rows.Close()
	var members []collabstore.HostedMember
	for rows.Next() {
		member := collabstore.HostedMember{SessionID: sessionID}
		if err := rows.Scan(&member.Issuer, &member.Subject, &member.ActorID, &member.DisplayName, &member.Role, &member.Disabled); err != nil {
			return nil, fmt.Errorf("scan hosted member: %w", err)
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (store *Store) ResolveHostedMember(ctx context.Context, sessionID, issuer, subject string) (collabstore.HostedMember, bool, error) {
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.HostedMember{}, false, collabstore.ErrStoreClosed
	}
	member := collabstore.HostedMember{SessionID: sessionID, Issuer: issuer, Subject: subject}
	err := store.pool.QueryRow(ctx, `SELECT actor_id, display_name, role, disabled FROM collaboration_hosted_members WHERE session_id = $1 AND issuer = $2 AND subject = $3`, sessionID, issuer, subject).Scan(&member.ActorID, &member.DisplayName, &member.Role, &member.Disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return collabstore.HostedMember{}, false, nil
	}
	if err != nil {
		return collabstore.HostedMember{}, false, fmt.Errorf("resolve hosted member: %w", err)
	}
	return member, true, nil
}

func (store *Store) UpdateHostedMemberDisplayName(ctx context.Context, sessionID string, actorID model.ActorID, displayName string) error {
	displayName = strings.TrimSpace(displayName)
	if sessionID == "" || displayName == "" || len(displayName) > 128 {
		return fmt.Errorf("hosted member display name is invalid")
	}
	if err := actorID.Validate(); err != nil {
		return err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	result, err := store.pool.Exec(ctx, `UPDATE collaboration_hosted_members SET display_name = $3 WHERE session_id = $1 AND actor_id = $2`, sessionID, actorID, displayName)
	if err != nil {
		return fmt.Errorf("update hosted member display name: %w", err)
	}
	if result.RowsAffected() != 1 {
		return collabstore.ErrHostedSessionMissing
	}
	return nil
}

func (store *Store) CreateHostedInvitation(ctx context.Context, invitation collabstore.HostedInvitation) error {
	if invitation.SessionID == "" || len(invitation.SessionID) > 128 || (invitation.Role != collabstore.HostedRoleViewer && invitation.Role != collabstore.HostedRoleEditor) || invitation.ExpiresAt.IsZero() {
		return fmt.Errorf("hosted invitation is invalid")
	}
	if err := invitation.CreatedByActorID.Validate(); err != nil {
		return err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.ErrStoreClosed
	}
	_, err := store.pool.Exec(ctx, `INSERT INTO collaboration_hosted_invitations(token_hash, session_id, role, created_by_actor_id, expires_at) VALUES($1, $2, $3, $4, $5)`, invitation.TokenHash[:], invitation.SessionID, invitation.Role, invitation.CreatedByActorID, invitation.ExpiresAt)
	if err != nil {
		return fmt.Errorf("insert hosted invitation: %w", err)
	}
	return nil
}

func (store *Store) RedeemHostedInvitation(ctx context.Context, expectedSessionID string, tokenHash [sha256.Size]byte, identity collabstore.HostedIdentity, now time.Time) (collabstore.HostedMember, error) {
	if identity.Issuer == "" || identity.Subject == "" || identity.DisplayName == "" || len(identity.DisplayName) > 128 {
		return collabstore.HostedMember{}, collabstore.ErrHostedInvitationInvalid
	}
	if err := identity.ActorID.Validate(); err != nil {
		return collabstore.HostedMember{}, err
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	if store.closed {
		return collabstore.HostedMember{}, collabstore.ErrStoreClosed
	}
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return collabstore.HostedMember{}, fmt.Errorf("begin hosted invitation redeem: %w", err)
	}
	defer func() { _ = transaction.Rollback(context.Background()) }()
	var sessionID string
	var role collabstore.HostedRole
	var expiresAt time.Time
	var redeemedAt *time.Time
	if err := transaction.QueryRow(ctx, `SELECT session_id, role, expires_at, redeemed_at FROM collaboration_hosted_invitations WHERE token_hash = $1 AND session_id = $2 FOR UPDATE`, tokenHash[:], expectedSessionID).Scan(&sessionID, &role, &expiresAt, &redeemedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return collabstore.HostedMember{}, collabstore.ErrHostedInvitationInvalid
		}
		return collabstore.HostedMember{}, fmt.Errorf("load hosted invitation: %w", err)
	}
	if redeemedAt != nil || !now.Before(expiresAt) {
		return collabstore.HostedMember{}, collabstore.ErrHostedInvitationInvalid
	}
	member := collabstore.HostedMember{SessionID: sessionID, Issuer: identity.Issuer, Subject: identity.Subject, ActorID: identity.ActorID, DisplayName: identity.DisplayName, Role: role}
	if err := insertHostedMember(ctx, transaction, member); err != nil {
		return collabstore.HostedMember{}, err
	}
	if _, err := transaction.Exec(ctx, `UPDATE collaboration_hosted_invitations SET redeemed_at = $2 WHERE token_hash = $1`, tokenHash[:], now); err != nil {
		return collabstore.HostedMember{}, fmt.Errorf("redeem hosted invitation: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return collabstore.HostedMember{}, fmt.Errorf("commit hosted invitation redeem: %w", err)
	}
	return member, nil
}

func validateHostedSession(session collabstore.HostedSession, owner collabstore.HostedMember) error {
	if session.SessionID == "" || len(session.SessionID) > 128 || session.CreatedAt.IsZero() || owner.SessionID != session.SessionID || owner.Role != collabstore.HostedRoleOwner || owner.Disabled {
		return fmt.Errorf("hosted session or owner is invalid")
	}
	if err := session.DocumentID.Validate(); err != nil {
		return err
	}
	return validateHostedMember(owner)
}

func validateHostedMember(member collabstore.HostedMember) error {
	if member.SessionID == "" || member.Issuer == "" || member.Subject == "" || member.DisplayName == "" || len(member.DisplayName) > 128 || !collabstore.ValidHostedRole(member.Role) {
		return fmt.Errorf("hosted member is invalid")
	}
	return member.ActorID.Validate()
}

func insertHostedMember(ctx context.Context, transaction pgx.Tx, member collabstore.HostedMember) error {
	if err := validateHostedMember(member); err != nil {
		return err
	}
	_, err := transaction.Exec(ctx, `INSERT INTO collaboration_hosted_members(session_id, issuer, subject, actor_id, display_name, role, disabled) VALUES($1, $2, $3, $4, $5, $6, $7)`, member.SessionID, member.Issuer, member.Subject, member.ActorID, member.DisplayName, member.Role, member.Disabled)
	if postgresCode(err) == "23505" {
		return collabstore.ErrHostedMemberExists
	}
	if err != nil {
		return fmt.Errorf("insert hosted member: %w", err)
	}
	return nil
}

func postgresCode(err error) string {
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) {
		return postgresError.Code
	}
	return ""
}

var _ collabstore.HostedRegistry = (*Store)(nil)
