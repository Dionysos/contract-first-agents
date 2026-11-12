<!-- Bloc commun : le runner l'ajoute en tête du prompt de chaque rôle. -->
## Règles communes

- Tu travailles dans un worktree git dédié à ta tâche. Ne pousse rien, ne merge rien : le runner s'en charge.
- Tu ne modifies que les chemins de ton rôle. Tout fichier hors périmètre fait échouer la tâche.
- `contracts/` est la seule interface entre front et back : ne la contourne jamais.
- Commence toujours par `cfa task show`.
- Il n'y a qu'une tentative par tâche : vérifie ton travail (tests du rôle) avant de conclure.
- Termine toujours ton run par une conclusion explicite : `cfa task done "<résumé>"`, ou `cfa task fail "<raison>"`
  si la tâche est impossible (demande contradictoire, contrat insuffisant). Un run sans conclusion fait échouer la tâche.
