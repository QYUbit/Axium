package s2

import (
	"context"
	"strings"
)

type IncomingMessage struct {
	Sender *Session
	Path   string
	Data   []byte
}

type MessageHandler func(ctx context.Context, msg IncomingMessage)

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
	sep      string
	root     *routerNode
	fallback []MessageHandler
}

func NewRouter() *Router {
	return &Router{
		sep:  ".",
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

func (r *Router) HandleFallback(handler MessageHandler) {
	r.fallback = append(r.fallback, handler)
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

	if len(matching) == 0 {
		for _, handler := range r.fallback {
			matching = append(matching, handler)
		}
	}

	return matching
}

func (r *Router) trigger(ctx context.Context, msg IncomingMessage) {
	matching := r.matching(msg.Path)
	for _, handler := range matching {
		handler(ctx, msg)
	}
}
