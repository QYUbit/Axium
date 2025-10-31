package lib

import (
	"container/list"
)

// Queue represents a generic FIFO data structure using a linked list
type Queue[T any] struct {
	items *list.List
}

// NewQueue creates a new empty queue
func NewQueue[T any]() *Queue[T] {
	return &Queue[T]{
		items: list.New(),
	}
}

// Push adds an item to the rear of the queue
func (q *Queue[T]) Push(item T) {
	q.items.PushBack(item)
}

// Pop removes and returns the front item
func (q *Queue[T]) Pop() T {
	var zero T
	if q.IsEmpty() {
		return zero
	}
	front := q.items.Front()
	item := front.Value.(T)
	q.items.Remove(front)
	return item
}

// Peek returns the front item without removing it
func (q *Queue[T]) Peek() T {
	var zero T
	if q.IsEmpty() {
		return zero
	}
	return q.items.Front().Value.(T)
}

// IsEmpty checks if the queue is empty
func (q *Queue[T]) IsEmpty() bool {
	return q.items.Len() == 0
}

// Size returns the number of items in the queue
func (q *Queue[T]) Size() int {
	return q.items.Len()
}

// Clear removes all items from the queue
func (q *Queue[T]) Clear() {
	q.items.Init()
}
