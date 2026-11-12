-- Schéma initial de cfa (phase 1). Documentation : docs/database.md.

-- +goose Up

CREATE TABLE goals (
  id             BIGSERIAL PRIMARY KEY,
  title          TEXT NOT NULL CHECK (char_length(title) BETWEEN 5 AND 120),
  description    TEXT NOT NULL CHECK (char_length(description) BETWEEN 20 AND 5000),
  status         TEXT NOT NULL DEFAULT 'planning' CHECK (status IN
                   ('planning','running','awaiting_approval','done','failed','cancelled')),
  base_branch    TEXT NOT NULL,
  branch         TEXT NOT NULL GENERATED ALWAYS AS ('goal/' || id::text) STORED,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at    TIMESTAMPTZ
);
CREATE INDEX goals_status_idx ON goals (status);

CREATE TABLE tasks (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT NOT NULL REFERENCES goals(id),
  role         TEXT NOT NULL,
  title        TEXT NOT NULL CHECK (char_length(title) BETWEEN 5 AND 120),
  spec         TEXT NOT NULL CHECK (char_length(spec) BETWEEN 1 AND 20000),
  status       TEXT NOT NULL DEFAULT 'todo' CHECK (status IN
                 ('todo','running','review','done','failed')),
  branch       TEXT NOT NULL GENERATED ALWAYS AS
                 ('task/' || goal_id::text || '-' || id::text || '-' || role) STORED,
  summary      TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX tasks_goal_idx ON tasks (goal_id);

CREATE TABLE task_deps (
  task_id      BIGINT NOT NULL REFERENCES tasks(id),
  depends_on   BIGINT NOT NULL REFERENCES tasks(id),
  PRIMARY KEY (task_id, depends_on),
  CHECK (task_id <> depends_on)
);
CREATE INDEX task_deps_depends_on_idx ON task_deps (depends_on);

CREATE TABLE runs (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT NOT NULL REFERENCES goals(id),
  task_id      BIGINT REFERENCES tasks(id),
  kind         TEXT NOT NULL CHECK (kind IN ('task','plan')),
  role         TEXT NOT NULL,
  model        TEXT NOT NULL,
  status       TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','succeeded','failed','killed')),
  error        TEXT,
  started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at  TIMESTAMPTZ,
  CHECK (kind <> 'task' OR task_id IS NOT NULL)
);
CREATE INDEX runs_goal_idx ON runs (goal_id);
-- Un seul run actif par tâche.
CREATE UNIQUE INDEX runs_active_task_idx ON runs (task_id) WHERE status = 'running' AND task_id IS NOT NULL;

-- Phase 1 : seule l'approbation du merge est demandée à l'humain.
CREATE TABLE approvals (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT NOT NULL REFERENCES goals(id),
  kind         TEXT NOT NULL CHECK (kind IN ('merge')),
  payload      JSONB NOT NULL,
  status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
  comment      TEXT,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  decided_at   TIMESTAMPTZ,
  CHECK ((status = 'pending') = (decided_at IS NULL))
);
CREATE INDEX approvals_status_idx ON approvals (status, id);
-- Une seule approbation en attente par type et par objectif.
CREATE UNIQUE INDEX approvals_pending_uniq ON approvals (kind, goal_id) WHERE status = 'pending';

CREATE TABLE tokens (
  hash         TEXT PRIMARY KEY,               -- SHA-256 hexadécimal du token
  role         TEXT NOT NULL,                  -- human, runner ou un rôle d'agent
  run_id       BIGINT REFERENCES runs(id),     -- token d'agent lié à un run
  label        TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at   TIMESTAMPTZ,
  revoked_at   TIMESTAMPTZ,
  CHECK (role IN ('human','runner') OR run_id IS NOT NULL)
);
CREATE INDEX tokens_run_idx ON tokens (run_id) WHERE run_id IS NOT NULL;

CREATE TABLE sessions (
  hash         TEXT PRIMARY KEY,               -- SHA-256 du cookie cfa_session
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at   TIMESTAMPTZ NOT NULL
);

CREATE TABLE login_codes (
  hash         TEXT PRIMARY KEY,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at   TIMESTAMPTZ NOT NULL,
  used_at      TIMESTAMPTZ
);

CREATE TABLE events (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT REFERENCES goals(id),
  type         TEXT NOT NULL,
  actor        TEXT NOT NULL,
  payload      JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX events_goal_idx ON events (goal_id, id);

-- Temps réel : chaque événement inséré est signalé sur le canal cfa_events.
-- +goose StatementBegin
CREATE FUNCTION cfa_notify_event() RETURNS trigger AS $$
BEGIN
  PERFORM pg_notify('cfa_events', NEW.id::text);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER events_notify AFTER INSERT ON events
  FOR EACH ROW EXECUTE FUNCTION cfa_notify_event();

-- Le journal est immuable : il alimente l'historique et le dashboard.
-- +goose StatementBegin
CREATE FUNCTION cfa_events_immutable() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'events est en ajout seul';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER events_immutable BEFORE UPDATE OR DELETE ON events
  FOR EACH ROW EXECUTE FUNCTION cfa_events_immutable();

-- +goose Down

DROP TABLE events;
DROP FUNCTION cfa_events_immutable();
DROP FUNCTION cfa_notify_event();
DROP TABLE login_codes;
DROP TABLE sessions;
DROP TABLE tokens;
DROP TABLE approvals;
DROP TABLE runs;
DROP TABLE task_deps;
DROP TABLE tasks;
DROP TABLE goals;
