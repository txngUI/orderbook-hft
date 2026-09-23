# Order Book HFT

Moteur d'appariement d'un carnet d'ordres (*order book matching engine*), écrit en **Go**.

Projet réalisé pour le cours « Optimisations & Performances Backend » (Sup de Vinci, RNCP Bloc 4).
Le moteur ingère un flux d'ordres d'achat/vente et les apparie en temps réel pour produire des
transactions, en servant de support à une démarche d'optimisation mesurée (baseline → profiling →
leviers → comparatif).

## Résultats (séances 1–4)

De la baseline naïve à la version optimisée, sur `BenchmarkMatching` (n = 200 000, benchstat n=10,
AMD Ryzen 7 7735U) :

| | Baseline | Finale | Gain |
|---|---|---|---|
| Temps | 77,75 ms | **10,70 ms** | **÷ 7,3** |
| Allocations | 281 415 | **457** | **÷ 615** |

Détails, mesures avant/après et analyse : [`RAPPORT-AUDIT.md`](./RAPPORT-AUDIT.md).

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

Tout passe par le `Makefile` (voir la cible détaillée dans le fichier) :

```bash
make test         # tests de correction (filet de sécurité)
make bench        # benchmark du moteur : temps, mémoire, allocations (10 runs)
make bench-cache  # expérience de localité de cache (contigu vs dispersé)
make profile      # profil CPU → flamegraph (go tool pprof -http=:8080 cpu.prof)
make save NAME=x  # archive un benchmark dans bench-results/x.txt
make compare A=baseline B=opt-zero-alloc   # comparatif benchstat entre deux versions
```

La baseline chronométrée seule : `go run ./cmd/bench`.

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
| Localité de cache (`[]*Order` → `[]Order` contigu) | ✅ |
| Profiling du goulot `bestAsk`/`bestBid` (O(n), ~61 % CPU) | ✅ |
| Prix `int64` (ticks) | ✅ |
| Structure indexée par tick → best-price O(1) | ✅ |
| Zéro-allocation (index de tête, recyclage des niveaux) | ✅ |
| Concurrence / worker pool (séance J3) | ⬜ à venir |
| Réseau + persistance (séance J4) | ⬜ à venir |

## Documentation

- **Rapport d'audit de performance** : voir [`RAPPORT-AUDIT.md`](./RAPPORT-AUDIT.md) — méthodologie,
  banc d'essai, mesures avant/après et analyse des leviers.
- **Gouvernance technique** : voir [`constitution.md`](./constitution.md).