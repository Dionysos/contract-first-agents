package runner

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/events"
)

// builder fabrique des événements comme l'API le ferait.
type builder struct {
	v  *View
	id int64
}

func (b *builder) emit(goal int64, typ string, payload any) {
	b.id++
	raw, _ := json.Marshal(payload)
	b.v.Apply(apitypes.Event{ID: b.id, GoalID: &goal, Type: typ, Actor: "test", CreatedAt: time.Now(), Payload: raw})
}

func (b *builder) goal(id int64, status string) {
	b.emit(id, events.GoalCreated, events.GoalCreatedPayload{Goal: apitypes.Goal{ID: id, Status: "planning", Branch: "goal/1", BaseBranch: "main"}})
	if status != "planning" {
		b.emit(id, events.GoalStatusChanged, events.GoalStatusChangedPayload{GoalID: id, From: "planning", To: status})
	}
}

func (b *builder) task(goal, id int64, role, status string, deps ...int64) {
	if deps == nil {
		deps = []int64{}
	}
	b.emit(goal, events.TaskCreated, events.TaskCreatedPayload{Task: apitypes.Task{ID: id, GoalID: goal, Role: role, Status: "todo", DependsOn: deps}})
	if status != "todo" {
		b.emit(goal, events.TaskStatusChanged, events.TaskStatusChangedPayload{TaskID: id, Role: role, From: "todo", To: status})
	}
}

func (b *builder) run(goal, id int64, task *int64, kind, role, status string) {
	b.emit(goal, events.RunStarted, events.RunStartedPayload{Run: apitypes.Run{ID: id, GoalID: goal, TaskID: task, Kind: kind, Role: role, Status: "running"}})
	if status != "running" {
		b.emit(goal, events.RunFinished, events.RunFinishedPayload{Run: apitypes.Run{ID: id, GoalID: goal, TaskID: task, Kind: kind, Role: role, Status: status}})
	}
}

func keys(actions []Action) map[string]bool {
	out := map[string]bool{}
	for _, a := range actions {
		out[a.Key()] = true
	}
	return out
}

func i64(v int64) *int64 { return &v }

func TestDecidePlanning(t *testing.T) {
	cfg := config.Default()
	b := &builder{v: NewView()}
	b.goal(1, "planning")
	if k := keys(Decide(b.v, cfg)); !k["run:plan:1:planner"] {
		t.Fatalf("plan attendu : %v", k)
	}
	b.run(1, 1, nil, "plan", "planner", "running")
	if k := keys(Decide(b.v, cfg)); len(k) != 0 {
		t.Fatalf("rien pendant le plan : %v", k)
	}
	b.task(1, 1, "contracts", "todo")
	b.emit(1, events.RunFinished, events.RunFinishedPayload{Run: apitypes.Run{ID: 1, GoalID: 1, Kind: "plan", Role: "planner", Status: "succeeded"}})
	if k := keys(Decide(b.v, cfg)); !k["goal-running:1"] {
		t.Fatalf("démarrage attendu : %v", k)
	}
}

func TestDecideEmptyPlanFails(t *testing.T) {
	b := &builder{v: NewView()}
	b.goal(1, "planning")
	b.run(1, 1, nil, "plan", "planner", "succeeded")
	if k := keys(Decide(b.v, config.Default())); !k["goal-fail:1"] {
		t.Fatalf("échec attendu : %v", k)
	}
}

func TestDecideTasks(t *testing.T) {
	cfg := config.Default()
	b := &builder{v: NewView()}
	b.goal(1, "running")
	b.task(1, 1, "contracts", "done")
	b.task(1, 2, "back", "todo", 1)
	b.task(1, 3, "front", "todo", 2) // dépend d'une tâche non terminée
	b.task(1, 4, "back", "review")   // vérifiée par le runner à la fin de son run
	k := keys(Decide(b.v, cfg))
	if !k["run:task:2"] || k["run:task:3"] || len(k) != 1 {
		t.Fatalf("actions : %v", k)
	}
	// Une tâche avec un run actif n'est pas relancée.
	b.run(1, 10, i64(2), "task", "back", "running")
	if k := keys(Decide(b.v, cfg)); k["run:task:2"] {
		t.Fatalf("relance d'une tâche occupée : %v", k)
	}
}

func TestDecideFailedTaskFailsGoal(t *testing.T) {
	b := &builder{v: NewView()}
	b.goal(1, "running")
	b.task(1, 1, "contracts", "done")
	b.task(1, 2, "back", "failed", 1)
	b.task(1, 3, "front", "todo", 1)
	// Pas de nouvelle tentative en phase 1 : l'objectif échoue, rien d'autre ne démarre.
	k := keys(Decide(b.v, config.Default()))
	if !k["goal-fail:1"] || len(k) != 1 {
		t.Fatalf("échec de l'objectif attendu : %v", k)
	}
}

func TestDecideMerge(t *testing.T) {
	cfg := config.Default()
	b := &builder{v: NewView()}
	b.goal(1, "running")
	b.task(1, 1, "back", "done")
	if k := keys(Decide(b.v, cfg)); !k["prepare-merge:1"] {
		t.Fatalf("fusion attendue : %v", k)
	}
	b.emit(1, events.ApprovalRequested, events.ApprovalRequestedPayload{Approval: apitypes.Approval{ID: 1, GoalID: 1, Kind: "merge", Status: "pending"}})
	b.emit(1, events.GoalStatusChanged, events.GoalStatusChangedPayload{GoalID: 1, From: "running", To: "awaiting_approval"})
	if k := keys(Decide(b.v, cfg)); len(k) != 0 {
		t.Fatalf("rien avant la décision : %v", k)
	}
	b.emit(1, events.ApprovalResolved, events.ApprovalResolvedPayload{ApprovalID: 1, Kind: "merge", Decision: "approve"})
	if k := keys(Decide(b.v, cfg)); !k["merge:1"] {
		t.Fatalf("merge attendu : %v", k)
	}
	b.emit(1, events.MergeDone, events.MergeDonePayload{Branch: "goal/1", BaseBranch: "main", Commit: "abc"})
	if k := keys(Decide(b.v, cfg)); len(k) != 0 {
		t.Fatalf("rien après le merge : %v", k)
	}
}

func TestOutsidePerimeter(t *testing.T) {
	got := OutsidePerimeter([]string{"api/a.go", "db/m.sql", "web/x.tsx", "contracts/o.yaml", "README.md"}, []string{"api/**", "db/**", "*.md"})
	if len(got) != 2 || got[0] != "web/x.tsx" || got[1] != "contracts/o.yaml" {
		t.Fatalf("hors périmètre : %v", got)
	}
}

func TestDecideKillOnCancel(t *testing.T) {
	b := &builder{v: NewView()}
	b.goal(1, "running")
	b.task(1, 1, "back", "running")
	b.run(1, 1, i64(1), "task", "back", "running")
	b.emit(1, events.GoalStatusChanged, events.GoalStatusChangedPayload{GoalID: 1, From: "running", To: "cancelled"})
	if k := keys(Decide(b.v, config.Default())); !k["kill:1"] {
		t.Fatalf("arrêt attendu : %v", k)
	}
}
