package feed

import (
	"math/rand"
	"orderbook-hft/internal/engine"
)

// Generate produit n ordres déterministes à partir d'une graine.
// Même seed → même flux : indispensable pour comparer les mesures.
func Generate(n int, seed int64) []engine.Order {
	r := rand.New(rand.NewSource(seed)) // générateur semé
	orders := make([]engine.Order, n)          // slice de n Order, préallouée

	for i := 0; i < n; i++ {
		// côté : r.Intn(2) tire 0 ou 1
		side := engine.Buy
		if r.Intn(2) == 1 {
			side = engine.Sell
		}

		// type : ~10 % d'ordres MARKET (r.Intn(10) vaut 0 une fois sur dix)
		typ := engine.Limit
		if r.Intn(10) == 0 {
			typ = engine.Market
		}

		// prix resserré autour de 100.00 (entre 99.00 et 101.00, pas de 0.05)
		// → garantit beaucoup de matches, donc on mesure le vrai matching
		price := int64(10000 + (r.Intn(41)-20)*5)

		orders[i] = engine.Order{
			ID:       uint64(i + 1),
			Side:     side,
			Type:     typ,
			Price:    price,
			Quantity: uint64(r.Intn(100) + 1), // entre 1 et 100
		}
	}
	return orders
}