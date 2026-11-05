# Third-party notices

## Manim Community

The migration design and behavioral reference are based on Manim Community
0.22.1 at commit `16ea443e2a5f1d79db0a721161411ce916d6e733`.
Relevant upstream MIT notices are retained in:

- `LICENSES/manim-LICENSE.txt` (3Blue1Brown LLC)
- `LICENSES/manim-LICENSE.community.txt` (Manim Community Developers)

No Python runtime or upstream native dependency is embedded in the library.

## Go modules

Pinned versions are in `go.mod` and verified by `go.sum`:

- `golang.org/x/image`: BSD-3-Clause, Go Authors.
- `golang.org/x/text`: BSD-3-Clause, Go Authors (transitive).
- `golang.org/x/sys`: BSD-3-Clause, Go Authors (module graph; optional SIMD CPU
  detection is excluded from the strict `noasm` vector build).

Keep the corresponding module license notices when vendoring or distributing
their source. Go standard library code is provided by the Go toolchain.

## Go fonts

The embedded regular, bold, italic and bold-italic text faces and regular
monospaced code face come from `golang.org/x/image/font/gofont` packages
`goregular`, `gobold`, `goitalic`, `gobolditalic` and `gomono`. These fonts were
created by Bigelow & Holmes for the Go project. The complete accompanying
BSD-style font copyright and license notice is retained in
`LICENSES/go-fonts.txt` and must accompany redistributed embedded font data.
No system fonts are copied into this repository.

## Gomath Set Symbols and Gomath Math Alphabets

`mathtex/set_symbols_data.go` and `mathtex/math_alphabets_data.go` contain
renamed vector subsets derived from STIXGeneral 1.1.0 regular/bold outlines
under SIL Open Font License 1.1. Mathematical alphabets use the regular source;
their derivative name is Gomath Math Alphabets, with 166 real double-struck,
Script and Fraktur glyphs. These font-data derivatives retain their own license
and source copyright notices in `mathtex/LICENSE-STIX.txt`; they are distinct
from the surrounding Go code's MIT
license. Source hashes, extraction tools and independent font references are
recorded under `tests/data/mathtex/set-symbols`, `tests/data/mathtex/alphabets`
and `tests/reference`. The font-fallback example additionally embeds the
three-character OpenType subset as `examples/scenes/assets/GomathDemoMath.otf`;
its role, hash and unchanged subset provenance are in the adjacent README, and
the accompanying original/derivative font license is retained in
`examples/scenes/assets/LICENSE-STIX.txt`.

## Formats and algorithms

SVG arc conversion follows the W3C SVG implementation notes. MP4 and H.264 use
independently written Go format/bitstream code.
