#!/usr/bin/env python3
"""Development-only AIFF references. No production code or Go test invokes this.

Run from the repository root with Python 3.11/3.12 (stdlib aifc) and FFmpeg:
  python3 tests/reference/audio_aiff.py

Independent FFmpeg/CPython encoders create inputs from explicit integer codes;
FFmpeg completely decodes them to signed, left-aligned s32le. The central pure
Go test then checks every sample against those codes. Only small reference
inputs are retained; verification.json records the full matrix. No gomath AIFF
encoder, generated PCM, or decoder supplies expected values.
"""
import argparse
import aifc  # development-only CPython oracle, removed in Python 3.13
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import struct
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[2]
SOURCES = [
    {"title": "Apple Audio Interchange File Format 1.3",
     "url": "https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/AIFF/Docs/AIFF-1.3.pdf",
     "sha256": "56902eb9a5410adcde5f928830b26234442070e0ab1f047f564b23af84067e0a",
     "sections": "Common, Sound Data, Block-Aligning Sound Data, Text Chunks"},
    {"title": "Apple AIFF-C specification, 1991-09-26 distribution",
     "url": "https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/AIFF/Docs/AIFF-C.9.26.91.pdf",
     "sha256": "04417236851b4e5a9c792d144ed6aa73e9154553c60369de3d9919dcf20845bd",
     "sections": "FVER (PDF page10), COMM, SSND, Appendix A example3 (44100 Hz, NONE)"},
    {"title": "FFmpeg n8.0.1 AIFF demuxer (independent implementation)",
     "url": "https://raw.githubusercontent.com/FFmpeg/FFmpeg/n8.0.1/libavformat/aiffdec.c",
     "sha256": "c8a0ee8334479e8bf60ae08bdce721d0c41216b244f413805b85d1daf1af7ac9"},
]


def run(args, **kwargs):
    try:
        result = subprocess.run(args, check=True, capture_output=True, **kwargs)
    except subprocess.CalledProcessError as error:
        sys.stderr.buffer.write(error.stdout)
        sys.stderr.buffer.write(error.stderr)
        raise
    return result.stdout


def digest(data):
    return hashlib.sha256(data).hexdigest()


def codes_for(depth, channels, frames):
    scale = 1 << (depth - 1)
    extremes = [-scale, -scale + 1, -scale // 2, -1, 0, 1, scale // 2, scale - 1]
    values = []
    for i in range(frames * channels):
        if i < 8:
            value = extremes[i]
        else:
            value = ((i * 0x9E3779B9) & (2 * scale - 1)) - scale
        values.append(value)
    return values


def signed_bytes(codes, depth, endian):
    return b"".join(c.to_bytes(depth // 8, endian, signed=True) for c in codes)


def chunks(encoded):
    position = 12
    result = []
    while position < len(encoded):
        name = encoded[position:position + 4]
        size, = struct.unpack_from(">I", encoded, position + 4)
        result.append((name, encoded[position + 8:position + 8 + size]))
        position += 8 + size + (size & 1)
    assert position == len(encoded)
    return result


def chunk(name, data):
    return name + struct.pack(">I", len(data)) + data + b"\0" * (len(data) & 1)


def form(kind, entries):
    data = kind + b"".join(chunk(name, data) for name, data in entries)
    return b"FORM" + struct.pack(">I", len(data)) + data


def python_input(depth, channels, frames, rate, compressed):
    values = codes_for(depth, channels, frames)
    # aifc.close owns its file; retain BytesIO contents with an explicit wrapper.
    class KeepOpen(io.BytesIO):
        def close(self):
            pass
    output = KeepOpen()
    with aifc.open(output, "wb") as writer:
        if compressed:
            writer.aifc()
        else:
            writer.aiff()
        writer.setparams((channels, depth // 8, rate, frames, b"NONE", b"not compressed"))
        writer.writeframes(signed_bytes(values, depth, "big"))
    return output.getvalue(), values


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=ROOT / "tests/data/audio/aiff")
    args = parser.parse_args()
    destination = args.output.resolve()
    destination.mkdir(parents=True, exist_ok=True)
    cases = []
    verified = []
    with tempfile.TemporaryDirectory(prefix="gomath-aiff-", dir=destination) as temporary:
        temp = Path(temporary)

        def verify(name, encoded, values, depth, channels, frames, rate, source, discard=False, unused_tail=False):
            target = temp / (name + ".aiff")
            target.write_bytes(encoded)
            actual = run(["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error",
                          "-xerror", "-err_detect", "explode", "-threads", "1",
                          "-i", str(target), "-map", "0:a:0", "-c:a", "pcm_s32le",
                          "-f", "s32le", "-"])
            expected = b"".join(struct.pack("<i", v << (32 - depth)) for v in values)
            if unused_tail:
                # FFmpeg emits unused SSND storage past COMM.numSampleFrames.
                # Do not conceal this by applying an output duration/trim. The
                # independent CPython reader, explicitly bounded by its own
                # parsed COMM count, verifies the normative samples instead.
                assert actual[:len(expected)] == expected and len(actual) > len(expected), name
                with aifc.open(io.BytesIO(encoded), "rb") as reader:
                    assert reader.getnframes() == frames and reader.getnchannels() == channels
                    assert reader.getframerate() == rate and reader.getsampwidth() * 8 == depth
                    assert reader.readframes(reader.getnframes()) == signed_bytes(values, depth, "big"), name
            else:
                assert actual == expected, (name, len(actual), len(expected))
            entry = {"File": name + ".aiff", "EncodedSHA256": digest(encoded),
                     "RawSHA256": digest(expected), "FFmpegWholeSHA256": digest(actual),
                     "FFmpegExtraBytes": len(actual) - len(expected),
                     "Rate": rate, "Depth": depth,
                     "Channels": channels, "Frames": frames,
                     "DiscardMetadata": discard, "Codes": values, "Source": source}
            # Preserve these real muxer files as explicit unsupported-layout
            # evidence. Do not strip CHAN and silently lose speaker semantics.
            if any(name == b"CHAN" for name, data in chunks(encoded)):
                entry["Unsupported"] = "CHAN speaker-layout semantics"
            (temp / entry["File"]).write_bytes(encoded)
            cases.append(entry)
            verified.append({k: v for k, v in entry.items() if k != "Codes"})

        # Independent real muxer: signed BE AIFF plus the bounded LE sowt case.
        for depth, little in [(8, False), (16, False), (24, False), (32, False), (16, True)]:
            for channels in (1, 2, 6):
                for frames in (1, 17, 10923):
                    rate = 44100 if frames == 17 else 48000
                    values = codes_for(depth, channels, frames)
                    raw = b"".join(struct.pack("<i", v << (32 - depth)) for v in values)
                    codec = "pcm_s8" if depth == 8 else f"pcm_s{depth}{'le' if little else 'be'}"
                    target = temp / "encoded.aiff"
                    run(["ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y",
                         "-fflags", "+bitexact", "-f", "s32le", "-ar", str(rate), "-ac", str(channels),
                         "-i", "-", "-map_metadata", "-1", "-flags:a", "+bitexact",
                         "-c:a", codec, "-f", "aiff", str(target)], input=raw)
                    encoded = target.read_bytes()
                    verify(f"ffmpeg_{codec}_{channels}ch_n{frames}", encoded, values,
                           depth, channels, frames, rate, "FFmpeg 8.0.1 encoder")

        # A second, independent AIFF/AIFC NONE implementation at every depth.
        for compressed in (False, True):
            for depth in (8, 16, 24, 32):
                for channels in (1, 2, 6):
                    encoded, values = python_input(depth, channels, 16, 44100, compressed)
                    verify(f"cpython_{'NONE' if compressed else 'AIFF'}_s{depth}_{channels}ch",
                           encoded, values, depth, channels, 16, 44100,
                           "CPython stdlib aifc " + platform.python_version())

        # Spec-derived structure variations, based on independent PCM bytes.
        base, values = python_input(24, 2, 17, 44100, True)
        entries = chunks(base)
        ssnd = next(data for name, data in entries if name == b"SSND")
        rest = [(name, data) for name, data in entries if name != b"SSND"]
        # SSND first requires seek support in FFmpeg, but not in ReadAIFF.
        shifted = struct.pack(">II", 3, 512) + b"abc" + ssnd[8:]
        reordered = form(b"AIFC", [(b"SSND", shifted)] + rest)
        verify("spec_SSNDfirst_offset3_NONE24", reordered, values, 24, 2, 17, 44100,
               "Apple SSND/COMM arbitrary ordering and offset, CPython sample bytes")
        annotated = form(b"AIFC", [(b"NAME", b"sample"), (b"AUTH", b"reference"),
                                  (b"(c) ", b"2026"), (b"ANNO", b"odd"),
                                  (b"ANNO", b"another")] + entries)
        verify("spec_explicit_text_discard", annotated, values, 24, 2, 17, 44100,
               "Apple text chunk definitions, CPython sample bytes", discard=True)

        # Exercise exact low/max rate, scalar extrema and odd-byte sample count.
        for rate in (1, 8000, 11025, 192000, 768000):
            encoded, values = python_input(8, 1, 16, rate, False)
            verify(f"cpython_s8_rate{rate}", encoded, values, 8, 1, 16, rate,
                   "CPython stdlib aifc " + platform.python_version())

        # Explicit independent evidence for normative COMM count vs unused
        # storage. CPython puts one unused zero byte inside odd-length PCM8 SSND;
        # FFmpeg decodes that byte as audio, while ReadAIFF must honor COMM.
        encoded, values = python_input(8, 1, 17, 44100, False)
        verify("spec_COMM_bounds_unused_SSND", encoded, values, 8, 1, 17, 44100,
               "Apple SSND frame-count rule, independent CPython reader/writer",
               unused_tail=True)

        # The pure-Go API is checked against every independent case before any
        # fixture selection. Redirect output to a compact retained validation log.
        (temp / "manifest.json").write_text(json.dumps({"Cases": cases}, indent=2) + "\n")
        env = dict(os.environ, CGO_ENABLED="0", GOMAXPROCS="1", GOMATH_AIFF_REFERENCE_DIR=str(temp))
        output = run(["go", "run", "./tests/cmd/run", "-pkg=audio",
                      "-run=^TestReadAIFFIndependentFixtures$", "-count=1", "-p", "1"],
                     cwd=ROOT, env=env)
        (destination / "go_verification.txt").write_bytes(output)
        retained = [c for c in cases if c["Frames"] <= 17]
        for c in retained:
            (destination / c["File"]).write_bytes((temp / c["File"]).read_bytes())
        (destination / "manifest.json").write_text(json.dumps({"Cases": retained}, indent=2) + "\n")
        record = {"sources": SOURCES, "python": sys.version,
                  "ffmpeg": run(["ffmpeg", "-version"]).decode().splitlines(),
                  "streams": len(cases), "scalar_samples": sum(len(c["Codes"]) for c in cases),
                  "go_decoded_streams": sum(not c.get("Unsupported") for c in cases),
                  "go_rejected_CHAN_streams": sum(bool(c.get("Unsupported")) for c in cases),
                  "retained_streams": len(retained), "cases": verified}
        (destination / "verification.json").write_text(json.dumps(record, indent=2) + "\n")
        print(json.dumps({k: record[k] for k in ("streams", "scalar_samples", "retained_streams")}))


if __name__ == "__main__":
    main()
