package main

import (
	"sync"
	"time"
)

type System interface {
	Init(se *StateEngine)
	Update(se *StateEngine, dt time.Duration)
	Name() string
	Stage() int
}

type StateEngine struct {
	store  *Store
	events *eventBus

	systems map[int][]System
	stages  []int
}

func NewStateEngine(store *Store, bus *eventBus) *StateEngine {
	return &StateEngine{
		store:   store,
		events:  bus,
		systems: make(map[int][]System),
	}
}

func (se *StateEngine) RegisterSystem(sys System) {
	inserted := false
	for i, stage := range se.stages {

		if sys.Stage() < stage {
			se.stages = append(se.stages[:i], append([]int{sys.Stage()}, se.stages[i:]...)...)
			inserted = true
			break
		}
	}

	if !inserted {
		se.stages = append(se.stages, sys.Stage())
	}

	systems, ok := se.systems[sys.Stage()]
	if !ok {
		se.systems[sys.Stage()] = []System{sys}
		return
	}

	se.systems[sys.Stage()] = append(systems, sys)
}

func (se *StateEngine) Update(dt time.Duration) {
	startTime := time.Now()

	var wg sync.WaitGroup
	for _, stage := range se.stages {
		systems, ok := se.systems[stage]
		if !ok {
			continue
		}

		wg.Wait()

		for _, sys := range systems {
			wg.Add(1)
			go func(s System) {
				defer wg.Done()
				s.Update(se, dt)
			}(sys)
		}
	}
	wg.Wait()

	_ = time.Since(startTime)
}
