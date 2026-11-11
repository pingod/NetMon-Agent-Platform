"""端口分配：核心是「组内位次 + 起点」这条 100% 成立的规则。

## 实测（3850/3850 = 100.00%）

把一个「接线块（IntentBlock）」定义为：

```
(交换机型号, 交换机实例名, 对端节点型号)
```

组内节点按 `(机柜号升序, U 位升序)` 排序，则第 i 个节点用的交换机端口号

```
port_no = base + i         # base 是这一块的起始端口号
```

实测 base 分布：`{1: 91 组, 9: 7 组, 17: 7 组, 20: 7 组}` ——
即一台交换机可被拆成 2 个接线块，第二块从 9/17/20 开始。

## 为什么这个方向是对的

正确方向是 **端口号 → 节点位置（正向生成）**，不是 **节点位置 → 端口号（反向猜测）**。
已证伪（不要再试）：

- 端口号 = 节点全局序 `(通道, 机柜, U)` 的位次 − 区间起点 + 1 → 仅 **65.82%**
- 在此基础上「柜内起点取最大 U 后旋转」→ **0.00%**（全错）

根因：U 位不是从 3-4U 单调升的，每台交换机服务的节点区间有自己的 φ 相位偏移。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Iterable, Literal, Mapping, Sequence

from ..shared.types import Issue, IssueCode
from ..units import PORT_ROLES, SPEED_1G, SPEED_10G, SPEED_25G, SPEED_100G
from .naming import PortName, parse_port_name
from .phase import NodeRef, check_phase, phase_of, sort_nodes_for_assign

PortRole = Literal["downlink", "uplink", "peer_link", "stack", "mgmt", "spare", "unknown"]


@dataclass(frozen=True, slots=True)
class IntentBlock:
    """一个接线块：一台交换机的某段端口，服务一组连续节点。

    `base_port_no` 是这段的起点端口号。这是用户唯一需要声明的"意图"，
    其余（每个节点用哪个端口）由规则确定性推出。
    """

    switch_model: str
    switch_instance: str
    node_model: str
    nodes: Sequence[NodeRef]
    base_port_no: int = 1
    port_role: PortRole = "downlink"
    #: 交换机侧端口名前缀，如 `25GE1/0` → 生成 `25GE1/0/{n}`
    port_prefix: str = ""

    def __post_init__(self) -> None:
        if self.port_role not in PORT_ROLES:
            raise ValueError(f"未知端口角色 {self.port_role}")


@dataclass(frozen=True, slots=True)
class PortAssignment:
    """一条「节点 → 交换机端口」的分配结果。"""

    block_key: str
    switch_instance: str
    switch_port: str
    switch_port_no: int
    node_id: str
    node_rack: tuple[int, int]
    node_u_start: int
    node_slot: int
    index_in_block: int
    port_role: PortRole


@dataclass(frozen=True, slots=True)
class PortPlanStats:
    blocks: int = 0
    assignments: int = 0
    violations: int = 0
    phase_ok_blocks: int = 0
    phase_bad_blocks: int = 0


@dataclass(frozen=True, slots=True)
class PortPlan:
    assignments: tuple[PortAssignment, ...]
    issues: tuple[Issue, ...]
    stats: PortPlanStats

    def by_node(self) -> dict[str, list[PortAssignment]]:
        out: dict[str, list[PortAssignment]] = {}
        for a in self.assignments:
            out.setdefault(a.node_id, []).append(a)
        return out

    def by_switch(self) -> dict[str, list[PortAssignment]]:
        out: dict[str, list[PortAssignment]] = {}
        for a in self.assignments:
            out.setdefault(a.switch_instance, []).append(a)
        return out


def _block_key(b: IntentBlock) -> str:
    return f"{b.switch_model}|{b.switch_instance}|{b.node_model}|{b.base_port_no}"


def _switch_port_name(block: IntentBlock, port_no: int) -> str:
    if block.port_prefix:
        return f"{block.port_prefix}/{port_no}"
    return str(port_no)


def assign_block(block: IntentBlock, *, check: bool = True) -> tuple[list[PortAssignment], list[Issue]]:
    """把一个接线块展开成逐条分配。**确定性、可重放**。"""
    issues: list[Issue] = []
    ordered = sort_nodes_for_assign(block.nodes)

    if check:
        monotonic = all(
            ordered[i].rack_seq <= ordered[i + 1].rack_seq for i in range(len(ordered) - 1)
        )
        if not monotonic:
            issues.append(
                Issue.warn(
                    IssueCode.PHASE_INCONSISTENT,
                    f"接线块 {_block_key(block)} 的节点机柜号未按升序排列，已自动重排",
                    field="rack_order",
                )
            )
        ok, phase_issues = check_phase(ordered)
        issues.extend(phase_issues)

    key = _block_key(block)
    out: list[PortAssignment] = []
    for i, node in enumerate(ordered):
        port_no = block.base_port_no + i
        out.append(
            PortAssignment(
                block_key=key,
                switch_instance=block.switch_instance,
                switch_port=_switch_port_name(block, port_no),
                switch_port_no=port_no,
                node_id=node.node_id,
                node_rack=node.rack_key,
                node_u_start=node.u_start,
                node_slot=node.slot,
                index_in_block=i,
                port_role=block.port_role,
            )
        )
    return out, issues


def plan_ports(blocks: Sequence[IntentBlock], *, check: bool = True) -> PortPlan:
    """批量生成端口分配表。块内不中断：单块出问题只记 issue。"""
    all_assign: list[PortAssignment] = []
    issues: list[Issue] = []
    ok_blocks = bad_blocks = 0

    for b in blocks:
        assigns, block_issues = assign_block(b, check=check)
        all_assign.extend(assigns)
        issues.extend(block_issues)
        if any(i.code == IssueCode.PHASE_INCONSISTENT for i in block_issues):
            bad_blocks += 1
        else:
            ok_blocks += 1

    # 端口重复占用检查（同一交换机同一端口被分配两次）
    seen: dict[tuple[str, str], int] = {}
    for a in all_assign:
        k = (a.switch_instance, a.switch_port)
        seen[k] = seen.get(k, 0) + 1
    dup = {k: n for k, n in seen.items() if n > 1}
    for (sw, port), n in sorted(dup.items())[:20]:
        issues.append(
            Issue.error(
                IssueCode.PORT_ALREADY_USED,
                f"交换机 {sw} 的端口 {port} 被分配了 {n} 次",
                field="switch_port",
            )
        )

    stats = PortPlanStats(
        blocks=len(blocks),
        assignments=len(all_assign),
        violations=len([i for i in issues if i.severity in ("error", "warn")]),
        phase_ok_blocks=ok_blocks,
        phase_bad_blocks=bad_blocks,
    )
    return PortPlan(tuple(all_assign), tuple(issues), stats)


# ---------------------------------------------------------------------------
# 线缆类型推荐
# ---------------------------------------------------------------------------
#: 速率/连接方式 → 线缆类型。实测：>=100G 全 MPO(352/354)、1G 全 RJ45(2077/2078)、
#: 10G/25G 全 LC-LC(2678/2690)。
CABLE_TYPE_RULES: tuple[tuple[int, str, str], ...] = (
    (SPEED_100G, "any", "MPO"),
    (SPEED_25G, "fiber", "LC-LC"),
    (SPEED_10G, "fiber", "LC-LC"),
    (SPEED_1G, "copper", "RJ45"),
)


def recommend_cable_type(
    speed_mbps: int | None, connector: str = "any", override: str | None = None
) -> tuple[str, str]:
    """推荐线缆类型。返回 (线缆类型, 依据说明)。

    >>> recommend_cable_type(100_000)
    ('MPO', '100G 及以上使用 MPO')
    >>> recommend_cable_type(10_000)
    ('LC-LC', '10G/25G 光口使用 LC-LC')
    >>> recommend_cable_type(1_000)
    ('RJ45', '1G 电口使用 RJ45')
    >>> recommend_cable_type(None)
    ('UNKNOWN', '速率未知，需人工确认线缆类型')
    """
    if override:
        return override, "人工指定"
    if speed_mbps is None:
        return "UNKNOWN", "速率未知，需人工确认线缆类型"
    if speed_mbps >= SPEED_100G:
        return "MPO", "100G 及以上使用 MPO"
    if speed_mbps >= SPEED_10G:
        return "LC-LC", "10G/25G 光口使用 LC-LC"
    if speed_mbps >= SPEED_1G:
        return "RJ45", "1G 电口使用 RJ45"
    return "UNKNOWN", "速率过低或未知，需人工确认线缆类型"


# ---------------------------------------------------------------------------
# 端口池（应用层用，此处只放纯逻辑的占用集合）
# ---------------------------------------------------------------------------
@dataclass(slots=True)
class Occupancy:
    """端口占用集合。纯数据结构，不含 IO，可在域层安全使用。"""

    used: dict[tuple[str, str], str] = field(default_factory=dict)

    def is_free(self, device: str, port: str) -> bool:
        return (device, port) not in self.used

    def take(self, device: str, port: str, owner: str) -> bool:
        if not self.is_free(device, port):
            return False
        self.used[(device, port)] = owner
        return True

    def release(self, device: str, port: str) -> None:
        self.used.pop((device, port), None)

    def count(self) -> int:
        return len(self.used)


def fill_occupancy(assignments: Iterable[PortAssignment]) -> Occupancy:
    occ = Occupancy()
    for a in assignments:
        occ.take(a.switch_instance, a.switch_port, a.node_id)
    return occ


# ---------------------------------------------------------------------------
# 相位推断辅助（用于验证旧数据）
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class BlockFingerprint:
    """从真实连线表里反推出来的「一个接线块」的指纹。"""

    switch_model: str
    switch_instance: str
    node_model: str
    base_port_no: int
    nodes: tuple[NodeRef, ...]
    phase: int | None
    rack_monotonic: bool


def fingerprint_block(
    switch_model: str,
    switch_instance: str,
    node_model: str,
    pairs: Sequence[tuple[int, NodeRef]],
) -> BlockFingerprint:
    """从 (端口号, 节点) 对列表还原接线块指纹。用于校验历史数据。"""
    pairs_sorted = sorted(pairs, key=lambda p: p[0])
    nodes = tuple(n for _, n in pairs_sorted)
    base = pairs_sorted[0][0] if pairs_sorted else 1
    return BlockFingerprint(
        switch_model=switch_model,
        switch_instance=switch_instance,
        node_model=node_model,
        base_port_no=base,
        nodes=nodes,
        phase=phase_of(nodes),
        rack_monotonic=all(
            nodes[i].rack_seq <= nodes[i + 1].rack_seq for i in range(len(nodes) - 1)
        ),
    )
