# Independent WAVE fixtures

Created with FFmpeg 8.0.1 (Homebrew), using raw f64le input. No gomath WAVE
encoder was involved. The checked-in `fixtures.json` records input samples,
sample rate, channel count, speaker mask and SHA-256 for each output.

The five stereo files contain the same eight frames at 48000 Hz. Samples are
exactly representable in all five encodings (including integer PCM16).
`pcm24_surround.wav` has two frames at 44100 Hz and 5.1 layout, speaker mask 63.

Generation command pattern (raw input is packed IEEE float64 little-endian):

```sh
ffmpeg -nostdin -v error -y -fflags +bitexact \
  -f f64le -ar 48000 -ac 2 -i stereo.f64le \
  -map_metadata -1 -flags:a +bitexact -c:a pcm_s16le pcm16_stereo.wav
```

Other codecs: `pcm_s24le`, `pcm_s32le`, `pcm_f32le`, `pcm_f64le`.
For surround input use `-ar 44100 -ac 6 -channel_layout 5.1`.
FFmpeg includes its LIST/INFO encoder metadata even with this command;
fixtures intentionally retain it. Tests assert the default reader rejects
metadata loss, then explicitly opt into `DiscardMetadata` to decode audio.

All normal tests use these committed files and standard Go only. FFmpeg is
development-only, never a runtime dependency or decoder fallback.

## FLAC writer validation

`flac/` contains six small **gomath-encoded** regression streams independently
fully decoded with FFmpeg 8.0.1, not FFmpeg-encoded reference files. They cover
one-sample silence, stereo extrema, order-4 prediction, verbatim random samples,
one-bit escaped residuals, and a short final stereo frame. `fixtures.json`
records encoded and independently decoded PCM SHA-256 and stream parameters.
Ordinary tests inspect these files using Go only.

`flac/verification.json` records the complete 2026-11-04 independent decode
matrix: 130 native FLAC streams, 2,077,648 scalar samples compared exactly.
Expected PCM originates from seeded integer generators and explicit rounding
codes, not from the production encoder or test reader. The record includes all
encoded/decoded hashes; only the six small fixtures are retained here.

Reproduce from the repository root (development dependencies only):

```sh
python3 tests/data/audio/verify_flac.py --keep /absolute/temporary/directory
```

The tool runs `CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio` with only the
artifact-producing test selected, then invokes independent ffprobe and FFmpeg.
It fully decodes every stream with CRC checking and compares signed little-endian
PCM bytes, exact lengths, format, MD5 and hashes. No external process is used by
`audio.WriteFLAC` or ordinary tests. Without `GOMATH_FLAC_REFERENCE_OUT`, the
artifact test skips; the checked-in regression-fixture test still runs.

TestMain centralizes all test/fuzz working directories here so any Go fuzz
failure corpus stays under `tests/data/audio/testdata/fuzz/`, never `audio/`.

## Independent libFLAC inputs for ReadFLAC

`flac_decode/` contains eight small files encoded by libFLAC 1.5.0 directly
from independently generated signed integer PCM. These did not use gomath's
writer. `manifest.json` records the original PCM and encoded file SHA-256,
format and length. FFmpeg 8.0.1 and the pure Go ReadFLAC each completely decoded
the files to the same integer PCM bytes. Ordinary Go tests need neither tool.

`flac_decode/verification.json` records 132 independently encoded inputs and
1,220,112 compared scalar samples, plus libFLAC analysis output features.
The real encoder selected constant/verbatim/fixed/LPC, Rice/Rice2, multiple
partitions, wasted bits, independent/left-side/right-side/mid-side stereo and
LPC orders 3..12. Test-only constructed bitstreams additionally cover LPC 1/2/32,
escaped residuals, unusual header codes, variable blocks and invalid inputs.

```sh
python3 tests/data/audio/verify_flac_read.py --keep /absolute/temporary/directory
# Append --retain-fixtures only when intentionally updating committed fixtures.
```

The tool encodes with the independent `flac` CLI using levels 0/5/8 and an
additional maximum-LPC-order-32 setting, fully decodes with FFmpeg, and selects
the Go-only fixture test through the focused noasm overlay runner. Inputs carry
Vorbis metadata deliberately: default decoding must reject its loss, and
`DiscardMetadata: true` is required for the exact PCM comparison.

## Exact channel-matrix oracle

`remix_reference.json` is generated independently with Python's standard-library
`fractions.Fraction` by `generate_remix_reference.py`. No gomath code is called.
All values are dyadic and exactly representable as float64, so Go tests compare
every output sample exactly, without tolerance. Cases contain each input-channel
impulse, asymmetric/negative weights, explicit LFE/surround contributions, channel
expansion, zero rows and headroom beyond [-1,1]. These are caller-chosen matrices,
not a claimed broadcast downmix standard. Ordinary tests read JSON using Go only.

```sh
python3 tests/data/audio/generate_remix_reference.py
```

This script updates only `remix_reference.json`; all WAV/FLAC fixtures are preserved.
