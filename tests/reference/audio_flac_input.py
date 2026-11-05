#!/usr/bin/env python3
"""Development-only libFLAC -> Go / FFmpeg / libsndfile complete PCM oracle.

Run from the repository root; all generated files stay under tests/data/audio.
No external program or library participates in ReadFLAC or ordinary Go tests.
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


def run(args, **kw):
    p = subprocess.run([str(x) for x in args], capture_output=True, timeout=120, **kw)
    if p.returncode:
        raise RuntimeError(f'{args}: {p.stderr.decode(errors="replace")}')
    return p.stdout


def pcm(depth, channels, frames, pattern):
    scale = 1 << (depth - 1)
    rng = random.Random(20261104 + depth * 100 + channels)
    codes = []
    for f in range(frames):
        for c in range(channels):
            if pattern == 'constant':
                v = -scale + 1 + c
            elif pattern == 'extremes':
                v = [-scale, scale - 1, -1, 0, 1][(f + c) % 5]
            elif pattern == 'random':
                v = rng.randrange(-scale, scale)
            elif pattern == 'ramp':
                v = (f * (c + 1)) % 63 - 31
            else:
                v = int((scale - 1) * .65 * math.sin(f * (0.031 + c * 0.003)))
                if pattern == 'wasted':
                    v = (v >> min(7, depth - 2)) << min(7, depth - 2)
            codes.append(v)
    raw = b''.join(v.to_bytes(depth // 8, 'little', signed=True) for v in codes)
    wide = b''.join((v << (32 - depth)).to_bytes(4, 'little', signed=True) for v in codes)
    return raw, wide


def verify_hand(root, directory):
    hand = directory / 'hand'
    env = dict(os.environ, CGO_ENABLED='0', GOMATH_FLAC_HAND_DUMP=str(hand))
    print(run(['go', 'run', './tests/cmd/run', '-pkg=audio',
               '-run=^TestReadFLACInputHandVectors$', '-count=1'], cwd=root, env=env).decode(), end='')
    entries = json.loads((hand / 'manifest.json').read_text())
    verified = []
    for e in entries:
        path = hand / e['File']
        raw = path.with_suffix('.raw').read_bytes()
        depth = e['Depth']
        codes = [int.from_bytes(raw[i:i+depth//8], 'little', signed=True)
                 for i in range(0, len(raw), depth//8)]
        wide = b''.join((v << (32-depth)).to_bytes(4, 'little', signed=True) for v in codes)
        for p in (path, path.with_name(path.stem + '_explicit.flac')):
            decoded = run(['flac', '-d', '-f', '-s', '--force-raw-format',
                           '--endian=little', '--sign=signed', '-c', p])
            if decoded != raw:
                raise RuntimeError(f'libFLAC hand decode differs: {p}')
            decoded = run(['ffmpeg', '-v', 'error', '-xerror', '-err_detect',
                           'crccheck+explode', '-i', p, '-c:a', 'pcm_s32le',
                           '-f', 's32le', 'pipe:1'])
            if decoded != wide:
                raise RuntimeError(f'FFmpeg hand decode differs: {p}')
            verified.append(dict(e, File=p.name,
                                 SHA256=hashlib.sha256(p.read_bytes()).hexdigest(),
                                 PCMSHA256=hashlib.sha256(raw).hexdigest()))
    report = {'cases': len(verified), 'scalar_samples': sum(e['Frames']*e['Channels'] for e in verified),
              'comparison': 'Go hand-builder vectors, inherited+explicit header depth; full libFLAC raw + FFmpeg widened int32 equal original PCM',
              'verified': verified}
    (hand / 'verification.json').write_text(json.dumps(report, indent=2) + '\n')
    print(f'PASS: {len(verified)} hand streams through complete libFLAC and FFmpeg decoding')


def verify(root, directory, generate=True, go=True):
    import soundfile as sf
    import numpy as np
    flac, ffmpeg = shutil.which('flac'), shutil.which('ffmpeg')
    if not flac or not ffmpeg:
        raise RuntimeError('development verification requires flac and ffmpeg')
    directory.mkdir(parents=True, exist_ok=True)
    manifest, features = [], set()
    # Every depth/channel pair is checked against six patterns, including full
    # precision random low bits, true signed extremes, and a partial last block.
    cases = [(d, c, n, p, b) for d in (8, 16, 24, 32) for c in range(1, 9)
             for n, p, b in ((1, 'constant', 16), (17, 'extremes', 16),
                            (255, 'random', 256), (257, 'wasted', 256),
                            (1025, 'sine', 256), (513, 'ramp', 256))]
    # Additional real LPC high-order input and largest legal block exercise
    # decoder scratch bounds without making ordinary suites encode anything.
    cases += [(32, 8, 4097, 'sine', 4096), (32, 2, 65535, 'wasted', 65535)]
    sf_results = {'passed': 0, 'unsupported': []}
    for depth, channels, frames, pattern, block in cases:
        name = f'pcm{depth}_{channels}ch_{pattern}_n{frames}_b{block}'
        raw, wide = pcm(depth, channels, frames, pattern)
        src, dst = directory / (name + '.raw'), directory / (name + '.flac')
        if generate:
            src.write_bytes(raw)
            args = [flac, '-f', '-s', '-V', '-8', '--lax', '--no-padding',
                    '--no-seektable', '--force-raw-format', '--endian=little',
                    '--sign=signed', f'--bps={depth}', f'--channels={channels}',
                    '--sample-rate=48000', '-b', str(block), '-o', dst]
            if frames == 4097:
                args += ['-l', '32', '-q', '15', '-r', '0,7']
            run(args + [src])
        if src.read_bytes() != raw:
            raise RuntimeError(f'input oracle changed: {src}')
        decoded = run([flac, '-d', '-f', '-s', '--force-raw-format', '--endian=little',
                       '--sign=signed', '-c', dst])
        if decoded != raw:
            raise RuntimeError(f'libFLAC full precision decode differs: {dst}')
        decoded = run([ffmpeg, '-v', 'error', '-xerror', '-err_detect',
                       'crccheck+explode', '-i', dst, '-c:a', 'pcm_s32le',
                       '-f', 's32le', 'pipe:1'])
        if decoded != wide:
            raise RuntimeError(f'FFmpeg complete widened PCM differs: {dst}')
        try:
            samples, rate = sf.read(dst, dtype='int32', always_2d=True)
        except sf.LibsndfileError as e:
            # A decoder's explicit unsupported format is recorded, never
            # replaced with lower precision or fewer channels.
            sf_results['unsupported'].append({'File': dst.name, 'error': str(e)})
        else:
            if rate != 48000 or samples.shape != (frames, channels):
                raise RuntimeError(f'libsndfile shape/rate differs: {dst}')
            if np.asarray(samples, dtype='<i4').tobytes() != wide:
                raise RuntimeError(f'libsndfile complete widened PCM differs: {dst}')
            sf_results['passed'] += 1
        ana = directory / (name + '.ana')
        run([flac, '-f', '-s', '-a', '-o', ana, dst])
        features.update(re.findall(r'(?:type|channel_assignment|order|partition_order|wasted_bits)=\S+', ana.read_text()))
        manifest.append({'File': dst.name, 'SHA256': hashlib.sha256(dst.read_bytes()).hexdigest(),
                         'PCMSHA256': hashlib.sha256(raw).hexdigest(), 'Rate': 48000,
                         'Depth': depth, 'Channels': channels, 'Frames': frames})
    (directory / 'manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    if go:
        env = dict(os.environ, CGO_ENABLED='0', GOMATH_FLAC_INPUT_DIR=str(directory))
        print(run(['go', 'run', './tests/cmd/run', '-pkg=audio',
                   '-run=^TestReadFLACInputFixtures$', '-count=1'], cwd=root, env=env).decode(), end='')
    report = {'date': '2026-11-04', 'libFLAC': run([flac, '--version']).decode().strip(),
              'ffmpeg': run([ffmpeg, '-version']).decode().splitlines()[0],
              'libsndfile': sf.__libsndfile_version__, 'soundfile': sf.__version__,
              'cases': len(manifest), 'scalar_samples': sum(e['Frames'] * e['Channels'] for e in manifest),
              'comparison': 'all frames/channels; libFLAC raw == signed input; FFmpeg/libsndfile int32 == exact left-aligned input; Go == original bytes+hash',
              'libsndfile_results': sf_results, 'features': sorted(features), 'go_verified': go}
    (directory / 'verification.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report, indent=2))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--directory', type=Path)
    parser.add_argument('--verify-only', action='store_true')
    parser.add_argument('--skip-go', action='store_true')
    parser.add_argument('--hand-only', action='store_true')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    directory = (args.directory or root / 'tests/data/audio/flac_input').resolve()
    if not directory.is_relative_to(root / 'tests'):
        raise SystemExit('reference output must be under tests/')
    if not args.hand_only:
        verify(root, directory, not args.verify_only, not args.skip_go)
    if not args.skip_go:
        verify_hand(root, directory)
