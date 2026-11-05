package h264

// Forward 4x4 integer transform. Coefficients use row-major storage; frame
// scanning is deferred until entropy coding. See H.264 8.5.12 for the inverse.
func forward4(v [16]int) [16]int {
	var tmp, out [16]int
	for y := range 4 {
		i := 4 * y
		a, b := v[i]+v[i+3], v[i+1]+v[i+2]
		c, d := v[i+1]-v[i+2], v[i]-v[i+3]
		tmp[i], tmp[i+1], tmp[i+2], tmp[i+3] = a+b, 2*d+c, a-b, d-2*c
	}
	for x := range 4 {
		a, b := tmp[x]+tmp[x+12], tmp[x+4]+tmp[x+8]
		c, d := tmp[x+4]-tmp[x+8], tmp[x]-tmp[x+12]
		out[x], out[x+4], out[x+8], out[x+12] = a+b, 2*d+c, a-b, d-2*c
	}
	return out
}

func inverse4(v [16]int) [16]int {
	var tmp, out [16]int
	for y := range 4 {
		i := 4 * y
		a, b := v[i]+v[i+2], v[i]-v[i+2]
		c, d := (v[i+1]>>1)-v[i+3], v[i+1]+(v[i+3]>>1)
		tmp[i], tmp[i+1], tmp[i+2], tmp[i+3] = a+d, b+c, b-c, a-d
	}
	for x := range 4 {
		a, b := tmp[x]+tmp[x+8], tmp[x]-tmp[x+8]
		c, d := (tmp[x+4]>>1)-tmp[x+12], tmp[x+4]+(tmp[x+12]>>1)
		out[x], out[x+4], out[x+8], out[x+12] = (a+d+32)>>6, (b+c+32)>>6, (b-c+32)>>6, (a-d+32)>>6
	}
	return out
}

// Flat scaling lists (all 16), as inferred for Baseline PPS syntax. Positions
// are classified as even/even, odd/odd, or mixed; see H.264 8.5.9.
var inverseScale = [6][3]int{
	{10, 16, 13}, {11, 18, 14}, {13, 20, 16},
	{14, 23, 18}, {16, 25, 20}, {18, 29, 23},
}

// Fixed-point reciprocals of the inverse scaling and integer-transform gains.
// Intra rounding uses a one-third quantization dead zone.
var forwardScale = [6][3]int{
	{13107, 5243, 8066}, {11916, 4660, 7490}, {10082, 4194, 6554},
	{9362, 3647, 5825}, {8192, 3355, 5243}, {7282, 2893, 4559},
}

var frameScan = [16]int{0, 1, 4, 8, 5, 2, 3, 6, 9, 12, 13, 10, 7, 11, 14, 15}

func scaleClass(i int) int {
	x, y := i%4, i/4
	if x%2 == 0 && y%2 == 0 {
		return 0
	}
	if x%2 == 1 && y%2 == 1 {
		return 1
	}
	return 2
}

func quantize(v, multiplier, shift int) int {
	level := (absInt(v)*multiplier + (1<<shift)/3) >> shift
	if v < 0 {
		return -level
	}
	return level
}

func quant4(v [16]int, qp int) [16]int {
	for i := range v {
		v[i] = quantize(v[i], forwardScale[qp%6][scaleClass(i)], 15+qp/6)
	}
	return v
}

func dequant4(v [16]int, qp int) [16]int {
	for i := range v {
		v[i] *= inverseScale[qp%6][scaleClass(i)] << (qp / 6)
	}
	return v
}

// The 2x2 chroma DC transform is its own inverse up to a factor of four.
func hadamard2(v [4]int) [4]int {
	return [4]int{v[0] + v[1] + v[2] + v[3], v[0] - v[1] + v[2] - v[3],
		v[0] + v[1] - v[2] - v[3], v[0] - v[1] - v[2] + v[3]}
}

func chromaQP(qp int) int {
	if qp < 30 {
		return qp
	}
	return [...]int{29, 30, 31, 32, 32, 33, 34, 34, 35, 35, 36, 36, 37, 37, 37, 38, 38, 38, 39, 39, 39, 39}[qp-30]
}
