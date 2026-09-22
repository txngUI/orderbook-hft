package engine

// Side = côté de l'ordre : achat ou vente.
// Go n'a pas d'enum comme Java. L'idiome : un type entier + des constantes.
type Side uint8

const (
	Buy  Side = iota // iota = 0 pour la 1re ligne
	Sell             // puis 1 automatiquement
)

// OrderType : LIMIT (reste au carnet si pas exécuté) ou MARKET (exécuté tout de suite).
type OrderType uint8

const (
	Limit OrderType = iota // 0
	Market                 // 1
)

// Order = un ordre dans le carnet.
type Order struct {
	ID       uint64    // identifiant. uint64 = entier non signé 64 bits (comme long en Java, mais >= 0)
	Side     Side      // Buy ou Sell
	Type     OrderType // Limit ou Market
	Price    float64   // prix. float64 = double Java. (choix NAÏF, on y reviendra)
	Quantity uint64    // quantité restant à exécuter
}

type Trade struct {
	BuyOrderID  uint64
	SellOrderID uint64
	Price       float64
	Quantity    uint64
}