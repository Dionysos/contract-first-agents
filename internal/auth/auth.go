// Package auth authentifie les appels : tokens (CLI, runner, agents) et sessions (dashboard).
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// Principal est l'appelant authentifié.
type Principal struct {
	Role    string
	RunID   *int64
	GoalID  *int64
	TaskID  *int64
	RunKind string
	Session bool
}

// IsHuman indique l'humain (token ou session).
func (p Principal) IsHuman() bool { return p.Role == domain.RoleHuman }

// IsRunner indique le runner.
func (p Principal) IsRunner() bool { return p.Role == domain.RoleRunner }

// IsAgent indique un token d'agent lié à un run.
func (p Principal) IsAgent() bool { return p.RunID != nil }

// CanSeeGoal indique si l'appelant peut voir l'objectif : tout le monde, sauf un agent
// hors de l'objectif de son run.
func (p Principal) CanSeeGoal(goalID int64) bool {
	return !p.IsAgent() || (p.GoalID != nil && *p.GoalID == goalID)
}

// NewToken génère un token aléatoire et son empreinte.
func NewToken() (plain, hash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand ne échoue pas sur les systèmes supportés
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	return plain, Hash(plain)
}

// Hash renvoie l'empreinte SHA-256 hexadécimale stockée en base.
func Hash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// Authenticator résout le principal d'une requête.
type Authenticator struct {
	q   *db.Queries
	now func() time.Time
}

// NewAuthenticator crée un authentificateur sur les requêtes q.
func NewAuthenticator(q *db.Queries) *Authenticator {
	return &Authenticator{q: q, now: time.Now}
}

// SessionCookie est le nom du cookie de session du dashboard.
const SessionCookie = "cfa_session"

// FromRequest lit le token Bearer, sinon le cookie de session du dashboard (rôle human).
func (a *Authenticator) FromRequest(ctx context.Context, r *http.Request) (Principal, error) {
	h := r.Header.Get("Authorization")
	plain, ok := strings.CutPrefix(h, "Bearer ")
	if !ok || plain == "" {
		if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
			if _, err := a.q.GetSession(ctx, Hash(c.Value)); err == nil {
				return Principal{Role: domain.RoleHuman, Session: true}, nil
			} else if !store.IsNotFound(err) {
				return Principal{}, err
			}
		}
		return Principal{}, domain.Unauthorized()
	}
	row, err := a.q.GetTokenPrincipal(ctx, Hash(plain))
	if store.IsNotFound(err) {
		return Principal{}, domain.Unauthorized()
	}
	if err != nil {
		return Principal{}, err
	}
	now := a.now()
	if row.RevokedAt != nil || (row.ExpiresAt != nil && !now.Before(*row.ExpiresAt)) {
		return Principal{}, domain.Unauthorized()
	}
	p := Principal{Role: row.Role, RunID: row.RunID}
	if row.RunID != nil {
		if row.RunStatus == nil || *row.RunStatus != "running" {
			return Principal{}, domain.Unauthorized()
		}
		p.GoalID, p.TaskID = row.RunGoalID, row.RunTaskID
		if row.RunKind != nil {
			p.RunKind = *row.RunKind
		}
	}
	return p, nil
}

type ctxKey struct{}

// WithPrincipal range le principal dans le contexte.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext récupère le principal.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// CreateToken enregistre un token human ou runner et renvoie sa valeur en clair.
func CreateToken(ctx context.Context, q *db.Queries, role, label string) (string, error) {
	switch role {
	case domain.RoleHuman, domain.RoleRunner:
	default:
		return "", domain.Validation(map[string]string{"role": "human ou runner"})
	}
	plain, hash := NewToken()
	err := q.InsertToken(ctx, db.InsertTokenParams{Hash: hash, Role: role, Label: label})
	return plain, err
}

// EnsureTokenFile garantit que path contient un token valide du rôle donné : le fichier
// existant est gardé s'il correspond à un token actif de ce rôle, sinon un token est créé
// et écrit. Sert au démarrage sous docker compose (CFA_BOOTSTRAP_DIR). Renvoie true si
// un token a été créé.
func EnsureTokenFile(ctx context.Context, q *db.Queries, role, label, path string) (bool, error) {
	if b, err := os.ReadFile(path); err == nil {
		if plain := strings.TrimSpace(string(b)); plain != "" {
			row, err := q.GetTokenPrincipal(ctx, Hash(plain))
			switch {
			case err == nil:
				if row.Role == role && row.RunID == nil && row.RevokedAt == nil && row.ExpiresAt == nil {
					return false, nil
				}
			case !store.IsNotFound(err):
				return false, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	plain, err := CreateToken(ctx, q, role, label)
	if err != nil {
		return false, err
	}
	// Écriture atomique : un lecteur ne voit jamais un fichier à moitié écrit.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(plain+"\n"), 0o644); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}
