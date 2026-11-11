"""25 列导出契约（AC-1 / AC-3 的唯一判据）。

## 为什么单独抽出 `contract25.py`

导出是整个产品的**验收铁线**：用户拿到的文件必须能和他手上的 Excel 逐格对上。
所以 25 列的列名、列序、来源、形态必须是一份**声明式契约**，
而不是散落在导出代码里的 `if`。

## 三种单元格形态

| 形态 | 用途 | 例子 |
|---|---|---|
| `VALUE` | 直接写常量 | `本端设备`、`线缆类型` |
| `FORMULA` | 写可编辑公式（formula 模式） | `=N2+O2` |
| `RAW` | 原样文本，绝不改（sentinel） | `待定`、`一`、`设备自带` |

`RAW` 是**数据保真**的关键：`待定` 不能被转成空，`设备自带` 不能被转成 0。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Final, Literal

from ..cable_length.ladder import IFS_COMPAT, Rung

CellKind = Literal["value", "formula", "raw"]

#: 源文件列宽（Excel 字符宽度），来自 V13 实测
V13_COL_WIDTHS: Final[tuple[float, ...]] = (
    26.0, 47.0, 8.0, 8.0, 9.0, 16.0,
    26.0, 47.0, 8.0, 8.0, 9.0, 16.0,
    10.0, 8.0, 8.0, 8.0, 9.0, 9.0, 9.0, 10.0, 10.0,
    11.0, 11.0, 10.0,
)

#: 英文列名（A..X），供生成公式引用
COL_LETTERS: Final[tuple[str, ...]] = tuple("ABCDEFGHIJKLMNOPQRSTUVWX")


@dataclass(frozen=True, slots=True)
class ColumnSpec:
    """一列的完整定义。"""

    index: int
    header: str
    #: 域层字段名（None 表示无对应域字段，如第 25 个空表头列）
    field: str | None
    kind: CellKind
    #: 公式模板（kind == "formula" 时使用），`{r}` 会被替换成行号
    formula: str | None = None
    #: 是否允许为空
    nullable: bool = True
    width: float = 10.0
    note: str = ""


def _ifs_formula(r: int) -> str:
    """按 IFS_COMPAT 规则表**生成**等价的 IFS 公式文本。

    注意：不是硬编码字符串，而是从 `IFS_COMPAT` 反推 —— 这样规则改了公式自动跟着改，
    避免「代码里一套、导出里另一套」。
    """
    parts: list[str] = []
    prev_le: float | None = None
    for rung in IFS_COMPAT:
        if rung.op == "le":
            parts.append(f"S{r}<={_fmt(rung.limit)},{rung.out}")
            prev_le = rung.limit
        elif rung.op == "eq":
            parts.append(f"S{r}={_fmt(rung.limit)},{rung.out}")
        elif rung.op == "lt":
            parts.append(f"S{r}<{_fmt(rung.limit)},{rung.out}")
    return f"=IFS({','.join(parts)})"


def _fmt(x: float) -> str:
    """Excel 公式里的数字文本：整数不带小数点。"""
    return str(int(x)) if float(x).is_integer() else str(x)


#: 25 列契约（列序与 V13 完全一致）
CONTRACT_25: Final[tuple[ColumnSpec, ...]] = (
    ColumnSpec(0, "本端设备", "local_model", "value", width=26.0, note="设备型号名"),
    ColumnSpec(1, "本端设备名称", "local_device", "value", width=47.0, note="实例名，可能为空（实测 14 行）"),
    ColumnSpec(2, "本端通道", "local_channel", "value", width=8.0),
    ColumnSpec(3, "本端机柜", "local_rack", "value", width=8.0),
    ColumnSpec(4, "U位", "local_u", "raw", width=9.0, note="13 种字符串形态，必须原样"),
    ColumnSpec(5, "本端接口", "local_port", "value", width=16.0),
    ColumnSpec(6, "对端设备", "peer_model", "value", width=26.0),
    ColumnSpec(7, "对端设备名称", "peer_device", "value", width=47.0),
    ColumnSpec(8, "对端通道", "peer_channel", "value", width=8.0),
    ColumnSpec(9, "对端机柜", "peer_rack", "value", width=8.0),
    ColumnSpec(10, "U位", "peer_u", "raw", width=9.0, note="与第 5 列同名，导出必须保持"),
    ColumnSpec(11, "对端接口", "peer_port", "value", width=16.0),
    ColumnSpec(12, "线缆类型", "cable_type", "value", width=10.0, note="UNKNOWN 时保留原文"),
    ColumnSpec(13, "跨机柜", "n_cross_rack", "value", width=8.0),
    ColumnSpec(14, "间隔机柜", "o_gap_rack", "value", width=8.0),
    ColumnSpec(15, "机柜数量", "p_rack_count", "formula", "=N{r}+O{r}", width=8.0),
    ColumnSpec(16, "机内线缆", "q_in_rack_m", "value", width=9.0),
    ColumnSpec(17, "柜间线缆", "r_inter_rack_m", "formula", "=P{r}*0.6*120%", width=9.0),
    ColumnSpec(18, "线缆2", "s_total_m", "formula", "=R{r}+Q{r}", width=9.0),
    ColumnSpec(19, "线缆长度1", "t_ladder_m", "formula", None, width=10.0, note="逐字 IFS，含 (6,7) 缝隙"),
    ColumnSpec(20, "线缆长度", "u_approved_m", "raw", width=10.0, note="人工核定；待定 原样输出"),
    ColumnSpec(21, "本端模块", "local_module", "raw", width=11.0, note="默认可空，绝不强行生成"),
    ColumnSpec(22, "对端模块", "peer_module", "raw", width=11.0),
    ColumnSpec(23, "备注1", "remark", "raw", width=10.0),
)

#: 第 25 列：表头为 None，实测仅 12 行有值 → 视作脏列，导出不写
PHANTOM_COLUMN_INDEX: Final[int] = 24
PHANTOM_COLUMN_ROWS: Final[int] = 12

#: 表头行（第 1 行）
HEADER_ROW: Final[int] = 1
#: 数据起始行
FIRST_DATA_ROW: Final[int] = 2
#: 冻结窗格：冻结前 6 列 + 第 1 行（复刻源文件 freeze A2 的等价效果）
FREEZE_SPLIT_COLS: Final[int] = 6
FREEZE_SPLIT_ROWS: Final[int] = 1


def headers() -> tuple[str, ...]:
    """24 个表头文本（第 25 列无表头，不导出）。"""
    return tuple(c.header for c in CONTRACT_25)


def column_of(field: str) -> ColumnSpec | None:
    for c in CONTRACT_25:
        if c.field == field:
            return c
    return None


def t_formula_text(row: int) -> str:
    """生成第 `row` 行的 T 列 IFS 公式文本。"""
    return _ifs_formula(row)


def data_validation_lists() -> dict[int, str]:
    """需要加数据验证下拉的列 → 选项串（防脏数据回流）。"""
    return {
        column_of("cable_type").index: '"RJ45,LC-LC,MPO"',  # type: ignore[union-attr]
        column_of("local_module").index: '"一,设备自带,10G-SR,25G-SR,100G-SR4,光转电"',  # type: ignore[union-attr]
        column_of("peer_module").index: '"一,设备自带,10G-SR,25G-SR,100G-SR4,光转电"',  # type: ignore[union-attr]
    }


def rungs_summary() -> tuple[tuple[int, str, float, int], ...]:
    """规则表摘要（供 UI 展示「线长是怎么分档的」）。"""
    return tuple((r.seq, r.op, r.limit, r.out) for r in IFS_COMPAT)


__all__ = [
    "COL_LETTERS",
    "CONTRACT_25",
    "ColumnSpec",
    "CellKind",
    "FIRST_DATA_ROW",
    "FREEZE_SPLIT_COLS",
    "FREEZE_SPLIT_ROWS",
    "HEADER_ROW",
    "PHANTOM_COLUMN_INDEX",
    "PHANTOM_COLUMN_ROWS",
    "V13_COL_WIDTHS",
    "column_of",
    "data_validation_lists",
    "headers",
    "rungs_summary",
    "t_formula_text",
]
