package main

import (
	"fmt"
	"time"
	"orderbook-hft/internal/engine"
	"orderbook-hft/internal/feed"
)

func main() {
	const n = 200_000 // le _ est juste un séparateur lisible : 200000

	// 1. On génère le flux AVANT de chronométrer.
	//    On ne veut mesurer que le matching, pas la génération.
	orders := feed.Generate(n, 42)

	book := engine.NewBook()

	// 2. On chronomètre UNIQUEMENT la boucle d'injection.
	start := time.Now()
	for i := 0; i < n; i++ {
		o := orders[i]   // copie de l'ordre
		book.Submit(&o)  // on soumet un pointeur vers la copie
	}
	elapsed := time.Since(start)

	// 3. On calcule les indicateurs de référence.
	debit := float64(n) / elapsed.Seconds()
	nsParOrdre := float64(elapsed.Nanoseconds()) / float64(n)

	fmt.Println("=== BASELINE — Order Book naïf ===")
	fmt.Printf("Ordres injectés : %d\n", n)
	fmt.Printf("Trades générés  : %d\n", len(book.Trades()))
	fmt.Printf("Temps total     : %v\n", elapsed)
	fmt.Printf("Débit           : %.0f ordres/s\n", debit)
	fmt.Printf("Coût unitaire   : %.1f ns/ordre\n", nsParOrdre)
}