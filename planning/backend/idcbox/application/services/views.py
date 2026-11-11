"""视图投影层：把域数据转成前端可直接渲染的结构。

## 为什么投影要放在应用层，而不是在 API 里现拼

前端有三种视图（平面图 / 机柜 U 位图 / 布线表），它们看的是**同一份数据的不同切面**。
如果每个 router 自己从 `LoadedProject` 里取字段拼 dict，会出现三处各自实现
「一台设备占几个 U」「这个机柜装了几台设备」的逻辑 —— 然后彼此不一致。

所以这里做**一次投影、三个消费者**。这也让投影可以被 pytest 直接测
（不需要起 HTTP 服务）。

## 为什么返回 plain dict 而不是 dataclass

投影结果只用于序列化，没有行为。用 dataclass 会诱导后来人往里加方法，
把视图逻辑沉淀到"看起来像领域对象"的地方。dict + 显式 key 更诚实：
它是 JSON 的形状，不是领域概念。

## 为什么分页在后端做

5127 行 × 24 列 = 12 万个格子。一次性传给浏览器是 8~10 MB JSON，
首屏要等 2 秒。AG Grid 支持无限滚动（`datasource`），默认每页 200 行，
首屏只要 200/5127 ≈ 4% 的数据。这是"看得见"能秒开的关键。
"""

from __future__ import annotations

from collections import Counter, defaultdict
from dataclasses import dataclass
from typing import Any, Iterable, Mapping, Sequence

from ...domain.cable_length.link import LinkInput
from ...domain.cable_length.policy import LengthPolicy, ResolvedLength
from ...domain.exporting.consumables import PURCHASE_LADDER, build_consumable_report
from ...domain.geometry.layout import GeometrySnapshot
from ...domain.validation.instances import (
    DeviceInstance,
    InstanceInventory,
    extract_instances,
    find_u_conflicts,
)
from ...domain.ports.portspec import usage_of
from ...domain.validation.rules import HealthReport
from .pipeline import ComputedProject, LoadedProject

#: U 位图的机柜高度。真实机柜有 42U 与 47U 两种，但源数据最大用到 40U，
#: 且平面图没有记录柜高。用 42 作默认，前端按 `max_used` 自适应缩放。
DEFAULT_RACK_HEIGHT_U = 42


@dataclass(slots=True)
class ProjectViews:
    """一个项目的全部视图数据。构造一次，多次切片。"""

    project: LoadedProject
    computed: ComputedProject
    inventory: InstanceInventory
    #: (通道, 机柜) → 该柜设备列表
    rack_devices: Mapping[tuple[int, int], tuple[DeviceInstance, ...]]
    #: stable_key → 该连线的核定长度解析结果
    resolved_by_key: Mapping[str, ResolvedLength]
    #: stable_key → 原始行值（导出/展示原文用）
    raw_by_key: Mapping[str, Mapping[str, object]]


def build_views(project: LoadedProject, computed: ComputedProject) -> ProjectViews:
    """从已计算的项目构造全部视图数据。"""
    ctx = project.audit_context()
    inventory = extract_instances(ctx.rows, ctx.row_nos, dict(ctx.col))

    by_rack: dict[tuple[int, int], list[DeviceInstance]] = defaultdict(list)
    for inst in inventory.normalized:
        by_rack[inst.rack_key].append(inst)
    # 柜内按 U 位从高到低排（与真实机柜视觉一致：柜顶 U 号大）
    rack_devices = {
        key: tuple(sorted(items, key=lambda x: (-x.span[1], x.model)))
        for key, items in by_rack.items()
    }

    return ProjectViews(
        project=project,
        computed=computed,
        inventory=inventory,
        rack_devices=rack_devices,
        resolved_by_key=computed.by_key(),
        raw_by_key=project.raw_values,
    )


# ---------------------------------------------------------------- 元信息


def meta_payload(views: ProjectViews) -> dict[str, Any]:
    """首页概览。前端用它决定要不要显示"数据未就绪"提示。"""
    p, c, inv = views.project, views.computed, views.inventory
    report_ready = len(c.batch.results) > 0
    return {
        "projectLabel": p.label,
        "sourceFile": p.source_path.name,
        "rows": {
            "total": p.total_rows,
            "effective": p.effective_rows,
            "empty": p.empty_rows,
            "skipped": len(p.skipped),
        },
        "geometry": {
            "rackPositions": p.rack_count,
            "cmaxByRoom": dict(p.cmax_by_room),
            "rooms": sorted(p.cmax_by_room.keys()),
        },
        "devices": {
            # 两个口径都要给：834 是真实设备数，836 是"表里看起来有这么多"
            "normalized": inv.count_normalized,
            "raw": inv.count_raw,
        },
        "cables": {
            "computed": len(c.batch.results),
            "errorRows": c.batch.stats.error_rows,
            "warnRows": c.batch.stats.warn_rows,
            "overLimitRows": c.batch.stats.over_limit_rows,
        },
        "ready": report_ready,
    }


# ---------------------------------------------------------------- 平面图


def floor_payload(views: ProjectViews) -> dict[str, Any]:
    """机房平面图：房间 → 通道 → 机柜位。

    ## 为什么坐标用「列/行」而不是像素

    平面图是网格：列 3..38、行 3..16。前端用 `col*cellW` 换像素，
    这样窗口缩放、导出 SVG、将来换 3D 都只是换一次缩放系数，
    不用改数据。把像素写进接口会让"导出 SVG 施工图"变成重新排版。
    """
    p = views.project
    geo = p.geometry

    rooms: dict[str, dict[str, Any]] = {}
    for (room, ch, seq), pos in sorted(geo.racks.items()):
        r = rooms.setdefault(
            room,
            {
                "roomCode": room,
                "cmax": p.cmax_by_room.get(room),
                "channels": {},
                "cols": [pos.col, pos.col],
                "rows": [pos.row, pos.row],
            },
        )
        r["cols"][0] = min(r["cols"][0], pos.col)
        r["cols"][1] = max(r["cols"][1], pos.col)
        r["rows"][0] = min(r["rows"][0], pos.row)
        r["rows"][1] = max(r["rows"][1], pos.row)

        ch_entry = r["channels"].setdefault(
            ch,
            {"channelNo": ch, "north": [], "south": []},
        )
        side_key = "north" if pos.side == "N" else "south"
        devices = views.rack_devices.get((ch, seq), ())
        ch_entry[side_key].append(
            {
                "rackNo": seq,
                "col": pos.col,
                "row": pos.row,
                "side": pos.side,
                "deviceCount": len(devices),
                "usedU": sum(d.size_u for d in devices),
                "tools": sorted({d.model for d in devices}),
            }
        )

    # 通道内按列排序，保证前端连线的顺序稳定
    for r in rooms.values():
        for ch_entry in r["channels"].values():
            ch_entry["north"].sort(key=lambda x: x["col"])
            ch_entry["south"].sort(key=lambda x: x["col"])
        r["channels"] = [r["channels"][k] for k in sorted(r["channels"])]

    return {
        "rackHeightU": DEFAULT_RACK_HEIGHT_U,
        "rooms": [rooms[k] for k in sorted(rooms)],
        "totals": {
            "racks": p.rack_count,
            "racksWithDevices": len(views.rack_devices),
            "devices": views.inventory.count_normalized,
        },
    }


# ---------------------------------------------------------------- 机柜 U 位图


def rack_payload(views: ProjectViews, channel_no: int, seq_no: int) -> dict[str, Any] | None:
    """单机柜 U 位图。返回 None 表示该柜不在平面图中。

    ## 为什么 U 位用 `startU/endU` 而不是字符串

    `3-12U` 这种占 10U 的整机必须能算高度和位置。字符串给前端，
    每个视图都要自己解析一遍 —— 那就是三处重复的正则。
    """
    geo = views.project.geometry
    pos = geo.racks.get(_rack_key(geo, channel_no, seq_no))
    if pos is None:
        return None

    devices = views.rack_devices.get((channel_no, seq_no), ())
    slots: list[dict[str, Any]] = []
    for d in devices:
        slots.append(
            {
                "deviceKey": "|".join(str(x) for x in d.key),
                "model": d.model_raw,
                "displayName": d.display_name,
                "startU": d.span[0],
                "endU": d.span[1],
                "sizeU": d.size_u,
                "portCount": len(d.ports),
                "ports": sorted(d.ports)[:60],  # 一台设备最多 89 口，先给 60 够看
                "sourceRows": d.rows[:5],
            }
        )
    slots.sort(key=lambda s: -s["endU"])

    conflicts = [
        {
            "a": c.a.model_raw,
            "b": c.b.model_raw,
            "uRangeA": list(c.a.span),
            "uRangeB": list(c.b.span),
            "isNameVariant": c.is_name_variant,
        }
        for c in find_u_conflicts(devices)
    ]

    return {
        "channelNo": channel_no,
        "seqNo": seq_no,
        "roomCode": pos.room_code,
        "side": pos.side,
        "col": pos.col,
        "row": pos.row,
        "rackHeightU": DEFAULT_RACK_HEIGHT_U,
        "maxUsedU": max((d.span[1] for d in devices), default=0),
        "devices": slots,
        "conflicts": conflicts,
        "cables": _rack_cable_summary(views, channel_no, seq_no),
    }


def _rack_key(geo: GeometrySnapshot, channel_no: int, seq_no: int) -> tuple[str, int, int]:
    """在没有房间信息时找唯一匹配。找不到或歧义时返回一个必然 miss 的键。"""
    hits = [k for k in geo.racks if k[1] == channel_no and k[2] == seq_no]
    if len(hits) == 1:
        return hits[0]
    return ("", -1, -1)


def _rack_cable_summary(
    views: ProjectViews, channel_no: int, seq_no: int
) -> dict[str, int]:
    """该柜涉及的连线数（按本端/对端分别计）。"""
    local = peer = 0
    for link in views.project.links:
        if link.a.rack.channel_no == channel_no and link.a.rack.seq_no == seq_no:
            local += 1
        if link.b.rack.channel_no == channel_no and link.b.rack.seq_no == seq_no:
            peer += 1
    return {"asLocal": local, "asPeer": peer, "total": local + peer}


# ---------------------------------------------------------------- 布线表


#: 表格列定义。字段名与 `contract25` 不同：这里是**展示名**，
#: 导出契约用的是 Excel 列语义，两者不能混（展示可以合并/省略，导出不能）。
CABLE_GRID_COLUMNS: tuple[dict[str, Any], ...] = (
    {"field": "rowNo", "header": "行号", "width": 70, "pinned": "left"},
    {"field": "localDevice", "header": "本端设备", "width": 210, "pinned": "left"},
    {"field": "localPort", "header": "本端接口", "width": 130},
    {"field": "localRack", "header": "本端机柜", "width": 90},
    {"field": "localU", "header": "U位", "width": 80},
    {"field": "peerDevice", "header": "对端设备", "width": 210},
    {"field": "peerPort", "header": "对端接口", "width": 150},
    {"field": "peerRack", "header": "对端机柜", "width": 90},
    {"field": "peerU", "header": "U位", "width": 80},
    {"field": "cableType", "header": "线缆类型", "width": 100},
    {"field": "nCross", "header": "跨机柜", "width": 90},
    {"field": "oGap", "header": "间隔机柜", "width": 90},
    {"field": "pCount", "header": "机柜数量", "width": 90},
    {"field": "qInRack", "header": "机内线缆", "width": 90},
    {"field": "rInter", "header": "柜间线缆", "width": 90},
    {"field": "sTotal", "header": "线缆2", "width": 90},
    {"field": "tLadder", "header": "线缆长度1", "width": 100},
    {"field": "uApproved", "header": "线缆长度", "width": 100},
    {"field": "finalLength", "header": "最终长度", "width": 100},
    {"field": "policy", "header": "策略", "width": 100},
    {"field": "confidence", "header": "置信度", "width": 90},
    {"field": "oClass", "header": "空间类", "width": 80},
    {"field": "localModule", "header": "本端模块", "width": 110},
    {"field": "peerModule", "header": "对端模块", "width": 110},
    {"field": "remark", "header": "备注", "width": 100},
)


def cable_row(views: ProjectViews, link: LinkInput, row_no: int) -> dict[str, Any]:
    """一行的展示投影。"""
    res = views.resolved_by_key.get(link.stable_key)
    raw = views.raw_by_key.get(link.stable_key, {})
    return {
        "rowNo": row_no,
        "stableKey": link.stable_key,
        "localDevice": link.a.device_model or "",
        "localDeviceName": link.a.device_name or "",
        "localPort": link.a.port_name or "",
        "localRack": f"ch{link.a.rack.channel_no}/rack{link.a.rack.seq_no}",
        "localChannel": link.a.rack.channel_no,
        "localRackNo": link.a.rack.seq_no,
        "localU": raw.get("local_u"),
        "peerDevice": link.b.device_model or "",
        "peerDeviceName": link.b.device_name or "",
        "peerPort": link.b.port_name or "",
        "peerRack": f"ch{link.b.rack.channel_no}/rack{link.b.rack.seq_no}",
        "peerChannel": link.b.rack.channel_no,
        "peerRackNo": link.b.rack.seq_no,
        "peerU": raw.get("peer_u"),
        "cableType": link.cable_type,
        "nCross": raw.get("n_cross"),
        "oGap": raw.get("o_gap"),
        "pCount": raw.get("p_count"),
        "qInRack": raw.get("q_in_rack"),
        "rInter": raw.get("r_inter"),
        "sTotal": raw.get("s_total"),
        "tLadder": raw.get("t_ladder"),
        "uApproved": raw.get("u_approved"),
        "finalLength": res.approved_len_u if res else None,
        "policy": res.policy.value if res else None,
        "confidence": res.confidence if res else None,
        "oClass": res.o_class if res else None,
        "localModule": raw.get("mod_local"),
        "peerModule": raw.get("mod_peer"),
        "remark": link.remark or "",
    }


def cables_payload(
    views: ProjectViews,
    *,
    offset: int = 0,
    limit: int = 200,
    channel: int | None = None,
    rack_no: int | None = None,
    cable_type: str | None = None,
    text: str | None = None,
) -> dict[str, Any]:
    """分页 + 过滤的布线表。

    ## 为什么过滤在后端

    5127 行传完 8MB 再在前端筛，用户点"只看通道 2"要等 1 秒。
    后端筛完只传 200 行是 300KB。代价是每次筛选一次 HTTP，
    但这比"传全量再本地筛"在慢网络下快得多。
    """
    filtered: list[tuple[LinkInput, int]] = []
    needle = text.strip().lower() if text else None
    for i, link in enumerate(views.project.links):
        if channel is not None and not (
            link.a.rack.channel_no == channel or link.b.rack.channel_no == channel
        ):
            continue
        if rack_no is not None and not (
            link.a.rack.seq_no == rack_no or link.b.rack.seq_no == rack_no
        ):
            continue
        if cable_type and link.cable_type != cable_type:
            continue
        if needle and needle not in (
            f"{link.a.device_name}{link.a.port_name}"
            f"{link.b.device_name}{link.b.port_name}"
        ).lower():
            continue
        filtered.append((link, i))

    total = len(filtered)
    page = filtered[offset : offset + limit]
    rows = [cable_row(views, link, views_raw_row_no(views, link, idx)) for link, idx in page]

    return {
        "offset": offset,
        "limit": limit,
        "total": total,
        "rows": rows,
        "columns": list(CABLE_GRID_COLUMNS),
        "filterOptions": {
            "channels": sorted({l.a.rack.channel_no for l in views.project.links}),
            "cableTypes": sorted({l.cable_type for l in views.project.links}),
        },
    }


def views_raw_row_no(views: ProjectViews, link: LinkInput, fallback: int) -> int:
    """源 Excel 行号。导出对齐、施工核对、打印都要用它。"""
    return link.source_row_no if link.source_row_no is not None else fallback + 2


# ---------------------------------------------------------------- 设备连线高亮


def device_cables_payload(
    views: ProjectViews, device_name: str, *, limit: int = 200
) -> dict[str, Any]:
    """点设备 → 它的所有连线（FR-6.5 连线高亮的数据源）。"""
    hits: list[dict[str, Any]] = []
    for i, link in enumerate(views.project.links):
        if link.a.device_name == device_name or link.b.device_name == device_name:
            hits.append(cable_row(views, link, views_raw_row_no(views, link, i)))
            if len(hits) >= limit:
                break
    return {
        "deviceName": device_name,
        "total": len(hits),
        "rows": hits,
        "truncated": len(hits) >= limit,
    }


# ---------------------------------------------------------------- 体检


def health_payload(report: HealthReport) -> dict[str, Any]:
    """体检报告投影。findings 全量返回 —— 实测最多几百条，不构成体积问题。"""
    return {
        "projectLabel": report.project_label,
        "score": report.score,
        "totalRows": report.total_rows,
        "counts": {
            "blocker": report.blocker_count,
            "warn": report.warn_count,
            "info": report.info_count,
        },
        "scoreBreakdown": [
            {"dimension": d, "score": s, "weight": w} for d, s, w in report.score_breakdown
        ],
        "rules": [
            {
                "code": r.rule_code,
                "severity": r.severity,
                "title": r.title,
                "count": r.count,
                "affectedRows": r.affected_rows,
            }
            for r in report.summaries
        ],
        "findings": [
            {
                "ruleCode": f.rule_code,
                "severity": f.severity,
                "title": f.title,
                "detail": f.detail,
                "sheet": f.sheet,
                "rowNo": f.row_no,
                "column": f.column,
                "group": f.group,
                "action": f.action,
            }
            for f in report.findings
        ],
        "facts": dict(report.facts),
    }


# ---------------------------------------------------------------- 端口利用率


def _model_conflict_names(views: ProjectViews) -> set[str]:
    """源表里同一实例名被记成多个型号的那些实例名。

    ## 为什么单独挑出来

    端口台账按 `(通道, 机柜, U位, 型号)` 认设备（服务器没有名字，只能这样）。
    于是「同一个实例名出现两个型号」会被拆成两台设备，端口分散到两边 ——
    在利用率表里表现为一台"只用 2 个口"、另一台"用 30 个口"，
    并且凭空冒出「未知端口」。用户看到的是端口模板不准，
    真实原因是**源表把型号记错了**。

    不把这个根因说出来，修的人会去改模板（改坏了更麻烦）。
    """
    models: dict[str, set[str]] = defaultdict(set)
    for inst in views.inventory.normalized:
        if inst.display_name:
            models[inst.display_name].add(inst.model)
    return {name for name, ms in models.items() if len(ms) > 1}


def port_utilization_payload(views: ProjectViews) -> dict[str, Any]:
    """端口利用率（FR-3.7）。

    ## 分母终于有了（v0.5）

    v0.2~v0.4 这个函数只给「已用端口数」，并在 note 里写明
    「分母需端口模板入库后才有，当前不做假算」。v0.5 落地了
    `domain/ports/portspec.py`（26 个型号的人工核定模板，带配置原文出处），
    实测 953/954 台设备 0 未知端口、0 越界，所以这里可以给真分母了。

    ## 为什么按「设备实例」算而不是按型号均值

    同型号内端口占用差异极大（`带内交换机1` 34 台从 27 口到 51 口），
    按型号给一个均值会让「这台还能插几个」完全失真。所以逐台算完再汇总。

    ## 三个字段必须同时给

    - `usedPorts`：已用（来自实测端口名集合，不是估的）
    - `totalPorts`：标称容量（来自模板）
    - `freePorts`：`total - used`，只有前两者都有值时才给，否则 None

    只给利用率百分比会在「用了 3/3 口」和「用了 3/56 口」都是 100% 与 5.4%
    的情况下看不出绝对余量，而现场问的是「还能插几台」。
    """
    by_model: dict[str, dict[str, Any]] = defaultdict(
        lambda: {
            "deviceCount": 0,
            "usedPorts": 0,
            "totalPorts": 0,
            "freePorts": 0,
            "noSpec": 0,
            "unknownPortDevices": 0,
            "ports": Counter(),
        }
    )
    no_spec_models: set[str] = set()
    unknown_port_devices: list[str] = []

    for inst in views.inventory.normalized:
        entry = by_model[inst.model]
        entry["deviceCount"] += 1
        ports = sorted(inst.ports)
        for p in ports:
            entry["ports"][p] += 1
        usage = usage_of(inst.model, ports)
        if usage is None:
            # 没有模板：只如实报已用数，不编造分母
            entry["usedPorts"] += len(ports)
            entry["noSpec"] += 1
            no_spec_models.add(inst.model or "(型号为空)")
            continue
        entry["usedPorts"] += usage.used
        entry["totalPorts"] += usage.total
        entry["freePorts"] += max(usage.total - usage.used, 0)
        if usage.unknown_ports:
            entry["unknownPortDevices"] += 1
            label = inst.display_name or f"{inst.model}@{inst.rack_key[0]}-{inst.rack_key[1]}-{inst.u_raw}"
            unknown_port_devices.append(f"{label}: {'、'.join(usage.unknown_ports[:3])}")

    rows = []
    models_with_spec = 0
    for model in sorted(by_model):
        e = by_model[model]
        has_spec = e["totalPorts"] > 0
        if has_spec:
            models_with_spec += 1
        rows.append(
            {
                "model": model or "(型号为空)",
                "deviceCount": e["deviceCount"],
                "usedPorts": e["usedPorts"],
                "totalPorts": e["totalPorts"] if has_spec else None,
                "freePorts": e["freePorts"] if has_spec else None,
                "ratio": round(e["usedPorts"] / e["totalPorts"], 4) if has_spec else None,
                "hasSpec": has_spec,
                "noSpecDevices": e["noSpec"],
                "avgPortsPerDevice": round(
                    e["usedPorts"] / e["deviceCount"], 1
                ) if e["deviceCount"] else 0,
                "topPorts": [
                    {"port": p, "count": n} for p, n in e["ports"].most_common(8)
                ],
            }
        )

    total_used = sum(r["usedPorts"] for r in rows)
    total_cap = sum(r["totalPorts"] or 0 for r in rows)
    return {
        "byModel": rows,
        "summary": {
            "models": len(rows),
            "modelsWithSpec": models_with_spec,
            "usedPorts": total_used,
            "totalPorts": total_cap,
            "freePorts": total_cap - total_used,
            "ratio": round(total_used / total_cap, 4) if total_cap else None,
        },
        "noSpecModels": sorted(no_spec_models),
        "unknownPortDevices": unknown_port_devices[:10],
        #: 同一实例名在源表里被记成两个型号 —— 这会把一台设备拆成两台，
        #: 端口分散到两个「设备」上，于是看起来出现"未知端口"。
        #: 实测 V13 有 16 处这种矛盾（`管理接入交换机-1` 自报 `业务接入交换机1`、
        #: 被引用时写 `带外交换机1`），是源数据问题，不是端口模板问题。
        "modelConflicts": sorted(_model_conflict_names(views)),
        "note": (
            f"分母来自 {models_with_spec} 个型号的人工核定端口模板"
            "（配置原文见 `domain/ports/portspec.py`）。"
            if models_with_spec
            else "还没有任何型号匹配到端口模板，无法给出分母。"
        ),
    }


def policy_payload(views: ProjectViews) -> dict[str, Any]:
    """四策略分布 + 置信度分布 + 「假如是新建项目」的覆盖能力。

    ## 为什么要多给一个 `freshProject` 视角

    存量导入的项目 99.8% 行都是 `IMPORTED`（原表已填长度），
    这个数字本身真实但对用户没有决策价值 —— 它只说明"这是一份历史文件"。

    用户真正想知道的是：**换成新建项目，系统能替我算多少行？**
    所以这里额外按「忽略已有长度、纯几何/经验表」重算一遍，
    给出 GEOMETRIC / EMPIRICAL / 无建议 三档的实际分布。
    这才是"自动化覆盖率"的诚实口径。
    """
    policy_count: Counter[str] = Counter()
    conf_count: Counter[str] = Counter()
    o_class_count: Counter[str] = Counter()
    for res in views.computed.resolved:
        policy_count[res.policy.value] += 1
        conf_count[res.confidence] += 1
        if res.o_class:
            o_class_count[res.o_class] += 1

    # 「假如新建」口径：有几何 T 就算 GEOMETRIC 能覆盖；
    # 否则若经验表给出建议算 EMPIRICAL；否则必须人工。
    fresh = {"geometric": 0, "empirical": 0, "manual": 0}
    for res in views.computed.resolved:
        if res.calc_len_t is not None:
            fresh["geometric"] += 1
        elif res.suggest_len_m is not None:
            fresh["empirical"] += 1
        else:
            fresh["manual"] += 1
    fresh_total = sum(fresh.values()) or 1

    return {
        "policies": [
            {
                "policy": p.value,
                "count": policy_count.get(p.value, 0),
                "participatesInDeviation": p
                in (LengthPolicy.GEOMETRIC, LengthPolicy.EMPIRICAL),
            }
            for p in LengthPolicy
        ],
        "confidence": [{"level": k, "count": v} for k, v in sorted(conf_count.items())],
        "oClasses": [{"class": k, "count": v} for k, v in sorted(o_class_count.items())],
        "freshProject": {
            "geometric": fresh["geometric"],
            "empirical": fresh["empirical"],
            "manual": fresh["manual"],
            "total": sum(fresh.values()),
            "geometricPct": round(fresh["geometric"] / fresh_total * 100, 2),
            "manualPct": round(fresh["manual"] / fresh_total * 100, 2),
        },
    }


def consumable_payload(views: ProjectViews, *, phase: str | None = None) -> dict[str, Any]:
    """耗材与模块需求清单：从布线表汇总出「要买多少根」。

    ## 每一行取哪一个长度字段

    取**核定长度 U**（人工定稿），空则回退计算长度 T。理由：
    采购依据是最终装的长度，不是公式算出来的那个值。
    实测按这个口径汇总出的 24 组需求与原表 `首期耗材需求清单`
    **逐组相等**（450 根，24/24 组），模块 4/4 类也逐项相等。

    ## `phase` 过滤为什么由调用方决定

    原表 `首期` 标记只覆盖 450/5127 行（8.8%），其余是后续批次。
    不做默认过滤会让「首期采购量」变成「全项目采购量」——
    这是最危险的错法：数字看着合理，实际多买 11 倍。
    所以 `phase` 默认 None 表示全量，界面必须显式让用户选。

    ## 覆盖率披露不可省

    模块列只有 570/5113 行有值。外推会少买模块导致现场停工，
    所以 `modules.disclosure` 强制随 payload 下发。
    """
    lines: list[tuple[str | None, int | None, str | None, str | None]] = []
    seen = 0
    for link in views.project.links:
        raw = views.raw_by_key.get(link.stable_key, {})
        if phase is not None:
            remark = str(raw.get("remark") or "").strip()
            if remark != phase:
                continue
        length: int | None = None
        for key in ("u_approved", "t_ladder"):
            text = str(raw.get(key) or "").strip()
            if text and text.isdigit():
                length = int(text)
                break
        lines.append(
            (
                link.cable_type,
                length,
                str(raw.get("local_module") or ""),
                str(raw.get("peer_module") or ""),
            )
        )
        seen += 1

    report = build_consumable_report(lines, total_rows=seen)

    # 原表 `采购数量` 是人工核定的（实测倍数 1.124~2.314，与任何固定系数都对不上）。
    # 这里给出的 suggestion 只为让清单不为空，界面上必须允许逐行改。
    purchase_of = {
        (r.cable_type, r.length_m): r.count for r in report.cable_purchase_rows()
    }
    cable_rows = [
        {
            "type": r.cable_type,
            "length": r.length_m,
            "demand": r.count,
            #: 建议采购量：按 `purchase_ratio` 向上取整。**不是最终采购量**。
            "suggestedPurchase": purchase_of.get((r.cable_type, r.length_m), 0),
        }
        for r in report.cables.rows
    ]

    return {
        "phase": phase,
        "rowsTotal": seen,
        "cables": {
            "total": report.cables.total_count,
            "byType": dict(report.cables.by_type()),
            "ladder": list(PURCHASE_LADDER),
            "matrix": report.cables.matrix(include_zero=True),
            "rows": cable_rows,
            "purchaseRatio": report.purchase_ratio,
        },
        "modules": {
            "rows": [
                {
                    "module": r.module,
                    "local": r.local_count,
                    "peer": r.peer_count,
                    "total": r.total,
                }
                for r in report.modules.rows
            ],
            "coverageRows": report.modules.coverage_rows,
            "totalRows": report.modules.total_rows,
            "coverageRatio": round(report.modules.coverage_ratio, 4),
            "disclosure": report.modules.disclosure(),
        },
        "notes": list(report.notes),
    }


def rack_list_payload(views: ProjectViews) -> dict[str, Any]:
    """机柜清单：给"跳到机柜"下拉框用。"""
    items = []
    for (ch, seq), devices in sorted(views.rack_devices.items()):
        items.append(
            {
                "channelNo": ch,
                "seqNo": seq,
                "deviceCount": len(devices),
                "label": f"通道{ch} / 机柜{seq}",
            }
        )
    return {"racks": items, "total": len(items)}


__all__ = [
    "CABLE_GRID_COLUMNS",
    "DEFAULT_RACK_HEIGHT_U",
    "ProjectViews",
    "build_views",
    "cable_row",
    "cables_payload",
    "consumable_payload",
    "device_cables_payload",
    "floor_payload",
    "health_payload",
    "meta_payload",
    "policy_payload",
    "port_utilization_payload",
    "rack_list_payload",
    "rack_payload",
]
