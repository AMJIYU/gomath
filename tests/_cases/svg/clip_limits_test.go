package svg

import (
	"fmt"
	"strings"
	"testing"
)

func TestSVGClipLimitsAbortWithoutPartialOutput(t *testing.T) {
	var definitions, ancestors, chain, instances strings.Builder
	for i := range maxClipDefinitions + 1 {
		fmt.Fprintf(&definitions, `<clipPath id="c%d"/>`, i)
	}
	for range 65 {
		ancestors.WriteString(`<g clip-path="url(#c)">`)
	}
	ancestors.WriteString(`<rect width="5" height="5"/>`)
	ancestors.WriteString(strings.Repeat(`</g>`, 65))
	for i := range 65 {
		fmt.Fprintf(&chain, `<clipPath id="c%d"`, i)
		if i != 64 {
			fmt.Fprintf(&chain, ` clip-path="url(#c%d)"`, i+1)
		}
		chain.WriteString(`><rect width="5" height="5"/></clipPath>`)
	}
	for range maxClipPaths {
		instances.WriteString(`<rect width="5" height="5" clip-path="url(#c)"/>`)
	}
	for _, tc := range []struct{ name, source, want string }{
		{"definitions", definitions.String(), "definition count"},
		{"contours", `<clipPath><path d="` + strings.Repeat(`M0 0 `, 4097) + `"/></clipPath>`, "4096 contours"},
		{"curves", `<clipPath><path d="M0 0 ` + strings.Repeat(`L1 1 `, 65537) + `"/></clipPath>`, "65536 cubic"},
		{"source geometry bytes", `<clipPath><path d="M0 0` + strings.Repeat(" ", maxClipShapeBytes) + `"/></clipPath>`, "geometry attribute"},
		{"intersection depth", chain.String(), "depth exceeds"},
		{"ancestor depth", `<defs><clipPath id="c"><rect width="5" height="5"/></clipPath></defs>` + ancestors.String(), "ancestors exceed"},
		{"instance paths", `<defs><clipPath id="c"><rect width="5" height="5"/></clipPath></defs>` + instances.String(), "materialization budget"},
		{"instance contours", `<defs><clipPath id="c"><path d="` + strings.Repeat(`M0 0 `, 4096) + `"/></clipPath></defs>` + strings.Repeat(`<rect width="5" height="5" clip-path="url(#c)"/>`, 16), "materialization budget"},
		{"instance curves", `<defs><clipPath id="c"><path d="M0 0 ` + strings.Repeat(`L1 1 `, 2048) + `"/></clipPath></defs>` + strings.Repeat(`<rect width="5" height="5" clip-path="url(#c)"/>`, 512), "materialization budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := Read(strings.NewReader(`<svg>`+tc.source+`</svg>`), DefaultOptions())
			if root != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("root=%v err=%v; want %s", root, err, tc.want)
			}
		})
	}
}

func TestSVGClipAcceptsNativeAncestorBoundary(t *testing.T) {
	source := `<svg><defs><clipPath id="c"><rect width="5" height="5"/></clipPath></defs>` +
		strings.Repeat(`<g clip-path="url(#c)">`, 64) + `<rect width="5" height="5"/>` + strings.Repeat(`</g>`, 64) + `</svg>`
	root, err := Read(strings.NewReader(source), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Validate(); err != nil {
		t.Fatal(err)
	}
}
