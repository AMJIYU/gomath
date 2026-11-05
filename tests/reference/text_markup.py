"""Independent development-only finite markup layout reference.

Runs below are specified manually, not parsed from Go output. fontTools reads
original Go faces plus frozen real-outline kern fixtures; hmtx/kern/hhea and
BoundsPen define run placement and boxes. CairoSVG renders outline paths.
This validates the declared per-rune layout, not Pango shaping/markup parity.
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


SOURCES = {
    "Go-Regular.ttf": "197d9f3703b4c00af609178876a8d73e396f64fe438b2c871778566632374be3",
    "Go-Bold.ttf": "c18494baa7ea35b8dbfac3861787d360b9c0ab91232562371a09e6fbf09aaa40",
    "Go-Italic.ttf": "6efef00d080c8fe68bbe2bf08526294c83d52d7b90b16f938953025c0868cc57",
    "Go-Bold-Italic.ttf": "d6c33857c7ea7d740a8e1d81d37261782f698e9c17a2c9bf3bb0abbaf99e3c90",
    "primary.ttf": "5ef2785c0d15488681ee77598939d3aaaa41268530c42e714c07fd6529c68ade",
    "secondary.ttf": "a5bb499bd3027823412d2901132a9ffe3f8347bf3275e3b1db5c15f2da300155",
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--fonts", required=True, type=Path)
    args = ap.parse_args()
    root = Path(__file__).resolve().parents[2]
    out = root / "tests/data/text/markup"
    out.mkdir(parents=True, exist_ok=True)
    fonts = []
    for name, expected in SOURCES.items():
        path = args.fonts/name if name.startswith("Go-") else root/"tests/data/text/fallback"/name
        if digest(path.read_bytes()) != expected:
            raise ValueError("Source hash mismatch: " + name)
        fonts.append(TTFont(path))
    white, red, green = "ffffff", "ff4000", "20c080"
    # Font indices 0..3 = regular/bold/italic/bold-italic. 4/5 = frozen
    # complementary test fixtures with declared real kern tables/line metrics.
    cases = [
        ("nested", "A<b>V<i>g</i>V</b><i>x<b>A</b>x</i>A", [0,1,2,3],
         [(c, f, white) for c, f in zip("AVgVxAxA", [0,1,3,1,2,3,2,0])]),
        ("scope", '<span foreground="#ff4000">A<b>V<i>g</i></b><span foreground="#20c080">xβ</span>A</span>V\n<i>g</i>\t&amp;',
         [0,1,2,3], [("A",0,red),("V",1,red),("g",3,red),("x",0,green),("β",0,green),
                     ("A",0,red),("V",0,white),("\n",0,white),("g",2,white),("\t",0,white),("&",0,white)]),
        ("kerning", '<span foreground="#ff4000">A<span foreground="#20c080">V</span>A</span>V<b>AV</b>A',
         [4,5], [("A",4,red),("V",4,green),("A",4,red),("V",4,white),("A",5,white),("V",5,white),("A",4,white)]),
        ("escaped", '&lt;&amp;&gt;&quot;&apos; &#x3b2;\nA&#10;<b>g</b> ', [0,1,2,3],
         [(c, 1 if c == "g" else 0, white) for c in '<&>"\' β\nA\ng ']),
    ]
    records = []
    for name, source, configured, runes in cases:
        x, y, width, lines, previous = 0., 0., 0., 1, None
        glyphs, paths = [], []
        for rune, index, color in runes:
            if rune == "\n":
                width, x, y, lines, previous = max(width,x), 0., y-1.3, lines+1, None
                continue
            rune = " " if rune == "\t" else rune
            f = fonts[index]
            upem, cmap, glyphset = f["head"].unitsPerEm, f.getBestCmap(), f.getGlyphSet()
            glyph_name = cmap[ord(rune)]
            kern = 0.
            if previous and previous[0] == index and "kern" in f:
                kern = f["kern"].kernTables[0].kernTable.get((previous[1],glyph_name),0)/upem
            x += kern
            bounds, pen = BoundsPen(glyphset), SVGPathPen(glyphset)
            glyphset[glyph_name].draw(bounds)
            glyphset[glyph_name].draw(pen)
            bbox = ([x+bounds.bounds[0]/upem, y+bounds.bounds[1]/upem,
                     x+bounds.bounds[2]/upem, y+bounds.bounds[3]/upem] if bounds.bounds else None)
            advance = f["hmtx"][glyph_name][0]/upem
            glyphs.append(dict(rune=ord(rune), font=index, color=color, x=x, baseline=y,
                               bbox=bbox, kern=kern, advance=advance))
            if bbox:
                paths.append(f'<path fill="#{color}" transform="translate({x:.17g} {y:.17g}) '
                             f'scale({1/upem:.17g})" d="{pen.getCommands()}"/>')
            x += advance
            previous = (index,glyph_name)
        ascent = max(fonts[i]["hhea"].ascent/fonts[i]["head"].unitsPerEm for i in configured)
        descent = max(-fonts[i]["hhea"].descent/fonts[i]["head"].unitsPerEm for i in configured)
        svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="640" viewBox="0 0 1024 640">'
               '<rect width="1024" height="640" fill="black"/><g transform="translate(64 192) scale(128 -128)">'
               +"".join(paths)+'</g></svg>\n').encode()
        png = cairosvg.svg2png(bytestring=svg)
        (out/(name+".svg")).write_bytes(svg)
        (out/(name+".png")).write_bytes(png)
        records.append(dict(name=name, source=source, custom_fonts=name=="kerning",
                            text="".join(c for c,_,_ in runes), glyphs=glyphs,
                            metrics=dict(width=max(width,x), height=ascent+descent+(lines-1)*1.3,
                                         ascent=ascent, descent=descent, lines=lines),
                            svg_sha256=digest(svg), png_sha256=digest(png)))
    metadata = dict(provenance="Manually specified decoded styled runes; independent fontTools tables/BoundsPen/SVGPathPen + CairoSVG. No Go output read; not a Pango/Manim shaping oracle.",
                    manim_source_inspected="16ea443e2a5f1d79db0a721161411ce916d6e733:manim/mobject/text/text_mobject.py:MarkupText",
                    sources=SOURCES, records=records, generator_sha256=digest(Path(__file__).read_bytes()),
                    versions=dict(Python=platform.python_version(), fontTools=fontTools.__version__,
                                  CairoSVG=cairosvg.__version__, Cairo=cairocffi.cairo_version_string()),
                    raster=dict(width=1024,height=640,scale=128,origin=[64,192]))
    (out/"reference.json").write_text(json.dumps(metadata,indent=2)+"\n")
    print("Wrote four independent styled-run fontTools/SVG/Cairo references.")


if __name__ == "__main__":
    main()
