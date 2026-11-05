package path

import (
	"fmt"
	"math"
)

// TrimByParameter extracts a normalized explicit-cubic interval, with equal
// parameter spans per stored cubic, independent of its geometric length. It
// uses exact de Casteljau splitting, preserves contour gaps and only retains
// Closed for a complete authored contour. An implicit closing edge is included
// only when that entire contour is retained. Point-only contours survive a full
// [0,1] copy; a zero-width interval is empty. This is not arc-length trimming.
func (p Path) TrimByParameter(start, end float64) (Path, error) {
	if !lengthFinite(start) || !lengthFinite(end) || start < 0 || end < start || end > 1 {
		return Path{}, fmt.Errorf("parameter interval must satisfy 0 <= start <= end <= 1")
	}
	if err := p.Validate(); err != nil {
		return Path{}, err
	}
	if start == end {
		return Path{}, nil
	}
	if start == 0 && end == 1 {
		return p.Clone(), nil
	}
	n := 0
	for _, c := range p.Contours {
		n += len(c.Curves)
	}
	lo, hi := start*float64(n), end*float64(n)
	var out Path
	index := 0
	for _, c := range p.Contours {
		begin := index
		q := Contour{}
		for _, curve := range c.Curves {
			a, b := math.Max(0, lo-float64(index)), math.Min(1, hi-float64(index))
			index++
			if b <= a {
				continue
			}
			v := trimLengthCubic(curve, a, b)
			if len(q.Curves) == 0 {
				q.Start = v.P0
			}
			q.Curves = append(q.Curves, v)
		}
		if len(q.Curves) > 0 {
			q.Closed = c.Closed && lo <= float64(begin) && hi >= float64(index)
			out.Contours = append(out.Contours, q)
		}
	}
	return out, nil
}
