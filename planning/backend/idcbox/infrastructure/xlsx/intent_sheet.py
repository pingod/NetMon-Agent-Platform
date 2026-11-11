"""接线意图表格（六段）的 Excel 读写（基础设施层，允许出现 openpyxl）。

## 为什么写在 infrastructure 而不是 domain

域层的 `ports/sheet.py` 只认「表名 → 行字典」这种纯数据结构，不碰文件。
openpyxl 是外部依赖，import-linter 的「L1 域层零 IO」契约禁止它进 domain。

## 为什么读的时候按表头名匹配，不按列位置

用户会在表里插列、调顺序、加备注列。按位置读的话，他插入一列「备注」，
后面所有列就整体错位，而程序**不会报错**，只会算出错误的结果 ——
这是最危险的一类 bug（静默错误）。

按表头名匹配的代价是表头必须写对。所以导出时会把表头写成
用户不能改的样子：加粗 + 底色 + 冻结首行，让「改表头」变成明显不划算的事。

## 为什么缺失的表要报名字

六张表里 `布局`、`交换机`、`路由`、`交换机组` 是生成连线的必要条件。
若用户把 sheet 名改成 `布局 `（带空格）就整段丢失，生成结果变成 0 行。
`read_intent_sheets()` 返回 `found` 与 `missing`，让调用方能明确告诉用户
「没找到这几张表，已按空处理」而不是静默产出空结果。

例外：`特殊设备` 与 `显式连线` 缺了是合法的（新项目还没有这些）。
"""

from __future__ import annotations

from pathlib import Path
from typing import Any, Iterable, Mapping, Sequence

from openpyxl import Workbook
from openpyxl.styles import Alignment, Border, Font, PatternFill, Side
from openpyxl.utils import get_column_letter

from ...domain.ports.intent import Intent
from ...domain.ports.sheet import SHEET_SPECS, intent_to_sheet, sheet_to_intent
from .openpyxl_reader import load_workbook

#: 必须存在的表。缺了会让生成结果不完整而不报错，所以导入时要主动提示。
REQUIRED_SHEETS: tuple[str, ...] = ("布局", "交换机", "路由", "交换机组")

#: 可选表。缺了是合法的。
OPTIONAL_SHEETS: tuple[str, ...] = ("特殊设备", "显式连线")

_HEADER_FILL = PatternFill("solid", fgColor="1F4E79")
_HEADER_FONT = Font(color="FFFFFF", bold=True, size=11)
_BODY_FONT = Font(size=10)
_CENTER = Alignment(horizontal="center", vertical="center")
_LEFT = Alignment(horizontal="left", vertical="center")
_THIN = Side(style="thin", color="BFBFBF")
_BORDER = Border(left=_THIN, right=_THIN, top=_THIN, bottom=_THIN)

#: 各列的建议宽度（按中文字符估算，窄的给 12，宽的给 40）。
_COL_WIDTHS: dict[str, int] = {
    "channel_no": 8,
    "rack_from": 10,
    "rack_to": 10,
    "node_model": 26,
    "slots": 14,
    "slot_map": 20,
    "instance": 26,
    "model": 24,
    "rack_no": 8,
    "u_start": 10,
    "u_span": 10,
    "nic": 12,
    "switch_model": 24,
    "instances": 52,
    "capacity": 12,
    "port_prefix": 14,
    "order": 12,
    "a_device": 26,
    "a_port": 16,
    "b_device": 26,
    "b_port": 16,
    "cable_type": 12,
}


#: 提示行前缀标记。读回时据此跳过，避免把提示文本当成一行数据。
#:
#: 为什么用固定前缀而不是「跳过第 2 行」：用户可能在表头下插入自己的行，
#: 也可能删掉提示行。按内容标记跳过，两种情况都不出错。
HINT_MARKER = "▸ "

def _note_row(ws: Any, text: str) -> None:
    """在表头下一行写提示语，灰色斜体，且**不占数据行**（数据从第 3 行起）。

    为什么要提示行：用户打开一张「起始机柜 / 结束机柜 / 槽位序列」的表，
    第一反应是"槽位序列怎么填？"。把格式示例放在他眼前比写在文档里有效。
    """
    ws.cell(row=2, column=1, value=HINT_MARKER + text).font = Font(
        size=9, italic=True, color="808080"
    )


#: 每张表的填写提示（写在表头下方灰色行）。
SHEET_HINTS: dict[str, str] = {
    "布局": "一行一个机柜段：某通道从起始机柜到结束机柜，全部放同一型号。"
    "槽位序列留空=标准 8 槽位(0-7)；机柜槽位覆盖写 7:0-6;11:0-7,7 这种形式",
    "交换机": "一行一台交换机。起始U位填数字(3 不填 3U)。框式设备占用U数填 10",
    "特殊设备": "一行一台有名字的非交换机设备（防火墙/负载均衡/GSO 等）。占用U数默认 2",
    "路由": "一行一条型号级规则：某型号服务器的某网口接某型号交换机。14 条能覆盖 3850 行",
    "交换机组": "一行一条链：顺序列写交换机实例名，逗号分隔，**顺序就是接线顺序**。"
    "每台端口数=该端口面的下行口数(不是整机最大端口号)",
    "显式连线": "一行一条手工连线。用于交换机互联与特殊机型。线缆类型留空则按两端端口派生",
}


def build_intent_workbook(intent: Intent) -> Workbook:
    """Intent → 六 sheet 的 workbook（纯内存，不落盘）。"""
    data = intent_to_sheet(intent)
    wb = Workbook()
    # 删掉默认 sheet，避免最后多一个空 sheet
    wb.remove(wb.active)

    for sheet_name, cols in SHEET_SPECS:
        ws = wb.create_sheet(sheet_name)
        for i, (key, label) in enumerate(cols, start=1):
            cell = ws.cell(row=1, column=i, value=label)
            cell.fill = _HEADER_FILL
            cell.font = _HEADER_FONT
            cell.alignment = _CENTER
            cell.border = _BORDER
            ws.column_dimensions[get_column_letter(i)].width = _COL_WIDTHS.get(key, 14)
        ws.row_dimensions[1].height = 24
        if sheet_name in SHEET_HINTS:
            _note_row(ws, SHEET_HINTS[sheet_name])
        for r, row in enumerate(data.get(sheet_name, ()), start=3):
            for i, (key, _label) in enumerate(cols, start=1):
                cell = ws.cell(row=r, column=i, value=row.get(key) or None)
                cell.font = _BODY_FONT
                cell.alignment = _LEFT if key in _WIDE_KEYS else _CENTER
                cell.border = _BORDER
        ws.freeze_panes = "A3"
        if data.get(sheet_name):
            last = get_column_letter(len(cols))
            ws.auto_filter.ref = f"A1:{last}{2 + len(data[sheet_name])}"

    return wb


#: 这些列内容长，左对齐更好读。
_WIDE_KEYS = frozenset(
    {"node_model", "model", "instance", "switch_model", "instances",
     "a_device", "b_device", "slot_map", "a_port", "b_port", "port_prefix"}
)


def write_intent_xlsx(intent: Intent, path: str | Path) -> None:
    """把意图写成六段 Excel。"""
    build_intent_workbook(intent).save(str(path))


def _sheet_rows(ws: Any, headers: Sequence[str]) -> list[dict[str, str]]:
    """按**表头名**把一张 sheet 读成行字典列表。

    `headers` 是期望的中文列名（来自 `SHEET_SPECS`）。
    返回的键是 `SHEET_SPECS` 里的列键，不是中文名。
    """
    rows = ws.iter_rows(values_only=True)
    try:
        title_row = next(rows)
    except StopIteration:
        return []
    # 表头可能不是第 1 行（用户可能加了标题行）：在前 5 行里找含有期望列名的那行
    title = [str(v).strip() if v is not None else "" for v in title_row]
    if not any(h in title for h in headers):
        found = False
        for _ in range(4):
            try:
                candidate = next(rows)
            except StopIteration:
                break
            cand = [str(v).strip() if v is not None else "" for v in candidate]
            if sum(1 for h in headers if h in cand) >= 2:
                title, found = cand, True
                break
        if not found:
            return []
    index = {h: i for i, h in enumerate(title) if h in headers}
    out: list[dict[str, str]] = []
    for values in rows:
        row = {
            h: (
                str(values[i]).strip()
                if i < len(values) and values[i] is not None
                else ""
            )
            for h, i in index.items()
        }
        first = row.get(headers[0], "") if headers else ""
        if first.startswith(HINT_MARKER):
            continue
        if any(row.values()):
            out.append(row)
    return out


def read_intent_workbook(
    path: str | Path,
    *,
    project: str = "",
    site: str = "",
) -> tuple[Intent, tuple[str, ...], tuple[str, ...]]:
    """读六段 Excel → `(Intent, found_sheets, missing_sheets)`。

    `project` 留空时用文件名（不含扩展名）—— 用户通常就叫「XX项目意图.xlsx」。
    """
    wb = load_workbook(path, data_only=True, read_only=True)
    try:
        available = set(wb.sheetnames)
        found = tuple(n for n, _ in SHEET_SPECS if n in available)
        missing = tuple(n for n, _ in SHEET_SPECS if n not in available)
        data: dict[str, list[dict[str, str]]] = {}
        # 中文表名 → 期望的中文列名
        label_of = {
            name: tuple(label for _key, label in cols) for name, cols in SHEET_SPECS
        }
        # 中文列名 → 列键
        key_of = {
            name: {label: key for key, label in cols} for name, cols in SHEET_SPECS
        }
        for name in found:
            raw = _sheet_rows(wb[name], label_of[name])
            data[name] = [
                {key_of[name][label]: value for label, value in row.items()}
                for row in raw
            ]
    finally:
        wb.close()

    effective_project = project or Path(path).stem
    intent = sheet_to_intent(data, project=effective_project, site=site)
    return intent, found, missing


__all__ = [
    "OPTIONAL_SHEETS",
    "REQUIRED_SHEETS",
    "SHEET_HINTS",
    "build_intent_workbook",
    "read_intent_workbook",
    "write_intent_xlsx",
]
