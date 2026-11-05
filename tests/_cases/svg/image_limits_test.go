package svg

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"strings"
	"testing"
)

func TestEmbeddedImageBudgetBoundaries(t *testing.T) {
	data, err := os.ReadFile("../tests/data/svg/embedded.png")
	if err != nil {
		t.Fatal(err)
	}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	for _, tc := range []struct {
		name   string
		budget imageBudget
		want   string
	}{
		{"single decode", imageBudget{}, ""},
		{"exact aggregate budgets", imageBudget{encoded: maxImageTotalEncodedBytes - len(data), pixels: maxImageTotalPixels - 128, assets: maxImageAssets - 1}, ""},
		{"aggregate encoded", imageBudget{encoded: maxImageTotalEncodedBytes - len(data) + 1}, "encoded byte budget"},
		{"aggregate pixels", imageBudget{pixels: maxImageTotalPixels - 127}, "total decoded pixel budget"},
		{"asset count", imageBudget{assets: maxImageAssets}, "asset count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.budget
			asset, err := decodeImageURI(uri, &b)
			if tc.want == "" {
				if err != nil || asset == nil {
					t.Fatalf("decode: %v", err)
				}
				if b.assets != tc.budget.assets+1 || b.pixels != tc.budget.pixels+128 || b.encoded != tc.budget.encoded+len(data) {
					t.Fatalf("accounting: %+v", b)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v want %q", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name   string
		budget imageBudget
		n      int
		fail   bool
	}{
		{"exact per URI", imageBudget{}, maxImageURIBytes, false},
		{"per URI exceeded", imageBudget{}, maxImageURIBytes + 1, true},
		{"exact total URI", imageBudget{uri: maxImageTotalURIBytes - 1}, 1, false},
		{"total URI exceeded", imageBudget{uri: maxImageTotalURIBytes}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.budget.consumeURI(tc.n)
			if (err != nil) != tc.fail {
				t.Fatalf("URI budget: %v", err)
			}
		})
	}
	// This header is valid enough for DecodeConfig but has no compressed pixels.
	// A dimension bomb must fail the pixel check before attempting full decode.
	var header bytes.Buffer
	header.WriteString("\x89PNG\r\n\x1a\n")
	binary.Write(&header, binary.BigEndian, uint32(13))
	payload := append([]byte("IHDR"), make([]byte, 13)...)
	binary.BigEndian.PutUint32(payload[4:8], 4097)
	binary.BigEndian.PutUint32(payload[8:12], 1024)
	payload[12], payload[13] = 8, 6
	header.Write(payload)
	binary.Write(&header, binary.BigEndian, crc32.ChecksumIEEE(payload))
	bomb := "data:image/png;base64," + base64.StdEncoding.EncodeToString(header.Bytes())
	if _, err := decodeImageURI(bomb, &imageBudget{}); err == nil || !strings.Contains(err.Error(), "decoded pixel budget") {
		t.Fatalf("dimension bomb: %v", err)
	}
	huge := "data:image/png;base64," + strings.Repeat("AAAA", maxImageEncodedBytes/3+1)
	if _, err := decodeImageURI(huge, &imageBudget{}); err == nil || !strings.Contains(err.Error(), "encoded byte budget") {
		t.Fatalf("encoded image cap: %v", err)
	}
	// Exercise the streaming XML cap, including allocation of a single token.
	input := io.MultiReader(strings.NewReader("<svg><!--"), io.LimitReader(repeatingImageByte('x'), maxSVGInputBytes), strings.NewReader("--></svg>"))
	if _, err := Read(input, DefaultOptions()); err == nil || !strings.Contains(err.Error(), "SVG input exceeds") {
		t.Fatalf("XML budget: %v", err)
	}
	// Cache hits consume URI text but no additional pixels/assets/decode bytes.
	doc := `<svg><defs><image id="a" href="` + uri + `"/></defs><use href="#a"/><image href="` + uri + `"/></svg>`
	root, err := Read(strings.NewReader(doc), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if root.Children[0].Children[0].Children[0].Bitmap != root.Children[0].Children[1].Bitmap {
		t.Fatal("repeated href not cached")
	}
}

type repeatingImageByte byte

func (b repeatingImageByte) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}
