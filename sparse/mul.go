package sparse

import (
	"context"
	"fmt"
	"math/bits"
)

// A cursor merges one selected right-hand CSR row. Charge 24 bytes/slot even
// on 32-bit systems; no hash map or output-column-sized marker array is used.
type productCursor struct {
	position, end int
	factor        float64
}

// In addition to array payloads, SpGEMM reserves 16 KiB for its single fixed-
// precision accumulator and numeric temporaries. This is an accounting reserve,
// not a bound on allocator overhead, GC memory, or cumulative allocations.
const productScratchBytes uint64 = 16 << 10
const productCursorBytes uint64 = 24

// Mul returns A*B with default limits. Output rows have strictly increasing
// columns, no duplicates and no stored zeros. Inputs are never modified.
func (a *CSR) Mul(b *CSR) (*CSR, error) { return a.MulWithLimits(b, Limits{}) }

// MulWithLimits computes A*B under a per-call resource policy. Both operands
// must independently satisfy the policy. Output arrays, the sparse row heap and
// a numeric scratch reserve share MaxStorageBytes. MaxNNZ bounds both output
// count and cursor slots (the latter also cannot exceed a left row's NNZ).
// Product traversal work for both numeric passes is reserved before evaluation.
func (a *CSR) MulWithLimits(b *CSR, limits Limits) (*CSR, error) {
	return a.MulContext(context.Background(), b, limits)
}

// MulContext is MulWithLimits with cancellation; ctx must be nonnil. It merges
// sorted sparse rows with a bounded heap instead of allocating dense workspace.
// Exact sums of binary64 products use the same accumulator/range policy as
// MulVec: exact zero is omitted, while overflow or nonzero underflow on final
// component rounding returns ErrRange. Any error returns nil, never a partial
// matrix. Neither input nor any caller-owned slice is retained in the result.
func (a *CSR) MulContext(ctx context.Context, b *CSR, limits Limits) (*CSR, error) {
	if ctx == nil {
		return nil, ErrOptions
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	l, e := limits.normalized()
	if e != nil {
		return nil, e
	}
	if e = a.check(l); e != nil {
		return nil, e
	}
	if e = b.check(l); e != nil {
		return nil, e
	}
	if a.cols != b.rows {
		return nil, ErrShape
	}
	// Hard dimension/NNZ caps have already been checked. Still use checked count
	// helpers so work/storage arithmetic does not depend on native-int width.
	base, e := productAdd(uint64(a.rows), uint64(len(a.val)))
	if e != nil {
		return nil, e
	}
	base, e = productAdd(base, 1)
	if e != nil {
		return nil, e
	}
	base, e = productScale(base, 4)
	if e != nil {
		return nil, e
	}
	if e = l.work(base); e != nil {
		return nil, e
	}
	remaining := l.MaxWork - base
	maxSlots, scan := 0, 0
	for r := 0; r < a.rows; r++ {
		if e = tick(ctx, r); e != nil {
			return nil, e
		}
		slots := 0
		var pairs uint64
		for j := a.ptr[r]; j < a.ptr[r+1]; j++ {
			if e = tick(ctx, scan); e != nil {
				return nil, e
			}
			scan++
			if a.val[j] == 0 {
				continue
			}
			k := a.col[j]
			count := b.ptr[k+1] - b.ptr[k]
			if count == 0 {
				continue
			}
			slots++
			pairs, e = productAdd(pairs, uint64(count))
			if e != nil {
				return nil, e
			}
		}
		// Each pair is visited in both numeric passes. bits.Len(slots) bounds
		// heap height; the constant also covers bottom-up heap creation and sums.
		weight := uint64(2 * (8 + 2*bits.Len(uint(slots))))
		if pairs > remaining/weight {
			return nil, fmt.Errorf("%w: product work at row %d", ErrLimit, r)
		}
		remaining -= pairs * weight
		if slots > maxSlots {
			maxSlots = slots
		}
	}
	if uint64(maxSlots) > l.MaxNNZ {
		return nil, ErrLimit
	}
	pointers, e := productScale(uint64(a.rows)+1, 8)
	if e != nil {
		return nil, e
	}
	heapBytes, e := productScale(uint64(maxSlots), productCursorBytes)
	if e != nil {
		return nil, e
	}
	workspace, e := productAdd(pointers, heapBytes)
	if e != nil {
		return nil, e
	}
	if maxSlots > 0 {
		workspace, e = productAdd(workspace, productScratchBytes)
		if e != nil {
			return nil, e
		}
	}
	if e = l.storage(workspace); e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	p := make([]int, a.rows+1)
	if maxSlots == 0 {
		return &CSR{rows: a.rows, cols: b.cols, ptr: p}, nil
	}
	heap := make([]productCursor, maxSlots)
	acc := newAccumulator()
	n := 0
	e = a.productRows(ctx, b, heap, acc, func(r, c int, v float64) error {
		next, err := productAdd(uint64(n), 1)
		if err != nil {
			return err
		}
		if next > l.MaxNNZ {
			return ErrLimit
		}
		outputBytes, err := productScale(next, 16)
		if err != nil {
			return err
		}
		total, err := productAdd(workspace, outputBytes)
		if err != nil {
			return err
		}
		if err = l.storage(total); err != nil {
			return err
		}
		n++
		p[r+1]++
		return nil
	})
	if e != nil {
		return nil, e
	}
	for i := 1; i < len(p); i++ {
		if e = tick(ctx, i); e != nil {
			return nil, e
		}
		p[i] += p[i-1]
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	col, values := make([]int, n), make([]float64, n)
	index := 0
	e = a.productRows(ctx, b, heap, acc, func(_ int, c int, v float64) error { col[index] = c; values[index] = v; index++; return nil })
	if e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return &CSR{a.rows, b.cols, p, col, values}, nil
}

func productAdd(a, b uint64) (uint64, error) {
	if b > ^uint64(0)-a {
		return 0, ErrLimit
	}
	return a + b, nil
}
func productScale(a, b uint64) (uint64, error) {
	if b != 0 && a > ^uint64(0)/b {
		return 0, ErrLimit
	}
	return a * b, nil
}

func productLess(heap []productCursor, i, j int, columns []int) bool {
	a, b := heap[i].position, heap[j].position
	if columns[a] != columns[b] {
		return columns[a] < columns[b]
	}
	return a < b // deterministic tie order across participating right-hand rows
}
func productDown(heap []productCursor, parent int, columns []int) {
	for {
		left := 2*parent + 1
		if left >= len(heap) {
			return
		}
		child := left
		if right := left + 1; right < len(heap) && productLess(heap, right, left, columns) {
			child = right
		}
		if !productLess(heap, child, parent, columns) {
			return
		}
		heap[parent], heap[child] = heap[child], heap[parent]
		parent = child
	}
}
func (a *CSR) productRows(ctx context.Context, b *CSR, storage []productCursor, acc *accumulator, emit func(int, int, float64) error) error {
	scan, steps := 0, 0
	for r := 0; r < a.rows; r++ {
		if e := tick(ctx, r); e != nil {
			return e
		}
		heap := storage[:0]
		for j := a.ptr[r]; j < a.ptr[r+1]; j++ {
			if e := tick(ctx, scan); e != nil {
				return e
			}
			scan++
			if a.val[j] == 0 {
				continue
			}
			k := a.col[j]
			if b.ptr[k] < b.ptr[k+1] {
				heap = append(heap, productCursor{b.ptr[k], b.ptr[k+1], a.val[j]})
			}
		}
		for i := len(heap)/2 - 1; i >= 0; i-- {
			if e := tick(ctx, i); e != nil {
				return e
			}
			productDown(heap, i, b.col)
		}
		for len(heap) > 0 {
			c := b.col[heap[0].position]
			acc.reset()
			for len(heap) > 0 && b.col[heap[0].position] == c {
				if e := tick(ctx, steps); e != nil {
					return e
				}
				steps++
				acc.add(heap[0].factor, b.val[heap[0].position])
				heap[0].position++
				if heap[0].position == heap[0].end {
					heap[0] = heap[len(heap)-1]
					heap = heap[:len(heap)-1]
				}
				if len(heap) > 0 {
					productDown(heap, 0, b.col)
				}
			}
			v, e := rounded(&acc.s)
			if e != nil {
				return fmt.Errorf("%w: matrix product row %d column %d", e, r, c)
			}
			if v != 0 {
				if e = emit(r, c, v); e != nil {
					return e
				}
			}
		}
	}
	return ctx.Err()
}
