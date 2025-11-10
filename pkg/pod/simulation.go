package pod

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

var (
	ErrSimulationRunning    = errors.New("simulation is already running")
	ErrSimulationNotRunning = errors.New("simulation is not running")
)

type Simulation struct {
	tickRate  time.Duration
	lastTick  time.Time
	onTick    func(dt time.Duration)
	ctx       context.Context
	cancel    context.CancelFunc
	isRunning atomic.Bool
}

func NewSimulation(tickRate time.Duration) *Simulation {
	ctx, cancel := context.WithCancel(context.Background())
	return &Simulation{
		tickRate: tickRate,
		onTick:   func(dt time.Duration) {},
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (s *Simulation) Start() error {
	if !s.isRunning.CompareAndSwap(false, true) {
		return ErrSimulationRunning
	}

	t := time.NewTicker(s.tickRate)

	for {
		select {
		case <-s.ctx.Done():
			return nil
		case now := <-t.C:
			s.step(now)
		}
	}
}

func (s *Simulation) Stop() error {
	if !s.isRunning.CompareAndSwap(true, false) {
		return ErrSimulationNotRunning
	}
	s.cancel()
	return nil
}

func (s *Simulation) step(now time.Time) {
	var d time.Duration
	if !s.lastTick.IsZero() {
		d = now.Sub(s.lastTick)
	}

	s.lastTick = now

	go s.onTick(d)
}

func (s *Simulation) SetTickRate(d time.Duration) {
	s.tickRate = d
}

func (s *Simulation) OnTick(fn func(dt time.Duration)) {
	s.onTick = fn
}
