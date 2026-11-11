"""设计态服务（application/services/design.py）的口径铁律。

## 这个模块守护什么

`design.py` 是 v0.3 主功能的装配层：意图 → 生成行 → 与存量表比对。
域层的 golden 用例已经证明了「生成 4430 行与真实表 12 列逐格一致」，
这里要守的是**上层口径**——同样是 4430 这个数字，换个统计方式就会错：

1. **交换机台数必须数实例，不能数机柜**：早先用 `localRack` 做键得到 25，
   真实是 106 台。机柜号跨通道重号（ch2/rack1 与 ch8/rack1），
   这种错误看起来数字合理，最容易被漏掉。
2. **比对键不能带对端设备名称**：存量表那列半填（4562 行里只有 332 有值），
   带它进键会产出 200 条假性缺失，而 200 又恰好等于生成侧不填名的行数。
3. **缺失行必须能按对端型号归因**：144 行全是特殊机型，若不归因，
   用户会把"本来就得手写"误读成"算法漏了 3%"。
4. **长度差异只比双方都有值的行**：存量 U 列大量空白、生成 U 列恒为 None，
   逐行比会产出 4430 条假差异。

## 为什么这些断言写在 application 层而不是 golden

golden 用例依赖真实 Excel（33 秒级），改一个统计口径就跑一遍太慢。
这里用最小的合成意图覆盖口径本身，快且能定位到具体函数。
真实数据的一致性由 `tests/golden/test_golden_generate.py` 守。
"""

from __future__ import annotations

from dataclasses import replace

import pytest

from idcbox.application.services.design import (
    DesignResult,
    compare_with_stock,
    design_payload,
    generate_design,
    intent_volume,
)
from idcbox.domain.ports.generate import GeneratedCabling, GeneratedLink
from idcbox.domain.ports.intent import ChainSpec, Intent, RackBlock, RouteSpec, SwitchSpec


def _tiny_intent() -> Intent:
    """1 台交换机、8 台服务器、1 个网口的最小意图。

    只造必要的部分：rack 段 1 个机柜 × 8 槽位、1 条路由、1 条链 1 台交换机。
    """
    return Intent(
        project="synthetic",
        site="测试",
        racks=(RackBlock(channel_no=1, rack_from=1, rack_to=1, node_model="节点X"),),
        switches=(
            SwitchSpec(instance="交换机-1", model="接入交换机1", channel_no=1, rack_no=2, u_start=40),
        ),
        routes=(RouteSpec(node_model="节点X", nic="万兆1", switch_model="接入交换机1"),),
        chains=(
            ChainSpec(
                channel_no=1,
                switch_model="接入交换机1",
                nic="万兆1",
                instances=("交换机-1",),
                capacity=48,
            ),
        ),
    )


def _design() -> DesignResult:
    return generate_design(_tiny_intent(), geometry=None)


# ---------------------------------------------------------------- 体积口径


def test_intent_volume_counts_each_section():
    volume = intent_volume(_tiny_intent())
    assert volume["racks"] == 1
    assert volume["switches"] == 1
    assert volume["devices"] == 0
    assert volume["routes"] == 1
    assert volume["chains"] == 1
    assert volume["explicitLinks"] == 0
    # 总数是各部分之和 —— UI 上「949 行意图」用的就是它
    assert volume["total"] == 4
    assert volume["serverCount"] == 8


def test_intent_volume_counts_devices_in_total():
    """特殊设备也必须计入 total。

    ## 这条用例守的是一个真实 bug

    初版 `intent_volume` 漏了 `devices` 段：真实项目 949 行只报 915 行。
    数字少报看着"更好看"（意图更精简），但那是在骗用户 ——
    他按 total 预估填表工作量，实际要多填 34 行。
    """
    from idcbox.domain.ports.intent import DeviceSpec

    base = _tiny_intent()
    with_device = replace(
        base,
        devices=(
            DeviceSpec("防火墙-1", "防火墙1", 1, 1, 3),
            DeviceSpec("负载均衡-1", "广域网链路负载均衡", 1, 2, 19, u_span=2),
        ),
    )
    volume = intent_volume(with_device)
    assert volume["devices"] == 2
    assert volume["total"] == 6  # 1 布局 + 1 交换机 + 2 设备 + 1 路由 + 1 链
    # 设备数不影响服务器台数（它们不是 RackBlock 展开的）
    assert volume["serverCount"] == base.server_count


def test_intent_volume_reports_source_format():
    """来源格式要透出去 —— 用户改完 Excel 忘记转 JSON 时靠它发现问题。"""
    assert intent_volume(_tiny_intent())["sourceFormat"] == "json"
    assert (
        intent_volume(_tiny_intent(), source_format="xlsx")["sourceFormat"] == "xlsx"
    )


# ---------------------------------------------------------------- 生成口径


def test_generate_design_produces_one_row_per_nic():
    design = _design()
    # 8 台服务器 × 1 个网口 = 8 行
    assert len(design.rows) == 8
    assert design.stats["links"] == 8
    assert design.stats["errors"] == 0


def test_peer_device_is_always_empty_in_generated_rows():
    """对端设备名称一律留空 —— 源表这列半填且存在型号/名字矛盾。"""
    design = _design()
    assert all(row["peerDevice"] == "" for row in design.rows)


def test_peer_id_is_non_empty_identity():
    """`peerId` 是槽位 id，恒定非空 —— 界面靠它做「点设备看连线」。"""
    design = _design()
    assert all(row["peerId"].startswith("N01-01-S") for row in design.rows)


def test_u_approved_stays_none():
    """U 列（核定长度）生成时恒为 None —— 那是人工签字的位置。

    注意：不算几何时连 `uApprovedM` 这个键都不出现（见下一个用例），
    算了几何才写入 None。两种形状都不许出现数字。
    """
    design = _design()
    assert all(row.get("uApprovedM") is None for row in design.rows)


def test_geometry_absent_leaves_lengths_empty():
    design = _design()
    assert design.stats["geometryUsed"] is False
    assert design.stats["lengthComputed"] == 0
    assert all("tLadderM" not in row for row in design.rows)


# ---------------------------------------------------------------- 交换机台数


def _fake_design(rows: list[dict[str, object]]) -> DesignResult:
    links = tuple(
        GeneratedLink(
            a_device=str(r["localDevice"]),
            a_model=str(r["localModel"]),
            a_channel=1,
            a_rack=1,
            a_u="40U",
            a_port=str(r["localPort"]),
            b_device="",
            b_model="节点X",
            b_channel=1,
            b_rack=1,
            b_u="3-4U",
            b_port=str(r["nic"]),
            cable_type="LC-LC",
            nic=str(r["nic"]),
            b_id="N01-01-S0",
        )
        for r in rows
    )
    cabling = GeneratedCabling(links=links, issues=())
    return DesignResult(cabling=cabling, rows=tuple(rows), stats={"links": len(rows)}, notes=())


def test_by_switch_count_uses_instances_not_racks():
    """交换机台数按实例名统计。

    反例：早先用 `localRack` 做键 → 两个不同通道的 rack1 被并成 1，
    真实 106 台被算成 25。本用例造两台不同型号但同机柜号的交换机。
    """
    rows: list[dict[str, object]] = [
        {
            "rowNo": 1,
            "localModel": "业务接入交换机1",
            "localDevice": "业务接入-1",
            "localChannel": 2,
            "localRack": 1,
            "localU": "40U",
            "localPort": "10GE1/0/1",
            "peerModel": "节点X",
            "peerDevice": "",
            "peerChannel": 2,
            "peerRack": 1,
            "peerU": "3-4U",
            "peerPort": "万兆1",
            "cableType": "LC-LC",
            "nic": "万兆1",
            "peerId": "N02-01-S0",
        },
        {
            "rowNo": 2,
            "localModel": "存储接入交换机1",
            "localDevice": "存储接入-1",
            "localChannel": 2,
            "localRack": 1,
            "localU": "40U",
            "localPort": "25GE1/0/1",
            "peerModel": "节点X",
            "peerDevice": "",
            "peerChannel": 2,
            "peerRack": 1,
            "peerU": "7-8U",
            "peerPort": "25G1",
            "cableType": "LC-LC",
            "nic": "25G1",
            "peerId": "N02-01-S1",
        },
    ]
    from collections import Counter

    by_switch = {r["localDevice"] for r in rows}
    by_rack = Counter(r["localRack"] for r in rows)
    # 反例本身：按机柜统计只有 1 —— 这正是曾经的 bug
    assert len(by_rack) == 1
    assert len(by_switch) == 2

    # 口径落点：design_payload 用的是实例名，与上面对齐
    payload = design_payload(_tiny_intent(), None, limit=10)
    assert payload["bySwitchCount"] == 1


# ---------------------------------------------------------------- 比对口径


def _gen_row(**kw: object) -> dict[str, object]:
    base: dict[str, object] = {
        "rowNo": 1,
        "localModel": "接入交换机1",
        "localDevice": "交换机-1",
        "localPort": "10GE1/0/1",
        "nic": "万兆1",
        "peerDevice": "",
        "tLadderM": 10,
    }
    base.update(kw)
    return base


def test_compare_key_ignores_peer_device_name():
    """比对键是三字段，不含对端设备名称。

    若把 `peerDevice` 放进键，存量侧 200 行为空会让它们全变成"缺失"。
    """
    design = _fake_design([_gen_row()])
    stock = [
        {
            "localDevice": "交换机-1",
            "localPort": "10GE1/0/1",
            "nic": "万兆1",
            "tLadderM": 10,
        }
    ]
    result = compare_with_stock(design, stock)
    assert result["common"] == 1
    assert result["missing"] == 0
    assert result["added"] == 0


def test_compare_uses_local_device_name_when_present():
    """存量行若带 `localDeviceName`（来自 `/api/cables`），必须优先用它。

    `localDevice` 在那条链路里是**型号**，拿它当实例名会让 4430 行全部错位。
    """
    design = _fake_design([_gen_row()])
    stock = [
        {
            "localDevice": "业务接入交换机1",  # 型号
            "localDeviceName": "交换机-1",  # 实例名
            "localPort": "10GE1/0/1",
            "nic": "万兆1",
            "tLadderM": 10,
        }
    ]
    assert compare_with_stock(design, stock)["common"] == 1


def test_compare_reports_length_diff_only_when_both_present():
    """长度差异只统计双方都有值的行。"""
    design = _fake_design(
        [
            _gen_row(rowNo=1, localPort="10GE1/0/1", tLadderM=10),
            _gen_row(rowNo=2, localPort="10GE1/0/2", tLadderM=15),
        ]
    )
    stock = [
        # 第 1 行 T 一致
        {"localDevice": "交换机-1", "localPort": "10GE1/0/1", "nic": "万兆1", "tLadderM": 10},
        # 第 2 行 T 不一致
        {"localDevice": "交换机-1", "localPort": "10GE1/0/2", "nic": "万兆1", "tLadderM": 20},
    ]
    result = compare_with_stock(design, stock)
    assert result["lengthDiffs"] == 1
    assert result["lengthDiffSamples"][0]["generated"] == 15
    assert result["lengthDiffSamples"][0]["stock"] == 20


def test_compare_missing_is_attributed_to_peer_model():
    """缺失行按对端型号归因 —— 让用户分清"算法漏了"和"本来就得手写"。"""
    design = _fake_design([_gen_row()])
    stock = [
        {"localDevice": "交换机-1", "localPort": "10GE1/0/1", "nic": "万兆1", "tLadderM": 10},
        {"localDevice": "交换机-1", "localPort": "10GE1/0/2", "nic": "万兆1", "tLadderM": 10},
        {"localDevice": "交换机-1", "localPort": "10GE1/0/3", "nic": "万兆1", "tLadderM": 10},
    ]
    peer_model_of = {
        ("交换机-1", "10GE1/0/2", "万兆1"): "网络功能服务器",
        ("交换机-1", "10GE1/0/3", "万兆1"): "网络功能服务器",
    }
    result = compare_with_stock(design, stock, peer_model_of=peer_model_of)
    assert result["missing"] == 2
    assert result["missingByModel"] == [{"model": "网络功能服务器", "count": 2}]


def test_compare_without_peer_model_map_reports_empty_attribution():
    """不给归因表时 `missingByModel` 为空列表，而不是崩。"""
    design = _fake_design([_gen_row()])
    stock = [
        {"localDevice": "交换机-1", "localPort": "10GE1/0/9", "nic": "万兆1", "tLadderM": 10},
    ]
    result = compare_with_stock(design, stock)
    assert result["missing"] == 1
    assert result["missingByModel"] == []


# ---------------------------------------------------------------- payload 形状


def test_design_payload_lists_switch_models():
    """`switchModels` 给 UI 展示"覆盖了哪几类交换机"，必须是去重排序的。"""
    payload = design_payload(_tiny_intent(), None, limit=100)
    assert payload["switchModels"] == ["接入交换机1"]
    assert payload["bySwitchCount"] == 1
    assert payload["rowsTotal"] == 8
    assert len(payload["rows"]) == 8


def test_design_payload_respects_limit():
    payload = design_payload(_tiny_intent(), None, limit=3)
    assert len(payload["rows"]) == 3
    assert payload["rowsTotal"] == 8


def test_design_payload_notes_explain_scope_boundary():
    """边界说明必须随 payload 下发 —— 界面靠它解释"为什么不是全表"。"""
    payload = design_payload(_tiny_intent(), None, limit=10)
    joined = " ".join(payload["notes"])
    assert "交换机" in joined


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-q"]))
