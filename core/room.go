package core

import (
	"context"
	"sync"
	"time"
)

type RoomConfig struct {
	Id           string
	Transport    AxiumTransport
	UseTicker    bool
	TickInterval time.Duration
	Context      context.Context
}

type MessageHandler func(origin *Session, data []byte)

type Room struct {
	id           string
	tickInterval time.Duration
	members      map[string]*Session
	memberMu     sync.RWMutex
	transport    AxiumTransport
	ctx          context.Context
	cancel       context.CancelFunc

	messageHandlers  map[string]MessageHandler
	messageHandlerMu sync.RWMutex
	fallback         MessageHandler

	OnCreate  func()
	OnJoin    func(session *Session)
	OnLeave   func(session *Session)
	OnDestroy func()
	OnTick    func(dt time.Duration)
}

func NewRoom(config RoomConfig) *Room {
	ctx, cancel := context.WithCancel(config.Context)

	r := &Room{
		id:              config.Id,
		tickInterval:    config.TickInterval,
		members:         make(map[string]*Session),
		transport:       config.Transport,
		messageHandlers: make(map[string]MessageHandler),
		ctx:             ctx,
		cancel:          cancel,

		OnCreate:  func() {},
		OnJoin:    func(session *Session) {},
		OnLeave:   func(session *Session) {},
		OnDestroy: func() {},
		OnTick:    func(dt time.Duration) {},
		fallback:  func(origin *Session, data []byte) {},
	}

	if config.UseTicker {
		go r.run()
	}

	return r
}

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

func (r *Room) Assign(session *Session) error {
	r.memberMu.Lock()
	r.members[session.id] = session
	r.memberMu.Unlock()

	if err := r.transport.Subscribe(session.id, r.id); err != nil {
		return err
	}

	r.OnJoin(session)
	return nil
}

func (r *Room) Unassign(session *Session) error {
	r.memberMu.Lock()
	delete(r.members, session.id)
	r.memberMu.Unlock()

	if err := r.transport.Unsubscribe(session.id, r.id); err != nil {
		return err
	}

	r.OnLeave(session)
	return nil
}

func (r *Room) destroy() error {
	r.cancel()

	r.memberMu.RLock()
	for sessionId := range r.members {
		if err := r.transport.Unsubscribe(sessionId, r.id); err != nil {
			return err
		}
	}
	r.memberMu.RUnlock()

	return r.transport.DeleteTopic(r.id)
}

func (r *Room) Broadcast(data []byte, reliable bool) error {
	return r.transport.Publish(r.id, data) // TODO Transport: reliable Publish
}

// func (r *Room) SendTo(sessionId string, data []byte, reliable bool) error {
// 	return r.transport.Send(sessionId, data, reliable)
// }

func (r *Room) RegisterHandler(event string, handler MessageHandler) {
	r.messageHandlerMu.Lock()
	r.messageHandlers[event] = handler
	r.messageHandlerMu.Unlock()
}

func (r *Room) FallbackHandler(handler MessageHandler) {
	r.fallback = handler
}

func (r *Room) run() {
	ticker := time.NewTicker(r.tickInterval)
	defer ticker.Stop()

	var lastTick time.Time

	for {
		select {
		case <-r.ctx.Done():
			return
		case t := <-ticker.C:
			dt := t.Sub(lastTick) // First delta time?
			lastTick = t
			go r.OnTick(dt)
		}
	}
}
