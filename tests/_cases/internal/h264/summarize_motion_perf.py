#!/usr/bin/env python3
"""Summarize preserved before/after five-sample exhaustive-motion benchmarks.

Usage: python3 tests/_cases/internal/h264/summarize_motion_perf.py <capture-dir>
Reads before_bench.txt and after_bench.txt, without changing historical captures.
Each operation covers 60 frames. This is development-only, standard-library
tooling; no Go test or production codec invokes it.
"""
import argparse
import json
from pathlib import Path
import statistics


def read(path):
    cases = {}
    for line in path.read_text().splitlines():
        if not line.startswith("BenchmarkMotionSearch/"):
            continue
        fields = line.split()
        sample = {unit: float(fields[fields.index(unit) - 1])
                  for unit in ("ns/frame", "B/op", "allocs/op")}
        cases.setdefault(fields[0].split("/", 1)[1], []).append(sample)
    assert len(cases) == 9, (path, len(cases))
    for name, samples in cases.items():
        assert len(samples) == 5, (path, name, len(samples))
    return cases


def summarize(samples):
    return {unit: {"median": statistics.median(s[unit] for s in samples),
                   "min": min(s[unit] for s in samples),
                   "max": max(s[unit] for s in samples)}
            for unit in samples[0]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("capture", type=Path)
    directory = parser.parse_args().capture
    baseline = read(directory / "before_bench.txt")
    optimized = read(directory / "after_bench.txt")
    assert baseline.keys() == optimized.keys()
    results = {}
    rows = ["| Pattern/radius | Before median [min,max] ms/frame | "
            "After median [min,max] ms/frame | Time change | B/frame before → after | "
            "Allocations/frame before → after |",
            "|---|---:|---:|---:|---:|---:|"]
    for name, before in baseline.items():
        after = optimized[name]
        a, b = summarize(before), summarize(after)
        change = 100 * (b["ns/frame"]["median"] / a["ns/frame"]["median"] - 1)
        results[name] = {"before_samples": before, "after_samples": after,
                         "before": a, "after": b, "time_change_percent": change}
        times = ["{median:.4f} [{min:.4f},{max:.4f}]".format(
            **{k: v / 1e6 for k, v in x["ns/frame"].items()}) for x in (a, b)]
        rows.append(f"| {name} | {times[0]} | {times[1]} | {change:+.1f}% | "
                    f"{a['B/op']['median']/60:,.1f} → {b['B/op']['median']/60:,.1f} | "
                    f"{a['allocs/op']['median']/60:.3f} → {b['allocs/op']['median']/60:.3f} |")
    (directory / "benchmark_summary.json").write_text(json.dumps(results, indent=2) + "\n")
    (directory / "benchmark_summary.md").write_text("\n".join(rows) + "\n")
    print("\n".join(rows))


if __name__ == "__main__":
    main()
