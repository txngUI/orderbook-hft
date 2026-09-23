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
	Price    int32     // prix. int32 = int Java. (on met sous forme de ticks pour éviter les float et les arrondis et prendre moins de place mémoire)
	Quantity uint64    // quantité restant à exécuter
}

type Trade struct {
	BuyOrderID  uint64
	SellOrderID uint64
	Price       int32
	Quantity    uint64
}