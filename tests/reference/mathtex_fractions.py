#!/usr/bin/env python3
"""Development-only source-font metrics/unhinted FreeType operand references.

No gomath is imported. These establish operand outlines at the bounded subset's
documented scales; they are not TeX whole-formula placement goldens.
"""
import argparse
import hashlib
import json
from pathlib import Path

import fontTools
from fontTools.pens.boundsPen import BoundsPen
from fontTools.ttLib import TTFont
import freetype
from PIL import Image, __version__ as pillow_version

FILES = ["Go-Regular.ttf", "Go-Bold.ttf", "Go-Italic.ttf", "Go-Bold-Italic.ttf"]
CONTEXTS = [("dfrac", 1), ("tfrac", .7), ("substack", .65), ("script-binom", .65*.7)]


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--gofont-dir", required=True, type=Path)
    ap.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    args = ap.parse_args()
    data_dir = args.root / "tests" / "data" / "mathtex" / "fractions"
    data_dir.mkdir(parents=True, exist_ok=True)
    records = []
    for style, filename in enumerate(FILES):
        source = args.gofont_dir / "ttfs" / filename
        f = TTFont(source)
        units = f["head"].unitsPerEm
        face = freetype.Face(str(source))
        glyphs = f.getGlyphSet()
        meta = {"file": filename, "sha256": hashlib.sha256(source.read_bytes()).hexdigest(),
                "units_per_em": units, "versions": sorted({n.toUnicode() for n in f["name"].names if n.nameID == 5})}
        for rune in "ngk":
            name = f.getBestCmap()[ord(rune)]
            bounds = BoundsPen(glyphs)
            glyphs[name].draw(bounds)
            for context, ratio in CONTEXTS:
                face.set_char_size(0, round(256 * 64 * ratio), 72, 72)
                face.load_char(rune, freetype.FT_LOAD_RENDER | freetype.FT_LOAD_NO_HINTING | freetype.FT_LOAD_NO_BITMAP)
                slot = face.glyph
                bitmap = slot.bitmap
                if bitmap.pixel_mode != freetype.FT_PIXEL_MODE_GRAY or bitmap.pitch < bitmap.width:
                    raise ValueError("unexpected source bitmap format")
                image = Image.new("L", (384, 384))
                mask = Image.frombytes("L", (bitmap.width, bitmap.rows), bytes(bitmap.buffer), "raw", "L", bitmap.pitch)
                origin = (48 + slot.bitmap_left, 288 - slot.bitmap_top)
                if min(origin) < 0 or origin[0]+mask.width > 384 or origin[1]+mask.height > 384:
                    raise ValueError("source glyph clips canvas")
                image.paste(mask, origin)
                raster = f"{context}-{style}-{rune}.png"
                image.save(data_dir / raster)
                records.append({"context": context, "style": style, "rune": rune,
                                "ratio": ratio, "raster": raster, "font": meta, "glyph": name,
                                "bbox_em": [v/units for v in bounds.bounds],
                                "advance_em": f["hmtx"].metrics[name][0]/units,
                                "outline_origin_x_em": (f["glyf"][name].xMin-f["hmtx"].metrics[name][1])/units})
    manifest = {"provenance": "Source Go-font hmtx/glyf metrics via fontTools; source outlines rasterized directly by FreeType. No gomath/TeX layout output.",
                "fonttools": fontTools.__version__, "freetype": ".".join(map(str, freetype.version())),
                "pillow": pillow_version, "load_flags": "FT_LOAD_RENDER | FT_LOAD_NO_HINTING | FT_LOAD_NO_BITMAP",
                "raster": {"width": 384, "height": 384, "ppem": 256, "origin_x": 48, "baseline_y": 288},
                "records": records}
    (data_dir / "reference.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+"\n")


if __name__ == "__main__":
    main()
