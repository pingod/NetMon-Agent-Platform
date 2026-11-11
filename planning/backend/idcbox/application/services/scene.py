"""3D 场景数据（应用层）—— 一次给全房间/机柜/设备，供 WebGL 渲染。

## 为什么单独一个载荷，不复用 `/api/floor` + `/api/racks`

3D 与 2D 的**取数形状不同**，而这个差别是本质的：

| | 2D 需要 | 3D 需要 |
|---|---|---|
| 机柜 | 一个格子（`col/row`） | 一个**实心盒子**的位置 + 朝向 |
| 设备 | 点进某个柜才要 | **一次要全部 824 台**（要算 instance 矩阵） |
| 通道 | 可忽略 | 决定排的间距与通道净宽 |

若让 3D 复用 2D 形状，它得为 102 个有设备的机柜各发一次
`/api/racks/{ch}/{seq}` —— 启动就 102 个请求，且每次都要等。
反过来让 2D 吃 3D 形状，它要多解析 824 台设备才画出 102 个格子。

所以两个形状并存，**但几何真值只有一个**（`domain/geometry/`）。
这个模块只做「把同一份真值投影成 3D 能直接吃的形状」，不做任何几何推导。

## 坐标口径：毫米，且只有一处换算

数据里所有长度都是**整数毫米**。像素/世界单位都是渲染时算的派生量。
这条规矩来自 `docs/research/02-visualization-stack.md` §4.1：

- 42U 柜高 = 42 × 44.45 = 1866.9mm（EIA-310 标准 1U），加框才是 2000mm
- 柜宽 600mm（= 布线表 R 列 `0.6m/柜位` 的同一个事实）
- 通道净宽 1200mm（实测约 1.2m）

**为什么不能给像素**：一旦像素进数据模型，导出 SVG 缩放、打印 300dpi、
3D 世界单位就各自乱掉。所以这里给 mm，前端 `mmToWorld()` 一次转完。

## 为什么 `room` 要算成矩形

真实平面图是网格（列 3..38、行 3..16），而网格的**行列不连续**
（403 房只用列 23..38，404 房只用列 3..18）。3D 里画一面 36 列宽的墙
会把两个房间糊成一个 —— 所以按房间各自求包围盒。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from ...domain.exporting.svg import categorize
from .views import DEFAULT_RACK_HEIGHT_U, ProjectViews

#: EIA-310 标准 1U = 1.75 inch = 44.45mm。**这是硬标准，不是可调参数。**
MM_PER_U = 44.45

#: 柜体尺寸（mm）。600 宽来自布线表 R 列的 `0.6m/柜位`——**与线长公式同源**，
#: 不是另拍的数。改这里等于改线长公式，必须同步 `domain/units.py`。
CABINET_WIDTH_MM = 600
CABINET_HEIGHT_MM = 2000
CABINET_DEPTH_MM = 1200

#: 通道净宽：冷/热通道中心线到两侧柜面的距离（实测约 1.2m）
AISLE_WIDTH_MM = 1200

#: 平面图一格 = 一个机柜位 = 一个柜宽（紧排无间隙）
CELL_MM = CABINET_WIDTH_MM


@dataclass(frozen=True, slots=True)
class CabinetBox:
    """一个机柜在 3D 里的位置与朝向。"""

    channel_no: int
    rack_no: int
    #: 机柜中心的世界坐标（mm），y 为高度
    x: float
    y: float
    z: float
    #: 朝向角（度，绕 y 轴）。0 = 柜门朝 +z（北侧柜朝通道），180 = 朝 -z
    rotation_y: float
    device_count: int
    used_u: int


def _room_bounds(views: ProjectViews) -> dict[str, dict[str, Any]]:
    """每个房间的包围盒（以格子为单位）。"""
    out: dict[str, dict[str, Any]] = {}
    for (room, _ch, _seq), pos in views.project.geometry.racks.items():
        r = out.setdefault(room, {"minCol": pos.col, "maxCol": pos.col,
                                  "minRow": pos.row, "maxRow": pos.row})
        r["minCol"] = min(r["minCol"], pos.col)
        r["maxCol"] = max(r["maxCol"], pos.col)
        r["minRow"] = min(r["minRow"], pos.row)
        r["maxRow"] = max(r["maxRow"], pos.row)
    return out


def _room_x_origins(bounds: dict[str, dict[str, Any]]) -> dict[str, float]:
    """每个房间在世界坐标里的 x 起点。

    ## 为什么必须给房间偏移（否则两个房间完全重叠）

    403 房用列 23..38、404 房用列 3..18，**两个房间的列号各自独立**。
    若都按「减本房间最左列」算 x，两间房的柜会得到**完全相同**的
    世界坐标 —— 3D 里表现为「一个房间，柜数是实际的两倍」，两个房间的
    设备也混在一起。实测不修的话 362 个柜里 146 个坐标重复。

    真实平面图上两间房是左右并排的两个区域，所以按房间号的字典序横向排开，
    中间留一条 `AISLE_WIDTH_MM` 的空隙（就是图上两个虚线框之间的距离）。
    """
    out: dict[str, float] = {}
    cursor = 0.0
    for code in sorted(bounds):
        out[code] = cursor
        cols = bounds[code]["maxCol"] - bounds[code]["minCol"] + 1
        cursor += cols * CELL_MM + AISLE_WIDTH_MM
    return out


def cabinet_boxes(views: ProjectViews) -> tuple[CabinetBox, ...]:
    """全部机柜的位置（含空柜 —— 3D 里空柜也要画，否则排是断的）。

    ## 坐标系约定

    - `x` 沿通道方向（= 平面图的列方向，向右增大）
    - `z` 垂直通道方向（= 平面图的行方向，向下增大）
    - `y` 是高度，柜底在 y=0，柜顶在 y=CABINET_HEIGHT_MM

    ## 为什么用 `floor_row()` 而不是直接拿 `pos.row`

    `pos.row` 是 Excel 网格行号（403/404 两个房间都从第 3 行开始）。
    3D 里要的是**相对位置**，所以减掉该房间的起始行 —— 否则 403 房
    从第 3 行起、404 房也从第 3 行起，两个房间在世界坐标里会重叠。

    ## 朝向为什么只可能是 0 或 180

    北侧柜的门朝南（+z），南侧柜的门朝北（-z）。3D 里「背对背/面对面」的
    区别全靠这个角度；写错的话整排柜的门朝同一边，通道就看不见了。
    """
    geo = views.project.geometry
    bounds = _room_bounds(views)

    #: 每个房间内通道的先后序号（按通道号升序）。
    #:
    #: ## 为什么用通道序号算 z，而不是用平面图行号
    #:
    #: 平面图一行 = 一个柜位格子，行距 600mm；但**柜深是 1200mm**。
    #: 若直接 `z = (row - minRow) * 600`，北侧柜（行 3）与南侧柜（行 4）
    #: 只隔 600mm，两个 1200mm 深的柜子会**互相穿进去** —— 3D 里看就是
    #: 一整块实心长方体，通道完全消失（实测第一版就是这样）。
    #:
    #: 真实几何：一个通道占两行（北/南），两排柜面之间是净宽 1200mm 的
    #: 通道，两排柜**背靠背**地排到下一个通道。所以
    #:   z(通道 c 北侧) = c × (2×柜深 + 通道宽)
    #:   z(通道 c 南侧) = 上式 + 柜深 + 通道宽
    channel_order: dict[tuple[str, int], int] = {}
    for room in bounds:
        chs = sorted({ch for (r, ch, _s) in geo.racks if r == room})
        for idx, ch in enumerate(chs):
            channel_order[(room, ch)] = idx

    room_x0 = _room_x_origins(bounds)

    boxes: list[CabinetBox] = []
    for (room, ch, seq), pos in sorted(geo.racks.items()):
        b = bounds[room]
        # 列 → x：房间内以最左列为 0，再叠加本房间的世界起点。
        x = room_x0[room] + (pos.col - b["minCol"]) * CELL_MM + CABINET_WIDTH_MM / 2
        # 通道+侧别 → z（见上方说明）
        stride = 2 * CABINET_DEPTH_MM + AISLE_WIDTH_MM
        z0 = channel_order[(room, ch)] * stride
        z = z0 + CABINET_DEPTH_MM / 2
        if pos.side == "S":
            z += CABINET_DEPTH_MM + AISLE_WIDTH_MM
        devices = views.rack_devices.get((ch, seq), ())
        boxes.append(
            CabinetBox(
                channel_no=ch,
                rack_no=seq,
                x=x,
                y=CABINET_HEIGHT_MM / 2,
                z=z,
                rotation_y=0.0 if pos.side == "N" else 180.0,
                device_count=len(devices),
                used_u=sum(d.size_u for d in devices),
            )
        )
    return tuple(boxes)


def scene_payload(views: ProjectViews) -> dict[str, Any]:
    """3D 场景的完整载荷。约 824 台设备 / 362 柜位，JSON 约 120KB。

    ## 为什么设备标签要带 `category`

    3D 的颜色按类别上色（核心/接入/服务器/存储/安全/其它）。这个分类
    **必须由后端给**，前端不许自己按型号名再判一遍 —— `svg.py::categorize()`
    的判定顺序是硬约束（「核心」必须在「交换机」之前，「服务器密码机」
    必须归安全），前端复制一份迟早会歪。歪了的后果是**屏幕上红色、
    纸上黄色，现场以为拿错了图**。

    ## 为什么空柜也要给

    施工图要能看出「这排第 7 个柜是空的，可以放」。不给空柜，3D 里
    整排会断一个口子，看起来像"那里没地方"。
    """
    geo = views.project.geometry
    racks: list[dict[str, Any]] = []
    for box in cabinet_boxes(views):
        racks.append(
            {
                "channel": box.channel_no,
                "rack": box.rack_no,
                "x": round(box.x, 1),
                "y": round(box.y, 1),
                "z": round(box.z, 1),
                "rotationY": box.rotation_y,
                "deviceCount": box.device_count,
                "usedU": box.used_u,
            }
        )

    # 设备：按机柜分组，前端每组算一次柜内原点，再逐台算 U 高度
    devices: list[dict[str, Any]] = []
    for (ch, seq), items in sorted(views.rack_devices.items()):
        for d in items:
            devices.append(
                {
                    "key": "|".join(str(x) for x in d.key),
                    "channel": ch,
                    "rack": seq,
                    "model": d.model_raw,
                    "displayName": d.display_name,
                    "startU": d.span[0],
                    "endU": d.span[1],
                    "sizeU": d.size_u,
                    "category": categorize(d.model_raw),
                }
            )

    # 房间：各自包围盒 → 3D 里的地板矩形
    bounds = _room_bounds(views)
    room_x0 = _room_x_origins(bounds)
    #: (通道, 机柜) → 房间号。建一次 O(n)，别在循环里再扫全表。
    room_of: dict[tuple[int, int], str] = {
        (ch, seq): room for (room, ch, seq) in geo.racks
    }
    per_room: dict[str, int] = {}
    for r in racks:
        code = room_of.get((r["channel"], r["rack"]))
        if code is not None:
            per_room[code] = per_room.get(code, 0) + 1
    rooms: list[dict[str, Any]] = []
    for code in sorted(bounds):
        b = bounds[code]
        cols = b["maxCol"] - b["minCol"] + 1
        n_channels = len({ch for (r, ch, _s) in geo.racks if r == code})
        # 深度按「通道数 × (两排柜 + 一条通道)」算，不能用平面图行数 × 600
        # —— 行距是柜位格距，不是柜深（见 `cabinet_boxes` 里的说明）。
        depth = n_channels * (2 * CABINET_DEPTH_MM + AISLE_WIDTH_MM)
        rooms.append(
            {
                "roomCode": code,
                "widthMm": round(cols * CELL_MM, 1),
                "depthMm": round(depth, 1),
                "channels": n_channels,
                "cabinets": per_room.get(code, 0),
                #: 地板矩形在世界坐标里的起点（左前角，y=0 平面）
                "originX": round(room_x0[code], 1),
                "originZ": 0.0,
            }
        )

    return {
        #: 单位声明。前端据此断言，防止有人把 mm 当米用（差 1000 倍，画面全白）
        "units": "mm",
        "constants": {
            "mmPerU": MM_PER_U,
            "cabinetWidth": CABINET_WIDTH_MM,
            "cabinetHeight": CABINET_HEIGHT_MM,
            "cabinetDepth": CABINET_DEPTH_MM,
            "aisleWidth": AISLE_WIDTH_MM,
            "rackHeightU": DEFAULT_RACK_HEIGHT_U,
        },
        "rooms": rooms,
        "racks": racks,
        "devices": devices,
        "totals": {
            "rooms": len(rooms),
            "racks": len(racks),
            "racksWithDevices": len(views.rack_devices),
            "devices": len(devices),
        },
    }


__all__ = [
    "AISLE_WIDTH_MM",
    "CABINET_DEPTH_MM",
    "CABINET_HEIGHT_MM",
    "CABINET_WIDTH_MM",
    "CabinetBox",
    "MM_PER_U",
    "cabinet_boxes",
    "scene_payload",
]
