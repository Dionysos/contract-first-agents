# 0010. Garde-fous de coordination

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Des agents autonomes peuvent tourner en rond (questions-réponses sans fin), réessayer indéfiniment une tâche impossible ou dépenser sans limite.

## Décision

- **hop** : 5 échanges au plus par fil ; au-delà, le message est refusé (`409 hop_limit_reached`) et une escalade est créée pour l'humain.
- **Quota** de messages par rôle et par objectif (30 par défaut).
- **3 tentatives** par tâche, puis escalade ; une escalade approuvée remet le compteur à zéro.
- **Budget** par objectif (défaut 5 $, plafond 20 $), avertissement à 80 %, arrêt et approbation `budget` à 100 % ; plafond de tokens par run.
- **Token par run**, révoqué à la fin du run ; **aucun agent ne merge** : le merge final passe par une approbation humaine.
- Les messages de l'humain ne comptent pas dans le hop.

## Conséquences

- Les refus sont des réponses métier (`409` avec un `code` stable), traités par les prompts, pas des erreurs techniques.
- L'humain n'intervient qu'aux points prévus (contrat cassant, escalade, budget, merge).

## Alternatives écartées

- Limites dans les prompts seulement : non garanties.
