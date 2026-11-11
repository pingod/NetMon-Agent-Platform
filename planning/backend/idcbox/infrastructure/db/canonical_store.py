"""把 `CanonBundle`（意图摊平后的规范行）写进 schema.sql 的规范表。

## 这是「物化」不是「主存储」

意图 JSON 仍是唯一真相源（人写的 949 行），这里的表是它的**物化视图**：
给定同一份意图，重跑一次必须得到完全一样的行。所以每次写入是
「先清空该项目的规范行，再全量重建」，而不是增量 upsert。

这个选择有三层理由：

1. **正确性**：增量 upsert 要自己判断「哪一行被删了」。判断错的后果是
   库里留着一台已经不存在的设备，而所有聚合都把它算进去 ——
   这类错误在界面上完全看不出来（数字都不会跳）。
2. **可验证**：全量重建让「写入后行数 == 输入行数」成为硬断言。
   增量更新下这个断言不成立，只能验证「总行数不变或增加」，等于没验证。
3. **代价可接受**：824 台设备 / 12820 个端口 / 5127 条连线，实测写入
   < 1.5 秒。用户改一次意图后重新生成一次，这个延迟不影响任何操作。

**因此：不要直接改这些表。** 要改就改意图，然后重新物化。
「改 U 位」这类操作在 v0.4 已经做成改意图（`domain/ports/moveu.py`），
规范表只是它的投影。将来若要做「在台账页直接改」，也应该是改意图后物化。

## ID 为什么是确定性哈希而不是 uuid

重跑要得到同样的行。用 `uuid4` 的话每次物化 ID 全变，
「同一台设备的历史」就断了（`cable.status` 从 planned 改到 installed
这种状态在重新物化后会丢）。用 `sha1(自然键)[:12]` 则同一台设备
永远是同一个 ID，将来要做「保留人工改过的状态字段」时才有基础。
"""

from __future__ import annotations

import hashlib
from typing import Any, Iterable, Mapping, Sequence

from sqlalchemy import text
from sqlalchemy.engine import Engine

from ...domain.ports.canon import (
    CanonAisle,
    CanonCable,
    CanonConflict,
    CanonDevice,
    CanonModel,
    CanonModelPort,
    CanonPort,
    CanonRack,
    CanonRange,
    CanonRoom,
    CanonicalBundle,
)
from .session import utcnow_iso

#: 物化时要先清空的表，按外键依赖从叶到根排列。
#:
#: 顺序错了会撞外键（`foreign_keys = ON` 是真开着的）。这里列成常量而
#: 不是在代码里现写，是为了让「加了新表忘记清」变成一眼可见的问题。
CLEAR_ORDER: tuple[str, ...] = (
    "cable",
    "switch_service_range",
    "port",
    "device_conflict",
    "device_instance",
    "device_model_port",
    "device_model",
    "rack",
    "grid_cell",
    "aisle",
    "room",
)


def _digest(*parts: object) -> str:
    """确定性短 ID。`|` 分隔避免 `a`+`bc` 与 `ab`+`c` 撞。"""
    raw = "|".join("-" if p is None or p == "" else str(p) for p in parts)
    return hashlib.sha1(raw.encode("utf-8")).hexdigest()[:16]


def room_id(project_id: str, code: str) -> str:
    return f"rom_{_digest(project_id, code)}"


def aisle_id(project_id: str, code: str, channel_no: int) -> str:
    return f"ais_{_digest(project_id, code, channel_no)}"


def rack_id(project_id: str, code: str, channel_no: int, side: str, seq_no: int) -> str:
    return f"rck_{_digest(project_id, code, channel_no, side, seq_no)}"


def model_id(project_id: str, name_normalized: str) -> str:
    return f"mdl_{_digest(project_id, name_normalized)}"


def model_port_id(model: str, physical_name: str) -> str:
    return f"mdp_{_digest(model, physical_name)}"


def device_id(project_id: str, key: Sequence[object]) -> str:
    return f"dev_{_digest(project_id, *key)}"


def port_id(project_id: str, device_key: Sequence[object], physical_name: str) -> str:
    return f"prt_{_digest(project_id, *device_key, physical_name)}"


def cable_id(project_id: str, stable_key: str, occurrence: int) -> str:
    return f"cbl_{_digest(project_id, stable_key, occurrence)}"


def range_id(project_id: str, switch_key: Sequence[object], model_norm: str, base: int) -> str:
    return f"rng_{_digest(project_id, *switch_key, model_norm, base)}"


def conflict_id(project_id: str, kind: str, channel: int, rack: int, u: int, detail: str) -> str:
    return f"cft_{_digest(project_id, kind, channel, rack, u, detail)}"


# ---------------------------------------------------------------------------
# 写入
# ---------------------------------------------------------------------------
def _speed_fallback(bundle: CanonicalBundle) -> dict[tuple[str, str], int]:
    """型号端口的速度表 —— 端口条目解析不出速率时兜底用。

    `port.speed_mbps NOT NULL`，而真实数据里有 4 个口（`广域网链路负载均衡`
    的 `GE8`）解析不出速率。用型号模板里的标称速率兜底，
    比写 0 或 NULL 都更接近真相：那个口确实是 10G 口位。
    """
    return {(p.model_normalized, p.physical_name): p.speed_mbps for p in bundle.model_ports}


def persist_canonical(
    engine: Engine,
    project_id: str,
    bundle: CanonicalBundle,
    *,
    note: str = "",
) -> dict[str, int]:
    """全量物化。返回各表写入了多少行。

    ## 一个事务

    中途失败必须整体回滚。否则库里会出现「有设备没端口」的中间态，
    而界面上它看起来像「这个项目还没导入端口」—— 用户会去查导入流程，
    而真正的原因是上次物化崩了。
    """
    speeds = _speed_fallback(bundle)
    now = utcnow_iso()
    counts: dict[str, int] = {}

    with engine.begin() as con:
        for table in CLEAR_ORDER:
            if table in ("grid_cell",):  # 不物化格子（平面图不是本层职责）
                continue
            if table == "room":
                con.execute(text("DELETE FROM room WHERE project_id = :p"), {"p": project_id})
            elif table == "aisle":
                con.execute(
                    text("DELETE FROM aisle WHERE room_id IN "
                         "(SELECT id FROM room WHERE project_id = :p)"),
                    {"p": project_id},
                )
            elif table == "rack":
                con.execute(
                    text("DELETE FROM rack WHERE room_id IN "
                         "(SELECT id FROM room WHERE project_id = :p)"),
                    {"p": project_id},
                )
            elif table in ("device_model", "device_model_port"):
                if table == "device_model_port":
                    con.execute(
                        text("DELETE FROM device_model_port WHERE model_id IN "
                             "(SELECT id FROM device_model WHERE project_id = :p)"),
                        {"p": project_id},
                    )
                else:
                    con.execute(
                        text("DELETE FROM device_model WHERE project_id = :p"),
                        {"p": project_id},
                    )
            elif table in ("device_instance", "port", "cable", "switch_service_range",
                           "device_conflict"):
                if table == "device_instance":
                    con.execute(
                        text("DELETE FROM device_instance WHERE project_id = :p"),
                        {"p": project_id},
                    )
                elif table == "port":
                    con.execute(
                        text("DELETE FROM port WHERE device_id IN "
                             "(SELECT id FROM device_instance WHERE project_id = :p)"),
                        {"p": project_id},
                    )
                else:
                    con.execute(
                        text(f"DELETE FROM {table} WHERE project_id = :p"), {"p": project_id}
                    )

        # ---- room / aisle / rack ----
        room_ids: dict[str, str] = {}
        for room in bundle.rooms:
            rid = room_id(project_id, room.code)
            room_ids[room.code] = rid
            con.execute(
                text(
                    "INSERT INTO room (id, project_id, name, code, col_min, col_max, "
                    "declared_rack_count) VALUES (:id, :p, :name, :code, :c0, :c1, :n)"
                ),
                {"id": rid, "p": project_id, "name": room.name, "code": room.code,
                 "c0": room.col_min, "c1": room.col_max,
                 "n": sum(1 for r in bundle.racks if r.room_code == room.code)},
            )
        counts["room"] = len(bundle.rooms)

        aisle_ids: dict[tuple[str, int], str] = {}
        for aisle in bundle.aisles:
            room = room_ids.get(aisle.room_code)
            if room is None:
                continue
            aid = aisle_id(project_id, aisle.room_code, aisle.channel_no)
            aisle_ids[(aisle.room_code, aisle.channel_no)] = aid
            con.execute(
                text(
                    "INSERT INTO aisle (id, room_id, channel_no, name, plan_row_n, plan_row_s) "
                    "VALUES (:id, :room, :ch, :name, :rn, :rs)"
                ),
                {"id": aid, "room": room, "ch": aisle.channel_no,
                 "name": f"{aisle.room_code}-{aisle.channel_no}通道",
                 "rn": aisle.plan_row_n, "rs": aisle.plan_row_s},
            )
        counts["aisle"] = len(bundle.aisles)

        rack_ids: dict[tuple[int, int], str] = {}
        for rack in bundle.racks:
            room = room_ids.get(rack.room_code)
            aisle = aisle_ids.get((rack.room_code, rack.channel_no))
            if room is None or aisle is None:
                continue
            rid = rack_id(project_id, rack.room_code, rack.channel_no, rack.side, rack.seq_no)
            rack_ids[(rack.channel_no, rack.seq_no)] = rid
            con.execute(
                text(
                    "INSERT INTO rack (id, room_id, aisle_id, seq_no, side, display_name, "
                    "col_index, row_index, rack_units) "
                    "VALUES (:id, :room, :aisle, :seq, :side, :dn, :col, :row, 42)"
                ),
                {"id": rid, "room": room, "aisle": aisle, "seq": rack.seq_no,
                 "side": rack.side, "dn": rack.display_name,
                 "col": rack.col_index, "row": rack.row_index},
            )
        counts["rack"] = len(bundle.racks)

        # ---- device_model / device_model_port ----
        model_ids: dict[str, str] = {}
        for model in bundle.models:
            mid = model_id(project_id, model.name_normalized)
            model_ids[model.name_normalized] = mid
            con.execute(
                text(
                    "INSERT INTO device_model (id, project_id, name, name_normalized, "
                    "category, u_height, spec_text) "
                    "VALUES (:id, :p, :name, :norm, :cat, :uh, :spec)"
                ),
                {"id": mid, "p": project_id, "name": model.name,
                 "norm": model.name_normalized, "cat": model.category,
                 "uh": model.u_height, "spec": model.spec_text},
            )
        counts["device_model"] = len(bundle.models)

        for mp in bundle.model_ports:
            mid = model_ids.get(mp.model_normalized)
            if mid is None:
                continue
            con.execute(
                text(
                    "INSERT INTO device_model_port (id, model_id, seq, physical_name, "
                    "display_name, speed_mbps, connector, port_type, panel_slot, "
                    "panel_row, panel_col) "
                    "VALUES (:id, :m, :seq, :pn, :dn, :sp, :cn, :pt, :ps, :pr, :pc)"
                ),
                {"id": model_port_id(mid, mp.physical_name), "m": mid, "seq": mp.seq,
                 "pn": mp.physical_name, "dn": mp.physical_name, "sp": mp.speed_mbps,
                 "cn": mp.connector, "pt": mp.port_type, "ps": mp.panel_slot,
                 "pr": mp.panel_row, "pc": mp.panel_col},
            )
        counts["device_model_port"] = len(bundle.model_ports)

        # ---- device_instance ----
        device_ids: dict[tuple, str] = {}
        skipped_devices = 0
        for dev in bundle.devices:
            rid = rack_ids.get((dev.channel_no, dev.rack_no))
            mid = model_ids.get(dev.model_normalized)
            if rid is None or mid is None:
                skipped_devices += 1
                continue
            did = device_id(project_id, dev.key)
            device_ids[dev.key] = did
            con.execute(
                text(
                    "INSERT INTO device_instance (id, project_id, model_id, rack_id, "
                    "u_start, u_end, name, name_normalized, status, first_seen_at, updated_at) "
                    "VALUES (:id, :p, :m, :r, :u0, :u1, :name, :norm, :st, :now, :now)"
                ),
                {"id": did, "p": project_id, "m": mid, "r": rid,
                 "u0": dev.u_start, "u1": dev.u_end,
                 "name": dev.name or None, "norm": dev.name_normalized or None,
                 "st": dev.status, "now": now},
            )
        counts["device_instance"] = len(device_ids)
        if skipped_devices:
            counts["device_skipped"] = skipped_devices

        # ---- port ----
        port_ids: dict[tuple, str] = {}
        port_rows = 0
        for port in bundle.ports:
            did = device_ids.get(port.device_key)
            if did is None:
                continue
            pid = port_id(project_id, port.device_key, port.physical_name)
            port_ids[(port.device_key, port.physical_name)] = pid
            speed = port.speed_mbps
            if speed is None:
                speed = speeds.get((port.device_key[3], port.physical_name), 0)
            con.execute(
                text(
                    "INSERT INTO port (id, device_id, physical_name, display_name, "
                    "speed_mbps, connector, port_type, port_role, status, "
                    "panel_slot, panel_row, panel_col) "
                    "VALUES (:id, :d, :pn, :dn, :sp, :cn, :pt, :role, :st, :ps, :pr, :pc)"
                ),
                {"id": pid, "d": did, "pn": port.physical_name, "dn": port.display_name,
                 "sp": speed, "cn": port.connector, "pt": port.port_type,
                 "role": port.port_role, "st": port.status,
                 "ps": port.panel_slot, "pr": port.panel_row, "pc": port.panel_col},
            )
            port_rows += 1
        counts["port"] = port_rows

        # ---- device_conflict ----
        conflict_rows = 0
        for c in bundle.conflicts:
            rid = rack_ids.get((c.channel_no, c.rack_no))
            con.execute(
                text(
                    "INSERT INTO device_conflict (id, project_id, kind, rack_id, u_start, "
                    "u_end, detail_json, severity) "
                    "VALUES (:id, :p, :k, :r, :u0, :u1, :dj, :sev)"
                ),
                {"id": conflict_id(project_id, c.kind, c.channel_no, c.rack_no, c.u_start, c.detail),
                 "p": project_id, "k": c.kind, "r": rid, "u0": c.u_start, "u1": c.u_end,
                 "dj": f'{{"detail":"{c.detail}"}}', "sev": c.severity},
            )
            conflict_rows += 1
        counts["device_conflict"] = conflict_rows

        # ---- cable ----
        cable_rows = 0
        missing_port = 0
        for cbl in bundle.cables:
            a_pid = port_ids.get((cbl.a_device_key, cbl.a_port_name))
            z_pid = port_ids.get((cbl.z_device_key, cbl.z_port_name))
            if a_pid is None:
                missing_port += 1
                continue
            con.execute(
                text(
                    "INSERT INTO cable (id, project_id, stable_key, occurrence, a_port_id, "
                    "z_port_id, cable_type, a_module, z_module, remark) "
                    "VALUES (:id, :p, :sk, :occ, :a, :z, :ct, :am, :zm, :rm)"
                ),
                {"id": cable_id(project_id, cbl.stable_key, cbl.occurrence),
                 "p": project_id, "sk": cbl.stable_key, "occ": cbl.occurrence,
                 "a": a_pid, "z": z_pid, "ct": cbl.cable_type,
                 "am": cbl.a_module or None, "zm": cbl.z_module or None,
                 "rm": cbl.remark or None},
            )
            cable_rows += 1
        counts["cable"] = cable_rows
        if missing_port:
            counts["cable_skipped"] = missing_port

        # ---- switch_service_range ----
        range_rows = 0
        for rng in bundle.ranges:
            did = device_ids.get(rng.switch_device_key)
            mid = model_ids.get(rng.node_model_normalized)
            if did is None or mid is None:
                continue
            con.execute(
                text(
                    "INSERT INTO switch_service_range (id, project_id, switch_device_id, "
                    "node_model_id, base_port_no, sort_rule, port_role, node_count_expected) "
                    "VALUES (:id, :p, :sw, :nm, :base, :sort, :role, :cnt)"
                ),
                {"id": range_id(project_id, rng.switch_device_key, rng.node_model_normalized,
                                rng.base_port_no),
                 "p": project_id, "sw": did, "nm": mid, "base": rng.base_port_no,
                 "sort": rng.sort_rule, "role": rng.port_role,
                 "cnt": rng.node_count_expected},
            )
            range_rows += 1
        counts["switch_service_range"] = range_rows

    return counts


def canonical_counts(engine: Engine, project_id: str) -> dict[str, int]:
    """读回各表行数。用于「写入后自检」与端到端测试的断言。"""
    queries = {
        "room": "SELECT COUNT(*) FROM room WHERE project_id = :p",
        "aisle": "SELECT COUNT(*) FROM aisle WHERE room_id IN "
                 "(SELECT id FROM room WHERE project_id = :p)",
        "rack": "SELECT COUNT(*) FROM rack WHERE room_id IN "
                "(SELECT id FROM room WHERE project_id = :p)",
        "device_model": "SELECT COUNT(*) FROM device_model WHERE project_id = :p",
        "device_model_port": "SELECT COUNT(*) FROM device_model_port WHERE model_id IN "
                             "(SELECT id FROM device_model WHERE project_id = :p)",
        "device_instance": "SELECT COUNT(*) FROM device_instance WHERE project_id = :p",
        "port": "SELECT COUNT(*) FROM port WHERE device_id IN "
                "(SELECT id FROM device_instance WHERE project_id = :p)",
        "cable": "SELECT COUNT(*) FROM cable WHERE project_id = :p",
        "switch_service_range": "SELECT COUNT(*) FROM switch_service_range WHERE project_id = :p",
        "device_conflict": "SELECT COUNT(*) FROM device_conflict WHERE project_id = :p",
    }
    with engine.connect() as con:
        return {name: int(con.execute(text(sql), {"p": project_id}).scalar_one())
                for name, sql in queries.items()}


__all__ = [
    "CLEAR_ORDER",
    "aisle_id",
    "cable_id",
    "canonical_counts",
    "conflict_id",
    "device_id",
    "model_id",
    "model_port_id",
    "persist_canonical",
    "port_id",
    "rack_id",
    "range_id",
    "room_id",
]
