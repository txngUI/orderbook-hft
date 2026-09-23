package engine

// Level : un niveau de prix agrégé (somme des quantités au repos à ce prix).
type Level struct {
	Price int32  // en ticks (centimes)
	Size  uint64
}

// Snapshot : photo en LECTURE SEULE du carnet (appelée hors Hot Path).
type Snapshot struct {
	Bids    []Level // meilleur d'abord (prix le plus HAUT)
	Asks    []Level // meilleur d'abord (prix le plus BAS)
	BestBid, BestAsk int32
	HasBid, HasAsk   bool
}

func (b *Book) Snapshot(depth int) Snapshot {
	var s Snapshot
	for idx := b.bestAskIdx; idx < len(b.asks) && len(s.Asks) < depth; idx++ {
		lvl := b.asks[idx][b.asksHead[idx]:] // ordres encore au repos (après la tête)
		if len(lvl) == 0 {
			continue
		}
		var sz uint64
		for i := range lvl {
			sz += lvl[i].Quantity
		}
		s.Asks = append(s.Asks, Level{Price: int32(idx) + b.minTick, Size: sz})
	}
	for idx := b.bestBidIdx; idx >= 0 && len(s.Bids) < depth; idx-- {
		lvl := b.bids[idx][b.bidsHead[idx]:]
		if len(lvl) == 0 {
			continue
		}
		var sz uint64
		for i := range lvl {
			sz += lvl[i].Quantity
		}
		s.Bids = append(s.Bids, Level{Price: int32(idx) + b.minTick, Size: sz})
	}
	if len(s.Asks) > 0 {
		s.BestAsk, s.HasAsk = s.Asks[0].Price, true
	}
	if len(s.Bids) > 0 {
		s.BestBid, s.HasBid = s.Bids[0].Price, true
	}
	return s
}