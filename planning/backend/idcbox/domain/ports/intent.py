"""接线意图（Intent）：用户需要手填的最小信息量，以及把它展开成设备/连线。

## 这个模块存在的理由

v0.2 的系统只能「读」一份已经填好的布线表，不能「生成」。
但用户真正的痛点是：**新建一个机房时，那 5000 行连线表得靠人一行一行填**。
`flow.py` 的三条规则已经证明「布局 + 少量意图 → 全部端口号」是可算的
（真实数据 3850/3850 复现），本模块定义的正是「用户到底要填什么」。

## 反解 V13 后得到的结论：只要三层信息

| 层 | 内容 | V13 实际体积 |
|---|---|---|
| **布局** | 哪个通道的哪些机柜放哪型号服务器 | 3 条 `RackBlock` |
| **路由** | 该型号的哪个网口接哪型号交换机 | 14 条 `RouteSpec` |
| **交换机组** | 每通道里某型号交换机按什么顺序承接某网口 | 51 条 `ChainSpec` |

加上交换机自身位置（上架规划本来就要填），总共约 **70 行**输入。
对 5127 行的布线表来说，这是 1.4% 的输入量。

## 设计上的三个硬约束（都来自实测，不是猜测）

1. **服务器不能用名字引用**。计算节点行的「对端设备名称」在真实数据里
   **是空的**（4562 行全空），只有型号。所以只能靠 `(通道, 机柜, 槽位)` 定位。

2. **交换机组必须带网口**。ch2 的计算接入交换机(业务) 共 10 台，
   `-1/-3/-5/-7/-9` 接 `万兆1`，`-2/-4/-6/-8/-10` 接 `万兆3`。
   只按 `(通道, 交换机型号)` 建链会把这 10 台混成一条 —— 实测配对率从 100% 掉到 14.09%。

3. **容量必须逐台声明**。存储接入交换机实测只有 40/36 口，不是通用的 44。
   写死 44 会让块边界全错，后续所有端口号连锁错位。
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path
from typing import Mapping

from ..shared.types import Issue, IssueCode
from ..units import SLOT_BASE_U, SLOT_COUNT, SLOT_PITCH_U
from .flow import (
    DEFAULT_CAPACITY,
    FlowOrder,
    NicRoute,
    Placement,
    SwitchChain,
    default_port_role,
    derive_port_prefix,
)

# ---------------------------------------------------------------------------
# 布局层
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class RackBlock:
    """一个「机柜段」：某通道里连续几个机柜，全部放同一型号的服务器。

    这是用户最省力的填法。V13 的真实布局正好能归纳成 6 段：
    `ch2 rack1..12 = 计算节点A`、`ch2 rack13..25 = 计算节点B` 之类。

    >>> b = RackBlock(channel_no=2, rack_from=1, rack_to=6, node_model="计算节点A")
    >>> b.rack_count, b.device_count
    (6, 48)

    ## 为什么需要 `slot_map`（非标准机柜）

    默认假设每个机柜是标准的 8 个 2U 槽位（U3/U7/…/U31）。
    但真实上架表里机柜并不总是满配：V13 实测 69 个机柜里有 7 个不是 8 台
    （ch2/ch8/ch9 的 rack7 各 7 台、rack11 各 9 台、ch9 的 rack17 只有 6 台）。

    不规则必须能表达，否则后面所有端口号会连锁错位 ——
    实测：忽略这 7 个机柜，3850 行里只有 3754 行能对上（97.51%），
    且失败集中在 rack8..rack11（相位轮转被一台之差整体推偏）。

    `slot_map` 用「机柜号 → 槽位序列」显式覆盖，槽位可重复（同 U 位多台）。

    >>> b2 = RackBlock(2, 7, 7, "A", slot_map=((7, (0, 1, 2, 3, 4, 5, 6)),))
    >>> b2.device_count
    7
    """

    channel_no: int
    rack_from: int
    rack_to: int
    node_model: str
    #: 机柜号 → 槽位序列。未列出的机柜用 `slots`，再退回标准 `range(SLOT_COUNT)`
    slot_map: tuple[tuple[int, tuple[int, ...]], ...] = ()
    #: 本段所有机柜共用的槽位序列。留空 = 标准 8 槽位。
    #:
    #: 存储通道（ch3）靠这个表达「4 个机柜只占前 2 个槽位，另外 4 个槽位给别的型号」：
    #:
    #: >>> a = RackBlock(3, 1, 4, "SSD", slots=(0, 1, 2, 3, 4, 5))
    #: >>> b = RackBlock(3, 1, 4, "EHDD1", slots=(6, 7))
    #: >>> a.device_count, b.device_count
    (24, 8)
    slots: tuple[int, ...] = ()

    def __post_init__(self) -> None:
        if self.rack_from > self.rack_to:
            raise ValueError(
                f"机柜段起止反了：{self.rack_from} > {self.rack_to}"
                f"（ch{self.channel_no} {self.node_model}）"
            )
        if self.rack_from < 1:
            raise ValueError(f"机柜号从 1 开始，收到 {self.rack_from}")
        for rack, slots in self.slot_map:
            if not self.rack_from <= rack <= self.rack_to:
                raise ValueError(
                    f"slot_map 里的机柜 {rack} 不在本段 {self.rack_from}..{self.rack_to} 内"
                )
            bad = [s for s in slots if not 0 <= s < SLOT_COUNT]
            if bad:
                raise ValueError(f"机柜 {rack} 的槽位号越界（应为 0..{SLOT_COUNT - 1}）：{bad}")
        bad = [s for s in self.slots if not 0 <= s < SLOT_COUNT]
        if bad:
            raise ValueError(f"块内槽位号越界（应为 0..{SLOT_COUNT - 1}）：{bad}")
        if self.slots and len(set(self.slots)) != len(self.slots):
            dup = [s for s in set(self.slots) if self.slots.count(s) > 1]
            raise ValueError(f"块内槽位重复：{sorted(dup)}；同机柜叠放请用 slot_map 显式声明")

    @property
    def rack_count(self) -> int:
        return self.rack_to - self.rack_from + 1

    @property
    def _override(self) -> dict[int, tuple[int, ...]]:
        return dict(self.slot_map)

    def slots_of(self, rack: int) -> tuple[int, ...]:
        """某个机柜的槽位序列：`slot_map` → `slots` → 标准 8 槽位。"""
        if rack in self._override:
            return self._override[rack]
        if self.slots:
            return self.slots
        return tuple(range(SLOT_COUNT))

    @property
    def device_count(self) -> int:
        return sum(len(self.slots_of(r)) for r in self.racks())

    def racks(self) -> range:
        return range(self.rack_from, self.rack_to + 1)


@dataclass(frozen=True, slots=True)
class SwitchSpec:
    """一台交换机（或任何「本端」网络设备）的上架位置。

    ## 为什么容量不在这里

    实测 `存储接入交换机-1` 有多个端口面：
    `25G1` 面 40 个下行口、`25GE1/0/48` 是上行口、`100GE1/0/7|8` 是上行。
    整机 max 端口号是 48，但**下行容量只有 40**。

    所以容量是「**某个端口面**能接多少台」的属性，属于 `ChainSpec` 而不是交换机。
    这一点是被 golden 用例抓出来的：把容量写成整机 max 会让块边界错，
    `[40, 40, 36]` 会被算成 `[48, 48, 20]`，后面所有端口号连锁错位。
    """

    instance: str
    model: str
    channel_no: int
    rack_no: int
    #: 上架起始 U 位
    u_start: int
    #: 占用高度（U 数）。默认 1U（TOR 交换机）。
    #:
    #: ## 为什么必须能声明
    #:
    #: 真实 V13 里 4 台核心交换机（业务核心-1/-2、存储核心-1/-2）是
    #: **框式设备，占 `3-12U` 共 10U**，不是 1U。它们作为本端出现在
    #: 548 行交换机互联里，U 位一律写成 `3-12U`。
    #:
    #: 只记起始 U 位、一律输出 `40U` 会让这 298 行 U 位全错
    #: （实测 298 处差异都源于此）。
    u_span: int = 1


@dataclass(frozen=True, slots=True)
class DeviceSpec:
    """一台**有名字的**非交换机设备的上架位置。

    ## 为什么需要它（与 `SwitchSpec`、`RackBlock` 的关系）

    三类设备的表达方式不同，因为它们的信息量本来就不同：

    | 设备 | 表达方式 | 为什么 |
    |---|---|---|
    | 计算/存储节点（550 台） | `RackBlock` 段 + 槽位 | 同型号成千台，按规程排布，写段就行 |
    | 交换机（130 台） | `SwitchSpec` 逐台 | 位置与接线组都影响端口号，必须逐台 |
    | 特殊机型（34 台） | `DeviceSpec` 逐台 | 每台接法不同，且**有业务名字** |

    特殊机型指网络功能服务器、GSO、管理节点、密码机、签名服务器、
    加密机、负载均衡、防火墙 —— 实测 34 台，占 144 行连线。

    它们不进 `RackBlock` 是因为按型号写段会展开出并不存在的设备
    （`网络功能服务器` 8 台分散在 4 个机柜，同一机柜内的其他槽位是别人的）。
    它们也不能并进 `SwitchSpec`：那个类的语义是「网络设备」，
    而且里面没有 `capacity` 之外的端口面信息，混用会让后来的读者误判。

    ## 名字必须是真实业务名

    `ExplicitLink` 靠名字引用设备，而计算节点的名字在源表里普遍为空。
    所以这里的 `instance` 必须是真实表「对端设备名称」列里的值
    （`xkgw-1`、`nat-2`、`controller-3`），否则显式连线解析不到。
    """

    instance: str
    model: str
    channel_no: int
    rack_no: int
    u_start: int
    #: 占用高度（U 数）。默认 2U —— 服务器机型都是 2U。
    #:
    #: 例外是 `广域网链路负载均衡-1/-2`，真实表里写 `19-20U`（2U，与默认一致），
    #: 而计算节点类都写 `3-4U`。留这个字段是为了将来出现 4U 存储机也能表达。
    u_span: int = 2


# ---------------------------------------------------------------------------
# 路由层
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class RouteSpec:
    """「某型号服务器的某个网口，接到某型号交换机」。

    这是**型号级**声明，不是实例级。V13 实测 14 种路由覆盖 3850 行：
    计算节点A 的 `万兆1` 接 `业务接入交换机 1`，所有 A 型机都一样。

    为什么不分实例：16 个服务器型号中 13 个的「网口→交换机型号」指纹
    100% 一致，合计主指纹覆盖率 92.9%。剩下不一致的（网络功能服务器
    只有 50%）用 `ExplicitLink` 逐条补，不污染规则层。
    """

    node_model: str
    nic: str
    switch_model: str


@dataclass(frozen=True, slots=True)
class ChainSpec:
    """一个通道里，某型号交换机承接某网口的**顺序**与**容量**。

    ## 为什么必须带网口

    ch2 的计算接入交换机(业务) 10 台分两组：`-1/-3/-5/-7/-9` 接 `万兆1`，
    `-2/-4/-6/-8/-10` 接 `万兆3`。只按 `(通道, 交换机型号)` 建链会把
    两组混成一条，实测端口配对率从 100% 掉到 14.09%。

    ## 为什么容量在这里而不在交换机上

    同一台交换机不同端口面的容量不同。实测 `存储接入交换机-1`：
    `25G1` 面下行 40 口，`25GE1/0/48` 是上行口，整机 max 端口号 48。
    容量写 48 会让 `[40, 40, 36]` 变成 `[48, 48, 20]`，全盘错位。

    ## 为什么顺序有意义

    节点按规则排序后**连续切块**：第 1 台吃前 `capacity` 个，第 2 台吃接着的。
    所以 `instances` 的顺序就是「谁先接」，不能乱。最后一台吃剩余全部
    （可超出容量），实测 ch9 三台是 `[44, 44, 46]`。
    """

    channel_no: int
    switch_model: str
    nic: str
    #: 按「端口 1 起始」顺序排列的交换机实例名
    instances: tuple[str, ...]
    #: 该端口面的下行端口数。24 = 只接了 24 台，40/44 = 满载
    capacity: int = DEFAULT_CAPACITY
    #: 端口名前缀，如 `25GE1/0`。留空则按网口推导
    port_prefix: str = ""
    #: 节点排布主轴。`rack_major` = 机柜外层（ch2/ch8/ch9 计算通道实测），
    #: `slot_major` = 槽位外层（ch3 存储通道实测）。
    #: 两种形态都真实存在，写错主轴会让整条链的端口号全错。
    order: FlowOrder = "rack_major"


# ---------------------------------------------------------------------------
# 显式连线（规则覆盖不到的少数）
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class ExplicitLink:
    """手工指定的一条连线。用于交换机互联与规则覆盖不到的机型。

    V13 全量 5113 行里，3850 行是「交换机→服务器」下行（可由规则生成），
    其余 1263 行是交换机互联与安全设备连线 —— 这些没有规律，只能显式填。
    但那 1263 行只占 24.7%，且集中在少数设备之间，手填成本可控。
    """

    a_device: str
    a_port: str
    b_device: str
    b_port: str
    #: 留空则由两端端口速率派生（实测 99.71% 可靠）
    cable_type: str = ""


# ---------------------------------------------------------------------------
# 意图整体
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class Intent:
    """一个项目的完整接线意图。可序列化成 YAML/JSON，是 v0.3 的输入格式。"""

    project: str
    #: 机房/区域标签，仅用于展示
    site: str = ""
    racks: tuple[RackBlock, ...] = ()
    switches: tuple[SwitchSpec, ...] = ()
    devices: tuple[DeviceSpec, ...] = ()
    routes: tuple[RouteSpec, ...] = ()
    chains: tuple[ChainSpec, ...] = ()
    explicit_links: tuple[ExplicitLink, ...] = ()

    @property
    def server_count(self) -> int:
        return sum(b.device_count for b in self.racks)


def expand_placements(intent: Intent) -> tuple[Placement, ...]:
    """布局 → 逐台设备的 `Placement`。

    槽位 `s` 对应的 U 位起始是 `SLOT_BASE_U + s * SLOT_PITCH_U`（实测 3, 7, …, 31）。
    设备 id 用 `N{通道}-{机柜}-S{槽位}`：稳定、可读、跨次生成不变。

    >>> it = Intent(project="demo", racks=(RackBlock(2, 1, 1, "A"),))
    >>> ps = expand_placements(it)
    >>> len(ps), ps[0].node_id, ps[0].u_start
    (8, 'N02-01-S0', 3)

    ## 同一槽位出现两次时，id 带 `-k` 后缀

    `flow.py` 对重复槽位用「顺延整轮」排位，且同槽位内的先后由 **node_id 字典序**
    决定。所以第 2 台的 id 必须排在前面那些之后 —— `N02-11-S7-1 > N02-11-S7`
    正好满足，无需额外排序键。

    >>> it2 = Intent("demo", racks=(RackBlock(2, 11, 11, "A",
    ...     slot_map=((11, (0, 1, 2, 3, 4, 5, 6, 7, 7)),)),))
    >>> [p.node_id for p in expand_placements(it2)][-2:]
    ['N02-11-S7', 'N02-11-S7-1']
    """
    #: 位置覆盖：`devices` 段里 instance 恰是槽位 id 的条目，用来把布局段
    #: 自动排布的单台设备钉在某个位置（可能换了机柜）。见 `placement_moves`。
    moves = placement_moves(intent)

    out: list[Placement] = []
    for block_index, block in enumerate(intent.racks):
        for rack in block.racks():
            seen: dict[int, int] = {}
            for slot in block.slots_of(rack):
                occ = seen.get(slot, 0)
                seen[slot] = occ + 1
                suffix = f"-{occ}" if occ else ""
                node_id = f"N{block.channel_no:02d}-{rack:02d}-S{slot}{suffix}"
                ch, rk, u = moves.get(
                    node_id,
                    (block.channel_no, rack, SLOT_BASE_U + slot * SLOT_PITCH_U),
                )
                out.append(
                    Placement(
                        node_id=node_id,
                        model=block.node_model,
                        channel_no=ch,
                        rack_no=rk,
                        u_start=u,
                        block_index=block_index,
                    )
                )
    return tuple(out)


def placement_overrides(intent: Intent) -> dict[str, int]:
    """`devices` 段里「钉住」某台布局段设备的结果：`{槽位 id: U 位}`。

    ## 为什么需要这一层

    布局段（`RackBlock`）一次描述「某通道连续几个机柜全放同一型号」，
    V13 里 663 台设备（79.5%）是这么来的 —— 它们的 U 位由槽位号算出来，
    **没有字段可以单独改**。这导致 U 位图上这 663 台拖不动。

    要改单台只有两条路：把整个布局段拆散（用户要重填几十行），
    或者引入一个「例外清单」。这里选后者：`devices` 段本来就逐台声明，
    它的 `instance` 字段允许写槽位 id（`N02-01-S0`），含义是
    「这台的位置不用布局段算，用我写的 `u`」。

    ## 为什么槽位 id 不变、只改 U 位

    如果拆出来的设备换了 node_id（比如改成用户起的名字），它就不再是
    那条交换机组里的第 N 台，端口分配会整体错位。保持 node_id 不变、
    只挪 U 位，语义正好是「这台机器换了个机架上位置」——
    线要跟着重接，所以端口分配按新顺序重排是对的。

    >>> blk = RackBlock(2, 1, 1, "A")
    >>> it = Intent("p", racks=(blk,), devices=(
    ...     DeviceSpec("N02-01-S0", "A", 2, 1, u_start=7, u_span=2),))
    >>> placement_overrides(it)
    {'N02-01-S0': 7}
    >>> [p.u_start for p in expand_placements(it)][:2]
    [7, 7]
    """
    return {slot_id: u for slot_id, (_ch, _rk, u) in placement_moves(intent).items()}


def placement_moves(intent: Intent) -> dict[str, tuple[int, int, int]]:
    """`devices` 段对布局段设备的完整位置覆盖：`{槽位 id: (通道, 机柜, U 位)}`。

    `placement_overrides` 只给 U 位（同柜内微调够用）。跨机柜拖动要同时
    覆盖机柜号 —— 否则「把 ch2 rack1 的第 1 台挪到 rack5」写进 `devices` 后，
    `expand_placements` 仍按布局段算出 rack1，用户看到位置没变。

    >>> blk = RackBlock(2, 1, 3, "A")
    >>> it = Intent("p", racks=(blk,), devices=(
    ...     DeviceSpec("N02-01-S0", "A", 2, 3, u_start=27, u_span=2),))
    >>> placement_moves(it)
    {'N02-01-S0': (2, 3, 27)}
    >>> [(p.rack_no, p.u_start) for p in expand_placements(it)][:2]
    [(3, 27), (1, 7)]
    """
    owner: dict[str, int] = {}          # 槽位 id → 它所属布局段的通道号
    allowed: set[tuple[int, int]] = set()   # (通道, 机柜)：真实存在的排布位
    for block in intent.racks:
        for rack in block.racks():
            allowed.add((block.channel_no, rack))
            seen: dict[int, int] = {}
            for slot in block.slots_of(rack):
                occ = seen.get(slot, 0)
                seen[slot] = occ + 1
                suffix = f"-{occ}" if occ else ""
                owner[f"N{block.channel_no:02d}-{rack:02d}-S{slot}{suffix}"] = block.channel_no

    out: dict[str, tuple[int, int, int]] = {}
    for dev in intent.devices:
        home_channel = owner.get(dev.instance)
        if home_channel is None:
            continue
        # 两条约束，都是为了「写进去就一定排得出来」：
        # ①通道不变 —— 换通道意味着换了一组 ToR 来服务它，那是重新规划，
        #   不是拖动（端口分配链按通道分组，跨通道会让两条链的容量都对不上）。
        # ②目标机柜必须在某个布局段里 —— 否则它在这份规划中不存在，
        #   写进去只会得到一个「在库里但不参与任何排布」的幽灵位置。
        if dev.channel_no != home_channel:
            continue
        if (home_channel, dev.rack_no) not in allowed:
            continue
        out[dev.instance] = (home_channel, dev.rack_no, dev.u_start)
    return out


def to_flow_inputs(
    intent: Intent,
) -> tuple[tuple[Placement, ...], tuple[NicRoute, ...], tuple[SwitchChain, ...], list[Issue]]:
    """把意图翻译成 `plan_flow` 的三个入参，同时报出「意图自身不自洽」的问题。

    三件必查的事（都是真实项目里会犯的错）：

    1. 路由引用了不存在的交换机型号 → 该网口永远接不上
    2. 链里的交换机实例不在 `switches` 里 → 位置未知，生成不出本端 U 位
    3. 同一 `(通道, 型号, 网口)` 声明了两条链 → 节点会被分配两次
    """
    issues: list[Issue] = []
    placements = expand_placements(intent)

    switch_models = {s.model for s in intent.switches}
    known_models = {b.node_model for b in intent.racks}

    routes: list[NicRoute] = []
    for r in intent.routes:
        if r.switch_model not in switch_models:
            issues.append(
                Issue(
                    IssueCode.UNKNOWN_CABLE_TYPE,
                    "error",
                    f"路由 {r.node_model}/{r.nic} 指向交换机型号「{r.switch_model}」，"
                    f"但设备清单里没有这个型号",
                    field="routes",
                )
            )
            continue
        if r.node_model not in known_models:
            issues.append(
                Issue(
                    IssueCode.PORT_UNASSIGNED,
                    "warn",
                    f"路由声明了服务器型号「{r.node_model}」，但布局里没有这种机型，"
                    f"该路由不会产生任何连线",
                    field="routes",
                )
            )
        routes.append(
            NicRoute(
                node_model=r.node_model,
                nic=r.nic,
                switch_model=r.switch_model,
                port_role=default_port_role(r.nic),
            )
        )

    by_instance = {s.instance: s for s in intent.switches}
    chains: list[SwitchChain] = []
    seen_chain_key: dict[tuple[int, str, str], str] = {}
    for c in intent.chains:
        key = (c.channel_no, c.switch_model, c.nic)
        if key in seen_chain_key:
            issues.append(
                Issue(
                    IssueCode.PORT_ALREADY_USED,
                    "error",
                    f"ch{c.channel_no} {c.nic}→{c.switch_model} 的交换机组重复声明，"
                    f"节点会被分配两次",
                    field="chains",
                )
            )
            continue
        seen_chain_key[key] = c.switch_model

        missing = [i for i in c.instances if i not in by_instance]
        if missing:
            shown = "、".join(missing[:5]) + ("…" if len(missing) > 5 else "")
            issues.append(
                Issue(
                    IssueCode.PORT_UNASSIGNED,
                    "error",
                    f"ch{c.channel_no} {c.nic}→{c.switch_model} 引用了设备清单里"
                    f"没有的交换机：{shown}",
                    field="chains",
                )
            )

        # 容量与端口前缀由链自己声明（同一台交换机不同端口面容量不同）
        chains.append(
            SwitchChain(
                channel_no=c.channel_no,
                switch_model=c.switch_model,
                switch_instances=c.instances,
                nic=c.nic,
                capacity=c.capacity,
                order=c.order,
                port_prefix=c.port_prefix or derive_port_prefix(c.nic),
            )
        )

    return placements, tuple(routes), tuple(chains), issues


# ---------------------------------------------------------------------------
# 序列化：JSON 双向转换（零第三方依赖）
# ---------------------------------------------------------------------------
#: 意图文件格式版本。未来字段语义变了就升这个号，旧文件能被识别而不是静默读错。
INTENT_SCHEMA_VERSION = 1


def _rack_block_to_dict(b: RackBlock) -> dict:
    d: dict = {
        "channel": b.channel_no,
        "from": b.rack_from,
        "to": b.rack_to,
        "model": b.node_model,
    }
    #: `slots` 与 `slotMap` 是**两件不同的事**，必须都序列化：
    #: - `slots`   = 本块内**所有机柜共用**的槽位序列（ch3 存储的 2 槽/6 槽机型）
    #: - `slotMap` = **个别机柜**的槽位与默认不同（rack11 叠放 9 台）
    #:
    #: 只存 `slotMap` 会把 `slots` 整个丢掉，回读后每柜按默认 8 槽展开：
    #: ch3 存储设备数从 128 台涨到 192 台，分配数从 4430 涨到 5061，全盘错位。
    #: 这个 bug 只在「存盘再读回」的路径上暴露 —— 内存里直接构造 Intent 是好的，
    #: 所以必须由「JSON 往返后仍 100% 命中」这条用例来守（见 golden 用例）。
    if b.slots:
        d["slots"] = list(b.slots)
    if b.slot_map:
        d["slotMap"] = {str(r): list(s) for r, s in b.slot_map}
    return d


def _rack_block_from_dict(d: Mapping) -> RackBlock:
    raw = d.get("slotMap") or {}
    return RackBlock(
        channel_no=int(d["channel"]),
        rack_from=int(d["from"]),
        rack_to=int(d["to"]),
        node_model=str(d["model"]),
        slot_map=tuple((int(k), tuple(int(s) for s in v)) for k, v in raw.items()),
        slots=tuple(int(s) for s in (d.get("slots") or ())),
    )


def intent_to_dict(intent: Intent) -> dict:
    """意图 → 可 JSON 序列化的 dict。

    >>> it = Intent("p", racks=(RackBlock(2, 1, 2, "A"),))
    >>> d = intent_to_dict(it)
    >>> d["schema"], d["racks"]
    (1, [{'channel': 2, 'from': 1, 'to': 2, 'model': 'A'}])
    """
    return {
        "schema": INTENT_SCHEMA_VERSION,
        "project": intent.project,
        "site": intent.site,
        "racks": [_rack_block_to_dict(b) for b in intent.racks],
        "switches": [
            {
                "instance": s.instance,
                "model": s.model,
                "channel": s.channel_no,
                "rack": s.rack_no,
                "u": s.u_start,
                **({"uSpan": s.u_span} if s.u_span != 1 else {}),
            }
            for s in intent.switches
        ],
        "devices": [
            {
                "instance": x.instance,
                "model": x.model,
                "channel": x.channel_no,
                "rack": x.rack_no,
                "u": x.u_start,
                **({"uSpan": x.u_span} if x.u_span != 2 else {}),
            }
            for x in intent.devices
        ],
        "routes": [
            {"nodeModel": r.node_model, "nic": r.nic, "switchModel": r.switch_model}
            for r in intent.routes
        ],
        "chains": [
            {
                "channel": c.channel_no,
                "switchModel": c.switch_model,
                "nic": c.nic,
                "instances": list(c.instances),
                "capacity": c.capacity,
                **({"portPrefix": c.port_prefix} if c.port_prefix else {}),
                **({"order": c.order} if c.order != "rack_major" else {}),
            }
            for c in intent.chains
        ],
        "explicitLinks": [
            {
                "a": [x.a_device, x.a_port],
                "b": [x.b_device, x.b_port],
                **({"cableType": x.cable_type} if x.cable_type else {}),
            }
            for x in intent.explicit_links
        ],
    }


def intent_from_dict(d: Mapping) -> Intent:
    """dict → 意图。字段缺失按默认值处理，未知字段忽略（向前兼容）。

    >>> d = {"project": "p", "racks": [{"channel": 2, "from": 1, "to": 1, "model": "A"}]}
    >>> intent_from_dict(d).server_count
    8
    """
    ver = d.get("schema", INTENT_SCHEMA_VERSION)
    if ver > INTENT_SCHEMA_VERSION:
        raise ValueError(
            f"意图文件格式版本 {ver} 比本程序支持的 {INTENT_SCHEMA_VERSION} 新，请升级程序"
        )
    return Intent(
        project=str(d.get("project", "")),
        site=str(d.get("site", "")),
        racks=tuple(_rack_block_from_dict(x) for x in d.get("racks", ())),
        switches=tuple(
            SwitchSpec(
                instance=str(x["instance"]),
                model=str(x["model"]),
                channel_no=int(x["channel"]),
                rack_no=int(x["rack"]),
                u_start=int(x["u"]),
                u_span=int(x.get("uSpan", 1)),
            )
            for x in d.get("switches", ())
        ),
        devices=tuple(
            DeviceSpec(
                instance=str(x["instance"]),
                model=str(x["model"]),
                channel_no=int(x["channel"]),
                rack_no=int(x["rack"]),
                u_start=int(x["u"]),
                u_span=int(x.get("uSpan", 2)),
            )
            for x in d.get("devices", ())
        ),
        routes=tuple(
            RouteSpec(
                node_model=str(x["nodeModel"]),
                nic=str(x["nic"]),
                switch_model=str(x["switchModel"]),
            )
            for x in d.get("routes", ())
        ),
        chains=tuple(
            ChainSpec(
                channel_no=int(x["channel"]),
                switch_model=str(x["switchModel"]),
                nic=str(x["nic"]),
                instances=tuple(str(i) for i in x["instances"]),
                capacity=int(x.get("capacity", DEFAULT_CAPACITY)),
                port_prefix=str(x.get("portPrefix", "")),
                order=x.get("order", "rack_major"),
            )
            for x in d.get("chains", ())
        ),
        explicit_links=tuple(
            ExplicitLink(
                a_device=str(x["a"][0]),
                a_port=str(x["a"][1]),
                b_device=str(x["b"][0]),
                b_port=str(x["b"][1]),
                cable_type=str(x.get("cableType", "")),
            )
            for x in d.get("explicitLinks", ())
        ),
    )


def load_intent(path: str | Path) -> Intent:
    """从 JSON 文件读意图。

    选 JSON 不选 YAML 的理由：域层**零第三方依赖**是硬合同
    （`pyproject.toml` 的 `dependencies = []` + import-linter 契约在守）。
    YAML 需要 PyYAML，JSON 是标准库。手写 JSON 的体验差别不大，
    而且程序会提供 `intent export` 把现有项目导成模板。
    """
    with open(path, encoding="utf-8") as f:
        return intent_from_dict(json.load(f))


def dump_intent(intent: Intent, path: str | Path) -> None:
    """把意图写成 JSON 文件（`ensure_ascii=False` + 缩进，人能直接改）。"""
    with open(path, "w", encoding="utf-8") as f:
        json.dump(intent_to_dict(intent), f, ensure_ascii=False, indent=2)
        f.write("\n")


__all__ = [
    "INTENT_SCHEMA_VERSION",
    "ChainSpec",
    "DeviceSpec",
    "ExplicitLink",
    "Intent",
    "RackBlock",
    "RouteSpec",
    "SwitchSpec",
    "dump_intent",
    "expand_placements",
    "placement_moves",
    "placement_overrides",
    "intent_from_dict",
    "intent_to_dict",
    "load_intent",
    "to_flow_inputs",
]
