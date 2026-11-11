"""端口台账的守护用例。

## 这个模块守护什么

`ledger.py` 唯一的产出是「一台设备用了几个口、还剩几个」，而这两个数字会
直接进现场判断（「这个柜还能不能再插 4 台」）。所以这里守的不是代码跑通，
而是**三个会让数字失真的错法**：

1. 端口身份用端口名而不是 `(组, 面板, 序号)` —— 同一物理口在布线表里可能
   写成 `25GE1/0/5` 也可能写成 `10GE1/0/5`，按名字数就是两个口。
2. 一条线只记本端、忘了对端 —— 服务器的端口永远是 0 个在用，而服务器恰恰
   是「还剩几个口」问题里最需要算准的部分（830 台里 566 台是服务器）。
3. 没有模板时返回 0 —— 「不知道几个口」被当成「0 个口」，利用率算成 0%。

## 为什么这些用例不接真实数据

真实数据的验证在 `.scratch/v05/verify_ledger.py`（830 台 / 0 越界 / 0 未知）。
这里用最小合成数据把每条规则钉死，坏掉时能直接指出是哪条规则被改坏了，
不用先跑一遍 5127 行再猜。
"""

from __future__ import annotations

import pytest

from idcbox.domain.ports.generate import GeneratedCabling, GeneratedLink
from idcbox.domain.ports.ledger import (
    DevicePorts,
    PortEntry,
    PortLedger,
    RawPortUse,
    build_ledger,
    ledger_from_cabling,
)


def _use(
    device: str,
    model: str,
    port: str,
    *,
    peer: str = "peer-1",
    peer_port: str = "peer-port-1",
    channel: int = 2,
    rack: int = 1,
    u: int = 40,
    span: int = 1,
    cable: str = "LC-LC",
) -> RawPortUse:
    return RawPortUse(
        side="a",
        device_key=device,
        device_label=device,
        model=model,
        channel_no=channel,
        rack_no=rack,
        u_start=u,
        u_span=span,
        port=port,
        peer_device=peer,
        peer_port=peer_port,
        cable_type=cable,
    )


def _one(ledger: PortLedger, label: str) -> DevicePorts:
    for d in ledger.devices:
        if d.device_label == label:
            return d
    raise AssertionError(f"台账里没有 {label}；现有 {[d.device_label for d in ledger.devices]}")


def _by_key(ledger: PortLedger, key: str) -> DevicePorts:
    """按设备身份查。

    服务器**没有真实的实例名**（实测 4230 行对端设备名为空），所以它的
    `device_key` 是生成器给的位置号（`N02-01-S0`），`device_label` 则是
    拼出来的可读标签（`计算节点A（ARM） @2-1-3-4U`）。两者不同，查服务器
    必须用 key —— 用 label 查会失败，而失败的原因跟台账正确性无关。
    """
    for d in ledger.devices:
        if d.device_key == key:
            return d
    raise AssertionError(f"台账里没有 key={key}；现有 {[d.device_key for d in ledger.devices]}")


class TestPortIdentity:
    """端口身份必须是 `(组, 面板, 序号)`。"""

    def test_same_physical_port_two_rate_names_is_one_port(self) -> None:
        """同一个物理口写成 `25GE1/0/5` 与 `10GE1/0/5` 只能算 1 个口。

        ## 这条用例的来源
        `业务接入交换机-1` 实测同时有 `10GE1/0/5` 与 `25GE1/0/5` ——
        它们是同一个 SFP28 口按不同速率协商后的两种写法。按端口名计数会
        把 56 口的设备数成 57 口，利用率算到 101%。
        """
        ledger = build_ledger(
            [
                _use("SW-1", "业务接入交换机1", "25GE1/0/5"),
                _use("SW-1", "业务接入交换机1", "10GE1/0/5"),
            ]
        )
        sw = _one(ledger, "SW-1")
        assert sw.used_count == 1, "同一物理口两种速率写法必须归并成 1 个"
        assert sw.used[0].bay == ("25GE", 1, 5)

    def test_different_index_is_different_port(self) -> None:
        ledger = build_ledger(
            [
                _use("SW-1", "业务接入交换机1", "25GE1/0/5"),
                _use("SW-1", "业务接入交换机1", "25GE1/0/6"),
            ]
        )
        assert _one(ledger, "SW-1").used_count == 2

    def test_different_panel_is_different_port(self) -> None:
        """`100GE1/0/1` 与 `100GE2/0/1` 是不同板卡上的不同口。"""
        ledger = build_ledger(
            [
                _use("SW-CORE", "业务核心交换机", "100GE1/0/1"),
                _use("SW-CORE", "业务核心交换机", "100GE2/0/1"),
            ]
        )
        sw = _one(ledger, "SW-CORE")
        assert sw.used_count == 2
        assert {e.bay for e in sw.used} == {("100GE", 1, 1), ("100GE", 2, 1)}

    def test_label_style_port_identity(self) -> None:
        """服务器标签式端口（`万兆1`）的身份是 `(万兆, 1, 1)`。"""
        ledger = build_ledger([_use("SRV-1", "计算节点A（ARM）", "万兆2")])
        srv = _one(ledger, "SRV-1")
        assert srv.used[0].bay == ("万兆", 1, 2)


class TestBothEndsRecorded:
    """一条线必须给两端都记一条占用。"""

    def test_both_sides_counted(self) -> None:
        """两端各一条 `RawPortUse` → 两台设备各 1 个口在用。

        ## 为什么这条必须单独测
        调用方（服务层）如果只给本端建 `RawPortUse`，交换机侧的利用率是对的，
        服务器侧永远是 0/7 —— 而服务器占全表 566/830 台，这个错会让
        「服务器还剩几个口」这个最常被问的问题永远答错，且界面上看不出异常
        （0 口在用看起来只是"没接线的服务器"）。
        """
        ledger = build_ledger(
            [
                _use("SW-1", "业务接入交换机1", "25GE1/0/1", peer="SRV-1", peer_port="万兆1"),
                _use(
                    "SRV-1",
                    "计算节点A（ARM）",
                    "万兆1",
                    peer="SW-1",
                    peer_port="25GE1/0/1",
                    u=3,
                    span=2,
                ),
            ]
        )
        assert _one(ledger, "SW-1").used_count == 1
        assert _one(ledger, "SRV-1").used_count == 1
        assert ledger.used_total == 2

    def test_peer_recorded_on_entry(self) -> None:
        ledger = build_ledger(
            [_use("SW-1", "业务接入交换机1", "25GE1/0/1", peer="SRV-1", peer_port="万兆1")]
        )
        entry = _one(ledger, "SW-1").used[0]
        assert entry.is_used
        assert entry.peer_device == "SRV-1"
        assert entry.peer_port == "万兆1"
        assert "SRV-1 万兆1" in entry.describe()


class TestMissingSpec:
    """没有模板 ≠ 0 个口。"""

    def test_unknown_model_has_no_spec_but_keeps_usage(self) -> None:
        ledger = build_ledger([_use("X-1", "还没核定的型号", "port1")])
        dev = _one(ledger, "X-1")
        assert dev.has_spec is False
        assert dev.spec is None
        assert dev.total_count == 0
        assert dev.ratio is None, "没有模板时利用率必须是 None，不能是 0.0"
        assert dev.used_count == 1, "没有模板不能丢掉已知的占用事实"
        assert dev.free_count == 0, "不知道总共几个口，就不能编造空闲口"
        assert dev in ledger.no_spec_devices

    def test_known_model_reports_free_ports(self) -> None:
        """有模板时才能说「还剩几个、在哪」。"""
        ledger = build_ledger([_use("SW-1", "业务接入交换机1", "25GE1/0/1")])
        sw = _one(ledger, "SW-1")
        assert sw.has_spec
        assert sw.total_count == 56, "48 × 25GE + 8 × 100GE"
        assert sw.free_count == 55
        assert ("25GE", 1, 2) in sw.free

    def test_free_port_entries_are_in_entries(self) -> None:
        """空闲口也要出现在 `entries` 里 —— 导出「全部端口状态」靠它。"""
        ledger = build_ledger([_use("SW-1", "业务接入交换机1", "25GE1/0/1")])
        idle = [e for e in ledger.entries if not e.is_used]
        assert len(idle) == 55
        assert all(e.device_key == "SW-1" for e in idle)


class TestAggregates:
    """汇总数字必须能对上明细。"""

    def test_totals_match_devices(self) -> None:
        ledger = build_ledger(
            [
                _use("SW-1", "业务接入交换机1", "25GE1/0/1"),
                _use("SW-1", "业务接入交换机1", "25GE1/0/2"),
                _use("SRV-1", "计算节点A（ARM）", "万兆1", u=3, span=2),
                _use("SRV-2", "计算节点A（ARM）", "万兆1", u=7, span=2),
            ]
        )
        assert ledger.device_count == 3
        assert ledger.used_total == sum(d.used_count for d in ledger.devices) == 4
        assert ledger.free_total == sum(d.free_count for d in ledger.devices)
        assert ledger.capacity_total == 56 + 9 + 9

    def test_no_spec_excluded_from_capacity(self) -> None:
        """没有模板的设备不能计入标称容量（否则分母偏小、利用率虚高）。"""
        ledger = build_ledger(
            [
                _use("SW-1", "业务接入交换机1", "25GE1/0/1"),
                _use("X-1", "还没核定的型号", "port1"),
            ]
        )
        assert ledger.capacity_total == 56, "只有 SW-1 的 56 口计入"
        assert len(ledger.no_spec_devices) == 1

    def test_over_used_is_structurally_empty(self) -> None:
        """`over_used` 应当恒为空 —— 越界端口变成 `unknown_ports`，不抬高 `used` 分母。

        ## 为什么保留这个「永远为真」的断言
        `used` 按端口身份去重，而超出模板的端口身份是 None、单独记进
        `unknown_ports`，所以 `used_count > total_count` 在当前设计下**不可能发生**。

        但这条断言仍然值得留：若哪天 `bay_key` 的归并逻辑被改坏（比如同一物理口
        因速率写法不同返回了两个不同身份），`used` 就会自己数到自己，
        `over_used` 立刻非空。那时这条会红，而其它用例可能全绿 ——
        因为它只在这一种回归下才有反应。

        同时它钉住了另一半事实：越界端口**不能从 `used` 里消失**，
        否则台账的「空闲口」里会冒出一个实际已被占用的口，现场照台账插线会撞车。
        """
        uses = [_use("SW-1", "带内交换机1", f"GE1/0/{i}") for i in range(1, 50)]
        ledger = build_ledger(uses)
        sw = _one(ledger, "SW-1")
        assert sw.used_count == 49, "48 个在模板内 + 1 个越界，占用事实都要保留"
        assert sw.total_count == 54, "48 × GE + 6 × 10GE"
        assert sw.unknown_ports == ("GE1/0/49",)
        assert ledger.over_used == (), "越界端口不抬高 used，所以 over_used 必须为空"

    def test_by_role_splits_downlink_and_uplink(self) -> None:
        """按角色分账 —— 现场最关心「上行口还剩几个」。"""
        ledger = build_ledger(
            [
                _use("SW-1", "带内交换机1", "GE1/0/1"),
                _use("SW-1", "带内交换机1", "10GE1/0/1"),
            ]
        )
        roles = _one(ledger, "SW-1").by_role()
        assert roles == {"downlink": 1, "uplink": 1}

    def test_by_rack_and_by_model_grouping(self) -> None:
        ledger = build_ledger(
            [
                _use("SW-1", "业务接入交换机1", "25GE1/0/1", channel=2, rack=1),
                _use("SW-2", "业务接入交换机1", "25GE1/0/1", channel=2, rack=2),
            ]
        )
        assert len(ledger.by_rack()) == 2
        assert len(ledger.by_rack()[(2, 1)]) == 1
        assert len(ledger.by_model()["业务接入交换机1"]) == 2


class TestEdgeCases:
    def test_empty_input_gives_empty_ledger(self) -> None:
        ledger = build_ledger([])
        assert ledger.device_count == 0
        assert ledger.entries == ()
        assert ledger.capacity_total == 0

    def test_port_outside_template_is_unknown_not_dropped(self) -> None:
        """端口号超出模板范围时，占用事实保留、但标成未知端口。

        ## 为什么不能丢
        丢了就等于「这个口没插线」，于是同一台设备的空闲口里会出现一个
        实际上已经占用的口 —— 现场按台账去插线会撞车。
        """
        ledger = build_ledger([_use("SW-1", "业务接入交换机1", "25GE1/0/99")])
        sw = _one(ledger, "SW-1")
        assert sw.used_count == 1
        assert sw.unknown_ports == ("25GE1/0/99",)
        assert sw in ledger.with_unknown_ports

    def test_unparseable_port_name_is_kept(self) -> None:
        """完全认不出的端口名（如 `GE8` 早期版本）也要保留。"""
        ledger = build_ledger([_use("SW-1", "业务接入交换机1", "奇怪端口")])
        sw = _one(ledger, "SW-1")
        assert sw.used_count == 1
        assert sw.unknown_ports == ("奇怪端口",)

    def test_duplicate_use_of_same_port_counts_once(self) -> None:
        """同一端口被记两次（源数据 1 处三行占同一口）只算 1 个口。"""
        ledger = build_ledger(
            [
                _use("SW-1", "分线接入交换机", "25GE1/0/48", peer="A", peer_port="p1"),
                _use("SW-1", "分线接入交换机", "25GE1/0/48", peer="B", peer_port="p2"),
            ]
        )
        sw = _one(ledger, "SW-1")
        assert sw.used_count == 1, "同一个口插不了两根线，物理上就是一个口"
        assert len(ledger.entries) == 1 + sw.free_count

    def test_devices_sorted_by_position(self) -> None:
        """台账顺序按 (通道, 机柜, U位)，与现场走线顺序一致。"""
        ledger = build_ledger(
            [
                _use("C", "业务接入交换机1", "25GE1/0/1", channel=2, rack=3),
                _use("A", "业务接入交换机1", "25GE1/0/1", channel=1, rack=9),
                _use("B", "业务接入交换机1", "25GE1/0/1", channel=2, rack=1),
            ]
        )
        assert [d.device_key for d in ledger.devices] == ["A", "B", "C"]

    def test_server_without_name_keeps_given_label(self) -> None:
        """服务器没有真实名字时，用调用方给的位置标签 —— 不能显示成空。"""
        ledger = build_ledger(
            [
                _use(
                    "N02-11-S7-1",
                    "计算节点A（ARM）",
                    "万兆1",
                    u=31,
                    span=2,
                )
            ]
        )
        dev = _one(ledger, "N02-11-S7-1")
        assert dev.device_label == "N02-11-S7-1"
        assert dev.u_span == 2
        assert "31" in dev.describe()


class TestDescribeOutput:
    """`describe()` 是界面上直接显示的文字，不能出现 `3UU` 这类错误。"""

    def test_describe_has_no_double_unit_suffix(self) -> None:
        ledger = build_ledger([_use("SW-1", "业务接入交换机1", "25GE1/0/1", u=40, span=1)])
        text = _one(ledger, "SW-1").describe()
        assert "3UU" not in text and "40UU" not in text
        assert "40U" in text
        assert "1/56" in text

    def test_describe_without_spec_says_so(self) -> None:
        ledger = build_ledger([_use("X-1", "还没核定的型号", "p1")])
        text = _one(ledger, "X-1").describe()
        assert "规格未知" in text


@pytest.mark.parametrize(
    ("model", "expected_total"),
    [
        ("业务接入交换机1", 56),
        ("带内交换机1", 54),
        ("业务核心交换机", 156),
        ("计算节点A（ARM）", 9),
        ("网络功能服务器", 7),
    ],
)
def test_spec_totals_are_pinned(model: str, expected_total: int) -> None:
    """模板总口数钉死 —— 改模板必须回来改这里。

    ## 为什么钉死
    分母错了利用率就全错，而利用率是个「看起来合理」的数字（比如 83%），
    不会引起怀疑。所以任何一次模板调整都必须在测试里显式留下痕迹。
    """
    ledger = build_ledger([_use("D-1", model, "25GE1/0/1")])
    assert _one(ledger, "D-1").total_count == expected_total


def test_port_entry_is_frozen() -> None:
    """台账条目不可变 —— 它会被多个视图共享。"""
    entry = PortEntry(
        device_key="d",
        device_label="d",
        model="m",
        channel_no=1,
        rack_no=1,
        u_start=1,
        physical_name="p",
        bay=None,
    )
    with pytest.raises(Exception):
        entry.peer_device = "x"  # type: ignore[misc]


class TestLedgerFromCabling:
    """从生成结果构造台账 —— 「一条线 → 两端各一个占用」的换算不许漏边。"""

    def _link(self, **kw) -> GeneratedLink:
        base = dict(
            a_device="SW-1",
            a_model="业务接入交换机1",
            a_channel=2,
            a_rack=1,
            a_u="40U",
            a_port="25GE1/0/1",
            b_device="",
            b_model="计算节点A（ARM）",
            b_channel=2,
            b_rack=1,
            b_u="3-4U",
            b_port="万兆1",
            cable_type="LC-LC",
            nic="万兆1",
            b_id="N02-01-S0",
        )
        base.update(kw)
        return GeneratedLink(**base)

    def _cabling(self, links: list[GeneratedLink]) -> GeneratedCabling:
        return GeneratedCabling(links=tuple(links), issues=())

    def test_both_ends_get_a_port(self) -> None:
        """一条线必须让两台设备各多一个在用的口。

        ## 为什么这是本函数唯一的验收点
        服务层只调它一次，之后所有数字（利用率、剩余口）都从台账出来。
        若只记 a 端，交换机利用率是对的、服务器永远是 0%，而界面上
        0% 看起来只是「这台服务器没接线」—— 不会引起任何怀疑。
        """
        led = ledger_from_cabling(self._cabling([self._link()]))
        assert led.device_count == 2
        assert led.used_total == 2
        sw = _one(led, "SW-1")
        srv = _by_key(led, "N02-01-S0")
        assert sw.used_count == 1 and srv.used_count == 1
        assert sw.used[0].peer_device == "N02-01-S0"
        assert srv.used[0].peer_device == "SW-1"

    def test_span_comes_from_u_label(self) -> None:
        """`3-4U` 要解析成跨度 2U，不是 1U —— 台账里设备高度靠它。

        ## 为什么必须在域层做
        服务层若自己解析 `a_u`/`b_u`，就要重复 `u_position_span` 对全角 `Ｕ`、
        `~`、`—` 的容错。两处实现一旦不一致，表现是「同一台设备在台账里
        出现两次、各占一半端口」，从界面上几乎看不出来。
        """
        led = ledger_from_cabling(self._cabling([self._link(b_u="19-20U")]))
        assert _by_key(led, "N02-01-S0").u_span == 2
        assert _by_key(led, "N02-01-S0").u_start == 19

    def test_server_without_name_uses_b_id(self) -> None:
        """`b_device` 为空时用 `b_id` 聚合 —— 实测 4230 行服务器没有名字。"""
        links = [
            self._link(b_port="万兆1", b_id="N02-01-S0"),
            self._link(b_port="万兆2", b_id="N02-01-S0"),
        ]
        led = ledger_from_cabling(self._cabling(links))
        srv = _by_key(led, "N02-01-S0")
        assert srv.used_count == 2, "两条线必须聚到同一台服务器，不是两台"

    def test_different_b_id_is_different_device(self) -> None:
        links = [
            self._link(b_port="万兆1", b_id="N02-01-S0"),
            self._link(b_port="万兆1", b_id="N02-01-S4"),
        ]
        led = ledger_from_cabling(self._cabling(links))
        assert led.device_count == 3, "1 台交换机 + 2 台服务器"

    def test_unparsable_u_skips_that_end_only(self) -> None:
        """U 位解析不出来时只跳过那一端，另一端的占用仍要记。

        ## 为什么要这样处理
        整条丢弃会让交换机少算一个在用口（台账说有空口，实际插着线）；
        两端都硬记又会编造一个不存在的 U 位。折中的代价最小：
        丢的那端在体检里会作为「无法定位的设备」暴露出来。
        """
        led = ledger_from_cabling(self._cabling([self._link(a_u="", b_u="3-4U")]))
        assert led.device_count == 1
        assert _by_key(led, "N02-01-S0").used_count == 1

    def test_empty_cabling(self) -> None:
        led = ledger_from_cabling(self._cabling([]))
        assert led.device_count == 0
        assert led.entries == ()
