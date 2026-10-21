package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/auth"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
)

// Token kinds (FR-SRV-101): short-lived access tokens, rotating refresh
// tokens and long-lived personal API tokens for the open API.
const (
	TokenAccess  = "access"
	TokenRefresh = "refresh"
	TokenAPI     = "api"
)

func (s *Service) issue(ctx context.Context, user pgtype.UUID, kind, label string, ttl time.Duration) (string, error) {
	prefix := auth.UserTokenPrefix
	switch kind {
	case TokenRefresh:
		prefix = "cor_"
	case TokenAPI:
		prefix = "coa_"
	}
	tok := auth.NewToken(prefix)
	err := s.Q.CreateTokenKind(ctx, db.CreateTokenKindParams{
		TokenHash: auth.Hash(tok), UserID: user, Kind: kind, Label: label,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(ttl), Valid: true},
	})
	return tok, err
}

// TokenPair is an access token plus its refresh token.
type TokenPair struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    string `json:"expires_at"`
}

func (s *Service) issuePair(ctx context.Context, user pgtype.UUID) (TokenPair, error) {
	access, err := s.issue(ctx, user, TokenAccess, "", s.Cfg.AccessTokenTTL)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := s.issue(ctx, user, TokenRefresh, "", s.Cfg.RefreshTokenTTL)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{Token: access, RefreshToken: refresh, ExpiresAt: time.Now().Add(s.Cfg.AccessTokenTTL).UTC().Format(time.RFC3339)}, nil
}

// Refresh rotates a refresh token: the old one is revoked, a new pair issued.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (TokenPair, error) {
	t, err := s.Q.GetActiveToken(ctx, auth.Hash(refreshToken))
	if err != nil || t.Kind != TokenRefresh {
		return TokenPair{}, apperr.ErrUnauthorized
	}
	if err := s.Q.RevokeToken(ctx, t.TokenHash); err != nil {
		return TokenPair{}, err
	}
	return s.issuePair(ctx, t.UserID)
}

// Logout revokes the presented access and refresh tokens.
func (s *Service) Logout(ctx context.Context, access, refresh string) {
	_ = s.Q.RevokeToken(ctx, auth.Hash(access))
	if refresh != "" {
		_ = s.Q.RevokeToken(ctx, auth.Hash(refresh))
	}
}

// APITokenDTO describes a personal API token (the secret is shown once).
type APITokenDTO struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Token     string `json:"token,omitempty"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// CreateAPIToken issues a personal API token for the open API/SDK.
func (s *Service) CreateAPIToken(ctx context.Context, userID, label string) (APITokenDTO, error) {
	if label == "" || len(label) > 64 {
		return APITokenDTO{}, apperr.BadRequest("label is required (max 64 chars)")
	}
	tok, err := s.issue(ctx, util.MustUUID(userID), TokenAPI, label, 365*24*time.Hour)
	if err != nil {
		return APITokenDTO{}, err
	}
	h := auth.Hash(tok)
	return APITokenDTO{ID: h[:16], Label: label, Token: tok, ExpiresAt: time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339)}, nil
}

// ListAPITokens lists the user's live API tokens.
func (s *Service) ListAPITokens(ctx context.Context, userID string) ([]APITokenDTO, error) {
	rows, err := s.Q.ListApiTokens(ctx, util.MustUUID(userID))
	if err != nil {
		return nil, err
	}
	out := make([]APITokenDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, APITokenDTO{ID: r.TokenHash[:16], Label: r.Label, ExpiresAt: util.TS(r.ExpiresAt), CreatedAt: util.TS(r.CreatedAt)})
	}
	return out, nil
}

// RevokeAPIToken revokes one of the user's API tokens by id.
func (s *Service) RevokeAPIToken(ctx context.Context, userID, id string) error {
	rows, err := s.Q.ListApiTokens(ctx, util.MustUUID(userID))
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.TokenHash[:16] == id {
			return s.Q.RevokeToken(ctx, r.TokenHash)
		}
	}
	return apperr.ErrNotFound
}

// RevokeDaemon revokes a device's node token and drops its connection.
func (s *Service) RevokeDaemon(ctx context.Context, wsID, userID, daemonID string) error {
	d, err := s.ownDaemon(ctx, wsID, userID, daemonID)
	if err != nil {
		return err
	}
	if err := s.Q.RevokeDaemon(ctx, d.ID); err != nil {
		return err
	}
	s.Daemons.Kick(daemonID)
	s.Audit(ctx, d.WorkspaceID, "user", userID, "daemon.revoke", "daemon", daemonID, nil)
	s.dock(d.WorkspaceID, "devices")
	return nil
}
