# Rapport d'Audit de Performance — Order Book HFT

**Auteur :** Tanguy David
**Cours :** Optimisations & Performances Backend — Sup de Vinci, RNCP Bloc 4
**Projet :** Order Book HFT (moteur d'appariement, Go)
**Dépôt :** `[lien Git]` — baseline figée au tag `baseline-naive`

> **Comment remplir ce squelette :** chaque section correspond à un axe du barème (points
> indiqués). Les blocs en _italique_ rappellent ce qui est attendu. Remplace les `[À REMPLIR]`
> au fur et à mesure des séances. Le code n'est pas noté en soi : ce document est la seule pièce
> qui fait foi. Chaque affirmation de perf doit être **chiffrée et reproductible**.

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

> _Attendu : spécification précise du banc d'essai matériel + rigueur du protocole de mesure
> (warmup, nombre d'itérations, isolation du bruit, stats complètes)._

### 1.1 Banc d'essai matériel

| Élément | Valeur |
|---|---|
| CPU | AMD Ryzen 7 7735U (16 threads) |
| Hiérarchie de cache (L1/L2/L3) | L1d 32 Ko + L1i 32 Ko (par cœur) · L2 512 Ko (par cœur) · L3 16 Mo (partagé) · ligne 64 o |
| RAM | 16 Go LPDDR5 (4 × 4 Go) @ 6400 MT/s — ≈ 14 Go utilisables (~2 Go réservés à l'iGPU Radeon) |
| OS | Arch Linux — noyau `7.1.8-arch1-3` |
| Runtime | Go `go1.26.5-X:nodwarf5 linux/amd64` |
| Alimentation / gouverneur | **batterie** · gouverneur `powersave` |

> _Note de métrologie : les mesures ont été réalisées **sur batterie** avec le gouverneur `powersave`
> (DVFS actif → la fréquence CPU varie selon la charge et la température). L'usage le plus strict serait
> **secteur + gouverneur `performance`**, qui fige la fréquence pendant la mesure. Deux garde-fous
> rendent néanmoins les résultats fiables : (1) l'écart-type du banc est très faible (**1,2 %**, §1.3.2),
> preuve que le bruit résiduel est négligeable ; (2) **toutes** les versions sont mesurées dans les
> **mêmes conditions**, donc les gains relatifs (−77 %, etc.) restent valides — seuls les temps absolus
> seraient un peu plus bas sur secteur._

### 1.2 Protocole de mesure

Points communs aux deux mesures :
- **Charge :** `Generate(n, seed)` — flux d'ordres déterministe (même graine → même flux).
- **Isolation :** génération faite **hors chronomètre** ; on ne mesure que la boucle de matching.
- **Paramètres :** n = `[200 000]`, seed = `[42]`.

La mesure a été menée en **deux niveaux de rigueur croissants** :

| Niveau | Protocole | Warmup | Itérations | Statistiques |
|---|---|---|---|---|
| 1 — Chronométrage simple (Séance 1) | une exécution chronométrée | non | 1 run | temps brut, débit |
| 2 — Baseline consolidée | protocole rigoureux de référence | 1 run non mesuré | `[10]` runs | médiane + écart-type |

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

> _Limite assumée : un run unique, sans warmup, est sensible au bruit (le 1ᵉʳ run démarre caches
> froids et CPU à basse fréquence). Deux exécutions successives donnaient d'ailleurs ~103 vs ~106 ms.
> Ce chiffre sert d'ordre de grandeur, pas de référence défendable → d'où la consolidation ci-dessous._

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
| `bestAsk/bestBid` par parcours complet de la map | O(n) par ordre (Table Scan) | Structure triée → O(log n)/O(1) | 🔬 **prouvé par profiling (§2) : ~61 % du CPU** — refactor à venir |
| `[]*Order` (pointeurs dispersés) | Cache misses + pression GC | Structs contiguës | ✅ fait & mesuré (§3.1) |
| Prix `float64` | Arrondi + hashing lent | `int64` (ticks) | ✅ fait (§3.3) : `f64hash` éliminé du profil |
| Padding de `Order` | < ordres par ligne de cache 64 B | Réordonner les champs | ✅ vérifié (`make align`) : structs déjà alignées (32 o), non applicable |
| Flux tout en RAM | Empreinte O(n) | Streaming binaire | ⬜ à faire |
| Mono-thread | 1 cœur exploité | Worker pool borné | ⬜ à faire |

### 3.1 Levier : Localité de cache (contigu vs dispersé)

**Hypothèse d'impact matériel :** _des données dispersées (pointeurs) provoquent des cache misses
(~200 cycles de CPU stall chacun), là où un tableau contigu exploite les lignes de cache 64 B et
le prefetcher._

**Commande de vérification :** `go test -bench='Contiguous|Dispersed' -benchmem`

**Résultat de l'expérience (banc d'essai isolé, calcul identique, 0 alloc dans la boucle) :**

| Disposition | ns/op | Rapport |
|---|---|---|
| Contigu `[]Order` | 422 502 | référence |
| Dispersé `[]*Order` (accès mélangé) | 11 185 513 | **× 26,5** |

**Observation & analyse :**

- À **calcul strictement identique** (additionner 2 M `Quantity`), la seule variable est la
  disposition mémoire, et l'écart atteint **× 26,5**. Le `0 allocs/op` des deux benchmarks confirme
  qu'aucune allocation n'a lieu dans la boucle mesurée : l'écart est **100 % attribuable au cache**,
  pas au GC.
- **Pourquoi un écart aussi violent :** le working-set vaut 2 000 000 × 32 o ≈ **64 Mo**, très
  au-dessus du L3 (16 Mo). En accès **dispersé**, chaque lecture saute à une adresse imprévisible,
  rate les trois niveaux de cache et paie la pleine latence RAM (~200 cycles de _CPU stall_). En
  accès **contigu**, le _prefetcher_ matériel et les lignes de 64 o (≈ 2 `Order` par ligne) masquent
  cette latence : le cœur ne s'arrête quasiment jamais.
- **Limite assumée (honnêteté méthodologique) :** ce × 26,5 est un **majorant** — il mesure du
  pointer-chasing *pur* sur un très grand tableau. Le carnet réel ne gagnera pas ×26, car son Hot
  Path fait aussi autre chose (recherche dans la map, comparaisons de prix, appends). Cette
  expérience prouve que le levier **existe** et **borne** son potentiel ; le gain réel est mesuré
  ci-dessous sur le moteur.

**En clair (sans jargon) :** le processeur va chercher les données en mémoire par paquets, et il est
bien plus rapide quand elles sont **rangées côte à côte** (il en ramène plusieurs d'un coup) que
**dispersées** (un aller-retour lent par donnée). Stocker les ordres en un seul bloc plutôt
qu'éparpillés, c'est comme prendre toutes ses courses en un passage au lieu d'un aller-retour au
magasin par article. En prime, créer moins de « petites boîtes » séparées (allocations) évite au
ménage automatique de Go (le ramasse-miettes) de repasser sans arrêt.

**Application au carnet.** Mesure du moteur réel via `BenchmarkMatching` (n = 200 000) comme
« avant » ; le « après » sera relevé une fois le refactor contigu appliqué.

Banc : AMD Ryzen 7 7735U, Go/linux/amd64. Comparaison via `benchstat` (10 runs chacun).

| Métrique (`BenchmarkMatching`, n = 200 000) | Avant (naïf) | Après (contigu) | Gain |
|---|---|---|---|
| Temps | 77,8 ms ± 2 % | 72,8 ms ± 3 % | −6,4 % |
| Mémoire (`B/op`) | 38,9 MiB | 37,8 MiB | −3,0 % |
| Allocations (`allocs/op`) | 281 416 | **81 421** | **−71,1 %** |

> _Tous les écarts sont statistiquement significatifs (p = 0,000, n = 10) — ce ne sont pas du bruit.
> `allocs/op` et `B/op` sont **déterministes** (dépendent du code + seed 42, pas du CPU). Remarque de
> métrologie : sur un run isolé, le 1ᵉʳ tir était ~1,8× plus lent (démarrage à froid) — d'où l'usage
> de 10 runs + benchstat plutôt qu'un chronométrage unique._

**Analyse du gain (loi d'Amdahl, cours §6).** Le levier a **massivement réduit les allocations
(−71 %)** — donc la pression sur le Garbage Collector et l'empreinte mémoire — mais le **temps n'a
quasiment pas bougé (~4 %)**. Ce n'est pas un échec : c'est une preuve que, sur ce carnet, **les
allocations n'étaient pas le goulot d'étranglement du temps**. Le coût dominant est ailleurs : la
recherche du meilleur prix (`bestAsk`/`bestBid`) parcourt toute la map à **chaque** ordre — un
travail CPU en O(n) que ce refactor mémoire ne touche pas. Conclusion conforme à la loi d'Amdahl :
optimiser une fraction qui ne domine pas le temps ne peut donner qu'un gain marginal *sur le temps*.

Le levier reste un vrai acquis (moins de GC = latence plus prévisible sous charge soutenue, exigence
typique d'un système HFT). L'analyse pointe le vrai goulot **temps** — le O(n) de `bestAsk`/`bestBid`
— mais sur ce carnet, le supprimer **et** viser le zéro-allocation passent par **le même refactor**
(remplacer la `map` par un tableau de niveaux pré-alloué, indexé par tick). Le prochain levier traité,
**zéro-allocation** (§3.2), amorce donc ce chantier.

### 3.2 Levier : Zéro-allocation (escape analysis)

**Objectif :** tendre vers `0 allocs/op` sur le Hot Path (matching) pour supprimer la pression sur
le Garbage Collector.

**Rappel — l'analyse d'échappement.** En Go, on n'alloue pas la mémoire à la main : c'est le
**compilateur qui décide seul**, à la compilation, si une variable vit sur la **pile** (gratuit,
libéré au retour de fonction, zéro GC, cache L1) ou « s'échappe » sur le **tas** (coûteux, à la
charge du GC). Le drapeau `-gcflags="-m"` expose ces décisions, sans exécuter le programme.

**Hypothèse d'impact :** _si des variables du Hot Path s'échappent involontairement (pointeur local
retourné, boxing `interface{}`, slice de taille variable), elles génèrent des allocations tas
évitables → pression GC._

**Commande de vérification :** `go build -gcflags="-m" ./... 2>&1 | grep "escapes to heap\|moved to heap"`

**Résultats, classés par chemin d'exécution :**

| Origine | Échappement signalé | Chemin | Verdict |
|---|---|---|---|
| `NewBook` | `&Book{}` + 2 × `make(map…)` | froid (1×/carnet) | négligeable |
| `book.go` (matchBuy/matchSell) | `append escapes to heap` (×4) | **CHAUD** | **source des 81 421 allocs** |
| `generator.go` | `make([]Order, n)`, `rand.rng` | setup (hors mesure) | négligeable |
| `main.go` | arguments de `fmt.Printf` | froid (1×) | négligeable (boxing `interface{}`) |

**Analyse :**

- **Aucun échappement _accidentel_ sur le Hot Path** : pas de pointeur local retourné par erreur,
  pas de boxing involontaire dans la boucle de matching. Le code chaud est sain de ce côté.
- Les **4 `append escapes to heap`** sont les `append(b.bids/asks[prix], *o)` qui déposent un ordre
  au repos. Ils s'échappent **légitimement** : la slice d'un niveau est stockée dans une `map` (sur
  le tas), donc son tableau sous-jacent doit vivre sur le tas. **C'est la source des allocations** —
  confirmée dès la compilation.
- Le `fmt.Printf` de `main.go` illustre la cause « assignation à une interface » (ses arguments sont
  boxés en `any`). Sans conséquence ici (chemin froid, 1 appel), mais interdit sur le Hot Path
  (cf. `constitution.md`).

**Confirmation par profil mémoire** (`go test -memprofile` + `pprof -sample_index=alloc_objects`) —
les allocations se concentrent dans le matching :

| Fonction | Part des allocations |
|---|---|
| `matchBuy` | 50,3 % |
| `matchSell` | 47,5 % |
| runtime (bruit) | 1,7 % |

Soit **97,8 % dans le Hot Path**. Le nœud `runtime.growslice` présent dans le profil confirme le
mécanisme : c'est l'`append` qui agrandit les slices de niveaux (adossées à la map) qui alloue. La
source *runtime* coïncide exactement avec l'analyse d'échappement *compile-time* — diagnostic
cohérent des deux côtés.

**Conséquence.** Ces allocations sont **structurelles** (slices adossées à une `map`), pas un
ajustement local. Les éliminer impose de remplacer la `map` par un tableau de niveaux pré-alloué
(indexé par tick) — le refactor identifié au §3.1, à mener **après** le profiling (Séance J2_PM) qui
confirmera formellement le goulot. Ce refactor réglera zéro-allocation **et** le O(n) de `bestAsk`
d'un même geste.

**Acquis connexe — padding :** vérifié via `make align` (`fieldalignment`) → structs déjà alignées
(32 o), aucun réordonnancement gagnant (cf. §3.0).

### 3.3 Levier : Prix en `int64` (ticks)

**Objectif :** remplacer le prix `float64` par un entier `int64` en **ticks** (plus petite unité —
ici le centime : `100,05 € → 10005`).

**Hypothèse d'impact matériel :** _le profiling (§2) montre ~9 % du CPU dans le hachage des clés
`float64` de la map (`f64hash` + `aeshashbody`). Une clé `int64` se hache et se compare en beaucoup
moins de cycles → ce coût doit disparaître. Bénéfice métier en prime : plus d'erreur d'arrondi
flottant. Ce changement est aussi le **prérequis** de l'indexation par tableau (§3.4)._

**Commande de vérification :** `go test -bench=Matching -cpuprofile cpu.prof … ` puis
`go tool pprof -top cpu.prof | grep -i hash`

**Endroits modifiés :** `order.go` (`Order.Price`, `Trade.Price` → `int64`), `book.go`
(`map[int64][]Order`, `bestAsk`/`bestBid` → `int64`), `generator.go` (prix en ticks),
`book_test.go` (prix des cas de test).

**Résultat :** tests verts (comportement identique — même flux, mêmes trades).

| Métrique | Avant (`float64`) | Après (`int64` ticks) | Gain |
|---|---|---|---|
| Temps (benchstat) | 72,8 ms ± 3 % | 68,6 ms ± 2 % | **−5,8 %** |
| Hachage des clés (CPU) | `f64hash` + `aeshashbody` ~9 % | `memhash64` ~2,5 % | ÷ ~4 |
| Allocations (`allocs/op`) | 81,42 k | 81,42 k | inchangé |
| Mémoire (`B/op`) | 37,75 MiB | 37,75 MiB | inchangé |

Le levier ne touche pas au stockage (mémoire/allocs identiques) : son gain est purement **CPU**
(hachage des clés). Le O(n) de `bestAsk`/`bestBid` reste le goulot dominant → §3.4.

---

### 3.4 Levier : `map` → tableau de ticks (meilleur prix en O(1)) — **levier majeur**

**Objectif :** remplacer `map[int64][]Order` par un **tableau pré-alloué indexé par tick**
(`[][]Order`, index = `tick − minTick`), avec deux **curseurs** `bestBidIdx` / `bestAskIdx`
maintenus à jour. Le meilleur prix devient une **lecture directe du curseur — O(1)** — au lieu d'un
**parcours complet de la map — O(n)** — à chaque ordre.

**Hypothèse d'impact matériel :** _le profiling (§2.3) prouve que `bestAsk` + `bestBid` consomment
**~61 % du CPU** : à chaque ordre, on itère toute la structure pour trouver le min/max prix. En
rangeant les niveaux dans un tableau contigu et en gardant un curseur sur le meilleur niveau non
vide, cette recherche disparaît (O(n) → O(1)). C'est précisément le segment désigné par la Loi
d'Amdahl (§2) : on optimise ce qui domine réellement le temps, pas une intuition._

**Pré-requis :** clé de prix `int64` (§3.3) — un tableau ne s'indexe que par un entier borné. Les
deux leviers forment une paire : `int64` était le socle, l'indexation est la macro-optimisation.

**Commande de vérification :** `make save NAME=opt-ticks-array` puis
`make compare A=opt-ticks B=opt-ticks-array` (benchstat)

**Endroits modifiés :**

| Fichier | Modification |
|---|---|
| `book.go` | `map[int64][]Order` → `bids`, `asks [][]Order` ; bornes **paramétrées** (`minTick`/`maxTick` passés à `NewBook`, stockés en champ) qui pré-allouent les deux tableaux ; curseurs `bestBidIdx`/`bestAskIdx` ; méthodes `bestAsk`/`bestBid` **supprimées** (remplacées par la lecture du curseur) ; avance **amortie** du curseur quand un niveau se vide |
| `book_test.go` | `NewBook(9900, 10100)` ; accès interne par index : `b.bids[int(10000−b.minTick)][0]` ; vérif reliquat MARKET via `b.bestBidIdx != -1` |
| `matching_bench_test.go`, `cmd/bench/main.go` | appel `NewBook(9900, 10100)` (bornes de la plage) |
| `order.go` | inchangé (déjà `int64` depuis §3.3) |
| `generator.go` | inchangé (prix déjà en ticks, dans la plage `minTick..maxTick`) |

**Choix de conception (bornes en paramètre, pas en `const`) :** on aurait pu figer `minTick`/`maxTick`
en constantes de compilation. On les passe en **paramètres** de `NewBook` : la plage devient une
_donnée_ explicite au point d'appel, sans nombre magique ni couplage caché avec le générateur. Le
surcoût (lire un champ au lieu d'une constante inlinée) est **non mesurable** — vérifié : le benchmark
est identique aux deux formes. Une constante ne redeviendrait _obligatoire_ que pour une variante à
**tableau de taille fixe** `[N][]Order` (une indirection mémoire de moins, mais taille figée à la
compilation) — piste possible, non retenue ici.

**Résultat :** tests verts (même flux → mêmes trades ; le refactor ne change pas le comportement).

| Métrique | Avant (`map`, O(n)) | Après (tableau, O(1)) | Gain (benchstat, n=10) |
|---|---|---|---|
| Temps (`sec/op`) | 68,59 ms ± 2 % | **15,48 ms ± 2 %** | **−77,44 %** (p=0,000) |
| Allocations (`allocs/op`) | 81,42 k | **78,09 k** | **−4,09 %** (p=0,000) — buckets de `map` supprimés |
| Mémoire (`B/op`) | 37,75 MiB | 37,62 MiB | −0,35 % (p=0,000) |
| `bestAsk`/`bestBid` (CPU) | ~61 % du profil | ~0 % (lecture curseur) | **goulot supprimé** |

_Écart validé statistiquement (Mann-Whitney, `p=0,000`, n=10) : le gain temps est réel, pas du bruit
de mesure._

> Banc témoin (conteneur Xeon, même code avant/après) : 178,7 → 37,1 ms (÷ 4,8) — même ordre de
> grandeur que la Ryzen, ce qui confirme que le gain est structurel (algorithmique), pas un artefact
> de machine.

**Lecture critique :** le gain est **essentiellement algorithmique** (O(n) → O(1)). Il attaque le
**CPU** — les cycles brûlés à scanner la structure à chaque ordre — et non la mémoire : les
allocations ne baissent que de 4 % (disparition des *buckets* de la `map`), le *volume* de données
restant identique. C'est l'inverse du levier §3.1 (localité), qui ne touchait qu'aux allocations sans
bouger le temps. Contrairement aux micro-leviers §3.1–3.3 (gains de −5,8 % à −6,4 %), celui-ci est
**macro** et cible le plus gros segment du profil : c'est le **levier dominant** de l'audit (−77 % à
lui seul), et il n'a de sens qu'*après* la preuve du profiling — on ne l'aurait pas deviné avant §2.

**Contrepartie assumée :** le tableau suppose une **plage de prix bornée** (`minTick..maxTick`,
fenêtre autour du prix de référence) — choix classique d'un carnet HFT, les prix évoluant dans une
bande étroite (documenté dans `book.go`). Un prix hors plage n'est pas géré ; acceptable ici car le
générateur reste dans la fenêtre. En production : repli sur une map de débordement, ou re-centrage
dynamique de la fenêtre.

**Observation — le goulot s'est déplacé (re-profilage) → prochain levier « zéro-alloc ».** Après avoir
supprimé le O(n), on **re-profile** (démarche itérative, §2) : le hot path a changé de nature.

| Fonction (nouveau top CPU) | Part | Nature |
|---|---|---|
| `matchBuy` + `matchSell` (temps propre / *flat*) | **~41 %** | logique de matching : comparaisons, curseurs, `append` |
| `runtime.growslice` (*cum*, dont `memmove` 16 %, `memclr` 5 %) | **~30 %** | réallocation + recopie des slices de niveaux à chaque `append` |
| `bestAsk` / `bestBid` / `maps.(*Iter).Next` / `memhash64` | **0 %** | **disparus** — le O(n) et le hachage de map sont éliminés (preuve du levier) |
| `madvise` / `procyield` / GC | reste | pression GC induite par les allocations |

Un profil mémoire (`-memprofile`, `-sample_index=alloc_objects`) confirme la source exacte des
allocations :

```
matchBuy/matchSell : b.bids[idx]/b.asks[idx] = append(…, *o)   → 99,96 % des objets
matchBuy/matchSell : b.trades = append(…, Trade{})             →  0,04 %
```

Ces allocations ne peuvent pas tomber à 0 : un carnet **stocke** par nature les ordres au repos et
**produit** des trades. Mécanisme du surplus : quand un niveau se vide, `level[1:]` avance la tête mais
**réduit la capacité** du slice ; au re-remplissage, `append` réalloue un tableau neuf (d'où le
`growslice`/`memmove`). Les niveaux proches du spread se remplissant/vidant en continu, chaque
re-remplissage coûte une allocation.

**Point de méthode (le profil s'est aplati) :** avant le refactor, `bestAsk`/`bestBid` écrasaient tout
(61 % du CPU) ; après, **plus aucune fonction ne domine**. Le temps se répartit entre la logique de
matching (~41 %, largement incompressible) et la croissance des slices (~30 %, l'allocation). C'est le
signe qu'on entre dans les **rendements décroissants** : le grand gain (O(n) → O(1)) est derrière nous.
Le seul segment encore franchement attaquable est l'allocation.

**Levier futur (cible « zéro-alloc » de la `constitution.md`) :** pré-dimensionner les niveaux
(`make([]Order, 0, cap)`) ou remplacer `level[1:]` par un **index de tête** (`head int`, file
circulaire) qui réutilise le tableau au lieu de le réallouer → `allocs/op` de ~78 k vers quelques
centaines, et un gain temps **réel mais borné** (le `growslice`/`memmove`, soit ~30 % du CPU — plus les
61 % du levier précédent) — à mesurer avant d'affirmer (Règle d'Or).

---

## 4. Confrontation critique & « Échec constructif » — /3

> _Attendu : documenter au moins UNE tentative d'optimisation contre-productive ou infructueuse,
> avec explication mécanique ET chiffrée de la régression avant retour arrière._

**Tentative :** `[À REMPLIR — ex. parallélisation prématurée, cache trop gros, verrouillage trop fin]`

**Hypothèse initiale :** `[À REMPLIR]`

**Résultat mesuré (régression) :** `[À REMPLIR — chiffres]`

**Explication mécanique :** `[À REMPLIR — ex. context-switching > gain, cache augmente la pression GC…]`

**Décision :** retour arrière — `[À REMPLIR]`

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
| + tableau de ticks O(1) (`opt-ticks-array`) | **15,48 ms ± 2 %** | **78,1 k** | 37,6 MiB | **−77 % (÷ 4,4)** |
| **Finale (état actuel)** | **15,48 ms** | **78,1 k** | 37,6 MiB | **÷ 5,0 vs baseline** |

_Lecture : les leviers §3.1–3.3 sont des **micro-optimisations** : la localité gagne surtout sur les
**allocations** (−71 %) mais le temps ne baisse que de ~6 %, car le goulot temps est ailleurs — le
**O(n) de `bestAsk`/`bestBid`** (§2.3, ~61 % du CPU). Le levier `map → tableau de ticks` (§3.4)
supprime ce O(n) : c'est le levier **macro**, le seul qui attaque le segment dominant du profil, d'où
le saut de **−77,44 %** (p=0,000) sur le temps à lui seul (68,59 → 15,48 ms), qui porte le cumul à
**÷ 5,0 vs la baseline** (77,8 → 15,48 ms). Les allocations, elles, ne bougent quasiment pas (−4 %) :
le gain est algorithmique (CPU), pas allocatoire._

#### Mesure processus (hyperfine)

Complément **niveau processus** du binaire `cmd/bench` (`hyperfine --warmup 5 --runs 50`, 50 runs) :

| Binaire | Temps processus (mean ± σ) | Min / Max |
|---|---|---|
| Baseline naïve (`ob`) | 83,1 ms ± 7,8 ms | 76,9 / 117,8 ms |
| **Finale — tableau O(1)** (`ob_opti`) | **20,9 ms ± 1,9 ms** | 18,3 / 26,1 ms |
| **Rapport** | **÷ 3,97 ± 0,52** | — |

**Décomposition (contrôle de cohérence).** Le coût **fixe** du binaire (démarrage du runtime Go +
génération des 200 000 ordres) est incompressible et identique des deux côtés :

| Binaire | Process total | ≈ matching (benchstat) | ≈ fixe |
|---|---|---|---|
| `ob` (baseline) | 83,1 ms | 77,8 ms | **5,3 ms** |
| `ob_opti` (finale) | 20,9 ms | 15,5 ms | **5,4 ms** |

Le terme fixe tombe sur ~5,4 ms des deux côtés : cela **confirme** que `ob` est bien la version naïve
(et non une version intermédiaire) et que le seul écart entre les deux binaires est le matching.

_Lecture (Loi d'Amdahl au niveau processus) : le ratio processus (**÷3,97**) est plus faible que le
**÷5,0** du matching pur (benchstat), parce que hyperfine mesure **tout le binaire**, y compris les
~5,4 ms de coût fixe que l'optimisation ne touche pas. Autrement dit, plus on optimise le matching,
plus la part fixe (startup + génération) pèse dans le total — c'est la limite d'Amdahl qui se déplace
vers le segment non optimisé. (hyperfine a signalé un premier run à froid à 117,8 ms sur `ob` —
outlier de cache, absorbé par la moyenne sur 50 runs.)_

---

## 6. BONUS — Gouvernance technique (`constitution.md`) — +2

> _Attendu : fichier de contrainte à la racine, 4 directives respectées (posture système, gardes-fous
> négatifs, justification empirique hypothèse/commande, formatage impératif)._

- [ ] `constitution.md` présent à la racine du dépôt
- [ ] Directive 1 — posture système (ingénieur contraint par métriques)
- [ ] Directive 2 — gardes-fous (bannir `fmt.Sprintf` sur Hot Path, goroutines bornées…)
- [ ] Directive 3 — justification empirique (hypothèse / commande de preuve)
- [ ] Directive 4 — formatage impératif et vérifiable

---

## Annexes

- Sorties brutes de benchmark : `baseline.txt`, `optim.txt`
- Historique Git : tag `baseline-naive` → commits par levier