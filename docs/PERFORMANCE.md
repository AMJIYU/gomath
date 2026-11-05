# Performance record

This is a development measurement of a bounded workload, not whole-library
performance superiority or a claim of identical rendering quality.

## Historical integrated checkpoint — 2026-11-03

After all five agents completed and integration tests/builds stopped, the same
100-circle, 640x360, single-worker workload was rerun in serial batches. The
conditions and timing inclusion/exclusions below are unchanged. Five trials of
300 frames measured:

| Engine | Median seconds / 300 frames | Median frames/s | Trial range, seconds |
| --- | ---: | ---: | ---: |
| GoMath | 0.497678 | 602.8 | 0.489781–0.545562 |
| Manim Cairo | 1.212160 | 247.5 | 1.171596–1.323825 |

The throughput ratio is **2.44x for this static workload**. This is not a claim
of whole-library superiority, identical rendering, or better memory usage.
The unchanged image metrics are RGB MAE 0.165930/255, maximum channel error 11,
PSNR 48.974 dB, and 4.240% of pixels differing by more than 1 in any RGB channel.
Go accumulates approximately 584.4 MB allocations per 300 frames; peak RSS and
Python memory were not measured. Thermal/background effects and batch ordering
remain possible. Earlier 2.32x and 1.84x batches remain below and in raw records;
their variation is not evidence that the new features caused a speedup.

Raw distributions, all Go source/module hashes and metrics are in
[reference-parallel.json](benchmarks/2026-11-03/reference-parallel.json); exact
geometry is in [geometry-parallel.json](benchmarks/2026-11-03/geometry-parallel.json).
Use the complete existing [reference harness](benchmarks/2026-11-03/REFERENCE.md)
with fresh filenames to reproduce. No reference encoder/renderer is called by
the Go runtime.

New hot paths were then measured with `-p 1` (packages run serially),
`CGO_ENABLED=0 -tags noasm`, three 200ms samples, same M2/Go1.27.3/GOMAXPROCS=8.
These are Go-only baselines, not comparisons with Manim:

| Workload | Median time / operation | Allocations / operation |
| --- | ---: | ---: |
| Camera projection, 100 square objects | 38.313 µs | 338; 84,368 B |
| Rasterize 20 pre-sampled circles, 640x360 | 420.927 µs/frame | 298; about 1.13 MB |
| Complex cubic length, tolerance 1e-8 | 22.871 µs | 0 |
| Cached inverse-length query on same cubic | 57.598 µs | 0 |
| 3,968-triangle sphere, 320x180, 1x | 2.408 ms/frame | 9; about 0.85 MB |
| Same sphere, 2x linear supersampling | 4.425 ms/frame | 11; about 3.15 MB |
| Same sphere, 4x linear supersampling | 10.157 ms/frame | 11; about 11.44 MB |

Full samples are in [parallel-hotpaths.txt](benchmarks/2026-11-03/parallel-hotpaths.txt).
Camera timing excludes rendering. Raster timing excludes Scene.Sample and uses
20 ordinary unfilled stroked circles, not the 100 filled-circle comparison.
Path timing excludes snapshot construction for the cached query. Three timing
includes validation, geometry transforms, clipping, fresh color/depth buffers,
per-sample shading/depth and resolve; it excludes mesh construction, Scene
integration/bitmap wrapping, encoding and I/O. Camera/path operations emit zero
frames; renderer benchmark iteration counts are recorded as timed frame counts.
Inverse length is bounded and allocation-free here but still a noticeable cost
for large numbers of moving objects; packed snapshots and inverse lookup caches
remain optimization work, subject to accuracy and isolation tests.

## Earlier checkpoint — static Cairo comparison, 2026-11-03

Apple M2, 8 logical CPUs, macOS 15.7.1 arm64. Go toolchain `go1.27.3`,
CGO_ENABLED=0, noasm, GOMAXPROCS=8, one render worker. Manim Community 0.22.1,
source `16ea443e2a5f1d79db0a721161411ce916d6e733`, Python 3.12.10,
Cairo 1.18.4.

Both render 100 static circles at 640x360 on black, using the **same exported
cubic control points**, fill colors/alpha, stroke widths and camera bounds.
Each engine ran 5 trials of 300 frames in serial batches with no competing
builds/tests. Scene construction, startup, compilation, encoding and I/O are
excluded. Go includes Scene.Sample and its private snapshots; Manim uses
Camera.reset + capture_mobjects on preconstructed VMobjects.

| Engine | Median seconds / 300 frames | Median frames/s | Trial range, seconds |
| --- | ---: | ---: | ---: |
| GoMath | 0.508561 | 589.9 | 0.481482–0.524151 |
| Manim Cairo | 1.180133 | 254.2 | 1.138805–1.190239 |

The final batch throughput ratio is **2.32x for this workload**. An earlier
serial batch measured 1.84x and is retained in the raw data; the between-batch
variation is not evidence of another optimization. Engines were not randomly
interleaved, and thermal/background-system variation is possible. These results
do not establish an advantage for text, formulae, 3D, codecs, animation semantics
or end-to-end production workloads.

The rasterizers use different antialiasing implementations. In the measured
images, RGB MAE is 0.165930/255, maximum channel error is
11, and PSNR is 48.974 dB. 4.240% of pixels differ by
more than 1 in at least one RGB channel. **The images are not pixel-identical.**
Alpha is excluded from this opaque-background comparison. Fixed PNG regression
thresholds are project quality guards, not proof of Manim behavior compatibility.

Go accumulated approximately 583.5 MB of allocations across 300 frames.
This is not peak memory. Python memory was not measured, so no relative memory
advantage is claimed.

The complete trial samples, source hashes, exported geometry and image metrics
are in [reference.json](benchmarks/2026-11-03/reference.json) and
[geometry.json](benchmarks/2026-11-03/geometry.json). The full development-only
Python harness and commands are in [REFERENCE.md](benchmarks/2026-11-03/REFERENCE.md).
The older batch is in [reference-earlier.json](benchmarks/2026-11-03/reference-earlier.json).

## Go baselines and reproduction

```sh
CGO_ENABLED=0 go run -tags noasm ./cmd/gomath benchmark \
  -scene lissajous -frames 300 -width 640 -height 360
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=geom -pkg=path -pkg=render \
  -run '^$' -bench . -benchmem -count 5
```

The CLI warms up one raster frame, performs GC, then measures serial scene
sampling plus rasterization. Sample times are evenly distributed over duration;
Create means early frames may contain fewer paths. The static reference workload
avoids that variability. Results include cumulative allocated bytes/allocation
counts, Go version, platform, dimensions and GOMAXPROCS. No peak-RSS result or
memory comparison is implied.

## Renderer implementation and quality

The initial rasterizer cleared and composited frame-sized masks per object.
The current renderer clips work to geometry plus stroke/antialias padding and
reuses worker-local vector buffers. This helps small objects and glyphs. Fill
and stroke use adaptive flattening with a 0.02 pixel default error bound.
Pixel tests cover fill, stroke holes, join continuity, arrow visibility, affine
images, transparent image edges, and serial/parallel equivalence.

Preliminary ROI before/after files in `artifacts/` overlapped another benchmark;
do not report their ratio as a controlled optimization speedup. Image sampling
currently uses bilinear filtering without mipmaps; vector-reference results do
not establish image-rendering superiority.

## Codec measurements — 2026-11-04

The CLI now defaults to compressed H.264 intra output. On the 12-frame,
320×180 static reference scene at 12 fps and QP24/DC, the emitted MP4 is
142,263 bytes, versus approximately 1.4 MiB for I_PCM. Independent RGB
comparison for that small-circle image measured MAE 2.380098/255, maximum 57,
PSNR 34.646891 dB, including both YUV420 subsampling and quantization.
This is a bitrate/quality observation for one workload, not equivalent quality
or a general compression ratio. Encoder mode-search trades additional CPU
for local prediction choices; detailed per-image/QP results, exact boundaries
and serial benchmarks are in [H.264 validation](../internal/h264/VALIDATION.md).

New numerical, transparent/texture raster and audio baseline measurements
are documented in [LINALG.md](LINALG.md), [three/README.md](../three/README.md)
and [audio/README.md](../audio/README.md). They are Go-only microbenchmarks;
none establishes a whole-library advantage over Manim. The historical raw
commands/hashes above remain unchanged even though tests now live in tests/.

## I_PCM baseline caveat

I_PCM consumes roughly 1.5 bytes per padded pixel per frame before emulation
prevention and container overhead. At 1920x1080/30fps this is about 93 MB/s before
those overheads. This is a correctness baseline, not efficient compression.
Encoding speed alone is insufficient: assess bitrate, fidelity, peak memory,
latency and file size. FFmpeg is used only as an independent development decoder,
never by GoMath to render or encode.

## Relative authoring and sequential sampling — 2026-11-04

Raw output: `tests/data/root/sequence-benchmark-20261104.txt`, Go1.27.3,
darwin/arm64, Apple M2, CGO_ENABLED=0, noasm, GOMAXPROCS=1. Command:

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go run -tags noasm ./tests/cmd/run -pkg=root \
  -run '^$' -bench 'BenchmarkRelativeAnimationConstruction|BenchmarkSharedSequenceSampling' \
  -benchmem -benchtime=300ms -count=5
```

A static 32-circle family is shared by the two constructor workloads. Median
explicit `Transform(o, o.Copy().Shift(delta))` construction was 10.256µs,
35,792B/162allocations, versus 25.46ns,48B/1allocation for deferred Shift.
**These constructors do different amounts of work**: relative Shift defers the
geometry capture to Play. The benchmark excludes preparation, Play, rendering
and output; it establishes no end-to-end speedup or comparison with Manim.

For a four-step family animation, sampling the same interior time took median
11.402µs for four explicit Plays and11.430µs for one Succession. Both used
37,968B/171allocations. This is a new Go feature control, not a sampling
optimization. Authoring and input construction are excluded. Parallel agents
were developing during these measurements; system contention was not fully
isolated. No image dimensions or encoded frames apply to these geometry-only
microbenchmarks.

Subsequently, interpolation was changed to reuse the already private sampled
path buffers instead of cloning them again. Complete pinned root fixtures and
the root race suite passed. Fresh paired five-sample records are
`tests/data/root/sample-reuse-{before,after}-20261104.txt`, with source hashes,
exact command and medians in `sample-reuse-provenance-20261104.json`.
For this same 32-circle/four-step sample, allocation fell from
37,968B/171allocations to23,632B/107allocations. Median explicit-Play time was
12.379→9.289µs and Succession time12.747→9.301µs. Before samples include a
visible20.932µs contention outlier and other agents were active; this is not a
fully isolated CPU comparison. The deterministic allocation reduction and
preserved independent snapshots are the primary result. It is not whole-frame
rendering, whole-library performance or superiority over Manim.
