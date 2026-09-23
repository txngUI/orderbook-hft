package engine

// Reset remet le carnet à zéro en RÉUTILISANT les tableaux déjà alloués
// (les niveaux repassent à len 0 mais gardent leur capacité). Permet de
// recycler un Book via sync.Pool sans réallouer (séance J3_PM).
func (b *Book) Reset() {
	for i := range b.bids {
		b.bids[i] = b.bids[i][:0]
		b.asks[i] = b.asks[i][:0]
		b.bidsHead[i] = 0
		b.asksHead[i] = 0
	}
	b.bestBidIdx = -1
	b.bestAskIdx = len(b.asks)
	b.trades = b.trades[:0]
}