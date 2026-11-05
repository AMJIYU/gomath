# 有限 mathtex array 环境

`mathtex.New` 现在接受 `\begin{array}{colspec}…\end{array}`。
不新增公共 option、不增加依赖；布局、字体轮廓、规则线和动画均使用纯 Go。
这是明确 spacing policy 的有限 math subset，不是完整 TeX/LaTeX/amsmath 或
Manim MathTex 排版等价声明，也不通过外部程序 fallback。

```tex
\color{blue}\begin{array}{|l|c|r|}
\hline
n & \frac{n}{g} & W \\
\hline
\mathbf{W} & \phantom{g} & \mathit{i} \\
\hline
\end{array}
```

## 接受的语法与作用域

- colspec 必须是一个 literal braced token，至少一个小写 `l`/`c`/`r`，分别按
  cell 的完整 layout advance 左/中/右对齐；允许 Unicode 空白。
- 每个列边界可有一条 `|`，包括首尾；没有边界线时也保留列 padding。
  `||`、递归 braces、宏/展开、`p/m/b`、`@/!/>/<`、`*{n}{…}`、长度、任意
  插入文本等均明确报错。不是先解析再丢掉不认识的 spec。
- cell 用 `&` 分隔、行用 `\\` 分隔。每行必须恰好匹配 colspec 列数，不隐式
  补空列。显式空 cell 可写 `{}`；多列可写 `&`/`&&` 等。空环境、裸空行、
  连续行分隔符报错；允许一个末尾 `\\`，不生成额外空行。
- `\hline` 只在首行之前或紧随 `\\` 的行边界接受，每个边界最多一条。
  底线写作最后一行的 `\\\hline\end{array}`。cell 中途的 `\hline`、重复
  `\hline`、`\cline`、`\noalign`、`\multicolumn`、可选/starred 行距和
  `[t]/[b]` 环境定位等明确拒绝。首边界只有 hline 而没有 row 仍是错误。
- cell 复用既有 math grammar，可包含嵌套 array/matrix/aligned/gathered、
  分数、根式、scripts、accent、支持的数学字母、真实 regular/bold/italic/
  bold-italic face、颜色和 phantom。未知命令、缺字、缺失 custom font variant
  即使位于 hidden operand 中也报错。
- array 继承调用点的 display/text 模式与字体。显式 `\displaystyle`/
  `\textstyle` 只影响当前 cell，其状态按既有 row scope 恢复。没有新增 TeX
  强制 textstyle/default style 规则。script 中的 array 使用实际局部 size；
  分数命令仍遵循自身已有 nominal-em policy。
- cell 颜色 declaration 在该 cell 结束时恢复。所有边界规则线使用进入 array
  时的颜色；cell 内的 `\color` 不改变相邻 cell 或边界颜色。整个 array 可以
  被 `\textcolor`/字体 group 包裹，不把 style 状态泄漏到后续兄弟。

## 可核对的 layout policy

设 `s` 为当前 array 的局部 em。每列内容宽度
`C[j] = max(.25s, max(cell.width))`，cell.width 包括 advance 和 phantom 留白，
不使用可见 ink bbox 替代。每行
`A[i] = max(.70s, max(cell.height))`，
`D[i] = max(.20s, max(cell.depth))`；同一行的所有 cell 共用 baseline。

| 项目 | 值 |
| --- | ---: |
| 每列左右各自 padding | `.25s` |
| 表格上下各自 padding | `.15s` |
| 相邻行 clearance | `.30s` |
| 规则线 thickness | `.045s` |
| 表格中心相对 math axis | `.16s` |

每条竖线另占 `.045s` 宽的 band，每条 hline 另占 `.045s` 高的 band。
表格宽度为 `sum(C) + .5s * columnCount + .045s * verticalRuleCount`。
总高度为 `sum(A+D) + .3s * (rowCount-1) + .3s + .045s * hlineCount`；
layout ascent 为 `.16s + total/2`，depth 为 `total-ascent`。

内容列起点在左 padding 之后；l/c/r 的局部平移依次为
`0`、`(C-cell.width)/2`、`C-cell.width`。行 baseline 从 top 扣除顶部 rule band、
`.15s` padding 与第一行 ascent，之后逐行扣除 depth、clearance、可选 rule band
和下一行 ascent。中间 hline 在两行 clearance 的中心 band；首尾 hline 在表格
外侧 band 中心。竖线从 layout top 延伸到 `-depth`，横线从 `x=0` 到总宽度。

每条规则线都是独立的原生 Line path、一个真实 cubic segment、butt cap；
不借字符 `|/-`、bitmap 或 placeholder 表示。规则的半 stroke 包含在预留 band
内。规则线在 cell 前绘制；半透明颜色的交叉处沿用普通对象 alpha 合成，不新增
组隔离。返回的内部节点名包括 `Environment array`、`array cell[i,j]`、
`ArrayVRule[j]`/`ArrayHRule[i]`，便于检查，未提供新的稳定索引 API。

`phantom` 保留宽/ascent/depth，`hphantom` 只留宽，`vphantom` 只留 ascent/depth；
隐藏后的 array 不返回假透明轮廓。其所有源 cell、规则线、glyph 和结构路径已
计费，不因为丢弃 ink 而返还。空/phantom cell 仍参与所在列与行的 layout metrics。

## 共享限额、错误和兼容性

继续使用一个 parser 的既有预算，不为 array 或每个 child 创建新 parser/ledger：

| 限额 | 值 |
| --- | ---: |
| 整个 formula source | 16,384 runes，原始字节最多其4倍，必须有效 UTF-8 |
| row/group/environment 等解析深度 | 128 |
| 单 array 行 / 列 | 64 / 64 |
| 全 formula environment cells + substack rows | 4,096 |
| 共享 charged layout curves | 1,048,576 |
| Options.Size | finite，`(0,1e6]`；不可表示的局部 size/clearance 明确报错 |

spec 不展开，因此最多64列、65个竖线边界；最多64行、65个 hline 边界。
一个满规则空64×64 array 的 cell计数为4096、规则curves为130。array 在解析新
cell 前预留该 cell 的费用，因此嵌套环境不能先耗尽4096后漏计外层 cell；隐藏
phantom 同样计费。嵌套的旧环境仍沿用自身既有计费/错误时机，失败时不发布结果。
source 与 depth 包括 spec、nested 和 phantom 中的内容。

进入 array 即启用既有 operand audit，复用同一个 `curves` 和 `charged` 集合：
每个 cell 中既有 glyph/accent 已计费路径不会重复扣费，legacy `\frac`、未加
index 的 `\sqrt`、delimiter 等此前仅受 source/depth/cell 约束的结构路径，
在这个新 array operand 内也会计费。规则线同样进入共享账本。嵌套 array、boxed、
indexed radical 与 phantom 的审计不重置计数；discarded hidden geometry 保留费用。
新功能不重写 array 外旧 legacy structural path 的历史预算语义。

旧 matrix/cases/aligned/gathered 与其他 begin 分支保持原布局代码；仅在环境分派
增加 array case。原先缺少 colspec 的 `\begin{array}x…` 继续错误。旧 fixtures 与
阈值不修改。所有出错路径都经 `New` 返回 `nil,error`，不发布部分 cell 或线条。
Options 按值传入，layout 状态留在单次 parser 内；各次 New、快照与返回的规则/
字体几何互不共享可变路径。并发只读 font/layout、作者完成后的 Scene.Sample
沿用既有契约；不支持采样时同时修改共享 authoring tree。

## 独立参考与动画

数据在 `tests/data/mathtex/arrays/`，脚本为 `tests/reference/mathtex_arrays.py`。
fontTools 直接读取原始 Go font TTF 的 cmap/hmtx/glyf，手写 operand 与上面的布局
公式生成 SVG，再由 CairoSVG/Cairo 解释路径；没有读取 gomath 输出或运行 TeX。
7个静态用例包含四个实际字体 face、l/c/r、边界线、fraction 和 phantom/空 cell；
另有 `.35/.7` 两个独立线性仿射 Transform 参考。源 font、脚本、SVG/PNG SHA256
和版本写入 `reference.json`。Go 测试/运行时不需要启动 Python 或安装这些开发库。

内部 policy/rule/advance/baseline 检查误差阈值为 `1e-8`（affine controls 为1e-12）。
独立 source-font metrics 容许每行累计一个 design-unit 差异：`(rows+1)/2048` em；
这是原始 quadratic extrema 与 sfnt design-unit rounding 的边界，不宣称精确 TeX metrics。
新 raster 参考阈值为 RGB MAE `.08/255`、RMS `1`、最大通道差 `64`、逐色核心 mask
IoU `.97`。此阈值只用于新增参考，不修改旧阈值。

Write 在0/.35/.7/1采样，验证实际规则与 glyph reveal、不同中间像素和快照独立；
Transform 还检查所有控制点及独立 Cairo 中间帧。当前 Write 是既有 Create 别名，
不新增 TeX 阅读顺序/手写笔顺策略，也不声称独立验证了 partial glyph 的抗锯齿。
原生 scale1 signed-area AA 对局部复杂轮廓的已知边界不由此功能修复。

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=mathtex -tags noasm -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./mathtex
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -vet=all -run '^$'
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=mathtex -tags noasm -run '^$' \
  -bench BenchmarkArrayLayout -benchmem -benchtime=150ms -p=1 -cpu=1 -count=3
```

race 使用 Go race instrumentation 所需的开发 cgo；生产与普通 noasm 验证为CGO0。
全新专项验证记录于同目录 `validation.json`，不引用第六轮 checkpoint 作为新功能
通过依据。共享主机基准只观察一个2×3 array（实际7条边界规则、bold glyph、分数、
phantom、indexed radical、operator/script、nested matrix）的 New 布局；计入解析、
字体轮廓、audit 和对象构造，排除参考生成/渲染/文件读取。图像尺寸不适用、帧数0，
不作全库性能提升声明。

## 本轮独立像素结果

2026-11-05，原始768×768参考与原生 scale1 输出逐 RGB 通道比较；没有放宽上述
新增阈值或旧参考阈值。下表的误差单位是8-bit通道值，IoU为最差逐色核心mask。

| 用例 | MAE | RMS | 最大差 | 最差 IoU |
| --- | ---: | ---: | ---: | ---: |
| lcr-rules-0 / regular | .02314645 | .52404015 | 50 | .99296283 |
| plain-lcr | .01718083 | .42514367 | 32 | .99176299 |
| fraction-phantom | .01115248 | .32208078 | 40 | .99015219 |
| empty-phantom | .00058718 | .02423181 | 1 | 1 |
| lcr-rules-1 / bold | .01489597 | .34275226 | 37 | .98271768 |
| lcr-rules-2 / italic | .02651638 | .53915475 | 43 | .99330438 |
| lcr-rules-3 / bold-italic | .01772845 | .37000098 | 35 | .99706432 |
| Transform .35 | .01702146 | .34292782 | 26 | .99459764 |
| Transform .70 | .02570597 | .55922622 | 47 | .99136299 |

开发参考版本为 Python3.11.5、fontTools4.45.1、CairoSVG2.7.1、Cairo1.18.4；
Go字体来自 `golang.org/x/image@v0.47.0` 原始TTF。脚本、4个字体、18个SVG/PNG
文件的SHA256已逐一复核。详细测试输出保存在同目录 `independent-results.txt`。

## 正式源码验证与受限基准

最终源码包含“先预留外层 cell”修正及其可复现的精确限额回归；最终验证未使用
暂存 overlay，也未排除旧测试：

- 完整 mathtex CGO0/noasm：通过，2.936s；包含旧精确 fixtures 与全部新测试。
- 完整 mathtex race/noasm：通过，44.374s；包括独立 layout、Scene.Sample、返回
  快照修改，以及规则线/字体轮廓所有权检查。
- mathtex 生产源码 vet 与中央测试 `-vet=all`：通过。
- CGO0/noasm purecheck：42个非标准包通过，无 native/process-launch runtime imports。
- 独立参考：7个静态和2个中间 Transform 帧通过；末尾 row、隐藏预算、实际64×64
  最大表格、错误 `nil,error`、Write 中间帧等回归通过。

基准主机为共享 Apple M2 / darwin-arm64 / macOS15.7.1，Go1.27.3；CPU=1、p=1，
每样本150ms、共3次。固定上述2×3 array 的最终样本为125632、131931、130218 ns/op，
251185、250927、250927 B/op，各749 allocs/op。布局帧数0，无图像尺寸；该数字包含
真实字体与结构路径构造，不代表整库加速或独占主机性能。原始输出为 `benchmark.txt`。

`validation.json` 记录本轮专项命令、工作量、像素结果及源码/文档/工具/参考/输出
SHA256。这是 mathtex array 的聚焦验证，不是整仓冻结验收记录；未修改旧fixtures、
其他模块生产源码或共享主文档。暂存目录已移除，本切片交付后冻结。
