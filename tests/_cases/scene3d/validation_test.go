package scene3d

import (
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
)

func TestNewValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Config, *[]CameraKey, *[]MeshTrack)
	}{
		{"negative-duration", func(c *Config, _ *[]CameraKey, _ *[]MeshTrack) { c.Duration = -1 }},
		{"nan-duration", func(c *Config, _ *[]CameraKey, _ *[]MeshTrack) { c.Duration = math.NaN() }},
		{"duration-bound", func(c *Config, _ *[]CameraKey, _ *[]MeshTrack) { c.Duration = MaxDuration + 1 }},
		{"width-zero", func(c *Config, _ *[]CameraKey, _ *[]MeshTrack) { c.FrameWidth = 0 }},
		{"width-bound", func(c *Config, _ *[]CameraKey, _ *[]MeshTrack) { c.FrameWidth = MaxFrameWidth + 1 }},
		{"renderer", func(c *Config, _ *[]CameraKey, _ *[]MeshTrack) { c.Renderer.Width = 0 }},
		{"no-camera", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) { *k = nil }},
		{"camera-time", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) { (*k)[0].Time = 3 }},
		{"camera-duplicate-time", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) { *k = append(*k, (*k)[0]) }},
		{"camera-nan-time", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) { (*k)[0].Time = math.NaN() }},
		{"camera-look", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) { (*k)[0].Camera.Position = (*k)[0].Camera.Target }},
		{"camera-fov", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) { (*k)[0].Camera.FOVY = 0 }},
		{"camera-unused-field", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) { (*k)[0].Camera.OrthoHeight = math.NaN() }},
		{"projection-switch", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) {
			b := (*k)[0]
			b.Time = 2
			b.Camera.Projection = three.Orthographic
			*k = append(*k, b)
		}},
		{"up-switch", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) {
			b := (*k)[0]
			b.Time = 2
			b.Camera.Up = geom.Vec3{X: 1}
			*k = append(*k, b)
		}},
		{"near-switch", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) {
			b := (*k)[0]
			b.Time = 2
			b.Camera.Near = .2
			*k = append(*k, b)
		}},
		{"far-switch", func(_ *Config, k *[]CameraKey, _ *[]MeshTrack) {
			b := (*k)[0]
			b.Time = 2
			b.Camera.Far = 30
			*k = append(*k, b)
		}},
		{"zero-id", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].ID = 0 }},
		{"duplicate-id", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { *m = append(*m, (*m)[0]) }},
		{"nil-mesh", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Mesh = nil }},
		{"degenerate", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Mesh.Vertices[1] = (*m)[0].Mesh.Vertices[0] }},
		{"normals-count", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Mesh.Normals = []geom.Vec3{{Z: 1}} }},
		{"nan-axis", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Axis.X = math.NaN() }},
		{"axis-bound", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Axis.Y = 1e12 + 1 }},
		{"sampler", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			(*m)[0].Material.Sampler.Filter = three.TextureFilter(255)
		}},
		{"texture-zero", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Material.Texture = &three.Texture{} }},
		{"key-time", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			(*m)[0].Keys = []PoseKey{{Time: -1, Pose: IdentityPose()}}
		}},
		{"key-duplicate", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			(*m)[0].Keys = []PoseKey{{Pose: IdentityPose()}, {Pose: IdentityPose()}}
		}},
		{"zero-pose", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Keys = []PoseKey{{}} }},
		{"nan-position", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			p := IdentityPose()
			p.Position.X = math.NaN()
			(*m)[0].Keys = []PoseKey{{Pose: p}}
		}},
		{"position-bound", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			p := IdentityPose()
			p.Position.Z = 1e12 + 1
			(*m)[0].Keys = []PoseKey{{Pose: p}}
		}},
		{"angle-bound", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			p := IdentityPose()
			p.Angle = MaxAngle + 1
			(*m)[0].Keys = []PoseKey{{Pose: p}}
		}},
		{"scale-bound", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			p := IdentityPose()
			p.Scale.X = MaxScale + 1
			(*m)[0].Keys = []PoseKey{{Pose: p}}
		}},
		{"scale-ratio", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			p := IdentityPose()
			p.Scale.X = MaxScale
			p.Scale.Y = MinScale
			(*m)[0].Keys = []PoseKey{{Pose: p}}
		}},
		{"reflection-switch", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			p := IdentityPose()
			p.Scale.X = -1
			(*m)[0].Keys = []PoseKey{{Pose: IdentityPose()}, {Time: 2, Pose: p}}
		}},
		{"visibility-negative", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Visible = []Visibility{{Add: -1}} }},
		{"visibility-remove-negative", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Visible = []Visibility{{Remove: -1}} }},
		{"visibility-empty", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Visible = []Visibility{{Add: 1, Remove: 1}} }},
		{"visibility-overlap", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) {
			(*m)[0].Visible = []Visibility{{Remove: 1.5}, {Add: 1}}
		}},
		{"visibility-open-first", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Visible = []Visibility{{}, {Add: 1}} }},
		{"visibility-infinite", func(_ *Config, _ *[]CameraKey, m *[]MeshTrack) { (*m)[0].Visible = []Visibility{{Remove: math.Inf(1)}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, k, m := basicFixture(t)
			tc.change(&c, &k, &m)
			if s, err := New(c, k, m); s != nil || err == nil {
				t.Fatalf("invalid input accepted: %v %v", s, err)
			}
		})
	}
	// Explicit valid extreme poses and a subnormal axis are stored, not clamped.
	for _, size := range []float64{MinScale, MaxScale} {
		c, k, m := basicFixture(t)
		p := IdentityPose()
		p.Scale = geom.Vec3{X: -size, Y: size, Z: size}
		p.Angle = MaxAngle
		m[0].Keys = []PoseKey{{Pose: p}}
		m[0].Axis = geom.Vec3{X: math.SmallestNonzeroFloat64}
		s := mustNew(t, c, k, m)
		if got, _, _ := s.PoseAt(7, 0); got != p {
			t.Fatal("valid bound pose clamped")
		}
		if _, err := s.Sample(0); err != nil {
			t.Fatal("valid bound transform rejected", err)
		}
	}
}

type fixedImage struct {
	bounds image.Rectangle
	value  color.Color
}

func (i fixedImage) ColorModel() color.Model { return color.NRGBAModel }
func (i fixedImage) Bounds() image.Rectangle { return i.bounds }
func (i fixedImage) At(x, y int) color.Color { return i.value }

func TestSceneBudgets(t *testing.T) {
	c, k, m := basicFixture(t)
	tooMany := make([]MeshTrack, three.MaxInstances+1)
	if _, err := New(c, k, tooMany); err == nil {
		t.Fatal("track budget accepted")
	}
	keys := make([]PoseKey, MaxKeyframes) // plus one camera key exceeds total
	bad := m[0]
	bad.Keys = keys
	if _, err := New(c, k, []MeshTrack{bad}); err == nil {
		t.Fatal("key budget accepted")
	}
	keys = keys[:MaxKeyframes-1]
	for i := range keys {
		keys[i] = PoseKey{Time: 2 * float64(i) / float64(len(keys)-1), Pose: IdentityPose()}
	}
	bad.Keys = keys
	s := mustNew(t, c, k, []MeshTrack{bad})
	if _, _, err := s.PoseAt(7, 1.234); err != nil {
		t.Fatal(err)
	}
	bad = m[0]
	bad.Visible = make([]Visibility, MaxVisibilityIntervals+1)
	if _, err := New(c, k, []MeshTrack{bad}); err == nil {
		t.Fatal("visibility budget accepted")
	}
	bad.Visible = bad.Visible[:MaxVisibilityIntervals]
	for i := range bad.Visible {
		bad.Visible[i] = Visibility{Add: 2 * float64(i) / float64(len(bad.Visible)+1), Remove: 2 * (float64(i) + .5) / float64(len(bad.Visible)+1)}
	}
	mustNew(t, c, k, []MeshTrack{bad})
	large := *m[0].Mesh
	large.UVs = nil
	large.Vertices = make([]geom.Vec3, three.MaxVertices)
	copy(large.Vertices, m[0].Mesh.Vertices)
	bad = m[0]
	bad.Mesh = &large
	mustNew(t, c, k, []MeshTrack{bad}) // exact aggregate vertex boundary
	second := m[0]
	second.ID = 8
	if _, err := New(c, k, []MeshTrack{bad, second}); err == nil {
		t.Fatal("aggregate vertex budget accepted")
	}
	large.Vertices = m[0].Mesh.Vertices
	large.Triangles = make([]three.Triangle, three.MaxTriangles+1)
	if _, err := New(c, k, []MeshTrack{bad}); err == nil {
		t.Fatal("triangle budget accepted")
	}
	// Five distinct 16 MiB immutable textures exceed 64 MiB. Repeated use of
	// the same immutable asset counts once. The source image itself is tiny.
	textureTracks := make([]MeshTrack, 5)
	for i := range textureTracks {
		tex, err := three.NewTexture(fixedImage{image.Rect(0, 0, 2048, 2048), color.NRGBA{R: 211, A: 255}})
		if err != nil {
			t.Fatal(err)
		}
		textureTracks[i] = m[0]
		textureTracks[i].ID = uint64(i + 1)
		textureTracks[i].Material.Texture = tex
	}
	mustNew(t, c, k, textureTracks[:4])
	if _, err := New(c, k, textureTracks); err == nil || !strings.Contains(err.Error(), "textures exceed") {
		t.Fatal("texture payload budget missing", err)
	}
	for i := 1; i < len(textureTracks); i++ {
		textureTracks[i].Material.Texture = textureTracks[0].Material.Texture
	}
	mustNew(t, c, k, textureTracks)
}

func TestSampleErrorsAndRecovery(t *testing.T) {
	c, k, m := basicFixture(t)
	c.Renderer.TransparentFragmentLimit = 1
	m[0].Material.Color.A = 127
	m[0].Visible = []Visibility{{Remove: 1}}
	s := mustNew(t, c, k, m)
	if frame, err := s.Sample(.5); frame != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
		t.Fatal("missing transparent budget error", frame, err)
	}
	if _, err := s.Sample(1); err != nil {
		t.Fatal("render failure poisoned hidden sample", err)
	}
	c, k, m = basicFixture(t)
	c.Renderer.Shadow = three.ShadowConfig{Enabled: true, Size: 10, Depth: 20, Resolution: 16, Bias: .01}
	c.Renderer.Light.Direction = geom.Vec3{Z: 1}
	m[0].Material.Color.A = 127
	s = mustNew(t, c, k, m)
	if frame, err := s.Sample(0); frame != nil || err == nil || !strings.Contains(err.Error(), "requires opaque") {
		t.Fatal("unsupported caster silently changed", frame, err)
	}
	c.Renderer.Light.Kind = three.PointLight
	if _, err := New(c, k, m); err == nil {
		t.Fatal("point light used directional shadow map")
	}
	c, k, m = basicFixture(t)
	p := IdentityPose()
	p.Position.X = 1e12
	m[0].Mesh.Vertices[0].X = 1e12
	m[0].Keys = []PoseKey{{Pose: p}}
	m[0].Visible = []Visibility{{Add: 1}}
	s = mustNew(t, c, k, m)
	if _, err := s.Sample(0); err != nil {
		t.Fatal("hidden transformed mesh consumed render work", err)
	}
	if frame, err := s.Sample(1); frame != nil || err == nil || !strings.Contains(err.Error(), "coordinate bounds") {
		t.Fatal("world-bound error missing", frame, err)
	}
}
