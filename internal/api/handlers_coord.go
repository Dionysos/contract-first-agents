package api

import (
	"net/http"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
)

// Routes de coordination : approbation du merge, runs, contrôles.

func (s *Server) listApprovals(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	goal, err := queryInt(r, "goal")
	if err != nil {
		return err
	}
	items, err := s.svc.ListApprovals(r.Context(), p, r.URL.Query().Get("status"), goal)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
	return nil
}

func (s *Server) createApproval(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	var in apitypes.ApprovalCreate
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	a, err := s.svc.CreateApproval(r.Context(), p, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, a)
	return nil
}

func (s *Server) decideApproval(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in apitypes.ApprovalDecision
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	a, err := s.svc.DecideApproval(r.Context(), p, id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, a)
	return nil
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	var in apitypes.RunCreate
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	out, err := s.svc.CreateRun(r.Context(), p, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

func (s *Server) finishRun(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in apitypes.RunFinish
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	run, err := s.svc.FinishRun(r.Context(), p, id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, run)
	return nil
}

func (s *Server) reportCheck(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	var in apitypes.CheckReport
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	ev, err := s.svc.ReportCheck(r.Context(), p, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusAccepted, apitypes.EventRef{EventID: ev})
	return nil
}

func (s *Server) markGoalMerged(w http.ResponseWriter, r *http.Request, p auth.Principal) error {
	id, err := pathID(r)
	if err != nil {
		return err
	}
	var in apitypes.GoalMerged
	if err := decodeJSON(r, &in, false); err != nil {
		return err
	}
	g, err := s.svc.MarkGoalMerged(r.Context(), p, id, in)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, g)
	return nil
}
