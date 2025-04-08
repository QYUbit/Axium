package axium

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/quic-go/quic-go"
)

type ClientHub interface {
	RegisterClient(clientId string, conn any) error
	UnregisterClient(clientId string) error
	GetClients() []string
	SendTo(clientId string, p []byte)
}

type PubSubHub interface {
	ClientHub
	Publish(roomId string, p []byte) error
	Subscribe(clientId string, roomId string) error
	Unsubscribe(clientId string, roomId string) error
	CreateRoom(roomId string) error
	CloseRoom(roomId string) error
	GetRooms() []string
	GetClientsOfRoom(roomId string) []string
}

var (
	ErrClientNotFound = errors.New("can not find client")
	ErrRoomNotFound   = errors.New("can not find room")
)

type Hub struct {
	listener *quic.Listener
	clients  map[string]quic.Stream
	rooms    map[string]map[string]bool
	mu       sync.RWMutex
}

func NewHub(address string, tlsConf *tls.Config, config *quic.Config) (*Hub, error) {
	h := &Hub{
		clients: make(map[string]quic.Stream),
		rooms:   make(map[string]map[string]bool),
	}

	listener, err := quic.ListenAddr(address, tlsConf, config)
	if err != nil {
		return nil, err
	}
	h.listener = listener

	go func() {
		for {
			conn, err := listener.Accept(context.Background())
			if err != nil {
				fmt.Printf("failed accepting connection: %s\n", err)
				continue
			}
			go handleConnection(conn)
		}
	}()

	return h, nil
}

func (h *Hub) handleConnection(conn quic.Connection) {
	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		fmt.Printf("failed accepting stream: %s\n", err)
		return
	}
	defer stream.Close()

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil && err != io.EOF {
		fmt.Printf("failed reading stream: %s\n", err)
		return
	}
	fmt.Printf("received message: %s\n", string(buf[:n]))
}

func (h *Hub) getStream(clientId string) (quic.Stream, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	stream, exists := h.clients[clientId]
	if !exists {
		return nil, fmt.Errorf("%w %s", ErrClientNotFound, clientId)
	}
	return stream, nil
}

func (h *Hub) getRoom(roomId string) (map[string]bool, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	room, exists := h.rooms[roomId]
	if !exists {
		return nil, fmt.Errorf("%w %s", ErrRoomNotFound, roomId)
	}
	return room, nil
}

func (h *Hub) clientExists(clientId string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.clients[clientId]
	return exists
}

func (h *Hub) roomExists(roomId string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.rooms[roomId]
	return exists
}

func (h *Hub) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.listener.Close()
}

func (h *Hub) RegisterClient(clientId string, conn quic.Stream) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[clientId] = conn
}

func (h *Hub) UnregisterClient(clientId string) error {
	if !h.clientExists(clientId) {
		return fmt.Errorf("%w %s", ErrClientNotFound, clientId)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, clientId)
	return nil
}

func (h *Hub) CloseClient(clientId string) error {
	stream, err := h.getStream(clientId)
	if err != nil {
		return err
	}
	return stream.Close()
}

func (h *Hub) GetClients() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var clients []string
	for clientId := range h.clients {
		clients = append(clients, clientId)
	}
	return clients
}

func (h *Hub) SendTo(clientId string, p []byte) error {
	stream, err := h.getStream(clientId)
	if err != nil {
		return err
	}
	_, err = stream.Write(p)
	return err
}

func (h *Hub) CreateRoom(roomId string) error {
	if h.roomExists(roomId) {
		return fmt.Errorf("%w %s", ErrRoomNotFound, roomId)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rooms[roomId] = make(map[string]bool)
	return nil
}

func (h *Hub) CloseRoom(roomId string) error {
	if h.roomExists(roomId) {
		return fmt.Errorf("%w %s", ErrRoomNotFound, roomId)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, roomId)
	return nil
}

func (h *Hub) GetRooms() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var rooms []string
	for roomId := range h.rooms {
		rooms = append(rooms, roomId)
	}
	return rooms
}

func (h *Hub) Subscribe(clientId string, roomId string) error {
	if !h.clientExists(clientId) {
		return fmt.Errorf("%w %s", ErrClientNotFound, clientId)
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.roomExists(roomId) {
		h.rooms[roomId][clientId] = true
	} else {
		h.rooms[roomId] = map[string]bool{clientId: true}
	}
	return nil
}

func (h *Hub) Unsubscribe(clientId string, roomId string) error {
	room, err := h.getRoom(roomId)
	if err != nil {
		return err
	}
	if !h.clientExists(clientId) {
		return fmt.Errorf("%w %s", ErrClientNotFound, clientId)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(room, clientId)
	return nil
}

func (h *Hub) Publish(roomId string, p []byte) error {
	room, err := h.getRoom(roomId)
	if err != nil {
		return err
	}
	for clientId := range room {
		client, err := h.getStream(clientId)
		if err != nil {
			return err
		}
		if _, err := client.Write(p); err != nil {
			return err
		}
	}
	return nil
}

func (h *Hub) GetRoomClients(roomId string) ([]string, error) {
	room, err := h.getRoom(roomId)
	if err != nil {
		return nil, err
	}
	var clients []string
	for clientId := range room {
		clients = append(clients, clientId)
	}
	return clients, nil
}
