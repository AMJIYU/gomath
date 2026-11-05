package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/text"
	"image"
	"image/color"
)

func ImagePixels() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Immutable pixels / invert, tint and saved-alpha restoration")
	if err != nil {
		return nil, err
	}
	source := image.NewNRGBA(image.Rect(0, 0, 96, 96))
	for y := range 96 {
		for x := range 96 {
			alpha := uint8(255)
			if (x/12+y/12)%2 == 0 {
				alpha = 90
			}
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / 95), G: uint8(y * 255 / 95), B: uint8((x + y) * 255 / 190), A: alpha})
		}
	}
	asset, err := rasterimage.NewAsset(source)
	if err != nil {
		return nil, err
	}
	inverted, err := asset.Invert()
	if err != nil {
		return nil, err
	}
	tinted, err := asset.Tint(gomath.RGB(.35, .8, 1))
	if err != nil {
		return nil, err
	}
	faded, err := asset.SetOpacity(.2)
	if err != nil {
		return nil, err
	}
	restored, err := faded.SetOpacity(1)
	if err != nil {
		return nil, err
	}
	options := text.DefaultOptions()
	options.Size = .3
	var panels []*gomath.Object
	var images []*gomath.Object
	for i, item := range []*rasterimage.Asset{asset, inverted, tinted, faded} {
		picture, err := item.NewImage(2.45)
		if err != nil {
			return nil, err
		}
		x := -4.8 + 3.2*float64(i)
		picture.MoveTo(geom.V(x, .3))
		label, err := text.New([]string{"original alpha", "255 - RGB", "RGB tint", "opacity .2 -> 1"}[i], text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(x, -1.5))
		images = append(images, picture)
		panels = append(panels, gomath.Group(picture, label))
	}
	caption, err := text.New("Original asset remains intact / pixel edits create new immutable assets", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.5))
	if err = scene.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = scene.Play(1.2, gomath.FadeIn(panels[0]), gomath.FadeIn(panels[1]), gomath.FadeIn(panels[2]), gomath.FadeIn(panels[3])); err != nil {
		return nil, err
	}
	replacement, err := restored.WithPixels(images[3])
	if err != nil {
		return nil, err
	}
	if err = scene.Play(1.8, gomath.FadeTransform(images[3], replacement)); err != nil {
		return nil, err
	}
	if err = scene.Wait(.5); err != nil {
		return nil, err
	}
	return scene, nil
}
