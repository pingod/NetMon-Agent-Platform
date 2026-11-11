"""25 列布线表的 xlsx 写出（openpyxl）。

## 双模式（架构 §6.2）

| 模式 | P/R/S/T | 用途 |
|---|---|---|
| `formula` | 写可编辑公式，Excel 打开自行计算 | 交付设计院二次编辑 |
| `value` | 写常量值 | 归档、跨系统交换、审计 |

## 保真优先级（这是验收铁线的实现细节）

导出不是「把域层对象打印出来」，而是 **「原值优先 + 计算值回填」**：

1. 原始单元格有值且该列属于 `raw` 形态（U位/线缆长度/模块/备注）→ **原样写回**
2. 原始单元格有值且属于 `value` 形态 → 原样写回（不做格式化、不做 trim）
3. 原始为空但本次算出了值（新建项目场景）→ 写计算值
4. 原始是 sentinel 文本（`待定`/`一`/`设备自带`）→ **原样写回，绝不转成空或 0**

第 4 条是数据保真最容易被写错的地方：`待定` 不是「没值」，是「有个值叫待定」。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Iterable, Literal, Mapping, Sequence

from openpyxl import Workbook
from openpyxl.styles import Alignment, Border, Font, PatternFill, Side
from openpyxl.utils import get_column_letter

from ...domain.exporting.contract25 import (
    CONTRACT_25,
    FIRST_DATA_ROW,
    FREEZE_SPLIT_COLS,
    FREEZE_SPLIT_ROWS,
    HEADER_ROW,
    V13_COL_WIDTHS,
    data_validation_lists,
    headers,
    t_formula_text,
)

ExportMode = Literal["formula", "value"]

#: 表头样式（复刻源文件观感：深蓝底、白字、加粗、居中、细边框）
_HEADER_FILL = PatternFill("solid", fgColor="FF1F4E79")
_HEADER_FONT = Font(name="微软雅黑", size=10, bold=True, color="FFFFFFFF")
_BODY_FONT = Font(name="微软雅黑", size=10)
_THIN = Side(style="thin", color="FFBFBFBF")
_BORDER = Border(left=_THIN, right=_THIN, top=_THIN, bottom=_THIN)
_CENTER = Alignment(horizontal="center", vertical="center", wrap_text=True)
_LEFT = Alignment(horizontal="left", vertical="center")


@dataclass(frozen=True, slots=True)
class ExportRow:
    """导出一行所需的一切。`raw` 是导入时的原始单元格值（0-based 列索引）。

    ## 关键语义：`raw` 里出现某列 = 该列源值权威

    这解决了一个容易做错的地方：**"源值是空" 和 "没有源值" 是两件事**。

    - 行是从用户的 Excel 导入的 → `raw` 覆盖全部 24 列。某列源值为空，
      导出就必须写空 —— 用户没填，我们不替他填（那会凭空改变交付物）。
    - 行是系统新生成的（没有对应的源行）→ `raw` 为 None。
      此时才用 `values` 里的计算值填充。

    如果只判断 `_is_blank(raw)`，两种情形无法区分，导入保真会立刻破功：
    5113 行里 4477 行源表没填 N/O/P/Q/R/S，一旦回填计算值，
    导出文件就与用户手里的文件不是同一份了。
    """

    source_row_no: int | None
    values: Mapping[str, Any]
    raw: Mapping[int, Any] | None = None

    def raw_of(self, col_index: int) -> Any:
        if not self.raw:
            return None
        return self.raw.get(col_index)

    def knows(self, col_index: int) -> bool:
        """这一列的源值是否权威（即：该行来自导入，且我们知道它原来的样子）。"""
        return self.raw is not None and col_index in self.raw


def _is_blank(v: object) -> bool:
    return v is None or (isinstance(v, str) and v.strip() == "")


def build_workbook(
    rows: Sequence[ExportRow],
    *,
    mode: ExportMode = "formula",
    sheet_name: str = "布线表",
) -> Workbook:
    """生成 workbook。纯内存操作，不落盘（便于测试与流式响应）。"""
    wb = Workbook()
    ws = wb.active
    ws.title = sheet_name

    # ---- 表头 ----
    hs = headers()
    for i, h in enumerate(hs, start=1):
        cell = ws.cell(row=HEADER_ROW, column=i, value=h)
        cell.fill = _HEADER_FILL
        cell.font = _HEADER_FONT
        cell.alignment = _CENTER
        cell.border = _BORDER
    ws.row_dimensions[HEADER_ROW].height = 28

    # ---- 列宽 ----
    for i, w in enumerate(V13_COL_WIDTHS, start=1):
        ws.column_dimensions[get_column_letter(i)].width = w

    # ---- 数据 ----
    for offset, row in enumerate(rows):
        r = FIRST_DATA_ROW + offset
        for spec in CONTRACT_25:
            cell = ws.cell(row=r, column=spec.index + 1)

            # 1) 公式列（仅 formula 模式）
            if spec.kind == "formula" and mode == "formula":
                if spec.field == "t_ladder_m":
                    # T 列：原始是 sentinel（待定）时绝不写公式，否则会把人工判断覆盖掉
                    raw_u = row.raw_of(spec.index)
                    if isinstance(raw_u, str) and raw_u.strip() and not raw_u.strip().isdigit():
                        cell.value = raw_u
                        cell.alignment = _CENTER
                        cell.font = _BODY_FONT
                        continue
                    cell.value = t_formula_text(r)
                elif spec.formula:
                    cell.value = spec.formula.replace("{r}", str(r))
                cell.alignment = _CENTER
                cell.font = _BODY_FONT
                continue

            # 2) 源值权威列：原样写回（含写成空 —— 用户没填就不替他填）
            if row.knows(spec.index):
                raw = row.raw_of(spec.index)
                if not _is_blank(raw):
                    cell.value = raw
                # 源值为空：什么都不写，单元格保持空白
            else:
                v = row.values.get(spec.field) if spec.field else None
                if not _is_blank(v):
                    cell.value = v

            cell.alignment = _CENTER if spec.kind != "value" or spec.index in (2, 3, 8, 9) else _LEFT
            cell.font = _BODY_FONT

    # ---- 冻结窗格 ----
    ws.freeze_panes = ws.cell(
        row=FREEZE_SPLIT_ROWS + 1, column=FREEZE_SPLIT_COLS + 1
    ).coordinate

    # ---- 数据验证下拉（防脏数据回流）----
    for col_index, options in data_validation_lists().items():
        letter = get_column_letter(col_index + 1)
        ws.add_data_validation(_make_validation(f"{letter}2:{letter}65535", options))

    # ---- 自动筛选 ----
    last_col = get_column_letter(len(CONTRACT_25))
    ws.auto_filter.ref = f"A{HEADER_ROW}:{last_col}{HEADER_ROW + len(rows)}"

    return wb


def _make_validation(cell_range: str, options: str) -> Any:
    from openpyxl.worksheet.datavalidation import DataValidation

    dv = DataValidation(type="list", formula1=options, allow_blank=True, showDropDown=False)
    dv.add(cell_range)
    return dv


def write_xlsx(
    rows: Sequence[ExportRow],
    path: str | Path,
    *,
    mode: ExportMode = "formula",
    sheet_name: str = "布线表",
) -> Path:
    """写出到文件（原子写：先写临时文件再替换，避免半个文件）。"""
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    tmp = p.with_suffix(p.suffix + ".tmp")
    wb = build_workbook(rows, mode=mode, sheet_name=sheet_name)
    wb.save(tmp)
    tmp.replace(p)
    return p


__all__ = ["ExportMode", "ExportRow", "build_workbook", "write_xlsx"]
