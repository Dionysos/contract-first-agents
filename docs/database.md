# Base de données

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). Une section qui commence par *Phase 2*, *Phase 3*, *Phase 4* ou *Prévu* n'est pas encore implémentée ; une note *En phase 1* signale ce qui diffère aujourd'hui. La [feuille de route](../README.fr.md#feuille-de-route) indique ce qui est déjà en place.

PostgreSQL 16. Le schéma est défini par les migrations de [migrations/](../migrations/) ; ce document les explique. En cas d'écart, la migration fait foi.

## Principes

- **L'API est seule à écrire.** Le runner, la CLI et le dashboard passent par elle.
- **État + journal.** Les tables d'état (`goals`, `tasks`, …) donnent la situation courante ; la table `events` garde tout l'historique. Chaque écriture modifie l'état **et** insère un événement dans la même transaction.
- **Journal immuable.** Un trigger refuse `UPDATE` et `DELETE` sur `events`.
- **Les règles sont dans la base quand c'est possible** : énumérations par `CHECK`, longueurs, unicité des approbations en attente, un seul run actif par tâche. Le code Go les vérifie aussi pour renvoyer des erreurs lisibles ; la base est le dernier filet.
- **Pas de suppression.** Aucune donnée métier n'est supprimée : les objectifs finis restent consultables et exportables. Seules les tables techniques (`sessions`, `login_codes`, `idempotency_keys`) sont purgées.
- **Branches calculées.** `goals.branch` et `tasks.branch` sont des colonnes générées : impossible de les désynchroniser de l'id.

## Vue d'ensemble

```
goals 1──n tasks n──n tasks          (task_deps)
  │          │
  │          └──1 threads 1──n messages ──reply_to──► messages
  │                                          ▲
  │          tasks.wait_for ─────────────────┘
  ├──n threads
  ├──n runs 1──n tokens               (tokens d'agents)
  ├──n approvals
  ├──1 idempotency_keys
  └──n events                         (goal_id nullable)

sessions, login_codes                 (dashboard, sans lien)
```

## Tables

*En phase 1* : le schéma réel est [migrations/00001_init.sql](../migrations/00001_init.sql). Il n'a ni `threads`, ni `messages`, ni `idempotency_keys`, ni colonnes de budget, de coût ou de tentatives.

### `goals` — objectifs

| Colonne | Type | Règle |
| --- | --- | --- |
| `id` | `BIGSERIAL` | Clé |
| `title` | `TEXT` | 5 à 120 caractères |
| `description` | `TEXT` | 20 à 5 000 caractères, Markdown |
| `status` | `TEXT` | `planning` (défaut), `running`, `awaiting_approval`, `done`, `failed`, `cancelled` |
| `budget_usd` | `NUMERIC(10,2)` | > 0 ; bornes de configuration vérifiées par l'API |
| `spent_usd` | `NUMERIC(10,4)` | Somme des `runs.cost_usd`, mise à jour par `POST /runs/{id}/finish` |
| `budget_warned` | `BOOLEAN` | `budget.warning` déjà publié (une seule fois par objectif) |
| `base_branch` | `TEXT` | Branche de base du dépôt cible |
| `branch` | `TEXT` généré | `goal/<id>` |
| `created_at`, `finished_at` | `TIMESTAMPTZ` | `finished_at` renseigné au passage en statut terminal |

### `tasks` — tâches

| Colonne | Type | Règle |
| --- | --- | --- |
| `id` | `BIGSERIAL` | Clé |
| `goal_id` | `BIGINT` | → `goals` |
| `role` | `TEXT` | Rôle d'agent de `cfa.yaml` (vérifié par l'API) |
| `title` | `TEXT` | 5 à 120 caractères |
| `spec` | `TEXT` | 1 à 20 000 caractères |
| `status` | `TEXT` | `todo` (défaut), `running`, `blocked`, `review`, `changes_requested`, `done`, `failed` |
| `branch` | `TEXT` généré | `task/<g>-<id>-<role>` |
| `attempts` | `INT` | Runs `task` lancés depuis `todo` ou `changes_requested` |
| `wait_for` | `BIGINT` | → `messages` ; message attendu en statut `blocked` |
| `summary` | `TEXT` | Résumé donné par l'agent au passage en `review` |
| `created_at`, `updated_at` | `TIMESTAMPTZ` | `updated_at` mis à jour à chaque changement de statut |

### `task_deps` — dépendances

`(task_id, depends_on)`, clé composite, les deux → `tasks`. Une tâche ne dépend pas d'elle-même (`CHECK`). L'API n'accepte que des dépendances déjà existantes du même objectif : le graphe ne peut pas contenir de cycle.

### `threads` — fils

*Phase 2 : pas encore implémenté.*

| Colonne | Type | Règle |
| --- | --- | --- |
| `goal_id` | `BIGINT` | → `goals` |
| `task_id` | `BIGINT` | → `tasks`, optionnel |
| `subject` | `TEXT` | 3 à 200 caractères |
| `hop` | `INT` | Nombre d'échanges comptés ; au plus `limits.max_hop` (vérifié par l'API) |
| `status` | `TEXT` | `open` (défaut), `closed`, `escalated` |

### `messages`

*Phase 2 : pas encore implémenté.*

| Colonne | Type | Règle |
| --- | --- | --- |
| `thread_id` | `BIGINT` | → `threads` |
| `goal_id` | `BIGINT` | → `goals` ; recopié du fil, pour compter le quota et lire la boîte de réception sans jointure |
| `from_role`, `to_role` | `TEXT` | Émetteur déduit du token |
| `type` | `TEXT` | `question`, `answer`, `contract_change`, `notify`, `escalate` |
| `body` | `JSONB` | Objet avec au moins une clé `text` |
| `reply_to` | `BIGINT` | → `messages`, optionnel |
| `read_at` | `TIMESTAMPTZ` | Première lecture (`ack`) |

### `runs` — exécutions d'agents

| Colonne | Type | Règle |
| --- | --- | --- |
| `goal_id`, `task_id` | `BIGINT` | `task_id` obligatoire pour `task` et `review` |
| `kind` | `TEXT` | `task`, `answer`, `review`, `plan` |
| `role`, `model` | `TEXT` | Modèle au format `<fournisseur>/<modèle>` |
| `status` | `TEXT` | `running` (défaut), `succeeded`, `failed`, `killed` |
| `tokens_in`, `tokens_out` | `INT` | Consommation déclarée par le runner |
| `cost_usd` | `NUMERIC(10,4)` | Coût du run |
| `error` | `TEXT` | Cause d'un échec ou d'un arrêt |
| `started_at`, `finished_at` | `TIMESTAMPTZ` | |

Le diff, les fichiers et le diffstat ne sont pas stockés ici : ils sont dans l'événement `run.finished`, la seule source pour le dashboard et le replay.

### `approvals`

| Colonne | Type | Règle |
| --- | --- | --- |
| `goal_id`, `task_id` | `BIGINT` | `task_id` optionnel |
| `kind` | `TEXT` | `contract_change`, `merge`, `escalation`, `budget` |
| `payload` | `JSONB` | Contenu propre au type (voir [api.md](api.md#contenu-payload)) |
| `status` | `TEXT` | `pending` (défaut), `approved`, `rejected` |
| `comment` | `TEXT` | Commentaire de la décision |
| `created_at`, `decided_at` | `TIMESTAMPTZ` | `decided_at` renseigné si et seulement si la décision est prise |

### `tokens`

| Colonne | Type | Règle |
| --- | --- | --- |
| `hash` | `TEXT` | SHA-256 hexadécimal du token ; le token en clair n'est jamais stocké |
| `role` | `TEXT` | `human`, `runner`, `ci`, ou rôle d'agent (alors `run_id` obligatoire) |
| `run_id` | `BIGINT` | → `runs` pour un token d'agent |
| `label` | `TEXT` | Libellé libre (« laptop », « jenkins ») |
| `created_at`, `expires_at`, `revoked_at` | `TIMESTAMPTZ` | Un token est valide s'il n'est ni expiré ni révoqué |

### Tables techniques

| Table | Contenu | Purge |
| --- | --- | --- |
| `sessions` | Hash du cookie `cfa_session`, expiration (12 h) | Sessions expirées, au démarrage puis toutes les heures |
| `login_codes` | Hash du code de connexion, expiration (2 min), date d'utilisation | Codes de plus d'une heure |
| `idempotency_keys` | Clé, empreinte du corps, objectif créé | Clés de plus de 24 h |

### `events` — journal

| Colonne | Type | Règle |
| --- | --- | --- |
| `id` | `BIGSERIAL` | Ordre global des événements ; sert d'`id` SSE |
| `goal_id` | `BIGINT` | → `goals`, nullable (événements globaux) |
| `type` | `TEXT` | Voir le catalogue dans [api.md](api.md#catalogue) |
| `actor` | `TEXT` | Rôle appelant, ou `system` |
| `payload` | `JSONB` | Objet autosuffisant pour l'affichage |
| `created_at` | `TIMESTAMPTZ` | Horodatage, utilisé par le replay pour respecter les intervalles |

## Contraintes et index notables

| Objet | Rôle |
| --- | --- |
| `approvals_pending_uniq` (unique partiel sur `kind, goal_id, COALESCE(task_id, 0)` où `status = 'pending'`) | Une seule approbation en attente par type et par tâche ; l'API traduit la violation en `409 duplicate_approval` |
| `runs_active_task_idx` (unique partiel sur `task_id, kind` où `status = 'running'`) | Pas deux runs simultanés sur la même tâche |
| `messages_unread_idx` (partiel, `read_at IS NULL`) | `GET /inbox?unread=true` et détection des rôles à réveiller |
| `messages_quota_idx` | Comptage du quota de messages par rôle et par objectif |
| `events_goal_idx` (`goal_id, id`) | Rattrapage SSE filtré par objectif, export |
| `tasks.wait_for → messages` | Ajoutée après la création de `messages` (référence croisée) |

## Temps réel

```sql
CREATE TRIGGER events_notify AFTER INSERT ON events
  FOR EACH ROW EXECUTE FUNCTION cfa_notify_event();   -- pg_notify('cfa_events', id)
```

Le `NOTIFY` part au `COMMIT` : un abonné ne reçoit jamais l'id d'un événement annulé.

**Ordre des ids.** Deux transactions concurrentes pourraient valider leurs événements dans le désordre (l'id 41 visible après le 42), et un client SSE qui reprend après le 42 manquerait le 41. Toute transaction qui insère un événement commence donc par :

```sql
SELECT pg_advisory_xact_lock(7243);   -- clé fixe « cfa_events »
```

Les écritures sont sérialisées, et les ids deviennent visibles dans l'ordre. Le volume visé (quelques écritures par seconde) le permet sans peine. Le verrou est pris en début de transaction, avant toute lecture d'état, ce qui sérialise aussi les contrôles métier (hop, quota, budget) sans `SELECT … FOR UPDATE` dispersés.

## Montants

*Phase 2 : pas encore implémenté.*

`NUMERIC` en base, `float64` en Go (conversion par pgx). Les montants restent petits (budgets de quelques dollars, quatre décimales) : l'imprécision du flottant est négligeable pour l'affichage. Les comparaisons au budget se font en SQL, sur des `NUMERIC`.

## Migrations

- Outil : [goose](https://github.com/pressly/goose), utilisé comme bibliothèque. Les fichiers `migrations/*.sql` sont embarqués dans le binaire.
- `cfa-api` applique les migrations au démarrage (`--migrate=false` pour désactiver) ; `cfa-api migrate up|down|status` existe pour l'exploitation.
- Une migration n'est jamais modifiée une fois publiée : on en ajoute une nouvelle.
- Nommage : `NNNNN_description.sql`, sections `-- +goose Up` et `-- +goose Down`.

## Accès depuis Go

Les requêtes sont écrites en SQL dans `internal/store/queries/*.sql`, puis [sqlc](https://sqlc.dev) génère du code Go typé sur pgx (`internal/store/db`). Le code généré est commité : il n'est pas nécessaire d'installer sqlc pour compiler. Après modification d'une requête ou d'une migration :

```bash
make generate        # sqlc generate
```

Détail des types et des méthodes dans [architecture.md](architecture.md).

## Exploitation

- Sauvegarde : `pg_dump` (le code produit par les agents est dans les branches git, pas en base).
- Taille : `events` est la seule table qui grossit vite (surtout `run.finished`, avec les diffs tronqués à 256 Kio). Pour archiver, exporter les objectifs terminés (`cfa export`) ; il n'y a pas de purge automatique.
