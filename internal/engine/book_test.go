package engine

import "testing"

func mk(id uint64, s Side, t OrderType, p int32, q uint64) *Order {
	return &Order{ID: id, Side: s, Type: t, Price: p, Quantity: q}
}

func TestSimpleCross(t *testing.T) {
	b := NewBook(9900, 10100)
	b.Submit(mk(1, Sell, Limit, 10000, 10))
	b.Submit(mk(2, Buy, Limit, 10000, 10))
	if len(b.trades) != 1 {
		t.Fatal("cross")
	}
}

func TestPartialFill(t *testing.T) {
	b := NewBook(9900, 10100)
	b.Submit(mk(1, Sell, Limit, 10000, 8))
	b.Submit(mk(2, Buy, Limit, 10000, 20))
	if len(b.trades) != 1 || b.bids[int(10000-b.minTick)][0].Quantity != 12 {
		t.Fatal("partial")
	}
}

func TestTwoLevels(t *testing.T) {
	b := NewBook(9900, 10100)
	b.Submit(mk(1, Sell, Limit, 10000, 3))
	b.Submit(mk(2, Sell, Limit, 10005, 3))
	b.Submit(mk(3, Buy, Limit, 10005, 5))
	if len(b.trades) != 2 {
		t.Fatal("2levels")
	}
}

func TestMarket(t *testing.T) {
	b := NewBook(9900, 10100)
	b.Submit(mk(1, Sell, Limit, 10000, 3))
	b.Submit(mk(2, Buy, Market, 0, 10))
	if len(b.trades) != 1 || b.bestBidIdx != -1 {
		t.Fatal("market")
	}
}