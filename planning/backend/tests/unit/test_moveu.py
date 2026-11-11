"""`moveu` 的守卫 —— U 位图拖拽编辑的数据部分。

## 这个模块守护的东西

拖拽看起来是纯前端交互，但「拖完之后什么变了」有业务含义。这组用例锁的是：

1. **改 U 位不影响线长**（同机柜 `path_units = 0`）。这是本功能能上架的
   安全前提 —— 如果拖一格会改线长，用户点一下就把 5000 行长度全改了。
2. **失败必须能落地成一句话**：拖不动的时候用户要知道「为什么」和「怎么办」，
   而不是「不支持」。
3. **失败不留半改**：`ok=False` 时返回的 doc 与输入等价（不出现中间态）。
4. **默认拒绝重叠**，但留 `allow_overlap` 后门（「先占位后调」是真实动作）。

## 为什么这些必须写在域层用例里而不是只测 API

前端的拖拽落点计算会被浏览器坐标、缩放、滚动干扰，那些测不到。
但「鼠标落在第 X U，该不该允许这次移动」是纯逻辑 —— 这是唯一能
稳定回归的部分。API 用例只验证「协议通」。
"""

from __future__ import annotations

import copy

import pytest

from idcbox.domain.cable_length.formulas import path_units
from idcbox.domain.cable_length.rules import DEFAULT_RULE
from idcbox.domain.geometry.floor import RackPlacement
from idcbox.domain.geometry.layout import RackRef, build_snapshot
from idcbox.domain.ports.moveu import (
    RACK_U_HEIGHT,
    editable_devices,
    find_device_row,
    is_layout_generated,
    move_device,
    occupied_spans,
)


@pytest.fixture()
def doc() -> dict:
    """一台 10U 交换机 + 一台 1U 交换机 + 一台 2U 特殊设备，同一机柜。"""
    return {
        "schema": 1,
        "project": "t",
        "site": "t",
        # 布局段放 rack2-3 而不是 rack1：rack1 已逐台声明了三台设备，
        # 布局段再铺满 8 个槽位就会和它们真叠在一起 —— 那会让
        # 「移动」类用例全部撞上重叠检查，测不到原本要测的东西。
        "racks": [
            {"channel": 2, "from": 2, "to": 3, "model": "计算节点A（ARM）"},
        ],
        "switches": [
            {
                "instance": "业务核心交换机-1",
                "model": "业务核心交换机",
                "channel": 2,
                "rack": 1,
                "u": 3,
                "uSpan": 10,
            },
            {
                "instance": "业务接入交换机-1",
                "model": "业务接入交换机1",
                "channel": 2,
                "rack": 1,
                "u": 40,
            },
        ],
        "devices": [
            {
                "instance": "防火墙-1",
                "model": "防火墙1",
                "channel": 2,
                "rack": 1,
                "u": 20,
                "uSpan": 2,
            },
        ],
        "routes": [],
        "chains": [],
        "explicitLinks": [],
    }


class TestSafetyPremise:
    """前提 1：拖 U 位不改线长。"""

    def test_same_rack_path_is_zero(self) -> None:
        """同一机柜两端 `path_units = 0` —— 拖拽不会改变任何长度档位。

        `path_units` 是经验表与几何公式共用的距离口径：
        A 类（同排）取列差、B/C 类取排端折返距离。同柜时两端是同一台机柜，
        列差为 0，折返项也相消 —— 所以恒为 0，与 U 位多少无关。
        """
        racks = [
            RackPlacement("403", 2, 1, "N", col=30, row=5),
            RackPlacement("403", 2, 2, "N", col=29, row=5),
        ]
        geo = build_snapshot(racks, version="t", cmax_override={"403": 38})
        a = geo.pos(RackRef(2, 1))
        assert a is not None
        assert path_units(a, a, DEFAULT_RULE) == 0


class TestFindDeviceRow:
    """定位一台设备。"""

    def test_finds_switch(self, doc: dict) -> None:
        assert find_device_row(doc, 2, 1, "业务核心交换机-1") == ("switches", 0)

    def test_finds_device(self, doc: dict) -> None:
        """`devices` 段也要找 —— 用户看不出两者区别。"""
        assert find_device_row(doc, 2, 1, "防火墙-1") == ("devices", 0)

    def test_name_normalized(self, doc: dict) -> None:
        """名字带空格也要命中（源表 13 种写法归一后 12 种）。"""
        doc["switches"][0]["instance"] = "业务核心交换机 -1"
        assert find_device_row(doc, 2, 1, "业务核心交换机-1") == ("switches", 0)

    def test_wrong_channel_misses(self, doc: dict) -> None:
        """## 为什么键必须带通道

        `-1` 后缀的编号规则跨通道重复（ch2 也有 `业务接入交换机-1`、
        ch8 也有）。只按名字查会把 ch2 的设备拖到 ch8 去。
        """
        assert find_device_row(doc, 8, 1, "业务核心交换机-1") is None

    def test_wrong_rack_misses(self, doc: dict) -> None:
        assert find_device_row(doc, 2, 2, "业务核心交换机-1") is None


class TestOccupiedSpans:
    def test_lists_all_with_span(self, doc: dict) -> None:
        spans = occupied_spans(doc, channel_no=2, rack_no=1)
        assert (3, 12, "业务核心交换机-1") in spans
        assert (20, 21, "防火墙-1") in spans
        assert (40, 40, "业务接入交换机-1") in spans

    def test_skip_excludes_self(self, doc: dict) -> None:
        """排除自己 —— 否则它的旧位置会和自己冲突，移到自己原位都算冲突。"""
        spans = occupied_spans(
            doc, channel_no=2, rack_no=1, skip=("switches", 0)
        )
        assert all(name != "业务核心交换机-1" for _, _, name in spans)

    def test_other_rack_excluded(self, doc: dict) -> None:
        assert occupied_spans(doc, channel_no=2, rack_no=9) == []


class TestMoveDevice:
    def test_move_to_free_spot(self, doc: dict) -> None:
        res = move_device(doc, channel_no=2, rack_no=1, instance="业务接入交换机-1", new_u=30)
        assert res.ok is True
        assert res.section == "switches"
        assert res.row_no == 1
        assert res.doc["switches"][1]["u"] == 30

    def test_input_doc_untouched(self, doc: dict) -> None:
        """失败不留半改、成功也不改输入 —— 调用方按需替换。"""
        before = copy.deepcopy(doc)
        move_device(doc, channel_no=2, rack_no=1, instance="业务接入交换机-1", new_u=30)
        assert doc == before

    def test_move_to_own_place_is_ok(self, doc: dict) -> None:
        """移到自己原位不算冲突（skip 生效）。"""
        res = move_device(doc, channel_no=2, rack_no=1, instance="防火墙-1", new_u=20)
        assert res.ok is True
        assert res.overlaps == ()

    def test_range_low(self, doc: dict) -> None:
        res = move_device(doc, channel_no=2, rack_no=1, instance="业务接入交换机-1", new_u=0)
        assert res.ok is False
        assert res.error is not None and "1~42" in res.error

    def test_range_high(self, doc: dict) -> None:
        res = move_device(doc, channel_no=2, rack_no=1, instance="业务接入交换机-1", new_u=99)
        assert res.ok is False
        assert res.error is not None and "99" in res.error

    def test_span_overflow(self, doc: dict) -> None:
        """10U 设备放到 38U 会顶到 47U。

        ## 为什么这条必须单独说

        只校验 `newU <= 42` 是不够的：10U 设备的合法起点最大是 33U。
        用户把柜顶的 2U 设备位置套给 10U 设备时，错的是「顶到柜外」，
        报「U 位必须 1~42」会让他以为输入非法而反复重试。
        """
        res = move_device(
            doc, channel_no=2, rack_no=1, instance="业务核心交换机-1", new_u=38
        )
        assert res.ok is False
        assert res.error is not None
        assert "47U" in res.error and "42U" in res.error

    def test_overlap_refused_by_default(self, doc: dict) -> None:
        res = move_device(
            doc, channel_no=2, rack_no=1, instance="业务接入交换机-1", new_u=5
        )
        assert res.ok is False
        assert res.error is not None and "已被占用" in res.error
        assert "业务核心交换机-1（3-12U）" in res.error
        assert res.overlaps

    def test_overlap_allowed_with_flag(self, doc: dict) -> None:
        """「先占位后调」是真实动作，要留后门。"""
        res = move_device(
            doc,
            channel_no=2,
            rack_no=1,
            instance="业务接入交换机-1",
            new_u=5,
            allow_overlap=True,
        )
        assert res.ok is True
        assert "业务核心交换机-1（3-12U）" in res.overlaps

    def test_layout_device_needs_current_u_to_locate(self, doc: dict) -> None:
        """布局段设备不传 `currentU` 时必须问清楚，不能随便挑一台。

        ## 为什么不猜

        同机柜 8 台计算节点的显示名完全相同（都没有实例名，显示名就是型号）。
        猜错的话用户看到的「已移到 30U」提示完全正确，动的却是另一个设备 ——
        这类错误要等生成完布线表、现场按图插线才发现。

        调用方（前端 U 位图）手上一定有当前 U 位，传它就行。
        """
        res = move_device(
            doc, channel_no=2, rack_no=2, instance="计算节点A（ARM）", new_u=30
        )
        assert res.ok is False
        assert res.error is not None
        assert "第几 U" in res.error

    def test_layout_device_moves_when_current_u_given(self, doc: dict) -> None:
        """★ 布局段的设备现在能拖了 —— v0.4 时这里报「这个操作还没做」。"""
        res = move_device(
            doc,
            channel_no=2,
            rack_no=2,
            instance="计算节点A（ARM）",
            new_u=34,
            current_u=3,
        )
        assert res.ok, res.error
        assert res.section == "devices", "覆盖记录写进 devices 段"
        assert res.doc["devices"][-1]["instance"] == "N02-02-S0"

    def test_unknown_device(self, doc: dict) -> None:
        res = move_device(doc, channel_no=2, rack_no=9, instance="不存在-1", new_u=10)
        assert res.ok is False
        assert res.error is not None and "找不到" in res.error

    def test_ok_result_carries_no_error(self, doc: dict) -> None:
        res = move_device(doc, channel_no=2, rack_no=1, instance="防火墙-1", new_u=30)
        assert res.ok is True
        assert res.error is None


class TestIsLayoutGenerated:
    def test_true_for_block_model(self, doc: dict) -> None:
        assert is_layout_generated(doc, 2, 2, "计算节点A（ARM）") is True

    def test_false_outside_range(self, doc: dict) -> None:
        assert is_layout_generated(doc, 2, 9, "计算节点A（ARM）") is False

    def test_false_for_other_model(self, doc: dict) -> None:
        assert is_layout_generated(doc, 2, 2, "业务接入交换机1") is False


class TestEditableDevices:
    def test_counts_declared_only(self, doc: dict) -> None:
        """只返回逐台声明过的；布局段排布的 663 台不在其中。"""
        ed = editable_devices(doc)
        assert ed[(2, 1, "业务核心交换机-1")] == "switches"
        assert ed[(2, 1, "防火墙-1")] == "devices"
        assert (2, 2, "计算节点A（ARM）") not in ed

    def test_empty_doc(self) -> None:
        assert editable_devices({}) == {}

    def test_skips_bad_rows(self) -> None:
        """缺字段的行跳过而不是抛异常 —— 意图是用户手写的，会有半行。"""
        doc = {"switches": [{"instance": "A"}, {"channel": 1, "rack": 1, "instance": "B"}]}
        assert editable_devices(doc) == {(1, 1, "B"): "switches"}


class TestRackConstant:
    def test_u_height(self) -> None:
        assert RACK_U_HEIGHT == 42
