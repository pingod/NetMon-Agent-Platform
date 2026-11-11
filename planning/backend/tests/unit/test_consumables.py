"""耗材汇总的边界与口径（纯域层用例，不需要真实源文件）。

## 这组用例守护的四个「差一格就出错」的地方

1. **长度并入档位**：35 米并入 40 米（原表没有 35 米档）。超最大档则原样保留，
   不能静默截断到 40 —— 那会让 50 米的线记成 40 米，采购买短了就接不上。
2. **向上取整不能浮点误伤**：`100 * 1.3 = 130.00000000000003`，
   直接 `ceil` 会得 131。必须用整数比较。
3. **`一` 与 `设备自带` 算「已覆盖」但不产生需求**：
   把 sentinel 算成未覆盖会让覆盖率虚低，用户以为还要补几百行。
4. **空长度必须跳过而不是当 0**：把空当 0 会并入最小档 5 米，凭空多买一批短线。
"""

from __future__ import annotations

import pytest

from idcbox.domain.exporting.consumables import (
    NON_PROCURE_MODULES,
    PROCURE_MODULES,
    PURCHASE_LADDER,
    build_consumable_report,
    cable_demand,
    module_demand,
)


class TestLengthFolding:
    """长度并入采购档位。"""

    @pytest.mark.parametrize(
        ("raw", "want"),
        [
            (5, 5), (6, 7), (7, 7), (8, 10), (10, 10), (13, 15), (15, 15),
            (20, 20), (25, 25), (30, 30), (35, 40), (40, 40),
        ],
    )
    def test_fold_to_purchase_tier(self, raw: int, want: int) -> None:
        """35 米并入 40 米 —— 原表没有 35 米采购档。"""
        demand = cable_demand([("LC-LC", raw)])
        assert [r.length_m for r in demand.rows] == [want]

    def test_over_max_tier_kept_as_is(self) -> None:
        """超过 40 米的长度原样保留，不截断到 40。

        截断会让「50 米的线」记成「40 米的线」，采购买了 40 米现场接不上。
        宁可报出一个非标档位让人看到，也不能静默归到 40。
        """
        demand = cable_demand([("LC-LC", 50)])
        assert [r.length_m for r in demand.rows] == [50]

    def test_none_length_is_skipped(self) -> None:
        """`None` 长度整行跳过，不能当 0 并入 5 米档。

        把空当 0 会让每个没填长度的行都产生一根 5 米线的需求 —— 凭空多买。
        """
        demand = cable_demand([("LC-LC", None), ("LC-LC", 10)])
        assert demand.total_count == 1
        assert [(r.length_m, r.count) for r in demand.rows] == [(10, 1)]

    def test_unknown_type_is_kept(self) -> None:
        """未知线缆类型不丢弃，标成 `UNKNOWN` 让人看到。"""
        demand = cable_demand([(None, 10)])
        assert [r.cable_type for r in demand.rows] == ["UNKNOWN"]


class TestCeilMultiply:
    """建议采购量的取整。"""

    @pytest.mark.parametrize(
        ("n", "ratio", "want"),
        [
            (100, 1.3, 130),   # 100*1.3 在浮点里是 130.00000000000003
            (10, 1.3, 13),     # 10*1.3 = 13.000000000000002
            (1, 1.3, 2),
            (7, 1.3, 10),      # 9.1 → 10
        ],
    )
    def test_no_float_overshoot(self, n: int, ratio: float, want: int) -> None:
        """浮点误差不能把 130 抬成 131。

        `100 * 1.3 == 130.00000000000003`，若直接用 `math.ceil` 会得 131。
        采购单上多买一根是小事，但同类问题出现在 10000 根时就是一万根。
        """
        demand = cable_demand([("LC-LC", 10)] * n)
        got = demand.purchase_suggestion(ratio)
        assert got[0].count == want

    def test_zero_demand_produces_no_row(self) -> None:
        """没有需求的行不会出现 —— 与「需求 0 但建议采购 0」是两件事。

        域层只报「有需求的组合」，补零行是导出层的展示决定（见 `matrix`）。
        """
        assert cable_demand([]).rows == ()
        assert cable_demand([]).purchase_suggestion(1.3) == ()

    def test_suggestion_does_not_mutate_demand(self) -> None:
        """`purchase_suggestion` 不能改原始需求值。"""
        demand = cable_demand([("LC-LC", 10)] * 3)
        before = [(r.cable_type, r.length_m, r.count) for r in demand.rows]
        demand.purchase_suggestion(2.0)
        after = [(r.cable_type, r.length_m, r.count) for r in demand.rows]
        assert before == after


class TestMatrix:
    """`matrix()` 的两种形态。"""

    def test_without_zero_only_reports_existing(self) -> None:
        m = cable_demand([("LC-LC", 10)]).matrix()
        assert m == {"LC-LC": {10: 1}}

    def test_with_zero_fills_all_tiers(self) -> None:
        """补全 8 个档位，复现原表"0 也列出来"的形态。"""
        m = cable_demand([("LC-LC", 10)]).matrix(include_zero=True)
        assert set(m["LC-LC"]) == set(PURCHASE_LADDER)
        assert m["LC-LC"][5] == 0
        assert m["LC-LC"][10] == 1


class TestModuleDemand:
    """模块需求与覆盖率。"""

    def test_sentinel_counted_as_covered_not_demanded(self) -> None:
        """`一` / `设备自带` 计入覆盖，但不产生需求行。"""
        m = module_demand(
            [("一", "设备自带"), ("10G-SR", None)], total_rows=2
        )
        assert m.coverage_rows == 2
        assert [r.module for r in m.rows] == ["10G-SR"]

    def test_empty_row_not_covered(self) -> None:
        """两端都空 = 没填，不计覆盖。"""
        m = module_demand([("", ""), ("10G-SR", "")], total_rows=2)
        assert m.coverage_rows == 1

    def test_unknown_module_is_counted(self) -> None:
        """不在 `PROCURE_MODULES` 里的模块也计入需求，用原值。

        静默丢掉会让「100G-LR4」这种少见的模块凭空消失。
        """
        m = module_demand([("100G-XR4", "")], total_rows=1)
        assert [r.module for r in m.rows] == ["100G-XR4"]

    def test_local_and_peer_kept_separate(self) -> None:
        """本端/对端必须分列 —— 合并后无法解释"为什么是 2 倍"。"""
        m = module_demand([("100G-SR4", "设备自带")], total_rows=1)
        row = m.rows[0]
        assert (row.local_count, row.peer_count) == (1, 0)
        assert row.total == 1

    def test_disclosure_mentions_no_extrapolation(self) -> None:
        m = module_demand([("10G-SR", "")], total_rows=100)
        text = m.disclosure()
        assert "1/100" in text
        assert "未做外推" in text

    def test_fully_covered(self) -> None:
        m = module_demand([("10G-SR", "")], total_rows=1)
        assert m.is_fully_covered()

    def test_zero_total_rows_does_not_divide_by_zero(self) -> None:
        m = module_demand([], total_rows=0)
        assert m.coverage_ratio == 0.0


class TestBuildReport:
    """一次遍历同时算两者的组合入口。"""

    def test_cables_and_modules_together(self) -> None:
        rep = build_consumable_report(
            [
                ("LC-LC", 10, "10G-SR", "设备自带"),
                ("LC-LC", 10, "10G-SR", ""),
                ("RJ45", 5, "一", "一"),
            ],
            total_rows=3,
        )
        assert rep.cables.total_count == 3
        assert dict(rep.cables.by_type()) == {"LC-LC": 2, "RJ45": 1}
        mods = {r.module: (r.local_count, r.peer_count) for r in rep.modules.rows}
        assert mods == {"10G-SR": (2, 0)}

    def test_total_rows_defaults_to_line_count(self) -> None:
        """省略 `total_rows` 时按行数兜底（覆盖率会虚高，生产路径必须显式传）。"""
        rep = build_consumable_report([("LC-LC", 10, "", "")])
        assert rep.modules.total_rows == 1
        assert rep.modules.coverage_rows == 0

    def test_purchase_rows_use_report_ratio(self) -> None:
        rep = build_consumable_report([("LC-LC", 10, "", "")] * 10, total_rows=10)
        rep2 = build_consumable_report(
            [("LC-LC", 10, "", "")] * 10, total_rows=10, purchase_ratio=2.0
        )
        assert rep.cable_purchase_rows()[0].count == 13
        assert rep2.cable_purchase_rows()[0].count == 20


class TestConstants:
    """常量表本身的语义。"""

    def test_purchase_ladder_has_eight_tiers(self) -> None:
        """8 档（无 35）。展示档位是 9 档（含 35），两者不能混。"""
        assert PURCHASE_LADDER == (5, 7, 10, 15, 20, 25, 30, 40)
        assert len(PURCHASE_LADDER) == 8

    def test_sentinel_sets_are_disjoint(self) -> None:
        """`PROCURE_MODULES` 与 `NON_PROCURE_MODULES` 不能有交集。"""
        assert not (PROCURE_MODULES & NON_PROCURE_MODULES)
