#!/usr/bin/env python3
"""Development-only SVG container bbox oracles; no Go output/code is consumed.

Direct Cairo paths/clips provide the primary raster oracle. fontTools reads the
pinned Go font's hmtx/kern/hhea and original outlines, independently of gomath.
Run with --fonts pointing at x/image v0.47.0/font/gofont/ttfs. On Homebrew macOS
set DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib before starting Python.
"""
import argparse
import base64
import hashlib
import io
import json
import math
from pathlib import Path
import platform

import cairocffi as cairo
import fontTools
from fontTools.pens.basePen import BasePen
from fontTools.ttLib import TTFont
from PIL import Image, __version__ as pillow_version

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "tests/data/svg/container-bbox"
W, H = 128, 96
HALF = '<clipPath id="c" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/></clipPath>'
FULL = '<clipPath id="full" clipPathUnits="objectBoundingBox"><rect width="1" height="1"/></clipPath>'
BLUE = (32/255, 160/255, 224/255)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def svg(body, defs=""):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="128" height="96" viewBox="0 0 128 96"><defs>{HALF}{FULL}{defs}</defs>{body}</svg>\n'


def clip(ctx, box, fraction=.5):
    x, y, w, h = box
    ctx.rectangle(x, y, w*fraction, h)
    ctx.clip()


def rectangle(ctx, box):
    ctx.rectangle(*box)
    ctx.fill()


def curve(ctx):
    ctx.move_to(0, 0)
    ctx.curve_to(0, 60, 60, 60, 60, 0)
    ctx.close_path()


class CairoPen(BasePen):
    def __init__(self, glyphset, ctx):
        super().__init__(glyphset)
        self.ctx = ctx

    def _moveTo(self, p):
        self.ctx.move_to(*p)

    def _lineTo(self, p):
        self.ctx.line_to(*p)

    def _curveToOne(self, a, b, c):
        self.ctx.curve_to(*a, *b, *c)

    def _qCurveToOne(self, a, b):
        p = self._getCurrentPoint()
        self.ctx.curve_to(p[0]+2*(a[0]-p[0])/3, p[1]+2*(a[1]-p[1])/3,
                          b[0]+2*(a[0]-b[0])/3, b[1]+2*(a[1]-b[1])/3, *b)

    def _closePath(self):
        self.ctx.close_path()

    def _endPath(self):
        pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--fonts", required=True, type=Path)
    args = parser.parse_args()
    regular = args.fonts / "Go-Regular.ttf"
    assert sha(regular.read_bytes()) == "197d9f3703b4c00af609178876a8d73e396f64fe438b2c871778566632374be3"
    font = TTFont(regular)
    kern_font = TTFont(ROOT / "tests/data/svg/text-kern.ttf")
    fonts = {"regular": font, "kern": kern_font}

    def text_cells(value, x, y, size, anchor="start", font=font):
        scale = size/font["head"].unitsPerEm
        cmap = font.getBestCmap()
        result = []
        cursor = 0.
        previous = None
        for c in value:
            name = cmap[ord(c)]
            if previous and "kern" in font:
                cursor += font["kern"].kernTables[0].kernTable.get((previous, name), 0)*scale
            advance = font["hmtx"][name][0]*scale
            result.append([c, cursor, advance])
            cursor += advance
            previous = name
        offset = {"start": 0, "middle": -cursor/2, "end": -cursor}[anchor]
        ascent = font["hhea"].ascent*scale
        descent = -font["hhea"].descent*scale
        cells = [[x+offset+p, y-ascent, advance, ascent+descent] for c, p, advance in result]
        return result, cells, offset, cursor

    def paint_text(ctx, value, x, y, size, anchor="start", font=font):
        glyphs, cells, offset, width = text_cells(value, x, y, size, anchor, font)
        glyphset = font.getGlyphSet()
        cmap = font.getBestCmap()
        for c, p, advance in glyphs:
            ctx.save()
            ctx.translate(x+offset+p, y)
            scale = size/font["head"].unitsPerEm
            ctx.scale(scale, -scale)
            glyphset[cmap[ord(c)]].draw(CairoPen(glyphset, ctx))
            ctx.restore()
            ctx.fill()

    def union(cells):
        x = min(b[0] for b in cells)
        y = min(b[1] for b in cells)
        return [x, y, max(b[0]+b[2] for b in cells)-x, max(b[1]+b[3] for b in cells)-y]

    sources, painters, boxes = {}, {}, []
    # x(t)=90t+90t²-120t³ for the sheared curve; interior maximum
    # at (1+sqrt(5))/4. The old transformed local AABB would give 82.5.
    t = (1+math.sqrt(5))/4
    xmax = 90*t+90*t*t-120*t*t*t
    curve_box = [8, 6, xmax, 64]
    sources["group-curves"] = svg('<g id="target" transform="matrix(1 .04 .08 1 1 1)" clip-path="url(#c)" fill="#20a0e0"><path d="M0 0C0 60 60 60 60 0Z" transform="matrix(1 0 .5 1 8 6)"/><rect x="14" y="60" width="45" height="10"/></g>')

    def group_curves(ctx):
        ctx.transform(cairo.Matrix(1, .04, .08, 1, 1, 1))
        clip(ctx, curve_box)
        ctx.save()
        ctx.transform(cairo.Matrix(1, 0, .5, 1, 8, 6))
        curve(ctx)
        ctx.fill()
        ctx.restore()
        rectangle(ctx, (14, 60, 45, 10))

    painters["group-curves"] = group_curves
    boxes.append(dict(name="sheared-cubic", body='<g id="target" clip-path="url(#full)"><path d="M0 0C0 60 60 60 60 0Z" transform="matrix(1 0 .5 1 8 6)"/></g>', bbox=[8, 6, xmax, 45], method="analytic cubic derivative root (1+sqrt(5))/4"))
    sources["display-visibility"] = svg('<g id="target" clip-path="url(#c)" fill="#20a0e0"><rect x="8" y="10" width="100" height="65"/><g display="none"><rect x="-500" y="-500" width="1000" height="1000" display="inline"/></g><rect x="108" y="20" width="12" height="20" visibility="hidden"/><rect x="4" y="15" width="2" height="2" opacity="0"/></g>')

    def display(ctx):
        clip(ctx, (4, 10, 116, 65))
        rectangle(ctx, (8, 10, 100, 65))

    painters["display-visibility"] = display
    boxes.append(dict(name="display-visibility", body='<g id="target" clip-path="url(#full)"><rect x="8" y="10" width="100" height="65"/><g display="none"><rect x="-500" width="1000" height="1000"/></g><rect x="108" y="20" width="12" height="20" visibility="hidden"/><rect x="4" y="15" width="2" height="2" opacity="0"/></g>', bbox=[4, 10, 116, 65], method="explicit union; display excluded, visibility/opacity included"))
    defs = '<style>.host .tile .excluded{display:none}.tile > .excluded{display:inherit}</style><g transform="translate(900 900)" display="none"><g id="tile" class="tile" transform="translate(2 3)"><rect class="excluded" x="4" y="5" width="64" height="45"/><rect x="20" y="55" width="50" height="15"/></g><use id="chain" href="#tile" x="5" y="7" transform="scale(1.1 .9)"/></g>'
    sources["use-chain"] = svg('<g class="host"><use id="target" href="#chain" x="8" y="6" clip-path="url(#c)" fill="#20a0e0"/></g>', defs)
    use_box = [8+1.1*11, 6+.9*15, 1.1*66, .9*65]

    def use_chain(ctx):
        clip(ctx, use_box)
        ctx.translate(8, 6)
        ctx.scale(1.1, .9)
        ctx.translate(7, 10)
        rectangle(ctx, (4, 5, 64, 45))
        rectangle(ctx, (20, 55, 50, 15))

    painters["use-chain"] = use_chain
    boxes.append(dict(name="use-chain", body='<g class="host"><use id="target" href="#chain" x="8" y="6" clip-path="url(#full)"/></g>', defs=defs, bbox=use_box, method="explicit affine placement; source ancestors excluded; use shadow selector boundary"))
    sources["nested-clips"] = svg('<g id="target" clip-path="url(#c)" fill="#20a0e0"><g clip-path="url(#small)"><rect x="10" y="10" width="100" height="70"/></g><rect x="70" y="14" width="40" height="60" fill="none" stroke="#20a0e0" stroke-width="4"/></g>', '<clipPath id="small"><rect x="10" y="10" width="80" height="20"/></clipPath>')

    def nested(ctx):
        clip(ctx, (10, 10, 100, 70))
        ctx.save()
        clip(ctx, (10, 10, 80, 20), 1)
        rectangle(ctx, (10, 10, 100, 70))
        ctx.restore()
        ctx.set_line_width(4)
        ctx.rectangle(70, 14, 40, 60)
        ctx.stroke()

    painters["nested-clips"] = nested
    # Spaces and cell ascent/descent differ substantially from the ink bbox.
    for name, value, anchor in [("text-cells", "  AVg  ", "middle"), ("text-end", "AV ", "end")]:
        _, cells, _, _ = text_cells(value, 64, 58, 28, anchor)
        box = union(cells)
        sources[name] = svg(f'<text id="target" x="64" y="58" font-size="28" xml:space="preserve" text-anchor="{anchor}" fill="#20a0e0" clip-path="url(#c)">{value}</text>')

        def painter(ctx, value=value, anchor=anchor, box=box):
            clip(ctx, box)
            paint_text(ctx, value, 64, 58, 28, anchor)

        painters[name] = painter
        boxes.append(dict(name=name, body=f'<text id="target" x="64" y="58" font-size="28" xml:space="preserve" text-anchor="{anchor}" clip-path="url(#full)">{value}</text>', bbox=box, method="fontTools hmtx/hhea full glyph cells with advance anchor"))
    # A shared kerned run crosses the span boundary; its V belongs to the span
    # AFTER the A/V kern. No advance or anchor reset is allowed at that boundary.
    _, cells, _, _ = text_cells("AV A", 12, 58, 28, font=kern_font)
    span_box = union(cells[1:3])
    sources["tspan-kern"] = svg('<text x="12" y="58" font-size="28" fill="#20a0e0">A<tspan id="target" clip-path="url(#c)">V </tspan>A</text>')

    def span(ctx, cells=cells, span_box=span_box):
        paint_text(ctx, "A", 12, 58, 28, font=kern_font)
        ctx.save()
        clip(ctx, span_box)
        # Paint each independently positioned original glyph; this is not an
        # assumption that Cairo's text parser shares SVG chunk semantics.
        for c, cell in zip("V ", cells[1:3]):
            paint_text(ctx, c, cell[0], 58, 28, font=kern_font)
        ctx.restore()
        paint_text(ctx, "A", cells[3][0], 58, 28, font=kern_font)

    painters["tspan-kern"] = span
    boxes.append(dict(name="tspan-kern", font="kern", body='<text x="12" y="58" font-size="28">A<tspan id="target" clip-path="url(#full)">V </tspan>A</text>', bbox=span_box, method="fontTools original kern table across tspan ownership boundary"))
    for value in ["   ", "jAV  "]:
        _, cells, _, _ = text_cells(value, 20, 50, 24)
        boxes.append(dict(name="space-only" if value.strip()=="" else "bearing-and-space", body=f'<text id="target" x="20" y="50" font-size="24" xml:space="preserve" clip-path="url(#full)">{value}</text>', bbox=union(cells), method="full glyph cells; whitespace with no outlines remains geometry"))
    # Group text after a shear: transform each actual cell, not an ink box.
    _, cells, _, _ = text_cells("AV g", 10, 40, 24)
    corners = [(x+.4*y+3, y+8) for cx, cy, cw, ch in cells for x in [cx,cx+cw] for y in [cy,cy+ch]]
    bbox = [min(x for x,y in corners), min(y for x,y in corners), max(x for x,y in corners)-min(x for x,y in corners), max(y for x,y in corners)-min(y for x,y in corners)]
    boxes.append(dict(name="group-text-shear", body='<g id="target" clip-path="url(#full)"><text x="10" y="40" font-size="24" transform="matrix(1 0 .4 1 3 8)">AV g</text></g>', bbox=bbox, method="fontTools glyph cells transformed independently into ancestor space"))

    # Image meet content is smaller than its SVG viewport geometry.
    bitmap = io.BytesIO()
    Image.new("RGBA", (16, 8), (32, 160, 224, 255)).save(bitmap, format="PNG")
    uri = "data:image/png;base64,"+base64.b64encode(bitmap.getvalue()).decode()
    sources["group-image"] = svg(f'<g id="target" clip-path="url(#c)"><image x="10" y="8" width="100" height="80" href="{uri}" preserveAspectRatio="xMaxYMid meet"/></g>')

    def image(ctx):
        clip(ctx, (10, 8, 100, 80))
        rectangle(ctx, (10, 23, 100, 50))

    painters["group-image"] = image
    boxes.append(dict(name="group-image", body=f'<g id="target" clip-path="url(#full)"><image x="10" y="8" width="100" height="80" href="{uri}"/></g>', bbox=[10, 8, 100, 80], method="SVG image viewport before meet placement"))
    OUT.mkdir(parents=True, exist_ok=True)
    for name, source in sources.items():
        (OUT / (name+".svg")).write_text(source)
    for case in boxes:
        (OUT / ("oracle-"+case["name"]+".svg")).write_text(svg(case.pop("body"), case.pop("defs", "")))

    def render(name, painter, matrix=None):
        for ss in [1, 4]:
            surface = cairo.ImageSurface(cairo.FORMAT_ARGB32, W*ss, H*ss)
            ctx = cairo.Context(surface)
            ctx.scale(ss, ss)
            ctx.set_source_rgb(*BLUE)
            if matrix:
                ctx.transform(cairo.Matrix(*matrix))
            painter(ctx)
            data = io.BytesIO()
            surface.write_to_png(data)
            im = Image.open(io.BytesIO(data.getvalue())).convert("RGBA")
            if ss > 1:
                im = im.convert("RGBa").resize((W, H), Image.Resampling.BOX).convert("RGBA")
            im.save(OUT / f"{name}-cairo-ss{ss}.png")

    for name, painter in painters.items():
        render(name, painter)
    animation = []
    for tick in [0, 25, 50, 75, 100]:
        t = tick/100
        # Linear Go Transform between original and this affine-transformed
        # snapshot is precisely the interpolated affine on all control points.
        matrix = [1-.2*t, .12*t, -.1*t, 1+.05*t, 10*t, -3*t]
        render(f"animated-{tick:03}", group_curves, matrix)
        animation.append(dict(time=t, svg_matrix=matrix))
    (OUT / "bbox-oracle.json").write_text(json.dumps(dict(cases=boxes, animation=animation), indent=2)+"\n")
    provenance = dict(
        generator="tests/reference/svg_container_bbox.py", generator_sha256=sha(Path(__file__).read_bytes()),
        versions=dict(Python=platform.python_version(), Cairo=cairo.cairo_version_string(), fontTools=fontTools.__version__, Pillow=pillow_version),
        font_sha256={"Go-Regular.ttf": sha(regular.read_bytes()), "text-kern.ttf": sha((ROOT/"tests/data/svg/text-kern.ttf").read_bytes())},
        primary_spec=["https://www.w3.org/TR/SVG2/coords.html#BoundingBoxes", "https://www.w3.org/TR/SVG2/coords.html#ObjectBoundingBoxUnits"],
        oracle="Independently authored direct Cairo geometry and explicit bbox clips. Exact analytic cubic derivative extrema and fontTools hmtx/kern/hhea numerical oracles. No Go source, exported SVG, Go pixels or production bbox values consumed.",
        text_policy="SVG full glyph cells: horizontal advances and full font ascent/descent, including preserved spaces. Original outlines paint the glyph ink; these are intentionally different geometries.",
        animation="Linear interpolation of original geometry/clip and a separately transformed snapshot, tested at 0,.25,.5,.75,1; shared imported scene is sampled concurrently by Go tests.",
        limitations="Bounded horizontal text API and existing cubic SVG shape approximations. No nested viewport/symbol bbox claim. Direct Cairo paths avoid CairoSVG container bbox/clip parser behavior. This is SVG evidence, not a new Manim parity claim.",
        files_sha256={p.name: sha(p.read_bytes()) for p in sorted(OUT.iterdir()) if p.suffix in [".svg", ".png"] or p.name == "bbox-oracle.json"})
    (OUT / "provenance.json").write_text(json.dumps(provenance, indent=2)+"\n")
    print(f"wrote {len(sources)} Cairo cases, {len(boxes)} numerical oracles and {len(animation)} animation frames at SS1/SS4")


if __name__ == "__main__":
    main()
