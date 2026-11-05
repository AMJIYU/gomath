# Curved-network independent reference

`curved_math.json` comes from `tests/reference/network_curved_reference.py`.
The development-only generator uses NumPy polynomial roots (instead of Go's
monotone derivative isolation/bisection), SciPy quad and brentq (instead of the
Go length kernels), and circle-center polar tangent cubic construction.
Ordinary tests consume static JSON and never start an external program.

Fourteen cases cover straight/signed/semicircular/diagonal arrows and every
self-loop side/direction. Source vertices have radius .12; tip trim .16 and full
width .08 are explicit. The fixture records NumPy/SciPy versions. Five points per
retained cubic plus three arrowhead vertices are compared within 2e-8 at the
fixture's ordinary coordinate scale; separate tests check bbox boundaries,
nonpenetration and strict extreme-scale errors. No exact-conic or Manim pixel
parity assertion is made. Old reference data was not changed.
