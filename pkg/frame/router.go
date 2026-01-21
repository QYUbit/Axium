package frame

import (
	"context"
	"reflect"
	"strings"
	"sync"
)

var (
	typeOfBytes  = reflect.TypeFor[[]byte]()
	typeOfString = reflect.TypeFor[string]()
)

type HandlerFunc[T any] func(c *Context, payload T) error

type compiledHandler func(ctx context.Context, ses *Session, msg Message) error

type Router struct {
	serializer Serializer
	tree       *routerTree

	contextPool sync.Pool
}

func NewRouter() *Router {
	return &Router{
		tree: newRouterTree(),
		contextPool: sync.Pool{
			New: func() any {
				return new(Context)
			},
		},
	}
}

func (r *Router) RegisterRaw(route string, handler HandlerFunc[[]byte]) {
	fn := func(ctx context.Context, ses *Session, msg Message) error {
		c := r.contextPool.Get().(*Context)
		c.Path = msg.Path
		c.ReqID = msg.ReqID
		c.Session = ses
		c.serializer = r.serializer

		defer func() {
			c.reset()
			r.contextPool.Put(c)
		}()

		return handler(c, msg.Data)
	}

	r.tree.insert(route, fn)
}

func Register[T any](r *Router, route string, handler HandlerFunc[T]) {
	t := reflect.TypeFor[T]()

	isBytes := t == typeOfBytes
	isString := t == typeOfString

	fn := func(ctx context.Context, ses *Session, msg Message) error {
		c := r.contextPool.Get().(*Context)
		c.Path = msg.Path
		c.ReqID = msg.ReqID
		c.Session = ses
		c.serializer = r.serializer

		defer func() {
			c.reset()
			r.contextPool.Put(c)
		}()

		// TODO Determine payload extraction method at registration time

		var payload T

		if isBytes {
			payload = any(msg.Data).(T) // Boxing, maybe use unsafe in the future

		} else if isString {
			payload = any(string(msg.Data)).(T) // String alloc, maybe use unsafe in the future

		} else {
			if err := r.serializer.Unmarshal(&payload, msg.Data); err != nil {
				return err
			}
		}

		return handler(c, payload)
	}

	r.tree.insert(route, fn)
}

func (r *Router) Handle(ctx context.Context, ses *Session, msg Message) {
	handlers := r.tree.match(msg.Path)

	for _, h := range handlers {
		if err := h(ctx, ses, msg); err != nil {
			// TODO Log
		}
	}
}

type routerNode struct {
	children  map[string]*routerNode
	wildcards []compiledHandler
	handlers  []compiledHandler
}

func newRouterNode() *routerNode {
	return &routerNode{
		children: make(map[string]*routerNode),
	}
}

type routerTree struct {
	sep      string
	root     *routerNode
	fallback []compiledHandler
}

func newRouterTree() *routerTree {
	return &routerTree{
		sep:  ".",
		root: newRouterNode(),
	}
}

func (t *routerTree) insert(route string, handler compiledHandler) {
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

func (t *routerTree) match(route string) []compiledHandler {
	// TODO Validate route

	parts := strings.Split(route, t.sep)

	node := t.root

	var matching []compiledHandler

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
