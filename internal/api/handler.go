package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"orderbook-hft/internal/engine"
)

// Handler encapsule un Book thread-safe pour le servir via HTTP.
// Le mutex protège le Book : un seul goroutine modifie le carnet à la fois.
// C'est intentionnel pour cette Version A (baseline REST) — la Version B gRPC
// comparera l'impact de ce verrou vs streaming sans contention.
type Handler struct {
	mu   sync.Mutex
	book *engine.Book
}

// NewHandler crée un Handler avec un Book neuf sur la plage de ticks [minTick, maxTick].
func NewHandler(minTick, maxTick int32) *Handler {
	return &Handler{book: engine.NewBook(minTick, maxTick)}
}

// Register enregistre les routes sur le mux fourni.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /order", h.submitOrder)
	mux.HandleFunc("GET /trades", h.getTrades)
	mux.HandleFunc("GET /health", h.health)
}

// POST /order — soumet un ordre, répond avec les trades produits.
func (h *Handler) submitOrder(w http.ResponseWriter, r *http.Request) {
	var req OrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "corps JSON invalide : "+err.Error(), http.StatusBadRequest)
		return
	}

	o, err := toEngineOrder(req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// snapshot du compteur de trades avant soumission pour n'extraire que les nouveaux
	h.mu.Lock()
	before := len(h.book.Trades())
	h.book.Submit(&o)
	after := h.book.Trades()
	h.mu.Unlock()

	resp := SubmitResponse{Trades: make([]TradeResponse, 0, len(after)-before)}
	for _, t := range after[before:] {
		resp.Trades = append(resp.Trades, TradeResponse{
			BuyOrderID:  t.BuyOrderID,
			SellOrderID: t.SellOrderID,
			Price:       t.Price,
			Quantity:    t.Quantity,
		})
	}
	jsonOK(w, resp)
}

// GET /trades — retourne tous les trades depuis le démarrage.
func (h *Handler) getTrades(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	trades := h.book.Trades()
	resp := make([]TradeResponse, len(trades))
	for i, t := range trades {
		resp[i] = TradeResponse{
			BuyOrderID:  t.BuyOrderID,
			SellOrderID: t.SellOrderID,
			Price:       t.Price,
			Quantity:    t.Quantity,
		}
	}
	h.mu.Unlock()
	jsonOK(w, resp)
}

// GET /health — liveness probe.
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toEngineOrder(req OrderRequest) (engine.Order, error) {
	var side engine.Side
	switch req.Side {
	case "buy":
		side = engine.Buy
	case "sell":
		side = engine.Sell
	default:
		return engine.Order{}, &validationError{"side doit être \"buy\" ou \"sell\""}
	}

	var typ engine.OrderType
	switch req.Type {
	case "limit":
		typ = engine.Limit
	case "market":
		typ = engine.Market
	default:
		return engine.Order{}, &validationError{"type doit être \"limit\" ou \"market\""}
	}

	return engine.Order{
		ID:       req.ID,
		Side:     side,
		Type:     typ,
		Price:    req.Price,
		Quantity: req.Quantity,
	}, nil
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}