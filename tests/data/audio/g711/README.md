# Frozen G.711 / WAVE decode slice

`ReadWAV(io.Reader,WAVReadOptions) (*PCM,error)` and existing options/Limits remain
unchanged. Supports direct tags 6/7, 8-bit A-law/mu-law, 1..64 indexed channels
in file order, ChannelMask=0, and declared rates 1..768000 Hz. No writer added.

Strict subset: exactly 18-byte fmt, cbSize=0, blockAlign=channels, byte rate
rate*channels, exactly one four-byte fact with count dataBytes/channels (including
empty data), no incomplete frame or tail trimming. All RIFF pads must be present,
in bounds and zero for these formats. Older PCM/IEEE/IMA pad/fact rules stay as
before. Metadata remains explicit opt-in. Errors never return partial PCM.

Legacy fmt16, unknown extensions, extensible compression GUIDs/speaker layouts,
other compression and unknown chunks are explicit errors. Real FFmpeg six-channel
encoder outputs have extensible GUID/layout and are preserved as rejection
fixtures; no layout stripping is used to manufacture support.

`audio/README.md` documents scope, signed16/32768 scaling, A-law's lack of an exact
zero, and both mu-law zero codes producing positive float64 zero. Tables are
initialized once (4096 shared bytes); each call owns PCM plus 32KiB scratch.

## Independent references

```sh
python3 tests/reference/audio_g711.py
CGO_ENABLED=0 env -u GOMATH_G711_REFERENCE_DIR -u GOMATH_IMA_REFERENCE_DIR -u GOMATH_AIFF_REFERENCE_DIR go run ./tests/cmd/run -pkg=audio -count=1
```

Only the development script requires Python soundfile/numpy and independent
FFmpeg/libsndfile. Normal Go tests neither invoke nor require external programs.
The fixture directory is found from go.mod regardless of test cwd. Set
GOMATH_G711_REFERENCE_DIR to explicitly override it.

`manifest.json` records standards URL/SHA256, tool versions, all encoded and full
decoded hashes. All 42 supported streams (207898 scalar samples) exactly match
the independent ITU-T G.711 interval-midpoint oracle, FFmpeg8.0.1 and
libsndfile1.2.2, for complete samples AND lengths. No trim/duration parameters,
sample tolerance or hidden trailing data are used. Every code in each law is
covered in every indexed channel of the all-code fixtures. No arithmetic decoder
disagreement was observed.

There are also 16 rejected references, all accepted by the external tools:
missing/short/long fact, legacy fmt16, inconsistent cbSize, nonzero pad,
extensible GUID and the two actual multichannel encoder outputs. External
acceptance and full-output lengths/hashes are recorded rather than silently
weakening this decoder's policy.

Existing PCM/float independent regression was rerun without changing its script
or IMA data, directing output to `_pcm_float_regression/`:

```python
from pathlib import Path
import runpy
ns = runpy.run_path("tests/reference/audio_wav_regression.py")
ns["main"].__globals__["OUT"] = Path("tests/data/audio/g711/_pcm_float_regression").resolve()
ns["main"]()
```

Its 15 complete WAV streams / 135 scalar samples exactly match FFmpeg across all
five prior encodings and mono/stereo/5.1; all existing IMA/AIFF/FLAC fixtures remain
unchanged and pass the complete audio suite.

## Validation and bounded benchmark

Full audio CGO0/noasm test log: `full_audio_tests.txt` (1.241s,95.8% coverage).
New G.711 source: 38/38 covered statements. Full audio race: `race.txt` (9.286s;
CGO1 solely for Go race instrumentation). `fuzz.txt`:20s,797240 executions, PASS.
Package vet and CGO0/noasm Linux/386 audio build passed. `dependency_audit.json`
records 89 stdlib packages+audio and no external/cgo/audio assembly/exec/plugin.

Test/benchmark source is only in `tests/_cases/audio/wav_g711_test.go`.
Five serial300ms benchmark samples per case are in `bench.txt` and summarized
in `benchmark_summary.json`. Go1.27.3/darwin-arm64/AppleM2/macOS15.7.1,
CGO0/noasm/GOMAXPROCS1/-p1. Each op fully decodes one second48000Hz/48000frames,
1/2/6 channels, with code byte=(frame+37*channel)mod256. Includes validation,
reads, mapping, PCM/scratch allocation; excludes construction, disk, independent
codecs and startup/table initialization. Medians0.0394..0.1922ms/op,14allocs/op,
418024/803048/2343144 B/op depending on channel count. Shared machine observations;
allocation volume is not peak/RSS, and no cross-decoder speedup is claimed.

`summary.json` and before/final source hashes record the frozen delivery scope.
Production edits: `audio/wav.go`, new `audio/wav_g711.go`, `audio/README.md`.
New test, reference script and all data are centralized under tests. No CLI,
export, H264, root production files, old test sources or go.mod were modified.
