# Indexed radicals, boxes and invisible layout

`mathtex.New(source, options)` accepts the following additional finite subset.
No new Go API or `Options` fields are required. Runtime and Go tests use only
Go; no TeX installation, native font renderer or process fallback is involved.

```go
formula, err := mathtex.New(
    `\boxed{\sqrt[n_i]{\dfrac{g}{n}}\phantom{W}A}_j`,
    mathtex.DefaultOptions(),
)
```

These are explicit GoMath layout policies, not full TeX/amsmath spacing or
Manim MathTex equivalence. Existing unindexed `\sqrt{...}` and ordinary formulas
keep their previous paths, metrics, drawable order and pixels. Existing
fixture files have not been regenerated.

## Syntax and scope

| Command | Operand and result |
| --- | --- |
| `\sqrt[index]{radicand}` | Nonempty mathematical index row in square brackets; mandatory braced radicand. Both operands can contain supported nested mathematics. |
| `\boxed{body}` | Mandatory braced body with a real closed four-segment vector border, transparent fill and inherited color stroke. Empty/invisible bodies are allowed. |
| `\phantom{body}` | Parse and lay out the body; retain its advance, ascent and depth; emit no body objects or paths. |
| `\hphantom{body}` | Retain only body advance; ascent and depth are zero. |
| `\vphantom{body}` | Retain only body ascent/depth; advance is zero. |

All new commands form ordinary atoms, so outer scripts attach to the resulting
layout box. They do not inherit operator eligibility for `\limits`. Commands
still require their documented braces; unsupported commands, font glyphs and
syntax inside invisible operands are errors, not discarded content.

An unprotected `]` closes the index. Literal brackets inside it must occur
inside `{...}`, `\text{...}`, or a matched `\left...\right` construct; a raw
`[` or raw bracket as an unbraced script atom is rejected. For example:

```tex
\sqrt[n_i]{g}
\sqrt[\frac{n}{g}]{\boxed{\mathbb{R}}}
\sqrt[{[n]}]{g}
\sqrt[\left[n\right]]{g}
\sqrt[\sqrt[3]{n}]{g}
```

Empty indices, including `[]`, `[{}]` and `[\phantom{}]`, are rejected. A
positive-size advance-only/invisible index such as `[\quad]` or `[\phantom{n}]`
is intentional and valid. Empty radicands are valid, as in the old sqrt.

The index enters text style at half the current local em. Its internal scripts
use the existing `.65` scale. Explicit `\displaystyle`/`\textstyle`, forced
fractions and font/alphabet commands retain their previously documented
semantics, including nominal-em restoration for explicit declarations and
forced fractions. Thus `.5em` describes the initial index row, not a forced
size override of every possible descendant.

Color and display declarations are scoped independently in index, radicand and
boxed/phantom operands. They do not leak to the enclosing row or another
operand. A radical rule or boxed border uses the enclosing command's color;
an internal `\color` affects the body only. Fonts use the same real outlines,
strict custom-font variant/missing-glyph rules and immutable shared caches as
other mathtex commands.

## Baseline and explicit dimensions

Here `e` is the current local em, and `w`, `h`, `d` are an operand's advance,
ascent and depth relative to its baseline. Dimensions are layout metadata,
independent of `Object.Bounds()`, which describes actual paths and excludes
stroke expansion. A phantom has invalid/empty ink bounds even when all three
layout dimensions are positive. No transparent rectangle is retained.

The existing row gap `.04e` still separates atoms, including zero-width
vphantoms and empty phantoms. Phantom is not a glue or kerning command. Spaces
inside literal text and explicit spacing commands preserve their actual
advance; ordinary math-source whitespace is still ignored. Phantom dimensions
affect surrounding fractions, scripts, delimiters and matrix/multiline row
metrics. Existing commands requiring visible ink (such as an accent body or
a substack row) still reject a standalone invisible body.

**Boxed:** padding is `.2e` from each operand layout edge to the inside edge
of a `.045e` border. With `s = .045e`, `p = .2e`, `q = p+s`:

```text
advance = w + 2q
ascent  = h + q
depth   = d + q
body x shift = q; body baseline remains y = 0
border centerline: x = s/2 .. advance-s/2
                   y = -depth+s/2 .. ascent-s/2
```

The stroke uses the engine's ordinary round joins/caps. The whole stroke fits
within the layout box. The padding uses operand layout edges, including
advance-only whitespace; it is not an ink-bounds-derived frame. Scripts start
from the outer box's right edge and use its padded ascent/depth.

**Indexed radical:** reuse the established four-line radical outline with a
reserved horizontal prefix. The radicand baseline remains zero; no font MATH
table, display-size radical glyph or automatic TeX degree positioning is used.
Let index dimensions after initial half-em layout be `iw`, `ih`, `id`:

```text
s = .045e; a = s/2
start = a + max(0, iw-.18e)
top = max(.4e, radicand.h+.1e)
index x = start+.18e-iw
index baseline y = max(.28e, .55top)+id
radicand x = start+.48e
outline vertices:
  (start, .12e)
  (start+.12e, .18e)
  (start+.24e, -radicand.d-.06e)
  (start+.43e, top)
  (radicand.x+radicand.w+.05e, top)
advance = radicand.x+radicand.w+.05e+a
ascent = max(top+a, index.y+ih)
depth = radicand.d+.06e+a
```

This keeps the degree's layout right edge before the rising radical stroke,
clears the short hook and handles wide/tall nested indices without overwriting
the radicand. It is a bounded, explicit placement convention rather than
TeX's script-script-style algorithm. The old unindexed radical intentionally
continues to use its exact previous geometry.

## Shared limits and errors

All operands, including discarded phantom bodies, consume the formula's shared
source, nesting, environment/substack-cell and curve budgets:

- At most 16,384 UTF-8 source runes (and at most four times as many bytes).
- At most 128 parser nesting frames; index rows and braced operands claim
  frames and share the limit with all other nesting.
- At most 64 rows/columns per environment and 4,096 aggregate environment
  cells/substack rows per formula. Invisible bodies do not reset these counts.
- At most 1,048,576 charged cubic curves. Every new radical/border and every
  path in its operands is charged once, including legacy frac/sqrt/delimiter
  rules that were previously bounded only by structural/source limits.
  Nested phantom removal never refunds work. The parser-local path ledger is
  enabled on demand; formulas using only old commands retain the old accounting.
- Existing public size range is finite `(0, 1e6]`. New structural clearances
  and font scaling must remain representable; unrepresentable subnormal sizes,
  zero/collapsed padding and nonfinite dimensions return errors. The public
  source/size bounds prevent ordinary layout additions from reaching floating
  point overflow; validation also checks each discarded operand's path/style.

All `New` failures return `(nil, error)`. There is no successful partial object,
missing-glyph replacement, silent dimension clamping or external fallback.
Layout output owns its geometry. Concurrent calls and immutable scene sampling
are supported after authoring; callers must not mutate a shared authored tree
while sampling/reading it.

## Validation and references

All new tests, fixtures and development tools are under `tests/`:

- `tests/_cases/mathtex/radical_boxes*_test.go`: analytical box/baseline and
  script placement, nested styles/colors/environments, hidden work accounting,
  exact curve-limit boundary checks, malformed input, shared-cell/depth/source
  limits, numeric errors, strict fonts, ownership and concurrency.
- Twenty fontTools/manual SVG/Cairo references cover indexed roots, boxed
  descenders and all three phantom advances in four real Go font faces. The
  generator reads source `hmtx`/`glyf` metrics/outlines and explicit operand
  placements, never Go layout output. Source font hashes, generator hash and
  tool versions are recorded in `tests/data/mathtex/radical_boxes/reference.json`.
  Go-font notices remain in `LICENSES/go-fonts.txt`. No new font is embedded.
- Metric tolerances allow two font design units (`2/2048em`) for sfnt implied
  midpoint rounding. Whole-image RGB MAE must be at most one byte and each
  color's interior silhouette IoU at least `.94`. These references establish
  source-font geometry and the explicit policy, not full TeX layout fidelity.
- `legacy.json` is a separate pre-extension compatibility capture: five old
  formulas' exact drawable/control-point/style/order and 640×320 RGBA hashes.
  Existing eight old metric/control-point fixtures also remain unchanged.
- New real-ink raster ablation, no-ink phantom tests and PNG/SVG Write/Transform
  previews include intermediate frames. Sampling checks identities, independent
  snapshots, concurrent reads and actual changing pixels. These are engine
  integration checks, not independent Manim animation goldens.

Run from the repository root:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./mathtex
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -run '^$' \
  -bench '^BenchmarkRadicalBoxes$' -benchmem -benchtime=300ms -count=3 -p 1
```

Race uses cgo only for Go's race instrumentation; product compilation and
ordinary tests are checked with cgo disabled. The focused benchmark measures
two short layout workloads, excluding rasterization, SVG and external tools.
Shared-host timings do not establish a repository-wide speed improvement.

Observed on November 5, 2026, Go 1.27.3 darwin/arm64, Apple M2,
GOMAXPROCS=8, with `-p 1 -benchtime=300ms -count=3` (cgo disabled):

| Workload | Three samples, ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| indexed | 45,533 / 44,521 / 46,575 | 95,936–95,937 | 292 |
| boxed-phantoms | 87,547 / 82,844 / 80,857 | 193,794 | 509 |

The twenty independent reference images achieved minimum per-color IoU
`.990031` and maximum whole-image RGB MAE `.013065` bytes. Exact old root
path/pixel captures and the existing eight legacy control-point/metric fixtures
passed. These observed results are specific to this slice; the focused command
results and source hashes are recorded under
`tests/data/mathtex/radical_boxes/validation.json`.

Optional preview and independent regeneration commands:
Use a new/empty preview destination; the existing exporters reject overwrites.

```sh
GOMATH_RADICAL_BOXES_PREVIEW_DIR="$PWD/tests/data/mathtex/radical_boxes/preview" \
  CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex \
  -run '^TestRadicalBoxWriteTransformSnapshotsAndPreview$' -count=1

DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib:/opt/homebrew/opt/cairo/lib \
  PYTHONDONTWRITEBYTECODE=1 python3 tests/reference/mathtex_radical_boxes.py \
  --gofont-dir /path/to/golang.org/x/image/font/gofont
```

The legacy capture tool under `tests/reference/_radical_legacy_capture` refuses
to regenerate after the feature is present. Its output pins prior behavior;
it is not an independent reference. Existing checkpoints and prior whole-repo
results do not certify this slice.
