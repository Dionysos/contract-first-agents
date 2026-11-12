package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/domain"
)

// PingInterval est l'intervalle des commentaires de maintien de connexion.
var PingInterval = 15 * time.Second

// streamEvents sert GET /events en SSE (docs/api.md#flux-sse).
func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	goal, err := queryInt(r, "goal")
	if err != nil {
		return err
	}
	if p.IsAgent() {
		if goal != nil && *goal != *p.GoalID {
			return domain.NotFound("Objectif")
		}
		goal = p.GoalID
	}

	// Point de départ : Last-Event-ID, sinon since, sinon seulement les nouveaux événements.
	var after int64 = -1
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return domain.Validation(map[string]string{"Last-Event-ID": "entier attendu"})
		}
		after = n
	} else if since, err := queryInt(r, "since"); err != nil {
		return err
	} else if since != nil {
		if *since < 0 {
			return domain.Validation(map[string]string{"since": "positif"})
		}
		after = *since
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming non supporté")
	}

	// S'abonner avant de lire l'historique : aucun événement ne peut tomber entre les deux.
	sub := s.source.Subscribe(goal)
	defer sub.Close()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	var last int64
	if after >= 0 {
		last = after
		for {
			batch, err := s.source.Backlog(r.Context(), goal, last, 1000)
			if err != nil {
				s.log.Error("sse : lecture de l'historique", "err", err)
				return nil // en-têtes déjà envoyés
			}
			for _, ev := range batch {
				if err := writeEvent(w, ev); err != nil {
					return nil
				}
				last = ev.ID
			}
			flusher.Flush()
			if len(batch) < 1000 {
				break
			}
		}
	}

	ping := time.NewTicker(PingInterval)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return nil
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return nil
			}
			flusher.Flush()
		case ev, ok := <-sub.C:
			if !ok {
				return nil // abonné trop lent ou arrêt : le client se reconnecte
			}
			if ev.ID <= last {
				continue // déjà envoyé avec l'historique
			}
			if err := writeEvent(w, ev); err != nil {
				return nil
			}
			last = ev.ID
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, ev apitypes.Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, b)
	return err
}
