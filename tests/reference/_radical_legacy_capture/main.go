// Development-only capture of pre-indexed-root layout/pixels, not a TeX oracle.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/mathtex"
	"gomath/path"
	"gomath/render"
	"os"
	"strings"
)

type drawable struct {
	Path   path.Path
	Style  gomath.Style
	ZIndex int
}
type record struct{ Source, GeometrySHA256, PixelSHA256 string }

func main() {
	code, err := os.ReadFile("mathtex/mathtex.go")
	must(err)
	if strings.Contains(string(code), "indexedRadical") {
		panic("capture must use pre-wave6 production implementation; do not regenerate from the extension")
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 640, 320, 1, 8, gomath.Black
	r, err := render.NewRasterizer(c)
	must(err)
	records := []record{}
	for _, src := range []string{`\sqrt{g}`, `\sqrt{\frac{n}{g}}_i^n`, `\color{red}\sqrt{\mathit{g}+\mathbf{n}}`, `\left[\frac{\sqrt{g}}{\sqrt{\mathbb{R}}}\right]`, `\begin{matrix}\sqrt{g}&\hat{n}\\\sqrt{\sqrt{g}}&\displaystyle\sum_i^n\end{matrix}`} {
		o, err := mathtex.New(src, mathtex.DefaultOptions())
		must(err)
		d := []drawable{}
		o.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 {
				d = append(d, drawable{n.Path, n.Style, n.ZIndex})
			}
		})
		raw, err := json.Marshal(d)
		must(err)
		g := sha256.Sum256(raw)
		img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		must(err)
		p := sha256.Sum256(img.Pix)
		records = append(records, record{src, hex.EncodeToString(g[:]), hex.EncodeToString(p[:])})
	}
	manifest := struct {
		Provenance   string
		SourceSHA256 string
		Records      []record
	}{"Pre-wave6 library compatibility capture; not independent TeX/font layout evidence.", fmt.Sprintf("%x", sha256.Sum256(code)), records}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	must(err)
	must(os.WriteFile("tests/data/mathtex/radical_boxes/legacy.json", append(raw, '\n'), 0644))
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
