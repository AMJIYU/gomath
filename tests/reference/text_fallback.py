"""Development-only font fallback reference; never reads Go layout output.

Build small licensed Go-font derivatives with real unmodified outlines and
explicit test kerning/line tables. fontTools hmtx/kern/hhea + BoundsPen provide
independent analytical evidence; SVGPathPen and CairoSVG provide pixel evidence.
Run with --fonts pointing at x/image/font/gofont/ttfs. No runtime/test dependency.
"""
import argparse
import hashlib
import json
from pathlib import Path
import platform

import cairocffi
import cairosvg
import fontTools
from fontTools import subset
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.ttLib import TTFont, newTable
from fontTools.ttLib.tables._k_e_r_n import KernTable_format_0


SOURCES = {
    "Go-Regular.ttf": "197d9f3703b4c00af609178876a8d73e396f64fe438b2c871778566632374be3",
    "Go-Bold.ttf": "c18494baa7ea35b8dbfac3861787d360b9c0ab91232562371a09e6fbf09aaa40",
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def make_fixture(source, target, characters, family, pairs, metrics=None):
    f = TTFont(source, recalcTimestamp=False)
    opts = subset.Options()
    opts.notdef_glyph = opts.notdef_outline = opts.recommended_glyphs = True
    cutter = subset.Subsetter(options=opts)
    cutter.populate(text=characters)
    cutter.subset(f)
    for record in f["name"].names:
        if record.nameID in (1, 2, 3, 4, 6):
            name = "Regular" if record.nameID == 2 else family
            record.string = name.encode(record.getEncoding())
    cmap = f.getBestCmap()
    kern = newTable("kern")
    kern.version = 0
    table = KernTable_format_0()
    table.version, table.coverage = 0, 1
    table.kernTable = {(cmap[ord(a)], cmap[ord(b)]): amount for a, b, amount in pairs}
    kern.kernTables = [table]
    f["kern"] = kern
    if metrics:
        f["hhea"].ascent, f["hhea"].descent = metrics
    f.save(target)
    return TTFont(target, recalcTimestamp=False)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--fonts", required=True, type=Path)
    args = ap.parse_args()
    out = Path(__file__).resolve().parents[1] / "data/text/fallback"
    out.mkdir(parents=True, exist_ok=True)
    for name, expected in SOURCES.items():
        if digest((args.fonts/name).read_bytes()) != expected:
            raise ValueError("Source font hash mismatch: " + name)
    fixtures = [
        ("primary.ttf", "Go-Regular.ttf", " AVx", "GoMathFixturePrimary",
         [("A", "V", -160), ("V", "A", -120)], None),
        ("secondary.ttf", "Go-Bold.ttf", " AVβg", "GoMathFixtureSecondary",
         [("A", "V", -64), ("β", "g", -96), ("A", "β", -128), ("β", "A", -80)], (2300, -700)),
    ]
    fonts, font_records = [], []
    for filename, source, chars, family, pairs, line_metrics in fixtures:
        f = make_fixture(args.fonts/source, out/filename, chars, family, pairs, line_metrics)
        fonts.append(f)
        font_records.append(dict(file=filename, sha256=digest((out/filename).read_bytes()), source=source,
                                 characters=chars, kern_design_units=pairs,
                                 upem=f["head"].unitsPerEm,
                                 ascent=f["hhea"].ascent, descent=-f["hhea"].descent,
                                 modifications="Subset cmap/name; explicit fixture-only kern table; secondary hhea ascent/descent. Original glyf/hmtx preserved."))
    # A font can parse while its selected outline is malformed. Deliberately
    # corrupt A's contour count to test error propagation, never glyph fallback.
    broken = bytearray((out/"primary.ttf").read_bytes())
    primary = fonts[0]
    glyph_id = primary.getGlyphID(primary.getBestCmap()[ord("A")])
    location = primary.reader.tables["glyf"].offset + primary["loca"].locations[glyph_id]
    broken[location:location+2] = (32767).to_bytes(2, "big")
    (out/"broken.ttf").write_bytes(broken)
    records = []
    for name, value, order, allow in [
        ("ordered", "AV βgA\n\tVx ", [0, 1], False),
        ("reversed", "AV βgA\n\tVx ", [1, 0], False),
        ("primary-ink", "AV\n x ", [0, 1], False),
        ("missing", "A\U0010ffffV", [0, 1], True),
    ]:
        x, y, width, lines, previous = 0., 0., 0., 1, None
        glyph_records, paths = [], []
        for rune in value:
            if rune == "\n":
                width, x, y, lines, previous = max(width, x), 0., y-1.3, lines+1, None
                continue
            rune = " " if rune == "\t" else rune
            selected, glyph_name = order[0], ".notdef"
            for index in order:
                candidate = fonts[index].getBestCmap().get(ord(rune))
                if candidate:
                    selected, glyph_name = index, candidate
                    break
            if glyph_name == ".notdef" and not allow:
                raise ValueError("Missing reference glyph")
            f, upem = fonts[selected], fonts[selected]["head"].unitsPerEm
            kern = 0
            if previous and previous[0] == selected:
                kern = f["kern"].kernTables[0].kernTable.get((previous[1], glyph_name), 0)/upem
            x += kern
            glyphset = f.getGlyphSet()
            bounds, pen = BoundsPen(glyphset), SVGPathPen(glyphset)
            glyphset[glyph_name].draw(bounds)
            glyphset[glyph_name].draw(pen)
            bbox = ([x+bounds.bounds[0]/upem, y+bounds.bounds[1]/upem,
                     x+bounds.bounds[2]/upem, y+bounds.bounds[3]/upem] if bounds.bounds else None)
            advance = f["hmtx"][glyph_name][0]/upem
            glyph_records.append(dict(rune=ord(rune), font=selected, glyph=glyph_name,
                                      x=x, baseline=y, advance=advance, kern=kern, bbox=bbox))
            if bbox:
                paths.append(f'<path transform="translate({x:.17g} {y:.17g}) scale({1/upem:.17g})" d="{pen.getCommands()}"/>')
            x += advance
            previous = (selected, glyph_name)
        ascent = max(fonts[i]["hhea"].ascent/fonts[i]["head"].unitsPerEm for i in order)
        descent = max(-fonts[i]["hhea"].descent/fonts[i]["head"].unitsPerEm for i in order)
        svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="768" height="512" viewBox="0 0 768 512">'
               '<rect width="768" height="512" fill="black"/><g fill="white" '
               'transform="translate(64 192) scale(128 -128)">'+"".join(paths)+'</g></svg>\n').encode()
        png = cairosvg.svg2png(bytestring=svg)
        (out/(name+".svg")).write_bytes(svg)
        (out/(name+".png")).write_bytes(png)
        records.append(dict(name=name, text=value, order=order, allow_missing=allow,
                            metrics=dict(width=max(width,x), height=ascent+descent+(lines-1)*1.3,
                                         ascent=ascent, descent=descent, lines=lines),
                            glyphs=glyph_records, svg_sha256=digest(svg), png_sha256=digest(png)))
    result = dict(provenance="Independent fontTools table/outline analysis and manually specified ordered font policy; no Go output consumed.",
                  sources=SOURCES, fixtures=font_records, records=records,
                  broken=dict(file="broken.ttf", sha256=digest(broken),
                              modification="Primary fixture: A numberOfContours corrupted to 32767; intentionally malformed lazy outline."),
                  generator_sha256=digest(Path(__file__).read_bytes()),
                  versions=dict(Python=platform.python_version(), fontTools=fontTools.__version__,
                                CairoSVG=cairosvg.__version__, Cairo=cairocffi.cairo_version_string()),
                  raster=dict(width=768, height=512, scale=128, origin=[64,192]))
    (out/"reference.json").write_text(json.dumps(result,indent=2)+"\n")
    print("Wrote two real-outline font derivatives and four independent layout/SVG/PNG references.")


if __name__ == "__main__":
    main()
