#!/usr/bin/env python3
"""Development-only strict independent decoder check; never used by Go tests.

First dump vectors with GOMATH_H264_PREDICTIVE_DUMP_DIR and the central Go
runner. This program requires an independently installed FFmpeg. It checks
complete frame count/size and EVERY visible reconstructed Y/Cb/Cr sample,
then optionally installs hashes as the pure-Go fixture. It also starts fresh
FFmpeg decoders at every IDR of the recovery, cropped 3x3-MB and GOP60 cases.
"""
import argparse
import hashlib
import json
import pathlib
import subprocess
import tempfile


def decode(src, dst):
    subprocess.run([
        "ffmpeg", "-hide_banner", "-loglevel", "error", "-xerror",
        "-err_detect", "explode", "-threads", "1", "-i", str(src),
        "-pix_fmt", "yuv420p", "-f", "rawvideo", "-y", str(dst),
    ], check=True, capture_output=True)
    return dst.read_bytes()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("dump", type=pathlib.Path)
    parser.add_argument("--fixture", type=pathlib.Path)
    parser.add_argument("--manifest", default="predictive_vectors.json")
    parser.add_argument("--all-idr", action="store_true",
                        help="also decode a fresh suffix at every IDR of every vector")
    args = parser.parse_args()
    manifest = args.dump / args.manifest
    vectors = json.loads(manifest.read_bytes())
    frames = suffixes = 0
    with tempfile.TemporaryDirectory(prefix="gomath-h264-independent-") as temp:
        temp = pathlib.Path(temp)
        for v in vectors:
            stem = args.dump / v["Name"]
            stream = pathlib.Path(str(stem) + ".h264").read_bytes()
            expected = pathlib.Path(str(stem) + ".rec.yuv").read_bytes()
            actual = decode(pathlib.Path(str(stem) + ".h264"), temp / "decoded.yuv")
            size = v["Width"] * v["Height"] * 3 // 2
            assert len(actual) == len(expected) == size * v["Frames"], v["Name"]
            assert actual == expected, (v["Name"], next((i for i, (a,b) in enumerate(zip(actual,expected)) if a != b), None))
            assert hashlib.sha256(stream).hexdigest() == v["BitstreamSHA256"], v["Name"]
            assert hashlib.sha256(actual).hexdigest() == v["DecodedSHA256"], v["Name"]
            if args.all_idr or v["GOP"] == 60 or v["Kind"] == "recovery" or (v["Width"], v["Height"]) == (34,38) and v["Kind"] == "local":
                units = stream.split(b"\0\0\0\1")[1:]
                assert len(units) == v["Frames"] + 2
                for f, nal in enumerate(units[2:]):
                    if nal[0] & 31 != 5:
                        continue
                    suffix = b"".join(b"\0\0\0\1"+n for n in units[:2]+units[f+2:])
                    (temp / "entry.h264").write_bytes(suffix)
                    decoded = decode(temp / "entry.h264", temp / "entry.yuv")
                    assert decoded == expected[f*size:], (v["Name"], f, "IDR entry mismatch")
                    suffixes += 1
            frames += v["Frames"]
    if args.fixture:
        args.fixture.write_bytes(manifest.read_bytes())
    print(json.dumps({"streams": len(vectors), "frames": frames,
                      "fresh_IDR_entries": suffixes,
                      "verified_manifest_sha256": hashlib.sha256(manifest.read_bytes()).hexdigest()}))


if __name__ == "__main__":
    main()
