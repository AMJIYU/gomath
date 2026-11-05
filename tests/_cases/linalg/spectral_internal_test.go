package linalg

import (
	"errors"
	"math"
	"slices"
	"testing"
)

// Private overlay tests verify actual backing-array ownership, rather than
// testing only Data(), which always returns its own copy.
func TestSpectralInternalOwnership(t *testing.T) {
	a, _ := NewDense(2, 2, []float64{2, 1, 1, 2})
	before := a.Data()
	e, err := FactorizeEigenSym(a)
	if err != nil {
		t.Fatal(err)
	}
	s, err := FactorizeSVD(a)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := e.Vectors()
	u, w, _ := s.Factors()
	if &s.u[0] == &s.v[0] || &u.data[0] == &w.data[0] || &a.data[0] == &s.u[0] || &a.data[0] == &s.original[0] || &a.data[0] == &e.vectors[0] {
		t.Fatal("factors alias each other or input")
	}
	v.data[0], u.data[0], w.data[0] = math.NaN(), math.NaN(), math.NaN()
	v2, _ := e.Vectors()
	u2, w2, _ := s.Factors()
	if !finite(v2.data[0]) || !finite(u2.data[0]) || !finite(w2.data[0]) || !slices.Equal(a.data, before) {
		t.Fatal("exported factor storage aliases internal state")
	}
	a.data[0] = 99
	if e.values[0] != 3 || math.Abs(s.values[0]-3) > 1e-14 {
		t.Fatal("factor retained input storage")
	}
}

func TestSVDOrthogonalCompletionBound(t *testing.T) {
	// Deliberately impossible completion after a basis already spans R^2:
	// the helper must exhaust its finite coordinate budget and return an error.
	u := []float64{1, 0, 0, 0, 1, 0}
	if err := completeSVDColumn(u, 2, 3, 2); !errors.Is(err, ErrConvergence) {
		t.Fatal("unbounded or invalid orthogonal completion", err)
	}
	// A valid candidate may appear only after rejecting occupied coordinates.
	u = []float64{1, 0, 0, 0, 1, 0, 0, 0, 0}
	if err := completeSVDColumn(u, 3, 3, 2); err != nil || !slices.Equal(u, []float64{1, 0, 0, 0, 1, 0, 0, 0, 1}) {
		t.Fatal("bounded completion", u, err)
	}
}

func TestSVDPrivateInverseAndSolveOwnership(t *testing.T) {
	a, _ := NewDense(2, 3, []float64{1, 0, 1, 0, 1, 1})
	f, err := FactorizeSVD(a)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewDense(2, 1, []float64{1, 2})
	p, _ := f.PseudoInverse()
	x, _, _ := f.SolveMatrix(b)
	pwant, xwant := p.Data(), x.Data()
	a.data[0], p.data[0], x.data[0] = 99, math.NaN(), math.NaN()
	p2, _ := f.PseudoInverse()
	x2, _, _ := f.SolveMatrix(b)
	if !slices.Equal(p2.data, pwant) || !slices.Equal(x2.data, xwant) || b.data[0] != 1 {
		t.Fatal("original/exported input or output storage aliases factors")
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		b.data[0] = bad
		if x, r, err := f.SolveMatrix(b); x != nil || r != nil || !errors.Is(err, ErrNonFinite) {
			t.Fatal("nonfinite private RHS", x, r, err)
		}
	}
}

func TestSpectralInternalNonFinite(t *testing.T) {
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, data := range [][]float64{{bad, 0, 0, 1}, {1, bad, bad, 1}, {1, 0, bad, 1}} {
			a := &Dense{rows: 2, cols: 2, data: data}
			if f, err := FactorizeEigenSym(a); f != nil || !errors.Is(err, ErrNonFinite) {
				t.Fatal("eigen nonfinite", f, err)
			}
			if f, err := FactorizeSVD(a); f != nil || !errors.Is(err, ErrNonFinite) {
				t.Fatal("SVD nonfinite", f, err)
			}
		}
	}
}
