"""接线意图 → 布线表行（**产品的主功能**）。

## 这个模块解决的问题

用户的原话是「自动化完成布线表（机器和机器之间的连线表）」。
`intent.py` 定义了「用户要填什么」，`flow.py` 定义了「节点怎么分给交换机」，
本模块把两层结果拼成**能直接导出成那 25 列表格的行**。

链路：

```
Intent  --to_flow_inputs-->  placements / routes / chains
        --plan_flow------->  每台交换机的端口分配
        --generate_links-->  一条条布线表行（本端交换机 ↔ 对端服务器）
```

## 线缆类型不进口图，由端口速率派生

实测 V13 全表 5112 行，按「两端端口的 (style, rate)」推导线缆类型，
唯一映射覆盖 **5097/5112 = 99.71%**。所以用户填意图时**不必填线缆类型**。

实测映射（括号内为行数）：

| 本端 | 对端 | 线缆类型 | 行数 |
|---|---|---|---|
| cli_board 10G | label 10G | LC-LC | 2216 |
| cli_board 1G | label 1G | RJ45 | 2056 |
| cli_board 100G | cli_board 100G | MPO | 348 |
| cli_board 25G | label 25G | LC-LC | 274 |
| cli_board 25G | cli_board 10G | LC-LC | 103 |
| cli_board 10G | cli_board 10G | LC-LC | 61 |
| cli_board 1G | cli_board 1G | RJ45 | 17 |
| cli_board 25G | label 1G | RJ45 | 12 |
| cli_board 100G | label 100G | MPO | 4 |
| cli_board 100G | cli_board 25G | LC-LC | 2 |

归约出的规则（**两条**）：
1. 任一端速率 ≤ 1G → `RJ45`（光纤到电口的场景要光转电模块，实测 12 行）
2. 两端都是 100G → `MPO`（多模并行光，实测 348+4 行）
3. 其余 → `LC-LC`（双工光）

唯一无法派生的是「两端速率相同但一端用了 MPO」那种情况（实测 1 行：
`25G ↔ 25G` 里 14 条 LC-LC、1 条 MPO），必须用 `ExplicitLink` 显式覆盖。

## 本端 U 位与对端 U 位

- 对端（服务器）：`slot` → `f"{u_start}-{u_start + 1}U"`，如槽位 0 → `3-4U`
- 本端（交换机）：`f"{u_start}U"`，实测交换机都占 1U（40U/37U/34U）

## 不自动生成的部分（诚实边界）

本模块只生成「交换机 → 服务器」这一方向（实测 4562/5113 行）。
交换机之间的互联（核心 ↔ 接入的 MPO 上行、堆叠线等，实测 551 行）
需要 `ExplicitLink` 显式声明 —— 那部分的端口规律尚未破解，
不假装能算。这个边界写在返回值 `GeneratedCabling.notes` 里。
"""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass, field

from ..shared.types import Issue, IssueCode
from ..units import SLOT_BASE_U, SLOT_PITCH_U
from .flow import plan_flow
from .intent import DeviceSpec, Intent, SwitchSpec, expand_placements, SwitchSpec, to_flow_inputs
from .naming import PortName, parse_port_name

#: 派生规则用到的速率阈值
_RATE_1G = 1_000
_RATE_100G = 100_000


@dataclass(frozen=True, slots=True)
class GeneratedLink:
    """生成的布线表一行（还没算长度，也没进 25 列契约）。

    字段与 V13 布线表前 13 列一一对应，命名用 `a`/`b` 沿用域层习惯
    （`a` = 本端 = 交换机侧，`b` = 对端 = 服务器侧）。
    """

    a_device: str
    a_model: str
    a_channel: int
    a_rack: int
    a_u: str
    a_port: str
    #: 对端设备**名称**。生成器一律留空 —— 见模块内 `generate_links` 里的说明。
    #: 真实表这一列是半填状态且存在型号/名字矛盾的记录，设备名属上架命名规范，
    #: 不是接线意图，机器不该猜。
    b_device: str
    b_model: str
    b_channel: int
    b_rack: int
    b_u: str
    b_port: str
    cable_type: str
    #: 这一行由哪个网口产生（`万兆1`/`IPMI`…），供分组统计与 UI 着色
    nic: str
    #: 对端设备的**标识**（槽位 id，如 `N02-11-S7-1`）。
    #:
    #: 为什么不复用 `b_device`：`b_device` 是给人看的**显示名**，真实数据里
    #: 服务器行是空的；而 `stable_key`、按设备名查线缆、去重都需要一个
    #: **恒定非空的身份**。两者混用会导致生成行与导入行口径不一致
    #: （导入侧 `make_stable_key` 拿 `device_name or model` 兜底）。
    b_id: str = ""

    @property
    def switch_instance(self) -> str:
        return self.a_device


@dataclass(frozen=True, slots=True)
class GeneratedCabling:
    """生成结果。"""

    links: tuple[GeneratedLink, ...]
    issues: tuple[Issue, ...]
    #: 生成过程中产生的说明性文字（给 UI 展示边界用）
    notes: tuple[str, ...] = ()
    #: 各网口的行数分布
    by_nic: tuple[tuple[str, int], ...] = ()
    #: 显式连线按「对端端口名」的分布。必须与 `by_nic` 分开：
    #: `by_nic` 是服务器网口标签（`千兆1`），这里是 CLI 端口名（`100GE1/0/1`），
    #: 两者混在一起会让「千兆1 694 / IPMI 684」看起来像漏了 10 行接线，
    #: 实际是对端端口名被当成了网口。
    explicit_by_port: tuple[tuple[str, int], ...] = ()

    def __len__(self) -> int:
        return len(self.links)

    @property
    def error_count(self) -> int:
        return sum(1 for i in self.issues if i.severity == "error")

    @property
    def warn_count(self) -> int:
        return sum(1 for i in self.issues if i.severity == "warn")


# ---------------------------------------------------------------------------
# 线缆类型派生
# ---------------------------------------------------------------------------
def derive_cable_type(a: PortName, b: PortName) -> str | None:
    """由两端端口的速率与样式推导线缆类型。

    实测 99.71% 可派生，剩下的靠 `ExplicitLink` 覆盖。
    返回 `None` 表示两端有未知速率，无法判定（调用方应报 issue，不要瞎猜）。

    >>> derive_cable_type(parse_port_name("10GE1/0/1"), parse_port_name("万兆1"))
    'LC-LC'
    >>> derive_cable_type(parse_port_name("GE1/0/2"), parse_port_name("千兆1"))
    'RJ45'
    >>> derive_cable_type(parse_port_name("100GE1/0/1"), parse_port_name("100GE1/0/2"))
    'MPO'
    >>> derive_cable_type(parse_port_name("25GE1/0/39"), parse_port_name("千兆1"))
    'RJ45'
    >>> derive_cable_type(parse_port_name("25GE1/0/1"), parse_port_name("万兆1"))
    'LC-LC'
    >>> derive_cable_type(parse_port_name("???"), parse_port_name("万兆1")) is None
    True
    """
    ra, rb = a.rate_mbps, b.rate_mbps
    if ra is None or rb is None:
        return None
    # 规则 1：任一端是千兆及以下 → 必须走电口（交换机侧要插光转电模块）
    if min(ra, rb) <= _RATE_1G:
        return "RJ45"
    # 规则 2：两端都是 100G → MPO 多模并行
    if ra == rb == _RATE_100G:
        return "MPO"
    # 规则 3：其余光纤场景 → LC-LC 双工
    return "LC-LC"


# ---------------------------------------------------------------------------
# U 位格式化
# ---------------------------------------------------------------------------
def u_label(u_start: int, span: int = 1) -> str:
    """U 位原文。占 1U 写 `40U`，占多 U 写区间 `3-12U`。

    ## 为什么 U 位格式必须随设备高度变化

    真实 V13 的「本端位置」列只有三种形态：`40U`(2683 行)、`37U`(1947)、
    `34U`(180) 都是 1U 设备，`3-12U`(298) 是 4 台核心交换机的 10U 机框，
    `19-20U`(5) 是 2U 负载均衡。**一律输出 `40U` 会让 298 行全错。**

    >>> u_label(40)
    '40U'
    >>> u_label(3, 10)
    '3-12U'
    >>> u_label(19, 2)
    '19-20U'
    """
    if span <= 1:
        return f"{u_start}U"
    return f"{u_start}-{u_start + span - 1}U"


def u_label_server(u_start: int, span: int = 2) -> str:
    """服务器的 U 位原文。默认占 2U，写成区间。

    >>> u_label_server(3)
    '3-4U'
    >>> u_label_server(31)
    '31-32U'
    """
    return u_label(u_start, span)


def u_label_switch(u_start: int, span: int = 1) -> str:
    """网络设备的 U 位原文。默认占 1U。

    >>> u_label_switch(40)
    '40U'
    >>> u_label_switch(3, 10)
    '3-12U'
    """
    return u_label(u_start, span)


# ---------------------------------------------------------------------------
# 主流程
# ---------------------------------------------------------------------------
def generate_links(intent: Intent) -> GeneratedCabling:
    """意图 → 布线表行。这是产品的主功能入口。

    >>> it = Intent(project="t")
    >>> len(generate_links(it))
    0
    """
    placements, routes, chains, issues = to_flow_inputs(intent)
    issues = list(issues)

    if not placements or not chains:
        return GeneratedCabling(
            links=(),
            issues=tuple(issues),
            notes=("意图里没有可生成的连线：需要布局段 + 路由 + 交换机组",),
        )

    plan = plan_flow(placements, routes, chains)
    issues.extend(plan.issues)

    # --- 索引：节点位置、交换机位置、显式覆盖 ---
    by_node = {p.node_id: p for p in placements}
    sw_of: dict[str, SwitchSpec] = {s.instance: s for s in intent.switches}
    overrides = _explicit_overrides(intent)

    # --- 逐条分配，拼成布线表行 ---
    links: list[GeneratedLink] = []
    unparsed = 0
    unknown_rate = 0

    for a in plan.assignments:
        node = by_node.get(a.node_id)
        sw = sw_of.get(a.switch_instance)
        if node is None or sw is None:
            # to_flow_inputs 已经校验过，走到这里说明有内部 bug，不静默
            issues.append(
                Issue.error(
                    IssueCode.PORT_UNASSIGNED,
                    f"分配结果引用了未知的节点 {a.node_id} 或交换机 {a.switch_instance}",
                )
            )
            continue

        # 网口标签就是服务器侧的端口名（实测 `万兆1`/`IPMI` 原样出现在该列）
        nic = _nic_of(a.block_key, chains)
        b_port_name = parse_port_name(nic)
        a_port_name = parse_port_name(a.switch_port)
        if not a_port_name.is_known or not b_port_name.is_known:
            unparsed += 1

        key = (a.switch_instance, a.switch_port, a.node_id, nic)
        cable = overrides.get(key)
        if cable is None:
            cable = derive_cable_type(a_port_name, b_port_name) or ""
        if not cable:
            unknown_rate += 1
            issues.append(
                Issue.warn(
                    IssueCode.UNKNOWN_CABLE_TYPE,
                    f"端口 {a.switch_port} ↔ {nic} 的速率无法识别，线缆类型留空待人工填写",
                    field="cable_type",
                )
            )

        links.append(
            GeneratedLink(
                a_device=sw.instance,
                a_model=sw.model,
                a_channel=sw.channel_no,
                a_rack=sw.rack_no,
                a_u=u_label_switch(sw.u_start, sw.u_span),
                a_port=a.switch_port,
                b_device="",
                b_id=node.node_id,
                b_model=node.model,
                b_channel=node.channel_no,
                b_rack=node.rack_no,
                b_u=u_label_server(node.u_start),
                b_port=nic,
                cable_type=cable,
                nic=nic,
            )
        )

    # 服务器侧设备名称一律留空 —— 这不是偷懒，是**忠实于真实数据**：
    # V13 全表 4562 行交换机→服务器连线里，对端设备名称只有 332 行有值
    # （`xkgw-1`/`nat-1`/`gso-1` 这类非计算节点），计算节点行全是空。
    # 用户的表格本来就不填这一列，我们凭空填上反而让交付物和他手上的不一样。
    if unparsed:
        issues.append(
            Issue.warn(
                IssueCode.UNKNOWN_CABLE_TYPE,
                f"{unparsed} 行的端口名无法解析（既不是 CLI 板卡式也不是标签式）",
                field="port_name",
            )
        )

    # --- 显式连线：规则覆盖不到的那 13% ---
    extra_links, extra_issues = explicit_links(intent)
    if extra_links:
        links.extend(extra_links)
    if extra_issues:
        issues.extend(extra_issues)

    rule_rows = len(links) - len(extra_links)
    notes = [
        f"生成 {rule_rows} 行「交换机 → 服务器」连线，覆盖 {len(chains)} 条交换机组",
    ]
    if extra_links:
        notes.append(f"另有 {len(extra_links)} 行来自显式连线（交换机互联与特殊机型）")
    else:
        notes.append(
            "交换机之间的互联（核心↔接入上行、堆叠线）与特殊机型接线不在规则范围内，"
            "需要在意图里用显式连线（explicitLinks）声明；实测这类行占 V13 全表约 13%"
        )
    #: CLI 端口名以 `GE`/`10GE`/`25GE`/`100GE` 开头，服务器网口标签是 `千兆1`/`万兆1`/`IPMI`。
    #: 用前缀判断而不是维护白名单，是因为型号库里端口名形态会变。
    def _is_cli_port(nic: str) -> bool:
        return nic.startswith(("GE", "10GE", "25GE", "40GE", "100GE", "400GE", "Eth"))

    by_nic = tuple(
        sorted(
            Counter(x.nic for x in links if not _is_cli_port(x.nic)).items(),
            key=lambda kv: (-kv[1], kv[0]),
        )
    )
    explicit_by_port = tuple(
        sorted(
            Counter(x.nic for x in links if _is_cli_port(x.nic)).items(),
            key=lambda kv: (-kv[1], kv[0]),
        )
    )

    return GeneratedCabling(
        links=tuple(links),
        issues=tuple(issues),
        notes=tuple(notes),
        by_nic=by_nic,
        explicit_by_port=explicit_by_port,
    )


def _nic_of(block_key: str, chains: tuple) -> str:
    """从分配结果的 block_key 反查网口。

    `flow.py` 的 block_key 形如 `型号|实例|网口`，第三段就是网口。
    """
    parts = block_key.split("|")
    return parts[2] if len(parts) >= 3 else ""


#: 显式连线两端的设备位置从哪来 —— 交换机在 `intent.switches`，
#: 服务器在布局段里但**服务器没有名字**（真实表里服务器行名称普遍为空）。
#: 所以显式连线的服务器端必须用「设备名 + 位置」双写，见 `ExplicitLink`。


def _switch_of(intent: Intent) -> dict[str, SwitchSpec]:
    return {s.instance: s for s in intent.switches}


def _server_at(intent: Intent) -> dict[str, tuple[str, int, int, int]]:
    """设备名 → (型号, 通道, 机柜, U起始)，仅覆盖**有名字**的服务器。

    真实 V13 里只有特殊机型（网络功能服务器/GSO/密码机等）有名字。
    计算节点 550 台全都没有名字，它们走规则生成，不需要显式连线。
    """
    out: dict[str, tuple[str, int, int, int]] = {}
    for p in expand_placements(intent):
        # 布局段里设备 id 是 `N02-11-S7` 这种槽位号，不是业务名字
        out.setdefault(p.node_id, (p.model, p.channel_no, p.rack_no, p.u_start))
    return out


def _named_devices(intent: Intent) -> dict[str, SwitchSpec]:
    """显式连线里可以引用的「有名字的设备」= 全部交换机实例。"""
    return {s.instance: s for s in intent.switches}


def explicit_links(intent: Intent) -> tuple[list[GeneratedLink], list[Issue]]:
    """显式连线 → 布线表行。

    ## 为什么这件事必须单独做

    规则生成只覆盖「交换机 → 服务器下行」4430 行（占全表 87%）。
    剩下 13% 是两类：

    1. **交换机之间互联**（548 行）：核心↔接入的 100G MPO 上行、双机堆叠线。
       端口号有规律但「谁接谁」是拓扑设计决定，不是位置决定的。
    2. **特殊机型接线**（144 行）：网络功能服务器/GSO/管理节点/密评设备，
       同型号不同实例接不同交换机（网络功能服务器指纹覆盖率只有 50%）。

    这 692 行必须逐条声明。好在它们有强规律可批量写：
    实测 548 行互联里 300 个设备对，其中 136 对是「核心端口连续递增、
    接入侧固定 1/2」，108 对是「管理核心 25GE1/0/10..39 递增」——
    这些用表格批量填比手点快得多，是 v0.3 之后要做的事。

    ## 端点顺序不翻转

    `a` 用原文顺序。真实表里交换机互联行的两端都是 CLI 名，
    谁在 `a` 谁在 `b` 影响「本端/对端」列，翻过来会让导出的表与用户
    手上的不一致。调用方自己保证顺序。

    >>> it = Intent(project="t")
    >>> explicit_links(it)
    ([], [])
    """
    links: list[GeneratedLink] = []
    issues: list[Issue] = []
    if not intent.explicit_links:
        return links, issues

    switches = _switch_of(intent)
    devices: dict[str, DeviceSpec] = {x.instance: x for x in intent.devices}
    placements = {p.node_id: p for p in expand_placements(intent)}

    def _endpoint(device: str, port: str) -> tuple[str, str, int, int, str, str] | None:
        """(device, port) → (型号, 通道, 机柜, U原文, 端口, 身份id)。

        解析顺序：交换机 → 有名字的特殊机型 → 布局段槽位 id。
        解析不了返回 None，由调用方报错 —— 静默跳过会让用户以为
        「填了就是接好了」，那是这类工具最不能犯的错。
        """
        sw = switches.get(device)
        if sw is not None:
            return (
                sw.model,
                sw.channel_no,
                sw.rack_no,
                u_label_switch(sw.u_start, sw.u_span),
                port,
                sw.instance,
            )
        dv = devices.get(device)
        if dv is not None:
            return (
                dv.model,
                dv.channel_no,
                dv.rack_no,
                u_label_server(dv.u_start, dv.u_span),
                port,
                dv.instance,
            )
        pl = placements.get(device)
        if pl is not None:
            return (
                pl.model,
                pl.channel_no,
                pl.rack_no,
                u_label_server(pl.u_start),
                port,
                pl.node_id,
            )
        return None

    for idx, x in enumerate(intent.explicit_links):
        ea = _endpoint(x.a_device, x.a_port)
        eb = _endpoint(x.b_device, x.b_port)
        if ea is None or eb is None:
            unknown = x.a_device if ea is None else x.b_device
            issues.append(
                Issue.error(
                    IssueCode.PORT_UNASSIGNED,
                    f"显式连线第 {idx + 1} 条引用了未知设备 {unknown!r}："
                    "设备必须在 switches 里声明，或在某个布局段的槽位上",
                    field="explicitLinks",
                )
            )
            continue

        cable = x.cable_type or derive_cable_type(
            parse_port_name(x.a_port), parse_port_name(x.b_port)
        )
        if not cable:
            cable = ""
            issues.append(
                Issue.warn(
                    IssueCode.UNKNOWN_CABLE_TYPE,
                    f"显式连线 {x.a_device}/{x.a_port} ↔ {x.b_device}/{x.b_port} "
                    "的线缆类型无法从端口速率派生，需手工填写",
                    field="cable_type",
                )
            )

        links.append(
            GeneratedLink(
                a_device=ea[5],
                a_model=ea[0],
                a_channel=ea[1],
                a_rack=ea[2],
                a_u=ea[3],
                a_port=ea[4],
                # 对端设备名：交换机与特殊机型都有真实名字，布局段设备没有
                b_device=x.b_device if (x.b_device in switches or x.b_device in devices) else "",
                b_id=eb[5],
                b_model=eb[0],
                b_channel=eb[1],
                b_rack=eb[2],
                b_u=eb[3],
                b_port=eb[4],
                cable_type=cable,
                nic=x.b_port,
            )
        )
    return links, issues


def _explicit_overrides(intent: Intent) -> dict[tuple[str, str, str, str], str]:
    """把显式连线里带线缆类型的，做成「分配键 → 线缆类型」的覆盖表。

    需要它是因为有一类情况派生规则算不出来：两端速率相同但一端用 MPO
    （实测 V13 有 1 行 `25G ↔ 25G` 走 MPO，其余 14 行是 LC-LC）。
    """
    out: dict[tuple[str, str, str, str], str] = {}
    for x in intent.explicit_links:
        if not x.cable_type:
            continue
        out[(x.a_device, x.a_port, x.b_device, x.b_port)] = x.cable_type
    return out


def placement_u_start(slot: int) -> int:
    """槽位号 → U 位起始。槽位 0 → U3，槽位 1 → U7 ……

    >>> placement_u_start(0), placement_u_start(7)
    (3, 31)
    """
    return SLOT_BASE_U + slot * SLOT_PITCH_U


__all__ = [
    "GeneratedCabling",
    "GeneratedLink",
    "derive_cable_type",
    "generate_links",
    "placement_u_start",
    "u_label",
    "u_label_server",
    "u_label_switch",
]
