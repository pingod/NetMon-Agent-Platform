"""`domain/ports/clipboard.py` 的口径用例 —— Excel 粘贴。

## 这个模块守护什么

它是「Excel 化交互层」里唯一有真实逻辑的部分。剪贴板格式是一堆历史包袱的叠加：

- Windows 用 `\r\n`，老 Mac 用 `\r`，浏览器有时两样都给
- Excel 复制时最后一行总多带一个换行
- 单元格内换行会被双引号包住
- 用户框选歪了，各行列数不一致

每一条处理错了，用户的感受都是「我明明粘的是 5 行，程序说 7 行」或者
「有一列数据不见了」，而他不会知道该改什么。

## 五条不可协商

1. **末尾多一个换行不产生空行**
2. **引号内的换行不切行**（Alt+Enter 的场景）
3. **短行补空、长行报 `extraColumns`**（不静默丢数据）
4. **空块不产生任何行**（粘空白 = 什么都不做）
5. **类型转换走 `coerce_cell`**（粘进来的 `3` 与手打的 `3` 必须是同一个类型）
"""

from __future__ import annotations

import pytest

from idcbox.domain.ports.clipboard import (
    CellRange,
    FillPlan,
    PasteBlock,
    apply_fill,
    apply_paste,
    column_names_ok,
    format_for_clipboard,
    looks_like_formula,
    normalize_range,
    parse_clipboard,
    plan_fill,
    plan_paste,
)
from idcbox.domain.ports.intentcheck import coerce_cell


class TestParse:
    def test_simple_tsv(self) -> None:
        block = parse_clipboard("a\tb\tc\n1\t2\t3")
        assert block.rows == (("a", "b", "c"), ("1", "2", "3"))
        assert block.width == 3
        assert not block.ragged

    def test_crlf_and_lone_cr(self) -> None:
        """Windows 剪贴板是 \\r\\n；不归一化会让每行末尾多一个 \\r，
        存进意图后设备名尾部带控制字符，生成时匹配不上。"""
        block = parse_clipboard("a\tb\r\nc\td\r\n")
        assert block.rows == (("a", "b"), ("c", "d"))
        assert block.rows[0][1] == "b"  # 没有 \r

        block2 = parse_clipboard("a\rb")
        assert block2.rows == (("a",), ("b",))

    def test_trailing_newline_does_not_add_row(self) -> None:
        """Excel 复制时最后一行总带换行。多出来的空行会让用户
        以为粘了 3 行，实际数据只有 2 行。"""
        block = parse_clipboard("a\nb\n")
        assert block.height == 2

    def test_multiple_trailing_newlines(self) -> None:
        block = parse_clipboard("a\nb\n\n\n")
        assert block.height == 2

    def test_quoted_newline_stays_in_one_cell(self) -> None:
        """Excel 里 Alt+Enter 换行的单元格，复制出来是 `"第一行\\n第二行"`。"""
        block = parse_clipboard('"第一行\n第二行"\tx')
        assert block.height == 1
        assert block.width == 2
        assert block.rows[0][0] == "第一行\n第二行"

    def test_quoted_tab_stays_in_one_cell(self) -> None:
        block = parse_clipboard('"a\tb"\tc')
        assert block.rows == (("a\tb", "c"),)

    def test_escaped_quote_becomes_single(self) -> None:
        """Excel 里值含双引号时写成两个。"""
        block = parse_clipboard('"他说""好"""\tx')
        assert block.rows[0][0] == '他说"好"'

    def test_ragged_rows_flagged(self) -> None:
        block = parse_clipboard("a\tb\tc\nd\te")
        assert block.ragged
        assert block.width == 3
        assert block.rows[1] == ("d", "e")

    def test_empty_input(self) -> None:
        assert parse_clipboard("").is_empty
        assert parse_clipboard("").height == 0

    def test_whitespace_only_is_empty(self) -> None:
        assert parse_clipboard("  \n\t\n  ").is_empty

    def test_single_cell(self) -> None:
        block = parse_clipboard("只有一个值")
        assert block.rows == (("只有一个值",),)

    def test_preserves_internal_spaces(self) -> None:
        """值里的空格不能动：`业务接入交换机 1` 的型号名带空格是原始数据的一部分。"""
        block = parse_clipboard("业务接入交换机 1\t2")
        assert block.rows[0][0] == "业务接入交换机 1"


class TestPlan:
    def test_maps_columns_by_position(self) -> None:
        block = parse_clipboard("接入-1\t业务接入交换机1\t2\t26\t40")
        plan = plan_paste(block, ["instance", "model", "channel", "rack", "u"])
        assert plan.count == 1
        assert plan.values[0] == {
            "instance": "接入-1",
            "model": "业务接入交换机1",
            "channel": "2",
            "rack": "26",
            "u": "40",
        }

    def test_short_row_padded_with_empty(self) -> None:
        """短行补空而不是补 0 —— 空的语义是「这格没填」，会被校验报缺字段；
        补 0 会被当成合法值悄悄过掉。"""
        block = parse_clipboard("接入-1\t业务接入交换机1")
        plan = plan_paste(block, ["instance", "model", "channel"])
        assert plan.values[0]["channel"] == ""

    def test_extra_columns_reported(self) -> None:
        """多出来的列不能被静默吃掉 —— 用户可能选错了一片区域。"""
        block = parse_clipboard("a\tb\tc\td")
        plan = plan_paste(block, ["instance", "model"])
        assert plan.extra_columns == 2

    def test_start_row_and_end_row(self) -> None:
        block = parse_clipboard("a\nb\nc")
        plan = plan_paste(block, ["instance"], start_row=10)
        assert plan.start_row == 10
        assert plan.end_row == 12

    def test_unknown_mode_rejected(self) -> None:
        block = parse_clipboard("a")
        with pytest.raises(ValueError):
            plan_paste(block, ["instance"], mode="替换")

    def test_empty_column_names_rejected(self) -> None:
        assert not column_names_ok([])
        assert not column_names_ok(["a", ""])
        assert not column_names_ok(["a", "  "])
        assert column_names_ok(["a", "b"])


class TestApply:
    def _doc(self, n: int = 2) -> dict:
        return {"switches": [{"instance": f"旧-{i}"} for i in range(n)]}

    def test_overwrite_at_start(self) -> None:
        doc = self._doc(3)
        plan = plan_paste(parse_clipboard("新-1\tM\t2\t26\t40"), ["instance", "model", "channel", "rack", "u"], start_row=1)
        written, notes = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 1
        assert notes == []
        assert doc["switches"][1]["instance"] == "新-1"
        assert doc["switches"][1]["channel"] == 2  # 真的转成了 int

    def test_append_when_start_row_none(self) -> None:
        doc = self._doc(2)
        plan = plan_paste(parse_clipboard("新-1\tM"), ["instance", "model"])
        written, _ = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 1
        assert len(doc["switches"]) == 3
        assert doc["switches"][-1]["instance"] == "新-1"

    def test_overwrite_beyond_end_appends(self) -> None:
        """起点超过末尾时补空行再写，等价于追加 —— 不能直接崩或静默丢弃。"""
        doc = self._doc(1)
        plan = plan_paste(parse_clipboard("新-1\tM"), ["instance", "model"], start_row=5)
        written, _ = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 1
        assert doc["switches"][-1]["instance"] == "新-1"

    def test_insert_mode_shifts_rows(self) -> None:
        doc = self._doc(3)
        plan = plan_paste(parse_clipboard("插-1\tM\n插-2\tM"), ["instance", "model"], start_row=1, mode="insert")
        written, _ = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 2
        assert [r.get("instance") for r in doc["switches"]] == ["旧-0", "插-1", "插-2", "旧-1", "旧-2"]

    def test_bad_cell_keeps_others(self) -> None:
        """一格打错不能回滚整块 —— 用户粘 50 行，回滚会让他丢 49 行成果。"""
        doc = self._doc(2)
        plan = plan_paste(
            parse_clipboard("好\t业务接入交换机1\t2\n坏\t业务接入交换机1\t二十五"),
            ["instance", "model", "channel"],
        )
        written, notes = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 2
        assert len(notes) == 1
        assert "需要整数" in notes[0]
        assert doc["switches"][2]["instance"] == "好"
        assert doc["switches"][2]["channel"] == 2
        # 出错的那格**不写入**，而不是被悄悄写成 0。
        # 新建的行本来就没有原值，所以「留原值」在这里表现为「该字段不存在」，
        # 校验器会把它报成「缺少字段」—— 这正是我们要的效果：
        # 宁可报缺字段让人看见，也不要写一个 0 让它悄悄过掉。
        assert doc["switches"][3]["instance"] == "坏"
        assert "channel" not in doc["switches"][3]

    def test_empty_cell_clears_value(self) -> None:
        """粘进来的空格 = 用户想清空这格，不是「保持原值」。"""
        doc = {"switches": [{"instance": "x", "model": "旧型号"}]}
        plan = plan_paste(parse_clipboard("x\t"), ["instance", "model"], start_row=0)
        apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert doc["switches"][0]["model"] == ""

    def test_missing_section_creates_it(self) -> None:
        doc: dict = {}
        plan = plan_paste(parse_clipboard("a"), ["instance"])
        written, _ = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 1
        assert doc["switches"] == [{"instance": "a"}]

    def test_ragged_note_generated_by_caller(self) -> None:
        """`apply_paste` 不管「各行列数不一致」的提示 —— 那是 API 层的措辞。
        这里只确认它能处理 ragged 的块而不崩。"""
        doc = self._doc(1)
        plan = plan_paste(parse_clipboard("a\tb\tc\nd"), ["instance", "model"])
        written, _ = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 2


class TestFormula:
    def test_formula_detected(self) -> None:
        """从 Excel 粘 `=A1+B1` 时要能识别出来提示。

        意图表不该有公式（坐标是人填的），静默算出一个值比报错更危险 ——
        用户以为占了随动，实际存的是他粘那一刻的数值。
        """
        assert looks_like_formula("=A1+B1")
        assert looks_like_formula("=SUM(A:A)")

    def test_plain_text_not_formula(self) -> None:
        assert not looks_like_formula("业务接入交换机1")
        assert not looks_like_formula("=")  # 单个等号不是公式


class TestRealWorldShape:
    """用真实数据的形状跑一遍，确认解析器在 949 行意图的列结构下没跑偏。"""

    def test_switch_block_from_excel(self) -> None:
        """模拟用户从 Excel 复制 3 台交换机（6 列）。"""
        text = (
            "计算接入交换机(业务)-1\t业务接入交换机1\t2\t2\t40\t1\r\n"
            "计算接入交换机(业务)-2\t业务接入交换机1\t2\t4\t40\t1\r\n"
            "计算接入交换机(业务)-3\t业务接入交换机1\t2\t7\t40\t1\r\n"
        )
        block = parse_clipboard(text)
        assert block.height == 3
        assert block.width == 6
        plan = plan_paste(
            block, ["instance", "model", "channel", "rack", "u", "uSpan"]
        )
        doc: dict = {"switches": []}
        written, notes = apply_paste(doc, "switches", plan, coerce=coerce_cell)
        assert written == 3 and notes == []
        assert doc["switches"][0] == {
            "instance": "计算接入交换机(业务)-1",
            "model": "业务接入交换机1",
            "channel": 2,
            "rack": 2,
            "u": 40,
            "uSpan": 1,
        }
        assert [r["rack"] for r in doc["switches"]] == [2, 4, 7]

    def test_route_block_with_full_width_parens(self) -> None:
        """路由段第一列是型号，含全角括号 `计算节点A（ARM）`。"""
        block = parse_clipboard("计算节点A（ARM）\t万兆1\t业务接入交换机1")
        plan = plan_paste(block, ["nodeModel", "nic", "switchModel"])
        assert plan.values[0]["nodeModel"] == "计算节点A（ARM）"

    def test_chain_instances_semicolon_list(self) -> None:
        """交换机组第 4 列是「分号分隔的实例名」，粘贴时是一整个单元格。"""
        block = parse_clipboard(
            "2\t业务接入交换机1\t万兆1\t计算接入交换机(业务)-1; 计算接入交换机(业务)-3\t44"
        )
        assert block.width == 5
        plan = plan_paste(
            block, ["channel", "switchModel", "nic", "instances", "capacity"]
        )
        assert "计算接入交换机(业务)-1" in plan.values[0]["instances"]


class TestCellRange:
    """区域坐标与规整 —— 复制出、填充柄共用的基础类型。"""

    def test_rejects_reversed_coords(self) -> None:
        """反着的坐标必须报错，不能静默当成空区域。

        静默接受的话，前端拖拽方向一变就「复制出空文本」，
        而用户看到的是「复制成功了但粘出来是空的」—— 无法自我诊断。
        """
        with pytest.raises(ValueError, match="区域坐标反了"):
            CellRange(5, 2, 0, 1)

    def test_rejects_negative(self) -> None:
        with pytest.raises(ValueError, match="不能为负"):
            CellRange(0, 1, -1, 2)

    def test_normalize_swaps_drag_direction(self) -> None:
        """鼠标从右下往左上拖也必须得到同一个区域。

        前端传来的坐标取决于用户拖动方向，不做规整就会随机报
        「区域坐标反了」—— 同一个操作有时成功有时失败。
        """
        r = normalize_range(5, 2, 4, 1)
        assert (r.row_start, r.row_end, r.col_start, r.col_end) == (2, 5, 1, 4)
        assert r.height == 4
        assert r.width == 4

    def test_contains_and_describe(self) -> None:
        r = CellRange(2, 4, 1, 3)
        assert r.contains(3, 2)
        assert not r.contains(5, 2)
        assert "第 3~5 行" in r.describe()
        assert CellRange(7, 7, 2, 2).describe() == "第 8 行 1 格"


class TestFormatForClipboard:
    """复制出：序列化成 TSV。"""

    def _rows(self) -> list[dict[str, object]]:
        return [
            {"instance": "SW-1", "model": "业务接入交换机1", "u": 40},
            {"instance": "SW-2", "model": "业务接入交换机1", "u": 37},
        ]

    def test_tab_separated_crlf_joined(self) -> None:
        """行间必须用 `\r\n`。

        实测：只给 `\n` 时 Excel/WPS 的 Windows 版会把整块数据粘进
        **一个**单元格。离线内网里客户用什么系统我们控制不了。
        """
        text = format_for_clipboard(self._rows(), ["instance", "model", "u"], CellRange(0, 1, 0, 2))
        assert "\r\n" in text
        assert text.split("\r\n")[0] == "SW-1\t业务接入交换机1\t40"

    def test_roundtrip_is_identity(self) -> None:
        """复制出去再解析回来必须完全相等。

        这是「复制出」与「粘贴入」对称性的唯一硬保证。
        不测往返的话，转义规则两边各写一遍，迟早出现
        「复制出去粘回来丢了一列」。
        """
        rows = [
            {"a": "x", "b": 'has "quote"', "c": "has\ttab"},
            {"a": "中", "b": "", "c": "多行?\n不换行"},
        ]
        text = format_for_clipboard(rows, ["a", "b", "c"], CellRange(0, 1, 0, 2))
        back = parse_clipboard(text)
        assert back.rows[0] == ("x", 'has "quote"', "has\ttab")
        assert back.rows[1][0] == "中"

    def test_missing_values_become_empty(self) -> None:
        """`None` 序列化成空串，不是字符串 "None"。

        写 `str(None)` 会让粘回来的是文本 `None`，续而校验报
        「u 需要整数，收到「None」」—— 用户完全不知道哪里来的 None。
        """
        text = format_for_clipboard([{"a": None}], ["a"], CellRange(0, 0, 0, 0))
        assert text == ""

    def test_partial_range(self) -> None:
        """只导出区域的子矩形，不整行整列。"""
        rows = [{"a": "1", "b": "2", "c": "3"}, {"a": "4", "b": "5", "c": "6"}]
        text = format_for_clipboard(rows, ["a", "b", "c"], CellRange(1, 1, 1, 2))
        assert text == "5\t6"


class TestFillPatterns:
    """填充柄的三种确定性规律。"""

    def test_numeric_step_down(self) -> None:
        """U 位往下挪：38,37 → 36,35,34。这是填充柄的主场景。"""
        plan = plan_fill([{"u": 38}, {"u": 37}], ["u"], CellRange(0, 1, 0, 0), 4)
        assert [v["u"] for v in plan.values] == [36, 35, 34]
        assert "递增 -1" in plan.pattern

    def test_numeric_step_up(self) -> None:
        """`target_rows` 是 **0-based 的终点行号（含）**，不是「要写几行」。

        与 API 的约定一致：前端拖到界面第 N 行（1-based），传 `N - 1`。
        写满到那一行为止 —— 用户拖到哪就该填到哪。
        """
        plan = plan_fill([{"u": 3}, {"u": 5}], ["u"], CellRange(0, 1, 0, 0), 4)
        assert [v["u"] for v in plan.values] == [7, 9, 11]

    def test_single_value_copies(self) -> None:
        """单值照抄 —— 不猜成 +1。

        Excel 拖单值时是复制而不是递增（除非按住 Ctrl）。跟着 Excel 走，
        因为用户的肌肉记忆就在那儿。
        """
        plan = plan_fill([{"u": 40}], ["u"], CellRange(0, 0, 0, 0), 3)
        assert [v["u"] for v in plan.values] == [40, 40, 40]
        assert plan.pattern == "照抄"

    def test_text_sequence_cycles(self) -> None:
        """内置序列循环：周一,周二 → 周三,周四。"""
        plan = plan_fill([{"nic": "周一"}, {"nic": "周二"}], ["nic"], CellRange(0, 1, 0, 0), 4)
        assert [v["nic"] for v in plan.values] == ["周三", "周四", "周五"]

    def test_unknown_text_copies(self) -> None:
        """认不出的文本老实照抄，不做 abc→abd 这种猜测。

        Excel 的智能填充猜错时用户往往不会发现（值看着都合理）。
        意图表里猜错一个机柜号，生成出来就是接错一柜的设备。
        """
        plan = plan_fill([{"model": "业务接入交换机1"}], ["model"], CellRange(0, 0, 0, 0), 2)
        assert [v["model"] for v in plan.values] == ["业务接入交换机1", "业务接入交换机1"]
        assert plan.pattern == "照抄"

    def test_non_uniform_numbers_copy(self) -> None:
        """步长不一致时不猜等差数列。

        `38, 36` 是等差的（-2），会算成 `34, 32`；真正不齐的是 `38, 36, 33`。
        这种多半是「几台设备分散摆在不同 U 位」，往下延伸没有唯一答案，
        老实照抄比猜一个好看的数列安全。
        """
        plan = plan_fill(
            [{"u": 38}, {"u": 36}, {"u": 33}], ["u"], CellRange(0, 2, 0, 0), 5
        )
        assert plan.pattern == "照抄"

    def test_two_values_always_form_a_step(self) -> None:
        """两个数总能构成等差 —— 这是刻意的，不是漏判。

        只有两个值时无法判断「是不是真等差」，按 Excel 的行为直接延伸。
        若要求三个值才延伸，用户拖两格就发现「没动静」，会觉得功能坏了。
        """
        plan = plan_fill([{"u": 38}, {"u": 36}], ["u"], CellRange(0, 1, 0, 0), 3)
        assert plan.pattern == "递增 -2"
        assert [v["u"] for v in plan.values] == [34, 32]

    def test_rejects_multi_row_multi_col_source(self) -> None:
        """多行多列源区域看不出该按哪种规律延伸，必须拒绝。"""
        with pytest.raises(ValueError, match="只看得出单行或单列|只支持单行或单列"):
            plan_fill(
                [{"u": 1, "rack": 2}, {"u": 3, "rack": 4}],
                ["u", "rack"],
                CellRange(0, 1, 0, 1),
                5,
            )

    def test_rejects_backward_target(self) -> None:
        with pytest.raises(ValueError, match="之后"):
            plan_fill([{"u": 40}], ["u"], CellRange(2, 2, 0, 0), 2)


class TestApplyFill:
    """填充落盘。"""

    def _coerce(self, section: str, column: str, raw: object):
        return coerce_cell(section, column, raw)

    def test_does_not_overwrite_source(self) -> None:
        """源区域一行都不能被覆盖。

        ## 这条用例抓过一个真 bug

        第一版 `apply_fill` 从 `target.row_start + 1` 开始写。源区是
        `row_start..row_start+1`（2 行）时侥幸对，源区 3 行时就会把
        源区第 2 行覆盖掉。用户拖填充柄时源区一般就是 1~2 行，
        所以 3 行才会暴露 —— 而这恰好是「连续三个相近值」的常见情形。
        """
        doc: dict[str, object] = {"switches": [{"u": 40}, {"u": 38}, {"u": 36}]}
        plan = plan_fill(
            [{"u": 40}, {"u": 38}, {"u": 36}], ["u"], CellRange(0, 2, 0, 0), 5
        )
        apply_fill(doc, "switches", plan, coerce=self._coerce)
        rows = doc["switches"]
        assert [r["u"] for r in rows] == [40, 38, 36, 34, 32, 30]

    def test_extends_rows_when_needed(self) -> None:
        """目标超过现有行数时自动补行。"""
        doc: dict[str, object] = {"switches": [{"u": 10}]}
        plan = plan_fill([{"u": 10}], ["u"], CellRange(0, 0, 0, 0), 3)
        written, notes = apply_fill(doc, "switches", plan, coerce=self._coerce)
        assert written == 3
        assert len(doc["switches"]) == 4
        assert notes == []

    def test_coerce_errors_are_collected_not_fatal(self) -> None:
        """类型转换失败只记提示，其余行照写。

        一格错就整块回滚，会让用户丢了另外几十格的成果。
        """
        doc: dict[str, object] = {"switches": [{"u": 1, "instance": "X"}]}
        plan = FillPlan(
            target=CellRange(0, 2, 0, 1),
            source_row_end=0,
            values=({"u": "abc", "instance": "Y"}, {"u": 5, "instance": "Z"}),
            pattern="照抄",
        )
        written, notes = apply_fill(doc, "switches", plan, coerce=self._coerce)
        # 两行都算「处理过」：`written` 是处理的行数，不是成功格的数目。
        # 第一行 `u` 失败但 `instance` 成功了，该行不能算整个丢。
        assert written == 2
        assert any("需要整数" in n for n in notes)
        assert doc["switches"][1]["instance"] == "Y", "失败行里成功的列仍要写入"
        assert doc["switches"][2]["u"] == 5, "后面成功的行不受影响"
