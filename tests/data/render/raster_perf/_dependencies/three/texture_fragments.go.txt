package three

import (
	"fmt"
	"image"
	"math"
	"sort"
)

// Textured alpha includes a filtered texel alpha times material alpha. Rounding
// it to 8 bits before compositing would lose many faint layers or falsely make
// nearly opaque layers write depth. Only frames that can emit textured partial
// alpha use these 24-byte records; constant-color frames retain 16-byte records.
// Record budgets and fixed-block allocation rules are the same for both forms.
type texturedFragment struct {
	depth  float64
	sample uint32
	rgb    uint32
	alpha  float64
}

type texturedFragmentBuffer struct {
	blocks [][]texturedFragment
	count  int
	limit  int
}

func (b *transparentBuffer) addTextured(sample int, depth float64, rgb uint32, alpha float64) error {
	if b.precise == nil {
		return fmt.Errorf("three: textured alpha storage was not prepared")
	}
	if err := b.precise.add(sample, depth, rgb, alpha); err != nil {
		return err
	}
	b.count++
	return nil
}

func (b *texturedFragmentBuffer) add(sample int, depth float64, rgb uint32, alpha float64) error {
	if b.count == b.limit {
		return fmt.Errorf("transparent fragment budget exceeded (limit %d, internal samples after opaque occlusion)", b.limit)
	}
	if b.blocks == nil {
		b.blocks = make([][]texturedFragment, (b.limit+transparentBlockSize-1)/transparentBlockSize)
	}
	block, offset := b.count/transparentBlockSize, b.count%transparentBlockSize
	if offset == 0 {
		b.blocks[block] = make([]texturedFragment, min(transparentBlockSize, b.limit-b.count))
	}
	b.blocks[block][offset] = texturedFragment{depth: depth, sample: uint32(sample), rgb: rgb, alpha: alpha}
	b.count++
	return nil
}

func (b *texturedFragmentBuffer) at(i int) *texturedFragment {
	return &b.blocks[i/transparentBlockSize][i%transparentBlockSize]
}

func (b *texturedFragmentBuffer) Len() int { return b.count }
func (b *texturedFragmentBuffer) Swap(i, j int) {
	a, c := b.at(i), b.at(j)
	*a, *c = *c, *a
}
func (b *texturedFragmentBuffer) Less(i, j int) bool {
	a, c := b.at(i), b.at(j)
	if a.sample != c.sample {
		return a.sample < c.sample
	}
	if a.depth != c.depth {
		return a.depth > c.depth
	}
	if a.rgb != c.rgb {
		return a.rgb < c.rgb
	}
	return a.alpha < c.alpha
}

func (b *texturedFragmentBuffer) composite(img *image.RGBA) {
	if b.count == 0 {
		return
	}
	sort.Sort(b)
	width := img.Bounds().Dx()
	for i := 0; i < b.count; {
		sample := b.at(i).sample
		p := int(sample)/width*img.Stride + int(sample)%width*4
		var dst [4]float64
		for c := range dst {
			dst[c] = float64(img.Pix[p+c])
		}
		for i < b.count && b.at(i).sample == sample {
			f := b.at(i)
			remaining := 1 - f.alpha
			for c := range 3 {
				source := float64((f.rgb >> uint(16-8*c)) & 255)
				dst[c] = source*f.alpha + dst[c]*remaining
			}
			dst[3] = 255*f.alpha + dst[3]*remaining
			i++
		}
		for c := range dst {
			img.Pix[p+c] = uint8(math.Round(dst[c]))
		}
	}
}
