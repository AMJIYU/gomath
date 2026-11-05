# Horizontal text, explicit fallback and bounded markup

`Parse` copies a bounded OpenType font into an immutable `*Font`.
`DefaultFont` and `DefaultFontForStyle` return cached, real Go font faces.
`New` and `Layout` retain their existing single-font geometry, metrics and pixels.

For explicit fallback, supply an ordered list:

```go
fallback, err := text.Parse(fontBytes)
if err != nil {
    return err
}
object, metrics, err := text.LayoutWithFallback(
    "Aβ", []*text.Font{text.DefaultFont(), fallback}, text.DefaultOptions(),
)
```

`NewWithFallback` returns just the editable object and error. Both APIs accept
1–16 valid fonts, copy the list during the call, and retain no caller slice.
Font pointers can be shared concurrently; do not mutate the slice while a call
is copying it. Returned objects and paths belong to their individual call.

Each rune uses the first font with a nonzero glyph index. Empty outlines such
as spaces are valid coverage. Kerning applies only between adjacent glyphs
selected from the same font pointer. An actual selected glyph that fails to
load is an error, even if another font covers the character.

All fonts share a baseline and normalized em at `Options.Size`. Returned
ascent/descent are maxima across **all configured fonts**, including unused
fonts and empty strings. Newline baselines step by `Size*LineHeight`; a caller
can choose a line height that overlaps taller ink. Tabs remain one space,
empty lines count, and width includes leading/trailing whitespace. Metrics are
font line/advance metrics; use object bounds for ink bounds.

Missing characters fail by default. With `AllowMissing`, all fonts are searched
first, then the primary font's `.notdef` is explicitly used. There is no automatic
system-font discovery, shaping, bidi, grapheme selection, combining-mark
positioning, ligature substitution, or synthesized font style. This API does
not make arbitrary Unicode text correctly shaped.

Bounds: `MaxFallbackFonts=16`, `MaxRunes=65536`,
`MaxGlyphSegments=1048576` for the combined run, and `MaxFontBytes=64 MiB`
per parsed font. Size and line height must each be finite, positive and at most
`1e6`, with non-underflowing layout scale/line step. Colors use existing strict
style validation. Multiple-font calls preflight selected outlines against the
shared segment cap before allocating the output glyph tree.

For a strict, bounded markup fragment, use `DefaultMarkupOptions` with
`NewMarkup` or `LayoutMarkup`:

```go
opt := text.DefaultMarkupOptions()
object, metrics, err := text.LayoutMarkup(
    `A<b>V<i>g</i></b><span foreground="#20c080">β&amp;x</span>`+"\n"+`<i>g</i>`,
    opt,
)
```

Only `<b>`, `<i>` and `<span foreground="#RRGGBB">` are supported. Tags may nest;
bold plus italic selects the actual BoldItalic face. XML entities are decoded,
and actual newlines (or `&#10;`) make new lines. Unknown tags/attributes,
namespaces, malformed nesting/entities/colors, DTD, comments, CDATA and
processing instructions fail. Use escaped text for literal XML punctuation.
No whitespace collapsing, HTML recovery or implicit markup detection occurs;
`New`, `Layout` and fallback APIs still treat their input as ordinary text.

`MarkupOptions` embeds `Options`. Its optional `Fonts *MarkupFonts` maps
`Regular`, `Bold`, `Italic`, `BoldItalic` to explicit immutable fonts. Nil Fonts
uses the four embedded real faces. A custom table requires Regular, validates
every nonnil face, and errors if a tag requests an absent variant (even an empty
tag). There is no implicit fallback between style faces. `AllowMissing` uses
the selected style font's `.notdef`. The table is copied and never retained;
do not mutate it during a call.

Colors restore at closing tags; spans set opaque RGB and accept no alpha syntax.
Color-only changes retain kerning in the same font pointer; actual font changes
and newlines break adjacency. Empty tags insert no glyph boundary. Metrics use
the maximum ascent/descent across configured faces, including unused ones;
baseline intervals, tab/space advances and ink bounds follow the text rules above.

Markup bounds are 1 MiB source bytes, 32 nested elements, 16384 total elements,
four named font slots, 65536 decoded runes and 1048576 combined glyph segments.
The limits include empty/repeated elements and whitespace/newline runes. Segment
preflight covers all styles/colors/lines before constructing a glyph tree.
This is a finite per-rune subset, without Pango shaping or full Manim markup
parity. See `docs/TEXT_MARKUP.md` for exact XML behavior and independent evidence.

Tests and benchmarks live under `tests/` and use the central runner:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -tags noasm -vet=all -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=text -tags noasm -race -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -tags noasm -run '^$' -bench '^BenchmarkFallbackLayout$' -benchmem -benchtime=300ms -count=3 -p 1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=text -tags noasm -run '^$' -bench '^BenchmarkMarkupLayout$' -benchmem -benchtime=300ms -count=3 -p 1
```

Go's race detector requires CGO during test instrumentation; product builds and
normal tests use CGO disabled. Neither fallback layout nor rendering invokes a
process or native font library. See `docs/FONT_FALLBACK.md` for fixture provenance,
independent validation, exact limitations and measurements.
