# SVG container object bounding boxes — wave 8

`svg.Read` now accepts `clipPathUnits="objectBoundingBox"` on bounded `g`,
`text`, and `tspan` targets, and on `use` chains that expand to supported
shapes, images, groups, or text. This is an SVG importer extension. The pinned
Manim source remains unchanged; this document makes no new Manim parity claim.
The runtime remains pure Go, with no native renderer, process fallback, public
API addition, or dependency change.

## Coordinate and geometry contract

The target's bbox is measured in its own user space, before its own transform.
Descendant transforms, including `use` x/y and source transforms, contribute.
The final clip mapping remains

```
target CTM × translate(bbox.min) × scale(bbox.size) × clipPath transform
```

`svg/container_bbox.go` maintains private target-local geometry scopes alongside
the normal expansion matrices in `svg/instance.go`. A descendant contributes its
original outline to every active scope. Each outline is transformed into that
scope **before** its cubic derivative extrema are measured. Transforming a
previously computed AABB, using the control hull, or measuring the finished
object's painted bounds would produce incorrect results for curved/sheared
children. Nested target scopes measure independently and retain unclipped
geometry. Singular target transforms need no matrix inverse. Every scope CTM,
transformed outline, extrema calculation, and bbox clip mapping must remain
finite; overflow returns an error.

These scopes reuse the existing document, source parsing, CSS matching, use
cycle validation, expanded-element counter, text layout, and per-document clip
ledger. No separate document traversal or clone cache is introduced. There was
no need to change `svg/document.go`: its existing reference graph validation and
shared clip budget remain the single import-wide authorities.

Object geometry excludes strokes, clipping (including descendant clips), fill
rule, paints, and opacity. An image contributes its SVG image viewport rectangle
before `meet` placement; transparent pixels and letterboxing do not shrink it.
`visibility:hidden`, `fill:none`, and `opacity:0` still contribute geometry.
`display:none` removes a subtree from its ancestor's bbox, including descendants
that specify `display:inline`. Inert definitions never contribute merely because
they occur below a group. Existing strict validation of hidden inputs remains.

Use expansion keeps the existing shadow selector boundary and inheritance.
Source light-tree ancestors do not contribute styles or transforms to clones.
The outer use's bbox includes its x/y and all supported target transforms; the
use's own transform stays outside that local bbox. Independent uses, copies,
and sampled scene snapshots retain separately owned geometry and native clips.

Empty and zero-width/height bboxes ignore the object-bbox effect, preserving the
existing basic-shape behavior. They still spend work budget. Invalid content or
an unsupported branch returns `(nil, error)` from `svg.Read`, including failures
after earlier siblings have expanded.

## Text: glyph cells versus ink

The SVG 2 bounding-box algorithm explicitly treats each glyph as a separate
graphics element occupying its **full glyph cell**. For the supported horizontal
layout this means the glyph's horizontal advance and the font's full ascent and
descent. This is different from the visible outline's ink bbox. Side bearings,
descenders, and preserved spaces can make the difference substantial; a
space-only preserved string has a bbox despite having no ink outlines.

`svg/text.go` retains its real outline layout and mixed-content draw order.
Cells are positioned in the same kerned runs as those outlines, retain their
individual tspan ownership, and receive the same completed chunk anchor shift.
A tspan's bbox therefore excludes neighboring characters while preserving the
cursor position and kerning established by them. Absolute/scalar relative
positions, nested spans, font-size changes, start/middle/end anchors, whitespace
normalization, and visibility follow the existing bounded text model. A group's
text contribution transforms each cell into the group's coordinate space.

Cell metrics come from `gomath/text.Layout` with the same resolved immutable
font and size as painting. A per-import `(font pointer, size, rune/pair)` cache
stores scalar metrics, never mutable glyph outlines. Cache misses use the
existing layout-call limit. Temporary metric outlines also spend the existing
glyph, segment, expanded-element, and clip-geometry budgets. Glyph-cell boxes
and ink outlines are deliberately not interchangeable. Text object-bbox
**gradients** remain unsupported as before; this change concerns clip targets.

Tspan bbox clips are materialized only after all text chunks and anchors finish.
Inherited clip values are still evaluated independently for each target.
Existing text limits and explicit errors for unsupported scripts/shaping,
position lists, `textLength`, bidi, text paths, and tspan transforms remain.

## Bounded and rejected branches

- Supported target trees: basic/path/image, `g`, horizontal `text`/`tspan`, and
  finite local `use` chains to those elements.
- A referenced `symbol` anywhere in an active container bbox tree is explicitly
  rejected, including `overflow="visible"`. Unreferenced symbol definitions are
  still inert. Nested SVG viewports and bbox clips on root SVG or symbol targets
  remain explicitly rejected. Symbol viewport dimensions, viewBox mapping,
  reference points, and clipping need their own correct geometry model before
  this restriction can be removed.
- Clip **contents** retain their existing supported shape/direct-use and bounded
  union rules. This wave does not enable group/text/symbol silhouettes inside
  clipPath, bbox clips on a clipPath container or a clipping-use host, or
  clone-local clip definitions. Normalized clip geometry still requires finite
  unitless lengths; percentages and units are not silently converted.

Basic/path/image clips and direct basic/path/image use targets retain their
existing measurement path, output, and exact budget boundaries. Five obsolete
container/text/use-chain rejection entries were removed from the legacy
`clip_bbox_test.go` table; all remaining rejection cases, old fixtures, and
legacy comparison thresholds remain unchanged. Positive replacements and
further viewport rejection cases live in `*container_bbox*_test.go`.

## Shared limits

Every new container scope spends one empty-path charge, even if it has no
geometry. Each original geometry/cell contribution spends two path/curve/contour
charges per active scope: the scan and the temporary affine-transformed outline.
Materialized clips spend their existing charges. Geometry remains unpainted in
this ledger; there is no stroke expansion or boolean operation for bboxes.

The existing import-wide limits remain 8,192 path/materialization charges,
1,048,576 curves, and 65,536 contours, shared with source clip plans, all clip
instances, and old/new bbox work. XML and expanded graphs retain the existing
100,000 element and 128 depth limits; clip ancestors retain the native limit of
64. Source/expanded text, font, glyph, segment, and layout limits are unchanged.
Nested bbox targets spend geometry once for each receiving scope. Cache hits
avoid actual metric layout work without refunding earlier reservations.

## Independent evidence and reproduction

All test sources, fixtures, reference scripts, numerical data, and performance
logs are under `tests/`. `tests/reference/svg_container_bbox.py` independently
authors direct Cairo paths, affine transforms, and bbox clips. fontTools reads
the pinned original font tables and outlines. The generator consumes no Go
source, production bbox values, exported SVG, or Go-rendered pixels.

The fixture set contains eight static SVG scenes, ten exact numerical bbox
oracles, and five animation snapshots at `0, .25, .5, .75, 1`, each rasterized
by Cairo at SS1 and SS4. The animation test executes actual linear Go
`Transform` sampling between original and affine-transformed snapshots; the
Cairo reference independently interpolates the affine control-point mapping.
Scene samples and separate imports are also exercised concurrently under race
detection. Camera inverse-transform checks allow at most one channel level.

The exact numerical checks use a `1e-8` SVG-user-unit tolerance and include an
analytic sheared-cubic derivative root, font advances/ascent/descent, preserved
spaces, kerned tspan ownership, transformed text cells, use chains, and image
viewports. Additional curve tests span source sizes `1e-100` through `1e100`.

New raster checks use MAE ≤ 0.35 and at most 2% of pixels differing by more than
8 channel levels. Their maximum channel difference is ≤ 80 at SS1 and ≤ 45 at
SS4. Native rendering multiplies independently antialiased clip coverage;
Cairo combines geometry before coverage, producing localized SS1 corner
differences. Measured maxima are 78 at SS1 and 31 at SS4; measured MAE is at most
0.215. The older fixtures keep their original maximum of 45 and all original
thresholds. Numerical bbox checks are separate from raster tolerance.

```sh
# Development references only; adjust the font cache and dylib paths per host.
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
  python3 tests/reference/svg_container_bbox.py \
  --fonts /Users/eleven/go/pkg/mod/golang.org/x/image@v0.47.0/font/gofont/ttfs

CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=svg -race -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -vet=all -run '^$'
CGO_ENABLED=0 go test -tags noasm ./...
CGO_ENABLED=0 go vet -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

`tests/data/svg/container-bbox/provenance.json` records renderer/font versions
and reference hashes. `manifest.json` records the frozen source hashes,
commands/results, measurements, reference commit, and legacy fixture checks.
The benchmark imports 64 curved group-use targets and renders one 128×96 frame
at SS1/SS4 with one worker. Rasterizer construction, file I/O, reference
generation, and PNG encoding are excluded; import and frame allocation count.
