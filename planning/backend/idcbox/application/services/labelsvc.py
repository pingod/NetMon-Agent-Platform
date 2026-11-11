"""标签清单服务层：布线表 → 两端标签（纯转换，不碰 IO）。

## 为什么标签要单独一层

`domain/exporting/labels.py` 只管「三段怎么拼、怎么排序、怎么体检」。
但拼标签需要的信息散在三处：

| 需要 | 来自 |
|---|---|
| 机柜施工标识（`11A14`） | `views.project.geometry` 的 `RackPlacement.room_code` + 机柜号 |
| 设备编码 | `views.inventory` 的实例名（交换机有、服务器多为空） |
| 设备类别（决定是否加 `(SW)`） | `DevicePorts.category` 或型号关键字 |

把它们凑到一起、并且**两端都要出标签**，是这个模块的全部职责。
放在服务层而不是域层，是因为 `views` 是应用层对象。

## 三条口径

1. **一根线出两张标签**（两端各一）。5127 根线 = 10254 张标签，
   但实际会少 —— 见下条。
2. **两端都没设备编码的线不出标签**。两端都只有「位置+端口」时，
   贴纸跟「照布线表手抄」没有区别，还多一道工序。
3. **机柜标识取不到时不猜**。`rack_code` 留空，标签变「`-16U HLW-XX 40GE1`」，
   由检查器报 `NO_RACK_CODE` —— 猜成「通道2-机柜5」会让现场找不到柜。
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

from ...domain.exporting.labels import (
    LabelSheet,
    LabelTarget,
    build_sheet,
)


#: 布局段生成的槽位号，形如 `N02-01-S0` 或 `N02-11-S7-1`。
#:
#: ## 为什么要认得出它
#:
#: 这些是**程序内部 id**，不是设备名。源表里 564 台服务器没有实例名，
#: 它们在台账里的 `device_label` 是「型号 @ 位置」，而 `device_key` 是槽位号。
#: 一旦槽位号漏进标签，现场就会去找一台叫 `N02-01-S0` 的机器 —— 找不到。
_SLOT_ID_RE = re.compile(r"^N\d{2}-\d{2}-S\d(-\d)?$")


@dataclass(frozen=True, slots=True)
class LabelSource:
    """一根线的一端，凑齐拼标签所需的最小信息。"""

    rack_code: str
    u_start: int
    device_code: str
    port_name: str
    category: str = "other"


#: 型号关键字 → 类别。与 `RackView.deviceColor()`、`svg.categorize()` 同一套顺序。
_CATEGORY_HINTS: tuple[tuple[tuple[str, ...], str], ...] = (
    (("核心",), "core"),
    (("接入", "交换机"), "switch"),
    (("防火墙", "负载", "密码", "签名", "加密", "密评"), "security"),
    (("存储",), "storage"),
    (("服务器", "节点"), "server"),
)


def category_of(model: str) -> str:
    """按型号名猜类别。

    ## 为什么按关键字而不是查型号库

    型号库（`portspec`）里有 26 个型号的分类，但标签导出要能处理**任意**
    型号（用户自己加的、源表写错的）。关键字覆盖不到的落 `other`，
    影响只是少一个 `(SW)` 后缀，不影响位置与端口。
    """
    for words, cat in _CATEGORY_HINTS:
        if any(w in model for w in words):
            return cat
    return "other"


def _rack_code_of(channel_no: int, rack_no: int) -> str:
    """机柜施工标识。

    ## 为什么是 `{通道}A{机柜}` 而不是查平面图

    实测 `11+12冷通道（示例）` 的施工标识就是 `11A14`（11 通道 14 号柜）、
    `12A02`。而 V13 的机房用 `A模组-1-13` 这种完全不同的命名 —— 两者映射
    关系在源数据里没有，所以**只能按最通用的口径拼**，并允许上层覆盖。

    这个函数是唯一需要按项目改的地方（换机房换命名规则时改它）。
    """
    return f"{channel_no}A{rack_no:02d}"


def _device_code_of(
    *, device_name: str, model: str, channel_no: int, rack_no: int, u_start: int
) -> str:
    """设备编码。

    ## 三级回退

    1. 有实例名 → 用它（`业务核心交换机-1`、`ebs-ehdd-1`）
    2. 没有实例名 → 空字符串（631 台服务器）
    3. **不用槽位号**（`N02-01-S0`）—— 那是我们内部的 id，现场认不出

    第 3 条是刻意的：把内部 id 印到标签上，等于让现场把线插到「N02-01-S0」
    这个不存在的设备上。
    """
    name = device_name.strip()
    if name and not _SLOT_ID_RE.match(name):
        return name
    return ""


#: 服务器编码的默认模板。`{seq}` 是同类设备的序号（从 1 开始，两位补零）。
#:
#: ## 为什么需要自动编号
#:
#: 源表里 564 台服务器**一个实例名都没有**（`device_instance.name` 全空）。
#: 若标签上不印编码，这 4230 张标签只剩「位置 + 端口」——
#: 而现场两台相邻服务器都插着 `万兆1` 时，标签无法区分谁接谁。
#:
#: 真实施工也确实这么干：`11+12冷通道` 示例里服务器编码是
#: `HLW-ARM-Computer01..24`，就是按型号+序号自动编的。
#:
#: ## 为什么默认关闭
#:
#: 自动编号会**发明一个客户没见过的名字**。若客户已有自己的编号规则，
#: 印上去的编码与他们的台账对不上，等于制造新的一份错误数据。
#: 所以默认留空 + 报 `NO_DEVICE_CODE`（提示性质），由用户显式打开。
SERVER_CODE_TEMPLATE = "{model}{seq:02d}"


def number_missing(
    targets: tuple[LabelTarget, ...], *, template: str = SERVER_CODE_TEMPLATE
) -> tuple[LabelTarget, ...]:
    """给没有编码的设备按「同型号 + 同通道」顺序编号。

    ## 编号范围必须全项目连续（按类别各一组）

    **曾经按通道独立计数，结果通道 2/3/8/9 各自从 01 开始 → 实测 4230 张
    标签只得到 235 个唯一编码**（`SRV80` 同时出现在 4 个机柜）。标签的作用
    就是唯一标识一端，重号等于没有编号。

    改全项目连续后，服务器 `SRV01..SRV564`、存储 `STG01..STG124` 各自唯一。
    分类别计数（不混在一起）是因为现场说的「存储第 3 台」和「服务器第 3 台」
    是两台不同设备。

    ## 只编服务器/存储，不动交换机

    交换机全都有实例名（130 台一个不缺），不会走到这里。
    安全设备/负载均衡也都有名字。真正缺编码的只有服务器与存储节点。
    """
    #: (设备位置, 类别) → 已分配编码。**必须按「设备」记而不是按「标签」记** ——
    #: 一台服务器有 7 个端口就出 7 张标签，按标签编号会让同一台机器的
    #: 7 张标签印上 7 个不同编码（实测踩过：`2A01-31U` 出现 SRV03/27/51/75/147）。
    #: 现场按编码找线时会找到 7 台不存在的设备。
    assigned: dict[tuple[str, int, str], str] = {}
    counters: dict[str, int] = {}
    out: list[LabelTarget] = []
    for t in targets:
        if t.device_code.strip():
            out.append(t)
            continue
        device_key = (t.rack_code, t.u_start, t.category)
        code = assigned.get(device_key)
        if code is None:
            short = _model_short(t)
            counters[short] = counters.get(short, 0) + 1
            code = template.format(model=short, seq=counters[short])
            assigned[device_key] = code
        out.append(
            LabelTarget(
                rack_code=t.rack_code,
                u_start=t.u_start,
                device_code=code,
                port_name=t.port_name,
                category=t.category,
            )
        )
    return tuple(out)


def _model_short(target: LabelTarget) -> str:
    """从标签反推型号短名。

    `LabelTarget` 不存型号（标签上不印型号，印的是编码）。自动编号时
    需要一个前缀，退化成类别缩写 —— 总比 `01` 强，至少同型号能连号。
    """
    return {
        "server": "SRV",
        "storage": "STG",
        "switch": "SW",
        "core": "CORE",
        "security": "SEC",
    }.get(target.category, "DEV")


def targets_from_links(links: Any, inventory: Any) -> tuple[LabelTarget, ...]:
    """遍历连线，两端各生成一个 `LabelTarget`。

    ## 为什么参数是 `links` + `inventory` 而不是 `views`

    CLI 走的是「意图 → 生成连线 → 标签」，手上只有 `GeneratedCabling.links`；
    API 走的是「项目视图 → 标签」，手上有 `views.project.links`。
    两者都是同一族对象，只要求 `a`/`b` 有 `rack`/`device_name`/`port_name`/
    `u_position`，`inventory` 有 `.normalized` —— 用鸭子类型让两边共用一份逻辑，
    而不是在 CLI 里再拼一个假的 views 对象（那种适配层最容易与真实现漂移）。
    """
    out: list[LabelTarget] = []
    #: (通道, 机柜, 起始U) → 设备实例。**用设备台账而不是连线两端** ——
    #: 连线里服务器那一侧的 `device_name` 普遍是空的（源表没填），
    #: 而台账的实例名是归一化后的权威值。
    devices: dict[tuple[int, int, int], Any] = {}
    for inst in getattr(inventory, "normalized", ()) or ():
        if inst.span is None or inst.span[0] is None:
            continue
        devices.setdefault((inst.channel_no, inst.rack_no, inst.span[0]), inst)

    for link in links:
        for end in _ends_of(link):
            channel_no, rack_no, u_raw, device_name, port_name, model = end
            u_start = _first_u(u_raw)
            if u_start is None:
                continue
            inst = devices.get((channel_no, rack_no, u_start))
            use_model = (inst.model if inst is not None else model) or ""
            name = ""
            if inst is not None and inst.names:
                #: 同一位置可能有多个实例名（源表型号冲突），取最短的：
                #: `广域网负载均衡` 比 `广域网链路负载均衡-1` 更可能是本名。
                name = sorted(inst.names, key=lambda x: (len(x), x))[0]
            elif device_name:
                name = device_name
            out.append(
                LabelTarget(
                    rack_code=_rack_code_of(channel_no, rack_no),
                    u_start=u_start,
                    device_code=_device_code_of(
                        device_name=name,
                        model=use_model,
                        channel_no=channel_no,
                        rack_no=rack_no,
                        u_start=u_start,
                    ),
                    port_name=port_name or "",
                    category=category_of(use_model),
                )
            )
    return tuple(out)


def _ends_of(link: Any) -> tuple[tuple[int, int, str, str, str, str], ...]:
    """把一根线的两端归一成统一的六元组，适配两种端点形态。

    ## 为什么需要这个适配

    API 侧用的是 `LinkInput`（`a`/`b` 是 `Endpoint`，`rack` 是 `RackRef`，
    `u_position` 是 `3-12U` 原文）；CLI 侧用的是 `GeneratedLink`
    （`a_channel`/`a_rack`/`a_u`/`a_device`/`a_port`/`a_model` 六个平铺字段）。
    两者字段语义完全一样，只是组织方式不同 —— 在这里归一，而不是让上游
    造一个假的 views（那种适配最容易与真实现漂移）。
    """
    if hasattr(link, "a") and hasattr(link, "b"):
        out = []
        for end in (link.a, link.b):
            rack = end.rack
            out.append(
                (
                    rack.channel_no,
                    rack.seq_no,
                    getattr(end, "u_position", None) or "",
                    getattr(end, "device_name", "") or "",
                    getattr(end, "port_name", "") or "",
                    getattr(end, "device_model", "") or "",
                )
            )
        return tuple(out)
    return (
        (
            link.a_channel,
            link.a_rack,
            link.a_u,
            link.a_device,
            link.a_port,
            link.a_model,
        ),
        (
            link.b_channel,
            link.b_rack,
            link.b_u,
            link.b_device,
            link.b_port,
            link.b_model,
        ),
    )


def _first_u(raw: str) -> int | None:
    """U 位原文 → 起始 U 数字（`3-12U` → 3，`40U` → 40）。"""
    from ...domain.importing.normalize import u_position_span

    start, _ = u_position_span(raw)
    return start


def targets_from_cables(views: Any) -> tuple[LabelTarget, ...]:
    """`views` 口径的便捷入口（API 侧用）。"""
    return targets_from_links(views.project.links, views.inventory)


def build_label_payload(
    *,
    links: Any,
    inventory: Any,
    project_label: str,
    limit: int | None = None,
    number_missing_codes: bool = False,
) -> dict[str, Any]:
    """标签清单（API 与 CLI 共用的唯一实现）。

    `number_missing_codes=True` 时给没有实例名的设备自动编号，
    详见 `number_missing()` 与 `SERVER_CODE_TEMPLATE` 的说明。
    """
    targets = targets_from_links(links, inventory)
    if number_missing_codes:
        targets = number_missing(targets)
    sheet = build_sheet(targets)
    rows = [
        {"text": t.joined(), "position": t.position, "device": t.device, "port": t.port}
        for t in sheet.labels
    ]
    if limit is not None:
        rows = rows[:limit]
    return {
        "project": project_label,
        "total": sheet.count,
        "rows": rows,
        "issues": [
            {"code": i.code, "message": i.message, "label": i.label, "severity": i.severity}
            for i in sheet.issues
        ],
        "countsByCode": sheet.counts_by_code(),
        "describe": sheet.describe(),
    }


def label_payload(
    views: Any, *, limit: int | None = None, number_missing_codes: bool = False
) -> dict[str, Any]:
    """`views` 口径的便捷入口（API 侧用）。"""
    return build_label_payload(
        links=views.project.links,
        inventory=views.inventory,
        project_label=views.project.label,
        limit=limit,
        number_missing_codes=number_missing_codes,
    )


__all__ = [
    "SERVER_CODE_TEMPLATE",
    "LabelSource",
    "build_label_payload",
    "category_of",
    "label_payload",
    "number_missing",
    "targets_from_cables",
    "targets_from_links",
]
