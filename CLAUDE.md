# contract-first-agents : un développeur, une équipe d'agents IA

## Contexte

Un développeur dirige une équipe d'agents IA spécialisés, à la manière d'un tech lead. Il soumet des objectifs ; les agents découpent le travail et codent chacun dans leur périmètre, coordonnés par un bus maison, `cfa` (initiales du repo). L'humain valide le merge.

Le projet avance en quatre phases (feuille de route dans le `README.md`). **Ce dépôt est en phase 1 (MVP)** : un objectif va du planner jusqu'à `main`, approuvé par l'humain. Il se lance en une commande et se suit en direct dans un dashboard.

## À lire avant de commencer

- `README.md` : périmètre de la phase 1, démarrage, CLI, développement, feuille de route.
- `docs/` : la **cible complète** (les quatre phases), pas seulement la phase 1. Chaque document porte un bandeau qui le rappelle. En cas d'écart, le code et les migrations font foi pour la phase 1.
  - `docs/cfa.md` (bus, API, SSE, CLI, runner), `docs/integration.md` (utilisation, configuration, git, contrôles).
  - `docs/architecture.md` (code Go), `docs/infrastructure.md`, `docs/api.md` + `docs/openapi.yaml`, `docs/data-contracts.md`, `docs/database.md`, `docs/dashboard.md`.
- `docs/adr/` : décisions d'architecture. Un choix structurant nouveau ou remis en cause donne un nouvel ADR.

## Choix déjà arrêtés (phase 1)

- Harness d'agent : pi.dev (modèle au choix par rôle), lancé en mode non interactif par le runner via `cfa-pi`.
- Bus : API HTTP en Go (`net/http` + `pgx`, requêtes `sqlc`, migrations `goose`), PostgreSQL 16, temps réel via `LISTEN/NOTIFY` exposé en SSE.
- Les agents utilisent le bus via la CLI `cfa` (pas de MCP).
- Une table `events` alimente le temps réel et l'historique ; le dashboard ne dépend que de ce flux.
- Quatre rôles : planner, contracts, back, front (prompts dans `roles/`), enchaînés par dépendances entre tâches.
- Isolation : un worktree git par run ; les agents tournent en sous-processus dans le conteneur du runner (conteneur par run prévu en phase 4). `contracts/` en lecture seule sauf pour le rôle `contracts`.
- Contract-first : `contracts/` est la seule interface front/back.
- Garde-fous : token par run, périmètre d'écriture par rôle, durée maximale par run, aucun agent ne merge.
- Pas de nouvelle tentative : une tâche en échec fait échouer l'objectif ; un merge refusé annule l'objectif.
- Dashboard : React + TypeScript + Bootstrap (Vite), embarqué dans `cfa-api` ; types générés depuis les contrats Go par `make types` (ADR 0016). Lecture seule, connexion par `cfa dashboard`.
- Agent factice configurable pour les tests sans LLM.
- Docker compose d'abord : `docker compose up` sans `.env` démarre postgres, cfa-api et runner (agent factice, dépôt de démo, tokens créés au démarrage) ; CLI via le service `cli` (ADR 0015).

Hors phase 1 (ne pas réintroduire sans en parler) : messages et fils, rôle reviewer, budget et coûts, replay, OpenTelemetry, tests de contrat OpenAPI, idempotence.

## Vérifications

- Go : `make vet`, `make test` (PostgreSQL embarqué si `CFA_TEST_DATABASE_URL` est vide), `sqlc diff`.
- Dashboard : `make ui-test` (typecheck + Vitest), `make ui` (build).
- Après un changement dans `internal/apitypes` ou `internal/events` : `make types` (un test Go vérifie que `dashboard/src/api/types.ts` est à jour).

## Prochaines étapes

1. Lancer la pile sous Docker : `make up` puis `make smoke`.
2. Run réel avec pi et une clé API (`CFA_AGENT=cfa-pi`), critère de fin de la phase 1.
3. CI GitHub Actions verte (tests Go, dashboard, contrats, compose).
4. Phase 2 : l'équipe qui se coordonne (voir la feuille de route).
