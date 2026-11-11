"""体检报告的渲染（纯文本 / Markdown），纯函数，无 IO。

## 为什么报告要分两个视图

- **总览视图**：给项目经理看。只要「多少行、几个问题、健康度几分」。
- **明细视图**：给执行人看。要能定位到行号、列名、错误值、建议动作。

同一份 `HealthReport` 渲染两种粒度，避免两套数据源不一致。
"""

from __future__ import annotations

from typing import Sequence

from .rules import Finding, HealthReport

SEVERITY_CN = {"blocker": "阻断", "warn": "警告", "info": "提示"}
SEVERITY_ICON = {"blocker": "🛑", "warn": "⚠️", "info": "ℹ️"}

#: 规则说明（报告里每条规则只解释一次，不重复在每行）
RULE_DOC: dict[str, str] = {
    "D2": "跨机柜 N：按平面图行号差重算 N=4×Δrow+4。偏离多为人手填错。",
    "D2b": "间隔机柜 O：A 类同排取列坐标差；B/C 类取排端走线架往返距离。",
    "D3": "U 位区间重叠：同一机柜内两台设备的 U 位区间相交，物理上装不进去。",
    "D4": "同一实例名占多个 U 位：一台设备只能有一个安装位置。",
    "D5": "命名规范：空格变体、全半角、实例名缺失。不影响计算，影响可读性与标签。",
    "D6": "端口被多行占用：一个物理端口只能接一根线。",
    "D7": "孤儿机柜：引用了平面图上不存在的机柜位置。",
    "D12": "数值列出现文本：sentinel（待定/设备自带）属正常；真正的型号写进数值列是填错。",
    "R30": "模块有值但未标批次：采购范围无法按期切分，耗材汇总会失真。",
    "F3": "长度已算未核定：新建项目的正常中间态，不是错误。",
    "F4": "公式缝隙：S 落在 (6,7) 开区间时，原公式比连续口径多给一档。",
}


def _fmt_ratio(a: int, b: int) -> str:
    if b <= 0:
        return "—"
    return f"{a}/{b} ({100.0 * a / b:.1f}%)"


def render_markdown(report: HealthReport, *, detail_limit: int = 20) -> str:
    """完整报告：概览 → 健康度 → 按规则分组明细。"""
    lines: list[str] = []
    a = lines.append

    a(f"# 数据体检报告：{report.project_label}")
    a("")
    a(f"- 数据行：**{report.total_rows}**")
    a(f"- 健康度：**{report.score} / 100**")
    a(
        f"- 问题分布：阻断 {report.blocker_count} · 警告 {report.warn_count} · "
        f"提示 {report.info_count}"
    )
    a("")

    # ---- 健康度分维度 ----
    a("## 一、健康度")
    a("")
    a("| 维度 | 得分 | 权重 | 说明 |")
    a("|---|---|---|---|")
    for name, score, weight in report.score_breakdown:
        a(f"| {name} | {score} | {weight:.0%} |{'见下文明细' if score < 100 else '无问题'} |")
    a("")

    # ---- 关键事实 ----
    f = report.facts
    a("## 二、关键事实")
    a("")
    if "n_comparable" in f:
        a(
            f"- 跨机柜(N) 可比对 {f['n_comparable']} 行，"
            f"与平面图不符 {f['n_bad']} 行"
            f"（一致率 {_fmt_ratio(int(f['n_comparable']) - int(f['n_bad']), int(f['n_comparable']))}）"
        )
    if "o_comparable" in f:
        a(
            f"- 间隔机柜(O) 可比对 {f['o_comparable']} 行，"
            f"与平面图不符 {f['o_bad']} 行"
            f"（一致率 {_fmt_ratio(int(f['o_comparable']) - int(f['o_bad']), int(f['o_comparable']))}）"
        )
        a(
            f"  - A 类（同排）{f['o_class_A']} 行，不符 {f['o_bad_A']} 行；"
            f"B 类（同通道跨排）{f['o_class_B']} 行，不符 {f['o_bad_B']} 行；"
            f"C 类（跨通道）{f['o_class_C']} 行，不符 {f['o_bad_C']} 行"
        )
    a(
        f"- U 位冲突：原样口径 **{f.get('u_conflicts_raw', 0)} 对**，"
        f"归一化口径 **{f.get('u_conflicts_normalized', 0)} 对**"
        f"（差值来自型号名空格变体，是同一台设备两种写法）"
    )
    a("")

    # ---- 按规则汇总 ----
    a("## 三、问题汇总")
    a("")
    a("| 规则 | 级别 | 问题 | 命中 | 涉及行 |")
    a("|---|---|---|---|---|")
    for s in report.summaries:
        a(
            f"| {s.rule_code} | {SEVERITY_ICON[s.severity]} {SEVERITY_CN[s.severity]} "
            f"| {s.title} | {s.count} | {s.affected_rows} |"
        )
    a("")

    # ---- 明细 ----
    a("## 四、明细（每类最多列 20 条）")
    a("")
    for s in report.summaries:
        items = report.by_rule(s.rule_code)
        a(f"### {s.rule_code} {s.title} — {s.count} 条")
        a("")
        if s.rule_code in RULE_DOC:
            a(f"> {RULE_DOC[s.rule_code]}")
            a("")
        for it in items[:detail_limit]:
            loc = f"第 {it.row_no} 行" if it.row_no else "全文"
            if it.column:
                loc += f" / {it.column}"
            a(f"- `{loc}` {it.detail}")
            if it.action:
                a(f"  - 建议：{it.action}")
        if len(items) > detail_limit:
            a(f"- …… 另有 {len(items) - detail_limit} 条同类问题（导出 Excel 查看全部）")
        a("")

    return "\n".join(lines)


def render_summary_text(report: HealthReport) -> str:
    """控制台一行式摘要（CLI 用）。"""
    rows = [
        f"体检：{report.project_label} · {report.total_rows} 行 · 健康度 {report.score}/100",
    ]
    for s in report.summaries:
        rows.append(f"  {SEVERITY_ICON[s.severity]} {s.rule_code:<4} {s.title:<24} {s.count:>5} 条")
    if not report.summaries:
        rows.append("  ✅ 未发现问题")
    return "\n".join(rows)


def render_findings_table(findings: Sequence[Finding], headers: tuple[str, ...] | None = None):
    """把 findings 转成「可直接写 Excel」的二维表（infra 层负责落盘）。"""
    hs = headers or ("规则", "级别", "问题", "Sheet", "行号", "列", "详情", "建议")
    rows: list[tuple[object, ...]] = [hs]
    for f in findings:
        rows.append(
            (
                f.rule_code,
                SEVERITY_CN[f.severity],
                f.title,
                f.sheet,
                f.row_no,
                f.column,
                f.detail,
                f.action,
            )
        )
    return rows


__all__ = [
    "RULE_DOC",
    "SEVERITY_CN",
    "SEVERITY_ICON",
    "render_findings_table",
    "render_markdown",
    "render_summary_text",
]
