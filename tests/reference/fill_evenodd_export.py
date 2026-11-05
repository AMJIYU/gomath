#!/usr/bin/env python3
"""Development-only validation of actual SVG ordinary evenodd fill exports.

CGO_ENABLED=0 go run ./tests/reference/_fill_evenodd_export
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
 /usr/local/bin/python3.11 tests/reference/fill_evenodd_export.py

CairoSVG implements ordinary fill-rule; unlike evenodd clip-rule this is a
suitable actual-export oracle. This fixture has ONLY nonzero clips; reject
evenodd clips and verify the ordinary evenodd circle-hole probe explicitly.
Independent directly authored Cairo path fixtures are in fill_evenodd.py.
"""
import hashlib
import io
import json
from pathlib import Path
import platform
import xml.etree.ElementTree as ET

import cairocffi as cairo
import cairosvg
from PIL import Image, __version__ as pillow_version

ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT/'tests/data/export/fill-evenodd'
source=(OUT/'actual.svg').read_bytes()
root=ET.fromstring(source)
assert sum(e.attrib.get('fill-rule')=='evenodd' for e in root.iter())==2
assert all(e.attrib.get('clip-rule')!='evenodd' for e in root.iter())
probe=b'<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><path fill-rule="evenodd" d="M0 0H32V32H0Z M8 8H24V24H8Z"/></svg>'
im=Image.open(io.BytesIO(cairosvg.svg2png(bytestring=probe)))
assert im.getpixel((16,16))[3]==0
for scale in [1,4]:
    raw=cairosvg.svg2png(bytestring=source,scale=scale)
    im=Image.open(io.BytesIO(raw)).convert('RGBA')
    if scale>1:im=im.convert('RGBa').resize((128,96),Image.Resampling.BOX).convert('RGBA')
    im.save(OUT/f'cairo-ss{scale}.png')
a=Image.open(OUT/'native.png').convert('RGBa')
b=Image.open(OUT/'cairo-ss1.png').convert('RGBa')
deltas=[abs(x-y) for p,q in zip(a.getdata(),b.getdata()) for x,y in zip(p,q)]
metrics=dict(mae=sum(deltas)/len(deltas),rms=(sum(x*x for x in deltas)/len(deltas))**.5,max=max(deltas))
assert metrics['mae']<.4 and metrics['rms']<2 and metrics['max']<=32,metrics
files=['actual.svg','native.png','cairo-ss1.png','cairo-ss4.png','input.json']
go='tests/reference/_fill_evenodd_export/main.go'
provenance=dict(generator='tests/reference/fill_evenodd_export.py',generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 go_generator=go,go_generator_sha256=hashlib.sha256((ROOT/go).read_bytes()).hexdigest(),
 versions=dict(Python=platform.python_version(),Cairo=cairo.cairo_version_string(),CairoSVG=cairosvg.__version__,Pillow=pillow_version),
 oracle='Unpatched CairoSVG/Cairo interprets actual SVG ordinary evenodd solid/gradient paths, including partial cubic geometry and a nonzero ancestor clip; explicit ordinary hole probe. No evenodd clip oracle claim.',
 cairo_svg_source_sha256=hashlib.sha256((Path(cairosvg.__file__).parent/'surface.py').read_bytes()).hexdigest(),
 metrics=metrics,files_sha256={name:hashlib.sha256((OUT/name).read_bytes()).hexdigest() for name in files})
(OUT/'provenance.json').write_text(json.dumps(provenance,indent=2)+'\n')
print(metrics)
