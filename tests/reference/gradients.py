"""Development-only independent, hand-authored SVG gradient fixtures.

Never imports/runs gomath. CairoSVG/Cairo rasterize these source SVGs. Go tests
consume the frozen SVG/PNG/JSON files without Python or native dependencies.
"""
import hashlib
import json
from pathlib import Path
import cairosvg
import cairocffi

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / 'tests/data/render/gradients'
OUT.mkdir(parents=True, exist_ok=True)
SOURCES = {
 'linear-hard-hole': '''<svg xmlns="http://www.w3.org/2000/svg" width="160" height="120"><defs>
 <linearGradient id="g" gradientUnits="userSpaceOnUse" x1="20" y1="60" x2="140" y2="60" color-interpolation="sRGB" spreadMethod="pad">
 <stop offset="0" stop-color="#ff0000"/><stop offset=".4" stop-color="#00ff00"/><stop offset=".4" stop-color="#0000ff"/><stop offset="1" stop-color="#ffffff"/>
 </linearGradient></defs><path fill="url(#g)" d="M8 8H152V112H8Z M60 40V80H100V40Z"/></svg>''',
 'radial-affine-clip': '''<svg xmlns="http://www.w3.org/2000/svg" width="160" height="120"><defs>
 <radialGradient id="g" gradientUnits="userSpaceOnUse" cx="0" cy="0" r="1" fx="0" fy="0" gradientTransform="matrix(60 15 10 35 80 60)" color-interpolation="sRGB" spreadMethod="pad">
 <stop offset="0" stop-color="#ffffff"/><stop offset=".5" stop-color="#ff0000"/><stop offset="1" stop-color="#0000ff"/>
 </radialGradient><clipPath id="c"><path d="M20 10H140V110H20Z"/></clipPath></defs>
 <path fill="url(#g)" opacity=".6" clip-path="url(#c)" d="M5 5H155V115H5Z"/></svg>''',
 'linear-alpha': '''<svg xmlns="http://www.w3.org/2000/svg" width="160" height="120"><defs>
 <linearGradient id="g" gradientUnits="userSpaceOnUse" x1="20" y1="60" x2="140" y2="60" color-interpolation="sRGB" spreadMethod="pad">
 <stop offset="0" stop-color="#ff0000" stop-opacity="0"/><stop offset="1" stop-color="#0000ff" stop-opacity="1"/>
 </linearGradient></defs><path fill="url(#g)" d="M8 8H152V112H8Z"/></svg>'''
}
for name, svg in SOURCES.items():
 (OUT/(name+'.svg')).write_text(svg+'\n')
 cairosvg.svg2png(bytestring=svg.encode(),write_to=str(OUT/(name+'.png')))
metadata = {'provenance':'Independent hand-authored SVG inputs; CairoSVG/Cairo raster, no gomath code loaded.',
 'CairoSVG':cairosvg.__version__, 'Cairo':cairocffi.cairo_version_string(),
 'generator_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
 'files':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(OUT.iterdir()) if p.suffix in ('.svg','.png')}}
(OUT/'provenance.json').write_text(json.dumps(metadata,indent=2)+'\n')
print(json.dumps(metadata,indent=2))
