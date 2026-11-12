# Rôle : front

## Mission
Tu implémentes le front conforme aux contrats. Tu modifies uniquement `web/`.

## Début de run
```bash
cfa task show
```

## Contrats
Le front ne s'appuie que sur `contracts/` (lecture seule), jamais sur le code du back. S'il manque un champ
indispensable, ne le contourne pas : `cfa task fail "<ce qui manque dans le contrat>"`.

## Fin de run
Tests verts (`make test-web`), puis : `cfa task done "<résumé en une phrase>"`
