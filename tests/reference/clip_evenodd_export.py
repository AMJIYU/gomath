#!/usr/bin/env python3
"""Strict development-only Cairo interpreter for the actual exported clip fixture.

CGO_ENABLED=0 go run ./tests/reference/_clip_evenodd_export
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib PYTHONDONTWRITEBYTECODE=1 \
 /usr/local/bin/python3.11 tests/reference/clip_evenodd_export.py

CairoSVG 2.7.1 does not implement evenodd clip-rule. This deliberately small
independent reader handles only this export's svg/g/defs/clipPath/path/rect,
M/L/C/Z, affine transforms, solid paints and explicit clip rules. It errors on
other geometry/paint; it is never a runtime implementation or full SVG engine.
"""
import hashlib
import io
import json
from pathlib import Path
import platform
import re
import xml.etree.ElementTree as ET

import cairocffi as cairo
from PIL import Image, __version__ as pillow_version

ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT/'tests/data/export/clip-evenodd'
source=(OUT/'actual.svg').read_bytes()
root=ET.fromstring(source)
W,H=int(root.attrib['width']),int(root.attrib['height'])
surface=cairo.ImageSurface(cairo.FORMAT_ARGB32,W,H)
ctx=cairo.Context(surface)
number=r'[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?'

def tag(e):return e.tag.rsplit('}',1)[-1]
def numbers(s):return [float(x) for x in re.findall(number,s)]
def transform(value):
    rest=value
    for match in re.finditer(r'(translate|scale|matrix)\(([^)]*)\)',value):
        op,values=match.group(1),numbers(match.group(2))
        if op=='translate':ctx.translate(values[0],values[1] if len(values)>1 else 0)
        elif op=='scale':ctx.scale(values[0],values[1] if len(values)>1 else values[0])
        else:
            assert len(values)==6
            ctx.transform(cairo.Matrix(*values))
        rest=rest.replace(match.group(0),'',1)
    assert not rest.strip(),rest

def outline(value):
    tokens=re.findall('[A-Za-z]|'+number,value)
    i=0
    while i<len(tokens):
        op=tokens[i];i+=1
        count={'M':2,'L':2,'C':6,'Z':0}[op]
        values=[float(x) for x in tokens[i:i+count]];assert len(values)==count;i+=count
        {'M':ctx.move_to,'L':ctx.line_to,'C':ctx.curve_to,'Z':ctx.close_path}[op](*values)

def rule(value):
    assert value in ['nonzero','evenodd'],value
    ctx.set_fill_rule(cairo.FILL_RULE_EVEN_ODD if value=='evenodd' else cairo.FILL_RULE_WINDING)

def color(value,alpha):
    if value=='none':return False
    assert re.fullmatch('#[0-9a-fA-F]{6}',value),value
    ctx.set_source_rgba(*(int(value[i:i+2],16)/255 for i in [1,3,5]),alpha)
    return True

clips={e.attrib['id']:e for e in root.iter() if tag(e)=='clipPath'}
def apply_clip(ident):
    definition=clips[ident]
    assert definition.attrib.get('clipPathUnits')=='userSpaceOnUse'
    children=list(definition);assert len(children)==1 and tag(children[0])=='path'
    shape=children[0]
    matrix=ctx.get_matrix()
    transform(definition.attrib.get('transform',''))
    transform(shape.attrib.get('transform',''))
    ctx.new_path();outline(shape.attrib['d'])
    ctx.set_matrix(matrix)
    rule(shape.attrib.get('clip-rule',definition.attrib.get('clip-rule','nonzero')))
    ctx.clip()

def draw(e):
    kind=tag(e)
    if kind=='defs':return
    assert kind in ['svg','g','path','rect'],kind
    ctx.save()
    transform(e.attrib.get('transform',''))
    if 'clip-path' in e.attrib:
        match=re.fullmatch(r'url\(#([^)]*)\)',e.attrib['clip-path']);assert match
        apply_clip(match.group(1))
    if kind in ['svg','g']:
        for child in e:draw(child)
    else:
        # Empty group-placeholder paths have no fill or stroke geometry.
        if kind=='path' and not e.attrib.get('d','').strip():
            ctx.restore()
            return
        assert float(e.attrib.get('stroke-opacity','0'))==0 or float(e.attrib.get('stroke-width','0'))==0
        ctx.new_path()
        if kind=='path':outline(e.attrib['d'])
        else:
            def length(key,default):
                value=e.attrib.get(key,str(default))
                return default if value=='100%' else float(value)
            ctx.rectangle(float(e.attrib.get('x','0')),float(e.attrib.get('y','0')),length('width',W),length('height',H))
        rule(e.attrib.get('fill-rule','nonzero'))
        if color(e.attrib.get('fill','#000000'),float(e.attrib.get('fill-opacity','1'))):ctx.fill()
    ctx.restore()

draw(root)
surface.write_to_png(str(OUT/'cairo.png'))
actual=Image.open(OUT/'native.png').convert('RGBa')
expected=Image.open(OUT/'cairo.png').convert('RGBa')
deltas=[abs(a-b) for p,q in zip(actual.getdata(),expected.getdata()) for a,b in zip(p,q)]
metrics=dict(mae=sum(deltas)/len(deltas),rms=(sum(x*x for x in deltas)/len(deltas))**.5,max=max(deltas))
assert metrics['mae']<.4 and metrics['max']<=32,metrics
files=['actual.svg','native.png','cairo.png','input.json']
provenance=dict(generator='tests/reference/clip_evenodd_export.py',generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 go_generator='tests/reference/_clip_evenodd_export/main.go',go_generator_sha256=hashlib.sha256((ROOT/'tests/reference/_clip_evenodd_export/main.go').read_bytes()).hexdigest(),
 versions=dict(Python=platform.python_version(),Cairo=cairo.cairo_version_string(),Pillow=pillow_version),
 oracle='Actual Go SVG export interpreted independently with direct Cairo clip rules by a strict M/L/C/Z solid-path subset reader. No CairoSVG evenodd clip oracle or patched upstream implementation.',
 metrics=metrics,files_sha256={name:hashlib.sha256((OUT/name).read_bytes()).hexdigest() for name in files})
(OUT/'provenance.json').write_text(json.dumps(provenance,indent=2)+'\n')
print(metrics)
