#!/usr/bin/env python3
"""Read-only independent full decoding of frozen FLAC fixtures (development).

Only the new flac_perf result files are written. Neither wave-five fixtures
nor their manifests/reference records are regenerated or changed.
"""
import hashlib
import json
import os
from pathlib import Path
import subprocess

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
AUDIO = ROOT / 'tests/data/audio'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def run(args, env=None):
    p = subprocess.run([str(x) for x in args], cwd=ROOT, env=env,
                       capture_output=True, timeout=120)
    if p.returncode:
        raise RuntimeError(f'{args}: {p.stderr.decode(errors="replace")}')
    return p.stdout


def main():
    cases = []
    for folder in ('flac_decode', 'flac_input'):
        for e in json.loads((AUDIO / folder / 'manifest.json').read_text()):
            cases.append((AUDIO / folder, e, e['SHA256'], e['PCMSHA256']))
    for e in json.loads((AUDIO / 'flac_input/hand/verification.json').read_text())['verified']:
        cases.append((AUDIO / 'flac_input/hand', e, e['SHA256'], e['PCMSHA256']))
    for e in json.loads((AUDIO / 'flac/fixtures.json').read_text()):
        cases.append((AUDIO / 'flac', e, e['FLACSHA256'], e['SHA256']))
    report = {'date': '2026-11-05',
              'flac': run(['flac', '--version']).decode().strip(),
              'ffmpeg': run(['ffmpeg', '-version']).decode().splitlines()[0],
              'cases': [], 'comparison': 'full frame/channel counts and signed PCM bytes, no trim/channel conversion; original PCM hashes'}
    for folder, e, encoded_sha, pcm_sha in cases:
        path = folder / e['File']
        before = sha(path.read_bytes())
        if before != encoded_sha:
            raise RuntimeError(f'frozen encoded input changed: {path}')
        raw = run(['flac', '-d', '-f', '-s', '--force-raw-format', '--endian=little',
                   '--sign=signed', '-c', path])
        width = e['Depth'] // 8
        if len(raw) != e['Frames']*e['Channels']*width or sha(raw) != pcm_sha:
            raise RuntimeError(f'complete libFLAC PCM mismatch: {path}')
        # Widen smaller depths exactly; signed low bits must remain zero.
        # No 32-bit sample is truncated to 24 bits for this comparison.
        wide = b''.join((int.from_bytes(raw[i:i+width], 'little', signed=True) << (32-e['Depth'])).to_bytes(4, 'little', signed=True)
                        for i in range(0, len(raw), width))
        other = run(['ffmpeg', '-v', 'error', '-xerror', '-err_detect', 'crccheck+explode',
                     '-i', path, '-c:a', 'pcm_s32le', '-f', 's32le', 'pipe:1'])
        if other != wide:
            raise RuntimeError(f'complete strict FFmpeg PCM mismatch: {path}')
        if sha(path.read_bytes()) != before:
            raise RuntimeError(f'input changed during decode: {path}')
        report['cases'].append({'file': str(path.relative_to(ROOT)), 'encoded_sha256': before,
                                'pcm_sha256': pcm_sha, 'frames': e['Frames'],
                                'channels': e['Channels'], 'depth': e['Depth']})
    env = dict(os.environ, CGO_ENABLED='0', GOMAXPROCS='1')
    # bench.py builds the same central test sources for pinned before/current.
    for variant in ('before', 'after'):
        output = run([HERE / '_bin' / f'{variant}.test',
                      '-test.run=^Test(ReadFLAC|FLACVerifiedFixtures)',
                      '-test.count=1'], env=env)
        (HERE / f'reference-go-{variant}.txt').write_bytes(output)
    report['total_streams'] = len(cases)
    report['total_scalar_samples'] = sum(e['frames']*e['channels'] for e in report['cases'])
    report['go_variants'] = 'pinned before and candidate/current: full ReadFLAC test set passed, including original per-sample/hash oracles, exact hand bytes, invalidity and budgets'
    (HERE / 'reference.json').write_text(json.dumps(report, indent=2) + '\n')
    print(f'PASS {len(cases)} complete streams; {report["total_scalar_samples"]} scalars; libFLAC + strict FFmpeg + before/after Go tests')


if __name__ == '__main__':
    main()
