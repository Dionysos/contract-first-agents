-- name: LockEvents :exec
SELECT pg_advisory_xact_lock(7243);

-- name: InsertEvent :one
INSERT INTO events (goal_id, type, actor, payload)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListEventsAfter :many
SELECT * FROM events
WHERE id > sqlc.arg(after)::bigint
  AND (sqlc.narg(goal_id)::bigint IS NULL OR goal_id = sqlc.narg(goal_id)::bigint)
ORDER BY id
LIMIT sqlc.arg(max_rows)::int;

-- name: MaxEventID :one
SELECT COALESCE(MAX(id), 0)::bigint FROM events;
