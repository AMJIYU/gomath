package export

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
)

// GIF streams frames with a fixed 3-3-2 palette, retaining one frame in memory.
// GIF timing has centisecond precision; error diffusion preserves total timing.
type GIF struct {
	out                      *atomicFile
	width, height, fps, next int
	palette                  color.Palette
	started                  bool
}

func NewGIF(target string, width, height, fps int) (*GIF, error) {
	if width <= 0 || height <= 0 || width > 65535 || height > 65535 || fps <= 0 || fps > 100 {
		return nil, fmt.Errorf("GIF requires valid dimensions and FPS in [1,100]")
	}
	out, e := newAtomicFile(target)
	if e != nil {
		return nil, e
	}
	p := make(color.Palette, 256)
	for i := range 256 {
		p[i] = color.RGBA{uint8((i >> 5) * 255 / 7), uint8(((i >> 2) & 7) * 255 / 7), uint8((i & 3) * 255 / 3), 255}
	}
	return &GIF{out: out, width: width, height: height, fps: fps, palette: p}, nil
}
func (g *GIF) WriteFrame(index int, img *image.RGBA) error {
	if g.out.closed || index != g.next {
		return fmt.Errorf("GIF expected frame %d", g.next)
	}
	if img.Rect.Dx() != g.width || img.Rect.Dy() != g.height {
		return fmt.Errorf("GIF frame size mismatch")
	}
	p := image.NewPaletted(image.Rect(0, 0, g.width, g.height), g.palette)
	for y := range g.height {
		for x := range g.width {
			c := color.NRGBAModel.Convert(img.At(img.Rect.Min.X+x, img.Rect.Min.Y+y)).(color.NRGBA)
			p.Pix[y*p.Stride+x] = (c.R & 0xe0) | ((c.G & 0xe0) >> 3) | (c.B >> 6)
		}
	}
	var b bytes.Buffer
	if e := gif.Encode(&b, p, nil); e != nil {
		return e
	}
	data := b.Bytes()
	header := 13
	if data[10]&0x80 != 0 {
		header += 3 * (2 << uint(data[10]&7))
	}
	if !g.started {
		if _, e := g.out.file.Write(data[:header]); e != nil {
			return e
		}
		loop := []byte{0x21, 0xff, 0x0b, 'N', 'E', 'T', 'S', 'C', 'A', 'P', 'E', '2', '.', '0', 3, 1, 0, 0, 0}
		if _, e := g.out.file.Write(loop); e != nil {
			return e
		}
		g.started = true
	}
	delay := ((index+1)*100+g.fps/2)/g.fps - (index*100+g.fps/2)/g.fps
	gce := []byte{0x21, 0xf9, 4, 4, 0, 0, 0, 0}
	binary.LittleEndian.PutUint16(gce[4:6], uint16(delay))
	if _, e := g.out.file.Write(gce); e != nil {
		return e
	}
	if _, e := g.out.file.Write(data[header : len(data)-1]); e != nil {
		return e
	}
	g.next++
	return nil
}
func (g *GIF) Close() error {
	if !g.started {
		return fmt.Errorf("cannot close empty GIF")
	}
	if _, e := g.out.file.Write([]byte{0x3b}); e != nil {
		return e
	}
	return g.out.commit()
}
func (g *GIF) Abort() error { return g.out.abort() }
