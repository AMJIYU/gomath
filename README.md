# GoMath

**纯 Go 数学计算、动画与可视化基础库。**

GoMath 将数值计算、数学对象、文字与公式、动画时间轴、二维/三维 CPU 渲染和媒体输出放在同一套 Go API 中。可以直接在 Go 程序里构造场景、编排动画、采样任意时刻，并输出图片、矢量图或视频。

项目长期目标是完整覆盖 Manim 的能力，并通过可复现的测量持续改进速度、内存和画质。当前已经打通从场景构造到原生视频输出的主要链路，**仍处于持续开发阶段，尚未完成全量 Manim 兼容，也未证明全库性能领先**。

本文功能范围截至 **2026 年 11 月 5 日**。逐项支持边界以 [兼容性清单](docs/PARITY.md) 和各专题文档为准。

## 目录

- [项目优势](#项目优势)
- [快速开始](#快速开始)
- [编写第一个场景](#编写第一个场景)
- [已经实现的功能](#已经实现的功能)
- [内置示例](#内置示例)
- [输出格式与质量选项](#输出格式与质量选项)
- [架构与包结构](#架构与包结构)
- [验证与性能记录](#验证与性能记录)
- [当前边界与后续方向](#当前边界与后续方向)
- [开发与贡献](#开发与贡献)
- [来源与许可](#来源与许可)

## 项目优势

### 纯 Go 的完整生成链路

场景采样、字体轮廓、受支持的公式排版、光栅渲染、视频编码、音频处理和文件封装均在 Go 中完成。产品运行时无需安装 Python、FFmpeg、LaTeX、Cairo 或浏览器引擎，也不使用 cgo、C/C++ 绑定和动态本地库加载。

通过 `CGO_ENABLED=0` 构建，可将 GoMath 集成到已有 Go 程序，或构建独立命令行程序。严格构建使用 `-tags noasm` 关闭第三方可选汇编路径；Go 工具链、标准库和运行时仍属于正常运行环境。

### 计算结果可以直接成为动画对象

数值线性代数、稀疏求解、坐标变换、函数绘图、流线积分与动画对象共享 Go 的类型和数据结构。计算结果可以直接用于构造曲线、矩阵、网络图和三维网格，减少跨语言交换及格式转换。

数值计算与展示保持独立：`linalg`、`sparse` 负责计算，`matrix`、`table` 负责排版与动画。可以只使用需要的包。

### 支持并行、乱序和可重复采样

完成场景编排后，可以按任意时间顺序采样，并发生成独立帧。对象在编排阶段可变，采样使用独立快照；修改某一帧不会污染其他帧或源对象。共享回调必须满足并发读取约束。

渲染器通过有界 worker 管线生成帧并按序写出，最多保留约 `2 × workers` 个处理中或等待写出的帧，避免将整个视频的像素帧同时放入内存。编码器另有帧数和元数据限额。

### 面向嵌入式使用的无界面渲染

二维和三维渲染均可在无图形窗口的环境中执行，适合数学教学内容、算法演示、科学计算可视化和批量媒体生成。`FrameSource` 与 `Sink` 接口将场景采样和输出解耦，可接入自定义数据源或输出端。

### 显式错误与资源预算

解析器、路径计算、图布局、流线、三维渲染及媒体处理对输入规模和工作量设有限额。受检查的接口对无效或不支持的输入返回错误；文件输出在成功完成后发布，失败时清理临时输出，避免留下半成品或覆盖已有文件。

### 有可追溯的正确性与优化证据

验证覆盖数值残差、对象身份、动画中间状态、独立参考图像、完整媒体解码和并发竞态。局部性能优化记录输入、版本、分配和画质边界，详见 [验证与性能记录](#验证与性能记录)。

## 快速开始

### 环境要求

- **Go 1.26.0 或以上**，依赖版本由 `go.mod` 与 `go.sum` 固定。
- 当前模块路径为本地 `gomath`，正式发布路径尚未确定。
- 已有 macOS arm64 测试及 Linux/Windows amd64 交叉构建记录；后两者仍需实机执行和视觉验证。

以下命令在项目根目录执行。

```sh
# 构建独立 CLI
CGO_ENABLED=0 go build -tags noasm -o bin/gomath ./cmd/gomath

# 导出函数与微积分动画
./bin/gomath demo -scene calculus -format gif \
  -width 640 -height 360 -fps 24 -out artifacts/quickstart-calculus.gif

# 导出纯 Go H.264 MP4
./bin/gomath demo -scene transform -format mp4 \
  -width 640 -height 360 -fps 24 -out artifacts/quickstart-transform.mp4

# 导出公式匹配动画中的一个矢量帧
./bin/gomath demo -scene matching-parts -format svg \
  -time 1.7 -out artifacts/quickstart-matching.svg

# 查看命令行参数和当前可选场景
./bin/gomath demo -h
```

输出路径必须不存在；重复执行时请使用新文件名。静态 `png` / `svg` 的 `-time` 必须位于场景时间范围内，`reference` 场景可使用 `-time .5`。

在另一个本地 Go 模块中使用时，可先通过 `go mod edit -replace=gomath=/你的绝对路径/gomath` 指向本项目，再执行 `go get gomath`。这是一种本地开发接入方式，不代表已存在公开可下载的模块版本。

## 编写第一个场景

下面是可运行的完整程序：创建方形，将其变为圆，再移动位置并显示公式，最后导出 MP4。

```go
package main

import (
    "context"
    "log"

    "gomath"
    "gomath/export"
    "gomath/geom"
    "gomath/mathtex"
    "gomath/render"
)

func main() {
    if err := run(); err != nil {
        log.Fatal(err)
    }
}

func run() error {
    scene := gomath.NewScene()
    square := gomath.Square(2).SetFill(gomath.Blue)
    if err := scene.Add(square); err != nil {
        return err
    }

    target := gomath.Circle(1.2).SetFill(gomath.Pink)
    if err := scene.Play(1.2, gomath.Transform(square, target)); err != nil {
        return err
    }
    if err := scene.Play(.8, gomath.Shift(square, geom.V(2, -.8))); err != nil {
        return err
    }

    formula, err := mathtex.New(`\frac{a^{2}+b^{2}}{c^{2}}=1`, mathtex.DefaultOptions())
    if err != nil {
        return err
    }
    formula.MoveTo(geom.V(-2, 1.5))
    if err := scene.Play(.8, gomath.FadeIn(formula)); err != nil {
        return err
    }
    if err := scene.Wait(.5); err != nil {
        return err
    }

    config := render.DefaultConfig()
    config.Width, config.Height, config.FPS, config.Workers = 640, 360, 24, 2
    sink, err := export.NewMP4WithOptions(
        "artifacts/my-first-scene.mp4",
        config.Width, config.Height, config.FPS,
        export.DefaultMP4Options(),
    )
    if err != nil {
        return err
    }
    _, err = render.RenderScene(context.Background(), scene, config, sink)
    return err
}
```

将程序保存到自己的 Go 模块后执行 `CGO_ENABLED=0 go run -tags noasm .`。坐标使用世界单位，Y 轴向上；`Play` 的第一个参数是秒数。

`Transform` 更新源对象，并保留已有对象及子对象引用；`ReplacementTransform` 会在场景中安装真实目标对象。若要同时改变多个属性，可以修改 `object.Copy()`，再提交一个 `Transform`。同一时段的并行动画不能同时写入同一对象或父子对象。

### 显式公式分片匹配

`mathtex.NewParts` 将分别合法的公式片段排在共同基线上，支持精确 key 选择、重复 key 和匹配动画。例如在场景构造代码中：

```go
options := mathtex.DefaultOptions()
source, err := mathtex.NewParts([]string{"x", `\quad+\quad`, "x"}, options)
if err != nil {
    return err
}
target, err := mathtex.NewParts([]string{"2", "x"}, options)
if err != nil {
    return err
}
if err := scene.Add(source.Object()); err != nil {
    return err
}
animation, err := mathtex.TransformMatchingParts(source, target)
if err != nil {
    return err
}
if err := scene.Play(1.5, animation); err != nil {
    return err
}
```

匹配使用显式片段字符串作为 key，支持重复数量变化、未匹配组淡入淡出及最终目标替换。它尚不提供自动 TeX 源码分片或自动形状识别，详见 [公式分片](docs/MATH_PARTS.md) 和 [匹配变换](docs/MATCHING_PARTS.md)。

## 已经实现的功能

以下列表描述当前可用子集；同名 API 不表示与 Manim 的所有默认值和生命周期完全一致。

### 数学计算与路径

| 方向 | 当前能力 |
| --- | --- |
| 几何计算 | 三维向量、4×4 矩阵、包围盒、轴角旋转、四元数和最短弧 SLERP |
| 稠密线性代数 | 不可变实数/复数矩阵；LU、列主元 QR、Cholesky；实对称 Jacobi 特征分解；一般实矩阵 Schur、复特征值和复右特征向量 |
| 分解与求解 | 高/宽/秩亏矩阵 economy SVD、伪逆、最小二乘和最小范数求解；复数 LU、多右端项求解、逆与 LogDet |
| 稀疏计算 | 严格 CSR、转置、加减、矩阵向量积、SpGEMM；支持显式 Jacobi 预处理的 CG / BiCGSTAB，以原系统残差验收 |
| 曲线路径 | 二次/三次贝塞尔、细分、包围盒、自适应展平、轮廓对齐、弧长与逆弧长、切线、重采样、按距离和参数区间裁剪 |
| 路径编辑 | 不可变测量快照与逆弧长查询索引；曲线数量扩展、平滑/折线控制柄、路径与对象族反向、逐点/复数映射变形 |
| 多边形布尔 | 闭合 XY 多边形并/交/差/异或；nonzero/evenodd、孔洞、自交与共线重叠；精确有理数求交及输出拓扑检查 |

详见 [线性代数](docs/LINALG.md)、[稀疏计算](docs/SPARSE.md)、[路径测量](docs/PATH.md)、[曲线编辑](docs/CURVE_EDIT.md)、[路径反向](docs/PATH_DIRECTION.md) 和 [多边形布尔](docs/BOOLEAN.md)。曲线布尔及通用非线性自适应细分尚未实现。

### 对象、几何与动画

| 方向 | 当前能力 |
| --- | --- |
| 对象系统 | ID、深拷贝、分组与父子树、样式、布局、独立快照、整树变换及受检查的原子编辑 |
| 基础几何 | 点、线、圆、椭圆、圆弧、矩形、圆角矩形、多边形、圆环、扇形、虚线、箭头和双箭头 |
| 连接与标注 | 两点圆弧、相切圆弧、曲线箭头、角/直角标记、包围框、背景框、下划线、叉号、Elbow、平面及弧形 brace |
| 核心动画 | Transform、TransformFromCopy、ReplacementTransform、显式 key 匹配、FadeIn/FadeOut/FadeTransform、Shift/MoveTo/Scale/Rotate |
| 绘制与强调 | Create/Write 及反向子集；Grow、SpinIn、Indicate、Wiggle、ApplyWave、PassingFlash、沿路径运动、圆弧变换轨迹 |
| 时间编排 | 缓动、时间窗口、lag、嵌套 AnimationGroup/LaggedGroup、不同子动画时长、受限共享对象和嵌套 Succession |
| 动态对象 | ValueTracker、绝对时间回调、固定拓扑 redraw、受预算约束的动态显示拓扑、固定步长 dt 积分和检查点回放 |
| 更新器生命周期 | 注册、暂停、恢复、永久移除；Play 期间对源对象族自动暂停受管理 updater |

详见 [动画](docs/ANIMATION.md)、[更新与回放](docs/UPDATERS.md)、[对象 updater](docs/OBJECT_UPDATERS.md)、[几何](docs/GEOMETRY.md) 和 [圆弧轨迹](docs/TRANSFORM_PATHS.md)。当前 Succession 的时钟/嵌套组合、dt 子树与动画重叠存在约束，未实现完整 `always_redraw` 语义。

### 坐标系、图表、网络与向量场

| 方向 | 当前能力 |
| --- | --- |
| 坐标系 | 带标签及方向的 NumberLine、Axes、NumberPlane；显式线性/对数刻度；复平面、极坐标平面及仿射正向/投影逆向映射 |
| 曲线与面积 | 函数、参数曲线、均匀/自适应绘图；带符号黎曼矩形、分段线性面积和两函数间区域 |
| 柱状图 | 正负数值、基线锚定、固定柱数更新与动画、稳定柱对象引用 |
| 网络图 | Graph/DiGraph；圆形、网格、固定种子的弹簧布局；显式布局/边更新、原子增删顶点和边、关联边级联删除 |
| 曲线网络 | 独立 CurvedDiGraph、双向曲线分离、自环及保留身份的显式更新 |
| 向量场与流线 | 有限 XY 采样、长度/颜色函数；固定 RK4、自适应 Dormand–Prince RK45、接受时间与终止原因；物理时间尾迹窗口、反向、延迟及循环/排空时钟 |

详见 [刻度](docs/SCALES.md)、[复平面](docs/COMPLEX_PLANE.md)、[极坐标](docs/POLAR_PLANE.md)、[柱状图](docs/BARCHART.md)、[网络更新](docs/NETWORK_MUTATIONS.md)、[曲线网络](docs/CURVED_NETWORKS.md)、[自适应积分](docs/ADAPTIVE_FIELDS.md) 和 [流线动画](docs/STREAMLINE_FLOW.md)。自适应绘图和积分区域属于有界数值近似，不能发现任意未采样间断点，也不提供符号积分。

### 文字、公式、数字、矩阵与代码

| 方向 | 当前能力 |
| --- | --- |
| 文字 | OpenType 真实字形轮廓、横排字距、多行、常规/粗体/斜体/粗斜体；缓存内嵌字体，最多 16 个显式有序回退字体 |
| 有限富文本 | 嵌套粗体/斜体、六位颜色、XML 转义和换行；同字体跨颜色字距 |
| 数字对象 | DecimalNumber、精确 int64 Integer；固定精度、正负号、千分组、文字单位、基线锚、原子更新与独立快照 |
| 基础公式 | 分组、分数、带指数根号、上下标、可伸缩/不可见定界符、矩阵/cases、积分/求和/连乘、命名算子与上下限 |
| 扩展公式 | dfrac/tfrac、二项式、多行 substack、aligned/gathered、有限 l/c/r array 与竖线/hline、上下堆叠标注 |
| 数学样式 | 真实 mathbb/mathcal/mathfrak 字形、集合符号、上下划线、帽号/向量/点号等装饰、boxed、phantom、作用域颜色、上下花括号及标签 |
| 分片与匹配 | 分别合法公式片段的共同基线与 advance、精确 key 选择、独立复制/快照、重复 key 匹配动画及真实目标替换 |
| 显示矩阵 | 公式/整数/小数/对象矩阵、自适应行列布局、六种括号、单元格选择及着色 |
| 表格 | 文字/对象表格、内外边框、背景和高亮、独立行列标签/角标、稳定数据索引与完整网格访问 |
| 代码展示 | Go 词法高亮、Go Mono 字体轮廓、制表与 CRLF 行网格、行号、背景、行/token 选择及独立快照 |

默认内嵌 Go 字体不覆盖中文；显示中文需要提供具有相应字形的字体。复杂文字塑形、双向排版和系统字体发现仍待实现。公式是有明确边界的 TeX 数学子集，尚不支持完整 LaTeX/Typst、宏包或任意自定义宏。上下花括号使用有限 em 排版策略，不是完整 TeX 可伸缩字形组装。

详见 [文字与字体](text/README.md)、[字体回退](docs/FONT_FALLBACK.md)、[富文本](docs/TEXT_MARKUP.md)、[数字](docs/NUMBERS.md)、[公式](mathtex/README.md)、[公式数组](docs/MATH_ARRAYS.md)、[公式花括号](docs/MATH_BRACES.md)、[矩阵/表格](docs/TABLES.md) 和 [代码展示](docs/CODE.md)。

### SVG、图片与二维渲染

| 方向 | 当前能力 |
| --- | --- |
| SVG 几何 | 基本图形、全部路径命令及椭圆弧、嵌套仿射变换、样式、端帽/连接/斜接和真实 dash-array |
| SVG 引用与文字 | 本地 defs/symbol/use、有限内嵌 CSS、data URI 图片、text/tspan 的真实轮廓及受限定位/空白/anchor |
| SVG 裁剪与渐变 | 受限 userSpaceOnUse/objectBoundingBox 裁剪；部分容器和文字 bbox；有限多多边形裁剪并集；线性/同心径向填充渐变、局部 href 模板及整段文字 bbox 渐变 |
| 图片 | PNG/JPEG/GIF 首帧解码、不可变共享资产、仿射变换、双线性采样、边缘抗锯齿、透明度与 SVG 内嵌 |
| 像素操作 | 不可变反色、RGB 替换、显式 tint、导入 alpha 恢复；不同资产可使用双层 FadeTransform |
| CPU 渲染 | 抗锯齿填充/描边、nonzero/evenodd、ZIndex、透明合成、ROI、嵌套矢量/位图裁剪、渐变、二维相机平移/缩放/旋转 |
| 画质与并行 | 1/2/4/8 倍整帧二维超采样、有界 worker、按序输出、worker 局部缓冲复用与独立输出帧 |

详见 [SVG](svg/README.md)、[图片像素操作](docs/IMAGE_PIXELS.md)、[裁剪](docs/CLIPPING.md)、[填充规则](docs/FILL_RULES.md)、[渐变](docs/GRADIENTS.md)、[容器 bbox](docs/SVG_CONTAINER_BBOX.md)、[文字渐变](docs/SVG_TEXT_GRADIENTS.md) 和 [光栅画质](docs/RASTER_QUALITY.md)。

### 三维几何、场景与相机

| 方向 | 当前能力 |
| --- | --- |
| 几何与曲面 | 校验后的索引网格；盒、球、圆柱、圆锥、圆环、参数曲面；五种柏拉图多面体、有界闭合凸多面体、任意端点圆柱/箭头、双材质棋盘曲面 |
| 三维渲染 | 模型/法线变换、六面裁剪、透视正确插值、Z-buffer、Lambert/环境光、方向光及最多 16 个带距离衰减的 RGB 点光源 |
| 材质与画质 | 不透明方向光硬阴影、逐采样透明排序、UV 纹理、nearest/bilinear、clamp/repeat、1/2/4 倍超采样 |
| 点云 | CPU 圆形点、位置/颜色/像素半径、网格混合深度与透明合成、固定点数关键帧 |
| 动画与相机 | 网格 TRS、四元数旋转、显隐、固定拓扑顶点/法线/UV 和材质颜色/alpha 关键帧；透视/正交 look-at 相机及显式四元数朝向轨道 |
| 场景接入 | 三维结果作为 RGBA 位图组合到二维 Scene；独立 scene3d 直接提供 FrameSource，支持并发乱序采样及显式二维叠加 |

详见 [三维渲染](three/README.md)、[三维时间轴](docs/SCENE3D.md)、[凸多面体](docs/POLYHEDRA.md)、[三维连接体](docs/SEGMENT3D.md) 和 [棋盘曲面](docs/CHECKERBOARD_SURFACES.md)。目前使用 CPU 渲染，完整 ThreeDScene 生命周期、多相机和点光源阴影仍待补齐。

### 音频与字幕

- 不可变 PCM；WAV PCM16/24/32 和 float32/64 输入输出；受限 IMA ADPCM、Microsoft ADPCM、G.711 A-law/μ-law WAV 解码。
- 原生 FLAC 编码及严格 PCM8/16/24/32、1–8 声道解码；AIFF/AIFC 的受支持 PCM 子集及 AU PCM/G.711 解码。
- 增益、包络、混音、有界 sinc 重采样、显式声道矩阵、拼接和重复。
- Scene/Replay 声音时间轴；MOV 的 PCM16 音轨；整数毫秒字幕轨道、纯文本 SRT/WebVTT 导入导出。

各容器和编码都有明确的位深、声道、块结构、元数据与时长策略，详见 [音频](audio/README.md)、[字幕](subtitle/README.md)、[FLAC 输入](docs/FLAC_INPUT.md) 和 [媒体管线](docs/MEDIA.md)。

## 内置示例

示例位于 `examples/scenes/`，与 CLI 共用场景构造代码。下面按用途列出代表性入口；完整列表由 `demo -h` 输出。

| 用途 | 场景名称 |
| --- | --- |
| 基础动画与路径 | `transform`、`primitives`、`paths`、`curve-edit`、`path-direction`、`transform-paths`、`succession` |
| 微积分与坐标系 | `calculus`、`lissajous`、`logarithmic`、`complex-plane`、`polar-plane`、`bar-chart` |
| 数值计算 | `spectral`、`schur`、`complex-solve`、`sparse-solve`、`sparse-product`、`bicgstab` |
| 公式与排版 | `mathematics`、`fractions`、`multiline-math`、`math-arrays`、`math-parts`、`matching-parts`、`math-braces` |
| 文字、表格与代码 | `font-fallback`、`text-markup`、`numbers`、`linear-algebra`、`table-labels`、`code-display` |
| 动态与网络 | `dynamics`、`replay`、`updater-lifecycle`、`dynamic-topology`、`networks`、`curved-networks`、`network-mutations` |
| 流线与矢量场 | `adaptive-streamlines`、`streamline-flow` |
| SVG 与图像 | `images`、`image-pixels`、`gradients`、`clipping`、`clip-rules`、`fill-rules`、`svg-container-bbox`、`svg-text-gradients` |
| 三维与材质 | `three`、`textures`、`shadows`、`point-lights`、`platonic-solids`、`segments3d`、`checkerboard-surface`、`mesh-morph` |
| 相机、点云与媒体 | `quaternion`、`oriented-camera`、`pointcloud`、`point-morph`、`scene3d-timeline`、`composition` |

## 输出格式与质量选项

| 格式 | 当前用途与范围 |
| --- | --- |
| PNG / SVG | 指定时间点的位图或矢量帧；SVG 可保留受支持的渐变、裁剪及内嵌图片 |
| GIF / frames | 流式动画 GIF、PNG 帧序列 |
| MP4 | 纯 Go H.264 视频；CLI 默认帧内压缩、QP24 |
| MOV | H.264 视频与 PCM16 音轨，支持独立视听时钟 |
| WAV / FLAC | 单独导出音轨；FLAC 输出为 PCM16/24 量化结果的无损编码 |
| SRT / VTT | 独立纯文本字幕文件，当前不内嵌或烧录到视频 |

常用参数：

| 参数 | 说明 |
| --- | --- |
| `-width` / `-height` / `-fps` | 输出尺寸与帧率；MP4/MOV 宽高必须为偶数 |
| `-workers` | 并行光栅 worker 数量；会影响工作内存 |
| `-supersampling 1/2/4/8` | 提高光栅采样质量，输出尺寸不变，同时增加 CPU 与内存成本 |
| `-time` | PNG/SVG 的采样时间，单位为秒 |
| `-codec intra` | 默认 H.264 I4x4、整数变换、量化和 CAVLC |
| `-codec predictive -gop 60` | 受限单参考 IDR/P 编码；可跳过与前一重建 YUV 完全相同的宏块 |
| `-inter-residual -motion-radius 2` | predictive 模式下启用帧间残差及半径 1–4 的整数亮度运动搜索，增加 CPU 成本 |
| `-qp` / `-mode-search` | 量化参数与有限帧内预测模式搜索；更低 QP 通常保留更多细节，QP0 仍不保证无损 |
| `-audio` | 导入受支持 WAV/FLAC/AIFF/AIFC/已知长度 AU，适用于 MOV/WAV/FLAC |
| `-audio-remix` | 显式开启单声道复制或双声道平均，不自动推断声道角色 |
| `-discard-audio-metadata` | 显式允许丢弃解码器识别但不保留的受支持元数据 |

旧库接口 `export.NewMP4` 保留 I_PCM 行为，文件较大；压缩输出使用 `NewMP4WithOptions` 与 `DefaultMP4Options`。目前没有完整 H.264 编码器的所有预测、码率控制或硬件编码能力，详见 [媒体文档](docs/MEDIA.md)。

## 架构与包结构

主要数据流为：

```text
数学计算 / 字体 / 公式 / SVG / 图片
                  ↓
        Object、Group、Scene 编排
                  ↓
        时间轴 / updater / Replay
                  ↓
     FrameSource.Sample(t) 独立快照
                  ↓
       二维 / 三维 CPU 渲染
                  ↓
       Sink → 图片 / 视频文件

音频与字幕按各自时间轴合成，并由媒体输出接口写出。
```

| 包或目录 | 职责 |
| --- | --- |
| 根包 `gomath` | 对象、几何、样式、场景、动画、相机、updater、声音与字幕时间轴 |
| `geom` / `path` | 几何运算与曲线路径引擎 |
| `linalg` / `sparse` | 稠密和稀疏数值计算 |
| `graph` / `network` / `field` | 坐标绘图、网络布局、向量场及流线 |
| `text` / `mathtex` / `number` / `code` | 字体、公式、数字和代码排版 |
| `matrix` / `table` | 可动画矩阵与表格 |
| `svg` / `rasterimage` | 矢量与像素资产导入 |
| `render` / `three` / `scene3d` | 二维光栅、三维渲染和三维动画时间轴 |
| `audio` / `subtitle` / `export` | 音频、字幕、图片及视频输出 |
| `cmd/gomath` / `cmd/purecheck` | CLI 与开发期纯 Go 构建审计 |
| `examples/scenes` | 可运行的示例场景 |
| `compat` / `docs` | 固定参考清单、范围说明与设计文档 |
| `tests` | 全部测试、benchmark、fixture、参考生成器与验证记录 |
| `artifacts` | 本地生成的演示与媒体产物 |

对象在编排完成后再进入并发采样；不要在采样期间修改场景或让回调写入共享可变状态。更详细的所有权和时钟设计见 [架构文档](docs/ARCHITECTURE.md)；新能力的具体范围以专题文档和 [PARITY](docs/PARITY.md) 为准。

## 验证与性能记录

### 正确性验证

当前验证包括数值分解重构/残差/正交性、路径独立积分参考、固定来源 Manim 控制点与动画中间状态、独立字体/Cairo 图像参考、SVG 导出与光栅集成、媒体完整解码、快照隔离及竞态检查。

2026 年 11 月 5 日第十批冻结检查点记录了：

- `CGO_ENABLED=0`、`noasm` 整仓集中测试通过。
- 完整 race、vet、纯 Go 构建输入检查通过。
- Linux/Windows amd64 整仓交叉构建通过。
- 同一源码/二进制的分片匹配、路径反向、公式花括号 SVG/光栅集成通过。

原始记录见 [第十批检查点](tests/data/integration/wave10-checkpoint-20261105/checkpoint.json)。集成示例共享 Go 场景构造，其对照范围不等于独立验证全部 Manim 几何；通过项目图像误差阈值也不表示像素完全相同。

### 已有性能证据

| 测量项目 | 已记录结果 | 适用范围 |
| --- | --- | --- |
| 静态 100 圆对照 | GoMath 602.8 帧/秒，Manim Cairo 247.5 帧/秒，约 **2.44 倍吞吐** | 2026-11-03 历史检查点；640×360、单 worker、5×300 帧中位数；排除构造/编码/I/O，像素不完全相同 |
| 重复逆弧长查询 | 55.402 → 1.840 μs，两者均零分配 | Go 内部算法前后对照；排除索引构造，不是与 Manim 的比较 |
| FLAC 解码 | 指定旧 PCM16/24 双声道工作负载中位耗时分别下降 **44.98% / 69.58%** | CRC16 查表优化，交错五样本，分配次数保持；不是所有音频的保证 |
| 混合裁剪帧 | 两个工作负载分配字节下降 **56.97% / 60.00%** | worker 局部 scratch 复用；部分其他计时工作负载有回退，且增加缓冲保留寿命 |

这些记录使用 Apple M2、macOS arm64、Go 1.27.3、CGO0/noasm；具体 GOMAXPROCS、采样方法和排除项见各专题。共享主机、批次顺序和后台负载会影响计时。静态圆对照的 RGB MAE 为 0.165930/255、最大通道误差 11、PSNR 48.974 dB，未测 Python 内存或双方峰值 RSS。

裁剪 scratch 的保留像素载荷最高可达每 worker 96 MiB，降低累计分配不代表等量降低进程驻留内存。上述结果不能合并成“所有场景更快”或“整体更省内存”的结论。

完整原始数据与复现条件见 [性能记录](docs/PERFORMANCE.md)、[路径查询](docs/PATH.md)、[FLAC 优化](docs/FLAC_PERFORMANCE.md) 和 [光栅分配优化](docs/RASTER_PERFORMANCE.md)。

## 当前边界与后续方向

当前各能力族仍为部分实现或尚未开始。重要缺口包括：

- 完整 TeX/Typst、宏包、数学间距和 MATH 可伸缩字形组装；复杂文字塑形、双向排版及系统字体发现。
- 自动公式/形状语义匹配、一般拓扑对齐、完整 updater/对象生命周期，以及更多动画组合与专用对象。
- 曲线布尔、完整 SVG/CSS/viewport/滤镜、软蒙版、组透明度隔离、渐变描边与混合模式。
- 完整 ThreeDScene、多相机、投影切换、曲面周期接缝和极点处理、点光源及透明物体阴影。
- 更完整的视频预测、更多压缩音频、透明视频及字幕样式/内嵌/烧录。
- 完整配置、持久缓存、热重载、scene sections；GPU、实时预览及 Go 原生 notebook/插件工作流尚未开始。
- Linux/Windows 实机验收，以及更广泛、统一条件下的兼容与性能测量。

作为更广泛数学基础库的扩展目标，稀疏直接分解、张量代数和更多科学计算算法也仍待实现；这些应与 Manim 行为对等验收分别追踪。

固定参考清单包含 177 个 Python 模块与 2,498 个公开定义，但逐符号条目仍为 `unverified`，常量、重导出和动态 API 还需额外审计。它是来源清单，**既不代表这些功能全部缺失，也不能用来计算完成百分比**。详细计划见 [兼容性清单](docs/PARITY.md) 与 [开发状态](docs/STATUS.md)。

## 开发与贡献

### 测试集中管理

所有测试源码、benchmark、fixture、参考脚本和验证记录均放在 `tests/`，不在生产包旁新增 `_test.go` 或 `testdata/`。

```text
tests/
├── _cases/<package>/   # 包测试；根包使用 root/
├── data/<package>/     # 固定输入、图像、参考数据和验证记录
├── reference/          # 仅开发期使用的独立参考生成器
├── cmd/run/            # 包测试与 benchmark 的统一入口
├── internal/harness/   # Go source overlay 测试工具
└── suite_test.go       # 整仓集中测试入口
```

```sh
# 完整测试与静态检查
CGO_ENABLED=0 go test -tags noasm ./...
CGO_ENABLED=0 go vet -tags noasm ./...
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck

# 指定包或测试；根包名为 root，-pkg 可以重复
CGO_ENABLED=0 go run -tags noasm ./tests/cmd/run \
  -pkg=root -pkg=render -count=1

# 包级 benchmark
CGO_ENABLED=0 go run -tags noasm ./tests/cmd/run \
  -pkg=path -run '^$' -bench . -benchmem

# 场景级采样与光栅基准、来源清单
CGO_ENABLED=0 go run -tags noasm ./cmd/gomath benchmark \
  -scene lissajous -frames 300 -width 640 -height 360
CGO_ENABLED=0 go run -tags noasm ./cmd/gomath parity

# 开发期 race 检查需要 Go 竞态工具链的 cgo 支持
CGO_ENABLED=1 go test -race -tags noasm ./...
```

`make build`、`make check`、`make bench` 提供对应入口。按测试名称筛选时使用集中 runner；直接对生产包执行 `go test -run ...` 不会注入集中测试。详见 [测试说明](tests/README.md)。

开发期允许 Python、Cairo、FFmpeg 等工具生成独立参考或验证媒体，它们不进入产品运行链路，普通 Go 测试也不要求安装这些参考工具。race 的 cgo 要求仅属于开发工具链。

新增能力时，应同时明确支持范围和错误边界、保持对象身份及快照隔离、更新 `docs/PARITY.md`，并为兼容性声明提供固定来源及动画中间帧证据。热点修改需记录相关工作负载、画质、耗时和内存；不以空 API 或最终帧相似代替实现。

## 来源与许可

迁移参考为 **Manim Community 0.22.1**，固定提交：

```text
16ea443e2a5f1d79db0a721161411ce916d6e733
```

项目代码采用 [MIT License](LICENSE)。派生实现保留 Manim 和 3Blue1Brown 的相应版权与许可声明。Go 模块、内嵌 Go 字体及 STIX 派生数学字形分别遵循各自许可证；完整来源和分发要求见 [第三方声明](THIRD_PARTY_NOTICES.md) 与 `LICENSES/`。
