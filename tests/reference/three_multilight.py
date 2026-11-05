"""Development-only 90-digit Decimal point-light oracle; no runtime dependency.

Run from the repository root: python3 tests/reference/three_multilight.py
Positions/RGB/normal components are first rounded to the actual float64 inputs,
then converted exactly to Decimal. All lighting distances, dots, attenuation,
positive additions and final saturation are independently evaluated at 90 digits.
No Go renderer, projection or production lighting helper is invoked.
"""
import decimal
import json
import math
from pathlib import Path
import random

decimal.getcontext().prec = 90
D = decimal.Decimal.from_float
rng = random.Random(20261104)


def source(position, rgb=(1.0, 1.0, 1.0), diffuse=1.0, attenuation=0.0):
    return dict(position=position, rgb=rgb, diffuse=diffuse, attenuation=attenuation)


def case(name, world, normal, lights, ambient=0.0):
    rgb = [D(ambient)] * 3
    for light in lights:
        delta = [D(float(a)) - D(float(b)) for a, b in zip(light["position"], world)]
        d2 = sum(x*x for x in delta)
        if not d2:
            continue
        dot = sum(D(float(a))*b for a, b in zip(normal, delta))
        energy = D(light["diffuse"]) * max(decimal.Decimal(0), dot/d2.sqrt())
        energy /= 1 + D(light["attenuation"])*d2
        for i in range(3):
            rgb[i] += D(float(light["rgb"][i]))*energy
    return dict(name=name, world=world, normal=normal, lights=lights, ambient=ambient,
                want=[float(min(1, c)) for c in rgb])


cases = [
    case("subnormal", (0., 0., 0.), (1., 0., 0.), [source((5e-324, 0., 0.))]),
    case("maximal-distance-attenuation", (-1e12, 0., 0.), (1., 0., 0.), [source((1e12, 0., 0.), attenuation=1e12)]),
    case("coincident", (1., -2., 3.), (0., 0., 1.), [source((1., -2., 3.))], .13),
    case("opposed-not-vector-canceled", (0., 0., 0.), (0., 0., 1.), [source((0., 0., 2.), (1., 0., 0.), .7), source((0., 0., -2.), (0., 1., 0.), .9)], .2),
    case("zero-normal", (0., 0., 0.), (0., 0., 0.), [source((1., 2., 3.))], .17),
    case("no-sources", (0., 0., 0.), (0., 0., 1.), [], .23),
]
for index in range(64):
    p = [rng.uniform(-5., 5.) for _ in range(3)]
    n = [rng.uniform(-1., 1.) for _ in range(3)]
    length = math.sqrt(sum(x*x for x in n))
    n = [x/length for x in n]
    lights = []
    for _ in range((1, 3, 16)[index % 3]):
        lights.append(source([rng.uniform(-9., 9.) for _ in range(3)],
                             [rng.random() for _ in range(3)], rng.random(),
                             (0., .07, 1e12)[rng.randrange(3)]))
    cases.append(case(f"seeded-{index}", p, n, lights, rng.random()*.23))
output = Path(__file__).resolve().parents[1]/"data/three/multilight/numerical.json"
output.write_text(json.dumps(dict(oracle="Python Decimal, 90 digits, seed 20261104", cases=cases), indent=2)+"\n")
print(output)
