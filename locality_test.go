package main

import (
	"math/rand"
	"testing"
)

const benchN = 2_000_000

// CONTIGU : un seul bloc mémoire. Les Order sont collés les uns aux autres.
func makeContiguous(n int) []Order {
	s := make([]Order, n)
	for i := range s {
		s[i].Quantity = uint64(i)
	}
	return s
}

// DISPERSÉ : n allocations séparées (new = un Order sur le tas), puis on
// MÉLANGE l'ordre de visite pour casser toute régularité d'accès.
func makeDispersed(n int) []*Order {
	ptrs := make([]*Order, n)
	for i := range ptrs {
		o := new(Order) // alloue UN Order isolé sur le tas, renvoie son pointeur
		o.Quantity = uint64(i)
		ptrs[i] = o
	}
	rand.New(rand.NewSource(1)).Shuffle(n, func(i, j int) {
		ptrs[i], ptrs[j] = ptrs[j], ptrs[i]
	})
	return ptrs
}

// On fait EXACTEMENT le même calcul dans les deux cas : additionner les Quantity.
// Seule la disposition mémoire change.

func BenchmarkContiguous(b *testing.B) {
	data := makeContiguous(benchN)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum uint64
		for j := range data {
			sum += data[j].Quantity
		}
		_ = sum
	}
}

func BenchmarkDispersed(b *testing.B) {
	data := makeDispersed(benchN)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sum uint64
		for j := range data {
			sum += data[j].Quantity // ici on suit un pointeur → saut mémoire
		}
		_ = sum
	}
}