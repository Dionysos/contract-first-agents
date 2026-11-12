package service

import (
	"context"
	"net/url"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// Durées de vie des accès au dashboard.
const (
	LoginCodeTTL = 2 * time.Minute
	SessionTTL   = 12 * time.Hour
)

// LoginCode est la réponse de POST /auth/code.
type LoginCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
	LoginURL  string    `json:"login_url"`
}

// CreateLoginCode crée un code de connexion à usage unique (POST /auth/code).
// Réservé à l'humain authentifié par token (pas par session).
func (s *Service) CreateLoginCode(ctx context.Context, p auth.Principal) (LoginCode, error) {
	if !p.IsHuman() || p.Session {
		return LoginCode{}, domain.Forbidden("Réservé au token humain (cfa dashboard)")
	}
	plain, hash := auth.NewToken()
	exp := s.now().Add(LoginCodeTTL)
	if err := s.store.Q.InsertLoginCode(ctx, db.InsertLoginCodeParams{Hash: hash, ExpiresAt: exp}); err != nil {
		return LoginCode{}, err
	}
	return LoginCode{Code: plain, ExpiresAt: exp, LoginURL: s.cfg.DashboardOrigin + "/login?code=" + url.QueryEscape(plain)}, nil
}

// Login échange un code contre un cookie de session (GET /login).
func (s *Service) Login(ctx context.Context, code string) (cookie string, expires time.Time, err error) {
	if code == "" {
		return "", time.Time{}, domain.Unauthorized()
	}
	if _, err := s.store.Q.UseLoginCode(ctx, auth.Hash(code)); store.IsNotFound(err) {
		return "", time.Time{}, domain.Unauthorized()
	} else if err != nil {
		return "", time.Time{}, err
	}
	plain, hash := auth.NewToken()
	expires = s.now().Add(SessionTTL)
	if err := s.store.Q.InsertSession(ctx, db.InsertSessionParams{Hash: hash, ExpiresAt: expires}); err != nil {
		return "", time.Time{}, err
	}
	return plain, expires, nil
}

// PurgeExpired supprime les sessions et codes de connexion expirés.
func (s *Service) PurgeExpired(ctx context.Context) error {
	return s.store.Q.PurgeAuth(ctx)
}
