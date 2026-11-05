# fractions .35 partial glyph fill-AA investigation

2026-11-04. Investigation delivered without production changes. The original demo comparison still fails maximum delta 64 with maximum delta 83. No threshold was relaxed and this report does not claim a renderer fix.

## Finding

All three reported pixels belong to **filled, open partial `2` outlines**, not stroked glyphs. Source SVG paths 70, 84, 115 have fill `#58c4dd`, fill-opacity 1, stroke-opacity 0, stroke-width 0. `render.paint` only calls `strokeStyledLine` when stroke alpha and width are positive; those glyphs never call it. Both SVG fill and native fill implicitly close the open partial contour. That closing segment crosses the curved outline inside the selected pixel.

The low-resolution fill-mask backend, `golang.org/x/image/vector v0.47.0`, accumulates signed pixel winding area and then takes its absolute value. In this scene the glyph ROIs use its fixed-point raster path. The relevant dependency functions are `fixedAccumulateOpOver`, `fixedAccumulateOpSrc`, `fixedAccumulateMask` in `vector/raster_fixed.go` (lines 256–315); the floating counterparts apply the same accumulation/absolute-value approximation. `render/raster.go` constructs the flattened closed fill contour, then calls `z.Draw`.

For regions of opposite winding inside one pixel, these quantities differ:

- Backend approximation: `min(1, abs(integral(pixel, winding)))`.
- Nonzero filled-area coverage: `integral(pixel, winding != 0)`.

The first can cancel opposite lobes even though both belong to the filled region. This is a fill-AA approximation limitation, not evidence of an acute stroke join, incorrect miter limit, missing cap, omitted glyph path, or exporter stroke mismatch. It is larger than numerical roundoff. Full glyphs at the parent's later instants do not have this particular partial-path closing intersection.

## Minimal linear reproduction

16×16 transparent canvas, white nonzero fill, no stroke:

```svg
<path fill="white" fill-rule="nonzero" stroke="none"
      d="M4 4L5 5L4 5L5 4"/>
```

The entire bowtie lies in pixel (4,4). Two opposite-oriented triangles each occupy area 1/4: true nonzero filled area is 1/2, signed net area is zero. Native alpha is 0. CairoSVG alpha is 126; at 32×, area-averaged Cairo alpha is 127.5098 and native alpha is 127.5313. The exact polygon area corresponds to alpha 127.5. No cubic flattening, text, animation, stroke, export, or image import is required to reproduce it.

## Frozen glyph evidence

Source: `artifacts/fractions-demo-svg-20261104/frame-00.svg`, SHA256 `5ffe25d2f8910a7d7934fc3832c70bb735dfbb0e234187994c8105b778c30f16`. The generator freezes its selected cubic controls into 16×16 pixel-space crops. Pixel (4,4) is the original selected source pixel. Coordinates use the source 45px/world-unit scale (its emitted extra decimal digits change these coordinates by less than 1e-9px).

| Source pixel | Native alpha, default .02px | Native alpha, 1e-5px | Cairo alpha, native size | Native alpha, 32× area average | Cairo alpha, 32× area average | Skia pixel area ×255 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| (293,70) | 10 | 12 | 113 | 103.2832 | 103.5527 | 103.5140 |
| (293,111) | 10 | 12 | 114 | 102.9561 | 103.2568 | 103.2079 |
| (389,108) | 44 | 43 | 137 | 131.8799 | 132.2510 | 132.0713 |

An independent fine-flattened oriented-area clip gives absolute signed pixel area ×255 of 12.0613, 12.8328, 43.1368. Native with tightened curve tolerance tracks that **signed** area rather than nonzero region area. Tightening flatten tolerance therefore does not fix the discrepancy. Conversely the 32× native averages agree with both independent region-area and Cairo 32× averages within one alpha unit. This confirms that enlarged geometry retains the lobes; resolution determines the amount of pixel-local cancellation.

Cairo itself is also an AA approximation: its native-size 113/114/137 is not exactly the Skia region-area 103.5140/103.2079/132.0713. The investigation does not treat Cairo native alpha as mathematical truth. Skia uses float32; frozen local crop coordinates and the separate exact bowtie keep its rounding error immaterial to this diagnosis.

## Scope and remaining limitation

No changes were made to render/stroke.go, root APIs, fill painting, SVG exporter, examples, parent comparison script, or main docs. A production correction would have to address **fill-mask winding evaluation before pixel-area averaging**, rather than adjust stroke joins or export attributes. No speculative normalization, curve tolerance change, global supersampling, or threshold relaxation was added. The original .35 maximum-delta failure remains unresolved at native resolution.

The central test is a diagnostic and a **32× geometry/reference regression**, not a passing low-resolution bug regression. It logs the low-resolution discrepancy without asserting that erroneous alpha values are desired. Its fixed bound is alpha error ≤1 between the enlarged native result, enlarged Cairo result, and independent Skia pixel-region area. A future fill-mask fix can improve native-size coverage without breaking that test.

## Files and validation

- `tests/_cases/render/partial_glyph_aa_test.go`
- `tests/reference/partial_glyph_aa.py`
- This directory: four frozen SVGs/PNGs, `cases.json`, `provenance.json`, this report.

Reference generation: Python 3.11.5, pathops 0.9.2, CairoSVG 2.7.1, Cairo 1.18.4. Script/source/fixture SHA256 values are in provenance.json. Test execution: Go 1.27.3, darwin/arm64, Apple M2.

```sh
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib /usr/local/bin/python3.11 tests/reference/partial_glyph_aa.py artifacts/fractions-demo-svg-20261104/frame-00.svg
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -run TestPartialGlyphFillAA -v -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=render -run TestPartialGlyphFillAA -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./render
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

All listed Go checks passed. The temporarily encountered scene3d parallel compilation state was resolved before these results. No full-repository test run or production benchmark was performed; production code is unchanged.
