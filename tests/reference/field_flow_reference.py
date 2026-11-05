"""Development-only exact window/cubic oracle and pinned Manim clock probe.
No third-party imports. Run from gomath root; output is central JSON.
The AST probes run the pinned methods with minimal animation/container stubs;
they compare clocks/bounds only, never claim a Manim pixel/render comparison.
Go runtime/tests do not execute Python. See accompanying MIT license.
"""
from fractions import Fraction as F
from pathlib import Path
import ast
import __future__
import hashlib
import json
import subprocess
from types import SimpleNamespace as NS

root = Path(__file__).resolve().parents[2]
reference = root.parent / "manim"
commit = "16ea443e2a5f1d79db0a721161411ce916d6e733"
assert subprocess.check_output(["git", "-C", str(reference), "rev-parse", "HEAD"], text=True).strip() == commit

# Independent Bernstein evaluation using rational arithmetic, rather than Go's
# de Casteljau subdivision. All input times and controls are binary-exact.
controls = [
    [[0,0],[F(1,4),1],[F(3,4),-1],[1,0]],
    [[1,0],[1,2],[2,2],[2,0]],
    [[2,0],[3,-1],[4,1],[4,0]],
]
def point(c, u):
    w = [(1-u)**3, 3*u*(1-u)**2, 3*u*u*(1-u), u**3]
    return [float(sum(wi*pi[axis] for wi,pi in zip(w,c))) for axis in range(2)]
cases = []
for reverse_times in (False, True):
    elapsed = [F(0), F(1,8), F(7,8), F(2)]
    times = [F(10) + (-e if reverse_times else e) for e in elapsed]
    for speed in (F(5,4), F(-5,4)):
        width, delay = F(5,8), F(1,8)
        for loop in (False, True):
            samples = []
            for display in map(F, ["0", ".125", ".175", ".625", "1.525", "2.225", "2.625", "3.125", "4.325"]):
                head = max(F(0),display-delay)*abs(speed)
                if loop:
                    head %= elapsed[-1]+width
                lo, hi = max(F(0),head-width), min(elapsed[-1],head)
                output = []
                if lo < hi:
                    if speed < 0:
                        lo,hi = elapsed[-1]-hi,elapsed[-1]-lo
                    for i,c in enumerate(controls):
                        a,b = max(lo,elapsed[i]), min(hi,elapsed[i+1])
                        if a < b:
                            u,v = (a-elapsed[i])/(elapsed[i+1]-elapsed[i]),(b-elapsed[i])/(elapsed[i+1]-elapsed[i])
                            output.append([point(c,u+(v-u)*s) for s in (F(0),F(1,4),F(1,2),F(3,4),F(1))])
                samples.append(dict(time=float(display), curves=output))
            cases.append(dict(times=list(map(float,times)), speed=float(speed), width=float(width),delay=float(delay),loop=loop,samples=samples))

# MIT licensed source extraction: keep source repo read-only and record hashes.
def method(relative, cls, name):
    source = (reference/relative).read_text()
    pinned = subprocess.check_output(["git", "-C", str(reference), "show", f"{commit}:{relative}"], text=True)
    assert source == pinned, f"reference working file differs from pinned blob: {relative}"
    tree = ast.parse(source)
    node = next(n for c in tree.body if isinstance(c,ast.ClassDef) and c.name==cls for n in c.body if isinstance(n,ast.FunctionDef) and n.name==name)
    return node, hashlib.sha256(source.encode()).hexdigest()
start, vector_hash = method("manim/mobject/vector_field.py", "StreamLines", "start_animation")
bounds, indication_hash = method("manim/animation/indication.py", "ShowPassingFlash", "_get_bounds")
ns = dict(interpolate=lambda a,b,t:(1-t)*a+t*b,Self=object, linear=lambda x:x, random=NS(random=lambda:0.0), np=NS(clip=lambda x,a,b:min(b,max(a,x))))
exec(compile(ast.Module(body=[bounds],type_ignores=[]),"pinned_indication", "exec",flags=__future__.annotations.compiler_flag),ns)
get_bounds = ns["_get_bounds"]
class Flash:
    def __init__(self,line,run_time,rate_func,time_width,**kwargs):
        self.mobject=line;self.run_time=run_time;self.time_width=time_width;self.alpha=0
    def begin(self): pass
    def interpolate(self,alpha): self.alpha=float(alpha);self.window=get_bounds(self,alpha)
class StreamLines:
    def __init__(self):self.stream_lines=[NS(duration=2.),NS(duration=1.)];self.virtual_time=2.
    def add(self,_):pass
    def add_updater(self,fn):self.updater=fn
ns.update(ShowPassingFlash=Flash,StreamLines=StreamLines)
exec(compile(ast.Module(body=[start],type_ignores=[]),"pinned_vector_field","exec",flags=__future__.annotations.compiler_flag),ns)
probes=[]
for speed in (1.,2.):
    for at in (.25,.75,1.,2.,2.25):
        container=StreamLines()
        ns["start_animation"](container,warm_up=False,flow_speed=speed,time_width=.25)
        container.updater(container,at)
        probes.append(dict(speed=speed,time=at,lines=[dict(duration=l.duration,clock=l.time,alpha=l.anim.alpha,bounds=l.anim.window) for l in container.stream_lines]))
payload=dict(oracle="Python fractions.Fraction Bernstein polynomial",controls=[[[float(x) for x in p] for p in c] for c in controls],cases=cases,manim=dict(commit=commit,vector_field_sha256=vector_hash,indication_sha256=indication_hash,scope="actual start_animation updater and ShowPassingFlash bounds; stub objects, zero random phase; no pixels",probes=probes))
(root/"tests/data/field/flow_reference.json").write_text(json.dumps(payload,indent=2)+"\n")
