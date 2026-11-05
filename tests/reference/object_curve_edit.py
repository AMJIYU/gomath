"""Independent source-pinned family handle edits/preparation and Transform states."""
import hashlib,json,subprocess,sys
from pathlib import Path
import numpy as np
import manim
from manim import VMobject,VGroup,NumberPlane,Transform,ORIGIN,linear
root=Path(__file__).resolve().parents[2];reference=root.parent/'manim';commit='16ea443e2a5f1d79db0a721161411ce916d6e733'
assert manim.__version__=='0.22.1'
assert subprocess.check_output(['git','-C',str(reference),'rev-parse','HEAD'],text=True).strip()==commit
hashes = {}
for name in ["utils/bezier.py", "utils/paths.py", "utils/space_ops.py",
             "mobject/types/vectorized_mobject.py", "mobject/mobject.py",
             "mobject/graphing/coordinate_systems.py", "animation/animation.py",
             "animation/transform.py"]:
    blob = subprocess.check_output(["git", "-C", str(reference), "show", f"{commit}:manim/{name}"])
    assert blob == (Path(manim.__file__).parent / name).read_bytes(), name
    assert blob == (reference / "manim" / name).read_bytes(), name
    hashes["manim/" + name] = hashlib.sha256(blob).hexdigest()

out=Path(sys.argv[1]);assert not out.exists()
def line(a,b):return [a,(2*a+b)/3,(a+2*b)/3,b]
def contour(anchors):
    a=np.array(anchors,dtype=float);return np.concatenate([line(x,y) for x,y in zip(a,a[1:])]).tolist()
parts=[[
 contour([[-2,0,0],[-1,.8,0],[0,-.2,0]]),
 contour([[1,0,.2],[2,.7,.2],[2.5,-.5,.2],[1,0,.2]])
],[contour([[-.4,-1,-.2],[.2,-.6,-.2],[1,-1.2,-.2]])]]
cases=[]
for form in ['single','multi','nested']:
 for mode in ['smooth','jagged','prepare11','prepare3']:
  children=[VMobject().set_points(np.concatenate([np.array(c) for c in contours])) for contours in parts]
  obj=children[1] if form=='single' else children[0] if form=='multi' else VGroup(children[0],VGroup(children[1]))
  if mode=='smooth':obj.make_smooth()
  elif mode=='jagged':obj.make_jagged()
  else:NumberPlane.prepare_for_nonlinear_transform(obj,int(mode.removeprefix('prepare')))
  states=[]
  for alpha in [0.,.125,.25,.5,.75,.875,1.]:
   current=obj.copy();target=obj.copy().scale(1.08,about_point=ORIGIN).rotate(.27,about_point=ORIGIN).shift(np.array([.4,-.3,.1]))
   animation=Transform(current,target,rate_func=linear);animation.begin();animation.interpolate(alpha)
   if alpha==1:animation.finish()
   states.append({'alpha':alpha,'points':current.get_all_points().tolist()})
  cases.append({'form':form,'mode':mode,'states':states})
out.parent.mkdir(parents=True,exist_ok=True)
with out.open('x') as f:json.dump({'source':{'version':manim.__version__,'commit':commit,'generator_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),'files':hashes},'parts':parts,'cases':cases},f,indent=2);f.write('\n')
print(len(cases),'constructions',sum(len(c['states']) for c in cases),'states')
