"""单条连线的输入结构。

刻意独立于数据库实体：域层不知道 `cable` 表长什么样，只知道「一条线有两个端点、
一个线缆类型、可能已经有人工核定的长度」。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Mapping

from ..geometry.layout import RackRef


@dataclass(frozen=True, slots=True)
class Endpoint:
    """连线的一端。真实数据里一端是交换机（有 CLI 端口名），一端是服务器（有标签名）。"""

    rack: RackRef
    device_name: str
    port_name: str
    u_position: str | None = None
    device_model: str | None = None

    def __str__(self) -> str:
        return f"{self.device_name}:{self.port_name}@{self.rack}"


@dataclass(frozen=True, slots=True)
class LinkInput:
    """一条待算长度的连线。

    字段与 V13 布线表 25 列对齐，但只保留**算长度需要的**部分。
    `source_row_no` 用于导出时稳定排序与逐行 diff。
    """

    stable_key: str
    a: Endpoint
    b: Endpoint
    cable_type: str
    source_row_no: int | None = None
    #: 存量原值（人工填的 U）。IMPORTED 策略下必须原样保真。
    imported_len_u: int | None = None
    #: 存量原值的 sentinel 文本（`待定` 等），导出时必须原样输出
    imported_len_raw: str | None = None
    #: 备注列原值
    remark: str | None = None
    #: 阶段/批次标记（原表备注1 = `首期`）
    phase: str | None = None
    #: 导入时发现的、需要原样保留的其他列（本端模块/对端模块/插槽等）
    extra: Mapping[str, object] | None = None

    @property
    def same_rack(self) -> bool:
        return (
            self.a.rack.channel_no == self.b.rack.channel_no
            and self.a.rack.seq_no == self.b.rack.seq_no
            and (self.a.rack.room_code or None) == (self.b.rack.room_code or None)
        )

    def reversed_same_physical_rack(self) -> bool:
        """两端是不是同一个物理机柜（忽略 room 缺省的差异）。"""
        if self.a.rack.channel_no != self.b.rack.channel_no:
            return False
        if self.a.rack.seq_no != self.b.rack.seq_no:
            return False
        ra, rb = self.a.rack.room_code, self.b.rack.room_code
        return ra is None or rb is None or ra == rb
