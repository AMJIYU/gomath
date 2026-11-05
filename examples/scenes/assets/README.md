# Embedded demonstration font

GomathDemoMath.otf is the 2916-byte three-character (ℝ𝒜𝔄) STIX-derived
OpenType subset produced by tests/reference/mathtex_alphabets.py. Its original
source/table provenance is recorded in tests/data/mathtex/alphabets/reference.json.
The font data is unchanged from that independently validated subset and retains
its distinct Gomath internal family name. The filename here describes its runtime
role: the font-fallback example embeds it to exercise actual missing-glyph fallback.
The example does not read tests/ or depend on installed fonts at runtime.

SHA256: `efc6436770e206cf9d2c3a10684f9cea19b2f5fb5b80a0cc881177464acdc92f`

The derivative and original copyright/license notices are in LICENSE-STIX.txt.
This tiny subset is demonstration data, not a general mathematical font.
