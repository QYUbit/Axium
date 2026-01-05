package main

import (
	"context"
	"crypto/tls"
	"log"
	"time"

	s2 "github.com/QYUbit/Axium/pkg/server_v2"
	"github.com/quic-go/quic-go"
)

func main() {
	ctx := context.Background()

	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"axium-quic"},
		ServerName:         "localhost",
	}

	conn, err := quic.DialAddr(
		ctx,
		"localhost:8080",
		tlsConf,
		&quic.Config{},
	)
	if err != nil {
		log.Fatal("dial failed:", err)
	}
	defer conn.CloseWithError(0, "bye")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		log.Fatal("stream failed:", err)
	}

	proto := &s2.DefaultProtocol{}

	// Reader
	go func() {
		for {
			var msg s2.Message
			if err := proto.ReadMessage(&msg, stream); err != nil {
				log.Println("read error:", err)
				return
			}

			log.Printf(
				"RECV | room=%v roomName=%s path=%s data=%s\n",
				msg.IsRoom,
				msg.Room,
				msg.Path,
				string(msg.Data),
			)
		}
	}()

	// CREATE
	err = proto.WriteMessage(stream, s2.Message{
		IsRoom: false,
		Path:   "create",
		Data:   []byte("create room"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// JOIN
	err = proto.WriteMessage(stream, s2.Message{
		IsRoom: true,
		Room:   "chat",
		Path:   "join",
		Data:   []byte("join"),
	})
	if err != nil {
		log.Fatal(err)
	}

	// MESSAGE
	err = proto.WriteMessage(stream, s2.Message{
		IsRoom: true,
		Room:   "chat",
		Path:   "message",
		Data:   []byte("Hello QUIC"),
	})
	if err != nil {
		log.Fatal(err)
	}

	time.Sleep(time.Second * 10)
	log.Println("Exiting")
}
