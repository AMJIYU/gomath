package number

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
	"unicode/utf8"
)

type pointRef struct{ slot, contour, curve, point int }
type display struct {
	root      *gomath.Object
	rootID    uint64
	slots     []*gomath.Object
	ids       []uint64
	canonical []path.Path // Anchored em outlines, independent of mutable Objects.
	basis     [3]pointRef
	opt       Options
	formatted string
}
type appearance struct {
	style  gomath.Style
	z      int
	scalar float64
}
type snapshot struct {
	opt    Options
	matrix geom.Mat4
	styles []appearance
	root   appearance
	name   string
}
type prepared struct {
	slots []*gomath.Object
	paths []path.Path
	basis [3]pointRef
}

func build(s string, o Options) (*prepared, error) {
	if len(s) > 4*o.MaxGlyphs || utf8.RuneCountInString(s) > o.MaxGlyphs {
		return nil, fmt.Errorf("number exceeds %d glyph slots", o.MaxGlyphs)
	}
	opt := text.DefaultOptions()
	opt.Size, opt.Color = 1, o.Color
	whole, metrics, err := text.Layout(s, o.Font, opt)
	if err != nil {
		return nil, fmt.Errorf("number text: %w", err)
	}
	if !finite(metrics.Width) || metrics.Width <= 0 || metrics.Width > MaxLocalCoordinate || !finite(metrics.Height) || metrics.Height <= 0 || metrics.Height > MaxLocalCoordinate {
		return nil, fmt.Errorf("number font metrics outside local coordinate bounds")
	}
	hasInk := map[rune]bool{}
	curves := 0
	for _, g := range whole.Children {
		r, n := utf8.DecodeRuneInString(g.Name)
		if n != len(g.Name) || len(g.Children) != 0 {
			return nil, fmt.Errorf("number text has unexpected glyph structure")
		}
		hasInk[r] = true
		for _, c := range g.Path.Contours {
			curves += len(c.Curves)
		}
	}
	if curves > o.MaxCurves {
		return nil, fmt.Errorf("number exceeds %d outline curves", o.MaxCurves)
	}
	dx := 0.0
	if o.Anchor == Right {
		dx = -metrics.Width
	} else if o.Anchor == Center {
		dx = -metrics.Width / 2
	}
	p := &prepared{slots: make([]*gomath.Object, 0, utf8.RuneCountInString(s))}
	i := 0
	for _, r := range s {
		var slot *gomath.Object
		if hasInk[r] {
			if i >= len(whole.Children) || whole.Children[i].Name != string(r) {
				return nil, fmt.Errorf("number glyph order mismatch")
			}
			slot = whole.Children[i]
			i++
			slot.Shift(geom.V(dx, 0))
		} else {
			// A covered but empty outline (e.g. space) still has an ordinal slot.
			// text.Layout has already checked actual font coverage for every rune.
			slot = gomath.Group().SetFill(o.Color).SetStroke(gomath.Transparent, 0)
			slot.Name = string(r)
		}
		p.slots = append(p.slots, slot)
		p.paths = append(p.paths, slot.Path.Clone())
	}
	if i != len(whole.Children) {
		return nil, fmt.Errorf("number has unconsumed glyphs")
	}
	valid := true
	visit(p.paths, func(_ pointRef, v geom.Vec3) { valid = valid && bounded(v, MaxLocalCoordinate) })
	if !valid {
		return nil, fmt.Errorf("number source outline exceeds local coordinate bounds")
	}
	p.basis, err = chooseBasis(p.paths)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func newDisplay(s string, o Options, name string) (*display, error) {
	p, err := build(s, o)
	if err != nil {
		return nil, err
	}
	m := geom.Scaling(o.Size, o.Size, 1)
	if err := place(p, m, nil); err != nil {
		return nil, err
	}
	root := gomath.Group(p.slots...)
	root.Name = name
	d := &display{root: root, rootID: root.ID, slots: p.slots, canonical: p.paths, basis: p.basis, opt: o, formatted: s}
	for _, g := range p.slots {
		d.ids = append(d.ids, g.ID)
	}
	if err := root.Validate(); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *snapshot) at(value string) (*gomath.Object, error) {
	p, err := build(value, s.opt)
	if err != nil {
		return nil, err
	}
	if err := place(p, s.matrix, s.styles); err != nil {
		return nil, err
	}
	o := gomath.Group(p.slots...)
	o.Style, o.ZIndex, o.Scalar, o.Name = s.root.style, s.root.z, s.root.scalar, s.name
	if err := o.Validate(); err != nil {
		return nil, err
	}
	return o, nil
}

func place(p *prepared, m geom.Mat4, styles []appearance) error {
	for i, g := range p.slots {
		g.Path = g.Path.Transform(m)
		if len(styles) > 0 {
			// Retain appearance by ordinal position from the right. Newly added
			// left slots inherit the old leftmost slot's appearance.
			j := max(0, len(styles)-len(p.slots)+i)
			g.Style, g.ZIndex, g.Scalar = styles[j].style, styles[j].z, styles[j].scalar
		}
		valid := true
		visit([]path.Path{g.Path}, func(_ pointRef, v geom.Vec3) { valid = valid && bounded(v, MaxWorldCoordinate) })
		if !valid {
			return fmt.Errorf("number transformed outline exceeds finite world coordinate bounds")
		}
	}
	return nil
}

func (d *display) update(object *gomath.Object, value string) error {
	s, err := d.capture(object)
	if err != nil {
		return err
	}
	if value == d.formatted {
		return nil
	}
	p, err := build(value, d.opt)
	if err != nil {
		return err
	}
	if err := place(p, s.matrix, s.styles); err != nil {
		return err
	}
	// No operation below can fail. Retain right-position Object pointers/IDs;
	// removed left slots are detached, never cleared, recycled or reattached.
	shift := len(d.slots) - len(p.slots)
	next := make([]*gomath.Object, len(p.slots))
	ids := make([]uint64, len(p.slots))
	for i, fresh := range p.slots {
		j := i + shift
		if j >= 0 {
			old := d.slots[j]
			old.Path = fresh.Path
			old.Name = fresh.Name
			next[i] = old
		} else {
			next[i] = fresh
		}
		ids[i] = next[i].ID
	}
	object.Children = next
	// Keep the identity ledger independent of the public Children slice. A
	// caller replacing a child must not also replace the expected slot pointer.
	d.slots = append([]*gomath.Object(nil), next...)
	d.ids, d.canonical, d.basis, d.formatted = ids, p.paths, p.basis, value
	return nil
}

func (d *display) capture(object *gomath.Object) (*snapshot, error) {
	if object == nil || object != d.root || object.ID != d.rootID || len(object.Path.Contours) > 0 || object.Bitmap != nil || object.Clip != nil || len(object.Children) != len(d.slots) {
		return nil, fmt.Errorf("number root identity/topology changed or contains unsupported image/clip/path")
	}
	paths := make([]path.Path, len(d.slots))
	styles := make([]appearance, len(d.slots))
	for i, g := range d.slots {
		if g == nil || object.Children[i] != g || g.ID != d.ids[i] || len(g.Children) != 0 || g.Bitmap != nil || g.Clip != nil || !samePathTopology(g.Path, d.canonical[i]) {
			return nil, fmt.Errorf("number glyph slot topology or identity changed")
		}
		paths[i] = g.Path
		styles[i] = appearance{g.Style, g.ZIndex, g.Scalar}
	}
	if err := object.Validate(); err != nil {
		return nil, fmt.Errorf("number object: %w", err)
	}
	m, err := recoverAffine(d.canonical, paths, d.basis)
	if err != nil {
		return nil, err
	}
	return &snapshot{opt: d.opt, matrix: m, styles: styles, root: appearance{object.Style, object.ZIndex, object.Scalar}, name: object.Name}, nil
}

func bounded(v geom.Vec3, limit float64) bool {
	return v.Finite() && math.Abs(v.X) <= limit && math.Abs(v.Y) <= limit && math.Abs(v.Z) <= limit
}
func samePathTopology(a, b path.Path) bool {
	if len(a.Contours) != len(b.Contours) {
		return false
	}
	for i, c := range a.Contours {
		if len(c.Curves) != len(b.Contours[i].Curves) || c.Closed != b.Contours[i].Closed {
			return false
		}
	}
	return true
}
func visit(paths []path.Path, f func(pointRef, geom.Vec3)) {
	for i, p := range paths {
		for j, c := range p.Contours {
			f(pointRef{i, j, -1, 0}, c.Start)
			for k, v := range c.Curves {
				for l, point := range [4]geom.Vec3{v.P0, v.P1, v.P2, v.P3} {
					f(pointRef{i, j, k, l}, point)
				}
			}
		}
	}
}
func pointAt(paths []path.Path, r pointRef) geom.Vec3 {
	c := paths[r.slot].Contours[r.contour]
	if r.curve < 0 {
		return c.Start
	}
	v := c.Curves[r.curve]
	return [4]geom.Vec3{v.P0, v.P1, v.P2, v.P3}[r.point]
}
func chooseBasis(paths []path.Path) ([3]pointRef, error) {
	var refs [3]pointRef
	var a, b geom.Vec3
	first := true
	distance := 0.0
	visit(paths, func(r pointRef, v geom.Vec3) {
		if first {
			a = v
			refs[0] = r
			first = false
		}
		dist := math.Hypot(v.X-a.X, v.Y-a.Y)
		if dist > distance {
			refs[1] = r
			b = v
			distance = dist
		}
	})
	area := 0.0
	visit(paths, func(r pointRef, v geom.Vec3) {
		cross := math.Abs((b.X-a.X)*(v.Y-a.Y) - (b.Y-a.Y)*(v.X-a.X))
		if cross > area {
			refs[2] = r
			area = cross
		}
	})
	if first || distance == 0 || area/(distance*distance) < 1e-12 {
		return refs, fmt.Errorf("number requires nondegenerate two-dimensional real font outlines")
	}
	return refs, nil
}

func recoverAffine(local, world []path.Path, basis [3]pointRef) (geom.Mat4, error) {
	a, b, c := pointAt(local, basis[0]), pointAt(local, basis[1]), pointAt(local, basis[2])
	p, q, r := pointAt(world, basis[0]), pointAt(world, basis[1]), pointAt(world, basis[2])
	dx1, dy1, dx2, dy2 := b.X-a.X, b.Y-a.Y, c.X-a.X, c.Y-a.Y
	det := dx1*dy2 - dx2*dy1
	u := q.Sub(p).Mul(dy2).Sub(r.Sub(p).Mul(dy1)).Mul(1 / det)
	v := r.Sub(p).Mul(dx1).Sub(q.Sub(p).Mul(dx2)).Mul(1 / det)
	w := p.Sub(u.Mul(a.X)).Sub(v.Mul(a.Y))
	ul, vl := u.Norm(), v.Norm()
	minimumScale := 1e-12 * (1 - 256*(math.Nextafter(1, 2)-1))
	if !bounded(u, MaxWorldCoordinate) || !bounded(v, MaxWorldCoordinate) || !bounded(w, MaxWorldCoordinate) || ul < minimumScale || vl < minimumScale || u.Mul(1/ul).Cross(v.Mul(1/vl)).Norm() < 1e-10 {
		return geom.Mat4{}, fmt.Errorf("number affine frame is singular, ill-conditioned, underflowed or outside coordinate bounds")
	}
	m := geom.Identity()
	m[0], m[4], m[8] = u.X, u.Y, u.Z
	m[1], m[5], m[9] = v.X, v.Y, v.Z
	m[3], m[7], m[11] = w.X, w.Y, w.Z
	// Recovering the translation subtracts basis-sized terms. Carry that
	// rounding scale even for a later point whose coordinate is exactly zero.
	var basisScale [3]float64
	for i := range 3 {
		pi, qi, ri := [3]float64{p.X, p.Y, p.Z}[i], [3]float64{q.X, q.Y, q.Z}[i], [3]float64{r.X, r.Y, r.Z}[i]
		ui, vi := [3]float64{u.X, u.Y, u.Z}[i], [3]float64{v.X, v.Y, v.Z}[i]
		basisScale[i] = math.Max(math.Max(math.Abs(pi), math.Abs(qi)), math.Max(math.Abs(ri), math.Abs(ui*a.X)+math.Abs(vi*a.Y)))
	}
	valid := true
	visit(world, func(ref pointRef, actual geom.Vec3) {
		lp := pointAt(local, ref)
		want := m.Apply(lp)
		if !bounded(actual, MaxWorldCoordinate) || !bounded(want, MaxWorldCoordinate) {
			valid = false
			return
		}
		// Account for multiply/add cancellation after whole-object affine edits.
		// This is a floating-point consistency check, not arbitrary path editing.
		for i, g := range [3]float64{actual.X, actual.Y, actual.Z} {
			expected := [3]float64{want.X, want.Y, want.Z}[i]
			origin := [3]float64{w.X, w.Y, w.Z}[i]
			ux, vy := [3]float64{u.X, u.Y, u.Z}[i], [3]float64{v.X, v.Y, v.Z}[i]
			scale := math.Abs(origin) + math.Abs(ux*lp.X) + math.Abs(vy*lp.Y)
			tol := 128 * (math.Nextafter(1, 2) - 1) * math.Max(1e-300, math.Max(basisScale[i], math.Max(scale, math.Max(math.Abs(g), math.Abs(expected)))))
			if math.Abs(g-expected) > tol {
				valid = false
			}
		}
	})
	if !valid {
		return geom.Mat4{}, fmt.Errorf("number paths no longer share the captured affine frame")
	}
	return m, nil
}
