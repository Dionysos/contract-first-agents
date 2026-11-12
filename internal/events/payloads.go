// Package events définit les événements de cfa et leur diffusion en temps réel.
//
// Un événement contient tout ce qu'il faut pour l'afficher : le dashboard et le runner
// n'utilisent que ce journal (docs/api.md#catalogue).
package events

import "github.com/Dionysos/contract-first-agents/internal/apitypes"

// Types d'événements.
const (
	GoalCreated       = "goal.created"
	GoalStatusChanged = "goal.status_changed"
	TaskCreated       = "task.created"
	TaskStatusChanged = "task.status_changed"
	RunStarted        = "run.started"
	RunFinished       = "run.finished"
	ApprovalRequested = "approval.requested"
	ApprovalResolved  = "approval.resolved"
	CheckFinished     = "check.finished"
	MergeDone         = "merge.done"
)

// GoalCreatedPayload accompagne goal.created.
type GoalCreatedPayload struct {
	Goal apitypes.Goal `json:"goal"`
}

// GoalStatusChangedPayload accompagne goal.status_changed.
type GoalStatusChangedPayload struct {
	GoalID int64   `json:"goal_id"`
	Title  string  `json:"title"`
	From   string  `json:"from"`
	To     string  `json:"to"`
	Reason *string `json:"reason"`
}

// TaskCreatedPayload accompagne task.created.
type TaskCreatedPayload struct {
	Task apitypes.Task `json:"task"`
}

// TaskStatusChangedPayload accompagne task.status_changed.
type TaskStatusChangedPayload struct {
	TaskID  int64   `json:"task_id"`
	Role    string  `json:"role"`
	Title   string  `json:"title"`
	From    string  `json:"from"`
	To      string  `json:"to"`
	Reason  *string `json:"reason"`
	Summary *string `json:"summary"`
}

// RunStartedPayload accompagne run.started.
type RunStartedPayload struct {
	Run       apitypes.Run `json:"run"`
	TaskTitle *string      `json:"task_title"`
}

// RunFinishedPayload accompagne run.finished.
type RunFinishedPayload struct {
	Run        apitypes.Run       `json:"run"`
	DurationMS int64              `json:"duration_ms"`
	Error      *string            `json:"error"`
	Diffstat   *apitypes.Diffstat `json:"diffstat"`
	Files      []string           `json:"files"`
}

// ApprovalRequestedPayload accompagne approval.requested.
type ApprovalRequestedPayload struct {
	Approval apitypes.Approval `json:"approval"`
}

// ApprovalResolvedPayload accompagne approval.resolved.
type ApprovalResolvedPayload struct {
	ApprovalID int64   `json:"approval_id"`
	Kind       string  `json:"kind"`
	Decision   string  `json:"decision"`
	Comment    *string `json:"comment"`
}

// CheckFinishedPayload accompagne check.finished.
type CheckFinishedPayload struct {
	TaskID     *int64  `json:"task_id"`
	RunID      *int64  `json:"run_id"`
	Name       string  `json:"name"`
	Command    *string `json:"command"`
	Status     string  `json:"status"`
	DurationMS *int64  `json:"duration_ms"`
	Output     *string `json:"output"`
}

// MergeDonePayload accompagne merge.done.
type MergeDonePayload struct {
	Branch     string `json:"branch"`
	BaseBranch string `json:"base_branch"`
	Commit     string `json:"commit"`
}
