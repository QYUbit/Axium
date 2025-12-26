package main

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"

	"github.com/QYUbit/Axium/pkg/ecs"
	"github.com/QYUbit/Axium/pkg/server"
	"github.com/QYUbit/Axium/pkg/transport/webtransport"
	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
)

type NetworkID struct{ ClientId string }

func (NetworkID) Id() ecs.ComponentID { return 0 } // TODO Id allocation at engine level & Id tabel

type Position struct{ X, Y float64 }

func (Position) Id() ecs.ComponentID { return 1 }

type Velocity struct{ X, Y float64 }

func (Velocity) Id() ecs.ComponentID { return 2 }

type PlayerJoinedMessage struct {
	ClientID string
	EntityID ecs.EntityID
}

func (PlayerJoinedMessage) Id() ecs.ComponentID { return 3 }

type PlayerLeftMessage struct {
	EntityID ecs.EntityID
}

func (PlayerLeftMessage) Id() ecs.ComponentID { return 4 }

type MoveInput struct {
	EntityID ecs.EntityID
	Up       bool `json:"up"`
	Down     bool `json:"down"`
	Right    bool `json:"right"`
	Left     bool `json:"left"`
}

func (MoveInput) Id() ecs.ComponentID { return 5 }

func PlayerManagementSystem(ctx ecs.SystemContext) {
	joined := ecs.CollectMessages[PlayerJoinedMessage](ctx.World)
	for _, msg := range joined {
		ctx.Commands.CreateEntity(msg.EntityID)
		ecs.AddComponent(ctx.Commands, msg.EntityID, Position{100, 100})
		ecs.AddComponent(ctx.Commands, msg.EntityID, Velocity{})
		ecs.AddComponent(ctx.Commands, msg.EntityID, NetworkID{msg.ClientID})
	}

	left := ecs.CollectMessages[PlayerLeftMessage](ctx.World)
	for _, msg := range left {
		ctx.Commands.DestroyEntity(msg.EntityID)
	}
}

func InputSystem(ctx ecs.SystemContext) {
	inputs := ecs.CollectMessages[MoveInput](ctx.World)

	for _, msg := range inputs {
		for row := range ecs.Query1[Velocity](ecs.With(Velocity{}, NetworkID{})) {
			if row.ID == msg.EntityID {
				vel := row.GetMutable()

				if msg.Up {
					vel.Y += 5
				}
				if msg.Down {
					vel.Y -= 5
				}
				if msg.Left {
					vel.X += 5
				}
				if msg.Right {
					vel.X -= 5
				}
			}
		}
	}
}

func MovementSystem(ctx ecs.SystemContext) {
	q := ecs.SimpleQuery2[Position, Velocity](ctx.World)

	for row := range q {
		// Derive dirty components from system scope, not at update time ?
		pos := row.GetMutable1()
		vel := row.Get2()
		pos.X += vel.X
		pos.Y += vel.Y
	}
}

func GamePlugin(engine *ecs.ECSEngine) {
	ecs.RegisterComponent[NetworkID](engine)
	ecs.RegisterComponent[Position](engine)
	ecs.RegisterComponent[Velocity](engine)

	ecs.RegisterMessage[PlayerJoinedMessage](engine)
	ecs.RegisterMessage[PlayerLeftMessage](engine)

	engine.RegisterSystem(PlayerManagementSystem)

	engine.RegisterSystem(
		InputSystem,
		ecs.Writes(Velocity{}),
	)

	engine.RegisterSystem(
		MovementSystem,
		ecs.Writes(Position{}, Velocity{}),
		ecs.Writes(Position{}),
	)
}

func GameRoom(r *server.Room) {
	engine := ecs.NewEngine()

	engine.RegisterPlugin(GamePlugin)

	router := server.NewRouter()
	r.SetRouter(router)

	router.Handle("leave", func(s *server.Session, msg server.IncomingMessage) {
		r.Unassign(s)
		send(s, "room_left", []byte(r.ID()))

		if r.MemberCount() == 0 {
			r.Destroy()
		}
	})

	router.Handle("game.move", func(s *server.Session, msg server.IncomingMessage) {
		var input MoveInput

		if err := json.Unmarshal(msg.Payload, &input); err != nil {
			log.Printf("Failed to decode input: %v", err)
			return
		}

		ecs.PushMessageSafe(engine, input)
	})

	r.OnCreate(func() {
		go engine.Run(r.Context(), 60)
	})

	r.OnJoin(func(s *server.Session) {
		player := rand.Uint32()

		ecs.PushMessageSafe(engine, PlayerJoinedMessage{
			ClientID: s.ID(),
			EntityID: ecs.EntityID(player),
		})

		s.Set("player", player)

		send(s, "game.spawn", binary.LittleEndian.AppendUint32(nil, player))
	})

	r.OnLeave(func(s *server.Session) {
		p, ok := s.Get("player")
		if !ok {
			return
		}
		player := p.(uint32)

		ecs.PushMessageSafe(engine, PlayerLeftMessage{
			EntityID: ecs.EntityID(player),
		})
	})

	r.OnDestroy(func() {
		if err := r.Broadcast("game.end", []byte(r.ID())); err != nil {
			log.Printf("Failed to broadcast: %v", err)
		}

		engine.Close()
		engine.Wait()
	})
}

func send(s *server.Session, path string, data []byte) {
	if err := s.Send(path, data); err != nil {
		log.Printf("Failed sending message: %v", err)
	}
}

func main() {
	// A lot of DI
	wtServer := &wt.Server{
		H3: http3.Server{
			Addr: "0.0.0.0:8080",
		},
	}

	t := webtransport.NewWebTransport(wtServer)

	serv := server.NewServer(t)

	serv.RegisterRoom("game", GameRoom)

	r := server.NewRouter()

	serv.SetRouter(r)
	serv.SetProtocol(server.JSONProtocol{})

	r.Handle("create", func(s *server.Session, msg server.IncomingMessage) {
		// TODO Pass ctx to room
		room, err := serv.CreateRoom("game")
		if err != nil {
			log.Printf("Failed creating room: %v", err)
			send(s, "error", []byte("Failed to create room"))
		}

		send(s, "game_created", []byte(room.ID()))
	})

	r.Handle("join", func(s *server.Session, msg server.IncomingMessage) {
		room, ok := serv.GetRoom(string(msg.Payload))
		if !ok {
			send(s, "error.room_404", []byte(room.ID()))
			return
		}

		if room.MemberCount() == 4 {
			send(s, "error.room_full", []byte(room.ID()))
			return
		}

		room.Assign(s)
		send(s, "room_joined", []byte(room.ID()))
	})

	ctx, stop := signal.NotifyContext(
		context.Background(),
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
