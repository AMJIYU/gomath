# Number reference data

`formats.json` stores exact binary64 bits, independent Python fixed formats,
pinned Manim formats, the documented zero-sign policy, exact Python integer
formats, and five sampled linear-tracker values. `layout.json` stores source
font hashes, generator/fixture/image hashes, manually specified hmtx/glyf/kern
placements and real-font Cairo outputs. Neither is derived from Go output.

The 20 top-level SVG/PNG pairs are the independent source-font/Cairo reference.
`preview/` contains Go-rendered Write/Transform intermediate artifacts for visual
inspection only; those previews are not an oracle. `validation.json` records
this slice's checks, bounds, measurements and source hashes.

`restricted.ttf` is a renamed subset of the real Go-Regular face, with authentic
outlines/advances for `012+,. AV` and a fixture-only AV kern of -160 design units.
The missing `9` and minus intentionally exercise late atomic failure. The source
is `golang.org/x/image@v0.47.0/font/gofont/ttfs/Go-Regular.ttf`; its hash is recorded
in `layout.json`. See `LICENSE.txt` for the retained Bigelow & Holmes/Go notice.

Development-only reproduction (requires NumPy, fontTools, CairoSVG and Cairo):

```sh
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib:/opt/homebrew/opt/cairo/lib PYTHONDONTWRITEBYTECODE=1 python3 tests/reference/number.py --manim ../manim --fonts /Users/eleven/go/pkg/mod/golang.org/x/image@v0.47.0/font/gofont/ttfs
```

Tests/runtime require none of those reference tools. These are font/format
references, not a full TeX, Pango or Manim layout/render oracle. The Python
reference repository stays read-only.
