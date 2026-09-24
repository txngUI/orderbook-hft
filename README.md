# Order Book HFT

Moteur d'appariement d'un carnet d'ordres (*order book matching engine*), écrit en **Go**.

Projet réalisé pour le cours « Optimisations & Performances Backend » (Sup de Vinci, RNCP Bloc 4).
Le moteur ingère un flux d'ordres d'achat/vente et les apparie en temps réel pour produire des
transactions, en servant de support à une démarche d'optimisation mesurée (baseline → profiling →
leviers → comparatif).

** Ce readme n'est pas le rapport d'audit, il se trouve à la racine sous le nom RAPPORT-AUDIT disponible en .dm et en pdf**

## Résultats (séances 1–4)

De la baseline naïve à la version optimisée, sur `BenchmarkMatching` (n = 200 000, benchstat n=10,
AMD Ryzen 7 7735U) :

| | Baseline | Finale | Gain |
|---|---|---|---|
| Temps | 77,75 ms | **9,67 ms** | **÷ 8,0** |
| Allocations | 281 415 | **455** | **÷ 618** |

Détails, mesures avant/après et analyse : [`RAPPORT-AUDIT.md`](./RAPPORT-AUDIT.md).

## Fonctionnalités

- Carnet à deux côtés (*bids* / *asks*) avec priorité **prix-temps** (meilleur prix, puis FIFO).
- Ordres **LIMIT** (restent au carnet s'ils ne sont pas exécutés) et **MARKET** (exécutés au
  meilleur prix, reliquat annulé).
- Exécution partielle et traversée de plusieurs niveaux de prix.
- Générateur de flux **reproductible** (même graine → même flux) pour des mesures rejouables.
- **Visualisation HTML** du carnet (snapshot statique généré par le moteur).
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

## Visualisation (interface web)

Le moteur peut produire une **vue HTML du carnet** — profondeur *bids* / *asks*, spread, et les
dernières transactions — générée à partir d'un vrai run :

```bash
go run ./cmd/snapshot     # écrit snapshot.html à la racine
xdg-open snapshot.html    # ou ouvrir le fichier dans un navigateur
```

C'est un **snapshot statique** : une photo de l'état final du carnet, sans JavaScript. Le flux étant
déterministe (graine fixe), l'image est reproductible d'un run à l'autre. Le fichier `snapshot.html`
est une **sortie** (ignorée par git, régénérable à volonté). Une version **temps réel** (serveur +
WebSocket) est prévue pour la séance J4 (réseau).

## Structure

```
orderbook-hft/
├── cmd/
│   ├── bench/
│   │   └── main.go                 # point d'entrée : lanceur chronométré (binaire ./ob)
│   └── snapshot/
│       ├── main.go                 # lance le moteur puis remplit le gabarit HTML
│       └── snapshot.tmpl.html      # gabarit de la page (embarqué via //go:embed)
├── internal/
│   ├── engine/                     # cœur du moteur
│   │   ├── order.go                # types Order, Side, OrderType, Trade
│   │   ├── book.go                 # Book indexé par tick + appariement prix-temps
│   │   ├── snapshot.go             # photo en lecture seule du carnet (hors Hot Path)
│   │   ├── book_test.go            # tests de correction (filet de sécurité)
│   │   └── locality_test.go        # expérience de localité de cache (contigu vs dispersé)
│   ├── engine_test/
│   │   └── matching_bench_test.go  # BenchmarkMatching (boîte noire, 200 000 ordres)
│   └── feed/
│       └── generator.go            # générateur de flux d'ordres reproductible (graine fixe)
├── bench-results/                  # benchmarks archivés par étape (entrées de benchstat)
│   ├── baseline.txt
│   ├── opt-cache.txt
│   ├── opt-prealloc-trap.txt
│   ├── opt-ticks.txt
│   ├── opt-ticks-array.txt
│   └── opt-zero-alloc.txt
├── flamegraph-cpu.png              # flamegraph du profil CPU (goulot bestAsk/bestBid)
├── hyperfine.md                    # mesure end-to-end du binaire (./ob vs ./ob_opti)
├── snapshot.html                   # vue HTML du carnet — GÉNÉRÉE (ignorée par git)
├── Makefile                        # test, bench, profile, save, compare, clean…
├── constitution.md                 # règles de gouvernance technique (contraintes de perf)
├── RAPPORT-AUDIT.md                # rapport d'audit de performance (démarche + mesures)
├── .gitignore
├── go.mod
└── README.md
```

Les artefacts générés (`snapshot.html`, `cpu.prof`, `mem.prof`, `*.test`, `ob`, `ob_opti`) se
régénèrent (via le `Makefile` ou `go run ./cmd/snapshot`) et sont ignorés par git.

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
| Prix `int32` (compacité `Order` 32→24 o, densité cache) | ✅ |
| Visualisation HTML du carnet (snapshot statique) | ✅ |
| Concurrence / worker pool (séance J3) — ×6,35 sur 8 cœurs | ✅ |
| Recyclage des carnets (`sync.Pool`, séance J3) | ✅ |
| Réseau + persistance + interface temps réel (séance J4) | ⬜ à venir |

## Documentation

- **Rapport d'audit de performance** : voir [`RAPPORT-AUDIT.md`](./RAPPORT-AUDIT.md) — méthodologie,
  banc d'essai, mesures avant/après et analyse des leviers.
- **Gouvernance technique** : voir [`constitution.md`](./constitution.md).