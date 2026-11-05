# Trackers, cameras, redraw and DT replay

Nested animation groups, easing and vector indications are described in
[ANIMATION.md](ANIMATION.md). Scene/Replay sound timelines and integer-ms
captions are described in [MEDIA.md](MEDIA.md); frozen replays retain both.

Authoring is mutable and single-threaded. After authoring, Scene and Replay can
sample arbitrary times concurrently. User callbacks and rate functions must be
deterministic and read only captured data; put changes in the private frame or
the provided integration state. Their closures are not made thread safe by GoMath.

## Tracker and redraw

```go
value, err := gomath.NewValueTracker(0)
if err != nil { return err }
dot := gomath.Dot(geom.V(0, 0), .1)
if err := scene.Add(value.Object, dot); err != nil { return err }
id := value.ID
if err := scene.AddRedraw(dot, func(f *gomath.Frame, t float64) (*gomath.Object, error) {
    x, err := f.Value(id)
    if err != nil { return nil, err }
    return gomath.Dot(geom.V(x, math.Sin(x)), .1), nil
}); err != nil { return err }
if err := scene.Play(3, gomath.ValueTo(value, 4)); err != nil { return err }
```

Read `Frame.Value(id)` inside callbacks. The captured authoring tracker already
has its final value after Play. `Object.Scalar` is a finite animatable slot and
does not create visible geometry. `ValueTo` follows the normal rate/window rules.

`AddRedraw` runs after tracks and earlier registered frame updaters. It requires
a scene-owned target, independent valid output, and the same child topology.
Existing child IDs and pointers are retained in the sampled frame; immutable
bitmap assets may change. Missing targets (before appearance or after removal)
are skipped. Changing the output topology returns an error. This is a bounded
redraw API, not general Manim always_redraw equivalence.

An updater can overwrite an animation on the same object. Suspend that redraw
in the relevant time interval or choose a separate display object. The runnable
`dynamics` example stops its dot redraw before ReplacementTransform starts.

## Dynamic display topology

`AddDynamicRedraw(object, draw, DynamicRedrawOptions{})` accepts different child
counts/topologies at different sample times. It replaces only the private
display tree, preserving the root ID. Registration reserves a range of child
IDs; each sample assigns those slots in preorder. They identify ordinal
positions, not semantic correspondence when an earlier sibling is inserted.
Original author-child IDs do not remain in the generated tree. Repeated,
out-of-order and concurrent samples have identical resulting IDs/geometry for
pure callbacks. Independent generated geometry is copied before installation.

Defaults bound each callback output to 4096 objects, 262144 cubics, 16384
contours and depth64. Hard maxima are 65536 objects/contours, 1048576 cubics and
depth256. The iterative preflight checks counts before copying and rejects
cycles, shared children, malformed styles/paths and duplicate generated IDs.
Reserved-slot collisions caused by manually reassigned public IDs elsewhere in
a frame return errors. The authoring tree and original child pointers stay
unchanged. Missing roots are skipped; callback errors propagate.

As with AddRedraw, this callback runs after authored tracks and earlier
registered updaters. It supplies the complete replacement display state and can
overwrite an animation's geometry/style; it does not automatically inherit an
authored parent transform or suspend itself during animations. Read sampled
values from the frame or explicitly gate the callback for that interaction.
Later callbacks see the generated family. This absolute-time bounded API does
not establish general Manim always_redraw/DT updater ordering parity.
Run the `dynamic-topology` scene for a family growing from zero to sixteen bars.

## Camera and path movement

`Scene.SetCamera(Camera2D{Center: ..., FrameWidth: ..., Rotation: ...})` captures
an instantaneous view at the authoring cursor. `AnimateCamera` advances the
cursor and can accept simultaneous object animations. `AnimateCameraWithRate`
chooses camera easing explicitly. An initial SetCamera is required for animation.
Width/center/rotation interpolate as written, including full turns. Rotation is
in radians, center must have Z=0, and height follows the output aspect ratio.

Camera sampling happens before frame updaters. Render and SVG export project a
private snapshot; geometry and world stroke widths are preserved. Rasterizer
configuration is restored after each frame, including failures. This is one
orthographic camera per frame; multi-camera and fixed-in-frame overlays remain
future work. Independent 3D camera/depth rendering lives in `three`.

`MoveAlongPath(object, route)` captures a single childless vector path during
Play, then moves the object's center by numerical inverse arc length. The
default rate is Linear; `WithRate` and `WithWindow` apply normally. It preserves
orientation, shapes and subtree identity. Contour gaps are jumps. The tolerance
overload selects an absolute world-unit accuracy budget; invalid/empty routes
fail atomically. This is more uniform than Manim's per-cubic approximate
length-weighted parameter mapping, so nonlinear-curve timing is not identical.

`ReplacementTransform` keeps source identity during the morph, then installs the
actual target pointer in the source's root/child position at the Play endpoint.
The target tree must not already be owned by the scene. Subsequent animations
use the target. FadeOut and Remove also remove nested children while preserving
surviving child pointers and order. General semantic matching remains pending.

## Fixed-step integration

```go
state := gomath.Group()
state.Scalar = 0
if err := scene.Add(state); err != nil { return err }
if err := scene.Wait(3); err != nil { return err }
replay, err := gomath.CompileReplay(scene, gomath.DefaultReplayOptions(),
    gomath.Integration{Object: state, Update: func(o *gomath.Object, dt, t float64) error {
        o.Scalar += dt
        return nil
    }})
if err != nil { return err }
// render.RenderScene(ctx, replay, config, sink)
```

CompileReplay freezes tracks, snapshots, camera data and callback lists. It
integrates globally aligned fixed steps (default 1/120 s), with a shorter final
step, and stores geometry/state checkpoints every 30 steps by default. Sampling
restores the preceding checkpoint, replays full steps, then uses a partial step
at the requested time. It does not reuse an earlier sampled frame. Display
updaters run after integration and their changes do not feed into persistent
integration state. A partial-step sample may differ from interpolating the two
nearest full-step states; the caller chooses its numerical integration method.

Each integration target must exist unchanged throughout the authored timeline.
It cannot overlap another integration target or an authored animation subtree,
and updates must preserve all IDs and child topology. Removal, replacement,
baseline mutation, nonfinite results and ambiguous ownership return errors.
This initial restriction is intentional and is not Manim's arbitrary dt updater
or update-order compatibility. Use separate scalar state and redraw display
objects, as the runnable symplectic-Euler `replay` oscillator does.

Step/CheckpointEvery/MaxSteps bound work and checkpoint frequency. MaxSteps must
be 1..1,000,000. Checkpoint memory grows with state size and stored checkpoints;
it is separate from the renderer's bounded frame queue and is not constant for
arbitrarily long scenes. There is no implicit file cache or user checkpoint I/O.
Compiled callbacks still share their captured references; do not mutate them.
