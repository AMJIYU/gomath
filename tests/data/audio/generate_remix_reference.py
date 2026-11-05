#!/usr/bin/env python3
"""Development-only exact rational remix oracle; no gomath code is invoked."""
import json
from fractions import Fraction
from pathlib import Path


def case(name, matrix, frames):
    # Float inputs here are exact dyadic fractions, so both serialized inputs
    # and the independently accumulated rational outputs are exactly binary64.
    expected = [
        float(sum((Fraction(x) * Fraction(w) for x, w in zip(frame, row)), Fraction()))
        for frame in frames for row in matrix
    ]
    return dict(name=name, matrix=matrix, samples=sum(frames, []), expected=expected)


cases = [
    case("three_to_two_impulses_and_headroom",
         [[2, -0.5, 0.25], [-1, 3, 0]],
         [[1, 0, 0], [0, 1, 0], [0, 0, 1], [2, -4, 8], [-0.125, 0.5, -2]]),
    case("six_to_two_explicit_center_surround_lfe",
         [[1, 0, 0.5, 0.25, 0.5, 0], [0, 1, 0.5, 0.25, 0, 0.5]],
         [[int(i == j) for i in range(6)] for j in range(6)] + [[-2, 4, 1, -1, 0.5, -0.25]]),
    case("two_to_four_invert_duplicate_and_silence",
         [[0, 1], [-1, 0], [0.5, 0.5], [0, 0]],
         [[1, 0], [0, 1], [4, -8], [-0.5, 0.25]]),
]
target = Path(__file__).with_name("remix_reference.json")
target.write_text(json.dumps(dict(oracle="Python fractions.Fraction, exact dyadic arithmetic", cases=cases),
                             indent=2) + "\n", encoding="utf-8")
