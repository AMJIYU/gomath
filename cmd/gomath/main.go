package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gomath"
	"gomath/audio"
	"gomath/compat"
	"gomath/examples/scenes"
	"gomath/export"
	"gomath/render"
	"gomath/subtitle"
	"os"
	"os/signal"
	"runtime"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "gomath:", e)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: gomath demo [flags] | benchmark [flags] | fixture [flags] | compare [flags] | parity")
	}
	switch os.Args[1] {
	case "demo":
		return demo(os.Args[2:])
	case "benchmark":
		return benchmark(os.Args[2:])
	case "parity":
		return parity()
	case "fixture":
		return fixture(os.Args[2:])
	case "compare":
		return compare(os.Args[2:])
	default:
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}
func demo(args []string) error {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	name := fs.String("scene", "calculus", fmt.Sprint(scenes.Names()))
	format := fs.String("format", "gif", "gif, mp4, mov (PCM audio), wav, flac, srt, vtt, png, svg or frames")
	output := fs.String("out", "artifacts/calculus.gif", "output path (must not exist)")
	width := fs.Int("width", 640, "pixel width")
	height := fs.Int("height", 360, "pixel height")
	fps := fs.Int("fps", 24, "frames per second")
	codec := fs.String("codec", "intra", "MP4/MOV video codec: intra, predictive (single-reference IDR/P), or pcm (I_PCM)")
	qp := fs.Int("qp", 24, "MP4 intra quantization parameter, 0..51; smaller preserves more detail")
	modeSearch := fs.Bool("mode-search", false, "search three luma prediction modes for intra MP4/MOV; uses more CPU")
	gop := fs.Int("gop", 0, "predictive IDR interval, 1..65536; zero selects 60, requires -codec predictive")
	interResidual := fs.Bool("inter-residual", false, "enable zero-motion inter residuals; requires predictive mp4 or mov")
	motionRadius := fs.Int("motion-radius", 0, fmt.Sprintf("integer-luma search radius 0..%d; nonzero requires predictive mp4/mov and -inter-residual; adds substantial CPU work", export.MaxMotionSearchRadius))
	audioInput := fs.String("audio", "", "optional native WAV, FLAC, AIFF or known-size AU input mixed from scene time zero; requires mov, wav or flac")
	audioRate := fs.Int("audio-rate", 48000, "soundtrack sample rate for mov/wav")
	audioChannels := fs.Int("audio-channels", 2, "soundtrack channels; mov requires mono/stereo")
	audioRemix := fs.Bool("audio-remix", false, "explicit mono/stereo conversion of -audio to -audio-channels; stereo to mono averages left/right")
	discardAudioMetadata := fs.Bool("discard-audio-metadata", false, "explicitly discard supported WAV/FLAC/AIFF metadata or AU annotation when loading -audio")
	workers := fs.Int("workers", max(1, runtime.GOMAXPROCS(0)/2), "parallel render workers")
	supersampling := fs.Int("supersampling", 1, "raster quality scale: 1, 2, 4 or 8; higher values use more CPU and memory")
	sample := fs.Float64("time", 2.3, "sample time for png/svg")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if (*format == "mp4" || *format == "mov") && (*codec != "intra" && *codec != "pcm" && *codec != "predictive") {
		return fmt.Errorf("unknown MP4 codec %q", *codec)
	}
	if (*format == "mp4" || *format == "mov") && *codec != "pcm" && (*qp < 0 || *qp > 51) {
		return fmt.Errorf("MP4 QP must be in 0..51")
	}
	if *modeSearch && ((*format != "mp4" && *format != "mov") || *codec == "pcm") {
		return fmt.Errorf("-mode-search requires compressed mp4 or mov output")
	}
	if *gop != 0 && ((*format != "mp4" && *format != "mov") || *codec != "predictive" || *gop < 1 || *gop > 65536) {
		return fmt.Errorf("-gop requires predictive mp4/mov and a value in 1..65536")
	}
	if *interResidual && ((*format != "mp4" && *format != "mov") || *codec != "predictive") {
		return fmt.Errorf("-inter-residual requires predictive mp4 or mov output")
	}
	if *motionRadius < 0 || *motionRadius > export.MaxMotionSearchRadius || *motionRadius != 0 && ((!*interResidual) || (*format != "mp4" && *format != "mov") || *codec != "predictive") {
		return fmt.Errorf("-motion-radius must be 0..%d; nonzero requires predictive mp4/mov and -inter-residual", export.MaxMotionSearchRadius)
	}
	if *audioInput != "" && *format != "mov" && *format != "wav" && *format != "flac" {
		return fmt.Errorf("-audio requires mov, wav or flac output")
	}
	if *audioRemix && (*audioInput == "" || (*audioChannels != 1 && *audioChannels != 2)) {
		return fmt.Errorf("-audio-remix requires -audio and -audio-channels 1 or 2")
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FPS, c.Workers = *width, *height, *fps, *workers
	c.Supersampling = *supersampling
	if *workers < 1 || *workers > 256 {
		return fmt.Errorf("workers must be in [1,256]")
	}
	if *format == "png" || *format == "svg" {
		// Still-image output uses one Rasterizer and no frame-worker queue.
		c.Workers = 1
	}
	if *name == "reference" {
		c.Background = gomath.Black
	}
	if e := c.Validate(); e != nil {
		return e
	}
	s, e := scenes.BuildSource(*name)
	if e != nil {
		return e
	}
	if *audioInput != "" {
		clip, err := readAudioInput(*audioInput, *discardAudioMetadata)
		if err != nil {
			return err
		}
		if *audioRemix {
			clip, err = remixAudioInput(clip, *audioChannels)
			if err != nil {
				return err
			}
		}
		adder, ok := s.(interface {
			AddSound(*audio.PCM, float64) error
		})
		if !ok {
			return fmt.Errorf("this compiled scene cannot accept additional authored sounds")
		}
		if err = adder.AddSound(clip, -s.Duration()); err != nil {
			return err
		}
	}
	var soundtrack *audio.PCM
	if *format == "srt" || *format == "vtt" {
		source, ok := s.(interface {
			Subcaptions(subtitle.Options) (*subtitle.Track, error)
		})
		if !ok {
			return fmt.Errorf("scene has no subtitle API")
		}
		track, err := source.Subcaptions(subtitle.Options{Order: subtitle.OrderStableSort, Overlap: subtitle.OverlapAllow})
		if err != nil {
			return err
		}
		kind := subtitle.SRT
		if *format == "vtt" {
			kind = subtitle.WebVTT
		}
		if err = export.Subtitles(*output, track, kind, subtitle.WriteOptions{}); err != nil {
			return err
		}
		fmt.Println(*output)
		return nil
	}
	if *format == "mov" || *format == "wav" || *format == "flac" {
		source, ok := s.(interface {
			Soundtrack(audio.Format, gomath.SoundtrackOptions) (*audio.PCM, error)
		})
		if !ok {
			return fmt.Errorf("scene has no soundtrack API")
		}
		mask := uint32(0)
		if *audioChannels == 1 {
			mask = 4
		} else if *audioChannels == 2 {
			mask = 3
		}
		soundtrack, e = source.Soundtrack(audio.Format{SampleRate: *audioRate, Channels: *audioChannels, ChannelMask: mask}, gomath.SoundtrackOptions{})
		if e != nil {
			return e
		}
		if *format == "wav" || *format == "flac" {
			if *format == "flac" {
				e = export.FLAC(*output, soundtrack, audio.FLACWriteOptions{})
			} else {
				e = export.WAV(*output, soundtrack, audio.WAVWriteOptions{})
			}
			if e != nil {
				return e
			}
			fmt.Println(*output)
			return nil
		}
	}
	if *format == "png" || *format == "svg" {
		frame, e := s.Sample(*sample)
		if e != nil {
			return e
		}
		if *format == "svg" {
			e = export.SVG(*output, frame, c)
		} else {
			r, err := render.NewRasterizer(c)
			if err != nil {
				return err
			}
			img, err := r.Render(frame)
			if err != nil {
				return err
			}
			e = export.PNG(*output, img)
		}
		if e != nil {
			return e
		}
		fmt.Println(*output)
		return nil
	}
	var sink render.Sink
	switch *format {
	case "gif":
		sink, e = export.NewGIF(*output, c.Width, c.Height, c.FPS)
	case "mp4", "mov":
		options := export.DefaultMP4Options()
		options.QP = *qp
		options.ModeSearch = *modeSearch
		options.GOP = *gop
		options.InterResidual = *interResidual
		options.MotionSearchRadius = *motionRadius
		if *codec == "predictive" {
			options.Codec = export.H264Predictive
		}
		if *codec == "pcm" {
			options.Codec, options.QP = export.H264PCM, 0
		}
		if *format == "mov" {
			sink, e = export.NewMOVWithAudio(*output, c.Width, c.Height, c.FPS, soundtrack, options)
		} else {
			sink, e = export.NewMP4WithOptions(*output, c.Width, c.Height, c.FPS, options)
		}
	case "frames":
		sink, e = export.NewPNGSequence(*output)
	default:
		return fmt.Errorf("unknown output format %q", *format)
	}
	if e != nil {
		return e
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	start := time.Now()
	stats, e := render.RenderScene(ctx, s, c, sink)
	if e != nil {
		return e
	}
	elapsed := time.Since(start)
	fmt.Printf("%s: %d frames, %.2fs scene, %.3fs elapsed, %.1f frames/s\n", *output, stats.Frames, stats.Duration, elapsed.Seconds(), float64(stats.Frames)/elapsed.Seconds())
	return nil
}
func benchmark(args []string) error {
	fs := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	name := fs.String("scene", "lissajous", "scene name")
	frames := fs.Int("frames", 60, "number of samples")
	width := fs.Int("width", 640, "pixel width")
	height := fs.Int("height", 360, "pixel height")
	supersampling := fs.Int("supersampling", 1, "raster quality scale: 1, 2, 4 or 8")
	out := fs.String("out", "", "optional JSON result path")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if *frames < 1 || *frames > 100000 {
		return fmt.Errorf("frames must be in [1,100000]")
	}
	s, e := scenes.BuildSource(*name)
	if e != nil {
		return e
	}
	c := render.DefaultConfig()
	c.Width, c.Height = *width, *height
	c.Supersampling = *supersampling
	if *name == "reference" {
		c.Background = gomath.Black
	}
	c.Workers = 1
	r, e := render.NewRasterizer(c)
	if e != nil {
		return e
	}
	f, e := s.Sample(s.Duration() / 2)
	if e != nil {
		return e
	}
	if _, e = r.Render(f); e != nil {
		return e
	}
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	for i := range *frames {
		t := s.Duration() * float64(i) / float64(*frames)
		f, e := s.Sample(t)
		if e != nil {
			return e
		}
		if _, e = r.Render(f); e != nil {
			return e
		}
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	result := map[string]any{"scene": *name, "width": c.Width, "height": c.Height, "supersampling": c.Supersampling, "frames": *frames, "workers": 1, "elapsed_seconds": elapsed.Seconds(), "frames_per_second": float64(*frames) / elapsed.Seconds(), "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocations": after.Mallocs - before.Mallocs, "go_version": runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH, "gomaxprocs": runtime.GOMAXPROCS(0), "comparison": "GoMath only; no Manim speedup claim"}
	data, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		return e
	}
	fmt.Println(string(data))
	if *out != "" {
		f, e := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if e != nil {
			return e
		}
		_, writeErr := f.Write(append(data, '\n'))
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	return nil
}

func fixture(args []string) error {
	fs := flag.NewFlagSet("fixture", flag.ContinueOnError)
	out := fs.String("out", "artifacts/reference.json", "output JSON (must not exist)")
	if e := fs.Parse(args); e != nil {
		return e
	}
	s, e := scenes.Reference()
	if e != nil {
		return e
	}
	f, e := s.Sample(.5)
	if e != nil {
		return e
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers = 640, 360, 1
	c.Background = gomath.Black
	data, e := json.MarshalIndent(struct {
		Config render.Config
		Frame  *gomath.Frame
	}{c, f}, "", "  ")
	if e != nil {
		return e
	}
	file, e := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	_, e = file.Write(data)
	closeErr := file.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Println(*out)
	return nil
}
func parity() error {
	ledger, e := compat.Load()
	if e != nil {
		return e
	}
	fmt.Printf("Manim %s, source %s\n%d Python modules; %d declared public symbols\n", ledger.Source.Version, ledger.Source.Commit, len(ledger.Modules), len(ledger.Symbols))
	counts := map[string]int{}
	for _, s := range ledger.Symbols {
		counts[s.Status]++
	}
	data, _ := json.MarshalIndent(counts, "", "  ")
	fmt.Println(string(data))
	fmt.Println("See docs/PARITY.md for feature-level progress. A similar Go API is not proof of compatibility.")
	return nil
}
