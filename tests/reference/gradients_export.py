"""Development-only exported paintserver check; not a source-scene oracle.

First generate SVG/native PNG pairs with the focused central export/render
preview tests into tests/data/export/gradients (or pass another directory).
CairoSVG interprets the Go SVG independently; this complements gradients.py's
hand-authored SVG source references. Go tests need no external renderer.
"""
import hashlib
import json
from pathlib import Path
import sys
import cairosvg
import cairocffi
from PIL import Image

root=Path(__file__).resolve().parents[2]
out=Path(sys.argv[1]) if len(sys.argv)>1 else root/'tests/data/export/gradients'
results=[]
for name in ['linear-hard-hole','radial-affine-clip','linear-alpha']:
 svg=out/(name+'-export.svg')
 cairo=out/(name+'-cairo.png')
 cairosvg.svg2png(bytestring=svg.read_bytes(),write_to=str(cairo))
 # Compare premultiplied RGBA, matching Go image.RGBA and not exaggerating
 # unassociated RGB differences near alpha=0.
 a=Image.open(out/(name+'-native.png')).convert('RGBa')
 b=Image.open(cairo).convert('RGBa')
 assert a.size==b.size==(160,120)
 absolute=maximum=changed=0
 for x,y in zip(a.getdata(),b.getdata()):
  delta=[abs(i-j) for i,j in zip(x,y)]
  absolute+=sum(delta);maximum=max(maximum,max(delta));changed+=max(delta)>4
 mae=absolute/(160*120*4)
 result={'name':name,'RGBA_MAE':mae,'maximum_channel_delta':maximum,'fraction_over_4':changed/(160*120)}
 assert mae<=.55 and maximum<=4,result
 results.append(result)
metadata={'provenance':'CairoSVG independently interprets gomath SVG paintservers; not an independent source-scene oracle. See render/gradients fixtures for hand-authored sources.',
 'CairoSVG':cairosvg.__version__,'Cairo':cairocffi.cairo_version_string(),'results':results,
 'generator_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 'files':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(out.iterdir()) if p.suffix in ('.svg','.png')}}
(out/'provenance.json').write_text(json.dumps(metadata,indent=2)+'\n')
print(json.dumps(results,indent=2))
