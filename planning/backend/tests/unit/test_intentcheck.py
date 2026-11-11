"""意图填写校验的用例 —— 每一条都对应一类「人会怎么填错」。

## 这组用例守护什么

`intentcheck` 不是「格式校验器」，它是**在花 40 秒生成 5127 行之前，
用 40 毫秒告诉人哪一格写错了**。所以用例的写法是对着「真实填错场景」来的：

| 场景 | 真实发生过的例子 |
|---|---|
| 型号漏填 | 原表 `带外交换机-2` 型号栏就是空的 |
| 实例名写重 | 26 台交换机都叫「业务接入交换机1」，必须靠实例名区分 |
| 引用了不存在的实例 | 改实例名后忘了改交换机组 |
| 同一 U 叠两台 | 原表 11 处 U 位冲突里 4 处是真的撞车 |
| 数字列打了字 | 原表 `间隔机柜` 列出现 `25G-SR` |

## 一条不可协商

**严重度不能全定成 error。** 真实意图里有故意「不合规」的地方
（临时不上架的交换机、U 位叠放的规划占位）。全定 error 会逼人删声明，
下次恢复又得重填 —— 用例 `test_overlap_is_warn_not_error` 守这一条。
"""

from __future__ import annotations

from typing import Any

import pytest

from idcbox.domain.ports.intentcheck import (
    IntentIssue,
    append_row,
    apply_cell,
    check_intent,
    coerce_cell,
    delete_row,
    issues_for_row,
)

# ---------------------------------------------------------------------------
# 装置：一份最小可用的意图
# ---------------------------------------------------------------------------


def _minimal() -> dict[str, Any]:
    """1 个布局块 + 2 台交换机 + 1 个交换机组 + 1 条显式连线。"""
    return {
        "schema": 1,
        "project": "测试",
        "site": "测试机房",
        "racks": [{"channel": 2, "from": 1, "to": 2, "model": "计算节点A（ARM）"}],
        "switches": [
            {"instance": "接入-1", "model": "业务接入交换机1", "channel": 2, "rack": 1, "u": 40},
            {"instance": "接入-2", "model": "业务接入交换机1", "channel": 2, "rack": 2, "u": 40},
        ],
        "devices": [],
        "routes": [{"nodeModel": "计算节点A（ARM）", "nic": "万兆1", "switchModel": "业务接入交换机1"}],
        "chains": [
            {
                "channel": 2,
                "switchModel": "业务接入交换机1",
                "nic": "万兆1",
                "instances": ["接入-1", "接入-2"],
                "capacity": 44,
            }
        ],
        "explicitLinks": [{"a": ["接入-1", "100GE1/0/1"], "b": ["接入-2", "100GE1/0/1"]}],
    }


def _codes(result) -> list[str]:
    return [x.code for x in result.issues]


def _errors(result) -> list[IntentIssue]:
    return [x for x in result.issues if x.severity == "error"]


# ---------------------------------------------------------------------------
# 基线：最小意图必须是干净的
# ---------------------------------------------------------------------------


class TestBaseline:
    def test_minimal_intent_has_no_errors(self) -> None:
        """最小可用意图不应报任何 error —— 否则校验器会天天误报，人就关了它。"""
        result = check_intent(_minimal())
        assert result.error_count == 0, [x.message for x in _errors(result)]
        assert result.ok is True

    def test_counts_reflect_severity(self) -> None:
        result = check_intent(_minimal())
        assert result.counts["error"] == 0
        assert result.counts["warn"] + result.counts["info"] == len(result.issues)

    def test_ok_only_depends_on_errors(self) -> None:
        """有 warn 不等于不能生成。

        `接入-3` 故意不进交换机组 —— 它只声明位置上架、暂不接线，
        这样只会产生 `POSITION_TAKEN` 警告，不会连带出 `CHAIN_INSTANCE_UNKNOWN`。
        真实场景里「先占位、后接线」很常见。
        """
        doc = _minimal()
        doc["switches"].append(
            {"instance": "接入-3", "model": "业务接入交换机1", "channel": 2, "rack": 1, "u": 40}
        )  # 与接入-1 同位置 → warn
        result = check_intent(doc)
        assert result.warn_count >= 1
        assert result.error_count == 0
        assert result.ok is True


# ---------------------------------------------------------------------------
# 交换机段
# ---------------------------------------------------------------------------


class TestSwitches:
    def test_missing_instance_is_error(self) -> None:
        doc = _minimal()
        doc["switches"][0]["instance"] = ""
        result = check_intent(doc)
        assert "SW_INSTANCE_EMPTY" in _codes(result)
        assert result.ok is False

    def test_duplicated_instance_is_error(self) -> None:
        """同名会让引用指向不确定的那台 —— 必须是 error。"""
        doc = _minimal()
        doc["switches"][1]["instance"] = "接入-1"
        result = check_intent(doc)
        assert "SW_INSTANCE_DUPLICATED" in _codes(result)
        assert result.ok is False

    def test_model_empty_is_warn_when_not_in_chain(self) -> None:
        """原表 `带外交换机-2` 就缺型号，但它只走显式连线 —— 不该阻断生成。"""
        doc = _minimal()
        doc["switches"].append(
            {"instance": "带外-2", "model": "", "channel": 1, "rack": 23, "u": 34}
        )
        doc["explicitLinks"].append({"a": ["带外-2", "100GE1/0/1"], "b": ["接入-1", "100GE1/0/2"]})
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "SW_MODEL_EMPTY"]
        assert len(issues) == 1
        assert issues[0].severity == "warn"
        assert result.ok is True

    def test_model_empty_is_error_when_in_chain(self) -> None:
        """进交换机组就必须有型号 —— 生成器要靠它查端口容量。"""
        doc = _minimal()
        doc["switches"][0]["model"] = ""
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "SW_MODEL_EMPTY"]
        assert len(issues) == 1
        assert issues[0].severity == "error"
        assert "端口容量" in issues[0].message

    def test_overlap_is_warn_not_error(self) -> None:
        """原表 11 处 U 位重叠里有规划占位；全定 error 会逼人删声明。

        用新增一台「占了别人位置但还没接线」的交换机来造冲突 ——
        不改 `接入-2` 的名字，否则会连带出 `CHAIN_INSTANCE_UNKNOWN`，
        那就测不出「单纯的位置冲突是否只算 warn」了。
        """
        doc = _minimal()
        doc["switches"].append(
            {"instance": "接入-2b", "model": "业务接入交换机1", "channel": 2, "rack": 2, "u": 40}
        )  # 与接入-2 同 rack 同 u
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "POSITION_TAKEN"]
        assert len(issues) == 1
        assert issues[0].severity == "warn"
        assert "接入-2" in issues[0].message
        assert result.ok is True

    def test_u_out_of_range_is_error(self) -> None:
        doc = _minimal()
        doc["switches"][0]["u"] = 43
        result = check_intent(doc)
        assert "U_RANGE" in _codes(result)

    def test_row_no_is_the_segment_index(self) -> None:
        """界面上表格行索引 = rowNo，所以必须是段内 0-based。"""
        doc = _minimal()
        doc["switches"][1]["instance"] = ""
        result = check_intent(doc)
        bad = [x for x in result.issues if x.code == "SW_INSTANCE_EMPTY"]
        assert len(bad) == 1
        assert bad[0].rowNo == 1
        assert issues_for_row(result, "switches", 1) == bad


# ---------------------------------------------------------------------------
# 布局段
# ---------------------------------------------------------------------------


class TestRacks:
    def test_reversed_range_is_error(self) -> None:
        doc = _minimal()
        doc["racks"][0]["from"] = 5
        doc["racks"][0]["to"] = 2
        result = check_intent(doc)
        assert "RACK_RANGE_REVERSED" in _codes(result)
        assert result.ok is False

    def test_single_rack_is_info_only(self) -> None:
        doc = _minimal()
        doc["racks"][0]["from"] = 1
        doc["racks"][0]["to"] = 1
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "RACK_SINGLE"]
        assert len(issues) == 1
        assert issues[0].severity == "info"

    def test_channel_out_of_range_is_error(self) -> None:
        doc = _minimal()
        doc["racks"][0]["channel"] = 15
        result = check_intent(doc)
        assert "RACK_CHANNEL_RANGE" in _codes(result)

    def test_empty_model_is_error(self) -> None:
        doc = _minimal()
        doc["racks"][0]["model"] = ""
        result = check_intent(doc)
        assert "RACK_MODEL_EMPTY" in _codes(result)


# ---------------------------------------------------------------------------
# 路由段
# ---------------------------------------------------------------------------


class TestRoutes:
    def test_missing_field_is_error(self) -> None:
        doc = _minimal()
        doc["routes"][0]["nic"] = ""
        result = check_intent(doc)
        assert "ROUTE_FIELD_EMPTY" in _codes(result)

    def test_duplicated_nic_is_warn(self) -> None:
        """同一网口声明两种交换机只会有一个生效 —— 但可能是有意的过渡期配置。"""
        doc = _minimal()
        doc["routes"].append(
            {"nodeModel": "计算节点A（ARM）", "nic": "万兆1", "switchModel": "存储接入交换机1"}
        )
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "ROUTE_DUPLICATED"]
        assert len(issues) == 1
        assert issues[0].severity == "warn"
        assert result.ok is True


# ---------------------------------------------------------------------------
# 交换机组段
# ---------------------------------------------------------------------------


class TestChains:
    def test_empty_instances_is_error(self) -> None:
        doc = _minimal()
        doc["chains"][0]["instances"] = []
        result = check_intent(doc)
        assert "CHAIN_EMPTY" in _codes(result)
        assert result.ok is False

    def test_unknown_instance_is_error(self) -> None:
        """报的是「找不到这台交换机」而不是「这台交换机有问题」——
        差别在于人知道该去改哪一段。"""
        doc = _minimal()
        doc["chains"][0]["instances"] = ["接入-1", "不存在的交换机"]
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "CHAIN_INSTANCE_UNKNOWN"]
        assert len(issues) == 1
        assert "不存在的交换机" in issues[0].message
        assert "没有在交换机段里声明过" in issues[0].message

    def test_duplicated_instance_in_same_chain_is_error(self) -> None:
        doc = _minimal()
        doc["chains"][0]["instances"] = ["接入-1", "接入-1"]
        result = check_intent(doc)
        assert "CHAIN_INSTANCE_DUPLICATED" in _codes(result)
        assert result.ok is False

    def test_unknown_switch_model_is_warn(self) -> None:
        """型号拼错是常见笔误，但路由段可能才是真相 —— 交给生成器去发现。"""
        doc = _minimal()
        doc["chains"][0]["switchModel"] = "业务接入交换机X"
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "CHAIN_MODEL_UNKNOWN"]
        assert len(issues) == 1
        assert issues[0].severity == "warn"

    def test_bad_capacity_is_error(self) -> None:
        doc = _minimal()
        doc["chains"][0]["capacity"] = 0
        result = check_intent(doc)
        assert "CHAIN_CAPACITY" in _codes(result)

    def test_unknown_order_is_error(self) -> None:
        doc = _minimal()
        doc["chains"][0]["order"] = "chaos"
        result = check_intent(doc)
        assert "CHAIN_ORDER" in _codes(result)

    def test_slot_major_is_accepted(self) -> None:
        doc = _minimal()
        doc["chains"][0]["order"] = "slot_major"
        result = check_intent(doc)
        assert "CHAIN_ORDER" not in _codes(result)


# ---------------------------------------------------------------------------
# 显式连线段
# ---------------------------------------------------------------------------


class TestExplicitLinks:
    def test_bad_endpoint_shape_is_error(self) -> None:
        doc = _minimal()
        doc["explicitLinks"][0]["a"] = ["只有一段"]
        result = check_intent(doc)
        assert "LINK_ENDPOINT_SHAPE" in _codes(result)

    def test_unknown_device_is_warn(self) -> None:
        doc = _minimal()
        doc["explicitLinks"][0]["a"] = ["神秘设备", "100GE1/0/1"]
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "LINK_DEVICE_UNKNOWN"]
        assert len(issues) == 1
        assert issues[0].severity == "warn"

    def test_duplicated_link_detected_in_both_directions(self) -> None:
        """A→B 与 B→A 是同一条线缆，写两遍会生成两根。"""
        doc = _minimal()
        doc["explicitLinks"].append({"a": ["接入-2", "100GE1/0/1"], "b": ["接入-1", "100GE1/0/1"]})
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "LINK_DUPLICATED"]
        assert len(issues) == 1
        assert issues[0].severity == "warn"

    def test_self_loop_is_error(self) -> None:
        doc = _minimal()
        doc["explicitLinks"][0]["b"] = ["接入-1", "100GE1/0/1"]
        result = check_intent(doc)
        assert "LINK_SELF_LOOP" in _codes(result)
        assert result.ok is False

    def test_unknown_cable_type_is_warn(self) -> None:
        doc = _minimal()
        doc["explicitLinks"][0]["cableType"] = "量子纠缠线"
        result = check_intent(doc)
        issues = [x for x in result.issues if x.code == "LINK_CABLE_TYPE"]
        assert len(issues) == 1
        assert issues[0].severity == "warn"


# ---------------------------------------------------------------------------
# 段级
# ---------------------------------------------------------------------------


class TestSections:
    @pytest.mark.parametrize("section", ["racks", "switches", "chains"])
    def test_empty_required_section_is_error(self, section: str) -> None:
        doc = _minimal()
        doc[section] = []
        result = check_intent(doc)
        assert "SECTION_EMPTY" in _codes(result)
        assert result.ok is False

    def test_empty_optional_section_is_fine(self) -> None:
        doc = _minimal()
        doc["devices"] = []
        doc["explicitLinks"] = []
        result = check_intent(doc)
        assert "SECTION_EMPTY" not in _codes(result)

    def test_as_dict_shape_for_frontend(self) -> None:
        result = check_intent(_minimal())
        data = result.as_dict()
        assert set(data) == {"ok", "errorCount", "warnCount", "infoCount", "sections", "issues"}
        assert isinstance(data["sections"], dict)
        assert "chains" in data["sections"]

    def test_section_label_is_chinese(self) -> None:
        doc = _minimal()
        doc["switches"][0]["instance"] = ""
        result = check_intent(doc)
        assert result.issues[0].section_label == "交换机"
        assert result.issues[0].as_dict()["sectionLabel"] == "交换机"


# ---------------------------------------------------------------------------
# 单元格编辑
# ---------------------------------------------------------------------------


class TestCoerceCell:
    @pytest.mark.parametrize(
        ("raw", "want"),
        [(3, 3), ("3", 3), (" 3 ", 3), (3.0, 3), ("-2", -2)],
    )
    def test_int_column_accepts_clean_values(self, raw: Any, want: int) -> None:
        value, err = coerce_cell("switches", "u", raw)
        assert err is None
        assert value == want

    def test_int_column_rejects_text_instead_of_silently_zeroing(self) -> None:
        """原表 `间隔机柜` 列真的出现过 `25G-SR`。静默转 0 会让结果全错而界面正常。"""
        value, err = coerce_cell("switches", "u", "abc")
        assert err is not None
        assert "需要整数" in err
        assert value == "abc"  # 原值保留，供人看见

    def test_float_that_is_not_integer_is_rejected(self) -> None:
        _, err = coerce_cell("switches", "u", 3.5)
        assert err is not None

    def test_blank_becomes_none(self) -> None:
        value, err = coerce_cell("switches", "u", "")
        assert (value, err) == (None, None)

    def test_bool_is_rejected_for_int(self) -> None:
        _, err = coerce_cell("switches", "u", True)
        assert err is not None


class TestApplyCell:
    def test_cell_is_written(self) -> None:
        doc = _minimal()
        ok, err = apply_cell(doc, "switches", 0, "u", "41")
        assert ok is True and err is None
        assert doc["switches"][0]["u"] == 41

    def test_failed_cell_leaves_doc_untouched(self) -> None:
        """写不进去时不能留半改状态。"""
        doc = _minimal()
        before = doc["switches"][0]["u"]
        ok, err = apply_cell(doc, "switches", 0, "u", "混乱")
        assert ok is False and err is not None
        assert doc["switches"][0]["u"] == before

    def test_row_out_of_range(self) -> None:
        doc = _minimal()
        ok, err = apply_cell(doc, "switches", 99, "u", 3)
        assert ok is False
        assert "越界" in err

    def test_unknown_section(self) -> None:
        doc = _minimal()
        ok, err = apply_cell(doc, "不存在", 0, "u", 3)
        assert ok is False

    def test_editing_creates_new_issue(self) -> None:
        """改坏一格，校验立刻能看见。"""
        doc = _minimal()
        apply_cell(doc, "switches", 0, "instance", "")
        result = check_intent(doc)
        assert "SW_INSTANCE_EMPTY" in _codes(result)


class TestRowOps:
    def test_append_row_returns_index(self) -> None:
        doc = _minimal()
        idx = append_row(doc, "devices")
        assert idx == 0
        assert doc["devices"] == [{}]

    def test_appended_row_must_fail_validation(self) -> None:
        """空行用 None 而不是 0 —— 校验必须报「缺字段」而不是悄悄过掉。"""
        doc = _minimal()
        append_row(doc, "switches")
        result = check_intent(doc)
        assert "SW_INSTANCE_EMPTY" in _codes(result)

    def test_append_with_template(self) -> None:
        doc = _minimal()
        append_row(doc, "devices", {"instance": "新设备", "model": "防火墙1",
                                    "channel": 1, "rack": 2, "u": 3})
        assert doc["devices"][0]["instance"] == "新设备"

    def test_delete_row(self) -> None:
        doc = _minimal()
        assert delete_row(doc, "switches", 0) is True
        assert len(doc["switches"]) == 1
        assert doc["switches"][0]["instance"] == "接入-2"

    def test_delete_out_of_range(self) -> None:
        doc = _minimal()
        assert delete_row(doc, "switches", 99) is False

    def test_delete_makes_chain_dangling(self) -> None:
        """删掉交换机后，引用它的交换机组必须报错 —— 这是编辑器最常见的坑。"""
        doc = _minimal()
        delete_row(doc, "switches", 0)
        result = check_intent(doc)
        assert "CHAIN_INSTANCE_UNKNOWN" in _codes(result)
        assert result.ok is False
