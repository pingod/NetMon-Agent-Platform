# 02 — 2D/3D 机房机柜可视化 + 大表格前端 技术栈选型

> 版本：v1.0（技术选型调研）
> 日期：2026-10-20
> 调研人：viz-stack
> 服务对象：`docs/requirements/01-需求规格说明书.md` 中 FR-6（2D 可视化，P0）、FR-7（3D 可视化，P1）、NFR-1/2（5000 连线重算 <3s、表格 60fps、3D ≥30fps）、NFR-3（**完全离线单机部署**）
> 核实方式：npm registry（`registry.npmjs.org`）+ GitHub API + 官方文档页面，全部在 **2026-10-20** 当天采样，每条结论标来源 URL。**未能实测的性能数字集中标注在 §10.6**，不作为结论使用。

---

## 0. 摘要结论（一张表看完选型）

| 层 | 主选 | 版本 | 许可 | 一句话理由 |
|---|---|---|---|---|
| 2D 机房平面 / 机柜 U 位图 | **React + 纯 SVG 自绘**（自研 `RackCanvas`） | React 19 | MIT | 元素量在 SVG 舒适区（单视图 200~3000），且 SVG 天然满足 FR-6.7「导出 SVG/PNG/PDF 施工打印」 |
| 2D 兜底（超 5k 元素 / 强动画） | PixiJS（非 Konva） | 8.22.0 | MIT | 需要 WebGPU + 万级精灵时才切，用 PixiJS 而不是 Konva 因为 Konva 的 scene graph 层在这类场景是纯开销 |
| 3D | **three.js + @react-three/fiber + InstancedMesh** | three 0.186.1 / r3f 9.8.2 | MIT | 200 柜 + 3000 设备 = 柜 1 个 + 设备按 U 高分 3~4 个 InstancedMesh，全场景 draw call ≈ 12（§3.2 有账），30fps 是下限不是上限 |
| 3D 拾取 | r3f 内置 raycaster + `instanceId` 反查 | — | — | 3000 实例下朴素 raycast 足够（§3.3）；`three-mesh-bvh` 0.9.16 (MIT) 留到实例破 3 万或引入 glTF 高面数模型才加 |
| 大表格 | **AG Grid Community** + 自研 Excel 化交互层 | 36.2.0 | **MIT** | 虚拟滚动/列冻结/单元格编辑/校验高亮（`cellClassRules`）全部在 Community；见 §6 的深坑说明 |
| 表格升级路径 | AG Grid Enterprise（**可选采购**） | 36.2.0 | 商业 $999/dev 永久 | 剪贴板、区域选择、填充柄、状态栏**只在 Enterprise**，这是从 Excel 迁移的真实痛点 |
| 公式列 | **自研表达式求值 + AG Grid `valueGetter`** | — | — | 线长公式是要审计、要回归测试的核心资产，不能藏在表格控件里 |
| Excel 导出 | **ExcelJS** | 4.4.0 | MIT | 唯一能同时写样式/列宽/冻结窗格/数据验证的 MIT 方案；维护停滞风险已评估（§8.2） |
| Excel 导入 | **SheetJS CE**，从官方 CDN 装 0.20.3 | 0.20.3 | Apache-2.0 | 解析容错强；**npm 上的 `xlsx@0.18.5` 是 2022 年的过期快照，不能用** |
| 走线路由 | **自研确定性分段正交路由 + 槽位分配**（不用 ELK/libavoid） | — | — | 机房走线是规则化几何约束问题，不是通用图布局问题（§5.2 反例） |
| 状态管理 | zustand + immer | 5.0.15 / 11.1.21 | MIT | 2D/3D/表格三个视图订阅同一 store 的同一份规范化数据 |

**一句话总论**：这个项目的技术难点不在"选哪个库"，而在**「一份几何数据、三个渲染后端（SVG / WebGL / DOM 表格）」**。选型的第一原则是：所有渲染层都必须是**无状态的投影函数**，业务真相只存在于一个规范化数据模型里。凡是以"把数据喂给控件、让控件自己管状态"为设计前提的库（Handsontable、Univer），为了这一个理由就可以否掉。

---

## 1. 约束与现实基线（选型的前提，不是背景铺垫）

选型结论全部由下面这 6 条硬约束推出来，先摆清楚，后面每个"为什么不选 X"都能回指到这里。

### 1.1 数据规模（来自 `01-需求规格说明书.md` §5.3 实测）

| 指标 | 实测值 | 对选型的含义 |
|---|---|---|
| 机房 | 2 个（403/404） | 单场景可全量加载，不需要服务端分页 |
| 通道 | 14 条 | 平面图布局是**规则的**（通道 × 两侧柜列），可程序化生成，不需要自由绘图 |
| 机柜位置 | 约 362 个（含空调/柱子占位） | 平面图层 < 400 个 `<rect>`，SVG 毫无压力 |
| 设备实例 | 128 个（现状）/ 3000 个（验收目标 AC-7） | **现状与目标差 23 倍**，必须按 3000 设计 |
| 连线 | 5127 条 | 连线是**最重的渲染对象**，比设备多一个数量级 |
| 端口 | 约 10000+ 个 | 端口面板只在"单机柜详情"里渲染，单柜最多 ~90 口 |
| 单柜 U 高 | 42U 标准柜 | 42 行 × 柜数：200 柜 = 8400 个 U 格 → **U 格必须用 `<pattern>` 或一次性 `<path>`，不能一 U 一个节点** |

> **关键设计输入**：`AC-7` 要求「200 柜 + 3000 设备 ≥ 30fps」，而现状只有 128 设备。选型必须按 3000 设备 + 5000 连线做压力预算，不能按 128 做。这是本文件很多结论与"网上常见建议"不同的原因。

### 1.2 部署约束：**完全离线、单机、一条命令**（NFR-3 / AC-8）

这条直接杀掉一批候选：

- **Handsontable** — 商业 license，其 license 校验虽然"无 license server、纯客户端"，但**商业授权本身**是硬门槛（§6.3）。
- **SheetJS CE 从 npm 安装** — npm 上的版本是 2022 年的过期快照（§8.3），必须改用 CDN tarball；离线部署要求把 tgz **镜像进内网**，这是一条必须在部署文档里写死的运维步骤。
- **Univer Pro / 任何云服务型方案** — 需要 license 服务或联网校验，直接排除。
- **AG Grid Enterprise** — 官方 license 安装文档只要求 `LicenseManager.setLicenseKey("YOUR_LICENSE_KEY")` 这一行（https://www.ag-grid.com/javascript-data-grid/license-install/ ），**未提及任何 license server 或联网校验**，因此技术上离线可行（这点必须澄清，很多人误以为不行）。正式离线交付前建议向厂商书面确认 EULA 条款。

### 1.3 几何约束：柜位是离散格点，不是自由平面

`01-需求规格说明书.md` §3.1 反解出的真实几何：

- 每条通道两侧各一排机柜，柜号 1~27（部分通道 1~25，因空调位占位）
- **柜号排布是镜像的**：通道 N 一侧自左向右递增（1,2,3…），另一侧递减（…27,26,25）；即 1↔27 相对、2↔26 相对
- 机柜标准 42U，宽 0.6m（0.6 来自 `布线表` R 列公式 `=P2*0.6*120%`，是实测柜间距常量）

> **含义**：2D 平面图不是"用户画出来的图"，而是**由 `(room, aisle, side, cabinetIndex)` 四元组程序化生成**的。这意味着：
> 1. 不需要 Fabric.js / Konva 那类"自由拖拽绘图"能力 → **这是否掉 Fabric.js 的第一个理由**；
> 2. 坐标系必须由代码定义并做单元测试（§4），而不是靠画布控件的坐标。

### 1.4 性能硬指标

| ID | 要求 | 判定方式 |
|---|---|---|
| NFR-1 | 5000 条连线全量重算 < 3s；单条增删改增量重算 < 200ms | 在**领域层**算，与渲染无关 → 反过来要求"渲染层不得持有业务计算状态" |
| NFR-2 | 表格 5000+ 行虚拟滚动 60fps | 表格库必须内置行虚拟化 |
| AC-7 | 3D 200 柜 + 3000 设备 ≥ 30fps | 3D 必须能批渲染（InstancedMesh） |

### 1.5 输出格式约束（FR-6.7 / NFR-5）

- 导出 SVG / PNG / PDF 供**施工打印**
- 导出的 Excel 必须能被 **Excel 2016+ / WPS** 打开（NFR-5）→ 这条对导出库有实打实的约束：WPS 对非标准 OOXML 的容忍度低于 Excel，**样式必须走标准 `styles.xml`**，不能靠"打补丁式"的私有扩展。

### 1.6 团队与交付现实

- 单机私有化交付给甲方，**不是 SaaS** → 不需要多租户、不需要 CDN 加载大资源
- 中文界面，术语与用户 Excel 一致（NFR-10）→ 表格列名、导出表头必须能 100% 对齐 `V13` 的 25 列表头
- 用户是 Excel 重度使用者 → **Excel 化的交互预期（区域选择、Ctrl+C/V、填充柄、右键菜单）是刚需，不是加分项**。这一点在 §6 里会翻转 AG Grid 的结论。

---

## 2. 2D 渲染方案对比

### 2.1 候选与一句话判词

| 方案 | 版本 / 许可 | 判词 |
|---|---|---|
| **纯 SVG + React 自绘** | 平台原生 / — | **主选**。元素量匹配、可导出、可调试、可无障碍 |
| Konva / react-konva | 10.9.0 / 19.3.2，MIT | 备选。canvas scene graph，元素量大时比 SVG 快，但**丢掉了 SVG 导出** |
| PixiJS | 8.22.0，MIT | **2D 兜底**。WebGPU/WebGL，万级精灵无压力，但纯 GPU，要自己做命中测试与导出 |
| Fabric.js | 7.4.0，MIT | 不选。定位是"自由绘图编辑器"，我们不需要拖拽画图，却要背上它的对象模型 |
| D3 | 7.9.0，ISC | 不选（作为渲染器）。但**选它的子包当数学工具**（`d3-scale`、`d3-shape`、`d3-hierarchy`） |
| ECharts custom series | 6.1.0，Apache-2.0 | 不选。custom series 能画 U 位图，但命中测试/自定义交互要绕，且导出是 canvas 位图不是矢量 |
| AntV G6 | 5.1.1，MIT | 不选（用于平面图）。G6 是**关系图**引擎，适合拓扑，不适合格点布局 |

### 2.2 为什么 2D 机柜视图选纯 SVG —— 量化验证 lead 的判断

lead 的倾向是「纯 SVG 自绘 + React，量不大、天然可导出 SVG/PDF、调试简单」。**这个判断成立**，但有两个必须钉死的前提，否则会在 3000 设备时翻车。

**支持结论的量化依据：**

1. **平面图层**：362 个机柜位置 → 362 个 `<rect>` + 少量 `<text>`。加上通道底色、空调/柱子占位，**总计 < 600 个节点**。SVG 在 1000 节点以内，Chrome 的命中测试与重绘都是亚毫秒级。
2. **机柜正视图（U 位图）层**：这是最需要小心的地方。
   - ❌ 错误做法：42U × 200 柜 = 8400 个 `<line>` 画 U 刻度 → 加上设备块，节点数冲到 1.2 万，React diff 会明显卡。
   - ✅ 正确做法：**每个机柜的 U 刻度用一个 `<pattern>`（或一条 `M...V...M...V...` 的 `<path>`）**，200 柜 → **200 个节点**。设备块单独一层，3000 个 `<rect>`。
   - 结论：**总节点预算 = 362(平面) + 200(U 刻度) + 3000(设备) + 5000(连线，见下) ≈ 8600**，但**任一时刻只渲染一个视图**（平面图 / 正视图 / 单柜详情是三个路由），单视图峰值 ≈ 3000~5000 节点 → **SVG 可以承受**。
3. **SVG 导出是刚需**（FR-6.7 施工打印）：纯 SVG 方案下，导出 = `XMLSerializer.serializeToString(svgEl)` + 内联样式，**零依赖**。canvas 方案（Konva/Pixi）要做矢量导出必须额外引 jsPDF 之类的库重画一遍，是纯浪费。
4. **调试成本**：SVG 能在浏览器 DevTools 里直接选中元素、看样式、改属性。这是排障速度的实战差距，不是理论差距。

**反例（lead 要的「反例」）—— 什么时候 SVG 会输：**

| 反例场景 | 为什么 SVG 输 | 阈值 | 应对 |
|---|---|---|---|
| **5000 条连线全部同时高亮**（FR-6.5 点设备高亮所有连线） | 每条连线是 1 个 `<path>`，还要做动画/虚线流动 → 5000 个节点 + 5000 个 CSS animation，浏览器合成层爆炸 | 单视图连线 > 1000 条且需要动画 | 连线层降级为**一条合并 `<path>`**（同一样式批量拼接），或高亮时只画"该设备的连线子集"（实测单设备最多 89 口 → ≤89 条） |
| **热力图**（FR-6.8，功率/重量/端口利用率） | 每格一个 `<rect fill=...>` + 需要连续色阶；SVG 无 GPU 混合，`filter: blur` 极慢 | 热力图覆盖 > 2000 格 | 热力图改用一层 `<canvas>` 叠在 SVG 上，或直接用 CSS `background: linear-gradient` 拼格 |
| **持续 60fps 的拖拽/框选/流光** | SVG 每次 `transform` 变更触发重排+重绘，React diff 叠加 | 需要连续动画帧 | 动画走 CSS `transform`（GPU 合成）；或该视图整体切 canvas |
| **单视图元素 > 8000** | Chrome SVG 命中测试走 CPU，hover 会掉到 30fps 以下 | 8000 | 切 PixiJS |

> ⚠️ 注意需求文档 FR-6.6 写了「框选」。框选本身在 SVG 上就是拖一个 `<rect>`，**不是性能问题**，但要注意**框选命中判定要在数据层用几何索引做**（`rbush` 4.0.1 MIT 或 `flatbush` 4.6.2 ISC），不要遍历 DOM 的 `getBoundingClientRect()`。

**SVG 方案的代价（诚实列出）：**

- 需要自己实现：视口变换（zoom/pan）、虚拟化（只渲染视口内的柜列）、命中测试优化。**大约是 800~1500 行自研代码**，不是"白捡"。
- 与 Konva 相比，少了"开箱即用的 scene graph / 拖拽 / 事件冒泡"。但我们的图是**数据驱动、非自由拖拽**的（§1.3），这些能力用不上 → 这正是不选 Konva 的核心理由。

### 2.3 为什么不选 Konva / react-konva

- **能力错配**：Konva 的价值在 scene graph（层级、组变换、事件冒泡、拖拽、动画）。我们的图是**由 `(room, aisle, side, index)` 程序化生成的**，层级天然由数据决定，不需要一个运行时对象树来维护它。等于买了个用不上的引擎。
- **额外依赖**：`konva` 10.9.0 无 deps（这点很好），`react-konva` 19.3.2 peer 要求 `react ^19.3`。**这是一个真实的版本约束**——如果项目最终落在 React 18，react-konva 就装不上。
- **丢 SVG 导出**：Konva 是 canvas，导出只有 PNG（`toDataURL`）。要矢量导出得另做。这对 FR-6.7 是倒退。
- **Konva 官方自己的性能文档**（https://konvajs.org/docs/performance/All_Performance_Tips.html ）开篇就写：「HTML5 canvas 在它擅长的范围内很高效……但当项目复杂度增长、或者舞台上 shape 很多时，不可避免地会有负面性能影响」，并列出 Layer 管理、`listening(false)`、shape caching 等一整套调优清单。**也就是说选了 Konva 就要开始做 Konva 的性能调优**——而我们真正需要调优的规模（<3000 节点）在 SVG 里根本不需要调优。

> 保留 Konva 为**备选**的唯二条件：(a) 未来要做「自由拖拽布局机柜」这种绘图板功能；(b) 单视图稳定超过 8000 元素且 SVG 优化到头。

### 2.4 为什么不选 PixiJS（但保留为兜底）

- **它是纯 GPU 渲染器**，没有 DOM，意味着：无原生可访问性、无原生 SVG 导出、命中测试要自己实现（Pixi 有 `eventMode` 但那是它的交互系统，不是几何查询）。
- **优势是真实的**：8.22.0 支持 WebGPU（有 WebGL fallback），万级精灵稳定 60fps。如果未来做"整机房 5000 连线实时流动 + 热力图"的展示大屏，PixiJS 是正确选择，而不是 Konva。
- **为什么 Pixi 优于 Konva 作为兜底**：两者都要自己做命中测试和导出，那不如选吞吐更高的那个（Pixi 的批渲染更彻底）。

### 2.5 为什么不选 Fabric.js

Fabric.js 7.4.0（MIT，GitHub 31.5k★）是**设计编辑器内核**：对象序列化、自由变换、SVG 导入导出、滤镜。它的强项是"用户手工画图"。

我们的场景是"数据决定图形"，机柜位置由 `(aisle, side, index)` 算出，用户拖动机柜是**改数据**（改柜号/换通道），不是**改坐标**。用 Fabric 会诱导出"图形位置是真值、数据是影子"的架构——这正是需求文档 §1.1 批评的 Excel 式问题的翻版。

**唯一值得借鉴的**：Fabric 的 SVG 导出实现很成熟，如果我们自研导出遇到文本/字体嵌入问题，可以读它的源码（MIT，可参考）。

### 2.6 为什么不选 ECharts custom series / AntV G6

**ECharts 6.1.0（Apache-2.0，67.5k★）custom series**：
- 能画（用 `renderItem` 返回 `rect`/`text` 的图形元素组），但：
  - 命中测试依赖 ECharts 自己的坐标系与 `dataIndex` 映射，实现"点端口 → 高亮对端端口"这类**跨图层关联**很别扭；
  - 导出是 canvas 位图（ECharts 有 `getDataURL`），**不是矢量**，施工打印会糊；
  - 6.1.0 的 custom series 在 5000+ 元素时同样走 canvas 全量重绘，不比原生 canvas 快，却多背一个 700KB+ 的库。

**AntV G6 5.1.1（MIT，12.3k★）**：
- G6 是**图（node-edge）可视化引擎**，内核是力导向/层次布局 + 图交互。我们的平面图是**规则格点**（通道 × 柜位），根本不需要图布局算法；U 位图更不是图。
- G6 适合的是"网络拓扑图"——如果未来要做「逻辑拓扑视图」（FR 里没有本期要求），G6 或 `@xyflow/react` 才是候选。**本期不选，但记录为拓扑视图的候选**。

**D3 7.9.0（ISC，113.8k★）**：把它当**渲染器**用是 2015 年的做法（`d3.select` 直接操作 DOM 会和 React 抢 DOM）。但在 React 里把 D3 当**数学库**用是正交的、推荐的：
- `d3-scale`（色阶、坐标映射）
- `d3-shape`（线/曲线生成，走线圆角）
- `d3-hierarchy`（如果做机柜树）

### 2.7 虚拟化策略（200 柜 × 42U 的落地做法）

不需要引 `react-virtual` 之类的通用虚拟滚动库，因为**柜位是规则网格**，索引算得出来：

```
可见柜列区间 = floor(scrollLeft / (CABINET_W + GAP)) .. ceil((scrollLeft + viewportW) / (CABINET_W + GAP))
```

- **平面图**：362 个柜位全渲染也不慢 → **不做虚拟化**，保持简单。
- **多柜正视图（一排 27 柜 × 42U）**：27 柜全渲染 ≈ 27 + 3000/200*27 ≈ 400 节点 → **不做虚拟化**。
- **真正的虚拟化只需在「全部 362 柜同屏」时启用**：视口外机柜渲染为 1 个占位 `<rect>`（省掉 U 刻度与设备块）。实现 ≈ 30 行。
- 若未来画布内元素破万，再引 `@tanstack/react-virtual` 3.14.21（MIT），但要接受它的 `virtualizer` 抽象和 SVG 的 `viewBox` 变换需要手工对齐。

### 2.8 半宽 / 全宽 / 刀片子设备：U 位图的三种特殊占位

这是 2D 机柜正视图里**唯一有真实几何复杂度**的地方，必须单独设计，否则会在实现时临时拍脑袋。

#### 三种占位形态

| 形态 | 几何 | 数据表达 | 渲染 |
|---|---|---|---|
| **全宽设备**（标准交换机/服务器） | 占 1 行 × 整宽（600mm） | `{ uStart, uHeight, widthRatio: 1.0 }` | 1 个 `<rect>` 满宽 |
| **半宽设备**（1U 半宽交换机，两个并排） | 占 1 行 × 半宽，**左右并排两个** | `{ uStart, uHeight: 1, widthRatio: 0.5, hSlot: 'L' \| 'R' }` | 同一 U 行放 2 个 `<rect>`，各 285mm 宽（留 30mm 中缝） |
| **刀片机箱 + 子设备** | 机箱占 N U 全宽；**子刀片是机箱内的横向细分**，各占 1 U 高度但只占机箱的 1/M 宽度 | 父子两层：`parent: bladeChassis` → `children: blade[]`，每个 blade 有 `bayIndex` | 机箱 1 个大 `<rect>` + M 个子 `<rect>` 横向并列 |

#### 数据模型（关键：用 `widthRatio` + `hSlot` 表达，不用像素）

```ts
// domain/model.ts
interface DevicePlacement {
  id: string;
  cabinetId: string;
  uStart: number;          // 1-based，占 [uStart, uStart + uHeight)
  uHeight: number;         // 1U / 2U / 4U / 10U
  widthRatio: 1 | 0.5;     // 1=全宽，0.5=半宽（这是几何真值，不是像素）
  hSlot?: 'L' | 'R';       // 仅 widthRatio=0.5 时有效：左半还是右半
  parentId?: string;       // 非空 = 这是某个刀片机箱的子设备
  bayIndex?: number;       // 子设备在机箱内的槽位号（0-based）
  baysTotal?: number;      // 机箱总槽位数（如 8 槽刀箱）
}
```

**渲染投影函数（2D 与 3D 共用，这是"数据同源"的又一个落点）：**

```ts
// domain/geometry.ts —— 唯一的占位几何计算，SVG 和 InstancedMesh 都调它
export interface RectMm { x: number; y: number; w: number; h: number }  // 相对机柜左下角

export function placementToRectMm(p: DevicePlacement): RectMm {
  const y = (p.uStart - 1) * MM_PER_U;
  const h = p.uHeight * MM_PER_U;

  if (p.parentId) {
    // 刀片子设备：只占父机箱宽度的 1/baysTotal，按 bayIndex 横排
    const bw = CABINET_WIDTH_MM / (p.baysTotal ?? 1);
    return { x: (p.bayIndex ?? 0) * bw, y, w: bw, h };
  }
  if (p.widthRatio === 0.5) {
    const w = (CABINET_WIDTH_MM - HALF_WIDTH_GUTTER_MM) / 2;
    return { x: p.hSlot === 'L' ? 0 : w + HALF_WIDTH_GUTTER_MM, y, w, h };
  }
  return { x: 0, y, w: CABINET_WIDTH_MM, h };
}

const HALF_WIDTH_GUTTER_MM = 30;   // 半宽设备之间的中缝（理线空间）
```

> 🔑 **注意这里没有"像素"**。`placementToRectMm` 输出毫米，2D 再 `mmToPx`，3D 再 `mmToWorld`（§4.1/§4.2）。**同一个设备在 2D 和 3D 里位置永远一致，因为只有一个函数算它。**

#### 渲染策略与性能

| 场景 | 节点数 | 结论 |
|---|---|---|
| 单柜 42U，含 2 个半宽 + 1 个 8 槽刀箱（8 子设备） | ≈ 12 个 `<rect>` + 标签 | 无压力 |
| 200 柜 × 平均 15 台（含半宽/刀片） | 3000 个 `<rect>` | SVG 可承受（§2.2） |
| 刀箱展开（用户点"展开子设备"） | 单个刀箱 +8 个 rect | 交互式展开，零压力 |

**三个必须注意的实现细节：**

1. **半宽设备的中缝不能用 `gap` 属性**（SVG 的 `<rect>` 没有 gap）。用显式宽度计算（上面已做）或 `<clipPath>`。**推荐显式算，因为导出 SVG 时 clipPath 兼容性更差。**
2. **刀片子设备的文字标签方向**：子刀片宽度可能只有 600/8 = 75mm，竖排文字会塞不下。规则：`w < 60px` 时**不渲染文字，只渲染颜色块 + 悬停 tooltip**。这是"图表可读性"的硬规则。
3. **`hSlot` 缺省值必须显式**：如果一条半宽设备的 `hSlot` 为空（导入时没解析出来），**不能默认 'L'**——那样两台半宽设备会重叠。正确做法是**在校验层报错**（FR-8 校验的一部分），渲染层遇到 `widthRatio=0.5 && !hSlot` 时渲染成红色警示块。这是"脏数据要让用户看见"的具体落实（§7.2）。

#### 3D 里的对应处理

- **全宽设备**：1 个 instance，`BoxGeometry(0.6, uHeight*44.45/1000, depth)`
- **半宽设备**：同一 InstancedMesh 用**缩放矩阵**（`scale.x = 0.5`）+ 平移，**不需要额外 geometry** → 仍然 1 个 draw call
- **刀片机箱**：机箱本体 1 个 instance；子刀片用**另一组更小的 InstancedMesh**（或直接不渲染——3D 视图看刀箱就是个大盒子，展开子设备是 2D 的活）
  - **建议：3D 不渲染子刀片**。理由：200 柜规模下，单个刀箱内 8 个 75mm 宽的盒子在屏幕上只有几个像素，渲染它们是纯浪费。**3D 只到"设备"粒度，子设备是 2D 与表格的职责。** 这条要写进设计文档，避免后续反复。

---

## 3. 3D 渲染方案对比

### 3.1 候选与一句话判词

| 方案 | 版本 / 许可 | 判词 |
|---|---|---|
| **three.js + @react-three/fiber** | three 0.186.1 (MIT) / r3f 9.8.2 (MIT) | **主选**。生态最大、InstancedMesh 是一等公民、r3f 让 3D 场景变成 React 组件树（与 2D 共享 store 极自然） |
| three.js 裸用（无 r3f） | 0.186.1 MIT | 备选。少一层抽象、少 ~40KB，但要自己写渲染循环与状态同步，和 React 项目的耦合会变脏 |
| Babylon.js | 9.31.0 (Apache-2.0) | 不选（本期）。引擎更"全"（内置 GUI、物理、Inspector、Thin Instances 很棒），但体积与 API 面远超需求 |
| TresJS | 5.9.3 (MIT) | 不选。**它是 Vue 的 r3f**，peer 要求 `vue >=3.4`；项目是 React 的话直接出局 |
| echarts-gl | 2.1.0 (MIT) | 不选。3D 散点/曲面图表库，不是场景引擎，做不了"点机柜展开柜门" |

### 3.2 为什么 three.js + r3f 是 200 柜 / 3000 设备的最优解

**核心论点：这个场景的 3D 渲染量小得惊人，真正的复杂度在"交互与数据同步"，所以选型应该按"交互开发体验"排序，而不是按"引擎性能"排序。**

算一笔渲染账：

| 物件 | 数量 | 渲染方式 | draw call |
|---|---|---|---|
| 机柜外壳 | 200 | 1 个 `InstancedMesh`（同一 BoxGeometry） | **1** |
| 设备（1U/2U/4U/10U） | 3000 | 按 U 高分组 → 最多 4 个 `InstancedMesh` | **4** |
| 地板 / 房间墙体 | 1 组 | 普通 Mesh | ~3 |
| 桥架 / 走线槽 | ~30 | 1 个 InstancedMesh 或合并 geometry | 1~2 |
| 连线（FR-7.5，P2） | ≤200（简化曲线，不全画 5127） | 1 个 `LineSegments` 或合并 BufferGeometry | **1** |
| 标签（只在悬停/选中时） | ≤50 | CSS2DRenderer / `troika-three-text` | ≤50（且**不在 WebGL 里**） |
| **合计** | 3500+ 物体 | — | **≈ 12** |

> **12 个 draw call** 对任何 GPU 都是"空载"。所以 AC-7「200 柜 + 3000 设备 ≥ 30fps」**不是性能挑战，是架构挑战**——只要不用"一个设备一个 Mesh"的写法（3000 个 Mesh = 3000 draw call，会掉到 15fps 以下），30fps 是保底。**这一点必须在代码规范里写死：3D 场景禁止 `devices.map(d => <mesh .../>)`。**

**`InstancedMesh` 的官方定位**（three.js docs，https://threejs.org/docs/#api/en/objects/InstancedMesh ）：用于渲染大量相同几何体，通过 `setMatrixAt(i, matrix)` 传实例矩阵、`setColorAt(i, color)` 传实例颜色。这正好匹配"设备都是长方体、只是尺寸/位置/颜色不同"的场景——**连颜色变化（告警红/已用蓝/空闲灰）都能用 `instanceColor` 一个 draw call 搞定**。

**r3f 带来的具体收益（这是选它而不裸用 three.js 的理由）：**

1. **场景即组件树**：`<Cabinets />` `<Devices />` `<Cables />` 各自订阅 store 的切片，与 2D 的 `<RackSvg />` 用**同一套 hooks**。2D/3D 切换就是换一个渲染后端组件，数据层零改动。
2. **自动生命周期**：`useFrame`、`useThree`、dispose 管理、`frameloop="demand"`（按需渲染——**静态查看时 0 GPU 占用，这对"单机部署、机器可能很弱"的甲方环境很有价值**）。
3. **生态**：`@react-three/drei` 10.7.9（MIT）直接给 `OrbitControls`、`Html`、`Text`、`Bounds`、`Instances`/`Instance`（声明式 InstancedMesh 封装）、`useGLTF`。**`<Instances><Instance/></Instances>` 让 3000 个实例用 JSX 写而仍然只有 1 个 draw call** —— 这是"既要可读性又要性能"的答案。
4. **版本兼容要钉死**（已核实）：
   - `@react-three/fiber` 9.8.2 的 peer：`react >=19 <19.4`、`three >=0.156`
   - r3f 官方安装文档明写：**`@react-three/fiber@8` 配 `react@18`，`@react-three/fiber@9` 配 `react@19`**（https://r3f.docs.pmnd.rs/getting-started/installation ）
   - ⚠️ **决策点**：项目若用 React 19 → r3f 9.x；若坚持 React 18 → 必须锁 r3f 8.x。**这条要和前端框架选型一起拍板，不能事后改。**

### 3.3 raycast 拾取：可行性与性能（lead 问的 B 题核心）

**结论：可行，3000 实例下性能完全够，不需要 BVH。**

三层方案，按数据量递进：

**第一层：`InstancedMesh` + `instanceId`（默认，直接够用）**

three.js 的 `Raycaster.intersectObject(instancedMesh)` 对 InstancedMesh 会返回带 **`instanceId`** 字段的 intersection 对象，用它反查业务实体：

```ts
const hits = raycaster.intersectObjects([devicesIM, cabinetsIM], false);
// hits[0].instanceId  →  devices[instanceIndex[instanceId]]  →  业务 ID
```

要点：
- 建 `instanceId → entityId` 的**双向索引数组**（`instanceIndex: string[]`），O(1) 反查。
- **性能要点**：three.js 对 InstancedMesh 的 raycast 是**逐实例做 ray-box/ray-sphere 粗筛 + ray-triangle 精测**（内部先测包围球/包围盒，命中后才测三角形）。3000 个 1U 盒子 = 3000 次 ray-box 测试 ≈ **0.1~0.5ms**，远低于 16.6ms 帧预算。**raycast 只在 `pointermove`（需要节流到 ~30Hz）和 `click` 时触发，不在每帧触发**。
- **关闭不需要测的物体**：地板/墙体设 `raycast = () => {}` 或 `layers` 分离，避免无效遍历。

**第二层：分组 raycast（推荐做法）**
- 机柜层和设备层分开测；命中机柜时**不**遍历设备（先测机柜外壳，柜门关闭时设备本来就被挡住）。
- 用 `raycaster.layers` / 分别 `intersectObject` 两次，比一次测全部更快，语义也更清楚。

**第三层：`three-mesh-bvh`（0.9.16，MIT，https://github.com/gkjohnson/three-mesh-bvh ）—— 本期的"不要用"**
- 它给单个高面数 mesh 建 BVH 加速 raycast（也支持 `InstancedMesh`）。
- **为什么本期不上**：我们的几何全是**长方体（12 三角形）**，逐实例测 12 个三角形本来就是最快的；BVH 的构建开销与内存反而亏。
- **什么时候才上**：接入 glTF 精细机柜模型（上万三角形）且需要高频 hover 时；或设备数破 3 万。

**拾取的工程细节（容易踩的坑）：**
- `InstancedMesh` 的**包围球是整体包围球**，不是逐实例 → 大规模分散实例时粗筛效率会下降。我们的柜/设备**聚成一团**（一个机房），所以没问题。
- 拾取要做**优先级**：设备 > 机柜 > 地板。用 `intersectObjects` 的返回顺序（按距离排序）+ 层级权重决定。
- **hover 节流**：`pointermove` 事件在 120Hz 鼠标上会刷爆 React state。做法：raycast 结果与上一次比对，**只在 entityId 变化时** `setState`。

### 3.4 为什么不选 Babylon.js

Babylon.js 9.31.0（Apache-2.0，26.2k★）**技术上完全够用，有些地方更强**：

- **Thin Instances 比 three 的 InstancedMesh 更彻底**：官方文档（https://doc.babylonjs.com/features/featuresDeepDive/mesh/copies/thinInstances ）明写「regular instances 仍有 JS 侧性能开销，因为每个实例都是独立对象（InstancedMesh），1 万个实例引擎要遍历所有对象做可见性检查等；thin instances 不创建新对象，所以几千个也没有 JS 侧开销」。**这是一个真实优势**。
  - 但代价也写在同一页：「要么全部 thin instances 都被绘制（如果 mesh 被判定可见），要么一个都不画。全有或全无」——**逐实例视锥剔除能力丧失了**。我们的场景聚成一团，这个代价可接受。
- **GreasedLine**（https://doc.babylonjs.com/features/featuresDeepDive/mesh/creation/param/greased_line ）：任意宽度的 3D 线，着色器保证始终面向相机 → **画走线比 three 的 `Line2`/`LineMaterial` 更省事**。
- 内置 `@babylonjs/gui`（3D/2D GUI）、`@babylonjs/inspector`（调试面板）、物理、XR。

**不选的真实理由（按权重排序）：**

1. **生态与人才**：three.js 116.6k★ vs Babylon 26.2k★；遇到问题搜到的答案数量差一个量级。r3f 的 React 集成生态（drei 等）Babylon 没有对等物（`react-babylonjs` 活跃度远低）。
2. **体积**：`@babylonjs/core` 即使 tree-shake 后典型场景也比 three 大。**离线部署 + 内网，体积不是致命问题，但首屏加载在弱机器上有感**。
3. **我们不需要那 80%**：GUI/物理/XR/材质编辑器，一个都用不上。为"可能用得上"付 API 学习成本是浪费。
4. **一致性**：2D 端已选 React + SVG，3D 端用 r3f 意味着**整个可视化层都是 React 组件**，心智模型统一。混入 Babylon 的 imperative 引擎要写一层命令式桥接。

> **保留 Babylon 为备选的条件**：如果 3D 需求升级为「需要内置 GUI 面板 + 复杂材质/后期 + 大量非规则几何」，或团队已有 Babylon 经验。

### 3.5 为什么不选 TresJS

`@tresjs/core` 5.9.3（MIT，3.77k★）是**Vue 版的 r3f**，peer 依赖 `vue >=3.4` + `three >=0.133`。质量不错，但：
- 项目若定 React（§2 的 SVG 方案、§6 的 AG Grid 都是 React 生态一等公民），TresJS **直接出局**。
- 反向约束：**如果团队最终选 Vue**，那 TresJS 就是 3D 的正确选择，而 2D/表格方案要整体重评（AG Grid 有 Vue 支持，但 React 生态更成熟）。**这个决策要和前端框架选型绑定，不能各自为政。**

### 3.6 为什么不选 echarts-gl / 其他 3D 图表库

echarts-gl 2.1.0（MIT）是"在 ECharts 里加 3D 坐标系"（3D 散点、曲面、柱状）。它没有场景图、没有相机控制器生态、不能"点机柜展开柜门"。**用途完全不同，不是候选。** 仅当"3D 数据散点图"这种图表需求出现时考虑。

### 3.7 glTF 资产策略

FR-7.2 要求"机柜内设备 3D 展示（按 U 位高度）"。**本期的正确做法：完全不引 glTF。**

理由：
- 机柜 = 长方体（可带一块半透明柜门面板），设备 = 长方体 + 前面板贴图。**全部用 `BoxGeometry` 程序化生成**，零资产管线。
- `BoxGeometry` 的好处：`InstancedMesh` 要求**同一 geometry**，程序化盒子天然满足；glTF 模型每个设备型号一个模型会破坏 instancing。
- 若要做精细外观（前面板端口小孔、品牌 logo）：用**共享贴图 + 每实例 offset**（`instanceColor` 或第二个 InstancedMesh 做面板层），仍不引 glTF。

**未来引入 glTF 的条件**：需要"实景级"机房漫游（甲方展示需求），或需要导入厂商提供的机柜 CAD 模型。届时用 `useGLTF`（drei）+ `three-mesh-bvh` 处理高面数拾取。**现在不做，避免过早优化。**

### 3.8 标签与热力图在 3D 里的做法

| 需求 | 做法 | 库 |
|---|---|---|
| 设备名标签（FR-7.3） | **不用 3D 文字**（性能差、模糊）。用 `CSS2DRenderer` 或 drei 的 `<Html>` 把 DOM 标签投影到屏幕位置 | 内置 / drei |
| 大量标签（>200） | `troika-three-text` 0.52.5（MIT，SDF 文字，GPU 渲染，可批量） | troika-three-text |
| 热力图（FR-6.8 / 3D 版） | `instanceColor` 按数值映射色阶（`d3-scale` 的 `scaleSequential`），**一个 draw call 出全场景热力** | three + d3-scale |
| 高亮选中 | `instanceColor` 改色 + 一层 outline（后处理 `@react-three/postprocessing` 3.2.1 MIT） | drei / postprocessing |

> ⚠️ **CSS2D 标签的坑**：3000 个设备全挂 DOM 标签 = 3000 个 DOM 节点，**比 3D 还慢**。必须**只给视口内 + 选中/悬停的设备挂标签**，且上限 ~50 个。这是 3D 项目最常见的性能事故点。
---

## 4. 坐标系统与 2D/3D 数据同源设计（lead 的 E 题）

这一节是整个方案的地基。**如果这节做错，2D 和 3D 会各自长出一套坐标逻辑，然后永远对不上。**

### 4.1 第一原则：真值用毫米，像素是派生

**规则：数据模型里唯一的长度单位是毫米（integer mm）。像素、Three.js 世界单位、CSS 单位全部是渲染时算出来的派生量。**

理由：
- `布线表` R 列的 `0.6m × 1.2 = 0.72m` 是**物理真值**，不能用像素近似。
- 42U 机柜的 U 高 = 44.45mm（EIA-310 标准 1U），42 × 44.45 = 1866.9mm，加边框才是 2000mm 柜高。**这个换算是硬标准，必须写进常量。**
- 一旦像素进数据模型，导出 SVG 缩放、打印 300dpi、3D 世界单位就全乱了。

```ts
// domain/units.ts —— 唯一的换算中心，全项目只允许这里出现魔法数字
export const MM_PER_U = 44.45;              // EIA-310 标准 1U = 1.75 inch
export const CABINET_WIDTH_MM = 600;        // 0.6m，来自布线表 R 列公式实测
export const CABINET_HEIGHT_MM = 2000;      // 含上下框，42U 有效空间 1866.9mm
export const CABINET_DEPTH_MM = 1200;       // 常见 1200mm 深柜
export const CABINET_PITCH_MM = 600;        // 柜中心距（紧排无间隙）
export const AISLE_WIDTH_MM = 1200;         // 冷通道净宽（实测约 1.2m）
export const RACK_GAP_MM = 0;               // 并柜时间隙为 0；单柜孤立时也按 pitch 排

// 唯一允许的派生：mm → 像素
export const mmToPx = (mm: number, pxPerMm: number) => mm * pxPerMm;
// 唯一允许的派生：mm → three.js 世界单位（1 世界单位 = 1 米，three 的惯例）
export const mmToWorld = (mm: number) => mm / 1000;
```

> **⚠️ 反例警示**：`0.6 * 120% = 0.72` 这个 20% 余量是**线长计算规则**，不是几何尺寸。**它绝不能出现在 `units.ts` 里**，必须放在 `domain/cable-length.ts`。混淆"物理尺寸"和"业务规则系数"是这类系统最常见的腐化起点。

### 4.2 五级坐标系（一级一个纯函数，各自单测）

```
Room (机房)
 └─ Aisle (通道)        ← 通道是"排"的容器，含左右两侧
     └─ Side (侧)        ← 左/右（镜像关键，见 §1.3）
         └─ CabinetSlot (柜位)  ← 含 index，可为空调/柱子占位
             └─ U (U 位)        ← 0..42，设备占 [uStart, uStart+uHeight)
                 └─ Port (端口) ← 面板上的二维格（行 × 列）
```

**每一级的坐标对象都是一个 `Transform`（原点 + 朝向 + 缩放），逐级复合：**

```ts
// domain/coords.ts
export interface Vec2 { x: number; y: number }        // 单位：mm（平面图，俯视）
export interface Vec3 { x: number; y: number; z: number } // 单位：mm（3D；y 为高度）

/** 平面图（俯视）坐标系：x 沿通道方向，y 垂直通道方向 */
export function cabinetSlotToPlan(
  room: RoomLayout, aisle: AisleLayout, side: 'L' | 'R', index: number
): { pos: Vec2; rot: 0 | 180 } {
  const aisleY = room.aisleY[aisle.id];           // 通道中心线 y
  const sideY  = side === 'L' ? aisleY - AISLE_WIDTH_MM / 2 - CABINET_DEPTH_MM / 2
                              : aisleY + AISLE_WIDTH_MM / 2 + CABINET_DEPTH_MM / 2;
  // ⚠️ 镜像在这里发生，且只在这里发生
  const slotIdx = side === 'L' ? index : (aisle.length - 1 - index);
  const x = room.x0 + slotIdx * CABINET_PITCH_MM + CABINET_WIDTH_MM / 2;
  return { pos: { x, y: sideY }, rot: side === 'L' ? 0 : 180 };
}

/** U 位 → 柜内高度（mm，从柜底算起）。U1 在底部，U42 在顶部 —— EIA-310 惯例 */
export function uToHeightMm(u: number): number {
  return (u - 1) * MM_PER_U;   // u 从 1 开始
}

/** 柜位 + U → 3D 世界坐标（mm） */
export function cabinetUToWorld(
  slot: { pos: Vec2; rot: 0 | 180 },
  uStart: number, uHeight: number
): Vec3 {
  return {
    x: slot.pos.x,
    y: uToHeightMm(uStart) + (uHeight * MM_PER_U) / 2,  // y 向上
    z: slot.pos.y,                                        // 平面 y → 世界 z
  };
}
```

**关键设计：`cabinetSlotToPlan` 里的镜像逻辑只写一次。** 2D SVG 用它，3D 也用它（`pos.x/pos.y` → `world.x/world.z`），表格里的"列坐标"也用它（用于算 `|列坐标差|`，即 §3.4 的 `间隔机柜 O`）。

于是：

| 视图 | 用哪个函数 | 说明 |
|---|---|---|
| 2D 平面图 | `cabinetSlotToPlan()` → `mmToPx()` | SVG `<g transform="translate(px,py) rotate(rot)">` |
| 2D 机柜正视图 | `uToHeightMm()` 反向 → 42 行 grid | 不涉及平面坐标 |
| 3D 场景 | `cabinetUToWorld()` → `mmToWorld()` | `setMatrixAt()` |
| 布线表"间隔机柜" | `Math.abs(x1 - x2)`（**用 slotIdx，不用 mm**） | 见下 |

> 🔑 **`间隔机柜 O` 用的是"列坐标差"（离散柜位索引），不是毫米距离。** 这是需求文档 §3.4 明确的：「同通道时，实测 601 行中 81.0% 满足 `O = |列坐标差|`」。所以坐标系统必须**同时暴露毫米坐标与离散槽位索引**，两者都要有，不能只留一个。

**端口级坐标（第五级，最容易漏掉）**

端口是坐标体系的最末级，但需求里 `§3.3` 明确有**两套并存的端口命名**（CLI 板卡式 `GE1/0/24` 与标签式 `千兆1`），且 `FR-6.3` 要求"单机柜详情视图"能看到端口。做法：

```ts
// 端口在设备前面板上的位置不存数据库，由 deviceType 的面板模板算出来
interface PortPanelTemplate {
  deviceTypeId: string;
  rows: number;            // 面板行数（如 24 口交换机 = 2 行 × 12 列）
  cols: number;
  portWidthMm: number;     // 单口宽（RJ45 ≈ 12mm，SFP+ ≈ 14mm）
  portHeightMm: number;
  startXMm: number;        // 面板左侧留白
  startYMm: number;        // 面板底部留白
}

/** 端口 → 设备局部坐标（mm，相对设备左下角）→ 再复合到机柜 → 到房间 */
export function portToRectMm(
  p: Port, tpl: PortPanelTemplate, device: RectMm
): RectMm {
  const idx = p.panelIndex;                       // 0-based，导入时按端口名解析
  const r = Math.floor(idx / tpl.cols), c = idx % tpl.cols;
  return {
    x: device.x + tpl.startXMm + c * tpl.portWidthMm,
    y: device.y + tpl.startYMm + (tpl.rows - 1 - r) * tpl.portHeightMm,  // 面板自上而下
    w: tpl.portWidthMm - 1,                        // 1mm 间隔，视觉分隔
    h: tpl.portHeightMm - 1,
  };
}
```

**关键决策：端口坐标不入库**（只存 `panelIndex` + `deviceTypeId`）。理由：
- 10000+ 端口如果每个都存坐标，数据体积和迁移成本爆炸；
- 设备型号换了（面板布局变），存下来的坐标就是错的；
- `panelIndex` 从 5 种版式的端口名（`GE1/0/24` → 24；`千兆1` → 1）解析出来即可（`§3.3` 已有两套解析规则）。

> ⚠️ **端口的 `panelIndex` 解析必须按 deviceType 分别配置**，不能一个正则通吃。`GE1/0/24` 的板卡/槽位/端口三段式与 `千兆1` 的中文序号完全不同的解析路径。这是导入器 `§3.5`「模板驱动」原则在端口级的延伸。

### 4.3 单一数据模型 + 三个渲染后端

**架构：Normalized Store（唯一真值） → Selector（视图投影） → Renderer（三个后端）。**

```
                    ┌─────────────────────────────┐
                    │  domain/model.ts (规范化的)   │  ← 唯一真值，纯数据，无渲染字段
                    │  rooms, aisles, cabinets,    │
                    │  devices, cables, ports      │
                    │  —— 全部以 id 为主键，扁平   │
                    └──────────────┬──────────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              ▼                    ▼                    ▼
    selectPlanLayout()    selectRackView()      selectTableRows()
    (SVG 视图投影)         (2D/3D 共用几何)        (AG Grid 行)
              │                    │                    │
              ▼                    ▼                    ▼
        <PlanSvg/>          <RackSvg/> + <Scene3D/>   <AgGridReact/>
      (纯 SVG DOM)      (SVG 或 InstancedMesh)      (DOM)
```

**为什么要 zustand + immer（5.0.15 / 11.1.21，均 MIT）：**

1. **zustand 的 selector 天然做"派生态"**：三个视图各自 `useStore(s => selectXxx(s))`，数据变了只有真正依赖的视图重渲染。对比 Redux 更少的样板，对比 Context 更好的重渲染控制（Context 一变全树重渲染，2D+3D+表格同挂一棵树时会互相拖累）。
2. **immer 让"改一个 U 位"写起来像改普通对象**，但产出结构共享的新树 → zustand 的引用比较才有意义。
3. **关键约束：store 里绝不存"渲染状态"**。缩放级别、悬停的 DOM 元素、3D 相机姿态**不入 store**（或放在独立的 `uiStore` 里，且用 `subscribeWithSelector` 避免污染领域 selector）。

**渲染层必须是无状态纯函数。这是一条可以在 Code Review 里机械检查的规则：**

> ❌ 禁止：`<KonvaLayer>` 自己维护 `children` 数组；`handsontable` 自己维护单元格值
> ✅ 要求：`render(state): JSX`，所有交互通过 `dispatch(action)` 回到 store

**这条规则直接否掉了 Handsontable 与 Univer**（§6.3 / §6.5）：它们是"自带数据源 + 自己管编辑状态"的控件，接入就要做双向同步，而双向同步在两个以上视图并存时必然产生竞态。

### 4.4 2D ↔ 3D 联动的具体实现（FR-6.5 / FR-7.3）

联动不是"两个画布互相发消息"，而是**共享选中态**：

```ts
// uiStore.ts —— 跨视图的瞬时交互状态
interface UIState {
  selectedEntity: { kind: 'device' | 'cable' | 'cabinet' | 'port'; id: string } | null;
  hoveredEntity:  { kind: string; id: string } | null;
  highlightedCableIds: string[];   // 由 selectedEntity 派生，缓存起来供 2D/3D 同时读
  cameraPreset: 'top' | 'front' | 'aisle' | 'free';
}
```

**联动流程（2D 点击 → 3D 高亮）：**

1. 2D SVG 设备块 `onClick` → `uiStore.setSelected({kind:'device', id})`
2. 一个 `subscribe` 派生 `highlightedCableIds = cablesOf(deviceId)`（**算一次，两个视图共用**）
3. `<PlanSvg>` 的连线层按 `highlightedCableIds` 加高亮 class
4. `<Scene3D>` 的 `useFrame` 里按 `highlightedCableIds` 更新 `instanceColor`（**注意：不要在 render 里直接改，用 `useEffect` + `instanceColor.needsUpdate = true`**）
5. 3D 相机若需"飞到该设备"：`cameraPreset` 变化 → `useEffect` 里 tween（drei `CameraControls` 或手写 lerp）

> ⚠️ **踩坑预警**：2D 和 3D 同时挂载时（分屏视图），**同一个 hover 事件会触发两套 raycast / hit-test**。必须做统一入口：`hitTest(event) → entityId`，由当前获得焦点的视图负责，另一个视图只读 `hoveredEntity`。

### 4.5 坐标系统的测试策略（这部分必须写单测，不能靠肉眼）

```ts
describe('coords', () => {
  it('镜像：通道 N 左侧 index=0 与右侧 index=26 物理相对', () => {
    const l = cabinetSlotToPlan(room, aisle8, 'L', 0);
    const r = cabinetSlotToPlan(room, aisle8, 'R', 26);
    expect(Math.abs(l.pos.x - r.pos.x)).toBeLessThan(CABINET_PITCH_MM);
  });
  it('U1 在柜底，U42 在柜顶', () => {
    expect(uToHeightMm(1)).toBe(0);
    expect(uToHeightMm(42)).toBeCloseTo(41 * 44.45);
  });
  it('3D 与 2D 的柜位平面位置一致（x,z 对 x,y）', () => {
    const p = cabinetSlotToPlan(room, aisle, 'L', 5);
    const w = cabinetUToWorld(p, 1, 1);
    expect(w.x).toBe(p.pos.x);
    expect(w.z).toBe(p.pos.y);
  });
});
```

**验收锚点**：`AC-4`「导出的 `404房间1-3通道` U 位图，设备位置与现有 Excel 完全一致」——这条验收**只能靠坐标单测 + golden file 对比**来保证，不能靠人眼看。建议把 V13 里 `404房间1-3通道` 的 U 位图导出成 fixture，作为回归基线。
---

## 5. 走线可视化

### 5.1 需求拆解：这是四类不同的线，不是一类

需求要"走线可视化"，但实际是**四个几何场景**，各自算法完全不同：

| 场景 | 几何性质 | 数量级 | 算法 |
|---|---|---|---|
| **A. 柜内跳线** | 柜内 2D 面板上，两端口之间的短线 | 单柜 ≤ 90 条 | 简单折线 + 圆角 |
| **B. 柜间同通道** | 同一通道两侧机柜之间，跨冷通道（水平） | 占比最大 | 曼哈顿路由（水平 → 竖直 → 水平） |
| **C. 跨通道（骨干汇聚）** | 经顶部桥架跨通道 | 少数长线 | **三段式**：柜顶 → 桥架 → 目标柜顶 |
| **D. bundle（合并）** | 多条同路径线合并成一条粗线 | 展示用 | 路径签名分组 + 计数 |

> **关键判断：这些线不能用一个"通用图布局引擎"解决。** 因为它们不是"图（graph）"，而是**在已知的规则几何结构上的路径规划**。机柜位置、桥架位置、通道宽度都是**已知常量**（§4.1），不存在"布局"问题，只需要"路由"。

### 5.2 为什么不选 ELK / libavoid / dagre（这是本节最重要的一节）

**候选与核实结果：**

| 库 | 版本 / 许可 | 官方定位 | 判词 |
|---|---|---|---|
| elkjs | 0.12.0，**EPL-2.0 OR GPL-3.0-or-later** | 「基于 Sugiyama 算法的自动图布局。专为数据流图和**带端口**的场景优化」（npm description） | **不选**。见下 |
| libavoid-js | 0.5.0-beta.5，**LGPL-2.1-or-later** | 「libavoid 的 JS 移植」（Adaptagrams 项目的正交连接器） | **不选**。见下，但记住它是最接近正确的候选 |
| @dagrejs/dagre | 3.1.1，MIT | 有向图分层布局（旧 `dagre` 0.8.5 停在 2019） | 不选。它是**布局**不是**路由**，不产出折线路径 |
| d3-dag | 1.2.2，MIT | 同上，DAG 布局 | 不选，同上 |
| @antv/layout | 2.0.0，MIT | 图布局集合 | 不选，同上 |
| cytoscape | 3.34.4，MIT | 图可视化 + 部分边路由 | 不选。**为图而生的重库**，边路由只是附属功能 |
| @xyflow/react | 12.12.0，MIT | 节点连线编辑器 | **不选但有参考价值**。它证明"正交边 + 手工折点"是可控的，但它的坐标是用户拖的，我们是算的 |

**不选 ELK 的理由（3 条，按权重）：**

1. **方向根本不同**。ELK 输入是「节点 + 边 + 端口约束」，输出是「节点该放哪 + 边怎么绕」。**我们的节点位置是给定的、不可动的**（机柜在物理位置上）。让 ELK 只做边路由，等于用 90% 的算力去算一个我们不需要的答案，还得设法把节点钉死（`fixed` 约束），配置复杂度陡增。
2. **ELK 的正交路由是为"任意拓扑"设计的**。它会为了保证不重叠做全局优化，产出的路径**不可预测**——同一份数据两次布局可能给出不同折点。而施工图**必须稳定**：用户打印出来的图，前后两次不能不一样。**确定性（determinism）在这个场景是硬需求，ELK 不保证。**
3. **许可证**：`EPL-2.0 OR GPL-3.0-or-later`。ELK 是 Java 写的，elkjs 是 GWT 转译的 JS。EPL-2.0 是**弱 copyleft**（文件级），用在**未修改**的依赖里通常没问题，但**甲方私有化交付时法务一定会问**，为一条走线引入这个审查成本不值得。GPL-3.0 选项更不可用（会传染整个前端）。

**不选 libavoid-js 的理由（它其实最接近正确）：**

- **优点**：`libavoid`（Adaptagrams）的 `OrthogonalConnector` **就是**做"节点固定、只算正交连线、避开障碍物"的——正中我们的需求。`libavoid-js` 是 WASM/JS 移植，0.5.0-beta.5（2026-02-23），**仍是 beta**。
- **不选的原因**：
  1. **许可证是 LGPL-2.1-or-later**。前端 bundle 里静态链接 LGPL 的 WASM，法律上处于灰区（LGPL 的"动态链接"定义在 JS/WASM 打包场景没有共识）。**私有化交付项目不值得踩这个雷。**
  2. **仍是 beta**，0.5.0 系列从 beta.1 到 beta.5 反复，API 未稳定。
  3. **能力过剩**：它能处理"任意障碍物 + 任意端口方向"的一般问题。我们的障碍物是**规则的矩形网格**（机柜、桥架），有解析解。
- **保留为"技术债备选"**：若未来要做"自由摆放的机房"或"复杂多层桥架"，重新评估 libavoid。

**结论：自研。但要说清楚自研的是什么——不是"重写 libavoid"，而是一个 ~300 行的规则化路由器。**

### 5.3 自研走线路由器设计（可落地）

#### 5.3.1 三层路径模型

```ts
type PathSegment =
  | { kind: 'cabinet-internal'; cabinetId: string; from: PortRef; to: PortRef }
  | { kind: 'inter-cabinet';    from: CabinetRef; to: CabinetRef; channel: 'front' | 'top' }
  | { kind: 'backbone';         from: CabinetRef; to: CabinetRef; viaTrayIds: string[] };

interface RoutedCable {
  cableId: string;
  points: Vec3[];          // 折点序列，mm
  signature: string;       // 用于 bundle 合并：`${fromSlot}>${toSlot}#${trayPath}`
  laneIndex: number;       // 槽位序号，避免同路径重叠
}
```

#### 5.3.2 柜间同通道：曼哈顿 + 槽位（lane）分配

```
情况 1：同一通道，面对面（§1.3 的镜像柜，1↔27）
   路径：本端柜面板 → 水平穿过冷通道 → 对端柜面板
   折点：3 个（起、通道中线、终）

情况 2：同一通道，同侧相邻
   路径：柜顶出线 → 桥架 → 邻柜柜顶入线
   折点：5 个

情况 3：跨通道（骨干）
   路径：本柜柜顶 → 本通道顶部桥架 → 主桥架 → 目标通道桥架 → 目标柜柜顶
   折点：7~9 个
```

**槽位（lane）分配——这是"不穿模"的关键：**

同一条路径上如果有多根线（例如 1 号柜到 27 号柜有 40 根线），它们会完全重叠成一根。两种处理：

| 方式 | 做法 | 用在哪 |
|---|---|---|
| **偏移（offset）** | 第 i 根线在垂直路径方向偏移 `i * LANE_PITCH_MM`（如 15mm） | 柜间 2D 走线（视觉上要能数清根数） |
| **合并（bundle）** | 路径相同的 N 根线合成一条，线宽 `base + log2(N)*k`，标 `N 根` | 3D 场景 + 骨干（不然 3D 里 5000 根线糊成一团） |

```ts
const LANE_PITCH_MM = 15;      // 线束里每根线的间距（相当于理线梳的齿距）
const MAX_LANES = 12;          // 超过 12 根不再偏移，转 bundle

function assignLanes(cables: CableGeom[]): CableGeom[] {
  // 按 signature 分组，组内按 cableId 稳定排序（保证确定性！）
  const groups = groupBy(cables, c => c.signature);
  for (const [sig, group] of groups) {
    group.sort((a, b) => a.cableId.localeCompare(b.cableId));   // ← 确定性排序
    group.forEach((c, i) => {
      c.laneIndex = i % MAX_LANES;
      c.bundledCount = i >= MAX_LANES ? Math.floor(i / MAX_LANES) + 1 : 1;
    });
  }
  return cables;
}
```

> 🔑 **确定性排序（`cableId.localeCompare`）是必须的**。如果按数组顺序（可能是 Map 迭代顺序或异步加载顺序）分配 lane，用户刷新页面后走线会跳位置。**这就是 ELK 那类库不可接受的同款问题，自研时必须自己保证。**

#### 5.3.3 不穿模（避障）：用"通道槽位"而不是通用避障

我们**不需要 A\***。原因：障碍物（机柜、桥架、空调、柱子）**全部是轴对齐矩形且位置已知**，路径必须走**预定义的走线区**（冷通道上方的桥架、柜顶空间）。这是"在有向图上走最短路"，而图很小：

```
节点 = 每个通道的桥架入口/出口 + 主干桥架交叉点  （≈ 14 通道 × 2 + 若干 = 30~50 个节点）
边   = 相邻节点之间的直线段，权重 = 物理长度
```

```ts
// 极小规模的 Dijkstra，节点 < 100，无需任何图库
function routeBackbone(fromTray: string, toTray: string): string[] {
  return dijkstra(TRAY_GRAPH, fromTray, toTray);  // 30 行手写即可
}
```

**为什么不用 `pathfinding` 0.4.18**：该包 npm 上最后发布是 2016 年（已核实），早已停更；而我们的图节点 < 100，手写 Dijkstra/BFS 30 行就够，**引一个十年未更新的依赖是净负分**。

#### 5.3.4 圆角与视觉质量

直角折线在施工图里应该带圆角（更接近真实线缆弯曲半径）：

```ts
// 用 d3-shape 的 line + curveBumpX / 或手写 arcTo
import { line, curveCatmullRom } from 'd3-shape';   // d3-shape 3.x ISC
```

- **圆角半径要遵守物理弯曲半径**（光纤最小弯曲半径 ≈ 10× 外径）。加这条能让"走线示意图"变成有工程意义的图，而不是装饰。
- 2D 用 `<path d="...">`，3D 用 `THREE.CatmullRomCurve3` 采样成 `Line2`/`LineSegments`。

### 5.4 2D 与 3D 走线的差异化策略

| | 2D（施工图） | 3D（示意） |
|---|---|---|
| 目标 | **可信、可打印、可数清根数** | **看清走向、不糊** |
| 精度 | 精确折点（mm） | 简化（柜顶 → 桥架 → 柜顶） |
| 全部 5127 条 | **分层按需显示**（默认只显示选中设备的） | 只显示 bundle |
| 线宽 | 固定 1~2px，按线型区分颜色 | 按 bundle 计数加权 |
| 交叉处理 | 用"跳线弧"（line hop）画交叉 | 不做，靠 3D 深度 + 透明 |
| FR-7.5 原文 | — | 「简化曲线，不追求物理精确走线」→ **与本节结论一致** |

> **重要**：需求 FR-7.5 明确说 3D 连线"不追求物理精确走线"，优先级 P2。所以**3D 端只做简化示意（柜顶 → 桥架 → 柜顶的三段折线 + bundle 合并）**，把精确路由的算力与精度全部投在 2D。这是一个可以省掉大量工作的需求解读。

### 5.5 走线性能预算

| 操作 | 目标 | 做法 |
|---|---|---|
| 全量 5127 条路由计算 | < 200ms | 纯几何计算，无 DOM；Web Worker 里跑 |
| 单条连线增删改 | < 20ms | 增量：只重算该 cable 的 path + 重新分配同 signature 组的 lane |
| 2D 渲染 500 条高亮 | 60fps | 合并为一个 `<path>`（同色批量拼 `d`） |
| 3D 渲染 bundle 线 | 30fps | 1 个 `LineSegments`，`BufferGeometry` 一次性构建 |

> ⚠️ **路由计算必须在 Web Worker 里**。5000 条线的路由 + lane 分配在主线程会卡住输入。`Comlink` 或直接 `postMessage` + `Transferable` 共享 `Float32Array`。NFR-1 的"全量重算 < 3s"包括线长计算 + 路由计算，两个都要 worker 化。
---

## 6. 大表格方案（5000+ 行）

### 6.1 需求硬指标逐项拆解

需求文档的表格要求可以逐项转成"功能清单 + 许可影响"：

| # | 需求 | 来源 | AG Grid Community | TanStack Table | Glide | Handsontable | Univer |
|---|---|---|---|---|---|---|---|
| T1 | 行虚拟化（5000+ 行 60fps） | NFR-2 | ✅ | ❌ 需自配 react-virtual | ✅ 原生 | ✅ | ✅ |
| T2 | 列冻结（左侧 6 列本端固定） | 布线表 25 列太宽 | ✅ | ⚠️ 手写 CSS sticky | ✅ | ✅ | ✅ |
| T3 | 单元格可编辑 | 核定长度 U 需人工填 | ✅ | ⚠️ 无 UI，全自研 | ✅ | ✅ | ✅ |
| T4 | 校验高亮（脏数据标红） | §7.2 五种脏数据 | ✅ `cellClassRules` | ⚠️ 自研 | ⚠️ 自研 | ✅ | ⚠️ |
| T5 | 公式列（线长 P/R/S/T） | §3.4 核心资产 | ✅ `valueGetter` | ✅ 天然 | ⚠️ 需 HyperFormula | ⚠️ **HyperFormula 是 GPL-3.0** | ✅ 内置 |
| T6 | **区域选择 + Ctrl+C/V** | Excel 用户刚需 | ❌ **仅 Enterprise** | ❌ | ⚠️ 部分 | ✅ | ✅ |
| T7 | 填充柄（拖拽复制） | Excel 用户刚需 | ❌ **仅 Enterprise** | ❌ | ⚠️ | ✅ | ✅ |
| T8 | 撤销/重做 | FR-8.3 (P2) | ✅ `undoRedo` | ❌ | ❌ | ✅ | ✅ |
| T9 | 状态栏（选中求和/计数） | Excel 用户习惯 | ❌ **仅 Enterprise** | ❌ | ❌ | ⚠️ | ✅ |
| T10 | 右键菜单 | 用户习惯 | ❌ **仅 Enterprise** | ❌ | ❌ | ✅ | ✅ |
| T11 | 导出 Excel | AC-3 | ❌ 仅 Enterprise | ❌ | ❌ | ❌ 用 HyperFormula 无关 | ⚠️ Pro |
| **许可** | — | — | **MIT** | MIT | MIT | **商业/非商用双许可** | Apache-2.0 + Pro 商业 |
| **维护** | — | — | 活跃（36.2.0 / 2026-09-16，GitHub pushed 2026-10-20） | 活跃（9.3.1 / 2026-10-12） | **事实停更** | 活跃（18.1.2 / 2026-10-07） | 活跃（1.1.0 / 2026-10-17） |

> 表格里最扎眼的四个 ❌ 全在 AG Grid Community 的 T6/T7/T9/T10 —— 都是 Excel 化交互。这些正是 §1.6 说的"刚需"，**也是本节结论要比"选 AG Grid Community"更复杂的原因**。

### 6.2 AG Grid Community（主选）

**核实到的许可边界（来源：官方 feature matrix，https://www.ag-grid.com/license-pricing/ ，2026-10-20 抓取）**

Community（MIT，免费）**包含**：
- 行虚拟化、列虚拟化
- 列固定（Pinning）、列分组、列宽调整、列移动
- 排序、过滤（文本/数字/日期）、Quick Filter
- 单元格编辑（Text / Number / Date / Checkbox / Large Text / Select 编辑器）
- 单元格样式规则 `cellClassRules`（→ 校验高亮）
- **Undo / Redo**（✅ 在 Community，这条很多人以为要企业版）
- CSV 导出
- 分页、选择（行选择）
- Tooltips
- 无限滚动行模型（Infinite Row Model）

Community **不包含**（Enterprise，$999 USD/dev，永久 license 含 1 年更新）：
- **Clipboard（剪贴板复制粘贴）** ← 官方文档 `clipboard.md` 显式标 `enterprise: true`
- **Cell Range Selection（区域选择）**
- **Fill Handle（填充柄）**
- **Status Bar（状态栏）**
- **Context Menu / Column Menu**
- **Excel Export / PDF Export**
- Row Grouping / Pivoting / Tree Data / Master Detail
- Server-Side Row Model（Infinite 是 Community，Server-Side 是 Enterprise —— 已核实 `server-side-model.md` 标 `enterprise: true`，`infinite-scrolling.md` 无 enterprise 标记）
- Set Filter / Advanced Filter / Find / Batch Editing
- **Calculated Columns（"计算列"）** ← 这一条要特别点明，见下

> ⚠️ **术语陷阱：`Calculated Columns` ≠ `colDef.valueGetter`，前者是 Enterprise 模块，后者是 Community 的普通列定义字段。**
>
> - 官方 feature matrix 中 `Calculated Columns` 一行的 Community 列是 **✗**；其文档页 https://www.ag-grid.com/javascript-data-grid/calculated-columns/ 的侧边导航标 `isEnterprise: true`，正文示例的 import 是 `import { CalculatedColumnsModule, ColumnMenuModule } from "ag-grid-enterprise";`。
> - 我们需要的"公式列"（线长 P/R/S/T）**不走这个模块**，而是用 Community 就有的 `colDef.valueGetter` 指向 domain 层的纯函数：`valueGetter: p => computeCableLen(p.data)`。
> - 理由反而更强：**公式是要审计、要回归测试的核心资产，应该活在 domain 层（可单测、无 UI 依赖），不应该活在表格控件的配置里。** 所以这里"Community 少一个功能"不是损失，是更对的架构。
> - 唯一损失：Enterprise 的 Calculated Columns 支持用户在**界面上**手写表达式；我们没有这个需求（NFR-8 要求规则配置化，走配置文件而不是让用户写 Excel 语法）。

**这个边界必须让 lead 和用户都看到**，因为它决定了一个真实的采购决策：

> **用户的 Excel 使用习惯（区域选择、Ctrl+C/V、填充柄、状态栏求和）100% 落在 Enterprise 里。** 这不是"锦上添花"，是从 Excel 迁移的**核心肌肉记忆**。

**但注意 `community-vs-enterprise` 的定价语义**：$999/dev 是**永久 license**（官方 feature matrix 的 Perpetual License 行原文：`✓ (Including 1 year of updates (see EULA))`），不是订阅。离线部署技术上可行——官方 license 安装文档全文只要求 `LicenseManager.setLicenseKey()` 一行字符串，**通篇未出现 license server / 联网校验 / phone-home**。**对私有化交付项目，这是一次性成本，不是 recurring cost。**

### 6.3 结论：AG Grid Community 打底 + 自研 Excel 化交互层

**推荐方案（三层）：**

```
┌─────────────────────────────────────────────────────────┐
│ L3  Excel 化交互层（自研，~600 行）                        │
│     · 区域选择（mousedown + shift 扩展，用 cellClass 高亮）  │
│     · Ctrl+C / Ctrl+V（监听 keydown，读写剪贴板 TSV）       │
│     · 填充柄（右下角 6px 手柄 + 拖拽复制）                   │
│     · 右键菜单（AG Grid Community 的 cellContextMenu 事件）  │
│     · 状态栏（自己画一个 div，读 selection 状态）            │
├─────────────────────────────────────────────────────────┤
│ L2  AG Grid Community 36.2.0 (MIT)                       │
│     · 虚拟滚动 / 列冻结 / 编辑 / cellClassRules            │
│     · valueGetter 承接线长公式列                           │
├─────────────────────────────────────────────────────────┤
│ L1  domain（纯 TS，无 UI 依赖）                            │
│     · 线长计算、校验规则、规范化数据                         │
└─────────────────────────────────────────────────────────┘
```

**为什么自研而不是直接买 Enterprise：**

1. **自研量的真实估算**：AG Grid 暴露了所需的所有原语——`getCellRanges` 虽在 Enterprise，但 Community 有 `getFocusedCell`、`getSelectedRows`、`cellMouseDown`/`cellMouseOver` 事件、`api.applyTransaction`、`getCellClassList`。用这些原语做区域选择 + 复制粘贴 **≈ 300 行**，填充柄 **≈ 150 行**，状态栏 **≈ 80 行**。合计 500~600 行。**这是一个能在一周内做完并写测试的量。**
2. **但 L3 并不是"省 999 美元"**——它的真实价值是：**交互逻辑掌握在自己手里**。比如"粘贴 25 列到布线表时要做脏数据归一化"（§7.2 的 608+76 处空格不一致、中英文括号混用），这种**粘贴即清洗**的能力，Enterprise 的通用剪贴板给不了。
3. **保留采购路径**：如果用户反馈"自研交互层不够顺滑"，随时可以买 Enterprise 替换 L3（L2 代码不用改，只需去掉自研层、开 Enterprise 模块）。**这是一个可以反悔的决策**——这是选它的核心原因。

**为什么不选 TanStack Table v8/v9：**

- `@tanstack/react-table` 9.3.1（MIT，28.5k★，2026-10-12）是**headless** 的——它只给你状态管理和 API，**UI 一行都不给**。
- 这意味着虚拟滚动、列冻结、编辑、校验高亮**全部要自研**。T1~T10 里 TanStack 原生只解决 T5（公式列）。**自研量从 600 行变成 3000+ 行。**
- TanStack 的定位是"完全自定义表格 UI"（设计系统场景），我们是"要一个 Excel 样子的业务表格"。**能力错配。**
- ⚠️ **另一个风险**：v9.0.0 于 2026-08-04 发布，**v8→v9 是大版本跃迁**（v8 最后 8.21.3 / 2025-04-14）。网上绝大多数教程还是 v8 的 API。选它意味着**踩新版本坑 + 迁移成本**。
- **Takeaway**：TanStack Table 是**"如果我们要做设计系统"**的选择，不是"要做 Excel 表格"的选择。**但它的虚拟化搭档 `@tanstack/react-virtual` 3.14.21（MIT）值得单独用**——如果将来 2D SVG 需要虚拟化（§2.7）。

**为什么不选 Glide Data Grid：**

- `@glideapps/glide-data-grid` 6.0.3（MIT，5.4k★）**技术上是极优秀的**：canvas 渲染、自绘、超大规模数据性能顶尖（它的目标就是 Airtable/Google Sheets 那种亿级数据表格）。
- **但维护状态是致命的**（已核实）：
  - npm `latest` = **6.0.3，发布于 2024-02-03**
  - 之后所有发布都是 alpha：`6.0.4-alpha21`(2025-08-28) → `alpha24`(2025-10-08)
  - GitHub 最新 commit 2026-01-21，**2025-10-01 之后只有 6 个 commit**
  - → **事实停更**（formal maintenance 停滞）
- 引入一个**停更的 canvas 表格组件**到私有化交付项目里，意味着：浏览器 API 变更、React 19 兼容、安全漏洞都无人修。
- **另一点**：canvas 渲染 = **无 DOM 可访问性、无法用浏览器 Ctrl+F、无法选中文字**。业务表格（要给甲方看、要能用系统字体渲染中文）需要 DOM。

**为什么不选 Handsontable（这条必须写清楚，因为它最像 Excel）：**

`handsontable` 18.1.2（2026-10-07，22.1k★）。**功能上它是最接近 Excel 的**（区域选择、填充柄、剪贴板、右键菜单、状态栏全有）。**但许可不通：**

从 npm tarball 里解出的 `LICENSE.txt` 原文（`handsontable@18.1.2` package/LICENSE.txt）：

> "This software is protected by applicable copyright laws ... and dual-licensed - depending on whether your use for commercial purposes, **meaning intended for or resulting in commercial advantage or monetary compensation**, or not."
> "If your use is strictly personal or solely for evaluation purposes ... you agree to be bound by the terms included in the 'handsontable-non-commercial-license.pdf' file."
> "Your use of this software for commercial purposes is subject to the terms included in an applicable license agreement."

**判定：本系统是商业交付产品（给甲方做 IDC 管理），属于 "commercial purposes"。非商用许可不适用，必须购买商业 license。**

补充事实（官方 pricing 页，https://handsontable.com/pricing ）：
- 定价方式：「**By the number of end users of the applications you build with it**」（不是按开发者！）
- license 有 subscription（会过期）与 perpetual（永久）两种
- HyperFormula（公式引擎）**是独立授权的 GPLv3 或商业 license**；用 `gpl-v3` key 只在非商用/开源项目合法
- 提供 45 天试用、离线部署（「no phone-home or license server」—— 这点对 NFR-3 友好）

→ **排除理由：商业 license + 按终端用户计价，成本模型与"私有化交付给一个甲方"不匹配且不可控。**

**为什么不选 Univer（这条也要写清楚，它最新最热）：**

`@univerjs/*` 1.1.0（**Apache-2.0**，2026-10-17，GitHub `dream-num/univer` 22.7k★，LICENSE 经 GitHub contents API 解码确认为标准 Apache License 2.0 全文）。

**Univer 是什么**：不是"表格组件"，是**一整套 Office SDK**（Docs / Sheets / Slides / Boards / Bases / PDFs）。官方文档 https://docs.univer.ai/llms.txt 的描述：「Univer Office SDK, which includes Web SDK, Server SDK, and AI SDK」。

**不选的理由（按权重）：**

1. **为一张表引入一整套 Office 套件**。Univer 的安装要求是注册几十个 plugin（core / design / engine-render / ui / sheets / sheets-ui / sheets-formula / docs / …）。包体积、初始化时间、学习成本全部按"Office 套件"计价，而我们要的是一张 25 列的业务表格。
2. **它有 Pro 层，且 Pro 层许可不明**（这是一个必须记录的尽调发现）：

   | 包 | 版本 | npm `license` 字段 |
   |---|---|---|
   | `@univerjs/core` | 1.1.0 | `Apache-2.0` |
   | `@univerjs/sheets` | 1.1.0 | `Apache-2.0` |
   | `@univerjs/presets` / `preset-sheets-core` | 1.1.0 | `Apache-2.0` |
   | `@univerjs-pro/license` | 1.1.0 | **`None`（未声明）** |
   | `@univerjs-pro/bases` / `sheets-chart` / `sheets-print` / `exchange-client` / `collaboration` / `sheets-pivot` / `sheets-shape` / `engine-formula` | 1.1.0 | **`None`（未声明）** |
   | `@univerjs/preset-sheets-advanced` | 1.1.0 | **`None`** |
   | `@univerjs-pro/edit-history` | 1.1.0 | `Apache-2.0` |

   官方 license 文档（https://docs.univer.ai/server/license.md ）写明：「For features that require a license, visit the Univer website ... Register `UniverLicensePlugin`」，且「A license determines which features are available」。
   → **结论：社区版 Apache-2.0 明确可用；但"哪些功能属于 Pro"在网上没有清晰的功能对照表（未找到公开的 Community vs Pro feature matrix），且 Pro 包的 npm license 字段缺失。对一个要过法务的私有化交付项目，这是不可接受的模糊性。**
3. **架构冲突**：Univer 自己维护一套 workbook/sheet/cell 数据模型 + 自己的命令系统与撤销栈。要把它接到我们的规范化 store 上，需要双向同步（§4.3 的禁令）。**这是最根本的排除理由。**
4. **我们要的"公式列"不是"电子表格公式"**。线长 P/R/S/T 是**业务规则**（`3000 行代码里要能单测、能审计、能配置化` —— NFR-8「线长规则配置化」）。把它写进电子表格公式引擎，等于把核心资产托付给第三方引擎的行为。

> **但 Univer 有一个真实的使用场景**：如果用户明确要求「**在系统里直接打开、编辑、导出他们原来的 .xlsx 文件**」（不只是导入导数据），那 Univer 是目前唯一能"在浏览器里跑一个真的 Excel"的 MIT/Apache 生态方案。**本期需求没要求这个**（需求是"导入 + 导出"，FR-4.5），所以不做。**记录为未来候选。**

### 6.4 react-data-grid（次级备选）

`react-data-grid` 7.0.0-beta.62（MIT，2026-10-19 仍在发布）。**注意：`latest` tag 指向 beta！** 最后一个稳定版是 `6.1.0`。GitHub license 文件是标准 MIT 正文，但 GitHub API 的 license 检测返回 `NOASSERTION`（因为文件里同时列了 2014 Prometheus Research 与 2015 Comcast 的版权行）——**实际许可是 MIT**（已用 contents API 取 `LICENSE` base64 解码逐字确认）。另外**仓库已从 `adazzle/react-data-grid` 301 永久迁移到 `Comcast/react-data-grid`**（旧地址访问会重定向），引用来源时用新地址。

- **优点**：DOM 渲染、虚拟滚动、列冻结、编辑都在、API 轻、体积小。
- **缺点**：7.0 的 beta 已经持续很久（`beta.62`），且从 6.x 到 7.x 是重写级变更；功能密度远低于 AG Grid（没有 `cellClassRules` 这种声明式校验钩子，没有内置过滤 UI）。
- **定位**：如果 AG Grid 因为某些原因不能用（比如 bundle 体积硬约束），这是第一顺位的替代。

### 6.5 表格方案总结

| 维度 | 结论 |
|---|---|
| **主选** | AG Grid Community 36.2.0（MIT）+ 自研 Excel 化交互层（~600 行） |
| **备选 1** | AG Grid Enterprise 36.2.0（$999/dev 永久，离线可用）—— 上线后按用户反馈决定 |
| **备选 2** | react-data-grid 6.1.0（稳定版，MIT）—— 若 AG Grid 因体积/许可争议出局 |
| **明确排除** | Handsontable（商业 license 按终端用户计价）、Univer（Office 套件过重 + Pro 许可边界模糊 + 数据模型冲突）、Glide Data Grid（事实停更 + canvas 无 DOM）、TanStack Table（headless，自研量 5 倍） |
| **算力放哪** | **公式列一律在 domain 层算**，AG Grid 只用 `valueGetter` 取值。绝不把线长公式写进表格控件 |

### 6.6 5000 行性能的具体落地

| 措施 | 说明 |
|---|---|
| 行虚拟化 | AG Grid 默认开（`rowBuffer` 默认 10） |
| **只给可见列 `valueGetter`** | 25 列 × 5000 行 = 12.5 万次取值；若全量算会卡。用 `colDef.valueGetter` 时 AG Grid 本就只对可见行求值，**但要避免在 valueGetter 里做 O(n) 查找** → 预建 `Map` |
| 校验结果缓存 | `validationErrors: Map<rowId, Set<colId>>`，`cellClassRules` 只查 Map（O(1)），不重算 |
| 增量更新 | 单条改动用 `api.applyTransaction({update:[row]})`，**不要**整个 `rowData` 换新引用（那是 5000 行全 diff） |
| 编辑不阻塞 | 编辑提交 → 异步校验（debounce 150ms）→ 只更新脏行的 class |

> **验收锚点**：`NFR-2`「5000+ 行虚拟滚动 60fps」。建议用 Chrome DevTools Performance 录制滚动，目标 frame time < 12ms（留 25% 余量）。
---

## 7. 导出方案

### 7.1 四种导出格式，是四个独立问题

需求里"导出"出现过三次，但其实是四件事，每件要用不同的技术：

| # | 导出物 | 需求 | 目标格式 | 技术 |
|---|---|---|---|---|
| E1 | 布线表（25 列 × 5000+ 行） | AC-3 / NFR-5 | **.xlsx**（带样式、列宽、冻结、公式） | ExcelJS |
| E2 | U 位图 / 平面图（施工打印） | FR-6.7 / AC-4 | **SVG / PNG / PDF** | 原生 SVG → PDF |
| E3 | 3D 截图 | 未明确要求 | PNG | `renderer.domElement.toDataURL()` |
| E4 | 导入（反向） | FR-4.5 / §3.5 | 读 5 种版式 .xlsx | **SheetJS CE** |

> **关键判断：导出（E1）和导入（E4）应该用两个不同的库。** 这不是浪费——两件事的能力要求恰好互补（见 §7.2 与 §8）。

### 7.2 Excel 导出：ExcelJS 是唯一可行的 MIT 选择

**为什么需要"真正能写样式"的库：**

`AC-3`「导出的布线表 25 列表头与 V13 完全一致」+ `AC-4`「导出的 U 位图与现有 Excel 完全一致」+ `NFR-5`「能被 Excel 2016+/WPS 打开」。这三条合起来意味着导出物**不能是纯数据 dump**：

- 表头要有底色、加粗、居中、边框（用户拿去给施工队看）
- 25 列要设列宽（否则打开全是 `####`）
- 要**冻结首行 + 左侧 6 列**（否则 5000 行滚起来看不懂）
- 关键列要**数据验证下拉**（线缆类型、档位）
- 计算列要**写成真正的 Excel 公式**（用户还能在 Excel 里改参数重算）
- 条件格式（校验失败的行标红）

**候选核实对比：**

| 库 | 版本 | 许可 | 写样式 | 冻结窗格 | 数据验证 | 真公式 | 维护状态 |
|---|---|---|---|---|---|---|---|
| **exceljs** | 4.4.0 (2023-10-19) | **MIT** | ✅ 完整 | ✅ `views[].state='frozen'` | ✅ | ✅ `{formula: 'IFS(...)'}` | ⚠️ **事实停更**（见 §8.2） |
| xlsx (SheetJS CE) | npm 0.18.5 / CDN 0.20.3 | Apache-2.0 | ❌ **极弱** | ⚠️ 有限 | ⚠️ | ⚠️ | ⚠️ 已离开 npm 生态 |
| @e965/xlsx | 0.20.3 (2025-04-08) | Apache-2.0 | 同 SheetJS CE | — | — | — | 社区转发，跟着上游走 |
| write-excel-file | 4.1.1 (2026-08-20) | MIT | ✅ 较好 | ✅ | ❌ | ⚠️ | ✅ 活跃 |
| @zurmokeeper/exceljs | 4.4.9 (2025-02-26) | MIT | ✅ 同 exceljs | ✅ | ✅ | ✅ | ✅ 社区 fork，仍在维护 |
| xlsx-js-style | 1.2.0 (2022-08-22) | Apache-2.0 | ✅ 有样式 | ? | ? | ? | ❌ 停更 4 年 |
| node-xlsx | 0.24.0 (2025-03-14) | Apache-2.0 | ❌ 无样式 | ❌ | ❌ | ⚠️ | ✅ 活跃但能力不足 |

**结论：ExcelJS 4.4.0 (MIT) 是主选**，理由是"能力 × 许可"的唯一交点：

1. **写样式是完整档**：`font` / `fill` / `border` / `alignment` / `numFmt` 全支持，走标准 `styles.xml`（这对 WPS 兼容是必要的，见 §1.5）。
2. **冻结窗格**：`worksheet.views = [{ state: 'frozen', xSplit: 6, ySplit: 1 }]`。这正好满足"25 列太宽，冻结左侧本端 6 列"。
3. **数据验证**：`cell.dataValidation = { type: 'list', formulae: ['"RJ45,LC-LC,MPO"'] }`——用户在 Excel 里改线缆类型不会打出错字，这是防止脏数据回流的关键。
4. **真公式**：`{ formula: 'IFS(S2<=5,5,S2<=6,7,...)' }` 写进去是**活公式**，用户在 Excel 里能看到、能改、能审计。这一点极其重要：`§3.4` 的 P/R/S/T 是用户的核心资产，导出后**必须仍是公式**，不能是算好的死值。
5. **流式写**：`new ExcelJS.stream.xlsx.WorkbookWriter()` 支持流式写，5000 行 × 25 列在 Node 端不占内存。但**我们的导出在前端做**（用户点"导出"直接下载），浏览器端用普通 `workbook.xlsx.writeBuffer()` 完全够（25×5000 = 12.5 万单元格，约 3~8MB，耗时 1~2s）。

**为什么不选 SheetJS 做导出（必须写清楚，因为它是"看起来最标准"的选择）：**

我把 `xlsx-0.20.3` 的 tarball 解包检查了 `xlsx.mjs` 的写入实现，发现 `write_sty_xml` 内部是**硬编码的单一样式**：

```
<fonts count="1"> ... <fills count="2"> ... <borders count="1">
```

也就是说：**SheetJS CE 的"样式"只够把整个工作簿写成一种字体/一种填充/一种边框。** 它没有暴露"给第 3 行加粗、给 A1:Y1 加底色"的 API（社区版的 `cell.s` 字段不被写出）。**这对 AC-3「表头与 V13 完全一致」是直接不满足的。**

> 注：SheetJS 的样式能力在 **Pro 版**里才有。CE 版的设计取舍是"读优先、写保守"。

### 7.3 SVG → PNG → PDF：为什么"零依赖"是最优解（FR-6.7）

这是纯 SVG 渲染方案（§2.2）带来的最大红利。三条路径：

**路径 1：导出 SVG（施工打印要求矢量）**

```ts
function exportSvg(svgEl: SVGSVGElement, filename: string) {
  const clone = svgEl.cloneNode(true) as SVGSVGElement;
  // 1) 内联所有 CSS（SVG 文件不认外部样式表）
  inlineComputedStyles(svgEl, clone);
  // 2) 设显式 width/height（很多查看器不认 viewBox-only 的 SVG）
  clone.setAttribute('width', String(svgEl.clientWidth));
  clone.setAttribute('height', String(svgEl.clientHeight));
  // 3) 声明 xmlns
  clone.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  const blob = new Blob([new XMLSerializer().serializeToString(clone)],
                        { type: 'image/svg+xml;charset=utf-8' });
  download(blob, filename);
}
```

**路径 2：SVG → PNG（位图，用于贴 PPT / 微信发）**

```ts
async function svgToPng(svgText: string, scale = 2): Promise<Blob> {
  const img = new Image();
  img.src = 'data:image/svg+xml;base64,' + btoa(unescape(encodeURIComponent(svgText)));
  await img.decode();
  const canvas = document.createElement('canvas');
  canvas.width  = img.width  * scale;   // scale=2 → 相当于 2x 高清
  canvas.height = img.height * scale;
  canvas.getContext('2d')!.drawImage(img, 0, 0, canvas.width, canvas.height);
  return new Promise(r => canvas.toBlob(b => r(b!), 'image/png'));
}
```
> ⚠️ **中文乱码坑**：`btoa` 不能处理非 Latin1 字符（设备名全是中文）。必须 `unescape(encodeURIComponent(...))` 或直接用 Blob URL。**这是 SVG→PNG 最高频的 bug。**

**路径 3：SVG → PDF 或 打印**

两条路，按优先级：
1. **`window.print()` + `@media print`**（零依赖）：如果只是"用户按 Ctrl+P 出图"，CSS 就够。
2. **jsPDF + `svg2pdf.js`**：需要"点击按钮直接下载 PDF"时用。
   - `jspdf` **4.2.1，MIT**（发布 2026-03-17，活跃）—— https://registry.npmjs.org/jspdf
   - `svg2pdf.js` **2.8.1，MIT**（发布 2026-08-31，活跃）—— https://registry.npmjs.org/svg2pdf.js 。官方定位：「A javascript-only SVG to PDF conversion utility that runs in the browser leveraging jsPDF」，把 SVG 转成 PDF **矢量**绘图指令，不栅格化。
   - ⚠️ **落地前需 spike 的点**：`svg2pdf.js` 对 `<pattern>`（§2.2 里我建议用它画 42U 刻度）与外部 CSS 的支持度。**若 `<pattern>` 不被支持，改用一条合并的 `<path>` 画 U 刻度——这是 §2.2 已备好的 B 方案，两者效果等同。** 建议 spike 半天，优先验证。

**为什么不用 html-to-image / dom-to-image-more：**

| 库 | 版本 | 许可 | 判词 |
|---|---|---|---|
| html-to-image | 1.11.13 (2026-08-20) | MIT | 不选。它是"把 DOM 转成 PNG"，但我们的 DOM 本身是 SVG → **已经有矢量源了，再栅格化是倒退** |
| dom-to-image-more | 3.11.0 (2026-07-24) | MIT | 不选。同上；且这类库用 `<foreignObject>` 外链 CSS，**跨域字体/中文排版会莫名丢字** |

> **原则：有矢量源就永远不要先栅格化再导出矢量。** PDF 打印质量的分水岭就在这一步。

### 7.4 3D 截图

```ts
renderer.render(scene, camera);                       // 确保 buffer 有效
const dataUrl = renderer.domElement.toDataURL('image/png');
```
- 必须 `preserveDrawingBuffer: true`（或在 `toDataURL` 前同步 `render` 一次），否则得到**空白图**。这是 three.js 最经典的坑。
- 用 `frameloop="demand"`（r3f）时尤其要注意顺序。

### 7.5 导出方案的最终清单

| 导出 | 库 | 版本 | 许可 |
|---|---|---|---|
| Excel 写 | **exceljs** | 4.4.0 | MIT |
| Excel 读（导入） | **xlsx (SheetJS CE)** 从 CDN 装 | 0.20.3 | Apache-2.0 |
| SVG 写 | 原生 `XMLSerializer` | — | — |
| PNG | 原生 canvas `toBlob` | — | — |
| PDF | `jspdf` + `svg2pdf.js`（打印优先用 CSS） | 4.2.1 / 2.8.1 | MIT |
| 3D 截图 | 原生 `toDataURL` | — | — |
---

## 8. 维护状态深挖：三个"成熟但停更"的库怎么用

这一节单独成节，因为它是本文件里**最容易在半年后反咬一口**的部分。三个库都很流行、都 MIT、都"看起来该用"，但**都事实停更**。逐个给证据 + 给应对策略。

### 8.1 Glide Data Grid —— 直接不用

| 证据 | 值 |
|---|---|
| npm `latest` | **6.0.3，发布于 2024-02-03** |
| `latest` 之后的所有发布 | 全是 alpha：`6.0.4-alpha21`(2025-08-28) … `6.0.4-alpha24`(2025-10-08) |
| GitHub | `glideapps/glide-data-grid` 5,403★，最后 push 2026-01-21 |
| commit 密度 | 2025-10-01 之后仅 **6** 个 commit |

**判定：事实停更**（正式版两年半未发）。**不用。** 理由已在 §6.3 说明（还有 canvas 无 DOM 的额外问题）。

### 8.2 ExcelJS —— 用，但要立刻做三件事

| 证据 | 值 |
|---|---|
| npm `latest` | **4.4.0，发布于 2023-10-19**（`dist-tags` 还有 `beta: 0.4.8`、`prerelease: 4.4.1-prerelease.0`） |
| GitHub | `exceljs/exceljs` 15,490★ |
| master 分支 HEAD | sha `5bed18b45e`，日期 **2024-01-12**，提交信息 `fix: table column style bug and addTable colum type (#2649)` |
| 2025-01-01 之后的 commit | **0 条**（GitHub API `commits?since=2025-01-01` 返回空数组） |
| 未关闭 issue + PR | 约 **818** 个 |

**判定：事实停更（两年无 commit，近三年无发布）。**

**但因为它的能力是 MIT 生态里的唯一交点（§7.2），必须用。用法的三件事：**

1. **锁死版本 + 锁死 lockfile**。`"exceljs": "4.4.0"`（精确版本，不要 `^`）。它是纯 JS、零原生依赖、API 稳定，**停更的库只要不升级、不受 CVE 影响就可以用**。要做的是一次 `npm audit` 基线，记录到部署文档。
2. **立刻写导出回归测试**（这是关键）。用 `AC-3`/`AC-4` 作为 golden test：
   ```
   test/export/布线表-25列.spec.ts
     → 生成 xlsx → 用 exceljs 自己读回 → 断言 25 列表头文本逐列等于 V13 表头
     → 断言首行冻结、列宽、数据验证下拉存在
   ```
   **JSON 化断言**（把导出结果的样式树 dump 成 JSON 存 fixture），这样升级/替换时能立刻发现回归。
3. **准备好替换路径**：`@zurmokeeper/exceljs` 4.4.9（MIT，2025-02-26 发布）是社区 fork，API 兼容，**作为维护兜底**。不强求现在就用（fork 的长期生命力也未知），但要在导出模块外围包一层自己的 facade（`domain/export/xlsx-writer.ts`），**保证换库只改一个文件**。

> 这个"包一层 facade"的建议适用于所有第三方库：**AG Grid、ExcelJS、SheetJS、three.js 全部只通过本项目自己的 adapter 使用**。这是把第三方风险隔离在单点的最低成本做法。

### 8.3 SheetJS CE —— 用，但必须从 CDN 装，且只用来读

**这是最需要澄清的一个，因为 npm 上的状态具有强烈误导性。**

| 证据 | 值 |
|---|---|
| npm `xlsx` 的 `dist-tags` | `{'latest': '0.18.5'}` |
| `0.18.5` 发布时间 | **2022-03-24**（4 年半前） |
| npm 上总版本数 | 108，但**最新只到 0.18.5** |
| 官方权威分发 | `https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`（curl HEAD 实测 **HTTP/2 200**，content-length 2,409,319） |
| GitHub `SheetJS/sheetjs` | 36,344★，最后 push **2024-04-18**，仓库描述已改为 **"New home https://git.sheetjs.com/SheetJS/sheetjs"** |
| 新家 `git.sheetjs.com` | 最新 commit **2026-02-09** `d2f2e17963` "sheet_to_html sanitize links option (fixes #3345)"；上一 commit 2026-01-31 "throw error while reading PDF file" |

**为什么 npm 上的版本停在 0.18.5：** SheetJS 官方文档明确把 npm registry 上的版本描述为过期/有 bug，并引导用户从其自有 CDN 安装。这是官方主动的发布策略变更（**社区版仍然 Apache-2.0、仍然在维护，只是不走 npm 了**）。

**结论（三条，必须一起执行）：**

1. **装法**：`npm install https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz`（或把 tgz 下载进仓库 `vendor/` 目录）。
2. **离线部署（NFR-3）要求把 tgz 提交进仓库或内网 npm 镜像**。这是**必须写进部署文档的一步**——否则"一条命令离线部署"会在 `npm install` 阶段断网失败。**这是本文件里唯一一个"部署流程必须特殊处理"的依赖。**
3. **只用来读（导入），不用来写（导出）**。理由：它的写样式能力是硬编码单一样式（§7.2 已给 `write_sty_xml` 证据），不满足 AC-3。
   - **读的能力它是最强的**：容错解析（脏数据、合并单元格、多 sheet、非标准表头）是 SheetJS 的核心竞争力，正好对上 `§3.5` 的 5 种版式和 `§7.2` 的一堆脏数据。

> **补充：还有一个社区转发版本 `@e965/xlsx` 0.20.3（Apache-2.0）** 在 npm 上，是把 SheetJS 0.20.3 重新发到 npm 的社区行为。**不建议用**——它不是官方发布源，无法验证字节一致性。用官方 CDN。

### 8.4 三个库的风险汇总与缓解

| 库 | 停更？ | 影响 | 缓解 |
|---|---|---|---|
| Glide Data Grid | ✅ 是 | 无（不用） | — |
| ExcelJS | ✅ 是 | 导出模块 | 锁版本 + golden test + facade 隔离 + fork 兜底 |
| SheetJS CE | ❌ 否（只是离开 npm） | 导入模块 + **部署流程** | 官方 CDN + tgz 入库 + 写进部署文档 |
| AG Grid | ❌ 否（很活跃） | — | — |
| three.js / r3f | ❌ 否 | — | — |
| react-data-grid | ⚠️ 只在 beta 频道发版 | 备选 | 用稳定版 6.1.0 |

---

## 9. 最终推荐栈（含版本清单）

### 9.1 主选技术栈（可直接写进 package.json）

```jsonc
{
  "dependencies": {
    // ── 框架 ──────────────────────────────────────────────
    "react": "19.3.x",                  // MIT
    "react-dom": "19.3.x",              // MIT

    // ── 状态与领域（与渲染无关，三个视图共享） ───────────────
    "zustand": "5.0.15",                // MIT
    "immer": "11.1.21",                 // MIT
    "zod": "4.6.7",                     // MIT   —— 导入数据校验
    "nanoid": "6.0.2",                  // MIT

    // ── 几何/数学工具（不用于渲染 DOM） ─────────────────────
    "d3-scale": "^4",                   // ISC   —— 色阶、坐标映射
    "d3-shape": "^3",                   // ISC   —— 走线圆角路径
    "rbush": "4.0.1",                   // MIT   —— 2D 空间索引（框选/命中）

    // ── 3D ───────────────────────────────────────────────
    "three": "0.186.1",                 // MIT
    "@react-three/fiber": "9.8.2",      // MIT   —— peer: react >=19 <19.4, three >=0.156
    "@react-three/drei": "10.7.9",      // MIT
    // three-mesh-bvh 0.9.16 (MIT) —— 本期不装，>3 万实例或上 glTF 时再加

    // ── 大表格 ────────────────────────────────────────────
    "ag-grid-community": "36.2.0",      // MIT
    "ag-grid-react": "36.2.0",          // MIT

    // ── Excel ────────────────────────────────────────────
    "exceljs": "4.4.0",                 // MIT（精确锁版本，不可用 ^）
    "xlsx": "https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz",  // Apache-2.0

    // ── PDF（按需） ────────────────────────────────────────
    "jspdf": "4.2.1",                   // MIT（仅在需要"下载 PDF"按钮时引入）
    "svg2pdf.js": "2.8.1"               // MIT
  }
}
```

**2D 渲染零依赖**：机房平面图与机柜 U 位图全部用 React + 原生 `<svg>` 元素，**不引入任何图形库**。这是 §2 的核心结论。

**注意版本联动约束（必须一起拍板，事后改成本高）：**

| 约束 | 说明 |
|---|---|
| `@react-three/fiber@9` ⟺ `react@19` | 官方明写 `r3f@8` 配 `react@18`，`r3f@9` 配 `react@19`。peer 还要求 `react <19.4` |
| `react-konva@19.3.2` 需要 `react ^19.3` | 如果走备选路线（§2.3），React 版本必须 ≥19.3 |
| `ag-grid-community@36` 与 React 19 | AG Grid 36.x 支持 React 19（36.2.0 于 2026-09-16 发布，晚于 React 19 稳定版） |
| SheetJS 走 tarball URL | `package.json` 里直接写 https URL，需确认内网 npm 代理允许 |

### 9.2 备选技术栈（什么条件下切换）

| 触发条件 | 切换动作 | 影响面 |
|---|---|---|
| 单视图 SVG 元素稳定 > 8000，或需要 60fps 连续动画 | 2D 换 **PixiJS 8.22.0**（MIT） | 只换渲染后端，数据层/坐标层不动（§4.3 的架构就是为这个准备的） |
| 需要"自由拖拽布局机柜"的绘图板功能 | 2D 换 **Konva 10.9.0 + react-konva 19.3.2** | 同上；但要接受 SVG 导出能力丢失 |
| 用户强烈要求 Excel 化的顺滑交互 | 买 **AG Grid Enterprise 36.2.0**（$999/dev 永久） | 去掉自研 L3 交互层，开 Enterprise 模块。**L2 代码不改** |
| AG Grid 因体积/许可争议出局 | **react-data-grid 6.1.0**（MIT，稳定版） | 表格层重写，但 `valueGetter`/校验逻辑可复用 |
| 需要"在浏览器里直接编辑原始 .xlsx" | 评估 **Univer 1.1.0**（Apache-2.0 + Pro 商业） | 大改，需重新做许可尽调（§6.3 的问题清单） |
| 3D 需要内置 GUI/物理/复杂材质 | **Babylon.js 9.31.0**（Apache-2.0） | 3D 层重写（Thin Instances 可替代 InstancedMesh） |
| 前端框架改 Vue | **TresJS 5.9.3** + AG Grid Vue 版 | 全栈重评，不建议 |
| 走线需要任意障碍物避障 | **libavoid-js 0.5.0-beta.5**（LGPL-2.1+，需法务确认） | 只换路由器（§5.3 的接口设计允许替换） |

### 9.3 明确不选清单及一句话理由（汇总）

| 候选 | 不选的理由 |
|---|---|
| **纯 SVG** | ✅ **选**（§2.2） |
| Konva / react-konva | 能力错配（我们需要数据驱动而非自由拖拽）+ 丢 SVG 导出 + React 19.3 版本约束 |
| PixiJS | 本期元素量不需要 GPU 吞吐；保留为兜底 |
| Fabric.js | 定位是"绘图编辑器"，会诱导"图形是真值"的反模式 |
| D3（作渲染器） | React 与 D3 抢 DOM；只取子包当数学库 |
| ECharts custom series | 命中测试体系不匹配 + 导出是位图不是矢量 |
| AntV G6 | 是关系图引擎，我们的图是规则格点 |
| **three.js + r3f** | ✅ **选**（§3.2） |
| Babylon.js | 生态小一个量级、体积大、80% 能力用不上（Thin Instances 是真实优势但可接受 three 的方案） |
| TresJS | Vue 专用，React 项目出局 |
| echarts-gl | 3D 图表库，不是场景引擎 |
| glTF 资产管线 | 本期不引（§3.7）—— 程序化 BoxGeometry 反而保住 instancing |
| **AG Grid Community** | ✅ **选**（§6.3） |
| AG Grid Enterprise | 暂不买，保留按需采购路径（$999/dev 永久、可离线） |
| Handsontable | **商业 license，按终端用户计价**，成本模型不匹配（§6.3 已给 LICENSE.txt 原文） |
| Univer | Office 套件过重 + Pro 层 npm license 字段为 None + 数据模型冲突 |
| Glide Data Grid | 事实停更（latest 停在 2024-02）+ canvas 无 DOM |
| TanStack Table | headless，自研量是 AG Grid 方案的 5 倍；v9 是新大版本 |
| react-data-grid | 备选（只在 beta 频道发版） |
| **ExcelJS** | ✅ **选（写）**（§7.2）—— 停更风险已给缓解方案 |
| **SheetJS CE** | ✅ **选（读）** —— 但必须从 CDN 装 0.20.3，npm 上的 0.18.5 是 2022 年快照 |
| write-excel-file | 备选，但不支持数据验证 |
| xlsx-js-style | 停更 4 年（2022-08） |
| **自研正交路由** | ✅ **选**（§5.3） |
| ELK / elkjs | 解决的是"布局"不是"路由"；不保证确定性；EPL-2.0 引入法务审查成本 |
| libavoid-js | 最接近正确但 LGPL-2.1+ 且在 beta；保留为技术债备选 |
| dagre / d3-dag / @antv/layout | 是布局不是路由，不产出折线路径 |
| pathfinding (npm) | 2016 年后停更；我们的图 <100 节点，手写 30 行更好 |
| 原生 SVG 导出 + jspdf/svg2pdf | ✅ **选**（§7.3） |
| html-to-image / dom-to-image-more | 有矢量源却先栅格化，是倒退 |

### 9.4 落地顺序建议（把选型变成可执行的第一步）

| 阶段 | 做什么 | 验证点 |
|---|---|---|
| **S0 spike（3~5 天）** | ① 纯 SVG 画 1 个通道 27 柜 × 42U + 3000 设备块，测帧率与节点数 ② three.js 画 200 柜 + 3000 设备 InstancedMesh，测 draw call 与 raycast 耗时 ③ AG Grid Community 载 5000 行 25 列，测滚动帧率 ④ ExcelJS 导出 V13 的 25 列表头，用 WPS 打开 | 四个 spike 全部达标才继续；任何一个不达标，按 §9.2 切换 |
| **S1 骨架** | `units.ts` / `coords.ts` / `model.ts` + 坐标单测（§4.5） | 坐标单测全绿；`AC-4` 的 golden fixture 建立 |
| **S2 领域层** | 线长公式 P/R/S/T（配置化，NFR-8）+ 导入器（模板驱动）+ 校验规则 | `AC-6` 5000 连线重算 <3s |
| **S3 表格** | AG Grid + 自研 L3 Excel 交互层 | `NFR-2` 60fps |
| **S4 2D** | 平面图 → U 位图 → 走线 → 导出 | `AC-3` / `AC-4` |
| **S5 3D** | 场景 + instanced 渲染 + 拾取 + 简化走线 | `AC-7` ≥30fps |

> **最重要的一条建议**：**S0 spike 用可丢弃的原型代码做，不要往主工程里写。** 我在 §2/§3/§6 给的性能结论是基于元素量推算的（draw call 账、SVG 节点预算），**推算需要实测确认**。spike 的唯一目的就是把推算变成实测数字。

---

## 10. 来源列表

所有事实性结论的出处。采样时间统一为 **2026-10-20**（Asia/Shanghai）。npm 元数据通过 `https://registry.npmjs.org/<pkg>` 直接读取；GitHub 数据通过 `api.github.com` 读取；官方文档为直接抓取的页面原文。

### 10.1 npm registry（版本 / 发布日期 / license 字段）

| 包 | 版本 | 发布日期 | license | 来源 |
|---|---|---|---|---|
| ag-grid-community | 36.2.0 | 2026-09-16 | MIT | https://registry.npmjs.org/ag-grid-community |
| ag-grid-enterprise | 36.2.0 | 2026-09-16 | Commercial | https://registry.npmjs.org/ag-grid-enterprise |
| ag-grid-react | 36.2.0 | 2026-09-16 | MIT | https://registry.npmjs.org/ag-grid-react |
| handsontable | 18.1.2 | 2026-10-07 | SEE LICENSE IN LICENSE.txt | https://registry.npmjs.org/handsontable |
| @handsontable/react | 16.2.0 | — | SEE LICENSE IN LICENSE.txt | https://registry.npmjs.org/@handsontable/react |
| hyperformula | 3.5.0 | — | GPL-3.0-only | https://registry.npmjs.org/hyperformula |
| @tanstack/react-table | 9.3.1 | 2026-10-12 | MIT | https://registry.npmjs.org/@tanstack/react-table |
| @tanstack/react-virtual | 3.14.21 | — | MIT | https://registry.npmjs.org/@tanstack/react-virtual |
| @glideapps/glide-data-grid | 6.0.3 | 2024-02-03 | MIT | https://registry.npmjs.org/@glideapps/glide-data-grid |
| react-data-grid | 7.0.0-beta.62 | 2026-10-19 | MIT | https://registry.npmjs.org/react-data-grid |
| @revolist/revogrid | 4.29.0 | — | MIT | https://registry.npmjs.org/@revolist/revogrid |
| @univerjs/core | 1.1.0 | 2026-10-17 | Apache-2.0 | https://registry.npmjs.org/@univerjs/core |
| @univerjs/presets | 1.1.0 | 2026-10-17 | Apache-2.0 | https://registry.npmjs.org/@univerjs/presets |
| @univerjs-pro/license | 1.1.0 | 2026-10-17 | **None（未声明）** | https://registry.npmjs.org/@univerjs-pro/license |
| @univerjs-pro/sheets-chart | 1.1.0 | 2026-10-17 | **None** | https://registry.npmjs.org/@univerjs-pro/sheets-chart |
| @univerjs-pro/sheets-pivot | 1.1.0 | 2026-10-17 | **None** | https://registry.npmjs.org/@univerjs-pro/sheets-pivot |
| @univerjs-pro/sheets-print | 1.1.0 | 2026-10-17 | **None** | https://registry.npmjs.org/@univerjs-pro/sheets-print |
| @univerjs-pro/exchange-client | 1.1.0 | 2026-10-17 | **None** | https://registry.npmjs.org/@univerjs-pro/exchange-client |
| @univerjs-pro/collaboration | 1.1.0 | 2026-10-17 | **None** | https://registry.npmjs.org/@univerjs-pro/collaboration |
| @univerjs-pro/edit-history | 1.1.0 | 2026-10-17 | Apache-2.0 | https://registry.npmjs.org/@univerjs-pro/edit-history |
| @univerjs/preset-sheets-advanced | 1.1.0 | 2026-10-17 | **None** | https://registry.npmjs.org/@univerjs/preset-sheets-advanced |
| exceljs | 4.4.0 | 2023-10-19 | MIT | https://registry.npmjs.org/exceljs |
| @zurmokeeper/exceljs | 4.4.9 | 2025-02-26 | MIT | https://registry.npmjs.org/@zurmokeeper/exceljs |
| **xlsx (SheetJS CE)** | **0.18.5** | **2022-03-24** | Apache-2.0 | https://registry.npmjs.org/xlsx |
| @e965/xlsx | 0.20.3 | 2025-04-08 | Apache-2.0 | https://registry.npmjs.org/@e965/xlsx |
| write-excel-file | 4.1.1 | 2026-08-20 | MIT | https://registry.npmjs.org/write-excel-file |
| xlsx-js-style | 1.2.0 | 2022-08-22 | Apache-2.0 | https://registry.npmjs.org/xlsx-js-style |
| node-xlsx | 0.24.0 | 2025-03-14 | Apache-2.0 | https://registry.npmjs.org/node-xlsx |
| three | 0.186.1 | 2026-09-24 | MIT | https://registry.npmjs.org/three |
| @react-three/fiber | 9.8.2 | — | MIT | https://registry.npmjs.org/@react-three/fiber |
| @react-three/drei | 10.7.9 | — | MIT | https://registry.npmjs.org/@react-three/drei |
| @react-three/postprocessing | 3.2.1 | — | MIT | https://registry.npmjs.org/@react-three/postprocessing |
| three-mesh-bvh | 0.9.16 | — | MIT | https://registry.npmjs.org/three-mesh-bvh |
| troika-three-text | 0.52.5 | — | MIT | https://registry.npmjs.org/troika-three-text |
| babylonjs / @babylonjs/core | 9.31.0 | — | Apache-2.0 | https://registry.npmjs.org/@babylonjs/core |
| @tresjs/core | 5.9.3 | — | MIT | https://registry.npmjs.org/@tresjs/core |
| pixi.js | 8.22.0 | — | MIT | https://registry.npmjs.org/pixi.js |
| konva | 10.9.0 | — | MIT | https://registry.npmjs.org/konva |
| react-konva | 19.3.2 | — | MIT | https://registry.npmjs.org/react-konva |
| fabric | 7.4.0 | — | MIT | https://registry.npmjs.org/fabric |
| d3 | 7.9.0 | — | ISC | https://registry.npmjs.org/d3 |
| echarts | 6.1.0 | — | Apache-2.0 | https://registry.npmjs.org/echarts |
| echarts-gl | 2.1.0 | — | MIT | https://registry.npmjs.org/echarts-gl |
| @antv/g6 | 5.1.1 | — | MIT | https://registry.npmjs.org/@antv/g6 |
| elkjs | 0.12.0 | 2026-07-17 | **EPL-2.0 OR GPL-3.0-or-later** | https://registry.npmjs.org/elkjs |
| libavoid-js | 0.5.0-beta.5 | 2026-02-23 | **LGPL-2.1-or-later** | https://registry.npmjs.org/libavoid-js |
| @dagrejs/dagre | 3.1.1 | — | MIT | https://registry.npmjs.org/@dagrejs/dagre |
| d3-dag | 1.2.2 | — | MIT | https://registry.npmjs.org/d3-dag |
| cytoscape | 3.34.4 | — | MIT | https://registry.npmjs.org/cytoscape |
| @xyflow/react | 12.12.0 | — | MIT | https://registry.npmjs.org/@xyflow/react |
| pathfinding | 0.4.18 | 2016（停更） | MIT | https://registry.npmjs.org/pathfinding |
| rbush | 4.0.1 | — | MIT | https://registry.npmjs.org/rbush |
| flatbush | 4.6.2 | — | ISC | https://registry.npmjs.org/flatbush |
| zustand | 5.0.15 | — | MIT | https://registry.npmjs.org/zustand |
| immer | 11.1.21 | — | MIT | https://registry.npmjs.org/immer |
| zod | 4.6.7 | — | MIT | https://registry.npmjs.org/zod |
| jspdf | 4.2.1 | 2026-03-17 | MIT | https://registry.npmjs.org/jspdf |
| svg2pdf.js | 2.8.1 | 2026-08-31 | MIT | https://registry.npmjs.org/svg2pdf.js |
| html-to-image | 1.11.13 | 2026-08-20 | MIT | https://registry.npmjs.org/html-to-image |
| dom-to-image-more | 3.11.0 | 2026-07-24 | MIT | https://registry.npmjs.org/dom-to-image-more |

### 10.2 许可证原文（本案判定依据）

| 结论 | 依据 | URL |
|---|---|---|
| AG Grid Community = MIT；仅 Enterprise 收费 | 官方「Licence & Pricing」页（含完整 Community/Enterprise/Bundle feature matrix）+ 各功能文档 `.md` 的 `enterprise:` frontmatter | https://www.ag-grid.com/license-pricing/ |
| AG Grid Enterprise 价格 $999/dev，永久 license 含 1 年更新 | 同上 | https://www.ag-grid.com/license-pricing/ |
| 剪贴板功能仅在 Enterprise | 文档页标 `enterprise: true` | https://www.ag-grid.com/javascript-data-grid/clipboard/ |
| Server-Side Row Model 是 Enterprise，Infinite Row Model 是 Community | 两份文档 frontmatter 对比 | https://www.ag-grid.com/javascript-data-grid/server-side-model/ |
| AG Grid 仓库许可构成（社区 MIT / 企业 Commercial） | `LICENSE.txt` 原文 | https://github.com/ag-grid/ag-grid |
| Handsontable = 双许可，商用须买 license | `handsontable@18.1.2` tarball 内 `package/LICENSE.txt` 原文 | https://registry.npmjs.org/handsontable/-/handsontable-18.1.2.tgz |
| Handsontable 按「终端用户数」计价；HyperFormula 独立 GPLv3/商业授权；可离线部署 | 官方 pricing 页 | https://handsontable.com/pricing |
| Univer 社区版 Apache-2.0 | LICENSE 全文（GitHub contents API base64 解码核验） | https://github.com/dream-num/univer/blob/dev/LICENSE |
| Univer Pro 功能需注册 `UniverLicensePlugin` + 商业 license | 官方 license 文档原文 | https://docs.univer.ai/server/license.md |
| Univer 定位为「Office SDK（Web/Server/AI）」 | 官方 llms.txt | https://docs.univer.ai/llms.txt |
| Babylon.js = Apache-2.0 | GitHub license 元数据 spdx_id | https://github.com/BabylonJS/Babylon.js |
| SheetJS CE = Apache-2.0 | npm license 字段 + 官方 CDN 包内 LICENSE | https://registry.npmjs.org/xlsx |
| ExcelJS = MIT | npm license 字段 | https://registry.npmjs.org/exceljs |
| react-data-grid 实为 MIT（GitHub API 报 NOASSERTION 是检测瑕疵） | 仓库 `LICENSE` 文件正文（`The MIT License (MIT)` + `Permission is hereby granted`，含 2014 Prometheus Research / 2015 Comcast 双版权行），经 contents API base64 解码核验；npm `license` 字段亦为 MIT | https://github.com/Comcast/react-data-grid/blob/main/LICENSE （注：旧地址 `adazzle/react-data-grid` 已 301 永久迁移至 `Comcast/react-data-grid`） |
| elkjs = EPL-2.0 OR GPL-3.0-or-later | npm license 字段 | https://registry.npmjs.org/elkjs |
| libavoid-js = LGPL-2.1-or-later | npm license 字段 | https://registry.npmjs.org/libavoid-js |

### 10.3 维护状态（GitHub / 官方仓库）

| 结论 | 依据 | URL |
|---|---|---|
| ag-grid 活跃（pushed 2026-10-20，15,645★） | GitHub API repo 元数据 | https://github.com/ag-grid/ag-grid |
| handsontable 活跃（pushed 2026-10-20，22,068★） | 同上 | https://github.com/handsontable/handsontable |
| glide-data-grid **事实停更**（pushed 2026-01-21，latest 停在 2024-02-03，5,403★） | GitHub repo 元数据 + npm dist-tags | https://github.com/glideapps/glide-data-grid |
| TanStack Table 活跃（pushed 2026-10-12，28,506★） | GitHub API | https://github.com/TanStack/table |
| **exceljs 事实停更**（master HEAD `5bed18b45e` @ 2024-01-12；`commits?since=2025-01-01` 返回 0 条；15,490★） | GitHub commits API | https://github.com/exceljs/exceljs/commits/master |
| **SheetJS/sheetjs 已迁走**（pushed 2024-04-18；描述 "New home https://git.sheetjs.com/SheetJS/sheetjs"；36,344★） | GitHub API | https://github.com/SheetJS/sheetjs |
| SheetJS 新家仍在维护（最新 commit `d2f2e17963` @ 2026-02-09 "sheet_to_html sanitize links option (fixes #3345)"） | Gitea commits API | https://git.sheetjs.com/SheetJS/sheetjs |
| SheetJS 权威分发为 CDN（0.20.3 tarball HTTP 200，2,409,319 bytes） | curl HEAD 实测 | https://cdn.sheetjs.com/xlsx-0.20.3/xlsx-0.20.3.tgz |
| three.js 活跃（116,648★） | GitHub API | https://github.com/mrdoob/three.js |
| react-three-fiber 活跃（33,095★） | GitHub API | https://github.com/pmndrs/react-three-fiber |
| Babylon.js 活跃（26,200★） | GitHub API | https://github.com/BabylonJS/Babylon.js |
| TresJS（3,770★，Vue 生态） | GitHub API | https://github.com/Tresjs/tres |
| Univer 活跃（22,691★） | GitHub API | https://github.com/dream-num/univer |

### 10.4 官方文档（技术结论依据）

| 用于支撑的技术结论 | 文档原文要点 | URL |
|---|---|---|
| InstancedMesh 的用法（`setMatrixAt` / `setColorAt`） | three.js API 文档 | https://threejs.org/docs/#api/en/objects/InstancedMesh |
| r3f 版本与 React 版本配对关系（`r3f@8`→`react@18`，`r3f@9`→`react@19`） | 安装页原文 | https://r3f.docs.pmnd.rs/getting-started/installation |
| Babylon Thin Instances 的 JS 开销优势与"全有或全无"代价 | 「regular instances still have a performance penalty on the JavaScript side because each instance is its own object… Thin instances don't create new objects… either all thin instances are drawn … or none are」 | https://doc.babylonjs.com/features/featuresDeepDive/mesh/copies/thinInstances |
| Babylon GreasedLine（任意宽度、始终面向相机的 3D 线） | 「a special type of line that uses a regular mesh with triangles to display a line of any width… shaders designed to ensure that the mesh always faces the camera」 | https://doc.babylonjs.com/features/featuresDeepDive/mesh/creation/param/greased_line |
| Konva 在 shape 数量增长时需要一整套性能调优 | 官方性能指南开篇即列 Layer 管理 / `listening(false)` / caching 等调优项 | https://konvajs.org/docs/performance/All_Performance_Tips.html |
| three-mesh-bvh 的定位（BVH 加速 raycast） | 仓库 README | https://github.com/gkjohnson/three-mesh-bvh |
| SheetJS 官方安装方式（CDN tarball） | 官方安装文档 | https://docs.sheetjs.com/docs/getting-started/installation/nodejs |

### 10.5 需求与数据基线（本仓库内文件）

| 文件 | 用途 |
|---|---|
| `docs/requirements/01-需求规格说明书.md` | FR-6/FR-7/NFR-1~3/AC-3/AC-4/AC-6~8、§3.1 空间模型、§3.4 线长公式、§3.5 五种版式、§7.2 脏数据基线 |
| `信息港部署上架/`（12 个 xlsx/pptx 交付物） | `AC-3`/`AC-4` golden fixture 的原始来源；V13 的 25 列表头是导出格式的验收基准 |

### 10.6 未能完全核实的项（诚实标注）

| 项 | 状态 | 建议 |
|---|---|---|
| `svg2pdf.js` 对 `<pattern>` / `<marker>` / 外部 CSS 的支持度 | **未实测** | 半天 spike；不达标则改用合并 `<path>` 画 U 刻度（§2.2 已有等价的 B 方案） |
| 纯 SVG 在 3000 设备 + 5000 连线下的真实帧率 | **推算，未实测**（依据是节点数与已知浏览器行为） | S0 spike 必测项（§9.4） |
| three.js InstancedMesh raycast 在 3000 实例下的真实耗时（推算 0.1~0.5ms） | **推算，未实测** | S0 spike 必测项 |
| Univer 社区版与 Pro 版的完整功能对照表 | **官方未公开** | 若未来评估 Univer，需要向官方索取 feature matrix |
| AG Grid 36.x 与 React 19.3 的精确兼容性 | npm peer 未限制上限，但未实测 | S0 一起验证 |

> 本文件的推算部分集中在"性能数字"，均已在 §9.4 的 S0 spike 中列出验证方式。**推算不是结论，spike 才是。**
