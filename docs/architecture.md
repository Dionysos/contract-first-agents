# Architecture de l'application Go

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). Une section qui commence par *Phase 2*, *Phase 3*, *Phase 4* ou *Prévu* n'est pas encore implémentée ; une note *En phase 1* signale ce qui diffère aujourd'hui. La [feuille de route](../README.fr.md#feuille-de-route) indique ce qui est déjà en place.

Ce document décrit l'organisation du code Go de cfa : les packages, leurs types et leurs méthodes. Il complète [cfa.md](cfa.md) (le quoi), [api.md](api.md) (les règles de l'API), [database.md](database.md) (le schéma) et [data-contracts.md](data-contracts.md) (les formats échangés).

## Choix techniques

| Sujet | Choix | Pourquoi |
| --- | --- | --- |
| Go | 1.26+ | Routage par méthode et motif dans `net/http` (`POST /goals/{id}/cancel`), `log/slog` |
| HTTP | `net/http` seul | Pas de framework : le routeur standard suffit |
| Base | `pgx/v5` + `pgxpool` | Pilote natif PostgreSQL, `LISTEN/NOTIFY`, `JSONB` |
| Accès aux données | **sqlc** (génération de code à partir du SQL) | Joue le rôle d'un ORM sans en avoir le coût : on écrit du SQL, sqlc génère des fonctions Go typées sur pgx. Les erreurs de requête apparaissent à la génération, pas à l'exécution. |
| Migrations | goose (bibliothèque), fichiers embarqués | Appliquées au démarrage de `cfa-api` |
| Configuration | `cfa.yaml` (`gopkg.in/yaml.v3`) + variables d'environnement | Voir [integration.md](integration.md#configuration) |
| Tests d'intégration | PostgreSQL réel : `CFA_TEST_DATABASE_URL`, sinon PostgreSQL embarqué (`embedded-postgres`, sans Docker) | Les règles vivent en partie dans la base : on teste contre elle |
| Logs | `log/slog`, JSON en production, texte en développement | |

Pas d'ORM classique (GORM, ent) : leur valeur (requêtes générées, relations chargées automatiquement) compte peu ici. En revanche, les verrous, les index partiels et `LISTEN/NOTIFY` demandent du SQL précis.

## Vue d'ensemble

Quatre exécutables Go, un dépôt, un module. Le déploiement (conteneurs, réseaux, isolement) est décrit dans [infrastructure.md](infrastructure.md).

```
            humain                                   agents (conteneurs éphémères)
   ┌──────────┴──────────┐                          ┌──────────────┐
   │ navigateur   CLI cfa│                          │ pi.dev       │
   │ (dashboard)         │                          │  └─ CLI cfa  │
   └────┬────────────┬───┘                          └──────┬───────┘
   SSE  │   REST     │ REST                         REST   │ (token du run)
        ▼            ▼                                     ▼
   ┌───────────────────────────────────────────────────────────────┐
   │ cfa-api                                                       │
   │  api (HTTP, SSE) ─► service (règles) ─► store (sqlc, pgx)     │
   │        ▲                                   │                  │
   │        └──── events.Hub ◄── LISTEN ────────┤                  │
   └────────────────────────────────────────────┼──────────────────┘
                                                ▼
                                         PostgreSQL 16
                                                ▲ (rien d'autre n'y écrit)
   ┌────────────────────────────────────────────┴──────────────────┐
   │ runner : SSE ─► règles ─► REST (cfa-api) ; git ; Docker       │
   └───────────────────────────────────────────────────────────────┘
```

| Exécutable | Rôle | Parle à |
| --- | --- | --- |
| `cfa-api` | Seul composant qui écrit en base ; applique toutes les règles métier ; diffuse les événements ; sert le dashboard ; mode replay | PostgreSQL |
| `runner` | Réagit aux événements en appliquant des règles fixes : lance les runs, prépare git, vérifie périmètres et contrôles, merge après approbation. Aucune décision de fond. | `cfa-api` (REST + SSE), Docker, git |
| `cfa` | CLI de l'humain (objectifs, approbations, export) et des agents (tâches, messages) | `cfa-api` (REST + SSE) |
| `cfa-mock` | Génère des exports de démonstration avec les types des contrats | — |

**Responsabilités.** Les règles métier (transitions, hop, quotas, budget, droits) sont **dans l'API**, nulle part ailleurs : le runner et la CLI peuvent se tromper ou être remplacés sans casser les garde-fous. Le runner porte l'**orchestration** (quand lancer quoi), et tout ce qui touche git et Docker, que l'API ne voit pas.

## Organisation

```
cmd/
  cfa-api/          main : serve | migrate | token ; --replay
  cfa/              CLI (humain et agents)
  runner/           runner
  cfa-mock/         générateur de la démo du dashboard
  fake-agent/       agent factice scripté (tests sans LLM)
internal/
  config/           chargement de cfa.yaml et de l'environnement
  domain/           statuts, transitions, erreurs métier ; aucune dépendance
  apitypes/         contrats JSON de l'API (requêtes, réponses), partagés avec la CLI
  events/           types et payloads d'événements, hub temps réel
  auth/             tokens, principal, middleware
  store/            pool, transactions, migrations
    db/             code généré par sqlc (ne pas modifier)
    queries/        requêtes SQL sources de sqlc
  service/          cas d'usage : règles métier et transactions
  api/              serveur HTTP : routes, handlers, erreurs, SSE
  replay/           source d'événements lue depuis un export
  client/           client HTTP typé de l'API (+ lecteur SSE), utilisé par la CLI et le runner
  runner/           logique du runner : règles, ordonnanceur, git, Docker
  testdb/           base PostgreSQL de test (réelle ou embarquée)
migrations/         schéma SQL versionné (embarqué)
dashboard/          page du dashboard (embarquée)
runs/               exports pour le replay et la démo
roles/              prompts des rôles
build/              Dockerfiles
scripts/            outils (captures d'écran…)
```

Les dépendances vont dans un seul sens :

```
cmd ─► api ─► service ─► store ─► store/db
        │        │  └──► events ─► apitypes
        │        └─────► domain, config, auth
        └──► auth ─► store
```

`domain` et `apitypes` ne dépendent de rien. `service` ne connaît pas HTTP. `api` ne contient pas de SQL.

## `internal/config`

```go
type Config struct {
    TargetRepo   string          `yaml:"target_repo"`
    BaseBranch   string          `yaml:"base_branch"`
    Remote       string          `yaml:"remote"`
    Concurrency  int             `yaml:"concurrency"`
    AgentCommand string          `yaml:"agent_command"` // remplacé par CFA_AGENT_COMMAND
    Budget       Budget          `yaml:"budget"`
    Limits       Limits          `yaml:"limits"`
    Approvals    Approvals       `yaml:"approvals"`
    Roles        map[string]Role `yaml:"roles"`

    // Environnement
    DatabaseURL     string // DATABASE_URL
    Addr            string // CFA_ADDR, défaut 127.0.0.1:8080
    DashboardOrigin string // CFA_DASHBOARD_ORIGIN, défaut http://localhost:8080
}

type Budget struct { PerGoalUSD, MaxGoalUSD float64; PerRunTokens int; PerRunTimeout time.Duration }
type Limits struct { MaxHop, MaxAttempts, MessagesPerRolePerGoal int }
type Role   struct { Model, Prompt string; Write, Checks []string }

func Load(path string) (*Config, error)   // lit le fichier, applique défauts et environnement, valide
func Default() *Config                     // valeurs par défaut (tests, démarrage sans fichier)
func (c *Config) Validate() error
func (c *Config) IsAgentRole(name string) bool
```

## `internal/domain`

Logique pure, testée sans base.

```go
type GoalStatus string   // GoalPlanning, GoalRunning, GoalAwaitingApproval, GoalDone, GoalFailed, GoalCancelled
type TaskStatus string   // TaskTodo, TaskRunning, TaskBlocked, TaskReview, TaskChangesRequested, TaskDone, TaskFailed
type MessageType string
type RunKind string
type ApprovalKind string

func (s GoalStatus) Terminal() bool
func (s TaskStatus) Terminal() bool

// Qui demande une transition.
type Actor int  // ActorRunner, ActorTaskAgent, ActorReviewer, ActorHuman, ActorSystem

func CanTransitionGoal(from, to GoalStatus, a Actor) bool
func CanTransitionTask(from, to TaskStatus, a Actor) bool

// Erreur métier, traduite telle quelle en réponse HTTP.
type Error struct {
    Status     int
    Code       string
    Message    string
    Fields     map[string]string
    ApprovalID *int64
}
func (e *Error) Error() string

func NotFound(what string) *Error
func Forbidden(msg string) *Error
func Validation(fields map[string]string) *Error
func Conflict(code, msg string) *Error
func InvalidTransition(from, to string) *Error

// Validation de champs.
type Fields map[string]string
func (f Fields) Len(name, v string, min, max int)
func (f Fields) Range(name string, v, min, max float64)
func (f Fields) Required(name string, ok bool)
func (f Fields) Err() error   // nil si vide, sinon *Error validation_failed
```

## `internal/apitypes`

Les structures JSON de l'API, une par schéma de [openapi.yaml](openapi.yaml) : `Goal`, `GoalCreate`, `GoalDetail`, `GoalPage`, `Task`, `TaskCreate`, `TaskDetail`, `TaskStatusChange`, `Review`, `ReviewCreate`, `Message`, `MessageCreate`, `Thread`, `ThreadDetail`, `Approval`, `ApprovalCreate`, `ApprovalDecision`, `Run`, `RunCreate`, `RunCreated`, `RunFinish`, `RunProgress`, `CheckReport`, `Event`, `ExportHeader`, `PublicConfig`, `Health`, `Error`. Règles de forme dans [data-contracts.md](data-contracts.md).

## `internal/events`

```go
const (
    GoalCreated = "goal.created"; GoalStatusChanged = "goal.status_changed"
    TaskCreated = "task.created"; TaskStatusChanged = "task.status_changed"; TaskReviewed = "task.reviewed"
    RunStarted = "run.started"; RunProgress = "run.progress"; RunFinished = "run.finished"
    MessageSent = "message.sent"; MessageRejected = "message.rejected"
    ApprovalRequested = "approval.requested"; ApprovalResolved = "approval.resolved"
    CheckFinished = "check.finished"; BudgetWarning = "budget.warning"; BudgetExceeded = "budget.exceeded"
    MergeDone = "merge.done"
)

// Un payload Go par type : GoalCreatedPayload, GoalStatusChangedPayload, TaskStatusChangedPayload…
// (détail dans data-contracts.md).

// Insère un événement dans la transaction courante.
func Emit(ctx context.Context, q *db.Queries, goalID *int64, typ, actor string, payload any) (int64, error)

// Diffusion temps réel.
type Hub struct { /* pool, abonnés, dernier id diffusé */ }
func NewHub(pool *pgxpool.Pool, log *slog.Logger) *Hub
func (h *Hub) Run(ctx context.Context) error                  // LISTEN cfa_events ; relit et diffuse à chaque NOTIFY
func (h *Hub) Subscribe(goal *int64) *Subscription
func (h *Hub) Backlog(ctx context.Context, goal *int64, after int64, limit int) ([]apitypes.Event, error)

type Subscription struct { C <-chan apitypes.Event }
func (s *Subscription) Close()

// Source abstraite, pour servir le SSE depuis la base ou depuis un export (replay).
type Source interface {
    Subscribe(goal *int64) *Subscription
    Backlog(ctx context.Context, goal *int64, after int64, limit int) ([]apitypes.Event, error)
}
```

**Fonctionnement du hub.** Une connexion dédiée fait `LISTEN cfa_events`. À chaque notification, le hub lit `events WHERE id > dernier_id ORDER BY id` et pousse chaque événement aux abonnés concernés. Un abonné trop lent (tampon de 1 000 événements plein) est fermé : son client se reconnecte avec `Last-Event-ID`. Si la connexion `LISTEN` tombe, le hub se reconnecte et relit depuis le dernier id : il ne perd rien.

## `internal/auth`

```go
type Principal struct {
    Role    string    // human, runner, ci, ou rôle d'agent
    RunID   *int64    // agents
    GoalID  *int64    // agents : objectif du run
    TaskID  *int64    // agents : tâche du run (task, review)
    RunKind string    // agents
    Session bool      // authentifié par cookie
}
func (p Principal) IsHuman() bool
func (p Principal) IsRunner() bool
func (p Principal) IsAgent() bool
func (p Principal) CanSeeGoal(goalID int64) bool

func NewToken() (plain, hash string)        // 32 octets aléatoires, base64url ; hash SHA-256 hexadécimal
func Hash(plain string) string

type Authenticator struct { /* queries, horloge */ }
func (a *Authenticator) FromRequest(ctx context.Context, r *http.Request) (Principal, error)
func (a *Authenticator) Middleware(next http.Handler) http.Handler   // 401 si absent ou invalide ; principal dans le contexte

func WithPrincipal(ctx context.Context, p Principal) context.Context
func FromContext(ctx context.Context) (Principal, bool)
```

## `internal/store`

```go
type Store struct {
    Pool *pgxpool.Pool
    Q    *db.Queries            // requêtes hors transaction (lectures)
}
func Open(ctx context.Context, url string) (*Store, error)
func (s *Store) Close()
func (s *Store) Ping(ctx context.Context) error

// Exécute fn dans une transaction qui commence par pg_advisory_xact_lock(EventsLockKey).
// À utiliser pour toute écriture (elle émet au moins un événement).
func (s *Store) Write(ctx context.Context, fn func(q *db.Queries) error) error

func Migrate(ctx context.Context, url string, direction string) error   // up | down | status
func IsUniqueViolation(err error, constraint string) bool
```

`store/db` est généré par sqlc à partir de `store/queries/*.sql` (un fichier par table). Les noms de requêtes suivent sqlc : `GetGoal`, `ListGoals`, `InsertGoal`, `UpdateGoalStatus`, `CountMessagesByRole`… La configuration est dans `sqlc.yaml` : `NUMERIC` → `float64`, `TIMESTAMPTZ` → `time.Time`, colonnes nullables → pointeurs.

## `internal/service`

Un type `Service` porte tous les cas d'usage. Chaque méthode reçoit le `Principal`, vérifie les droits et les règles, écrit l'état et les événements dans une seule `Store.Write`, puis renvoie un type `apitypes`.

```go
type Service struct {
    store *store.Store
    cfg   *config.Config
    now   func() time.Time
}
func New(st *store.Store, cfg *config.Config) *Service
```

| Méthode | Route | Phase |
| --- | --- | --- |
| `CreateGoal(ctx, p, in GoalCreate, idemKey string) (Goal, created bool, error)` | `POST /goals` | 1 |
| `ListGoals(ctx, p, status string, limit int, before int64) (GoalPage, error)` | `GET /goals` | 1 |
| `GetGoal(ctx, p, id) (GoalDetail, error)` | `GET /goals/{id}` | 1 |
| `CancelGoal(ctx, p, id, reason) (Goal, error)` | `POST /goals/{id}/cancel` | 1 |
| `SetGoalStatus(ctx, p, id, status, reason) (Goal, error)` | `POST /goals/{id}/status` | 1 |
| `MarkGoalMerged(ctx, p, id, in GoalMerged) (Goal, error)` | `POST /goals/{id}/merged` | 2 |
| `CreateTask(ctx, p, in TaskCreate) (Task, error)` | `POST /tasks` | 1 |
| `GetTask(ctx, p, id) (TaskDetail, error)` | `GET /tasks/{id}` | 1 |
| `SetTaskStatus(ctx, p, id, in TaskStatusChange) (Task, error)` | `POST /tasks/{id}/status` | 1 |
| `ReviewTask(ctx, p, id, in ReviewCreate) (Task, error)` | `POST /tasks/{id}/review` | 1 |
| `SendMessage(ctx, p, in MessageCreate) (Message, error)` | `POST /messages` | 2 |
| `Inbox(ctx, p, unreadOnly bool) ([]Message, error)` | `GET /inbox` | 2 |
| `AckMessage(ctx, p, id) (Message, error)` | `POST /messages/{id}/ack` | 2 |
| `GetThread(ctx, p, id) (ThreadDetail, error)` | `GET /threads/{id}` | 2 |
| `ListApprovals(ctx, p, status string, goal int64) ([]Approval, error)` | `GET /approvals` | 2 |
| `CreateApproval(ctx, p, in ApprovalCreate) (Approval, error)` | `POST /approvals` | 2 |
| `DecideApproval(ctx, p, id, in ApprovalDecision) (Approval, error)` | `POST /approvals/{id}/decision` | 2 |
| `CreateRun(ctx, p, in RunCreate) (RunCreated, error)` | `POST /runs` | 2 |
| `ReportRunProgress(ctx, p, id, in RunProgress) (int64, error)` | `POST /runs/{id}/progress` | 2 |
| `FinishRun(ctx, p, id, in RunFinish) (Run, error)` | `POST /runs/{id}/finish` | 2 |
| `ReportCheck(ctx, p, in CheckReport) (int64, error)` | `POST /checks` | 2 |
| `CreateLoginCode(ctx, p) (LoginCode, error)` / `Login(ctx, code) (cookie string, error)` | `/auth/code`, `/login` | 3 |
| `PublicConfig(ctx, p) (PublicConfig, error)` | `GET /config/public` | 3 |
| `Export(ctx, p, goal int64, w io.Writer) error` | `GET /events/export` | 3 |

Fonctions internes notables :

```go
func (s *Service) escalate(ctx, q, goalID int64, taskID *int64, payload EscalationPayload) (approvalID int64, err error)
func (s *Service) chargeRun(ctx, q, run db.Run, cost float64) error   // dépense, budget.warning, budget.exceeded
func (s *Service) changeTaskStatus(ctx, q, t db.Task, to TaskStatus, actor string, fields TaskStatusFields) (db.Task, error)
func (s *Service) changeGoalStatus(ctx, q, g db.Goal, to GoalStatus, actor, reason string) (db.Goal, error)
```

Chaque changement de statut passe par `changeTaskStatus` ou `changeGoalStatus`, qui vérifient la transition et publient l'événement : impossible d'oublier l'un ou l'autre.

## `internal/api`

```go
type Server struct {
    svc     *service.Service
    auth    *auth.Authenticator
    source  events.Source
    cfg     *config.Config
    log     *slog.Logger
    replay  bool
}
func New(...) *Server
func (s *Server) Handler() http.Handler      // routes + middlewares

// Une méthode par route : createGoal, listGoals, getGoal, cancelGoal, setGoalStatus,
// createTask, getTask, setTaskStatus, reviewTask, streamEvents, healthz…

// Aides
func decodeJSON(r *http.Request, v any) error        // 415, 413, 422 champs inconnus
func writeJSON(w http.ResponseWriter, status int, v any)
func writeError(w http.ResponseWriter, err error)    // *domain.Error → réponse ; autre → 500 internal
func pathID(r *http.Request) (int64, error)
```

Middlewares, de l'extérieur vers l'intérieur : récupération des paniques, journalisation (méthode, route, statut, durée, rôle), limite de taille (1 Mio), authentification, contrôle d'`Origin` pour les écritures par cookie, refus des écritures en mode replay.

## `cmd/cfa-api`

```
cfa-api [serve]            # défaut : migrations puis serveur
cfa-api migrate up|down|status
cfa-api token create --role human|runner|ci [--label laptop]   # affiche le token une fois
cfa-api --replay runs/goal-7.jsonl --speed 4
```

Variables : `CFA_CONFIG` (défaut `cfa.yaml`), `DATABASE_URL`, `CFA_ADDR`, `CFA_DASHBOARD_ORIGIN`, `CFA_LOG_FORMAT` (`text` ou `json`), `CFA_BOOTSTRAP_DIR` (docker compose : après les migrations, `serve` écrit `<dir>/runner/token` et `<dir>/human/token`, gardés tant qu'ils sont valides ; voir [ADR 0015](adr/0015-docker-compose-d-abord.md)).

Arrêt propre sur `SIGINT`/`SIGTERM` : fin des requêtes en cours (10 s), fermeture des flux SSE, du hub puis du pool.

## `internal/client`

Client typé partagé par la CLI et le runner : une méthode par route, avec les types `apitypes`.

```go
type Client struct { BaseURL, Token string; HTTP *http.Client }
func New(baseURL, token string) *Client
func FromEnv() (*Client, error)   // CFA_URL ; CFA_TOKEN, sinon le fichier CFA_TOKEN_FILE, sinon ~/.config/cfa/token

func (c *Client) CreateGoal(ctx, in apitypes.GoalCreate, idemKey string) (apitypes.Goal, error)
func (c *Client) CreateTask(ctx, in apitypes.TaskCreate) (apitypes.Task, error)
func (c *Client) SetTaskStatus(ctx, id int64, in apitypes.TaskStatusChange) (apitypes.Task, error)
// … une méthode par route de openapi.yaml

// Erreur renvoyée pour toute réponse non 2xx : le code stable est exploitable.
type APIError struct { Status int; Body apitypes.Error }

// Flux SSE avec reconnexion et Last-Event-ID.
func (c *Client) Events(ctx context.Context, goal *int64, since int64) (<-chan apitypes.Event, <-chan error)
```

## Runner (`cmd/runner`, `internal/runner`)

Le runner est un service sans état propre : tout ce qu'il sait vient des événements et de l'API. Redémarré, il relit le flux depuis le début (`since=0`) et reconstitue sa vue.

### Composants

```go
type Runner struct {
    api     *client.Client
    cfg     *config.Config
    view    *View          // état reconstruit à partir des événements (comme le dashboard)
    sched   *Scheduler
    git     Git
    agents  AgentLauncher
}

func (r *Runner) Run(ctx context.Context) error   // boucle : événement → View.Apply → decide → actions

// Vue minimale : objectifs, tâches, dépendances, runs actifs, messages non lus par rôle, approbations.
type View struct { /* … */ }
func (v *View) Apply(ev apitypes.Event)

// Décisions pures, testées sans Docker ni git : à partir de la vue, la liste d'actions à mener.
func Decide(v *View, cfg *config.Config) []Action
type Action interface{ isAction() }   // StartPlan, StartTask, StartAnswer, StartReview, MergeGoal, AskMergeApproval, …

// Limite de concurrence et file d'attente.
type Scheduler struct { /* sémaphore de taille cfg.Concurrency */ }

// Git sur le dépôt bare.
type Git interface {
    EnsureGoalBranch(ctx, goal apitypes.Goal) error
    AddWorktree(ctx, task apitypes.Task) (path string, err error)
    ChangedFiles(ctx, path string) ([]string, error)
    Diff(ctx, path string, maxBytes int) (diff string, stat apitypes.Diffstat, truncated bool, err error)
    Commit(ctx, path, message string) error
    MergeTaskIntoGoal(ctx, task apitypes.Task) error          // conflit → ErrConflict
    MergeGoal(ctx, goal apitypes.Goal) (commit string, err error)
    RemoveWorktree(ctx, path string) error
}

// Lancement des agents : Docker en production, processus local pour l'agent factice.
type AgentLauncher interface {
    Launch(ctx, spec RunSpec) (Handle, error)
}
type RunSpec struct {
    Run       apitypes.Run
    Token     string
    Workdir   string
    ReadOnly  []string        // chemins remontés en lecture seule (contracts/)
    Prompt    string
    Env       map[string]string
    Timeout   time.Duration
}
type Handle interface {
    Wait(ctx) (Result, error)  // code de sortie, tokens et coût (sortie JSON de pi.dev)
    Progress() <-chan apitypes.RunProgress
    Kill(ctx) error
}
```

Implémentations : `DockerLauncher` (API Docker via le socket, paramètres d'isolement de [infrastructure.md](infrastructure.md#isolement-dun-run)) et `ProcessLauncher` (agent factice lancé en sous-processus, pour les tests).

### Règles de décision

*En phase 1* : pas de runs `answer` ni `review`, ni de statuts `blocked` et `changes_requested` ; un périmètre ou un contrôle en échec fait passer la tâche en `failed`, et un merge refusé annule l'objectif.

| Situation (vue) | Action |
| --- | --- |
| Objectif `planning` sans run `plan` actif | `StartPlan` |
| Run `plan` terminé avec au moins une tâche | `POST /goals/{id}/status running` |
| Tâche `todo` ou `changes_requested`, dépendances `done`, place libre | `StartTask` |
| Rôle avec messages non lus et sans run actif | `StartAnswer` (lecture seule) |
| Tâche `blocked` dont le message attendu a une réponse | `StartTask` (reprise) |
| `run.finished` d'une tâche `review` : périmètre et contrôles OK | `StartReview` ; sinon `changes_requested` avec la raison |
| `run.finished` d'une tâche restée `running` | `failed` (l'agent n'a pas conclu) |
| Toutes les tâches `done` | Fusion des branches, contrôles globaux, `POST /approvals` (`merge`) |
| `approval.resolved` `merge` approuvé | Merge ou PR, puis `POST /goals/{id}/merged` |
| `goal.status_changed` vers un statut terminal | `Kill` des runs actifs de l'objectif |

### Déroulé d'un run

```
Scheduler.Acquire
  → POST /runs (token d'agent)
  → git.AddWorktree
  → AgentLauncher.Launch (prompt = rôle + spec + relectures + messages non lus)
      ↳ Progress → POST /runs/{id}/progress
  → Handle.Wait (ou Kill au timeout)
  → git.ChangedFiles : hors périmètre ? → changes_requested
  → contrôles du rôle → POST /checks
  → git.Commit
  → POST /runs/{id}/finish (tokens, coût, diff)
  → statut de la tâche si besoin
Scheduler.Release
```

## CLI `cfa` (`cmd/cfa`)

Un binaire, sous-commandes par domaine, sans framework (paquet `flag`). Sortie lisible par défaut, `--json` pour les scripts et les agents.

| Commande | Route |
| --- | --- |
| `cfa goal new|list|show|cancel` | `/goals…` |
| `cfa approvals`, `cfa approve <id>`, `cfa reject <id> <motif>` | `/approvals…` |
| `cfa watch [--goal]` | `GET /events` (SSE) |
| `cfa export <goal>`, `cfa replay <fichier>` | `/events/export`, `cfa-api --replay` |
| `cfa dashboard` | `POST /auth/code`, puis ouverture du navigateur |
| `cfa task create|show|block|done` | `/tasks…` (`CFA_TASK` par défaut) |
| `cfa send`, `cfa reply`, `cfa inbox`, `cfa thread` | `/messages`, `/inbox`, `/threads` |
| `cfa review approve|changes` | `POST /tasks/{id}/review` |

Codes de sortie : `0` succès, `1` erreur d'usage, `2` erreur de l'API (le `code` stable est affiché), `3` API injoignable. Les agents s'appuient sur ces codes dans leurs prompts.

## Agent factice (`cmd/fake-agent`, `internal/fakeagent`)

Remplace pi.dev dans les tests (`agent_command: fake-agent`). Il lit un scénario YAML (`CFA_FAKE_SCENARIO`) et rejoue, pour son rôle et son type de run, une liste d'étapes via l'API, comme un vrai agent avec la CLI. Exemple complet : [internal/runner/testdata/scenario.yaml](../internal/runner/testdata/scenario.yaml).

```yaml
planner:
  plan:
    - task: {key: contract, role: contracts, title: "Contrat paginé", spec: "…"}
    - task: {key: api, role: back, title: "API paginée", spec: "…", after: [contract]}
back:
  task:                       # une liste d'étapes par run de la tâche : 1er run, 2e run…
    - - reply_inbox: "Oui."
      - write: {path: api/orders.go, content: "package api\n"}
      - done: "Endpoint paginé"
  answer:                     # runs de réponse
    - reply_inbox: "Oui."
front:
  task:
    - - write: {path: api/hack.go, content: "x"}     # hors périmètre : refusé par le runner
      - done: "fini"
    - - remove: api/hack.go
      - ask: {to: back, text: "total_count toujours renvoyé ?"}
      - block: true                                  # attend la réponse au dernier message
reviewer:
  review:                     # verdicts par rôle de la tâche relue, un par run de relecture
    back: [{changes: "Manque un test"}, {approve: "OK"}]
```

Étapes disponibles : `task`, `write`, `remove`, `progress`, `done`, `fail`, `ask`, `block`, `reply_inbox`, `approve`, `changes`. L'agent écrit sur sa sortie le protocole attendu par le runner (lignes `{"type":"progress"}` puis `{"type":"result"}` avec une consommation fictive).

Le binaire de test du runner sert lui-même d'agent factice (`CFA_BE_FAKE_AGENT=1`) : le test de bout en bout n'a besoin d'aucun binaire supplémentaire.

## Adaptateur pi.dev (`cmd/cfa-pi`, `internal/piagent`)

`cfa-pi` est la commande d'agent par défaut (`agent_command: cfa-pi`). Le runner la lance dans le worktree du run, comme n'importe quel agent. Elle lance pi en mode non interactif, puis traduit son flux d'événements dans le protocole du runner. Le runner ignore donc tout de pi : changer de harness revient à écrire un autre adaptateur.

```
runner ──► cfa-pi ──► pi --mode json --no-session --model <role.model> [--thinking <role.thinking>] "<prompt>"
              │            │
              │            └─ outil bash : cfa task show, cfa send… (coordination via l'API)
              ◄────────────┘ JSON lines (AgentSessionEvent)
              │
              └─► stdout : {"type":"progress",…}* puis {"type":"result","tokens_in",…,"cost_usd"}
```

**Entrées** (variables posées par le runner) : `CFA_PROMPT_FILE` (prompt complet : `_commun.md` + rôle + objectif + tâche), `CFA_MODEL`, `CFA_THINKING`, `CFA_MAX_TOKENS` (`budget.per_run_tokens`), plus `CFA_URL`, `CFA_TOKEN`, etc. utilisées par la CLI `cfa` dans la session. `PI_BIN` remplace l'exécutable `pi`. Les options écrites après `cfa-pi` dans `agent_command` sont transmises à pi (`agent_command: cfa-pi --tools read,bash,edit,write`).

**Traduction des événements de pi**

| Événement pi | Effet |
| --- | --- |
| `tool_execution_start` | Étape `progress` : `bash` (commande), `cfa` (commande `cfa …`, mise en avant dans le dashboard), `read`/`write`/`edit` (chemin), `grep`/`find`, `ls` |
| `tool_execution_end` avec `isError` | Étape `tool_error` |
| `message_end` d'un message `assistant` | Cumul de `usage` : `tokens_in = input + cacheRead + cacheWrite`, `tokens_out = output`, `cost_usd += cost.total` |
| `message_end` avec `stopReason` `error` ou `aborted` | Erreur du modèle retenue (clé API invalide, surcharge…) |
| `auto_retry_start`, `compaction_start` | Étapes `retry`, `compaction` |
| `agent_end` | Fin normale de la session |

**Issue du run.** La ligne `result` est toujours écrite, même en cas d'échec, pour que le coût soit compté dans le budget. `cfa-pi` sort en erreur (le run passe `failed`) si pi échoue, si la dernière réponse du modèle est une erreur (pi sort alors avec le code 0 : seul le flux le signale), si la session s'arrête sans `agent_end`, ou si le plafond de tokens est dépassé (pi est alors arrêté).

**Coût.** pi calcule le coût avec son catalogue de modèles. Pour un modèle qu'il ne connaît pas, il l'accepte mais compte **0 $** : le budget de l'objectif ne verrait pas la dépense. D'où [build/pi/models.json](../build/pi/models.json), qui déclare tarifs, fenêtre de contexte et sortie maximale des modèles récents (copié dans l'image du runner, `PI_CODING_AGENT_DIR=/srv/pi`). En dernier recours, `cfa-pi` signale un run qui consomme des tokens sans coût par une étape `warning`, visible dans le dashboard.

**Tests.** `internal/piagent` est testé sur un enregistrement de session (`testdata/session.jsonl`, au format réel de pi 0.73) et avec un faux exécutable `pi` (script shell) : arguments, dossier de travail, échec, plafond de tokens, coût inconnu. Le format et le comportement en erreur ont été vérifiés avec le vrai pi 0.73.1.

## État du runner

| Élément | État |
| --- | --- |
| Vue, règles de décision, boucle, reprise après redémarrage | ✅ (tests unitaires des règles) |
| Git : branches, worktrees, commit, diff, fusion, merge | ✅ |
| Périmètre, contrôles du rôle, run sans conclusion | ✅ |
| Lancement des agents en sous-processus (`ProcessLauncher`) | ✅ agent factice ; test de bout en bout en ~10 s |
| Adaptateur pi.dev `cfa-pi` (étapes, tokens, coût, plafond de tokens par run) | ✅ testé sur enregistrement et avec pi 0.73.1 ; pas encore de run réel avec une clé API |
| Lancement en conteneur Docker isolé (`DockerLauncher`, réseau filtré) | à faire |
| PR sur un dépôt distant (`remote`) | à faire |

**Messages en attente.** Les accusés de lecture ne produisent pas d'événement : le runner considère qu'un message adressé à un rôle est pris en charge par le premier run de ce rôle qui **démarre après** lui. D'où la règle des prompts : tout run commence par `cfa inbox --unread`.

## Observabilité (OpenTelemetry)

*Phase 4 : pas encore implémenté.*

Paquet `internal/telemetry`, décision : [ADR 0014](adr/0014-opentelemetry-optionnel.md). **Désactivé par défaut** : sans `OTEL_EXPORTER_OTLP_ENDPOINT` (ou `…_TRACES_ENDPOINT`, `…_METRICS_ENDPOINT`), aucun exporteur n'est créé et l'instrumentation ne coûte rien. Configuration par les variables standard (`OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_SDK_DISABLED`…), export OTLP/HTTP.

| Fonction | Rôle |
| --- | --- |
| `Setup(ctx, service, version) (shutdown, error)` | Propagateur W3C (toujours), fournisseurs de traces et de métriques si activé ; `shutdown` vide les tampons à l'arrêt |
| `Enabled() bool` | Un endpoint OTLP est configuré |
| `LogHandler(slog.Handler)` | Ajoute `trace_id` et `span_id` aux journaux émis dans une trace |
| `ContextFromEnv`, `EnvFromContext` | Passent la trace par `TRACEPARENT`/`TRACESTATE` à un processus enfant |
| `EventPublished`, `SubscriberDelta`, `RunFinished` | Métriques métier |

**Traces**

| Où | Spans |
| --- | --- |
| `cfa-api` | Une span serveur par requête d'API (`otelhttp`), nommée d'après la route (`POST /goals/{id}/cancel`), attributs `http.route`, `cfa.role`. Non tracés : `/events` (une connexion SSE dure des heures), `/healthz`, fichiers du dashboard |
| Base | Une span client par requête SQL (`otelpgx`), texte de la requête sans paramètres |
| Runner | Une trace par action : `run task`, `run review`, `prepare-merge`, `merge`… ; dans un run : `git worktree`, `agent` (tokens, coût), `vérifications`, `check` (une par contrôle) ; attributs `cfa.goal.id`, `cfa.task.id`, `cfa.run.id`, `cfa.role`, `cfa.model`, `cfa.run.status` |
| Client Go | `otelhttp.NewTransport` : chaque appel du runner à l'API est une span enfant, propagée par `traceparent` |
| Agent et CLI | Le runner pose `TRACEPARENT` dans l'environnement de l'agent ; la CLI `cfa` s'y rattache. Une trace montre donc : run → agent → `cfa task done` → `POST /tasks/{id}/status` → SQL |

**Métriques** (calculées dans le hub à partir des événements validés, donc cohérentes avec le journal)

| Nom | Type | Attributs |
| --- | --- | --- |
| `cfa.events` | compteur | `type` |
| `cfa.sse.subscribers` | jauge (up/down) | — |
| `cfa.runs` | compteur | `role`, `kind`, `model`, `status` |
| `cfa.run.tokens` | compteur | `role`, `model`, `direction` (`in`, `out`) |
| `cfa.run.cost` (USD) | compteur | `role`, `model` |
| `cfa.run.duration` (s) | histogramme | `role`, `model`, `kind` |

Les métriques HTTP (durée par route, statut) sont celles d'`otelhttp`. Export des métriques toutes les 15 s, sauf `OTEL_METRIC_EXPORT_INTERVAL`.

**Tests et vérification.** `internal/api/telemetry_test.go` vérifie avec un enregistreur en mémoire : nom de la span HTTP, rattachement au `traceparent` entrant, spans SQL enfants, rien pour `/healthz`. Vérifié aussi en conditions réelles : export OTLP de `cfa-api` reçu sur `/v1/traces` (span `POST /goals`, `INSERT INTO goals`) et `/v1/metrics` (`cfa.events`), `trace_id` présent dans les journaux.

## Séquences clés

**Nouvel objectif jusqu'aux premières tâches**

```
humain ──POST /goals──► API ──goal.created──► runner
runner ──POST /runs (plan)──► API ; lance planner
planner ──cfa task create ×3──► API ──task.created ×3──► runner, dashboard
runner (fin du run plan) ──POST /goals/{id}/status running──► API
runner ──StartTask (contracts)──► …
```

**Question entre rôles**

```
front ──cfa send --to back question──► API (hop 1) ──message.sent──► runner
front ──cfa task block --wait 88──► API (task blocked) ; fin du run front
runner : back a un message non lu et pas de run actif → StartAnswer(back)
back ──cfa reply 88──► API (hop 2) ──message.sent──► runner
runner : réponse au message attendu par la tâche front → StartTask(front) (reprise)
```

**Fin d'objectif**

```
reviewer ──cfa review approve──► API (task done) ──task.status_changed──► runner
runner : toutes les tâches done → merge des branches dans goal/<id> → contrôles globaux
runner ──POST /approvals (merge)──► API (goal awaiting_approval) ──approval.requested──► dashboard
humain ──approuver──► API ──approval.resolved──► runner ── merge ──► POST /goals/{id}/merged
```

## Étapes de construction de l'API (cible)

Découpage interne de la construction de l'API cible, à ne pas confondre avec les quatre phases de la [feuille de route](../README.fr.md#feuille-de-route). Dans ce dépôt (phase 1), seule une partie de chaque étape existe : la colonne de droite l'indique.

| Étape | Contenu (cible) | Dans ce dépôt (phase 1) |
| --- | --- | --- |
| **1 — Socle** | Config, migrations, store et sqlc, tokens, erreurs, événements, hub et SSE, objectifs, tâches, relecture, `/healthz`, dashboard embarqué, mode replay | Tout, sauf la relecture (phase 2) et le mode replay (phase 3) |
| **2 — Coordination** | Messages, fils, boîte de réception, garde-fous (hop, quota), approbations et leurs effets, runs et tokens d'agents, budget, contrôles, merge | Runs et tokens d'agents, contrôles, approbation `merge` ; messages, fils, garde-fous et budget en phase 2 |
| **3 — Accès et replay** | Sessions du dashboard, `/config/public`, export, mode replay, client Go et CLI `cfa` | Sessions, client Go et CLI `cfa` ; `/config/public`, export et replay en phase 3 |

## Tests

| Niveau | Où | Base |
| --- | --- | --- |
| Unitaires | `domain`, `config`, `auth` (hachage), `events` (encodage) | Aucune |
| Intégration | `service`, `api` (via `httptest`), `store` (migrations) | PostgreSQL réel |
| Contrat | `internal/apitest` enveloppe le serveur des tests `api` et du bout en bout : chaque réponse est validée contre `docs/openapi.yaml` (code documenté, schéma du corps, ligne par ligne pour l'export JSON Lines), et une requête hors contrat ne doit jamais être acceptée (seul un 4xx est admis). Bibliothèque `pb33f/libopenapi-validator` (OpenAPI 3.1). Non couverts : le flux SSE et les fichiers du dashboard | PostgreSQL réel |
| Bout en bout | Runner + agent factice + API, en HTTP : toutes les routes sont exercées | PostgreSQL réel |

```bash
make test                 # unitaires + intégration (PostgreSQL embarqué si CFA_TEST_DATABASE_URL est vide)
```
