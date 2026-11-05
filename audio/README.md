# audio

标准库实现的不可变 PCM、RIFF/WAVE（含 Microsoft ADPCM / IMA ADPCM、G.711 A-law/μ-law 解码）、native FLAC 编解码、AIFF/AIFC 与 Sun/NeXT AU PCM/G.711 解码、剪辑、混音、显式声道矩阵、拼接/重复、增益包络和带限重采样基础。
运行时没有 cgo、native 库、外部程序或其他 Go 模块依赖。

## PCM 与编辑 API

```go
p, err := audio.NewPCM(audio.Format{
    SampleRate: 48000,
    Channels: 2,
    ChannelMask: 3, // 可选：前左、前右；0 表示仅有声道索引
}, []float64{0.25, -0.25, 0.5, -0.5})
if err != nil { return err }

quiet, err := p.Gain(0.5)
if err != nil { return err }
mixed, err := p.Mix(quiet, 0.1) // 秒；重叠求和，空隙补零，延长结果
if err != nil { return err }
faded, err := mixed.Envelope([]audio.GainPoint{
    {Time: 0, Gain: 0},
    {Time: mixed.Duration(), Gain: 1},
})
if err != nil { return err }
converted, err := faded.Resample(44100, audio.ResampleOptions{})
if err != nil { return err }
return audio.WriteWAV(dst, converted, audio.WAVWriteOptions{Encoding: audio.Float32})
```

| API | 行为 |
| --- | --- |
| `NewPCM(Format, []float64)` / `NewPCMWithLimits(Format, []float64, Limits)` | 验证后复制交错样本 |
| `Format`, `SampleRate`, `Channels`, `Frames`, `Duration` | 返回值类型元信息；时长单位为秒 |
| `Samples()` / `Sample(frame, channel)` | 返回样本副本 / 带索引检查的单个样本 |
| `Trim(start, end)` / `TrimFrames(start, end)` | 复制半开区间；越界报错 |
| `Mix(other, seconds)` / `MixWithLimits(other, seconds, Limits)` | 非负时间偏移混音 |
| `MixFrames(other, offset, Limits)` | 用精确帧偏移混音 |
| `Remix(output Format, matrix [][]float64, RemixOptions)` | 显式输出声道 × 输入声道矩阵，保持采样率和帧数 |
| `Concat(others []*PCM, Limits)` | 复制接收者及后续片段，格式必须完全相同 |
| `Repeat(count int, Limits)` | 复制 count 个连续片段；0 次或空输入得到保留格式的空结果 |
| `Gain(factor)` / `Envelope([]GainPoint)` | 线性增益 / 分段线性包络 |
| `Resample(rate, ResampleOptions)` | Blackman 窗 sinc 重采样 |
| `ReadWAV(io.Reader, WAVReadOptions)` / `WriteWAV(io.Writer, *PCM, WAVWriteOptions)` | 流式有界 RIFF/WAVE I/O |
| `WriteFLAC(io.Writer, *PCM, FLACWriteOptions)` | 有界 PCM16/24 mono/stereo 无损 FLAC 编码 |
| `ReadFLAC(io.Reader, FLACReadOptions)` | 有界严格 native FLAC 解码，返回独立不可变 PCM |
| `ReadAIFF(io.Reader, AIFFReadOptions)` | 有界 AIFF / AIFC NONE、sowt 整数 PCM 解码 |

每帧按声道索引交错存储：`samples[frame*Channels+channel]`。
非零 `ChannelMask` 按 WAVE 扬声器位 0..17 的升序定义声道，置位数必须与声道数一致。
采样率为 1..768000，声道数为 1..64。构造时必须提供完整帧，全部样本必须 finite；允许超过 ±1 的混音余量。

`PCM` 的零值无效，应通过构造器或解码器创建；合法空片段保留格式，帧数为零。
输入切片会被复制，`Samples` 返回副本，每项编辑返回独立样本存储。
构造完成的 PCM 可并发读取、编辑和编码；调用者仍须独立管理传入的 reader/writer 和控制点切片。
获取元信息的方法用于合法构造的 PCM；接受 PCM 的编辑和 I/O 方法会检查 nil/零值。

时间参数按最近整帧取整，半帧向上；精确编辑使用帧 API。
混音要求采样率、声道数及 mask 完全一致，不自动重采样、复制声道、降混或削波。
负偏移被拒绝；空叠加片段不增加时长。
包络接受 1..4096 个时间严格递增、位于 `[0,Duration]` 内的有限控制点；区间外保持端点增益，各声道共享包络。
负增益反转极性；增益、混音或滤波结果非有限时返回错误。

## 显式声道矩阵与序列编辑

```go
// 立体声降为单声道的权重由调用者指定。这不是默认降混标准。
mono, err := p.Remix(audio.Format{
    SampleRate: p.SampleRate(), Channels: 1, ChannelMask: 4,
}, [][]float64{{0.5, 0.5}}, audio.RemixOptions{})
if err != nil { return err }
loop, err := mono.Repeat(3, audio.Limits{})
if err != nil { return err }
joined, err := mono.Concat([]*audio.PCM{loop, mono}, audio.Limits{})
if err != nil { return err }
_ = joined
```

`matrix[out][in]` 必须恰为输出声道行数 × 输入声道列数，拒绝缺行、多行、参差列和非有限系数，包括空输入。
输入/输出都为 1..64 声道，最多 4096 个系数；输出 `Format.SampleRate` 必须等于输入，采样率转换需另行调用 `Resample`。
矩阵按输入/输出各自的声道索引解释，非零 mask 使用扬声器 bit 升序。
输出 mask 由调用者完整声明，不根据矩阵猜测或继承：0 明确表示不带扬声器分配的索引声道；非零仍须符合 `Format` 规则。
这允许显式删除、指派或变更布局；调用者负责使系数与所声明的扬声器含义一致。
不会自动下混、归一化、削波或加 fade；可使用负权重、全零行、重复路由和超过 ±1 的 finite headroom。

每个输出样本从零开始，依输入声道升序逐项乘加，乘积和累加分别舍入到 float64，不使用融合乘加。
每个中间乘加结果必须 finite；即使后续项可能抵消，中间溢出仍报 `ErrInvalidInput`，不会返回部分 PCM。
返回值不保留调用者矩阵；调用期间须保持矩阵不被修改，并发调用可共享只读矩阵。
`RemixOptions.MaxWork` 默认 `512<<20`，精确预检成本为 `inputChannels * outputChannels * (1+frames)`，含一次系数验证及每项乘加，零权重也计费。
采用除法预检，避免预算乘积溢出；超限返回 `ErrLimit`，没有低质量或部分声道回退。

`Concat` 按接收者、`others[0]`、`others[1]` 顺序直接复制；零后续片段也复制接收者。
所有片段（包括空片段）的采样率、声道数和 mask 必须完全相同，nil/零值片段报错。
`MaxConcatClips=4096` 包含接收者和空片段，确保空片段列表也有验证工作上限；总帧数逐段检查，之后只分配一份输出样本数组。
`Repeat` 接受非负整数；负数报错，0 次或空输入在常数工作内返回保留格式的空 PCM，空输入可安全接受 `MaxInt` 次。
非空结果在分配前通过除法检查帧数乘积，再检查样本数及架构的字节容量上限；使用倍增复制，耗时线性于输出样本数，不进行 count 次空循环。
两项操作保持样本逐位不变，包括负零和有限极值，1 次重复、身份矩阵和单片段拼接均拥有独立样本。

三项新操作的 `Limits` 约束输出帧数与 float64 样本数，默认 16M 帧且 16M 样本（最多 128 MiB 输出数组）；不限制调用者已持有的输入总内存。
`MaxBytes`/`MaxChunks` 仅适用于编码流，正值不限制这些 PCM 操作，负值仍为无效选项。
`Concat` 工作量为片段数加输出样本数，`Repeat` 为输出样本数；两者没有额外可调工作预算，由输出上限和片段上限约束。
所有结果独立、输入不变，可并发编辑同一 PCM。所有验证错误都返回 nil 结果。

## WAV 支持和拒绝策略

支持小端 `RIFF/WAVE`：有符号 PCM16/24/32、IEEE float32/64；以及完整 valid bits、合法扬声器 mask 的 `WAVE_FORMAT_EXTENSIBLE`。
这些 PCM/IEEE 格式允许 fmt 长度 16/18（无扩展）或 40（extensible），检查 block align、byte rate、帧对齐、chunk 长度及奇数 payload 的 pad。
`fmt` 必须在唯一的 `data` 前；PCM/IEEE 的可选 `fact` 必须恰为四字节且帧数一致。另支持下述 tag `0x11` IMA ADPCM 子集及 tag 6/7 G.711。
读取严格限制在声明的 RIFF 边界内，后续流字节留给调用者。

写出时，有 mask 或超过两声道使用 extensible；float 使用 `fact`。
默认输出 PCM16，float64 可逐位往返包括负零和有限非规格化值；float32 进行正常精度量化，但拒绝溢出。
整数输出仅接受 `[-1,1]`，按最近整数取整，半数远离零；`-1` 对应最小代码，`+1` 对应最大正代码。
不会自动削波或归一化；有余量的混音应先显式降低增益，或输出 float。
无抖动。编码参数、范围、资源检查在第一笔写入前完成；I/O 错误可能留下部分文件。

`JUNK`、`PAD ` 填充块会跳过。含 `LIST/INFO`、`bext`、`iXML`、`axml` 或 ID3 元数据时，默认报错，避免读写过程中静默丢注释。
仅当调用者明确设置 `WAVReadOptions{DiscardMetadata: true}` 时丢弃这些已知元数据。
未知块、`LIST/wavl`、`slnt`、播放列表、采样循环、多段 data、缩减 valid bits、除下述 IMA/G.711 子集外的压缩编码、PCM8、RIFX、RF64 均明确拒绝。
本包不提供 MP3/AAC 解码、自动声道重映射、Scene 集成或容器复用。

### Microsoft IMA ADPCM（WAVE_FORMAT_DVI_ADPCM，0x11）

继续使用 `ReadWAV(src, WAVReadOptions{...})`，`WAVReadOptions`、`Limits` 和既有 PCM/float API 不变；没有新选项或编码器，也没有外部程序 fallback。
仅支持 4-bit mono/stereo，保持文件声道索引顺序，`ChannelMask=0`；每个输出有符号 16-bit 代码除以 32768，最小值为 -1、最大值为 32767/32768。
这是有损编码格式的真实解码，不保证与编码前原始 PCM 相等。

`fmt` 必须恰为 20 字节、`cbSize=2`、`wBitsPerSample=4`。
`blockAlign` 必须至少 `4*channels`，且为 `4*channels` 的倍数；
`samplesPerBlock` 必须恰为 `1 + (blockAlign - 4*channels)*2/channels`，且能由文件的 16-bit 字段表示。
不假设块长为 256/512/1024 或二次幂；允许只有 predictor 头的单帧块。
采样率保持 1..768000 Hz 范围。压缩 `nAvgBytesPerSec` 作为平均码率提示，不反推样本数或时长；FFmpeg 会写入与块参数不一致的目标码率。
仅当 `floor(sampleRate*blockAlign/samplesPerBlock)==0` 时接受零平均码率，以支持极低合法整数采样率；常规采样率下零值报错。
原有 PCM/IEEE 的 byte rate 精确一致性检查保留。

每块读取各声道的 little-endian signed predictor、0..88 step index 和必须为零的 reserved byte；初始 predictor 本身就是该块第一帧。
其后每组先读取左/唯一声道四字节，再读取右声道四字节，各字节 low nibble 先解码，得到每声道八个新样本。
各声道的 predictor/index 独立，块头重新初始化，不从上一块继承；每步将 predictor 饱和到 [-32768,32767]、index 限到 [0,88]。
全部块头均验证，包括被 `fact` 裁尾的块；失败始终返回 nil PCM。

必须有唯一且恰为四字节的 `fact`；它可在 `fmt`/`data` 前后出现，表示每声道实际帧数。
空 `data` 只接受 `fact=0`；非空时必须 `(blocks-1)*samplesPerBlock < fact <= blocks*samplesPerBlock`，只允许最后一块含尾部填充样本。
不会让 `fact` 隐去完整块、虚增样本或改变 RIFF/data 边界；全部编码字节与所有块仍完整读取。
`data` 必须含完整的 `blockAlign` 块；即使短末块碰巧含完整 nibble 分组，本轮仍明确返回 `ErrUnsupported`，不猜测其终止语义。
3-bit DVI、超过两声道、Microsoft ADPCM `0x02`、其他 IMA/Apple/QuickTime 变体及 extensible 压缩 GUID 均不支持。
缺失/矛盾 `fact`、坏 index/reserved、块布局不一致为 `ErrInvalidInput`；未知扩展、压缩和短末块为 `ErrUnsupported`。
metadata opt-in、RIFF pad/完整读取、后续流保留与既有 WAVE 行为相同，没有静默丢弃未知 chunks。

`Limits.MaxBytes` 包含整个 RIFF，`MaxChunks` 包含 `fact`。
在读取 data payload / 分配输出前，按 **全部完整块的解码容量** 检查 `MaxFrames` 与 `MaxSamples`，包括最终被 `fact` 裁掉的尾部样本；不能用小 `fact` 绕过解码工作或存储预算。
输出拥有该容量的一份 float64 数组，返回切片的长度为 fact 帧数；裁尾不再复制，底层数组仍保留最后一块的容量。
额外 scratch 仅一个 `blockAlign` 字节缓冲，不缓存整条编码流，也不新增 `MaxWork`；循环工作由编码字节与上述样本容量共同限制。
底层 I/O 错误保留 `errors.Is` 链，格式没有校验和，合法 nibble 被更改不一定能检测。

#### IMA 独立验证与成本（2026-11-04）

`tests/reference/audio_ima_adpcm.py` 是开发工具，使用 FFmpeg 8.0.1、libsndfile 1.2.2（Python soundfile）及独立 1994 规范 oracle；普通测试只读取已经保存的 fixtures。
规范为 Microsoft Multimedia Standards Update Revision 3.0 的 DVI ADPCM 部分（pp.28-34），来源 URL、SHA256、工具版本、每个输入与完整输出哈希保存在 `tests/data/audio/ima_adpcm/manifest.json`。
37 个输入含 FFmpeg 与 libsndfile 实际编码文件、全部 89 个初始 index × 16 个 nibble × 三个 predictor 极值/零、两声道不对称数据、65528 字节大块、header-only 块、1/768000 Hz、空音频及 fact 在后的裁尾。
Go 默认测试逐个比较全部 287069 个返回标量样本，严格等于独立规范输出；全部 292300 个编码容量样本的规范 oracle 与 libsndfile 完整解码一致。

**不会声称与 FFmpeg 输出逐样本普遍一致。** 1994 规范和 libsndfile 分别计算 `step>>3`、`step>>2`、`step>>1` 后相加；FFmpeg 使用合并乘法后右移，低位舍入不同。
本实现遵循规范，最小反例为 step=7/code=1：规范增量 1，FFmpeg 增量 2。
37 个流均经 FFmpeg `-xerror -err_detect explode` 完整解码，完整输出精确等于独立乘法 oracle；其中 31 个出现舍入差异，本矩阵最大代码差 32，共 121887 个容量样本不同，不作为其他输入的误差上限。
FFmpeg 和 libsndfile 的本次完整解码还都输出编码块容量，17 个带裁尾输入多出 padding；脚本保留完整输出 hash/长度，并另按解析出的 `fact` 核对规范音频，未用 FFmpeg 时长/裁剪参数隐藏差异。
只有 4 个输入的 FFmpeg 完整长度和全部 PCM 都与本实现相同（另两个仅舍入相同，但仍有填充）；不能把这几个样本推广到其他文件。

```sh
python3 tests/reference/audio_ima_adpcm.py # 开发独立检查；需 FFmpeg、soundfile、numpy
CGO_ENABLED=0 env -u GOMATH_IMA_REFERENCE_DIR go run ./tests/cmd/run -pkg=audio -count=1
CGO_ENABLED=1 GOMAXPROCS=8 go run ./tests/cmd/run -pkg=audio -race -count=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio \
  -run='^$' -fuzz='^FuzzWAVIMA$' -fuzztime=20s -parallel=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio \
  -run='^$' -bench='^BenchmarkReadWAVIMA$' -benchmem -benchtime=300ms -count=5 -p 1
```

测试源/benchmark 均在 `tests/_cases/audio/wav_ima_test.go`，fixture 默认从 `go.mod` 定位中央目录，保留 `GOMATH_IMA_REFERENCE_DIR` 开发覆盖。
完整 audio（含既有 PCM/float/WAV/FLAC/AIFF）95.6% statement coverage，新 IMA 生产文件 100%；完整 audio race 通过，20s 专项 fuzz 962958 次执行通过。
最终显式清除 IMA/AIFF fixture 环境变量的完整回归为 1.133s；完整 audio race 为 9.367s。
`tests/reference/audio_wav_regression.py` 另复验既有 WAV writer 的五种编码 × mono/stereo/5.1，共 15 个流、135 个标量样本，经 FFmpeg 严格完整解码全部字节、长度和布局一致。
旧 PCM/IEEE 的可选 fact 仍须精确等于 data 帧数，不应用 IMA 的末块裁尾规则；原有默认测试和独立 fixtures 均保留。
另通过包 vet、CGO0/noasm Linux/386 构建；依赖审计为 89 个标准库包加 audio，无新模块、cgo、audio 汇编或 os/exec/plugin。
race 的 CGO1 仅供 Go race instrumentation；解码与默认测试使用 CGO0/noasm。

Go1.27.3 / darwin-arm64 / Apple M2 / macOS15.7.1，CGO0/noasm/GOMAXPROCS1、串行 `-p 1`，五次 300ms 样本：
每次解码 48000 Hz、48000 帧，mono/stereo、256/1024 字节块，固定 seed 的 nibble 字节覆盖不同符号/step 变化（并非自然音乐质量基准）。
计时含完整 RIFF/fact 校验、尾部解码、PCM 与 scratch 分配，排除输入构造、磁盘、程序启动与外部 codec。

| 声道/块字节 | 中位 ms/op | 五次范围 ms/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| mono / 256 | 0.197349 | 0.195518..0.210567 | 393704 | 14 |
| mono / 1024 | 0.202115 | 0.197991..0.234514 | 394472 | 14 |
| stereo / 256 | 0.339100 | 0.338466..0.343309 | 770536 | 14 |
| stereo / 1024 | 0.350784 | 0.349173..0.430001 | 787688 | 14 |

原始数据在 `bench.txt`，汇总在 `benchmark_summary.json`。B/op 为分配总量，不是 RSS/峰值；较大块可能因末块填充保留更多 PCM 容量。
这是共享机器上的限定观察，没有与其他解码器作速度比较，也不承诺普遍性能或压缩画质。

### G.711 A-law / μ-law 解码（tag 6 / 7）

使用既有 `ReadWAV(io.Reader, WAVReadOptions) (*PCM, error)` 自动识别，options、Limits 和默认值不变。
本切片只增加解码；`WAVEncoding` 与 writer 仍仅支持原有 PCM16/24/32、float32/64。
支持 8-bit A-law（tag 6）和 μ-law（tag 7），所有 256 个代码均有明确映射；它们不是 unsigned PCM8，tag 1 的 PCM8 仍不支持。
没有 native、第三方 Go 模块或外部进程解码路径。

输出为 ITU-T G.711 量化区间中点扩展至 signed16 的代码除以 32768，与既有 PCM 的归一化一致，不改增益或归一化到各 law 自己的最大值。
A-law `0xd5/0x55` 为 ±8，`0xaa/0x2a` 为 ±32256，没有精确零代码；μ-law `0xff` 和 `0x7f` 均输出规范化的正 float64 零，`0x80/0x00` 为 ±32124。
这是真实有损格式解码，不承诺恢复编码前的线性 PCM，也不保留 μ-law 两个零代码的区别。

支持 1..64 声道；一帧恰含每声道一个字节，按文件索引顺序解码为交错 PCM，`ChannelMask=0`。
不推断扬声器标签，不重排、降混或复制声道，包括 mono/stereo。采样率沿用 1..768000 Hz，按 WAVE 声明解释时长，不假设或重采样到电话常用的 8000 Hz。
仅支持直接 tag 6/7 的 WAVEFORMATEX；extensible 压缩 GUID、带 speaker mask 的输入明确报 `ErrUnsupported`。
例如 FFmpeg 的六声道 G.711 writer 会生成 extensible 格式，当前拒绝该真实输出，不剥除其布局信息；无布局声明的 indexed 6/64 声道可正常读取。

`fmt` 必须恰为 18 字节且 `cbSize=0`、`bitsPerSample=8`；legacy 16 字节 fmt、未知扩展和其他位宽明确不支持。
`blockAlign` 必须等于 channels，`avgBytesPerSec` 必须精确等于 `sampleRate*channels`（G.711 没有 IMA 那样的块开销或目标码率例外）。
存在唯一 fmt 和 data，fmt 必须在 data 前；data 字节数必须整除声道数。

非 PCM 的 G.711 必须有唯一、恰为四字节的 `fact`，可在 fmt/data 前后出现；它必须精确等于 `dataBytes/channels`。
空音频也必须有 `fact=0`；任何短/长计数都报 `ErrInvalidInput`，没有未知长度 sentinel、末块填充或 fact 裁尾。
RIFF 的奇数 payload pad 不属于音频样本。新 G.711 路径还验证所有 RIFF pad 值为零，包括 fmt 前的 JUNK/PAD、opt-in metadata 和 data；pad 缺失、越界、非零均报错。
原有 PCM/IEEE/IMA 仍只检查 pad 的存在和边界，不在本切片收紧历史接受策略。

`JUNK/PAD`、metadata opt-in、未知/重复 chunks、RIFF 大小、底层 I/O 错误链和单个对象读取边界均保持既有 WAVE 策略。
只有显式 `DiscardMetadata=true` 才可丢弃既有白名单文本 metadata；该选项不会允许未知 chunks、压缩 GUID、坏 pad 或矛盾 fact。
无论错误在音频前后何处发现都返回 nil PCM，不交付部分样本；成功读取后保留后续流字节。
G.711/WAVE 没有样本校验和，所有字节又都是合法代码，因此某些音频字节损坏无法检测。

Limits 在分配输出/读取 data 前检查精确帧数和标量样本数，MaxBytes 包含 headers、所有 chunks 和 pad，MaxChunks 包含 fact。
每次解码只持有一份输出 float64 数组与 32 KiB scratch，不缓存整条编码流、不做增长复制；返回样本独立，支持独立 reader 上的并发调用。
两种 law 共用初始化一次的 4096 字节只读查表存储；逐样本工作量线性，由编码字节/帧数/样本预算共同界定，不新增 MaxWork。

#### G.711 独立验证（2026-11-04）

测试及 benchmark 在 `tests/_cases/audio/wav_g711_test.go`，二进制输入、signed16 预期、来源及 SHA256 在 `tests/data/audio/g711/`。
`tests/reference/audio_g711.py` 使用 ITU-T G.711（11/1988）区间中点 oracle、FFmpeg8.0.1 与 libsndfile1.2.2，包含两个独立编码器的实际输出。
42 个支持流、207898 个标量样本，Go、规范 oracle、两个外部解码器的**完整样本和完整长度全部精确一致**；没有时长/裁剪参数，也没有对预期误差设置容差。
512 个 law/code 组合逐位覆盖；多声道每个索引都遍历全部代码，另测 1/768000 Hz、空输入、奇数 data/零 pad、超过 32 KiB 的 mono/三声道、fact 在前/后及显式 metadata。

映射上没有发现独立解码器分歧；容器的接受策略有差异。
另 16 个保留拒绝样本包括两个真实六声道 extensible 输出，以及缺失/短/长 fact、16 字节 fmt、不一致 cbSize、非零 pad、extensible GUID；FFmpeg/libsndfile 均宽松接受，本解码器按上述严格边界拒绝。
这些外部完整输出哈希/长度和接受结果也保存在 manifest，未把“外部接受”当成可绕过规范/资源策略的理由。

```sh
python3 tests/reference/audio_g711.py # 开发独立生成；需 FFmpeg、soundfile、numpy
CGO_ENABLED=0 env -u GOMATH_G711_REFERENCE_DIR -u GOMATH_IMA_REFERENCE_DIR \
  -u GOMATH_AIFF_REFERENCE_DIR go run ./tests/cmd/run -pkg=audio -count=1
CGO_ENABLED=1 GOMAXPROCS=8 go run ./tests/cmd/run -pkg=audio -race -count=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio \
  -run='^$' -fuzz='^FuzzWAVG711$' -fuzztime=20s -parallel=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio \
  -run='^$' -bench='^BenchmarkReadWAVG711$' -benchmem -benchtime=300ms -count=5 -p 1
```

默认 fixture 由 go.mod 定位中央目录，保留 `GOMATH_G711_REFERENCE_DIR` 覆盖；普通测试无需外部程序。
完整 audio 回归（含全部旧 PCM/IEEE/IMA/FLAC/AIFF）：95.8% statement coverage，新 G.711 文件 100%；完整 audio race 通过，20s fuzz 797240 次执行通过。
包 vet 与 CGO0/noasm Linux/386 构建通过；依赖审计仍为 89 个标准库包加 audio，无外部模块/cgo/audio assembly/os/exec/plugin。
race 的 CGO1 仅用于 Go race instrumentation，默认测试和解码在 CGO0/noasm 下验证。
旧 PCM/float writer 的 15 个独立 FFmpeg 完整解码输入（135 标量样本、全部五种编码 × mono/stereo/5.1）再次逐字节通过；旧 IMA/AIFF/FLAC fixtures 和 fact 规则均保持。

Go1.27.3 / darwin-arm64 / Apple M2 / macOS15.7.1，CGO0/noasm/GOMAXPROCS1、串行 `-p 1`，五次 300ms 样本：
每次解码一秒 48000 Hz、48000 帧、mono/stereo/六声道索引输入，代码字节为 `(frame+37*channel) mod 256`，固定遍历全部代码。
计时包含完整 RIFF/fmt/fact/pad 校验、输入读取、样本查表及 PCM/32KiB scratch 分配；排除输入构造、磁盘、外部解码与进程启动（包括共享查表初始化）。

| law / 声道 | 中位 ms/op | 五次范围 ms/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| A-law / mono | 0.040093 | 0.039670..0.041949 | 418024 | 14 |
| A-law / stereo | 0.072364 | 0.071400..0.074506 | 803048 | 14 |
| A-law / 6 | 0.192233 | 0.187231..0.196249 | 2343144 | 14 |
| μ-law / mono | 0.039445 | 0.037359..0.056152 | 418024 | 14 |
| μ-law / stereo | 0.072985 | 0.072120..0.073907 | 803048 | 14 |
| μ-law / 6 | 0.191168 | 0.188869..0.198085 | 2343144 | 14 |

原始五次样本、负载、范围和排除项在 `g711/bench.txt` / `benchmark_summary.json`；完整默认回归为 1.241s，完整 audio race 为 9.286s。
数值是共享开发机的限定观察，不代表峰值/RSS，也没有作外部解码器速度比较或宣称普遍加速。

## FLAC writer

```go
err := audio.WriteFLAC(dst, p, audio.FLACWriteOptions{
    Encoding: audio.PCM24, // 零值默认 PCM16；不接受 PCM32 或 float
    BlockSize: 4096,       // 零值默认 4096，可选 16..65535
    // Limits: audio.Limits{MaxBytes: 64 << 20},
    // MaxWork: 1 << 32,
})
```

输出原生 `fLaC` 流，按 RFC 9639 构造唯一的 STREAMINFO metadata block、固定块帧及短末帧。
写出实际总帧数（FLAC 的 interchannel samples）、最小/最大编码帧字节数、完整交错整数 PCM 的 MD5，以及每帧 header CRC8、frame CRC16。
STREAMINFO 的最小/最大 block size 均为配置的固定块长；短于 16 的片段作为合法短末帧写出，不补样本。

每个声道独立搜索固定预测 0..4 阶，按精确位数比较 Rice 参数 0..14、escape 有符号残差与 verbatim。
残差使用 partition order 0，每声道每帧自适应选一个参数；零残差允许零位 escape，常量使用 constant subframe。
只有比 verbatim 更省位时才使用预测编码；不因难压缩而丢样本，也不输出巨大的低参数 unary 序列。
没有 LPC、wasted-bit 搜索、分区搜索或立体声 decorrelation，因此不保证达到参考 FLAC 编码器的压缩率。

PCM→整数规则与 WAV 一致：输入必须 finite 且在 `[-1,1]`；`min(round(v*2^(bits-1)), 2^(bits-1)-1)`，最近整数、半数远离零，限制正端点取整后的整数代码。
`-1` 为最小负整数，`+1` 为最大正整数；没有抖动、增益或超范围削波。
无损指量化后整数的逐样本无损，不保证任意 float64 样本的原精度。
mono/stereo 的 mask 可为 0（约定 mono / left,right），非零必须分别为 4（front-center）或 3（front-left/front-right），其他布局明确报错。
采样率沿用 PCM 的 1..768000 Hz 范围；空片段明确拒绝，因为 STREAMINFO 的 total=0 表示未知长度。
writer 不提供多于两声道、PCM32/float、元数据导入/透传、LPC 或立体声 decorrelation 编码。解码使用下述独立 `ReadFLAC` API。

`Limits` 同时约束输入帧/样本及完整编码字节；`MaxChunks` 对 FLAC 指 metadata block 数，此 writer 固定为一个。
`MaxWork` 零值为 4,294,967,296；预检按 `256*scalarSamples + 128*encodedFrames` 收费，覆盖两次有界预测/参数搜索及输出。
预算单位是保守算法工作量，不是 CPU 指令数或时间保证。总样本还必须可放入 36 位，帧编号必须可放入 31 位。
所有样本、选项、工作量和实际文件长度均在第一笔写入前校验，超限返回 `ErrLimit`，不调低编码质量。
先预检计算精确长度和 MD5，再重复逐块编码，适用于不能 seek 的 writer；辅助内存为 `O(min(BlockSize, Frames)*Channels)`。
块长硬上限 65535，最多两声道；量化缓冲每样本 8 字节，MD5 原始字节缓冲和编码帧缓冲各至多约 `bits/8` 字节/样本，另有少量 header/对齐开销。
输入 PCM 保持不可变，不保留全文件编码或全文件帧计划。I/O 错误保留错误链，短写返回 `io.ErrShortWrite`，可能留下部分文件。
若运行环境禁用 MD5（如 `GODEBUG=fips140=only`），首笔输出前返回带底层原因的 `ErrUnsupported`；不会省略校验或触发 MD5 Sum 的 panic。

## FLAC reader

```go
decoded, err := audio.ReadFLAC(src, audio.FLACReadOptions{
    // DiscardMetadata: true, // 明确允许丢弃支持的 metadata 注释/图片等
    // Limits: audio.Limits{MaxFrames: 48000 * 60, MaxBytes: 64 << 20},
    // MaxWork: 1 << 32,
})
```

API 固定为 `ReadFLAC(io.Reader, FLACReadOptions) (*PCM, error)`；options 为 `Limits Limits`、`MaxWork uint64`、`DiscardMetadata bool`。
只接受 native `fLaC`、1..8 个索引声道、byte-aligned PCM8/16/24/32、1..768000 Hz，完整消费到 EOF，拒绝尾随垃圾、拼接流、Ogg/ID3 包装或中途改变格式。
支持固定/可变块及标准 header 编码、constant/verbatim、fixed 0..4、LPC 1..32、Rice/Rice2 的全部合法分区与 escape、wasted bits，以及 independent/left-side/right-side/mid-side stereo。
负 LPC shift、保留代码、超位宽预测值、超出 FLAC 残差数值范围的 Rice 编码、非法分区或块长、非零对齐位均报错；LPC 用 int64 累加，并逐样本检查有效位宽，再做 stereo 恢复与最终位宽检查。

返回值自行拥有样本，不引用输入或解码缓冲；整数除以 `2^(bits-1)`，在 float64 中精确表示。
输出 `Format.ChannelMask=0`，保持 FLAC 文件约定的声道索引顺序；不推断 speaker mask、不重排/混合声道或施加增益。独立 assignment 0..7 对应 1..8 声道；left-side/right-side/mid-side assignment 8..10 严格只接受双声道。
PCM32 的 side 子帧可为 33 位；预测和 stereo 恢复使用 int64，逐样本限制有效位宽，最终限制原始 PCM 位宽。MD5 使用恢复后的交错有符号整数、每样本恰好 1/2/3/4 个 little-endian 字节。
逐帧校验 CRC8、CRC16、帧/样本序号、固定块一致性及 STREAMINFO 的块/帧边界，最终检查总采样数和完整交错整数 PCM 的 MD5。
总采样数为零时视为未知，按 EOF 实际计数；MD5 全零代表未提供，本严格 API 返回 `ErrUnsupported`，不声称完成无法进行的 MD5 校验。
缺帧、截断、MD5 不符和数值损坏返回错误且不返回部分 PCM。运行环境禁用 MD5 也明确报 `ErrUnsupported`。

首个 metadata 必须是唯一、长度恰为 34 的 STREAMINFO；PADDING 必须全零，可安全跳过。
APPLICATION、SEEKTABLE、VORBIS_COMMENT、PICTURE 默认返回 `ErrUnsupported`，只有显式 `DiscardMetadata=true` 才丢弃；即使允许丢弃，仍校验结构、字段长度、UTF-8、seek point 顺序及单例 metadata 的唯一性。
PICTURE 只校验封装结构与 MIME/描述，授权丢弃后不解析图片像素，也不访问 URI。
CUESHEET、保留 metadata 类型和 `WAVEFORMATEXTENSIBLE_CHANNEL_MASK` 覆盖标签始终明确拒绝，避免丢弃尚未支持的时间轴/声道语义。

`Limits` 同时限制实际和声明的输入字节、metadata 个数、解码帧数/样本数；遇到未知总长度也逐帧执行上限检查。
`MaxWork=0` 默认 4,294,967,296；每编码位、还原样本和预测系数乘加对各计一个工作单位（保守算法预算，不是 CPU 指令数），不足报 `ErrLimit`。
metadata 长度先检查字节与工作预算再分配，单块格式硬上限约 16 MiB；位流输入缓冲 4 KiB，最多八个 65535 项 int64 声道缓冲（4,194,240 bytes），以及一块交错 MD5 字节缓冲（最多 2,097,120 bytes）。
PCM 输出逐块有界增长；增长时旧/新数组可能短暂并存，各自容量受 `MaxSamples`、`MaxFrames` 及存储整数边界限制，不保留整份编码文件。
为区分文件恰好达到 `MaxBytes` 与额外数据，允许最多一字节的 EOF 探测；不会无界预读、搜索下一同步字或解码外部 fallback。
输入必须是最终会 EOF 的完整文件流；本 API 不提供直播、seek、流式 PCM 回调或取消接口。

第五批输入扩展的完整边界、声道顺序、独立验证与匹配基准见 [FLAC_INPUT.md](../docs/FLAC_INPUT.md)。
194 个真实 libFLAC 流覆盖四种位深 × 八种声道数，全部 Go/libFLAC/FFmpeg 完整整数比较通过；libsndfile 1.2.2 通过 144 个 8/16/24-bit 流，明确不支持其余 50 个 32-bit 流，未降位深替代。
另外 98 个手工位流由 libFLAC 与 FFmpeg 完整解码确认，覆盖 33-bit side、LPC32、Rice2/escape 极值及显式/继承位深 header。数据和工具位于 `tests/data/audio/flac_input/` 与 `tests/reference/audio_flac_input.py`，普通测试无需外部工具。
`WriteFLAC` 的 PCM16/24 mono/stereo 范围和既有码流保持不变，CLI 的 mono/stereo remix 范围也没有随 reader 扩展。

## 重采样策略和资源上界

默认使用 `Radius=32`、`Rolloff=0.94` 的 Blackman 窗 sinc。
归一化截止频率为 `min(1, outputRate/inputRate)*Rolloff`；降采样时按截止频率倒数扩大核支持范围。
`Radius` 可选 4..256，`Rolloff` 可选 `(0,1]`；零值使用默认。
较小的 rolloff 留出过渡带，也会削弱靠近目标奈奎斯特频率的内容；较大的半径增加滤波工作量。
输入输出同采样率时精确复制，不滤波。

输出帧数为 `ceil(inputFrames*outputRate/inputRate)`，第 j 帧对应 `j/outputRate` 秒，起点对齐，无额外滤波延迟。
每个分数相位独立归一化 DC 增益，时间用整数商余数计算以避免累计相位漂移。
有限长度带限插值会发生振铃和边缘瞬态，不保证样本峰值保持在 ±1 内。

边缘策略可选：

- `EdgeReflect`（默认）：半采样点对称；`-1→0`，`N→N-1`，反复反射以处理极短片段。
- `EdgeHold`：重复端点样本。
- `EdgeZero`：缺失样本为零，归一化仍包含缺失位置的权重，因此边界有零填充瞬态。

`Limits` 每个零字段选择默认，负数报错：

| 字段 | 默认上限 |
| --- | --- |
| `MaxFrames` | 16,777,216 帧 |
| `MaxSamples` | 16,777,216 个 float64 样本，即单片段样本存储 128 MiB |
| `MaxBytes` | 编码总长度 256 MiB，含 header/pad/metadata |
| `MaxChunks` | 4096 个 RIFF/AIFF local chunks / FLAC metadata blocks |

帧数和样本数上限同时生效。构造、WAV、FLAC 和混音可传自定义 `Limits`。
裁剪、增益和包络只分配不大于原片段的新样本数组；原片段仍由调用者持有。
WAV I/O 额外使用 32 KiB 样本编码缓冲；不会整体加载编码后的文件。

`ResampleOptions.Limits` 同时限制输入及输出。
`MaxTaps` 默认为 8193，可显式配置但最多 65537；额外核存储为每抽头 8 字节。
`MaxWork` 默认为 536,870,912，保守按 `outputFrames*taps*(channels+1)` 计算滤波工作量；同率复制按样本数计。
支持范围、工作量和输出存储均在分配输出前检查；超限报 `ErrLimit`，不会缩短核或退回线性插值。
提高上限会相应增加内存/CPU 成本；重采样为离线整片段处理，未提供流式状态或取消接口。

调用者可用 `errors.Is` 区分 `ErrInvalidInput`、`ErrUnsupported`、`ErrLimit`；底层 I/O 错误保留包装链。

## 验证

测试源仅在 `tests/_cases/audio/`，二进制参考输入在 `tests/data/audio/`。
使用仓库 overlay runner；不要在 audio 生产包目录放置 `_test.go`：

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -race -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzReadWAV$' -fuzztime=5s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzResampleFiniteAndBounded$' -fuzztime=5s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzFLACIntegerRoundTrip$' -fuzztime=10s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzReadFLAC$' -fuzztime=20s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -bench='Benchmark(Resample48kStereo|WAV48kStereo|Mix48kStereo)$' -benchmem -benchtime=300ms -count=3 -p=1
CGO_ENABLED=0 go vet -tags noasm ./audio
python3 tests/data/audio/verify_flac.py # 开发独立验证；需 FFmpeg/ffprobe
python3 tests/data/audio/verify_flac_read.py # 开发独立验证；需 libFLAC flac 与 FFmpeg
```

runner 自动添加 `-tags noasm`。测试包含独立 FFmpeg fixtures、整数全量程/float 精度、RIFF pad/顺序/截断/资源上限、多声道隔离、编辑独立存储、并发一致性及解析/重采样 fuzz。
正弦比较使用 1000 Hz、0.5 幅度，六组输入/输出采样率；滤波器默认质量下，已测内部 RMS 误差约 `9.4e-8..7.9e-7`。
五组降采样超奈奎斯特输入的已测内部衰减约 `87..127 dB`；排除首尾各 256 输出帧，不代表整个过渡带、所有频率或边缘的统一保证。

独立解码验证可设置 `GOMATH_AUDIO_REFERENCE_OUT=<临时目录>` 后运行 `-run='^TestWAVIndependentDecoderArtifacts$'`。
它仅在开发测试中输出 15 个 WAVE 和期望样本 manifest，由外部解码器另行验证；普通测试不需要 FFmpeg，也不会启动它。
已用 FFmpeg 8.0.1 完整解码五种编码的单声道/立体声/5.1 输出，核对精确样本、完整长度、采样率和 5.1 声道布局。

FLAC 普通测试用独立的测试位流读取器检查 MD5、两种 CRC、总样本、帧长及逐样本还原，并校验六个已由 FFmpeg 独立完整解码确认的小型 fixture。
覆盖 0..4 阶固定预测、Rice/escape/verbatim、极短及短末帧、最大块长、帧编号 128/2048 转换、量化端点/半数、多声道隔离、资源和 I/O 错误及并发确定性。
测试入口将 cwd 定位到 `tests/data/audio`，包括 fuzz worker；失败 corpus 只会进入其 `testdata/fuzz` 子目录。
FLAC fuzz 首轮 10 秒执行 46,953 次，后续 30 秒执行 107,053 次，均通过；audio 完整 race 与包内 vet 通过。
另已用 `GODEBUG=fips140=only CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^TestFLACMD5Availability$' -count=1 -v` 验证 MD5 不可用时的有界报错。

开发脚本 `tests/data/audio/verify_flac.py` 使用 focused runner 生成样本，然后由 FFmpeg 完整解码为 s16le/s24le，比较全部字节、长度、采样率、声道、位深及 MD5。
2026-11-04 使用 FFmpeg 8.0.1 完成 130 个流、2,077,648 个标量样本的精确比较；含 1..65,536 帧、1..768000 Hz、多种块长、所有固定预测阶数、constant/Rice/escape/verbatim、随机/边界/舍入输入。
详细文件哈希记录在 `tests/data/audio/flac/verification.json`；加 `--keep /绝对临时目录` 可保留全部输出重新检查。
FFmpeg、ffprobe 和 Python 都仅用于此开发验证；audio 运行时和普通测试没有这些依赖。

FLAC 解码还以 libFLAC 1.5.0 直接从独立整数 PCM 编码，Go 与 FFmpeg 8.0.1 分别完整解码比较全部样本。
2026-11-04 已验证 132 个流、1,220,112 个标量样本精确相等，包含 PCM16/24、mono/stereo、1..10001 帧、level 0/5/8、wasted bits、Rice/Rice2、不同分区、fixed/LPC 与全部四种声道 assignment。
libFLAC 实际选择的 LPC 阶数覆盖 3..12；Go 手工位流测试另覆盖 LPC 1/2/32、escape、所有块长/不常见采样率 header、可变块、数值溢出和恶意 unary 工作量。
八个独立 fixture 及完整哈希/分析记录位于 `tests/data/audio/flac_decode/`，普通测试仅用 Go 读取它们。
`verify_flac_read.py --keep /绝对临时目录` 可保留完整矩阵；`--retain-fixtures` 在全部验证通过后更新已选 fixture。
解码恶意输入 fuzz 首轮 20 秒执行 1,390,275 次通过；完整 audio race 与 vet 通过。

2026-11-04 串行 benchmark 记录：Go 1.27.3，darwin/arm64，Apple M2，`CGO_ENABLED=0`、`noasm`；`-p=1`，各取三次 300 ms 样本。
负载为一秒 48000 Hz 立体声（48,000 帧），左声道 0.5 幅度 1000 Hz 正弦，右声道静音；重采样使用默认质量。
排除输入生成、磁盘读写、独立解码和程序启动；包含结果分配，WAV 解码含 PCM 分配，编码输出到 `io.Discard`，混音在 0.5 秒开始叠加同一输入并输出 1.5 秒。
没有视频渲染、图像尺寸或比较其他实现的加速比。

| 操作 | 三次 ms/op | B/op（约） | allocs/op |
| --- | --- | --- | --- |
| 48000→16000 重采样 | 14.841 / 12.828 / 12.455 | 264,000 | 3 |
| 48000→44100 重采样 | 66.287 / 65.906 / 73.253 | 713,392 | 3 |
| PCM24 解码 | 0.2394 / 0.2393 / 0.2375 | 803,038 | 11 |
| PCM24 编码 | 0.4911 / 0.5105 / 0.5243 | 32,848 | 2 |
| 偏移混音 | 0.1378 / 0.1435 / 0.1687 | 1,155,125 | 2 |

这些是共享开发机上的观察值，不作隔离环境下的性能保证。

FLAC 在同一天、同一 Go/硬件/纯 Go 配置下串行测量：

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -bench='^BenchmarkFLAC48kStereo$' -benchmem -benchtime=300ms -count=3 -p=1
```

每次编码一秒 48000 Hz 立体声，块长 4096，分别为全零、0.8 幅度正弦（左 991 Hz、右 1098 Hz）和固定 seed 满量程整数随机信号。
输出到 `io.Discard`；包含量化、MD5、完整预检、二次搜索、编码与 CRC，排除输入生成、启动、磁盘和独立解码。
没有图像尺寸或视频帧；编码目标的原始 PCM 为 192000/288000 字节。

| 编码/信号 | 三次 ms/op | B/op（约） | allocs/op |
| --- | --- | --- | --- |
| PCM16 / silence | 0.7302 / 0.7256 / 0.7271 | 100800 | 29 |
| PCM16 / sine | 12.061 / 11.606 / 11.676 | 100800 | 29 |
| PCM16 / random | 14.068 / 13.870 / 13.805 | 100800 | 29 |
| PCM24 / silence | 0.8618 / 0.8543 / 0.8663 | 117824 | 29 |
| PCM24 / sine | 13.288 / 13.203 / 13.246 | 117824 | 29 |
| PCM24 / random | 16.551 / 16.561 / 16.463 | 117824 | 29 |

ReadFLAC 同一天在上述 Go 1.27.3 / Apple M2 / `CGO_ENABLED=0`、`noasm` 配置下，以 `-p=1 -benchtime=300ms -count=3` 串行测量。
输入为一秒 48000 Hz 立体声正弦，左 991 Hz、右 1098 Hz、0.8 幅度，预先由 writer 编码成默认 4096 块长的 PCM16/24 FLAC。
计时包含完整解析、CRC/MD5、输出分配与样本还原；排除输入生成、编码、程序启动、磁盘和外部解码。

| 解码 | 三次 ms/op | B/op（约） | allocs/op |
| --- | --- | --- | --- |
| PCM16 / sine | 5.3728 / 5.3320 / 5.2898 | 2879800 | 19 |
| PCM24 / sine | 7.8819 / 7.8451 / 7.8143 | 2887993 | 19 |

### Remix / Concat / Repeat 验证与成本

普通 Go 测试读取独立 Python `fractions.Fraction` 精确有理数 oracle，逐样本核对三个矩阵的 40 个输出值；
包含逐声道 impulse、显式 6→2 权重、反相、复制、静音行和超 ±1 headroom。
其余测试覆盖格式/mask、64×64 维度、精确工作/存储边界、空输入、MaxInt 重复、整数/float 溢出、独立存储、负零逐位复制及并发。
新功能完成后，完整 audio 测试覆盖率 94.4%，完整 audio race、vet 和 linux/386 纯 Go 编译通过。
测试与基准源文件均位于 `tests/_cases/audio/`，oracle 及其开发生成脚本位于 `tests/data/audio/`。

2026-11-04，同一 Go 1.27.3 / darwin/arm64 / Apple M2 / `CGO_ENABLED=0` / `noasm` 配置：

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio -run='^$' -bench='^(BenchmarkRemix48k|BenchmarkSequence48kStereo)$' -benchmem -benchtime=300ms -count=3 -p=1
```

Remix 输入为一秒 48000 Hz（48,000 帧），第一声道 0.5 幅度 1 kHz 正弦，其余静音。
2→1 使用 `[.5,.5]`，6→2 使用 `[[1,0,.5,.25,.5,0],[0,1,.5,.25,0,.5]]`；均计算全部系数，包括零。
默认工作预算分别扣除 96,002 / 576,012 单位。拼接/重复使用同一秒立体声输入的四次连续副本，输出 192,000 帧、384,000 个标量样本（3,072,000 字节）。
计时包含校验、结果分配与全部运算/复制；排除输入构造、程序启动、磁盘、编解码与 oracle 生成。没有视频或图像尺寸。

| 操作 | 三次 ms/op | B/op（约） | allocs/op |
| --- | --- | --- | --- |
| Remix 2→1 | 0.183573 / 0.171790 / 0.185741 | 385074 | 2 |
| Remix 6→2 | 0.458147 / 0.460947 / 0.468947 | 770096 | 2 |
| Concat 四个片段 | 0.151174 / 0.134030 / 0.132460 | 3072054 | 2 |
| Repeat 四次 | 0.146856 / 0.176818 / 0.194321 | 3072057 | 2 |

观察值来自共享开发机；矩阵成本随 `frames × inputChannels × outputChannels` 增长，序列复制随输出样本数增长。
所有既有 WAV/FLAC fixture 保留，并通过完整 audio 回归测试。

## AIFF / AIFC PCM 解码

```go
p, err := audio.ReadAIFF(src, audio.AIFFReadOptions{
    // DiscardMetadata: true, // 仅明确允许丢弃受支持的 ASCII 文本注释
    // Limits: audio.Limits{MaxFrames: 48000 * 60, MaxBytes: 64 << 20},
    // MaxWork: 1 << 32,
})
```

固定接口为 `ReadAIFF(io.Reader, AIFFReadOptions) (*PCM, error)`；options 包含
`Limits Limits`、`MaxWork uint64`、`DiscardMetadata bool`。本切片仅提供解码。

| FORM / compressionType | 支持的样本 |
| --- | --- |
| AIFF | 大端 signed PCM8 / PCM16 / PCM24 / PCM32 |
| AIFC / `NONE` | 大端 signed PCM8 / PCM16 / PCM24 / PCM32 |
| AIFC / `sowt` | 小端 signed PCM16；其他位宽明确报错 |

支持 1..64 个交错声道，严格保持文件索引顺序，输出 `ChannelMask=0`；不推断、重排或丢弃扬声器标签。
样本使用有符号二补码除以 `2^(depth-1)`，与现有整数 WAV/FLAC 读取语义一致。
例如 PCM8 的合法最小/最大值分别映射为 `-1` 和 `127/128`，PCM32 最大值为 `2147483647/2147483648`，不映射成 `+1`。
返回的不可变 PCM 独立持有样本，`Samples()` 仍返回副本。

80-bit SANE/IEEE 扩展采样率直接检查 exponent 和全部 64 个显式 significand bits，不先转换为 float64。
仅接受精确整数 1..768000 Hz；即使 float64 会舍入为整数的低位小数也返回 `ErrUnsupported`。
负数、零、NaN/Infinity、unnormal/pseudo-denormal 编码返回 `ErrInvalidInput`；范围外正数与非整数明确不支持。
COMM 声明帧数可以为零，此时 SSND 可缺省；否则必须有唯一 COMM 与唯一 SSND。
AIFC 还必须有唯一、长度为 4、时间戳恰为 `0xA2805140` 的 FVER，未知版本不按日期范围猜测兼容。
COMM 中 Pascal compressionName 的长度、内嵌零 pad 与准确 COMM 大小均验证；按 compressionType 选择解码，不依赖可本地化的显示名称。

读取恰好一个 FORM 对象，成功后保留输入后续字节；不要求 EOF，也不读取下一对象作为探测。
FORM/chunk 长度按规范的非负 signed 32-bit 字段检查；所有 chunk、payload、奇数字节零 pad 均须在 FORM 边界内。
COMM/SSND/FVER 可任意排序。SSND 的 offset 必须位于 payload 内；跳过 offset 后按 COMM 帧数读取连续样本。
SSND `blockSize` 描述外部存储对齐，不表示帧间有空隙；帧后未用存储按规范完整消费，不当作额外音频。
声道帧所需字节不足、重复必需块、缺块或非法零 pad 均报错，不补样本或返回部分 PCM。
AIFF 没有本解码器可验证的内容校验和；合法样本字节被修改不一定能被容器结构检查发现。

默认遇到 `NAME`、`AUTH`、`(c) `、`ANNO` 返回 `ErrUnsupported`。
只有显式 `DiscardMetadata=true` 才可丢弃这四类 ASCII 文本；仍检查完整读取、零 pad、单例 NAME/AUTH/版权唯一性，ANNO 可以重复。
非 ASCII 文本也明确报错。Markers、loops、COMT、APPL、MIDI、AESD、CHAN、ID3、未知或嵌套 chunks 始终拒绝，包括允许丢弃文本时。
尤其 FFmpeg 生成的部分多声道 AIFF 带 CHAN，当前会拒绝这些布局语义；没有 CHAN 的多声道索引样本可正常读取。
浮点、μ-law/A-law、IMA4、`twos`、`raw `、压缩音频和非整字节位深均不支持，没有外部解码 fallback。

`Limits` 复用现有默认值，`MaxChunks` 对 AIFF 包括 COMM/SSND/FVER 和全部文本块。
`MaxWork=0` 为 `2^32`，精确预检收费为 **FORM 总字节数（含 8 字节 FORM header、所有 pad/未用存储）+ 标量样本数**。
超字节/工作预算在读取 FORM header 后、处理 chunk 前报 `ErrLimit`；帧数/样本数与样本工作预算在 COMM 后、分配输出前检查。
负 Limits 返回 `ErrInvalidInput`，I/O 错误保留包装链，任何失败均返回 nil PCM。

COMM 在前时不保留编码音频，样本读取通过 32 KiB scratch，未用存储/文本通过 4 KiB scratch。
SSND 在前时暂存其 offset 后 payload，先成功读取小块再增长缓冲，避免巨额虚报长度立即触发大分配。
每个暂存数组的容量不超过该 SSND payload（受 `MaxBytes` 约束），增长时旧/新数组可短暂并存，之后还可能与独立 PCM 输出并存。
`MaxSamples` 约束输出 float64 存储，不包含这种暂存或调用者输入；需要更低暂存内存时应降低 `MaxBytes` 或使用 COMM-first 输入。

### AIFF 独立验证与成本（2026-11-04）

普通测试与 fuzz 源在 `tests/_cases/audio/aiff_read_test.go`，fixtures 与日志在 `tests/data/audio/aiff/`，开发工具为 `tests/reference/audio_aiff.py`。
该脚本用独立 FFmpeg 8.0.1、CPython 3.11.5 标准库 `aifc` 从显式整数生成输入，不调用 gomath 编码器生成期望数据。
Apple AIFF 1.3 与 AIFF-C 规范的来源 URL、SHA256、章节，以及工具版本、每个输入/解码 SHA256 均记录在 `verification.json`。
当前矩阵 77 个输入、493662 个原始标量样本；保留 62 个小 fixture，较大的跨 32 KiB 边界输入验证后删除。
普通测试从 `go.mod` 定位仓库根目录后读取 `tests/data/audio/aiff/manifest.json`，不依赖包 cwd 或 TestMain 是否切换目录；`GOMATH_AIFF_REFERENCE_DIR` 仍可覆盖位置。
15 个真实 FFmpeg CHAN 输入按 API 策略在两种 DiscardMetadata 设置下均拒绝；其余 62 个流、165432 个标量样本由纯 Go 精确比对。
其中 61 个支持流的 FFmpeg 严格完整解码逐字节等于预期；另 15 个 CHAN 输入也由 FFmpeg 完整精确解码，但 Go 明确不接受其布局。

单列的 `spec_COMM_bounds_unused_SSND.aiff` 含合法帧后未用零字节：FFmpeg 完整解码会额外输出一个 PCM8 样本（s32le 共多 4 字节）。
这里**没有用时长/裁剪参数掩盖差异**；记录其完整输出 hash/额外字节，用独立 CPython reader 自身解析出的 COMM 帧数读取并核对规范音频，Go 得到相同的精确样本与帧数。
因此不声称所有 77 个流都与 FFmpeg 原始输出长度相同。SSND-before-COMM、非零 offset、文本 opt-in 和极限整数采样率另有独立样本覆盖。

```sh
python3 tests/reference/audio_aiff.py  # 开发环境需 Python3.11/3.12 aifc 与 FFmpeg
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=audio \
  -run='^TestReadAIFFIndependentFixtures$' -v -count=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio -count=1 -cover -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio \
  -run='^$' -fuzz='^FuzzReadAIFF$' -fuzztime=20s -parallel=1 -p 1
CGO_ENABLED=1 GOMAXPROCS=8 go run ./tests/cmd/run -pkg=audio \
  -run='^TestReadAIFF' -race -count=1 -p 1
```

确定性测试覆盖全部 768000 个可接受整数采样率、低位小数/特殊 80-bit 编码、signed extrema、1..64 声道边界、双向 chunk 顺序、所有 Pascal 长度、非零 offset/blockSize/未用尾部、空输入、逐字节截断、底层 I/O 包装链和各项预算精确边界。
20s fuzz 无错误，787101 次执行；AIFF 专项 race 1.484s 通过。完整 audio 回归包含所有既有 WAV/FLAC 测试，95.1% statement coverage，AIFF 新源码 99.5%。
默认 fixture 根定位修复后，明确清除 `GOMATH_AIFF_REFERENCE_DIR` 的完整中央 runner 命令再次通过（1.013s），显式 env 覆盖也通过。
CGO0/noasm 依赖检查为 89 个标准库包加 audio，无外部模块、cgo、audio native/assembly 或 os/exec/plugin；linux/386 纯 Go 构建通过。
race 的 CGO1 仅用于 Go race instrumentation；普通解码和全部 fixture 检查用 CGO0/noasm，不需要外部工具。

Go1.27.3、darwin/arm64、Apple M2，CGO0/noasm/GOMAXPROCS1，`-p 1 -benchtime=300ms -count=5`：
每次解码一秒 48000 Hz 立体声、96000 个标量样本，覆盖各位宽的精确极值和固定整数序列；输入均预先构造。
计时包含 FORM/chunk 校验、完整样本读取/转换、输出分配和 SSND-first 暂存，排除输入构造、程序启动、磁盘、独立编码/解码；无图像或视频帧。

| 深度 | COMM-first 中位 ms/op | SSND-first 中位 ms/op | COMM-first B/op / allocs | SSND-first B/op / allocs |
| --- | ---: | ---: | ---: | ---: |
| PCM8 | 0.1397 | 0.1563 | 811472 / 14 | 1040893 / 22 |
| PCM16 | 0.1795 | 0.2275 | 811472 / 14 | 1270271 / 23 |
| PCM24 | 0.1806 | 0.2305 | 811472 / 14 | 1630720 / 24 |
| PCM32 | 0.1818 | 0.2358 | 811472 / 14 | 1720831 / 24 |

五次原始数据、范围和汇总在 `bench.txt`、`benchmark_summary.json` / `.md`。
B/op 是分配总量，不是峰值或驻留内存；SSND-first 因暂存而更慢、分配更多。
以上为共享机器固定场景观察，不主张普遍优于其他解码器。

```sh
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio \
  -run='^$' -bench='^BenchmarkReadAIFF$' -benchmem -benchtime=300ms -count=5 -p 1
```

## Microsoft ADPCM WAV 解码（tag 2）

`ReadWAV(io.Reader, WAVReadOptions) (*PCM, error)` 自动识别直接格式标记 `2`，无需新 API 或 option；本切片只解码，不新增编码器。
原 PCM/IEEE、IMA、G.711、FLAC 和 AIFF 的 API、默认策略与 fixtures 保留。

支持有界的 4-bit mono/stereo；保持交错声道索引，`ChannelMask=0`，不推断扬声器布局。
PCM 输出为 signed16 除以 32768，合法极值分别是 `-1`、`32767/32768`。
块头按 predictor、delta、sample1、sample2 的分声道数组读取；先播放 sample2 再播放 sample1。
mono 每字节先高 nibble 后低 nibble；stereo 高 nibble 为声道 0、低 nibble 为声道 1。每块独立重置状态。
预测按 Microsoft 1994 规范向零除以 256，再加 signed nibble × delta，最后饱和到 signed16；不会用负数右移代替向零舍入。

明确范围及边界：

- `fmt` 必须恰为 `22+4*numCoef` 字节，`cbSize=4+4*numCoef`；系数表为 7..256 对 signed16，前七对必须是规范标准表。
  额外系数确实用于预测，支持索引 7..255；没有忽略系数或偷换标准表。未知扩展、压缩 GUID、非 4-bit 或多于两个声道明确拒绝。
- 采样率为整数 1..768000 Hz。`blockAlign>=7*channels`，`samplesPerBlock=2+(blockAlign-7*channels)*2/channels`，且必须可由 uint16 精确声明。
  mono `blockAlign` 最大 32773（65534 帧），stereo 最大 65535（65523 帧）。允许只有块头的两帧块和奇数块长。
  压缩 byte rate 按既有 IMA 策略作为 advisory 值，不用它推断时长；除计算所得 floor rate 为零外，字段必须非零。
- 初始 delta 支持字面值 0..32767；0..15 在首 nibble 后才按规范进入最小 16 的自适应步长。
  delta 高位为 1 的块头明确返回 `ErrUnsupported`，不猜测 signed/unsigned 解释。
- 预测、残差与 adaptation 使用 checked int64 范围；每个 nibble 后要求 delta 不超过 `floor(MaxInt64/768)`。
  这保证后续乘加安全。超过该边界返回 `ErrLimit`，不回绕、不静默封顶；即使发生在 fact 裁去的填充或块末也会报错。
- data 只接受完整物理块；短末块返回 `ErrUnsupported`，不补数据。必须有唯一、恰为 4 字节的 fact，每声道计帧。
  空 data 只接受 fact=0；非空 data 要求 `capacity-samplesPerBlock < fact <= capacity`，只允许裁去最后一块的尾部，不能隐藏整块。
  所有填充 nibble 和全部块头仍须成功解码/验证。fact 可在 fmt/data 前后，fmt 必须先于 data。
- 复用 `Limits` 的 RIFF 总字节/块数及解码帧数/样本数预算；**按全块 capacity 计费和分配**，fact 不降低资源预算。
  额外存储只有最多 1024 字节系数数据和一个 `blockAlign` 字节块缓冲；输出 float64 数组在 fact 裁剪后仍保留完整 capacity。
- RIFF 奇数 payload 必须有零 pad（含 fmt 前的 JUNK）；仅此新格式采用该严格策略，旧 PCM/IEEE/IMA 的既有 pad 接受行为保留。
  metadata 复用 `WAVReadOptions.DiscardMetadata` 白名单；默认拒绝注释，未知块、loops、playlists 等即使 opt-in 也拒绝。
  只消费一个 RIFF 对象并保留后续流字节；所有失败返回 nil PCM，底层 I/O 错误保留包装链。

### Microsoft ADPCM 独立验证与成本

工具为 `tests/reference/audio_ms_adpcm.py`；生产和普通 Go 测试不会启动它或任何外部进程。
`tests/data/audio/ms_adpcm/manifest.json` 记录规范/独立解码器来源 URL 与 SHA256、FFmpeg 8.0.1 / libsndfile 1.2.2 版本、每个输入及期望 PCM 的 hash、**完整**外部输出 hash/长度/差异数量。
开发工具的规范 oracle 独立使用 Python 整数运算，不调用 Go；首次抓取来源，之后复用 manifest 中已抓取的不可变来源 hash。
普通 fixtures 默认从 go.mod 定位到 `tests/data/audio/ms_adpcm`；可显式用 `GOMATH_MS_ADPCM_REFERENCE_DIR` 覆盖，不依赖包 cwd。

50 个支持流、312468 个 fact 内标量样本由纯 Go 全量逐样本比对，包含六个真实 FFmpeg/libsndfile 编码流、全部七个标准 predictor、全部 nibble、零/极值/饱和、低 delta、奇数块长、header-only、末块裁剪、额外系数索引 7/255 及超过 signed32 的预测中间值。
FFmpeg 严格模式完整解码接受其中 46 个，全部 **全块 capacity** 输出逐字节等于规范 oracle；其中 31 个也恰等于 fact 长度，另 15 个保留了末块填充。
FFmpeg 对四个选用额外系数的流拒绝；这四个由独立规范 oracle 验证，不声称 FFmpeg 支持它们。

libsndfile 接受 43 个流，其中仅 3 个完整输出与 fact 内规范 PCM 同 hash；这并不表示另外 40 个都在有效样本上不同。
六个真实编码器流的有效样本前缀均相同，五个的独立输出还含填充。
分离的负数预测 fixture 证实 libsndfile 右移舍入比规范向零舍入低一个 PCM unit；高 adaptation 的构造流还触发其 signed16 delta 存储回绕。
标准系数流上的差异可由独立的「floor 预测 + signed16 delta 存储」oracle 完整复现，记录在 `Signed16DeltaFloorComparison`；没有为匹配外部实现改变规范运算。
libsndfile 也拒绝部分短块，或不能正确使用额外系数。完整输出、拒绝信息和未裁剪的差异均保留；不使用 duration/trim 参数遮掩。
另有三个拒绝输入保存独立行为，覆盖高位 delta、短末块及表外 predictor。旧 PCM/float 的 15 个 mono/stereo/5.1 流再次通过 FFmpeg 严格完整解码，记录在 `_pcm_float_regression/verification.json`。

复验（所有测试/benchmark 源在 `tests/_cases/audio/wav_ms_adpcm_test.go`）：

```sh
python3 tests/reference/audio_ms_adpcm.py # 开发专用，需 FFmpeg、numpy/soundfile
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio -count=1 -cover -p 1
CGO_ENABLED=1 GOMAXPROCS=8 go run ./tests/cmd/run -pkg=audio -race -count=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzWAVMS$' -fuzztime=20s -parallel=1 -p 1
CGO_ENABLED=0 go vet -tags noasm ./audio
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./audio
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio -run='^$' -bench='^BenchmarkReadWAVMS$' -benchmem -benchtime=300ms -count=5 -p 1
```

runner 自动加 noasm。race 的 CGO1 仅用于 Go 检测工具链；普通运行和回归均 CGO0。
确定性测试另覆盖 fact 所有末块位置、每字节截断/I/O 错误、无部分结果、精确/超一预算、超大 capacity 分配前拒绝、metadata/pad/重复块/帧界、全部系数及格式边界、并发独立存储和自适应算术越界。
20 秒 fuzz 925796 次执行通过；完整 audio race 最终复验 18.400 秒通过；CGO0/noasm 完整 audio 2.153 秒，整体覆盖率 96.1%，新解码文件覆盖率 100%。生产包及中央 audio 测试均已交叉编译为静态 linux/386（当前 ARM Mac 未执行该二进制，编译证明在 `linux386_tests.json`）。依赖审计为 89 个标准库包加 audio，无第三方模块、cgo、os/exec、plugin 或 audio 汇编。

2026-11-04，Go 1.27.3 / darwin-arm64 / Apple M2，CGO0/noasm/GOMAXPROCS1，串行 `-p 1 -benchtime=300ms -count=5`：
每次读取一秒 48000 Hz、fact=48000 帧的独立 FFmpeg 正弦 fixture，所有块的填充仍执行完整解码。
包含 RIFF/fmt/fact/块校验、样本重建和结果分配；排除 fixture 磁盘读取、输入生成/编码、外部解码、启动。没有图像/视频负载。

| 声道 / blockAlign | 五次中位 ms/op | 范围 ms/op | 中位 B/op | allocs/op |
| --- | ---: | --- | ---: | ---: |
| mono / 256 | 0.357422 | 0.353878..0.363061 | 385577 | 20 |
| mono / 1024 | 0.360970 | 0.355544..0.362857 | 394538 | 20 |
| stereo / 256 | 0.385224 | 0.383483..0.389497 | 770622 | 20 |
| stereo / 1024 | 0.388332 | 0.387504..0.389327 | 779582 | 20 |

原始样本见 `bench.txt`，环境/负载及汇总见 `benchmark_summary.json`。
B/op 为分配总量，不是峰值内存；编码器选择的 padding capacity 不同，较大块并不保证更快或更省内存。
这是共享开发机的固定负载观察，不宣称优于 FFmpeg/libsndfile 或其他编码格式。

## Sun / NeXT AU 解码

```go
p, err := audio.ReadAU(src, audio.AUReadOptions{
    // Limits: audio.Limits{MaxFrames: 48000 * 60, MaxBytes: 64 << 20},
    // DiscardMetadata: true,
    // AllowUnknownSize: true, // 显式允许 dataSize=0xffffffff；必须读到 EOF
})
```

固定 API 为 `ReadAU(io.Reader, AUReadOptions) (*PCM, error)`；options 只有
`Limits Limits`、`DiscardMetadata bool`、`AllowUnknownSize bool`。本切片只提供解码，不新增 AU 编码器。

| AU encoding | 样本 |
| --- | --- |
| 1 | G.711 μ-law，8-bit code |
| 2 | signed PCM8 |
| 3 | 大端 signed PCM16 |
| 4 | 大端 signed PCM24 |
| 5 | 大端 signed PCM32 |
| 27 | G.711 A-law，8-bit code |

只接受六个大端 uint32 字段组成的 24 字节 `.snd` header。
采样率为精确整数 1..768000 Hz，声道数 1..64；零值非法，超出支持范围明确报错。
按文件交错声道索引保持顺序，输出 `ChannelMask=0`；不隐式重排、remix 或推断多声道扬声器分配。
PCM8 是 signed，区别于 unsigned WAV PCM8；PCM 按 `signed/2^(bits-1)` 归一化，最小值为 `-1`，正最大值严格小于 `+1`。
G.711 复用已验证的完整 256-code 重建表及 signed16/32768 归一化，包含两个 μ-law 零码的正 float64 零。
浮点(6/7)、fragmented、DSP、fixed/emphasis、G.72x ADPCM、其他编码和 `dns.` 小端/host-endian 变体始终返回 `ErrUnsupported`，没有外部 fallback。

### Offset / annotation / 数据边界

`dataOffset` 是相对 AU magic 的字节偏移；支持 24（无 info），或至少 28（info 至少 4 字节）。25..27 属于本实现明确不支持的短 info 字段。
不另加四字节/八字节对齐要求，因此 offset=29、31、4125 等可正常读取。
annotation 恰好是 `[24,dataOffset)`，始终按 offset 定位数据，不扫描 NUL 猜测终点。
默认只接受不存在或全零 annotation。任何非零字节返回 `ErrUnsupported`；`DiscardMetadata=true` 才允许明确丢弃任意文本/二进制 info，包括没有 NUL 终止或含非 ASCII 字节的旧文件。
AU info 本身允许文本或数据；本 API 不解释其私有语义，也不把 annotation 内嵌的 header 当作音频。

已知 dataSize：

- 必须是完整的 `channels × bytesPerSample` 帧，零长度合法。
- 在读取 annotation、分配样本前检查 offset+size 的字节预算以及帧/样本/native-int 存储预算。
- 恰好读取声明的样本；缺 header、annotation 或数据返回截断错误，无部分 PCM。
- 不要求 EOF，不消费 AU 后面的对象、尾字节或所谓 pad。需要单文件 EOF 的 CLI/调用方继续单独检查尾部；不能把尾部当成合法样本或容器 pad。

`dataSize=0xffffffff`：

- 默认返回 `ErrUnsupported`；必须显式 `AllowUnknownSize=true`。
- 从 offset 读到干净 EOF；每个完整帧都是真实音频，没有内建下一个对象的边界。若需要外层分帧，调用者须提供独立定界的 reader。
- EOF 必须落在完整声道帧之后，半样本/半帧返回 `ErrInvalidInput` 和包装的 `io.ErrUnexpectedEOF`；非 EOF 底层错误即使同时带回完整数据也不当作成功。
- 每次输出增长前由帧、样本、可用编码字节及 native-int 存储上限共同限制容量；达到边界时额外探测 **一个字节**，区分恰好 EOF 与数据超限。超限报 `ErrLimit`，该探测字节在失败时已消费，不返回截短结果。
- 不依赖 `io.Seeker`、文件大小或整体编码缓冲。持续不提供 EOF 的 reader 可能等待；调用方负责超时/取消或外层定界。

`Limits` 默认值与其他 audio reader 相同，MaxBytes 包括 header、annotation、有效编码数据；unknown-size 失败时至多多消费前述一个探测字节。
AU 没有 chunk 表，`MaxChunks` 仅参与负数/默认值验证，没有额外 AU chunk 计数语义。
每 100 次连续 `(0,nil)` 读取失败为 `io.ErrNoProgress`，有进展就重新计数；不会无限空转。
全部错误返回 nil PCM；除将不完整已知对象的 EOF 表述为 `io.ErrUnexpectedEOF` 外，底层 I/O 错误保留包装链。

已知长度仅持有一个精确大小 float64 样本数组、32 KiB 样本 scratch、4 KiB annotation scratch 和固定 header。
unknown-size 在读取到完整帧后增长 float64 数组；每个数组容量独立受上述预算限制，增长时旧/新数组可短暂并存，最终容量可能大于有效样本数。
MaxSamples 不包括调用方持有的输入或增长时另一数组，B/op 也不是峰值内存；需要更低内存应降低预算，或使用已知大小对象。
返回 PCM 独立持有数据，仍遵守 `Samples()` 返回副本及并发只读语义。

### AU 独立验证与成本

开发工具 `tests/reference/audio_au.py` 不调用 Go 生成期望结果；使用独立 signed32 整数 oracle、已验证的 ITU G.711 midpoint 表，并使用 libsndfile 1.2.2 独立编码真实 AU。
SunOS/Oracle `au(4)`、libsndfile 1.2.2 AU reader 和 FFmpeg 8.0.1 AU demuxer 的来源 URL、抓取 SHA256 记在 `tests/data/audio/au/manifest.json`。
首次生成抓取来源，后续复用已抓取来源 hash；G.711 midpoint 输入也记录独立 SHA256。
测试源为 `tests/_cases/audio/au_read_test.go`，fixtures/原始日志为 `tests/data/audio/au/`。
默认从 go.mod 找到中央 fixture 位置；`GOMATH_AU_REFERENCE_DIR` 可覆盖，不依赖 TestMain 的 cwd。
普通测试只读 Go 和固定二进制数据，不需要 FFmpeg、libsndfile、Python、cgo 或进程启动。

当前矩阵 106 个支持流、1478712 个标量样本，包含 12 个真实 libsndfile 编码流、全部六种编码、全部 G.711 codes、PCM 极值/符号/大小端、1/2/3/6/64 声道、空/单帧、32 KiB 边界、未知大小、非对齐 offset、全零/文本/二进制 annotation、整数采样率边界。
libsndfile 完整解码 **106/106** 的全部字节、长度、格式均与 oracle 一致。
FFmpeg 严格 `-xerror -err_detect explode` 完整解码 **100/106** 精确一致；其余六个是合法空音频输入，FFmpeg 拒绝打开，而 Go/libsndfile 返回零帧。
没有用时长、裁剪或声道转换掩盖长度差异。每个流记录编码 hash、期望 PCM hash、两个独立解码器完整输出 hash/字节数/差异数。
另保存十个拒绝流，包括真实 libsndfile float/double/G.72x/小端文件、header 内 offset、声明半帧及已知/未知长度截断；外部实现的宽松接受行为单独记录，Go 不跟随它们补齐或忽略坏帧。

复验命令（runner 自动加 `noasm`）：

```sh
python3 tests/reference/audio_au.py # 开发环境需 FFmpeg / numpy / soundfile
CGO_ENABLED=0 GOMAXPROCS=1 env -u GOMATH_AU_REFERENCE_DIR go run ./tests/cmd/run -pkg=audio -count=1 -cover -p 1
CGO_ENABLED=1 GOMAXPROCS=8 go run ./tests/cmd/run -pkg=audio -race -count=1 -p 1
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio -run='^$' -fuzz='^FuzzReadAU$' -fuzztime=20s -parallel=1 -p 1
CGO_ENABLED=0 go vet -tags noasm ./audio
CGO_ENABLED=0 GOOS=linux GOARCH=386 go build -tags noasm ./audio
CGO_ENABLED=0 GOMAXPROCS=1 go run ./tests/cmd/run -pkg=audio -run='^$' -bench='^BenchmarkReadAU$' -benchmem -benchtime=300ms -count=5 -p 1
```

20 秒 fuzz 884138 次执行通过；完整 audio race 38.539 秒通过。race 的 CGO1 仅用于 Go race instrumentation，普通回归/解码均为 CGO0/noasm。
新解码文件 statement coverage 100%；完整 audio 96.4%。既有全部 audio 格式和 fixtures 通过中央 runner。
linux/386 生产包及全部中央 audio 测试均交叉编译通过；编译产物已核对为静态 ELF32/i386 后移除，证明保存在 `linux386_tests.json`，未在当前 ARM Mac 上执行。
依赖审计为 89 个标准库包加 audio，无第三方模块、cgo、os/exec、plugin 或 audio 汇编；go.mod 与既有生产 Go 源码、测试源码不变。

2026-11-04，Go 1.27.3 / darwin-arm64 / Apple M2；CGO0/noasm/GOMAXPROCS1，`-p 1 -benchtime=300ms -count=5`。
每次解码一秒 48000 Hz 立体声（48000 帧、96000 标量样本），PCM 使用固定极值序列，G.711 使用所有 code 的循环。
输入事先构造；计时包含 header 校验、全部解码、结果分配、unknown-size 增长及 EOF 检查，排除 fixture 磁盘读取、输入构造、独立编解码和程序启动；没有图像/视频负载。

| 编码 | known 中位 ms/op | unknown 中位 ms/op | known B/op / allocs | unknown B/op / allocs |
| --- | ---: | ---: | --- | --- |
| μ-law | 0.068750 | 0.146297 | 807032 / 6 | 1871992 / 8 |
| PCM8 | 0.126039 | 0.201711 | 807032 / 6 | 1871992 / 8 |
| PCM16 | 0.141997 | 0.222433 | 807032 / 6 | 2003064 / 9 |
| PCM24 | 0.167874 | 0.281199 | 807032 / 6 | 2764920 / 10 |
| PCM32 | 0.130529 | 0.244328 | 807032 / 6 | 2068600 / 10 |
| A-law | 0.067564 | 0.143283 | 807032 / 6 | 1871992 / 8 |

原始五次样本见 `bench.txt`，环境、全部样本和 min/max 见 `benchmark_summary.json`。
unknown-size 因增长、复制和 EOF 检查，实测更慢、分配更多；不同位宽产生不同增长容量。B/op 为总分配而非峰值/驻留内存。
以上仅为共享开发机固定负载观察，不宣称普遍更快或优于外部解码器。
