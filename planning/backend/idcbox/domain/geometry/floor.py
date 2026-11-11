"""平面图解析：把「机房平面图」sheet 的单元格网格变成机柜物理坐标。

## 为什么必须从平面图解析，而不是从柜号算

403 房间通道 1 的北侧柜号**自右向左递减**（列 38 = 柜 1，列 23 = 柜 13），
南侧自左向右递增（列 23 = 柜 14，列 38 = 柜 27）。也就是说柜 1 与柜 27
是面对面的邻居，`|柜号差|` 会算出 26 而真实横向距离是 0。

实测证据：`O = |柜号差|` 只命中 36.0%，而 `O = |列坐标差|` 命中 93.1%。
所以「柜号」只是标签，**列坐标才是物理位置**。同理 N 必须用平面图行号差。

## 本模块的边界

域层零 IO：这里只接收**已经读出来的单元格值**（`FloorCell`），不碰 openpyxl。
读取由 `idcbox.infrastructure.xlsx.openpyxl_reader` 负责。
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Final, Iterable, Literal, Sequence

from ..shared.types import Issue, IssueCode
from ..units import FLOOR_FIRST_ROW, FLOOR_ROW_STRIDE, ROOM_OF_CHANNEL

Side = Literal["N", "S"]

#: 平面图里除机柜以外的占位符号，原样保留以支持「该位置不是机柜」告警
NON_RACK_MARKERS: Final[tuple[str, ...]] = (
    "空调",
    "柱子",
    "走道",
    "管道",
    "间",
)

_DIGITS = re.compile(r"^\d+$")


def floor_row(channel_no: int, side: Side) -> int:
    """通道 + 侧别 → 平面图行号。

    每条通道占两行：北侧在上、南侧在下。通道号在房间内 1..7 循环
    （403 房 1..7，404 房 8..14），故取 `(channel-1) % 7` 作行序。

    >>> floor_row(1, "N"), floor_row(1, "S"), floor_row(2, "N")
    (3, 4, 5)
    >>> floor_row(8, "N"), floor_row(14, "S")
    (3, 16)
    """
    idx = (channel_no - 1) % 7
    return FLOOR_FIRST_ROW + FLOOR_ROW_STRIDE * idx + (0 if side == "N" else 1)


def floor_room_of(channel_no: int) -> str:
    """通道号 → 房间号。未知通道直接 KeyError（配置错误应当早失败）。"""
    return ROOM_OF_CHANNEL[channel_no]


@dataclass(frozen=True, slots=True)
class FloorCell:
    """平面图里的一个单元格。`raw` 是原值（可能是柜号、也可能是「空调」）。"""

    row: int
    col: int
    raw: object

    @property
    def text(self) -> str:
        return "" if self.raw is None else str(self.raw).strip()

    @property
    def seq_no(self) -> int | None:
        """是机柜则返回柜号，否则 None。"""
        t = self.text
        return int(t) if _DIGITS.match(t) else None


@dataclass(frozen=True, slots=True)
class RackPlacement:
    """一台机柜的物理位置。这是几何层的最小单元。"""

    room_code: str
    channel_no: int
    seq_no: int
    side: Side
    col: int
    row: int

    @property
    def key(self) -> tuple[str, int, int]:
        return (self.room_code, self.channel_no, self.seq_no)


@dataclass(frozen=True, slots=True)
class NonRackSlot:
    """不是机柜的位置（空调 / 柱子 / 走道 / 管道间）。"""

    room_code: str
    channel_no: int
    side: Side
    col: int
    row: int
    marker: str


@dataclass(frozen=True, slots=True)
class FloorParseResult:
    racks: tuple[RackPlacement, ...]
    non_racks: tuple[NonRackSlot, ...]
    issues: tuple[Issue, ...]

    @property
    def by_key(self) -> dict[tuple[str, int, int], RackPlacement]:
        return {p.key: p for p in self.racks}


@dataclass(frozen=True, slots=True)
class RoomLayout:
    """一个房间的物理布局规格（新机房只需改这份配置，不动代码）。"""

    room_code: str
    channels: tuple[int, ...]
    col_range: tuple[int, int]


#: 信息港实测布局。换机房时必须重新标定，这是 v1.0 通用化的首要验证项。
V13_LAYOUT: Final[tuple[RoomLayout, ...]] = (
    RoomLayout("403", tuple(range(1, 8)), (23, 38)),
    RoomLayout("404", tuple(range(8, 15)), (3, 18)),
)


def _marker_kind(text: str) -> str:
    for m in NON_RACK_MARKERS:
        if m in text:
            return m
    return text


def parse_floor(
    cells: Iterable[FloorCell],
    layout: Sequence[RoomLayout] = V13_LAYOUT,
) -> FloorParseResult:
    """把平面图单元格解析成机柜位置表。

    只保留落在 `layout` 声明范围内的单元格，其余（图例、标题、尺寸标注）忽略。
    同一 `(room, channel, seq)` 出现两次视为布局配置错误 —— 但**不抛异常**，
    因为 5113 行导入不能因为一张图坏掉就整批失败。
    """
    # ★ 两个房间的通道占用**同一批行号**（都从第 3 行开始），只能靠列坐标区分房间。
    #   故先按列范围判房间，再按行号判通道。
    col_bound: list[tuple[int, int, str]] = [
        (r.col_range[0], r.col_range[1], r.room_code) for r in layout
    ]
    room_rows: dict[str, dict[int, tuple[int, Side]]] = {}
    for room in layout:
        m: dict[int, tuple[int, Side]] = {}
        for ch in room.channels:
            for side in ("N", "S"):
                m[floor_row(ch, side)] = (ch, side)  # type: ignore[assignment]
        room_rows[room.room_code] = m

    def locate(row: int, col: int) -> tuple[str, int, Side] | None:
        room_code = next((rc for lo, hi, rc in col_bound if lo <= col <= hi), None)
        if room_code is None:
            return None
        ch_side = room_rows[room_code].get(row)
        if ch_side is None:
            return None
        return (room_code, ch_side[0], ch_side[1])

    racks: list[RackPlacement] = []
    non_racks: list[NonRackSlot] = []
    issues: list[Issue] = []
    seen: dict[tuple[str, int, int], RackPlacement] = {}

    for c in cells:
        loc = locate(c.row, c.col)
        if loc is None:
            continue
        room, ch, side = loc
        seq = c.seq_no
        if seq is None:
            text = c.text
            if text:
                non_racks.append(NonRackSlot(room, ch, side, c.col, c.row, _marker_kind(text)))
            continue
        p = RackPlacement(room, ch, seq, side, c.col, c.row)
        prev = seen.get(p.key)
        if prev is not None:
            issues.append(
                Issue.warn(
                    IssueCode.AMBIGUOUS_RACK,
                    f"通道 {ch} 出现了两次柜号 {seq}（列 {prev.col} 与列 {c.col}），"
                    f"后出现的已忽略，请核对平面图",
                    field="seq_no",
                    room=room,
                    channel_no=ch,
                    seq_no=seq,
                    first_col=prev.col,
                    second_col=c.col,
                )
            )
            continue
        seen[p.key] = p
        racks.append(p)

    racks.sort(key=lambda p: (p.room_code, p.channel_no, p.side, p.col))
    return FloorParseResult(tuple(racks), tuple(non_racks), tuple(issues))


def cmax_of(racks: Sequence[RackPlacement], room_code: str) -> int | None:
    """房间的 CMAX = 该房间机柜列坐标最大值 = 排端走线架入口列。

    实测 403 → 38、404 → 18。CMAX 是 O 的跨排公式的一等输入，
    **不得硬编码进公式**（换机房就变）。
    """
    cols = [p.col for p in racks if p.room_code == room_code]
    return max(cols) if cols else None


def derive_cmax_by_room(racks: Sequence[RackPlacement]) -> dict[str, int]:
    rooms = sorted({p.room_code for p in racks})
    out: dict[str, int] = {}
    for r in rooms:
        m = cmax_of(racks, r)
        if m is not None:
            out[r] = m
    return out
