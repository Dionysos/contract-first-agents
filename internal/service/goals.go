package service

import (
	"context"
	"regexp"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// branchName accepte les noms de branches git usuels. L'existence de la branche est
// vérifiée par le runner (l'API n'a pas accès au dépôt cible).
var branchName = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

// CreateGoal crée un objectif (POST /goals).
func (s *Service) CreateGoal(ctx context.Context, p auth.Principal, in apitypes.GoalCreate) (apitypes.Goal, error) {
	if !p.IsHuman() {
		return apitypes.Goal{}, domain.Forbidden("")
	}
	base := s.cfg.BaseBranch
	if in.BaseBranch != nil {
		base = *in.BaseBranch
	}
	f := domain.Fields{}
	f.Len("title", in.Title, 5, 120)
	f.Len("description", in.Description, 20, 5000)
	if !branchName.MatchString(base) {
		f.Add("base_branch", "nom de branche invalide")
	}
	if err := f.Err(); err != nil {
		return apitypes.Goal{}, err
	}

	var goal db.Goal
	err := s.store.Write(ctx, func(q *db.Queries) error {
		var err error
		goal, err = q.InsertGoal(ctx, db.InsertGoalParams{Title: in.Title, Description: in.Description, BaseBranch: base})
		if err != nil {
			return err
		}
		_, err = events.Emit(ctx, q, &goal.ID, events.GoalCreated, p.Role, events.GoalCreatedPayload{Goal: goalToAPI(goal)})
		return err
	})
	if err != nil {
		return apitypes.Goal{}, err
	}
	return goalToAPI(goal), nil
}

// ListGoals liste les objectifs (GET /goals), du plus récent au plus ancien.
func (s *Service) ListGoals(ctx context.Context, p auth.Principal, status string, limit int, before *int64) (apitypes.GoalPage, error) {
	f := domain.Fields{}
	if status != "" && !domain.GoalStatus(status).Valid() {
		f.Add("status", "statut inconnu")
	}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		f.Add("limit", "entre 1 et 200")
	}
	if err := f.Err(); err != nil {
		return apitypes.GoalPage{}, err
	}
	params := db.ListGoalsParams{Before: before, MaxRows: int32(limit)}
	if status != "" {
		params.Status = &status
	}
	if p.IsAgent() {
		params.OnlyID = p.GoalID
	}
	rows, err := s.store.Q.ListGoals(ctx, params)
	if err != nil {
		return apitypes.GoalPage{}, err
	}
	page := apitypes.GoalPage{Items: make([]apitypes.Goal, len(rows))}
	for i, g := range rows {
		page.Items[i] = goalToAPI(g)
	}
	if len(rows) == limit {
		page.NextBefore = ptr(rows[len(rows)-1].ID)
	}
	return page, nil
}

// GetGoal renvoie un objectif avec ses tâches, approbations et runs (GET /goals/{id}).
func (s *Service) GetGoal(ctx context.Context, p auth.Principal, id int64) (apitypes.GoalDetail, error) {
	q := s.store.Q
	g, err := getGoalFor(ctx, q, p.CanSeeGoal, id)
	if err != nil {
		return apitypes.GoalDetail{}, err
	}
	d := apitypes.GoalDetail{Goal: goalToAPI(g)}

	tasks, err := q.ListTasksByGoal(ctx, id)
	if err != nil {
		return d, err
	}
	deps, err := q.ListDepsByGoal(ctx, id)
	if err != nil {
		return d, err
	}
	byTask := map[int64][]int64{}
	for _, dep := range deps {
		byTask[dep.TaskID] = append(byTask[dep.TaskID], dep.DependsOn)
	}
	d.Tasks = make([]apitypes.Task, len(tasks))
	for i, t := range tasks {
		d.Tasks[i] = taskToAPI(t, byTask[t.ID])
	}

	approvals, err := q.ListApprovalsByGoal(ctx, id)
	if err != nil {
		return d, err
	}
	d.Approvals = make([]apitypes.Approval, len(approvals))
	for i, a := range approvals {
		d.Approvals[i] = approvalToAPI(a)
	}

	runs, err := q.ListRunsByGoal(ctx, id)
	if err != nil {
		return d, err
	}
	d.Runs = make([]apitypes.Run, len(runs))
	for i, r := range runs {
		d.Runs[i] = runToAPI(r)
	}
	return d, nil
}

// CancelGoal annule un objectif (POST /goals/{id}/cancel) : tâches ouvertes en échec,
// approbations en attente refusées. Idempotent sur un objectif déjà annulé.
func (s *Service) CancelGoal(ctx context.Context, p auth.Principal, id int64, reason string) (apitypes.Goal, error) {
	if !p.IsHuman() {
		return apitypes.Goal{}, domain.Forbidden("")
	}
	var goal db.Goal
	err := s.store.Write(ctx, func(q *db.Queries) error {
		g, err := getGoalFor(ctx, q, p.CanSeeGoal, id)
		if err != nil {
			return err
		}
		if g.Status == string(domain.GoalCancelled) {
			goal = g
			return nil
		}
		var why *string
		if reason != "" {
			why = &reason
		}
		if goal, err = s.changeGoalStatus(ctx, q, g, domain.GoalCancelled, domain.ActorHuman, p.Role, why); err != nil {
			return err
		}
		open, err := q.ListOpenTasksByGoal(ctx, id)
		if err != nil {
			return err
		}
		for _, t := range open {
			if _, err := s.changeTaskStatus(ctx, q, t, domain.TaskFailed, domain.ActorSystem, domain.RoleSystem, taskChange{reason: ptr("cancelled")}); err != nil {
				return err
			}
		}
		rejected, err := q.RejectPendingApprovalsByGoal(ctx, db.RejectPendingApprovalsByGoalParams{GoalID: id, Comment: "objectif annulé"})
		if err != nil {
			return err
		}
		for _, a := range rejected {
			if _, err := events.Emit(ctx, q, &id, events.ApprovalResolved, domain.RoleSystem, events.ApprovalResolvedPayload{
				ApprovalID: a.ID, Kind: a.Kind, Decision: "reject", Comment: a.Comment,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return goalToAPI(goal), err
}

// SetGoalStatus change le statut d'un objectif à la demande du runner (POST /goals/{id}/status).
func (s *Service) SetGoalStatus(ctx context.Context, p auth.Principal, id int64, in apitypes.GoalStatusChange) (apitypes.Goal, error) {
	if !p.IsRunner() {
		return apitypes.Goal{}, domain.Forbidden("")
	}
	to := domain.GoalStatus(in.Status)
	switch to {
	case domain.GoalPlanning, domain.GoalRunning, domain.GoalFailed:
	default:
		return apitypes.Goal{}, domain.Validation(map[string]string{"status": "planning, running ou failed"})
	}
	var goal db.Goal
	err := s.store.Write(ctx, func(q *db.Queries) error {
		g, err := getGoalFor(ctx, q, p.CanSeeGoal, id)
		if err != nil {
			return err
		}
		if g.Status == string(domain.GoalPlanning) && to == domain.GoalRunning {
			tasks, err := q.ListTasksByGoal(ctx, id)
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				return domain.Conflict(domain.CodeInvalidTransition, "Aucune tâche : l'objectif ne peut pas démarrer")
			}
		}
		var why *string
		if in.Reason != "" {
			why = &in.Reason
		}
		goal, err = s.changeGoalStatus(ctx, q, g, to, domain.ActorRunner, p.Role, why)
		return err
	})
	return goalToAPI(goal), err
}
