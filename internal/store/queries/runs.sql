-- name: InsertRun :one
INSERT INTO runs (goal_id, task_id, kind, role, model)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ListRunsByGoal :many
SELECT * FROM runs WHERE goal_id = $1 ORDER BY id;

-- name: GetActiveTaskRun :one
SELECT * FROM runs WHERE task_id = $1 AND status = 'running' ORDER BY id DESC LIMIT 1;

-- name: GetRun :one
SELECT * FROM runs WHERE id = $1;

-- name: FinishRun :one
UPDATE runs
SET status = $2, error = $3, finished_at = now()
WHERE id = $1
RETURNING *;
