package export

import (
	"fmt"
	"gomath/subtitle"
)

// Subtitles atomically publishes SRT or plain-text WebVTT as a separate file.
// Unsupported visible markup/settings are errors in the strict subtitle codec.
// MOV/MP4 do not embed this track or burn its text into video pixels.
func Subtitles(target string, track *subtitle.Track, format subtitle.Format, opt subtitle.WriteOptions) error {
	if format != subtitle.SRT && format != subtitle.WebVTT {
		return fmt.Errorf("unsupported subtitle output format %d", format)
	}
	out, err := newAtomicFile(target)
	if err != nil {
		return err
	}
	defer out.abort()
	if format == subtitle.SRT {
		err = subtitle.WriteSRT(out.file, track, opt)
	} else {
		err = subtitle.WriteWebVTT(out.file, track, opt)
	}
	if err != nil {
		return err
	}
	return out.commit()
}
