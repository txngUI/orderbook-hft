package main

import "testing"

// TestSimpleCross : un achat qui croise une vente au repos produit 1 trade.
func TestSimpleCross(t *testing.T) {
	b := NewBook()
	b.Submit(&Order{ID: 1, Side: Sell, Type: Limit, Price: 100, Quantity: 10})
	b.Submit(&Order{ID: 2, Side: Buy, Type: Limit, Price: 100, Quantity: 10})

	if len(b.trades) != 1 {
		t.Fatalf("attendu 1 trade, obtenu %d", len(b.trades))
	}
	if b.trades[0].Quantity != 10 {
		t.Fatalf("attendu quantité 10, obtenu %d", b.trades[0].Quantity)
	}
}