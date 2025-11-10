package main

import (
	"context"
	"crypto/tls"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/QYUbit/Axium/axium"
	quic "github.com/QYUbit/Axium/quic-transport"
	"github.com/google/uuid"
)

func main() {
	transport, err := quic.NewQuicTransport("localhost:8080", &tls.Config{
		InsecureSkipVerify: true,
	}, nil)
	if err != nil {
		log.Fatal(err)
	}

	transport.OnConnect(func(ac axium.AxiumConnection, f1, f2 func(string)) {
		log.Printf("New client connected: %s\n", ac.GetRemoteAddress())
		f1(uuid.New().String())
	})

	transport.OnDisconnect(func(s string) {
		log.Printf("Client disconnected: %s\n", s)
	})

	transport.OnMessage(func(s string, b []byte) {
		log.Printf("Message from %s: %s\n", s, string(b))
	})

	transport.OnError(func(err error) {
		log.Printf("Transport error: %v\n", err)
	})

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v — shutting down gracefully...", sig)
		cancel()

		if err := transport.Close(); err != nil {
			log.Printf("Error during shutdown: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Server stopped cleanly.")
}
