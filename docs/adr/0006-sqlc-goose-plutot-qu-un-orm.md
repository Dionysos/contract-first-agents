# 0006. Accès aux données avec sqlc et goose plutôt qu'un ORM

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

La demande initiale parlait d'« ORM ». Or les règles du bus reposent sur du SQL précis : verrous consultatifs, index uniques partiels (une approbation en attente par type), colonnes générées, `LISTEN/NOTIFY`, triggers.

## Décision

- **sqlc** génère des fonctions Go typées à partir de requêtes SQL écrites à la main (`internal/store/queries`), sur pgx. Le code généré est commité ; la CI vérifie qu'il est à jour (`sqlc diff`).
- **goose** applique les migrations SQL, embarquées dans le binaire et jouées au démarrage de `cfa-api`.

## Conséquences

- Le SQL exécuté est celui qu'on lit ; les erreurs de requête apparaissent à la génération.
- Pas de chargement implicite de relations ni de requêtes N+1 cachées.
- Il faut écrire chaque requête : acceptable pour une quinzaine de tables.

## Alternatives écartées

- GORM, ent : valeur faible ici, et ils masquent précisément ce dont on a besoin (verrous, index partiels).
