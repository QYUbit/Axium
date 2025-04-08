package axium

import "fmt"

type Vector2D struct {
	X float64
	Y float64
}

func NewVector2D(x float64, y float64) Vector2D {
	return Vector2D{X: x, Y: y}
}

func (v Vector2D) String() string {
	return fmt.Sprintf("{%.2f, %.2f}", v.X, v.Y)
}
