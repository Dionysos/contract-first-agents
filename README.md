# contract-first-agents

[🇫🇷 Français](README.fr.md) · 🇬🇧 English

A developer leads a team of specialized AI agents (planner, contracts, back, front), the way a tech lead would. The agents break goals down and each one codes within its own scope, coordinated by a home-grown bus (`cfa`). The human approves the merge.

![Dashboard: goal waiting for merge approval](docs/screenshots/dashboard-phase1-dark.png)

> **Phase 1 of 4 (MVP).** This repository contains the core loop: a goal goes from the planner all the way to `main`, approved by the human. The next phases are described in the [roadmap](#roadmap); the documents in `docs/` describe the full target.

## How it works

The human submits a goal; the planner breaks it into tasks, the `contracts` role defines the interface, then back and front code in parallel against that contract. The runner checks every task. Nothing reaches `main` without the human's approval.

```mermaid
flowchart TD
    H1(["👤 Human: new goal<br/>title, description"]) --> P["🧭 planner<br/>splits into tasks with dependencies"]
    P --> C["📜 contracts<br/>defines the interface in contracts/"]
    C --> PAR

    subgraph PAR ["Parallel work, one git worktree per run"]
        direction LR
        B["⚙️ back<br/>api/, db/"]
        F["🖥️ front<br/>web/"]
    end

    PAR --> V{"Runner: checks<br/>role scope<br/>make contracts-lint<br/>make test-api, make test-web"}
    V -->|"failure"| KO(["❌ goal failed"])
    V -->|"OK"| M["Merge into goal/&lt;id&gt;"]
    M --> AM{"👤 Merge<br/>approval"}
    AM -->|"accepted"| MAIN(["✅ main"])
    AM -->|"rejected"| KO2(["goal cancelled"])

    classDef human fill:#fde68a,stroke:#b45309,color:#1f2937
    classDef agent fill:#bbf7d0,stroke:#15803d,color:#1f2937
    classDef go fill:#bae6fd,stroke:#0369a1,color:#1f2937
    class H1,AM,MAIN human
    class P,C,B,F agent
    class V,M go
```

Guardrails enforced by the bus and the runner: one token per run, a write scope per role, `contracts/` read-only outside the `contracts` role, and a maximum duration per run. One attempt per task in phase 1: a failed task fails the goal. No agent ever merges.

## Architecture

```mermaid
flowchart LR
    subgraph USER ["Human's machine"]
        direction TB
        DASH["Dashboard (read-only)<br/>React · TypeScript · Bootstrap<br/>EventSource, useReducer"]
        CLIH["cfa CLI<br/>Go"]
    end

    subgraph COMPOSE ["docker compose"]
        direction LR
        subgraph APIC ["cfa-api · Go"]
            direction TB
            HTTP["api<br/>net/http, REST routes, SSE"]
            SVC["service + domain<br/>business rules, transitions"]
            AUTH["auth<br/>hashed tokens, sessions"]
            HUB["events.Hub<br/>LISTEN → SSE subscribers"]
            STORE["store<br/>pgx + sqlc, goose migrations"]
            EMB["embedded dashboard<br/>go:embed"]
            HTTP --> AUTH
            HTTP --> SVC --> STORE
            HUB --> HTTP
        end

        PG[("PostgreSQL 16<br/>goals, tasks, runs<br/>events table + NOTIFY")]

        subgraph RUNC ["runner · container"]
            direction TB
            RUN["runner · Go<br/>decisions, git worktrees,<br/>checks, merges"]
            ADAPT["cfa-pi · Go<br/>pi.dev adapter"]
            FAKE["fake-agent · Go<br/>YAML scenario, no LLM"]
            PI["pi.dev · Node<br/>coding agent, model per role"]
            CLIA["cfa CLI · Go<br/>used by the agent"]
            RUN -->|"launches"| ADAPT --> PI
            RUN -.->|"or, in tests"| FAKE
            PI -->|"bash tool"| CLIA
        end

        GIT[("Target git repository<br/>goal/&lt;id&gt;<br/>task/&lt;goal&gt;-&lt;task&gt;-&lt;role&gt;<br/>contracts/ api/ web/")]
    end

    LLM["LLM providers<br/>Anthropic, OpenAI, Gemini"]

    DASH -->|"SSE GET /events"| HTTP
    CLIH -->|"HTTP + token"| HTTP
    STORE <-->|"SQL"| PG
    PG -->|"NOTIFY cfa_events"| HUB
    RUN <-->|"SSE + REST"| HTTP
    CLIA -->|"HTTP + run token"| HTTP
    FAKE -->|"HTTP"| HTTP
    RUN <-->|"worktree, commit, merge"| GIT
    PI <-->|"reads / writes code"| GIT
    PI -->|"HTTPS"| LLM

    classDef go fill:#bae6fd,stroke:#0369a1,color:#1f2937
    classDef front fill:#fde68a,stroke:#b45309,color:#1f2937
    classDef pi fill:#bbf7d0,stroke:#15803d,color:#1f2937
    classDef data fill:#e9d5ff,stroke:#7e22ce,color:#1f2937
    classDef ext fill:#e5e7eb,stroke:#4b5563,color:#1f2937
    class CLIH,HTTP,SVC,AUTH,HUB,STORE,EMB,RUN,ADAPT,FAKE,CLIA go
    class DASH front
    class PI pi
    class PG,GIT data
    class LLM ext
```

| Color | Part | Technology |
| --- | --- | --- |
| Blue | Bus, runner, CLI, adapter, fake agent | Go (`net/http`, `pgx`, sqlc, goose) |
| Yellow | Dashboard | React, TypeScript, Bootstrap (Vite), embedded in `cfa-api`; types generated from the Go contracts |
| Green | Agents | pi.dev (Node), one LLM model per role |
| Purple | Data | PostgreSQL 16 (`events` log), target git repository |

Key points: the dashboard, the CLI and the runner only read the **event stream** (SSE, resumable with `Last-Event-ID`); agents talk to the bus **through the `cfa` CLI**, without MCP; pi and the fake agent go through the same CLI, so everything can be tested without an LLM. Details: [docs/architecture.md](docs/architecture.md), [ADRs](docs/adr/README.md) (in French).

## Getting started (Docker only)

```bash
docker compose up -d --build --wait            # or: make up
docker compose run --rm cli dashboard          # login link → http://localhost:8080
docker compose run --rm cli goal new "Paginate the orders list" \
  --description "20 per page, tests expected, no breaking change."
docker compose run --rm cli approvals          # then: approve <id> or reject <id> "reason"
```

Without a `.env` file, the stack runs with the **fake agent** (no API key) on a **demo target repository** created in the `target` volume; tokens are created at startup. For real agents, copy `.env.example` to `.env` and set `CFA_AGENT=cfa-pi`, the provider's key and, if needed, `TARGET_REPO`. Tip: `alias cfa='docker compose run --rm cli'`.

| Command | Effect |
| --- | --- |
| `make up` / `make down` / `make logs` | Start, stop, follow the logs |
| `make smoke` | End to end: one goal from the planner to the merge, approval included (requires jq) |
| `docker compose down -v` | Start over from scratch (database, demo repository, tokens) |

## CLI

| Who | Commands |
| --- | --- |
| Human | `goal new` · `goal list` · `goal show` · `goal cancel` · `approvals` · `approve` · `reject` · `watch` · `dashboard` |
| Agents | `task create` (planner) · `task show` · `task done` · `task fail` |

## Developing cfa

Requirements: Go 1.26+, and Node 18+ for the dashboard. Docker is not required to work on the code: an embedded PostgreSQL 16 is downloaded on first use (tests, and `cfa-devdb` for a local database).

```bash
make test            # Go tests: unit, integration and end to end (fake agent)
make vet             # go vet + gofmt
make generate        # regenerates internal/store/db (sqlc)
make types           # regenerates dashboard/src/api/types.ts from the Go contracts
make ui-test         # dashboard: type checking + tests (Vitest)
make ui-dev          # dashboard with hot reload (Vite, :5173), proxied to the API on :8080
                     # (CFA_DASHBOARD_ORIGIN=http://localhost:5173 so that "cfa dashboard" points to Vite)

make build                                                  # builds the dashboard, then the binaries
bin/cfa-devdb &                                             # local PostgreSQL on port 5433
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cfa?sslmode=disable"
bin/cfa-api &                                               # API + dashboard on 127.0.0.1:8080
export CFA_TOKEN=$(bin/cfa-api token create --role human)
bin/cfa goal new "Paginate the orders list" --description "20 per page, tests expected."
bin/cfa dashboard                                           # opens the dashboard with a session
```

## Documentation

The documents are written in French. They describe the **full target** (all four phases); the [roadmap](#roadmap) shows what is already in place.

| Document | Contents |
| --- | --- |
| [docs/cfa.md](docs/cfa.md) | General specification: concepts, roles, bus, runner |
| [docs/integration.md](docs/integration.md) | Usage: goal lifecycle, dashboard, configuration, git, CI |
| [docs/architecture.md](docs/architecture.md) | Software architecture: components, Go packages, runner, CLI, sequences |
| [docs/infrastructure.md](docs/infrastructure.md) | Infrastructure: containers, networks, agent isolation, secrets, CI |
| [docs/api.md](docs/api.md) · [docs/openapi.yaml](docs/openapi.yaml) | API reference: permissions, statuses, guardrails, events, SSE |
| [docs/openapi-phase1.yaml](docs/openapi-phase1.yaml) | Phase 1 API only, as implemented in this repository (routes, schemas, event catalog) |
| [docs/data-contracts.md](docs/data-contracts.md) | Data contracts and their Go counterparts |
| [docs/database.md](docs/database.md) | PostgreSQL schema |
| [docs/dashboard.md](docs/dashboard.md) | Dashboard |
| [docs/load-testing.md](docs/load-testing.md) | Load testing method (planned) |
| [docs/adr/](docs/adr/README.md) | Architecture decision records (ADRs): one page per structural choice, with its reasons |

## Roadmap

The project moves forward in four phases. Each one adds a visible layer without rewriting the previous one: the `events` table is the foundation from phase 1 onwards.

### Phase 1: MVP, the loop that runs ✅ (this repository)

A goal goes from the planner all the way to `main`, approved by the human.

- Go API: goals, tasks, runs, checks, merge approval, `events` table, SSE stream, one token per run.
- `cfa` CLI for the human and the agents.
- Runner:
  - one git worktree per run;
  - scope checks and role checks (`make contracts-lint`, `make test-api`, `make test-web`), with `contracts/` read-only;
  - merge into `goal/<id>`, then into `main` after approval.
- Four roles: planner, contracts, back, front, chained by dependencies.
- Fake agent and end-to-end tests without an LLM (full run, and rejection of a file outside the role's scope); minimal `cfa-pi`.
- Docker compose first: starts without `.env`, demo repository, tokens created at startup.
- Read-only dashboard on the event stream (React, TypeScript, Bootstrap), login through `cfa dashboard`.

*Left to close the phase:* a real run with pi and an API key.

### Phase 2: the team that coordinates

- Questions and answers between agents through the bus (5 exchanges max per thread).
- Reviewer role and change requests; 3 attempts per task.
- Human approval of breaking contract changes; escalations.
- Run tracking: live steps, tokens, cost, token cap; budget per goal.

### Phase 3: the cockpit

- Interactive dashboard: goal form, approvals.
- JSON Lines export, replay through the API, static demo page.

### Phase 4: production-ready

- OpenTelemetry: HTTP, SQL and run traces; business metrics.
- Contract tests: every exchange in the tests validated against `docs/openapi.yaml`.
- Agents in ephemeral containers with an egress proxy.
- Load testing bench ([method](docs/load-testing.md)).

## License

[MIT](LICENSE)
