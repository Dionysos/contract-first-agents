-- name: InsertGoal :one
INSERT INTO goals (title, description, base_branch)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetGoal :one
SELECT * FROM goals WHERE id = $1;

-- name: ListGoals :many
SELECT * FROM goals
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(before)::bigint IS NULL OR id < sqlc.narg(before)::bigint)
  AND (sqlc.narg(only_id)::bigint IS NULL OR id = sqlc.narg(only_id)::bigint)
ORDER BY id DESC
LIMIT sqlc.arg(max_rows)::int;

-- name: UpdateGoalStatus :one
UPDATE goals
SET status = sqlc.arg(status)::text,
    finished_at = CASE WHEN sqlc.arg(status)::text IN ('done', 'failed', 'cancelled') THEN now() ELSE NULL END
WHERE id = sqlc.arg(id)
RETURNING *;
