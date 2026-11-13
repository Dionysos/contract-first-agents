# 0003. Une table `events` pour le temps réel, l'historique et le replay

- **Statut** : Accepté (export et replay : phase 3)
- **Date** : 2026-11-07

## Contexte

Trois besoins se ressemblent : le dashboard en direct, l'historique d'un objectif et le rejeu d'un run sans clé API (démo, débogage). Les servir par trois mécanismes différents garantit qu'ils divergeront.

## Décision

Chaque écriture produit un événement **autosuffisant** dans la table `events` (ajout seul, protégée par trigger). Le dashboard et le runner reconstruisent leur état uniquement à partir de ce flux. L'export JSON Lines (`cfa-events/1`) est une copie de ce flux, rejouable par `cfa-api --replay` ou dans le navigateur.

## Conséquences

- Replay identique au direct, par construction ; démo et captures d'écran sans backend.
- Les événements doivent porter tout ce qu'il faut pour être affichés (titre de la tâche, rôle…) : un peu de duplication assumée.
- Changer le format d'un événement est un changement de contrat : documenté dans [data-contracts.md](../data-contracts.md), versionné par le format d'export.
- Le runner n'a besoin d'aucune route de lecture en plus : il est redémarrable sans état local.

## Alternatives écartées

- Event sourcing complet (l'état dérivé des seuls événements, côté API aussi) : plus lourd, sans bénéfice ici, l'état relationnel reste la source pour les règles.
- Tables d'audit par entité : pas de flux ordonné unique, replay impossible.
