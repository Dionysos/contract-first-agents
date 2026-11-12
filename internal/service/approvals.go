package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// Phase 1 : la seule décision demandée à l'humain est le merge d'un objectif terminé.

// ListApprovals liste les approbations (GET /approvals).
func (s *Service) ListApprovals(ctx context.Context, p auth.Principal, status string, goal *int64) ([]apitypes.Approval, error) {
	if !p.IsHuman() {
		return nil, domain.Forbidden("")
	}
	params := db.ListApprovalsParams{GoalID: goal}
	switch status {
	case "":
	case "pending", "approved", "rejected":
		params.Status = &status
	default:
		return nil, domain.Validation(map[string]string{"status": "pending, approved ou rejected"})
	}
	rows, err := s.store.Q.ListApprovals(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]apitypes.Approval, len(rows))
	for i, a := range rows {
		out[i] = approvalToAPI(a)
	}
	return out, nil
}

// CreateApproval demande l'approbation du merge au nom du runner (POST /approvals) :
// l'objectif passe en awaiting_approval.
func (s *Service) CreateApproval(ctx context.Context, p auth.Principal, in apitypes.ApprovalCreate) (apitypes.Approval, error) {
	if !p.IsRunner() {
		return apitypes.Approval{}, domain.Forbidden("")
	}
	if in.Kind != domain.ApprovalMerge {
		return apitypes.Approval{}, domain.Validation(map[string]string{"kind": "merge"})
	}
	var payload apitypes.MergePayload
	dec := json.NewDecoder(bytes.NewReader(in.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		return apitypes.Approval{}, domain.Validation(map[string]string{"payload": "invalide : " + err.Error()})
	}
	f := domain.Fields{}
	f.Len("payload.branch", payload.Branch, 1, 200)
	f.Len("payload.base_branch", payload.BaseBranch, 1, 200)
	if err := f.Err(); err != nil {
		return apitypes.Approval{}, err
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return apitypes.Approval{}, err
	}

	var a db.Approval
	err = s.store.Write(ctx, func(q *db.Queries) error {
		g, err := getGoalFor(ctx, q, p.CanSeeGoal, in.GoalID)
		if err != nil {
			return err
		}
		if domain.GoalStatus(g.Status).Terminal() {
			return domain.GoalClosed(g.ID)
		}
		if _, err := q.GetPendingApproval(ctx, db.GetPendingApprovalParams{Kind: in.Kind, GoalID: g.ID}); err == nil {
			return domain.Conflict(domain.CodeDuplicateApproval, "Une approbation identique est déjà en attente")
		} else if !store.IsNotFound(err) {
			return err
		}
		if _, err := s.changeGoalStatus(ctx, q, g, domain.GoalAwaitingApproval, domain.ActorSystem, domain.RoleSystem, nil); err != nil {
			return err
		}
		if a, err = q.InsertApproval(ctx, db.InsertApprovalParams{GoalID: g.ID, Kind: in.Kind, Payload: b}); err != nil {
			return err
		}
		_, err = events.Emit(ctx, q, &g.ID, events.ApprovalRequested, p.Role, events.ApprovalRequestedPayload{Approval: approvalToAPI(a)})
		return err
	})
	return approvalToAPI(a), err
}

// DecideApproval enregistre la décision humaine (POST /approvals/{id}/decision).
// Accepté : le runner merge. Refusé : l'objectif est annulé (pas de reprise en phase 1).
func (s *Service) DecideApproval(ctx context.Context, p auth.Principal, id int64, in apitypes.ApprovalDecision) (apitypes.Approval, error) {
	if !p.IsHuman() {
		return apitypes.Approval{}, domain.Forbidden("")
	}
	f := domain.Fields{}
	comment := ""
	if in.Comment != nil {
		comment = *in.Comment
		f.Len("comment", comment, 0, 10000)
	}
	status := map[string]string{"approve": "approved", "reject": "rejected"}[in.Decision]
	if status == "" {
		f.Add("decision", "approve ou reject")
	}
	if in.Decision == "reject" && comment == "" {
		f.Add("comment", "obligatoire pour refuser")
	}
	if err := f.Err(); err != nil {
		return apitypes.Approval{}, err
	}

	var a db.Approval
	err := s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		a, err = q.GetApproval(ctx, id)
		if store.IsNotFound(err) {
			return domain.NotFound("Approbation")
		}
		if err != nil {
			return err
		}
		if a.Status != "pending" {
			return domain.Conflict(domain.CodeAlreadyDecided, fmt.Sprintf("L'approbation %d est déjà %s", id, a.Status))
		}
		g, err := q.GetGoal(ctx, a.GoalID)
		if err != nil {
			return err
		}
		var c *string
		if comment != "" {
			c = &comment
		}
		if a, err = q.DecideApproval(ctx, db.DecideApprovalParams{ID: id, Status: status, Comment: c}); err != nil {
			return err
		}
		if in.Decision == "reject" && !domain.GoalStatus(g.Status).Terminal() {
			if _, err := s.changeGoalStatus(ctx, q, g, domain.GoalCancelled, domain.ActorSystem, domain.RoleSystem, ptr("merge refusé : "+comment)); err != nil {
				return err
			}
		}
		_, err = events.Emit(ctx, q, &a.GoalID, events.ApprovalResolved, p.Role, events.ApprovalResolvedPayload{
			ApprovalID: a.ID, Kind: a.Kind, Decision: in.Decision, Comment: c,
		})
		return err
	})
	return approvalToAPI(a), err
}
