"""端口台账 —— 「哪个机柜哪台设备哪个口插了什么线、用了几口、还剩几口」。

## 这个模块回答的问题，和已有模块的区别

已有模块各自回答一个问题，但都不是「端口」这一层：

| 模块 | 回答 | 不回答 |
|---|---|---|
| `ports/intent.py` | 我打算怎么接（949 行意图） | 实际用了哪个口 |
| `ports/generate.py` | 生成 5127 行布线表 | 一台设备一共几个口、还剩几个 |
| `validation/instances.py` | 机柜里有哪几台设备、占哪个 U | 设备上插了几根线 |
| `cable_length/*` | 这根线多长 | 口够不够用 |

端口台账把这三层缝起来：**设备（含型号）→ 端口（含标称容量）→ 连线（含对端）**。
它输出的核心结论是「这台交换机 56 个口用了 47 个、剩 9 个在哪」。

## 为什么「还剩几个口」必须当真算，不能估

现场问「这个柜子还能不能再插 4 台服务器」时，答案取决于两件事：
空 U 位够不够、**要接的交换机还有没有空口**。前者看 U 位图，后者只有台账能答。

如果分母是估的（比如按"同型号里用得最多的那台"当分母），
就会出现「台账说有 9 个空口、现场装上发现只空 2 个」——
这类错误一旦发生，整个工具的可信度就没了。

所以分母来自 `portspec.py` 的人工核定模板（有原文出处），
且实测 953/954 台设备 0 未知端口、0 越界。

## 三条硬约束（每条都对应一个真实的错法）

1. **端口身份是 `(组, 面板, 序号)`，不是端口名。** 同一物理口可能是
   `25GE1/0/5` 也可能是 `10GE1/0/5`（协商速率不同），按名字计数会重复。
2. **服务器没有名字，身份靠位置。** 实测 550 台计算节点在布线表里
   `对端设备名称` 普遍为空，只能用 `(通道, 机柜, U位, 型号)` 认设备。
3. **没有模板的型号返回 None，不算 0。** 「这台设备几个口我不知道」
   与「这台设备 0 个口」是两件完全不同的事，混为一谈会让利用率算成 0%。
"""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass
from typing import TYPE_CHECKING, Iterable

from ..importing.normalize import u_position_span
from .naming import parse_port_name
from .modellib import ModelLibrary, free_bays_of
from .portspec import PortSpec, free_bays, spec_of

if TYPE_CHECKING:  # pragma: no cover - 只为类型检查，避免与 generate 循环导入
    from .generate import GeneratedCabling


@dataclass(frozen=True, slots=True)
class PortEntry:
    """端口台账的一行 —— 一个具体端口的状态。"""

    device_key: str
    """设备身份。交换机用实例名；服务器用 `N{通道}-{机柜}-S{槽位}` 这类位置号。"""

    device_label: str
    """给人看的名字。服务器无名时退化成型号+位置，如 `计算节点A（ARM） @2-11-31U`。"""

    model: str
    channel_no: int
    rack_no: int
    u_start: int

    physical_name: str
    """端口物理名，原样保留（`25GE1/0/5`、`万兆2`）。导出与现场核对都要原文。"""

    bay: tuple[str, int, int] | None
    """端口身份 `(组, 面板, 序号)`。None 表示模板里没有这个口。"""

    role: str = "unknown"
    speed_mbps: int | None = None
    peer_device: str = ""
    peer_port: str = ""
    cable_type: str = ""

    @property
    def is_used(self) -> bool:
        return bool(self.peer_device)

    def describe(self) -> str:
        where = f"通道{self.channel_no} 机柜{self.rack_no} {self.u_start}U"
        if self.is_used:
            return f"{self.device_label} {self.physical_name} → {self.peer_device} {self.peer_port}"
        return f"{self.device_label} {self.physical_name} （空闲）@ {where}"


@dataclass(frozen=True, slots=True)
class DevicePorts:
    """一台设备的端口使用情况（台账的设备级视图）。"""

    device_key: str
    device_label: str
    model: str
    channel_no: int
    rack_no: int
    u_start: int
    u_span: int
    category: str
    """"switch" / "server" / "core" / "security" / "storage" / "other"。"""

    spec: PortSpec | None
    """型号模板。None 表示这个型号还没有核定过端口规格。"""

    used: tuple[PortEntry, ...]
    """已用的口（每个口一条，已去重）。"""

    free: tuple[tuple[str, int, int], ...]
    """空闲的端口身份。模板缺失时为空。"""

    unknown_ports: tuple[str, ...]
    """归不进模板的端口名。非空 = 模板与数据不符，必须显式提示。"""

    @property
    def used_count(self) -> int:
        return len(self.used)

    @property
    def total_count(self) -> int:
        """标称总口数。没有模板返回 0 —— 用 `has_spec` 区分「0 个口」与「不知道」。"""

        return self.spec.total_ports if self.spec else 0

    @property
    def free_count(self) -> int:
        return len(self.free)

    @property
    def has_spec(self) -> bool:
        return self.spec is not None

    @property
    def ratio(self) -> float | None:
        """利用率。没有模板返回 None（**不是 0**）。"""
        if not self.spec or not self.spec.total_ports:
            return None
        return self.used_count / self.spec.total_ports

    def by_role(self) -> dict[str, int]:
        """按端口角色统计已用数。现场关心「上行口还剩几个」。"""
        out: dict[str, int] = defaultdict(int)
        for entry in self.used:
            out[entry.role] += 1
        return dict(out)

    def describe(self) -> str:
        where = f"通道{self.channel_no} 机柜{self.rack_no} {self.u_start}U"
        if not self.spec:
            return f"{self.device_label}（{self.model}）@ {where}：{self.used_count} 口在用，规格未知"
        pct = self.ratio or 0.0
        return f"{self.device_label}（{self.model}）@ {where}：{self.used_count}/{self.total_count} 口（{pct:.0%}）"


@dataclass(frozen=True, slots=True)
class PortLedger:
    """一个项目的端口台账。"""

    devices: tuple[DevicePorts, ...]
    entries: tuple[PortEntry, ...]
    """全部端口条目（含空闲口），按设备、端口的自然顺序。"""

    @property
    def device_count(self) -> int:
        return len(self.devices)

    @property
    def used_total(self) -> int:
        return sum(d.used_count for d in self.devices)

    @property
    def capacity_total(self) -> int:
        """有模板的设备标称口数合计。没有模板的设备不计入（不猜）。"""
        return sum(d.total_count for d in self.devices if d.has_spec)

    @property
    def free_total(self) -> int:
        return sum(d.free_count for d in self.devices)

    @property
    def no_spec_devices(self) -> tuple[DevicePorts, ...]:
        return tuple(d for d in self.devices if not d.has_spec)

    @property
    def over_used(self) -> tuple[DevicePorts, ...]:
        """用了超过标称口数的设备。**非空一定是 bug**，不是正常情况。"""
        return tuple(d for d in self.devices if d.has_spec and d.used_count > d.total_count)

    @property
    def with_unknown_ports(self) -> tuple[DevicePorts, ...]:
        return tuple(d for d in self.devices if d.unknown_ports)

    def by_rack(self) -> dict[tuple[int, int], tuple[DevicePorts, ...]]:
        out: dict[tuple[int, int], list[DevicePorts]] = defaultdict(list)
        for d in self.devices:
            out[(d.channel_no, d.rack_no)].append(d)
        return {k: tuple(v) for k, v in out.items()}

    def by_model(self) -> dict[str, tuple[DevicePorts, ...]]:
        out: dict[str, list[DevicePorts]] = defaultdict(list)
        for d in self.devices:
            out[d.model].append(d)
        return {k: tuple(v) for k, v in out.items()}


# ---------------------------------------------------------------------------
# 构造
# ---------------------------------------------------------------------------


@dataclass(frozen=True, slots=True)
class RawPortUse:
    """构造台账的输入 —— 一个「某设备的某端口接了某对端」的最小事实。

    刻意与 `GeneratedLink` 解耦：生成器的类型在 `ports/generate.py`，
    而台账也要能描述**人工录入的**端口（v0.5 之后会有手工改台账的入口）。
    """

    side: str
    """`a` 或 `b`。指这条线的本端/对端，台账为**两端都建条目**。"""

    device_key: str
    device_label: str
    model: str
    channel_no: int
    rack_no: int
    u_start: int
    u_span: int
    port: str
    peer_device: str
    peer_port: str
    cable_type: str = ""


def build_ledger(
    uses: Iterable[RawPortUse], *, library: ModelLibrary | None = None
) -> PortLedger:
    """从「端口使用事实」构造台账。

    ## 为什么输入是 `side` 而不是已经拆好的两端

    布线表一行 = 一条线 = **两个端口各被占用一次**。
    调用方若自己拆两端，很容易犯「只记本端、忘了对端」的错 ——
    那样服务器的端口永远是 0 个在用，而服务器恰恰是「还剩几个口」
    问题里最需要算准的部分（925 台服务器 vs 128 台交换机）。

    所以这里收「一条线」，内部展开成两条占用记录。
    """
    # device_key → {端口身份 或 端口名 → 该口的占用}
    per_device: dict[str, dict[object, PortEntry]] = defaultdict(dict)
    meta: dict[str, tuple[str, str, int, int, int, int]] = {}
    lookup = library.spec_of if library is not None else spec_of

    for use in uses:
        spec = lookup(use.model)
        key = spec.bay_key(use.port) if spec else None
        # 端口身份优先用 bay（同物理口不同速率写法会归到一起）；
        # 模板缺失或端口越界时退回端口名 —— 至少不会丢掉这个占用事实。
        ident: object = key if key is not None else ("name", use.port.upper())
        role = spec.group_of(parse_port_name(use.port)) if spec else None
        speed = parse_port_name(use.port).rate_mbps

        per_device[use.device_key][ident] = PortEntry(
            device_key=use.device_key,
            device_label=use.device_label,
            model=use.model,
            channel_no=use.channel_no,
            rack_no=use.rack_no,
            u_start=use.u_start,
            physical_name=use.port,
            bay=key,
            role=role.role if role else "unknown",
            speed_mbps=speed,
            peer_device=use.peer_device,
            peer_port=use.peer_port,
            cable_type=use.cable_type,
        )
        meta[use.device_key] = (
            use.device_label,
            use.model,
            use.channel_no,
            use.rack_no,
            use.u_start,
            use.u_span,
        )

    devices: list[DevicePorts] = []
    for device_key in sorted(per_device, key=lambda k: meta[k][2:5]):
        label, model, ch, rack, u_start, u_span = meta[device_key]
        spec = lookup(model)
        used = tuple(sorted(per_device[device_key].values(), key=_entry_order))
        port_names = [e.physical_name for e in used]
        if spec is not None:
            unknown = tuple(sorted({p for p in port_names if spec.bay_key(p) is None}))
            free = free_bays_of(spec, port_names) if library else free_bays(model, port_names)
            total = spec.total_ports
        else:
            unknown = ()
            free = ()
            total = 0
        devices.append(
            DevicePorts(
                device_key=device_key,
                device_label=label,
                model=model,
                channel_no=ch,
                rack_no=rack,
                u_start=u_start,
                u_span=u_span,
                category=spec.category if spec else "other",
                spec=spec,
                used=used,
                free=free,
                unknown_ports=unknown,
            )
        )

    entries: list[PortEntry] = []
    for d in devices:
        entries.extend(d.used)
        for bay in d.free:
            entries.append(
                PortEntry(
                    device_key=d.device_key,
                    device_label=d.device_label,
                    model=d.model,
                    channel_no=d.channel_no,
                    rack_no=d.rack_no,
                    u_start=d.u_start,
                    physical_name=_bay_name(bay, d.model),
                    bay=bay,
                    role=_bay_role(d.spec, bay),
                )
            )

    return PortLedger(devices=tuple(devices), entries=tuple(entries))


def _entry_order(entry: PortEntry) -> tuple[int, int, int, str]:
    """端口排序：按组名字母序、面板、序号。空闲口与已用口能对上位置。"""
    if entry.bay is None:
        return (9, 0, 0, entry.physical_name)
    group, panel, idx = entry.bay
    return (0, panel, idx, group)


def _bay_name(bay: tuple[str, int, int], model: str) -> str:
    """空闲口的显示名。生成成与已用口同款的名字，便于现场比对。"""
    group, panel, idx = bay
    if group.endswith("GE"):
        return f"{group}{panel}/0/{idx}"
    return f"{group}{idx}"


def _bay_role(spec: PortSpec | None, bay: tuple[str, int, int]) -> str:
    if spec is None:
        return "unknown"
    for b in spec.bays:
        if b.group == bay[0]:
            return b.role
    return "unknown"


def ledger_from_cabling(
    cabling: "GeneratedCabling", *, library: ModelLibrary | None = None
) -> PortLedger:
    """从生成的布线表构造端口台账 —— 一条连线给两端各记一次占用。

    ## 为什么这一步要在域层，不放服务层

    「一条线 → 两个端口占用」这个换算只有域层知道：
    `GeneratedLink` 的 `a_u`/`b_u` 是 U 位**原文**（`3-12U`、`40U`），
    要转成 `(起始 U, 跨度)` 才能算设备身份。服务层若自己写一遍，
    就得重复实现 `u_position_span` 的容错（全角 `Ｕ`、`~`、`—`），
    两处实现迟早不一致 —— 而这类不一致的表现是「同一台设备在台账里
    出现两次、各占一半端口」，极难从界面上看出来。

    ## 服务器为什么用 `b_id`

    `b_device` 是显示名，而实测 4562 行交换机→服务器连线里 4230 行的
    对端设备名是**空的**（服务器从来没被命名过）。`b_id` 是生成器给的
    恒定身份（`N02-11-S7-1`），用它才能把 44 条线正确聚到同一台服务器。
    显示时再拼位置标签，让人看得懂是哪台。
    """
    uses: list[RawPortUse] = []
    for link in cabling.links:
        a_start, a_end = u_position_span(link.a_u)
        b_start, b_end = u_position_span(link.b_u)
        b_key = link.b_id or link.b_device or f"{link.b_model}@{link.b_channel}-{link.b_rack}-{link.b_u}"
        b_label = link.b_device or f"{link.b_model} @{link.b_channel}-{link.b_rack}-{link.b_u}"

        if a_start is not None and link.a_port:
            uses.append(
                RawPortUse(
                    side="a",
                    device_key=link.a_device,
                    device_label=link.a_device,
                    model=link.a_model,
                    channel_no=link.a_channel,
                    rack_no=link.a_rack,
                    u_start=a_start,
                    u_span=a_end - a_start + 1,
                    port=link.a_port,
                    peer_device=b_key,
                    peer_port=link.b_port,
                    cable_type=link.cable_type,
                )
            )
        if b_start is not None and link.b_port:
            uses.append(
                RawPortUse(
                    side="b",
                    device_key=b_key,
                    device_label=b_label,
                    model=link.b_model,
                    channel_no=link.b_channel,
                    rack_no=link.b_rack,
                    u_start=b_start,
                    u_span=b_end - b_start + 1,
                    port=link.b_port,
                    peer_device=link.a_device,
                    peer_port=link.a_port,
                    cable_type=link.cable_type,
                )
            )
    return build_ledger(uses, library=library)


__all__ = [
    "DevicePorts",
    "PortEntry",
    "PortLedger",
    "RawPortUse",
    "build_ledger",
    "ledger_from_cabling",
]
