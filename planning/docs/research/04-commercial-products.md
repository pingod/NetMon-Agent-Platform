# 商用 DCIM / 布线规划产品竞品调研与功能优先级

> 调研目的：为「IDC 上架规划 + 布线管理工具」判断**必须有的功能**、**可放弃的功能**、**差异化机会**，并给出中国区可交付的现实约束。
> 调研方法：只采用厂商官方页面 / 官方数据表 / 官方文档 / 公开仓库等可核实公开信息，未做实际产品试用。价格一律给出源 URL，查不到的写「未公开」。
> 场景锚点：中国西北某省国资云 IDC 实施交付（2026.4 启动，约 128 台设备 / 5127 条连线 / 2 机房 / 14 通道），承接方目前**全靠 Excel** 交付。实物证据见 `信息港部署上架/`。

---

## 1. 摘要结论（先看这一节）

**结论 1：商用 DCIM 的主战场是「运维期」的监控与容量，不是「交付期」的布线表。**
国外产品（Sunbird、Nlyte、Schneider、Vertiv、华为）功能重心高度一致地落在动环监控、PUE/能耗、容量规划、资产全生命周期、变更工单上。**「自动生成布线表 + 精确线长计算」在几乎所有商用产品里都是弱项或半个功能**——它们能记录一条已经存在的链路（连线管理），但很难在**还没施工**的时候，从零把 5127 条连线一次性推出来、算准每根线的长度、并按施工队的习惯排好序导出。

**结论 2：真正把「布线表 + 线长」做好的两类产品，都不在 DCIM 主赛道。**
- **路径规划类**（Pathfinder / NetZoom）：功能表里明确有 "Generation of patch lists"（生成跳线表）、"Cable properties with information on cable lengths"、"Import patches from Excel and .csv"。但它们同时背着一个巨大的资产器库/图元库（Pathfinder 有 **30,000+ 设备模板**），学习成本高，且**不是为"中国施工队拿一张 Excel 去干活"设计的**。价格见 2.7 / 2.8 与 4.1 价格汇总表：**Pathfinder 月租 360 € 起、买断 4,000 € 起；NetZoom 起价 $700/用户/年**。
- **电子配线架（AIM）类**（CommScope imVision、普天智能配线）：靠**智能配线架 + 智能跳线**硬件自动映射端口连接、端到端链路追踪、LED 寻线。**精度极高，但要求甲方买硬件**，纯规划阶段（设备还没上架、配线架还没装）完全用不上。

**结论 3：中国区客户的"最终交付物是 Excel"不是落后，是刚需，且被产品设计普遍忽视。**
国内厂商（华为 NetEco、共济 xbrother、维谛 Trellis-Facility）都提供报表与导出，但导出的多是**平台报表（PDF）或台账视图**，不是施工班组能直接照着干活的**多列宽表**。客户真正要的是 `信息港部署上架/` 里那种 24 列、5127 行、带公式的「布线表」——本端设备/机柜/U位/接口 ↔ 对端设备/机柜/U位/接口、线缆类型、跨机柜、间隔机柜、机柜数量、机内线缆、柜间线缆、线缆长度 1/2、本端模块、对端模块、备注。

**结论 4：「表格驱动 + 一键生成布线表」是可成立的差异化定位，但护城河不在"生成"，在"双向保真"。**
- 这个细分**已经有人做**：开源项目 `bangbang8000-cell/AutoLink`（MIT，TypeScript，2026-07 建仓）已实现"输入 GPU 规模 → 一键出拓扑 / 布线指导表 / BOM / 机柜表 + 9 章 PDF"，且完全离线。但它走的是**模板 + 生成式**路线（面向 1024 GPU 这类标准化智算集群），**不是"已有异构设备 + 真实机柜位置 + 在 Excel 里迭代 6 版"的路线**。
- 商用 DCIM 则站在另一端：**系统内编辑**，Excel 只是导出产物，导回去就丢信息。
- 中间的空白：**让 Excel 继续当权威数据源，系统负责"校验 + 计算 + 重排版 + 回写"**。谁能做到 `导出 → 施工队改 → 导回 → 不丢列、不丢公式、不丢合并单元格、还能 diff 出改了什么`，谁就切走了这块市场。

**结论 5：交付形态必须是「私有化 + 完全离线 + 单机可跑」，这是硬约束不是偏好。**
国资云/政务云项目普遍要求内网隔离、等保合规，SaaS 形态在这些项目里直接出局。AutoLink 在文档里把"不需要联网，所有规划、计算、校验均在本地完成"写进 Q&A，说明**离线本身就是卖点**。

---

## 2. 国外产品分析

### 2.1 Sunbird dcTrack / Power IQ（美国，现属 Vertiv）

产品拆成两条线，定价也按两条线算，是**唯一把价格明码标在官网**的国外厂商：

| 产品 | 计价单位 | 单价 | 折年 |
|---|---|---|---|
| Power IQ（监控） | node（节点/机柜 PDU 等设施） | $5.50 / node / 月 | $66.00 / node / 年 |
| dcTrack（运维/资产/连接） | cabinet（机柜） | $19.50 / cabinet / 月 | $234.00 / cabinet / 年 |
| DCIM Suite（两条线打包） | cabinet | $27.50 / cabinet / 月 | $333.00 / cabinet / 年 |

- 计费口径："1 DCIM Operations cabinet license / 2 DCIM Monitoring node licenses"，两个 node 用于支撑每个机柜的两条网络 PDU；额外的 PDU、UPS、RPP、CRAC、列头柜、机架转换开关需另买 node 许可。
- 许可策略："all features and functionality are included with no limit on the number of sites, users, square footage, or integration points"，即**不按用户数、不按面积、不按站点数**，按机柜/节点走量，"pay as you grow"。另有**永久许可 + 年维护**（Perpetual License + Annual Maintenance）可选。
- 来源：<https://www.sunbirddcim.com/pricing>
- 能力面：资产全生命周期、容量、连接（connectivity）文档化、机房/机柜视图与 "bird's eye view"、3D 可视化搜索。
- **对本项目的含义**：`$234/机柜/年` 是最干净的锚点。信息港这种量级（2 机房、约 181×2 架位、首期十几架）**按机柜买的钱很少**，但 Sunbird 依然解决不了"施工前生成 5127 行布线表"这件事——它的连接管理是**记录已存在的连接**。

### 2.2 Nlyte（美国）

- **价格：未公开，只做定制报价。** 官网原文：\"Nlyte pricing is tailored to your operational goals, scale, and deployment model\"；报价因子列出：设施数量与规模、基础设施资产与电力容量、所需模块、部署形态与集成、支持与服务等级。来源：<https://www.nlyte.com/company/pricing/>
- 能力面（<https://www.nlyte.com/solutions/data-center-infrastructure-management-dcim/>）：资产生命周期管理、容量规划、电力与环境监控、能耗优化、变更管理与工作流自动化、与 ServiceNow 等 ITSM/CMDB 同步、可视化（机柜布局、电力路径、机柜利用率、热力分布、**connectivity diagrams**）、合规与报表。
- 值得注意的是它把**物理盘点做成一项人工服务**：派人到现场做非接触式盘点，按 "Nlyte-prescribed template" 结构化交付再上传系统；采集粒度包括机柜位置、RU 位置、前后安装朝向、刀片/模块在机箱内的位置、序列号。来源：<https://www.nlyte.com/physical-inventory/>
  → 这说明连头部厂商都承认**"把现场数据搞进系统"是瓶颈**，且默认解法是"用模板（≈Excel）收集"。
- **对本项目的含义**：Nlyte 的「模板化采集 → 上传」流程，正是我们要用产品替代的人工环节。

### 2.3 Device42（美国，现属 Freshworks）

- 计价：年度订阅，**按设备数量（number of devices）**。"Device42 pricing is designed to be simple and flexible. Our software is offered as an annual subscription model, based on the number of devices"。具体单价未公开。来源：<https://www.device42.com/pricing/>
- 强项在 **CMDB / 自动发现（discovery）**：靠网络扫描自动发现设备与依赖关系，反过来补 IPAM 与资产台账。
- **对本项目的含义**：自动发现解决的是**存量机房**问题，对**新建交付**（设备在箱子里、还没上电）完全无效。这恰恰是我们场景的边界条件。

### 2.4 Schneider Electric EcoStruxure IT（法国）

- 产品分 IT Expert（监控/可视化/告警）、IT Advisor（规划/仿真，需单独采购）、IT Optimum 等。官网产品页：<https://www.se.com/us/en/product-range/65972-ecostruxure-it-expert/>
- 定位是**监控与可用性**：环境监测、告警、资产视图、远程诊断；30 天试用。**单价未公开。**
- 端口级连线管理、布线表、线长计算：**不在其公开能力列表内**（其物理层能力更多依赖 APC 硬件与 Data Center Expert 本地监控设备）。
- 部署形态：SaaS 为主，同时有本地部署的 Data Center Expert。
- **对本项目的含义**：施耐德是"卖硬件顺带卖软件"的典型，软件是**硬件入口**，不解决交付期文档问题。

### 2.5 Vertiv Trellis-Facility（美国）

- 有中文数据表（<https://www.vertiv.com/globalassets/products/monitoring-control-and-management/monitoring/trellis-facility-dcim-system/trellis-facility-.pdf>），功能模块：组态监控、3D 监控、门禁管理、PUE 展示、温度云图、告警管理、报表管理、配电管理、视频监控、容量管理、增值服务管理、接口服务管理；采集平台覆盖动力设施、空调设施、机房环境安全消防、机柜微环境。
- 明确卖点：**3D 组态技术 + 虚拟现实漫游**、**一键式生成运维报表**、2D 组态在线配置、3D 告警巡游。
- **对本项目的含义**：3D 是这类产品的"平台科技感"卖点，但它是**给领导看的**，不是给施工队用的。**3D 不该进 P0。**

### 2.6 CableSolve（美国，历史产品）

- 定位：企业级基础设施管理系统，"portable, end-to-end management and documentation of the IT infrastructure and **network connectivity**"。
- 关键能力（2005 年产品手册，<https://ww1.prweb.com/prfiles/2005/05/24/244521/NEWCSBrochure050412lowres.pdf>）：桌面端 + **Pocket PC 便携端**（条码枪扫码记录）、连接与资产统一文档化、诊断与告警、未授权变更告警、**"powerful CableSolve reporting and Import/Export tools"**、**第三方系统交换用 CableSolve API**、可定义用户访问权限。
- **现状：产品线已停止演进，当前商用归属与定价未公开（待核实）。**
- **对本项目的含义**：CableSolve 在 2005 年就想清楚了"**移动端扫码 + 导入导出 + API**"这套组合，20 年后国内项目还在用 Excel 手工抄——说明**需求真实存在，只是没有被本地化产品满足**。

### 2.7 NetZoom（美国，NetZoom Inc. / 经销商 ERP Systems US）

- 许可模式（官方购买页 <https://netzoom.com/products/nzenterprise-purchase.html>）：
  - **On-Premises 按用户计价**，站点/机房/机柜/设备/端口**不限量**；
  - **SaaS 按机柜计价**，**用户不限量**；
  - "All modules are included in the purchase price except DCM which is an extra charge"（除 DCM 模块外全包）。
- 具体价格官网不列，但经销商页面公开了起价（<https://erpsystemsus.com/products-dcim>）：**NetZoom 起价 $700/用户，12 个月订阅**；**NetZoom Pro 起价 $1500/用户**（本地或 SaaS，12 个月订阅）；NetZoom DC 需询价。
- 能力面：资产（有源/无源、机柜、配线架）、图形化组件与安装卡、机柜复制向导、**组件互换时自动重映射线缆连接**、按规则自动命名、变更记录。
- **对本项目的含义**：`$700/用户/年` 说明**按人卖**在中小规模下比按机柜卖更贵（10 个用户就是 $7000/年）。

### 2.8 Pathfinder（德国 tripunkt GmbH）— 布线能力最完整的商业产品

- 功能清单（<https://www.pathfinder.de/en/fact-sheet/>）几乎逐条命中我们的需求：
  - 一/二/三级布线文档化、跨站点布线、LAN/光纤/WAN/电话/**电力**链路建模；
  - **"Generation of patch lists"（生成跳线表）**、**"Import patches from Excel and .csv files"**、**"Import and Export cable connection records"**；
  - 端口状态（已跳接/预留/故障/空闲）、**端口矩阵（Port-Matrix mapping）**、拖拽跳接与重跳接、批量跳接向导；
  - **"Cable properties with information on cable lengths, qualities"**、光纤/铜缆类型与连接状态统计、**预留跳线与安装线缆数量的计算（Calculation of the reserved patch and installation cables）**；
  - 按层（站点/楼/层/房间）逐节点图形化、完整物理路径（含面板、插座、连接器等无源中间件）；
  - 校验规则（连接器、介质、环路、芯数/纤数）。
- **价格（公开，三档，<https://www.pathfinder.de/en/pricing/>）**：

  | 版本 | 价格 | 用户数 | 说明 |
  |---|---|---|---|
  | Pathfinder Rental | **from 360 € / month** | 多用户 | 含维护（支持/更新）、可按月取消、含用户组管理与 AD/LDAP/SSO |
  | Pathfinder Full | **价格需询价（Price quotation upon request）** | 多用户 | 面向中小企业与大型企业，可选维护（+20%） |
  | Pathfinder Lite | **from 4,000 €**（一次性） | 单用户 | 入门版，**不支持附加模块、不含企业特性**（即无 AD/LDAP/SSO） |

  三档共同点：完整基础功能、**不限 IT 资产数量**、**30,000+ 设备模板库**。
- **对本项目的含义**：**这是"功能上已经有人做到了"的最强证据**——但它是德国工程软件：图元库庞大（30,000+ 模板，这正是我们要做但很难做的东西）、界面重、本地化差、实施周期长。`360 €/月` 和 `4,000 € 买断` 说明**这个市场的价格天花板并不高**，客户不会为一个布线工具付几十万。**它能证明需求成立，不能证明中国市场被满足。**

### 2.9 开源对照：NetBox（作参照系）

- 有完整的 cable 数据模型，且**原生支持多位置（multi-position）线缆与 MPO 分线**：
  - `Profile` 字段支持 `Straight (single position)`、`Straight (multi-position)`、`Shuffle (2x2 MPO8)`、`Shuffle (4x4 MPO8)`；
  - 文档原文："The profile determines the mapping of termination between the two ends and enables logical tracing across complex connections, such as **breakout cables**"；"A single-position cable is allowed only one termination point at each end. There is no limit to the number of terminations a multi-position cable may have."
  - 来源：<https://netboxlabs.com/docs/netbox/models/dcim/cable/>
  - **这条对我们的 MPO 场景极其重要**：信息港布线表里 `线缆类型=MPO` 的一行，物理上是一根 8/12 芯主干分到多个端口，NetBox 的数据模型可以直接借鉴。
- cable 有 `Length` 字段："The numeric length of the cable, including a unit designation (e.g. 100 meters or 25 feet)."（<https://netbox.readthedocs.io/en/stable/models/dcim/cable/>）
  → **但长度是人工填的属性值，系统不做任何计算**，也不从机柜位置推导。
- REST API 按应用分治：`/api/dcim/`、`/api/ipam/`、`/api/circuits/`、`/api/tenancy/` 等（<https://netboxlabs.com/docs/netbox/integrations/rest-api/>）。
- **无原生 3D、无「一键生成布线表」、无线长自动计算**。
- 价值在于**数据模型可以抄**（device / interface / cable / rack / rack-unit / tenant + cable profile），不适合直接当施工交付工具。

---

## 3. 中国区产品分析

### 3.1 华为 iManager NetEco 6000

- 华为数据中心基础设施管理系统，配套 ECC800-Pro 管理单元、iManager-M 移动 App、iCooling 能效调优。
- 能力：**动环监控、能效管理（PUE 报表、电费策略、自定义单位电价）、容量管理（机柜使用率、峰值用电）、多租户**。应用场景覆盖中大型/预制化/微小型数据中心。
- 官方文档示例：<https://support.huawei.com/enterprise/zh/doc/EDOC1100136378/85dae474>
- **端口级连线管理与布线表/线长：官方功能列表与差异表中均未出现相关词条 → 判为 ✘/部分。**
- 价格：**未公开**（随项目报价）。
- **对本项目的含义**：华为强在"机房建好之后怎么运营"，弱在"机房建的过程中怎么把线布对"。

### 3.2 维谛技术 Vertiv（中国区）

- 见 2.5 Trellis-Facility（有完整中文数据表），3D 组态、PUE、告警、报表、容量。
- 另有 Aperture 系列（Vertiv 收购的 DCIM 品牌，现多与 Trellis 合并演进；**Aperture 独立产品线现状待核实**）。
- 价格：**未公开**。
- 强监控、弱布线，与华为同构。

### 3.3 共济科技 xbrother（深圳）DCIM

- 官网：<https://www.xbrother.com/product/dcim>
- 定位："以标准化工业物模型统一治理资产、空间与容量，为设备上架、容量规划与能效优化提供精准的数据决策支撑"。
- 关键能力（**与本项目重合度最高的国内产品**）：
  - 园区/楼栋/机房/**机柜/U位/设备**统一数字化建模；
  - **"多约束智能算法最优机位秒级推荐"**：基于设备规格、连续 U 位需求、供电冗余、制冷阈值、承重限制等多维约束，输出最优机位候选集，**支持一键预占机位与资源容量**；
  - 正向追溯（设备→供电链路/制冷覆盖）与反向定位（供电/制冷节点→关联设备）；
  - 对接现有 CMDB 与流程系统。
- **布线表/线长/端口级连线：公开页面未列为能力 → 判为 部分/✘。**
- 价格：**未公开**。

### 3.4 电子配线架（AIM）类：普天线缆 智能配线系统 / CommScope imVision

- 架构一致：**智能配线架 + 智能跳线 + 管理主机 + 系统软件**。
- 普天描述（<https://www.potel-group.com/products/data-center/intelligent-wiring-system/>）："基于'电子配线架+智能跳线'架构，提供物理层实时可视化管控。系统自动映射端口连接关系，实时追踪资产变更，并在链路异常时秒级告警……实现从'人找端口'到'图点端口'的转变"。
- CommScope imVision 三件套（<https://www.commscope.com/insights/the-enterprise-source/automated-infrastructure-management-aim-the-fact-file/>）：智能铜缆配线架/光纤配线架监控端口占用、控制器实时追踪物理层变化、imVision System Manager 软件把连接数据与网络设备/IP 端点关联；**端到端线路追踪靠按端口按钮点亮两端 LED**；控制器可下发/展示工单。
- **对本项目的含义**：这是"端口管理 + 连线管理"的**天花板答案**，但**必须有硬件**。新建项目在设备上架前根本没有智能配线架，所以**这一类产品与我们的规划阶段不冲突，反而是下游**——我们的布线表可以成为它们施工时的作业指导书。

### 3.5 其他被点名但需澄清的厂商

任务清单里提到科华/科士达、浪潮 DCIM、紫光/新华三 U-Center、云宏、深信服、袋鼠云/明道云（低代码）：

- **科华数据 / 科士达**：主营 UPS、精密空调、微模块（MDC），其"监控平台"多随微模块一体交付，**独立 DCIM 产品线命名与定价未公开（待核实）**。
- **新华三 U-Center**：统一运维平台，强项是**网络设备运维（网管）**而非机房空间/布线；**是否含 port-level cabling 未在公开资料确认（待核实）**。
- **浪潮 DCIM**：随服务器/整机柜交付的机房管理能力为主，**独立商用产品与价格未公开（待核实）**。
- **低代码（明道云/袋鼠云等）**：可以做"资产台账 + 工单"这类表单应用，但**做不了 U 位几何布局、线长计算、5000 行布线表的重排版与校验**。作为"要不要自己写 CRUD"的替代方案值得评估，作为布线工具不合适。

### 3.6 中国区的真实共性（本节回答 lead 的问题 B）

1. **交付物 = Excel，这几乎是行业惯例。** 施工队、监理、审计、甲方档案，全链路只认 Excel：能打印、能批注、能发微信、能归档、能当结算依据（耗材按表算钱）。任何"请登录系统看"的方案在中国区实施现场都会被抵制。
2. **"表格驱动 + 一键出布线表"的产品，国内商用市场基本空白。** 现有产品要么是**平台报表**（PDF/固定模板，不可二次编辑），要么是**硬件依赖的 AIM**，要么是**网管**。没有一家把"Excel 是第一等公民"写进产品定位。
3. **离线内网是刚性前置条件。** 政务云/国资云项目普遍物理隔离，安装包需过安全审核；SaaS、CDN 字体、在线地图、在线 AI 接口一律不可用。
4. **等保 2.0 要求**：账号/口令策略、最小权限、操作审计日志、数据备份恢复——意味着**必须有账号体系与审计日志**，哪怕是单机版。
5. **已有先例**：开源项目 `AIDC AutoLink` 已经用"一键生成布线表"作为卖点，说明**这个卖点在中国市场是被验证过的**（详见 5.3）。

---

## 4. 功能矩阵表（核心交付）

**图例**
- `✔` = 厂商公开资料中明确列为产品能力
- `部分` = 仅有局部能力 / 需搭配硬件 / 需插件或第三方集成 / 公开资料表述不明确
- `✘` = 公开资料中未列为能力
- 说明：本表依据各厂商官方页面与官方数据表判断，**未做实际产品测试**；`部分` 不等于"做不到"，只等于"公开信息不足以判 ✔"。

| 产品 | 机房空间 | 资产台账 | IPAM | 端口管理 | 连线管理 | 布线表/线长 | 工单 | 巡检 | 报表 | 权限 | 导入导出 | API | 3D |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| **Sunbird dcTrack** | ✔ | ✔ | 部分¹ | ✔ | ✔ | 部分² | ✔ | 部分 | ✔ | ✔ | ✔ | ✔ | 部分³ |
| **Nlyte** | ✔ | ✔ | 部分¹ | ✔ | ✔ | 部分² | ✔ | 部分 | ✔ | ✔ | ✔⁴ | ✔ | 部分³ |
| **Device42** | 部分⁵ | ✔ | ✔ | ✔ | ✔ | ✘² | 部分 | 部分 | ✔ | ✔ | ✔ | ✔ | 部分 |
| **Schneider EcoStruxure IT** | 部分 | 部分 | ✘ | 部分 | ✘ | ✘ | 部分 | ✔ | ✔ | ✔ | 部分 | ✔ | 部分⁶ |
| **Vertiv Trellis-Facility** | ✔ | ✔ | 部分 | ✘ | ✘ | 部分⁷ | 部分 | ✔ | ✔ | ✔ | 部分 | ✔ | ✔ |
| **华为 NetEco 6000** | ✔ | ✔ | 部分 | 部分 | ✘ | ✘ | 部分 | ✔ | ✔ | ✔ | 部分 | ✔ | 部分 |
| **共济 xbrother DCIM** | ✔ | ✔ | 部分 | 部分 | 部分 | 部分⁷ | ✔ | 部分 | ✔ | ✔ | 部分 | ✔ | 部分 |
| **CommScope imVision** | ✔ | 部分 | 部分 | ✔⁸ | ✔⁸ | 部分⁹ | ✔ | 部分 | ✔ | ✔ | 部分 | ✔ | 部分 |
| **Pathfinder** | ✔ | ✔ | 部分 | ✔ | ✔ | ✔¹⁰ | 部分 | ✘ | ✔ | ✔ | ✔¹⁰ | 部分 | ✘¹⁶ |
| **NetZoom** | ✔ | ✔ | 部分 | ✔ | ✔ | ✔¹¹ | 部分 | 部分 | ✔ | ✔ | ✔ | 部分 | ✔ |
| **NetBox（开源对照）** | ✔ | ✔ | ✔ | ✔ | ✔ | 部分¹² | ✘ | ✘ | 部分 | ✔ | ✔ | ✔ | ✘¹³ |
| **普天 智能配线系统（AIM）** | 部分 | 部分 | ✘ | ✔⁸ | ✔⁸ | 部分⁹ | 部分 | ✘ | 部分 | 部分 | 部分 | 部分 | ✘ |
| **AIDC AutoLink（开源）** | ✔ | 部分 | ✘ | 部分 | ✔ | ✔¹⁴ | ✘ | ✘ | ✔ | 部分 | ✔¹⁵ | 部分 | ✔ |

**矩阵注释**

1. IPAM 在本类产品里通常是"IP 属性挂在设备/接口上"，或通过集成第三方 IPAM（如 Infoblox、phpIPAM）实现，而非内置完整的地址段规划、分配、回收工作流。
2. "连线管理"✔ 与"布线表/线长"部分 的区别是本报告最重要的一条分界：**能记录已有链路 ≠ 能在施工前生成带长度和排序的布线表**。前者是"事后记账"，后者需要几何模型（机柜位置、通道、走线路径）+ 长度算法 + 施工排序。
3. 有机柜/房间的 2.5D/3D 视图或"bird's eye view"，但深度远不及 Vertiv/AutoLink 的场景化 3D。
4. 具备结构化模板导入（物理盘点按 Nlyte 规定模板收集后上传）。
5. Device42 的机房空间能力弱于其 CMDB 能力，机房/机柜布局不是其核心叙事。
6. EcoStruxure 侧有 3D 视图能力（部分版本/需配套硬件）。
7. 有容量/空间维度的部分"布线/连接"记录能力，但**未把"生成布线表"与"线长计算"列为独立能力**。
8. 依赖智能配线架 + 智能跳线 + 管理主机硬件，精度最高但**规划阶段不可用**。
9. 能导出链路/端口清单，但导出物是**平台报表**，不是可交付施工的宽表。
10. Pathfinder 明确有 "Generation of patch lists"、"Import patches from Excel and .csv"、"Cable properties with information on cable lengths"、"Calculation of the reserved patch and installation cables"。
11. NetZoom 有"组件互换时自动重映射线缆连接"与机柜复制向导，布线能力偏"配线与资产映射"。
12. NetBox 的 cable 有 `Length` 字段（"The numeric length of the cable, including a unit designation (e.g. 100 meters or 25 feet)"），但**长度是人工填的属性值，系统不计算**，也不从机柜几何位置推导；无"一键生成布线表"。其 cable `Profile` 已原生支持 `Straight (multi-position)` / `Shuffle (2x2 MPO8)` / `Shuffle (4x4 MPO8)` 这类 MPO 分线模型。
13. NetBox 原生无 3D，需社区插件。
14. AutoLink 输出"布线指导表"，但它是**从设计拓扑生成的指导表**，不是"从真实机柜点位反算线长"的施工布线表。
15. AutoLink 导出 Excel（连接表/布线指导表/BOM/设备清单/机柜表）+ 9 章 PDF。
16. Pathfinder 只有 2D 平面/立面与图形库（支持 dwg/dxf/svg/png 底图、无限缩放设计器），**功能全表中 "3D" 出现 0 次，无 VR/AR**。这反证：**布线做得好不好，与有没有 3D 无关**。

**矩阵读数（三条）**

- **机房空间 + 资产台账 + 报表 + 权限**：13 个产品里几乎人人 ✔ → **这是入场券，不是差异化**。
- **IPAM**：普遍 `部分` → 恰恰是我们容易做出亮点的低成本项（信息港的 IP 地址段规划只有 10 条数据行、网段级，见 `信息港部署上架/` 的 `IP地址段规划` sheet）。
- **布线表/线长**：**没有任何一个产品是干净的 ✔ 且不依赖硬件、且面向中国施工交付**。Pathfinder 与 NetZoom 最接近，但都不是"Excel 一等公民"：Pathfinder 月租 360 € 起 / 买断 4,000 € 起，NetZoom 起价 $700/用户/年。

### 4.1 价格与许可模式汇总（lead 的 E 问题）

**这是唯一一张把 13 个产品价格放在一起的表。币种为官网原币种，不做汇率换算。**

| 产品 | 价格 | 计价单位 | 许可模式 | 部署形态 | 来源 |
|---|---|---|---|---|---|
| **Sunbird Power IQ** | **$5.50 / node / 月**（$66.00 / node / 年） | 节点（机柜 PDU、UPS、RPP、CRAC 等设施） | 订阅 / 可选永久+年维护 | SaaS / 本地 | [sunbirddcim.com/pricing](https://www.sunbirddcim.com/pricing) |
| **Sunbird dcTrack** | **$19.50 / cabinet / 月**（$234.00 / cabinet / 年） | 机柜 | 订阅 / 可选永久+年维护 | SaaS / 本地 | 同上 |
| **Sunbird DCIM Suite** | **$27.50 / cabinet / 月**（$333.00 / cabinet / 年） | 机柜（含 2 node） | 订阅 / 可选永久+年维护 | SaaS / 本地 | 同上 |
| **Pathfinder Rental** | **from 360 € / month** | 未按资产计价（不限资产数） | 月订阅，可月付取消 | 本地 | [pathfinder.de/en/pricing](https://www.pathfinder.de/en/pricing/) |
| **Pathfinder Full** | **需询价** | 不限资产数 | 永久 + 可选维护（+20%） | 本地 | 同上 |
| **Pathfinder Lite** | **from 4,000 €** | 单用户、不限资产数 | 永久 + 可选维护（+20%） | 本地 | 同上 |
| **NetZoom** | **from $700 / 用户**（12 个月订阅） | 用户（资产/机柜/端口不限） | 年订阅 | 本地 | [erpsystemsus.com/products-dcim](https://erpsystemsus.com/products-dcim) |
| **NetZoom Pro** | **from $1,500 / 用户**（12 个月订阅） | 用户（SaaS 按机柜） | 年订阅 | 本地 / SaaS | 同上 |
| **NetZoom DC** | **需询价** | 未公开 | 未公开 | 本地 / SaaS | 同上 |
| **Nlyte** | **未公开（定制报价）** | 按设施数/资产与电力容量/模块组合 | 未公开 | 本地 / SaaS | [nlyte.com/company/pricing](https://www.nlyte.com/company/pricing/) |
| **Device42** | **未公开（按设备数年度订阅）** | 设备数量 | 年订阅 | 本地 / SaaS | [device42.com/pricing](https://www.device42.com/pricing/) |
| **Schneider EcoStruxure IT** | **未公开**（有 30 天试用） | 未公开 | 订阅 | SaaS 为主 + 本地（DCE） | [se.com 产品页](https://www.se.com/us/en/product-range/65972-ecostruxure-it-expert/) |
| **Vertiv Trellis-Facility** | **未公开** | 未公开 | 随项目 | 本地 | [Vertiv 数据表](https://www.vertiv.com/globalassets/products/monitoring-control-and-management/monitoring/trellis-facility-dcim-system/trellis-facility-.pdf) |
| **华为 iManager NetEco 6000** | **未公开** | 随项目 | 随项目 | 本地（配 ECC800-Pro） | [华为文档](https://support.huawei.com/enterprise/zh/doc/EDOC1100136378/85dae474) |
| **共济 xbrother DCIM** | **未公开** | 随项目 | 随项目 | 本地 | [xbrother.com/product/dcim](https://www.xbrother.com/product/dcim) |
| **CommScope imVision / 普天智能配线** | **未公开**（硬件+软件打包） | 按配线架端口数（隐含） | 硬件采购 + 软件许可 | 本地 | [CommScope AIM 说明](https://www.commscope.com/insights/the-enterprise-source/automated-infrastructure-management-aim-the-fact-file/) |
| **CableSolve** | **未公开（产品线已停止演进）** | — | — | 本地 | [2005 产品手册](https://ww1.prweb.com/prfiles/2005/05/24/244521/NEWCSBrochure050412lowres.pdf) |
| **NetBox** | **免费**（开源） | 不限 | 开源许可 | 本地 | [netboxlabs.com](https://netboxlabs.com/docs/netbox/integrations/rest-api/) |
| **AIDC AutoLink** | **免费**（MIT 开源） | 不限 | MIT | 本地、完全离线 | [GitHub](https://github.com/bangbang8000-cell/AutoLink) |

**价格读数（三条结论）**

1. **能查到明确数字的只有 3 家**：Sunbird（按机柜 $234/年）、Pathfinder（360 €/月 或 4,000 € 买断）、NetZoom（$700/用户/年）。**其余全部"未公开"**——这本身就是机会：这个市场报价不透明，客户无法比价。
2. **量级锚点**：按机柜买最便宜（$234/机柜/年 ≈ 一个机柜一年不到 2000 元人民币量级）；按用户买最贵（10 个用户 $7,000/年）；买断制 4,000 € 起。
3. **对本项目的定价启示**：客户是**项目制交付**（一个项目 2 机房、十几架首期），**年订阅在这个场景卖不动**——项目结束就没预算了。合理形态是**按项目/按机房一次性买断 + 可选年维护**，价格量级参考 Pathfinder Lite（≈3 万人民币）往下，做到**万元级**才有竞争力。

---

## 5. P0 / P1 / P2 功能优先级建议

### 5.1 P0（没有它产品不成立，第一期必须做）

| # | 能力 | 理由（来自竞品与实物证据） |
|---|---|---|
| P0-1 | **机柜 U 位点位图（2D 正视图）** | 实物证据：`404房间1-3通道` sheet 用 305 行 × 84 列、**500 个合并单元格**硬画机柜阵列（每柜占 5 列：序号列 + `1U` 标签列 + 3 列内容），每行代表一个 U；`404房间8-9通道` 是同构的 215 行 × 84 列、451 个合并单元格。这是客户当前最痛的手工活，也是所有 DCIM 的入场券。 |
| P0-2 | **连线表 ↔ 布线表的双向生成** | 实物证据：`连线表(不含长度)` 有 **5110 条数据行 × 12 列**（线缆长度列整列为空），`布线表` 有 **5127 条数据行 × 24 列**（含线长与模块）。**同一份数据的两个视图**，客户靠手工维护两份，必然不一致。产品必须做到"一份数据，两种排版"。 |
| P0-3 | **线长计算** | **这是整份报告里最值钱的一条实物证据**。`布线表` 的长度是**算出来的**，链条为：<br>`P2 = N2+O2`（机柜数量 = 跨机柜 + 间隔机柜）→ `R2 = P2*0.6*120%`（柜间线缆 = 机柜数 × 0.6m/柜 × 1.2 冗余）→ `S2 = R2+Q2`（线缆2 = 柜间 + 机内）→ `T2`（线缆长度1）= **数组公式**<br>`=_xlfn.IFS(S2<=5,5, S2<=6,7, S2=7,7, S2<=8.5,10, S2<=13,15, S2<=17.5,20, S2<=22,25, S2<=27,30, S2<=32,35, S2<=37,40)`<br>即**把连续算出的长度向上取整到成品线缆的规格档位（5/7/10/15/20/25/30/35/40 米）**。然后 `U2`（线缆长度）人工覆写为整数（如 15）。<br>这套"几何 → 米数 → 向上取整到商品规格"的规则是**可直接产品化的业务知识**，也是任何 DCIM 都没有内置的。 |
| P0-4 | **Excel 高保真导出（列序、表头、公式、合并单元格、样式）** | 客户最终交付物就是 Excel。导出必须能**直接发给施工队/甲方**，不需要二次加工。这是与所有商用 DCIM 的分水岭。 |
| P0-5 | **Excel 反向导入 + 差异比对** | 现场改表是常态（V3→V13 六版 / 40 天）。必须支持"导回 → 找出改了哪几行 → 增量更新"，否则产品会被 Excel 反噬。 |
| P0-6 | **耗材清单自动汇总** | 实物证据（精确到单元格）：`首期耗材需求清单` C 列 `=COUNTIFS(布线表!X:X,"首期",布线表!M:M,A3,布线表!U:U,B3)` —— 按 **备注1="首期" + 线缆类型 + 线缆长度** 三个条件从 5127 行布线表统计需求数量；`E2` 单元格写着 **"采购数量在需求数量上，增加30%冗余"**，但 D 列采购数量是**人工填的死数**（60/70/130/40/15/30…），**冗余没有公式化**。产品应一键出"线缆需求 → 采购量（含可配置冗余系数）"，并支持"MPO 采购 0 根"这类现场判断。<br>同表还有一条**范围声明**："耗材里，只考虑了业务核心交换机与防火墙连接线缆、负载均衡与防火墙连接线缆，安全设备所需其他线缆未考虑" → **说明人工汇总会漏项**，这正是产品价值。 |

### 5.2 P1（第一期后半或第二期，形成竞争力）

| # | 能力 | 理由 |
|---|---|---|
| P1-1 | **端口级连接管理（端口矩阵）** | 竞品里 Pathfinder/NetZoom/imVision 都有；是我们从"表格工具"升级为"管理系统"的关键。信息港已到 `100GE1/0/1` 这种端口粒度，且存在 **MPO 多通道**（一行 `线缆类型=MPO` 背后是 8/12 芯）——**必须支持 1:N 与通道拆分**。好消息：NetBox 的 cable `Profile`（`Straight (multi-position)` / `Shuffle (2x2 MPO8)` / `Shuffle (4x4 MPO8)`）已经把模型想清楚了，可直接抄（<https://netboxlabs.com/docs/netbox/models/dcim/cable/>）。 |
| P1-2 | **IP 地址段规划（网段级即可）** | 实物证据：`IP地址段规划` 只有 10 条数据行、按"应用范围 → 地址段"登记（交换机互联 / 服务器管理 / 业务 / SDN / 存储 / 带外 / 租户浮动），**不是 IPAM 级别的逐地址分配**。做到"网段规划 + 使用率 + 冲突校验"就够，不要过度设计。 |
| P1-3 | **校验规则引擎** | 可抄的规则集：端口速率与模块匹配（`100G-SR4` ↔ `MPO`）、端口守恒、跨机柜 U 位可达性、线缆类型与接口匹配、同一端口被占用两次。Pathfinder 有 connector/medium/loop 校验，AutoLink 号称 23 条规则（V001–V023）。**规则数量是可视化卖点**。 |
| P1-4 | **报表（PDF/Excel）与耗材 BOM** | 人人都有（矩阵中报表一列几乎全 ✔），属于必备但**不构成差异**。 |
| P1-5 | **权限与审计日志** | 等保 2.0 硬要求（账号/口令策略、最小权限、操作审计、备份恢复）。单机版也要有。 |
| P1-6 | **平面图与通道管理** | 实物证据：`机房平面图` 是 40 列 × 16 行（B2:AN16）的文本网格（"404机房（功率4kW/总181架）丝路信息港业务" / "走道" / "空调" / "柱子" / 机柜编号 1–27），**是格子不是矢量图**。产品做到"网格化平面图 + 机柜编号 + 通道归属"即可，不必上 CAD。 |

### 5.3 P2（差异化/锦上添花，别在第一期做）

| # | 能力 | 理由 |
|---|---|---|
| P2-1 | **3D 机房漫游** | Vertiv/AutoLink 都有，但**是给领导汇报看的**。最强反证：**Pathfinder 是布线能力最完整的商业产品，功能全表里 "3D" 出现 0 次、无 VR/AR**，却依然把跳线表、线长、端口矩阵做到了最好 → **布线做得好不好与有没有 3D 无关**。信息港这类交付项目的验收物是 Excel 和平面图，3D 不解决任何施工问题。投入产出比最低。 |
| P2-2 | **工单 / 巡检** | 商用 DCIM 标配（Nlyte 的 change workflow、imVision 的 controller 工单），但**属于运维期**。本项目是交付期，甲方还没进入运维节奏。 |
| P2-3 | **CMDB / ITSM 对接（ServiceNow 等）** | Nlyte 的核心卖点，中国区项目里对应的是"对接甲方已有运维平台"。**等有第二个客户提出再说。** |
| P2-4 | **自动发现（discovery）** | Device42 的强项，对新建项目无效（设备未上电）。 |
| P2-5 | **多租户 / 容量计费** | 华为 NetEco 强调"提升机柜出租/使用率"，是 IDC 运营商视角。国资云自用场景用不上。 |

---

## 6. 差异化机会：`表格驱动 + 一键生成布线表` 成立吗？

### 6.1 结论：成立，但要重新定义护城河

**成立的三个证据链**

1. **需求侧证据**：客户已经用 Excel 做到第 13 版、40 天迭代 6 版，说明「表格 + 布线表」就是他们的工作方式，**不需要教育市场**。
2. **供给侧证据（商用）**：13 个产品矩阵里，**布线表/线长一列没有任何一个干净的 ✔ 且不依赖硬件**。最接近的 Pathfinder 是德国重软件、价格未公开、本地化差；NetZoom 起价 $700/用户/年。
3. **供给侧证据（开源）**：`bangbang8000-cell/AutoLink`（MIT，2026-07 建仓，4 star）已经验证"一键出布线表/BOM/PDF"在中国市场是**有人认的卖点**，且它把"完全离线"写进 Q&A。

**但"一键生成"本身不是护城河**——AutoLink 已经做了，任何团队三个月都能做出一个生成器。真正的护城河是下面三件事：

| 护城河 | 为什么难被抄 | 对应证据 |
|---|---|---|
| **H1. 双向保真**（导出 Excel → 现场改 → 导回系统，不丢列/不丢公式/不丢合并单元格，且能 diff） | 需要自己实现 Excel 的样式与公式模型，还要处理客户在 Excel 里"乱改"的容错。这是脏活，开源项目和商用 DCIM 都不愿意碰（它们要你"在系统里改"）。 | `布线表` 25 列里有 5 列是公式（`=N2+O2`、`=P2*0.6*120%`、`=R2+Q2`、数组公式）；`首期耗材需求清单` 整列是 `COUNTIFS` |
| **H2. 真实几何 → 真实线长** | 线长不是查表，取决于机柜在房间里的位置、通道走向、跨几个机柜、是否同通道。需要"平面图 → 通道 → 机柜 → U 位"的完整坐标链。 | `机房平面图` 的机柜编号 1–27 按通道排布；`布线表` 有 `跨机柜`、`间隔机柜`、`机柜数量` 三列，说明**长度模型就是按间隔机柜数推的** |
| **H3. 异构设备的型号库** | 客户设备是混编的（业务核心交换机、密评接入交换机、防火墙、负载均衡、服务器……），每台设备的端口命名、模块型号、U 高都不同。AutoLink 的 131 款设备库是为 GPU 集群精选的，**覆盖不了政务云的杂牌军**。 | `布线表` 出现 `100G-SR4` 模块、`100GE1/0/1` 端口、`MPO` 线缆；`安全产品型号参数.xlsx` 是另一套型号资料 |

### 6.2 与 AutoLink 的正面比较（必读，这是最直接的竞品）

| 维度 | AIDC AutoLink | 本项目应走的路 |
|---|---|---|
| 起点 | **规模参数**（GPU 数 64–2048）+ 模板 | **已有 Excel / 已有设备清单** |
| 目标客户 | 智算中心规划院、方案商做**售前方案** | IDC 实施交付团队做**施工文档** |
| 拓扑来源 | 算法生成（Fat-Tree / Rail-Optimized，按收敛比推） | **人工给定 + 系统校验** |
| 布线表 | "布线指导表"，由拓扑推导 | "施工布线表"，由**真实机柜位置 + 端口**推导 |
| 机柜 | 42U/49U 参数化，`rack_layout.json` 自动生成 | 真实机柜编号、通道、U 位、`跨机柜/间隔机柜` |
| 强项 | 生成速度快、23 条校验、131 款设备、9 章 PDF | 保真、可回写、可 diff、异构设备 |
| 定价 | MIT 开源免费 | 待定 |
| 弱点 | 不能承接客户已有的 Excel 资产；政务云设备型号覆盖不足 | 生成能力弱于它，需要补 |

**判断：不是替代关系，是不同工位。** 但**必须在产品叙事上明确切割**，否则客户会说"这个 AutoLink 免费就有"。

### 6.3 建议的产品定位一句话

> **"你的 Excel 不用扔。我们让它会算线长、会查冲突、会一键出布线表和耗材清单，改完还能导回去。"**

---

## 7. 交付形态建议（回答 lead 的问题 D）

### 7.1 私有化单机 + 完全离线 = 刚需，不是偏好

| 约束 | 具体要求 | 依据 |
|---|---|---|
| **网络** | 安装后**零外网依赖**：不加载 CDN 字体/JS、不调在线地图、不调在线 AI、不做许可在线校验 | 政务云/国资云物理隔离；AutoLink 把"不需要联网，所有规划、计算、校验均在本地完成"写进 Q&A 作为卖点 |
| **部署** | 优先**单机可跑**：`Docker Compose` 一键起 或 **单二进制**（Go/Rust 打包前端）或 `pip install` + 本地 SQLite | 实施现场常常只有一台跳板机；客户 IT 不一定给 K8s |
| **数据** | 默认 **SQLite/内嵌库**，可选 PostgreSQL；数据文件可整包备份/迁移 | 项目制交付，数据要能跟着项目走、能归档 |
| **账号** | **必须有**：本地账号 + 角色（管理员/规划员/只读）+ 口令复杂度 + 登录失败锁定 + 操作审计日志 | 等保 2.0 三级对"身份鉴别/访问控制/安全审计"的硬要求；**不要做单点免登录** |
| **许可** | 离线许可文件（绑定机器码）或**干脆不设许可**（项目内交付） | 内网无法做在线激活 |
| **导入导出** | Excel 必须**双向**且保真；另支持 CSV/JSON 便于与甲方系统交换 | 见 6.1 H1 |

### 7.2 是否需要账号体系？

**需要，但可以极简。** 理由不是"多用户协作"，而是**等保合规 + 审计留痕**：
- 交付文档是要归档的，谁在什么时候改了哪条连线，将来出问题要能查。
- 角色只需三档：`管理员`（管账号/项目/配置）、`规划员`（改数据）、`只读`（甲方/监理看）。
- **不要做组织架构、不要做单点登录、不要做复杂的 RBAC 权限矩阵**——那是运维平台的事。

### 7.3 是否需要与 CMDB / 工单系统对接？

**第一期不需要。** 判断依据：
- 本项目是**新建交付**，甲方 CMDB 里还没有这批设备，没有可同步的源。
- 商用产品里 Nlyte 的 ServiceNow 同步是**运维期**价值，不是交付期价值。
- 第一期只需把**数据模型设计成可导出**（设备/机柜/U位/端口/连线/线缆 六张表 + 稳定 ID），将来对接是加一个 importer 的事。
- 反向的机会：**我们的布线表可以成为甲方 CMDB 的初始数据源**——这是比"对接"更好的卖点。

---

## 8. 来源列表

**国外产品**
- Sunbird DCIM 定价（$5.50/node/月、$19.50/cabinet/月、$27.50/cabinet/月、永久许可+年维护）：<https://www.sunbirddcim.com/pricing>
- Nlyte 定价策略（定制报价，不公开）：<https://www.nlyte.com/company/pricing/>
- Nlyte DCIM 能力清单：<https://www.nlyte.com/solutions/data-center-infrastructure-management-dcim/>
- Nlyte 物理盘点服务（模板化采集）：<https://www.nlyte.com/physical-inventory/>
- Device42 定价（按设备数年度订阅，单价未公开）：<https://www.device42.com/pricing/>
- Schneider EcoStruxure IT Expert 产品页：<https://www.se.com/us/en/product-range/65972-ecostruxure-it-expert/>
- Vertiv Trellis-Facility 中文数据表（3D 组态、PUE、报表、容量）：<https://www.vertiv.com/globalassets/products/monitoring-control-and-management/monitoring/trellis-facility-dcim-system/trellis-facility-.pdf>
- NetZoom 购买页（本地按用户/SaaS 按机柜，模块全包除 DCM）：<https://netzoom.com/products/nzenterprise-purchase.html>
- NetZoom 经销商价格（起价 $700/用户、Pro 起价 $1500/用户）：<https://erpsystemsus.com/products-dcim>
- Pathfinder 功能全表（生成跳线表、Excel/csv 导入跳线、线缆长度属性、预留线缆计算）：<https://www.pathfinder.de/en/fact-sheet/>
- Pathfinder 价格与许可（Rental from 360 €/月、Full 询价、Lite from 4,000 €）：<https://www.pathfinder.de/en/pricing/>
- CableSolve 产品手册（Pocket PC 扫码、导入导出、API）：<https://ww1.prweb.com/prfiles/2005/05/24/244521/NEWCSBrochure050412lowres.pdf>

**中国区产品**
- 华为 iManager NetEco 相关产品文档（容量/能效/租户）：<https://support.huawei.com/enterprise/zh/doc/EDOC1100136378/85dae474>
- 共济科技 xbrother DCIM（数字孪生、U 位建模、多约束机位推荐、一键预占）：<https://www.xbrother.com/product/dcim>
- 普天线缆 智能配线系统（电子配线架 + 智能跳线，端口自动映射、秒级告警）：<https://www.potel-group.com/products/data-center/intelligent-wiring-system/>
- CommScope AIM / imVision 说明（智能配线架 + 控制器 + System Manager，LED 寻线、工单）：<https://www.commscope.com/insights/the-enterprise-source/automated-infrastructure-management-aim-the-fact-file/>

**开源对照**
- AIDC AutoLink 仓库（MIT，TypeScript，创建 2026-07-26，4 star）：<https://github.com/bangbang8000-cell/AutoLink>
- AIDC AutoLink 使用文档（一键生成布线指导表/BOM/机柜表 + 9 章 PDF，完全离线）：<http://www.evergreenzhou.com/zh/docs/>
- NetBox Device42/REST API 文档（`/api/dcim/`、`/api/ipam/` 端点层级）：<https://netboxlabs.com/docs/netbox/integrations/rest-api/>
- NetBox Cable 模型（`Length` 字段、`Profile` 支持 `Straight (multi-position)` / `Shuffle (2x2 MPO8)` / `Shuffle (4x4 MPO8)`）：<https://netboxlabs.com/docs/netbox/models/dcim/cable/>
- NetBox Cable Length 字段说明（"The numeric length of the cable, including a unit designation"）：<https://netbox.readthedocs.io/en/stable/models/dcim/cable/>
- NetBox Devices & Cabling 文档（device/interface 建模）：<https://netbox.readthedocs.io/en/stable/features/devices-cabling/>

**本次调研的实物证据（本地文件）**
- `信息港部署上架/V13(内部)甘肃信息港-设备上架规划-20260714.xlsx`
  - `布线表`：24 列 × 5127 条数据行（本端设备/名称/通道/机柜/U位/接口 → 对端同构 → 线缆类型/跨机柜/间隔机柜/机柜数量/机内线缆/柜间线缆/线缆2/线缆长度1/线缆长度/本端模块/对端模块/备注1）
  - `连线表(不含长度)`：12 列 × 5110 条数据行（`线缆长度` 列整列为空，等待回填）
  - `404房间1-3通道` / `404房间8-9通道`：305 行 × 84 列 / 215 行 × 84 列，分别 500 与 451 个合并单元格的 U 位点位图（每柜 5 列：序号 + `1U` + 3 列内容，每行一个 U，共 42U）
  - `机房平面图`：40 列 × 16 行（B2:AN16）文本网格（`404机房（功率4kW/总181架）丝路信息港业务`、`走道`、`空调`、`柱子`、机柜编号 1–27）
  - `IP地址段规划`：10 条数据行 × 2 列（应用范围 → 网段，含交换机互联/服务器管理/业务/SDN/存储/带外/租户浮动；含 `172.30.x.0/24` 段区段与 `172.31.0.0/16`）
  - `首期耗材需求清单`：C 列 `=COUNTIFS(布线表!X:X,"首期",布线表!M:M,A3,布线表!U:U,B3)`；`E2` 备注"采购数量在需求数量上，增加30%冗余"，但 D 列采购数量为人工死数
  - `首期设备清单`：7 列 × 22 条（类型/名称/配置/数量/套餐/使用规划/上联设备），配置列含完整选型参数（交换容量、包转发率、板卡与光模块配置）
  - 版本痕迹：`布线表（旧）`、`404房间1-3通道（旧）`、`404房间8-9通道（旧）`、`首期耗材需求清单（旧）` → **版本管理靠"另存为（旧）"**
- 同目录其他文件（说明交付物矩阵）：`V3/V8/V9/V11/V12/V13` 六个版本、`甘肃信创云-到货829根耗材盘点-20260716.xlsx`、`政务外网区交换机连线表-0606.xlsx`、`安全产品型号参数.xlsx`、`11+12冷通道（示例）.xlsx`、`信息港项目计划表.xlsx`、`甘肃省信创云-丝绸之路信息港拓扑图-20260716.pptx`

### 8.1 真实设备构成（用于校准"型号库"需求）

`首期设备清单` 共 22 条设备类型，覆盖：业务核心交换机、存储核心交换机、管理核心交换机、分线接入交换机、业务接入交换机 1/2、存储接入交换机、带外交换机、带内交换机、网络节点交换机、链路负载均衡、服务器（管理节点/SDN服务节点/GSO独立部署/计算节点A(ARM 鲲鹏920)/计算节点B(C86 海光7390H)/EHDD云硬盘/SSD云硬盘/备份一体机）、密码设备（服务器密码机/签名服务器/国密SSL VPN）。

**三个对产品设计的直接结论**

1. **"异构混编"是常态**：同一机房内 ARM（鲲鹏920）与 C86（海光7390H）两类服务器并存，交换机分业务/存储/管理/带内/带外五张网，还有独立密评设备区。AutoLink 那种"131 款精选 GPU 集群设备库"覆盖不了这种组合。
2. **端口命名与速率极不统一**：既有 `100GE1/0/1` 这种 Cisco 风格，也有 `48*25GE SFP28`、`8*100GE QSFP28`、`4*10GE 光口`、`2*10/100/1000BASE-T 电口`。**产品必须支持"按设备型号定义端口模板"，而不是硬编码端口名。**
3. **光模块型号必须跟着走**：`100G-SR4` 与本端/对端模块列在布线表里是独立字段，且与 `MPO` 线缆强相关（MPO 多模主干 + 分支）。这印证 P1-1 的 1:N 通道模型是**必需**而非可选。

---

## 9. 调研局限（必须明示）

1. **未做产品试用**，矩阵判断基于厂商公开页面/数据表/文档，`部分` 与 `✘` 只代表"公开信息未列为能力"。
2. **以下信息未能核实，标为未公开/待核实**：Nlyte、Schneider、Vertiv、华为、共济、CableSolve、Device42 的具体价格与许可单价（Pathfinder 与 NetZoom 已取得公开价，Sunbird 已取得完整价目）；CableSolve 当前商用归属与产品状态；Vertiv Aperture 独立产品线现状；科华/科士达/浪潮 DCIM/新华三 U-Center 独立 DCIM 能力与价格。
3. **exa 搜索配额受限**，部分中文厂商（科华、科士达、浪潮、新华三、云宏、深信服）未能取得官方功能页，故未在本报告的功能矩阵中给出行，避免臆断。
4. 国外厂商的中国区定价通常按项目折扣，官网美元价只能作为**量级锚点**，不能直接换算成人民币报价。
