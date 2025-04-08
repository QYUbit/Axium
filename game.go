package axium

import (
	"context"
	"fmt"
	"io"
	"time"

	bufti "github.com/QYUbit/Bufti/go"
	"github.com/quic-go/quic-go"
)

var PositionUpdateModel = bufti.NewModel("position_update",
	bufti.NewField(0, "entity", bufti.StringType),
	bufti.NewField(1, "pos_x", bufti.Float64Type),
	bufti.NewField(2, "pos_y", bufti.Float64Type),
)

type Config struct {
	tickInterval time.Duration
	addres       string
}

type Game struct {
	tickInterval  time.Duration
	listener      *quic.Listener
	stopListening bool
	onTick        func()
}

func NewGame(config *Config) (*Game, error) {
	game := &Game{}

	game.tickInterval = config.tickInterval

	listener, err := quic.ListenAddr(config.addres, generateTLSConfig(), nil)
	if err != nil {
		return nil, err
	}
	game.listener = listener
	go game.listen()

	go game.tickLoop()

	return game, nil
}

func (g *Game) listen() {
	for {
		if g.stopListening {
			break
		}
		conn, err := g.listener.Accept(context.Background())
		if err != nil {
			fmt.Printf("failed accepting connection: %s}\n", err)
		}
		go g.handleConnection(conn)
	}
}

func (g *Game) handleConnection(conn quic.Connection) {
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		fmt.Printf("failed accepting stream: %s}\n", err)
		return
	}
	defer stream.Close()

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		fmt.Printf("failed reading stream: %s}\n", err)
		return
	}
	fmt.Printf("received message: %s\n", string(buf[:n]))
}

func (g *Game) tickLoop() {
	for {
		g.onTick()
		time.Sleep(g.tickInterval)
	}
}

func (g *Game) Close() {
	g.listener.Close()
	g.stopListening = true
}

func (g *Game) OnTick(fn func()) {
	g.onTick = fn
}

func (g *Game) OnClientMessage(fn func()) {

}
