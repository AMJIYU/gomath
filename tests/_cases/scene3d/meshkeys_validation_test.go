package scene3d

import (
	"bytes"
	"gomath/geom"
	"gomath/three"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestMeshKeyValidation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*MeshTrack, *three.Mesh, *three.Mesh)
	}{
		{"neither", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys = nil }},
		{"both", func(t *MeshTrack, a, _ *three.Mesh) { t.Mesh = a }},
		{"nil-first", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[0].Mesh = nil }},
		{"nil-later", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[1].Mesh = nil }},
		{"duplicate-time", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[1].Time = .25 }},
		{"reverse-time", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[1].Time = .1 }},
		{"negative-time", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[0].Time = -1 }},
		{"nan-time", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[0].Time = math.NaN() }},
		{"infinite-time", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[0].Time = math.Inf(1) }},
		{"late-time", func(t *MeshTrack, _, _ *three.Mesh) { t.MeshKeys[1].Time = 3 }},
		{"vertex-count", func(_ *MeshTrack, _, b *three.Mesh) {
			b.Vertices = append(b.Vertices, geom.Vec3{X: 1})
			b.Normals = append(b.Normals, geom.Vec3{Z: 1})
			b.UVs = append(b.UVs, three.UV{})
		}},
		{"triangle-count", func(_ *MeshTrack, _, b *three.Mesh) { b.Triangles = append(b.Triangles, b.Triangles[0]) }},
		{"triangle-winding", func(_ *MeshTrack, _, b *three.Mesh) { b.Triangles[0] = three.Triangle{0, 2, 1} }},
		{"triangle-order", func(_ *MeshTrack, a, b *three.Mesh) {
			a.Triangles = []three.Triangle{{0, 1, 2}, {0, 2, 1}}
			b.Triangles = []three.Triangle{{0, 2, 1}, {0, 1, 2}}
		}},
		{"normal-presence", func(_ *MeshTrack, _, b *three.Mesh) { b.Normals = nil }},
		{"normal-zero", func(_ *MeshTrack, _, b *three.Mesh) { b.Normals[0] = geom.Vec3{} }},
		{"uv-presence", func(_ *MeshTrack, _, b *three.Mesh) { b.UVs = nil }},
		{"uv-bound", func(_ *MeshTrack, _, b *three.Mesh) { b.UVs[0].U = three.MaxUVCoordinate + 1 }},
		{"vertex-finite", func(_ *MeshTrack, _, b *three.Mesh) { b.Vertices[0].X = math.NaN() }},
		{"degenerate-key", func(_ *MeshTrack, _, b *three.Mesh) { b.Vertices[0] = b.Vertices[1] }},
		{"material-time", func(t *MeshTrack, _, _ *three.Mesh) { t.MaterialKeys = []MaterialKey{{Time: 3}} }},
		{"material-nan", func(t *MeshTrack, _, _ *three.Mesh) { t.MaterialKeys = []MaterialKey{{Time: math.NaN()}} }},
		{"material-infinite", func(t *MeshTrack, _, _ *three.Mesh) { t.MaterialKeys = []MaterialKey{{Time: math.Inf(1)}} }},
		{"material-negative", func(t *MeshTrack, _, _ *three.Mesh) { t.MaterialKeys = []MaterialKey{{Time: -1}} }},
		{"material-duplicate", func(t *MeshTrack, _, _ *three.Mesh) { t.MaterialKeys = []MaterialKey{{Time: 1}, {Time: 1}} }},
		{"material-order", func(t *MeshTrack, _, _ *three.Mesh) { t.MaterialKeys = []MaterialKey{{Time: 1}, {Time: .5}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, cameras, tracks, a, b := meshKeyFixture(t)
			tc.edit(&tracks[0], a, b)
			if s, err := New(c, cameras, tracks); s != nil || err == nil {
				t.Fatal("invalid mesh/material keys accepted", s, err)
			}
		})
	}
}

func TestMeshKeyDegenerateSamplesAndRecovery(t *testing.T) {
	for _, normal := range []bool{false, true} {
		c, cameras, tracks := basicFixture(t)
		a := tracks[0].Mesh
		b := &three.Mesh{Vertices: append([]geom.Vec3(nil), a.Vertices...), UVs: a.UVs, Triangles: a.Triangles}
		message := "degenerate"
		if normal {
			a.Normals = []geom.Vec3{{Z: 1}, {Z: 1}, {Z: 1}}
			b.Normals = []geom.Vec3{{Z: -1}, {Z: -1}, {Z: -1}}
			message = "interpolated normal"
		} else {
			for i := range b.Vertices {
				b.Vertices[i].X = -b.Vertices[i].X
			}
		}
		tracks[0].Mesh, tracks[0].MeshKeys = nil, []MeshKey{{Mesh: a}, {Time: 2, Mesh: b}}
		tracks[0].Material.Color.A = 0 // local geometry still validates
		s := mustNew(t, c, cameras, tracks)
		for _, time := range []float64{0, .5, 1, 1.5, 2, 1, 0} {
			frame, err := s.Sample(time)
			if time == 1 {
				if frame != nil || err == nil || !strings.Contains(err.Error(), message) {
					t.Fatal("invalid interior state was hidden", normal, frame, err)
				}
			} else if err != nil {
				t.Fatal("invalid sample poisoned valid sample", time, err)
			}
		}
		tracks[0].Visible = []Visibility{{Remove: .75}, {Add: 1.25}}
		s = mustNew(t, c, cameras, tracks)
		if _, err := s.Sample(1); err != nil {
			t.Fatal("hidden geometry should not be generated", err)
		}
	}
	// Very large finite authored normal magnitudes normalize safely in an
	// interior sample; there is no overflow-based fallback to flat normals.
	c, cameras, tracks, a, b := meshKeyFixture(t)
	for i := range a.Normals {
		a.Normals[i], b.Normals[i] = geom.Vec3{X: 1e308, Z: 1e308}, geom.Vec3{Y: 1e308, Z: 1e308}
	}
	s := mustNew(t, c, cameras, tracks)
	if _, err := s.Sample(1); err != nil {
		t.Fatal("finite large normals rejected", err)
	}
	// A one-key, zero-duration geometry/material source is a valid endpoint.
	c.Duration = 0
	tracks[0].MeshKeys = []MeshKey{{Mesh: a}}
	tracks[0].MaterialKeys = []MaterialKey{{Color: color.NRGBA{R: 255, A: 255}}}
	s = mustNew(t, c, cameras, tracks)
	if _, err := s.Sample(0); err != nil {
		t.Fatal(err)
	}
}

func TestMaterialKeysFragmentAndShadowErrors(t *testing.T) {
	c, cameras, tracks := basicFixture(t)
	c.Renderer.Width, c.Renderer.Height = 1, 1
	c.Renderer.TransparentFragmentLimit = 1
	tracks[0].MaterialKeys = []MaterialKey{{Color: color.NRGBA{R: 231, G: 83, A: 255}}, {Time: 2, Color: color.NRGBA{B: 255}}}
	cloud := mustCloud(t, []geom.Vec3{{Z: .5}}, []color.NRGBA{{G: 255, A: 128}}, nil)
	s := mustNewPoints(t, c, cameras, tracks, []PointTrack{{ID: 11, Cloud: cloud}})
	if _, err := s.Sample(0); err != nil {
		t.Fatal("opaque material endpoint charged as transparent", err)
	}
	if frame, err := s.Sample(1); frame != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
		t.Fatal("animated alpha failed to share mesh/point fragment budget", frame, err)
	}
	if _, err := s.Sample(2); err != nil {
		t.Fatal("invisible material endpoint consumed a fragment", err)
	}
	c, cameras, tracks = basicFixture(t)
	c.Renderer.Light.Direction = geom.Vec3{Z: 1}
	c.Renderer.Shadow = three.ShadowConfig{Enabled: true, Size: 8, Depth: 20, Resolution: 32, Bias: .01}
	tracks[0].MaterialKeys = []MaterialKey{{Color: color.NRGBA{R: 255, A: 255}}, {Time: 2}}
	s = mustNew(t, c, cameras, tracks)
	for _, time := range []float64{0, 1, 2, 0} {
		frame, err := s.Sample(time)
		if time == 1 {
			if frame != nil || err == nil || !strings.Contains(err.Error(), "requires opaque") {
				t.Fatal("animated shadow caster opacity silently approximated", frame, err)
			}
		} else if err != nil {
			t.Fatal("shadow alpha failure poisoned valid endpoint", time, err)
		}
	}
}

func TestMeshSnapshotAndKeyBudgets(t *testing.T) {
	c, cameras, tracks := basicFixture(t)
	track := tracks[0]
	track.MaterialKeys = make([]MaterialKey, MaxKeyframes)
	if _, err := New(c, cameras, []MeshTrack{track}); err == nil {
		t.Fatal("material key budget accepted")
	}
	track.MaterialKeys, track.Mesh, track.MeshKeys = nil, nil, make([]MeshKey, MaxKeyframes)
	track.MeshKeys[0].Mesh = tracks[0].Mesh
	if _, err := New(c, cameras, []MeshTrack{track}); err == nil {
		t.Fatal("mesh key budget accepted")
	}
	// Total camera + mesh geometry/material + point pose/buffer keys is shared.
	track = tracks[0]
	track.MaterialKeys = make([]MaterialKey, MaxKeyframes-3)
	for i := range track.MaterialKeys {
		track.MaterialKeys[i] = MaterialKey{Time: 2 * float64(i) / float64(len(track.MaterialKeys)-1)}
	}
	track.Mesh, track.MeshKeys = nil, []MeshKey{{Mesh: tracks[0].Mesh}}
	cloud := pointCountCloud(t, 1)
	point := PointTrack{ID: 11, PointKeys: []PointKey{{Cloud: cloud}}}
	mustNewPoints(t, c, cameras, []MeshTrack{track}, []PointTrack{point})
	point.Keys = []PoseKey{{Pose: IdentityPose()}}
	if _, err := NewWithPoints(c, cameras, []MeshTrack{track}, []PointTrack{point}); err == nil {
		t.Fatal("combined geometry/material/point key budget accepted")
	}
	// Eight repeated full-attribute buffers consume 64 MiB minus 320 bytes.
	// Valid 96-byte and 224-byte static meshes fill the logical limit exactly.
	large := &three.Mesh{Vertices: make([]geom.Vec3, 131071), Normals: make([]geom.Vec3, 131071), UVs: make([]three.UV, 131071), Triangles: []three.Triangle{{0, 1, 2}}}
	copy(large.Vertices, tracks[0].Mesh.Vertices)
	for i := range large.Normals {
		large.Normals[i] = geom.Vec3{Z: 1}
	}
	keys := make([]MeshKey, 8)
	for i := range keys {
		keys[i] = MeshKey{Time: 2 * float64(i) / 7, Mesh: large}
	}
	small := &three.Mesh{Vertices: tracks[0].Mesh.Vertices, Triangles: tracks[0].Mesh.Triangles}
	pad := &three.Mesh{Vertices: append(append([]geom.Vec3(nil), small.Vertices...), geom.Vec3{}, geom.Vec3{}), UVs: make([]three.UV, 5), Triangles: []three.Triangle{{0, 1, 2}}}
	tracks = []MeshTrack{{ID: 7, MeshKeys: keys}, {ID: 8, Mesh: small}, {ID: 9, Mesh: pad}}
	mustNew(t, c, cameras, tracks)
	pad.Triangles = append(pad.Triangles, pad.Triangles[0])
	if s, err := New(c, cameras, tracks); s != nil || err == nil || !strings.Contains(err.Error(), "mesh snapshot budget") {
		t.Fatal("owned mesh snapshot byte budget accepted", s, err)
	}
}

func TestMeshKeyInputIsolationAndConcurrentOrder(t *testing.T) {
	c, cameras, tracks, a, b := meshKeyFixture(t)
	c.Renderer.Supersampling = 2
	tracks[0].MaterialKeys = []MaterialKey{{Color: color.NRGBA{R: 231, G: 83, A: 255}}, {Time: 2, Color: color.NRGBA{G: 173, B: 231, A: 128}}}
	s := mustNew(t, c, cameras, tracks)
	times := []float64{0, .25, .625, 1, 1.375, 1.75, 2}
	wants := make([][]byte, len(times))
	for i, time := range times {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		wants[i] = bitmapBytes(f.Objects[0].Bitmap)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for range 40 {
			a.Vertices[0].X, b.Vertices[0].Y = math.NaN(), math.NaN()
			a.Normals[0].Z, b.Normals[0].X = math.NaN(), math.NaN()
			a.UVs[0].U, b.UVs[0].V = math.NaN(), math.NaN()
			a.Triangles[0][0], b.Triangles[0][0] = 999, 999
			tracks[0].MeshKeys[0].Time = math.NaN()
			tracks[0].MeshKeys[1].Mesh = nil
			tracks[0].MaterialKeys[0].Time = math.NaN()
			tracks[0].MaterialKeys[1].Color = color.NRGBA{}
		}
		*a, *b = three.Mesh{}, three.Mesh{}
	}()
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := range 15 {
				i := (worker + 3*j) % len(times)
				frame, err := s.Sample(times[i])
				if err != nil {
					t.Error(err)
					return
				}
				if frame.Objects[0].ID != s.BitmapID() || !bytes.Equal(wants[i], bitmapBytes(frame.Objects[0].Bitmap)) {
					t.Error("geometry/material authoring data or samples shared mutable state")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}
