# GoMath development contract

The product objective is comprehensive Manim capability coverage in pure Go,
with measured improvements to performance and memory. This objective is not yet
complete. Do not reduce it to a demo or declare parity from similar API names.

## Runtime constraints

- No cgo, C/C++ libraries, dynamic native bindings, embedded Python, browser
  engines, or subprocess-based rendering/encoding/typesetting.
- Use portable Go dependencies; validate with `CGO_ENABLED=0` and `-tags noasm`.
- The Go toolchain is allowed for builds/development. Independent reference
  renderers/decoders are allowed as development-only validation, never fallback
  runtime implementations.
- This is a reusable numerical/graphics library, not an HTTP service. Keep web
  frameworks and database dependencies out of the core.

## Correctness and scope

- Preserve identity and existing child references during transforms.
- Authoring may mutate objects; sampling must use independent snapshots and be
  safe to call concurrently after authoring completes.
- Frame callbacks must be pure with respect to shared state. Document execution
  and timing semantics explicitly.
- Reject unsupported inputs clearly; do not silently discard visible content.
- Return errors for invalid input. Do not add empty API stubs to inflate coverage.
- Update `docs/PARITY.md` when adding capabilities. Only mark Manim behavior
  verified after a source-pinned, matched-scene comparison, including frames
  inside an animation rather than only its final state.
- The checked-in source ledger is an inventory, not a percentage-complete claim.

## Required checks

Keep every test source, benchmark, fixture, and reference-generation script under
`tests/`. Package tests live in `tests/_cases/<package>/` (root package: `root/`),
fixtures in `tests/data/<package>/`. Do not add `_test.go` or `testdata/` to
production directories. The centralized harness uses Go source overlays to retain
package-private access without copying files. See `tests/README.md`.
For focused tests and benchmarks use
`CGO_ENABLED=0 go run ./tests/cmd/run -pkg=<package> <go-test-flags>`.

1. `CGO_ENABLED=0 go test -tags noasm ./...`
2. `CGO_ENABLED=0 go vet -tags noasm ./...`
3. `CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck`
4. Run relevant benchmarks after hot-path changes and record workload, image
   size, frame count, versions, hardware, sample times and what is excluded.
5. For codec changes, validate complete decoding with an independent decoder
   when available. Pure-Go unit tests must not require that decoder.

Use the existing Manim source as a reference and preserve its MIT notices where
implementation is derived. Keep the reference Python repository unchanged.
