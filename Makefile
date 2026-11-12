# cfa — commandes. Docker compose d'abord (docs/adr/0015) ; les cibles Go sont pour développer cfa.
GO ?= go
COMPOSE ?= docker compose

.PHONY: up down logs ps dashboard smoke build ui ui-dev ui-test types test test-race vet generate run

up: ## Construit et démarre la pile (agent factice sans .env) : http://localhost:8080
	$(COMPOSE) up -d --build --wait

down: ## Arrête la pile (les volumes sont gardés ; « docker compose down -v » pour repartir de zéro)
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f

ps:
	$(COMPOSE) ps

dashboard: ## Lien de connexion au dashboard
	$(COMPOSE) run --rm cli dashboard

smoke: up ## Bout en bout sous compose : un objectif du planner au merge (jq requis)
	scripts/compose-smoke.sh

build: ui ## Construit le dashboard puis compile toutes les commandes dans bin/
	$(GO) build -o bin/ ./cmd/...

ui: ## Construit le dashboard (React) dans dashboard/dist, embarqué par cfa-api
	cd dashboard && npm ci --no-audit --no-fund && npm run build

ui-dev: ## Dashboard en développement (Vite, rechargement à chaud), relayé vers l'API sur :8080
	cd dashboard && npm run dev

ui-test: ## Vérification des types et tests du dashboard
	cd dashboard && npm run typecheck && npm test

types: ## Régénère les types TypeScript du dashboard depuis les contrats Go
	$(GO) run ./cmd/cfa-tsgen

test: ## Tests unitaires et d'intégration (PostgreSQL embarqué si CFA_TEST_DATABASE_URL est vide)
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...
	@test -z "$$(gofmt -l cmd internal dashboard migrations)" || (gofmt -l cmd internal dashboard migrations; exit 1)

generate: ## Régénère le code d'accès aux données (sqlc)
	sqlc generate

run: build ## Lance l'API (DATABASE_URL requis)
	bin/cfa-api
