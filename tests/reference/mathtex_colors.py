"""Development-only independent math color reference; no Go output is read.

The two SVGs are manually assembled from the published mathtex spacing policy
and original GoRegular hmtx/glyf outlines read by fontTools. CairoSVG/Cairo render
them directly. This establishes solid color, scoped operand/decoration painting
and these two analytic layouts, not full TeX/xcolor equivalence. Go tests only
read the fixed artifacts; Python/native tools are never runtime dependencies.
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
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--font", type=Path, required=True)
    args = parser.parse_args()
    out = Path(__file__).resolve().parents[1] / "data/mathtex/colors"
    out.mkdir(parents=True, exist_ok=True)
    font = TTFont(args.font)
    glyphs = font.getGlyphSet()
    upem = font["head"].unitsPerEm

    def glyph(letter):
        name = font.getBestCmap()[ord(letter)]
        path = SVGPathPen(glyphs)
        bounds = BoundsPen(glyphs)
        glyphs[name].draw(path)
        glyphs[name].draw(bounds)
        return dict(path=path.getCommands(), advance=font["hmtx"][name][0]/upem,
                    bounds=[v/upem for v in bounds.bounds])

    n, g = glyph("n"), glyph("g")
    # No negative bearings for these chosen source glyphs. Advances exceed ink.
    assert n["bounds"][0] >= 0 and g["bounds"][0] >= 0
    assert n["advance"] >= n["bounds"][2] and g["advance"] >= g["bounds"][2]

    def text(data, x, y, color):
        return (f'<path fill="{color}" transform="translate({x:.17g} {y:.17g}) '
                f'scale({1/upem:.17g})" d="{data["path"]}"/>')

    width = max(n["advance"], g["advance"]) + .25
    ny = .16 + .24 + .045/2 + max(0, -n["bounds"][1])
    dy = .16 - .24 - .045/2 - max(0, g["bounds"][3])
    fraction = (text(n, (width-n["advance"])/2, ny, "#0000ff")
                + text(g, (width-g["advance"])/2, dy, "#008040")
                + f'<path d="M0 .16H{width:.17g}" fill="none" stroke="#ff0000" '
                  'stroke-width=".045" stroke-linecap="round"/>')
    overline = (text(g, 0, 0, "#008040")
                + f'<path d="M0 {g["bounds"][3]+.1:.17g}h{g["advance"]:.17g}'
                  'v.045H0Z" fill="#ff0000"/>')
    records = []
    for name, formula, body, colors in [
        ("fraction", r"\color{red}\dfrac{\textcolor{blue}{n}}{\textcolor[HTML]{008040}{g}}",
         fraction, ["ff0000", "0000ff", "008040"]),
        ("overline", r"\color{red}\overline{\textcolor[HTML]{008040}{g}}",
         overline, ["ff0000", "008040"]),
    ]:
        svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="512" height="512" '
               'viewBox="0 0 512 512"><rect width="512" height="512" fill="#000000"/>'
               '<g transform="translate(160 256) scale(204.8 -204.8)">'
               + body + '</g></svg>\n').encode()
        png = cairosvg.svg2png(bytestring=svg)
        (out / (name+".svg")).write_bytes(svg)
        (out / (name+".png")).write_bytes(png)
        records.append(dict(name=name, source=formula, colors=colors,
                            svg_sha256=hashlib.sha256(svg).hexdigest(),
                            png_sha256=hashlib.sha256(png).hexdigest()))
    manifest = dict(
        provenance="Independent manually assembled SVG + original fontTools outlines; no gomath output read.",
        font=dict(file=args.font.name, sha256=hashlib.sha256(args.font.read_bytes()).hexdigest()),
        versions=dict(Python=platform.python_version(), fontTools=fontTools.__version__,
                      CairoSVG=cairosvg.__version__, Cairo=cairocffi.cairo_version_string()),
        generator_sha256=hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        geometry=dict(width=512, height=512, ppem=204.8, origin=[160, 256],
                      fraction_axis=.16, fraction_gap=.24, fraction_stroke=.045,
                      overline_gap=.1, overline_thickness=.045),
        limits="Two selected analytic layouts and sRGB paints, not a whole TeX engine/xcolor oracle.",
        records=records,
    )
    (out / "reference.json").write_text(json.dumps(manifest, indent=2)+"\n")
    print(json.dumps(manifest, indent=2))


if __name__ == "__main__":
    main()
