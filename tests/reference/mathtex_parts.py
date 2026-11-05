#!/usr/bin/env python3
"""Development-only independent source-font/manual fragment/Cairo oracle.

No gomath output is read. This checks the documented finite fragment policy,
not TeX concatenation or Manim MathTex isolation parity. Go fonts retain their
notices in LICENSES/go-fonts.txt. Python/native libraries are never runtime or
Go-test dependencies. --overlay-only requires only Python's standard library.
"""
import argparse
import hashlib
import json
from pathlib import Path
import platform

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/data/mathtex/parts"
PIN = "16ea443e2a5f1d79db0a721161411ce916d6e733"
FILES = ["Go-Regular.ttf", "Go-Bold.ttf", "Go-Italic.ttf", "Go-Bold-Italic.ttf"]


def sha(data):
    return hashlib.sha256(data).hexdigest()


def overlay():
    OUT.mkdir(parents=True, exist_ok=True)
    replacements = {}
    for source in (ROOT / "tests/_cases/mathtex").glob("*_test.go"):
        replacements[str(ROOT / "mathtex" / source.name)] = str(source)
    for source in (ROOT / "tests/_cases/mathtex/_wave9").glob("*_test.go"):
        replacements[str(ROOT / "mathtex" / source.name)] = str(source)
    for source in (ROOT / "mathtex/_wave9").glob("*.go"):
        replacements[str(ROOT / "mathtex" / source.name)] = str(source)
    target = OUT / "overlay.json"
    target.write_text(json.dumps({"Replace": replacements}, indent=2) + "\n")
    print(target)


def moved(bounds, x, y):
    return [bounds[0]+x, bounds[1]+y, bounds[2]+x, bounds[3]+y] if bounds else None


def union(boxes):
    boxes = [b for b in boxes if b]
    return [min(b[0] for b in boxes), min(b[1] for b in boxes),
            max(b[2] for b in boxes), max(b[3] for b in boxes)] if boxes else None


def generate(fontdir):
    import cairocffi
    import cairosvg
    import fontTools
    from fontTools.ttLib import TTFont
    from fontTools.pens.boundsPen import BoundsPen
    from fontTools.pens.svgPathPen import SVGPathPen

    OUT.mkdir(parents=True, exist_ok=True)
    fonts, glyphs = [], []
    for filename in FILES:
        file = fontdir / "ttfs" / filename
        font = TTFont(file)
        glyphset, upem = font.getGlyphSet(), font["head"].unitsPerEm
        fonts.append(dict(file=filename, sha256=sha(file.read_bytes()), units_per_em=upem))
        face = {}
        for letter in "ngWi+=":
            name = font.getBestCmap()[ord(letter)]
            pen, bounds = SVGPathPen(glyphset), BoundsPen(glyphset)
            glyphset[name].draw(pen)
            glyphset[name].draw(bounds)
            origin = (font["hmtx"][name][1]-font["glyf"][name].xMin)/upem
            bbox = [bounds.bounds[0]/upem+origin, bounds.bounds[1]/upem,
                    bounds.bounds[2]/upem+origin, bounds.bounds[3]/upem]
            left, advance = max(0, -bbox[0]), font["hmtx"][name][0]/upem
            face[letter] = dict(path=pen.getCommands(), origin=origin, left=left,
                                bbox=moved(bbox, left, 0), width=max(advance, bbox[2])+left,
                                height=max(0, bbox[3]), depth=max(0, -bbox[1]), upem=upem)
        glyphs.append(face)

    def glyph(letter, style=0, color="ffffff", source=None):
        g = glyphs[style][letter]
        return dict(source=letter if source is None else source, width=g["width"], height=g["height"],
                    depth=g["depth"], bbox=g["bbox"],
                    svg=f'<path fill="#{color}" transform="translate({g["origin"]+g["left"]:.17g} 0) scale({1/g["upem"]:.17g})" d="{g["path"]}"/>')

    def empty(source="", width=0, height=0, depth=0):
        return dict(source=source, width=width, height=height, depth=depth, bbox=None, svg="")

    def phantom(letter, kind="phantom"):
        g = glyph(letter)
        return empty("\\"+kind+"{"+letter+"}", width=0 if kind=="vphantom" else g["width"],
                     height=0 if kind=="hphantom" else g["height"], depth=0 if kind=="hphantom" else g["depth"])

    def fraction(n, d):
        num, den = glyph(n), glyph(d)
        width = max(num["width"], den["width"])*.8+.25
        nx, dx = (width-num["width"]*.8)/2, (width-den["width"]*.8)/2
        ny, dy = .32+num["depth"]*.8, -den["height"]*.8
        nb = moved([v*.8 for v in num["bbox"]], nx, ny)
        db = moved([v*.8 for v in den["bbox"]], dx, dy)
        svg = (f'<g transform="translate({nx:.17g} {ny:.17g}) scale(.8)">{num["svg"]}</g>'
               f'<g transform="translate({dx:.17g} {dy:.17g}) scale(.8)">{den["svg"]}</g>'
               f'<path d="M0 .16H{width:.17g}" fill="none" stroke="#ffffff" stroke-width=".045" stroke-linecap="round"/>')
        return dict(source="\\frac{"+n+"}{"+d+"}", width=width,
                    height=max(ny+num["height"]*.8,.1825), depth=max(-dy+den["depth"]*.8,0),
                    bbox=union([nb, db, [0,.16,width,.16]]), svg=svg)

    hidden = fraction("n", "g")
    hidden = empty(r"\phantom{\frac{n}{g}}", hidden["width"], hidden["height"], hidden["depth"])
    cases = [
        ("baseline", 0, [glyph("n"), glyph("g"), glyph("W")]),
        ("repeated", 0, [glyph("n"),glyph("+"),glyph("n"),glyph("="),glyph("g")]),
        ("zero-ink", 0, [empty(),empty(r"\quad",1),phantom("W","hphantom"),glyph("g"),phantom("n","vphantom")]),
        ("color-scope", 0, [glyph("n",color="ff0000",source=r"\color{red}n"),glyph("g"),glyph("W",color="0000ff",source=r"\textcolor{blue}{W}")]),
        ("fractions", 0, [fraction("n","g"),glyph("="),fraction("W","i")]),
        ("face-scope", 0, [glyph("n",1,source=r"\mathbf{n}"),glyph("g",2,source=r"\mathit{g}"),glyph("W",0,source=r"\mathrm{W}")]),
        ("italic", 2, [glyph(c,2) for c in "ngW"]),
        ("bold-italic", 3, [glyph(c,3) for c in "ngW"]),
        ("hidden-fraction", 0, [hidden,glyph("g"),glyph("n")]),
    ]
    records, images = [], {}

    def write(name, content):
        svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="768" height="768">'
               '<rect width="768" height="768" fill="#000000"/>'
               '<g transform="translate(64 384) scale(128 -128)">'+content+'</g></svg>\n').encode()
        png = cairosvg.svg2png(bytestring=svg)
        (OUT/(name+".svg")).write_bytes(svg)
        (OUT/(name+".png")).write_bytes(png)
        images[name+".svg"], images[name+".png"] = sha(svg), sha(png)

    for name, style, fragments in cases:
        x, placements, content = 0, [], []
        for i, f in enumerate(fragments):
            placements.append(dict(index=i, source=f["source"], x=x, baseline=0, advance=f["width"],
                                   height=f["height"], depth=f["depth"], bbox=moved(f["bbox"],x,0)))
            content.append(f'<g transform="translate({x:.17g} 0)">{f["svg"]}</g>')
            x += f["width"]
        write(name, "".join(content))
        records.append(dict(name=name,style=style,sources=[f["source"] for f in fragments],
                            advance=x,height=max(f["height"] for f in fragments),depth=max(f["depth"] for f in fragments),parts=placements))
        # Independently interpolate a selected fragment's geometry in world
        # coordinates. Glyph-only case avoids SVG's scaling of stroke width.
        if name=="baseline":
            for time,label in [(.35,"035"),(.7,"070")]:
                sx,sy,tx,ty=1-.2*time,1+.1*time,.2*time,.3*time
                write("transform-whole-"+label, f'<g transform="translate({tx:.17g} {ty:.17g}) scale({sx:.17g} {sy:.17g})">'+"".join(content)+"</g>")
                selected=content.copy()
                selected[1]=f'<g transform="translate({tx:.17g} {ty:.17g}) scale({sx:.17g} {sy:.17g})">'+content[1]+"</g>"
                write("transform-part-"+label,"".join(selected))

    metadata = dict(provenance=__doc__, generator_sha256=sha(Path(__file__).read_bytes()),
                    manim=dict(version="0.22.1",commit=PIN,role="source identity/behavior review only; not a MathTex layout oracle"),
                    versions=dict(Python=platform.python_version(),fontTools=fontTools.__version__,CairoSVG=cairosvg.__version__,Cairo=cairocffi.cairo_version_string()),
                    fonts=fonts,coordinates=dict(width=768,height=768,origin=[64,384],ppem=128),
                    policy=dict(baseline=0,boundary_gap=0,fraction_ratio=.8,fraction_padding=.25,
                                fraction_axis=.16,rule_width=.045,rule_cap="round"),
                    records=records,files_sha256=images,
                    limitations="Manual finite fragment boxes and original quadratic glyph outlines, not TeX macro parsing, full source mapping or Manim MathTex parity. sfnt font extrema may differ by one design unit. Interior references use linear affine transforms of glyph geometry.")
    (OUT/"reference.json").write_text(json.dumps(metadata,indent=2)+"\n")
    print(f"wrote {len(records)} independent expressions and 4 interior Transform references")


def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--gofont-dir",type=Path)
    ap.add_argument("--overlay-only",action="store_true")
    args=ap.parse_args()
    if args.overlay_only:
        overlay()
    else:
        if args.gofont_dir is None:
            ap.error("--gofont-dir is required for independent fixture generation")
        generate(args.gofont_dir)


if __name__=="__main__":
    main()
