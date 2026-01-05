package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log"
	"math/big"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	s2 "github.com/QYUbit/Axium/pkg/server_v2"
)

func generateId() string {
	return rand.Text()
}

func ChatRoom(r *s2.Room) {
	r.OnCreate(func(ctx context.Context) {
		log.Println("Room opened")
	})

	r.OnJoin(func(s *s2.Session) {
		log.Println("Session joined:", s.ID())
		if err := r.BroadcastExcept("message", []byte("seesion joined: "+s.ID()), []string{s.ID()}); err != nil {
			log.Println("Failed to broadcast:", err)
		}
	})

	r.OnLeave(func(s *s2.Session) {
		log.Println("Session left:", s.ID())
		if err := r.BroadcastExcept("message", []byte("seesion left: "+s.ID()), []string{s.ID()}); err != nil {
			log.Println("Failed to broadcast:", err)
		}
	})

	r.OnDestroy(func() {
		log.Println("Room closed")
	})

	router := s2.NewRouter()

	router.HandleFallback(func(ctx context.Context, msg s2.IncomingMessage) {
		log.Printf("Invalid path %s\n", msg.Path)
	})

	router.Handle("message", func(ctx context.Context, msg s2.IncomingMessage) {
		log.Printf("Message received from %s: %s\n", msg.Sender.ID(), string(msg.Data))
		if err := r.BroadcastExcept("message", msg.Data, []string{msg.Sender.ID()}); err != nil {
			log.Println("Failed to broadcast:", err)
		}
	})

	router.Handle("join", func(ctx context.Context, msg s2.IncomingMessage) {
		log.Printf("Join message received from %s: %s\n", msg.Sender.ID(), string(msg.Data))
		r.Assign(msg.Sender)
	})
}

func main() {
	serv := s2.NewServer(
		":8080",
		selfSignedTLS(),
		nil,
		generateId,
		s2.DefaultProtocol{},
	)

	serv.RegisterRoomFactory("chat", ChatRoom)

	router := s2.NewRouter()

	router.HandleFallback(func(ctx context.Context, msg s2.IncomingMessage) {
		log.Printf("Invalid path %s\n", msg.Path)
	})

	router.Handle("create", func(ctx context.Context, msg s2.IncomingMessage) {
		log.Printf("Create message received from %s: %s\n", msg.Sender.ID(), string(msg.Data))
		room, err := serv.CreateRoom(ctx, "chat")
		if err != nil {
			log.Println("Failed to create room:", err)
			return
		}
		room.Assign(msg.Sender)
	})

	ctx := context.Background()

	ctx, stop := signal.NotifyContext(
		ctx,
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	go func() {
		log.Println("Starting server")
		if err := serv.Start(ctx); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutdown initiated")

	if err := serv.Close(); err != nil {
		log.Printf("Failed to shutdown: %v", err)
	}

	log.Println("Exiting")
}

func selfSignedTLS() *tls.Config {
	priv, _ := rsa.GenerateKey(rand.Reader, 2048)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	der, _ := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)

	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	key := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	tlsCert, _ := tls.X509KeyPair(cert, key)

	return &tls.Config{
		InsecureSkipVerify: true,
		Certificates:       []tls.Certificate{tlsCert},
		NextProtos:         []string{"axium-quic"},
	}
}
