# Tests de montée en charge

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). La [feuille de route](../README.md#feuille-de-route) indique ce qui est déjà en place.

Ce document décrit comment vérifier que `cfa-api` tient la charge. **Il est prévu pour un second temps**, une fois la phase 2 de l'API livrée (messages, runs, approbations) : rien n'est encore implémenté.

## Ce que « charge » veut dire ici

cfa n'est pas un service grand public : un humain, quelques dizaines d'agents au plus, quelques dashboards ouverts. Les risques ne sont donc pas le nombre d'utilisateurs, mais quatre points précis de l'architecture :

| Point sensible | Pourquoi | Où |
| --- | --- | --- |
| **Écritures sérialisées** | Toute écriture prend `pg_advisory_xact_lock` pour garantir l'ordre des événements ([database.md](database.md#temps-réel)) : le débit d'écriture est plafonné par la durée d'une transaction | `store.Write` |
| **Diffusion temps réel** | Chaque `NOTIFY` déclenche une lecture en base puis une copie vers chaque abonné SSE | `events.Hub`, `api.streamEvents` |
| **Rattrapage d'historique** | `?since=0` ou une reconnexion relit tout l'historique d'un objectif ; les `run.finished` contiennent des diffs (jusqu'à 256 Kio) | `Hub.Backlog`, `events_goal_idx` |
| **Croissance de `events`** | Table en ajout seul, jamais purgée | Index, taille des payloads |

Les tests visent à chiffrer ces limites et à vérifier qu'aucune n'est atteinte avec une marge confortable dans l'usage prévu.

## Objectifs chiffrés (à confirmer)

Usage prévu : 3 objectifs simultanés, `concurrency` jusqu'à 20 runs, 10 dashboards ouverts.

| Indicateur | Cible | Mesure |
| --- | --- | --- |
| Débit d'écriture soutenu | ≥ 200 écritures/s (×20 l'usage prévu) | requêtes `POST` réussies par seconde |
| Latence d'écriture | p95 < 50 ms, p99 < 150 ms | côté client |
| Délai de diffusion SSE | p95 < 200 ms entre le commit et la réception | `created_at` de l'événement vs heure de réception |
| Clients SSE simultanés | 500 sans dégradation des écritures | |
| Intégrité du flux | 0 événement perdu, dupliqué ou désordonné, y compris après reconnexion | contrôle des ids côté client |
| Rattrapage | 100 000 événements d'un objectif relus en < 5 s | `?since=0` |
| Ressources | Mémoire de `cfa-api` stable sur 2 h (pas de fuite) ; < 300 Mo à 500 clients | `/metrics`, `docker stats` |

## Outil : `cmd/cfa-load`

Un générateur de charge en Go, dans le dépôt, plutôt qu'un outil externe :

- il réutilise `internal/client` et `apitypes` : les requêtes respectent le contrat, sans duplication ;
- il sait **vérifier** le flux SSE (ordre, trous, doublons, délai), ce que k6 ou vegeta ne font pas sans extension ;
- il simule des agents réalistes (tokens de run, enchaînement tâche → message → statut).

```
cfa-load write   --rate 50:400:30s --duration 5m      # rampe d'écritures (débit:début:fin:pas)
cfa-load sse     --clients 500 --goal 3 --duration 5m # abonnés, mesure du délai et intégrité
cfa-load agents  --goals 5 --agents 40 --duration 30m # scénario réaliste (voir plus bas)
cfa-load backlog --events 100000                       # remplissage puis rattrapage since=0
cfa-load report  results/*.json > results/report.md
```

Chaque commande écrit un fichier de résultats JSON (histogrammes HDR des latences, compteurs, erreurs par `code`) ; `report` produit un tableau Markdown comparable d'une version à l'autre.

**Préparation des données.** Des tokens `human` et `runner` sont créés au démarrage (`cfa-api token create`). Pour simuler des agents, `cfa-load` déclare des runs (`POST /runs`) et utilise les tokens renvoyés, comme le runner.

Pour un premier débrouillage avant `cfa-load`, `vegeta` ou `k6` suffisent sur les routes REST (`POST /goals`, `POST /tasks`, `GET /goals/{id}`).

## Scénarios

| # | Scénario | Ce qu'il vérifie |
| --- | --- | --- |
| 1 | **Rampe d'écritures** : mélange 40 % `POST /messages`, 30 % `POST /tasks/{id}/status`, 20 % `POST /runs/{id}/progress`, 10 % `POST /tasks`, de 50 à 400/s | Débit maximal sous le verrou des événements ; point de rupture (latence p99 > 500 ms ou erreurs) |
| 2 | **Diffusion** : 10, 100, 500 abonnés SSE pendant le scénario 1 à 100/s | Délai de diffusion, coût CPU par abonné, impact des abonnés sur les écritures |
| 3 | **Tempête de reconnexions** : 500 clients coupés en même temps, qui reviennent avec `Last-Event-ID` | Rattrapage sans perte ; charge sur la base pendant le rattrapage |
| 4 | **Client lent** : un abonné qui ne lit plus | Il est coupé à 1 000 événements en attente ; les autres ne ralentissent pas |
| 5 | **Gros historique** : 100 000 événements dont 5 % de `run.finished` avec diff de 200 Kio, puis `?since=0` | Durée du rattrapage, mémoire, taille de la table et de l'index |
| 6 | **Agents réalistes** : 5 objectifs, 40 agents simulés qui enchaînent créations de tâches, messages (dont refus pour hop), relectures, fins de runs avec coût | Comportement global ; garde-fous corrects sous concurrence (aucun hop > 5, aucun dépassement de quota, budget cohérent) |
| 7 | **Endurance** : scénario 6 à charge nominale pendant 2 h | Fuites mémoire, goroutines, connexions du pool, croissance de la base |
| 8 | **Panne de base** : redémarrage de PostgreSQL pendant le scénario 2 | Le hub se reconnecte, les clients reprennent sans trou ; les écritures échouent proprement (`500 internal`) puis reprennent |

Le scénario 6 sert aussi de **test de concurrence** des règles métier : avec 40 agents en parallèle, le verrou des événements doit empêcher toute course (deux messages qui passeraient tous deux le hop 5, par exemple). Le rapport compte les violations : il doit y en avoir zéro.

## Environnement

- Profil compose dédié : `docker compose --profile load up` (PostgreSQL 16, `cfa-api`, `cfa-load` ; ni runner ni agents).
- Limites fixées pour des résultats reproductibles : `cfa-api` à 2 vCPU et 1 Go, PostgreSQL à 2 vCPU et 2 Go, réglages par défaut puis réglages ajustés (`shared_buffers`, `max_connections`).
- Machine de test distincte du poste de développement si possible ; sinon, noter la machine dans le rapport.
- Base vierge à chaque scénario, sauf le 5 et le 7.

## Instrumentation nécessaire

À ajouter avant la campagne :

| Élément | Contenu |
| --- | --- |
| OpenTelemetry (déjà en place) | Durée par route et statut (`otelhttp`), spans SQL, `cfa.events`, `cfa.sse.subscribers`, runs, tokens, coût ([architecture.md](architecture.md#observabilité-opentelemetry)) |
| À ajouter | Abonnés coupés pour lenteur, durée d'attente du verrou des événements, statistiques du pool pgx (`otelpgx.RecordStats`) |
| `pprof` | `net/http/pprof` sur une adresse séparée, activé par `CFA_PPROF_ADDR` |
| PostgreSQL | `pg_stat_statements`, `log_lock_waits`, `pg_stat_activity` (attentes sur le verrou consultatif) |
| Tableau de bord | `grafana/otel-lgtm` (profil `observability` de compose) |

## Goulots attendus et pistes

| Symptôme | Cause probable | Piste |
| --- | --- | --- |
| Débit d'écriture plafonné, attente sur `pg_advisory_xact_lock` | Transactions trop longues sous le verrou | Raccourcir les transactions (valider avant le verrou), prendre le verrou juste avant l'insertion de l'événement plutôt qu'en début de transaction |
| Débit encore insuffisant | Verrou global | Ordre par objectif (verrou par `goal_id`) et détection de trous côté hub (relire les ids manquants avant de diffuser) |
| CPU élevé avec beaucoup d'abonnés | Encodage JSON répété par abonné | Encoder une fois par événement dans le hub, diffuser les octets |
| Rafale de lectures en base | Une lecture par `NOTIFY` | Regrouper les notifications (fenêtre de 10 ms) avant `catchUp` |
| Rattrapage lent ou gourmand | Diffs volumineux dans `run.finished` | Diff stocké à part et chargé à la demande (avec un lien dans le payload), ou compression du payload |
| Pool saturé | Connexions tenues par des transactions sous verrou | Taille du pool, délai d'acquisition, métrique dédiée |

## Critères de réussite et rapport

Une campagne est réussie si les cibles chiffrées sont tenues, si le scénario 6 compte zéro violation des garde-fous et si l'endurance ne montre aucune croissance continue de mémoire ou de goroutines.

Le rapport (`results/report.md`, à joindre à la PR ou à l'ADR concernée) indique la version testée, la machine, les réglages, puis par scénario : débit, latences p50/p95/p99, délai SSE, erreurs par code, mémoire et CPU. Il compare avec la campagne précédente.

## Calendrier

1. Après la phase 2 : compléter l'instrumentation (métriques ci-dessus, `pprof`), puis `cfa-load write` et `sse` (scénarios 1 à 4).
2. Avant la première version publiée : scénarios 5 à 8 ; ajout d'un test de fumée de charge court (scénario 1 à 100/s pendant 30 s) dans la CI, qui échoue si le p95 dépasse la cible.
