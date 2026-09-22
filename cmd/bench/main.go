package main

import (
	"fmt"
	"time"
	"orderbook-hft/internal/engine"
	"orderbook-hft/internal/feed"
)

func main() {
	const n = 200_000 // le _ est juste un séparateur lisible : 200000

	orders := feed.Generate(n, 42)
	book := engine.NewBook(9900, 10100)
	start := time.Now()

	// 2. On chronomètre UNIQUEMENT la boucle d'injection.
	for i := 0; i < n; i++ {
		o := orders[i]   // copie de l'ordre
		book.Submit(&o)  // on soumet un pointeur vers la copie
	}
	elapsed := time.Since(start)
	fmt.Printf("Ordres: %d | Trades: %d | Temps: %v\n", n, len(book.Trades()), elapsed)
}