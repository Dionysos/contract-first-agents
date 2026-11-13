# Dashboard

> **Cible complète.** Ce document décrit le système visé au terme des quatre phases, pas seulement le code de ce dépôt (phase 1). Une section qui commence par *Phase 2*, *Phase 3*, *Phase 4* ou *Prévu* n'est pas encore implémentée ; une note *En phase 1* signale ce qui diffère aujourd'hui. La [feuille de route](../README.fr.md#feuille-de-route) indique ce qui est déjà en place.

Le dashboard montre en direct le travail des agents : objectifs, graphe des tâches, conversations, approbations, coûts. Il sert aussi de lecteur pour les runs enregistrés (replay). Ce document décrit ce qu'il affiche, d'où viennent ses données et comment il est construit. Paramètres utiles (démos, captures) : `theme=dark|light`, `at=end|<n>` (position), `autoplay=0`, `speed=1|4|16`, `goal=<id>`, `tab=threads|approvals|runs`, `run=<id>` (ouvre le détail), `new` (ouvre le formulaire « Nouvel objectif »). Les vues et le formulaire sont présentés du point de vue de l'utilisateur dans [integration.md](integration.md#le-dashboard).

## Principes

- **Un seul flux de données : les événements.** L'état affiché est reconstruit en appliquant les événements dans l'ordre (`state = reduce(state, event)`). Aucune autre lecture de l'API n'est nécessaire. C'est ce qui rend le replay identique au direct.
- **Trois sources, un même moteur** : flux SSE de l'API, fichier `.jsonl` chargé dans le navigateur, ou démo intégrée.
- **React, TypeScript, Bootstrap** ([ADR 0016](adr/0016-dashboard-react-typescript-bootstrap.md)) : construit par Vite, embarqué dans le binaire `cfa-api`, sans CDN ; la page fonctionne hors ligne et, à terme, en page statique (phase 3).
- **Lecture d'abord.** Les actions (approuver, refuser, nouvel objectif) passent par les routes REST. Elles sont masquées ou désactivées en replay.

## Sources de données

*En phase 1* : seule la source Direct existe.

| Source | Quand | Comment |
| --- | --- | --- |
| Direct | Page servie par `cfa-api` avec une base | `EventSource('/events?since=0')`, authentifié par le cookie de session ([api.md](api.md#flux-sse)) |
| Replay serveur | `cfa-api --replay fichier.jsonl` | Même flux SSE, sans authentification ; `/healthz` répond `db: "replay"` |
| Fichier | Bouton « Ouvrir un run » ou glisser-déposer | Lecture du fichier dans le navigateur, puis lecteur local |
| Démo | `?demo`, page statique, ou API inaccessible | Charge `demo.jsonl` (embarqué), puis lecteur local |
| URL | `?src=/runs/goal-7.jsonl` | Charge un export publié à côté de la page (démos sur GitHub Pages) |

Au démarrage, la page interroge `/healthz` :

1. réponse `db: "replay"` : flux SSE en lecture seule ;
2. réponse `db: "ok"` : flux SSE en direct ; un `401` affiche « Lancez `cfa dashboard` pour ouvrir une session » avec un lien vers la démo ;
3. pas de réponse (page statique) ou `?demo` : démo.

## Lecteur (replay local)

*Phase 3 : pas encore implémenté.*

- Les événements du fichier sont chargés en mémoire. Le lecteur applique ceux dont l'index est inférieur à la position courante.
- **Lecture / pause**, **vitesse** ×1, ×4, ×16, **curseur** pour aller à n'importe quel événement, **fin** pour tout afficher d'un coup.
- Les intervalles d'origine sont respectés, mais chaque attente est plafonnée à 2 secondes (à ×1) : un run de 20 minutes sans événement ne fige pas la démo.
- Revenir en arrière recalcule l'état depuis le début : quelques centaines d'événements se rejouent en quelques millisecondes.

## Écran

*En phase 1* : pas de fils, de coûts, de budget, de tentatives ni de lecteur ; les approbations s'affichent sans boutons (décision par la CLI). Capture réelle : [dashboard-phase1-dark.png](screenshots/dashboard-phase1-dark.png).

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ cfa  ● Replay · demo.jsonl          Objectif ▾ Pagination…   $3.94 / $5.00  │
├───────────────┬──────────────────────────────────────────┬───────────────────┤
│ Objectifs     │ Graphe des tâches                        │ Activité          │
│ ▸ Pagination  │  [contracts ✓]──►[back ⟳ 2/3]            │ 14:02 front → back│
│   running     │           └────►[front ⏸ blocked]        │  question …       │
│ ▸ Export CSV  │                                          │ 14:01 run.finished│
│   planning    ├──────────────────────────────────────────┤  back  $0.41      │
│               │ Fils (2) │ Approbations (1) │ Runs (7)    │ …                 │
│               │ ┌ #31 Nullabilité de status    hop 2/5 ┐ │                   │
│               │ │ front: status peut-il être null ?    │ │ filtres : rôle,   │
│               │ │ back : Non, défaut 'draft'.          │ │ type              │
│               │ └──────────────────────────────────────┘ │                   │
├───────────────┴──────────────────────────────────────────┴───────────────────┤
│ Agents : planner ✓ · contracts ✓ · back ⟳ run #9 · front ⏸ · reviewer ·      │
├──────────────────────────────────────────────────────────────────────────────┤
│ ▶ ❚❚  ×1 ×4 ×16  ─────────●──────────────  87 / 142 événements   14:03:21    │
└──────────────────────────────────────────────────────────────────────────────┘
```

| Zone | Contenu | Événements utilisés |
| --- | --- | --- |
| En-tête | Source (Direct, Replay, Démo), objectif sélectionné, coût consommé / budget avec jauge | `goal.*`, `run.finished`, `budget.*` |
| Objectifs | Liste avec statut, coût, nombre de tâches terminées ; clic pour sélectionner | `goal.created`, `goal.status_changed`, `run.finished` |
| Graphe des tâches | Une colonne par niveau de dépendance, une carte par tâche : rôle, titre, statut (couleur), tentatives, branche ; flèches de dépendance | `task.created`, `task.status_changed`, `task.reviewed` |
| Fils | Conversations entre rôles, compteur `hop` / 5, statut du fil ; messages refusés signalés | `message.sent`, `message.rejected` |
| Approbations | Demandes en attente en premier, avec le contenu (changements cassants, diffstat et contrôles du merge, raison de l'escalade) ; boutons Approuver / Refuser | `approval.requested`, `approval.resolved` |
| Runs | Liste des runs (rôle, type, modèle, durée, tokens, coût) ; clic pour le détail | `run.*`, `check.finished` |
| Activité | Fil chronologique, filtrable par rôle et par type | tous |
| Agents | Une pastille par rôle : run en cours, dernier statut, coût cumulé | `run.*` |
| Lecteur | Visible en replay et en démo | — |

**Détail d'un run** (panneau latéral) : rôle, modèle, durée, tokens, coût, étapes (`run.progress`), contrôles (`check.finished`), fichiers modifiés et diff coloré.

### Couleurs des statuts

| Statut de tâche | Couleur |
| --- | --- |
| `todo` | gris |
| `running` | bleu, animé |
| `blocked` | ambre |
| `review` | violet |
| `changes_requested` | orange |
| `done` | vert |
| `failed` | rouge |

Les couleurs sont doublées d'une icône et d'un libellé : l'information ne passe jamais par la couleur seule.

## Code

```
dashboard/
  index.html, vite.config.ts   point d'entrée et build (Vite)
  src/main.tsx, App.tsx        montage, mise en page, thème (?theme=dark|light)
  src/useEventStream.ts        connexion SSE (EventSource, reprise par Last-Event-ID)
  src/state.ts                 réducteur : état dérivé des événements (useReducer)
  src/api/types.ts             types générés depuis les contrats Go (make types)
  src/components/              TopBar, GoalList, TaskGraph, DetailTabs, ActivityFeed, AgentBar, RunDrawer, StatusBadge
  embed.go                     embarquement du build dans cfa-api
```

Prévus : le lecteur de replay et la démo intégrée (phase 3), les fils et les coûts (phase 2), les actions REST depuis la page (phase 3).

Tout texte venant des événements est échappé avant insertion dans la page.

## Données de démo

*Phase 3 : pas encore implémenté.*

`cmd/cfa-mock` génère `dashboard/demo.jsonl` avec les types Go des contrats (`apitypes`, `events`) : la démo respecte donc exactement le format réel. Le scénario couvre :

- un objectif terminé, un objectif en attente de l'approbation de merge, un objectif en planification ;
- trois tâches avec dépendances (contrat, back, front) ;
- une question entre `front` et `back`, avec une tâche bloquée puis relancée ;
- un `contract_change` avec un changement cassant soumis à l'humain ;
- une relecture qui demande des corrections, puis une deuxième tentative ;
- un avertissement de budget ;
- un message refusé par la limite d'échanges.

```bash
go run ./cmd/cfa-mock > dashboard/demo.jsonl
```

## Formulaire « Nouvel objectif »

*Phase 3 : pas encore implémenté.*

Bouton « ＋ Nouvel objectif » en haut du panneau Objectifs ; le formulaire s'ouvre dans le panneau latéral. Règles fonctionnelles : [integration.md](integration.md#formulaire-nouvel-objectif).

| Source | Comportement |
| --- | --- |
| Direct | `GET /config/public` (budget par défaut, bornes, branches), puis `POST /goals` avec le cookie de session et un `Idempotency-Key` généré à l'ouverture |
| Démo, fichier | Envoi simulé : le navigateur ajoute au lecteur les événements qu'aurait produits le système (`goal.created`, `run.started` du planner, deux étapes) |
| Replay serveur | Bouton masqué, mention « Run rejoué — lecture seule » |

- Validation locale (longueurs, bornes du budget), puis validation de l'API : les erreurs `errors.<champ>` s'affichent sous le champ concerné, les autres (session expirée, API injoignable) dans un bandeau, sans perdre la saisie.
- Brouillon (titre, description) dans `localStorage` (`cfa.goal-draft`), effacé à l'envoi ou à l'annulation ; l'accès est protégé (navigation privée).
- Succès : panneau fermé, objectif créé sélectionné ; on voit le planner démarrer en direct.
- Paramètre `?new` : ouvre le formulaire au chargement (captures d'écran).

Captures : [dashboard-new-goal.png](screenshots/dashboard-new-goal.png) (démo), [dashboard-live-new-goal-errors.png](screenshots/dashboard-live-new-goal-errors.png) (direct, erreurs de validation).

**Étapes d'un run.** Le détail d'un run affiche le type de chaque étape (`cfa`, `bash`, `write`, `retry`…). Les commandes `cfa` (coordination) sont mises en avant ; `warning` (coût inconnu) et `tool_error` sont en ambre.

## Évolutions prévues

- Lien profond vers un objectif ou un run (`#goal=7&run=12`).
- Vue graphe pleine page pour les gros objectifs.
