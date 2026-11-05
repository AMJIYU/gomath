# H.264 development validation

Run deterministic correctness tests through the central overlay runner:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -count=1 -p 1
```

All ReferenceVectors, including the full motion/QP/radius/recovery matrix, run
normally. Their verified decoder hashes require no external programs. Race
runs retain the same correctness matrix; there is no race-specific skip.

`TestMotionMeasurements` only collects/logs size and PSNR for 540 large frames.
It has no correctness assertions and is explicitly opt-in for development:

```sh
CGO_ENABLED=0 GOMAXPROCS=1 GOMATH_H264_MEASURE=1 \
  go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestMotionMeasurements$' -v -count=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkMotionSearch$' -benchmem \
  -benchtime=300ms -count=5 -p 1
```

Independent strict decoder validation is development-only. Generate fresh
vectors with `GOMATH_H264_MOTION_DUMP_DIR=<absolute-temporary-directory>`, then
run `validate_predictive.py <directory> --manifest motion_vectors.json --all-idr`.
It checks every stream and every IDR-entry suffix against internal reconstruction.
Existing historical raw data are preserved under `tests/data/internal/h264/`.

The byte-identical motion optimization capture is in
`tests/data/internal/h264/perf_motion_20261104/`, including fresh before source
hashes/archive, all five raw before/after benchmark samples, CPU/allocation
profiles, full regression logs and strict independent-decoder results. Rebuild
its comparison table without overwriting historical motion captures:

```sh
python3 tests/_cases/internal/h264/summarize_motion_perf.py \
  tests/data/internal/h264/perf_motion_20261104
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkMotionSyntax(Writer|Counter)$' -benchmem \
  -benchtime=300ms -count=5 -p 1
```

The syntax counter is exact and differentially checked against the original
writer, including nonzero contexts. Search traversal, costs and ties are
unchanged. See `internal/h264/VALIDATION.md` for workload boundaries, shared-host
measurement caveats, all nine cases, and verification details. No new full race
run is claimed by this focused capture.
