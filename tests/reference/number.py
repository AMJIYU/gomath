#!/usr/bin/env python3
"""Independent development-only number formats and source-font/Cairo layouts.

Pinned Manim methods are read by AST, never imported as a rendering runtime.
Python binary64 format and pinned Manim outputs are recorded separately, including
known negative-rounded-zero differences. Font layouts use source hmtx/glyf/kern
and manually declared single-line baseline/advance anchors, not TeX or Go output.
"""
import argparse, ast, hashlib, json, math, platform, random, struct
from pathlib import Path
import numpy as np
import fontTools, cairocffi, cairosvg
from fontTools import subset
from fontTools.ttLib import TTFont, newTable
from fontTools.ttLib.tables._k_e_r_n import KernTable_format_0
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen

PIN='16ea443e2a5f1d79db0a721161411ce916d6e733'
SOURCE_SHA='d2d0b1bf1505a14129fec5b72b5612818a9c1d3c6ce3b21fe2f9f1a16452deaa'
FONTS=['Go-Regular.ttf','Go-Bold.ttf','Go-Italic.ttf','Go-Bold-Italic.ttf']
def sha(b): return hashlib.sha256(b).hexdigest()
def bits(v): return struct.pack('>d',v).hex()
def value(b): return struct.unpack('>d',bytes.fromhex(b))[0]
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument('--manim',required=True,type=Path)
    ap.add_argument('--fonts',required=True,type=Path)
    args=ap.parse_args()
    root=Path(__file__).resolve().parents[2];out=root/'tests/data/number';out.mkdir(parents=True,exist_ok=True)
    code=(args.manim/'manim/mobject/text/numbers.py').read_bytes()
    if sha(code)!=SOURCE_SHA: raise ValueError('Pinned numbers.py source differs')
    node=next(n for n in ast.parse(code).body if isinstance(n,ast.ClassDef) and n.name=='DecimalNumber')
    methods=[n for n in node.body if isinstance(n,ast.FunctionDef) and n.name in ['_get_num_string','_get_formatter','_get_complex_formatter']]
    tree=ast.fix_missing_locations(ast.Module(body=[ast.ClassDef(name='Pinned',bases=[],keywords=[],body=methods,decorator_list=[])],type_ignores=[]))
    ns={'np':np,'Any':object};exec(compile(tree,'pinned-numbers-format','exec'),ns)
    edge=[0.,-0.,.005,-.005,2.675,-2.675,1.125,1.375,9.995,999.995,math.nextafter(1000.,0),1000.,math.nextafter(1000.,math.inf),
          2.**53-1,2.**53,math.nextafter(2.**53,math.inf),1e-300,-1e-300,math.ulp(0.),-math.ulp(0.),float.fromhex('0x1.fffffffffffffp+1023'),-float.fromhex('0x1.fffffffffffffp+1023')]
    rng=random.Random(20261105)
    raw=[bits(v) for v in edge]+[f'{rng.getrandbits(64):016x}' for _ in range(64)]
    formats=[]
    for b in raw:
        v=value(b)
        if not math.isfinite(v): continue
        for p in [0,2,6,18]:
            for sign,group in [(False,False),(True,False),(False,True),(True,True)]:
                spec=('+' if sign else '')+(',' if group else '')+'.'+str(p)+'f'
                direct=format(v,spec)
                expected=direct
                if expected.startswith('-') and all(c in '0.,' for c in expected[1:]):
                    expected=('+' if sign else '')+expected[1:]
                pinned=ns['Pinned']();pinned.num_decimal_places=p;pinned.include_sign=sign;pinned.group_with_commas=group
                with np.errstate(over='ignore',under='ignore',invalid='ignore'):
                    manim=pinned._get_num_string(v)
                formats.append(dict(bits=b,precision=p,include_sign=sign,group_thousands=group,expected=expected,
                                    python_format=direct,manim_format=manim,agrees_with_pinned=expected==manim))
    integers=[]
    for v in [-(2**63),-(2**53)-1,-1000,-1,0,1,999,1000,2**53-1,2**53,2**53+1,2**63-1]:
        for sign,group in [(False,False),(True,False),(False,True),(True,True)]:
            integers.append(dict(value=str(v),include_sign=sign,group_thousands=group,expected=format(v,('+' if sign else '')+(',' if group else '')+'d')))
    differences=sum(not r['agrees_with_pinned'] for r in formats)
    frames=[]
    for alpha in [0,.25,.5,.75,1]:
        v=999.994*(1-alpha)+1000.014*alpha
        frames.append(dict(alpha=alpha,time=2*alpha,bits=bits(v),text=format(v,',.2f')+' kg'))
    (out/'formats.json').write_text(json.dumps(dict(provenance=__doc__,manim_commit=PIN,manim_numbers_sha256=SOURCE_SHA,
        generator_sha256=sha(Path(__file__).read_bytes()),Python=platform.python_version(),numpy=np.__version__,
        policy='Fixed Python format of represented binary64; normalize sign only if the formatted magnitude is zero. Pinned numpy.round zero-sign check can disagree near ties.',
        differing_pinned_records=differences,records=formats,integers=integers,linear_frames=frames),indent=2)+'\n')
    # Small authentic face: 9 and '-' deliberately absent for late atomic errors.
    f=TTFont(args.fonts/FONTS[0],recalcTimestamp=False)
    sub=subset.Subsetter();sub.populate(text='012+,. AV');sub.subset(f)
    for n in f['name'].names:
        if n.nameID in (1,2,3,4,6): n.string=('Regular' if n.nameID==2 else 'GoMathNumberFixture').encode(n.getEncoding())
    cmap=f.getBestCmap();k=newTable('kern');k.version=0;t=KernTable_format_0();t.version,t.coverage=0,1;t.kernTable={(cmap[65],cmap[86]):-160};k.kernTables=[t];f['kern']=k
    f.save(out/'restricted.ttf')
    (out/'LICENSE.txt').write_bytes((root/'LICENSES/go-fonts.txt').read_bytes())
    fixtures=[dict(file='restricted.ttf',sha256=sha((out/'restricted.ttf').read_bytes()),source='Go-Regular.ttf',characters='012+,. AV',modifications='cmap/name subset and explicit AV=-160 design unit fixture-only kern. Original glyph outlines/hmtx preserved.')]
    records=[];fonts=[]
    # Manually chosen displayed strings, independent of generated Go output.
    specs=[('right-decimal','decimal','-1234.5',2,False,True,'kg',0,.8,'-1,234.50 kg',0),
           ('center-signed','decimal','12.375',3,True,False,'m/s',2,.8,'+12.375 m/s',0),
           ('left-unit','decimal','0',2,False,False,'AV',1,1,'0.00 AV',0),
           ('right-int64','integer','9223372036854775807',0,False,True,'',0,.5,'9,223,372,036,854,775,807',0),
           ('affine','decimal','19.25',2,False,True,'kg',0,.8,'19.25 kg',1)]
    for style,filename in enumerate(FONTS):
        source=args.fonts/filename;fonts.append(dict(file=filename,sha256=sha(source.read_bytes())))
        for name,kind,v,p,sign,group,unit,anchor,size,formatted,affine in specs:
            f=TTFont(source);glyphs=f.getGlyphSet();upem=f['head'].unitsPerEm;cmap=f.getBestCmap()
            x=0;paths=[];items=[];previous=None
            for r in formatted:
                gname=cmap[ord(r)]
                if previous is not None and 'kern' in f:
                    x+=sum(k.kernTable.get((previous,gname),0) for k in f['kern'].kernTables)/upem
                b,pen=BoundsPen(glyphs),SVGPathPen(glyphs);glyphs[gname].draw(b);glyphs[gname].draw(pen)
                origin=0 if b.bounds is None else (f['hmtx'][gname][1]-f['glyf'][gname].xMin)/upem
                items.append(dict(rune=ord(r),x=x,origin=origin,bbox=None if b.bounds is None else [a/upem for a in b.bounds],path=pen.getCommands()))
                x+=f['hmtx'][gname][0]/upem;previous=gname
            anchor_shift=-x if anchor==0 else -x/2 if anchor==2 else 0
            tx=4 if anchor==0 else -4 if anchor==1 else 0
            matrix=[.9,.3,-.25,1.1,tx,.1] if affine else [1,0,0,1,tx,0]
            for item in items:
                dx=(item['x']+item['origin']+anchor_shift)*size
                if item['bbox'] is not None:
                    paths.append(f'<path d="{item["path"]}" transform="translate({dx:.17g} 0) scale({size/upem:.17g})"/>')
                item['x']=(item['x']+anchor_shift)*size;item['origin']*=size
                if item['bbox'] is not None:item['bbox']=[a*size for a in item['bbox']]
                del item['path']
            a,b,c,d,e,ff=matrix
            svg=('<svg xmlns="http://www.w3.org/2000/svg" width="960" height="384" viewBox="0 0 960 384"><rect width="960" height="384" fill="#000000"/>'
                 f'<g transform="translate(480 192) scale(96 -96)"><g fill="#32a0ff" transform="matrix({a} {b} {c} {d} {e} {ff})">'+''.join(paths)+'</g></g></svg>\n').encode()
            prefix=f'{name}-{style}';png=cairosvg.svg2png(bytestring=svg)
            (out/(prefix+'.svg')).write_bytes(svg);(out/(prefix+'.png')).write_bytes(png)
            records.append(dict(name=prefix,kind=kind,value=v,bits=bits(float(v)) if kind=='decimal' else None,
                precision=p,include_sign=sign,group_thousands=group,unit=unit,anchor=anchor,size=size,style=style,
                text=formatted,advance=x*size,matrix=matrix,glyphs=items,svg_sha256=sha(svg),png_sha256=sha(png)))
    rf=TTFont(out/'restricted.ttf');rg=rf.getGlyphSet();ru=rf['head'].unitsPerEm
    fixture_metrics={}
    for ch in '0 AV':
        name=rf.getBestCmap()[ord(ch)];b=BoundsPen(rg);rg[name].draw(b)
        fixture_metrics[ch]=dict(advance=rf['hmtx'][name][0]/ru,bbox=None if b.bounds is None else [v/ru for v in b.bounds],
            origin=0 if b.bounds is None else (rf['hmtx'][name][1]-rf['glyf'][name].xMin)/ru)
    manifest=dict(provenance=__doc__,fonts=fonts,fixtures=fixtures,fixture_metrics=fixture_metrics,fixture_AV_kern=-160/ru,generator_sha256=sha(Path(__file__).read_bytes()),
        versions=dict(Python=platform.python_version(),fontTools=fontTools.__version__,CairoSVG=cairosvg.__version__,Cairo=cairocffi.cairo_version_string()),
        raster=dict(width=960,height=384,pixels_per_unit=96,origin=[480,192]),records=records)
    (out/'layout.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print(f'Wrote {len(formats)} binary64 / {len(integers)} int64 formats ({differences} explicit pinned deviations), 20 source-font/Cairo layouts and restricted real-font fixture.')
if __name__=='__main__': main()
