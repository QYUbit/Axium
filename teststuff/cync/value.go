package cync

// Miniatur Konzept Beispiel
type Value[T any] struct {
	Val   T // read only
	dirty bool
}

func (v *Value[T]) Set(new T) {
	v.Val = new
	v.dirty = true
}

func (v *Value[T]) IsDirty() bool {
	return v.dirty
}
