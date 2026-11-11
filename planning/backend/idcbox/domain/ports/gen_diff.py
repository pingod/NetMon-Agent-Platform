"""两次生成结果之间的差异（纯函数，零 IO）。

## 这个模块存在的理由

`moveu.py` 证明了一件看着无害、实际很危险的事：**挪一台设备会改掉别的行**。

实测（V13 真数据，通道 2 机柜 1 的 `N02-01-S0` 从 1 号柜挪到 5 号柜）：

| 变化 | 行数 |
|---|---|
| 端口号（`10GE1/0/1` → `10GE1/0/33`） | **182 行**，涉及 7 台交换机 |
| 物理位置（U 位/机柜） | 7 行 |
| 线长（`T` 列档位） | 6 行 |

根因是端口分配规则 C2：`端口号 = 组内位次 + base`，而组内位次按
`(机柜升序, U 位升序)` 排。设备一换柜，它在组里的位次就变了，
**同一组里排在它后面的所有设备端口号整体前移**。

这不是 bug —— 规则就是这样的，真实 Excel 也这样。但它是**用户看不见的**：
界面上拖完只显示「已移到通道2 机柜5 的 3U」，没人告诉他 182 行端口变了。
现场按旧表插线，全部插错口。

所以这个模块回答一个问题：**这次改动到底动了哪些行**。

## 身份键为什么不含端口号

要检出的正是「端口号变了」，所以端口号不能进身份键 —— 否则
`10GE1/0/1` 消失 + `10GE1/0/33` 新增，会被报成「删了 1 行、加了 1 行」，
而实际是**同一根线换了口**。这两件事的处置完全不同：
前者要人重新设计接线，后者只是标签要重打。

身份键取**接线意图层面的业务关系**：`(交换机实例, 对端节点, 网口, 对端网口)`。
端口号与线缆类型都是**属性**，不是身份 —— 线缆类型换了仍是同一根线。

## 一对多的情况怎么配

实测有且仅有 1 组身份键重复：`分线接入交换机-1 ⇄ 分线接入交换机-2`
在 `25GE1/0/48` 上有 3 根线（两台交换机之间的堆叠/互联，走的是
`ExplicitLink`）。这种组内按 `(位置, 端口号)` 排序后逐个配对 ——
确定性、可重放。它们不参与端口重编号（显式连线的端口是写死的），
所以排序错配的风险只存在于「同一对交换机之间同网口多根线且端口号又变了」
这一种情形，实测为 0 行。**这是已知的近似，不是隐藏的假设。**

## 为什么在域层

它只做两件事：按业务键分组、组内配对比较。不读文件、不连库、不认 Excel。
长度影响要读几何，那是应用层的事（`application/services/impact.py`）。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable

from .generate import GeneratedCabling, GeneratedLink

#: 端口号变了（同一根线换了口）—— 改标签就行
CHANGE_PORT = "port"
#: 物理位置变了（机柜或 U 位）—— 影响线长，要关注
CHANGE_POSITION = "position"
#: 线缆类型变了 —— 影响耗材数量
CHANGE_CABLE = "cable"


@dataclass(frozen=True, slots=True)
class LinkIdentity:
    """一根线的**业务身份** —— 不含端口号，所以端口重编号能被检出。"""

    switch: str
    node: str
    nic: str
    peer_port: str

    def as_tuple(self) -> tuple[str, str, str, str]:
        return (self.switch, self.node, self.nic, self.peer_port)


@dataclass(frozen=True, slots=True)
class LinkSnapshot:
    """一行生成结果里**参与比较**的部分。

    刻意不含 `a_model` / `b_model`：型号由机型决定，挪位置不会改。
    留着它们只会让「同一行」的判定多两个永远相等的字段。
    """

    identity: LinkIdentity
    cable_type: str
    #: 对端型号。**不是身份、也不参与比较**，只用于把内部槽位 id
    #: （`N02-01-S6`）翻成人看得懂的话（「计算节点A（ARM） 27-28U」）。
    #: 舞台人员对 `N02-01-S6` 没有任何办法在现场找到那台机器。
    b_model: str
    a_channel: int
    a_rack: int
    a_u: str
    a_port: str
    b_channel: int
    b_rack: int
    b_u: str

    @property
    def position(self) -> tuple[int, int, int, int, str, str]:
        """物理位置六元组。换柜、换 U 都算位置变化。"""
        return (
            self.a_channel,
            self.a_rack,
            self.b_channel,
            self.b_rack,
            self.a_u,
            self.b_u,
        )

    @staticmethod
    def of(link: GeneratedLink) -> "LinkSnapshot":
        return LinkSnapshot(
            identity=LinkIdentity(
                switch=link.a_device,
                node=link.b_id or link.b_device or link.b_model,
                nic=link.nic,
                peer_port=link.b_port,
            ),
            cable_type=link.cable_type,
            b_model=link.b_model,
            a_channel=link.a_channel,
            a_rack=link.a_rack,
            a_u=link.a_u,
            a_port=link.a_port,
            b_channel=link.b_channel,
            b_rack=link.b_rack,
            b_u=link.b_u,
        )


@dataclass(frozen=True, slots=True)
class LinkDelta:
    """一条线的变化。`before` / `after` 恰有一个为 None 表示增删。"""

    identity: LinkIdentity
    before: LinkSnapshot | None
    after: LinkSnapshot | None
    #: 变化的类别（`CHANGE_PORT` / `CHANGE_POSITION` / `CHANGE_CABLE` 的子集）
    kinds: tuple[str, ...] = ()

    @property
    def is_added(self) -> bool:
        return self.before is None

    @property
    def is_removed(self) -> bool:
        return self.after is None

    @property
    def port_changed(self) -> bool:
        return CHANGE_PORT in self.kinds

    @property
    def position_changed(self) -> bool:
        return CHANGE_POSITION in self.kinds


@dataclass(frozen=True, slots=True)
class GenerationDiff:
    """两份生成结果的差异。

    ## 为什么增删与「改了」分开存

    增删意味着**接线拓扑变了**（要人重新看设计），而端口重编号只是
    **同一根线换了口**（改标签就行）。把两者混成一个 `changes` 列表，
    界面只能说「共 189 处变化」，用户不知道要不要停下来重新规划。
    """

    added: tuple[LinkDelta, ...] = ()
    removed: tuple[LinkDelta, ...] = ()
    changed: tuple[LinkDelta, ...] = ()

    @property
    def is_empty(self) -> bool:
        return not (self.added or self.removed or self.changed)

    @property
    def port_changes(self) -> tuple[LinkDelta, ...]:
        return tuple(d for d in self.changed if d.port_changed)

    @property
    def position_changes(self) -> tuple[LinkDelta, ...]:
        return tuple(d for d in self.changed if d.position_changed)

    @property
    def affected_switches(self) -> tuple[str, ...]:
        """端口号被动过的交换机（按名字排序，供界面点名）。"""
        return tuple(sorted({d.identity.switch for d in self.port_changes}))

    @property
    def affected_racks(self) -> tuple[tuple[int, int], ...]:
        """**换了柜**的机柜 `(通道, 机柜)`，搬出与搬入两端都算。

        ## 为什么只算换柜，不算改 U 位

        柜内上下挪一格，机柜编号没变 —— 界面要重画的是那张 U 位图，
        不是平面图上的机柜列表。把「改 U 位」也算进来，跨柜拖动时
        会返回一堆没换柜的机柜，界面上的「受影响的机柜」就成了噪音。

        想知道「有多少行的位置变了」用 `position_changes`。
        """
        out: set[tuple[int, int]] = set()
        for d in self.changed:
            if d.before is None or d.after is None:
                continue
            if (d.before.a_channel, d.before.a_rack) != (d.after.a_channel, d.after.a_rack):
                out.add((d.after.a_channel, d.after.a_rack))
                out.add((d.before.a_channel, d.before.a_rack))
            if (d.before.b_channel, d.before.b_rack) != (d.after.b_channel, d.after.b_rack):
                out.add((d.after.b_channel, d.after.b_rack))
                out.add((d.before.b_channel, d.before.b_rack))
        return tuple(sorted(out))

    @property
    def total(self) -> int:
        return len(self.added) + len(self.removed) + len(self.changed)


def snapshot_all(cabling: GeneratedCabling) -> tuple[LinkSnapshot, ...]:
    return tuple(LinkSnapshot.of(link) for link in cabling.links)


def _group(
    snaps: Iterable[LinkSnapshot],
) -> dict[tuple[str, str, str, str], list[LinkSnapshot]]:
    out: dict[tuple[str, str, str, str], list[LinkSnapshot]] = {}
    for s in snaps:
        out.setdefault(s.identity.as_tuple(), []).append(s)
    return out


def _pair_key(s: LinkSnapshot) -> tuple[object, ...]:
    """组内配对顺序：先按物理位置，再按端口号。

    同一身份键出现多次时（实测只有 1 组、3 行），靠它做确定性配对。
    见模块 docstring「一对多的情况怎么配」。
    """
    return (*s.position, s.a_port)


def _kinds(before: LinkSnapshot, after: LinkSnapshot) -> tuple[str, ...]:
    kinds: list[str] = []
    if before.a_port != after.a_port:
        kinds.append(CHANGE_PORT)
    if before.position != after.position:
        kinds.append(CHANGE_POSITION)
    if before.cable_type != after.cable_type:
        kinds.append(CHANGE_CABLE)
    return tuple(kinds)


def _as_snapshots(
    value: GeneratedCabling | Iterable[LinkSnapshot],
) -> tuple[LinkSnapshot, ...]:
    if isinstance(value, GeneratedCabling):
        return snapshot_all(value)
    return tuple(value)


def diff_generation(
    before: GeneratedCabling | Iterable[LinkSnapshot],
    after: GeneratedCabling | Iterable[LinkSnapshot],
) -> GenerationDiff:
    """比对两次生成结果。

    >>> from .generate import GeneratedCabling, GeneratedLink

    >>> def mk(port, u="3-4U"):
    ...     return GeneratedLink(
    ...         a_device="sw-1", a_model="M", a_channel=2, a_rack=2, a_u="40U",
    ...         a_port=port, b_device="", b_model="计算节点A（ARM）", b_channel=2,
    ...         b_rack=1, b_u=u, b_port="万兆1", cable_type="LC-LC", nic="万兆1",
    ...         b_id="N02-01-S0",
    ...     )

    >>> d = diff_generation(
    ...     GeneratedCabling(links=(mk("10GE1/0/1"),), issues=()),
    ...     GeneratedCabling(links=(mk("10GE1/0/33"),), issues=()),
    ... )
    >>> len(d.changed), d.changed[0].port_changed, d.changed[0].position_changed
    (1, True, False)
    >>> d.affected_switches
    ('sw-1',)

    位置变化与端口变化可以同时发生（换柜后位次变、位置也变）：

    >>> d2 = diff_generation(
    ...     GeneratedCabling(links=(mk("10GE1/0/1"),), issues=()),
    ...     GeneratedCabling(links=(mk("10GE1/0/33", "5-6U"),), issues=()),
    ... )
    >>> sorted(d2.changed[0].kinds), d2.affected_racks
    (['port', 'position'], ())

    `affected_racks` 只在**换柜**时非空（改 U 位不算换柜）：

    >>> mk2 = lambda rack, port="10GE1/0/1": GeneratedLink(  # noqa: E731
    ...     a_device="sw-1", a_model="M", a_channel=2, a_rack=2, a_u="40U",
    ...     a_port=port, b_device="", b_model="计算节点A（ARM）", b_channel=2,
    ...     b_rack=rack, b_u="3-4U", b_port="万兆1", cable_type="LC-LC",
    ...     nic="万兆1", b_id="N02-01-S0",
    ... )
    >>> d2b = diff_generation(
    ...     GeneratedCabling(links=(mk2(1),), issues=()),
    ...     GeneratedCabling(links=(mk2(5),), issues=()),
    ... )
    >>> d2b.affected_racks
    ((2, 1), (2, 5))

    增删是拓扑变化，与「改了」分开报：

    >>> d3 = diff_generation(
    ...     GeneratedCabling(links=(mk("10GE1/0/1"),), issues=()),
    ...     GeneratedCabling(links=(), issues=()),
    ... )
    >>> len(d3.removed), len(d3.changed), d3.is_empty
    (1, 0, False)
    """
    gb = _group(_as_snapshots(before))
    ga = _group(_as_snapshots(after))

    added: list[LinkDelta] = []
    removed: list[LinkDelta] = []
    changed: list[LinkDelta] = []

    for key, a_list in ga.items():
        b_list = gb.get(key)
        if not b_list:
            added.extend(
                LinkDelta(identity=s.identity, before=None, after=s) for s in a_list
            )
            continue
        b_sorted = sorted(b_list, key=_pair_key)
        a_sorted = sorted(a_list, key=_pair_key)
        # 数量不等时：先配对，剩下的算增删。实测未出现（身份键已含网口与对端口），
        # 但真数据的显式连线随时可能多一根 —— 宁可报「新增 1 行」也不要静默吞掉。
        for i, a_snap in enumerate(a_sorted):
            if i >= len(b_sorted):
                added.append(LinkDelta(identity=a_snap.identity, before=None, after=a_snap))
                continue
            b_snap = b_sorted[i]
            kinds = _kinds(b_snap, a_snap)
            if kinds:
                changed.append(
                    LinkDelta(
                        identity=a_snap.identity,
                        before=b_snap,
                        after=a_snap,
                        kinds=kinds,
                    )
                )
        for b_snap in b_sorted[len(a_sorted):]:
            removed.append(LinkDelta(identity=b_snap.identity, before=b_snap, after=None))

    for key, b_list in gb.items():
        if key in ga:
            continue
        removed.extend(LinkDelta(identity=s.identity, before=s, after=None) for s in b_list)

    return GenerationDiff(
        added=tuple(added), removed=tuple(removed), changed=tuple(changed)
    )


__all__ = [
    "CHANGE_CABLE",
    "CHANGE_PORT",
    "CHANGE_POSITION",
    "GenerationDiff",
    "LinkDelta",
    "LinkIdentity",
    "LinkSnapshot",
    "diff_generation",
    "snapshot_all",
]
