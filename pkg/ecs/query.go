package main

import "sort"

type baseView struct {
	ids    []EntityID
	cursor int
}

func newBaseView(ids []EntityID) baseView {
	return baseView{
		cursor: -1,
		ids:    ids,
	}
}

func (bv *baseView) Next() bool {
	bv.cursor++
	return bv.cursor < len(bv.ids)
}

func (bv *baseView) GetEntity() EntityID {
	return bv.ids[bv.cursor]
}

type View1[T any] struct {
	baseView
	store *Store[T]
}

func (v *View1[T]) Get() *T {
	return v.store.Get(v.ids[v.cursor])
}

func (v *View1[T]) GetMutable() *T {
	return v.store.GetMutable(v.ids[v.cursor])
}

func (v *View1[T]) GetStatic() T {
	return v.store.GetStatic(v.ids[v.cursor])
}

type View2[T1, T2 any] struct {
	baseView
	store1 *Store[T1]
	store2 *Store[T2]
}

func (v *View2[T1, T2]) Get1() *T1 {
	return v.store1.Get(v.ids[v.cursor])
}

func (v *View2[T1, T2]) GetMutable1() *T1 {
	return v.store1.GetMutable(v.ids[v.cursor])
}

func (v *View2[T1, T2]) GetStatic1() T1 {
	return v.store1.GetStatic(v.ids[v.cursor])
}

func (v *View2[T1, T2]) Get2() *T2 {
	return v.store2.Get(v.ids[v.cursor])
}

func (v *View2[T1, T2]) GetMutable2() *T2 {
	return v.store2.GetMutable(v.ids[v.cursor])
}

func (v *View2[T1, T2]) GetStatic2() T2 {
	return v.store2.GetStatic(v.ids[v.cursor])
}

type View3[T1, T2, T3 any] struct {
	baseView
	store1 *Store[T1]
	store2 *Store[T2]
	store3 *Store[T3]
}

func (v *View3[T1, T2, T3]) Get1() *T1 {
	return v.store1.Get(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) GetMutable1() *T1 {
	return v.store1.GetMutable(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) GetStatic1() T1 {
	return v.store1.GetStatic(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) Get2() *T2 {
	return v.store2.Get(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) GetMutable2() *T2 {
	return v.store2.GetMutable(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) GetStatic2() T2 {
	return v.store2.GetStatic(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) Get3() *T3 {
	return v.store3.Get(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) GetMutable3() *T3 {
	return v.store3.GetMutable(v.ids[v.cursor])
}

func (v *View3[T1, T2, T3]) GetStatic3() T3 {
	return v.store3.GetStatic(v.ids[v.cursor])
}

func sortStores(stores ...TypedStore) []TypedStore {
	if len(stores) == 0 {
		return nil
	}

	sort.Slice(stores, func(i, j int) bool {
		return len(stores[i].GetEntities()) < len(stores[j].GetEntities())
	})

	return stores
}

func Query1[T any](w *World) *View1[T] {
	s, ok := getStoreFromWorld[T](w)
	if !ok {
		return &View1[T]{}
	}

	v := &View1[T]{
		baseView: newBaseView(s.GetEntities()),
		store:    s,
	}

	return v
}

func Query2[T1, T2 any](w *World) *View2[T1, T2] {
	s1, ok1 := getStoreFromWorld[T1](w)
	s2, ok2 := getStoreFromWorld[T2](w)

	if !(ok1 && ok2) {
		return &View2[T1, T2]{}
	}

	v := &View2[T1, T2]{
		baseView: newBaseView([]EntityID{}),
		store1:   s1,
		store2:   s2,
	}

	sorted := sortStores(s1, s2)

	for _, id := range sorted[0].GetEntities() {
		if sorted[1].HasEntity(id) {
			v.ids = append(v.ids, id)
		}
	}

	return v
}

func Query3[T1, T2, T3 any](w *World) *View3[T1, T2, T3] {
	s1, ok1 := getStoreFromWorld[T1](w)
	s2, ok2 := getStoreFromWorld[T2](w)
	s3, ok3 := getStoreFromWorld[T3](w)

	if !(ok1 && ok2 && ok3) {
		return &View3[T1, T2, T3]{}
	}

	v := &View3[T1, T2, T3]{
		baseView: newBaseView([]EntityID{}),
		store1:   s1,
		store2:   s2,
		store3:   s3,
	}

	sorted := sortStores(s1, s2, s3)

	for _, id := range sorted[0].GetEntities() {
		if sorted[1].HasEntity(id) && sorted[2].HasEntity(id) {
			v.ids = append(v.ids, id)
		}
	}

	return v
}
