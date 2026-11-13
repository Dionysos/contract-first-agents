# 0014. Observabilité : OpenTelemetry optionnel, export OTLP

- **Statut** : Accepté (phase 4 : pas encore implémenté)
- **Date** : 2026-11-07

## Contexte

Les journaux (`slog`) et le flux d'événements suffisent à suivre un objectif. Ils ne répondent pas à : quelle route est lente, où passe le temps d'un run (git, agent, contrôles), combien coûte chaque modèle par heure. Le projet doit aussi se lancer en une commande, sans backend d'observabilité obligatoire.

## Décision

- Instrumentation OpenTelemetry dans `cfa-api` et le runner (`internal/telemetry`) : traces des requêtes HTTP et des requêtes SQL, span par run côté runner avec ses étapes, propagation `traceparent` du runner vers l'API et jusqu'à la CLI `cfa` appelée par l'agent ; métriques métier (événements, abonnés SSE, runs, tokens, coût).
- **Désactivée par défaut** : sans `OTEL_EXPORTER_OTLP_ENDPOINT`, aucun exporteur n'est créé. Configuration par les variables standard `OTEL_*`.
- Les journaux restent en `slog`, enrichis de `trace_id` quand une trace est active.

## Conséquences

- Aucun coût ni dépendance de déploiement pour qui ne s'en sert pas ; compatible avec tout backend OTLP (Grafana LGTM, Jaeger, Honeycomb…).
- Quelques dépendances Go de plus (SDK OpenTelemetry, contrib `otelhttp`, `otelpgx`).
- Le flux SSE (`/events`) et `/healthz` ne sont pas tracés : une connexion SSE dure des heures.

## Alternatives écartées

- Prometheus seul : pas de traces, et un format de plus.
- Rien : acceptable aujourd'hui, mais le banc de mesure et les tests de charge ([load-testing.md](../load-testing.md)) en ont besoin.
