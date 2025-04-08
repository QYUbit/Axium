package axium

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"

	"github.com/quic-go/quic-go"
)

//var config = &quic.Config{
//	MaxIdleTimeout:  60 * time.Second,
//	KeepAlivePeriod: 20 * time.Second,
//}

func main() {
	listener, err := quic.ListenAddr("localhost:4242", generateTLSConfig(), nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("QUIC server is listening on localhost:4242")

	for {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			log.Println(err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn quic.Connection) {
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		log.Println(err)
		return
	}
	defer stream.Close()

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		log.Println(err)
		return
	}
	fmt.Printf("Received message: %s\n", string(buf[:n]))

	// Send a response back to the client
	_, _ = stream.Write([]byte("Message received!"))
}

func generateTLSConfig() *tls.Config {
	// Minimal TLS configuration for QUIC
	cert, _ := tls.X509KeyPair(serverCert, serverKey)
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"quic-echo-example"},
	}
}

// Example certificates for testing (not secure for production use)
var serverCert = []byte(`-----BEGIN CERTIFICATE-----
MIIB... (dummy certificate data)
-----END CERTIFICATE-----`)
var serverKey = []byte(`-----BEGIN PRIVATE KEY-----
MIIE... (dummy key data)
-----END PRIVATE KEY-----`)
