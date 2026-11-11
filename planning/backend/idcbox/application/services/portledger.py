"""端口台账服务：把域层台账摊成界面对的 JSON。

## 为什么要单独一层（而不是直接在 API 里拼 dict）

台账有三个消费者：API（界面）、CLI（导出/巡检脚本）、以及未来的导出到 Excel。
三者若各自拼 JSON，会出现「界面上 77.7%、CLI 打印 77.6%」这种对不上的情况 ——
用户第一反应是"这软件算得不准"，而实际只是四舍五入位置不同。

所以汇总口径（分母是谁、无模板设备怎么算、空口算不算容量）只在这里定一次。

## 三个口径决定

1. **分母只算有模板的设备。** 没有模板等于「不知道几个口」，
   放进分母会把整体利用率拉低，看起来像"用了很少"——比不给数字更糟。
2. **同时给「用了多少口」和「多少台设备」。** 现场关心两件事：
   「这个型号还剩几个口」（按设备看）和「整个机房还有多少余量」（按型号汇总）。
3. **无模板、越界、未知端口三条都要单独暴露。** 它们是数据问题，
   混进正常数字里就永远没人修。
"""

from __future__ import annotations

from collections import Counter, defaultdict
from typing import Any

from ...domain.ports.ledger import DevicePorts, PortLedger


def device_payload(dev: DevicePorts, *, ports: bool = False) -> dict[str, Any]:
    """一台设备的台账摘要。`ports=True` 时附上端口明细。"""
    out: dict[str, Any] = {
        "key": dev.device_key,
        "label": dev.device_label,
        "model": dev.model,
        "category": dev.category,
        "channel": dev.channel_no,
        "rack": dev.rack_no,
        "uStart": dev.u_start,
        "uSpan": dev.u_span,
        "used": dev.used_count,
        "total": dev.total_count,
        "free": dev.free_count,
        "hasSpec": dev.has_spec,
        # 利用率用 None 表示「算不出来」。前端据此显示「—」而不是 0%。
        "ratio": None if dev.ratio is None else round(dev.ratio, 4),
        "byRole": dev.by_role(),
        "unknownPorts": list(dev.unknown_ports),
    }
    if ports:
        out["portList"] = [
            {
                "port": e.physical_name,
                "bay": None if e.bay is None else list(e.bay),
                "role": e.role,
                "speed": e.speed_mbps,
                "peer": e.peer_device,
                "peerPort": e.peer_port,
                "cable": e.cable_type,
                "used": e.is_used,
            }
            for e in dev.used
        ]
        out["freeBays"] = [list(b) for b in dev.free]
    return out


def summary_payload(ledger: PortLedger) -> dict[str, Any]:
    """全局汇总。页面顶部那排数字。"""
    by_cat: dict[str, dict[str, int]] = defaultdict(
        lambda: {"devices": 0, "used": 0, "capacity": 0, "free": 0, "noSpec": 0}
    )
    for d in ledger.devices:
        row = by_cat[d.category]
        row["devices"] += 1
        row["used"] += d.used_count
        row["capacity"] += d.total_count
        row["free"] += d.free_count
        if not d.has_spec:
            row["noSpec"] += 1

    cats = []
    for name, row in sorted(by_cat.items(), key=lambda kv: -kv[1]["used"]):
        pct = row["used"] / row["capacity"] if row["capacity"] else None
        cats.append(
            {
                "category": name,
                "devices": row["devices"],
                "used": row["used"],
                "capacity": row["capacity"],
                "free": row["free"],
                "noSpec": row["noSpec"],
                "ratio": None if pct is None else round(pct, 4),
            }
        )

    return {
        "devices": ledger.device_count,
        "used": ledger.used_total,
        "capacity": ledger.capacity_total,
        "free": ledger.free_total,
        "ratio": (
            round(ledger.used_total / ledger.capacity_total, 4)
            if ledger.capacity_total
            else None
        ),
        "noSpecDevices": len(ledger.no_spec_devices),
        "overUsedDevices": len(ledger.over_used),
        "unknownPortDevices": len(ledger.with_unknown_ports),
        "byCategory": cats,
    }


def model_payload(ledger: PortLedger, *, limit: int = 200) -> list[dict[str, Any]]:
    """按型号汇总 —— 回答「这个型号还剩多少口能插」。

    现场最常见的问法不是「3 号柜那台还剩几口」，而是
    「业务接入交换机 1 这一批一共还剩多少口、能不能再挂 100 台服务器」。
    """
    rows: list[dict[str, Any]] = []
    for model, devices in ledger.by_model().items():
        used = sum(d.used_count for d in devices)
        capacity = sum(d.total_count for d in devices)
        free = sum(d.free_count for d in devices)
        no_spec = sum(1 for d in devices if not d.has_spec)
        rows.append(
            {
                "model": model,
                "category": devices[0].category,
                "devices": len(devices),
                "used": used,
                "capacity": capacity,
                "free": free,
                "noSpec": no_spec,
                "ratio": round(used / capacity, 4) if capacity else None,
                "minUsed": min((d.used_count for d in devices), default=0),
                "maxUsed": max((d.used_count for d in devices), default=0),
            }
        )
    rows.sort(key=lambda r: (-r["capacity"], r["model"]))
    return rows[:limit]


def rack_payload(ledger: PortLedger) -> list[dict[str, Any]]:
    """按机柜汇总 —— 回答「这个柜子还有没有余量」。"""
    out: list[dict[str, Any]] = []
    for (channel, rack), devices in sorted(ledger.by_rack().items()):
        used = sum(d.used_count for d in devices)
        capacity = sum(d.total_count for d in devices)
        out.append(
            {
                "channel": channel,
                "rack": rack,
                "devices": len(devices),
                "used": used,
                "capacity": capacity,
                "free": sum(d.free_count for d in devices),
                "ratio": round(used / capacity, 4) if capacity else None,
                "models": dict(Counter(d.model for d in devices)),
            }
        )
    return out


def ledger_payload(
    ledger: PortLedger,
    *,
    include_devices: bool = True,
    limit: int = 200,
) -> dict[str, Any]:
    """台账的完整载荷（分页只作用于设备列表，汇总永远是全量）。"""
    payload: dict[str, Any] = {
        "summary": summary_payload(ledger),
        "byModel": model_payload(ledger),
        "byRack": rack_payload(ledger),
        "devicesTotal": ledger.device_count,
    }
    if include_devices:
        payload["devices"] = [device_payload(d) for d in ledger.devices[:limit]]
    questions = attention_issues(ledger)
    if questions:
        payload["attention"] = questions
    return payload


def attention_issues(ledger: PortLedger) -> list[dict[str, Any]]:
    """需要人看一眼的数据问题。空列表表示台账干净。

    ## 为什么这三条要单独列出来
    它们都在「正常范围」内不会被任何断言拦住：
    - 无模板 → 利用率显示「—」，看起来只是没数据
    - 越界 → 用了 105%，很容易被显示逻辑 clamp 成 100%
    - 未知端口 → 只影响那么一两台设备，整体统计看不出异常
    但每一条都会导致现场按台账插线时撞车，所以必须主动提示。
    """
    issues: list[dict[str, Any]] = []
    for d in ledger.no_spec_devices:
        issues.append(
            {
                "kind": "NO_SPEC",
                "severity": "warn",
                "device": d.device_label,
                "message": (
                    f"{d.device_label}（{d.model or '型号为空'}）没有端口模板："
                    f"已用 {d.used_count} 口，标称容量未知，无法判断还剩几口"
                ),
            }
        )
    for d in ledger.over_used:
        issues.append(
            {
                "kind": "OVER_USED",
                "severity": "error",
                "device": d.device_label,
                "message": (
                    f"{d.device_label}（{d.model}）用了 {d.used_count} 口，"
                    f"超过模板标称的 {d.total_count} 口 —— 模板写小了"
                ),
            }
        )
    for d in ledger.with_unknown_ports:
        shown = "、".join(d.unknown_ports[:4])
        more = f" 等 {len(d.unknown_ports)} 个" if len(d.unknown_ports) > 4 else ""
        issues.append(
            {
                "kind": "UNKNOWN_PORT",
                "severity": "warn",
                "device": d.device_label,
                "message": (
                    f"{d.device_label}（{d.model}）有模板里没有的端口：{shown}{more}"
                    " —— 空闲口清单可能不准"
                ),
            }
        )
    return issues


__all__ = [
    "attention_issues",
    "device_payload",
    "ledger_payload",
    "model_payload",
    "rack_payload",
    "summary_payload",
]
