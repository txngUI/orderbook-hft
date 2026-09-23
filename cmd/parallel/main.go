package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"orderbook-hft/internal/engine"
	"orderbook-hft/internal/feed"
)

const (
	numSymbols      = 64        // 64 carnets indépendants (≈ 4 par cœur sur 16 threads)
	ordersPerSymbol = 200_000
	baseSeed        = 42
)

func processSymbol(sym int) int {
	orders := feed.Generate(ordersPerSymbol, int64(baseSeed+sym))
	book := engine.NewBook(9900, 10100)
	for i := range orders {
		o := orders[i]
		book.Submit(&o)
	}
	return len(book.Trades())
}

func main() {
	workers := runtime.GOMAXPROCS(0)
	jobs := make(chan int, numSymbols)
	var totalTrades atomic.Int64

	start := time.Now()

	for s := 0; s < numSymbols; s++ {
		jobs <- s
	}
	close(jobs)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sym := range jobs {
				totalTrades.Add(int64(processSymbol(sym)))
			}
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)
	fmt.Printf("workers=%d | symboles=%d (%d ordres) | trades=%d | temps=%v\n",
		workers, numSymbols, ordersPerSymbol, totalTrades.Load(), elapsed)
}