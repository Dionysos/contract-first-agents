-- name: InsertTask :one
INSERT INTO tasks (goal_id, role, title, spec)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasksByGoal :many
SELECT * FROM tasks WHERE goal_id = $1 ORDER BY id;

-- name: InsertTaskDep :exec
INSERT INTO task_deps (task_id, depends_on) VALUES ($1, $2);

-- name: ListDepsByGoal :many
SELECT d.task_id, d.depends_on
FROM task_deps d JOIN tasks t ON t.id = d.task_id
WHERE t.goal_id = $1
ORDER BY d.task_id, d.depends_on;

-- name: ListDepsByTask :many
SELECT depends_on FROM task_deps WHERE task_id = $1 ORDER BY depends_on;

-- name: ListTasksByIDs :many
SELECT * FROM tasks WHERE id = ANY(sqlc.arg(ids)::bigint[]) ORDER BY id;

-- name: UpdateTaskStatus :one
UPDATE tasks
SET status = sqlc.arg(status)::text,
    summary = COALESCE(sqlc.narg(summary)::text, summary),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: ListOpenTasksByGoal :many
SELECT * FROM tasks
WHERE goal_id = $1 AND status NOT IN ('done', 'failed')
ORDER BY id;
