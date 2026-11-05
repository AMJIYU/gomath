# Bounded native multiline mathematical environments

`gomath/mathtex.New(source, Options)` accepts two additional mathematical
environments using real existing boxes, editable glyph paths and vector rules.
No external TeX, process launch, cgo, system-font lookup or additional runtime
dependency is introduced. Existing matrix/cases code, default formulas and
their exact layout fixtures remain unchanged.

```go
object, err := mathtex.New(
    `\begin{aligned}a&=\dfrac{n}{g}\\&=\binom{n}{k}\end{aligned}`,
    mathtex.DefaultOptions(),
)
```

## Accepted structure and explicit alignment

```tex
\begin{aligned}
  f(x)&=\dfrac{\hat{n}}{g}&A&=\mathbb{R}\\
  \sum_{\substack{i=1\\j=2}}^n i&=\binom{n}{k}&B&=\mathcal{A}
\end{aligned}

\begin{gathered}
  \color{red}f(x)=\dfrac{\hat{n}}{g}\\
  \sum_{\substack{i=1\\j=2}}^n i=\binom{n}{k}\\
  \textcolor{violet}{\mathbb{R}=\mathcal{A}}
\end{gathered}
```

`aligned` requires an explicit, even number of cells on every row, at least
two, separated by `&`. Each pair is right-aligned then left-aligned. Multiple
pairs have separate shared column widths; e.g. `a&=b&c&=d` has two pairs.
All rows must contain exactly the same number of cells. The engine does not
invent missing trailing cells or pad irregular input silently.

Alignment uses complete advances, including authored spacing, rather than
centering visible ink. Each column width is the maximum full advance of that
column. Even-indexed cells shift right by `columnWidth-cellWidth`; odd-indexed
cells start at the column's left boundary. There is a fixed **0.12 local-em gap
within each pair**, and **1.0 local-em separation between pairs**. A blank
column has width zero, with no matrix-style minimum width added. This is an
explicit spacing policy, not amsmath relation glue or a font MATH-table rule.

Explicit empty `aligned` cells are legal, for example `&=b` for a continuation
or `a&` for a blank right side. A row must contain some visible body; rows that
are entirely empty or spacing/declarations only are errors. Empty pairs may
serve as deliberate placeholders when another pair in that row is visible.

`gathered` accepts exactly one visible math body per row and rejects `&`.
The widest row determines the advance width, and each row shifts horizontally
by `(maximumWidth-rowWidth)/2`. It centers full advances, so asymmetric authored
spacing is intentionally retained. It does not align relation symbols.

`\\` separates rows in both environments. Empty environments, blank rows,
consecutive row separators, **trailing row separators**, odd `aligned` column
counts, ragged rows and mismatched ends are rejected with rune-position errors.

## Baseline boxes, local size and styles

All cells in a row share a baseline. Row ascent is at least **0.7 local em**
and its depth at least **0.2 local em**, expanded by every cell's complete
baseline box. Thus descenders, side scripts, operator limits, accents, tall
fractions, binomials and nested environments determine the actual clearance.
There is a **0.3 local-em gap between complete row boxes**, rather than a
fixed baseline step that could overlap tall rows.

For total row-box height plus gaps `T`, the environment box has baseline y=0,
top `0.16*localEm + T/2`, depth `T-top`, and the measured full horizontal
advance. It centers the stack around its **0.16 local-em math axis**. It does
not use the first or last row's baseline as the enclosing baseline. These
metrics feed outer scripts, fractions, accents, delimiters and matrices.

Each new environment cell starts in the library's **display mode at the current
local font size**. The environment does not magnify glyphs in a surrounding
script or annotation. Default sum/product/lim limits stack, while integrals and
unstarred named operators retain their existing default side scripts. Explicit
`\limits`/`\nolimits` after supported operators still work.

An automatic `\binom` uses display operand sizing at that inherited local em.
Explicit `\displaystyle`/`\textstyle` can override the current cell, following
the existing finite style rules (including nominal-em restoration for the new
fraction family). The next cell restarts the environment's local display mode;
the previous enclosing mode resumes after the environment. In particular,
`\textstyle` before an environment does not suppress its per-cell display
default; put the declaration inside each affected cell. Legacy `\frac` retains
its exact existing 0.8 operand sizing. There is no TeX eight-style font system
or display-size MATH-table glyph selection.

Color declarations restore at every cell/row boundary. A color before the
environment is inherited by all cells, while a declaration inside one cell
affects that cell only. Nested `textcolor`, font/alphabet groups and scripts
retain their existing scopes. The real regular/bold/italic faces and genuine
mathematical alphabet outlines continue to be used. A missing custom-font
glyph/style remains an error; these environments add no font fallback.

Both environments can nest in the supported groups, each other, matrix/cases,
substack, scripts, fractions, accents and explicit delimiters. They create no
automatic enclosing delimiters. Wrap them in `\left...\right` when needed.
Their objects are ordinary compound atoms, so outer scripts use the existing
side-script layout, and an outer `\limits` directive is invalid.

## Bounds and unsupported syntax

The existing shared limits apply, including to nested combinations:

| Resource | Bound |
| --- | --- |
| Whole source | 16,384 runes, valid UTF-8, at most four bytes per permitted rune budget. |
| Active row/group/script nesting | 128. |
| Rows per environment | 64. |
| Cells per `aligned` row | 64: at most 32 right/left pairs. |
| Cells per `gathered` row | Exactly one. |
| Cells across all environments and substacks | 4,096, including explicitly empty cells. |
| Shared glyph/alphabet/accent/new-fraction curves | 1,048,576. |
| Nominal `Options.Size` | Finite, positive, at most 1e6; collapsed/overflowing layout dimensions are errors. |

Row and column bounds are checked before parsing an additional row/cell. Every
parsed cell consumes the common work budget once, including nested environment
and substack cells. The parser consumes the source without macro expansion;
each environment places its stored cells in bounded passes. Nested bounds
queries are also limited by the shared source/curve/depth budgets. New
environment alignment itself adds no curves and does not reset contour counts.
An actual 64×64 source is tested at the exact cell budget; adding a nested cell
exceeds the common bound even though outer row/column counts still fit.

Optional positioning (`[t]`, `[b]`, `[c]`), optional `\\[dimension]` row spacing,
starred `\\*` row breaks,
custom column preambles, equation tags/labels, `\notag`, `\intertext`,
`\multicolumn`, automatic numbering and other unlisted commands remain errors.
`align`, `align*`, `gather`, `gather*`, `alignedat`, `split`, `array` and other
unlisted environments are not aliases. A leading literal `[` at the start of
an environment or after `\\` must be braced (`{[x]}`) or expressed with
`\left[...\right]`, so it cannot be mistaken for an unsupported option.
A literal `*` at the start of a row after `\\` must likewise be braced as `{*}`.

These are finite mathematical environments, not general TeX/amsmath syntax,
spacing, default-font or default-style equivalence. There is no complete
TeX/Manim parity claim and no external fallback for rejected input.

## Pinned source inspection and independent references

The read-only Manim reference was inspected at commit
`16ea443e2a5f1d79db0a721161411ce916d6e733`. Its `SingleStringMathTex` defaults to
`align*` and delegates layout to `tex_to_svg_file`; the template imports amsmath
and wraps the supplied expression in a named environment. The searched reference
tree contains no literal `\begin{aligned}` or `\begin{gathered}` fixture that
could establish independent pixel parity for these additions. The relevant
inspected files are unchanged and have these SHA256 values:

```text
manim/mobject/text/tex_mobject.py
4065d0d4c2ce4d69074bf8dd1642eb38c24aece8b2e05aca99a1918f8369d8f6
manim/utils/tex.py
f7aa83daeba7bf312f80c0b348fe452eff01a6340d161543974de29d7be3cc78
manim/utils/tex_file_writing.py
57d2179de2e7f099ecf6cbaf9e3dd407667f51549a8b89cbf763bdfacd5a6853
```

Independent development references live in `tests/data/mathtex/multiline/`.
Two manually specified operand grids cover three rows/two aligned pairs and
four gathered rows, with **16 individual placements**, unequal advances and
descenders. `tests/reference/mathtex_multiline.py` reads the original GoRegular
TTF hmtx/glyf metrics and outlines with fontTools 4.45.1. It assembles independent
SVGs using the stated analytic alignment/baseline policy, then CairoSVG 2.7.1 /
Cairo 1.18.4 renders the original source-font outlines at 768×768, 128 px/em.
It never reads GoMath output or imports its implementation.

The manifest records generator/source-font/artifact hashes and full glyph/cell
metrics. Go tests verify the exact embedded font hash, SVG/PNG hashes, total
baseline boxes and individual operand bounding boxes. Native quadratic midpoint
rounding permits `(rows+1)/2048` em for accumulated placement metrics. Colored
interior-mask IoU must be at least 0.95, allowing two byte levels per channel;
whole-image RGB mean absolute error must not exceed one byte. Measured minimum
IoU is **0.990984**. The aligned/gathered image MAE values are **0.013939** and
**0.006308** bytes. These validate the declared finite layout policy and real
font painting, not an independent full-TeX layout engine.

The development extractor needs Python 3.11.5, fontTools, CairoSVG/Cairo and the
original font; Go tests/runtime need none of them. On macOS the development
loader must be able to discover the installed Cairo library. Regenerate with:

```sh
python3 tests/reference/mathtex_multiline.py --font /path/to/golang.org/x/image/font/gofont/ttfs/Go-Regular.ttf
```

The source font retains the existing [Go font license](../LICENSES/go-fonts.txt).
Separate native integration previews are `tests/data/mathtex/multiline-aligned`
and `multiline-gathered` (`.png` and `.svg`), each one 960×480 frame. They test
actual PNG pixel roundtrips and vector-only SVG paths, and are not independent
reference goldens. Regenerate in a fresh absolute directory with:

```sh
GOMATH_TYPESET_PREVIEW_DIR=/absolute/fresh-preview CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -run '^TestTypesetPNGAndVectorExamples/multiline-(aligned|gathered)$' -count=1
```

## Validation checkpoint

On 2026-11-04 the final complete central mathtex suite passed in **1.184 s** with
**96.1% statement coverage**, including all eight unchanged exact legacy
fixtures. The complete race run passed in **22.448 s**, including the actual
64×64/shared-cell-boundary test and unsupported starred-row-break checks.
Package vet, the full production
CGO0/noasm build and purecheck passed; purecheck reports 42 non-standard
packages and no native inputs or runtime process-launch imports.

Meaningful checks cover advance-based right/left/center alignment, baseline
struts and tall/deep operands, local display/color/font scopes, nested
environments/fractions/scripts/substacks, strict grammar, exact source/depth/
row/column/cell/curve boundaries, custom-font errors, independently owned paths
and concurrent layouts/reads/copies. Render tests remove each cell and require
visible pixel changes confined to its bbox, with no viewport clipping. The
two integration previews retain 30/26 drawable vector paths and 17,972/10,632
visible pixels respectively, with exact PNG roundtrips and no SVG image/text
fallback.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -count=1 -cover -vet=all
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -count=1 -vet=all
CGO_ENABLED=0 go vet -tags noasm ./mathtex
CGO_ENABLED=0 go build -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
```

Only the new multiline workloads were benchmarked, serially with `-p 1`, after
validation completed: Apple M2, darwin/arm64, Go 1.27.3 (module Go 1.26.0),
GOMAXPROCS=8, CGO_ENABLED=0, `noasm`, three 150 ms samples.

| Workload | ns/op, three samples | B/op, three samples | allocs/op |
| --- | --- | --- | --- |
| aligned: two rows, two pairs | 198496 / 207826 / 195254 | 438871 / 438861 / 438864 | 1219 |
| gathered: three rows | 191392 / 190381 / 183991 | 393406 / 393412 / 393410 | 1076 |

These different sources are not an equal-work speed comparison. They measure
parsing, vector layout, bounds and validation of one formula, excluding raster,
export, animation and independent reference tools. There is no image size/frame
count for the benchmark; the separate integration previews above are one
960×480 frame each. The exact workloads are:

```tex
\begin{aligned}\color{red}f(x)&=\dfrac{\hat{n}}{g}&A&=\mathbb{R}\\\sum_{\substack{i=1\\j=2}}^n i&=\binom{n}{k}&B&=\mathcal{A}\end{aligned}
\begin{gathered}\color{red}f(x)=\dfrac{\hat{n}}{g}\\\sum_{\substack{i=1\\j=2}}^n i=\binom{n}{k}\\\mathbb{R}=\mathcal{A}\end{gathered}
```

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -p 1 -run '^$' -bench '^BenchmarkMultilineLayout$' -benchmem -benchtime=150ms -count=3
```

## CLI integration

The actual `multiline-math` scene composes two-pair aligned equations and
gathered fractions with scoped colors. Its final640x360 SSAA4 layout was visually
inspected. Independent CairoSVG interpretation of exported geometry at .6/1.1/2.5
seconds passes unchanged thresholds: RGBA MAE .078498–.094219/255, maximum17–18.
Provenance: artifacts/multiline-math-layout2-svg-20261104/provenance.json.
It shares Go scene construction and is separate from operand-layout references.
