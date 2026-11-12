# 0008. Isolation : un worktree git par run, puis un conteneur éphémère

- **Statut** : Accepté (conteneur : à faire)
- **Date** : 2026-11-07

## Contexte

Plusieurs agents travaillent en parallèle sur le même dépôt cible. Un agent ne doit ni voir le travail en cours d'un autre, ni écrire hors de son périmètre, ni avoir accès aux secrets du runner.

## Décision

- Un **worktree git** par run, sur la branche de la tâche ; le runner commite, calcule le diff et vérifie le périmètre (`write` du rôle) après le run. Un fichier hors périmètre renvoie la tâche en corrections. `contracts/` n'est modifiable que par le rôle `contracts`.
- Cible : un **conteneur Docker éphémère** par run, sur un réseau `agents` sans accès à la base, sortie limitée aux fournisseurs LLM par un proxy (voir [infrastructure.md](../infrastructure.md)).

## Conséquences

- Le périmètre est vérifié après coup, pas empêché : un agent peut écrire hors périmètre, mais son travail est refusé.
- **État actuel** : les agents tournent en sous-processus dans le conteneur du runner. Tant que le conteneur par run n'existe pas, réserver pi.dev à des dépôts de confiance.

## Alternatives écartées

- Un clone complet par run : lent et lourd.
- Montages en lecture seule par dossier : complexe, et ne couvre pas les fichiers à la racine.
