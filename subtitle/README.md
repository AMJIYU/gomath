# subtitle

纯 Go、标准库实现的不可变字幕集合及严格有界 UTF-8 SRT/WebVTT 读写。
时间使用整数毫秒，运行时无外部程序、native/cgo 或额外 Go 模块依赖。

## API

```go
cue, err := subtitle.CueAt("你好，世界\nSecond line", 1000, 1234, -500)
if err != nil { return err }
// cue.StartMS == 500; cue.EndMS == 1734

track, err := subtitle.New([]subtitle.Cue{cue}, subtitle.Options{
    Order: subtitle.OrderStableSort,
    Overlap: subtitle.OverlapAllow,
})
if err != nil { return err }
track, err = track.Append(subtitle.Cue{
    StartMS: 2000, EndMS: 3001, Text: "下一条字幕",
})
if err != nil { return err }
return subtitle.WriteSRT(dst, track, subtitle.WriteOptions{})
```

| API | 行为 |
| --- | --- |
| `Cue{ID, StartMS, EndMS, Text}` | 值类型字幕，非空半开时间区间 `[StartMS,EndMS)` |
| `CueAt(text, nowMS, durationMS, offsetMS)` | 当前场景时间加有符号偏移，检查溢出和默认限制 |
| `New([]Cue, Options)` | 验证并复制输入，构造不可变集合 |
| `Track.Cues()` / `Track.Cue(index)` | 返回 cue 切片副本 / 检查索引后返回值 |
| `Track.Len()` / `Track.Options()` | 集合长度 / 实际策略和限制值 |
| `Track.Append(...Cue)` | 返回新集合；沿用原集合的策略和限制 |
| `Track.Renumber()` | 显式把所有 ID 改为从 1 开始的十进制序号，返回新集合 |
| `ReadSRT(reader, Options)` / `ReadWebVTT(reader, Options)` | 有界解析，不跳过损坏 cue |
| `WriteSRT(writer, track, WriteOptions)` / `WriteWebVTT(...)` | 预先检查全部内容和输出大小，再流式写出 |
| `ParseTimestamp(text, Format)` / `FormatTimestamp(ms, Format)` | 精确毫秒解析和格式化，格式常量为 `SRT`、`WebVTT` |

`Track` 的零值无效，合法空集合用 `New(nil, Options{})` 创建。
构造器复制 cue 切片并克隆新传入的字符串，避免保留大字符串的子串后备存储。
派生集合有独立 cue 数组，可共享不可变的既有字符串。读取、追加、编号和写出可并发调用。
调用者负责独立管理传入 reader/writer；不得在方法执行时并发修改传入切片。
无效输入返回错误；仅查询元信息的 getter 用于合法构造的集合。

## 时间、顺序和重叠

`StartMS >= 0`，`EndMS > StartMS`。使用精确整数运算，不经浮点数、`time.Duration`、截断或帧率换算。
`CueAt` 接受正时长及有符号偏移，偏移后的起点不得为负，并检查所有加法溢出。
默认时间范围是 24 小时，可通过 `Limits.MaxTimestampMS` 改为其他正 int64 上限。
单独的时间戳函数支持全部非负 int64 毫秒，超出范围或语法错误会报错。

- `OrderStrict`（默认）：起点必须非递减；不会自动调整顺序。
- `OrderStableSort`：按起点稳定排序，相同起点保持输入顺序。
- `OverlapReject`（默认）：拒绝任何相交时间区间；相邻 `end == next.start` 合法。
- `OverlapAllow`：完整保留重叠，不合并、不裁剪、不覆盖文本。

非空 ID 在一个集合中必须唯一。`Append` 成功或失败都不修改原集合；解析也使用同一套策略。
这个包提供后续 `add_subcaption` 集成所需基础，不连接 Scene、导出器或视频容器。
参考时间关系来自只读 Manim 源码 `manim/manager.py:add_subcaption`，参考提交
`16ea443e2a5f1d79db0a721161411ce916d6e733`。这里只固定时间关系与明确的毫秒模型，不宣称已验证 Scene 行为对齐。

## 文本与格式边界

文本为 UTF-8，可有多个 LF 分隔的非空、非纯 ASCII 空格/制表符行。
保留 Unicode、emoji、行内和行首尾空格、制表符，不自动 trim、重排或 Unicode 正规化。
拒绝 NUL、CR、其他 C0/C1 控制字符、无效 UTF-8、空文本和空行。
输入文件接受 LF、CRLF 和文件起始的单个 UTF-8 BOM；解析为逻辑 LF，规范输出不写 BOM。
控制字符不允许出现在 ID 中，ID 也不得包含 `-->`。

SRT：

- 读取规范正十进制 uint64 序号，不接受 0、正负号、前导零或缺失序号；非连续序号可用并会保留。
- 时间要求 `HH:MM:SS,mmm`；小时至少两位，分秒严格两位且为 00..59，毫秒严格三位。
- 写出时保留已有数字 ID，无 ID 的 cue 按集合位置生成 1-based 序号；若发生 ID 冲突则报错。
- 匿名 cue 经 SRT 写入再读取后会带上生成的序号。要求全部字段一致时，可先显式 `Renumber()`。
- 不静默丢弃 WebVTT 的 opaque ID；转 SRT 前应明确调用 `Renumber()`。
- 仅支持纯文本子集：`<` 和实体形式的 `&...;` 会报 `ErrUnsupported`，避免不同 SRT 播放器隐式解释 HTML；普通 `R&D` 可用。
- 不支持坐标/字体等 timing 扩展；纯空文件合法。

WebVTT：

- 要求 `WEBVTT` 单独一行，后接空行；可有 ID，时间支持 `MM:SS.mmm` 或 `HH:MM:SS.mmm`，写出统一使用带小时的形式。
- 支持纯文本和 `&amp;`、`&lt;`、`&gt;`、`&nbsp;`、`&lrm;`、`&rlm;` 六个实体，解析为字面 Unicode 文本。
- 写出时转义 `&`、`<`、`>`，因此文本中的 `<b>`、`<时间戳>`、`-->` 或 `&#123;` 均保持字面含义，不会变成可见样式或语法。
- 原始标签、inline timestamp、ruby、voice、language、class 等标记明确报错。
- line/position/align/size/vertical/region 及任何其他 cue settings 明确报错。
- STYLE、REGION、NOTE 块、header annotation 和 `X-TIMESTAMP-MAP` 等元数据明确报错，不会跳过或丢弃。
- 数字或未知实体、裸 `&` 和原始 `-->` 也被严格拒绝；可用转义或直接 UTF-8 表示字面文本。
- ID 不得使用会被解释为 NOTE/STYLE/REGION 块开头的保留形式。

这是可往返的纯文本子集，不是支持全部 WebVTT/CSS/HTML 的渲染器。遇到未支持的可见内容会失败，不提供剥离标记的降级路径。
完整解析再写出的 cue 时间、文本和已有 ID 保持一致，但换行编码、时间戳写法、实体写法和块间空行会规范化。

## 资源限制和 I/O

各限制零字段使用默认，负数报错：

| `Limits` 字段 | 默认 |
| --- | --- |
| `MaxCues` | 10,000 个 |
| `MaxTextBytes` | 每 cue 解码后文本 65,536 bytes |
| `MaxTotalBytes` | 全部解码文本和 ID 合计 16 MiB |
| `MaxLineBytes` | 每条编码行 16 KiB，不含 BOM/CRLF/LF |
| `MaxIDBytes` | 每 ID 1024 bytes |
| `MaxBytes` | 编码输入或输出总量 32 MiB |
| `MaxTimestampMS` | 86,400,000 ms（24 小时） |

解析不整体加载文件，用 4 KiB 读取缓冲逐行处理，并限制逐行累积和每 cue 文本。
最多从 reader 读取 `MaxBytes+1` 个字节，以检测恰好超限；因此 `MaxBytes == math.MaxInt64` 被拒绝。
编码 byte 和 line 限制计算的是转义后的输出；解码文本限制计算的是 Unicode UTF-8 字节数。
构造/追加按 cue 数量和解码字符串总量界定；中间 cue 数组、排序和 ID map 也是有界的。
调高限制会相应增加内存/CPU 成本。解析完整流至 EOF，不提供流式 cue 回调、取消或 Reader 内容之后的 framing。

写出前验证全部内容，并用计数 sink 运行同一序列化器检查精确输出长度；随后用 4 KiB 缓冲流式输出。
不会为预检构造完整编码字符串。验证错误不会写入任何字节；实际 I/O 错误仍可能留下部分流。
调用者可用 `errors.Is` 检查 `ErrInvalidInput`、`ErrUnsupported`、`ErrLimit`、`ErrOrder`、`ErrOverlap`；底层 I/O 错误保留包装链。

## 验证

测试源/benchmark 仅在 `tests/_cases/subtitle/`，独立手写 fixtures 在 `tests/data/subtitle/`。
普通测试只依赖标准 Go 和本地 fixtures，不启动外部程序。
测试专用 `TestMain` 在执行前把普通测试及 fuzz worker 的工作目录固定到 `tests/data/subtitle/`。
Go 自动保存的失败 corpus 因而位于 `tests/data/subtitle/testdata/fuzz/<FuzzTarget>/`，不会写入生产 `subtitle/testdata/`；重放仍使用下列集中 runner。

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=subtitle -count=1 -cover
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=subtitle -race -count=1
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=subtitle -run='^$' -fuzz='^FuzzSubtitleStreams$' -fuzztime=5s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=subtitle -run='^$' -fuzz='^FuzzTimestampExactRoundTrip$' -fuzztime=5s -parallel=2
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=subtitle -run='^$' -bench=BenchmarkCodec1000Cues -benchmem -benchtime=300ms -count=3 -p=1
CGO_ENABLED=0 go vet -tags noasm ./subtitle
```

runner 自动使用 `-tags noasm`。测试覆盖精确毫秒和 int64 极限、Unicode 多行与实体、已有 ID 保留、三次连续读写往返、独立切片、并发追加/读写、时间排序/重叠、恶意无终止 reader、超长/跨缓冲 UTF-8 行、总量限制及拒绝可见 markup/定位。

可设置 `GOMATH_SUBTITLE_REFERENCE_OUT=<临时目录>`，运行 `-run='^TestIndependentDecoderArtifacts$'` 输出开发验证用 SRT/WebVTT 和期望 cue JSON。
已在开发环境用 FFmpeg 8.0.1 完整解码本包生成的两种格式，核对 cue 数量、精确毫秒、中文/emoji、多行文本、双空格和字面 `&`；这不是运行时依赖，也不证明未支持的 VTT 特性。

2026-11-04 记录：普通测试语句覆盖率 95.5%，完整 race 通过；五秒流解析 fuzz 执行 36,207 次、精确时间戳 fuzz 执行 321,727 次，无失败。

串行 benchmark 环境为 Go 1.27.3、darwin/arm64、Apple M2、`CGO_ENABLED=0`、`noasm`，`-p=1`，每项三次 300 ms 样本。
每次处理 1000 条不重叠字幕、总时间跨度 999.5 秒，每条含数字 ID、中文/emoji 和两行文本，文本包含普通 `&` 及双空格。
排除输入生成、磁盘读写、外部解码和程序启动，包含解析结果分配、写出前验证/计数及转义，写出目标是 `io.Discard`。
不涉及视频帧或图像尺寸；共享开发机的测量值不作隔离环境性能保证。

| 操作 | 三次 ms/op | B/op（约） | allocs/op |
| --- | --- | --- | --- |
| SRT 读取 | 0.891 / 0.882 / 0.924 | 693,005 | 15,912 |
| SRT 写出 | 1.656 / 1.461 / 1.404 | 320,701 | 13,914 |
| WebVTT 读取 | 2.564 / 1.176 / 1.165 | 869,019 | 22,014 |
| WebVTT 写出 | 1.646 / 1.650 / 1.667 | 313,499 | 13,013 |
