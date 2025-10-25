package axium

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type RoomConfig struct {
	Id           string
	Transport    AxiumTransport
	Serializer   AxiumSerializer
	UseTicker    bool
	TickInterval time.Duration
	Context      context.Context
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

	messageHandlers  map[string]MessageHandler
	messageHandlerMu sync.RWMutex
	fallback         MessageHandler

	onCreate  func()
	onJoin    func(session *Session)
	onLeave   func(session *Session)
	onDestroy func()
	onTick    func(dt time.Duration)
}

func NewRoom(config RoomConfig) *Room {
	ctx, cancel := context.WithCancel(config.Context)

	r := &Room{
		id:              config.Id,
		tickInterval:    config.TickInterval,
		members:         make(map[string]*Session),
		transport:       config.Transport,
		serializer:      config.Serializer,
		messageHandlers: make(map[string]MessageHandler),
		//state:           make(map[string]any),
		ctx:    ctx,
		cancel: cancel,

		onCreate:  func() {},
		onJoin:    func(session *Session) {},
		onLeave:   func(session *Session) {},
		onDestroy: func() {},
		onTick:    func(dt time.Duration) {},
		fallback:  func(origin *Session, data []byte) {},
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

func (r *Room) BroadcastEvent(eventType string, data []byte, reliable bool) error {
	msg := Message{
		MessageAction: string(RoomEventAction),
		RoomEventMsg: &RoomEventMsg{
			EventType: eventType,
			Data:      data,
			RoomId:    r.id,
		},
	}

	encoded, err := r.serializer.EncodeMessage(msg)
	if err != nil {
		return err
	}

	return r.Broadcast(encoded, reliable)
}

func (r *Room) BroadcastExcept(sessionId string, data []byte, reliable bool) error {
	r.memberMu.RLock()
	members := make([]*Session, 0, len(r.members))
	for id, session := range r.members {
		if id != sessionId {
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

func (r *Room) BroadcastEventExcept(sessionId string, eventType string, data []byte, reliable bool) error {
	msg := Message{
		MessageAction: string(RoomEventAction),
		RoomEventMsg: &RoomEventMsg{
			EventType: eventType,
			Data:      data,
			RoomId:    r.id,
		},
	}

	encoded, err := r.serializer.EncodeMessage(msg)
	if err != nil {
		return err
	}

	return r.BroadcastExcept(sessionId, encoded, reliable)
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

func (r *Room) RegisterHandler(event string, handler MessageHandler) {
	r.messageHandlerMu.Lock()
	r.messageHandlers[event] = handler
	r.messageHandlerMu.Unlock()
}

func (r *Room) UnregisterHandler(event string) {
	r.messageHandlerMu.Lock()
	delete(r.messageHandlers, event)
	r.messageHandlerMu.Unlock()
}

func (r *Room) FallbackHandler(handler MessageHandler) {
	r.fallback = handler
}

// ==============================================
// Ticker
// ==============================================

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

func (r *Room) SetTickInterval(interval time.Duration) {
	r.tickInterval = interval
}

func (r *Room) GetTickInterval() time.Duration {
	return r.tickInterval
}
