package frame

import (
	"context"
	"strings"
	"sync"
)

type HandlerFunc[T any] func(c *Context[T]) error

type MessageHandler func(ctx context.Context, ses *Session, msg Message) error

type messageService struct {
	serializer Serializer
	tree       *routerTree

	contextPool sync.Pool
}

func NewMessageService() *messageService {
	return &messageService{
		tree: newRouterTree(),
		contextPool: sync.Pool{
			New: func() any {
				return new(baseContext)
			},
		},
	}
}

func Register[T any](s *messageService, route string, handler HandlerFunc[T]) {
	fn := func(ctx context.Context, ses *Session, msg Message) error {

		base := s.contextPool.Get().(*baseContext)
		base.Path = msg.Path
		base.ReqID = msg.ReqID
		base.Session = ses
		base.serializer = s.serializer

		defer func() {
			base.reset()
			s.contextPool.Put(base)
		}()

		payload := new(T)
		if err := s.serializer.Unmarshal(payload, msg.Data); err != nil {
			return err
		}

		c := &Context[T]{
			baseContext: base,
			Msg:         payload,
		}

		return handler(c)
	}

	s.tree.insert(route, fn)
}

func (s *messageService) Handle(ctx context.Context, ses *Session, msg Message) {
	handlers := s.tree.match(msg.Path)

	for _, h := range handlers {
		if err := h(ctx, ses, msg); err != nil {
			// TODO Log
		}
	}
}

type routerNode struct {
	children  map[string]*routerNode
	wildcards []MessageHandler
	handlers  []MessageHandler
}

func newRouterNode() *routerNode {
	return &routerNode{
		children: make(map[string]*routerNode),
	}
}

type routerTree struct {
	sep      string
	root     *routerNode
	fallback []MessageHandler
}

func newRouterTree() *routerTree {
	return &routerTree{
		sep:  ".",
		root: newRouterNode(),
	}
}

func (t *routerTree) insert(route string, handler MessageHandler) {
	// TODO Validate route

	parts := strings.Split(route, t.sep)

	n := len(parts)
	node := t.root

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

func (t *routerTree) match(route string) []MessageHandler {
	// TODO Validate route

	parts := strings.Split(route, t.sep)

	node := t.root

	var matching []MessageHandler

	foundNode := true

	for i := range parts {
		part := parts[i]

		for _, handler := range node.wildcards {
			matching = append(matching, handler)
		}

		next, ok := node.children[part]
		if !ok {
			foundNode = false
			break
		}
		node = next
	}

	if !foundNode {
		for _, handler := range t.fallback {
			matching = append(matching, handler)
		}
		return matching
	}

	foundHandler := false

	for _, handler := range node.handlers {
		foundHandler = true
		matching = append(matching, handler)
	}

	if !foundHandler {
		for _, handler := range t.fallback {
			matching = append(matching, handler)
		}
	}

	return matching
}
