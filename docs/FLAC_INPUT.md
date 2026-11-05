# Strict native FLAC input

`audio.ReadFLAC(io.Reader, audio.FLACReadOptions) (*audio.PCM, error)` now accepts
byte-aligned signed PCM8/16/24/32 and 1..8 channels. The API and option fields
(`Limits`, `MaxWork`, `DiscardMetadata`) are unchanged. Runtime decoding uses Go
and the standard library. No native codec, subprocess, or external fallback runs.

This change expands the reader only. `WriteFLAC` still writes PCM16/24
mono/stereo with its existing bytes. The CLI and audio remix interfaces have
their own mono/stereo limits; an eight-channel decoded PCM does not imply an
automatic downmix or expanded CLI support.

## Sample and channel contract

Samples are exact signed integers divided by `2^(bits-1)`. All four supported
depths are exactly representable in float64, including PCM32's `-2147483648`
and `2147483647`, which become `-1` and `1-2^-31`. There is no clipping, dither,
channel conversion, or gain. Returned storage is independent of the input and
decoder scratch; separate calls and concurrent reads of returned PCM are safe.

`Format.ChannelMask` stays zero. Independent frame assignments 0..7 preserve
the RFC 9639 channel indexes below; the reader does not infer a speaker mask.
Explicit `WAVEFORMATEXTENSIBLE_CHANNEL_MASK` metadata remains unsupported.

| Channels | Ordered indexes defined by FLAC |
|---|---|
| 1 | mono |
| 2 | left, right |
| 3 | left, right, center |
| 4 | front left, front right, back left, back right |
| 5 | front left, front right, front center, back/surround left, back/surround right |
| 6 | front left, front right, front center, LFE, back/surround left, back/surround right |
| 7 | front left, front right, front center, LFE, back center, side left, side right |
| 8 | front left, front right, front center, LFE, back left, back right, side left, side right |

Assignments 8/9/10 restore left-side, side-right, and mid-side respectively and
require exactly two channels. Assignments 11..15 are reserved and rejected.
Channel count, bit depth, rate, and blocking strategy cannot change midstream.
Rate must be an integer in 1..768000 Hz. Non-byte-aligned PCM, floating point,
Ogg/ID3 wrappers, concatenated streams, and trailing data are unsupported.

Constant, verbatim, fixed predictors 0..4, LPC orders 1..32, partitioned
Rice/Rice2, escape width 0..31, and wasted bits are supported. Negative LPC
shifts, reserved precision/methods, invalid partitions, nonzero alignment bits,
and invalid numeric recovery return errors. Legal final blocks may be shorter
than STREAMINFO's nominal minimum; the existing blocking/sequence checks apply.

## Arithmetic and integrity

PCM32 stereo sides use 33 signed bits. The decoder carries subframe samples,
fixed/LPC predictions, and stereo restoration in int64, validating every
predicted sample at its effective bit depth before using it as history.
For LPC, at most 32 coefficients, signed precision at most 15, and samples at
most 33 bits bound the accumulator magnitude by `2^51`. Adding a residual
of magnitude less than `2^31` remains inside int64. Wasted bits are restored
only after successful effective-depth reconstruction; final stereo samples
must fit the original PCM depth. No wrapping conversion substitutes for checks.

Residuals, including those in 33-bit side subframes, must have absolute value
less than `2^31` (RFC 9639 §9.2.7.3). In particular, `-2147483648` is not a
legal residual. An encoder must use another predictor or verbatim when needed.
Rice2 handles the valid extrema ±2147483647; five-bit escape lengths permit
only signed widths up to 31. Input bit work and unary lengths are bounded.

Every frame's header CRC8 and frame CRC16, frame/sample number, format and
STREAMINFO block/frame bounds are validated. MD5 is computed over fully
restored, interleaved signed PCM, little-endian, exactly 1/2/3/4 bytes per
sample. Depth32 side samples are never hashed as five-byte values.
Known total sample counts must match. Zero total means count through EOF;
zero MD5 or an environment that disables MD5 returns `ErrUnsupported`.
Malformed input, checksum mismatch, truncation, I/O errors, or exhausted
budgets always return nil PCM, even after earlier frames decoded successfully.

The metadata policy is unchanged: STREAMINFO must be first and unique;
PADDING must be zero. APPLICATION, SEEKTABLE, VORBIS_COMMENT, and PICTURE
require explicit `DiscardMetadata=true`, and their structure is still checked.
CUESHEET, reserved metadata, and channel-mask overrides remain unsupported.
No picture pixels or linked resources are loaded.

## Bounds

`Limits` bounds bytes, metadata blocks, frames, and scalar samples. Declared
totals are checked before sample allocation and unknown totals before each
decoded block. All output capacity arithmetic divides limits by channels
before multiplying, avoiding `MaxFrames*channels` overflow with large custom
limits. Storage must also fit the target's `int` byte size, including on 386.

The encoded-input buffer is 4 KiB. A block is at most 65535 frames: eight
int64 channel buffers use at most 4,194,240 bytes, and the PCM32 interleaved
MD5 buffer at most 2,097,120 bytes. Metadata payloads have the existing 24-bit
length bound and are budgeted before allocation. PCM output grows within
frame/sample/storage limits; old and new arrays can coexist during a growth.
Default 16M scalar samples allow at most 128 MiB per output backing array.

`MaxWork=0` means `2^32` units. One encoded bit, one reconstructed scalar, and
one predictor multiply/add pair each cost one unit. This is a bounded algorithm
budget, not a timing guarantee. An exact encoded byte limit permits one extra
source byte probe to distinguish EOF from excess. Readers must eventually
return EOF; streaming callbacks, seeking, and cancellation are not provided.

## Independent evidence (2026-11-04)

The development generator is `tests/reference/audio_flac_input.py`; all
fixtures and raw evidence are under `tests/data/audio/flac_input/`.
`sources.json` pins the downloaded RFC 9639 text and relevant sections.

* 194 real libFLAC 1.5.0 streams cover all 32 depth/channel combinations:
  six patterns each (constant, extrema, random, wasted, sine, ramp), blocks
  16/256/4096/65535, short blocks and partial final blocks. There are 461,638
  scalar samples, with real fixed/LPC/Rice/Rice2/wasted-bit subframes.
* Complete pure-Go decoding matches every original scalar exactly and the
  original integer PCM hashes. Complete libFLAC raw decoding and FFmpeg 8.0.1
  decoding (`-xerror -err_detect crccheck+explode`) also match every scalar.
  FFmpeg output is signed int32, left-aligned exactly for smaller depths;
  there is no precision reduction, truncation, or channel-count conversion.
* libsndfile 1.2.2 / soundfile 0.13.1 independently matches all 144 8/16/24-bit
  streams. It explicitly refuses all 50 32-bit streams as unimplemented.
  These are recorded as unavailable comparisons, never replaced with 24-bit
  data and never reported as decoder agreement.
* 49 additional hand-built cases, each with inherited and explicit header
  depths (98 complete streams), match libFLAC and strict FFmpeg decoding.
  They cover every independent depth/channel pair, all three stereo modes,
  ±4294967295 sides, mid-side parity/signs, 32 wasted side bits, fixed 1..4
  at 33 bits, LPC 1/2/12/32, 15-bit coefficients and shift0/15, large signed
  products that cancel, residual extrema, and signed31-bit escape values.
  Ordinary tests require the exact independently decoded hand bytes.

The new deterministic tests also require nil on reserved/mismatched
assignments/depths, illegal side restoration, predictor overflow approaching
the accumulator bound, prohibited residual `-2^31`, invalid wasted depth,
CRC/MD5 corruption, every truncation of a small eight-channel stream,
trailing/concatenated input, and EOF I/O errors. Byte/frame/sample/work limits
are tested at the exact boundary and one below, including known/unknown
totals, large-limit multiplication, exact LPC work charging, independent
storage, and eight concurrent decodes.

## Validation commands

Run from the repository root. The central runner supplies `noasm` and resolves
fixtures through the existing audio TestMain; no environment override is
needed for ordinary regression.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -count=1
CGO_ENABLED=0 go vet -tags noasm ./audio
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=audio -race -count=1
CGO_ENABLED=0 GOMAXPROCS=2 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzReadFLACInput$' -fuzztime=20s -parallel=2
GOMAXPROCS=1 CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -bench='^BenchmarkReadFLAC(48kStereo|Input)$' -benchmem -benchtime=300ms -count=5 -p=1
python3 tests/reference/audio_flac_input.py --verify-only
```

The independent generator requires libFLAC/FFmpeg/soundfile/numpy only during
development. Omit `--verify-only` to regenerate the real streams; `--hand-only`
rechecks the hand cases. `GOMATH_FLAC_INPUT_DIR` overrides only the Go fixture
directory; `GOMATH_FLAC_HAND_DUMP` explicitly enables development hand exports.

For cross-compilation, first build the central runner for the host, then pass
the target environment to its child Go toolchain. All temporary artifacts
stay under tests; remove the two binaries afterward.

```sh
CGO_ENABLED=0 go build -tags noasm -o tests/reference/_flac_runner ./tests/cmd/run
GOOS=linux GOARCH=386 CGO_ENABLED=0 tests/reference/_flac_runner -pkg=audio -c -o=tests/data/audio/flac_input/audio-linux386.test
```

The 386 check compiles production and every central audio test with noasm; the
Linux binary is not executed on the ARM macOS host. Race instrumentation uses
CGO=1 because Go requires it; product/runtime checks use CGO=0.

Recorded results: complete CGO0/noasm audio suite 2.852 s, 96.6% statement
coverage (ReadFLAC 93.2%); complete audio race 40.389 s; final input-only race
1.491 s; vet passed. The 20 s input fuzz run passed after 86,324 executions
(21.012 s including shutdown). Linux/386 production and all audio overlay
tests compiled successfully. The CGO0 dependency audit found 90 packages,
all standard library except `gomath/audio`, with no cgo files or `os/exec`.
These are focused audio checks; the parent owns whole-repository validation.

## Measured costs

Fresh before/after raw files are `before-bench.txt` and `after-bench.txt`, with
source hashes in `baseline.json` and medians in `benchmarks.json`.
Apple M2, Go 1.27.3, darwin/arm64, CGO=0/noasm, GOMAXPROCS=1, `-p=1`, five
300 ms samples. Preparation, file I/O, and independent decoders are excluded.
Other agents share this host; unfiltered ranges remain in the JSON evidence.

| Existing one-second stereo sine | Before median | After median | Change | After B/op | allocs/op |
|---|---:|---:|---:|---:|---:|
| PCM16 | 5.326 ms | 5.509 ms | +3.44% | 2,879,783 | 19 |
| PCM24 | 7.760 ms | 8.126 ms | +4.72% | 2,887,975 | 19 |

This expansion shows a small time regression on these measurements and no
allocation-count change; it is not a performance improvement claim. The old
bytes and sample values remain unchanged.

| New input | Median | B/op | allocs/op |
|---|---:|---:|---:|
| PCM8, 8ch, 1025 frames, block256 | 0.273 ms | 194,900 | 24 |
| PCM16, 8ch, 1025 frames, block256 | 0.370 ms | 196,948 | 24 |
| PCM24, 8ch, 1025 frames, block256 | 0.507 ms | 198,996 | 24 |
| PCM32, 8ch, 1025 frames, block256 | 0.846 ms | 201,044 | 24 |
| PCM32, 2ch, 65535 frames, block65535, wasted bits | 8.212 ms | 2,625,880 | 15 |

Input patterns and block sizes differ, so these rows do not isolate a bit-depth
cost or predict arbitrary music performance. Complete test/race/fuzz/386 logs,
coverage, preservation hashes, and independent results are in the same data
directory. Writer, AU/WAV/AIFF implementation files and old FLAC fixtures are
preserved; no shared CLI or parent-owned docs are edited by this slice.
