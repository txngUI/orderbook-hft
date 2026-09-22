package engine_test

import (
	"testing"
	"orderbook-hft/internal/engine"
	"orderbook-hft/internal/feed"
)

func BenchmarkMatching(b *testing.B) {
	orders := feed.Generate(200_000, 42)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book := engine.NewBook(9900, 10100)
		for j := range orders {
			o := orders[j]
			book.Submit(&o)
		}
	}
}