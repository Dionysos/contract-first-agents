# 0007. pi.dev comme harness d'agent, branché par un adaptateur `cfa-pi`

- **Statut** : Accepté
- **Date** : 2026-11-07

## Contexte

Le harness d'agent est pi.dev (choix du modèle par rôle). Le runner doit connaître, pour chaque run, les étapes (pour le dashboard), les tokens et le coût (pour le budget). Il ne doit pas dépendre du format interne de pi, qui évolue vite (0.73 au moment de l'intégration).

## Décision

- Le runner lance une **commande d'agent** (`agent_command`) qui respecte un protocole minimal sur sa sortie : des lignes `{"type":"progress"}` puis une ligne `{"type":"result"}` avec tokens et coût.
- `cfa-pi` (`cmd/cfa-pi`, `internal/piagent`) implémente ce protocole pour pi : il lance `pi --mode json --no-session`, traduit les événements, applique le plafond de tokens du run et signale les erreurs du modèle (pi sort avec le code 0 même dans ce cas).
- Les tarifs des modèles que pi ne connaît pas encore sont déclarés dans `build/pi/models.json` : sans eux pi compte un coût nul et le budget ne protège plus rien.

## Conséquences

- L'agent factice et pi sont interchangeables : les tests de bout en bout tournent sans LLM.
- Changer de harness = écrire un autre adaptateur, le runner ne change pas.
- Un run repart d'un prompt reconstruit (pas de reprise de session pi) : plus simple, un peu plus de tokens.
- Les tarifs de `models.json` sont à maintenir à chaque nouveau modèle.

## Alternatives écartées

- Lire la session pi depuis ses fichiers : format interne, moins stable que le mode JSON documenté.
- Mode RPC de pi : utile pour piloter une session longue, inutile pour un run non interactif.
