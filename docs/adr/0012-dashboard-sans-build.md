# 0012. Dashboard sans étape de build, piloté par les événements

- **Statut** : Remplacé par [0016](0016-dashboard-react-typescript-bootstrap.md)
- **Date** : 2026-11-07

## Contexte

Le dashboard doit montrer le travail en direct, rejouer un run enregistré et fonctionner en page statique (démo sur GitHub Pages), sans complexifier le dépôt Go.

## Décision

HTML, CSS et JavaScript natifs, sans dépendance ni CDN, embarqués dans `cfa-api` (`dashboard/embed.go`). L'état est `reduce(state, event)` sur le flux d'événements ([0003](0003-journal-d-evenements-unique.md)). Trois sources : SSE, fichier `.jsonl`, démo intégrée générée par `cmd/cfa-mock` avec les types Go réels.

## Conséquences

- Aucun outillage Node pour construire ou tester l'API ; la page marche hors ligne.
- Le replay est exact ; les actions (approbations, nouvel objectif) sont simulées localement en démo.
- Pas de composants ni de typage : le fichier `app.js` doit rester lisible (une fonction de rendu par zone).

## Alternatives écartées

- React/Vite : outillage et dépendances pour un écran unique.
