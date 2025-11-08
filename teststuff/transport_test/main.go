package main

import (
	"crypto/tls"
	"log"

	"github.com/QYUbit/Axium/axium"
	quic "github.com/QYUbit/Axium/quic-transport"
	"github.com/google/uuid"
)

func main() {
	transport, err := quic.NewQuicTransport("localhost:8080", &tls.Config{InsecureSkipVerify: true}, nil)
	if err != nil {
		log.Fatal(err)
	}

	transport.OnConnect(func(ac axium.AxiumConnection, f1, f2 func(string)) {
		log.Printf("New client connected: %s\n", ac.GetRemoteAddress())
		f1(uuid.New().String())
	})

	transport.OnDisconnect(func(s string) {
		log.Panicf("Client disconnected: %s\n", s)
	})

	transport.OnMessage(func(s string, b []byte) {
		log.Panicf("Message from %s: %s\n", s, b)
	})

	transport.OnError(func(err error) {
		log.Fatal(err)
	})
}
