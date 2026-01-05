package ecs

import (
	"iter"
	"math"
	"reflect"
)

// TODO Cache queries

// ==================================================================
// Config
// ==================================================================

// QueryConfig represents query criteria.
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

// QueryOption is a function that mutates QueryConfig. It provides a practical
// way to define query criteria.
type QueryOption func(*QueryConfig)

// Require returns a QueryOption. Each queried entity must contain components
// matching the types of comps.
func Require(comps ...any) QueryOption {
	return func(config *QueryConfig) {
		for _, comp := range comps {
			config.Required[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

// Exclude returns a QueryOption. Queried entities must not contain components
// matching the types of comps.
func Exclude(comps ...any) QueryOption {
	return func(config *QueryConfig) {
		for _, comp := range comps {
			config.Excluded[reflect.TypeOf(comp)] = struct{}{}
		}
	}
}

// Entities returns a QueryOption. Queries must include entities.
func Entities(entities ...Entity) QueryOption {
	return func(config *QueryConfig) {
		for _, e := range entities {
			config.Entities[e] = struct{}{}
		}
	}
}

// Entities returns a QueryOption. Queries must not include entities.
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
	entities() iter.Seq[Entity]
	hasEntity(e Entity) bool
	len() int
}

type entitySet map[Entity]struct{}

func (set entitySet) entities() iter.Seq[Entity] {
	return func(yield func(Entity) bool) {
		for e := range set {
			if !yield(e) {
				return
			}
		}
	}
}

func (set entitySet) hasEntity(e Entity) bool {
	_, ok := set[e]
	return ok
}

func (set entitySet) len() int {
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
		if ei.len() > minCost {
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
		for e := range q.driver.entities() {
			match := true

			for _, s := range q.filters {
				if !s.hasEntity(e) {
					match = false
					break
				}
			}

			if !match {
				continue
			}

			for _, s := range q.exclusions {
				if s.hasEntity(e) {
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
	s     *store[T]
	it    iter.Seq[Entity]
	empty bool
}

// Query1 builds a query (with the options opts) and returns a view that can
// iterate over one component of each entity matching the constructed query.
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

// MarkDirty marks every quried component as dirty. This is faster than marking
// each component manually, but in some cases not as precise.
func (v *View1[T]) MarkDirty() {
	v.s.markDirty()
}

// Row1 represents an entity E and one corresponding component of type T.
type Row1[T any] struct {
	E Entity
	s *store[T]
}

// Iter returns an iterator that can interate over each queried entity.
func (v *View1[T]) Iter() iter.Seq[Row1[T]] {
	if v.empty {
		return func(yield func(Row1[T]) bool) {}
	}
	return func(yield func(Row1[T]) bool) {
		var row Row1[T]
		row.s = v.s

		for id := range v.it {
			row.E = id

			if !yield(row) {
				return
			}
		}
	}
}

// Get retrives the component for the current entity.
func (r *Row1[T]) Get() *T {
	return r.s.get(r.E)
}

// Mut retrives the component for the current entity and marks it as dirty.
func (r *Row1[T]) Mut() *T {
	return r.s.mut(r.E)
}

// ==================================================================
// Query 2
// ==================================================================

type View2[T1, T2 any] struct {
	s1    *store[T1]
	s2    *store[T2]
	it    iter.Seq[Entity]
	empty bool
}

// Query2 builds a query (with the options opts) and returns a view that can
// iterate over two components of each entity matching the constructed query.
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

// Row2 represents an entity E and two corresponding components of types T1 and T2.
type Row2[T1, T2 any] struct {
	E  Entity
	s1 *store[T1]
	s2 *store[T2]
}

// Iter returns an iterator that can interate over each queried entity.
func (v *View2[T1, T2]) Iter() iter.Seq[Row2[T1, T2]] {
	if v.empty {
		return func(yield func(Row2[T1, T2]) bool) {}
	}
	return func(yield func(Row2[T1, T2]) bool) {
		var row Row2[T1, T2]
		row.s1 = v.s1
		row.s2 = v.s2

		for id := range v.it {
			row.E = id

			if !yield(row) {
				return
			}
		}
	}
}

// Get retrives the component of type T1 for the current entity.
func (r *Row2[T1, T2]) Get1() *T1 {
	return r.s1.get(r.E)
}

// Mut retrives the component of type T1 for the current entity and marks it as dirty.
func (r *Row2[T1, T2]) Mut1() *T1 {
	return r.s1.mut(r.E)
}

// Get retrives the component of type T2 for the current entity.
func (r *Row2[T1, T2]) Get2() *T2 {
	return r.s2.get(r.E)
}

// Mut retrives the component of type T2 for the current entity and marks it as dirty.
func (r *Row2[T1, T2]) Mut2() *T2 {
	return r.s2.mut(r.E)
}

// ==================================================================
// Query 3
// ==================================================================

type View3[T1, T2, T3 any] struct {
	s1    *store[T1]
	s2    *store[T2]
	s3    *store[T3]
	it    iter.Seq[Entity]
	empty bool
}

// Query3 builds a query (with the options opts) and returns a view that can
// iterate over three components of each entity matching the constructed query.
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

// Row3 represents an entity E and three corresponding components of types T1, T2 and T3.
type Row3[T1, T2, T3 any] struct {
	E  Entity
	s1 *store[T1]
	s2 *store[T2]
	s3 *store[T3]
}

// Iter returns an iterator that can interate over each queried entity.
func (v *View3[T1, T2, T3]) Iter() iter.Seq[Row3[T1, T2, T3]] {
	if v.empty {
		return func(yield func(Row3[T1, T2, T3]) bool) {}
	}
	return func(yield func(Row3[T1, T2, T3]) bool) {
		var row Row3[T1, T2, T3]
		row.s1 = v.s1
		row.s2 = v.s2
		row.s3 = v.s3

		for id := range v.it {
			row.E = id

			if !yield(row) {
				return
			}
		}
	}
}

// Get retrives the component of type T1 for the current entity.
func (r *Row3[T1, T2, T3]) Get1() *T1 {
	return r.s1.get(r.E)
}

// Mut retrives the component of type T1 for the current entity and marks it as dirty.
func (r *Row3[T1, T2, T3]) Mut1() *T1 {
	return r.s1.mut(r.E)
}

// Get retrives the component of type T2 for the current entity.
func (r *Row3[T1, T2, T3]) Get2() *T2 {
	return r.s2.get(r.E)
}

// Mut retrives the component of type T2 for the current entity and marks it as dirty.
func (r *Row3[T1, T2, T3]) Mut2() *T2 {
	return r.s2.mut(r.E)
}

// Get retrives the component of type T3 for the current entity.
func (r *Row3[T1, T2, T3]) Get3() *T3 {
	return r.s3.get(r.E)
}

// Mut retrives the component of type T3 for the current entity and marks it as dirty.
func (r *Row3[T1, T2, T3]) Mut3() *T3 {
	return r.s3.mut(r.E)
}
