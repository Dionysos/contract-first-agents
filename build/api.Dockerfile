# Image de cfa-api : binaire statique (migrations et dashboard embarqués).

# Dashboard (React, TypeScript, Bootstrap) : build Vite dans dist/.
FROM node:22-alpine AS ui
WORKDIR /ui
COPY dashboard/package.json dashboard/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY dashboard/ ./
RUN npm run build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /ui/dist ./dashboard/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cfa-api ./cmd/cfa-api \
 && mkdir -p /out/state/runner /out/state/human

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cfa-api /cfa-api
# Tokens créés au démarrage (CFA_BOOTSTRAP_DIR) : un volume nommé monté sur chaque
# sous-dossier hérite de ce propriétaire (65532 = nonroot), sans shell dans l'image.
COPY --from=build --chown=65532:65532 /out/state /var/lib/cfa
ENV CFA_ADDR=0.0.0.0:8080 CFA_LOG_FORMAT=json
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/cfa-api"]
