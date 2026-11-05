#!/usr/bin/env python3
"""Development-only Microsoft WAVE IMA references; never run by Go tests.

Requires FFmpeg and Python soundfile (libsndfile) / numpy. No gomath decoder or
encoder supplies expected samples. Saves complete FFmpeg/libsndfile output
hashes, including their fact-tail and arithmetic differences from the spec.
"""
import hashlib
import io
import json
import math
from pathlib import Path
import struct
import subprocess

import numpy as np
import soundfile as sf

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/data/audio/ima_adpcm"
STEPS = [7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 21, 23, 25, 28, 31,
         34, 37, 41, 45, 50, 55, 60, 66, 73, 80, 88, 97, 107, 118, 130,
         143, 157, 173, 190, 209, 230, 253, 279, 307, 337, 371, 408, 449,
         494, 544, 598, 658, 724, 796, 876, 963, 1060, 1166, 1282, 1411,
         1552, 1707, 1878, 2066, 2272, 2499, 2749, 3024, 3327, 3660,
         4026, 4428, 4871, 5358, 5894, 6484, 7132, 7845, 8630, 9493,
         10442, 11487, 12635, 13899, 15289, 16818, 18500, 20350,
         22385, 24623, 27086, 29794, 32767]
INDEX = [-1, -1, -1, -1, 2, 4, 6, 8]
SOURCES = [
    {"title": "Microsoft Multimedia Standards Update, Revision 3.0 (1994-04-15)",
     "url": "https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/Docs/RIFFNEW.pdf",
     "sha256": "b2473ccc73d0f6378f7f56b9f47bea10a9d90a162e7be2d31049cf664c0eb303",
     "sections": "DVI ADPCM pp.28-34: fmt, fact, blocks, nibble order, tables and decoder"},
    {"title": "libsndfile 1.2.2 independent WAVE IMA decoder",
     "url": "https://raw.githubusercontent.com/libsndfile/libsndfile/1.2.2/src/ima_adpcm.c",
     "sha256": "dee6d75468d156e7402812a66cfcb45ff9c46ad524b052dd81e6c1072b2447f8",
     "section": "ima_reader_init, wavlike_ima_decode_block, ima_decode_block"},
    {"title": "FFmpeg n8.0.1 independent ADPCM decoder",
     "url": "https://raw.githubusercontent.com/FFmpeg/FFmpeg/n8.0.1/libavcodec/adpcm.c",
     "sha256": "9515b7a5dc02869f094376d12a350886106364779ac94caa642cc2001cb2f9b7",
     "section": "ADPCM_IMA_WAV and adpcm_ima_expand_nibble (shift=3)"},
]


def sha(b):
    return hashlib.sha256(b).hexdigest()


def run(cmd, data=None):
    p = subprocess.run(cmd, input=data, capture_output=True)
    if p.returncode:
        raise RuntimeError((cmd, p.stderr.decode(errors="replace")))
    return p.stdout


def chunk(kind, b):
    return kind + struct.pack("<I", len(b)) + b + bytes(len(b) & 1)


def wave(channels, align, data, fact=None, rate=22050, fact_after=False):
    spb = 1 + (align - 4 * channels) * 2 // channels
    if fact is None:
        fact = len(data) // align * spb
    fmt = struct.pack("<HHIIHHHH", 17, channels, rate, rate * align // spb,
                      align, 4, 2, spb)
    entries = [chunk(b"fmt ", fmt), chunk(b"fact", struct.pack("<I", fact)), chunk(b"data", data)]
    if fact_after:
        entries[1], entries[2] = entries[2], entries[1]
    b = b"WAVE" + b"".join(entries)
    return b"RIFF" + struct.pack("<I", len(b)) + b


def chunks(b):
    assert b[:4] == b"RIFF" and b[8:12] == b"WAVE"
    assert struct.unpack_from("<I", b, 4)[0] + 8 == len(b)
    p = 12
    result = {}
    while p < len(b):
        kind = b[p:p + 4]
        n, = struct.unpack_from("<I", b, p + 4)
        assert kind not in result
        result[kind] = b[p + 8:p + 8 + n]
        p += 8 + n + (n & 1)
    assert p == len(b)
    return result


def oracle(b, multiply=False):
    parts = chunks(b)
    tag, channels, rate, avg, align, bits, cb, spb = struct.unpack("<HHIIHHHH", parts[b"fmt "])
    assert (tag, bits, cb) == (17, 4, 2)
    fact, = struct.unpack("<I", parts[b"fact"])
    payload = parts[b"data"]
    assert len(payload) % align == 0
    result = []
    for start in range(0, len(payload), align):
        block = payload[start:start + align]
        planes = []
        for ch in range(channels):
            pred, index, reserved = struct.unpack_from("<hBB", block, ch * 4)
            assert index <= 88 and reserved == 0
            samples = [pred]
            codes = b"".join(block[g + ch * 4:g + ch * 4 + 4]
                             for g in range(channels * 4, align, channels * 4))
            for byte in codes:
                for nibble in (byte & 15, byte >> 4):
                    step = STEPS[index]
                    if multiply:
                        diff = ((2 * (nibble & 7) + 1) * step) // 8
                    else:
                        # Algorithm on p.34: each division rounds separately.
                        diff = sum(step // divisor for mask, divisor in ((4, 1), (2, 2), (1, 4))
                                   if nibble & mask) + step // 8
                    pred += -diff if nibble & 8 else diff
                    pred = min(32767, max(-32768, pred))
                    index = min(88, max(0, index + INDEX[nibble & 7]))
                    samples.append(pred)
            assert len(samples) == spb
            planes.append(samples)
        result.extend(v for frame in zip(*planes) for v in frame)
    full = struct.pack("<" + "h" * len(result), *result)
    return full[:fact * channels * 2], full, rate, channels, fact


def block(channels, align, seed):
    b = bytearray()
    for ch in range(channels):
        b += struct.pack("<hBB", (-32768, 32767, 0, -1000)[(seed + ch) % 4], (seed * 13 + ch * 37) % 89, 0)
    for i in range(align - channels * 4):
        b.append((seed * 97 + i * 37 + i // 4 * 17) & 255)
    return bytes(b)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    streams = []
    def add(name, b, origin):
        streams.append((name + ".wav", b, origin))

    # Every legal initial index and code, with both saturated predictors and 0.
    b = b"".join(struct.pack("<hBB", pred, index, 0) + bytes([nibble * 17]) * 4
                 for pred in (-32768, 0, 32767) for index in range(89) for nibble in range(16))
    add("spec_all_indices_codes_mono", wave(1, 8, b), "1994 spec explicit headers/codes")
    for ch in (1, 2):
        for align in (4 * ch, 8 * ch, 12 * ch, 256, 1024):
            b = b"".join(block(ch, align, s) for s in range(3))
            add(f"spec_{ch}ch_b{align}", wave(ch, align, b), "1994 spec explicit headers/codes")
        align = 16 * ch
        spb = 25
        b = block(ch, align, 1) + block(ch, align, 2)
        for tail in (1, 2, 8, 24):
            add(f"spec_{ch}ch_fact_tail{tail}", wave(ch, align, b, fact=spb + tail, fact_after=True), "spec fact limits last block")
        for rate in (1, 768000):
            add(f"spec_{ch}ch_rate{rate}", wave(ch, 8 * ch, block(ch, 8 * ch, 19), rate=rate), "spec explicit integer rate")
        add(f"spec_{ch}ch_empty", wave(ch, 8 * ch, b""), "empty data, fact=0")
        align = 32768 if ch == 1 else 65528
        add(f"spec_{ch}ch_large_block", wave(ch, align, block(ch, align, 41)), "maximum representable samplesPerBlock/alignment boundary")

    # Real independent encoder outputs; preserve their metadata/fact verbatim.
    for ch in (1, 2):
        for frames, align in ((1, 256), (17, 256), (1000, 256), (10001, 1024)):
            values = [int(24000 * math.sin((i // ch) * (ch + i % ch + 11) * .023))
                      for i in range(frames * ch)]
            source = struct.pack("<" + "h" * len(values), *values)
            path = OUT / f"ffmpeg_{ch}ch_n{frames}_b{align}.wav"
            run(["ffmpeg", "-v", "error", "-f", "s16le", "-ar", "44100", "-ac", str(ch),
                 "-i", "pipe:0", "-map_metadata", "-1", "-c:a", "adpcm_ima_wav", "-block_size", str(align), "-y", str(path)], source)
            add(path.stem, path.read_bytes(), "FFmpeg adpcm_ima_wav from explicit s16 sine")
        values = np.array([(-32768, 32767, -1, 0, 1)[i % 5] for i in range(257 * ch)], dtype=np.int16).reshape(-1, ch)
        stream = io.BytesIO()
        sf.write(stream, values, 22050, format="WAV", subtype="IMA_ADPCM")
        add(f"libsndfile_{ch}ch_n257", stream.getvalue(), "libsndfile encoder from explicit integer extremes")

    manifest = {"Sources": SOURCES, "FFmpegVersion": run(["ffmpeg", "-version"]).decode().splitlines()[0],
                "LibsndfileVersion": sf.__libsndfile_version__, "Cases": []}
    for name, encoded, origin in streams:
        expected, full_spec, rate, channels, frames = oracle(encoded)
        _, full_multiply, _, _, _ = oracle(encoded, multiply=True)
        path = OUT / name
        path.write_bytes(encoded)
        raw = run(["ffmpeg", "-v", "error", "-xerror", "-err_detect", "explode", "-i", str(path),
                   "-map", "0:a:0", "-c:a", "pcm_s16le", "-f", "s16le", "pipe:1"])
        assert raw == full_multiply, (name, "FFmpeg disagrees with independent multiplicative oracle")
        sf_values, sf_rate = sf.read(io.BytesIO(encoded), dtype="int16", always_2d=True)
        sf_raw = sf_values.astype("<i2").tobytes()
        assert sf_rate == rate and sf_raw == full_spec, (name, "libsndfile differs from complete spec oracle")
        assert sf_raw[:len(expected)] == expected
        raw_name = path.stem + ".s16le"
        (OUT / raw_name).write_bytes(expected)
        a = struct.unpack("<" + "h" * (len(full_spec) // 2), full_spec)
        b = struct.unpack("<" + "h" * (len(raw) // 2), raw)
        differences = [abs(x - y) for x, y in zip(a, b)]
        manifest["Cases"].append({"File": name, "PCMFile": raw_name, "Origin": origin,
            "Rate": rate, "Channels": channels, "Frames": frames, "CapacityFrames": len(full_spec) // (2 * channels),
            "EncodedSHA256": sha(encoded), "PCMSHA256": sha(expected), "FFmpegWholeSHA256": sha(raw),
            "LibsndfileWholeSHA256": sha(sf_raw), "FFmpegWholeBytes": len(raw),
            "FFmpegDifferentSamples": sum(d != 0 for d in differences), "FFmpegMaxCodeDifference": max(differences, default=0),
            "DiscardMetadata": b"LIST" in chunks(encoded)})
        print(name, frames, "frames", "FFmpeg differing codes", sum(d != 0 for d in differences), flush=True)
    (OUT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print("saved", len(streams), "independent streams")


if __name__ == "__main__":
    main()
