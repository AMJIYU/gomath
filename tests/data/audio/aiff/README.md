# AIFF/AIFC independent references

Development-only generator: `tests/reference/audio_aiff.py`. Runtime and ordinary
Go tests use no FFmpeg, Python, subprocess or cgo.

`manifest.json` contains62 retained small inputs, original integer codes,
encoded SHA256 and expected normalized PCM/s32le SHA256. FFmpeg8.0.1 and
CPython3.11.5 stdlib `aifc` encoded the inputs independently of gomath.
`verification.json` records all77 inputs/493662 scalar samples, versions,
source URLs/SHA256 and complete independent output hashes. Larger inputs were
discarded only after both independent and Go verification.

Of77 inputs,15 have real FFmpeg CHAN speaker-layout chunks. ReadAIFF explicitly
rejects these with either metadata setting; they were not stripped. The other
62 streams/165432 scalar samples were checked exactly with ReadAIFF.61 supported
streams and the15 CHAN inputs have byte-identical complete strict FFmpeg s32le
output versus original integer codes. One normative unused-storage case differs
in FFmpeg output length, as described below. There is no claim of supporting all
AIFF compression or metadata variants.

`spec_COMM_bounds_unused_SSND.aiff` has17 PCM8 frames and one unused storage
byte inside SSND. COMM is authoritative by the Apple specification. FFmpeg
decodes the unused byte too (one extra sample, four extra s32le bytes). Its whole
output hash/extra length are preserved separately. No output trim or duration
limit was used. CPython's independent reader parses COMM and reads exactly its
own declared frame count; its samples and ReadAIFF match the original codes.
This is a decoder limitation record, not a silently changed golden.

The default fixture test locates `go.mod` upward and uses the absolute
`tests/data/audio/aiff` path; it works from the repository, audio package, or
central data directory. `GOMATH_AIFF_REFERENCE_DIR` explicitly overrides it for
the larger temporary development matrix. Default tests need no environment.

```sh
CGO_ENABLED=0 env -u GOMATH_AIFF_REFERENCE_DIR \
  go run ./tests/cmd/run -pkg=audio -count=1
python3 tests/reference/audio_aiff.py
```

The generator needs Python3.11/3.12 (`aifc` was removed in3.13) and FFmpeg.
Apple AIFF1.3/AIFF-C normative source fingerprints are in verification.json;
full specification PDFs are not vendored. COMM/SSND ordering, offset and text
examples are independently assembled from those structures and CPython PCM.

Validation logs: complete audio `package_tests.txt` (95.1% coverage), explicit
no-env full suite `no_env_package_tests.txt`, default and overridden fixture
checks, AIFF race,20s fuzz(787101 executions), vet, linux/386 build and scoped
CGO0/noasm dependency audit. `coverage_summary.json` separates new AIFF99.5%
from complete audio95.1%. The remaining AIFF branch is a32-bit storage bound
unreachable on this64-bit host. No whole-repository validation is claimed here.

`bench.txt` holds five300ms samples for each depth/order: one second of48000Hz
stereo/96000 scalar samples, CGO0/noasm/GOMAXPROCS1/-p1 on AppleM2/darwin-arm64,
Go1.27.3/macOS15.7.1. Inputs are prebuilt; parsing, sample conversion, output
allocation and optional SSND-first buffering are included. Disk/encoder/external
decoder/startup are excluded. Summary tables retain all individual samples and
their ranges. B/op is total allocations, not peak memory or RSS. Timings are
shared-machine observations; SSND-first costs more CPU/storage than COMM-first.

Only `audio/aiff_read.go` and `audio/README.md` production/documentation files
were added/edited in this slice. Existing audio Go sources and WAV/FLAC fixtures
are preserved. The new central test and reference script own this directory.
