# 0013. Session du dashboard par cookie et code de connexion à usage unique

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Le dashboard agit au nom de l'humain (approbations, nouvel objectif). Le token humain de la CLI ne doit pas être stocké dans le navigateur.

## Décision

`cfa dashboard` demande un code à usage unique (2 minutes) avec le token humain et ouvre `/login?code=…`. L'API l'échange contre un cookie `cfa_session` `HttpOnly`, `SameSite=Strict`, valable 12 heures (haché en base). Les écritures par cookie vérifient l'en-tête `Origin`.

## Conséquences

- Pas de mot de passe ni de gestion d'utilisateurs : un seul humain, celui qui a la CLI.
- Le code est consommé à la première ouverture : un navigateur qui l'ouvre avant l'utilisateur le « vole » (constaté pendant un test avec `xdg-open`).

## Alternatives écartées

- Token dans `localStorage` : exposé à tout script de la page.
- OAuth : disproportionné pour un outil mono-utilisateur.
