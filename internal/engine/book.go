package engine

// Le carnet suppose une PLAGE DE PRIX BORNÉE (fenêtre autour du prix de référence),
// pratique classique d'un carnet HFT. Les bornes sont PARAMÉTRÉES (passées à NewBook),
// pas codées en dur : la plage est une donnée, partagée avec le générateur.

type Book struct {
	minTick    int32
	bids, asks [][]Order
	bidsHead   []int // front FIFO par niveau bid (index du plus ancien ordre non consommé)
	asksHead   []int // front FIFO par niveau ask
	bestBidIdx int
	bestAskIdx int
	trades     []Trade
}

// NewBook crée un carnet couvrant les prix de minTick à maxTick inclus (en ticks).
func NewBook(minTick, maxTick int32) *Book {
	n := int(maxTick - minTick + 1)
	return &Book{
		minTick:    minTick,
		bids:       make([][]Order, n),
		asks:       make([][]Order, n),
		bidsHead:   make([]int, n),
		asksHead:   make([]int, n),
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
		askPrice := int32(b.bestAskIdx) + b.minTick
		if o.Type == Limit && askPrice > o.Price {
			break
		}
		level := b.asks[b.bestAskIdx]
		h := b.asksHead[b.bestAskIdx]
		resting := &level[h]
		qty := min(o.Quantity, resting.Quantity)
		b.trades = append(b.trades, Trade{o.ID, resting.ID, askPrice, qty})
		o.Quantity -= qty
		resting.Quantity -= qty
		if resting.Quantity == 0 {
			h++
			if h == len(level) {             // niveau vidé → reset : le tableau est RÉUTILISÉ
				b.asks[b.bestAskIdx] = level[:0]
				b.asksHead[b.bestAskIdx] = 0
				for b.bestAskIdx < len(b.asks) && len(b.asks[b.bestAskIdx]) == 0 {
					b.bestAskIdx++
				}
			} else {                         // sinon on avance juste la tête
				b.asksHead[b.bestAskIdx] = h
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
			break
		}
		bidPrice := int32(b.bestBidIdx) + b.minTick
		if o.Type == Limit && bidPrice < o.Price {
			break
		}
		level := b.bids[b.bestBidIdx]
		h := b.bidsHead[b.bestBidIdx]
		resting := &level[h]
		qty := min(o.Quantity, resting.Quantity)
		b.trades = append(b.trades, Trade{resting.ID, o.ID, bidPrice, qty})
		o.Quantity -= qty
		resting.Quantity -= qty
		if resting.Quantity == 0 {
			h++
			if h == len(level) {
				b.bids[b.bestBidIdx] = level[:0]
				b.bidsHead[b.bestBidIdx] = 0
				for b.bestBidIdx >= 0 && len(b.bids[b.bestBidIdx]) == 0 {
					b.bestBidIdx--
				}
			} else {
				b.bidsHead[b.bestBidIdx] = h
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