"""Independent development-only multiline font/layout references.

Fixed operand grids are specified here, never parsed from a Go-generated file.
fontTools reads original GoRegular hmtx/glyf data; manual SVG placements follow
the documented finite baseline/alignment policy. CairoSVG/Cairo rasterize these
source outlines. This is not an amsmath/TeX or Manim pixel-equivalence claim.
No reference tool is required by Go tests or by runtime.
"""
import argparse
import hashlib
import json
from pathlib import Path
import platform

import cairocffi
import cairosvg
import fontTools
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--font", required=True, type=Path)
    args = ap.parse_args()
    out = Path(__file__).resolve().parents[1] / "data/mathtex/multiline"
    out.mkdir(parents=True, exist_ok=True)
    font = TTFont(args.font)
    glyphs, upem = font.getGlyphSet(), font["head"].unitsPerEm
    operands = {}
    for letter in "ngWiA":
        name = font.getBestCmap()[ord(letter)]
        path, bounds = SVGPathPen(glyphs), BoundsPen(glyphs)
        glyphs[name].draw(path)
        glyphs[name].draw(bounds)
        bbox = [v/upem for v in bounds.bounds]
        advance = font["hmtx"][name][0]/upem
        left = max(0, -bbox[0])
        operands[letter] = dict(path=path.getCommands(), bbox=bbox, advance=advance,
                                origin_shift=left, width=max(advance, bbox[2])+left)

    # Manually selected operands, including unequal advances and descenders.
    grids = [
        ("aligned-2pairs", "aligned", [
            [("n", "ff0000"), ("g", "0000ff"), ("W", "008040"), ("i", "8040c0")],
            [("W", "008040"), ("i", "ff0000"), ("g", "8040c0"), ("n", "0000ff")],
            [("i", "0000ff"), ("n", "8040c0"), ("A", "ff0000"), ("g", "008040")],
        ]),
        ("gathered-4rows", "gathered", [
            [("W", "ff0000")], [("i", "0000ff")], [("g", "008040")], [("n", "8040c0")],
        ]),
    ]
    records = []
    for name, environment, rows in grids:
        widths = [max(operands[row[j][0]]["width"] for row in rows) for j in range(len(rows[0]))]
        ascents = [max(.7, *(operands[letter]["bbox"][3] for letter, _ in row)) for row in rows]
        depths = [max(.2, *(-operands[letter]["bbox"][1] for letter, _ in row)) for row in rows]
        total = sum(ascents)+sum(depths)+.3*(len(rows)-1)
        top, bottom = .16+total/2, total/2-.16
        starts = [sum(widths[:j]) + sum(.12 if k % 2 else 1 for k in range(1, j+1))
                  for j in range(len(widths))]
        width = starts[-1]+widths[-1]
        cells, paths = [], []
        for i, row in enumerate(rows):
            baseline = top-sum(ascents[:i+1])-sum(depths[:i])-.3*i
            for j, (letter, color) in enumerate(row):
                glyph = operands[letter]
                dx = ((widths[j]-glyph["width"])/2 if environment == "gathered"
                      else widths[j]-glyph["width"] if j % 2 == 0 else 0)
                x = starts[j]+dx
                cells.append(dict(row=i, column=j, glyph=letter, color=color,
                                  x=x, baseline=baseline, width=glyph["width"],
                                  bbox=[x+glyph["origin_shift"]+glyph["bbox"][0], baseline+glyph["bbox"][1],
                                        x+glyph["origin_shift"]+glyph["bbox"][2], baseline+glyph["bbox"][3]]))
                paths.append(f'<path fill="#{color}" transform="translate({x+glyph["origin_shift"]:.17g} '
                             f'{baseline:.17g}) scale({1/upem:.17g})" d="{glyph["path"]}"/>')
        lines = ["&".join(r"\textcolor[HTML]{"+color+"}{"+letter+"}" for letter, color in row) for row in rows]
        source = r"\begin{"+environment+"}"+r"\\".join(lines)+r"\end{"+environment+"}"
        svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="768" height="768" '
               'viewBox="0 0 768 768"><rect width="768" height="768" fill="#000000"/>'
               '<g transform="translate(64 384) scale(128 -128)">'+"".join(paths)+'</g></svg>\n').encode()
        png = cairosvg.svg2png(bytestring=svg)
        (out/(name+".svg")).write_bytes(svg)
        (out/(name+".png")).write_bytes(png)
        records.append(dict(name=name, environment=environment, source=source,
                            rows=len(rows), columns=len(widths), width=width, height=top, depth=bottom,
                            cells=cells, svg_sha256=hashlib.sha256(svg).hexdigest(),
                            png_sha256=hashlib.sha256(png).hexdigest()))
    metadata = dict(
        provenance="Manually specified operand grids, original fontTools hmtx/glyf outlines, analytical placement, independent CairoSVG raster; no Go output read.",
        versions=dict(Python=platform.python_version(), fontTools=fontTools.__version__,
                      CairoSVG=cairosvg.__version__, Cairo=cairocffi.cairo_version_string()),
        font=dict(file=args.font.name, sha256=hashlib.sha256(args.font.read_bytes()).hexdigest()),
        generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        coordinates=dict(width=768, height=768, ppem=128, origin=[64,384]),
        policy=dict(pair_gap=.12, pair_separation=1, row_gap=.3, strut_height=.7, strut_depth=.2, axis=.16),
        operands={letter:{k:v for k,v in data.items() if k != "path"} for letter,data in operands.items()},
        limits="Finite declared layout policy, not full TeX/amsmath spacing/default equivalence.",
        records=records,
    )
    (out/"reference.json").write_text(json.dumps(metadata,indent=2)+"\n")
    print(f"Wrote two independent 768x768 multiline SVG/PNG pairs and {sum(len(r['cells']) for r in records)} operand metric/placement records.")


if __name__ == "__main__":
    main()
