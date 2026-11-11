"""源数据读取适配器：把 Excel 文件变成域层能吃的两个输入。

## 为什么拆成两个独立的小适配器而不是一个大 Reader

平面图和连线表是**两种完全不同的数据**：
- 平面图 → 一整块矩形网格，用来算几何快照（362 个机柜位）
- 连线表 → 5127 行 24 列，用来算长度、做体检

它们的失败模式也不同：平面图缺 sheet 是致命的（没有几何什么都算不了），
连线表缺 sheet 也是致命的，但**平面图里有空调位/柱子**是正常的。
拆开后调用方可以只用一个（比如"只重算几何不重读连线表"）。

## 为什么返回 audit 列式视图

体检规则要报「源值 vs 计算值」，所以必须拿到**未归一的原始值**。
`ImportPreview.links` 里的值是归一过的（`带外交换机 1` → `带外交换机1`），
若体检拿归一值去比对，就永远看不到"名称带空格"这类问题。

因此 `read_links()` 返回四元组：`(preview, audit_rows, audit_row_nos, audit_col)`。
前一个是给域层计算的，后三个是给体检的。**同一批行的两种视图，不是两份数据。**

## 为什么两个源要共享一个 `WorkbookSession`

各自 `load_workbook` 一次 = 打开两遍工作簿。实测常规模式每次 1979ms、
read_only 每次 180ms。共享会话后冷启动从 4.2s 降到 0.35s。

`XlsxProjectSource` 就是共享会话的载体：**一个源文件、一次打开、两个视图**。
两个旧适配器保留（单测和 CLI 里按需单独用），但完整流水线走 `XlsxProjectSource`。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Mapping, Sequence

from .openpyxl_reader import (
    FLOOR_SCAN_COLS,
    FLOOR_SCAN_ROWS,
    RawFloorCell,
    RawSheet,
    WorkbookSession,
)
from .v13_adapter import ImportPreview, adapt_sheet

__all__ = [
    "V13_AUDIT_COLUMNS",
    "XlsxCablingSource",
    "XlsxFloorSource",
    "XlsxProjectSource",
]

#: 体检规则的列名 → 24 列索引。**这是体检与源表之间的唯一契约**，
#: 加列/换版式只需改这里，规则函数一行不用动。
V13_AUDIT_COLUMNS: dict[str, int] = {
    "model_local": 0,
    "dev_local": 1,
    "ch_local": 2,
    "rack_local": 3,
    "u_local": 4,
    "port_local": 5,
    "model_peer": 6,
    "dev_peer": 7,
    "ch_peer": 8,
    "rack_peer": 9,
    "u_peer": 10,
    "port_peer": 11,
    "cable_type": 12,
    "n_cross": 13,
    "o_gap": 14,
    "p_count": 15,
    "q_in_rack": 16,
    "r_inter": 17,
    "s_total": 18,
    "t_ladder": 19,
    "u_approved": 20,
    "mod_local": 21,
    "mod_peer": 22,
    "remark": 23,
}


@dataclass(slots=True)
class XlsxProjectSource:
    """**完整流水线的推荐入口**：一个文件、一次打开、两个视图。

    实现 `application/ports.py` 的 `FloorSource` 与 `CablingSource` 两个协议，
    内部惰性执行一次读盘并把结果缓存 —— 所以即使调用方先 `read_floor_cells()`
    再 `read_links()`，工作簿也只会被打开一次。
    """

    path: Path
    floor_sheet: str = "机房平面图"
    cabling_sheet: str = "布线表"
    floor_rows: tuple[int, int] = FLOOR_SCAN_ROWS
    floor_cols: tuple[int, int] = FLOOR_SCAN_COLS

    _cabling: RawSheet | None = None
    _floor: list[RawFloorCell] | None = None

    def _ensure_loaded(self) -> None:
        if self._cabling is not None and self._floor is not None:
            return
        with WorkbookSession(self.path) as s:
            self._cabling = s.read_cabling_sheet(self.cabling_sheet)
            self._floor = s.read_floor_grid(
                self.floor_sheet,
                row_range=self.floor_rows,
                col_range=self.floor_cols,
            )

    # ------------------------------------------------------- FloorSource
    def read_floor_cells(
        self,
    ) -> tuple[list[tuple[int, int, object]], tuple[int, int], tuple[int, int]]:
        self._ensure_loaded()
        assert self._floor is not None
        return (
            [(c.row, c.col, c.value) for c in self._floor],
            self.floor_rows,
            self.floor_cols,
        )

    # ----------------------------------------------------- CablingSource
    def read_links(
        self,
    ) -> tuple[
        ImportPreview,
        Sequence[Sequence[object]],
        Sequence[int],
        Mapping[str, int],
    ]:
        self._ensure_loaded()
        assert self._cabling is not None
        preview = adapt_sheet(self._cabling)
        rows = [r.values for r in self._cabling.rows]
        row_nos = [r.row_no for r in self._cabling.rows]
        return preview, rows, row_nos, dict(V13_AUDIT_COLUMNS)


@dataclass(slots=True)
class XlsxFloorSource:
    """只读平面图。

    读的是原始单元格文本，不解释含义 —— `parse_floor` 才负责判断
    哪些是机柜号、哪些是「空调」「柱子」这类占位文字。

    只在"单独重算几何"场景使用；完整流水线请用 `XlsxProjectSource`。
    """

    path: Path
    sheet_name: str = "机房平面图"
    row_range: tuple[int, int] = FLOOR_SCAN_ROWS
    col_range: tuple[int, int] = FLOOR_SCAN_COLS

    def read_floor_cells(
        self,
    ) -> tuple[list[tuple[int, int, object]], tuple[int, int], tuple[int, int]]:
        with WorkbookSession(self.path) as s:
            cells = s.read_floor_grid(
                self.sheet_name, row_range=self.row_range, col_range=self.col_range
            )
        return (
            [(c.row, c.col, c.value) for c in cells],
            self.row_range,
            self.col_range,
        )


@dataclass(slots=True)
class XlsxCablingSource:
    """只读连线表。`read_links()` 返回四元组，见模块 docstring。

    只在"单独重读连线表"场景使用；完整流水线请用 `XlsxProjectSource`。
    """

    path: Path
    sheet_name: str = "布线表"

    def read_raw_sheet(self) -> RawSheet:
        with WorkbookSession(self.path) as s:
            return s.read_cabling_sheet(self.sheet_name)

    def read_links(
        self,
    ) -> tuple[
        ImportPreview,
        Sequence[Sequence[object]],
        Sequence[int],
        Mapping[str, int],
    ]:
        sheet = self.read_raw_sheet()
        preview = adapt_sheet(sheet)
        rows = [r.values for r in sheet.rows]
        row_nos = [r.row_no for r in sheet.rows]
        return preview, rows, row_nos, dict(V13_AUDIT_COLUMNS)
