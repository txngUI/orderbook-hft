# Order Book HFT — automatisation des mesures.
# Barème axe 5 : « automatisation complète en UNE commande ».
#
# Chemins : `./...` fonctionne que le projet soit à plat ou en cmd/ + internal/.

.PHONY: all test vet align bench bench-cache profile bench-hyperfine clean

FIELDALIGN := $(shell go env GOPATH)/bin/fieldalignment

## all : vérifie la correction puis mesure le moteur.
all: test bench

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

## bench : benchmark du moteur (temps, mémoire, allocations), 10 runs pour benchstat.
##   Sauver un "avant" :  make bench > baseline.txt
bench:
	go test -bench=Matching -benchmem -count=10 ./...

## bench-cache : expérience de localité contigu vs dispersé (le ×26).
bench-cache:
	go test -bench='Contiguous|Dispersed' -benchmem ./...

## profile : génère un profil CPU exploitable en flamegraph (barème axe 2).
##   Puis :  go tool pprof -http=:8080 cpu.prof
profile:
	go test -bench=Matching -benchmem -cpuprofile cpu.prof ./...
	@echo ">> profil écrit dans cpu.prof — visualiser : go tool pprof -http=:8080 cpu.prof"

## bench-hyperfine : mesure PROCESSUS du binaire entier (niveau end-to-end, complément de benchstat).
##   Installer : sudo pacman -S hyperfine
##   Avant/après : construire 2 binaires et faire
##     hyperfine --warmup 5 --runs 50 './ob_naif' './ob_opti' --export-markdown hyperfine.md
bench-hyperfine:
	go build -o ob ./cmd/bench
	hyperfine --warmup 5 --runs 50 './ob' --export-markdown hyperfine.md
	@echo ">> résultats dans hyperfine.md"

## clean : supprime les fichiers de mesure générés.
clean:
	rm -f cpu.prof *.test baseline.txt opt-*.txt final.txt hyperfine.md ob ob_naif ob_opti