#!/usr/bin/env python3
"""Rebuild pinned before/current FLAC tests and run alternating A/B samples.

Development only. Run from the module root. All artifacts stay in this folder.
The before source is a text snapshot, never copied into a production directory.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import statistics
import subprocess


HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
PATTERN = '^BenchmarkReadFLAC(48kStereo|PerfFixtures)$'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(command, logfile, environment):
    with logfile.open('w') as log:
        result = subprocess.run([str(x) for x in command], cwd=ROOT, env=environment,
                                stdout=log, stderr=subprocess.STDOUT, timeout=180)
    if result.returncode:
        raise RuntimeError(f'{command}: see {logfile}')


def build(variant, source, environment):
    directory = HERE / '_build'
    directory.mkdir(exist_ok=True)
    mapping = {str(ROOT / 'audio' / f.name): str(f)
               for f in (ROOT / 'tests/_cases/audio').glob('*_test.go')}
    mapping[str(ROOT / 'audio/flac_read.go')] = str(source)
    overlay = directory / f'{variant}.json'
    overlay.write_text(json.dumps({'Replace': mapping}, indent=2) + '\n')
    binary = HERE / '_bin' / f'{variant}.test'
    binary.parent.mkdir(exist_ok=True)
    run(['go', 'run', './tests/cmd/run', '-pkg=audio', f'-overlay={overlay}',
         '-c', f'-o={binary}'], HERE / f'build-{variant}.txt', environment)
    return binary


def parse(text):
    values = {}
    pattern = r'(Benchmark\S+)\s+\d+\s+(\d+) ns/op\s+(?:[\d.]+ MB/s\s+)?(\d+) B/op\s+(\d+) allocs/op'
    for line in text.splitlines():
        m = re.match(pattern, line)
        if m:
            name, ns, nb, na = m.groups()
            values[name] = {'ns': int(ns), 'B': int(nb), 'allocs': int(na)}
    if len(values) != 9:
        raise RuntimeError(f'expected nine measured workloads, got {len(values)}')
    return values


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--rounds', type=int, default=5)
    parser.add_argument('--benchtime', default='300ms')
    parser.add_argument('--after-source', type=Path, default=ROOT / 'audio/flac_read.go')
    args = parser.parse_args()
    before, after = HERE / 'before.go.txt', args.after_source.resolve()
    baseline = json.loads((HERE / 'baseline.json').read_text())
    if sha(before) != baseline['before_source_sha256']:
        raise RuntimeError('before source changed')
    env = dict(os.environ, CGO_ENABLED='0', GOMAXPROCS='1')
    bins = {v: build(v, source, env) for v, source in [('before', before), ('after', after)]}
    report = {'date': '2026-11-05', 'go': subprocess.check_output(['go', 'version'], text=True).strip(),
              'config': {'CGO_ENABLED': '0', 'GOMAXPROCS': '1', 'tags': 'noasm',
                         'test.cpu': '1', 'benchtime': args.benchtime, 'rounds': args.rounds},
              'source_sha256': {'before': sha(before), 'after': sha(after)},
              'binary_sha256': {v: sha(b) for v, b in bins.items()},
              'test_sources_sha256': {str(f.relative_to(ROOT)): sha(f) for f in sorted((ROOT / 'tests/_cases/audio').glob('*.go'))},
              'exclusions': ['input encoding and file I/O', 'compilation', 'external decoders',
                             'process startup and one-time CRC table initialization'],
              'host_caveat': 'shared Apple M2 host; no machine isolation; all samples retained', 'runs': []}
    samples = {'before': {}, 'after': {}}
    for i in range(args.rounds):
        for variant in (('before', 'after') if i % 2 == 0 else ('after', 'before')):
            log = HERE / f'interleaved-{i+1}-{variant}.txt'
            command = [bins[variant], '-test.run=^$', f'-test.bench={PATTERN}',
                       '-test.benchmem', f'-test.benchtime={args.benchtime}',
                       '-test.count=1', '-test.cpu=1']
            run(command, log, env)
            parsed = parse(log.read_text())
            for name, measurement in parsed.items():
                samples[variant].setdefault(name, []).append(measurement)
            report['runs'].append({'round': i+1, 'variant': variant, 'file': log.name,
                                   'sha256': sha(log), 'measurements': parsed})
            print(f'{i+1}/{args.rounds} {variant}: PASS', flush=True)
    report['summary'] = []
    for name in samples['before']:
        b, a = samples['before'][name], samples['after'][name]
        bm, am = statistics.median(x['ns'] for x in b), statistics.median(x['ns'] for x in a)
        paired = [(y['ns']/x['ns']-1)*100 for x, y in zip(b, a)]
        report['summary'].append({'name': name, 'before_median_ns': bm, 'after_median_ns': am,
                                  'median_delta_percent': (am/bm-1)*100,
                                  'paired_delta_percent': paired,
                                  'before_range_ns': [min(x['ns'] for x in b), max(x['ns'] for x in b)],
                                  'after_range_ns': [min(x['ns'] for x in a), max(x['ns'] for x in a)],
                                  'before_B_per_op': [x['B'] for x in b], 'after_B_per_op': [x['B'] for x in a],
                                  'before_allocs_per_op': [x['allocs'] for x in b], 'after_allocs_per_op': [x['allocs'] for x in a]})
    if sha(before) != report['source_sha256']['before'] or sha(after) != report['source_sha256']['after']:
        raise RuntimeError('source changed during measurement')
    (HERE / 'interleaved.json').write_text(json.dumps(report, indent=2) + '\n')
    for row in report['summary']:
        print(row['name'], round(row['median_delta_percent'], 2), '%')


if __name__ == '__main__':
    main()
