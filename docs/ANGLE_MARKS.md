# Angle and elbow marks

`NewAngle(line1, line2, options)` reads two infinite directed lines from ordinary
single-line Objects; `NewAngleFromPoints(A,B,C,options)` constructs angle ABC.
`NewRightAngle` draws an elbow. Use the returned `AngleMark.Object` for styling,
scene membership and animation. The lines are neither changed nor retained.
`Vertex`, `Radius`, `Sweep` and `Value(degrees)` describe construction only;
subsequent edits or animation of Object do not recompute these values.

Zero radius selects the pinned Manim rule: take the smaller distance from the
intersection to the endpoint selected by each quadrant sign, use two thirds of
that distance below .6, otherwise .4. Zero quadrant means `{1,1}`; explicit
entries are ±1. Positive signs follow each line's start-to-end direction. The
ordinary arc runs counterclockwise in `(0,2π)`; OtherAngle chooses the negative
clockwise complement. Elbows permit oblique lines, as Manim's RightAngle does:
they are a visual assertion by the author, not a perpendicularity check.

Dot radius and distance default to radius/10 and .55. DotColor is explicit:
`DefaultAngleOptions()` supplies white; a zero color is transparent. Dot position
follows the bounding-box center of the arc's nine anchors, matching the pinned
VMobject boundary calculation rather than a mathematical angle bisector. The
mark and optional dot each use eight tangent cubic segments. Existing ordinary
Arc/Circle constructors keep their earlier subdivision and output.

Inputs must lie in the XY plane, with finite coordinates at most 1e12 in absolute
value and line lengths at least 1e-9. Object inputs must contain one open cubic
with uniform line handles, no children and no bitmap. Style/clip are ignored
when reading the line's geometry. Exactly parallel/collinear directions produce
an empty mark with Parallel=true and zero metadata, matching the empty-object
behavior in pinned Manim. Nonparallel direction sine at most 1e-12, an out-of-
bounds intersection, an automatically selected radius below 1e-9, or invalid
output geometry fails explicitly. An explicit valid radius can handle a selected
endpoint at the intersection. Near-zero anchor-center directions for dots fail
instead of amplifying an unstable normalization. Elbows reject Dot/OtherAngle.
Zero radius/dot distance mean defaults, so explicit zero-size marks or zero dot
distance are not available. Arbitrary 3D planes, automatic live line tracking
and the complete Python Angle API remain pending.

All checks and references are under tests/. The source-pinned generator
`tests/reference/angle_marks.py` records 64 constructions and 448 linear Transform
states, including animation interiors. Every root and dot cubic control point
matches Manim CE0.22.1 commit16ea443e2a5f1d79db0a721161411ce916d6e733 within
2e-12. These comparisons cover quadrant signs, automatic/explicit radii,
clockwise/reflex arcs, dots, elbows and intersections outside the input segments;
they do not establish full updater, style, stroke raster or API equivalence.

The `angle-marks` CLI example exercises Create, grouped marks/dots, a rotating
pair of opposite signed arcs and an indicated four-quadrant elbow group.

2026-11-04 construction benchmark: Go1.27.3, darwin/arm64 Apple M2,
CGO_ENABLED=0/noasm, GOMAXPROCS=1, two prebuilt XY lines, one eight-cubic
arc plus one eight-cubic dot per construction. Three samples were
2.214/2.270/2.244µs, 6904 B/op, 23 allocs/op. Includes validation and owned
geometry, excludes rendering and input-line construction. Shared host; no
Manim comparison or overall speedup is inferred. Raw samples are recorded in
tests/data/root/angle-marks-benchmark-20261104.txt.

## Integrated fourth-wave example — 2026-11-04

Integrated angle-marks SVG checks at .6/1.8/2.5seconds,640x360/SSAA4, give RGBA MAE .061147–.085616 and maxima22/38/44. The actual exported SVG is independently interpreted by CairoSVG; Go scene construction is shared. See artifacts/angle-marks-wave4-svg-20261104/provenance.json.
