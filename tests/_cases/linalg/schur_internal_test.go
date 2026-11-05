package linalg

import (
	"errors"
	"math"
	"slices"
	"testing"
)

func TestSchurInternalOwnershipAndNonfinite(t *testing.T) {
	a, _ := NewDense(3, 3, []float64{1, 2, 3, 4, 5, 6, 7, 8, 10})
	f, err := FactorizeSchur(a)
	if err != nil {
		t.Fatal(err)
	}
	q, s, _ := f.Factors()
	if &f.q[0] == &q.data[0] || &f.t[0] == &s.data[0] || &f.q[0] == &a.data[0] || &f.t[0] == &a.data[0] || &f.q[0] == &f.t[0] {
		t.Fatal("shared backing storage")
	}
	qwant, twant := slices.Clone(f.q), slices.Clone(f.t)
	a.data[0], q.data[0], s.data[0] = math.NaN(), math.NaN(), math.NaN()
	if !slices.Equal(f.q, qwant) || !slices.Equal(f.t, twant) {
		t.Fatal("mutable exports/input retained")
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		a.data[0] = bad
		if f, err := FactorizeSchur(a); f != nil || !errors.Is(err, ErrNonFinite) {
			t.Fatal("nonfinite internal input", f, err)
		}
	}
}

func TestSchurWorkAndPostconditions(t *testing.T) {
	a, _ := NewDense(3, 3, []float64{1, 2, 3, 4, 5, 6, 7, 8, 10})
	for _, work := range []uint64{153, 154, 300} { // checks reserve=3*n^3+8*n^2=153
		opt := DefaultSchurOptions()
		opt.MaxWork = work
		if f, err := FactorizeSchurWithOptions(a, opt); f != nil || !errors.Is(err, ErrSchurLimit) {
			t.Fatal("dynamic work budget", work, f, err)
		}
	}
	for _, which := range []string{"reconstruction", "orthogonality"} {
		opt := DefaultSchurOptions()
		if which == "reconstruction" {
			opt.ReconstructionTolerance = 0
		} else {
			opt.OrthogonalityTolerance = 0
		}
		f, err := FactorizeSchurWithOptions(a, opt)
		var validation *SchurValidationError
		if f != nil || !errors.Is(err, ErrReconstruction) || !errors.As(err, &validation) || validation.Check != which || validation.Error() == "" {
			t.Fatal("postcondition", which, f, err)
		}
	}
	for _, n := range []int{0, 1, 2, 4} {
		identity, _ := Identity(n)
		f, err := FactorizeSchurWithOptions(identity, SchurOptions{})
		if err != nil || f.Iterations() != 0 || f.RelativeReconstructionError() != 0 || f.OrthogonalityError() != 0 {
			t.Fatal("literal zero options", n, err)
		}
	}
	identity, _ := Identity(2)
	opt := DefaultSchurOptions()
	opt.MaxWork = 62
	if _, err := FactorizeSchurWithOptions(identity, opt); err != nil {
		t.Fatal("exact initial/scan budget", err)
	}
	opt.MaxWork--
	if f, err := FactorizeSchurWithOptions(identity, opt); f != nil || !errors.Is(err, ErrSchurLimit) {
		t.Fatal("scan budget", f, err)
	}
	pair, _ := NewDense(2, 2, []float64{0, 1, 1, 0})
	opt = DefaultSchurOptions()
	opt.MaxWork = 115
	opt.MaxIterations = 0
	if f, err := FactorizeSchurWithOptions(pair, opt); err != nil || f.Iterations() != 0 {
		t.Fatal("exact direct rotation budget", err)
	}
	opt.MaxWork--
	if f, err := FactorizeSchurWithOptions(pair, opt); f != nil || !errors.Is(err, ErrSchurLimit) {
		t.Fatal("rotation budget", f, err)
	}
	large, _ := Zeros(257, 257)
	if f, err := FactorizeSchur(large); f != nil || !errors.Is(err, ErrSchurLimit) {
		t.Fatal("default size bound", f, err)
	}
	if got := schurDeflate([]float64{1, 1, 1e-20, 1}, 2, 1, 0); got {
		t.Fatal("zero tolerance deflated nonzero")
	}
}
