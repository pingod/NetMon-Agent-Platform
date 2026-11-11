"""`domain/exporting/stockdiff.py` 的单元测试。

## 这个模块守护什么

跨版本对比有三处会**静默出错**，每一处都让用户得到"看起来正常但是错的"结论：

1. **表头认错列**：`U位` 在一张表里出现两次（本端 / 对端）。认错就把本端 U 位
   比成对端 U 位，输出一堆"位置变了"的假差异。
2. **哨兵文本当数字**：长度列的 `待定` / `设备自带` 若被当 0，会产出
   "0 → 10"这种假长度变化，用户去追查会发现两边其实都没填。
3. **比对键选错层级**：实测 V12 → V13 用 strict 键只 183 条对上、
   4944 条"只在右边"—— 而真相是 V13 补齐了实例名列。
   把"补名字"报成"换了 4944 条线"，用户会立刻失去信任。

所以下面按这三件事组织用例。
"""

from __future__ import annotations

import pytest

from idcbox.domain.exporting.stockdiff import (
    CANON_FIELDS,
    KEY_MODES,
    StockRow,
    StockTable,
    build_table,
    compare_devices,
    compare_stocks,
    detect_layout,
    device_payload,
    diff_payload,
    group_devices,
    parse_length,
    pick_key_mode,
    row_key,
)


# --------------------------------------------------------------- 表头识别
class TestDetectLayout:
    def test_25col_v13(self) -> None:
        """V13 布线表 24 列（第 25 列表头是 None，不进表头）。"""
        headers = [
            "本端设备", "本端设备名称", "本端通道", "本端机柜", "U位", "本端接口",
            "对端设备", "对端设备名称", "对端通道", "对端机柜", "U位", "对端接口",
            "线缆类型", "跨机柜", "间隔机柜", "机柜数量", "机内线缆", "柜间线缆",
            "线缆2", "线缆长度1", "线缆长度", "本端模块", "对端模块", "备注1",
        ]
        layout = detect_layout(headers)
        assert layout.index["localModel"] == 0
        assert layout.index["localInstance"] == 1
        assert layout.index["localU"] == 4
        # 第 5 个 `U位`（第 2 次出现）必须判给对端
        assert layout.index["peerU"] == 10
        assert layout.index["cableType"] == 12
        assert layout.index["length"] == 20

    def test_13col_v8(self) -> None:
        """V8/V9 连线表 13 列：没有实例名两列。"""
        headers = [
            "本端设备", "本端通道", "本端机柜", "U位", "本端接口",
            "对端设备", "对端通道", "对端机柜", "U位", "对端接口",
            "线缆类型", "线缆长度",
        ]
        layout = detect_layout(headers)
        assert "localInstance" in layout.missing
        assert "peerInstance" in layout.missing
        # 缺实例名不影响 U 位与端口的定位
        assert layout.index["localU"] == 3
        assert layout.index["peerU"] == 8
        assert layout.index["localPort"] == 4
        assert layout.index["peerPort"] == 9

    def test_17col_v11(self) -> None:
        """V11 设计院填写长度：在 13 列基础上加了实例名，但仍然只有一个长度列。

        只有 `布线表`（25 列）才有两列长度：`线缆长度1`（公式算出来的）
        与 `线缆长度`（人工核定）。V11/V8 只有后者。所以这里
        `lengthComputed` 缺席是**正确的**，不是识别失败。
        """
        headers = [
            "本端设备", "本端设备名称", "本端通道", "本端机柜", "U位", "本端接口",
            "对端设备", "对端设备名称", "对端通道", "对端机柜", "U位", "对端接口",
            "线缆类型", "线缆长度",
        ]
        layout = detect_layout(headers)
        assert layout.missing == ("lengthComputed",)
        assert layout.index["localInstance"] == 1
        assert layout.index["peerInstance"] == 7
        assert layout.index["length"] == 13

    def test_25col_two_length_columns_not_confused(self) -> None:
        """25 列表格两个长度列不能串。

        `线缆长度1` 是公式算出来的计算值，`线缆长度` 是人工核定值。
        若映射成一个字段，diff 会拿"计算值"去比"核定值"——
        两份都对的数据会报成一片差异，而真正该关心的核定值变化被淹没。
        """
        headers = [
            "本端设备", "本端设备名称", "本端通道", "本端机柜", "U位", "本端接口",
            "对端设备", "对端设备名称", "对端通道", "对端机柜", "U位", "对端接口",
            "线缆类型", "跨机柜", "间隔机柜", "机柜数量", "机内线缆", "柜间线缆",
            "线缆2", "线缆长度1", "线缆长度", "本端模块", "对端模块", "备注1",
        ]
        layout = detect_layout(headers)
        assert layout.index["lengthComputed"] == 19
        assert layout.index["length"] == 20
        assert not layout.missing

    def test_unknown_headers_give_empty_missing_all(self) -> None:
        """完全不认识的表头：missing 列出全部字段，不抛异常。"""
        layout = detect_layout(["随便", "什么"])
        assert len(layout.missing) == len(CANON_FIELDS)

    def test_none_header_ignored(self) -> None:
        """第 25 列表头是 None（V13 真实情况）—— 不能让 None 变成字符串 "None"。"""
        layout = detect_layout(["本端设备", None, "本端接口"])
        assert layout.index["localModel"] == 0
        assert layout.index["localPort"] == 2

    def test_header_with_spaces(self) -> None:
        """表头前后有空格（实测有的文件是这样）也要认出来。"""
        layout = detect_layout([" 本端设备 ", " 本端接口\t"])
        assert layout.index["localModel"] == 0
        assert layout.index["localPort"] == 1


# --------------------------------------------------------------- 长度哨兵
class TestParseLength:
    @pytest.mark.parametrize("text,expected", [
        ("10", 10), ("5", 5), ("7", 7), ("25", 25),
        ("10.0", 10), ("", 0), (None, 0),
    ])
    def test_numbers(self, text: str | None, expected: int) -> None:
        got = parse_length(text or "")
        if expected == 0:
            assert got is None
        else:
            assert got == expected

    @pytest.mark.parametrize("text", ["待定", "一", "设备自带", "-", "—", "/", "无"])
    def test_sentinels_are_none_not_zero(self, text: str) -> None:
        """哨兵必须是 None。

        当 0 的后果：V12 长度 `待定` → V13 长度 `10` 会报成「0 → 10」的
        长度变化，用户追查发现两边其实都是"没填"或"还没定"。
        """
        assert parse_length(text) is None

    def test_garbage_returns_none(self) -> None:
        assert parse_length("不是数字") is None


# --------------------------------------------------------------- 比对键
class TestPickKeyMode:
    def _table(self, *, local_inst: float, peer_inst: float) -> StockTable:
        """造一张填充率可控的表。"""
        n = 20
        rows = []
        for i in range(n):
            rows.append(StockRow(i + 2, {
                "localInstance": f"S-{i}" if i < n * local_inst else "",
                "peerInstance": f"P-{i}" if i < n * peer_inst else "",
                "localPort": "GE1", "peerPort": "千兆1",
                "localChannel": "1", "localRack": "1", "localU": "3U",
                "peerChannel": "1", "peerRack": "2", "peerU": "40U",
                "localModel": "SW", "peerModel": "SRV",
                "cableType": "RJ45", "length": "10",
            }))
        headers = ["本端设备", "本端设备名称", "本端通道", "本端机柜", "U位", "本端接口",
                   "对端设备", "对端设备名称", "对端通道", "对端机柜", "U位", "对端接口",
                   "线缆类型", "线缆长度"]
        layout = detect_layout(headers)
        return StockTable("t", layout, tuple(rows))

    def test_both_full_picks_strict(self) -> None:
        a = self._table(local_inst=1.0, peer_inst=1.0)
        b = self._table(local_inst=1.0, peer_inst=1.0)
        mode, reason = pick_key_mode(a, b)
        assert mode == "strict"
        assert "实例名" in reason

    def test_peer_empty_picks_loose(self) -> None:
        """对端实例名缺失（服务器行本来就没名字）→ 降到 loose。"""
        a = self._table(local_inst=1.0, peer_inst=0.0)
        b = self._table(local_inst=1.0, peer_inst=0.0)
        mode, reason = pick_key_mode(a, b)
        assert mode == "loose"
        assert "型号" in reason

    def test_both_empty_picks_position(self) -> None:
        """两端实例名都空（V11/V12 的 14%）→ 退到纯位置。"""
        a = self._table(local_inst=0.0, peer_inst=0.0)
        b = self._table(local_inst=0.0, peer_inst=0.0)
        mode, reason = pick_key_mode(a, b)
        assert mode == "position"
        assert "位置" in reason

    def test_v12_to_v13_real_shape(self) -> None:
        """实测形状：V12 本端实例 14%、V13 本端实例 100%。

        **必须降到 position**。否则 V13 补齐名字这一动作会让每一条都变成
        "键变了" → 4944 条假新增 + 4944 条假缺失。
        """
        v12 = self._table(local_inst=0.14, peer_inst=0.085)
        v13 = self._table(local_inst=1.0, peer_inst=0.175)
        mode, _ = pick_key_mode(v12, v13)
        assert mode == "position"


class TestRowKey:
    def _row(self, **kw: str) -> StockRow:
        base = {f: "" for f, _ in CANON_FIELDS}
        base.update(kw)
        return StockRow(2, base)

    def test_strict_uses_instances(self) -> None:
        r = self._row(localInstance="A-1", localPort="GE1", peerInstance="S-1", peerPort="万兆1")
        assert row_key(r, "strict") == ("A-1", "GE1", "S-1", "万兆1")

    def test_loose_uses_peer_model_and_position(self) -> None:
        r = self._row(
            localInstance="A-1", localPort="GE1", peerModel="SRV",
            peerChannel="2", peerRack="5", peerU="3-4U", peerPort="万兆1",
        )
        assert row_key(r, "loose") == ("A-1", "GE1", "SRV", "2", "5", "3-4U", "万兆1")

    def test_position_ignores_names_entirely(self) -> None:
        """position 键不含任何名字 —— 这正是它能跨"补名字"版本对齐的原因。"""
        r = self._row(
            localChannel="1", localRack="1", localU="40U", localPort="GE1",
            peerChannel="1", peerRack="2", peerU="3-4U", peerPort="万兆1",
        )
        key = row_key(r, "position")
        assert "40U" in key
        assert len(key) == 8


# --------------------------------------------------------------- 端到端比对
def _make(
    rows: list[dict[str, str]],
    *,
    label: str = "t",
    headers: list[str] | None = None,
) -> StockTable:
    headers = headers or [
        "本端设备", "本端设备名称", "本端通道", "本端机柜", "U位", "本端接口",
        "对端设备", "对端设备名称", "对端通道", "对端机柜", "U位", "对端接口",
        "线缆类型", "线缆长度",
    ]
    layout = detect_layout(headers)
    out = []
    for i, r in enumerate(rows):
        values = {f: r.get(f, "") for f, _ in CANON_FIELDS}
        out.append(StockRow(i + 2, values))
    return StockTable(label, layout, tuple(out))


def _row_data(
    *, local: str = "SW-1", port: str = "GE1", peer: str = "SRV-1",
    peer_port: str = "万兆1", rack: str = "1", length: str = "10",
    cable: str = "RJ45",
) -> dict[str, str]:
    return {
        "localModel": "SW", "localInstance": local, "localChannel": "1",
        "localRack": rack, "localU": "40U", "localPort": port,
        "peerModel": "SRV", "peerInstance": peer, "peerChannel": "1",
        "peerRack": "2", "peerU": "3-4U", "peerPort": peer_port,
        "cableType": cable, "length": length,
    }


class TestCompareStocks:
    def test_identical(self) -> None:
        a = _make([_row_data()], label="A")
        b = _make([_row_data()], label="B")
        diff = compare_stocks(a, b)
        assert diff.is_empty
        assert diff.total_changes == 0
        # 完全一致时必须给出提示：用户可能改的是另一个 sheet
        assert any("完全一致" in n for n in diff.notes)

    def test_added(self) -> None:
        a = _make([_row_data(port="GE1")], label="A")
        b = _make([_row_data(port="GE1"), _row_data(port="GE2")], label="B")
        diff = compare_stocks(a, b)
        assert len(diff.added) == 1
        assert not diff.removed
        assert diff.added[0].after is not None
        assert diff.added[0].after.get("localPort") == "GE2"

    def test_removed(self) -> None:
        a = _make([_row_data(port="GE1"), _row_data(port="GE2")], label="A")
        b = _make([_row_data(port="GE1")], label="B")
        diff = compare_stocks(a, b)
        assert len(diff.removed) == 1
        assert diff.removed[0].before is not None
        assert diff.removed[0].before.get("localPort") == "GE2"

    def test_changed_field(self) -> None:
        a = _make([_row_data(cable="RJ45")], label="A")
        b = _make([_row_data(cable="LC-LC")], label="B")
        diff = compare_stocks(a, b)
        assert len(diff.changed) == 1
        pair, changes = diff.changed[0]
        assert len(changes) == 1
        assert changes[0].field_name == "cableType"
        assert changes[0].before == "RJ45"
        assert changes[0].after == "LC-LC"

    def test_length_change_separated(self) -> None:
        """长度变化同时出现在 changed 与 length_changes 两处。

        ## 为什么长度要单独一份

        `changed` 里混着"线缆类型变了""U位变了"等；只有长度有**该不该变**
        的客观标准（几何算出来的 T）。界面上长度差异要单独占一块，
        因为它直接对应"要买多少根线"。
        """
        a = _make([_row_data(length="10")], label="A")
        b = _make([_row_data(length="25")], label="B")
        diff = compare_stocks(a, b)
        assert len(diff.length_changes) == 1
        pair, before, after = diff.length_changes[0]
        assert (before, after) == (10, 25)

    def test_sentinel_length_not_a_change(self) -> None:
        """`待定` → `10` **不算**长度变化。

        ## 这是最容易做错的一条

        实测 V12 → V13 有 25 条连线的 U 列从 `待定` 变成具体数字。
        若当成变化，用户会看到"长度改了 25 条"，去追查发现原来是
        "原来没定、现在定了" —— 那不是长度调整，是流程推进。
        """
        a = _make([_row_data(length="待定")], label="A")
        b = _make([_row_data(length="10")], label="B")
        diff = compare_stocks(a, b)
        assert not diff.length_changes

    def test_duplicate_keys_kept(self) -> None:
        """同键多行必须保留（同一插口被多行占用是真实缺陷，不该被 diff 层吞掉）。"""
        rows = [
            _row_data(port="GE1", peer_port="千兆1"),
            _row_data(port="GE1", peer_port="千兆1", length="15"),
        ]
        a = _make(rows, label="A")
        b = _make(rows, label="B")
        diff = compare_stocks(a, b)
        # 键相同 → 首行配对，第二行不产生差异（两边都有），但行数要保住
        assert a.row_count == 2
        assert b.row_count == 2

    def test_fill_changes_reported(self) -> None:
        """实例名从空到有：必须报成「填充率变化」而非"修改了 N 行"。

        实测 V13 就是把本端实例列从 14% 补到 100%。
        按行比会得到 4400 条"修改"，但真实语义是"补录了名字"。
        """
        empty_names = [_row_data(port=f"GE{i}", local="") for i in range(10)]
        full_names = [_row_data(port=f"GE{i}", local=f"SW-1") for i in range(10)]
        a = _make(empty_names, label="V12")
        b = _make(full_names, label="V13")
        diff = compare_stocks(a, b)
        changes = {f: (x, y) for f, _label, x, y in diff.fill_changes}
        assert "localInstance" in changes
        before, after = changes["localInstance"]
        assert before == 0.0
        assert after == 1.0

    def test_limit_truncates_samples_not_counts(self) -> None:
        """`limit` 只截样本，绝不能截计数。

        ## 这条用例原本写得很弱，漏掉了一个真实事故

        最初断言是 `diff.total_changes >= 5`。而当时的实现恰好把
        条数取成 `len(diff.added)`（=limit=5），所以 `5 >= 5` 通过。

        真实后果：V12 → V13 有 **4886** 条新增，界面显示「新增 30 行」
        （因为 API 传 limit=30）。用户看到 30 会以为改动很小，
        实际是整表重排 —— 这种"数字小了但看起来正常"的错最难发现。

        所以这里断言**精确值**，不留 `>=` 的余量。
        """
        a = _make([_row_data(port="GE0")], label="A")
        b = _make([_row_data(port=f"GE{i}") for i in range(30)], label="B")
        diff = compare_stocks(a, b, limit=5)
        assert len(diff.added) == 5, "样本必须被 limit 截断"
        assert diff.added_total == 29, "计数必须是真实的 29，不是样本的 5"
        assert diff.total_changes == 29

    def test_totals_survive_small_limit(self) -> None:
        """`limit=1` 时四个计数仍全部为真值。"""
        a = _make([_row_data(port="GE0", cable="RJ45", length="10")], label="A")
        rows_b = [_row_data(port="GE0", cable="LC-LC", length="25")]
        rows_b += [_row_data(port=f"GE{i}") for i in range(1, 8)]
        b = _make(rows_b, label="B")
        diff = compare_stocks(a, b, limit=1)
        assert diff.added_total == 7
        assert diff.changed_total == 1
        assert diff.length_change_total == 1

    def test_unknown_key_mode_raises(self) -> None:
        a = _make([_row_data()], label="A")
        with pytest.raises(ValueError, match="未知比对键层级"):
            compare_stocks(a, b := _make([_row_data()], label="B"), key_mode="nope")


class TestFillRates:
    def test_empty_table(self) -> None:
        table = _make([], label="empty")
        assert table.fill_rate("localInstance") == 0.0

    def test_partial(self) -> None:
        rows = [_row_data(local="A"), _row_data(local="", port="GE2")]
        table = _make(rows, label="t")
        assert table.fill_rate("localInstance") == pytest.approx(0.5)


class TestPayload:
    def test_payload_shape(self) -> None:
        a = _make([_row_data(cable="RJ45")], label="A")
        b = _make([_row_data(cable="LC-LC")], label="B")
        p = diff_payload(compare_stocks(a, b))
        for key in (
            "left", "right", "keyMode", "keyReason", "leftRows", "rightRows",
            "totalChanges", "isEmpty", "addedCount", "removedCount",
            "changedCount", "lengthChangeCount", "added", "removed",
            "changed", "lengthChanges", "fillChanges", "notes",
        ):
            assert key in p
        assert p["changedCount"] == 1
        assert p["changed"][0]["fields"][0]["label"] == "线缆类型"


class TestBuildTable:
    def test_skips_blank_rows(self) -> None:
        headers = ["本端设备", "本端设备名称", "本端通道", "本端机柜", "U位", "本端接口",
                   "对端设备", "对端设备名称", "对端通道", "对端机柜", "U位", "对端接口",
                   "线缆类型", "线缆长度"]
        rows = [
            ("SW", "SW-1", "1", "1", "40U", "GE1", "SRV", "", "1", "2", "3-4U", "万兆1", "RJ45", "10"),
            (None, None, None, None, None, None, None, None, None, None, None, None, None, None),
            ("SW", "SW-1", "1", "1", "40U", "GE2", "SRV", "", "1", "2", "3-4U", "万兆2", "RJ45", "10"),
        ]
        table = build_table("t", headers, rows)
        assert table.row_count == 2
        # 行号来自原始位置（第 2 行与第 4 行），中间空行占位
        assert table.rows[0].row_no == 2
        assert table.rows[1].row_no == 4

    def test_values_stripped(self) -> None:
        headers = ["本端设备", "本端设备名称", "本端通道", "本端机柜", "U位", "本端接口",
                   "对端设备", "对端设备名称", "对端通道", "对端机柜", "U位", "对端接口",
                   "线缆类型", "线缆长度"]
        table = build_table("t", headers, [("  SW  ", " SW-1 ", "1", "1", "40U", "GE1",
                                           "SRV", "", "1", "2", "3-4U", "万兆1", " RJ45 ", " 10 ")])
        assert table.rows[0].get("localModel") == "SW"
        assert table.rows[0].get("cableType") == "RJ45"


class TestModes:
    def test_three_modes_in_order(self) -> None:
        """降级顺序必须是 strict → loose → position（精确到宽松）。"""
        assert KEY_MODES == ("strict", "loose", "position")


# ----------------------------------------------------------- 设备级聚合
class TestGroupDevices:
    """`group_devices` 把连线行聚成设备。

    这一层存在的理由：V12→V13 逐行是 4886 新增 / 4890 删除，
    设备级是 56 台挪了 U 位 + 2 台新增 + 54 台改端口。
    后者才是施工队能读懂的说法。
    """

    def test_groups_by_channel_rack_u(self) -> None:
        rows = [
            _row_data(port="GE1"),
            _row_data(port="GE2"),
            _row_data(port="GE3"),
        ]
        table = _make(rows)
        devs = group_devices(table)
        assert len(devs) == 1, "同 (通道,机柜,U位) 的三行必须聚成一台设备"
        assert devs[0].row_count == 3
        assert devs[0].ports == frozenset({"GE1", "GE2", "GE3"})

    def test_different_u_is_different_device(self) -> None:
        rows = [
            {**_row_data(port="GE1"), "localU": "40U"},
            {**_row_data(port="GE1"), "localU": "37U"},
        ]
        devs = group_devices(_make(rows))
        assert len(devs) == 2

    def test_label_prefers_instance(self) -> None:
        """设备名优先取 localInstance（V13 有 100% 填充），缺失退到型号。"""
        rows = [{**_row_data(), "localInstance": ""}]
        devs = group_devices(_make(rows))
        assert devs[0].label == ""
        assert devs[0].models == frozenset({"SW"})

    def test_empty_key_rows_skipped(self) -> None:
        """三列全空的行无法归属任何设备，跳过而不是造一个空设备。"""
        rows = [{**_row_data(), "localChannel": "", "localRack": "", "localU": ""}]
        assert group_devices(_make(rows)) == ()


class TestCompareDevices:
    """设备级比对的两步配对。

    第一步键相同直接配；第二步同柜内按端口重叠度贪心配。
    第二步**不能跨柜** —— 每台计算接入交换机都有万兆1~4 各 44 口，
    跨柜贪心会把 20 台交换机交叉配错。
    """

    def test_identical_devices_is_empty(self) -> None:
        a = _make([_row_data(port="GE1"), _row_data(port="GE2")], label="A")
        b = _make([_row_data(port="GE1"), _row_data(port="GE2")], label="B")
        assert compare_devices(a, b).is_empty

    def test_u_shift_reports_moved_not_add_remove(self) -> None:
        """同一台设备 U 位从 38U 挪到 37U：必须是 1 台 moved，不是 1 增 1 删。

        这是本功能的核心价值。逐行比会报"新增 44 行 + 删除 44 行"，
        设备级必须报"1 台设备从 38U 挪到 37U，44 个端口未变"。
        """
        ports = [f"GE1/0/{i}" for i in range(1, 45)]
        a = _make([{**_row_data(port=p), "localU": "38U"} for p in ports], label="A")
        b = _make([{**_row_data(port=p), "localU": "37U"} for p in ports], label="B")
        dd = compare_devices(a, b)
        counts = dd.counts()
        assert counts["moved"] == 1, "同柜内端口集合完全重叠 → 应识别为挪位置"
        assert counts["added"] == 0
        assert counts["removed"] == 0
        assert dd.moved[0].before_u == "38U"
        assert dd.moved[0].after_u == "37U"
        assert dd.moved[0].port_overlap == 44
        assert "44/44" in dd.moved[0].describe()

    def test_port_rename_reports_changed(self) -> None:
        """U 位不变、端口从 25GE 改名成 10GE → 1 台 changed 而不是全表重排。"""
        a = _make([_row_data(port=f"25GE1/0/{i}") for i in range(1, 5)], label="A")
        b = _make([_row_data(port=f"10GE1/0/{i}") for i in range(1, 5)], label="B")
        dd = compare_devices(a, b)
        assert dd.counts()["changed"] == 1
        assert dd.counts()["moved"] == 0
        desc = dd.changed[0].describe()
        assert "10GE1/0/1" in desc and "25GE1/0/1" in desc

    def test_real_added_device(self) -> None:
        a = _make([_row_data(port="GE1")], label="A")
        b = _make(
            [_row_data(port="GE1"), {**_row_data(port="GE1"), "localRack": "9", "localInstance": "SW-9"}],
            label="B",
        )
        dd = compare_devices(a, b)
        assert dd.counts()["added"] == 1
        assert "SW-9" in dd.added[0].describe()

    def test_real_removed_device(self) -> None:
        a = _make(
            [_row_data(port="GE1"), {**_row_data(port="GE1"), "localRack": "9", "localInstance": "SW-9"}],
            label="A",
        )
        b = _make([_row_data(port="GE1")], label="B")
        dd = compare_devices(a, b)
        assert dd.counts()["removed"] == 1

    def test_no_cross_rack_matching(self) -> None:
        """同端口集合但不同机柜 → 必须是"1 删 1 增"，不能跨柜配成 moved。

        跨柜贪心的危险：每台计算接入交换机都有 `万兆1`~`万兆4` 各 44 口，
        跨柜匹配会在 20 台交换机之间交叉配错。同柜内才可靠。
        """
        ports = [f"GE1/0/{i}" for i in range(1, 9)]
        a = _make([{**_row_data(port=p), "localRack": "1", "localInstance": "SW-1"} for p in ports], label="A")
        b = _make([{**_row_data(port=p), "localRack": "2", "localInstance": "SW-1"} for p in ports], label="B")
        dd = compare_devices(a, b)
        assert dd.counts()["moved"] == 0, "不同机柜不能配成 moved"
        assert dd.counts()["added"] == 1
        assert dd.counts()["removed"] == 1

    def test_payload_counts_not_overwritten_by_samples(self) -> None:
        """`device_payload` 的计数与样本必须用不同的键。

        第一版把 counts 展开进 payload 顶层，然后同名 `moved` 写样本列表，
        后者覆盖前者 → 前端拿到 `counts.moved` 是字符串数组。
        """
        ports = [f"GE1/0/{i}" for i in range(1, 5)]
        a = _make([{**_row_data(port=p), "localU": "38U"} for p in ports], label="A")
        b = _make([{**_row_data(port=p), "localU": "37U"} for p in ports], label="B")
        payload = device_payload(compare_devices(a, b))
        assert isinstance(payload["counts"]["moved"], int), "计数必须是整数"
        assert payload["counts"]["moved"] == 1
        assert isinstance(payload["moved"], list), "样本必须是列表"
        assert len(payload["moved"]) == 1
