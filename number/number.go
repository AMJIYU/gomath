// Package number provides bounded fixed-decimal and exact int64 authoring
// objects using real text outlines. It is not a TeX, locale or shaping engine.
package number

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxDecimalPlaces = 18
	MaxUnitRunes     = 64
	MaxGlyphs        = 512
	MaxCurves        = 1 << 20
	DefaultMaxCurves = 65536
	// These bound local source coordinates and transformed world coordinates.
	MaxLocalCoordinate = 1e6
	MaxWorldCoordinate = 1e12
)

// Alignment selects a baseline anchor in the entire display's advance box,
// including the literal unit and spaces. It does not use an ink-bounds edge.
type Alignment uint8

const (
	Right Alignment = iota
	Left
	Center
)

type Options struct {
	DecimalPlaces               int
	IncludeSign, GroupThousands bool
	Unit                        string // Plain single-line text; a single space is inserted before it.
	Size                        float64
	Color                       gomath.Color
	// Nil Font selects an embedded real Style face. A nonnil Font is the exact
	// face; Style must then be Regular (supply a real custom bold/italic face).
	Font   *text.Font
	Style  text.FontStyle
	Anchor Alignment
	// Zero selects MaxGlyphs / DefaultMaxCurves; hard limits are above.
	MaxGlyphs, MaxCurves int
}

func DefaultOptions() Options {
	return Options{DecimalPlaces: 2, GroupThousands: true, Size: .5, Color: gomath.White, Anchor: Right}
}
func DefaultIntegerOptions() Options { o := DefaultOptions(); o.DecimalPlaces = 0; return o }

// DecimalNumber retains the original finite binary64 input in Value, even when
// Text is rounded. SetValue is authoring-only. Whole-Object affine transforms
// and valid style edits are supported; path/topology/identity edits are not.
type DecimalNumber struct {
	Object *gomath.Object
	d      *display
	value  float64
}
type Integer struct {
	Object *gomath.Object
	d      *display
	value  int64
}

func New(value float64, opt Options) (*DecimalNumber, error) {
	o, err := normalize(opt)
	if err != nil {
		return nil, err
	}
	s, err := formatDecimal(value, o)
	if err != nil {
		return nil, err
	}
	d, err := newDisplay(s, o, "DecimalNumber")
	if err != nil {
		return nil, err
	}
	return &DecimalNumber{Object: d.root, d: d, value: value}, nil
}

// NewInteger never converts its int64 value through binary64. DecimalPlaces
// must be zero; use DefaultIntegerOptions for an exact integer configuration.
func NewInteger(value int64, opt Options) (*Integer, error) {
	o, err := normalize(opt)
	if err != nil {
		return nil, err
	}
	if o.DecimalPlaces != 0 {
		return nil, fmt.Errorf("integer requires zero decimal places; use DefaultIntegerOptions")
	}
	d, err := newDisplay(formatInteger(value, o), o, "Integer")
	if err != nil {
		return nil, err
	}
	return &Integer{Object: d.root, d: d, value: value}, nil
}
func (n *DecimalNumber) Value() float64 {
	if n == nil {
		return 0
	}
	return n.value
}
func (n *Integer) Value() int64 {
	if n == nil {
		return 0
	}
	return n.value
}
func (n *DecimalNumber) Text() string {
	if n == nil || n.d == nil {
		return ""
	}
	return n.d.formatted
}
func (n *Integer) Text() string {
	if n == nil || n.d == nil {
		return ""
	}
	return n.d.formatted
}

func (n *DecimalNumber) SetValue(value float64) error {
	if n == nil || n.d == nil {
		return fmt.Errorf("nil or uninitialized decimal")
	}
	s, err := formatDecimal(value, n.d.opt)
	if err != nil {
		return err
	}
	if err := n.d.update(n.Object, s); err != nil {
		return err
	}
	n.value = value
	return nil
}
func (n *Integer) SetValue(value int64) error {
	if n == nil || n.d == nil {
		return fmt.Errorf("nil or uninitialized integer")
	}
	if err := n.d.update(n.Object, formatInteger(value, n.d.opt)); err != nil {
		return err
	}
	n.value = value
	return nil
}
func (n *DecimalNumber) AnchorPoint() (geom.Vec3, error) {
	if n == nil || n.d == nil {
		return geom.Vec3{}, fmt.Errorf("nil or uninitialized decimal")
	}
	s, err := n.d.capture(n.Object)
	if err != nil {
		return geom.Vec3{}, err
	}
	return s.matrix.Apply(geom.V(0, 0)), nil
}
func (n *Integer) AnchorPoint() (geom.Vec3, error) {
	if n == nil || n.d == nil {
		return geom.Vec3{}, fmt.Errorf("nil or uninitialized integer")
	}
	s, err := n.d.capture(n.Object)
	if err != nil {
		return geom.Vec3{}, err
	}
	return s.matrix.Apply(geom.V(0, 0)), nil
}

// Sources snapshot options, styles and the current affine baseline frame;
// they retain no live authoring object, glyph path or mutable font input bytes.
// At returns fresh independent geometry and is safe for concurrent calls.
type DecimalSource struct{ s *snapshot }
type IntegerSource struct{ s *snapshot }

func (n *DecimalNumber) Source() (*DecimalSource, error) {
	if n == nil || n.d == nil {
		return nil, fmt.Errorf("nil or uninitialized decimal")
	}
	s, err := n.d.capture(n.Object)
	if err != nil {
		return nil, err
	}
	return &DecimalSource{s: s}, nil
}
func (n *Integer) Source() (*IntegerSource, error) {
	if n == nil || n.d == nil {
		return nil, fmt.Errorf("nil or uninitialized integer")
	}
	s, err := n.d.capture(n.Object)
	if err != nil {
		return nil, err
	}
	return &IntegerSource{s: s}, nil
}
func (s *DecimalSource) At(value float64) (*gomath.Object, error) {
	if s == nil || s.s == nil {
		return nil, fmt.Errorf("nil or uninitialized decimal source")
	}
	v, err := formatDecimal(value, s.s.opt)
	if err != nil {
		return nil, err
	}
	return s.s.at(v)
}
func (s *IntegerSource) At(value int64) (*gomath.Object, error) {
	if s == nil || s.s == nil {
		return nil, fmt.Errorf("nil or uninitialized integer source")
	}
	return s.s.at(formatInteger(value, s.s.opt))
}

func normalize(o Options) (Options, error) {
	if o.DecimalPlaces < 0 || o.DecimalPlaces > MaxDecimalPlaces {
		return Options{}, fmt.Errorf("decimal places must be in [0,%d]", MaxDecimalPlaces)
	}
	if !finite(o.Size) || o.Size <= 0 || o.Size > 1e6 || o.Size < 1e-12 {
		return Options{}, fmt.Errorf("number size must be finite and in [1e-12,1e6]")
	}
	if o.Anchor > Center || o.Style > text.BoldItalic {
		return Options{}, fmt.Errorf("unsupported number anchor or font style")
	}
	if err := (gomath.Style{Fill: o.Color}).Validate(); err != nil {
		return Options{}, err
	}
	if !utf8.ValidString(o.Unit) || len(o.Unit) > 4*MaxUnitRunes || utf8.RuneCountInString(o.Unit) > MaxUnitRunes {
		return Options{}, fmt.Errorf("unit must be valid UTF-8 and at most %d runes", MaxUnitRunes)
	}
	for _, r := range o.Unit {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.M) || unicode.IsSpace(r) && r != ' ' {
			return Options{}, fmt.Errorf("unit requires single-line printable horizontal text without controls, format or combining characters")
		}
	}
	if o.Font == nil {
		f, err := text.DefaultFontForStyle(o.Style)
		if err != nil {
			return Options{}, err
		}
		o.Font = f
	} else if o.Style != text.Regular {
		return Options{}, fmt.Errorf("custom Font is an exact face; Style must be Regular")
	}
	if err := o.Font.Validate(); err != nil {
		return Options{}, err
	}
	if o.MaxGlyphs == 0 {
		o.MaxGlyphs = MaxGlyphs
	}
	if o.MaxCurves == 0 {
		o.MaxCurves = DefaultMaxCurves
	}
	if o.MaxGlyphs < 1 || o.MaxGlyphs > MaxGlyphs || o.MaxCurves < 1 || o.MaxCurves > MaxCurves {
		return Options{}, fmt.Errorf("invalid number glyph/curve limits")
	}
	return o, nil
}

func finite(v float64) bool { return !math.IsInf(v, 0) && !math.IsNaN(v) }
func formatDecimal(v float64, o Options) (string, error) {
	if !finite(v) {
		return "", fmt.Errorf("decimal value must be finite")
	}
	// FormatFloat rounds the represented binary64 directly; no multiply by
	// 10^precision, int conversion, decimal truncation or exponent overflow.
	s := strconv.FormatFloat(v, 'f', o.DecimalPlaces, 64)
	if strings.HasPrefix(s, "-") && strings.Trim(s[1:], "0.") == "" {
		s = s[1:]
	}
	return decorate(s, o), nil
}
func formatInteger(v int64, o Options) string { return decorate(strconv.FormatInt(v, 10), o) }
func decorate(s string, o Options) string {
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	} else if o.IncludeSign {
		sign = "+"
	}
	if o.GroupThousands {
		end := strings.IndexByte(s, '.')
		if end < 0 {
			end = len(s)
		}
		var b strings.Builder
		b.Grow(len(s) + (end-1)/3)
		for i := 0; i < end; i++ {
			if i > 0 && (end-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteByte(s[i])
		}
		b.WriteString(s[end:])
		s = b.String()
	}
	s = sign + s
	if o.Unit != "" {
		s += " " + o.Unit
	}
	return s
}
