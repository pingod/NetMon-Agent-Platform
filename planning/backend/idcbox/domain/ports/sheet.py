"""接线意图的**表格形态**：把 Intent 摊平成一张人能填的 Excel 五段表。

## 为什么需要这一层

`intent.py` 的 JSON 格式机器友好，但用户填不了 —— V13 那份意图是 915 行 JSON，
让一个做机房规划的工程师手写 JSON 是不现实的。他手上的工具是 Excel。

所以意图需要两种形态，**同一份数据、两个视图**：

| 形态 | 谁用 | 特点 |
|---|---|---|
| JSON（`intent.py`） | 程序、版本控制、测试 | 精确、可 diff、可序列化 |
| 表格（本模块） | 人 | 一段一表、每行一件事、能加批注 |

关键设计：**表格只是视图，不是第二份数据**。`sheet_to_intent(intent_to_sheet(i)) == i`
（往返恒等）是本模块的第一条不变量，由 golden 用例守着。

## 五张表的分工

`布局` / `交换机` / `特殊设备` / `路由` / `交换机组` / `显式连线` —— 六段，
但只有前三段是「上架规划」（谁在哪），后三段是「接线意图」（谁连谁）。

这样拆的理由：真实项目里**上架规划与接线是由不同的人在不同时间做的**。
机房上架图确定后基本不动，接线则随网络方案反复调整。混成一张表会让
每次改接线都要重新确认上架数据，容易误改。

## 为什么用「一行一段」而不是「一台一行」

`布局` 表一行代表一个 `RackBlock`（如「ch2 的 rack1..12 全放计算节点A」），
不是一台服务器。V13 的 550 台服务器只需要 **28 行**就能描述完 ——
这正是「约 30 行量级」的来源。让用户填 550 行是不可接受的。

同理 `交换机组` 一行代表一条链（某通道某型号交换机承接某网口的**有序**实例列表），
26 行覆盖 119 台交换机。

## 表格里的列名用中文，但**不含单位与括号**

`u_start` 列叫「起始U位」而不是「起始U位(U)」：后者在 Excel 里会被用户
照着填 `3U`，解析时又得处理一遍。列名是给人看的标签，值是纯数据。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Iterable, Mapping, Sequence

from .intent import (
    ChainSpec,
    DeviceSpec,
    ExplicitLink,
    Intent,
    RackBlock,
    RouteSpec,
    SwitchSpec,
)


#: 六张表的表名与列定义。顺序即 Excel 里 sheet 的顺序。
#:
#: 每条是 `(表名, [(列键, 中文列名), ...])`。
#: 列键与 `Intent` 字段同名，序列化时不需第二套映射。
SHEET_SPECS: tuple[tuple[str, tuple[tuple[str, str], ...]], ...] = (
    (
        "布局",
        (
            ("channel_no", "通道"),
            ("rack_from", "起始机柜"),
            ("rack_to", "结束机柜"),
            ("node_model", "服务器型号"),
            ("slots", "槽位序列"),
            ("slot_map", "机柜槽位覆盖"),
        ),
    ),
    (
        "交换机",
        (
            ("instance", "交换机名称"),
            ("model", "型号"),
            ("channel_no", "通道"),
            ("rack_no", "机柜"),
            ("u_start", "起始U位"),
            ("u_span", "占用U数"),
        ),
    ),
    (
        "特殊设备",
        (
            ("instance", "设备名称"),
            ("model", "型号"),
            ("channel_no", "通道"),
            ("rack_no", "机柜"),
            ("u_start", "起始U位"),
            ("u_span", "占用U数"),
        ),
    ),
    (
        "路由",
        (
            ("node_model", "服务器型号"),
            ("nic", "网口"),
            ("switch_model", "接哪种交换机"),
        ),
    ),
    (
        "交换机组",
        (
            ("channel_no", "通道"),
            ("switch_model", "交换机型号"),
            ("nic", "网口"),
            ("instances", "交换机顺序"),
            ("capacity", "每台端口数"),
            ("port_prefix", "端口前缀"),
            ("order", "排序方式"),
        ),
    ),
    (
        "显式连线",
        (
            ("a_device", "本端设备"),
            ("a_port", "本端端口"),
            ("b_device", "对端设备"),
            ("b_port", "对端端口"),
            ("cable_type", "线缆类型"),
        ),
    ),
)

#: 表名 → 列键列表（外部要按名字取时用）。
SHEET_COLUMNS: dict[str, tuple[str, ...]] = {
    name: tuple(k for k, _ in cols) for name, cols in SHEET_SPECS
}


def _join_slots(slots: Sequence[int]) -> str:
    """槽位序列 → `0,1,2,3` / 连续区间 → `0-5`。

    ## 为什么要压缩成区间

    用户看一眼 `0-5` 就知道「6 个连续槽位」，看 `0,1,2,3,4,5` 要数一遍。
    实测量最大的 `RackBlock` 用区间写法能短 5 倍。解析时两种都认。
    """
    if not slots:
        return ""
    values = list(slots)
    parts: list[str] = []
    start = prev = values[0]
    for v in values[1:]:
        if v == prev + 1:
            prev = v
            continue
        parts.append(f"{start}-{prev}" if prev != start else str(start))
        start = prev = v
    parts.append(f"{start}-{prev}" if prev != start else str(start))
    return ",".join(parts)


def _parse_slots(text: str) -> tuple[int, ...]:
    """`0-5` / `0,1,2` / `0-2,5` → 槽位元组。空文本 → `()`（表示用默认 8 槽位）。

    >>> _parse_slots("0-3")
    (0, 1, 2, 3)
    >>> _parse_slots("0-1,5,7")
    (0, 1, 5, 7)
    >>> _parse_slots("")
    ()
    """
    values: list[int] = []
    for chunk in str(text).replace("，", ",").split(","):
        chunk = chunk.strip()
        if not chunk:
            continue
        if "-" in chunk:
            lo, hi = chunk.split("-", 1)
            values.extend(range(int(lo), int(hi) + 1))
        else:
            values.append(int(chunk))
    return tuple(values)


def _join_slot_map(slot_map: Sequence[tuple[int, tuple[int, ...]]]) -> str:
    """`((7, (0,1,2)), (11, (0,7)))` → `7:0-2;11:0,7`。"""
    return ";".join(f"{rack}:{_join_slots(slots)}" for rack, slots in slot_map)


def _parse_slot_map(text: str) -> tuple[tuple[int, tuple[int, ...]], ...]:
    """`7:0-6` / `7:0-6;11:0-7,7` → 覆盖表。空文本 → `()`。

    >>> _parse_slot_map("7:0-2")
    ((7, (0, 1, 2)),)
    """
    out: list[tuple[int, tuple[int, ...]]] = []
    for chunk in str(text).replace("；", ";").split(";"):
        chunk = chunk.strip()
        if not chunk:
            continue
        rack, _, slots = chunk.partition(":")
        out.append((int(rack), _parse_slots(slots)))
    return tuple(out)


def _join_instances(instances: Sequence[str]) -> str:
    """实例列表 → `A-1,A-3,A-5`（顺序有意义，不排序）。"""
    return ",".join(instances)


def _parse_instances(text: str) -> tuple[str, ...]:
    """`A-1,A-3` → `('A-1','A-3')`。**保持原顺序** —— 顺序就是接线顺序。

    >>> _parse_instances("A-1, A-3")
    ('A-1', 'A-3')
    """
    return tuple(x.strip() for x in str(text).replace("，", ",").split(",") if x.strip())


def _cell(value: Any) -> str:
    """表格单元格 → 字符串。None → `""`，整数不带 `.0`。"""
    if value is None:
        return ""
    if isinstance(value, float) and value.is_integer():
        return str(int(value))
    return str(value).strip()


def _int_of(row: Mapping[str, Any], key: str, default: int = 0) -> int:
    """取整数单元格。空 → default；`3U` / `3.0` 这类噪音容忍。"""
    raw = _cell(row.get(key))
    if not raw:
        return default
    try:
        return int(float(raw.replace("U", "").replace("u", "").strip()))
    except ValueError:
        return default


# ---------------------------------------------------------------------------
# Intent → 表格（导出方向）


def intent_to_sheet(intent: Intent) -> dict[str, list[dict[str, str]]]:
    """Intent → 六张表的数据。值是**字符串**，与 Excel 单元格同构。

    为什么值全用字符串：Excel 读回来本来就是字符串或数字混合，
    若这里返回 int，往返一次 `3` 会变成 `3.0`，golden 用例会假失败。
    """
    return {
        "布局": [
            {
                "channel_no": str(b.channel_no),
                "rack_from": str(b.rack_from),
                "rack_to": str(b.rack_to),
                "node_model": b.node_model,
                "slots": _join_slots(b.slots),
                "slot_map": _join_slot_map(b.slot_map),
            }
            for b in intent.racks
        ],
        "交换机": [
            {
                "instance": s.instance,
                "model": s.model,
                "channel_no": str(s.channel_no),
                "rack_no": str(s.rack_no),
                "u_start": str(s.u_start),
                "u_span": str(s.u_span),
            }
            for s in intent.switches
        ],
        "特殊设备": [
            {
                "instance": d.instance,
                "model": d.model,
                "channel_no": str(d.channel_no),
                "rack_no": str(d.rack_no),
                "u_start": str(d.u_start),
                "u_span": str(d.u_span),
            }
            for d in intent.devices
        ],
        "路由": [
            {
                "node_model": r.node_model,
                "nic": r.nic,
                "switch_model": r.switch_model,
            }
            for r in intent.routes
        ],
        "交换机组": [
            {
                "channel_no": str(c.channel_no),
                "switch_model": c.switch_model,
                "nic": c.nic,
                "instances": _join_instances(c.instances),
                "capacity": str(c.capacity),
                "port_prefix": c.port_prefix,
                "order": c.order,
            }
            for c in intent.chains
        ],
        "显式连线": [
            {
                "a_device": e.a_device,
                "a_port": e.a_port,
                "b_device": e.b_device,
                "b_port": e.b_port,
                "cable_type": e.cable_type,
            }
            for e in intent.explicit_links
        ],
    }


# ---------------------------------------------------------------------------
# 表格 → Intent（导入方向）


def sheet_to_intent(
    sheets: Mapping[str, Iterable[Mapping[str, Any]]],
    *,
    project: str,
    site: str = "",
) -> Intent:
    """六张表的数据 → Intent。空行自动跳过，多余列忽略。

    ## 空行怎么判

    Excel 导出后常留尾部空行（用户点过第 100 行就产生格式）。
    判据是**该行所有列都空**才跳过，不能只看第一列 —— `路由` 表的
    第一列是型号，用户可能先填了网口再补型号（顺序无关）。

    ## 缺表怎么办

    缺某张表 = 该段为空，不是错误。新项目可能还没有交换机，
    或者只填布局想先看看能展开多少台。

    >>> sheets = {"路由": [{"node_model": "A", "nic": "万兆1", "switch_model": "交换机1"}]}
    >>> it = sheet_to_intent(sheets, project="demo")
    >>> len(it.routes), it.routes[0].nic
    (1, '万兆1')
    """
    def rows_of(name: str) -> list[Mapping[str, Any]]:
        out = []
        for row in sheets.get(name, ()):
            if any(_cell(v) for v in row.values()):
                out.append(row)
        return out

    racks = tuple(
        RackBlock(
            channel_no=_int_of(r, "channel_no"),
            rack_from=_int_of(r, "rack_from"),
            rack_to=_int_of(r, "rack_to") or _int_of(r, "rack_from"),
            node_model=_cell(r.get("node_model")),
            slots=_parse_slots(_cell(r.get("slots"))),
            slot_map=_parse_slot_map(_cell(r.get("slot_map"))),
        )
        for r in rows_of("布局")
    )
    switches = tuple(
        SwitchSpec(
            instance=_cell(r.get("instance")),
            model=_cell(r.get("model")),
            channel_no=_int_of(r, "channel_no"),
            rack_no=_int_of(r, "rack_no"),
            u_start=_int_of(r, "u_start"),
            u_span=_int_of(r, "u_span") or 1,
        )
        for r in rows_of("交换机")
    )
    devices = tuple(
        DeviceSpec(
            instance=_cell(r.get("instance")),
            model=_cell(r.get("model")),
            channel_no=_int_of(r, "channel_no"),
            rack_no=_int_of(r, "rack_no"),
            u_start=_int_of(r, "u_start"),
            u_span=_int_of(r, "u_span") or 2,
        )
        for r in rows_of("特殊设备")
    )
    routes = tuple(
        RouteSpec(
            node_model=_cell(r.get("node_model")),
            nic=_cell(r.get("nic")),
            switch_model=_cell(r.get("switch_model")),
        )
        for r in rows_of("路由")
    )
    chains = tuple(
        ChainSpec(
            channel_no=_int_of(r, "channel_no"),
            switch_model=_cell(r.get("switch_model")),
            nic=_cell(r.get("nic")),
            instances=_parse_instances(_cell(r.get("instances"))),
            capacity=_int_of(r, "capacity") or 44,
            port_prefix=_cell(r.get("port_prefix")),
            order=_cell(r.get("order")) or "rack_major",
        )
        for r in rows_of("交换机组")
    )
    explicit_links = tuple(
        ExplicitLink(
            a_device=_cell(r.get("a_device")),
            a_port=_cell(r.get("a_port")),
            b_device=_cell(r.get("b_device")),
            b_port=_cell(r.get("b_port")),
            cable_type=_cell(r.get("cable_type")),
        )
        for r in rows_of("显式连线")
    )

    return Intent(
        project=project,
        site=site,
        racks=racks,
        switches=switches,
        devices=devices,
        routes=routes,
        chains=chains,
        explicit_links=explicit_links,
    )


__all__ = [
    "SHEET_COLUMNS",
    "SHEET_SPECS",
    "intent_to_sheet",
    "sheet_to_intent",
]
