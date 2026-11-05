# SVG 导入扩展

## linearGradient / radialGradient 填充 paintserver

`svg.Read` 的公开入口和 `Options` 不变。`fill="url(#id)"`（presentation、
CSS 或 inline）现在物化为不可变 `gomath.Gradient`，沿用原生真实逐像素
渐变填充。未修改 root/render/export，也不需要新增核心接口。

```xml
<svg viewBox="0 0 128 96">
  <defs>
    <linearGradient id="base" gradientUnits="userSpaceOnUse"
                    x1="16" y1="0" x2="112" y2="0">
      <stop stop-color="red" stop-opacity=".3"/>
      <stop offset="100%" stop-color="blue"/>
    </linearGradient>
    <linearGradient id="paint" href="#base"
                    gradientTransform="rotate(8 64 48)"/>
  </defs>
  <rect x="8" y="8" width="112" height="80"
        fill="url(#paint)" fill-opacity=".7"/>
</svg>
```

坐标与绘制范围：

- `linearGradient` 默认 `(x1,y1)=(0%,0%)`、`(x2,y2)=(100%,0%)`；
  `radialGradient` 默认 `cx=cy=r=50%`、`fx=cx`、`fy=cy`、`fr=0`。
  径向只接受同心模型，允许显式写相同焦点；不把偏心焦点静默改成圆心。
- `gradientUnits` 默认 `objectBoundingBox`，普通 basic/path 形状使用
  **元素变换前、排除 stroke 的真实曲线几何 bounds**。fraction/percentage
  先进入单位 bbox；矩阵顺序为 element × bbox × gradientTransform ×
  gradient frame，支持非等比、旋转、反射、剪切及径向椭圆。bbox 任一
  维度为零时不产生填充，原有实色 stroke 保留。bbox 坐标不接受 px 等单位。
- `userSpaceOnUse` 使用当前 drawable 的用户坐标；百分比 x/y 依当前
  viewport 的宽/高，r/fr 使用归一化对角线 `hypot(width,height)/sqrt(2)`。
  symbol 的百分比依其 viewBox 用户尺寸重新求值。支持既有绝对长度单位。
- `gradientTransform` 仅来自本定义或 href 模板；defs/gradient 祖先的
  普通 transform 不参与渐变坐标。形状、use 的 x/y、symbol、世界宽度
  归一化、ApplyMatrix 和 camera 同时移动几何与渐变。可见 stroke 仍保留
  已有非等比变换拒绝规则。
- group/use 上继承的是填充引用，直到实际 drawable 才求值。定义自身
  不绘制，模板和重复 use 都不会共享可变对象 ID、几何或 gradient frame。
  bitmap 继续忽略向量 fill；资产、实际图片和 opacity 语义不变。
- text/tspan 使用现有真实 glyph outlines，支持 `userSpaceOnUse` 及有界
  `objectBoundingBox` 填充。后者使用整个 enclosing text 的完整 glyph cells
  （advance + ascent/descent，含空格），所有 chunk 完成定位/anchor 后才绘制；
  tspan fill 也共享整个 text 的 bbox，单独的 tspan clip bbox 仍按既有规则。
  已支持的 use 链按各实例继承字体/paint 并独立测量；自身/祖先变换只在
  bbox 后应用。原有文字定位/字符/字体及嵌套 viewport/container-symbol
  bbox 拒绝边界不变。见 `docs/SVG_TEXT_GRADIENTS.md`。

stop、CSS、模板与透明度：

- stop offset 接受有限 number/percentage，默认 0，钳制到 `[0,1]`；
  后一 offset 小于前一时提升到前一值。重复 offset 保留真实硬切换。
  首尾不足 0/1 时复制端点色实现 pad。无 stop 的填充透明；单 stop、
  零长度 linear 或零半径 radial 是最后一 stop 的实色；负 r 报错。
- 只接受 `spreadMethod=pad`（或省略）及 `color-interpolation=sRGB`。
  RGB/alpha 分别按编码 sRGB 数值线性插值，不声称线性光空间插值。
  `transparent`/既有带 alpha hex 颜色参与实际 alpha 插值。
- stop-color 默认 black，stop-opacity 默认 1，二者**不隐式继承**；
  显式 `inherit` 可以继承。`color` 正常继承，`currentColor` 在 stop
  自己的级联/继承环境解析。定义树的 stop 复用既有 specificity、source
  order、presentation/inline、initial/unset/inherit 解析器。
- href/xlink:href 只接受同类型本地模板；href 优先。未指定的坐标、units、
  transform、spread 属性逐链继承；自己的 stop 列表整体替换模板 stops，
  不是拼接。没有自己 stops 时复用源 stop 元素的属性/class/inline，并在
  引用梯度的 shadow 继承环境重新求 CSS：单 stop 的 type/class/id 规则
  可匹配，后代/子代选择器不能穿过该边界，继承值来自引用梯度。模板
  元素自己的 CSS 属性不会被当成 href 几何属性复制。模板链不产生额外
  的中间 stop 祖先。此边界与已有 use shadow 级联一致，不模拟全浏览器。
- 实际 stop alpha = 颜色 alpha × stop-opacity × drawable fill-opacity；
  drawable/group opacity 保留在对象树，避免重复乘入 stops。stop/gradient
  上的普通 opacity 不是 stop-opacity，且不作为独立可绘制图形合成。
  使用现有逐 drawable 的相乘 opacity，不新增隔离 group compositing。

明确报错：stroke paintserver、repeat/reflect、linearRGB/auto 插值、偏心
fx/fy、非零 fr、异类型 href、external/missing/wrong-type/cyclic 引用、
URL fallback、CSS URL escapes/percent encoding、带渐变文档的 xml:base、
未知 gradient/stop 属性和子元素、奇异/病态/超出原生范围的矩阵。
现有 CSS 不支持的 !important/at-rule 等仍报错。可见 use 克隆子树内部
的 gradient 定义或模板依赖暂不支持实例局部解析；将共享定义置于克隆
子树外。空/透明/退化渐变也校验坐标、模板、spread 和 transform。

每次 Read 的新增预算：1,024 个定义；8,192 个源 stops；单定义含补齐
端点最多 256 stops；模板链最多 64；所有已解析计划累计 65,536 stops；
输出最多 8,192 个 gradient 填充求值（空/实色退化也计数）、1,048,576
个实例 stop。gradient/stop 的每个源属性最多 4,096 bytes（包括 inline
style），避免长属性随实例反复解析造成无界工作。坐标和仿射/逆矩阵受
原生 ±1e9/条件数限额约束；既有 XML/input/CSS/text/clip/image 预算继续
有效。它们是阶段预算，不是进程峰值内存承诺。包括未使用定义在内都会
校验；未匹配/被覆盖 CSS 的渐变引用仍检查目标。失败返回 nil 和明确错误。

中央测试为 `tests/_cases/svg/gradient*_test.go`，独立参考生成器为
`tests/reference/svg_gradients.py`；五组 128×96 CairoSVG source/PNG 及
SHA256、版本、误差和运行记录在 `tests/data/svg/gradient-import-*`。
覆盖硬 stop/洞、bbox 仿射、同心径向、CSS/currentColor/alpha、模板、
use、嵌套 clip 和 bitmap。text/字体轮廓沿用此前独立验证，并新增渐变
坐标/透明度断言；没有新增复杂 shaping 或 SVG 全规范兼容声明。
CairoSVG 2.7.1 重复使用 href 派生渐变会修改内部定义并漏掉后续绘制；
该参考只绘制一次派生 paint，并用 base paint 验证重复 use；Go 的重复
模板和 shadow CSS 另用显式几何/颜色断言，不把 CairoSVG 缺陷当规范。

## 本地 clipPath 裁剪

`svg.Read` 可将 presentation 属性、内嵌 CSS 和 inline style 中的
`clip-path="url(#id)"` 转为现有不可变 `gomath.Clip`。入口、Options 和
字体 API 不变；没有增加 root/render/export API，也没有用修改路径轮廓
或删除对象来冒充裁剪。例如：

```xml
<svg viewBox="0 0 100 100">
  <defs>
    <clipPath id="window" clipPathUnits="userSpaceOnUse"
              transform="translate(10 10)">
      <path d="M0 0H80V80H0Z M25 25V55H55V25Z"/>
    </clipPath>
  </defs>
  <rect width="100" height="100" fill="blue" clip-path="url(#window)"/>
</svg>
```

支持范围：

- `clipPathUnits` 省略为 `userSpaceOnUse`，也支持下述有界 `objectBoundingBox`。clipPath 内至多一个
  graphical child：`path/rect/circle/ellipse/line/polygon/polyline`，或
  **直接**指向其中一个形状的 `use`。compound path 可有多个 contour，
  按 nonzero winding 或 evenodd 表达洞；开放轮廓按填充语义隐式闭合。空 clipPath
  是合法空裁剪，会隐藏内容。title/desc/metadata/defs/style 不参与轮廓。
- 使用现有 shape/path/transform 解析器。clipPath、子形状、内部 use 及
  被引用形状的矩阵依次相乘；允许非等比、反射、旋转和剪切。形状坐标
  与内部 use 的 x/y/width/height 在 userSpaceOnUse 下接受用户单位和现有绝对长度，拒绝百分比。objectBoundingBox 下只接受有限无单位数字，拒绝百分比和 px/in 等单位。
  内部 use 兼容 href/xlink:href，href 优先。它的 width/height 不修改形状。
- 定义不绘制。clipPath 的 CSS 匹配与继承来自定义所在的原始文档树，
  不来自使用该裁剪的图形；定义祖先的 transform 和祖先裁剪不参与其几何。
  clip 内部 use 保持既有 shadow 样式隔离，被引用形状继承 use 的值。
  clipRule 是可继承属性，支持 `clip-rule=nonzero/evenodd`；缓存按实际规则隔离。
  presentation、inline 与已支持的 CSS 选择器参与层叠；use 的目标自身可覆盖继承规则。
- clipping silhouette 不受 fill/stroke、opacity、fill-opacity 或虚线影响。
  graphical child 的 display:none/visibility:hidden 产生空轮廓；visibility
  可以由内部 use 的目标恢复为 visible。clipPath 自身 display 不阻止引用。
- clip-path 本身**不隐式继承**；`none/initial/unset` 取消本节点裁剪，
  `inherit` 显式复制父节点的引用，并在子节点自己的坐标系重新求值。
  CSS specificity/source order、presentation 与 inline 优先级复用既有实现。
- userSpaceOnUse 的 shape、group、text/tspan、image、use 和 symbol 均可应用裁剪。use 自身
  clip 在 transform 之后、x/y 的附加平移之前求值；目标内的 clip 随 x/y
  移动。symbol 内部 clip 使用 viewBox 映射后的坐标。最终世界宽度归一化、
  ApplyMatrix 和 camera 同时移动几何与 clip；布局 bounds 不因裁剪改变。
- 父子图形上的 clip 自动相交。clipPath 或其唯一子形状还可通过 clip-path
  引用另一个 clipPath；导入器以原生父子 Clip 链表达交集，不拼接轮廓。
  同一次 Read 可共享不可变定义计划；返回实例的路径、节点 ID 和变换独立，
  Clip.Path() 的副本可修改，不能影响原资产或其他实例。

objectBoundingBox 的目标限 basic/path/image，或直接引用它们的 use：先取目标
**未裁剪、排除描边的局部几何 bbox**，再依次应用 bbox 映射、clipPath/子形状
变换和目标的祖先/自身变换；不会用旋转/剪切后的世界 AABB。image 使用
x/y/width/height 的定位 viewport，取 preserveAspectRatio meet 收缩前的矩形。
use host 的 bbox 包含 x/y 和目标自身变换，忽略目标原始祖先及目标上的裁剪。
source cubic 使用局部导数差值归一化的 float64 极值，避免极小曲线随后放大时被固定 epsilon 判成零高；保留既有弧转 cubic 近似。
宽或高为零（包括只带水平/垂直线但有粗描边的目标）时按 SVG 2 忽略这个
bbox 裁剪效果，不能用空 Clip 隐藏描边；其他引用/属性仍先严格验证。

clip 内 basic/path 子形状可嵌套引用 bbox clip；clipPath 容器本身、clip 内
use host，以及普通 group/text/tspan/svg/symbol 或 use 链上的 bbox clip
明确拒绝。userSpaceOnUse 的这些既有能力保留。两个坐标模式和计算后的
clip-rule 均参与源缓存隔离，目标专属 bbox 只在实例化时映射。
完整坐标顺序、预算和参考记录见 [SVG_BBOX_CLIPS](../docs/SVG_BBOX_CLIPS.md)。

多 graphical child 新增一个严格的闭合直线子集：2–16 个 sharp rect、polygon、
只含 M/L/H/V/Z 的闭合 path 或直接引用它们的 use。逐 child 按计算后的 clip-rule、
CSS 和变换解释区域，再用纯 Go `path.Boolean` 做真实 union，物化为一个 nonzero Clip；
不会把不同 children 的轮廓拼接或用 alpha max 冒充并集。支持 userSpaceOnUse 和
objectBoundingBox，沿用上述目标局部 bbox 映射。所有定义共享32次 Boolean 调用
预留、512源边/128源轮廓和既有 clip 曲线/轮廓计费；每次 operands 最多256边/128轮廓。

新 multi 分支明确拒绝 curves/circle/ellipse/roundedrect、open/空/退化边、
g/text/image/symbol silhouette、use 链以及定义/child/use target 的 nested clip-path，
即使 hidden 也严格验证。旧0/1-child含曲线、两种规则和嵌套能力保留原分支。
普通可见祖先的旧 clip 与子图形的 multi clip 可正常相交。
完整算法、保守预算、动画/缓存边界和独立参考见 [SVG_CLIP_UNIONS](../docs/SVG_CLIP_UNIONS.md)。

继续明确拒绝：clip 内的 g/text/image/symbol、内部 use 链或非 basic/path 目标、未知属性/namespace、
CSS basic-shape/geometry-box、URL fallback、外部 URL/文件、URI percent
encoding/CSS escapes，以及带 clipPath 的文档中的 xml:base。
`url(#id)` 可用单/双引号和外围 CSS 空白，ID 不接受空白或 URL 转义。

**use 克隆子树内部的 clipPath 定义暂不支持实例局部解析**：当克隆内容
引用这种定义时明确报错。将共享定义放在被克隆子树外即可使用。原始
文档图形可正常引用子树内的定义，外部 use host 也可引用原定义。
未新增 symbol/root overflow 视口裁剪、image slice、mask 或 filter；它们
仍按此前的边界明确报错。原生逐 drawable 的相乘 opacity 与抗锯齿语义不变，
不声称与浏览器逐像素相同。

每次 Read 的新增预算：最多 1,024 个 clipPath；clipping graphical element
的每个源属性最多 1 MiB（解析几何前检查）；每个原生 compound clip 最多
4,096 contours / 65,536 cubic curves；定义源轮廓、计划变换及输出实例累计
最多 8,192 次路径物化/额外 bbox 几何扫描、1,048,576 curves、65,536 contours（最终统一坐标
归一化不重复计数）。空/零曲线轮廓同样计数，不能绕过几何预算。basic/path bbox 目标计一次额外扫描；direct use bbox 另解析并变换目标几何，两次计费；退化 bbox 仍计扫描。image 的固定四角 viewport 不另计路径扫描，实际 clip 实例照常计费；clipping 子形状嵌套 bbox 引用的源副本/扫描另计一次。
clip 引用深度、单计划交集和输出祖先 clip 均最多 64。额外相交组计入原有
100,000 展开对象预算；既有 XML/use/CSS/input 限额继续有效。这些是阶段
预算，不是进程峰值堆内存承诺；渲染器还有独立的像素和展平限额。

包括未使用定义在内，都会编译/校验 clipPath；missing/wrong-type 引用也
检查未匹配或被覆盖的 CSS 声明。非法引用、循环、不支持的语义和超限使
整个 Read 返回错误和 nil，不返回消失图形的部分对象树。

测试在 `tests/_cases/svg/clip*_test.go`；独立生成器为
`tests/reference/svg_clipping.py`，五组 128×96 CairoSVG 场景及来源记录为
`tests/data/svg/clip-import-*`，包括仿射变换、洞、嵌套图形 clip、bitmap、
CSS、内部 use 和 use host 坐标。嵌套 clipPath 引用另用解析几何/像素断言
校验。provenance 记录 CairoSVG 的已知参考限制、哈希、版本与验证结果；
开发 Python/Cairo 不参与运行时或 Go 测试执行。

## text/tspan 的真实字体轮廓

`<text>` 和嵌套 `<tspan>` 通过已有 `gomath/text.Layout` 排布并生成可编辑
glyph 路径。基线使用 SVG 的向下 Y 坐标，再与普通 SVG 几何一起变换、
归一化；没有位图文字替代、系统字体查询或外部字体服务。

默认字族明确为 `Go`，对应 `gomath/text` 内嵌的四套真实 regular、bold、
italic、bold-italic 字体。`font-weight` 接受 `normal/400/bold/700`，
`font-style` 接受 `normal/italic`。不会用加粗描边或剪切冒充字体。
`font-family` 接受一个名称，可用单/双引号包围；不支持 fallback 列表。
默认解析器只接受 `Go`（大小写不敏感），`serif/sans-serif` 等名字不会
偷偷映射到 Go 字体。自带字体可通过新选项提供：

```go
font, err := text.Parse(ttfBytes) // Parse 复制字体数据
if err != nil {
    return err
}
object, err := svg.Read(reader, svg.Options{
    Width: 4,
    FontResolver: func(family string, style text.FontStyle) (*text.Font, error) {
        if family == "My Font" && style == text.Regular {
            return font, nil
        }
        return nil, fmt.Errorf("unsupported font %q style %d", family, style)
    },
})
```

非 nil resolver 处理全部请求，包括默认 `Go`。返回错误、nil 或未初始化
字体会使导入失败，不回退。每次 Read 按 family/style 缓存，缓存不跨导入；
返回的 Font 可共享，glyph 对象、路径、样式与 ID 均由实例独立拥有。
共享 resolver 自身的并发安全由调用方负责。

支持范围：

- `x/y/dx/dy` 各接受一个有限的用户单位或现有绝对长度单位，包括负值。
  x/y 默认 0，dx/dy 默认 0；它们作用于元素内第一个规范化后仍存在的
  字符，空 span 不消耗位置。同一字符上，内层 span 的同名标量覆盖外层。
  后续内容延续当前 cursor；dx/dy 不清除同字体/字号的边界 kerning。
- `font-size` 为正绝对长度，默认 16 用户单位，上限 1e6。字体属性、
  `text-anchor` 和 fill/stroke/currentColor/opacity 等复用现有 CSS 层叠、
  继承和 initial/inherit/unset；同字体/字号的连续 span 保留一个 kerned run，
  并分别绘制各 span 的颜色、描边和透明度。混合内容的绘制顺序保持原顺序。
- `text-anchor="start/middle/end"` 使用 advance（含保留的前后空格），
  不用 ink bounds 冒充排布宽度。绝对 x 或 y 开始新文本块；块内的相对
  位移与字号变化参与定位，anchor 取块首字符的值。子 span 只改变 anchor
  而没有新的绝对位置时，继续使用该块的 anchor。
- 空白为 SVG 1.1 子集：默认删去换行、tab 转空格、跨 span 合并连续
  ASCII 空格并去掉整段首尾空格；`xml:space="preserve"` 把换行/tab
  转为单个空格并保留所有空格，不产生自动换行。`xml:space` 可继承、可
  在子 span 用 `default` 复原。注释和 CDATA 边界不打断排布。
- 支持 Latin/Greek/Cyrillic 和 Common script 的非组合字符，实际字体必须
  覆盖每个字符。例如 `éΩЖ∑` 可直接产生轮廓；字体缺字明确报错。
  `text-rendering="auto/geometricPrecision"` 采用同一套 unhinted 轮廓。
- text、祖先组、use/symbol 的既有矩阵和样式隔离继续生效。填充文字可
  使用真实仿射变换；可见描边仍要求等比变换。symbol 需要裁剪 glyph
  时明确报错。defs 不直接绘制，use 可以引用整个 text。

明确拒绝：复杂塑形脚本（含汉字在内的其他 script 当前也未开放）、
combining marks、bidi 控制/双向排布、joiner/variation selector、Unicode
行/段分隔符、字体缺字、ligature/feature/variation 配置、字体 fallback、
非支持 weight/style、百分比或 em/rem 字号与坐标、字符位置数组、rotate、
textLength/lengthAdjust、textPath、tspan transform、letter/word-spacing、
writing-mode、baseline/decoration 等未实现属性。CSS `white-space` 不支持，
使用上述 `xml:space` 子集。没有隐式 Pango 或浏览器等价承诺。

文字属性/字符白名单和嵌套限制也检查未引用的 defs；字体查找、缺字和
轮廓/裁剪检查发生在实例化时。display:none 的内容不参与排布，hidden
仍保留 advance，子 span 可恢复 visible。opacity 沿用现有逐级相乘语义。

每次 Read 的文字预算：源字符数据 256 KiB 且不超过 65,536 runes；
展开后处理至多 65,536 runes（包含规范化前空白）；绘制至多 32,768
有轮廓 glyph、1,048,576 轮廓段（含 move/闭合计数）；最多 32 层 text/tspan、
32 个字体 family/style、4,096 次 Layout 调用（包括边界 kerning 度量）。
glyph 和内容容器也计入既有 100,000 展开对象预算，既有 XML/use/CSS
限额继续生效。超限使整个 Read 失败，不返回漏字的部分对象树。这些是
阶段限额，不是峰值内存承诺。

测试在 `tests/_cases/svg/text*_test.go`；fontTools 独立字形边界/advance、
四套精确 Go 字体的 CairoSVG 矢量参考及像素结果在 `tests/data/svg/text-*`。
来源、字体校验值、开发参考步骤、误差与基准见 `text-provenance.json`。
开发参考中的 Python/FreeType/Cairo 不参与 Go 运行时或测试执行。

## 嵌入图片

入口仍为 `svg.Read(reader, svg.Options{Width: 4})`，返回包含普通
`gomath.Object` 图片节点的对象树。运行时仅使用 Go 标准库解码与
`gomath/rasterimage`，没有外部进程或原生库。主文档坐标仍按 viewBox
居中、等比归一化到指定世界宽度，并翻转 Y 轴；图片像素方向保持正确。

## 已支持

- `<image href="data:image/png;base64,...">`、`image/jpeg` 和 `image/gif`。
  兼容 `xlink:href`；同时出现时 `href` 优先。MIME 大小写不敏感，必须
  与文件实际格式相符。只接受标准、正确补位的 base64，不接受空白折行、
  percent encoding、额外 MIME 参数或省略 MIME。
- PNG/JPEG 静态解码；GIF 仅首帧，且首帧必须覆盖完整 logical canvas。
  不播放动画，不合成局部首帧的背景或 disposal。不解释 JPEG EXIF 方向、
  ICC 色彩配置等额外元数据；沿用 rasterimage 的标准库解码语义。
- `x/y` 默认 0，支持现有绝对长度单位和相对当前 SVG/symbol 视口的百分比。
  `width/height` 支持绝对长度、百分比、`auto` 和省略；两项 auto 时采用
  像素固有尺寸，仅一项 auto 时保持固有比例。负数、非有限值和溢出报错。
  零尺寸产生零面积图片，与现有 bitmap 渲染行为一致。
- `preserveAspectRatio` 默认 `xMidYMid meet`，支持全部九种对齐和 `none`。
  `meet` 的空白区域透明；`none` 可非等比拉伸。
- 图片节点、祖先和 use/symbol 变换相乘，支持旋转、缩放、反射、剪切。
  图片平面是真实仿射几何，不是变换后的包围盒替代物。
- defs 中的图片不直接绘制。use 引用图片、含图片的组或 symbol 时，每个
  实例有独立 ID、路径和样式；同一次导入中完全相同的 URI 共享不可变
  `gomath.Bitmap`，无需重复解码。缓存不跨导入。use 的 width/height
  仅对 symbol 视口生效，不改写被引用 image 的尺寸。
- 复用现有 CSS 层叠、use 样式隔离及 `opacity`、`display`、`visibility`。
  object/group 的 opacity 按现有渲染器逐级相乘；没有新增离屏组透明合成。
  `fill/stroke/fill-opacity/stroke-opacity` 等向量绘制属性不作用于图片像素，
  不生成 bitmap tint 或描边。滤波沿用现有 premultiplied sRGB 双线性采样，
  边缘采用像素延伸；抗锯齿与 CairoSVG 不逐像素相同。

## 明确拒绝

外部 URL、文件路径、fragment 图片 href、其他格式/URI 编码、损坏图片、
MIME 不符、局部 GIF 首帧、`slice`、`defer`、非中性 mask/filter、
未知 image 属性和未知 CSS。现有 CSS 本身不支持 CSS geometry、transform
或 image-rendering 声明。image 支持上文的 clipPath 子集。
symbol 内容需要视口裁剪时返回错误；不伪造
裁剪结果。根视口的显式非 visible overflow 仍报错。

所有 image 元素（包括未引用 defs 中的元素）都会校验 URI、属性白名单并
解码；尺寸、变换、样式和 symbol 裁剪在实例化时校验。一个错误使整个
Read 失败，不返回漏掉图片的部分结果。图片标题/描述/metadata 可复用现有
非绘制节点处理；可见子元素报错。

## 每次导入的资源上限

| 资源 | 上限 |
| --- | --- |
| 整个 XML 输入（解析前流式限制） | 32 MiB |
| 单条 image URI / URI 字符串累计 | 12 MiB / 24 MiB |
| 单个编码资产 / 唯一资产编码累计 | 8 MiB / 16 MiB |
| 单个解码资产 / 唯一资产解码累计 | 4,194,304 / 16,777,216 像素 |
| 不同 URI 资产数 | 128 |

URI 累计包括重复 image 元素，use 仅计其引用展开，不复制 URI。
文件头先检验尺寸和总像素预算，随后调用 rasterimage 完整解码；不会在
解码后才检测尺寸炸弹。现有 XML/use 深度、展开节点及 CSS 工作预算继续
生效。这些是各阶段资源上限，不是 Go 进程峰值堆内存承诺。

## 验证资料

测试源位于 `tests/_cases/svg/image*_test.go`；真实 PNG/JPEG/动画 GIF、
Pillow 完整解码参考、CairoSVG 仿射场景参考均在
`tests/data/svg/`。`image-provenance.json` 记录来源、版本、像素差异、限额、
验证命令和导入 benchmark。Python/Cairo 仅用于开发生成参考，Go 测试不
调用它们。本功能不声称与 Manim 的纯向量 SVG 转换能力等价。

## 原生与 SVG 虚线描边

原生对象通过不可变模式配置虚线：

```go
pattern, err := gomath.NewDashPattern([]float64{0.3, 0.15}, -0.1)
if err != nil {
    return err
}
object.Style.Dash = pattern
```

`Style.Dash == nil` 为实线；`DashPattern` 零值无效。构造函数复制输入，
`Lengths()` 返回副本，`Offset()` 和 `Period()` 返回规范化后的相位与周期。
模式可以安全地由对象副本、快照和并行采样共享。`Scaled()` 返回按正数
等比缩放的新模式；不会修改原模式。样式插值到终点才切换整个模式，
不会在插值期间渐变、重采样或共享可写 slice。

数组按 dash/gap 交替，奇数项列表重复一次。每项必须有限且非负；周期
必须有限且大于零，全零列表报错。正负有限 offset 按周期取模；正 offset
表示从模式中向前取样，遵循 SVG 相位方向。单个零 dash 配圆帽得到圆点，
配方帽得到按原路径切线旋转的方点，配 butt 帽不绘制点。零 gap 连接
相邻绘制区间，保持经过原路径顶点时的 join。

SVG presentation 属性、内嵌 CSS、inline style 都支持 `stroke-dasharray`
和 `stroke-dashoffset`，复用已有 specificity/source order、inherit/initial/
unset 及 defs/use 样式语义。数组接受逗号、空白分隔的用户单位和现有
绝对长度单位。offset 独立继承，直到与最终数组组合时才取模；改变子节点
数组不会错误复用父节点已经取模的相位。`none` 选实线。当前明确拒绝
百分比长度、CSS calc/var、`!important`、pathLength 虚线校准和非法分隔符。

SVG 的等比 transform、symbol 视口映射及最终世界宽度归一化都会同时
缩放 dash 长度和 offset；可见描边的非等比/剪切 transform 仍明确拒绝。
原生对象几何变换沿用已有 Style 世界单位语义，不自动改写描边宽度或
虚线模式；需要同时缩放样式时显式调用 `pattern.Scaled(scale)`。

渲染先使用现有 `CurveTolerance` 展平曲线（默认 0.02 像素），再按投影后
XY 轮廓的实际折线长度切分，不按贝塞尔参数或每个原始线段重新排布。
模式在每个轮廓起点重置；跨顶点的 dash 保留 join。闭合接缝两侧连续的
绘制段合并为一段，整圈都在一个 dash 内时保留闭合轮廓。填充不受虚线
影响，Reveal 仍先裁路径再按同一模式描边。曲线长度误差来自已有展平
近似，不声称解析弧长精确。SVG 导出保留精度，写出规范化数组和 offset，
与原有 cap/join/miter 和世界到像素变换共同生效。

每个模式最多 1,024 项（奇数重复后计数）。每个可见描边对象跨所有轮廓
最多处理 65,536 个 dash 片段、262,144 次模式步进、1,048,576 个输出
折线点；点状 dash 和零项也计入预算。超过上限或浮点精度无法继续推进
时返回错误，不丢片段或退回实线。图像节点仍不应用向量描边属性。

虚线验证资料为 `tests/data/svg/dash-*.{svg,png,json}`，包括 CairoSVG
彩色曲线、零长度方帽、零 gap、闭合接缝场景，以及源码固定版本的
Manim 直线/闭合方形几何。精确能力、误差和性能记录见
`tests/data/svg/dash-provenance.json`。这不替换固定 `DashedLine` API，
也不声称与 Manim 的按数量排布或动画行为完全等价。
