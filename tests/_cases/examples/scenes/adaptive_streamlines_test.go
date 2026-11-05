package scenes

import (
	"gomath/field"
	"math"
	"testing"
)

func TestAdaptiveStreamlineDemoUsesAcceptedTimesAndAnalyticStates(t *testing.T) {
	lines, err := adaptiveDemoLines()
	if err != nil {
		t.Fatal(err)
	}
	stats := lines.Stats()
	if len(lines.Lines()) != 12 || stats.AcceptedSteps < 24 || stats.RejectedSteps == 0 || stats.Evaluations <= stats.AcceptedSteps {
		t.Fatal("adaptive example did not exercise actual RK45 accounting", stats)
	}
	for _, line := range lines.Lines() {
		if line.Result.Reason() != field.TimeComplete || line.Result.Time() != 6 {
			t.Fatal("unexpected termination", line.Result.Reason())
		}
		times, points := line.Result.Times(), line.Result.Points()
		nonuniform := false
		for i := 2; i < len(times); i++ {
			if math.Abs((times[i]-times[i-1])-(times[1]-times[0])) > 1e-8 {
				nonuniform = true
			}
		}
		if !nonuniform || len(line.Object.Path.Contours[0].Curves) != len(points)-1 {
			t.Fatal("adaptive numerical times/geometry replaced by fixed steps")
		}
	}
}
