package main

/*
import (
	"crypto/tls"
	"fmt"
	"log"

	"github.com/QYUbit/Axium/axium"
	serializer "github.com/QYUbit/Axium/default_serializer"
	quic "github.com/QYUbit/Axium/quic-transport"
	"github.com/google/uuid"
)

func generator() string {
	return uuid.New().String()
}

func main() {
	transport, err := quic.NewQuicTransport("localhost:8080", &tls.Config{InsecureSkipVerify: true}, nil)
	if err != nil {
		log.Fatal(err)
	}

	serializer := serializer.NewSerializer()

	server := axium.NewServer(axium.ServerOptions{
		Transport:   transport,
		Serializer:  serializer,
		IdGenerator: generator,
	})

	server.OnConnect(func(session *axium.Session, ip string) (bool, string) {
		if ip == "" {
			return false, "invalid remote address"
		}
		session.Set("ip", ip)
		return true, ""
	})

	server.OnBeforeMessage(func(session *axium.Session, data []byte) bool {
		return len(data) <= 4096
	})

	server.MessageHandler("auth", func(session *axium.Session, data []byte) {
		token := string(data)

		if token == "very_safe_secret" {
			session.Set("authorized", true)
			if err := session.Send([]byte("successfull auth"), true); err != nil {
				fmt.Println("Error:", err)
			}
		}
	})

	server.MessageHandler("create_game", func(session *axium.Session, data []byte) {
		if err := server.CreateRoom("game_room", "game-1234"); err != nil {
			fmt.Println("Error:", err)
			return
		}
		// TODO Cleaner design
		room, exists := server.GetRoom("game-1234")
		if !exists {
			return
		}
		if err := room.Assign(session); err != nil {
			fmt.Println("Error:", err)
			return
		}
		if err := session.Send([]byte("game created: "+"game-1234"), true); err != nil {
			fmt.Println("Error:", err)
		}
	})

	server.DefineRoom("game_room", func(r *axium.Room) {
		r.OnCreate(func() {
			fmt.Println("room created")
		})

		r.OnJoin(func(session *axium.Session) {
			fmt.Println("new room member:", session.Id())
		})

		r.OnLeave(func(session *axium.Session) {
			fmt.Println("room member left:", session.Id())
			if r.MemberCount() == 0 {
				server.DestroyRoom(r.Id())
			}
		})

		r.OnDestroy(func() {
			fmt.Println("room destroyed")
		})

		r.MiddlewareHandler(func(session *axium.Session, data []byte) (pass bool) {
			fmt.Println("some message")
			return true
		})

		r.MessageHandler("greeting", func(session *axium.Session, data []byte) {
			fmt.Printf("greeting message from %s: %s\n", session.Id(), data)
		})

		r.MessageHandler("chat", func(session *axium.Session, data []byte) {
			if err := r.BroadcastExcept(data, true, session.Id()); err != nil {
				fmt.Println(err)
			}
		})

		r.FallbackHandler(func(session *axium.Session, data []byte) {
			fmt.Println("event type not found")
		})
	})
}
*/
// Naming:
// * listener   -> transport -> server
// * connection -> client    -> session
// *               topic     -> room
// * stream     -> message   -> message
