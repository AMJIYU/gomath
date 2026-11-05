// Package render implements deterministic, headless, pure-Go raster rendering.
package render

import (
	"fmt"
	"gomath"
	"math"
	"runtime"
)

type Config struct {
	Width, Height, FPS, Workers int
	FrameWidth                  float64
	Background                  gomath.Color
	// CurveTolerance is the flattening error bound in pixels. Zero uses 0.02.
	CurveTolerance float64
	// Supersampling renders the complete 2D frame at 1, 2, 4 or 8 times each
	// output dimension, then box-resolves premultiplied RGBA. Zero means 1.
	// Scales above 1 have explicit working-pixel and configured-worker budgets.
	Supersampling int
}

func DefaultConfig() Config {
	return Config{Supersampling: 1, Width: 854, Height: 480, FPS: 30, Workers: max(1, runtime.GOMAXPROCS(0)/2), FrameWidth: 14.2222222222, Background: gomath.RGB(.035, .045, .075)}
}
func (c Config) Validate() error {
	if math.IsNaN(c.CurveTolerance) || math.IsInf(c.CurveTolerance, 0) || c.CurveTolerance < 0 || c.CurveTolerance > 1 {
		return fmt.Errorf("curve tolerance must be in [0,1] pixels")
	}
	if c.Width <= 0 || c.Height <= 0 || c.Width > 16384 || c.Height > 16384 {
		return fmt.Errorf("frame dimensions must be in [1,16384]")
	}
	if c.FPS <= 0 || c.FPS > 1000 {
		return fmt.Errorf("FPS must be in [1,1000]")
	}
	if c.Workers <= 0 || c.Workers > 256 {
		return fmt.Errorf("workers must be in [1,256]")
	}
	if c.FrameWidth <= 0 || math.IsNaN(c.FrameWidth) || math.IsInf(c.FrameWidth, 0) {
		return fmt.Errorf("frame width must be finite and positive")
	}
	for _, v := range []float64{c.Background.R, c.Background.G, c.Background.B, c.Background.A} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("background channels must be in [0,1]")
		}
	}
	_, err := c.samplingConfig()
	return err
}
