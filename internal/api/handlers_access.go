package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Dionysos/contract-first-agents/internal/auth"
)

// Connexion du dashboard : « cfa dashboard » obtient un code à usage unique (POST /auth/code),
// échangé contre un cookie de session par GET /login (docs/adr/0013).

func (s *Server) createLoginCode(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	c, err := s.svc.CreateLoginCode(r.Context(), p)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, c)
	return nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	cookie, expires, err := s.svc.Login(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "Code de connexion invalide, expiré ou déjà utilisé. Relancez « cfa dashboard ».")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: auth.SessionCookie, Value: cookie, Path: "/", Expires: expires,
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: strings.HasPrefix(s.cfg.DashboardOrigin, "https://"),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
