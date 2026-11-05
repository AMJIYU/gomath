// Package scene3d compiles immutable, bounded mesh/point/camera timelines
// into real CPU-rendered bitmap Frames. Scene satisfies render.FrameSource
// without callbacks, integrated state, external runtimes or core Scene changes.
package scene3d

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/three"
	"slices"
)

const (
	MaxDuration            = 1e9
	MaxKeyframes           = 65536            // total camera, pose, buffer, material and rotation keys
	MaxVisibilityIntervals = 65536            // total across every track
	MaxTextureBytes        = 64 * 1024 * 1024 // distinct referenced texture payload
	MinFrameWidth          = 1e-6
	MaxFrameWidth          = 1e6
)

type Config struct {
	Duration   float64
	FrameWidth float64 // bitmap width in 2D overlay world units; default helper uses 8
	Renderer   three.Config
}

func DefaultConfig(width, height int, duration float64) Config {
	return Config{Duration: duration, FrameWidth: 8, Renderer: three.DefaultConfig(width, height)}
}

// Scene is immutable after New. No mutable input slice is retained or exposed.
// Concurrent arbitrary-order Sample, CameraAt and PoseAt calls are supported.
type Scene struct {
	duration, frameWidth float64
	renderer             *three.Renderer
	cameras              []CameraKey
	orientedCamera       *OrientedCameraTrack
	tracks               []MeshTrack
	points               []pointTrack
	index                map[uint64]int
	plane                *gomath.Object
}

// New copies all authoring data synchronously. Finish authoring before calling
// it; caller mutations after return cannot affect sampling. Texture assets are
// already immutable and shared. Validation of sampled camera degeneracy and
// transformed geometry/shadow/fragment work remains in CameraAt/three.Render;
// New does not claim to pre-render or validate every real-valued instant.
func New(config Config, cameras []CameraKey, tracks []MeshTrack) (*Scene, error) {
	return NewWithPoints(config, cameras, tracks, nil)
}

// NewWithPoints adds point tracks to New's immutable timeline. Points and
// meshes share IDs, pose/visibility semantics, one raster depth buffer and one
// transparent sort. Each point buffer is copied, including every PointKey.
// The combined scene has at most 1024 tracks and three.MaxPoints point indices.
func NewWithPoints(config Config, cameras []CameraKey, tracks []MeshTrack, points []PointTrack) (*Scene, error) {
	return newScene(config, cameras, nil, tracks, points)
}

func newScene(config Config, cameras []CameraKey, oriented *OrientedCameraTrack, tracks []MeshTrack, points []PointTrack) (*Scene, error) {
	if !finite(config.Duration) || config.Duration < 0 || config.Duration > MaxDuration || !finite(config.FrameWidth) || config.FrameWidth < MinFrameWidth || config.FrameWidth > MaxFrameWidth {
		return nil, fmt.Errorf("scene3d: duration must be in [0,%g] and frame width in [%g,%g]", MaxDuration, MinFrameWidth, MaxFrameWidth)
	}
	cameraCount := len(cameras)
	if oriented != nil {
		cameraCount = len(oriented.Keys)
	}
	if len(tracks) > three.MaxInstances || len(points) > three.MaxInstances-len(tracks) || cameraCount > MaxKeyframes {
		return nil, fmt.Errorf("scene3d: at most %d tracks and %d total keyframes", three.MaxInstances, MaxKeyframes)
	}
	renderer, err := three.NewRenderer(config.Renderer)
	if err != nil {
		return nil, fmt.Errorf("scene3d: renderer: %w", err)
	}
	if oriented == nil {
		if err := validateCameraKeys(cameras, config.Duration); err != nil {
			return nil, err
		}
	} else {
		if len(cameras) != 0 {
			return nil, fmt.Errorf("scene3d: cannot mix oriented and look-at camera keys")
		}
		if err := oriented.validate(config.Duration); err != nil {
			return nil, err
		}
	}
	// Check aggregate sizes before validating/copying the potentially large
	// slices. All tracks count, even if they never appear at the same instant.
	vertices, triangles, keys, spans, meshBytes := 0, 0, cameraCount, 0, 0
	for _, track := range tracks {
		mesh, err := track.meshSource()
		if err != nil {
			return nil, err
		}
		if len(mesh.Vertices) > three.MaxVertices-vertices || len(mesh.Triangles) > three.MaxTriangles-triangles || len(track.Keys) > MaxKeyframes-keys || len(track.MeshKeys) > MaxKeyframes-keys-len(track.Keys) || len(track.MaterialKeys) > MaxKeyframes-keys-len(track.Keys)-len(track.MeshKeys) || len(track.RotationKeys) > MaxKeyframes-keys-len(track.Keys)-len(track.MeshKeys)-len(track.MaterialKeys) || len(track.Visible) > MaxVisibilityIntervals-spans {
			return nil, fmt.Errorf("scene3d: aggregate geometry/keyframe/visibility budget exceeded")
		}
		n, err := track.meshBudget(MaxMeshBufferBytes - meshBytes)
		if err != nil {
			return nil, err
		}
		meshBytes += n
		vertices += len(mesh.Vertices)
		triangles += len(mesh.Triangles)
		keys += len(track.Keys) + len(track.MeshKeys) + len(track.MaterialKeys) + len(track.RotationKeys)
		spans += len(track.Visible)
	}
	pointCount, pointBytes := 0, 0
	for _, track := range points {
		if len(track.Keys) > MaxKeyframes-keys || len(track.PointKeys) > MaxKeyframes-keys-len(track.Keys) || len(track.RotationKeys) > MaxKeyframes-keys-len(track.Keys)-len(track.PointKeys) || len(track.Visible) > MaxVisibilityIntervals-spans {
			return nil, fmt.Errorf("scene3d: aggregate keyframe/visibility budget exceeded")
		}
		n, bytes, err := track.budget(config.Duration, MaxPointBufferBytes-pointBytes)
		if err != nil {
			return nil, err
		}
		if n > three.MaxPoints-pointCount {
			return nil, fmt.Errorf("scene3d: aggregate point count exceeds %d", three.MaxPoints)
		}
		pointCount += n
		pointBytes += bytes
		keys += len(track.Keys) + len(track.PointKeys) + len(track.RotationKeys)
		spans += len(track.Visible)
	}
	index := make(map[uint64]int, len(tracks)+len(points))
	textures := make(map[*three.Texture]bool)
	textureBytes := 0
	for i, track := range tracks {
		if _, exists := index[track.ID]; exists {
			return nil, fmt.Errorf("scene3d: duplicate mesh track ID %d", track.ID)
		}
		index[track.ID] = i
		if _, err := rotationAxis(track.Axis); err != nil {
			return nil, fmt.Errorf("scene3d: track %d: %w", track.ID, err)
		}
		if err := track.validate(config.Duration); err != nil {
			return nil, err
		}
		if tex := track.Material.Texture; tex != nil && !textures[tex] {
			b := tex.Bounds()
			n := b.Dx() * b.Dy() * 4
			if n > MaxTextureBytes-textureBytes {
				return nil, fmt.Errorf("scene3d: referenced textures exceed %d bytes", MaxTextureBytes)
			}
			textureBytes += n
			textures[tex] = true
		}
	}
	for i, track := range points {
		if _, exists := index[track.ID]; exists {
			return nil, fmt.Errorf("scene3d: duplicate track ID %d", track.ID)
		}
		index[track.ID] = len(tracks) + i
	}
	s := &Scene{duration: config.Duration, frameWidth: config.FrameWidth, renderer: renderer, cameras: slices.Clone(cameras), tracks: make([]MeshTrack, len(tracks)), index: index}
	if oriented != nil {
		s.orientedCamera = snapshotOrientedCamera(*oriented)
	}
	for i, track := range tracks {
		s.tracks[i] = snapshotMeshTrack(track)
	}
	if len(points) != 0 {
		s.points = make([]pointTrack, len(points))
		for i, track := range points {
			s.points[i], err = snapshotPointTrack(track)
			if err != nil {
				return nil, err
			}
		}
	}
	// Allocate the actual affine image-plane geometry and its identity once.
	// Each sample clones that geometry and attaches its own rendered bitmap.
	height := config.FrameWidth * float64(config.Renderer.Height) / float64(config.Renderer.Width)
	s.plane = gomath.Rectangle(config.FrameWidth, height).SetFill(gomath.Transparent).SetStroke(gomath.Transparent, 0)
	s.plane.Name = "Scene3D"
	return s, nil
}

// Duration returns zero for a nil Scene; its Sample still reports an error.
func (s *Scene) Duration() float64 {
	if s == nil {
		return 0
	}
	return s.duration
}

// BitmapID is the stable gomath.Object identity of this scene's bitmap plane.
// Mesh/point track IDs share a scene-local namespace separate from Frame.Objects.
func (s *Scene) BitmapID() uint64 {
	if s == nil || s.plane == nil {
		return 0
	}
	return s.plane.ID
}

func (s *Scene) checkTime(time float64) error {
	if s == nil || s.renderer == nil || len(s.cameras) == 0 && (s.orientedCamera == nil || len(s.orientedCamera.Keys) == 0) || s.plane == nil {
		return fmt.Errorf("scene3d: nil or uninitialized scene")
	}
	if !validTime(time, s.duration) {
		return fmt.Errorf("scene3d: sample time %g outside [0,%g]", time, s.duration)
	}
	return nil
}

// CameraAt returns the interpolated camera by value, with the same validation
// as Sample. Before/after the first/last key, the respective endpoint holds.
func (s *Scene) CameraAt(time float64) (three.Camera, error) {
	if err := s.checkTime(time); err != nil {
		return three.Camera{}, err
	}
	if s.orientedCamera != nil {
		return s.orientedCamera.at(time)
	}
	return cameraAt(s.cameras, time)
}

// PoseAt returns a value and visibility for a scene-local mesh or point ID.
// Unknown IDs and invalid times error. No owned geometry/buffers are exposed.
func (s *Scene) PoseAt(id uint64, time float64) (Pose, bool, error) {
	if err := s.checkTime(time); err != nil {
		return Pose{}, false, err
	}
	i, ok := s.index[id]
	if !ok {
		return Pose{}, false, fmt.Errorf("scene3d: unknown track ID %d", id)
	}
	if i >= len(s.tracks) {
		t := s.points[i-len(s.tracks)]
		return poseAt(t.keys, time), visibleAt(t.visible, time), nil
	}
	pose, visible := s.tracks[i].at(time)
	return pose, visible, nil
}

// Sample renders all visible meshes and point clouds in one three call and
// returns one immutable bitmap asset on an independent object/path/camera.
// The 2D camera fixes FrameWidth, so the bitmap fills matching-aspect media
// outputs. Other sizes/aspects use the existing 2D bitmap resampling/cropping
// policy. Append ordinary 2D objects to this returned frame for overlays.
// All failures return a nil frame; no partially rendered scene is exposed.
func (s *Scene) Sample(time float64) (*gomath.Frame, error) {
	camera, err := s.CameraAt(time)
	if err != nil {
		return nil, err
	}
	instances := make([]three.Instance, 0, len(s.tracks))
	for _, track := range s.tracks {
		pose, visible := track.at(time)
		if visible {
			mesh, err := track.meshAt(time)
			if err != nil {
				return nil, err
			}
			var model geom.Mat4
			model, err = trackModel(pose, track.Axis, track.RotationKeys, time)
			if err != nil {
				return nil, fmt.Errorf("scene3d: track %d at %g: %w", track.ID, time, err)
			}
			instances = append(instances, three.Instance{Mesh: mesh, Material: track.materialAt(time), Model: model})
		}
	}
	var clouds []*three.PointCloud
	if len(s.points) != 0 {
		clouds = make([]*three.PointCloud, 0, len(s.points))
		for _, track := range s.points {
			if visibleAt(track.visible, time) {
				cloud, err := track.cloudAt(time, poseAt(track.keys, time))
				if err != nil {
					return nil, err
				}
				clouds = append(clouds, cloud)
			}
		}
	}
	pixels, err := s.renderer.RenderWithPoints(camera, clouds, instances...)
	if err != nil {
		return nil, fmt.Errorf("scene3d: render at %g: %w", time, err)
	}
	bitmap, err := gomath.NewBitmap(pixels)
	if err != nil {
		return nil, fmt.Errorf("scene3d: bitmap at %g: %w", time, err)
	}
	object := *s.plane
	object.Path, object.Bitmap = s.plane.Path.Clone(), bitmap
	return &gomath.Frame{Time: time, Objects: []*gomath.Object{&object}, Camera: &gomath.Camera2D{FrameWidth: s.frameWidth}}, nil
}
