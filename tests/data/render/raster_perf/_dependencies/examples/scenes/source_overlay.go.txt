package scenes

import (
	"gomath"
	"gomath/audio"
	"gomath/render"
	"gomath/subtitle"
)

type sourceOverlay struct {
	source  render.FrameSource
	overlay *gomath.Scene
}

func (s *sourceOverlay) Duration() float64 { return s.source.Duration() }

// The ordinary overlay Scene supplies the same media clock as the sampled source.
// Finish these authoring calls before any concurrent sampling/rendering.
func (s *sourceOverlay) AddSound(clip *audio.PCM, offset float64) error {
	return s.overlay.AddSound(clip, offset)
}
func (s *sourceOverlay) Soundtrack(format audio.Format, opt gomath.SoundtrackOptions) (*audio.PCM, error) {
	return s.overlay.Soundtrack(format, opt)
}
func (s *sourceOverlay) Subcaptions(opt subtitle.Options) (*subtitle.Track, error) {
	return s.overlay.Subcaptions(opt)
}
func (s *sourceOverlay) Sample(time float64) (*gomath.Frame, error) {
	frame, err := s.source.Sample(time)
	if err != nil {
		return nil, err
	}
	overlay, err := s.overlay.Sample(time)
	if err != nil {
		return nil, err
	}
	frame.Objects = append(frame.Objects, overlay.Objects...)
	return frame, nil
}
