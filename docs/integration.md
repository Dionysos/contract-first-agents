# Solution multi-agents — Utilisation et intégration

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). La [feuille de route](../README.md#feuille-de-route) indique ce qui est déjà en place.

## Vue d'ensemble

Un développeur dirige une équipe d'agents IA comme le ferait un tech lead : il formule des objectifs, suit le travail en direct et valide les décisions importantes. Les agents planifient, codent, se posent des questions et se relisent entre eux.

La solution assemble cinq briques :

- **Les agents** : pi.dev, un rôle par agent (planner, contracts, back, front, reviewer), modèle au choix par rôle.
- **Le bus cfa** : API Go + PostgreSQL pour les objectifs, tâches, messages et approbations. Détail dans [cfa.md](cfa.md).
- **Le runner** : lance chaque agent dans un conteneur isolé, sur sa propre branche git.
- **Le dashboard** : interface web temps réel pour soumettre les objectifs, suivre les agents, approuver, et rejouer des runs.
- **Le dépôt cible** : le projet sur lequel les agents travaillent, avec sa convention `contracts/`, `api/`, `db/`, `web/`.

Tout tourne sur une seule machine avec `docker compose up`.

## Description fonctionnelle

### Cycle de vie d'un objectif

```
Humain : nouvel objectif (dashboard ou CLI)
        │
        ▼
planner découpe en tâches ──► contracts ──► back ─┐
                                       └──► front ┤   (selon les dépendances)
                                                  ▼
        ┌──────────── besoin hors périmètre ? ── messages entre agents
        │                                          (question, contract_change)
        ▼
  tâche terminée ──► contrôles ──► reviewer ── corrections ──► retour à l'agent
                                     │ approuve
                                     ▼
              toutes les tâches faites : fusion dans goal/<id> + contrôles globaux
                                     │
                                     ▼
                  Humain : approbation de merge ──► main (ou PR)
```

L'humain n'intervient qu'à trois endroits : la soumission de l'objectif, les approbations en cours de route (changement de contrat cassant, escalade, budget) et le merge final.

### Soumettre une demande

Une demande est un **objectif** en langage naturel, avec un budget optionnel. Deux points d'entrée, qui appellent la même route `POST /goals` :

- **Dashboard** : formulaire « Nouvel objectif ». C'est le point d'entrée principal (détail ci-dessous).
- **CLI** : `cfa goal new "Ajouter la pagination à la liste des commandes" --budget 5`. Disponible dès la première version, avant le dashboard.

Plus tard : création depuis une issue GitHub ou GitLab portant un label dédié.

Un objectif bien formulé décrit le résultat attendu et les contraintes (« sans breaking change », « avec tests »), pas la façon de faire : le découpage est le travail du `planner`.

### Formulaire « Nouvel objectif »

Le formulaire s'ouvre depuis un bouton « Nouvel objectif » en haut de la vue Objectifs, dans un panneau latéral : la liste et l'activité restent visibles pendant la saisie.

```
┌─ Nouvel objectif ─────────────────────────────┐
│ Titre *                                       │
│ [Pagination de la liste des commandes       ] │
│                                               │
│ Description *                                 │
│ ┌───────────────────────────────────────────┐ │
│ │ La liste des commandes doit être paginée  │ │
│ │ (20 par page). Pas de breaking change sur │ │
│ │ l'API existante. Tests attendus.          │ │
│ └───────────────────────────────────────────┘ │
│ Résultat attendu et contraintes, pas la       │
│ façon de faire.                               │
│                                               │
│ Budget (USD)        [ 5.00 ]  max 20.00       │
│                                               │
│ ▸ Options avancées                            │
│   Branche de base   [ main ▾ ]                │
│                                               │
│            [ Annuler ]  [ Lancer l'objectif ] │
└───────────────────────────────────────────────┘
```

**Champs**

| Champ | Obligatoire | Règles | Défaut |
| --- | --- | --- | --- |
| Titre | oui | 5 à 120 caractères | — |
| Description | oui | 20 à 5 000 caractères, Markdown accepté | — |
| Budget (USD) | non | entre 0,50 et `budget.max_goal_usd` | `budget.per_goal_usd` |
| Branche de base | non | branche existante du dépôt cible | `base_branch` |

Les valeurs par défaut et les limites viennent de la configuration, exposées par `GET /config/public`.

**Comportement**

- **Validation** dans le navigateur (champs obligatoires, longueurs, bornes du budget), puis à nouveau par l'API, qui fait foi. Une erreur s'affiche sous le champ concerné.
- **Envoi** : le bouton est désactivé pendant l'appel pour éviter un double envoi ; la requête porte un en-tête `Idempotency-Key` généré à l'ouverture du formulaire, pour qu'un renvoi ne crée pas deux objectifs.
- **Succès** : le panneau se ferme et le dashboard ouvre la vue de l'objectif créé. On voit le `planner` démarrer puis les tâches apparaître en direct.
- **Échec** : erreurs de validation sous les champs ; autre erreur (API indisponible, session expirée) dans un bandeau en haut du formulaire, sans perdre la saisie.
- **Brouillon** : titre et description sont gardés dans le stockage local du navigateur jusqu'à l'envoi ou l'annulation, pour ne pas perdre une description longue.
- **Plusieurs objectifs** : on peut en soumettre un pendant qu'un autre tourne ; ils partagent la limite `concurrency` du runner. Un avertissement s'affiche si un objectif est déjà en cours.
- **Mode replay** : le bouton « Nouvel objectif » est masqué, remplacé par la mention « Run rejoué — lecture seule ».

**Accès au dashboard**

Le dashboard agit au nom de l'humain, il lui faut donc une session :

1. `cfa dashboard` (CLI, avec le token humain) demande un code de connexion à usage unique et ouvre `http://localhost:8080/login?code=…`.
2. L'API échange le code contre un cookie de session `HttpOnly`, `SameSite=Strict`, valable 12 heures.
3. Les routes d'écriture vérifient le cookie et l'en-tête `Origin`. Le token humain n'est jamais stocké dans le navigateur.

### Le dashboard

Une page web servie par l'API, mise à jour en temps réel par le flux SSE.

| Vue | Contenu |
| --- | --- |
| Objectifs | Liste avec statut, avancement, coût consommé / budget ; formulaire de création |
| Graphe des tâches | Tâches d'un objectif et leurs dépendances, colorées par statut, avec le rôle et la branche |
| Activité | Fil d'événements en direct, filtrable par rôle et par type |
| Fils | Conversation entre agents, avec le compteur d'échanges (`hop`) |
| Agents | Rôle, modèle, run en cours, dernier événement, tokens et coût cumulés |
| Approbations | Décisions en attente, mises en évidence, avec diff ou message associé ; boutons approuver / refuser |
| Run | Détail d'un run : prompt, étapes, fichiers modifiés, diff, résultat des contrôles |
| Replay | Chargement d'un fichier exporté, lecture / pause / vitesse |

### Approbations

| Type | Déclencheur | Si approuvée | Si refusée |
| --- | --- | --- | --- |
| `contract_change` | Changement cassant de `contracts/` (configurable : toujours, cassant seulement, jamais) | La tâche `contracts` continue | Le fil est renvoyé à l'émetteur avec le commentaire |
| `escalation` | Limite d'échanges ou de tentatives atteinte, désaccord | L'humain tranche dans le commentaire ; le travail reprend | La tâche ou l'objectif passe en échec |
| `budget` | Budget de l'objectif atteint | Budget augmenté, reprise | Objectif arrêté |
| `merge` | Objectif terminé, contrôles verts | Merge dans la branche de base ou PR ouverte | Retour au `planner` avec le commentaire |

### Rôles et responsabilités

| Acteur | Fait | Ne fait pas |
| --- | --- | --- |
| Humain | Soumet les objectifs, approuve, tranche les escalades | Ne code pas pendant un run sur les mêmes branches |
| `planner` | Découpe, ordonne et assigne les tâches | Ne modifie aucun fichier |
| `contracts` | Maintient `contracts/`, traite les `contract_change` | Ne code pas le front ou le back |
| `back`, `front` | Codent dans leur périmètre, posent des questions | Ne touchent pas `contracts/`, ne mergent pas |
| `reviewer` | Relit chaque tâche, approuve ou demande des corrections | Ne modifie aucun fichier |
| Runner | Lance les agents, vérifie périmètres et contrôles, fusionne, merge après approbation | Ne prend aucune décision de fond |

### Règles d'utilisation

- Un objectif = une branche `goal/<id>` ; une tâche = une branche `task/<id>-<n>-<rôle>` (pas `goal/<id>/…` : git refuse une branche `goal/<id>` et des branches sous `goal/<id>/`).
- `main` n'est modifiée qu'après une approbation humaine.
- Les échanges entre agents passent toujours par le bus, pour garder tout l'historique au même endroit.
- Un run important est exporté (`cfa export`) pour pouvoir être rejoué et montré.

## Description technique

### Prérequis

| Élément | Choix |
| --- | --- |
| Machine | Linux ou macOS, 4 vCPU, 8 à 16 Go RAM (inférence chez le fournisseur, pas en local) |
| Logiciels | Docker + compose (seul prérequis pour utiliser cfa) ; Go pour développer cfa |
| Accès | `localhost` par défaut ; Tailscale en option pour un accès distant |
| Comptes | Une clé API par fournisseur LLM utilisé (aucune pour le replay) |

### Organisation du dépôt cfa

```
cmd/cfa-api/     API HTTP + SSE + mode replay
cmd/runner/            runner
cmd/cfa/         CLI
internal/              domaine, stockage, contrôles, SSE
migrations/            schéma SQL versionné
dashboard/             front du dashboard (embarqué dans l'API au build)
roles/                 prompts des rôles (planner.md, contracts.md, back.md…)
build/                 Dockerfiles : api, runner, agent
runs/                  runs exportés pour le replay et la démo
cfa.yaml         configuration
docker-compose.yml
```

### Configuration

```yaml
target_repo: /srv/target.git
base_branch: main
remote: ""                          # si renseigné : push + PR au lieu du merge local
concurrency: 3                      # runs simultanés
agent_command: cfa-pi               # adaptateur pi.dev ; « fake-agent » pour l'agent factice

budget:
  per_goal_usd: 5                   # défaut proposé dans le formulaire
  max_goal_usd: 20                  # plafond accepté par l'API
  per_run_tokens: 400000
  per_run_timeout: 20m

limits:
  max_hop: 5
  max_attempts: 3
  messages_per_role_per_goal: 30

approvals:
  contract_change: breaking         # always | breaking | never

roles:
  # model : identifiant pi « fournisseur/modèle » ; thinking (facultatif) : off, minimal, low, medium, high, xhigh
  planner:   { model: "<fournisseur>/<modèle>", thinking: high, prompt: roles/planner.md, write: [] }
  contracts: { model: "<fournisseur>/<modèle>", prompt: roles/contracts.md, write: ["contracts/**"],
               checks: ["make contracts-lint", "make contracts-diff"] }
  back:      { model: "<fournisseur>/<modèle>", prompt: roles/back.md,      write: ["api/**", "db/**"],
               checks: ["make test-api"] }
  front:     { model: "<fournisseur>/<modèle>", prompt: roles/front.md,     write: ["web/**"],
               checks: ["make test-web"] }
  reviewer:  { model: "<fournisseur>/<modèle>", prompt: roles/reviewer.md,  write: [] }
```

Choisir des modèles différents par rôle (un modèle rapide pour le `reviewer`, un plus puissant pour le `planner`) est un des intérêts de pi.dev, et une comparaison intéressante à mesurer.

**Brancher pi.dev.** `cfa-pi` lance `pi --mode json` dans le worktree du run et remonte étapes, tokens et coût ([architecture.md](architecture.md#adaptateur-pidev-cmdcfa-pi-internalpiagent)). Il faut :

1. pi installé (`npm install -g @mariozechner/pi-coding-agent`, Node 20 ou plus ; déjà présent dans l'image du runner) ;
2. la clé du fournisseur de chaque modèle utilisé (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`…) dans l'environnement du runner (`.env`) ;
3. un tarif connu de pi pour chaque modèle : sinon le coût vaut 0 et le budget ne protège plus rien. Les modèles récents sont déclarés dans [build/pi/models.json](../build/pi/models.json) ; en local, copier ce fichier dans `~/.pi/agent/models.json` ou pointer `PI_CODING_AGENT_DIR` vers `build/pi`.

Vérification rapide : `pi --list-models sonnet` doit lister les modèles de `cfa.yaml`.

### Prompts des rôles et AGENTS.md

Deux niveaux d'instructions :

- **`roles/<rôle>.md`** (dépôt cfa) : mission du rôle, chemins autorisés, usage de la CLI, quand écrire aux autres, comment terminer. Injecté par le runner dans chaque run.
- **`AGENTS.md`** (dépôt cible, racine et sous-dossiers) : conventions du projet (style, tests, architecture). Lu par pi.dev comme dans n'importe quel projet.

Extrait de `roles/back.md` :

```markdown
## Mission
Tu es l'agent back. Tu modifies uniquement api/ et db/.

## Début de run
cfa task show       # spec et remarques de relecture
cfa inbox --unread

## Contrats
contracts/ est en lecture seule. S'il te manque quelque chose :
cfa send --to contracts --type contract_change "<besoin + justification>"
cfa task block --wait <id du message>
Puis termine ton run : tu seras relancé avec la réponse.

## Fin de run
Tests verts, puis : cfa task done "<résumé en une phrase>"
```

### Intégration git

- Le dépôt cible est un dépôt bare monté dans le runner. Chaque run travaille dans un worktree dédié.
- Les branches de tâches partent de `goal/<id>` ; le runner les y fusionne après relecture. Un conflit de fusion crée une escalade.
- Le merge dans `main` est fait par le runner, uniquement après l'approbation `merge`. Avec `remote` configuré, le runner pousse `goal/<id>` et ouvre une PR à la place.
- Les worktrees sont supprimés à la fin de l'objectif ; les branches sont conservées.

### Contrôles et CI

**Sur le dépôt cible**, les contrôles sont des commandes déclarées par rôle (`checks`), exécutées par le runner dans le conteneur après chaque run, puis sur `goal/<id>` avant le merge. Leur résultat est publié (`check.finished`) et, en cas d'échec, renvoyé à l'agent. Pour `contracts/` : lint OpenAPI / JSON Schema et détection des changements cassants par rapport à la branche de base.

Une CI externe (Jenkins, GitHub Actions) peut publier sur le bus avec un token `ci`.

**Sur le dépôt cfa** (GitHub Actions) :

- `go vet`, lint, tests unitaires ;
- tests d'intégration avec PostgreSQL (testcontainers) ;
- test de bout en bout avec l'agent factice : un objectif complet, du planner au merge, sans LLM ;
- test de replay : rejoue les fichiers de `runs/` et vérifie l'état final du dashboard ;
- build des images Docker.

### Replay et démo

Le mode replay rejoue un run exporté, sans clé API, sans base et sans runner (détail dans [cfa.md](cfa.md#mode-replay)).

```bash
cfa export 7 > runs/goal-7.jsonl          # après un vrai run
cfa-api --replay runs/goal-7.jsonl         # puis ouvrir http://localhost:8080
```

Usages :

- **Démo** : quelques runs choisis dans `runs/`, publiés avec le dashboard en page statique, accessibles depuis le README.
- **Analyse** : revoir pas à pas un run qui a mal tourné (boucle, mauvais découpage).
- **Non-régression** : vérifier qu'une évolution du dashboard affiche toujours correctement les anciens runs.

### Mesure

Un petit banc d'objectifs de référence permet de chiffrer la solution :

| Indicateur | Source |
| --- | --- |
| Taux de réussite (objectif mergé sans intervention autre que les approbations) | `goals`, `approvals` |
| Coût et durée moyens par objectif | `runs` |
| Nombre de messages, de corrections demandées, d'escalades | `messages`, `tasks`, `approvals` |
| Comparaison par modèle ou par configuration de rôles | `runs.model` |

`cfa bench <dossier d'objectifs>` soumet les objectifs, attend leur fin (approbations automatiques en mode bench) et produit un rapport.

### Exploitation

- **Supervision** : `/healthz` ; état des runs et des agents dans le dashboard.
- **Arrêt d'urgence** : `cfa goal cancel <id>` arrête les runs de l'objectif ; `docker compose stop runner` arrête tout.
- **Sauvegarde** : `pg_dump` de la base ; le code produit est dans les branches git.
- **Coûts** : suivis par run, par rôle et par objectif ; plafond aussi réglé chez chaque fournisseur.
- **Logs** : événements en base, logs de l'API et du runner via `docker compose logs`.

## Mise en route

Démarrage : `docker compose up -d --build --wait`, puis `docker compose run --rm cli dashboard` ([ADR 0015](adr/0015-docker-compose-d-abord.md)). Détail dans [infrastructure.md](infrastructure.md#démarrage).

- [x] Coder l'API, le schéma et la CLI (objectifs, tâches, messages, événements, SSE)
- [x] Coder le runner avec l'agent factice, et un test de bout en bout sans LLM
- [ ] Brancher pi.dev avec deux rôles (`contracts`, `back`) sur un dépôt cible
- [x] Dashboard en lecture seule (événements, graphe des tâches, fils)
- [ ] Approbations, `reviewer`, `front`, formulaire de nouvel objectif
- [x] Export et replay (page statique de démo : dashboard avec `demo.jsonl`, publication à faire)
- [ ] README avec schéma et GIF, ADR sur les choix principaux
- [ ] Banc de mesure et premiers chiffres
