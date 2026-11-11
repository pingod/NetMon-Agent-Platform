"""导入归一的边界：哪些是格式噪声（归一到同值），哪些是业务语义（必须保留原值）。

## 这条分界线的产品意义

分错了会有两个方向的灾难：

- **把格式噪声当业务值**：`业务接入交换机 1` 与 `业务接入交换机1` 被当成两台设备
  → 实例数虚高 2 个、U 位冲突从 11 对变 13 对、耗材多算、体检报告出现假告警。
- **把业务语义当噪声**：把 `待定` 归一成 `None`、把 `一` 归一成 `0`
  → 导出时把人的判断改掉，"原值保真"这条铁线当场破。

所以本模块的用例都是**成对出现**的：一个断言"归一到同值"，一个断言"保留原值"。
"""

from __future__ import annotations

import pytest

from idcbox.domain.importing.normalize import (
    LENGTH_SENTINELS,
    MODULE_SENTINELS,
    is_length_sentinel,
    make_stable_key,
    norm_cable_type,
    norm_int,
    norm_name,
    norm_text,
    norm_u_position,
    parse_length,
    u_position_span,
)


class TestNormName:
    """名称归一：去所有空白（含全角），这是**格式噪声**。"""

    @pytest.mark.parametrize(
        ("raw", "expected"),
        [
            ("业务接入交换机 1", "业务接入交换机1"),
            ("业务接入交换机  1", "业务接入交换机1"),   # 双空格
            ("业务接入交换机\u30001", "业务接入交换机1"),  # 全角空格
            (" 业务接入交换机1 ", "业务接入交换机1"),      # 首尾
            ("业务接入交换机\t1", "业务接入交换机1"),      # 制表符
        ],
    )
    def test_strips_all_whitespace(self, raw, expected):
        assert norm_name(raw) == expected

    def test_real_impact(self):
        """实测来源：4 个带空格的型号名造成 836 vs 834 的差。"""
        pairs = [
            ("业务接入交换机 1", "业务接入交换机1"),
            ("业务接入交换机 2", "业务接入交换机2"),
            ("存储接入交换机 1", "存储接入交换机1"),
            ("带外交换机 1", "带外交换机1"),
        ]
        for a, b in pairs:
            assert norm_name(a) == norm_name(b)

    def test_none_and_empty(self):
        assert norm_name(None) == ""
        assert norm_name("") == ""
        assert norm_name("   ") == ""

    def test_does_not_change_inner_chars(self):
        """只去空白，不动其他字符 —— 中文括号、连字符都必须保留。"""
        assert norm_name("计算节点A（ARM）") == "计算节点A（ARM）"
        assert norm_name("4APU-13A-06-35U-control-guanli-1") == (
            "4APU-13A-06-35U-control-guanli-1"
        )


class TestNormText:
    """文本归一：只去首尾，保留内部（用于备注这类自由文本）。"""

    def test_strip_only(self):
        assert norm_text("  备注 有 空格  ") == "备注 有 空格"

    def test_inner_preserved(self):
        assert norm_text("首期") == "首期"


class TestNormInt:
    @pytest.mark.parametrize(
        ("raw", "expected"),
        [
            (1, 1),
            (1.0, 1),
            ("1", 1),
            ("1.0", 1),
            ("rack1", 1),
            ("RACK1", 1),
            ("2号柜", 2),
            (" 38 ", 38),
        ],
    )
    def test_variants(self, raw, expected):
        assert norm_int(raw) == expected

    def test_invalid(self):
        assert norm_int(None) is None
        assert norm_int("") is None
        assert norm_int("abc") is None
        assert norm_int("待定") is None


class TestNormCableType:
    """线缆类型归一：中文/别名/拼接写法都要收敛到三个标准值之一。"""

    @pytest.mark.parametrize(
        ("raw", "expected"),
        [
            ("MPO", "MPO"),
            ("40GMPO线缆", "MPO"),
            ("MPO光纤", "MPO"),
            ("RJ45", "RJ45"),
            ("网线", "RJ45"),
            ("千兆网线", "RJ45"),
            ("LC-LC", "LC-LC"),
            ("光纤", "LC-LC"),
            ("万兆光纤", "LC-LC"),
            ("lclc", "LC-LC"),
        ],
    )
    def test_aliases(self, raw, expected):
        assert norm_cable_type(raw) == expected

    def test_unknown(self):
        """空 → UNKNOWN；认不出的原文原样返回（由调用方决定是否报错）。

        为什么不把认不出的也返回 UNKNOWN：原文是排查线索。
        用户写`40GMPO线缆`我们能猜，但写`奇怪的东西`时应把原文带给用户看，
        而不是吞掉变成 UNKNOWN。
        """
        assert norm_cable_type(None) == "UNKNOWN"
        assert norm_cable_type("") == "UNKNOWN"
        assert norm_cable_type("奇怪的东西") == "奇怪的东西"

    def test_three_standard_values(self):
        """只允许三个标准值 + UNKNOWN —— 多一个都会让 Q 查表漏。"""
        allowed = {"MPO", "RJ45", "LC-LC", "UNKNOWN"}
        for raw in ("MPO", "网线", "光纤", None, "40GMPO线缆"):
            assert norm_cable_type(raw) in allowed


class TestSentinels:
    """业务 sentinel 必须被识别为 sentinel，但**原值不得丢弃**。"""

    def test_length_sentinels_membership(self):
        for s in ("一", "设备自带", "待定", "-", "—", "/", "无", "N/A", "NA"):
            assert is_length_sentinel(s), f"{s} 应被识别为 sentinel"

    def test_real_numbers_not_sentinel(self):
        for v in (5, 10, "10", 0, "0"):
            assert not is_length_sentinel(v), f"{v} 不是 sentinel"

    def test_parse_length_number(self):
        assert parse_length(10) == (10, None)
        assert parse_length("10") == (10, None)
        assert parse_length(10.0) == (10, None)

    def test_parse_length_sentinel_keeps_raw(self):
        """sentinel → (None, 原文)：数值位为 None 但原文必须带出来。"""
        assert parse_length("待定") == (None, "待定")
        assert parse_length("设备自带") == (None, "设备自带")
        assert parse_length("一") == (None, "一")

    def test_parse_length_sentinel_never_zero(self):
        """绝不可把 sentinel 变成 0 —— 0 会被当成"算出来的长度"。"""
        for s in LENGTH_SENTINELS:
            num, raw = parse_length(s)
            assert num != 0, f"sentinel {s!r} 不能变成 0"
            assert raw is not None, "原文必须保留"


class TestUPosition:
    """U 位 13 种形态的归一与区间解析。"""

    @pytest.mark.parametrize(
        ("raw", "expected"),
        [
            ("40U", "40U"),
            ("40u", "40U"),
            ("40", "40U"),
            ("40 U", "40U"),
            ("5U", "5U"),
            ("5U", "5U"),
        ],
    )
    def test_single_value(self, raw, expected):
        assert norm_u_position(raw) == expected

    @pytest.mark.parametrize(
        ("raw", "expected"),
        [
            ("3-12U", "3-12U"),
            ("3 - 12 U", "3-12U"),
            ("3-4U", "3-4U"),
            ("19-20U", "19-20U"),
            ("3—12U", "3-12U"),  # 中文破折号
        ],
    )
    def test_range(self, raw, expected):
        assert norm_u_position(raw) == expected

    def test_span_single(self):
        assert u_position_span("40U") == (40, 40)
        assert u_position_span("37U") == (37, 37)

    def test_span_range(self):
        """`3-12U` 占 10U（3..12 含两端）—— 实测 306 行这种写法。"""
        start, end = u_position_span("3-12U")
        assert start == 3
        assert end == 12
        assert end - start + 1 == 10

    def test_span_range_small(self):
        assert u_position_span("19-20U") == (19, 20)
        assert u_position_span("3-4U") == (3, 4)

    def test_span_invalid(self):
        """无效输入返回 `(None, None)` 而不是 None —— 保持返回类型稳定，
        调用方可以无条件解包 `a, b = u_position_span(v)`。"""
        assert u_position_span(None) == (None, None)
        assert u_position_span("") == (None, None)
        assert u_position_span("abc") == (None, None)


class TestStableKey:
    """稳定键：导入幂等的基石。同一物理连线重复导入必须得到同一个键。"""

    def test_deterministic(self):
        args = ("devA", "25GE1/0/1", "devB", "25G1", "LC-LC")
        assert make_stable_key(*args) == make_stable_key(*args)

    def test_order_matters(self):
        """A→B 与 B→A 是不同的键（本端/对端有语义，不能当成同一条线）。"""
        k1 = make_stable_key("devA", "p1", "devB", "p2", "LC-LC")
        k2 = make_stable_key("devB", "p2", "devA", "p1", "LC-LC")
        assert k1 != k2

    def test_empty_placeholder(self):
        """空值用 `∅` 占位，避免 `a||b` 与 `a|b` 撞键。"""
        k = make_stable_key("devA", None, "devB", "", "RJ45")
        assert "∅" in k

    def test_no_collision_with_empty(self):
        k1 = make_stable_key("a", None, "b", "c")
        k2 = make_stable_key("a", "", "b", "c")
        assert k1 == k2, "None 与空字符串应等价"

    def test_different_cable_type_different_key(self):
        """同两端点不同线缆类型是两条线（现实中不会，但键必须区分）。"""
        k1 = make_stable_key("a", "p1", "b", "p2", "LC-LC")
        k2 = make_stable_key("a", "p1", "b", "p2", "RJ45")
        assert k1 != k2


class TestModuleSentinels:
    def test_module_sentinels(self):
        for s in ("一", "-", "—", "无"):
            assert s in MODULE_SENTINELS

    def test_real_modules_not_sentinel(self):
        for m in ("10G-SR", "25G-SR", "100G-SR4", "光转电", "设备自带"):
            # 设备自带是 sentinel 但不在 MODULE_SENTINELS 里（它是"无需采购"语义）
            if m != "设备自带":
                assert m not in MODULE_SENTINELS
