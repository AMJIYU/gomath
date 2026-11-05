# Font fallback test fixtures

`primary.ttf` and `secondary.ttf` are permissively licensed derivatives of
Go Regular and Go Bold from `golang.org/x/image@v0.47.0`; see `LICENSE.txt`.
Original glyf outlines and horizontal advances are retained. Their cmap subsets
are deliberately complementary; names identify the test derivatives.

For deterministic boundary tests, these derivatives add **fixture-only** kern
tables. The secondary also sets hhea ascent/descent to 2300/-700 design units.
These are declared test inputs, not claims about the original Go fonts' kerning
or line metrics. Original Go fonts have no kern table. `broken.ttf` intentionally
corrupts the primary A contour count to exercise lazy outline errors; do not
use it as a font asset.

`reference.json` records original/derivative font hashes, fixture modifications,
independent analytical layouts and raster hashes. `ordered`, `reversed`,
`primary-ink` and `missing` SVG/PNG pairs come from original font-table analysis
and fontTools/CairoSVG, with no Go output read. `preview.svg` / `preview.png`
come from Go for visual review only and are not accuracy oracles.

Regenerate independent references with `tests/reference/text_fallback.py`.
Generate the Go review preview from the repository root:

```sh
CGO_ENABLED=0 go run ./tests/reference/_text_fallback_preview
```

The separate `../legacy/` snapshots pin the pre-extension single-font algorithm,
with fallback-only edits removed before snapshot generation, including exact
control points/styles/metrics and pixels. They are compatibility fixtures,
not independent font-layout accuracy evidence.
