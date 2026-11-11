"""接线流：从「布局 + 网口路由」确定性生成逐端口分配（3850/3850 = 100%）。

## 已破解的完整规则链（三条规则串联，缺一不可）

### 规则 1：柜内相位轮转排序

通道内先按机柜升序，**柜内不是按 slot 升序**，而是按

```
(slot - carry % 8) % 8        升序
```

`carry` 是**前面所有机柜的节点数累计**。

为什么必须带轮转：`ch2 rack7` 只有 7 台（缺一个槽位），走到 rack8 时
`carry = 55`，`55 % 8 = 7`，于是 rack8 的柜内顺序变成
`slot7 → slot0 → slot1 → … → slot6`（整体轮转 1 位）。

实测对比（3850 个计算节点，逐行比对真实端口号）：

| 排序模型 | 端口号命中 |
|---|---|
| 纯 `(机柜, slot)` 升序 | 3178/3850 = 82.55% |
| 相位轮转（本条） | 3682/3850 = 95.64% |
| 相位轮转 + 规则 2 | **3850/3850 = 100.00%** |

### 规则 2：重复槽位顺延到下一轮

同一机柜同一 slot 出现多台时（`ch2/ch8/ch9` 的 `rack11 31-32U` 各有 2 台，
全表共 21 组），第 k 个重复项的排序键 = `k * 8 + 轮转序号`，
即**顺延到下一整轮**，而不是与第一个相邻。

漏掉这条规则就卡在 95.64%：`rank 79/80..86/87` 本该是
`slot7(第一台) → slot0..slot6 → slot7(第二台)`，
若把两台 slot7 排在一起，整段会错位一位（真实 `p36→生成 p37`、`p44→生成 p37`）。

### 规则 3：连续切块 + 末台吸收余量

有序序列按交换机清单顺序**连续切开**：前 K-1 台各装 `capacity`（实测 44）个，
末台装剩下的全部（**可能超过 capacity**）。

实测 21 个 (通道, 网口) 组的块大小全部吻合：

```
ch2  n=200  K=5  → [44, 44, 44, 44, 24]     # 末台不足
ch8  n=216  K=5  → [44, 44, 44, 44, 40]     # 末台不足
ch9  n=134  K=3  → [44, 44, 46]             # 末台超出 2
```

`ch9` 的 46 证明「末台吸收」不是「末台也最多 44 然后开新交换机」——
实际就是硬塞进最后一台。

## 为什么这条链是产品护城河

它把「规划」和「接线」彻底解耦：

- **布局层**只说「哪个机柜哪个 U 位放什么型号」（用户本来就要填）
- **路由层**只说「某型号的某个网口接哪几台交换机、按什么顺序」
- **端口号、端口名全部确定性推出**，不需要人填 3850 行端口

反解验证（只用布局 + 路由 + 容量 44，不碰任何真实端口号）：
交换机配对 **3850/3850**、端口号 **3850/3850**，零失败。

## 已证伪的方向（不要再试）

- 意图 = 「交换机 + 节点型号 + 机柜区间」→ 展开区间内全部同型号节点：
  命中 **19/174 = 10.9%**。根因：机柜被多台交换机共享，
  一个 6 柜区间里 125 台节点，交换机只接 44 台。
- `(型号, 网口, 交换机)` 组合口径 **51/184 = 27.7%**。
- 严格只算计算节点的区间展开 **7/112 = 6.2%**。

根因都是同一个：**分配不是按区间，是按全局有序序列连续切块**。
"""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass
from typing import Iterable, Literal, Sequence

from ..shared.types import Issue, IssueCode
from ..units import (
    SLOT_COUNT,
    SPEED_10G,
    SPEED_100G,
    SPEED_1G,
    SPEED_25G,
)
from .assign import PortAssignment, PortRole
from .naming import parse_port_name
from .phase import slot_of

#: 交换机单个下行网口的默认容量（实测 44，来自业务/存储/带内/带外接入交换机）
DEFAULT_CAPACITY: int = 44

#: 节点排布主轴（决定「先遍历机柜还是先遍历槽位」）。见 `order_by_slot_major`。
FlowOrder = Literal["rack_major", "slot_major"]


# ---------------------------------------------------------------------------
# 输入：布局层与路由层
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class Placement:
    """一台已上架设备的物理位置 + 型号身份。

    这是**布局层的唯一产出**：用户填「哪个机柜哪个 U 位放什么型号」，
    不需要填端口、不需要填对端。
    """

    node_id: str
    model: str
    channel_no: int
    rack_no: int
    #: U 位起始（1-based），如 `3-4U` → 3
    u_start: int
    #: 该设备来自意图里的第几个布局块（`RackBlock` 的声明序号）。
    #:
    #: 为什么需要它：`slot_major` 排布下，块的**声明顺序**就是接线顺序，
    #: 而「块」是二维瓦片（如 `EHDD1 机柜 5-8 槽位 2-5`），
    #: 无法用 `(机柜, 槽位)` 的一维排序键表达。详见 `order_by_slot_major`。
    block_index: int = 0

    @property
    def slot(self) -> int:
        """8 槽位编号（U3→0, U7→1, …, U31→7）。"""
        return slot_of(self.u_start)

    @property
    def rack_key(self) -> tuple[int, int]:
        return (self.channel_no, self.rack_no)


@dataclass(frozen=True, slots=True)
class NicRoute:
    """路由层的单条声明：「某型号的某个网口，接到某型号的交换机」。

    注意这里**不说接第几台**——顺序由 `SwitchChain` 决定。
    """

    node_model: str
    nic: str
    switch_model: str
    port_role: PortRole = "downlink"


@dataclass(frozen=True, slots=True)
class SwitchChain:
    """某通道里某型号交换机**按接线顺序**排列的实例清单。

    `switch_instances` 的顺序就是分配顺序：第 1 台吃前 capacity 个节点，
    第 2 台吃接下来的 capacity 个，最后一台吃剩余全部。

    ## `nic` 字段为什么必须存在

    同一型号的交换机在**同一通道**里可能分成几组、各接不同的网口。
    实测 `ch2` 的 `计算接入交换机(业务)` 共 10 台：

    ```
    -1 / -3 / -5 / -7 / -9   → 承接 万兆1
    -2 / -4 / -6 / -8 / -10  → 承接 万兆3
    ```

    如果只按 `(通道, 交换机型号)` 建链，10 台会被混成一条，端口号整段错位 ——
    实测配对率会从 100% 掉到 14%。所以链必须带上它服务的网口。
    """

    channel_no: int
    switch_model: str
    switch_instances: tuple[str, ...]
    #: 该链服务的网口标签（`万兆1`/`千兆2`/`IPMI`…）。留空表示不限网口。
    nic: str = ""
    capacity: int = DEFAULT_CAPACITY
    #: 交换机侧端口名前缀；留空则按网口速率自动推导（`万兆1` → `10GE1/0`）
    port_prefix: str = ""
    #: 节点排布主轴，见 `FlowOrder`。默认 `rack_major`（机柜外层、槽位内层）。
    order: FlowOrder = "rack_major"


@dataclass(frozen=True, slots=True)
class FlowBlock:
    """切块结果：一台交换机 + 它分到的节点序列（已按规则排好序）。"""

    switch_instance: str
    switch_model: str
    channel_no: int
    nic: str
    nodes: tuple[Placement, ...]
    port_prefix: str
    port_role: PortRole

    @property
    def size(self) -> int:
        return len(self.nodes)


@dataclass(frozen=True, slots=True)
class FlowPlan:
    """接线流的完整结果。"""

    blocks: tuple[FlowBlock, ...]
    assignments: tuple[PortAssignment, ...]
    issues: tuple[Issue, ...]

    @property
    def total_assignment(self) -> int:
        return len(self.assignments)

    def by_switch(self) -> dict[str, list[PortAssignment]]:
        out: dict[str, list[PortAssignment]] = {}
        for a in self.assignments:
            out.setdefault(a.switch_instance, []).append(a)
        return out


# ---------------------------------------------------------------------------
# 规则 1 + 规则 2：有序化
# ---------------------------------------------------------------------------
def order_by_phase_chain(nodes: Sequence[Placement]) -> tuple[Placement, ...]:
    """机柜升序 → 柜内相位轮转 → 重复槽位顺延。**确定性、可重放**。

    同一通道内传入的节点会被排成一个全局有序序列，后续按容量连续切块。

    >>> ns = [Placement(f"n{i}", "A", 2, 1 if i < 8 else 2, 3 + 4 * (i % 8)) for i in range(16)]
    >>> [p.node_id for p in order_by_phase_chain(ns)][:3]
    ['n0', 'n1', 'n2']
    """
    by_rack: dict[int, list[Placement]] = {}
    for n in nodes:
        by_rack.setdefault(n.rack_no, []).append(n)

    ordered: list[Placement] = []
    carry = 0
    for rack_no in sorted(by_rack):
        group = by_rack[rack_no]
        base = carry % SLOT_COUNT
        seen: Counter[int] = Counter()
        # 同槽位内的顺序：按 U 位、再按 node_id，保证两端一致（不是按 Excel 行号）
        keyed: list[tuple[int, int, str, Placement]] = []
        for n in sorted(group, key=lambda p: (p.u_start, p.node_id)):
            s = n.slot
            occ = seen[s]
            seen[s] += 1
            # 规则 2：同槽位第 k 个重复项顺延 k 个整轮
            keyed.append((occ * SLOT_COUNT + (s - base) % SLOT_COUNT, n.u_start, n.node_id, n))
        keyed.sort(key=lambda t: (t[0], t[1], t[2]))
        ordered.extend(t[3] for t in keyed)
        carry += len(group)
    return tuple(ordered)


def order_by_slot_major(nodes: Sequence[Placement]) -> tuple[Placement, ...]:
    """槽位外层 → 机柜内层排布。**存储通道（ch3）实测为 100% 正确**。

    ## 为什么需要第二种排布主轴

    `ch2` / `ch8` / `ch9`（计算节点）是 `rack_major`：机柜外层，柜内 8 槽位内层，
    且柜内顺序随前序机柜节点数做相位轮转（规则 1）。

    `ch3`（存储节点）完全不同 —— 实测真实端口序列：

    ```
    1:SSD1s0  2:SSD2s0  3:SSD3s0  4:SSD4s0  5:SSD1s1  6:SSD2s1  7:SSD3s1  8:SSD4s1
    …
    25:SSD5s0 26:SSD6s0 27:SSD7s0 28:SSD8s0 29:SSD5s1 30:SSD6s1 31:SSD7s1 32:SSD8s1
    33:EH15s2 34:EH16s2 35:EH17s2 36:EH18s2 …
    ```

    即 **槽位外层、机柜内层**，且**块按声明顺序拼接**（SSD 的 4 柜×6 槽先走完，
    再走 SSD 的 4 柜×2 槽，再 EHDD1 …）。用 `rack_major` 跑 ch3 只有 17.24%。

    ## 排序键

    1. `block_index` —— 布局块的声明顺序（**这是接线顺序的真正主轴**）
    2. 该块内的槽位集合按槽位升序（槽外层）
    3. 机柜升序（柜内层）

    第 2/3 步通过 (块内槽位序号, 机柜号) 实现：槽位取块内**去重排序后的位次**，
    这样 `{0,1}` 和 `{2,3,4,5}` 两个块能各自从 0 开始编号。

    >>> ns = [
    ...     Placement("a", "A", 3, 1, 3, block_index=0),
    ...     Placement("b", "A", 3, 2, 3, block_index=0),
    ...     Placement("c", "A", 3, 1, 7, block_index=0),
    ... ]
    >>> [p.node_id for p in order_by_slot_major(ns)]
    ['a', 'b', 'c']
    """
    by_block: dict[int, list[Placement]] = {}
    for n in nodes:
        by_block.setdefault(n.block_index, []).append(n)

    ordered: list[Placement] = []
    for bi in sorted(by_block):
        group = by_block[bi]
        slots = sorted({p.slot for p in group})
        slot_rank = {s: i for i, s in enumerate(slots)}
        seen: Counter[tuple[int, int]] = Counter()
        keyed: list[tuple[int, int, int, str, Placement]] = []
        for n in sorted(group, key=lambda p: (p.u_start, p.node_id)):
            # 重复槽位序号必须按 (机柜, 槽位) 计：跨机柜的同槽位是不同位置，不算重复
            occ = seen[(n.rack_no, n.slot)]
            seen[(n.rack_no, n.slot)] += 1
            keyed.append((occ, slot_rank[n.slot], n.rack_no, n.node_id, n))
        keyed.sort(key=lambda t: t[:4])
        ordered.extend(t[4] for t in keyed)
    return tuple(ordered)


def order_nodes(nodes: Sequence[Placement], order: FlowOrder = "rack_major") -> tuple[Placement, ...]:
    """按声明的主轴排布节点。

    >>> ns = [Placement("a", "A", 3, 1, 3), Placement("b", "A", 3, 1, 7)]
    >>> [p.node_id for p in order_nodes(ns, "slot_major")]
    ['a', 'b']
    """
    if order == "slot_major":
        return order_by_slot_major(nodes)
    return order_by_phase_chain(nodes)


def duplicate_slot_racks(nodes: Sequence[Placement]) -> dict[tuple[int, int, int], int]:
    """找出「同一机柜同一槽位出现多台」的数据异常，返回 {(通道, 机柜, slot): 台数}。

    这是**数据体检项**，不是生成逻辑：真实数据里 `rack11 31-32U` 有 2 台，
    而 `rack7` 只有 7 台 —— 总数因此仍是 8 的整数倍。典型成因是某台设备
    上架位置录错机柜。生成时按规则 2 顺延不影响结果，但必须报出来让人核。
    """
    seen: Counter[tuple[int, int, int]] = Counter()
    for n in nodes:
        seen[(n.channel_no, n.rack_no, n.slot)] += 1
    return {k: c for k, c in seen.items() if c > 1}


# ---------------------------------------------------------------------------
# 规则 3：连续切块
# ---------------------------------------------------------------------------
def block_sizes(total: int, switch_count: int, capacity: int) -> list[int]:
    """连续切块的块大小：前 K-1 台各 capacity，末台吸收余量。

    >>> block_sizes(200, 5, 44)
    [44, 44, 44, 44, 24]
    >>> block_sizes(134, 3, 44)
    [44, 44, 46]
    >>> block_sizes(10, 1, 44)
    [10]
    """
    if switch_count <= 0:
        return []
    sizes: list[int] = []
    remaining = total
    for i in range(switch_count):
        if i == switch_count - 1:
            sizes.append(remaining)
        else:
            take = min(capacity, remaining)
            sizes.append(take)
            remaining -= take
    return sizes


def split_into_blocks(
    ordered: Sequence[Placement],
    switch_instances: Sequence[str],
) -> list[tuple[str, tuple[Placement, ...]]]:
    """按容量把有序序列切给各交换机。空块（分不到节点）会被丢弃。

    >>> ps = [Placement(f"n{i}", "A", 2, 1, 3) for i in range(3)]
    >>> [(s, len(ns)) for s, ns in split_into_blocks(ps, ["sw1", "sw2"])]
    [('sw1', 3), ('sw2', 0)]
    """
    sizes = block_sizes(len(ordered), len(switch_instances), DEFAULT_CAPACITY)
    out: list[tuple[str, tuple[Placement, ...]]] = []
    idx = 0
    for sw, size in zip(switch_instances, sizes):
        out.append((sw, tuple(ordered[idx : idx + size])))
        idx += size
    return out


# ---------------------------------------------------------------------------
# 端口名推导
# ---------------------------------------------------------------------------
def derive_port_prefix(nic: str) -> str:
    """由网口标签推导交换机侧端口名前缀。

    实测交换机侧全部是 `{速率}1/0/{n}` 形态（板卡 1、子槽 0）：
    `万兆1` → `10GE1/0`、`千兆1` → `GE1/0`、`25G1` → `25GE1/0`、`100G1` → `100GE1/0`。

    >>> derive_port_prefix("万兆1"), derive_port_prefix("千兆2"), derive_port_prefix("IPMI")
    ('10GE1/0', 'GE1/0', 'GE1/0')
    """
    rate = parse_port_name(nic).rate_mbps
    if rate is None:
        return ""
    if rate >= SPEED_100G:
        return "100GE1/0"
    if rate >= SPEED_25G:
        return "25GE1/0"
    if rate >= SPEED_10G:
        return "10GE1/0"
    if rate >= SPEED_1G:
        return "GE1/0"
    return ""


def default_port_role(nic: str) -> PortRole:
    """带外管理口（IPMI）是 mgmt，其余是下行。"""
    return "mgmt" if parse_port_name(nic).is_mgmt else "downlink"


# ---------------------------------------------------------------------------
# 主入口
# ---------------------------------------------------------------------------
def plan_flow(
    placements: Iterable[Placement],
    routes: Iterable[NicRoute],
    chains: Iterable[SwitchChain],
    *,
    check: bool = True,
) -> FlowPlan:
    """把「布局 + 路由 + 交换机组」展开成逐条端口分配。

    分组键是 `(通道, 网口, 交换机型号)`：同一通道里，凡是声明了
    「这个网口接这个型号交换机」的节点型号，**合并成一个有序序列**再切块。
    这一点很关键——`ch2 万兆1` 前 88 台是计算节点A、后 112 台是计算节点B，
    但它们共用同一串交换机（-1/-3/-5/-7/-9），必须一起排序。
    """
    pool = tuple(placements)
    route_list = tuple(routes)
    # (网口, 交换机型号) → 该路由覆盖的节点型号集合
    nic_targets: dict[tuple[str, str], set[str]] = {}
    role_of: dict[tuple[str, str], PortRole] = {}
    for r in route_list:
        key = (r.nic, r.switch_model)
        nic_targets.setdefault(key, set()).add(r.node_model)
        role_of[key] = r.port_role

    blocks: list[FlowBlock] = []
    assignments: list[PortAssignment] = []
    issues: list[Issue] = []

    for chain in chains:
        ch_nodes = [p for p in pool if p.channel_no == chain.channel_no]
        if not ch_nodes:
            continue
        for (nic, sw_model), models in sorted(nic_targets.items()):
            if sw_model != chain.switch_model:
                continue
            # 链声明了网口就只服务该网口；否则服务所有指向该型号的网口
            if chain.nic and chain.nic != nic:
                continue
            targets = [p for p in ch_nodes if p.model in models]
            if not targets:
                continue

            ordered = order_nodes(targets, chain.order)
            sizes = block_sizes(len(ordered), len(chain.switch_instances), chain.capacity)
            prefix = chain.port_prefix or derive_port_prefix(nic)
            role = role_of.get((nic, sw_model), default_port_role(nic))

            if check:
                for (dup_ch, dup_rack, dup_slot), cnt in sorted(
                    duplicate_slot_racks(targets).items()
                ):
                    issues.append(
                        Issue.warn(
                            IssueCode.RACK_SLOT_CONFLICT,
                            f"ch{dup_ch} rack{dup_rack} 槽位 {dup_slot} 上有 {cnt} 台设备，"
                            f"已按「顺延整轮」规则排位；请核对上架位置是否录错",
                            field="u_start",
                        )
                    )
                expected = sum(sizes)
                if expected != len(ordered):
                    issues.append(
                        Issue.error(
                            IssueCode.PORT_CAPACITY_EXCEEDED,
                            f"ch{chain.channel_no} {nic}→{sw_model}：{len(ordered)} 个节点"
                            f"无法按 {len(chain.switch_instances)} 台×{chain.capacity} 口装完",
                            field="capacity",
                        )
                    )

            idx = 0
            for sw, size in zip(chain.switch_instances, sizes):
                if size <= 0:
                    issues.append(
                        Issue.warn(
                            IssueCode.PORT_UNASSIGNED,
                            f"交换机 {sw} 分不到节点（{len(ordered)} 个节点已由前序交换机吃完）",
                            field="switch_instance",
                        )
                    )
                    continue
                chunk = ordered[idx : idx + size]
                idx += size
                blocks.append(
                    FlowBlock(
                        switch_instance=sw,
                        switch_model=sw_model,
                        channel_no=chain.channel_no,
                        nic=nic,
                        nodes=chunk,
                        port_prefix=prefix,
                        port_role=role,
                    )
                )
                for i, node in enumerate(chunk):
                    port_no = i + 1
                    assignments.append(
                        PortAssignment(
                            block_key=f"{sw_model}|{sw}|{nic}",
                            switch_instance=sw,
                            switch_port=f"{prefix}/{port_no}" if prefix else str(port_no),
                            switch_port_no=port_no,
                            node_id=node.node_id,
                            node_rack=node.rack_key,
                            node_u_start=node.u_start,
                            node_slot=node.slot,
                            index_in_block=i,
                            port_role=role,
                        )
                    )

    # 端口重复占用检查
    if check:
        seen: Counter[tuple[str, str]] = Counter(
            (a.switch_instance, a.switch_port) for a in assignments
        )
        dups = sorted(((sw, port), n) for (sw, port), n in seen.items() if n > 1)
        for (sw, port), n in dups[:20]:
            issues.append(
                Issue.error(
                    IssueCode.PORT_ALREADY_USED,
                    f"交换机 {sw} 的端口 {port} 被分配了 {n} 次",
                    field="switch_port",
                )
            )

    return FlowPlan(tuple(blocks), tuple(assignments), tuple(issues))
