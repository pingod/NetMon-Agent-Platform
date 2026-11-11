"""`labels` 的守卫 —— 线缆标签清单。

## 这个模块守护的东西

标签是唯一会被**打印出来贴到设备上**的交付物。表算错了可以重算，
标签贴错了要一根一根撕。所以这组用例锁的是现场真会踩的坑：

1. **三段结构不能被空格破坏**。端口名里带空格 → 打印机按空格切段，
   切出四段后位置/编码/端口全错位，而人眼扫过去「看着挺像」。
2. **缺编码不能拼出双空格**。631 台服务器没有实例名，直接 join 会得到
   `12A01-23U  10GE1` —— 现场会以为中间漏印了一截。
3. **自动编号必须按设备编号，不是按标签编号**。一台服务器 7 个口出 7 张
   标签，按标签编号会让同一台机器印上 7 个不同编码（实测踩过：
   `2A01-31U` 出现 SRV03/27/51/75/147）。
4. **自动编号必须全局唯一**。曾按通道独立计数 → 通道 2/3/8/9 各自从 01 开始
   → 4230 张标签只有 235 个唯一编码，`SRV80` 同时出现在 4 个机柜。
5. **排序按施工顺序**：机柜升序、U 位降序（柜顶先贴）、端口升序。
   按 U 位升序会让打线的人在柜子里来回弯腰几十次。
6. **槽位号不能印上标签**。`N02-01-S0` 是程序内部 id，印上去现场会去找一台
   叫这个名字的机器。

## 为什么这些必须写在域层用例里

标签的错法全部是「看起来没问题」的类型：双空格、重复编码、段数错位。
它们不会抛异常、不会让页面变红，只会让施工方在几百米外的机柜前发现对不上。
"""

from __future__ import annotations

import pytest

from idcbox.application.services.labelsvc import (
    SERVER_CODE_TEMPLATE,
    category_of,
    number_missing,
    targets_from_links,
)
from idcbox.domain.exporting.labels import (
    MAX_LABEL_CHARS,
    LabelTarget,
    build_sheet,
    check_label,
    render_label,
    sort_labels,
    sort_key,
)


class TestRender:
    def test_switch_gets_sw_suffix(self) -> None:
        """交换机编码后加 `(SW)` —— 提示后面那段是 CLI 端口名。"""
        got = render_label(LabelTarget("11A14", 16, "HLW-YWHX01", "40GE2/0/1", "switch")).joined()
        assert got == "11A14-16U HLW-YWHX01(SW) 40GE2/0/1"

    def test_server_has_no_suffix(self) -> None:
        got = render_label(LabelTarget("12A01", 23, "HLW-SRV01", "10GE1", "server")).joined()
        assert got == "12A01-23U HLW-SRV01 10GE1"

    def test_core_is_switch(self) -> None:
        """`core` 也加 `(SW)` —— 端口名同样是 CLI 口径。"""
        got = render_label(LabelTarget("1A01", 3, "CORE01", "100GE1/0/1", "core")).joined()
        assert "(SW)" in got

    def test_missing_code_single_space(self) -> None:
        """缺编码时**单空格**，不是双空格。

        双空格会被人工核对当成「漏印一段」，而实际是上游没填。
        """
        got = render_label(LabelTarget("12A01", 23, "", "10GE1", "server")).joined()
        assert got == "12A01-23U 10GE1"
        assert "  " not in got

    def test_u_no_leading_zero(self) -> None:
        got = render_label(LabelTarget("12A01", 3, "X", "GE1", "server")).joined()
        assert got.startswith("12A01-3U ")


class TestCheck:
    def test_ok_label_has_no_issue(self) -> None:
        assert check_label(render_label(LabelTarget("11A14", 16, "X", "40GE1", "switch"))) == ()

    def test_port_with_space_is_error(self) -> None:
        """端口名带空格 → 三段变四段，打印机与人都会按错位截断。"""
        issues = check_label(render_label(LabelTarget("1A1", 3, "X", "GE 1", "server")))
        assert [i.code for i in issues] == ["PORT_HAS_SPACE"]
        assert issues[0].severity == "error"

    def test_missing_code_is_info_not_error(self) -> None:
        """缺编码只是提示 —— 报 error 会让 4230 张标签整份导不出。

        而「位置 + 端口」其实已能定位到具体哪一个口。
        """
        issues = check_label(render_label(LabelTarget("1A1", 3, "", "GE1", "server")))
        assert [i.code for i in issues] == ["NO_DEVICE_CODE"]
        assert issues[0].severity == "info"

    def test_missing_port_is_error(self) -> None:
        issues = check_label(render_label(LabelTarget("1A1", 3, "X", "", "server")))
        assert "NO_PORT" in [i.code for i in issues]

    def test_too_long(self) -> None:
        long_port = "X" * MAX_LABEL_CHARS
        issues = check_label(render_label(LabelTarget("1A1", 3, "CODE", long_port, "server")))
        assert "TOO_LONG" in [i.code for i in issues]


class TestSort:
    def test_rack_asc_u_desc_port_asc(self) -> None:
        """机柜升序 → U 位**降序** → 端口升序（柜顶先贴）。"""
        a = LabelTarget("2A01", 3, "X", "GE1", "server")
        b = LabelTarget("1A05", 31, "X", "GE1", "server")
        c = LabelTarget("1A05", 3, "X", "GE1", "server")
        d = LabelTarget("1A05", 3, "X", "GE10", "server")
        got = [(t.rack_code, t.u_start, t.port_name) for t in sort_labels([a, b, c, d])]
        assert got == [("1A05", 31, "GE1"), ("1A05", 3, "GE1"), ("1A05", 3, "GE10"), ("2A01", 3, "GE1")]

    def test_key_uses_natural_rack_and_port(self) -> None:
        """排序键首段是**自然拆解后的元组**，不是原始字符串。

        若改回字符串键，上面两条 `TestNaturalSort` 会立刻变红。
        """
        key = sort_key(LabelTarget("1A5", 5, "X", "GE1"))
        assert key[0] == (1, "A", 5)
        assert key[1] == -5
        assert isinstance(key[2], tuple)  # 端口走 sort_key_port 的四元组


class TestNaturalSort:
    """排序必须按**数字序**，不能按字符串序。

    ## 两个实测踩过的坑

    - 机柜码：字符串序下 `12A02` < `1A01`（第二位 `2`(0x32) < `A`(0x41)），
      于是通道 12 的标签排在通道 1 前面 —— 一份按通道分工的标签被打乱。
    - 端口名：字符串序下 `100GE1/0/10` < `100GE1/0/2`，现场按顺序贴会从
      1 跳到 10 再跳回 2。实测 30 张连续标签里 19 张是乱序的。
    """

    def test_rack_code_numeric_order(self) -> None:
        targets = [LabelTarget(r, 3, "X", "GE1", "s") for r in ("1A01", "12A02", "2A01")]
        assert [t.rack_code for t in sort_labels(targets)] == ["1A01", "2A01", "12A02"]

    def test_port_numeric_order(self) -> None:
        ports = ("100GE1/0/1", "100GE1/0/10", "100GE1/0/2", "100GE1/0/20")
        targets = [LabelTarget("1A01", 3, "X", p, "switch") for p in ports]
        assert [t.port_name for t in sort_labels(targets)] == [
            "100GE1/0/1",
            "100GE1/0/2",
            "100GE1/0/10",
            "100GE1/0/20",
        ]

    def test_label_style_ports_sorted_by_index(self) -> None:
        """服务器的 `千兆1/千兆3/千兆10` 同样按数字序。"""
        targets = [LabelTarget("2A01", 31, "S", p, "server") for p in ("千兆10", "千兆1", "千兆3")]
        assert [t.port_name for t in sort_labels(targets)] == ["千兆1", "千兆3", "千兆10"]

    def test_ports_grouped_by_rate(self) -> None:
        """**速率降序优先**，同速率内再按板卡/槽位/序号。

        同一台核心交换机上 `100GE1/0/1` 与 `25GE1/0/1` 的 (chassis, slot, port)
        都是 (1,0,1)，只看它们会交替排出两种速率 —— 现场理线会把两种线缠在一起。
        按速率分组后排成干净的几段，与「线缆按类型成捆」的实际做法一致。
        """
        ports = ("25GE1/0/1", "100GE1/0/1", "25GE1/0/2", "100GE1/0/2")
        targets = [LabelTarget("1A01", 3, "X", p, "core") for p in ports]
        assert [t.port_name for t in sort_labels(targets)] == [
            "100GE1/0/1",
            "100GE1/0/2",
            "25GE1/0/1",
            "25GE1/0/2",
        ]

    def test_u_still_descending(self) -> None:
        """自然排序不能破坏「U 位降序」这个前提。"""
        targets = [LabelTarget("1A01", u, "X", "GE1", "s") for u in (3, 31, 15)]
        assert [t.u_start for t in sort_labels(targets)] == [31, 15, 3]


class TestCategory:
    @pytest.mark.parametrize(
        "model,expected",
        [
            ("业务核心交换机", "core"),
            ("存储核心交换机", "core"),
            ("业务接入交换机 1", "switch"),
            ("带内交换机1", "switch"),
            ("服务器密码机", "security"),
            ("数据库加密机", "security"),
            ("签名服务器", "security"),
            ("EHDD云硬盘服务器1", "server"),
            ("GSO独立部署服务器", "server"),
            # 「存储」优先于「节点」：`对象存储数据节点服务器` 同时含两者，
            # 与 `svg.categorize()` 一致判成 storage（存储是更具体的归类）。
            ("对象存储数据节点服务器", "storage"),
            ("某某存储阵列", "storage"),
            ("不认识的东西", "other"),
        ],
    )
    def test_category_of(self, model: str, expected: str) -> None:
        """分类顺序必须与 `RackView.deviceColor()`、`svg.categorize()` 一致。

        曾经「服务器」排在「安全设备」之前，导致 `服务器密码机`、`签名服务器`
        被画成计算节点黄色 —— 屏幕上黄、纸上红，现场会以为拿错图。
        """
        assert category_of(model) == expected

    def test_security_before_server(self) -> None:
        """`服务器密码机` 必须判成安全设备，不能因为含「服务器」就归到 server。"""
        assert category_of("服务器密码机") == "security"


class _FakeRack:
    def __init__(self, ch: int, seq: int) -> None:
        self.channel_no = ch
        self.seq_no = seq


class _FakeEnd:
    def __init__(self, ch: int, seq: int, u: str, name: str, port: str, model: str) -> None:
        self.rack = _FakeRack(ch, seq)
        self.u_position = u
        self.device_name = name
        self.port_name = port
        self.device_model = model


class _FakeLink:
    def __init__(self, a: _FakeEnd, b: _FakeEnd) -> None:
        self.a = a
        self.b = b


class _FakeInv:
    pass


def _links(*pairs: tuple[_FakeEnd, _FakeEnd]) -> list[_FakeLink]:
    return [_FakeLink(a, b) for a, b in pairs]


class TestTargets:
    def test_both_ends_produce_labels(self) -> None:
        """一根线出**两张**标签（两端各一）。

        只出一张是最容易犯的错：那份「标签清单」看起来也像模像样，
        但现场贴完才发现有一端全是手写。
        """
        link = _FakeLink(
            _FakeEnd(2, 1, "3-4U", "SW-1", "万兆1", "业务接入交换机 1"),
            _FakeEnd(2, 1, "7-8U", "", "千兆1", "计算节点A（ARM）"),
        )
        targets = targets_from_links([link], _FakeInv())
        assert len(targets) == 2
        assert targets[0].device_code == "SW-1"
        assert targets[1].device_code == ""  # 服务器无名

    def test_rack_code_from_channel_and_seq(self) -> None:
        link = _FakeLink(
            _FakeEnd(11, 14, "16U", "X", "GE1", "switch"),
            _FakeEnd(11, 14, "16U", "X", "GE2", "switch"),
        )
        assert targets_from_links([link], _FakeInv())[0].rack_code == "11A14"

    def test_rack_code_zero_padded(self) -> None:
        link = _FakeLink(
            _FakeEnd(12, 2, "41U", "X", "GE1", "switch"),
            _FakeEnd(12, 2, "41U", "X", "GE2", "switch"),
        )
        assert targets_from_links([link], _FakeInv())[0].rack_code == "12A02"

    def test_bad_u_is_skipped(self) -> None:
        """U 位解析不出来时跳过该端 —— 印 `-U` 比印错数字好。"""
        link = _FakeLink(
            _FakeEnd(2, 1, "坏值", "X", "GE1", "switch"),
            _FakeEnd(2, 1, "7U", "Y", "GE2", "switch"),
        )
        targets = targets_from_links([link], _FakeInv())
        assert len(targets) == 1

    def test_slot_id_never_printed(self) -> None:
        """槽位号 `N02-01-S0` 是内部 id，绝不能出现在标签上。"""
        link = _FakeLink(
            _FakeEnd(2, 1, "3U", "N02-01-S0", "万兆1", "计算节点A（ARM）"),
            _FakeEnd(2, 1, "3U", "N02-11-S7-1", "万兆1", "计算节点A（ARM）"),
        )
        targets = targets_from_links([link], _FakeInv())
        assert all(t.device_code == "" for t in targets)


def _targets(positions: list[tuple[str, int, str]], category: str = "server") -> tuple[LabelTarget, ...]:
    return tuple(LabelTarget(rack, u, "", port, category) for rack, u, port in positions)


class TestNumberMissing:
    def test_same_device_same_code(self) -> None:
        """**一台设备的多个端口必须同一个编码。**

        曾经按标签编号 → `2A01-31U` 的 7 张标签得到 SRV03/27/51/75/147
        五个不同编码，现场按编码找线会找到 5 台不存在的设备。
        """
        targets = _targets([("2A01", 31, p) for p in ("IPMI", "万兆1", "万兆2", "千兆1")])
        out = number_missing(targets)
        assert len({t.device_code for t in out}) == 1
        assert out[0].device_code == "SRV01"

    def test_different_devices_different_codes(self) -> None:
        targets = _targets([("2A01", 31, "IPMI"), ("2A01", 27, "IPMI")])
        out = number_missing(targets)
        assert len({t.device_code for t in out}) == 2

    def test_global_unique_not_per_channel(self) -> None:
        """**必须全项目唯一**。曾按通道独立计数 → 4 个通道各从 01 开始。

        实测后果：4230 张标签只有 235 个唯一编码，`SRV80` 同时出现在
        2A13/3A16/8A11/9A11 四个机柜。
        """
        targets = _targets(
            [("2A01", 31, "GE1"), ("3A01", 31, "GE1"), ("8A01", 31, "GE1"), ("9A01", 31, "GE1")]
        )
        codes = [t.device_code for t in number_missing(targets)]
        assert len(set(codes)) == 4

    def test_category_has_separate_sequence(self) -> None:
        """服务器与存储各编各的号 —— 现场说的「存储第 3 台」不是「服务器第 3 台」。"""
        server = _targets([("2A01", 31, "GE1")], "server")
        storage = _targets([("3A01", 31, "GE1")], "storage")
        out = number_missing(server + storage)
        assert {t.device_code for t in out} == {"SRV01", "STG01"}

    def test_existing_codes_untouched(self) -> None:
        targets = (
            LabelTarget("1A01", 3, "业务核心交换机-1", "100GE1/0/1", "core"),
            LabelTarget("2A01", 31, "", "GE1", "server"),
        )
        out = number_missing(targets)
        assert out[0].device_code == "业务核心交换机-1"
        assert out[1].device_code == "SRV01"

    def test_custom_template(self) -> None:
        out = number_missing(_targets([("2A01", 31, "GE1")]), template="{model}A{seq:02d}")
        assert out[0].device_code == "SRVA01"

    def test_default_template_shape(self) -> None:
        assert SERVER_CODE_TEMPLATE.format(model="SRV", seq=7) == "SRV07"


class TestSheet:
    def test_issues_counted_by_code(self) -> None:
        targets = _targets([("2A01", 31, "GE1"), ("2A01", 27, "GE1")])
        sheet = build_sheet(targets)
        assert sheet.count == 2
        assert sheet.counts_by_code() == {"NO_DEVICE_CODE": 2}
        assert "NO_DEVICE_CODE 2" in sheet.describe()

    def test_clean_sheet_describe(self) -> None:
        targets = _targets([("2A01", 31, "GE1")])
        sheet = build_sheet(number_missing(targets))
        assert sheet.issues == ()
        assert sheet.describe() == "1 条标签，未发现问题"
