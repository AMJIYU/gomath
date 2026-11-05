package three

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
)

const (
	// DefaultTransparentFragmentLimit is the default per-frame budget after
	// opaque occlusion, counted at the internal supersampling resolution.
	DefaultTransparentFragmentLimit = 1024 * 1024
	// MaxTransparentFragments is the hard per-frame fragment budget. Records
	// occupy 16 bytes each (64 MiB maximum), or 24 bytes in frames that can emit
	// textured partial alpha (96 MiB maximum), separately from raster buffers.
	MaxTransparentFragments = 4 * 1024 * 1024
	transparentBlockSize    = 4096
)

// A record has no pointers and occupies 16 bytes on supported Go platforms.
// rgba packs shaded, unassociated 8-bit R,G,B,A from most to least significant.
// Blending converts to premultiplied float64 and rounds only after all layers.
type transparentFragment struct {
	depth  float64
	sample uint32
	rgba   uint32
}

// Blocks never grow or copy, bounding live fragment payload by limit*16 rather
// than relying on append's capacity growth or the garbage collector. A block
// directory is allocated only on the first surviving fragment; at the hard
// limit it has 1024 slice headers (24 KiB on 64-bit platforms). No per-sample
// heads, depth-layer arrays or unbounded per-pixel allocations are needed.
type transparentBuffer struct {
	blocks [][]transparentFragment
	count  int
	limit  int
	// Textured transparency retains filtered/product alpha as float64. Frames
	// without it keep their original 16-byte record storage and blend path.
	precise *texturedFragmentBuffer
}

func (b *transparentBuffer) add(sample int, depth float64, rgba uint32) error {
	if b.precise != nil {
		return b.addTextured(sample, depth, rgba>>8, float64(rgba&255)/255)
	}
	if b.count == b.limit {
		return fmt.Errorf("transparent fragment budget exceeded (limit %d, internal samples after opaque occlusion)", b.limit)
	}
	if b.blocks == nil {
		b.blocks = make([][]transparentFragment, (b.limit+transparentBlockSize-1)/transparentBlockSize)
	}
	block, offset := b.count/transparentBlockSize, b.count%transparentBlockSize
	if offset == 0 {
		b.blocks[block] = make([]transparentFragment, min(transparentBlockSize, b.limit-b.count))
	}
	b.blocks[block][offset] = transparentFragment{depth: depth, sample: uint32(sample), rgba: rgba}
	b.count++
	return nil
}

func (b *transparentBuffer) at(i int) *transparentFragment {
	return &b.blocks[i/transparentBlockSize][i%transparentBlockSize]
}

func (b *transparentBuffer) Len() int { return b.count }
func (b *transparentBuffer) Swap(i, j int) {
	a, c := b.at(i), b.at(j)
	*a, *c = *c, *a
}
func (b *transparentBuffer) Less(i, j int) bool {
	a, c := b.at(i), b.at(j)
	if a.sample != c.sample {
		return a.sample < c.sample
	}
	if a.depth != c.depth {
		return a.depth > c.depth // source-over from far to near
	}
	// Coplanar layers have no geometric ordering. This explicit color tie rule
	// is deterministic under instance and triangle permutations, including
	// identical records; it does not use a submission index or depth epsilon.
	return a.rgba < c.rgba
}

func (b *transparentBuffer) composite(img *image.RGBA) {
	if b.precise != nil {
		b.precise.composite(img)
		return
	}
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
			rgba := b.at(i).rgba
			alpha := float64(rgba&255) / 255
			remaining := 1 - alpha
			for c := range 3 {
				source := float64((rgba >> uint(24-8*c)) & 255)
				dst[c] = source*alpha + dst[c]*remaining
			}
			dst[3] = float64(rgba&255) + dst[3]*remaining
			i++
		}
		for c := range dst {
			img.Pix[p+c] = uint8(math.Round(dst[c]))
		}
	}
}

// transparentEdge evaluates a line equation with reversed endpoint evaluations
// as exact negatives. The endpoint-relative opaque formula can round both sides
// of a shared edge positive; depth hides that for opaque surfaces, but blending
// must not count the same surface twice along a triangulation edge.
func transparentEdge(a, b screenVertex, x, y float64) float64 {
	// Explicit rounding conversions prohibit fused multiply-add, preserving
	// antisymmetry even on architectures/toolchains that otherwise fuse it.
	return float64((b.x-a.x)*y) - float64((b.y-a.y)*x) + (float64(a.x*b.y) - float64(a.y*b.x))
}

// rasterizeTransparent uses the opaque path's top-left rule, perspective depth
// and normal interpolation equations. It never writes depth: every surviving
// layer must reach the per-sample sort, including intersecting and rear surfaces.
func (r *Renderer) rasterizeTransparent(img *image.RGBA, depth []float64, a, b, c screenVertex, base color.NRGBA, perspective bool, fragments *transparentBuffer) error {
	area := edge(a, b, c.x, c.y)
	if area == 0 || !finite(area) || (r.config.CullBackfaces && area >= 0) {
		return nil
	}
	if area < 0 {
		b, c = c, b
		area = -area
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	x0 := max(0, int(math.Ceil(math.Min(a.x, math.Min(b.x, c.x))-.5)))
	x1 := min(w-1, int(math.Floor(math.Max(a.x, math.Max(b.x, c.x))-.5)))
	y0 := max(0, int(math.Ceil(math.Min(a.y, math.Min(b.y, c.y))-.5)))
	y1 := min(h-1, int(math.Floor(math.Max(a.y, math.Max(b.y, c.y))-.5)))
	t0, t1, t2 := topLeft(b, c), topLeft(c, a), topLeft(a, b)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px, py := float64(x)+.5, float64(y)+.5
			if !covered(transparentEdge(b, c, px, py), t0) || !covered(transparentEdge(c, a, px, py), t1) || !covered(transparentEdge(a, b, px, py), t2) {
				continue
			}
			e0, e1, e2 := edge(b, c, px, py), edge(c, a, px, py), edge(a, b, px, py)
			wa, wb, wc := e0/area, e1/area, e2/area
			z := wa*a.z + wb*b.z + wc*c.z
			if perspective {
				q := wa*a.q + wb*b.q + wc*c.q
				z = 1 / q
				wa, wb, wc = wa*a.q/q, wb*b.q/q, wc*c.q/q
			}
			i := y*w + x
			if !finite(z) || z >= depth[i] {
				continue
			}
			n, ok := unit(a.n.Mul(wa).Add(b.n.Mul(wb)).Add(c.n.Mul(wc)))
			lambert := 0.0
			if ok {
				lambert = math.Max(0, n.Dot(r.lightDirection))
			}
			intensity := math.Min(1, r.config.Light.Ambient+r.config.Light.Diffuse*lambert)
			var rgba uint32
			for _, channel := range [3]uint8{base.R, base.G, base.B} {
				rgba = rgba<<8 | uint32(math.Round(float64(channel)*intensity))
			}
			rgba = rgba<<8 | uint32(base.A)
			if err := fragments.add(i, z, rgba); err != nil {
				return err
			}
		}
	}
	return nil
}
