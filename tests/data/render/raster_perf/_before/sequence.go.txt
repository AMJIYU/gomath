package render

import (
	"context"
	"fmt"
	"gomath"
	"image"
	"math"
	"reflect"
	"sync"
)

// Sink receives ordered frames and owns output finalization. Abort must remove
// partial output. RenderScene calls Close only after all frames succeed.
type Sink interface {
	WriteFrame(index int, frame *image.RGBA) error
	Close() error
	Abort() error
}
type Stats struct {
	Frames   int
	Duration float64
}
type frameResult struct {
	index int
	frame *image.RGBA
	err   error
}

// FrameSource can be an authored Scene or a compiled, checkpointed Replay.
// After authoring/compilation, Sample must allow concurrent arbitrary-order calls.
type FrameSource interface {
	Duration() float64
	Sample(time float64) (*gomath.Frame, error)
}

// RenderScene bounds outstanding work to twice the worker count, including
// out-of-order results, so memory use does not grow with scene length.
func RenderScene(ctx context.Context, scene FrameSource, c Config, sink Sink) (stats Stats, err error) {
	if sink == nil {
		return stats, fmt.Errorf("nil sink")
	}
	defer func() {
		if err != nil {
			_ = sink.Abort()
		}
	}()
	if scene == nil || (reflect.ValueOf(scene).Kind() == reflect.Pointer && reflect.ValueOf(scene).IsNil()) {
		return stats, fmt.Errorf("nil scene")
	}
	if e := c.Validate(); e != nil {
		return stats, e
	}
	duration := scene.Duration()
	if !finite(duration) || duration < 0 {
		return stats, fmt.Errorf("frame source has invalid duration")
	}
	nf := math.Ceil(duration * float64(c.FPS))
	if nf > float64(math.MaxInt32) {
		return stats, fmt.Errorf("scene has too many frames")
	}
	count := max(1, int(nf))
	workers := min(c.Workers, count)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	results := make(chan frameResult, workers)
	slots := make(chan struct{}, 2*workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := NewRasterizer(c)
			if e != nil {
				return
			}
			for {
				select {
				case <-ctx.Done():
					return
				case i, ok := <-jobs:
					if !ok {
						return
					}
					t := math.Min(float64(i)/float64(c.FPS), duration)
					f, e := scene.Sample(t)
					var pixels *image.RGBA
					if e == nil {
						pixels, e = r.Render(f)
					}
					select {
					case results <- frameResult{i, pixels, e}:
					case <-ctx.Done():
						return
					}
					if e != nil {
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range count {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(results); close(done) }()
	defer func() { cancel(); <-done }()
	pending := map[int]*image.RGBA{}
	next := 0
	for next < count {
		select {
		case <-ctx.Done():
			return stats, ctx.Err()
		case result, ok := <-results:
			if !ok {
				return stats, fmt.Errorf("workers stopped after %d of %d frames", next, count)
			}
			if result.err != nil {
				return stats, fmt.Errorf("frame %d: %w", result.index, result.err)
			}
			pending[result.index] = result.frame
			for {
				img, ok := pending[next]
				if !ok {
					break
				}
				if e := sink.WriteFrame(next, img); e != nil {
					return stats, fmt.Errorf("write frame %d: %w", next, e)
				}
				delete(pending, next)
				next++
				<-slots
				stats.Frames = next
			}
		}
	}
	if e := ctx.Err(); e != nil {
		return stats, e
	}
	if e := sink.Close(); e != nil {
		return stats, e
	}
	stats.Duration = duration
	return stats, nil
}
