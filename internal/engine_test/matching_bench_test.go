package engine

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
		book := engine.NewBook()
		for j := range orders {
			o := orders[j]
			book.Submit(&o)
		}
	}
}