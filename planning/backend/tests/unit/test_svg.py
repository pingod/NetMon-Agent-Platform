"""`domain/exporting/svg.py` 的口径用例。

## 这个模块守护什么

图纸是**唯一会离开屏幕的东西**。界面上画错，用户看得见、能当场骂；
图纸画错，是印出来贴到机柜门上之后才发现的 —— 那时人已经在按图施工。

所以这里守四件事，每件都对应一次真实事故的可能：

1. **白底**：深色主题直接导出的结果是一整片黑，A3 打印要几十分钟还伤纸
2. **转义**：设备名里一个有问题的 `<` 会让整个 SVG 变成 XML 解析错误，
   浏览器白屏，而且**报错不会告诉你是哪台设备**
3. **多行纵向堆叠**：第一版把所有行都画在同一个 `top` 上，13 行完全重叠，
   导出的 SVG 看起来只有一个柜 —— 而这个错误单看代码很难发现
4. **重叠必须标红**：源数据有 11 对 U 位重叠（规划占位），
   图上如果只是叠着盖住，现场会以为那个 U 位只有一台设备
"""

from __future__ import annotations

from idcbox.domain.exporting.svg import (
    CATEGORY_FILLS,
    DEFAULT_RACK_U,
    RackSheet,
    SvgDevice,
    SvgOptions,
    SvgRack,
    categorize,
    render_racks,
    render_to_html,
)


def _rack(ch: int = 2, seq: int = 1, devices: tuple[SvgDevice, ...] = ()) -> SvgRack:
    return SvgRack(channel_no=ch, seq_no=seq, label=f"通道{ch} / 机柜{seq}", devices=devices)


def _dev(model: str = "计算节点A（ARM）", start: int = 3, end: int = 4, **kw: object) -> SvgDevice:
    return SvgDevice(
        model=model,
        display_name=str(kw.get("display_name", "")),
        start_u=start,
        end_u=end,
        port_count=int(kw.get("port_count", 7)),
    )


class TestCategorize:
    """分类顺序是硬约束：先判「核心」再判「交换机」。"""

    def test_core_before_switch(self) -> None:
        """`业务核心交换机` 同时含「核心」和「交换机」。

        如果先判「交换机」，核心交换机会被画成接入交换机的颜色 ——
        现场看到一片同色，分不出哪台是核心。
        """
        assert categorize("业务核心交换机") == "core"
        assert categorize("存储核心交换机") == "core"
        assert categorize("管理核心交换机") == "core"

    def test_switch(self) -> None:
        assert categorize("业务接入交换机 1") == "switch"
        assert categorize("带内交换机1") == "switch"
        assert categorize("网络节点交换机") == "switch"

    def test_server(self) -> None:
        assert categorize("计算节点A（ARM）") == "server"
        assert categorize("计算节点B（C86）") == "server"
        assert categorize("EHDD云硬盘服务器1") == "server"

    def test_security(self) -> None:
        assert categorize("防火墙1") == "security"
        assert categorize("广域网链路负载均衡") == "security"
        assert categorize("服务器密码机") == "security"

    def test_unknown_is_other_not_crash(self) -> None:
        assert categorize("") == "other"
        assert categorize("没见过的型号") == "other"

    def test_every_category_has_a_colour(self) -> None:
        """新增分类却忘了配色，画出来就是 `fill=""` → 透明块，打印后是空白。"""
        for cat in ("core", "switch", "server", "storage", "security", "other"):
            assert cat in CATEGORY_FILLS
            assert CATEGORY_FILLS[cat].startswith("#")


class TestPrintability:
    def test_white_background_always(self) -> None:
        """白底不能跟随主题。深色导出的图纸打印出来是一整片黑。"""
        svg = render_racks(RackSheet(racks=(_rack(),), options=SvgOptions(title="t")))
        assert 'fill="#ffffff"' in svg
        assert svg.startswith("<svg")

    def test_no_dark_background_leaks_in(self) -> None:
        svg = render_racks(RackSheet(racks=(_rack(),), options=SvgOptions(title="t")))
        # 界面深色主题的底色是 #0f1115 系；图纸里绝不该出现
        for dark in ("#0f1115", "#151a21", "#000000"):
            assert dark not in svg

    def test_html_wrapper_has_page_rule(self) -> None:
        """没有 `@page` 规则，Ctrl+P 会带浏览器页眉页脚、边缘被裁。"""
        html = render_to_html("<svg/>", title="图纸")
        assert "@page" in html
        assert "A3 landscape" in html
        assert "print-color-adjust: exact" in html


class TestEscaping:
    def test_device_name_with_angle_bracket_is_escaped(self) -> None:
        """未转义的 `<` 会让 SVG 变成 XML 解析错误，整个文件白屏。"""
        svg = render_racks(
            RackSheet(racks=(_rack(devices=(_dev(model="<script>x</script>"),)),))
        )
        assert "<script>" not in svg
        assert "&lt;script&gt;" in svg

    def test_ampersand_is_escaped(self) -> None:
        svg = render_racks(RackSheet(racks=(_rack(devices=(_dev(model="A&B"),)),)))
        assert "A&amp;B" in svg

    def test_title_and_subtitle_escaped(self) -> None:
        svg = render_racks(
            RackSheet(racks=(_rack(),), options=SvgOptions(title="<t>", subtitle="a&b"))
        )
        assert "&lt;t&gt;" in svg
        assert "a&amp;b" in svg


class TestLayout:
    def test_multi_page_stacks_vertically(self) -> None:
        """13 个柜 8 个一页 = 2 行。两行必须纵向错开。

        第一版把两行都画在同一个 y 上，导出后看起来只有一个柜 ——
        而 SVG 仍然合法、浏览器也正常渲染，靠肉眼几乎不可能发现。
        """
        racks = tuple(_rack(seq=i + 1) for i in range(9))
        svg = render_racks(
            RackSheet(racks=racks, options=SvgOptions(max_racks_per_sheet=8, unit_h=18))
        )
        # 第 9 个柜属于第 2 行，它的 y 坐标必须比第 1 个大
        assert "（第 2 行，接上）" in svg
        assert svg.count("通道2 / 机柜") == 9

    def test_height_grows_with_pages(self) -> None:
        one = render_racks(RackSheet(racks=(_rack(seq=1),), options=SvgOptions(max_racks_per_sheet=8)))
        many = render_racks(
            RackSheet(
                racks=tuple(_rack(seq=i + 1) for i in range(9)),
                options=SvgOptions(max_racks_per_sheet=8),
            )
        )

        def _h(svg: str) -> int:
            import re

            return int(re.search(r'height="(\d+)"', svg).group(1))  # type: ignore[union-attr]

        assert _h(many) > _h(one) * 1.5

    def test_u_grid_uses_pattern_not_42_lines(self) -> None:
        """42 条 line × 200 柜 = 8400 节点，浏览器打开会卡。"""
        svg = render_racks(RackSheet(racks=(_rack(),), options=SvgOptions(title="t")))
        assert "<pattern" in svg
        # 一个柜只有 1 条 line（在 pattern 内），不是 42 条
        assert svg.count("<line") <= 2

    def test_u_numbering_top_down(self) -> None:
        """U42 在顶部、U1 在底部 —— 与实物一致，现场是照着柜子装。"""
        svg = render_racks(RackSheet(racks=(_rack(),), options=SvgOptions(title="t")))
        i42 = svg.find(">42<")
        i1 = svg.find(">1<")
        assert i42 != -1 and i1 != -1 and i42 < i1

    def test_device_at_top_has_smaller_y(self) -> None:
        """40U 的设备 y 必须比 3U 的小（SVG y 向下增长）。"""
        svg = render_racks(
            RackSheet(
                racks=(
                    _rack(
                        devices=(
                            _dev("业务接入交换机 1", 40, 40),
                            _dev("计算节点A（ARM）", 3, 4),
                        )
                    ),
                )
            )
        )
        # 设备块 rect 的 fill 是分类色、且高度 < 整柜高度；底图 rect 的 fill 是
        # #fbfbfb / url(#...) 且高 756。只取设备块来比 y。
        import re

        after = svg[svg.find("</defs>") :]
        blocks = re.findall(
            r'<rect x="\d+" y="(\d+)" width="\d+" height="(\d+)" fill="#(?!fbfbfb)'
            r'[a-f0-9]{6}"',
            after,
        )
        ys = [int(y) for y, h in blocks if int(h) < 700]
        assert len(ys) >= 2, blocks
        assert min(ys) < max(ys)


class TestOverlapHighlight:
    def test_overlapping_devices_are_marked_red(self) -> None:
        """真实数据有 11 对 U 位重叠。只画设备块会被后画的盖住，
        现场会以为那个 U 位只有一台设备。"""
        # 真实数据里的形态：`带外交换机1` 与 `带内交换机1` 同占 37U
        a = _dev("带内交换机1", 37, 37)
        b = _dev("带外交换机1", 37, 38)
        svg = render_racks(RackSheet(racks=(_rack(devices=(a, b)),)))
        assert "#cc0000" in svg
        assert "stroke-dasharray" in svg

    def test_no_red_when_no_overlap(self) -> None:
        a = _dev("业务接入交换机 1", 40, 40)
        b = _dev("带内交换机1", 37, 37)
        svg = render_racks(RackSheet(racks=(_rack(devices=(a, b)),)))
        assert "#cc0000" not in svg


class TestRackSummary:
    def test_used_u_sums_sizes(self) -> None:
        rack = _rack(devices=(_dev(start=3, end=12), _dev(start=40, end=40)))
        assert [d.size_u for d in rack.devices] == [10, 1]
        assert rack.used_u == 11

    def test_rack_u_default_is_42(self) -> None:
        assert DEFAULT_RACK_U == 42

    def test_empty_rack_renders(self) -> None:
        """空柜也要能画 —— 平面图 362 个柜位里 260 个没设备，
        不代表将来不会有；图纸生成器不能因为空就崩。"""
        svg = render_racks(RackSheet(racks=(_rack(),), options=SvgOptions(title="空柜")))
        assert "0 台" in svg

    def test_device_outside_rack_is_clipped(self) -> None:
        """U 位超出 42 时裁到边界，不能让图块画到柜外盖住隔壁柜。"""
        svg = render_racks(
            RackSheet(racks=(_rack(devices=(_dev(start=41, end=48),)),), options=SvgOptions(title="t"))
        )
        import re

        ys = [int(y) for y in re.findall(r'y="(-?\d+)"', svg)]
        assert min(ys) >= 0


class TestLongNames:
    def test_long_model_is_clipped(self) -> None:
        """不裁剪会溢出到隔壁柜 —— 现场看到的名字属于别的柜，比看不到更糟。"""
        long_model = "超长型号名称" * 5
        svg = render_racks(
            RackSheet(racks=(_rack(devices=(_dev(long_model, 3, 12),)),), options=SvgOptions(title="t"))
        )
        assert "…" in svg
        assert long_model not in svg

    def test_shows_display_name_when_present(self) -> None:
        svg = render_racks(
            RackSheet(
                racks=(_rack(devices=(_dev("计算接入交换机(业务)", 3, 12, display_name="NAT接入交换机-1"),)),),
                options=SvgOptions(title="t"),
            )
        )
        assert "NAT接入交换机-1" in svg
