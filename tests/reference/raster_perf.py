#!/usr/bin/env python3
"""Development-only frozen-source A/B render measurement, from module root.

Build time, scene/text construction, worker warm-up, process startup and external
references are excluded. All output allocation, clearing, composition and GC
inside the measured render loop are included. No external renderer runs here.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import statistics
import subprocess

ROOT = Path(__file__).resolve().parents[2]
DATA = ROOT / 'tests/data/render/raster_perf'
PATTERN = '^Benchmark(RasterPerfMixed|ClippedRaster|SupersamplingMixedFrame)$'


def sha(p):
    return hashlib.sha256(p.read_bytes()).hexdigest()


def run(cmd, log, env, cwd=ROOT):
    with log.open('w') as out:
        p = subprocess.run([str(x) for x in cmd], cwd=cwd, env=env,
                           stdout=out, stderr=subprocess.STDOUT, timeout=240)
    if p.returncode:
        raise RuntimeError(f'{cmd}: see {log}')


def build(variant, source, env):
    mapping = {str(ROOT/'render'/p.name): str(p)
               for p in (ROOT/'tests/_cases/render').glob('*_test.go')}
    dependencies = json.loads((DATA/'dependencies.json').read_text())
    mapping.update({str(ROOT/rel): str(DATA/'_dependencies'/(rel+'.txt')) for rel in dependencies})
    for path in source.glob('*.go.txt' if variant == 'before' else '*.go'):
        name = path.name[:-4] if variant == 'before' else path.name
        mapping[str(ROOT/'render'/name)] = str(path)
    overlay = DATA/'_staging'/f'build-{variant}.json'
    overlay.parent.mkdir(exist_ok=True)
    overlay.write_text(json.dumps({'Replace': mapping}, indent=2)+'\n')
    binary = DATA/'_bin'/f'{variant}.test'
    binary.parent.mkdir(exist_ok=True)
    run(['go', 'run', './tests/cmd/run', '-pkg=render', f'-overlay={overlay}',
         '-c', f'-o={binary}'], DATA/f'build-{variant}.txt', env)
    return binary


def parse(s):
    values = {}
    for line in s.splitlines():
        m = re.match(r'(Benchmark\S+)\s+\d+\s+(\d+) ns/op\s+(?:[\d.]+ \S+/frame\s+)?(\d+) B/op\s+(\d+) allocs/op', line)
        if m:
            name, ns, nb, na = m.groups()
            values[name] = dict(ns=int(ns), B=int(nb), allocs=int(na))
    if len(values) != 9:
        raise RuntimeError(f'expected nine workloads, got {len(values)}')
    return values


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--after-dir', type=Path, default=ROOT/'render')
    ap.add_argument('--rounds', type=int, default=5)
    ap.add_argument('--benchtime', default='300ms')
    args = ap.parse_args()
    before, after = DATA/'_before', args.after_dir.resolve()
    baseline = json.loads((DATA/'baseline.json').read_text())
    for p in before.glob('*.go.txt'):
        if sha(p) != baseline['files']['render/'+p.name[:-4]]:
            raise RuntimeError(f'changed before snapshot: {p}')
    env = dict(os.environ, CGO_ENABLED='0', GOMAXPROCS='1')
    bins = {v: build(v, source, env) for v, source in [('before', before), ('after', after)]}
    frozen = [*list((ROOT/'render').glob('*.go')), *list(after.glob('*.go')),
              *list((ROOT/'tests/_cases/render').glob('*.go')),
              *list((DATA/'_dependencies').rglob('*.txt')), ROOT/'go.mod', ROOT/'go.sum']
    hashes = {str(p.relative_to(ROOT)): sha(p) for p in sorted(set(frozen))}
    report = dict(date='2026-11-05', go=subprocess.check_output(['go','version'],text=True).strip(),
                  config=dict(CGO_ENABLED='0', GOMAXPROCS='1', tags='noasm', cpu=1,
                              benchtime=args.benchtime, rounds=args.rounds),
                  source_sha256=hashes,
                  shared_dependency_snapshot=json.loads((DATA/'dependencies.json').read_text()),
                  binary_sha256={v: sha(p) for v,p in bins.items()},
                  exclusions=['compilation', 'process startup', 'scene and vector-text construction',
                              'one warm-up Render per workload', 'independent reference processes'],
                  host_caveat='shared Apple M2; other agents/processes active; no host isolation; all samples kept',
                  runs=[])
    samples = dict(before={}, after={})
    for i in range(args.rounds):
        for variant in (('before','after') if i%2 == 0 else ('after','before')):
            log = DATA/f'interleaved-{i+1}-{variant}.txt'
            run([bins[variant], '-test.run=^$', f'-test.bench={PATTERN}',
                 '-test.benchmem', f'-test.benchtime={args.benchtime}', '-test.count=1', '-test.cpu=1'],
                log, env, cwd=ROOT/'render')
            parsed = parse(log.read_text())
            for name, v in parsed.items():
                samples[variant].setdefault(name, []).append(v)
            report['runs'].append(dict(round=i+1, variant=variant, file=log.name,
                                       sha256=sha(log), measurements=parsed))
            print(f'{i+1}/{args.rounds} {variant}: PASS', flush=True)
    report['summary'] = []
    for name, bv in samples['before'].items():
        av = samples['after'][name]
        bm, am = [statistics.median(x['ns'] for x in v) for v in [bv, av]]
        report['summary'].append(dict(name=name, before_median_ns=bm, after_median_ns=am,
            median_delta_percent=(am/bm-1)*100,
            paired_delta_percent=[(a['ns']/b['ns']-1)*100 for a,b in zip(av,bv)],
            before_range_ns=[min(x['ns'] for x in bv),max(x['ns'] for x in bv)],
            after_range_ns=[min(x['ns'] for x in av),max(x['ns'] for x in av)],
            before_B_per_op=[x['B'] for x in bv],after_B_per_op=[x['B'] for x in av],
            before_allocs_per_op=[x['allocs'] for x in bv],after_allocs_per_op=[x['allocs'] for x in av]))
    report['source_unchanged_during_measurement'] = all(sha(ROOT/p)==v for p,v in hashes.items())
    (DATA/'interleaved.json').write_text(json.dumps(report,indent=2)+'\n')
    for row in report['summary']:
        print(row['name'], round(row['median_delta_percent'],2),'%')
    if not report['source_unchanged_during_measurement']:
        raise RuntimeError('source changed during measurement; see report')


if __name__ == '__main__':
    main()
