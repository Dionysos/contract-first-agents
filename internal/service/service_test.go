package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/config"
	"github.com/Dionysos/contract-first-agents/internal/domain"
	"github.com/Dionysos/contract-first-agents/internal/store"
	"github.com/Dionysos/contract-first-agents/internal/store/db"
	"github.com/Dionysos/contract-first-agents/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

var (
	human  = auth.Principal{Role: domain.RoleHuman}
	runner = auth.Principal{Role: domain.RoleRunner}
)

func setup(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return New(st, config.Default()), st
}

// agent simule le token d'un run en insérant le run en base.
func agent(t *testing.T, st *store.Store, goalID int64, taskID *int64, kind, role string) auth.Principal {
	t.Helper()
	r, err := st.Q.InsertRun(context.Background(), db.InsertRunParams{GoalID: goalID, TaskID: taskID, Kind: kind, Role: role, Model: "fake/model"})
	if err != nil {
		t.Fatal(err)
	}
	return auth.Principal{Role: role, RunID: &r.ID, GoalID: &goalID, TaskID: taskID, RunKind: kind}
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != code {
		t.Fatalf("erreur %v, attendu le code %s", err, code)
	}
}

func newGoal(t *testing.T, s *Service) apitypes.Goal {
	t.Helper()
	g, err := s.CreateGoal(context.Background(), human, apitypes.GoalCreate{
		Title:       "Pagination des commandes",
		Description: "La liste doit être paginée, 20 par page. Tests attendus.",
	})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCreateGoal(t *testing.T) {
	s, st := setup(t)
	ctx := context.Background()

	g := newGoal(t, s)
	if g.Status != "planning" || g.Branch == "" || g.BaseBranch != "main" {
		t.Fatalf("objectif inattendu : %+v", g)
	}

	// Validation par champ.
	_, err := s.CreateGoal(ctx, human, apitypes.GoalCreate{Title: "abc", Description: "court", BaseBranch: ptr("pas de blanc")})
	var de *domain.Error
	if !errors.As(err, &de) || len(de.Fields) != 3 {
		t.Fatalf("validation : %v", err)
	}

	// Seul l'humain crée un objectif.
	_, err = s.CreateGoal(ctx, runner, apitypes.GoalCreate{})
	wantCode(t, err, domain.CodeForbidden)

	// Un événement goal.created par objectif créé.
	evs, err := st.Q.ListEventsAfter(ctx, db.ListEventsAfterParams{After: 0, MaxRows: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Type != "goal.created" {
		t.Fatalf("événements : %d", len(evs))
	}
}

func TestListGoalsPagination(t *testing.T) {
	s, _ := setup(t)
	for range 3 {
		newGoal(t, s)
	}
	page, err := s.ListGoals(context.Background(), human, "", 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextBefore == nil {
		t.Fatalf("page 1 : %+v", page)
	}
	page2, err := s.ListGoals(context.Background(), human, "", 2, page.NextBefore)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.NextBefore != nil || page2.Items[0].ID >= page.Items[1].ID {
		t.Fatalf("page 2 : %+v", page2)
	}
}

func TestTaskLifecycle(t *testing.T) {
	s, st := setup(t)
	ctx := context.Background()
	g := newGoal(t, s)

	planner := agent(t, st, g.ID, nil, domain.RunPlan, "planner")
	contract, err := s.CreateTask(ctx, planner, apitypes.TaskCreate{Role: "contracts", Title: "Contrat paginé", Spec: "Ajouter page et per_page."})
	if err != nil {
		t.Fatal(err)
	}
	back, err := s.CreateTask(ctx, planner, apitypes.TaskCreate{Role: "back", Title: "API paginée", Spec: "Implémenter.", DependsOn: []int64{contract.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if back.Branch != "task/1-2-back" || len(back.DependsOn) != 1 {
		t.Fatalf("tâche : %+v", back)
	}

	// Le planner ne crée pas de tâche planner, ni hors de son objectif.
	_, err = s.CreateTask(ctx, planner, apitypes.TaskCreate{Role: "planner", Title: "Replanifier", Spec: "x"})
	wantCode(t, err, domain.CodeValidationFailed)
	other := newGoal(t, s)
	_, err = s.CreateTask(ctx, planner, apitypes.TaskCreate{GoalID: &other.ID, Role: "back", Title: "Ailleurs", Spec: "x"})
	wantCode(t, err, domain.CodeValidationFailed)

	// Démarrage de l'objectif par le runner.
	if _, err := s.SetGoalStatus(ctx, runner, g.ID, apitypes.GoalStatusChange{Status: "running"}); err != nil {
		t.Fatal(err)
	}

	// Run task : la tâche passe en running (POST /runs, voir runs_test.go ; ici à la main).
	worker := agent(t, st, g.ID, &contract.ID, domain.RunTask, "contracts")
	ct, _ := st.Q.GetTask(ctx, contract.ID)
	if _, err := s.changeTaskStatus(ctx, st.Q, ct, domain.TaskRunning, domain.ActorRunner, "runner", taskChange{}); err != nil {
		t.Fatal(err)
	}

	// Un autre agent ne peut pas changer le statut.
	_, err = s.SetTaskStatus(ctx, planner, contract.ID, apitypes.TaskStatusChange{Status: "review", Summary: ptr("fini")})
	wantCode(t, err, domain.CodeForbidden)
	// review exige un résumé ; l'agent ne peut pas se déclarer done lui-même.
	_, err = s.SetTaskStatus(ctx, worker, contract.ID, apitypes.TaskStatusChange{Status: "review"})
	wantCode(t, err, domain.CodeValidationFailed)
	_, err = s.SetTaskStatus(ctx, worker, contract.ID, apitypes.TaskStatusChange{Status: "done"})
	wantCode(t, err, domain.CodeInvalidTransition)

	rv, err := s.SetTaskStatus(ctx, worker, contract.ID, apitypes.TaskStatusChange{Status: "review", Summary: ptr("Contrat publié")})
	if err != nil || rv.Status != "review" || *rv.Summary != "Contrat publié" {
		t.Fatalf("review : %+v %v", rv, err)
	}

	// Vérification par le runner.
	done, err := s.SetTaskStatus(ctx, runner, contract.ID, apitypes.TaskStatusChange{Status: "done"})
	if err != nil || done.Status != "done" {
		t.Fatalf("done : %+v %v", done, err)
	}

	// Un agent ne voit pas les objectifs des autres.
	_, err = s.GetGoal(ctx, worker, other.ID)
	wantCode(t, err, domain.CodeNotFound)

	full, err := s.GetGoal(ctx, human, g.ID)
	if err != nil || len(full.Tasks) != 2 || len(full.Runs) != 2 {
		t.Fatalf("détail objectif : %d tâches, %d runs, %v", len(full.Tasks), len(full.Runs), err)
	}
}

func TestCancelGoal(t *testing.T) {
	s, st := setup(t)
	ctx := context.Background()
	g := newGoal(t, s)
	planner := agent(t, st, g.ID, nil, domain.RunPlan, "planner")
	task, err := s.CreateTask(ctx, planner, apitypes.TaskCreate{Role: "back", Title: "API paginée", Spec: "x"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.CancelGoal(ctx, runner, g.ID, "")
	wantCode(t, err, domain.CodeForbidden)

	c, err := s.CancelGoal(ctx, human, g.ID, "plus utile")
	if err != nil || c.Status != "cancelled" || c.FinishedAt == nil {
		t.Fatalf("annulation : %+v %v", c, err)
	}
	tk, _ := st.Q.GetTask(ctx, task.ID)
	if tk.Status != "failed" {
		t.Fatalf("tâche : %s", tk.Status)
	}
	// Idempotent, puis objectif fermé aux écritures.
	if _, err := s.CancelGoal(ctx, human, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateTask(ctx, human, apitypes.TaskCreate{GoalID: &g.ID, Role: "back", Title: "Trop tard", Spec: "x"})
	wantCode(t, err, domain.CodeGoalClosed)
}

func TestGoalCannotStartWithoutTasks(t *testing.T) {
	s, _ := setup(t)
	g := newGoal(t, s)
	_, err := s.SetGoalStatus(context.Background(), runner, g.ID, apitypes.GoalStatusChange{Status: "running"})
	wantCode(t, err, domain.CodeInvalidTransition)
}
