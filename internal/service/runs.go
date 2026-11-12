package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/events"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
)

// MaxOutputBytes limite la sortie d'un contrôle déclarée par le runner.
const MaxOutputBytes = 64 << 10

// TokenGrace prolonge la validité d'un token d'agent au-delà du timeout du run.
const TokenGrace = 5 * time.Minute

// CreateRun déclare un run et émet son token d'agent (POST /runs).
func (s *Service) CreateRun(ctx context.Context, p auth.Principal, in apitypes.RunCreate) (apitypes.RunCreated, error) {
	if !p.IsRunner() {
		return apitypes.RunCreated{}, domain.Forbidden("")
	}
	f := domain.Fields{}
	switch in.Kind {
	case domain.RunPlan:
		if in.Role != "planner" {
			f.Add("role", "un run plan est fait par le planner")
		}
	case domain.RunTask:
		f.Required("task_id", in.TaskID != nil)
	default:
		f.Add("kind", "task ou plan")
	}
	if !s.cfg.IsAgentRole(in.Role) {
		f.Add("role", "rôle d'agent inconnu")
	}
	f.Len("model", in.Model, 1, 200)
	if err := f.Err(); err != nil {
		return apitypes.RunCreated{}, err
	}

	var out apitypes.RunCreated
	err := s.store.Write(ctx, func(q *db.Queries) error {
		g, err := getGoalFor(ctx, q, p.CanSeeGoal, in.GoalID)
		if err != nil {
			return err
		}
		if domain.GoalStatus(g.Status).Terminal() {
			return domain.GoalClosed(g.ID)
		}

		var task *db.Task
		if in.TaskID != nil {
			t, err := q.GetTask(ctx, *in.TaskID)
			if store.IsNotFound(err) || (err == nil && t.GoalID != g.ID) {
				return domain.Validation(map[string]string{"task_id": "tâche de cet objectif"})
			}
			if err != nil {
				return err
			}
			task = &t
		}

		switch in.Kind {
		case domain.RunPlan:
			if g.Status != string(domain.GoalPlanning) {
				return domain.Conflict(domain.CodeInvalidTransition, "Un run plan exige un objectif en planning")
			}
		case domain.RunTask:
			if in.Role != task.Role {
				return domain.Validation(map[string]string{"role": "doit être le rôle de la tâche (" + task.Role + ")"})
			}
			if task.Status != string(domain.TaskTodo) {
				return domain.InvalidTransition(task.Status, string(domain.TaskRunning))
			}
			deps, err := q.ListDepsByTask(ctx, task.ID)
			if err != nil {
				return err
			}
			if len(deps) > 0 {
				dts, err := q.ListTasksByIDs(ctx, deps)
				if err != nil {
					return err
				}
				for _, d := range dts {
					if d.Status != string(domain.TaskDone) {
						return domain.Conflict(domain.CodeInvalidTransition, fmt.Sprintf("Dépendance %d non terminée", d.ID))
					}
				}
			}
			if _, err := s.changeTaskStatus(ctx, q, *task, domain.TaskRunning, domain.ActorRunner, p.Role, taskChange{}); err != nil {
				return err
			}
		}

		run, err := q.InsertRun(ctx, db.InsertRunParams{GoalID: g.ID, TaskID: in.TaskID, Kind: in.Kind, Role: in.Role, Model: in.Model})
		if err != nil {
			return err
		}
		plain, hash := auth.NewToken()
		expires := s.now().Add(s.cfg.RunTimeout + TokenGrace)
		if err := q.InsertToken(ctx, db.InsertTokenParams{Hash: hash, Role: in.Role, RunID: &run.ID, Label: fmt.Sprintf("run %d", run.ID), ExpiresAt: &expires}); err != nil {
			return err
		}
		var title *string
		if task != nil {
			title = &task.Title
		}
		if _, err := events.Emit(ctx, q, &g.ID, events.RunStarted, p.Role, events.RunStartedPayload{Run: runToAPI(run), TaskTitle: title}); err != nil {
			return err
		}
		out = apitypes.RunCreated{Run: runToAPI(run), Token: plain, ExpiresAt: expires}
		return nil
	})
	if err != nil {
		return apitypes.RunCreated{}, err
	}
	return out, nil
}

// FinishRun enregistre la fin d'un run et révoque son token (POST /runs/{id}/finish).
func (s *Service) FinishRun(ctx context.Context, p auth.Principal, id int64, in apitypes.RunFinish) (apitypes.Run, error) {
	if !p.IsRunner() {
		return apitypes.Run{}, domain.Forbidden("")
	}
	f := domain.Fields{}
	switch in.Status {
	case "succeeded", "failed", "killed":
	default:
		f.Add("status", "succeeded, failed ou killed")
	}
	if err := f.Err(); err != nil {
		return apitypes.Run{}, err
	}
	if in.Files == nil {
		in.Files = []string{}
	}

	var run db.Run
	err := s.store.Write(ctx, func(q *db.Queries) error {
		r, err := q.GetRun(ctx, id)
		if store.IsNotFound(err) {
			return domain.NotFound("Run")
		}
		if err != nil {
			return err
		}
		if r.Status != "running" {
			return domain.InvalidTransition(r.Status, in.Status)
		}
		run, err = q.FinishRun(ctx, db.FinishRunParams{ID: id, Status: in.Status, Error: in.Error})
		if err != nil {
			return err
		}
		if err := q.RevokeRunTokens(ctx, &run.ID); err != nil {
			return err
		}
		_, err = events.Emit(ctx, q, &run.GoalID, events.RunFinished, p.Role, events.RunFinishedPayload{
			Run: runToAPI(run), DurationMS: run.FinishedAt.Sub(run.StartedAt).Milliseconds(), Error: in.Error,
			Diffstat: in.Diffstat, Files: in.Files,
		})
		return err
	})
	return runToAPI(run), err
}

// ReportCheck publie le résultat d'un contrôle (POST /checks).
func (s *Service) ReportCheck(ctx context.Context, p auth.Principal, in apitypes.CheckReport) (int64, error) {
	if !p.IsRunner() {
		return 0, domain.Forbidden("")
	}
	f := domain.Fields{}
	f.Len("name", in.Name, 1, 100)
	switch in.Status {
	case "passed", "failed", "error":
	default:
		f.Add("status", "passed, failed ou error")
	}
	if in.Output != nil && len(*in.Output) > MaxOutputBytes {
		f.Add("output", "64 Kio maximum")
	}
	if err := f.Err(); err != nil {
		return 0, err
	}
	var evID int64
	err := s.store.Write(ctx, func(q *db.Queries) error {
		if _, err := getGoalFor(ctx, q, p.CanSeeGoal, in.GoalID); err != nil {
			return err
		}
		if in.TaskID != nil {
			t, err := q.GetTask(ctx, *in.TaskID)
			if store.IsNotFound(err) || (err == nil && t.GoalID != in.GoalID) {
				return domain.Validation(map[string]string{"task_id": "tâche de cet objectif"})
			}
			if err != nil {
				return err
			}
		}
		if in.RunID != nil {
			r, err := q.GetRun(ctx, *in.RunID)
			if store.IsNotFound(err) || (err == nil && r.GoalID != in.GoalID) {
				return domain.Validation(map[string]string{"run_id": "run de cet objectif"})
			}
			if err != nil {
				return err
			}
		}
		ev, err := events.Emit(ctx, q, &in.GoalID, events.CheckFinished, p.Role, events.CheckFinishedPayload{
			TaskID: in.TaskID, RunID: in.RunID, Name: in.Name, Command: in.Command, Status: in.Status, DurationMS: in.DurationMS, Output: in.Output,
		})
		evID = ev.ID
		return err
	})
	return evID, err
}

// MarkGoalMerged enregistre le merge après approbation (POST /goals/{id}/merged).
func (s *Service) MarkGoalMerged(ctx context.Context, p auth.Principal, id int64, in apitypes.GoalMerged) (apitypes.Goal, error) {
	if !p.IsRunner() {
		return apitypes.Goal{}, domain.Forbidden("")
	}
	f := domain.Fields{}
	f.Len("commit", in.Commit, 1, 100)
	if err := f.Err(); err != nil {
		return apitypes.Goal{}, err
	}
	var goal db.Goal
	err := s.store.Write(ctx, func(q *db.Queries) error {
		g, err := getGoalFor(ctx, q, p.CanSeeGoal, id)
		if err != nil {
			return err
		}
		if _, err := q.GetApprovedMerge(ctx, id); store.IsNotFound(err) {
			return domain.Conflict(domain.CodeInvalidTransition, "Aucune approbation de merge acceptée")
		} else if err != nil {
			return err
		}
		if goal, err = s.changeGoalStatus(ctx, q, g, domain.GoalDone, domain.ActorRunner, p.Role, nil); err != nil {
			return err
		}
		_, err = events.Emit(ctx, q, &id, events.MergeDone, p.Role, events.MergeDonePayload{
			Branch: g.Branch, BaseBranch: g.BaseBranch, Commit: in.Commit,
		})
		return err
	})
	return goalToAPI(goal), err
}
