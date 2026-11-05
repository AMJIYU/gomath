# 普通对象填充规则

普通二维矢量对象可以显式选择奇偶填充。`Style.FillRule` 是
`path.FillRule` 值，零值 `path.NonZero` 保留原有非零绕数语义、Style 字面量、
默认渲染像素和 SVG 输出；JSON 省略零值字段。只接受 `path.NonZero` 和
`path.EvenOdd`，其他枚举由 `Style.Validate`、对象/场景验证、渲染及导出报错。

```go
outline := gomath.Circle(2).Path
outline.Contours = append(outline.Contours, gomath.Circle(.8).Path.Contours...)
ring := gomath.NewObject(outline).
    SetFill(gomath.Blue).
    SetStroke(gomath.White, .03).
    SetFillRule(path.EvenOdd)
```

两条圆形轮廓方向相同：NonZero 填满中心，EvenOdd 留孔。奇偶规则按整个
compound path 判定，不依赖方向；两份完全重复的轮廓相消，三份等价于一份。
开放轮廓的填充隐式闭合，空路径没有填充面积。规则不会执行几何布尔运算，
不会改变 Path、Object ID 或 Bounds。

`Object.SetFillRule(rule)` 与 SetFill 一样递归设置当前节点及后代，返回原
Object 以便链式 authoring。它不立即验证参数；无效值留在对象中，后续验证
明确失败，不静默替换规则。直接写 `o.Style.FillRule` 只影响该节点；普通
原生 Group 不做 SVG 式动态样式继承。规则是值，Copy、Scene/Replay 快照、
ApplyMatrix 和 camera 复制时无需共享可变存储。authoring 完成后可并发采样；
禁止采样过程中修改作者对象的既有契约不变。

同一对象的 solid Fill 与 FillGradient 都使用选择的内部覆盖率；渐变仍按
工作像素中心采样，保留既有 stop alpha、对象及祖先 opacity 和合成语义。
`Reveal`/Create/Write 先取既有的 cubic 参数顺序前缀，再对该实际路径应用
填充规则；没有引入按长度 reveal 或特殊“动画孔洞”定义。描边中心线使用
实际前缀；**stroke outlines 始终走原 nonzero 描边光栅器**，不因两份相同
轮廓的 evenodd fill 相消而漏掉描边。虚线、cap/join 仍使用原语义。

FillRule 不影响位图采样；bitmap 上可存储/复制规则值，但它不改变图片像素。
FillRule 与每个不可变 Clip 的 FillRule 独立：先画对象填充及描边，再与祖先
裁剪 coverage mask 相交。支持混合 NonZero/EvenOdd 的祖先 Clip、gradient、
bitmap 兄弟对象、camera 及 1/2/4/8× SSAA。没有新增 group isolation、alpha
mask、filter、objectBoundingBox clips 或多 shape clip union。

## 动画与原子错误

Transform、ReplacementTransform、TransformFromCopy 和嵌套子对象对应节点
必须具有相同 FillRule，包括没有 Path 的对应组节点。准备阶段预检规则，
alignment 再保留防御检查；不同规则不能在普通 morph 的中间帧离散切换。
相同规则通过插值保留；新增/移除的透明占位子树使用其对应节点自己的规则。
Succession 中后续规则不匹配会使整个 Play 原子失败，作者对象、已有子指针、
身份及时间轴保持不变。显式 `FadeTransform` 使用两层独立绘制，因此允许
跨规则过渡。无效规则不通过 crossfade 绕过验证。

## 光栅算法与共享预算

NonZero 仍走原 vector 后端，不进行全局算法切换，也不隐藏其 signed-area AA
在自交/partial glyph 上的既有局限。EvenOdd 复用原生 clip 的有界扫描带面积
积分器：在顶点 Y、边内部交点及整数像素行处切分，活动交点排序后奇偶配对，
解析积分线性边与像素列的交面积。1/32 像素细带可保留 alpha8；不是固定
scanline centers 或隐式 SSAA fallback。曲线按 `CurveTolerance` 展平，默认
0.02 工作像素，最多18层；这是展平多边形的 float64 积分，不宣称精确 Bézier
面积或有理数拓扑保证。数值不一致明确报错。

原来的公开 clip 常量也约束 ordinary parity fills，无第二套无限预算：

| 资源 | 边界与计费 |
| --- | --- |
| 每个 coverage mask 的 ROI | `MaxClipRasterPixels = 16<<20` 像素，分配前检查 |
| 每帧共享工作 | `MaxClipRasterWork = 64<<20`，包括所有裁剪 mask 像素处理、parity fill ROI 像素、边收集、候选边对、事件插入、活动边扫描、排序 charge 及覆盖积分 |
| 每帧共享展平点 | `MaxClipRasterPoints = 1<<20`，所有祖先 clip、parity fills 和这些对象的描边展平共同消耗；同一路径的 fill/stroke 两次展平分别计费 |
| 每个 parity mask 的 Y 事件 | `MaxEvenOddClipEvents = 2<<20`，不是独立的逐帧累计事件配额；所有事件同时消耗上面的共享逐帧工作 |
| 投影坐标 | 可见 ROI 路径展平控制点限 ±1e9 工作像素；非有限数值报错 |

排序 charge 是 `n*bits.Len(n)` 工作单位，不等同 CPU 指令。源路径 bounds/
Validate/Partial、原 nonzero 绘制与既有 stroke tessellation 并不伪称由这一个
扫描单位预算约束；描边仍受自己的既有分段/虚线限制。每次 Render 重置共享
逐帧计数器，失败后可继续渲染下一帧。超限返回 nil image 与错误；序列输出
通过原有 Sink.Abort 契约处理，不交付部分帧，不漏画、不降质。

Parity fills 复用 gradient coverage 的一个 alpha scratch，单 worker 最多
16 MiB 像素 payload；不新增第二张常驻 alpha 图，不改变现有 SSAA 的28字节/
工作像素估算及 worker/queue 配额。几何存储仍另计：积分器的边/事件/活动边/
交点/单行面积最大有效容量约72.125 MiB；新增有界描边 polyline 存储最多
约48 MiB 点容量和32 MiB轮廓 slice容量。它们不是同时可达峰值的精确估算；
共享点预算会进一步限制组合，但此处给出独立保守上界。所有显式容量增长
受上限限制；源 Path/Partial 副本、旧 slice 尚未 GC、stroke scratch、分配器、
资产和 sink 输出不在这些 payload 数字内，不是进程 RSS 保证。
每个并发 rasterizer/worker 各自使用 scratch 和预算，不共享可变掩码。

## SVG

导入 `fill-rule="nonzero|evenodd"`、inline style 和已有 CSS 的 type/class/id、
selector list、后代/子代层叠。specificity、source order、presentation attributes
与 inline 优先级沿用现有 parser；inherit/unset/initial 通过同一继承实现。
use 从 host 继承规则，目标自身声明可覆盖，定义原来的祖先不泄漏进 shadow
实例；相同目标可在不同 use 中采用不同规则，拥有独立几何和 ID。
实际 text/tspan glyph 也通过同一个 styledShape 赋值，不覆盖局部 tspan 规则。
未知规则、!important 和其他原不支持的 CSS 语义继续报错。

导出仅在显式 EvenOdd 时给普通 path 添加 `fill-rule="evenodd"`，solid 和
gradient paint servers 相同；默认 NonZero 不新增属性，保持旧 SVG 字节格式。
Clip defs 的 `clip-rule` 不受普通 fill 影响。导出渐变的精度格式
`rgb(r%,g%,b%)` 现可由 importer 精确读取：仅三项逗号分隔百分比，要求有限且
0..100，不接受混合数值/百分比单位；原有0..255数值 RGB仍可用。未扩展其他
CSS color functions。普通 primitive 的百分比几何限制保持，roundtrip 测试
仅将导出背景100%矩形尺寸等价改成绝对宽高，不改路径、gradient 或 clip defs。

## 独立参考

`tests/reference/fill_evenodd.py` 独立定义顶点/cubic controls，用直接 Cairo
生成12组128×96透明参考及4×预乘 RGBA BOX resolve。包括同向洞、两/三份
重复、仿射曲线、自交、细带、linear/radial stop alpha、独立 de Casteljau
partial prefix、混合祖先裁剪、虚线和 CSS/use。Go 测试分别通过原生API与
SVG importer 构建；普通测试只读 PNG/JSON，运行时无 Python/Cairo依赖。
最大全图 MAE .131592、RMS .883941、单通道差23；新参考边界为 MAE≤.4、
RMS≤2、max≤32。没有改变任何旧 fixture 或旧阈值。

`tests/reference/_fill_evenodd_export/main.go` 生成真实 Go SVG 和 PNG，
`tests/reference/fill_evenodd_export.py` 使用未修改的 CairoSVG 解释该实际
SVG，包含普通 EvenOdd solid/linear gradient、partial cubic 与 nonzero 祖先
clip。CairoSVG 普通 fill-rule 可用，工具验证了同向矩形洞；**不使用它作为
evenodd clip-rule 真值**。当前原始 Go premultiplied 输出对参考：1× MAE
.069702、RMS .441527、max14；4× MAE .047017、RMS .239314、max3。
PNG 直/预乘转换后的离线脚本统计略有量化差，另行写入 provenance，不混报。

Python3.11.5、Cairo1.18.4、CairoSVG2.7.1、Pillow11.2.1，工具/输入/输出hash
位于 `tests/data/render/fill-evenodd/provenance.json` 与
`tests/data/export/fill-evenodd/provenance.json`。未修改 Python 参考项目，
不作 Manim 全面填充兼容声明。中央测试另检验所有权、原子 morph、Replay、
相机、位图不受影响、渐变变化 alpha、描边不相消、共享 scratch/预算、并发
worker、奇数输出尺寸、失败后的恢复及默认 NonZero 回归。

## 性能测量

2026-11-04，Go1.27.3、Apple M2、darwin/arm64、CGO0/noasm、GOMAXPROCS1、
Workers1，320×240固定输出、FrameWidth128。每个 workload/倍率运行三次
300ms串行样本。包含验证、完整光栅/合成、输出分配和SSAA resolve；不含输入
构造、一次scratch warm-up、Scene采样、camera、编码和I/O。共享开发机器
测量，不作隔离硬件或整体速度优势声明。

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -run '^$' \
  -bench '^BenchmarkOrdinaryEvenOddFill$' -benchmem -benchtime=300ms \
  -count=3 -p=1 -cpu=1 -tags noasm
```

| 完整帧 workload | SSAA | 中位 ms/frame | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| 同向 compound 洞，solid alpha .8 | 1 / 2 / 4 | .583828 / 3.759877 / 12.666250 | 316144 / 1548976 / 5242929 | 12 / 14 / 14 |
| 两圆 .72 cubic 参数前缀 | 1 / 2 / 4 | .652662 / 3.658879 / 11.647376 | 361504 / 1600096 / 5334112 | 25 / 27 / 28 |
| 两圆洞 linear gradient，alpha .7 | 1 / 2 / 4 | 1.842722 / 8.333683 / 29.943180 | 362909 / 1636706 / 5335777 | 16 / 19 / 19 |

三次原始样本保存在 `tests/data/render/fill-evenodd-benchmark-20261104.txt`。

## 完成验证

2026-11-04，完整中央定向测试，无 skip/排除项：

| 包 | CGO0/noasm/count=1 | race/count=1 |
| --- | --- | --- |
| root | 0.077s | 1.360s |
| render | 2.108s | 20.492s |
| svg | 1.156s | 12.009s |
| export | 0.449s | 2.603s |

使用 `CGO_ENABLED=0 go run ./tests/cmd/run -pkg=<包> -tags noasm -count=1`
及 `CGO_ENABLED=1 go run ./tests/cmd/run -pkg=<包> -race -count=1`。
`CGO_ENABLED=0 go vet -tags noasm . ./render ./svg ./export` 和四包中央
runner的 `-tags noasm -vet=all -run '^$'` 均通过。
`CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck` 检查42个非标准包通过，
无native输入或运行时进程启动依赖。全仓 checkpoint 由父线程统一执行。

## Integrated fourth-wave example — 2026-11-04

父线程fill-rules例子直接用普通渐变Fill（无Clip）比较同向双圆的两种规则，内圆形变和Camera2D经过五个时刻中心像素检查。640×360/SSAA4实际SVG的独立CairoSVG解释最大差18/16/19，RGBA MAE .092292–.098427，记录artifacts/fill-rules-wave4-svg-20261104/provenance.json；这只验证共享Go场景构造的导出/光栅一致性。
