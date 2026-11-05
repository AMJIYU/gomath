# Raster clipping scratch: wave-seven performance evidence

Date: 2026-11-05. This is a fresh scoped investigation, not validation inherited
from the sixth checkpoint. No public API, default, pixel tolerance, geometry,
AA, clip rule, gradient sampling, image filter or compositing operation changes.

## Selected hotspot and implementation

Two pre-authored 640×360 scenes contain vector text (`Go: clip & AA`, embedded
regular Go font), an alpha linear gradient with a compound hole, a clipped
rotated bitmap, nested nonzero clips and dashed cap/join strokes. The ordinary
scene has one panel; complex has six separately transformed panels. Scene/text
construction is outside measurement. Each timed operation renders a complete
fresh output image on one reusable worker. The existing SSAA mixed benchmark
adds scale 1/2/4 workloads; the existing clipped raster benchmark includes an
unclipped vector control and vector/bitmap clips. Ordinary and complex input
construction lives in `tests/_cases/render/raster_perf_test.go`.

The before allocation profile attributed 64.14% of cumulative allocated bytes
to `paintClipped` including its paint callees. `image.NewRGBA` and
`image.NewAlpha` together accounted for 93.44% of allocation across both scenes,
including the required output canvas. CPU includes gradient/color evaluation,
composition and substantial scheduler/VM noise; this change addresses repeated
allocation, not those numerical algorithms. CPU and exact-rate allocation
profiles were captured separately; the initial combined profiler run remains
as `profile-before.txt` and is not a timing sample.

`paintClipped` now borrows one worker-local allocation split into one tight RGBA
layer and two tight alpha masks. Every acquisition rebuilds rectangles, origins
and strides, clears the active layer, initializes the accumulated mask to 255,
and clears the part mask for each ancestor as before. Existing pixel/work/point
checks run before acquisition. `paint` does not recursively invoke
`paintClipped`; composition finishes before another drawable borrows storage.
No cache keys, object IDs, paths, paints or masks survive as semantic content.
The SSAA working copy returns the scratch owner to its base worker, including
on failure. Returned output images remain separate allocations.

**Tradeoff:** the worker retains its largest clip buffer until the Rasterizer
is collected, including through empty frames and errors. Retained pixel payload
is at most `6 * MaxClipRasterPixels` = 96 MiB per worker, plus a small image/header
owner. At SSAA it also fits the existing six-byte-per-working-pixel clipping
component of the worker/aggregate estimates. This changes scratch lifetime,
not the configured limits. Growth allocates exactly the required payload rather
than geometric spare capacity; stable or smaller regions reuse it. This is not
a whole-process RSS bound: output images, vector/gradient/bitmap scratch and Go
GC/allocator overhead still exist, and old/new backing arrays can coexist until
GC after growth. A caller retaining idle workers retains their high-water mark.

## Matched measurement

Apple M2, macOS 15.7.1, Go 1.27.3 darwin/arm64, CGO_ENABLED=0,
`-tags noasm`, GOMAXPROCS=1, `-test.cpu=1`. The tag is the repository's noasm
configuration; it does not disable assembly in the Go runtime/standard library.
Five alternating A→B/B→A rounds, one second calibration per workload, nine
workloads per binary. Both binaries use the same frozen local dependency
sources and centralized tests. Compiler time, process startup, scene/text
construction, one worker warm-up in mixed scenes and external references are
excluded. Per-frame allocations, clearing, compositing and loop GC remain
included. Existing `BenchmarkClippedRaster` has no warm-up and therefore
amortizes initial scratch allocations in its numbers.

| Workload | Median ms/frame before → after | Time change | Bytes/frame before → after | Allocs/frame |
|---|---:|---:|---:|---:|
| `RasterPerfMixed/ordinary` | 12.957 → 10.074 | -22.25% | 2,382,892 → 1,025,444 | 405 → 327 |
| `RasterPerfMixed/complex` | 14.651 → 14.290 | -2.46% | 3,261,116 → 1,304,504 | 2025 → 1557 |
| `SupersamplingMixedFrame/scale1` | 12.672 → 9.955 | -21.43% | 1,926,336 → 955,197 | 164 → 152 |
| `SupersamplingMixedFrame/scale2` | 41.343 → 41.695 | +0.85% | 8,465,728 → 4,697,021 | 166 → 154 |
| `SupersamplingMixedFrame/scale4` | 198.904 → 157.633 | -20.75% | 30,911,874 → 15,952,893 | 166 → 154 |
| `ClippedRaster/vector-baseline` | 0.389 → 0.416 | +7.09% | 926,379 → 926,383 | 13 → 13 |
| `ClippedRaster/vector-one-clip` | 0.588 → 0.602 | +2.33% | 1,172,748 → 926,915 | 22 → 16 |
| `ClippedRaster/vector-four-clips` | 0.922 → 0.970 | +5.23% | 1,166,085 → 928,517 | 34 → 28 |
| `ClippedRaster/bitmap-one-clip` | 4.674 → 4.612 | -1.32% | 1,213,938 → 968,946 | 17 → 11 |

The stable result is reduced allocation: ordinary −56.97%, complex −60.00%,
with 78/468 fewer allocations per frame. Ordinary elapsed time improved in all
five paired samples, but this shared host was not isolated. Complex elapsed
median improved only 2.46% and one paired sample regressed 4.62%. SSAA scale 2
regressed 0.85% by median. One/four-clip vector medians regressed 2.33%/5.23%.
Even the unchanged unclipped control regressed 7.09%, with one pair +220.57%.
Do not infer a universal CPU improvement or attribute all timing differences
to scratch reuse. The scale-4 paired range includes +54.11%. All raw samples,
including worse ones, remain in `interleaved-*.txt` and `interleaved.json`.
The earlier five-round 300ms trial, including severe host outliers, remains in
`trial-short/`; it is exploratory, not silently discarded. The only subsequent
candidate edit moved the private scratch type into raster.go to allow safe,
compiling publication of raster.go before clip.go; runtime logic was unchanged.

## Correctness and validation

Six fresh before RGBA SHA256 values cover ordinary/complex at scales 1/2/4;
normal tests only read them. Additional regressions compare reused and fresh
workers across changing aspect ratios, absolute origins, transparent fills,
fill rules, cameras and empty frames; retain returned images; fail both before
scratch acquisition and after painting a layer; recover with different alpha;
and render one shared immutable scene on four independent workers. Existing
budget, camera, bitmap, analytical parity, AA, stroke/dash, gradient and Cairo
reference tests remain unchanged. These references retain their documented
numerical differences and thresholds; no pixel-quality defect was repaired or
threshold relaxed by this optimization.

Fresh development-only CairoSVG 2.7.1 / Cairo 1.18.4 independently re-rendered
the two hand-authored clip scenes plus three gradient scenes. All five complete
RGBA oracles equal the old independent PNGs. Original oracle files were not
rewritten. `independent-cairo.json` records all source/image hashes and scope.
Go/Cairo edges still use the existing tolerances; oracle regeneration equality
is not a claim of Go/Cairo byte identity. The old pinned-Manim and high-resolution
partial-glyph checks also run in the full render suite.

Source snapshots, profiles, raw samples, build commands/overlays, fixture hashes
and fresh validation logs are under `tests/data/render/raster_perf/`.
`baseline.json` inventories original sources and old references. `_before/`
contains text snapshots of original render sources. `_dependencies/` and
`dependencies.json` freeze the common local dependency inputs to avoid parent
edits changing an A/B build. `_staging/` holds the exact verified candidate;
`validation.json` records delivered production hashes and final commands/results.
Binaries are development-only, hashed in measurement JSON and removed after use.

## Reproduction

From the repository root:

```sh
# Rebuild both variants from frozen before and delivered source; five rounds.
python3 tests/reference/raster_perf.py --benchtime=1s

# Standard regression: Go tests never run Cairo, Python or another renderer.
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -count=1
go run ./tests/cmd/run -pkg=render -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./render
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

`tests/reference/_raster_perf_capture/capture_test.go` is a development-only
before-corpus generator outside the central suite; never run it against the
candidate to bless different pixels. The recorded capture overlay maps all
render production files to `_before/`. `tests/reference/raster_perf_references.py`
requires a fresh output directory and explicitly rejects overwriting existing
evidence. Runtime and normal tests have no external-renderer dependency.
