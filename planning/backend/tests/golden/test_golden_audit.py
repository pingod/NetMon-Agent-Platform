"""数据体检 golden：11 条规则的命中数必须与真实数据一致。

## 这批数字是产品最硬的 demo 素材

用户在 Excel 里看不出这些问题，但系统一导入就报出来 —— 这是「顺手告诉你表里哪儿错了」
的全部内容。因此每条规则的命中数都必须被锁住，不能让实现漂移。

| 规则 | 期望命中 | 说明 |
|---|---|---|
| D2 跨机柜 N 不符 | 14 | N 可比对 641 行 |
| D2b 间隔机柜 O 不符 | 52 | O 可比对 636 行（A 34 / B 2 / C 16） |
| D3 U 位区间重叠 | 11 | **归一化口径**；原样口径是 13 对（差 2 对是名称空格变体） |
| D4 同一设备多 U 位 | 1 | 同型号占多 U 位是正常的，只有同一实例名才是缺陷 |
| D6 端口被多行占用 | 22 | 分线接入交换机的 MPO 一分三是主因 |
| D12 数值列出现文本 | 10 | O 列 4 + P 列 4 + T 列 2 |
| R30 模块有值无批次 | 36 | 只算真实模块型号；宽松口径（任一模板块非空）是 126 |
| R30b 有批次无模块 | 2 | 反方向 |
| F3 已算 T 未核定 U | 9 | **正常中间态，不是错误** |
| F4 公式缝隙多买一档 | 49 | 全部 S=6.44 |

## 最容易搞错的三个口径（都踩过）

1. **U 位冲突必须按设备实例判**：连线表一台 44 口交换机占 44 行，按行判会得 833 对噪音。
2. **R30 有宽严两口径**：126 是"任一模板块非空"，其中 90 行的值是 `一`（铜缆直连
   根本不需要买模块）。真需要采购但没标批次的是 36 行。
3. **F3 不是错误**：`T` 算出来了、`U` 还没核定，这是每个新建项目在人工核定之前的
   天然状态。报成错误会让用户以为系统坏了。
"""

from __future__ import annotations

import pytest

from idcbox.domain.validation.rules import (
    AuditContext,
    RULE_TITLES,
    check_geometry,
    check_instances,
    check_ladder_gap,
    check_module_phase,
    check_non_numeric,
    check_pending_approval,
    check_ports,
    run_audit,
)

pytestmark = pytest.mark.golden


@pytest.fixture(scope="module")
def ctx(rows, row_nos, col, geo):
    return AuditContext(
        rows=rows,
        row_nos=row_nos,
        col=col,
        sheet_name="布线表",
        geometry=geo,
        project_label="V13",
    )


@pytest.fixture(scope="module")
def report(ctx):
    return run_audit(ctx)


class TestGeometryRules:
    def test_d2_n_deviation(self, ctx):
        devs, stats = check_geometry(ctx)
        n_devs = [d for d in devs if d.kind == "N"]
        assert stats["n_comparable"] == 641
        assert stats["n_bad"] == 14
        assert len(n_devs) == 14
        # N 的偏差行号必须与长度基线一致（同一批源数据缺陷）
        from baseline_io import load_baseline

        base_rows = {m["row"] for m in load_baseline("v13_baseline")["n"]["mismatches"]}
        assert {d.row_no for d in n_devs} == base_rows

    def test_d2b_o_deviation(self, ctx):
        devs, stats = check_geometry(ctx)
        o_devs = [d for d in devs if d.kind == "O"]
        assert stats["o_comparable"] == 636
        assert stats["o_bad"] == 52
        assert len(o_devs) == 52
        # 分空间类：A 34 / B 2 / C 16
        assert stats["o_class_A"] == 496
        assert stats["o_bad_A"] == 34
        assert stats["o_class_B"] == 100
        assert stats["o_bad_B"] == 2
        assert stats["o_class_C"] == 40
        assert stats["o_bad_C"] == 16

    def test_no_geometry_missing(self, ctx):
        _, stats = check_geometry(ctx)
        assert stats["geom_missing"] == 0, "362 个机柜位应能定位全部 5127 行"


class TestInstanceRules:
    def test_d3_two_scopes(self, ctx):
        """D3 的两个口径必须同时被记住：11 对真冲突 vs 13 对（含 2 对名称变体）。

        注意 `count_raw` / `count_normalized` 是**实例数**（836 / 834），
        不是冲突对数 —— 冲突来自 `find_u_conflicts` 的独立计算。
        """
        from idcbox.domain.validation.instances import find_u_conflicts

        inv, findings = check_instances(ctx)
        d3 = [f for f in findings if f.rule_code == "D3"]
        assert len(d3) == 11, f"D3 归一化口径应为 11 对，实际 {len(d3)}"

        raw_pairs = find_u_conflicts(inv.raw)
        norm_pairs = find_u_conflicts(inv.normalized)
        assert len(raw_pairs) == 13, f"原样口径 13 对，实际 {len(raw_pairs)}"
        assert len(norm_pairs) == 11
        variant_pairs = [p for p in raw_pairs if p.is_name_variant]
        assert len(variant_pairs) == 2, "原样比归一化多的 2 对全是名称空格变体"

        assert inv.count_normalized == 834, "归一化实例数"
        assert inv.count_raw == 836, "原样实例数（多 2 个来自带空格的型号名）"

    def test_d4_same_name_multi_u(self, ctx):
        _, findings = check_instances(ctx)
        d4 = [f for f in findings if f.rule_code == "D4"]
        assert len(d4) == 1, f"D4 应只有 1 处，实际 {len(d4)}：{[f.detail for f in d4]}"
        assert "计算带外交换机-2" in d4[0].detail

    def test_instance_counts(self, ctx):
        inv, _ = check_instances(ctx)
        assert inv.count_normalized == 834, "归一化实例数"
        assert inv.count_raw == 836, "原样实例数（多 2 个来自带空格的型号名）"


class TestPortRules:
    def test_d6_port_multi_use(self, ctx):
        findings = check_ports(ctx)
        assert len(findings) == 22, f"D6 应为 22 个端口，实际 {len(findings)}"

    def test_d6_includes_mp_star(self, ctx):
        """分线接入交换机的 25GE1/0/48 被 3 行占用（MPO 一分三）——最典型的假冲突。"""
        findings = check_ports(ctx)
        mpo = [f for f in findings if "25GE1/0/48" in f.detail]
        assert mpo, "应能报出分线接入交换机的 25GE1/0/48 冲突"


class TestDataRules:
    def test_d12_non_numeric(self, ctx):
        findings = check_non_numeric(ctx)
        assert len(findings) == 10, f"D12 应为 10 条，实际 {len(findings)}"
        texts = " ".join(f.detail for f in findings)
        assert "待定" in texts
        assert "设备自带" in texts
        assert "25G-SR" in texts

    def test_r30_module_without_batch(self, ctx):
        findings, stats = check_module_phase(ctx)
        r30 = [f for f in findings if f.rule_code == "R30"]
        assert len(r30) == 36, f"R30 真实口径应为 36，实际 {len(r30)}"
        assert stats.real_module_no_batch == 36
        # 宽松口径（任一模板块非空）必须被单独记录，不能与 36 混用
        assert stats.sentinel_only_no_batch == 90, (
            f"sentinel-only 应为 90（126-36），实际 {stats.sentinel_only_no_batch}"
        )
        assert stats.real_module_no_batch + stats.sentinel_only_no_batch == 126

    def test_r30b_reverse(self, ctx):
        findings, stats = check_module_phase(ctx)
        r30b = [f for f in findings if f.rule_code == "R30b"]
        assert len(r30b) == 2
        assert stats.batch_no_module == 2


class TestTransitionRules:
    def test_f3_pending_approval_is_info(self, ctx):
        """F3 必须是 info 而不是 warn/blocker —— 这是正常中间态。"""
        findings = check_pending_approval(ctx)
        assert len(findings) == 9, f"F3 应为 9 行，实际 {len(findings)}"
        assert all(f.severity == "info" for f in findings), (
            "F3「已算 T 未核定 U」是正常状态，不能报成警告或阻断"
        )
        rows_expected = {707, 708, 709, 715, 716, 717, 723, 724, 725}
        assert {f.row_no for f in findings} == rows_expected

    def test_f4_ladder_gap(self, ctx):
        findings = check_ladder_gap(ctx)
        assert len(findings) == 49, f"F4 应为 49 行，实际 {len(findings)}"
        # 全部 S=6.44
        assert all("6.44" in f.detail for f in findings), "F4 全部应为 S=6.44"


class TestHealthReport:
    def test_score_in_range(self, report):
        assert 90.0 <= report.score <= 99.9, f"健康度 {report.score} 超出预期区间"
        assert abs(report.score - 94.2) < 0.2, f"健康度应为 94.2，实际 {report.score}"

    def test_all_rules_have_titles(self, report):
        for s in report.summaries:
            assert s.rule_code in RULE_TITLES, f"规则 {s.rule_code} 缺标题"
            assert s.title == RULE_TITLES[s.rule_code]

    def test_findings_locatable(self, report):
        """FR-D10：每条 finding 必须能定位到 sheet + 行号。"""
        for f in report.findings:
            assert f.sheet, f"{f.rule_code} 缺 sheet"
            assert f.row_no is not None or f.group is not None, (
                f"{f.rule_code} 既不指行也不指分组，无法定位"
            )

    def test_no_blockers(self, report):
        """V13 没有阻断级缺陷 —— 有的话导入都会被拦下来。"""
        assert report.blocker_count == 0, (
            f"出现阻断级缺陷：{[f.detail for f in report.findings if f.severity == 'blocker'][:5]}"
        )

    def test_markdown_renders(self, report):
        from idcbox.domain.validation.report import render_markdown, render_summary_text

        md = render_markdown(report, detail_limit=3)
        assert "健康度" in md
        assert "D2b" in md
        # 渲染不能崩
        txt = render_summary_text(report)
        assert "健康度" in txt
