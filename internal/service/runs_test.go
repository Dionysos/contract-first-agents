package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Dionysos/contract-first-agents/internal/apitypes"
	"github.com/Dionysos/contract-first-agents/internal/auth"
	"github.com/Dionysos/contract-first-agents/internal/domain"
)

// runAs déclare un run par l'API (comme le runner) et renvoie le principal de son token.
func runAs(t *testing.T, s *Service, goalID int64, taskID *int64, kind, role string) (auth.Principal, apitypes.RunCreated) {
	t.Helper()
	rc, err := s.CreateRun(context.Background(), runner, apitypes.RunCreate{GoalID: goalID, TaskID: taskID, Kind: kind, Role: role, Model: "fake/model"})
	if err != nil {
		t.Fatalf("CreateRun %s/%s : %v", kind, role, err)
	}
	return auth.Principal{Role: role, RunID: &rc.Run.ID, GoalID: &goalID, TaskID: taskID, RunKind: kind}, rc
}

func finish(t *testing.T, s *Service, rc apitypes.RunCreated) {
	t.Helper()
	if _, err := s.FinishRun(context.Background(), runner, rc.Run.ID, apitypes.RunFinish{Status: "succeeded"}); err != nil {
		t.Fatalf("FinishRun : %v", err)
	}
}

func bearer(token string) *http.Request {
	r, _ := http.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

type plan struct {
	goal              apitypes.Goal
	contract, back, f apitypes.Task
}

// planned crée un objectif, le planifie (3 tâches) et le démarre.
func planned(t *testing.T, s *Service) plan {
	t.Helper()
	ctx := context.Background()
	g := newGoal(t, s)
	pl, rc := runAs(t, s, g.ID, nil, domain.RunPlan, "planner")
	c, err := s.CreateTask(ctx, pl, apitypes.TaskCreate{Role: "contracts", Title: "Contrat paginé", Spec: "x"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.CreateTask(ctx, pl, apitypes.TaskCreate{Role: "back", Title: "API paginée", Spec: "x", DependsOn: []int64{c.ID}})
	f, _ := s.CreateTask(ctx, pl, apitypes.TaskCreate{Role: "front", Title: "Liste paginée", Spec: "x", DependsOn: []int64{c.ID}})
	finish(t, s, rc)
	if _, err := s.SetGoalStatus(ctx, runner, g.ID, apitypes.GoalStatusChange{Status: "running"}); err != nil {
		t.Fatal(err)
	}
	return plan{g, c, b, f}
}

// completeTask fait passer une tâche de todo à done : run, cfa task done, vérification du runner.
func completeTask(t *testing.T, s *Service, task apitypes.Task) {
	t.Helper()
	ctx := context.Background()
	ag, rc := runAs(t, s, task.GoalID, &task.ID, domain.RunTask, task.Role)
	if _, err := s.SetTaskStatus(ctx, ag, task.ID, apitypes.TaskStatusChange{Status: "review", Summary: ptr("fait")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTaskStatus(ctx, runner, task.ID, apitypes.TaskStatusChange{Status: "done"}); err != nil {
		t.Fatal(err)
	}
	finish(t, s, rc)
}

func TestRunPreconditionsAndToken(t *testing.T) {
	s, st := setup(t)
	ctx := context.Background()
	p := planned(t, s)

	// Dépendance non terminée.
	_, err := s.CreateRun(ctx, runner, apitypes.RunCreate{GoalID: p.goal.ID, TaskID: &p.back.ID, Kind: "task", Role: "back", Model: "m"})
	wantCode(t, err, domain.CodeInvalidTransition)
	// Mauvais rôle.
	_, err = s.CreateRun(ctx, runner, apitypes.RunCreate{GoalID: p.goal.ID, TaskID: &p.contract.ID, Kind: "task", Role: "back", Model: "m"})
	wantCode(t, err, domain.CodeValidationFailed)
	// Type de run hors phase 1.
	_, err = s.CreateRun(ctx, runner, apitypes.RunCreate{GoalID: p.goal.ID, TaskID: &p.contract.ID, Kind: "review", Role: "contracts", Model: "m"})
	wantCode(t, err, domain.CodeValidationFailed)

	_, rc := runAs(t, s, p.goal.ID, &p.contract.ID, domain.RunTask, "contracts")
	// Le token renvoyé authentifie l'agent, limité à son run.
	a := auth.NewAuthenticator(st.Q)
	pr, err := a.FromRequest(ctx, bearer(rc.Token))
	if err != nil || pr.Role != "contracts" || *pr.TaskID != p.contract.ID || pr.RunKind != "task" {
		t.Fatalf("principal : %+v %v", pr, err)
	}
	task, _ := st.Q.GetTask(ctx, p.contract.ID)
	if task.Status != "running" {
		t.Fatalf("tâche : %s", task.Status)
	}
	// Une tâche déjà lancée ne repart pas (pas de nouvelle tentative en phase 1).
	_, err = s.CreateRun(ctx, runner, apitypes.RunCreate{GoalID: p.goal.ID, TaskID: &p.contract.ID, Kind: "task", Role: "contracts", Model: "m"})
	wantCode(t, err, domain.CodeInvalidTransition)
	// Fin du run : token révoqué.
	finish(t, s, rc)
	if _, err := a.FromRequest(ctx, bearer(rc.Token)); err == nil {
		t.Fatal("token encore valide après la fin du run")
	}
	// Finir deux fois est refusé.
	_, err = s.FinishRun(ctx, runner, rc.Run.ID, apitypes.RunFinish{Status: "succeeded"})
	wantCode(t, err, domain.CodeInvalidTransition)
}

func TestMergeApproval(t *testing.T) {
	s, st := setup(t)
	ctx := context.Background()
	p := planned(t, s)
	completeTask(t, s, p.contract)
	completeTask(t, s, p.back)
	completeTask(t, s, p.f)

	// Merge : demandé par le runner, refusé sans approbation, puis accepté.
	payload, _ := json.Marshal(apitypes.MergePayload{Branch: p.goal.Branch, BaseBranch: "main", Diffstat: apitypes.Diffstat{Files: 3}})
	_, err := s.CreateApproval(ctx, runner, apitypes.ApprovalCreate{GoalID: p.goal.ID, Kind: "budget", Payload: payload})
	wantCode(t, err, domain.CodeValidationFailed)
	a, err := s.CreateApproval(ctx, runner, apitypes.ApprovalCreate{GoalID: p.goal.ID, Kind: "merge", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateApproval(ctx, runner, apitypes.ApprovalCreate{GoalID: p.goal.ID, Kind: "merge", Payload: payload})
	wantCode(t, err, domain.CodeDuplicateApproval)
	if g, _ := st.Q.GetGoal(ctx, p.goal.ID); g.Status != "awaiting_approval" {
		t.Fatalf("objectif : %s", g.Status)
	}
	_, err = s.MarkGoalMerged(ctx, runner, p.goal.ID, apitypes.GoalMerged{Commit: "abc"})
	wantCode(t, err, domain.CodeInvalidTransition)
	_, err = s.DecideApproval(ctx, runner, a.ID, apitypes.ApprovalDecision{Decision: "approve"})
	wantCode(t, err, domain.CodeForbidden)
	if _, err := s.DecideApproval(ctx, human, a.ID, apitypes.ApprovalDecision{Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	g, err := s.MarkGoalMerged(ctx, runner, p.goal.ID, apitypes.GoalMerged{Commit: "abc"})
	if err != nil || g.Status != "done" {
		t.Fatalf("merge : %+v %v", g, err)
	}
}

func TestMergeRejectedCancelsGoal(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	p := planned(t, s)
	completeTask(t, s, p.contract)
	completeTask(t, s, p.back)
	completeTask(t, s, p.f)
	payload, _ := json.Marshal(apitypes.MergePayload{Branch: p.goal.Branch, BaseBranch: "main"})
	a, err := s.CreateApproval(ctx, runner, apitypes.ApprovalCreate{GoalID: p.goal.ID, Kind: "merge", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	// Refuser exige un motif.
	_, err = s.DecideApproval(ctx, human, a.ID, apitypes.ApprovalDecision{Decision: "reject"})
	wantCode(t, err, domain.CodeValidationFailed)
	if _, err := s.DecideApproval(ctx, human, a.ID, apitypes.ApprovalDecision{Decision: "reject", Comment: ptr("Pas comme ça")}); err != nil {
		t.Fatal(err)
	}
	d, err := s.GetGoal(ctx, human, p.goal.ID)
	if err != nil || d.Status != "cancelled" || d.Approvals[0].Status != "rejected" {
		t.Fatalf("objectif : %s %v", d.Status, err)
	}
	_, err = s.DecideApproval(ctx, human, a.ID, apitypes.ApprovalDecision{Decision: "approve"})
	wantCode(t, err, domain.CodeAlreadyDecided)
}
