package geom

import "math"

type Vector2D struct{ X, Y float64 }

func (v Vector2D) Add(o Vector2D) Vector2D {
	return Vector2D{v.X + o.X, v.Y + o.Y}
}

func (v Vector2D) Sub(o Vector2D) Vector2D {
	return Vector2D{v.X - o.X, v.Y - o.Y}
}

func (v Vector2D) Dot(o Vector2D) float64 {
	return v.X*o.X + v.Y*o.Y
}

func (v Vector2D) Length() float64 {
	return math.Hypot(v.X, v.Y)
}

func (v Vector2D) Normalize() Vector2D {
	len := v.Length()
	if len == 0 {
		return Vector2D{0, 0}
	}
	return Vector2D{v.X / len, v.Y / len}
}

func (v Vector2D) Perpendicular() Vector2D {
	return Vector2D{-v.Y, v.X}
}

func (v Vector2D) Scale(s float64) Vector2D {
	return Vector2D{v.X * s, v.Y * s}
}
