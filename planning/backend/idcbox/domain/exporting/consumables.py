"""耗材与模块需求汇总（纯函数，零 IO）。

## 这个模块解决什么

布线表回答「哪根线接哪里」，耗材清单回答「**要买多少根**」。
后者是采购依据，写错会直接造成停工或积压。

## 实测：需求数量 100% 可由布线表汇总得出（不是估算）

对 V13 真实数据逐组核验（`.scratch/probe_consumable.py`）：

| 口径 | 布线表汇总 | 原表「需求数量」 | 结论 |
|---|---|---|---|
| 线缆（类型 × 长度，共 24 组） | 450 根 | 450 根 | **24/24 组逐组相等** |
| 模块（4 类，本端/对端分列） | 178/140/100/12 | 178/140/100/12 | **4/4 类逐项相等** |

所以 `cable_demand()` 与 `module_demand()` 是**确定性函数**，不需要人工核。

## 但「采购数量」必须人工定 —— 实测冗余系数从 1.12 到 2.31

原表标题写着「采购数量在需求数量上，增加30%冗余」，实测却完全不是：

| 模块 | 需求 | 采购 | 实际倍数 |
|---|---|---|---|
| 100G-SR4 | 178 | 200 | 1.124 |
| 10G-SR | 140 | 324 | **2.314** |
| 25G-SR | 100 | 156 | 1.560 |
| 光转电 | 12 | 15 | 1.250 |

`10G-SR` 买到 2.3 倍是因为**备件与扩容预留**，`100G-SR4` 只买 1.12 倍是因为贵。
这类判断依赖预算与供应商交期，程序只能给「需求量 + 建议采购量（按可配置系数）」，
**最终采购量必须人工核定并留痕**。这与线长 U 列是同一个道理（见 `cable_length` 的四策略）。

## 覆盖率必须显式披露，禁止外推

模块列只有 570/5113 行有值（11.1%）。若按 11.1% 的覆盖率外推全量需求，
会**少买模块**导致现场停工。所以 `ModuleDemand.coverage` 强制携带
「覆盖了多少行 / 总行数」，UI 与导出都必须显示，不允许静默按比例放大。
"""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass, field
from typing import Iterable, Literal, Mapping, Sequence

from ..units import PURCHASE_TIERS

#: 采购档位（8 档）。35 米不单独采购，并入 40 米 —— 实测原表无 35 米列。
PURCHASE_LADDER: tuple[int, ...] = PURCHASE_TIERS

#: 计入采购的模块型号。`一`（铜缆直连）与 `设备自带` 不进采购清单。
PROCURE_MODULES: frozenset[str] = frozenset(
    {"100G-SR4", "10G-SR", "25G-SR", "光转电", "40G-SR4", "100G-LR4"}
)

#: 不计采购的 sentinel 值（原样保留在布线表，但不产生采购需求）。
NON_PROCURE_MODULES: frozenset[str] = frozenset({"一", "设备自带", "-", "—", "无", ""})

#: 采集端别：本端 / 对端。模块需求要分列，因为同一模块可能只在一侧需要。
ModuleSide = Literal["local", "peer"]


@dataclass(frozen=True, slots=True)
class CableDemandRow:
    """一条线缆需求：`(类型, 长度)` → 需要多少根。"""

    cable_type: str
    length_m: int
    count: int


@dataclass(frozen=True, slots=True)
class CableDemand:
    """线缆需求汇总。

    ## 为什么 `rows` 只含有需求的组合、不铺满全档位

    24 组里有 7 组需求为 0（如 MPO 5 米、MPO 7 米）。原表把 0 也列出来了 ——
    那是为了让人一眼看到「这一档我一根都不需要」。导出时由调用方决定是否补零行
    （`include_zero=True`），域层只如实报有需求的组合。
    """

    rows: tuple[CableDemandRow, ...]
    rows_total: int

    @property
    def total_count(self) -> int:
        return sum(r.count for r in self.rows)

    def by_type(self) -> Mapping[str, int]:
        """按线缆类型汇总根数。"""
        acc: Counter[str] = Counter()
        for r in self.rows:
            acc[r.cable_type] += r.count
        return dict(acc)

    def matrix(self, *, include_zero: bool = False) -> dict[str, dict[int, int]]:
        """`{类型: {长度: 根数}}`。`include_zero=True` 时补全 8 个采购档位。"""
        out: dict[str, dict[int, int]] = {}
        for r in self.rows:
            out.setdefault(r.cable_type, {})[r.length_m] = r.count
        if include_zero:
            for ct in out:
                for tier in PURCHASE_LADDER:
                    out[ct].setdefault(tier, 0)
        return out

    def purchase_suggestion(
        self, ratio: float = 1.3, *, ratio_overrides: Mapping[str, float] | None = None
    ) -> tuple[CableDemandRow, ...]:
        """建议采购量（向上取整到整根）。

        ## 为什么这是「建议」而不是「结论」

        实测原表的人工采购量与任何固定系数都对不上（模块侧 1.12~2.31 倍）。
        这里给一个**可配置的默认系数**只是为了让清单不为空，
        真正的采购量必须由人改 —— 所以返回类型叫 `suggestion` 不叫 `purchase`。
        """
        overrides = ratio_overrides or {}
        out: list[CableDemandRow] = []
        for r in self.rows:
            k = overrides.get(r.cable_type, ratio)
            out.append(
                CableDemandRow(
                    cable_type=r.cable_type,
                    length_m=r.length_m,
                    count=_ceil_mul(r.count, k),
                )
            )
        return tuple(out)


@dataclass(frozen=True, slots=True)
class ModuleDemandRow:
    """一类模块的需求：本端 / 对端分列，实际需求 = 两端之和。

    ## 为什么本端对端要分列

    同一根光纤两端各插一个模块，采购时必须买两个。但有些链路一端是
    `设备自带`（服务器原厂配好），这时只有交换机侧要买 ——
    实测光转电 12/0 就是这种（12 台密评设备自带电口，交换机侧配光转电）。
    合并成单列会丢掉这个差别，采购时无法解释「为什么是 2 倍」。
    """

    module: str
    local_count: int
    peer_count: int

    @property
    def total(self) -> int:
        return self.local_count + self.peer_count


@dataclass(frozen=True, slots=True)
class ModuleDemand:
    """模块需求汇总 + 覆盖率披露。

    `coverage_rows / total_rows` 必须原样显示给用户：
    实测模块列只填了 11.1% 的行，按这个比例外推会**少买**。
    """

    rows: tuple[ModuleDemandRow, ...]
    coverage_rows: int
    total_rows: int

    @property
    def coverage_ratio(self) -> float:
        return self.coverage_rows / self.total_rows if self.total_rows else 0.0

    def is_fully_covered(self) -> bool:
        return self.total_rows > 0 and self.coverage_rows >= self.total_rows

    def disclosure(self) -> str:
        """一句话披露覆盖率。UI 与 Excel 都必须显示这句。"""
        pct = self.coverage_ratio * 100
        return (
            f"模块需求覆盖 {self.coverage_rows}/{self.total_rows} 行（{pct:.1f}%）；"
            "其余行未填模块，未做外推 —— 请补齐后再据此采购"
        )


def _ceil_mul(n: int, ratio: float) -> int:
    """`n * ratio` 向上取整。`n=0` 时返回 0（0 根的 1.3 倍仍是 0）。"""
    if n <= 0:
        return 0
    # 用整数运算避免浮点误差把 200.0000001 抬成 201
    scaled = n * ratio
    whole = int(scaled)
    return whole if float(whole) == scaled else whole + 1


def cable_demand(
    rows: Iterable[tuple[str | None, int | None]],
    *,
    ladder: Sequence[int] = PURCHASE_LADDER,
) -> CableDemand:
    """线缆需求汇总。

    参数 `rows` 是 `(线缆类型, 长度)` 的可迭代流 —— 一行代表一根线。
    长度落在采购档位之外时**向上并入最近的档位**（实测原表 35 米并入 40 米），
    超过最大档位则原样保留该长度并单独成行（不静默丢弃）。

    >>> cable_demand([("LC-LC", 10), ("LC-LC", 10), ("RJ45", 5)]).total_count
    3
    >>> list(cable_demand([("LC-LC", 10)] * 3).by_type().items())
    [('LC-LC', 3)]
    >>> [r.count for r in cable_demand([("MPO", 35)]).rows]
    [1]
    >>> [r.length_m for r in cable_demand([("MPO", 35)]).rows]
    [40]
    """
    acc: Counter[tuple[str, int]] = Counter()
    total = 0
    for raw_type, raw_len in rows:
        if raw_len is None:
            continue
        ct = (raw_type or "UNKNOWN").strip() or "UNKNOWN"
        length = _fold_to_tier(int(raw_len), ladder)
        acc[(ct, length)] += 1
        total += 1

    ordered = sorted(acc.items(), key=lambda kv: (kv[0][0], kv[0][1]))
    return CableDemand(
        rows=tuple(
            CableDemandRow(cable_type=ct, length_m=ln, count=n) for (ct, ln), n in ordered
        ),
        rows_total=total,
    )


def _fold_to_tier(length: int, ladder: Sequence[int]) -> int:
    """把长度并到采购档位：取第一个 ≥ 它的档位；超出最大档则原值返回。

    >>> _fold_to_tier(10, PURCHASE_LADDER)
    10
    >>> _fold_to_tier(35, PURCHASE_LADDER)
    40
    >>> _fold_to_tier(40, PURCHASE_LADDER)
    40
    >>> _fold_to_tier(50, PURCHASE_LADDER)
    50
    """
    for tier in ladder:
        if length <= tier:
            return tier
    return length


def module_demand(
    rows: Iterable[tuple[str | None, str | None]],
    *,
    total_rows: int,
    procure: frozenset[str] = PROCURE_MODULES,
    non_procure: frozenset[str] = NON_PROCURE_MODULES,
) -> ModuleDemand:
    """模块需求汇总。

    参数 `rows` 是 `(本端模块, 对端模块)` 的可迭代流 —— 一行代表一根线。
    `None` / 空串 / sentinel（`一`、`设备自带`）不计入需求，但**计入覆盖率分母**
    里的「已填」判定要区别对待：

    - 有值且是真实模块 → 计入需求，也计入 coverage
    - 有值但是 sentinel → 不计需求，但计入 coverage（这一行确实填过了）
    - 空 → 不计需求，不计 coverage

    ## 为什么 sentinel 算「已覆盖」

    `一` 表示铜缆直连、`设备自带` 表示服务器原厂模块 —— 这两种都是**明确的结论**，
    不是「没填」。把它们算成未覆盖会让覆盖率虚低，用户会以为还有很多行要补。

    >>> m = module_demand([("10G-SR", "设备自带"), (None, None)], total_rows=2)
    >>> m.coverage_rows
    1
    >>> [(r.module, r.local_count, r.peer_count) for r in m.rows]
    [('10G-SR', 1, 0)]
    >>> m.is_fully_covered()
    False
    """
    local: Counter[str] = Counter()
    peer: Counter[str] = Counter()
    covered = 0
    for raw_local, raw_peer in rows:
        hit = False
        for side, raw in (("local", raw_local), ("peer", raw_peer)):
            text = (raw or "").strip()
            if not text:
                continue
            hit = True
            if text in non_procure:
                continue
            if text not in procure:
                # 未知模块型号：也计入需求，但用原值 —— 不静默丢
                pass
            (local if side == "local" else peer)[text] += 1
        if hit:
            covered += 1

    modules = sorted(set(local) | set(peer))
    return ModuleDemand(
        rows=tuple(
            ModuleDemandRow(
                module=m, local_count=local.get(m, 0), peer_count=peer.get(m, 0)
            )
            for m in modules
        ),
        coverage_rows=covered,
        total_rows=total_rows,
    )


@dataclass(frozen=True, slots=True)
class ConsumableReport:
    """耗材与模块的完整报告 —— 一个 sheet 两块。"""

    cables: CableDemand
    modules: ModuleDemand
    purchase_ratio: float = 1.3
    notes: tuple[str, ...] = field(default=())

    def cable_purchase_rows(self) -> tuple[CableDemandRow, ...]:
        return self.cables.purchase_suggestion(self.purchase_ratio)


def build_consumable_report(
    lines: Iterable[tuple[str | None, int | None, str | None, str | None]],
    *,
    total_rows: int | None = None,
    purchase_ratio: float = 1.3,
    notes: Sequence[str] = (),
) -> ConsumableReport:
    """一次遍历同时算线缆与模块。

    参数 `lines` 每项是 `(线缆类型, 长度, 本端模块, 对端模块)`。
    长度取**核定长度 U**（人工定稿值）而不是计算长度 T ——
    采购依据必须是最终装的长度，不是公式算出来的。

    ## `total_rows` 为什么可省略

    调用方通常已经知道总行数。省略时按 `lines` 实际条数兜底，
    但这会让覆盖率虚高（把没填的行也算成已处理），所以生产路径必须显式传。

    >>> rep = build_consumable_report([("LC-LC", 10, "10G-SR", "设备自带")], total_rows=1)
    >>> rep.cables.total_count
    1
    >>> rep.modules.rows[0].module
    '10G-SR'
    >>> rep.cable_purchase_rows()[0].count
    2
    """
    cable_rows: list[tuple[str | None, int | None]] = []
    module_rows: list[tuple[str | None, str | None]] = []
    n = 0
    for ct, ln, lm, pm in lines:
        cable_rows.append((ct, ln))
        module_rows.append((lm, pm))
        n += 1
    return ConsumableReport(
        cables=cable_demand(cable_rows),
        modules=module_demand(module_rows, total_rows=total_rows if total_rows is not None else n),
        purchase_ratio=purchase_ratio,
        notes=tuple(notes),
    )


__all__ = [
    "CableDemand",
    "CableDemandRow",
    "ConsumableReport",
    "ModuleDemand",
    "ModuleDemandRow",
    "NON_PROCURE_MODULES",
    "PROCURE_MODULES",
    "PURCHASE_LADDER",
    "build_consumable_report",
    "cable_demand",
    "module_demand",
]
