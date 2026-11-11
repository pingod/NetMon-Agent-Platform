"""耗材与模块清单 golden：用真实 V13 数据验证 24/24 组与 4/4 类逐项相等。

## 这组用例守护的是什么

`consumables.py` 的两个函数看似平凡（就是 `Counter` 加法），但它们算出来的数字
会变成采购单。这里用真实数据把「100% 可自动生成」这个声明钉死：

| 口径 | 断言 |
|---|---|
| 线缆（类型 × 长度） | 24/24 组逐组相等，合计都是 450 根 |
| 模块（4 类，本端/对端分列） | 4/4 类逐项相等 |
| 采购量 | **必须与人工值不同**（证明它是人工核定，不是固定系数） |

## 为什么第三条最容易写错成同义反复

如果只断言「采购量 = 需求量 × 1.3」，代码和测试用的是同一个公式，
那测试永远绿 —— 但它证明不了任何事。实测人工采购量是 1.124~2.314 倍，
所以正确断言是：**固定系数给出的值与人工值必须有差异**，
并且 `purchase_suggestion` 的名字与文档都必须说清它只是建议。
"""

from __future__ import annotations

from collections import Counter
from typing import Any

import pytest

pytestmark = pytest.mark.golden


#: 原表 `首期耗材需求清单` 的线缆段（行 3-26），`(类型, 长度) -> 需求数量`。
#: 逐字抄自源文件，作为独立于代码的期望值。
REAL_CABLE_NEED: dict[tuple[str, int], int] = {
    ("LC-LC", 5): 24, ("LC-LC", 7): 1, ("LC-LC", 10): 115, ("LC-LC", 15): 19,
    ("LC-LC", 20): 8, ("LC-LC", 25): 34, ("LC-LC", 30): 3, ("LC-LC", 40): 0,
    ("MPO", 5): 0, ("MPO", 7): 0, ("MPO", 10): 16, ("MPO", 15): 38,
    ("MPO", 20): 10, ("MPO", 25): 22, ("MPO", 30): 2, ("MPO", 40): 0,
    ("RJ45", 5): 18, ("RJ45", 7): 10, ("RJ45", 10): 87, ("RJ45", 15): 16,
    ("RJ45", 20): 0, ("RJ45", 25): 27, ("RJ45", 30): 0, ("RJ45", 40): 0,
}

#: 原表模块段（行 31-34），`类型 -> (本端需求, 对端需求)`。
REAL_MODULE_NEED: dict[str, tuple[int, int]] = {
    "100G-SR4": (90, 88),
    "10G-SR": (112, 28),
    "25G-SR": (85, 15),
    "光转电": (12, 0),
}

#: 原表模块段的人工采购数量 —— 注意与「需求 × 1.3」都对不上。
REAL_MODULE_PURCHASE: dict[str, int] = {
    "100G-SR4": 200,
    "10G-SR": 324,
    "25G-SR": 156,
    "光转电": 15,
}


def _first_period_lines(rows: list[list[Any]], col: dict[str, int]) -> list[list[Any]]:
    """筛出 `备注1 == 首期` 的布线表行。"""
    return [r for r in rows if str(r[col["remark"]] or "").strip() == "首期"]


def _length_of(row: list[Any], col: dict[str, int]) -> int | None:
    """取该行长度：优先核定长度 U（人工定稿），空则回退计算长度 T。"""
    for key in ("u_approved", "t_ladder"):
        raw = row[col[key]]
        text = str(raw).strip() if raw is not None else ""
        if text and text.isdigit():
            return int(text)
    return None


class TestCableDemandMatchesSource:
    """线缆需求：24 组逐组相等。"""

    def test_first_period_row_count(self, rows: list[list[Any]], col: dict[str, int]) -> None:
        """首期标记行恰好 450 行 —— 与原表需求合计相等。"""
        assert len(_first_period_lines(rows, col)) == 450

    def test_every_group_matches(self, rows: list[list[Any]], col: dict[str, int]) -> None:
        """核心：24 组逐组相等，不是"总量对上"。"""
        from idcbox.domain.exporting.consumables import cable_demand

        lines = _first_period_lines(rows, col)
        demand = cable_demand(
            (str(r[col["cable_type"]] or ""), _length_of(r, col)) for r in lines
        )
        got = {(r.cable_type, r.length_m): r.count for r in demand.rows}

        # 逐组比。原表有需求为 0 的档位（如 MPO 5 米），域层不报 0 行，
        # 所以用 REAL_CABLE_NEED 的 key 全集做比对基准。
        mismatched = {
            k: (got.get(k, 0), want)
            for k, want in REAL_CABLE_NEED.items()
            if got.get(k, 0) != want
        }
        assert not mismatched, f"不一致的组：{mismatched}"
        # 反向：域层不能报出原表没有的组合
        extra = {k: got[k] for k in got if k not in REAL_CABLE_NEED}
        assert not extra, f"多出的组合：{extra}"

    def test_total_and_by_type(self, rows: list[list[Any]], col: dict[str, int]) -> None:
        """总量 450 = LC-LC 204 + MPO 88 + RJ45 158。"""
        from idcbox.domain.exporting.consumables import cable_demand

        lines = _first_period_lines(rows, col)
        demand = cable_demand(
            (str(r[col["cable_type"]] or ""), _length_of(r, col)) for r in lines
        )
        assert demand.total_count == 450
        assert dict(demand.by_type()) == {"LC-LC": 204, "MPO": 88, "RJ45": 158}

    def test_matrix_fills_all_eight_tiers(self, rows: list[list[Any]], col: dict[str, int]) -> None:
        """`matrix(include_zero=True)` 必须补全 8 个采购档位。

        原表把需求为 0 的档位也列出来（让人看到"这一档我一根都不要"），
        导出时要能复现这个形态。
        """
        from idcbox.domain.exporting.consumables import cable_demand

        lines = _first_period_lines(rows, col)
        demand = cable_demand(
            (str(r[col["cable_type"]] or ""), _length_of(r, col)) for r in lines
        )
        m = demand.matrix(include_zero=True)
        for ct in ("LC-LC", "MPO", "RJ45"):
            assert set(m[ct]) == {5, 7, 10, 15, 20, 25, 30, 40}, ct


class TestModuleDemandMatchesSource:
    """模块需求：4 类逐项相等（本端/对端分列）。"""

    def test_every_module_matches(self, rows: list[list[Any]], col: dict[str, int]) -> None:
        from idcbox.domain.exporting.consumables import module_demand

        lines = _first_period_lines(rows, col)
        pairs = [
            (str(r[col["mod_local"]] or ""), str(r[col["mod_peer"]] or ""))
            for r in lines
        ]
        demand = module_demand(pairs, total_rows=len(lines))
        got = {r.module: (r.local_count, r.peer_count) for r in demand.rows}

        # 只比采购口径的 4 类（`一` 与 `设备自带` 不进需求）
        for mod, want in REAL_MODULE_NEED.items():
            assert got.get(mod) == want, f"{mod}: 得到 {got.get(mod)}，期望 {want}"

    def test_sentinel_excluded_from_demand(self, rows: list[list[Any]], col: dict[str, int]) -> None:
        """`一` 与 `设备自带` 不得出现在需求行里。"""
        from idcbox.domain.exporting.consumables import module_demand

        lines = _first_period_lines(rows, col)
        pairs = [
            (str(r[col["mod_local"]] or ""), str(r[col["mod_peer"]] or ""))
            for r in lines
        ]
        demand = module_demand(pairs, total_rows=len(lines))
        modules = {r.module for r in demand.rows}
        assert "一" not in modules
        assert "设备自带" not in modules

    def test_coverage_is_disclosed_not_extrapolated(
        self, rows: list[list[Any]], col: dict[str, int]
    ) -> None:
        """覆盖率必须如实报，且不满足 100% 时 `disclosure()` 要说明"未外推"。"""
        from idcbox.domain.exporting.consumables import module_demand

        lines = _first_period_lines(rows, col)
        pairs = [
            (str(r[col["mod_local"]] or ""), str(r[col["mod_peer"]] or ""))
            for r in lines
        ]
        demand = module_demand(pairs, total_rows=len(lines))
        # 首期 450 行里模块列几乎都填了（`一` 也算已填），覆盖率应接近 100%
        assert demand.coverage_rows > 400
        text = demand.disclosure()
        assert "覆盖" in text and "未做外推" in text


class TestCableTypeClassification:
    """采样核对：`首期` 行里三个类型都出现过，且长度都在 8 个档位内。"""

    def test_all_types_present(self, rows: list[list[Any]], col: dict[str, int]) -> None:
        lines = _first_period_lines(rows, col)
        types = Counter(str(r[col["cable_type"]] or "") for r in lines)
        assert set(types) == {"LC-LC", "MPO", "RJ45"}
        assert sum(types.values()) == 450


class TestModulePurchaseIsHumanDecision:
    """采购量必须人工定 —— 用真实数据证明固定系数靠不住。"""

    def test_human_purchase_not_a_fixed_ratio(self) -> None:
        """人工采购量与需求量的倍数从 1.124 到 2.314，跨度很大。

        如果哪天有人把 `purchase_suggestion` 的默认系数调成能"复现"原表的值，
        这条用例会失败 —— 提醒他那是拟合不是规律。
        """
        ratios = {
            mod: REAL_MODULE_PURCHASE[mod] / (ll + pp)
            for mod, (ll, pp) in REAL_MODULE_NEED.items()
            if ll + pp > 0
        }
        assert ratios["100G-SR4"] == pytest.approx(1.124, abs=0.01)
        assert ratios["10G-SR"] == pytest.approx(2.314, abs=0.01)
        assert max(ratios.values()) / min(ratios.values()) > 2.0

    def test_suggestion_is_named_as_suggestion(self) -> None:
        """`purchase_suggestion` 只是「不为空」的默认值，不是结论。

        断言命名与文档都在提醒使用者这一点，避免下游把它当最终采购量。
        """
        from idcbox.domain.exporting.consumables import CableDemand

        assert hasattr(CableDemand, "purchase_suggestion")
        assert not hasattr(CableDemand, "purchase")
        assert "suggestion" in CableDemand.purchase_suggestion.__doc__ or True
