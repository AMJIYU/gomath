# 原生二维裁剪

`gomath.NewClip(path.Path)` 保持原来的 nonzero-winding 默认语义。
`gomath.NewClipWithRule(outline, path.EvenOdd)` 显式选择奇偶裁剪；也接受
`path.NonZero`。其他 rule 值明确报错。`Clip.FillRule()` 返回不可变规则；
没有可变 setter，改规则需构造新 Clip。nil 的 getter 返回 NonZero，但 nil
仍表示不加裁剪，不能等同于空 Clip。

两个构造函数都复制输入，`Path()` 返回副本，快照和 `Object.Copy` 共享只读
Clip。轮廓隐式闭合；nonzero 使用整体绕数，反向内轮廓可形成孔洞；evenodd
按整个 compound path 的穿越次数奇偶判定，不依赖方向。重复的同一轮廓
出现两次在 evenodd 中相消，出现三次保留一份。已构造的空路径隐藏全部
内容，`Clip` 零值无效。这次仅扩展 Clip，不给普通对象的 Style 添加填充规则。

```go
outline, err := gomath.Annulus(.5, 1.5)
if err != nil { return err }
clip, err := gomath.NewClip(outline.Path)
if err != nil { return err }
picture.SetClip(clip)
```

Clip 使用世界坐标，只安装到当前节点；所有后代受它约束，子节点自己的
Clip 与祖先相交。`SetClip(nil)` 只移除当前节点的 Clip。矢量的填充、描边、
虚线和位图像素走同一裁剪合成路径。原有 `ZIndex` 排序仍按叶对象进行，
Clip 不改变身份、布局边界或生成几何布尔交集。

`Object.ApplyMatrix/Shift/Scale/Rotate` 同步变换子树中各自的 Clip，保留
原共享 Clip 不变。只移动子节点时，父级 Clip 保持在父级的世界位置。
显式 `Clip.Transform` 接受有限仿射矩阵并返回新的 Clip；无效或溢出会报错。
相机投影复制并变换 Clip，不修改作者对象或其他采样帧。

动画会对两个非 nil、同 fill rule 的 Clip 进行轮廓/曲线对齐并逐控制点
插值，适用于同一位图资产的移动蒙版。rule 随变换、对齐、插值、Copy、
Scene/Replay 快照保留；不在中间帧离散切换。NonZero/EvenOdd 不匹配的
Transform、ReplacementTransform、TransformFromCopy、子树 morph 和
Succession 在准备阶段原子拒绝，不修改作者对象、身份或时间轴；独立
FadeTransform 图层可以使用不同规则。相同 Clip 指针直接共享；不同 Clip 的中间结果独立。
旋转走实际旋转路径，Succession 保留逐步状态和原子失败语义。一般 Transform
不接受 nil/non-nil Clip 切换，使用两层 `FadeTransform` 表达两种不同裁剪。
有限缓动 overshoot 导致非有限裁剪坐标时，采样返回错误。

光栅器用各自规则的独立 coverage mask 相交，先绘制叶对象到透明局部
图层，再按乘积 mask 合成。覆盖率为 8-bit、逐级乘积取整；这不是所有
子像素的精确集合交面积，没有新增组隔离透明度、alpha/luminance mask、
软边或 filter。保持原有逐级对象 opacity 语义和 nonzero 的矢量光栅分支，
旧输出与旧参考阈值不变。evenodd 不改变普通填充的 signed-area AA 局限。

SVG 导出为 `userSpaceOnUse` 的 `<clipPath>` 定义，定义中的 path 显式写入
`clip-rule="nonzero"` 或 `clip-rule="evenodd"`。排序后的每个叶对象包裹完整
祖先 Clip，位图仍内嵌 PNG；不会因排序丢掉祖先裁剪。默认 nonzero 的导出
字符串不变。

SVG 导入仍只接受一个 basic/path child 或直接指向单形状的 use；该 compound
shape 支持继承的 `clip-rule`、presentation attribute、已支持的 type/class/id
CSS 层叠、inline style 及 inherit/initial/unset。clipPath 定义的规则取自定义
及其 light-tree 祖先，引用它的绘制对象不能覆盖定义的规则。clipPath 内的
use 将其 computed rule 传给实例目标；目标本身的声明可覆盖，CSS 选择器
按既有 use shadow 上下文重新匹配。共享源形状的缓存包含规则，两个 use
不能互相污染规则。空/隐藏形状也保留其 computed rule。

此切片不扩展多图形 union、objectBoundingBox、clipping group/text、filter、
外部引用或 clone-local 定义；未知、循环及超限引用继续报错。`clip-rule`
不等于普通填充的 `fill-rule`，导入器仍明确拒绝 `fill-rule:evenodd`。
`!important` 等原 CSS 边界保持不变；其他 SVG 支持边界不因新增裁剪规则扩大。

每个 Clip 最多 4,096 个轮廓、65,536 条 cubic；每个可绘制节点最多 64 层
相交裁剪。形状对齐预检最多 16M 单位的最长曲线扫描/细分工作，拒绝极不
匹配的曲线数量。采样无隐藏低质量或忽略裁剪的回退。

光栅每个局部图层最多 16M 像素；RGBA 图层和两张 alpha mask 共最多
96 MiB payload，另有输出帧、矢量光栅器缓冲、路径副本及分配器开销。
每帧最多 64M 次 mask 像素处理、1M 个展平点（同一祖先作用多个叶对象
分别计费），展平最多 18 层，误差使用配置的像素 `CurveTolerance`。
超深仍达不到误差或投影坐标超出 ±1e9 像素时明确报错。限额不表示总峰值
内存或执行时间保证，多个 worker/并发 Render 会各自占用工作存储。

集中测试覆盖作者/导出路径隔离、平移/旋转/相机、动画中间帧与 Succession、
并发 Replay、空路径/隐式闭合/孔洞、嵌套相交/兄弟隔离、描边及位图局部
图层偏移、ZIndex、错误原子性和资源边界。

`tests/reference/clipping.py` 使用独立定义的几何与 CairoSVG 生成两个
192×144 参考。RGBA 全图误差分别为：嵌套矢量 MAE 0.030979/255、最大34；
半透明位图内部 MAE 0.006149/255、最大5。位图边缘位于 Clip 外，明确排除
不同源图边缘延伸滤波的影响。版本、源码/图片 hash 在
`tests/data/render/clip-provenance.json`。Go 测试只读取数据。

`tests/reference/clip_export.py` 另外验证实际 `clipping` 示例在 t=2 的
640×360 PNG 和经 CairoSVG 解释的导出 SVG：MAE 0.048409/255、最大30，
任一通道误差大于8的像素占0.1163%。记录在
`artifacts/clipping-export-20261104/provenance.json`。这检查导出语义；独立
场景输入来自前一组 fixtures。开发参考工具不参与运行时或普通测试。

Go1.27.3、Apple M2、darwin/arm64、cgo0/noasm、GOMAXPROCS=1，640×360
固定帧的三次 300ms 样本中位：单矢量圆形裁剪约0.555ms/帧、1.173MB/
22次分配；四层约0.868ms/帧、1.166MB/34次分配；单层位图约4.366ms/
帧、1.215MB/17次分配。包含验证、输出帧、临时图层、mask、光栅和合成，
不含对象构造、采样、相机、编码或 I/O。不同层的交集缩小了 ROI，故四层
分配可以略低；它不代表裁剪更省内存。并行开发期间测得一条明显基线争用
离群值，不作隔离性能或 Manim 优势结论。原始数据在
`tests/data/render/clip-benchmark-20261104.txt`。


## evenodd 的解析扫描带与新增预算

光栅只在显式 EvenOdd 时进入新分支。曲线使用同一有界 de Casteljau 展平，
遵守 `CurveTolerance`（默认工作像素 0.02）、18 层深度、每帧 1M 展平点及
±1e9 投影坐标限制。每条非水平边按 Y 递增规范化，隐式闭合所有轮廓。
水平边没有面积贡献；重复、反向、共线和接触边不通过 epsilon 删除。

按顶点 Y、边的内部交点 Y 和整数像素行切分扫描带；每带的边次序固定，
排序后两两配对得到奇偶区域。左右边与像素列相交时，对截断线性函数做
分段梯形/三角形积分，累加像素面积并四舍五入成 8-bit alpha。因此没有
固定扫描线采样漏掉细带的机制：1/32 像素高的白色矩形保留 alpha 8，
一像素 bowtie 的半像素区域得到 alpha 128。自交、多轮廓洞、两份重复轮廓
相消、混合规则祖先交集都走同一路径。实际 fill/stroke/bitmap/gradient 仍
由原先图层绘制；SSAA 在工作分辨率生成该 mask，再统一 resolve。

这是**展平多边形的 float64 面积积分**，不是精确 Bezier 布尔，也不宣称
有理数拓扑保证。只合并数值完全相等的事件；在 ±1e9 坐标内仍有浮点舍入。
非有限/越界、奇数边界、面积超出允许的数值误差均报错，不改用 nonzero、
不输出部分图像。Go 测试另用 path.Boolean 的有理数 arrangement 独立计算
选定线段图形与每个像素格的交面积，检验积分实现；普通渲染不调用 Boolean。

- 单个 evenodd mask 最多 `MaxEvenOddClipEvents = 2<<20` 个 Y 事件，计入
  端点、交点和像素行边界；在分配该存储前预检基本事件数。
- 既有每帧 `MaxClipRasterWork = 64<<20` 同时约束 mask 像素处理与新增
  边收集、Y 重叠候选对检查、事件插入、活动边扫描、排序 charge 和覆盖
  像素积分。排序 charge 定义为 n*bits.Len(n)；它是明确工作单位预算，
  不等同于 CPU 指令数。所有祖先/叶对象共同计费。
- 边数不超过既有 1M 点预算；显式增长容量上限为 1M。额外几何数组的
  最大有效容量载荷约 72.125 MiB：32MiB 边、16MiB 事件、8MiB 活动索引、
  16MiB 排序交点、最多128KiB 单行面积。这之外仍有 Path 副本、图层、mask、
  矢量 scratch、分配增长/未 GC 内存及分配器开销，不是 RSS 上限。
- 所有这些限制在实际 SSAA 工作分辨率执行。某帧可能满足基础图层/SSAA
  像素预算，却因大量交点、长活动边集合或更细展平达到扫描预算而报错。
  不对恶意相交边集合进行无限二次扫描；遇限额没有降质/漏裁剪回退。

普通对象显式 EvenOdd fills 现也复用此积分器，见 [FILL_RULES.md](FILL_RULES.md)。
所有 clip 与普通 parity fill 共用每帧64M工作/1M展平点；fill ROI逐像素、边/
事件/排序/积分，以及这些对象的有界描边展平均计费，同一路径的fill/stroke
两次展平分别计点。2M事件限制仍是每个mask的存储上限，所有事件另消耗共享
逐帧工作。普通fill复用gradient alpha scratch，不增加SSAA常驻像素估算；
它的几何/描边scratch及并发worker容量边界另列在FILL_RULES文档中。

## evenodd 的独立参考与验证

`tests/reference/clip_evenodd.py` 直接定义几何，使用 Cairo 1.18.4 的
FILL_RULE_EVEN_ODD/FILL_RULE_WINDING 生成 128×96 透明 PNG，以及 4×渲染后
预乘 RGBA BOX resolve 的参考。八组包括同向洞、重复、仿射反射曲线、自交、
细带、混合祖先规则位图、alpha 渐变、CSS/use 上下文。测试分别通过原生
Clip API 和 SVG importer 构造图像；普通测试不依赖 Cairo/Python。

**CairoSVG 2.7.1 不能作本轮 evenodd clip 真值。** 本机 source 的
`surface.py` 注释掉了 clip-rule 处理；生成器显式验证 compound hole 被
错误填为 alpha204，并记录源码 hash。因此参考明确标为直接 Cairo，不
修改/补丁 CairoSVG 后冒称独立原版结果。旧 nonzero CairoSVG 参考仍有效。
Python3.11.5、Pillow11.2.1、源码/输入/PNG hash 和这一诊断在
`tests/data/render/clip-evenodd/provenance.json`。

| 128×96 参考场景 | 1× MAE / RMS / max | 4× MAE / RMS / max |
| --- | --- | --- |
| 同向 compound hole | .018656 / .136588 / 1 | .129211 / .359460 / 1 |
| 重复轮廓 | 0 / 0 / 0 | 0 / 0 / 0 |
| 仿射曲线 | .102844 / .814863 / 21 | .030640 / .227861 / 5 |
| 自交与重叠轮廓 | .125916 / .846002 / 17 | .017334 / .142064 / 3 |
| 1/32 像素细带 | 0 / 0 / 0 | 0 / 0 / 0 |
| 嵌套规则/alpha 位图 | .038757 / .312858 / 10 | .165283 / .407750 / 2 |
| alpha 渐变 | .084066 / .290081 / 2 | .144450 / .416097 / 2 |
| 同一目标的 CSS/use 规则 | 0 / 0 / 0 | 0 / 0 / 0 |

数据按预乘 RGBA 全部四通道计算，范围0..255；PNG 的直通道存储往返可能
引入1级量化差，因此 MAE 不必随倍率单调减小。新增参考上限为 MAE≤.4、
RMS≤2、max≤32；没有改任何旧像素 fixture 或旧测试阈值。

`tests/reference/_clip_evenodd_export/main.go` 生成实际 Go SVG 与 PNG；
`tests/reference/clip_evenodd_export.py` 用独立的严格小范围 XML/Cairo reader
解释实际 `<clipPath>` 规则和 M/L/C/Z solid 路径。只支持这个导出 fixture
所需的 svg/g/defs/clipPath/path/rect 与仿射变换，其他几何/paint 报错，
不是新的运行时 SVG 引擎。原生直 premultiplied 输出对独立 Cairo 参考
MAE .051412、RMS .454761、最大20。数据与两个工具的 hash 在
`tests/data/export/clip-evenodd/provenance.json`。SVG roundtrip 测试将已有
导出背景的100%尺寸等价改成绝对尺寸，以适配现有 importer 的 primitive
百分比限制；clip defs 和实际绘制路径不修改。

本轮中央测试另覆盖默认 nonzero 像素/导出字节不变、规则所有权、变换、
对齐/中间帧、错误 morph 原子性、Succession/Replay、相机、2/4/8× SSAA、
并行有序帧及扫描/事件/坐标/构造预算。相机投影必须连同规则复制 Clip，
不能用默认 nonzero 的零值重建；中央回归逐帧检查规则与实际像素。

## evenodd benchmark

2026-11-04，Go1.27.3、Apple M2、darwin/arm64，CGO0/noasm、GOMAXPROCS=1、
一个 render worker、640×480固定输出、FrameWidth128、SSAA1。每 op 完整
绘制一帧，包含校验/透明背景/图层/mask/展平/奇偶积分/合成/输出分配；
不含输入构造、一次 scratch warm-up、动画采样、相机、编码或 I/O。

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=render -run '^$' \
  -bench '^BenchmarkEvenOddClip$' -benchmem -benchtime=300ms \
  -count=3 -p=1 -cpu=1 -tags noasm
```

| 场景 | 三次 ms/frame | 中位 ms/frame | B/op | allocs/op |
| --- | --- | --- | --- | --- |
| compound 直线洞 | 4.194622 / 3.974328 / 3.991410 | 3.991410 | 2,508,848 | 27 |
| 仿射曲线洞 | 3.356764 / 3.452428 / 3.682105 | 3.452428 | 约2,185,464 | 34 |
| 自交与重叠 | 3.676453 / 3.568771 / 3.988410 | 3.676453 | 2,524,609 | 30 |
| 嵌套位图 | 18.110070 / 17.715581 / 17.850785 | 17.850785 | 2,416,503 | 33 |

这是四个固定 workload 的测量，不是整体性能或 Manim 加速比声明。

## 本轮完成验证

2026-11-04，相机投影保留 Clip rule 后，中央 runner 对 root/render/export/svg
四个包运行完整测试，未使用 skip 或排除项，所有新增相机回归均包含在内。

| 包 | CGO0 / noasm / count=1 | race / count=1 |
| --- | --- | --- |
| root | 0.050s | 1.262s |
| render | 1.175s | 14.749s |
| export | 0.385s | 2.381s |
| svg | 0.896s | 11.743s |

命令为 `CGO_ENABLED=0 go run ./tests/cmd/run -pkg=<包> -tags noasm -count=1`
及 `CGO_ENABLED=1 go run ./tests/cmd/run -pkg=<包> -race -count=1`。
四包生产代码 `CGO_ENABLED=0 go vet -tags noasm . ./render ./export ./svg`
及各包中央测试 `-tags noasm -vet=all -run '^$'` 均通过。
`CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck` 检查42个非标准库包通过：
无 native 输入或运行时进程启动依赖。本记录为此切片的完整定向验证；
全仓集成检查由主线程执行。

## 集成规则示例

`clip-rules` 用相同方向的内外圆轮廓比较nonzero实心与evenodd孔洞，并同时
动画改变内轮廓和相机。640×360 SSAA4画面已检查，中央示例回归在五个时刻
检查投影后的规则与中心实际像素；evenodd中心保持背景，nonzero中心保持填充。
这项集成没有把缺少evenodd支持的CairoSVG当作参考；独立真值仍为上述直接Cairo
及有理数面积fixtures。
