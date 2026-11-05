# 有界纯 Go 多边形布尔

`gomath/path.Boolean(a, b, op, options)` 实现填充区域的并、交、差与异或，生产实现仅使用 Go 标准库（含 `math/big`）。不调用 Python、Skia、cgo、native 库或子进程。此 API 独立于已有路径测量、展平和渲染的默认语义。

```go
options := path.DefaultBooleanOptions()
options.RuleA = path.NonZero
options.RuleB = path.EvenOdd
result, err := path.Boolean(a, b, path.BooleanDifference, options)
```

`BooleanUnion`、`BooleanIntersection`、`BooleanDifference`、`BooleanXor` 中，difference 为 A 减 B。`BooleanOptions{}` 等价于两侧 NonZero 和默认限额。`RuleA`、`RuleB` 分别解释各自输入的全部轮廓；洞的方向规则、相同方向的嵌套、重复轮廓及反向抵消均参与真实绕数计算。

## 接受的几何范围

- 输入为空路径，或由闭合、精确连接的 XY 线段轮廓组成；所有坐标和控制点必须有限，Z 必须恰为 0。
- `Closed` 标志允许最后一个端点到 Start 的隐式闭合边。这个规则也保留 Builder.Close 在极小尺度下未显式添加的最后一条边。
- 支持自交、轮廓相互穿越、孔洞、边接触、点接触、共线部分重合和完全相同的输入。
- `path.Line` 生成的 cubic（以及反向控制点顺序）按端点线段解释。其他 cubic 仅在两个控制点精确共线且都在线段内时接受。
- 非线性 cubic 明确报错。本接口没有 Tolerance 参数，也不执行隐式曲线展平。调用者若先自行展平，结果只对应其提供的多边形；不能据此声称精确 Bézier 布尔或曲线拓扑保证。
- 开放轮廓、move-only 轮廓、显式零长边、全共线轮廓、连接缝隙、非平面和非有限坐标明确报错。全共线轮廓不能伪装成合法空输入；合法多轮廓的绕数抵消则可以产生空结果。

结果采用面积正则化：孤立的点或线没有填充面积，因此纯点/边接触的 intersection 返回空路径。它不会返回零面积的独立折线。所有错误均返回空 Path 和非 nil error，不返回部分成功的几何。

## 精度、拓扑和所有权

每个 float64 坐标先转换为其精确二进制有理数。所有源边对（也包括同一输入内部）通过有理数计算交点，共线重叠按端点分裂；原子边去重后，使用边中点两侧的符号无穷小法精确分类填充绕数。没有吸附 epsilon，也不以一个浮点采样距离决定边界。

输出边统一令填充在左侧，并依各顶点的循环射线顺序连接闭合轮廓。外边界逆时针、孔洞顺时针；在边界外的点上，输出的 NonZero 与 EvenOdd 填充区域一致。点接触可能保留共享顶点，不保证所有轮廓互不接触或轮廓顺序具有几何排序意义。

有理交点最终必须舍入到 float64。交付前再次用精确算术检查：

1. 不同输出顶点不能舍入为同一顶点；坐标和输出 Line 控制点必须有限。
2. 不能引入新交叉、T 接触、共线重叠或同向重叠射线。
3. 顶点的循环射线次序必须保留；相反共线射线的微小弯折允许通过。
4. 每条舍入边左侧绕数必须为 1、右侧必须为 0，拒绝极薄轮廓翻转、消失或孔洞层次改变。

浮点结果是经过这些检查的直线多边形近似，并非有理坐标的无损表示。不可表达的狭缝或交点会明确报错；不会自动增大容差、消除小边或假造空结果。内部精确算术也受位数限额约束，因此并非所有有限 float64 多边形都保证成功。

输入从不修改，输出轮廓和曲线切片独立分配。完成输入构造后可并发只读调用；调用期间同时修改输入切片不受支持。成功输出具有确定性，不依赖 map 遍历顺序；交换 union/intersection/xor 的输入保持相同几何和规范输出顺序。

## 资源限制

`BooleanLimits` 的 0 字段采用默认值，正值可覆盖默认但不能超过硬上限，负值或超限报错。所有限额按单次调用累计，两个操作数共享预算。

| 字段 | 默认 | 硬上限 | 含义 |
| --- | ---: | ---: | --- |
| Contours | 1,024 | 4,096 | 输入轮廓数之和 |
| Segments | 512 | 2,048 | 源线段总数，包含隐式闭合边 |
| Pairs | 262,144 | 1,048,576 | 源边对、舍入边对和循环射线排序比较的累计次数 |
| Vertices | 8,192 | 16,384 | 精确去重顶点数（包含输入顶点及交点） |
| Edges | 8,192 | 16,384 | 去重后的全部原子边数，含最终未保留的边 |
| SplitPoints | 16,384 | 65,536 | 各源边分裂点引用的总数，含端点 |
| Work | 2,000,000 | 8,000,000 | 有理运算、比较及图操作的计费工作量 |
| RationalBits | 4,096 | 8,192 | 每个持久有理数的分子、分母各自的位数 |
| AllocationBytes | 128 MiB | 512 MiB | 累计算法分配计费预算 |

有理运算/比较每次计费 `256 + 64*ceil(操作数分子分母总位数/64)` 字节，图、切片、键字符串等另计。即使临时对象已被回收，费用也不返还。位数、顶点、边、工作量等限额同时生效；算术结果检查发生在单次有界运算之后。

AllocationBytes 是保守的算法分配计费，不是 Go 进程 RSS 或 runtime/GC 的硬内存限制，也不是实际 `B/op`。默认数值不承诺所有低于 Segments 的输入都能完成：交点密集或位宽大的输入可能先耗尽其他预算。实现使用源边两两求交、原子边对源边的绕数分类，以及输出边两两校验，时间包含 O(S² + E·S + B²) 项；不适合无上限的大型 GIS 数据。

## 独立验证与复现

测试代码全部位于 `tests/_cases/path/boolean*_test.go`。参考数据在 `tests/data/path/boolean-*`，开发生成脚本为 `tests/reference/path_boolean.py`。不会在生产包中生成测试文件，也不会在 Go 测试中启动参考程序。

参考固定于 Manim 源码提交 `16ea443e2a5f1d79db0a721161411ce916d6e733` 的 `boolean_ops.py` 所使用的 Skia pathops 后端。生成环境为 Python 3.11.5、pathops 0.9.2、CairoSVG 2.7.1、Cairo 1.18.4；源码、脚本及数据 SHA256 记录在 `boolean-provenance.json`。12 组输入覆盖四种运算，比较面积和每组 324 个离边界采样点。Skia 使用 float32，故仅把这些中等尺度整数/二进制小数输入作为独立参考；极小尺度、精确交点和舍入拒绝另有 Go 测试。

4 张独立 PNG 由 pathops 输出轮廓交给 CairoSVG 栅格化：128×128、透明背景、实色填充、无描边，包含洞和倾斜交边。当前逐图 premultiplied RGBA MAE 为 0.01816–0.03988/255，最大通道差 15；测试上限 MAE 0.08、最大差 24、差值大于 8 的像素占比 0.003。这个参考不代表 Manim 动画中间帧或精确 Bézier 对齐。

还有固定种子的复合矩形、非轴对齐三角形、自交多边形真值表、两侧填充规则、触点/重边/孔洞、独立输出所有权、并发只读、全部限额、最小 subnormal 轮廓和不可表达狭缝/翻转轮廓测试。

```sh
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -tags noasm -count=1
CGO_ENABLED=1 go run ./tests/cmd/run -pkg=path -race -count=1
CGO_ENABLED=0 go vet -tags noasm ./path
CGO_ENABLED=0 go run -tags noasm ./cmd/purecheck
CGO_ENABLED=0 go run ./tests/cmd/run -pkg=path -run '^$' -bench BenchmarkBooleanPolygonIntersection -benchmem -benchtime=150ms -count=3 -p=1
```

2026-11-04，Go 1.27.3、darwin/arm64、Apple M2，CGO_ENABLED=0，串行 `-p=1`，三次样本：

| 两个输入各自顶点数 | intersection 时间（ms/op） | B/op | allocs/op |
| --- | --- | ---: | ---: |
| 8 | 1.2802 / 1.2716 / 1.2764 | 992,324–992,335 | 19,019 |
| 24 | 3.1549 / 3.1708 / 3.1745 | 3,100,487–3,100,533 | 63,039–63,040 |

基准包含精确转换、求交、分类、舍入校验和结果构造；输入为半径 5、横向错开 2 的两正多边形。排除输入构造、参考生成和渲染：图像尺寸不适用、帧数为 0。该测量没有与 native pathops 做性能比较，也不构成速度或内存领先声明。

## 集成示例

`gomath demo -scene boolean` 使用实际多边形输入计算全部四种运算，然后对结果
做真实图形动画。640×360 的三个时刻 SVG 再渲染对照 MAE0.10036–0.11004/255、
最大41–47，记录在 `artifacts/boolean-demo-svg-20261104/provenance.json`。
它共享 Go 场景构造，只验收导出/光栅一致性；独立几何依据仍是上面的 pathops 参考。
