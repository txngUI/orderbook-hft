package engine

// Book = le carnet d'ordres.
type Book struct {
	// naïf 
	// bids   map[int64][]*Order 
	bids   map[int64][]Order // acheteurs : on cherchera le prix le PLUS HAUT
	// naïf
	// asks   map[int64][]*Order 
	asks   map[int64][]Order // vendeurs : on cherchera le prix le PLUS BAS
	trades []Trade              // l'historique des transactions produites
}

func (b *Book) Trades() []Trade { return b.trades }


// NewBook crée un carnet vide et prêt à l'emploi.
func NewBook() *Book {
	return &Book{
		// naïf : on stocke des pointeurs vers les ordres	
		// bids: make(map[int64][]*Order),
		// asks: make(map[int64][]*Order),

		bids: make(map[int64][]Order),
		asks: make(map[int64][]Order),
	}
}

/**
* bestAsk renvoie le meilleur (plus bas) prix vendeur, et un booléen
* qui dit s'il en existe au moins un.
* @param b : le carnet d'ordres
* @return (best, found) : le meilleur prix et un booléen qui dit s'il en existe au moins un
**/
func (b *Book) bestAsk() (int64, bool) {
	var best int64
	found := false

	// NAÏF : on parcourt TOUTE la map à chaque appel → O(n).
	// C'est le "Table Scan" du cours. Ce sera LE goulot que le profiling révélera.
	for price, level := range b.asks {
		if len(level) == 0 {
			continue // niveau vide, on ignore
		}
		if !found || price < best {
			best = price
			found = true
		}
	}
	return best, found
}

/**
* bestBid renvoie le meilleur (plus haut) prix acheteur, et un booléen
* qui dit s'il en existe au moins un.
* @param b : le carnet d'ordres
* @return (best, found) : le meilleur prix et un booléen qui dit s'il en existe au moins un
**/
func (b *Book) bestBid() (int64, bool) {
	var best int64
	found := false

	// NAÏF : on parcourt TOUTE la map à chaque appel → O(n).
	// C'est le "Table Scan" du cours. Ce sera LE goulot que le profiling révélera.
	for price, level := range b.bids {
		if len(level) == 0 {
			continue // niveau vide, on ignore
		}
		if !found || price > best {
			best = price
			found = true
		}
	}
	return best, found
}

/**
* Submit ingère un ordre entrant. C'est le futur Hot Path.
* @param b : le carnet d'ordres
* @param o : l'ordre entrant
**/
func (b *Book) Submit(o *Order) {
	switch o.Side {
	case Buy:
		b.matchBuy(o)
	case Sell:
		b.matchSell(o)
	}
}

/**
* matchBuy tente d'exécuter un ordre acheteur contre les ordres vendeurs.
* @param b : le carnet d'ordres
* @param o : l'ordre acheteur entrant
**/
func (b *Book) matchBuy(o *Order) {
	// tant que l'acheteur a encore des unités à acheter
	for o.Quantity > 0 {
		askPrice, ok := b.bestAsk()

		if !ok {
			break // plus aucun vendeur → on sort
		}

		// Un LIMIT n'achète pas au-dessus de son prix. Un MARKET, lui, accepte tout.
		if o.Type == Limit && askPrice > o.Price {
			break // le meilleur vendeur est trop cher → on sort
		}

		level := b.asks[askPrice] // la file d'ordres à ce prix
		// resting := level[0]       // la version naïve : on copie le 1er élément de la file pour le stocker dans une variable
		resting := &level[0]       // on prend un pointeur vers le 1er élément de la file

		// qty := min(o.Quantity, resting.Quantity) <-- naïf 
		qty := min(o.Quantity, resting.Quantity) // on échange le plus petit des deux

		// on enregistre la transaction
		b.trades = append(b.trades, Trade{
			BuyOrderID:  o.ID,
			SellOrderID: resting.ID,
			Price:       askPrice,
			Quantity:    qty,
		})

		// on met à jour les deux quantités
		o.Quantity -= qty
		resting.Quantity -= qty

		// si le vendeur est complètement vidé, on le retire de la file
		if level[0].Quantity == 0 {
			b.asks[askPrice] = level[1:] // on enlève le 1er élément
			if len(b.asks[askPrice]) == 0 {
				delete(b.asks, askPrice) // niveau vide → on supprime la clé
			}
		}
	}

	// SORTIE DE BOUCLE : s'il reste des unités ET que c'est un LIMIT → en attente
	if o.Quantity > 0 && o.Type == Limit {
		// b.bids[o.Price] = append(b.bids[o.Price], o) <-- version naïve
		b.bids[o.Price] = append(b.bids[o.Price], *o) // on stocke une copie de l'ordre
	}
}

/**
* matchSell tente d'exécuter un ordre vendeur contre les ordres acheteurs.
* @param b : le carnet d'ordres
* @param o : l'ordre vendeur entrant
**/
func (b *Book) matchSell(o *Order) {
	// tant que le vendeur a encore des unités à vendre
	for o.Quantity > 0 {
		bidPrice, ok := b.bestBid()

		if !ok {
			break // plus aucun acheteur → on sort
		}

		// Un LIMIT ne vend pas en dessous de son prix. Un MARKET, lui, accepte tout.
		if o.Type == Limit && bidPrice < o.Price {
			break // le meilleur acheteur est trop bas → on sort
		}
		
		level := b.bids[bidPrice]
		// resting := level[0] <-- version naïve : on copie le 1er élément de la file pour le stocker dans une variable
		resting := &level[0] // on prend un pointeur vers le 1er élément de la file

		qty := min(o.Quantity, resting.Quantity)

		b.trades = append(b.trades, Trade{
			BuyOrderID:  resting.ID,
			SellOrderID: o.ID,
			Price:       bidPrice,
			Quantity:    qty,
		})

		o.Quantity -= qty
		resting.Quantity -= qty

		// si l'acheteur est complètement vidé, on le retire de la file
		if level[0].Quantity == 0 {
			b.bids[bidPrice] = level[1:] // on enlève le 1er élément
			if len(b.bids[bidPrice]) == 0 {
				delete(b.bids, bidPrice) // niveau vide → on supprime la clé
			}
		}
	}

	// SORTIE DE BOUCLE : reliquat + LIMIT → on met en attente
	if o.Quantity > 0 && o.Type == Limit {
		// b.asks[o.Price] = append(b.asks[o.Price], o) <-- version naïve
		b.asks[o.Price] = append(b.asks[o.Price], *o) // on stocke une copie de l'ordre
	}
}