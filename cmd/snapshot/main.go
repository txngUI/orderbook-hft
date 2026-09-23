package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"strings"
	"time"

	"orderbook-hft/internal/engine"
	"orderbook-hft/internal/feed"
)

//go:embed snapshot.tmpl.html
var tmplSrc string

type row struct {
	Price string
	Size  uint64
	Width int
	Best  bool
}
type tradeRow struct {
	Ref, Price string
	Size       uint64
	Up         bool
}
type pageData struct {
	Last, Spread   string
	Trades, Volume string
	Asks, Bids     []row
	Tape           []tradeRow
	Generated      string
}

func eur(cents int64) string {
	return strings.Replace(fmt.Sprintf("%.2f", float64(cents)/100), ".", ",", 1)
}
func group(n uint64) string {
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func main() {
	const n = 200_000
	orders := feed.Generate(n, 42)
	book := engine.NewBook(9900, 10100)
	for i := 0; i < n; i++ {
		o := orders[i]
		book.Submit(&o)
	}

	snap := book.Snapshot(10)
	trades := book.Trades()

	var maxSz uint64 = 1
	for _, l := range append(append([]engine.Level{}, snap.Asks...), snap.Bids...) {
		if l.Size > maxSz {
			maxSz = l.Size
		}
	}
	mkRow := func(l engine.Level, best bool) row {
		return row{Price: eur(l.Price), Size: l.Size, Width: int(l.Size * 100 / maxSz), Best: best}
	}

	var asks []row // du plus HAUT au meilleur (en bas, près du spread)
	for i := len(snap.Asks) - 1; i >= 0; i-- {
		asks = append(asks, mkRow(snap.Asks[i], i == 0))
	}
	var bids []row // meilleur (plus haut) en premier
	for i := range snap.Bids {
		bids = append(bids, mkRow(snap.Bids[i], i == 0))
	}

	const tapeN = 16
	var tape []tradeRow
	var totalVol uint64
	for _, t := range trades {
		totalVol += t.Quantity
	}
	for i := len(trades) - 1; i >= 0 && len(tape) < tapeN; i-- {
		up := i == 0 || trades[i].Price >= trades[i-1].Price
		tape = append(tape, tradeRow{
			Ref:   fmt.Sprintf("#%d", i+1),
			Price: eur(trades[i].Price),
			Size:  trades[i].Quantity,
			Up:    up,
		})
	}

	last, spread := "—", "—"
	if len(trades) > 0 {
		last = eur(trades[len(trades)-1].Price)
	}
	if snap.HasBid && snap.HasAsk {
		spread = eur(snap.BestAsk - snap.BestBid)
	}

	data := pageData{
		Last: last, Spread: spread,
		Trades: group(uint64(len(trades))), Volume: group(totalVol),
		Asks: asks, Bids: bids, Tape: tape,
		Generated: time.Now().Format("2006-01-02 15:04"),
	}

	tmpl := template.Must(template.New("snap").Parse(tmplSrc))
	f, err := os.Create("snapshot.html")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := tmpl.Execute(f, data); err != nil {
		panic(err)
	}
	fmt.Printf("snapshot.html écrit — %s trades, bid %s € / ask %s €\n",
		group(uint64(len(trades))), eur(snap.BestBid), eur(snap.BestAsk))
}