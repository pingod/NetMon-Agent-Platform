"""布局段设备也能拖 —— `placement_overrides` + `_move_layout_device` 的守卫。

## 这组用例守护的东西

v0.4 拖拽功能上线时，**663 台设备（占 79.5%）拖不动**：它们由布局段
（`RackBlock`）按槽位序列自动排布，意图里没有它们的 `u` 字段。当时的
提示是「拆出来声明成独立设备（这个操作还没做）」。

本模块锁的是补完后的四条不变量：

1. **只有被拖那一台动**，同机柜另外 7 台保持原位（键必须带 U 位）。
2. **覆盖记录写进 `devices` 段**，`instance` 是槽位 id（`N02-01-S0`），
   不是型号 —— 生成器与端口分配都按槽位 id 认设备。
3. **重叠仍然拦住**：布局段排布的 663 台现在也算「占用」，
   否则把交换机拖进计算节点的 U 位不会被发现，现场会真叠在一起。
4. **不传 `currentU` 时不能瞎猜**：布局段设备没有实例名，
   猜错会改掉同柜的另一台。

## 为什么这些只能在域层测

前端只知道「鼠标落在第几 U」，它拿不到槽位 id、也算不出槽位序列。
"""

from __future__ import annotations

import pytest

from idcbox.domain.ports.intent import (
    DeviceSpec,
    Intent,
    RackBlock,
    expand_placements,
    placement_overrides,
)
from idcbox.domain.ports.moveu import (
    RACK_U_HEIGHT,
    editable_items,
    move_device,
    occupied_spans,
)


def _doc(*, extra_devices: list[dict] | None = None, blocks: list[dict] | None = None) -> dict:
    """一个通道一个机柜、8 个槽位全放「节点X」的意图。

    `slot` 0..7 对应 U 位 3/7/11/15/19/23/27/31（`SLOT_BASE_U=3`、
    `SLOT_PITCH_U=4`），设备高 2U。
    """
    return {
        "schema": 1,
        "project": "t",
        "site": "t",
        "racks": blocks
        or [{"channel": 2, "from": 1, "to": 1, "model": "节点X"}],
        "switches": [
            {
                "instance": "交换-1",
                "model": "业务接入交换机1",
                "channel": 2,
                "rack": 1,
                "u": 40,
            }
        ],
        "devices": list(extra_devices or []),
        "routes": [],
        "chains": [],
        "explicitLinks": [],
    }


class TestPlacementOverrides:
    def test_no_override_is_empty(self) -> None:
        it = Intent("t", racks=(RackBlock(2, 1, 1, "节点X"),))
        assert placement_overrides(it) == {}

    def test_devices_entry_with_slot_id_overrides_u(self) -> None:
        """★ 覆盖记录生效：这一台改用 devices 段写的 U 位。"""
        it = Intent(
            "t",
            racks=(RackBlock(2, 1, 1, "节点X"),),
            devices=(DeviceSpec("N02-01-S0", "节点X", 2, 1, u_start=37, u_span=2),),
        )
        assert placement_overrides(it) == {"N02-01-S0": 37}
        starts = [p.u_start for p in expand_placements(it)]
        assert starts[0] == 37, "第一台必须被覆盖到 37U"
        assert starts[1:] == [7, 11, 15, 19, 23, 27, 31], "其余 7 台不能跟着动"

    def test_normal_instance_name_is_not_an_override(self) -> None:
        """普通实例名（`防火墙-1`）不是槽位 id，不能被当成覆盖。"""
        it = Intent(
            "t",
            racks=(RackBlock(2, 1, 1, "节点X"),),
            devices=(DeviceSpec("防火墙-1", "防火墙1", 2, 1, u_start=37, u_span=2),),
        )
        assert placement_overrides(it) == {}

    def test_slot_map_duplicate_ids_are_recognized(self) -> None:
        """同槽位叠两台时槽位 id 带 `-1` 后缀，覆盖记录也要能对上。"""
        block = RackBlock(2, 1, 1, "节点X", slot_map=((1, (7, 7)),))
        it = Intent("t", racks=(block,))
        ids = [p.node_id for p in expand_placements(it)]
        assert ids == ["N02-01-S7", "N02-01-S7-1"]
        it2 = Intent(
            "t",
            racks=(block,),
            devices=(DeviceSpec("N02-01-S7-1", "节点X", 2, 1, u_start=37, u_span=2),),
        )
        assert placement_overrides(it2) == {"N02-01-S7-1": 37}


class TestOccupiedSpansIncludesLayout:
    def test_layout_devices_count_as_occupied(self) -> None:
        """★ 布局段排布的设备也算「这个 U 位被占了」。

        不加这一层的话，把交换机拖进计算节点占的 U 位不会被拦住 ——
        生成后会真的叠在一起，而 U 位图上看起来只是两块重合。
        """
        spans = occupied_spans(_doc(), channel_no=2, rack_no=1)
        starts = sorted(lo for lo, _hi, _name in spans)
        # 8 台布局段设备（3/7/11/15/19/23/27/31）+ 1 台交换机（40）
        assert starts == [3, 7, 11, 15, 19, 23, 27, 31, 40]

    def test_layout_span_uses_2u(self) -> None:
        spans = occupied_spans(_doc(), channel_no=2, rack_no=1)
        layout = [sp for sp in spans if sp[2] == "节点X"]
        assert len(layout) == 8
        assert all(hi - lo == 1 for lo, hi, _n in layout), "2U 设备占 lo..lo+1"

    def test_other_rack_not_mixed_in(self) -> None:
        spans = occupied_spans(_doc(), channel_no=2, rack_no=3)
        assert spans == []

    def test_broken_doc_does_not_raise(self) -> None:
        """半成品 doc 不能让拖拽整个挂掉，只是少一层保护。"""
        assert occupied_spans({"racks": "坏"}, channel_no=2, rack_no=1) == []


class TestMoveLayoutDevice:
    def test_moves_one_device_only(self) -> None:
        """★ 只有被拖那一台动，同柜另外 7 台保持原位。"""
        r = move_device(
            _doc(),
            channel_no=2,
            rack_no=1,
            instance="节点X",
            new_u=37,
            current_u=3,
        )
        assert r.ok, r.error
        assert r.section == "devices"
        assert r.u_span == 2
        written = r.doc["devices"][-1]
        assert written["instance"] == "N02-01-S0"
        assert written["u"] == 37
        # u_span 默认 2，不该被写进 JSON（`intent.py` 的省略规则）
        assert "uSpan" not in written

        starts = [p.u_start for p in expand_placements(_intent_of(r.doc))]
        assert starts == [37, 7, 11, 15, 19, 23, 27, 31]

    def test_overlap_is_refused(self) -> None:
        """★ 拖到别的槽位上必须被拦住。"""
        r = move_device(
            _doc(), channel_no=2, rack_no=1, instance="节点X", new_u=7, current_u=3
        )
        assert not r.ok
        assert "U7-8 已被占用" in (r.error or "")
        assert r.overlaps

    def test_overlap_allowed_with_flag(self) -> None:
        r = move_device(
            _doc(),
            channel_no=2,
            rack_no=1,
            instance="节点X",
            new_u=7,
            current_u=3,
            allow_overlap=True,
        )
        assert r.ok, r.error
        assert r.overlaps

    def test_move_to_own_position_is_ok(self) -> None:
        """拖回原位不算重叠（自己不能和自己冲突）。"""
        r = move_device(
            _doc(), channel_no=2, rack_no=1, instance="节点X", new_u=3, current_u=3
        )
        assert r.ok, r.error
        assert r.overlaps == ()

    def test_overflow_is_refused(self) -> None:
        r = move_device(
            _doc(), channel_no=2, rack_no=1, instance="节点X", new_u=RACK_U_HEIGHT, current_u=3
        )
        assert not r.ok
        assert "超出机柜" in (r.error or "")

    def test_without_current_u_refuses_instead_of_guessing(self) -> None:
        """★ 不传 `currentU` 时必须拒绝，不能随便挑一台改。

        同机柜 8 台同型号，猜错会改掉另一台 —— 而用户看到的
        「已移到 37U」提示完全正确，只是动的是别的设备。
        """
        r = move_device(_doc(), channel_no=2, rack_no=1, instance="节点X", new_u=37)
        assert not r.ok
        assert "第几 U" in (r.error or "")
        assert "同名设备" in (r.error or "")

    def test_second_move_updates_existing_record(self) -> None:
        """同一台拖两次：改已有覆盖记录，不能长出两条。"""
        r1 = move_device(
            _doc(), channel_no=2, rack_no=1, instance="节点X", new_u=37, current_u=3
        )
        assert r1.ok
        r2 = move_device(
            r1.doc, channel_no=2, rack_no=1, instance="节点X", new_u=35, current_u=37
        )
        assert r2.ok, r2.error
        rows = [x for x in r2.doc["devices"] if x.get("instance") == "N02-01-S0"]
        assert len(rows) == 1, "不能出现两条覆盖记录"
        assert rows[0]["u"] == 35

    def test_input_doc_untouched(self) -> None:
        doc = _doc()
        before = __import__("copy").deepcopy(doc)
        move_device(doc, channel_no=2, rack_no=1, instance="节点X", new_u=37, current_u=3)
        assert doc == before


class TestUnknownDeviceGuard:
    """★ 单台路径的名字白名单 —— v0.12 补的一道守卫。

    ## 这个 bug 有多坏

    `_slot_id_at` 在 instance 匹配不上时会退回「按型号 + U 位找第一台」，
    而 `_guess_model` 只看机柜属于哪个布局块、不看名字。于是「拖一台
    不存在的设备」会被解析成「该 U 位上那台真设备」并**成功挪动它**，
    返回 `ok=True`，界面上看不出任何异常。

    v0.11 给批量路径（`_identity`）加过同一道守卫，但单台路径漏了 ——
    `POST /api/intent/move` 挪错机器、`POST /api/intent/move-batch` 拒绝，
    两个入口对同一件事给出相反答案。
    """

    def test_unknown_name_is_refused(self) -> None:
        r = move_device(
            _doc(),
            channel_no=2,
            rack_no=1,
            instance="不存在的设备",
            new_u=37,
            current_u=3,
        )
        assert not r.ok
        assert "意图里找不到" in (r.error or "")
        # 失败不留半改
        assert r.doc["devices"] == []

    def test_unknown_name_does_not_touch_real_device(self) -> None:
        """★ 拒绝之后，真设备的位置必须纹丝不动。

        这条比上一条更重要：上一条只证明「返回了失败」，
        这条证明「没有悄悄改掉某台真机」。
        """
        doc = _doc()
        move_device(doc, channel_no=2, rack_no=1, instance="不存在的设备", new_u=37, current_u=3)
        starts = [p.u_start for p in expand_placements(_intent_of(doc))]
        assert starts == [3, 7, 11, 15, 19, 23, 27, 31]

    def test_model_name_is_accepted(self) -> None:
        """按型号指认是合法的（前端就是这么传的）。"""
        r = move_device(_doc(), channel_no=2, rack_no=1, instance="节点X", new_u=37, current_u=3)
        assert r.ok, r.error

    def test_slot_id_is_accepted(self) -> None:
        """按槽位 id 指认也合法（覆盖记录写的就是它）。"""
        r = move_device(
            _doc(), channel_no=2, rack_no=1, instance="N02-01-S2", new_u=37, current_u=11
        )
        assert r.ok, r.error
        assert r.doc["devices"][-1]["instance"] == "N02-01-S2"

    def test_normalized_name_is_accepted(self) -> None:
        """名字里多空格仍要认（源表 13 种写法，归一后 12 种）。"""
        r = move_device(_doc(), channel_no=2, rack_no=1, instance="节点 X", new_u=37, current_u=3)
        assert r.ok, r.error


class TestEditableItems:
    def test_all_devices_are_listed(self) -> None:
        """★ 827 台全部可拖（布局段 663 + 交换机 130 + 特殊设备 34）。"""
        items = editable_items(_intent_dict())
        by_section: dict[str, int] = {}
        for it in items:
            by_section[it["section"]] = by_section.get(it["section"], 0) + 1
        assert sum(by_section.values()) == 827
        assert by_section["layout"] == 663
        assert by_section["switches"] == 130
        assert by_section["devices"] == 34

    def test_layout_items_carry_u_start(self) -> None:
        """布局段项必须带 `uStart`，否则前端分不清同柜同名 8 台。"""
        items = [x for x in editable_items(_intent_dict()) if x["section"] == "layout"]
        keys = {(x["channel"], x["rack"], x["instance"], x["uStart"]) for x in items}
        assert len(keys) == len(items), "四元组必须唯一"

    def test_override_record_is_exposed_as_model_not_slot_id(self) -> None:
        """★ 覆盖记录必须按「型号 + U 位」暴露，不能暴露槽位 id。

        前端认设备用的是台账 `displayName`（布局段设备就是型号）。
        若这里返回 `N02-01-S0`，前端永远匹配不上 ——
        现象是「拖过一次之后这台就拖不动了」，而且完全不报错。
        """
        doc = _doc(
            extra_devices=[
                {"instance": "N02-01-S0", "model": "节点X", "channel": 2, "rack": 1, "u": 37}
            ]
        )
        items = editable_items(doc)
        layout = [x for x in items if x["section"] == "layout"]
        assert len(layout) == 8, "8 个槽位都还在（覆盖只挪位置，不减台数）"
        assert all(x["instance"] == "节点X" for x in layout), "不能有槽位 id 漏出来"
        assert 37 in {x["uStart"] for x in layout}, "被覆盖那台在 37U"
        assert 3 not in {x["uStart"] for x in layout}, "旧位置 3U 不该再有设备"

    def test_editable_everything_after_move(self) -> None:
        """拖完之后仍全部可拖（覆盖记录不能把设备从清单里挤掉）。"""
        r = move_device(
            _doc(), channel_no=2, rack_no=1, instance="节点X", new_u=37, current_u=3
        )
        assert r.ok
        items = editable_items(r.doc)
        layout = [x for x in items if x["section"] == "layout"]
        assert len(layout) == 8, "被覆盖的那台仍要可拖（它现在位置 37U）"
        assert {x["uStart"] for x in layout if x["instance"] == "节点X"} >= {37}


# ---------------------------------------------------------------- 真实数据


_ROOT = __import__("pathlib").Path(__file__).resolve().parents[3]
_INTENT_PATH = _ROOT / "projects" / "v13" / "intent_full.json"


@pytest.fixture(scope="module")
def real_intent() -> dict:
    if not _INTENT_PATH.exists():
        pytest.skip("需要真实意图文件（只在完整仓库里跑）")
    return __import__("json").loads(_INTENT_PATH.read_text(encoding="utf-8"))


def _intent_dict() -> dict:
    return __import__("json").loads(_INTENT_PATH.read_text(encoding="utf-8"))


def _intent_of(doc: dict) -> Intent:
    from idcbox.domain.ports.intent import intent_from_dict

    return intent_from_dict(doc)


class TestRealIntent:
    def test_ch2_rack1_has_eight_slots(self, real_intent: dict) -> None:
        starts = [
            p.u_start
            for p in expand_placements(_intent_of(real_intent))
            if p.channel_no == 2 and p.rack_no == 1
        ]
        assert starts == [3, 7, 11, 15, 19, 23, 27, 31]

    def test_move_real_layout_device(self, real_intent: dict) -> None:
        """真机：ch2 rack1 的 3U 那台挪到 35U，只有它动。"""
        r = move_device(
            real_intent,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=35,
            current_u=3,
        )
        assert r.ok, r.error
        starts = [
            p.u_start
            for p in expand_placements(_intent_of(r.doc))
            if p.channel_no == 2 and p.rack_no == 1
        ]
        assert starts == [35, 7, 11, 15, 19, 23, 27, 31]

    def test_real_overlap_refused(self, real_intent: dict) -> None:
        r = move_device(
            real_intent,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=7,
            current_u=3,
        )
        assert not r.ok
        assert "U7-8 已被占用" in (r.error or "")

    def test_real_canvas_becomes_readonly_exception_only(self, real_intent: dict) -> None:
        """真实意图里只有 7 台「无处可改」，其余 827 台都可拖。"""
        items = editable_items(real_intent)
        assert len(items) == 827
