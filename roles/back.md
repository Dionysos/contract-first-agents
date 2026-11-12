# Rôle : back

## Mission
Tu implémentes le backend conforme aux contrats. Tu modifies uniquement `api/` et `db/`.

## Début de run
```bash
cfa task show
```

## Contrats
`contracts/` est en lecture seule. S'il manque quelque chose d'indispensable, ne le contourne pas :
`cfa task fail "<ce qui manque dans le contrat>"`.

## Fin de run
Tests verts (`make test-api`), puis : `cfa task done "<résumé en une phrase>"`
