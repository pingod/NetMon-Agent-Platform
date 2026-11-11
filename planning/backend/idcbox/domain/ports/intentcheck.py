"""接线意图的**填写校验** —— 在生成之前把人填错的地方指出来。

## 与「数据体检」（`domain/validation/`）的分工

- 体检查的是**布线表**（5127 行，导入的存量数据）：U 位重叠、N/O 与几何不符、
  模块未标批次……那是「已经画完的图往哪里错」。
- 这里查的是**意图**（949 行，人写给程序的）：字段漏填、引用了不存在的设备、
  同一台交换机声明了两次……那是「填的时候就错了」。

后者必须更早、更便宜。等存进库、生成 5127 行、再跑体检才发现
「第 37 个交换机组里的实例名拼错了」，人已经不知道从哪改起。

## 三级严重度（为什么不是两级）

| 级别 | 含义 | 行为 |
|---|---|---|
| `error` | 生成必然失败或结果确定错误 | 阻断，「生成」按钮禁用 |
| `warn` | 能生成，但结果很可能不是想要的 | 放行，界面标黄 |
| `info` | 提示，帮助人发现漏填 | 放行，不强提醒 |

分开的理由：真实意图里有些「不合规」是**故意**的 ——
例如某台交换机临时不上架但保留声明。把它定成 error 会逼人删掉声明，
下次恢复又要重填；定成 warn 才符合实际工作方式。

## 为什么每条都要 `rowNo` 与 `section`

界面要在表格里把出错的那一行标红。只给 `"chains 有问题"` 人得自己找半天。
`(section, rowNo)` 是唯一的定位方式 —— 与 `validation.Finding` 的设计同源。
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any, Iterator, Literal, Mapping, Sequence

# 意图的六个段，与 `docdiff.DIFF_SECTIONS` 保持同一套 key
Section = Literal["racks", "switches", "devices", "routes", "chains", "explicitLinks"]

Severity = Literal["error", "warn", "info"]

SECTION_LABELS: dict[str, str] = {
    "racks": "布局",
    "switches": "交换机",
    "devices": "特殊设备",
    "routes": "路由",
    "chains": "交换机组",
    "explicitLinks": "显式连线",
}

#: 从「端口名」判断端口面 —— 与 `generate.py` 的 `_is_cli_port` 同一套前缀。
#: 参数化是为了让调用方（含测试）能改，但默认值与生产一致。
CLI_PORT_PREFIXES: tuple[str, ...] = (
    "GE", "10GE", "25GE", "40GE", "100GE", "400GE", "Eth",
)


@dataclass(frozen=True, slots=True)
class IntentIssue:
    """一条校验问题。

    `rowNo` 是**段内**行号（0-based，与数组下标一致）——
    界面上表格的行索引正好是它。`None` 表示问题在段级（例如整段缺关键字段）。
    """

    section: str
    rowNo: int | None
    field: str
    severity: Severity
    code: str
    message: str

    @property
    def section_label(self) -> str:
        return SECTION_LABELS.get(self.section, self.section)

    def as_dict(self) -> dict[str, Any]:
        return {
            "section": self.section,
            "sectionLabel": self.section_label,
            "rowNo": self.rowNo,
            "field": self.field,
            "severity": self.severity,
            "code": self.code,
            "message": self.message,
        }


@dataclass(frozen=True, slots=True)
class CheckResult:
    issues: tuple[IntentIssue, ...]
    counts: Mapping[str, int]
    sections: Mapping[str, int]

    @property
    def error_count(self) -> int:
        return sum(1 for x in self.issues if x.severity == "error")

    @property
    def warn_count(self) -> int:
        return sum(1 for x in self.issues if x.severity == "warn")

    @property
    def info_count(self) -> int:
        return sum(1 for x in self.issues if x.severity == "info")

    @property
    def ok(self) -> bool:
        """能不能生成。**只看 error** —— warn 是给人的提醒不是路障。"""
        return self.error_count == 0

    def by_section(self) -> dict[str, list[IntentIssue]]:
        out: dict[str, list[IntentIssue]] = {k: [] for k in SECTION_LABELS}
        for x in self.issues:
            out.setdefault(x.section, []).append(x)
        return out

    def as_dict(self) -> dict[str, Any]:
        return {
            "ok": self.ok,
            "errorCount": self.error_count,
            "warnCount": self.warn_count,
            "infoCount": self.info_count,
            "sections": dict(self.sections),
            "issues": [x.as_dict() for x in self.issues],
        }


# ---------------------------------------------------------------------------
# 内部小工具
# ---------------------------------------------------------------------------

def _blank(value: Any) -> bool:
    return value is None or (isinstance(value, str) and not value.strip())


def _is_cli_port(name: str) -> bool:
    return name.startswith(CLI_PORT_PREFIXES)


def _str(value: Any) -> str:
    return "" if value is None else str(value).strip()


def _items(doc: Mapping[str, Any], section: str) -> Sequence[Mapping[str, Any]]:
    raw = doc.get(section)
    if not isinstance(raw, list):
        return ()
    return tuple(x for x in raw if isinstance(x, Mapping))


# ---------------------------------------------------------------------------
# 各段校验
# ---------------------------------------------------------------------------

def _check_racks(doc: Mapping[str, Any]) -> Iterator[IntentIssue]:
    for i, x in enumerate(_items(doc, "racks")):
        if _blank(x.get("model")):
            yield IntentIssue("racks", i, "model", "error", "RACK_MODEL_EMPTY",
                              "布局块缺少服务器型号 —— 这一块不知道怎么装机")
        for key in ("channel", "from", "to"):
            if _blank(x.get(key)):
                yield IntentIssue("racks", i, key, "error", "RACK_FIELD_EMPTY",
                                  f"布局块缺少 {key} —— 无法定位到机柜")
        lo, hi = x.get("from"), x.get("to")
        if isinstance(lo, int) and isinstance(hi, int):
            if lo > hi:
                yield IntentIssue("racks", i, "from", "error", "RACK_RANGE_REVERSED",
                                  f"起始机柜 {lo} 大于结束机柜 {hi} —— 这段是空的")
            if lo == hi:
                yield IntentIssue("racks", i, "from", "info", "RACK_SINGLE",
                                  f"只覆盖机柜 {lo}。确认不是漏写了终点？")
        ch = x.get("channel")
        if isinstance(ch, int) and not (1 <= ch <= 14):
            yield IntentIssue("racks", i, "channel", "error", "RACK_CHANNEL_RANGE",
                              f"通道 {ch} 不在 1..14 之内")


def _check_switches(
    doc: Mapping[str, Any],
    seen_positions: dict[tuple[int, int, int], str],
    chain_members: set[str],
) -> Iterator[IntentIssue]:
    names: dict[str, int] = {}
    for i, x in enumerate(_items(doc, "switches")):
        inst = _str(x.get("instance"))
        if not inst:
            yield IntentIssue("switches", i, "instance", "error", "SW_INSTANCE_EMPTY",
                              "交换机缺少实例名 —— 交换机组和显式连线都要靠它引用")
            continue
        if inst in names:
            yield IntentIssue(
                "switches", i, "instance", "error", "SW_INSTANCE_DUPLICATED",
                f"实例名「{inst}」在第 {names[inst] + 1} 行已出现过 —— 同名会让引用指向不确定的那台",
            )
        names.setdefault(inst, i)
        if _blank(x.get("model")):
            # 严重度取决于「这台交换机要不要接服务器」。
            # 只出现在显式连线里时，不需要型号就能生成（端口名两端都写全了）；
            # 但若它挂在某个交换机组里，生成器要靠型号查端口容量 —— 那时必须填。
            in_chain = inst in chain_members
            yield IntentIssue(
                "switches", i, "model",
                "error" if in_chain else "warn",
                "SW_MODEL_EMPTY",
                "交换机缺少型号 —— 它在交换机组里，生成器需要型号查端口容量"
                if in_chain
                else "交换机缺少型号 —— 只走显式连线时能生成，但布线表这一列会是空的",
            )
        # 位置唯一性：同一 (通道,机柜,U) 不能站两台设备
        ch, rk, u = x.get("channel"), x.get("rack"), x.get("u")
        if all(isinstance(v, int) for v in (ch, rk, u)):
            key = (ch, rk, u)  # type: ignore[assignment]
            prev = seen_positions.get(key)
            if prev is not None:
                yield IntentIssue(
                    "switches", i, "u", "warn", "POSITION_TAKEN",
                    f"通道{ch} 机柜{rk} 的 {u}U 已被「{prev}」占用 —— "
                    f"真实数据里确有叠放情况，确认不是填错",
                )
            else:
                seen_positions[key] = inst
        if isinstance(u, int) and not (1 <= u <= 42):
            yield IntentIssue("switches", i, "u", "error", "U_RANGE",
                              f"U 位 {u} 不在 1..42 之内")


def _check_devices(
    doc: Mapping[str, Any], seen_positions: dict[tuple[int, int, int], str]
) -> Iterator[IntentIssue]:
    names: dict[str, int] = {}
    for i, x in enumerate(_items(doc, "devices")):
        inst = _str(x.get("instance"))
        if not inst:
            yield IntentIssue("devices", i, "instance", "error", "DEV_INSTANCE_EMPTY",
                              "特殊设备缺少实例名 —— 显式连线要靠它引用")
            continue
        if inst in names:
            yield IntentIssue("devices", i, "instance", "error", "DEV_INSTANCE_DUPLICATED",
                              f"实例名「{inst}」在第 {names[inst] + 1} 行已出现过")
        names.setdefault(inst, i)
        if _blank(x.get("model")):
            yield IntentIssue("devices", i, "model", "warn", "DEV_MODEL_EMPTY",
                              "缺少型号 —— 生成时这一列会留空")
        ch, rk, u = x.get("channel"), x.get("rack"), x.get("u")
        if all(isinstance(v, int) for v in (ch, rk, u)):
            key = (ch, rk, u)  # type: ignore[assignment]
            prev = seen_positions.get(key)
            if prev is not None:
                yield IntentIssue(
                    "devices", i, "u", "warn", "POSITION_TAKEN",
                    f"通道{ch} 机柜{rk} 的 {u}U 已被「{prev}」占用",
                )
            else:
                seen_positions[key] = inst


def _check_routes(doc: Mapping[str, Any]) -> Iterator[IntentIssue]:
    seen: dict[tuple[str, str], int] = {}
    for i, x in enumerate(_items(doc, "routes")):
        node = _str(x.get("nodeModel"))
        nic = _str(x.get("nic"))
        sw = _str(x.get("switchModel"))
        for key, val in (("nodeModel", node), ("nic", nic), ("switchModel", sw)):
            if not val:
                yield IntentIssue("routes", i, key, "error", "ROUTE_FIELD_EMPTY",
                                  f"路由缺少 {key} —— 三条缺一条都配不出线")
        if node and nic:
            k = (node, nic)
            if k in seen:
                yield IntentIssue(
                    "routes", i, "nic", "warn", "ROUTE_DUPLICATED",
                    f"{node} 的 {nic} 在第 {seen[k] + 1} 行已声明过 —— "
                    f"同一网口接两种交换机只会有一个生效",
                )
            seen.setdefault(k, i)


def _check_chains(
    doc: Mapping[str, Any], switch_names: set[str], switch_models: set[str]
) -> Iterator[IntentIssue]:
    for i, x in enumerate(_items(doc, "chains")):
        sw_model = _str(x.get("switchModel"))
        nic = _str(x.get("nic"))
        instances = x.get("instances")
        if not sw_model:
            yield IntentIssue("chains", i, "switchModel", "error", "CHAIN_MODEL_EMPTY",
                              "交换机组缺少交换机型号")
        elif switch_models and sw_model not in switch_models:
            yield IntentIssue(
                "chains", i, "switchModel", "warn", "CHAIN_MODEL_UNKNOWN",
                f"型号「{sw_model}」没在任何交换机行里出现过 —— 确认拼写",
            )
        if not nic:
            yield IntentIssue("chains", i, "nic", "error", "CHAIN_NIC_EMPTY",
                              "交换机组缺少网口名（如 万兆1）")
        if not isinstance(instances, list) or not instances:
            yield IntentIssue("chains", i, "instances", "error", "CHAIN_EMPTY",
                              "交换机组没有列出任何交换机实例 —— 这一组不会生成连线")
            continue
        seen: set[str] = set()
        for j, name in enumerate(instances):
            nm = _str(name)
            if not nm:
                yield IntentIssue("chains", i, f"instances[{j}]", "error",
                                  "CHAIN_INSTANCE_BLANK", f"第 {j + 1} 个实例名为空")
                continue
            if nm in seen:
                yield IntentIssue("chains", i, f"instances[{j}]", "error",
                                  "CHAIN_INSTANCE_DUPLICATED",
                                  f"「{nm}」在同一组里出现了两次 —— 会导致端口重复分配")
            seen.add(nm)
            if switch_names and nm not in switch_names:
                yield IntentIssue(
                    "chains", i, f"instances[{j}]", "error", "CHAIN_INSTANCE_UNKNOWN",
                    f"交换机「{nm}」没有在交换机段里声明过 —— 生成时找不到它的位置",
                )
        cap = x.get("capacity")
        if cap is not None and (not isinstance(cap, int) or cap <= 0):
            yield IntentIssue("chains", i, "capacity", "error", "CHAIN_CAPACITY",
                              f"端口容量 {cap!r} 不是正整数")
        order = x.get("order", "rack_major")
        if order not in ("rack_major", "slot_major"):
            yield IntentIssue("chains", i, "order", "error", "CHAIN_ORDER",
                              f"排序轴「{order}」不认识（只支持 rack_major / slot_major）")


def _check_explicit_links(
    doc: Mapping[str, Any], known_devices: set[str]
) -> Iterator[IntentIssue]:
    seen: dict[tuple[str, str, str, str], int] = {}
    for i, x in enumerate(_items(doc, "explicitLinks")):
        a, b = x.get("a"), x.get("b")
        for side, pair in (("a", a), ("b", b)):
            if not isinstance(pair, (list, tuple)) or len(pair) != 2:
                yield IntentIssue("explicitLinks", i, side, "error",
                                  "LINK_ENDPOINT_SHAPE",
                                  f"{side} 端必须是 [设备名, 端口名] 两元素数组")
                continue
            dev, port = _str(pair[0]), _str(pair[1])
            if not dev or not port:
                yield IntentIssue("explicitLinks", i, side, "error",
                                  "LINK_ENDPOINT_EMPTY",
                                  f"{side} 端设备名或端口名为空")
            elif known_devices and dev not in known_devices:
                yield IntentIssue(
                    "explicitLinks", i, side, "warn", "LINK_DEVICE_UNKNOWN",
                    f"设备「{dev}」没在交换机段或特殊设备段里声明过 —— "
                    f"生成时需要它的位置信息",
                )
        if (
            isinstance(a, (list, tuple)) and len(a) == 2
            and isinstance(b, (list, tuple)) and len(b) == 2
        ):
            k = (_str(a[0]), _str(a[1]), _str(b[0]), _str(b[1]))
            rev = (k[2], k[3], k[0], k[1])
            if k in seen or rev in seen:
                prev = seen.get(k, seen.get(rev, 0))
                yield IntentIssue(
                    "explicitLinks", i, "a", "warn", "LINK_DUPLICATED",
                    f"这条连线与第 {prev + 1} 行重复（正反算同一条）—— "
                    f"会生成两根线缆",
                )
            else:
                seen[k] = i
            if k[0] and k[2] and k[0] == k[2] and k[1] == k[3]:
                yield IntentIssue("explicitLinks", i, "b", "error",
                                  "LINK_SELF_LOOP", "两端是同一个设备的同一个端口")
        ctype = _str(x.get("cableType"))
        if ctype and ctype not in ("LC-LC", "RJ45", "MPO", "DAC", "AOC"):
            yield IntentIssue("explicitLinks", i, "cableType", "warn", "LINK_CABLE_TYPE",
                              f"线缆类型「{ctype}」不在已知集合内，生成时会在表里原样出现")


# ---------------------------------------------------------------------------
# 入口
# ---------------------------------------------------------------------------

def check_intent(doc: Mapping[str, Any]) -> CheckResult:
    """校验意图文档，返回全部问题。

    ## 为什么收集全部问题而不是遇错就抛

    人改完一份 949 行的表，最烦的是「改一个错 → 重新校验 → 又报下一个」。
    一次报全，让他一次改完。

    ## 为什么跨段引用只判到「名称存在」

    这里不做完整的接线可行性推导（那是 `generate_links` 的职责，
    它有几何、有容量、有相位校验）。意图校验只回答「填得对不对」，
    「接得通不通」让生成器回答 —— 两层职责不能混，否则错误信息会互相污染：
    人分不清「我名字拼错了」和「这个机柜放不下 44 台」。
    """
    issues: list[IntentIssue] = []
    issues.extend(_check_racks(doc))

    chain_members: set[str] = set()
    for c in _items(doc, "chains"):
        members = c.get("instances")
        if isinstance(members, list):
            chain_members.update(_str(m) for m in members if _str(m))

    positions: dict[tuple[int, int, int], str] = {}
    issues.extend(_check_switches(doc, positions, chain_members))
    issues.extend(_check_devices(doc, positions))

    switch_items = _items(doc, "switches")
    switch_names = {_str(x.get("instance")) for x in switch_items if _str(x.get("instance"))}
    switch_models = {_str(x.get("model")) for x in switch_items if _str(x.get("model"))}

    issues.extend(_check_routes(doc))
    issues.extend(_check_chains(doc, switch_names, switch_models))

    device_names = {_str(x.get("instance")) for x in _items(doc, "devices") if _str(x.get("instance"))}
    known = switch_names | device_names
    issues.extend(_check_explicit_links(doc, known))

    # 段级：整体空
    for section in SECTION_LABELS:
        n = len(_items(doc, section))
        if n == 0 and section in ("racks", "switches", "chains"):
            issues.append(
                IntentIssue(section, None, "", "error", "SECTION_EMPTY",
                            f"「{SECTION_LABELS[section]}」段是空的 —— 无法生成任何连线")
            )

    counts = {"error": 0, "warn": 0, "info": 0}
    for x in issues:
        counts[x.severity] = counts.get(x.severity, 0) + 1
    sections = {k: len(_items(doc, k)) for k in SECTION_LABELS}
    return CheckResult(tuple(issues), counts, sections)


def issues_for_row(result: CheckResult, section: str, row_no: int) -> list[IntentIssue]:
    """给表格用：某一行上的全部问题（`rowNo` 为 None 的段级问题不算）。"""
    return [x for x in result.issues if x.section == section and x.rowNo == row_no]


# ---------------------------------------------------------------------------
# 单元格编辑：把「用户改了一格」这件事做对
# ---------------------------------------------------------------------------

#: 每段每列的期望类型。`"int"` 会尝试转数字，失败则留原值并报错。
COLUMN_TYPES: dict[str, dict[str, str]] = {
    "racks": {"channel": "int", "from": "int", "to": "int", "model": "str"},
    "switches": {"instance": "str", "model": "str", "channel": "int", "rack": "int",
                 "u": "int", "uSpan": "int"},
    "devices": {"instance": "str", "model": "str", "channel": "int", "rack": "int",
                "u": "int", "uSpan": "int"},
    "routes": {"nodeModel": "str", "nic": "str", "switchModel": "str"},
    "chains": {"channel": "int", "switchModel": "str", "nic": "str", "capacity": "int",
               "order": "str"},
    "explicitLinks": {"cableType": "str"},
}

_INT_RE = re.compile(r"^\s*(-?\d+)\s*$")


def coerce_cell(section: str, column: str, raw: Any) -> tuple[Any, str | None]:
    """把表格里输入的一格转成意图里的值。

    返回 `(值, 错误信息或 None)`。

    ## 为什么不做「静默转换」

    用户在数字列里打了 `abc`，如果悄悄存成 `0`，生成器会拿 0 去算，
    结果全错而界面看起来正常。**宁可留下原值 + 报一条 error**，
    让人看见自己填错了 —— 这一条在 v0.2 比对的三个基准 bug 里都出现过
    同类问题（脏数据被当成合法值）。
    """
    types = COLUMN_TYPES.get(section, {})
    want = types.get(column, "str")
    if want == "int":
        if raw is None or (isinstance(raw, str) and not raw.strip()):
            return None, None
        if isinstance(raw, bool):
            return raw, f"{column} 需要整数，收到布尔值"
        if isinstance(raw, int):
            return raw, None
        if isinstance(raw, float):
            if raw.is_integer():
                return int(raw), None
            return raw, f"{column} 需要整数，收到 {raw}"
        m = _INT_RE.match(str(raw))
        if m:
            return int(m.group(1)), None
        return raw, f"{column} 需要整数，收到「{raw}」"
    text = "" if raw is None else str(raw)
    return text, None


def apply_cell(
    doc: dict[str, Any], section: str, row_no: int, column: str, raw: Any
) -> tuple[bool, str | None]:
    """把一格改动写回文档（原地修改）。

    返回 `(是否写入, 错误信息或 None)`。
    写不进去时**文档保持原样** —— 不留下半改的状态。
    """
    items = doc.get(section)
    if not isinstance(items, list):
        return False, f"没有「{SECTION_LABELS.get(section, section)}」这一段"
    if not (0 <= row_no < len(items)):
        return False, f"行号 {row_no} 越界（共 {len(items)} 行）"
    row = items[row_no]
    if not isinstance(row, dict):
        return False, f"第 {row_no + 1} 行不是对象"
    value, err = coerce_cell(section, column, raw)
    if err:
        return False, err
    if column == "instances" and section == "chains":
        return False, "交换机组实例请用「分号分隔」在整行里编辑"
    row[column] = value
    return True, None


def append_row(doc: dict[str, Any], section: str, template: Mapping[str, Any] | None = None) -> int:
    """在某段末尾加一个空行，返回新行号。

    空行用 `None` 而不是 `0`/`""` —— 校验会把 `None` 报成「缺少字段」，
    而 `0` 会被当成一个合法但错误的值悄悄过掉。
    """
    items = doc.setdefault(section, [])
    if not isinstance(items, list):
        items = []
        doc[section] = items
    seed: dict[str, Any] = {}
    if template:
        seed.update(template)
    items.append(seed)
    return len(items) - 1


def delete_row(doc: dict[str, Any], section: str, row_no: int) -> bool:
    items = doc.get(section)
    if not isinstance(items, list) or not (0 <= row_no < len(items)):
        return False
    del items[row_no]
    return True
