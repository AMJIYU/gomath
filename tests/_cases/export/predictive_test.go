package export_test

import (
	"bytes"
	"encoding/binary"
	"gomath/export"
	"gomath/internal/h264"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestPredictiveMP4SyncSamplesAndFailedInputRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "predictive.mp4")
	options := export.DefaultMP4Options()
	options.Codec, options.GOP, options.MaxFrames = export.H264Predictive, 3, 7
	m, err := export.NewMP4WithOptions(path, 66, 34, 24, options)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Abort()
	// Neutral YUV=128 is an exact DC reconstruction at this QP. Arbitrary
	// repeated lossy input is deliberately not assumed to skip.
	black := image.NewRGBA(image.Rect(0, 0, 66, 34))
	for y := range 34 {
		for x := range 66 {
			black.SetRGBA(x, y, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		}
	}
	for i := range 7 {
		if err = m.WriteFrame(i, image.NewRGBA(image.Rect(0, 0, 32, 32))); err == nil {
			t.Fatal("invalid dimensions accepted")
		}
		if err = m.WriteFrame(i+1, black); err == nil {
			t.Fatal("out-of-order input accepted")
		}
		if err = m.WriteFrame(i, black); err != nil {
			t.Fatal(err)
		}
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	top := boxes(t, data)
	mdat := find(t, top, "mdat")
	moov := find(t, top, "moov")
	trak := find(t, boxes(t, moov.payload), "trak")
	mdia := find(t, boxes(t, trak.payload), "mdia")
	minf := find(t, boxes(t, mdia.payload), "minf")
	stbl := find(t, boxes(t, minf.payload), "stbl")
	tables := boxes(t, stbl.payload)
	stss := find(t, tables, "stss")
	if binary.BigEndian.Uint32(stss.payload[4:8]) != 3 {
		t.Fatal("wrong keyframe count")
	}
	for i, want := range []uint32{1, 4, 7} {
		if binary.BigEndian.Uint32(stss.payload[8+i*4:]) != want {
			t.Fatal("wrong one-based sync index")
		}
	}
	stsz := find(t, tables, "stsz")
	pos := 0
	for i := range 7 {
		size := int(binary.BigEndian.Uint32(stsz.payload[12+4*i:]))
		if size < 5 || pos+size > len(mdat.payload) {
			t.Fatal("bad sample range")
		}
		nal := mdat.payload[pos+4] & 31
		if (i%3 == 0 && nal != 5) || (i%3 != 0 && nal != 1) {
			t.Fatal("sync table differs from picture coding", i, nal)
		}
		if i%3 != 0 && size >= 32 {
			t.Fatal("exact reconstructed neutral frame did not skip", i, size)
		}
		pos += size
	}
	if pos != len(mdat.payload) {
		t.Fatal("sample table does not cover media")
	}
}

func TestMP4IntegerMotionSearchReachesContainerAndSyncTables(t *testing.T) {
	for _, radius := range []int{1, 2, 4} {
		t.Run(strconv.Itoa(radius), func(t *testing.T) {
			options := export.DefaultMP4Options()
			options.Codec, options.InterResidual, options.MotionSearchRadius, options.GOP = export.H264Predictive, true, radius, 3
			output := filepath.Join(t.TempDir(), "motion.mp4")
			writer, err := export.NewMP4WithOptions(output, 66, 34, 24, options)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Abort()
			encoder, err := h264.NewPredictive(66, 34, 24, h264.PredictiveOptions{QP: 24, GOP: 3, InterResidual: true, MotionSearchRadius: radius})
			if err != nil {
				t.Fatal(err)
			}
			zero, err := h264.NewPredictive(66, 34, 24, h264.PredictiveOptions{QP: 24, GOP: 3, InterResidual: true})
			if err != nil {
				t.Fatal(err)
			}
			var expected []byte
			different := false
			for f := range 8 {
				img := image.NewRGBA(image.Rect(0, 0, 66, 34))
				for y := range 34 {
					for x := range 66 {
						sx := max(0, x-f)
						v := uint8(50 + (sx*17+y*23+sx*y)%150)
						img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
					}
				}
				frame, err := encoder.Encode(img)
				if err != nil {
					t.Fatal(err)
				}
				baseline, err := zero.Encode(img)
				if err != nil {
					t.Fatal(err)
				}
				different = different || !bytes.Equal(frame.NAL, baseline.NAL)
				expected = binary.BigEndian.AppendUint32(expected, uint32(len(frame.NAL)))
				expected = append(expected, frame.NAL...)
				if err = writer.WriteFrame(f, img); err != nil {
					t.Fatal(err)
				}
			}
			if !different {
				t.Fatal("moving fixture does not distinguish enabled motion search")
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			top := boxes(t, data)
			if !bytes.Equal(find(t, top, "mdat").payload, expected) {
				t.Fatal("motion option dropped or changed in container")
			}
			moov := find(t, top, "moov")
			trak := find(t, boxes(t, moov.payload), "trak")
			mdia := find(t, boxes(t, trak.payload), "mdia")
			minf := find(t, boxes(t, mdia.payload), "minf")
			stbl := find(t, boxes(t, minf.payload), "stbl")
			stss := find(t, boxes(t, stbl.payload), "stss")
			if binary.BigEndian.Uint32(stss.payload[4:8]) != 3 {
				t.Fatal("motion changed keyframe count")
			}
			for i, want := range []uint32{1, 4, 7} {
				if binary.BigEndian.Uint32(stss.payload[8+i*4:]) != want {
					t.Fatal("motion changed one-based sync index")
				}
			}
		})
	}
}

func TestPredictiveMP4Options(t *testing.T) {
	for _, o := range []export.MP4Options{
		{Codec: export.H264Predictive, GOP: -1},
		{Codec: export.H264Predictive, GOP: 65537},
		{Codec: export.H264Predictive, QP: 52},
		{Codec: export.H264Intra, GOP: 3},
		{Codec: export.H264PCM, GOP: 3},
		{Codec: export.H264Intra, InterResidual: true},
		{Codec: export.H264PCM, InterResidual: true},
		{Codec: export.H264Predictive, InterResidual: true, MotionSearchRadius: -1},
		{Codec: export.H264Predictive, InterResidual: true, MotionSearchRadius: 5},
		{Codec: export.H264Predictive, MotionSearchRadius: 1},
		{Codec: export.H264Intra, MotionSearchRadius: 1},
		{Codec: export.H264PCM, MotionSearchRadius: 1},
	} {
		dir := t.TempDir()
		if m, err := export.NewMP4WithOptions(filepath.Join(dir, "bad.mp4"), 32, 32, 24, o); err == nil || m != nil {
			t.Fatal("invalid predictive options accepted", o)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatal("invalid options leaked output")
		}
	}
}

func TestMP4InterResidualReachesVideoSamples(t *testing.T) {
	options := export.DefaultMP4Options()
	options.Codec, options.GOP, options.InterResidual = export.H264Predictive, 4, true
	output := filepath.Join(t.TempDir(), "inter.mp4")
	writer, err := export.NewMP4WithOptions(output, 66, 34, 24, options)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()
	encoder, err := h264.NewPredictive(66, 34, 24, h264.PredictiveOptions{QP: 24, GOP: 4, InterResidual: true})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := h264.NewPredictive(66, 34, 24, h264.PredictiveOptions{QP: 24, GOP: 4})
	if err != nil {
		t.Fatal(err)
	}
	var expected []byte
	different := false
	for f := range 6 {
		img := image.NewRGBA(image.Rect(0, 0, 66, 34))
		for y := range 34 {
			for x := range 66 {
				v := uint8(70 + (x*17+y*23+x*y)%100 + f)
				img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
			}
		}
		frame, err := encoder.Encode(img)
		if err != nil {
			t.Fatal(err)
		}
		old, err := baseline.Encode(img)
		if err != nil {
			t.Fatal(err)
		}
		different = different || !bytes.Equal(frame.NAL, old.NAL)
		expected = binary.BigEndian.AppendUint32(expected, uint32(len(frame.NAL)))
		expected = append(expected, frame.NAL...)
		if err = writer.WriteFrame(f, img); err != nil {
			t.Fatal(err)
		}
	}
	if !different {
		t.Fatal("fixture does not distinguish residual and original predictive encoding")
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	mdat := find(t, boxes(t, data), "mdat")
	if !bytes.Equal(mdat.payload, expected) {
		t.Fatal("inter residual option was dropped or samples changed in container")
	}
}
