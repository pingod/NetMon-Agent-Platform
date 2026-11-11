# 开源 DCIM / IPAM / 布线管理产品与可复用框架调研

> 版本：v1.0
> 调研日期：2026-10-20
> 调研人：dcim-scout（调研员）
> 服务对象：IDC 上架规划 + 布线管理工具的技术路线决策
> 数据核实方式：**所有 Star 数 / 最近提交时间 / 许可证均通过 GitHub REST API 实时查询**（`GET https://api.github.com/repos/OWNER/REPO`），查询时间基准 **2026-10-20**。数据模型结论来自**直接阅读该版本源码与建表 SQL**，不是读宣传文档。
> 配套文档：[需求规格说明书](../requirements/01-需求规格说明书.md)

---

## 1. 摘要结论（先看这一节）

### 1.1 一句话结论

**不要把 NetBox 当底座做二次开发，但一定要把 NetBox 的数据模型当"参考系"和"导出目标"。** 本项目应自研数据模型与线长算法，同时保证能向 NetBox 单向导出/双向同步。

### 1.2 五条支撑结论

| # | 结论 | 关键证据（本文 §5 展开） |
|---|---|---|
| C1 | **"跨机柜线长自动计算 + 布线表派生列"在整个开源生态里是空白** | 全网 GitHub 搜索 + 源码核实：唯一做这件事的是 `gadler01/Netbox_Cable_Calculator_Plugin`，**★2**，个人项目，参数是英寸硬编码 + 统一 overhead，无档位表、无路径规划。NetBox 本体、openDCIM、RackTables、phpIPAM、GLPI、iTop **全部没有**线长计算 |
| C2 | NetBox 的 Cable 模型有 `length` 字段但没有"算"的能力 | `netbox/dcim/models/cables.py:95` Cable 确有 `length`(DecimalField 8,2) + `length_unit`(km/m/cm/mi/ft/in)，但**纯手工录入**；无机柜坐标、无路径、无档位表。用户 Excel 里的 `跨机柜/间隔机柜/机柜数量/机内线缆/柜间线缆/线缆2` 六列派生字段，**任何一个开源 DCIM 都没有对应字段** |
| C3 | NetBox 的插件扩展点在"改数据模型/加页面"上够用，在"替换核心算法"上不够用 | 官方插件文档自己警告：*"the NetBox plugins API is necessarily limited in its scope … Any part of the NetBox code base not documented here is **not** part of the supported plugins API"*。插件是 Django app，可以加模型/加页面/加 API，但**不能改 Cable 的 clean/save 语义，也不能给 Rack 加坐标字段而不动 core** |
| C4 | NetBox vs Nautobot：**选 NetBox 当参考系，不选 Nautobot** | NetBox ★21734 / Apache-2.0 / 2026-10-20 推送；Nautobot ★1628 / Apache-2.0 / 2026-10-20 推送。Nautobot 架构更好（Jobs、App framework、原生 Git 集成），但生态规模差一个数量级（同功能插件 ★数普遍是 NetBox 版本的 1/10~1/30），且本项目不需要 Jobs/自动化引擎 |
| C5 | **可以直接白拿的资产是"枚举体系 + 建模设计"，不是"设备型号数据"** | `devicetype-library` ★1631 确实是 **CC0-1.0（公共领域，无任何使用限制）**，含 12390 个 `device-types` YAML + 2036 个 `module-types` + 147 个 `rack-types`，覆盖 371 个厂商 —— **但实测国内信创设备覆盖率仅 0.97%**（华为 103 个 / 中兴 8 / 浪潮 2 / 瑞斯康达 4 / 锐捷 3，**H3C、绿盟、启明星辰、深信服、曙光、烽火全部为 0**）。详见 §7.1.1。真正该抄的是 `InterfaceTypeChoices`/`CableTypeChoices` 枚举命名 + Nautobot `CableType.mapping` 的极性建模 + NetBox 的 SVG/JSON 双通道 API 形态 |

### 1.3 三条路线的一句话评价

| 路线 | 一句话评价 | 建议 |
|---|---|---|
| **完全复用**（装 NetBox 直接用） | 布线表要靠 Export Template 手搓 Jinja2，线长要人工填 5133 行，跨机柜派生列无处安放 → **等于把 Excel 的痛点搬进 Web** | ❌ 否决 |
| **插件化**（NetBox + 自研插件） | 要在别人的 Django 里实现"机柜坐标系 + 走线路径 + 档位表 + 五版式导入"，且官方明说插件 API 不保证稳定；升级 NetBox 大版本 = 重测全部 | ⚠️ 仅当"必须与现网 NetBox 共存"时才考虑 |
| **自研核心 + 借鉴互操作** | 核心算法与 Excel 交付物 100% 自主可控；设备型号库白拿 CC0 资产；对外提供 NetBox 导出，未来可双向同步 | ✅ **推荐** |

---

## 2. 调研方法与口径

### 2.1 数据核实口径

- **Star / Fork / 最近推送 / 许可证 / 主语言 / 是否归档**：`GET /repos/{owner}/{repo}` 的 `stargazers_count` / `forks_count` / `pushed_at` / `license.spdx_id` / `language` / `archived` 字段。
- **许可证"准确版本"判定规则**：
  1. GitHub API 的 `license.spdx_id` 只作初判；
  2. 对**关键候选**逐个下载 `LICENSE` / `license.txt` / `COPYING` 原文，读第一行确认版本号（例：`GNU GENERAL PUBLIC LICENSE Version 2, June 1991`）；
  3. 仓库无 LICENSE 文件但 README 声明的，按"README 声明 + 无标准文件"如实记录（这对法务是有意义的差异）；
  4. `spdx_id = NOASSERTION` 的，下载原文人工判定。
- **数据模型结论**：直接读源码/建表 SQL，文中给 `文件:行号`。
- **"未公开/待核实"**：任何无法用上述方法确认的，本文一律标注，不做推测性填写。

### 2.2 明确不在本文范围

- 商用 DCIM（Sunbird / Nlyte / Device42 / 华为 / 维谛等）→ 见 [04-commercial-products.md](./04-commercial-products.md)
- 前端 2D/3D 技术栈选型 → 见 [02-visualization-stack.md](./02-visualization-stack.md)
- 布线算法与线长公式推导 → 见 [03-cabling-algorithms.md](./03-cabling-algorithms.md)

### 2.3 一个必须先纠正的常见误解

调研过程中发现一个容易踩的坑，先写在这里避免后续讨论跑偏：

> **"NetBox 的 Cable 模型不存线长" —— 这句话是错的。**
> NetBox 的 `Cable` 模型**有** `length` 和 `length_unit` 字段（`netbox/dcim/models/cables.py:95`，`length = models.DecimalField(max_digits=8, decimal_places=2, blank=True, null=True)`）。
>
> 准确的表述是：**NetBox 能"存"线长，但不能"算"线长，也不承载算线长所需的几何信息。** 具体缺三样：
> 1. **机柜没有平面坐标**（`Rack` 模型无 `position_x/y`，已 grep 全文件确认）；
> 2. **没有走线路径模型**（Cable 是 A↔B 两点直连，中间经过什么桥架不记录）；
> 3. **没有档位表/派生列**（跨机柜、间隔机柜、机柜数量、机内、柜间、取整档位）。
>
> 这个区别很重要：它决定了"NetBox 能不能当底座"的答案是 **能当数据仓库，不能当计算引擎**。

---

## 3. 候选清单概览表

> 全部数据 2026-10-20 经 GitHub API 核实。★ = Star 数；"推送"= `pushed_at`（最近一次任意分支提交时间，不等于发布版本时间）。

### 3.1 核心 DCIM / IPAM 平台

| 项目 | ★ | Fork | 许可证（已核原文） | 最近推送 | 语言 | 定位 | 布线/线长 | 3D |
|---|---|---|---|---|---|---|---|---|
| [netbox-community/netbox](https://github.com/netbox-community/netbox) | 21734 | 3157 | Apache-2.0 | 2026-10-20 | Python | DCIM + IPAM 事实标准 | 存 length，不算 | ❌ |
| [nautobot/nautobot](https://github.com/nautobot/nautobot) | 1628 | 423 | Apache-2.0 | 2026-10-20 | Python | Network SoT + 自动化平台 | 存 length，不算 | ❌ |
| [openDCIM/openDCIM](https://github.com/openDCIM/openDCIM) | 366 | 223 | **README 声明 GPLv3，但仓库无 LICENSE 文件** | 2026-08-14 | PHP | 纯 DCIM（机房物理层） | ❌ 无线长概念 | ❌ |
| [RackTables/racktables](https://github.com/RackTables/racktables) | 812 | 266 | GPL-2.0（原文 `Version 2, June 1991`） | 2026-06-26 | PHP | 机柜/端口/IP 一体化 | ❌ | ❌ |
| [phpipam/phpipam](https://github.com/phpipam/phpipam) | 2816 | 802 | **README 声明 GPLv3（`misc/gpl-3.0.txt` 已核），仓库无 LICENSE 文件** | 2026-10-05 | PHP | IPAM 为主 | ❌ 无端口表 | ❌ |
| [Combodo/iTop](https://github.com/Combodo/iTop) | 1204 | 303 | AGPL-3.0（`license.txt` 已核） | 2026-10-20 | PHP | CMDB + ITSM | ❌ | ❌ |
| [glpi-project/glpi](https://github.com/glpi-project/glpi) | 6475 | 1836 | GPL-3.0（`LICENSE` 已核） | 2026-10-20 | PHP | ITAM + ITSM（含 DCIM 模块） | ❌ | ❌ |
| [OCSInventory-NG/OCSInventory-Server](https://github.com/OCSInventory-NG/OCSInventory-Server) | 405 | 150 | GPL-2.0 | 2026-08-25 | Perl | 客户端资产清点 | ❌ | ❌ |
| [opsmill/infrahub](https://github.com/opsmill/infrahub) | 538 | 66 | Apache-2.0（`LICENSE.txt` 已核） | 2026-10-20 | Python | 图数据库式 SoT | ❌ | 生态有 net3d |
| [allegro/ralph](https://github.com/allegro/ralph) | 2529 | 592 | Apache-2.0 | 2026-10-19 | Python | DCIM + 资产管理（Allegro 内部） | ❌ | ❌ |
| [netdisco/netdisco](https://github.com/netdisco/netdisco) | 930 | 108 | BSD-3-Clause | 2026-10-20 | Perl | 网络设备发现/拓扑 | ❌ | ❌ |
| [Kobii-git/rackpad](https://github.com/Kobii-git/rackpad) | 417 | 21 | MIT | 2026-10-20 | TypeScript | homelab 级机柜台账 | ❌ | ❌ |

### 3.2 NetBox 生态（官方 + 社区插件）

| 项目 | ★ | 许可证 | 最近推送 | 作用 | 对本项目价值 |
|---|---|---|---|---|---|
| [devicetype-library](https://github.com/netbox-community/devicetype-library) | 1631 | **CC0-1.0** | 2026-10-19 | 12390 设备型号 + 2036 模块 + 147 机柜型号 YAML，覆盖 371 厂商 | ⭐ 华为 103 个可用；**国内信创设备覆盖仅 0.97%**（见 §7.1.1） |
| [netbox-docker](https://github.com/netbox-community/netbox-docker) | 2793 | Apache-2.0 | 2026-10-14 | 官方 Docker 部署 | ⭐⭐ 评估/对照用 |
| [pynetbox](https://github.com/netbox-community/pynetbox) | 687 | Apache-2.0 | 2026-10-09 | Python REST 客户端 | ⭐⭐⭐ 导出对接用 |
| [go-netbox](https://github.com/netbox-community/go-netbox) | 225 | **NOASSERTION（无 LICENSE 文件，待核实）** | 2025-05-09 | Go REST 客户端 | ⭐ |
| [ansible_modules](https://github.com/netbox-community/ansible_modules) | 402 | GPL-3.0 | 2026-10-13 | Ansible 集合 | ⭐ |
| [netbox-topology-views](https://github.com/netbox-community/netbox-topology-views) | 1103 | Apache-2.0 | 2026-10-19 | 拓扑图（L2/L3） | ⭐⭐ 拓扑可视化参考 |
| [netbox-reorder-rack](https://github.com/netbox-community/netbox-reorder-rack) | 112 | Apache-2.0 | 2026-09-10 | 拖拽重排机柜内设备 | ⭐⭐⭐ U 位拖拽交互参考 |
| [netbox-qrcode](https://github.com/netbox-community/netbox-qrcode) | 271 | Apache-2.0 | 2026-09-03 | 设备/机柜二维码标签 | ⭐⭐ 现场施工标签 |
| [netbox-napalm-plugin](https://github.com/netbox-community/netbox-napalm-plugin) | 74 | Apache-2.0 | 2026-09-30 | NAPALM 对接 | ⭐ 本期不需要 |
| [nextbox-ui-plugin](https://github.com/iDebugAll/nextbox-ui-plugin) | 642 | MIT | **2025-05-24（约 17 个月未更新）** | 3D/2D 机房视图（topoSphere SDK） | ⚠️ 可看交互，不可依赖 |
| [netbox-branching](https://github.com/netboxlabs/netbox-branching) | 157 | **NetBox Limited Use License 1.0（非 OSI）** | 2026-10-19 | 数据分支/变更隔离 | ⚠️ 许可受限，只可参考思路 |
| [diode](https://github.com/netboxlabs/diode) | 167 | **NetBox Limited Use License 1.0（非 OSI）** | 2026-09-25 | 高速数据摄入管道（Go） | ⚠️ 许可受限 |

### 3.3 专用插件（"有没有人做过我们要做的事"的直接答案）

| 项目 | ★ | 许可证 | 最近推送 | 能力 | 结论 |
|---|---|---|---|---|---|
| [gadler01/Netbox_Cable_Calculator_Plugin](https://github.com/gadler01/Netbox_Cable_Calculator_Plugin) | **2** | MIT | 2026-10-05 | 跨机柜线长计算 + 平面图拖拽 + BOM | 🔴 **唯一同类，★2 的个人项目，不足以依赖** |
| [jsenecal/netbox-pathways](https://github.com/jsenecal/netbox-pathways) | 21 | AGPL-3.0 | 2026-10-15 | 管道路由 + PostGIS 几何 + `geo_length` | 🟡 室外管线级，思路可借鉴 |
| [jsenecal/netbox-fms](https://github.com/jsenecal/netbox-fms) | 25 | AGPL-3.0 | 2026-10-19 | 光纤管理系统（FMS） | 🟡 光纤链路管理参考 |
| [jsenecal/netbox-cable-labels](https://github.com/jsenecal/netbox-cable-labels) | 48 | Apache-2.0（原文已核，API 报 NOASSERTION） | 2026-01-13 | 线缆标签批量生成 | ⭐⭐ 施工标签可参考 |
| [hervehildenbrand/net3d](https://github.com/hervehildenbrand/net3d) | 25 | MIT | 2026-09-29 | NetBox/Infrahub → 3D（地球→楼→机柜） | 🟡 3D 分层交互参考 |
| [tbotnz/netbox_floorplan](https://github.com/tbotnz/netbox_floorplan) | 80 | LGPL-3.0 | 2024-03-06 | 机房平面图 | 🔴 **已归档（archived=true）** |
| [drygdryg/netbox-plugin-device-map](https://github.com/drygdryg/netbox-plugin-device-map) | 46 | MIT | 2024-08-06 | 设备地图 | 🔴 约 2 年未更新 |
| [ravenrs/netbox-rack-design](https://github.com/ravenrs/netbox-rack-design) | 14 | Apache-2.0 | 2026-10-20 | 机柜规划 | 🟡 太小 |
| [teemIP/teemip-cable-mgmt](https://github.com/teemIP/teemip-cable-mgmt) | 7 | GPL-3.0 | 2026-06-25 | TeemIP 设备互联文档化 | 🔴 太小 |

### 3.4 Nautobot App 生态（对比用，说明"为什么不选 Nautobot"）

| 项目 | ★ | 许可证（原文已核） | 最近推送 |
|---|---|---|---|
| [nautobot-app-device-onboarding](https://github.com/nautobot/nautobot-app-device-onboarding) | 59 | Apache-2.0 | 2026-10-14 |
| [nautobot-app-nornir](https://github.com/nautobot/nautobot-app-nornir) | 28 | Apache-2.0 | 2026-10-20 |
| [nautobot-app-design-builder](https://github.com/nautobot/nautobot-app-design-builder) | 15 | Apache-2.0 | 2026-10-14 |
| [nautobot-app-floor-plan](https://github.com/nautobot/nautobot-app-floor-plan) | 15 | Apache-2.0 | 2026-10-14 |

> 注意对比：NetBox 的对应能力由**社区**做（topology-views ★1103、reorder-rack ★112），Nautobot 的对应能力由**厂商自己**做但只有 ★15。这不是说 Nautobot 差，而是说：**生态规模**和**厂商支持**是两个不同的东西，本项目的诉求（找个能白拿资产的生态）偏向后者之外的前者。

### 3.5 已核实不存在的仓库（避免引用二手资料时踩坑）

以下仓库名常见于各类文章但 **GitHub API 返回 404**，已确认不存在或名字错误：

| 错误名 | 实际情况 |
|---|---|
| `NAPALM/napalm` | 正确是 [napalm-automation/napalm](https://github.com/napalm-automation/napalm) ★2515 Apache-2.0 |
| `nipap/nipap` | 正确是 [SpriteLink/NIPAP](https://github.com/SpriteLink/NIPAP) ★593 MIT |
| `Combodo/teemip` | 正确是 [teemIP/teemip-core-ip-mgmt](https://github.com/teemIP/teemip-core-ip-mgmt) ★32 AGPL-3.0 |
| `gestionip/gestioip` | 未找到官方 GitHub 仓库（原 SourceForge 项目），**待核实** |
| `netbox-patchpanel-*` 系列 | 搜索无活跃结果 |
| `netbox-device-map` 官方系列 | 不存在官方版；仅 `drygdryg/netbox-plugin-device-map`（★46，2024-08 后未更新） |

另：搜索 `netbox-cable-*` 得到 8 个仓库，除 `netbox-cable-labels`（★48）外**全部 ★≤3**，其中 `a21371/netbox-fast-cable` 的 README 自述 "this is ai generated"。

---

## 4. 逐项目分析

> 每个项目按统一维度：许可证 / 活跃度 / 数据模型 / 2D 3D / 布线能力 / 导入导出 / API / 扩展点 / 二次开发难度。
> 标注**【源码级】**的结论来自直接读该版本源码；标注**【文档级】**的仅来自官方文档，未逐行读源码。

### 4.1 NetBox（★21734，Apache-2.0）— 事实标准，但不是布线引擎

**基本信息**
- 仓库：https://github.com/netbox-community/netbox
- 许可证：**Apache License 2.0**（`license.spdx_id = Apache-2.0`，仓库根有 `LICENSE`）
- 最新 release：**v4.7.3**（2026-10-13），`netbox/release.yaml` 内容逐字为 `version: "4.7.3"` / `edition: "Community"` / `published: "2026-10-13"`
- 最近推送：2026-10-20；主分支 `main`；语言 Python
- 近 90 天提交约 418 次 —— **本项目候选中最活跃的 Python DCIM**

**数据模型（源码级核实）**

DCIM 侧 `docs/models/dcim/` 官方模型清单：
`cable`, `cablebundle`, `consoleport`(+template), `coolingfeed`/`coolingintake`/`coolingoutflow`/`coolingsource`, `device`, `devicebay`(+template), `devicerole`, `devicetype`, `frontport`(+template), `interface`(+template), `inventoryitem`(+role/template), `location`, `macaddress`, `manufacturer`, `module`/`modulebay`/`moduletype`/`moduletypeprofile`, `platform`, `powerfeed`/`poweroutlet`(+template)/`powerpanel`/`powerport`(+template)

IPAM 侧 `docs/models/ipam/`：
`aggregate`, `asn`, `asnrange`, `fhrpgroup`(+assignment), `ipaddress`, `iprange`, `prefix`, `rir`, `role`, `routetarget`, `service`(+template), `vlan`, `vlangroup`, `vlantranslationpolicy`/`vlantranslationrule`, `vrf`

关键类位置：

| 模型 | 文件:行 | 关键字段 |
|---|---|---|
| `Manufacturer` | `netbox/dcim/models/devices.py:56` | — |
| `DeviceType` | `devices.py:66` | u_height / is_full_depth / subdevice_role |
| `DeviceRole` | `devices.py:435` | — |
| `Platform` | `devices.py:484` | — |
| `Device` | `devices.py:531` | site / **rack(FK)** / **position = DecimalField(max_digits=4, decimal_places=1)** / face / vc_position / airflow / cooling_method / primary_ip4 / primary_ip6 |
| `VirtualChassis` | `devices.py:1243` | — |
| `VirtualDeviceContext` | `devices.py:1321` | — |
| `MACAddress` | `devices.py:1431` | — |
| `PowerPanel` | `netbox/dcim/models/power.py:24` | — |
| `PowerFeed` | `power.py:73` | `(PrimaryModel, PathEndpoint, CabledObjectModel)` |
| `Region` / `SiteGroup` / `Site` / `Location` | `netbox/dcim/models/sites.py:28 / :93 / :158 / :289` | Site 有 **latitude / longitude** |
| `RackBase` / `RackType` / `Rack` / `RackReservation` | `netbox/dcim/models/racks.py:58 / :167 / :286 / :722` | width / u_height / starting_unit / desc_units / outer_width / outer_depth / max_weight |
| `CableBundle` | `netbox/dcim/models/cables.py:51` | — |
| `Cable` | `cables.py:95` | 见下 |

**`Cable` 模型逐字段（`cables.py:95` 起）**

```
type            CharField(choices=CableTypeChoices)
status          (LinkStatusChoices: connected/planned/decommissioning)
profile         CharField(choices=CableProfileChoices)
tenant          FK
label           CharField(max_length=100)
color           ColorField
length          DecimalField(max_digits=8, decimal_places=2, blank=True, null=True)   ← 有线长！
length_unit     CharField(choices=CableLengthUnitChoices)
_abs_length     DecimalField(max_digits=14, decimal_places=4)   ← 仅用于排序（归一化到米）
bundle          FK dcim.CableBundle
```
另 `cables.py:302 def clean()` 有校验：`cables.py:307` `raise ValidationError(_("Must specify a unit when setting a cable length"))`。

**`Cable.length` 能不能自动算？答案是不能，但有个坑要澄清**

- NetBox 确实提供 `CablePath.get_total_length()`（`cables.py:1301`），但它算的是**一条 cable path 上各段 Cable 已录入 length 的累加**，**不是根据几何位置推算**。实现细节（`cables.py:1329-1349`）：遍历 `self.path` 的每个 `node_set`，**同一 node_set 内取各电缆 `_abs_length` 的最大值**作为该跳长度（`:1334-1338`），逐跳累加（`:1345`），并同时累加沿途 `CircuitTermination` 的 `_abs_distance`（`:1351-1360`）；返回 `(total_length, is_definitive)`，只要有任一电缆未填 length 则 `is_definitive = False`（`:1349`）。**前提是每根 Cable 的 `length` 已经被人手工填过。**
- 所以：**NetBox 提供"线长汇总"，不提供"线长计算"。**

**`CableLengthUnitChoices`（`netbox/dcim/choices.py`）**：`km` / `m` / `cm` / `mi` / `ft` / `in` 六种单位。**没有档位表概念**（不存在 5/7/10/15/20/25/30/35/40 这种采购档位）。

**`CableProfileChoices`（`choices.py:1830`）**：用于描述多芯/breakout 的"连接器×位置"映射，实测枚举：
`single-1c1p`, `single-1c2p`, `single-1c4p`, `single-1c6p`, `single-1c8p`, `single-1c12p`, `single-1c16p`,
`trunk-2c1p`, `trunk-2c2p`, `trunk-2c4p`, `trunk-2c4p-shuffle`, `trunk-2c6p`, `trunk-2c8p`, `trunk-2c12p`,
`trunk-4c1p`, `trunk-4c2p`, `trunk-4c4p`, `trunk-4c4p-shuffle`, `trunk-4c6p`, `trunk-4c8p`, …（c=connector 数，p=position 数）
→ **这个设计对"MPO 主干拆 4×25G"场景是有用的**，值得借鉴（见 §7.2）。

**`CableTypeChoices`（`choices.py:1906`）**：cat3/cat5e/cat6/cat6a/cat7/cat8、mrj21-trunk、dac-active/dac-passive、coaxial/RG 系列、**mmf / mmf-om1…om5**、**smf / smf-os1 / smf-os2**、**aoc**、power、usb。
→ 覆盖度足够本项目用（用户 Excel 的线缆类型是 `MPO` / `LC-LC` / `RJ45` 三类，可映射到 `mmf-om3/om4`、`smf`、`cat6a`）。

**2D/3D 能力**
- **机柜正视图（U 位图）：有，且实现得不错。** REST API 已暴露：`netbox/dcim/api/views.py:206 def elevation(self, request, pk=None)`，`@action(detail=True)`，`operation_id='dcim_racks_elevation_retrieve'`；**支持 `render=svg`** 直接返回 SVG（走 `Rack.get_elevation_svg()`），否则返回 JSON 的 RackUnit 列表。相关模型方法：`racks.py:510 get_rack_units()`、`:573 get_available_units()`、`:613 get_reserved_units()`、`:623 get_elevation_svg()`（参数 face / unit_width / unit_height / legend_width / include_images / base_url / highlight）。
  → **这是"U 位可视化"最值得借鉴的现成设计：服务端渲染 SVG + 同时提供 JSON 结构。**
- **机房平面图：没有。** `Rack` 模型**无任何坐标字段**（已 grep 全 `racks.py` 确认无 `position_x`/`position_y`/`coordinate`/`latitude`）。第三方插件 `net3d` 的 README 直接印证这一点：*"NetBox stores no rack coordinates, so the floor plan is schematic"*。
- **3D：本体没有。** 生态里 `nextbox-ui-plugin`（★642，MIT，但 **2025-05-24 后未更新**，约 17 个月）和 `net3d`（★25）做了 3D，都属于"外部读取 NetBox 数据再渲染"的模式，不是 NetBox 内部能力。

**导入导出**
- **CSV 批量导入**：各 app 都有 `bulk_import.py` 表单，覆盖 circuits / core / dcim / extras / ipam / tenancy / users / virtualization / vpn / wireless。
  - Cable 的 CSV 导入列（`netbox/dcim/forms/bulk_import.py:1866` `Meta.fields` 逐字）：
    `side_a_site`, `side_a_device`, `side_a_power_panel`, `side_a_type`, `side_a_name`, `side_b_site`, `side_b_device`, `side_b_power_panel`, `side_b_type`, `side_b_name`, `type`, `status`, `profile`, `tenant`, `bundle`, `label`, `color`, `length`, `length_unit`, `description`, `owner`, `comments`, `tags`
  - 注意：**`length` 在导入列里**，说明"外部算好长度再灌进 NetBox"这条路是通的。
- **导出**：Export Templates（Jinja2 模板，输出任意文本格式）。`docs/customization/export-templates.md`。→ 做"布线表导出"技术上可行，但需要自己写 Jinja2 模板，且**模板只能拿到 NetBox 已有字段**，派生列（跨机柜/间隔机柜/柜间线缆/档位取整）算不出来（Jinja2 里做算术不现实）——这是"NetBox 当底座"最致命的一刀。
- **YAML 设备型号库**：`devicetype-library` 提供 12390 个 `device-types/*.yaml`，可批量导入。

**API 完备度：优秀**
- REST：Django REST Framework，token 认证，OpenAPI 文档，`pynetbox`（★687 Apache-2.0）/`go-netbox`（★225，**无 LICENSE 文件，NOASSERTION，待核实**）客户端
- GraphQL：**明确声明是 "special-purpose read-only API"**（`docs/features/api-integration.md` 原文），只读，不能写
- Webhooks + Event Rules、`/metrics`（django-prometheus）

**扩展点（`netbox/netbox/plugins/registration.py`，262 行，源码级）**
`__all__`（`:16-26`）公开注册函数共 **9 个**：`register_graphql_filter_extensions`, `register_graphql_schema`, `register_graphql_type_extensions`, `register_jinja_filters`, `register_menu`, `register_menu_items`, `register_serializer_resolver`, `register_template_extensions`, `register_user_preferences`。
插件本质是 **Django app**，能做：加模型、加页面、加 API、注入模板片段、加载额外 Django app、加中间件。
**注意：这 9 个注册点里没有一个是"模型保存前后钩子"**，也没有任何"替换已有模型字段/行为"的机制——这一点是判断"插件能不能补上派生列"的关键（见 §5.2）。

**但官方对插件 API 的边界有明确警告（`docs/plugins/development/index.md` 逐字）**：
> *"While very powerful, the NetBox plugins API is necessarily limited in its scope. The plugins API is discussed here in its entirety: Any part of the NetBox code base not documented here is **not** part of the supported plugins API, and should not be employed by a plugin. Internal elements of NetBox are subject to change at any time and without warning."*

**二次开发难度**：中等偏低（Python + Django，生态成熟，文档好），但**"改核心"的成本极高**——要给 Rack 加坐标、要给 Cable 加派生列，就得 fork。

---

### 4.2 Nautobot（★1628，Apache-2.0）— 架构更现代，但生态小一个数量级

**基本信息**
- 仓库：https://github.com/nautobot/nautobot
- 许可证：**Apache-2.0**（根 `LICENSE.txt`）
- 最新 release：**v3.2.7**（2026-10-13）；`pyproject.toml` 里 `version = "3.2.8a0"`（开发中版本）
- 最近推送：2026-10-20；默认分支 `develop`；近 90 天提交约 205 次
- 商业背景：由 **Network to Code** 主导，有 Professional / Enterprise / Cloud 三个商业版

**一个必须记录的发现：Nautobot v3 的 Cable 模型比 NetBox 先进**

【源码级】`nautobot/dcim/models/cables.py`：

1. **新增 `CableType` 模型**（`cables.py:132`），把"线缆类型"从枚举提升为一等实体，字段包括：
   - `a_connectors` / `b_connectors`（两侧物理连接器数，`cables.py:160` / `:165`）
   - `total_lanes`（逻辑通道总数，`cables.py:170`）
   - `mapping` — **JSONField**，A→B 的 lane 映射，注释逐字：*"A→B lane mapping as a JSON array of objects with keys: label, a_connector, a_position, b_connector, b_position. If empty, will be auto-populated."*（`cables.py:175`）
   - `is_shuffle`（极性打乱，信息性）
   - `strands_per_lane`（每 lane 纤芯数：铜 1、双工光纤 2）
   - `polarity_method`（`CableTypePolarityMethodChoices`：`straight-through` / `reversed` / `pair-reversed` / `other`，`nautobot/dcim/choices.py:1452`）
   - `has_embedded_transceivers`（是否内置光模块，如 AOC/DAC）
   - `get_diagram_svg()` 方法（把 lane 映射画成图）
2. **`CableLane` / `CableLaneSide` dataclass**（`cables.py:77` / `:90`）—— 把一根线缆拆成"逻辑通道"，每条通道解析出两侧 termination。
3. **`Cable.length` 是 `PositiveSmallIntegerField`**（`cables.py:495`）—— 注意：**NetBox 是 `DecimalField(8,2)`，Nautobot 是整数**。如果要做亚米精度（用户 Excel 里有 `3.6`、`10.08`、`15.08` 这种小数），**Nautobot 的整数长度不够用**。
4. `CableLengthUnitChoices`（`choices.py:1426`）与 NetBox 一致：km/m/cm/mi/ft/in。

**2D/3D**
- 机柜正视图：有，`nautobot/dcim/svg/rack_elevation.py:11 class RackElevationSVG`，方法 `render(face, unit_width, unit_height, legend_width)`（`:213`）、`merge_elevations(face)`（`:195`）。与 NetBox 同源设计。
- 机房平面图：靠 App `nautobot-app-floor-plan`（**★15**，Apache-2.0，2026-10-14）。
- 3D：本体无。

**扩展点**：App framework，官方文档 `nautobot/docs/development/apps/index.md` 明确 *"Apps can introduce their own models and views, but cannot interfere with existing components"*。另有 **Jobs** 引擎（`nautobot/extras/jobs.py:123 class BaseJob`，注释逐字 *"Jobs functionality - consolidates and replaces legacy 'custom scripts' and 'reports' features"*）、原生 Git 数据源、Config Contexts。

**NetBox vs Nautobot 取舍（回答 lead 的问题 A）**

| 维度 | NetBox | Nautobot | 本项目偏向 |
|---|---|---|---|
| 许可证 | Apache-2.0 | Apache-2.0 | 平手 |
| ★ / Fork | **21734 / 3157** | 1628 / 423 | **NetBox**（13 倍） |
| 近 90 天提交 | ~418 | ~205 | **NetBox** |
| 数据模型现代性 | Cable 用枚举 | **Cable 用 CableType + lane mapping + 极性** | **Nautobot** |
| 线长精度 | `DecimalField(8,2)` | `PositiveSmallInteger`（**不够用**） | **NetBox** |
| 生态规模 | topology-views ★1103 / reorder-rack ★112 | floor-plan ★15 / design-builder ★15 | **NetBox** |
| 自动化引擎 | 无（靠 Ansible/NAPALM 外部） | **有 Jobs + Nornir App** | Nautobot（但本项目不需要） |
| 上手成本 | 低（文档/社区/中文资料多） | 中 | **NetBox** |

**结论**：两个都不是底座，但**参考数据模型时，Nautobot 的 `CableType` + `CableLane` 设计比 NetBox 的 `CableProfileChoices` 更值得抄**（尤其是 MPO 主干拆分和极性 A/B/C）。如果要选一个当"导出目标/对照系统"，选 NetBox（生态大、社区资料多、Decimal 精度够）。

---

### 4.3 openDCIM（★366，README 声明 GPLv3 但无 LICENSE 文件）— 物理层做得最细，但架构老旧

**基本信息**
- 仓库：https://github.com/openDCIM/openDCIM
- 许可证：**README 逐字声明 GPLv3**（*"you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, version 3"*），但**仓库根目录没有 `LICENSE` 文件**，GitHub API `license.spdx_id = NONE`。→ **法务上建议按 "GPLv3 per README" 理解，但需向维护者确认**。
- 最近推送：2026-08-14；语言 PHP
- **最新 GitHub release 仍是 `23.04`（2023-10-15）**，但 master 分支 `version.php` 写 `define("VERSION","25.01")` → **有开发但三年未打 release**，近 90 天仅约 4 次提交。累计 contributor 约 66。
- 仓库 README 自己写：*"THE CURRENT RELEASE IN GITHUB IS FOR DEVELOPMENT ONLY"*

**数据模型（源码级，`create.sql` 1171 行）**

| 表 | 关键字段 | 对本项目的价值 |
|---|---|---|
| `fac_Ports` | `DeviceID, PortNumber, Label, ConnectorID, ProtocolID, RateID, MediaID, ColorID, ConnectedDeviceID, ConnectedPort, Notes`，带 `UNIQUE KEY ConnectedDevice (ConnectedDeviceID,ConnectedPort)` | ⭐ **用唯一索引强制"端口一对一占用"**，是最轻量的实现 |
| `fac_TemplatePorts` | 模板级端口定义 | ⭐ 设备型号 → 端口模板，可借鉴 |
| `fac_Cabinet` | `CabinetID, DataCenterID, Location, ZoneID, CabRowID, CabinetHeight, Model, MaxKW, MaxWeight, **MapX1, MapX2, MapY1, MapY2**, FrontEdge, U1Position` | ⭐⭐ **机房平面图用矩形坐标（MapX1/X2/Y1/Y2）** —— 这是 4 个开源 DCIM 里唯一原生支持机柜平面摆放的 |
| `fac_Device` | `DeviceID, Label, SerialNo, AssetTag, PrimaryIP, …, Cabinet, Position, Height, Ports, FirstPortNum, TemplateID, NominalWatts, PowerSupplyCount, DeviceType, ChassisSlots, RearChassisSlots, ParentDevice` | U 位用 `Position` + `Height` 整数（不是 Decimal） |
| `fac_Slots` | `TemplateID, Position, BackSide, X, Y, W, H` | 机箱插槽的 2D 坐标 |
| `fac_PanelSchedule`（:502）/ `fac_PowerPanel`（:646） | 配电盘/配电柜 | 配电管理 |
| `fac_SensorReadings` / `fac_SensorTemplate` | `Temperature, Humidity, LastRead` + 温湿度 OID | 环境监控 |
| `fac_CabRow` / `fac_CabinetTags` | 机柜排、标签 | — |

**布线能力：有 Excel 布线表导出，但无线长**
- ⭐ **`connections_spreadsheet.php`：专门的 Excel 布线表导出**，用 PhpSpreadsheet，每个 device 一个 worksheet，列头 A1..G1 逐字为
  `SourceDevice | SourcePort | TargetDevice | TargetPort | Notes | MediaType | Color`
  worksheet 里还有 Device Label / Manufacturer / Model / Serial Number / Asset Tag / Target Cabinet / Position。
  → **这是最接近用户交付物的现成实现**：思路是"一个设备一个 sheet，列对端"。但它**没有长度列**。
- ❌ **全库搜索 `CableLength` 命中 0** —— 确认无任何线长概念。

**其他能力**
- API：`api/v1/` 下 `getRoutes.php` / `postRoutes.php` / `putRoutes.php` / `deleteRoutes.php` + `index.php`，用 **Slim Framework v3**（CHANGELOG 23.03 记录从 Slim v2 升级）。→ REST 有，但很薄。
- 批量导入：`bulk_importer.php`（**Excel 上传**）、`bulk_cabinet.php`、`bulk_container.php`、`bulk_departments.php`、`bulk_moves.php`、`bulk_network.php`、`bulk_power.php`、`bulk_templates.php`、`bulk_users.php`
- 运维工具：`cabaudit.php`（机柜审计）、`conflicts.php`（冲突检查）、`dc_stats.php` / `dc_dashboard.php`（平面图 + 统计）
- 3D：无
- 技术栈：PHP + MySQL，无 Composer 现代依赖管理（vendor 直接committed）

**评价**：openDCIM 是**最"懂机房物理层"的那个**（有平面坐标、有配电盘、有温湿度、有端口模板、有 Excel 布线表导出），但代码古老（PHP 过程式）、三年未发 release、无 3D、无线长。**它的 `fac_Cabinet` 坐标方案和 `fac_Ports` 唯一索引方案值得直接抄。**

---

### 4.4 RackTables（★812，GPL-2.0）— 三面机柜建模最严谨，但已接近停滞

**基本信息**
- 仓库：https://github.com/RackTables/racktables
- 许可证：**GPL-2.0**（`LICENSE` 原文首行 `GNU GENERAL PUBLIC LICENSE Version 2, June 1991`）
- 最近推送：2026-06-26；**近 90 天 0 提交，近 1 年约 5 次提交** → **接近停滞**
- **无 GitHub release**；最新 tag 为 `RackTables-0.22.0`
- 语言 PHP；要求 PHP ≥ 7.0（推荐 7.3）、MySQL/MariaDB ≥ 5，**DB 用户需要 TRIGGER 权限**（用了数据库触发器做一致性）

**数据模型（源码级，`wwwroot/inc/install.php`）**

| 表 | 定义 | 亮点 |
|---|---|---|
| `Link` | `(porta int, portb int, cable char(64))`，`PRIMARY KEY (porta,portb)` + **`UNIQUE KEY porta`** + **`UNIQUE KEY portb`** | ⭐⭐ **用两个 UNIQUE KEY 强制端口唯一占用**，比应用层校验更可靠。`cable` 字段只是个字符串标签 |
| `Port` | `(id, object_id, name, iif_id, type, l2address, reservation_comment, label)`，`UNIQUE KEY object_iif_oif_name (object_id, iif_id, type, name)` | 端口名在设备内唯一 |
| `RackSpace` | `(rack_id, unit_no, atom enum('front','interior','rear'), state enum('A','U','T'), object_id)`，`PRIMARY KEY (rack_id, unit_no, atom)` | ⭐⭐⭐ **三面建模（front/interior/rear）× 每 U 状态（A=Available/U=Used/T=Temporarily reserved）** —— 这是所有候选里**最严谨的机柜空间模型** |
| `Molecule` / `MountOperation` | 机柜变更的"分子"抽象 + 操作流水 | ⭐ **原子化机柜变更 + 可审计**，对应本项目的"变更管理"需求 |
| `PortCompat` / `PortOuterInterface` / `PortInnerInterface` / `PortInterfaceCompat` | 端口类型兼容矩阵 | ⭐⭐ **连接器↔端口类型兼容性用表驱动**，可做"LC 不能插 RJ45"类校验 |
| `PortAllowedVLAN` / `PortNativeVLAN` / `PortVLANMode` | VLAN 配置 | — |
| `ObjectHistory` / `ObjectLog` | 变更历史 | ⭐ 内置审计 |
| `RackThumbnail` | 机柜缩略图 | — |

**布线能力**
- ❌ 无 cable length（`Link.cable` 只是 `char(64)` 标签，不存长度）
- ❌ **无 REST API**（无 OpenAPI/Swagger；只有 `wwwroot/inc/ajax-interface.php` + `remote.php` 的私有协议）
- 插件目录 `plugins/` 只有 `cacti`、`munin` 两个监控集成
- 3D：无

**评价**：RackTables 的**数据模型严谨度是全场第一**（三面 RackSpace、端口唯一约束、端口兼容矩阵、原子化变更 + 审计），但**项目已停滞、无 API、PHP 老技术栈**。→ **只抄模型，不用代码。**

---

### 4.5 phpIPAM（★2816，README 声明 GPLv3 但无 LICENSE 文件）— IPAM 强，DCIM 弱

**基本信息**
- 仓库：https://github.com/phpipam/phpipam
- 许可证：**README 声明 GPL v3，仓库根无 `LICENSE` 文件**（GitHub `spdx_id = NONE`），但仓库内 `misc/gpl-3.0.txt` 已核实为 GPLv3 全文 → 声明与文件一致
- 最近推送：2026-10-05；最新 release **v1.8.3**（2026-09-09）；近 90 天约 22 次提交
- 语言 PHP

**数据模型（源码级，`db/SCHEMA.sql`，1106 行 / 48 张表）**

全部表名（按出现顺序）：
`instructions`, `customers`, `ipaddresses`, `logs`, `requests`, `sections`, `settings`, `settingsMail`, `subnets`, `devices`, `userGroups`, `users`, `lang`, `vlans`, `vlanDomains`, `vrf`, `nameservers`, `api`, `apiLock`, `changelog`, `widgets`, `deviceTypes`, `loginAttempts`, `usersAuthMethod`, `ipTags`, `firewallZones`, `firewallZoneMapping`, `firewallZoneSubnet`, `scanAgents`, `nat`, `racks`, `rackContents`, `locations`, `pstnPrefixes`, `pstnNumbers`, `circuitProviders`, `circuits`, `circuitsLogical`, `circuitsLogicalMapping`, `circuitTypes`, `php_sessions`, `routing_bgp`, `routing_subnets`, `vaults`, `vaultItems`, `passkeys`, `nominatim`, `nominatim_cache`

关键点：
- `devices`（`:320`）有 `rack / rack_start / rack_size / rack_deep` + 一堆 `snmp_*` 字段 → 设备上架靠**机柜 ID + 起始 U + 高度**，无坐标
- `racks`（`:799`）字段：`id, name, size, subrack, location, row, hasBack, topDown, description, customer_id` → **只有机柜图（`hasBack`/`topDown` 控制渲染方向），没有平面坐标**
- `rackContents`（`:820`）字段：`name, rack, rack_start, rack_size, rack_deep, subrackId`
- `settings` 有 `enableRACK`、`rackImageFormat enum('png','svg')`、`rackAllowOverlap`
- ❌ **全库无 port / port-link / cable 表** → 布线能力为 **0**
- ✅ IPAM 部分很强：`subnets`(含嵌套)/`ipaddresses`/`vlans`/`vlanDomains`/`vrf`/`nat`/`routing_bgp`/`routing_subnets`/`firewallZones*`/`circuits*`/`vaults`(凭据保险箱)
- ✅ 有 `scanAgents` 表 → 支持分布式扫描发现

**其他**
- REST API：有，文档在 `https://phpipam.net/api-documentation/`
- 认证：`functions/adLDAP/`（LDAP 集成，自带 `LICENCE.txt`）
- 3D：无
- 布线表：无

**评价**：**纯 IPAM 项目**，DCIM 部分只到"机柜图"级别。→ 对本项目只在 **IP 地址段规划**（用户 Excel 有 `IP地址段规划` sheet）这一小块有参考价值。

---

### 4.6 iTop（★1204，AGPL-3.0）与 TeemIP（★32，AGPL-3.0）

**iTop**
- 仓库：https://github.com/Combodo/iTop
- 许可证：**AGPL-3.0**（`license.txt` 原文首行 `GNU AFFERO GENERAL PUBLIC LICENSE Version 3, 19 November 2007`）
- 最近推送：2026-10-20；最新 release **3.3.0**（2026-09-15）；近 90 天约 226 次提交 → **很活跃**
- 定位：**CMDB 框架 + ITSM**（Ticket / CI / 契约 / 变更），不是 DCIM
- 机房/机柜视图靠扩展 `Molkobain/itop-datacenter-view`（**★31**，2026-09-21，**无 LICENSE 字段 = 未声明许可，商用需确认**）
  - 生态里还有 `itop-datacenter-view-extended-bridge-for-teemip`（★2）、`itop-datacenter-view-bridge-for-combodo-location-hierarchy`（★0，MIT）、`iTop-Generic-Connection-To-Datacenter-View`（★1，GPL-3.0）
- 布线/线长：无

**TeemIP**
- 仓库：https://github.com/teemIP/teemip-core-ip-mgmt（**注意：`Combodo/teemip` 是 404**）
- 许可证：**AGPL-3.0**（`LICENSE` 原文已核）
- 最近推送：2026-10-06；★32 —— **非常小众**
- 定位：跑在 iTop 上的 IPAM 扩展
- 另有 `teemIP/teemip-cable-mgmt`（**★7**，GPL-3.0，2026-06-25），描述 *"Documents physical connections between devices in TeemIP"* → 只做"文档化"，无长度计算

**评价**：iTop 本体活跃但**是 ITSM/CMDB 不是 DCIM**；AGPL-3.0 对"私有化交付给客户"有传染性风险（AGPL 要求向网络用户提供源码）。→ **不推荐**，除非未来要做工单/流程。

---

### 4.7 GLPI（★6475，GPL-3.0）— 活跃度全场第一，且真有 DCIM 模块

**基本信息**
- 仓库：https://github.com/glpi-project/glpi
- 许可证：**GPL-3.0**（`LICENSE` 原文首行 `GNU GENERAL PUBLIC LICENSE Version 3, 29 June 2007`）
- 最近推送：2026-10-20；最新 release **12.0.0**（2026-10-07）；**近 90 天约 739 次提交 —— 全部候选中活跃度最高**
- 语言 PHP

**一个需要纠正的常见说法："GLPI 没有 rack/端口/线缆模型" —— 这是错的**

【源码级】GLPI 12.0 有完整的 DCIM 模块，`install/mysql/glpi-empty.sql` 中确认存在：

| 表 | 行号 | 关键字段 |
|---|---|---|
| `glpi_datacenters` | 8506 | 数据中心 |
| `glpi_dcrooms` | 8527 | `vis_cols, vis_rows, vis_cell_width, vis_cell_height, column_labels, row_labels, **blueprint**` |
| `glpi_racks` | 8589 | `width, height, depth, number_units, dcrooms_id, room_orientation, **position varchar(50)**, bgcolor, max_power, mesured_power, max_weight` |
| `glpi_items_racks` | — | `racks_id, itemtype, items_id, position, orientation, hpos, is_reserved` — 多态关联（任何资产都能上架） |
| `glpi_sockets` | 4623 | `position, name, socketmodels_id, **wiring_side**, itemtype, items_id, networkports_id` |
| `glpi_cables` | 4651 | `itemtype_endpoint_a/b, items_id_endpoint_a/b, socketmodels_id_endpoint_a/b, sockets_id_endpoint_a/b, cablestrands_id, color, cabletypes_id` |
| `glpi_cabletypes` | — | 只有 `id, name, comment` —— **极简，无芯数/极性** |
| `glpi_pdus` | 8773 | PDU 配电 |
| `glpi_networkports` | 5045 | 网络端口 |

对应源码：`src/Datacenter.php`, `src/DCRoom.php`, `src/Rack.php`, `src/RackType.php`, `src/RackModel.php`, `src/Item_Rack.php`, `src/PDU_Rack.php`, `src/Cable.php`, `src/Glpi/Features/DCBreadcrumb.php`。

**⭐ 重要亮点：GLPI 有机房平面图的"网格 + 底图"方案**
- `glpi_dcrooms` 有 `vis_cols` / `vis_rows`（网格行列数）+ `vis_cell_width` / `vis_cell_height`（单元格像素）+ `column_labels` / `row_labels`（自定义轴标签）+ **`blueprint`（底图图片）**
- `glpi_racks.position` 是 `varchar(50)`，存 `"x,y"` 字符串；`src/DCRoom.php:425-440` 的 `getFilled()` 用 `preg_match('/(\d+),\s?(\d+)/', $rack['position'])` 解析，`:445` `getAllPositions()` 用 `vis_cols × vis_rows` 枚举所有格位
→ **方案是"网格单元定位 + 底图叠加"，比 openDCIM 的自由矩形坐标简单，但工程上更实用**（不需要精确坐标也能排出机房图）。**这个思路最适合本项目起步**（用户 Excel 的"机房平面图"本来就是单元格拼的）。

**布线能力**
- ❌ **无 cable length**（`glpi_cables` 无 length 字段，全表 grep `length` 仅命中 `name varchar(255)` 之类）
- ❌ 无跨机柜计算
- `glpi_cabletypes` 只有 `id/name/comment` → 无法表达芯数、极性、连接器

**其他**
- 3D：无
- 定位：**ITAM + ITSM**（资产 + 工单 + 软件许可），DCIM 只是其中一块。README 自述 *"Data Center Infrastructure Management (DCIM): Provides features for managing data center infrastructure"*
- 配套：`OCSInventory-NG/OCSInventory-Server`（★405，GPL-2.0，Perl）做客户端自动清点
- 生态：`OpenMandrivaAssociation/glpi-plugin-racks` **★1** —— 说明 GLPI 的机柜插件生态几乎为零

**评价**：GLPI 是"顺带做 DCIM 的 ITAM"，功能广度大但 DCIM 深度浅。**唯一值得抄的是 `dcrooms` 的网格 + blueprint 平面图方案。**

---

### 4.8 其他值得记录的项目（简要）

| 项目 | 定位 | 对本项目 |
|---|---|---|
| [opsmill/infrahub](https://github.com/opsmill/infrahub) ★538 Apache-2.0 | 图数据库式 SoT（节点/关系/分支），有 GraphQL + Git 式分支 | 🟡 若未来要做"变更分支/合并"，其分支模型值得看；但学习曲线陡 |
| [allegro/ralph](https://github.com/allegro/ralph) ★2529 Apache-2.0 | Allegro 内部 DCIM + 资产管理，Python/Django | 🟡 数据模型可参考，但强绑定波兰机房流程 |
| [netdisco/netdisco](https://github.com/netdisco/netdisco) ★930 BSD-3-Clause | Perl，SNMP 自动发现网络设备 + 拓扑 | 🟡 **本期不做巡检**，未来"自动化巡检"阶段再看 |
| [SpriteLink/NIPAP](https://github.com/SpriteLink/NIPAP) ★593 MIT | Python，IP 地址池管理 + 分配 | 🟡 IP 分配算法可参考 |
| [Kobii-git/rackpad](https://github.com/Kobii-git/rackpad) ★417 MIT | TypeScript，homelab 级机柜台账，有 API 路线图 | 🟡 技术栈与本案接近（TS），可看其机柜交互 |
| [napalm-automation/napalm](https://github.com/napalm-automation/napalm) ★2515 Apache-2.0 | 多厂商网络设备驱动抽象 | ⚪ 本期不需要（不做设备配置下发） |

---

### 4.9 小结：没有一个开源项目解决本项目核心问题

把 §4.1–4.8 的结论压成一张表：

| 能力 | NetBox | Nautobot | openDCIM | RackTables | phpIPAM | iTop | GLPI |
|---|---|---|---|---|---|---|---|
| 设备/端口/连接建模 | ✅ 强 | ✅ 最强（lane/极性） | ✅ 中 | ✅ 强（三面） | ❌ 无端口 | ⚪ 弱 | ✅ 中 |
| 机柜 U 位可视化 | ✅ **SVG + JSON API** | ✅ SVG | ✅ | ✅ | ✅ 仅图 | ⚪ 靠扩展 | ✅ |
| **机柜平面坐标** | ❌ | ❌ | ✅ **矩形坐标** | ❌ | ❌ | ⚪ 扩展 | ✅ **网格+底图** |
| **3D** | ❌ 靠插件 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **线缆长度存储** | ✅ Decimal(8,2) | ✅ 整数 | ❌ | ❌ | ❌ | ❌ | ❌ |
| **线缆长度计算** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| **布线表派生列** | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Excel 布线表导出 | ⚪ Export Template 手搓 | ⚪ 同 | ✅ **原生** | ❌ | ❌ | ❌ | ⚪ |
| CSV 批量导入 | ✅ 全模型 | ✅ | ✅（Excel） | ❌ | ⚪ | ✅ | ✅ |
| REST API | ✅ 优秀 | ✅ 优秀 | ⚪ 薄 | ❌ **无** | ✅ | ✅ | ✅ |
| GraphQL | ✅ 只读 | ✅ 只读 | ❌ | ❌ | ❌ | ❌ | ⚪ |
| 插件扩展点 | ✅ Django app | ✅ App framework + Jobs | ❌ | ⚪ 2 个 | ⚪ | ✅ | ✅ |
| 近 90 天活跃度 | ~418 | ~205 | ~4 | **0** | ~22 | ~226 | **~739** |

**这张表里"线缆长度计算"和"布线表派生列"两行全是 ❌，就是本项目存在的理由。**

---

## 5. NetBox 能不能当底座：六条判据逐条打分

> 这一节回答的是"把 NetBox 装上、写个插件、当成我们产品的后端"这条路是否成立。
> 判据不是拍脑袋定的，是从**用户已有的 Excel 交付物反推**出来的：一个系统要能替代现在的 Excel 流程，必须逐列、逐流程对得上。

### 5.1 判据一：数据模型能否承载现有交付物的列？

先把用户真实交付物的列逐列映射一遍。数据源：`信息港部署上架/V13(内部)甘肃信息港-设备上架规划-20260714.xlsx` 的 `布线表` sheet（5133 行 × 25 列）。

| # | 用户 Excel 列（逐字） | NetBox 对应 | 判定 |
|---|---|---|---|
| 1 | 本端设备 | `Device.name` | ✅ |
| 2 | 本端设备名称 | `Device.name` / `DeviceType.model` | ⚠️ 与第 1 列语义重叠，NetBox 只有一个 name |
| 3 | 本端通道 | 无（`CableBundle` 语义不同） | ❌ 需自建 |
| 4 | 本端机柜 | `Device.rack` | ✅ |
| 5 | U位 | `Device.position`（DecimalField 4,1） | ✅ 但用户是 `3-12U` 这种"区间"字符串，NetBox 存的是起始 U + `DeviceType.u_height` |
| 6 | 本端接口 | `Cable` 的 A 端 termination（`Interface`） | ✅ |
| 7 | 对端设备 | 同上 B 端 | ✅ |
| 8 | 对端设备名称 | 同上 | ⚠️ |
| 9 | 对端通道 | 无 | ❌ |
| 10 | 对端机柜 | B 端 `Device.rack` | ✅ |
| 11 | U位（对端） | B 端 `Device.position` | ✅ |
| 12 | 对端接口 | B 端 termination | ✅ |
| 13 | 线缆类型 | `Cable.type`（CableTypeChoices） | ✅ 需映射 MPO→`mmf-om4`、LC-LC→`smf-os2`、RJ45→`cat6a` |
| 14 | **跨机柜** | **无** | ❌ **派生列** |
| 15 | **间隔机柜** | **无** | ❌ **派生列** |
| 16 | **机柜数量** | **无** | ❌ **派生列** |
| 17 | **机内线缆** | **无** | ❌ **派生列（长度）** |
| 18 | **柜间线缆** | **无** | ❌ **派生列（长度）** |
| 19 | **线缆2** | **无** | ❌ **派生列（长度小计）** |
| 20 | **线缆长度1** | `Cable.length`（可存） | ⚠️ 只能存，不能算 |
| 21 | **线缆长度** | `Cable.length` | ⚠️ 同上，且与第 20 列取值重复 |
| 22 | 本端模块 | 无（`InventoryItem` 勉强，非结构化） | ❌ |
| 23 | 对端模块 | 同上 | ❌ |
| 24 | 备注1 | `Cable.description` / `comments` | ✅ |
| 25 | （第 25 列） | — | 见下 |

**打分：25 列里 6 列完全无处安放（14–19），3 列只能存不能算（20、21）+ 2 列模块信息无结构（22、23）。**

→ **判据一结论：不通过。** 这不是"字段名对不上"的小问题，而是**派生列没有容身之处**。派生列是这套交付物的价值所在（施工队按"柜间线缆 10.08 米"备料），把它挪到 NetBox 里就意味着**要么放弃派生列，要么在 NetBox 外面再维护一张表** —— 后者等于"NetBox 只管一半，另一半还是 Excel"。

顺带核实一件事：用户 Excel 里 `线缆长度1` 与 `线缆长度` 两列取值**完全相同**（抽查第 2 行均为 15，另一行为 20）。这说明现有流程里这两列是历史遗留冗余，**不是两套不同口径的长度**。自研时可以直接合并成一列，减少一次口径歧义。

### 5.2 判据二：插件能不能补上这些缺口？

这是"插件化路线"的核心假设。逐项验证：

| 要补的能力 | 插件能做到吗 | 证据 |
|---|---|---|
| 给 `Rack` 加平面坐标 | ⚠️ 技术上可以（插件可注册新模型 + 通过 OneToOne 挂到 Rack） | `netbox/netbox/plugins/registration.py` 支持注册模型；但**改不了 Rack 的 API 序列化器**，等于坐标只能在自己的插件页面里用，NetBox 原生 UI 看不到 |
| 给 `Cable` 加派生列（跨机柜/间隔机柜/…） | ⚠️ 同上，能加 sidecar 表 | 但 **`Cable.clean()` / `Cable.save()` 改不了**（`netbox/dcim/models/cables.py:302`），意味着派生列无法在 NetBox 原生"Cable 编辑页"上自动重算，只能在插件自己的页面里维护 |
| **拦截 Cable 创建并触发线长计算** | ❌ **做不到干净** | 官方插件 API 里没有"模型保存前后钩子"这一类注册点。`registration.py` 的 `__all__`（`:16-26`）共 **9 个**注册函数：`register_graphql_filter_extensions` / `register_graphql_schema` / `register_graphql_type_extensions` / `register_jinja_filters` / `register_menu` / `register_menu_items` / `register_serializer_resolver` / `register_template_extensions` / `register_user_preferences` —— **没有任何一个是模型信号/保存钩子**。`register_serializer_resolver`（`:240`）虽然能让插件接管自家模型的序列化器，但文档明确限定 *"Plugins (and internal apps) should only register resolvers for their own models"*，**不能接管 Cable 的序列化**。想挂 `post_save` 只能靠 monkey-patch，官方明确不保证 |
| 替换"布线表导出"格式 | ✅ 能做到 | Export Templates（Jinja2），插件可注册 Jinja filter（`register_jinja_filters`）。**但 filter 只能对单条记录做变换，拿不到"整个机房所有机柜的相对位置"这种全局上下文** |
| 加一套"布线规划"页面 | ✅ 能做到 | 插件可以加 view + template + 导航菜单项 |

**打分：5 项里 1 项干净可做、3 项能凑合但被限制在插件自己的页面里、1 项（保存钩子）做不到干净。**

关键的官方警告再引一次（`docs/plugins/development/index.md` 逐字）：
> *"Any part of the NetBox code base not documented here is **not** part of the supported plugins API, and should not be employed by a plugin. Internal elements of NetBox are subject to change at any time and without warning."*

→ **判据二结论：不通过。** 插件能"在旁边盖房子"，不能"改地基"。而本项目要的恰恰是改地基（线长是 Cable 的核心语义，不是附加信息）。

### 5.3 判据三：5133 行布线表能不能顺利灌进去？

- **导入通道**：有。`netbox/dcim/forms/bulk_import.py:1866` 的 Cable CSV 导入列已核实，`length` 和 `length_unit` 都在列内。理论上可以从 Excel 转 CSV 后导入。
- **但导入有三个硬门槛**：
  1. **termination 必须先存在**。Cable 的 A/B 端写的是 `side_a_device` + `side_a_name`（接口名），意味着**导入 Cable 之前必须先把设备、设备型号、接口全部建好**。用户 Excel 的 5133 行涉及的设备/接口数量级不小，这是一次"先建设备再建线"的两阶段导入。
  2. **`DeviceType` 必须先有**。NetBox 的 `Device` 必须挂 `DeviceType`（不能裸建），所以要么从 `devicetype-library` 里找到对应型号，要么为现网设备手搓 YAML。**已实测（§7.1.1）：该库对国内信创设备覆盖率仅 0.97%，H3C/绿盟/启明星辰/深信服等全部为 0** —— 意味着导入前要先为本案 28 类设备手工建 `DeviceType`。
  3. **派生列无处可放**（同 5.1），导入时只能丢弃第 14–19 列。
- **结论**：**技术上能导，但导进去的是"残缺版"交付物**。导入完成后，用户拿 NetBox 导不出原来那张表。

### 5.4 判据四：四项具体能力各自到什么程度（回答 lead 的 B 问题）

| 能力 | NetBox 本体 | + 生态插件 | 最终判定 | 差什么 |
|---|---|---|---|---|
| **2D 机柜正视图（U 位）** | ✅ **很强**。`GET /api/dcim/racks/{id}/elevation/?render=svg` 直接返回 SVG；不带 `render` 返回 JSON RackUnit 列表 | `netbox-reorder-rack`（★112）做了**拖拽重排** | 🟢 **够用且值得抄** | 只差"按我们的配色/图例定制"。**服务端渲 SVG + 同时给 JSON 这个双通道设计是本项目应该直接照搬的** |
| **2D 机房平面图** | ❌ **完全没有**。`Rack` 无坐标字段（grep 全 `racks.py` 无 `position_x/y`/`coordinate`） | `tbotnz/netbox_floorplan`（★80）**已归档**；`net3d` 有可编辑平面图但 ★25；`gadler01` 线长插件带拖拽定位但 ★2 | 🔴 **不够用** | 需要一个"机柜 → (x, y, rotation)"的坐标层，NetBox 不提供，插件层全是小项目 |
| **3D 机房** | ❌ 无 | `nextbox-ui-plugin`（★642，MIT）**2025-05-24 后未更新，至今约 17 个月**；`net3d`（★25）为 globe→site→rack 分层 | 🔴 **不够用** | 3D 必须自研或用通用 Three.js 栈（见 [02-visualization-stack.md](./02-visualization-stack.md)） |
| **跨机柜线长计算** | ❌ **完全没有** | 唯一实现是 `gadler01/Netbox_Cable_Calculator_Plugin`（**★2**，MIT，2026-10-05），参数为英寸硬编码（`aisle_width: 60`、`rack_depth: 48`、`port_depth: 4`、`fiber_overhead: 18`、`copper_overhead: 12`、`default_slack_pct: 10`），**无 TIA 档位表、无路径规划图**；`netbox-pathways`（★21）用 PostGIS 算 `geo_length`，是**室外管线**级，且需 PostGIS + 自称 alpha | 🔴 **不够用** | 整套线长算法要自己写（见 [03-cabling-algorithms.md](./03-cabling-algorithms.md)） |
| **批量导入导出** | ✅ 导入强（全模型 CSV bulk import）；导出**弱**（Jinja2 Export Template 只能拿已有字段，算不了派生列；且输出的是文本不是 xlsx） | `openDCIM` 的 `connections_spreadsheet.php` 是唯一"原生 Excel 布线表导出"参考实现 | 🟡 **导入够用，导出不够用** | 导出 xlsx（带多 sheet、带样式、带耗材汇总）必须自研 |

**一句话总结（B 问题答案）**：
> NetBox + 插件在 **U 位可视化**这一项上做得比大多数自研方案都好（SVG + JSON 双通道是教科书级设计，直接抄）；在 **批量导入** 上够用；在 **机房平面图** 上必须外挂且生态全是不成熟的小项目；在 **3D** 和 **跨机柜线长计算** 上等于零。

### 5.5 判据五：升级与维护成本

| 项 | 事实 | 影响 |
|---|---|---|
| 发布节奏 | NetBox v4.7.3 发布于 2026-10-13，近 90 天约 418 次提交 | 快节奏 → 插件需持续跟进 |
| 插件兼容性压力 | `netbox-topology-views` 的兼容矩阵：NetBox 4.7.X ↔ v5.0.0，4.5.X ↔ v4.5.1（**每个 NetBox 大版本都要发插件新版本**） | 我们的插件每次 NetBox 升级都要重测 |
| 插件 API 稳定性 | 官方明说 "Internal elements … subject to change at any time and without warning" | 无兼容性承诺 |
| 反面案例 | `nextbox-ui-plugin`（★642）2025-05 后停更，如今已跟不上 NetBox 4.7；`netbox_floorplan`（★80）直接归档 | **社区插件的死亡率很高**，把核心能力押在插件上风险大 |

→ **判据五结论：插件化路线的长期维护成本被严重低估。** 我们要维护的不是"一个插件"，而是"一个永远追着 NetBox 大版本跑的插件"。

### 5.6 判据六：许可证与商业化

| 组件 | 许可证 | 商用/私有化交付风险 |
|---|---|---|
| NetBox 本体 | Apache-2.0 | 🟢 无（可闭源分发、可商用） |
| `devicetype-library` | **CC0-1.0** | 🟢 无（公共领域） |
| `pynetbox` | Apache-2.0 | 🟢 无 |
| `netbox-branching` / `diode` | **NetBox Limited Use License 1.0** | 🔴 **非 OSI 许可**：仅限自有内部使用，不得分发、不得作为商业产品的一部分提供 |
| `netbox-pathways` / `netbox-fms` | AGPL-3.0 | 🟡 若做成网络服务需开源 |
| `openDCIM` / `phpIPAM` | GPLv3（README 声明，无 LICENSE 文件） | 🟡 GPL 传染性；且**无标准 LICENSE 文件本身是法务隐患** |
| `RackTables` | GPL-2.0 | 🟡 同上 |
| `iTop` / `TeemIP` | **AGPL-3.0** | 🔴 网络服务场景下传染性最强 |
| `GLPI` | GPL-3.0 | 🟡 |
| `net3d` / `netbox-cable-labels` 等小插件 | MIT / Apache-2.0 / AGPL 混杂 | 逐个确认 |

→ **判据六结论**：**许可证本身不阻碍"以 NetBox 为导出目标"**（Apache-2.0 干净），但**阻碍"以 openDCIM/iTop 为底座"**（GPL/AGPL 传染 + 无 LICENSE 文件）。

### 5.7 六条判据汇总

| 判据 | 结论 | 是否通过 |
|---|---|---|
| 一、数据模型承载交付物列 | 25 列中 6 列无处安放、3 列只能存不能算 | ❌ |
| 二、插件能补缺口 | 保存钩子类扩展做不到干净，官方不承诺 API 稳定 | ❌ |
| 三、5133 行可灌入 | 技术上可导，但导入后导不出原表 | ⚠️ 半通过 |
| 四、四项能力覆盖 | U 位可视化够用；平面图/3D/线长计算三项等于零 | ❌ |
| 五、维护成本 | 需永久追 NetBox 大版本，社区插件死亡率高 | ❌ |
| 六、许可证 | NetBox 本体 Apache-2.0 干净；其他候选 GPL/AGPL 有传染 | ✅（仅对 NetBox） |

**最终结论（C 问题答案）：**

> **不值得把 NetBox 当底座。** lead 的倾向是对的，但理由需要修正一处：
> 不是"NetBox 的 cable 模型极简且**不存**线长"（它**存**，`DecimalField(8,2)`，见 §2.3），
> 而是 **"NetBox 存得下线长、存不下算线长所需的几何信息，也算不出派生列，且插件层补不上"**。
>
> 更准确的一句话：**NetBox 是一个优秀的"连接关系数据库"，而本项目要的是一个"布线工程量计算器"。两者是不同物种，不是同一物种的不同实现。**

**但要保留一条通道**：把 NetBox 当**导出目标 / 对照系统**（单向导出 `Site/Rack/Device/Interface/Cable` + `Cable.length`），成本很低（`pynetbox` ★687 Apache-2.0 现成），收益是"客户已有 NetBox 时能对接"。这条要做，但不构成"当底座"。

---

## 6. 三条路线对比

### 6.1 对比表

| 维度 | 路线 A：完全复用（装 NetBox 直接用） | 路线 B：插件化（NetBox + 自研插件） | 路线 C：自研核心 + 借鉴互操作 |
|---|---|---|---|
| **交付物能否 1:1 复现现有 Excel** | ❌ 不能（6 列派生字段无处安放） | ⚠️ 勉强（派生列住插件表，与 NetBox 原生 Cable 页割裂） | ✅ 能（数据模型自己定） |
| 线长算法可控性 | ❌ 全靠人工填 5133 行 | 🟡 算法在插件里，但触发点（保存钩子）挂不干净 | ✅ 完全可控 |
| 5133 行导入 | ⚠️ 需两阶段（先设备/接口，再 Cable），派生列丢弃 | ⚠️ 同 + 要写插件导入器 | ✅ 直接按 Excel 结构导入 |
| Excel 导出（多 sheet/耗材汇总/xlsx 样式） | ❌ Export Template 只能出文本 | 🟡 插件里用 openpyxl 重写一遍 | ✅ 原生用 openpyxl |
| 2D 机柜 U 位图 | ✅ 现成（且质量高） | ✅ 现成 | 🟡 需自研，但可照抄 NetBox 的 SVG+JSON 双通道设计 |
| 2D 机房平面图 | ❌ 无 | ⚠️ 插件生态全是不成熟小项目 | ✅ 自研（参考 GLPI 网格+底图 / openDCIM 矩形坐标） |
| 3D | ❌ | ⚠️ 只能挂外部渲染器 | ✅ 自研（见 02 文档） |
| 开发周期（估） | 1 周部署 + 无限期人工填数 | **8–12 周**（含 NetBox 学习、插件框架、兼容性适配） | **6–8 周**（核心算法 + 导入导出 + 前端） |
| 长期维护 | 低（跟着官方升级即可） | **高**（永久追 NetBox 大版本 + 插件 API 无承诺） | 中（自己的代码自己控） |
| 技术栈自由度 | ❌ 绑死 Python/Django | ❌ 绑死 Python/Django | ✅ 自由 |
| 与现网 NetBox 对接 | 天然 | 天然 | 🟡 需写导出适配器（用 pynetbox，成本低） |
| 许可证风险 | 🟢 Apache-2.0 | 🟢 Apache-2.0 | 🟢 自己定 |
| **结论** | ❌ 否决 | ⚠️ 仅当"必须与现网 NetBox 共存"时 | ✅ **推荐** |

### 6.2 为什么"插件化"看起来很美但实际更贵

这是最容易被低估的一条。把它拆开算：

1. **学习成本不低**：要读懂 NetBox 的 `Cable`/`CablePath`/`CableBundle` 语义、`PathEndpoint`/`CabledObjectModel` 抽象、DRF 序列化器体系、模板扩展机制 —— 这部分成本与"自研一套等价模型"相比，**并不更省**，因为自研时你只需要实现自己用到的部分。
2. **被绑在别人的抽象上**：NetBox 的 `Cable` 是两点直连模型，没有"路径"概念。要在它上面表达"这根线经过 3 个桥架、跨 14 个机柜"，只能加 sidecar 表，于是你得到**两套并行的真相**（NetBox 的 Cable + 你的路径表），一致性全靠自己维护。
3. **触发点挂不上**（见 §5.2）：最关键的"改了机柜位置要重算线长"，在插件体系里没有干净的钩子。
4. **升级即重测**：`netbox-topology-views` 的兼容矩阵证明——每来一个 NetBox 大版本，插件就要发一版。我们的插件同样躲不掉。
5. **社区插件的死亡率佐证**：`netbox_floorplan`（★80）已归档、`nextbox-ui-plugin`（★642）停更 17 个月、`netbox-plugin-device-map`（★46）停更 2 年。**★ 数不低也照样死**。

### 6.3 路线 C 的具体形态（推荐）

```
┌──────────────────────────────────────────────────────────┐
│  自研核心（唯一真相源）                                    │
│                                                          │
│  数据模型层：Site → Room → Row → Rack(x,y,rotation)       │
│              Device → Slot(U位) → Port                    │
│              Cable(含 length 派生列 + 路径引用)            │
│              Pathway(桥架/走线槽，含几何)                  │
│                                                          │
│  算法层：线长计算（机内/柜间/档位取整）                    │
│          耗材汇总（+30% 采购冗余）                         │
│                                                          │
│  交付层：xlsx 导出（布线表/连线表/耗材清单，多 sheet）      │
│          导入（V9/V11/V12/V13 五版式兼容）                 │
└──────────────────────────────────────────────────────────┘
        │ 白拿资产                    │ 对外互操作
        ▼                            ▼
  devicetype-library           NetBox 导出适配器
  (CC0, 华为 103 型号可用)      (pynetbox, 单向/双向可选)
```

**白拿清单（回答 lead 的 D 问题）**，详见 §7。

### 6.4 什么情况下应该改选路线 B

诚实起见，列一下**会推翻上述结论的条件**：

| 条件 | 为什么会影响结论 |
|---|---|
| 客户明确要求"数据必须存在我们自己已有的 NetBox 里" | 那 NetBox 就是既成事实的 SoT，自研核心要退化为"规划器 + 导出器"，而不是替代 NetBox |
| 需求扩展到"现网设备自动发现 + 配置备份 + 变更下发" | NetBox + NAPALM/Ansible 生态成熟（`napalm` ★2515、`ansible_modules` ★402），自研这套不划算 |
| 团队只有 Django 经验、无前端能力 | 插件化能用 NetBox 现成 UI 顶掉大部分页面工作 |

**目前这三条都不成立**（本项目核心是"上架规划 + 布线计算 + Excel 交付"，不是设备运维），所以推荐路线 C。

---

## 7. 互操作与数据模型借鉴建议

这一节是"自研时抄什么、怎么抄、抄哪里"的清单。原则：**抄设计和数据，不抄代码**（避免许可证问题，也避免继承别人的历史包袱）。

### 7.1 可以直接白拿的资产（许可证干净，０成本）

> ⚠️ **先说一个实测结论，它会影响下面这一节的判断力度。**
> 我把 `devicetype-library` 的完整文件树拉下来（`git/trees?recursive=1`，`truncated=false`，共 18195 个条目）按厂商统计了一遍，结论是：**这个库对国内信创设备几乎无用**。详见 §7.1.1。

| 资产 | 来源 | 许可证 | 怎么用 |
|---|---|---|---|
| **设备型号库**（⚠️ 国外设备可用，国内设备基本无覆盖） | [netbox-community/devicetype-library](https://github.com/netbox-community/devicetype-library) ★1631 | **CC0-1.0（公共领域）** | 已核实含 **12390** 个 `device-types` 下的 `.yaml` 或 `.yml`（其中 11994 个 `.yaml` + 396 个 `.yml`）+ **2036** 个 `module-types` + **147** 个 `rack-types` + 2887 张 `elevation-images`，覆盖 **371 个厂商**。YAML 里有 `manufacturer` / `model` / `u_height` / `is_full_depth` / `front_image` / `rear_image` / `interfaces`（含 type、mgmt_only）。**但见 §7.1.1 的覆盖率实测——对本案作用有限** |
| **接口/端口类型枚举** | NetBox `CableTypeChoices`（`netbox/dcim/choices.py:1906`） | Apache-2.0（枚举值本身无版权争议，仍建议注明来源） | cat3–cat8 / dac-active / dac-passive / **mmf-om1..om5** / **smf-os1/os2** / **aoc** / coaxial / power / usb。**本项目线缆类型直接采用这套命名，避免自造词** |
| **线缆 profile（芯数×位置）** | NetBox `CableProfileChoices`（`choices.py:1830`） | 同上 | `single-1c1p` … `single-1c16p`、`trunk-2c1p`…`trunk-4c8p`。**用于表达 MPO 主干拆 4×25G 这类场景**（用户 Excel 实测有 353 条 MPO 连线） |
| **机柜 U 位渲染 API 设计** | NetBox `netbox/dcim/api/views.py:206` + `racks.py:510/573/613/623` | 设计思路 | ⭐ **双通道**：`?render=svg` 返回图，否则返回结构化 RackUnit JSON；`get_rack_units(face, user, exclude, expand_devices)` / `get_available_units()` / `get_reserved_units()` 三个方法的职责切分非常清晰。**直接照这个接口形态设计我们自己的 API** |
| **连接器↔端口兼容矩阵** | RackTables `PortCompat` / `PortOuterInterface` / `PortInnerInterface` | GPL-2.0（**只抄"用表驱动"这个设计，不要抄表数据**） | 用于"LC 不能插 RJ45""MPO 母头不能直连 LC"这类校验 |

#### 7.1.1 ⚠️ 关键实测：`devicetype-library` 对国内信创设备**基本没有覆盖**

这一条原计划标"待核实"，实际做了，结论比预期严重，单独列出来。

**做法**：拉 `device-types/` 下全部 12390 个文件的路径，按 `<vendor>/<model>.yaml` 的第一级目录统计厂商。

**结果（按设备数排序，前 30 厂商）**：

| 厂商 | 型号数 | | 厂商 | 型号数 |
|---|---|---|---|---|
| Dahua（大华） | 1248 | | Juniper | 296 |
| Cisco | 1030 | | Arista | 291 |
| HPE | 534 | | LILIN | 282 |
| Vivotek | 496 | | HiLook | 244 |
| TVT Digital | 403 | | Extreme Networks | 231 |
| i-PRO | 354 | | Ubiquiti | 210 |
| Uniview（宇视） | 353 | | Bosch | 195 |
| Hikvision（海康） | 298 | | Kedacom（科达） | 187 |

**国内网络/安全厂商的实际情况**：

| 厂商 | 在库型号数 | 说明 |
|---|---|---|
| Huawei（华为） | **103** | 有，且包含 CE6855/CE6865/CE8850/NE8000/NE40E 等**与本案相关的型号** |
| ZTE（中兴） | 8 | 极少 |
| Inspur（浪潮） | 2 | 极少 |
| Raisecom（瑞斯康达） | 4 | 极少 |
| Ruijie（锐捷） | **3** | 几乎为零 |
| **H3C / 新华三** | **0（厂商目录完全不存在）** | ❌ |
| **Sugon（曙光）** | **0** | ❌ |
| **Fiberhome（烽火）** | **0** | ❌ |
| **Maipu（迈普）** | **0** | ❌ |
| **绿盟 NSFOCUS** | **0** | ❌ |
| **启明星辰 Venustech** | **0** | ❌ |
| **深信服 Sangfor** | **0** | ❌ |
| **天融信 Topsec** | **0** | ❌ |
| **迪普 DPtech** | **0** | ❌ |
| **山石 Hillstone** | **0** | ❌ |
| **奇安信 QiAnXin** | **0** | ❌ |

**汇总**：国内网络厂商（华为+中兴+浪潮+瑞斯康达+锐捷）合计 **120 条 / 12390 条 = 0.97%**；**中国厂商之外的欧美/日韩/安防厂商占 99%+**，其中大华、海康、宇视这类视频监控厂商就占了近 2400 条。

**为什么会这样**：这个库是 NetBox 社区自发贡献的，贡献者以欧美 homelab/企业为主，国内设备要靠国内用户自己 PR。**这是"社区众包"模式的必然结果，不是库的质量问题。**

**对本项目的实际影响**：

1. **用户 Excel 里根本没有品牌/型号列**。`首期设备清单` 的列是 `类型 | 名称 | 配置 | 数量 | 套餐 | 使用规划 | 上联设备`，`布线表` 里的"设备"是 `业务核心交换机-1`、`计算节点A（ARM）`、`防火墙1` 这类**功能性命名**，不是型号。型号信息只出现在 `配置` 列的散文描述里（如"48*25GE SFP28,8*100GE QSFP28"）。
2. 因此**"白拿型号库"这条路在本项目中收益极小** —— 真正要建的不是"设备型号库"，而是**"设备角色/套餐 → 端口配置模板"**（比如"25G8A 套餐 = 48×25GE + 8×100GE"）。这是 NetBox 的 `DeviceType` 加 `InterfaceTemplate` 能表达、但库里没有现成数据的东西。
3. **可白拿的部分缩小为**：华为那 103 个型号里的交换机/路由器（CE 系列、NE 系列、S 系列）**可以直接用**，能省一点手工；其余国内设备要自己建。
4. **真正值钱的是 YAML 的 schema 和数据组织方式**（见下），不是数据本身。

**YAML 的结构本身值得抄**（这是 `DeviceType` 最小可用字段集的现成答案）。下面是我从 `device-types/Huawei/CE6865-48S8CQ-EI.yaml` **原样摘录的真实内容**（不是示意）：

```yaml
---
manufacturer: Huawei
model: CE6865-48S8CQ-EI
slug: huawei-ce6865-48s8cq-ei
part_number: 02351RFC
description: CE6865-48S8CQ switch (48*25G SFP28, 8*100G QSFP28)
u_height: 1
is_full_depth: true
weight: 6.2
weight_unit: kg
comments: CE6865-48S8CQ-EI (https://support.huawei.com/enterprise/en/doc/...)
console-ports:
  - name: console0
    type: rj-45
    label: CONSOLE
module-bays:
  - name: FAN1
    position: '1'
  - name: FAN2
    position: '2'
  - name: PSU1
    position: '1'
  - name: PSU2
    position: '2'
interfaces:
  - name: MEth0/0/0
    type: 1000base-t
    label: ETH
    mgmt_only: true
  - name: 25GE1/0/1
    type: 25gbase-x-sfp28
  - name: 25GE1/0/2
    type: 25gbase-x-sfp28
  ...
```

> 💡 **一个值得注意的巧合**：这个 YAML 里的端口名 `25GE1/0/1`、`MEth0/0/0`，和用户 Excel 布线表里的 `100GE1/0/1` 是**同一套华为 CLI 板卡式命名**。说明用户现有数据本来就贴着厂商命名规范走，不需要做命名转换 —— 这是一个意外的好消息。

→ **`schema/devicetype.json` 的官方字段全集**（`required` 只有 5 个：`manufacturer` / `model` / `slug` / `u_height` / `is_full_depth`）：
`manufacturer` · `model` · `slug` · `part_number` · `description` · `comments` · `u_height` · `is_full_depth` · `airflow` · `weight` · `weight_unit` · `front_image` · `rear_image` · `subdevice_role` · `is_powered` · `console-ports[]` · `console-server-ports[]` · `power-ports[]` · `power-outlets[]` · `interfaces[]` · `front-ports[]` · `rear-ports[]` · `port-mappings[]` · `module-bays[]` · `device-bays[]` · `inventory-items[]`
→ 同目录另有 `moduletype.json` / `racktype.json` / `reusable.json` / `components.json` / `generated_schema.json`，**这套"JSON Schema 分文件 + components 复用"的组织方式也值得照搬**（可直接用 `jsonschema` 做 CI 校验）。
→ **`interfaces[].type` 用的就是 NetBox `InterfaceTypeChoices` 的枚举**（`1000base-t` / `10gbase-x-sfpp` / `25gbase-x-sfp28` / `100gbase-x-qsfp28` 等）——**这套命名建议整套沿用**，因为它把"速率 + 封装 + 介质"三件事编码在一个字符串里，比自造 `SFP28-25G` 这类名字严谨。

**另外核实到的图资源规模**：`elevation-images/` 有 **2887** 个 `<slug>.front.{jpg,png}` / `.rear.*`（用于 NetBox 机柜正视图贴真机照片），`module-images/` 有 64 个。华为目录下确认存在 `huawei-ne8000-f1a-8h20q.front.png` 等。**如果做 2D 机柜正视图，这批图是唯一能白拿的现成素材**（CC0）—— 但同样只有国外设备和少量华为型号有。

#### 7.1.2 所以"抄什么"的最终清单（修正后）

| 优先级 | 抄什么 | 抄法 |
|---|---|---|
| 🔴 P0 | **端口类型命名体系**（`InterfaceTypeChoices` + `CableTypeChoices`） | 整套照搬枚举值，避免自造词 |
| 🔴 P0 | **`CableType` 的多芯/极性建模**（Nautobot `mapping` JSONField + `polarity_method`） | 抄数据结构设计 |
| 🔴 P0 | **机柜 U 位渲染的双通道 API 形态**（SVG + JSON） | 抄接口设计 |
| 🟡 P1 | **端口唯一占用的数据库约束写法**（RackTables 双向 UNIQUE） | 抄 SQL 设计 |
| 🟡 P1 | **三面 × 每 U 状态的机柜空间模型**（RackTables `RackSpace`） | 抄建表设计 |
| 🟡 P1 | **YAML 设备型号的字段清单与组织方式** | 抄 schema，不指望数据 |
| 🟢 P2 | 华为那 103 个型号的 YAML 数据（CE/NE/S 系列可直接用） | 直接取（CC0） |
| ⚪ 不值得 | 国外安防/服务器厂商的 YAML 数据（占库 99%，与本案无关） | 跳过 |

### 7.2 值得抄的数据模型设计（按价值排序）

#### ① 端口唯一占用：用数据库唯一约束，别用应用层判断

三个项目用两种方式做了同一件事，**都值得抄**：

```sql
-- openDCIM: create.sql, fac_Ports
UNIQUE KEY ConnectedDevice (ConnectedDeviceID, ConnectedPort)

-- RackTables: wwwroot/inc/install.php, Link 表
PRIMARY KEY (porta, portb),
UNIQUE KEY porta (porta),
UNIQUE KEY portb (portb)
```

RackTables 的写法更彻底（**两个方向各一个 UNIQUE**，天然杜绝 A→B 和 B→A 重复登记）。→ **自研时 `cable` 表的 (port_a_id) 和 (port_b_id) 各建 UNIQUE 索引。**

#### ② 机柜空间：三面 × 每 U 状态

RackTables `RackSpace` 表（`install.php`）：
```sql
RackSpace(rack_id, unit_no, atom enum('front','interior','rear'),
          state enum('A','U','T'), object_id)
PRIMARY KEY (rack_id, unit_no, atom)
```
- `atom`：**front / interior / rear 三面** —— 解决了"同一 U 位前后都能装设备"这个真实问题（用户 Excel 的 `U位` 列是 `3-12U` 区间，没区分前后，但实际施工中全深度设备和半深度设备会冲突）
- `state`：**A=可占用 / U=已占用 / T=临时预留**

→ **比 NetBox 的 `Device.position + DeviceType.u_height` 更严谨**。**建议自研采用三面建模 + 每 U 状态**，这是 RackTables 最值钱的设计。

#### ③ 机柜平面坐标：两种方案，按阶段选

| 方案 | 出处 | 表示 | 优点 | 缺点 |
|---|---|---|---|---|
| **矩形坐标** | openDCIM `fac_Cabinet` 的 `MapX1, MapX2, MapY1, MapY2` | 四个整数像素/毫米值 | 自由摆放、支持任意尺寸机柜 | 需要前端做碰撞检测 |
| **网格 + 底图** | GLPI `glpi_dcrooms` 的 `vis_cols` / `vis_rows` / `vis_cell_width` / `vis_cell_height` / `column_labels` / `row_labels` / **`blueprint`**（底图图片）+ `glpi_racks.position varchar(50)` 存 `"x,y"`（`src/DCRoom.php:425-440` 用 `preg_match('/(\d+),\s?(\d+)/', ...)` 解析） | 行列格位 + 底图叠加 | 实现简单、不需要精确坐标、**用户 Excel 的"机房平面图"本来就是格子拼的** | 不支持非整数位置 |

→ **建议：一期用 GLPI 的网格+底图方案（快、贴合现有 Excel），二期若要精确排布再升级为 openDCIM 的矩形坐标。** 两个方案的数据结构可兼容（网格坐标本质是矩形坐标的特例）。

#### ④ 多芯/极性建模：抄 Nautobot 不抄 NetBox

| | NetBox | Nautobot v3 |
|---|---|---|
| 表达方式 | `CableProfileChoices` 枚举（`single-1c1p`、`trunk-2c4p-shuffle`…） | **`CableType` 一等实体**（`nautobot/dcim/models/cables.py:132`），含 `a_connectors`/`b_connectors`/`total_lanes`/`strands_per_lane`/`polarity_method`/`has_embedded_transceivers` + **`mapping` JSONField**（A→B lane 映射，注释：*"A→B lane mapping as a JSON array of objects with keys: label, a_connector, a_position, b_connector, b_position"*） |
| 极性 | 无独立概念（靠 profile 名里的 shuffle 暗示） | `polarity_method`：`straight-through` / `reversed` / `pair-reversed` / `other`（`nautobot/dcim/choices.py:1452`） |
| 现状拆分 | `CablePath` 按 termination 走 | **`CableLane` / `CableLaneSide` dataclass**（`cables.py:77` / `:90`） |
| 线长精度 | `DecimalField(8,2)` ✅ | `PositiveSmallInteger` ❌（存不了 10.08） |

→ **建议：多芯/极性抄 Nautobot 的 `CableType` + `mapping` JSON 设计；线长字段用 NetBox 的 `DecimalField(max_digits=8, decimal_places=2)`。**（因为用户 Excel 的柜间线缆是 `10.08` 这种两位小数，整数存不下。）

⚠️ 顺带记一个坑：**不要天真地以为"抄 Nautobot 的 cable 模型"就够了** —— Nautobot v3 的 `Cable.length` 是整数，如果直接照搬会把小数长度截断。

#### ⑤ 原子化变更 + 审计

RackTables 的 `Molecule` / `MountOperation` 表：把一次机柜变更抽象成"分子"，记录操作流水；配合 `ObjectHistory` / `ObjectLog`。
→ **建议自研时用"变更集（ChangeSet）+ 操作明细"两张表**，这样"重新规划一版机柜布局"可以整体回滚、可以对比两版差异。**这正好对应用户现在用 `V9/V11/V12/V13` 多个 Excel 文件做版本管理的痛点。**

#### ⑥ Excel 布线表导出的参考实现

openDCIM `connections_spreadsheet.php`（PhpSpreadsheet）：
- 列头（A1..G1 逐字）：`SourceDevice | SourcePort | TargetDevice | TargetPort | Notes | MediaType | Color`
- **每个 device 一个 worksheet**，worksheet 内还有 Device Label / Manufacturer / Model / Serial Number / Asset Tag / Target Cabinet / Position

→ **可借鉴的是"按设备分 sheet"这个组织结构**（施工时按设备逐个核对很自然）。**但本项目应该改成"按通道/按楼层分 sheet + 一个总表 + 一个耗材汇总表"**，因为用户现有交付物是"布线表（全量）+ 耗材需求清单"的结构，且 openDCIM 的导出**没有长度列**（全库无 `CableLength` 概念），不够用。

### 7.3 与 NetBox 的互操作设计（如果要对接）

**方向一：单向导出（推荐先做这个）**

```
自研系统  ──pynetbox──▶  NetBox
  Site          → Site
  Room          → Location
  Rack(x,y)     → Rack（坐标丢失，NetBox 无此字段）
  DeviceType    → Manufacturer + DeviceType
  Device        → Device（rack + position）
  Port          → Interface
  Cable         → Cable（含 length！Decimal(8,2) + length_unit）
                 派生列（跨机柜/间隔机柜/…）→ 无处可放，只能塞进 Cable.description 或丢弃
```

- 客户端：`pynetbox`（★687，Apache-2.0，2026-10-09）—— 许可证干净、成熟
- 关键映射：`Cable.length` 是 `DecimalField(max_digits=8, decimal_places=2)`，**能装下 `10.08`**；但 `length_unit` 只有 `km/m/cm/mi/ft/in` 六种，我们的"档位"（15/20 米这种采购档位）**没有对应单位，只能塞进 `label`（`CharField(max_length=100)`）或 `description`**
- **诚实结论：导出是"有损"的**。这一点必须在产品文档里写清楚，不能让客户以为导出到 NetBox 后信息完整。

**方向二：批量导入（如果客户已有 NetBox 数据）**

- 通道：NetBox 的 REST API（`pynetbox` 批量创建）比 CSV 导入更适合程序化，CSV 导入适合人工。
- 注意：CSV 导入 Cable 的列（`netbox/dcim/forms/bulk_import.py:1866`）是 `side_a_device` + `side_a_name` 这种"名称引用"，需要目标库已有同名设备/接口。

### 7.4 明确不建议借鉴的东西

| 不要抄 | 原因 |
|---|---|
| `netbox-branching` 的分支模型 | **NetBox Limited Use License 1.0，非 OSI**，仅限自有内部使用、不得分发、不得作为商业产品一部分提供。**连抄思路都要谨慎**（法律上抄思路通常可以，但代码绝对不能碰） |
| NetBox 的 `CablePath` 算法 | 它是"沿 termination 走通一条链"，用于 tracing，**不是几何路径**。本项目的路径是空间路径（经过哪些桥架），语义不同，硬套会变形 |
| openDCIM 的代码 | PHP 过程式代码、三年未发 release、无 Composer 依赖管理（vendor 直接 committed）。**只抄表结构思想，不抄代码** |
| GLPI 的 `glpi_cables` 表 | 只有 `itemtype_endpoint_a/b` + `cables_id` + `color` + `cabletypes_id`，`glpi_cabletypes` 只有 `id/name/comment` —— **表达不了芯数/极性/长度**，比 NetBox 还简 |
| iTop / TeemIP 的 AGPL 代码 | AGPL-3.0 传染性最强，私有化交付场景风险高 |

---

## 8. 来源列表

> 所有 GitHub 数据于 **2026-10-20** 通过 GitHub REST API（`GET https://api.github.com/repos/{owner}/{repo}`）实时核实。许可证除 API 的 `license.spdx_id` 外，关键候选均下载 `LICENSE` / `license.txt` / `COPYING` 原文读首行确认版本。

### 8.1 核心平台

| 项目 | URL | 核实项 |
|---|---|---|
| NetBox | https://github.com/netbox-community/netbox | ★21734 / Apache-2.0 / pushed 2026-10-20 / release v4.7.3 (2026-10-13) |
| Nautobot | https://github.com/nautobot/nautobot | ★1628 / Apache-2.0（LICENSE.txt）/ pushed 2026-10-20 / release v3.2.7 (2026-10-13) |
| openDCIM | https://github.com/openDCIM/openDCIM | ★366 / **无 LICENSE 文件，README 声明 GPLv3** / pushed 2026-08-14 / 最新 release 23.04 (2023-10-15) |
| RackTables | https://github.com/RackTables/racktables | ★812 / GPL-2.0（原文 `Version 2, June 1991`）/ pushed 2026-06-26 / 无 release，最新 tag `RackTables-0.22.0` |
| phpIPAM | https://github.com/phpipam/phpipam | ★2816 / **无 LICENSE 文件，README 声明 GPLv3**（`misc/gpl-3.0.txt` 已核）/ pushed 2026-10-05 / release v1.8.3 (2026-09-09) |
| iTop | https://github.com/Combodo/iTop | ★1204 / AGPL-3.0（`license.txt` 原文 `Version 3, 19 November 2007`）/ pushed 2026-10-20 / release 3.3.0 (2026-09-15) |
| GLPI | https://github.com/glpi-project/glpi | ★6475 / GPL-3.0（`LICENSE` 原文 `Version 3, 29 June 2007`）/ pushed 2026-10-20 / release 12.0.0 (2026-10-07) |
| OCS Inventory | https://github.com/OCSInventory-NG/OCSInventory-Server | ★405 / GPL-2.0 / pushed 2026-08-25 |
| TeemIP Core | https://github.com/teemIP/teemip-core-ip-mgmt | ★32 / AGPL-3.0 / pushed 2026-10-06 |
| TeemIP Cable Mgmt | https://github.com/teemIP/teemip-cable-mgmt | ★7 / GPL-3.0 / pushed 2026-06-25 |
| Infrahub | https://github.com/opsmill/infrahub | ★538 / Apache-2.0（LICENSE.txt）/ pushed 2026-10-20 |
| Ralph | https://github.com/allegro/ralph | ★2529 / Apache-2.0 / pushed 2026-10-19 |
| Netdisco | https://github.com/netdisco/netdisco | ★930 / BSD-3-Clause / pushed 2026-10-20 |
| NIPAP | https://github.com/SpriteLink/NIPAP | ★593 / MIT / pushed 2026-10-13 |
| RackPad | https://github.com/Kobii-git/rackpad | ★417 / MIT / pushed 2026-10-20 |
| NAPALM | https://github.com/napalm-automation/napalm | ★2515 / Apache-2.0 / pushed 2026-08-12 |

### 8.2 NetBox 生态

| 项目 | URL | 核实项 |
|---|---|---|
| devicetype-library | https://github.com/netbox-community/devicetype-library | ★1631 / **CC0-1.0** / pushed 2026-10-19 / 12390 device-types + 2036 module-types + 147 rack-types |
| netbox-docker | https://github.com/netbox-community/netbox-docker | ★2793 / Apache-2.0 / pushed 2026-10-14 |
| pynetbox | https://github.com/netbox-community/pynetbox | ★687 / Apache-2.0 / pushed 2026-10-09 |
| go-netbox | https://github.com/netbox-community/go-netbox | ★225 / **NOASSERTION（无 LICENSE 文件）— 待核实** / pushed 2025-05-09 |
| ansible_modules | https://github.com/netbox-community/ansible_modules | ★402 / GPL-3.0 / pushed 2026-10-13 |
| netbox-topology-views | https://github.com/netbox-community/netbox-topology-views | ★1103 / Apache-2.0 / pushed 2026-10-19 / 兼容矩阵 4.7.X↔v5.0.0 |
| netbox-reorder-rack | https://github.com/netbox-community/netbox-reorder-rack | ★112 / Apache-2.0 / pushed 2026-09-10 / 1.1.5↔4.7.x |
| netbox-qrcode | https://github.com/netbox-community/netbox-qrcode | ★271 / Apache-2.0 / pushed 2026-09-03 |
| netbox-napalm-plugin | https://github.com/netbox-community/netbox-napalm-plugin | ★74 / Apache-2.0 / pushed 2026-09-30 |
| nextbox-ui-plugin | https://github.com/iDebugAll/nextbox-ui-plugin | ★642 / MIT / **pushed 2025-05-24（约 17 个月未更新）** |
| netbox-branching | https://github.com/netboxlabs/netbox-branching | ★157 / **NetBox Limited Use License 1.0（非 OSI）** / pushed 2026-10-19 |
| diode | https://github.com/netboxlabs/diode | ★167 / **NetBox Limited Use License 1.0（非 OSI）** / pushed 2026-09-25 |

### 8.3 专用插件（线长 / 平面图 / 3D）

| 项目 | URL | 核实项 |
|---|---|---|
| Netbox_Cable_Calculator_Plugin | https://github.com/gadler01/Netbox_Cable_Calculator_Plugin | **★2** / MIT / pushed 2026-10-05 / **全场唯一做跨机柜线长计算的** |
| netbox-pathways | https://github.com/jsenecal/netbox-pathways | ★21 / AGPL-3.0 / pushed 2026-10-15 / 需 PostGIS / 自称 alpha |
| netbox-fms | https://github.com/jsenecal/netbox-fms | ★25 / AGPL-3.0 / pushed 2026-10-19 |
| netbox-cable-labels | https://github.com/jsenecal/netbox-cable-labels | ★48 / Apache-2.0（原文已核，API 报 NOASSERTION）/ pushed 2026-01-13 |
| net3d | https://github.com/hervehildenbrand/net3d | ★25 / MIT / pushed 2026-09-29 / TypeScript |
| netbox_floorplan | https://github.com/tbotnz/netbox_floorplan | ★80 / LGPL-3.0 / pushed 2024-03-06 / **archived = true** |
| netbox-plugin-device-map | https://github.com/drygdryg/netbox-plugin-device-map | ★46 / MIT / pushed 2024-08-06 |
| netbox-rack-design | https://github.com/ravenrs/netbox-rack-design | ★14 / Apache-2.0 / pushed 2026-10-20 |
| netbox-spatial-lens | https://github.com/antoinekh/netbox-spatial-lens | ★5 / MIT / pushed 2026-10-08 |

### 8.4 Nautobot App 生态

| 项目 | URL | 核实项 |
|---|---|---|
| nautobot-app-device-onboarding | https://github.com/nautobot/nautobot-app-device-onboarding | ★59 / Apache-2.0 / pushed 2026-10-14 |
| nautobot-app-nornir | https://github.com/nautobot/nautobot-app-nornir | ★28 / Apache-2.0 / pushed 2026-10-20 |
| nautobot-app-design-builder | https://github.com/nautobot/nautobot-app-design-builder | ★15 / Apache-2.0 / pushed 2026-10-14 |
| nautobot-app-floor-plan | https://github.com/nautobot/nautobot-app-floor-plan | ★15 / Apache-2.0 / pushed 2026-10-14 |
| nornir-nautobot | https://github.com/nautobot/nornir-nautobot | ★39 / Apache-2.0 |

### 8.5 iTop 扩展

| 项目 | URL | 核实项 |
|---|---|---|
| itop-datacenter-view | https://github.com/Molkobain/itop-datacenter-view | ★31 / **无 license 字段 = 未声明许可，商用需确认** |

### 8.6 源码级核实的具体文件（便于复核）

| 结论 | 文件:行 |
|---|---|
| NetBox `Cable` 模型（含 `length`） | `netbox/dcim/models/cables.py:95` |
| NetBox `CableBundle` | `netbox/dcim/models/cables.py:51` |
| NetBox `Cable.clean()` 校验长度单位 | `netbox/dcim/models/cables.py:302`（`:307` 抛 `"Must specify a unit when setting a cable length"`） |
| NetBox `CablePath.get_total_length()` | `netbox/dcim/models/cables.py:1301`（`:1329-1345` 累加逻辑） |
| NetBox `CableLengthUnitChoices` | `netbox/dcim/choices.py:2049` |
| NetBox `CableProfileChoices` | `netbox/dcim/choices.py:1830` |
| NetBox `CableTypeChoices` | `netbox/dcim/choices.py:1906` |
| NetBox `RackBase` / `RackType` / `Rack` / `RackReservation` | `netbox/dcim/models/racks.py:58` / `:167` / `:286` / `:722` |
| NetBox Rack U 位方法 | `netbox/dcim/models/racks.py:510` `get_rack_units` / `:573` `get_available_units` / `:613` `get_reserved_units` / `:623` `get_elevation_svg` |
| NetBox rack elevation API | `netbox/dcim/api/views.py:206`（`operation_id='dcim_racks_elevation_retrieve'`） |
| NetBox `Device` 模型 | `netbox/dcim/models/devices.py:531`（`position = DecimalField(max_digits=4, decimal_places=1)`） |
| NetBox `Site`（含经纬度） | `netbox/dcim/models/sites.py:158` |
| NetBox Cable CSV 导入列 | `netbox/dcim/forms/bulk_import.py:1866` |
| NetBox 插件注册函数 | `netbox/netbox/plugins/registration.py`（262 行，`__all__` 列 9 个 `register_*`，见 `:16-26`） |
| NetBox 插件 API 边界警告 | `docs/plugins/development/index.md` |
| NetBox GraphQL 只读声明 | `docs/features/api-integration.md` |
| Nautobot `CableType` 模型 | `nautobot/dcim/models/cables.py:132`（`mapping` JSONField 在 `:175`） |
| Nautobot `Cable.length`（整数） | `nautobot/dcim/models/cables.py:495` |
| Nautobot `CableLane` / `CableLaneSide` | `nautobot/dcim/models/cables.py:77` / `:90` |
| Nautobot `CableTypePolarityMethodChoices` | `nautobot/dcim/choices.py:1452` |
| Nautobot Rack 正视图 | `nautobot/dcim/svg/rack_elevation.py:11`（`render` 在 `:213`，`merge_elevations` 在 `:195`） |
| openDCIM 表结构 | `create.sql`（1171 行）；`fac_PanelSchedule` 在 `:502`，`fac_PowerPanel` 在 `:646` |
| openDCIM Excel 布线表导出 | `connections_spreadsheet.php` |
| RackTables 表结构 | `wwwroot/inc/install.php` |
| phpIPAM 表结构 | `db/SCHEMA.sql`（1106 行 / 48 表）；`devices` 在 `:320`，`racks` 在 `:799`，`rackContents` 在 `:820` |
| GLPI DCIM 表结构 | `install/mysql/glpi-empty.sql`；`glpi_dcrooms` 在 `:8527`，`glpi_racks` 在 `:8589`，`glpi_sockets` 在 `:4623`，`glpi_cables` 在 `:4651`，`glpi_pdus` 在 `:8773`，`glpi_datacenters` 在 `:8506` |
| GLPI 机房网格坐标解析 | `src/DCRoom.php:425-440`（`preg_match('/(\d+),\s?(\d+)/')`）、`:445` `getAllPositions()` |

### 8.7 标为"待核实"的项目（本文未完成验证）

| 项 | 状态 |
|---|---|
| `go-netbox` 的许可证 | GitHub API 报 `NOASSERTION`，仓库根无 `LICENSE` 文件。**待核实**（可能是 Apache-2.0 但未放标准文件） |
| `Molkobain/itop-datacenter-view` 的许可证 | 仓库无 `license` 字段。**未声明**——商用前必须联系作者确认 |
| 国内厂商设备（华为/华三/锐捷等）在 `devicetype-library` 中的覆盖率 | ✅ **已实测，不再是待核实项**。结果：国内网络厂商合计 120/12390 = **0.97%**；华为 103、中兴 8、瑞斯康达 4、锐捷 3、浪潮 2；**H3C / 曙光 / 烽火 / 迈普 / 绿盟 / 启明星辰 / 深信服 / 天融信 / 迪普 / 山石 / 奇安信 全部为 0**。见 §7.1.1 |
| `gestionip/gestioip` 官方仓库 | 未找到（原 SourceForge 项目）。**待核实** |
| NetBox 4.7 的 `Rack` 是否通过某个插件机制（如 `RackType`）间接支持坐标 | 已 grep `racks.py` 确认无坐标字段，但**未逐一排查 NetBox 官方插件**。结论风险低 |

---

## 9. 附：本文结论对下游文档的输入

| 下游文档 | 本文提供的输入 |
|---|---|
| [02-visualization-stack.md](./02-visualization-stack.md) | NetBox 的 SVG+JSON 双通道 elevation API 设计可照抄；3D 生态全是小项目，须自研 |
| [03-cabling-algorithms.md](./03-cabling-algorithms.md) | 生态中唯一同类实现是 ★2 插件（英寸参数 + 统一 overhead + `default_slack_pct`），**无可复用的成熟算法**；RackTables 三面 RackSpace、openDCIM 端口唯一约束可作数据基础 |
| [04-commercial-products.md](./04-commercial-products.md) | 开源侧结论：线长计算是空白地带，商用产品是否覆盖需在 04 中确认 |
| [需求规格说明书](../requirements/01-需求规格说明书.md) | 用户 Excel 25 列中 6 列派生字段是核心价值；`线缆长度1` 与 `线缆长度` 取值重复可合并 |
