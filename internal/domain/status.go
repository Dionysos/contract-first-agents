// Package domain contient les règles métier pures : statuts, transitions, erreurs.
package domain

// GoalStatus est le statut d'un objectif.
type GoalStatus string

const (
	GoalPlanning         GoalStatus = "planning"
	GoalRunning          GoalStatus = "running"
	GoalAwaitingApproval GoalStatus = "awaiting_approval"
	GoalDone             GoalStatus = "done"
	GoalFailed           GoalStatus = "failed"
	GoalCancelled        GoalStatus = "cancelled"
)

// Terminal indique un statut définitif.
func (s GoalStatus) Terminal() bool {
	return s == GoalDone || s == GoalFailed || s == GoalCancelled
}

// Valid indique un statut connu.
func (s GoalStatus) Valid() bool {
	switch s {
	case GoalPlanning, GoalRunning, GoalAwaitingApproval, GoalDone, GoalFailed, GoalCancelled:
		return true
	}
	return false
}

// TaskStatus est le statut d'une tâche.
type TaskStatus string

const (
	TaskTodo    TaskStatus = "todo"
	TaskRunning TaskStatus = "running"
	TaskReview  TaskStatus = "review" // terminée par l'agent, en vérification par le runner
	TaskDone    TaskStatus = "done"
	TaskFailed  TaskStatus = "failed"
)

// Terminal indique un statut définitif.
func (s TaskStatus) Terminal() bool { return s == TaskDone || s == TaskFailed }

// Actor désigne qui demande une transition.
type Actor int

const (
	ActorSystem    Actor = iota // l'API elle-même (effets automatiques)
	ActorRunner                 // le runner
	ActorTaskAgent              // l'agent du run task en cours sur la tâche
	ActorHuman                  // l'humain
)

type goalEdge struct {
	from, to GoalStatus
}

var goalTransitions = map[goalEdge][]Actor{
	{GoalPlanning, GoalRunning}:           {ActorRunner},
	{GoalRunning, GoalPlanning}:           {ActorRunner},
	{GoalRunning, GoalAwaitingApproval}:   {ActorSystem},
	{GoalAwaitingApproval, GoalRunning}:   {ActorSystem},
	{GoalAwaitingApproval, GoalDone}:      {ActorRunner},
	{GoalPlanning, GoalFailed}:            {ActorRunner, ActorSystem},
	{GoalRunning, GoalFailed}:             {ActorRunner, ActorSystem},
	{GoalAwaitingApproval, GoalFailed}:    {ActorRunner, ActorSystem},
	{GoalPlanning, GoalCancelled}:         {ActorHuman, ActorSystem},
	{GoalRunning, GoalCancelled}:          {ActorHuman, ActorSystem},
	{GoalAwaitingApproval, GoalCancelled}: {ActorHuman, ActorSystem},
}

// CanTransitionGoal indique si a peut faire passer un objectif de from à to.
func CanTransitionGoal(from, to GoalStatus, a Actor) bool {
	return allowed(goalTransitions[goalEdge{from, to}], a)
}

type taskEdge struct {
	from, to TaskStatus
}

var taskTransitions = map[taskEdge][]Actor{
	// Lancement d'un run task (POST /runs).
	{TaskTodo, TaskRunning}: {ActorRunner},
	// Fin du travail : l'agent rend la main (cfa task done).
	{TaskRunning, TaskReview}: {ActorTaskAgent},
	// Vérification par le runner : périmètre et contrôles.
	{TaskReview, TaskDone}: {ActorRunner},
	// Échecs. Phase 1 : pas de nouvelle tentative, l'objectif échoue.
	{TaskRunning, TaskFailed}: {ActorTaskAgent, ActorRunner, ActorSystem},
	{TaskTodo, TaskFailed}:    {ActorRunner, ActorSystem},
	{TaskReview, TaskFailed}:  {ActorRunner, ActorSystem},
}

// CanTransitionTask indique si a peut faire passer une tâche de from à to.
func CanTransitionTask(from, to TaskStatus, a Actor) bool {
	return allowed(taskTransitions[taskEdge{from, to}], a)
}

func allowed(actors []Actor, a Actor) bool {
	for _, x := range actors {
		if x == a {
			return true
		}
	}
	return false
}

// Types de runs et d'approbations.
const (
	RunTask = "task"
	RunPlan = "plan"

	ApprovalMerge = "merge"
)

// Participants non agents.
const (
	RoleHuman  = "human"
	RoleRunner = "runner"
	RoleSystem = "system" // acteur des événements produits par l'API
)
