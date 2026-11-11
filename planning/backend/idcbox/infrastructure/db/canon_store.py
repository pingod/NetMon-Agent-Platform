"""规范台账落库：把 `domain/ports/canon.py` 摊平的平面记录写进 schema.sql 的规范表。

## 这一层存在的理由

v0.5 的台账是**现算的**（每次请求都从意图重新展开一遍），这在三个地方不够用：

1. 没法按状态/角色/批次筛选 —— 「哪些线上周刚改过」查不出来
2. 没法记录人的改动 —— 人工核定的线长、改成 reserve 的口、标记 moved 的设备
3. 没法回答「这根线什么时候改过」—— 没有行就没有 created_at

落库之后这三件事都成立。但**落库不是「意图的第二份真相」**：
意图文档（`intent_document.doc_json`）仍是唯一真相源，本表是它的投影，
每次 `write()` 都是全量重建（先删后插），不做增量 —— 因为
949 行意图摊平出 31k 行规范记录只要 ~300ms，而增量同步的 bug
（漏删、错配、级联顺序）比省下的 300ms 贵得多。

## 四条必须做对的约定（每条都有对应的实测依据）

### ★1 `core` 分类要落成 `switch` + `role_hint='core'`
`schema.sql` 的 `device_model.category` CHECK 只允许
`switch/server/security/balancer/storage/other` —— **没有 `core`**。
但 `canon_models()` 给出 6 个 `core`（业务核心/存储核心/管理核心交换机）。
直接写会 `CHECK constraint failed`。

选 `category='switch' + role_hint='core'` 而不是去改 CHECK：
分类是**采购维度**（交换机就是交换机），「核心」是**角色维度**
（同样一台 S7706 可以做核心也可以做接入）。混在一列里，
以后想按「核心交换机一共几台」统计就得靠字符串匹配。

### ★2 确定性 ID，不用 UUID
`device_key = (通道,机柜,U位,型号)` → `dev_` + sha1 → 前 12 位。
理由：①重复写入幂等，同一份意图永远得到同一批 ID ②测试可以直接算期望 ID，
不必先查库 ③外键在纯函数层就能接上，落库只是照抄。

UUID 的唯一好处是「不冲突」，而这里的自然键本来就是唯一的。
`project`/`intent_document` 用 UUID 是因为用户起的项目名会改名，
而机柜位置不会。

### ★3 先删后插，且删除顺序不能错
`cable.a_port_id REFERENCES port(id)` **没有 ON DELETE CASCADE**，
`device_instance.rack_id REFERENCES rack` 也没有 —— 这两处必须手工按
依赖倒序删：cable → switch_service_range → device_conflict → port →
device_instance → device_model_port → device_model → rack → aisle → room。
顺序错了会 `FOREIGN KEY constraint failed`，而且报错只说是哪张表，
不说是哪个引用 —— 所以顺序写死在 `_DELETE_ORDER` 里，不在各方法里散着。

### ★4 长度字段留空，本层不负责算
`cable` 有 15 个长度相关列（N/O/P/Q/R/S/T/U/policy/confidence…），
本层只写结构（两端、线缆类型、模块、备注），长度全部留 NULL。
理由：长度是 `domain/cable_length/` 的事，它有四策略、经验表、
置信度门控，需要 `LengthRule` + `GeometrySnapshot` + `EmpiricalTable` 三样输入。
把长度也算进 `write()` 会让这个函数的签名膨胀到 7 个参数，
且没法回答「只重算长度、不动结构」这种常见操作。
"""

from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass, field
from typing import Any, Iterable, Mapping, Sequence

from sqlalchemy import Engine, text

from ...domain.importing.normalize import norm_name
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
    CanonicalBundle,
)
from .session import utcnow_iso

#: 删除顺序：必须按外键依赖倒序，见模块 docstring ★3。
_DELETE_ORDER: tuple[str, ...] = (
    "cable",
    "switch_service_range",
    "device_conflict",
    "port",
    "device_instance",
    "device_model_port",
    "device_model",
    "rack",
    "aisle",
    "room",
)

#: `device_model.category` 的合法值（schema.sql CHECK 原文）。
_DB_CATEGORIES = frozenset({"switch", "server", "security", "balancer", "storage", "other"})

#: 型号为空时用的占位型号名。
#:
#: 真实数据里恰好一台：`带外交换机-2`（通道1 机柜23 34U），源表里它作为本端
#: 0 行、作为对端 2 行且对端型号列也是空。`device_instance.model_id` 是
#: `NOT NULL REFERENCES device_model` —— 不给它建一个型号行，这台的 2 个端口
#: 和 2 条连线就都写不进去，而且**不会报错**（`_insert_devices` 静默跳过）。
#:
#: 用占位型号而不是「跳过这台」：跳过等于把「这台型号没填」这个事实藏起来，
#: 而它正是需要被看见、需要人去补的那台。
UNKNOWN_MODEL_NAME = "（型号未填）"
_UNKNOWN_MODEL_NORMALIZED = norm_name(UNKNOWN_MODEL_NAME)


def _sid(prefix: str, *parts: Any) -> str:
    """确定性短 ID：`prefix_` + 自然键的 sha1 前 12 位。见模块 docstring ★2。"""
    raw = "|".join(str(p) for p in parts)
    return f"{prefix}_{hashlib.sha1(raw.encode('utf-8')).hexdigest()[:12]}"


def room_id(project_id: str, code: str) -> str:
    return _sid("rm", project_id, code)


def aisle_id(project_id: str, room_code: str, channel_no: int) -> str:
    return _sid("ais", project_id, room_code, channel_no)


def rack_id(project_id: str, room_code: str, channel_no: int, side: str, seq_no: int) -> str:
    return _sid("rk", project_id, room_code, channel_no, side, seq_no)


def model_id(project_id: str, name_normalized: str) -> str:
    return _sid("mdl", project_id, name_normalized)


def model_port_id(project_id: str, name_normalized: str, physical_name: str) -> str:
    return _sid("mpt", project_id, name_normalized, physical_name)


def device_id(project_id: str, key: tuple[int, int, int, str]) -> str:
    return _sid("dev", project_id, *key)


def port_id(project_id: str, key: tuple[int, int, int, str], physical_name: str) -> str:
    return _sid("prt", project_id, *key, physical_name)


def cable_id(project_id: str, stable_key: str, occurrence: int) -> str:
    return _sid("cbl", project_id, stable_key, occurrence)


def conflict_id(project_id: str, kind: str, channel_no: int, rack_no: int, u_start: int) -> str:
    return _sid("cfl", project_id, kind, channel_no, rack_no, u_start)


def range_id(project_id: str, key: tuple[int, int, int, str], model: str, base: int) -> str:
    return _sid("rng", project_id, *key, model, base)


def db_category(category: str) -> str:
    """`core` → `switch`。见模块 docstring ★1。"""
    return category if category in _DB_CATEGORIES else "switch"


def db_role_hint(category: str) -> str | None:
    """`core` 落进 `role_hint` 保留角色语义。见模块 docstring ★1。"""
    return "core" if category == "core" else None


@dataclass(frozen=True, slots=True)
class WriteStats:
    """一次全量重建写了多少行。前端/CLI 用它做「导入完成」提示。"""

    rooms: int = 0
    aisles: int = 0
    racks: int = 0
    models: int = 0
    model_ports: int = 0
    devices: int = 0
    ports: int = 0
    cables: int = 0
    ranges: int = 0
    conflicts: int = 0
    deleted: Mapping[str, int] = field(default_factory=dict)

    @property
    def total(self) -> int:
        return (
            self.rooms + self.aisles + self.racks + self.models + self.model_ports
            + self.devices + self.ports + self.cables + self.ranges + self.conflicts
        )

    def describe(self) -> str:
        return (
            f"房间 {self.rooms} / 通道 {self.aisles} / 机柜 {self.racks} / "
            f"型号 {self.models} / 型号端口 {self.model_ports} / 设备 {self.devices} / "
            f"端口 {self.ports} / 连线 {self.cables} / 接线区间 {self.ranges} / "
            f"冲突 {self.conflicts}（合计 {self.total} 行）"
        )


class CanonicalStore:
    """规范台账的读写。写是全量重建，读只有几个统计查询。

    ## 为什么读这么少

    界面要的设备明细、端口明细、按型号汇总，全都从 `PortLedger` 现算
    （v0.5 已经做好），因为它们要的是「按角色分组」「空闲口列表」这类
    派生视图，SQL 反而更啰嗦。这里落库是为了**筛选与审计**，
    不是为了替代现算 —— 所以只提供计数与按状态/角色的聚合。
    """

    def __init__(self, engine: Engine) -> None:
        self._engine = engine
        #: `(通道, 柜号) → rack_id`，由 `write()` 填充，供设备/冲突查机柜用。
        self._rack_ids_by_seq: Mapping[tuple[int, int], str] = {}

    # ---------- 写 ----------

    def clear(self, project_id: str) -> dict[str, int]:
        """删掉该项目下的全部规范记录。返回每张表删了多少行。

        ## 为什么不用 `DELETE FROM room` 靠级联
        `rack` / `device_instance` / `cable` 三处外键都没写 ON DELETE CASCADE，
        靠级联会中途断掉。而且「每张表删了多少」这个信息本身有价值：
        CLI 输出「清理旧台账：端口 12820 行」比「清理完成」可信。
        """
        counts: dict[str, int] = {}
        with self._engine.begin() as conn:
            for table in _DELETE_ORDER:
                if table in ("cable", "switch_service_range", "device_conflict", "device_instance"):
                    n = conn.execute(
                        text(f"DELETE FROM {table} WHERE project_id = :pid"), {"pid": project_id}
                    ).rowcount
                elif table == "port":
                    n = conn.execute(
                        text(
                            "DELETE FROM port WHERE device_id IN"
                            " (SELECT id FROM device_instance WHERE project_id = :pid)"
                        ),
                        {"pid": project_id},
                    ).rowcount
                elif table == "device_model_port":
                    n = conn.execute(
                        text(
                            "DELETE FROM device_model_port WHERE model_id IN"
                            " (SELECT id FROM device_model WHERE project_id = :pid)"
                        ),
                        {"pid": project_id},
                    ).rowcount
                elif table == "device_model":
                    n = conn.execute(
                        text("DELETE FROM device_model WHERE project_id = :pid"), {"pid": project_id}
                    ).rowcount
                elif table == "rack":
                    n = conn.execute(
                        text(
                            "DELETE FROM rack WHERE room_id IN"
                            " (SELECT id FROM room WHERE project_id = :pid)"
                        ),
                        {"pid": project_id},
                    ).rowcount
                elif table == "aisle":
                    n = conn.execute(
                        text(
                            "DELETE FROM aisle WHERE room_id IN"
                            " (SELECT id FROM room WHERE project_id = :pid)"
                        ),
                        {"pid": project_id},
                    ).rowcount
                else:  # room
                    n = conn.execute(
                        text("DELETE FROM room WHERE project_id = :pid"), {"pid": project_id}
                    ).rowcount
                counts[table] = max(n, 0)
        return counts

    def write(self, project_id: str, bundle: CanonicalBundle, *, note: str = "") -> WriteStats:
        """全量重建：先清空该项目，再按依赖顺序插入。单个事务，失败即回滚。

        `note` 只用于日志（写进 `audit_log` 会污染版本语义，这里不写）。
        """
        deleted = self.clear(project_id)
        now = utcnow_iso()

        room_ids = {r.code: room_id(project_id, r.code) for r in bundle.rooms}
        aisle_ids = {
            a.channel_no: aisle_id(project_id, a.room_code, a.channel_no)
            for a in bundle.aisles
        }
        # 两个索引：完整键 `(通道, 侧别, 柜号)` 与省略侧别的 `(通道, 柜号)`。
        #
        # 为什么要第二个：`CanonDevice.side` 目前恒为 "N"（canon 层没有几何快照，
        # 无法判断设备在通道哪一侧），而实测**同一通道内柜号唯一确定侧别**
        # （14 个通道全部满足，无一例外），所以 `(通道, 柜号)` 足以定位机柜。
        # 只留完整键会让 368 台在 "S" 侧机柜里的设备找不到 rack_id 被静默跳过。
        rack_ids = {
            (r.channel_no, r.side, r.seq_no): rack_id(
                project_id, r.room_code, r.channel_no, r.side, r.seq_no
            )
            for r in bundle.racks
        }
        rack_ids_by_seq = {
            (r.channel_no, r.seq_no): rack_id(
                project_id, r.room_code, r.channel_no, r.side, r.seq_no
            )
            for r in bundle.racks
        }
        model_ids = {
            m.name_normalized: model_id(project_id, m.name_normalized)
            for m in bundle.models
        }

        # 有设备用了不在型号表里的型号（型号为空，或没进 `canon_models()`）
        # → 补一个占位型号行，见 `UNKNOWN_MODEL_NAME` 的说明。
        extra_models: list[CanonModel] = []
        if any(d.model_normalized not in model_ids for d in bundle.devices):
            model_ids[_UNKNOWN_MODEL_NORMALIZED] = model_id(project_id, _UNKNOWN_MODEL_NORMALIZED)
            extra_models.append(
                CanonModel(
                    name=UNKNOWN_MODEL_NAME,
                    name_normalized=_UNKNOWN_MODEL_NORMALIZED,
                    category="other",
                    spec_text="源表未填型号，端口规格未知",
                    u_height=1.0,
                )
            )
        self._rack_ids_by_seq = rack_ids_by_seq

        # 只保留「型号与机柜都能对上」的设备 —— 这些才是真能插入的。
        #
        # 不这么做的后果：`dev_ids` 里有全部 824 台，但 `_insert_devices`
        # 会静默跳过对不上的（实测 1 台型号为空），随后 `port` 插入引用
        # 那台的 id → `FOREIGN KEY constraint failed`，而报错只说是 port 表，
        # 完全看不出是哪台设备的问题。
        dev_ids = {
            d.key: device_id(project_id, d.key)
            for d in bundle.devices
            if (
                (d.channel_no, d.side, d.rack_no) in rack_ids
                or (d.channel_no, d.rack_no) in rack_ids_by_seq
            )
        }

        with self._engine.begin() as conn:
            self._insert_rooms(conn, project_id, bundle.rooms, room_ids)
            self._insert_aisles(conn, bundle.aisles, room_ids, aisle_ids)
            self._insert_racks(conn, bundle.racks, room_ids, aisle_ids, rack_ids)
            self._insert_models(conn, project_id, (*bundle.models, *extra_models), model_ids)
            self._insert_model_ports(conn, project_id, bundle.model_ports, model_ids)
            self._insert_devices(conn, project_id, bundle.devices, model_ids, rack_ids, dev_ids, now)
            self._insert_ports(conn, project_id, bundle.ports, dev_ids)
            self._insert_cables(conn, project_id, bundle.cables, dev_ids, now)
            self._insert_ranges(conn, project_id, bundle.ranges, model_ids, dev_ids)
            self._insert_conflicts(conn, project_id, bundle.conflicts, rack_ids, dev_ids)

        return WriteStats(
            rooms=len(bundle.rooms),
            aisles=len(bundle.aisles),
            racks=len(bundle.racks),
            models=len(bundle.models) + len(extra_models),
            model_ports=len(bundle.model_ports),
            devices=len(bundle.devices),
            ports=len(bundle.ports),
            cables=len(bundle.cables),
            ranges=len(bundle.ranges),
            conflicts=len(bundle.conflicts),
            deleted=deleted,
        )

    # ---------- 各表插入 ----------

    def _insert_rooms(
        self, conn: Any, project_id: str, rooms: Sequence[CanonRoom], ids: Mapping[str, str]
    ) -> None:
        if not rooms:
            return
        conn.execute(
            text(
                "INSERT INTO room (id, project_id, name, code, col_min, col_max,"
                " power_kw_per_rack, declared_rack_count) VALUES"
                " (:id, :pid, :name, :code, :cmin, :cmax, :kw, :cnt)"
            ),
            [
                {
                    "id": ids[r.code], "pid": project_id, "name": r.name, "code": r.code,
                    "cmin": r.col_min, "cmax": r.col_max, "kw": 4.0, "cnt": None,
                }
                for r in rooms
            ],
        )

    def _insert_aisles(
        self,
        conn: Any,
        aisles: Sequence[CanonAisle],
        room_ids: Mapping[str, str],
        ids: Mapping[int, str],
    ) -> None:
        if not aisles:
            return
        conn.execute(
            text(
                "INSERT INTO aisle (id, room_id, channel_no, name, plan_row_n, plan_row_s)"
                " VALUES (:id, :rid, :ch, :name, :rn, :rs)"
            ),
            [
                {
                    "id": ids[a.channel_no], "rid": room_ids[a.room_code], "ch": a.channel_no,
                    "name": f"{a.channel_no}通道", "rn": a.plan_row_n, "rs": a.plan_row_s,
                }
                for a in aisles
            ],
        )

    def _insert_racks(
        self,
        conn: Any,
        racks: Sequence[CanonRack],
        room_ids: Mapping[str, str],
        aisle_ids: Mapping[int, str],
        ids: Mapping[tuple[int, str, int], str],
    ) -> None:
        if not racks:
            return
        conn.execute(
            text(
                "INSERT INTO rack (id, room_id, aisle_id, seq_no, side, display_name,"
                " col_index, row_index, status) VALUES"
                " (:id, :rid, :aid, :seq, :side, :name, :col, :row, 'planned')"
            ),
            [
                {
                    "id": ids[(r.channel_no, r.side, r.seq_no)],
                    "rid": room_ids[r.room_code],
                    "aid": aisle_ids[r.channel_no],
                    "seq": r.seq_no, "side": r.side, "name": r.display_name,
                    "col": r.col_index, "row": r.row_index,
                }
                for r in racks
            ],
        )

    def _insert_models(
        self, conn: Any, project_id: str, models: Sequence[CanonModel], ids: Mapping[str, str]
    ) -> None:
        if not models:
            return
        conn.execute(
            text(
                "INSERT INTO device_model (id, project_id, name, name_normalized, category,"
                " role_hint, u_height, spec_text) VALUES"
                " (:id, :pid, :name, :norm, :cat, :role, :uh, :spec)"
            ),
            [
                {
                    "id": ids[m.name_normalized], "pid": project_id, "name": m.name,
                    "norm": m.name_normalized, "cat": db_category(m.category),
                    "role": db_role_hint(m.category), "uh": m.u_height, "spec": m.spec_text,
                }
                for m in models
            ],
        )

    def _insert_model_ports(
        self,
        conn: Any,
        project_id: str,
        ports: Sequence[CanonModelPort],
        model_ids: Mapping[str, str],
    ) -> None:
        if not ports:
            return
        conn.execute(
            text(
                "INSERT INTO device_model_port (id, model_id, seq, physical_name, display_name,"
                " speed_mbps, connector, port_type, panel_slot, panel_row, panel_col)"
                " VALUES (:id, :mid, :seq, :pname, :pname, :spd, :conn, :ptype,"
                " :pslot, :prow, :pcol)"
            ),
            [
                {
                    "id": model_port_id(project_id, p.model_normalized, p.physical_name),
                    "mid": model_ids[p.model_normalized], "seq": p.seq,
                    "pname": p.physical_name, "spd": p.speed_mbps, "conn": p.connector,
                    "ptype": p.port_type, "pslot": p.panel_slot,
                    "prow": p.panel_row, "pcol": p.panel_col,
                }
                for p in ports
            ],
        )

    def _insert_devices(
        self,
        conn: Any,
        project_id: str,
        devices: Sequence[CanonDevice],
        model_ids: Mapping[str, str],
        rack_ids: Mapping[tuple[int, str, int], str],
        dev_ids: Mapping[tuple[int, int, int, str], str],
        now: str,
    ) -> None:
        if not devices:
            return
        rows: list[dict[str, Any]] = []
        for d in devices:
            mid = model_ids.get(d.model_normalized) or model_ids.get(_UNKNOWN_MODEL_NORMALIZED)
            rid = rack_ids.get((d.channel_no, d.side, d.rack_no)) or self._rack_ids_by_seq.get(
                (d.channel_no, d.rack_no)
            )
            if mid is None or rid is None:
                # 型号或机柜缺失时跳过：真实数据上是 0 条；
                # 强行插入会因外键失败把整批事务回滚掉。
                continue
            rows.append(
                {
                    "id": dev_ids[d.key], "pid": project_id, "mid": mid, "rid": rid,
                    "us": d.u_start, "ue": d.u_end,
                    # ★5：name 可空。绝不用位置号填充。
                    "name": d.name or None,
                    "norm": d.name_normalized or None,
                    "role": "core" if d.category == "core" else d.category,
                    "status": d.status, "at": now,
                }
            )
        if not rows:
            return
        conn.execute(
            text(
                "INSERT INTO device_instance (id, project_id, model_id, rack_id, u_start, u_end,"
                " name, name_normalized, role, status, first_seen_at, updated_at) VALUES"
                " (:id, :pid, :mid, :rid, :us, :ue, :name, :norm, :role, :status, :at, :at)"
            ),
            rows,
        )

    def _insert_ports(
        self,
        conn: Any,
        project_id: str,
        ports: Sequence[CanonPort],
        dev_ids: Mapping[tuple[int, int, int, str], str],
    ) -> None:
        if not ports:
            return
        rows: list[dict[str, Any]] = []
        for p in ports:
            did = dev_ids.get(p.device_key)
            if did is None:
                continue
            rows.append(
                {
                    "id": port_id(project_id, p.device_key, p.physical_name),
                    "did": did, "pname": p.physical_name, "dname": p.display_name,
                    # speed_mbps 是 NOT NULL：未知速率写 0（真实数据里 0 条）
                    "spd": p.speed_mbps if p.speed_mbps is not None else 0,
                    "conn": p.connector or "OTHER", "ptype": p.port_type or "optical",
                    "role": p.port_role, "status": p.status,
                    "pslot": p.panel_slot, "prow": p.panel_row, "pcol": p.panel_col,
                }
            )
        if not rows:
            return
        conn.execute(
            text(
                "INSERT INTO port (id, device_id, physical_name, display_name, speed_mbps,"
                " connector, port_type, port_role, status, panel_slot, panel_row, panel_col)"
                " VALUES (:id, :did, :pname, :dname, :spd, :conn, :ptype, :role, :status,"
                " :pslot, :prow, :pcol)"
            ),
            rows,
        )

    def _insert_cables(
        self,
        conn: Any,
        project_id: str,
        cables: Sequence[CanonCable],
        dev_ids: Mapping[tuple[int, int, int, str], str],
        now: str,
    ) -> None:
        if not cables:
            return
        rows: list[dict[str, Any]] = []
        for c in cables:
            if c.a_device_key not in dev_ids or c.z_device_key not in dev_ids:
                continue
            rows.append(
                {
                    "id": cable_id(project_id, c.stable_key, c.occurrence),
                    "pid": project_id, "skey": c.stable_key, "occ": c.occurrence,
                    "apid": port_id(project_id, c.a_device_key, c.a_port_name),
                    "zpid": port_id(project_id, c.z_device_key, c.z_port_name),
                    "ctype": c.cable_type or "UNKNOWN",
                    "amod": c.a_module or None, "zmod": c.z_module or None,
                    "remark": c.remark or None, "at": now,
                }
            )
        if not rows:
            return
        conn.execute(
            text(
                "INSERT INTO cable (id, project_id, stable_key, occurrence, a_port_id, z_port_id,"
                " cable_type, a_module, z_module, remark, created_at, updated_at) VALUES"
                " (:id, :pid, :skey, :occ, :apid, :zpid, :ctype, :amod, :zmod, :remark, :at, :at)"
            ),
            rows,
        )

    def _insert_ranges(
        self,
        conn: Any,
        project_id: str,
        ranges: Sequence[CanonRange],
        model_ids: Mapping[str, str],
        dev_ids: Mapping[tuple[int, int, int, str], str],
    ) -> None:
        if not ranges:
            return
        rows: list[dict[str, Any]] = []
        for r in ranges:
            sid = dev_ids.get(r.switch_device_key)
            mid = model_ids.get(r.node_model_normalized)
            if sid is None or mid is None:
                continue
            rows.append(
                {
                    "id": range_id(project_id, r.switch_device_key, r.node_model_normalized, r.base_port_no),
                    "pid": project_id, "sid": sid, "mid": mid, "base": r.base_port_no,
                    "rule": r.sort_rule, "role": r.port_role, "cnt": r.node_count_expected,
                }
            )
        if not rows:
            return
        conn.execute(
            text(
                "INSERT INTO switch_service_range (id, project_id, switch_device_id,"
                " node_model_id, base_port_no, sort_rule, port_role, node_count_expected)"
                " VALUES (:id, :pid, :sid, :mid, :base, :rule, :role, :cnt)"
            ),
            rows,
        )

    def _insert_conflicts(
        self,
        conn: Any,
        project_id: str,
        conflicts: Sequence[CanonConflict],
        rack_ids: Mapping[tuple[int, str, int], str],
        dev_ids: Mapping[tuple[int, int, int, str], str],
    ) -> None:
        if not conflicts:
            return
        rows: list[dict[str, Any]] = []
        for c in conflicts:
            # 冲突没有 side 信息，两个侧别都试一遍；都不中就只记位置不记 rack_id
            rid = rack_ids.get((c.channel_no, "N", c.rack_no)) or rack_ids.get(
                (c.channel_no, "S", c.rack_no)
            )
            rid = rid or self._rack_ids_by_seq.get((c.channel_no, c.rack_no))
            rows.append(
                {
                    "id": conflict_id(project_id, c.kind, c.channel_no, c.rack_no, c.u_start),
                    "pid": project_id, "kind": c.kind, "rid": rid,
                    "us": c.u_start, "ue": c.u_end,
                    "detail": json.dumps(
                        {"detail": c.detail, "channel": c.channel_no, "rack": c.rack_no,
                         "uStart": c.u_start, "uEnd": c.u_end},
                        ensure_ascii=False,
                    ),
                    "sev": c.severity,
                }
            )
        if not rows:
            return
        conn.execute(
            text(
                "INSERT INTO device_conflict (id, project_id, kind, rack_id, u_start, u_end,"
                " detail_json, severity) VALUES (:id, :pid, :kind, :rid, :us, :ue, :detail, :sev)"
            ),
            rows,
        )

    # ---------- 读 ----------

    def counts(self, project_id: str) -> dict[str, int]:
        """每张表该项目有多少行。CLI/API 用它确认「真的落库了」。"""
        queries: Mapping[str, str] = {
            "rooms": "SELECT COUNT(*) FROM room WHERE project_id = :pid",
            "aisles": (
                "SELECT COUNT(*) FROM aisle WHERE room_id IN"
                " (SELECT id FROM room WHERE project_id = :pid)"
            ),
            "racks": (
                "SELECT COUNT(*) FROM rack WHERE room_id IN"
                " (SELECT id FROM room WHERE project_id = :pid)"
            ),
            "models": "SELECT COUNT(*) FROM device_model WHERE project_id = :pid",
            "modelPorts": (
                "SELECT COUNT(*) FROM device_model_port WHERE model_id IN"
                " (SELECT id FROM device_model WHERE project_id = :pid)"
            ),
            "devices": "SELECT COUNT(*) FROM device_instance WHERE project_id = :pid",
            "ports": (
                "SELECT COUNT(*) FROM port WHERE device_id IN"
                " (SELECT id FROM device_instance WHERE project_id = :pid)"
            ),
            "cables": "SELECT COUNT(*) FROM cable WHERE project_id = :pid",
            "ranges": "SELECT COUNT(*) FROM switch_service_range WHERE project_id = :pid",
            "conflicts": "SELECT COUNT(*) FROM device_conflict WHERE project_id = :pid",
        }
        with self._engine.connect() as conn:
            return {
                key: conn.execute(text(sql), {"pid": project_id}).scalar_one()
                for key, sql in queries.items()
            }

    def port_status_counts(self, project_id: str) -> dict[str, int]:
        """按端口状态分组。这是「落库之后才查得出来」的第一类问题。"""
        with self._engine.connect() as conn:
            rows = conn.execute(
                text(
                    "SELECT p.status AS st, COUNT(*) AS n FROM port p"
                    " JOIN device_instance d ON d.id = p.device_id"
                    " WHERE d.project_id = :pid GROUP BY p.status ORDER BY n DESC"
                ),
                {"pid": project_id},
            ).all()
        return {str(r.st): int(r.n) for r in rows}

    def port_role_counts(self, project_id: str) -> dict[str, int]:
        with self._engine.connect() as conn:
            rows = conn.execute(
                text(
                    "SELECT p.port_role AS r, COUNT(*) AS n FROM port p"
                    " JOIN device_instance d ON d.id = p.device_id"
                    " WHERE d.project_id = :pid GROUP BY p.port_role ORDER BY n DESC"
                ),
                {"pid": project_id},
            ).all()
        return {str(r.r): int(r.n) for r in rows}

    def cable_type_counts(self, project_id: str) -> dict[str, int]:
        with self._engine.connect() as conn:
            rows = conn.execute(
                text(
                    "SELECT cable_type, COUNT(*) FROM cable"
                    " WHERE project_id = :pid GROUP BY cable_type ORDER BY COUNT(*) DESC"
                ),
                {"pid": project_id},
            ).all()
        # 用位置索引（`r[0]`/`r[1]`）而不是属性名：带别名的 `AS t` 在
        # SQLAlchemy 2.0 的 Row 上偶尔会解析到整行，实测得到
        # `{"('LC-LC', 2683)": 2683}` 这种键。
        return {str(r[0]): int(r[1]) for r in rows}

    def devices_by_state(self, project_id: str) -> dict[str, int]:
        with self._engine.connect() as conn:
            rows = conn.execute(
                text(
                    "SELECT status AS s, COUNT(*) AS n FROM device_instance"
                    " WHERE project_id = :pid GROUP BY status ORDER BY n DESC"
                ),
                {"pid": project_id},
            ).all()
        return {str(r.s): int(r.n) for r in rows}

    def open_conflicts(self, project_id: str, *, limit: int = 50) -> list[dict[str, Any]]:
        with self._engine.connect() as conn:
            rows = conn.execute(
                text(
                    "SELECT id, kind, rack_id, u_start, u_end, detail_json, severity"
                    " FROM device_conflict WHERE project_id = :pid AND resolution = 'open'"
                    " ORDER BY kind, u_start LIMIT :lim"
                ),
                {"pid": project_id, "lim": limit},
            ).all()
        out: list[dict[str, Any]] = []
        for r in rows:
            detail: Any = {}
            try:
                detail = json.loads(r.detail_json)
            except (TypeError, ValueError):  # pragma: no cover - 库里的 JSON 是我们自己写的
                detail = {}
            out.append(
                {
                    "id": r.id, "kind": r.kind, "rackId": r.rack_id,
                    "uStart": r.u_start, "uEnd": r.u_end,
                    "severity": r.severity, "detail": detail,
                }
            )
        return out


__all__ = [
    "CanonicalStore",
    "WriteStats",
    "aisle_id",
    "cable_id",
    "conflict_id",
    "db_category",
    "db_role_hint",
    "device_id",
    "model_id",
    "model_port_id",
    "port_id",
    "rack_id",
    "range_id",
    "room_id",
]
