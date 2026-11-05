"""Compare native demo PNGs with independent rendering of exported vector SVGs.

Development only. Build bin/gomath with CGO_ENABLED=0 -tags noasm first. Use a
new output directory, --scene and one or more --time arguments. This validates
export/raster agreement, not independent construction of the intended scene.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import xml.etree.ElementTree as ET

import cairosvg
import cairocffi
from PIL import Image

parser=argparse.ArgumentParser(description=__doc__)
parser.add_argument("output",type=Path)
parser.add_argument("--scene",required=True)
parser.add_argument("--time",type=float,action="append",required=True)
parser.add_argument("--supersampling",type=int,choices=[1,2,4,8],default=1)
options=parser.parse_args()
root=Path(__file__).resolve().parents[2]
out=options.output.resolve();out.mkdir(parents=True,exist_ok=False)
binary=root/"bin/gomath"
def source_hashes():
    return {str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest()
            for p in sorted(root.rglob("*.go"))
            if p.relative_to(root).parts[0] not in {"tests","artifacts","bin",".git"}
            and not any(part.startswith("_") for part in p.relative_to(root).parts)}
sources_before=source_hashes()
binary_before=hashlib.sha256(binary.read_bytes()).hexdigest()
commands=[];checks=[]
for index,time in enumerate(options.time):
    for kind in ["png","svg"]:
        args=[str(binary),"demo","-scene",options.scene,"-time",str(time),"-width","640","-height","360",
              "-format",kind,"-out",str(out/f"frame-{index:02d}.{kind}")]
        if kind=="png":
            args.extend(["-supersampling",str(options.supersampling)])
        subprocess.check_call(args,cwd=root);commands.append(args[1:])
    svg=(out/f"frame-{index:02d}.svg").read_bytes()
    assert not any(e.attrib.get("clip-rule")=="evenodd" for e in ET.fromstring(svg).iter()), "CairoSVG is not an evenodd clip oracle; use direct Cairo references"
    cairosvg.svg2png(bytestring=svg,write_to=str(out/f"cairo-{index:02d}.png"))
    native=Image.open(out/f"frame-{index:02d}.png").convert("RGBA")
    cairo=Image.open(out/f"cairo-{index:02d}.png").convert("RGBA")
    assert native.size==cairo.size==(640,360)
    absolute=maximum=changed=0
    for a,b in zip(native.getdata(),cairo.getdata()):
        delta=[abs(x-y) for x,y in zip(a,b)]
        absolute+=sum(delta);maximum=max(maximum,max(delta));changed+=max(delta)>8
    mae=absolute/(640*360*4);fraction=changed/(640*360)
    check={"time":time,"RGBA_MAE":mae,"maximum_channel_delta":maximum,"fraction_pixels_any_channel_over_8":fraction}
    assert mae<.4 and maximum<=64 and fraction<.015,check
    checks.append(check)
assert source_hashes()==sources_before,"production sources changed during verification"
assert hashlib.sha256(binary.read_bytes()).hexdigest()==binary_before,"binary changed during verification"
result={"scene":options.scene,"supersampling":options.supersampling,"checks":checks,"commands":commands,
        "generator_sha256":hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "binary_sha256":binary_before,
        "CairoSVG":cairosvg.__version__,"Cairo":cairocffi.cairo_version_string(),
        "sources_sha256":sources_before,
        "files":{p.name:{"sha256":hashlib.sha256(p.read_bytes()).hexdigest(),"bytes":p.stat().st_size} for p in sorted(out.iterdir()) if p.is_file()},
        "limits":"Selected 640x360 demo instants, native Go raster vs independent CairoSVG interpretation of actual exported SVG. Shares Go scene construction. Bounded AA/color comparison, not general Manim/SVG parity or throughput measurement."}
with (out/"provenance.json").open("x") as stream:
    json.dump(result,stream,indent=2);stream.write("\n")
print(json.dumps(checks,indent=2))
