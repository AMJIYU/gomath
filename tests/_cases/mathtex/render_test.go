package mathtex_test

import (
	"bytes"
	"encoding/xml"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/render"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are integration examples of our bounded subset, not TeX/Manim goldens.
// An optional preview directory preserves PNG/SVG pairs for visual inspection;
// ordinary test runs create temporary files and need no external renderer.
func TestTypesetPNGAndVectorExamples(t *testing.T) {
	dir := t.TempDir()
	if preview := os.Getenv("GOMATH_TYPESET_PREVIEW_DIR"); preview != "" {
		dir = preview
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	examples := []struct{ name, source string }{
		{"multiline-aligned", `\left\{\begin{aligned}\color{red}f(x)&=\dfrac{\textcolor{blue}{\hat{n}}}{g}&A&=\mathbb{R}\\\sum_{\substack{i=1\\j=2}}^n i&=\binom{n}{k}&B&=\textcolor{violet}{\mathcal{A}}\end{aligned}\right.`},
		{"multiline-gathered", `\begin{gathered}\color{red}f(x)=\dfrac{\hat{n}}{g}\\\sum_{\substack{i=1\\j=2}}^n i=\binom{n}{k}\\\textcolor{violet}{\mathbb{R}=\mathcal{A}}\end{gathered}`},
		{"delimiters", `f(x)=\left[\frac{\mathbf{x}_{i}^{2}+\mathit{y}}{\sqrt{1+\frac{a}{b}}}\right]_{i=1}^{n}+\left.\frac{d}{dx}\right|_{x=0}`},
		{"nested-matrices", `\begin{pmatrix}\mathbf{a}&\frac{1}{1+x}\\\sqrt{y}&\begin{bmatrix}1&0\\0&1\end{bmatrix}\end{pmatrix}\begin{Bmatrix}u\\v\end{Bmatrix}=\begin{Vmatrix}a&b\\c&d\end{Vmatrix}`},
		{"cases-and-fonts", `f(x)=\begin{cases}\mathit{x}^{2}&\text{  if }x<0\\\frac{\mathbf{x}}{1+x}&\text{  otherwise  }\end{cases}+\left\langle\begin{matrix}\mathbf{\mathit{A}}&\textbf{B}\\\textit{c}&d\end{matrix}\right\rangle`},
		{"operators", `\left[\begin{matrix}\mathbf{a}&b\\c&\mathit{d}\end{matrix}\right]\quad\int_{0}^{1}f(x)dx+\sum_{i=1}^{n}i+\prod_{j=1}^{m}x_j`},
		{"accents", `\overline{x+y}+\underline{y_i}+\hat{x}+\widehat{xyz}+\bar{z}+\vec{v}+\dot{x}+\ddot{y}+\widehat{\overline{x}_{i}}^{2}`},
		{"operator-limits", `\displaystyle\sum_{i=1}^{n}x_i+\prod_{j=1}^{m}y_j+\lim_{x\rightarrow0}f(x)+\int\limits_0^1g(x)dx+\textstyle\sum\limits_{k=1}^{\hat{p}}\frac{1}{k}`},
		{"named-operators-stacks", `\operatorname{rank}(A)+\displaystyle\operatorname*{arg max}_{x}^{\hat{n}}f(x)+a\stackrel{\operatorname{def}}{=}b+\overset{\hat{n}}{\underset{g}{x}}+\operatorname{Éλ2}_i^2`},
		{"operator-membership", `\displaystyle\operatorname*{arg max}_{x\in R}\overset{\hat{n}}{f}(x)`},
		{"set-symbols", `\begin{matrix}x\in A&x\notin B&A\ni x&A\subset B\\A\supset B&A\subseteq B&A\supseteq B&A\cup B\\A\cap B&\emptyset&A\setminus B&A\subsetneq B\\A\supsetneq B&A\nsubseteq B&A\nsupseteq B&\mathbf{A\subseteq B}\end{matrix}`},
		{"math-alphabets", `\begin{matrix}\mathbb{ABCDEFGHIJKLMNOPQRSTUVWXYZ}\\\mathbb{abcdefghijklmnopqrstuvwxyz0123456789}\\\mathcal{ABCDEFGHIJKLMNOPQRSTUVWXYZ}\\\mathcal{abcdefghijklmnopqrstuvwxyz}\\\mathfrak{ABCDEFGHIJKLMNOPQRSTUVWXYZ}\\\mathfrak{abcdefghijklmnopqrstuvwxyz}\\\displaystyle\operatorname*{arg max}_{x\in\mathbb{R}}\overset{\mathcal{A}}{\mathfrak{f}}(x)+\mathbb{C}^{2}\end{matrix}`},
		{"fractions-binomials", `\begin{matrix}\dfrac{g}{n}+\tfrac{g}{n}&\dbinom{n}{k}+\tbinom{n}{k}\\\displaystyle\binom{\hat{n}}{\tfrac{a}{g}}&x^{\binom{n}{k}}+x^{\dbinom{n}{k}}\\\displaystyle\sum_{\substack{i=1\\j\in\mathbb{N}}}^{n}\dfrac{\binom{n}{i}}{1+i}&\dfrac{\mathbb{R}}{\tfrac{g}{k}}\end{matrix}`},
	}
	for _, example := range examples {
		t.Run(example.name, func(t *testing.T) {
			opt := mathtex.DefaultOptions()
			opt.Color = gomath.Blue
			object, err := mathtex.New(example.source, opt)
			if err != nil {
				t.Fatal(err)
			}
			if err := object.Validate(); err != nil {
				t.Fatal(err)
			}
			object.MoveTo(geom.V(0, 0))
			bounds := object.Bounds()
			c := render.DefaultConfig()
			c.Width, c.Height, c.Workers, c.Background = 960, 480, 1, gomath.Black
			c.FrameWidth = math.Max(bounds.Size().X*1.2, bounds.Size().Y*float64(c.Width)/float64(c.Height)*1.25)
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			frame := &gomath.Frame{Objects: []*gomath.Object{object}}
			raster, err := r.Render(frame)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for y := range c.Height {
				for x := range c.Width {
					pixel := raster.RGBAAt(x, y)
					if pixel.R > 0 || pixel.G > 0 || pixel.B > 0 {
						count++
						if x < 3 || y < 3 || x >= c.Width-3 || y >= c.Height-3 {
							t.Fatal("typeset content clipped by viewport")
						}
					}
				}
			}
			if count < 500 {
				t.Fatal("typesetting produced insufficient visible ink", count)
			}
			pngPath, svgPath := filepath.Join(dir, example.name+".png"), filepath.Join(dir, example.name+".svg")
			if err := export.PNG(pngPath, raster); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(pngPath)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds() != raster.Bounds() {
				t.Fatal("PNG dimensions lost")
			}
			// The decoded examples preserve actual rendered pixels, not just a header.
			for y := range c.Height {
				for x := range c.Width {
					a, b, d, e := raster.At(x, y).RGBA()
					w, z, h, k := decoded.At(x, y).RGBA()
					if a != w || b != z || d != h || e != k {
						t.Fatal("PNG pixel roundtrip differs")
					}
				}
			}
			if err := export.SVG(svgPath, frame, c); err != nil {
				t.Fatal(err)
			}
			vector, err := os.ReadFile(svgPath)
			if err != nil {
				t.Fatal(err)
			}
			paths := 0
			decoder := xml.NewDecoder(bytes.NewReader(vector))
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				start, ok := token.(xml.StartElement)
				if !ok {
					continue
				}
				if start.Name.Local == "text" || start.Name.Local == "image" {
					t.Fatal("glyph outlines rasterized or delegated to external fonts")
				}
				if start.Name.Local == "path" {
					for _, attr := range start.Attr {
						if attr.Name.Local == "d" && strings.Contains(attr.Value, "C") {
							paths++
						}
					}
				}
			}
			leaves := 0
			object.Walk(func(o *gomath.Object) {
				if len(o.Path.Contours) > 0 {
					leaves++
				}
			})
			if paths != leaves || paths < 10 {
				t.Fatal("SVG lost drawable vector geometry", paths, leaves)
			}
			t.Logf("%s: %d ink pixels; %d vector paths; %s", example.name, count, paths, pngPath)
		})
	}
}
