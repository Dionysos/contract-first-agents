# cfa-api — Référence de l'API

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). Une section qui commence par *Phase 2*, *Phase 3*, *Phase 4* ou *Prévu* n'est pas encore implémentée ; une note *En phase 1* signale ce qui diffère aujourd'hui. La [feuille de route](../README.fr.md#feuille-de-route) indique ce qui est déjà en place.

Ce document complète [openapi.yaml](openapi.yaml), qui décrit les routes, les corps et les codes de réponse. Il couvre ce qu'OpenAPI exprime mal : droits par rôle, cycles de statut, garde-fous, effets des approbations, catalogue des événements, SSE et replay. Le contexte général est dans [cfa.md](cfa.md).

Les deux fichiers font foi ensemble. `openapi.yaml` est vérifié par Redocly en CI, et les tests d'intégration et de bout en bout valident chaque échange contre lui (`internal/apitest`) : toute réponse doit être conforme, et une requête hors contrat ne doit jamais être acceptée.

## Conventions

*En phase 1* : la référence exacte de ce qui est implémenté (19 routes, statuts, codes d'erreur, 10 types d'événements) est [openapi-phase1.yaml](openapi-phase1.yaml). Pas de montants ni d'`Idempotency-Key`.

| Sujet | Règle |
| --- | --- |
| Format | JSON UTF-8, `Content-Type: application/json` exigé sur les `POST` avec corps (sinon `415 unsupported_media_type`) |
| Noms | `snake_case` ; énumérations en minuscules |
| Identifiants | Entiers (`BIGSERIAL`) |
| Dates | RFC 3339, UTC (`2026-11-06T14:02:11Z`) |
| Montants | Nombres décimaux en dollars (`budget_usd`, `cost_usd`) ; stockés en `NUMERIC` |
| Champs inconnus | Refusés (`422`) : un agent qui se trompe de champ doit le savoir |
| Taille | Corps limité à 1 Mio (`413 payload_too_large`) ; diff d'un run tronqué à 256 Kio par le runner, sortie d'un contrôle à 64 Kio |
| Listes | Enveloppe `{"items": [...]}` ; pagination par curseur `before` / `next_before` sur `GET /goals` |
| Version | Pas de préfixe `/v1` tant que l'API n'a qu'un client (CLI et dashboard du même dépôt) |
| Écoute | `127.0.0.1:8080` par défaut |

**Style des routes.** Les ressources se lisent en `GET`, mais les changements d'état sont des commandes en `POST` (`/goals/{id}/cancel`, `/tasks/{id}/status`, `/approvals/{id}/decision`) plutôt que des `PATCH` ou `DELETE` : une transition est une décision métier, validée selon le rôle de l'appelant et le statut courant, qui produit un événement typé, et non la simple modification d'un champ.

## Authentification et droits

### Moyens d'authentification

- **Token** : `Authorization: Bearer <token>`. Stocké haché (SHA-256) dans `tokens`. Le rôle, et pour un agent le run et l'objectif, sont déduits du token, jamais du corps.
- **Session** : cookie `cfa_session` obtenu par `POST /auth/code` puis `GET /login?code=`. Rôle `human` uniquement. Avec un cookie, toute écriture exige un en-tête `Origin` égal à l'adresse du dashboard, sinon `403 origin_mismatch`.
- `GET /login`, `GET /healthz` et `GET /` sont publics.

### Tokens

| Token | Émis par | Durée |
| --- | --- | --- |
| `human`, `runner`, `ci` | `cfa-api token create --role <rôle>` (commande d'administration, accès direct à la base) | Sans expiration, révocable |
| Agent | `POST /runs` (renvoyé une seule fois) | Jusqu'à `POST /runs/{id}/finish`, au plus `per_run_timeout` + 5 min |

Un token d'agent est limité à **son objectif** : toute ressource d'un autre objectif répond `404`, pour ne rien révéler.

### Qui peut appeler quoi

`agent` = token d'un run ; « de la tâche » = run `task` en cours sur cette tâche.

| Route | human | runner | ci | Agents |
| --- | --- | --- | --- | --- |
| `POST /goals`, `POST /goals/{id}/cancel` | oui | | | |
| `POST /goals/{id}/status`, `/merged` | | oui | | |
| `GET /goals`, `/goals/{id}`, `/tasks/{id}`, `/threads/{id}` | oui | oui | oui | son objectif |
| `POST /tasks` | oui | | | `planner` (run `plan`) |
| `POST /tasks/{id}/status` | | oui | | agent de la tâche |
| `POST /tasks/{id}/review` | | | | `reviewer` (run `review` sur la tâche) |
| `POST /messages` | oui | | oui | oui |
| `GET /inbox`, `POST /messages/{id}/ack` | | | | destinataire |
| `GET /approvals`, `POST /approvals/{id}/decision` | oui | | | |
| `POST /approvals` | | oui | | |
| `POST /runs`, `/runs/{id}/progress`, `/runs/{id}/finish` | | oui | | |
| `POST /checks` | | oui | oui | |
| `GET /events` | oui | oui | oui | son objectif |
| `GET /events/export`, `GET /config/public` | oui | | | |
| `POST /auth/code` | token seulement | | | |

Un appel hors de ce tableau répond `403 forbidden`.

## Erreurs

*En phase 1* : seuls les codes listés dans [openapi-phase1.yaml](openapi-phase1.yaml) existent ; ceux liés aux messages, au budget, aux tentatives et à l'idempotence arrivent avec les phases suivantes.

Toutes les erreurs ont la même forme :

```json
{ "code": "hop_limit_reached", "message": "Le fil 31 a atteint 5 échanges ; une escalade a été créée.", "approval_id": 12 }
```

`errors` (par champ) n'apparaît qu'avec `validation_failed` ; `approval_id` quand l'erreur a créé une escalade. Les clients se basent sur `code`, jamais sur `message`.

| HTTP | `code` | Cas |
| --- | --- | --- |
| 401 | `unauthorized` | Token ou session absent, inconnu, expiré ou révoqué |
| 403 | `forbidden` | Rôle non autorisé pour la route ou la ressource |
| 403 | `origin_mismatch` | Écriture par cookie sans `Origin` valide |
| 403 | `replay_mode` | Route indisponible en mode replay |
| 404 | `not_found` | Ressource inexistante ou hors de l'objectif du run |
| 413 | `payload_too_large` | Corps de plus de 1 Mio |
| 415 | `unsupported_media_type` | Corps non JSON |
| 422 | `validation_failed` | Champ manquant, invalide ou inconnu |
| 409 | `invalid_transition` | Transition de statut non autorisée, ou précondition non remplie |
| 409 | `goal_closed` | Objectif en `done`, `failed` ou `cancelled` |
| 409 | `idempotency_key_reused` | Même `Idempotency-Key` avec un corps différent |
| 409 | `hop_limit_reached` | Fil à 5 échanges ; escalade créée |
| 409 | `message_quota_exceeded` | Quota de messages du rôle atteint pour l'objectif ; escalade créée |
| 409 | `attempts_exhausted` | Tâche à 3 tentatives ; escalade créée |
| 409 | `budget_exceeded` | Budget de l'objectif atteint, approbation `budget` en attente |
| 409 | `duplicate_approval` | Une approbation identique est déjà en attente |
| 409 | `already_decided` | Approbation déjà tranchée |
| 500 | `internal` | Erreur inattendue (détail dans les logs, jamais dans la réponse) |

Les refus métier restent en `409` (plutôt que `429` pour les quotas) : la CLI n'a qu'un cas à traiter et lit `code`.

## Objectifs

### Cycle de statuts

```
planning ──► running ──► awaiting_approval ──► done
   ▲           │  ▲              │
   └───────────┘  └──── merge refusé
   (replanification)
tout statut non terminal ──► failed | cancelled
```

| Transition | Déclencheur |
| --- | --- |
| `planning → running` | `runner`, via `POST /goals/{id}/status`, quand le run `plan` a créé au moins une tâche |
| `running → planning` | `runner`, pour relancer le `planner` (replanification) |
| `running → awaiting_approval` | Automatique : création de l'approbation `merge` |
| `awaiting_approval → running` | Automatique : approbation `merge` refusée |
| `awaiting_approval → done` | `runner`, via `POST /goals/{id}/merged`, après approbation `merge` acceptée |
| `* → failed` | `runner` (plan vide, erreur), ou escalade refusée sans tâche |
| `* → cancelled` | `human` via `POST /goals/{id}/cancel`, ou approbation `budget` refusée |

`done`, `failed` et `cancelled` sont terminaux : toute écriture sur l'objectif répond ensuite `409 goal_closed` (sauf les lectures et `POST /checks`).

### Création

- Validation : `title` 5 à 120 caractères ; `description` 20 à 5 000 ; `budget_usd` entre 0,50 et `budget.max_goal_usd` (défaut `budget.per_goal_usd`) ; `base_branch` existante (défaut `base_branch`).
- `Idempotency-Key` : conservée 24 heures avec l'empreinte du corps. Même clé et même corps : `200` et l'objectif existant, sans nouvel événement. Même clé et corps différent : `409 idempotency_key_reused`.
- L'API crée la ligne et publie `goal.created`. La branche `goal/<id>` est créée par le runner.

### Annulation

Objectif `cancelled`, tâches non terminées `failed` (raison `cancelled`), approbations en attente `rejected` (commentaire « objectif annulé »). Le runner arrête les runs en réagissant à `goal.status_changed`, puis appelle `POST /runs/{id}/finish` avec `killed`. Annuler un objectif déjà annulé renvoie `200` ; annuler un objectif `done` ou `failed` renvoie `409 goal_closed`.

## Tâches

### Cycle de statuts

*En phase 1* : `todo → running → review → done`, et `failed` depuis tout statut non terminal. Pas de `blocked`, de `changes_requested`, de reviewer ni de compteur de tentatives : le runner passe la tâche en `done` si le périmètre et les contrôles sont bons, en `failed` sinon, et l'objectif échoue.

```
todo ──► running ──► review ──► done
           │  ▲        │
           │  │        ▼
           │  └── changes_requested
           ▼  ▲
         blocked
tout statut non terminal ──► failed
```

| Transition | Qui | Route | Condition |
| --- | --- | --- | --- |
| `todo → running` | runner | `POST /runs` (`kind: task`) | Dépendances `done` ; `attempts` + 1 |
| `changes_requested → running` | runner | `POST /runs` | `attempts` + 1 |
| `blocked → running` | runner | `POST /runs` | Réponse reçue ; `attempts` inchangé |
| `running → blocked` | agent de la tâche | `POST /tasks/{id}/status` | `wait_for` : message de ce rôle, dans cet objectif, sans réponse |
| `running → review` | agent de la tâche | `POST /tasks/{id}/status` | `summary` obligatoire (`cfa task done`) |
| `review → changes_requested` | runner | `POST /tasks/{id}/status` | Périmètre ou contrôle en échec ; `reason` obligatoire |
| `running → changes_requested` | runner | `POST /tasks/{id}/status` | Run terminé sans conclusion (ni `done` ni `block`), ou arrêté au timeout ; `reason` obligatoire |
| `review → changes_requested` | reviewer | `POST /tasks/{id}/review` | `verdict: changes`, `comment` obligatoire |
| `review → done` | reviewer | `POST /tasks/{id}/review` | `verdict: approve` |
| `* → failed` | agent de la tâche (depuis `running`), runner | `POST /tasks/{id}/status` | `reason` obligatoire |

Une tâche passe donc en `review` dès que l'agent déclare avoir fini ; le runner vérifie ensuite le périmètre et les contrôles. **Le runner ne lance le run `review` qu'après un `run.finished` réussi et des contrôles verts** ; sinon il renvoie la tâche en `changes_requested` avec la raison.

`attempts` compte les runs `task` lancés depuis `todo` ou `changes_requested`. Si un nouveau lancement dépasse `limits.max_attempts` (3), `POST /runs` répond `409 attempts_exhausted` et crée une escalade.

### Relectures

*Phase 2 : pas encore implémenté.*

Chaque `POST /tasks/{id}/review` publie `task.reviewed`. `GET /tasks/{id}` renvoie la liste `reviews`, reconstruite à partir de ces événements, que `cfa task show` affiche au run suivant.

## Runs

*En phase 1* : `kind` vaut `plan` ou `task` seulement ; `POST /runs/{id}/finish` n'enregistre ni tokens ni coût (étapes 2, 4 et 5 absentes), et `run.finished` ne porte pas `goal_spent_usd` ni `goal_budget_usd`.

`POST /runs` enregistre le run, publie `run.started` et renvoie le token d'agent à transmettre au conteneur (`CFA_TOKEN`), avec `CFA_RUN` et `CFA_TASK`.

| `kind` | Rôle | Préconditions | Effet |
| --- | --- | --- | --- |
| `plan` | `planner` | Objectif en `planning` | — |
| `task` | rôle de la tâche | Tâche en `todo`, `changes_requested` ou `blocked` ; dépendances `done` | Tâche `running` |
| `answer` | tout rôle d'agent | Messages non lus pour ce rôle dans l'objectif | Lecture seule (montage, non vérifié par l'API) |
| `review` | `reviewer` | Tâche en `review` | — |

Pour tous les types : objectif non terminal (`409 goal_closed`) et pas d'approbation `budget` en attente (`409 budget_exceeded`). La limite `concurrency` est appliquée par le runner, pas par l'API.

`POST /runs/{id}/finish` :

1. enregistre statut, tokens, coût, et révoque le token d'agent ;
2. ajoute `cost_usd` à `goals.spent_usd` ;
3. publie `run.finished` (avec diffstat, fichiers et diff) ;
4. si `spent_usd` franchit 80 % du budget : `budget.warning` (une fois par objectif) ;
5. s'il atteint 100 % : `budget.exceeded` et approbation `budget`.

Le statut de la tâche ne change pas : c'est le runner qui le fait ensuite (`review` confirmé, `changes_requested` ou `failed`). Les runs déjà lancés au moment du dépassement vont à leur terme ; ils sont bornés par `per_run_tokens` et `per_run_timeout`.

## Messages et fils

*Phase 2 : pas encore implémenté.*

### Désigner le fil

| Champs fournis | Effet |
| --- | --- |
| `reply_to` | Fil du message cité ; `to` = émetteur du message cité (un `to` fourni doit correspondre) |
| `thread_id` | Fil existant du même objectif, en statut `open` |
| `subject` (+ `task_id` optionnel) | Nouveau fil ; `goal_id` déduit du run pour un agent, obligatoire pour `human` et `ci` |

Un seul de `reply_to`, `thread_id`, `subject`. Un fil `escalated` refuse les messages (`409 invalid_transition`) jusqu'à la décision humaine.

### Règles par type

| Type | Destinataire | Fil | Remarque |
| --- | --- | --- | --- |
| `question` | rôle d'agent, autre que soi | nouveau ou existant | Peut réveiller le destinataire (run `answer`) |
| `answer` | — | `reply_to` ou `thread_id` obligatoire | Débloque une tâche dont `wait_for` est le message cité ou un message du fil |
| `contract_change` | `contracts` uniquement | nouveau ou existant | |
| `notify` | rôle d'agent | nouveau ou existant | Aucune réponse attendue |
| `escalate` | `human` uniquement | nouveau ou existant | Crée une approbation `escalation` (raison `agent`) et passe le fil en `escalated` |

Destinataire inconnu (absent de `cfa.yaml`) : `422`.

### Garde-fous

Contrôlés dans cet ordre, dans la transaction d'insertion :

1. **Objectif ouvert**, sinon `409 goal_closed`.
2. **Hop** : chaque message d'un agent ou de `ci` fait `hop + 1`. Si le résultat dépasse `limits.max_hop` (5), le message est refusé, le fil passe en `escalated` et une escalade (`reason: hop_limit`) est créée. Les messages de `human` ne comptent pas.
3. **Quota** : au-delà de `limits.messages_per_role_per_goal` (30) messages envoyés par un rôle dans l'objectif, refus et escalade (`reason: message_quota`), une seule par rôle et par objectif.

Tout refus, y compris de validation, publie `message.rejected` avec la raison : le dashboard montre les tentatives refusées, pas seulement les messages acceptés.

## Approbations

*En phase 1* : seul le type `merge` existe. Un merge refusé **annule** l'objectif (`cancelled`) au lieu de relancer le `planner`.

`human` tranche avec `POST /approvals/{id}/decision` (`approve` ou `reject`, `comment`). L'API applique les effets ci-dessous dans la même transaction, puis publie `approval.resolved` ; le runner fait le reste.

| Type | Créée par | Si approuvée (API) | Si refusée (API) | Puis le runner |
| --- | --- | --- | --- | --- |
| `contract_change` | runner (changement cassant détecté) | — | Message `answer` de `human` à l'émetteur du `contract_change`, avec le commentaire | Approuvée : poursuit la tâche `contracts` ; refusée : la renvoie en `changes_requested` |
| `escalation` | API (hop, quota, tentatives, `escalate`) ou runner (conflit de fusion, run en échec) | Le commentaire est envoyé comme `answer` de `human` dans le fil ; fil `open`, `hop` remis à 0 ; tentatives de la tâche remises à 0 | Tâche `failed` ; objectif `failed` si l'escalade n'a pas de tâche | Relance le travail |
| `budget` | API (100 % du budget) | `budget_usd` remplacé par la valeur fournie (obligatoire, > `spent_usd`, ≤ `max_goal_usd`) | Objectif `cancelled` | Reprend ou arrête les runs |
| `merge` | runner (toutes les tâches `done`, contrôles verts) | — | Objectif `running` | Approuvée : merge ou PR puis `POST /goals/{id}/merged` ; refusée : relance le `planner` avec le commentaire |

`comment` est obligatoire pour un refus, et pour l'approbation d'une escalade : c'est la consigne transmise aux agents. Une seule approbation en attente par type et par tâche (ou par objectif sans tâche) : sinon `409 duplicate_approval`.

### Contenu (`payload`)

| Type | Champs |
| --- | --- |
| `contract_change` | `summary`, `diff`, `breaking[]`, `thread_id`, `message_id` |
| `merge` | `branch`, `base_branch`, `diffstat`, `checks[]` (`name`, `status`) |
| `escalation` | `reason` (`hop_limit`, `message_quota`, `attempts_exhausted`, `agent`, `merge_conflict`, `run_failed`), `text`, `thread_id`, `message_id`, `from_role` |
| `budget` | `budget_usd`, `spent_usd` |

## Événements

### Enveloppe

Tous les événements ont la même forme, en SSE comme dans l'export :

```json
{
  "id": 1284,
  "goal_id": 7,
  "type": "message.sent",
  "actor": "front",
  "created_at": "2026-11-06T14:02:11Z",
  "payload": { }
}
```

`actor` est le rôle appelant, ou `system` pour ce que l'API produit d'elle-même (budget, escalades automatiques). Chaque `payload` contient tout ce qu'il faut pour l'afficher : le dashboard ne fait aucune autre requête.

### Catalogue

*En phase 1* : 10 types seulement, `goal.created`, `goal.status_changed`, `task.created`, `task.status_changed`, `run.started`, `run.finished`, `check.finished`, `approval.requested`, `approval.resolved` et `merge.done` (schémas dans [openapi-phase1.yaml](openapi-phase1.yaml)).

| Type | `payload` |
| --- | --- |
| `goal.created` | `goal` : objet `Goal` complet |
| `goal.status_changed` | `goal_id`, `title`, `from`, `to`, `reason` |
| `task.created` | `task` : objet `Task` complet (avec `depends_on`) |
| `task.status_changed` | `task_id`, `role`, `title`, `from`, `to`, `attempts`, `reason`, `summary`, `wait_for` |
| `task.reviewed` | `task_id`, `role`, `title`, `verdict`, `comment`, `attempt`, `run_id` |
| `run.started` | `run` : objet `Run` ; `task_title` |
| `run.progress` | `run_id`, `role`, `step` (`tool`, `file`, `note`), `text`, `path` |
| `run.finished` | `run` : objet `Run` final ; `duration_ms`, `error`, `diffstat`, `files[]`, `diff`, `diff_truncated`, `goal_spent_usd`, `goal_budget_usd` |
| `message.sent` | `message` : objet `Message` ; `thread` : objet `Thread` (avec `hop` à jour) |
| `message.rejected` | `from`, `to`, `type`, `thread_id`, `code`, `reason`, `body`, `approval_id` |
| `approval.requested` | `approval` : objet `Approval` ; `task_title` |
| `approval.resolved` | `approval_id`, `kind`, `decision`, `comment`, `task_id`, `budget_usd` |
| `check.finished` | `task_id`, `run_id`, `name`, `command`, `status`, `duration_ms`, `output` |
| `budget.warning`, `budget.exceeded` | `budget_usd`, `spent_usd`, `ratio` |
| `merge.done` | `branch`, `base_branch`, `mode` (`local`, `pr`), `commit`, `pr_url` |

Les champs non pertinents valent `null` plutôt que d'être omis : le dashboard et les tests de replay lisent un schéma stable.

### Évolution

- Ajouter un champ ou un type d'événement est compatible : le dashboard ignore ce qu'il ne connaît pas.
- Renommer ou supprimer un champ casse le replay des anciens exports : on incrémente alors `format` (`cfa-events/2`) et le dashboard garde la lecture des versions précédentes.

## Flux SSE

`GET /events?goal=&since=` ouvre une connexion `text/event-stream` :

```
retry: 3000

id: 1284
event: message.sent
data: {"id":1284,"goal_id":7,"type":"message.sent","actor":"front","created_at":"2026-11-06T14:02:11Z","payload":{...}}

: ping
```

- **Point de départ** : `Last-Event-ID` s'il est présent (reconnexion du navigateur), sinon `since`, sinon seulement les nouveaux événements. `since=0` rejoue tout.
- **Rattrapage sans trou** : l'API commence par `LISTEN cfa_events`, puis lit en base les événements d'id supérieur au point de départ, puis diffuse ceux reçus par `NOTIFY` en ignorant les ids déjà envoyés. Aucun événement n'est perdu entre la lecture et l'écoute.
- **Ordre** : par id croissant. Les ids sont attribués à l'insertion, et deux transactions concurrentes pourraient valider dans le désordre : un client qui reprend « après le dernier id reçu » manquerait alors un événement. Pour l'éviter, toute transaction qui insère un événement prend d'abord `pg_advisory_xact_lock(<clé cfa_events>)` : les insertions d'événements sont sérialisées, les ids sont visibles dans l'ordre. Le débit visé (quelques événements par seconde) le permet sans peine. À chaque `NOTIFY`, l'API relit en base tout ce qui suit le dernier id diffusé.
- **Filtre** : `goal` limite aux événements de l'objectif ; un token d'agent est toujours limité au sien.
- **Maintien** : commentaire `: ping` toutes les 15 secondes ; `retry: 3000` pour la reconnexion.
- **Authentification** : cookie pour le navigateur (`EventSource` n'envoie pas d'en-tête `Authorization`), token pour la CLI (`cfa watch`).
- **Client lent** : tampon de 1 000 événements par client ; au-delà, la connexion est fermée et le client se reconnecte avec `Last-Event-ID`.

## Export et replay

*Phase 3 : pas encore implémenté.*

### Export

`GET /events/export?goal=7` renvoie `application/x-ndjson`. Sans `goal`, l'export couvre toute l'instance (`goal_id: null` dans l'en-tête) : utile pour une démo à plusieurs objectifs ou une archive. La première ligne est un en-tête, puis vient un événement par ligne, par id croissant :

```
{"format":"cfa-events/1","goal_id":7,"exported_at":"2026-11-06T18:00:00Z","events":412}
{"id":1201,"goal_id":7,"type":"goal.created","actor":"human","created_at":"...","payload":{...}}
...
```

### Mode replay

`cfa-api --replay runs/goal-7.jsonl --speed 4` démarre sans base :

| Route | Comportement |
| --- | --- |
| `GET /` | Dashboard, en lecture seule |
| `GET /events` | Réémet les événements en respectant les intervalles d'origine divisés par `--speed`. Sans authentification. `since` et `Last-Event-ID` fonctionnent. |
| `GET /config/public` | `replay: true` |
| `GET /healthz` | `{"status":"ok","db":"replay"}` |
| Toutes les autres | `403 replay_mode` |

## Écarts avec cfa.md

Cette référence ajoute des éléments absents de [cfa.md](cfa.md), nécessaires pour que le runner et la CLI aient une route pour chaque action.

**Nouvelles routes**

| Route | Pourquoi |
| --- | --- |
| `GET /tasks/{id}` | `cfa task show` (spec, relectures, message attendu) |
| `POST /goals/{id}/status` | Le runner passe l'objectif de `planning` à `running` ou en `failed` |
| `POST /goals/{id}/merged` | Le runner enregistre le merge (`merge.done`, objectif `done`) |
| `POST /approvals` | Le runner demande les approbations `contract_change`, `merge` et certaines escalades |
| `POST /runs/{id}/progress` | Publication de `run.progress` |
| `POST /checks` | Publication de `check.finished` (runner et CI) |

**Choix précisés**

- `data` du flux SSE contient l'événement complet (enveloppe + `payload`), et non le seul `payload` comme dans l'exemple de cfa.md : même format qu'à l'export.
- Le token d'agent est renvoyé par `POST /runs`.
- Format d'erreur unique `{code, message, errors?}` ; l'exemple `422` de cfa.md reste valide (même champ `errors`).
- Nouveau type d'événement `task.reviewed`.
- `POST /messages` accepte `reply_to` (utilisé par `cfa reply`).
- Les escalades dues aux limites sont créées par l'API, celles dues à git par le runner.

**À reporter dans le modèle de données** (migration initiale)

| Table | Ajout |
| --- | --- |
| `goals` | `base_branch TEXT NOT NULL` ; `budget_warned BOOLEAN NOT NULL DEFAULT false` |
| `tasks` | `wait_for BIGINT REFERENCES messages(id)` ; `summary TEXT` |
| `threads` | `created_at TIMESTAMPTZ NOT NULL DEFAULT now()` |
| `messages` | `goal_id BIGINT NOT NULL REFERENCES goals(id)` (quota sans jointure) ; `reply_to BIGINT REFERENCES messages(id)` ; index `(goal_id, from_role)` |
| `approvals` | `created_at TIMESTAMPTZ NOT NULL DEFAULT now()` |
| `tokens` | `created_at`, `revoked_at TIMESTAMPTZ` |
| nouvelle `sessions` | `hash`, `expires_at`, `created_at` (cookie du dashboard) |
| nouvelle `login_codes` | `hash`, `expires_at`, `used_at` |
| nouvelle `idempotency_keys` | `key`, `body_hash`, `goal_id`, `created_at` (purge après 24 h) |

## Points à valider

1. **Messages de `human`** hors du compteur `hop` : l'humain peut toujours débloquer un fil. À confirmer.
2. **Budget refusé → `cancelled`** plutôt que `failed` : c'est une décision humaine, pas une erreur.
3. **Tâche en `review` avant la vérification du runner** : l'agent déclare avoir fini, le runner peut ensuite la renvoyer en `changes_requested`. L'autre option serait une route `POST /tasks/{id}/done`, avec un passage en `review` réservé au runner.
4. **Escalade approuvée → tentatives remises à 0** : sinon l'humain ne peut pas accorder de nouvel essai.
5. **Refus métier en `409`**, y compris pour les quotas, plutôt qu'en `429`.
