# 0015. Docker compose d'abord : `docker compose up` suffit

- **Statut** : Accepté
- **Date** : 2026-11-10

## Contexte

Le projet doit se lancer en une commande. Le compose existait mais demandait un `.env` complet, la création manuelle de deux tokens entre deux `docker compose up`, et un dépôt cible bare préparé à la main. Le README présentait Go comme le chemin principal ; Docker n'y était qu'une option de déploiement.

## Décision

`docker compose up` est le point d'entrée pour utiliser, montrer et tester cfa :

- **Sans `.env`** : la pile démarre avec l'agent factice (`CFA_AGENT=fake-agent`), `cfa.example.yaml` comme configuration et un mot de passe PostgreSQL par défaut (base non exposée hors du réseau compose).
- **Tokens créés au démarrage** : avec `CFA_BOOTSTRAP_DIR`, `cfa-api` écrit après les migrations un token runner et un token humain, chacun dans son volume (`runner-token`, `human-token`). Un fichier existant est gardé tant que son token est valide. Le runner, et donc les agents qu'il lance en sous-processus, ne montent jamais le token humain.
- **Dépôt cible de démonstration** : avec `CFA_INIT_TARGET=1`, le runner initialise un dépôt bare dans le volume `target` s'il est vide. `TARGET_REPO` dans `.env` le remplace par un dépôt de l'hôte.
- **CLI sans Go** : le service `cli` (profil `cli`, image du runner) lit le token humain : `docker compose run --rm cli <commande>`.
- **Démo** : le service `demo` (profil `demo`) rejoue `runs/demo.jsonl` sur le port 8081.
- **CI** : un job `compose` démarre la pile sans `.env` et mène un objectif du planner au merge (`scripts/compose-smoke.sh`).

Les cibles Go (`make test`, `cfa-devdb`, PostgreSQL embarqué) restent le chemin pour développer cfa lui-même.

## Conséquences

- Un premier essai ne demande que Docker ; passer à pi.dev, c'est `CFA_AGENT=cfa-pi` et une clé dans `.env`.
- Les valeurs par défaut sont des valeurs de poste de développement. Le port n'est publié que sur `127.0.0.1`, mais un déploiement partagé doit fixer `PG_PASSWORD` avant le premier démarrage.
- Le token humain est dans un volume : `cfa` installé sur l'hôte doit le récupérer (`docker compose run --rm --entrypoint cat cli /run/cfa/token > ~/.config/cfa/token`).
- Un dépôt cible monté depuis l'hôte doit être accessible en écriture par l'uid 1001 du runner.
- `CFA_AGENT` remplace `agent_command` du fichier de configuration, y compris dans un `cfa.yaml` personnalisé.

## Alternatives écartées

- Service d'initialisation séparé (conteneur à usage unique) pour les tokens : un service de plus, et l'image `cfa-api` (distroless, sans shell) aurait de toute façon besoin d'une commande Go dédiée. Le faire dans `cfa-api serve` garantit en plus que les tokens existent dès que l'API est saine, condition déjà attendue par le runner.
- Token fixe dans `.env.example` : secret identique pour tout le monde, et `.env` redevient obligatoire.
- Un seul volume pour les deux tokens : le token humain serait lisible par les agents, qui pourraient approuver leur propre merge.
