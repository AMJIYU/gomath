# Circular braces

`NewArcBrace(center, radius, startAngle, sweep, options)` builds a real filled
vector `Brace` from explicit circle data. `DefaultArcBraceOptions()` selects
outside (`Direction=+X`), buffer `.2`, sharpness `2`. Negative X selects inside;
other directions reject. Circle data is explicit because ordinary editable
Objects do not retain authoritative arc parameters after arbitrary transforms.

The implementation follows the MIT Manim CE0.22.1 ArcBrace construction:

1. Make a vertical line from start to end angle, multiplied by `max(1,radius)`.
2. Fit the ordinary planar brace to that line and divide its coordinates by
   `max(1,radius)`. Thus buffer and cap height shrink for radii at least1.
3. Add `log(max(.3,radius))` to X and map XY with the complex exponential.
4. Translate by the explicit center, preserving its Z plane.

The `.3` radial floor is source behavior: a radius `.1` and `.3` produce the
same brace at identical angles/options. An inside brace does not automatically
avoid crossing the circle center or intersecting another outline. Reversing a
signed sweep produces the same interval fit, rather than reversing contour
direction. These rules are not physical world-distance padding.

Each cubic's handles contract toward its anchors by `.01` before exponentiation,
then expand by100. Directly exponentiating four controls gives different
tangents. The output retains25 cubics and is an approximation of the nonlinear
image, not an exact exponential curve or adaptive subdivision. All construction
is pure Go; the static brace template retains Manim's license notice.

The returned `Brace` supports `Tip`, anchor-box `Direction`, and copied
`LabelAtTip` placements. These follow its actual editable path, with the same
identity/topology checks as the planar brace. Ordinary styles and Scene
Transform work. No source arc, callback or live label attachment is retained.

Inputs require finite center coordinates within +/-1e12, radius `[1e-9,1e6]`,
start/end angles within +/-1e6 and absolute sweep `[1e-9,2pi]`. Buffer/sharpness
have the planar Brace limits. The intermediate planar fit must be representable;
exponent radii must remain `[1e-12,1e12]`, all restored controls must remain
bounded, and positive cap handle details must span32 ULPs at the final coordinate
scale. Excessive buffer, tiny/singular details and unrepresentable translations
return `nil,error`; no partial object is published.

`tests/reference/arc_braces.py` independently constructs36 braces in pinned
Manim commit `16ea443e2a5f1d79db0a721161411ce916d6e733`. Six radii cover the floor
and scaling boundary; inside/outside, three signed intervals, buffer/sharpness
variants, tips and anchor-box directions are checked. Seven ordinary linear
Transform times per construction give252 ordered full-control states at2e-11
world-unit tolerance. Independent radial extrema, interval reversal, explicit
nonzero-Z translation, errors and concurrent snapshot ownership are also tested.

The first exploratory reference with a nonzero-Z Manim Arc exposed its existing
`get_arc_center` fallback to ORIGIN (the underlying intersection is planar).
Its original output remains under `tests/data/root/arc-braces/manim-0.22.1.json`
as diagnostic evidence and is not a passing fixture. The accepted planar
reference is `manim-planar-0.22.1.json`. Go's explicit center API intentionally
does not infer a center through that fallback. No source Python files changed.

Focused central tests are `TestPinnedManimArcBraceTransformStates`,
`TestArcBraceRadiusFloorScalingAndInterval` and
`TestArcBraceInvalidInputsAndIndependentSnapshots`. All test sources, references
and data remain under `tests/`. Run the `arc-braces` CLI scene for actual vector
construction and animation. Full BraceLabel convenience/lifecycle methods,
recovery from arbitrary edited Arc Objects and full Manim parity remain pending.
