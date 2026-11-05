package path

import "fmt"

// clonePath reconstructs authored contours without materializing their
// implicit closing lines, and gives the caller independent curve slices.
func (m *ArcLength) clonePath() Path {
	var p Path
	if m == nil {
		return p
	}
	p.Contours = make([]Contour, len(m.contours))
	for i, meta := range m.contours {
		p.Contours[i] = m.cloneContour(meta)
	}
	return p
}
func (m *ArcLength) cloneContour(meta measuredContour) Contour {
	c := Contour{Start: meta.start, Closed: meta.closed, Curves: make([]Cubic, meta.explicit)}
	for i := range c.Curves {
		c.Curves[i] = m.curves[meta.first+i].curve
	}
	return c
}

// TrimByLength returns an independent path restricted to the absolute distance
// interval [start,end]. It preserves cubics by de Casteljau splitting, never
// connects separate contours, and retains Closed only for whole contours.
// Reversed, wrapping and out-of-range intervals are errors. An empty interval
// returns an empty path, including on a stationary path.
func (m *ArcLength) TrimByLength(start, end float64) (Path, error) {
	if err := checkLengthRange(start, end, m.Length()); err != nil {
		return Path{}, err
	}
	if start == end {
		return Path{}, nil
	}
	if start == 0 && end == m.Length() {
		return m.clonePath(), nil
	}
	var out Path
	for _, meta := range m.contours {
		if meta.total <= start || meta.startLength >= end {
			continue
		}
		if start <= meta.startLength && end >= meta.total {
			out.Contours = append(out.Contours, m.cloneContour(meta))
			continue
		}
		var contour Contour
		for i := meta.first; i < meta.first+meta.count; i++ {
			s := &m.curves[i]
			if s.end <= start || s.start >= end || s.length == 0 {
				continue
			}
			a, b := 0.0, 1.0
			if start > s.start {
				var err error
				a, err = m.parameterForCurve(s, start-s.start)
				if err != nil {
					return Path{}, err
				}
			}
			if end < s.end {
				var err error
				b, err = m.parameterForCurve(s, end-s.start)
				if err != nil {
					return Path{}, err
				}
			}
			q := trimLengthCubic(s.curve, a, b)
			if len(contour.Curves) == 0 {
				contour.Start = q.P0
			}
			contour.Curves = append(contour.Curves, q)
		}
		if len(contour.Curves) > 0 {
			out.Contours = append(out.Contours, contour)
		}
	}
	return out, nil
}

func trimLengthCubic(c Cubic, start, end float64) Cubic {
	if start == end {
		p := c.At(start)
		return Cubic{p, p, p, p}
	}
	if end < 1 {
		c, _ = c.Split(end)
	}
	if start > 0 {
		_, c = c.Split(start / end)
	}
	return c
}

// PartialByLength returns a prefix at normalized arc length alpha in [0,1].
// Unlike Partial, it measures distance rather than counting cubic parameters;
// Partial's API and behavior are unchanged. Alpha zero is empty, alpha one is
// an independent copy, including point-only/constant contours.
func (m *ArcLength) PartialByLength(alpha float64) (Path, error) {
	if !lengthFinite(alpha) || alpha < 0 || alpha > 1 {
		return Path{}, fmt.Errorf("arc length fraction must be finite and within [0,1]")
	}
	if alpha == 1 {
		return m.clonePath(), nil
	}
	return m.TrimByLength(0, alpha*m.Length())
}
func (p Path) PartialByLength(alpha, tolerance float64) (Path, error) {
	m, err := p.Measure(tolerance)
	if err != nil {
		return Path{}, err
	}
	return m.PartialByLength(alpha)
}
func (p Path) TrimByLength(start, end, tolerance float64) (Path, error) {
	m, err := p.Measure(tolerance)
	if err != nil {
		return Path{}, err
	}
	return m.TrimByLength(start, end)
}
