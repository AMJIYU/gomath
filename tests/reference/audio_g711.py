#!/usr/bin/env python3
"""Development-only G.711/WAVE reference generation; never invoked by Go tests.

Requires FFmpeg, Python soundfile/libsndfile and numpy. The independent midpoint
oracle follows ITU-T G.711 tables; it does not import or run gomath. All complete
external outputs are compared without duration/trim/channel conversion options.
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
OUT = ROOT / "tests/data/audio/g711"
SOURCES = [
    {"title": "ITU-T G.711 (11/1988)",
     "url": "https://www.itu.int/rec/dologin_pub.asp?lang=e&id=T-REC-G.711-198811-I!!PDF-E&type=items",
     "sha256": "4348b527fbb1488677503d3b0f65094fb8a4b01e159ce01d7adbc29d5efdc556",
     "section": "Tables 1a/b, 2a/b: sign, segment, quantization interval and alternating/complemented code bits"},
    {"title": "Microsoft Multimedia Standards Update, Revision3.0 (1994-04-15)",
     "url": "https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/Docs/RIFFNEW.pdf",
     "sha256": "b2473ccc73d0f6378f7f56b9f47bea10a9d90a162e7be2d31049cf664c0eb303",
     "section": "WAVEFORMATEX, fact and registered mu-law/A-law formats"},
    {"title": "Microsoft/IBM Multimedia Programming Interface and Data Specifications1.0 (1991)",
     "url": "https://www.mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/Docs/riffmci.pdf",
     "sha256": "d2340df3978259b366cbd5acaff049bb3fe81d0d345335a946ee25e4f8b167fa",
     "section": "RIFF zero pad, WAVE fmt-before-data, non-PCM fact sample count per channel"},
]


def sha(b):
    return hashlib.sha256(b).hexdigest()


def run(args, data=None, strict=True):
    p = subprocess.run(args, input=data, capture_output=True)
    if strict and p.returncode:
        raise RuntimeError((args, p.stderr.decode(errors="replace")))
    return p


def chunk(kind, payload):
    return kind + struct.pack("<I", len(payload)) + payload + bytes(len(payload) & 1)


def riff(entries):
    body = b"WAVE" + b"".join(chunk(k, b) for k, b in entries)
    return b"RIFF" + struct.pack("<I", len(body)) + body


def parts(w):
    assert w[:4] == b"RIFF" and w[8:12] == b"WAVE" and len(w) == struct.unpack_from("<I", w, 4)[0] + 8
    result = {}
    p = 12
    while p < len(w):
        k = w[p:p + 4]
        n, = struct.unpack_from("<I", w, p + 4)
        assert k not in result
        result[k] = w[p + 8:p + 8 + n]
        p += 8 + n + (n & 1)
    assert p == len(w)
    return result


def standard_codes(tag):
    # Quantization interval midpoints and increments, expanded to signed16
    # scale from the A-law13-bit / mu-law14-bit linear domains of G.711.
    base = [8, 264, 528, 1056, 2112, 4224, 8448, 16896] if tag == 6 else [0, 132, 396, 924, 1980, 4092, 8316, 16764]
    increment = [16, 16, 32, 64, 128, 256, 512, 1024] if tag == 6 else [8, 16, 32, 64, 128, 256, 512, 1024]
    result = []
    for code in range(256):
        wire = code ^ (0x55 if tag == 6 else 0xff)
        segment, mantissa = (wire >> 4) & 7, wire & 15
        magnitude = base[segment] + increment[segment] * mantissa
        positive = bool(wire & 128) if tag == 6 else not bool(wire & 128)
        result.append(magnitude if positive else -magnitude)
    return result


def wave(tag, channels, frames, rate=8000, order="normal"):
    fmt = struct.pack("<HHIIHHH", tag, channels, rate, rate * channels, channels, 8, 0)
    data = bytes((frame + 37 * ch) & 255 for frame in range(frames) for ch in range(channels))
    entries = [(b"fmt ", fmt), (b"fact", struct.pack("<I", frames)), (b"data", data)]
    if order == "fact_first":
        entries[0], entries[1] = entries[1], entries[0]
    elif order == "fact_after":
        entries[1], entries[2] = entries[2], entries[1]
    elif order == "junk_pad":
        entries.insert(0, (b"JUNK", b"\x91"))
        entries.append((b"PAD ", b"\x23\x24\x25"))
    elif order == "metadata":
        entries.append((b"LIST", b"INFO" + chunk(b"INAM", b"reference\0")))
    return riff(entries)


def ffmpeg_decode(path, strict=True):
    return run(["ffmpeg", "-v", "error", "-xerror", "-err_detect", "explode", "-i", str(path),
                "-map", "0:a:0", "-c:a", "pcm_s16le", "-f", "s16le", "pipe:1"], strict=strict)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    manifest = {"Sources": SOURCES, "FFmpegVersion": run(["ffmpeg", "-version"]).stdout.decode().splitlines()[0],
                "LibsndfileVersion": sf.__libsndfile_version__, "Cases": [], "RejectedCases": []}
    streams = []
    extensible = []
    for tag, law in ((6, "alaw"), (7, "mulaw")):
        for ch in (1, 2, 3, 6, 64):
            streams.append((f"spec_{law}_all_codes_{ch}ch", wave(tag, ch, 256), "ITU midpoint oracle, every code in each channel"))
        for rate in (1, 768000):
            streams.append((f"spec_{law}_rate{rate}", wave(tag, 1, 17, rate), "exact integer sample rate"))
        for ch, frames in ((1, 0), (1, 1), (1, 17), (1, 32769), (3, 16385)):
            streams.append((f"spec_{law}_{ch}ch_n{frames}", wave(tag, ch, frames), "empty, odd-pad and 32KiB boundary"))
        for order in ("fact_first", "fact_after", "junk_pad", "metadata"):
            streams.append((f"spec_{law}_{order}", wave(tag, 3, 17, order=order), "independent RIFF ordering/pad/metadata construction"))
        for ch in (1, 2, 6):
            values = [int(30000 * math.sin((f + 1) * (17 + c * 7) * .023)) for f in range(257) for c in range(ch)]
            source = struct.pack("<" + "h" * len(values), *values)
            path = OUT / f"ffmpeg_{law}_{ch}ch.wav"
            run(["ffmpeg", "-v", "error", "-f", "s16le", "-ar", "44100", "-ac", str(ch), "-i", "pipe:0",
                 "-map_metadata", "-1", "-c:a", "pcm_alaw" if tag == 6 else "pcm_mulaw", "-y", str(path)], source)
            encoded = path.read_bytes()
            if len(parts(encoded)[b"fmt "]) == 40:
                extensible.append((path, encoded, tag))
            else:
                streams.append((path.stem, encoded, "FFmpeg encoder from explicit signed16 sine"))
        for ch in (1, 2, 3):
            values = np.array([(-32768, 32767, -1, 0, 1)[i % 5] for i in range(257 * ch)], dtype=np.int16).reshape(-1, ch)
            stream = io.BytesIO()
            sf.write(stream, values, 22050, format="WAV", subtype="ALAW" if tag == 6 else "ULAW")
            streams.append((f"libsndfile_{law}_{ch}ch", stream.getvalue(), "libsndfile encoder from explicit signed16 extrema/zero"))

    tables = {"ALaw": standard_codes(6), "MuLaw": standard_codes(7)}
    (OUT / "code_tables.json").write_text(json.dumps(tables, indent=2) + "\n")
    for name, encoded, origin in streams:
        info = parts(encoded)
        tag, channels, rate, avg, align, depth, cb = struct.unpack("<HHIIHHH", info[b"fmt "])
        assert depth == 8 and cb == 0 and align == channels and avg == rate * channels
        frames, = struct.unpack("<I", info[b"fact"])
        table = standard_codes(tag)
        codes = [table[c] for c in info[b"data"]]
        expected = struct.pack("<" + "h" * len(codes), *codes)
        assert frames * channels == len(codes)
        path = OUT / (name + ".wav")
        path.write_bytes(encoded)
        ff = ffmpeg_decode(path).stdout
        values, sf_rate = sf.read(io.BytesIO(encoded), dtype="int16", always_2d=True)
        snd = values.astype("<i2").tobytes()
        assert ff == expected and snd == expected, (name, "independent complete PCM mismatch")
        assert sf_rate == rate and values.shape == (frames, channels)
        rawname = name + ".s16le"
        (OUT / rawname).write_bytes(expected)
        manifest["Cases"].append({"File": path.name, "PCMFile": rawname, "Origin": origin,
            "Tag": tag, "Channels": channels, "Rate": rate, "Frames": frames,
            "EncodedSHA256": sha(encoded), "PCMSHA256": sha(expected),
            "FFmpegWholeSHA256": sha(ff), "LibsndfileWholeSHA256": sha(snd),
            "DiscardMetadata": b"LIST" in info})
        print(name, frames, "frames, complete PCM exact in FFmpeg/libsndfile", flush=True)

    # Preserve actual FFmpeg multichannel extensible layouts as unsupported;
    # never strip the GUID/mask to turn them into indexed-channel fixtures.
    for path, encoded, tag in extensible:
        codes = [standard_codes(tag)[c] for c in parts(encoded)[b"data"]]
        expected = struct.pack("<" + "h" * len(codes), *codes)
        ff = ffmpeg_decode(path).stdout
        values, _ = sf.read(io.BytesIO(encoded), dtype="int16", always_2d=True)
        snd = values.astype("<i2").tobytes()
        assert ff == expected and snd == expected
        manifest["RejectedCases"].append({"File": path.name, "EncodedSHA256": sha(encoded), "Category": "unsupported",
            "Reason": "real FFmpeg extensible compression GUID and speaker layout; preserved verbatim",
            "FFmpeg": {"Accepted": True, "WholeBytes": len(ff), "WholeSHA256": sha(ff)},
            "Libsndfile": {"Accepted": True, "ScalarSamples": int(values.size), "WholeSHA256": sha(snd)}})

    # Record external permissiveness on structurally unsupported/invalid WAVE.
    # No fallback/trim is used by gomath; normal Go tests reject every case.
    for tag, law in ((6, "alaw"), (7, "mulaw")):
        entries = list(parts(wave(tag, 1, 1)).items())
        for reason in ("missing_fact", "fact_short", "fact_long", "fmt16", "cbSize2", "nonzero_pad", "extensible"):
            e = list(entries)
            category = "invalid"
            if reason == "missing_fact":
                e = [(k, v) for k, v in e if k != b"fact"]
            elif reason in ("fact_short", "fact_long"):
                e[1] = (b"fact", struct.pack("<I", 0 if reason == "fact_short" else 2))
            elif reason == "fmt16":
                e[0] = (b"fmt ", e[0][1][:16])
                category = "unsupported"
            elif reason == "cbSize2":
                e[0] = (b"fmt ", e[0][1][:16] + b"\x02\x00")
                category = "unsupported"
            elif reason == "extensible":
                fmt = bytearray(e[0][1][:16] + struct.pack("<HHI", 22, 8, 4) + struct.pack("<I", tag) + bytes.fromhex("00001000800000aa00389b71"))
                struct.pack_into("<H", fmt, 0, 0xfffe)
                e[0] = (b"fmt ", bytes(fmt))
                category = "unsupported"
            encoded = riff(e)
            if reason == "nonzero_pad":
                encoded = encoded[:-1] + b"\x91"
            path = OUT / f"reject_{law}_{reason}.wav"
            path.write_bytes(encoded)
            ff = ffmpeg_decode(path, strict=False)
            try:
                snd, _ = sf.read(io.BytesIO(encoded), dtype="int16", always_2d=True)
                sf_result = {"Accepted": True, "ScalarSamples": int(snd.size), "WholeSHA256": sha(snd.astype("<i2").tobytes())}
            except Exception as error:
                sf_result = {"Accepted": False, "Error": str(error)}
            manifest["RejectedCases"].append({"File": path.name, "EncodedSHA256": sha(encoded), "Category": category,
                "FFmpeg": {"Accepted": ff.returncode == 0, "WholeBytes": len(ff.stdout), "WholeSHA256": sha(ff.stdout), "Stderr": ff.stderr.decode(errors="replace")},
                "Libsndfile": sf_result})
    (OUT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print("saved", len(streams), "accepted streams and", len(manifest["RejectedCases"]), "explicit rejections")


if __name__ == "__main__":
    main()
