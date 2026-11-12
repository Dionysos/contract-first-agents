# 0005. Les agents parlent au bus par la CLI `cfa`, pas par MCP

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Les agents (pi.dev) disposent d'un outil `bash`. Ils doivent créer des tâches, envoyer des messages, se bloquer en attente d'une réponse, conclure.

## Décision

Une CLI `cfa` (même dépôt Go que l'API) appelée par l'agent depuis son shell, authentifiée par le token du run (`CFA_TOKEN`). Sortie lisible par défaut, `--json` pour les scripts ; codes de sortie stables (`0`, `1` usage, `2` refus de l'API avec `code`, `3` API injoignable), cités dans les prompts des rôles.

## Conséquences

- Fonctionne avec n'importe quel harness qui sait lancer une commande ; aucun serveur MCP à maintenir.
- Le même outil sert à l'humain et aux agents : ce que fait un agent se rejoue à la main.
- Les commandes `cfa` apparaissent telles quelles dans les étapes du run (dashboard).
- Moins de structure qu'un schéma d'outil MCP : les prompts doivent documenter les commandes.

## Alternatives écartées

- Serveur MCP : couplage au support MCP du harness, et un composant de plus.
- Appels HTTP directs (`curl`) : prompts plus longs, erreurs moins lisibles.
