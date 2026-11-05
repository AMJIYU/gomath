package linalg_test

import (
	"encoding/json"
	"math"
	"math/cmplx"
	"os"
	"testing"

	"gomath/linalg"
)

func TestGeneralEigenSciPyDirectionsAndSubspaces(t *testing.T) {
	data, err := os.ReadFile("eigen_scipy.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		NumPy, SciPy string
		Cases        []struct {
			Name, Mode       string
			N                int
			Data             []float64
			Scale, Tolerance float64
			Values           [][2]float64
			Vectors          [][][2]float64
			Groups           []struct {
				Value     [2]float64
				Dimension int
				Projector [][][2]float64
			}
		}
	}
	if err = json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Cases) != 30 || file.NumPy == "" || file.SciPy == "" {
		t.Fatal("reference inventory/version")
	}
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			a, _ := linalg.NewDense(tc.N, tc.N, tc.Data)
			f, err := linalg.FactorizeEigen(a)
			if err != nil {
				t.Fatal(err)
			}
			checkGeneralEigen(t, a, f)
			if f.BasisRank() != tc.N {
				t.Fatalf("well-conditioned reference basis rank %d/%d", f.BasisRank(), tc.N)
			}
			used := make([]bool, tc.N)
			for col, lambda := range f.Values() {
				scaled := complex(real(lambda)/tc.Scale, imag(lambda)/tc.Scale)
				match, distance := -1, math.Inf(1)
				for j, w := range tc.Values {
					d := cmplx.Abs(scaled - complex(w[0], w[1]))
					if !used[j] && d < distance {
						match, distance = j, d
					}
				}
				if match < 0 || distance > tc.Tolerance*(1+cmplx.Abs(scaled)) {
					t.Fatalf("eigenvalue %v reference gap %g", scaled, distance)
				}
				used[match] = true
				v, _ := f.RightVector(col)
				if tc.Mode == "directions" {
					var inner complex128
					for i, z := range tc.Vectors[match] {
						inner += cmplx.Conj(complex(z[0], z[1])) * v[i]
					}
					phase := inner / complex(cmplx.Abs(inner), 0)
					difference := 0.0
					for i, z := range tc.Vectors[match] {
						difference = math.Hypot(difference, cmplx.Abs(v[i]-phase*complex(z[0], z[1])))
					}
					if difference > tc.Tolerance {
						t.Fatalf("direction differs from SciPy by %g (lambda %v)", difference, scaled)
					}
				} else {
					group := -1
					for j, g := range tc.Groups {
						if cmplx.Abs(scaled-complex(g.Value[0], g.Value[1])) < tc.Tolerance {
							group = j
							break
						}
					}
					if group < 0 {
						t.Fatal("unmatched repeated eigenspace")
					}
					difference := 0.0
					for i, row := range tc.Groups[group].Projector {
						sum := -v[i]
						for j, z := range row {
							sum += complex(z[0], z[1]) * v[j]
						}
						difference = math.Hypot(difference, cmplx.Abs(sum))
					}
					if difference > tc.Tolerance {
						t.Fatalf("outside independent repeated-root subspace by %g", difference)
					}
				}
			}
			if tc.Mode == "subspaces" {
				for _, g := range tc.Groups {
					count := 0
					for _, lambda := range f.Values() {
						if cmplx.Abs(lambda/complex(tc.Scale, 0)-complex(g.Value[0], g.Value[1])) < tc.Tolerance {
							count++
						}
					}
					if count != g.Dimension {
						t.Fatalf("subspace dimension %d want %d", count, g.Dimension)
					}
				}
			}
		})
	}
}
