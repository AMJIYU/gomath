# Microsoft WAVE IMA ADPCM reference evidence

API and codec scope are frozen for this slice. `ReadWAV` and its options/limits
are unchanged. Production changes: `audio/wav.go`, new `audio/wav_ima.go`, and
`audio/README.md`. Test sources/benchmarks: `tests/_cases/audio/wav_ima_test.go`.

Reproduce independent references from the repository root:

```sh
python3 tests/reference/audio_ima_adpcm.py
python3 tests/reference/audio_wav_regression.py
CGO_ENABLED=0 env -u GOMATH_IMA_REFERENCE_DIR -u GOMATH_AIFF_REFERENCE_DIR go run ./tests/cmd/run -pkg=audio -count=1
```

Only the development scripts require FFmpeg/libsndfile/Python packages. Normal
tests neither invoke nor require them; they read `manifest.json` and retained
`.wav` / signed little-endian `.s16le` files. The fixture path is found from the
module root, independently of package cwd; `GOMATH_IMA_REFERENCE_DIR` can override.

37 streams: Go checks all 287069 returned scalar samples against the 1994
Microsoft separate-shift oracle. That oracle's complete block-capacity output
(292300 samples) is exactly equal to libsndfile 1.2.2's complete decode.
All 37 inputs are also completely decoded using FFmpeg 8.0.1 with strict errors.
FFmpeg uses merged multiplication instead of separate shifts: its entire PCM
matches a separate multiplication oracle, but 31 inputs differ from the specified
arithmetic (121887 capacity samples; largest observed code difference 32).
This observed difference is not a guarantee for arbitrary inputs.

For 17 fact-trimmed inputs, both external decoders output padded block capacity.
Full output hashes/lengths are recorded before separately checking the fact-sized
prefix; no FFmpeg trim/duration argument hides the difference. Only four inputs
have both complete FFmpeg length and PCM exactly equal to Go. These caveats are
intentional and must not be described as universal FFmpeg byte equality.

`_pcm_float_regression/verification.json` separately confirms all 15 existing
PCM16/24/32 and float32/64 mono/stereo/5.1 writer outputs decode completely and
exactly with FFmpeg (135 scalar samples). The existing PCM/float fact rule stays
exact equality; only IMA can trim the last block. Other WAV/FLAC/AIFF tests remain.

The bounded subset rejects partial final blocks, 3-bit DVI, more than two channels,
other ADPCM variants, extensible compression GUIDs and unknown chunks. A required
four-byte fact may trim only the last complete block; all coded capacity is
charged to Limits and all block headers are checked. Metadata remains opt-in.

`summary.json`, full/no-env test log, race/fuzz logs, dependency audit and coverage
record validation. `bench.txt` has five raw samples per bounded workload;
`benchmark_summary.json` records parameters/exclusions. Allocation volume is not
peak memory. `before_hashes.json` and `source_hashes.json` record the slice scope.
No root/CLI/export/H264 files or go.mod were changed by this slice.
