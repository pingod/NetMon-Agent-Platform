"""七公式的边界与契约（不读 Excel，全手工构造）。

## 为什么需要这些用例（golden 覆盖不到的）

golden 只用真实数据跑，真实数据里**很多边界从未出现**：
- 没有一行 `S > 37`（实测最大 25.88）→ 超档位分支从未被真实数据触发
- 没有一行机柜不在平面图里 → `GEOM_MISSING` 从未触发
- 没有一行 `cable_type` 未知 → `UNKNOWN_CABLE_TYPE` 只靠政务外网表能触发

这些分支如果不用单元测试锁，就是**上线后第一次遇见才爆炸**的地方。

## 三条铁律的用例

1. **纯函数**：同输入必同输出、不读时钟、无全局状态
2. **批量不中断**：单行出错不能让整批失败
3. **结构化问题**：错误走 `issues` 而不是抛异常
"""

from __future__ import annotations

import math

import pytest

from idcbox.domain.cable_length.formulas import (
    classify_o,
    n_cross_rack,
    o_gap_rack,
    p_rack_count,
    q_in_rack,
    r_inter_rack,
    s_total,
    t_ladder,
)
from idcbox.domain.cable_length.numeric import excel15, feq, to_number
from idcbox.domain.geometry.layout import RackRef


def pos(geo, ch, rack, room=None):
    ref = RackRef(channel_no=ch, seq_no=rack, room_code=room)
    return geo.pos(ref)


class TestClassifyO:
    def test_same_row_is_A(self, geo):
        """同一平面图行 = 同排面对面，`O = |Δcol|`。"""
        a, b = pos(geo, 1, 1), pos(geo, 1, 2)
        assert classify_o(a, b) == "A"

    def test_same_channel_diff_row_is_B(self, geo):
        """同通道跨侧 = 隔着通道面对面，走排端折返。"""
        a, b = pos(geo, 1, 1), pos(geo, 1, 27)
        assert classify_o(a, b) == "B"

    def test_diff_channel_is_C(self, geo):
        """跨通道 = 必须绕行，置信度最低。"""
        a, b = pos(geo, 1, 1), pos(geo, 2, 1)
        assert classify_o(a, b) == "C"

    def test_symmetry(self, geo):
        a, b = pos(geo, 1, 1), pos(geo, 2, 1)
        assert classify_o(a, b) == classify_o(b, a)


class TestNFormula:
    def test_same_row_zero(self, geo):
        """同一平面图行 → N=0（不跨排，无需排间距离）。"""
        assert n_cross_rack(pos(geo, 1, 1), pos(geo, 1, 2), _rule()) == 0

    def test_one_row_apart(self, geo):
        """差 1 行 → N = 4*1+4 = 8。实测 100 行里 98 行如此。"""
        assert n_cross_rack(pos(geo, 1, 1), pos(geo, 1, 27), _rule()) == 8

    def test_two_rows_apart(self, geo):
        """差 2 行 → N = 4*2+4 = 12。实测 11 行里 10 行如此。"""
        assert n_cross_rack(pos(geo, 1, 1), pos(geo, 2, 1), _rule()) == 12

    def test_uses_config_not_hardcoded(self, geo):
        """系数必须来自 rule，不能硬编码 —— 换机房时 N 的系数可能不同。"""
        from idcbox.domain.cable_length.rules import LengthRule

        custom = LengthRule(n_row_coef=5, n_row_extra=3)
        assert n_cross_rack(pos(geo, 1, 1), pos(geo, 1, 27), custom) == 8  # 5*1+3
        assert n_cross_rack(pos(geo, 1, 1), pos(geo, 2, 1), custom) == 13  # 5*2+3

    def test_symmetry(self, geo):
        a, b = pos(geo, 1, 1), pos(geo, 2, 1)
        assert n_cross_rack(a, b, _rule()) == n_cross_rack(b, a, _rule())


class TestOFormula:
    def test_class_a_uses_col_delta(self, geo):
        """A 类：列坐标差的绝对值。柜 1 在 col38、柜 2 在 col37 → O=1。"""
        a, b = pos(geo, 1, 1), pos(geo, 1, 2)
        o, issues = o_gap_rack(a, b, _rule(), "A")
        assert o == 1
        assert not issues

    def test_class_b_uses_rack_end(self, geo):
        """B 类：(CMAX-col_a) + (CMAX-col_b) + 1。

        ch1 柜1 N 侧 col38、柜27 S 侧 col38，CMAX=38 →
        (38-38)+(38-38)+1 = 1。这与实测 r76 的 O=1 完全一致。
        """
        a, b = pos(geo, 1, 1), pos(geo, 1, 27)
        o, _ = o_gap_rack(a, b, _rule(), "B")
        assert o == 1

    def test_class_b_offset_is_two_times_cmax(self, geo):
        """通用式：偏移恒为 `2*CMAX + 1 - (col_a + col_b) - |Δcol|`，即到排端往返。"""
        a, b = pos(geo, 1, 1), pos(geo, 1, 27)
        cmax = 38
        expected = (cmax - a.col) + (cmax - b.col) + 1
        assert o_gap_rack(a, b, _rule(), "B")[0] == expected

    def test_class_c_same_formula_but_info(self, geo):
        """C 类公式与 B 类相同，但必须附加「建议人工复核」提示。"""
        a, b = pos(geo, 1, 1), pos(geo, 2, 1)
        o, issues = o_gap_rack(a, b, _rule(), "C")
        assert o == (38 - 38) + (38 - 38) + 1
        assert any(i.severity == "info" for i in issues), "C 类必须提示人工复核"

    def test_manual_only_mode_returns_none(self, geo):
        """降级开关：C 类只给空值 + 强制人工。这是唯一允许的降级开关。"""
        from idcbox.domain.cable_length.rules import LengthRule

        rule = LengthRule(o_cross_aisle_mode="manual_only")
        a, b = pos(geo, 1, 1), pos(geo, 2, 1)
        o, issues = o_gap_rack(a, b, rule, "C")
        assert o is None
        assert any("人工" in i.message for i in issues)

    def test_missing_cmax_raises_issue_not_exception(self, geo):
        """CMAX 缺失 → 结构化 error，绝不抛异常（批量不能中断）。"""
        from idcbox.domain.cable_length.rules import LengthRule

        rule = LengthRule(cmax_by_room={})  # 故意清空
        a, b = pos(geo, 1, 1), pos(geo, 1, 27)
        o, issues = o_gap_rack(a, b, rule, "B")
        assert o is None
        assert any(i.severity == "error" for i in issues)

    def test_override_takes_precedence(self, geo):
        """人工覆盖必须优先于几何公式（经验值 > 算出来的值）。"""
        from idcbox.domain.cable_length.rules import LengthRule

        rule = LengthRule(cross_aisle_overrides={(1, 2): 9})
        a, b = pos(geo, 1, 1), pos(geo, 2, 1)
        assert o_gap_rack(a, b, rule, "C")[0] == 9


class TestPQRST:
    def test_p_is_sum(self):
        assert p_rack_count(8, 1) == 9
        assert p_rack_count(0, 0) == 0
        assert p_rack_count(12, 5) == 17

    def test_q_lookup(self, rule):
        assert q_in_rack("RJ45", rule)[0] == 7
        assert q_in_rack("LC-LC", rule)[0] == 5
        assert q_in_rack("MPO", rule)[0] == 5

    def test_q_unknown_type_warns(self, rule):
        """未知线缆类型 → 用默认值 + 警告，不是崩溃。"""
        q, issues = q_in_rack("40GMPO线缆", rule)
        assert q == rule.default_unknown_q
        assert issues and issues[0].severity == "warn"

    def test_q_override(self, rule):
        assert q_in_rack("RJ45", rule, override=9.5)[0] == 9.5

    def test_r_uses_excel15(self, rule):
        """R = P*0.6*1.2 必须走 excel15，否则 170 行会与 Excel 不符。"""
        assert r_inter_rack(5, rule) == 3.6
        assert r_inter_rack(3, rule) == 2.16
        # 直接算会得到 3.5999999999999996
        assert 5 * 0.6 * 1.2 != 3.6
        assert excel15(5 * 0.6 * 1.2) == 3.6

    def test_s_uses_excel15(self):
        assert s_total(3.6, 5) == 8.6
        assert s_total(2.88, 5) == 7.88

    def test_r_scaling_linear(self, rule):
        """R 对 P 线性 —— 0.72 这个系数是常数，但不能硬编码（可能按机房调）。"""
        for p in (1, 5, 17, 40):
            assert r_inter_rack(p, rule) == excel15(p * 0.72)

    def test_t_normal(self, rule):
        assert t_ladder(5, rule) == 5
        assert t_ladder(7.88, rule) == 10

    def test_t_overflow(self, rule):
        """S>37 原公式无定义（IFS 无 else 分支）→ 返回 None，交人工定档。"""
        assert t_ladder(37.5, rule) is None
        assert t_ladder(100, rule) is None


class TestOClassTolerance:
    def test_b_and_c_share_formula(self, geo):
        """B 与 C 用同一个公式，差别只在置信度与产品行为。"""
        a, bb = pos(geo, 1, 1), pos(geo, 1, 27)
        c, d = pos(geo, 1, 1), pos(geo, 2, 1)
        assert o_gap_rack(a, bb, _rule(), "B")[0] == o_gap_rack(c, d, _rule(), "C")[0]


def _rule():
    from idcbox.domain.cable_length.rules import DEFAULT_RULE

    return DEFAULT_RULE
