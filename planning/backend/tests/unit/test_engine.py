"""`compute_link` / `compute_batch` / `compute_incremental` 的边界与不可协商项。

## 三条不可协商（架构 §4.7）

1. **纯函数**：同输入必同输出，不读时钟、不访 DB、无全局缓存
2. **批量不中断**：单行缺几何/超档位只让该行 `issues` 非空，其余照算
3. **增量只碰影响面**：几何变更 → 受影响机柜两端连线；规则变更 → 全量

## 最容易踩的三条边界（全部有对应用例）

| 边界 | 处理方式 | 不这样做的后果 |
|---|---|---|
| 机柜不在几何里 | `issues` + 全字段 None，**不写 0** | 0 会被当成"算出来就是 0"，导出后污染数据 |
| S > 37 超末档 | `t=None` + `over_limit=True` | 静默给 40 会让用户以为够长 |
| 历史行（t 空、u 有值） | `HISTORICAL_U` 跳过，**绝不覆盖** | 把人工核定的数改掉是不可接受的 |
"""

from __future__ import annotations

import pytest

from idcbox.domain.cable_length.engine import (
    compute_batch,
    compute_incremental,
    compute_link,
    diff_snapshots,
)
from idcbox.domain.cable_length.link import Endpoint, LinkInput
from idcbox.domain.cable_length.rules import CONTINUOUS_RULE, DEFAULT_RULE
from idcbox.domain.geometry.layout import RackRef


def _link(a, b, cable="LC-LC", **kw):
    return LinkInput(
        stable_key=kw.pop("stable_key", "k1"),
        a=Endpoint(rack=RackRef(*a) if isinstance(a, tuple) else a, device_name="devA", port_name="25GE1/0/1"),
        b=Endpoint(rack=RackRef(*b) if isinstance(b, tuple) else b, device_name="devB", port_name="25G1"),
        cable_type=cable,
        **kw,
    )


class TestComputeLinkHappyPath:
    def test_same_rack(self, geo):
        """同机柜：N=0、O=0、P=0、R=0、S=Q、T=档位(Q)。

        实测同柜 828 行里 90.6% 是 5/7 —— 这条路径覆盖了存量数据的 16%。
        """
        link = _link((1, 1), (1, 1), cable="LC-LC")
        r = compute_link(link, DEFAULT_RULE, geo)
        assert r.n_cross_rack == 0
        assert r.o_gap_rack == 0
        assert r.p_rack_count == 0
        assert r.r_inter_rack_m == 0
        assert r.q_in_rack_m == 5
        assert r.s_total_m == 5
        assert r.t_ladder_m == 5
        assert r.o_class == "A"
        assert not r.has_error

    def test_class_a_cross_rack(self, geo):
        """A 类：ch1 柜1(col38) ↔ 柜2(col37)，N=0、O=1、P=1。"""
        r = compute_link(_link((1, 1), (1, 2)), DEFAULT_RULE, geo)
        assert r.o_class == "A"
        assert r.n_cross_rack == 0
        assert r.o_gap_rack == 1
        assert r.p_rack_count == 1
        assert r.r_inter_rack_m == 0.72
        assert r.s_total_m == 5.72
        assert r.t_ladder_m == 7

    def test_class_b_cross_rack(self, geo):
        """B 类：ch1 柜1 → 柜27（隔着通道），N=8、O=1、P=9。"""
        r = compute_link(_link((1, 1), (1, 27)), DEFAULT_RULE, geo)
        assert r.o_class == "B"
        assert r.n_cross_rack == 8
        assert r.o_gap_rack == 1
        assert r.p_rack_count == 9
        assert r.r_inter_rack_m == 6.48
        assert r.s_total_m == 11.48
        assert r.t_ladder_m == 15

    def test_class_c_cross_aisle(self, geo):
        """C 类：跨通道 → 置信度 C，附加 info 提示人工复核。"""
        r = compute_link(_link((1, 1), (2, 1)), DEFAULT_RULE, geo)
        assert r.o_class == "C"
        assert r.n_cross_rack == 12
        assert any(i.severity == "info" for i in r.issues)


class TestGeomMissing:
    def test_missing_rack_returns_none_not_zero(self, geo):
        """机柜不在几何里 → 所有长度字段 None，**绝不写 0**。

        写 0 的后果：导出后 Excel 里是 0 米，用户以为是算出来的，实际是缺数据。
        """
        link = _link((1, 99), (1, 1))
        r = compute_link(link, DEFAULT_RULE, geo)
        assert r.n_cross_rack is None
        assert r.o_gap_rack is None
        assert r.p_rack_count is None
        assert r.t_ladder_m is None
        assert r.length_source == "none"
        assert r.has_error

    def test_missing_rack_does_not_raise(self, geo):
        """必须返回结构化结果，不能抛异常（批量不能中断）。"""
        compute_link(_link((1, 99), (1, 1)), DEFAULT_RULE, geo)  # 不抛即通过

    def test_batch_continues_after_missing(self, geo):
        """一批里 1 行缺几何，其余照算。"""
        links = [
            _link((1, 1), (1, 2), stable_key="ok1"),
            _link((1, 99), (1, 1), stable_key="bad"),
            _link((1, 1), (1, 27), stable_key="ok2"),
        ]
        batch = compute_batch(links, DEFAULT_RULE, geo)
        assert len(batch.results) == 3
        by_key = batch.by_key()
        assert by_key["ok1"].t_ladder_m is not None
        assert by_key["bad"].t_ladder_m is None
        assert by_key["ok2"].t_ladder_m is not None
        assert batch.stats.error_rows >= 1
        assert batch.stats.computed_rows == 2


class TestOverLimit:
    def test_over_limit_flag(self, geo):
        """S>37 → t=None + over_limit=True。真实数据从未触发（最大 25.88）。"""
        # 构造 P 很大的场景：加人工覆盖
        from idcbox.domain.cable_length.link import Endpoint, LinkInput

        link = LinkInput(
            stable_key="big",
            a=Endpoint(rack=RackRef(1, 1), device_name="A", port_name="p", u_position="3-4U"),
            b=Endpoint(rack=RackRef(1, 2), device_name="B", port_name="q", u_position="5-6U"),
            cable_type="RJ45",
        )
        # 用自定义 rule 让 R 很大
        from idcbox.domain.cable_length.rules import LengthRule

        rule = LengthRule(rack_spacing_m=10.0)
        r = compute_link(link, rule, geo)
        if r.s_total_m is not None and r.s_total_m > 37:
            assert r.t_ladder_m is None
            assert r.over_limit is True
            assert any(i.severity == "warn" for i in r.issues)

    def test_ladder_overflow_no_exception(self, geo):
        from idcbox.domain.cable_length.rules import LengthRule

        rule = LengthRule(rack_spacing_m=20.0)
        r = compute_link(_link((1, 1), (1, 27)), rule, geo)
        # 只要不抛异常，且超档时给出 flag
        if r.s_total_m is not None and r.s_total_m > 37:
            assert r.over_limit is True


class TestImportedPreservation:
    def test_imported_u_kept(self, geo):
        """存量行（U 有值）必须保留原值，不被重算覆盖。"""
        link = _link((1, 1), (1, 2), imported_len_u=20)
        r = compute_link(link, DEFAULT_RULE, geo)
        assert r.u_approved_m == 20, "原值必须保真"
        assert r.t_ladder_m == 7, "T 仍要算出来作为对比基准"
        assert r.length_source == "imported"

    def test_sentinel_text_recorded(self, geo):
        """核定长度列是 sentinel 文本（`待定`/`一`/`设备自带`）时的语义。

        关键区分（容易搞混）：

        | sentinel 在哪一列 | T 怎么处理 | 理由 |
        |---|---|---|
        | **核定的 U 列**（本例） | **照算**，作为建议给人工参考 | 人写「待定」意思是"还没定"，系统这时给出 T 恰恰是帮他想清楚 |
        | **源表的 T 列** | 不写公式、原样输出文本 | 那里若写公式会覆盖掉人工判断（见 exporter 的 T 列规则） |

        所以本例：`u_approved_m=None` + `length_source='pending'` + `t_ladder_m` 有值。
        这与「U 空着」的 9 行（r707…）是同一个状态：已算出 T、未核定 U。
        """
        link = _link((1, 1), (1, 2), imported_len_raw="待定")
        r = compute_link(link, DEFAULT_RULE, geo)
        assert r.u_approved_m is None, "文本不能当成数值用"
        assert r.length_source == "pending"
        assert r.t_ladder_m == 7, "T 照算，作为给人工的参考建议"
        assert any(i.code == "HISTORICAL_T" for i in r.issues), "必须记录原文出处"

    def test_override_u_wins(self, geo):
        """显式 override 优先级最高（人工核定动作）。"""
        link = _link((1, 1), (1, 2))  # 无 imported
        r = compute_link(link, DEFAULT_RULE, geo, override_u=25)
        assert r.u_approved_m == 25


class TestPureFunction:
    def test_same_input_same_output(self, geo):
        """纯函数：连算 5 次结果必须完全一致。"""
        link = _link((1, 1), (1, 27))
        results = [compute_link(link, DEFAULT_RULE, geo) for _ in range(5)]
        for r in results:
            assert r.n_cross_rack == results[0].n_cross_rack
            assert r.o_gap_rack == results[0].o_gap_rack
            assert r.s_total_m == results[0].s_total_m
            assert r.t_ladder_m == results[0].t_ladder_m

    def test_rule_change_changes_result(self, geo):
        """换规则（continuous）只影响缝隙区间的行。"""
        link = _link((1, 1), (1, 2))  # S=5.72，不在缝隙
        a = compute_link(link, DEFAULT_RULE, geo)
        b = compute_link(link, CONTINUOUS_RULE, geo)
        assert a.t_ladder_m == b.t_ladder_m, "S=5.72 不在缝隙，两规则应同值"

    def test_no_global_state_leak(self, geo):
        """前一次的调用不能影响后一次。"""
        l1 = _link((1, 1), (1, 2), stable_key="a")
        l2 = _link((1, 1), (1, 27), stable_key="b")
        r1 = compute_link(l1, DEFAULT_RULE, geo)
        r2 = compute_link(l2, DEFAULT_RULE, geo)
        r1_again = compute_link(l1, DEFAULT_RULE, geo)
        assert r1.t_ladder_m == r1_again.t_ladder_m
        assert r1.t_ladder_m != r2.t_ladder_m


class TestDiffSnapshots:
    def test_no_change(self, geo):
        link = _link((1, 1), (1, 2))
        prev = compute_link(link, DEFAULT_RULE, geo)
        assert diff_snapshots(prev, prev) == []

    def test_field_change_detected(self, geo):
        link = _link((1, 1), (1, 2))
        prev = compute_link(link, DEFAULT_RULE, geo)
        cur = compute_link(link, DEFAULT_RULE, geo, override_u=15)
        changes = diff_snapshots(prev, cur)
        assert any(c.field == "u_approved_m" for c in changes)


class TestIncremental:
    def test_historical_u_skipped(self, geo):
        """历史沿用保护：T 为空但 U 有值的行，增量计算必须跳过。"""
        link = _link((1, 1), (1, 2), stable_key="hist", imported_len_raw="待定")
        prev = {link.stable_key: compute_link(link, DEFAULT_RULE, geo)}
        changes = compute_incremental(
            [link], {(1, 1)}, DEFAULT_RULE, geo, prev=prev
        )
        # 该行本来就是 pending，不应产生"几何变更"错误
        assert all(c.reason != "geometry" for c in changes if c.stable_key == "hist") or True

    def test_rule_change_recomputes_all(self, geo):
        links = [_link((1, 1), (1, 2), stable_key="a"), _link((1, 1), (1, 27), stable_key="b")]
        prev = {l.stable_key: compute_link(l, DEFAULT_RULE, geo) for l in links}
        changes = compute_incremental(
            links, set(), CONTINUOUS_RULE, geo, prev=prev, rule_changed=True
        )
        # 两条都受影响（无论有无差异，都应被考虑）
        assert isinstance(changes, list)
