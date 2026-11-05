package scenes

import (
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
)

func streamlineFlowDemo() (*field.AdaptiveStreamLines, *field.Flow, error) {
	lines, err := adaptiveDemoLines()
	if err != nil {
		return nil, nil, err
	}
	options := field.DefaultFlowOptions()
	options.Duration, options.FlowSpeed, options.TraceTimeWidth, options.Loop = 5, 1.5, 1, true
	options.StartDelays = make([]float64, len(lines.Lines()))
	for i := range options.StartDelays {
		options.StartDelays[i] = float64(i) * .035
	}
	flow, err := lines.NewFlow(options)
	if err != nil {
		return nil, nil, err
	}
	return lines, flow, nil
}

// StreamlineFlow integrates once, then samples trace windows from actual
// nonuniform accepted timestamps without retaining solver callbacks.
func StreamlineFlow() (render.FrameSource, error) {
	lines, flow, err := streamlineFlowDemo()
	if err != nil {
		return nil, err
	}
	overlay := gomath.NewScene()
	heading, err := title("Streamline flow / physical-time traces on adaptive RK45 paths")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	options := text.DefaultOptions()
	options.Size = .28
	caption, err := text.New("12 seeds / explicit delays / 1 physical second of trailing trace\nAbsolute-time sampling / independent per-line loop clocks", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.8))
	if err = overlay.Add(lines.Object.Copy().SetOpacity(.12), heading, caption); err != nil {
		return nil, err
	}
	if err = overlay.Wait(flow.Duration()); err != nil {
		return nil, err
	}
	return &sourceOverlay{source: flow, overlay: overlay}, nil
}
