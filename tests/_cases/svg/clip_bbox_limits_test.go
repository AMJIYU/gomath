package svg

import (
	"fmt"
	"strings"
	"testing"
)

func TestSVGBBoxClipGeometryAndInstancesShareBudget(t *testing.T) {
	for _, use := range []bool{false, true} {
		t.Run(fmt.Sprint(use), func(t *testing.T) {
			perInstance := 2 // one target-geometry scan and one materialized clip
			body := `<rect width="4" height="6" clip-path="url(#c)"/>`
			if use {
				perInstance = 3
				body = `<use href="#r" clip-path="url(#c)"/>`
			}
			prefix := `<svg><defs><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/></clipPath><rect id="r" width="4" height="6"/></defs>`
			// Source clip plus definition transform cost two, even when reused.
			accepted := (maxClipPaths - 2) / perInstance
			if _, err := Read(strings.NewReader(prefix+strings.Repeat(body, accepted)+`</svg>`), DefaultOptions()); err != nil {
				t.Fatal("valid budget boundary rejected", err)
			}
			o, err := Read(strings.NewReader(prefix+strings.Repeat(body, accepted+1)+`</svg>`), DefaultOptions())
			if o != nil || err == nil || !strings.Contains(err.Error(), "materialization budget") {
				t.Fatal("target bbox scans bypassed shared budget", err)
			}
		})
	}
}

func TestSVGBBoxClipEvenDegenerateTargetsChargeGeometry(t *testing.T) {
	source := `<svg><defs><clipPath id="c" clipPathUnits="objectBoundingBox"/></defs>` + strings.Repeat(`<line x2="4" clip-path="url(#c)"/>`, maxClipPaths) + `</svg>`
	o, err := Read(strings.NewReader(source), DefaultOptions())
	if o != nil || err == nil || !strings.Contains(err.Error(), "materialization budget") {
		t.Fatal("degenerate bbox bypassed scan budgets", err)
	}
}

func TestSVGBBoxClipMixedModeCacheCannotBypassUnitValidation(t *testing.T) {
	for _, order := range []bool{false, true} {
		user := `<clipPath id="user"><use href="#r"/></clipPath>`
		bbox := `<clipPath id="bbox" clipPathUnits="objectBoundingBox"><use href="#r"/></clipPath>`
		defs := user + bbox
		if order {
			defs = bbox + user
		}
		o, err := Read(strings.NewReader(`<svg><defs><rect id="r" width="1px" height="1"/>`+defs+`</defs></svg>`), DefaultOptions())
		if o != nil || err == nil || !strings.Contains(err.Error(), "unitless") {
			t.Fatal("cache mode bypassed normalized coordinate validation", err)
		}
	}
}
