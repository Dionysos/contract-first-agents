# 0004. Temps réel par `LISTEN/NOTIFY`, exposé en SSE, ordre garanti par verrou

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Le dashboard, la CLI (`cfa watch`) et le runner suivent le flux d'événements. Il faut un mécanisme simple côté navigateur, robuste aux reconnexions, et des identifiants d'événements dans l'ordre de validation : un client qui reprend à `Last-Event-ID` ne doit rien manquer.

## Décision

- Un trigger `pg_notify('cfa_events', id)` à chaque insertion ; un seul `LISTEN` par instance d'API, redistribué aux abonnés (`events.Hub`).
- Le flux est exposé en **Server-Sent Events** (`GET /events`), avec reprise par `Last-Event-ID` ou `since`, et un `: ping` toutes les 15 s.
- Chaque transaction d'écriture prend `pg_advisory_xact_lock(7243)` : les événements sont validés dans l'ordre de leurs identifiants. Sans cela, une transaction lente pourrait valider l'id 41 après que l'id 42 a été diffusé, et un client reprenant à 42 perdrait le 41.
- Abonnement avant lecture de l'historique, tampon de 1 000 événements par abonné ; un abonné trop lent est déconnecté et reprend.

## Conséquences

- `EventSource` natif dans le navigateur, sans bibliothèque ; `curl` suffit pour déboguer.
- Les écritures sont sérialisées : quelques centaines par seconde au mieux, très au-delà du besoin (voir [load-testing.md](../load-testing.md)).
- Plusieurs instances d'API fonctionnent (chacune a son `LISTEN`), mais le verrou reste global à la base.

## Alternatives écartées

- WebSocket : bidirectionnel inutile, plus de code, reconnexion à écrire.
- Polling : latence et charge.
- Ordre par `created_at` ou par séquence relue après coup : ne règle pas les validations dans le désordre.
