# Reference harness (development only)

This is the complete harness used for the recorded measurement. It is not part
of GoMath runtime. Copy the fenced Python code into `/tmp/gomath-reference.py`.
Use the exact local Manim checkout identified in `reference.json`. This invokes
Python/Manim/Cairo only as an independent reference; GoMath generation and tests
have no dependency on them.

Build and export geometry before timing, and do not run competing builds/tests.
Use fresh output paths; the CLI refuses overwriting files.

```sh
CGO_ENABLED=0 go build -tags noasm -o bin/gomath ./cmd/gomath
./bin/gomath fixture -out artifacts/reference-rerun.json
./bin/gomath demo -scene reference -format png -time .5 \
  -width 640 -height 360 -out artifacts/reference-go-rerun.png
UV_HTTP_TIMEOUT=120 uv run --quiet --isolated --no-project \
  --with ../manim python /tmp/gomath-reference.py \
  artifacts/reference-rerun.json artifacts/reference-manim-rerun.png \
  > artifacts/reference-manim-rerun.json
for trial in 1 2 3 4 5; do
  ./bin/gomath benchmark -scene reference -frames 300 \
    -out "artifacts/reference-go-rerun-${trial}.json"
done
./bin/gomath compare -reference artifacts/reference-manim-rerun.png \
  -actual artifacts/reference-go-rerun.png
```

Manim's color constructor treats integer tuples as 0–255 channels. The explicit
float conversions below are essential, especially for white `(1,1,1)`.
Stroke widths are converted using the pinned Cairo camera's 0.01 world-unit
multiplier; the fixture fixes matching aspect ratio and camera bounds.

```python
import json,sys,time,statistics,platform
from pathlib import Path
import numpy as np
import manim
from manim import Camera, VMobject, ManimColor, LineJointType, CapStyleType
fixture=Path(sys.argv[1]);out=Path(sys.argv[2]);d=json.loads(fixture.read_text());c=d['Config'];objects=[]
def point(p): return [p['X'],p['Y'],p['Z']]
for o in d['Frame']['Objects']:
 style=o['Style'];fill,stroke=style['Fill'],style['Stroke']
 v=VMobject(joint_type=LineJointType.MITER,cap_style=CapStyleType.ROUND)
 pts=[]
 for contour in o['Path']['Contours']:
  for curve in contour['Curves']:
   pts.extend(point(curve[k]) for k in ['P0','P1','P2','P3'])
 v.set_points(np.array(pts,dtype=float))
 v.set_fill(ManimColor(tuple(float(fill[k]) for k in ['R','G','B'])),opacity=fill['A']*style['Opacity'])
 v.set_stroke(ManimColor(tuple(float(stroke[k]) for k in ['R','G','B'])),width=style['StrokeWidth']/.01,opacity=stroke['A']*style['Opacity'])
 objects.append(v)
bg=c['Background']
camera=Camera(pixel_width=c['Width'],pixel_height=c['Height'],frame_width=c['FrameWidth'],frame_height=c['FrameWidth']*c['Height']/c['Width'],background_color=ManimColor(tuple(float(bg[k]) for k in ['R','G','B'])),background_opacity=bg['A'])
for i in range(10): camera.reset();camera.capture_mobjects(objects)
frames=300;times=[]
for trial in range(5):
 start=time.perf_counter()
 for i in range(frames): camera.reset();camera.capture_mobjects(objects)
 times.append(time.perf_counter()-start)
camera.get_image().save(out)
print(json.dumps({'engine':'Manim','version':manim.__version__,'renderer':'Cairo','scene':'reference: 100 exact imported cubic circles','width':c['Width'],'height':c['Height'],'frames_per_trial':frames,'trials':5,'elapsed_seconds':times,'median_frames_per_second':frames/statistics.median(times),'python':platform.python_version(),'platform':platform.platform(),'included':'camera.reset + capture_mobjects; construction, PNG encoding, startup excluded'},indent=2))
```
