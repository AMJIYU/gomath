package audio

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The frozen wave-five fixtures remain the input for both compiled variants.
// File I/O and input encoding are outside the measured loop.
func BenchmarkReadFLACPerfFixtures(b *testing.B) {
	files := []string{
		"flac_decode/libflac_pcm16_2ch_correlated_n8193_l8.flac",
		"flac_decode/libflac_pcm24_2ch_sine_n8193_l8_lpc32.flac",
		"flac_input/pcm8_8ch_sine_n1025_b256.flac",
		"flac_input/pcm24_8ch_random_n255_b256.flac",
		"flac_input/pcm32_8ch_sine_n1025_b256.flac",
		"flac_input/pcm32_2ch_wasted_n65535_b65535.flac",
		"flac_input/hand/side33_LPC32_shift15.flac",
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.FromSlash(name))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(filepath.Base(name), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for i := 0; i < b.N; i++ {
				if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{DiscardMetadata: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Check all 65536 CRC16 states with three input bytes, independently using
// polynomial long division. Reuse input storage to keep the test bounded.
// This exercises the actual get path, including its work/read accounting.
func TestReadFLACCRC16StateTransitions(t *testing.T) {
	var src bytes.Reader
	r := bufio.NewReaderSize(&src, 16)
	var input [1]byte
	for seed := uint32(0); seed <= 0xffff; seed++ {
		for _, b := range [...]byte{0, 0x5a, byte(seed >> 8)} {
			input[0] = b
			src.Reset(input[:])
			r.Reset(&src)
			d := flacDecoder{r: r, work: 8, crc16: uint16(seed), frameCRC: true}
			want := uint16(seed) ^ uint16(b)<<8
			for range 8 {
				if want&0x8000 != 0 {
					want = want<<1 ^ 0x8005
				} else {
					want <<= 1
				}
			}
			got := d.get(8)
			if d.err != nil || got != uint64(b) || d.crc16 != want || d.work != 0 || d.used != 1 || d.avail != 0 {
				t.Fatalf("seed=%04x byte=%02x: got=%x crc=%04x want=%04x work=%d used=%d avail=%d err=%v", seed, b, got, d.crc16, want, d.work, d.used, d.avail, d.err)
			}
		}
	}
}

func TestReadFLACCRCByteAndBudgetTiming(t *testing.T) {
	for _, header := range []bool{false, true} {
		for _, frame := range []bool{false, true} {
			t.Run(fmt.Sprintf("header%t_frame%t", header, frame), func(t *testing.T) {
				data := []byte{0xa5, 0x71}
				d := flacDecoder{r: bufio.NewReader(bytes.NewReader(data)), work: 16, headerCRC: header, frameCRC: frame}
				if got := d.get(1); got != 1 {
					t.Fatal("first bit", got, d.err)
				}
				want8, want16 := byte(0), uint16(0)
				if header {
					want8 = byte(flacTestCRC(data[:1], 8))
				}
				if frame {
					want16 = uint16(flacTestCRC(data[:1], 16))
				}
				if d.crc8 != want8 || d.crc16 != want16 || d.used != 1 || d.work != 15 {
					t.Fatal("CRC must update once when the byte is read", d)
				}
				if got := d.get(7); got != 0x25 || d.crc8 != want8 || d.crc16 != want16 || d.used != 1 || d.work != 8 {
					t.Fatal("buffered bits changed CRC or accounting", got, d)
				}
				// A failed work preflight must not consume another byte or
				// change either running checksum.
				d.get(9)
				if d.err == nil || d.crc8 != want8 || d.crc16 != want16 || d.used != 1 || d.work != 8 {
					t.Fatal("failed work preflight consumed input", d)
				}
			})
		}
	}
}
