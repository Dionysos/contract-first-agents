# 0002. Bus maison : API HTTP en Go sur PostgreSQL

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Les agents doivent se passer des tâches et des messages, avec des règles (hop, quotas, approbations) et un historique complet. Les volumes sont faibles (quelques runs simultanés, quelques milliers d'événements par objectif), mais les garanties comptent : pas de message perdu, pas de double traitement, état cohérent après un redémarrage.

## Décision

Un service `cfa-api` en Go (`net/http`, `pgx`) devant PostgreSQL 16. Toutes les écritures passent par l'API, dans des transactions qui modifient l'état **et** écrivent l'événement correspondant. Pas de framework HTTP ni de broker de messages.

## Conséquences

- Un seul composant à état (PostgreSQL), sauvegardable avec `pg_dump`.
- Les règles métier vivent dans le service et dans les contraintes SQL (index partiels, `CHECK`), testées contre une vraie base.
- Pas de garantie de livraison à fournir côté broker : la base fait foi, le flux temps réel n'est qu'un signal.
- Limite : un seul nœud d'API pour l'instant (voir [0004](0004-temps-reel-listen-notify-sse.md)).

## Alternatives écartées

- NATS, Redis Streams, RabbitMQ : un composant de plus à opérer, et il faudrait quand même une base pour l'état et l'historique.
- MCP pour exposer le bus aux agents : voir [0005](0005-agents-via-cli.md).
