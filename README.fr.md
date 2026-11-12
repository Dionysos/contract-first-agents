# contract-first-agents

🇫🇷 Français · [🇬🇧 English](README.md)

Un développeur dirige une équipe d'agents IA spécialisés (planner, contracts, back, front), à la manière d'un tech lead. Les agents découpent les objectifs et codent chacun dans leur périmètre, coordonnés par un bus maison (`cfa`). L'humain valide le merge.

![Dashboard : objectif en attente de l'approbation du merge](docs/screenshots/dashboard-phase1-dark.png)

> **Phase 1 sur 4 (MVP).** Ce dépôt contient la boucle de base : un objectif va du planner jusqu'à `main`, approuvé par l'humain. Les phases suivantes sont décrites dans la [feuille de route](#feuille-de-route) ; les documents de `docs/` décrivent la cible complète.

## Fonctionnement

L'humain soumet un objectif ; le planner le découpe en tâches, le rôle `contracts` définit l'interface, puis back et front codent en parallèle à partir de ce contrat. Le runner vérifie chaque tâche. Rien n'arrive sur `main` sans l'accord de l'humain.

```mermaid
flowchart TD
    H1(["👤 Humain : nouvel objectif<br/>titre, description"]) --> P["🧭 planner<br/>découpe en tâches avec dépendances"]
    P --> C["📜 contracts<br/>définit l'interface dans contracts/"]
    C --> PAR

    subgraph PAR ["Travail en parallèle, un worktree git par run"]
        direction LR
        B["⚙️ back<br/>api/, db/"]
        F["🖥️ front<br/>web/"]
    end

    PAR --> V{"Runner : contrôles<br/>périmètre du rôle<br/>make contracts-lint<br/>make test-api, make test-web"}
    V -->|"échec"| KO(["❌ objectif en échec"])
    V -->|"OK"| M["Fusion dans goal/&lt;id&gt;"]
    M --> AM{"👤 Approbation<br/>du merge"}
    AM -->|"accepté"| MAIN(["✅ main"])
    AM -->|"refusé"| KO2(["objectif annulé"])

    classDef human fill:#fde68a,stroke:#b45309,color:#1f2937
    classDef agent fill:#bbf7d0,stroke:#15803d,color:#1f2937
    classDef go fill:#bae6fd,stroke:#0369a1,color:#1f2937
    class H1,AM,MAIN human
    class P,C,B,F agent
    class V,M go
```

Garde-fous appliqués par le bus et le runner : token par run, périmètre d'écriture par rôle, `contracts/` en lecture seule hors du rôle `contracts`, durée maximale par run. Une seule tentative par tâche en phase 1 : une tâche en échec fait échouer l'objectif. Aucun agent ne merge.

## Architecture

```mermaid
flowchart LR
    subgraph USER ["Poste de l'humain"]
        direction TB
        DASH["Dashboard (lecture seule)<br/>React · TypeScript · Bootstrap<br/>EventSource, useReducer"]
        CLIH["CLI cfa<br/>Go"]
    end

    subgraph COMPOSE ["docker compose"]
        direction LR
        subgraph APIC ["cfa-api · Go"]
            direction TB
            HTTP["api<br/>net/http, routes REST, SSE"]
            SVC["service + domain<br/>règles métier, transitions"]
            AUTH["auth<br/>tokens hachés, sessions"]
            HUB["events.Hub<br/>LISTEN → abonnés SSE"]
            STORE["store<br/>pgx + sqlc, migrations goose"]
            EMB["dashboard embarqué<br/>go:embed"]
            HTTP --> AUTH
            HTTP --> SVC --> STORE
            HUB --> HTTP
        end

        PG[("PostgreSQL 16<br/>objectifs, tâches, runs<br/>table events + NOTIFY")]

        subgraph RUNC ["runner · conteneur"]
            direction TB
            RUN["runner · Go<br/>décisions, worktrees git,<br/>contrôles, fusions"]
            ADAPT["cfa-pi · Go<br/>adaptateur pi.dev"]
            FAKE["fake-agent · Go<br/>scénario YAML, sans LLM"]
            PI["pi.dev · Node<br/>agent de code, modèle par rôle"]
            CLIA["CLI cfa · Go<br/>utilisée par l'agent"]
            RUN -->|"lance"| ADAPT --> PI
            RUN -.->|"ou, en test"| FAKE
            PI -->|"outil bash"| CLIA
        end

        GIT[("Dépôt cible git<br/>goal/&lt;id&gt;<br/>task/&lt;objectif&gt;-&lt;tâche&gt;-&lt;rôle&gt;<br/>contracts/ api/ web/")]
    end

    LLM["Fournisseurs LLM<br/>Anthropic, OpenAI, Gemini"]

    DASH -->|"SSE GET /events"| HTTP
    CLIH -->|"HTTP + token"| HTTP
    STORE <-->|"SQL"| PG
    PG -->|"NOTIFY cfa_events"| HUB
    RUN <-->|"SSE + REST"| HTTP
    CLIA -->|"HTTP + token du run"| HTTP
    FAKE -->|"HTTP"| HTTP
    RUN <-->|"worktree, commit, merge"| GIT
    PI <-->|"lit / écrit le code"| GIT
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

| Couleur | Partie | Technologie |
| --- | --- | --- |
| Bleu | Bus, runner, CLI, adaptateur, agent factice | Go (`net/http`, `pgx`, sqlc, goose) |
| Jaune | Dashboard | React, TypeScript, Bootstrap (Vite), embarqué dans `cfa-api` ; types générés depuis les contrats Go |
| Vert | Agents | pi.dev (Node), un modèle LLM par rôle |
| Violet | Données | PostgreSQL 16 (journal `events`), dépôt git cible |

Points clés : le dashboard, la CLI et le runner ne lisent que le **flux d'événements** (SSE avec reprise par `Last-Event-ID`) ; les agents parlent au bus **par la CLI** `cfa`, sans MCP ; pi et l'agent factice passent par la même CLI, donc tout se teste sans LLM. Détail : [docs/architecture.md](docs/architecture.md), [ADR](docs/adr/README.md).

## Démarrer (Docker seulement)

```bash
docker compose up -d --build --wait            # ou : make up
docker compose run --rm cli dashboard          # lien de connexion → http://localhost:8080
docker compose run --rm cli goal new "Pagination de la liste des commandes" \
  --description "20 par page, tests attendus, pas de breaking change."
docker compose run --rm cli approvals          # puis : approve <id> ou reject <id> "motif"
```

Sans `.env`, la pile tourne avec l'**agent factice** (aucune clé API) sur un **dépôt cible de démonstration** créé dans le volume `target` ; les tokens sont créés au démarrage. Pour de vrais agents, copier `.env.example` en `.env` et régler `CFA_AGENT=cfa-pi`, la clé du fournisseur et, au besoin, `TARGET_REPO`. Astuce : `alias cfa='docker compose run --rm cli'`.

| Commande | Effet |
| --- | --- |
| `make up` / `make down` / `make logs` | Démarre, arrête, suit les logs |
| `make smoke` | Bout en bout : un objectif du planner au merge, approbation comprise (jq requis) |
| `docker compose down -v` | Repart de zéro (base, dépôt de démo, tokens) |

## CLI

| Qui | Commandes |
| --- | --- |
| Humain | `goal new` · `goal list` · `goal show` · `goal cancel` · `approvals` · `approve` · `reject` · `watch` · `dashboard` |
| Agents | `task create` (planner) · `task show` · `task done` · `task fail` |

## Développer cfa

Prérequis : Go 1.26+, et Node 18+ pour le dashboard. Pour travailler sur le code, Docker n'est pas indispensable : un PostgreSQL 16 embarqué est téléchargé au premier lancement (tests, et `cfa-devdb` pour une base locale).

```bash
make test            # tests Go : unitaires, d'intégration et de bout en bout (agent factice)
make vet             # go vet + gofmt
make generate        # régénère internal/store/db (sqlc)
make types           # régénère dashboard/src/api/types.ts depuis les contrats Go
make ui-test         # dashboard : vérification des types + tests (Vitest)
make ui-dev          # dashboard en rechargement à chaud (Vite, :5173), relayé vers l'API sur :8080
                     # (CFA_DASHBOARD_ORIGIN=http://localhost:5173 pour que « cfa dashboard » pointe vers Vite)

make build                                                  # construit le dashboard puis les binaires
bin/cfa-devdb &                                             # PostgreSQL local sur le port 5433
export DATABASE_URL="postgres://postgres:postgres@localhost:5433/cfa?sslmode=disable"
bin/cfa-api &                                               # API + dashboard sur 127.0.0.1:8080
export CFA_TOKEN=$(bin/cfa-api token create --role human)
bin/cfa goal new "Pagination de la liste des commandes" --description "20 par page, tests attendus."
bin/cfa dashboard                                           # ouvre le dashboard avec une session
```

## Documentation

Les documents décrivent la **cible complète** (les quatre phases) ; la [feuille de route](#feuille-de-route) indique ce qui est déjà en place.

| Document | Contenu |
| --- | --- |
| [docs/cfa.md](docs/cfa.md) | Spécification générale : concepts, rôles, bus, runner |
| [docs/integration.md](docs/integration.md) | Utilisation : cycle de vie d'un objectif, dashboard, configuration, git, CI |
| [docs/architecture.md](docs/architecture.md) | Architecture logicielle : composants, packages Go, runner, CLI, séquences |
| [docs/infrastructure.md](docs/infrastructure.md) | Architecture infra : conteneurs, réseaux, isolement des agents, secrets, CI |
| [docs/api.md](docs/api.md) · [docs/openapi.yaml](docs/openapi.yaml) | Référence de l'API : droits, statuts, garde-fous, événements, SSE |
| [docs/data-contracts.md](docs/data-contracts.md) | Contrats de données et leur correspondance en Go |
| [docs/database.md](docs/database.md) | Schéma PostgreSQL |
| [docs/dashboard.md](docs/dashboard.md) | Dashboard |
| [docs/load-testing.md](docs/load-testing.md) | Méthode de test de montée en charge (prévue) |
| [docs/adr/](docs/adr/README.md) | Décisions d'architecture (ADR) : une page par choix structurant, avec ses raisons |

## Feuille de route

Le projet avance en quatre phases. Chacune ajoute une couche visible sans réécrire la précédente : la table `events` sert de socle dès la phase 1.

### Phase 1 : MVP, la boucle qui tourne ✅ (ce dépôt)

Un objectif va du planner jusqu'à `main`, approuvé par l'humain.

- API Go : objectifs, tâches, runs, contrôles, approbation du merge, table `events`, flux SSE, token par run.
- CLI `cfa` pour l'humain et les agents.
- Runner :
  - un worktree git par run ;
  - contrôles de périmètre et contrôles du rôle (`make contracts-lint`, `make test-api`, `make test-web`), avec `contracts/` en lecture seule ;
  - fusion dans `goal/<id>`, puis merge dans `main` après approbation.
- Quatre rôles : planner, contracts, back, front, enchaînés par dépendances.
- Agent factice et tests de bout en bout sans LLM (parcours complet, et refus d'un fichier hors périmètre) ; `cfa-pi` minimal.
- Docker compose d'abord : démarrage sans `.env`, dépôt de démo, tokens créés au démarrage.
- Dashboard en lecture seule sur le flux d'événements (React, TypeScript, Bootstrap), connexion par `cfa dashboard`.

*Reste à faire pour clore la phase :* un run réel avec pi et une clé API.

### Phase 2 : l'équipe qui se coordonne

- Questions et réponses entre agents par le bus (5 échanges max par fil).
- Rôle reviewer et demandes de corrections ; 3 tentatives par tâche.
- Approbation humaine des changements de contrat cassants ; escalades.
- Suivi des runs : étapes en direct, tokens, coût, plafond de tokens ; budget par objectif.

### Phase 3 : le cockpit

- Dashboard interactif : formulaire d'objectif, approbations.
- Export JSON Lines, replay par l'API, page de démo statique.

### Phase 4 : prêt pour la prod

- OpenTelemetry : traces HTTP, SQL et runs ; métriques métier.
- Tests de contrat : chaque échange des tests validé contre `docs/openapi.yaml`.
- Agents dans des conteneurs éphémères avec proxy de sortie.
- Banc de montée en charge ([méthode](docs/load-testing.md)).

## Licence

[MIT](LICENSE)
