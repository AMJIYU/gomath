# SVG 多子形状 clipPath：有界多边形并集

本功能扩展 `svg.Read`，没有新增公共 API。生产实现只依赖既有纯 Go
`path.Boolean` 和不可变 `gomath.Clip`，运行时不加载 native 库、不启动进程。
这不是任意 SVG clipping/精确 Bézier Boolean 支持。

## 语义与接受范围

CSS Masking 的 [clipping paths](https://www.w3.org/TR/css-masking-1/#ClippingPaths)
与 [clipPath 定义](https://www.w3.org/TR/css-masking-1/#elementdef-clippath)
规定：不同直接子图形的剪裁区域取并集；每个图形内部由自己的 `clip-rule`
解释轮廓。因此不能把所有 children 的轮廓拼接成一个 evenodd/nonzero path。
相反方向的重叠 children 仍然可见，同一个 evenodd child 的重复轮廓则相消。

新增分支接受 **2–16 个 graphical children**，其中每个只能是：

- `rect`，`rx`/`ry` 均为零或未指定，尺寸严格正且能表示非退化区域；
- `polygon`，闭合、至少三条非零边；
- `path`，只使用 `M/m L/l H/h V/v Z/z`，每个 contour 显式闭合且非退化；
- `use`，直接引用上述一种形状，支持 `href`/`xlink:href`、x/y 和 transform。

允许多轮廓、洞、自交、方向相反的轮廓、相同区域、部分重叠、共线重叠、边接触、
点接触；具体输入仍须通过精确算术、舍入拓扑和资源限制。零长度边、全共线
contour、空/move-only/open path、零尺寸 rect、重复相邻点会明确报错；不会
自动去重、删边、补闭合或返回伪造的空成功结果。有效的轮廓相消/全部 hidden
则可以得到真正的空 Clip。

每个 child 独立经过现有 CSS cascade、定义树继承、`clip-rule` 和仿射变换。
`use` 的目标在自己的 shadow tree 中重匹配 selector、从 use 继承，不从源祖先
继承。fill、stroke、opacity 不改变剪裁 silhouette；display/visibility 沿用既有
SVG importer 的规则，目标显式 visibility 可覆盖 use 的继承值。hidden 几何
也先严格验证，不借隐藏绕过格式或预算检查。clipPath 自身的变换参与计算，
其源祖先变换不参与。`use` width/height 验证为非负绝对长度，不缩放普通形状。

先对每个 child 按自己的 rule 做 `BooleanUnion(child, empty)` 规范化，
再对规范区域逐次做真正 `BooleanUnion`。最终生成 **一个 NonZero 原生 Clip**，
洞的方向由 Boolean 输出保证；不会用 child 的 alpha max 或逐 child Clip 交集
冒充并集。输入语法已经证明边是直线，仿射只变换端点后构造 canonical Line
controls，避免独立舍入 controls 造成假曲线。Boolean 不展平曲线，也无 epsilon
snap；精确有理交点转回 float64 时不能保持拓扑的输入会报错。

## 单子形状、坐标与嵌套边界

0/1 graphical child 继续经过原编译分支，保留既有圆/曲线/rounded rect、
nonzero/evenodd、userSpaceOnUse/objectBoundingBox、嵌套单形状交集的输出和限额。
新分支不会把已有曲线偷偷展平成多边形；既有 fixture/阈值不修改。

新增 multi 分支同时支持 userSpaceOnUse 和 objectBoundingBox，复用已有
[bbox 坐标约定](SVG_BBOX_CLIPS.md)：取目标未裁剪、排除描边的局部几何 bbox，
随后应用 bbox 映射、clipPath 变换、child/use 变换以及目标仿射。旋转、shear、
reflection、非均匀缩放不改成世界 AABB。image 使用 meet 前的定位 viewport；
basic/path/image 以及 direct use 目标沿用原范围。按
[SVG 2 Object bounding box units](https://www.w3.org/TR/SVG2/coords.html#ObjectBoundingBoxUnits)，
目标 bbox 任一尺寸为零时忽略该裁剪效果；整个定义仍先验证。bbox clipping
几何只接受有限 unitless 数（小数不是百分号）；百分比和带单位长度明确拒绝。
源计划在单次导入内复用，目标 bbox 在实例化时映射，不跨目标缓存世界坐标。

新 multi 定义不允许自己或任一 child/use target 上带 clip-path，也不允许被另一个
clipPath 的 clip-path 引用。缓存命中和定义声明顺序都不能绕过此限制。
普通可见图形的父 clip 与 child multi clip 仍由原生祖先链相交，允许 group 的旧
single clip 包住应用 multi clip 的 drawable。应用 multi clip 的图形可以再包含
普通可见子图形，沿用既有 user-space/目标类型限制。

multi 中明确拒绝 circle/ellipse/rounded rect、C/Q/A/S/T 曲线命令（即使 controls
恰好共线）、line/polyline/open path、group/text/image/symbol silhouette、use 链、
child 内部 graphical children 和上述 nested clip 分支。clone-local 定义、未知属性、
外部或缺失引用、重复 id、循环引用继续明确报错。多子形状 union 不扩展 group bbox、
filter/mask、文本 shaping、混合规则 morph 或组 opacity 隔离。

并集在导入时物化；camera、快照、普通同规则 morph 与 export SVG 都复用原生
不可变 Clip。动画中间帧插值已物化边界，不在每帧重新计算子形状的集合并集。
拓扑不同的目标仍受既有 path alignment/interpolation 契约约束，不能把边界 morph
解释成逐帧 Boolean 动画。导出为一个 `userSpaceOnUse`/nonzero path 定义。

## 共享预算与失败行为

所有限制同时生效；满足边数并不保证不会先耗尽 work/算术位数/分配预算。
每个 `Read` 一个 compiler/ledger；children、use 和不同定义不能各自重置额度。
所有 source 先解析、验证并计费，再执行该定义的任何 Boolean 操作。

| 限额 | 值 | 计费范围 |
| --- | ---: | --- |
| graphical children | 16 | 一个 multi 定义 |
| clipping graphic attribute bytes | 65,536 | 每个 child/use/target 属性，解析前检查 |
| source leaf occurrences | 128 | 全部 multi 定义，use 重复引用也计次 |
| source line segments | 512 | 全部 multi 定义，包含隐式闭合边 |
| source contours | 128 | 全部 multi 定义 |
| Boolean calls | 32 | 全部 multi 定义；N children 预留可用性检查为 2N−1 次 |
| operand segments/contours | 256 / 128 | 每次两个操作数合计；输出也不得超过此数 |

`path.Boolean` 没有向调用者返回实际消耗统计，因此每次调用**保守扣除整份最大
许可预算**，成功后不返还未用额度：

| Boolean 字段 | 每次上限 | 32 次累计预留上限 |
| --- | ---: | ---: |
| Work | 250,000 | 8,000,000 |
| Pairs | 32,768 | 1,048,576 |
| Vertices / Edges | 1,024 / 1,024 | 32,768 / 32,768 |
| SplitPoints | 2,048 | 65,536 |
| AllocationBytes | 16 MiB | 512 MiB |
| RationalBits | 1,024 | 每个持久有理数的分子/分母各自上限，不按次数相加 |

Contours/Segments 等字段全部显式传正值，不使用零值选取更大的 backend 默认。
16 children 消耗31次；两个这样的定义即使很简单也超出共享32次预算。
`AllocationBytes` 是保守累计算法分配计费，不是常驻内存/RSS 上限或实测 B/op。

现有整个 Read 的 clipping 预算仍共享：1,024 definitions、8,192 path materializations、
1,048,576 curves、65,536 contours。新分支为每个源几何和变换后的几何各计一次，
每次 Boolean 结果、最终 Clip 构造、目标实例变换也计费。每条直线在原生 path 中
是一条 cubic，因此同时消耗 curve 额度。新旧分支不会分别享有完整旧额度。
原生每 Clip4096 contours/65536 curves、祖先最多64仍有效。XML/CSS/展开和
bitmap/gradient 等已有预算也不变。解析临时空间还受32MiB输入与64KiB属性限额
约束；不声称输入尚未解析时就知道所有 segment 数。

任一格式/语义/数值/配额错误，`Read` 返回 `nil,error`；不发布部分对象、不自动
放宽规则、不 silently flatten、不调用参考程序。只读资产、source plan 与各实例路径
的所有权沿用既有契约；并发的不同 Read 不共享可变 ledger。

## 验证与复现

- 中央测试：`tests/_cases/svg/clip_union_test.go`、`clip_union_private_test.go`。
  同时运行全部既有 SVG 测试，保留旧单子形状像素 fixture。
- 数据：`tests/data/svg/clip-unions/`；开发参考：`tests/reference/svg_clip_unions.py`。
  11 个直接 Cairo 场景、1x/4x 共22张 PNG；另有.35/.7两个线性动画中间帧，
  以及3个独立面积/离边界 membership 几何用例。
- 独立 oracle：Skia pathops0.9.2逐 child 解释规则并做集合 UNION，然后将输出直线
  边界交给 direct Cairo1.18.4。Python3.11.5、Pillow11.2.1。不读取 Go 输出；
  不使用 CairoSVG 的 evenodd clip 作为 oracle。Skia float32 只用于这些中等尺度
  场景，不能当作任意 dyadic 数值的精确判据。脚本/数据 SHA256、primary spec
  和局限记录于 `provenance.json`；Go 测试不需要安装或启动这些开发依赖。
- 实测22图 premultiplied RGBA：MAE0–0.20844/255、RMS0–0.81190、最大通道差17。
  新静态参考上限 MAE0.4、RMS2、max40；两个SS4动画中间帧实测MAE0.01733/0.02209、RMS0.13181/0.18377、max2（上限MAE0.15/RMS1/max24）；原有参考阈值未动。覆盖反向重叠、mixed holes、
  自交、接触、child affine、CSS/use、bbox仿射、渐变、bitmap meet、祖先交集与退化bbox。
- 数值/错误/预算/并发另有测试：one native Clip、规则规范化、缓存与声明顺序、
  independent IDs/snapshots、atomic nil errors、hidden 验证、共享调用/源边/contour/curve
  额度、16/17 children、不可表示和极小输入、camera、导出和中间 morph。

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=svg -tags noasm -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./svg
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -vet=all -run '^$'
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -run '^$' \
  -bench BenchmarkSVGClipUnionImportRender -benchmem -benchtime=150ms -p=1 -cpu=1 -count=3
```

基准工作量：每次导入2个child、一个evenodd洞与一个nonzero重叠矩形，实际执行
3次Boolean，然后透明背景128×96、单worker渲染1帧，分别SS1/SS4。计入XML/CSS解析、
精确求交/分类/舍入校验、原生Clip构造和渲染；排除文件读取、参考生成和rasterizer构造。
共享主机只观察这项受限工作量，不作全库性能领先或加速声明。新的验证独立记录，
不把第五批 checkpoint 当作第六批实现通过依据。

2026-11-05，Go1.27.3 / darwin-arm64 / macOS15.7.1 / Apple M2，
`CGO_ENABLED=0`、noasm、`-p=1 -cpu=1`、每样本150ms，3次观测：

| 工作量 | ns/op（三次） | B/op | allocs/op |
| --- | --- | ---: | ---: |
| 2 children，128×96，SS1 | 728292 / 734101 / 763122 | 901101–901114 | 22567–22568 |
| 2 children，128×96，SS4 | 3292588 / 3215123 / 3210844 | 2462874–2463256 | 22569–22570 |

原始输出在 `tests/data/svg/clip-unions/benchmark.txt`；定向验证及本切片源码
SHA256 在同目录 `validation.json`。它不是全仓 checkpoint，也不继承第五批通过状态。
