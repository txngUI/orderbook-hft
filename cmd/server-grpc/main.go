package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"

	"orderbook-hft/internal/engine"
	pb "orderbook-hft/internal/pb"
)

// server implémente pb.OrderBookServiceServer.
// Le mutex protège le Book pour permettre plusieurs streams concurrents
// (comme le sync.Mutex du handler REST — baseline honnête pour la comparaison).
type server struct {
	pb.UnimplementedOrderBookServiceServer
	mu   sync.Mutex
	book *engine.Book
}

func newServer(minTick, maxTick int32) *server {
	return &server{book: engine.NewBook(minTick, maxTick)}
}

// Submit — appel unaire, équivalent HTTP POST /order (comparable à REST).
func (s *server) Submit(ctx context.Context, req *pb.OrderRequest) (*pb.SubmitResponse, error) {
	o := toEngineOrder(req)

	s.mu.Lock()
	before := len(s.book.Trades())
	s.book.Submit(&o)
	after := s.book.Trades()
	s.mu.Unlock()

	resp := &pb.SubmitResponse{Trades: make([]*pb.Trade, 0, len(after)-before)}
	for _, t := range after[before:] {
		resp.Trades = append(resp.Trades, &pb.Trade{
			BuyOrderId:  t.BuyOrderID,
			SellOrderId: t.SellOrderID,
			Price:       t.Price,
			Quantity:    t.Quantity,
		})
	}
	return resp, nil
}

// StreamOrders — streaming bidirectionnel : le client envoie un flux d'ordres,
// le serveur renvoie un flux de trades en temps réel. C'est l'atout gRPC
// qu'aucun handler REST ne peut égaler (une seule connexion HTTP/2, zéro HOL).
func (s *server) StreamOrders(stream pb.OrderBookService_StreamOrdersServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		o := toEngineOrder(req)

		s.mu.Lock()
		before := len(s.book.Trades())
		s.book.Submit(&o)
		after := s.book.Trades()
		s.mu.Unlock()

		for _, t := range after[before:] {
			if err := stream.Send(&pb.Trade{
				BuyOrderId:  t.BuyOrderID,
				SellOrderId: t.SellOrderID,
				Price:       t.Price,
				Quantity:    t.Quantity,
			}); err != nil {
				return err
			}
		}
	}
}

// toEngineOrder convertit un OrderRequest Protobuf en engine.Order.
func toEngineOrder(req *pb.OrderRequest) engine.Order {
	var side engine.Side
	if req.Side == pb.Side_SIDE_SELL {
		side = engine.Sell
	} else {
		side = engine.Buy
	}

	var typ engine.OrderType
	if req.Type == pb.OrderType_ORDER_TYPE_MARKET {
		typ = engine.Market
	} else {
		typ = engine.Limit
	}

	return engine.Order{
		ID:       req.Id,
		Side:     side,
		Type:     typ,
		Price:    req.Price,
		Quantity: req.Quantity,
	}
}

func main() {
	addr    := flag.String("addr", ":50051", "adresse d'écoute (ex: :50051)")
	minTick := flag.Int("min", 9900, "prix minimum en ticks")
	maxTick := flag.Int("max", 10100, "prix maximum en ticks")
	flag.Parse()

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("écoute impossible sur %s : %v", *addr, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterOrderBookServiceServer(grpcServer, newServer(int32(*minTick), int32(*maxTick)))

	fmt.Printf("Order Book gRPC server — écoute sur %s\n", *addr)
	fmt.Println("Services :")
	fmt.Println("  Submit(OrderRequest) → SubmitResponse           (unaire)")
	fmt.Println("  StreamOrders(stream OrderRequest) → stream Trade (bidi)")

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serveur arrêté : %v", err)
	}
}