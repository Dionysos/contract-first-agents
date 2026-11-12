-- name: InsertToken :exec
INSERT INTO tokens (hash, role, run_id, label, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetTokenPrincipal :one
SELECT t.role, t.run_id, t.expires_at, t.revoked_at,
       r.goal_id AS run_goal_id, r.task_id AS run_task_id, r.kind AS run_kind, r.status AS run_status
FROM tokens t
LEFT JOIN runs r ON r.id = t.run_id
WHERE t.hash = $1;

-- name: RevokeRunTokens :exec
UPDATE tokens SET revoked_at = now() WHERE run_id = $1 AND revoked_at IS NULL;

-- name: InsertLoginCode :exec
INSERT INTO login_codes (hash, expires_at) VALUES ($1, $2);

-- name: UseLoginCode :one
UPDATE login_codes SET used_at = now()
WHERE hash = $1 AND used_at IS NULL AND expires_at > now()
RETURNING hash;

-- name: InsertSession :exec
INSERT INTO sessions (hash, expires_at) VALUES ($1, $2);

-- name: GetSession :one
SELECT * FROM sessions WHERE hash = $1 AND expires_at > now();

-- name: PurgeAuth :exec
WITH s AS (DELETE FROM sessions WHERE expires_at < now())
DELETE FROM login_codes WHERE created_at < now() - interval '1 hour';
