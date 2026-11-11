"""几何层：平面图 → 机柜物理坐标。"""

from .floor import (
    RackPlacement,
    FloorCell,
    NonRackSlot,
    FloorParseResult,
    RoomLayout,
    V13_LAYOUT,
    parse_floor,
    floor_row,
    floor_room_of,
    cmax_of,
    derive_cmax_by_room,
)
from .layout import RackRef, GeometrySnapshot, build_snapshot, racks_of

__all__ = [
    "RackPlacement",
    "FloorCell",
    "NonRackSlot",
    "FloorParseResult",
    "RoomLayout",
    "V13_LAYOUT",
    "parse_floor",
    "floor_row",
    "floor_room_of",
    "cmax_of",
    "derive_cmax_by_room",
    "RackRef",
    "GeometrySnapshot",
    "build_snapshot",
    "racks_of",
]
