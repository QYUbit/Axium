package server

import (
	"encoding/json"
	"strings"

	"github.com/QYUbit/Axium/pkg/transport"
)

type MessageHandler func(s *Session, msg IncomingMessage)

// ==================================================================
// Protocol
// ==================================================================

type MessageProtocol interface {
	Decode(dest *IncomingMessage, src transport.Message) error
	Encode(dest []byte, src OutgoingMessage) error
}

type IncomingMessage struct {
	Sender    string
	Path      string
	Target    string
	HasTarget bool
	Payload   []byte
}

type OutgoingMessage struct {
	Path     string
	Room     string
	FromRoom bool
	Payload  []byte
}

type JSONProtocol struct{}

func (JSONProtocol) Decode(dest *IncomingMessage, src transport.Message) error {
	var envelope struct {
		Path      string          `json:"path"`
		Target    string          `json:"target"`
		HasTarget bool            `json:"has_target"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(src.Data, &envelope); err != nil {
		return err
	}

	dest.Path = envelope.Path
	dest.Target = envelope.Target
	dest.HasTarget = envelope.HasTarget
	dest.Payload = envelope.Data
	dest.Sender = src.ClientId

	return nil
}

func (JSONProtocol) Encode(dest []byte, src OutgoingMessage) error {
	var envelope struct {
		Path     string          `json:"path"`
		Room     string          `json:"room"`
		FromRoom bool            `json:"from_room"`
		Data     json.RawMessage `json:"data"`
	}

	envelope.Path = src.Path
	envelope.Room = src.Room
	envelope.FromRoom = src.FromRoom
	envelope.Data = src.Payload

	out, err := json.Marshal(envelope)
	if err != nil {
		return err
	}

	copy(dest, out)

	return nil
}

// ==================================================================
// Router
// ==================================================================

type routerNode struct {
	children  map[string]*routerNode
	handlers  []MessageHandler
	wildcards []MessageHandler
}

func newRouterNode() *routerNode {
	return &routerNode{children: make(map[string]*routerNode)}
}

type Router struct {
	sep  string
	root *routerNode
}

func NewRouter() *Router {
	return &Router{
		root: newRouterNode(),
	}
}

func NewRouterWithSep(sep string) *Router {
	return &Router{
		sep:  sep,
		root: newRouterNode(),
	}
}

func (r *Router) Handle(route string, handler MessageHandler) {
	if route == "" {
		return
	}
	if strings.HasPrefix(route, r.sep) {
		return
	}
	if strings.HasPrefix(route, r.sep) {
		return
	}

	parts := strings.Split(route, r.sep)

	n := len(parts)
	node := r.root

	for i := range parts {
		part := parts[i]

		if part == "*" && i == n-1 {
			return
		}

		if part == "*" {
			node.wildcards = append(node.wildcards, handler)
			return
		}

		next, ok := node.children[part]
		if !ok {
			newNode := newRouterNode()
			node.children[part] = newNode
			next = newNode
		}

		node = next
	}

	node.handlers = append(node.handlers, handler)
}

func (r *Router) matching(path string) []MessageHandler {
	if path == "" {
		return nil
	}
	if strings.HasPrefix(path, r.sep) {
		return nil
	}
	if strings.HasPrefix(path, r.sep) {
		return nil
	}

	parts := strings.Split(path, r.sep)

	node := r.root

	var matching []MessageHandler

	for i := range parts {
		part := parts[i]

		for _, handler := range node.wildcards {
			matching = append(matching, handler)
		}

		next, ok := node.children[part]
		if !ok {
			return matching
		}
		node = next
	}

	for _, handler := range node.handlers {
		matching = append(matching, handler)
	}

	return matching
}

func (r *Router) trigger(s *Session, msg IncomingMessage) {
	matching := r.matching(msg.Path)
	for _, handler := range matching {
		handler(s, msg)
	}
}
