package scene3d

import (
	"bytes"
	"gomath/geom"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestRotationKeysValidation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*geom.Vec3, *[]PoseKey, *[]RotationKey)
	}{
		{"axis", func(a *geom.Vec3, _ *[]PoseKey, _ *[]RotationKey) { *a = geom.Vec3{Z: 1} }},
		{"angle", func(_ *geom.Vec3, p *[]PoseKey, _ *[]RotationKey) {
			pose := IdentityPose()
			pose.Angle = .1
			*p = []PoseKey{{Pose: pose}}
		}},
		{"zero", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[1].Rotation = geom.Quat{} }},
		{"nan", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[1].Rotation.X = math.NaN() }},
		{"infinite", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[1].Rotation.Z = math.Inf(1) }},
		{"duplicate", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[1].Time = 0 }},
		{"decreasing", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[0].Time = 1; (*k)[1].Time = .5 }},
		{"negative", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[0].Time = -1 }},
		{"late", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[1].Time = 3 }},
		{"nan-time", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[1].Time = math.NaN() }},
		{"infinite-time", func(_ *geom.Vec3, _ *[]PoseKey, k *[]RotationKey) { (*k)[1].Time = math.Inf(1) }},
	}
	for _, kind := range []string{"mesh", "point"} {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				c, cameras, meshes := basicFixture(t)
				keys := []RotationKey{{Rotation: geom.IdentityQuat()}, {Time: 2, Rotation: geom.Quat{W: 1, X: 1}}}
				if kind == "mesh" {
					meshes[0].RotationKeys = keys
					meshes[0].Visible = []Visibility{{Add: 2}}
					tc.edit(&meshes[0].Axis, &meshes[0].Keys, &meshes[0].RotationKeys)
					if s, err := New(c, cameras, meshes); s != nil || err == nil {
						t.Fatal("invalid hidden mesh rotation accepted", s, err)
					}
				} else {
					points := []PointTrack{{ID: 11, Cloud: pointCountCloud(t, 1), RotationKeys: keys, Visible: []Visibility{{Add: 2}}}}
					tc.edit(&points[0].Axis, &points[0].Keys, &points[0].RotationKeys)
					if s, err := NewWithPoints(c, cameras, meshes, points); s != nil || err == nil {
						t.Fatal("invalid hidden point rotation accepted", s, err)
					}
				}
			})
		}
	}
}

func TestRotationEndpointsLegacyTurnsAndBudgets(t *testing.T) {
	c, cameras, meshes := basicFixture(t)
	first, last := IdentityPose(), IdentityPose()
	last.Angle = 2 * math.Pi
	meshes[0].Keys = []PoseKey{{Pose: first}, {Time: 2, Pose: last}}
	legacy := mustNew(t, c, cameras, meshes)
	last.Angle = 0
	meshes[0].Keys[1].Pose = last
	meshes[0].RotationKeys = []RotationKey{{Time: .25, Rotation: geom.Quat{W: 1e308}}, {Time: 1.75, Rotation: geom.Quat{W: -math.SmallestNonzeroFloat64}}}
	s := mustNew(t, c, cameras, meshes)
	for _, time := range []float64{0, .25, 1, 1.75, 2} {
		q, err := s.RotationAt(7, time)
		if err != nil {
			t.Fatal(err)
		}
		if time < 1.75 && q != geom.IdentityQuat() || time >= 1.75 && q != (geom.Quat{W: -1}) {
			t.Fatal("held/exact or antipodal interpolation", time, q)
		}
		pose, _, err := s.PoseAt(7, time)
		if err != nil || pose != IdentityPose() {
			t.Fatal(pose, err)
		}
	}
	lmid, _ := legacy.Sample(1)
	qmid, _ := s.Sample(1)
	if bytes.Equal(bitmapBytes(lmid.Objects[0].Bitmap), bitmapBytes(qmid.Objects[0].Bitmap)) {
		t.Fatal("legacy full turn was replaced by shortest arc")
	}
	for _, time := range []float64{0, 2} {
		lf, _ := legacy.Sample(time)
		qf, _ := s.Sample(time)
		if !bytes.Equal(bitmapBytes(lf.Objects[0].Bitmap), bitmapBytes(qf.Objects[0].Bitmap)) {
			t.Fatal("legacy/quat endpoint orientation mismatch")
		}
	}
	for _, time := range []float64{-1, 3, math.NaN(), math.Inf(1)} {
		if _, err := s.RotationAt(7, time); err == nil {
			t.Fatal("invalid query time accepted")
		}
	}
	if _, err := s.RotationAt(999, 0); err == nil {
		t.Fatal("unknown id accepted")
	}
	q, err := legacy.RotationAt(7, 1)
	if err != nil || math.Abs(q.Z-1) > 1e-14 {
		t.Fatal("legacy query", q, err)
	}
	c.Duration = 0
	meshes[0].Keys = nil
	meshes[0].RotationKeys = []RotationKey{{Rotation: geom.Quat{X: 1e308}}}
	s = mustNew(t, c, cameras, meshes)
	if _, err := s.Sample(0); err != nil {
		t.Fatal(err)
	}

	// Exactly 65536 mixed camera, pose, mesh/material/point buffer/orientation keys.
	c, cameras, meshes = basicFixture(t)
	meshes[0].MeshKeys = []MeshKey{{Mesh: meshes[0].Mesh}}
	meshes[0].Mesh = nil
	meshes[0].Keys = []PoseKey{{Pose: IdentityPose()}}
	meshes[0].MaterialKeys = []MaterialKey{{Color: color.NRGBA{A: 255}}}
	meshes[0].RotationKeys = make([]RotationKey, MaxKeyframes-7)
	for i := range meshes[0].RotationKeys {
		meshes[0].RotationKeys[i] = RotationKey{Time: 2 * float64(i) / float64(len(meshes[0].RotationKeys)-1), Rotation: geom.IdentityQuat()}
	}
	points := []PointTrack{{ID: 11, PointKeys: []PointKey{{Cloud: pointCountCloud(t, 1)}}, Keys: []PoseKey{{Pose: IdentityPose()}}, RotationKeys: []RotationKey{{Rotation: geom.IdentityQuat()}}}}
	mustNewPoints(t, c, cameras, meshes, points)
	points[0].RotationKeys = append(points[0].RotationKeys, RotationKey{Time: 2, Rotation: geom.IdentityQuat()})
	if _, err := NewWithPoints(c, cameras, meshes, points); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("combined rotation budget accepted", err)
	}
	meshes[0].RotationKeys = make([]RotationKey, MaxKeyframes)
	if _, err := New(c, cameras, meshes); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("mesh rotation count accepted", err)
	}
	points[0].RotationKeys = make([]RotationKey, MaxKeyframes)
	if _, err := NewWithPoints(c, cameras, nil, points); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("point rotation count accepted", err)
	}
}

func TestRotationSampleBoundsFailureRecovery(t *testing.T) {
	c, cameras, _ := basicFixture(t)
	q, err := geom.QuatFromAxisAngle(geom.Vec3{Z: 1}, math.Pi/2)
	if err != nil {
		t.Fatal(err)
	}
	cloud := mustCloud(t, []geom.Vec3{{X: .8e12, Y: -.8e12}}, []color.NRGBA{{A: 255}}, nil)
	s := mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, Cloud: cloud, RotationKeys: []RotationKey{{Rotation: geom.IdentityQuat()}, {Time: 2, Rotation: q}}}})
	for _, time := range []float64{0, 1, 2, 0, 1, 2} {
		frame, err := s.Sample(time)
		if time == 1 {
			if frame != nil || err == nil || !strings.Contains(err.Error(), "point track 11") {
				t.Fatal("out-of-bounds intermediate point hidden", frame, err)
			}
		} else if err != nil {
			t.Fatal("failed rotation sample poisoned endpoint", time, err)
		}
	}
}

func TestRotationInputIsolationAndConcurrentOrder(t *testing.T) {
	f, keys := loadOrientation(t)
	c, cameras, meshes := basicFixture(t)
	c.Renderer.Supersampling = 2
	meshes[0].RotationKeys = append([]RotationKey(nil), keys...)
	points := []PointTrack{{ID: 11, Cloud: mustCloud(t, []geom.Vec3{{X: .7, Y: -.3, Z: .5}}, []color.NRGBA{{B: 255, A: 129}}, []float64{3.1}), RotationKeys: append([]RotationKey(nil), keys...)}}
	s := mustNewPoints(t, c, cameras, meshes, points)
	wants := make([][]byte, len(f.Samples))
	queries := make([]geom.Quat, len(f.Samples))
	for i, sample := range f.Samples {
		frame, err := s.Sample(sample.Time)
		if err != nil {
			t.Fatal(err)
		}
		wants[i] = bitmapBytes(frame.Objects[0].Bitmap)
		queries[i], err = s.RotationAt(11, sample.Time)
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for range 80 {
			meshes[0].RotationKeys[0].Rotation = geom.Quat{W: math.NaN()}
			meshes[0].RotationKeys[1].Time = -1
			points[0].RotationKeys[1].Rotation = geom.Quat{}
			points[0].RotationKeys[2].Time = math.NaN()
		}
		meshes[0].RotationKeys = nil
		points[0].RotationKeys = nil
	}()
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := range 35 {
				index := (worker + 13*j) % len(f.Samples)
				time := f.Samples[index].Time
				frame, err := s.Sample(time)
				if err != nil {
					t.Error(err)
					return
				}
				q, err := s.RotationAt(11, time)
				if err != nil || q != queries[index] {
					t.Error("rotation query shared storage", q, err)
					return
				}
				if frame.Objects[0].ID != s.BitmapID() || !bytes.Equal(wants[index], bitmapBytes(frame.Objects[0].Bitmap)) {
					t.Error("orientation sampling depends on inputs/order")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}
