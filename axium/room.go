package axium

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type IRoom interface {
	Id() string
	Context() context.Context
	MemberCount() int
	Members() []*Session
	GetMember(sessionId string) (*Session, bool)
	HasMember(sessionId string) bool
	Assign(session *Session) error
	Unassign(session *Session) error
	Broadcast(data []byte, reliable bool) error
	BroadcastExcept(data []byte, reliable bool, exceptions ...string) error
	OnCreate(func())
	OnJoin(func(s *Session))
	OnLeave(func(s *Session))
	OnDestroy(func())
	OnTick(func(dt time.Duration))
	MiddlewareHandler(handler MiddlewareHandler)
	MessageHandler(event string, handler MessageHandler)
	FallbackHandler(handler MessageHandler)
}

type Room struct {
	id           string
	tickInterval time.Duration
	members      map[string]*Session
	memberMu     sync.RWMutex
	transport    AxiumTransport
	serializer   AxiumSerializer
	ctx          context.Context
	cancel       context.CancelFunc

	middlewareHandler MiddlewareHandler
	messageHandlers   map[string]MessageHandler
	messageHandlerMu  sync.RWMutex
	fallback          MessageHandler

	onCreate  func()
	onJoin    func(session *Session)
	onLeave   func(session *Session)
	onDestroy func()
	onTick    func(dt time.Duration)
}

type RoomConfig struct {
	Id           string
	Transport    AxiumTransport
	Serializer   AxiumSerializer
	UseTicker    bool
	TickInterval time.Duration
	Context      context.Context
}

func NewRoom(config RoomConfig) *Room {
	ctx, cancel := context.WithCancel(config.Context)

	r := &Room{
		id:           config.Id,
		tickInterval: config.TickInterval,
		members:      make(map[string]*Session),
		transport:    config.Transport,
		serializer:   config.Serializer,
		ctx:          ctx,
		cancel:       cancel,

		middlewareHandler: func(session *Session, data []byte) bool { return true },
		messageHandlers:   make(map[string]MessageHandler),
		fallback:          func(session *Session, data []byte) {},

		onCreate:  func() {},
		onJoin:    func(session *Session) {},
		onLeave:   func(session *Session) {},
		onDestroy: func() {},
		onTick:    func(dt time.Duration) {},
	}

	if config.UseTicker && config.TickInterval > 0 {
		go r.run()
	}

	return r
}

// ==============================================
// Getters
// ==============================================

func (r *Room) Id() string {
	return r.id
}

func (r *Room) MemberCount() int {
	r.memberMu.RLock()
	defer r.memberMu.RUnlock()
	return len(r.members)
}

func (r *Room) Members() []*Session {
	r.memberMu.RLock()
	defer r.memberMu.RUnlock()
	members := make([]*Session, 0, len(r.members))
	for _, member := range r.members {
		members = append(members, member)
	}
	return members
}

func (r *Room) GetMember(sessionId string) (*Session, bool) {
	r.memberMu.RLock()
	defer r.memberMu.RUnlock()
	session, exists := r.members[sessionId]
	return session, exists
}

func (r *Room) HasMember(sessionId string) bool {
	r.memberMu.RLock()
	defer r.memberMu.RUnlock()
	_, exists := r.members[sessionId]
	return exists
}

func (r *Room) Context() context.Context {
	return r.ctx
}

// ==============================================
// Member management
// ==============================================

func (r *Room) Assign(session *Session) error {
	r.memberMu.Lock()
	r.members[session.id] = session
	r.memberMu.Unlock()

	if err := r.transport.Subscribe(session.id, r.id); err != nil {
		r.memberMu.Lock()
		delete(r.members, session.id)
		r.memberMu.Unlock()
		return err
	}

	session.joinRoom(r)

	r.onJoin(session)
	return nil
}

func (r *Room) Unassign(session *Session) error {
	r.memberMu.Lock()
	_, exists := r.members[session.id]
	if exists {
		delete(r.members, session.id)
	}
	r.memberMu.Unlock()

	if !exists {
		return fmt.Errorf("session %s is not a member of room %s", session.id, r.id)
	}

	if err := r.transport.Unsubscribe(session.id, r.id); err != nil {
		return err
	}

	session.leaveRoom(r)

	r.onLeave(session)
	return nil
}

func (r *Room) destroy() error {
	r.onDestroy()
	r.cancel()

	r.memberMu.RLock()
	members := make([]*Session, 0, len(r.members))
	for _, session := range r.members {
		members = append(members, session)
	}
	r.memberMu.RUnlock()

	for _, session := range members {
		if err := r.Unassign(session); err != nil {
			// propergate error
			fmt.Printf("Error unassigning session %s from room %s: %s\n", session.id, r.id, err)
		}
	}

	return r.transport.DeleteTopic(r.id)
}

// ==============================================
// Broadcasting
// ==============================================

func (r *Room) Broadcast(data []byte, reliable bool) error {
	return r.transport.Publish(r.id, data)
}

func (r *Room) BroadcastExcept(data []byte, reliable bool, exceptions ...string) error {
	r.memberMu.RLock()
	members := make([]*Session, 0, len(r.members))

	excluded := make(map[string]struct{}, len(exceptions))
	for _, id := range exceptions {
		excluded[id] = struct{}{}
	}

	for id, session := range r.members {
		if _, skip := excluded[id]; !skip {
			members = append(members, session)
		}
	}
	r.memberMu.RUnlock()

	var lastErr error
	for _, session := range members {
		if err := session.Send(data, reliable); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// ==============================================
// Hooks
// ==============================================

func (r *Room) OnCreate(fn func()) {
	r.onCreate = fn
}

func (r *Room) OnJoin(fn func(s *Session)) {
	r.onJoin = fn
}

func (r *Room) OnLeave(fn func(s *Session)) {
	r.onLeave = fn
}

func (r *Room) OnDestroy(fn func()) {
	r.onDestroy = fn
}

func (r *Room) OnTick(fn func(dt time.Duration)) {
	r.onTick = fn
}

// ==============================================
// Message handlers
// ==============================================

func (r *Room) MiddlewareHandler(handler MiddlewareHandler) {
	r.middlewareHandler = handler
}

func (r *Room) MessageHandler(event string, handler MessageHandler) {
	r.messageHandlerMu.Lock()
	r.messageHandlers[event] = handler
	r.messageHandlerMu.Unlock()
}

func (r *Room) FallbackHandler(handler MessageHandler) {
	r.fallback = handler
}

// ==============================================
// Ticker
// ==============================================

// TODO Maybe remove tickers from room

func (r *Room) run() {
	ticker := time.NewTicker(r.tickInterval)
	defer ticker.Stop()

	lastTick := time.Now()

	for {
		select {
		case <-r.ctx.Done():
			return
		case t := <-ticker.C:
			dt := t.Sub(lastTick)
			lastTick = t
			go r.onTick(dt)
		}
	}
}

// TODO StopTicker, StartTicker

func (r *Room) SetTickInterval(interval time.Duration) {
	r.tickInterval = interval
}

func (r *Room) GetTickInterval() time.Duration {
	return r.tickInterval
}
