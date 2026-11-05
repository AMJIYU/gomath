package field_test

import (
	"fmt"
	"gomath"
	"gomath/field"
	"gomath/geom"
)

func ExampleNewVectorField() {
	options := field.DefaultVectorOptions()
	options.ColorFunc, _ = field.SpeedColor(gomath.Blue, gomath.Red, 0, 2)
	rotation := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	v, err := field.NewVectorField(rotation, field.Range{Min: -1, Max: 1, Step: 1}, field.Range{Min: -1, Max: 1, Step: 1}, options)
	if err != nil {
		panic(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(v.Object); err != nil {
		panic(err)
	}
	fmt.Println(len(v.Samples()), len(v.Object.Children))
	// Output: 9 8
}

func ExampleNewStreamLines() {
	options := field.DefaultStreamOptions()
	options.StepSize, options.MaxSteps = .1, 10
	s, err := field.NewStreamLines(func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }, []geom.Vec3{geom.V(0, 0)}, options)
	if err != nil {
		panic(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(s.Object); err != nil {
		panic(err)
	}
	line := s.Lines()[0]
	fmt.Println(line.Steps, line.Reason)
	fmt.Printf("x=%.3f\n", line.Points[len(line.Points)-1].X)
	// Output:
	// 10 step_limit
	// x=1.000
}
