package ecs

import (
	"iter"
	"math"
	"reflect"
)

// ==================================================================
// Config
// ==================================================================

type QueryConfig struct {
	Required         map[reflect.Type]struct{}
	Excluded         map[reflect.Type]struct{}
	Entities         map[Entity]struct{}
	ExcludedEntities map[Entity]struct{}
}

func buildQueryConfig(opts []QueryOption) QueryConfig {
	config := QueryConfig{
		Required:         make(map[reflect.Type]struct{}),
		Excluded:         make(map[reflect.Type]struct{}),
		Entities:         make(map[Entity]struct{}),
		ExcludedEntities: make(map[Entity]struct{}),
	}
	for _, opt := range opts {
		opt(&config)
	}
	return config
}

type QueryOption func(*QueryConfig)

func Require(comps ...any) QueryOption {
	return func(config *QueryConfig) {
		for _, comp := range comps {
			config.Required[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

func Exclude(comps ...any) QueryOption {
	return func(config *QueryConfig) {
		for _, comp := range comps {
			config.Excluded[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

func Entities(entities ...Entity) QueryOption {
	return func(config *QueryConfig) {
		for _, e := range entities {
			config.Entities[e] = struct{}{}
		}
	}
}

func ExcludeEntities(entities ...Entity) QueryOption {
	return func(config *QueryConfig) {
		for _, e := range entities {
			config.ExcludedEntities[e] = struct{}{}
		}
	}
}

// ==================================================================
// Abstraction
// ==================================================================

type entitySource interface {
	Entities() iter.Seq[Entity]
	HasEntity(e Entity) bool
	Len() int
}

type entitySet map[Entity]struct{}

func (set entitySet) Entities() iter.Seq[Entity] {
	return func(yield func(Entity) bool) {
		for e := range set {
			if !yield(e) {
				return
			}
		}
	}
}

func (set entitySet) HasEntity(e Entity) bool {
	_, ok := set[e]
	return ok
}

func (set entitySet) Len() int {
	return len(set)
}

type query struct {
	driver     entitySource
	filters    []entitySource
	exclusions []entitySource
}

func newQuery(w *World, config QueryConfig) *query {
	q := &query{}

	// exclusions

	if len(config.ExcludedEntities) > 0 {
		q.exclusions = append(q.exclusions, entitySet(config.ExcludedEntities))
	}

	for t := range config.Excluded {
		s, ok := w.stores[t]
		if !ok {
			continue
		}
		q.exclusions = append(q.exclusions, s)
	}

	// requirements

	var requirements []entitySource

	if len(config.Entities) > 0 {
		requirements = append(requirements, entitySet(config.Entities))
	}

	for t := range config.Required {
		s, ok := w.stores[t]
		if !ok {
			return nil
		}
		requirements = append(requirements, s)
	}

	// get driver

	minCost := math.MaxInt

	for _, ei := range requirements {
		if ei.Len() > minCost {
			q.filters = append(q.filters, ei)
			continue
		}

		if q.driver != nil {
			q.filters = append(q.filters, q.driver)
		}
		q.driver = ei
	}

	return q
}

func (q *query) iter() iter.Seq[Entity] {
	return func(yield func(Entity) bool) {
		for e := range q.driver.Entities() {
			match := true

			for _, s := range q.filters {
				if !s.HasEntity(e) {
					match = false
					break
				}
			}

			if !match {
				continue
			}

			for _, s := range q.exclusions {
				if s.HasEntity(e) {
					match = false
					break
				}
			}

			if match {
				if !yield(e) {
					return
				}
			}
		}
	}
}

// ==================================================================
// Query 1
// ==================================================================

type View1[T any] struct {
	s     *Store[T]
	it    iter.Seq[Entity]
	empty bool
}

func Query1[T any](w *World, opts ...QueryOption) *View1[T] {
	s, ok := getStoreFromWorld[T](w)
	if !ok {
		return &View1[T]{empty: true}
	}

	config := buildQueryConfig(opts)
	config.Required[reflect.TypeFor[T]()] = struct{}{}

	q := newQuery(w, config)
	if q == nil {
		return &View1[T]{empty: true}
	}

	return &View1[T]{
		it: q.iter(),
		s:  s,
	}
}

type Row1[T any] struct {
	ID Entity
	C  *T
}

func (v *View1[T]) Iter() iter.Seq[Row1[T]] {
	return func(yield func(Row1[T]) bool) {
		var row Row1[T]

		for id := range v.it {
			row.ID = id
			row.C = v.s.Get(id)

			if !yield(row) {
				return
			}
		}
	}
}

// ==================================================================
// Query 2
// ==================================================================

type View2[T1, T2 any] struct {
	s1    *Store[T1]
	s2    *Store[T2]
	it    iter.Seq[Entity]
	empty bool
}

func Query2[T1, T2 any](w *World, opts ...QueryOption) *View2[T1, T2] {
	s1, ok := getStoreFromWorld[T1](w)
	if !ok {
		return &View2[T1, T2]{empty: true}
	}
	s2, ok := getStoreFromWorld[T2](w)
	if !ok {
		return &View2[T1, T2]{empty: true}
	}

	config := buildQueryConfig(opts)
	config.Required[reflect.TypeFor[T1]()] = struct{}{}
	config.Required[reflect.TypeFor[T2]()] = struct{}{}

	q := newQuery(w, config)
	if q == nil {
		return &View2[T1, T2]{empty: true}
	}

	return &View2[T1, T2]{
		it: q.iter(),
		s1: s1,
		s2: s2,
	}
}

type Row2[T1, T2 any] struct {
	ID Entity
	C1 *T1
	C2 *T2
}

func (v *View2[T1, T2]) Iter() iter.Seq[Row2[T1, T2]] {
	return func(yield func(Row2[T1, T2]) bool) {
		var row Row2[T1, T2]

		for id := range v.it {
			row.ID = id
			row.C1 = v.s1.Get(id)
			row.C2 = v.s2.Get(id)

			if !yield(row) {
				return
			}
		}
	}
}

// ==================================================================
// Query 3
// ==================================================================

type View3[T1, T2, T3 any] struct {
	s1    *Store[T1]
	s2    *Store[T2]
	s3    *Store[T3]
	it    iter.Seq[Entity]
	empty bool
}

func Query3[T1, T2, T3 any](w *World, opts ...QueryOption) *View3[T1, T2, T3] {
	s1, ok := getStoreFromWorld[T1](w)
	if !ok {
		return &View3[T1, T2, T3]{empty: true}
	}
	s2, ok := getStoreFromWorld[T2](w)
	if !ok {
		return &View3[T1, T2, T3]{empty: true}
	}
	s3, ok := getStoreFromWorld[T3](w)
	if !ok {
		return &View3[T1, T2, T3]{empty: true}
	}

	config := buildQueryConfig(opts)
	config.Required[reflect.TypeFor[T1]()] = struct{}{}
	config.Required[reflect.TypeFor[T2]()] = struct{}{}
	config.Required[reflect.TypeFor[T3]()] = struct{}{}

	q := newQuery(w, config)
	if q == nil {
		return &View3[T1, T2, T3]{empty: true}
	}

	return &View3[T1, T2, T3]{
		it: q.iter(),
		s1: s1,
		s2: s2,
		s3: s3,
	}
}

type Row3[T1, T2, T3 any] struct {
	ID Entity
	C1 *T1
	C2 *T2
	C3 *T3
}

func (v *View3[T1, T2, T3]) Iter() iter.Seq[Row3[T1, T2, T3]] {
	return func(yield func(Row3[T1, T2, T3]) bool) {
		var row Row3[T1, T2, T3]

		for id := range v.it {
			row.ID = id
			row.C1 = v.s1.Get(id)
			row.C2 = v.s2.Get(id)
			row.C3 = v.s3.Get(id)

			if !yield(row) {
				return
			}
		}
	}
}
