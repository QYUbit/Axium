package server

import (
	"context"
	"sync"

	"github.com/QYUbit/Axium/pkg/transport"
)

type Room struct {
	id string

	members   map[string]*Session
	membersMu sync.RWMutex

	onCreate  func()
	onJoin    func(session *Session)
	onLeave   func(session *Session)
	onDestroy func()

	server    *Server
	transport transport.Transport

	router *Router

	ctx    context.Context
	cancel context.CancelFunc
}

type RoomConfig struct {
	ID        string
	Context   context.Context
	Server    *Server
	Transport transport.Transport
}

func NewRoom(config RoomConfig) *Room {
	ctx, cancel := context.WithCancel(config.Context)

	r := &Room{
		id:        config.ID,
		members:   make(map[string]*Session),
		transport: config.Transport,
		server:    config.Server,
		ctx:       ctx,
		cancel:    cancel,

		onCreate:  func() {},
		onJoin:    func(session *Session) {},
		onLeave:   func(session *Session) {},
		onDestroy: func() {},
	}

	return r
}

func (r *Room) ID() string {
	return r.id
}

func (r *Room) MemberCount() int {
	r.membersMu.RLock()
	defer r.membersMu.RUnlock()
	return len(r.members)
}

func (r *Room) Members() []*Session {
	r.membersMu.RLock()
	defer r.membersMu.RUnlock()
	members := make([]*Session, 0, len(r.members))
	for _, member := range r.members {
		members = append(members, member)
	}
	return members
}

func (r *Room) MemberIDs() []string {
	r.membersMu.RLock()
	defer r.membersMu.RUnlock()
	members := make([]string, 0, len(r.members))
	for id := range r.members {
		members = append(members, id)
	}
	return members
}

func (r *Room) GetMember(sessionId string) (*Session, bool) {
	r.membersMu.RLock()
	defer r.membersMu.RUnlock()
	session, exists := r.members[sessionId]
	return session, exists
}

func (r *Room) HasMember(sessionId string) bool {
	r.membersMu.RLock()
	defer r.membersMu.RUnlock()
	_, exists := r.members[sessionId]
	return exists
}

func (r *Room) Context() context.Context {
	return r.ctx
}

func (r *Room) Assign(session *Session) {
	r.membersMu.Lock()
	r.members[session.id] = session
	r.membersMu.Unlock()

	session.addToRoom(r)

	r.onJoin(session)
}

func (r *Room) Unassign(session *Session) {
	r.membersMu.Lock()
	_, exists := r.members[session.id]
	if exists {
		delete(r.members, session.id)
	}
	r.membersMu.Unlock()

	if !exists {
		return
	}

	session.removeFromRoom(r)

	r.onLeave(session)
}

func (r *Room) Destroy() {
	r.server.destroyRoom(r)

	r.onDestroy()
	r.cancel()

	r.membersMu.RLock()
	members := make([]*Session, 0, len(r.members))
	for _, session := range r.members {
		members = append(members, session)
	}
	r.membersMu.RUnlock()

	for _, session := range members {
		r.Unassign(session)
	}
}

func (r *Room) buildMsg(path string, payload []byte) ([]byte, error) {
	msg := OutgoingMessage{
		Path:     path,
		FromRoom: true,
		Room:     r.id,
		Payload:  payload,
	}
	var data []byte
	return data, r.server.protocol.Encode(data, msg)
}

func (r *Room) Broadcast(path string, payload []byte) error {
	data, err := r.buildMsg(path, payload)
	if err != nil {
		return err
	}
	return r.transport.Broadcast(r.MemberIDs(), data, true)
}

func (r *Room) BroadcastUnreliable(path string, payload []byte) error {
	data, err := r.buildMsg(path, payload)
	if err != nil {
		return err
	}
	return r.transport.Broadcast(r.MemberIDs(), data, false)
}

func (r *Room) BroadcastExcept(path string, payload []byte, exceptions ...string) error {
	data, err := r.buildMsg(path, payload)
	if err != nil {
		return err
	}

	except := make(map[string]struct{}, len(exceptions))
	for _, id := range exceptions {
		except[id] = struct{}{}
	}

	r.membersMu.RLock()
	included := make([]string, 0, len(r.members))
	for id := range r.members {
		if _, ok := except[id]; !ok {
			included = append(included, id)
		}
	}
	r.membersMu.RUnlock()

	return r.transport.Broadcast(included, data, true)
}

func (r *Room) BroadcastUnreliableExcept(path string, payload []byte, exceptions ...string) error {
	data, err := r.buildMsg(path, payload)
	if err != nil {
		return err
	}

	except := make(map[string]struct{}, len(exceptions))
	for _, id := range exceptions {
		except[id] = struct{}{}
	}

	r.membersMu.RLock()
	included := make([]string, 0, len(r.members))
	for id := range r.members {
		if _, ok := except[id]; !ok {
			included = append(included, id)
		}
	}
	r.membersMu.RUnlock()

	return r.transport.Broadcast(included, data, false)
}

func (r *Room) SetRouter(router *Router) {
	r.router = router
}

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
