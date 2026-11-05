package render

import (
	"fmt"
	"gomath"
)

// ProjectFrame prepares a camera frame for raster or SVG rendering. Without a
// camera it returns the original frame and config without allocation or extra
// validation. Otherwise it validates config and camera, projects independent
// geometry retaining IDs, clears the projected camera, and replaces only
// Config.FrameWidth. The returned config must be used for every projection,
// clipping, stroke and bitmap operation for that frame, without persisting it
// into the caller's base config. Output height follows the pixel aspect ratio.
func ProjectFrame(frame *gomath.Frame, c Config) (*gomath.Frame, Config, error) {
	if frame == nil {
		return nil, c, fmt.Errorf("nil frame")
	}
	if frame.Camera == nil {
		return frame, c, nil
	}
	if err := c.Validate(); err != nil {
		return nil, c, err
	}
	projected, err := frame.ProjectCamera()
	if err != nil {
		return nil, c, err
	}
	effective := c
	effective.FrameWidth = frame.Camera.FrameWidth
	// Finite positive widths can still produce unusable floating-point scales.
	scale := float64(effective.Width) / effective.FrameWidth
	height := effective.FrameWidth * (float64(effective.Height) / float64(effective.Width))
	if !finite(scale) || scale <= 0 || !finite(height) || height <= 0 {
		return nil, c, fmt.Errorf("camera width exceeds render coordinate range")
	}
	return projected, effective, nil
}
