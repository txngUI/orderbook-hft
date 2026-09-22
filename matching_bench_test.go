package main

import "testing"

func BenchmarkMatching(b *testing.B) {
	orders := Generate(200_000, 42)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book := NewBook()
		for j := range orders {
			o := orders[j]
			book.Submit(&o)
		}
	}
}