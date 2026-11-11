"""机柜引用与几何快照。

`GeometrySnapshot` 是**不可变快照**：一次计算用一份快照，避免「算到一半平面图变了」
导致同一次批量里两行用了不同几何。这也是增量更新的基础（快照版本变了才重算）。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Iterable, Mapping, Sequence

from ..shared.types import Issue, IssueCode
from .floor import RackPlacement, derive_cmax_by_room


@dataclass(frozen=True, slots=True)
class RackRef:
    """连线表里对机柜的引用。

    真实数据里机柜有三种写法（`rack1` / `A模组-通道-柜号` / 平面图列坐标），
    导入层统一成这个结构；`room_code` 可空，因为大部分行只写了通道+柜号。
    """

    channel_no: int
    seq_no: int
    room_code: str | None = None

    def __str__(self) -> str:
        room = f"{self.room_code}-" if self.room_code else ""
        return f"{room}ch{self.channel_no}/rack{self.seq_no}"


@dataclass(frozen=True, slots=True)
class GeometrySnapshot:
    """一次解析结果的只读投影。"""

    version: str
    racks: Mapping[tuple[str, int, int], RackPlacement]
    cmax_by_room: Mapping[str, int] = field(default_factory=dict)

    # ---------------------------------------------------------------- 查询
    def pos(self, ref: RackRef) -> RackPlacement | None:
        """按引用查位置。`room_code` 为空时若跨房间有重号则返回 None（歧义）。"""
        if ref.room_code is not None:
            return self.racks.get((ref.room_code, ref.channel_no, ref.seq_no))
        hits = [
            p
            for (room, ch, seq), p in self.racks.items()
            if ch == ref.channel_no and seq == ref.seq_no
        ]
        if len(hits) == 1:
            return hits[0]
        return None

    def resolve(self, ref: RackRef) -> tuple[RackPlacement | None, tuple[Issue, ...]]:
        """查位置并给出结构化问题（区分「不存在」与「歧义」）。"""
        if ref.room_code is not None:
            p = self.racks.get((ref.room_code, ref.channel_no, ref.seq_no))
            if p is None:
                return None, (
                    Issue.error(
                        IssueCode.GEOM_MISSING,
                        f"机柜 {ref} 不在平面图中，线长的跨机柜/间隔机柜无法计算",
                        field="rack",
                        rack=str(ref),
                    ),
                )
            return p, ()
        hits = [
            p
            for (room, ch, seq), p in self.racks.items()
            if ch == ref.channel_no and seq == ref.seq_no
        ]
        if len(hits) == 1:
            return hits[0], ()
        if not hits:
            return None, (
                Issue.error(
                    IssueCode.GEOM_MISSING,
                    f"机柜 {ref} 不在平面图中，线长的跨机柜/间隔机柜无法计算",
                    field="rack",
                    rack=str(ref),
                ),
            )
        rooms = "、".join(sorted({p.room_code for p in hits}))
        return None, (
            Issue.error(
                IssueCode.AMBIGUOUS_RACK,
                f"机柜 {ref} 在 {rooms} 都存在，请在连线中补上房间号",
                field="rack",
                rack=str(ref),
                rooms=rooms,
            ),
        )

    def cmax(self, room_code: str) -> int | None:
        return self.cmax_by_room.get(room_code)

    @property
    def room_codes(self) -> tuple[str, ...]:
        return tuple(sorted({p.room_code for p in self.racks.values()}))


def build_snapshot(
    placements: Sequence[RackPlacement],
    *,
    version: str,
    cmax_override: Mapping[str, int] | None = None,
) -> GeometrySnapshot:
    """从机柜位置表构建快照。CMAX 默认由数据推导，可被规则文件覆盖。"""
    racks = {p.key: p for p in placements}
    cmax = derive_cmax_by_room(placements)
    if cmax_override:
        cmax.update({k: v for k, v in cmax_override.items() if v is not None})
    return GeometrySnapshot(version=version, racks=racks, cmax_by_room=cmax)


def racks_of(snapshot: GeometrySnapshot, room_code: str | None = None) -> Iterable[RackPlacement]:
    for p in snapshot.racks.values():
        if room_code is None or p.room_code == room_code:
            yield p
