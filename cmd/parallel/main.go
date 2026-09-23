package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"orderbook-hft/internal/engine"
	"orderbook-hft/internal/feed"
)

const (
	numSymbols      = 64
	ordersPerSymbol = 200_000
	baseSeed        = 42
)

var bookPool = sync.Pool{New: func() any { return engine.NewBook(9900, 10100) }}

func processSymbol(sym int, usePool bool) int {
	orders := feed.Generate(ordersPerSymbol, int64(baseSeed+sym))
	var book *engine.Book
	if usePool {
		book = bookPool.Get().(*engine.Book)
		book.Reset()
	} else {
		book = engine.NewBook(9900, 10100)
	}
	for i := range orders {
		o := orders[i]
		book.Submit(&o)
	}
	n := len(book.Trades())
	if usePool {
		bookPool.Put(book)
	}
	return n
}

func main() {
	pool := flag.Bool("pool", false, "recycler les carnets via sync.Pool")
	flag.Parse()

	workers := runtime.GOMAXPROCS(0)
	jobs := make(chan int, numSymbols)
	var totalTrades atomic.Int64

	var m0 runtime.MemStats
	runtime.ReadMemStats(&m0)
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
				totalTrades.Add(int64(processSymbol(sym, *pool)))
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	fmt.Printf("pool=%-5v workers=%d | temps=%-12v | trades=%d | alloc cumulé=%d MiB | Sys=%d MiB | GC=%d\n",
		*pool, workers, elapsed, totalTrades.Load(),
		(m1.TotalAlloc-m0.TotalAlloc)/1024/1024, m1.Sys/1024/1024, m1.NumGC-m0.NumGC)
}