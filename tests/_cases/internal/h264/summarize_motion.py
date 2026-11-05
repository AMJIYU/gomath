#!/usr/bin/env python3
"""Summarize five matched motion-search timings and fixed-picture measurements.

Run from the repository root. Input/output files live in tests/data/internal/h264.
No third-party dependency; this script is never used by Go tests or the runtime.
"""
import json
import pathlib
import re
import statistics

directory = pathlib.Path("tests/data/internal/h264")
cases = {}
for line in (directory / "motion_bench.txt").read_text().splitlines():
    if not line.startswith("BenchmarkMotionSearch/"):
        continue
    fields = line.split()
    name = fields[0].split("/", 1)[1]
    cases.setdefault(name, []).append({
        unit: float(fields[fields.index(unit)-1])
        for unit in ("ns/frame", "B/op", "allocs/op")})
measurements = {}
for line in (directory / "motion_measurements.txt").read_text().splitlines():
    match = re.search(r"(\w+/r\d+): bytes=(\d+) PSNR=([\d.]+)", line)
    if match:
        measurements[match[1]] = {"bytes": int(match[2]), "PSNR": float(match[3])}
assert len(cases) == len(measurements) == 9
rows = ["| Pattern / radius | Bytes | YUV PSNR dB | Median [min,max] ms/frame | B/frame | Allocations/frame |",
        "|---|---:|---:|---:|---:|---:|"]
result = {}
for name, samples in cases.items():
    assert len(samples) == 5, (name, len(samples))
    summary = {unit: {"median": statistics.median(s[unit] for s in samples),
                      "min": min(s[unit] for s in samples),
                      "max": max(s[unit] for s in samples)} for unit in samples[0]}
    m = measurements[name]
    result[name] = {"samples": samples, "summary": summary, **m}
    timing = summary["ns/frame"]
    rows.append(f"| {name} | {m['bytes']:,} | {m['PSNR']:.6f} | "
                f"{timing['median']/1e6:.4f} [{timing['min']/1e6:.4f},{timing['max']/1e6:.4f}] | "
                f"{summary['B/op']['median']/60:,.1f} | {summary['allocs/op']['median']/60:.3f} |")
(directory / "motion_summary.json").write_text(json.dumps(result, indent=2) + "\n")
(directory / "motion_summary.md").write_text("\n".join(rows) + "\n")
print("\n".join(rows))
