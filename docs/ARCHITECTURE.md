# Architecture

## Data flow

```text
Go scene authoring
  -> Object tree (identity + paths + styles)
  -> Play snapshots and immutable animation tracks
  -> Scene.Sample(time) or frozen Replay.Sample(time), private Frame + camera
  -> optional checkpointed DT state merge
  -> absolute-time FrameUpdaters
  -> private 2D camera projection (raster and SVG)
  -> worker-local Rasterizer
  -> bounded ordered frame delivery
  -> Sink (PNG sequence / GIF / MP4)
```

`geom` and `path` have no knowledge of scene execution, I/O, fonts or codecs.
The root package depends on these mathematical primitives plus independent audio/subtitle value APIs. `text`, `mathtex`,
`graph`, `svg`, `matrix`, `table`, `network`, `field` and `rasterimage` create root objects. `linalg` is an independent numerical layer, including checked general real Schur factors, complex right eigenvectors and immutable complex dense/LU arithmetic. `three` consumes mesh/camera data and produces immutable-frame images for the bitmap pipeline; `scene3d` compiles copied mesh/point/camera keyframes into an independent immutable FrameSource. `render` consumes frames. `export` owns
output transactions and codecs. `internal/h264` has no process or native-library
dependency. `compat` is linked into the CLI, not the mathematical core.

## Identity and state

Every object has an authoring ID. `Copy` recursively assigns new IDs. Internal
snapshots retain IDs so animation tracks can locate the right object.
`Transform` interpolates paths and styles while retaining the source object and
existing child references. Missing contours are seeded as degenerate paths and
curves are aligned by subdivision; shape-semantic contour matching is pending.

`Play` validates inputs and evaluates endpoint state on staged snapshots before
committing. Conflicting targets are rejected. User-defined rate functions are
expected to be deterministic, finite and safe for concurrent invocation.

Sampling uses binary search over segments, clones one segment's state, applies
tracks, then invokes frame updaters. Snapshots intentionally favor correctness
over minimal allocations in this initial implementation. Packed point buffers,
reusable frame arenas and an immutable compiled scene representation are future
optimizations, subject to isolation and concurrency tests.

## Time semantics

`Play(duration, animations...)` advances the scene timeline. Individual tracks
can occupy normalized windows inside a segment. Frame sample times are `i/FPS`
for `i` in `[0, ceil(duration*FPS))`; the authoring endpoint is independently
available through `Sample(Duration())`. Nested animation groups compile once
into private per-leaf clocks with unequal runtimes and recursive endpoint
finishing. Parallel writable subtrees remain disjoint. Shared-object Succession
stages each step on canonical private objects, defers relative preparation and
commits only after all steps validate, with per-step cleanup. It currently
requires linear full-window sequence clocks and no sequence inside parallel
groups. See ANIMATION.md. Encoded duration can exceed
scene duration by less than one frame. A zero-duration scene emits one frame.

Segment boundaries resolve to the next segment's initial state. Objects added
between segments become visible at that boundary. FadeOut removes a root or
nested child at the Play endpoint. ReplacementTransform installs the actual
target pointer/IDs in the source's position at that endpoint; intermediate
samples retain source IDs. Planned endpoint topology is validated before commit.

ValueTracker uses the finite Object.Scalar slot. AddRedraw replaces sampled
geometry while retaining IDs and fixed child topology. AddDynamicRedraw instead
reserves deterministic preorder child-ID slots and validates bounded changing
display topology before copying; it preserves only the root's original ID.
Both callbacks run after tracks and may override them. CompileReplay freezes
authoring tracks, preintegrates fixed steps, and stores object-state checkpoints.
Samples replay from an immutable checkpoint and then invoke display updaters;
display changes do not feed back into integration. Reserved integration subtrees
cannot overlap authored animation, mutation/removal or other integrations.
Checkpoint memory scales with state and checkpoint count. Callback closures
remain subject to the purity/concurrency contract. See UPDATERS.md for limits.

MoveAlongPath captures an immutable measured path at Play and uses bounded
inverse arc-length queries, preserving shape/orientation and subtree identity.
It differs from Manim's length-weighted cubic-parameter mapping. Original Create
uses parameter-space Partial; PartialByLength is an explicit separate path API.

`path.Boolean` combines bounded closed XY polygons using exact rational edge
intersections and winding classification, then verifies that float64 output
rounding preserves boundary topology. Each operand chooses nonzero or evenodd;
output contours put the filled region on their left for ordinary nonzero
rendering. Curved cubic inputs reject; callers must explicitly provide their
own polygon approximation. Work, arithmetic width and allocation accounting are
bounded independently. This API does not change default path or fill semantics.
See BOOLEAN.md for supported degeneracies and failure conditions.

`sparse` owns immutable strict CSR independently of `linalg`: transpose,
addition/subtraction and matrix-vector products allocate checked outputs.
Context-aware conjugate gradients optionally use Jacobi and accept a candidate
only after recomputing its original-system residual. Exact symmetry is checked;
SPD is a caller prerequisite. Fixed-width extended accumulators protect product
sums across binary64 scales; recurrence coefficients have explicit 53-bit
significands. Strict vector range, storage/work/iteration limits and cancellation
return errors without partial results. SpGEMM merges selected right-hand rows with a sparse deterministic heap and
performs two bounded exact-sum passes, reserving cursors, scratch and output
payload before publication. No output-column-sized marker array is allocated.
See SPARSE.md for the numerical contract.

SolveBiCGSTAB independently handles nonsymmetric real CSR with optional right
Jacobi and copied initial guess. Extended-exponent 53-bit recurrences use fixed
wide product sums, original-system checks at each half/full candidate and bounded
restarts. Breakdown, range, budget and cancellation expose no partial solution.
It makes no SPD or universal convergence claim.

## Rendering

Coordinates are Y-up world units, projected orthographically onto a 2D frame.
The root vector renderer does not use Z for depth occlusion; `ZIndex` controls
2D paint ordering. Sampled Camera2D translates/rotates a private geometry snapshot
and chooses an effective frame width. The worker restores its config after each
Render, so camera changes do not leak. The separate three renderer implements
real depth and perspective before its RGBA output joins the 2D pipeline.

Immutable world-space Clip paths apply to vector fill/stroke and bitmap pixels.
Descendant masks intersect with ancestors after leaf ZIndex ordering. Affine
object/camera transforms update private clip geometry; compatible morphs align
and interpolate clip controls. Bounded local transparent layers and nonzero
coverage masks implement raster clipping; SVG output retains the same ancestry
with explicit clipPath definitions. See CLIPPING.md for work/memory and AA limits.

Explicit EvenOdd clips instead integrate paired boundaries in bounded scan bands
of flattened polygons. Vertex/intersection/pixel-row events define fixed-order
bands, with checked float64 areas and shared work/event budgets. Rule survives
camera projection, alignment and snapshots; mismatched-rule morphs reject
atomically. Ordinary object Style.FillRule now chooses nonzero or evenodd for solid, gradient and partial/revealed fills. Evenodd reuses this integrator and the same frame budgets as clips. Stroke outlines remain nonzero; mixed-rule morphs reject before commit, with FadeTransform available as separate layers.

Gradient fills keep immutable stops and a complete affine paint frame. Object
and camera transforms move this frame together with geometry; rasterization
inverse-maps covered samples into linear or concentric radial coordinates and
evaluates straight sRGB stops before premultiplied composition. SVG exports the
same frame as a paint server. Only pad spread is supported; compatible animation
requires the same gradient kind and stop positions. See GRADIENTS.md.

The renderer uses `golang.org/x/image/vector` for anti-aliased nonzero path fill.
Fill and stroke share adaptive path flattening with a default 0.02 pixel error
bound, configurable through `Config.CurveTolerance`. Stroke outlines are built
in Go using miter/round/bevel joins and butt/round/square caps. Miter limit is
configurable (default four); ROI padding accounts for its possible extent.
Native defaults keep round caps, while SVG import defaults to butt caps.
Each object's affected pixel rectangle is clipped to the frame;
worker-local raster buffers cover only that rectangle. This avoids clearing and
compositing a whole frame for every small object.

Frame workers own rasterizer buffers. At most `2*workers` frames can be active
or waiting for an earlier frame. Results are delivered in index order. On
failure or cancellation, workers stop and the sink aborts its partial output.

The independent three renderer validates indexed geometry and affine models,
uses inverse-transpose normals, clips triangles against all six view planes,
and interpolates perspective-correct depth/normals into a fresh Z-buffer/RGBA
frame. It supports Lambert/ambient shading and optional culling. Opaque surfaces
resolve first; transparent fragments strictly in front of opaque depth sort per
sample and composite back-to-front with a hard fragment budget. Immutable copied
textures use clipped, perspective-correct UVs; bilinear filtering uses
premultiplied texels. Mixed texel alpha joins the same opaque/transparent passes.
Supersampling at linear scale 1/2/4 gives each sample depth/shading and resolves
premultiplied RGBA by box averaging. Working buffers/dimensions and geometry are
bounded explicitly; concurrency multiplies per-call storage. Optional directional
hard shadow maps support opaque meshes. A single world-space point light uses
perspective-correct per-sample positions, Lambert shading and distance attenuation;
A separate opt-in mode copies/sorts at most16 colored world point lights, adds ambient once and limits per-frame source evaluations across both mesh passes; point-light shadows reject. Circular point-cloud disks share
mesh depth/transparency/SSAA but remain unlit and do not participate in shadows.
Mipmaps, GPU and full ThreeDScene semantics remain pending. See three/README.md.

`scene3d.New` snapshots every authoring mesh/key/visibility slice. Arbitrary-order
sampling reconstructs TRS poses with real fixed-axis angle rotation and signed
nonuniform scale, interpolates look-at camera values, applies add/remove intervals,
and renders all visible meshes into one bitmap. `NewWithPoints` additionally
snapshots fixed-count point keys, interpolating positions, premultiplied colors
and output-pixel radii before TRS and shared mesh/point depth composition.
The bitmap object identity is
stable, with private geometry/pixels/camera for each frame. This independent
keyframe source has no root Scene cursor, general updater or full 3D object API;
its explicit camera/geometry/resource bounds are documented in SCENE3D.md.
MeshKeys additionally interpolate fixed-topology local vertices, authored
normals and UVs before TRS. Normals normalize between keys; topology, attribute
presence and triangle order stay fixed. MaterialKeys independently interpolate
premultiplied base color/alpha with fixed texture/sampler. Invalid intermediate
geometry fails the sample, and copied mesh payload is capped at 64 MiB.

## Images and structured layout

`Bitmap` copies source pixels once into private premultiplied RGBA storage.
Objects, snapshots and copied objects share only this immutable asset. The
ordinary four-edge path holds the affine plane, so existing tree transforms
and matching-asset animation tracks operate on independent geometry. Validation
rejects curved/nonaffine image planes, unsupported image paint and Create/Write.
Ordinary different-asset morphs reject. FadeTransform instead composites two
independent, fitted layers, supporting different bitmap assets and image/vector
transitions, with explicit cleanup/identity semantics in ANIMATION.md.

Raster image rendering inverse-maps destination pixel centers, bilinearly samples
premultiplied sRGB pixels with edge extension, and combines a vector-rasterized
boundary coverage mask with inherited opacity. It clips work to the image ROI.
This is not a mipmapped minification filter. SVG export embeds PNG plus an affine
matrix; no external file or process is needed. Image decoders validate the encoded
byte limit and header dimensions before pixel allocation. GIF uses frame zero.

`internal/grid` validates and copies cells before arranging them. Maximum ink
widths/heights define columns/rows, with explicit minima for empty cells. Matrix
brackets are geometric outlines, independent of TeX font delimiters. Table rules
and background cells are ordinary objects in the same tree. Accessors return
live cell pointers in fresh selection slices. Layout is performed at construction,
not dynamically reflowed when a cell changes. Whole-tree transforms preserve
existing references, but changing tree topology invalidates semantic selections.
Optional copied table row/column labels and corner take part in the same layout;
data access remains zero-based and excludes headers. Explicit Grid access includes
headers and the empty corner placeholder. Table construction caps the full layout
at 65536 cells before text creation or copies. See TABLES.md.

`graph.Scale` is an immutable checked mapping from axis coordinate to data:
positive linear scaling or log-base>1 exponentiation. Scaled number lines and
axes keep construction geometry fixed, accept actual data in Point/Coordinates,
and sample plots uniformly in the X scale's coordinate space. Callbacks and tick
formatters receive actual data. Domain/range errors reject; NaN/Inf plot values
create gaps. Object transforms do not rebuild these coordinate maps. See SCALES.md.

Signed BarChart uses four canonical edges per bar, including zeros, so numeric
targets interpolate through the baseline with stable child IDs. A private affine
frame inside plot bounds follows whole-chart transforms; later checked Point and
value updates follow that frame. Targets are independent copies and SetValues
commits only after all paths validate. Category labels are fixed snapshots below
the plot; range inference and automatic value labels are pending. See BARCHART.md.

ComplexPlane owns the existing Cartesian grid plus a private affine frame.
Checked complex Point/Number operations follow whole-tree transforms; the
dual-basis inverse projects off-plane points and handles nonorthogonal axes.
Independent axis scaling protects orthogonal extreme aspect ratios, while
nearly collinear bases reject. Complex trajectory plots own sampled geometry
and break at nonfinite callback values. See COMPLEX_PLANE.md for precise bounds.

PolarPlane reuses that affine inverse with a reference triangle inside its outer
circle. It draws bounded circle/ray grids and explicitly applies phase/direction
to data mapping; radial plots reject negative radii and break nonfinite gaps.
Coordinates canonicalize phase to [0,2*pi). See POLAR_PLANE.md for differences
from Manim and the finite grid/resource contract.

## Output

PNG and SVG are written through temporary files. GIF keeps one indexed frame
at a time and distributes centisecond timing error to preserve duration. PNG
sequences use a temporary directory. Existing output paths are rejected.
Single-file publication uses same-filesystem hard links for no-replace
semantics; filesystems without hard-link support need a future publication
adapter. Directory publication has the platform's rename semantics.

MP4 stores one video track, a single sample chunk, sample sizes, timing tables
and a final `moov` box. Sample metadata uses memory proportional to frame count;
pixel memory is bounded. The `mdat` uses a 64-bit size. H.264 is Constrained
Baseline with progressive, full-range YUV420 IDR slices and cropping to the
requested even dimensions. The compressed path uses I4x4 prediction, integer
transform/quantization and CAVLC; optional mode search compares legal DC,
vertical and horizontal predictions. I_PCM is retained separately. SPS/PPS live
in `avcC`. Sample-table size is explicitly bounded. An optional IDR/P subset skips exact previous-reconstruction YUV matches and
uses intra coding elsewhere; its SPS stores one reference and stss marks actual
IDRs. Optional zero-motion P_L0_16x16 residuals and bounded local inter/intra
choice preserve the previous default bitstreams. Optional exhaustive radius1..4
integer luma motion search preserves legal motion prediction, reference padding
and fractional chroma interpolation. No fractional luma search, B pictures/reordering
exists; broader prediction is described in MEDIA.md. There is
no fragmentation, fast-start metadata or rate control.

MOV uses QuickTime H.264 and little-endian PCM16 tracks with separate clocks.
Version-1 movie/track headers and an LCM movie timescale preserve audio tails
shorter than one video frame. Audio is one initial chunk; video follows in one
chunk. Scene/Replay sound timelines mix independently using bounded sinc
resampling; WAV, FLAC and subtitle files use the same atomic publication mechanism.
Native AIFF/AIFC readers support bounded uncompressed signed PCM and explicit
metadata handling. Native AU also supports bounded signed PCM and G.711 with explicit known/unknown-size and annotation policy. CLI audio selection inspects WAV/FLAC/AIFF/AU signatures and
requires file EOF after the selected decoder succeeds. Native streaming readers
retain their documented stream boundaries. Explicit mono/stereo remix is separate
from decoding. The compiled 3D demo adapter forwards sound/caption authoring to
an ordinary overlay Scene with the same duration.
The WAV reader also decodes Microsoft IMA ADPCM 4-bit mono/stereo complete blocks.
It validates extension fields, predictor/index headers and channel interleaving,
charges all decoded block capacity, then trims only final-block padding using
mandatory fact frames. Its integer reconstruction follows the specification and
libsndfile; known FFmpeg rounding and padding differences are recorded explicitly.
Captions retain plain UTF-8 and integer millisecond boundaries in immutable
tracks; they are exported separately as SRT/WebVTT, without burn-in or embedding.
See MEDIA.md for exact timing, speaker and resource policies.

## Text, formulas and SVG

Font data is parsed once and shared read-only; each layout call has its own
sfnt buffer. Glyph contours become ordinary objects. Font selection is explicit
and missing glyphs are errors by default. Embedded regular/bold/italic/bold-italic
fonts use sync.OnceValue caches and real outlines. Complex-script shaping is pending.

Math layout currently produces a bounded TeX subset. Every visible component is
an object, including fraction bars and radical outlines. Recursive rows support
scalable/invisible delimiters and nested matrix variants/cases; text groups keep
spaces. Source length, nesting, environment cells and outline complexity are
bounded. Integral/sum/product/lim operators support scoped display/text styles
and explicit limits/nolimits, plus real-font named operators and stacked
annotations. Full MATH-table glyph sizing, TeX spacing/source-token mapping and
TransformMatchingTex are pending.
Common set relations/operators use real Go/STIX outline subsets with explicit
font licensing, rather than synthetic characters or raster fallback.
The mathbb/mathcal/mathfrak commands use 166 real STIX CFF outlines with Unicode
Letterlike exceptions; unsupported alphabet characters reject explicitly.
Explicit dfrac/tfrac and binomial variants use scoped nominal em and actual
operand baseline boxes; legacy frac geometry stays unchanged. Substack creates
at most 64 centered nonempty rows, sharing source/depth/cell/outline budgets.
These have a documented finite style policy, rather than a full TeX style engine.
Unsupported commands are errors.

SVG import parses XML and path numbers in Go. Elliptical arcs follow SVG's
endpoint-to-center conversion and are approximated by cubic arcs. Unsupported
paint servers and filters reject unless explicitly supported below. Local
defs/symbol/use references expand with bounded depth/count and independent IDs.
Embedded CSS uses a bounded selector matcher and explicit specificity/order
cascade; unknown selectors/properties and external references fail explicitly.
Actual text/tspan runs use the same OpenType vector layout, bounded scalar
positioning/whitespace semantics and an optional explicit font resolver. Unsupported
shaping/bidi/position arrays reject. Bounded nonzero userSpaceOnUse clipPath import
creates native Clip ancestry, including single-shape holes, transforms and nested
intersections. Multiple clipping-child unions and objectBoundingBox clipping
remain unsupported; importer scope is documented in svg/README.md.
Linear and concentric radial fill servers support local href templates, CSS
stops/opacity/currentColor, userSpaceOnUse and ordinary-shape objectBoundingBox
coordinates by constructing native affine gradients. Gradient strokes, text bbox
paints, off-center radial focus and repeat/reflect spread reject.

## Explicit quality and orientation extensions

2D Supersampling is an explicit whole-frame quality option: complete fill,
stroke, clip, gradient, bitmap and camera processing runs at the checked working
resolution before premultiplied box resolve to unchanged output dimensions.
Default scale1 retains legacy pixels. Extra buffers and configured-worker budgets
are checked; this improves coverage but does not turn the low-resolution backend
into exact nonzero region integration. See RASTER_QUALITY.md.

geom.Quat uses scalar-first Hamilton components and active right-handed rotations.
Mesh/point RotationKeys snapshot normalized orientations and use shortest-arc
SLERP, independently of translation/scale. They require zero fixed Axis and
Pose.Angle; existing fixed-axis full-turn tracks keep their original arithmetic.
Look-at camera semantics are unchanged. SCENE3D.md records numeric and pixel refs.

NewWithOrientedCamera instead snapshots eye/lens keys and camera-to-world
quaternions. Local +X is right, +Y up and -Z forward; shortest-arc SLERP allows
roll/pole-crossing without look-at Up degeneracy. Projection/Near/Far stay fixed,
mixed modes reject, and legacy look-at pixels remain unchanged.

Math color declarations and textcolor groups apply finite named/HTML opaque sRGB
colors during vector construction, with scoped state restoration for nested
operands/scripts/environments. Unknown models and alpha reject. G.711 WAVE tag6/7
readers use verified 256-code tables; mandatory fact equals complete decoded frames,
without IMA tail trimming. These share the existing bounded streaming/publication
pipeline; references and differences are in the package READMEs.

Bounded aligned/gathered formulas use operand advances, ascent/depth and row
baselines. Aligned alternates right/left column pairs; gathered centers rows.
Ragged/odd-pair/empty rows and optional TeX positioning/spacing parameters reject.
See MULTILINE_MATH.md; this is not complete amsmath layout.

Microsoft ADPCM WAV tag2 adds mono/stereo 4-bit full blocks, file coefficient
pairs, checked predictor/delta arithmetic and mandatory final-block fact trim.
Prediction divides toward zero and saturates samples; short physical blocks and
adaptive arithmetic overflow reject. Independent decoder differences remain in
audio/README.md.

Explicit text fallback copies an ordered immutable font selection, tries actual
glyph coverage before outlines, and kerns only within the same selected Font.
One-font layout preserves previous geometry/metrics; fallback metrics combine
configured font ascent/descent. Shaping, bidi and font discovery are separate
remaining systems. See FONT_FALLBACK.md.

Adaptive field integration is separate from the legacy fixed-step RK4 contract.
Dormand–Prince RK45 stores accepted nonuniform times/points, FSAL derivatives and
explicit termination; numerical-only calls create no Objects. Geometry builds
share evaluation/step/storage budgets across seeds and finish all callbacks
during authoring. Sample/render does not invoke the vector field.
