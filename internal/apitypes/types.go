// Package apitypes définit les contrats JSON de l'API cfa.
//
// Règles (docs/data-contracts.md) :
//   - pas de omitempty dans les réponses : un champ absent vaut null, jamais « manquant » ;
//   - les champs nullables sont des pointeurs ;
//   - les listes sont toujours des tableaux ([] et non null).
//
// Les types de requête utilisent des pointeurs pour distinguer « absent » de « zéro ».
package apitypes

import (
	"encoding/json"
	"time"
)

// Error est le corps de toute réponse d'erreur.
type Error struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors,omitempty"`
}

// ---- Objectifs ----

// GoalCreate est le corps de POST /goals.
type GoalCreate struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	BaseBranch  *string `json:"base_branch,omitempty"`
}

// Goal est un objectif.
type Goal struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	BaseBranch  string     `json:"base_branch"`
	Branch      string     `json:"branch"`
	CreatedAt   time.Time  `json:"created_at"`
	FinishedAt  *time.Time `json:"finished_at"`
}

// GoalPage est une page de GET /goals.
type GoalPage struct {
	Items      []Goal `json:"items"`
	NextBefore *int64 `json:"next_before"`
}

// GoalDetail est la réponse de GET /goals/{id}.
type GoalDetail struct {
	Goal
	Tasks     []Task     `json:"tasks"`
	Approvals []Approval `json:"approvals"`
	Runs      []Run      `json:"runs"`
}

// GoalCancel est le corps (optionnel) de POST /goals/{id}/cancel.
type GoalCancel struct {
	Reason string `json:"reason"`
}

// GoalStatusChange est le corps de POST /goals/{id}/status.
type GoalStatusChange struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// GoalMerged est le corps de POST /goals/{id}/merged.
type GoalMerged struct {
	Commit string `json:"commit"`
}

// ---- Tâches ----

// TaskCreate est le corps de POST /tasks.
type TaskCreate struct {
	GoalID    *int64  `json:"goal_id,omitempty"`
	Role      string  `json:"role"`
	Title     string  `json:"title"`
	Spec      string  `json:"spec"`
	DependsOn []int64 `json:"depends_on"`
}

// Task est une tâche.
type Task struct {
	ID        int64     `json:"id"`
	GoalID    int64     `json:"goal_id"`
	Role      string    `json:"role"`
	Title     string    `json:"title"`
	Spec      string    `json:"spec"`
	Status    string    `json:"status"`
	Branch    string    `json:"branch"`
	DependsOn []int64   `json:"depends_on"`
	Summary   *string   `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TaskDetail est la réponse de GET /tasks/{id}.
type TaskDetail struct {
	Task
	CurrentRun *Run `json:"current_run"`
}

// TaskStatusChange est le corps de POST /tasks/{id}/status.
type TaskStatusChange struct {
	Status  string  `json:"status"`
	Summary *string `json:"summary,omitempty"`
	Reason  *string `json:"reason,omitempty"`
}

// ---- Approbations ----

// ApprovalCreate est le corps de POST /approvals.
type ApprovalCreate struct {
	GoalID  int64           `json:"goal_id"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

// Approval est une demande de décision humaine. Phase 1 : seul le merge (kind « merge »).
type Approval struct {
	ID        int64           `json:"id"`
	GoalID    int64           `json:"goal_id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	Status    string          `json:"status"`
	Comment   *string         `json:"comment"`
	CreatedAt time.Time       `json:"created_at"`
	DecidedAt *time.Time      `json:"decided_at"`
}

// ApprovalDecision est le corps de POST /approvals/{id}/decision.
type ApprovalDecision struct {
	Decision string  `json:"decision"`
	Comment  *string `json:"comment,omitempty"`
}

// MergePayload est le payload d'une approbation merge.
type MergePayload struct {
	Branch     string   `json:"branch"`
	BaseBranch string   `json:"base_branch"`
	Diffstat   Diffstat `json:"diffstat"`
}

// ---- Runs et contrôles ----

// RunCreate est le corps de POST /runs.
type RunCreate struct {
	GoalID int64  `json:"goal_id"`
	TaskID *int64 `json:"task_id,omitempty"`
	Kind   string `json:"kind"`
	Role   string `json:"role"`
	Model  string `json:"model"`
}

// RunCreated est la réponse de POST /runs.
type RunCreated struct {
	Run       Run       `json:"run"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Run est une exécution d'agent.
type Run struct {
	ID         int64      `json:"id"`
	GoalID     int64      `json:"goal_id"`
	TaskID     *int64     `json:"task_id"`
	Kind       string     `json:"kind"`
	Role       string     `json:"role"`
	Model      string     `json:"model"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// RunFinish est le corps de POST /runs/{id}/finish.
type RunFinish struct {
	Status   string    `json:"status"`
	Error    *string   `json:"error,omitempty"`
	Diffstat *Diffstat `json:"diffstat,omitempty"`
	Files    []string  `json:"files"`
}

// Diffstat résume un diff.
type Diffstat struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

// CheckReport est le corps de POST /checks.
type CheckReport struct {
	GoalID     int64   `json:"goal_id"`
	TaskID     *int64  `json:"task_id,omitempty"`
	RunID      *int64  `json:"run_id,omitempty"`
	Name       string  `json:"name"`
	Command    *string `json:"command,omitempty"`
	Status     string  `json:"status"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Output     *string `json:"output,omitempty"`
}

// EventRef est la réponse des routes qui ne font que publier un événement.
type EventRef struct {
	EventID int64 `json:"event_id"`
}

// ---- Événements ----

// Event est l'enveloppe commune des événements (SSE).
type Event struct {
	ID        int64           `json:"id"`
	GoalID    *int64          `json:"goal_id"`
	Type      string          `json:"type"`
	Actor     string          `json:"actor"`
	CreatedAt time.Time       `json:"created_at"`
	Payload   json.RawMessage `json:"payload"`
}

// ---- Système ----

// Health est la réponse de GET /healthz.
type Health struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}
