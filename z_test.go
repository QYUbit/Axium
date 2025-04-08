package axium

import (
	"fmt"
	"testing"
	"time"
)

func TestStuff(t *testing.T) {
	v := NewVector2D(3.4563535, 0.1+0.2)
	fmt.Println(v)
}

func TestMM(t *testing.T) {
	game, err := NewGame(&Config{addres: "localhost:8080", tickInterval: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}

	game.OnTick(func() {

	})
}
