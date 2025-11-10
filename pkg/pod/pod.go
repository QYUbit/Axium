package pod

import "github.com/QYUbit/Axium/pkg/transport/quic"

type IdGenerator func() string

type Pod struct {
	transport   quic.Transport
	idGenerator IdGenerator
	onConnect   func(remoteAddr string) (pass bool, reason string)
	onError     func(error)
}

type Options struct {
	Transport   quic.Transport
	IdGenerator IdGenerator
}

func NewPod(o Options) *Pod {
	p := &Pod{
		transport:   o.Transport,
		idGenerator: o.IdGenerator,
	}

	p.onConnect = func(remoteAddr string) (pass bool, reason string) {
		return true, ""
	}

	p.transport.OnConnect(p.handleConnect)
	p.transport.OnDisconnect(p.handleDisconnect)
	p.transport.OnMessage(p.handleMessage)
	p.transport.OnError(p.onError)

	return p
}

func (p *Pod) handleConnect(remoteAddr string, accept func(id string), reject func(reason string)) {
	pass, reason := p.onConnect(remoteAddr)
	if pass {
		accept(p.idGenerator())
	} else {
		reject(reason)
	}
}

func (p *Pod) handleDisconnect(client string) {

}

func (p *Pod) handleMessage(from string, data []byte) {

}

func (p *Pod) Start() error {
	return p.transport.Start()
}

func DO() {
	pod := NewPod(Options{})
	pod.Start()
}
