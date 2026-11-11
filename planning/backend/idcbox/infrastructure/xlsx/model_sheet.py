"""设备型号库的 Excel 读写（基础设施层，允许出现 openpyxl）。

## 为什么版式是「两段表上下排」而不是两个 sheet

现场工程师维护型号库时是「照着设备面板抄一遍」：先在左表填型号名与类别，
再往下面那张表里一行一行加端口。两段放一个 sheet 里，他能一眼看到
「这个型号一共几行口」，也方便整块复制粘贴。

分成两个 sheet 的话，两次滚动之间会填错型号名 —— 而型号名填错的表现
是「新型号没有模板」（静默），不是报错。

## 读的时候按表头名匹配

跟 `intent_sheet.py` 同一个理由：用户会插列、调顺序、加备注。
按列位置读，插一列「备注」就整体错位且不报错 —— 最危险的一类 bug。

## 型号表允许「只有型号名、没有端口组」

这是**故意支持**的：先建档、后补端口。这样的型号会被加载器当成
「没有端口组，已跳过」并给一条提示，而不是让整个库加载失败。
"""

from __future__ import annotations

from pathlib import Path
from typing import Any, Sequence

from openpyxl import Workbook
from openpyxl.styles import Alignment, Border, Font, PatternFill, Side
from openpyxl.utils import get_column_letter

from ...domain.ports.modellib import (
    CATEGORY_VALUES,
    ROLE_VALUES,
    ModelLibrary,
    entry_from_rows,
)
from .openpyxl_reader import load_workbook

SHEET_NAME = "型号库"

#: 第一段（型号表）的表头。按名匹配，顺序可变。
MODEL_HEADERS: tuple[str, ...] = (
    "型号名",
    "类别",
    "U高",
    "规格原文",
    "停用",
)

#: 第二段（端口组）的表头。一行 = 一个口的一种速率。
PORT_HEADERS: tuple[str, ...] = (
    "型号名",
    "组名",
    "面板",
    "序号",
    "速率Mbps",
    "角色",
)

#: 第一段的结束标记 —— 空行 + 这一行就是分界。找不到就从没对齐的行开始第二段。
SECTION_MARK = "—— 以下为端口组 ——"

_CATEGORY_CN = {
    "core": "核心",
    "switch": "交换机",
    "security": "安全设备",
    "storage": "存储",
    "server": "服务器",
    "balancer": "负载均衡",
    "other": "其他",
}

_ROLE_CN = {
    "downlink": "下联",
    "uplink": "上联",
    "peer_link": "互联",
    "stack": "堆叠",
    "mgmt": "管理",
    "spare": "备用",
    "unknown": "未知",
}

_HEADER_FILL = PatternFill("solid", fgColor="FF1F4E79")
_HEADER_FONT = Font(name="微软雅黑", size=10, bold=True, color="FFFFFFFF")
_THIN = Side(style="thin", color="FFBFBFBF")
_BORDER = Border(left=_THIN, right=_THIN, top=_THIN, bottom=_THIN)


def _style_header(ws: Any, row: int, count: int) -> None:
    for col in range(1, count + 1):
        cell = ws.cell(row=row, column=col)
        cell.fill = _HEADER_FILL
        cell.font = _HEADER_FONT
        cell.alignment = Alignment(horizontal="center", vertical="center")
        cell.border = _BORDER


def write_model_sheet(lib: ModelLibrary, path: str | Path, *, only_kind: str | None = None) -> Path:
    """把型号库写成两段表的 Excel。

    `only_kind="json"` 时只写项目库自己声明的型号（避免把内置 26 条
    抄成用户数据）。导出模板时用 `only_kind=None`（默认导出全部）。
    """
    wb = Workbook()
    ws = wb.active
    ws.title = SHEET_NAME

    # 第一段：型号表
    for i, h in enumerate(MODEL_HEADERS, start=1):
        ws.cell(row=1, column=i, value=h)
    _style_header(ws, 1, len(MODEL_HEADERS))

    row = 2
    for spec in lib.all():
        key = spec.model
        src = lib.source_of(key)
        if only_kind and (src is None or src.kind != only_kind):
            continue
        ws.cell(row=row, column=1, value=spec.model)
        ws.cell(row=row, column=2, value=_CATEGORY_CN.get(spec.category, "其他"))
        ws.cell(row=row, column=3, value=spec.bays[0].nominal_mbps and 1.0)
        ws.cell(row=row, column=4, value=spec.source_text)
        row += 1

    row += 1
    ws.cell(row=row, column=1, value=SECTION_MARK).font = Font(
        name="微软雅黑", size=10, bold=True, color="FF7F7F7F"
    )
    row += 1
    header_row = row
    for i, h in enumerate(PORT_HEADERS, start=1):
        ws.cell(row=row, column=i, value=h)
    _style_header(ws, row, len(PORT_HEADERS))

    row += 1
    for spec in lib.all():
        src = lib.source_of(spec.model)
        if only_kind and (src is None or src.kind != only_kind):
            continue
        seen: dict[str, int] = {}
        for bay in spec.bays:
            if bay.panel:
                panel = bay.panel
                seen[bay.group] = max(seen.get(bay.group, 0), panel)
            else:
                panel = 1
                if bay.group.endswith("GE"):
                    panel = seen.get(bay.group, 0) + 1
                seen[bay.group] = panel
            for idx, speed in bay.pairs():
                ws.cell(row=row, column=1, value=spec.model)
                ws.cell(row=row, column=2, value=bay.group)
                ws.cell(row=row, column=3, value=panel)
                ws.cell(row=row, column=4, value=idx)
                ws.cell(row=row, column=5, value=speed)
                ws.cell(row=row, column=6, value=_ROLE_CN.get(bay.role, "未知"))
                row += 1

    widths = (28, 14, 10, 48, 8)
    for i, w in enumerate(widths, start=1):
        ws.column_dimensions[get_column_letter(i)].width = w
    for i, w in enumerate(widths, start=1):
        ws.column_dimensions[get_column_letter(i)].width = max(w, (28, 12, 8, 8, 12, 10)[i - 1])
    ws.freeze_panes = "A2"

    out = Path(path)
    out.parent.mkdir(parents=True, exist_ok=True)
    tmp = out.with_suffix(out.suffix + ".tmp")
    wb.save(tmp)
    tmp.replace(out)
    return out


def _header_map(values: Sequence[object]) -> dict[str, int]:
    """表头文字 → 列索引（1-based）。容忍空格与全角空格。"""
    out: dict[str, int] = {}
    for i, v in enumerate(values, start=1):
        if v is None:
            continue
        text = str(v).replace(" ", "").replace("\u3000", "").strip()
        if text and text not in out:
            out[text] = i
    return out


def _role_from_cn(value: object) -> str:
    text = str(value or "").strip()
    for key, cn in _ROLE_CN.items():
        if text == cn or text == key:
            return key
    return text.lower() if text else "unknown"


def _category_from_cn(value: object) -> str:
    text = str(value or "").strip()
    for key, cn in _CATEGORY_CN.items():
        if text == cn or text == key:
            return key
    lowered = text.lower()
    return lowered if lowered in CATEGORY_VALUES else "other"


def read_model_sheet(path: str | Path, *, sheet: str = SHEET_NAME) -> tuple[list[Any], tuple[str, ...]]:
    """读两段型号库。返回 (条目列表, 提示列表)。"""
    wb = load_workbook(path, data_only=True)
    if sheet not in wb.sheetnames:
        return [], (f"没有找到 sheet「{sheet}」（现有：{'、'.join(wb.sheetnames)}）",)
    ws = wb[sheet]

    def cell(r: int, c: int) -> object:
        if r < 1 or c < 1:
            return None
        return ws.cell(row=r, column=c).value

    max_row = ws.max_row or 0
    max_col = ws.max_column or 0

    model_map: dict[str, int] | None = None
    port_map: dict[str, int] | None = None
    model_start = 0
    port_start = 0
    for r in range(1, min(max_row, 30) + 1):
        values = [cell(r, c) for c in range(1, max_col + 1)]
        hdrs = _header_map(values)
        if model_map is None and "型号名" in hdrs and "类别" in hdrs:
            model_map = hdrs
            model_start = r + 1
            continue
        if model_map is not None and "型号名" in hdrs and "组名" in hdrs:
            port_map = hdrs
            port_start = r + 1
            break

    if model_map is None:
        return [], ("型号库缺少第一段表头（需要「型号名」「类别」两列）",)

    names: list[str] = []
    meta: dict[str, dict[str, Any]] = {}
    for r in range(model_start, max_row + 1):
        name = cell(r, model_map["型号名"])
        if name is None or not str(name).strip():
            continue
        text = str(name).strip()
        if text == SECTION_MARK or text.startswith("——"):
            break
        if port_map is None and "组名" in _header_map(
            [cell(r, c) for c in range(1, max_col + 1)]
        ):
            # 只有第一段没有第二段（用户只建了型号名）
            break
        names.append(text)
        meta[text] = {
            "category": _category_from_cn(cell(r, model_map["类别"])) if "类别" in model_map else "other",
            "source_text": (
                str(cell(r, model_map["规格原文"]) or "")
                if "规格原文" in model_map
                else ""
            ),
            "remove": bool(cell(r, model_map["停用"])) if "停用" in model_map else False,
        }

    groups: dict[str, list[tuple[str, int, int, int, str]]] = {n: [] for n in names}
    warnings: list[str] = []
    if port_map is not None:
        for r in range(port_start, max_row + 1):
            name = cell(r, port_map["型号名"])
            group = cell(r, port_map["组名"])
            if name is None or not str(name).strip() or group is None:
                continue
            name = str(name).strip()
            if name not in groups:
                warnings.append(f"第 {r} 行的型号「{name}」不在第一段型号表里，已跳过")
                continue
            seq = cell(r, port_map.get("序号", 0))
            speed = cell(r, port_map.get("速率Mbps", 0))
            if not isinstance(seq, int) or not isinstance(speed, int) or speed <= 0:
                warnings.append(f"第 {r} 行的序号/速率不是整数，已跳过")
                continue
            panel_raw = cell(r, port_map.get("面板", 0))
            panel = panel_raw if isinstance(panel_raw, int) and panel_raw > 0 else 0
            role = _role_from_cn(cell(r, port_map["角色"]) if "角色" in port_map else "")
            if role not in ROLE_VALUES:
                warnings.append(f"第 {r} 行的角色「{role}」不认识，已按 unknown 处理")
                role = "unknown"
            groups[name].append((str(group).strip(), panel, seq, speed, role))

    entries = [
        entry_from_rows(
            name,
            groups.get(name, []),
            category=meta[name]["category"],
            source_text=meta[name]["source_text"],
            remove=meta[name]["remove"],
        )
        for name in names
    ]
    return entries, tuple(warnings)


__all__ = [
    "MODEL_HEADERS",
    "PORT_HEADERS",
    "SHEET_NAME",
    "read_model_sheet",
    "write_model_sheet",
]
