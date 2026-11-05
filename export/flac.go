package export

import "gomath/audio"

// FLAC atomically publishes native lossless FLAC without replacing an existing
// output. Losslessness is relative to the selected PCM16/24 quantization.
func FLAC(target string, pcm *audio.PCM, opt audio.FLACWriteOptions) error {
	out, err := newAtomicFile(target)
	if err != nil {
		return err
	}
	defer out.abort()
	if err = audio.WriteFLAC(out.file, pcm, opt); err != nil {
		return err
	}
	return out.commit()
}
