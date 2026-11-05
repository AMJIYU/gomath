package render

import (
	"fmt"
	"golang.org/x/image/vector"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"sort"
)

// strokeLine builds a single outline instead of overlapping segment rectangles.
// This avoids excessive antialias coverage at joins on subpixel-width strokes.
// Joins are mitered with a limit of four radii; caps are round.
func (r *Rasterizer) strokeLine(z *vector.Rasterizer, line path.Polyline, radius float64) {
	_ = r.strokeStyledLine(z, line, radius, gomath.Style{})
}

// strokeStyledLine builds a single outline with the supplied cap and join.
// The caller must propagate errors and pad its ROI with Style.StrokeExtent().
// Radius is already in pixels; Style.StrokeWidth remains in world units.
func (r *Rasterizer) strokeStyledLine(z *vector.Rasterizer, line path.Polyline, radius float64, style gomath.Style) error {
	if err := style.Validate(); err != nil {
		return err
	}
	if !finite(radius) || radius < 0 {
		return fmt.Errorf("stroke radius must be finite and nonnegative")
	}
	if z == nil {
		return fmt.Errorf("nil stroke rasterizer")
	}
	if len(line.Points) == 0 || radius == 0 {
		return nil
	}
	if style.Dash != nil {
		pieces, err := dashLines([]path.Polyline{line}, style.Dash)
		if err != nil {
			return err
		}
		style.Dash = nil
		for _, piece := range pieces {
			if err := r.strokeDashPiece(z, piece, radius, style); err != nil {
				return err
			}
		}
		return nil
	}
	points := make([][2]float64, 0, len(line.Points))
	for _, p := range line.Points {
		x, y := r.point(p)
		if !finite(float64(x)) || !finite(float64(y)) {
			return fmt.Errorf("stroke point is not finite in pixel space")
		}
		v := [2]float64{float64(x), float64(y)}
		if len(points) == 0 || math.Hypot(v[0]-points[len(points)-1][0], v[1]-points[len(points)-1][1]) > 1e-7 {
			points = append(points, v)
		}
	}
	if len(points) > 1 && line.Closed && math.Hypot(points[0][0]-points[len(points)-1][0], points[0][1]-points[len(points)-1][1]) < 1e-7 {
		points = points[:len(points)-1]
	}
	if len(points) < 2 {
		p := points[0]
		switch style.StrokeCap {
		case gomath.CapRound:
			outline := [][2]float64{{p[0] + radius, p[1]}}
			outline = appendCap(outline, p, 0, radius)
			outline = appendCap(outline, p, -math.Pi, radius)
			drawStrokeContour(z, outline)
		case gomath.CapSquare:
			drawStrokeContour(z, [][2]float64{{p[0] - radius, p[1] - radius}, {p[0] - radius, p[1] + radius}, {p[0] + radius, p[1] + radius}, {p[0] + radius, p[1] - radius}})
		}
		return nil
	}
	left := offsetStyledSide(points, radius, 1, line.Closed, style)
	right := offsetStyledSide(points, radius, -1, line.Closed, style)
	if line.Closed {
		drawStrokeContour(z, left)
		reversePoints(right)
		drawStrokeContour(z, right)
		return nil
	}
	if style.StrokeCap == gomath.CapSquare {
		// Extend both sides by one radius in the endpoint's tangent direction.
		for _, end := range []int{0, len(points) - 1} {
			next := 1
			index := 0
			if end != 0 {
				next = end - 1
				index = len(left) - 1
			}
			dx, dy := points[end][0]-points[next][0], points[end][1]-points[next][1]
			factor := radius / math.Hypot(dx, dy)
			left[index][0] += dx * factor
			left[index][1] += dy * factor
			index = 0
			if end != 0 {
				index = len(right) - 1
			}
			right[index][0] += dx * factor
			right[index][1] += dy * factor
		}
	}
	outline := append([][2]float64(nil), left...)
	end := points[len(points)-1]
	last := left[len(left)-1]
	if style.StrokeCap == gomath.CapRound {
		outline = appendCap(outline, end, math.Atan2(last[1]-end[1], last[0]-end[0]), radius)
	}
	reversePoints(right)
	outline = append(outline, right...)
	first := points[0]
	last = right[len(right)-1]
	if style.StrokeCap == gomath.CapRound {
		outline = appendCap(outline, first, math.Atan2(last[1]-first[1], last[0]-first[0]), radius)
	}
	drawStrokeContour(z, outline)
	return nil
}
func drawStrokeContour(z *vector.Rasterizer, ps [][2]float64) {
	if len(ps) == 0 {
		return
	}
	z.MoveTo(float32(ps[0][0]), float32(ps[0][1]))
	for _, p := range ps[1:] {
		z.LineTo(float32(p[0]), float32(p[1]))
	}
	z.ClosePath()
}
func reversePoints(p [][2]float64) {
	for i, j := 0, len(p)-1; i < j; i, j = i+1, j-1 {
		p[i], p[j] = p[j], p[i]
	}
}
func appendCap(dst [][2]float64, center [2]float64, start, radius float64) [][2]float64 {
	n := max(4, min(128, int(math.Ceil(math.Pi*math.Sqrt(radius/.2)))))
	for i := 1; i <= n; i++ {
		a := start - math.Pi*float64(i)/float64(n)
		dst = append(dst, [2]float64{center[0] + radius*math.Cos(a), center[1] + radius*math.Sin(a)})
	}
	return dst
}
func offsetSide(points [][2]float64, radius, side float64, closed bool) [][2]float64 {
	return offsetStyledSide(points, radius, side, closed, gomath.Style{})
}

func offsetStyledSide(points [][2]float64, radius, side float64, closed bool, style gomath.Style) [][2]float64 {
	n := len(points)
	out := make([][2]float64, 0, n)
	normal := func(a, b [2]float64) [2]float64 {
		dx, dy := b[0]-a[0], b[1]-a[1]
		d := math.Hypot(dx, dy)
		return [2]float64{-dy / d * side, dx / d * side}
	}
	for i, p := range points {
		var a, b [2]float64
		if !closed && i == 0 {
			a = normal(p, points[1])
			b = a
		} else if !closed && i == n-1 {
			a = normal(points[n-2], p)
			b = a
		} else {
			a = normal(points[(i+n-1)%n], p)
			b = normal(p, points[(i+1)%n])
		}
		sx, sy := a[0]+b[0], a[1]+b[1]
		den := sx*b[0] + sy*b[1]
		if den <= 1e-8 {
			// Near a reversal, 1+dot(a,b) loses precision. The equivalent
			// half squared sum keeps large configured miter limits usable.
			den = (sx*sx + sy*sy) / 2
		}
		cross := a[0]*b[1] - a[1]*b[0]
		outer := cross*side < 0
		if !outer && den > 0 {
			// The inside intersection must stay on both adjacent segments;
			// otherwise use the center to avoid spikes on short sharp turns.
			distance := radius * math.Abs(cross) / den
			prev, next := points[(i+n-1)%n], points[(i+1)%n]
			ax, ay := p[0]-prev[0], p[1]-prev[1]
			bx, by := p[0]-next[0], p[1]-next[1]
			if distance*distance <= ax*ax+ay*ay && distance*distance <= bx*bx+by*by {
				out = append(out, [2]float64{p[0] + radius*sx/den, p[1] + radius*sy/den})
			} else if cross == 0 {
				out = append(out, [2]float64{p[0] + radius*a[0], p[1] + radius*a[1]})
			} else {
				out = append(out, [2]float64{p[0] + radius*a[0], p[1] + radius*a[1]}, p, [2]float64{p[0] + radius*b[0], p[1] + radius*b[1]})
			}
		} else if style.StrokeJoin == gomath.JoinMiter && den > 0 && math.Hypot(sx, sy)/den <= style.EffectiveMiterLimit() {
			// Keep the original miter arithmetic so default subpixel outlines
			// retain their coverage. The limit only clips the outside join.
			out = append(out, [2]float64{p[0] + radius*sx/den, p[1] + radius*sy/den})
		} else if style.StrokeJoin == gomath.JoinRound && (outer || den == 0) {
			out = append(out, [2]float64{p[0] + radius*a[0], p[1] + radius*a[1]})
			angle := math.Atan2(cross, a[0]*b[0]+a[1]*b[1])
			if den == 0 {
				angle = -side * math.Pi
			}
			out = appendJoinArc(out, p, math.Atan2(a[1], a[0]), angle, radius)
		} else {
			out = append(out, [2]float64{p[0] + radius*a[0], p[1] + radius*a[1]}, [2]float64{p[0] + radius*b[0], p[1] + radius*b[1]})
		}
	}
	return out
}

func appendJoinArc(dst [][2]float64, center [2]float64, start, sweep, radius float64) [][2]float64 {
	// Match cap tessellation density (at most 0.2px chord error at ordinary
	// sizes), including the final endpoint at the next segment's normal.
	n := max(1, min(128, int(math.Ceil(math.Abs(sweep)*math.Sqrt(radius/.2)))))
	for i := 1; i <= n; i++ {
		a := start + sweep*float64(i)/float64(n)
		dst = append(dst, [2]float64{center[0] + radius*math.Cos(a), center[1] + radius*math.Sin(a)})
	}
	return dst
}

// Dashing is bounded per painted object, across all flattened contours. Count
// zero-length entries and gap transitions too, so invisible patterns cannot
// evade the work cap. Errors abort rendering rather than dropping dashes.
const (
	maxDashFragments = 65536
	maxDashSteps     = 262144
	maxDashPoints    = 1048576
)

type dashSpan struct{ start, end float64 }

type dashPiece struct {
	path.Polyline
	tangent geom.Vec3 // zero-length dash cap direction, in projected world XY
}
type dashBudget struct{ fragments, steps, points int }

func dashLines(lines []path.Polyline, pattern *gomath.DashPattern) ([]dashPiece, error) {
	if pattern == nil {
		pieces := make([]dashPiece, len(lines))
		for i, line := range lines {
			pieces[i].Polyline = line
		}
		return pieces, nil
	}
	if err := pattern.Validate(); err != nil {
		return nil, err
	}
	lengths := pattern.Lengths()
	budget := dashBudget{}
	var result []dashPiece
	for _, line := range lines {
		pieces, err := dashContour(line, lengths, pattern.Offset(), &budget)
		if err != nil {
			return nil, err
		}
		result = append(result, pieces...)
	}
	return result, nil
}

func dashContour(line path.Polyline, lengths []float64, phase float64, budget *dashBudget) ([]dashPiece, error) {
	if len(line.Points) == 0 {
		return nil, nil
	}
	points := make([]geom.Vec3, 0, len(line.Points)+1)
	distances := make([]float64, 0, len(line.Points)+1)
	total := 0.0
	add := func(p geom.Vec3) error {
		if !p.Finite() {
			return fmt.Errorf("dash path point must be finite")
		}
		if len(points) > 0 {
			prev := points[len(points)-1]
			distance := math.Hypot(p.X-prev.X, p.Y-prev.Y)
			if distance == 0 {
				return nil
			}
			next := total + distance
			if next <= total {
				return fmt.Errorf("dash contour length cannot advance at this numeric scale")
			}
			total = next
			if !finite(total) {
				return fmt.Errorf("dash contour length exceeds numeric range")
			}
		}
		points = append(points, p)
		distances = append(distances, total)
		return nil
	}
	for _, p := range line.Points {
		if err := add(p); err != nil {
			return nil, err
		}
	}
	if line.Closed {
		first, last := points[0], points[len(points)-1]
		// Trigonometric circles may finish a few rounding bits away from
		// their start. Snap an implicit seam within four ULPs of arc length;
		// do not reject a valid ring because that closure cannot advance sum.
		seam := math.Hypot(first.X-last.X, first.Y-last.Y)
		if len(points) > 2 && seam < total/2 && seam <= 4*(total-math.Nextafter(total, 0)) {
			points[len(points)-1] = first
		} else if err := add(first); err != nil {
			return nil, err
		}
	}
	index := 0
	for phase > 0 {
		if phase < lengths[index] {
			break
		}
		phase -= lengths[index]
		index = (index + 1) % len(lengths)
	}
	remaining := lengths[index] - phase
	position := 0.0
	var spans []dashSpan
	emit := func(start, end float64) error {
		budget.fragments++
		if budget.fragments > maxDashFragments {
			return fmt.Errorf("dash fragment budget exceeds %d per object", maxDashFragments)
		}
		// A zero gap does not break a painted run: preserve joins at
		// original vertices instead of manufacturing adjacent endpoint caps.
		if end > start {
			// Zero on entries at this same boundary add no extra cap when
			// another positive run continues through it.
			for len(spans) > 0 {
				last := spans[len(spans)-1]
				if last.start != start || last.end != start {
					break
				}
				spans = spans[:len(spans)-1]
			}
			if len(spans) > 0 {
				last := &spans[len(spans)-1]
				if last.end == start && last.end > last.start {
					last.end = end
					return nil
				}
			}
		}
		spans = append(spans, dashSpan{start, end})
		return nil
	}
	for {
		budget.steps++
		if budget.steps > maxDashSteps {
			return nil, fmt.Errorf("dash subdivision work exceeds %d steps per object", maxDashSteps)
		}
		if remaining == 0 {
			if index%2 == 0 {
				if err := emit(position, position); err != nil {
					return nil, err
				}
			}
			index = (index + 1) % len(lengths)
			remaining = lengths[index]
			continue
		}
		if position >= total {
			if total == 0 && index%2 == 0 && len(spans) == 0 {
				if err := emit(0, 0); err != nil {
					return nil, err
				}
			}
			break
		}
		available := total - position
		step := math.Min(remaining, available)
		end := position + step
		if remaining >= available {
			end = total
		}
		if end <= position {
			return nil, fmt.Errorf("dash subdivision cannot advance at this numeric scale")
		}
		if index%2 == 0 {
			if err := emit(position, end); err != nil {
				return nil, err
			}
		}
		position = end
		if remaining > available {
			break
		}
		index = (index + 1) % len(lengths)
		remaining = lengths[index]
	}
	// A zero on event exactly at the path end is already covered by the
	// preceding dash's identical end cap. Do not rasterize that cap twice.
	lastSpan := len(spans) - 1
	for lastSpan > 0 && spans[lastSpan].start == total && spans[lastSpan].end == total {
		lastSpan--
	}
	if lastSpan >= 0 && lastSpan < len(spans)-1 && spans[lastSpan].end == total && spans[lastSpan].end > spans[lastSpan].start {
		spans = spans[:lastSpan+1]
	}
	at := func(s float64) geom.Vec3 {
		i := sort.SearchFloat64s(distances, s)
		if i == 0 {
			return points[0]
		}
		if i == len(points) {
			return points[len(points)-1]
		}
		return points[i-1].Lerp(points[i], (s-distances[i-1])/(distances[i]-distances[i-1]))
	}
	pieces := make([]dashPiece, 0, len(spans))
	for _, span := range spans {
		piece := dashPiece{Polyline: path.Polyline{Points: []geom.Vec3{at(span.start)}}}
		if span.start == span.end && len(points) > 1 {
			j := sort.Search(len(distances), func(i int) bool { return distances[i] > span.start })
			if j == len(points) {
				j--
			}
			direction := points[j].Sub(points[j-1])
			length := math.Hypot(direction.X, direction.Y)
			piece.tangent = geom.V(direction.X/length, direction.Y/length)
		}
		i := sort.Search(len(distances), func(i int) bool { return distances[i] > span.start })
		for i < len(points) && distances[i] < span.end {
			if budget.points+len(piece.Points)+1 > maxDashPoints {
				return nil, fmt.Errorf("dash point budget exceeds %d per object", maxDashPoints)
			}
			piece.Points = append(piece.Points, points[i])
			i++
		}
		if span.end > span.start {
			piece.Points = append(piece.Points, at(span.end))
		}
		budget.points += len(piece.Points)
		if budget.points > maxDashPoints {
			return nil, fmt.Errorf("dash point budget exceeds %d per object", maxDashPoints)
		}
		pieces = append(pieces, piece)
	}
	// Join through the original seam, preserving that corner's join rather than
	// adding two artificial caps. A dash covering the entire ring stays closed.
	if line.Closed && total > 0 && len(spans) > 0 && spans[0].start == 0 && spans[0].end > 0 {
		last := len(spans) - 1
		for last > 0 && spans[last].start == total && spans[last].end == total {
			last--
		}
		if spans[last].end == total && spans[last].start < total {
			if last == 0 {
				pieces[0].Closed = true
				pieces = pieces[:1]
			} else {
				joined := pieces[last]
				joined.Points = append(joined.Points, pieces[0].Points[1:]...)
				pieces[0] = joined
				pieces = pieces[:last]
			}
		}
	}
	return pieces, nil
}

// A zero-length square dash retains the source tangent; an axis-aligned square
// would be wrong for dots on diagonal/curved paths. Round dots are isotropic.
func (r *Rasterizer) strokeDashPiece(z *vector.Rasterizer, piece dashPiece, radius float64, style gomath.Style) error {
	if len(piece.Points) == 1 && style.StrokeCap == gomath.CapSquare && (piece.tangent.X != 0 || piece.tangent.Y != 0) {
		if z == nil || !finite(radius) || radius < 0 {
			return fmt.Errorf("invalid zero-length dash rasterizer/radius")
		}
		x, y := r.point(piece.Points[0])
		if !finite(float64(x)) || !finite(float64(y)) {
			return fmt.Errorf("dash point exceeds pixel coordinate range")
		}
		tx, ty := piece.tangent.X*radius, -piece.tangent.Y*radius
		nx, ny := -ty, tx
		px, py := float64(x), float64(y)
		drawStrokeContour(z, [][2]float64{{px - tx - nx, py - ty - ny}, {px - tx + nx, py - ty + ny}, {px + tx + nx, py + ty + ny}, {px + tx - nx, py + ty - ny}})
		return nil
	}
	return r.strokeStyledLine(z, piece.Polyline, radius, style)
}
