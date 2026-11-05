# Pinned Manim raster reference

`manim-reference-0.22.1.png` was rendered by the local Manim Community 0.22.1
checkout at commit `16ea443e2a5f1d79db0a721161411ce916d6e733`, using Cairo 1.18.4.
The geometry is the `examples/scenes.Reference` workload: 100 exact cubic circles,
640x360 pixels, a black background, frame width 14.2222222222, fill (0.2,0.6,0.8,0.7)
and white stroke of 0.025 world units. Full harness, timing and image error data
are in `../../../docs/benchmarks/2026-11-03/`.

SHA-256: `808a8f56d8a7e91cdf6f572fd319d3c80183051e419ba9e90cd4a6a3f9d94372`.

The regression test enforces a bounded visual difference. It does not establish
pixel identity or Manim API/animation compatibility. The golden is development
test data; ordinary Go tests only decode PNG using the Go standard library.
