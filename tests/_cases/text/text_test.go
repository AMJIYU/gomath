package text_test

import (
	"gomath"
	"gomath/text"
	"sync"
	"testing"
)

func TestGlyphOutlinesAndMissingCharacters(t *testing.T) {
	font := text.DefaultFont()
	o, e := text.New("Math 123\nα+β", font, text.DefaultOptions())
	if e != nil {
		t.Fatal(e)
	}
	if !o.Bounds().Valid || o.Bounds().Size().Y < .5 {
		t.Fatal("multiline layout missing")
	}
	o.Walk(func(o *gomath.Object) {
		if e := o.Path.Validate(); e != nil {
			t.Fatal(e)
		}
	})
	if _, e = text.New("数学", font, text.DefaultOptions()); e == nil {
		t.Fatal("missing glyph silently replaced")
	}
	opt := text.DefaultOptions()
	opt.AllowMissing = true
	if _, e = text.New("数学", font, opt); e != nil {
		t.Fatal(e)
	}
}
func TestSharedFontConcurrentLayout(t *testing.T) {
	font := text.DefaultFont()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				if _, e := text.New("GoMath αβ", font, text.DefaultOptions()); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
}
func TestBadInput(t *testing.T) {
	if _, e := text.Parse([]byte("invalid")); e == nil {
		t.Fatal("invalid font accepted")
	}
	if _, e := text.New(string([]byte{0xff}), text.DefaultFont(), text.DefaultOptions()); e == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if _, e := text.New("x", nil, text.DefaultOptions()); e == nil {
		t.Fatal("nil font accepted")
	}
}
