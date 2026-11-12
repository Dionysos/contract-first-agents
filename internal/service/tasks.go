package service

import (
	"context"
	"fmt"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// CreateTask crée une tâche (POST /tasks). Appelant : le planner d'un run plan, ou l'humain.
func (s *Service) CreateTask(ctx context.Context, p auth.Principal, in apitypes.TaskCreate) (apitypes.Task, error) {
	isPlanner := p.IsAgent() && p.Role == "planner" && p.RunKind == domain.RunPlan
	if !isPlanner && !p.IsHuman() {
		return apitypes.Task{}, domain.Forbidden("")
	}
	f := domain.Fields{}
	var goalID int64
	switch {
	case isPlanner && in.GoalID != nil && *in.GoalID != *p.GoalID:
		f.Add("goal_id", "doit être l'objectif du run")
	case isPlanner:
		goalID = *p.GoalID
	case in.GoalID == nil:
		f.Required("goal_id", false)
	default:
		goalID = *in.GoalID
	}
	if !s.cfg.IsAgentRole(in.Role) || in.Role == "planner" {
		f.Add("role", "rôle d'agent déclaré, autre que planner")
	}
	f.Len("title", in.Title, 5, 120)
	f.Len("spec", in.Spec, 1, 20000)
	seen := map[int64]bool{}
	for _, d := range in.DependsOn {
		if seen[d] {
			f.Add("depends_on", "doublon")
		}
		seen[d] = true
	}
	if err := f.Err(); err != nil {
		return apitypes.Task{}, err
	}

	var task db.Task
	err := s.store.Write(ctx, func(q *db.Queries) error {
		g, err := getGoalFor(ctx, q, p.CanSeeGoal, goalID)
		if err != nil {
			return err
		}
		switch st := domain.GoalStatus(g.Status); {
		case st.Terminal():
			return domain.GoalClosed(g.ID)
		case st != domain.GoalPlanning && st != domain.GoalRunning:
			return domain.Conflict(domain.CodeInvalidTransition, "L'objectif attend une approbation : pas de nouvelle tâche")
		}
		if len(in.DependsOn) > 0 {
			deps, err := q.ListTasksByIDs(ctx, in.DependsOn)
			if err != nil {
				return err
			}
			if len(deps) != len(in.DependsOn) {
				return domain.Validation(map[string]string{"depends_on": "tâche inconnue"})
			}
			for _, d := range deps {
				if d.GoalID != goalID {
					return domain.Validation(map[string]string{"depends_on": fmt.Sprintf("la tâche %d appartient à un autre objectif", d.ID)})
				}
			}
		}
		task, err = q.InsertTask(ctx, db.InsertTaskParams{GoalID: goalID, Role: in.Role, Title: in.Title, Spec: in.Spec})
		if err != nil {
			return err
		}
		for _, d := range in.DependsOn {
			if err := q.InsertTaskDep(ctx, db.InsertTaskDepParams{TaskID: task.ID, DependsOn: d}); err != nil {
				return err
			}
		}
		_, err = events.Emit(ctx, q, &goalID, events.TaskCreated, p.Role, events.TaskCreatedPayload{Task: taskToAPI(task, in.DependsOn)})
		return err
	})
	if err != nil {
		return apitypes.Task{}, err
	}
	return taskToAPI(task, in.DependsOn), nil
}

// GetTask renvoie une tâche avec son run en cours (GET /tasks/{id}).
func (s *Service) GetTask(ctx context.Context, p auth.Principal, id int64) (apitypes.TaskDetail, error) {
	q := s.store.Q
	t, err := s.getTaskFor(ctx, q, p, id)
	if err != nil {
		return apitypes.TaskDetail{}, err
	}
	deps, err := q.ListDepsByTask(ctx, id)
	if err != nil {
		return apitypes.TaskDetail{}, err
	}
	d := apitypes.TaskDetail{Task: taskToAPI(t, deps)}
	run, err := q.GetActiveTaskRun(ctx, &id)
	switch {
	case err == nil:
		d.CurrentRun = ptr(runToAPI(run))
	case !store.IsNotFound(err):
		return d, err
	}
	return d, nil
}

// SetTaskStatus change le statut d'une tâche (POST /tasks/{id}/status).
// L'agent du run task en cours rend la main (review) ou abandonne (failed) ;
// le runner conclut sa vérification (done ou failed).
func (s *Service) SetTaskStatus(ctx context.Context, p auth.Principal, id int64, in apitypes.TaskStatusChange) (apitypes.Task, error) {
	var actor domain.Actor
	switch {
	case p.IsRunner():
		actor = domain.ActorRunner
	case p.IsAgent() && p.RunKind == domain.RunTask && p.TaskID != nil && *p.TaskID == id:
		actor = domain.ActorTaskAgent
	default:
		return apitypes.Task{}, domain.Forbidden("Seul l'agent de la tâche ou le runner peut changer son statut")
	}

	to := domain.TaskStatus(in.Status)
	f := domain.Fields{}
	switch to {
	case domain.TaskReview:
		f.Required("summary", in.Summary != nil)
		if in.Summary != nil {
			f.Len("summary", *in.Summary, 1, 2000)
		}
	case domain.TaskFailed:
		f.Required("reason", in.Reason != nil)
		if in.Reason != nil {
			f.Len("reason", *in.Reason, 1, 10000)
		}
	case domain.TaskDone:
	default:
		f.Add("status", "review, done ou failed")
	}
	if err := f.Err(); err != nil {
		return apitypes.Task{}, err
	}

	var task db.Task
	err := s.store.Write(ctx, func(q *db.Queries) error {
		t, err := s.getTaskFor(ctx, q, p, id)
		if err != nil {
			return err
		}
		g, err := q.GetGoal(ctx, t.GoalID)
		if err != nil {
			return err
		}
		if domain.GoalStatus(g.Status).Terminal() {
			return domain.GoalClosed(g.ID)
		}
		c := taskChange{reason: in.Reason}
		if to == domain.TaskReview {
			c.summary = in.Summary
		}
		task, err = s.changeTaskStatus(ctx, q, t, to, actor, p.Role, c)
		return err
	})
	if err != nil {
		return apitypes.Task{}, err
	}
	deps, err := s.store.Q.ListDepsByTask(ctx, id)
	return taskToAPI(task, deps), err
}

func (s *Service) getTaskFor(ctx context.Context, q *db.Queries, p auth.Principal, id int64) (db.Task, error) {
	t, err := q.GetTask(ctx, id)
	if store.IsNotFound(err) || (err == nil && !p.CanSeeGoal(t.GoalID)) {
		return t, domain.NotFound("Tâche")
	}
	return t, err
}
