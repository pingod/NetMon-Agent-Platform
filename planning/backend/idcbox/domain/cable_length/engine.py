"""线长计算引擎：把七公式串成一次完整计算。

三条不可协商（架构 §4.7）：

1. **纯函数**：不读时钟、不访 DB、无全局缓存，同输入必同输出。
2. **批量不中断**：单行缺几何/超档位只让该行 `issues` 非空，其余照算。
   5113 行里 1 行机柜不在平面图中，不能让整批失败 —— 现场就是要先看到那 5112 行。
3. **增量只碰影响面**：几何变更 → 受影响机柜两端连线；规则变更 → 全量。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Iterable, Literal, Mapping, Sequence

from ..geometry.layout import GeometrySnapshot
from ..shared.types import BatchStats, Issue, IssueCode, has_error
from .formulas import (
    classify_o,
    n_cross_rack,
    o_gap_rack,
    p_rack_count,
    q_in_rack,
    r_inter_rack,
    s_total,
    t_ladder,
)
from .link import LinkInput
from .rules import LengthRule, OClass

LengthSource = Literal[
    "computed",     # 七公式全链路算出 T
    "imported",     # 存量原值直接沿用
    "pending",      # 已算出 T 但未核定 U（新建项目的天然状态）
    "historical",   # 历史行没有长度块（Q 缺失）
    "none",         # 完全没有长度信息
]


@dataclass(frozen=True, slots=True)
class LengthResult:
    """单条连线的长度计算结果。

    结构化问题而非异常：任何一步算不出来就给 None + issues，
    **绝不写 0** —— 0 米线缆在现场是灾难。
    """

    stable_key: str
    n_cross_rack: int | None = None
    o_gap_rack: int | None = None
    p_rack_count: int | None = None
    q_in_rack_m: float | None = None
    r_inter_rack_m: float | None = None
    s_total_m: float | None = None
    t_ladder_m: int | None = None
    u_approved_m: int | None = None
    o_class: OClass | None = None
    path_units: int | None = None
    confidence: Literal["A", "B", "C", "NA"] = "NA"
    over_limit: bool = False
    length_source: LengthSource = "none"
    issues: tuple[Issue, ...] = ()

    @property
    def is_complete(self) -> bool:
        return self.s_total_m is not None and self.t_ladder_m is not None

    @property
    def has_error(self) -> bool:
        return has_error(self.issues)

    @property
    def final_length_m(self) -> int | None:
        """最终用于采购/施工的长度：核定值优先，否则计算值。"""
        if self.u_approved_m is not None:
            return self.u_approved_m
        return self.t_ladder_m


@dataclass(frozen=True, slots=True)
class BatchResult:
    results: tuple[LengthResult, ...]
    stats: BatchStats

    def by_key(self) -> dict[str, LengthResult]:
        return {r.stable_key: r for r in self.results}


#: 置信度映射：空间类 → 置信度标签（A/B 高、C 低）
_CONFIDENCE_OF_CLASS: dict[OClass, Literal["A", "B", "C"]] = {"A": "A", "B": "B", "C": "C"}


def compute_link(
    link: LinkInput,
    rule: LengthRule,
    geo: GeometrySnapshot,
    *,
    override_u: int | None = None,
    respect_imported: bool = True,
) -> LengthResult:
    """算一条连线。

    `respect_imported=True` 时，若该行带存量原值（`imported_len_u`），
    则 `u_approved_m` 原样保真，但 `t_ladder_m` / `suggest` 仍然照算 ——
    没有对比基准就没法告诉用户「你填的和公式算的差多少」。
    """
    issues: list[Issue] = []

    # --- 0. 存量原值（sentinel 文本如「待定」）--------------------------
    if link.imported_len_raw is not None and link.imported_len_u is None:
        issues.append(
            Issue.info(
                IssueCode.HISTORICAL_T,
                f"该行长度是文本「{link.imported_len_raw}」，导出时原样输出，不参与长度计算",
                field="u_approved_m",
                raw=link.imported_len_raw,
            )
        )

    # --- 1. 几何 ------------------------------------------------------
    pos_a, issues_a = geo.resolve(link.a.rack)
    pos_b, issues_b = geo.resolve(link.b.rack)
    issues.extend(issues_a)
    issues.extend(issues_b)

    if pos_a is None or pos_b is None:
        # 几何缺失：N/O/P/Q/R/S/T 全部为 None，不写 0
        return LengthResult(
            stable_key=link.stable_key,
            u_approved_m=override_u if override_u is not None else link.imported_len_u,
            length_source="none",
            issues=tuple(issues),
        )

    # --- 2. 空间关系分类 -----------------------------------------------
    o_class = classify_o(pos_a, pos_b)

    # --- 3. N / O / P --------------------------------------------------
    n = n_cross_rack(pos_a, pos_b, rule)
    o, o_issues = o_gap_rack(pos_a, pos_b, rule, o_class)
    issues.extend(o_issues)

    if o is None:
        # O 被人工/降级挡下：P/R/S/T 无法计算，但 N 仍有值
        return LengthResult(
            stable_key=link.stable_key,
            n_cross_rack=n,
            o_class=o_class,
            confidence=_CONFIDENCE_OF_CLASS[o_class],
            u_approved_m=override_u if override_u is not None else link.imported_len_u,
            length_source="none",
            issues=tuple(issues),
        )

    p = p_rack_count(n, o)

    # --- 4. Q / R / S --------------------------------------------------
    q, q_issues = q_in_rack(link.cable_type, rule)
    issues.extend(q_issues)

    if q is None:
        # Q 缺失：长度块整体 none，不补 0（历史行 4477 行属此情形）
        return LengthResult(
            stable_key=link.stable_key,
            n_cross_rack=n,
            o_gap_rack=o,
            p_rack_count=p,
            o_class=o_class,
            confidence=_CONFIDENCE_OF_CLASS[o_class],
            u_approved_m=override_u if override_u is not None else link.imported_len_u,
            length_source="historical",
            issues=tuple(issues),
        )

    r = r_inter_rack(p, rule)
    s = s_total(r, q)

    # --- 5. T ----------------------------------------------------------
    t = t_ladder(s, rule)
    over_limit = t is None
    if over_limit:
        issues.append(
            Issue.warn(
                IssueCode.OVER_LIMIT,
                f"线缆合计 {s:g} 米超过末档 37 米，原 Excel 公式会返回 #N/A，请人工定档",
                field="t_ladder_m",
                s_total_m=s,
            )
        )

    # --- 6. U 核定值 ----------------------------------------------------
    if override_u is not None:
        u: int | None = override_u
    elif respect_imported and link.imported_len_u is not None:
        u = link.imported_len_u
    else:
        u = None  # 待核定：新建项目的天然状态，不是错误

    if u is None and t is not None:
        length_source: LengthSource = "pending"
    elif u is not None and link.imported_len_u is not None:
        length_source = "imported"
    else:
        length_source = "computed"

    return LengthResult(
        stable_key=link.stable_key,
        n_cross_rack=n,
        o_gap_rack=o,
        p_rack_count=p,
        q_in_rack_m=q,
        r_inter_rack_m=r,
        s_total_m=s,
        t_ladder_m=t,
        u_approved_m=u,
        o_class=o_class,
        path_units=None,
        confidence=_CONFIDENCE_OF_CLASS[o_class],
        over_limit=over_limit,
        length_source=length_source,
        issues=tuple(issues),
    )


def compute_batch(
    links: Sequence[LinkInput],
    rule: LengthRule,
    geo: GeometrySnapshot,
    *,
    overrides: Mapping[str, int] | None = None,
    respect_imported: bool = True,
) -> BatchResult:
    """批量计算。任何单行失败都不影响其他行。"""
    results: list[LengthResult] = []
    error_rows = warn_rows = over_limit_rows = 0
    counter: dict[str, int] = {}

    for link in links:
        try:
            r = compute_link(
                link,
                rule,
                geo,
                override_u=(overrides or {}).get(link.stable_key),
                respect_imported=respect_imported,
            )
        except Exception as exc:  # noqa: BLE001 —— 批量合同要求兜住一切
            r = LengthResult(
                stable_key=link.stable_key,
                length_source="none",
                issues=(
                    Issue.error(
                        IssueCode.GEOM_MISSING,
                        f"该行计算时出现未预期错误：{type(exc).__name__}: {exc}",
                        field="*",
                    ),
                ),
            )
        results.append(r)
        if r.has_error:
            error_rows += 1
        if any(i.severity == "warn" for i in r.issues):
            warn_rows += 1
        if r.over_limit:
            over_limit_rows += 1
        for i in r.issues:
            counter[str(i.code)] = counter.get(str(i.code), 0) + 1

    stats = BatchStats(
        total_rows=len(links),
        computed_rows=sum(1 for r in results if r.is_complete),
        error_rows=error_rows,
        warn_rows=warn_rows,
        skipped_rows=sum(1 for r in results if r.length_source == "none"),
        over_limit_rows=over_limit_rows,
        issues_by_code=tuple(sorted(counter.items(), key=lambda kv: -kv[1])),
    )
    return BatchResult(tuple(results), stats)


# ---------------------------------------------------------------------------
# 增量与差异
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class FieldChange:
    field: str
    before: object
    after: object


@dataclass(frozen=True, slots=True)
class LengthChange:
    stable_key: str
    changes: tuple[FieldChange, ...]
    reason: Literal["geometry", "rule", "row", "initial"]

    @property
    def is_empty(self) -> bool:
        return not self.changes


#: 计算链派生的七个字段（N/O/P/Q/R/S/T）——重算时可能因几何或规则变化而变
_LENGTH_FIELDS: tuple[str, ...] = (
    "n_cross_rack",
    "o_gap_rack",
    "p_rack_count",
    "q_in_rack_m",
    "r_inter_rack_m",
    "s_total_m",
    "t_ladder_m",
)

#: 人工核定值与它所依赖的辅助字段。
#: 必须纳入 diff：策略切换（GEOMETRIC→MANUAL）或批量核定时 U 会变，
#: 这正是变更摘要里用户最关心的那一项 —— 漏了它，UI 会说"什么都没变"。
_APPROVED_FIELDS: tuple[str, ...] = ("u_approved_m", "length_source")


def diff_snapshots(prev: LengthResult | None, cur: LengthResult) -> list[FieldChange]:
    """比对新旧两次计算结果。首次计算（prev=None）只记录有值的字段。

    比对范围分两组：计算链七个字段 + 核定值/长度来源。
    后者必须纳入，否则「人工改了 U」在变更摘要里会静默消失。
    """
    out: list[FieldChange] = []
    for f in _LENGTH_FIELDS + _APPROVED_FIELDS:
        before = None if prev is None else getattr(prev, f)
        after = getattr(cur, f)
        if prev is None:
            if after is not None:
                out.append(FieldChange(f, None, after))
        elif before != after:
            out.append(FieldChange(f, before, after))
    return out


def compute_incremental(
    links: Sequence[LinkInput],
    changed_racks: set[tuple[int, int]],
    rule: LengthRule,
    geo: GeometrySnapshot,
    *,
    prev: Mapping[str, LengthResult] | None = None,
    rule_changed: bool = False,
) -> list[LengthChange]:
    """增量计算：只在影响面内重算。

    - `rule_changed=True` → 全量重算
    - 否则只重算「至少一端落在 `changed_racks` 里」的连线
    - 历史沿用保护：`t 为空但 u 有值` 的行一律跳过（绝不覆盖人工历史值）
    """
    out: list[LengthChange] = []
    for link in links:
        if not rule_changed:
            a_hit = (link.a.rack.channel_no, link.a.rack.seq_no) in changed_racks
            b_hit = (link.b.rack.channel_no, link.b.rack.seq_no) in changed_racks
            if not (a_hit or b_hit):
                continue

        old = None if prev is None else prev.get(link.stable_key)

        # 历史沿用保护
        if old is not None and old.t_ladder_m is None and old.u_approved_m is not None:
            continue

        cur = compute_link(link, rule, geo)
        changes = diff_snapshots(old, cur)
        if changes:
            out.append(
                LengthChange(
                    stable_key=link.stable_key,
                    changes=tuple(changes),
                    reason="geometry" if not rule_changed else "rule",
                )
            )
    return out
