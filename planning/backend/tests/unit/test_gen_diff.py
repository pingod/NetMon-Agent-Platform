"""`gen_diff` 的守卫 —— 「挪一台设备到底改了哪些行」。

## 这个模块守护的东西

它存在的唯一理由是：**拖动会改掉用户没选中的行**。实测把 V13 通道2 机柜1
的一台计算节点挪到 5 号柜，91 行端口号变了、6 行线长变了，而旧界面只说
「已移到 5 号柜 3U」。

所以这组用例锁三件事：

1. **端口重编号必须被检出**（身份键不能含端口号，否则会被报成增删）；
2. **「改了」与「增删」必须分开报**（前者改标签，后者改设计）；
3. **配对是确定性的**（同一份输入跑两次结果逐字相同）。

## 为什么用真实生成器的形状构造样本

`GeneratedLink` 有 15 个字段，用 `dict` 造假数据会让「字段改名了但测试还绿」。
用真 dataclass，字段改名时这里会直接炸 —— 那正是我们要的提醒。
"""

from __future__ import annotations

import pytest

from idcbox.domain.ports.generate import GeneratedCabling, GeneratedLink
from idcbox.domain.ports.gen_diff import (
    CHANGE_CABLE,
    CHANGE_PORT,
    CHANGE_POSITION,
    diff_generation,
    snapshot_all,
)


def mk(
    *,
    switch: str = "sw-1",
    node: str = "N02-01-S0",
    nic: str = "万兆1",
    peer_port: str = "万兆1",
    port: str = "10GE1/0/1",
    a_rack: int = 2,
    a_u: str = "40U",
    b_rack: int = 1,
    b_u: str = "3-4U",
    cable: str = "LC-LC",
) -> GeneratedLink:
    return GeneratedLink(
        a_device=switch,
        a_model="计算接入交换机(业务)1",
        a_channel=2,
        a_rack=a_rack,
        a_u=a_u,
        a_port=port,
        b_device="",
        b_model="计算节点A（ARM）",
        b_channel=2,
        b_rack=b_rack,
        b_u=b_u,
        b_port=peer_port,
        cable_type=cable,
        nic=nic,
        b_id=node,
    )


def cab(*links: GeneratedLink) -> GeneratedCabling:
    return GeneratedCabling(links=tuple(links), issues=())


class TestIdentity:
    """身份键的口径。"""

    def test_port_change_is_not_add_remove(self) -> None:
        """★ 端口号变了必须是「改了」，不是「删一行加一行」。

        这是整个模块的存在理由。身份键若含端口号，`10GE1/0/1` 消失 +
        `10GE1/0/33` 新增会被报成 1 增 1 删 —— 而实际是同一根线换了口，
        处置完全不同（前者要重打标签，后者要重新设计接线）。
        """
        d = diff_generation(cab(mk(port="10GE1/0/1")), cab(mk(port="10GE1/0/33")))
        assert len(d.added) == 0
        assert len(d.removed) == 0
        assert len(d.changed) == 1
        assert d.changed[0].port_changed
        assert not d.changed[0].position_changed

    def test_cable_type_change_detected(self) -> None:
        """线缆类型变了仍是同一根线 —— 属性变了，身份没变。

        若把 `cable_type` 也放进身份键，这条会变成 1 增 1 删，
        而耗材汇总会看到「LC-LC 少一根、MPO 多一根」，对不上是同一根线改规格。
        """
        d = diff_generation(cab(mk(cable="LC-LC")), cab(mk(cable="MPO")))
        assert len(d.changed) == 1
        assert d.changed[0].kinds == (CHANGE_CABLE,)
        assert len(d.added) == 0 and len(d.removed) == 0

    def test_position_change_detected(self) -> None:
        d = diff_generation(cab(mk(b_rack=1)), cab(mk(b_rack=5)))
        assert len(d.changed) == 1
        assert d.changed[0].kinds == (CHANGE_POSITION,)

    def test_port_and_position_both(self) -> None:
        """换柜的典型形态：位次变（端口号变）+ 位置变。"""
        d = diff_generation(
            cab(mk(port="10GE1/0/1", b_rack=1)),
            cab(mk(port="10GE1/0/33", b_rack=5)),
        )
        assert sorted(d.changed[0].kinds) == [CHANGE_PORT, CHANGE_POSITION]

    def test_identical_is_empty(self) -> None:
        d = diff_generation(cab(mk()), cab(mk()))
        assert d.is_empty
        assert d.total == 0


class TestAddRemove:
    """增删 = 接线拓扑变了。"""

    def test_added(self) -> None:
        d = diff_generation(cab(mk()), cab(mk(), mk(node="N02-01-S1")))
        assert len(d.added) == 1
        assert len(d.removed) == 0
        assert d.added[0].is_added
        assert d.added[0].after is not None

    def test_removed(self) -> None:
        d = diff_generation(cab(mk(), mk(node="N02-01-S1")), cab(mk()))
        assert len(d.removed) == 1
        assert d.removed[0].is_removed

    def test_removing_everything_is_not_silently_empty(self) -> None:
        """★ 空结果必须报「全删」，不能报「什么都没变」。

        这条守的是 `impact.py` 曾经的真 bug：移动被拒绝时用
        `diff_generation(before, ())` 表示「没有改动后的结果」，
        接口于是返回 `removed: 5127` —— 一个失败的动作配一段最坏的消息。
        正确做法是失败时根本不该构造 diff（见 `impact._failed`），
        而「真的删光了」这种情形则必须老实报出来。
        """
        d = diff_generation(cab(mk(), mk(node="N02-01-S1")), cab())
        assert len(d.removed) == 2
        assert not d.is_empty

    def test_same_switch_same_node_different_nic_is_two_rows(self) -> None:
        """同一台节点走两个网口 = 两根线（不同 nic），不能被当成一行。"""
        d = diff_generation(
            cab(mk(nic="万兆1", peer_port="万兆1"), mk(nic="IPMI", peer_port="IPMI")),
            cab(mk(nic="万兆1", peer_port="万兆1"), mk(nic="IPMI", peer_port="IPMI")),
        )
        assert d.is_empty


class TestOneToManyPairing:
    """同一身份键出现多次时的配对。"""

    def test_duplicate_identity_pairs_deterministically(self) -> None:
        """实测只有 1 组：`分线接入交换机-1 ⇄ -2` 在 `25GE1/0/48` 上 3 根线。

        组内按 `(位置, 端口号)` 排序后逐个配对 —— 同样输入跑两次必须
        得到逐字相同的结果，否则界面上「哪一行变成哪一行」会随机跳。
        """
        before = cab(
            mk(switch="分线接入交换机-1", node="分线接入交换机-2", port="25GE1/0/48", peer_port="25GE1/0/48"),
            mk(switch="分线接入交换机-1", node="分线接入交换机-2", port="100GE1/0/7", peer_port="25GE1/0/48"),
            mk(switch="分线接入交换机-1", node="分线接入交换机-2", port="100GE1/0/8", peer_port="25GE1/0/48"),
        )
        after = cab(
            mk(switch="分线接入交换机-1", node="分线接入交换机-2", port="25GE1/0/48", peer_port="25GE1/0/48"),
            mk(switch="分线接入交换机-1", node="分线接入交换机-2", port="100GE1/0/7", peer_port="25GE1/0/48"),
            mk(switch="分线接入交换机-1", node="分线接入交换机-2", port="100GE1/0/8", peer_port="25GE1/0/48"),
        )
        a = diff_generation(before, after)
        b = diff_generation(before, after)
        assert a.is_empty
        assert [d.identity.as_tuple() for d in a.changed] == [
            d.identity.as_tuple() for d in b.changed
        ]

    def test_count_mismatch_reports_added_not_swallowed(self) -> None:
        """组内多出一根 → 报「新增 1 行」。宁可多报也不要静默吞掉。"""
        two = (
            mk(port="100GE1/0/7", peer_port="25GE1/0/48"),
            mk(port="100GE1/0/8", peer_port="25GE1/0/48"),
        )
        d = diff_generation(cab(*two), cab(*two, mk(port="100GE1/0/9", peer_port="25GE1/0/48")))
        assert len(d.added) == 1
        assert len(d.removed) == 0


class TestAggregates:
    """聚合口径 —— 界面上的数字直接取这些。"""

    def test_affected_switches_dedup_sorted(self) -> None:
        d = diff_generation(
            cab(mk(switch="b", port="1"), mk(switch="a", port="1"), mk(switch="b", port="2")),
            cab(mk(switch="b", port="9"), mk(switch="a", port="9"), mk(switch="b", port="8")),
        )
        assert d.affected_switches == ("a", "b")

    def test_affected_racks_only_on_rack_change(self) -> None:
        """改 U 位不算换柜 —— 界面要重画的是 U 位图，不是平面图。"""
        d = diff_generation(cab(mk(b_u="3-4U")), cab(mk(b_u="35-36U")))
        assert d.position_changes
        assert d.affected_racks == ()

    def test_affected_racks_both_ends(self) -> None:
        d = diff_generation(cab(mk(b_rack=1)), cab(mk(b_rack=5)))
        assert d.affected_racks == ((2, 1), (2, 5))

    def test_total_is_sum(self) -> None:
        d = diff_generation(
            cab(mk(node="n1"), mk(node="n2", port="p2")),
            cab(mk(node="n1", port="p9"), mk(node="n3", port="p3")),
        )
        assert d.total == len(d.added) + len(d.removed) + len(d.changed)
        assert d.total == 3  # n1 改端口、n2 删、n3 增


class TestSnapshot:
    def test_snapshot_uses_slot_id_not_display_name(self) -> None:
        """身份里的 node 取 `b_id`（槽位号），不用 `b_device`（显示名可空）。

        显示名在真实数据里 100% 为空（服务器行），拿它做身份键会把
        所有行塌缩成同一个键。
        """
        s = snapshot_all(cab(mk(node="N02-01-S0")))[0]
        assert s.identity.node == "N02-01-S0"

    def test_snapshot_falls_back_when_id_empty(self) -> None:
        """`b_id` 缺失时退回 `b_device` → `b_model`，不产生空身份。"""
        link = mk()
        object.__setattr__(link, "b_id", "")
        s = snapshot_all(cab(link))[0]
        assert s.identity.node  # 非空

    @pytest.mark.parametrize("field", ["a_u", "b_u", "a_rack", "b_rack"])
    def test_position_covers_both_ends(self, field: str) -> None:
        """位置变化必须覆盖两端 —— 只比本端会漏掉「对端换柜」。

        注意基准里 `a_rack=2 / b_rack=1`，所以改 rack 必须真的换一个值。
        （最早写成 `after.b_rack + 1`，在 `a_rack` 这一项上恰好算出 2 ——
        与基准相同，于是「没检出变化」，红的是测试不是实现。）
        """
        before = mk()
        after = mk()
        object.__setattr__(after, field, 9 if "rack" in field else "99U")
        d = diff_generation(cab(before), cab(after))
        assert d.position_changes, f"{field} 变了却没被检出"
