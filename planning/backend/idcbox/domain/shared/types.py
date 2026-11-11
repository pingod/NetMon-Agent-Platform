"""域层共享值对象：Issue / Result / 枚举。

域层零 IO，所以「出问题」必须用**返回值**表达，而不是抛异常 —— 批量计算
5113 行时任何一行缺几何都不能中断其他行（架构 §4.7 第 2 条不可协商）。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from typing import Generic, Literal, TypeVar

T = TypeVar("T")

#: 问题级别。error 阻断该行结果，warn/info 不阻断。
Severity = Literal["error", "warn", "info"]


class IssueCode(StrEnum):
    """域层问题码。命名与 validation 规则码（R1..R30）刻意分离。

    域层只关心「能不能算出来」，validation 层关心「业务上对不对」。
    """

    # --- 几何 ---
    GEOM_MISSING = "GEOM_MISSING"
    AMBIGUOUS_RACK = "AMBIGUOUS_RACK"
    NOT_A_RACK = "NOT_A_RACK"
    CMAX_MISSING = "CMAX_MISSING"

    # --- 长度块 ---
    Q_MISSING = "Q_MISSING"
    Q_NOT_NUMERIC = "Q_NOT_NUMERIC"
    O_NOT_NUMERIC = "O_NOT_NUMERIC"
    P_NOT_NUMERIC = "P_NOT_NUMERIC"
    T_NOT_NUMERIC = "T_NOT_NUMERIC"
    OVER_LIMIT = "OVER_LIMIT"
    CROSS_AISLE_MANUAL = "CROSS_AISLE_MANUAL"

    # --- 经验表 ---
    EMPIRICAL_NO_HIT = "EMPIRICAL_NO_HIT"
    EMPIRICAL_FORCE_MANUAL = "EMPIRICAL_FORCE_MANUAL"
    EMPIRICAL_LOW_PURITY = "EMPIRICAL_LOW_PURITY"

    # --- 策略 ---
    HISTORICAL_U = "HISTORICAL_U"
    HISTORICAL_T = "HISTORICAL_T"

    # --- 端口 ---
    PORT_POOL_EXHAUSTED = "PORT_POOL_EXHAUSTED"
    PORT_ALREADY_USED = "PORT_ALREADY_USED"
    PHASE_INCONSISTENT = "PHASE_INCONSISTENT"
    NO_NET_PLANE_MAPPING = "NO_NET_PLANE_MAPPING"
    UNKNOWN_CABLE_TYPE = "UNKNOWN_CABLE_TYPE"
    #: 同一机柜同一槽位有多台设备（上架位置可能录错）
    RACK_SLOT_CONFLICT = "RACK_SLOT_CONFLICT"
    #: 交换机端口不够装下全部节点
    PORT_CAPACITY_EXCEEDED = "PORT_CAPACITY_EXCEEDED"
    #: 交换机分不到节点（前面的交换机已吃完）
    PORT_UNASSIGNED = "PORT_UNASSIGNED"


@dataclass(frozen=True, slots=True)
class Issue:
    """一条结构化问题。`message` 必须是给人看的中文，不用英文码凑。"""

    code: IssueCode
    severity: Severity
    message: str
    field: str | None = None
    context: tuple[tuple[str, object], ...] = ()

    @staticmethod
    def error(code: IssueCode, message: str, **ctx: object) -> "Issue":
        return Issue(code, "error", message, context=tuple(ctx.items()))

    @staticmethod
    def warn(code: IssueCode, message: str, **ctx: object) -> "Issue":
        return Issue(code, "warn", message, context=tuple(ctx.items()))

    @staticmethod
    def info(code: IssueCode, message: str, **ctx: object) -> "Issue":
        return Issue(code, "info", message, context=tuple(ctx.items()))


def has_error(issues: tuple[Issue, ...]) -> bool:
    return any(i.severity == "error" for i in issues)


@dataclass(frozen=True, slots=True)
class BatchStats:
    """批量计算的汇总。批量不中断的证据就是这张表。"""

    total_rows: int = 0
    computed_rows: int = 0
    error_rows: int = 0
    warn_rows: int = 0
    skipped_rows: int = 0
    over_limit_rows: int = 0
    #: code → 出现次数，便于「先看哪类问题最多」
    issues_by_code: tuple[tuple[str, int], ...] = ()

    @property
    def error_rate(self) -> float:
        return 0.0 if self.total_rows == 0 else self.error_rows / self.total_rows


@dataclass(slots=True)
class IssueCollector:
    """可变累加器，仅在引擎内部使用；对外一律给不可变 tuple。"""

    _items: list[Issue] = field(default_factory=list)

    def add(self, issue: Issue) -> None:
        self._items.append(issue)

    def extend(self, issues: tuple[Issue, ...]) -> None:
        self._items.extend(issues)

    def freeze(self) -> tuple[Issue, ...]:
        return tuple(self._items)


@dataclass(frozen=True, slots=True)
class Ok(Generic[T]):
    value: T


@dataclass(frozen=True, slots=True)
class Err:
    issues: tuple[Issue, ...]


Result = Ok[T] | Err
