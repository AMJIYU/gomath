# Manim migration ledger

Reference: Manim Community **0.22.1**, commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`.

The source inventory in `compat/manim-api.json` lists 177 Python modules and
2,498 declared public classes/functions/methods. It was extracted from the local
source AST during development; no Python execution is needed by GoMath.
Overloads may appear more than once; constants, imports/re-exports and dynamic
attributes need an additional public-API audit. This is not a completeness
percentage. All per-symbol entries remain **unverified** until a matched
behavioral reference test has passed.

| Capability family | Current status | Implemented subset / remaining acceptance |
| --- | --- | --- |
| Numerical geometry | Partial | Vec3, Mat4, rotations, bounds; immutable dense algebra, partial-pivot LU, column-pivot Householder QR least squares, Cholesky SPD solve, real symmetric Jacobi eigen, direct one-sided economy SVD including wide/rank-deficient inputs, pseudoinverse/minimum-norm solves; bounded general real Schur decomposition and complex right eigenvectors with original-matrix residual checks and numerical basis-rank reporting. Immutable bounded ComplexDense arithmetic/conjugate transpose and checked complex partial-pivot LU, multi-RHS solves, LogDet/Inverse are available. Immutable strict real CSR, transpose/add/sub/matvec and bounded context-aware CG with explicit Jacobi and original-system residual acceptance are available; bounded deterministic CSR SpGEMM uses sparse row heaps and exact product sums with explicit work/storage/cancellation. Bounded right-preconditioned BiCGSTAB accepts nonsymmetric CSR, initial guesses and explicit Jacobi, verifying original-system residual at each half/full candidate; breakdown/convergence/range are distinct. Sparse factorization and SPD proof remain pending. Tensor algebra and broader SciPy-equivalent algorithms remain pending. |
| Bezier/path engine | Partial | Cubic paths, subdivision, bounds, flattening, contour alignment; bounded adaptive arc length, inverse length, tangents, resampling, trim/prefix by length and immutable measurement snapshots, bounded accepted-quadrature lookup for repeated inverse queries, exact parameter-interval trimming. Closed XY polygon union/intersection/difference/xor with independent nonzero/evenodd rules, exact rational intersections and checked float64 rounding; holes, self-intersections and collinear overlap supported within budgets. Checked explicit curve-count expansion and jagged/smooth anchor handles use global equal-parameter subdivision and linear open/cyclic solvers;24 anchor cases,18 insertion cases and20 actual equal-topology Transform states pass (CURVE_EDIT.md). Exact joins, explicit smoothing seams and bounded work/storage apply. Checked independent contour/path reversal and atomic receiver-only ReverseDirection/family ReversePoints preserve true endpoints, IDs and children;54 pinned Transform states pass (PATH_DIRECTION.md). Implicit closing gaps reject; force_direction remains pending. Curved Boolean and general shape-semantic matching remain pending. |
| Mobject/VMobject/Group | Partial | IDs, deep copy, tree transforms, style, layout, snapshots. Checked ApplyPointwise/ApplyComplex performs atomic path-only authoring with contracted/restored cubic handles, retained child pointers and bounded callbacks;24 pinned constructions/168 ordered interior Transform states pass (POINTWISE.md). Atomic family MakeSmooth/MakeJagged and PrepareForNonlinearTransform preserve identity, with12 constructions/84 ordered pinned Transform states (OBJECT_CURVE_EDIT.md). Circle floating-point closing gaps reject handle conversion. No adaptive nonlinear subdivision or image/clip/gradient deformation; not all Manim object semantics or dynamic attributes. |
| Basic geometry | Partial | Circle, arc, rectangle, polygon, line, arrow, dot, regular polygon, Bezier, ellipse, annulus/annular sector/disk sector, rounded rectangle, fixed-period dashed lines, double arrow and checked XY angle/right-angle marks (quadrants, signed sweep, automatic radius and dot; 448 pinned Transform states). Checked ArcBetweenPoints signed sweep/radius/explicit plane and XY TangentialArc fillets add343 pinned Transform states; actual trimmed-shaft CurvedArrow/DoubleArrow have explicit tip semantics that differ from Manim defaults (ARC_CONNECTIONS.md). Checked SurroundingRectangle/BackgroundRectangle/Underline/Cross use complete geometric XY bounds and bounded source trees;336 pinned Transform control-multiset states on straight-path inputs, with explicit background lifecycle differences; standalone origin-rotated Elbow adds105 ordered control states (ANNOTATIONS.md). Checked planar Brace/BraceBetweenPoints reuse the MIT source outline, project full target cubic bounds and provide tip/anchor-direction accessors plus independent label copies;224 pinned ordered Transform states and12 label placements pass (BRACES.md). Explicit-circle NewArcBrace adds the source exponential/handle policy, radius floor and radius-dependent scaling;36 constructions/252 ordered pinned Transform states pass (ARC_BRACES.md). Complete live brace-label semantics and edited-arc parameter recovery remain pending. Bounded polygon Boolean available through path.Boolean; curved booleans and many specialized primitives remain pending. |
| Animation core | Partial | Transform/TransformFromCopy (44 pinned equal-topology reversed-rate states), true root/nested ReplacementTransform, explicit default-key-bucket TransformMatchingParts with transparent repeats, unmatched fades, source restoration and actual target replacement (MATCHING_PARTS.md), nested FadeOut removal, independent-layer FadeTransform (33 pinned geometry/opacity states, distinct-asset/image-vector crossfade), Shift/MoveTo/Scale/Rotate, Create/Write subset, windows/rates/lag, inverse-arc-length MoveAlongPath. Source-pinned sigmoid Smooth and standard ease families, GrowFromPoint/Center/Edge/Arrow, SpinIn, Indicate, WiggleObject, ApplyWave, PassingFlash and Uncreate/Unwrite. FadeOut cleanup restores original style for reuse. Checked WithPathArc/WithPathArcAxis plus clockwise/counterclockwise defaults add actual per-control circle trajectories, copied/reversed geometry, group/window/Succession and planar clip/gradient-translation contracts;105 pinned states and12 Cairo interiors pass (TRANSFORM_PATHS.md). Source oblique-axis endpoint projection is preserved explicitly; full-turn singularities/custom paths reject. Implicit TeX/shape matching, key aliases and many specialized animations remain pending; inverse-length motion timing differs. Cubic easing remains explicitly available as SmoothStep. |
| Composition | Partial | Nested AnimationGroup/LaggedGroup, unequal child durations, independently warped clocks and recursive finish verified against 44 pinned states. Shared-object/nested Succession, deferred relative operations, atomic whole-sequence staging and per-step cleanup/replacement; 51 pinned control-point states for relative motion. Parallel steps retain disjoint targets. Sequence must be sole Play argument/direct sequence child with linear full-window clock; warped/parallel-contained sequences and complete Manim cleanup parity remain pending. |
| Updaters/ValueTracker | Partial | Absolute-time callbacks, animatable scalar trackers, fixed-topology redraw, bounded dynamic display topology with deterministic preorder child-ID slots, fixed-step DT integration with checkpoints/concurrent replay. DT subtrees must exist unchanged throughout authoring and cannot overlap tracks. Managed object callbacks/redraw add timeline registration/suspend/resume/permanent removal, shared callback order, fixed sampled topology and source-family suspension during complete Play segments;91 pinned ordered unit states pass (OBJECT_UPDATERS.md). General Manim dt/parent-child updater ordering, inherited runtime lifecycle and complete always_redraw semantics remain pending. |
| Coordinate systems | Partial | Axes, labeled/oriented NumberLine, NumberPlane, parametric/uniform/adaptive plots; signed Riemann sums and piecewise-linear area. Explicit linear/log Scale and checked scaled axes pass 90 mappings/inverses and60 tick centers. Signed fixed-count bar targets pass fourteen pinned update states. ComplexPlane and PolarPlane add affine forward/projected inverse maps and bounded complex/radial plots; pinned references cover explicit rigid poses. Unsampled discontinuities, complete defaults, minor ticks, automatic ranges/value labels, full polar labels/subdivisions and other probability plots remain pending. |
| Text | Partial | Kerned horizontal OpenType glyphs, multiline, cached default regular/bold/italic/bold-italic fonts. Explicit ordered fallback (at most16 fonts), actual glyph coverage, same-font kerning and shared outline budgets are available. Bounded NewMarkup/LayoutMarkup adds nested b/i with real BoldItalic, scoped six-digit foreground colors, strict XML escapes/newlines, same-font kerning across color boundaries and shared glyph budgets (TEXT_MARKUP.md). A separate number package supplies checked DecimalNumber/exact-int64 Integer with bounded fixed precision, sign/grouping/literal units, baseline anchors, atomic identity-preserving authoring and immutable concurrent snapshot factories (NUMBERS.md). Independent references cover1376 binary64 formats,48 int64 formats and20 font/Cairo layouts; four pinned negative-zero policy differences are explicit. Shaping, bidi, full Pango markup, rich code layout, system discovery and color fonts remain pending. |
| MathTex/Tex/Typst | Partial | Fractions/radicals/scripts, real font styles, preserved text spaces, scalable/invisible delimiters, nested matrix variants/cases, integral/sum/product glyphs with side scripts, overline/underline and bounded hat/widehat/bar/vec/dot/ddot decorations, bounded operator limits/display styles, named operators and overset/underset/stackrel, explicit dfrac/tfrac/binom/dbinom/tbinom style sizing, bounded substack rows and aligned/gathered right/left pairs or centered rows, plus real mathbb/mathcal/mathfrak outline alphabets (166 glyphs). Real Go/STIX set-notation outlines cover in/notin, subsets, cup/cap and related symbols. Indexed sqrt, vector boxed and phantom/hphantom/vphantom add explicit bounded layout policies and hidden-operand charging (RADICAL_BOXES.md);20 independent font/Cairo cases and old exact geometry/pixel captures pass. Bounded array adds l/c/r advance alignment, single vertical rules and boundary hline with shared nested-cell budgets; seven independent static font/Cairo references plus two interior affine frames pass (MATH_ARRAYS.md). Explicit separately valid Parts share baseline/advance, parser resource budgets, exact-key live selections and immutable snapshots;9 independent font/Cairo expressions and4 affine animation frames pass (MATH_PARTS.md). A separate exact-key matching animation now groups explicit fragment keys, handles repeated counts/default mismatches and installs the actual target root;63 source-pinned polygon states and formula live-handle checks pass (MATCHING_PARTS.md). Cross-fragment kerning/syntax, implicit source isolation, key aliases and automatic shape matching remain pending. Original bounded vector overbrace/underbrace support centered upper/lower labels, nested operands and shared hidden-ink budgets;8 independent static and2 affine font/Cairo frames pass (MATH_BRACES.md). This finite em policy is not TeX extensible-glyph layout. Full TeX/Typst, MATH-table large glyphs, macros/packages, semantic source maps remain pending. Scoped color/textcolor supports nineteen named colors and opaque HTML six-digit sRGB; broader xcolor models/mixing reject. |
| SVG | Partial | Basic shapes, all path commands/arcs, nested transforms/styles, caps/joins/miter, immutable real dash arrays and explicit budgets. Local defs/symbol/use references and bounded embedded CSS cascade; base64 PNG/JPEG/GIF-first-frame image embedding, meet/none and affine geometry. Actual text/tspan font outlines with bounded scalar positioning, whitespace/advance anchor and explicit font resolver. userSpaceOnUse single-shape/use nonzero/evenodd clipPath, holes, transforms and nested intersections reach text/vector/bitmap raster and export. Ordinary nonzero/evenodd fills preserve inheritance/CSS/use and export. Bounded objectBoundingBox clips on basic/path/image/direct-use targets use local pre-transform geometry excluding stroke, image viewport before meet, and ignore zero-span bbox effects; bounded g containers, text/tspan full glyph cells and supported use chains now have target-local scopes measuring actual transformed geometry;8 static scenes,10 exact bboxes and5 interior animation states have independent Cairo/font references (SVG_CONTAINER_BBOX.md). Symbol/nested viewport branches and percentage/unit clip lengths still reject; old basic/direct-use geometry and budgets remain (SVG_BBOX_CLIPS.md). Multi-child clip union supports2–16 sharp rect/polygon/closed linear path/direct-use silhouettes with independent rules/transforms, materialized by bounded exact polygon Boolean into one nonzero Clip (SVG_CLIP_UNIONS.md). Curved/open/multi-nested clipping, external references, unsupported CSS, gradient strokes, off-center focus/spread variants and filters remain pending. Linear/concentric-radial fill import supports userSpaceOnUse, ordinary-shape objectBoundingBox, CSS stops/opacity/currentColor and local href templates; whole-text objectBoundingBox paints use full advance/ascent/descent glyph cells, including tspan overrides and supported use instances;11 static references and5 animation times at SS1/4 pass (SVG_TEXT_GRADIENTS.md). Per-span/per-glyph independent bbox paints and clone-local definitions remain unsupported. |
| Raster images | Partial | PNG/JPEG/GIF-first-frame decode, immutable premultiplied assets, affine bilinear sampling, AA boundaries, opacity, shared snapshots and SVG embedding. Immutable rasterimage.Asset adds actual byte inversion/RGB replacement/explicit tint and saved-import-alpha opacity restoration, retaining straight colors privately and returning new assets;65536-pixel operation chains and9 NumPy/Pillow animation frames pass (IMAGE_PIXELS.md). Hidden alpha0 source RGB is deliberately discarded on import. Missing masks, image sequences, perspective/mesh warping, general different-asset pixel morphs and sampling variants (explicit two-layer FadeTransform is available). |
| Tables/matrices/code | Partial | Adaptive formula/integer/decimal/object matrices, six bracket modes, selections/colors; text/object tables, inner/outer rules, backgrounds/highlights, owned row/column labels and optional corner. Stable zero-based data access plus explicit full-grid access and live label selections; 15 pinned static label geometries. Separate linalg numerical API. Bounded code.New supplies Go-only go/scanner highlighting, real Go Mono glyph outlines, physical tab/CRLF grid, line numbers/background, live line/token selections and independent immutable copies; six independent font/token fixtures and ten animation frames pass (CODE.md). Baseline/corner alignment modes, spans, automatic reflow, other code languages, semantic highlighting, full Pygments/Pango styles and decorations remain pending. |
| Graph/network layout | Partial | Graph/DiGraph, circular/grid/seeded spring layouts, borrowed vertex/edge refs, explicit edge/layout updates preserving identity. Separate checked CurvedDiGraph adds explicit signed curves, reciprocal separation, side/radius/direction self-loops and atomic identity-preserving updates against current node bboxes (CURVED_NETWORKS.md). Straight Graph/DiGraph now provide atomic AddVertices/AddEdges/RemoveVertices/RemoveEdges with explicit positions and GraphLimits, incident-edge cascading and survivor pointer/ID/style preservation; ten pinned topology states verify twenty edge endpoints (NETWORK_MUTATIONS.md). Legacy straight geometry defaults remain unchanged. Multiedges, automatic per-frame node routing, obstacle routing, full label and Manim graph parity remain pending. |
| Vector fields/streamlines | Partial | Finite XY field sampling, length/color functions, bounded RK4 streamlines and termination reasons. Separate bounded adaptive Dormand–Prince RK45 produces nonuniform accepted times, explicit termination and shared step/evaluation/storage budgets; callbacks never run outside Domain. Immutable Flow provides physical-time trace windows, actual nonuniform timestamps, signed speed, delays and explicit loop/drain clocks with bounded arbitrary/concurrent samples; sampling integrates nothing. Limited to continuous open solid strokes; Manim timing/random phases and full animated traces, arbitrary dimensions and event localization/dense output remain pending (STREAMLINE_FLOW.md). |
| Point clouds | Partial | Immutable copied position/color/pixel-radius buffers; actual CPU circular point disks share opaque/transparent depth, clipping, SSAA and resource budgets with meshes. scene3d adds copied fixed-count point buffers with position/premultiplied-color/pixel-radius keys, real TRS/visibility and concurrent samples. World-space splats, changing point count, broader PMobject operations and GPU paths remain pending. |
| 3D geometry/surfaces | Partial | Validated indexed meshes; box/sphere/cylinder/cone/torus/parametric surfaces, flat-normal Tetrahedron/Octahedron/Icosahedron/Dodecahedron with checked edge lengths and face UVs, bounded closed convex Polyhedron rejecting invalid geometry/topology/winding (POLYHEDRA.md), arbitrary-endpoint closed CylinderBetween/ArrowBetween meshes with deterministic roll, hard seams, physical dimensions and no internal caps (SEGMENT3D.md), two material-independent checkerboard parametric mesh partitions with once-per-node sampling, full-grid area-weighted shared normals and global UVs;108 independent ray frames,three analytic plane frames and14 scene3d frames pass (CHECKERBOARD_SURFACES.md). Periodic seam welding, collapsed poles and full Surface semantics remain pending. Normals/model transforms, six-plane clipping, perspective-correct depth/normals, z-buffer, directional Lambert/ambient shading, optional world-space point light or copied list of at most16 colored sources with distance attenuation and bounded per-frame source evaluations, opaque directional hard shadow maps, per-sample depth-sorted transparency, perspective-correct immutable UV textures with nearest/bilinear clamp/repeat, bounded 1/2/4x supersampling. scene3d adds fixed-topology vertex/normal/UV keys and independent premultiplied base color/alpha keys, with copied buffers and intermediate geometry validation. Composed into Scene as RGBA bitmap; point-light shadows, translucent/point-cloud shadows, mipmaps and full ThreeDScene semantics pending. |
| Cameras | Partial | Timeline-based orthographic 2D pan/zoom/rotation in raster/SVG; look-at perspective/orthographic camera in three. Immutable scene3d mesh TRS/camera/visibility keys and direct FrameSource output, bounded arbitrary/concurrent samples; explicit 2D overlays can share its returned frames. Explicit oriented-camera eye/lens/quaternion keys use shortest-arc SLERP and fixed projection/near/far. Multi-camera, projection switches, integrated 3D updater/object semantics and full ThreeDScene parity pending. Explicit quaternion RotationKeys are available for mesh/point local orientations with shortest-arc SLERP; look-at camera keys remain separate. |
| CPU vector rendering | Partial | Anti-aliased fill/stroke, real dashes/caps/joins/miter limits, alpha, ZIndex, ROI, bounded workers; optional whole-frame 1/2/4/8 supersampling with premultiplied RGBA box resolve and explicit memory/worker limits; immutable world-space nonzero/evenodd clips and ancestor intersections on vector/bitmap leaves, affine/camera transforms and SVG export, explicit mask/flattening/alignment budgets. Native immutable linear/concentric-radial vector gradient fills with alpha, hard stops, full affine frames, compatible animation and SVG paint servers; pad/sRGB subset only. Ordinary solid/gradient/revealed fills also accept evenodd, sharing the bounded flattened-polygon parity integrator and frame budgets with clips. Worker-local reusable clip RGBA/two-Alpha scratch keeps old pixels and output ownership; paired workloads reduce allocated bytes56.97%/60.00%, with mixed timing and retained high-water pixel payload up to96MiB/worker (RASTER_PERFORMANCE.md). soft masks, isolated group opacity, other gradient spread/focus/stroke modes, blend variants and remaining stroke properties pending. |
| GPU rendering/live preview | Not started | Requires a design that satisfies the pure-Go/native-binding constraint. No hidden OpenGL/Cairo fallback. |
| File output | Partial | PNG, SVG with nested clip definitions, GIF, PNG sequences, H.264 I4x4 transform/quantization/CAVLC MP4, optional IDR/P exact reconstructed-YUV skip and correct stss, plus legacy I_PCM; H.264 + PCM16 MOV, WAV/PCM16-24 FLAC, SRT/WebVTT. Optional P_L0_16x16 inter residuals and exhaustive radius1..4 integer luma motion search with fractional chroma interpolation; fractional luma/subpartitions/multiple references, additional compressed-audio decoding, alpha video profiles, sections and encoder variants pending. |
| Audio/subtitles | Partial | Immutable PCM, strict WAV PCM16/24/32 and float32/64 I/O plus native Microsoft IMA ADPCM tag0x11, 4-bit mono/stereo complete-block decoding and mandatory fact tail trimming, native Microsoft ADPCM tag2 4-bit mono/stereo complete blocks with file coefficients and mandatory final-block fact trim, plus tag6/7 G.711 A-law/mu-law 8-bit indexed-channel decode with exact mandatory fact count, gain/envelope/mixing and bounded sinc resampling; explicit channel matrix Remix, format-identical Concat/Repeat, CLI opt-in mono/stereo remix. FLAC fixed-predictor/Rice encoding and strict native PCM8/16/24/32 decoding for1..8 indexed channels (fixed/LPC, Rice/Rice2, stereo decorrelation, CRC/MD5, checked33-bit sides and restored predictor ranges; FLAC_INPUT.md). Equivalent table CRC16 reduces measured old stereo16/24 decoder medians44.98%/69.58% with unchanged checks/allocations, limited to the documented paired shared-host workloads (FLAC_PERFORMANCE.md); bounded AIFF/AIFC NONE signed PCM8/16/24/32 and sowt PCM16 decode with explicit metadata/chunk/channel-order policy; native big-endian AU signed PCM8/16/24/32 and G.711 with explicit annotation and known/opt-in unknown-size policy (CLI known-size only); Scene/Replay sound timeline, PCM16 MOV with exact independent clocks; strict plain-text SRT/WebVTT, immutable integer-ms tracks and Scene captions. Broader compressed-audio decoding, inferred speaker routing, subtitle styling/embedding/burn-in and complete source timing parity pending. |
| Cache/config/CLI | Partial | Demo/benchmark/parity CLI, validation, transactional outputs. Missing full configuration, persistent cache, watch/reload and sections. |
| Notebook/plugins/tools | Not started | Go-native extension and notebook workflows; arbitrary Python execution is outside the pure-Go runtime contract. |
| Cross-platform | Partial | Darwin/arm64 tests; pure-Go Linux/amd64 and Windows/amd64 cross-builds. Native Windows/Linux execution and visual validation remain required. |
| Performance superiority | Limited measurement | Source-pinned static 100-circle Cairo comparison recorded in PERFORMANCE.md, including pixel error. No whole-library or all-workload advantage established. |

## Order of work

1. Expand reference tests for topology alignment, nested replacement,
   shared-target succession and general updater scheduling; optimize immutable sampling allocations.
2. Complete portable 2D/image/SVG semantics: clipping/gradients/booleans and remaining stroke properties,
   source mapping and specialized mathematical objects.
3. Broaden numerical methods, graph/streamline animation, matrix/table semantics,
   and semantic matching transforms without weakening ownership or error checks.
4. Extend 3D primitives/shadows and scene/camera integration, then
   design pure-Go preview within the runtime constraints.
5. Complete text shaping, TeX/Typst compatibility, motion-compensated video and compressed-audio decoding.
6. Run source-pinned reference suites, including difficult mid-animation frames,
   memory/throughput comparisons, cancellation and long scenes.

Items can advance in parallel only when ownership/interfaces are defined.
Foundational correctness fixes take precedence over adding API names.

Complex-plane extension: checked complex128 forward/inverse mapping follows
whole-object affine transforms, with a dual-basis inverse for shear and orthogonal
off-plane projection. Uniform complex trajectory plots break nonfinite gaps.
Seventy mappings and seventy inverse projections match fourteen pinned rigid
Manim poses; defaults/fonts/nonorthogonal inverse and complete ComplexPlane
semantics remain unverified. See COMPLEX_PLANE.md.

Polar-plane extension: bounded circles/rays, explicit radian offset/CW mapping,
affine forward/projected inverse and radial plots with nonfinite gaps. 336 pinned
rigid-pose mappings/radius inverses and252 nonzero phases pass. Angle normalization,
negative-radius policy, offset/CW and outer-boundary conventions differ explicitly
from Manim. Labels, faded subdivisions and complete PolarPlane semantics remain
pending. See POLAR_PLANE.md.

## Current reference evidence

The source-pinned Cairo PNG in `tests/data/render` exercises static rendering of
exact exported cubic geometry. The Go test enforces MAE <= 0.25/255, maximum
channel difference <= 16, and at most 6% of pixels differing by more than 1 in
any RGB channel. These are project regression thresholds, not Manim's acceptance
thresholds. The measured image is not pixel-identical.

`tests/data/root/manim-animation-0.22.1.json` adds 35 independent numerical/identity
samples from the same pinned source: seven times for each of linear translation,
full-turn rotation, equal-topology root replacement, scalar tracker and moving
camera pan/zoom. Five samples per behavior lie inside the animation. The Go test
compares control points/scalars at absolute tolerance 1e-9 and checks replacement
identity at cleanup. See `tests/data/root/README.md` and `tests/reference/animation.py`.
Additional pinned fixtures verify 728 rate samples + 55 eased transform states,
44 nested group states, and 88 growing/indication path states (including Manim's
VMobject pre-function handle scaling for ApplyWave). See the fixture README for
exact tolerances and boundaries. SVG refs/CSS have independent CairoSVG raster
fixtures; audio/video codecs have independent development decoder records.
None establishes arbitrary topology, nested replacement, full rendering parity
or entire methods. All 2,498 per-symbol statuses stay unverified.

The passing-flash fixture adds 27 path-endpoint states and a FadeOut cleanup
opacity check; the primitive raster checks five static filled shapes (MAE
0.057326/255, maximum33). These do not validate complete partial-path geometry
or all shape defaults. Integrated predictive MOV/WAV/FLAC/SRT/WebVTT decoding
records are produced by tests/reference/media.py; development decoders never
participate in runtime.

`tests/data/table/manim-labels-0.22.1.json` verifies 15 static rectangular-ink
table configurations with row/column labels, empty/explicit corner, spacing,
two rigid transforms, visible entry bounds and rule endpoints at tolerance
1e-11. Data indices intentionally remain zero-based and exclude headers.
It does not verify font metrics, all defaults, animation timing or whole Table
methods. See TABLES.md.

`tests/data/graph/manim-scales-0.22.1.json` adds ten scalar/NumberLine
configurations with90 forward/inverse/point samples and60 tick centers. Positive
linear and log base>1 scales use exponent-coordinate ranges; actual data is passed
to plot callbacks. No log Axes default-placement or glyph parity is claimed.
See SCALES.md for domains, precision and plotting limits.

CLIPPING.md records independent CairoSVG checks for native nested vector/bitmap
clips, camera transforms and SVG output. SVG importer clip references additionally
cover five bounded configurations. Three's world point light is checked against
144 independent ray-derived samples; directional shadows retain their separate
bounded references. These are renderer-specific evidence, not complete Manim
ThreeDScene or arbitrary SVG compatibility. General real Schur uses SciPy
references and original-matrix residual checks, not an assertion that all NumPy
or SciPy functionality has migrated.

GRADIENTS.md records independent hand-authored and exported-SVG CairoSVG
references for native vector fields, alpha, affine transforms and clipping.
SCENE3D.md records84 independent ray-derived cases for mesh/camera timelines.
The integrated 40-frame/five-second MP4 decoder record in
`artifacts/scene3d-media-20261104/provenance.json` additionally checks clock,
IDR indices, visible motion and seven decoded-vs-native RGB frames; it shares
Go scene construction and is not an independent geometric oracle.

`tests/data/root/manim-succession-0.22.1.json` adds 51 complete rectangle
control-point states for shared relative operations, deferred rotation centers,
unequal runtime weights and nested linear sequences. It uses Manim ApplyMethod
and Rotating, not precomputed animation-builder targets. GoMath's per-step scene
cleanup policy differs from some Manim outer-group cleanup. Unsupported warped
sequence clocks and sequence/parallel nesting reject; no full Succession claim.

BOOLEAN.md records 48 independent Skia pathops polygon operations, area and point-membership checks, and four CairoSVG raster references. Exact arithmetic, output rounding rejection and limits have separate Go cases. These are closed polygon checks; no curved Boolean or animation-wide Manim compatibility is claimed.

SPARSE.md records independent SciPy systems and the retained 31-unknown Poisson integration regression; every accepted solve checks the original matrix residual. Mesh geometry/material keys add 114 independent pixel cases. Explicit fractions/binomials/substack have 48 independent font operand masks; no independent whole-formula TeX layout oracle is claimed. IMA references compare 37 streams against the specification and libsndfile; FFmpeg low-bit rounding and padded-output differences remain documented in audio/README.md.

The second checkpoint adds 1300 SciPy quaternion numerical samples and420 independent mesh/point pixel cases; sparse products use independent SciPy/Fraction cases. G.711 full-stream outputs match specification/FFmpeg/libsndfile for42 accepted streams/207898 samples. Formula colors use independent manually specified SVG/Cairo references. Optional SSAA4 makes the retained fractions .35 export/raster comparison pass unchanged thresholds (max18); native-scale signed-winding-area cancellation remains documented, rather than being labelled fixed.

The third checkpoint adds explicit oriented-camera tracks (126 SciPy numerical,420 mixed pixel,54 clipping and105 lighting/shadow cases),11 SciPy BiCGSTAB systems,16 multiline operand layouts (minimum per-color IoU .990984), and50 MS ADPCM streams/312468 returned samples. Evenodd clips have direct Cairo references and rational polygon-per-pixel checks; this machine's CairoSVG2.7.1 lacks evenodd clipping and is explicitly excluded as its oracle. Default nonzero output remains unchanged.

ANGLE_MARKS.md records64 constructions/448 source-pinned equal-topology control-point
states including animation interiors. The fourth-wave fallback, adaptive fields,
AU, multi-light and ordinary evenodd modules have their own narrower references;
their final integration checkpoint is recorded in STATUS.md only after all checks
pass. Neither these counts nor API names imply full module parity.

Fourth-wave integrated acceptance passed CGO0/noasm, full race, vet, purecheck42
and Linux/Windows amd64 builds; the frozen checkpoint is
`tests/data/integration/wave4-checkpoint-20261104.json`. All families remain partial
or not started. Four actual-SVG comparisons and a40-frame strict video decode
share Go construction and do not expand the independent geometry/API claims.

Sixth-wave integrated acceptance passed CGO0/noasm, full race, vet, purecheck42
and Linux/Windows amd64 whole-repository builds. The frozen checkpoint is
`tests/data/integration/wave6-checkpoint-20261105/checkpoint.json`; its four SVG
comparisons and40-frame strict video decode match the same source/binary hashes.
New independent geometry/font/codec evidence is scoped in the respective docs.
All families remain partial or not started; no full parity or global speed claim.

Seventh frozen acceptance: CGO0/noasm central suite10.274s, complete race73.720s,
vet, purecheck43 and Linux/Windows amd64 whole-repository cross-builds passed.
See `tests/data/integration/wave7-checkpoint-20261105/checkpoint.json` for frozen
production/binary hashes, logs and five matching SVG/media integrations. Tests
remain centralized under tests/ and the pinned Python repository is clean.
Checkerboard video passed at explicit640×360/QP18; original320×180/QP24 andQP18
quality failures are preserved and not relabeled as passing. This shared-host
checkpoint covers the documented subsets, not complete Manim or later changes.

Eighth frozen acceptance passed CGO0/noasm central suite4.714s, full race50.954s,
vet, purecheck45 and Linux/Windows amd64 whole-repository builds. Checkpoint:
`tests/data/integration/wave8-checkpoint-20261105/checkpoint.json`. Six attached
SVG integrations share its exact production/binary hashes: five SSAA4 scenes
and explicit SSAA8 for svg-container-bbox. That scene's original SSAA4 maximum71
failure is retained; the64 threshold is unchanged. These scoped checks do not
establish complete Manim parity, default-scale pixel identity or global speed.

Ninth frozen acceptance passed central CGO0/noasm11.817s, complete race77.977s,
vet, purecheck45 and Linux/Windows amd64 whole-repository builds; four matching
SSAA4 SVG integrations use identical source/binary hashes. See
`tests/data/integration/wave9-checkpoint-20261105/checkpoint.json`.

Tenth frozen acceptance passed central CGO0/noasm10.891s, complete race75.427s,
vet, purecheck45 and Linux/Windows amd64 whole-repository builds. Three SSAA4 SVG
integrations share its exact production/binary hashes. The authoritative times
and full logs are in `tests/data/integration/wave10-checkpoint-20261105/checkpoint.json`;
all families remain partial/not started. Shared-host acceptance durations and
matching allocation observations are not global or Manim speed comparisons.
