package three

import (
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"math"
)

const (
	MaxDimension = 4096
	MaxPixels    = 4 * 1024 * 1024
	MaxInstances = 1024
	// MaxSampleDimension and MaxSamplePixels bound the internal supersampled
	// raster, independently of the output limits above.
	MaxSampleDimension = 8192
	MaxSamplePixels    = 16 * 1024 * 1024
	// MaxRasterBufferBytes bounds the payload of the sample RGBA/depth buffers
	// plus the resolved RGBA output. It excludes transparent fragment storage,
	// geometry scratch, callers' inputs, concurrent frames, allocator overhead
	// and garbage collector slack.
	MaxRasterBufferBytes = 208 * 1024 * 1024
)

// Material multiplies a constant unassociated color by an optional UV texture.
// Effective alpha classifies each sample as opaque, transparent or invisible.
// A nil Texture retains the original constant-color raster paths. Sampler's
// zero value selects nearest/clamp. Textures require one UV per mesh vertex.
type Material struct {
	Color   color.NRGBA
	Texture *Texture
	Sampler Sampler
}

func DefaultMaterial() Material { return Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}} }

// Light selects world-space directional, single-point or point-list lighting.
// Kind's zero value keeps directional lighting and its original raster paths.
// RGB channels multiply directly in sRGB; there is no gamma/HDR conversion.
type Light struct {
	Direction        geom.Vec3
	Ambient, Diffuse float64
	Kind             LightKind
	// PointLight uses Position and Attenuation; Direction is ignored. Its
	// diffuse contribution is max(0,n dot unit(Position-worldPosition)) /
	// (1+Attenuation*distance^2). At coincidence the diffuse term is zero.
	// DirectionalLight ignores Position/Attenuation and uses Direction.
	Position    geom.Vec3
	Attenuation float64
}

type Config struct {
	Width, Height int
	Background    color.NRGBA
	Light         Light
	// PointLights is used only with Light.Kind=PointLightList (maximum 16).
	// Empty lists give ambient-only illumination. NewRenderer copies/sorts the
	// list; later caller mutations cannot affect concurrent Render calls.
	PointLights []PointLightSource
	// PointLightEvaluationLimit bounds source evaluations across both mesh
	// passes of one PointLightList render. Zero selects 64 Mi evaluations;
	// explicit limits are 1..MaxPointLightEvaluations. Other modes require zero.
	// Includes opaque overdraw, SSAA and zero-strength sources; depth-rejected,
	// alpha-zero and wrong-pass samples are excluded. This is not a CPU deadline.
	PointLightEvaluationLimit int
	CullBackfaces             bool
	// Supersampling is a linear scale of 1, 2 or 4 (1, 4 or 16 samples per
	// output pixel). Zero preserves the default scale of 1. Samples have their
	// own depth and shading; resolution uses a premultiplied RGBA box average.
	// Unsupported scales or excessive working dimensions/buffers return errors
	// from NewRenderer before any raster buffer allocation.
	Supersampling int
	// TransparentFragmentLimit bounds the total visible transparent fragments
	// per Render call, across all internal samples. Zero selects
	// DefaultTransparentFragmentLimit; valid explicit values are
	// 1..MaxTransparentFragments. Exceeding the budget returns nil and an error.
	// Opaque and zero-alpha fragments do not consume this budget.
	TransparentFragmentLimit int
	// PointSampleLimit bounds clipped point bounding-box sample candidates per
	// mixed render, including opaque points. Zero selects 16,777,216; maximum
	// is MaxPointSamples. Each point is charged once, before opaque occlusion.
	PointSampleLimit int
	// Shadow optionally enables bounded opaque-mesh directional hard shadows.
	// The zero value is disabled and retains original frame arithmetic/paths.
	Shadow ShadowConfig
}

func DefaultConfig(width, height int) Config {
	return Config{Width: width, Height: height, Light: Light{Direction: geom.Vec3{X: 1, Y: 1, Z: 2}, Ambient: .2, Diffuse: .8}}
}

// Instance associates a mesh with an affine local-to-world transform. A zero
// Model means identity. Reflections automatically reverse effective triangle
// winding to preserve outward orientation; normals use the inverse transpose.
type Instance struct {
	Mesh     *Mesh
	Model    geom.Mat4
	Material Material
}

func NewInstance(mesh *Mesh) Instance {
	return Instance{Mesh: mesh, Model: geom.Identity(), Material: DefaultMaterial()}
}

// Renderer owns only an immutable configuration. Each Render call allocates
// independent color/depth/fragment buffers; even the same renderer may be used in
// parallel when callers do not mutate input mesh slices during rendering.
type Renderer struct {
	config         Config
	lightDirection geom.Vec3
	sampling       sampleLayout
}

func validateSize(w, h int) error {
	if w <= 0 || h <= 0 || w > MaxDimension || h > MaxDimension || w > MaxPixels/h {
		return fmt.Errorf("three: dimensions must be 1..%d and at most %d pixels", MaxDimension, MaxPixels)
	}
	return nil
}

func NewRenderer(c Config) (*Renderer, error) {
	sampling, err := sampleSize(c)
	if err != nil {
		return nil, err
	}
	if c.TransparentFragmentLimit < 0 || c.TransparentFragmentLimit > MaxTransparentFragments {
		return nil, fmt.Errorf("three: transparent fragment limit must be 0 (default %d) or 1..%d", DefaultTransparentFragmentLimit, MaxTransparentFragments)
	}
	if c.TransparentFragmentLimit == 0 {
		c.TransparentFragmentLimit = DefaultTransparentFragmentLimit
	}
	if c.PointSampleLimit < 0 || c.PointSampleLimit > MaxPointSamples {
		return nil, fmt.Errorf("three: point sample limit must be 0 (default %d) or 1..%d", DefaultPointSampleLimit, MaxPointSamples)
	}
	if c.PointSampleLimit == 0 {
		c.PointSampleLimit = DefaultPointSampleLimit
	}
	d, err := c.Light.validate(c.Shadow.Enabled)
	if err != nil {
		return nil, err
	}
	if err := c.snapshotPointLights(); err != nil {
		return nil, err
	}
	if err := c.Shadow.validate(c.Light.Direction); err != nil {
		return nil, err
	}
	return &Renderer{config: c, lightDirection: d, sampling: sampling}, nil
}

type modelFrame struct {
	matrix    geom.Mat4
	normal    [9]float64
	reflected bool
}

func prepareModel(m geom.Mat4) (modelFrame, error) {
	var f modelFrame
	if m == (geom.Mat4{}) {
		m = geom.Identity()
	}
	for _, x := range m {
		if !finite(x) || math.Abs(x) > MaxCoordinate {
			return f, fmt.Errorf("three: model matrix must be finite and bounded")
		}
	}
	if m[12] != 0 || m[13] != 0 || m[14] != 0 || m[15] != 1 {
		return f, fmt.Errorf("three: model must be affine with bottom row [0,0,0,1]")
	}
	s := 0.0
	for _, i := range []int{0, 1, 2, 4, 5, 6, 8, 9, 10} {
		s = math.Max(s, math.Abs(m[i]))
	}
	if s == 0 {
		return f, fmt.Errorf("three: model must be invertible")
	}
	a, b, c := m[0]/s, m[1]/s, m[2]/s
	d, e, g := m[4]/s, m[5]/s, m[6]/s
	h, i, j := m[8]/s, m[9]/s, m[10]/s
	cofactor := [9]float64{e*j - g*i, g*h - d*j, d*i - e*h, c*i - b*j, a*j - c*h, b*h - a*i, b*g - c*e, c*d - a*g, a*e - b*d}
	det := a*cofactor[0] + b*cofactor[1] + c*cofactor[2]
	if !finite(det) || math.Abs(det) <= 1e-12 {
		return f, fmt.Errorf("three: model is singular or ill-conditioned (normalized determinant <= 1e-12)")
	}
	for k, v := range cofactor {
		f.normal[k] = v / det
	}
	f.matrix = m
	f.reflected = det < 0
	return f, nil
}

func (f modelFrame) transformNormal(v geom.Vec3) (geom.Vec3, bool) {
	v, _ = unit(v)
	a := f.normal
	return unit(geom.Vec3{X: a[0]*v.X + a[1]*v.Y + a[2]*v.Z, Y: a[3]*v.X + a[4]*v.Y + a[5]*v.Z, Z: a[6]*v.X + a[7]*v.Y + a[8]*v.Z})
}

type clipVertex struct{ p, n geom.Vec3 }
type screenVertex struct {
	x, y, z, q float64
	n          geom.Vec3
}
type preparedInstance struct {
	instance Instance
	model    modelFrame
}

// Render returns a new premultiplied *image.RGBA. Opaque surfaces replace the
// background and write depth at each sample. Transparent fragments strictly in
// front of opaque depth blend far-to-near at each sample, then optional SSAA
// resolves premultiplied RGBA by area. Opaque depth ties keep the first submitted
// triangle; transparent ties use a fixed RGBA color order. Alpha 0 is invisible.
// Validation or fragment-budget failure returns nil and an error, never a
// partially rendered image. This is full supersampling, not MSAA.
func (r *Renderer) Render(camera Camera, instances ...Instance) (*image.RGBA, error) {
	return r.render(camera, nil, instances)
}

func (r *Renderer) render(camera Camera, clouds []*PointCloud, instances []Instance) (*image.RGBA, error) {
	if r == nil {
		return nil, fmt.Errorf("three: nil renderer")
	}
	c := r.config
	c.Width, c.Height = r.sampling.width, r.sampling.height
	f, err := camera.frame(float64(c.Width) / float64(c.Height))
	if err != nil {
		return nil, err
	}
	if len(instances) > MaxInstances {
		return nil, fmt.Errorf("three: too many instances (maximum %d)", MaxInstances)
	}
	prepared := make([]preparedInstance, len(instances))
	vertices, triangles := 0, 0
	hasTransparency := false
	hasTexturedTransparency := false
	for i, in := range instances {
		if err := in.Mesh.Validate(); err != nil {
			return nil, fmt.Errorf("three: instance %d: %w", i, err)
		}
		vertices += len(in.Mesh.Vertices)
		triangles += len(in.Mesh.Triangles)
		if vertices > MaxVertices || triangles > MaxTriangles {
			return nil, fmt.Errorf("three: frame geometry exceeds %d vertices or %d triangles", MaxVertices, MaxTriangles)
		}
		if err := in.Material.Sampler.validate(); err != nil {
			return nil, fmt.Errorf("three: instance %d: %w", i, err)
		}
		if in.Material.Texture != nil {
			if !in.Material.Texture.valid() {
				return nil, fmt.Errorf("three: instance %d: invalid texture", i)
			}
			if len(in.Mesh.UVs) != len(in.Mesh.Vertices) {
				return nil, fmt.Errorf("three: instance %d: textured meshes need one UV per vertex; split vertices at UV seams", i)
			}
		}
		transparent := materialHasTransparency(in.Material)
		if c.Shadow.Enabled && in.Material.Color.A > 0 && (in.Material.Color.A != 255 || (in.Material.Texture != nil && !in.Material.Texture.opaque)) {
			return nil, fmt.Errorf("three: shadow instance %d requires opaque material alpha and an entirely opaque texture; transparent/alpha-textured mesh shadows are unsupported", i)
		}
		hasTransparency = hasTransparency || transparent
		hasTexturedTransparency = hasTexturedTransparency || (transparent && in.Material.Texture != nil)
		model, err := prepareModel(in.Model)
		if err != nil {
			return nil, fmt.Errorf("three: instance %d: %w", i, err)
		}
		prepared[i] = preparedInstance{in, model}
	}
	points, pointTransparency, err := r.preparePoints(camera, f, clouds)
	if err != nil {
		return nil, err
	}
	shadow, err := r.buildShadowMap(prepared)
	if err != nil {
		return nil, err
	}
	hasTransparency = hasTransparency || pointTransparency
	img := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	bg := color.RGBAModel.Convert(c.Background).(color.RGBA)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = bg.R, bg.G, bg.B, bg.A
	}
	depth := make([]float64, c.Width*c.Height)
	for i := range depth {
		depth[i] = math.Inf(1)
	}
	// Preserve the opaque traversal and its exact-depth submission semantics.
	// Transparent geometry is processed only after the final opaque z-buffer.
	lightWork := pointLightWork{remaining: c.PointLightEvaluationLimit}
	for index, in := range prepared {
		if in.instance.Material.Color.A == 255 || in.instance.Material.Color.A == 0 {
			var err error
			if shadow != nil && in.instance.Material.Color.A == 255 {
				err = r.renderWorldInstance(img, depth, camera, f, in, index, nil, shadow, nil)
			} else if c.Light.Kind == PointLightList {
				err = r.renderWorldInstance(img, depth, camera, f, in, index, nil, nil, &lightWork)
			} else {
				err = r.renderInstance(img, depth, camera, f, in, index, nil)
			}
			if err != nil {
				return nil, err
			}
		}
	}
	if err := rasterizePoints(img, depth, points, nil); err != nil {
		return nil, err
	}
	if hasTransparency {
		fragments := &transparentBuffer{limit: c.TransparentFragmentLimit}
		if hasTexturedTransparency {
			fragments.precise = &texturedFragmentBuffer{limit: c.TransparentFragmentLimit}
		}
		for index, in := range prepared {
			if materialHasTransparency(in.instance.Material) {
				var err error
				if c.Light.Kind == PointLightList {
					err = r.renderWorldInstance(img, depth, camera, f, in, index, fragments, nil, &lightWork)
				} else {
					err = r.renderInstance(img, depth, camera, f, in, index, fragments)
				}
				if err != nil {
					return nil, err
				}
			}
		}
		if err := rasterizePoints(img, depth, points, fragments); err != nil {
			return nil, err
		}
		fragments.composite(img)
	}
	if r.sampling.scale > 1 {
		return resolveSamples(img, r.config.Width, r.config.Height, r.sampling.scale), nil
	}
	return img, nil
}

// renderInstance shares transformation, clipping and validation between both
// passes. Rasterization of opaque materials retains its original fast path.
func (r *Renderer) renderInstance(img *image.RGBA, depth []float64, camera Camera, f cameraFrame, prepared preparedInstance, index int, fragments *transparentBuffer) error {
	if r.config.Light.Kind == PointLight {
		return r.renderWorldInstance(img, depth, camera, f, prepared, index, fragments, nil, nil)
	}
	c := img.Bounds().Size()
	in, model := prepared.instance, prepared.model
	world := make([]geom.Vec3, len(in.Mesh.Vertices))
	view := make([]geom.Vec3, len(world))
	for i, p := range in.Mesh.Vertices {
		world[i] = model.matrix.Apply(p)
		if !bounded(world[i]) {
			return fmt.Errorf("three: instance %d transformed vertex %d outside coordinate bounds", index, i)
		}
		view[i] = f.view(camera, world[i])
	}
	var normals []geom.Vec3
	if len(in.Mesh.Normals) > 0 {
		normals = make([]geom.Vec3, len(world))
		for i, n := range in.Mesh.Normals {
			var ok bool
			normals[i], ok = model.transformNormal(n)
			if !ok {
				return fmt.Errorf("three: instance %d transformed normal %d is invalid", index, i)
			}
		}
	}
	if in.Material.Texture != nil && in.Material.Color.A > 0 {
		return r.renderTexturedMesh(img, depth, camera, f, prepared, index, world, view, normals, fragments)
	}
	for _, indices := range in.Mesh.Triangles {
		if model.reflected {
			indices[1], indices[2] = indices[2], indices[1]
		}
		var input [3]clipVertex
		face, ok := unit(world[indices[1]].Sub(world[indices[0]]).Cross(world[indices[2]].Sub(world[indices[0]])))
		if !ok {
			return fmt.Errorf("three: instance %d transformed triangle is degenerate", index)
		}
		for k, i := range indices {
			input[k] = clipVertex{p: view[i], n: face}
			if normals != nil {
				input[k].n = normals[i]
			}
		}
		polygon, count := clipTriangle(input, camera, f)
		for k := 1; k+1 < count; k++ {
			a := projectVertex(polygon[0], f, c.X, c.Y)
			b := projectVertex(polygon[k], f, c.X, c.Y)
			d := projectVertex(polygon[k+1], f, c.X, c.Y)
			if in.Material.Color.A == 255 {
				r.rasterize(img, depth, a, b, d, in.Material.Color, f.perspective)
			} else if in.Material.Color.A > 0 {
				if err := r.rasterizeTransparent(img, depth, a, b, d, in.Material.Color, f.perspective, fragments); err != nil {
					return fmt.Errorf("three: instance %d: %w", index, err)
				}
			}
		}
	}
	return nil
}

func planeDistance(v geom.Vec3, plane int, c Camera, f cameraFrame) float64 {
	switch plane {
	case 0:
		return v.Z - c.Near
	case 1:
		return c.Far - v.Z
	}
	x, y := f.sx, f.sy
	if f.perspective {
		x *= v.Z
		y *= v.Z
	}
	switch plane {
	case 2:
		return x + v.X
	case 3:
		return x - v.X
	case 4:
		return y + v.Y
	default:
		return y - v.Y
	}
}

// Sutherland-Hodgman in view space, including near/far before perspective
// division. A triangle clipped against six planes has at most nine vertices.
func clipTriangle(input [3]clipVertex, c Camera, f cameraFrame) ([12]clipVertex, int) {
	var a, b [12]clipVertex
	copy(a[:], input[:])
	count := 3
	for plane := 0; plane < 6 && count > 0; plane++ {
		next := 0
		prev := a[count-1]
		dp := planeDistance(prev.p, plane, c, f)
		for i := 0; i < count; i++ {
			cur := a[i]
			dc := planeDistance(cur.p, plane, c, f)
			if (dp >= 0) != (dc >= 0) {
				t := dp / (dp - dc)
				b[next] = clipVertex{p: prev.p.Lerp(cur.p, t), n: prev.n.Lerp(cur.n, t)}
				// Snap the depth intersection so reciprocal depth never sees a
				// rounded value below the near plane or beyond the far plane.
				if plane == 0 {
					b[next].p.Z = c.Near
				}
				if plane == 1 {
					b[next].p.Z = c.Far
				}
				next++
			}
			if dc >= 0 {
				b[next] = cur
				next++
			}
			prev, dp = cur, dc
		}
		a, b = b, a
		count = next
	}
	return a, count
}

func projectVertex(v clipVertex, f cameraFrame, width, height int) screenVertex {
	x, y, q := v.p.X/f.sx, v.p.Y/f.sy, 1.0
	if f.perspective {
		q = 1 / v.p.Z
		x *= q
		y *= q
	}
	return screenVertex{x: (x + 1) * float64(width) / 2, y: (1 - y) * float64(height) / 2, z: v.p.Z, q: q, n: v.n}
}

func edge(a, b screenVertex, x, y float64) float64 { return (b.x-a.x)*(y-a.y) - (b.y-a.y)*(x-a.x) }
func topLeft(a, b screenVertex) bool               { return b.y < a.y || (b.y == a.y && b.x > a.x) }
func covered(e float64, inclusive bool) bool       { return e > 0 || (e == 0 && inclusive) }

func (r *Renderer) rasterize(img *image.RGBA, depth []float64, a, b, c screenVertex, base color.NRGBA, perspective bool) {
	area := edge(a, b, c.x, c.y)
	// Flipping screen y makes a front-facing CCW world triangle negative.
	if area == 0 || !finite(area) || (r.config.CullBackfaces && area >= 0) {
		return
	}
	if area < 0 {
		b, c = c, b
		area = -area
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	x0 := max(0, int(math.Ceil(math.Min(a.x, math.Min(b.x, c.x))-.5)))
	x1 := min(w-1, int(math.Floor(math.Max(a.x, math.Max(b.x, c.x))-.5)))
	y0 := max(0, int(math.Ceil(math.Min(a.y, math.Min(b.y, c.y))-.5)))
	y1 := min(h-1, int(math.Floor(math.Max(a.y, math.Max(b.y, c.y))-.5)))
	t0, t1, t2 := topLeft(b, c), topLeft(c, a), topLeft(a, b)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+.5, float64(y)+.5
			e0, e1, e2 := edge(b, c, px, py), edge(c, a, px, py), edge(a, b, px, py)
			if !covered(e0, t0) || !covered(e1, t1) || !covered(e2, t2) {
				continue
			}
			wa, wb, wc := e0/area, e1/area, e2/area
			z := wa*a.z + wb*b.z + wc*c.z
			if perspective {
				q := wa*a.q + wb*b.q + wc*c.q
				z = 1 / q
				wa, wb, wc = wa*a.q/q, wb*b.q/q, wc*c.q/q
			}
			i := y*w + x
			if !finite(z) || z >= depth[i] {
				continue
			}
			n, ok := unit(a.n.Mul(wa).Add(b.n.Mul(wb)).Add(c.n.Mul(wc)))
			// Opposing authored normals may interpolate to zero; use ambient.
			lambert := 0.0
			if ok {
				lambert = math.Max(0, n.Dot(r.lightDirection))
			}
			intensity := math.Min(1, r.config.Light.Ambient+r.config.Light.Diffuse*lambert)
			p := y*img.Stride + x*4
			img.Pix[p] = uint8(math.Round(float64(base.R) * intensity))
			img.Pix[p+1] = uint8(math.Round(float64(base.G) * intensity))
			img.Pix[p+2] = uint8(math.Round(float64(base.B) * intensity))
			img.Pix[p+3] = 255
			depth[i] = z
		}
	}
}
