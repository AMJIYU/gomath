// Development-only preview writer. These Go-produced images are review
// artifacts, not independent accuracy oracles; see text_fallback.py for those.
package main

import (
	"fmt"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	if err := preview(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func preview() error {
	dir := filepath.Join("tests", "data", "text", "fallback")
	fonts := make([]*text.Font, 2)
	for i, name := range []string{"primary.ttf", "secondary.ttf"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		fonts[i], err = text.Parse(data)
		if err != nil {
			return err
		}
	}
	opt := text.DefaultOptions()
	opt.Size = 1
	o, metrics, err := text.LayoutWithFallback("AV βgA\n\tVx ", fonts, opt)
	if err != nil {
		return err
	}
	o.Shift(geom.V(-2.5, .5))
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 768, 512, 1, 6, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		return err
	}
	frame := &gomath.Frame{Objects: []*gomath.Object{o}}
	img, err := r.Render(frame)
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "preview.png"))
	if err != nil {
		return err
	}
	encodeErr := png.Encode(f, img)
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := export.SVG(filepath.Join(dir, "preview.svg"), frame, c); err != nil {
		return err
	}
	fmt.Printf("Go preview: %s; %d ink glyphs; metrics %+v\n", dir, len(o.Children), metrics)
	return nil
}
