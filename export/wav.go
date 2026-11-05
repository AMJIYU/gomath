package export

import "gomath/audio"

// WAV publishes a complete WAVE file atomically without replacing an existing
// output. The pure-Go audio writer validates encoding, sample and byte limits.
func WAV(target string, pcm *audio.PCM, opt audio.WAVWriteOptions) error {
	out, err := newAtomicFile(target)
	if err != nil {
		return err
	}
	defer out.abort()
	if err = audio.WriteWAV(out.file, pcm, opt); err != nil {
		return err
	}
	return out.commit()
}
