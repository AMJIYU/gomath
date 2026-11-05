# CPU renderer baseline — 2026-11-03

Command:

```sh
CGO_ENABLED=0 go test -tags noasm ./three -run '^$' \
  -bench BenchmarkRenderSphere -benchmem -count 3 -benchtime 500ms
```

Environment: Go 1.27.3, darwin/arm64, Apple M2 (8 cores, 16 GB RAM), default
GOMAXPROCS=8. The renderer itself is single-threaded. Other development agents
were active on the shared machine; these are baseline observations, not
isolated performance guarantees or a comparison against Manim.

Workload: one radius-1 sphere with 64 longitude segments and 32 latitude
intervals (1,986 vertices, 3,968 indexed triangles, smooth radial normals),
320×180 pixels, perspective camera at (0,0,3) looking at the origin, vertical
FOV pi/2, near=1, far=10. Default ambient/directional light, transparent clear
background, backface culling disabled. The same immutable mesh and renderer
are reused, while every frame receives fresh output and depth buffers.

| Sample | Timed frames | Time/frame | Bytes/frame | Allocations/frame |
| --- | ---: | ---: | ---: | ---: |
| 1 | 174 | 2,903,140 ns | 852,400 | 9 |
| 2 | 230 | 2,387,950 ns | 852,400 | 9 |
| 3 | 249 | 2,876,779 ns | 852,424 | 9 |

Timing includes validation, input preparation, model/view/normal transforms,
six-plane clipping, color/depth buffer allocation and clearing, z-buffer
rasterization, per-pixel normal interpolation and Lambert shading. Primitive
construction and renderer construction are outside the timed loop. Bitmap
wrapping, Scene callbacks/composition, image encoding, disk I/O and video
encoding are excluded. No warm output/depth buffer pooling is used. Go's
benchmark calibration runs are excluded from the timed frame counts above.

First-delivery validation and dependency check (before supersampling):

```text
CGO_ENABLED=0 go test -tags noasm ./three     PASS
CGO_ENABLED=0 go vet -tags noasm ./three      PASS
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
PASS: 35 non-standard packages; CGO_ENABLED=0; noasm;
      no native inputs or runtime process launch imports
```

Full-repository `go test` and `go vet` also passed earlier in this shared
checkout. Their final rerun was blocked by concurrent changes outside `three`:
`mathtex/mathtex.go` referenced parser methods `delimiter`, `enclose`,
`environmentName` and `environment` that were not yet defined. Those files
belong to other tasks and were not modified here. The final `three` package
checks above passed independently.

Package tests include concurrent use of separate renderers and a shared
renderer, with immutable shared mesh data and independent output buffers.
These checks used CGO_ENABLED=0; no race-detector result is claimed.

## Supersampling follow-up — 2026-11-03

Command:

```sh
CGO_ENABLED=0 go test -tags noasm ./three -run '^$' \
  -bench '^BenchmarkRenderSphereSupersampling$' -benchmem -count 3 -benchtime 200ms
```

Environment: Go 1.27.3, darwin/arm64, Apple M2, default GOMAXPROCS=8.
The workload, camera, material, light and 320x180 **output** dimensions match
the first baseline above: 1,986 sphere vertices and 3,968 triangles, culling
disabled. Scale 1 uses 320x180 samples, scale 2 uses 640x360 samples, and scale 4
uses 1280x720 samples. Other agents remained active on the shared machine;
measurements were not isolated, and short-run variability is visible.

| Scale | Sample | Timed frames | Time/frame (ns) | Bytes/frame | Allocations/frame |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1x | 1 | 90 | 2,464,049 | 852,522 | 9 |
| 1x | 2 | 99 | 3,339,992 | 852,402 | 9 |
| 1x | 3 | 99 | 2,423,423 | 852,405 | 9 |
| 2x | 1 | 52 | 4,345,155 | 3,154,416 | 11 |
| 2x | 2 | 56 | 4,385,641 | 3,154,414 | 11 |
| 2x | 3 | 54 | 4,408,600 | 3,154,418 | 11 |
| 4x | 1 | 24 | 10,225,738 | 11,444,746 | 11 |
| 4x | 2 | 18 | 11,560,148 | 11,444,729 | 11 |
| 4x | 3 | 22 | 9,578,489 | 11,444,740 | 11 |

All timings include fresh sample RGBA/depth allocations and clearing, mesh
validation, transforms, clipping, per-sample shading/depth tests, and (at 2x/4x)
the premultiplied area resolution pass and independent output allocation.
Sphere/renderer construction, bitmap wrapping, Scene callbacks/composition,
image/video encoding and I/O remain excluded. There is no sample buffer pool.

Sample counts and raster/depth storage grow with the square of the linear
scale, but mesh validation/transformation/clipping do not. These observations
are not a promise of a particular whole-frame slowdown. Per-frame allocation
is roughly 0.81/3.01/10.91 MiB for this small 1x/2x/4x workload; larger targets
consume much more. Scale 4 at 1920x1080 is explicitly rejected by the internal
pixel cap. The maximum accepted raster-buffer payload is 208 MiB per call,
plus at most 18 MiB transformed-vector payload, bounded instance preparation
and runtime/allocator overhead. Concurrency multiplies per-call memory use.
The limit does not bound process RSS or GC slack.

Coverage is a regular sample-grid estimate, not an exact area integral. Box
resolution averages quantized premultiplied sRGB bytes, so it is not
linear-light/HDR filtering. Subsample details and temporal shimmer remain
possible. At that stage, transparent materials/textures were unsupported;
the transparent-material follow-up is measured below.

Follow-up validation, limited to this package as requested:

```text
CGO_ENABLED=0 go test -tags noasm ./three     PASS
CGO_ENABLED=0 go vet -tags noasm ./three      PASS
```

New checks include old default/explicit-1x equality, exact known coverage and
premultiplication, compositing coverage edges onto white, clear/partial
background preservation, clipped edges against independent subpixel rays,
shared edges and viewport bounds, per-sample opaque occlusion, rejected scales
and layouts, maximum accepted memory layouts without allocating their buffers,
and shared/separate-renderer concurrent determinism with independent images.
No full-repository checks or race-detector run were performed for this update.

## Per-sample transparent layers — 2026-11-04

All current test and benchmark sources live in `tests/_cases/three/` and are
compiled into the original package by the central overlay runner. Command:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=three -run '^$' \
  -bench '^(BenchmarkRenderSphere|BenchmarkRenderTransparentSphere)$' \
  -benchmem -count 3 -benchtime 200ms -p 1
```

Environment: Go 1.27.3, darwin/arm64, Apple M2, GOMAXPROCS=8. `-p 1` serializes
package execution; each frame is rendered on a single goroutine. Other agents
were active on this shared machine. Short, non-isolated measurements do not
establish an opaque throughput regression or improvement versus the earlier
baseline. Opaque allocations remain nine per frame with the original raster
path and no transparent fragment storage.

Workload: the same 1,986-vertex, 3,968-triangle sphere and 320x180 output as
above, default lighting, perspective camera at (0,0,3), no backface culling.
Transparent runs use NRGBA (201,101,53,128), including both front and rear
surface fragments. Scale 1/2/4 uses 320x180 / 640x360 / 1280x720 samples.
The opaque control uses its existing opaque white material at scale 1.

| Material / scale | Sample | Timed frames | ns/frame | Bytes/frame | Allocations/frame |
| --- | ---: | ---: | ---: | ---: | ---: |
| Opaque 1x | 1 | 69 | 3,286,691 | 852,480 | 9 |
| Opaque 1x | 2 | 66 | 3,670,367 | 852,475 | 9 |
| Opaque 1x | 3 | 68 | 3,217,681 | 852,400 | 9 |
| Transparent 1x | 1 | 75 | 3,250,197 | 990,048 | 13 |
| Transparent 1x | 2 | 74 | 3,249,394 | 990,041 | 13 |
| Transparent 1x | 3 | 70 | 3,243,648 | 990,046 | 13 |
| Transparent 2x | 1 | 30 | 7,848,782 | 3,619,926 | 20 |
| Transparent 2x | 2 | 30 | 7,623,493 | 3,619,745 | 20 |
| Transparent 2x | 3 | 30 | 7,670,338 | 3,619,755 | 20 |
| Transparent 4x | 1 | 9 | 26,012,000 | 13,089,704 | 38 |
| Transparent 4x | 2 | 9 | 22,734,398 | 13,089,697 | 38 |
| Transparent 4x | 3 | 9 | 25,447,329 | 13,089,694 | 38 |

Timing includes mesh validation/transforms, clipping, color/depth allocation
and clearing, opaque pass, transparent sample shading and opaque rejection,
block allocation, per-sample depth ordering, float64 premultiplied source-over,
sample quantization, and SSAA resolve where enabled. Geometry/renderer
construction, bitmap wrapping, Scene callbacks, encoding, disk I/O and Go
benchmark calibration are excluded. There is no warm buffer pool.

Fragment records have 16-byte payloads and are stored in blocks of up to 4096
records. The default fragment budget permits at most 16 MiB plus a 6 KiB
directory; the hard maximum permits 64 MiB plus 24 KiB (64-bit Go), separately
from the 208 MiB raster payload cap and geometry scratch. Actual allocation for
this sphere is smaller because blocks allocate on demand after opaque rejection.
Sorting is in place, O(F log F) over F surviving fragments. Transparent closed
meshes shade/store rear layers rather than using opaque early depth writes.
Fragment overflow returns a nil image and an explicit error, including when
SSAA makes a previously accepted scene exceed its budget.

Scoped verification:

```text
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=three -count=3   PASS
CGO_ENABLED=0 go vet -tags noasm ./three                  PASS
CGO_ENABLED=0 go list -deps -tags noasm ./three           PASS
```

Dependency metadata showed no cgo, C/C++, Objective-C or SWIG inputs. Package
production code adds only Go standard-library imports and no runtime process
launch. Twelve new transparency regressions cover independent ray references
for crossing depth and clipped smooth lighting, both projections, SSAA scales
1/2/4, premultiplied output and bitmap composition, instance/triangle order,
coplanar ties, opaque masking before fragment accounting, clipped shared edges,
inverse-transpose/reflection/culling, 512 low-alpha layers, exact and exceeded
budgets, block capacity and cross-block sorting, validation at alpha zero, and
shared/separate-renderer concurrent determinism. The old opaque-only material
alpha rejection test now checks the supported blended result.

No full-repository tests or race-detector run are claimed. The race detector
requires CGO_ENABLED=1 on this toolchain; these checks keep CGO_ENABLED=0.
Results establish this package's geometric/alpha behavior, not Manim parity or
linear-light/physically based transparency. Textures remain unsupported.
