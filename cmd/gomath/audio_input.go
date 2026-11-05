package main

import (
	"bufio"
	"fmt"
	"gomath/audio"
	"io"
	"os"
)

// readAudioInput selects by native stream signature, independent of filename.
// All readers verify a complete bounded stream before returning immutable PCM.
func readAudioInput(path string, discardMetadata bool) (*audio.PCM, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	header, err := reader.Peek(4)
	if err != nil {
		return nil, fmt.Errorf("audio input header: %w", err)
	}
	var clip *audio.PCM
	switch string(header) {
	case "RIFF":
		clip, err = audio.ReadWAV(reader, audio.WAVReadOptions{DiscardMetadata: discardMetadata})
	case "fLaC":
		clip, err = audio.ReadFLAC(reader, audio.FLACReadOptions{DiscardMetadata: discardMetadata})
	case "FORM":
		clip, err = audio.ReadAIFF(reader, audio.AIFFReadOptions{DiscardMetadata: discardMetadata})
	case ".snd":
		clip, err = audio.ReadAU(reader, audio.AUReadOptions{DiscardMetadata: discardMetadata})
	default:
		return nil, fmt.Errorf("audio input requires native WAV, FLAC, AIFF/AIFC or known-size AU")
	}
	if err != nil {
		return nil, err
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("audio input trailer: %w", err)
		}
		return nil, fmt.Errorf("audio input contains trailing data after its native stream")
	}
	return clip, nil
}

// remixAudioInput implements only the CLI's explicitly requested conventional
// mono/stereo routing. Library callers can choose arbitrary matrices via Remix.
// Rate conversion is performed later by Soundtrack, preserving time zero.
func remixAudioInput(clip *audio.PCM, channels int) (*audio.PCM, error) {
	if clip == nil || clip.SampleRate() == 0 || (channels != 1 && channels != 2) {
		return nil, fmt.Errorf("audio remix requires valid PCM and mono/stereo output")
	}
	input := clip.Format()
	if input.Channels != 1 && input.Channels != 2 {
		return nil, fmt.Errorf("audio remix supports only mono/stereo input")
	}
	mask := uint32(4)
	if input.Channels == 2 {
		mask = 3
	}
	if input.ChannelMask != 0 && input.ChannelMask != mask {
		return nil, fmt.Errorf("audio remix requires unspecified or conventional mono/stereo speaker masks")
	}
	if channels == input.Channels {
		return clip, nil // PCM is immutable; no numerical conversion is needed.
	}
	output := audio.Format{SampleRate: input.SampleRate, Channels: channels, ChannelMask: 4}
	matrix := [][]float64{{.5, .5}}
	if channels == 2 {
		output.ChannelMask = 3
		matrix = [][]float64{{1}, {1}}
	}
	return clip.Remix(output, matrix, audio.RemixOptions{})
}
