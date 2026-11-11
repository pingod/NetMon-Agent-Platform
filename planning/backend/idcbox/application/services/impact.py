"""拖动预演：这次改动会动到哪些行、多少根线的长度变（应用层）。

## 为什么需要这一层

`moveu.py` 只管「能不能挪」，`gen_diff.py` 只管「两次生成结果差在哪」。
但界面要回答的是更完整的一句话：

> 把通道 2 机柜 1 的这台挪到 5 号柜，**会改掉 91 行端口号和 6 行线长，
> 涉及 7 台交换机**，确定吗？

第三项（线长）需要几何快照，而几何是应用层才有的东西。所以编排在这里。

## 实测基线（V13 真数据，可复现）

| 动作 | 端口重编号 | 位置变化 | 线长变化 |
|---|---|---|---|
| 柜内 3U → 5U（同槽位） | 0 | 7 | 0 |
| 柜内 3U → 35U（跨出 8 槽位） | 0 | 7 | 0 |
| 柜内 3U → 39U（落在第 10 槽） | 56 行 / 7 台 | 7 | 6 |
| **跨柜 机柜1 → 机柜5** | **91 行 / 7 台** | 7 | 6 |

## 「柜内挪就不影响端口」是个错觉

看起来柜内拖动最安全，但 `3U → 39U` 一样改了 56 行 —— 因为端口位次按
`(机柜, U 位)` 排，在同一个柜里换个 U 也可能换位次。所以**任何拖动都要算影响面**，
不能按「同柜 = 安全」跳过。

## 为什么线长要单独重算，不能从 diff 里推

`T` 是阶梯值，两端位置都变了才可能变，但也可能变了位置而档位不变
（实测 7 行位置变、只有 6 行档位变）。用「位置变了 ⇒ 线长变了」代替会多报，
而用户看到「有 1 行线长会变」却发现没变，就会开始怀疑整个提示。

## 性能

全量重算一次 122ms（生成 33 + 桥 13 + 算长 74，实测中位数），
两次正好卡在 NFR-1 的 200ms 增量预算边缘。所以：
**同一份 doc 的生成结果由调用方复用**（`baseline` 参数），只重算改动后的那一份。
界面传 `doc` 时服务端每次都要两次全量 —— 这就是为什么这个端点默认
`deep=false`（只比端口与位置，不算线长），要线长时显式 `deep=true`。
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any, Mapping

from ...domain.cable_length.engine import compute_link
from ...domain.cable_length.rules import DEFAULT_RULE, LengthRule
from ...domain.geometry.layout import GeometrySnapshot
from ...domain.ports.bridge import to_link_input
from ...domain.ports.gen_diff import GenerationDiff, diff_generation
from ...domain.ports.generate import GeneratedCabling, generate_links
from ...domain.ports.intent import intent_from_dict
from ...domain.ports.moveu import BatchMoveResult, DeviceTarget, MoveResult, move_device, move_devices


@dataclass(frozen=True, slots=True)
class LengthChange:
    """一根线的线长档位变化。"""

    switch: str
    node: str
    nic: str
    before_m: int
    after_m: int

    @property
    def delta(self) -> int:
        return self.after_m - self.before_m


@dataclass(frozen=True, slots=True)
class ImpactReport:
    """一次拖动的完整影响面。"""

    ok: bool
    error: str | None
    #: 改完的文档（`ok=False` 时是传入的原文档）
    doc: dict[str, Any]
    diff: GenerationDiff
    #: 线长档位变化（`deep=False` 时为空，`lengthChecked` 为 False）
    length_changes: tuple[LengthChange, ...] = ()
    #: 线长到底算没算 —— 界面必须能区分「没变」和「没算」
    length_checked: bool = False
    #: 移动本身的结果（失败原因、重叠、U 跨度…）
    move: MoveResult | BatchMoveResult | None = None

    @property
    def port_rows(self) -> int:
        return len(self.diff.port_changes)

    @property
    def position_rows(self) -> int:
        return len(self.diff.position_changes)

    @property
    def length_rows(self) -> int:
        return len(self.length_changes)

    @property
    def has_hidden_impact(self) -> bool:
        """除了「设备自己挪了」之外，还动了别的行。

        这是界面必须弹提示的判据：只挪自己（`position_rows == 1` 且
        无端口/长度变化）时一句「已移动」就够；一旦动了别人的行，
        用户必须在保存前知道。
        """
        return self.port_rows > 0 or self.length_rows > 0 or len(self.diff.added) > 0 or len(self.diff.removed) > 0


def _cabling_of(doc: Mapping[str, Any]) -> GeneratedCabling:
    return generate_links(intent_from_dict(dict(doc)))


def _length_changes(
    diff: GenerationDiff,
    before: GeneratedCabling,
    after: GeneratedCabling,
    geometry: GeometrySnapshot,
    rule: LengthRule,
) -> tuple[LengthChange, ...]:
    """按 `gen_diff` 配对出来的「位置变了的行」逐对重算。

    只重算位置变了的行 —— 端口重编号不改变两端机柜坐标，
    N/O 只看机柜 → 长度不可能变。这是 `moveu.py` 已经证明的前提
    （同柜 path_units = 0），这里用它省掉 90% 的重算。
    """
    if geometry is None:
        return ()

    # 位置变了的行才需要看长度。按身份键 + 配对顺序回查两边原始行。
    wanted = [d for d in diff.changed if d.position_changed]
    if not wanted:
        return ()

    def index(cabling: GeneratedCabling) -> dict[tuple[str, str, str, str], list]:
        out: dict[tuple[str, str, str, str], list] = {}
        for link in cabling.links:
            key = (link.a_device, link.b_id or link.b_device or link.b_model, link.nic, link.b_port)
            out.setdefault(key, []).append(link)
        for items in out.values():
            items.sort(key=lambda l: (l.a_channel, l.a_rack, l.b_channel, l.b_rack, l.a_u, l.b_u, l.a_port))
        return out

    b_idx = index(before)
    a_idx = index(after)

    out: list[LengthChange] = []
    for delta in wanted:
        key = delta.identity.as_tuple()
        b_links = b_idx.get(key) or []
        a_links = a_idx.get(key) or []
        if not b_links or not a_links:
            continue
        # 与 `gen_diff._pair_key` 同一顺序，保证配对一致
        for b_link, a_link in zip(b_links, a_links):
            rb = compute_link(to_link_input(b_link), rule, geometry)
            ra = compute_link(to_link_input(a_link), rule, geometry)
            if rb.t_ladder_m is None or ra.t_ladder_m is None:
                continue
            if rb.t_ladder_m != ra.t_ladder_m:
                out.append(
                    LengthChange(
                        switch=delta.identity.switch,
                        # 在**源头**就翻掉，不要等到摊 payload 时再翻 ——
                        # 两处翻译迟早会漏掉一处（`lengthSamples` 与
                        # `portSamples` 各走一条代码路径）。
                        node=_node_label(delta),
                        nic=delta.identity.nic,
                        before_m=rb.t_ladder_m,
                        after_m=ra.t_ladder_m,
                    )
                )
    return tuple(out)


def _failed(
    move: MoveResult | BatchMoveResult, doc: Mapping[str, Any]
) -> "ImpactReport":
    """移动被拒绝时的影响面：**空 diff**，不是「全表被删」。

    ## 这个分支出过一个会吓人的 bug

    最早写成 `diff_generation(before, ())`，意思是「改动后的结果是空的」。
    于是移动失败时接口返回 `removed: 5127` —— 界面会显示
    「这次改动删除了 5127 行」，外加一条「U7-8 已被占用」的失败原因。
    用户看到的是最坏的消息配一个失败的动作，完全对不上。

    失败的正确语义是**什么都没发生**：`diff` 全空，原因只由 `error` 说。
    """
    return ImpactReport(
        ok=False,
        error=move.error,
        doc=dict(doc),
        diff=GenerationDiff(),
        length_checked=False,
        move=move,
    )


def _report(
    *,
    before: GeneratedCabling,
    after: GeneratedCabling,
    doc: dict[str, Any],
    geometry: GeometrySnapshot | None,
    deep: bool,
    rule: LengthRule,
    move: MoveResult | BatchMoveResult,
) -> ImpactReport:
    diff = diff_generation(before, after)
    changes: tuple[LengthChange, ...] = ()
    checked = False
    if deep and geometry is not None:
        changes = _length_changes(diff, before, after, geometry, rule)
        checked = True
    return ImpactReport(
        ok=bool(move.ok),
        error=move.error,
        doc=doc,
        diff=diff,
        length_changes=changes,
        length_checked=checked,
        move=move,
    )


# ---------------------------------------------------------------------------
# 单台
# ---------------------------------------------------------------------------
def preview_move(
    doc: Mapping[str, Any],
    *,
    channel_no: int,
    rack_no: int,
    instance: str,
    new_u: int,
    allow_overlap: bool = False,
    current_u: int | None = None,
    target_rack_no: int | None = None,
    geometry: GeometrySnapshot | None = None,
    deep: bool = True,
    baseline: GeneratedCabling | None = None,
    rule: LengthRule = DEFAULT_RULE,
) -> ImpactReport:
    """预演一次单台移动。**不落库、不接受半改**：失败时 `doc` 原样返回。"""
    before = baseline if baseline is not None else _cabling_of(doc)
    move = move_device(
        dict(doc),
        channel_no=channel_no,
        rack_no=rack_no,
        instance=instance,
        new_u=new_u,
        allow_overlap=allow_overlap,
        current_u=current_u,
        target_rack_no=target_rack_no,
    )
    if not move.ok:
        return _failed(move, doc)
    after = _cabling_of(move.doc)
    return _report(
        before=before,
        after=after,
        doc=move.doc,
        geometry=geometry,
        deep=deep,
        rule=rule,
        move=move,
    )


# ---------------------------------------------------------------------------
# 批量
# ---------------------------------------------------------------------------
def preview_move_devices(
    doc: Mapping[str, Any],
    *,
    targets: list[DeviceTarget],
    delta_u: int,
    target_rack_no: int | None = None,
    allow_overlap: bool = False,
    geometry: GeometrySnapshot | None = None,
    deep: bool = True,
    baseline: GeneratedCabling | None = None,
    rule: LengthRule = DEFAULT_RULE,
) -> ImpactReport:
    """预演一次批量平移。失败的语义与单台一致：`doc` 原样返回。"""
    before = baseline if baseline is not None else _cabling_of(doc)
    move = move_devices(
        dict(doc),
        targets=targets,
        delta_u=delta_u,
        target_rack_no=target_rack_no,
        allow_overlap=allow_overlap,
    )
    if not move.ok:
        return _failed(move, doc)
    after = _cabling_of(move.doc)
    return _report(
        before=before,
        after=after,
        doc=move.doc,
        geometry=geometry,
        deep=deep,
        rule=rule,
        move=move,
    )


#: 布局段生成的槽位 id，形如 `N02-01-S0` 或 `N02-11-S7-1`。
#:
#: ## 为什么要在影响面里把它翻掉
#:
#: 这些是**程序内部 id**，机器上没有任何东西写着它们。影响面横幅是给
#: 「决定要不要保存这次拖动」的人看的，而真正去插线的是另一个人 ——
#: 告诉他「N02-01-S6 的端口变了」，他在现场没有任何办法找到那台机器。
#:
#: 同一个判据在 `labelsvc.py` 里也有一份（那里是防止槽位号漏进贴纸）。
#: 两处都需要的理由相同：**内部 id 一旦流到人眼前，就一定会有人照着它做事**。
_SLOT_ID_RE = re.compile(r"^N\d{2}-(\d{2})-S(\d)(?:-(\d))?$")


def display_device(node: str, model: str, u: str, rack: int) -> str:
    """把对端设备翻成现场能对上号的写法。

    槽位 id → `型号 机柜N 27-28U`（C1 会话里型号 + 位置足够定位）；
    普通设备名（交换机等）原样返回 —— 那些名字本来就印在设备上。
    """
    m = _SLOT_ID_RE.match(node)
    if not m:
        return node
    _rack_in_id, _slot, _dup = m.groups()
    pieces = [model] if model else []
    pieces.append(f"机柜{rack}")
    if u:
        pieces.append(u)
    return " ".join(pieces)


def _node_label(delta: Any) -> str:
    """一条变化里对端设备的显示名。取「改后」优先，「改前」兜底。"""
    snap = delta.after or delta.before
    if snap is None:
        return delta.identity.node
    return display_device(delta.identity.node, snap.b_model, snap.b_u, snap.b_rack)


def impact_payload(report: ImpactReport, *, limit: int = 20) -> dict[str, Any]:
    """把 `ImpactReport` 摊成界面能直接吃的 JSON。

    ## 为什么把「增删」与「改」分三块给

    界面要讲的话不同：增删 = 「接线变了」，改端口 = 「同一根线换了口」。
    混成一个数字，用户没法判断要不要停下来重新设计。
    """
    diff = report.diff
    return {
        "ok": report.ok,
        "error": report.error,
        #: 除设备自己之外还动了多少行 —— 界面靠它决定要不要弹提示
        "hasHiddenImpact": report.has_hidden_impact,
        "counts": {
            "added": len(diff.added),
            "removed": len(diff.removed),
            "portChanges": report.port_rows,
            "positionChanges": report.position_rows,
            "lengthChanges": report.length_rows,
            "affectedSwitches": len(diff.affected_switches),
            "total": diff.total,
        },
        #: 线长到底算没算。`lengthChecked=false` 时 `lengthChanges=0` 的含义是
        #: 「没算」而不是「没变」—— 界面不能把两者说成同一句话。
        "lengthChecked": report.length_checked,
        "affectedSwitches": list(diff.affected_switches[:limit]),
        "affectedRacks": [{"channel": ch, "rack": rk} for ch, rk in diff.affected_racks[:limit]],
        "portSamples": [
            {
                "switch": d.identity.switch,
                "node": _node_label(d),
                "nic": d.identity.nic,
                "before": d.before.a_port if d.before else None,
                "after": d.after.a_port if d.after else None,
            }
            for d in diff.port_changes[:limit]
        ],
        "lengthSamples": [
            {
                "switch": c.switch,
                "node": c.node,
                "nic": c.nic,
                "before": c.before_m,
                "after": c.after_m,
                "delta": c.delta,
            }
            for c in report.length_changes[:limit]
        ],
        "addedSamples": [
            {
                "switch": d.identity.switch,
                "node": _node_label(d),
                "port": d.after.a_port if d.after else None,
                "peerPort": d.identity.peer_port,
            }
            for d in diff.added[:limit]
        ],
        "removedSamples": [
            {
                "switch": d.identity.switch,
                "node": _node_label(d),
                "port": d.before.a_port if d.before else None,
                "peerPort": d.identity.peer_port,
            }
            for d in diff.removed[:limit]
        ],
        "doc": report.doc,
    }


__all__ = [
    "ImpactReport",
    "LengthChange",
    "impact_payload",
    "preview_move",
    "preview_move_devices",
]
