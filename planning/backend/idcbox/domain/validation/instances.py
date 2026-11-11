"""设备实例抽取与 U 位体检（D3/D4）。

## 为什么「实例」必须先抽出来

连线表是**以连线为行**的：一台 44 口交换机占 44 行，一台 8 槽计算节点占 8 行 × N 口。
直接按行判「同机柜 U 位重叠」会得到 **833 对**噪音；按**唯一物理实例**判是 **13 对**。
这两个数字差 64 倍 —— 这就是「按行体检」和「按设备体检」的区别。

## 实例身份（四元组）

`(通道, 机柜, U位原文, 型号名归一化)`

四个分量各有理由：
- 通道 + 机柜 → 物理位置
- U位原文 → **必须用原文**：`40U` 与 `37U` 是不同槽位；`3-12U` 是占 10U 的整机
- 型号归一化 → 区分同柜同 U 的两个不同设备，同时合并 `带外交换机 1` / `带外交换机1`

## 两个口径必须同时给（见 FR-D3）

- **原样口径**（型号名不去空格）= 13 对 → 用户看到的「表里有多少处看起来冲突」
- **归一化口径**（型号名去空格）= 11 对 → 真实物理冲突

差 2 对来自 `带外交换机 1` / `带外交换机1` 是同一台设备。
报告必须同时给两个数并解释差异，否则用户会以为系统在乱报。
"""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass, field
from typing import Iterable, Literal, Sequence

from ..importing.normalize import norm_int, norm_name, norm_u_position
from ..shared.types import Issue, IssueCode

Side = Literal["local", "peer"]


def parse_u_span(value: object) -> tuple[int, int] | None:
    """把 13 种 U 位字符串形态解析成 (起始U, 结束U)。

    实测形态：`40U`(单U)、`3-12U`(区间跨10U)、`19-20U`(区间跨2U)、
    `34U`、`37U`、`38U`；容忍 `40`、`40u`、`3 - 12 U` 等变体。
    """
    if value is None:
        return None
    s = str(value).strip().upper().replace("U", "").replace(" ", "").replace("　", "")
    if not s:
        return None
    if "-" in s:
        parts = s.split("-")
        if len(parts) != 2:
            return None
        try:
            lo, hi = int(parts[0]), int(parts[1])
        except ValueError:
            return None
        return (lo, hi) if lo <= hi else (hi, lo)
    try:
        n = int(float(s))
    except ValueError:
        return None
    return (n, n)


def spans_overlap(a: tuple[int, int], b: tuple[int, int]) -> bool:
    return a[0] <= b[1] and b[0] <= a[1]


@dataclass(slots=True)
class DeviceInstance:
    """一台物理设备在表中的投影。同一台设备会被多行引用，故 rows 是列表。"""

    channel_no: int
    rack_no: int
    u_raw: str
    span: tuple[int, int]
    model_raw: str
    model: str
    names: set[str] = field(default_factory=set)
    ports: set[str] = field(default_factory=set)
    rows: list[int] = field(default_factory=list)

    @property
    def rack_key(self) -> tuple[int, int]:
        return (self.channel_no, self.rack_no)

    @property
    def key(self) -> tuple:
        return (self.channel_no, self.rack_no, self.u_raw, self.model)

    @property
    def size_u(self) -> int:
        return self.span[1] - self.span[0] + 1

    @property
    def display_name(self) -> str:
        named = sorted(n for n in self.names if n)
        return named[0] if named else self.model


@dataclass(frozen=True, slots=True)
class UConflict:
    """一对 U 位重叠。`is_name_variant` 为真时是"假冲突"（同一台设备两种写法）。"""

    channel_no: int
    rack_no: int
    a: DeviceInstance
    b: DeviceInstance
    is_name_variant: bool

    @property
    def label(self) -> str:
        return f"通道{self.channel_no} 机柜{self.rack_no}"


@dataclass(frozen=True, slots=True)
class InstanceInventory:
    """实例抽取结果。两个口径同时保留。"""

    normalized: tuple[DeviceInstance, ...]
    raw: tuple[DeviceInstance, ...]
    rows_total: int

    @property
    def count_normalized(self) -> int:
        return len(self.normalized)

    @property
    def count_raw(self) -> int:
        return len(self.raw)


def extract_instances(
    rows: Sequence[RowValues],
    row_nos: Sequence[int],
    col: dict[str, int],
) -> InstanceInventory:
    """从连线表行抽取设备实例（两种口径）。`col` 是列名→索引的映射。"""

    # 用索引遍历：行值可能重复，不能用 list.index 反查行号
    def build(normalize_model: bool) -> dict[tuple, DeviceInstance]:
        out: dict[tuple, DeviceInstance] = {}
        for i, v in enumerate(rows):
            for side in ("local", "peer"):
                ch = norm_int(v[col[f"ch_{side}"]])
                rk = norm_int(v[col[f"rack_{side}"]])
                u_raw = v[col[f"u_{side}"]]
                model_raw = v[col[f"model_{side}"]]
                span = parse_u_span(u_raw)
                if ch is None or rk is None or span is None or not model_raw:
                    continue
                mraw = str(model_raw).strip()
                model = norm_name(mraw) if normalize_model else mraw
                # 身份键必须走 norm_u_position，不能用本地的大写去空格：
                # 否则 `3-12U` 与 `3—12U` 会成两个实例，把同一台设备报成叠放冲突。
                u_norm = norm_u_position(u_raw) or str(u_raw).strip()
                key = (ch, rk, u_norm, model)
                inst = out.get(key)
                if inst is None:
                    inst = DeviceInstance(ch, rk, u_norm, span, mraw, model)
                    out[key] = inst
                inst.names.add(norm_name(v[col[f"dev_{side}"]]))
                port = v[col[f"port_{side}"]]
                if port is not None and str(port).strip():
                    inst.ports.add(str(port).strip())
                inst.rows.append(row_nos[i])
        return out

    norm = build(True)
    raw = build(False)
    return InstanceInventory(
        normalized=tuple(norm.values()), raw=tuple(raw.values()), rows_total=len(rows)
    )


def find_u_conflicts(
    instances: Iterable[DeviceInstance],
) -> list[UConflict]:
    """同机柜内 U 区间相交即冲突。区间相交是**包含判断**，不是相等判断：

    `3-12U` 与 `3-4U` 相交 → 冲突。这是实测里最容易被漏掉的一类（整机占 10U）。
    """
    by_rack: dict[tuple[int, int], list[DeviceInstance]] = defaultdict(list)
    for i in instances:
        by_rack[i.rack_key].append(i)

    out: list[UConflict] = []
    for (ch, rk), items in by_rack.items():
        ordered = sorted(items, key=lambda x: (x.span[0], x.span[1]))
        for i in range(len(ordered)):
            for j in range(i + 1, len(ordered)):
                x, y = ordered[i], ordered[j]
                if x.span[0] > y.span[1]:
                    break  # 已按起点排序，后续不可能再相交
                if spans_overlap(x.span, y.span):
                    out.append(
                        UConflict(
                            channel_no=ch,
                            rack_no=rk,
                            a=x,
                            b=y,
                            is_name_variant=norm_name(x.model) == norm_name(y.model),
                        )
                    )
    out.sort(key=lambda c: (c.channel_no, c.rack_no, c.a.span[0], c.b.span[0]))
    return out


@dataclass(frozen=True, slots=True)
class SameNameMultiU:
    """同一个实例名被记在同机柜多个 U 位 —— 真缺陷（不是"同型号多台"）。"""

    channel_no: int
    rack_no: int
    name: str
    spans: tuple[tuple[int, int], ...]


def find_same_name_multi_u(
    instances: Iterable[DeviceInstance],
) -> list[SameNameMultiU]:
    """注意区分两件事：

    - 同**型号**占多个 U 位 = 正常（一个柜里 8 台同型号计算节点）
    - 同**实例名**占多个 U 位 = 缺陷（这台设备到底装哪？）
    """
    holder: dict[tuple, set[tuple[int, int]]] = defaultdict(set)
    for i in instances:
        for n in i.names:
            if n:
                holder[(i.channel_no, i.rack_no, n)].add(i.span)
    out = [
        SameNameMultiU(k[0], k[1], k[2], tuple(sorted(v)))
        for k, v in holder.items()
        if len(v) > 1
    ]
    out.sort(key=lambda x: (x.channel_no, x.rack_no, x.name))
    return out


def conflict_issues(
    *,
    conflicts_raw: Sequence[UConflict],
    conflicts_norm: Sequence[UConflict],
) -> tuple[Issue, ...]:
    """把两个口径的差异讲清楚 —— 报告里必须同时出现两个数字。"""
    issues: list[Issue] = []
    variant = [c for c in conflicts_raw if c.is_name_variant]
    for c in conflicts_norm:
        if c.is_name_variant:
            continue
        issues.append(
            Issue.warn(
                IssueCode.PORT_ALREADY_USED,
                f"{c.label}：{c.a.display_name}({c.a.u_raw}) 与 "
                f"{c.b.display_name}({c.b.u_raw}) U 位区间重叠，需人工裁决",
                channel=c.channel_no,
                rack=c.rack_no,
            )
        )
    for c in variant:
        issues.append(
            Issue.info(
                IssueCode.AMBIGUOUS_RACK,
                f"{c.label}：{c.a.model_raw}({c.a.u_raw}) 与 "
                f"{c.b.model_raw}({c.b.u_raw}) 是同一设备名称的空格变体，不是真冲突",
                channel=c.channel_no,
                rack=c.rack_no,
            )
        )
    return tuple(issues)


__all__ = [
    "DeviceInstance",
    "InstanceInventory",
    "SameNameMultiU",
    "UConflict",
    "conflict_issues",
    "extract_instances",
    "find_same_name_multi_u",
    "find_u_conflicts",
    "parse_u_span",
    "spans_overlap",
]
