"""端口槽位相位模型（模块 8）。

## 实测结论（112/112 组 = 100%）

计算节点的标准上架方式是「8 个 2U 槽位，每 4U 一个」，槽位 0 从 U3 开始：

```
slot_of(u_start) = (u_start - 3) // 4      # U3→0, U7→1, U11→2, ... U31→7
```

给定一台交换机服务的节点列表，按端口号升序排列后，**节点 U 位的槽位值严格满足**

```
slot_i ≡ (i + φ) mod 8
```

且 `φ == slot_of(组内第一个节点的 U 位)` —— 即 φ 不是独立参数，由首节点位置直接决定。
实测首节点 slot=0 → φ=0（70 组）、=3 → φ=3（7 组）、=4 → φ=4（35 组）。

## 这条规则为什么是产品护城河

它是**自动发现人工排错**的检测器：如果用户给的节点列表有重叠、漏项或顺序错，
φ 就不唯一，系统立刻能指出「这 44 个节点里有 3 个不满足相位规律」。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Sequence

from ..shared.types import Issue, IssueCode
from ..units import SLOT_BASE_U, SLOT_COUNT, SLOT_PITCH_U


def slot_of(u_start: int) -> int:
    """U 位 → 槽位号（0..7）。非标准 U 位会被夹到合法区间并保持单调。

    >>> [slot_of(u) for u in (3, 7, 11, 15, 19, 23, 27, 31)]
    [0, 1, 2, 3, 4, 5, 6, 7]
    """
    raw = (u_start - SLOT_BASE_U) // SLOT_PITCH_U
    return raw % SLOT_COUNT


@dataclass(frozen=True, slots=True)
class NodeRef:
    """一个待接线的节点（服务器）。只需位置与身份。"""

    node_id: str
    rack_channel: int
    rack_seq: int
    u_start: int

    @property
    def rack_key(self) -> tuple[int, int]:
        return (self.rack_channel, self.rack_seq)

    @property
    def slot(self) -> int:
        return slot_of(self.u_start)


def phase_of(nodes_by_port: Sequence[NodeRef]) -> int | None:
    """求相位 φ。所有节点算出同一个 φ 才算一致；否则 None。

    >>> ns = [NodeRef(f"n{i}", 2, 2, 3 + 4 * i) for i in range(4)]
    >>> phase_of(ns)
    0
    """
    if not nodes_by_port:
        return None
    phis = {(n.slot - i) % SLOT_COUNT for i, n in enumerate(nodes_by_port)}
    return phis.pop() if len(phis) == 1 else None


def expected_slot(i: int, phase: int) -> int:
    """第 i 个端口应接的节点槽位。"""
    return (i + phase) % SLOT_COUNT


def diagnose_phase(nodes_by_port: Sequence[NodeRef]) -> list[dict[str, object]]:
    """逐节点列出「实际槽位 vs 期望槽位」，供 UI 直接渲染不合规清单。"""
    if not nodes_by_port:
        return []
    phase = slot_of(nodes_by_port[0].u_start)
    out: list[dict[str, object]] = []
    for i, n in enumerate(nodes_by_port):
        exp = expected_slot(i, phase)
        if n.slot != exp:
            out.append(
                {
                    "index": i,
                    "node_id": n.node_id,
                    "rack": f"ch{n.rack_channel}/rack{n.rack_seq}",
                    "u_start": n.u_start,
                    "actual_slot": n.slot,
                    "expected_slot": exp,
                }
            )
    return out


def check_phase(nodes_by_port: Sequence[NodeRef]) -> tuple[bool, tuple[Issue, ...]]:
    """校验相位。返回 (是否合规, 问题清单)。"""
    if not nodes_by_port:
        return True, ()
    detail = diagnose_phase(nodes_by_port)
    if not detail:
        return True, ()
    sample = "、".join(
        f"第{d['index'] + 1}个({d['node_id']}) 实槽{d['actual_slot']} 应槽{d['expected_slot']}"
        for d in detail[:5]
    )
    more = f" 等 {len(detail)} 个" if len(detail) > 5 else ""
    return False, (
        Issue.warn(
            IssueCode.PHASE_INCONSISTENT,
            f"节点顺序与 U 位槽位规律不符：{sample}{more}。"
            f"通常是节点列表漏项、重叠或排序用了 U 位倒序",
            field="phase",
            violations=len(detail),
        ),
    )


def sort_nodes_for_assign(nodes: Sequence[NodeRef]) -> list[NodeRef]:
    """生成时的排序规则 `rack_then_u`：机柜号升序、同柜 U 位升序。

    实测 112/112 组的机柜号严格单调不减，且组内节点区间连续。
    """
    return sorted(nodes, key=lambda n: (n.rack_seq, n.u_start))


def check_rack_monotonic(nodes_by_port: Sequence[NodeRef]) -> tuple[bool, tuple[Issue, ...]]:
    """校验组内机柜号单调不减（C5）。"""
    racks = [n.rack_seq for n in nodes_by_port]
    for i in range(1, len(racks)):
        if racks[i] < racks[i - 1]:
            return False, (
                Issue.warn(
                    IssueCode.PHASE_INCONSISTENT,
                    f"第 {i + 1} 个节点的机柜号 {racks[i]} 小于前一个 {racks[i - 1]}，"
                    f"接线顺序应为机柜号升序",
                    field="rack_order",
                ),
            )
    return True, ()
