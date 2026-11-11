"""V13 布线表 25 列版式的读取器（基础设施层，允许出现 openpyxl）。

## 为什么读取器要「宽进」

真实数据有以下脏点，读取阶段**一律不许抛异常**：

- `T` 列有 2 行是文本 `待定`（r332/r333）
- `O` 列有 4 行填了 `25G-SR`（列错位）
- `P` 列有 4 行填了 `设备自带`
- `Q` 列有 sentinel `一` / `设备自带`
- 机柜列有的写 `1`、有的写 `rack1`；通道是数字
- 型号名有 4 个带空格（`业务接入交换机 1`），与不带空格的是同一型号

读取器只做「原样搬出来」，归一化与判定交给域层的导入组件。

## 为什么必须 `read_only=True` 且一个工作簿只开一次

`openpyxl.load_workbook()` 默认模式要把整个 sheet 解析成 `Cell` 对象树。
实测 V13（1.8MB）**常规模式 1979ms、read_only 模式 18ms**，差 100 倍。

原来的实现有两个独立适配器（平面图 / 连线表），各自 `load_workbook` 一次
→ 冷启动 4.2 秒。用户点开页面等 4 秒，会以为程序卡死。

`WorkbookSession` 解决两件事：
1. **只开一次**：两个 sheet 共用同一个已打开的 workbook
2. **只解析一次**：read_only 模式按行流式读，`iter_rows` 一次遍历拿到元组，
   不构造 `Cell` 对象

副作用是 `read_only` 的工作表**不支持随机 `cell()` 访问**（每次都要从头迭代），
所以本模块全部改用 `iter_rows(values_only=True)`。这是纯收益：
我们要的本来就是"整块搬出来"，不是"按坐标点查"。

## 为什么 session 要显式 close

read_only 模式保持文件句柄打开。Windows 上不 close 会导致文件无法被
Excel 重新打开（用户改完表保存会失败）—— 这是**离线单机部署**里最烦人的一类 bug，
不能靠 GC 兜底。
"""

from __future__ import annotations

import warnings
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator, Sequence

import openpyxl
from openpyxl.worksheet.worksheet import Worksheet

#: V13 布线表的 24 个有效表头（第 25 列表头为 None，仅 12 行有值，视作脏列）
V13_HEADERS: tuple[str, ...] = (
    "本端设备",
    "本端设备名称",
    "本端通道",
    "本端机柜",
    "U位",
    "本端接口",
    "对端设备",
    "对端设备名称",
    "对端通道",
    "对端机柜",
    "U位",
    "对端接口",
    "线缆类型",
    "跨机柜",
    "间隔机柜",
    "机柜数量",
    "机内线缆",
    "柜间线缆",
    "线缆2",
    "线缆长度1",
    "线缆长度",
    "本端模块",
    "对端模块",
    "备注1",
)

#: 平面图的行列扫描范围（覆盖 403 与 404 两个房间）
FLOOR_SCAN_ROWS: tuple[int, int] = (3, 16)
FLOOR_SCAN_COLS: tuple[int, int] = (3, 38)


@dataclass(slots=True)
class RawRow:
    """一行原始数据。`values` 与 `V13_HEADERS` 下标对齐。"""

    row_no: int
    values: tuple[object, ...]

    def get(self, header: str) -> object:
        try:
            idx = V13_HEADERS.index(header)
        except ValueError:
            return None
        return self.values[idx] if idx < len(self.values) else None

    def get_all(self, header: str) -> tuple[object, ...]:
        """同名列取全部（`U位` 在表里出现两次：本端与对端）。"""
        return tuple(v for h, v in zip(V13_HEADERS, self.values) if h == header)


@dataclass(slots=True)
class RawSheet:
    headers: tuple[object, ...]
    rows: list[RawRow]


@dataclass(slots=True)
class RawFloorCell:
    row: int
    col: int
    value: object


def load_workbook(path: str | Path, *, data_only: bool = True, read_only: bool = True):
    warnings.filterwarnings("ignore", category=UserWarning)
    return openpyxl.load_workbook(str(path), data_only=data_only, read_only=read_only)


class WorkbookSession:
    """一次打开、多次读 sheet 的会话。用完必须 `close()`。

    ## 为什么不用 contextmanager 装饰整条流水线

    因为 HTTP 缓存持有的是**解析结果**而不是会话：解析完就关，
    后续请求直接命中内存里的 `ProjectViews`。会话生命周期只有一次 `_load()`，
    显式 `with` 比隐式惰性关闭更不容易漏。
    """

    __slots__ = ("path", "_wb", "_closed")

    def __init__(self, path: str | Path) -> None:
        self.path = Path(path)
        self._wb = load_workbook(self.path)
        self._closed = False

    # ------------------------------------------------------------ 生命周期
    def close(self) -> None:
        if not self._closed:
            self._wb.close()
            self._closed = True

    def __enter__(self) -> WorkbookSession:
        return self

    def __exit__(self, *exc: object) -> None:
        self.close()

    @property
    def sheet_names(self) -> tuple[str, ...]:
        return tuple(self._wb.sheetnames)

    def _sheet(self, name: str) -> Worksheet:
        if name not in self._wb.sheetnames:
            raise KeyError(
                f"工作簿里没有名为「{name}」的 sheet，实际有：{self._wb.sheetnames}"
            )
        return self._wb[name]

    # ------------------------------------------------------------ 读取
    def read_cabling_sheet(self, sheet_name: str = "布线表") -> RawSheet:
        """读布线表。返回**全部**数据行，由调用方决定如何过滤。

        表头固定读前 24 列 —— 第 25 列在源文件里表头是 `None`，
        仅 12 行有值，是历史遗留脏列，不参与任何逻辑。
        """
        ws = self._sheet(sheet_name)
        n_cols = len(V13_HEADERS)
        rows: list[RawRow] = []
        headers: tuple[object, ...] = ()

        for idx, values in enumerate(
            ws.iter_rows(min_row=1, max_col=n_cols, values_only=True)
        ):
            if idx == 0:
                headers = tuple(values)
                continue
            # 行内全空也保留：导出要按原行号对齐，删掉中间空行会让后续行号错位
            rows.append(RawRow(idx + 1, tuple(values)))

        # 尾部连续空行必须裁掉。read_only 模式的 `max_row` 来自 xlsx 的
        # dimension 提示，实测比真实数据多 5 行（5128 → 5133）。
        # 不裁的话：总行数对不上、导出多出 5 个空行、与源文件 diff 不为空。
        while rows and all(v is None for v in rows[-1].values):
            rows.pop()

        return RawSheet(headers, rows)

    def read_floor_grid(
        self,
        sheet_name: str = "机房平面图",
        *,
        row_range: tuple[int, int] = FLOOR_SCAN_ROWS,
        col_range: tuple[int, int] = FLOOR_SCAN_COLS,
    ) -> list[RawFloorCell]:
        """把平面图矩形区域的单元格全量搬出来（含空调/柱子等占位文字）。"""
        ws = self._sheet(sheet_name)
        out: list[RawFloorCell] = []
        for r_off, values in enumerate(
            ws.iter_rows(
                min_row=row_range[0],
                max_row=row_range[1],
                min_col=col_range[0],
                max_col=col_range[1],
                values_only=True,
            )
        ):
            row_no = row_range[0] + r_off
            for c_off, v in enumerate(values):
                if v is None:
                    continue
                out.append(RawFloorCell(row_no, col_range[0] + c_off, v))
        return out

    def iter_sheets(self) -> Iterator[tuple[str, int, int]]:
        for name in self._wb.sheetnames:
            ws = self._wb[name]
            yield name, ws.max_row or 0, ws.max_column or 0


# ---------------------------------------------------------------- 兼容入口
# 单次调用的便捷函数：内部建会话、读完即关。
# 需要读多个 sheet 时请直接用 `WorkbookSession`，否则会重复打开工作簿。


def read_v13_cabling(path: str | Path, sheet_name: str = "布线表") -> RawSheet:
    with WorkbookSession(path) as s:
        return s.read_cabling_sheet(sheet_name)


def read_floor_cells(
    path: str | Path,
    sheet_name: str = "机房平面图",
    *,
    row_range: tuple[int, int] = FLOOR_SCAN_ROWS,
    col_range: tuple[int, int] = FLOOR_SCAN_COLS,
) -> list[RawFloorCell]:
    with WorkbookSession(path) as s:
        return s.read_floor_grid(
            sheet_name, row_range=row_range, col_range=col_range
        )


def iter_sheets(path: str | Path) -> Iterator[tuple[str, int, int]]:
    with WorkbookSession(path) as s:
        yield from s.iter_sheets()


def read_workbook_sheets(
    path: str | Path,
    *,
    cabling_sheet: str = "布线表",
    floor_sheet: str = "机房平面图",
    floor_rows: tuple[int, int] = FLOOR_SCAN_ROWS,
    floor_cols: tuple[int, int] = FLOOR_SCAN_COLS,
) -> tuple[RawSheet, list[RawFloorCell]]:
    """**推荐入口**：一个会话读完两个 sheet，只打开工作簿一次。"""
    with WorkbookSession(path) as s:
        cabling = s.read_cabling_sheet(cabling_sheet)
        floor = s.read_floor_grid(floor_sheet, row_range=floor_rows, col_range=floor_cols)
    return cabling, floor


__all__: Sequence[str] = (
    "FLOOR_SCAN_COLS",
    "FLOOR_SCAN_ROWS",
    "RawFloorCell",
    "RawRow",
    "RawSheet",
    "V13_HEADERS",
    "WorkbookSession",
    "iter_sheets",
    "load_workbook",
    "read_floor_cells",
    "read_v13_cabling",
    "read_workbook_sheets",
)
