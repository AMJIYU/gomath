# Bounded vector code listings

`code.New(source string, options code.Options) (*code.Code, error)` builds a
syntax-colored, editable vector listing in pure Go. Go is the supported language;
`Language: ""` also selects Go. Other strings return `ErrLanguage`. Source is
lexed with the standard `go/scanner` and is never executed. No files, highlighter
processes, system fonts, Pango, Pygments, or runtime Python are needed.

The production package is `gomath/code`. Tests and independent fixtures are
centralized under `tests/`; no runtime highlighter or native dependency is used.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=code -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=code -race -count=1
```

```go
opt := code.DefaultOptions()
opt.LineNumbers = true
opt.Background = true
opt.FirstLineNumber = 10
listing, err := code.New("package demo\n\nfunc answer() int {\n\treturn 42\n}\n", opt)
if err != nil {
    return err
}
listing.Object.MoveTo(geom.V(0, 0))
scene := gomath.NewScene()
if err := scene.Play(1, gomath.Write(listing.Object)); err != nil {
    return err
}
```

## Font and layout contract

`DefaultFont() (*text.Font, error)` lazily parses the licensed embedded
`golang.org/x/image/font/gofont/gomono` regular TTF with `text.Parse`. It returns
one immutable shared parsed font. A custom `Options.Font` must be initialized;
space must have a positive advance and every used visible glyph must have that
same advance. Used glyphs with a different advance, missing glyphs, outline load
failures and visible runes without an outline return `ErrFont`. The error names
the affected Unicode scalar. No `.notdef` substitution or fallback occurs.

Font extraction uses `text.Layout` one rune at a time at unit em, then scales
and positions independent outlines. Thus there is no inter-glyph kerning,
ligature substitution or shaping. Glyphs retain editable cubic outlines and
export as paths, including counters and contour winding.

The first source baseline is `(0,0)`. Each rune occupies one cell with the
font's space advance times `Size`; tabs advance to the next multiple of
`TabWidth` cells. Leading and trailing spaces affect grid width but create no
fake glyphs. LF and CRLF each break one line; a final terminator adds an empty
line, and empty input has one line. Bare CR is rejected. Source text and byte
offsets retain the original CRLF bytes. Tabs inside literals and comments follow
the same physical grid rule.

Line numbers are decimal, right aligned, and end two cells left of the source
baseline origin. The gutter reserves the widest displayed label. The optional
background is a plain filled rectangle around the complete line-box grid,
gutter and `Padding`. It is the first child so it paints behind the listing.
No window title, rounded frame, buttons or scroll viewport are supplied.

`Metrics()` returns initial local `CellWidth`, `LineAdvance`, font `Ascent` and
`Descent`, whitespace-inclusive `ContentWidth`, outer `Left/Right/Top/Bottom`,
and counts `Lines`, `Columns`, `Glyphs`, `Curves`. Outer metrics include padding
and the configured gutter even when the rectangle is disabled. Counts include
line-number ink and background curves. These metadata do not change after
authoring transforms.

## Highlighting, selections and ownership

`Category` values are `Keyword`, `Identifier`, `Number`, `String`, `Comment`,
`Operator`, `Punctuation`. Builtin functions, types, `true`, `false` and `nil`
are lexical identifiers. Explicit semicolons are punctuation; scanner-inserted
semicolons are omitted. `//line` directives never rewrite physical metadata.
Lexically valid incomplete snippets are accepted; no parsing or type checking
is performed.

`DefaultPalette()` returns explicit sRGB values. A nonnil `Options.Palette` is
copied immediately; later palette edits cannot change existing geometry. The
palette has one color per category plus `LineNumber` and `Background`.

| API | Result and ownership |
| --- | --- |
| `Text() string` | Original source, including CRLF and tabs. |
| `Lines() []Line` | Copied metadata: zero-based `Index`, original byte `Start/End` excluding terminators, expanded `Columns`, displayed `Number`. |
| `Tokens() []Token` | Copied metadata: zero-based `Index`, original byte `Start/End`, physical `Line/Column`, lexical `Category`, verbatim `Text`. Multiline raw strings and block comments retain internal CRLF. |
| `Glyphs() []Glyph` | Copied slice of ink-bearing source cells: `Rune`, byte `Offset`, `Line`, `Column`, `Token`, and live `Object` pointer. No number glyphs or whitespace. |
| `Line(index)` | Live line group; empty lines are stable empty groups. |
| `Token(index)` | Fresh slice of live glyph pointers; a multiline token can span line groups. |
| `LineNumber(index)` | Live number group; error if disabled or out of range. |
| `Background()` | Live rectangle or nil when disabled. |
| `TokenCopy(index)` | Independent group of copied token outlines with fresh identities. |
| `Copy()` | Independent `*Code` with copied paths, metadata, selections and fresh identities. |
| `Snapshot()` | Immutable private capture. `snapshot.Copy()` concurrently returns independent `*Code` instances. |

Line, token, number and copy methods return `(value, error)`; invalid indices
return `ErrSelection`. Animate live selections directly. Adding the same glyph
under a second scene root would violate tree ownership; use `TokenCopy` or
`Copy` when adding another root.

Authoring is single-threaded. Freeze a completed listing with `Snapshot` or
build a scene before concurrent sampling. A snapshot retains no live authoring
paths or mutable caller palette. Every copy/sample has independent paths and
child slices. The parsed font is immutable. Source edits after capture cannot
change that capture.

Keep the fixed child topology, root pointer and valid identities. Whole-object
affine transforms and valid path/style edits are allowed within budgets.
`Copy`, `TokenCopy` and `Snapshot` check topology and budgets before recursive
copying, rejecting cycles, replaced/reparented children, duplicate identities,
bitmap/clip additions and invalid visuals. Compatible `gomath.Transform` keeps
existing child references; metadata continues to describe the original source,
and content morphs do not retokenize it. A transform that adds children requires
a newly constructed `Code` for subsequent metadata-based authoring/copying.

## Bounds and errors

Start with `DefaultOptions`; the all-zero `Options{}` deliberately fails.
`Size` is finite in `[1e-6,1e3]` em; `LineHeight` in `[1,64]` em; `TabWidth` in
`[1,64]`; `Padding` in `[0,1e4]`. `FirstLineNumber: 0` defaults to 1, otherwise
it is in `[1,1e9]` (subsequent labels can exceed that starting-number bound).
All palette channels must be finite in `[0,1]`.

| `Limits` field | Hard maximum | Zero default |
| --- | ---: | ---: |
| `SourceBytes` | 262144 | hard maximum |
| `Runes` | 32768 | hard maximum |
| `Lines` | 2048 | hard maximum |
| `Columns` | 4096 per line, after tabs | hard maximum |
| `Tokens` | 16384 | hard maximum |
| `Depth` | 64 unmatched opening delimiters | hard maximum |
| `Glyphs` | 16384 source plus line-number ink cells | hard maximum |
| `Curves` | 1048576 total cubic segments, including background | 262144 |

Limits may only lower these maxima. Negative or excessive requested limits are
`ErrOptions`. Delimiter depth is a bounded lexical counter, not a recursive
parser or a claim of balanced/matching syntax. Local outer dimensions and all
outline control coordinates are bounded by `MaxCoordinate = 1e6`; copy/capture
also checks authored coordinates and aggregate curves/contours.

Invalid UTF-8, C0/C1 controls other than tab/LF/CRLF, Unicode format controls,
combining marks, and whitespace other than ordinary space/tab/newline are
rejected. This avoids silently hiding or reordering source in a one-rune grid.
It is not terminal-width, grapheme, bidi or complex-script layout. Supported
Unicode still requires a covered real glyph and valid Go lexical context.

Use `errors.Is` with `ErrLanguage`, `ErrOptions`, `ErrSource`, `ErrLimit`,
`ErrFont`, `ErrSelection`, `ErrObject`. Scanner errors also expose
`*SourceError` through `errors.As`, with original byte `Offset`, zero-based
physical `Line/Column`, and `Message`. Constructors return nil on error.

## Independent evidence and limits of the claim

`tests/reference/code_layout.py` uses independently specified specimens, its own
small Go specimen tokenizer, a manual rune/tab/CRLF grid, fontTools tables and
cubic outlines, and CairoSVG pixels. It never reads Go layout output. Six
fixtures cover Go tokens, real Greek glyphs, indentation, tab stops, blank and
trailing lines, CRLF inside raw strings/comments, `//line`, and gutter digit
transitions. Ten frames cover `Write` and different-content `Transform` at
0, 0.25, 0.5, 0.75 and 1, including changed glyph contour counts.

Tests compare every cubic point/contour, exact grid metrics and source spans,
glyph ink bounds, palette, SVG paths and raster pixels. The control-point
tolerance is one 2048-unit font design unit (under 0.04 reference pixel), allowing
sfnt's rounding of implied TrueType points. Metrics and ink bounds have a much
tighter `2e-8` tolerance. Pixel checks require RGB MAE <= 0.6 bytes, nonblack IoU
>= 0.965, and per-color silhouette IoU >= 0.94 (0.90 for under 100 reference
interior pixels). Tests use frozen files and need no Python/native libraries.
Concurrent snapshot construction/sampling and rejected mutations are covered.

The inspected Manim source is commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`,
`manim/mobject/text/code_mobject.py`. It informs capability scope, not an
end-to-end matched Manim/Pango/Pygments output claim. Python and other languages,
language guessing, file loading, semantic highlighting, named themes, complete
Pygments styles, Pango shaping and Manim window decorations remain unsupported.
`Write` here uses GoMath's existing per-path reveal contract.

The source/reference hash freeze and validation logs live in
`tests/data/code/manifest.json` and `tests/data/code/validation/`. Go Mono's
existing license is also copied beside the deliberately damaged test font.
Benchmarks separately measure construction and immutable copy for 100 code rows
plus a package header and final empty line (102 physical lines, 4118 bytes), and
a 1200×720 single-frame raster of 100 shorter code rows plus a header/final empty
line. Reference generation, file I/O, encoding and per-frame font/layout work are
excluded from the raster benchmark; construction includes its font/layout work.

To regenerate and re-freeze on a development machine with fontTools/CairoSVG:

```sh
python3 tests/reference/code_layout.py \
  --font /path/to/golang.org/x/image@v0.47.0/font/gofont/ttfs/Go-Mono.ttf \
  --freeze --manim /path/to/pinned/manim
```

Production and central test files were promoted unchanged from their staging
directories. The original agent manifest is retained in validation/agent-manifest.json;
the current manifest updates only this documentation hash after promotion. Validation overlays are
development artifacts in `validation/` and are excluded from frozen runtime and
reference inputs. The staged purecheck log uses an overlay that exposes the
candidate as `gomath/code` for `cmd/purecheck`'s ordinary package discovery.
