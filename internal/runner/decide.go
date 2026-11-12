package runner

import (
	"fmt"
	"sort"

	"github.com/Dionysos/contract-first-agents/internal/config"
)

// Action est une chose à faire, déduite de la vue. Key identifie l'action pour ne pas
// la lancer deux fois pendant qu'elle s'exécute.
type Action interface {
	Key() string
}

// StartRun lance un agent (plan ou task).
type StartRun struct {
	GoalID int64
	TaskID *int64
	Kind   string
	Role   string
}

func (a StartRun) Key() string {
	if a.TaskID != nil {
		return fmt.Sprintf("run:%s:%d", a.Kind, *a.TaskID)
	}
	return fmt.Sprintf("run:%s:%d:%s", a.Kind, a.GoalID, a.Role)
}

// SetGoalRunning démarre l'objectif une fois le plan fait.
type SetGoalRunning struct{ GoalID int64 }

func (a SetGoalRunning) Key() string { return fmt.Sprintf("goal-running:%d", a.GoalID) }

// FailGoal met l'objectif en échec (plan vide, tâche en échec).
type FailGoal struct {
	GoalID int64
	Reason string
}

func (a FailGoal) Key() string { return fmt.Sprintf("goal-fail:%d", a.GoalID) }

// PrepareMerge fusionne les branches des tâches dans goal/<id> et demande l'approbation de merge.
type PrepareMerge struct{ GoalID int64 }

func (a PrepareMerge) Key() string { return fmt.Sprintf("prepare-merge:%d", a.GoalID) }

// MergeGoal merge goal/<id> dans la branche de base après approbation.
type MergeGoal struct{ GoalID int64 }

func (a MergeGoal) Key() string { return fmt.Sprintf("merge:%d", a.GoalID) }

// KillRuns arrête les runs d'un objectif terminé.
type KillRuns struct{ GoalID int64 }

func (a KillRuns) Key() string { return fmt.Sprintf("kill:%d", a.GoalID) }

// Decide déduit les actions à mener de la vue. Fonction pure : ni réseau, ni git, ni horloge.
//
// Phase 1 : pas de nouvelle tentative. Une tâche en échec fait échouer l'objectif.
func Decide(v *View, cfg *config.Config) []Action {
	var out []Action
	ids := make([]int64, 0, len(v.Goals))
	for id := range v.Goals {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		g := v.Goals[id]
		tasks := v.goalTasks(id)
		sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
		active := v.ActiveRuns(id)

		switch g.Status {
		case "done", "failed", "cancelled":
			if len(active) > 0 {
				out = append(out, KillRuns{id})
			}
			continue

		case "planning":
			switch {
			case len(active) > 0:
			case !v.planFinished(id):
				out = append(out, StartRun{GoalID: id, Kind: "plan", Role: "planner"})
			case len(tasks) > 0:
				out = append(out, SetGoalRunning{id})
			default:
				out = append(out, FailGoal{id, "le planner n'a créé aucune tâche"})
			}
			continue

		case "awaiting_approval":
			if a := v.latestApproval(id, "merge"); a != nil && a.Status == "approved" && !g.Merged {
				out = append(out, MergeGoal{id})
			}
			continue

		case "running":
		default:
			continue
		}

		// Objectif en cours.
		if failed := firstFailed(tasks); failed != nil {
			out = append(out, FailGoal{id, fmt.Sprintf("la tâche %d (%s) a échoué", failed.ID, failed.Role)})
			continue
		}
		allDone := len(tasks) > 0
		for _, t := range tasks {
			if t.Status != "done" {
				allDone = false
			}
			if t.Status == "todo" && !v.taskBusy(t.ID) && v.depsDone(t) {
				tid := t.ID
				out = append(out, StartRun{GoalID: id, TaskID: &tid, Kind: "task", Role: t.Role})
			}
		}
		if allDone && !v.anyPending(id) {
			out = append(out, PrepareMerge{id})
		}
	}
	return out
}

func firstFailed(tasks []*Task) *Task {
	for _, t := range tasks {
		if t.Status == "failed" {
			return t
		}
	}
	return nil
}

func (v *View) latestApproval(goalID int64, kind string) *Approval {
	var best *Approval
	for _, a := range v.Approvals {
		if a.GoalID == goalID && a.Kind == kind && (best == nil || a.ID > best.ID) {
			best = a
		}
	}
	return best
}

func (v *View) anyPending(goalID int64) bool {
	for _, a := range v.Approvals {
		if a.GoalID == goalID && a.Status == "pending" {
			return true
		}
	}
	return false
}
