"""从 Excel / TSV 剪贴板解析出表格块 —— 「Excel 化交互层」的纯数据部分。

## 为什么这件事非做不可

用户的原始工作方式就是 Excel。949 行意图听起来不多，但**没人愿意在网页上
一格一格敲 949 个格子**。真实动作永远是：在 Excel 里排好一片，框选、Ctrl+C、
切到浏览器、Ctrl+V。

AG Grid Community 的剪贴板粘贴是 Enterprise 功能（$999/dev），
而本项目离线部署、不给客户再加一笔授权成本。所以自己实现 ——
好在「粘贴二维文本到表格」这件事本身不难，难的是**粘贴到哪、冲突怎么办**。

## 三个必须处理的现实

### 1. 剪贴板格式是「制表符分列、换行分行」，但来源五花八门

- Excel / WPS 单元格内换行 → 内容被双引号包起来（`"a\nb"`）
- 从网页/PDF 复制 → 可能有 `\r\n`、尾随空格、全角空格
- 末尾常有空行（Excel 复制时最后一行带了换行符）

解析器必须容忍这些，否则「明明粘的是 5 行，程序说 7 行」。

### 2. 粘贴必须能落在「任意位置」，不只表格末尾

用户的真实诉求是「我在这 44 行里改第 12~20 行」或者「从第 13 行开始插 8 行」。
只支持「追加到末尾」的话，他还是得回 Excel。

### 3. 粘进来的值必须走同一条类型转换与校验

不能因为「是粘贴的」就跳过 `coerce_cell`，否则界面上手打的 `3` 是整数、
粘进来的 `3` 是字符串 —— 生成时一处崩、一处过。这是「两套规则迟早不一致」
的同一个坑。

## 刻意不做的事

- **不做公式求值**：从 Excel 粘 `=A1+B1` 时按文本存，并报一条提示。
  意图表里本来就不该有公式（坐标是人填的），静默算出一个值更危险。
- **不做合并单元格展开**：Excel 的合并单元格在剪贴板里表现为「只有左上角有值、
  其余空」。展开需要知道合并区域，而剪贴板不携带这个信息。
  空着就空着 —— 校验会报「缺少字段」，比瞎猜安全。
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any, Mapping, Sequence

#: 单元格之间可能出现的分隔符。制表符是标准，但有些工具用多个空格对齐。
_COL_SEP = "\t"

#: Excel 的 CSV/TSV 引号规则：字段里有分隔符或换行时整体被双引号包住，
#: 内部的引号写成两个。这里是 TSV 语境，主要处理换行与制表符。
_QUOTED_RE = re.compile(r'"((?:[^"]|"")*)"', re.DOTALL)


@dataclass(frozen=True, slots=True)
class PasteBlock:
    """解析出来的二维块。

    `rows` 保持原样的字符串，不做任何类型转换 —— 转换是 `coerce_cell` 的事，
    两者分开才能单测各自的语义。
    """

    rows: tuple[tuple[str, ...], ...]
    #: 每行的列数与首行不一致时给出提示（Excel 里选歪了很常见）
    ragged: bool = False
    #: 检测到的最大列数，用于提示「你选了 6 列，但这张表有 5 列」
    width: int = 0

    @property
    def height(self) -> int:
        return len(self.rows)

    @property
    def is_empty(self) -> bool:
        return not any(cell.strip() for row in self.rows for cell in row)


def parse_clipboard(text: str) -> PasteBlock:
    """把剪贴板文本解析成二维块。

    ## 引号处理为什么需要

    Excel 里一个单元格内按 Alt+Enter 换行时，复制出来是 `"第一行\n第二行"`。
    如果不认引号，会被当成两行 —— 而意图里**没有**允许换行的字段，
    所以这两种解释的结果完全不同：
    - 认引号：1 行 1 列，值是含换行的文本（随后校验报「型号名里有换行」）
    - 不认引号：2 行，用户以为粘了 2 行数据

    认引号更接近用户意图。
    """
    if not text:
        return PasteBlock(rows=(), width=0)

    # 统一换行符。Windows 剪贴板是 \r\n，有时候还有孤立的 \r（老 Mac）
    normalized = text.replace("\r\n", "\n").replace("\r", "\n")
    # 去掉末尾多余的换行（Excel 复制时最后一行总带一个）
    normalized = normalized.rstrip("\n")

    rows: list[tuple[str, ...]] = []
    for line in _split_respecting_quotes(normalized):
        rows.append(_split_row(line))

    # 全空块没有意义：粘贴空内容不该产生空行
    if not any(cell.strip() for row in rows for cell in row):
        return PasteBlock(rows=(), width=0)

    widths = {len(r) for r in rows}
    width = max(widths) if widths else 0
    return PasteBlock(rows=tuple(rows), ragged=len(widths) > 1, width=width)


def _split_respecting_quotes(text: str) -> list[str]:
    """按换行切行，但双引号内的换行不切。"""
    out: list[str] = []
    buf: list[str] = []
    in_quote = False
    i = 0
    n = len(text)
    while i < n:
        ch = text[i]
        if ch == '"':
            # 连续两个引号是转义，不切换状态
            if in_quote and i + 1 < n and text[i + 1] == '"':
                buf.append('""')
                i += 2
                continue
            in_quote = not in_quote
            buf.append(ch)
        elif ch == "\n" and not in_quote:
            out.append("".join(buf))
            buf = []
        else:
            buf.append(ch)
        i += 1
    out.append("".join(buf))
    return out


def _split_row(line: str) -> tuple[str, ...]:
    """按制表符切列，双引号内的制表符不切，并剥掉包裹用的引号。"""
    cells: list[str] = []
    buf: list[str] = []
    in_quote = False
    i = 0
    n = len(line)
    while i < n:
        ch = line[i]
        if ch == '"':
            if in_quote and i + 1 < n and line[i + 1] == '"':
                buf.append('"')
                i += 2
                continue
            in_quote = not in_quote
            # 引号本身不进入值；但只有「整格被引号包住」时才应剥掉。
            # 简化处理：引号一律不进入值 —— 意图表里没有需要在值里保留
            # 双引号的字段（设备名/端口名/型号名都不含引号）。
        elif ch == _COL_SEP and not in_quote:
            cells.append("".join(buf))
            buf = []
        else:
            buf.append(ch)
        i += 1
    cells.append("".join(buf))
    return tuple(cells)


@dataclass(frozen=True, slots=True)
class PastePlan:
    """一次粘贴要做什么。**先算计划、再落盘** —— 用户得能看见「要改 8 行」。"""

    #: 目标起始行号（0-based）。`None` 表示追加到末尾。
    start_row: int | None
    #: 要写入的值，已按列顺序对齐到 `columns`
    values: tuple[dict[str, str], ...]
    #: 超出表格范围的列（用户多选了几列）
    extra_columns: int
    #: 覆盖 vs 插入
    mode: str

    @property
    def count(self) -> int:
        return len(self.values)

    @property
    def end_row(self) -> int | None:
        if self.start_row is None:
            return None
        return self.start_row + self.count - 1


def plan_paste(
    block: PasteBlock,
    columns: Sequence[str],
    *,
    start_row: int | None = None,
    mode: str = "overwrite",
) -> PastePlan:
    """把二维块映射成「第几列 = 哪个字段」。

    ## 为什么列按顺序对齐，不按表头匹配

    意图六段表格的列顺序是固定的（`intentcheck.COLUMN_TYPES` 与前端
    `SECTIONS` 同一套），用户从 Excel 粘过来时习惯「第一列对第一列」。
    按表头文字匹配反而脆弱：Excel 里的表头可能写成「机型」「机柜号」，
    而这里认的是 `model`/`rack`。

    代价是：如果用户从别处粘一列宽度不同的数据，会错位。
    所以 `extra_columns > 0` 时要提示，`block.ragged` 时也要提示。

    ## 为什么 `mode="insert"` 也要支持

    「在 44 行中间插 8 行」是真实动作（新增一组交换机、中间补几条路由）。
    只支持覆盖的话用户得先手动插空行。
    """
    if mode not in ("overwrite", "insert"):
        raise ValueError(f"不认识粘贴模式：{mode!r}")
    if not column_names_ok(columns):
        raise ValueError("列名不能为空")

    ncols = len(columns)
    out: list[dict[str, str]] = []
    extra = 0
    for raw_row in block.rows:
        extra = max(extra, max(0, len(raw_row) - ncols))
        item: dict[str, str] = {}
        for idx, column in enumerate(columns):
            item[column] = raw_row[idx] if idx < len(raw_row) else ""
        out.append(item)
    return PastePlan(
        start_row=start_row,
        values=tuple(out),
        extra_columns=extra,
        mode=mode,
    )


def column_names_ok(columns: Sequence[str]) -> bool:
    return bool(columns) and all(isinstance(c, str) and c.strip() for c in columns)


def apply_paste(
    doc: dict[str, Any],
    section: str,
    plan: PastePlan,
    *,
    coerce: Any,
) -> tuple[int, list[str]]:
    """把计划写进文档。返回 `(写入行数, 提示信息列表)`。

    `coerce` 是 `intentcheck.coerce_cell` 的注入点 —— 域层之间不互相 import
    具体实现，调用方传进来。这样粘贴逻辑可以脱离校验器单测。

    ## 空值与「不改」的区别

    粘贴区域里某格是空的，意味着「用户想把这格清空」，不是「保持原值」。
    所以空格会写入 `None`，而不是跳过。唯一的例外是 `None` 本身无法表示的
    字段（目前没有）—— 意图里所有字段都允许 `None`。

    ## 出错的格怎么处理

    类型转换失败时**该格留原值**、其余格照写，并把错误收集成提示。
    不做「一格错则整块回滚」：用户粘 50 行，其中有 1 格打错，
    回滚会让他丢了另外 49 行的成果，而提示能让他只改那一格。
    """
    items = doc.get(section)
    if not isinstance(items, list):
        items = []
        doc[section] = items

    notes: list[str] = []
    if plan.mode == "insert":
        at = len(items) if plan.start_row is None else max(0, min(plan.start_row, len(items)))
        new_rows: list[Any] = []
        for i, values in enumerate(plan.values):
            row: dict[str, Any] = {}
            for column, raw in values.items():
                value, err = coerce(section, column, raw)
                if err:
                    notes.append(f"第 {i + 1} 行 {column}：{err}")
                    continue
                row[column] = value
            new_rows.append(row)
        items[at:at] = new_rows
    else:
        start = len(items) if plan.start_row is None else plan.start_row
        # 先把目标区间补齐到够长，再逐行写。
        # 不能在循环里边写边 append —— 那样 `items[idx]` 的 idx 与
        # 实际长度不同步，起点超过末尾时会 IndexError。
        need = start + len(plan.values)
        while len(items) < need:
            items.append({})
        written = 0
        for i, values in enumerate(plan.values):
            idx = start + i
            row = items[idx]
            if not isinstance(row, dict):
                notes.append(f"第 {idx + 1} 行不是对象，已跳过")
                continue
            for column, raw in values.items():
                value, err = coerce(section, column, raw)
                if err:
                    notes.append(f"第 {idx + 1} 行 {column}：{err}")
                    continue
                row[column] = value
            written += 1
        if plan.extra_columns:
            notes.append(
                f"粘贴块比表格多 {plan.extra_columns} 列，多出的数据被丢弃"
            )
        return written, notes

    if plan.extra_columns:
        notes.append(f"粘贴块比表格多 {plan.extra_columns} 列，多出的数据被丢弃")
    return len(plan.values), notes


#: Excel 里以 `=` 开头的是公式。意图表不该有公式，粘进来要提示而不是求值。
def looks_like_formula(text: str) -> bool:
    return text.startswith("=") and len(text) > 1




# ---------------------------------------------------------------------------
# 区域序列化（复制出）
# ---------------------------------------------------------------------------


@dataclass(frozen=True, slots=True)
class CellRange:
    """表格里的一个矩形区域，行列都是 0-based 闭区间。

    ## 为什么区域要能跨列不跨段

    意图六段表格每段的列数是固定的（布局 4 列、交换机 6 列……）。
    框选只在一段内部发生，不存在跨段的区域 —— 跨段粘贴没有语义
    （布局的行怎么能贴进交换机表里）。
    """

    row_start: int
    row_end: int
    col_start: int
    col_end: int

    def __post_init__(self) -> None:
        if self.row_start > self.row_end or self.col_start > self.col_end:
            raise ValueError(
                f"区域坐标反了：行 {self.row_start}~{self.row_end}、"
                f"列 {self.col_start}~{self.col_end}"
            )
        if self.row_start < 0 or self.col_start < 0:
            raise ValueError("区域坐标不能为负")

    @property
    def height(self) -> int:
        return self.row_end - self.row_start + 1

    @property
    def width(self) -> int:
        return self.col_end - self.col_start + 1

    @property
    def cell_count(self) -> int:
        return self.height * self.width

    def contains(self, row: int, col: int) -> bool:
        return (
            self.row_start <= row <= self.row_end
            and self.col_start <= col <= self.col_end
        )

    def describe(self) -> str:
        if self.height == 1 and self.width == 1:
            return f"第 {self.row_start + 1} 行 1 格"
        if self.width == 1:
            return f"第 {self.row_start + 1}~{self.row_end + 1} 行"
        return (
            f"第 {self.row_start + 1}~{self.row_end + 1} 行 × "
            f"第 {self.col_start + 1}~{self.col_end + 1} 列"
        )


def format_for_clipboard(
    rows: Sequence[Mapping[str, Any]],
    columns: Sequence[str],
    cell_range: CellRange,
) -> str:
    """把一块区域序列化成制表符分隔的文本，写进剪贴板。

    ## 为什么按「列顺序」而不是「字段名」输出

    粘回 Excel 时用户期望「和表格里看到的一样」。跳过表头直接给数据行，
    是因为 Excel 里通常已经有表头了，再贴一行表头会错位。

    ## 为什么要给 `\r\n`

    Excel 在 Linux/macOS 上认 `\n`，但在 Windows 上必须 `\r\n`，
    否则整块粘进**一个**单元格。这是实测过的：只给 `\n` 时 WPS/Excel
    Windows 版会把多行合并成一行。用 `\r\n` 两者都认。

    ## 值里的制表符与换行怎么办

    意图字段理论上不该有，但设备名可能带。含制表符/换行/引号时按 Excel
    规则整体加双引号、内部引号翻倍 —— 与 `parse_clipboard` 对称，
    这样「复制出去再粘回来」是恒等的。
    """
    lines: list[str] = []
    for row_index in range(cell_range.row_start, cell_range.row_end + 1):
        row = rows[row_index] if 0 <= row_index < len(rows) else {}
        cells: list[str] = []
        for col_index in range(cell_range.col_start, cell_range.col_end + 1):
            column = columns[col_index] if col_index < len(columns) else ""
            value = row.get(column) if isinstance(row, Mapping) and column else None
            cells.append(_escape_cell("" if value is None else str(value)))
        lines.append("\t".join(cells))
    return "\r\n".join(lines)


def _escape_cell(text: str) -> str:
    """按 Excel 的规则转义一格。"""
    if any(ch in text for ch in ("\t", "\n", "\r", '"')):
        return '"' + text.replace('"', '""') + '"'
    return text


# ---------------------------------------------------------------------------
# 填充柄（拖拽把一个值/一串值延伸下去）
# ---------------------------------------------------------------------------


@dataclass(frozen=True, slots=True)
class FillPlan:
    """一次填充要写什么。与 `PastePlan` 分开是因为语义不同。

    ## 为什么要和粘贴分开

    粘贴是「用户明确给了 N 行数据」，填充是「用户给了 1~2 行样例，让程序
    按规律延伸」。前者照抄，后者要推断 —— 混在一起会让两边的校验逻辑
    互相打结（粘贴不该推断，填充不该照抄空值）。
    """

    #: 目标区间（含源区域本身）。只写源区之后的行
    target: CellRange
    #: 源区域的最后一行（0-based）。写入从它的下一行开始 ——
    #: 用它而不是 `target.row_start`：源区可能是 3 行（`row_start..row_start+2`），
    #: 拿 `row_start + 1` 当起点会把第 2 行覆盖掉。
    source_row_end: int
    #: 要写入的值，按行从上到下
    values: tuple[dict[str, Any], ...]
    #: 推断出的规律，给用户看的（"递增 +1" / "照抄" / "序列 weekday"）
    pattern: str

    @property
    def count(self) -> int:
        return len(self.values)


#: 能识别的文本序列。命中则按序列循环，否则照抄。
#: 只放最常用的几个 —— 猜太多反而让人意外（Excel 也不会把 "abc" 变 "abd"）。
TEXT_SEQUENCES: tuple[tuple[str, ...], ...] = (
    ("MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN"),
    ("星期一", "星期二", "星期三", "星期四", "星期五", "星期六", "星期日"),
    ("周一", "周二", "周三", "周四", "周五", "周六", "周日"),
)


def _infer_numeric(values: Sequence[Any]) -> tuple[str, Any]:
    """从 1~2 个数字里推断步长。返回 `(规律描述, 步长)`。"""
    nums = [v for v in values if isinstance(v, (int, float)) and not isinstance(v, bool)]
    if len(nums) != len(values) or not nums:
        return "", None
    if len(nums) == 1:
        return "照抄", 0
    step = nums[-1] - nums[-2]
    for i in range(1, len(nums)):
        if nums[i] - nums[i - 1] != step:
            return "", None
    if step == 0:
        return "照抄", 0
    return f"递增 {step:+g}", step


def _infer_text_sequence(values: Sequence[Any]) -> tuple[str, tuple[str, ...] | None, int]:
    """识别内置文本序列。返回 `(描述, 序列, 起点下标)`。"""
    if not values or not all(isinstance(v, str) for v in values):
        return "", None, 0
    texts = [str(v) for v in values]
    for seq in TEXT_SEQUENCES:
        upper = [s.upper() for s in seq]
        if texts[0].upper() not in upper:
            continue
        start = upper.index(texts[0].upper())
        if all(texts[i].upper() == seq[(start + i) % len(seq)].upper() for i in range(len(texts))):
            return f"序列（{seq[0]}…）", seq, start
    return "", None, 0


def plan_fill(
    rows: Sequence[Mapping[str, Any]],
    columns: Sequence[str],
    source: CellRange,
    target_rows: int,
) -> FillPlan:
    """按源区域的规律，把值延伸到 `target_rows` 行。

    ## 支持哪三种规律（按优先级）

    1. **数字等差**：源区一列是 `38, 37` → 继续 `36, 35…`；单值则照抄。
       这是 U 位填充的主场景（一台设备往下挪一格）。
    2. **内置文本序列**：`MON,TUE` / `周一,周二` → 循环。
    3. **照抄**：其余情况整行重复。

    ## 为什么不做「智能推断」（比如 abc→abd、1号→2号）

    Excel 的智能填充看着聪明，但猜错时用户往往**不会发现**（值看着都合理）。
    意图表里猜错一个机柜号，生成出来就是接错一柜的设备。
    只做确定性规律，其余老实照抄 —— 用户至少知道自己在复制什么。

    ## 源区域必须是单行或单列

    多行多列的源区域延伸规律不唯一（是整体重复，还是每列各自等差？），
    所以拒绝，让用户自己说清楚。
    """
    if source.height > 1 and source.width > 1:
        raise ValueError(
            f"源区域是 {source.height} 行 × {source.width} 列，"
            "填充只支持单行或单列（多行多列看不出该按哪种规律延伸）"
        )
    if target_rows <= source.row_end:
        raise ValueError(
            f"填充终点第 {target_rows} 行必须在源区域（到第 {source.row_end + 1} 行）之后"
        )

    # 取出每列在源区域里的值序列（单行源区域 → 每列一个值；单列 → 一列多个值）
    src_values: dict[int, list[Any]] = {}
    for col_index in range(source.col_start, source.col_end + 1):
        column = columns[col_index] if col_index < len(columns) else ""
        vals: list[Any] = []
        for row_index in range(source.row_start, source.row_end + 1):
            row = rows[row_index] if row_index < len(rows) else {}
            vals.append(row.get(column) if isinstance(row, Mapping) and column else None)
        src_values[col_index] = vals

    patterns: list[str] = []
    extenders: dict[int, Any] = {}
    for col_index, vals in src_values.items():
        desc, step = _infer_numeric(vals)
        if step is not None:
            patterns.append(desc)
            extenders[col_index] = ("numeric", vals, step)
            continue
        desc, seq, start = _infer_text_sequence(vals)
        if seq is not None:
            patterns.append(desc)
            extenders[col_index] = ("sequence", seq, start + len(vals))
            continue
        patterns.append("照抄")
        extenders[col_index] = ("copy", vals, 0)

    # 去重后合并描述（多列可能各自不同）
    uniq = list(dict.fromkeys(patterns))
    pattern_text = " / ".join(uniq) if uniq else "照抄"

    out: list[dict[str, Any]] = []
    # 要写 `source.row_end+1 .. target_rows`，共 `target_rows - source.row_end` 行。
    # `range` 右端不含，所以是 `+1`。
    for offset in range(1, target_rows - source.row_end + 1):
        item: dict[str, Any] = {}
        for col_index, extender in extenders.items():
            column = columns[col_index] if col_index < len(columns) else ""
            if not column:
                continue
            kind = extender[0]
            if kind == "numeric":
                _, vals, step = extender
                base = vals[-1]
                item[column] = base + step * offset
            elif kind == "sequence":
                _, seq, cursor = extender
                item[column] = seq[(cursor + offset - 1) % len(seq)]
            else:
                _, vals, _ = extender
                item[column] = vals[(offset - 1) % len(vals)] if vals else None
        out.append(item)

    return FillPlan(
        target=CellRange(
            row_start=source.row_start,
            row_end=target_rows,
            col_start=source.col_start,
            col_end=source.col_end,
        ),
        source_row_end=source.row_end,
        values=tuple(out),
        pattern=pattern_text,
    )


def apply_fill(
    doc: dict[str, Any],
    section: str,
    plan: FillPlan,
    *,
    coerce: Any,
) -> tuple[int, list[str]]:
    """把填充结果写进文档。返回 `(写入行数, 提示信息列表)`。

    复用 `coerce` 保证与手输、粘贴走同一条类型转换 —— 三处入口不统一
    就会出「界面说没问题、生成时崩」这类只在特定路径下复现的缺陷。
    """
    items = doc.get(section)
    if not isinstance(items, list):
        items = []
        doc[section] = items

    # 从**源区最后一行之后**开始写。用 `target.row_start + 1` 是错的：
    # 源区若是 3 行，那样会从第 2 行开始，把源区自己覆盖掉。
    base = plan.source_row_end + 1
    need = base + len(plan.values)
    while len(items) < need:
        items.append({})

    notes: list[str] = []
    written = 0
    for i, values in enumerate(plan.values):
        idx = base + i
        row = items[idx]
        if not isinstance(row, dict):
            notes.append(f"第 {idx + 1} 行不是对象，已跳过")
            continue
        for column, raw in values.items():
            value, err = coerce(section, column, raw)
            if err:
                notes.append(f"第 {idx + 1} 行 {column}：{err}")
                continue
            row[column] = value
        written += 1
    return written, notes


def normalize_range(
    row_start: int,
    row_end: int,
    col_start: int,
    col_end: int,
) -> CellRange:
    """容忍「从右下角往左上角拖」—— 鼠标拖拽的方向不固定。

    前端传上来的坐标可能是（起点 > 终点）。这里统一成规范的矩形，
    免得 `CellRange.__post_init__` 抛「坐标反了」把用户的操作判成错误。
    """
    return CellRange(
        row_start=min(row_start, row_end),
        row_end=max(row_start, row_end),
        col_start=min(col_start, col_end),
        col_end=max(col_start, col_end),
    )


__all__ = [
    "TEXT_SEQUENCES",
    "CellRange",
    "FillPlan",
    "PasteBlock",
    "PastePlan",
    "apply_fill",
    "apply_paste",
    "column_names_ok",
    "format_for_clipboard",
    "looks_like_formula",
    "normalize_range",
    "parse_clipboard",
    "plan_fill",
    "plan_paste",
]
