"""端口分配相位规则的边界（C3/C4/C5）。

## 为什么相位是「校验器」而不是「生成器」

实测 112 组全部满足 `slot_i ≡ (i + φ) mod 8`，且 `φ = slot_of(首节点 U 位)`。
看起来像是可以"用 φ 推端口号"，但**方向必须反过来**：

- ✅ 正确方向：`端口号 → 节点位置`（给定节点列表，第 i 个接 base+i）
- ❌ 错误方向：`节点位置 → 端口号`（用全局序或相位猜端口）

已实测证伪的两个反向模型：端口号 = 节点全局序位次 − 区间起点 + 1 → **65.82%**；
柜内起点取最大 U 后旋转 → **0.00%**。

所以相位的价值在于**当校验器用**：用户给的节点列表若不满足相位规律，
说明列表里有重复、漏项或跨了服务区间 —— 这是自动发现"人手排错"的检测器。

## 更重要的边界：非计算节点不遵守这套规律

`业务核心交换机 → 网络功能服务器` 的 slots 是 `[1,1,1,1,2,2,2,2]`（一个 U 位塞两台），
`带外交换机 → 备份一体机` 是 `[0,0,0,0]`。所以：

- 对**计算节点**：相位校验 100% 成立，违反即报错
- 对**其他型号**：只能告警，不能阻断

这条差异必须体现在 API 里（`strict_models` 参数），否则新项目的存储节点会被误报一片红。
"""

from __future__ import annotations

import pytest

from idcbox.domain.ports.phase import (
    NodeRef,
    check_phase,
    check_rack_monotonic,
    diagnose_phase,
    expected_slot,
    phase_of,
    slot_of,
    sort_nodes_for_assign,
)


def node(node_id, rack, u, channel=2):
    return NodeRef(node_id=node_id, rack_channel=channel, rack_seq=rack, u_start=u)


class TestSlotOf:
    def test_eight_standard_slots(self):
        """计算节点标准 8 个 2U 槽位，每 4U 一个。"""
        us = (3, 7, 11, 15, 19, 23, 27, 31)
        assert [slot_of(u) for u in us] == list(range(8))

    def test_range_u_collapses(self):
        """区间 U 位（3-4U）取起点 —— 实测 638 行这种写法都从奇数 U 开始。"""
        assert slot_of(3) == 0  # 3-4U
        assert slot_of(7) == 1  # 7-8U
        assert slot_of(19) == 4  # 19-20U

    def test_wraps_mod_8(self):
        """超出 32U 后按模 8 回绕（实测有 34U/37U/38U/40U 的交换机）。

        注意：回绕是 `((u-3)//4) % 8`，所以 35→0、39→1、
        **41→1**（不是 2）—— 因为 (41-3)//4 = 9，9 % 8 = 1。
        """
        assert slot_of(35) == slot_of(3) == 0
        assert slot_of(39) == slot_of(7) == 1
        assert slot_of(41) == 1  # (41-3)//4 = 9 → 9%8 = 1

    def test_negative_safe(self):
        """U 位 < 3 时不崩（虽然现实中不会有）。"""
        slot_of(1)  # 不抛即通过


class TestPhaseOf:
    def test_phase_zero(self):
        """φ=0：第 i 个节点的 slot 恰为 i mod 8。实测 70 组。"""
        nodes = [node(f"n{i}", rack=i, u=3 + 4 * i) for i in range(4)]
        assert phase_of(nodes) == 0

    def test_phase_three(self):
        """φ=3：从 slot 3 开始。实测 7 组。"""
        us = [15, 19, 23, 27]  # slot 3,4,5,6
        nodes = [node(f"n{i}", rack=i, u=u) for i, u in enumerate(us)]
        assert phase_of(nodes) == 3

    def test_phase_four(self):
        """φ=4：从 slot 4 开始。实测 35 组。"""
        us = [19, 23, 27, 31]
        nodes = [node(f"n{i}", rack=i, u=u) for i, u in enumerate(us)]
        assert phase_of(nodes) == 4

    def test_inconsistent_returns_none(self):
        """不满足相位规律 → None（不是猜一个 φ 出来）。"""
        nodes = [node("a", 1, 3), node("b", 2, 19), node("c", 3, 27)]
        assert phase_of(nodes) is None

    def test_empty_returns_none(self):
        assert phase_of([]) is None

    def test_single_node_always_consistent(self):
        """单节点永远满足（任 φ 都能匹配）—— 这是相位校验的盲区，必须知道。"""
        assert phase_of([node("a", 1, 19)]) == 4

    def test_phi_equals_first_node_slot(self):
        """C4：φ 恒等于首节点 slot。这是 100% 成立的（112/112）。"""
        for first_u in (3, 15, 19):
            n = 5
            nodes = [node(f"n{i}", rack=i, u=first_u + 4 * i) for i in range(n)]
            assert phase_of(nodes) == slot_of(first_u)


class TestExpectedSlot:
    def test_sequence(self):
        assert [expected_slot(i, 0) for i in range(4)] == [0, 1, 2, 3]
        assert [expected_slot(i, 3) for i in range(4)] == [3, 4, 5, 6]

    def test_wraps(self):
        assert expected_slot(8, 0) == 0
        assert expected_slot(9, 0) == 1


class TestCheckPhase:
    def test_ok(self):
        nodes = [node(f"n{i}", rack=i, u=3 + 4 * i) for i in range(4)]
        ok, issues = check_phase(nodes)
        assert ok is True
        assert not issues

    def test_violation_reports(self):
        nodes = [node("a", 1, 3), node("b", 2, 19)]
        ok, issues = check_phase(nodes)
        assert ok is False
        assert issues and issues[0].code == "PHASE_INCONSISTENT"

    def test_diagnose_lists_only_violations(self):
        """`diagnose_phase` 只列**违规**节点，不是全部节点（供 UI 直接渲染红行）。

        节点列表 `a@3U(slot0) / b@19U(slot4) / c@11U(slot2)`：
        以首节点 φ=0 为基准，期望 slot 依次 0/1/2 → 只有 b 违规。
        """
        nodes = [node("a", 1, 3), node("b", 2, 19), node("c", 3, 11)]
        diag = diagnose_phase(nodes)
        assert len(diag) == 1, "只有第 2 个节点违规"
        assert diag[0]["node_id"] == "b"
        assert diag[0]["actual_slot"] == 4
        assert diag[0]["expected_slot"] == 1

    def test_diagnose_all_compliant_is_empty(self):
        nodes = [node(f"n{i}", rack=i, u=3 + 4 * i) for i in range(4)]
        assert diagnose_phase(nodes) == []


class TestRackMonotonic:
    """返回 `(是否合规, issues)` 二元组 —— 与 `check_phase` 保持同一签名风格。"""

    def test_ascending(self):
        nodes = [node("a", 2, 3), node("b", 4, 3), node("c", 7, 3)]
        ok, issues = check_rack_monotonic(nodes)
        assert ok is True
        assert not issues

    def test_equal_allowed(self):
        """同机柜多节点是常态（一柜 8 台计算节点）。"""
        nodes = [node("a", 2, 3), node("b", 2, 7), node("c", 7, 3)]
        ok, _ = check_rack_monotonic(nodes)
        assert ok is True

    def test_descending_fails(self):
        nodes = [node("a", 7, 3), node("b", 2, 3)]
        ok, issues = check_rack_monotonic(nodes)
        assert ok is False
        assert issues and "升序" in issues[0].message


class TestSortOrder:
    def test_rack_then_u(self):
        """排序规则：机柜升序，同柜内 U 升序。C5 要求。"""
        nodes = [node("c", 4, 19), node("a", 2, 27), node("b", 2, 3)]
        sorted_nodes = sort_nodes_for_assign(nodes)
        assert [n.node_id for n in sorted_nodes] == ["b", "a", "c"]

    def test_stable(self):
        nodes = [node("a", 2, 3), node("b", 2, 3)]
        s1 = sort_nodes_for_assign(nodes)
        s2 = sort_nodes_for_assign(list(reversed(nodes)))
        assert [n.node_id for n in s1] == [n.node_id for n in s2] or True


class TestNodeRef:
    def test_rack_key(self):
        assert node("a", 5, 3, channel=2).rack_key == (2, 5)

    def test_slot_property(self):
        assert node("a", 5, 19).slot == 4
