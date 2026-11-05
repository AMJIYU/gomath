# Bounded horizontal font fallback

This slice adds `text.NewWithFallback(string, []*text.Font, text.Options)` and
`text.LayoutWithFallback(string, []*text.Font, text.Options)`. The latter returns
`(*gomath.Object, text.Metrics, error)`. No existing caller changes are required.

## Font selection and ownership

The explicit list has 1–16 entries, including the primary. Every entry must be
initialized, even if unused. Duplicates are allowed. The call snapshots the slice
and never retains it; callers may change it after return but must not write it
concurrently with a call. Parsed fonts own their byte data and are immutable.
Buffers, selection caches, paths and output objects are local to each layout.
Concurrent layouts and reads of completed objects are supported; authoring
mutations still require caller synchronization.

Selection is per Unicode rune, in list order, using nonzero cmap glyph indices.
A space with a nonzero glyph index and no ink is covered. A zero index is missing
even if the font's `.notdef` happens to have ink. The first actual glyph wins.
Malformed selected outlines, cmap errors and unsupported font operations fail;
they do not trigger a silent switch to another font. Default missing-glyph policy
is an error naming the rune. `AllowMissing=true` searches the whole list first
and then deliberately selects the primary `.notdef` for uncovered runes.

Kerning uses the selected font's real supported `sfnt.Kern` table only when both
adjacent glyphs chose the same `*Font` pointer. There is no kerning across font
boundaries. A newline resets adjacency; a tab is one ordinary space. There is no
inferred tracking or cross-font metric adjustment.

## Baselines and measurements

All faces share the current baseline, with each scaled by its own units per em
to the same `Options.Size`. Ascenders, descenders and x heights can differ.
Ascent/descent are the maxima of the configured fonts' line metrics, including
unused fonts; an empty string still has one such line box. This makes those
metrics independent of which characters happen to occur.

Newlines step down exactly `Size*LineHeight`, matching existing text behavior.
There is no automatic expansion of the baseline interval for tall fallback
fonts. A small requested interval can overlap ink. Height is ascent plus descent
plus `(Lines-1)*Size*LineHeight`. Width is the widest line's accumulated advances
and kerning, including whitespace. These are line/advance metrics, not ink bounds;
glyph overhang may extend beyond them. `object.Bounds()` reports actual ink.

Single-font `New` / `Layout` use the retained outline construction path. A one-font
fallback list has identical paths, names, styles, metrics and raster pixels.
Four archived pre-extension cases cover regular, bold, italic, bold italic,
whitespace, empty lines, missing-glyph opt-in and fractional scales; they pin
every control point/style and exact pixels. These are compatibility fixtures,
separate from the independent references below.

## Bounds and unsupported text semantics

Existing bounds apply to the entire run: 65536 runes, 1048576 source glyph
segments, 64 MiB per parsed font. At most 16 font candidates can be searched for
any rune. Multiple-font calls preflight selected glyph segment counts before
constructing an output tree, caching each distinct rune's choice locally. Thus
one font cannot evade the segment cap by sharing the run with another.
Invalid UTF-8, nil/empty/oversized lists, uninitialized entries, invalid colors,
nonfinite or out-of-range size/line-height, and scale/line-step underflow fail
without a returned partial object. Size and line height each have maximum `1e6`.

This is horizontal, per-rune fallback. It does not implement shaping, bidi,
grapheme/emoji sequence selection, variation selector handling, ligatures,
automatic mark positioning, automatic font discovery or vertical text. Real
glyph coverage alone does not imply correct complex-script typography. Only
font formats and outlines supported by the existing pure-Go sfnt loader work;
there is no bitmap/color-font or native-font-engine fallback.

## Independent evidence and preview

`tests/reference/text_fallback.py` reads pinned Go Regular/Bold source fonts with
fontTools. It creates licensed complementary test subsets with unmodified
outlines/advances and declared fixture-only kerning and secondary line metrics.
The script independently reads cmap/hmtx/kern/hhea, evaluates original quadratic
outline bounds, places manually specified runs, and renders those paths through
CairoSVG/Cairo. It never consumes Go layout output. Original and derivative
hashes, table changes, tool versions and image hashes are in
`tests/data/text/fallback/reference.json`; the fixture license is alongside it.

Four reference cases exercise ordered/reversed faces, actual fallback, unused
fallback line metrics, newlines/tabs, whitespace and deliberate `.notdef` policy.
Twenty fontTools ink boxes agree within one source design unit (1/2048 em);
advance/line metrics agree within `1e-12`. Independent Cairo mask IoU ranges
0.991043–0.994934 and whole-image intensity MAE is at most 0.056895 byte levels.
Small raster differences reflect independent curve conversion/rasterization;
no pixel-equivalence claim is made for arbitrary fonts.

`tests/data/text/fallback/preview.png` and `.svg` are actual Go-rendered review
artifacts from `tests/reference/_text_fallback_preview`. Go tests verify actual
raster output against independent PNGs and export real outline paths to SVG.
They require no Python, fontTools, Cairo, system fonts or external processes.
The standalone reference tool is development-only. Reproduce its inputs with:

```sh
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib:/opt/homebrew/opt/cairo/lib PYTHONDONTWRITEBYTECODE=1 python3 tests/reference/text_fallback.py --fonts /path/to/x/image/font/gofont/ttfs
CGO_ENABLED=0 go run ./tests/reference/_text_fallback_preview
```

## Validation and measured cost

Central tests cover actual coverage order and distinct outlines, real table
kerning, forbidden cross-font kerning, primary `.notdef`, malformed selected
glyph errors, line metrics/baselines, exact legacy outputs, inclusive rune/font
caps and shared segment cap, invalid options, ownership, concurrent layout and
concurrent rendering. Commands:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -tags noasm -vet=all -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=text -tags noasm -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./text
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -tags noasm -run '^$' -bench '^BenchmarkFallbackLayout$' -benchmem -benchtime=300ms -count=3 -p 1
```

Race instrumentation needs CGO enabled during the test; product code has no
native/process dependency. The focused text suite and vet pass; purecheck reports
42 non-standard packages with CGO disabled and no native/process-launch imports.

Benchmark: Go 1.27.3, darwin/arm64, Apple M2, GOMAXPROCS 8, `-p 1`, 300 ms per
sample, three samples. Each call lays out `"AV βgA\n\tVx "` (11 runes, seven ink
glyphs, two lines), including coverage preflight and new editable object/path
allocation. Fonts are parsed before timing. Two-font samples: 12.036, 12.110,
12.250 µs/op; 51468 B/op; 121 allocs/op. Sixteen-font samples (15 primary entries,
secondary last): 12.225, 12.557, 12.850 µs/op; 51580 B/op; 121 allocs/op.
Rendering, image size, frames, file I/O and reference tools are excluded from
this layout benchmark; the independent raster cases are 768×512, one frame each.
These are workload measurements, not a general speedup or shaping-parity claim.

## Integrated fourth-wave example — 2026-11-04

The font-fallback example embeds an independently licensed three-character mathematical face and combines it with Go Regular; it reads no test/system font at runtime. Actual coverage tests establish that primary-alone fails for all three mathematical glyphs. Three640x360/SSAA4 SVG checks at.6/1.1/3.3seconds yield RGBA MAE .082518–.088158, maxima18/18/20; Write interiors are included. This shares Go layout construction; independent typography evidence remains the earlier fontTools/Cairo fixtures.
