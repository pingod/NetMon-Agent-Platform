"""设备型号的端口模板 —— 「这台设备一共几个口」的唯一来源。

## 为什么必须有这个文件

端口台账要回答两个问题：「用了几个口」和「还剩几个口」。
第一个好算（数一数就知道了），第二个没有分母就算不出来。

而源数据里**只有实际用到的端口名，没有「标称一共几口」**。
`首期设备清单` 的「配置」列是唯一线索，形如：

```
48*25GE SFP28,8*100GE QSFP28                 ← 业务接入交换机1
48个10/100/1000BASE-T,4个万兆SFP+,2个堆叠口    ← 带内交换机1
36 端口 100GE 板 * 3 / 48 端口 25GE 板 * 1     ← 业务核心交换机（多行）
2个25Gbps网口和2个1Gbps网口                    ← 管理节点
```

这些文本的写法**每一条都不一样**，还夹杂中文、换行、全角空格。
所以本文件不做自动解析，而是把 28 个型号的模板**人工核定后硬编码**，
并在注释里标出处（哪个型号来自哪段文本）。自动解析的结果留作核对依据。

## 关键实测发现：同一物理槽位会以不同速率前缀出现

这一点如果不建模，「还剩几个口」必定算错：

```
业务接入交换机-1 的观测端口
  10GE1/0/1 … 10GE1/0/48      ← 47 个
  25GE1/0/1 … 25GE1/0/48      ← 27 个
  100GE1/0/1 … 100GE1/0/8     ← 8 个
```

`10GE1/0/5` 与 `25GE1/0/5` 是**同一个物理口**（同一槽位，协商速率不同），
不是两个口。把端口名原样当身份，会把 48 口设备算成 55 口、利用率算成 150% ——
这种"看起来算出来了"的假数字比不显示更糟。

所以端口身份 = `(组名, 面板号, 组内序号)`，组名由**模板**决定而不是由速率前缀决定：
`10GE1/0/5` 与 `25GE1/0/5` 都归到 `25GE` 组的第 5 号口（该组的 `speeds` 允许两种速率）。

面板号取端口名里的 chassis 段：核心交换机 `100GE1/0/x`、`100GE2/0/x`、`100GE3/0/x`
是三块 36 口板，共 108 个百 G 口。

## 覆盖率是诚实的

模板只覆盖能在 `首期设备清单` 里找到配置文本的型号。实测 28 个型号里
25 个有模板。没有模板的型号，`utilization()` 返回 `None` 而不是猜一个数
——「不知道」比「猜一个」对现场更有用。

## 本文件现在是「内置方言」，不是唯一来源（v0.7）

`SPECS` 里这 26 条是**内置默认值**（信息港项目的实测定稿）。用户另有两类
数据源，优先级更高，统一由 `domain/ports/modellib.py` 管理：

- 项目内 JSON 库（`projects/<名>/models.json`）
- Excel 型号库（`型号库` sheet，两段：型号表 + 端口组表）

内置表比 Excel 多两样东西：逐条 `source_text` 出处在场、`_ALIASES` 别名。
所以 `load_model_library()` **总是把内置表作为底座**，
项目库与 Excel 只在上面覆盖/追加 —— 这样即便用户的库里只有 3 个自研型号，
另外 24 个内置型号仍然可用，不会因为「换了库」而突然全变未知端口。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Final, Literal

from ..importing.normalize import norm_name
from .naming import PortName, parse_port_name

PortRole = Literal["downlink", "uplink", "peer_link", "stack", "mgmt", "spare", "unknown"]


@dataclass(frozen=True, slots=True)
class PortBay:
    """模板里的一组端口 —— 一个物理面板上规格相同的一片口。

    ## 为什么 `count` 与 `speeds` 要分开

    `48*25GE SFP28` 这组口实测既有 `25GE1/0/5`（25G）也有 `10GE1/0/5`（10G，
    降速跑）。所以型号标称 25GE、实际可能是 10G —— `speeds` 列出允许的速率集合，
    `nominal_mbps` 是标称值（用于显示"48 × 25GE"）。

    ## 为什么 `indices` 不能简单写成 `1..count`

    实测**端口序号不从 1 开始、也不连续**，三条证据：

    ```
    带内交换机1    10GE1/0/3,4,5,6     ← 4 个万兆口，编号 3~6（1、2 不存在）
    计算节点B      千兆1, 千兆3         ← 配置说「2 个 GE 电口」，编号 1 和 3
    网络节点交换机   10GE1/0/1..9       ← 48 个万兆口，实际只用了前 9 个
    ```

    若强行按 `1..count` 生成，`10GE1/0/6` 会归不进 4 口组（越界），
    利用率就只能算成"未知"。真实网络设备的端口编号本来就是厂家按面板布局定的，
    所以这里直接记死**实际存在的序号集合**。
    """

    group: str
    """组名，与端口名解析后的标签对齐：`100GE` / `25GE` / `10GE` / `GE` / `万兆` / `千兆` / `IPMI`。"""

    indices: tuple[int, ...]
    """这组口**实际存在的序号**。长度就是口数。用 `PortBay.of(...)` 造 1..n。"""

    nominal_mbps: int
    """标称速率（Mbps）。"""

    speeds: tuple[int, ...]
    """允许出现的速率集合。匹配观测端口时用它，不用 `nominal_mbps`。

    网络节点交换机是典型：配置写「万兆 SFP+ 口 48 个」，实测这些口
    既跑 `10GE1/0/x` 也跑 `25GE1/0/x`（同一个口，协商成两种速率）。
    """

    role: PortRole = "downlink"
    """端口角色。实测证据：`100GE1/0/1..4` 全是上行到核心（每台 4 个），
    `10GE1/0/48` 是堆叠口（26 次），下行口是 `25GE1/0/1..44`。"""

    label: str = ""
    """给界面显示的文字，如 `48 × 25GE`。空则按 `count × nominal` 生成。"""

    panel: int = 0
    """面板号。`0` = 未指定，按「同组名出现的先后」自动编号（内置表的写法）。

    ## 为什么型号库必须有这个字段（v0.7）

    内置表靠**组名重复出现**来判断面板：核心交换机的 bays 是
    `100GE/uplink`、`100GE/uplink`、`100GE/peer_link`、`25GE/downlink`，
    `slots()` 里数第几个 `100GE` 就给它编号 1、2、3。

    这套约定**在型号库里无法表达**：型号库按 `(组名, 角色)` 分行存端口，
    两块 `100GE/uplink` 板会合并成一组 36 口（而不是两组各 36 口），
    实测往返一次 `业务核心交换机` 的总口数从 156 变成 108 —— 少 48 个口，
    而整体统计只差 3%，界面上完全看不出来。

    所以型号库导出时写死 `panel`，导入时用它；`0` 才回退到旧行为。
    默认 `0` 保证内置表 26 条一字不用改。
    """

    @staticmethod
    def of(
        group: str,
        count: int,
        nominal_mbps: int,
        speeds: tuple[int, ...],
        role: PortRole = "downlink",
        label: str = "",
    ) -> PortBay:
        """造一个序号为 1..count 的组。绝大多数型号都用它。"""
        return PortBay(group, tuple(range(1, count + 1)), nominal_mbps, speeds, role, label)

    @property
    def count(self) -> int:
        return len(self.indices)

    def describe(self) -> str:
        if self.label:
            return self.label
        rate = self.nominal_mbps // 1000
        return f"{self.count} × {rate}G"

    def matches(self, pn: PortName) -> bool:
        """这个观测端口是否落在本组。

        判据只有速率。原因见模块 docstring：
        `10GE1/0/5` 与 `25GE1/0/5` 是同一个物理口。
        """
        return pn.rate_mbps in self.speeds

    def pairs(self) -> tuple[tuple[int, int], ...]:
        """把 `indices` + `speeds` 展开成 `[(序号, 速率), ...]`。

        ## 为什么要这一层

        `indices` 是「哪些序号存在」，`speeds` 是「这些口允许跑哪些速率」，
        两轴交叉最多 `len(indices) × len(speeds)` 种组合。但写进
        型号库 JSON / Excel 时不可能写笛卡尔积，只能写两列。

        展开时**每个序号配标称速率为主**，额外速率按「同序号、按 speeds 顺序」
        补在后面 —— 这样导出的库再读回来，`indices` 与 `speeds` 都能还原。
        """
        out: list[tuple[int, int]] = []
        extra = tuple(sp for sp in self.speeds if sp != self.nominal_mbps)
        for idx in self.indices:
            out.append((idx, self.nominal_mbps))
            for sp in extra:
                out.append((idx, sp))
        return tuple(out)


@dataclass(frozen=True, slots=True)
class PortSpec:
    """一个型号的完整端口模板。"""

    model: str
    """型号名（已归一：去空格）。"""

    bays: tuple[PortBay, ...]

    category: str = "other"
    """`switch` / `server` / `security` / `storage` / `core` / `other`，与
    `exporting/svg.py:categorize()` 同口径，供台账按类统计。"""

    source_text: str = ""
    """模板出自 `首期设备清单` 的哪段配置文本（截断）。只用于核对，
    不参与计算 —— 三个月后有人质疑「这 48 口哪来的」，得能指回原文。"""

    @property
    def total_ports(self) -> int:
        """标称总口数。"""
        return sum(b.count for b in self.bays)

    def group_of(self, pn: PortName) -> PortBay | None:
        """把一个观测端口归到组。归不上返回 None（说明模板与数据不符）。

        ## 同速率多组时怎么选

        核心交换机有 3 组都是 100GE 的板（各 36 口）。此时光看速率分不出是哪块板，
        要看序号是否落在该组的 `indices` 里 —— `100GE3/0/35` 的 35 号只在第 3 组。
        板卡号（chassis）另行由 `panel_of()` 提供，不参与组选择。
        """
        if pn.style == "cli_board":
            for bay in self.bays:
                if any(bay.group.endswith(s) for s in ("GE",)) and bay.matches(pn):
                    if pn.port in bay.indices:
                        return bay
                    if len(self.bays) == 1 or not any(
                        b.matches(pn) and pn.port in b.indices for b in self.bays
                    ):
                        # 序号越界但速率吻合：仍归这一组，由 bay_index 判越界
                        return bay
            return None
        # 标签式：`千兆1` / `万兆2` / `25G2` / `IPMI`
        if pn.is_mgmt and not pn.label:
            for bay in self.bays:
                if bay.group == "IPMI":
                    return bay
            return None
        if pn.label:
            for bay in self.bays:
                if bay.group == pn.label:
                    return bay
            # 标签与组名不同但速率一致（如 `万兆` vs 组名 `25G`）
            for bay in self.bays:
                if bay.matches(pn) and not bay.group.endswith("GE"):
                    return bay
        return None

    def bay_index(self, pn: PortName) -> int | None:
        """观测端口在组内的序号。不在该组 `indices` 里返回 None。"""
        bay = self.group_of(pn)
        if bay is None:
            return None
        if pn.style == "cli_board":
            idx = pn.port
        elif pn.is_mgmt and not pn.label:
            idx = pn.index or 1
        else:
            idx = pn.index
        if idx is None or idx not in bay.indices:
            return None
        return idx

    def panel_of(self, pn: PortName) -> int:
        """面板号。CLI 式取 chassis 段，其余看模板有没有显式 `panel`，最后回退 1。

        核心交换机 `100GE2/0/1` 是第 2 块 36 口板 —— 不区分面板的话，
        `100GE1/0/1`、`100GE2/0/1`、`100GE3/0/1` 会被算成同一个口。

        型号库里的服务器类端口名不带 chassis（`万兆1`），面板号只能来自模板；
        内置表全是 1，所以显式值几乎只在「同一型号有多块同规格板卡」时出现。
        """
        if pn.style == "cli_board" and pn.chassis is not None:
            return pn.chassis
        bay = self.group_of(pn)
        if bay is not None and bay.panel:
            return bay.panel
        return 1

    def bay_key(self, raw: object) -> tuple[str, int, int] | None:
        """观测端口 → 全局唯一的端口身份 `(组名, 面板号, 组内序号)`。

        这是端口台账的主键。返回 None 表示这个端口名不在模板里
        （模板漏了、或端口名格式变了）。
        """
        pn = parse_port_name(raw)
        bay = self.group_of(pn)
        if bay is None:
            return None
        idx = self.bay_index(pn)
        if idx is None:
            return None
        return (bay.group, self.panel_of(pn), idx)

    def slots(self) -> tuple[tuple[str, int, int], ...]:
        """模板展开后的全部端口身份，按顺序。用于算「还剩哪些口空着」。"""
        out: list[tuple[str, int, int]] = []
        seen_panels: dict[str, int] = {}
        for bay in self.bays:
            # 显式 panel 优先（型号库写法）；否则按「同组名出现的先后」编号
            if bay.panel:
                panel = bay.panel
                seen_panels[bay.group] = max(seen_panels.get(bay.group, 0), panel)
            else:
                # 同组多面板（核心交换机的 3 块 100GE 板）靠连续出现来编号
                panel = 1
                if bay.group.endswith("GE"):
                    panel = seen_panels.get(bay.group, 0) + 1
                seen_panels[bay.group] = panel
            for i in bay.indices:
                out.append((bay.group, panel, i))
        return tuple(out)


# ---------------------------------------------------------------------------
# 模板表
# ---------------------------------------------------------------------------
#: 28 个型号的端口模板。来源全部是 V13「首期设备清单」的「配置」列原文，
#: 逐条核对过。没有配置文本的型号（`对象存储数据节点服务器`）按观测端口反推。
#:
#: ## 维护约定
#:
#: 新增型号时**必须**填 `source_text`。没有原文依据的模板宁可留空 ——
#: 一个错的分母会让所有设备的利用率一起错，而且错得很像对的。
_MODELS: Final[tuple[PortSpec, ...]] = (
    # ---- 核心交换机：3 块 36 口 100GE 板 + 1 块 48 口 25GE 板 ----
    PortSpec(
        model="业务核心交换机",
        category="core",
        source_text="36 端口 100GE 以太网光接口板 * 3 / 48 端口 25GE 以太网光接口板* 1",
        bays=(
            PortBay.of("100GE", 36, 100_000, (100_000,), "uplink"),
            PortBay.of("100GE", 36, 100_000, (100_000,), "uplink"),
            PortBay.of("100GE", 36, 100_000, (100_000,), "peer_link"),
            PortBay.of("25GE", 48, 25_000, (25_000, 10_000), "downlink"),
        ),
    ),
    PortSpec(
        model="存储核心交换机",
        category="core",
        source_text="36 端口 100GE 以太网光接口板 * 3 / 48 端口 25GE 以太网光接口板* 1",
        bays=(
            PortBay.of("100GE", 36, 100_000, (100_000,), "uplink"),
            PortBay.of("100GE", 36, 100_000, (100_000,), "uplink"),
            PortBay.of("100GE", 36, 100_000, (100_000,), "peer_link"),
            PortBay.of("25GE", 48, 25_000, (25_000, 10_000), "downlink"),
        ),
    ),
    # ---- 25G 接入：48 口下行 + 8 口 100G 上行 ----
    PortSpec(
        model="管理核心交换机",
        category="core",
        source_text="48*25GE SFP28,8*100GE QSFP28",
        bays=(
            PortBay.of("25GE", 48, 25_000, (25_000, 10_000), "downlink"),
            PortBay.of("100GE", 8, 100_000, (100_000,), "uplink"),
        ),
    ),
    PortSpec(
        model="分线接入交换机",
        category="switch",
        source_text="48*25GE SFP28,8*100GE QSFP28",
        bays=(
            PortBay.of("25GE", 48, 25_000, (25_000, 10_000), "downlink"),
            PortBay.of("100GE", 8, 100_000, (100_000,), "uplink"),
        ),
    ),
    PortSpec(
        model="业务接入交换机1",
        category="switch",
        source_text="48*25GE SFP28,8*100GE QSFP28",
        bays=(
            PortBay.of("25GE", 48, 25_000, (25_000, 10_000), "downlink"),
            PortBay.of("100GE", 8, 100_000, (100_000,), "uplink"),
        ),
    ),
    PortSpec(
        model="业务接入交换机2",
        category="switch",
        source_text="48*25GE SFP28,8*100GE QSFP28，支持堆叠功能",
        bays=(
            PortBay.of("25GE", 48, 25_000, (25_000, 10_000), "downlink"),
            PortBay.of("100GE", 8, 100_000, (100_000,), "stack"),
        ),
    ),
    PortSpec(
        model="存储接入交换机1",
        category="switch",
        source_text="48*25GE SFP28,8*100GE QSFP28",
        bays=(
            PortBay.of("25GE", 48, 25_000, (25_000, 10_000), "downlink"),
            PortBay.of("100GE", 8, 100_000, (100_000,), "uplink"),
        ),
    ),
    # ---- 千兆接入：48 口电口 + 4 口万兆 + 2 堆叠 ----
    PortSpec(
        model="带内交换机1",
        category="switch",
        source_text="48个10/100/1000BASE-T以太网端口，4个万兆SFP+，2个专用堆叠口",
        bays=(
            PortBay.of("GE", 48, 1_000, (1_000,), "downlink"),
            # 「4 个万兆 SFP+ + 2 个专用堆叠口」= 同一种 SFP+ 口位、共 6 个（编号 1~6）。
            # 实测不同角色的交换机占用情况不同：
            #   计算带内 用 1,2,3,4（4 个万兆上行）
            #   存储带内 用 1,2 + 5,6（2 个万兆 + 2 个堆叠）
            #   带内-1/-2 用 3,4（这 2 台只用到中间两个口）
            # 所以合成一组 6 口，而不是拆成「4 口组 + 2 口组」——
            # 拆开会让 5,6 号口在"4 口组"里越界、变成"未知端口"。
            PortBay.of("10GE", 6, 10_000, (10_000,), "uplink", label="4 × 万兆 + 2 × 堆叠"),
        ),
    ),
    PortSpec(
        model="带外交换机1",
        category="switch",
        source_text="48个10/100/1000BASE-T以太网端口，4个万兆SFP+，2个专用堆叠口",
        bays=(
            PortBay.of("GE", 48, 1_000, (1_000,), "downlink"),
            # 「4 个万兆 SFP+ + 2 个专用堆叠口」= 同一种 SFP+ 口位、共 6 个（编号 1~6）。
            # 实测不同角色的交换机占用情况不同：
            #   计算带内 用 1,2,3,4（4 个万兆上行）
            #   存储带内 用 1,2 + 5,6（2 个万兆 + 2 个堆叠）
            #   带内-1/-2 用 3,4（这 2 台只用到中间两个口）
            # 所以合成一组 6 口，而不是拆成「4 口组 + 2 口组」——
            # 拆开会让 5,6 号口在"4 口组"里越界、变成"未知端口"。
            PortBay.of("10GE", 6, 10_000, (10_000,), "uplink", label="4 × 万兆 + 2 × 堆叠"),
        ),
    ),
    # ---- 网络节点交换机：48 口万兆 + 8 口 100G/40G ----
    PortSpec(
        model="网络节点交换机",
        category="switch",
        source_text="支持万兆SFP+口48个，100G/40G口8个",
        bays=(
            # 这 48 个 SFP+ 口实测既有 `10GE1/0/x` 也有 `25GE1/0/x`
            # （同一批口协商成两种速率），所以 speeds 两个都收。
            PortBay.of("10GE", 48, 10_000, (10_000, 25_000), "downlink"),
            PortBay.of("100GE", 8, 100_000, (100_000, 40_000), "uplink"),
        ),
    ),
    # ---- 计算节点：四口网卡 + 板载电口 + IPMI ----
    PortSpec(
        model="计算节点A（ARM）",
        category="server",
        source_text="四口25G网卡*1（华为SP680），千兆网卡 4个GE电口",
        bays=(
            # 注意：这 4 个口是 25G 网卡，但布线表里一律写成 `万兆1..4`
            # （沿用"万兆=网卡口"的旧习惯叫法）。组名必须用 `万兆` 才能对上，
            # 速率同时收 25G 与 10G —— 只写 25G 会让这 315 台设备的四个口全变「未知」。
            PortBay.of("万兆", 4, 25_000, (25_000, 10_000), "downlink", label="4 × 25G 网卡"),
            # 配置写「4 个 GE 电口」（GE1~GE4），布线表里只出现 千兆1/千兆3
            # —— 其余两个口这次规划没接线，不是不存在
            PortBay.of("千兆", 4, 1_000, (1_000,), "downlink", label="4 × 千兆电口"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    PortSpec(
        model="计算节点B（C86）",
        category="server",
        source_text="四口10G网卡*1（X710），千兆网卡 2个GE电口",
        bays=(
            PortBay.of("万兆", 4, 10_000, (10_000,), "downlink", label="4 × 10G 网卡"),
            # 配置写「2 个 GE 电口」，实测编号是 1 和 3（2 不存在）
            PortBay("千兆", (1, 3), 1_000, (1_000,), "downlink", label="2 × 千兆电口"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    # ---- 存储 / 业务服务器：2 × 25G + 2 × 千兆 + IPMI ----
    PortSpec(
        model="SSD云硬盘服务器",
        category="storage",
        source_text="2个25Gbps网口和2个1Gbps网口",
        bays=(
            PortBay.of("25G", 2, 25_000, (25_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    PortSpec(
        model="EHDD云硬盘服务器1",
        category="storage",
        source_text="2个25Gbps网口和2个1Gbps网口",
        bays=(
            PortBay.of("25G", 2, 25_000, (25_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    PortSpec(
        model="EHDD云硬盘服务器2",
        category="storage",
        source_text="2个25Gbps网口和2个1Gbps网口",
        bays=(
            PortBay.of("25G", 2, 25_000, (25_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    PortSpec(
        model="对象存储数据节点服务器",
        category="storage",
        source_text="（配置列无端口描述，按观测端口反推：25G1/25G2 + 千兆1/千兆2 + IPMI）",
        bays=(
            PortBay.of("25G", 2, 25_000, (25_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    PortSpec(
        model="管理节点服务器",
        category="server",
        source_text="2个25Gbps网口和2个1Gbps网口",
        bays=(
            PortBay.of("25G", 2, 25_000, (25_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    # ---- 4 × 25G + 2 × 千兆 + IPMI ----
    PortSpec(
        model="GSO独立部署服务器",
        category="server",
        source_text="4个25Gbps网口和2个1Gbps网口",
        bays=(
            PortBay.of("25G", 4, 25_000, (25_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    PortSpec(
        model="网络功能服务器",
        category="server",
        source_text="2个1Gbps网口 + 4个25Gbps网口，CX6网卡",
        bays=(
            PortBay.of("25G", 4, 25_000, (25_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    # ---- 备份一体机：4 × 10GE + 4 × 1GE ----
    PortSpec(
        model="本地备份-备份一体机",
        category="storage",
        source_text="网络接口：4*1GE 电口，4*10GE 光口",
        bays=(
            PortBay.of("万兆", 4, 10_000, (10_000,), "downlink"),
            PortBay.of("千兆", 4, 1_000, (1_000,), "downlink"),
            PortBay.of("IPMI", 1, 1_000, (1_000,), "mgmt"),
        ),
    ),
    # ---- 安全设备 ----
    PortSpec(
        model="防火墙1",
        category="security",
        source_text="（按观测端口反推：100G1/100G2 + 10G1/10G2 + 千兆1/千兆2）",
        bays=(
            PortBay.of("100G", 2, 100_000, (100_000,), "uplink"),
            PortBay.of("10G", 2, 10_000, (10_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
        ),
    ),
    PortSpec(
        model="防火墙2",
        category="security",
        source_text="（同防火墙1）",
        bays=(
            PortBay.of("100G", 2, 100_000, (100_000,), "uplink"),
            PortBay.of("10G", 2, 10_000, (10_000,), "downlink"),
            PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),
        ),
    ),
    PortSpec(
        model="广域网链路负载均衡",
        category="security",
        source_text="8 万兆光口 SFP+（含光模块）+8 千兆光口 SFP",
        bays=(
            PortBay.of("10G", 8, 10_000, (10_000,), "downlink", label="8 × 万兆光口"),
            PortBay.of("千兆", 8, 1_000, (1_000,), "downlink", label="8 × 千兆光口"),
            # 源表里这 2 台还有一个口写成 `GE8`（不符合同型号其余写法 `千兆N`）。
            # 解析器把它解成 label=`GE`, index=8，所以这里补一个 GE 组接住它。
            # 不合并进 `千兆` 组：组名不同（`GE` vs `千兆`），合并会让 8 号口与
            # 千兆1~8 撞号，利用率凭空多算一个口。
            PortBay("GE", (8,), 1_000, (1_000,), "unknown", label="GE8（源表孤立写法）"),
        ),
    ),
    # ---- 密码设备：2 个千兆电口 ----
    PortSpec(
        model="服务器密码机",
        category="security",
        source_text="网络接口：2个千兆电口",
        bays=(PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),),
    ),
    PortSpec(
        model="签名服务器",
        category="security",
        source_text="网络接口：2个千兆电口",
        bays=(PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),),
    ),
    PortSpec(
        model="数据库加密机",
        category="security",
        source_text="（按观测端口反推：千兆1/千兆2；源表该设备实例名为国密SSL-VPN-1/-2）",
        bays=(PortBay.of("千兆", 2, 1_000, (1_000,), "downlink"),),
    ),
)

#: 内置型号模板（只读，`modellib` 拿它当底座）
MODELS: Final[tuple[PortSpec, ...]] = _MODELS

#: 型号归一名 → 模板
SPECS: Final[dict[str, PortSpec]] = {norm_name(s.model): s for s in _MODELS}

#: 型号别名 —— 源表里同一台设备的型号名写法不一致时，映射到同一个模板。
#:
#: ## 为什么需要这一层
#:
#: `广域网负载均衡` 与 `广域网链路负载均衡` 是采购清单里的同一个产品
#: （实测 ch1 rack5 / rack23 的 U19-20U 位置同时出现这两个名字，
#: 是同一台 2U 设备的两种写法）。不给别名，前者会被算成「无模板」，
#: 台账上直接少一台设备的容量 —— 而这只影响 2 台，整体统计看不出异常。
#:
#: ## 为什么不做模糊匹配
#:
#: `业务接入交换机1` 与 `业务接入交换机2` 只差一个字，但端口规格
#: （100GE 口是上行还是堆叠）不同。模糊匹配会把它们混成一个，
#: 比不匹配更糟。所以别名必须逐条显式声明。
_ALIASES: Final[dict[str, str]] = {
    norm_name("广域网负载均衡"): norm_name("广域网链路负载均衡"),
}


def spec_of(model: object) -> PortSpec | None:
    """按型号名查模板。查不到返回 None —— 调用方必须处理「不知道」。

    >>> spec_of("业务接入交换机 1").total_ports
    56
    >>> spec_of("不存在的型号") is None
    True
    """
    if not model:
        return None
    key = norm_name(model)
    if key in _ALIASES:
        key = _ALIASES[key]
    return SPECS.get(key)


@dataclass(frozen=True, slots=True)
class PortUsage:
    """一台设备的端口使用情况。"""

    model: str
    used: int
    """实际用了几个口（去重后的端口身份数，不是布线条数）。"""

    total: int
    """标称口数。"""

    unknown_ports: tuple[str, ...] = ()
    """归不进模板的端口名。非空说明模板与数据不符 —— 这是**契约违背**，
    不是"正常情况"，界面必须显式提示，不能静默算进 used。"""

    @property
    def free(self) -> int:
        return max(0, self.total - self.used)

    @property
    def ratio(self) -> float:
        return self.used / self.total if self.total else 0.0

    @property
    def has_spec(self) -> bool:
        return self.total > 0


def usage_of(model: object, port_names: object) -> PortUsage | None:
    """算一台设备的端口使用率。没有模板返回 None。

    ## 为什么去重而不是数条数

    一台 48 口交换机的每个口只接一根线，但同一台设备在布线表里可能出现多行
    （同一台交换机既接计算节点又接存储节点）。按行数会得到「用了 89 个口」
    而实际只有 44 个物理口被占。

    ## 为什么不把 unknown 算进 used

    模板说 56 口，数据里出现 `Eth1/2/1` 这种非标准名 —— 算进去会让 used 虚高、
    利用率超过 100%；不算进去又会让 used 偏低。两种都不对，所以照实单独报出来。

    >>> u = usage_of("业务接入交换机 1", ["25GE1/0/1", "10GE1/0/1", "100GE1/0/2"])
    >>> u.used, u.total, u.free
    (2, 56, 54)
    >>> usage_of("不存在的型号", []) is None
    True
    """
    spec = spec_of(model)
    if spec is None:
        return None
    keys: set[tuple[str, int, int]] = set()
    unknown: list[str] = []
    for raw in port_names:  # type: ignore[union-attr]
        key = spec.bay_key(raw)
        if key is None:
            unknown.append(str(raw))
        else:
            keys.add(key)
    return PortUsage(
        model=spec.model,
        used=len(keys),
        total=spec.total_ports,
        unknown_ports=tuple(sorted(set(unknown))),
    )


def free_bays(model: object, port_names: object) -> tuple[tuple[str, int, int], ...]:
    """还剩哪些口空着（按端口身份返回）。

    给「这个柜子还能再插几台机器」这类问题用：先看空口，再看空 U 位。

    >>> free_bays("服务器密码机", ["千兆1"])[:1]
    (('千兆', 1, 2),)
    """
    spec = spec_of(model)
    if spec is None:
        return ()
    taken = {spec.bay_key(p) for p in port_names}  # type: ignore[union-attr]
    return tuple(s for s in spec.slots() if s not in taken)


def all_models() -> tuple[PortSpec, ...]:
    """全部已知型号模板，按类别 + 型号名排序。"""
    order = {"core": 0, "switch": 1, "security": 2, "storage": 3, "server": 4, "other": 5}
    return tuple(sorted(_MODELS, key=lambda s: (order.get(s.category, 9), s.model)))


__all__ = [
    "PortBay",
    "PortRole",
    "PortSpec",
    "PortUsage",
    "SPECS",
    "MODELS",
    "_ALIASES",
    "all_models",
    "free_bays",
    "spec_of",
    "usage_of",
]
