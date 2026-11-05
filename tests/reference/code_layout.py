"""Independent development reference for the bounded Go Code display.

No Go output is read. A small separately written tokenizer covers only the
frozen Go specimens below; fontTools supplies original Go Mono outlines and
metrics, an explicit rune/tab/CRLF grid supplies placement, and CairoSVG supplies
pixels. Animation references use independently calculated cubic prefixes and
control-point interpolation, not Pango, Pygments or Manim Code output.
"""
import argparse
import copy
import hashlib
import json
import math
from pathlib import Path
import platform
import re
import subprocess

import cairocffi
import cairosvg
import fontTools
from fontTools.pens.basePen import BasePen
from fontTools.pens.boundsPen import BoundsPen
from fontTools.ttLib import TTFont

FONT_SHA = "8bc66a0154bbf69cd24e5bde41a12ec9495c8a242c5def2255e4a164900f1ed7"
PALETTE = dict(keyword="c678dd", identifier="dcdfe4", number="d19a66", string="98c379",
               comment="7f8b99", operator="56b6c2", punctuation="abb2bf",
               line_number="7f8b99", background="1e222a")
KEYWORDS = set("break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var".split())
OPS = sorted("+ - * / % & | ^ << >> &^ += -= *= /= %= &= |= ^= <<= >>= &^= && || <- ++ -- == < > = ! != <= >= := ... ( ) [ ] { } , . ; : ~".split(), key=len, reverse=True)
NUM = re.compile(r"(?:0[xX][0-9a-fA-F_]+(?:\.[0-9a-fA-F_]*)?(?:[pP][+-]?[0-9_]+)?|0[bB][01_]+|0[oO][0-7_]+|(?:[0-9][0-9_]*(?:\.[0-9_]*)?|\.[0-9][0-9_]*)(?:[eE][+-]?[0-9_]+)?)(?:i)?")
IDENT = re.compile(r"(?:[^\W\d]|_)\w*", re.UNICODE)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def grid(source, tab, first):
    lines, cells = [], []
    offset, start, col, i = 0, 0, 0, 0
    while i < len(source):
        char = source[i]
        if char in "\r\n":
            n = 2 if source[i:i+2] == "\r\n" else 1
            lines.append(dict(index=len(lines), start=start, end=offset, columns=col, number=first+len(lines)))
            i += n
            offset += n
            start, col = offset, 0
            continue
        if char not in " \t":
            cells.append(dict(rune=ord(char), offset=offset, line=len(lines), column=col))
        col += tab-col % tab if char == "\t" else 1
        offset += len(char.encode())
        i += 1
    lines.append(dict(index=len(lines), start=start, end=offset, columns=col, number=first+len(lines)))
    return lines, cells


def tokenize(source, tab):
    """Independent specimen lexer, not a general replacement for go/scanner."""
    i, offset, line, col = 0, 0, 0, 0
    out = []
    while i < len(source):
        tail = source[i:]
        category = None
        if tail[0] in " \t\r\n":
            value = re.match(r"[ \t\r\n]+", tail)[0]
        elif tail.startswith("//"):
            value, category = tail.split("\n", 1)[0].removesuffix("\r"), "comment"
        elif tail.startswith("/*"):
            value, category = tail[:tail.index("*/")+2], "comment"
        elif tail[0] == "`":
            value, category = tail[:tail.index("`", 1)+1], "string"
        elif tail[0] in "\"'":
            quote = re.escape(tail[0])
            value = re.match(quote+r"(?:\\.|[^\\"+quote+r"\r\n])*"+quote, tail)[0]
            category = "string"
        elif match := NUM.match(tail):
            value, category = match[0], "number"
        elif match := IDENT.match(tail):
            value = match[0]
            category = "keyword" if value in KEYWORDS else "identifier"
        else:
            value = next(op for op in OPS if tail.startswith(op))
            category = "punctuation" if value in "()[]{},.;:" else "operator"
        if category:
            out.append(dict(index=len(out), start=offset, end=offset+len(value.encode()),
                            line=line, column=col, category=category, text=value))
        for char in value:
            if char == "\n":
                line, col = line+1, 0
            elif char == "\t":
                col += tab-col % tab
            elif char != "\r":
                col += 1
        i += len(value)
        offset += len(value.encode())
    return out


def lerp(a, b, t):
    return [x+(y-x)*t for x, y in zip(a, b)]


def line_curve(a, b):
    return [a, lerp(a, b, 1/3), lerp(a, b, 2/3), b]


class CubicPen(BasePen):
    def __init__(self, glyphset, factor, x, y):
        super().__init__(glyphset)
        self.factor, self.x, self.y = factor, x, y
        self.contours = []

    def point(self, p):
        return [self.x+p[0]*self.factor, self.y+p[1]*self.factor]

    def _moveTo(self, p):
        self.contours.append(dict(start=self.point(p), curves=[], closed=False))

    def current(self):
        c = self.contours[-1]
        return c["curves"][-1][-1] if c["curves"] else c["start"]

    def _lineTo(self, p):
        self.contours[-1]["curves"].append(line_curve(self.current(), self.point(p)))

    def _qCurveToOne(self, p1, p2):
        a, b, c = self.current(), self.point(p1), self.point(p2)
        self.contours[-1]["curves"].append([a, lerp(a, b, 2/3), lerp(c, b, 2/3), c])

    def _curveToOne(self, p1, p2, p3):
        self.contours[-1]["curves"].append([self.current(), self.point(p1), self.point(p2), self.point(p3)])

    def _closePath(self):
        c = self.contours[-1]
        if math.dist(self.current(), c["start"]) > 1e-12:
            c["curves"].append(line_curve(self.current(), c["start"]))
        c["closed"] = True

    def _endPath(self):
        pass


def split(curve, t):
    a, b, c = (lerp(curve[i], curve[i+1], t) for i in range(3))
    d, e = lerp(a, b, t), lerp(b, c, t)
    f = lerp(d, e, t)
    return [curve[0], a, d, f], [f, e, c, curve[3]]


def partial(contours, alpha):
    if alpha == 1:
        return copy.deepcopy(contours)
    left = alpha*sum(len(c["curves"]) for c in contours)
    result = []
    for c in contours:
        if left <= 0:
            break
        out = dict(start=c["start"], curves=[], closed=False)
        for curve in c["curves"]:
            if left >= 1:
                out["curves"].append(curve)
                left -= 1
            else:
                if left > 0:
                    out["curves"].append(split(curve, left)[0])
                left = 0
                break
        out["closed"] = c["closed"] and len(out["curves"]) == len(c["curves"]) and math.dist(out["curves"][-1][-1], c["start"]) < 1e-12
        result.append(out)
    return result


def align(a, b):
    a, b = copy.deepcopy(a), copy.deepcopy(b)
    def center(contours):
        # Only used when adding a missing contour; these specimens retain
        # contour counts. Bound center follows fontTools BoundsPen in layout.
        points = [p for c in contours for curve in c["curves"] for p in curve]
        return [(min(p[j] for p in points)+max(p[j] for p in points))/2 for j in range(2)]
    while len(a) < len(b):
        a.append(dict(start=center(a), curves=[], closed=False))
    while len(b) < len(a):
        b.append(dict(start=center(b), curves=[], closed=False))
    for ca, cb in zip(a, b):
        n = max(len(ca["curves"]), len(cb["curves"]))
        for c in (ca, cb):
            if n and not c["curves"]:
                c["curves"] = [line_curve(c["start"], c["start"])]
            while len(c["curves"]) < n:
                lengths = [sum(math.dist(p, q) for p, q in zip(curve, curve[1:])) for curve in c["curves"]]
                i = max(range(len(lengths)), key=lengths.__getitem__)
                c["curves"][i:i+1] = split(c["curves"][i], .5)
    return a, b


def interpolate(a, b, alpha):
    a, b = align(a, b)
    return [dict(start=lerp(ca["start"], cb["start"], alpha),
                 curves=[[lerp(pa, pb, alpha) for pa, pb in zip(a, b)] for a, b in zip(ca["curves"], cb["curves"])],
                 closed=cb["closed"] if alpha == 1 else ca["closed"] if alpha == 0 else ca["closed"] and cb["closed"])
            for ca, cb in zip(a, b)]


def svg_path(contours):
    result = []
    for c in contours:
        result.append("M"+" ".join(format(v, ".17g") for v in c["start"]))
        for curve in c["curves"]:
            result.append("C"+" ".join(format(v, ".17g") for p in curve[1:] for v in p))
        if c["closed"]:
            result.append("Z")
    return " ".join(result)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--font", type=Path, required=True)
    ap.add_argument("--freeze", action="store_true", help="Freeze candidate, central tests, documentation and independent references after generation")
    ap.add_argument("--manim", type=Path, help="Pinned reference repository; required for --freeze")
    args = ap.parse_args()
    if digest(args.font.read_bytes()) != FONT_SHA:
        raise ValueError("Go Mono source font differs")
    root = Path(__file__).resolve().parents[2]
    out = root/"tests/data/code"
    out.mkdir(parents=True, exist_ok=True)
    f = TTFont(args.font)
    upem, glyphset, cmap = f["head"].unitsPerEm, f.getGlyphSet(), f.getBestCmap()
    cell = f["hmtx"][cmap[32]][0]/upem
    ascent, descent = f["hhea"].ascent/upem, -f["hhea"].descent/upem
    raster = dict(width=1200, height=720, scale=80, origin=[180,130])

    def paint(name, paths):
        svg = ('<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="720">'
               '<rect width="1200" height="720" fill="black"/>'
               '<g transform="translate(180 130) scale(80 -80)">'
               +"".join(f'<path fill="#{p["color"]}" d="{svg_path(p["contours"])}"/>' for p in paths if p["contours"])
               +'</g></svg>\n').encode()
        png = cairosvg.svg2png(bytestring=svg)
        (out/(name+".svg")).write_bytes(svg)
        (out/(name+".png")).write_bytes(png)
        return dict(svg_sha256=digest(svg), png_sha256=digest(png))

    def layout(name, source, numbers=False, background=False, first=1, tab=4, size=.6):
        lines, cells = grid(source, tab, first)
        tokens = tokenize(source, tab)
        m = dict(cell_width=cell*size, line_advance=1.3*size, ascent=ascent*size, descent=descent*size,
                 lines=len(lines), columns=max(l["columns"] for l in lines))
        m["content_width"] = m["columns"]*m["cell_width"]
        digits = len(str(lines[-1]["number"])) if numbers else 0
        m.update(left=-.2-(digits+2)*m["cell_width"] if numbers else -.2,
                 right=m["content_width"]+.2, top=m["ascent"]+.2,
                 bottom=-(len(lines)-1)*m["line_advance"]-m["descent"]-.2)
        glyphs, number_glyphs, paths = [], [], []
        def glyph(rune, x, y, color, metadata):
            g = glyphset[cmap[ord(rune)]]
            assert f["hmtx"][cmap[ord(rune)]][0]/upem == cell
            pen, bounds = CubicPen(glyphset, size/upem, x, y), BoundsPen(glyphset)
            g.draw(pen); g.draw(bounds)
            metadata.update(rune=ord(rune), x=x, baseline=y, color=color, contours=pen.contours,
                            bbox=[x+bounds.bounds[0]*size/upem, y+bounds.bounds[1]*size/upem,
                                  x+bounds.bounds[2]*size/upem, y+bounds.bounds[3]*size/upem])
            return metadata
        for item in cells:
            tok = next(t for t in tokens if t["start"] <= item["offset"] < t["end"])
            item["token"] = tok["index"]
            glyphs.append(glyph(chr(item["rune"]), item["column"]*m["cell_width"], -item["line"]*m["line_advance"], PALETTE[tok["category"]], item))
        if background:
            a, b, c, d = [m["left"],m["bottom"]], [m["right"],m["bottom"]], [m["right"],m["top"]], [m["left"],m["top"]]
            paths.append(dict(color=PALETTE["background"], contours=[dict(start=a, closed=True, curves=[line_curve(a,b),line_curve(b,c),line_curve(c,d),line_curve(d,a)])]))
        if numbers:
            for line in lines:
                label = str(line["number"])
                for j, rune in enumerate(label):
                    number_glyphs.append(glyph(rune, (j-len(label)-2)*m["cell_width"], -line["index"]*m["line_advance"], PALETTE["line_number"], dict(line=line["index"])))
        paths += number_glyphs+glyphs
        m["glyphs"] = len(glyphs)+len(number_glyphs)
        m["curves"] = sum(len(c["curves"]) for p in paths for c in p["contours"])
        record = dict(name=name, source=source, numbers=numbers, background=background, first=first, tab=tab, size=size,
                      lines=lines, tokens=tokens, metrics=m, glyphs=glyphs, number_glyphs=number_glyphs)
        record.update(paint(name, paths))
        return record

    records = [
        layout("grid", "package demo\r\n\r\n// tabs stay exact\r\nfunc add(x int) int {\r\n\treturn x + 0x2A // αβ\r\n}\r\n", True, True, 98),
        layout("multiline", "//line imaginary.go:900\nvar α = `a\tb\r\nc` /* row\r\n\tend */\nα <<= 2; α++\n", True, True),
        layout("whitespace", "\t  \r\n \t\n", True, True, 9, 8),
        layout("tokens", 'var _ = []any{0b101, 0o17, 1_000,\n\t.5, 0x1.fp+2, 2.5e-3i,\n\t\'λ\', "a\\t"}\n'),
    ]
    a = layout("animation-source", "var n = 8\n")
    b = layout("animation-target", "var m = 3\n")
    # The target is an independently laid-out different listing; missing
    # contour insertion uses true ink bounds supplied above.
    frames = []
    for alpha in (0, .25, .5, .75, 1):
        for mode in ("write", "transform"):
            paths = []
            for ga, gb in zip(a["glyphs"], b["glyphs"]):
                if mode == "write":
                    contours = partial(ga["contours"], alpha)
                else:
                    ca, cb = copy.deepcopy(ga["contours"]), copy.deepcopy(gb["contours"])
                    # Independent bounds centers for degenerate missing contours.
                    for own, other, bounds in ((ca, cb, ga["bbox"]), (cb, ca, gb["bbox"])):
                        while len(own) < len(other):
                            own.append(dict(start=[(bounds[0]+bounds[2])/2,(bounds[1]+bounds[3])/2],curves=[],closed=False))
                    contours = interpolate(ca, cb, alpha)
                paths.append(dict(color=ga["color"], contours=contours))
            name = mode+"-"+str(round(alpha*100)).zfill(3)
            frame = dict(name=name, mode=mode, alpha=alpha, glyphs=paths)
            frame.update(paint(name, paths))
            frames.append(frame)
    records += [a,b]
    reference = dict(provenance=__doc__, font_sha256=FONT_SHA, palette=PALETTE, records=records, frames=frames, raster=raster,
                     generator_sha256=digest(Path(__file__).read_bytes()),
                     manim_source="16ea443e2a5f1d79db0a721161411ce916d6e733:manim/mobject/text/code_mobject.py",
                     versions=dict(Python=platform.python_version(), fontTools=fontTools.__version__, CairoSVG=cairosvg.__version__, Cairo=cairocffi.cairo_version_string()))
    (out/"reference.json").write_text(json.dumps(reference,indent=2,ensure_ascii=False)+"\n")
    # Development-only custom fixtures exercise proportional-font rejection,
    # missing glyphs, font parsing ownership and malformed lazy outline errors.
    broken = bytearray(args.font.read_bytes())
    gid = f.getGlyphID(cmap[ord("A")])
    offset = f.reader.tables["glyf"].offset+f["loca"].locations[gid]
    broken[offset:offset+2] = (32767).to_bytes(2,"big")
    (out/"broken.ttf").write_bytes(broken)
    (out/"FONT_LICENSE.txt").write_bytes((root/"LICENSES/go-fonts.txt").read_bytes())
    if args.freeze:
        if not args.manim:
            raise ValueError("--freeze requires --manim for source pin verification")
        commit = "16ea443e2a5f1d79db0a721161411ce916d6e733"
        manim_path = "manim/mobject/text/code_mobject.py"
        original = subprocess.check_output(["git", "-C", str(args.manim), "show", commit+":"+manim_path])
        go_version = subprocess.check_output(["go", "version"], text=True).strip()
        go_root = Path(subprocess.check_output(["go", "env", "GOROOT"], text=True).strip())
        production = root/"code/_staging" if (root/"code/_staging").exists() else root/"code"
        tests = root/"tests/_cases/code/_staging" if (root/"tests/_cases/code/_staging").exists() else root/"tests/_cases/code"
        inputs = [*production.glob("*.go"), *tests.glob("*_test.go"), root/"docs/CODE.md", Path(__file__),
                  *[p for p in out.iterdir() if p.is_file() and p.name != "manifest.json"]]
        files = {}
        for p in sorted(inputs):
            logical = str(p.relative_to(root)).replace("code/_staging/", "code/")
            files[logical] = digest(p.read_bytes())
        manifest = dict(schema=1, scope="wave8 bounded Go Code candidate; stage paths are mapped to their eventual promotion paths",
                        manim=dict(commit=commit, path=manim_path, sha256=digest(original)),
                        font=dict(path="golang.org/x/image@v0.47.0/font/gofont/ttfs/Go-Mono.ttf", sha256=FONT_SHA),
                        go=dict(version=go_version, scanner_sha256=digest((go_root/"src/go/scanner/scanner.go").read_bytes())),
                        files=files)
        (out/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n")
    print("Wrote six independent grid/token/outline references and ten interior/end animation frames.")


if __name__ == "__main__":
    main()
