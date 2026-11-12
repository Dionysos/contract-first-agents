#!/bin/sh
# Test de bout en bout sous docker compose, avec l'agent factice : un objectif du planner au merge.
# Prérequis : jq. Usage : docker compose up -d --build --wait && scripts/compose-smoke.sh
set -eu

cfa() { docker compose run --rm -T cli --json "$@"; }

id=$(cfa goal new "Pagination de la liste des commandes" --description "20 par page, tests attendus." | jq -r .id)
echo "objectif $id créé"

deadline=$(( $(date +%s) + ${SMOKE_TIMEOUT:-300} ))
while [ "$(date +%s)" -lt "$deadline" ]; do
  goal=$(cfa goal show "$id")
  status=$(echo "$goal" | jq -r .status)
  echo "objectif $id : $status"
  case "$status" in
    done)
      docker compose exec -T runner git -C /srv/target.git log --oneline -3 main
      exit 0 ;;
    failed|cancelled)
      docker compose logs --tail 100 runner >&2
      exit 1 ;;
  esac
  # Approuve le merge quand il est demandé.
  for a in $(echo "$goal" | jq -r '.approvals[]? | select(.status == "pending") | .id'); do
    cfa approve "$a" --comment "smoke" >/dev/null
    echo "approbation $a acceptée"
  done
  sleep 3
done
echo "délai dépassé" >&2
docker compose logs --tail 100 runner >&2
exit 1
