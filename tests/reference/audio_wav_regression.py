#!/usr/bin/env python3
"""Development-only complete FFmpeg check of the existing PCM/float WAV writer.

Uses the existing opt-in central artifact test. Runtime and default tests never
invoke this tool. Artifacts stay under tests/data/audio/ima_adpcm/.
"""
import hashlib
import json
import os
from pathlib import Path
import struct
import subprocess

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/data/audio/ima_adpcm/_pcm_float_regression"


def run(cmd, **kwargs):
    p = subprocess.run(cmd, capture_output=True, check=True, **kwargs)
    return p.stdout


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, CGO_ENABLED="0", GOMATH_AUDIO_REFERENCE_OUT=str(OUT))
    log = run(["go", "run", "./tests/cmd/run", "-pkg=audio",
               "-run=^TestWAVIndependentDecoderArtifacts$", "-count=1"], cwd=ROOT, env=env)
    (OUT / "go_artifacts.txt").write_bytes(log)
    manifest = json.loads((OUT / "manifest.json").read_text())
    results = []
    for case in manifest:
        encoding = int(case["file"].split("_")[1])
        fmt, codec, scale = {1: ("s16le", "pcm_s16le", 32768),
                             2: ("s24le", "pcm_s24le", 8388608),
                             3: ("s32le", "pcm_s32le", 2147483648),
                             4: ("f32le", "pcm_f32le", None),
                             5: ("f64le", "pcm_f64le", None)}[encoding]
        src = OUT / case["file"]
        raw = run(["ffmpeg", "-v", "error", "-xerror", "-err_detect", "explode", "-i", str(src),
                   "-map", "0:a:0", "-c:a", codec, "-f", fmt, "pipe:1"])
        if scale:
            width = {1: 2, 2: 3, 3: 4}[encoding]
            expected = b"".join(min(scale - 1, round(v * scale)).to_bytes(width, "little", signed=True)
                                for v in case["samples"])
        else:
            code = "f" if encoding == 4 else "d"
            expected = struct.pack("<" + code * len(case["samples"]), *case["samples"])
        assert raw == expected, (case["file"], "complete PCM/float mismatch")
        probe = json.loads(run(["ffprobe", "-v", "error", "-show_streams", "-of", "json", str(src)]))["streams"][0]
        assert int(probe["sample_rate"]) == case["rate"] and probe["channels"] == case["channels"]
        # Existing emitter's six-channel case explicitly uses mask 63.
        if case["channels"] == 6:
            assert probe["channel_layout"] == "5.1"
        results.append({"file": case["file"], "encodedSHA256": hashlib.sha256(src.read_bytes()).hexdigest(),
                        "decodedSHA256": hashlib.sha256(raw).hexdigest(), "scalarSamples": len(case["samples"]),
                        "decodedBytes": len(raw), "rate": case["rate"], "channels": case["channels"]})
    report = {"ffmpeg": run(["ffmpeg", "-version"]).decode().splitlines()[0],
              "completeExactStreams": len(results), "scalarSamples": sum(c["scalarSamples"] for c in results), "cases": results}
    (OUT / "verification.json").write_text(json.dumps(report, indent=2) + "\n")
    print("PCM/float complete FFmpeg regression:", len(results), "streams exact")


if __name__ == "__main__":
    main()
