# Décisions d'architecture (ADR)

Un ADR (*Architecture Decision Record*) consigne **une** décision structurante : le contexte, ce qui a été décidé, les conséquences et les alternatives écartées. Il est court, daté, et n'est jamais réécrit : une décision remise en cause donne un nouvel ADR qui remplace l'ancien. Format et règles : [0001](0001-consigner-les-decisions.md).

| N° | Décision | Statut |
| --- | --- | --- |
| 0001 | [Consigner les décisions d'architecture](0001-consigner-les-decisions.md) | Accepté |
| 0002 | [Bus maison : API HTTP en Go sur PostgreSQL](0002-bus-http-go-postgresql.md) | Accepté |
| 0003 | [Une table `events` pour le temps réel, l'historique et le replay](0003-journal-d-evenements-unique.md) | Accepté |
| 0004 | [Temps réel par `LISTEN/NOTIFY`, exposé en SSE, ordre garanti par verrou](0004-temps-reel-listen-notify-sse.md) | Accepté |
| 0005 | [Les agents parlent au bus par la CLI `cfa`, pas par MCP](0005-agents-via-cli.md) | Accepté |
| 0006 | [Accès aux données avec sqlc et goose plutôt qu'un ORM](0006-sqlc-goose-plutot-qu-un-orm.md) | Accepté |
| 0007 | [pi.dev comme harness d'agent, branché par un adaptateur `cfa-pi`](0007-pidev-via-adaptateur.md) | Accepté |
| 0008 | [Isolation : un worktree git par run, puis un conteneur éphémère](0008-isolation-par-run.md) | Accepté (conteneur : à faire) |
| 0009 | [Contract-first : `contracts/` est la seule interface front/back](0009-contract-first.md) | Accepté |
| 0010 | [Garde-fous de coordination](0010-garde-fous.md) | Accepté |
| 0011 | [Branches d'objectif `goal/<id>` et de tâche `task/<objectif>-<id>-<rôle>`](0011-nommage-des-branches.md) | Accepté (remplace le nommage de cfa.md d'origine) |
| 0012 | [Dashboard sans étape de build, piloté par les événements](0012-dashboard-sans-build.md) | Remplacé par 0016 |
| 0013 | [Session du dashboard par cookie et code de connexion à usage unique](0013-session-dashboard.md) | Accepté |
| 0014 | [Observabilité : OpenTelemetry optionnel, export OTLP](0014-opentelemetry-optionnel.md) | Accepté |
| 0015 | [Docker compose d'abord : `docker compose up` suffit](0015-docker-compose-d-abord.md) | Accepté |
| 0016 | [Dashboard en React, TypeScript et Bootstrap](0016-dashboard-react-typescript-bootstrap.md) | Accepté (remplace 0012) |

## Nouvel ADR

Copier [modele.md](modele.md) sous le numéro suivant, remplir, ajouter la ligne ci-dessus et faire relire avec le changement concerné.
