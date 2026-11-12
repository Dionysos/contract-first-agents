# 0011. Branches d'objectif `goal/<id>` et de tâche `task/<objectif>-<id>-<rôle>`

- **Statut** : Accepté (remplace le nommage de cfa.md d'origine)
- **Date** : 2026-11-07

## Contexte

La spécification d'origine prévoyait `goal/<id>` pour l'objectif et `goal/<id>/task-<n>` pour les tâches. Git range les références comme des fichiers : `refs/heads/goal/1` ne peut pas exister en même temps que le dossier `refs/heads/goal/1/`. Le test de bout en bout a échoué sur ce point.

## Décision

Branche d'objectif `goal/<id>`, branche de tâche `task/<goal_id>-<task_id>-<role>` (ex. `task/2-4-back`). Les deux noms sont des colonnes générées en base (`goals.branch`, `tasks.branch`), donc identiques partout (API, runner, dashboard).

## Conséquences

- Les deux espaces de noms sont disjoints ; le nom d'une tâche dit à quel objectif et à quel rôle elle appartient.
- Les bases créées avant ce changement gardent des branches à l'ancien format dans leurs événements (affichage seulement).

## Alternatives écartées

- `goal/<id>-tasks/<n>` : moins lisible.
- Pas de branche par tâche (commits directs sur la branche d'objectif) : plus de parallélisme possible ni de relecture par tâche.
