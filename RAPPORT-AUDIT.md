# Rapport d'Audit de Performance — Order Book HFT

**Auteur :** Tanguy David
**Cours :** Optimisations & Performances Backend — Sup de Vinci, RNCP Bloc 4
**Projet :** Order Book HFT (moteur d'appariement, Go)
**Dépôt :** `[lien Git]` — baseline figée au tag `baseline-naive`

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
_Hot Path_ au sens du cours. Chaque micro-coût (une allocation, un défaut de cache, une recherche en
O(n)) y est amplifié par le volume, ce qui en fait un terrain d'observation idéal pour les leviers
d'optimisation matérielle et algorithmique.

### Démarche

Le projet suit la **Règle d'Or** du cours — _make it work → make it right → make it fast_ — et son
principe directeur : **on ne devine jamais un goulot d'étranglement, on le mesure**. Aucune
optimisation n'est appliquée sans (1) une hypothèse d'impact matériel et (2) une mesure avant/après
qui la valide ou l'infirme.

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
| `opt-concurrency` | + worker pool borné aux cœurs physiques | `[À REMPLIR]` | `[…]` |
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
- **Isolation :** génération faite **hors chronomètre** ; on ne mesure que la boucle de matching.
- **Paramètres :** n = 200 000 ordres, seed = 42.

La mesure a été menée en **deux niveaux de rigueur croissants** :

| Niveau | Protocole | Warmup | Itérations | Statistiques |
|---|---|---|---|---|
| 1 — Chronométrage simple (Séance 1) | une exécution chronométrée | non | 1 run | temps brut, débit |
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

> _Attendu : captures flamegraphs / profils pprof annotés + identification FORMELLE du Hot Path
> (instructions bloquantes, contention mémoire, pression GC)._

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

**Cause secondaire — le prix en `float64`.** Le hachage des clés flottantes (`f64hash` + `aeshashbody`
≈ 9 %) confirme le choix naïf identifié dès l'origine : une clé `int64` (ticks) serait hachée plus vite.

**Cause tertiaire — allocations & GC.** `growslice`/`memmove` (~15 %) = la croissance des slices de
niveaux (cf. §3.2), et `gcBgMarkWorker` (~3,5 %) = la pression GC qui en découle.

**Conclusion.** Le design à base de `map[float64][]Order` est la racine **commune** des trois coûts
(scan O(n), hachage `float64`, allocations d'`append`). Le refactor `map → tableau de niveaux
pré-alloué indexé par tick int64` les adresse **tous les trois d'un seul geste**. Le profiling
justifie donc formellement ce refactor comme prochain levier prioritaire.

---

## 3. Journal d'optimisation & Démarche d'ingénierie — /5

> _Attendu : justification théorique ET physique des gains sur les axes vus en cours. Chaque
> optimisation = un couple « Hypothèse d'impact matériel / Commande de vérification » (cf.
> constitution.md), avec chiffres avant/après._

### 3.0 Tableau des leviers identifiés (choix naïfs de la baseline)

| Choix naïf | Problème mécanique | Levier | Statut |
|---|---|---|---|
| `bestAsk/bestBid` par parcours complet de la map | O(n) par ordre (Table Scan) | Tableau de niveaux indexé par tick → O(1) | ✅ **fait & mesuré (§3.4) : −77 %** (goulot n°1 supprimé) |
| `[]*Order` (pointeurs dispersés) | Cache misses + pression GC | Structs contiguës | ✅ fait & mesuré (§3.1) |
| Prix `float64` | Arrondi + hashing lent | `int64` (ticks) | ✅ fait (§3.3) : `f64hash` éliminé du profil |
| `level[1:]` détruit la capacité → réallocations | `growslice`/`memmove`, pression GC | Index de tête (recyclage du tableau) | ✅ **fait & mesuré (§3.5) : −99,4 % d'allocs, −31 % temps** |
| Padding de `Order` | < ordres par ligne de cache 64 B | Réordonner les champs | ✅ vérifié (`make align`) : structs déjà alignées (32 o), non applicable |
| Flux tout en RAM | Empreinte O(n) | Streaming binaire | ⬜ à faire |
| Mono-thread | 1 cœur exploité | Worker pool borné | ⬜ à faire |

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

| Métrique | Avant (`map`, O(n)) | Après (tableau, O(1)) | Gain (benchstat, n=10) |
|---|---|---|---|
| Temps | 68,59 ms ± 2 % | **15,48 ms ± 2 %** | **−77,44 %** (p=0,000) |
| Allocations | 81,42 k | 78,09 k | −4,09 % (p=0,000) |
| Mémoire | 37,75 MiB | 37,62 MiB | −0,35 % |
| `bestAsk`/`bestBid` (CPU) | ~61 % | ~0 % | goulot supprimé |

**Lecture critique :** gain **algorithmique** (O(n) → O(1)) : il attaque le CPU, pas la mémoire (les
allocations ne bougent presque pas). C'est le plus gros levier de l'audit (−77 %), et il n'avait de
sens qu'**après** la preuve du profiling. Deux remarques :

- _Contrepartie :_ le tableau suppose une **plage de prix bornée** (`minTick..maxTick`), classique en
  HFT (les prix restent dans une bande étroite) ; en production on ajouterait un repli hors plage.
- _Re-profil (démarche itérative) :_ une fois le O(n) parti, le nouveau goulot devient
  `runtime.growslice`/`memmove` (~30 % du CPU) — les réallocations de slices. C'est ce que le §3.5
  attaque. Le profil s'est **aplati** : plus aucune fonction ne domine.

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

| Métrique | Avant (O(1)) | Après (index de tête) | Gain (benchstat, n=10) |
|---|---|---|---|
| Temps | 15,48 ms ± 2 % | **10,70 ms ± 1 %** | **−30,89 %** (p=0,000) |
| Allocations | 78 088 | **457** | **−99,41 %** (p=0,000) |
| Mémoire | 37,62 MiB | 34,37 MiB | −8,65 % |

**Lecture critique :** ici, contrairement au §3.1, réduire les allocations **fait aussi baisser le
temps** (−31 %) — parce que le §3.4 avait d'abord supprimé le O(n), rendant le `growslice`/`memmove`
dominant. L'ordre des leviers compte (Amdahl). Enfin, « zéro-alloc » ne veut pas dire 0 littéral : les
457 restantes sont **structurelles** (croissance du slice `trades`, dimensionnement initial des niveaux)
et **justifiées** au sens de la constitution. Les forcer à 0 serait contre-productif — voir le §4.

---

## 4. Confrontation critique & « Échec constructif » — /3

> _Attendu : documenter au moins UNE tentative d'optimisation contre-productive ou infructueuse,
> avec explication mécanique ET chiffrée de la régression avant retour arrière._

**Tentative : pré-allocation avide des niveaux (« over-provisioning »).** Après le levier zéro-alloc
(§3.5), il restait 457 allocations. Tentation naturelle : les faire disparaître en **pré-allouant**
chaque niveau à une grosse capacité dans `NewBook` (`make([]Order, 0, 1024)` pour les 201 niveaux × 2
côtés), pour « ne plus jamais réallouer ».

**Hypothèse initiale :** _réserver la capacité d'avance élimine la croissance des niveaux → moins
d'allocations **et** gain de temps (plus de `growslice`/`memmove`)._

**Résultat mesuré (régression, benchstat n=10) :**

| Métrique | Zéro-alloc (§3.5) | Piège (pré-alloc 1024) | Effet |
|---|---|---|---|
| Temps (`sec/op`) | 10,70 ms | 11,36 ms | **+6,16 %** (p=0,000) |
| Mémoire (`B/op`) | 34,37 MiB | 45,33 MiB | **+31,89 %** (p=0,000) |
| Allocations (`allocs/op`) | 457 | 463 | +1,31 % (p=0,000) |

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
| `make clean` | Supprime les fichiers éphémères (profils, binaires) ; garde `bench-results/` | — |
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

Comparaison `benchstat` (`baseline.txt` vs `opt-cache.txt`), 10 runs, n = 200 000, seed 42.

| Version (tag) | Temps | Allocations | Mémoire | Gain temps vs baseline |
|---|---|---|---|---|
| Baseline naïve (`baseline-naive`) | 77,8 ms ± 2 % | 281,4 k | 38,9 MiB | — |
| + localité de cache (`opt-cache-locality`) | 72,8 ms ± 3 % | **81,4 k** | 37,8 MiB | **−6,4 %** |
| + prix `int64` ticks (`opt-int64-ticks`) | 68,6 ms ± 2 % | 81,4 k | 37,8 MiB | **≈ −12 %** |
| + tableau de ticks O(1) (`opt-ticks-array`) | 15,48 ms ± 2 % | 78,1 k | 37,6 MiB | −77 % (÷ 4,4) |
| + zéro-alloc / index de tête (`opt-zero-alloc`) | **10,70 ms ± 1 %** | **457** | 34,4 MiB | **−86 % (÷ 7,3)** |
| **Finale (état actuel)** | **10,70 ms** | **457** | 34,4 MiB | **÷ 7,3 vs baseline** |

_Lecture : les leviers §3.1–3.3 sont des **micro-optimisations** : la localité gagne surtout sur les
**allocations** (−71 %) mais le temps ne baisse que de ~6 %, car le goulot temps est ailleurs — le
**O(n) de `bestAsk`/`bestBid`** (§2.3, ~61 % du CPU). Le levier `map → tableau de ticks` (§3.4)
supprime ce O(n) — **−77,44 %** (68,59 → 15,48 ms). Puis le §3.5 (index de tête) supprime les
réallocations redevenues visibles une fois le O(n) parti : **−99,4 % d'allocations** (78 k → 457) et,
cette fois, **−30,9 % de temps** (15,48 → 10,70 ms). Cumul : **÷ 7,3 vs la baseline** (77,75 → 10,70 ms).
Les deux gros leviers (§3.4, §3.5) illustrent la Loi d'Amdahl : chaque fois qu'on supprime le segment
dominant, le suivant devient l'objectif — et une optimisation « invisible » sur le temps (allocs au
§3.1) peut le devenir plus tard (§3.5)._

#### Mesure processus (hyperfine)

Complément **niveau processus** du binaire `cmd/bench` (`hyperfine --warmup 5 --runs 50`, 50 runs) :

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

- Sorties brutes de benchmark : `baseline.txt`, `optim.txt`
- Historique Git : tag `baseline-naive` → commits par levier