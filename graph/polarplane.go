package graph

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
)

type PolarPlaneOptions struct {
	NumberPlaneOptions
	Rays      int
	Offset    float64 // Scene angle of azimuth zero, in radians before transforms.
	Clockwise bool
}

func DefaultPolarPlaneOptions() PolarPlaneOptions {
	return PolarPlaneOptions{NumberPlaneOptions: DefaultNumberPlaneOptions(), Rays: 20}
}

type PolarCircle struct {
	Radius float64
	Object *gomath.Object // Borrowed actual circle child.
}

// PolarPlane owns bounded concentric circles, radial rays and an affine frame.
// Radius data starts at zero; worldRadius is the outer circle's scene radius.
// Whole-Object affine transforms move the coordinate map and all grid geometry.
type PolarPlane struct {
	Object      *gomath.Object
	coordinates *ComplexPlane
	circles     []PolarCircle
	rays        []*gomath.Object
	offset      float64
	clockwise   bool
}

func NewPolarPlane(radius Range, worldRadius float64, options PolarPlaneOptions) (*PolarPlane, error) {
	if err := validateRange(radius); err != nil {
		return nil, err
	}
	if radius.Min != 0 || !finite(worldRadius) || !finite(2*worldRadius) || worldRadius <= 0 || options.Rays < 1 || options.Rays > 4096 || !finite(options.Offset) || !finite(options.GridWidth) || options.GridWidth <= 0 || !finite(options.AxisWidth) || options.AxisWidth <= 0 {
		return nil, fmt.Errorf("polar plane requires radius Min=0, finite positive dimensions/strokes and 1..4096 rays")
	}
	style := gomath.DefaultStyle()
	style.Fill, style.Stroke = options.AxisColor, options.GridColor
	if err := style.Validate(); err != nil {
		return nil, err
	}
	values, err := tickValues(radius)
	if err != nil {
		return nil, err
	}
	// The triangular reference frame is inside the circle's convex hull, so
	// rotating or shearing it cannot enlarge bounds beyond the visible grid.
	half := worldRadius / 2
	a, b, c := geom.V(-half, -half), geom.V(half, -half), geom.V(-half, half)
	frame := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: a, Curves: []path.Cubic{path.Line(a, b), path.Line(b, c)}}}}).
		SetFill(gomath.Transparent).SetStroke(gomath.Transparent, 0)
	frame.Name = "PolarPlane affine frame"
	object := gomath.Group(frame)
	domain := Range{Min: -radius.Max / 2, Max: radius.Max / 2, Step: radius.Step}
	if domain.Max-domain.Min == 0 {
		return nil, fmt.Errorf("polar radius frame loses coordinate precision")
	}
	coordinates := &ComplexPlane{Object: object, frame: frame, x: domain, y: domain}
	p := &PolarPlane{Object: object, coordinates: coordinates, offset: math.Mod(options.Offset, 2*math.Pi), clockwise: options.Clockwise}
	if _, _, _, err = coordinates.basis(); err != nil {
		return nil, err
	}
	if len(values) == 0 || values[len(values)-1] != radius.Max {
		values = append(values, radius.Max)
	}
	for _, value := range values {
		if value == 0 {
			continue
		}
		world := worldRadius * (value / radius.Max)
		if world == 0 || !finite(world) {
			return nil, fmt.Errorf("polar circle radius loses precision")
		}
		circle := gomath.Circle(world).SetStroke(options.GridColor, options.GridWidth)
		p.circles = append(p.circles, PolarCircle{Radius: value, Object: circle})
		object.Children = append(object.Children, circle)
	}
	for i := range options.Rays {
		end, err := p.Point(radius.Max, float64(i)*(2*math.Pi/float64(options.Rays)))
		if err != nil {
			return nil, err
		}
		ray := gomath.Line(geom.V(0, 0), end).SetStroke(options.GridColor, options.GridWidth)
		p.rays = append(p.rays, ray)
		object.Children = append(object.Children, ray)
	}
	if options.ShowAxes {
		for _, angle := range []float64{0, math.Pi / 2} {
			end, err := p.Point(radius.Max, angle)
			if err != nil {
				return nil, err
			}
			object.Children = append(object.Children, gomath.Line(end.Mul(-1), end).SetStroke(options.AxisColor, options.AxisWidth))
		}
	}
	if err = object.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *PolarPlane) complex(radius, azimuth float64) (complex128, error) {
	if p == nil || p.coordinates == nil {
		return 0, fmt.Errorf("uninitialized polar plane")
	}
	if !finite(radius) || radius < 0 || !finite(azimuth) {
		return 0, fmt.Errorf("polar coordinates require a finite nonnegative radius and finite azimuth")
	}
	angle := math.Mod(azimuth, 2*math.Pi)
	if p.clockwise {
		angle = -angle
	}
	s, c := math.Sincos(p.offset + angle)
	return complex(radius*c, radius*s), nil
}

// Point maps a nonnegative radius and radian azimuth. Extrapolation beyond the
// outer grid circle is allowed. Clockwise/offset apply before affine mapping.
func (p *PolarPlane) Point(radius, azimuth float64) (geom.Vec3, error) {
	z, err := p.complex(radius, azimuth)
	if err != nil {
		return geom.Vec3{}, err
	}
	return p.coordinates.Point(z)
}

// Coordinates projects into the current affine plane and returns radius >=0
// and azimuth in [0,2*pi). The exact origin uses azimuth zero by convention.
func (p *PolarPlane) Coordinates(point geom.Vec3) (radius, azimuth float64, err error) {
	if p == nil || p.coordinates == nil {
		return 0, 0, fmt.Errorf("uninitialized polar plane")
	}
	center, err := p.coordinates.Point(0)
	if err != nil {
		return 0, 0, err
	}
	if point == center {
		return 0, 0, nil
	}
	z, err := p.coordinates.Number(point)
	if err != nil {
		return 0, 0, err
	}
	radius = math.Hypot(real(z), imag(z))
	if !finite(radius) {
		return 0, 0, fmt.Errorf("polar radius overflows")
	}
	if radius == 0 {
		return 0, 0, nil
	}
	azimuth = math.Atan2(imag(z), real(z)) - p.offset
	if p.clockwise {
		azimuth = -azimuth
	}
	azimuth = math.Mod(azimuth, 2*math.Pi)
	if azimuth < 0 {
		azimuth += 2 * math.Pi
	}
	if azimuth == 0 || azimuth >= 2*math.Pi {
		azimuth = 0
	}
	return radius, azimuth, nil
}

func (p *PolarPlane) Circles() []PolarCircle {
	if p == nil {
		return nil
	}
	return append([]PolarCircle(nil), p.circles...)
}

func (p *PolarPlane) Rays() []*gomath.Object {
	if p == nil {
		return nil
	}
	return append([]*gomath.Object(nil), p.rays...)
}

// Plot samples a radial function at uniform azimuths in radians. Nonfinite
// callback values break contours; negative finite radii reject. Returned
// geometry is independent of future plane transformations.
func (p *PolarPlane) Plot(f func(float64) float64, start, end float64, samples int) (*gomath.Object, error) {
	if p == nil || p.coordinates == nil || f == nil || !finite(start) || !finite(end) || end <= start || !finite(end-start) || samples < 2 || samples > 1_000_000 {
		return nil, fmt.Errorf("polar plot requires an initialized plane, finite increasing interval and 2..1000000 samples")
	}
	origin, x, y, err := p.coordinates.basis()
	if err != nil {
		return nil, err
	}
	var builder path.Builder
	open := false
	for i := range samples {
		angle := partitionPoint(start, end, i, samples-1)
		radius := f(angle)
		if !finite(radius) {
			open = false
			continue
		}
		z, err := p.complex(radius, angle)
		if err != nil {
			return nil, err
		}
		point, err := p.coordinates.point(z, origin, x, y)
		if err != nil {
			return nil, err
		}
		if open {
			builder.LineTo(point)
		} else {
			builder.MoveTo(point)
			open = true
		}
	}
	return gomath.NewObject(builder.Path()).SetStroke(gomath.Blue, .035), nil
}
