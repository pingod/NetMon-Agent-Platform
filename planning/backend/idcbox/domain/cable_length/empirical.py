"""经验表：从历史数据学出 `(空间类, 线缆类型, 路径距离) → 长度`。

## 为什么必须要有这张表

V13 布线表 5113 条有效连线里，只有 **636 行（12.4%）** 走过公式链（P 列有值），
剩下 **4477 行（87.6%）** 连 N/O 都没填，U 是老师傅凭经验直接敲进去的。
如果系统只提供「几何公式 → T」，对这 88% 的行完全无能为力。

对这批行，唯一有意义的能力是：**从历史里学出经验规则，给出带置信度的建议值**。
实测（`rules/len_empirical_v1.json`，LOO 口径）：整体 87.11%，分层回退后
可建议覆盖率 85.2%、覆盖内纯度 94.02%。

## 为什么是三级回退而不是一张大表

细粒度键 `(空间类, 线缆, path)` 样本少时不可靠（path=14 只有 1 条样本）。
故按 `L1 → L2 → L3` 回退，并用门控 `min_sample_n=10 且 purity>=0.75` 兜住。
门控不达标的组合**直接转人工**，不猜。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal, Mapping, Protocol, Sequence

from ..shared.types import Issue, IssueCode
from .rules import OClass

ConfidenceLevel = Literal["HIGH", "MEDIUM", "LOW", "NA"]

#: 门控默认值，与 rules/len_empirical_v1.json 的 `gate` 一致
DEFAULT_MIN_SAMPLE_N: int = 10
DEFAULT_MIN_PURITY: float = 0.75
#: LOW 判定阈值：purity < 0.85 或 sample_n < 10 → 只作参考，不进自动汇总
ADVISORY_PURITY: float = 0.85
ADVISORY_SAMPLE_N: int = 10


@dataclass(frozen=True, slots=True)
class EmpiricalCell:
    """一条经验规则。`dist` 是完整分布，便于 UI 显示「还有 3 种别的长度」。"""

    length_m: int
    sample_n: int
    purity: float
    dist: Mapping[int, int] = field(default_factory=dict)

    @property
    def is_advisory(self) -> bool:
        """是否「仅供参考」：低纯度或样本不足，不得进自动采购汇总。"""
        return self.purity < ADVISORY_PURITY or self.sample_n < ADVISORY_SAMPLE_N


@dataclass(frozen=True, slots=True)
class EmpiricalLookup:
    """一次查询的完整结果，含命中的层级（便于解释「这个建议怎么来的」）。"""

    cell: EmpiricalCell | None
    level: Literal["L1", "L2", "L3", "NONE"]
    space_class: OClass
    cable_type: str
    path_units: int
    issues: tuple[Issue, ...] = ()

    @property
    def is_forced_manual(self) -> bool:
        return self.cell is None


class EmpiricalTable(Protocol):
    def lookup(self, space_class: OClass, cable_type: str, path_units: int) -> EmpiricalLookup: ...


@dataclass(frozen=True, slots=True)
class EmpiricalTableData:
    """经验表的纯数据形态（从 JSON 加载而来）。

    三级键刻意用扁平字符串而不是嵌套 dict —— JSON 不支持 tuple 键，
    且扁平结构便于按 `(cls, cab)` 前缀做覆盖率统计。
    """

    formula_version: str
    #: L1: (cls, cab, path) → cell
    l1: Mapping[tuple[str, str, int], EmpiricalCell]
    #: L2: (cls, cab) → cell
    l2: Mapping[tuple[str, str], EmpiricalCell]
    #: L3: cab → cell
    l3: Mapping[str, EmpiricalCell]
    gate: tuple[int, float] = (DEFAULT_MIN_SAMPLE_N, DEFAULT_MIN_PURITY)
    #: 强制人工的空间类（实测 C 类无可靠规律）
    force_manual_classes: tuple[str, ...] = ("C",)
    #: 强制人工的线缆类型（实测 MPO 样本少且纯度低）
    force_manual_cable_types: tuple[str, ...] = ("MPO",)

    # ------------------------------------------------------------------
    def lookup(self, space_class: OClass, cable_type: str, path_units: int) -> EmpiricalLookup:
        base: dict[str, object] = {
            "space_class": space_class,
            "cable_type": cable_type,
            "path_units": path_units,
        }

        if space_class in self.force_manual_classes or cable_type in self.force_manual_cable_types:
            return EmpiricalLookup(
                cell=None,
                level="NONE",
                space_class=space_class,
                cable_type=cable_type,
                path_units=path_units,
                issues=(
                    Issue.info(
                        IssueCode.EMPIRICAL_FORCE_MANUAL,
                        f"{_cls_label(space_class)} / {cable_type} 的历史数据不足以支撑建议，"
                        f"请人工核定长度",
                        field="suggest_len_m",
                        space_class=space_class,
                        cable_type=cable_type,
                    ),
                ),
            )

        min_n, min_purity = self.gate

        cell = self.l1.get((space_class, cable_type, path_units))
        if cell is not None and cell.sample_n >= min_n and cell.purity >= min_purity:
            return EmpiricalLookup(cell, "L1", space_class, cable_type, path_units)

        cell = self.l2.get((space_class, cable_type))
        if cell is not None and cell.sample_n >= min_n and cell.purity >= min_purity:
            return EmpiricalLookup(cell, "L2", space_class, cable_type, path_units)

        cell = self.l3.get(cable_type)
        if cell is not None and cell.sample_n >= min_n and cell.purity >= min_purity:
            return EmpiricalLookup(
                cell,
                "L3",
                space_class,
                cable_type,
                path_units,
                issues=(
                    Issue.info(
                        IssueCode.EMPIRICAL_LOW_PURITY,
                        f"该组合历史样本不足，按线缆类型 {cable_type} 的整体经验值 {cell.length_m} 米估算",
                        field="suggest_len_m",
                        cable_type=cable_type,
                    ),
                ),
            )

        return EmpiricalLookup(
            cell=None,
            level="NONE",
            space_class=space_class,
            cable_type=cable_type,
            path_units=path_units,
            issues=(
                Issue.info(
                    IssueCode.EMPIRICAL_NO_HIT,
                    f"没有找到「{_cls_label(space_class)} / {cable_type} / 距离 {path_units}」的历史经验，"
                    f"请人工核定长度",
                    field="suggest_len_m",
                    **base,  # type: ignore[arg-type]
                ),
            ),
        )

    # ------------------------------------------------------------------
    @property
    def entry_counts(self) -> tuple[int, int, int]:
        return len(self.l1), len(self.l2), len(self.l3)


def _cls_label(cls: str) -> str:
    return {"A": "同排", "B": "同通道跨侧", "C": "跨通道"}.get(cls, cls)


def gate_confidence(
    cell: EmpiricalCell | None, level: str
) -> tuple[ConfidenceLevel, bool]:
    """经验表命中 → (置信度, 是否仅供参考)。

    - 未命中 → NA，且必须转人工
    - L1 且高纯度 → HIGH
    - L2 / 纯度中等 → MEDIUM
    - 低纯度或样本不足 → LOW，`is_advisory=True`（不进自动采购汇总）
    """
    if cell is None:
        return "NA", True
    if cell.purity < ADVISORY_PURITY or cell.sample_n < ADVISORY_SAMPLE_N:
        return "LOW", True
    if level == "L1" and cell.purity >= 0.9:
        return "HIGH", False
    return "MEDIUM", False


def empty_table(formula_version: str = "unset") -> EmpiricalTableData:
    return EmpiricalTableData(formula_version=formula_version, l1={}, l2={}, l3={})


def build_table(
    formula_version: str,
    l1: Mapping[tuple[str, str, int], EmpiricalCell],
    l2: Mapping[tuple[str, str], EmpiricalCell],
    l3: Mapping[str, EmpiricalCell],
    *,
    gate: tuple[int, float] = (DEFAULT_MIN_SAMPLE_N, DEFAULT_MIN_PURITY),
    force_manual_classes: Sequence[str] = ("C",),
    force_manual_cable_types: Sequence[str] = ("MPO",),
) -> EmpiricalTableData:
    return EmpiricalTableData(
        formula_version=formula_version,
        l1=dict(l1),
        l2=dict(l2),
        l3=dict(l3),
        gate=gate,
        force_manual_classes=tuple(force_manual_classes),
        force_manual_cable_types=tuple(force_manual_cable_types),
    )
