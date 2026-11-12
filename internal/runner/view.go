// Package runner orchestre les agents : il reconstruit une vue à partir des événements,
// en déduit des actions (règles fixes, sans décision de fond) et les exécute (git, agents).
package runner

import (
	"encoding/json"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/events"
)

// View est l'état minimal dont le runner a besoin, reconstruit uniquement à partir
// du flux d'événements (comme le dashboard).
type View struct {
	Goals       map[int64]*Goal
	Tasks       map[int64]*Task
	Runs        map[int64]*Run
	Approvals   map[int64]*Approval
	LastEventID int64
}

// Goal est la vue d'un objectif.
type Goal struct {
	ID         int64
	Title      string
	Status     string
	Branch     string
	BaseBranch string
	Merged     bool
}

// Task est la vue d'une tâche.
type Task struct {
	ID        int64
	GoalID    int64
	Role      string
	Title     string
	Spec      string
	Status    string
	Branch    string
	DependsOn []int64
}

// Run est la vue d'un run.
type Run struct {
	ID     int64
	GoalID int64
	TaskID *int64
	Kind   string
	Role   string
	Status string
}

// Approval est la vue d'une approbation.
type Approval struct {
	ID     int64
	GoalID int64
	Kind   string
	Status string
}

// NewView crée une vue vide.
func NewView() *View {
	return &View{
		Goals: map[int64]*Goal{}, Tasks: map[int64]*Task{}, Runs: map[int64]*Run{},
		Approvals: map[int64]*Approval{},
	}
}

// Apply met la vue à jour avec un événement. Les événements inconnus sont ignorés.
func (v *View) Apply(ev apitypes.Event) {
	if ev.ID <= v.LastEventID {
		return
	}
	v.LastEventID = ev.ID
	switch ev.Type {
	case events.GoalCreated:
		var p events.GoalCreatedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			g := p.Goal
			v.Goals[g.ID] = &Goal{ID: g.ID, Title: g.Title, Status: g.Status, Branch: g.Branch, BaseBranch: g.BaseBranch}
		}
	case events.GoalStatusChanged:
		var p events.GoalStatusChangedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			if g := v.Goals[p.GoalID]; g != nil {
				g.Status = p.To
			}
		}
	case events.MergeDone:
		if ev.GoalID != nil {
			if g := v.Goals[*ev.GoalID]; g != nil {
				g.Merged = true
			}
		}
	case events.TaskCreated:
		var p events.TaskCreatedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			t := p.Task
			v.Tasks[t.ID] = &Task{ID: t.ID, GoalID: t.GoalID, Role: t.Role, Title: t.Title, Spec: t.Spec, Status: t.Status,
				Branch: t.Branch, DependsOn: t.DependsOn}
		}
	case events.TaskStatusChanged:
		var p events.TaskStatusChangedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			if t := v.Tasks[p.TaskID]; t != nil {
				t.Status = p.To
			}
		}
	case events.RunStarted:
		var p events.RunStartedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			r := p.Run
			v.Runs[r.ID] = &Run{ID: r.ID, GoalID: r.GoalID, TaskID: r.TaskID, Kind: r.Kind, Role: r.Role, Status: r.Status}
		}
	case events.RunFinished:
		var p events.RunFinishedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			if r := v.Runs[p.Run.ID]; r != nil {
				r.Status = p.Run.Status
			}
		}
	case events.ApprovalRequested:
		var p events.ApprovalRequestedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			a := p.Approval
			v.Approvals[a.ID] = &Approval{ID: a.ID, GoalID: a.GoalID, Kind: a.Kind, Status: a.Status}
		}
	case events.ApprovalResolved:
		var p events.ApprovalResolvedPayload
		if json.Unmarshal(ev.Payload, &p) == nil {
			if a := v.Approvals[p.ApprovalID]; a != nil {
				a.Status = map[string]string{"approve": "approved", "reject": "rejected"}[p.Decision]
			}
		}
	}
}

// ---- Requêtes sur la vue ----

// ActiveRuns renvoie les runs en cours d'un objectif.
func (v *View) ActiveRuns(goalID int64) []*Run {
	var out []*Run
	for _, r := range v.Runs {
		if r.GoalID == goalID && r.Status == "running" {
			out = append(out, r)
		}
	}
	return out
}

func (v *View) taskBusy(taskID int64) bool {
	for _, r := range v.Runs {
		if r.TaskID != nil && *r.TaskID == taskID && r.Status == "running" {
			return true
		}
	}
	return false
}

func (v *View) goalTasks(goalID int64) []*Task {
	var out []*Task
	for _, t := range v.Tasks {
		if t.GoalID == goalID {
			out = append(out, t)
		}
	}
	return out
}

func (v *View) depsDone(t *Task) bool {
	for _, d := range t.DependsOn {
		if dt := v.Tasks[d]; dt == nil || dt.Status != "done" {
			return false
		}
	}
	return true
}

func (v *View) planFinished(goalID int64) bool {
	for _, r := range v.Runs {
		if r.GoalID == goalID && r.Kind == "plan" && r.Status != "running" {
			return true
		}
	}
	return false
}
