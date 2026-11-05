# Pure-Go intra encoder — integration and measured bounds

Validated 2026-11-04. This record covers `internal/h264` only, not export/muxer,
CLI integration, whole-repository tests, hardware decoders, or Manim parity.

Changed files: `internal/h264/pcm.go` (preserved PCM behavior, shared parameter
builders), new `internal/h264/{intra.go,transform.go,cavlc.go,cavlc_tables.go}`,
new `tests/_cases/internal/h264/{intra_test.go,cavlc_test.go}`, new
`tests/data/internal/h264/intra_vectors.json`, and this validation record.
The existing central `pcm_test.go` was retained unchanged. No production
directory contains a test source file.

The optional mode-search update changes `internal/h264/intra.go` and adds
`internal/h264/prediction.go`. Its central tests are
`tests/_cases/internal/h264/prediction_test.go`, with the existing vector helper
extended in `intra_test.go`. Independent mode-search hashes are in
`tests/data/internal/h264/intra_modesearch_vectors.json`; the original
`intra_vectors.json` fixtures are unchanged. Export and CLI files were not edited.

```go
enc, err := h264.NewIntra(width, height, fps, h264.IntraOptions{QP: 24})
// Handle err before using enc.
sps, pps := enc.SPS(), enc.PPS()
idr, err := enc.Encode(rgba)
```

The public API is stable: `NewIntra(int,int,int,IntraOptions) (*IntraEncoder,error)`,
`SPS() []byte`, `PPS() []byte`, `Encode(*image.RGBA) ([]byte,error)`.
Each return is a complete escaped NAL including its one-byte header, without
an Annex B start code or MP4 length. Every encoded frame is an IDR/keyframe.
Use this encoder's SPS/PPS, not those of a separately constructed PCM encoder.
Changing QP requires a new encoder and its corresponding parameter sets.
`IntraOptions` also accepts `ModeSearch bool`; the default is false and preserves
the original DC-only bitstream byte for byte. Enable it with
`h264.IntraOptions{QP: 24, ModeSearch: true}`. The SPS/PPS and NAL framing remain
identical for the same dimensions/FPS/QP, regardless of this flag.

The original `New`/`Encoder` I_PCM API and output behavior remain available.
Only its SPS/PPS builders were factored into private helpers with identical
defaults; PCM pixel conversion and macroblock coding were retained.

Actual coding path: full-range 8-bit YCbCr 4:2:0, edge replication to 16-pixel
macroblocks with SPS cropping, I_4x4 luma DC prediction from reconstructed
neighbors (optionally vertical/horizontal as described below), 4x4 integer
transform/quantization, chroma DC prediction with 2x2 DC
Hadamard plus 4x4 AC coefficients, CAVLC coefficient tokens/levels/zero runs,
inverse quantization/transform and clipped reconstruction for later predictions.
The normative tables and inverse operations follow ITU-T H.264 (08/2024),
clauses 8.3, 8.5 and 9.2. The runtime imports only the Go standard library.

QP is the **luma** quantization parameter, 0–51; zero is not lossless. Chroma QP
uses the standard mapping, with a minimum of 4 explicitly signalled using
`chroma_qp_index_offset = max(0,4-QP)` in the PPS. At QP 0–3, extreme chroma DC
coefficients otherwise can exceed Baseline's maximum CAVLC level prefix of 15.
This is explicit quantization configuration, not coefficient truncation or a
fallback. Tests exhaust all 65,536 extremal 4x4 residual sign patterns at every
QP and all extremal 2x2 DC sign patterns to check the level bound.

Dimensions must be even, 2–4096, FPS 1–120. Nonzero origins, RGBA subimages and
premultiplied alpha over black work. Malformed pixel storage returns an error.
The encoder is immutable and concurrent Encode calls use independent buffers.
Single-call encoding is serial and allocates buffers per frame.

Limits: one slice, luma DC by default or optional DC/vertical/horizontal search;
chroma remains DC. No diagonal luma modes, inter prediction, motion estimation,
entropy-mode choice, deblocking, bitrate control or VBV/HRD.
It is an actual spatial codec, not a replacement for a mature rate-distortion
optimized encoder. Chroma subsampling and quantization discard detail; high
QP visibly loses fine text and edges. Small frames and low-QP noise can cost
more than the cropped raw pixels. SPS uses level 5.1, raising to 5.2/6.0/6.1
as needed for macroblock count/rate. Bitrate/buffer-level conformance is not
guaranteed, and browser/hardware compatibility was not measured.

Independent decoding used FFmpeg 8.0.1, libavcodec 62.11.100:

```sh
CGO_ENABLED=0 GOMATH_H264_DUMP_DIR=/tmp/gomath-h264-validation \
  go run ./tests/cmd/run -pkg=internal/h264 -run=TestIntraReferenceVectors -count=1
ffmpeg -hide_banner -loglevel error -xerror -err_detect explode \
  -i INPUT.h264 -pix_fmt yuv420p -f rawvideo -y OUTPUT.yuv
```

77 streams / 231 frames decoded completely, with every decoded Y/Cb/Cr byte
equal to the encoder's reconstruction. Matrix: five sizes (2x2, 18x22, 48x34,
160x96, 320x180), five QPs (0,12,24,36,51), three changing mixed-pattern frames;
plus 64x48 saturated-color and black/white checkerboards at **all 52 QPs**,
three frames each. Pure-Go tests use independently verified stream/decoded
SHA-256 fixtures in `tests/data/internal/h264/intra_vectors.json`; they never
execute FFmpeg or require it installed. Dumped reconstruction hashes were
checked against FFmpeg output before committing the fixtures.

320x180 mixed-pattern stream means over three frames (headers included):

| QP | Bytes/frame | YUV PSNR (dB) |
|---:|---:|---:|
| 0 | 76,912.67 | 64.37 |
| 12 | 49,390.67 | 51.93 |
| 24 | 28,169.67 | 40.61 |
| 36 | 14,690.67 | 29.11 |
| 51 | 4,402.00 | 18.93 |

PSNR is over all cropped YUV samples (luma weighted by sample count), compared
to the encoder's unquantized 4:2:0 conversion. It excludes RGB conversion and
chroma-subsampling error relative to the original RGBA. A smooth 320x180 color
gradient at QP24 uses 2,848 bytes versus PCM's 92,646 (32.53x smaller), at
44.75 dB YUV PSNR; this easy input is not a general compression-ratio claim.

Relevant checks passed:

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -count=1 -cover -p 1
CGO_ENABLED=0 go vet -tags noasm ./internal/h264
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -run='^$' \
  -bench=BenchmarkIntra -benchmem -benchtime=300ms -count=3 -p 1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -run='^$' \
  -bench=BenchmarkPCM -benchmem -benchtime=300ms -count=3 -p 1
```

Central runner enables `-tags noasm`; package coverage was 99.7%. A scoped
`go list -tags noasm -deps -json ./internal/h264` audit found no native inputs,
runtime subprocess imports or third-party runtime dependencies. Whole-repo
validation is left to the parent integration task. Race instrumentation was
not run; concurrent behavior has deterministic parallel-call unit coverage.

Benchmark environment: Apple M2, darwin/arm64, macOS 15.7.1, Go 1.27.3,
CGO_ENABLED=0, noasm, GOMAXPROCS=8, `-p 1`. One serial Encode per iteration,
same deterministic gradient/edge/25%-noise image; input construction, SPS/PPS,
muxing, filesystem I/O and FFmpeg excluded. Three 300ms samples; indicative
local measurements on a shared development machine, not latency guarantees.

| Size / QP | Three samples (ms/frame) | NAL bytes/frame | B/op | allocs/op |
|---|---|---:|---:|---:|
| 320x180 / 12 | 2.379 / 2.438 / 2.371 | 49,327 | 391,169–391,174 | 13 |
| 320x180 / 24 | 2.032 / 2.195 / 1.997 | 28,158 | 264,192–264,195 | 11 |
| 320x180 / 36 | 1.735 / 1.734 / 1.834 | 14,667 | 251,904–251,931 | 11 |
| 1280x720 / 12 | 35.345 / 34.811 / 40.079 | 728,126 | 5,414,924–5,414,938 | 13 |
| 1280x720 / 24 | 29.799 / 33.397 / 29.738 | 425,494 | 3,801,088–3,801,097 | 11 |
| 1280x720 / 36 | 25.804 / 28.130 / 29.071 | 222,480 | 3,596,288–3,596,305 | 11 |
| 320x180 / PCM | 0.449 / 0.386 / 0.389 | 92,644 | 417,793–417,798 | 6 |
| 1280x720 / PCM | 5.753 / 6.397 / 5.630 | 1,389,607 | 5,947,396–5,947,418 | 6 |

At QP24 this input is 3.29x / 3.27x smaller than PCM at 320x180 / 1280x720,
with approximately five times the encoding CPU cost. Workloads with repeated
frames still encode each frame independently and gain no temporal savings.

Optional mode-search validation (2026-11-04): the encoder considers the legal
subset of DC (2), vertical (0) and horizontal (1) for every 4x4 luma block.
Vertical requires the reconstructed row above; horizontal requires the
reconstructed column to the left. It quantizes and reconstructs every candidate,
measures exact CAVLC residual bit length in the current nC context, and includes
1 or 4 bits for the mode syntax. The integer decision cost is
`SSE*2048 + step*step*(residual_bits + mode_bits)`, where `step` is the 4x4 DC
inverse scale at the configured QP. Ties favor DC, then vertical, then horizontal.
This is a deterministic greedy block search; it does not optimize future blocks,
whole-frame size/PSNR, chroma or NAL emulation-prevention bytes. A smaller file,
higher PSNR or faster encoding is not guaranteed for every image/QP.

The selected modes are stored in picture-wide 4x4 neighbor state. Predicted mode
is DC if **either** A or B is unavailable; otherwise it is the minimum of A/B.
The encoder writes `prev_intra4x4_pred_mode_flag`, or a three-bit
`rem_intra4x4_pred_mode` excluding the predicted mode, before residual syntax.
Macroblock residuals/nC are staged while the selected blocks are reconstructed
in decoding order. One 128-byte candidate buffer is reused per frame; concurrent
Encode calls never share this workspace.

FFmpeg 8.0.1 strictly decoded another **92 streams / 276 frames** with search
enabled: the original 77-case matrix plus vertical stripes, horizontal stripes,
and smooth gradients at QP 0/12/24/36/51, three changing frames per case.
Every decoded Y/Cb/Cr byte matched the encoder reconstruction. The final buffer
reuse change retained all independently checked bitstream/reconstruction hashes.
The pure-Go slice parser also reconstructs luma from the actual mode flags/rem
values across macroblock boundaries, parses every CAVLC block and checks the
RBSP end. Mode signaling tests cover all 81 predicted/actual mode pairs;
encoding itself intentionally selects only modes 0–2. Original DC fixtures
and the existing PCM tests pass unchanged. Overall package coverage is 99.8%.

```sh
CGO_ENABLED=0 GOMATH_H264_DUMP_DIR=/tmp/gomath-h264-modesearch \
  go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestIntraModeSearchReferenceVectors$' -count=1
# Strict decoder invocation is the same as above; compare OUTPUT.yuv to *.rec.yuv.
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestIntraModeSearchComparison$' -v -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -run='^$' \
  -bench='^BenchmarkIntraModeSearch$' -benchmem -benchtime=300ms -count=3 -p 1
```

Fixed 320x180 input frame 0, comparing DC-only / ModeSearch at the **same QP**.
Lengths are complete IDR NAL bytes, excluding SPS/PPS, start codes and muxing.
PSNR uses the same cropped, unquantized YUV reference as the preceding tables.
The mixed image is the original deterministic gradient/edge/25%-noise image;
the stripe images are gray, repeating each row or column with sample values
`(coordinate*37)%256`; gradient uses the original smooth color test image.

| Image | QP | Bytes DC / search | PSNR DC / search (dB) |
|---|---:|---:|---:|
| Mixed | 12 | 49,327 / 44,252 | 52.017 / 52.401 |
| Mixed | 24 | 28,158 / 26,321 | 40.605 / 40.742 |
| Mixed | 36 | 14,667 / 14,312 | 29.100 / 29.240 |
| Vertical stripes | 12 | 45,663 / 2,442 | 61.272 / 63.963 |
| Vertical stripes | 24 | 30,752 / 2,164 | 45.050 / 45.770 |
| Vertical stripes | 36 | 13,089 / 1,731 | 33.201 / 33.381 |
| Horizontal stripes | 12 | 42,870 / 2,028 | 61.276 / 63.333 |
| Horizontal stripes | 24 | 28,703 / 1,868 | 45.170 / 45.807 |
| Horizontal stripes | 36 | 12,578 / 1,629 | 33.221 / 33.373 |
| Smooth gradient | 12 | 9,734 / 7,124 | 52.276 / 52.506 |
| Smooth gradient | 24 | 2,848 / 2,432 | 44.751 / 44.916 |
| Smooth gradient | 36 | 1,801 / 1,801 | 36.950 / 36.950 |

Paired QP24 timing uses the same fixed images, hardware/toolchain and exclusions
as above, with three 300ms samples per case, serial `-p 1`.
These are encoding times on a shared machine, not guarantees. Search is slower
on these inputs, with substantial size improvements on repeated rows/columns;
the mixed/gradient gains are smaller and input dependent.

| Image / size | Search | Samples (ms/frame) | NAL bytes | B/op | allocs/op |
|---|---|---|---:|---:|---:|
| Mixed / 320x180 | false | 2.029 / 2.008 / 2.111 | 28,158 | 264,192–264,193 | 11 |
| Mixed / 320x180 | true | 4.686 / 4.620 / 4.628 | 26,321 | 267,008–267,009 | 13 |
| Vertical / 320x180 | false | 1.781 / 1.880 / 3.256 | 30,752 | 309,248–309,304 | 12 |
| Vertical / 320x180 | true | 3.845 / 3.696 / 3.690 | 2,164 | 242,048–242,051 | 13 |
| Horizontal / 320x180 | false | 1.804 / 1.779 / 1.739 | 28,703 | 268,288–268,289 | 11 |
| Horizontal / 320x180 | true | 3.611 / 3.777 / 3.613 | 1,868 | 241,792 | 13 |
| Gradient / 320x180 | false | 1.266 / 1.244 / 1.734 | 2,848 | 238,592 | 11 |
| Gradient / 320x180 | true | 2.803 / 2.663 / 2.734 | 2,432 | 242,432 | 13 |
| Mixed / 1280x720 | false | 29.779 / 30.511 / 29.921 | 425,494 | 3,801,088–3,801,097 | 11 |
| Mixed / 1280x720 | true | 69.415 / 69.496 / 69.659 | 406,104 | 3,850,368–3,850,390 | 13 |

Final central tests (`-count=1 -cover -p 1`), targeted `go vet -tags noasm` and
the scoped noasm/pure-Go dependency audit pass. No runtime dependencies were
added. The original all-IDR, QP/chroma minimum and codec-level limits above
continue to apply; chroma prediction was not expanded in this update.

## Bounded predictive encoder (2026-11-04)

This addition is a separate stateful API. The preceding all-IDR limitations and
concurrency guarantees describe `IntraEncoder`; `PredictiveEncoder` has the
specific behavior below. `New` and `NewIntra` retain their original bytes,
including both DC-only and ModeSearch fixture sets. Export/CLI were not edited.

```go
enc, err := h264.NewPredictive(width, height, fps, h264.PredictiveOptions{
    QP: 24, ModeSearch: true, GOP: 33,
})
// Handle err before using enc. Use THIS encoder's SPS/PPS (one reference).
sps, pps := enc.SPS(), enc.PPS()
frame, err := enc.Encode(rgba)
// Handle err. frame.NAL is a complete escaped NAL without start code/length.
// frame.Keyframe identifies IDR; frame.FrameNum is the 4-bit H.264 frame_num.
// frame.POC is type-2 POC; frame.SkippedMacroblocks counts actual P_SKIP MBs.
enc.ForceIDR() // next successful Encode starts a GOP; failures keep it pending
enc.Reset()   // also releases the reference; next successful Encode is IDR
```

Public types/methods:

- `PredictiveOptions{QP int, ModeSearch bool, GOP int}`. GOP is explicitly
  required in 1..65536, counting the initial IDR. GOP 1 is all-IDR. QP 0..51,
  dimensions even 2..4096, FPS 1..120, and the minimum chroma QP remain unchanged.
- `NewPredictive(int,int,int,PredictiveOptions) (*PredictiveEncoder,error)`.
- `SPS() []byte`, `PPS() []byte`.
- `Encode(*image.RGBA) (PredictiveFrame,error)` returns fields `NAL []byte`,
  `Keyframe bool`, `FrameNum uint8`, `POC int`, `SkippedMacroblocks int`.
- `ForceIDR()` and `Reset()`.

The first successful picture and each GOP boundary are IDRs. The original intra
macroblock path is shared, with alternating `idr_pic_id` 0/1 even across Reset
so consecutive IDRs remain distinct pictures. P slices use NAL header 0x41,
one default L0 reference, no list modifications, and sliding-window reference
marking. Every successful picture is a short-term reference; the single retained
reference is always the immediately previous reconstructed picture. SPS has
`max_num_ref_frames=1`, four-bit frame_num, `pic_order_cnt_type=2`, and disallows
gaps. P frame_num increments modulo 16; type-2 POC is twice the number of
pictures since IDR, including frame_num wraps. No B pictures or output reordering:
DTS equals PTS. Container timestamps use the caller's continuous frame index;
FrameNum/POC are coding metadata and restart at IDR. Recovery entry points
must receive this encoder's SPS/PPS.

P macroblocks have precisely two supported forms:

1. P_SKIP, only when **every input-converted Y/Cb/Cr sample in the padded 16x16 /
   8x8 / 8x8 macroblock equals the previous reference reconstruction**. It copies
   that reference macroblock. All possible inter neighbors in this subset have
   reference index 0 and motion vector (0,0); intra/unavailable neighbors supply
   reference index -1 and motion vector (0,0). The P_SKIP early-zero and remaining
   median-predictor branches therefore both infer (0,0). No motion-vector or
   inter-residual branch is exposed or approximated. See H.264 (08/2024),
   8.4.1.1, 8.4.1.3 and 8.4.1.3.2.
2. P-slice I_NxN (mb_type 5), using the existing DC or optional DC/V/H search,
   transform/quantization/CAVLC and chroma reconstruction. Skip runs are signaled
   before coded MBs, including zero runs, and at the end when nonzero.

Skipped MB residual counts are zero for all planes; their inferred I4x4 modes
are DC. Picture-wide nC/mode grids are initialized per picture. With
`constrained_intra_pred_flag=0`, intra MBs may predict from already reconstructed
skipped MB pixels. Selected intra modes still use actual predicted/rem syntax.
Deblocking remains disabled, so saved reconstruction is the final reference.

Calls are mutex-serialized; callers must submit pictures in display order and
not mutate an input during Encode. Malformed frames are rejected before coding,
and errors advance neither reference, frame_num, GOP position nor pending-IDR
state. A successful output advances the reference even if a caller later drops
its NAL. After losing/dropping output, call ForceIDR or Reset; these create a new
independent entry point, not retrospective repair of already emitted P pictures.
There is no public mutable reference or returned reference buffer. Returned NALs
own their bytes and remain valid after future Encode/Reset calls.

**Important bound:** a repeated original RGBA image usually differs from its
lossy reconstructed YUV. Such MBs cannot skip. This implementation deliberately
does not use prior-original equality, an error threshold, or an unannounced
approximate reference match. All changed MBs are spatially recoded. There is no
motion search, explicit inter residual, multi-reference/long-term prediction,
non-IDR I-frame control, B frame, CABAC, deblocking, bitrate target or VBV/HRD.
It allocates source/reconstruction/context buffers per frame and retains one
padded reconstructed reference (~1.5 * padded width * padded height bytes).
A skip-only frame still converts every input sample and checks/copies MBs;
there is no constant-time repeated-frame shortcut. Bitrate/level-buffer
conformance and hardware/browser support are not established by these tests.

Changed files for this slice: `internal/h264/predictive.go` (new API/state/P
slice), `internal/h264/intra.go` (shared validation and intra-MB writer plus
parameterized IDR id), `internal/h264/pcm.go` (reference-count SPS helper with
old zero-reference wrapper), this record, and central test artifacts
`tests/_cases/internal/h264/{predictive_test.go,validate_predictive.py}` and
`tests/data/internal/h264/predictive_vectors.json`.

### Independent decoding and pure-Go checks

FFmpeg 8.0.1/libavcodec 62.11.100 strictly decoded **206 complete streams /
6,500 frames**, all visible reconstructed Y/Cb/Cr bytes exactly equal. It also
started **144 fresh independent decoders at IDRs inside the streams**; every
suffix matched the corresponding reconstruction. The strict command is:

```sh
ffmpeg -hide_banner -loglevel error -xerror -err_detect explode -threads 1 \
  -i INPUT.h264 -pix_fmt yuv420p -f rawvideo -y OUTPUT.yuv
```

The matrix has both ModeSearch settings; QP 0/24/51 across 2x2, 18x22, 34x38,
50x34, 320x180 local-update sequences (23 pictures, GOP7); saturated extremes
at **all 52 QPs** on 34x38 (7 pictures, GOP4); flat, repeated lossy mixed,
fully changing mixed, chroma-only, local, recovery and checker-update patterns
on 50x34 (40 pictures, GOP33); cropped all-IDR GOP1; and 70 pictures with
GOP65536 exercising four frame_num wraps. 34x38 is a **3x3 macroblock grid**
with cropping. Checker updates exercise skipped MBs whose A/B neighbors are
both intra. Recovery streams include consecutive forced IDRs, Reset, and
rejected frames with pending IDR requests. Independent entry checks cover each
IDR in the recovery and cropped 3x3 local-update streams. The GOP60 integration
matrix adds 26 streams of 125 pictures: QP24 flat/local/changing-mixed inputs at
2x2, 34x38 and 320x180, plus QP0/51 local/chroma-only inputs at 34x38, all with
both ModeSearch settings. Every GOP60 IDR also has an independent suffix decode;
there are 78 additional fresh entries. The prior 180 stream records remain
byte-for-byte identical in the expanded fixture.

The development helper checks frame count via exact raw output length, compares
every byte, verifies SHA-256, and only then optionally installs the manifest:

```sh
CGO_ENABLED=0 GOMATH_H264_PREDICTIVE_DUMP_DIR=/tmp/gomath-h264-predictive \
  go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestPredictiveReferenceVectors$' -count=1 -p 1
python3 tests/_cases/internal/h264/validate_predictive.py \
  /tmp/gomath-h264-predictive
# Add --fixture tests/data/internal/h264/predictive_vectors.json to install
# hashes ONLY after all independent comparisons pass.
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -count=1 -cover -p 1
CGO_ENABLED=0 go vet -tags noasm ./internal/h264
```

Verified predictive manifest SHA-256:
`1dfdd93b1be588ab47ebe24b22bbc15de445339d3d073cab5283e5fa27dfecc9`.
The Go tests never execute FFmpeg/Python/subprocesses. They parse emitted P/IDR
headers, skip runs, mode flags/rem values and all CAVLC blocks, reconstruct all
three padded planes and compare encoder references. Normal runs additionally
check the independently verified stream/reconstruction hashes. Error-state
comparison uses a control encoder after repeated malformed inputs; concurrent
identical-picture calls check serialized frame_num/POC state. Exact skip tests
change each raster MB including first/last, and reject differences in chroma
or padded edge samples. Existing PCM and both intra golden sets still pass.
Final package tests: **99.6% statement coverage**; targeted vet passed. Scoped
noasm dependency audit: 63 packages, standard library only, no native inputs,
`os/exec` or `plugin`. Race instrumentation and whole-repository tests were not
run for this owned slice.

### Fixed workload size, quality and performance

Fixed QP24, 320x180, 24fps, 33 pictures. Comparison is GOP1 against GOP33,
both through NewPredictive with matching ModeSearch. Sizes include one SPS/PPS,
all NALs and four-byte Annex B start codes; no container overhead. PSNR uses all
cropped YUV samples against the exact unquantized RGB-to-4:2:0 conversion, so
excludes color conversion/subsampling loss relative to RGBA. Infinity denotes
an actual exact reconstruction. These constructed workloads describe the
bounded skip behavior; they are not claims about typical animation bitrate.

- Flat is gray128, which reconstructs exactly at QP24.
- Local starts with gray128, then changes one raster MB per picture to
  `(x*37+y*13+frame*17)%256` gray. Remaining MBs match the prior reconstruction
  except the previous changed MB. The input generator is centralized in tests.
- Static repeats original `intraImage(...,0)` with gradients/edges/25% noise.
- Mixed uses changing `intraImage(...,frame)` with the same pattern family.

| Pattern | Search | GOP1 bytes / YUV PSNR | GOP33 bytes / YUV PSNR | Skipped / possible P MBs |
|---|---|---|---|---:|
| Flat | false | 47,817 / infinity | 1,801 / infinity | 7,680 / 7,680 |
| Flat | true | 47,817 / infinity | 1,801 / infinity | 7,680 / 7,680 |
| Local | false | 52,809 / 52.584 dB | 6,647 / 65.277 dB | 7,617 / 7,680 |
| Local | true | 52,322 / 57.150 dB | 6,545 / 65.422 dB | 7,617 / 7,680 |
| Static lossy | false | 929,379 / 40.605 dB | 934,179 / 40.605 dB | 0 / 7,680 |
| Static lossy | true | 868,758 / 40.742 dB | 873,558 / 40.742 dB | 0 / 7,680 |
| Changing mixed | false | 928,666 / 40.587 dB | 933,453 / 40.587 dB | 0 / 7,680 |
| Changing mixed | true | 864,581 / 40.730 dB | 869,373 / 40.730 dB | 0 / 7,680 |

At 24fps the GOP33 totals correspond to 10.48 kb/s (flat), 38.67/38.08 kb/s
(local, search false/true), 5,435.22/5,082.52 kb/s (static lossy), and
5,431.00/5,058.17 kb/s (changing mixed). Local's improved quality comes from
preserving exact gray reference MBs rather than spatially requantizing them
next to the changed MB. This is input dependent. When skip is unavailable,
P macroblock type/skip-run syntax costs extra bits, as the static/mixed cases
show; this subset cannot compress general motion efficiently.

Timing environment remains Apple M2, darwin/arm64, macOS15.7.1, Go1.27.3,
CGO_ENABLED=0/noasm, GOMAXPROCS8. Serial `-p 1`, three 300ms samples on a shared
machine. Each benchmark iteration encodes an entire **33-frame group**;
`ns/frame` is measured ns/op divided by33. Table memory/allocations likewise
normalizes B/op and allocs/op by33. Input generation, SPS/PPS generation,
filesystem, muxing and decoder are excluded; RGB conversion, reference matching,
copying, residual encoding and allocations are included. FFmpeg validation was
finished before timing. Shared-machine variance is retained, not discarded.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestPredictiveMeasurements$' -v -count=1 -p 1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -run='^$' \
  -bench='^BenchmarkPredictive$' -benchmem -benchtime=300ms -count=3 -p 1
```

| Pattern / search / GOP | Samples ms/frame | B/frame | Allocations/frame |
|---|---|---:|---:|
| flat / false / 1 | 1.181 / 1.164 / 1.157 | 237,056–237,077 | 11 |
| flat / false / 33 | 0.328 / 0.326 / 0.328 | 235,574–235,575 | 11 |
| flat / true / 1 | 2.462 / 2.468 / 2.591 | 241,280 | 13 |
| flat / true / 33 | 0.373 / 0.369 / 0.369 | 239,799 | 13 |
| local / false / 1 | 1.167 / 1.153 / 1.158 | 237,304–237,305 | 11 |
| local / false / 33 | 0.338 / 0.341 / 0.341 | 235,727 | 11 |
| local / true / 1 | 2.486 / 2.472 / 2.525 | 241,528–241,529 | 13 |
| local / true / 33 | 0.397 / 0.459 / 0.427 | 239,946–239,947 | 13 |
| static / false / 1 | 2.059 / 2.080 / 1.960 | 264,192–264,193 | 11 |
| static / false / 33 | 1.971 / 2.647 / 3.164 | 264,192–264,193 | 11 |
| static / true / 1 | 7.073 / 7.003 / 4.571 | 267,010 | 13 |
| static / true / 33 | 4.570 / 4.554 / 4.556 | 267,008 | 13 |
| mixed / false / 1 | 1.966 / 1.971 / 2.004 | 264,316–264,317 | 11 |
| mixed / false / 33 | 1.969 / 1.975 / 1.962 | 265,061 | 11 |
| mixed / true / 1 | 4.584 / 4.613 / 4.640 | 267,008 | 13 |
| mixed / true / 33 | 4.584 / 4.578 / 4.555 | 267,008 | 13 |

This bounded predictive path is much smaller/faster on these exact-background
workloads; it is not generally faster and offers no temporal size savings on
the repeated lossy/moving-noise examples. ModeSearch remains optional and
costs additional work on coded intra MBs. Future inter residual/motion work
must explicitly extend the supported syntax and reference proof rather than
loosening the current exact-match skip rule.


### GOP60 integration acceptance (2026-11-04)

`NewPredictive`, `PredictiveOptions{QP,ModeSearch,GOP}` and
`PredictiveFrame{NAL,Keyframe}` retain their exact signatures. No production Go
codec, export, CLI or muxer file changed in this acceptance update. The codec
constructor continues to require an explicit GOP in 1..65536; the parent may
choose GOP60 as the default only for the explicitly selected predictive codec.
PCM and existing intra constructors and golden data remain unchanged.

For 125 successful Encode calls with GOP60 and no ForceIDR/Reset, Keyframe is
true exactly at zero-based indices 0,60,120. The corresponding one-based sync
sample numbers are **1,61,121**. Each complete GOP exercises frame_num wraps at
positions16,32,48; IDR restarts frame_num/POC but not the caller's media clock.
Use continuous picture timestamps for MP4/MOV; DTS equals PTS. The codec returns
one NAL per successful picture, with no start code or length. SPS/PPS must come
from this same predictive encoder, including at independent entry points.

`TestPredictiveGOP60ErrorBoundary` additionally rejects malformed frames before,
at and after boundaries (59/60/61 and119/120/121) and compares valid outputs
against a control encoder. Sync sample positions, reference state and bytes do
not shift. Independent FFmpeg acceptance covers 3,250 new GOP60 pictures and
all 78 new IDR entries, with exact visible YUV equality; the expanded totals and
manifest hash above include the original vectors. The h264-package central tests
and targeted vet were rerun. Runtime and performance paths are unchanged, so
this update does not replace or extend the measured GOP33 timing claims.

## Optional zero-motion inter residuals (2026-11-04)

The exact-skip-only predictive bounds above apply with `InterResidual=false`.
`PredictiveOptions` now adds **`InterResidual bool`**, default false. Existing
`NewPredictive`, QP/ModeSearch/GOP fields, `PredictiveFrame{NAL,Keyframe,...}`,
SPS/PPS, reset/recovery and timestamp contracts retain their signatures.
Enable explicitly:

```go
enc, err := h264.NewPredictive(width, height, fps, h264.PredictiveOptions{
    QP: 24, ModeSearch: true, GOP: 60, InterResidual: true,
})
// Handle err. Use enc.SPS()/enc.PPS(), and frame.NAL/frame.Keyframe as before.
frame, err := enc.Encode(rgba)
```

Integration constraints: no new parameter-set or muxer policy is required.
The PPS still uses CAVLC, one active L0 reference, no weighted prediction,
`constrained_intra_pred_flag=0`, and `chroma_qp_index_offset=max(0,4-QP)`.
SPS remains one short-term reference and type-2 POC. DTS equals PTS, using the
caller's continuous successful-picture index, not GOP-local FrameNum/POC.
IDRs still use the original spatial path; periodic/forced IDRs and Reset keep
the original error transaction semantics. `SkippedMacroblocks` now counts both
exact matches and, when enabled, non-exact inputs with all-zero quantized inter
residuals. It must not be interpreted as a count of lossless input matches.
No export/CLI, muxer, animation/composition/scene/succession file was edited.

The P picture now selects among these bounded candidates in raster order:

1. Exact input-converted-YUV match: the original fast P_SKIP path, SSE zero.
2. Otherwise, compute one zero-motion **P_L0_16x16** candidate against the
   actual previous reconstructed YUV. Subtract each reference sample, perform
   the existing 4x4 transform/quantizer, and reconstruct/clamp against that same
   reference. Chroma uses separate 2x2 DC Hadamard/quantization and 4x4 AC.
3. Evaluate one I4x4 candidate using current reconstructed spatial neighbors;
   ModeSearch, if enabled, still compares DC/vertical/horizontal per block.
4. If **every** inter quantized luma/chroma DC/AC coefficient is zero, the
   inter reconstruction equals the reference and a legal P_SKIP represents it
   at lower cost. This is the only additional non-exact skip candidate. A skip
   cannot discard a nonzero quantized coefficient. Intra may still win on cost.

Local cost is **`2048*SSE + step*step*rate_bits`**, with SSE over all 384 padded
YUV samples, `step=inverseScale[QP%6][0] << (QP/6)`. Coded candidates include the
actual `ue(mb_skip_run)` length, macroblock type/prediction/CBP/delta syntax and
exact CAVLC bit counts with current nC. A skip candidate counts
`ueLength(pending_run+1)`. Ties prefer inter/skip. There is no recursive mode
search or motion search, no lookahead, and no optimization for future neighbor
changes, final NAL escape bytes or final picture PSNR/size. Maximum SSE is
384*255^2; SSE and costs use int64. The two macroblock syntax scratch buffers
are 4096 bytes each, allocated once per P picture and reused, plus the existing
optional 128-byte intra-mode scratch. Processing is bounded to the inter trial,
the intra trial, and writing the selected syntax. Quantization uses the existing
one-third dead zone; an encoder is not required to use a different rounding
rule for inter. The decoded inverse and scaling lists are normative.

P_L0_16x16 syntax: mb_type0; no ref_idx syntax with one active L0 reference;
`mvd_l0=(0,0)`; Table9-4(a) **inter** CBP mapping; mb_qp_delta0 only if CBP is
nonzero. Luma CBP gates all four 4x4 blocks in each 8x8 group, with absent groups
inferred zero. Chroma CBP0 has no DC/AC, CBP1 has both DC blocks only, CBP2 has
both DC and both sets of four AC blocks. The encoder never emits reserved
chroma CBP3. Both chroma DC residuals precede all Cb AC and then all Cr AC.
Present-but-zero blocks still write their CAVLC token. A zero-CBP coded inter
MB is syntax-tested but normal selection converts that candidate to P_SKIP.

Motion-vector correctness is an invariant, not an assumed missing syntax:
progressive pictures have one 16x16 L0 partition. Intra/unavailable neighbors
supply mv0/refIdx-1; every previous inter/skip partition has mv0/refIdx0.
All 8.4.1.3 predictor branches therefore return zero, including C-to-D fallback,
reference-index matching and median selection. Explicit mvd0 yields mv0, and
8.4.1.1 infers the same zero vector for skip. No nonzero-vector/partition,
weighted, field, multi-reference or long-term branch is exposed.

Each trial touches only its own MB. Before evaluating intra, inter trial counts
are cleared. If intra loses, its pixel/count/mode changes are replaced by the
selected inter reconstruction, exact selected residual counts and inferred DC
modes. Absent residual groups and skipped MBs supply zero counts. Previous
chosen MBs remain intact, so inter/intra/skip transitions use correct nC and
spatial references. The current MB is the only mutable trial region; a final
picture reference is still committed only after successful complete encoding.
There is no PCM, empty-frame or constant-color fallback.

### Independent reconstruction and regression evidence

FFmpeg8.0.1/libavcodec62.11.100 strictly decoded **428 new InterResidual streams /
6,708 pictures** and **50 fresh IDR-entry suffixes**, comparing every visible
Y/Cb/Cr byte with the encoder's actual reconstruction and checking exact output
length. The same independent strict command is used (`-xerror -err_detect
explode`, raw yuv420p output). The matrix includes:

- Both ModeSearch values; all52 QPs on 34x38 (3x3 MB grid), nine pictures/GOP4
  each for repeated lossy mixed patterns, saturated extremes and chroma edges.
- 2x2,18x22,50x34,320x180 local/drift/checker/chroma patterns at QP0/24/51,
  20 pictures/GOP17 (frame_num wrap followed by periodic IDR).
- 125-picture/GOP60 static/drift/chroma-edge/fully-changing/recovery/flat cases
  on 50x34; forced consecutive IDRs, Reset, and rejected inputs are included.
- The eight exact 320x180/60-picture/GOP60 groups used for the size/quality
  measurements below: four patterns, both ModeSearch settings, QP24.

The pure-Go parser independently decodes CBP from the normative forward table,
MVD, all CAVLC blocks, reconstructs all three **padded** planes and compares with
the retained reference. It observed 56,911 coded inter MBs, 73,409 intra MBs
(including IDR), and 52,691 non-exact quantized-zero skips. **Every nonzero CBP
value1..47** appears in the independently decoded natural-image matrix; the
matrix test asserts this coverage. Synthetic syntax tests additionally cover
all48 inter CBP values including zero, exact bit consumption, counts and
unaligned selected-syntax concatenation. Absent luma/chroma groups and cross-MB
nonzero contexts are checked. Existing exhaustive residual extrema/level-prefix
checks at all QPs remain in the full package suite, covering the same bounded
8-bit difference range used for inter. No coefficients are clamped or truncated.

State tests compare bytes with a control encoder after rejected frames and
ForceIDR/Reset, at all QPs and both ModeSearch settings. Concurrent identical
inputs serialize to the same byte sequence as ordered input, including frame_num
wrap. Enabled/disabled parameter sets match at all QPs. Original PCM, intra,
ModeSearch and predictive false fixtures pass unchanged. Their SHA-256 files
remain `2993d641...`, `4ecd5a33...` and `1dfdd93b...`, respectively; the new
independently verified `inter_vectors.json` hash is
`73ac76a1e775a9ab044926ec82d23a6b63e6858e92ee99e2987c267765f80b8e`.

```sh
CGO_ENABLED=0 GOMATH_H264_INTER_DUMP_DIR=/tmp/gomath-h264-inter \
  go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestInterReferenceVectors$' -count=1 -p 1
python3 tests/_cases/internal/h264/validate_predictive.py \
  /tmp/gomath-h264-inter --manifest inter_vectors.json
# Optional --fixture tests/data/internal/h264/inter_vectors.json installs hashes
# only after all independent byte comparisons pass.
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -count=1 -cover -p 1
CGO_ENABLED=0 go vet -tags noasm ./internal/h264
```

Full owned-package tests passed: **99.7% statement coverage**. Targeted vet and
scoped noasm runtime dependency audit passed: 63 standard-library-only packages,
no native inputs, os/exec or plugin. No race instrumentation or whole-repository
tests were run by this owned slice. Subsequent full/race validation of the integrated option belongs to the parent
integration task. Normal Go tests read verified hashes
and never execute FFmpeg, Python or any subprocess.

### QP24 size, quality and timing bounds

Fixed320x180,24fps,60 pictures/GOP60, matching QP24 and ModeSearch between paired
off/on runs. Sizes include one SPS/PPS and four-byte Annex B start codes, with
no container overhead. PSNR compares all cropped YUV samples to the exact
unquantized conversion, excluding RGB conversion/subsampling loss. Static
repeats original `intraImage(...,0)` (gradients/edges/25% noise); drift adds
frame%5 to each channel, clipped, of that fixed image; local changes one raster
MB against gray128; changing-noise uses `intraImage(...,frame)`. Fixed input
construction and exact source patterns are in central tests.

| Pattern | Search | Bytes off / on | YUV PSNR off / on (dB) | On skip / inter MBs |
|---|---|---:|---:|---:|
| Static lossy | false | 1,698,603 / 29,449 | 40.605 / 40.722 | 14,033 / 127 |
| Static lossy | true | 1,588,383 / 27,534 | 40.742 / 40.844 | 14,053 / 107 |
| Small channel drift | false | 1,698,051 / 111,252 | 40.586 / 40.709 | 1,164 / 12,454 |
| Small channel drift | true | 1,588,395 / 105,322 | 40.721 / 40.711 | 1,698 / 11,863 |
| Local | false | 11,024 / 10,679 | 65.200 / 65.147 | 14,043 / 59 |
| Local | true | 10,828 / 10,693 | 65.383 / 65.213 | 14,043 / 50 |
| Changing noise | false | 1,697,861 / 1,681,135 | 40.591 / 40.566 | 0 / 1,677 |
| Changing noise | true | 1,580,817 / 1,579,237 | 40.734 / 40.731 | 0 / 251 |

The remaining P MBs are coded intra; there are 59*240=14,160 possible P MBs per
group. Unlike the old path, repeated lossy inputs can settle to quantized-zero
skips, retaining actual reconstructed content. For these constructed static
cases, on reduces total bytes about58x, but quality can **decrease** in other
cases because rate/distortion selection trades errors for bits. This is not a
lossless mode, a fixed per-pixel error bound, or a general compression guarantee.
No motion search means moving noise/detailed patterns still largely use intra.

Benchmark setup: AppleM2, darwin/arm64, macOS15.7.1, Go1.27.3, CGO_ENABLED=0,
noasm, GOMAXPROCS8; serial `-p1`, three300ms samples, independent FFmpeg work
finished first. Each iteration encodes the complete60-picture group; `ns/frame`,
B/frame and allocations/frame divide the measured group totals by60. Input
construction, SPS/PPS generation, muxing, filesystem, decoder and pure-Go test
parsing are excluded; conversion, matching, trials, quantization, reconstruction,
selected syntax writes and allocations are included. Shared-machine timings
are indicative rather than latency guarantees.

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestInterMeasurements$' -v -count=1 -p 1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=internal/h264 -run='^$' \
  -bench='^BenchmarkInterResidual$' -benchmem -benchtime=300ms -count=3 -p 1
```

| Pattern / search / inter | Samples ms/frame | B/frame | Allocations/frame |
|---|---|---:|---:|
| static / false / false | 1.931 / 1.930 / 1.930 | 264,192–264,193 | 11 |
| static / false / true | 2.977 / 2.973 / 2.981 | 244,074 | 12.97 |
| static / true / false | 4.453 / 4.451 / 4.579 | 267,008 | 13 |
| static / true / true | 5.537 / 5.530 / 5.537 | 248,272 | 14.97 |
| drift / false / false | 1.935 / 1.938 / 1.937 | 264,192–264,193 | 11 |
| drift / false / true | 3.164 / 3.166 / 3.181 | 245,513 | 12.97 |
| drift / true / false | 4.475 / 4.461 / 4.465 | 267,008 | 13 |
| drift / true / true | 5.708 / 5.757 / 5.725 | 249,659 | 14.97 |
| local / false / false | 0.319 / 0.318 / 0.318 | 235,708 | 11 |
| local / false / true | 0.337 / 0.337 / 0.338 | 243,759 | 12.97 |
| local / true / false | 0.360 / 0.360 / 0.369 | 239,928 | 13 |
| local / true / true | 0.380 / 0.380 / 0.379 | 247,983 | 14.97 |
| changing noise / false / false | 1.944 / 1.944 / 1.942 | 265,216 | 11 |
| changing noise / false / true | 4.561 / 4.594 / 4.577 | 272,247 | 12.97 |
| changing noise / true / false | 4.583 / 4.456 / 4.465 | 267,008–267,009 | 13 |
| changing noise / true / true | 7.140 / 7.043 / 7.592 | 275,063–275,065 | 14.97–14.98 |

On these workloads the extra inter trials are slower, even where bitrate savings
are large. No speedup is claimed. The original all-IDR, chroma minimum, no-HRD
and level/hardware bounds remain. This option extends only progressive,
single-reference zero-motion 16x16 residual prediction; there is no general
motion compensation or additional profile support.

Changed for this slice: new `internal/h264/inter.go`; option/dispatch/comments
in `internal/h264/predictive.go`; package/neighbor-mode comments in
`internal/h264/{pcm.go,prediction.go}`; new
`tests/_cases/internal/h264/inter_test.go`; shared test decoder extended in
`tests/_cases/internal/h264/predictive_test.go`; optional manifest selection
in `tests/_cases/internal/h264/validate_predictive.py`; new
`tests/data/internal/h264/inter_vectors.json`; and this record. Existing three
golden files were not edited. No files outside the codec ownership were changed.

## InterResidual allocation and bit-writer optimization (2026-11-04)

Public APIs, option defaults, mode decisions, QP, cost weights, coefficient
selection, frame order and parameter sets are unchanged. This slice optimizes
the enabled path without changing its compression or reconstruction. Before
editing, a fresh 20-file source/test/fixture/document snapshot was recorded in
`tests/data/internal/h264/perf_inter_20261104/baseline_sources.zip`, with sizes
and SHA-256 values in `baseline_files.json`. This is the baseline for this
section, not the earlier GOMAXPROCS8 measurements above.

### Profile and focused changes

Two-second CPU and allocation profiles were captured before and after for
`static/searchfalse/intertrue` and `noisechange/searchtrue/intertrue`. Baseline
CPU samples attributed 12.84% and 22.22% flat to single-bit writes. Static
allocation space attributed 72.31% to new YUV pictures, with another 14.49%
directly in `encodePResidual`; CPU included 15.60% memory clearing. Profiles
include benchmark setup and profiler allocations, unlike benchmark B/op, so
their percentages are diagnostic and are not per-frame allocation measurements.

- The shared bit writer now appends byte fragments, masks partial writes and
  emits Exp-Golomb/CAVLC zero prefixes in bulk. Selected trial syntax is joined
  by copying aligned bytes or carrying unaligned bytes, instead of bit loops.
  This also affects PCM/intra and InterResidual=false; all bytes remain equal.
- Enabled predictive encoders reuse their source YUV, spare reconstruction,
  count/mode grids, bounded trial buffers and RBSP capacity under the existing
  mutex. Source conversion overwrites every padded sample. Each macroblock's
  trial/selected reconstruction overwrites its complete target region. Counts
  and modes are initialized per picture. Reference and target rotate only on
  successful P encoding and are always distinct. IDR encoding still allocates
  its own buffers; returned NALs still own independent allocations.
- Reset releases the reference and workspace. ForceIDR retains reusable storage.
  Invalid input is rejected before touching workspace or sequence state.

Optimized static CPU memory clearing was 0.97% flat; the remaining main costs
include transform/quantization, intra trials and CAVLC. In the changing-noise
profile, chooseLuma4x4 remained 36.22% cumulative and CAVLC 33.51%; bulk writes
were still 18.38% flat. These sample shares are not additive speedups. No trials,
quality checks or coding branches were removed to improve the measurements.

### Matched five-sample measurements

Apple M2, darwin/arm64, macOS15.7.1, Go1.27.3, **CGO_ENABLED=0, noasm,
GOMAXPROCS=1**, serial `-p 1`, five300ms samples per case. The runner supplies
`-tags noasm`. Baseline and optimized runs use the exact same unchanged
`BenchmarkInterResidual`: fixed320x180, padded320x192, QP24, GOP60, four input
patterns, both ModeSearch values and both InterResidual settings. One benchmark
operation encodes60 frames. Conversion, matching, full inter/intra trials,
quantization, reconstruction, NAL creation and one IDR per group are included.
Input construction, encoder construction, SPS/PPS, muxing, filesystem, parsing
and independent decoding are excluded. Workspace startup is included and
amortized over the benchmark; these are not first-frame/cold-start timings.

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkInterResidual$' -benchmem \
  -benchtime=300ms -count=5 -p 1
python3 tests/_cases/internal/h264/summarize_perf.py \
  tests/data/internal/h264/perf_inter_20261104
```

Enabled cases below show medians of five samples. B/frame and allocations/frame
divide B/op and allocs/op by60. The raw files and generated summary retain
**all five samples and minimum/maximum**, including disabled controls:
`baseline_bench.txt`, `optimized_bench.txt`, `benchmark_summary.json` and
`benchmark_summary.md` in the capture directory.

| Pattern / ModeSearch | Baseline → optimized ms/frame | Time change | Baseline → optimized B/frame | Baseline → optimized allocations/frame |
|---|---:|---:|---:|---:|
| static / false | 3.2441 → 2.8476 | -12.2% | 244,073.5 → 6,457.2 | 12.967 → 1.267 |
| static / true | 6.0259 → 5.2776 | -12.4% | 248,271.7 → 8,606.1 | 14.967 → 1.450 |
| drift / false | 3.2016 → 3.0479 | -4.8% | 245,512.5 → 7,896.3 | 12.967 → 1.267 |
| drift / true | 5.7811 → 5.4869 | -5.1% | 249,658.7 → 9,993.1 | 14.967 → 1.450 |
| local / false | 0.3452 → 0.3331 | -3.5% | 243,758.4 → 4,362.5 | 12.967 → 1.167 |
| local / true | 0.3910 → 0.3724 | -4.8% | 247,982.7 → 4,454.8 | 14.967 → 1.217 |
| changing-noise / false | 4.6423 → 3.8516 | -17.0% | 272,247.5 → 34,631.2 | 12.967 → 1.267 |
| changing-noise / true | 7.1299 → 6.3120 | -11.5% | 275,063.5 → 35,397.9 | 14.967 → 1.450 |

Enabled allocation volume fell87.1%–98.2% for these cases. Disabled controls
retained exactly the previous allocation counts/volumes; their observed timing
medians fell1.9%–14.4%, including the shared writer changes. Runs were sequential
on a shared machine, and baseline timings have visible outliers (for example
static/searchfalse/interfalse spans1.9340–2.6136ms/frame). Five samples do not
establish statistical significance or a general speed guarantee. Enabled inter
remains slower than its optimized disabled control on every measured pattern,
by up to about2.07x; compression/quality tradeoffs remain those documented above.

### Retained-memory cost and correctness

Reduced allocation is paid for with longer-lived buffers. After60 static
320x180 frames, dynamic byte-slice capacities retained by the encoder are:

| Configuration | Retained capacity |
|---|---:|
| baseline enabled, or optimized disabled | 92,160B |
| optimized enabled / ModeSearch false | 321,152B |
| optimized enabled / ModeSearch true | 325,120B |
| any configuration after Reset | 0B |

These are actual slice capacities, excluding struct/slice headers, runtime
allocator overhead, caller images and returned NALs. They are not RSS/peak heap
claims. The enabled encoder retains source plus two reconstruction pictures,
contexts, trial storage and RBSP high-water capacity. Larger dimensions and
more complex frames increase capacity; a long-lived idle instance retains it
until Reset or collection. ForceIDR does not shrink it. The first IDR, GOP1 and
the disabled path do not allocate the optional workspace.

Full owned-package tests passed with **99.7% statement coverage**, including all
existing PCM/intra/ModeSearch/predictive/inter fixtures and all52 QPs. New central
tests compare bulk writes with an independent bit specification over all byte
alignments and widths0..64; check buffer/reference separation across periodic
and forced IDRs, errors and Reset; verify retained capacities; and ensure old
returned NALs survive later encoding and Reset without modification. Existing
synthetic syntax tests cover every concatenation alignment. Serialization and
workspace tests additionally passed three times with GOMAXPROCS8. Targeted vet
and a scoped noasm dependency audit passed:62 standard-library dependencies,
no external/runtime native inputs, os/exec or plugin. Race instrumentation and
whole-repository tests were left to the parent integration task.

Fresh optimized vectors were independently strictly decoded with FFmpeg8.0.1 /
libavcodec62.11.100: **428 streams,6,708 frames,50 fresh IDR entries**, every
visible Y/Cb/Cr byte equal to the encoder reconstruction. The generated manifest
was byte-identical to the existing `inter_vectors.json` (SHA-256
`73ac76a1e775a9ab044926ec82d23a6b63e6858e92ee99e2987c267765f80b8e`). All four
existing golden files still match the fresh baseline hashes. Therefore sizes,
quality, skip/mode decisions and reconstruction for those fixtures are exactly
unchanged, not merely within a tolerance. Independent decoder tooling remains
development-only; normal Go tests require no FFmpeg/Python/subprocess.

Captured evidence includes raw benchmarks, both before/after CPU/allocation
profiles with text tops, source diff and before/after hashes, package/concurrency
test logs, retained-capacity logs, dependency audit and independent decode JSON.
Temporary decoder streams and test binaries were removed after verification.
Production edits are limited to `internal/h264/{pcm,cavlc,intra,inter,predictive}.go`;
new tests/tooling are `tests/_cases/internal/h264/{performance_test.go,summarize_perf.py}`.
Only owned codec/test/data paths and this validation record were changed.

## Opt-in bounded integer motion (2026-11-04)

### API, supported subset and work bound

```go
enc, err := h264.NewPredictive(width, height, fps, h264.PredictiveOptions{
    QP: 24, GOP: 60, InterResidual: true,
    MotionSearchRadius: 2, // zero by default; explicit supported range1..4
})
```

`MaxMotionSearchRadius` is4. A negative radius, radius above4, or nonzero radius
without InterResidual returns an error. `New`, `NewIntra`, SPS/PPS, Encode output
fields, ForceIDR and Reset retain their API/semantics. Radius0 dispatches through
the existing optimized exact-skip/intra or zero-motion residual path; existing
goldens remain unchanged. No export/CLI/root/public-documentation files changed.

This is single-reference, progressive 8-bit full-range4:2:0, one P_L0_16x16
partition per coded inter macroblock, referring to the immediately previous
reconstructed picture. IDRs still use the existing intra encoder. DTS=PTS,
POC2/frame_num4 and periodic/forced IDRs are unchanged. Fractional **luma** MV,
multiple references, other partition shapes, B pictures, field/MBAFF, weighted
prediction, long-term references, rate control and a real-time guarantee are
unsupported. Existing even-pixel geometry2..4096 and fps1..120 limits remain;
odd pixel widths/heights are rejected, while non-16-aligned even display sizes
and odd macroblock grids are supported. No dimension is silently rounded at API.

Every P macroblock evaluates **all `(2R+1)^2` integer-luma vectors** in the square
[-R,R] around the co-located reference position:9/25/49/81 for radius1/2/3/4.
The zero vector is evaluated first; ties then follow dy/dx raster order. Each
candidate performs all24 block transforms/quantization, chroma DC transforms,
actual context-dependent CAVLC, reconstruction and SSE. One existing intra trial
competes with them. There is no SAD shortlist, early exit, hidden radius shrink,
coefficient deletion, increased QP or reduced intra search. For a W×H image,
inter candidate count per P picture is exactly
`ceil(W/16)*ceil(H/16)*(2R+1)^2`, plus one intra trial per MB and final selected
syntax output. At320x180 this is2,160/6,000/11,760/19,440 inter candidates.
The allowed4096x4096 maximum would reach5,308,416 candidates/P picture at R4;
this is a finite computational bound, not an affordable latency promise.

The existing local cost is unchanged: `2048*SSE + step(QP)^2*rate`. Rate includes
the actual signed MVDs, CBP, CAVLC contexts and skip-run syntax. Intra ties favor
inter; a skip candidate is allowed only when its legal inferred vector equals
the evaluated vector **and all quantized coefficients are zero**. No nonzero
coefficient is discarded to create skip. Explicit zero-CBP P_L0_16x16 is legal
and emitted when its vector cannot be represented by skip. This is a local
rate/distortion policy: adding vectors does not guarantee lower frame bytes,
higher PSNR, a fixed per-pixel error bound or global optimality across future
macroblocks/pictures. The additional search is opt-in and the tradeoff is visible.

### Motion and reconstruction rules

Vectors are stored internally in integer-luma units and emitted as quarter-luma
MVDs relative to the actual spatial MVP. The progressive16x16/ref0 predictor
implements H.2648.4.1.3: A/B/C neighbors, C-to-D fallback, unavailable versus
intra/refIdx-1 distinction, A-only availability, unique-ref0 matching and
component-wise median. Skip uses its separate8.4.1.1 inference, including
forced zero when A/B is unavailable or a ref0 neighbor has zero motion. Thus a
zero-motion candidate after a moving neighbor writes the necessary nonzero
difference rather than assuming mvd0. All final inter and skipped vectors enter
the current picture's spatial grid; intra supplies refIdx-1/mv0. The grid is
initialized for every P picture, never carried as a temporal MVP across IDRs.

Integer luma prediction uses direct samples. Odd integer displacements require
half-chroma samples: the8.4.2.2.2 bilinear formula with eighth-chroma fractions0/4
and normative rounding is evaluated for both Cb and Cr. Negative coordinates
use floor division, not truncation. Every reference tap clips separately to the
**coded reconstructed plane**, including its reconstructed padding. Visible crop
dimensions are not used to clip reference samples. Chroma prediction remains
8x8 for each16x16 luma partition. Residual transform, QP-dependent PPS chroma
offset, CAVLC CBP mapping and nonzero-count contexts are shared with the verified
inter implementation; their full[-255,255] residual bounds remain applicable.

The optional spatial motion grid is retained in the mutex-owned workspace.
Reference and target still rotate only after complete successful encoding.
Invalid input is rejected before touching state, ForceIDR preserves its pending
request across errors, and Reset releases the workspace/grid/reference. Returned
NALs own their bytes. Calls serialize; callers still submit frames in display
order and must request an IDR after dropping a successful picture.

### Independent decoding and pure-Go regressions

`tests/data/internal/h264/motion_vectors.json` contains **303 independently
verified streams /5,182 pictures**. FFmpeg8.0.1/libavcodec62.11.100, using
`-xerror -err_detect explode`, decoded every full stream and **825 suffixes
starting at every IDR of every stream**. Exact output lengths and every visible
Y/Cb/Cr byte matched internal reconstruction; no error tolerance is used.
Manifest SHA-256:
`61964cc73100801da8f02c5c7e49ee2238258b26d56466d0117f2b946876299c`.
`motion_independent_decode.json` retains this result. The matrix includes:

- All52 QPs and both ModeSearch values on34x38/3x3 MB pictures, with moving
  detailed images and saturated chroma edges,9 frames/GOP4, radius1/2.
- Radius1..4, QP0/24/51, both ModeSearch values,2x2/18x22/50x34, bidirectional
  integer translation with odd displacements,20 frames/GOP17 (frame_num wraps).
- Both ModeSearch values,125 pictures/GOP60 at34x38: translated content,
  mixed moving tiles, chroma edges, static input and forced/consecutive IDRs,
  Reset and rejected-frame recovery. GOP1 and GOP65536 also have vectors.
- All nine320x180/60-picture/QP24/ModeSearchfalse groups in the measurements
  below, including their radius0 controls.

Normal Go tests read these verified hashes and use an independent central
syntax decoder: it reads MVDs, predicts in quarter-luma units, interpolates with
the denominator64 formula and compares **all padded reconstructed planes**.
Additional tests compare encoder/decoder prediction across availability,
intra/ref0, signs and median branches; compare every displacement in[-4,4]^2
at every MB of16x16/32x48/48x32 reference pictures, including all four clipped
borders; reject invalid radius/configuration/geometry/stride; compare control
encoders after failures/ForceIDR/Reset at every QP and both ModeSearch settings;
and verify concurrent serialization, Reset release and NAL byte ownership.
No normal Go test or production code runs FFmpeg, Python or a subprocess.

Full owned-package regression passed with **99.8% statement coverage**, including
all four existing golden manifests unchanged. The new matrix's syntax decoder
observed54,375 coded inter MBs,63,564 intra MBs,46,862 nonzero-motion coded MBs,
42,497 odd-chroma-motion coded MBs,12,959 nonzero MVD pairs and3,289 nonzero-motion
skips. Targeted vet and scoped CGO0/noasm dependency audit passed:62 standard
library dependencies plus the codec, no external/native inputs, os/exec or
plugin. Concurrency/sampling checks also passed with GOMAXPROCS8. No
whole-repository or race-instrumented run was performed in this owned slice.

```sh
CGO_ENABLED=0 GOMAXPROCS=1 \
GOMATH_H264_MOTION_DUMP_DIR=<absolute-owned-temporary-directory> \
  go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestMotionReferenceVectors$' -count=1 -p 1
python3 tests/_cases/internal/h264/validate_predictive.py <dump-directory> \
  --manifest motion_vectors.json --all-idr
# Install the new golden only after every independent comparison succeeds.
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -count=1 -cover -p 1
CGO_ENABLED=0 go vet -tags noasm ./internal/h264
```

### Fixed-picture size, quality and five-sample timing

AppleM2, macOS15.7.1, darwin/arm64, Go1.27.3, CGO_ENABLED=0, noasm,
GOMAXPROCS1, serial-p1. All cases use320x180/24fps, QP24, ModeSearchfalse,
InterResidualtrue, GOP60 and60 frames. The radius0 comparison uses the existing
zero-motion encoder. Sizes include SPS/PPS and four-byte Annex B start codes,
without mux overhead. PSNR compares all cropped converted YUV samples, excluding
conversion/subsampling loss. Pan samples a fixed larger detailed image with
frame%11 horizontal and frame%7 vertical offsets; wraps deliberately exceed the
search radius. Static repeats the existing fixed gradient/edge/noise image;
noisechange regenerates the existing changing-noise image each frame.

Timing is the median of **five300ms samples per case**, each operation encoding
60 frames. Setup, encoder creation, SPS/PPS, filesystem, parsing, decoder and
muxing are excluded; source conversion, all candidates, intra, selected syntax,
reconstruction, NAL allocation and periodic IDR are included. Workspace startup
is amortized over the measured operations; some expensive cases have only one
operation per sample. Raw samples/minimum/maximum and B/frame/allocations/frame
are retained in `motion_bench.txt`, `motion_summary.json` and `motion_summary.md`.

| Pattern | Radius | Total bytes | YUV PSNR dB | Median ms/frame |
|---|---:|---:|---:|---:|
| pan | 0 | 1,697,133 | 40.564473 | 4.0857 |
| pan | 1 | 851,798 | 40.936842 | 21.0670 |
| pan | 2 | 754,948 | 40.888852 | 55.0559 |
| static | 0 | 29,449 | 40.721709 | 2.9146 |
| static | 1 | 29,463 | 40.739302 | 19.5235 |
| static | 2 | 29,462 | 40.738587 | 53.3399 |
| changing-noise | 0 | 1,681,135 | 40.566387 | 3.9149 |
| changing-noise | 1 | 1,666,822 | 40.580109 | 21.1112 |
| changing-noise | 2 | 1,602,981 | 40.552949 | 55.2613 |

Pan bytes fall49.8096%/55.5163% at radius1/2, while time grows5.16x/13.48x.
Static grows14/13 bytes and costs6.70x/18.30x time. Changing-noise saves only
0.8514%/4.6489%, costs5.39x/14.12x time, and radius2 **reduces** PSNR0.013438dB.
There is no claim of a general quality, compression or speed improvement. The
full search intentionally avoids an undisclosed fast/low-quality shortlist.
Radius3/4 correctness is independently verified but not included in this timing
table; their larger81-candidate maximum is not claimed to be real-time.
Shared-machine sequential timings are indicative, not latency guarantees.

Measured allocation volumes for radius0/1/2 are respectively36,269.9/23,131.7/
21,190.4B/frame (pan),6,457.5/8,594.1/8,594.1 (static), and34,631.5/36,768.0/
35,406.9 (noise). Radius1/2 used1.400 allocations/frame versus radius0's1.267
in these captures; initialization amortization differs with operation count.
Motion vectors add one retained `motionInfo` per MB (240 slots here), plus the
existing workspace and RBSP high-water capacity. Most candidate data stay in
bounded stack/compact block storage; no candidate-sized full reference copy is
retained. Reset releases the grid. This does not claim reduced idle memory/RSS.

```sh
CGO_ENABLED=0 GOMAXPROCS=1 GOMATH_H264_MEASURE=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^TestMotionMeasurements$' -v -count=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkMotionSearch$' -benchmem \
  -benchtime=300ms -count=5 -p 1
python3 tests/_cases/internal/h264/summarize_motion.py
```

The log-only `TestMotionMeasurements` is development-only and skips unless
`GOMATH_H264_MEASURE=1` is explicitly set. Existing historical raw captures above
are unchanged. All deterministic ReferenceVectors and independent-decoder
fixtures remain complete in the default suite, including race runs; no
correctness matrix is gated or reduced for performance collection.

Production changes: new `internal/h264/motion.go`, opt-in API/dispatch in
`predictive.go`, shared signed-MVD writer and optional grid in `inter.go`, and
package comment in `pcm.go`. Central tests add `motion_test.go` and
`summarize_motion.py`, extend the existing decoder in `predictive_test.go` /
`inter_test.go`, and add opt-in `--all-idr` to `validate_predictive.py`. New golden,
raw measurements, independent decode output and test logs live only under
`tests/data/internal/h264`; this validation file is the only documentation edit.

## Byte-identical exhaustive motion optimization — 2026-11-04

`PredictiveOptions`, `MotionSearchRadius`, defaults and supported geometry are
unchanged. The search still transforms, quantizes and reconstructs every
candidate in the same order, using the same exact local rate/distortion cost
and tie rules. No candidate shortlist, early termination, changed QP or reduced
correctness matrix was introduced.

Fresh before sources/hashes, raw five-sample timings, separate CPU/allocation
profiles, after sources/hashes, test logs and decoder results are preserved in
`tests/data/internal/h264/perf_motion_20261104/`. The 26-file before archive
matches every recorded SHA256. The benchmark body itself is unchanged. Previous
motion captures outside this directory are historical and were not replaced.

The before pan/r2 and changing-noise/r2 CPU profiles identified temporary
candidate CAVLC byte writing: `bits.uint` alone accounted for26.55%/22.58% of
CPU samples, and CAVLC for45.17%/38.71% cumulatively. The changes are:

- `cost.go` counts the exact syntax length of a candidate, including context
  dependent tokens, signs, adaptive levels, zeros, runs, MVD, CBP and QP delta.
  It updates the same current-MB nonzero contexts. Selected syntax still uses
  the original byte writer. Its normative size tables are shared with that
  writer; mirrored traversal/level logic is a maintenance cost, covered by
  differential tests rather than treated as an approximate rate estimator.
- `makeInterMB` quantizes directly into the returned luma/chroma coefficient
  arrays, removing a separately zeroed temporary and copies. All transform,
  quantization, reconstruction and SSE arithmetic is unchanged.
- Motion trials no longer rewrite the unused current-MB intra modes, and motion
  encoders no longer allocate the obsolete4096-byte inter-trial byte buffer.
  Final selection still clears/updates modes and contexts normally. Radius0
  retains its original trial writer and buffer.

After profiles show candidate syntax counting at21.47%/19.17% cumulative CPU
samples and `bits.uint` at0.52%/0%; remaining work is primarily transform,
quantization and compensation. Sample percentages have different denominators
and are not speedup measurements. Allocation profiles are sampled; the exact
4096-byte/one-allocation startup reduction is established by benchmark counters
and a workspace/Reset regression, not by inferring small objects from pprof.

### Matched workload and observed performance

AppleM2, macOS15.7.1(24G231), Go1.27.3, darwin/arm64, CGO_ENABLED=0,
noasm, GOMAXPROCS1, serial-p1. Each case is320x180, QP24, ModeSearchfalse,
InterResidualtrue, GOP60,60 frames/operation, five300ms samples. Inputs, encoder
construction, SPS/PPS, mux, decoder and filesystem are excluded. Conversion,
every candidate, intra comparison, selected bitstream, reconstruction and NAL
allocation are included. First-use workspace allocation is included/amortized;
expensive radius1/2 samples each contain only one operation. CPU/allocation
profiling was a separate2s capture, outside the timing samples.

These sequential shared-machine captures are indicative. A parent integration
race process was active during the before capture and had exited before the
after capture. Consequently the full before/after percentages include possible
load differences and are not an isolated causal estimate or latency guarantee.
The separate unchanged-writer versus exact-counter microbenchmark below checks
the targeted operation under the same post-change measurement conditions.

| Pattern/radius | Before median ms/frame | After median ms/frame | Time change |
|---|---:|---:|---:|
| pan/r0 | 4.1871 | 4.0603 | -3.0% |
| pan/r1 | 21.8373 | 15.9918 | -26.8% |
| pan/r2 | 56.4084 | 40.1229 | -28.9% |
| static/r0 | 2.9864 | 2.9253 | -2.0% |
| static/r1 | 20.0658 | 15.2703 | -23.9% |
| static/r2 | 54.4977 | 39.6795 | -27.2% |
| changing-noise/r0 | 4.0089 | 3.9134 | -2.4% |
| changing-noise/r1 | 21.6214 | 15.8033 | -26.9% |
| changing-noise/r2 | 57.1778 | 40.1812 | -29.7% |

All five individual samples, minimum/maximum, bytes and allocations per frame
are retained in `benchmark_summary.json` / `benchmark_summary.md`, generated by
`tests/_cases/internal/h264/summarize_motion_perf.py`. Radius1/2 allocations
fall84→83 per60 frames; bytes fall4096 per operation in these one-operation
samples. For example pan/r2 falls1,271,424→1,267,328B/op. Radius0 allocation
counts are unchanged; an8B spread in static/r0 bytes is measurement noise.
The removed buffer saves4096 retained bytes per enabled motion encoder, with
no additional cache. This does not claim reduced RSS or zero allocation.

Paired post-change exact-MB syntax microbenchmarks (five300ms samples, same
MVD/context/reset, preallocated original byte writer, zero allocations for both):

| Coefficients | Original writer median ns/op | Exact counter median ns/op |
|---|---:|---:|
| zero | 69.89 | 41.91 |
| sparse | 619.0 | 343.7 |
| dense | 3907 | 1348 |

All nine opt-in size/PSNR measurements exactly match the previous motion table.
For pan/r2 this remains754,948 bytes/40.888852dB, static/r2 remains29,462 bytes/
40.738587dB, and changing-noise/r2 remains1,602,981 bytes/40.552949dB. The
optimization changes neither compression ratio nor reconstruction quality.
Radius2 still costs approximately9.9–13.6 times radius0 in these after captures;
exhaustive search remains expensive. Radius3/4 correctness is retained but no
timing or universal speed claim is made for those workloads.

### Regression and independent decoder evidence

- Complete owned-package suite: **PASS,46.518s,99.8% statement coverage**,
  including all52 QPs, all supported radii, both ModeSearch settings, state,
  error recovery, crops, borders and five unchanged golden manifests.
- Differential CAVLC tests check every coefficient-presence mask for4/15/16
  coefficients, rotating signs/contexts/alignment; random signed levels and all
  supported suffix thresholds; complete inter syntax for all48 CBPs, borders,
  interior positions and MVD combinations; actual quantizer output at every QP.
  Bit lengths, TotalCoeff and full nonzero-count grids match the original writer.
- Fresh optimized output: **303 streams,5182 frames,825 fresh IDR suffixes**
  strictly decoded by FFmpeg8.0.1/libavcodec62.11.100 with `-xerror
  -err_detect explode -threads 1`. Every cropped Y/Cb/Cr byte matches internal
  reconstruction. The regenerated manifest is byte-identical to the existing
  motion golden, SHA256
  `61964cc73100801da8f02c5c7e49ee2238258b26d56466d0117f2b946876299c`.
  Temporary streams were removed after verification; the result and manifest
  comparison are recorded in the capture directory. No fixture was promoted.
- GOMAXPROCS8 concurrency, returned-NAL ownership, workspace lifetime/Reset,
  retained capacity, sampling and state checks: **PASS,2.036s**. Targeted vet
  passed. Scoped CGO0/noasm dependency audit found62 standard-library packages
  plus the codec, no external packages, cgo, codec native/assembly inputs,
  `os/exec` or `plugin`. Standard Go runtime assembly is outside the codec scope.
- `TestMotionMeasurements` skips by default (verified0.005s); explicit
  `GOMATH_H264_MEASURE=1` executes all540 frames and reproduces all nine
  historical size/PSNR values (11.758s). No deterministic correctness test was
  gated, shortened or conditionally skipped for race. The parent reported its
  prior integration race pass487.432s; no new whole-repository or race run is
  claimed by this slice.

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkMotionSearch$' -benchmem \
  -benchtime=300ms -count=5 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -run='^$' -bench='^BenchmarkMotionSyntax(Writer|Counter)$' -benchmem \
  -benchtime=300ms -count=5 -p 1
python3 tests/_cases/internal/h264/summarize_motion_perf.py \
  tests/data/internal/h264/perf_motion_20261104
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=internal/h264 \
  -count=1 -cover -p 1
```

This slice changes `inter.go`, `motion.go`, adds `cost.go`, gates the log-only
measurement in central `motion_test.go`, adds central `motion_performance_test.go`
and `summarize_motion_perf.py`, and updates this validation record and the owned
test README. API/defaults, export/CLI and all historical golden data are unchanged.
