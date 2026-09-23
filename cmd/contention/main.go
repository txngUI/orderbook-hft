package main

import (
	"fmt"
	"sync"
	"time"

	"orderbook-hft/internal/engine"
	"orderbook-hft/internal/feed"
)

const (
	nOrders = 200_000
	seed    = 42
	runs    = 5 // on garde le MEILLEUR temps (réduit bruit + DVFS à froid)
)

func runSequential(orders []engine.Order) (time.Duration, int) {
	book := engine.NewBook(9900, 10100)
	start := time.Now()
	for i := range orders {
		o := orders[i]
		book.Submit(&o)
	}
	return time.Since(start), len(book.Trades())
}

// runContended : le PIÈGE — G goroutines se partagent UN carnet via UN Mutex.
func runContended(orders []engine.Order, goroutines int) (time.Duration, int) {
	book := engine.NewBook(9900, 10100)
	var mu sync.Mutex
	var wg sync.WaitGroup
	chunk := (len(orders) + goroutines - 1) / goroutines
	start := time.Now()
	for g := 0; g < goroutines; g++ {
		lo, hi := g*chunk, g*chunk+chunk
		if hi > len(orders) {
			hi = len(orders)
		}
		if lo >= hi {
			break
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				o := orders[i]
				mu.Lock()
				book.Submit(&o)
				mu.Unlock()
			}
		}(lo, hi)
	}
	wg.Wait()
	return time.Since(start), len(book.Trades())
}

// best garde le meilleur temps sur `runs` exécutions (warmup implicite).
func best(f func() (time.Duration, int)) (time.Duration, int) {
	bestT := time.Hour
	tr := 0
	for i := 0; i < runs; i++ {
		t, n := f()
		if t < bestT {
			bestT = t
		}
		tr = n
	}
	return bestT, tr
}

func main() {
	orders := feed.Generate(nOrders, seed)

	tSeq, trSeq := best(func() (time.Duration, int) { return runSequential(orders) })
	fmt.Printf("séquentiel (0 verrou)      : %-12v  trades=%d  (correct)\n", tSeq, trSeq)

	for _, g := range []int{1, 2, 4, 8, 16} {
		gg := g
		t, tr := best(func() (time.Duration, int) { return runContended(orders, gg) })
		flag := ""
		if tr != trSeq {
			flag = "  ⚠ trades FAUX"
		}
		fmt.Printf("contendu   (%2d goroutines) : %-12v  trades=%d  (×%.2f vs séq)%s\n",
			g, t, tr, float64(t)/float64(tSeq), flag)
	}
}