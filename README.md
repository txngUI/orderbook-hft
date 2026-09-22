# Order Book HFT
 
Moteur d'appariement d'un carnet d'ordres (*order book matching engine*), écrit en **Go**.
 
Projet réalisé pour le cours « Optimisations & Performances Backend » (Sup de Vinci, RNCP Bloc 4).
Le moteur ingère un flux d'ordres d'achat/vente et les apparie en temps réel pour produire des
transactions, en servant de support à une démarche d'optimisation mesurée (baseline → profiling →
leviers → comparatif).
 
## Fonctionnalités
 
- Carnet à deux côtés (*bids* / *asks*) avec priorité **prix-temps** (meilleur prix, puis FIFO).
- Ordres **LIMIT** (restent au carnet s'ils ne sont pas exécutés) et **MARKET** (exécutés au
  meilleur prix, reliquat annulé).
- Exécution partielle et traversée de plusieurs niveaux de prix.
- Générateur de flux **reproductible** (même graine → même flux) pour des mesures rejouables.
- Tests de correction et benchmarks (`go test`).
## Prérequis
 
- Go ≥ 1.21 (utilise les fonctions intégrées `min`/`max`).
## Démarrage rapide
 
```bash
# Lancer la baseline chronométrée
go run ./cmd/bench
 
# Lancer les tests de correction
go test ./...
 
# Benchmark du moteur (temps, mémoire, allocations)
go test -bench=Matching -benchmem -count=3 ./internal/engine
 
# Expérience de localité de cache (contigu vs dispersé)
go test -bench='Contiguous|Dispersed' -benchmem ./internal/engine
```
 
> Si le projet est resté à plat (tous les `.go` à la racine), remplace `./cmd/bench` par `.` et
> `./internal/engine` par `.`.
 
## Structure
 
```
orderbook-hft/
├── cmd/
│   └── bench/          # point d'entrée : lanceur de baseline
├── internal/
│   ├── engine/         # cœur : Order, Book, moteur d'appariement + tests/benchmarks
│   └── feed/           # générateur de flux d'ordres reproductible
├── constitution.md     # règles de gouvernance technique (contraintes de perf)
├── RAPPORT-AUDIT.md    # rapport d'audit de performance (démarche + mesures)
└── README.md
```
 
## Concepts clés
 
- **Order** : un ordre (id, côté, type, prix, quantité).
- **Book** : le carnet, qui stocke les ordres au repos par niveau de prix et exécute le matching.
- **Trade** : une transaction produite par l'appariement de deux ordres.
## Tests & qualité
 
```bash
go vet ./...        # analyse statique
go test ./...       # tests de correction
```
 
Les tests couvrent : croisement simple, exécution partielle, traversée de plusieurs niveaux, et
comportement des ordres MARKET. Ils servent de filet de sécurité : ils doivent rester verts après
chaque optimisation.
 
## État d'avancement
 
| Étape | État |
|---|---|
| Baseline naïve (moteur correct + mesuré) | ✅ |
| Levier localité de cache (`[]*Order` → `[]Order` contigu) | ✅ |
| Profiling du goulot `bestAsk`/`bestBid` (O(n)) | ⬜ en cours |
| Structure de prix triée (O(n) → O(log n)/O(1)) | ⬜ |
| Autres leviers (int64 ticks, workers, streaming…) | ⬜ |
 
## Documentation
 
- **Rapport d'audit de performance** : voir [`RAPPORT-AUDIT.md`](./RAPPORT-AUDIT.md) — méthodologie,
  banc d'essai, mesures avant/après et analyse des leviers.
- **Gouvernance technique** : voir [`constitution.md`](./constitution.md).
 