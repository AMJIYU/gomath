"""Development-only frozen-source Go acceptance run; never a runtime fallback.

Usage: python3 tests/reference/go_checkpoint.py NEW_OUTPUT_DIR --date YYYY-MM-DD
Run after all authoring/agent changes finish. Outputs commands, complete logs,
return codes and hashes; shared-host durations are not performance benchmarks.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("output", type=Path)
parser.add_argument("--date", required=True)
parser.add_argument("--scope", default="Frozen-source acceptance checks; no complete Manim parity or global speed claim")
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]
out = args.output.resolve()
out.mkdir(parents=True, exist_ok=False)

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def sources():
    return {str(p.relative_to(root)): digest(p) for p in sorted(root.rglob("*.go"))
            if p.relative_to(root).parts[0] not in {"tests", "artifacts", "bin", ".git"}
            and not any(part.startswith("_") for part in p.relative_to(root).parts)}

def git(*command):
    return subprocess.check_output(["git", *command], cwd=root, text=True).strip()

before = sources()
checks = []
commands = [
    ("binary", {"CGO_ENABLED": "0"}, ["go", "build", "-tags", "noasm", "-o", "bin/gomath", "./cmd/gomath"]),
    ("tests", {"CGO_ENABLED": "0"}, ["go", "test", "-tags", "noasm", "-count=1", "./..."]),
    ("race", {"CGO_ENABLED": "1"}, ["go", "test", "-race", "-tags", "noasm", "-count=1", "./..."]),
    ("vet", {"CGO_ENABLED": "0"}, ["go", "vet", "-tags", "noasm", "./..."]),
    ("purecheck", {"CGO_ENABLED": "0"}, ["go", "run", "-tags", "noasm", "./cmd/purecheck"]),
    ("linux-amd64", {"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"}, ["go", "build", "-tags", "noasm", "./..."]),
    ("windows-amd64", {"CGO_ENABLED": "0", "GOOS": "windows", "GOARCH": "amd64"}, ["go", "build", "-tags", "noasm", "./..."]),
]
binary_hash = None
for name, overrides, command in commands:
    print("START", name, flush=True)
    start = time.monotonic()
    result = subprocess.run(command, env={**os.environ, **overrides}, cwd=root,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    log = out / (name + ".txt")
    log.write_text(result.stdout)
    check = {"name": name, "environment": overrides, "command": command,
             "passed": result.returncode == 0, "returncode": result.returncode,
             "wall_seconds": round(time.monotonic() - start, 3),
             "log": log.name, "log_sha256": digest(log)}
    suite = re.search(r"^ok\s+gomath/tests\s+([0-9.]+)s", result.stdout, re.M)
    if suite:
        check["central_suite_seconds"] = float(suite[1])
    checks.append(check)
    if name == "binary" and result.returncode == 0:
        binary_hash = digest(root / "bin/gomath")
    print("PASS" if check["passed"] else "FAIL", name, check["wall_seconds"], flush=True)
    if result.returncode != 0:
        print(result.stdout[-12000:], flush=True)

misplaced = [str(p.relative_to(root)) for p in sorted(root.rglob("*_test.go"))
             if p.relative_to(root).parts[0] != "tests"]
misplaced_data = [str(p.relative_to(root)) for p in sorted(root.rglob("testdata"))
                  if p.is_dir() and p.relative_to(root).parts[0] != "tests"]
reference_status = subprocess.check_output(
    ["git", "-C", str(root.parent / "manim"), "status", "--porcelain"], text=True)
unchanged = sources() == before
binary_unchanged = binary_hash is not None and digest(root / "bin/gomath") == binary_hash
manifest = {
    "date": args.date, "scope": args.scope,
    "go": subprocess.check_output(["go", "version"], text=True).strip(),
    "host": platform.platform(), "hardware": "Apple M2; shared host",
    "branch": git("branch", "--show-current"), "checks": checks,
    "sources_sha256": before, "sources_unchanged_during_checks": unchanged,
    "binary_sha256": binary_hash, "binary_unchanged_during_checks": binary_unchanged,
    "module_sha256": {name: digest(root / name) for name in ["go.mod", "go.sum"]},
    "demo_assets_sha256": {str(p.relative_to(root)): digest(p)
                           for p in sorted((root / "examples/scenes/assets").iterdir()) if p.is_file()},
    "test_sources_outside_tests": misplaced, "testdata_outside_tests": misplaced_data,
    "python_reference_git_clean": not reference_status,
    "generator_sha256": digest(Path(__file__)),
}
manifest["passed"] = (all(c["passed"] for c in checks) and unchanged and binary_unchanged
                      and not misplaced and not misplaced_data and not reference_status)
(out / "checkpoint.json").write_text(json.dumps(manifest, indent=2) + "\n")
print("CHECKPOINT", "PASS" if manifest["passed"] else "FAIL", flush=True)
raise SystemExit(0 if manifest["passed"] else 1)
