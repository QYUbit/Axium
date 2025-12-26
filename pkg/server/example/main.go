package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/QYUbit/Axium/pkg/server"
	"github.com/QYUbit/Axium/pkg/transport/quic"
)

func main() {
	transport := quic.NewQuicTransport("0.0.0.0:8080", nil, nil)

	r := server.NewRouter()
	r.Handle("*", func(s *server.Session, msg server.IncomingMessage) {
		log.Printf("Message by %s on %s: %s", s.ID(), msg.Path, msg.Payload)
	})

	s := server.NewServer(transport)
	s.SetRouter(r)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go func() {
		log.Println("Starting server")
		if err := s.Start(ctx); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutdown initiated")

	if err := s.Close(); err != nil {
		log.Printf("Failed to shutdown: %v", err)
	}

	log.Println("Exiting")
}
