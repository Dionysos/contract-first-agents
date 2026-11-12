# 0001. Consigner les décisions d'architecture

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Le projet est conduit par un développeur et une équipe d'agents IA. Les choix structurants sont dispersés dans CLAUDE.md, `docs/cfa.md` et des fins de documents (« points à valider »). Dans six mois, personne ne saura pourquoi le bus n'est pas un broker ou pourquoi les branches s'appellent `task/…`. Les agents eux-mêmes ont besoin de connaître ces choix pour ne pas les défaire.

## Décision

Chaque décision structurante fait l'objet d'un ADR (*Architecture Decision Record*) dans `docs/adr/`, numéroté, au format court : contexte, décision, conséquences, alternatives écartées. Un ADR accepté n'est pas réécrit : s'il est remis en cause, un nouvel ADR le remplace et l'ancien passe au statut « Remplacé par NNNN ».

## Conséquences

- L'historique des choix est lisible et se relit en revue de code comme le reste.
- Une décision ne peut pas être changée en douce dans un document de référence : il faut un nouvel ADR.
- Coût : quelques minutes par décision.

## Alternatives écartées

- Tout garder dans CLAUDE.md : déjà trop long, et sans historique.
- Un wiki externe : sort du dépôt, donc des revues et du contexte des agents.
