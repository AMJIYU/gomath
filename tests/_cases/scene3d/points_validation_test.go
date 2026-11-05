package scene3d

import (
	"bytes"
	"gomath/geom"
	"gomath/three"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestPointTrackValidation(t *testing.T) {
	c, cameras, meshes := basicFixture(t)
	cloud := mustCloud(t, []geom.Vec3{{}}, []color.NRGBA{{R: 255, A: 255}}, nil)
	other := mustCloud(t, []geom.Vec3{{}, {X: 1}}, []color.NRGBA{{}, {}}, nil)
	cases := []PointTrack{
		{ID: 11},
		{ID: 11, Cloud: &three.PointCloud{}},
		{Cloud: cloud},
		{ID: 7, Cloud: cloud}, // mesh namespace collision
		{ID: 11, Cloud: cloud, PointKeys: []PointKey{{Cloud: cloud}}},
		{ID: 11, PointKeys: []PointKey{{Cloud: cloud}, {Time: 2, Cloud: other}}},
		{ID: 11, PointKeys: []PointKey{{Cloud: cloud}, {Time: 1}}},
		{ID: 11, PointKeys: []PointKey{{Cloud: cloud}, {Cloud: cloud}}},
		{ID: 11, PointKeys: []PointKey{{Time: 2, Cloud: cloud}, {Time: 1, Cloud: cloud}}},
		{ID: 11, PointKeys: []PointKey{{Time: -1, Cloud: cloud}}},
		{ID: 11, PointKeys: []PointKey{{Time: math.NaN(), Cloud: cloud}}},
		{ID: 11, PointKeys: []PointKey{{Time: math.Inf(1), Cloud: cloud}}},
		{ID: 11, PointKeys: []PointKey{{Time: 3, Cloud: cloud}}},
		{ID: 11, Cloud: cloud, Axis: geom.Vec3{Z: math.NaN()}},
		{ID: 11, Cloud: cloud, Axis: geom.Vec3{X: 1e12 + 1}},
		{ID: 11, Cloud: cloud, Keys: []PoseKey{{}}},
		{ID: 11, Cloud: cloud, Keys: []PoseKey{{Time: 3, Pose: IdentityPose()}}},
		{ID: 11, Cloud: cloud, Keys: []PoseKey{{Pose: IdentityPose()}, {Time: 2, Pose: Pose{Scale: geom.Vec3{X: -1, Y: 1, Z: 1}}}}},
		{ID: 11, Cloud: cloud, Visible: []Visibility{{Add: .5, Remove: .5}}},
		{ID: 11, Cloud: cloud, Visible: []Visibility{{}, {Add: 1}}},
	}
	for i, point := range cases {
		if s, err := NewWithPoints(c, cameras, meshes, []PointTrack{point}); err == nil || s != nil {
			t.Fatalf("invalid point case%d accepted: %v %v", i, s, err)
		}
	}
	track := PointTrack{ID: 11, Cloud: cloud}
	if s, err := NewWithPoints(c, cameras, nil, []PointTrack{track, track}); err == nil || s != nil {
		t.Fatal("duplicate point IDs accepted")
	}
	s := mustNewPoints(t, c, cameras, meshes, []PointTrack{track})
	for _, id := range []uint64{7, 11} {
		if pose, visible, err := s.PoseAt(id, 0); err != nil || !visible || pose != IdentityPose() {
			t.Fatal("shared point/mesh ID lookup", id, pose, visible, err)
		}
	}
	if _, _, err := s.PoseAt(12, 0); err == nil {
		t.Fatal("unknown ID accepted")
	}
	// A zero-duration point-only scene and a late first key hold exact values.
	c.Duration = 0
	s = mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, PointKeys: []PointKey{{Cloud: cloud}}}})
	if _, err := s.Sample(0); err != nil {
		t.Fatal(err)
	}
	c.Duration = 2
	s = mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, PointKeys: []PointKey{{Time: 1, Cloud: cloud}}, Visible: []Visibility{{Add: 2}}}})
	for _, time := range []float64{0, 1, math.Nextafter(2, 0), 2} {
		frame, err := s.Sample(time)
		if err != nil || (frame.Objects[0].Bitmap.RGBAAt(20, 15).A != 0) != (time == 2) {
			t.Fatal("endpoint-only point visibility", time, err)
		}
	}
}

func pointCountCloud(t testing.TB, count int) *three.PointCloud {
	t.Helper()
	return mustCloud(t, make([]geom.Vec3, count), make([]color.NRGBA, count), nil)
}

func TestPointTimelineBudgets(t *testing.T) {
	c, cameras, meshes := basicFixture(t)
	one := pointCountCloud(t, 1)
	tracks := make([]PointTrack, three.MaxInstances)
	for i := range tracks {
		tracks[i] = PointTrack{ID: uint64(i + 20), Cloud: one}
	}
	mustNewPoints(t, c, cameras, nil, tracks)
	if s, err := NewWithPoints(c, cameras, meshes, tracks); err == nil || s != nil {
		t.Fatal("combined track budget accepted")
	}
	track := PointTrack{ID: 11, Cloud: one, Keys: make([]PoseKey, MaxKeyframes)}
	if _, err := NewWithPoints(c, cameras, nil, []PointTrack{track}); err == nil {
		t.Fatal("point pose key budget accepted")
	}
	track.Keys, track.Cloud, track.PointKeys = nil, nil, make([]PointKey, MaxKeyframes)
	if _, err := NewWithPoints(c, cameras, nil, []PointTrack{track}); err == nil {
		t.Fatal("point buffer key budget accepted")
	}
	track.PointKeys, track.Cloud, track.Visible = nil, one, make([]Visibility, MaxVisibilityIntervals+1)
	if _, err := NewWithPoints(c, cameras, nil, []PointTrack{track}); err == nil {
		t.Fatal("point visibility budget accepted")
	}
	// Aggregate counts include every track, even hidden or never simultaneous.
	large := pointCountCloud(t, three.MaxPoints)
	track = PointTrack{ID: 11, Cloud: large, Visible: []Visibility{{Add: 2}}}
	mustNewPoints(t, c, cameras, nil, []PointTrack{track})
	if s, err := NewWithPoints(c, cameras, nil, []PointTrack{track, {ID: 12, Cloud: one, Visible: []Visibility{{Remove: 1}}}}); s != nil || err == nil || !strings.Contains(err.Error(), "point count") {
		t.Fatal("aggregate point count accepted", err)
	}
	// 14*131072+29127 = floor(64 MiB/36), leaving four bytes. Each point
	// key counts its full snapshot even when it reuses the same input pointer.
	half := pointCountCloud(t, three.MaxPoints/2)
	keys := make([]PointKey, 14)
	for i := range keys {
		keys[i] = PointKey{Time: 2 * float64(i) / 13, Cloud: half}
	}
	tracks = []PointTrack{{ID: 11, PointKeys: keys}, {ID: 12, Cloud: pointCountCloud(t, 29127)}}
	mustNewPoints(t, c, cameras, nil, tracks)
	tracks = append(tracks, PointTrack{ID: 13, Cloud: one})
	if s, err := NewWithPoints(c, cameras, nil, tracks); s != nil || err == nil || !strings.Contains(err.Error(), "point buffer budget") {
		t.Fatal("owned point snapshot byte budget accepted", err)
	}
	// Camera + mesh pose + point pose + point buffers use a single key budget.
	meshes[0].Keys = make([]PoseKey, MaxKeyframes-2)
	for i := range meshes[0].Keys {
		meshes[0].Keys[i] = PoseKey{Time: 2 * float64(i) / float64(len(meshes[0].Keys)-1), Pose: IdentityPose()}
	}
	track = PointTrack{ID: 11, PointKeys: []PointKey{{Cloud: one}}}
	mustNewPoints(t, c, cameras, meshes, []PointTrack{track})
	track.Keys = []PoseKey{{Pose: IdentityPose()}}
	if _, err := NewWithPoints(c, cameras, meshes, []PointTrack{track}); err == nil {
		t.Fatal("mixed key count accepted")
	}
}

func TestPointTimelineErrorsAndRecovery(t *testing.T) {
	for _, fragment := range []bool{false, true} {
		c, cameras, _ := basicFixture(t)
		alpha := uint8(255)
		message := "point sample budget"
		if fragment {
			c.Renderer.TransparentFragmentLimit = 1
			alpha, message = 128, "fragment budget"
		} else {
			c.Renderer.PointSampleLimit = 1
		}
		cloud := mustCloud(t, []geom.Vec3{{}}, []color.NRGBA{{R: 255, A: alpha}}, []float64{5})
		s := mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, Cloud: cloud, Visible: []Visibility{{Remove: 1}}}})
		for i := range 2 {
			if f, err := s.Sample(.5); f != nil || err == nil || !strings.Contains(err.Error(), message) {
				t.Fatal("missing explicit point budget failure", i, f, err)
			}
			if _, err := s.Sample(1); err != nil {
				t.Fatal("failure poisoned hidden sample", err)
			}
		}
	}
	c, cameras, meshes := basicFixture(t)
	c.Renderer.TransparentFragmentLimit = 1
	cloud := mustCloud(t, []geom.Vec3{{Z: -1}}, []color.NRGBA{{G: 255, A: 128}}, []float64{1.5})
	s := mustNewPoints(t, c, cameras, meshes, []PointTrack{{ID: 11, Cloud: cloud}})
	if _, err := s.Sample(0); err != nil {
		t.Fatal("opaque mesh failed to discard hidden point fragments", err)
	}
	// Validate transformed world bounds even for alpha-zero or clipped points.
	cloud = mustCloud(t, []geom.Vec3{{X: 1e12}}, []color.NRGBA{{}}, nil)
	pose := IdentityPose()
	pose.Scale.X = 2
	s = mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, Cloud: cloud, Keys: []PoseKey{{Pose: pose}}, Visible: []Visibility{{Add: 1}}}})
	if _, err := s.Sample(0); err != nil {
		t.Fatal("hidden transform was unnecessarily rendered", err)
	}
	if f, err := s.Sample(1); f != nil || err == nil || !strings.Contains(err.Error(), "coordinate bounds") {
		t.Fatal("transformed point bounds silently discarded", f, err)
	}
	// Radius interpolation can cross the work budget between valid endpoint
	// values. Failure is local to that sample; there is no implicit radius cap.
	c.Renderer.PointSampleLimit = 100
	a := mustCloud(t, []geom.Vec3{{}}, []color.NRGBA{{R: 255, A: 255}}, []float64{1})
	b := mustCloud(t, []geom.Vec3{{}}, []color.NRGBA{{R: 255, A: 255}}, []float64{64})
	s = mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, PointKeys: []PointKey{{Cloud: a}, {Time: 2, Cloud: b}}}})
	if _, err := s.Sample(1); err == nil {
		t.Fatal("intermediate radius work overflow accepted")
	}
	if _, err := s.Sample(0); err != nil {
		t.Fatal("radius work failure poisoned small endpoint", err)
	}
	// One mesh fragment and one point fragment must charge the same counter.
	c, cameras, meshes = basicFixture(t)
	c.Renderer.Width, c.Renderer.Height = 1, 1
	c.Renderer.TransparentFragmentLimit = 1
	meshes[0].Material.Color.A = 128
	cloud = mustCloud(t, []geom.Vec3{{Z: .5}}, []color.NRGBA{{G: 255, A: 128}}, nil)
	point := PointTrack{ID: 11, Cloud: cloud}
	for _, scene := range []*Scene{mustNew(t, c, cameras, meshes), mustNewPoints(t, c, cameras, nil, []PointTrack{point})} {
		if _, err := scene.Sample(0); err != nil {
			t.Fatal("one fragment should fit", err)
		}
	}
	s = mustNewPoints(t, c, cameras, meshes, []PointTrack{point})
	if frame, err := s.Sample(0); frame != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
		t.Fatal("mesh/point fragment budgets were not shared", err)
	}
}

func TestPointSnapshotConcurrentAndOrder(t *testing.T) {
	c, cameras, meshes := basicFixture(t)
	c.Renderer.Supersampling = 2
	positions := []geom.Vec3{{X: -.4, Z: .3}, {X: .4, Z: -.3}, {Y: .2, Z: .1}}
	colors := []color.NRGBA{{R: 241, A: 128}, {G: 231, A: 255}, {B: 223, A: 87}}
	radii := []float64{3, 4, 5}
	a := mustCloud(t, positions, colors, radii)
	b := mustCloud(t, []geom.Vec3{{X: -.3, Z: -.3}, {X: .3, Z: .4}, {Y: -.2, Z: .2}}, colors, radii)
	last := IdentityPose()
	last.Position, last.Angle = geom.Vec3{X: .2}, .7
	points := []PointTrack{{ID: 11, PointKeys: []PointKey{{Cloud: a}, {Time: 2, Cloud: b}}, Keys: []PoseKey{{Pose: IdentityPose()}, {Time: 2, Pose: last}}, Visible: []Visibility{{Add: .1, Remove: 1.9}}, Axis: geom.Vec3{X: 1, Z: 2}}}
	s := mustNewPoints(t, c, cameras, meshes, points)
	times := []float64{0, .1, .5, 1, 1.9, 2}
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
		for i := range 40 {
			positions[0].X, colors[0].A, radii[0] = math.NaN(), 0, 0
			*a, *b = three.PointCloud{}, three.PointCloud{}
			points[0].ID = 999
			points[0].Axis.X = math.NaN()
			points[0].PointKeys[0].Time = math.NaN()
			points[0].PointKeys[1].Cloud = nil
			points[0].Keys[0].Pose.Position.X = float64(i)
			points[0].Keys[1].Time = math.NaN()
			points[0].Visible[0].Add = math.NaN()
		}
	}()
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := range 15 {
				i := (j*5 + worker) % len(times)
				f, err := s.Sample(times[i])
				if err != nil {
					t.Error(err)
					return
				}
				_, visible, poseErr := s.PoseAt(11, times[i])
				if poseErr != nil || visible != (times[i] >= .1 && times[i] < 1.9) || f.Objects[0].ID != s.BitmapID() || !bytes.Equal(wants[i], bitmapBytes(f.Objects[0].Bitmap)) {
					t.Error("point input alias or unordered mutable sample state", poseErr)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}

func TestPointSubmissionOrderAndUnlitShadows(t *testing.T) {
	c, cameras, meshes := basicFixture(t)
	c.Renderer.Supersampling = 2
	meshes[0].Material.Color.A = 128
	positions := []geom.Vec3{{Z: .6}, {Z: -.6}, {X: .4, Z: .2}, {X: -.3, Z: .1}}
	colors := []color.NRGBA{{R: 255, A: 128}, {B: 255, A: 87}, {G: 255, A: 255}, {R: 93, G: 173, B: 231, A: 173}}
	radii := []float64{5, 7, 3, 4}
	track := func(id uint64, from, to int) PointTrack {
		return PointTrack{ID: id, Cloud: mustCloud(t, positions[from:to], colors[from:to], radii[from:to])}
	}
	s := mustNewPoints(t, c, cameras, meshes, []PointTrack{track(11, 0, 2), track(12, 2, 4)})
	want, _ := s.Sample(1)
	for i := range len(positions) / 2 {
		j := len(positions) - i - 1
		positions[i], positions[j], colors[i], colors[j], radii[i], radii[j] = positions[j], positions[i], colors[j], colors[i], radii[j], radii[i]
	}
	s = mustNewPoints(t, c, cameras, meshes, []PointTrack{track(12, 0, 2), track(11, 2, 4)})
	got, _ := s.Sample(1)
	if !bytes.Equal(bitmapBytes(want.Objects[0].Bitmap), bitmapBytes(got.Objects[0].Bitmap)) {
		t.Fatal("mixed transparent depth changed with cloud/point order")
	}
	// Point-only pixels are unaffected by any mesh-light or shadow setting.
	for _, mode := range []string{"dark", "point", "shadow"} {
		cfg := c
		cfg.Renderer.Light = three.Light{}
		if mode == "point" {
			cfg.Renderer.Light = three.Light{Kind: three.PointLight, Position: geom.Vec3{Z: 3}, Diffuse: 1}
		}
		if mode == "shadow" {
			cfg.Renderer.Light = three.Light{Direction: geom.Vec3{Z: 1}, Diffuse: 1}
			cfg.Renderer.Shadow = three.ShadowConfig{Enabled: true, Size: 8, Depth: 20, Resolution: 32, Bias: .01}
		}
		cloud := track(11, 0, 4)
		a := mustNewPoints(t, c, cameras, nil, []PointTrack{cloud})
		b := mustNewPoints(t, cfg, cameras, nil, []PointTrack{cloud})
		fa, ea := a.Sample(1)
		fb, eb := b.Sample(1)
		if ea != nil || eb != nil || !bytes.Equal(bitmapBytes(fa.Objects[0].Bitmap), bitmapBytes(fb.Objects[0].Bitmap)) {
			t.Fatal("points became lit or shadowed", mode, ea, eb)
		}
	}
}

func TestMeshOnlyPreviousContactSheetCompatibility(t *testing.T) {
	c, cameras, meshes := workloadFixture(t, 2)
	old := mustNew(t, c, cameras, meshes)
	withNil := mustNewPoints(t, c, cameras, meshes, nil)
	file, err := os.Open(filepath.Join("..", "tests", "data", "scene3d", "timeline-ssaa2.png"))
	if err != nil {
		t.Fatal(err)
	}
	golden, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i, time := range []float64{0, .5, 1, 1.5, 2} {
		a, err := old.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		b, err := withNil.Sample(time)
		if err != nil || !bytes.Equal(bitmapBytes(a.Objects[0].Bitmap), bitmapBytes(b.Objects[0].Bitmap)) {
			t.Fatal("New compatibility", time, err)
		}
		for y := range c.Renderer.Height {
			for x := range c.Renderer.Width {
				if got, want := a.Objects[0].Bitmap.RGBAAt(x, y), color.RGBAModel.Convert(golden.At(x+i*c.Renderer.Width, y)).(color.RGBA); got != want {
					t.Fatal("previous mesh-only pixels changed", time, x, y, got, want)
				}
			}
		}
	}
}
