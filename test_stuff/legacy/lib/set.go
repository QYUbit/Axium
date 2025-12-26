package lib

// Queue represents a generic set data structure using a hash map.
type Set[T comparable] struct {
	items map[T]bool
}

// NewSet returns a new empty set.
func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		items: map[T]bool{},
	}
}

// Add adds item to the set. If item is already in s, Add is a no-op.
func (s *Set[T]) Add(item T) {
	s.items[item] = true
}

// Remove removes item from s. If item is not in s, Remove is a no-op.
func (s *Set[T]) Remove(item T) {
	delete(s.items, item)
}

// Has reports whether item is in the set s.
func (s *Set[T]) Has(item T) bool {
	_, has := s.items[item]
	return has
}

// Slice returns a slice representation of s.
func Slice[T comparable](s Set[T]) []T {
	var slice []T
	for item := range s.items {
		slice = append(slice, item)
	}
	return slice
}
