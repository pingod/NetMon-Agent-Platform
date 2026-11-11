"""读任意版式的存量布线/连线表 —— 供跨版本对比用。

## 为什么单独一个模块

`openpyxl_reader.py` 是给**生成器与体检**用的：它硬编码 24 列表头
（`V13_HEADERS`）并按列索引取值。这个模块要读的是**任意历史版本**
（13 列 / 17 列 / 24 列 / 25 列，各版表头略有差别），
只能按表头文字认列。两者口径不同，混在一个文件里会互相牵制。

## 认 sheet 的规则（不靠用户指定）

实测六个版本里"有连线内容"的 sheet 名各不相同：
`连线表`（V8/V9）、`连线表(设计院填写长度)` + `连线表(不含长度)`（V11）、
`布线表` + `连线表(不含长度)`（V12/V13）、`布线表（旧）`（V13 存档）。

所以本模块按**表头特征**认，而不是按名字硬匹配：一个 sheet 的前几行
如果同时出现「本端设备」「对端设备」「本端接口」，它就是连线表。
名字只用来排序（`布线表` 优先于 `连线表`，`（旧）` 排最后）。

## 为什么"旧"要排最后而不是排除

`布线表（旧）` 是 V13 里的存档，正是它的上一版 —— 用户很可能就是想看
"这次改动前后的差别"，而那一刻前版就在同一个文件里。排除掉等于把
最方便的用法关掉。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Sequence

from ...domain.exporting.stockdiff import StockLayout, StockTable, build_table, detect_layout
from .openpyxl_reader import load_workbook

#: 判定"这是连线表"必须同时出现的表头片段。
CABLING_HEADER_MARKERS: tuple[str, ...] = ("本端设备", "对端设备", "本端接口")

#: sheet 排序权重。小的排前面（作为默认选择与列表顺序）。
#:
#: ## `（旧）` 必须排在同类之前判
#:
#: 实测 `布线表（旧）` 同时命中 `布线表`（权重 0）与 `（旧）`，取 min 会得 0 ——
#: 于是一份明确标了"旧"的表排在当前版本前面，成了默认选择。
#: 所以按**从上到下首个命中**取权重，而不是取最小。
#: 排序也体现「这份表值不值得比」：
#:
#: 实测 `连线表(不含长度)` 从 V8 到 V13 五个版本**逐行完全相同**（5110 行零差异），
#: 它是死文件。把它排在"当前布线表"旁边，用户第一次点开就选了它，
#: 得到"无差异"，会以为功能坏了。所以有长度的表（布线表 / 设计院填写长度）
#: 权重 0，无长度的连线表权重 3，`（旧）` 存档权重 9（可访问但排最后）。
SHEET_PRIORITY: tuple[tuple[str, int], ...] = (
    ("（旧）", 9),
    ("(旧)", 9),
    ("布线表", 0),
    ("连线表(设计院填写长度)", 0),
    ("连线表(不含长度)", 3),
    ("连线表", 3),
)

#: 读表头时最多扫多少行 —— 表头一定在第一行，但历史文件有合并单元格，
#: 第一行可能整行为空（实测 `11+12冷通道` 的连线表首行就是标题）。
HEADER_SCAN_ROWS = 3


@dataclass(frozen=True, slots=True)
class SheetInfo:
    """一个 sheet 的可读性画像。"""

    name: str
    rows: int
    cols: int
    is_cabling: bool
    headers: tuple[str, ...] = ()
    header_row: int = 1
    priority: int = 5

    @property
    def summary(self) -> str:
        kind = "连线表" if self.is_cabling else "非连线表"
        return f"{self.name}（{kind}，{self.rows} 行 × {self.cols} 列）"


def sheet_priority(name: str) -> int:
    """名字 → 排序权重。越像"当前版本"越小。

    首个命中即返回（顺序见 `SHEET_PRIORITY` 注释：`（旧）` 要先判）。
    """
    for needle, weight in SHEET_PRIORITY:
        if needle in name:
            return weight
    return 5


def _looks_like_cabling(headers: Sequence[Any]) -> bool:
    texts = {str(h).strip() for h in headers if h is not None}
    return all(marker in texts for marker in CABLING_HEADER_MARKERS)


def scan_workbook(path: str | Path) -> tuple[SheetInfo, ...]:
    """扫一个工作簿，按优先级返回全部 sheet 的画像。

    只读表头几行，不读数据 —— 用户在界面上选版本时要能立刻看到列表。
    """
    wb = load_workbook(path)
    try:
        out: list[SheetInfo] = []
        for name in wb.sheetnames:
            ws = wb[name]
            headers: tuple[str, ...] = ()
            header_row = 1
            for offset in range(HEADER_SCAN_ROWS):
                row = next(
                    ws.iter_rows(
                        min_row=1 + offset, max_row=1 + offset, values_only=True
                    ),
                    (),
                )
                texts = tuple(str(v).strip() for v in row if v is not None)
                if _looks_like_cabling(texts):
                    headers = texts
                    header_row = 1 + offset
                    break
            out.append(
                SheetInfo(
                    name=name,
                    rows=ws.max_row or 0,
                    cols=ws.max_column or 0,
                    is_cabling=bool(headers),
                    headers=headers,
                    header_row=header_row,
                    priority=sheet_priority(name),
                )
            )
        out.sort(key=lambda s: (not s.is_cabling, s.priority, s.name))
        return tuple(out)
    finally:
        wb.close()


def pick_cabling_sheets(path: str | Path) -> tuple[SheetInfo, ...]:
    """只返回连线表类的 sheet，按优先级排好。"""
    return tuple(s for s in scan_workbook(path) if s.is_cabling)


def read_stock_table(
    path: str | Path,
    *,
    sheet: str,
    label: str = "",
    header_row: int | None = None,
) -> StockTable:
    """读一个 sheet 成 `StockTable`。

    `header_row` 为 None 时自动探测（默认第 1 行；`11+12冷通道` 那种
    带大标题的文件表头在更下面）。
    """
    wb = load_workbook(path)
    try:
        if sheet not in wb.sheetnames:
            raise KeyError(
                f"工作簿里没有名为「{sheet}」的 sheet，实际有：{wb.sheetnames}"
            )
        ws = wb[sheet]
        row_no = header_row
        if row_no is None:
            for offset in range(HEADER_SCAN_ROWS):
                row = next(
                    ws.iter_rows(
                        min_row=1 + offset, max_row=1 + offset, values_only=True
                    ),
                    (),
                )
                texts = tuple(str(v).strip() for v in row if v is not None)
                if _looks_like_cabling(texts):
                    row_no = 1 + offset
                    break
            if row_no is None:
                row_no = 1
        header_values = next(
            ws.iter_rows(min_row=row_no, max_row=row_no, values_only=True), ()
        )
        data_rows = list(
            ws.iter_rows(min_row=row_no + 1, values_only=True)
        )
        # 尾部空行裁掉（read_only 的 max_row 常比真实数据多几行）
        while data_rows and not any(v not in (None, "") for v in data_rows[-1]):
            data_rows.pop()
        return build_table(
            label or f"{Path(path).stem} · {sheet}",
            header_values,
            data_rows,
            tag=sheet,
        )
    finally:
        wb.close()


def describe_layout(table: StockTable) -> StockLayout:
    """取出了哪几列 —— 供界面说明"这份表能不能比"。"""
    return table.layout


__all__ = [
    "CABLING_HEADER_MARKERS",
    "HEADER_SCAN_ROWS",
    "SHEET_PRIORITY",
    "SheetInfo",
    "describe_layout",
    "detect_layout",
    "pick_cabling_sheets",
    "read_stock_table",
    "scan_workbook",
    "sheet_priority",
]


class XlsxStockReader:
    """`StockReader` 端口的 openpyxl 实现。

    ## 为什么要有这个类（而不是让应用层直接调函数）

    应用层 `compare.py` 通过 `StockReader` Protocol 使用读表能力。
    这个类是它的具体实现，落在基础设施层 —— 依赖方向是
    基础设施 → 应用层定义的协议，不是应用层 → 基础设施。

    之前的写法是 `compare.py` 直接 `from ...infrastructure.xlsx.stock_reader
    import read_stock_table`，`lint-imports` 的第 4 条契约
    「应用层不得依赖基础设施（依赖倒置：靠 Protocol 注入）」直接报 BROKEN。

    做成类而不是一组函数还有个实际好处：将来支持 `.xls` / `.csv` 时，
    只需要再写一个实现类，`compare.py` 一行都不用改。
    """

    def scan(self, path: str) -> tuple[SheetInfo, ...]:
        return pick_cabling_sheets(path)

    def read(self, path: str, *, sheet: str, label: str = "") -> StockTable:
        return read_stock_table(path, sheet=sheet, label=label)


__all__ = [
    "CABLING_HEADER_MARKERS",
    "HEADER_SCAN_ROWS",
    "SHEET_PRIORITY",
    "SheetInfo",
    "XlsxStockReader",
    "describe_layout",
    "detect_layout",
    "pick_cabling_sheets",
    "read_stock_table",
    "scan_workbook",
    "sheet_priority",
]
