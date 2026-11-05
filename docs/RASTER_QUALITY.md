# Optional 2D raster supersampling

`render.Config.Supersampling` selects 1, 2, 4, or 8 samples per axis.
`DefaultConfig()` selects 1; zero also means 1 for existing config literals.
Other values return an error. This is an explicit quality/cost control, with no
implicit fallback or automatic selection.

```go
c := render.DefaultConfig()
c.Width, c.Height = 640, 360
c.Workers = 1
c.Supersampling = 4
r, err := render.NewRasterizer(c)
if err != nil {
    return err
}
pixels, err := r.Render(frame) // still 640 by 360
```

The same config field works with `render.RenderScene`. This API belongs to the
2D raster renderer; it does not add sampling to vector SVG output or the separate
3D rasterizer. SVG export consumes the shared `Config.Validate`, so a caller
reusing a raster config for vector export should deliberately choose the raster
sampling/worker values it wants validated. The CLI `demo` and `benchmark` commands
expose `-supersampling 1|2|4|8`. PNG/SVG outputs validate the supplied worker range
but charge one actual worker; multi-frame raster outputs retain their configured
worker budget. SVG remains vector output and gains no raster sampling from this
flag. Codecs consume the resolved original-size images.

## Rendering and resolve

At scale S, the complete frame renders at `(Width*S, Height*S)`: filled paths,
curves, stroke widths/caps/joins/dashes, inherited clip intersections, gradient
fields, affine bitmaps, backgrounds, object opacity, and camera projection all
use those working dimensions. Frame width and output aspect ratio stay fixed.
`CurveTolerance` is measured in **working pixels**: its existing default 0.02
therefore becomes a tighter world-space tolerance when S increases.

Each S-by-S block resolves into one original-size output pixel by averaging its
already composited premultiplied RGBA byte channels. Each average rounds to nearest,
with half upward. Alpha is averaged exactly once, together with RGB; there is no
unpremultiply/reweight step that could introduce transparent-color fringes.
This is averaging of encoded sRGB bytes, not linear-light filtering. Accumulators
are uint32; the maximum channel sum at 8x is 16,320.

Odd output dimensions need no special padding or cropping: every output pixel,
including the last row and column, has exactly S*S working samples. Each result
owns fresh output storage. Errors return a nil image, including errors occurring
after earlier drawables have painted the private canvas. `RenderScene` retains
its existing abort-on-error sink contract; it can already have delivered earlier
frames, so sink cleanup remains the sink's responsibility.

The public rasterizer keeps the original config dimensions. A private working
copy renders once, directly, without recursive `Render` calls. Camera view config
is restored after each frame. Rasterizer scratch may be reused; returned output,
input geometry, immutable clips/gradients/bitmap assets and caller config are not
reused as writable scratch. A `Rasterizer` is reusable by one worker and is **not
safe for concurrent Render calls**. `RenderScene` creates a separate one per worker.

Scale 0/1 calls the original rendering path directly: no added image/resolve or
pixel budget, and existing pixels and ordinary config acceptance are preserved.
The tests compare the original private rendering path byte-for-byte against 0/1
with transparent, opaque, and partially transparent backgrounds.

## Checked limits

For S greater than 1, validation runs before raster allocation. There is no
budget-based scale reduction.

| Limit | Value |
| --- | --- |
| Allowed scales | 0/1, 2, 4, 8 |
| Each working dimension | at most 16,384 |
| `MaxSupersamplingPixels` | 16,777,216 working pixels |
| `MaxSupersamplingWorkerBytes` | 448 MiB conservative pixel payload per worker |
| `MaxSupersamplingTotalBytes` | 1 GiB configured-worker plus queued-output pixel payload |
| Configured worker count | existing range 1 through 256 |

Dimension multiplication is checked by division first; later pixel/payload
arithmetic uses int64 under those validated dimension/worker limits.

Let P be output pixels, H = P*S*S working pixels and W configured workers:

- Worker payload estimate B = `28*H + 4*P` bytes; B must be at most 448 MiB.
- Aggregate estimate = `(B + 8*P)*W` bytes; this must be at most 1 GiB.

The 28 working bytes reserve the RGBA canvas (4), both vector rasterizers'
retained float/fixed/mask buffers (16), retained gradient alpha mask (1), a clipped
RGBA layer plus two clip alpha masks (6), and a bitmap alpha mask (1). The extra
4 output bytes reserve the resolve result. Aggregate accounting additionally
reserves two output images per configured worker, matching `RenderScene`'s bounded
outstanding-frame slots, including results awaiting order.

**All configured workers are charged**, even for `NewRasterizer` used alone or a
sequence shorter than the worker count. Set `Workers=1` for a serial caller. For
example, 640x360 at 8x is valid with one worker but fails the conservative aggregate
budget at four workers. This remains true even if a caller would only use one of
the four workers. `DefaultConfig().Workers` depends on GOMAXPROCS.

These are conservative limits on simultaneously live **pixel payload**, not an
RSS/Go heap cap or an allocation-free promise. They exclude caller-owned input
geometry/assets, geometry/projection/flattening data, allocator and garbage
collector overhead, temporarily uncollected buffers, and outputs retained by a
sink or caller after delivery. Existing clip, gradient, stroke and geometry
limits continue to apply independently at the actual working resolution: a frame
that fits the canvas budget can still return a bounded-work/flattening error.
No Go OOM-recovery guarantee is made.

## Known antialiasing boundary and independent evidence

Default fill AA uses `golang.org/x/image/vector`. In the confirmed partial glyph
case, opposite signed areas of a self-intersection can cancel within a pixel
before coverage is clamped. This differs from integrating the nonzero-filled
region. See `tests/data/render/partial-glyph-aa/INVESTIGATION.md` and its frozen
Cairo/Skia area evidence. It is a **fill** issue, with stroke width zero in the
three affected glyphs, not an omitted acute stroke join.

Supersampling reduces the area in which this cancellation occurs. It does not
replace the underlying algorithm, guarantee exact nonzero-region AA at any
finite scale, or fix the default 1x limitation. Arbitrarily small intersections
can still occur inside a working pixel. Neither default output nor existing
pixel-reference thresholds have been changed.

For the white one-pixel bowtie `M4 4L5 5L4 5L5 4`, nonzero-filled area is exactly
one half pixel. Default native alpha is 0; optional 2/4/8x each resolves to 128,
within rounding of the analytical alpha 127.5. For three frozen partial `2` glyph
pixel crops, maximum absolute alpha errors against independently computed Skia
nonzero region area*255 are 31.514 at 2x, 8.208 at 4x, and 2.208 at 8x. These are
fixture results, not global error bounds.

`tests/reference/raster_supersampling.py` generates independent CairoSVG renders
at 1/2/4/8x and premultiplied Pillow BOX references. Fixtures and hashes live in
`tests/data/render/supersampling/provenance.json`; tests require no Python/Cairo
runtime. Versions: Python 3.11.5, CairoSVG 2.7.1, Cairo 1.18.4, Pillow 11.2.1.
The full fractions .35 reference freezes the actual parent SVG export, sharing
scene construction but independently interpreting/rasterizing it. It does not
establish independent formula construction or Manim parity.

Fractions .35, 640x360, premultiplied RGBA channel differences:

| Native scale | Against original-size Cairo MAE / RMS / max | Against matching Cairo+BOX MAE / RMS / max |
| --- | --- | --- |
| 1 | 0.032938 / 0.472951 / 83 | 0.032938 / 0.472951 / 83 |
| 2 | 0.034172 / 0.461258 / 34 | 0.018638 / 0.239350 / 31 |
| 4 | 0.035637 / 0.470945 / 18 | 0.010340 / 0.143784 / 8 |
| 8 | 0.036227 / 0.477809 / 18 | 0.006029 / 0.104769 / 7 |

MAE/RMS/max average over all four channels, including alpha; channel range is
0 through 255. Higher scale need not monotonically reduce error against a
**native-size** Cairo result, which has its own coverage approximations.

Additional 95x63 transparent references exercise crossings, compound holes,
duplicate nonzero contours, alpha gradients, thin and dashed strokes, nested
clips and an embedded 2x2 alpha bitmap. Crossing RMS against matching Cairo+BOX
is 1.549840 / 0.883762 / 0.793431 / 0.160669 for scales 1/2/4/8. Duplicate/hole
RMS is 6.857178 / 3.515350 / 1.677546 / 1.027707. The mixed scene remains around
RMS 4.6, max 55 at all scales; this option does not make every existing sampling
or compositing result agree with Cairo. Hole interiors remain transparent,
duplicates do not double interior opacity, and boundary comparisons have explicit
new fixture-specific bounds. No old reference fixture or threshold is relaxed.

## Validation and measured cost

Verified on November 4, 2026, Apple M2, darwin/arm64, Go 1.27.3:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=render -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./render
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -tags noasm -vet=all -run '^$'
```

Focused render/noasm and race passed. Purecheck passed for 42 non-standard
packages with no native inputs or runtime process-launch imports. Tests cover
scale/config/arithmetic/pixel/worker limits, premultiplied resolve, transparent
odd-size images, default compatibility, complete high-resolution pipeline,
camera compensation/config isolation, output/source ownership, parallel ordered
frames, and working-resolution clip-budget failure returning no partial image.
Repository-wide integration and crossbuild validation are the parent's checks.

Benchmark command (serial subbenchmarks, one render worker):

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -run '^$' \
  -bench '^BenchmarkSupersamplingMixedFrame$' -benchmem \
  -benchtime=300ms -count=3 -p=1 -tags noasm
```

One complete 640x360 frame per operation: compound-hole alpha gradient with
stroke and rotated clip, rotated clipped 2x2 immutable alpha bitmap, and a dashed
polyline with square caps and round joins. Frame width is 110 world units.
Construction and one scratch-warming frame per scale are excluded. Per-frame
render/resolve/output allocations are included; sampling/encoding/I/O are excluded.
GOMAXPROCS=8; rendering uses one worker. This is one mixed workload, not a general
FPS or Manim speedup claim.

| Scale | Working pixels/frame | Three samples, ms/frame | Median ms/frame | B/op | allocs/op |
| --- | --- | --- | --- | --- | --- |
| 1 | 230,400 | 8.878330, 8.830718, 8.874703 | 8.874703 | about 1,926,340 | 164 |
| 2 | 921,600 | 39.164818, 38.993427, 39.259130 | 39.164818 | about 8,465,750 | 166 |
| 4 | 3,686,400 | 155.160292, 152.297271, 152.226416 | 152.297271 | 30,911,880 | 166 |

Sampling cost and storage grow with working pixels (S squared), with additional
curve subdivision and geometry work. Actual per-frame allocation is distinct
from the conservative simultaneously live pixel-payload limits above.
