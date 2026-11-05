# Pure-Go audio, subtitles and video output

The runtime uses Go code and portable Go dependencies. Python/FFmpeg/LaTeX/native
code are not rendering, typesetting or encoding fallbacks. Independent decoders
are development validation tools only.

## Scene sound

`audio.PCM` owns immutable interleaved float64 samples. WAV supports PCM16/24/32
and IEEE float32/64 with explicit metadata acceptance. See `audio/README.md` for
format and resource limits, resampling, gain, envelopes and mix APIs. Native PCM16/24 FLAC output uses fixed predictors/Rice coding
and integrity checks; losslessness is relative to integer quantization.
`export.FLAC` uses atomic publication. `audio.ReadFLAC` reads bounded native
PCM8/16/24/32 streams with1..8 indexed channels, constant/verbatim/fixed/LPC subframes,
Rice/Rice2/escape and stereo decorrelation, checking frame CRCs and the PCM MD5.
Unknown format/metadata branches reject explicitly. Metadata discard requires
opt-in; zero/absent MD5 is rejected. See the audio README and FLAC_INPUT.md for exact limits. The writer remains PCM16/24 mono/stereo. CLI WAV can keep the explicit input channel count with -audio-channels; MOV and FLAC output remain mono/stereo, and -audio-remix only converts mono/stereo input. No automatic multichannel downmix is inferred.

```go
if err := scene.AddSoundWithOptions(clip, gomath.SoundOptions{
    Offset: -.25, GainDB: -6,
}); err != nil { return err }
pcm, err := scene.Soundtrack(audio.Format{
    SampleRate: 48000, Channels: 2, ChannelMask: 3,
}, gomath.SoundtrackOptions{})
if err != nil { return err }
return export.WAV("sound.wav", pcm, audio.WAVWriteOptions{})
```

Offsets are relative to the authoring cursor; resulting starts must be
nonnegative. Adding sound does not advance the scene. Start times round to the
nearest output frame, half-frame ties up. Bounded Blackman-windowed sinc
resampling aligns clip time zero; output covers ceil(scene duration × rate)
and every clip tail. Maximum authored sound count is 1024.

Channels mix by index; there is no implicit mono/stereo conversion. Different
known speaker masks reject; zero masks retain channel indices. Floating-point
mixing preserves headroom above [-1,1]. Integer writers reject overrange rather
than silently normalize or clip. SoundtrackOptions controls audio limits and a
default 512M sample-work budget. One output buffer, its final owned copy and at
most one resampled clip coexist; concurrent calls multiply storage. No automatic
soundtrack cache exists. Compiled Replay retains frozen events and shares only
immutable clips.

Explicit library channel conversion uses `PCM.Remix(outputFormat, matrix,
RemixOptions{})`, with one row per output channel and one column per input
channel. It preserves sample rate/time zero, headroom and immutable inputs;
speaker layout and routing are caller decisions. `PCM.Concat` and `Repeat`
provide bounded, format-identical sequencing. See audio/README.md for limits.

## Captions

`Scene.AddSubcaption(text, durationSeconds, offsetSeconds)` adds plain UTF-8 at
the cursor plus offset. Both absolute boundaries round to nearest milliseconds,
ties up; an interval collapsed by rounding rejects. `AddSubcaptionMS` uses exact
integer duration/offset after rounding the cursor once. Neither advances the
scene. Default timeline limit is 24 hours; cue tails beyond scene end remain.
This explicit integer-ms policy differs from Manim's submillisecond timing.

`Scene.Subcaptions(options)` and frozen `Replay.Subcaptions` return immutable
tracks. Authored cues stable-sort by start and retain overlaps. Export callers
can request `OverlapReject` (default) or `OverlapAllow`. Strict SRT/WebVTT I/O
never drops unsupported visible text/settings. WebVTT escapes literal angle
brackets and ampersands; plain SRT rejects markup-looking content. See
`subtitle/README.md` for exact text/ID/ordering/byte limits.

`export.Subtitles(path, track, subtitle.SRT, subtitle.WriteOptions{})` writes
atomically. WebVTT uses `subtitle.WebVTT`. These are separate files; video output
does not embed captions or burn them into pixels.

## Video and MOV audio

`NewMP4` preserves legacy H.264 I_PCM output. Use `NewMP4WithOptions` and
`DefaultMP4Options()` for spatially compressed H.264 I4x4/quantization/CAVLC,
default QP24. QP0 is still lossy. `ModeSearch` optionally compares legal DC,
vertical and horizontal luma predictions with additional CPU cost. It does not
guarantee better size/quality on every image. All codecs use full-range YUV420 and even dimensions.
`H264Predictive` optionally uses IDR/P pictures, with GOP0 selecting60 and
explicit GOP in1..65536. It skips macroblocks only when source-converted YUV
exactly equals the previous reconstruction; remaining macroblocks use intra4x4.
Optional `MP4Options.InterResidual` (CLI `-inter-residual`) additionally compares
zero-motion P_L0_16x16 residuals against intra by a bounded local rate/distortion
cost, and allows skip when inter coefficients quantize to zero. It is valid
only with H264Predictive. False preserves the original predictive bitstreams.
`MP4Options.MotionSearchRadius` (CLI `-motion-radius`) accepts 0..4; a nonzero
radius additionally requires predictive video and InterResidual. It searches
every integer luma displacement in the square [-R,R]² against the single previous
reconstruction, with legal predicted motion deltas, padded edges and fractional
chroma interpolation. Radius zero preserves the earlier bitstreams. This does
not implement fractional luma motion, subpartitions, multiple references or B
frames. CPU cost grows with the candidate count, with no universal size/quality
improvement. See the measured pan/noise/static cases in the codec validation.
Repeated lossy source pictures need not skip. Its SPS has one reference picture;
the muxer writes one-based sync sample indices in stss. DTS=PTS, without B frames.
Input validation errors do not advance the reference; output I/O failure aborts
the file, so a dropped encoded picture cannot be followed by dependent output. MaxFrames bounds container metadata
(default 1,000,000, hard 67,108,864). Fractional luma motion, rate control and
hardware encoding remain pending; detailed decoder evidence is in
`internal/h264/VALIDATION.md`.

`NewMOVWithAudio(path, width, height, fps, pcm, options)` uses a QuickTime MOV
container with H.264 video plus raw little-endian PCM16 (`sowt`). Audio requires
nonempty mono/stereo PCM, rate 1..65535 Hz and an unspecified or conventional
speaker mask. Quantization rounds to nearest, +1 saturates at 32767, no dither.
The writer rejects values outside [-1,1]. It writes one audio chunk then one
video chunk; samples are streamed and metadata is bounded. Movie time uses an
LCM timescale and version-1 duration headers, preserving exact audio tails
shorter than one video frame. Track durations remain independent. No implicit
trimming, resampling, AAC, fragmentation, fast-start or interleaving exists. Predictive video can use
the same PCM MOV track; encoded video and audio timescales stay independent.

All file exports reject existing paths and publish only on successful completion.
The `composition` demo contains nested clocks, five seconds of stereo synthetic
audio and two captions:

```sh
CGO_ENABLED=0 go run -tags noasm ./cmd/gomath demo -scene composition \
  -format mov -width 480 -height 270 -fps 12 -out artifacts/composition.mov
CGO_ENABLED=0 go run -tags noasm ./cmd/gomath demo -scene composition \
  -format wav -out artifacts/composition.wav
CGO_ENABLED=0 go run -tags noasm ./cmd/gomath demo -scene composition \
  -format srt -out artifacts/composition.srt
```

`-audio input.wav`, `input.flac`, `input.aiff` or `input.au` mixes additional native WAV/FLAC/AIFF/AU decoded PCM
from time zero for MOV/WAV/FLAC. The stream signature selects the reader,
independent of extension. WAV additionally accepts Microsoft IMA ADPCM tag0x11, 4-bit mono/stereo complete blocks with a mandatory fact count; the final block alone may be trimmed. Microsoft ADPCM tag2 also decodes 4-bit mono/stereo full blocks, file coefficient pairs and mandatory final-block fact trimming; short physical final blocks, unsupported variants and adaptive arithmetic overflow reject. WAV G.711 A-law/mu-law tags6/7 additionally decode 8-bit indexed channels; an exact mandatory fact count, 18-byte fmt and cbSize=0 are required, with no tail trimming. Specification/libsndfile and FFmpeg rounding differences are documented in audio/README.md. AIFF supports signed PCM8/16/24/32 and AIFC NONE, plus AIFC sowt PCM16; other compression and CHAN layout chunks reject, including with metadata discard. CLI rejects trailing data after the native stream; lower-level streaming readers retain their documented stream-boundary policy. Stereo output is default; choose matching
`-audio-channels` explicitly. Supported metadata discard requires
`-discard-audio-metadata`. This is not a general media reader, and does not
truncate audio to the visual timeline.

`-audio-remix` explicitly converts only the imported clip to the selected
`-audio-channels 1` or `2`: mono duplicates into left/right; stereo averages
left/right with weights .5/.5. The input must use unspecified or conventional
mono/front-center or stereo/front-left+front-right layout. Unknown multichannel
routing rejects. Same-channel inputs are retained unchanged. The flag requires
`-audio`, does not remix existing authored scene sounds, and applies no clipping,
normalization or gain compensation. Soundtrack performs sample-rate conversion
afterwards. Without the flag, mismatched channel counts continue to reject.

Integrated independent validation is reproducible with tests/reference/media.py
after building bin/gomath, using a fresh output directory. At the 2026-11-04
checkpoint, it strictly decoded five seconds/60frames of predictive MOV, checked
keyframes/PTS, compared MOV/WAV/FLAC decoded PCM16 exactly, and decoded Unicode
SRT/WebVTT. The artifacts include a provenance JSON with code/binary hashes.
This is one synthetic scene, not general media or random-seek compatibility.

The additional `--inter-residual` validation run is recorded in
`artifacts/media-inter-20261104-verified/provenance.json`: five-second
480×270/12fps predictive MOV, 60 pictures and five IDRs, complete independent
decode/PTS checks, unchanged PCM across MOV/WAV/FLAC, and native Go FLAC import
to WAV with all five seconds retained on a one-second visual scene. The output
MOV was 1,011,558 bytes for this run; there is no equal-quality compression or
throughput claim. Previous artifacts remain historical records.

`artifacts/media-motion-20261104/provenance.json` records the corresponding
`--inter-residual --motion-radius 1` integrated run: all 60 pictures/5 IDRs and
presentation timestamps passed strict independent decoding, MOV/WAV/FLAC PCM16
matched exactly, the imported FLAC audio tail was retained, and Unicode SRT/VTT
decoded correctly. This run's MOV was 1,010,839 bytes; the synthetic scene and
different motion decisions do not establish an equal-quality compression gain.

The compiled mesh timeline and `streamline-flow` integration demos forward authored sound
and subtitle access to their ordinary overlay Scene, with the same five-second
clock as the sampled source. They support explicit imported audio and MOV/WAV/FLAC
output through the CLI; this adapter does not add audio APIs to the independent
scene3d package. Finish media authoring before concurrent render calls.

Native AU input recognizes `.snd` regardless of extension, with big-endian signed
PCM8/16/24/32 or G.711 A-law/mu-law and explicit index channel order. CLI accepts
known data sizes only, rejects bytes beyond that size, and requires
`-discard-audio-metadata` for nonzero annotation bytes. Library ReadAU additionally
permits unknown-size data through EOF only with AllowUnknownSize; callers must
bound that reader themselves to avoid consuming a following object. Float/ADPCM
AU and automatic speaker routing remain unsupported. Central CLI checks verify
all six encodings, exact samples/remix, full scene clock, annotation policy and
atomic failure on malformed/unknown/trailing input.

The sixth-wave `segments3d` actual closed-mesh timeline is independently decoded
in `artifacts/segments3d-wave6-media-20261105/provenance.json`:40 frames,5 seconds,
320×180/8fps,5 IDRs, exact PTS and40 distinct decoded pictures. Seven native PNG
comparisons have RGB PSNR41.746–42.090dB and MAE1.167–1.228/255. Its source and
binary hashes match the sixth frozen checkpoint. This validates timeline/media
integration; independent endpoint-mesh geometry evidence is in SEGMENT3D.md.
FLAC's equivalent CRC16 optimization and before/after workload evidence are
recorded separately in FLAC_PERFORMANCE.md; format and CLI policies stay the same.

The seventh-wave checkerboard timeline has a passing explicit640×360/QP18
integration in `artifacts/checkerboard-surface-wave7-640-qp18-media-20261105/provenance.json`:
40 pictures/5seconds/5IDRs, exact PTS and40 distinct decoded frames. Seven PNG
comparisons give RGB PSNR32.071–32.719dB and MAE2.073–2.217/255. Sources/binary
match the seventh frozen checkpoint. The development tool now exposes explicit
`--qp`, `--width` and `--height`; its previous defaults and28dB/MAE5 thresholds
stay unchanged. This high-contrast grid failed that quality bound at320×180 for
both QP24 (frame6,27.969dB) and QP18 (frame20,27.861dB). Both failed outputs and
failure manifests remain in the respective320-size artifact directories. No
claim of a320-size quality pass, encoder improvement or equal-quality throughput
is made. Independent checkerboard geometry is verified separately in
CHECKERBOARD_SURFACES.md.
