package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"image"
	"image/color"
	"math"
)

// Clipping exercises real bitmap/vector clipping, a nonzero hole, nested clip
// intersections and a changing clip outline on an immutable bitmap asset.
func Clipping() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Vector clips / bitmap masks / nested intersections")
	if err != nil {
		return nil, err
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := range 96 {
		for x := range 96 {
			c := color.NRGBA{R: uint8(40 + 2*x), G: uint8(60 + y), B: 225, A: 255}
			if (x/12+y/12)%2 == 0 {
				c.R, c.G = 240, uint8(90+y)
			}
			pixels.SetNRGBA(x, y, c)
		}
	}
	picture, err := rasterimage.New(pixels, 4)
	if err != nil {
		return nil, err
	}
	picture.Shift(geom.V(-3.1, 0))
	ring, err := gomath.Annulus(.65, 1.65)
	if err != nil {
		return nil, err
	}
	clip, err := gomath.NewClip(ring.Shift(geom.V(-3.1, 0)).Path)
	if err != nil {
		return nil, err
	}
	picture.SetClip(clip)
	var bands []*gomath.Object
	for i := range 7 {
		c := gomath.RGB(.15+.1*float64(i), .65-.06*float64(i), .9-.08*float64(i))
		bands = append(bands, gomath.Rectangle(4, .52).SetFill(c).SetStroke(gomath.Transparent, 0).Shift(geom.V(3.1, -1.56+.52*float64(i))))
	}
	group := gomath.Group(bands...)
	outer, err := gomath.NewClip(gomath.RegularPolygon(6, 1.8).Shift(geom.V(3.1, 0)).Path)
	if err != nil {
		return nil, err
	}
	inner, err := gomath.NewClip(gomath.Rectangle(3.6, 2.5).Shift(geom.V(3.1, 0)).Path)
	if err != nil {
		return nil, err
	}
	group.SetClip(inner)
	nested := gomath.Group(group).SetClip(outer)
	if err = s.Add(heading, picture, nested); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	targetClip, err := clip.Transform(geom.About(geom.V(-3.1, 0), geom.RotationZ(.5).Mul(geom.Scaling(1, .65, 1))))
	if err != nil {
		return nil, err
	}
	if err = s.Play(3, gomath.Transform(picture, picture.Copy().SetClip(targetClip)), gomath.Rotate(group, math.Pi/3)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
