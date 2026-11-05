# 有界 mathtex 横向花括号

第十批小切片支持真实 operand 上的 `\overbrace`、`\underbrace`，以及直接跟随
brace atom 的 `^` / `_` 上下标签。产品只调用 Go 的解析、字体轮廓、矢量和动画
接口；CGO0/noasm，无 Python、TeX、Cairo 或子进程运行时依赖。

```go
opt := mathtex.DefaultOptions()
o, err := mathtex.New(`\overbrace{x_i^2+y}^{n}_{\text{terms}}`, opt)
// err 必须检查；成功返回的 o 可直接交给 Scene、Write、Transform 或 SVG 导出。
```

operand 可以是单个 atom 或 `{...}` 中的完整数学行，包含现有受支持的脚本、
分数、根式、数组、颜色、字体和嵌套 brace。推荐给复杂 operand 和标签加花括号：

```tex
\underbrace{\frac{x_i^2}{\sqrt[3]{g}}}_{k^2}
\overbrace{\underbrace{x+y}_{m}}^{n_i}
z^{\overbrace{x}^{n}}
\color{green}\overbrace{\textcolor{red}{\mathbf{x}}}^{\mathit{n}}
```

两个标签可任意顺序出现；不重复同一方向。对 overbrace 和 underbrace，`^` 都
居中放在整个 brace/body 上方，`_` 都放在其下方，inline/display 都采用此策略。
标签不是任意位置的隐式脚本吞并：operand 内的 `x_i^2` 必须属于 operand 的组。
`\overbrace x^n` 中 `^n` 属于 brace。需要带标签的 brace 作另一个脚本时，应写
`z^{\overbrace{x}^{n}}`；无组的多层脚本歧义不扩展解释，重复脚本会报错。

只有直接解析的 brace atom 拥有此标签规则。外层组或字体/颜色包装会变成普通
atom，例如 `{\overbrace{x}}^n`、`\mathbf{\overbrace{x}}^n` 使用既有右侧脚本。
应把标签写进包装内部：`\mathbf{\overbrace{x}^{n}}`。现有 operator 的脚本规则
没有改变；新代码没有向 operatorKind 塞入伪 operator。

## 有限 em 布局

本功能不是完整 TeX 或 TeX 可伸缩字形组装。每一个花括号是原创建模的单个封闭、
实心 ribbon，含 8 段三次 Bézier 曲线；不是 Unicode 花括号字符、横线或空 API。

| 几何项 | 本地 em 策略 |
| --- | --- |
| 水平跨度 | `max(operand.advance, .6em)`，包括空格/phantom advance |
| 花括号高度 | `.28em`，宽度增加时不增加高度 |
| 带厚度 | `.045em`，外层 bold/bold-italic 时 `.060em` |
| operand 到 brace | 在布局 ascent/depth 和实际路径 bounds 的外侧留 `.10em` |
| 标签字号 | 调用点字号的 `.65`；嵌套脚本继续按已有规则缩小 |
| 标签间距 | 从完整 brace/body 的 ascent/depth 向外留 `.12em` |
| 标签水平位置 | 可见标签按真实 ink 中心对齐；无 ink 标签按 advance 中心对齐 |
| 基线与总宽度 | operand 的 y=0 保留；宽标签/最小跨度只增加横向占位 |

曲线有左右端 curl 和中央 cusp；underbrace 是垂直翻转。颜色取 brace 命令处的
上下文，operand/标签中的局部声明不泄露到标记或后续 atom。字体命令仍使用真实
字体轮廓；自定义字体缺少所需 style variant 时沿用既有错误，不以路径假粗体补齐。

`{}`、`\quad`、`\text{ }`、phantom/hphantom/vphantom 都是有效 operand。
无 ink 的 operand 不会出现假字形，但仍用语义 advance/ascent/depth 放置 brace。
零宽度 operand 的 `.6em` brace 在原 advance 中心居中，总布局为其预留宽度。
无 ink 标签也保留其语义尺寸；空标签有间距占位，不会被当作可见文字替代。

命令 `\limits`、`\nolimits`、`\displaylimits` 在 brace 后显式报错；不支持
宏、用户尺寸指令、overbracket、自动 TeX font assembly、完整数学间距、完整
TeX 样式/limits 切换。未知命令、损坏组、孤立/重复标签均返回 `nil,error`，
不丢弃内容继续渲染。已有子集能否排版某个 operand，仍由其原有语法边界决定。

## 预算、原子性与所有权

沿用同一个 parser 的 source/depth/cells/curves；不创建局部 parser 重置计数器。
source 最多16,384 runes，depth 最多128，environment/substack cells 合计4,096，
曲线合计1,048,576。operand 复用 accentArgument 的单 atom 嵌套计数，标签复用
scriptAtom 的字号、text style 和嵌套边界。每个 brace 新增8条曲线。

brace 在解析 operand 前开启现有 operand audit。glyph/mark 路径记录已收费节点，
legacy 分数线、根式、delimiter 等结构路径补收一次；标签也审计。phantom 被丢弃
的路径不退款，嵌套 brace、隐藏 operand、隐藏标签均不得绕过预算。新增语法遵循
已有预算覆盖范围；没有改写 brace 之外旧构造的全局收费政策。

尺寸为零、非有限值、下溢、非法路径/样式都会返回错误。`New` 不返回半成品，
`LayoutParts` 中后续无效 fragment 不暴露之前的对象；共享输入切片不会被修改。
没有改动 parts.go 或引入与父线程 matching wrapper 重叠的 API。

作者操作沿用可变 Object；完成作者操作后，独立 layout 和 Scene.Sample 可并发。
测试覆盖独立路径存储、快照修改隔离、Transform 的 root/mark/label ID 与原始
child pointer 保留。Write 沿用现有 Create/reveal 语义，未声称 TeX 笔顺。

## 独立参考与来源

`tests/reference/mathtex_braces.py` 直接读取 `x/image@v0.47.0` 的4种原始 Go TTF，
用 fontTools 读取 hmtx/glyf 并生成原始轮廓；另行手工实现上述有限布局，再由
CairoSVG/Cairo 生成768×768、128px/em参考。它不读取 Go 导出的 SVG、曲线、
布局或 PNG。Go 测试只读取已提交 fixtures，不启动参考程序。

Manim 固定为 CE0.22.1 commit `16ea443e2a5f1d79db0a721161411ce916d6e733`。
生成器只读核查其 HEAD 与 clean status，并记录 tex_mobject.py、tex_file_writing.py、
transform.py 与 LICENSE 的 SHA256；MIT 全文保留在 reference.json。Go 曲线为原创，
未复制 Manim/TeX 的轮廓；Go 字体版权保留在仓库 LICENSES/go-fonts.txt。
Manim 的 MathTex 经过外部 TeX 管线，本轮未执行 Manim/TeX，不声称其布局或
字形伸缩机制的像素 parity。这里只把它作为固定源码 API/管线参考。

独立样本：4种字体的双向标签、嵌套、空 operand、phantom、标签中的 brace，
共8张静态图；另有线性 Transform 在 `.35`、`.70` 的独立仿射中间帧。
Go 另测 Write 的0/.35/.70/1中间像素与 reveal。本轮没有以 Go oracle 冒充独立参考。

参考 hash、生成器 hash、字体 hash 在测试中校验。字体原始二次曲线的极值与 Go sfnt
设计单位取整允许4/2048em的累积度量误差；有限策略另有精确几何测试。像素限额为
RGB MAE≤.08、RMS≤1、最大通道差≤64、ink-mask IoU≥.97；现有阈值未改动。

## 验证与当前整包阻塞

所有日志位于 `tests/data/mathtex/braces/`。验证命令、退出码、环境、样本统计见
`validation.json`；最终源码/测试/文档/参考/日志的冻结 hash 见 `freeze-manifest.json`。

```sh
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/lib python3 tests/reference/mathtex_braces.py \
  --gofont-dir /Users/eleven/go/pkg/mod/golang.org/x/image@v0.47.0/font/gofont \
  --manim-dir ../manim
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -run '^TestBrace' -count=1 -v
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -run '^TestBrace' -count=1 -v
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -count=1 -v
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -count=1 -v
CGO_ENABLED=0 go vet -tags noasm ./mathtex
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -vet=all -run '^$'
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -run '^$' \
  -bench BenchmarkBraceLayout -benchmem -benchtime=150ms -p=1 -cpu=1 -count=3
```

race 使用工具链要求的开发 cgo；产品和普通测试为 CGO0。没有运行全仓 race 或
重跑大整仓测试。纯 Go 依赖检查与单包生产/集中测试 vet 分别执行。

已知整包冲突：原 `tests/_cases/mathtex/accents_test.go` 的
`TestAccentMalformedArgumentsAndInputBudgets` 仍把 `\overbrace{x}`、`\underbrace{x}`
列入 unsupported。这与新增能力互斥。该文件不在本轮允许写范围，故未修改、未跳过、
未用临时 overlay 隐藏；完整 CGO0 与 race 命令的真实结果应据日志读取，不能称整包
全绿。父线程整合时需要从此旧 unsupported 列表移除这两个命令；其余检查应保留。
新 brace 测试仍显式检查所有不支持的命令、非法标签和预算分支。

2026-11-05 最终结果：

| 检查 | 实际结果 |
| --- | --- |
| Brace focused CGO0/noasm | 10个顶层测试通过，0.196s |
| Brace focused race/noasm | 10个顶层测试通过，3.925s，无 race warning |
| 完整 mathtex CGO0/noasm | 122通过、上述1条旧断言失败，2.859s，退出1 |
| 完整 mathtex race/noasm | 122通过、同1条旧断言失败，47.763s，无 race warning，退出1 |
| 生产 vet / 集中测试 vet=all | 均退出0 |
| CGO0/noasm purecheck | 45个非标准包通过 |
| 独立静态及中间帧 | 10张通过，最差IoU .991572173，最大通道差43 |

基准仅测 New 的真实解析/字体/结构路径/audit/对象构造：short 为 `x+y` 上花括号
和上标签；nested 为分数、下花括号、indexed radical、substack、phantom 与脚本。
排除独立参考生成、栅格化与磁盘读写；图像尺寸不适用，布局帧数0。共享主机 Apple M2 /
macOS15.7.1 / darwin-arm64 / Go1.27.3；CPU=1，p=1，150ms，3样本；不作全库性能提升声明。

short 的3次采样为33,150 / 33,098 / 32,463 ns/op，各65,840 B/op、195 allocs/op；
nested 为162,689 / 159,407 / 161,175 ns/op，299,776–299,777 B/op、730 allocs/op。
原始样本在 benchmark.log，不将共享主机的微基准解释成吞吐承诺。

## 改动清单

- `mathtex/mathtex.go`：box 的直接 brace 标记、row 脚本分流、两个命令分发及预算注释。
- `mathtex/braces.go`：真实 ribbon 几何、operand 审计、上下标签及有限尺寸处理。
- `tests/_cases/mathtex/braces_test.go`：语法/几何/作用域/边界/共享预算/并发/Write/基准。
- `tests/_cases/mathtex/braces_reference_test.go`：来源与 hash、独立度量/像素、Transform 中间帧。
- `tests/reference/mathtex_braces.py`：开发阶段独立参考生成。
- `tests/data/mathtex/braces/`：独立 fixtures、验证日志、命令/结果与 freeze manifest。
- `docs/MATH_BRACES.md`：本说明。

共享 README/STATUS/PARITY、其他生产文件、parts.go、示例、依赖和 `../manim` 未由本轮修改。
共享工作区中父线程的其他改动不在本切片冻结范围；mathtex.go 的 hash 是记录时刻的整文件值。
