# SVG objectBoundingBox 裁剪导入

`svg.Read` 支持 `clipPathUnits="objectBoundingBox"` 的有界子集。没有新增
公开 Go API 或依赖；结果仍为原生不可变 `gomath.Clip`，`NonZero/EvenOdd`
规则、快照、camera、ApplyMatrix、现有同规则 morph 和 SVG 导出继续复用。
省略 clipPathUnits 仍为 userSpaceOnUse，既有测试、默认像素及参考阈值不变。
不修改原生 root/render/export 的坐标、填充或抗锯齿算法。

```xml
<svg viewBox="0 0 100 100">
  <defs>
    <clipPath id="left" clipPathUnits="objectBoundingBox">
      <rect width=".5" height="1"/>
    </clipPath>
  </defs>
  <rect x="10" y="20" width="60" height="40" fill="blue"
        transform="rotate(20 40 40)" clip-path="url(#left)"/>
</svg>
```

上例先在目标自身的坐标系中保留 x=10..40、y=20..60，再旋转整体。
粗描边不扩大 bbox，已有 clip、opacity、display/visibility 不缩小几何 bbox。
世界空间中的轴对齐 Bounds 不参与此映射。

## 几何和变换

依据 W3C SVG 2 Coordinates 的 [Bounding boxes](https://www.w3.org/TR/SVG2/coords.html#BoundingBoxes)
与 [Object bounding box units](https://www.w3.org/TR/SVG2/coords.html#ObjectBoundingBoxUnits)：
目标几何 bbox 与目标 user coordinate system 对齐，排除 stroke 和 clipping。
若局部 bbox 是 `(minX,minY,width,height)`，最终裁剪轮廓使用列向量顺序：

```text
SVG-to-world × ancestors × targetTransform
  × Translate(minX,minY) × Scale(width,height)
  × clipPathTransform × childTransform [× clip-use x/y × sourceTransform]
```

clipPath 定义祖先的 transform 不参与映射。定义 CSS 从其 light tree 继承，
不会从被裁剪目标继承；clip 内 use 按既有 shadow-tree 规则重新匹配选择器，
目标自己的 clip-rule 可覆盖 use 继承的值。`clip-path` 不默认继承，显式
`inherit` 会在当前节点自己的坐标系重新求值。source 缓存按源元素、计算后
规则和 bbox/user-space 模式隔离。目标 bbox 不写入共享定义计划。

支持的被裁剪目标：

- `path/rect/circle/ellipse/line/polygon/polyline`：原始局部 Path 的几何
  bounds。复合轮廓、洞及开放路径均不按可见填充裁掉 bbox。cubic 极值使用
  SVG bbox 专属的 float64 导数差值归一化和稳定二次根，不受通用 Path.Bounds
  固定 epsilon 对极小局部曲线的退化判定影响；直线保留端点 bbox。SVG 弧与圆仍使用原有 cubic 表示/近似，
  不宣称数学精确的 conic bbox。
- `image`：x/y/width/height 的**定位 viewport**，在 preserveAspectRatio
  meet 对齐/收缩之前取值；支持已有 auto/内在尺寸和目标 image 百分比尺寸。
  这与 clip 定义内部只支持无单位数字的限制不同。
- 直接引用上述一个目标的普通 `use`：在 use 自己的局部坐标系求 bbox，
  包含 use x/y 和目标自身 transform，排除 use 自身/祖先 transform、目标
  原始祖先和目标原有裁剪。对于变换后的曲线，先变换真实曲线再求极值，
  不把源 AABB 四角当作曲线 bbox；image 则变换其定位 viewport 的四角。

clip 定义仍至多一个 basic/path 子形状，或直接引用其中一个形状的 use。
nonzero 与 evenodd 均支持。clip 子形状可引用另一个 bbox clip，映射使用该
子形状**自身 transform 前**的局部几何 bbox，然后通过独立 Clip 链表达交集。
clipPath 容器本身或 clip 内 use host 再引用 bbox clip 暂不支持，明确报错。
这些位置引用 userSpaceOnUse clip 的旧能力不变。

若 bbox 宽或高为零，SVG 2 要求**忽略该 bbox 效果**。包括水平/垂直 line
和只有 moveto 的 path；粗描边不会把退化几何变成非退化 bbox。空路径同样
不安装 bbox clip。即使定义是空 clipPath，也不能把退化目标的粗描边隐藏。
被忽略的效果不取消其他祖先/子节点的裁剪。定义、引用、循环和属性仍严格
检查；这不是绕过错误的路径。非有限 bbox、宽高差溢出或映射坐标溢出返回错误。

## 明确边界

bbox clip 定义中的几何长度及内部 use x/y/width/height 只接受有限无单位
数字，含科学计数法。`.5` 表示 bbox 对应轴的一半；负坐标和大于1的坐标
允许，几何宽高/半径仍按现有规则非负。`50%`、px/in 等长度单位、auto
明确报错，不能把不同属性的百分比 viewport 规则擅自替换成 `.5`。path d、
points 和 transform 继续使用现有有限数字语法。

普通 group/text/tspan/svg/symbol，以及引用 group/text/symbol 或 use 链的
use host 不支持 bbox clip，导入原子失败。它们可继续使用 userSpaceOnUse。
不新增多 graphical child union、container bbox 推断、mask、filter、根/symbol
viewport clipping、image slice 或 clone-local clipPath 解析。所有原有 unknown/
external/cycle/namespace/CSS 限制保留，不能通过缺失引用静默漏图。

导入后 bbox 已物化，不在每帧随 Reveal 或 morph 重新求 bbox。普通 morph
插值已物化的几何和 clip，这是原生 Clip 动画语义；不是重新执行 SVG DOM
布局动画。不同 Clip rule 仍由原生普通 morph 预检原子拒绝。导出为原生世界
轮廓对应的 userSpaceOnUse 定义，写入原规则；不会重新输出 bbox 依赖。

原生渲染仍逐级乘 8-bit coverage，多个重合抗锯齿边界不等同连续几何交集
的精确面积；SSAA 是显式选项，默认 nonzero signed-area AA 局限不改变。
完整 exporter 输出含装饰属性及百分比背景，不承诺可被此有界 importer
原样全部重新导入；本切片验证导出的裁剪定义、几何和规则。

## 预算、所有权和并发

复用每次 Read 的同一 clip 预算，**没有新增不计费的 target bbox cache**：

| 资源 | 上限/计费 |
| --- | --- |
| 定义 | 1,024 个 clipPath，包括未使用定义 |
| clipping source 属性 | 每个至多 1 MiB；bbox use host 额外解析的 basic/path 目标属性也受此限制 |
| 原生 compound clip | 每条至多 4,096 contours / 65,536 cubic curves |
| 累计 clip 路径物化/额外 bbox 几何扫描 | 8,192 次 |
| 累计处理曲线/轮廓 | 1,048,576 curves / 65,536 contours |
| 交集、引用深度、输出 Clip 祖先 | 各至多64 |

源 clip、计划变换、实例变换照常计费。basic/path bbox 目标另外对源几何计
一次扫描；direct use host 的附加解析及变换后几何各计一次。image 固定四角
viewport 不计额外路径扫描，实际 clip 仍计实例。空/零曲线和退化 basic/path
目标也计扫描，不能用零面积绕过限额。clipping 子形状引用 bbox clip 时，其
源轮廓副本与额外 bbox 扫描同样计一次。每次额外相交组继续计入100,000
展开对象预算。输入32 MiB、XML/use 深度128、CSS 和图片解码预算不变。

这些是阶段工作/存储限额，不是进程 RSS 保证。源 Path、不可变 Clip 副本、
解析器、图片资产和 GC 暂留另计。没有新增全局可变状态；每次 Read 自有
compiler/cache/budget，输出共享的 Clip 与图片资产不可变，Path() 返回副本，
变换实例不会影响同源实例或快照。失败返回 nil object/error，不发布部分结果。
渲染端继续执行自己的逐帧点/事件/像素、SSAA 和 worker 内存预算。

## 独立验证

`tests/reference/svg_bbox_clips.py` 手工构造直接 Cairo 路径、矩阵和 winding/
parity clipping，不读取生产几何或 native 渲染结果。数据保存在
`tests/data/svg/bbox-clips/`，provenance 记录版本、primary spec、脚本和全部
SVG/PNG 的 SHA256。7组128×96场景分别生成1×及4× premultiplied BOX：
仿射旋转/剪切/反射/非等比、use host/source 裁剪、嵌套 bbox、gradient、
image meet、CSS use 两规则及退化线。bitmap oracle 使用与内嵌 PNG 相同的
常量 RGBA，clip 严格位于图片内部，排除无关的边缘 filter 差异；nested
fixture 避免多个 mask 的重合 AA 边界，另有 native 交集/退化/错误回归。
CairoSVG 2.7.1 不作为 evenodd clip oracle，也不能可靠解释本组复杂 nested
bbox/use/degenerate 场景；直接 Cairo 显式设置 fill rule 与几何。

中央测试覆盖局部 cubic 极值（含1e-100..1e100源坐标及逆尺度变换）、矩阵顺序、stroke 排除、image viewport/auto/
percentage、use shadow/显式 inherit、缓存隔离、洞、零宽高、未知/循环引用、
溢出、共享预算接受/拒绝边界、nil partial-output、快照所有权、并发导入、
camera、同规则 morph 和实际 SVG 导出裁剪定义。旧测试与阈值保留。

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=svg -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./svg
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -vet=all -run '^$'
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=svg -tags noasm -run '^$' \
  -bench BenchmarkSVGBBoxClipImportRender -benchmem -p=1 -cpu=1 -count=3
```

benchmark 包含64个direct-use bbox实例、完整 XML/CSS/definition/geometry导入及
一帧128×96光栅，分别1×/4×；rasterizer 建立在计时外，PNG编码/磁盘和Cairo
参考生成不在计时内。纯 Go runtime，不安装 native 或调用外部进程 fallback。

本次定向结果（2026-11-04，Go1.27.3，darwin/arm64，Apple M2）：SVG整包
CGO0/noasm 1.067s，race 13.343s，生产 vet、中央测试 vet=all、purecheck42
通过。7组直接Cairo参考共14次对比，MAE最高0.182292、单通道max45；4×时
max12。已有参考及阈值均未修改。源坐标1e-100至1e100的cubic极值、大/小
逆尺度、direct use和嵌套bbox回归通过；camera逆刚体变换允许至多1级浮点/
coverage舍入，导出轮廓bounds误差限1e-7世界单位。

| workload | 三次 ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| 64 use，128×96，SS1 | 894602 / 840197 / 788406 | 828824–828825 | 5309 |
| 64 use，128×96，SS4 | 2288717 / 2194596 / 2206675 | 2037786 | 5311 |

只记录本工作负载的成本，不宣称相对旧版本加速；共享主机负载可能影响时间。
结构化验证记录与修改文件清单见 `tests/data/svg/bbox-clips/validation.json`。
