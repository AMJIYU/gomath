package three

import (
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"
)

func testTexture(t *testing.T, width, height int, pixel func(x, y int) color.NRGBA) *Texture {
	t.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			src.SetNRGBA(x, y, pixel(x, y))
		}
	}
	texture, err := NewTexture(src)
	if err != nil {
		t.Fatal(err)
	}
	return texture
}

func TestTextureImmutableImageCopy(t *testing.T) {
	parent := image.NewNRGBA(image.Rect(-10, 20, 10, 30))
	parent.SetNRGBA(-4, 23, color.NRGBA{R: 201, G: 101, B: 53, A: 128})
	parent.SetNRGBA(-3, 23, color.NRGBA{G: 255, A: 255})
	source := parent.SubImage(image.Rect(-4, 23, -2, 24))
	texture, err := NewTexture(source)
	if err != nil {
		t.Fatal(err)
	}
	if texture.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("nonzero bounds not normalized: %v", texture.Bounds())
	}
	clear(parent.Pix)
	want := color.NRGBA{R: 201, G: 101, B: 53, A: 128}
	for range 2 {
		got, err := texture.Sample(UV{U: .25, V: .5}, Sampler{})
		if err != nil || got != want {
			t.Fatalf("source mutation reached texture: %v err=%v want=%v", got, err, want)
		}
		got.R = 0
		bounds := texture.Bounds()
		bounds.Max.X = 0
	}
	// Conversion from a premultiplied source must recover straight texel RGB.
	rgba := image.NewRGBA(image.Rect(2, 3, 3, 4))
	rgba.Set(2, 3, color.NRGBA{R: 255, A: 128})
	texture, err = NewTexture(rgba)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := texture.Sample(UV{}, Sampler{}); err != nil || got != (color.NRGBA{R: 255, A: 128}) {
		t.Fatalf("premultiplied source conversion: %v %v", got, err)
	}
	wide := image.NewNRGBA64(image.Rect(0, 0, 1, 1))
	wide.SetNRGBA64(0, 0, color.NRGBA64{R: 0xABCD, G: 0x7890, B: 0x1234, A: 0xFFFF})
	texture, err = NewTexture(wide)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := texture.Sample(UV{}, Sampler{}); err != nil || got != (color.NRGBA{R: 0xAB, G: 0x78, B: 0x12, A: 255}) {
		t.Fatalf("16-bit source quantization: %v %v", got, err)
	}
}

type invalidTextureImage struct{ bounds image.Rectangle }

func (s invalidTextureImage) Bounds() image.Rectangle { return s.bounds }
func (invalidTextureImage) ColorModel() color.Model   { return color.NRGBAModel }
func (invalidTextureImage) At(x, y int) color.Color {
	panic("invalid texture bounds must be rejected before accessing any texels")
}

func TestTextureAndUVValidation(t *testing.T) {
	var nilImage *image.NRGBA
	maxInt := int(^uint(0) >> 1)
	for _, source := range []image.Image{
		nil, nilImage,
		invalidTextureImage{image.Rectangle{}},
		invalidTextureImage{image.Rectangle{Min: image.Pt(1, 0), Max: image.Pt(0, 1)}},
		invalidTextureImage{image.Rect(0, 0, MaxTextureDimension+1, 1)},
		invalidTextureImage{image.Rect(0, 0, 4096, 1025)},
		invalidTextureImage{image.Rectangle{Min: image.Pt(-maxInt, 0), Max: image.Pt(maxInt, 1)}},
	} {
		if texture, err := NewTexture(source); texture != nil || err == nil {
			t.Fatalf("invalid source accepted: %T %v", source, err)
		}
	}
	texture := testTexture(t, 1, 1, func(x, y int) color.NRGBA { return color.NRGBA{A: 255} })
	for _, uv := range []UV{{U: math.NaN()}, {V: math.Inf(1)}, {U: MaxUVCoordinate + 1}, {V: -MaxUVCoordinate - 1}} {
		if _, err := texture.Sample(uv, Sampler{}); err == nil {
			t.Fatalf("invalid sample UV accepted: %v", uv)
		}
		m := triangleMesh()
		m.UVs = []UV{{}, uv, {}}
		if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "UV") {
			t.Fatalf("invalid vertex UV accepted: %v", err)
		}
	}
	for _, s := range []Sampler{{Filter: 2}, {AddressU: 2}, {AddressV: 255}} {
		if _, err := texture.Sample(UV{}, s); err == nil {
			t.Fatalf("invalid sampler accepted: %+v", s)
		}
		in := NewInstance(triangleMesh())
		in.Material.Sampler = s
		if img, err := ambientRenderer(t).Render(testCamera(Perspective), in); err == nil || img != nil {
			t.Fatalf("render ignored invalid sampler: %v", err)
		}
	}
	for _, invalid := range []*Texture{nil, {}} {
		if _, err := invalid.Sample(UV{}, Sampler{}); err == nil {
			t.Fatal("invalid/zero texture sampled")
		}
	}
	for _, uvs := range [][]UV{nil, {{}, {}}, {{}, {}, {}, {}}} {
		m := triangleMesh()
		m.UVs = uvs
		if len(uvs) != 0 && m.Validate() == nil {
			t.Fatal("UV count mismatch accepted")
		}
		in := NewInstance(m)
		in.Material.Texture = texture
		if img, err := ambientRenderer(t).Render(testCamera(Perspective), in); err == nil || img != nil {
			t.Fatalf("textured mesh without complete UVs accepted: %v", err)
		}
	}
	in := NewInstance(triangleMesh())
	in.Mesh.UVs = make([]UV, len(in.Mesh.Vertices))
	in.Material.Texture = &Texture{}
	if img, err := ambientRenderer(t).Render(testCamera(Perspective), in); err == nil || img != nil {
		t.Fatalf("zero-value texture rendered: %v", err)
	}
}

func TestTextureSamplerTexelCentersAndAddressing(t *testing.T) {
	texels := [2][2]color.NRGBA{{{R: 255, A: 255}, {G: 255, A: 255}}, {{B: 255, A: 255}, {R: 255, G: 255, B: 255, A: 255}}}
	texture := testTexture(t, 2, 2, func(x, y int) color.NRGBA { return texels[y][x] })
	for _, tc := range []struct {
		uv   UV
		s    Sampler
		want color.NRGBA
	}{
		{UV{}, Sampler{}, texels[0][0]},
		{UV{U: 1, V: 1}, Sampler{}, texels[1][1]},
		{UV{U: -MaxUVCoordinate, V: MaxUVCoordinate}, Sampler{}, texels[1][0]},
		{UV{U: .5}, Sampler{}, texels[0][1]},
		{UV{U: -.25, V: .25}, Sampler{AddressU: Repeat}, texels[0][1]},
		{UV{U: 1, V: .25}, Sampler{AddressU: Repeat}, texels[0][0]},
		{UV{U: MaxUVCoordinate, V: -MaxUVCoordinate}, Sampler{AddressU: Repeat, AddressV: Repeat}, texels[0][0]},
		{UV{U: .25, V: .25}, Sampler{Filter: Bilinear}, texels[0][0]},
		{UV{U: .75, V: .75}, Sampler{Filter: Bilinear}, texels[1][1]},
		{UV{U: .5, V: .5}, Sampler{Filter: Bilinear}, color.NRGBA{R: 128, G: 128, B: 128, A: 255}},
		{UV{U: 0, V: .25}, Sampler{Filter: Bilinear}, texels[0][0]},
		{UV{U: 0, V: .25}, Sampler{Filter: Bilinear, AddressU: Repeat}, color.NRGBA{R: 128, G: 128, A: 255}},
		{UV{U: 1, V: .25}, Sampler{Filter: Bilinear, AddressU: Repeat}, color.NRGBA{R: 128, G: 128, A: 255}},
		{UV{U: .25, V: 0}, Sampler{Filter: Bilinear, AddressV: Repeat}, color.NRGBA{R: 128, B: 128, A: 255}},
	} {
		if got, err := texture.Sample(tc.uv, tc.s); err != nil || got != tc.want {
			t.Fatalf("uv=%v sampler=%+v: %v err=%v want=%v", tc.uv, tc.s, got, err, tc.want)
		}
	}
}

func TestTextureBilinearPremultipliedNoTransparentRGBFringe(t *testing.T) {
	texture := testTexture(t, 2, 1, func(x, y int) color.NRGBA {
		if x == 0 {
			return color.NRGBA{R: 255, A: 255}
		}
		return color.NRGBA{G: 255}
	})
	for _, tc := range []struct {
		u    float64
		want color.NRGBA
	}{
		{.25, color.NRGBA{R: 255, A: 255}},
		{.375, color.NRGBA{R: 255, A: 191}},
		{.5, color.NRGBA{R: 255, A: 128}},
		{.625, color.NRGBA{R: 255, A: 64}},
		{.75, color.NRGBA{}},
	} {
		if got, err := texture.Sample(UV{U: tc.u, V: .5}, Sampler{Filter: Bilinear}); err != nil || got != tc.want {
			t.Fatalf("premultiplied filter u=%g: %v err=%v want=%v", tc.u, got, err, tc.want)
		}
	}
	// One-texel images work for every filter/address combination and large UVs.
	one := testTexture(t, 1, 1, func(x, y int) color.NRGBA { return color.NRGBA{R: 201, G: 101, B: 53, A: 128} })
	for _, filter := range []TextureFilter{Nearest, Bilinear} {
		for _, address := range []TextureAddress{Clamp, Repeat} {
			if got, err := one.Sample(UV{U: -MaxUVCoordinate, V: MaxUVCoordinate}, Sampler{Filter: filter, AddressU: address, AddressV: address}); err != nil || got != (color.NRGBA{R: 201, G: 101, B: 53, A: 128}) {
				t.Fatalf("one texel: %v %v", got, err)
			}
		}
	}
}

func TestTexturedFragmentStorageBound(t *testing.T) {
	if reflect.TypeFor[texturedFragment]().Size() != 24 {
		t.Fatal("textured record no longer has the documented 24-byte payload")
	}
	b := &texturedFragmentBuffer{limit: transparentBlockSize + 3}
	for i := range b.limit {
		if err := b.add(0, float64(i), 0xFF0000, 1.0/255); err != nil {
			t.Fatal(err)
		}
	}
	allocated := 0
	for _, block := range b.blocks {
		allocated += cap(block)
	}
	if allocated != b.limit || b.add(0, 0, 0, .5) == nil || b.count != b.limit {
		t.Fatalf("textured fragment memory/count limit: allocated=%d count=%d limit=%d", allocated, b.count, b.limit)
	}
}
