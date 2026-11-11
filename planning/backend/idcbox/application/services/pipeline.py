"""端到端流水线编排（应用层）。

## 为什么需要这一层

CLI、将来的 HTTP API、测试脚本，三者要做的是**同一件事**：

```
读源文件 → 解析机房几何 → 适配连线表 → 计算长度 → 体检 → 导出
```

如果每个入口各写一遍，就会出现「CLI 的体检规则和 API 的体检规则不一致」这类事故。
所以把流程编排收在这里，入口层只负责参数解析与输出渲染。

## 分三段而不是一个大函数

`load_project` → `compute_project` → `check_project`。

因为**增量场景要能只走其中一段**：改一个机柜位置只需重算受影响连线
（`compute_incremental`），不必重读 Excel、不必重跑全量体检。

## 两条数据通道并存

- `links`：域层计算用（结构化、已归一、带 stable_key）
- `audit_rows`/`audit_col`：体检用（**保留原始列值**，因为体检要报出"源值 vs 计算值"）

这两条通道不能合并：体检必须看到 `待定`、`25G-SR` 这种脏值原文，
而域层计算必须把它们归一成 None。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Mapping, Sequence

from ...domain.cable_length.empirical import EmpiricalTable, empty_table
from ...domain.cable_length.engine import BatchResult, compute_batch
from ...domain.cable_length.link import LinkInput
from ...domain.cable_length.policy import LengthPolicy, ResolvedLength, resolve_batch
from ...domain.cable_length.rules import DEFAULT_RULE, LengthRule
from ...domain.geometry.floor import FloorCell, derive_cmax_by_room, parse_floor
from ...domain.geometry.layout import GeometrySnapshot, build_snapshot
from ...domain.validation.rules import AuditContext, HealthReport, run_audit
from ..ports import CablingSource, FloorSource


@dataclass(slots=True)
class LoadedProject:
    """读入并适配完成的项目数据。"""

    label: str
    source_path: Path
    geometry: GeometrySnapshot
    rack_count: int
    cmax_by_room: Mapping[str, int]
    links: Sequence[LinkInput]
    raw_values: Mapping[str, Mapping[str, object]]
    total_rows: int
    empty_rows: int
    #: 体检用的原始行值（列式），与 `links` 是同一批行的两种视图
    audit_rows: Sequence[Sequence[object]]
    audit_row_nos: Sequence[int]
    audit_col: Mapping[str, int]
    skipped: Sequence[tuple[int, str]] = ()
    rule: LengthRule = DEFAULT_RULE

    @property
    def effective_rows(self) -> int:
        return len(self.links)

    def summary(self) -> str:
        skipped = f"，跳过 {len(self.skipped)} 行" if self.skipped else ""
        return (
            f"读取 {self.source_path.name}：{self.total_rows} 行"
            f"（有效 {self.effective_rows}，空行 {self.empty_rows}{skipped}）；"
            f"几何 {self.rack_count} 个机柜位，CMAX={dict(self.cmax_by_room)}"
        )

    def audit_context(self) -> AuditContext:
        return AuditContext(
            rows=self.audit_rows,
            row_nos=self.audit_row_nos,
            col=dict(self.audit_col),
            sheet_name="布线表",
            geometry=self.geometry,
            project_label=self.label,
        )


@dataclass(slots=True)
class ComputedProject:
    """加上长度计算结果。"""

    project: LoadedProject
    batch: BatchResult
    resolved: Sequence[ResolvedLength]

    def by_key(self) -> dict[str, ResolvedLength]:
        return {r.stable_key: r for r in self.resolved}

    def summary(self) -> str:
        s = self.batch.stats
        parts = [f"计算 {s.computed_rows}/{s.total_rows} 行"]
        if s.error_rows:
            parts.append(f"错误 {s.error_rows}")
        if s.warn_rows:
            parts.append(f"警告 {s.warn_rows}")
        if s.over_limit_rows:
            parts.append(f"超档位 {s.over_limit_rows}")
        return "，".join(parts)


@dataclass(slots=True)
class CheckedProject:
    """加上体检报告。"""

    computed: ComputedProject
    report: HealthReport

    @property
    def project(self) -> LoadedProject:
        return self.computed.project


def load_project(
    *,
    floor_source: FloorSource,
    cabling_source: CablingSource,
    label: str,
    source_path: Path,
    rule: LengthRule = DEFAULT_RULE,
) -> LoadedProject:
    """读入 + 解析几何 + 适配连线。不做任何计算。"""
    cells, _row_range, _col_range = floor_source.read_floor_cells()
    floor = parse_floor([FloorCell(c[0], c[1], c[2]) for c in cells])
    cmax = derive_cmax_by_room(floor.racks)
    snapshot = build_snapshot(floor.racks, version=label, cmax_override=cmax)

    preview, audit_rows, audit_row_nos, audit_col = cabling_source.read_links()
    return LoadedProject(
        label=label,
        source_path=source_path,
        geometry=snapshot,
        rack_count=len(floor.racks),
        cmax_by_room=cmax,
        links=preview.links,
        raw_values=preview.raw_values,
        total_rows=preview.total_rows,
        empty_rows=preview.empty_rows,
        audit_rows=audit_rows,
        audit_row_nos=audit_row_nos,
        audit_col=audit_col,
        skipped=tuple((s.row_no, s.reason) for s in preview.skipped),
        rule=rule,
    )


def compute_project(
    project: LoadedProject,
    *,
    table: EmpiricalTable | None = None,
    policies: Mapping[str, LengthPolicy] | None = None,
    overrides: Mapping[str, int] | None = None,
) -> ComputedProject:
    """算长度。同时给出几何批量结果与四策略决策。

    `batch` 是纯几何公式链（用于验证公式实现是否忠实于源表）；
    `resolved` 是产品实际用的决策（含策略推断与经验表建议）。
    两者都在，因为「算出来是多少」和「最终定多少」是两件事。
    """
    batch = compute_batch(project.links, project.rule, project.geometry)
    resolved = resolve_batch(
        project.links,
        project.rule,
        project.geometry,
        table or empty_table(project.rule.version_id),
        policies=policies,
        overrides=overrides,
    )
    return ComputedProject(project=project, batch=batch, resolved=resolved)


def check_project(computed: ComputedProject) -> CheckedProject:
    """跑体检规则。"""
    return CheckedProject(computed=computed, report=run_audit(computed.project.audit_context()))


__all__ = [
    "CheckedProject",
    "ComputedProject",
    "LoadedProject",
    "check_project",
    "compute_project",
    "load_project",
]
