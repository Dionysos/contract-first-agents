# Infrastructure

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). Une section qui commence par *Phase 2*, *Phase 3*, *Phase 4* ou *Prévu* n'est pas encore implémentée ; une note *En phase 1* signale ce qui diffère aujourd'hui. La [feuille de route](../README.fr.md#feuille-de-route) indique ce qui est déjà en place.

Ce document décrit le déploiement de cfa : conteneurs, images, réseaux, volumes, secrets, isolement des agents, supervision, sauvegarde et CI. L'architecture du code est dans [architecture.md](architecture.md), l'utilisation dans [integration.md](integration.md).

## Cible

| Contrainte | Conséquence |
| --- | --- |
| Une seule machine (poste de dev ou petite VM) | `docker compose`, pas d'orchestrateur |
| Lancement en une commande | `docker compose up` construit et démarre tout, **sans `.env`** ([ADR 0015](adr/0015-docker-compose-d-abord.md)) |
| Agents qui exécutent du code produit par un LLM | Conteneur éphémère par run, réseau filtré, pas de socket Docker |
| Inférence chez les fournisseurs | Pas de GPU ; 4 vCPU et 8 à 16 Go de RAM suffisent |
| Rejouable sans clé ni base | Le mode replay n'a besoin que du binaire `cfa-api` (ou d'une page statique) |

## Vue d'ensemble

```
 Machine hôte (Linux / macOS + Docker)
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │                                                                             │
 │  127.0.0.1:8080 ──► ┌────────────┐      réseau « core » (interne)           │
 │  (navigateur, CLI)  │  cfa-api   │◄──────────────┬───────────────┐          │
 │                     └─────┬──────┘               │               │          │
 │                           │ SQL / LISTEN         │ HTTP          │ HTTP     │
 │                     ┌─────▼──────┐        ┌──────┴─────┐  ┌──────┴──────┐   │
 │                     │  postgres  │        │   runner   │  │ egress-proxy│   │
 │                     │  (pgdata)  │        │            │  │ (allowlist) │   │
 │                     └────────────┘        └──┬───┬─────┘  └──────┬──────┘   │
 │                                              │   │ /var/run/     │ Internet │
 │                         dépôt cible (bare) ◄─┘   │ docker.sock   │ (LLM)    │
 │                         worktrees (work)         │               │          │
 │                                                  ▼               │          │
 │            réseau « agents » (interne, sans route vers Internet) │          │
 │   ┌────────────┐ ┌────────────┐ ┌────────────┐                   │          │
 │   │ agent run  │ │ agent run  │ │ agent run  │ ── cfa-api ───────┘          │
 │   │ #41 back   │ │ #42 front  │ │ #43 review │ ── proxy ──► fournisseurs    │
 │   └────────────┘ └────────────┘ └────────────┘                              │
 │     éphémères : créés par le runner, supprimés à la fin du run              │
 └─────────────────────────────────────────────────────────────────────────────┘
```

## Services

*En phase 1* : `docker-compose.yml` contient `postgres`, `cfa-api`, `runner` et `cli`. `demo` (phase 3), `egress-proxy` et les conteneurs d'agents (phase 4) n'existent pas encore.

| Service | Image | Rôle | Expose | Dépend de |
| --- | --- | --- | --- | --- |
| `postgres` | `postgres:16` (officielle) | État et journal d'événements | rien (réseau `core`) | — |
| `cfa-api` | `build/api.Dockerfile` | API, SSE, dashboard ; migrations au démarrage | `127.0.0.1:8080` | `postgres` (sain) |
| `runner` | `build/runner.Dockerfile` | Ordonnance les tâches, lance les agents, git ; crée le dépôt de démo si le volume `target` est vide | rien | `cfa-api` (sain) |
| `cli` (profil `cli`) | image du runner, entrypoint `cfa` | CLI de l'humain sans Go sur le poste : `docker compose run --rm cli …` | rien | `cfa-api` (sain) |
| `demo` (profil `demo`) | image de `cfa-api` | Replay de `runs/demo.jsonl`, sans base | `127.0.0.1:8081` | — |
| `egress-proxy` | `build/proxy.Dockerfile` (Squid ou tinyproxy configuré) | Seule sortie Internet des agents, liste blanche des domaines des fournisseurs LLM | rien | — |
| agents (`cfa-agent:<version>`) | `build/agent.Dockerfile` | Un conteneur par run ; pi.dev, git, CLI `cfa`, outils du dépôt cible | rien | lancés par le runner |

## Images

*En phase 1* : seules les images `cfa-api` et du runner existent ; le runner embarque pi, `cfa-pi` et l'agent factice (voir [État actuel](#état-actuel)). `cfa-agent` et `cfa-agent-fake` relèvent de la phase 4.

Toutes les images sont construites en plusieurs étapes et tournent avec un utilisateur non root.

| Image | Base | Contenu | Taille visée |
| --- | --- | --- | --- |
| `cfa-api` | `golang:1.26` → `gcr.io/distroless/static` | Binaire statique `cfa-api` (migrations et dashboard embarqués) | ~20 Mo |
| `cfa-runner` | `golang:1.26` → `alpine` | Binaire `runner`, `git`, client Docker (API via socket) | ~40 Mo |
| `cfa-agent` | `node` (LTS, pour pi.dev) | pi.dev, `git`, CLI `cfa`, outils du dépôt cible (Go, Node, make…) ; prompts des rôles | selon le dépôt cible |
| `cfa-agent-fake` | `alpine` | CLI `cfa` + script de l'agent factice | ~15 Mo |

L'image d'agent dépend du dépôt cible (langages, outils de test). Elle est paramétrée par `build/agent.Dockerfile` et le runner la référence par `agent_image` dans `cfa.yaml`. Une image par rôle est possible (`roles.<rôle>.image`) si les outils diffèrent beaucoup.

## Réseaux

*Phase 4 : pas encore implémenté.*

| Réseau | Type | Membres | Rôle |
| --- | --- | --- | --- |
| `core` | bridge interne | `postgres`, `cfa-api`, `runner`, `egress-proxy` | Communication entre services ; `postgres` n'est joignable que d'ici |
| `agents` | bridge `internal: true` (aucune route vers Internet) | agents, `cfa-api`, `egress-proxy` | Les agents ne joignent que l'API et le proxy |
| `egress` | bridge | `egress-proxy` | Seule interface vers Internet |

Les agents reçoivent `HTTPS_PROXY=http://egress-proxy:3128` ; le proxy n'autorise que les domaines des fournisseurs déclarés (`egress.allow` dans `cfa.yaml`, par exemple `api.anthropic.com`). Un agent ne peut donc ni télécharger de dépendances arbitraires ni exfiltrer le code ailleurs que chez le fournisseur. Les dépendances du dépôt cible sont précuites dans l'image d'agent, ou servies par un cache (option : miroir Go/npm dans le réseau `core`).

## Isolement d'un run

*Phase 4 : pas encore implémenté.* Voir [État actuel](#état-actuel).

Paramètres de création du conteneur d'agent par le runner (API Docker) :

| Paramètre | Valeur |
| --- | --- |
| Utilisateur | Non root (uid 1000), `no-new-privileges` |
| Capacités | `cap-drop ALL` |
| Système de fichiers | Racine en lecture seule ; `/tmp` en tmpfs ; worktree monté en écriture |
| `contracts/` | Remonté en lecture seule (sauf rôle `contracts`) par-dessus le worktree |
| Runs `answer` et `review` | Worktree entier en lecture seule |
| Réseau | `agents` uniquement |
| Ressources | `--cpus 2`, `--memory 4g`, `--pids-limit 512` (configurables) |
| Durée | Arrêt par le runner à `per_run_timeout` ; `--rm` |
| Socket Docker | Jamais monté |
| Variables | `CFA_URL`, `CFA_TOKEN` (token du run), `CFA_RUN`, `CFA_TASK`, `CFA_ROLE`, clé du fournisseur du rôle seulement, `HTTPS_PROXY` |

Le périmètre d'écriture est donc appliqué deux fois : par les montages, puis par la vérification du diff (`git diff --name-only`) avant la relecture.

## Volumes et données

| Volume | Contenu | Monté dans | Sauvegarde |
| --- | --- | --- | --- |
| `pgdata` | Base PostgreSQL | `postgres` | `pg_dump` quotidien (voir plus bas) |
| `target`, ou `${TARGET_REPO}` (bind) | Dépôt cible bare : dépôt de démonstration créé par le runner (`CFA_INIT_TARGET=1`) si le volume est vide, ou dépôt de l'utilisateur | `runner` | Démo : non. Dépôt de l'utilisateur : par ses moyens habituels, ou poussé sur `remote` |
| `runner-token` | Token du runner (`token`) | `cfa-api` (écriture), `runner` (lecture) | Non : recréé au démarrage |
| `human-token` | Token humain (`token`) | `cfa-api` (écriture), `cli` (lecture) | Non : recréé au démarrage |
| `work` | Worktrees des runs | `runner`, agents (sous-dossier du run) | Non : recréé à partir des branches |
| `${CFA_CONFIG_FILE:-./cfa.example.yaml}` (bind, lecture seule) | Configuration | `cfa-api`, `runner` | Versionné dans le dépôt cfa |
| `./roles` (bind, lecture seule) | Prompts des rôles | `runner` (copiés dans le conteneur d'agent) | Versionné |
| `./runs` (bind) | Exports pour le replay | hôte | Versionné pour les démos |

Le runner crée les worktrees dans `work/` puis les monte dans les conteneurs d'agents. Comme le runner parle au démon Docker de l'hôte, le chemin monté est celui de l'**hôte** : `work` est donc un bind mount sur un chemin connu (`${CFA_WORK_DIR}`, par défaut `./.cfa/work`), identique dans le runner et sur l'hôte.

## Secrets

| Secret | Où | Utilisé par |
| --- | --- | --- |
| `PG_PASSWORD` | `.env` (non versionné), défaut `cfa` ; base non publiée hors du réseau `core` | `postgres`, `cfa-api` |
| Token du runner | Volume `runner-token`, écrit par `cfa-api` au démarrage (`CFA_BOOTSTRAP_DIR`) | `runner` (`CFA_TOKEN_FILE`) |
| Token humain | Volume `human-token`, écrit par `cfa-api` au démarrage ; sur le poste, `~/.config/cfa/token` pour une CLI installée | service `cli`, CLI `cfa` |
| Clés des fournisseurs LLM | `.env` | `runner`, qui transmet à chaque agent la seule clé de son rôle |
| Tokens d'agents | Générés par run, hachés en base | agents (variable d'environnement) |

Règles : aucun secret en base en clair ni dans les événements ; `.env` est listé dans `.gitignore` ; le dashboard ne voit jamais de token (cookie de session). Les deux tokens sont dans des volumes distincts : le runner, et les agents qu'il lance, ne montent jamais le token humain. Au démarrage, `cfa-api` garde un fichier de token existant s'il est encore valide en base, sinon il en écrit un nouveau (base réinitialisée, token révoqué).

## Démarrage

```
docker compose up -d --build --wait          # make up
  1. postgres            healthcheck : pg_isready
  2. cfa-api             attend postgres sain ; migrations ; tokens runner et humain (CFA_BOOTSTRAP_DIR) ;
                         healthcheck : GET /healthz
  3. runner              attend cfa-api sain ; crée le dépôt de démo si besoin (CFA_INIT_TARGET) ;
                         écoute les événements
```

Aucun fichier à préparer : sans `.env`, la pile utilise l'agent factice, `cfa.example.yaml` et le volume `target`. Ensuite :

```bash
docker compose run --rm cli dashboard        # lien de connexion au dashboard (valable 2 minutes)
docker compose run --rm cli goal new "Pagination de la liste des commandes"
scripts/compose-smoke.sh                     # bout en bout : objectif → merge (make smoke)
```

Passer à de vrais agents, dans `.env` (voir `.env.example`) :

| Variable | Effet |
| --- | --- |
| `CFA_AGENT=cfa-pi` + `ANTHROPIC_API_KEY=…` | pi.dev au lieu de l'agent factice ; remplace `agent_command` de la configuration |
| `TARGET_REPO=/chemin/vers/depot.git` | Dépôt bare de l'hôte au lieu du volume `target` (accessible en écriture par l'uid 1001) |
| `CFA_CONFIG_FILE=./cfa.yaml` | Configuration personnalisée (copie de `cfa.example.yaml`) |
| `PG_PASSWORD=…` | À fixer avant le premier démarrage (il est figé à la création de `pgdata`) |

CLI installée sur l'hôte (Go) plutôt que le service `cli` :

```bash
docker compose run --rm --entrypoint cat cli /run/cfa/token > ~/.config/cfa/token
```

## Exposition

- Par défaut, seul `cfa-api` publie un port, sur `127.0.0.1:8080`.
- Accès distant : Tailscale sur l'hôte (`tailscale serve 8080`), avec `CFA_DASHBOARD_ORIGIN` réglé sur l'adresse Tailscale. Pas d'exposition publique prévue ; si nécessaire, un reverse proxy TLS (Caddy) devant `cfa-api`, avec `X-Accel-Buffering: no` pour le SSE.

## Supervision et logs

| Signal | Source |
| --- | --- |
| Santé | `GET /healthz` (`cfa-api`) ; healthchecks compose sur chaque service |
| Logs | `docker compose logs` ; JSON avec `CFA_LOG_FORMAT=json` (méthode, route, statut, durée, rôle) |
| Activité métier | Table `events` et dashboard |
| Coûts | Par run, rôle et objectif dans le dashboard ; plafonds aussi chez chaque fournisseur |
| Traces et métriques | OpenTelemetry, **optionnel** : `OTEL_EXPORTER_OTLP_ENDPOINT` dans `.env` (ex. `http://otel-lgtm:4318`), transmis à `cfa-api` et au runner ; sans cette variable, rien n'est exporté. Détail des spans et métriques : [architecture.md](architecture.md#observabilité-opentelemetry) |
| Backend de démonstration | `docker compose --profile observability up -d` lance `grafana/otel-lgtm` (Grafana, Tempo, Prometheus, Loki dans un conteneur) : Grafana sur `127.0.0.1:3000`, OTLP sur le port 4318 du réseau `core`. Non vérifié |

## Sauvegarde et restauration

```bash
docker compose exec postgres pg_dump -U postgres -Fc cfa > backups/cfa-$(date +%F).dump   # quotidien (cron hôte)
docker compose exec -T postgres pg_restore -U postgres -d cfa --clean < backups/cfa-2026-11-06.dump
```

- Le code produit est dans les branches git du dépôt cible : la base ne contient que l'état et l'historique.
- Archivage d'un objectif : `cfa export <id>` (rejouable sans base).
- Rétention : pas de purge automatique ; `events` est la table qui grossit (diffs tronqués à 256 Kio).

## Exploitation courante

| Besoin | Commande |
| --- | --- |
| Arrêter un objectif | `cfa goal cancel <id>` |
| Arrêt d'urgence de tous les agents | `docker compose stop runner` (les conteneurs d'agents sont arrêtés par le runner ; sinon `docker ps --filter label=cfa.run` puis `docker stop`) |
| Mise à jour | `git pull && docker compose up -d --build` (migrations appliquées au démarrage de `cfa-api`) |
| Migration manuelle | `docker compose exec cfa-api cfa-api migrate status` |
| Nettoyage | Worktrees supprimés en fin d'objectif ; conteneurs en `--rm` ; images d'agent anciennes : `docker image prune --filter label=cfa.agent` |

Les conteneurs d'agents portent les labels `cfa.run=<id>`, `cfa.goal=<id>`, `cfa.role=<rôle>` : au redémarrage, le runner retrouve et arrête les orphelins, puis marque leurs runs `killed`.

## CI (GitHub Actions)

| Job | Étapes |
| --- | --- |
| `lint` | `go vet`, `gofmt`, `staticcheck`, `sqlc diff` (le code généré est à jour), lint de `docs/openapi.yaml` (Redocly) |
| `test` | `go test -race ./...` avec un service `postgres:16` (`CFA_TEST_DATABASE_URL`) |
| `compose` | `docker compose up --wait` sans `.env`, puis `scripts/compose-smoke.sh` (agent factice : un objectif du planner au merge, approbations comprises) et démo rejouée |
| `replay` | Rejoue `runs/*.jsonl` dans le lecteur et vérifie l'état final ; captures du dashboard (`scripts/screenshots.sh`) en artefacts |
| `images` | Construction des images ; publication sur GHCR pour les tags |
| `pages` | Publication du dashboard en page statique avec la démo (GitHub Pages) |

## Environnements

| Environnement | Base | Agents | Usage |
| --- | --- | --- | --- |
| Développement | PostgreSQL local, embarqué (tests) ou compose | Agent factice | Écrire cfa |
| Démo | Aucune | Aucun | `docker compose --profile demo up demo`, `cfa-api --replay` ou page statique |
| Essai | Compose (défauts) | Agent factice, dépôt de démo | `docker compose up`, sans `.env` |
| Utilisation | Compose | pi.dev | Travail réel sur un dépôt cible |
| CI | Service `postgres:16` ; compose (job `compose`) | Agent factice | Tests, e2e |

## État actuel

Les agents sont pour l'instant lancés **en sous-processus dans le conteneur du runner** : l'image du runner contient pi (Node 22, `@mariozechner/pi-coding-agent`), l'adaptateur `cfa-pi`, l'agent factice et [build/pi/models.json](../build/pi/models.json) (tarifs des modèles récents). Conséquence : un agent a les mêmes droits réseau et fichiers que le runner. L'isolement décrit plus haut (conteneur éphémère par run, réseau `agents`, proxy de sortie) arrive avec le `DockerLauncher` ; il n'y a donc pas encore de proxy ni de réseau `agents` dans `docker-compose.yml`. Tant que ce n'est pas fait, réserver pi.dev à des dépôts cibles de confiance.

## Fichiers

| Fichier | État |
| --- | --- |
| `docker-compose.yml` (postgres, cfa-api, runner, `cli`, `demo`, healthchecks, démarrage sans `.env`) | ✅ vérifié : `make up` démarre la pile sans `.env`, healthchecks au vert |
| `scripts/compose-smoke.sh` (objectif → merge sous compose) | ✅ vérifié : `make smoke` mène un objectif jusqu'au merge, approbation comprise (agent factice) |
| `build/api.Dockerfile`, `build/runner.Dockerfile` (avec pi et `cfa-pi`) | ✅ construits par `make up` ; `cfa-pi` reste à valider par un run réel avec une clé API |
| `build/pi/models.json` (tarifs et fenêtres des modèles récents pour pi) | ✅ vérifié avec pi 0.73.1 |
| `.env.example`, `cfa.example.yaml` | ✅ (`cfa.example.yaml` chargé par un test) |
| `.github/workflows/ci.yml` (tests avec PostgreSQL, dashboard, contrats, compose) | ✅ vert sur GitHub Actions |
| `build/agent.Dockerfile` (pi.dev), `build/proxy.Dockerfile`, réseau `agents` | à faire, avec le `DockerLauncher` |
| `cmd/cfa-devdb` | ✅ PostgreSQL local sans Docker, pour le développement |
