# FLAC reader performance — wave six (2026-11-05)

This is a focused CPU change to `audio/flac_read.go`. `ReadFLAC`, its options,
samples, errors, resource limits, and supported formats remain unchanged.
The reader still accepts byte-aligned PCM8/16/24/32 and independent indexed
1..8 channels, with stereo assignments 8/9/10 restricted to two channels.
Writer, AU/WAV/AIFF, remix, CLI, and other modules are outside this change.

The starting reader hash is
`eecb5bffe0814a3edfe73a7f9fa0677ee9bc2b657090a0d45b7b41d6b8f26325`, matching
the frozen fifth-wave checkpoint. That checkpoint validates the old source
only. All performance and correctness evidence here is a fresh sixth-wave
run under `tests/data/audio/flac_perf/`; the fifth-wave fixtures, manifests,
reference records, and checkpoint are read-only inputs and are preserved.

## Profile and change

Before editing production, a fresh CGO0/noasm/GOMAXPROCS1 two-second CPU and
allocation profile of the existing one-second stereo benchmarks identified
`flacDecoder.get` as the useful hotspot. Its cumulative sample time was 1.65 s
of 5.23 s total. The eight-step CRC16 loop alone accounted for 1.19 s at the
loop/body lines. The shared macOS host also recorded 2.45 s in `runtime.kevent`;
these samples were retained rather than treated as useful decoder CPU.
Raw profiles and annotated listings are saved. Allocation profiling identified
the existing PCM/scratch allocations; this change does not pool or alias them.
The CRC loop already existed before wave five: this investigation identifies
a useful pre-existing cost, not the cause of the earlier small regression.

The only production algorithm change replaces eight polynomial long-division
steps per consumed frame byte with a single 256-entry uint16 remainder table:

```text
next = (previous << 8) XOR table[byte(previous >> 8) XOR inputByte]
```

The unreflected polynomial stays `0x8005` (`x^16+x^15+x^2+1`), with the same
initial value and no final XOR. The table is built once in pure Go, then only
read, so concurrent decoders need no lock. It adds 512 bytes of shared table
storage and 2048 fixed initialization iterations. Process startup/table
initialization is excluded from the steady-state benchmark and is not claimed
to improve. There are no architecture-specific or native implementations.

CRC8 is unchanged. Byte fetching, buffering, EOF probes, error construction,
work charges, residual/unary limits, predictors, 33-bit side recovery, PCM
normalization, output growth, metadata, and MD5 are unchanged. CRC16 still
updates exactly once on fetching each enabled byte, before consuming that
byte's buffered bits, and is disabled at exactly the same footer boundary.

The candidate remained in a `_staging` folder until validation and this
document were ready. Go overlays tested it in the real audio package without
replacing production files or modifying another agent's work.

## Matched five-sample measurements

Apple M2, macOS arm64, Go 1.27.3, CGO_ENABLED=0, noasm, GOMAXPROCS=1,
`-test.cpu=1`, 300 ms per sample. A native central-runner build creates two
test binaries from the same central test sources: a pinned old source overlay
and the candidate/current reader. Input file I/O, input encoding, compilation,
external decoders, process startup and table initialization are excluded.

Five fresh old-only samples were saved first (`before-five.txt`). Five matched
rounds then alternated before/after and after/before process order, with one
sample of all nine workloads per process. No samples were discarded.
`interleaved.json` includes per-round measurements, source/test/binary hashes,
all paired deltas and ranges. Raw outputs are `interleaved-*-*.txt`.

| Input | Before median | After median | Median change | allocs/op |
|---|---:|---:|---:|---:|
| Existing PCM16 stereo sine, 48000 frames | 5.376 ms | 2.958 ms | −44.98% | 19 → 19 |
| Existing PCM24 stereo sine, 48000 frames | 7.958 ms | 2.421 ms | −69.58% | 19 → 19 |
| libFLAC PCM16 stereo correlated, 8193 frames | 0.691 ms | 0.470 ms | −31.96% | 27 → 27 |
| libFLAC PCM24 stereo LPC, 8193 frames | 1.395 ms | 0.575 ms | −58.78% | 27 → 27 |
| PCM8, 8ch sine, 1025 frames | 0.265 ms | 0.202 ms | −23.73% | 24 → 24 |
| PCM24, 8ch verbatim random, 255 frames | 0.261 ms | 0.051 ms | −80.42% | 21 → 21 |
| PCM32, 8ch sine, 1025 frames | 0.835 ms | 0.299 ms | −64.22% | 24 → 24 |
| PCM32 stereo wasted, 65535 frames | 8.208 ms | 4.380 ms | −46.64% | 15 → 15 |
| PCM32 stereo, 33-bit side LPC32, 64 frames | 9.318 µs | 4.816 µs | −48.32% | 13 → 13 |

The existing sine workloads use the unchanged fifth-wave benchmark and writer
input generator. The seven additional inputs are exact frozen fifth-wave
files, pinned by hashes in the new baseline ledger. Old PCM16/24 stereo B/op
remain approximately 2,879,783 / 2,887,975; tiny per-op rounding differences
are retained in the raw samples. There is no allocation reduction claim.

These observations recover the prior +3.44%/+4.72% regression on the measured
old workloads; comparisons use the fresh before source, not historical timing
as a new baseline. The largest gain is on byte-heavy verbatim input, where
CRC work dominates. Input patterns/block sizes differ, so percentages do not
predict arbitrary music, other machines, or overall library performance.
Other agents share this host. In particular the first correlated-PCM16 pair
showed only 0.4% improvement, and other first-round samples had large scheduling
noise. All five paired old-sine samples still improved; all raw noise remains.

## Correctness and independent regression

The new deterministic CRC test exercises all 65536 prior states with three
input-byte choices (196608 transitions), comparing the actual bit-reader path
to independent polynomial division. It also checks returned bits, byte count,
remaining work and bit availability. Separate flag/timing tests exercise both
CRC enables, partial-byte reads, and work exhaustion before fetching a byte.

All pre-existing checks are retained: frame CRC8/CRC16, PCM MD5, all supported
depths/channel counts, exact float normalization, 33-bit side/subframe history,
LPC32 cancellation/overflow, residual extrema/escape/wasted bits, invalid
formats/content, complete EOF, exact budgets and nil PCM on errors. The fixed
table is shared read-only; returned PCM and scratch remain per-call owned.

Fresh independent redecoding covers 306 frozen complete streams and 519805
scalar samples: 194 new libFLAC inputs, 98 hand-boundary streams, eight old
libFLAC inputs and six old writer fixtures. libFLAC 1.5.0 raw output matches
the original signed PCM hash/frame/channel count. FFmpeg 8.0.1 uses
`-xerror -err_detect crccheck+explode` and matches the complete exact int32
widening, with no trimming, depth reduction, or channel conversion. Both
pinned old and candidate Go binaries pass the full `TestReadFLAC*` and old
writer-fixture oracles. `reference.json` and logs are new sixth-wave evidence;
the tools do not regenerate any fifth-wave input or reference ledger.

Normal Go tests do not require either external decoder. Product imports and
options are unchanged; independent programs run only in the development tool.

## Reproduction and validation

Run from the repository root. The central runner supplies noasm and maps tests
from `tests/_cases/audio/`; no tests or artifacts are added to production.

```sh
python3 tests/data/audio/flac_perf/bench.py
python3 tests/data/audio/flac_perf/reference.py
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=audio -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./audio
CGO_ENABLED=0 GOMAXPROCS=2 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzReadFLAC$' -fuzztime=20s -parallel=2
CGO_ENABLED=0 GOMAXPROCS=2 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzReadFLACInput$' -fuzztime=20s -parallel=2
```

`bench.py` uses a source overlay for the old snapshot, never swaps production
files, and builds both binaries with CGO0/noasm. Before promotion it accepted
`--after-source tests/data/audio/flac_perf/_staging/flac_read.go` to measure the
candidate. `before.go.txt` is the old production snapshot; the `.txt` extension
keeps it out of package discovery. All native/cross test binaries stay in
`tests/data/audio/flac_perf/_bin/`, an ignored-by-discovery directory.
Their hashes are retained in `builds.json`; generated binaries and temporary
overlays are removed at delivery. Run `bench.py` before `reference.py` to
recreate the native binaries when reproducing the complete evidence.

For 386, build the central runner for the host and then set the child Go
toolchain's target. The Linux binary is compiled, not executed on this ARM
macOS host; race instrumentation separately requires CGO=1.

```sh
CGO_ENABLED=0 go build -tags noasm -o tests/data/audio/flac_perf/_bin/runner ./tests/cmd/run
GOOS=linux GOARCH=386 CGO_ENABLED=0 tests/data/audio/flac_perf/_bin/runner -pkg=audio -c -o=tests/data/audio/flac_perf/_bin/audio-linux386.test
```

Fresh validation logs and the final source/fixture-preservation hashes are
recorded in `tests/data/audio/flac_perf/delivery.json`. Whole-repository
integration checks remain the parent agent's responsibility.

The identical staged source passed complete audio CGO0/noasm tests (2.525 s),
complete audio race (41.958 s), vet, and Linux/386 compilation of production
plus all central audio tests. Both 20-second fuzz targets passed: existing
`FuzzReadFLAC` executed 12829 inputs, and expanded `FuzzReadFLACInput` 192275
inputs. Both logs include later zero-execution intervals during fuzz-worker
processing/shutdown; they are retained, not represented as constant throughput.
These checks preserve the old invalid-input, checksum and resource semantics.
After promotion the same source hash is checked again through the normal
central test entry point without a source override (complete audio passed in
2.780 s; final plain-path vet also passed). Hash comparison preserves the
other 791 original source/fixture/metadata inputs, including go.mod/go.sum,
old audio implementation files and all fifth-wave inputs/reference records.
The CGO0/noasm dependency audit remains 90 packages, standard library plus
`gomath/audio`, with no cgo files or `os/exec` in the product dependency closure.
