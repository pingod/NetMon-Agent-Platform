"""批量拖动 —— 一批设备整体平移。

## 这组用例守的是「必须批量」这件事本身

如果只是把单台调用套个循环，下面第一条就会失败：8 台整体下移时，
每台的新位置恰好是下一台的旧位置。逐台校验时第 1 台看到第 2 台还站在
那里，第 2 台看到第 3 台 —— 一台都动不了，而用户只是把一摞机器往下挪。

## 第二条守的是「失败不留半改」

批量最容易出的错是：前 3 台成功了、第 4 台失败，返回一个改了 3 台的 doc。
用户看到界面上一半设备动了、一半没动，比整体失败更难收拾。
"""

from __future__ import annotations

import pytest

from idcbox.domain.ports.intent import (
    DeviceSpec,
    Intent,
    RackBlock,
    expand_placements,
    intent_to_dict,
)
from idcbox.domain.ports.moveu import (
    RACK_U_HEIGHT,
    DeviceTarget,
    move_device,
    move_devices,
    occupied_spans,
)


def _doc_with_nodes(rack_from: int = 1, rack_to: int = 2) -> dict:
    """两个机柜各 8 台计算节点（槽位 0..7 → U 3,7,11,...,31）。"""
    intent = Intent(
        project="t",
        racks=(RackBlock(2, rack_from, rack_to, "计算节点A（ARM）"),),
    )
    return intent_to_dict(intent)


def _targets(rack: int, us: tuple[int, ...]) -> list[DeviceTarget]:
    return [
        DeviceTarget(channel_no=2, rack_no=rack, instance="计算节点A（ARM）", current_u=u)
        for u in us
    ]


class TestBatchShift:
    def test_shift_whole_stack_down_by_one_slot(self) -> None:
        """★ 整摞下移一格：每台的新位置都是下一台的旧位置。

        逐台挪必然失败（第 1 台撞第 2 台），批量成功。
        """
        doc = _doc_with_nodes()
        targets = _targets(1, (27, 31))
        single = move_device(
            doc, channel_no=2, rack_no=1, instance="计算节点A（ARM）",
            new_u=31, current_u=27,
        )
        assert single.ok is False
        assert "31-32" in (single.error or "")

        res = move_devices(doc, targets=targets, delta_u=4)
        assert res.ok is True, res.error
        assert len(res.moved) == 2
        assert res.moved[0] == ("计算节点A（ARM）", 1, 27, 1, 31)
        assert res.moved[1] == ("计算节点A（ARM）", 1, 31, 1, 35)

    def test_all_eight_nodes_shift_up_by_one_slot(self) -> None:
        """8 台一起上移 4U：3→7, 7→11, ..., 31→35。"""
        doc = _doc_with_nodes()
        us = (3, 7, 11, 15, 19, 23, 27, 31)
        res = move_devices(doc, targets=_targets(1, us), delta_u=4)
        assert res.ok is True, res.error
        assert [m[4] for m in res.moved] == [7, 11, 15, 19, 23, 27, 31, 35]

    def test_shift_partial_stack_leaves_the_rest(self) -> None:
        """只搬上面 4 台：下面 4 台仍在原位 —— 不能顺手把没选的也挪了。"""
        doc = _doc_with_nodes()
        res = move_devices(doc, targets=_targets(1, (19, 23, 27, 31)), delta_u=4)
        assert res.ok is True, res.error
        assert [m[4] for m in res.moved] == [23, 27, 31, 35]
        # 未选的 3/7/11/15 必须原地不动。搬完后「按位置反查槽位 id」
        # 会看到 3/7/11/15 仍对应 S0/S1/S2/S3。
        from idcbox.domain.ports.intent import intent_from_dict

        placed = {
            p.node_id: (p.rack_no, p.u_start)
            for p in expand_placements(intent_from_dict(res.doc))
        }
        assert placed["N02-01-S0"] == (1, 3)
        assert placed["N02-01-S1"] == (1, 7)
        assert placed["N02-01-S2"] == (1, 11)
        assert placed["N02-01-S3"] == (1, 15)


class TestBatchRejects:
    def test_out_of_range_reports_offending_member(self) -> None:
        doc = _doc_with_nodes()
        res = move_devices(doc, targets=_targets(1, (27, 31)), delta_u=40)
        assert res.ok is False
        assert "超出机柜" in (res.error or "")
        assert "27U" in (res.error or "")  # 指出是哪一台越界

    def test_no_half_change_on_failure(self) -> None:
        """★ 失败时 doc 原样返回 —— 不允许「前几台成功了」。"""
        doc = _doc_with_nodes()
        before = intent_to_dict(Intent(project="t", racks=(
            RackBlock(2, 1, 2, "计算节点A（ARM）"),
        )))
        res = move_devices(doc, targets=_targets(1, (27, 31)), delta_u=40)
        assert res.ok is False
        assert res.doc == before
        assert res.moved == ()

    def test_empty_targets(self) -> None:
        res = move_devices(_doc_with_nodes(), targets=[], delta_u=4)
        assert res.ok is False
        assert "没有选中" in (res.error or "")

    def test_zero_delta(self) -> None:
        res = move_devices(_doc_with_nodes(), targets=_targets(1, (3,)), delta_u=0)
        assert res.ok is False
        assert "移动量为 0" in (res.error or "")

    def test_cross_channel_rejected(self) -> None:
        """一批里有 2 个通道 → 拒绝。换通道是重规划，不是拖动。"""
        doc = _doc_with_nodes()
        targets = [
            DeviceTarget(2, 1, "计算节点A（ARM）", 3),
            DeviceTarget(8, 1, "计算节点A（ARM）", 3),
        ]
        res = move_devices(doc, targets=targets, delta_u=4)
        assert res.ok is False
        assert "通道" in (res.error or "")

    def test_mixed_source_racks_without_target(self) -> None:
        """一批来自两个机柜又没给目标柜 → 拒绝并说清要一个共同目标。"""
        doc = _doc_with_nodes()
        targets = _targets(1, (3,)) + _targets(2, (3,))
        res = move_devices(doc, targets=targets, delta_u=4)
        assert res.ok is False
        assert "共同的目标机柜" in (res.error or "")

    def test_unknown_device(self) -> None:
        doc = _doc_with_nodes()
        targets = [DeviceTarget(2, 1, "不存在的设备", 3)]
        res = move_devices(doc, targets=targets, delta_u=4)
        assert res.ok is False
        assert "找不到" in (res.error or "")

    def test_collides_with_static_neighbour(self) -> None:
        """整批挪到别人头上 → 拒绝，并指出是谁占着。"""
        intent = Intent(
            project="t",
            racks=(RackBlock(2, 1, 1, "计算节点A（ARM）"),),
            devices=(DeviceSpec("核心交换机-1", "业务核心交换机", 2, 1, u_start=39),),
        )
        doc = intent_to_dict(intent)
        res = move_devices(doc, targets=_targets(1, (31,)), delta_u=8)
        assert res.ok is False
        assert "占用" in (res.error or "")


class TestBatchAcrossRacks:
    def test_whole_rack_moves_to_another_rack(self) -> None:
        """整柜 8 台搬到另一个机柜（机柜 2 本来也是满的，所以整体替换不可能 ——
        这是真实约束，改用只搬 2 台到机柜 2 的空位）。"""
        intent = Intent(
            project="t",
            racks=(
                RackBlock(2, 1, 1, "计算节点A（ARM）"),
                # 机柜 2 只放 6 台 → 35/39 空着
                RackBlock(2, 2, 2, "计算节点A（ARM）", slots=(0, 1, 2, 3, 4, 5)),
            ),
        )
        doc = intent_to_dict(intent)
        res = move_devices(
            doc, targets=_targets(1, (3, 7)), delta_u=32, target_rack_no=2
        )
        assert res.ok is True, res.error
        assert res.moved[0][3] == 2  # toRack
        assert res.moved[0][4] == 35

    def test_node_id_survives_cross_rack_batch(self) -> None:
        intent = Intent(
            project="t",
            racks=(
                RackBlock(2, 1, 1, "计算节点A（ARM）"),
                RackBlock(2, 2, 2, "计算节点A（ARM）", slots=(0, 1, 2, 3, 4, 5)),
            ),
        )
        doc = intent_to_dict(intent)
        res = move_devices(
            doc, targets=_targets(1, (3,)), delta_u=32, target_rack_no=2
        )
        assert res.ok is True, res.error
        placed = {
            p.node_id: (p.rack_no, p.u_start)
            for p in expand_placements(__import__(
                "idcbox.domain.ports.intent", fromlist=["x"]
            ).intent_from_dict(res.doc))
        }
        # 槽位 0 那台（N02-01-S0）搬到了机柜 2 的 35U，身份没变
        assert placed["N02-01-S0"] == (2, 35)
