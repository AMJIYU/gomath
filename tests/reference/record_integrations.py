"""Attach matching SVG/media evidence to a completed frozen-source checkpoint.

Development only. Usage: python3 tests/reference/record_integrations.py
    CHECKPOINT_JSON PROVENANCE_JSON [PROVENANCE_JSON ...]
Only evidence from exactly the checkpoint's production sources and CLI binary
is accepted. Historical checkpoints are never selected automatically.
"""
import argparse
import hashlib
import json
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("checkpoint", type=Path)
parser.add_argument("provenance", nargs="+", type=Path)
args = parser.parse_args()
root = Path(__file__).resolve().parents[2]


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def relative(path):
    return str(path.resolve().relative_to(root))


checkpoint_path = args.checkpoint.resolve()
relative(checkpoint_path)
checkpoint = json.loads(checkpoint_path.read_text())
if not checkpoint.get("passed"):
    raise ValueError("checkpoint must have passed before attaching integrations")
sources = {str(p.relative_to(root)): digest(p) for p in sorted(root.rglob("*.go"))
           if p.relative_to(root).parts[0] not in {"tests", "artifacts", "bin", ".git"}
           and not any(part.startswith("_") for part in p.relative_to(root).parts)}
if sources != checkpoint["sources_sha256"]:
    raise ValueError("current production sources differ from the frozen checkpoint")
if digest(root / "bin/gomath") != checkpoint["binary_sha256"]:
    raise ValueError("current binary differs from the frozen checkpoint")

records = list(checkpoint.get("integrations", []))
known = {r["provenance"] for r in records}
for path in args.provenance:
    path = path.resolve()
    name = relative(path)
    if name in known:
        raise ValueError(f"integration already recorded: {name}")
    evidence = json.loads(path.read_text())
    if evidence["sources_sha256"] != sources:
        raise ValueError(f"source hashes differ: {name}")
    if evidence["binary_sha256"] != checkpoint["binary_sha256"]:
        raise ValueError(f"binary hash differs: {name}")
    if not evidence.get("files"):
        raise ValueError(f"evidence has no artifact hashes: {name}")
    for file, metadata in evidence["files"].items():
        artifact = (path.parent / file).resolve()
        artifact.relative_to(path.parent)
        if digest(artifact) != metadata["sha256"]:
            raise ValueError(f"artifact hash differs: {artifact}")
        if artifact.stat().st_size != metadata["bytes"]:
            raise ValueError(f"artifact size differs: {artifact}")
    record = {"provenance": name, "provenance_sha256": digest(path),
              "scene": evidence["scene"], "limits": evidence["limits"]}
    if "checks" in evidence:
        record.update(kind="exported SVG vs CairoSVG", checks=evidence["checks"],
                      supersampling=evidence["supersampling"])
    elif "native_png_checks" in evidence:
        record.update(kind="strict independent video decode", frames=evidence["frames"],
                      keyframes_zero_based=evidence["keyframes_zero_based"],
                      distinct_decoded_frames=evidence["distinct_decoded_frames"],
                      native_png_checks=evidence["native_png_checks"])
    else:
        raise ValueError(f"unsupported integration evidence schema: {name}")
    records.append(record)
    known.add(name)

checkpoint["integrations"] = records
checkpoint["integration_recorder_sha256"] = digest(Path(__file__))
temporary = checkpoint_path.with_suffix(".json.pending")
with temporary.open("x") as stream:
    json.dump(checkpoint, stream, indent=2)
    stream.write("\n")
temporary.replace(checkpoint_path)
print(f"Recorded {len(args.provenance)} integrations; total {len(records)}")
