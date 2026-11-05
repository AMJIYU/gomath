package svg

import (
	"fmt"
	"strings"
	"testing"
)

func TestSVGGradientLimitsAbortWithoutPartialOutput(t *testing.T) {
	var definitions, sourceStops, chain, reverseChain strings.Builder
	for i := range maxGradientDefinitions + 1 {
		fmt.Fprintf(&definitions, `<linearGradient id="g%d"/>`, i)
	}
	stops := `<stop/><stop offset="1"/>`
	manyStops := `<stop/>` + strings.Repeat(`<stop offset=".5"/>`, 254) + `<stop offset="1"/>`
	for i := range maxGradientSourceStops/256 + 1 {
		fmt.Fprintf(&sourceStops, `<linearGradient id="g%d">%s</linearGradient>`, i, manyStops)
	}
	for i := range maxGradientDepth + 1 {
		fmt.Fprintf(&chain, `<linearGradient id="g%d"`, i)
		if i != maxGradientDepth {
			fmt.Fprintf(&chain, ` href="#g%d"`, i+1)
		}
		chain.WriteString(`/>`)
	}
	// Reverse source order exercises memoized height checks, not just recursion.
	for i := maxGradientDepth; i >= 0; i-- {
		fmt.Fprintf(&reverseChain, `<linearGradient id="g%d"`, i)
		if i != maxGradientDepth {
			fmt.Fprintf(&reverseChain, ` href="#g%d"`, i+1)
		}
		reverseChain.WriteString(`/>`)
	}
	resolved := `<linearGradient id="g0">` + manyStops + `</linearGradient>` + strings.Repeat(`<linearGradient href="#g0"/>`, 256)
	instances := `<defs><linearGradient id="g">` + stops + `</linearGradient></defs>` + strings.Repeat(`<rect width="1" height="1" fill="url(#g)"/>`, maxGradientInstances+1)
	instanceStops := `<defs><linearGradient id="g">` + manyStops + `</linearGradient></defs>` + strings.Repeat(`<rect width="1" height="1" fill="url(#g)"/>`, maxGradientInstanceStops/256+1)
	for _, tc := range []struct{ name, source, want string }{
		{"definitions", definitions.String(), "source budget"},
		{"gradient attribute", `<linearGradient gradientTransform="` + strings.Repeat(" ", maxGradientAttributeBytes+1) + `"/>`, "attribute exceeds"},
		{"stop attribute", `<linearGradient><stop offset="` + strings.Repeat(" ", maxGradientAttributeBytes+1) + `"/></linearGradient>`, "attribute exceeds"},
		{"raw stops", sourceStops.String(), "source budget"},
		{"per definition", `<linearGradient>` + strings.Repeat(`<stop/>`, 257) + `</linearGradient>`, "256 stops"},
		{"normalized endpoints", `<linearGradient>` + strings.Repeat(`<stop offset=".5"/>`, 256) + `</linearGradient>`, "normalized endpoints"},
		{"depth", chain.String(), "depth exceeds"},
		{"cached depth", reverseChain.String(), "depth exceeds"},
		{"resolved work", resolved, "resolved stop budget"},
		{"instances", instances, "instance budget"},
		{"copied stops", instanceStops, "instance budget"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := Read(strings.NewReader(`<svg>`+tc.source+`</svg>`), DefaultOptions())
			if root != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("root=%v err=%v want %q", root, err, tc.want)
			}
		})
	}
}

func TestSVGGradientAcceptsLimitBoundaries(t *testing.T) {
	var source strings.Builder
	source.WriteString(`<svg><defs><linearGradient id="g0"><stop/>` + strings.Repeat(`<stop offset=".5"/>`, 254) + `<stop offset="1"/></linearGradient>`)
	for i := 1; i < maxGradientDepth; i++ {
		fmt.Fprintf(&source, `<linearGradient id="g%d" href="#g%d"/>`, i, i-1)
	}
	source.WriteString(`</defs><rect width="10" height="10" fill="url(#g63)"/></svg>`)
	root, err := Read(strings.NewReader(source.String()), DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Validate(); err != nil {
		t.Fatal(err)
	}
}
