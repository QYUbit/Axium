package state

import (
	"fmt"
	"testing"
)

type health struct {
	max, current int
}

func (h health) Serialize() ([]byte, error) {
	return fmt.Appendf(nil, "max: %d, current %d", h.max, h.current), nil
}

func TestState(t *testing.T) {
	w := NewWorld()

	delta1 := w.BuildDelta()
	fmt.Println(delta1)

	player := w.CreateEntity()

	w.SetComponent(player, "health", health{10, 10})
	w.RemoveComponent(player, "health")
	w.DestroyEntity(player)

	delta2 := w.BuildDelta()
	fmt.Println(delta2)
}
