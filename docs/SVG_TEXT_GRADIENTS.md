# SVG text objectBoundingBox gradient fills

`svg.Read` now supports bounded linear and concentric radial objectBoundingBox
fills on real `text`/`tspan` outlines, including existing local `use` expansion.
The public API and dependencies are unchanged. Product rendering remains pure
Go; Python, fontTools, and Cairo are independent development reference tools.

```xml
<svg viewBox="0 0 128 96">
  <defs>
    <linearGradient id="g">
      <stop stop-color="red"/>
      <stop offset="1" stop-color="blue"/>
    </linearGradient>
  </defs>
  <text x="12" y="58" font-family="Go" font-size="28">
    A<tspan fill="url(#g)">V</tspan>g
  </text>
</svg>
```

The V uses the bbox of the entire text, including the A and g. A gradient does
not restart at each glyph, tspan, positioned chunk, font run, or fill override.
This follows SVG 1.1 Text's ObjectBoundingBoxUnitsTextObjects rule. SVG 2
BoundingBoxes supplies the full glyph-cell geometry: each advance spans the
font ascent and descent, including preserved spaces and bearings. Original
font outlines supply the visible ink; their ink bounds are not the paint bbox.

The existing `container_bbox.go` glyph metrics cache and transformed scope
ledger supply this geometry. Positioning, whitespace processing, kerning,
font changes, and start/middle/end anchoring finish before paint resolution.
Every laid-out character contributes, including `fill:none`, visibility:hidden,
zero opacity, and solid/currentColor overrides. `display:none` characters are
excluded from layout and bbox geometry. A tspan's existing bbox clip scope
remains separate from its enclosing text's gradient scope.

Bbox measurement stays in text user coordinates. Paint frame order is:

```
normalization × ancestors/use placement × text transform × local glyph-cell bbox
              × gradientTransform × gradient primitive frame
```

Rotation, reflection, nonuniform scale, and shear therefore preserve the local
paint frame. Each use clone resolves inherited font size, anchor, fill URL,
fill-opacity, and descendant styles in its own shadow tree. Source ancestors
are excluded, and selectors cannot cross the use shadow boundary. Inherited
fill is a paint reference; it is not an aggregate bbox for a group or use.
Gradient stop currentColor continues to resolve in the definition/template
style environment, while solid fill currentColor resolves per text descendant.

A zero-width/height or invalid-empty geometry bbox yields transparent fill and
preserves stroke. Empty text and text with only non-outlined whitespace create
no visible glyph objects. A fontTools-derived zero-advance font demonstrates
actual glyph ink with zero cell width and unchanged stroke. Singular,
ill-conditioned, or excessive nondegenerate paint frames still return errors.
Errors never return a partial imported root.

Resource accounting uses the existing per-import ledgers. One additional
bbox scope is created only when an effective laid-out fill needs bbox units.
Every scoped cell charges geometry scan and transformed path work; metrics
misses charge layout, glyph, segment, expanded-element and geometry budgets.
Queued glyph paints charge the 8,192 instance / 1,048,576 copied-stop limits
before allocation; deferred resolution spends no second paint allowance.
Scopes, queues, caches and object paths are import-local. Gradient assets are
immutable, and copies, use clones and concurrent samples preserve ownership.
Solids and userSpaceOnUse text do not allocate an extra bbox scope.

Validation is in `tests/_cases/svg/text_bbox_gradients*_test.go` and
`tests/data/svg/text-bbox-gradients/`. The independent generator
`tests/reference/svg_text_bbox_gradients.py` reads original pinned Go font
hmtx/kern/hhea tables and outlines with fontTools and paints direct Cairo
patterns. It consumes no Go values, exports, source code, or raster output.
`oracle.json` records expected affine paint matrices and stop colors; 11 SVG
cases have Cairo references at SS1 and SS4. Affine animation is checked at
0, .2, .5, .8 and 1, including concurrent sampling and camera inversion.
`--verify` regenerates 45 files in memory and checks exact bytes and hashes.
`provenance.json` pins fonts, tools, generator and spec references.

New raster limits are MAE <= .12, SS1 max <= 40 / SS4 max <= 24, and at most
.005 of pixels with any channel delta > 8, in premultiplied RGBA 0..255.
Existing fixture data, fixture tests and their thresholds are unchanged. The
single obsolete general rejection of text bbox fills was removed from
`gradient_test.go`; unsupported transformed tspans remain covered explicitly.
Resource boundaries, zero bbox, inherited paint overrides, malformed frames,
clone-local definitions, partial-output errors and identity are checked.

Existing limits remain: horizontal unshaped Latin/Greek/Cyrillic/Common text,
scalar absolute x/y/dx/dy, exact immutable font resolution, no textPath,
textLength, per-character positioning, bidi/shaping or tspan transforms.
Gradient stroke servers, nonconcentric focus, nonzero fr, repeat/reflect,
linearRGB, external/fallback URLs and clone-local definitions remain rejected.
Nested SVG viewports and container bbox scopes crossing symbol viewports keep
their existing explicit rejections. This adds no viewport-clipping solution or
isolated group compositing.

The source context remains Manim 0.22.1 at
`16ea443e2a5f1d79db0a721161411ce916d6e733`. These are independent SVG/font/Cairo
checks, not a new claim of Pango/browser/Manim text or full SVG parity. Shared
parity documentation, examples and root updater lifecycle work belong to the
parent integration and are not changed by this scope.
