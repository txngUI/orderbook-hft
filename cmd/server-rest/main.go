package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"orderbook-hft/internal/api"
)

func main() {
	addr    := flag.String("addr", ":8080", "adresse d'écoute (ex: :8080)")
	minTick := flag.Int("min", 9900, "prix minimum en ticks")
	maxTick := flag.Int("max", 10100, "prix maximum en ticks")
	flag.Parse()

	h := api.NewHandler(int32(*minTick), int32(*maxTick))

	mux := http.NewServeMux()
	h.Register(mux)

	fmt.Printf("Order Book REST server — écoute sur %s\n", *addr)
	fmt.Println("Routes :")
	fmt.Println("  POST /order   — soumettre un ordre")
	fmt.Println("  GET  /trades  — lister tous les trades")
	fmt.Println("  GET  /health  — liveness probe")

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("serveur arrêté : %v", err)
	}
}