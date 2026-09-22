package engine

// Le carnet suppose une PLAGE DE PRIX BORNÉE (fenêtre autour du prix de référence),
// pratique classique d'un carnet HFT. Les bornes sont PARAMÉTRÉES (passées à NewBook),
// pas codées en dur : la plage est une donnée, partagée avec le générateur.
type Book struct {
	minTick    int64
	bids       [][]Order // index = tick - minTick ; chaque case = la file FIFO à ce prix
	asks       [][]Order
	// Les deux curseurs suivants pointent vers les niveaux de prix non vides les plus proches
	bestBidIdx int // plus HAUT bid non vide ; -1 si aucun
	bestAskIdx int // plus BAS ask non vide ; len(asks) si aucun
	trades     []Trade
}

// NewBook crée un carnet couvrant les prix de minTick à maxTick inclus (en ticks).
func NewBook(minTick, maxTick int64) *Book {
	n := int(maxTick - minTick + 1)
	return &Book{
		minTick:    minTick,
		bids:       make([][]Order, n),
		asks:       make([][]Order, n),
		bestBidIdx: -1,
		bestAskIdx: n,
	}
}
func (b *Book) Trades() []Trade { return b.trades }

func (b *Book) Submit(o *Order) {
	switch o.Side {
	case Buy:
		b.matchBuy(o)
	case Sell:
		b.matchSell(o)
	}
}

func (b *Book) matchBuy(o *Order) {
	for o.Quantity > 0 {
		if b.bestAskIdx >= len(b.asks) {
			break // plus aucun vendeur
		}
		askPrice := int64(b.bestAskIdx) + b.minTick
		if o.Type == Limit && askPrice > o.Price {
			break
		}
		level := b.asks[b.bestAskIdx]
		resting := &level[0]
		qty := min(o.Quantity, resting.Quantity)
		b.trades = append(b.trades, Trade{o.ID, resting.ID, askPrice, qty})
		o.Quantity -= qty
		resting.Quantity -= qty
		if resting.Quantity == 0 {
			b.asks[b.bestAskIdx] = level[1:]
			for b.bestAskIdx < len(b.asks) && len(b.asks[b.bestAskIdx]) == 0 {
				b.bestAskIdx++ // avance amortie du curseur au prochain niveau non vide
			}
		}
	}
	if o.Quantity > 0 && o.Type == Limit {
		idx := int(o.Price - b.minTick)
		b.bids[idx] = append(b.bids[idx], *o)
		if idx > b.bestBidIdx {
			b.bestBidIdx = idx
		}
	}
}

func (b *Book) matchSell(o *Order) {
	for o.Quantity > 0 {
		if b.bestBidIdx < 0 {
			break // plus aucun acheteur
		}
		bidPrice := int64(b.bestBidIdx) + b.minTick
		if o.Type == Limit && bidPrice < o.Price {
			break
		}
		level := b.bids[b.bestBidIdx]
		resting := &level[0]
		qty := min(o.Quantity, resting.Quantity)
		b.trades = append(b.trades, Trade{resting.ID, o.ID, bidPrice, qty})
		o.Quantity -= qty
		resting.Quantity -= qty
		if resting.Quantity == 0 {
			b.bids[b.bestBidIdx] = level[1:]
			for b.bestBidIdx >= 0 && len(b.bids[b.bestBidIdx]) == 0 {
				b.bestBidIdx-- // avance amortie du curseur vers le bas
			}
		}
	}
	if o.Quantity > 0 && o.Type == Limit {
		idx := int(o.Price - b.minTick)
		b.asks[idx] = append(b.asks[idx], *o)
		if idx < b.bestAskIdx {
			b.bestAskIdx = idx
		}
	}
}