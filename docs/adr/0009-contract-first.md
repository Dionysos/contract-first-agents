# 0009. Contract-first : `contracts/` est la seule interface front/back

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Les rôles `front` et `back` travaillent en parallèle. S'ils se coordonnent en lisant le code l'un de l'autre, ils se bloquent ou divergent.

## Décision

- Les contrats (OpenAPI, schémas) vivent dans `contracts/` et sont écrits d'abord par le rôle `contracts`.
- `front` et `back` ne communiquent que par les contrats et par des messages (`question`, `contract_change`). Un changement cassant demande l'approbation humaine (`approvals.contract_change: breaking`).

## Conséquences

- Le planner place une tâche `contracts` avant les tâches qui en dépendent.
- Les échanges entre agents restent rares et visibles dans le dashboard.
- Une erreur dans un contrat coûte cher : d'où l'approbation des changements cassants.

## Alternatives écartées

- Coordination libre par messages : explosion des échanges (d'où aussi la limite de hop, [0010](0010-garde-fous.md)).
