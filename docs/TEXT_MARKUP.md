# Bounded native text markup

`text.NewMarkup(source, MarkupOptions)` returns `(*gomath.Object, error)`.
`text.LayoutMarkup` also returns `text.Metrics`, with the same result order as
`Layout`. `DefaultMarkupOptions` embeds the existing default `Options`; fonts
default to the four cached, real embedded Go faces. Existing `New`, `Layout`,
`NewWithFallback`, and `LayoutWithFallback` remain ordinary-text APIs. They do
not inspect markup, and their old geometry, metrics and pixel fixtures pass.

```go
opt := text.DefaultMarkupOptions()
opt.Size = .7
object, metrics, err := text.LayoutMarkup(
    `<b>Bold <i>and italic</i></b> <span foreground="#20c080">α &amp; β</span>`+
        "\n"+`regular again`, opt,
)
```

## Supported grammar and scopes

Input is an XML text **fragment**, so plain text and multiple top-level elements
can be adjacent without an artificial root. Supported tags are exactly:

| Syntax | Behavior |
| --- | --- |
| `<b>text</b>` | Add the bold style bit. |
| `<i>text</i>` | Add the italic style bit. |
| `<span foreground="#RRGGBB">text</span>` | Set opaque sRGB, restoring the enclosing color on close. |
| `&lt; &gt; &amp; &quot; &apos;` | The five XML named entities. |
| `&#65;`, `&#x41;` | Decimal/hex numeric references to legal XML Unicode characters. |
| Actual LF or `&#10;` | A new baseline/line; empty lines are retained. |

`<b><i>...</i></b>` and `<i><b>...</b></i>` select BoldItalic. Repeated bold or
italic tags are idempotent; closing any tag restores the complete prior font and
color scope. Self-closing supported tags are legal, request their style variant,
and add no glyphs or adjacency boundary. An empty color span cannot suppress
kerning between surrounding glyphs.

Only `span` accepts an attribute, and it requires **exactly** one `foreground`.
Its XML-decoded value must be a `#` and six ASCII hex digits, case insensitive;
named colors, abbreviated hex, alpha and whitespace around the value fail.
XML quoting rules apply, including single or double quotes and numeric escapes
inside a valid attribute value. Spans set alpha to 1; the base `Options.Color`
can still use the existing valid RGBA color API outside a span.

Tags/attributes are case sensitive. Unknown tags (`u`, `br`, `strong`, `markup`,
etc.), attributes (`size`, `font`, `color`, etc.), duplicate attributes, namespace
declarations/prefixes, malformed or crossed tags, DTD, comments, CDATA and
processing instructions all fail. No visible content is silently stripped.
Raw XML text must escape `<`, `&` and the forbidden `]]>` sequence; encoded
`]]&gt;` is valid text. Entity decoding is once only: `&amp;nbsp;` means the literal
characters `&nbsp;`, not a second entity expansion. Unknown entity names fail.
Numeric references to surrogates, out-of-range scalars, XML-forbidden controls,
U+FFFE and U+FFFF fail explicitly; the decoder cannot silently replace them with
U+FFFD. Invalid UTF-8 also fails.

Literal CR/CRLF are normalized to LF by XML rules. A numeric `&#13;` stays a CR
character and has the same ordinary-text coverage/AllowMissing behavior as CR
passed to `Layout`; it is not a line break. Spaces are preserved. Tabs retain
the existing one-space behavior and have no tab-stop alignment. The returned
group Name is the decoded text, with markup removed only after validation.

## Explicit fonts, kerning and measurements

For custom faces, set `MarkupOptions.Fonts` to a `*text.MarkupFonts` table with
`Regular`, `Bold`, `Italic`, `BoldItalic` fields. Regular is required; every
nonnil entry is validated. A style requested by a tag must be configured,
including intermediate styles and empty tags. For example, `<b><i>A</i></b>`
requests Bold and BoldItalic. Missing variants fail rather than substituting
embedded fonts, thickening strokes, or shearing outlines. Callers explicitly
choose the font assigned to each field; aliases are allowed.

There is no font fallback between style slots in this API. A selected font
without the character fails by default, even if another slot covers it.
`AllowMissing` explicitly selects that style font's glyph zero `.notdef`.
Actual malformed/unsupported selected outlines always fail. To lay out ordinary
text with ordered font fallback, use the separate fallback API.

Adjacent glyphs kern only when the selected `*Font` pointer stays the same.
Changing color or crossing element/entity boundaries does not end kerning.
Changing to another font or moving to a new line does end it; returning to a
previous font does not kern across the intervening glyph. Explicit style aliases
to the same pointer retain same-font kerning. This uses the font engine's real
supported kern tables, without synthetic tracking or inferred adjustment.

Faces share the same current baseline and normalized em at `Options.Size`.
Ascent/descent are the maxima of **all configured nonnil faces**, including
unused faces and empty input. Default Go faces have the same line metrics, so
unstyled default markup agrees exactly with ordinary regular layout. A taller
custom unused variant can deliberately enlarge the returned line box.

Width is the widest line's accumulated advances and kerning, including leading
and trailing spaces. Height is ascent plus descent plus
`(Lines-1)*Size*LineHeight`. The baseline interval is always `Size*LineHeight`;
there is no automatic expansion for tall fallback/style ink. Overhangs can
extend beyond advance width and tall glyphs can overlap at a small requested
line height. Use `object.Bounds()` for ink bounds. Glyph outlines and fills are
normal editable objects and work with existing raster/SVG and object-copy APIs.

The font table is copied during the call and never retained. Parsed fonts are
immutable; do not mutate the table while a call is snapshotting it. Per-call XML
state, style stack, glyph cache, sfnt buffer and returned paths are independent.
Completed fonts/objects can be read concurrently; mutable authoring still needs
caller synchronization.

## Resource limits and exclusions

| Limit | Value / accounting |
| --- | --- |
| `MaxMarkupBytes` | 1048576 source bytes, including tags and entity spellings. |
| `MaxMarkupDepth` | 32 open supported elements, without a virtual fragment root. |
| `MaxMarkupElements` | 16384 start elements, including empty/repeated elements. |
| `MaxMarkupFonts` | Four fixed named slots; default/custom fonts use the same bound. |
| `MaxRunes` | 65536 decoded runes, including tabs/newlines/spaces. |
| `MaxGlyphSegments` | 1048576 selected sfnt segments summed across all styles/colors/lines. |
| `MaxFontBytes` | 64 MiB per parsed font, unchanged. |

Size and line height must each be finite, positive and at most `1e6`; underflowing
coordinate scale or baseline step fails. The existing strict color validation
also applies to the base color, even if spans override all visible glyphs.
Glyph coverage/segment preflight finishes before allocating an output glyph
tree and never resets the work count between runs. Errors return a nil object
and zero metrics; no partial output is published.

This subset has no shaping, bidi, script itemization, font discovery, ligature
substitution, combining-mark positioning, fallback grapheme selection, automatic
wrapping/justification, HTML recovery, decorations, size/language/font attributes,
or full Pango/Manim markup parity. Glyph coverage alone is not a complex-script
typography guarantee. Product code uses only the existing pure-Go font/path
engine and standard XML parsing, with no browser, Pango, native library or
process launch.

## Independent evidence

The read-only Manim reference was inspected at
`16ea443e2a5f1d79db0a721161411ce916d6e733`,
`manim/mobject/text/text_mobject.py:MarkupText`. That implementation delegates a
broader markup/shaping system to Pango; inspecting it is not a pixel-parity test.

`tests/reference/text_markup.py` instead specifies decoded styled operands
manually, reads pinned original Go Regular/Bold/Italic/BoldItalic tables with
fontTools, and independently evaluates hmtx/kern/hhea, outlines and quadratic
bounds. A separate kerning case reads the existing licensed complementary test
font derivatives, whose fixture-only kern/line metrics are explicit. CairoSVG
renders those original outlines at manual coordinates. No Go output is used to
derive these oracles. Sources, SHA-256 hashes, tool versions, placement records
and image hashes are in `tests/data/text/markup/reference.json`; Go font license
notices are alongside the data.

Four independent cases cover 32 ink glyphs: nested four-face scopes, inherited
colors and Unicode, genuine same-font/cross-color kerning and font switches,
XML entities and multiline whitespace. All advance/line metrics agree within
`1e-12`; analytical ink boxes agree within one source design unit (1/2048 em).
Minimum per-color Cairo silhouette IoU is 0.992459; maximum whole-image RGB MAE
is 0.033145 byte levels. Curve/raster edge differences are expected between
independent engines; this is not a general pixel-equivalence claim.

Actual Go review PNG/SVG are `tests/data/text/markup/preview.png` and `.svg`.
They are generated by `tests/reference/_text_markup_preview` and are not used
as independent correctness oracles. The independent reference regeneration is
development-only; Go tests/runtime never require those external tools:

```sh
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib:/opt/homebrew/opt/cairo/lib PYTHONDONTWRITEBYTECODE=1 python3 tests/reference/text_markup.py --fonts /path/to/x/image/font/gofont/ttfs
CGO_ENABLED=0 go run ./tests/reference/_text_markup_preview
```

## Checks and benchmark

All test sources, benchmarks, fixtures and reference tools live under `tests/`.
Central tests include invalid/unsupported markup, inclusive/exceeded byte/depth/
element/rune caps, shared multi-style glyph budget, real style coverage and
missing variants, malformed outlines, color and font boundaries, baseline/line
metrics, whitespace, independent bounds/raster/SVG, ownership and concurrent
layout/copy/render reads. The complete text suite retains the legacy exact
control-point/style/metric/pixel snapshots and all fallback references.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -tags noasm -vet=all -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=text -tags noasm -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./text
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags noasm ./text
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -tags noasm ./text
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -tags noasm -run '^$' -bench '^BenchmarkMarkupLayout$' -benchmem -benchtime=300ms -count=3 -p 1
```

CGO0/noasm text tests, central vet, explicit vet, race, purecheck42 and Linux/
Windows text builds pass. Go's race instrumentation requires CGO enabled for
that test command; product code has no native/process dependency.

Benchmark: Go 1.27.3, darwin/arm64, Apple M2, GOMAXPROCS 8, `-p 1`, 300 ms per
sample, three samples. Each call parses 71 source bytes, decodes 11 runes, and
lays out nine ink glyphs over two lines and all four real font faces, including
a color span, entity, preflight and editable output allocation. Samples were
24.027, 22.615 and 22.978 µs/op, each 80168 B/op and 220 allocs/op. Embedded font
initialization occurs before timing. Rasterization, image size, frame count,
file I/O and independent tools are excluded; independent raster cases each use
one 1024×640 frame. No old benchmark was repeated and no general speedup is
claimed.
