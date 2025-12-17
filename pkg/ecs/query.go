package main

import (
	"iter"
	"math"
	"sort"
)

type QNType int

const (
	QNAnd QNType = iota
	QNOr
	QNNot
	QNHas
)

type QueryNode struct {
	typ       QNType
	children  []*QueryNode
	component ComponentID
}

func With(components ...Component) *QueryNode {
	if len(components) == 1 {
		return has(components[0].Id())
	}

	var children []*QueryNode
	for _, comp := range components {
		children = append(children, has(comp.Id()))
	}

	return And(children...)
}

func Without(components ...Component) *QueryNode {
	if len(components) == 1 {
		return not(
			has(components[0].Id()),
		)
	}

	var children []*QueryNode
	for _, comp := range components {
		children = append(children, not(
			has(comp.Id()),
		))
	}

	return And(children...)
}

func And(nodes ...*QueryNode) *QueryNode {
	return &QueryNode{
		typ:      QNAnd,
		children: nodes,
	}
}

func Or(nodes ...*QueryNode) *QueryNode {
	return &QueryNode{
		typ:      QNOr,
		children: nodes,
	}
}

func not(node *QueryNode) *QueryNode {
	return &QueryNode{
		typ:      QNNot,
		children: []*QueryNode{node},
	}
}

func has(cid ComponentID) *QueryNode {
	return &QueryNode{
		typ:       QNHas,
		component: cid,
	}
}

type Row1[T Component] struct {
	ID    EntityID
	store *Store[T]
}

func SimpleQuery1[T Component](w *World) iter.Seq[Row1[T]] {
	var z T
	return Query1[T](w, With(z))
}

func Query1[T Component](w *World, query *QueryNode) iter.Seq[Row1[T]] {
	s, ok := getStoreFromWorld[T](w)
	if !ok {
		return func(yield func(Row1[T]) bool) {}
	}

	it := query.Query(w)

	return func(yield func(Row1[T]) bool) {
		var row Row1[T]

		for id := range it {
			row.ID = id
			row.store = s

			if !yield(row) {
				return
			}
		}
	}
}

func (r Row1[T]) Get() *T {
	return r.store.Get(r.ID)
}

func (r Row1[T]) GetMutable() *T {
	return r.store.GetMutable(r.ID)
}

func (r Row1[T]) GetStatic() T {
	return r.store.GetStatic(r.ID)
}

type Row2[T1, T2 Component] struct {
	ID     EntityID
	store1 *Store[T1]
	store2 *Store[T2]
}

func SimpleQuery2[T1, T2 Component](w *World) iter.Seq[Row2[T1, T2]] {
	var z1 T1
	var z2 T2
	return Query2[T1, T2](w, With(z1, z2))
}

func Query2[T1, T2 Component](w *World, query *QueryNode) iter.Seq[Row2[T1, T2]] {
	s1, ok1 := getStoreFromWorld[T1](w)
	s2, ok2 := getStoreFromWorld[T2](w)

	if !ok1 || !ok2 {
		return func(yield func(Row2[T1, T2]) bool) {}
	}

	it := query.Query(w)

	return func(yield func(Row2[T1, T2]) bool) {
		var row Row2[T1, T2]

		for id := range it {
			row.ID = id
			row.store1 = s1
			row.store2 = s2

			if !yield(row) {
				return
			}
		}
	}
}

func (r Row2[T1, T2]) Get1() *T1 {
	return r.store1.Get(r.ID)
}

func (r Row2[T1, T2]) GetMutable1() *T1 {
	return r.store1.GetMutable(r.ID)
}

func (r Row2[T1, T2]) GetStatic1() T1 {
	return r.store1.GetStatic(r.ID)
}

func (r Row2[T1, T2]) Get2() *T2 {
	return r.store2.Get(r.ID)
}

func (r Row2[T1, T2]) GetMutable2() *T2 {
	return r.store2.GetMutable(r.ID)
}

func (r Row2[T1, T2]) GetStatic2() T2 {
	return r.store2.GetStatic(r.ID)
}

type Row3[T1, T2, T3 Component] struct {
	ID     EntityID
	store1 *Store[T1]
	store2 *Store[T2]
	store3 *Store[T3]
}

func SimpleQuery3[T1, T2, T3 Component](w *World) iter.Seq[Row3[T1, T2, T3]] {
	var z1 T1
	var z2 T2
	var z3 T3
	return Query3[T1, T2, T3](w, With(z1, z2, z3))
}

func Query3[T1, T2, T3 Component](w *World, query *QueryNode) iter.Seq[Row3[T1, T2, T3]] {
	s1, ok1 := getStoreFromWorld[T1](w)
	s2, ok2 := getStoreFromWorld[T2](w)
	s3, ok3 := getStoreFromWorld[T3](w)

	if !ok1 || !ok2 || !ok3 {
		return func(yield func(Row3[T1, T2, T3]) bool) {}
	}

	it := query.Query(w)

	return func(yield func(Row3[T1, T2, T3]) bool) {
		var row Row3[T1, T2, T3]

		for id := range it {
			row.ID = id
			row.store1 = s1
			row.store2 = s2
			row.store3 = s3

			if !yield(row) {
				return
			}
		}
	}
}

func (r Row3[T1, T2, T3]) Get1() *T1 {
	return r.store1.Get(r.ID)
}

func (r Row3[T1, T2, T3]) GetMutable1() *T1 {
	return r.store1.GetMutable(r.ID)
}

func (r Row3[T1, T2, T3]) GetStatic1() T1 {
	return r.store1.GetStatic(r.ID)
}

func (r Row3[T1, T2, T3]) Get2() *T2 {
	return r.store2.Get(r.ID)
}

func (r Row3[T1, T2, T3]) GetMutable2() *T2 {
	return r.store2.GetMutable(r.ID)
}

func (r Row3[T1, T2, T3]) GetStatic2() T2 {
	return r.store2.GetStatic(r.ID)
}

func (r Row3[T1, T2, T3]) Get3() *T3 {
	return r.store3.Get(r.ID)
}

func (r Row3[T1, T2, T3]) GetMutable3() *T3 {
	return r.store3.GetMutable(r.ID)
}

func (r Row3[T1, T2, T3]) GetStatic3() T3 {
	return r.store3.GetStatic(r.ID)
}

func (tree *QueryNode) Query(w *World) iter.Seq[EntityID] {
	optimized := tree.optimize(w)
	return optimized.queryPlan(w)
}

func (qn *QueryNode) optimize(w *World) *QueryNode {
	if qn == nil {
		return nil
	}

	for i, child := range qn.children {
		qn.children[i] = child.optimize(w)
	}

	if qn.typ == QNAnd {
		var newChildren []*QueryNode
		for _, child := range qn.children {
			if child.typ == QNAnd {
				newChildren = append(newChildren, child.children...)
			} else {
				newChildren = append(newChildren, child)
			}
		}
		qn.children = newChildren

		sort.Slice(qn.children, func(i, j int) bool {
			return qn.children[i].cost(w) < qn.children[j].cost(w)
		})
	}
	return qn
}

func (qn *QueryNode) cost(w *World) int {
	switch qn.typ {
	case QNHas:
		if store, ok := w.storesById[qn.component]; ok {
			return store.Len()
		}
		return 0

	case QNAnd:
		minCost := math.MaxInt
		for _, child := range qn.children {
			c := child.cost(w)
			if c < minCost {
				minCost = c
			}
		}
		return minCost
	}
	return math.MaxInt
}

func (qn *QueryNode) queryPlan(w *World) iter.Seq[EntityID] {
	return func(yield func(EntityID) bool) {
		switch qn.typ {

		case QNHas:
			store, ok := w.storesById[qn.component]
			if !ok {
				return
			}

			for id := range store.Entities() {
				if !yield(id) {
					return
				}
			}

		case QNAnd:
			if len(qn.children) == 0 {
				return
			}

			driverNode := qn.children[0]
			filterNodes := qn.children[1:]

			driverIter := driverNode.queryPlan(w)

			for id := range driverIter {
				match := true
				for _, filter := range filterNodes {
					if !filter.checkCondition(w, id) {
						match = false
						break
					}
				}

				if match {
					if !yield(id) {
						return
					}
				}
			}

		case QNOr:
			visited := make(map[EntityID]struct{})

			for _, child := range qn.children {
				childIter := child.queryPlan(w)
				for id := range childIter {
					if _, alreadySeen := visited[id]; !alreadySeen {
						visited[id] = struct{}{}
						if !yield(id) {
							return
						}
					}
				}
			}
		}
	}
}

func (qn *QueryNode) checkCondition(w *World, id EntityID) bool {
	switch qn.typ {
	case QNHas:
		if store, ok := w.storesById[qn.component]; ok {
			return store.HasEntity(id)
		}
		return false
	case QNAnd:
		for _, child := range qn.children {
			if !child.checkCondition(w, id) {
				return false
			}
		}
		return true
	case QNOr:
		for _, child := range qn.children {
			if child.checkCondition(w, id) {
				return true
			}
		}
		return false
	case QNNot:
		if len(qn.children) > 0 {
			return !qn.children[0].checkCondition(w, id)
		}
		return true
	}
	return false
}
