# Immutable image pixels

`rasterimage.Asset` performs real byte operations and supplies the existing
immutable `*gomath.Bitmap` to the rasterizer and PNG/SVG exporters. It adds no
renderer, root object behavior, dependencies, Python runtime, or native code.
The existing `rasterimage.New`, `Decode`, and `Load` APIs and pixels are unchanged.

```go
asset, err := rasterimage.NewAsset(decodedImage) // image.Image; copied
if err != nil { return err }
inverted, err := asset.Invert()
if err != nil { return err }
colored, err := inverted.Tint(gomath.RGB(.5, 1, .25))
if err != nil { return err }
faded, err := colored.SetOpacity(.37)
if err != nil { return err }
editedObject, err := faded.WithPixels(positionedImage)
if err != nil { return err }
// editedObject has independent identity and geometry; positionedImage is intact.
// Add editedObject to a scene or export faded.Bitmap() as an image.
```

Decode a PNG into its original straight `image.NRGBA` before `NewAsset` to retain
low-alpha source RGB. Importing `positionedImage.Bitmap` is also supported, but
any color information already lost by premultiplication cannot be recovered.
`asset.NewImage(worldWidth)` creates a fresh origin-centered affine plane with
the asset's aspect ratio. Use `WithPixels` for an existing positioned image.

| API | Exact behavior |
| --- | --- |
| `NewAsset(image.Image) (*Asset, error)` | Copy and normalize origin to `(0,0)`; validate dimensions before reading pixels. |
| `(*Asset).Bitmap() *gomath.Bitmap` | Immutable premultiplied render/export view; nil for nil/zero Asset. |
| `(*Asset).Invert() (*Asset, error)` | Replace each current straight RGB byte `c` with `255-c`; preserve alpha. |
| `(*Asset).SetColor(gomath.Color, ...float64) (*Asset, error)` | Replace RGB with `trunc(255*channel)`. No alpha argument preserves current alpha; one argument sets uniform `trunc(255*alpha)`. |
| `(*Asset).Tint(gomath.Color) (*Asset, error)` | Multiply current straight RGB bytes by respective channel factors and truncate; preserve alpha. |
| `(*Asset).SetOpacity(float64) (*Asset, error)` | Set each alpha to `trunc(importTimeAlpha*factor)`; preserve current straight RGB. |
| `(*Asset).NewImage(float64) (*gomath.Object, error)` | New plane with positive finite width and representable positive height. |
| `(*Asset).WithPixels(*gomath.Object) (*gomath.Object, error)` | Independent copy of a valid leaf image, replacing only Bitmap and identity; matching pixel dimensions required. |

All color channels, including `Color.A`, and all alpha/factor arguments must be
finite and in `[0,1]`. `Color.A` is validated but does not control image opacity
in `SetColor` or `Tint`; use the explicit optional alpha or `SetOpacity`.
More than one optional alpha is an error. Unsupported inputs return a nil result
and an error without modifying inputs or publishing output.

`WithPixels` preserves the exact path control points, affine placement,
reflections, name, scalar, Z index, clip, and style, including scene opacity.
It deep-copies geometry and shares only established immutable values. It
rejects groups, children on an image node, mismatched pixel sizes, invalid IDs,
curved/nonaffine image planes, unsupported paints and invalid styles. This is
a new object, so both original and edited images can belong to the same scene.

## Pinned Manim rules and deliberate boundaries

The reference is Manim Community commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`, specifically
`manim/mobject/types/image_mobject.py`, `manim/utils/color/core.py`, and
`manim/utils/images.py`. Existing MIT notices remain in `LICENSES/` and
`THIRD_PARTY_NOTICES.md`.

Manim's constructor `invert=True` subtracts only RGB from the dtype maximum;
this API implements its `uint8` case. `set_color` assigns integer RGB, then
optionally uniform alpha. Color conversion truncates rather than rounds.
`set_opacity` scales `orig_alpha_pixel_array`, saved before edits: repeated
calls are absolute against that mask, not cumulative against the last alpha.
Consequently `.SetOpacity(0)` followed by `.SetOpacity(1)` restores the original
alpha; `.SetColor(c, .5)` does not replace the saved mask. The immutable API
returns distinct assets instead of mutating a Manim image or its family.

`Tint` is an explicit extension: multiplication in encoded sRGB byte space,
not Manim's replacement operation and not a linear-light blend. NaN, infinity,
out-of-range factors, arbitrary NumPy alpha broadcasting and dtype overflow
are rejected or outside scope. No per-pixel alpha array API is claimed.

The renderable Asset bitmap is premultiplied RGBA. A private straight RGBA
buffer preserves the imported nonzero-alpha RGB bytes and current edited RGB;
an immutable alpha mask preserves the import-time alpha. Operations always
start from that private straight color, never from repeatedly unpremultiplying
rounded bitmap bytes. For a straight byte `c` and alpha byte `a`, the stored
Go bitmap byte is `floor(floor(c*a*257/255)/256)`.

**Hidden RGB at alpha zero is discarded on import.** Such a pixel becomes
straight `(0,0,0,0)` regardless of its source hidden color. This is a deliberate
representation difference from Manim's straight NumPy array, not full hidden
color parity. Later explicit color replacement may reveal it, and inversion
operates on its canonical zero RGB. Temporarily zero alpha caused by an edit
does not discard already owned straight RGB, allowing opacity restoration.
Exporting/reimporting `Bitmap()` crosses a lossy premultiplication boundary;
do further edits from the Asset when original straight RGB matters.

`color.NRGBA64` imports truncate each 16-bit channel to its high byte before
premultiplication. Alpha values `1..255` therefore become zero and lose hidden
RGB; values `256..511` become alpha `1`. Generic premultiplied input is converted
using `color.NRGBAModel`; quantization already present in that input is explicit.
PNG exports unpremultiply the Bitmap according to the existing Go encoder and
may quantize low-alpha straight color further; they do not export the private
straight-color ledger. SVG embeds that same canonical PNG.

Ordinary `gomath.Object.SetColor` is unchanged and is not this pixel API. It is
currently a no-op on a default unpainted image object; this wave does not add a
root guard or claim that operation as supported. Painted image fills/tints,
gradients, strokes and path reveal retain their existing validation failures.
Use `Asset.SetColor` and `WithPixels` to replace image pixels explicitly.

New assets work through ordinary image geometry transforms, scene/group
opacity, FadeIn/FadeOut, and PNG/SVG/PNG-sequence exports. `Transform` still
requires the same Bitmap pointer at both ends; a geometry-only target copy
works. Distinct assets work with the existing two-layer `FadeTransform`, or
separate FadeOut/FadeIn. Pixel-by-pixel interpolation between distinct assets
is not claimed. Manim camera ownership and mutable image families are outside
this immutable asset operation API.

## Budgets and ownership

`MaxPixelBytes = 67,108,864` bounds retained **pixel buffers** for one asset.
`MaxPixelPixels = floor(MaxPixelBytes/9) = 7,456,540`. A newly imported asset owns
four straight bytes, four premultiplied bytes and one saved-alpha byte per pixel.
A derived asset allocates eight bytes per pixel and shares only the immutable
saved-alpha mask. Struct headers, allocator rounding, caller-owned source
storage and simultaneously retained assets are excluded from this buffer limit.
The existing larger `gomath.MaxBitmapPixels` remains unchanged for ordinary
non-editing image construction.

There must be at least one pixel. Each span and the product are checked against
the budget using unsigned differences and division before allocating, avoiding
signed overflow even for `MinInt..MaxInt` rectangles. Nonzero and extreme origins
with small valid spans are normalized safely. Nil/typed-nil sources, nil colors
and invalid premultiplied channel values fail with errors. As with standard Go
image APIs, custom image implementations must honor their advertised bounds
and color contracts; arbitrary panics inside caller implementations are not
recovered.

Input images must remain stable during `NewAsset`; positioned objects must
remain stable during `WithPixels`. After construction, original input buffers
can be reused or changed. Assets and their edit methods support concurrent use;
no writable pixel slices are exported. Authored scene/object mutation follows
the repository's existing rules; samples have independent mutable geometry and
share immutable bitmap storage.

## Frozen validation

`tests/data/rasterimage/pixels/oracle.json` records upstream file SHA256 values,
generator SHA256, NumPy/Pillow versions, operations and every reference-file
SHA256. `manifest.json` freezes the API, limits, implementation/tests/docs file
hashes, source/reference hashes, commands, validation outcomes and benchmark
conditions. Neither manifest hashes itself.

The development-only `tests/reference/image_pixels.py` checks the pinned source
HEAD and executes AST-extracted original `ImageMobject.set_color`,
`set_opacity`, constructor inversion/saved-alpha statements and color conversion
methods, using minimal geometry-independent state and a float64 color
constructor. This isolates byte rules without loading a native Manim renderer.
The small tint extension uses separately specified NumPy multiplication.
Pillow writes and reads all PNG references independently.

The oracle contains 13 operation chains over 65,536 pixels, covering every
alpha/R, alpha/G and alpha/B pair. Additional fixtures cover 16-bit subbyte
alpha and nine 64×64 pixel-aligned intermediate scene frames, including a
90-degree rotation, integer translations, FadeIn and FadeOut. Scene references
use independent NumPy composition and Pillow rotation; they do not claim
arbitrary-angle Manim camera/resampling equivalence. Go tests also check
transformed/clipped geometry preservation, same-asset transforms, distinct-asset
crossfades, direct PNGs, embedded SVG pixels/matrices, atomic export failures,
PNG sequences, concurrent editing/sampling/rendering, bounds, finite factors,
nil/zero values, input isolation and all existing rasterimage tests.

```sh
python3 tests/reference/image_pixels.py --source ../manim --check
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=rasterimage -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=rasterimage -tags noasm -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go test -tags noasm ./...
GOMAXPROCS=1 CGO_ENABLED=0 go run ./tests/cmd/run -pkg=rasterimage -tags noasm \
  -run '^$' -bench BenchmarkPixelOperations -benchmem -benchtime=200ms -count=3
```

The race detector needs cgo for its development runtime on this host; all
production tests/build checks and pixel implementations use pure Go with
`CGO_ENABLED=0` and `noasm`.

Benchmark workload: 256×256 RGBA, one complete immutable asset edit per iteration,
including allocation/copy/premultiplication, with asset import outside the timer.
There are no scene frames in this benchmark; PNG encoding, disk I/O, source
decoding, and scene rendering are excluded. Raw samples and host/toolchain
details are frozen in `benchmark.txt` and the manifest; this shared host does
not support an uncontended comparative speedup claim.

The first complete-suite run passed rasterimage/render/export but failed on
concurrent root transform-path tests referring to not-yet-defined APIs and on
SVG container-bbox Cairo comparisons. Those files are outside this wave's
ownership; `full-tests.txt` preserves the errors rather than reporting a clean
integration run. Consult the manifest for the final run outcomes.
