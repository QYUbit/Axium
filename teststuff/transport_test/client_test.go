package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func TestClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, err := quic.DialAddr(ctx, "localhost:8080", &tls.Config{InsecureSkipVerify: true}, nil)
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.CloseWithError(0, "bye")

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}
	defer stream.Close()

	message := "Hello from QUIC client!"
	fmt.Printf("Client: Sending '%s'\n", message)
	_, err = stream.Write([]byte(message))
	if err != nil {
		log.Fatalf("Failed to write to stream: %v", err)
	}

	buf, err := io.ReadAll(stream)
	if err != nil {
		log.Fatalf("Failed to read from stream: %v", err)
	}
	fmt.Printf("Client: Received '%s'\n", string(buf))
}
