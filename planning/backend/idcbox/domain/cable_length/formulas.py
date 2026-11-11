"""七个长度公式的**唯一实现**。

架构 §2.1 第 2 条铁律：`N/O/P/Q/R/S/T` 只许在这里实现一次。前端镜像
`web/src/domain/cableLength.ts` 必须通过同一份 golden vectors 才允许合入。

## 公式逐字（不得改动）

| 列 | 公式 | 实测复现 |
|---|---|---|
| N | `0 if Δrow == 0 else n_row_coef * Δrow + n_row_extra` | 627/636 = 98.6% |
| O(A) | `abs(col_a - col_b)`（两端同一平面图行） | 462/496 = 93.1% |
| O(B/C) | `(CMAX - col_a) + (CMAX - col_b) + 1` | B 98/100；C 24/40 |
| P | `N + O` | 636/636 |
| Q | `in_rack_len_by_type[cable_type]` | 636/636 |
| R | `P * rack_spacing_m * slack_ratio`，走 excel15 | 636/636 |
| S | `R + Q`，走 excel15 | 636/636 |
| T | 阶梯规则表 | 636/636（ifs_compat） |

## 为什么 R 要写 `P * 0.6 * 1.2` 而不用 `P * 0.72`

`0.72` 是积，不是事实。真事实是「每个机柜位 0.6 米」×「留 20% 余量」。
把积写死会让「换个间距 0.8 米的机房」变成改公式，而正确做法是改配置。
"""

from __future__ import annotations

from typing import Literal

from ..shared.types import Issue, IssueCode
from .numeric import excel15
from .rules import LengthRule, OClass, RackPos

#: O 的物理含义常量：走线架在排端，跨侧/跨通道必须走到排端再折返，
#: 故路径长度 = 两段「到排端距离」之和 + 1 个机柜位（跨过通道本身）。
CROSS_AISLE_OFFSET: int = 1


# ---------------------------------------------------------------------------
# N —— 跨机柜（平面图行号差）
# ---------------------------------------------------------------------------
def classify_o(a: RackPos, b: RackPos) -> OClass:
    """判定两端机柜的空间关系类别。

    - A：同一平面图行（同排面对面，隔一条通道）→ 横向直穿
    - B：同一条通道，分处两侧 → 走到本排端部再折返
    - C：跨通道 → 同样是排端折返，但可选走线架跨度更大，置信度最低
    """
    if a.row == b.row:
        return "A"
    if a.channel_no == b.channel_no:
        return "B"
    return "C"


def n_cross_rack(a: RackPos, b: RackPos, rule: LengthRule) -> int:
    """跨机柜 N：以「平面图行号差」为唯一自变量。

    >>> import sys; sys.path.insert(0, 'backend')
    >>> from idcbox.domain.geometry.floor import RackPlacement
    >>> p = RackPlacement('403', 1, 1, 'N', 38, 3)
    >>> q = RackPlacement('403', 1, 27, 'S', 38, 4)
    >>> n_cross_rack(p, q, __import__('idcbox.domain.cable_length.rules', fromlist=['DEFAULT_RULE']).DEFAULT_RULE)
    8
    """
    d = abs(a.row - b.row)
    if d == 0:
        return 0
    return rule.n_row_coef * d + rule.n_row_extra


# ---------------------------------------------------------------------------
# O —— 间隔机柜
# ---------------------------------------------------------------------------
def o_gap_rack(
    a: RackPos,
    b: RackPos,
    rule: LengthRule,
    o_class: OClass | None = None,
) -> tuple[int | None, tuple[Issue, ...]]:
    """间隔机柜 O。分三种空间关系，A 类横向直穿，B/C 类排端折返。

    跨通道（C 类）在 `o_cross_aisle_mode='manual_only'` 下只返回 None + 强制人工，
    这是**唯一允许的降级开关**，且降级会记入 `formula_version`。
    """
    cls = o_class or classify_o(a, b)

    if cls == "A":
        return abs(a.col - b.col), ()

    # B / C：排端折返
    if cls == "C":
        if rule.o_cross_aisle_mode == "manual_only":
            return None, (
                Issue.warn(
                    IssueCode.CROSS_AISLE_MANUAL,
                    f"跨通道连线（通道 {a.channel_no} → {b.channel_no}）按配置不自动给值，请人工填写间隔机柜",
                    field="o_gap_rack",
                    channel_a=a.channel_no,
                    channel_b=b.channel_no,
                ),
            )
        manual = rule.override_for(a.channel_no, b.channel_no)
        if manual is not None:
            return manual, (
                Issue.info(
                    IssueCode.CROSS_AISLE_MANUAL,
                    f"通道 {a.channel_no}→{b.channel_no} 使用人工校准值 {manual}",
                    field="o_gap_rack",
                ),
            )

    cmax_a = rule.cmax(a.room_code)
    cmax_b = rule.cmax(b.room_code)
    issues: list[Issue] = []
    if cmax_a is None:
        issues.append(
            Issue.error(
                IssueCode.CMAX_MISSING,
                f"房间 {a.room_code} 未配置排端列坐标（CMAX），跨排间隔无法计算",
                field="cmax_by_room",
                room=a.room_code,
            )
        )
    if cmax_b is None:
        issues.append(
            Issue.error(
                IssueCode.CMAX_MISSING,
                f"房间 {b.room_code} 未配置排端列坐标（CMAX），跨排间隔无法计算",
                field="cmax_by_room",
                room=b.room_code,
            )
        )
    if issues:
        return None, tuple(issues)

    o = (cmax_a - a.col) + (cmax_b - b.col) + CROSS_AISLE_OFFSET  # type: ignore[operator]
    if cls == "C":
        issues.append(
            Issue.info(
                IssueCode.CROSS_AISLE_MANUAL,
                "跨通道连线的间隔机柜按排端折返公式估算，建议人工复核",
                field="o_gap_rack",
                channel_a=a.channel_no,
                channel_b=b.channel_no,
                o=o,
            )
        )
    return o, tuple(issues)


# ---------------------------------------------------------------------------
# P / Q / R / S
# ---------------------------------------------------------------------------
def p_rack_count(n: int, o: int) -> int:
    """机柜数量 P = N + O。原表单元格写的就是 `=N2+O2`。"""
    return n + o


def q_in_rack(
    cable_type: str,
    rule: LengthRule,
    override: float | None = None,
) -> tuple[float | None, tuple[Issue, ...]]:
    """机内线缆 Q：按线缆类型查表。RJ45 固定 7 米是硬规则（实测 254/254）。

    `override` 用于「导入的原值」—— 存量项目里 Q 是人工填的，必须尊重。
    """
    if override is not None:
        return override, ()
    v = rule.q_of(cable_type)
    if v is not None:
        return float(v), ()
    return rule.default_unknown_q, (
        Issue.warn(
            IssueCode.UNKNOWN_CABLE_TYPE,
            f"线缆类型「{cable_type}」不在机内长度表里，暂按 {rule.default_unknown_q:g} 米计算，请确认",
            field="q_in_rack_m",
            cable_type=cable_type,
        ),
    )


def r_inter_rack(p: int, rule: LengthRule) -> float:
    """柜间线缆 R = P × 机柜间距 × 余量系数，结果收敛到 Excel 15 位口径。

    原表是 `=P2*0.6*120%`，故实现必须是「P × 0.6 × 1.2」而不是「P × 0.72」。
    """
    return excel15(p * rule.rack_spacing_m * rule.slack_ratio)


def s_total(r: float, q: float) -> float:
    """线缆合计 S = R + Q，走 excel15。"""
    return excel15(r + q)


# ---------------------------------------------------------------------------
# T —— 阶梯
# ---------------------------------------------------------------------------
def t_ladder(s: float, rule: LengthRule) -> int | None:
    return rule.t_of(s)


#: 公式元数据，供导出与文档生成使用（避免公式说明散落在各处）
FORMULA_META: dict[str, dict[str, str]] = {
    "N": {"expr": "0 if Δrow==0 else 4*Δrow+4", "unit": "柜位", "impl": "n_cross_rack"},
    "O": {"expr": "A: |Δcol| ; B/C: (CMAX-col_a)+(CMAX-col_b)+1", "unit": "柜位", "impl": "o_gap_rack"},
    "P": {"expr": "N+O", "unit": "柜位", "impl": "p_rack_count"},
    "Q": {"expr": "in_rack_len_by_type[cable_type]", "unit": "米", "impl": "q_in_rack"},
    "R": {"expr": "P*0.6*1.2", "unit": "米", "impl": "r_inter_rack"},
    "S": {"expr": "R+Q", "unit": "米", "impl": "s_total"},
    "T": {"expr": "IFS 逐字阶梯", "unit": "米", "impl": "ladder_round"},
}


def path_units(a: RackPos, b: RackPos, rule: LengthRule) -> int:
    """经验表键：路径距离（列坐标单位）。

    - A 类：`abs(col_a - col_b)` —— 面对面直穿
    - B/C 类：`(CMAX[room_a] - col_a) + (CMAX[room_b] - col_b) + 1` —— 排端折返

    这个量与 O 的公式**同源**，但语义不同：O 是「间隔了几个机柜」，path 是
    「线要走多远（以柜位为尺）」。经验表用 path 作键，因为线长取决于走线距离
    而不是间隔数量。
    """
    if a.row == b.row:
        return abs(a.col - b.col)
    cmax_a = rule.cmax(a.room_code)
    cmax_b = rule.cmax(b.room_code)
    if cmax_a is None or cmax_b is None:
        return abs(a.col - b.col)
    return (cmax_a - a.col) + (cmax_b - b.col) + CROSS_AISLE_OFFSET
