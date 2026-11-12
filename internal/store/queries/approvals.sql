-- name: ListApprovalsByGoal :many
SELECT * FROM approvals WHERE goal_id = $1 ORDER BY id;

-- name: RejectPendingApprovalsByGoal :many
UPDATE approvals
SET status = 'rejected', comment = sqlc.arg(comment)::text, decided_at = now()
WHERE goal_id = sqlc.arg(goal_id) AND status = 'pending'
RETURNING *;

-- name: InsertApproval :one
INSERT INTO approvals (goal_id, kind, payload) VALUES ($1, $2, $3) RETURNING *;

-- name: GetApproval :one
SELECT * FROM approvals WHERE id = $1;

-- name: GetPendingApproval :one
SELECT * FROM approvals WHERE kind = $1 AND goal_id = $2 AND status = 'pending' LIMIT 1;

-- name: GetApprovedMerge :one
SELECT * FROM approvals WHERE goal_id = $1 AND kind = 'merge' AND status = 'approved' ORDER BY id DESC LIMIT 1;

-- name: ListApprovals :many
SELECT * FROM approvals
WHERE (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
  AND (sqlc.narg(goal_id)::bigint IS NULL OR goal_id = sqlc.narg(goal_id)::bigint)
ORDER BY id;

-- name: DecideApproval :one
UPDATE approvals SET status = $2, comment = $3, decided_at = now() WHERE id = $1 RETURNING *;
