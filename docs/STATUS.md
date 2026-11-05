# 迁移进展 — 2026-11-05

目标是纯 Go 覆盖 Manim 能力并取得可证明的性能收益，**尚未完成**。
Python 参考项目保持干净，固定 Manim Community0.22.1 / 提交
`16ea443e2a5f1d79db0a721161411ce916d6e733`。Go 位于相邻 gomath，
分支 `codex/pure-go-foundation`。来源清单177模块/2498条定义全部仍为
unverified；窄范围样本不证明完整方法或全库兼容。

## 第四批检查点通过

已集成最多16个RGB世界点光、独立自适应RK45流线、AU原生解码、显式字体回退、
普通对象evenodd填充，以及角/直角标记。五个代理交付已冻结；父线程已接入
angle-marks/adaptive-streamlines/point-lights/fill-rules/font-fallback真实示例和AU CLI。
整仓CGO0/noasm测试13.453s、完整race55.543s，vet、purecheck42及Linux/Windows
amd64整仓交叉构建通过。所有测试位于tests/，Python参考仓库git状态干净。
详细命令、冻结生产源码/二进制hash及集成结果在
`tests/data/integration/wave4-checkpoint-20261104.json`。这些是共享开发机验收耗时，
不是隔离性能测量或Manim全库对等证明。

## 第五批检查点通过（2026-11-05）

五个并行模块已集成并冻结：柏拉图多面体/有界凸Polyhedron、物理时间流线Flow、
FLAC PCM8/16/24/32与1–8声道输入、有限文字markup、SVG局部bbox裁剪。
父线程补两点圆弧/曲线箭头/相切圆弧，49个构造/343个固定Manim动画状态通过；
箭头裁剪方式与Manim默认值有明确差异。六个真实CLI示例均已渲染并目视检查。

整仓CGO0/noasm集中套件8.660s、完整race58.850s，整仓vet、purecheck42及
Linux/Windows amd64整仓交叉构建通过。测试源码/fixtures仍统一位于tests/，
Python参考仓库git状态干净。共享主机、编译/缓存状态影响这些验收耗时，
不构成隔离性能测量。命令、完整日志与冻结源码/二进制hash在
`tests/data/integration/wave5-checkpoint-20261105/checkpoint.json`。

五个640×360/SSAA4 SVG示例各三个时刻通过原阈值CairoSVG对照：圆弧max25，
相切圆弧max17/44/22，markup max17/17/23，flow max16，bbox clip max30/36/28。
这些共享Go场景构造，只验证导出/光栅集成；独立源几何/字体/时钟参考另见专题。
多面体5秒视频的40帧/5关键帧/PTS严格解码通过，40帧均不同；七帧RGB
PSNR39.796–40.126dB、MAE1.331–1.442，仍不是独立Manim渲染或全库对等证明。
以上引用provenance均与同一冻结源码、二进制hash一致。

FLAC扩展保持旧字节及样本，原有双声道16/24bit五样本中位解码耗时分别
增加3.44%/4.72%；这是已记录的待优化回退，不把功能扩展报告为性能提升。
后续新生产改动不由这个冻结检查点背书。

## 第六批检查点通过（2026-11-05）

五个并行模块已交付冻结：任意端点3D圆柱/箭头、曲线有向网络/自环、FLAC CRC16
性能优化、有限根指数/boxed/phantom排版、多子形状polygon裁剪union。父线程补齐
包围框/背景框/下划线/交叉标记和Elbow；前四类48个构造/336个固定Manim控制点
集合状态、Elbow15个构造/105个有序控制点状态通过。五个真实CLI示例已接入并目视检查。

整仓CGO0/noasm集中套件9.299s、完整race66.522s，整仓vet、purecheck42及
Linux/Windows amd64整仓交叉构建通过。源码和CLI二进制在整个验证期保持冻结；
测试源码/fixtures全部集中于tests/，Python参考仓库git状态干净。
这些是共享主机验收耗时，不是隔离性能测量。完整命令、日志、源码/二进制hash和
五项同版本集成证据在 `tests/data/integration/wave6-checkpoint-20261105/checkpoint.json`。

四个640×360/SSAA4实际SVG导出，各三个时刻通过原CairoSVG阈值：annotations
max18/52/42、radical-boxes max17/30/17、curved-networks max30/35/36、
svg-clip-unions max32/32/32，RGBA MAE均<.1/255。3D端点示例5秒40帧视频完整
严格解码，5个关键帧及PTS正确，40画面均不同；七个原生PNG比较RGB
PSNR41.746–42.090dB、MAE1.167–1.228。以上均共享Go场景构造，验证导出/光栅/
容器集成；不能替代专题中的独立几何、字体、时钟或完整Manim行为oracle。

独立证据包括3D108组解析/射线、网络14组567个点、公式20组字体/Cairo
最小IoU.990031、SVG22图max17及两个动画中间帧max2。FLAC306条冻结流/
519805标量样本经libFLAC与严格FFmpeg完整回归；同期交错五样本旧双声道
16/24bit解码中位耗时分别下降44.98%/69.58%，分配次数保持。该CRC16等价查表
优化与第五批的轻微回退分别记录，不将受限样本推广为全库或相对Manim性能优势。
具体支持边界以SEGMENT3D/CURVED_NETWORKS/RADICAL_BOXES/SVG_CLIP_UNIONS/
FLAC_PERFORMANCE/ANNOTATIONS专题为准。后续新生产改动不由这个冻结检查点背书。

## 第七批检查点通过（2026-11-05）

并行模块已冻结：棋盘参数曲面真实双网格、直线Graph/DiGraph原子增删、光栅裁剪
scratch复用、精确int64/有限小数数字对象、有限l/c/r array与真实规则线。父线程
补齐平面Brace/BraceBetweenPoints及标签放置：32构造/224个固定Manim有序控制点
状态和12个标签位置通过。五个真实CLI示例已接入并目视检查。

整仓CGO0/noasm集中套件10.274s、完整race73.720s，整仓vet、purecheck43和
Linux/Windows amd64整仓交叉构建通过；测试/参考仍统一于tests/，Python参考仓库
git干净。生产源码和二进制在整个验收期保持冻结。完整命令、日志、SHA256和五项
同版本集成证据位于 `tests/data/integration/wave7-checkpoint-20261105/checkpoint.json`。
上述耗时来自共享主机，不是隔离性能比较。

四个640×360/SSAA4 SVG示例各三个时刻按原阈值通过：braces max18、
network-mutations max20、math-arrays max19/20/19、numbers max18；RGBA MAE均<.1。
棋盘曲面640×360/QP18视频完整严格解码通过，5秒40帧、5关键帧、PTS正确，40帧
均不同；七帧原生PNG比较RGB PSNR32.071–32.719dB、MAE2.073–2.217。
320×180/QP24与QP18在既定28dB画质阈值下失败，原始输出和failure.json分别
保留于对应artifacts目录；不宣称320尺寸画质通过，不放宽阈值。调整的是显式输出
尺寸/编码质量，未修改生产编码器。这些媒体/SVG证据共享Go场景构造，不能替代
独立几何/字体/时钟参考。

独立切片证据：数字1376个binary64格式/48个int64格式/20组字体参考（4个Manim
零符号策略差异保留）；曲面108射线/3解析棋盘/14时间线帧；array7静态/2仿射中间帧；
图10拓扑状态/20端点。光栅scratch普通/复杂指定场景分配字节减少56.97%/60.00%，
时间中位变化为下降22.25%/2.46%，存在部分子场景回退；每worker保留最高96MiB
像素scratch，增长期间旧新内存可能并存等待GC。详见对应专题，不推广为全库性能
优势或完整Manim兼容。后续新改动不由此历史冻结检查点背书。

## 第八批冻结验收（2026-11-05）

检查点：`tests/data/integration/wave8-checkpoint-20261105/checkpoint.json`。
CGO0/noasm 整仓集中测试4.714s、完整race50.954s、整仓vet、purecheck45个非标准包、
Linux/Windows amd64整仓交叉构建均通过。验收期间生产源码与CLI binary哈希未改变，
没有tests外测试文件，Python参考仓库保持干净。耗时来自共享主机验收，不是性能基准。

本批六个切片是ArcBrace、原子逐点/复数映射、逐控制点圆弧Transform路径、SVG容器/
文字bbox裁剪、不可变图像像素编辑，以及Go代码显示（真实Go Mono字体、Go词法高亮、
行号/背景、行/token选择及不可变快照）。代码切片44项冻结输入提升前逐一核验，生产/
测试文件原样提升；提升文档的唯一hash变化记录在tests/data/code/validation/promotion.json，
原agent冻结清单保留。第三方字体完整许可已覆盖Regular/Bold/Italic/BoldItalic/Mono。

六个真实示例分别在.5/1.7/3.3秒核对原生PNG和实际导出SVG的独立CairoSVG渲染，
全部证据与检查点源码/binary哈希一致，已附加到checkpoint。arc-braces/pointwise/
image-pixels/transform-paths/code-display使用SSAA4，最大通道差分别25/40/18/17/18，
RGBA MAE均低于.253。svg-container-bbox初始SSAA4在3.3秒最大通道差71超过64，
失败像素与failure.json保留在artifacts/svg-container-bbox-wave8-svg-20261105；显式SSAA8
复验最大差54/55/56、MAE小于.079，原阈值不变。这是超采样设置下的有界边缘精度记录，
不宣称SSAA4失败已修复或任意SVG渲染等价。

独立源参考另有ArcBrace36构造/252有序动画控制点状态，逐点映射24构造/168状态，
圆弧路径105状态/12Cairo中间帧，SVG8静态/10精确bbox/5动画时刻，图像13条65536像素
操作链/9动画帧，代码6组字体/词法布局及10动画帧。各文档明确支持边界。这些证据不把
Manim API清单标为已验证，不代表全部迁移完成或整体性能更快。下一批新改动不由此
历史检查点背书。

## 第九批冻结验收（2026-11-05）

检查点：`tests/data/integration/wave9-checkpoint-20261105/checkpoint.json`。
CGO0/noasm 整仓集中套件11.817s、完整race77.977s、整仓vet、purecheck45个非标准包、
Linux/Windows amd64整仓交叉构建通过。生产源码和CLI二进制在验收期保持不变，
测试仍全部集中于tests/，Python参考仓库干净。以上是共享主机验收时间，不是性能基准。

新增有界曲线细分/开放及循环smooth控制柄、整对象树原子MakeSmooth/MakeJagged与
非线性变换前细分、显式公式分片及精确key稳定选择/快照、SVG整text glyph-cell bbox渐变，
以及对象更新器注册/暂停/恢复/永久移除与Play源family自动暂停。严格闭合、独立片段
语法、登记顺序/绝对时钟与Manim的差异分别见各专题文档；不是完整源语义匹配或通用dt兼容。

四个640×360/SSAA4示例已目视检查，实际导出SVG由CairoSVG独立解释：curve-edit
在.5/1.7/3.3秒最大差23/21/54，math-parts为17/19/19，svg-text-gradients为26/21/32；
updater-lifecycle在.5/1.5/2.5/3.5/4.5秒为16/17/16/16/16。RGBA MAE均小于.089，
原阈值未变。四项证据已核对同一源码/binary哈希并附加至检查点；它们共享Go场景构造，
不替代独立Manim/字体参考。

独立证据：path24锚点构造/18细分构造/20动画状态，对象曲线12构造/84有序动画状态，
更新器13场景/91有序控制点状态，公式9个独立字体/Cairo表达式及4动画帧，SVG文字11静态/
5动画时刻的两种采样精度。父线程补强对象曲线/更新器参考生成前的installed/local/git
源码一致性校验，再生数值保持完全相同。公式分片另加复制前child/contour预算和source key
独立存储，原agent清单保留，父线程修订单独记录。

更新器批量遍历优化仅与本批naive实现比较：1/32/128对象单Frame采样耗时中位分别
2262→1293ns、340718→31891ns、5047866→131900ns。单对象分配1488→2144B、10→20次
确有回退；共享主机、非交错三样本和历史工具链缺失等限制见OBJECT_UPDATERS.md。
不是Manim或全库性能比较。全部能力族仍Partial/Not started，第十批改动不由此历史检查点背书。

## 第十批冻结验收（2026-11-05）

检查点：`tests/data/integration/wave10-checkpoint-20261105/checkpoint.json`。
CGO0/noasm 整仓集中套件10.891s、完整race75.427s、整仓vet、purecheck45个非标准包、
Linux/Windows amd64整仓交叉构建通过。源码和CLI二进制在验收期保持冻结，tests外没有
测试文件/测试数据目录，Python参考仓库保持干净。共享主机验收时间不作为性能比较。

三个切片已落地：显式key分片匹配（重复key透明副本/未匹配淡入淡出/真实目标替换/
源对象复用）、有界Bezier路径反向（单contour、完整path、receiver-only和family原子编辑），
以及原创矢量公式overbrace/underbrace和上下标签/嵌套。公式花括号遵循明确有限em策略，
不是完整TeX字形伸缩；路径严格闭合，原Circle浮点端点间隙会被拒绝。完整边界见
MATCHING_PARTS.md、PATH_DIRECTION.md、MATH_BRACES.md。

独立参考有63个实际Manim分片匹配控制点/颜色/透明度中间状态（含圆弧路径）及9清理案例、
路径54个实际Transform状态（36个内部状态），公式8静态/2仿射字体/Cairo参考。匹配另经
8项独立反例检查，覆盖整frame唯一ID、并行嵌套替换、连续复用、managed updater/replay、
2→3分配、独立曲线预算和手推像素层顺序。原参考漏采FadeIn对象的失败资料保留，已修正
生成器；新公式示例误用未支持cdots的失败日志也保留，改为显式三项，不放宽解析器。

三个640×360/SSAA4示例已目视检查；在.5/1.7/3.3秒对照实际SVG的独立CairoSVG渲染：
matching-parts最大差18/38/18，path-direction24/18/18，math-braces18/20/18，RGBA MAE均
小于.086，原阈值不变。三项证据核对与检查点同一源码/binary哈希后附加，不能替代独立
Manim构造/TeX排版oracle。两个agent交付的38/24项冻结hash已逐一核验。花括号旧拒绝断言
仅移除新支持的两个命令，完整mathtex已通过；原失败和agent清单保留，父线程修订单独记录。

匹配采样避免重复克隆临时bucket树，32个key的单Frame采样中位分配65168→34960B、
334→173次（减少46.35%/48.20%）；同期非交错三样本耗时18592→20245ns，回退8.89%，
未宣称加速。原始日志、工具链、硬件、命令和源码hash完整记录在matching专题的performance
目录。没有全库或相对Manim的性能优势证明。所有能力族仍Partial/Not started，完整迁移未完成。

## 已集成能力

- 动画：身份保留 Transform/实际替换、TransformFromCopy、独立双层 FadeTransform；
  默认 sigmoid Smooth、ease families、有限 overshoot、嵌套组/各层时钟；
  共享对象 Succession、相对动作延迟初始化、全序列原子提交和逐步 cleanup；
  Growing/SpinIn/Indicate/Wiggle/ApplyWave/PassingFlash/Uncreate。
- 更新器：绝对时间 ValueTracker、固定结构 redraw、有预算和确定性编号的动态
  display topology、固定步长/检查点 Replay。通用 updater 顺序和动画暂停尚不等价。
- 矢量：三次贝塞尔、弧长/逆解与不可变查询索引、参数裁剪、真实虚线/端点/连接；
  圆环/扇形/椭圆/圆角矩形/双箭头；有界 nonzero/evenodd Clip 及祖先相交贯通矢量、
  位图、相机与 SVG。原生线性/同心径向渐变具有真实矢量覆盖、stop alpha/
  硬边界、完整仿射框架、兼容动画及 SVG paint server，仅 pad/sRGB 子集。
  新增闭合 XY 多边形并/交/差/异或，两侧独立 nonzero/evenodd；精确有理数
  求交与浮点输出拓扑检查，支持洞/自交/共线接触，非线性 cubic 明确拒绝。
- 坐标：数轴/平面、函数/参数/自适应曲线、黎曼矩形和两函数间面积；
  显式不可变正线性/log-base>1 Scale、检查域的 ScaledNumberLine/ScaledAxes，
  对数/半对数绘图向回调传入真实数据。表格新增独立行列标签/角标及稳定数据索引。
  正负柱状图支持固定数量的数值目标、零高度和跨零动画、类别名及随整体仿射变换
  更新的坐标框架；SetValues 原子更新并保留指针、ID、样式与子对象。
- 数值：LU/列主元 QR/Cholesky、对称 Jacobi eigen、宽/高/秩亏 economy SVD、
  伪逆/最小范数/多 RHS；一般实矩阵 Schur 和复右特征向量，逐列验证原矩阵
  residual，明确报告数值 basis rank，不保证 defective 矩阵有完整独立基；
  不可变复数稠密算术/共轭转置和部分主元 LU、多 RHS、LogDet/Inverse，带显式预算；
  独立严格 CSR、转置/加减/向量积、带取消/预算的 CG 与显式 Jacobi，成功依据
  原始系统残差；不证明 SPD，也不提供稀疏分解。
  SpGEMM 使用确定性稀疏行堆和精确乘积求和，显式限制工作量、存储和取消；
  不分配与输出列数相同大小的稠密标记数组。
- SVG：路径/基本形状、defs/symbol/use、受限 CSS、data URI 图片、真实 text/tspan
  字体轮廓、虚线描边；受限 userSpaceOnUse 单形状/use clipPath、nonzero 洞和
  嵌套相交。basic/path/image/direct-use的objectBoundingBox裁剪已支持，局部bbox排除描边，
  退化bbox忽略效果；container/text/symbol/use链目标仍拒绝。新增多子形状sharp rect/polygon/closed linear path/direct-use
  按独立规则做真实并集并物化为一个Clip；曲线/开放/内部嵌套组合明确报错。
  普通和裁剪两类evenodd均已支持。
- 文本：真实四字体轮廓、显式最多16字体回退；有限XML markup支持嵌套b/i、
  真实BoldItalic、六位前景色、转义及换行，同字体颜色边界保留kerning。
  仍无shaping/bidi/全Pango兼容。
- 流线：RK4及独立自适应RK45；不可变Flow按真实物理时间裁剪尾迹，支持正反速度、
  逐线延迟和循环/排空，并发乱序采样不积分、不运行场/颜色回调。
- 网络：单独CurvedDiGraph支持显式曲率、分离的双向边和指定方向/边/半径的自环；
  端点依据当前节点bbox，原子更新保留root/shaft/head身份；无自动逐帧重路由。
- 3D：新增任意端点CylinderBetween/ArrowBetween真实闭合网格，确定roll和UV/硬法线，
  箭头无内部重复盖；尺寸不自动缩放。四类柏拉图真实平面法线网格及有界闭合凸Polyhedron，拓扑/朝向/UV
  均检查；非凸/开放/非平面等明确拒绝。网格和真实点云共享深度/透明排序/SSAA，UV 纹理、方向光/世界点光、
  不透明方向硬阴影；独立 scene3d 不可变网格 TRS/相机/显隐关键帧，支持并发
  乱序 FrameSource 采样与普通 2D 叠加，已经接通 CLI 和 MP4；新增固定点数的
  点云位置/预乘颜色/像素半径关键帧及真实 TRS，与网格共享深度；
  新增固定拓扑顶点/法线/UV 及独立材质颜色/alpha 关键帧，64MiB 网格快照预算。
  网格/点云另有四元数旋转关键帧与最短弧 SLERP，保留固定轴整圈动画路径。
- 公式：分组/分数/根号/脚标、真实字体样式、定界符、矩阵/cases、八类重音、
  operator limits/display、命名算子/上下堆叠、真实 STIX 补充集合符号及166个
  mathbb/mathcal/mathfrak 数学字母，已保留许可和独立字形参考。
  显式 dfrac/tfrac/binom/dbinom/tbinom 与最多64行 substack，旧 frac 几何保持。
  color/textcolor 支持19种命名色及六位 HTML 不透明 sRGB，具有明确作用域。
  新增根指数、真实boxed边框和phantom/hphantom/vphantom占位，隐藏内容仍消耗共享预算。
  仍为有限 TeX 子集，不是完整 LaTeX/Typst。
- 音频/字幕：不可变 PCM、严格 WAV/原生 FLAC 编解码、gain/envelope/mix/sinc
  重采样、显式 Remix/Concat/Repeat、Scene/Replay 音轨，CLI 显式单/双声道转换；
  FLAC读取另扩PCM8/32与最多8索引声道，检查33bit side与预测范围；
  writer仍为16/24bit mono/stereo。AIFF/AIFC 整数 PCM 解码、WAV Microsoft IMA ADPCM 4bit mono/stereo 完整块
  解码和必需 fact 尾部裁剪，CLI 检查签名与原生流后的 EOF；
  整数毫秒 UTF-8 SRT/WebVTT 和独立音视频时钟 PCM16 MOV。
  G.711 A-law/mu-law WAV 解码覆盖完整256码值及严格 fact 帧数校验。
- 视频：纯 Go H.264 I4x4/整数变换/量化/CAVLC、可选帧内预测搜索；IDR/P、
  reconstructed-YUV skip、可选 P_L0_16x16 inter residual，以及穷举 radius1..4
  整数亮度运动搜索/分数色度插值。默认码流保持兼容；无分数亮度、B帧、
  多参考、码率控制或硬件编码。

测试源码、bench、fixtures 和开发参考工具全部在 tests/；overlay 保留包内访问，
不在生产目录复制测试。独立 Python/CairoSVG/FFmpeg/SciPy 等仅用于开发验收，
库和 CLI 运行不启动外部程序，也无 cgo/本地绑定。

## 验证记录与检查点

2026-11-04 第二批检查点：包含柱状图、四元数旋转、SpGEMM、G.711、公式颜色和
2D超采样的 CGO_ENABLED=0/noasm 整仓测试通过（集中套件11.227s），整仓 vet、
purecheck（42个非标准库包）、Linux/amd64 与 Windows/amd64 CLI 交叉构建通过。
完整 race 通过（集中套件42.547s）。这些时间来自共享主机，不是隔离性能测试。
后续第三批文件不由这一历史检查点背书，仍需交付后全套验收。

第三批检查点也已通过：包含复平面/极坐标、四元数相机、BiCGSTAB、Microsoft
ADPCM、aligned/gathered与evenodd裁剪，整仓CGO0/noasm集中套件13.628s，
完整race集中套件49.187s；整仓vet、purecheck42及Linux/Windows amd64交叉构建
通过。所有测试仍集中在tests/，Python参考仓库git状态干净。

默认公式 Write .35秒中间帧最大差83已定位为 x/image/vector 的有符号覆盖面积抵消。
显式2D超采样4倍使相同时刻及旧阈值的 SVG 对照通过，最大差18；默认1倍行为和
阈值保持，有限超采样不保证任意自交路径的精确覆盖积分。见 RASTER_QUALITY.md。

最新定向验证：

- root/render/export 的渐变全套与 race 通过；scene3d 的全套连续3次、race、
  vet 通过。graph/table/examples 全套通过；后续代理在途的新文件仍需复验。
- 2026-11-04：整仓 vet、Linux/amd64 和 Windows/amd64 CLI 交叉编译通过。
  尚无这两个平台的本机运行/视觉验收。纯 Go 审计通过42个非标准库包。
- 已有35基础状态、728 rate值/55 transform状态、44 group状态、88 growing/
  indication状态、27 flash端点；新增共享对象 Succession51、FromCopy44、
  FadeTransform33组固定来源状态。具体形状/时钟/默认差异见参考 README。
- 表格：15组固定 Manim 几何参考，可见数据/标签边界与全部线段端点误差<=1e-11；
  数轴：10组 Scale/NumberLine，90映射/逆解与60 tick中心，不含完整 Axes 默认布局。
- 原生 Clip 的独立 CairoSVG 嵌套矢量/位图样本；集成 clipping demo 的 SVG 再渲染
  RGBA MAE0.048409/255、最大30。SVG clipPath 导入五组参考最大18/255。
- 多边形布尔：48组独立pathops操作及4张CairoSVG参考；闭合线段子集和严格限额。
- 固定拓扑mesh/material关键帧：114组独立像素参考；集成40帧MP4完整解码通过，
  七帧PSNR32.913–37.180dB、MAE1.692–2.077/255，记录mesh-morph-media目录。
- 新增分式/二项式/substack：48个独立字体operand参考，最小IoU0.979035；
  完整公式SVG三个时刻最大差18。没有独立TeX整公式排版oracle。
- IMA：37流/287069返回样本精确符合规范参考；FFmpeg舍入/填充差异独立保留。
- 四元数：1300组 SciPy 数值及420组独立网格/点云像素参考；集成40帧MP4
  严格解码/时钟/关键帧通过，七帧RGB PSNR40.902–41.486dB。
- G.711：42条接受流/207898样本精确匹配规范/FFmpeg/libsndfile；明确记录16条
  拒绝流与外部宽松解码器差异。颜色公式及SpGEMM有各自独立参考。
- Microsoft ADPCM：50条参考流/312468返回样本；FFmpeg接受的46条流，其全块
  重建逐字节一致。fact填充、libsndfile舍入/步长和额外系数差异保留。
- 新相机：126组SciPy数值、420组像素、54组裁剪、105组光照/阴影对照；
  集成40帧视频严格解码通过，七帧PSNR38.562–39.074dB、MAE1.371–1.547。
- BiCGSTAB：11组SciPy系统；32未知量集成例子实际10步收敛，另算残差4.35e-11。
  三个SVG时刻最大差50。多行公式16组独立operand参考最小IoU .990984；
  集成Write中间帧SVG最大差17–18。详情见各专题文档。
- evenodd裁剪：直接Cairo参考、实际SVG导出参考与独立有理数像素面积对照；
  修复相机复制规则遗漏后全部回归通过。这是第三批历史检查点；第四批另行加入普通evenodd fill，见 FILL_RULES.md。
- 原生渐变：独立手写 CairoSVG 最大差1/255，导出 SVG 再渲染最大差2/255。
  三维世界点光144组独立射线参考；scene3d84组独立射线参考。
- 数值：Schur56组 SciPy 矩阵；一般复右特征向量30组向量/子空间参考，另验证
  原矩阵 residual、重复/defective/极端尺度及有界失败。不是全部 SciPy 替代证明。
- H.264 motion 独立严格解码303码流/5182帧/825 IDR入口，重建逐像素相等；
  cost优化后的 golden 和独立解码保持不变，codec覆盖99.8%。只记录测量的
  TestMotionMeasurements 已改为 GOMATH_H264_MEASURE=1 显式运行，真实正确性测试保留。
- 集成 motion MOV：5秒/60帧、480×270/12fps、GOP12、radius1，完整严格解码、
  PTS/关键帧/PCM与字幕验证通过；原生 FLAC 导入保留长于视觉场景的完整音尾。
  记录 artifacts/media-motion-20261104/provenance.json。
- 集成 scene3d MP4：5秒/40帧、320×180/8fps，GOP8/radius1，40画面均有变化；
  全部解码/PTS/关键帧验证通过，七帧相对原生 PNG 的 RGB PSNR39.939–43.108dB。
  记录 artifacts/scene3d-media-20261104/provenance.json。它不是独立几何 oracle。

## 性能证据的范围

- FLAC CRC16等价查表：同期交错五样本，旧双声道16/24bit解码中位耗时下降
  44.98%/69.58%，分配次数保持；仅512B共享表，完整独立回归见FLAC_PERFORMANCE.md。
- 重复逆弧长查询中位55.402→1.840µs、均零分配，构造成本见 PATH.md。
- motion radius2 指定320×180 pan/static/noise 场景耗时观测下降约27%–30%，
  候选/NAL/重建字节不变。基线同期有父线程 race 负载，不能当系统隔离测量。
- scene3d 指定320×240/4140三角形 Sample 中位 SSAA1约4.56ms、SSAA2约11.75ms。
- 1024条柱状图 SetValues 避免无用深拷贝后，每次分配1628672→782320字节、
  分配次数5221→2107；计时样本与共享主机限制见 BARCHART.md。
- 2D混合帧640×360、单worker，超采样1/2/4倍中位8.87/39.16/152.30ms。
  更高采样换取画质并增加成本，不作为性能加速宣称。
- 历史静态100圆的2.44倍吞吐记录和画质/内存边界原样保留在 PERFORMANCE.md。
  这些工作负载不等同，不组合为整体或相对 Manim 的全库性能优势。

## 继续实现的缺口

本轮五个并行切片均已交付：路径多边形布尔、稀疏矩阵/CG、固定拓扑网格
变形、二项式/多行下标及 IMA ADPCM。父线程已集成 boolean、sparse-solve、
mesh-morph、fractions 示例及 IMA CLI 端到端回归。该检查点全套与 race 已通过。
随后第二批的正负柱状图、四元数/旋转关键帧、SpGEMM、G.711 WAV、公式颜色和
显式2D超采样均已集成，并通过上述整仓检查点。已加入 bar-chart、quaternion、
sparse-product、formula-colors 示例与 CLI 音频/采样回归。
第三批的相机四元数轨道、BiCGSTAB、Microsoft ADPCM WAV、aligned/gathered
多行公式与evenodd裁剪全部已交付并通过上述整仓检查点。新增oriented-camera、
bicgstab、multiline-math、clip-rules集成示例，MS ADPCM CLI支持fact两种顺序、
完整scene时钟、准确样本/remix及无效输入不发布文件。
父线程新增复平面双向仿射映射和复数轨迹，graph全测/race及示例通过，70组映射与
70组逆投影匹配固定Manim参考；完整范围与独立SVG验收见 COMPLEX_PLANE.md。
极坐标平面及径向轨迹已通过graph全测/race、336组固定来源坐标/半径对照及示例
SVG验收；角度范围、偏移/顺逆时针和负半径规则差异见 POLAR_PLANE.md。

仍有曲线布尔运算、完整 SVG 填充/裁剪/滤镜语义、复杂文字塑形/双向/自动字体发现、完整
TeX/Typst、源语义匹配变换、通用 updater/对象生命周期、完整3D物体/相机/光照、
更多运动预测和压缩音频、预览/配置/缓存/工具生态等大量缺口。
详细范围见 PARITY.md 与各专题文档；所有能力族仍为 Partial 或 Not started。

第四批独立和集成证据：

- 角标记64组构造/448个固定Manim变换状态，每个root/dot控制点误差≤2e-12；
  集成SVG三个时刻最大差22/38/44。检查点不覆盖任意3D角或live line updater。
- RK45六组独立SciPy系统、极端尺度/预算/取消/fuzz通过；12轨迹示例实际892接受、
  72拒绝、5796次求值，每个接受点核对解析解。三个SVG时刻最大差20/20/40。
- 多点光70组Decimal数值和288组独立射线像素；旧方向/单点光哈希保持。
  point-lights五秒40帧视频全部严格解码，40画面均不同，PTS/关键帧正确；
  七个native PNG比较RGB PSNR39.449–40.126dB、MAE1.474–1.563。
- AU106流/1478712标量样本与libsndfile完整一致，FFmpeg100流完整一致，其余
  六个为空流且被FFmpeg拒绝。CLI六编码/annotation/EOF/时钟/remix/失败不发布通过。
- 字体回退独立fontTools/Cairo最小IoU .991043；旧单字体控制点/metrics/像素保持。
  font-fallback实际由嵌入三字符数学font补齐缺字，SVG三时刻最大差18/18/20。
- 普通evenodd填充独立Cairo十二组参考最大差23，实际SVG导出独立解释通过；
  fill-rules示例普通渐变孔洞（无Clip）经形变/相机仍正确，SVG最大差18/16/19。
  单个mask、共享工作/点数及stroke仍nonzero的边界见FILL_RULES.md。

四个集成SVG均为640×360、SSAA4，RGBA MAE<.1/255；阈值没有放宽。
最终多光源画面为artifacts/point-lights-wave4-layout3-20261104.png；此前layout1/2
为历史预览，不用于最新源码验收。各demo-svg和media目录均记录源码/二进制在
检查全程未变；媒体/SVG集成共享Go场景构造，不冒充独立几何或Manim真值。
