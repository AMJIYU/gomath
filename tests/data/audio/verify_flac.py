#!/usr/bin/env python3
"""Development-only full FLAC decode oracle; ordinary Go tests never run this.

python3 tests/data/audio/verify_flac.py [--keep /absolute/artifact/directory]
Requires independent ffmpeg/ffprobe on PATH. Generates inputs via the focused
Go overlay runner, compares every decoded signed integer byte, validates MD5
and declared format, and saves a machine-readable verification report.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile


def run(args, **kwargs):
    return subprocess.run(args, check=True, stdout=subprocess.PIPE,
                          stderr=subprocess.PIPE, **kwargs).stdout


def verify(root, output):
    ffmpeg, ffprobe = shutil.which("ffmpeg"), shutil.which("ffprobe")
    if not ffmpeg or not ffprobe:
        raise SystemExit("development verification requires ffmpeg and ffprobe")
    output.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, CGO_ENABLED="0", GOMATH_FLAC_REFERENCE_OUT=str(output))
    print(run(["go", "run", "./tests/cmd/run", "-pkg=audio",
               "-run=^TestFLACIndependentDecoderArtifacts$", "-count=1", "-v"],
              cwd=root, env=env).decode(), end="")
    manifest = json.loads((output / "flac_manifest.json").read_text())
    checked, sample_count = [], 0
    for entry in manifest:
        path = output / entry["File"]
        raw = (output / entry["Raw"]).read_bytes()
        assert hashlib.sha256(raw).hexdigest() == entry["SHA256"], path
        encoded = path.read_bytes()
        assert encoded[26:42] == hashlib.md5(raw).digest(), (path, "PCM MD5")
        packed = int.from_bytes(encoded[18:26], "big")
        assert packed & ((1 << 36) - 1) == entry["Frames"], path
        metadata = json.loads(run([
            ffprobe, "-v", "error", "-show_streams", "-of", "json", str(path)]))
        stream, = metadata["streams"]
        assert stream["codec_name"] == "flac", path
        assert int(stream["sample_rate"]) == entry["Rate"], path
        assert stream["channels"] == entry["Channels"], path
        assert int(stream["bits_per_raw_sample"]) == entry["Depth"], path
        fmt = f"s{entry['Depth']}le"
        decoded = run([
            ffmpeg, "-v", "error", "-xerror", "-err_detect", "crccheck+explode",
            "-i", str(path), "-map", "0:a:0", "-c:a", "pcm_" + fmt,
            "-f", fmt, "pipe:1"])
        assert len(decoded) == entry["Frames"] * entry["Channels"] * (entry["Depth"] // 8), path
        if decoded != raw:
            width = entry["Depth"] // 8
            offset = next(i for i, (a, b) in enumerate(zip(decoded, raw)) if a != b)
            raise AssertionError(f"{path}: integer sample {offset // width} differs")
        sample_count += entry["Frames"] * entry["Channels"]
        checked.append({"file": entry["File"], "pcm_sha256": entry["SHA256"],
                        "flac_sha256": hashlib.sha256(encoded).hexdigest()})
    report = {"ffmpeg": run([ffmpeg, "-version"]).decode().splitlines()[0],
              "cases": len(checked), "scalar_samples": sample_count,
              "comparison": "full decoded signed PCM bytes, exact length and every sample",
              "verified": checked}
    (output / "flac_verification.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"PASS: {len(checked)} FLAC streams; {sample_count:,} exact scalar samples")
    print(report["ffmpeg"])


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--keep", type=Path, help="retain artifacts and verification JSON")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[3]
    if args.keep:
        verify(root, args.keep.resolve())
    else:
        with tempfile.TemporaryDirectory(prefix="gomath-flac-") as directory:
            verify(root, Path(directory))


if __name__ == "__main__":
    main()
