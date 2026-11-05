package scenes

import (
	"context"
	"gomath/render"
	"image"
	"testing"
)

type discard struct{ frames int }

func (d *discard) WriteFrame(i int, _ *image.RGBA) error { d.frames++; return nil }
func (*discard) Close() error                            { return nil }
func (*discard) Abort() error                            { return nil }
func TestAllScenesRender(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			s, e := BuildSource(name)
			if e != nil {
				t.Fatal(e)
			}
			c := render.DefaultConfig()
			c.Width, c.Height, c.FPS, c.Workers = 160, 90, 4, 2
			sink := &discard{}
			if _, e = render.RenderScene(context.Background(), s, c, sink); e != nil {
				t.Fatal(e)
			}
			if sink.frames < 4 {
				t.Fatal("too few frames")
			}
		})
	}
}
