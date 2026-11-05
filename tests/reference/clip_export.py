"""Independently rasterize an exported clip demo; development tools only.

Build bin/gomath with CGO_ENABLED=0 -tags noasm first; use a fresh output directory.
This validates a rendered Go PNG against a CairoSVG interpretation of Go SVG.
It complements the independent hand-authored clipping.py input fixtures.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

import cairosvg
import cairocffi
from PIL import Image

root = Path(__file__).resolve().parents[2]
out = Path(sys.argv[1]).resolve()
out.mkdir(parents=True,exist_ok=False)
binary=root/"bin/gomath"
commands=[]
for kind in ["svg","png"]:
    args=[str(binary),"demo","-scene","clipping","-format",kind,"-time","2","-width","640","-height","360","-out",str(out/("clipping."+kind))]
    subprocess.check_call(args,cwd=root)
    commands.append(args[1:])
cairosvg.svg2png(bytestring=(out/"clipping.svg").read_bytes(),write_to=str(out/"cairo-clipping.png"))
got=Image.open(out/"clipping.png").convert("RGBA")
ref=Image.open(out/"cairo-clipping.png").convert("RGBA")
assert got.size==ref.size==(640,360)
absolute=maximum=changed=0
for a,b in zip(got.getdata(),ref.getdata()):
    delta=[abs(x-y) for x,y in zip(a,b)]
    absolute+=sum(delta); maximum=max(maximum,max(delta));changed+=max(delta)>8
mae=absolute/(640*360*4)
fraction=changed/(640*360)
result={"date":"2026-11-04","CairoSVG":cairosvg.__version__,"Cairo":cairocffi.cairo_version_string(),"commands":commands,
    "RGBA_MAE":mae,"maximum_channel_delta":maximum,"fraction_pixels_any_channel_over_8":fraction,
    "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),"binary_sha256":hashlib.sha256(binary.read_bytes()).hexdigest(),
    "sources_sha256":{str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(root.rglob("*.go")) if p.relative_to(root).parts[0] not in {"tests","artifacts","bin",".git"}},
    "files":{p.name:{"sha256":hashlib.sha256(p.read_bytes()).hexdigest(),"bytes":p.stat().st_size} for p in sorted(out.iterdir()) if p.is_file()},
    "limits":"One interior 640x360 demo frame, including bitmap/ring, two nested vector masks, animated clip/rotation, real vector title and alpha. Compares Go raster and exported SVG semantics via independent CairoSVG; not an independent scene-construction oracle, general clipping/Manim parity or performance measurement."}
assert mae<.4 and maximum<=64 and fraction<.015,result
with (out/"provenance.json").open("x") as f:
    json.dump(result,f,indent=2);f.write("\n")
print(json.dumps({k:result[k] for k in ["RGBA_MAE","maximum_channel_delta","fraction_pixels_any_channel_over_8"]},indent=2))
