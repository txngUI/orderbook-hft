# Order Book HFT

Moteur d'appariement d'un carnet d'ordres (*order book matching engine*), écrit en **Go**.

Projet réalisé pour le cours « Optimisations & Performances Backend » (Sup de Vinci, RNCP Bloc 4).
Le moteur ingère un flux d'ordres d'achat/vente et les apparie en temps réel pour produire des
transactions, en servant de support à une démarche d'optimisation mesurée (baseline → profiling →
leviers → comparatif).

> Ce README n'est pas le rapport d'audit. Celui-ci se trouve à la racine sous le nom `RAPPORT-AUDIT` (formats `.md` et `.pdf`).

## Résultats

De la baseline naïve à la version optimisée, sur `BenchmarkMatching` (n = 200 000, benchstat n=10,
AMD Ryzen 7 7735U) :

| | Baseline | Finale | Gain |
|---|---|---|---|
| Temps | 77,75 ms | **9,67 ms** | **÷ 8,0** |
| Allocations | 281 415 | **455** | **÷ 618** |

À cela s'ajoutent deux axes indépendants :
- **Parallélisme** — worker pool multi-symboles : **×6,35 débit** sur 8 cœurs.
- **Réseau** — comparatif REST/JSON vs gRPC/Protobuf : **P99 −30 %** en gRPC à 500 req/s.

Détails, mesures avant/après et analyse : [`RAPPORT-AUDIT.md`](./RAPPORT-AUDIT.md).

## Fonctionnalités

- Carnet à deux côtés (*bids* / *asks*) avec priorité **prix-temps** (meilleur prix, puis FIFO).
- Ordres **LIMIT** (restent au carnet s'ils ne sont pas exécutés) et **MARKET** (exécutés au
  meilleur prix, reliquat annulé).
- Exécution partielle et traversée de plusieurs niveaux de prix.
- Générateur de flux **reproductible** (même graine → même flux) pour des mesures rejouables.
- **Worker pool multi-symboles** (parallélisme entre carnets indépendants).
- **Visualisation HTML** du carnet (snapshot statique généré par le moteur).
- **Serveurs d'exposition** REST/JSON et gRPC/Protobuf (streaming bidirectionnel défini).
- Tests de correction et benchmarks (`go test`).

## Prérequis

- Go ≥ 1.21 (utilise les fonctions intégrées `min`/`max`).
- Pour la partie réseau : `protoc` (compilateur Protobuf) + plugins Go, `vegeta` (tir HTTP), `ghz` (tir gRPC).

## Démarrage rapide

Tout passe par le `Makefile` :

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

Le moteur produit une **vue HTML du carnet** — profondeur *bids* / *asks*, spread, et les
dernières transactions — générée à partir d'un vrai run :

```bash
go run ./cmd/snapshot     # écrit snapshot.html à la racine
xdg-open snapshot.html    # ou ouvrir le fichier dans un navigateur
```

C'est un **snapshot statique** : une photo de l'état final du carnet, sans JavaScript. Le flux étant
déterministe (graine fixe), l'image est reproductible d'un run à l'autre. Le fichier `snapshot.html`
est une **sortie** (ignorée par git, régénérable à volonté).

Pour un flux temps réel côté machine, le service gRPC `StreamOrders` (bidirectionnel HTTP/2) est
défini dans `proto/order.proto` — c'est le protocole adapté au contexte HFT (le WebSocket, orienté
navigateur, n'a pas sa place ici).

## Serveurs d'exposition

Deux serveurs exposent le moteur pour la comparaison de protocoles réseau :

```bash
make server-rest     # HTTP/JSON sur :8080 (POST /order, GET /trades, GET /health)
make server-grpc     # gRPC/Protobuf sur :50051 (Submit unaire + StreamOrders bidi)
make proto           # régénère les sources Protobuf/gRPC depuis proto/order.proto
```

Mesures de charge :

```bash
# REST via Vegeta
vegeta attack -duration=30s -rate=500/s -targets=targets.txt -body=body.json \
  -header="Content-Type: application/json" | vegeta report

# gRPC via ghz
ghz --insecure --proto=proto/order.proto \
  --call=orderbook.v1.OrderBookService/Submit \
  --data='{"id":1,"side":"SIDE_BUY","type":"ORDER_TYPE_LIMIT","price":9950,"quantity":1}' \
  --duration=30s --rps=500 --concurrency=50 localhost:50051
```

## Structure

```
orderbook-hft/
├── cmd/
│   ├── bench/                        # lanceur chronométré (binaire ./ob)
│   ├── snapshot/                     # génération de la vue HTML du carnet
│   ├── parallel/                     # worker pool multi-symboles
│   ├── contention/                   # démo de contention mutex (échec constructif)
│   ├── server-rest/                  # serveur HTTP/JSON
│   └── server-grpc/                  # serveur gRPC/Protobuf
├── internal/
│   ├── engine/                       # cœur du moteur (order.go, book.go, snapshot.go, reset.go...)
│   ├── engine_test/                  # BenchmarkMatching (boîte noire, 200 000 ordres)
│   ├── feed/                         # générateur de flux d'ordres reproductible
│   ├── api/                          # types + handler REST/JSON
│   └── pb/                           # sources gRPC générées (order.pb.go, order_grpc.pb.go)
├── proto/
│   └── order.proto                   # contrat gRPC (messages + service)
├── bench-results/                    # benchmarks archivés par tag
│   ├── baseline.txt · opt-cache.txt · opt-ticks.txt · opt-ticks-array.txt
│   ├── opt-zero-alloc.txt · opt-int32.txt · opt-prealloc-trap.txt
│   ├── rest-500rps.txt · rest-2000rps.txt
│   └── grpc-500rps.txt · grpc-2000rps.txt
├── flamegraph-cpu.png                # profil CPU du goulot bestAsk/bestBid
├── hyperfine.md                      # mesure end-to-end du binaire
├── Makefile · constitution.md · RAPPORT-AUDIT.md · go.mod
└── README.md
```

Les artefacts générés (`snapshot.html`, `cpu.prof`, `mem.prof`, `*.test`, `ob`, `ob_opti`) se
régénèrent via le `Makefile` et sont ignorés par git.

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
comportement des ordres MARKET. Ils servent de filet de sécurité : verts après chaque optimisation.

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
| Concurrence / worker pool — ×6,35 sur 8 cœurs | ✅ |
| Recyclage des carnets (`sync.Pool`) | ✅ |
| Profil de contention (échec constructif documenté) | ✅ |
| Serveur REST/JSON + tir Vegeta (P50/P95/P99) | ✅ |
| Serveur gRPC/Protobuf + tir ghz (P50/P95/P99) | ✅ |
| Comparatif réseau REST vs gRPC (§7 du rapport) | ✅ |

**Projet terminé.** Tous les leviers du programme sont implémentés, mesurés et documentés.

## Documentation

- **Rapport d'audit de performance** : voir [`RAPPORT-AUDIT.md`](./RAPPORT-AUDIT.md) — méthodologie,
  banc d'essai, mesures avant/après et analyse des leviers.
- **Gouvernance technique** : voir [`constitution.md`](./constitution.md).