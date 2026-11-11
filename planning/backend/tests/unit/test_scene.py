"""3D 场景数据（`application/services/scene.py`）的守卫。

## 这个模块守护什么

3D 的几何错误**几乎都是"差一点"**：柜深算成柜位格距 → 两排柜穿模；
两个房间不给 x 偏移 → 完全重叠；柜高小于 42U → 最上面那台设备捅出柜顶。
这些在屏幕上都"看着像那么回事"，只有数值断言抓得住。

## 为什么这部分测服务层而不测渲染

渲染要 WebGL 上下文（且 CI 里是 SwiftShader 软件渲染，行为与真显卡不同）。
而「柜子在哪、多大、朝哪」是**纯数据**，与渲染器无关 ——
几何对了，任何渲染器画出来都对。渲染的坑另有一套（见 `scene3d.test.ts`
与真机截图验证）。
"""

from __future__ import annotations

import pytest

from idcbox.application.services.scene import (
    AISLE_WIDTH_MM,
    CABINET_DEPTH_MM,
    CABINET_HEIGHT_MM,
    CABINET_WIDTH_MM,
    MM_PER_U,
    cabinet_boxes,
    scene_payload,
)
from idcbox.domain.validation.instances import DeviceInstance


class _FakePos:
    def __init__(self, room: str, ch: int, seq: int, side: str, col: int, row: int):
        self.room_code = room
        self.channel_no = ch
        self.seq_no = seq
        self.side = side
        self.col = col
        self.row = row


class _FakeGeometry:
    def __init__(self, racks: dict):
        self.racks = racks


class _FakeProject:
    def __init__(self, racks: dict, cmax=None):
        self.geometry = _FakeGeometry(racks)
        self.cmax_by_room = cmax or {}
        self.rack_count = len(racks)


class _FakeViews:
    """只带 `scene_payload` 用到的字段 —— 不读 Excel，毫秒级跑完。"""

    def __init__(self, racks: dict, devices: dict, inv_count: int = 0):
        self.project = _FakeProject(racks)
        self.rack_devices = devices
        self.inventory = type("Inv", (), {"count_normalized": inv_count})()


def _dev(ch: int, seq: int, model: str, start_u: int, size_u: int, name: str = "") -> DeviceInstance:
    return DeviceInstance(
        channel_no=ch,
        rack_no=seq,
        u_raw=f"{start_u}-{start_u + size_u - 1}U",
        span=(start_u, start_u + size_u - 1),
        model_raw=model,
        model=model,
        names={name} if name else set(),
    )


@pytest.fixture()
def two_rooms():
    """两个房间、各一条通道、北南各一个柜。模拟 403/404 的列号各自独立。"""
    racks = {
        # 403 房：列 23 起；404 房：列 3 起 —— 两房的列号范围不重叠
        ("403", 1, 1): _FakePos("403", 1, 1, "N", col=23, row=3),
        ("403", 1, 27): _FakePos("403", 1, 27, "S", col=23, row=4),
        ("404", 8, 1): _FakePos("404", 8, 1, "N", col=3, row=3),
        ("404", 8, 27): _FakePos("404", 8, 27, "S", col=3, row=4),
    }
    devices = {
        (1, 1): (_dev(1, 1, "业务核心交换机", 3, 10, "业务核心交换机-1"),),
        (8, 1): (_dev(8, 1, "计算节点A（ARM）", 3, 2),),
    }
    return _FakeViews(racks, devices, inv_count=2)


class TestConstants:
    def test_1u_is_eia310_standard(self) -> None:
        """1U = 44.45mm 是 EIA-310 标准，不是可调参数。"""
        assert MM_PER_U == 44.45

    def test_cabinet_width_matches_cabling_formula(self) -> None:
        """★ 柜宽 600mm 必须与布线表 R 列的 `0.6m/柜位` 同源。

        改了这里等于改线长公式 —— 而线长有导出铁线（123072 格零差异）守着，
        改错会立刻红。这里再钉一道，防止「3D 里柜子宽了 50mm」这种
        不会让铁线变红、但 3D 与 2D 对不上的错。
        """
        assert CABINET_WIDTH_MM == 600

    def test_42u_fits_in_cabinet(self) -> None:
        """★ 42U 有效空间必须塞得进柜高，否则最上面那台设备捅出柜顶。"""
        assert 42 * MM_PER_U < CABINET_HEIGHT_MM

    def test_aisle_narrower_than_cabinet_depth(self) -> None:
        """通道净宽应小于柜深（否则「背靠背」的物理关系不成立）。"""
        assert AISLE_WIDTH_MM <= CABINET_DEPTH_MM


class TestCabinetGeometry:
    def test_one_box_per_rack(self, two_rooms) -> None:
        boxes = cabinet_boxes(two_rooms)
        assert len(boxes) == 4

    def test_cabinets_are_not_overlapping(self, two_rooms) -> None:
        """★ 任意两个柜的世界坐标 (x,z) 不能相同。

        这条守的是第一个真 bug：两排柜只隔 600mm（柜位格距），
        而柜深是 1200mm —— 两排会互相穿进去，3D 里看是一整块实心体，
        通道完全消失。
        """
        boxes = cabinet_boxes(two_rooms)
        seen = {(round(b.x), round(b.z)) for b in boxes}
        assert len(seen) == len(boxes)

    def test_aisle_gap_between_north_and_south(self, two_rooms) -> None:
        """★ 北侧柜与南侧柜的中心距 = 柜深 + 通道宽。

        这是「两排柜面之间留一条 1200mm 通道」的数值表达。
        写成 600mm（格距）就穿模；写成 1200mm（只有柜深）就没通道。
        """
        boxes = cabinet_boxes(two_rooms)
        north = next(b for b in boxes if b.channel_no == 1 and b.rack_no == 1)
        south = next(b for b in boxes if b.channel_no == 1 and b.rack_no == 27)
        assert south.z - north.z == pytest.approx(CABINET_DEPTH_MM + AISLE_WIDTH_MM, abs=0.5)

    def test_rooms_are_offset_in_x(self, two_rooms) -> None:
        """★ 两个房间的柜不能落在同一世界坐标（实测不修会 146 个重复）。

        403 用列 23 起、404 用列 3 起，两房列号各有各的基准。
        若都按「减本房最左列」算 x，两房的柜会完全重叠 ——
        3D 里表现为「一个房间，柜数翻倍」。
        """
        boxes = cabinet_boxes(two_rooms)
        r403 = [b for b in boxes if b.channel_no == 1]
        r404 = [b for b in boxes if b.channel_no == 8]
        assert min(b.x for b in r403) != min(b.x for b in r404)

    def test_rotation_by_side(self, two_rooms) -> None:
        """北侧柜朝 0°、南侧柜朝 180° —— 否则整排柜门朝同一边，看不到通道。"""
        boxes = cabinet_boxes(two_rooms)
        north = next(b for b in boxes if b.rack_no == 1 and b.channel_no == 1)
        south = next(b for b in boxes if b.rack_no == 27 and b.channel_no == 1)
        assert north.rotation_y == 0.0
        assert south.rotation_y == 180.0

    def test_cabinet_y_is_half_height(self, two_rooms) -> None:
        """柜中心在柜高一半（three 的 BoxGeometry 以中心为原点）。"""
        boxes = cabinet_boxes(two_rooms)
        assert boxes[0].y == pytest.approx(CABINET_HEIGHT_MM / 2, abs=0.5)

    def test_empty_rack_still_present(self, two_rooms) -> None:
        """★ 空柜也要画 —— 施工图要能看出「这个柜是空的，可以放」。

        不画空柜整排会断一个口子，看起来像"那里没地方"。
        """
        boxes = cabinet_boxes(two_rooms)
        empty = [b for b in boxes if b.device_count == 0]
        assert len(empty) == 2  # 两个南侧柜都没有设备
        assert all(b.device_count == 0 for b in empty)

    def test_used_u_summed(self, two_rooms) -> None:
        boxes = cabinet_boxes(two_rooms)
        rack1 = next(b for b in boxes if b.channel_no == 1 and b.rack_no == 1)
        assert rack1.used_u == 10


class TestScenePayload:
    def test_units_declared_as_mm(self, two_rooms) -> None:
        """★ 载荷必须自报单位。前端据此断言，防有人把 mm 当米（差 1000 倍）。"""
        payload = scene_payload(two_rooms)
        assert payload["units"] == "mm"

    def test_constants_present(self, two_rooms) -> None:
        payload = scene_payload(two_rooms)
        c = payload["constants"]
        assert c["mmPerU"] == 44.45
        assert c["cabinetWidth"] == 600
        assert c["cabinetHeight"] == CABINET_HEIGHT_MM
        assert c["cabinetDepth"] == CABINET_DEPTH_MM
        assert c["aisleWidth"] == AISLE_WIDTH_MM

    def test_room_has_origin(self, two_rooms) -> None:
        """★ 房间必须带 `originX` —— 前端画地板要用，缺了地板会叠在一起。"""
        payload = scene_payload(two_rooms)
        rooms = {r["roomCode"]: r for r in payload["rooms"]}
        assert rooms["403"]["originX"] == 0.0
        assert rooms["404"]["originX"] > rooms["403"]["originX"]

    def test_room_depth_uses_channel_count(self, two_rooms) -> None:
        """房间深度 = 通道数 × (2×柜深 + 通道宽)，不是平面图行数 × 格距。"""
        payload = scene_payload(two_rooms)
        room = payload["rooms"][0]
        assert room["channels"] == 1
        assert room["depthMm"] == pytest.approx(2 * CABINET_DEPTH_MM + AISLE_WIDTH_MM, abs=0.5)

    def test_device_carries_category(self, two_rooms) -> None:
        """★ 设备类别必须由后端给，前端不许自己按型号名再判一遍。

        `categorize()` 的判定顺序是硬约束（「核心」在「交换机」之前、
        「服务器密码机」归安全）。前端复制一份迟早会歪，歪了的后果是
        **屏幕上红色、纸上黄色，现场以为拿错了图**。
        """
        payload = scene_payload(two_rooms)
        by_model = {d["model"]: d["category"] for d in payload["devices"]}
        assert by_model["业务核心交换机"] == "core"
        assert by_model["计算节点A（ARM）"] == "server"

    def test_device_has_u_range(self, two_rooms) -> None:
        payload = scene_payload(two_rooms)
        core = next(d for d in payload["devices"] if d["category"] == "core")
        assert core["startU"] == 3
        assert core["endU"] == 12
        assert core["sizeU"] == 10

    def test_totals(self, two_rooms) -> None:
        payload = scene_payload(two_rooms)
        assert payload["totals"]["rooms"] == 2
        assert payload["totals"]["racks"] == 4
        assert payload["totals"]["racksWithDevices"] == 2
        assert payload["totals"]["devices"] == 2

    def test_all_lengths_are_numbers(self, two_rooms) -> None:
        """★ 所有长度字段必须是数字，不能是字符串。

        前端直接拿去做乘除（`mmToWorld`）。给个 `"9600mm"` 会让
        `9600mm / 1000` 得到 NaN，画面静默变空白 —— 没有报错。
        """
        payload = scene_payload(two_rooms)
        for room in payload["rooms"]:
            assert isinstance(room["widthMm"], (int, float))
            assert isinstance(room["originX"], (int, float))
        for rack in payload["racks"]:
            assert isinstance(rack["x"], (int, float))
            assert isinstance(rack["z"], (int, float))
