package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"image"
	"image/color"
)

// Crossfade exercises different immutable bitmap assets, then bitmap/vector
// transitions and reintroduction of the restored first authoring object.
func Crossfade() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Crossfade / distinct assets / image and vector layers")
	if err != nil {
		return nil, err
	}
	assets := make([]*gomath.Object, 2)
	for i := range assets {
		pixels := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		for y := range 64 {
			for x := range 64 {
				c := color.NRGBA{R: 220, G: uint8(70 + 2*y), B: 70, A: 255}
				if i == 1 {
					c = color.NRGBA{R: 50, G: uint8(80 + 2*x), B: 235, A: 255}
				}
				if (x/8+y/8)%2 == 0 {
					c.A = 180
				}
				pixels.SetNRGBA(x, y, c)
			}
		}
		assets[i], err = rasterimage.New(pixels, 2.8-float64(i)*.7)
		if err != nil {
			return nil, err
		}
		assets[i].MoveTo(geom.V(-3+6*float64(i), 0))
	}
	polygon := gomath.RegularPolygon(5, 1.4).SetFill(gomath.Green).SetStroke(gomath.White, .04).MoveTo(geom.V(-3, 0))
	if err = s.Add(heading, assets[0]); err != nil {
		return nil, err
	}
	if err = s.Play(6, gomath.Succession(
		gomath.FadeTransform(assets[0], assets[1]),
		gomath.FadeTransform(assets[1], polygon),
		gomath.FadeTransform(polygon, assets[0]),
	)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
