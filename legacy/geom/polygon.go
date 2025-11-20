package geom

import "math"

type Polygon []Vector2D

func (p Polygon) Center() Vector2D {
	var sum Vector2D
	for _, v := range p {
		sum = sum.Add(v)
	}
	return sum.Scale(1 / float64(len(p)))
}

func (p Polygon) Project(axis Vector2D) (float64, float64) {
	min := math.Inf(1)
	max := math.Inf(-1)

	for _, v := range p {
		dot := v.Dot(axis)
		min = math.Min(min, dot)
		max = math.Max(max, dot)
	}
	return min, max
}

func DoPolygonsIntersect(p1, p2 Polygon) bool {
	if len(p1) == 0 || len(p2) == 0 {
		return false
	}

	for _, p := range [2]Polygon{p1, p2} {
		for i := range p {
			j := (i + 1) % len(p)

			proj1 := p[i]
			proj2 := p[j]

			normal := Vector2D{proj2.Y - proj1.Y, proj1.X - proj2.X}

			min1, max1 := p1.Project(normal)
			min2, max2 := p2.Project(normal)

			if !overlaps(min1, max1, min2, max2) {
				return false
			}
		}
	}
	return true
}

func MinimumTranslation(p1, p2 Polygon) (Vector2D, bool) {
	minOverlap := math.Inf(1)
	var mtvAxis Vector2D

	for _, poly := range []Polygon{p1, p2} {
		for i := range poly {
			a := poly[i]
			b := poly[(i+1)%len(poly)]
			axis := b.Sub(a).Perpendicular().Normalize()

			min1, max1 := p1.Project(axis)
			min2, max2 := p2.Project(axis)
			if !overlaps(min1, max1, min2, max2) {
				return Vector2D{}, false
			}

			overlap := math.Min(max1, max2) - math.Max(min1, min2)
			if overlap < minOverlap {
				minOverlap = overlap
				mtvAxis = axis

				if p1.Center().Sub(p2.Center()).Dot(mtvAxis) < 0 {
					mtvAxis = mtvAxis.Scale(-1)
				}
			}
		}
	}
	return mtvAxis.Scale(minOverlap), true
}

func overlaps(min1, max1, min2, max2 float64) bool {
	return !(max1 < min2 || max2 < min1)
}
