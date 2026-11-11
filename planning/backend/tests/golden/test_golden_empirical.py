"""经验表 golden：`rules/len_empirical_v1.json` 必须能被真实数据复现。

## 三个断言层次（从"表对不对"到"能承诺什么"）

1. **表本身对不对**：JSON 里 123 条 L1 规则的 `(len, n)` 必须与从 V13 重新统计出来的
   众数/计数**逐条相等**。这防的是"规则文件被手工改过但没人发现"。
2. **LOO 87.11%**：只用 L1 单层、留一法（把当前样本从统计里去掉再预测）。
   这是文档对外的"历史上 87% 的行能猜对"的唯一严谨口径。
3. **可承诺口径 84.31% / 94.00%**：三级回退 + 门控 + C类/MPO 强制人工后的真实产品质量。
   `85.2%` 是含样本自身的回测数字，**不得对外**。

## 为什么 LOO 要在测试里重算而不是直接读 JSON 的 measured 段

`measured` 段是生成脚本写进去的，读它等于自己证明自己。重算一遍才知道脚本逻辑没漂。
"""

from __future__ import annotations

from collections import Counter, defaultdict

import pytest

from baseline_io import load_baseline
from idcbox.domain.cable_length.empirical import (
    ADVISORY_PURITY,
    ADVISORY_SAMPLE_N,
    gate_confidence,
)
from idcbox.domain.cable_length.formulas import classify_o, path_units
from idcbox.domain.cable_length.rules import DEFAULT_RULE
from idcbox.domain.geometry.layout import RackRef
from idcbox.domain.importing.normalize import norm_cable_type
from idcbox.infrastructure.config.empirical_loader import (
    load_empirical_table,
    table_measured,
)

pytestmark = pytest.mark.golden

ROOT = __import__("pathlib").Path(__file__).resolve().parent.parent.parent.parent
RULE_FILE = ROOT / "rules" / "len_empirical_v1.json"


def _build_records(geo, rows, col) -> list[dict]:
    """从 V13 重建经验表的样本集：只取"有核定长度 U 且有几何"的行。

    必须复刻生成脚本的两个过滤条件，否则样本集会多 14 行：
    - 两端型号列**都**为空的行跳过（本端设备为空但有其他列的 14 行，非连线行）
    - U 必须是数值（排除 9 行待核定 + `待定` sentinel）
    """
    from idcbox.domain.cable_length.numeric import to_number

    recs: list[dict] = []
    for v in rows:
        if not v[col["model_local"]] and not v[col["model_peer"]]:
            continue
        act = to_number(v[col["u_approved"]])
        if act is None:
            continue  # U 空（9 行待核定）或 sentinel 文本
        a = geo.pos(RackRef(channel_no=v[col["ch_local"]], seq_no=v[col["rack_local"]]))
        b = geo.pos(RackRef(channel_no=v[col["ch_peer"]], seq_no=v[col["rack_peer"]]))
        if a is None or b is None:
            continue
        cab = norm_cable_type(v[col["cable_type"]])
        recs.append(
            {
                "cls": classify_o(a, b),
                "cab": cab,
                "path": path_units(a, b, DEFAULT_RULE),
                "actual": int(act),
            }
        )
    return recs


def _key(d) -> tuple[str, str, int]:
    return (d["cls"], d["cab"], d["path"])


class TestRuleFileFaithful:
    """层次 1：规则文件必须与数据一致。"""

    def test_entry_counts(self, emp_table):
        assert len(emp_table.l1) == 123, f"L1 规则条数变了：{len(emp_table.l1)}"
        assert len(emp_table.l2) == 8
        assert len(emp_table.l3) == 3
        assert emp_table.gate == (10, 0.75)
        assert "C" in emp_table.force_manual_classes
        assert "MPO" in emp_table.force_manual_cable_types

    def test_l1_matches_data(self, geo, rows, col, emp_table):
        """JSON 里每条 L1 规则的 (len, n) 必须等于数据里的众数/计数。"""
        counts: dict[tuple[str, str, int], Counter[int]] = defaultdict(Counter)
        for d in _build_records(geo, rows, col):
            counts[_key(d)][d["actual"]] += 1

        assert set(counts) == set(emp_table.l1), (
            f"L1 键集合不一致。数据多出 {set(counts) - set(emp_table.l1)}，"
            f"规则多出 {set(emp_table.l1) - set(counts)}"
        )
        for k, cnt in counts.items():
            cell = emp_table.l1[k]
            top_len, top_n = cnt.most_common(1)[0]
            assert cell.sample_n == sum(cnt.values()), (
                f"{k} 样本数不符：JSON {cell.sample_n} != 数据 {sum(cnt.values())}"
            )
            assert cell.length_m == top_len, f"{k} 众数不符：{cell.length_m} != {top_len}"
            # JSON 里纯度只存 3 位小数，比对必须放宽到 1e-3
            assert abs(cell.purity - top_n / sum(cnt.values())) < 1e-3, (
                f"{k} 纯度不符：{cell.purity} != {top_n / sum(cnt.values()):.4f}"
            )

    def test_measured_section_disclosed(self):
        measured = table_measured(RULE_FILE)
        assert measured["coverage_pct"] == 85.2, (
            "measured 段的 85.2% 是回测口径，保留但需与 84.31% 区分"
        )
        assert measured["loo_cls_cab_path"] == 87.11


class TestLoo:
    """层次 2、3：LOO 与门控口径。"""

    def test_loo_l1_only(self, geo, rows, col):
        """只用 L1 单层做 LOO —— 文档 87.11% 的精确来源。"""
        base = load_baseline("empirical_baseline")
        recs = _build_records(geo, rows, col)
        assert len(recs) == base["samples"], f"样本数变了：{len(recs)}"

        counts: dict[tuple[str, str, int], Counter[int]] = defaultdict(Counter)
        for d in recs:
            counts[_key(d)][d["actual"]] += 1

        ok = 0
        for d in recs:
            c = Counter(counts[_key(d)])
            c[d["actual"]] -= 1  # 留一：把自己剔除
            cand = [(n, v) for v, n in c.items() if n > 0]
            pred = max(cand)[1] if cand else None
            if pred == d["actual"]:
                ok += 1

        rate = ok / len(recs)
        assert abs(rate - base["loo_cls_cab_path"]) < 1e-6, (
            f"L1 LOO 变了：{rate:.6f}（基线 {base['loo_cls_cab_path']}）"
        )
        assert rate >= 0.87, f"L1 LOO 跌破 87%：{rate:.2%}"

    def test_gated_loo(self, geo, rows, col):
        """三级回退 + 门控 + 强制人工 —— 产品实际能承诺的口径。"""
        base = load_baseline("empirical_baseline")["loo_gated"]
        recs = _build_records(geo, rows, col)

        l1: dict[tuple[str, str, int], Counter[int]] = defaultdict(Counter)
        l2: dict[tuple[str, str], Counter[int]] = defaultdict(Counter)
        l3: dict[str, Counter[int]] = defaultdict(Counter)
        for d in recs:
            l1[_key(d)][d["actual"]] += 1
            l2[(d["cls"], d["cab"])][d["actual"]] += 1
            l3[d["cab"]][d["actual"]] += 1

        gate_n, gate_p = 10, 0.75
        covered = correct = forced = 0
        for d in recs:
            if d["cls"] in ("C",) or d["cab"] in ("MPO",):
                forced += 1
                continue
            pred = None
            for counter in (l1[_key(d)], l2[(d["cls"], d["cab"])], l3[d["cab"]]):
                c = Counter(counter)
                c[d["actual"]] -= 1
                total = sum(n for n in c.values() if n > 0)
                if total <= 0:
                    continue
                top_n, top_v = max((n, v) for v, n in c.items() if n > 0)
                if total >= gate_n and top_n / total >= gate_p:
                    pred = top_v
                    break
            if pred is None:
                forced += 1
                continue
            covered += 1
            if pred == d["actual"]:
                correct += 1

        assert covered == base["covered"], f"覆盖行数变了：{covered} != {base['covered']}"
        assert forced == base["forced_manual"], (
            f"强制人工行数变了：{forced} != {base['forced_manual']}"
        )
        assert abs(correct / covered - base["accuracy_within_covered"]) < 1e-3, (
            f"覆盖内准确率变了：{correct / covered:.6f}"
        )
        # 对外承诺线
        assert covered / len(recs) >= 0.84, "覆盖率跌破 84%"
        assert correct / covered >= 0.94, "覆盖内准确率跌破 94%"

    def test_force_manual_never_recommended(self, emp_table, geo, rows, col):
        """C 类与 MPO 必须 100% 命中强制人工分支，一条建议都不能给。"""
        recs = _build_records(geo, rows, col)
        for d in recs:
            hit = emp_table.lookup(d["cls"], d["cab"], d["path"])
            if d["cls"] == "C" or d["cab"] == "MPO":
                assert hit.is_forced_manual, (
                    f"C 类或 MPO 竟给出了建议：{d['cls']}/{d['cab']}/path={d['path']}"
                )
                assert hit.cell is None


class TestGateConfidence:
    """置信度门控：低纯度必须标 advisory（不进自动采购汇总）。

    注意两层门控的区别（容易混淆）：
    - **表门控** `purity >= 0.75`：不达标的单元格**根本不会给出建议**（lookup 返回 NONE）
    - **advisory 门控** `purity >= 0.85 且 n >= 10`：给了建议但质量一般 → 标 advisory，
      可以展示但**不进自动耗材汇总**
    所以 `A/RJ45/path=0`（纯度 0.503）表现是"不给建议"，不是"给建议但 advisory"。
    """

    def test_low_purity_blocked_by_gate(self, emp_table):
        """纯度 0.503 < 0.75，三级回退全部不达标 → 必须不给建议。"""
        hit = emp_table.lookup("A", "RJ45", 0)
        assert hit.cell is None, "低纯度单元格不应给出建议"
        assert hit.level == "NONE"
        level, advisory = gate_confidence(hit.cell, hit.level)
        assert level == "NA"
        assert advisory is True

    def test_borderline_purity_is_advisory(self, emp_table):
        """纯度 0.778 过了表门控（≥0.75）但低于 advisory 线（0.85）→ 给建议但标 advisory。"""
        hit = emp_table.lookup("A", "RJ45", 4)
        assert hit.cell is not None, "0.778 应当过了表门控"
        level, advisory = gate_confidence(hit.cell, hit.level)
        assert advisory is True, "0.778 低于 0.85，必须标 advisory"
        assert level == "LOW"

    def test_high_purity_not_advisory(self, emp_table):
        """纯度 0.996 → HIGH 且非 advisory。"""
        hit = emp_table.lookup("A", "RJ45", 1)
        assert hit.cell is not None
        level, advisory = gate_confidence(hit.cell, hit.level)
        assert advisory is False
        assert level == "HIGH"

    def test_advisory_thresholds(self):
        assert ADVISORY_PURITY == 0.85
        assert ADVISORY_SAMPLE_N == 10
