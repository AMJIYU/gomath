#!/usr/bin/env python3
"""Development oracle: libFLAC encode -> FFmpeg and pure Go complete decoding.

python3 tests/data/audio/verify_flac_read.py [--keep /absolute/directory]
Add --retain-fixtures to update the small committed fixture selection after a
successful full comparison. Requires flac (libFLAC CLI), FFmpeg, Python and Go
only for development; ordinary tests need none of the external codec tools.
"""
import argparse
import hashlib
import json
import math
import os
from pathlib import Path
import random
import re
import shutil
import subprocess
import tempfile


def run(args, **kw):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            timeout=120, **kw)
    if result.returncode:
        raise RuntimeError(f"{args}: {result.stderr.decode(errors='replace')}")
    return result.stdout


def make_raw(depth, channels, frames, pattern):
    rng = random.Random(90421 + frames * 13 + depth * 3 + channels)
    scale = 1 << (depth - 1)
    raw = bytearray()
    for frame in range(frames):
        base = int(.7 * (scale - 1) * math.sin(2 * math.pi * 731 * frame / 48000))
        for channel in range(channels):
            if pattern == 'silence':
                code = 0
            elif pattern == 'extremes':
                code = [-scale, scale - 1, 0, -1, 1][(frame + channel) % 5]
            elif pattern == 'random':
                code = rng.randrange(-scale, scale)
            elif pattern == 'wasted':
                code = (base + channel * 128) & ~255
            elif pattern == 'ramp':
                code = ((frame * (channel + 1)) % 5000) - 2500
            elif pattern == 'correlated':
                code = base + (frame % 11) * channel
            else:
                code = int(.4 * (scale - 1) * (
                    math.sin(2 * math.pi * (521 + channel * 77) * frame / 48000) +
                    .6 * math.sin(2 * math.pi * 1259 * frame / 48000)))
            raw.extend(code.to_bytes(depth // 8, 'little', signed=True))
    return bytes(raw)


def verify(root, out, retain):
    flac, ffmpeg = shutil.which('flac'), shutil.which('ffmpeg')
    if not flac or not ffmpeg:
        raise SystemExit('development verification requires libFLAC flac and FFmpeg')
    out.mkdir(parents=True, exist_ok=True)
    cases = []
    lengths = [1, 15, 16, 17, 255, 256, 257, 4095, 4096, 4097, 10001]
    patterns = ['silence', 'extremes', 'random', 'wasted', 'ramp', 'correlated', 'sine']
    for depth in [16, 24]:
        for channels in [1, 2]:
            for i, frames in enumerate(lengths):
                cases.append((depth, channels, frames, patterns[i % len(patterns)],
                              [0, 5, 8][i % 3], False))
            for level in [0, 5, 8]:
                for pattern in patterns:
                    cases.append((depth, channels, 8193, pattern, level, False))
            cases.append((depth, channels, 8193, 'sine', 8, True))
    manifest, analyses, lpc_orders = [], [], set()
    for depth, channels, frames, pattern, level, extended in cases:
        name = f'libflac_pcm{depth}_{channels}ch_{pattern}_n{frames}_l{level}'
        if extended:
            name += '_lpc32'
        raw = make_raw(depth, channels, frames, pattern)
        src, dst = out / (name + '.raw'), out / (name + '.flac')
        src.write_bytes(raw)
        args = [flac, '-f', '-s', '-V', f'-{level}', '--force-raw-format',
                '--endian=little', '--sign=signed', f'--bps={depth}',
                f'--channels={channels}', '--sample-rate=48000', '-b', '256',
                '-T', 'TITLE=独立 libFLAC fixture', '-o', str(dst)]
        if extended:
            args += ['--lax', '-l', '32', '-q', '15', '-r', '0,7']
        run(args + [str(src)])
        fmt = f's{depth}le'
        decoded = run([ffmpeg, '-v', 'error', '-xerror', '-err_detect',
                       'crccheck+explode', '-i', str(dst), '-c:a', 'pcm_' + fmt,
                       '-f', fmt, 'pipe:1'])
        if decoded != raw:
            raise RuntimeError(f'FFmpeg decoded PCM differs: {dst}')
        analysis = out / (name + '.ana')
        run([flac, '-f', '-s', '-a', '-o', str(analysis), str(dst)])
        text = analysis.read_text()
        lpc_orders.update(int(x) for x in re.findall(r'type=LPC\s+order=(\d+)', text))
        analyses.extend(re.findall(r'(?:type|channel_assignment|order|partition_order|wasted_bits)=\S+', text))
        manifest.append({'File': dst.name, 'SHA256': hashlib.sha256(dst.read_bytes()).hexdigest(),
                         'PCMSHA256': hashlib.sha256(raw).hexdigest(),
                         'Rate': 48000, 'Depth': depth, 'Channels': channels,
                         'Frames': frames})
    (out / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    env = dict(os.environ, CGO_ENABLED='0', GOMATH_FLAC_DECODE_INPUT=str(out))
    print(run(['go', 'run', './tests/cmd/run', '-pkg=audio',
               '-run=^TestReadFLACLibFLACFixtures$', '-count=1'], cwd=root,
              env=env).decode(), end='')
    report = {'date': '2026-11-04', 'libFLAC': run([flac, '--version']).decode().strip(),
              'ffmpeg': run([ffmpeg, '-version']).decode().splitlines()[0],
              'cases': len(manifest),
              'scalar_samples': sum(x['Frames'] * x['Channels'] for x in manifest),
              'comparison': 'integer input == complete FFmpeg decode == complete pure-Go decode (all bytes)',
              'analysis_features': sorted(set(analyses)), 'lpc_orders': sorted(lpc_orders),
              'verified': manifest}
    (out / 'verification.json').write_text(json.dumps(report, indent=2) + '\n')
    if retain:
        fixture_dir = root / 'tests/data/audio/flac_decode'
        fixture_dir.mkdir(exist_ok=True)
        chosen = [x for x in manifest if (
            x['File'] == 'libflac_pcm16_1ch_silence_n1_l0.flac' or
            x['File'] == 'libflac_pcm16_2ch_extremes_n15_l5.flac' or
            x['File'] == 'libflac_pcm24_2ch_random_n16_l8.flac' or
            x['File'] == 'libflac_pcm24_1ch_wasted_n17_l0.flac' or
            x['File'] == 'libflac_pcm16_2ch_correlated_n8193_l8.flac' or
            x['File'] == 'libflac_pcm24_2ch_sine_n8193_l8_lpc32.flac' or
            x['File'] == 'libflac_pcm24_1ch_ramp_n8193_l8.flac' or
            x['File'] == 'libflac_pcm16_1ch_sine_n257_l0.flac')]
        if len(chosen) != 8:
            raise RuntimeError('fixture selection incomplete')
        for entry in chosen:
            shutil.copyfile(out / entry['File'], fixture_dir / entry['File'])
        (fixture_dir / 'manifest.json').write_text(json.dumps(chosen, indent=2) + '\n')
        shutil.copyfile(out / 'verification.json', fixture_dir / 'verification.json')
    print(f"PASS: libFLAC -> FFmpeg and Go; {report['cases']} files; {report['scalar_samples']:,} exact samples")
    print('Features:', ', '.join(report['analysis_features']))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--keep', type=Path)
    parser.add_argument('--retain-fixtures', action='store_true')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[3]
    if args.keep:
        verify(root, args.keep.resolve(), args.retain_fixtures)
    else:
        with tempfile.TemporaryDirectory(prefix='gomath-flac-read-') as directory:
            verify(root, Path(directory), args.retain_fixtures)


if __name__ == '__main__':
    main()
