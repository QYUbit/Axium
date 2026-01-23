package server

import (
	"context"
	"reflect"
	"strings"
	"sync"

	"github.com/QYUbit/Axium/pkg/axlog"
)

var (
	typeOfBytes  = reflect.TypeFor[[]byte]()
	typeOfString = reflect.TypeFor[string]()
)

type HandlerFunc[T any] func(c *Context, payload T) error

type compiledHandler func(c *Context, payload []byte) error

type Router struct {
	logger      axlog.Logger
	serializer  Serializer
	tree        *routerTree
	contextPool sync.Pool
}

func NewRouter(serializer Serializer) *Router {
	return &Router{
		tree: newRouterTree(),
		contextPool: sync.Pool{
			New: func() any {
				return new(Context)
			},
		},
		serializer: serializer,
	}
}

func (r *Router) RegisterRaw(route string, handler HandlerFunc[[]byte]) {
	r.tree.insert(route, compiledHandler(handler))
}

func Register[T any](r *Router, route string, handler HandlerFunc[T]) {
	t := reflect.TypeFor[T]()

	isBytes := t == typeOfBytes
	isString := t == typeOfString

	fn := func(c *Context, data []byte) error {
		// TODO Determine payload extraction method at registration time ?

		var payload T

		if isBytes {
			payload = any(data).(T) // Boxing, maybe use unsafe in the future

		} else if isString {
			payload = any(string(data)).(T) // String alloc, maybe use unsafe in the future

		} else {
			if err := r.serializer.Unmarshal(&payload, data); err != nil {
				return err
			}
		}

		return handler(c, payload)
	}

	r.tree.insert(route, fn)
}

func (r *Router) RegisterFallbackRaw(handler HandlerFunc[[]byte]) {
	r.tree.insertFallback(compiledHandler(handler))
}

func (r *Router) Handle(ctx context.Context, ses *Session, msg Message) {
	handlers := r.tree.match(msg.Path)

	c := r.contextPool.Get().(*Context)
	c.Path = msg.Path
	c.ReqID = msg.ReqID
	c.Session = ses
	c.Values = map[string]any{}
	c.serializer = r.serializer

	defer func() {
		c.reset()
		r.contextPool.Put(c)
	}()

	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("message handler paniced", "error", rec)
		}
	}()

	for _, h := range handlers {
		if err := h(c, msg.Data); err != nil {
			r.logger.Error("message handler failes", "error", err)
			break
		}
		if c.halt {
			break
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

func (t *routerTree) insertFallback(handler compiledHandler) {
	t.fallback = append(t.fallback, handler)
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
