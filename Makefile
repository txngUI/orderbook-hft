# Order Book HFT — automatisation des mesures.
# Barème axe 5 : « automatisation complète en UNE commande ».
#
# Chemins : `./...` fonctionne que le projet soit à plat ou en cmd/ + internal/.

.PHONY: all help test vet align bench bench-cache profile bench-hyperfine save compare clean snapshot server-rest server-rest-build server-grpc server-grpc-build proto

FIELDALIGN := $(shell go env GOPATH)/bin/fieldalignment
RESULTS    := bench-results

BENCHPKG   := ./internal/engine_test  

## all : vérifie la correction puis mesure le moteur.
all: test bench

## help : affiche cette aide.
help:
	@awk 'BEGIN {printf "Targets disponibles:\n\n"} /^##[[:space:]]/ {sub(/^##[[:space:]]?/, "", $$0); if (help) help = help " " $$0; else help = $$0; next} /^[A-Za-z0-9_.-]+:([^=].*)?$$/ {if (help != "") printf "  %-18s %s\n", $$1, help; help = ""}' $(MAKEFILE_LIST)

## test : tests de correction (doivent rester verts après chaque optimisation).
test:
	go test ./...

## vet : analyse statique.
vet:
	go vet ./...

## align : détecte le padding des structs (alignement mémoire, barème axe 3).
##   Installe fieldalignment au besoin, puis l'exécute. Rien en sortie = structs déjà optimales.
##   Réordonner automatiquement : fieldalignment -fix ./...
align:
	@test -x "$(FIELDALIGN)" || go install golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest
	go vet -vettool=$(FIELDALIGN) ./...

## bench : benchmark du moteur (temps, mémoire, allocations), 10 runs. Affiche à l'écran.
bench:
	go test -bench=Matching -benchmem -count=10 ./...

## save NAME=<tag> : lance le benchmark et l'archive dans bench-results/<tag>.txt (pour benchstat).
##   Ex :  make save NAME=baseline   puis   make save NAME=opt-ticks
save:
	@test -n "$(NAME)" || { echo "Usage : make save NAME=<tag>"; exit 1; }
	@mkdir -p $(RESULTS)
	go test -bench=Matching -benchmem -count=10 ./... | tee $(RESULTS)/$(NAME).txt
	@echo ">> archivé dans $(RESULTS)/$(NAME).txt"

## compare A=<tag> B=<tag> : benchstat entre deux résultats archivés.
##   Ex :  make compare A=baseline B=opt-ticks
compare:
	@test -n "$(A)" -a -n "$(B)" || { echo "Usage : make compare A=<tag> B=<tag>"; exit 1; }
	benchstat $(RESULTS)/$(A).txt $(RESULTS)/$(B).txt

## bench-cache : expérience de localité contigu vs dispersé (le ×26).
bench-cache:
	go test -bench='Contiguous|Dispersed' -benchmem ./...

profile:
	go test -bench=Matching -cpuprofile cpu.prof $(BENCHPKG)
	@echo ">> profil écrit dans cpu.prof — visualiser : go tool pprof -http=:8080 cpu.prof"

## bench-hyperfine : mesure PROCESSUS du binaire entier (niveau end-to-end, complément de benchstat).
##   Installer : sudo pacman -S hyperfine
##   Avant/après : construire 2 binaires et faire
##     hyperfine --warmup 5 --runs 50 './ob_naif' './ob_opti' --export-markdown hyperfine.md
bench-hyperfine:
	go build -o ob ./cmd/bench
	hyperfine --warmup 5 --runs 50 './ob' --export-markdown hyperfine.md
	@echo ">> résultats dans hyperfine.md"

## clean : supprime les fichiers ÉPHÉMÈRES (profils, binaires). Garde bench-results/ (pièces à conviction).
##   Pour effacer aussi les résultats archivés :  rm -rf bench-results
clean:
	rm -f cpu.prof mem.prof *.test hyperfine.md ob ob_naif ob_opti
	rm -f baseline.txt opt-*.txt final.txt   # anciens .txt éventuellement restés à la racine

## snapshot : génère la vue HTML du carnet (snapshot statique) et affiche le chemin.
snapshot:
	go run ./cmd/snapshot
	@echo ">> ouvre snapshot.html dans un navigateur"

# ── Serveur REST (Version A) ─────────────────────────────────────────────────
server-rest:
	go run ./cmd/server-rest

server-rest-build:
	go build -o ob-rest ./cmd/server-rest

server-grpc:
	go run ./cmd/server-grpc

server-grpc-build:
	go build -o ob-grpc ./cmd/server-grpc

proto:
	protoc \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/order.proto
	mv proto/order.pb.go proto/order_grpc.pb.go internal/pb/