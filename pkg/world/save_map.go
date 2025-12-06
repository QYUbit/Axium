package main

import (
	"iter"
	"maps"
	"sync"
)

type SaveMap[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

func NewMap[K comparable, V any]() *SaveMap[K, V] {
	return &SaveMap[K, V]{
		m: make(map[K]V),
	}
}

func NewMapWithSize[K comparable, V any](size int) *SaveMap[K, V] {
	return &SaveMap[K, V]{
		m: make(map[K]V, size),
	}
}

func (sm *SaveMap[K, V]) Set(key K, value V) {
	sm.mu.Lock()
	sm.m[key] = value
	sm.mu.Unlock()
}

func (sm *SaveMap[K, V]) Get(key K) (V, bool) {
	sm.mu.RLock()
	value, ok := sm.m[key]
	sm.mu.RUnlock()
	return value, ok
}

func (sm *SaveMap[K, V]) Delete(key K) {
	sm.mu.Lock()
	delete(sm.m, key)
	sm.mu.Unlock()
}

func (sm *SaveMap[K, V]) Keys() iter.Seq[K] {
	sm.mu.RLock()
	mapClone := maps.Clone(sm.m)
	sm.mu.RUnlock()
	return maps.Keys(mapClone)
}

func (sm *SaveMap[K, V]) Values() iter.Seq[V] {
	sm.mu.RLock()
	mapClone := maps.Clone(sm.m)
	sm.mu.RUnlock()
	return maps.Values(mapClone)
}

func (sm *SaveMap[K, V]) All() iter.Seq2[K, V] {
	sm.mu.RLock()
	mapClone := maps.Clone(sm.m)
	sm.mu.RUnlock()
	return maps.All(mapClone)
}
