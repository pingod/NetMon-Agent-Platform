"""长度策略（LengthPolicy）：存量迁移的唯一出路。

## 为什么不是「系统算 + 人工微调」两层

早期架构写的是「几何层 + 经验层两层分离」。逐行审计 V13 后这个前提塌了：

- 5113 条有效连线里，**只有 636 行（12.4%）** 走过公式链（P 列有值）
- **4477 行（87.6%）** 连 N/O 都是空的，U 是人工从零凭经验填的
- 二者相加恰好闭合 636 + 4477 = 5113

如果系统只有「算」和「微调」，导入存量项目时这 88% 的行会被全部标红
「计算值与人工值不符」—— 产品当天不可用。

所以正确抽象是四种**行级可选**的策略：

| 策略 | 语义 | 参与「与计算值不符」告警 |
|---|---|---|
| GEOMETRIC | 几何公式链 → T | 是 |
| EMPIRICAL | 经验表 → 建议值（带置信度） | 是（LOW 仅参考） |
| MANUAL | 人工填写，公式与经验表只作参考 | 否（人工即权威） |
| IMPORTED | 存量迁移：保留原值 | **否**（否则 4477 行全红） |

## 四条语义铁律

1. `calc_len_t` 与 `suggest_len_m` **任何策略下都恒算**，否则失去对比基准。
2. `IMPORTED` 行 `approved_len_u` **恒等于原值**，重算不覆盖，导出原样输出。
3. 告警分层：`IMPORTED` 不参与长度偏差告警；`LOW` 置信默认 `is_advisory=True`，
   **不进自动耗材汇总**。
4. 策略切换是行级操作，必须写 `audit_log` 并保留旧值（由 application 层落实）。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from typing import Literal, Mapping, Sequence

from ..geometry.layout import GeometrySnapshot
from ..shared.types import Issue, IssueCode
from .empirical import EmpiricalTable, gate_confidence
from .engine import LengthResult, compute_link
from .formulas import classify_o, path_units
from .link import LinkInput
from .rules import LengthRule, OClass


class LengthPolicy(StrEnum):
    """长度策略。存库、导出、改判都用这个枚举的字符串值。"""

    GEOMETRIC = "GEOMETRIC"
    EMPIRICAL = "EMPIRICAL"
    MANUAL = "MANUAL"
    IMPORTED = "IMPORTED"


#: 是否参与「与计算值不符」告警
POLICY_IN_DEVIATION_CHECK: dict[LengthPolicy, bool] = {
    LengthPolicy.GEOMETRIC: True,
    LengthPolicy.EMPIRICAL: True,
    LengthPolicy.MANUAL: False,
    LengthPolicy.IMPORTED: False,
}


@dataclass(frozen=True, slots=True)
class LengthCandidate:
    """某一策略给出的结果，用于四路并排对比。"""

    policy: LengthPolicy
    length_m: int | None
    confidence: Literal["HIGH", "MEDIUM", "LOW", "NA"]
    sample_n: int = 0
    purity: float | None = None
    is_advisory: bool = False
    note: str = ""
    issues: tuple[Issue, ...] = ()


@dataclass(frozen=True, slots=True)
class ResolvedLength:
    """一条连线最终落地的长度决策。

    UI 上必须三列并排显示：**计算长度 T | 建议长度 | 核定长度 U**。
    只显示一个数字，用户永远不知道系统在替他做什么。
    """

    stable_key: str
    policy: LengthPolicy
    #: 几何公式算出的 T（只读）
    calc_len_t: int | None = None
    #: 经验表建议值（只读）
    suggest_len_m: int | None = None
    #: 最终核定值（可编辑）
    approved_len_u: int | None = None
    confidence: Literal["HIGH", "MEDIUM", "LOW", "NA"] = "NA"
    calc_confidence: Literal["A", "B", "C", "NA"] = "NA"
    sample_n: int = 0
    purity: float | None = None
    is_advisory: bool = False
    over_limit: bool = False
    o_class: OClass | None = None
    path_units: int | None = None
    length_source: str = "none"
    alternatives: Mapping[LengthPolicy, LengthCandidate] = field(default_factory=dict)
    issues: tuple[Issue, ...] = ()

    @property
    def has_error(self) -> bool:
        return any(i.severity == "error" for i in self.issues)

    @property
    def participates_in_deviation(self) -> bool:
        return POLICY_IN_DEVIATION_CHECK[self.policy]


# ---------------------------------------------------------------------------
# 四条候选路径
# ---------------------------------------------------------------------------
def candidate_geometric(
    link: LinkInput, rule: LengthRule, geo: GeometrySnapshot
) -> tuple[LengthCandidate, LengthResult]:
    r = compute_link(link, rule, geo)
    conf: Literal["HIGH", "MEDIUM", "LOW", "NA"] = {
        "A": "HIGH",
        "B": "HIGH",
        "C": "MEDIUM",
    }.get(r.confidence, "NA")  # type: ignore[arg-type]
    note = "" if r.o_class != "C" else "跨通道估计，建议复核"
    return (
        LengthCandidate(
            policy=LengthPolicy.GEOMETRIC,
            length_m=r.t_ladder_m,
            confidence=conf,
            note=note,
            issues=r.issues,
        ),
        r,
    )


def candidate_empirical(
    link: LinkInput,
    table: EmpiricalTable,
    rule: LengthRule,
    geo: GeometrySnapshot,
) -> LengthCandidate:
    pos_a, _ = geo.resolve(link.a.rack)
    pos_b, _ = geo.resolve(link.b.rack)
    if pos_a is None or pos_b is None:
        return LengthCandidate(
            policy=LengthPolicy.EMPIRICAL,
            length_m=None,
            confidence="NA",
            note="机柜不在平面图中，无法查经验表",
        )
    cls = classify_o(pos_a, pos_b)
    pu = path_units(pos_a, pos_b, rule)
    lk = table.lookup(cls, link.cable_type, pu)
    conf, advisory = gate_confidence(lk.cell, lk.level)
    return LengthCandidate(
        policy=LengthPolicy.EMPIRICAL,
        length_m=None if lk.cell is None else lk.cell.length_m,
        confidence=conf,
        sample_n=0 if lk.cell is None else lk.cell.sample_n,
        purity=None if lk.cell is None else lk.cell.purity,
        is_advisory=advisory,
        note=(
            f"命中 {lk.level}（样本 {lk.cell.sample_n}，纯度 {lk.cell.purity:.0%}）"
            if lk.cell is not None
            else "无历史经验，需人工"
        ),
        issues=lk.issues,
    )


def candidate_manual(link: LinkInput) -> LengthCandidate:
    return LengthCandidate(
        policy=LengthPolicy.MANUAL,
        length_m=link.imported_len_u,
        confidence="NA",
        note="人工填写值为准" if link.imported_len_u is not None else "尚未人工填写",
    )


def candidate_imported(link: LinkInput) -> LengthCandidate:
    return LengthCandidate(
        policy=LengthPolicy.IMPORTED,
        length_m=link.imported_len_u,
        confidence="NA",
        note=(
            f"沿用原表值 {link.imported_len_u} 米"
            if link.imported_len_u is not None
            else f"原值为文本「{link.imported_len_raw}」，原样保留"
            if link.imported_len_raw is not None
            else "原表无长度值"
        ),
    )


# ---------------------------------------------------------------------------
# 统一入口
# ---------------------------------------------------------------------------
def compute_all_policies(
    link: LinkInput,
    rule: LengthRule,
    geo: GeometrySnapshot,
    table: EmpiricalTable,
) -> tuple[dict[LengthPolicy, LengthCandidate], LengthResult]:
    """四条路一次算完，供 UI 并排对比。返回 (候选表, 几何结果)。"""
    geo_cand, geo_result = candidate_geometric(link, rule, geo)
    cands: dict[LengthPolicy, LengthCandidate] = {
        LengthPolicy.GEOMETRIC: geo_cand,
        LengthPolicy.EMPIRICAL: candidate_empirical(link, table, rule, geo),
        LengthPolicy.MANUAL: candidate_manual(link),
        LengthPolicy.IMPORTED: candidate_imported(link),
    }
    return cands, geo_result


def infer_policy(link: LinkInput, geo_result: LengthResult) -> LengthPolicy:
    """推断一条连线「本来」该用哪种策略。

    判据只有一条硬规则：**原表有值就当 IMPORTED**，因为存量项目的价值观是
    「现场已经这么干了」，系统没有资格在导入时改它。
    """
    if link.imported_len_u is not None or link.imported_len_raw is not None:
        return LengthPolicy.IMPORTED
    if link.phase is not None or link.remark is not None:
        # 手工标注过批次/备注但没有长度 → 人工在建
        return LengthPolicy.MANUAL
    if geo_result.t_ladder_m is not None:
        return LengthPolicy.GEOMETRIC
    return LengthPolicy.MANUAL


def resolve_length(
    link: LinkInput,
    rule: LengthRule,
    geo: GeometrySnapshot,
    table: EmpiricalTable,
    *,
    policy: LengthPolicy | None = None,
    override_u: int | None = None,
) -> ResolvedLength:
    """算出一条连线的最终长度与四路对比。

    `calc_len_t`（几何 T）与 `suggest_len_m`（经验建议）**无论如何都算**，
    哪怕策略是 IMPORTED —— 否则用户永远看不到「系统认为应该是多少」。
    """
    cands, geo_result = compute_all_policies(link, rule, geo, table)
    chosen = policy or infer_policy(link, geo_result)

    # 空间分类与路径距离（供 UI 显示与经验表解释）
    pos_a, _ = geo.resolve(link.a.rack)
    pos_b, _ = geo.resolve(link.b.rack)
    o_class = classify_o(pos_a, pos_b) if (pos_a and pos_b) else None
    pu = path_units(pos_a, pos_b, rule) if (pos_a and pos_b) else None

    calc_t = geo_result.t_ladder_m
    empirical = cands[LengthPolicy.EMPIRICAL]
    suggest = empirical.length_m

    # 核定值按策略决定
    if override_u is not None:
        approved: int | None = override_u
    elif chosen is LengthPolicy.IMPORTED:
        approved = link.imported_len_u
    elif chosen is LengthPolicy.MANUAL:
        approved = link.imported_len_u
    elif chosen is LengthPolicy.GEOMETRIC:
        approved = calc_t
    else:  # EMPIRICAL
        approved = suggest if suggest is not None else calc_t

    # 置信度按「该策略实际依据的那条路径」取
    if chosen is LengthPolicy.EMPIRICAL:
        confidence = empirical.confidence
    elif chosen is LengthPolicy.IMPORTED or chosen is LengthPolicy.MANUAL:
        confidence = "NA"
    else:
        confidence = {
            "A": "HIGH",
            "B": "HIGH",
            "C": "MEDIUM",
            "NA": "NA",
        }.get(geo_result.confidence, "NA")  # type: ignore[arg-type]

    issues: list[Issue] = list(geo_result.issues)
    if chosen is LengthPolicy.EMPIRICAL:
        issues.extend(empirical.issues)
    if chosen is LengthPolicy.IMPORTED and link.imported_len_raw is not None:
        issues.append(
            Issue.info(
                IssueCode.HISTORICAL_T,
                f"原表长度是文本「{link.imported_len_raw}」，已原样保留，导出时按原文输出",
                field="approved_len_u",
            )
        )

    return ResolvedLength(
        stable_key=link.stable_key,
        policy=chosen,
        calc_len_t=calc_t,
        suggest_len_m=suggest,
        approved_len_u=approved,
        confidence=confidence,
        calc_confidence=geo_result.confidence,
        sample_n=empirical.sample_n,
        purity=empirical.purity,
        is_advisory=empirical.is_advisory if chosen is LengthPolicy.EMPIRICAL else False,
        over_limit=geo_result.over_limit,
        o_class=o_class,
        path_units=pu,
        length_source=geo_result.length_source,
        alternatives=cands,
        issues=tuple(issues),
    )


def resolve_batch(
    links: Sequence[LinkInput],
    rule: LengthRule,
    geo: GeometrySnapshot,
    table: EmpiricalTable,
    *,
    policies: Mapping[str, LengthPolicy] | None = None,
    overrides: Mapping[str, int] | None = None,
) -> list[ResolvedLength]:
    """批量决策。策略可按行覆盖；未指定的行自动推断。"""
    out: list[ResolvedLength] = []
    for link in links:
        p = (policies or {}).get(link.stable_key)
        u = (overrides or {}).get(link.stable_key)
        out.append(resolve_length(link, rule, geo, table, policy=p, override_u=u))
    return out

