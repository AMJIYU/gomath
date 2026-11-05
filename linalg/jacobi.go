package linalg

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var (
	ErrConvergence = errors.New("linalg: Jacobi sweep budget exhausted")
	ErrBudget      = errors.New("linalg: invalid Jacobi sweep budget")
)

// ConvergenceError reports a bounded iteration failure. Residual is the
// maximum relative off-diagonal entry for EigenSym, or the maximum absolute
// normalized column correlation for SVD. No partial factors are returned.
type ConvergenceError struct {
	Algorithm           string
	Sweeps, MaxSweeps   int
	Residual, Tolerance float64
}

func (e *ConvergenceError) Error() string {
	return fmt.Sprintf("%v: %s after %d/%d sweeps, residual %g, tolerance %g", ErrConvergence, e.Algorithm, e.Sweeps, e.MaxSweeps, e.Residual, e.Tolerance)
}

func (e *ConvergenceError) Unwrap() error { return ErrConvergence }

func validateJacobi(tolerance float64, sweeps int) error {
	if !finite(tolerance) || tolerance < 0 || tolerance >= 1 {
		return fmt.Errorf("%w: Jacobi tolerance must be finite and in [0,1)", ErrTolerance)
	}
	if sweeps < 0 {
		return fmt.Errorf("%w: negative sweep count %d", ErrBudget, sweeps)
	}
	return nil
}

// jacobiRotation diagonalizes a symmetric 2-by-2 pair with half the diagonal
// difference delta and off-diagonal cross. Inputs are scaled by the caller.
// The chosen |t|<=1 avoids an overflowing cotangent or squared cotangent.
func jacobiRotation(delta, cross float64) (c, s, t float64) {
	if delta == 0 {
		t = math.Copysign(1, cross)
	} else {
		t = cross / (delta + math.Copysign(math.Hypot(delta, cross), delta))
	}
	c = 1 / math.Hypot(1, t)
	s = t * c
	return c, s, t
}

// rotateColumns applies a right plane rotation directly to separate columns.
func rotateColumns(data []float64, rows, cols, p, q int, c, s float64) error {
	for i := range rows {
		ip, iq := i*cols+p, i*cols+q
		x, y := data[ip], data[iq]
		data[ip], data[iq] = math.FMA(c, x, -s*y), math.FMA(s, x, c*y)
		if !finite(data[ip]) || !finite(data[iq]) {
			return fmt.Errorf("%w: Jacobi column rotation at row %d", ErrNonFinite, i)
		}
	}
	return nil
}

func descendingOrder(values []float64) []int {
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return values[order[i]] > values[order[j]] })
	return order
}

func orderedColumns(data []float64, rows, cols int, order []int) []float64 {
	out := make([]float64, len(data))
	for i := range rows {
		for j, old := range order {
			out[i*cols+j] = data[i*cols+old]
		}
	}
	return out
}
