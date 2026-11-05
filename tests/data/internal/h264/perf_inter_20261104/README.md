# InterResidual performance capture, 2026-11-04

`baseline_sources.zip` and `baseline_files.json` were captured before production
edits. The archive hashes were rechecked after optimization. `optimized_files.json`
and `source_changes.diff` identify the final source/test/document changes. The
four golden manifests and the benchmark source are unchanged.

Environment and workload: `environment.json`. All benchmark runs use the central
runner, which supplies `-tags noasm`, with CGO_ENABLED=0 and GOMAXPROCS=1.
Baseline and optimized files each contain all 16 paired cases, five samples:

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkInterResidual$' -benchmem \
  -benchtime=300ms -count=5 -p 1
python3 tests/_cases/internal/h264/summarize_perf.py \
  tests/data/internal/h264/perf_inter_20261104
```

`benchmark_summary.json` retains individual samples, medians, minimum and maximum;
`benchmark_summary.md` is the compact table. B/op and allocs/op are **per60 frames**.
The validation document explains exclusions, sequential sampling noise and the
retained-memory tradeoff. Profiles are two-second diagnostic captures, separate
from the five-sample benchmark runs. Each uses this command form, replacing
absolute output paths and the selected case (static/searchfalse or
noisechange/searchtrue), before and after optimization:

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkInterResidual$/^static$/^searchfalse$/^intertrue$' \
  -benchmem -benchtime=2s -count=1 -p 1 \
  -cpuprofile=<absolute-output.cpu.pprof> -memprofile=<absolute-output.allocs.pprof> \
  -o=<absolute-output.test>
go tool pprof -top <output.cpu.pprof>
go tool pprof -top -alloc_space <output.allocs.pprof>
```

Profiles include setup allocations; use benchmark B/op for per-frame allocation
volume. Text tops were captured with each matching binary before removing the
temporary binaries. Go profiles retain function names for offline inspection.

Validation logs:

- `package_tests.txt`: full package, CGO_ENABLED=0, noasm, GOMAXPROCS1, coverage.
- `concurrent_tests.txt`: three repetitions of InterConcurrentSerialization,
  PredictiveConcurrentSerialization and InterWorkspaceLifetime at GOMAXPROCS8.
- `retained_capacity.txt`: 60-frame static workload and Reset capacity checks.
- `dependency_audit.json`: scoped noasm deps, 62 stdlib packages plus the codec;
  no external dependencies, native inputs, os/exec or plugin. Targeted vet passed.
- `fresh_dump_tests.txt` / `independent_decode.json`: freshly generated optimized
  InterResidual vectors, manifest byte-equal to the checked-in golden, decoded
  by the central development-only `validate_predictive.py` with strict FFmpeg.
  428 streams / 6,708 frames / 50 fresh IDR entries passed exact YUV comparisons.
  Large temporary streams/reconstructions were removed after this verification.

No normal Go test or runtime code uses the independent decoder or Python.
No whole-repository or race run was performed in this disjoint ownership slice.
