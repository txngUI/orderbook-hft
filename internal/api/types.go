package api

// OrderRequest est le corps JSON d'une requête POST /order.
// Side  : "buy" | "sell"
// Type  : "limit" | "market"
// Price : en ticks (ignoré pour les ordres market)
type OrderRequest struct {
	ID       uint64 `json:"id"`
	Side     string `json:"side"`
	Type     string `json:"type"`
	Price    int32  `json:"price"`
	Quantity uint64 `json:"quantity"`
}

// TradeResponse est la représentation JSON d'une transaction.
type TradeResponse struct {
	BuyOrderID  uint64 `json:"buy_order_id"`
	SellOrderID uint64 `json:"sell_order_id"`
	Price       int32  `json:"price"`
	Quantity    uint64 `json:"quantity"`
}

// SubmitResponse est la réponse JSON d'un POST /order.
// Trades contient les transactions produites par cet ordre (peut être vide).
type SubmitResponse struct {
	Trades []TradeResponse `json:"trades"`
}

// ErrorResponse est la réponse JSON en cas d'erreur.
type ErrorResponse struct {
	Error string `json:"error"`
}