package three

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"reflect"
)

const (
	MaxTextureDimension = 4096
	MaxTexturePixels    = 4 * 1024 * 1024
)

type TextureFilter uint8

const (
	Nearest TextureFilter = iota
	Bilinear
)

type TextureAddress uint8

const (
	Clamp TextureAddress = iota
	Repeat
)

// Sampler's zero value selects nearest filtering and clamping on both axes.
// Bilinear interpolation operates on premultiplied sRGB and alpha, preventing
// RGB in transparent texels from bleeding into visible filtered colors.
type Sampler struct {
	Filter             TextureFilter
	AddressU, AddressV TextureAddress
}

func (s Sampler) validate() error {
	if s.Filter != Nearest && s.Filter != Bilinear {
		return fmt.Errorf("three: texture filter must be Nearest or Bilinear")
	}
	if (s.AddressU != Clamp && s.AddressU != Repeat) || (s.AddressV != Clamp && s.AddressV != Repeat) {
		return fmt.Errorf("three: texture addressing must be Clamp or Repeat on each axis")
	}
	return nil
}

// Texture owns an immutable, tightly packed 8-bit NRGBA copy of its source.
// The source image and its backing memory are never retained. A Texture can be
// shared across renderers/goroutines; its zero value is invalid for rendering.
type Texture struct {
	width, height int
	pixels        []color.NRGBA
	opaque        bool
}

// NewTexture copies src.At over its bounds, including nonzero image origins and
// subimages. Bounds are normalized to (0,0). Each axis is at most 4096, with at
// most 4,194,304 texels (16 MiB pixel payload). Source pixels convert using
// color.NRGBAModel; high-bit-depth sources quantize to 8 bits at construction.
// The caller must not mutate src concurrently with this synchronous copy.
func NewTexture(src image.Image) (*Texture, error) {
	if src == nil || (reflect.ValueOf(src).Kind() == reflect.Pointer && reflect.ValueOf(src).IsNil()) {
		return nil, fmt.Errorf("three: texture source must be nonnil")
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if bounds.Max.X <= bounds.Min.X || bounds.Max.Y <= bounds.Min.Y || w <= 0 || h <= 0 || w > MaxTextureDimension || h > MaxTextureDimension || w > MaxTexturePixels/h {
		return nil, fmt.Errorf("three: texture dimensions must be 1..%d and at most %d texels", MaxTextureDimension, MaxTexturePixels)
	}
	t := &Texture{width: w, height: h, pixels: make([]color.NRGBA, w*h), opaque: true}
	for y := range h {
		for x := range w {
			c := color.NRGBAModel.Convert(src.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.NRGBA)
			t.pixels[y*w+x] = c
			t.opaque = t.opaque && c.A == 255
		}
	}
	return t, nil
}

// Bounds returns normalized bounds by value, without exposing pixel memory.
func (t *Texture) Bounds() image.Rectangle {
	if t == nil {
		return image.Rectangle{}
	}
	return image.Rect(0, 0, t.width, t.height)
}

func (t *Texture) valid() bool {
	return t != nil && t.width > 0 && t.height > 0 && len(t.pixels) == t.width*t.height
}

func validUV(uv UV) bool {
	return finite(uv.U) && finite(uv.V) && math.Abs(uv.U) <= MaxUVCoordinate && math.Abs(uv.V) <= MaxUVCoordinate
}

// Sample returns an unassociated 8-bit sample. Render retains filtered alpha
// and material-alpha products in float64 until compositing instead of using
// this quantized public result. Invalid textures, UVs or samplers return errors.
func (t *Texture) Sample(uv UV, s Sampler) (color.NRGBA, error) {
	if !t.valid() {
		return color.NRGBA{}, fmt.Errorf("three: invalid texture")
	}
	if !validUV(uv) {
		return color.NRGBA{}, fmt.Errorf("three: texture sample UV must be finite with components in [-%g,%g]", MaxUVCoordinate, MaxUVCoordinate)
	}
	if err := s.validate(); err != nil {
		return color.NRGBA{}, err
	}
	v := t.sample(uv, s)
	if v.a == 0 {
		return color.NRGBA{}, nil
	}
	return color.NRGBA{R: textureByte(v.r / v.a), G: textureByte(v.g / v.a), B: textureByte(v.b / v.a), A: textureByte(v.a * 255)}, nil
}

func textureByte(x float64) uint8 { return uint8(math.Round(max(0, min(255, x)))) }

// RGB is premultiplied in byte units, alpha is in [0,1].
type textureSample struct{ r, g, b, a float64 }

func (a textureSample) lerp(b textureSample, t float64) textureSample {
	return textureSample{a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t, a.a + (b.a-a.a)*t}
}

func (t *Texture) texel(x, y int) textureSample {
	c := t.pixels[y*t.width+x]
	a := float64(c.A) / 255
	return textureSample{float64(c.R) * a, float64(c.G) * a, float64(c.B) * a, a}
}

func addressCoordinate(x float64, mode TextureAddress) float64 {
	if mode == Repeat {
		return x - math.Floor(x)
	}
	return max(0, min(1, x))
}

func addressTexel(i, size int, mode TextureAddress) int {
	if mode == Repeat {
		// Coordinates have already been reduced to one period. Bilinear taps
		// can be only -1..size; modulo is bounded and handles negative taps.
		i %= size
		if i < 0 {
			i += size
		}
		return i
	}
	return max(0, min(size-1, i))
}

func (t *Texture) sample(uv UV, s Sampler) textureSample {
	u, v := addressCoordinate(uv.U, s.AddressU), addressCoordinate(uv.V, s.AddressV)
	if s.Filter == Nearest {
		x := min(t.width-1, int(math.Floor(u*float64(t.width))))
		y := min(t.height-1, int(math.Floor(v*float64(t.height))))
		return t.texel(x, y)
	}
	x, y := u*float64(t.width)-.5, v*float64(t.height)-.5
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	fx, fy := x-float64(x0), y-float64(y0)
	ax, bx := addressTexel(x0, t.width, s.AddressU), addressTexel(x0+1, t.width, s.AddressU)
	ay, by := addressTexel(y0, t.height, s.AddressV), addressTexel(y0+1, t.height, s.AddressV)
	return t.texel(ax, ay).lerp(t.texel(bx, ay), fx).lerp(t.texel(ax, by).lerp(t.texel(bx, by), fx), fy)
}
