"""意图 → 规范台账（纯函数，零 IO）。

## 这一层解决什么问题

v0.5 的设备台账是**从意图现算**的：每次请求重算 830 台设备、12820 个端口，
算完就扔。这有三个后果：

1. 没法按端口状态、业务角色、批次筛选 —— 想查「所有 free 的 100GE 口」
   得把全部数据算出来再在内存里过滤；
2. 没法记录**人的改动**（改一个 U 位要重算全量，且没有行可更新）；
3. 没法回答「这根线什么时候改过」—— 没有行，就没有时间。

`schema.sql` 里的 `device_instance` / `port` / `cable` /
`switch_service_range` / `device_conflict` 就是为此准备的。
本模块把意图摊平成**可以直接写进那些表的平面记录**。

## 为什么在域层而不在基础设施层

「意图 → 规范行」的每条换算都带业务判断（见下面六个 ★），不是字段搬运。
放进基础设施层，这些判断会跟 SQL 混在一起：既没法单测，
也没法在 CLI / API 之外复用（比如将来的差异导入、跨项目比对）。

## 六个必须做对的地方（每条都有实测依据）

### ★1 设备身份必须先合并命名变体，再落库

实测 830 台设备里有 **6 组**是「同一台设备被记了两个名字」：
`备份-1` / `备份一体机-1`、`tgw-1` / `tpgw-1` 这类。同一个
`(通道, 机柜, U位)` 出现两次，且**端口集互补不重叠** ——
一台设备的一半端口按一个名字记、另一半按另一个名字记。

不合并的后果：这台设备在库里变成两台，各占一半端口，
台账上看起来「两台都只用了一半」，实际一台快满了。

### ★2 但也有 3 组是「真叠放」，绝不能合并

`ch2/ch8/ch9` 的 `rack11` 的 **31U** 上各有一对
`N02-11-S7` / `N02-11-S7-1`，**端口集完全相同**。这是意图里
`slot_map` 把 slot 7 声明两次（`[0..7,7]`）的真实语义：规划上同 U 位放两台。
它们落 `device_conflict(kind='U_OVERLAP')`。

**判据**：端口集有交集 → 真叠放（保留两台 + 冲突记录）；
无交集 → 命名变体（合并成一台）。

### ★3 `switch_service_range` 按型号拆行，不能一块一行

实测 106 个接线块里有的一台交换机同时接两种型号的节点
（`计算接入交换机(业务)-5` 的 44 个口里既有 `计算节点A（ARM）`
又有 `计算节点B（C86）`）。按「一块一行」写会违反
`UNIQUE(project_id, switch_device_id, node_model_id, base_port_no)`；
按 `(块 × 型号)` 拆开是 **142 行**。

### ★4 cable 的 a 侧唯一、z 侧可多根

实测 a 侧每个端口恰好 1 根线（`uq_cable_a_active` 唯一索引 100% 成立），
z 侧只有 1 处一对多：`分线接入交换机-2` 的 `25GE1/0/48` 接了 3 根 ——
那是 MPO 一分三的真实表达。所以 z 侧不能加唯一约束。

### ★5 服务器没有名字，`name` 必须可空

实测 824 个设备位置里 631 个没有实例名（全是服务器）。
本模块用槽位号（`N02-11-S7`）当设备键，但 `name` 留空 ——
把位置号写进 `name` 会让「按名字查设备」永远返回位置号串。

### ★6 端口角色取模板的名义角色，不取实际用途

同一个物理口可能既是 uplink 又是 peer_link（核心交换机的第 3 块
100GE 板）。模板声明的是**设计意图**，实际接了什么由 `cable` 表回答。
所以 `port.port_role` 写模板值，不做「按实际连线反推」。
"""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass, field
from typing import Iterable, Mapping, Sequence

from ..geometry.floor import floor_row, floor_room_of
from ..geometry.layout import GeometrySnapshot
from ..importing.normalize import norm_name, u_position_span
from ..units import ROOM_403_COLS, ROOM_404_COLS
from .ledger import DevicePorts, PortLedger
from .modellib import ModelLibrary
from .portspec import PortSpec, all_models


# ---------------------------------------------------------------------------
# 平面记录：字段与 schema.sql 一一对应，但用自然键而不是 UUID
# ---------------------------------------------------------------------------
#: 用自然键而不是 `prj_xxx` 这类随机 ID，是因为「谁指向谁」在意图里本来就是
#: 自然键表达的（`(通道,机柜,U位)` 定位一台设备、`(设备键,端口名)` 定位一个口）。
#: 若在这一层就生成 UUID，测试断言得先查库才能验证，而这里应该是纯函数。

DeviceKey = tuple[int, int, int, str]
"""`(通道, 机柜, 起始U, 型号归一名)` —— 设备的物理身份。

不含实例名：实测 824 个位置里 631 个没有名字，名字不能当键（★5）。
"""


@dataclass(frozen=True, slots=True)
class CanonRoom:
    code: str
    name: str
    col_min: int
    col_max: int


@dataclass(frozen=True, slots=True)
class CanonAisle:
    room_code: str
    channel_no: int
    plan_row_n: int
    plan_row_s: int


@dataclass(frozen=True, slots=True)
class CanonRack:
    room_code: str
    channel_no: int
    seq_no: int
    side: str
    col_index: int
    row_index: int
    display_name: str


@dataclass(frozen=True, slots=True)
class CanonModel:
    name: str
    name_normalized: str
    category: str
    spec_text: str
    u_height: float = 1.0


@dataclass(frozen=True, slots=True)
class CanonModelPort:
    model_normalized: str
    seq: int
    physical_name: str
    speed_mbps: int
    connector: str
    port_type: str
    panel_slot: int
    panel_row: int
    panel_col: int
    port_role: str


@dataclass(frozen=True, slots=True)
class CanonDevice:
    key: DeviceKey
    name: str
    """实例名。**服务器普遍为空**（★5），不要用位置号填充。"""
    name_normalized: str
    model: str
    model_normalized: str
    category: str
    channel_no: int
    rack_no: int
    side: str
    u_start: int
    u_end: int
    status: str = "planned"
    ledger_key: str = ""
    """台账里的原始设备键（交换机=实例名，服务器=槽位号 `N02-01-S0`）。

    `cable` 的端点必须靠它解析：服务器没有名字（★5），
    生成器给的是槽位号，只按 `name` 查会丢掉全部 4430 条服务器连线。
    """
    merged_from: tuple[str, ...] = ()
    """被合并掉的别名（★1）。空表示本来就没别名。"""

    @property
    def display_label(self) -> str:
        where = f"通道{self.channel_no} 机柜{self.rack_no} {self.u_start}U"
        return f"{self.name} @ {where}" if self.name else f"{self.model} @ {where}"

    @property
    def u_span(self) -> int:
        return self.u_end - self.u_start + 1


@dataclass(frozen=True, slots=True)
class CanonPort:
    device_key: DeviceKey
    physical_name: str
    display_name: str
    speed_mbps: int | None
    connector: str
    port_type: str
    port_role: str
    status: str
    """`occupied` / `free` / `unknown`。unknown 只给「归不进模板」的口。"""
    panel_slot: int | None
    panel_row: int | None
    panel_col: int | None


@dataclass(frozen=True, slots=True)
class CanonCable:
    stable_key: str
    occurrence: int
    a_device_key: DeviceKey
    a_port_name: str
    z_device_key: DeviceKey
    z_port_name: str
    cable_type: str
    a_module: str = ""
    z_module: str = ""
    remark: str = ""


@dataclass(frozen=True, slots=True)
class CanonRange:
    switch_device_key: DeviceKey
    node_model_normalized: str
    base_port_no: int
    node_count_expected: int
    sort_rule: str = "rack_then_u"
    port_role: str = "downlink"


@dataclass(frozen=True, slots=True)
class CanonConflict:
    kind: str
    """`U_OVERLAP` / `MODEL_CONFLICT` / `NAME_CONFLICT`。"""
    channel_no: int
    rack_no: int
    u_start: int
    u_end: int
    detail: str
    severity: str = "warn"


@dataclass(frozen=True, slots=True)
class CanonicalBundle:
    rooms: tuple[CanonRoom, ...]
    aisles: tuple[CanonAisle, ...]
    racks: tuple[CanonRack, ...]
    models: tuple[CanonModel, ...]
    model_ports: tuple[CanonModelPort, ...]
    devices: tuple[CanonDevice, ...]
    ports: tuple[CanonPort, ...]
    cables: tuple[CanonCable, ...]
    ranges: tuple[CanonRange, ...]
    conflicts: tuple[CanonConflict, ...]
    stats: Mapping[str, int] = field(default_factory=dict)

    @property
    def device_count(self) -> int:
        return len(self.devices)

    @property
    def port_count(self) -> int:
        return len(self.ports)

    def devices_by_key(self) -> dict[DeviceKey, CanonDevice]:
        return {d.key: d for d in self.devices}

    def describe(self) -> str:
        s = self.stats
        return (
            f"设备 {len(self.devices)} 台 / 端口 {len(self.ports)} / "
            f"连线 {len(self.cables)} / 接线区间 {len(self.ranges)} / "
            f"冲突 {len(self.conflicts)}（合并别名 {s.get('merged', 0)} 组）"
        )


# ---------------------------------------------------------------------------
# 机柜与几何（从 GeometrySnapshot）
# ---------------------------------------------------------------------------
_ROOM_COLS = {"403": ROOM_403_COLS, "404": ROOM_404_COLS}
_ROOM_NAMES = {"403": "403机房", "404": "404机房"}


def canon_rooms(geometry: GeometrySnapshot) -> tuple[CanonRoom, ...]:
    """房间表。列范围来自 units.py 的常量，不在这里写死。"""
    codes = sorted({p.room_code for p in geometry.racks.values()})
    return tuple(
        CanonRoom(
            code=code,
            name=_ROOM_NAMES.get(code, f"{code}机房"),
            col_min=_ROOM_COLS.get(code, (0, 0))[0],
            col_max=_ROOM_COLS.get(code, (0, 0))[1],
        )
        for code in codes
    )


def canon_aisles(geometry: GeometrySnapshot) -> tuple[CanonAisle, ...]:
    """通道表。`plan_row_n`/`plan_row_s` 用 `floor_row` 算，不抄平面图行号。

    抄行号的后果：平面图一旦调整（加一个空调位），库里存的行号全错，
    而 `floor_row` 的公式是「通道号 → 行号」，只要布局规则不变就永远对。
    """
    channels = sorted({p.channel_no for p in geometry.racks.values()})
    out: list[CanonAisle] = []
    for ch in channels:
        out.append(
            CanonAisle(
                room_code=floor_room_of(ch),
                channel_no=ch,
                plan_row_n=floor_row(ch, "N"),
                plan_row_s=floor_row(ch, "S"),
            )
        )
    return tuple(out)


def canon_racks(geometry: GeometrySnapshot) -> tuple[CanonRack, ...]:
    """机柜表。**362 个机柜位全部落库**，包括空柜 —— 空柜也是规划的一部分。"""
    out: list[CanonRack] = []
    for placement in sorted(
        geometry.racks.values(), key=lambda p: (p.channel_no, p.side, p.seq_no)
    ):
        out.append(
            CanonRack(
                room_code=placement.room_code,
                channel_no=placement.channel_no,
                seq_no=placement.seq_no,
                side=placement.side,
                col_index=placement.col,
                row_index=placement.row,
                display_name=f"{placement.room_code}-{placement.channel_no}-{placement.seq_no}",
            )
        )
    return tuple(out)


# ---------------------------------------------------------------------------
# 型号与端口模板（从 portspec）
# ---------------------------------------------------------------------------
_CONNECTOR_OF_GROUP = (
    ("100GE", "QSFP28"),
    ("40GE", "QSFP28"),
    ("25GE", "SFP28"),
    ("10GE", "SFP28"),
    ("10G", "SFP28"),
    ("GE", "RJ45"),
    ("千兆", "RJ45"),
    ("万兆", "SFP28"),
)


def _connector_of(group: str) -> str:
    for prefix, conn in _CONNECTOR_OF_GROUP:
        if group.startswith(prefix):
            return conn
    return "OTHER"


def _port_type_of(group: str) -> str:
    """电口 / 光口。GE 与「千兆」是电口（源表里这些口接 RJ45），其余光口。"""
    return "electrical" if group in ("GE", "千兆") else "optical"


def _u_height_of(category: str, total_ports: int) -> float:
    """机箱高度估计。核心交换机实测 U 位跨度 10U（4 台 `3-12U`），
    其余交换机 1~2U，服务器 2U。**只用于展示，不参与任何计算** ——
    真实 U 位来自 `device_instance.u_start/u_end`。"""
    if category == "core":
        return 10.0
    if category == "switch":
        return 1.0
    return 2.0


def canon_models(
    library: ModelLibrary | None = None,
) -> tuple[tuple[CanonModel, ...], tuple[CanonModelPort, ...]]:
    """型号表 + 型号端口表。26 个模板展开成 855 个端口行。"""
    models: list[CanonModel] = []
    ports: list[CanonModelPort] = []
    for spec in (library.all() if library is not None else all_models()):
        normalized = norm_name(spec.model)
        models.append(
            CanonModel(
                name=spec.model,
                name_normalized=normalized,
                category=spec.category,
                spec_text=spec.source_text,
                u_height=_u_height_of(spec.category, spec.total_ports),
            )
        )
        for seq, (group, panel, idx) in enumerate(spec.slots(), start=1):
            bay = next((b for b in spec.bays if b.group == group), None)
            if bay is None:  # pragma: no cover - slots() 的组必然来自 bays
                continue
            name = f"{group}{panel}/0/{idx}" if group.endswith("GE") else f"{group}{idx}"
            ports.append(
                CanonModelPort(
                    model_normalized=normalized,
                    seq=seq,
                    physical_name=name,
                    speed_mbps=bay.nominal_mbps,
                    connector=_connector_of(group),
                    port_type=_port_type_of(group),
                    panel_slot=panel,
                    panel_row=0,
                    panel_col=idx,
                    port_role=bay.role,
                )
            )
    return tuple(models), tuple(ports)


# ---------------------------------------------------------------------------
# ★1 + ★2 设备身份合并
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class MergeOutcome:
    devices: tuple[CanonDevice, ...]
    conflicts: tuple[CanonConflict, ...]
    merged_groups: int


def _device_key(d: DevicePorts) -> DeviceKey:
    return (d.channel_no, d.rack_no, d.u_start, norm_name(d.model))


def _port_names(d: DevicePorts) -> frozenset[str]:
    return frozenset(e.physical_name.upper() for e in d.used)


def merge_devices(ledger: PortLedger) -> MergeOutcome:
    """把台账的设备视图合并成规范设备（★1 / ★2）。

    同名同位置的两个 `DevicePorts` 只有两种可能：
    - 端口集**不重叠** → 一条设备被两条命名路径各记一半 → 合并（6 组）
    - 端口集**有重叠** → 同 U 位真放了两台 → 保留两台 + `U_OVERLAP` 冲突（3 组）

    用「有交集」而不是「完全相等」作判据：真叠放的两台可能有细微差异
    （一台少接一根线），完全相等的判据会漏掉。宁可多报一次冲突，
    也不要静默合并掉一台真实设备。
    """
    buckets: dict[DeviceKey, list[DevicePorts]] = defaultdict(list)
    for d in ledger.devices:
        buckets[_device_key(d)].append(d)

    devices: list[CanonDevice] = []
    conflicts: list[CanonConflict] = []
    merged = 0

    for key, group in sorted(buckets.items(), key=lambda kv: kv[0]):
        ch, rack, u_start, _ = key
        if len(group) == 1:
            d = group[0]
            span = u_position_span(_span_label(d))
            u_end = span[1] if span[1] is not None else u_start
            devices.append(
                CanonDevice(
                    key=key,
                    name=_instance_name(d),
                    name_normalized=norm_name(_instance_name(d)),
                    model=d.model,
                    model_normalized=key[3],
                    category=d.category,
                    channel_no=ch,
                    rack_no=rack,
                    side="N",
                    u_start=u_start,
                    u_end=u_end,
                    ledger_key=d.device_key,
                )
            )
            continue

        # 多台同位置：按端口集**是否相交**分成两组（★1 与 ★2 的分界）
        variants: list[DevicePorts] = []   # 端口集互不重叠 → 同一台设备的别名
        stacked: list[DevicePorts] = []    # 端口集相交 → 同 U 位真放了两台
        for d in group:
            names = _port_names(d)
            if any(names & _port_names(k) for k in variants):
                stacked.append(d)
            else:
                variants.append(d)
        kept = variants + stacked
        for d in stacked:
            # 真叠放：两台都留，记冲突
            other = next(k for k in variants if _port_names(d) & _port_names(k))
            conflicts.append(
                CanonConflict(
                    kind="U_OVERLAP",
                    channel_no=ch,
                    rack_no=rack,
                    u_start=u_start,
                    u_end=u_start + d.u_span - 1,
                    detail=(
                        f"同 U 位放了两台：{_who(other)} 与 {_who(d)}"
                        f"（端口集有交集，是规划上的重复占位，需人工确认是否真装两台）"
                    ),
                    severity="warn",
                )
            )

        if not stacked:
            # 全是命名变体 → 合并成一台（★1）
            d = variants[0]
            alias_of: dict[str, list[str]] = {}
            for x in group:
                alias_of.setdefault(norm_name(x.model), []).append(_instance_name(x))
            all_names = [n for ns in alias_of.values() for n in ns if n]
            primary = _pick_primary(all_names) or _instance_name(d)
            span = u_position_span(_span_label(d))
            u_end = span[1] if span[1] is not None else u_start
            devices.append(
                CanonDevice(
                    key=key,
                    name=primary,
                    name_normalized=norm_name(primary),
                    model=d.model,
                    model_normalized=key[3],
                    category=d.category,
                    channel_no=ch,
                    rack_no=rack,
                    side="N",
                    u_start=u_start,
                    u_end=u_end,
                    ledger_key=d.device_key,
                    merged_from=tuple(sorted(set(all_names) - {primary})),
                )
            )
            merged += 1
        else:
            # 有真叠放 → 每台各自成行（名字必须能区分，否则库里两行一模一样）
            for idx, d in enumerate(kept):
                name = _instance_name(d)
                span = u_position_span(_span_label(d))
                u_end = span[1] if span[1] is not None else u_start
                devices.append(
                    CanonDevice(
                        key=(
                            ch,
                            rack,
                            u_start,
                            key[3] if idx == 0 else f"{key[3]}#{idx + 1}",
                        ),
                        name=name,
                        name_normalized=norm_name(name),
                        model=d.model,
                        model_normalized=key[3],
                        category=d.category,
                        channel_no=ch,
                        rack_no=rack,
                        side="N",
                        u_start=u_start,
                        u_end=u_end,
                        ledger_key=d.device_key,
                    )
                )

    return MergeOutcome(
        devices=tuple(devices), conflicts=tuple(conflicts), merged_groups=merged
    )


def _pick_primary(names: Sequence[str]) -> str:
    """命名变体里挑一个当主名。优先非数字后缀的短名。

    实测两组别名：`备份-1` / `备份一体机-1`、`tgw-1` / `tpgw-1`。
    两次都挑了前者（短名）—— 因为短名出现在更多行里，是实际在用的名字。
    """
    if not names:
        return ""
    return sorted(names, key=lambda n: (len(n), n))[0]


def _instance_name(d: DevicePorts) -> str:
    """从台账取回实例名；没有名字返回空串（★5）。

    ## 判据是 `device_label` 里有没有 ` @ `，不是 `device_key` 是否以型号开头

    台账给两类设备起 `device_key` 的方式不同：
    - 交换机与特殊设备：`device_key` **就是**实例名（`业务核心交换机-1`）
    - 服务器：`device_key` 是槽位号（`N02-01-S0`），`device_label` 是
      `型号 @通道-机柜-U位`（`计算节点A（ARM） @2-1-3-4U`）

    第一版用 `device_key.startswith(model)` 判「是不是没名字」，
    结果 `业务核心交换机-1` 以型号名开头 → 被误判成无名，
    4 台核心交换机全丢了名字，连带 413 条连线找不到本端设备。
    """
    # 分隔符是 `" @"`（后面紧跟通道号，没有空格），不是 `" @ "`。
    # 第一版写成 `" @ "` 的结果：564 台服务器全部被当成「有名字」，
    # 名字取成了槽位号 `N02-01-S0` —— 看起来非空、实际是位置号（★5 的反例）。
    return "" if " @" in d.device_label else d.device_key


def _who(d: DevicePorts) -> str:
    """人可读的设备标识，用于冲突说明。

    ## 为什么不能只写 `_instance_name()`

    真实数据里 3 条 `U_OVERLAP` 全部是**计算节点**（`ch2/ch8/ch9` 的
    `rack11` 31U），而计算节点没有实例名（★5）。只写 `_instance_name()`
    会得到「同 U 位放了两台： 与 」—— 设备名是两个空格，
    报错等于没报。退到「型号 + 槽位号」后至少能定位到具体是哪两个槽位。
    """
    name = _instance_name(d)
    return name or f"{d.model}（{d.device_key}）"


def _span_label(d: DevicePorts) -> str:
    """台账里只有 `u_start` + `u_span`（没有 `u_end`），拼回 `3-12U` 形式再解析。

    为什么绕这一圈：`u_position_span` 是域层唯一的 U 位解析入口，
    直接写 `u_start + u_span - 1` 会多出第二份「U 位区间怎么算」的实现，
    而两份实现迟早不一致（跨度 1 那 126 台最容易差一）。
    """
    end = d.u_start + d.u_span - 1
    return f"{d.u_start}-{end}U" if d.u_span > 1 else f"{d.u_start}U"


# ---------------------------------------------------------------------------
# 端口（从台账条目）
# ---------------------------------------------------------------------------
def canon_ports(
    ledger: PortLedger, devices: Sequence[CanonDevice]
) -> tuple[CanonPort, ...]:
    """把台账的 12820 条端口条目摊平成端口行。

    设备键要跟着 `merge_devices` 的结果走 —— 合并后 6 组变体只剩一台设备，
    若还按台账原来的 `device_key` 找设备，那 6 台会找不到（漏库）。
    """
    # 直接按台账原始键（`ledger_key`）配对 —— 不能用名字：
    # 564 台服务器没有名字，按名字配会让它们全部漏掉，
    # 端口表里只剩交换机的口（实测会从 12770 掉到 7710）。
    # 别名也要登记：合并后的设备有两个台账键（`备份-1` 与 `备份一体机-1`），
    # 只登记主键会让被合并掉那半边的端口整体消失 ——
    # 实测丢 22 条已用端口（4 组备份一体机 × 4 口 + 2 组 tgw × 3 口），
    # 表现为「备份一体机用了 3/8 口」，而实际是 7/8。
    by_ledger_key: dict[str, CanonDevice] = {}
    for d in devices:
        if d.ledger_key:
            by_ledger_key[d.ledger_key] = d
        if d.name:
            by_ledger_key.setdefault(d.name, d)
        for alias in d.merged_from:
            by_ledger_key.setdefault(alias, d)

    # 按 `(设备键, 端口名)` 去重 —— 同一台设备可能从两个台账键各来一份端口：
    #
    # 合并设备（★1）在台账里有两个键（`备份一体机-1` 与 `备份-1`），
    # 两份都登记进了 `by_ledger_key`，于是同一台设备的同一批口会被产出两次：
    # 一次是「接了线的 occupied」（来自连线），一次是「模板里空闲的 free」。
    #
    # 真实数据上正好 44 条这样的重复（4 台备份一体机 × 8 口 + 2 组 tgw × 6 口），
    # 落库时撞 `UNIQUE(device_id, physical_name)`。
    # **occupied 优先**：接了线是物理事实，free 只是「模板里还有这个口」。
    merged: dict[tuple[DeviceKey, str], CanonPort] = {}
    for entry in ledger.entries:
        dev = by_ledger_key.get(entry.device_key)
        if dev is None:
            # 找不到就跳过 —— 这类条目在真实数据上是 0 条；若出现说明
            # merge_devices 与台账的口径不一致，测试会先在那一侧红。
            continue
        if entry.peer_device:
            status = "occupied"
        else:
            status = "free" if entry.bay is not None else "unknown"
        panel, idx = (entry.bay[1], entry.bay[2]) if entry.bay else (None, None)
        port = CanonPort(
            device_key=dev.key,
            physical_name=entry.physical_name,
            display_name=entry.physical_name,
            speed_mbps=entry.speed_mbps,
            connector=_connector_of(entry.bay[0]) if entry.bay else "OTHER",
            port_type=_port_type_of(entry.bay[0]) if entry.bay else "optical",
            port_role=entry.role,
            status=status,
            panel_slot=panel,
            panel_row=0,
            panel_col=idx,
        )
        key = (dev.key, entry.physical_name)
        prev = merged.get(key)
        if prev is None or (prev.status != "occupied" and status == "occupied"):
            merged[key] = port
    return tuple(merged.values())


# ---------------------------------------------------------------------------
# 连线（从 GeneratedCabling）
# ---------------------------------------------------------------------------
def canon_cables(
    links: Sequence[object], devices: Sequence[CanonDevice]
) -> tuple[CanonCable, ...]:
    """把生成的 5127 条连线摊平成 `cable` 行（★4）。

    两端都要解析成设备键。解析不到的（源数据冲突，比如
    `防火墙-1` 在意图里没有声明）跳过并计数 —— 硬塞会写出一条
    指向不存在设备的连线，`foreign_keys = ON` 会立刻报错，
    但错误信息是 `FOREIGN KEY constraint failed`，不说是哪条线。

    本函数刻意收 `Sequence[object]` 而不是 `Sequence[GeneratedLink]`：
    `generate.py` 里已有生成器的类型，域层这里若 import 它，
    `canon` 就会被拉进生成器的依赖链（生成器 → canon → 生成器）。
    用 duck typing 只取需要的属性，两个方向都不断。
    """
    # 三个查找表：
    # - by_name：主名 → 设备
    # - by_alias：被合并掉的别名 → 主设备（★1 的必然结果，6 组）
    # - by_slot：位置标签 → 设备（服务器没名字时的兜底）
    #
    # 少了 by_alias 会有 22 条连线被丢弃：`备份接入交换机-1` 的 4 个口接的是
    # `备份一体机-1..4`，而这 4 个名字已被合并进 `备份-1..4`。
    # 这类丢弃不会报错，只会让「备份交换机那 4 根线」在库里凭空消失。
    by_name: dict[str, CanonDevice] = {}
    by_alias: dict[str, CanonDevice] = {}
    by_slot: dict[str, CanonDevice] = {}
    for d in devices:
        if d.name:
            by_name[d.name] = d
        for alias in d.merged_from:
            by_alias[alias] = d
        if d.ledger_key:
            by_slot[d.ledger_key] = d
        by_slot[d.display_label] = d

    out: list[CanonCable] = []
    for link in links:
        a_dev = by_name.get(link.a_device) or by_alias.get(link.a_device)
        b_name = link.b_device or getattr(link, "b_id", "") or link.b_model
        z_dev = by_name.get(b_name) or by_alias.get(b_name) or by_slot.get(b_name)
        if a_dev is None or z_dev is None:
            continue
        out.append(
            CanonCable(
                stable_key=f"{link.a_device}|{link.a_port}|{b_name}|{link.b_port}|"
                f"{link.cable_type}",
                occurrence=0,
                a_device_key=a_dev.key,
                a_port_name=link.a_port,
                z_device_key=z_dev.key,
                z_port_name=link.b_port,
                cable_type=link.cable_type,
                remark=getattr(link, "nic", "") or "",
            )
        )
    return tuple(out)


# ---------------------------------------------------------------------------
# 接线区间（从 FlowPlan，★3 按型号拆行）
# ---------------------------------------------------------------------------
def canon_ranges(
    plan: object, devices: Sequence[CanonDevice]
) -> tuple[CanonRange, ...]:
    """`switch_service_range` 行：一台交换机的一块口服务了哪些型号的节点。

    ## 为什么按「块 × 型号」拆行

    实测 106 个块里，`计算接入交换机(业务)-5` 的 44 个口里有
    `计算节点A（ARM）` 也有 `计算节点B（C86）`。表上有
    `UNIQUE(project_id, switch_device_id, node_model_id, base_port_no)`，
    一块一行必然冲突。拆开后 **142 行**。

    ## base_port_no 取该型号节点的最小端口号

    不是 `block` 的起点 —— 混型号的块里，第二种型号的节点从中间某个口开始，
    取块起点会谎报「A 型节点从这个口开始」，用它去重放接线就会错位。
    """
    by_name = {d.name: d for d in devices if d.name}
    node2port: dict[tuple[str, str], int] = {}
    for a in getattr(plan, "assignments", ()):
        node2port[(a.switch_instance, a.node_id)] = a.switch_port_no

    out: list[CanonRange] = []
    for block in getattr(plan, "blocks", ()):
        sw = by_name.get(block.switch_instance)
        if sw is None:
            continue
        per_model: dict[str, list[int]] = defaultdict(list)
        for node in block.nodes:
            port = node2port.get((block.switch_instance, node.node_id))
            if port is not None:
                per_model[norm_name(node.model)].append(port)
        for model_norm, ports in sorted(per_model.items()):
            out.append(
                CanonRange(
                    switch_device_key=sw.key,
                    node_model_normalized=model_norm,
                    base_port_no=min(ports),
                    node_count_expected=len(ports),
                    sort_rule=block.order if hasattr(block, "order") else "rack_then_u",
                    port_role=getattr(block, "port_role", "downlink"),
                )
            )
    return tuple(out)


# ---------------------------------------------------------------------------
# 汇总
# ---------------------------------------------------------------------------
def build_canonical(
    ledger: PortLedger,
    *,
    geometry: GeometrySnapshot,
    links: Sequence[object] = (),
    plan: object | None = None,
    library: ModelLibrary | None = None,
) -> CanonicalBundle:
    """把台账（+ 生成的连线 + 接线计划）摊平成完整的一套规范行。

    ## 为什么台账是必填而连线/计划可选

    台账回答「有哪些设备、有哪些口」，是**底座**；连线回答「哪个口接哪个口」，
    接线区间回答「哪些口是自动分配的」。后两者缺失时仍然可以建出一个
    有设备有端口的库（只是没有连线），这在「先录上架图、后补连线」的
    真实流程里是正常状态。
    """
    models, model_ports = canon_models(library)
    outcome = merge_devices(ledger)
    devices = outcome.devices
    ports = canon_ports(ledger, devices)
    cables = canon_cables(links, devices) if links else ()
    ranges = canon_ranges(plan, devices) if plan is not None else ()

    conflicts = list(outcome.conflicts)
    conflicts.extend(_model_conflicts(devices))
    stats = {
        "devices": len(devices),
        "ports": len(ports),
        "cables": len(cables),
        "ranges": len(ranges),
        "merged": outcome.merged_groups,
        "occupied": sum(1 for p in ports if p.status == "occupied"),
        "free": sum(1 for p in ports if p.status == "free"),
        "unknown_port": sum(1 for p in ports if p.status == "unknown"),
        "dropped_cables": (len(links) - len(cables)) if links else 0,
    }
    return CanonicalBundle(
        rooms=canon_rooms(geometry),
        aisles=canon_aisles(geometry),
        racks=canon_racks(geometry),
        models=models,
        model_ports=model_ports,
        devices=devices,
        ports=ports,
        cables=cables,
        ranges=ranges,
        conflicts=tuple(conflicts),
        stats=stats,
    )


def _model_conflicts(devices: Sequence[CanonDevice]) -> tuple[CanonConflict, ...]:
    """同一实例名记了两个型号 —— 源表标注矛盾，不是本模块的错。

    实测 6 台：`广域网链路负载均衡-1/-2`、`管理接入交换机-1`、
    `计算接入交换机(业务)-1/-2`、`防火墙-2`。它们的端口会被拆到两台设备上，
    台账上表现为「两台各用了一半」，界面上看起来像模板不准。
    必须落一条冲突，把因果关系说清楚。
    """
    models_of: dict[str, set[str]] = defaultdict(set)
    pos_of: dict[str, tuple[int, int, int]] = {}
    for d in devices:
        if not d.name:
            continue
        models_of[d.name].add(d.model)
        pos_of.setdefault(d.name, (d.channel_no, d.rack_no, d.u_start))
    out: list[CanonConflict] = []
    for name, models in sorted(models_of.items()):
        if len(models) < 2:
            continue
        ch, rack, u = pos_of[name]
        out.append(
            CanonConflict(
                kind="MODEL_CONFLICT",
                channel_no=ch,
                rack_no=rack,
                u_start=u,
                u_end=u,
                detail=(
                    f"{name} 在源表里被记了两个型号：{'、'.join(sorted(models))}。"
                    "它的端口会被拆到两台设备上，台账看起来「两台各用一半」。"
                    "这是源表标注矛盾，改型号归属比改模板有效。"
                ),
                severity="warn",
            )
        )
    return tuple(out)


__all__ = [
    "CanonAisle",
    "CanonCable",
    "CanonConflict",
    "CanonDevice",
    "CanonModel",
    "CanonModelPort",
    "CanonPort",
    "CanonRack",
    "CanonRange",
    "CanonRoom",
    "CanonicalBundle",
    "DeviceKey",
    "MergeOutcome",
    "build_canonical",
    "canon_aisles",
    "canon_cables",
    "canon_models",
    "canon_ports",
    "canon_racks",
    "canon_ranges",
    "canon_rooms",
    "merge_devices",
]
