package s2

import (
	"context"
	"sync"

	"github.com/quic-go/quic-go"
)

type sessionManager struct {
	useDatagrams bool
	sessions     map[string]*Session
	sessionsMu   sync.RWMutex
	dispatcher   msgDispatcher
	idGenerator  func() string
	protocol     MessageProtocol
}

func newSessionManager(idGenerator func() string, dispatcher msgDispatcher, protocol MessageProtocol) *sessionManager {
	return &sessionManager{
		sessions:    make(map[string]*Session),
		idGenerator: idGenerator,
		dispatcher:  dispatcher,
		protocol:    protocol,
	}
}

func (m *sessionManager) addSession(ctx context.Context, conn *quic.Conn) {
	ctx, cancel := context.WithCancel(ctx)

	id := m.idGenerator()
	ses := newSession(cancel, id, m, m.protocol, conn)

	m.sessionsMu.Lock()
	m.sessions[id] = ses
	m.sessionsMu.Unlock()

	if m.useDatagrams {
		go ses.datagramLoop(ctx)
	}
	ses.streamLoop(ctx, m.dispatcher)

	println("end of read loop")

	if err := ses.Close(quic.ApplicationErrorCode(1000), "disconnected"); err != nil {
		// TODO Log
	}
}

func (m *sessionManager) getSession(id string) (*Session, bool) {
	m.sessionsMu.RLock()
	ses, ok := m.sessions[id]
	m.sessionsMu.RUnlock()
	return ses, ok
}

func (m *sessionManager) removeSession(id string) {
	m.sessionsMu.Lock()
	delete(m.sessions, id)
	m.sessionsMu.Unlock()
}
