package render

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestCaptureRasterPerfBefore(t *testing.T) {
	result := map[string]string{}
	for _, complex := range []bool{false, true} {
		f, c := rasterPerfScene(t, complex)
		for _, scale := range []int{1, 2, 4} {
			c.Supersampling = scale
			r, err := NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			im, err := r.Render(f)
			if err != nil {
				t.Fatal(err)
			}
			h := sha256.Sum256(im.Pix)
			result[fmt.Sprintf("complex=%t/ss%d", complex, scale)] = hex.EncodeToString(h[:])
		}
	}
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile("../tests/data/render/raster_perf/pixels.json", append(b, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
}
