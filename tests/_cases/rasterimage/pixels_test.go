package rasterimage_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/export"
	"gomath/rasterimage"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

type pixelStep struct {
	Op     string
	RGB    []float64
	Factor float64
	Alpha  *float64
}

type pixelOracle struct {
	Source struct {
		Commit          string
		GeneratorSHA256 string `json:"generator_sha256"`
	}
	Width, Height int
	Input         string
	Cases         []struct {
		Name, RGBA, PNG string
		Steps           []pixelStep
	}
	Subbyte16 [][4]uint16
	Scene     struct {
		Width, Height int
		Input         string
		Steps         []pixelStep
		States        []struct {
			Time, Opacity float64
			Shift         [2]float64
			PNG           string
		}
	}
	FilesSHA256 map[string]string `json:"files_sha256"`
}

func pixelFixture(name string) string {
	return filepath.Join("..", "tests", "data", "rasterimage", "pixels", name)
}

func readPixelFile(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(pixelFixture(name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readPixelOracle(t testing.TB) pixelOracle {
	t.Helper()
	var oracle pixelOracle
	if err := json.Unmarshal(readPixelFile(t, "oracle.json"), &oracle); err != nil {
		t.Fatal(err)
	}
	if oracle.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" {
		t.Fatal("unrecognized oracle source")
	}
	return oracle
}

func pixelInput(t testing.TB, name string, width, height int) *image.NRGBA {
	t.Helper()
	data := readPixelFile(t, name)
	if len(data) != width*height*4 {
		t.Fatal("bad fixture dimensions")
	}
	return &image.NRGBA{Pix: data, Rect: image.Rect(0, 0, width, height), Stride: width * 4}
}

func runPixelSteps(t testing.TB, a *rasterimage.Asset, steps []pixelStep) *rasterimage.Asset {
	t.Helper()
	for _, s := range steps {
		var (
			next *rasterimage.Asset
			err  error
		)
		switch s.Op {
		case "invert":
			next, err = a.Invert()
		case "opacity":
			next, err = a.SetOpacity(s.Factor)
		case "color", "tint":
			c := gomath.RGB(s.RGB[0], s.RGB[1], s.RGB[2])
			if s.Op == "tint" {
				next, err = a.Tint(c)
			} else if s.Alpha == nil {
				next, err = a.SetColor(c)
			} else {
				next, err = a.SetColor(c, *s.Alpha)
			}
		default:
			t.Fatalf("unknown oracle operation %q", s.Op)
		}
		if err != nil {
			t.Fatal(err)
		}
		if next == a || next.Bitmap() == a.Bitmap() {
			t.Fatal("pixel edit reused mutable output storage")
		}
		a = next
	}
	return a
}

func bitmapPixelBytes(t testing.TB, img image.Image) []byte {
	t.Helper()
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, blue, a := img.At(x, y).RGBA()
			if r > a || g > a || blue > a {
				t.Fatalf("non-premultiplied output at (%d,%d)", x, y)
			}
			out = append(out, byte(r>>8), byte(g>>8), byte(blue>>8), byte(a>>8))
		}
	}
	return out
}

func requirePixelBytes(t testing.TB, got, want []byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("pixel byte count %d != %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("byte %d (pixel %d channel %d): got %d, want %d", i, i/4, i%4, got[i], v)
		}
	}
}

func TestPixelFrozenOracleHashes(t *testing.T) {
	oracle := readPixelOracle(t)
	for file, want := range oracle.FilesSHA256 {
		sum := sha256.Sum256(readPixelFile(t, file))
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("frozen pixel oracle changed: %s", file)
		}
	}
	generator, err := os.ReadFile(filepath.Join("..", "tests", "reference", "image_pixels.py"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(generator)
	if hex.EncodeToString(sum[:]) != oracle.Source.GeneratorSHA256 {
		t.Fatal("reference generator differs from frozen oracle")
	}
}

func TestPixelFrozenManifest(t *testing.T) {
	var manifest struct {
		API    []string
		Bounds struct {
			MaxPixelBytes  int `json:"max_pixel_bytes"`
			MaxPixelPixels int `json:"max_pixel_pixels"`
		}
		FilesSHA256 map[string]string `json:"files_sha256"`
	}
	if err := json.Unmarshal(readPixelFile(t, "manifest.json"), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Bounds.MaxPixelBytes != rasterimage.MaxPixelBytes || manifest.Bounds.MaxPixelPixels != rasterimage.MaxPixelPixels || len(manifest.API) != 8 {
		t.Fatal("frozen API/bounds contract differs")
	}
	if len(manifest.FilesSHA256) < 40 {
		t.Fatal("incomplete file freeze")
	}
	for name, want := range manifest.FilesSHA256 {
		data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != want {
			t.Fatalf("pixel freeze changed: %s", name)
		}
	}
}

func TestPixelPinnedNumPyPillowByteOracle(t *testing.T) {
	oracle := readPixelOracle(t)
	input := pixelInput(t, oracle.Input, oracle.Width, oracle.Height)
	before := append([]byte(nil), input.Pix...)
	a, err := rasterimage.NewAsset(input)
	if err != nil {
		t.Fatal(err)
	}
	initial := bitmapPixelBytes(t, a.Bitmap())
	decodedInput, err := png.Decode(bytes.NewReader(readPixelFile(t, "input.png")))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := rasterimage.NewAsset(decodedInput)
	if err != nil {
		t.Fatal(err)
	}
	requirePixelBytes(t, bitmapPixelBytes(t, loaded.Bitmap()), initial)
	loadedInverse, err := loaded.Invert()
	if err != nil {
		t.Fatal(err)
	}
	requirePixelBytes(t, bitmapPixelBytes(t, loadedInverse.Bitmap()), readPixelFile(t, "invert.rgba"))
	for _, c := range oracle.Cases {
		t.Run(c.Name, func(t *testing.T) {
			output := runPixelSteps(t, a, c.Steps)
			requirePixelBytes(t, bitmapPixelBytes(t, output.Bitmap()), readPixelFile(t, c.RGBA))
			// Existing PNG export must agree with independently generated Pillow
			// canonical PNG bytes after decoding (encoder bytes need not match).
			name := filepath.Join(t.TempDir(), "pixels.png")
			if err := export.PNG(name, output.Bitmap()); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(bytes.NewReader(got))
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(bytes.NewReader(readPixelFile(t, c.PNG)))
			if err != nil {
				t.Fatal(err)
			}
			requirePixelBytes(t, bitmapPixelBytes(t, decoded), bitmapPixelBytes(t, want))
		})
	}
	requirePixelBytes(t, input.Pix, before)
	requirePixelBytes(t, bitmapPixelBytes(t, a.Bitmap()), initial)
}

func TestPixelSubbyteAlphaAndHiddenRGB(t *testing.T) {
	oracle := readPixelOracle(t)
	input := image.NewNRGBA64(image.Rect(-5, 7, -5+len(oracle.Subbyte16), 8))
	for i, c := range oracle.Subbyte16 {
		input.SetNRGBA64(-5+i, 7, color.NRGBA64{R: c[0], G: c[1], B: c[2], A: c[3]})
	}
	a, err := rasterimage.NewAsset(input)
	if err != nil {
		t.Fatal(err)
	}
	requirePixelBytes(t, bitmapPixelBytes(t, a.Bitmap()), readPixelFile(t, "subbyte16.rgba"))
	// Alpha=1 with RGB=1 must invert to 254 before premultiplication, not
	// unpremultiply the rounded-to-zero bitmap channel and invert to 255.
	p := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	p.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 17, B: 254, A: 1})
	p.SetNRGBA(1, 0, color.NRGBA{R: 127, G: 71, B: 33, A: 0})
	a, err = rasterimage.NewAsset(p)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := a.Invert()
	if err != nil {
		t.Fatal(err)
	}
	if got := inv.Bitmap().RGBAAt(0, 0); got != (color.RGBA{A: 1}) {
		t.Fatal("inverted premultiplied channels instead of original straight RGB", got)
	}
	// An operation that explicitly supplies new straight RGB can reveal an
	// alpha-zero pixel. Import itself discards that pixel's hidden old RGB.
	revealed, err := inv.SetColor(gomath.RGB(1, 0, .5), 1)
	if err != nil || revealed.Bitmap().RGBAAt(1, 0) != (color.RGBA{R: 255, B: 127, A: 255}) {
		t.Fatal("uniform alpha/color replacement failed", err)
	}
	// Importing an existing Bitmap cannot reconstruct its quantized RGB.
	imported, err := rasterimage.NewAsset(a.Bitmap())
	if err != nil {
		t.Fatal(err)
	}
	imported, err = imported.Invert()
	if err != nil || imported.Bitmap().RGBAAt(0, 0) != (color.RGBA{R: 1, G: 1, B: 1, A: 1}) {
		t.Fatal("premultiplied import's explicit recovery semantics changed", err)
	}
}

func TestPixelInputOwnershipAndConcurrentDerivation(t *testing.T) {
	backing := image.NewNRGBA(image.Rect(10, 20, 16, 24))
	for y := 20; y < 24; y++ {
		for x := 10; x < 16; x++ {
			backing.SetNRGBA(x, y, color.NRGBA{R: 1, G: 201, B: 19, A: 113})
		}
	}
	input := backing.SubImage(image.Rect(11, 21, 15, 23)).(*image.NRGBA)
	before := append([]byte(nil), backing.Pix...)
	a, err := rasterimage.NewAsset(input)
	if err != nil {
		t.Fatal(err)
	}
	if a.Bitmap().Bounds() != image.Rect(0, 0, 4, 2) {
		t.Fatal("subimage origin was not normalized")
	}
	requirePixelBytes(t, backing.Pix, before)
	original := bitmapPixelBytes(t, a.Bitmap())
	clear(backing.Pix)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 12 {
				next, err := a.SetOpacity(0)
				if err != nil {
					t.Error(err)
					return
				}
				next, err = next.SetOpacity(1)
				if err != nil {
					t.Error(err)
					return
				}
				if !reflect.DeepEqual(next.Bitmap().RGBAAt(0, 0), a.Bitmap().RGBAAt(0, 0)) {
					t.Error("opacity restore lost original straight RGB")
				}
				inv, err := next.Invert()
				if err != nil {
					t.Error(err)
					return
				}
				_, err = inv.Tint(gomath.RGB(.5, .25, 1))
				if err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	requirePixelBytes(t, bitmapPixelBytes(t, a.Bitmap()), original)
}

func BenchmarkPixelOperations(b *testing.B) {
	input := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for i := range input.Pix {
		input.Pix[i] = byte(i*37 + 13)
	}
	a, err := rasterimage.NewAsset(input)
	if err != nil {
		b.Fatal(err)
	}
	for _, op := range []string{"invert", "opacity", "color", "tint"} {
		b.Run(op, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(256 * 256 * 4)
			b.ResetTimer()
			for range b.N {
				var err error
				switch op {
				case "invert":
					_, err = a.Invert()
				case "opacity":
					_, err = a.SetOpacity(.37)
				case "color":
					_, err = a.SetColor(gomath.RGB(.1, .501, 1), .5)
				case "tint":
					_, err = a.Tint(gomath.RGB(.37, .8, .125))
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
