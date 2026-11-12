// Package service porte les cas d'usage de cfa : droits, règles métier, transactions
// et événements. Il ne connaît pas HTTP.
package service

import (
	"context"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// Service regroupe les cas d'usage.
type Service struct {
	store *store.Store
	cfg   *config.Config
	now   func() time.Time
}

// New crée le service.
func New(st *store.Store, cfg *config.Config) *Service {
	return &Service{store: st, cfg: cfg, now: time.Now}
}

// ---- Conversions base → contrats ----

func goalToAPI(g db.Goal) apitypes.Goal {
	return apitypes.Goal{
		ID: g.ID, Title: g.Title, Description: g.Description, Status: g.Status,
		BaseBranch: g.BaseBranch, Branch: g.Branch,
		CreatedAt: g.CreatedAt, FinishedAt: g.FinishedAt,
	}
}

func taskToAPI(t db.Task, deps []int64) apitypes.Task {
	if deps == nil {
		deps = []int64{}
	}
	return apitypes.Task{
		ID: t.ID, GoalID: t.GoalID, Role: t.Role, Title: t.Title, Spec: t.Spec, Status: t.Status,
		Branch: t.Branch, DependsOn: deps, Summary: t.Summary,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

func approvalToAPI(a db.Approval) apitypes.Approval {
	return apitypes.Approval{
		ID: a.ID, GoalID: a.GoalID, Kind: a.Kind, Payload: a.Payload,
		Status: a.Status, Comment: a.Comment, CreatedAt: a.CreatedAt, DecidedAt: a.DecidedAt,
	}
}

func runToAPI(r db.Run) apitypes.Run {
	return apitypes.Run{
		ID: r.ID, GoalID: r.GoalID, TaskID: r.TaskID, Kind: r.Kind, Role: r.Role, Model: r.Model,
		Status: r.Status, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt,
	}
}

// ---- Changements de statut ----
// Tout changement de statut passe par ces fonctions : transition vérifiée et événement publié.

func (s *Service) changeGoalStatus(ctx context.Context, q *db.Queries, g db.Goal, to domain.GoalStatus, a domain.Actor, actor string, reason *string) (db.Goal, error) {
	from := domain.GoalStatus(g.Status)
	if !domain.CanTransitionGoal(from, to, a) {
		if from.Terminal() {
			return g, domain.GoalClosed(g.ID)
		}
		return g, domain.InvalidTransition(string(from), string(to))
	}
	ng, err := q.UpdateGoalStatus(ctx, db.UpdateGoalStatusParams{ID: g.ID, Status: string(to)})
	if err != nil {
		return g, err
	}
	_, err = events.Emit(ctx, q, &g.ID, events.GoalStatusChanged, actor, events.GoalStatusChangedPayload{
		GoalID: g.ID, Title: g.Title, From: string(from), To: string(to), Reason: reason,
	})
	return ng, err
}

type taskChange struct {
	reason  *string
	summary *string
}

func (s *Service) changeTaskStatus(ctx context.Context, q *db.Queries, t db.Task, to domain.TaskStatus, a domain.Actor, actor string, c taskChange) (db.Task, error) {
	from := domain.TaskStatus(t.Status)
	if !domain.CanTransitionTask(from, to, a) {
		return t, domain.InvalidTransition(string(from), string(to))
	}
	nt, err := q.UpdateTaskStatus(ctx, db.UpdateTaskStatusParams{
		ID: t.ID, Status: string(to), Summary: c.summary,
	})
	if err != nil {
		return t, err
	}
	_, err = events.Emit(ctx, q, &t.GoalID, events.TaskStatusChanged, actor, events.TaskStatusChangedPayload{
		TaskID: t.ID, Role: t.Role, Title: t.Title, From: string(from), To: string(to),
		Reason: c.reason, Summary: nt.Summary,
	})
	return nt, err
}

// getGoalFor charge un objectif visible par l'appelant (404 sinon).
func getGoalFor(ctx context.Context, q *db.Queries, visible func(int64) bool, id int64) (db.Goal, error) {
	g, err := q.GetGoal(ctx, id)
	if store.IsNotFound(err) || (err == nil && !visible(g.ID)) {
		return g, domain.NotFound("Objectif")
	}
	return g, err
}

func ptr[T any](v T) *T { return &v }
