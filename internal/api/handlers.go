package api

import (
	"context"
	"net/http"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
)

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if s.health != nil {
		if err := s.health(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, apitypes.Health{Status: "degraded", DB: "down"})
			return
		}
	}
	writeJSON(w, http.StatusOK, apitypes.Health{Status: "ok", DB: "ok"})
}

// ---- Objectifs ----

func (s *Server) createGoal(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	var in apitypes.GoalCreate
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	g, err := s.svc.CreateGoal(r.Context(), p, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, g)
	return nil
}

func (s *Server) listGoals(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	limit, err := queryInt(r, "limit")
	if err != nil {
		return err
	}
	before, err := queryInt(r, "before")
	if err != nil {
		return err
	}
	n := 0
	if limit != nil {
		n = int(*limit)
	}
	page, err := s.svc.ListGoals(r.Context(), p, r.URL.Query().Get("status"), n, before)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, page)
	return nil
}

func (s *Server) getGoal(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	g, err := s.svc.GetGoal(r.Context(), p, id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, g)
	return nil
}

func (s *Server) cancelGoal(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in apitypes.GoalCancel
	if err := decodeJSON(r, &in, true); err != nil {
		return err
	}
	g, err := s.svc.CancelGoal(r.Context(), p, id, in.Reason)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, g)
	return nil
}

func (s *Server) setGoalStatus(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in apitypes.GoalStatusChange
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	g, err := s.svc.SetGoalStatus(r.Context(), p, id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, g)
	return nil
}

// ---- Tâches ----

func (s *Server) createTask(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	var in apitypes.TaskCreate
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	t, err := s.svc.CreateTask(r.Context(), p, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, t)
	return nil
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	t, err := s.svc.GetTask(r.Context(), p, id)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}

func (s *Server) setTaskStatus(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in apitypes.TaskStatusChange
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	t, err := s.svc.SetTaskStatus(r.Context(), p, id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, t)
	return nil
}
