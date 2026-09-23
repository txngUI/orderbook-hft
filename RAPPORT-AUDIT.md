# Rapport d'Audit de Performance — Order Book HFT

| Élément | Détail |
|---|---|
| Auteur | Tanguy David |
| Cours | Optimisations & Performances Backend — Sup de Vinci, RNCP Bloc 4 |
| Projet | Order Book HFT (moteur d'appariement, Go) |
| Dépôt | `https://github.com/txngUI/orderbook-hft` — baseline figée au tag `baseline-naive` |

---

## Sommaire

- [Introduction](#introduction)
  - [Contexte et objectif du document](#contexte-et-objectif-du-document)
  - [Présentation du sujet](#présentation-du-sujet)
  - [Pourquoi ce sujet pour un TP de performance](#pourquoi-ce-sujet-pour-un-tp-de-performance)
  - [Démarche](#démarche)
  - [Portée](#portée)
- [1. Environnement & Métrologie (Baseline) — /3](#1-environnement--métrologie-baseline--3)
  - [1.1 Banc d'essai matériel](#11-banc-dessai-matériel)
  - [1.2 Protocole de mesure](#12-protocole-de-mesure)
  - [1.3 Résultats](#13-résultats)
    - [1.3.1 Chronométrage simple](#131-chronométrage-simple-première-mesure--séance-1)
    - [1.3.2 Baseline consolidée](#132-baseline-consolidée-référence-pour-toutes-les-comparaisons-avantaprès)
- [2. Diagnostic matériel & Profiling réel — /5](#2-diagnostic-matériel--profiling-réel--5)
  - [2.1 Méthode](#21-méthode)
  - [2.2 Profil CPU (top cumulé)](#22-profil-cpu-top-cumulé)
  - [2.3 Identification formelle du Hot Path](#23-identification-formelle-du-hot-path)
- [3. Journal d'optimisation & Démarche d'ingénierie — /5](#3-journal-doptimisation--démarche-dingénierie--5)
  - [3.0 Tableau des leviers identifiés](#30-tableau-des-leviers-identifiés-choix-naïfs-de-la-baseline)
  - [3.1 Localité de cache (`[]*Order` → `[]Order`)](#31-levier--localité-de-cache-order-→-order)
  - [3.2 Origine des allocations (escape analysis)](#32-analyse--origine-des-allocations-escape-analysis)
  - [3.3 Prix en `int64` (ticks)](#33-levier--prix-en-int64-ticks)
  - [3.4 `map` → tableau de ticks (best-price O(1))](#34-levier--map-→-tableau-de-ticks-best-price-o1)
  - [3.5 Zéro-allocation par index de tête](#35-levier--zéro-allocation-par-index-de-tête)
  - [3.6 Parallélisation (worker pool multi-symboles)](#36-levier--parallélisation-worker-pool-multi-symboles)
  - [3.7 Recyclage des carnets (`sync.Pool`)](#37-levier--recyclage-des-carnets-syncpool)
  - [3.8 Compacité du prix (`int64` → `int32`)](#38-levier--compacité-du-prix-int64-→-int32)
- [4. Confrontation critique & « Échec constructif » — /3](#4-confrontation-critique--«-échec-constructif-»--3)
  - [4.1 Pré-allocation avide des niveaux](#41-pré-allocation-avide-des-niveaux-«-over-provisioning-»)
  - [4.2 Paralléliser le matching d'un seul carnet (contention)](#42-paralléliser-le-matching-dun-seul-carnet-contention)
- [5. Reproductibilité & Synthèse comparative — /4](#5-reproductibilité--synthèse-comparative--4)
  - [5.1 Automatisation](#51-automatisation)
  - [5.2 Synthèse des gains](#52-synthèse-des-gains)
- [6. BONUS — Gouvernance technique (`constitution.md`) — +2](#6-bonus--gouvernance-technique-constitutionmd--2)
- [Annexes](#annexes)

---

## Introduction

### Contexte et objectif du document

Ce rapport constitue l'audit de performance du projet **Order Book HFT**, réalisé dans le cadre du
cours « Optimisations & Performances Backend ». Son objectif est de **prouver, chiffres à l'appui**,
l'amélioration des performances obtenue en appliquant successivement les leviers d'optimisation vus
en cours (localité de cache, zéro-allocation, concurrence, etc.), en partant d'une version naïve de
référence (_baseline_) et en mesurant chaque gain de façon reproductible.

### Présentation du sujet

Un **carnet d'ordres** (_order book_) est le cœur d'une place de marché financière. Il maintient les
ordres d'achat (_bids_) et de vente (_asks_) en attente, et un **moteur d'appariement**
(_matching engine_) tente, à chaque nouvel ordre, de marier acheteurs et vendeurs pour produire des
transactions (_trades_), selon une priorité **prix-temps** (meilleur prix d'abord, puis ordre
d'arrivée). Deux types d'ordres sont gérés : **LIMIT** (reste au carnet s'il n'est pas exécuté) et
**MARKET** (exécuté immédiatement au meilleur prix, reliquat annulé).

### Pourquoi ce sujet pour un TP de performance

Le moteur d'appariement est une **boucle exécutée des millions de fois sous charge** : c'est un
_Hot Path_. Chaque micro-coût (une allocation, un défaut de cache, une recherche en
O(n)) y est amplifié par le volume, ce qui en fait un sujet idéal pour s'entraîner aux différents moyens d'optimisation matérielle et algorithmique.

### Démarche

On réalise dans un premier temps une version naïve mais **fonctionnelle** du projet : c'est la baseline. Par la suite, on **mesure les métriques** et on observe s'il y a besoin d'optimiser. On **fait** des hypothèses, on **résout**, on mesure et on analyse. On vérifie le goulot (si **c'en** est finalement un).

**Traçabilité (versionnement).** Chaque état mesurable du projet est figé par un **tag Git**, et
chaque levier fait l'objet d'un commit taggé distinct. La sortie de benchmark correspondante est
archivée dans un fichier texte. Ainsi, n'importe quel état est reproductible (`git checkout <tag>`)
et toute comparaison est rejouable (`benchstat <avant>.txt <après>.txt`).

| Tag Git | État du projet | Sortie benchmark | Séance |
|---|---|---|---|
| `baseline-naive` | Version naïve de référence | `baseline.txt` | 1 |
| `opt-cache-locality` | + disposition mémoire contiguë (fin des `[]*Order` dispersés) | `opt-cache.txt` | 2 |
| `opt-int64-ticks` | + prix en `int64` (ticks) | `opt-ticks.txt` | 4 |
| `opt-ticks-array` | + tableau de niveaux indexé par tick → best-price O(1) | `opt-ticks-array.txt` | 4 |
| `opt-zero-alloc` | + index de tête (recyclage des niveaux → zéro-alloc) | `opt-zero-alloc.txt` | 4 |
| `opt-concurrency` | + worker pool multi-symboles (parallélisme entre carnets) | speedup (§3.6) | 5 |
| `seance-6` | + recyclage `sync.Pool` (§3.7) + profil de contention mutex (§4.2) | RAM/`parallel` | 6 |
| `opt-int32` | + prix `int32` (`Order` 32→24 o, densité cache, §3.8) | `opt-int32.txt` | 6 |
| `final` | version optimisée finale | `final.txt` | — |

_(Convention de nommage : `opt-<levier>`. Compléter au fil des séances ; les lignes sont
indicatives et à ajuster selon les leviers réellement appliqués.)_

### Portée

Ce document couvre : la spécification du banc d'essai et le protocole de mesure (§1), le diagnostic
par profiling (§2), le journal des optimisations chiffrées (§3), les tentatives infructueuses
assumées (§4), la synthèse comparative reproductible (§5) et la gouvernance technique (§6).

---

## 1. Environnement & Métrologie (Baseline) — /3

### 1.1 Banc d'essai matériel

| Élément | Valeur |
|---|---|
| CPU | AMD Ryzen 7 7735U (16 threads) |
| Hiérarchie de cache (L1/L2/L3) | L1d 32 Ko + L1i 32 Ko (par cœur) · L2 512 Ko (par cœur) · L3 16 Mo (partagé) · ligne 64 o |
| RAM | 16 Go LPDDR5 (4 × 4 Go) @ 6400 MT/s — ≈ 14 Go utilisables (~2 Go réservés à l'iGPU Radeon) |
| OS | Arch Linux — noyau `7.1.8-arch1-3` |
| Runtime | Go `go1.26.5-X:nodwarf5 linux/amd64` |
| Alimentation / gouverneur | **batterie** · gouverneur `powersave` |

### 1.2 Protocole de mesure

Points communs aux deux mesures :
- **Charge :** `Generate(n, seed)` — flux d'ordres déterministe (même graine → même flux).
- **Plage de prix :** le flux est **borné** à 99,00–101,00 € (fenêtre autour d'un prix de référence de 100,00 €). C'est une condition de mesure qui rend possible le levier §3.4 (tableau indexé par tick) ; sa contrepartie face à la fluctuation des prix est discutée au §3.4.
- **Isolation :** génération faite **hors chronomètre** ; on ne mesure que la boucle de matching.
- **Paramètres :** n = 200 000 ordres, seed = 42.

La mesure a été menée en **deux niveaux de rigueur croissants** :

| Niveau | Protocole | Warmup | Itérations | Statistiques |
|---|---|---|---|---|
| 1 — Chronométrage simple | une exécution chronométrée | non | 1 run | temps brut, débit |
| 2 — Baseline consolidée | protocole rigoureux de référence | interne (`b.N` + `b.ResetTimer`) | 10 runs (`-count=10`) | médiane + écart-type |

### 1.3 Résultats

#### 1.3.1 Chronométrage simple (première mesure — Séance 1)

Première prise de mesure, volontairement minimale (objectif de la Séance 1 : obtenir un ordre de
grandeur de référence).

| Métrique | Valeur |
|---|---|
| Temps total (1 run) | `[~105 ms]` |
| Débit | `[~1,9 M ordres/s]` |
| Coût unitaire | `[~520 ns/ordre]` |
| Trades générés | `172 342` (invariant, seed fixe) |

#### 1.3.2 Baseline consolidée (référence pour toutes les comparaisons avant/après)

Protocole rigoureux : **10 runs mesurés** (`go test -bench -count=10`), le warmup étant assuré par la
montée en charge interne de `b.N` et par `b.ResetTimer` (qui exclut la génération du flux de la mesure).
**C'est cette médiane qui sert de point de comparaison** dans tout le reste du rapport (tag
`baseline-naive`, fichier `baseline.txt`).

| Métrique | Valeur |
|---|---|
| Temps total (médiane) | **77,75 ms** |
| Moyenne | 77,86 ms |
| Écart-type | 0,96 ms (**1,2 %** → banc stable) |
| Min / Max | 76,66 / 79,80 ms |
| Débit (médiane) | **2,57 M ordres/s** |
| Coût unitaire | **389 ns/ordre** |
| Allocations | 281 415 allocs/op |
| Mémoire | 38,91 MiB/op |
| Trades produits | 172 342 (invariant, seed fixe) |

> _Le faible écart-type (1,2 %) valide le banc : les mesures sont reproductibles, les gains ultérieurs
> seront donc attribuables aux optimisations, pas au bruit. La médiane (77,75 ms) est préférée à la
> moyenne car robuste aux outliers (ex. un run ralenti par une interruption système)._

---

## 2. Diagnostic matériel & Profiling réel — /5

### 2.1 Méthode

Deux profils complémentaires, capturés sur le benchmark du moteur (`BenchmarkMatching`, n = 200 000) :

```bash
# Profil CPU (échantillonnage 100 Hz) — où partent les cycles
go test -bench=Matching -cpuprofile cpu.prof -benchtime=3s ./internal/engine_test
go tool pprof -top -cum cpu.prof          # top textuel
go tool pprof -http=:8080 cpu.prof        # flamegraph interactif

# Profil mémoire (voir §3.2) — où sont les allocations
go test -bench=Matching -memprofile mem.prof ./internal/engine_test
go tool pprof -top -sample_index=alloc_objects mem.prof
```

### 2.2 Profil CPU (top cumulé)

Banc : AMD Ryzen 7 7735U. `Total samples = 9,13 s` sur 8,7 s de run.

| Fonction | flat | cum | Rôle |
|---|---|---|---|
| `matchBuy` | 3,9 % | **50,5 %** | matching acheteur |
| `matchSell` | 2,5 % | **41,8 %** | matching vendeur |
| `maps.(*Iter).Next` | **32,2 %** | 44,7 % | **parcours de la map** |
| `bestAsk` (inline) | 5,5 % | **32,2 %** | recherche meilleur prix |
| `bestBid` (inline) | 3,8 % | **28,8 %** | recherche meilleur prix |
| `runtime.growslice` + `memmove` | — | ~15 % | croissance des slices (append) |
| `f64hash` + `aeshashbody` | — | ~9 % | hachage des clés `float64` |
| `runtime.gcBgMarkWorker` | — | ~3,5 % | pression GC |

**Figure 1 — Flamegraph CPU** (`go tool pprof -http=:8080 cpu.prof`, vue *Flame Graph*) :

![Flamegraph CPU du matching](flamegraph-cpu.png)

_Lecture : la largeur d'une barre = sa part de temps CPU. Sous `Submit`, les plateaux `matchBuy`
(**49,7 %**) et `matchSell` couvrent presque toute la largeur. Sous chacun, la pile
`bestAsk`/`bestBid` → `runtime.mapIterNext` → `maps.(*Iter).Next` forme un **large plateau** = le
parcours complet de la map à chaque ordre (**O(n) « Table Scan », goulot n°1**). Les plateaux
`growslice`/`mapassign`/`memmove` = les allocations d'`append` (cf. §3.2). Aucun autre plateau large
→ goulot **unique et structurel** (le design `map[float64][]Order`)._

### 2.3 Identification formelle du Hot Path

**Goulot n°1 — la recherche de prix en O(n).** `bestAsk` + `bestBid` cumulent **~61 % du temps CPU**,
et leur coût est presque entièrement `maps.(*Iter).Next` (32 % en *flat*, le plus gros poste unique) :
c'est le **parcours complet de la map à chaque ordre** — le « Table Scan » O(n) suspecté depuis la
Séance 2, désormais **prouvé par la mesure**.

**Goulot n°2 — le prix en `float64`.** Le hachage des clés flottantes (`f64hash` + `aeshashbody`
≈ 9 %) confirme le choix naïf identifié dès l'origine : une clé `int64` (ticks) serait hachée plus vite.

**Goulot n°3 — allocations & GC.** `growslice`/`memmove` (~15 %) = la croissance des slices de
niveaux (cf. §3.2), et `gcBgMarkWorker` (~3,5 %) = la pression GC qui en découle.

**Conclusion.** Le design à base de `map[float64][]Order` est la racine **commune** des trois coûts
(scan O(n), hachage `float64`, allocations d'`append`). Le refactor `map → tableau de niveaux
pré-alloué indexé par tick int64` les adresse **tous les trois d'un seul geste**. Le profiling
justifie donc formellement ce refactor comme prochain levier prioritaire.

---

## 3. Journal d'optimisation & Démarche d'ingénierie — /5

> _Les leviers sont présentés dans l'**ordre chronologique** d'application. La localité (§3.1) a été
> traitée en Séance 2, **avant** le profiling formel (§2, Séance 4) : c'est précisément son gain **nul
> sur le temps** qui a **déclenché** ce profiling, lequel a ensuite désigné le vrai goulot (le O(n),
> §3.4). L'ordre de lecture suit donc le fil de la découverte._

### 3.0 Tableau des leviers identifiés (choix naïfs de la baseline)

| Choix naïf | Problème mécanique | Levier | Statut |
|---|---|---|---|
| `bestAsk/bestBid` par parcours complet de la map | O(n) par ordre (Table Scan) | Tableau de niveaux indexé par tick → O(1) | ✅ **fait & mesuré (§3.4) : −77 %** (goulot n°1 supprimé) |
| `[]*Order` (pointeurs dispersés) | Cache misses + pression GC | Structs contiguës | ✅ fait & mesuré (§3.1) |
| Prix `float64` | Arrondi + hashing lent | `int64` (ticks) | ✅ fait (§3.3) : `f64hash` éliminé du profil |
| `level[1:]` détruit la capacité → réallocations | `growslice`/`memmove`, pression GC | Index de tête (recyclage du tableau) | ✅ **fait & mesuré (§3.5) : −99,4 % d'allocs, −31 % temps** |
| Padding de `Order` | < ordres par ligne de cache 64 B | Réordonner les champs | ✅ vérifié (`make align`) : structs déjà alignées (32 o), non applicable |
| Flux tout en RAM | Empreinte O(n) | Streaming binaire | ⬜ à faire |
| Mono-thread | 1 cœur exploité (15/16 inactifs) | Worker pool borné, parallélisme **entre carnets** | ✅ **fait & mesuré (§3.6) : ×6,35 à 8 cœurs** |
| Allocation d'un carnet par symbole (runner parallèle) | Pression GC en multi-carnets | Recyclage via `sync.Pool` | ✅ **fait & mesuré (§3.7) : −36 % allocs, −27 % temps** |

### 3.1 Levier : localité de cache (`[]*Order` → `[]Order`)

**Objectif :** stocker les ordres d'un niveau dans un tableau contigu (`[]Order`) au lieu de pointeurs
dispersés (`[]*Order`).

**Hypothèse d'impact matériel :** _des pointeurs dispersés provoquent des cache misses (~200 cycles
chacun) ; un tableau contigu exploite les lignes de cache de 64 o et le prefetcher._

**Commande de vérification :** `go test -bench='Contiguous|Dispersed' -benchmem` (expérience isolée),
puis `benchstat` sur le moteur (10 runs).

**Endroits modifiés :** `book.go` — les niveaux passent de `[]*Order` à `[]Order`.

**Résultat.** Expérience isolée (même calcul, 0 alloc dans la boucle) :

| Disposition | ns/op | Rapport |
|---|---|---|
| Contigu `[]Order` | 422 502 | référence |
| Dispersé `[]*Order` | 11 185 513 | **× 26,5** |

Sur le moteur (`BenchmarkMatching`, n = 200 000, benchstat n=10) :

| Métrique | Avant (naïf) | Après (contigu) | Gain |
|---|---|---|---|
| Temps | 77,8 ms ± 2 % | 72,8 ms ± 3 % | −6,4 % |
| Mémoire (`B/op`) | 38,9 MiB | 37,8 MiB | −3,0 % |
| Allocations (`allocs/op`) | 281 416 | 81 421 | **−71,1 %** |

Écarts significatifs (p = 0,000). Le × 26,5 isolé est un **majorant** (pointer-chasing pur) : le moteur
fait aussi autre chose, d'où un gain réel plus faible.

**Lecture critique :** le levier réduit fortement les allocations (−71 %) mais le temps ne bouge presque
pas (−6 %). Ce n'est pas un échec : ça prouve que, sur ce carnet, les allocations ne sont **pas** le
goulot temps. Le vrai goulot est ailleurs — le parcours O(n) de `bestAsk`/`bestBid` (confirmé au §2.3).
C'est la loi d'Amdahl : optimiser une fraction qui ne domine pas le temps ne peut donner qu'un gain
marginal *sur le temps*. L'acquis reste utile (moins de GC = latence plus stable).

### 3.2 Analyse : origine des allocations (escape analysis)

_Cette section n'est pas un levier à gain immédiat mais un **diagnostic** : comprendre d'où viennent les
allocations avant de chercher à les supprimer (le levier lui-même est traité au §3.5)._

**Objectif :** localiser les allocations du hot path.

**Hypothèse d'impact matériel :** _si des variables du matching s'échappent sur le tas sans raison
(pointeur local, boxing `interface{}`, slice de taille variable), ce sont des allocations évitables._

**Commande de vérification :** `go build -gcflags="-m"` (échappement compile-time) + `go test
-memprofile` puis `pprof -sample_index=alloc_objects`.

**Résultat.** Aucun échappement *accidentel* dans le matching. Les allocations viennent des
`append(b.bids/asks[prix], *o)` : la slice d'un niveau est stockée dans une `map` (sur le tas), son
tableau doit donc vivre sur le tas — échappement **légitime**. Le memprofile confirme : **97,8 %** des
allocations dans `matchBuy`/`matchSell`, avec `runtime.growslice` en tête. Padding vérifié au passage
(`make align` → structs déjà alignées, 32 o, rien à réordonner).

**Lecture critique :** les allocations sont **structurelles** (adossées à la `map`), pas un défaut
local. Les supprimer impose donc de changer la structure — le refactor `map → tableau` (§3.4) puis
l'index de tête (§3.5). Le diagnostic est cohérent des deux côtés (compile-time et runtime).

### 3.3 Levier : Prix en `int64` (ticks)

**Objectif :** remplacer le prix `float64` par un `int64` en **ticks** (le centime : `100,05 € →
10005`).

**Hypothèse d'impact matériel :** _le profiling (§2) montre ~9 % du CPU dans le hachage des clés
`float64` (`f64hash` + `aeshashbody`). Une clé `int64` se hache en moins de cycles → ce coût doit
disparaître. Bonus métier : plus d'erreur d'arrondi. C'est aussi le **prérequis** de l'indexation par
tableau (§3.4)._

**Commande de vérification :** `go tool pprof -top cpu.prof | grep -i hash`.

**Endroits modifiés :** `order.go` (`Price` → `int64`), `book.go` (`map[int64][]Order`),
`generator.go`, `book_test.go`.

**Résultat :** tests verts (mêmes trades).

| Métrique | Avant (`float64`) | Après (`int64`) | Gain |
|---|---|---|---|
| Temps | 72,8 ms ± 3 % | 68,6 ms ± 2 % | **−5,8 %** |
| Hachage des clés (CPU) | ~9 % | `memhash64` ~2,5 % | ÷ ~4 |
| Allocations / Mémoire | 81,42 k / 37,75 MiB | idem | inchangé |

**Lecture critique :** gain purement **CPU** (le hachage), sans toucher au stockage. Le O(n) de
`bestAsk`/`bestBid` reste le goulot dominant → §3.4.

---

### 3.4 Levier : `map` → tableau de ticks (best-price O(1))

**Objectif :** remplacer `map[int64][]Order` par un tableau indexé par tick (`[][]Order`, index =
`tick − minTick`), avec deux curseurs `bestBidIdx` / `bestAskIdx`. Le meilleur prix devient une lecture
directe du curseur — **O(1)** au lieu du parcours complet de la map — **O(n)** — à chaque ordre.

**Hypothèse d'impact matériel :** _le profiling (§2.3) prouve que `bestAsk` + `bestBid` = **~61 % du
CPU** (parcours de la map à chaque ordre). Un curseur maintenu sur le meilleur niveau supprime cette
recherche (O(n) → O(1)). Prérequis : clé `int64` (§3.3), car un tableau ne s'indexe que par un entier
borné._

**Commande de vérification :** `make save NAME=opt-ticks-array` puis
`make compare A=opt-ticks B=opt-ticks-array`.

**Endroits modifiés :**

| Fichier | Modification |
|---|---|
| `book.go` | `map` → `bids`, `asks [][]Order` ; bornes `minTick`/`maxTick` **paramétrées** dans `NewBook` (pas de nombre magique) ; curseurs `bestBidIdx`/`bestAskIdx` ; méthodes `bestAsk`/`bestBid` **supprimées** ; avance amortie du curseur |
| `book_test.go`, `matching_bench_test.go`, `cmd/bench/main.go` | appel `NewBook(9900, 10100)` ; accès test par index |
| `order.go`, `generator.go` | inchangés |

**Résultat :** tests verts (mêmes trades).

| Métrique | Avant (`map`, O(n)) | Après (tableau, O(1)) | Gain | Significatif ? |
|---|---|---|---|---|
| Temps | 68,59 ms ± 2 % | **15,48 ms ± 2 %** | **−77,44 %** | ✅ p=0,000 (n=10) |
| Allocations | 81,42 k | 78,09 k | −4,09 % | ✅ p=0,000 (n=10) |
| Mémoire | 37,75 MiB | 37,62 MiB | −0,35 % | ✅ p=0,000 (n=10) — négligeable |
| `bestAsk`/`bestBid` (CPU) | ~61 % | ~0 % | goulot supprimé | profil (non benchstat) |

**Lecture critique :** gain **algorithmique** (O(n) → O(1)) : il attaque le CPU, pas la mémoire (les
allocations ne bougent presque pas). C'est le plus gros levier de l'audit (−77 %), et il n'avait de
sens qu'**après** la preuve du profiling. Deux remarques :

- _Contrepartie :_ le tableau suppose une **plage de prix bornée** (`minTick..maxTick`), classique en
  HFT (les prix restent dans une bande étroite) ; en production on ajouterait un repli hors plage.
- _Re-profil (démarche itérative) :_ une fois le O(n) parti, le nouveau goulot devient
  `runtime.growslice`/`memmove` (~30 % du CPU) — les réallocations de slices. C'est ce que le §3.5
  attaque. Le profil s'est **aplati** : plus aucune fonction ne domine.

**Fluctuation des prix & bande bornée.** Le tableau n'est rapide que parce qu'il est **borné**
(`idx = prix − minTick`, une case par prix) : il suppose que les prix restent dans la fenêtre
`minTick..maxTick` (ici 99,00–101,00 €). Or un marché **fluctue** — sur la durée, le prix de référence
dérive, et un prix hors bande tomberait **hors du tableau**. C'est le compromis classique
**vitesse ↔ généralité** : la `map` naïve acceptait n'importe quel prix mais était lente (O(n),
hachage) ; le tableau borné est en O(1) mais fige la plage. Trois façons de gérer la fluctuation sans
perdre le O(1) :

| Solution | Principe | Compromis |
|---|---|---|
| **Fenêtre glissante (re-centrage)** | la bande *suit* le prix de référence ; quand le mid dérive, on décale la fenêtre (principe du *ring buffer*) | garde l'O(1) ; petit coût ajouté au Hot Path (test de bande + décalage), **à mesurer** — solution HFT de référence |
| **Hybride tableau + map de débordement** | tableau pour la bande chaude (≈ 99 % des ordres), map lente pour les rares prix hors bande | O(1) courant, dégradé O(n) hors bande ; simple et robuste |
| **Élargir la bande** | couvrir un très grand intervalle d'emblée | **écarté** : sur-provisionnement mémoire, cf. le piège du §4 |

Dans ce TP, la bande fixe est une **hypothèse assumée** (le générateur reste dedans, §1.2). En
production, on retiendrait le **re-centrage** : il ne casse pas l'O(1) mais ajoute un coût au Hot Path
qu'il faudrait chiffrer — l'arbitrage « robustesse à la fluctuation ↔ vitesse » n'est donc pas gratuit.

---

### 3.5 Levier : zéro-allocation par index de tête

**Objectif :** supprimer les réallocations de slices pointées par le re-profil du §3.4
(`growslice`/`memmove` ≈ 30 % du CPU). C'est la consigne « pré-allouer & recycler » de la séance 4
(§9–11) et de la `constitution.md`.

**Hypothèse d'impact matériel :** _`level[1:]` avance la tête FIFO mais détruit la capacité du slice ;
au re-remplissage d'un niveau vidé, `append` réalloue. En gardant un index de tête `head` par niveau et
en réinitialisant le niveau (`level[:0]`, `head=0`) une fois vidé, le tableau est **réutilisé** — plus
de réallocation dans le cycle vidage/remplissage._

**Commande de vérification :** `make save NAME=opt-zero-alloc` puis
`make compare A=opt-ticks-array B=opt-zero-alloc` ; invariant : `172 342` trades.

**Endroits modifiés :**

| Fichier | Modification |
|---|---|
| `book.go` | 2 champs `bidsHead`, `asksHead []int` ; front = `level[h]` ; dépilage = `head++`, et reset `level[:0]`+`head=0` quand le niveau se vide (au lieu de `level[1:]`) |
| autres | inchangés |

**Résultat :** tests verts, invariant préservé (`172 342` trades).

| Métrique | Avant (O(1)) | Après (index de tête) | Gain | Significatif ? |
|---|---|---|---|---|
| Temps | 15,48 ms ± 2 % | **10,70 ms ± 1 %** | **−30,89 %** | ✅ p=0,000 (n=10) |
| Allocations | 78 088 | **457** | **−99,41 %** | ✅ p=0,000 (n=10) |
| Mémoire | 37,62 MiB | 34,37 MiB | −8,65 % | ✅ p=0,000 (n=10) |

**Lecture critique :** ici, contrairement au §3.1, réduire les allocations **fait aussi baisser le
temps** (−31 %) — parce que le §3.4 avait d'abord supprimé le O(n), rendant le `growslice`/`memmove`
dominant. L'ordre des leviers compte (Amdahl). Enfin, « zéro-alloc » ne veut pas dire 0 littéral : les
457 restantes sont **structurelles** (croissance du slice `trades`, dimensionnement initial des niveaux)
et **justifiées** au sens de la constitution. Les forcer à 0 serait contre-productif — voir le §4.

---

### 3.6 Levier : parallélisation (worker pool multi-symboles)

> _Séance J3_AM. Hypothèse et principe de conception posés **avant** de coder (démarche de la
> `constitution.md`), puis mesure du speedup._

**Objectif :** exploiter les cœurs du CPU en traitant **plusieurs carnets (symboles) en parallèle**,
via un worker pool borné à `runtime.NumCPU()` alimenté par un channel.

**Hypothèse d'impact matériel :** _le moteur est mono-thread → un seul cœur travaille (15/16 inactifs
chez nous). En répartissant N carnets **indépendants** sur un pool de goroutines dimensionné au nombre
de cœurs, le débit total doit croître ~linéairement avec les cœurs, jusqu'à saturation (cœurs
physiques, mémoire, ordonnanceur)._

**Principe de conception — parallélisme ENTRE carnets, pas DANS un carnet.** Le matching d'un carnet
est **intrinsèquement séquentiel** : la priorité prix-temps impose de traiter les ordres dans l'ordre,
donc deux goroutines ne peuvent pas matcher le même carnet sans un verrou partagé (`Mutex`) autour de
`Submit`. Elles se disputeraient alors ce verrou → **contention** → le débit s'effondre au lieu de
monter (Loi d'Amdahl / USL, cf. l'échec documenté au §4). La seule parallélisation correcte est donc
**entre carnets indépendants** : un carnet par **symbole**, aucune donnée partagée, donc **aucun
verrou**. Chaque worker traite **un carnet en entier, séquentiellement** ; le parallélisme vient du
nombre de carnets traités **simultanément**. Le moteur mesuré (`Book`, `Submit`, `match*`) reste
**inchangé** — on ajoute seulement une couche d'orchestration.

**Commande de vérification :** `GOMAXPROCS=k go run ./cmd/parallel` pour `k = 1, 2, 4, 8, 16` ; speedup
= `temps(1) / temps(k)`.

**Endroits modifiés :** nouveau `cmd/parallel/` — worker pool (`runtime.GOMAXPROCS(0)` goroutines) qui
piochent des symboles dans un channel ; agrégation des trades par `atomic.Int64` (pas de `Mutex`).
`internal/engine/` **intact**. Charge : 64 carnets indépendants × 200 000 ordres (seed `42+symbole`).

**Résultat :** invariant préservé (11 002 119 trades, identique à tout `k`).

| Cœurs (`GOMAXPROCS`) | Temps | Speedup | Efficacité | RAM de pointe (Sys) |
|---|---|---|---|---|
| 1 | 1 613 ms | ×1,00 | 100 % | ~50 MiB |
| 2 | 488 ms | ×3,31 | 166 % | ~86 MiB |
| 4 | 326 ms | ×4,94 | 124 % | ~179 MiB |
| 8 | 254 ms | **×6,35** | 79 % | ~340 MiB |
| 16 | 262 ms | ×6,14 | 38 % | **~658 MiB** |

_RAM = `runtime.MemStats.Sys` (empreinte mémoire). Elle est **pilotée par le nombre de workers** (N
carnets vivants en parallèle), donc ~stable d'une machine à l'autre — à confirmer via
`GOMAXPROCS=k go run ./cmd/parallel`._

**Lecture avant / après (le compromis en un coup d'œil).** La courbe ci-dessus se lit comme un
avant/après entre le point séquentiel (1 cœur) et le point optimal (8 cœurs) :

| | Temps | RAM de pointe (Sys) |
|---|---|---|
| **Avant** — séquentiel (`GOMAXPROCS=1`) | 1 613 ms | ~50 MiB |
| **Après** — parallèle (`GOMAXPROCS=8`) | **254 ms** | ~340 MiB |
| **Effet** | **×6,35 débit** | **~×6,8 RAM** |

On **échange ~×6,8 de RAM de pointe contre ×6,35 de débit** : c'est le trade-off CPU↔RAM du levier,
chiffré. _(Le « avant » exécute le même worker pool avec un seul worker — séquentiel de fait, l'overhead
du channel étant négligeable.)_

**Lecture critique :**

- **Le levier scale jusqu'aux cœurs physiques.** ×6,35 à 8 cœurs, sans contention (carnets
  indépendants → aucune donnée partagée → aucun verrou). C'est le seul découpage correct pour un order
  book (matching séquentiel par carnet, parallélisme *entre* carnets).
- **Plateau puis légère régression à 16.** Le 7735U a **8 cœurs physiques + SMT** (16 threads
  logiques). Au-delà de 8, les threads supplémentaires partagent les ressources d'un cœur physique →
  pour du calcul CPU/mémoire, ils n'ajoutent rien (16 cœurs = 262 ms, _pire_ que 8 = 254 ms). C'est la
  nuance « **cœurs physiques** » de la `constitution.md` et l'amorce de l'écroulement **USL** (§7 J3) :
  au-delà d'un seuil, ajouter des cœurs ne multiplie plus le débit. **Point optimal : 8 workers.**
- **Trade-off CPU ↔ RAM.** Le débit se paie en **mémoire de pointe** : chaque worker traite un carnet
  vivant → la RAM croît ~linéairement avec le nombre de workers (~50 MiB à 1, ~340 MiB à 8, ~658 MiB à
  16). Passer de 8 à 16 workers coûte **~2× de RAM pour un temps _pire_** → double raison de s'arrêter à
  8. C'est le compromis que le levier expose : on échange de la mémoire contre du débit.
- **Caveat de mesure (DVFS).** Le ×3,31 à 2 cœurs est **super-linéaire** — impossible en pur calcul :
  c'est un artefact de `powersave` + batterie (§1.1), qui laisse le CPU à basse fréquence en mono-thread
  et le fait booster sous charge, ce qui rend la baseline 1 cœur trop lente. La **forme** de la courbe
  (scaling jusqu'aux cœurs physiques, plateau ensuite) reste valide ; une courbe aux chiffres propres se
  mesurerait sur `performance` + secteur.
- **Dimensionnement des workers (choix raisonné, pas par défaut).** Ici `workers = GOMAXPROCS` (= nombre
  de cœurs) parce que le matching est **CPU-bound** : un worker qui calcule occupe un cœur à 100 %, en
  mettre davantage ne ferait que du context-switching stérile. Règle générale : charge **CPU-bound** →
  workers ≈ cœurs ; charge **I/O-bound** (attente réseau/disque, ex. le J4 réseau) → on peut monter à
  **cœurs × 2 à 10**, car les workers passent l'essentiel de leur temps à attendre et libèrent le CPU
  pour d'autres. Notre cas étant purement CPU, `GOMAXPROCS` est le bon réglage.

L'échec « un seul carnet parallélisé » (§4) fournit le contre-exemple qui **justifie** ce choix : là,
la contention sur le `Mutex` fait _chuter_ le débit.

---

### 3.7 Levier : recyclage des carnets (`sync.Pool`)

> _Séance J3_PM. S'applique au **runner parallèle** (§3.6), pas au moteur mono-carnet._

**Objectif :** dans le runner multi-symboles, **recycler** les carnets entre symboles via un
`sync.Pool` au lieu d'en allouer un neuf à chaque fois → réduire la pression d'allocation et le GC.

**Hypothèse d'impact matériel :** _chaque symbole allouait un `NewBook` (4 tableaux de `nTicks`) jeté
aussitôt → forte pression GC en multi-carnets. En empruntant un carnet au pool et en le `Reset()`
(réutilisation des tableaux), on supprime ces allocations répétées → moins de travail pour le GC, donc
plus de temps._

**Commande de vérification :** `GOMAXPROCS=8 go run ./cmd/parallel` (sans) vs `... -pool` (avec).

**Endroits modifiés :** `internal/engine/reset.go` (nouvelle méthode `Book.Reset()`, **hors Hot Path**) ;
`cmd/parallel` (`sync.Pool[*Book]` : Get → Reset → Submit → Put). Le moteur de matching est inchangé.

**Résultat :** invariant préservé (11 002 119 trades). GOMAXPROCS = 8 :

| Métrique | Sans pool | Avec pool | Effet |
|---|---|---|---|
| Temps | 222 ms | **163 ms** | **−27 %** |
| Alloc cumulé | 2 590 MiB | **1 664 MiB** | **−36 %** |
| Passages GC | 35 | 19 | **−46 %** |
| RAM de pointe (`Sys`) | 249 MiB | 281 MiB | +13 % |

**Lecture critique :** `sync.Pool` gagne là où on l'attend — **moins d'allocations (−36 %)** → **moins
de GC (−46 %)** → **moins de temps (−27 %)**. Mais ce n'est **pas** un gain sur la RAM de pointe : elle
monte même un peu (+13 %). C'est le **trade-off honnête** du pool : il **garde des objets vivants** pour
les réutiliser et collecte moins souvent, donc il échange un **pic mémoire plus haut** contre **moins de
churn d'allocation, moins de GC et plus de vitesse**. Le bon usage : quand la **pression GC** est le
problème (notre cas en multi-carnets), pas quand la RAM de pointe est la contrainte.

---

### 3.8 Levier : compacité du prix (`int64` → `int32`)

> _Retour au moteur mono-carnet. En revisitant la structure `Order` après la séance scalabilité, un
> dernier levier de compacité est apparu : le prix était stocké en `int64` alors qu'un `int32` suffit
> très largement._

**Objectif :** réduire la taille de `Order` en stockant le prix (un tick) sur `int32` au lieu d'`int64`,
pour améliorer la densité en cache du carnet au repos.

**Hypothèse d'impact matériel :** _un tick tient très largement dans un `int32` (±2,1 Md de ticks ; notre
bande n'en fait que ~200). Passer `Price` de `int64` à `int32` fait tomber `Order` de **32 à 24 octets**
(l'alignement se cale sur les deux `uint64` restants). Une ligne de cache de 64 o loge alors **2,67 ordres
au lieu de 2** → moins de défauts de cache en parcourant les files FIFO au repos → matching plus rapide._

**Commande de vérification :**

```bash
make save NAME=opt-int32
make compare A=opt-zero-alloc B=opt-int32     # temps + B/op + allocs, avec p-value
```

_(tailles réelles contrôlées via `unsafe.Sizeof` : `Order` 24 o, `Trade` 32 o.)_

**Endroits modifiés :** `Price` passe en `int32` dans `Order` et `Trade` (`order.go`), dans `Level` et
`Snapshot.BestBid`/`BestAsk` (`snapshot.go`) ; `minTick` du `Book` + les conversions `int32(idx)+minTick`
(`book.go`) ; le générateur (`feed`) et l'affichage `eur()` (`cmd/snapshot`). **Seul le champ `Price`
change de largeur** — `ID`, `Quantity` et `seed` gardent la leur (piège classique du chercher-remplacer
trop large).

**Résultat (benchstat n=10, Ryzen 7 7735U — CPU + RAM) :**

| Métrique | opt-zero-alloc (int64) | opt-int32 | Δ | Significatif ? |
|---|---|---|---|---|
| Temps (`sec/op`) | 10,70 ms | **9,67 ms** | **−9,63 %** | ✅ p=0,000 (n=10) |
| Mémoire (`B/op`) | 34,37 MiB | 33,80 MiB | −1,65 % | ✅ p=0,000 (n=10) |
| Allocations (`allocs/op`) | 457 | 455 | −0,44 % | ✅ p=0,000 (n=10) |
| `sizeof(Order)` | 32 o | **24 o** | **−25 %** | structurel |
| `sizeof(Trade)` | 32 o | 32 o | — | structurel |

**Lecture critique :**

- **Le gain est sur le TEMPS, pas sur les octets.** Le B/op ne baisse que de 1,65 % — parce qu'il est
  **dominé par le slice de trades** (172 342 trades + doublements d'`append`), et `Trade` **reste à 32 o**
  (trois `uint64` l'ancrent ; le prix `int32` ne fait qu'y déplacer le padding). Le −25 % ne s'applique
  qu'au **stockage des ordres au repos** (`[]Order` par niveau). Mais la densité cache qui en découle
  donne **−9,63 % de temps**, net et significatif (p=0,000) : hypothèse matérielle confirmée.
- **`int16` aurait été une fausse économie.** Les deux `uint64` (ID, Quantity) imposent un alignement de
  8 → `int16` donnerait **exactement la même** taille (24 o) qu'`int32`, **zéro octet** gagné, mais avec
  un plafond de ±32 767 ticks (overflow dès une action à quelques centaines d'euros). `int32` est le bon
  compromis : sûr, compact, natif au registre. La disposition mémoire réelle (le padding se déplace, la
  taille non) :

  | Type de `Price` | Disposition des champs (offsets en octets) | `sizeof(Order)` |
  |---|---|---|
  | `int64` (avant) | ID `0‑7` · Side `8` · Type `9` · _pad `10‑15`_ · Price `16‑23` · Qty `24‑31` | **32 o** |
  | `int32` (retenu) | ID `0‑7` · Side `8` · Type `9` · _pad `10‑11`_ · Price `12‑15` · Qty `16‑23` | **24 o** (−25 %) |
  | `int16` (rejeté) | ID `0‑7` · Side `8` · Type `9` · Price `10‑11` · _pad `12‑15`_ · Qty `16‑23` | **24 o** (aucun gain) |

  En `int16`, les 2 octets « économisés » sur le prix sont **repris par le padding** avant `Quantity`
  (qui doit s'aligner sur 8) → même taille finale qu'`int32`, mais avec le risque d'overflow en plus.
- **À l'échelle, ça se propage.** En multi-symboles (`cmd/parallel`, 64 × 200 000 ordres), l'allocation
  cumulée recule de ~2 590 à ~2 458 MiB : le −25 % par ordre au repos, multiplié par des millions
  d'ordres, allège la pression mémoire globale _(mesure indicative — un seul run, non benchstaté)_.
- **Plancher.** Descendre `Order` sous 24 o exigerait de rétrécir aussi `ID`/`Quantity` (`uint64`), ce
  qui toucherait leur plage utile → **hors périmètre** de ce levier (« un levier = une variable »).

> _Reproductible : tag `opt-int32`, `bench-results/opt-int32.txt`, `make compare A=opt-zero-alloc B=opt-int32`._

---

## 4. Confrontation critique & « Échec constructif » — /3

> _Attendu : documenter au moins UNE tentative d'optimisation contre-productive ou infructueuse,
> avec explication mécanique ET chiffrée de la régression avant retour arrière._

### 4.1 Pré-allocation avide des niveaux (« over-provisioning »)

**Tentative.** Après le levier zéro-alloc
(§3.5), il restait 457 allocations. Tentation naturelle : les faire disparaître en **pré-allouant**
chaque niveau à une grosse capacité dans `NewBook` (`make([]Order, 0, 1024)` pour les 201 niveaux × 2
côtés), pour « ne plus jamais réallouer ».

**Hypothèse initiale :** _réserver la capacité d'avance élimine la croissance des niveaux → moins
d'allocations **et** gain de temps (plus de `growslice`/`memmove`)._

**Résultat mesuré (régression, benchstat n=10) :**

| Métrique | Zéro-alloc (§3.5) | Piège (pré-alloc 1024) | Effet | Significatif ? |
|---|---|---|---|---|
| Temps (`sec/op`) | 10,70 ms | 11,36 ms | **+6,16 %** | ✅ p=0,000 (n=10) |
| Mémoire (`B/op`) | 34,37 MiB | 45,33 MiB | **+31,89 %** | ✅ p=0,000 (n=10) |
| Allocations (`allocs/op`) | 457 | 463 | +1,31 % | ✅ p=0,000 (n=10) |

**L'hypothèse est fausse sur les trois axes** : plus lent, plus gourmand, et même *plus* d'allocations.

**Explication mécanique :** l'index de tête (§3.5) **recycle déjà** les tableaux de niveaux → il ne
restait **aucune réallocation à éviter**. La pré-allocation n'apporte donc aucun gain, mais ajoute deux
coûts : (1) Go **zère** chaque tableau créé → réserver 201 × 2 × 1024 × 32 o ≈ **13 Mo par carnet** se
paie en `memclr` à chaque `NewBook`, d'où le **+6 % de temps** ; (2) ces 13 Mo restent **réservés**
(souvent pour des niveaux vides ou peu profonds), d'où le **+32 % de mémoire**. Pire : à l'échelle
**multi-symboles** (un carnet par instrument), ce gaspillage serait multiplié par le nombre de symboles
→ intenable.

**Décision : retour arrière** (`git checkout internal/engine/book.go`). On conserve la version §3.5,
qui n'alloue que le **justifié** (§3.5, « plancher structurel »). Leçon : optimiser une métrique (les
allocations) **à l'aveugle** peut **régresser les autres** (temps *et* mémoire) sans rien gagner. Le bon
critère d'arrêt n'est pas « 0 allocation » mais « **plus aucune allocation injustifiée** » — atteint
dès le §3.5.

> _Reproductible : patch de pré-allocation dans `NewBook`, `make save NAME=opt-prealloc-trap`,
> `make compare A=opt-zero-alloc B=opt-prealloc-trap`, puis `git checkout` pour revenir. Résultats
> archivés : `bench-results/opt-prealloc-trap.txt`._

### 4.2 Paralléliser le matching d'un seul carnet (contention)

**Tentative.** Le §3.6 gagne ×6,35 en parallélisant **entre** carnets. Tentation : paralléliser aussi
**dans** un carnet — répartir les 200 000 ordres sur `G` goroutines qui écrivent dans **le même** carnet,
protégé par un `Mutex`.

**Hypothèse initiale :** _plus de goroutines sur un carnet → plus de débit._

**Résultat mesuré (meilleur de 5 runs, Ryzen 7 7735U — 8 cœurs / 16 threads) :**

| Version | Temps | vs séquentiel | Correction |
|---|---|---|---|
| Séquentiel (0 verrou) | 11,5 ms | ×1,00 | ✅ 172 342 trades |
| Contendu — 1 goroutine | 13,5 ms | ×1,17 | ✅ 172 342 |
| Contendu — 2 | 17,5 ms | ×1,51 | ⚠️ **trades faux** (172 326) |
| Contendu — 4 | 19,6 ms | ×1,70 | ⚠️ faux (172 345) |
| Contendu — 8 | 22,9 ms | **×1,98** | ⚠️ faux (172 310) |
| Contendu — 16 | 18,0 ms | ×1,56 | ⚠️ faux (172 195) |

_(Mémoire ≈ constante : un seul carnet dans tous les cas. Le trade-off ici est **temps + correction**,
pas la RAM.)_

**L'hypothèse est doublement fausse : plus lent ET faux.**

**Explication mécanique — deux échecs simultanés :**

1. **Contention (performance).** Le `Mutex` **sérialise** tout le matching → **zéro** parallélisme gagné.
   Pire, il ajoute un coût : dès 1 goroutine, +17 % (lock/unlock par ordre, sans concurrent) ; puis la
   dégradation s'aggrave avec les goroutines — mises en attente/réveil et surtout **rebond de la ligne de
   cache** (le carnet et le verrou font des allers-retours entre cœurs → invalidations MESI, §6 J3).
   La dégradation culmine à **×1,98 sur 8 goroutines** — soit exactement le nombre de **cœurs physiques** :
   c'est là que le maximum de cœurs se disputent réellement le verrou en même temps. Au-delà (16 threads
   SMT), le léger reflux à ×1,56 n'est pas un gain : les threads logiques se partagent les mêmes unités
   d'exécution, il y a donc moins de contenders *réellement simultanés* sur le verrou. Le sens est
   univoque : **ajouter des cœurs ne divise jamais le temps, il l'augmente.** C'est l'**écroulement USL**
   (§7 J3) — le débit régresse au lieu de scaler.

   La **preuve chiffrée** vient du profil de contention (`SetMutexProfileFraction(1)` +
   `go tool pprof -top mutex.prof`) : sur **1,65 s de temps cumulé bloqué** à attendre le verrou,
   **99,35 % est passé dans `sync.(*Mutex).Unlock`**. Autrement dit les goroutines ne matchent quasiment
   pas — elles font la queue. Le verrou *est* le programme.
2. **Incorrection.** Dès 2 goroutines, l'entrelacement des `Submit` détruit la **priorité prix-temps** →
   le carnet n'apparie plus dans l'ordre d'arrivée → **trades différents** (172 326 ≠ 172 342). Pire,
   le compte varie d'un run à l'autre (172 345, 172 310, 172 195…) : le résultat est *non déterministe*
   et *faux*, indépendamment de la vitesse — c'est même le problème le plus grave.

**Décision : abandon.** On ne parallélise **pas** un carnet. La bonne architecture est le §3.6 :
parallélisme **entre carnets indépendants** (aucun état partagé → aucun verrou → aucune contention →
scaling ×6,35). Le contraste valide le choix : **partager l'état mutable = contention + incorrection ;
l'isoler = scaling**. (Mantra Go : _« don't communicate by sharing memory; share memory by
communicating »_.)

> _Reproductible : `go run ./cmd/contention` (compare séquentiel vs contendu 1/2/4/8/16 goroutines,
> puis écrit `mutex.prof`) ; profil de contention : `go tool pprof -top mutex.prof`._

---

## 5. Reproductibilité & Synthèse comparative — /4

> _Attendu : automatisation complète en UNE commande + tableau de synthèse chiffrant les gains
> (baseline vs version finale, via benchstat/hyperfine)._

### 5.1 Automatisation

Toute la mesure est pilotée par un **`Makefile`** à la racine du projet. L'exigence « une seule
commande » de l'axe 5 est tenue : `make bench` reproduit intégralement la mesure du moteur.

| Commande | Rôle | Commande Go sous-jacente |
|---|---|---|
| `make test` | Tests de correction (filet de sécurité) | `go test ./...` |
| `make vet` | Analyse statique | `go vet ./...` |
| `make align` | Alignement mémoire des structs / padding (axe 3) | `go vet -vettool=…/fieldalignment ./...` |
| `make bench` | Benchmark du moteur (10 runs), affiché à l'écran | `go test -bench=Matching -benchmem -count=10 ./...` |
| `make save NAME=<tag>` | Archive le benchmark dans `bench-results/<tag>.txt` | `… \| tee bench-results/<tag>.txt` |
| `make compare A=<t> B=<t>` | `benchstat` entre deux résultats archivés | `benchstat bench-results/A.txt bench-results/B.txt` |
| `make bench-cache` | Expérience de localité (contigu vs dispersé, le ×26) | `go test -bench='Contiguous\|Dispersed' -benchmem ./...` |
| `make profile` | Génère un profil CPU `cpu.prof` (→ flamegraph, axe 2) | `go test -bench=Matching -cpuprofile cpu.prof ./...` |
| `make bench-hyperfine` | Mesure **processus** du binaire entier (niveau end-to-end) | `hyperfine --warmup 5 --runs 50 './ob'` |
| `make snapshot` | Génère la vue HTML du carnet (visualisation, hors périmètre perf) | `go run ./cmd/snapshot` |
| `make clean` | Supprime les fichiers éphémères (profils, binaires, snapshot) ; garde `bench-results/` | — |
| `make` | Enchaîne `test` puis `bench` | — |

Tous les résultats de benchmark sont **archivés dans `bench-results/`** (créé automatiquement),
au lieu d'être éparpillés à la racine — pièces à conviction versionnées.

Banc d'essai : AMD Ryzen 7 7735U, Go/linux/amd64.

**Deux niveaux de mesure complémentaires :**
- **benchstat** (`make bench`) isole la *fonction* de matching en mémoire — le plus précis pour
  chiffrer un levier.
- **hyperfine** (`make bench-hyperfine`) chronomètre le *binaire entier* `cmd/bench` (lancement →
  sortie), au niveau **processus**. C'est le temps perçu par l'utilisateur.

> _Limite assumée : hyperfine inclut le démarrage Go et la génération des ordres, donc son gain %
> est plus **dilué** que celui de benchstat (qui isole le matching). Les deux focales sont
> complémentaires, pas contradictoires._

#### Système de résultats (avant / après)

Les mesures ne sont pas seulement affichées : elles sont **archivées dans `bench-results/`**, ce qui
permet une comparaison chiffrée et rejouable. Le Makefile s'en charge (dossier créé tout seul) :

```bash
make save NAME=baseline        # fige l'état AVANT → bench-results/baseline.txt
# ... application d'un levier ...
make save NAME=opt-cache       # état APRÈS → bench-results/opt-cache.txt
make compare A=baseline B=opt-cache   # benchstat entre les deux, avec significativité
```

`benchstat` indique si l'écart est **réel** ou dans le bruit de mesure. Chaque fichier de résultat
est associé au **tag Git** de la version correspondante (voir Introduction → Démarche : `baseline-naive`
→ `baseline.txt`, `opt-cache-locality` → `opt-cache.txt`, …), ce qui rend chaque comparaison
traçable et reproductible.

> _Installer benchstat si besoin : `go install golang.org/x/perf/cmd/benchstat@latest`._

### 5.2 Synthèse des gains

Comparaison `benchstat` de la **baseline aux versions successives**, 10 runs, n = 200 000, seed 42.

| Version (tag) | Temps | Allocations | Mémoire | Gain temps vs baseline |
|---|---|---|---|---|
| Baseline naïve (`baseline-naive`) | 77,8 ms ± 2 % | 281,4 k | 38,9 MiB | — |
| + localité de cache (`opt-cache-locality`) | 72,8 ms ± 3 % | **81,4 k** | 37,8 MiB | **−6,4 %** |
| + prix `int64` ticks (`opt-int64-ticks`) | 68,6 ms ± 2 % | 81,4 k | 37,8 MiB | **≈ −12 %** |
| + tableau de ticks O(1) (`opt-ticks-array`) | 15,48 ms ± 2 % | 78,1 k | 37,6 MiB | −77 % (÷ 4,4) |
| + zéro-alloc / index de tête (`opt-zero-alloc`) | 10,70 ms ± 1 % | 457 | 34,4 MiB | −86 % (÷ 7,3) |
| + prix `int32` (§3.8, `opt-int32`) | **9,67 ms ± 2 %** | **455** | **33,8 MiB** | **−87,6 % (÷ 8,0)** |
| **Finale (état actuel)** | **9,67 ms** | **455** | 33,8 MiB | **÷ 8,0 vs baseline** |

_Lecture : les premiers leviers (§3.1 localité, §3.3 `int64`) sont des **micro-optimisations** : la localité gagne surtout sur les
**allocations** (−71 %) mais le temps ne baisse que de ~6 %, car le goulot temps est ailleurs — le
**O(n) de `bestAsk`/`bestBid`** (§2.3, ~61 % du CPU). Le levier `map → tableau de ticks` (§3.4)
supprime ce O(n) — **−77,44 %** (68,59 → 15,48 ms). Puis le §3.5 (index de tête) supprime les
réallocations redevenues visibles une fois le O(n) parti : **−99,4 % d'allocations** (78 k → 457) et,
cette fois, **−30,9 % de temps** (15,48 → 10,70 ms). Enfin, le §3.8 (prix `int32`) grignote **−9,6 %**
de plus (10,70 → 9,67 ms) par densité de cache. Cumul : **÷ 8,0 vs la baseline** (77,75 → 9,67 ms).
Les deux gros leviers (§3.4, §3.5) illustrent la Loi d'Amdahl : chaque fois qu'on supprime le segment
dominant, le suivant devient l'objectif — et une optimisation « invisible » sur le temps (allocs au
§3.1) peut le devenir plus tard (§3.5)._

**Deux axes orthogonaux.** Le tableau ci-dessus mesure le **moteur mono-carnet** : **÷8,0** en temps,
**÷618** en allocations. À cela s'ajoute un **second axe indépendant**, la **parallélisation** (§3.6) :
le débit total scale **×6,35** sur 8 cœurs en traitant plusieurs carnets à la fois. Les deux se
**cumulent** (un moteur mono-carnet optimisé, exécuté en parallèle) — mais chacun a son **trade-off** :
le zéro-alloc échange un peu de complexité contre du temps ET de la mémoire ; la parallélisation échange
de la **RAM de pointe** (N carnets vivants) contre du débit (§3.6).

#### Mesure processus (hyperfine)

Complément **niveau processus** du binaire `cmd/bench` (`hyperfine --warmup 5 --runs 50`, 50 runs).
_Ces mesures ont été prises sur la version **zéro-alloc** (avant le levier §3.8 `int32`) ; le −9,6 % du
matching y retrancherait ~1 ms, sans changer la lecture (le coût fixe startup + génération domine)._

| Binaire | Temps processus (mean ± σ) | Min / Max |
|---|---|---|
| Baseline naïve (`ob`) | 83,1 ms ± 7,8 ms | 72,5 / 117,8 ms |
| **Finale — zéro-alloc** (`ob_opti`) | **17,0 ms ± 1,0 ms** | 15,0 / 19,1 ms |
| **Rapport** | **÷ 4,9** | — |

**Décomposition (contrôle de cohérence).** Le coût **fixe** du binaire (démarrage du runtime Go +
génération des 200 000 ordres) est incompressible ; on le retrouve des deux côtés :

| Binaire | Process total | ≈ matching (benchstat) | ≈ fixe |
|---|---|---|---|
| `ob` (baseline) | 83,1 ms | 77,8 ms | **5,3 ms** |
| `ob_opti` (finale) | 17,0 ms | 10,7 ms | **6,3 ms** |

Le terme fixe (~5–6 ms) est cohérent des deux côtés : le seul écart entre les deux binaires est bien le
matching.

_Lecture (Loi d'Amdahl au niveau processus) : le ratio processus (**÷4,9**) est plus faible que le
**÷7,3** du matching pur (benchstat), parce que hyperfine mesure **tout le binaire**, dont ~5–6 ms de
coût fixe que l'optimisation ne touche pas. Ce coût fixe pèse désormais **~37 %** du binaire finale
(6,3 / 17,0 ms) contre ~6 % de la baseline : plus on optimise le matching, plus la part non optimisée
domine — la limite d'Amdahl s'est déplacée vers le startup + la génération. Pour aller plus loin, ce
serait désormais **là** qu'il faudrait chercher (ex. génération en streaming). Note : sur batterie +
`powersave` (§1.1), le binaire naïf a montré un run à froid à ~168 ms (DVFS), d'où un σ plus large — la
finale, elle, reste très stable (σ 1,0 ms)._

---

## 6. BONUS — Gouvernance technique (`constitution.md`) — +2

> _Attendu : fichier de contrainte à la racine, 4 directives respectées (posture système, gardes-fous
> négatifs, justification empirique hypothèse/commande, formatage impératif)._

- [x] `constitution.md` présent à la racine du dépôt
- [x] **Directive 1 — posture système** (ingénieur contraint par des métriques réelles) → appliquée dans tout le rapport : aucun levier sans mesure préalable (§2 avant §3), ordre *work → right → fast* respecté.
- [x] **Directive 2 — gardes-fous** (bannir `fmt.Sprintf` sur Hot Path, `float64`, goroutines non bornées) → `fmt.Sprintf` absent du matching (§3.2), prix passés en `int64` (§3.3), allocations chaudes justifiées ou supprimées (§3.5).
- [x] **Directive 3 — justification empirique** (couple hypothèse / commande de preuve) → chaque levier §3.1–3.5 est formulé ainsi (« Hypothèse d'impact matériel » + « Commande de vérification »), et le §4 documente une hypothèse **infirmée** par la mesure.
- [x] **Directive 4 — formatage impératif et vérifiable** → chaque affirmation de perf est chiffrée et rejouable (tags Git + `bench-results/*.txt` + `benchstat`).

_La constitution n'a pas seulement été écrite : elle a **gouverné la démarche** (mesure avant optimisation, justification chiffrée, retour arrière au §4). C'est sa mise en pratique, pas sa simple présence, qui vaut le bonus._

---

## Annexes

- Sorties brutes de benchmark : `bench-results/*.txt` (`baseline`, `opt-cache`, `opt-ticks`, `opt-ticks-array`, `opt-zero-alloc`, `opt-prealloc-trap`).
- Historique Git : tag `baseline-naive` → commits par levier.