// Development-only Go review artifacts, not independent accuracy references.
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
	const source = `<span foreground="#ff4000">A<b>V<i>g</i></b><span foreground="#20c080">xβ</span>A</span>V` + "\n" + `<i>g</i>` + "\t&amp;"
	opt := text.DefaultMarkupOptions()
	opt.Size = 1
	o, metrics, err := text.LayoutMarkup(source, opt)
	if err != nil {
		return err
	}
	o.Shift(geom.V(-3.5, 1))
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 1024, 640, 1, 8, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		return err
	}
	frame := &gomath.Frame{Objects: []*gomath.Object{o}}
	img, err := r.Render(frame)
	if err != nil {
		return err
	}
	dir := filepath.Join("tests", "data", "text", "markup")
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
	fmt.Printf("Go markup preview: %s; %d ink glyphs; metrics %+v\n", dir, len(o.Children), metrics)
	return nil
}
