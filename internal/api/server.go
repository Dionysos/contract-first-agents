// Package api expose le service en HTTP : routes, erreurs, flux SSE, dashboard.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/service"
)

// MaxBodyBytes limite la taille des corps de requête.
const MaxBodyBytes = 1 << 20

// Server porte les dépendances des handlers.
type Server struct {
	svc       *service.Service
	auth      *auth.Authenticator
	source    events.Source
	cfg       *config.Config
	log       *slog.Logger
	dashboard fs.FS
	health    func(ctx context.Context) error
}

// Options configure le serveur.
type Options struct {
	Service   *service.Service
	Auth      *auth.Authenticator
	Source    events.Source
	Config    *config.Config
	Log       *slog.Logger
	Dashboard fs.FS                           // nil : pas de dashboard
	Health    func(ctx context.Context) error // nil : toujours ok
}

// New crée le serveur.
func New(o Options) *Server {
	return &Server{
		svc: o.Service, auth: o.Auth, source: o.Source, cfg: o.Config, log: o.Log,
		dashboard: o.Dashboard, health: o.Health,
	}
}

// Handler renvoie le routeur complet, middlewares compris.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Publiques.
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /login", s.login)
	if s.dashboard != nil {
		mux.Handle("GET /", http.FileServerFS(s.dashboard))
	}

	// Authentifiées.
	authed := func(pattern string, h func(http.ResponseWriter, *http.Request, auth.Principal) error) {
		mux.Handle(pattern, s.withAuth(h))
	}
	authed("GET /events", s.streamEvents)
	authed("POST /goals", s.createGoal)
	authed("GET /goals", s.listGoals)
	authed("GET /goals/{id}", s.getGoal)
	authed("POST /goals/{id}/cancel", s.cancelGoal)
	authed("POST /goals/{id}/status", s.setGoalStatus)
	authed("POST /goals/{id}/merged", s.markGoalMerged)
	authed("POST /tasks", s.createTask)
	authed("GET /tasks/{id}", s.getTask)
	authed("POST /tasks/{id}/status", s.setTaskStatus)
	authed("GET /approvals", s.listApprovals)
	authed("POST /approvals", s.createApproval)
	authed("POST /approvals/{id}/decision", s.decideApproval)
	authed("POST /runs", s.createRun)
	authed("POST /runs/{id}/finish", s.finishRun)
	authed("POST /checks", s.reportCheck)
	authed("POST /auth/code", s.createLoginCode)
	return s.recoverer(s.logger(mux))
}

// ---- Middlewares ----

func (s *Server) withAuth(h func(http.ResponseWriter, *http.Request, auth.Principal) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.auth.FromRequest(r.Context(), r)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		if rec, ok := w.(*statusRecorder); ok {
			rec.role = p.Role
		}
		// Écriture par cookie : l'en-tête Origin doit être celui du dashboard (CSRF).
		if p.Session && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get("Origin") != s.cfg.DashboardOrigin {
			s.writeError(w, r, &domain.Error{Status: http.StatusForbidden, Code: domain.CodeOriginMismatch, Message: "En-tête Origin invalide"})
			return
		}
		r = r.WithContext(auth.WithPrincipal(r.Context(), p))
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		}
		if err := h(w, r, p); err != nil {
			s.writeError(w, r, err)
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	role   string
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush laisse passer le flush du SSE.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap permet à http.ResponseController d'atteindre le writer d'origine.
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.InfoContext(r.Context(), "http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"role", rec.role, "ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panique", "path", r.URL.Path, "err", v)
				writeJSON(w, http.StatusInternalServerError, apitypes.Error{Code: domain.CodeInternal, Message: "Erreur interne"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ---- Aides JSON ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var de *domain.Error
	if errors.As(err, &de) {
		writeJSON(w, de.Status, apitypes.Error{Code: de.Code, Message: de.Message, Errors: de.Fields})
		return
	}
	s.log.Error("erreur interne", "method", r.Method, "path", r.URL.Path, "err", err)
	writeJSON(w, http.StatusInternalServerError, apitypes.Error{Code: domain.CodeInternal, Message: "Erreur interne"})
}

// decodeJSON lit un corps JSON strict : type de contenu, taille, champs inconnus.
// Un corps vide est accepté si optional vaut true.
func decodeJSON(r *http.Request, v any, optional bool) error {
	if r.ContentLength == 0 && optional {
		return nil
	}
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		return &domain.Error{Status: http.StatusUnsupportedMediaType, Code: domain.CodeUnsupportedMediaType, Message: "Content-Type: application/json attendu"}
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(v)
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		return &domain.Error{Status: http.StatusRequestEntityTooLarge, Code: domain.CodePayloadTooLarge, Message: "Corps trop volumineux (1 Mio maximum)"}
	case errors.Is(err, io.EOF) && optional:
		return nil
	case err != nil:
		return domain.Validation(map[string]string{"body": "JSON invalide : " + err.Error()})
	}
	if dec.More() {
		return domain.Validation(map[string]string{"body": "un seul objet JSON attendu"})
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, domain.NotFound("Ressource")
	}
	return id, nil
}

func queryInt(r *http.Request, name string) (*int64, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, domain.Validation(map[string]string{name: "entier attendu"})
	}
	return &n, nil
}
