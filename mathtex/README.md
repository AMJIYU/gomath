# Bounded mathematical TeX subset

`gomath/mathtex` parses and lays out a limited math notation subset in pure Go.
Base characters use the real outlines selected by `gomath/text`; a finite set
of additional mathematical symbols uses bundled source-font outlines. Decorations
are vector paths. No external LaTeX, subprocess, cgo or system font service is
required. Unsupported commands and malformed input return errors with rune
positions, without a partial object. This is not a complete TeX engine or a
claim of TeX/Manim layout parity.

## Multiline aligned and gathered mathematics

`New(source, Options)` also accepts `\begin{aligned}...\end{aligned}` with
an equal number of explicit right/left column pairs on each row, and
`\begin{gathered}...\end{gathered}` with centered single-column rows. Both use
real existing math boxes and glyphs, full advances, expanded baseline boxes,
per-cell local display mode and scoped colors/styles. Bounds are shared with
other environments/substacks. Ragged/odd alignment structure, blank rows,
trailing row separators and optional positioning/row spacing are errors.

See [the finite multiline contract](../docs/MULTILINE_MATH.md) for exact spacing,
baseline/style semantics, limits, independent source-font/Cairo references,
validation and benchmark workloads. These additions do not change existing
matrix/cases geometry or implement full amsmath/Manim equivalence.

## Scoped solid math colors

The existing `New(source, Options)` API now accepts a declaration and a braced
math-group command, with no new public options:

```tex
\color{red}x+{\color{blue}y}+z
\textcolor{violet}{\dbinom{\mathbb{N}}{\mathcal{A}}}
\color{red}\dfrac{\textcolor{blue}{\hat{n}}}{\textcolor[HTML]{008040}{g}}
\color{orange}\displaystyle\sum_{\substack{\color{cyan}i=1\\j\in\mathbb{N}}}^{n}\frac{1}{i}
```

`\color{name}` affects subsequent atoms in the current math row, without
creating a drawable, advance or extra inter-atom gap. `\textcolor{name}{math}`
parses its second argument as the supported math subset in a local color scope;
it does not turn that argument into literal text. Both also accept the explicit
syntax `[HTML]{RRGGBB}`, with exactly six ASCII hex digits, either case, no `#`
and no alpha. The only accepted model name is uppercase `HTML`. Surrounding
ASCII spaces within each color/model token are trimmed; interior spaces remain
invalid. Each delimited token, including its spaces, is limited to 16 runes.

The following **case-sensitive lowercase** names select opaque, straight sRGB
components. These are finite basic TeX/xcolor-style RGB values, deliberately
distinct from GoMath's animation palette such as `gomath.Red` or `gomath.Blue`:

| Name | RGB components in [0,1] | Name | RGB components in [0,1] |
| --- | --- | --- | --- |
| black | 0, 0, 0 | blue | 0, 0, 1 |
| brown | .75, .5, .25 | cyan | 0, 1, 1 |
| darkgray | .25, .25, .25 | gray | .5, .5, .5 |
| green | 0, 1, 0 | lightgray | .75, .75, .75 |
| lime | .75, 1, 0 | magenta | 1, 0, 1 |
| olive | .5, .5, 0 | orange | 1, .5, 0 |
| pink | 1, .75, .75 | purple | .75, 0, .25 |
| red | 1, 0, 0 | teal | 0, .5, .5 |
| violet | .5, 0, .5 | white | 1, 1, 1 |
| yellow | 1, 1, 0 | | |

Color state is private to each layout. It restores at every existing group,
font/alphabet group, `\left...\right` body, environment cell, substack row,
script argument, fraction operand and explicit-stack argument boundary,
including error returns. An inner declaration cannot affect a sibling operand,
next matrix cell/row or following outer atom. A declaration before a composite
command is inherited by its contents; nested overrides select their own colors.
The original `Options.Color`, including any valid alpha supplied through that
existing API, is restored on leaving a colored scope. Command colors themselves
always have alpha 1. Empty `textcolor` groups and trailing declarations are legal
and create no ink; the existing nonempty-body rules of accents/substacks remain.

All actual glyph fills inherit color, including real styled fonts, upright
operators, mathematical alphabets, supplemental symbols and literal-text
commands. Fraction/radical strokes, binomial/environment/explicit delimiter
strokes and accent fills use the color at their structural command's entry.
For example, `\color{red}\hat{\textcolor{blue}{x}}` has a red hat over blue x;
`\textcolor{blue}{\hat{x}}` colors both blue. A color declaration inside one
fraction operand does not recolor the fraction rule, and a declaration inside
a delimited body does not recolor either surrounding delimiter.

Scripts attached outside a closed `textcolor` group use the restored color:
`\textcolor{red}{x}_i` has red x and the original-color i, while
`\textcolor{red}{x_i}` colors both. A declaration requires a row, so brace it
inside an otherwise unbraced script/accent argument; `x^\color{red}n` is an
error. A `textcolor` group is an ordinary group rather than a new operator atom:
put `\limits`/`\nolimits` directly after the operator **inside** the group,
as in `\textcolor{red}{\sum\limits_i^n}`. This keeps existing limits eligibility
and script geometry unchanged.

Unknown names, case variants, color mixes (`red!50!blue`), CSS syntax, eight-digit
hex/alpha, other models (`rgb`, `RGB`, `gray`, `cmyk`, `named`), nested tokens,
macro expansion and unlisted color commands are errors. No custom color
definitions, gradients, opacity commands or full xcolor semantics are provided.
Color adds no outlines and cannot bypass the shared source, nesting, cell or
curve budgets. Existing font coverage/style errors remain strict. Uncolored
formulas retain the same metrics, paths, styles and legacy fixtures.

Colors reside in ordinary descendant `Style` fills/strokes. Native raster and
SVG paint those values directly; SVG retains paths without images/external
fonts. Object copies own their styles, and the existing path morph interpolates
the descendant solid colors at intermediate times. Tests cover independent
copies, concurrent layouts, concurrent reads/rendering of a frozen tree and
concurrent sampling after scene authoring is complete.

Two independent references in `tests/data/mathtex/colors/` are manually assembled
SVG fraction/overline layouts using the stated spacing policy and original
GoRegular source-font outlines/hmtx metrics from fontTools 4.45.1. CairoSVG 2.7.1
and Cairo 1.18.4 render those SVGs directly at 512×512, 204.8 px/em. The generator
does not import GoMath or read its output; it records source-font and artifact
hashes. The Go test verifies the embedded TTF hash, each SVG/PNG hash, per-color
interior-mask IoU ≥0.95 (RGB channels within two bytes) and whole-image RGB mean
absolute error ≤1 byte. Measured minimum IoU is **0.995070**; the fraction and
overline MAE are **0.008149** and **0.004157** bytes. These selected analytic
layouts and paint checks do not establish general TeX/xcolor parity.

Development-only regeneration (Python 3.11.5; Cairo's library must be discoverable
by the local development loader) and native preview commands:

```sh
python3 tests/reference/mathtex_colors.py --font /path/to/golang.org/x/image/font/gofont/ttfs/Go-Regular.ttf
GOMATH_TYPESET_PREVIEW_DIR=/absolute/fresh-preview CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -run '^TestMathColorRasterAndSVGDescendants$' -count=1
```

The separate native preview is `tests/data/mathtex/math-colors.png` / `.svg`, one
960×480 frame. Go tests/runtime only consume fixed references and have no
Python/Cairo/fontTools or other external-runtime dependency. The source font
retains its existing [license](../LICENSES/go-fonts.txt).

On 2026-11-04 the complete central mathtex suite passed in 0.927 s with **96.1%**
statement coverage, and its race run passed in 18.748 s. All eight exact legacy
fixtures passed. Package vet, the full production `CGO_ENABLED=0`/`noasm` build
and purecheck also passed (42 non-standard packages, no native inputs or runtime
process-launch imports):

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go vet -tags noasm ./mathtex
CGO_ENABLED=0 go build -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

Only the new color workload was benchmarked, using `-p 1`, three 150 ms samples,
Apple M2, darwin/arm64, Go 1.27.3 (module Go 1.26.0), GOMAXPROCS=8, CGO_ENABLED=0
and `noasm`. Both variants lay out the same sum/substack, nested fractions,
accent, mathematical alphabet and binomial geometry; the uncolored variant
retains the same braced groups in place of `textcolor` wrappers.

| New workload | ns/op, three samples | B/op, three samples | allocs/op |
| --- | --- | --- | --- |
| uncolored | 111040 / 113208 / 109232 | 248327 / 248324 / 248330 | 684 |
| colored | 109668 / 110012 / 110216 | 248774 / 248776 / 248773 | 697 |

The overlapping timing ranges are not evidence that adding color improves
performance. This measures parsing, vector layout and validation of one formula,
excluding rasterization, SVG export, animation and independent reference tools;
the benchmark has no image size/frame count. The separate preview above is one
960×480 frame. The exact colored workload and reproduction command are:

```tex
\color{red}\displaystyle\sum_{\substack{\color{blue}i=1\\j\in\mathbb{N}}}^n\textcolor[HTML]{008040}{\dfrac{\hat{n}}{\tfrac{g}{k}}}+\textcolor{violet}{\dbinom{n}{k}}
```

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkMathColorLayout$' -benchmem -benchtime=150ms -count=3
```

## Explicit fractions, binomials and multiline scripts

The existing `New(source, Options)` API accepts two braced operands for each
fraction/binomial command. The explicit sizing commands are now real distinct
layouts, replacing the earlier `dfrac`/`tfrac` aliases. Ordinary `\frac` retains
its exact previous 0.8 operand scale and geometry, including inside display rows.

| Command | Fraction em | Operand size | Visible structure |
| --- | --- | --- | --- |
| `\dfrac{a}{b}` | Nominal `Options.Size`. | 1.0 × fraction em, text mode. | Real stroked fraction rule. |
| `\tfrac{a}{b}` | Nominal `Options.Size`. | 0.7 × fraction em, text mode. | Real stroked fraction rule. |
| `\binom{n}{k}` | Current local size, or nominal em after an explicit display/text declaration. | 1.0 in display mode; 0.7 otherwise. | No rule; existing vector delimiter paths. |
| `\dbinom{n}{k}` | Nominal `Options.Size`. | 1.0 × fraction em, text mode. | No rule; existing vector delimiter paths. |
| `\tbinom{n}{k}` | Nominal `Options.Size`. | 0.7 × fraction em, text mode. | No rule; existing vector delimiter paths. |

```tex
\dfrac{g}{n}+\tfrac{g}{n}
\dbinom{n}{k}+\tbinom{n}{k}
{\displaystyle\binom{n}{k}}+\binom{n}{k}
x^{\binom{n}{k}}+x^{\dbinom{n}{k}}
\displaystyle\sum_{\substack{i=1\\j\in\mathbb{N}}}^{n}\dfrac{\binom{n}{i}}{1+i}
```

Forced `dfrac`/`tfrac`/`dbinom`/`tbinom` sizing restores nominal em even inside
scripts, nested fractions and annotations; it is not a scaled alias of the
surrounding small fraction. Automatic `binom` inside a script follows the
existing 0.65 local script scale. An explicit `\displaystyle` or `\textstyle`
inside that script selects nominal em for subsequent binomials in the row.
These declarations restore at existing group/delimiter/font/environment-cell
boundaries. They do not retroactively resize an earlier atom or change the
legacy sizing of ordinary glyphs and `\frac` in that row.

Both operands start in text mode, independently of the enclosing display mode,
so their sum/product operators use side scripts unless explicitly overridden.
A declaration inside one operand does not leak into its sibling or following
atoms. Real font styles, mathematical alphabets and color are inherited normally.
Both arguments may be empty/spacing-only, as with the existing `\frac`.

The body's baseline stays y=0; its math axis is 0.16 fraction em. Operands are
centered by their full advances, with 0.25 em total horizontal padding. Ruled
fractions use a 0.045 em stroke. Their visible rule edges have 0.24 em clearance
from the operand baseline boxes in display sizing, or 0.14 em in text sizing.
Numerator depth and denominator height are included, so descenders, accents,
inner fractions and multiline operands cannot collide with the rule. Full
width/ascent/descent feed outer scripts, accents, matrices and following atoms.

Binomials omit the rule and its half-stroke contribution to clearance. Their
parentheses reuse `delimiterBox`/`enclose` and the existing two-cubic stroked
curves, not ordinary font parentheses enlarged to fit. They clear the complete
operand box, retain em-based stroke thickness and contribute their width and
vertical extent to layout. The new rules and delimiter curves are charged to
`MaxLayoutCurves` exactly once, alongside their real operand glyphs.

`\substack{row\\row...}` is a single centered column, usable inside a side
script, operator limit, fraction or other supported math group. Each row uses
`min(current local size, 0.65*Options.Size)`, so a normal subscript is not reduced
twice; deeper scripts can still be smaller. Rows center by full advance and
retain baseline struts of at least 0.7 row-em ascent and 0.2 row-em descent.
There is a 0.2 row-em gap between adjacent full row boxes. The composite box is
centered about its own 0.16 row-em math axis, with baseline y=0, and contributes
all its height/depth when attached below an operator. It creates editable glyph
paths rather than an image or newline in an ordinary text glyph run.

Each substack starts in text mode; declarations within one row restore before
the next row and after the stack. Nesting, glyph styles, named operators,
alphabets, fractions and even nested environments are allowed in a row. At most
`MaxSubstackRows` (64) visible, nonempty rows are accepted. Empty/spacing-only
rows, including trailing row separators, are errors. `&` column alignment,
`\\[dimension]` row spacing and unlisted structural commands are rejected.
Every row shares the existing 4,096 environment-cell budget across the whole
formula, in addition to source, depth and curve limits. No extra TeX parsing,
macro expansion or external runtime is introduced.

This remains a bounded subset: the stated axis/spacing policy is not a font
MATH-table or TeX-eight-style implementation. Repeated automatic scripts keep
the existing 0.65 scaling rather than introducing TeX script/scriptscript font
tables. `\genfrac`, infix `\over`/`\choose`, `\cfrac`, arbitrary delimiters/rule
widths and optional spacing remain errors. No complete TeX/AMS or Manim layout
parity is claimed.

Independent references under `tests/data/mathtex/fractions/` cover n/g/k in all
four real embedded Go faces at operand scales 1.0, 0.7, 0.65 and 0.455: 48 source
hmtx/glyf metric records and 48 direct unhinted FreeType masks. Source-font hashes
are checked against the exact embedded TTF bytes. Glyph dimensions allow one
1/2048-em design unit for native quadratic midpoint rounding; derived baseline
box metrics allow two. Mask silhouette IoU must be at least 0.95, with unmatched
ink within two pixels; the measured minimum is **0.979035**. Mask comparison
normalizes individual operands to the independent source font origin. Formula
placement is checked separately against font measurements and analytic geometry.
These are not whole-formula TeX goldens. No usable external TeX engine was
available for this slice, and no external tool is required by Go tests/runtime.
The source Go fonts retain their existing [license](../LICENSES/go-fonts.txt).

The development extractor records source hashes and tool versions (fontTools
4.45.1, FreeType 2.13.2, Pillow 11.2.1):

```sh
python3 tests/reference/mathtex_fractions.py --gofont-dir /path/to/golang.org/x/image/font/gofont
```

`tests/data/mathtex/fractions-binomials.png` / `.svg` is a separate native
960×480 preview, with actual constituent-pixel, PNG roundtrip and vector-only
SVG tests. Regenerate in a fresh absolute directory:

```sh
GOMATH_TYPESET_PREVIEW_DIR=/absolute/fresh-preview CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -run '^TestTypesetPNGAndVectorExamples/fractions-binomials$' -count=1
```

Slice validation on 2026-11-04 passed the complete central mathtex suite
(95.9% statement coverage), its race run, package vet and the production build.
All eight exact legacy layout fixtures still pass. The suite includes baseline,
bounding-box, nesting/style scope, shared budget boundaries, ownership/concurrency,
independent font masks and constituent-pixel rendering checks:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go vet -tags noasm ./mathtex
CGO_ENABLED=0 go build -tags noasm ./...
```

Only the new layout workload was benchmarked, serially with `-p 1`, on an
Apple M2, darwin/arm64, Go 1.27.3 (module Go 1.26.0), GOMAXPROCS=8,
CGO_ENABLED=0 and `noasm`. Three 150 ms samples of
`BenchmarkGeneralFractionsAndSubstack` measured **133294 / 135750 / 136219 ns/op**,
**285030 / 285032 / 285028 B/op**, and **794 allocs/op** in every sample. Its
formula combines a display sum with two substack rows, nested display/text
fractions, display/text binomials, real mathematical alphabets and an accent:

```tex
\displaystyle\sum_{\substack{i=1\\j\in\mathbb{N}}}^{n}\dfrac{\dbinom{n}{i}}{\tfrac{g}{k}}+\tbinom{\mathcal{A}}{\hat{n}}
```

The benchmark measures parsing, vector layout and validation of one formula;
there is no image size/frame count for that workload. It excludes rasterization,
export, animation and external reference tools. The separate preview is one
960×480 frame. Reproduce the new benchmark with:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkGeneralFractionsAndSubstack$' -benchmem -benchtime=150ms -count=3
```

## Real mathematical alphabets

The existing `New(source, Options)` API accepts three new **braced math groups**:

| Command | Supported characters | Actual source family |
| --- | --- | --- |
| `\mathbb{...}` | Latin A–Z, a–z and ASCII 0–9, 62 glyphs. | Double-struck. |
| `\mathcal{...}` | Latin A–Z and a–z, 52 glyphs. | Mathematical Script. |
| `\mathfrak{...}` | Latin A–Z and a–z, 52 glyphs. | Mathematical Fraktur. |

```tex
x\in\mathbb{R}+\mathbb{C}^{2}+\mathbb{0123456789}
\mathcal{L}_{g}+\mathfrak{g}
\displaystyle\operatorname*{arg max}_{x\in\mathbb{R}}\overset{\mathcal{A}}{\mathfrak{f}}(x)
```

These are independent, real CFF outlines and advances from the already-pinned
**STIXGeneral 1.1.0 Regular** source, not ordinary Latin made bold, duplicated,
outlined or slanted. The derivative subset is named **Gomath Math Alphabets**.
No new runtime font file, system font lookup or dependency is introduced.
The existing [LICENSE-STIX.txt](LICENSE-STIX.txt) covers this subset and the
small renamed custom-font test fixture under SIL OFL 1.1. Source provenance:

```text
STIXGeneral.otf SHA256 5add3f3f2bd7fd897d2fa5ccbe468607c52111dc44cdfaaf2d851a574f5357a7
```

Unicode's Letterlike exceptions are mapped explicitly, rather than indexing
through reserved holes in the Mathematical Alphanumeric Symbols block:

| Family | Input characters | Mapped Letterlike glyphs |
| --- | --- | --- |
| Double-struck | C H N P Q R Z | ℂ ℍ ℕ ℙ ℚ ℝ ℤ |
| Script | B E F H I L R M e g o | ℬ ℰ ℱ ℋ ℐ ℒ ℛ ℳ ℯ ℊ ℴ |
| Fraktur | C H I R Z | ℭ ℌ ℑ ℜ ℨ |

The 166 corresponding encoded Unicode glyphs are also accepted directly as
math atoms. They select their encoded family even inside a different alphabet
group; their leaf object names are the actual mapped Unicode characters.
They do not become literal-text or operator-name fallbacks. Reserved Unicode
holes, unlisted alphabet variants and missing glyphs remain errors.

Alphabet commands select their own face and restore the enclosing face when
their group ends. The inner font command wins: `\mathbf{\mathbb{R}}` selects
double-struck R; `\mathbb{\mathbf{R}}` selects existing real bold Latin R.
The same scope rule applies to `\mathrm`, `\mathit`, `\boldsymbol` and nested
alphabet commands. Encoded Unicode characters already specify their family;
an enclosing bold/italic context does not synthesize another version of that
encoded glyph. Literal text, named operators, `\sin` and `\lim` retain their
existing explicit font-selection behavior, then alphabet selection resumes.

The groups parse the supported math subset, so nested fractions, scripts,
accents, delimiters, matrices, symbols, operators and stacks remain available.
Recognized punctuation and mathematical symbols retain their ordinary glyphs.
Other letters, numbers and combining marks are rejected when no character in
the selected alphabet is defined: for example `\mathbb{\alpha}`,
`\mathcal{é}`, `\mathfrak{Ж}`, `\mathcal{1}` and `\mathfrak{0}` are errors.
In particular `\mathcal{A_i^2}` fails on the digit: use `\mathcal{A}_i^2` for
ordinary external scripts, or explicitly select a different inner face.
Empty groups follow the existing font-command behavior and are legal.

Baseline y=0, original 1000-unit/em coordinates, hmtx advances and actual cubic
extrema determine the boxes. Negative left bearings are shifted into the
advance box using the existing math glyph rule; descenders contribute to depth.
Scripts use the existing 0.65 scale, inheriting the alphabet while inside its
group. An outer script after a closed alphabet group uses the restored face.
Color/fill, source/depth limits and the shared `MaxLayoutCurves` budget apply
normally. Every layout owns its mutable contours; immutable source tables are
shared. Default non-alphabet formulas retain their exact prior geometry.

Custom fonts remain strict, consistently with supplemental set symbols.
`Options.Font` must contain the mapped Unicode glyph itself, not just its ASCII
input. A selected custom style variant must also supply its own encoded glyph.
No bundled outline silently replaces a missing custom glyph. A command selecting
its own alphabet selects the regular custom face; a subsequent explicit face
command uses the existing custom-variant rules. There is no placeholder glyph.

This is a finite mathematical alphabet subset, not TeX alphabet-package parity.
It does not implement font MATH-table italic corrections/kerning/anchors,
automatic Unicode normalization, Greek double-struck alphabets, bold Script/
Fraktur variants, `\mathscr` or arbitrary font commands. Script here identifies
the specified source font's real Script design, not every LaTeX package's
potentially different calligraphic alphabet.

Independent references under `tests/data/mathtex/alphabets/` contain 166
source-font hmtx/bbox records and 166 masks. fontTools 4.45.1 reads original CFF
outlines; FreeType 2.13.2 directly rasterizes the original font at 256 ppem with
`FT_LOAD_RENDER | FT_LOAD_NO_HINTING | FT_LOAD_NO_BITMAP`. Disabling hinting
matches the runtime vector geometry model rather than testing a size-specific
grid-fit design. Pillow 11.2.1 writes the PNGs; it does not render the glyphs.
Metrics agree within 2e-12 em. Binary silhouette IoU must be at least 0.95 and
every unmatched ink pixel must be within two pixels of the other silhouette;
the measured minimum IoU is **0.986187**. These references establish glyph
geometry/rendering, not independent TeX layout behavior.

The development-only extractor pins the source hash and records Unicode/tool
versions, font metrics and the hash of the three-glyph custom test font:

```sh
python3 tests/reference/mathtex_alphabets.py --font /path/to/STIXGeneral.otf
gofmt -w mathtex/math_alphabets_data.go
```

The 169 generated outline/reference/font files reproduced byte-for-byte with
the recorded source and tool versions; the custom fixture retains the source
timestamps rather than embedding extraction time.

Python, fontTools, freetype-py, FreeType and the original font are not Go-test or
runtime dependencies. The formula preview `tests/data/mathtex/math-alphabets.png`
and `.svg` is generated by GoMath, separately from the independent font masks.
Regenerate it in a **fresh absolute output directory** with the central runner:

```sh
GOMATH_TYPESET_PREVIEW_DIR=/absolute/fresh-preview CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -run '^TestTypesetPNGAndVectorExamples/math-alphabets$' -count=1
```

## Operator limits and local display declarations

The existing `New(source, Options)` API now supports `\limits` and `\nolimits`
on the directly parsed commands `\sum`, `\prod`, `\lim`, and `\int`.
Explicit named operators described below accept the same directives.
No new options or external typesetting dependency are needed.

| Operator | Initial / `\textstyle` placement | `\displaystyle` placement |
| --- | --- | --- |
| `\sum`, `\prod`, `\lim` | Existing side scripts | Centered upper/lower limits |
| `\int` | Existing side scripts | Existing side scripts |
| `\operatorname{...}` | Side scripts | Side scripts |
| `\operatorname*{...}` | Side scripts | Centered upper/lower limits |

An explicit `\limits` selects centered placement on any of these
operators, including `\int`. An explicit `\nolimits` selects side scripts,
including in a display row. Only one placement directive is accepted per
operator; it may appear before, between, or after its `_` and `^` arguments.
The order of the upper/lower arguments does not affect geometry. A placement
directive without scripts is legal and adds no paths or dimensions.

```tex
\sum\limits_{i=1}^{n}
\prod^{n}\limits_{i=1}
\displaystyle\sum_{i=1}^{n}
\displaystyle\int_0^1
\displaystyle\int\limits_0^1
{\displaystyle\lim_{x\rightarrow0}f(x)}+\sum_i^n
```

The operator body's baseline remains y=0 and its real font outlines retain
their existing size. `\lim` uses upright text retaining the enclosing bold
style; symbol operators retain their existing real font style. Each limit uses
the existing 0.65 script-size scale. Visible limit ink is horizontally centered
on the operator's ink bbox. The body may shift horizontally to make room for
wide limits. Leading/trailing advance-only whitespace in a limit still consumes
width but does not move its visible ink off center. Limits above/below have at
least a 0.12 em gap from the body's ascent/descent box, measured at the body's
local size. Their full metrics contribute to the composite width, ascent and
descent, so following atoms, fractions, accents, delimiters and environment rows
clear the limits. Empty or spacing-only limits retain their horizontal advance
but add no vertical gap. Braces are required for multi-atom/nested limits, as for
ordinary scripts.

`\displaystyle` and `\textstyle` are **declarations in a math row**, not commands
taking one argument. They affect subsequent operators until the current row
ends or another declaration overrides them; they create no atoms or spacing.
They are inherited by ordinary braced groups, accents, radicals, delimiter
groups and matrix cells. A change inside a group, `\left...\right`, a font
command's group, or one environment cell/row is restored at that boundary.
Script arguments start in text style. Legacy `\frac` starts each argument in
text style at its existing 0.8 size scale; the explicit fraction/binomial
commands above use their own real operand sizes. Explicit declarations in
these braced arguments can override
that local mode without leaking to the parent or sibling argument. An unbraced
style declaration used as an atom, such as `x^\displaystyle`, is an error; write
`x^{\displaystyle\sum_i^n}` instead. A declaration after an operator does not
retroactively change that operator's script placement.

Eligibility is deliberately narrow: `x\limits`, `\sin\limits`, raw Unicode
`∑\limits`, `{\sum}\limits`, `\hat{\sum}\limits`, repeated directives and
duplicate scripts all return errors. Put the directive inside a style group,
for example `\mathbf{\sum\limits_i^n}`. A group as a whole does not become an
operator. `\displaylimits`, `\scriptstyle`, `\scriptscriptstyle`
and other unsupported commands remain rejected.

These declarations choose operator script placement and the new automatic
binomial mode/nominal em. They do not select
font MATH-table display-size glyphs, italic/skew corrections, optical variants,
TeX's eight math styles, display equation environments, or complete automatic
style transitions. The explicit fraction sizing policy is documented above;
the legacy `\frac` compatibility branch remains unchanged. There is no
TeX/Manim parity claim.

## Explicit named operators and stacked annotations

`\operatorname{rank}` lays out a literal name using actual upright font
outlines at the body's current size, retaining an enclosing bold style.
It is a single operator atom, with ordinary side scripts by default even in
display style. `\operatorname*{arg max}` uses the same upright outlines and
selects stacked limits by default in display style; it still uses side scripts
in text style. Explicit `\limits` / `\nolimits` override either default, with
the same order and duplicate-directive rules as builtin operators. A literal
name such as `lim` does not acquire the defaults of the builtin `\lim` command.
The name's upright style does not leak to following atoms or outer scripts.

The supported name grammar is deliberately finite:

- One required pair of braces, with at most `MaxOperatorNameRunes` (256) runes
  inside, including spaces and punctuation. The name claims one nesting frame.
- Unicode Latin, Greek and Cyrillic **letters**, ASCII decimal digits, literal
  ASCII spaces, ASCII hyphens (`-`) and ASCII periods (`.`) only. At least one
  letter or digit is required; leading/trailing spaces are errors. Internal
  spaces are preserved exactly as text advances, including repeated spaces.
  This literal-space behavior is a subset convention, not TeX math-space parsing.
- Precomposed letters such as `é` are accepted when present in the selected
  font. Combining marks, other scripts, non-ASCII digits, tabs/newlines,
  nonbreaking spaces, shaping/bidirectional controls and invalid UTF-8 are
  rejected. No Unicode normalization, shaping or bidi algorithm is applied.
  Missing font glyphs return errors; there is no replacement/fallback character.
- No commands, escapes, nested braces, `_`/`^`, alignment tokens or additional
  mathematical syntax are interpreted in a name. For example `arg max` is
  accepted; `arg\,max`, `\alpha`, `\mathbf{rank}` and `a+b` are rejected instead
  of having their semantics stripped. Hyphens/periods are literal text glyphs.

Whitespace outside the name's braces, including around the optional star, is
ignored by the existing parser. Use a surrounding style group to choose bold,
for example `\mathbf{\operatorname{rank}_i}`. Grouped/decorated names are
ordinary compound atoms: `{\operatorname{rank}}\limits` is rejected, while
`\mathbf{\operatorname{rank}\limits_i}` is supported.

`\overset{upper}{body}`, `\underset{lower}{body}`, and
`\stackrel{upper}{body}` each require **two braced arguments**. They reuse the
limits centering geometry: annotation ink is centered on the body's ink bbox,
with a 0.12 em vertical gap beyond the body's full ascent/descent box. The
body retains its y=0 baseline and current size; horizontal shifts reserve the
complete advance for a wide annotation. Annotations enter text style at 0.65
times the current size. A declaration inside an annotation may override that
local mode, without affecting the body; declarations in the body do not leak
out of its group. Font styles and color follow the existing group rules.

The body must have visible geometry; empty or spacing-only bodies are errors.
Horizontal rules such as `\frac{}{}` are supported through their stroke-aware
baseline metrics. An empty or spacing-only annotation adds no vertical gap;
its horizontal advance is retained. Both arguments can contain the supported
math subset, including nested stacks, named operators, accents, scripts,
fractions, delimiters and environments. Outer `_`/`^` scripts attach to the
complete compound stack as side scripts. A stack does not inherit operator
eligibility from its body, so `\overset{a}{\sum}\limits` is rejected; put any
operator directive inside the body instead.

```tex
\operatorname{rank}(A)
\displaystyle\operatorname*{arg max}_{x}^{n}f(x)
\operatorname{Éλ2}\limits_i^n
a\stackrel{\operatorname{def}}{=}b
\overset{\hat{n}}{\underset{g}{x}}_i
```

`\stackrel` has the same geometric placement as `\overset`. The library still
uses its existing uniform inter-atom spacing; neither command implements TeX
relation classification/glue, font MATH-table anchors, arbitrary `\mathop`,
`\DeclareMathOperator` or macro definitions. This remains a bounded notation
subset, not full TeX/AMS or Manim layout parity.

## Added rules and accents

All eight commands accept one math atom or a braced expression:

| Command | Geometry and width |
| --- | --- |
| `\overline{...}` | Filled rule across the argument's complete advance width. |
| `\underline{...}` | The same rule below the argument's descent. |
| `\hat{...}` | Short filled roof, width 0.42 em and height 0.18 em. |
| `\widehat{...}` | Roof spanning at least the complete argument width, height 0.22 em. |
| `\bar{...}` | Short rule, width 0.34 em. |
| `\vec{...}` | Short right arrow, width 0.58 em and height 0.18 em. |
| `\dot{...}` | One filled circular Bezier path. |
| `\ddot{...}` | Two equal circular paths with centers 0.18 em apart. |

Sizes are relative to the current local font size, including a reduced script
size. Rule/roof/arrow thickness is 0.045 em, or 0.060 em in an enclosing bold
style. Dot radius is the same thickness. Paths are filled with the scoped math
color (`Options.Color` until overridden) and have zero stroke width, so the object's geometric bbox includes their
actual vector extent. Circular outlines use the existing cubic-circle path
primitive; they are not font substitutions or bitmap marks.

The argument's baseline stays at y=0. Over accents start 0.10 em above its
ascent; underline ends 0.10 em below its descent. The layout includes both
baseline-box metrics and actual ink bounds, so nested accents, fractions,
radicals and matrix arguments retain vertical clearance. Short marks center on
the argument's ink bbox. Long rules and widehat use the full advance box,
including scripts/spacing inside the argument. A mark wider than the base can
increase the horizontal advance and shift the base to the right; it never
shifts the argument vertically. The complete mark contributes to ascent,
descent and width for subsequent layout, delimiters and scripts.

An accent consumes its argument before outer scripts are parsed:

```text
\hat x_i       equivalent to \hat{x}_i; i is a side script of the accent atom
\hat{x_i}      i is inside the argument, below the accent
\widehat{x_i^2} covers the grouped base and both inner scripts
\hat\bar x     equivalent to \hat{\bar{x}}; the outer mark clears the inner one
x^{\ddot{y}}   the mark uses the existing 0.65 script-size scale
```

Existing side-script placement is retained; there is no TeX MATH-table italic
correction, font-specific accent anchor, automatic dotless i/j substitution or
stretchable glyph assembly. `\vec` is a short accent, not a stretching
`\overrightarrow`. `\bar` remains short on multi-character arguments; use
`\overline` for a spanning rule. `\mathbf{\hat{x}}` selects actual bold base
glyphs and a thicker vector mark. Styling inside the argument changes those
glyphs; an enclosing style controls the mark. Custom fonts need explicit style
variants under the existing font contract.

Empty or spacing-only arguments such as `\hat{}`, `\bar{\quad}` and
`\dot{\text{ }}` are rejected. Missing arguments, duplicate scripts and
unsupported commands inside an argument are also rejected. Commands such as
`\tilde`, `\acute`, `\overbrace`, `\underbrace`, macros and file inclusion
remain unsupported. Supported existing notation includes braced fractions and
square roots, side scripts, the current Greek/operator list, scoped font/text
commands, scalable delimiters and the explicitly supported matrix/cases
environments; this does not imply support for arbitrary TeX syntax.

## Set membership, inclusion and set operations

The existing `New(source, Options)` API accepts these 16 commands (15 distinct
symbols), including the `\owns` alias for `\ni`:

| Commands | Symbols |
| --- | --- |
| `\in`, `\notin`, `\ni`, `\owns` | ∈, ∉, ∋, ∋ |
| `\subset`, `\supset`, `\subseteq`, `\supseteq` | ⊂, ⊃, ⊆, ⊇ |
| `\subsetneq`, `\supsetneq`, `\nsubseteq`, `\nsupseteq` | ⊊, ⊋, ⊈, ⊉ |
| `\cup`, `\cap`, `\emptyset`, `\setminus` | ∪, ∩, ∅, ∖ |

The equivalent Unicode characters are accepted as math atoms. They support
existing scripts, accents, fractions, matrices, limits and stacked annotations:

```tex
\displaystyle\operatorname*{arg max}_{x\in R}\overset{\hat{n}}{f}(x)
\overline{A\cup B}+x^{y_{\notin}}
A\subsetneq B+A\nsubseteq C+A\setminus B
```

A cmap check of every embedded Go font variant confirms that only ∪ and ∩ in
this table are native to those fonts. `\cup` and `\cap` use the existing
`gomath/text` glyph loading, including its real regular/bold/italic/bold-italic
variants and baseline origin. The other 13 symbols use the original CFF vector
contours and advance widths from **STIXGeneral 1.1.0** Regular/Bold, bundled as
`set_symbols_data.go` under the derivative name **Gomath Set Symbols**. They
are actual font glyphs, including the complete negated contours; no placeholder,
hand-drawn replacement, slash overlay or synthetic slant is used. Source hashes:

```text
STIXGeneral.otf     5add3f3f2bd7fd897d2fa5ccbe468607c52111dc44cdfaaf2d851a574f5357a7
STIXGeneralBol.otf  7e4dfb4979e9bb79a0bdedc56f055eab615fb9a3dff8788a52992ae65e8f572d
```

The selected source-font outlines are distributed under SIL OFL 1.1; copyright
and the full license are retained in [LICENSE-STIX.txt](LICENSE-STIX.txt).
Source STIX font names identify provenance, not the derivative's primary name.
No complete font file or system-font lookup is needed at runtime.

Supplemental mathematical symbols stay upright inside `\mathit`; an enclosing
bold style selects the real bold source face, also in bold-italic contexts.
Their own glyph coordinates are scaled from 1000 units/em at baseline y=0.
Advance width includes bearings, and actual Bezier extrema determine ascent
and descent, including the slash/underbar descenders of negated symbols. Their
closed paths inherit color with zero stroke width. Every curve shares
`MaxLayoutCurves` with text glyphs and accents. Collapsed dimensions return an
error. The selected font/style does not leak from a group to following atoms.

This supplementation is explicitly limited to the default embedded faces.
A custom `Options.Font`, or a selected custom style variant, must contain its
own glyph; missing glyphs return errors. Existing custom-font variant rules
remain in force. Mathematical supplementation does not change literal `\text`
or `\operatorname` grammars or supply missing glyphs in those contexts.

These symbols keep existing uniform inter-atom spacing. There is no TeX
relation/binary-operator glue, MATH-table axis alignment, display-size set
operator, generic `\not` overlay, or full TeX/Manim parity. In particular
`\varnothing`, `\bigcup`, `\bigcap`, and other unlisted commands still return
errors. Set symbols do not acquire `\limits`/`\nolimits` eligibility.

Independent references are frozen in `tests/data/mathtex/set-symbols/`:
60 source-font metrics/bbox records and 60 PNG masks cover 15 glyphs × four
style contexts. `fontTools` BoundsPen/hmtx reads the original fonts; Pillow /
FreeType rasterizes those source fonts directly at 256 ppem. Neither oracle
loads gomath. Go tests consume only committed JSON/PNG files. Supplemental
metrics use a 2e-12 em tolerance. Native Go quadratic contours permit one
1/2048 em design unit for implied midpoint/fixed-point rounding. Reference
metadata records the source-derived hmtx/glyf origin difference: existing
`gomath/text` uses raw glyph coordinates, while FreeType applies the left side
bearing; the italic ∪ reference is aligned by this explicit offset. Binary
silhouette IoU must be at least 0.95 and unmatched ink must lie within two
pixels of the opposite silhouette. The measured minimum is 0.967637.
This verifies source glyph geometry and rendering, not an independent TeX
layout engine's spacing or baseline decisions.

The development-only extractor `tests/reference/mathtex_symbols.py` records
source SHA256 hashes and tool versions (fontTools 4.45.1, Pillow 11.2.1,
FreeType 2.13.3 for the committed fixtures). To reproduce it, supply source
fonts with the hashes above and the module's Go font source directory:

```sh
python3 tests/reference/mathtex_symbols.py --font-dir /path/to/STIXGeneral-fonts --gofont-dir /path/to/golang.org/x/image/font/gofont
gofmt -w mathtex/set_symbols_data.go
```

Python, source fonts and those development tools are not runtime or Go-test
dependencies. The independently rendered glyph masks are distinct from this
library's own formula previews `operator-membership.png/.svg` and
`set-symbols.png/.svg` under `tests/data/mathtex/`. The previews each render one
960×480 frame, with PNG pixel roundtrip and vector-only SVG checks. Regenerate
in a new output directory using `GOMATH_TYPESET_PREVIEW_DIR` and the central
runner's `-run '^TestTypesetPNGAndVectorExamples/(operator-membership|set-symbols)$'`;
exports reject existing output files.

## Input and layout budgets

| Budget | Bound |
| --- | --- |
| Source | Valid UTF-8; at most 16,384 runes and a 65,536-byte precheck. |
| Font size | Finite and in `(0, 1e6]`; collapsed accent/supplemental-symbol path dimensions are rejected. |
| Structural nesting | At most 128 active frames, including the top row. Braced accent arguments use their group frame; every unbraced accent argument also claims a frame. Each script argument claims a frame, plus its group row when braced. |
| Glyph, accent, supplemental-symbol/alphabet and new fraction/binomial curves | At most 1,048,576 across the formula; each new generated curve is charged once, without recounting its base. |
| Literal operator names | At most 256 runes inside the braces; each name consumes one nesting frame. |
| Environments | At most 64 rows and 64 columns each; 4,096 total cells shared with all nested substack rows. |
| Substack | One column, at most 64 visible nonempty rows per stack; shared source/depth/curve/total-cell limits. |

Other generated structural paths remain bounded by source, nesting and cell
limits. The budgets bound accepted input/geometry, not elapsed time or a strict
heap quota. No expression evaluation, macro expansion or I/O command executes.

## Verification

Every test and benchmark source is under `tests/_cases/mathtex/`; the central
runner compiles private tests through Go overlays. Tests cover geometric boxes,
baseline preservation, width/centering, nonoverlapping marks and scripts,
nested composition, actual font styles/color, size limits, malformed/unsupported
input, braced/unbraced nesting and shared curve-budget boundaries. Concurrent
layout tests check independent geometry. Rendering tests compare each marked
formula against a cloned base with the mark removed: every command must add
visible pixels within its own vector bbox. PNG roundtrip and SVG path export
are also tested. These are subset integration checks, not independent TeX goldens.
Operator tests additionally check limits/directive order, unchanged inline
defaults, ink centering and baseline preservation, wide/nested limits, font
styles, all declaration boundaries, input/curve budgets and concurrent ownership.
Raster tests remove each upper/lower limit separately and require visible pixel
differences confined to its own bbox above/below the operator body.
Named-operator/stack tests include real upright Latin/Greek/Cyrillic outlines,
literal spaces, script/annotation scaling, scope restoration, stroke-only
bodies, malformed/unsupported names, exact name/nesting/curve bounds,
non-finite dimensions, concurrent ownership and visible raster annotations.
`tests/data/mathtex/legacy-layout.json` freezes eight pre-extension formulas:
the regression test hashes exact path control points, styles, drawable order
and baseline metrics, excluding fresh identities. These snapshots protect this
library's previous defaults and are not independent TeX goldens.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go vet -tags noasm ./mathtex
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkAccentNestedTypesetting$' -benchmem -benchtime=150ms -count=3
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkOperatorLimitsNestedTypesetting$' -benchmem -benchtime=150ms -count=3
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkNamedOperatorStacksTypesetting$' -benchmem -benchtime=150ms -count=3
```

`tests/data/mathtex/accents.png` and `accents.svg` are development previews
generated by the pure-Go integration test. To regenerate only that preview,
set `GOMATH_TYPESET_PREVIEW_DIR` to that directory and select
`-run '^TestTypesetPNGAndVectorExamples/accents$'` through the same runner.
The same mechanism generates `operator-limits.png` and `operator-limits.svg`
with `-run '^TestTypesetPNGAndVectorExamples/operator-limits$'`. Both previews
are produced by this library; neither is an independent TeX golden.
The named-operator/stack preview is `named-operators-stacks.png` / `.svg`,
generated with `-run '^TestTypesetPNGAndVectorExamples/named-operators-stacks$'`.

Measured on 2026-11-04 with Go 1.27.3 (module Go 1.26.0), darwin/arm64,
Apple M2, GOMAXPROCS=8 and `CGO_ENABLED=0 -tags noasm`. The new nested accent
benchmark uses warmed embedded fonts and this expression:

```tex
\widehat{\mathbf{x}_{i}+\vec{y}}^{\dot{n}}+\underline{\frac{\ddot{a}}{\overline{b}}}
```

Three serial-package samples (`-p 1`, 150 ms each) measured 111,763 / 85,148 /
81,746 ns/op, 159,401 / 159,393 / 159,393 B/op and 501 allocs/op. This measures
parsing, layout and object validation only; there is no image size or frame
count because rasterization, animation and export are excluded. This is a local
measurement, not an external-renderer performance comparison. The separate
preview renders one 960×480 frame.

The operator-limits benchmark uses the same environment, warmed fonts and
150 ms / three-sample / `-p 1` settings, with this workload:

```tex
\displaystyle\sum_{i=1}^{\hat{n}}\frac{x_i}{\prod\limits_{j=1}^{m}j}+\lim\limits_{x\rightarrow0}\int\limits_0^1f(x)dx
```

Samples measured 212,211 / 198,463 / 174,712 ns/op, 455,424 / 455,412 /
455,411 B/op and 1,080 allocs/op. This again measures parsing, vector layout
and object validation, excluding rasterization/animation/export and any
external renderer. The operator-limits preview is one separate 960×480 frame.

The new named-operator/stack benchmark uses the same Go 1.27.3 / darwin-arm64 /
Apple M2 environment, warmed fonts and three 150 ms samples with `-p 1`:

```tex
\displaystyle\operatorname*{arg max}_{x\infty}^{\hat{n}}\frac{\overset{m}{x}}{\underset{g}{y}}+a\stackrel{\operatorname{def}}{=}b
```

Samples measured 194,760 / 190,028 / 153,711 ns/op, 353,385 / 353,362 /
353,362 B/op and 761 allocs/op. This synthetic workload measures parsing,
vector layout and validation only, excluding rasterization/animation/export
and external renderers. The separate preview renders one 960×480 frame.


On 2026-11-04 the new set-symbol tests passed through the central runner with
`CGO_ENABLED=0 -tags noasm`: the complete mathtex suite reached 96.1% statement
coverage, the complete package race run passed, and package vet passed. The
eight legacy exact layout/control-point digests remain unchanged. Tests cover
source font coverage, independent metrics/masks, color/style/scope, Unicode and
aliases, custom-font failures, nested scripts/compositions, size/source/depth/
shared-curve budgets, unsupported commands and concurrent contour ownership.

Only the new symbol workload was benchmarked for this slice, with warmed fonts,
Go 1.27.3 / darwin-arm64 / Apple M2 / GOMAXPROCS=8, `CGO_ENABLED=0 -tags noasm`,
three serial-package (`-p 1`) 150 ms samples:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkSetSymbolsTypesetting$' -benchmem -benchtime=150ms -count=3
```

```tex
\displaystyle\operatorname*{arg max}_{x\in A\setminus B}\overset{\hat{n}}{f}(x)+\mathbf{A\subseteq B\cup C}+x\notin\emptyset
```

Samples measured 137,441 / 136,856 / 135,088 ns/op, 356,532 / 356,522 /
356,523 B/op and 840 allocs/op. This measures parsing, vector layout and object
validation; rasterization, animation, export and external renderers are excluded.
It has no image/frame workload; the preview frames are separate integration
checks. Previous slice benchmarks above were not rerun for this extension.

On 2026-11-04 the mathematical-alphabet extension passed the complete central
mathtex suite (95.8% statement coverage), complete package race and package vet
with `CGO_ENABLED=0 -tags noasm`. All eight legacy exact control-point/style
fixtures remain unchanged. Added checks include all 166 real glyphs, Unicode
exceptions, independent font metrics/masks, baseline and nested scripts,
font/display scoping, successful and missing custom-font coverage, exact
source/nesting/curve boundaries, nonfinite/collapsed dimensions, concurrent
contour ownership and PNG/vector-only SVG preview rendering.

Only the new mathematical-alphabet hotspot was benchmarked, with warmed fonts,
Go 1.27.3 (module Go 1.26.0), darwin/arm64 Apple M2, GOMAXPROCS=8,
`CGO_ENABLED=0 -tags noasm`, three serial-package 150 ms samples:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkMathAlphabetsTypesetting$' -benchmem -benchtime=150ms -count=3
```

```tex
\displaystyle\operatorname*{arg max}_{x\in\mathbb{R}}\overset{\mathcal{A}}{\mathfrak{f}}(x)+\mathbb{C^{2}}+\frac{\mathcal{L}_{g}}{\mathfrak{h}}
```

Samples measured 167,984 / 200,149 / 185,208 ns/op, 364,375 / 364,364 /
364,366 B/op and 756 allocs/op. This workload measures parsing, vector layout
and object validation, excluding rasterization/animation/export and external
tools. It has no image/frame workload; the separate preview is one 960×480
frame. No previous slice benchmark was rerun for this extension, and these
local measurements are not a cross-renderer performance comparison.
