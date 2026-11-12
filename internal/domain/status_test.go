package domain

import "testing"

func TestCanTransitionTask(t *testing.T) {
	cases := []struct {
		from, to TaskStatus
		actor    Actor
		want     bool
	}{
		{TaskTodo, TaskRunning, ActorRunner, true},
		{TaskTodo, TaskRunning, ActorTaskAgent, false},
		{TaskRunning, TaskReview, ActorTaskAgent, true},
		{TaskRunning, TaskReview, ActorRunner, false},
		{TaskReview, TaskDone, ActorRunner, true},
		{TaskReview, TaskDone, ActorTaskAgent, false},
		{TaskReview, TaskFailed, ActorRunner, true},
		{TaskDone, TaskFailed, ActorRunner, false},
		{TaskTodo, TaskDone, ActorRunner, false},
	}
	for _, c := range cases {
		if got := CanTransitionTask(c.from, c.to, c.actor); got != c.want {
			t.Errorf("%s → %s par %d : %v, attendu %v", c.from, c.to, c.actor, got, c.want)
		}
	}
}

func TestCanTransitionGoal(t *testing.T) {
	if !CanTransitionGoal(GoalPlanning, GoalRunning, ActorRunner) {
		t.Error("planning → running par le runner refusé")
	}
	if CanTransitionGoal(GoalPlanning, GoalRunning, ActorHuman) {
		t.Error("planning → running par l'humain accepté")
	}
	if CanTransitionGoal(GoalDone, GoalCancelled, ActorHuman) {
		t.Error("done → cancelled accepté")
	}
	if !CanTransitionGoal(GoalRunning, GoalCancelled, ActorHuman) {
		t.Error("running → cancelled par l'humain refusé")
	}
}

func TestFields(t *testing.T) {
	f := Fields{}
	f.Len("title", "abc", 5, 120)
	f.Len("description", "", 20, 5000)
	f.Range("count", 0.1, 0.5, 20)
	err := f.Err()
	de, ok := err.(*Error)
	if !ok || de.Code != CodeValidationFailed || len(de.Fields) != 3 {
		t.Fatalf("erreur inattendue : %#v", err)
	}
	if de.Fields["title"] != "5 caractères minimum" || de.Fields["description"] != "obligatoire" {
		t.Errorf("messages : %v", de.Fields)
	}
	if (Fields{}).Err() != nil {
		t.Error("Fields vide : erreur inattendue")
	}
}
