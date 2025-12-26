package actors

import (
	"sync"
)

type Actor interface {
	Identifier() string
	Init(e *Engine)
	Update(e *Engine, dt float64)
}

type BaseActor struct {
	identifier string
}

func NewBaseActor(identifier string) *BaseActor {
	return &BaseActor{identifier}
}

func (a *BaseActor) Identifier() string {
	return a.identifier
}

func (a *BaseActor) Init(_ *Engine) {}

func (a *BaseActor) Update(_ *Engine, _ float64) {}

type Engine struct {
	actors sync.Map
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Add(a Actor) {
	e.actors.Store(a.Identifier(), a)
	a.Init(e)
}

func (e *Engine) Remove(identifier string) {
	e.actors.Delete(identifier)
}

func (e *Engine) Actor(identifier string) (Actor, bool) {
	v, ok := e.actors.Load(identifier)
	if !ok {
		return nil, false
	}
	a, ok := v.(Actor)
	if !ok {
		e.actors.Delete(identifier)
		return nil, false
	}
	return a, true
}

func (e *Engine) Update(dt float64) {
	e.actors.Range(func(identifier any, value any) bool {
		a, ok := value.(Actor)
		if !ok {
			e.actors.Delete(identifier)
			return true
		}
		a.Update(e, dt)
		return true
	})
}

type Player struct {
	*BaseActor
	pos_x float64
	pos_y float64
	vel_x float64
	vel_y float64
}

func NewPlayer() *Player {
	return &Player{
		BaseActor: NewBaseActor("player"),
	}
}

func (p *Player) Update(_ *Engine, dt float64) {
	p.pos_x += p.vel_x
	p.pos_y += p.vel_y
}

func Example() {
	e := NewEngine()

	player := NewPlayer()
	player.vel_x = 10

	e.Add(player)

	e.Update(0.1)
}
