// Package svg imports vector SVG geometry without a browser or native library.
package svg

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
	"strconv"
	"strings"
)

type scanner struct {
	data string
	pos  int
}

func (s *scanner) space() {
	for s.pos < len(s.data) {
		switch s.data[s.pos] {
		case ' ', '\t', '\n', '\r', ',':
			s.pos++
		default:
			return
		}
	}
}
func (s *scanner) number() (float64, error) {
	s.space()
	start := s.pos
	if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
		s.pos++
	}
	digits := 0
	for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
		s.pos++
		digits++
	}
	if s.pos < len(s.data) && s.data[s.pos] == '.' {
		s.pos++
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
			digits++
		}
	}
	if digits == 0 {
		return 0, fmt.Errorf("SVG number expected at byte %d", start)
	}
	if s.pos < len(s.data) && (s.data[s.pos] == 'e' || s.data[s.pos] == 'E') {
		s.pos++
		if s.pos < len(s.data) && (s.data[s.pos] == '+' || s.data[s.pos] == '-') {
			s.pos++
		}
		exponent := s.pos
		for s.pos < len(s.data) && s.data[s.pos] >= '0' && s.data[s.pos] <= '9' {
			s.pos++
		}
		if exponent == s.pos {
			return 0, fmt.Errorf("invalid SVG exponent at byte %d", start)
		}
	}
	v, e := strconv.ParseFloat(s.data[start:s.pos], 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid SVG number at byte %d", start)
	}
	return v, nil
}
func (s *scanner) flag() (bool, error) {
	s.space()
	if s.pos >= len(s.data) || (s.data[s.pos] != '0' && s.data[s.pos] != '1') {
		return false, fmt.Errorf("SVG arc flag must be 0 or 1 at byte %d", s.pos)
	}
	v := s.data[s.pos] == '1'
	s.pos++
	return v, nil
}
func letter(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' }

// ParsePath supports all standard SVG path commands, including elliptical arcs.
// Arcs are approximated by cubic segments spanning no more than 90 degrees.
func ParsePath(data string) (path.Path, error) {
	s := scanner{data: data}
	var b path.Builder
	var cursor, start, control geom.Vec3
	var previous, command byte
	haveMove := false
	for {
		s.space()
		if s.pos == len(s.data) {
			break
		}
		if letter(s.data[s.pos]) {
			command = s.data[s.pos]
			s.pos++
		} else if command == 0 {
			return path.Path{}, fmt.Errorf("SVG path command expected at byte %d", s.pos)
		}
		absolute := command >= 'A' && command <= 'Z'
		upper := command
		if !absolute {
			upper -= 32
		}
		if !haveMove && upper != 'M' {
			return path.Path{}, fmt.Errorf("SVG path must start with moveto")
		}
		if previous == 'Z' && upper != 'M' && upper != 'Z' {
			b.MoveTo(cursor)
			start = cursor
		}
		number := func() (float64, error) { return s.number() }
		point := func() (geom.Vec3, error) {
			x, e := number()
			if e != nil {
				return geom.Vec3{}, e
			}
			y, e := number()
			if e != nil {
				return geom.Vec3{}, e
			}
			p := geom.V(x, y)
			if !absolute {
				p = p.Add(cursor)
			}
			return p, nil
		}
		switch upper {
		case 'M':
			p, e := point()
			if e != nil {
				return path.Path{}, e
			}
			b.MoveTo(p)
			cursor, start, control = p, p, p
			haveMove = true
			if absolute {
				command = 'L'
			} else {
				command = 'l'
			}
		case 'L':
			p, e := point()
			if e != nil {
				return path.Path{}, e
			}
			b.LineTo(p)
			cursor, control = p, p
		case 'H':
			v, e := number()
			if e != nil {
				return path.Path{}, e
			}
			if !absolute {
				v += cursor.X
			}
			cursor.X = v
			b.LineTo(cursor)
			control = cursor
		case 'V':
			v, e := number()
			if e != nil {
				return path.Path{}, e
			}
			if !absolute {
				v += cursor.Y
			}
			cursor.Y = v
			b.LineTo(cursor)
			control = cursor
		case 'C':
			a, e := point()
			if e != nil {
				return path.Path{}, e
			}
			c, e := point()
			if e != nil {
				return path.Path{}, e
			}
			p, e := point()
			if e != nil {
				return path.Path{}, e
			}
			b.CubicTo(a, c, p)
			cursor, control = p, c
		case 'S':
			a := cursor
			if previous == 'C' || previous == 'S' {
				a = cursor.Mul(2).Sub(control)
			}
			c, e := point()
			if e != nil {
				return path.Path{}, e
			}
			p, e := point()
			if e != nil {
				return path.Path{}, e
			}
			b.CubicTo(a, c, p)
			cursor, control = p, c
		case 'Q':
			a, e := point()
			if e != nil {
				return path.Path{}, e
			}
			p, e := point()
			if e != nil {
				return path.Path{}, e
			}
			b.QuadTo(a, p)
			cursor, control = p, a
		case 'T':
			a := cursor
			if previous == 'Q' || previous == 'T' {
				a = cursor.Mul(2).Sub(control)
			}
			p, e := point()
			if e != nil {
				return path.Path{}, e
			}
			b.QuadTo(a, p)
			cursor, control = p, a
		case 'A':
			rx, e := number()
			if e != nil {
				return path.Path{}, e
			}
			ry, e := number()
			if e != nil {
				return path.Path{}, e
			}
			angle, e := number()
			if e != nil {
				return path.Path{}, e
			}
			large, e := s.flag()
			if e != nil {
				return path.Path{}, e
			}
			sweep, e := s.flag()
			if e != nil {
				return path.Path{}, e
			}
			p, e := point()
			if e != nil {
				return path.Path{}, e
			}
			arcTo(&b, cursor, p, rx, ry, angle, large, sweep)
			cursor, control = p, p
		case 'Z':
			b.Close()
			cursor, control = start, start
			command = 0
		default:
			return path.Path{}, fmt.Errorf("unsupported SVG path command %q", command)
		}
		previous = upper
	}
	p := b.Path()
	if e := p.Validate(); e != nil {
		return path.Path{}, e
	}
	return p, nil
}

// Endpoint-to-center conversion follows SVG 2 Appendix B.2.
// https://www.w3.org/TR/SVG/implnote.html#ArcImplementationNotes
func arcTo(b *path.Builder, from, to geom.Vec3, rx, ry, rotation float64, large, sweep bool) {
	if from.Sub(to).Norm() < 1e-14 {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 {
		b.LineTo(to)
		return
	}
	phi := rotation * math.Pi / 180
	c, s := math.Cos(phi), math.Sin(phi)
	dx, dy := (from.X-to.X)/2, (from.Y-to.Y)/2
	x, y := c*dx+s*dy, -s*dx+c*dy
	lambda := x*x/(rx*rx) + y*y/(ry*ry)
	if lambda > 1 {
		v := math.Sqrt(lambda)
		rx *= v
		ry *= v
	}
	num := rx*rx*ry*ry - rx*rx*y*y - ry*ry*x*x
	den := rx*rx*y*y + ry*ry*x*x
	factor := math.Sqrt(math.Max(0, num/den))
	if large == sweep {
		factor = -factor
	}
	cxp, cyp := factor*rx*y/ry, -factor*ry*x/rx
	cx, cy := c*cxp-s*cyp+(from.X+to.X)/2, s*cxp+c*cyp+(from.Y+to.Y)/2
	ux, uy := (x-cxp)/rx, (y-cyp)/ry
	vx, vy := (-x-cxp)/rx, (-y-cyp)/ry
	start := math.Atan2(uy, ux)
	delta := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	if sweep && delta < 0 {
		delta += 2 * math.Pi
	}
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	}
	count := max(1, int(math.Ceil(math.Abs(delta)/(math.Pi/2))))
	step := delta / float64(count)
	point := func(a float64) geom.Vec3 {
		return geom.V(cx+rx*math.Cos(a)*c-ry*math.Sin(a)*s, cy+rx*math.Cos(a)*s+ry*math.Sin(a)*c)
	}
	derivative := func(a float64) geom.Vec3 {
		return geom.V(-rx*math.Sin(a)*c-ry*math.Cos(a)*s, -rx*math.Sin(a)*s+ry*math.Cos(a)*c)
	}
	for i := range count {
		a, z := start+float64(i)*step, start+float64(i+1)*step
		k := 4.0 / 3 * math.Tan(step/4)
		end := point(z)
		if i == count-1 {
			end = to
		}
		b.CubicTo(point(a).Add(derivative(a).Mul(k)), point(z).Sub(derivative(z).Mul(k)), end)
	}
}

func transform(value string) (geom.Mat4, error) {
	m := geom.Identity()
	s := scanner{data: strings.TrimSpace(value)}
	for {
		s.space()
		if s.pos == len(s.data) {
			return m, nil
		}
		start := s.pos
		for s.pos < len(s.data) && letter(s.data[s.pos]) {
			s.pos++
		}
		name := s.data[start:s.pos]
		s.space()
		if s.pos >= len(s.data) || s.data[s.pos] != '(' {
			return m, fmt.Errorf("invalid SVG transform")
		}
		s.pos++
		var args []float64
		for {
			s.space()
			if s.pos >= len(s.data) {
				return m, fmt.Errorf("unclosed SVG transform")
			}
			if s.data[s.pos] == ')' {
				s.pos++
				break
			}
			v, e := s.number()
			if e != nil {
				return m, e
			}
			args = append(args, v)
			if len(args) > 6 {
				return m, fmt.Errorf("too many transform arguments")
			}
		}
		local := geom.Identity()
		switch name {
		case "matrix":
			if len(args) != 6 {
				return m, fmt.Errorf("matrix requires six values")
			}
			local = geom.Mat4{args[0], args[2], 0, args[4], args[1], args[3], 0, args[5], 0, 0, 1, 0, 0, 0, 0, 1}
		case "translate":
			if len(args) < 1 || len(args) > 2 {
				return m, fmt.Errorf("translate requires one or two values")
			}
			y := 0.0
			if len(args) == 2 {
				y = args[1]
			}
			local = geom.Translation(geom.V(args[0], y))
		case "scale":
			if len(args) < 1 || len(args) > 2 {
				return m, fmt.Errorf("scale requires one or two values")
			}
			y := args[0]
			if len(args) == 2 {
				y = args[1]
			}
			local = geom.Scaling(args[0], y, 1)
		case "rotate":
			if len(args) != 1 && len(args) != 3 {
				return m, fmt.Errorf("rotate requires one or three values")
			}
			local = geom.RotationZ(args[0] * math.Pi / 180)
			if len(args) == 3 {
				local = geom.About(geom.V(args[1], args[2]), local)
			}
		case "skewX", "skewY":
			if len(args) != 1 {
				return m, fmt.Errorf("skew requires one value")
			}
			if name == "skewX" {
				local[1] = math.Tan(args[0] * math.Pi / 180)
			} else {
				local[4] = math.Tan(args[0] * math.Pi / 180)
			}
		default:
			return m, fmt.Errorf("unsupported transform %q", name)
		}
		m = m.Mul(local)
	}
}
