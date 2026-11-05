#!/usr/bin/env python3
"""Development-only independent Skia polygon union + direct Cairo clip oracle.
Never imports gomath geometry or invokes a product fallback. CairoSVG is not
used as the mixed-rule/evenodd clip oracle. Run with Python pathops/cairocffi/Pillow.
"""
import base64
import hashlib
import io
import json
import math
from pathlib import Path
import platform
import cairocffi as cairo
import pathops
from PIL import Image, __version__ as pillow_version

ROOT=Path(__file__).resolve().parents[2]
OUT=ROOT/'tests/data/svg/clip-unions'
OUT.mkdir(parents=True,exist_ok=True)
W,H=128,96
I=(1,0,0,1,0,0)

def rect(x,y,w,h):return [(x,y),(x+w,y),(x+w,y+h),(x,y+h)]
def d(contours):return ' '.join('M'+' L'.join(f'{x:.17g},{y:.17g}' for x,y in c)+' Z' for c in contours)
def path(contours,rule):
 p=pathops.Path();p.fillType=pathops.FillType.EVEN_ODD if rule=='evenodd' else pathops.FillType.WINDING
 for c in contours:
  p.moveTo(*c[0])
  for pt in c[1:]:p.lineTo(*pt)
  p.close()
 return p

def union(children):
 result=path([], 'nonzero')
 for contours,rule in children:result=pathops.op(result,path(contours,rule),pathops.PathOp.UNION,fix_winding=True,keep_starting_points=True,clockwise=False)
 return result

def emit(ctx,p):
 ctx.new_path()
 for verb,points in p:
  if verb==pathops.PathVerb.MOVE:ctx.move_to(*points[0])
  elif verb==pathops.PathVerb.LINE:ctx.line_to(*points[0])
  elif verb==pathops.PathVerb.CLOSE:ctx.close_path()
  else:raise ValueError('oracle unexpectedly produced curved output')

def clip(ctx,p):emit(ctx,p);ctx.set_fill_rule(cairo.FILL_RULE_WINDING);ctx.clip()
def paint(ctx,box=(0,0,128,96),rgba=(32/255,160/255,224/255,1)):
 ctx.new_path();ctx.rectangle(*box);ctx.set_source_rgba(*rgba);ctx.fill()
def source(defs,body):return '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="128" height="96" viewBox="0 0 128 96"><defs>'+defs+'</defs>'+body+'</svg>\n'
def childxml(children):return ''.join(f'<path clip-rule="{rule}" d="{d(contours)}"/>' for contours,rule in children)
def matrix(m):return 'matrix('+' '.join(format(v,'.17g') for v in m)+')'

cases={}
def basic(name,children):
 p=union(children)
 s=source('<clipPath id="c">'+childxml(children)+'</clipPath>','<rect width="128" height="96" fill="#20a0e0" clip-path="url(#c)"/>')
 def draw(ctx):clip(ctx,p);paint(ctx)
 cases[name]=(s,draw)
 return p

basic('opposite-overlap',[([rect(10,10,55,45)],'nonzero'),([rect(40,30,65,50)[::-1]],'evenodd')])
basic('mixed-holes',[([rect(8,8,108,78),rect(30,28,60,40)],'evenodd'),([rect(60,28,56,40)],'nonzero')])
basic('self-crossing',[([[(10,10),(100,80),(10,80),(100,10)]],'evenodd'),([[(30,20),(110,35),(80,85)]],'nonzero')])
basic('contacts',[([rect(8,8,32,32)],'nonzero'),([rect(40,8,32,16)],'evenodd'),([rect(40,40,32,32)],'nonzero')])

# Fully independent per-child affine endpoint construction; no production parser.
base=rect(0,0,32,24)
aff=[(x+.5*y+8,.25*x+y+5) for x,y in base]
basic('child-affine',[( [aff], 'nonzero'),([rect(28,18,50,40)[::-1]],'nonzero')])
s,_=cases['child-affine'];s=s.replace(f'<path clip-rule="nonzero" d="{d([aff])}"/>','<rect width="32" height="24" transform="matrix(1 .25 .5 1 8 5)"/>');cases['child-affine']=(s,cases['child-affine'][1])

unit=[([rect(.0625,.125,.6875,.75),rect(.25,.3125,.25,.375)],'evenodd'),([rect(.375,.25,.5625,.5)],'nonzero')]
bbox_union=union(unit)
a=math.pi/7
transforms=[(math.cos(a),math.sin(a),-math.sin(a),math.cos(a),14,0),(1,.2,.6,1,66,0),(-1.1,.1,.2,1.3,46,49),(1.5,0,0,.7,68,58)]
box=(3,4,26,20)
s=source('<clipPath id="c" clipPathUnits="objectBoundingBox" transform="translate(.03125 .0625)">'+childxml(unit)+'</clipPath>',''.join(f'<rect x="3" y="4" width="26" height="20" transform="{matrix(t)}" fill="#20a0e0" clip-path="url(#c)"/>' for t in transforms))
def bbox(ctx):
 for m in transforms:
  ctx.save();ctx.transform(cairo.Matrix(*m));old=ctx.get_matrix();ctx.translate(3,4);ctx.scale(26,20);ctx.translate(.03125,.0625);clip(ctx,bbox_union);ctx.set_matrix(old);paint(ctx,box);ctx.restore()
cases['bbox-affine']=(s,bbox)

# Same source gets independent inherited rules in each use shadow tree.
s=source('<style>clipPath > use.odd{clip-rule:evenodd} #s{clip-rule:inherit} .outside path{clip-rule:nonzero}</style><path id="s" d="M0 0H40V60H0Z M10 15H30V45H10Z"/><clipPath id="c" transform="translate(4 8)"><use class="odd" xlink:href="#s" x="4"/><use href="#s" x="72"/></clipPath>','<g class="outside"><rect width="128" height="96" fill="#20a0e0" clip-path="url(#c)"/></g>')
pu=union([([rect(8,8,40,60),rect(18,23,20,30)],'evenodd'),([rect(76,8,40,60),rect(86,23,20,30)],'nonzero')])
def use(ctx):clip(ctx,pu);paint(ctx)
cases['css-use']=(s,use)

poly=[([rect(8,8,108,78),rect(30,28,60,40)],'evenodd'),([rect(60,28,56,40)],'nonzero')]
pg=union(poly)
s=source('<linearGradient id="g" gradientUnits="userSpaceOnUse" x1="8" y1="8" x2="116" y2="8"><stop stop-color="red" stop-opacity=".7"/><stop offset="1" stop-color="blue" stop-opacity=".7"/></linearGradient><clipPath id="c">'+childxml(poly)+'</clipPath>','<rect x="4" y="4" width="116" height="84" fill="url(#g)" clip-path="url(#c)" transform="matrix(1 .03 .05 1 0 0)"/>')
def gradient(ctx):
 ctx.transform(cairo.Matrix(1,.03,.05,1,0,0));clip(ctx,pg);g=cairo.LinearGradient(8,8,116,8);g.add_color_stop_rgba(0,1,0,0,.7);g.add_color_stop_rgba(1,0,0,1,.7);ctx.set_source(g);ctx.rectangle(4,4,116,84);ctx.fill()
cases['gradient']=(s,gradient)

asset=io.BytesIO();Image.new('RGBA',(16,8),(48,160,224,180)).save(asset,format='PNG');uri='data:image/png;base64,'+base64.b64encode(asset.getvalue()).decode()
bitmap_polys=[([rect(.1,.3,.5,.4)],'nonzero'),([rect(.4,.35,.5,.3)],'nonzero')];pb=union(bitmap_polys)
s=source('<clipPath id="c" clipPathUnits="objectBoundingBox">'+childxml(bitmap_polys)+'</clipPath>',f'<image href="{uri}" x="10" y="6" width="100" height="80" preserveAspectRatio="xMaxYMid meet" clip-path="url(#c)" opacity=".6" transform="matrix(-1 .02 .04 1 122 0)"/>')
def bitmap(ctx):
 ctx.transform(cairo.Matrix(-1,.02,.04,1,122,0));old=ctx.get_matrix();ctx.translate(10,6);ctx.scale(100,80);clip(ctx,pb);ctx.set_matrix(old);paint(ctx,(10,21,100,50),(48/255,160/255,224/255,180/255*.6))
cases['bitmap']=(s,bitmap)

s=source('<clipPath id="outer"><rect x="18" y="20" width="80" height="52"/></clipPath><clipPath id="c">'+childxml(poly)+'</clipPath>','<g clip-path="url(#outer)"><rect width="128" height="96" fill="#20a0e0" clip-path="url(#c)"/></g>')
def ancestor(ctx):ctx.rectangle(18,20,80,52);ctx.clip();clip(ctx,pg);paint(ctx)
cases['ancestor']=(s,ancestor)

s=source('<clipPath id="c" clipPathUnits="objectBoundingBox">'+childxml(unit)+'</clipPath>','<line x1="12" x2="112" y1="20" y2="20" stroke="#20a0e0" stroke-width="8" clip-path="url(#c)"/>')
def degenerate(ctx):ctx.set_source_rgb(32/255,160/255,224/255);ctx.set_line_width(8);ctx.move_to(12,20);ctx.line_to(112,20);ctx.stroke()
cases['degenerate']=(s,degenerate)

geometry=[]
for name,children in [('mixed-holes',poly),('contacts',[([rect(8,8,32,32)],'nonzero'),([rect(40,8,32,16)],'evenodd'),([rect(40,40,32,32)],'nonzero')]),('opposite-overlap',[([rect(10,10,55,45)],'nonzero'),([rect(40,30,65,50)[::-1]],'evenodd')])]:
 p=union(children)
 # View is deliberately100x100 so Go helper preserves coordinate area exactly.
 src='<svg viewBox="0 0 100 100"><defs><clipPath id="c">'+childxml(children)+'</clipPath></defs><rect id="body" width="100" height="100" clip-path="url(#c)"/></svg>'
 samples=[[x+.5,y+.5,int(p.contains((x+.5,y+.5)))] for y in range(3,95,7) for x in range(3,95,7)]
 geometry.append(dict(name=name,source=src,area=p.area,samples=samples))
(OUT/'geometry.json').write_text(json.dumps(geometry,indent=2)+'\n')
for name,(src,draw) in cases.items():
 (OUT/f'{name}.svg').write_text(src)
 for scale in [1,4]:
  surface=cairo.ImageSurface(cairo.FORMAT_ARGB32,W*scale,H*scale);ctx=cairo.Context(surface);ctx.scale(scale,scale);draw(ctx);data=io.BytesIO();surface.write_to_png(data)
  im=Image.open(io.BytesIO(data.getvalue())).convert('RGBA')
  if scale>1:im=im.convert('RGBa').resize((W,H),Image.Resampling.BOX).convert('RGBA')
  im.save(OUT/f'{name}-cairo-ss{scale}.png')
# Native linear-time affine morph of the entire imported tree, expressed
# independently in SVG pixel coordinates around viewport center (64,48).
morph_union=union([([rect(10,10,55,45)],'nonzero'),([rect(40,30,65,50)[::-1]],'evenodd')])
for time,label in [(.35,'035'),(.7,'070')]:
 scale=4;surface=cairo.ImageSurface(cairo.FORMAT_ARGB32,W*scale,H*scale);ctx=cairo.Context(surface);ctx.scale(scale,scale)
 ctx.translate(time*(8+64*.2),time*(5+48*.1));ctx.scale(1-.2*time,1-.1*time);clip(ctx,morph_union);paint(ctx)
 data=io.BytesIO();surface.write_to_png(data);im=Image.open(io.BytesIO(data.getvalue())).convert('RGBA').convert('RGBa').resize((W,H),Image.Resampling.BOX).convert('RGBA');im.save(OUT/f'morph-{label}-cairo-ss4.png')
assert Image.open(OUT/'css-use-cairo-ss1.png').getpixel((28,38))[3]==0
assert Image.open(OUT/'css-use-cairo-ss1.png').getpixel((96,38))[3]==255
provenance=dict(generator='tests/reference/svg_clip_unions.py',generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 versions=dict(Python=platform.python_version(),Skia_pathops=pathops.__version__,Cairo=cairo.cairo_version_string(),Pillow=pillow_version),
 primary_spec=['https://www.w3.org/TR/css-masking-1/#ClippingPaths','https://www.w3.org/TR/css-masking-1/#elementdef-clippath','https://www.w3.org/TR/SVG2/coords.html#ObjectBoundingBoxUnits'],
 oracle='Independent Skia pathops normalizes each polygon child under its own rule and performs geometric UNION. Direct Cairo clips to the resulting single winding outline. No child alpha max, concatenation-as-union, gomath output or CairoSVG evenodd clip interpretation.',
 scope='Overlap/opposite directions/mixed holes/selfcrossing/contact/child affine transforms, CSS/use rules, local bbox with rotation/shear/reflection/nonuniform target transforms, gradient, bitmap meet viewport, old single clip ancestry and degenerate bbox.1x and4x premultiplied BOX. Two independently transformed intermediate native-animation frames at .35/.7. Separate numeric area/membership oracle.',
 limitations='Skia uses float32; moderate fixture coordinates avoid numeric precision boundaries. Not an exact dyadic/Bezier oracle. Constant bitmap color avoids unrelated image filter-edge differences. Camera/export tested separately; affine animation reference assumes linear rate and this fixed scene. No browser/group-isolation/general SVG parity claim.',
 files_sha256={f.name:hashlib.sha256(f.read_bytes()).hexdigest() for f in sorted(OUT.iterdir()) if f.suffix in ['.svg','.png'] or f.name=='geometry.json'})
(OUT/'provenance.json').write_text(json.dumps(provenance,indent=2)+'\n')
print(f'wrote {len(cases)} independent union scenes at1x/4x and{len(geometry)} geometry cases')
