"""两份存量布线/连线表之间的差异 —— 任意两个 Excel 版本互比。

## 为什么需要这个模块（而不是项目的版本 diff）

项目版本 diff（`domain/ports/docdiff.py`）比的是**意图**（949 行，人写给程序的）。
但用户手里还有 11 个 Excel：`V3 / V8 / V9 / V11 / V12 / V13`、`11+12冷通道（示例）`、
`政务外网区交换机连线表`。这些是**交付物**（5127 行，人对施工队的承诺）。
"V12 到 V13 到底改了什么"是真实存在的提问，而 Excel 自己查不出来
（没有 diff、没有版本控制、`布线表（旧）` 靠人记得）。

## 决定性实测：五版「连线表(不含长度)」逐行完全相同

对 V8 / V9 / V11 / V12 / V13 的 13 列 `连线表` 按前 11 列取键做集合比较：

| 版本对 | 只在左 | 只在右 | 共有 |
|---|---|---|---|
| V8 → V9 | 0 | 0 | 5110 |
| V9 → V11 | 0 | 0 | 5110 |
| V11 → V12 | 0 | 0 | 5110 |
| V12 → V13 | 0 | 0 | 5110 |

**从 V8（2026-06-07）到 V13（2026-07-14）这五个星期里，这份表一个格都没动。**
所以「比连线表」这个功能如果只比那一份，输出永远是"无差异"——
看起来像程序坏了，实际是文件是死的。真正在演进的是 `布线表` 和
`连线表(设计院填写长度)`。

**这条事实是本模块存在的理由**：用户不知道该比哪个 sheet，
所以模块必须自己认出哪个 sheet 有内容，并把"这份表没变"当成结论报出来，
而不是当成错误藏起来。

## 决定性实测：V12 → V13 的真正变化是「实例名列从 14% 填到 100%」

逐列填充率（5127~5131 行）：

| 版本 | 实例 | 对端实例 | 对端通道 | 对端机柜 |
|---|---|---|---|---|
| V11-17列 | 14% | 8% | 99% | 99% |
| V12-25列 | 14% | 8% | 99% | 99% |
| V13-25列 | **100%** | 17% | 100% | 100% |
| V13「布线表（旧）」 | 14% | 8% | 99% | 99% |

也就是说 V13 那次改动**不是改接线，是把「本端设备名称」这一列补齐了**，
顺带补齐对端位置列。这类变化用"新增/删除/修改行"三个数字根本表达不出来
（实例名从空到有，按行比会得到 4400 条"修改"），必须单独报**填充率变化**。

## 三层比对键（自动降级，并把选择结果告诉用户）

1. `strict`：(本端实例, 本端口, 对端实例, 对端口)
   —— 两端都有实例名时才成立。V13 的本端实例 100% 有值，但对端只有 17%，
   所以只有 183 条连线能对上（真实数字）。
2. `loose`：(本端实例, 本端口, 对端型号, 对端通道, 对端机柜, 对端U位, 对端口)
   —— 对端用"型号+位置"代替名字。服务器行本来就没有名字，这是唯一可行的键。
3. `position`：(本端通道, 本端机柜, 本端U位, 本端口, 对端通道, 对端机柜, 对端U位, 对端口)
   —— 连本端实例名都不可信时（V11/V12 只有 14%）退到纯位置。

**降级规则不是"尽量匹配更多"，而是"报告自己用了哪一层"**：
用 strict 键比出 4944 条"只在右边"是错的结论（实际是右边补了名字），
用户看到 4944 会以为 V13 把连线全换了。

## 零 IO

本模块只做纯数据转换。读 Excel 在 `infrastructure/xlsx/stock_reader.py`。
"""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass, field
from typing import Any, Iterable, Mapping, Sequence

#: 参与比对的规范字段。顺序即表格展示顺序。
CANON_FIELDS: tuple[tuple[str, str], ...] = (
    ("localModel", "本端型号"),
    ("localInstance", "本端实例"),
    ("localChannel", "本端通道"),
    ("localRack", "本端机柜"),
    ("localU", "本端U位"),
    ("localPort", "本端接口"),
    ("peerModel", "对端型号"),
    ("peerInstance", "对端实例"),
    ("peerChannel", "对端通道"),
    ("peerRack", "对端机柜"),
    ("peerU", "对端U位"),
    ("peerPort", "对端接口"),
    ("cableType", "线缆类型"),
    ("lengthComputed", "计算长度"),
    ("length", "线缆长度"),
)

#: 表头别名 → 规范字段。**同一个表头出现两次时分先后**（`U位` 第一次是本端）。
#:
#: 三种真实版式（实测表头逐字）：
#: - 13 列（V8/V9 `连线表`）：本端设备|本端通道|本端机柜|U位|本端接口|对端设备|对端通道|对端机柜|U位|对端接口|线缆类型|线缆长度
#: - 17 列（V11 `连线表(设计院填写长度)`）：多「本端设备名称」「对端设备名称」两列
#: - 25 列（V12/V13 `布线表`）：多跨机柜/间隔机柜/机柜数量/机内线缆/柜间线缆/线缆2/线缆长度1/模块/备注
HEADER_ALIASES: tuple[tuple[str, tuple[str, ...]], ...] = (
    ("localModel", ("本端设备", "本端设备型号")),
    ("localInstance", ("本端设备名称", "本端实例", "本端名称")),
    ("localChannel", ("本端通道",)),
    ("localRack", ("本端机柜",)),
    ("localU", ("U位", "本端U位", "U位数")),
    ("localPort", ("本端接口", "本端端口")),
    ("peerModel", ("对端设备", "对端设备型号")),
    ("peerInstance", ("对端设备名称", "对端实例", "对端名称")),
    ("peerChannel", ("对端通道",)),
    ("peerRack", ("对端机柜",)),
    ("peerU", ("U位", "对端U位")),
    ("peerPort", ("对端接口", "对端端口")),
    ("cableType", ("线缆类型", "线缆型号")),
    ("lengthComputed", ("线缆长度1", "计算长度", "线缆长度（计算）")),
    ("length", ("线缆长度", "长度", "核定长度")),
)

#: 出现在长度列的哨兵文本（不是数字，也不是空）。
LENGTH_SENTINELS: frozenset[str] = frozenset({"待定", "一", "设备自带", "-", "—", "/", "无"})


@dataclass(frozen=True, slots=True)
class StockLayout:
    """一个 sheet 的表头解析结果。"""

    tag: str
    headers: tuple[str, ...]
    index: Mapping[str, int]

    @property
    def column_count(self) -> int:
        return len(self.headers)

    @property
    def missing(self) -> tuple[str, ...]:
        return tuple(f for f, _ in CANON_FIELDS if f not in self.index)


@dataclass(frozen=True, slots=True)
class StockRow:
    """一行的规范化取值。缺失字段为 `""`。"""

    row_no: int
    values: Mapping[str, str]

    def get(self, field_name: str) -> str:
        return self.values.get(field_name, "")


@dataclass(frozen=True, slots=True)
class StockTable:
    """一份存量表的解析结果。"""

    label: str
    layout: StockLayout
    rows: tuple[StockRow, ...]

    @property
    def row_count(self) -> int:
        return len(self.rows)

    def fill_rate(self, field_name: str) -> float:
        if not self.rows:
            return 0.0
        n = sum(1 for r in self.rows if r.get(field_name))
        return n / len(self.rows)

    @property
    def fill_rates(self) -> Mapping[str, float]:
        return {f: self.fill_rate(f) for f, _ in CANON_FIELDS}


def detect_layout(headers: Sequence[Any], *, tag: str = "") -> StockLayout:
    """按表头文字认出各列位置。认不出的字段不进 `index`。

    同名表头出现多次时按出现顺序分配：`U位` 第 1 次给本端、第 2 次给对端。
    这是本数据集唯一会重复的表头，但用「按出现次数轮转」处理能兼容别的重复。
    """
    texts = [str(h).strip() if h is not None else "" for h in headers]
    #: 别名 → 可能对应的规范字段（同一别名可能映射到两个字段，如 `U位`）
    alias_to_fields: dict[str, list[str]] = {}
    for field_name, aliases in HEADER_ALIASES:
        for alias in aliases:
            alias_to_fields.setdefault(alias, []).append(field_name)

    index: dict[str, int] = {}
    seen: Counter[str] = Counter()
    for col, text in enumerate(texts):
        fields = alias_to_fields.get(text)
        if not fields:
            continue
        # 第 k 次遇到这个表头就用列表里第 k 个字段（越界则用最后一个）
        rank = min(seen[text], len(fields) - 1)
        seen[text] += 1
        field_name = fields[rank]
        if field_name not in index:
            index[field_name] = col
    return StockLayout(tag=tag or f"{len(texts)}col", headers=tuple(texts), index=index)


def build_table(
    label: str,
    headers: Sequence[Any],
    rows: Iterable[Sequence[Any]],
    *,
    tag: str = "",
) -> StockTable:
    """把原始行按表头映射成 `StockTable`。空行跳过。"""
    if not rows:
        return StockTable(label, detect_layout(headers, tag=tag), ())
    layout = detect_layout(headers, tag=tag)
    out: list[StockRow] = []
    for idx, raw in enumerate(rows):
        if not any(v not in (None, "") for v in raw):
            continue
        values: dict[str, str] = {}
        for field_name, _ in CANON_FIELDS:
            col = layout.index.get(field_name)
            if col is None or col >= len(raw):
                values[field_name] = ""
                continue
            cell = raw[col]
            values[field_name] = "" if cell is None else str(cell).strip()
        out.append(StockRow(row_no=idx + 2, values=values))
    return StockTable(label, layout, tuple(out))


def parse_length(text: str) -> int | None:
    """长度列 → 整数。哨兵文本与空都返回 None（不是 0）。

    ## 为什么哨兵不能当 0

    `待定` 表示「还没定」，`设备自带` 表示「不用买」。当成 0 会让
    「V12 长度 0 → V13 长度 10」这种假变化出现在差异列表里，用户追着看
    会发现两边其实都是"没填"。返回 None 让它们归入"两边都没值"从而不进差异。
    """
    if not text or text in LENGTH_SENTINELS:
        return None
    try:
        return int(float(text))
    except (TypeError, ValueError):
        return None


#: 三种比对键。顺序即降级顺序。
KEY_MODES: tuple[str, ...] = ("strict", "loose", "position")


def row_key(row: StockRow, mode: str) -> tuple[str, ...]:
    """按指定层级取比对键。"""
    if mode == "strict":
        return (
            row.get("localInstance"),
            row.get("localPort"),
            row.get("peerInstance"),
            row.get("peerPort"),
        )
    if mode == "loose":
        return (
            row.get("localInstance"),
            row.get("localPort"),
            row.get("peerModel"),
            row.get("peerChannel"),
            row.get("peerRack"),
            row.get("peerU"),
            row.get("peerPort"),
        )
    return (
        row.get("localChannel"),
        row.get("localRack"),
        row.get("localU"),
        row.get("localPort"),
        row.get("peerChannel"),
        row.get("peerRack"),
        row.get("peerU"),
        row.get("peerPort"),
    )


#: 键里必须有值的字段数下限 —— 少于这个数说明这一层根本不可用。
KEY_MIN_FILL: Mapping[str, tuple[str, ...]] = {
    "strict": ("localInstance", "peerInstance"),
    "loose": ("localInstance",),
    "position": (),
}


def pick_key_mode(left: StockTable, right: StockTable) -> tuple[str, str]:
    """选比对键层级，并给出理由。

    ## 为什么不能"选匹配最多的那层"

    实测：V12 → V13 用 strict 键只 183 条对上、4944 条"只在右边"。
    若按"匹配最多"选，strict 明显最差，会自动降级 —— 这个例子恰好降对了。
    但反例是两份**都补全了实例名**的表：strict 会给出精确的 183，
    而 loose 会把不同实例但同位置的连线混为一谈，匹配数更大却更错。

    所以规则是**前提条件**而不是**匹配率**：某层键要求的字段在两侧都够满
    （默认 50%）才准用，否则降一级。这样"用了哪层、为什么"可以如实告诉用户。
    """
    for mode in KEY_MODES:
        required = KEY_MIN_FILL[mode]
        if not required:
            return mode, "两端实例名都大面积缺失，退到纯位置比对"
        ok = True
        for field_name in required:
            if left.fill_rate(field_name) < 0.5 or right.fill_rate(field_name) < 0.5:
                ok = False
                break
        if ok:
            if mode == "strict":
                return mode, "两端实例名都填得较全，按实例名精确比对"
            return mode, "对端实例名大面积缺失（服务器行本来就没有名字），对端改用型号+位置比对"
    return "position", "两端实例名都大面积缺失，退到纯位置比对"


@dataclass(frozen=True, slots=True)
class RowPair:
    """一条连线的两侧对照。"""

    key: tuple[str, ...]
    before: StockRow | None
    after: StockRow | None

    def describe(self) -> str:
        row = self.after or self.before
        if row is None:
            return ""
        parts = [
            row.get("localInstance") or row.get("localModel"),
            row.get("localPort"),
            "→",
            row.get("peerInstance") or row.get("peerModel"),
            row.get("peerPort"),
        ]
        return " ".join(p for p in parts if p)


@dataclass(frozen=True, slots=True)
class FieldChange:
    field_name: str
    label: str
    before: str
    after: str


@dataclass(frozen=True, slots=True)
class StockDiff:
    """两份存量表的完整差异。"""

    left_label: str
    right_label: str
    key_mode: str
    key_reason: str
    left_rows: int
    right_rows: int
    added: tuple[RowPair, ...]
    removed: tuple[RowPair, ...]
    changed: tuple[tuple[RowPair, tuple[FieldChange, ...]], ...]
    length_changes: tuple[tuple[RowPair, int, int], ...]
    fill_before: Mapping[str, float]
    fill_after: Mapping[str, float]
    notes: tuple[str, ...] = ()
    #: 真实条数。`added`/`removed`/`changed`/`length_changes` 只是**样本**，
    #: 被 `compare_stocks(limit=...)` 截断过。分开存的原因：
    #: 早期版本直接用 `len(diff.added)` 当条数，`limit=30` 时
    #: 4886 条差异被报成「新增 30 行」—— 用户看到 30 条会以为改动很小，
    #: 而实际是整表重排。计数永远不能被"为了渲染快"截断。
    added_total: int = 0
    removed_total: int = 0
    changed_total: int = 0
    length_change_total: int = 0

    @property
    def is_empty(self) -> bool:
        return not self.added_total and not self.removed_total and not self.changed_total

    @property
    def total_changes(self) -> int:
        return self.added_total + self.removed_total + self.changed_total

    @property
    def fill_changes(self) -> tuple[tuple[str, str, float, float], ...]:
        """填充率变化超过 5 个百分点的列。"""
        out: list[tuple[str, str, float, float]] = []
        for field_name, label in CANON_FIELDS:
            before = self.fill_before.get(field_name, 0.0)
            after = self.fill_after.get(field_name, 0.0)
            if abs(after - before) >= 0.05:
                out.append((field_name, label, before, after))
        return tuple(out)


def _indexed(rows: Sequence[StockRow], mode: str) -> dict[tuple[str, ...], list[StockRow]]:
    """按键归组。**同键多行必须保留**（一份表里同一插口不该出现两次，但源数据
    不该由 diff 层做假设 —— 实测 V13 有 22 个端口被多行占用）。
    """
    out: dict[tuple[str, ...], list[StockRow]] = {}
    for row in rows:
        out.setdefault(row_key(row, mode), []).append(row)
    return out


def _pair_rows(
    before: Sequence[StockRow], after: Sequence[StockRow]
) -> tuple[RowPair, tuple[FieldChange, ...]]:
    """同键的两组行配对（按顺序），返回第一对的字段差异。"""
    b = before[0] if before else None
    a = after[0] if after else None
    changes: list[FieldChange] = []
    if b is not None and a is not None:
        for field_name, label in CANON_FIELDS:
            x, y = b.get(field_name), a.get(field_name)
            if x != y:
                changes.append(FieldChange(field_name, label, x, y))
    return RowPair(key=(), before=b, after=a), tuple(changes)


def compare_stocks(
    left: StockTable,
    right: StockTable,
    *,
    key_mode: str | None = None,
    limit: int = 50,
) -> StockDiff:
    """比两份存量表。`limit` 只截断返回的样本，不影响计数。"""
    mode, reason = (key_mode, "调用方指定") if key_mode else pick_key_mode(left, right)
    if mode not in KEY_MODES:
        raise ValueError(f"未知比对键层级：{mode}（可选 {'/'.join(KEY_MODES)}）")

    left_index = _indexed(left.rows, mode)
    right_index = _indexed(right.rows, mode)

    added_keys = [k for k in right_index if k not in left_index]
    removed_keys = [k for k in left_index if k not in right_index]
    common_keys = [k for k in right_index if k in left_index]

    #: 空键（这一层要的字段全是空）会让大量行挤在同一组，
    #: 那样的"配对"没有意义，单独计数并在 notes 里说明。
    empty_key_count = sum(1 for k in right_index if not any(k))

    added: list[RowPair] = []
    for k in added_keys[:limit]:
        pair, _ = _pair_rows((), right_index[k])
        added.append(RowPair(key=k, before=None, after=pair.after))
    removed: list[RowPair] = []
    for k in removed_keys[:limit]:
        pair, _ = _pair_rows(left_index[k], ())
        removed.append(RowPair(key=k, before=pair.before, after=None))

    changed: list[tuple[RowPair, tuple[FieldChange, ...]]] = []
    length_changes: list[tuple[RowPair, int, int]] = []
    #: 计数与样本分离：`limit` 只截样本，不截计数。
    changed_total = 0
    length_change_total = 0
    for k in common_keys:
        pair, changes = _pair_rows(left_index[k], right_index[k])
        pair = RowPair(key=k, before=pair.before, after=pair.after)
        if changes:
            changed_total += 1
            if len(changed) < limit:
                changed.append((pair, changes))
            # 长度单独提出来：它是唯一有"该不该变"判断标准的一列
            before_len = parse_length(pair.before.get("length") if pair.before else "")
            after_len = parse_length(pair.after.get("length") if pair.after else "")
            if before_len is not None and after_len is not None and before_len != after_len:
                length_change_total += 1
                if len(length_changes) < limit:
                    length_changes.append((pair, before_len, after_len))

    notes: list[str] = []
    if empty_key_count:
        notes.append(
            f"{empty_key_count} 组行的比对键全为空（该层要求的字段都没填），"
            "这些行无法可靠配对，已计入新增/缺失"
        )
    if left.layout.missing:
        notes.append(f"{left.label} 缺少列：{'、'.join(left.layout.missing)}")
    if right.layout.missing:
        notes.append(f"{right.label} 缺少列：{'、'.join(right.layout.missing)}")
    if left.row_count == right.row_count and not added and not removed and not changed:
        notes.append(
            "两份表逐行完全一致 —— 如果你以为改过，可能改的是另一个 sheet"
            "（实测 V8~V13 的「连线表(不含长度)」五版完全相同，真正在变的是「布线表」）"
        )

    return StockDiff(
        left_label=left.label,
        right_label=right.label,
        key_mode=mode,
        key_reason=reason,
        left_rows=left.row_count,
        right_rows=right.row_count,
        added=tuple(added),
        removed=tuple(removed),
        changed=tuple(changed),
        length_changes=tuple(length_changes),
        fill_before=left.fill_rates,
        fill_after=right.fill_rates,
        notes=tuple(notes),
        added_total=len(added_keys),
        removed_total=len(removed_keys),
        changed_total=changed_total,
        length_change_total=length_change_total,
    )


def diff_payload(diff: StockDiff, *, sample: int = 8) -> dict[str, Any]:
    """给 API / 前端用的可 JSON 化结果。"""
    return {
        "left": diff.left_label,
        "right": diff.right_label,
        "keyMode": diff.key_mode,
        "keyReason": diff.key_reason,
        "leftRows": diff.left_rows,
        "rightRows": diff.right_rows,
        "totalChanges": diff.total_changes,
        "isEmpty": diff.is_empty,
        "addedCount": diff.added_total,
        "removedCount": diff.removed_total,
        "changedCount": diff.changed_total,
        "lengthChangeCount": diff.length_change_total,
        "sampleCounts": {
            "added": len(diff.added),
            "removed": len(diff.removed),
            "changed": len(diff.changed),
            "lengthChanges": len(diff.length_changes),
        },
        "added": [p.describe() for p in diff.added[:sample]],
        "removed": [p.describe() for p in diff.removed[:sample]],
        "changed": [
            {
                "row": pair.describe(),
                "fields": [
                    {"field": c.field_name, "label": c.label, "before": c.before, "after": c.after}
                    for c in changes
                ][:sample],
                "fieldCount": len(changes),
            }
            for pair, changes in diff.changed[:sample]
        ],
        "lengthChanges": [
            {"row": pair.describe(), "before": b, "after": a}
            for pair, b, a in diff.length_changes[:sample]
        ],
        "fillChanges": [
            {"field": f, "label": label, "before": round(b, 4), "after": round(a, 4)}
            for f, label, b, a in diff.fill_changes
        ],
        "notes": list(diff.notes),
    }



# ---------------------------------------------------------------------------
# 设备级聚合：把「4886 条新增 / 4890 条删除」翻译成「56 台设备挪了 U 位」
# ---------------------------------------------------------------------------
#
# ## 为什么必须有这一层
#
# V12 → V13 按位置键比得 4886 新增 / 4890 删除。这个数字对用户毫无意义 ——
# 它会让人以为整个规划重做了一遍。真相是：同一批交换机整体从 38U 挪到 37U、
# 部分端口从 25GE1/0/x 改名成 10GE1/0/x。对施工队来说这是「同一批线换个标签」。
#
# 逐行 diff 只能回答「哪个格子变了」；用户真正问的是「哪几台设备干了什么」。
# 一柜 44 根线挂在同一个交换机 U 位上，改成 44 条新增 + 44 条删除是错的抽象层级。
#
# ## 设备键的选择
#
# 用 (本端通道, 本端机柜, 本端U位) —— 一台交换机在一个机柜里占一个 U 位（或一段），
# 它所有线都挂在这个位置上。实测 V13 有 128 个这样的键、每个键下 3~89 行，
# 恰好对应 128 台交换机（与 /api/design 报的 128 台交换一致）。
#
# ## 为什么不能直接按设备键配对
#
# U 位变了，键就变了。V12→V13 有 70 个设备键完全没动、58 个新键、56 个消失的键。
# 但「消失 56 + 新增 58」里大部分是同一台设备挪了 U 位。所以配对需要第二步：
# 在**同一机柜内**按端口集合的重叠度做贪心匹配。实测这样能配上 78 对，
# 其中 56 对是 U 位变了但端口集合高度重叠（44/44、45/47 这种）——
# 这正是「挪了位置，线没动」。

#: 设备键：本端通道 + 本端机柜 + 本端U位。
DEVICE_KEY_FIELDS = ("localChannel", "localRack", "localU")


@dataclass(frozen=True, slots=True)
class DeviceGroup:
    """一台设备（一个机柜里的一个 U 位）及其全部连线。"""

    key: tuple[str, ...]
    channel: str
    rack: str
    u: str
    label: str
    ports: frozenset[str]
    models: frozenset[str]
    row_count: int

    @property
    def rack_key(self) -> tuple[str, str]:
        return (self.channel, self.rack)

    def describe(self) -> str:
        name = self.label or "、".join(sorted(self.models)) or "（未命名设备）"
        return f"{name} @ 通道{self.channel} 机柜{self.rack} {self.u}"


@dataclass(frozen=True, slots=True)
class DeviceChange:
    """一台设备在两版之间的变化。

    `kind` 四种取值：
    - `moved`   U 位变了，端口集合基本不变（同一台设备挪了位置）
    - `added`   新版才有这台设备
    - `removed` 旧版才有这台设备
    - `changed` U 位没变，但端口或对端变了
    """

    kind: str
    label: str
    channel: str
    rack: str
    before_u: str
    after_u: str
    port_overlap: int
    port_total: int
    port_added: tuple[str, ...] = ()
    port_removed: tuple[str, ...] = ()

    def describe(self) -> str:
        name = self.label or "（未命名设备）"
        where = f"通道{self.channel} 机柜{self.rack}"
        if self.kind == "moved":
            return f"{name} @ {where}：{self.before_u} → {self.after_u}（{self.port_overlap}/{self.port_total} 个端口不变）"
        if self.kind == "added":
            return f"{name} @ {where} {self.after_u}（{self.port_total} 个端口）"
        if self.kind == "removed":
            return f"{name} @ {where} {self.before_u}（{self.port_total} 个端口）"
        bits = []
        if self.port_added:
            bits.append(f"新增端口 {'、'.join(self.port_added[:4])}")
        if self.port_removed:
            bits.append(f"移除端口 {'、'.join(self.port_removed[:4])}")
        return f"{name} @ {where} {self.after_u}：" + ("；".join(bits) or "对端有变化")


@dataclass(frozen=True, slots=True)
class DeviceDiff:
    """设备级差异汇总。"""

    left_devices: tuple[DeviceGroup, ...]
    right_devices: tuple[DeviceGroup, ...]
    changes: tuple[DeviceChange, ...]

    @property
    def moved(self) -> tuple[DeviceChange, ...]:
        return tuple(c for c in self.changes if c.kind == "moved")

    @property
    def added(self) -> tuple[DeviceChange, ...]:
        return tuple(c for c in self.changes if c.kind == "added")

    @property
    def removed(self) -> tuple[DeviceChange, ...]:
        return tuple(c for c in self.changes if c.kind == "removed")

    @property
    def changed(self) -> tuple[DeviceChange, ...]:
        return tuple(c for c in self.changes if c.kind == "changed")

    @property
    def is_empty(self) -> bool:
        return not self.changes

    def counts(self) -> dict[str, int]:
        return {
            "moved": len(self.moved),
            "added": len(self.added),
            "removed": len(self.removed),
            "changed": len(self.changed),
            "leftDevices": len(self.left_devices),
            "rightDevices": len(self.right_devices),
        }


def group_devices(table: StockTable) -> tuple[DeviceGroup, ...]:
    """按 (本端通道, 本端机柜, 本端U位) 聚合成设备。

    同一键下的多行视为同一台设备的多个端口。设备名优先取 `localInstance`
    （V13 有 100% 填充），缺失时退到 `localModel`。
    """
    buckets: dict[tuple[str, ...], list[StockRow]] = {}
    for row in table.rows:
        key = tuple(row.get(f) or "" for f in DEVICE_KEY_FIELDS)
        if not any(key):
            # 三列全空的行无法归属任何设备，单独丢弃（并计数见 notes）
            continue
        buckets.setdefault(key, []).append(row)

    out: list[DeviceGroup] = []
    for key, rows in buckets.items():
        instance = ""
        for r in rows:
            if r.get("localInstance"):
                instance = r.get("localInstance")
                break
        ports = frozenset(r.get("localPort") or "" for r in rows)
        models = frozenset(r.get("localModel") or "" for r in rows) - {""}
        out.append(
            DeviceGroup(
                key=key,
                channel=key[0],
                rack=key[1],
                u=key[2],
                label=instance,
                ports=frozenset(p for p in ports if p),
                models=models,
                row_count=len(rows),
            )
        )
    out.sort(key=lambda g: (g.channel, g.rack, g.u))
    return tuple(out)


def compare_devices(left: StockTable, right: StockTable) -> DeviceDiff:
    """设备级比对。

    两步：①键相同的设备直接配对（U 位没变）②同一机柜内按端口重叠度贪心配对
    （U 位变了但端口集合高度重叠 → 同一台设备挪了位置）。

    第二步只在**同一 (通道, 机柜) 内**做，不跨柜匹配。理由：跨柜匹配
    「端口集合 44/44 重叠」在许多柜里都能成立（每台计算接入交换机都是
    万兆1~4 各 44 口），跨柜贪心会把 20 台交换机交叉配错。同柜内一般
    只有 1~3 台同类交换机，重叠度就是可靠判据。
    """
    left_devs = group_devices(left)
    right_devs = group_devices(right)

    left_by_key = {g.key: g for g in left_devs}
    right_by_key = {g.key: g for g in right_devs}

    changes: list[DeviceChange] = []
    matched_left: set[tuple[str, ...]] = set()
    matched_right: set[tuple[str, ...]] = set()

    # 第一步：键完全相同的
    for key, g in right_by_key.items():
        old = left_by_key.get(key)
        if old is None:
            continue
        matched_left.add(key)
        matched_right.add(key)
        added_ports = tuple(sorted(g.ports - old.ports))
        removed_ports = tuple(sorted(old.ports - g.ports))
        if added_ports or removed_ports:
            changes.append(
                DeviceChange(
                    kind="changed",
                    label=g.label or old.label,
                    channel=g.channel,
                    rack=g.rack,
                    before_u=old.u,
                    after_u=g.u,
                    port_overlap=len(g.ports & old.ports),
                    port_total=len(g.ports | old.ports),
                    port_added=added_ports,
                    port_removed=removed_ports,
                )
            )

    # 第二步：同柜内按端口重叠度贪心配
    def _by_rack(devs, used):
        out: dict[tuple[str, str], list[DeviceGroup]] = {}
        for g in devs:
            if g.key in used:
                continue
            out.setdefault(g.rack_key, []).append(g)
        return out

    left_rest = _by_rack(left_devs, matched_left)
    right_rest = _by_rack(right_devs, matched_right)

    for rack_key, olds in left_rest.items():
        news = list(right_rest.get(rack_key, []))
        for old in olds:
            best: DeviceGroup | None = None
            best_overlap = 0
            for cand in news:
                overlap = len(cand.ports & old.ports)
                if overlap > best_overlap:
                    best, best_overlap = cand, overlap
            if best is None or best_overlap == 0:
                continue
            news.remove(best)
            matched_left.add(old.key)
            matched_right.add(best.key)
            changes.append(
                DeviceChange(
                    kind="moved",
                    label=best.label or old.label,
                    channel=best.channel,
                    rack=best.rack,
                    before_u=old.u,
                    after_u=best.u,
                    port_overlap=best_overlap,
                    port_total=len(best.ports | old.ports),
                )
            )

    # 第三步：剩下的是真正的新增 / 移除
    for g in right_devs:
        if g.key not in matched_right:
            changes.append(
                DeviceChange(
                    kind="added",
                    label=g.label,
                    channel=g.channel,
                    rack=g.rack,
                    before_u="",
                    after_u=g.u,
                    port_overlap=0,
                    port_total=len(g.ports),
                )
            )
    for g in left_devs:
        if g.key not in matched_left:
            changes.append(
                DeviceChange(
                    kind="removed",
                    label=g.label,
                    channel=g.channel,
                    rack=g.rack,
                    before_u=g.u,
                    after_u="",
                    port_overlap=0,
                    port_total=len(g.ports),
                )
            )

    order = {"moved": 0, "added": 1, "removed": 2, "changed": 3}
    changes.sort(key=lambda c: (order[c.kind], c.channel, c.rack, c.after_u or c.before_u))
    return DeviceDiff(
        left_devices=left_devs,
        right_devices=right_devs,
        changes=tuple(changes),
    )


def device_payload(
    diff: DeviceDiff,
    *,
    sample: int = 12,
    row_added: int = 0,
    row_removed: int = 0,
) -> dict[str, Any]:
    """设备级差异的可 JSON 化结果。

    ## 计数与样本必须用不同的键

    第一版把 `counts()` 直接展开进 payload，然后写
    `"moved": [c.describe() for c in diff.moved[:sample]]` ——
    同一个键被写两次，后者覆盖前者，计数变成了一串字符串。
    前端 `data.counts.moved` 拿到数组，`.toLocaleString()` 得到 `"undefined"`。

    所以计数统一放 `counts` 子对象，样本放同名的平级键。

    ## 为什么要带行级计数（`row_added` / `row_removed`）

    两个数字必须同屏出现，用户才能理解它们的差距：

    - 行级：V12→V13 是 4886 新增 / 4890 删除
    - 设备级：56 台挪了 U 位、2 台新增

    只给设备级，用户会问「我明明看到几千行差异」；
    只给行级，用户会以为整个规划重做了一遍。
    摆在一起 + 一句解释，才是完整答案。
    """
    counts = diff.counts()
    note = ""
    if row_added or row_removed:
        note = (
            f"行级看到 {row_added} 新增 / {row_removed} 删除，设备级只有 "
            f"{counts['moved']} 台挪位置：差距来自 U 位整体重排 —— "
            "同一台交换机的几十根线挂在同一个 U 位上，U 位一变，"
            "按行比就是几十条新增 + 几十条删除，实际一根线都没换。"
        )
    return {
        "counts": counts,
        "totalChanges": len(diff.changes),
        "isEmpty": diff.is_empty,
        "moved": [c.describe() for c in diff.moved[:sample]],
        "added": [c.describe() for c in diff.added[:sample]],
        "removed": [c.describe() for c in diff.removed[:sample]],
        "changed": [c.describe() for c in diff.changed[:sample]],
        "note": note,
    }


__all__ = [
    "CANON_FIELDS",
    "HEADER_ALIASES",
    "KEY_MIN_FILL",
    "KEY_MODES",
    "FieldChange",
    "RowPair",
    "StockDiff",
    "StockLayout",
    "StockRow",
    "StockTable",
    "DEVICE_KEY_FIELDS",
    "DeviceChange",
    "DeviceDiff",
    "DeviceGroup",
    "build_table",
    "compare_devices",
    "device_payload",
    "group_devices",
    "compare_stocks",
    "detect_layout",
    "diff_payload",
    "parse_length",
    "pick_key_mode",
    "row_key",
]
