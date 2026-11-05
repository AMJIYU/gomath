# Bounded vector primitives

`Ellipse`, `Annulus`, `Sector`, `AnnularSector`, `RoundedRectangle`, `DashedLine`
and `DoubleArrow` return `(*Object, error)`. They construct real editable vector
paths/children and participate in identity-preserving transforms, raster/SVG
export, grouping and vector animations.

Dimensions/radii are bounded by 1e12; positive dimensions are at least 1e-9.
Inputs must be finite. Constructors reject invalid domains before allocating
large geometry. They are XY-plane shapes except that fixed dashes can follow a
3D straight segment. Z is still paint ordering in the ordinary vector renderer.

| Constructor | Meaning / boundaries |
| --- | --- |
| `Ellipse(width, height)` | Full dimensions, origin centered; four cubic arcs. |
| `Annulus(inner, outer)` | Opposite closed winding creates a real nonzero-fill hole; inner=0 is a disk. No radial seam in the stroke. |
| `Sector(radius, start, sweep)` | Zero-inner-radius disk sector; angles in radians. |
| `AnnularSector(inner, outer, start, sweep)` | 0 ≤ inner < outer; 0 < abs(sweep) ≤ 2π. Signed sweeps; partial sectors include radial edges. Full turns use independent closed contours. |
| `RoundedRectangle(width, height, radius)` | Four quarter-circle cubic corners; radius in [0,min(width,height)/2], zero is an ordinary rectangle. |
| `DashedLine(start, end, dashLength, paintedRatio)` | Fixed world-space period dashLength/paintedRatio, ratio in (0,1], ≤4096 painted children; last dash may truncate or be followed by a gap. Butt caps preserve the chosen gaps. |
| `DoubleArrow(start, end, tipSize)` | A shaft and two filled triangular tips. Endpoints share Z; tip size strictly below half the full length. |

Cubic circle/ellipse arcs approximate conics. DashedLine explicitly uses a fixed
period, rather than Manim's fitted dash distribution or a general SVG dash-array
stroke. It does not implement dashing arbitrary curves. These distinctions are
part of the API, not silent rendering fallbacks.

Pinned Manim 0.22.1 raster evidence covers ellipse, annulus, a clockwise annular
sector, rounded rectangle and a disk sector together at 640×360, frame width 10,
white fill/black background/no stroke. The measured MAE is 0.057326/255, maximum
33/255, and fraction of pixels differing by >1 is 0.007253. Project regression
limits are MAE .5, maximum64 and fraction .025. The source uses a different arc
subdivision; the result is bounded visual evidence, not exact control-point or
API compatibility. Generator, image, hashes and provenance are under `tests/`.
Other tests verify holes, negative sweeps, endpoints, storage bounds and retained
child references. `primitives` is the runnable integration scene.
