package svg

import "gomath/path"

const (
	maxClipUnionChildren        = 16
	maxClipUnionInputs          = 128
	maxClipUnionSourceSegments  = 512
	maxClipUnionSourceContours  = 128
	maxClipUnionOperandSegments = 256
	maxClipUnionOperandContours = 128
	maxClipUnionCalls           = 32
	maxClipUnionAttributeBytes  = 64 << 10
)

// No backend usage statistics are exposed. Each call conservatively reserves
// its ENTIRE allowed budget, including unsuccessful calls; unused reservations
// are never returned. One compiler and ledger span every definition in a Read.
// At most 32 calls reserve 8M work, 1M pairs, 512MiB allocation charges,
// 32768 vertices/atomic edges and 65536 split references in aggregate.
type clipUnionBudget struct {
	inputs, segments, contours, calls int
	plans                             map[*element]bool
}

func clipUnionLimits() path.BooleanLimits {
	return path.BooleanLimits{Contours: 128, Segments: 256, Pairs: 32768, Vertices: 1024, Edges: 1024, SplitPoints: 2048, Work: 250000, RationalBits: 1024, AllocationBytes: 16 << 20}
}

type clipUnionInput struct {
	outline path.Path
	rule    path.FillRule
	hidden  bool
}
