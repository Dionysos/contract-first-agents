# cfa — Spécifications du bus, de l'API et du runner

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). Une section qui commence par *Phase 2*, *Phase 3*, *Phase 4* ou *Prévu* n'est pas encore implémentée ; une note *En phase 1* signale ce qui diffère aujourd'hui. La [feuille de route](../README.fr.md#feuille-de-route) indique ce qui est déjà en place.

## Contexte et objectifs

Un développeur pilote une équipe d'agents IA spécialisés (planification, contrats, back, front, relecture). Il formule des objectifs ; les agents les découpent en tâches, codent chacun dans leur périmètre et se coordonnent par messages. Le développeur n'intervient qu'aux points de validation.

`cfa` est la brique centrale : un bus de messages et de tâches, une API HTTP, un flux temps réel, une CLI et un runner qui lance les agents. Les agents tournent avec pi.dev, ce qui laisse le choix du modèle pour chaque rôle.

Objectifs :

- Démontrer une coordination multi-agents lisible et traçable, visible en direct dans un dashboard.
- Garder les contrats de données comme seule interface entre front et back (contract-first).
- Garder l'humain décisionnaire : aucun agent ne merge, les décisions sensibles passent par une approbation.
- Maîtriser les coûts : budget par objectif, limite d'échanges par fil, arrêt automatique.
- Pouvoir rejouer un run enregistré sans clé API ni LLM (mode replay).
- Rester simple : Go, PostgreSQL, Docker, pas de framework d'orchestration.

## Concepts

| Concept | Définition |
| --- | --- |
| Objectif (`goal`) | Demande formulée par l'humain, en langage naturel. Point d'entrée de tout le travail. |
| Tâche (`task`) | Unité de travail confiée à un rôle, avec ses dépendances. Une tâche = une branche git. |
| Rôle | Profil d'agent : prompt, modèle, chemins modifiables, contrôles à passer. |
| Run | Une exécution de pi.dev pour un rôle, sur une tâche ou pour répondre à un message. |
| Fil (`thread`) et message | Échange entre rôles, rattaché à un objectif et éventuellement à une tâche. |
| Approbation | Décision demandée à l'humain : changement de contrat, merge, escalade, dépassement de budget. |
| Événement (`event`) | Trace immuable de tout ce qui se passe. Alimente le temps réel, l'historique et le replay. |

## Description fonctionnelle

### Rôles

| Rôle | Modifie | Lit | Mission |
| --- | --- | --- | --- |
| `planner` | rien | tout le dépôt | Découpe un objectif en tâches ordonnées et les assigne aux rôles |
| `contracts` | `contracts/` | tout le dépôt | Maintient les contrats (OpenAPI, JSON Schema), traite les `contract_change` |
| `back` | `api/`, `db/` | tout le dépôt | Implémente le backend conforme aux contrats |
| `front` | `web/` | tout le dépôt | Implémente le front conforme aux contrats |
| `reviewer` | rien | tout le dépôt | Relit chaque tâche terminée, approuve ou demande des corrections |

Les rôles et leurs chemins sont déclarés dans la configuration (`cfa.yaml`) : on peut en ajouter (QA, docs, sécurité) sans changer le code. Les chemins ci-dessus sont la convention par défaut du dépôt cible.

Participants non agents : `human` (le développeur), `runner` (le système) et `ci` (contrôles automatiques).

### Types de messages

*Phase 2 : pas encore implémenté.*

| Type | Usage | Réponse attendue |
| --- | --- | --- |
| `question` | Demander une précision à un autre rôle | `answer` |
| `answer` | Répondre dans le même fil | aucune |
| `contract_change` | Demander une modification de `contracts/` au rôle `contracts` | `answer` (acceptée, refusée, à discuter) |
| `notify` | Annoncer un changement (contrat publié, contrôle en échec) | aucune |
| `escalate` | Remonter un sujet à l'humain ; crée une approbation | décision humaine |

### Cas d'usage

*En phase 1* : seuls les cas 1 et 5 existent, sans reviewer (le runner vérifie périmètre et contrôles). Les cas 2, 3, 4 et 6 relèvent de la phase 2.

1. **Nouvel objectif.** L'humain soumet « ajouter la pagination à la liste des commandes ». Le `planner` crée trois tâches : contrat (`contracts`), implémentation (`back`, dépend du contrat), affichage (`front`, dépend du contrat). Le runner lance chaque tâche dès que ses dépendances sont terminées.
2. **Besoin d'un champ en cours de route.** L'agent `front` envoie un `contract_change` à `contracts`, met sa tâche en attente et s'arrête. Le runner réveille `contracts`, qui modifie le schéma. Si le changement est cassant, une approbation est demandée à l'humain. Le runner relance ensuite la tâche `front` avec la réponse.
3. **Question entre rôles.** `front` demande à `back` si un champ peut être nul. Le runner lance un run de réponse de `back`, en lecture seule, qui répond à partir du code.
4. **Relecture.** Chaque tâche terminée passe par le `reviewer`. S'il demande des corrections, la tâche repart chez son rôle avec ses remarques ; au-delà de 3 tentatives, escalade.
5. **Fin d'objectif.** Toutes les tâches sont relues : le runner fusionne leurs branches dans la branche de l'objectif, lance les contrôles globaux et demande l'approbation de merge. L'humain approuve, le runner merge dans `main` (ou ouvre une PR sur le dépôt distant).
6. **Boucle ou dépassement.** Au-delà de 5 échanges dans un fil, ou si le budget de l'objectif est atteint, le bus bloque et crée une escalade.

### Règles de gestion

*En phase 1* : pas de fils, de tentatives ni de budget ; une tâche en échec fait échouer l'objectif, l'historique n'est pas encore exportable (phase 3).

- Un agent ne modifie que les chemins de son rôle ; le runner le vérifie avant de passer une tâche en relecture.
- Toute modification de `contracts/` est faite par le rôle `contracts`.
- Un fil est limité à 5 échanges (`hop`) ; au-delà, escalade.
- Une tâche est limitée à 3 tentatives ; au-delà, escalade.
- Chaque objectif a un budget (en dollars) ; chaque run a une limite de tokens et de durée.
- L'émetteur d'un message ou d'une action est toujours déduit du token, jamais du contenu.
- Aucun agent ne merge dans `main` : seule une approbation humaine déclenche le merge.
- Toute écriture produit un événement ; l'historique complet reste consultable et exportable.

## Description technique

### Architecture

```
                         Navigateur (dashboard)        Terminal (CLI cfa)
                               ▲   │ objectifs,               │
                          SSE  │   │ approbations             │
                               │   ▼                          ▼
                    ┌──────────────────────────────────────────────┐
                    │ cfa-api (Go, net/http + pgx)           │
                    │ REST · SSE · tokens · hop · budget · dashboard│
                    └───────────────┬──────────────────────▲───────┘
                                    │ SQL                   │ CLI cfa
                    ┌───────────────▼────────┐              │ (depuis les conteneurs)
                    │ PostgreSQL 16          │              │
                    │ état + table events    │              │
                    │ LISTEN/NOTIFY          │              │
                    └───────────────┬────────┘              │
                                    │ NOTIFY                │
                    ┌───────────────▼──────────────────────┐│
                    │ runner (Go)                          ││
                    │ ordonnance les tâches, crée worktrees,││
                    │ lance 1 conteneur pi.dev par run,    ││
                    │ vérifie périmètres, contrôles, merge ││
                    └───────┬─────────┬─────────┬──────────┘│
                            ▼         ▼         ▼           │
                       [planner] [contracts] [back] [front] [reviewer]
                        conteneurs éphémères pi.dev ────────┘
```

L'API est la seule à écrire en base. Le runner et les agents passent par elle. Le dashboard ne fait que lire le flux d'événements et appeler l'API pour les actions humaines.

### Composants

| Composant | Technologie | Rôle |
| --- | --- | --- |
| `postgres` | PostgreSQL 16 | État (objectifs, tâches, messages, runs, approbations) et journal d'événements ; `LISTEN/NOTIFY` |
| `cfa-api` | Go, `net/http` + `pgx` | API REST, flux SSE, contrôles, sert le dashboard (fichiers embarqués) |
| `runner` | Go | Ordonnancement, worktrees git, conteneurs d'agents, vérifications, merge |
| `cfa` | Binaire Go (même dépôt) | CLI pour l'humain et pour les agents ; export et replay |
| Image agent | Docker : pi.dev + git + outils du dépôt cible | Exécution isolée d'un run |
| Dashboard | Page statique embarquée dans l'API | Vue temps réel, soumission d'objectifs, approbations, replay |

### Modèle de données

```sql
CREATE TABLE goals (
  id           BIGSERIAL PRIMARY KEY,
  title        TEXT NOT NULL,
  description  TEXT NOT NULL,
  status       TEXT NOT NULL CHECK (status IN
                 ('planning','running','awaiting_approval','done','failed','cancelled')),
  budget_usd   NUMERIC(10,2) NOT NULL,
  spent_usd    NUMERIC(10,4) NOT NULL DEFAULT 0,
  branch       TEXT NOT NULL,                 -- goal/<id>
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at  TIMESTAMPTZ
);

CREATE TABLE tasks (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT NOT NULL REFERENCES goals(id),
  role         TEXT NOT NULL,
  title        TEXT NOT NULL,
  spec         TEXT NOT NULL,
  status       TEXT NOT NULL CHECK (status IN
                 ('todo','running','blocked','review','changes_requested','done','failed')),
  branch       TEXT,                          -- task/<g>-<id>-<role>
  attempts     INT NOT NULL DEFAULT 0,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE task_deps (
  task_id      BIGINT REFERENCES tasks(id),
  depends_on   BIGINT REFERENCES tasks(id),
  PRIMARY KEY (task_id, depends_on)
);

CREATE TABLE threads (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT NOT NULL REFERENCES goals(id),
  task_id      BIGINT REFERENCES tasks(id),
  subject      TEXT NOT NULL,
  hop          INT NOT NULL DEFAULT 0,
  status       TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed','escalated'))
);

CREATE TABLE messages (
  id           BIGSERIAL PRIMARY KEY,
  thread_id    BIGINT NOT NULL REFERENCES threads(id),
  from_role    TEXT NOT NULL,
  to_role      TEXT NOT NULL,
  type         TEXT NOT NULL CHECK (type IN
                 ('question','answer','contract_change','notify','escalate')),
  body         JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  read_at      TIMESTAMPTZ
);
CREATE INDEX ON messages (to_role) WHERE read_at IS NULL;

CREATE TABLE runs (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT NOT NULL REFERENCES goals(id),
  task_id      BIGINT REFERENCES tasks(id),
  kind         TEXT NOT NULL CHECK (kind IN ('task','answer','review','plan')),
  role         TEXT NOT NULL,
  model        TEXT NOT NULL,
  status       TEXT NOT NULL CHECK (status IN ('running','succeeded','failed','killed')),
  tokens_in    INT NOT NULL DEFAULT 0,
  tokens_out   INT NOT NULL DEFAULT 0,
  cost_usd     NUMERIC(10,4) NOT NULL DEFAULT 0,
  started_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at  TIMESTAMPTZ
);

CREATE TABLE approvals (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT NOT NULL REFERENCES goals(id),
  task_id      BIGINT REFERENCES tasks(id),
  kind         TEXT NOT NULL CHECK (kind IN ('contract_change','merge','escalation','budget')),
  payload      JSONB NOT NULL,
  status       TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
  comment      TEXT,
  decided_at   TIMESTAMPTZ
);

CREATE TABLE tokens (
  hash         TEXT PRIMARY KEY,
  role         TEXT NOT NULL,                 -- human, runner, ci ou un rôle d'agent
  run_id       BIGINT REFERENCES runs(id),    -- token d'agent lié à un run
  expires_at   TIMESTAMPTZ
);

CREATE TABLE events (
  id           BIGSERIAL PRIMARY KEY,
  goal_id      BIGINT,
  type         TEXT NOT NULL,
  actor        TEXT NOT NULL,
  payload      JSONB NOT NULL,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Un trigger sur `events` exécute `pg_notify('cfa_events', NEW.id::text)`.

### Événements

Chaque écriture de l'API modifie l'état **et** insère un événement dans la même transaction. Les événements sont la source du temps réel, de l'historique et du replay.

Règle clé : **un événement contient tout ce qu'il faut pour l'afficher** (titre de la tâche, contenu du message, coût du run…). Le dashboard construit son état uniquement en lisant les événements dans l'ordre, sans autre requête. C'est ce qui rend le replay trivial.

| Type | Émis quand |
| --- | --- |
| `goal.created`, `goal.status_changed` | Objectif soumis, changement de statut |
| `task.created`, `task.status_changed` | Tâche créée par le planner, transition de statut |
| `task.reviewed` | Relecture enregistrée (verdict et commentaire) |
| `run.started`, `run.progress`, `run.finished` | Lancement d'un agent, étapes notables (outil utilisé, fichier modifié), fin avec tokens, coût et diffstat |
| `message.sent`, `message.rejected` | Message accepté ; refusé (hop, limite, destinataire invalide) avec la raison |
| `approval.requested`, `approval.resolved` | Demande de décision humaine, décision prise |
| `check.finished` | Résultat d'un contrôle (lint de contrat, tests) |
| `budget.warning`, `budget.exceeded` | 80 % puis 100 % du budget d'un objectif |
| `merge.done` | Branche de l'objectif mergée ou PR ouverte |

### API

Référence complète : [openapi.yaml](openapi.yaml) (routes, corps, codes) et [api.md](api.md) (droits, cycles de statut, garde-fous, catalogue des événements, SSE, replay).

Authentification par `Authorization: Bearer <token>`. Le rôle appelant est déduit du token. Les tokens d'agents sont émis par le runner pour un run et expirent à sa fin.

| Méthode | Route | Appelant | Rôle |
| --- | --- | --- | --- |
| POST | `/goals` | human | Soumettre un objectif (voir ci-dessous) |
| GET | `/config/public` | human | Défauts et limites pour le formulaire (budget, branches du dépôt cible) |
| POST | `/auth/code` | human (token) | Créer un code de connexion au dashboard, à usage unique, valable 2 minutes |
| GET | `/login?code=` | — | Échanger le code contre un cookie de session |
| GET | `/goals`, `/goals/{id}` | tous | Lister, détail avec tâches, fils et approbations |
| POST | `/goals/{id}/cancel` | human | Annuler ; le runner arrête les runs en cours |
| POST | `/goals/{id}/status` | runner | `planning → running`, replanification, échec |
| POST | `/goals/{id}/merged` | runner | Enregistrer le merge ou la PR ; objectif `done` |
| POST | `/tasks` | planner, human | Créer une tâche (`goal_id`, `role`, `title`, `spec`, `depends_on`) |
| GET | `/tasks/{id}` | tous | Détail : spec, relectures, message attendu, run en cours |
| POST | `/tasks/{id}/status` | agent de la tâche, runner | Changer le statut (`blocked`, `review`, `failed`…) |
| POST | `/tasks/{id}/review` | reviewer | `approve` ou `changes` avec commentaire |
| POST | `/messages` | agents, ci, human | Envoyer (`to`, `type`, `body`, `thread_id` ou `subject`) |
| GET | `/inbox?unread=true` | agents | Messages reçus par le rôle |
| POST | `/messages/{id}/ack` | agents | Marquer comme lu |
| GET | `/threads/{id}` | tous | Historique d'un fil |
| GET | `/approvals?status=pending` | human | Décisions en attente |
| POST | `/approvals/{id}/decision` | human | `approve` ou `reject`, avec commentaire |
| POST | `/approvals` | runner | Demander une approbation (`contract_change`, `merge`, escalade) |
| POST | `/runs`, `/runs/{id}/finish` | runner | Déclarer un run (renvoie le token d'agent), enregistrer son résultat et sa consommation |
| POST | `/runs/{id}/progress` | runner | Publier une étape (`run.progress`) |
| POST | `/checks` | runner, ci | Publier un résultat de contrôle (`check.finished`) |
| GET | `/events?goal=&since=` | tous | Flux SSE (voir ci-dessous) |
| GET | `/events/export?goal=` | human | Export JSON Lines pour le replay |
| GET | `/healthz` | — | Supervision |
| GET | `/` | — | Dashboard |

L'humain s'authentifie par token (CLI) ou par cookie de session (dashboard). Avec le cookie, les routes d'écriture exigent un en-tête `Origin` égal à l'adresse du dashboard.

**`POST /goals`**

```json
{
  "title": "Pagination de la liste des commandes",
  "description": "La liste doit être paginée (20 par page). Pas de breaking change. Tests attendus.",
  "budget_usd": 5,
  "base_branch": "main"
}
```

- `title` : 5 à 120 caractères ; `description` : 20 à 5 000 caractères ; `budget_usd` optionnel, entre 0,50 et `budget.max_goal_usd` ; `base_branch` optionnelle, doit exister.
- En-tête `Idempotency-Key` accepté : une même clé dans les 24 heures renvoie l'objectif déjà créé.
- `201` : l'objectif créé (`id`, `status: "planning"`, `branch`) ; un événement `goal.created` est publié et le runner lance le `planner`.
- `422` : erreurs de validation par champ, `{"errors": {"title": "5 caractères minimum"}}`.
- `403` en mode replay ou sans droit humain.

Contrôles sur `POST /messages` : type valide ; `hop` du fil + 1 au maximum à 5, sinon refus et escalade ; limite de messages par rôle et par objectif ; `contract_change` uniquement vers `contracts`. Contrôles sur `POST /tasks/{id}/status` : seul l'agent du run en cours de cette tâche peut la modifier ; transitions autorisées uniquement.

### Flux temps réel (SSE)

SSE (Server-Sent Events) est une connexion HTTP qui reste ouverte et sur laquelle le serveur envoie des événements texte au fil de l'eau. Le sens est unique (serveur vers client), ce qui suffit ici : les actions du client passent par les routes REST. Le navigateur le gère nativement (`EventSource`), avec reconnexion automatique.

```
id: 1284
event: message.sent
data: {"goal_id":7,"thread_id":31,"from":"front","to":"back","type":"question","body":{"text":"status peut-il être null ?"}}
```

- L'API fait `LISTEN cfa_events`, relit l'événement par son id et le diffuse aux clients abonnés (filtre par objectif optionnel).
- `id` est l'id de l'événement : à la reconnexion, le navigateur renvoie `Last-Event-ID` et l'API renvoie ce qui a été manqué. `?since=0` rejoue tout l'historique.

### CLI `cfa`

La CLI lit `CFA_URL` et `CFA_TOKEN` (ou `CFA_TOKEN_FILE`, utilisé sous docker compose). Dans un conteneur d'agent, le runner fournit aussi `CFA_RUN` et `CFA_TASK`.

Pour l'humain :

```bash
cfa dashboard                      # ouvre le dashboard avec une session
cfa goal new "Ajouter la pagination à la liste des commandes" --budget 5
cfa goal list
cfa goal show 7
cfa approvals
cfa approve 12 --comment "OK pour le champ optionnel"
cfa reject 13 "Pas de breaking change sur cette version"
cfa watch --goal 7                 # événements en direct dans le terminal
cfa export 7 > runs/goal-7.jsonl
cfa replay runs/goal-7.jsonl --speed 4
```

Pour les agents :

```bash
cfa task create --role back --title "Pagination API" --spec-file spec.md --after 41
cfa task show                       # tâche courante, remarques de relecture
cfa task block --wait 88            # attendre la réponse au message 88, puis quitter
cfa task done "Endpoint paginé, tests ajoutés"
cfa send --to back --type question "Le champ status peut-il être null ?"
cfa reply 88 "Non, valeur par défaut 'draft'."
cfa inbox --unread
cfa thread 31
cfa review approve "RAS"            # reviewer
cfa review changes "Manque le test de la page vide"
```

### Runner

Le runner est un service Go qui écoute les événements et pilote les agents. Il n'a aucune intelligence propre : il applique des règles.

**Boucle principale :**

1. **Objectif créé** : lance un run `plan` du `planner`, qui crée les tâches via la CLI.
2. **Tâche prête** (statut `todo`, dépendances terminées, place disponible selon `concurrency`) : lance un run `task`.
3. **Message reçu par un rôle sans run actif** : lance un run `answer` de ce rôle, en lecture seule sur la branche de l'objectif.
4. **Réponse reçue pour une tâche `blocked`** : relance la tâche avec l'historique du fil.
5. **Tâche en `review`** : lance un run `review` du `reviewer`.
6. **Toutes les tâches `done`** : fusionne les branches dans `goal/<id>`, lance les contrôles globaux, crée l'approbation `merge`.
7. **Approbation `merge` acceptée** : merge dans la branche de base, ou pousse et ouvre une PR si un dépôt distant est configuré.

**Déroulé d'un run :**

1. Crée le worktree : `git worktree add <work>/task-<id> -b task/<g>-<id>-<role> goal/<g>`.
2. Émet un token lié au run et au rôle.
3. Lance un conteneur éphémère de l'image du rôle : worktree monté en écriture, `contracts/` remonté en lecture seule sauf pour `contracts`, variables d'environnement (URL, token, clé du fournisseur LLM).
4. Exécute pi.dev en mode non interactif avec un prompt composé du prompt du rôle, de la spec de la tâche, des remarques de relecture et des messages non lus. Le runner lance pour cela l'adaptateur `cfa-pi`, qui exécute `pi --mode json` et traduit le flux JSON en étapes, tokens et coût ([architecture.md](architecture.md#adaptateur-pidev-cmdcfa-pi-internalpiagent)).
5. Surveille la durée et les tokens ; arrête le conteneur en cas de dépassement.
6. À la fin : vérifie que les fichiers modifiés (`git diff --name-only`) sont dans les chemins du rôle, lance les contrôles du rôle, commite si besoin, passe la tâche en `review` ou en échec, publie `run.finished` avec coût et diffstat.

La commande d'agent est configurable (`agent_command`) : on peut remplacer pi.dev par un **agent factice** scripté, pour les tests et le développement sans LLM.

### Mode replay

*Phase 3 : pas encore implémenté.*

Le replay rejoue un run enregistré, à l'identique dans le dashboard, sans base de données, sans runner et sans LLM.

- **Enregistrement** : `cfa export <goal>` produit un fichier JSON Lines des événements de l'objectif, dans l'ordre, avec leurs horodatages. Les événements `run.finished` incluent le diff (tronqué au-delà d'une taille fixée) pour montrer le code produit.
- **Replay par l'API** : `cfa-api --replay runs/goal-7.jsonl --speed 4` sert le dashboard et réémet les événements en SSE en respectant les intervalles d'origine (accélérés). Les routes d'écriture sont désactivées.
- **Replay statique** : le dashboard sait aussi charger un fichier `.jsonl` directement dans le navigateur. Il peut donc être publié en page statique (GitHub Pages) avec quelques runs d'exemple, consultables sans rien installer.
- **Contrôles du lecteur** : lecture, pause, vitesse (×1, ×4, ×16), aller à un événement.

Ce mode fonctionne parce que le dashboard ne dépend que du flux d'événements (voir « Événements »).

### Sécurité et garde-fous

*En phase 1* : les agents tournent en sous-processus dans le conteneur du runner, sans conteneur éphémère ni réseau filtré (phase 4) ; les limites de fils, de tentatives et de budget arrivent en phase 2.

- Tokens hachés en base ; tokens d'agents à durée de vie limitée au run, avec un seul rôle.
- Agents isolés dans des conteneurs éphémères : pas d'accès au socket Docker, réseau limité à l'API cfa et au fournisseur LLM, seul le worktree de la tâche est monté.
- Périmètres appliqués deux fois : montage en lecture seule et vérification du diff par le runner.
- Limites : 5 échanges par fil, 3 tentatives par tâche, messages par rôle et par objectif, budget par objectif, tokens et durée par run.
- Le runner a accès au socket Docker pour lancer les agents : c'est le composant de confiance, il n'exécute aucun contenu produit par un LLM hors des conteneurs.
- Dashboard : session par cookie `HttpOnly` `SameSite=Strict` obtenue via un code à usage unique émis par la CLI ; vérification de `Origin` sur les écritures ; aucun token dans le navigateur.
- API et dashboard écoutent sur `127.0.0.1` par défaut ; exposition via Tailscale en option.
- Clés des fournisseurs LLM dans un fichier `.env` local, jamais en base ni dans les événements.

### Déploiement

Esquisse d'origine (cible avec le socket Docker pour les conteneurs d'agents). Le fichier en vigueur est [docker-compose.yml](../docker-compose.yml) : il démarre sans `.env`, crée les tokens au démarrage et un dépôt cible de démo ([ADR 0015](adr/0015-docker-compose-d-abord.md), [infrastructure.md](infrastructure.md#démarrage)).

```yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: cfa
      POSTGRES_PASSWORD: ${PG_PASSWORD}
    volumes: [pgdata:/var/lib/postgresql/data]

  cfa-api:
    build: { context: ., dockerfile: build/api.Dockerfile }
    environment:
      DATABASE_URL: postgres://postgres:${PG_PASSWORD}@postgres/cfa
    ports: ["127.0.0.1:8080:8080"]
    depends_on: [postgres]

  runner:
    build: { context: ., dockerfile: build/runner.Dockerfile }
    environment:
      CFA_URL: http://cfa-api:8080
      CFA_TOKEN: ${RUNNER_TOKEN}
    env_file: .env                      # clés des fournisseurs LLM
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ${TARGET_REPO}:/srv/target.git
      - work:/srv/work
      - ./cfa.yaml:/etc/cfa.yaml:ro
    depends_on: [cfa-api]

volumes:
  pgdata:
  work:
```

L'image d'agent (`build/agent.Dockerfile`) contient pi.dev, git, la CLI `cfa` et les outils du dépôt cible.

## Évolutions possibles

- Plusieurs humains : un humain n'est qu'un participant de plus, avec son token et ses approbations.
- Objectifs créés depuis les issues GitHub ou GitLab (label dédié).
- Reprise de session pi.dev entre deux runs d'une même tâche, au lieu d'un prompt reconstruit.
- Traces OpenTelemetry sur l'API et le runner.
- Rôles supplémentaires : QA (tests end-to-end), sécurité, documentation.
