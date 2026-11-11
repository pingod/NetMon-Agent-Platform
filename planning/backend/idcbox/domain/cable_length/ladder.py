"""长度阶梯 T：**有序规则表逐条匹配**，不是数组查找。

## 为什么这不是「档位向上取整」

原单元格（V13 布线表 T 列）是数组公式：

```
=_xlfn.IFS(S2<=5,5, S2<=6,7, S2=7,7, S2<=8.5,10, S2<=13,15, S2<=17.5,20,
           S2<=22,25, S2<=27,30, S2<=32,35, S2<=37,40)
```

注意第 2、3 条：`S<=6 → 7` 之后紧跟 `S=7 → 7`，接着 `S<=8.5 → 10`。
于是 **S ∈ (6, 7) 的开区间掉进了下一档 → 10 米**，而 S == 7 反而只有 7 米。
实测有 49 行的 S 恰好是 6.44（P=2 时的 `2*0.72+5`），全部被抬到 10 米。

三档实现的实测复现率（636 行有 T 的样本）：

| 实现 | 复现 | 差异行 |
|---|---|---|
| IFS 逐字（本模块 `IFS_COMPAT`） | 636/636 = 100.00% | 0 |
| 只补 (6,7) 缝隙的连续化（`CONTINUOUS`） | 587/636 = 92.30% | 49 |
| 档位表向上取整（`ladder_ceil_tiers`，仅测试负样本） | 424/636 = 66.67% | 212 |
| 档位表四舍五入（`ladder_round_nearest`，仅测试负样本） | 111/636 = 17.45% | 525 |

**默认走 `IFS_COMPAT`**：用户的 Excel 公式仍是原式，系统输出必须与它逐格一致，
否则导出的文件「自己把值改了」，信任当场崩塌。`CONTINUOUS` 只作为体检项：
由系统告诉用户「这 49 行按你的公式要买 10 米，按连续口径只需 7 米，多买一档」，
让用户主动决定。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Callable, Final, Literal, Sequence

from ..shared.types import Issue, IssueCode
from ..units import LADDER_TIERS, PURCHASE_TIERS
from .numeric import tier_eq, tier_le, tier_lt

LadderOp = Literal["le", "eq", "lt"]


@dataclass(frozen=True, slots=True)
class Rung:
    """一条阶梯规则。`seq` 仅用于报错定位与导出说明。"""

    seq: int
    op: LadderOp
    limit: float
    out: int
    label: str

    def matches(self, s: float) -> bool:
        if self.op == "le":
            return tier_le(s, self.limit)
        if self.op == "eq":
            return tier_eq(s, self.limit)
        return tier_lt(s, self.limit)


#: 原 IFS 公式逐字复刻。**这是生产默认档位表。**
IFS_COMPAT: Final[tuple[Rung, ...]] = (
    Rung(1, "le", 5.0, 5, "le_5"),
    Rung(2, "le", 6.0, 7, "le_6"),
    Rung(3, "eq", 7.0, 7, "eq_7"),
    Rung(4, "le", 8.5, 10, "le_8.5"),
    Rung(5, "le", 13.0, 15, "le_13"),
    Rung(6, "le", 17.5, 20, "le_17.5"),
    Rung(7, "le", 22.0, 25, "le_22"),
    Rung(8, "le", 27.0, 30, "le_27"),
    Rung(9, "le", 32.0, 35, "le_32"),
    Rung(10, "le", 37.0, 40, "le_37"),
)

#: 只把 `eq 7` 放宽成 `le 7`，补上 (6,7) 的开区间。仅供体检对比，不作默认。
CONTINUOUS: Final[tuple[Rung, ...]] = (
    Rung(1, "le", 5.0, 5, "le_5"),
    Rung(2, "le", 7.0, 7, "le_7"),
    Rung(3, "le", 8.5, 10, "le_8.5"),
    Rung(4, "le", 13.0, 15, "le_13"),
    Rung(5, "le", 17.5, 20, "le_17.5"),
    Rung(6, "le", 22.0, 25, "le_22"),
    Rung(7, "le", 27.0, 30, "le_27"),
    Rung(8, "le", 32.0, 35, "le_32"),
    Rung(9, "le", 37.0, 40, "le_37"),
)

LADDER_MODES: Final[dict[str, tuple[Rung, ...]]] = {
    "ifs_compat": IFS_COMPAT,
    "continuous": CONTINUOUS,
}


def ladder_round(s: float, rungs: Sequence[Rung] = IFS_COMPAT) -> int | None:
    """逐条匹配，第一条命中即返回。

    **禁止**用 bisect / ceil / 数组上取整替代 —— 本阶梯非单调
    （S=6.44 → 10 而 S=7 → 7），任何「单调档位」实现都会错 49 行。

    >>> ladder_round(5.0)
    5
    >>> ladder_round(6.44)   # 落在 (6,7) 开区间，被抬到 10
    10
    >>> ladder_round(7.0)    # 恰好等于 7 反而只有 7
    7
    >>> ladder_round(37.5) is None   # 超末档，Excel 返回 #N/A
    True
    """
    for r in rungs:
        if r.matches(s):
            return r.out
    return None


def ladder_round_with_issues(
    s: float, rungs: Sequence[Rung] = IFS_COMPAT
) -> tuple[int | None, tuple[Issue, ...]]:
    out = ladder_round(s, rungs)
    if out is None:
        return None, (
            Issue.warn(
                IssueCode.OVER_LIMIT,
                f"线缆长度合计 {s:g} 米超过末档 37 米，原公式会返回 #N/A，请人工定档",
                field="t_ladder_m",
                s_total_m=s,
            ),
        )
    return out, ()


# ---------------------------------------------------------------------------
# 下面两个函数**只用于测试负样本**，用来证明「档位法」不行。不得进生产路径。
# ---------------------------------------------------------------------------
def ladder_ceil_tiers(s: float, tiers: Sequence[int] = LADDER_TIERS) -> int | None:
    """档位向上取整（对照用，实测 424/636 正确）。生产禁用。"""
    for t in tiers:
        if tier_le(s, float(t)):
            return t
    return None


def ladder_round_nearest(s: float, tiers: Sequence[int] = LADDER_TIERS) -> int | None:
    """档位四舍五入（对照用，实测 111/636 正确）。生产禁用。"""
    if not tiers:
        return None
    best = min(tiers, key=lambda t: (abs(t - s), t))
    return best


#: 两个「档位法」的注册表，只给体检报告与测试用
NON_PRODUCTION_IMPLS: Final[dict[str, Callable[[float], int | None]]] = {
    "ladder_ceil_tiers": ladder_ceil_tiers,
    "ladder_round_nearest": ladder_round_nearest,
}


def to_purchase_tier(length_m: int | None) -> int | None:
    """展示档位 → 采购档位。35 米不单独采购，合并到 40。

    >>> to_purchase_tier(35), to_purchase_tier(10), to_purchase_tier(None)
    (40, 10, None)
    """
    if length_m is None:
        return None
    if length_m in PURCHASE_TIERS:
        return length_m
    for t in PURCHASE_TIERS:
        if t >= length_m:
            return t
    return None


def ladder_gap_rows(rungs: Sequence[Rung] = IFS_COMPAT) -> tuple[tuple[float, float], ...]:
    """找出档位表里被「漏掉」的开区间，用于体检报告。

    实测 `IFS_COMPAT` 有 2 段提前跳档区间：`(6, 7)` 与 `(7, 8.5]` 之前
    本应由 7 覆盖，但因 `eq 7` 的存在，(6,7) 无人接管。
    """
    gaps: list[tuple[float, float]] = []
    for prev, nxt in zip(rungs, rungs[1:]):
        if nxt.op == "eq" and prev.op == "le":
            gaps.append((prev.limit, nxt.limit))
    return tuple(gaps)
