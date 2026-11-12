# Contrats de données

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). La [feuille de route](../README.md#feuille-de-route) indique ce qui est déjà en place.

Ce document recense les formats échangés par l'application Go : avec la CLI, le runner, les agents, le dashboard et les fichiers de replay. Pour chacun, il indique la source de vérité et le type Go correspondant.

Ces contrats concernent **cfa lui-même**. Les contrats du dépôt cible (`contracts/`, maintenus par le rôle `contracts`) sont un autre sujet.

## Sources de vérité

| Contrat | Spécification | Code Go |
| --- | --- | --- |
| Requêtes et réponses de l'API | [openapi.yaml](openapi.yaml) | [internal/apitypes/types.go](../internal/apitypes/types.go) |
| Payloads des événements | [api.md](api.md#catalogue) | [internal/events/payloads.go](../internal/events/payloads.go) |
| Format d'export (replay) | [api.md](api.md#export-et-replay) | `apitypes.ExportHeader`, `apitypes.Event` |
| Payloads des approbations | [api.md](api.md#contenu-payload) | `apitypes.ContractChangePayload`, `MergePayload`, `EscalationPayload`, `BudgetPayload` |
| Schéma de la base | [migrations/](../migrations/) | `internal/store/db` (généré par sqlc) |
| Environnement d'un agent | Ce document | `cmd/cfa`, runner |
| Configuration | [integration.md](integration.md#configuration) | [internal/config/config.go](../internal/config/config.go) |

Une modification de contrat se fait dans la spécification **et** dans le code, dans le même commit.

## Règles JSON

| Règle | Détail |
| --- | --- |
| Encodage | UTF-8, champs en `snake_case` |
| Champs absents | Dans les réponses et les événements, tous les champs sont présents ; une valeur absente vaut `null`. Pas de `omitempty`, sauf `errors` et `approval_id` dans `Error`. |
| Listes | Toujours un tableau, `[]` si vide, jamais `null` |
| Identifiants | Entiers 64 bits (`int64`) |
| Dates | RFC 3339 en UTC (`time.Time`) |
| Montants | Nombres décimaux en dollars (`float64` ; `NUMERIC` en base) |
| Requêtes | Champs inconnus refusés (`422`). Champs optionnels en pointeurs, pour distinguer « absent » de « zéro ». |
| Texte libre | `body` d'un message : objet JSON avec au moins `text` (`apitypes.MessageBody`) |

Pourquoi des champs toujours présents : le dashboard et les tests de replay lisent un schéma stable, sans cas « champ manquant ». Un client en JavaScript peut alors tester `=== null` partout.

## Correspondance OpenAPI → Go

| Schéma OpenAPI | Type Go | Remarque |
| --- | --- | --- |
| `GoalCreate` | `apitypes.GoalCreate` | `budget_usd`, `base_branch` en pointeurs (défauts de la configuration) |
| `Goal` | `apitypes.Goal` | |
| `GoalDetail` | `apitypes.GoalDetail` | Embarque `Goal` (champs au même niveau en JSON) |
| — | `apitypes.GoalPage` | Réponse de `GET /goals` (`items`, `next_before`) |
| — | `apitypes.GoalStatusChange`, `GoalMerged`, `GoalCancel` | Corps des routes du runner et de l'annulation |
| `TaskCreate`, `Task`, `TaskDetail`, `Review` | `apitypes.TaskCreate`, `Task`, `TaskDetail`, `Review` | |
| `TaskStatusChange` | `apitypes.TaskStatusChange` | |
| corps de `/tasks/{id}/review` | `apitypes.ReviewCreate` | |
| `MessageCreate`, `Message`, `MessageBody` | `apitypes.MessageCreate`, `Message`, `MessageBody` | `Message.Body` en `json.RawMessage` : renvoyé tel que stocké |
| `Thread`, `ThreadDetail` | `apitypes.Thread`, `ThreadDetail` | |
| `ApprovalCreate`, `Approval`, `ApprovalDecision` | `apitypes.ApprovalCreate`, `Approval`, `ApprovalDecision` | `Payload` en `json.RawMessage`, validé selon `kind` |
| `RunCreate`, `Run`, `RunProgress`, `RunFinish`, `Diffstat` | `apitypes.RunCreate`, `Run`, `RunProgress`, `RunFinish`, `Diffstat` | |
| réponse de `POST /runs` | `apitypes.RunCreated` | Contient le token d'agent en clair |
| `CheckReport`, `EventRef` | `apitypes.CheckReport`, `EventRef` | |
| `Event`, `ExportHeader` | `apitypes.Event`, `ExportHeader` | `Payload` en `json.RawMessage` |
| `PublicConfig`, `Health`, `Error` | `apitypes.PublicConfig`, `Health`, `Error` | |

## Événements

Enveloppe commune (`apitypes.Event`), identique en SSE, dans l'export et dans le replay :

```json
{ "id": 1284, "goal_id": 7, "type": "message.sent", "actor": "front", "created_at": "2026-11-06T14:02:11Z", "payload": { } }
```

| Type | Payload Go |
| --- | --- |
| `goal.created` | `GoalCreatedPayload{Goal}` |
| `goal.status_changed` | `GoalStatusChangedPayload{GoalID, Title, From, To, Reason}` |
| `task.created` | `TaskCreatedPayload{Task}` |
| `task.status_changed` | `TaskStatusChangedPayload{TaskID, Role, Title, From, To, Attempts, Reason, Summary, WaitFor}` |
| `task.reviewed` | `TaskReviewedPayload{TaskID, Role, Title, Verdict, Comment, Attempt, RunID}` |
| `run.started` | `RunStartedPayload{Run, TaskTitle}` |
| `run.progress` | `RunProgressPayload{RunID, Role, Step, Text, Path}` |
| `run.finished` | `RunFinishedPayload{Run, DurationMS, Error, Diffstat, Files, Diff, DiffTruncated, GoalSpentUSD, GoalBudgetUSD}` |
| `message.sent` | `MessageSentPayload{Message, Thread}` |
| `message.rejected` | `MessageRejectedPayload{From, To, Type, ThreadID, Code, Reason, Body, ApprovalID}` |
| `approval.requested` | `ApprovalRequestedPayload{Approval, TaskTitle}` |
| `approval.resolved` | `ApprovalResolvedPayload{ApprovalID, Kind, Decision, Comment, TaskID, BudgetUSD}` |
| `check.finished` | `CheckFinishedPayload{TaskID, RunID, Name, Command, Status, DurationMS, Output}` |
| `budget.warning`, `budget.exceeded` | `BudgetPayload{BudgetUSD, SpentUSD, Ratio}` |
| `merge.done` | `MergeDonePayload{Branch, BaseBranch, Mode, Commit, PRURL}` |

Chaque payload suffit à l'affichage : un `task.status_changed` porte le titre et le rôle de la tâche, pour que le dashboard n'ait jamais à les chercher ailleurs.

## Export JSON Lines

```
{"format":"cfa-events/1","goal_id":7,"exported_at":"2026-11-06T18:00:00Z","events":412}
{"id":1201,"goal_id":7,"type":"goal.created","actor":"human","created_at":"…","payload":{…}}
…
```

- Ligne 1 : `ExportHeader`. `format` vaut `cfa-events/1` ; `goal_id` est `null` pour un export de toute l'instance (démo, archive).
- Lignes suivantes : un `Event` par ligne, ids strictement croissants.
- Lecteurs : `internal/replay` (serveur) et le dashboard (chargement d'un fichier dans le navigateur). Les deux refusent un format inconnu.

## Environnement d'un agent

Le runner lance chaque agent avec ces variables. C'est le contrat entre le runner, l'image d'agent et la CLI `cfa`.

| Variable | Contenu |
| --- | --- |
| `CFA_URL` | Adresse de l'API vue du conteneur (`http://cfa-api:8080`) |
| `CFA_TOKEN` | Token d'agent renvoyé par `POST /runs` ; limité au run, à son rôle et à son objectif |
| `CFA_RUN` | Id du run |
| `CFA_TASK` | Id de la tâche (runs `task` et `review`) ; vide sinon |
| `CFA_ROLE` | Rôle de l'agent |
| `CFA_KIND`, `CFA_GOAL` | Type de run (`plan`, `task`, `review`, `answer`) et id de l'objectif |
| `CFA_PROMPT_FILE` | Chemin du prompt complet du run (bloc commun, rôle, objectif, tâche) |
| `CFA_MODEL`, `CFA_THINKING` | Modèle du rôle (`fournisseur/modèle`) et niveau de réflexion (vide : défaut du harness) |
| `CFA_MAX_TOKENS` | Plafond de tokens du run (`budget.per_run_tokens`) ; appliqué par `cfa-pi` |
| `TRACEPARENT`, `TRACESTATE` | Contexte de trace W3C du run (OpenTelemetry) ; la CLI `cfa` s'y rattache |
| Clé du fournisseur LLM | Variable propre au fournisseur, lue depuis `.env` par le runner ; jamais en base ni dans les événements |

## Sortie d'une commande d'agent

Ce que le runner lit sur la sortie standard de `agent_command` (une ligne JSON par message ; les autres lignes sont ignorées). C'est le contrat entre le runner et un adaptateur (`cfa-pi`, agent factice).

| Ligne | Champs | Effet |
| --- | --- | --- |
| `{"type":"progress", …}` | `step` (type d'étape), `text` (≤ 200 caractères), `path` (facultatif) | `POST /runs/{id}/progress`, événement `run.progress` |
| `{"type":"result", …}` | `tokens_in`, `tokens_out`, `cost_usd` | Consommation transmise à `POST /runs/{id}/finish` ; la dernière ligne l'emporte |

Types d'étapes produits par `cfa-pi` : `bash`, `cfa` (commande de coordination), `read`, `write`, `edit`, `grep`, `find`, `ls`, `tool_error`, `retry`, `compaction`, `warning` (coût inconnu). Le code de sortie décide du statut du run : `0` réussi, autre chose échoué.

## Évolution

| Changement | Compatible ? | Que faire |
| --- | --- | --- |
| Ajouter un champ à une réponse ou à un payload | Oui | Spécification + type Go |
| Ajouter un type d'événement | Oui | Le dashboard ignore les types inconnus |
| Ajouter un champ optionnel à une requête | Oui | Pointeur en Go |
| Rendre obligatoire un champ de requête | Non | À éviter ; sinon, mettre à jour la CLI dans le même commit |
| Renommer ou supprimer un champ d'événement | Non | Nouveau format d'export (`cfa-events/2`) ; le dashboard garde la lecture de la version 1 |

## Vérification

- Les tests d'API vérifient les codes et la forme des réponses (`internal/api/api_test.go`), y compris l'absence de `omitempty` (`"finished_at":null`).
- À venir : un test qui valide chaque réponse contre `docs/openapi.yaml` (bibliothèque `kin-openapi`), et un test qui rejoue les exports de `runs/` dans le lecteur.
