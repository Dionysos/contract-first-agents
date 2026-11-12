# 0016. Dashboard en React, TypeScript et Bootstrap

- **Statut** : Accepté
- **Date** : 2026-11-11
- **Remplace** : [0012](0012-dashboard-sans-build.md)

## Contexte

Le dashboard en JavaScript natif (ADR 0012) tenait en un fichier `app.js` de 900 lignes : rendu par chaînes HTML, état non typé, aucun test. Chaque nouvel événement ou écran (approbations, formulaire, replay) alourdissait ce fichier, et rien ne signalait un écart avec les contrats de l'API.

## Décision

- **React 19 + TypeScript (strict), construit par Vite.** L'état reste `reduce(state, event)` sur le flux SSE ([0003](0003-journal-d-evenements-unique.md)), branché sur `useReducer`. Le reducer est pur et testé avec Vitest.
- **Bootstrap 5.3 via `react-bootstrap`** pour les composants (cartes, onglets, tableau, panneau latéral) et le mode sombre natif. Seuls les couleurs des rôles et le graphe des tâches ont un CSS propre.
- **Types générés depuis les contrats Go** : `cmd/cfa-tsgen` produit `dashboard/src/api/types.ts` par réflexion sur `internal/apitypes` et `internal/events`, dont une union discriminée des événements. Un test Go échoue si le fichier n'est plus à jour ; un champ renommé côté Go casse la compilation du dashboard.
- **Toujours embarqué dans `cfa-api`** : le build (`dashboard/dist`) est inclus par `go:embed`. L'image Docker le construit dans un stage Node ; `docker compose up` ne change pas.

## Conséquences

- Node est nécessaire pour développer le dashboard (`make ui`, `make ui-dev`), pas pour lancer le projet ni pour les tests Go.
- Sans build local, `cfa-api` démarre sans dashboard (avertissement au démarrage) : `dashboard/dist` ne contient que `.gitkeep` dans le dépôt.
- Rendu plus classique qu'un thème sur mesure, en échange de composants standard et accessibles.
- Un job CI dédié vérifie les types, lance les tests et construit le dashboard.

## Alternatives écartées

- Rester en JavaScript natif : pas de typage ni de composants, voir le contexte.
- templ + HTMX : rendu côté Go et presque pas de JavaScript, mais le graphe des tâches et l'état reconstruit par événements s'y prêtent mal.
- Générer les types avec tygo : même résultat, mais une dépendance de plus pour une centaine de lignes de réflexion.
