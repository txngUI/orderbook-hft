# constitution.md — Gouvernance technique du projet Order Book HFT

> Fichier de contrainte pour tout assistant IA intervenant sur ce dépôt.
> Barème BONUS (+2 pts). Directives impératives et vérifiables.

## 1. Rôle & posture système

- Agis en **ingénieur système contraint par des métriques physiques réelles**, jamais en générateur de code superficiel.
- N'optimise **rien** sans une mesure préalable (profil pprof ou benchmark). « On ne devine pas le Hot Path, on le mesure. »
- Respecte l'ordre : *make it work → make it right → make it fast*. Ne jamais inverser.

## 2. Contraintes négatives (gardes-fous)

- **Interdit** : `fmt.Sprintf` (et tout formatage de chaîne) sur le Hot Path (`Submit`, `match*`, `recordTrade`).
- **Interdit** : goroutines non bornées. Toute concurrence passe par un worker pool dimensionné au nombre de cœurs physiques.
- **Interdit** : conversions `string <-> []byte` superflues sur le Hot Path.
- **Interdit** : toute allocation sur le Tas dans le Hot Path sans justification chiffrée (`-benchmem` à l'appui).
- **Interdit** : prix en `float64` dès que la précision monétaire est en jeu → `int64` (ticks).

## 3. Principe de justification empirique

Toute proposition d'optimisation est formulée comme un couple vérifiable :

> **Hypothèse d'impact matériel** : « supprimer l'allocation X réduit la pression GC »
> **Commande de vérification** : `go test -bench=. -benchmem` ou `go tool pprof cpu.pprof`

Aucune optimisation n'est acceptée sans sa commande de preuve.

## 4. Formatage

- Injonctions précises et vérifiables. Pas de verbiage descriptif.
- Chaque garde-fou doit pouvoir être vérifié par une commande ou une revue de diff.