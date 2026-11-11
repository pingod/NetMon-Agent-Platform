"""四策略（LengthPolicy）的语义铁律。

## 这个模块守护的东西

`policy.py` 是整个「存量迁移」方案的落地机制。它存在的唯一原因是一个事实：

**V13 的 5113 条有效连线里，4477 行（87.6%）从来没走过公式链**，
N/O 全空、U 是人工凭经验直填的。如果系统只有「算 + 微调」两层，
导入存量项目时这 88% 的行会被全部标红「计算值与人工值不符」—— 产品当天不可用。

## 四条不可协商（每条都有对应用例）

1. `calc_len_t` 与 `suggest_len_m` **任何策略下都恒算** —— 否则没有对比基准
2. `IMPORTED` 行 `approved_len_u` **恒等于原值**，重算不覆盖
3. `IMPORTED` 与 `MANUAL` **不参与长度偏差告警**；`LOW` 置信默认不进自动汇总
4. 策略切换是行级操作（由 application 层写 audit_log，域层只保证语义）
"""

from __future__ import annotations

import pytest

from idcbox.domain.cable_length.empirical import EmpiricalTableData, EmpiricalCell, empty_table
from idcbox.domain.cable_length.link import Endpoint, LinkInput
from idcbox.domain.cable_length.policy import (
    POLICY_IN_DEVIATION_CHECK,
    LengthPolicy,
    compute_all_policies,
    infer_policy,
    resolve_batch,
    resolve_length,
)
from idcbox.domain.cable_length.rules import DEFAULT_RULE
from idcbox.domain.geometry.layout import RackRef


def link(a=(1, 1), b=(1, 2), cable="LC-LC", key="k", **kw):
    return LinkInput(
        stable_key=key,
        a=Endpoint(rack=RackRef(*a), device_name="devA", port_name="25GE1/0/1"),
        b=Endpoint(rack=RackRef(*b), device_name="devB", port_name="25G1"),
        cable_type=cable,
        **kw,
    )


def table_with(cls, cab, path, length, n=100, purity=0.95):
    """构造只含一条规则的经验表。"""
    return EmpiricalTableData(
        formula_version="test",
        l1={(cls, cab, path): EmpiricalCell(length, n, purity)},
        l2={},
        l3={},
    )


class TestPolicyEnum:
    def test_four_values(self):
        assert {p.value for p in LengthPolicy} == {
            "GEOMETRIC",
            "EMPIRICAL",
            "MANUAL",
            "IMPORTED",
        }

    def test_deviation_participation(self):
        """哪两个策略参与偏差告警，是这张表的全部意义。"""
        assert POLICY_IN_DEVIATION_CHECK[LengthPolicy.GEOMETRIC] is True
        assert POLICY_IN_DEVIATION_CHECK[LengthPolicy.EMPIRICAL] is True
        assert POLICY_IN_DEVIATION_CHECK[LengthPolicy.MANUAL] is False
        assert POLICY_IN_DEVIATION_CHECK[LengthPolicy.IMPORTED] is False


class TestInferPolicy:
    def test_imported_when_u_present(self, geo):
        """有原值 → IMPORTED（存量迁移，不告警）。"""
        l = link(imported_len_u=15)
        r = resolve_length(l, DEFAULT_RULE, geo, empty_table("t"))
        assert r.policy is LengthPolicy.IMPORTED

    def test_imported_when_raw_text(self, geo):
        """有 sentinel 文本（如 `待定`）也算 IMPORTED —— 它同样是"人的决定"。"""
        l = link(imported_len_raw="待定")
        r = resolve_length(l, DEFAULT_RULE, geo, empty_table("t"))
        assert r.policy is LengthPolicy.IMPORTED

    def test_geometric_when_no_import(self, geo):
        """新建行（无原值）→ GEOMETRIC。"""
        l = link()
        r = resolve_length(l, DEFAULT_RULE, geo, empty_table("t"))
        assert r.policy is LengthPolicy.GEOMETRIC


class TestConstantComputation:
    """铁律 1：`calc_len_t` 与 `suggest_len_m` 任何策略下都恒算。"""

    @pytest.mark.parametrize(
        "policy",
        [
            LengthPolicy.GEOMETRIC,
            LengthPolicy.EMPIRICAL,
            LengthPolicy.MANUAL,
            LengthPolicy.IMPORTED,
        ],
    )
    def test_both_computed_for_every_policy(self, geo, policy):
        l = link(imported_len_u=15)
        t = table_with("A", "LC-LC", 1, 10)
        r = resolve_length(l, DEFAULT_RULE, geo, t, policy=policy)
        assert r.calc_len_t is not None, f"{policy} 下 calc_len_t 必须算出来"
        assert r.suggest_len_m is not None, f"{policy} 下 suggest_len_m 必须算出来"

    def test_imported_still_shows_what_system_thinks(self, geo):
        """IMPORTED 行也要显示「系统认为是 7 米」，用户才能对比原值 15 米。"""
        l = link(imported_len_u=15)
        r = resolve_length(l, DEFAULT_RULE, geo, empty_table("t"))
        assert r.policy is LengthPolicy.IMPORTED
        assert r.approved_len_u == 15, "原值必须保真"
        assert r.calc_len_t == 7, "系统算出的 T 仍要能看到 — 这是对比基准"


class TestImportedFidelity:
    """铁律 2：IMPORTED 行原值恒等，绝不覆盖。"""

    def test_value_preserved(self, geo):
        l = link(imported_len_u=25)
        r = resolve_length(l, DEFAULT_RULE, geo, empty_table("t"))
        assert r.approved_len_u == 25

    def test_not_in_deviation_check(self, geo):
        l = link(imported_len_u=25)
        r = resolve_length(l, DEFAULT_RULE, geo, empty_table("t"))
        assert r.participates_in_deviation is False

    def test_manual_also_not_in_deviation(self, geo):
        l = link(imported_len_u=25)
        r = resolve_length(
            l, DEFAULT_RULE, geo, empty_table("t"), policy=LengthPolicy.MANUAL
        )
        assert r.participates_in_deviation is False

    def test_geometric_does_participate(self, geo):
        r = resolve_length(link(), DEFAULT_RULE, geo, empty_table("t"))
        assert r.participates_in_deviation is True


class TestApprovedByPolicy:
    def test_geometric_uses_calc_t(self, geo):
        r = resolve_length(
            link(), DEFAULT_RULE, geo, empty_table("t"), policy=LengthPolicy.GEOMETRIC
        )
        assert r.approved_len_u == r.calc_len_t

    def test_empirical_uses_suggest(self, geo):
        """经验表给 10，几何 T 给 7 → EMPIRICAL 策略取 10。"""
        t = table_with("A", "LC-LC", 1, 10)
        r = resolve_length(link(), DEFAULT_RULE, geo, t, policy=LengthPolicy.EMPIRICAL)
        assert r.calc_len_t == 7, "几何算出 7"
        assert r.approved_len_u == 10, "经验表建议 10 被采纳"

    def test_empirical_falls_back_to_t(self, geo):
        """经验表无命中 → 退回几何 T，不是留空。"""
        r = resolve_length(
            link(), DEFAULT_RULE, geo, empty_table("t"), policy=LengthPolicy.EMPIRICAL
        )
        assert r.suggest_len_m is None
        assert r.approved_len_u == r.calc_len_t

    def test_imported_uses_raw(self, geo):
        r = resolve_length(link(imported_len_u=15), DEFAULT_RULE, geo, empty_table("t"))
        assert r.approved_len_u == 15

    def test_override_wins_over_everything(self, geo):
        """显式 override（人工核定动作）优先级最高。"""
        l = link(imported_len_u=15)
        r = resolve_length(l, DEFAULT_RULE, geo, empty_table("t"), override_u=30)
        assert r.approved_len_u == 30


class TestConfidence:
    def test_empirical_confidence_from_table(self, geo):
        t = table_with("A", "LC-LC", 1, 10, n=100, purity=0.95)
        r = resolve_length(link(), DEFAULT_RULE, geo, t, policy=LengthPolicy.EMPIRICAL)
        assert r.confidence == "HIGH"

    def test_imported_confidence_na(self, geo):
        """IMPORTED 的置信度是 NA —— 不是"低置信"，是"不适用"（人的决定不需要置信度）。"""
        r = resolve_length(link(imported_len_u=15), DEFAULT_RULE, geo, empty_table("t"))
        assert r.confidence == "NA"

    def test_geometric_confidence_from_class(self, geo):
        """A/B 类 HIGH、C 类 MEDIUM。"""
        ra = resolve_length(link((1, 1), (1, 2)), DEFAULT_RULE, geo, empty_table("t"))
        assert ra.o_class == "A" and ra.confidence == "HIGH"
        rb = resolve_length(link((1, 1), (1, 27)), DEFAULT_RULE, geo, empty_table("t"))
        assert rb.o_class == "B" and rb.confidence == "HIGH"
        rc = resolve_length(link((1, 1), (2, 1)), DEFAULT_RULE, geo, empty_table("t"))
        assert rc.o_class == "C" and rc.confidence == "MEDIUM"

    def test_low_purity_is_advisory(self, geo):
        """纯度 <0.85 → is_advisory=True，不进自动耗材汇总（铁律 3）。"""
        t = table_with("A", "LC-LC", 1, 10, n=100, purity=0.70)
        r = resolve_length(link(), DEFAULT_RULE, geo, t, policy=LengthPolicy.EMPIRICAL)
        assert r.is_advisory is True

    def test_advisory_only_relevant_to_empirical(self, geo):
        """几何策略下不标 advisory —— 几何公式没有"建议"语义。"""
        r = resolve_length(link(), DEFAULT_RULE, geo, empty_table("t"))
        assert r.is_advisory is False


class TestAllPolicies:
    def test_four_candidates(self, geo):
        """四路一次算完，供 UI 并排对比。"""
        cands, geo_result = compute_all_policies(
            link(), DEFAULT_RULE, geo, table_with("A", "LC-LC", 1, 10)
        )
        assert set(cands) == {
            LengthPolicy.GEOMETRIC,
            LengthPolicy.EMPIRICAL,
            LengthPolicy.MANUAL,
            LengthPolicy.IMPORTED,
        }
        assert geo_result.t_ladder_m is not None

    def test_geometric_candidate_matches_engine(self, geo):
        cands, geo_result = compute_all_policies(link(), DEFAULT_RULE, geo, empty_table("t"))
        assert cands[LengthPolicy.GEOMETRIC].length_m == geo_result.t_ladder_m

    def test_imported_candidate_reads_raw(self, geo):
        cands, _ = compute_all_policies(
            link(imported_len_u=25), DEFAULT_RULE, geo, empty_table("t")
        )
        assert cands[LengthPolicy.IMPORTED].length_m == 25

    def test_resolved_alternatives_has_all(self, geo):
        r = resolve_length(link(), DEFAULT_RULE, geo, empty_table("t"))
        assert len(r.alternatives) == 4


class TestResolveBatch:
    def test_per_row_policy(self, geo):
        """策略可按行覆盖 —— 同一张表里 GEOMETRIC 与 IMPORTED 混存。"""
        links = [
            link(key="new", b=(1, 2)),
            link(key="old", b=(1, 27), imported_len_u=25),
        ]
        out = resolve_batch(links, DEFAULT_RULE, geo, empty_table("t"))
        by_key = {r.stable_key: r for r in out}
        assert by_key["new"].policy is LengthPolicy.GEOMETRIC
        assert by_key["old"].policy is LengthPolicy.IMPORTED

    def test_explicit_policy_override(self, geo):
        links = [link(key="x")]
        out = resolve_batch(
            links,
            DEFAULT_RULE,
            geo,
            empty_table("t"),
            policies={"x": LengthPolicy.MANUAL},
        )
        assert out[0].policy is LengthPolicy.MANUAL

    def test_batch_preserves_order(self, geo):
        links = [link(key=f"k{i}", b=(1, 2)) for i in range(5)]
        out = resolve_batch(links, DEFAULT_RULE, geo, empty_table("t"))
        assert [r.stable_key for r in out] == [f"k{i}" for i in range(5)]


class TestManualCandidate:
    def test_manual_reads_import(self, geo):
        """MANUAL 策略取人工值；无人工值时是 None（等人工填，不是猜）。"""
        cands, _ = compute_all_policies(link(), DEFAULT_RULE, geo, empty_table("t"))
        assert cands[LengthPolicy.MANUAL].length_m is None

    def test_manual_with_import(self, geo):
        cands, _ = compute_all_policies(
            link(imported_len_u=20), DEFAULT_RULE, geo, empty_table("t")
        )
        assert cands[LengthPolicy.MANUAL].length_m == 20
