# Image du runner : runner, CLI cfa, adaptateur cfa-pi, agent factice, git et pi (pi.dev).
# Sert aussi au service « cli » de docker compose (entrypoint cfa).
# Les agents sont pour l'instant lancés en sous-processus dans ce conteneur.
# Le lancement en conteneurs Docker isolés est l'étape suivante : voir docs/infrastructure.md.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/runner ./cmd/cfa ./cmd/cfa-pi ./cmd/fake-agent

FROM node:22-alpine
ARG PI_VERSION=0.73.1
RUN apk add --no-cache git ca-certificates make \
 && npm install -g --omit=dev @mariozechner/pi-coding-agent@${PI_VERSION} \
 && npm cache clean --force \
 && adduser -D -u 1001 cfa \
 && install -d -o cfa -g cfa /srv/work /srv/target.git \
 && install -d -o 65532 -g 65532 /run/cfa
# Un volume nommé monté sur ces dossiers hérite de leur propriétaire. /run/cfa (token lu par
# CFA_TOKEN_FILE) appartient à l'utilisateur de cfa-api, qui y écrit, quel que soit le
# conteneur qui monte le volume en premier.
COPY --from=build /out/ /usr/local/bin/
COPY roles /srv/roles
# Scénario de l'agent factice (agent_command: fake-agent), le même que le test de bout en bout.
COPY internal/runner/testdata/scenario.yaml /srv/fake/scenario.yaml
# Modèles que pi ne connaît pas encore (fenêtres de contexte, tarifs).
COPY --chown=cfa build/pi/models.json /srv/pi/models.json
USER cfa
WORKDIR /srv
# PI_OFFLINE : pas de vérification de version ni de télémétrie au démarrage de pi.
ENV CFA_LOG_FORMAT=json PI_OFFLINE=1 PI_CODING_AGENT_DIR=/srv/pi CFA_FAKE_SCENARIO=/srv/fake/scenario.yaml
ENTRYPOINT ["/usr/local/bin/runner"]
