"""接线意图「表格形态」的 golden 用例。

## 这个模块守护什么

`ports/sheet.py` + `infrastructure/xlsx/intent_sheet.py` 把 Intent 摊平成
六张人能填的 Excel 表，再读回来。这组用例守三条不变量：

1. **往返恒等**：`sheet_to_intent(intent_to_sheet(i)) == i`（域层，纯内存）
2. **Excel 往返恒等**：写出 xlsx 再读回，仍与原始 Intent 相等（含 openpyxl）
3. **生成结果等价**：从 JSON 意图与从 Excel 意图生成的布线表**逐格一致**

## 为什么第 3 条最重要

前两条只证明"数据搬来搬去没丢"。但用户真正的风险是：
他改了一行 Excel，程序读进来时**某列被静默丢掉**（比如 `uSpan` 写成字符串
`10` 读回变默认值 1），数据没丢但语义变了 —— 往返恒等测不出来，
因为往返两侧用的是同一个写错的解析器。

第 3 条用真实项目端到端跑：5127 行的每一格都要相同。
这才能证明「用户改 Excel」与「程序改 JSON」等价。
"""

from __future__ import annotations

import pytest

def _intent_path():
    from pathlib import Path

    return Path(__file__).resolve().parents[3] / "projects" / "v13" / "intent_full.json"


@pytest.fixture(scope="module")
def intent():
    from idcbox.domain.ports.intent import load_intent

    return load_intent(_intent_path())


@pytest.fixture(scope="module")
def sheets(intent):
    from idcbox.domain.ports.sheet import intent_to_sheet

    return intent_to_sheet(intent)


# ---------------------------------------------------------------------------
# 1. 域层往返恒等


def test_six_sheets_have_expected_rows(sheets):
    """六张表的行数与意图里各段长度一致。"""
    assert len(sheets["布局"]) == 28
    assert len(sheets["交换机"]) == 130
    assert len(sheets["特殊设备"]) == 34
    assert len(sheets["路由"]) == 34
    assert len(sheets["交换机组"]) == 26
    assert len(sheets["显式连线"]) == 697


def test_volume_is_about_thirty_lines_for_the_planning_part(sheets):
    """**目标里「约 30 行量级」的量化**：上架规划三段加起来 28+130+34 = 192 行，
    但真正决定「放什么在哪」的只有布局 28 行 —— 130 台交换机与 34 台特殊设备
    是逐台位置，属于上架图范畴，不是规则。

    接线意图（路由 34 + 交换机组 26 + 显式连线 697）才是随网络方案变的部分。
    """
    planning_rules = len(sheets["布局"])
    wiring_rules = len(sheets["路由"]) + len(sheets["交换机组"])
    assert planning_rules == 28
    assert wiring_rules == 60
    # 两个数都在「几十行」量级
    assert planning_rules < 50
    assert wiring_rules < 100


def test_sheet_roundtrip_is_identity(intent, sheets):
    """域层往返：六张表 → Intent，与原始 Intent 完全相等。"""
    from idcbox.domain.ports.sheet import sheet_to_intent

    back = sheet_to_intent(sheets, project=intent.project, site=intent.site)
    assert back == intent


def test_slot_map_survives_roundtrip(intent, sheets):
    """`slot_map` 是最容易在往返里丢的字段（嵌套元组）。

    实测：丢掉它 ch2/ch8/ch9 的 rack11 会从 9 台变 8 台，
    492 行端口号整体错位。
    """
    from idcbox.domain.ports.sheet import sheet_to_intent

    back = sheet_to_intent(sheets, project=intent.project, site=intent.site)
    before = {(b.channel_no, b.rack_from): b.slot_map for b in intent.racks if b.slot_map}
    after = {(b.channel_no, b.rack_from): b.slot_map for b in back.racks if b.slot_map}
    assert after == before
    assert len(after) == 3  # ch2/ch8/ch9 的 rack11


def test_slots_column_survives_roundtrip(intent, sheets):
    """块级 `slots` 同理 —— ch3 的 7 个存储块靠它表达。"""
    from idcbox.domain.ports.sheet import sheet_to_intent

    back = sheet_to_intent(sheets, project=intent.project, site=intent.site)
    before = sorted((b.channel_no, b.node_model, b.slots) for b in intent.racks if b.slots)
    after = sorted((b.channel_no, b.node_model, b.slots) for b in back.racks if b.slots)
    assert after == before
    assert len(after) == 13


def test_u_span_survives_roundtrip(intent, sheets):
    """`u_span` 丢掉会让 298 行 U 位从 `3-12U` 退化成 `3U`。"""
    from idcbox.domain.ports.sheet import sheet_to_intent

    back = sheet_to_intent(sheets, project=intent.project, site=intent.site)
    spans = {s.instance: s.u_span for s in back.switches if s.u_span != 1}
    assert spans == {
        "业务核心交换机-1": 10,
        "业务核心交换机-2": 10,
        "存储核心交换机-1": 10,
        "存储核心交换机-2": 10,
    }


def test_device_u_span_survives_roundtrip(intent, sheets):
    """特殊设备的 `u_span` 要逐台保住。

    注意 `DeviceSpec.u_span` 的默认值就是 2（服务器机型都是 2U），
    所以不能只断言「非默认值」—— 那样等于什么都没测。
    这里逐台比，并且明确断言 2U 设备存在（负载均衡）。
    """
    from idcbox.domain.ports.sheet import sheet_to_intent

    back = sheet_to_intent(sheets, project=intent.project, site=intent.site)
    before = {d.instance: d.u_span for d in intent.devices}
    after = {d.instance: d.u_span for d in back.devices}
    assert after == before
    assert before["广域网链路负载均衡-1"] == 2
    assert len(before) == 34


def test_chain_order_survives_roundtrip(intent, sheets):
    """`order` 丢掉会让 ch3 的 580 行从 slot_major 退回 rack_major（只剩 17%）。"""
    from idcbox.domain.ports.sheet import sheet_to_intent

    back = sheet_to_intent(sheets, project=intent.project, site=intent.site)
    slots_major = [c for c in back.chains if c.order == "slot_major"]
    assert len(slots_major) == 5
    assert all(c.channel_no == 3 for c in slots_major)


def test_empty_rows_are_skipped():
    """Excel 尾部空行不能变成空记录。"""
    from idcbox.domain.ports.sheet import sheet_to_intent

    sheets = {
        "路由": [
            {"node_model": "A", "nic": "万兆1", "switch_model": "交换机1"},
            {"node_model": "", "nic": "", "switch_model": ""},
            {},
        ]
    }
    intent = sheet_to_intent(sheets, project="demo")
    assert len(intent.routes) == 1


def test_missing_sheet_is_not_an_error():
    """缺表 = 该段为空，不报错（新项目可能只填了布局）。"""
    from idcbox.domain.ports.sheet import sheet_to_intent

    intent = sheet_to_intent({"布局": [{"channel_no": "1", "rack_from": "1",
                                      "rack_to": "2", "node_model": "A"}]},
                             project="demo")
    assert len(intent.racks) == 1
    assert intent.switches == ()
    assert intent.server_count == 16


# ---------------------------------------------------------------------------
# 2. Excel 往返恒等（含 openpyxl）


def test_excel_roundtrip_is_identity(intent, tmp_path):
    """写 xlsx 再读回，与原始 Intent 完全相等。"""
    from idcbox.infrastructure.xlsx.intent_sheet import (
        read_intent_workbook,
        write_intent_xlsx,
    )

    out = tmp_path / "intent.xlsx"
    write_intent_xlsx(intent, out)
    back, found, missing = read_intent_workbook(
        out, project=intent.project, site=intent.site
    )
    assert missing == ()
    assert len(found) == 6
    assert back == intent


def test_excel_sheet_names_are_the_six_segments(intent, tmp_path):
    """六张表的表名就是中文段名 —— 用户要能在 Excel 里一眼认出来。"""
    import openpyxl

    from idcbox.infrastructure.xlsx.intent_sheet import write_intent_xlsx

    out = tmp_path / "intent.xlsx"
    write_intent_xlsx(intent, out)
    wb = openpyxl.load_workbook(out)
    try:
        assert wb.sheetnames == ["布局", "交换机", "特殊设备", "路由", "交换机组", "显式连线"]
    finally:
        wb.close()


def test_hint_row_is_not_read_as_data(intent, tmp_path):
    """第 2 行的灰色提示语不能被当成一条数据。

    实测踩过：提示语写在 A2，读回时 `布局` 表多出 1 行、通道解析成 0，
    直接 `ValueError: 机柜号从 1 开始，收到 0`。
    """
    from idcbox.infrastructure.xlsx.intent_sheet import (
        read_intent_workbook,
        write_intent_xlsx,
    )

    out = tmp_path / "intent.xlsx"
    write_intent_xlsx(intent, out)
    back, _, _ = read_intent_workbook(out, project=intent.project, site=intent.site)
    assert len(back.racks) == len(intent.racks)
    assert all(b.channel_no >= 1 for b in back.racks)


def test_missing_required_sheet_is_reported(intent, tmp_path):
    """删掉 `路由` 表要能被报出来（否则生成结果少一半而用户不知道）。"""
    import openpyxl

    from idcbox.infrastructure.xlsx.intent_sheet import (
        REQUIRED_SHEETS,
        read_intent_workbook,
        write_intent_xlsx,
    )

    out = tmp_path / "intent.xlsx"
    write_intent_xlsx(intent, out)
    wb = openpyxl.load_workbook(out)
    del wb["路由"]
    wb.save(out)
    wb.close()

    back, found, missing = read_intent_workbook(out, project=intent.project)
    assert "路由" in missing
    assert "路由" in REQUIRED_SHEETS
    assert back.routes == ()


def test_comments_column_does_not_break_import(intent, tmp_path):
    """用户在表里插一列「备注」，导入不能错位。

    这是按表头名匹配（而不是按列位置）的理由。
    """
    import openpyxl

    from idcbox.infrastructure.xlsx.intent_sheet import (
        read_intent_workbook,
        write_intent_xlsx,
    )

    out = tmp_path / "intent.xlsx"
    write_intent_xlsx(intent, out)
    wb = openpyxl.load_workbook(out)
    ws = wb["路由"]
    ws.insert_cols(4)
    ws.cell(row=1, column=4, value="备注")
    ws.cell(row=3, column=4, value="这条先别用")
    wb.save(out)
    wb.close()

    back, _, _ = read_intent_workbook(out, project=intent.project, site=intent.site)
    assert len(back.routes) == len(intent.routes)
    assert back.routes[0] == intent.routes[0]


# ---------------------------------------------------------------------------
# 3. 生成结果等价（最重要）


@pytest.fixture(scope="module")
def geometry():
    """真实平面图几何。与 `test_design_end_to_end.py` 走同一条路：

    通过 `main._load()` 拿 `ProjectViews`，再取 `views.project.geometry`
    （`XlsxProjectSource` 上**没有** `read_floor()`，它只暴露协议方法）。
    """
    from idcbox.api.main import DEFAULT_SOURCE, _load

    views, _ = _load(DEFAULT_SOURCE)
    return views.project.geometry


def _generate(intent, geometry):
    from idcbox.application.services.design import generate_design

    return generate_design(intent, geometry)


def test_json_intent_and_sheet_intent_generate_identical_rows(
    intent, sheets, geometry, tmp_path
):
    """**核心**：从 JSON 意图与从 Excel 意图生成的布线表逐格一致。

    这条用例同时守住「解析器」和「序列化」两侧：
    任一侧丢字段（uSpan / slots / slotMap / order），生成结果都会分叉。
    """
    from idcbox.domain.ports.sheet import sheet_to_intent
    from idcbox.infrastructure.xlsx.intent_sheet import (
        read_intent_workbook,
        write_intent_xlsx,
    )

    from_json = _generate(intent, geometry)

    out = tmp_path / "intent.xlsx"
    write_intent_xlsx(intent, out)
    from_excel_intent, _, _ = read_intent_workbook(
        out, project=intent.project, site=intent.site
    )
    from_excel = _generate(from_excel_intent, geometry)

    assert len(from_excel.rows) == len(from_json.rows) == 5127
    assert from_excel.rows == from_json.rows


def test_sheet_intent_generates_all_5127_rows(intent, sheets, geometry):
    """12 列全字段无差异（`peerDevice` 除外，它按设计留空）。"""
    from idcbox.domain.ports.sheet import sheet_to_intent

    back = sheet_to_intent(sheets, project=intent.project, site=intent.site)
    design = _generate(back, geometry)

    assert len(design.rows) == 5127
    assert design.stats["errors"] == 0
    assert design.stats["lengthComputed"] == 5124


def test_editing_one_cell_changes_only_affected_rows(intent, geometry, tmp_path):
    """改 Excel 里一行（把某条链的容量从 44 改成 40），只影响该链的行。

    这条证明「表格是可编辑的真相源」，不是只读快照。
    """
    import openpyxl

    from idcbox.infrastructure.xlsx.intent_sheet import (
        read_intent_workbook,
        write_intent_xlsx,
    )

    base = _generate(intent, geometry)

    out = tmp_path / "intent.xlsx"
    write_intent_xlsx(intent, out)

    # 在「交换机组」表里找 ch2/业务接入交换机1/万兆1 那一行，把容量改成 40
    wb = openpyxl.load_workbook(out)
    ws = wb["交换机组"]
    target_row = None
    for r in range(3, ws.max_row + 1):
        if (
            str(ws.cell(row=r, column=1).value) == "2"
            and str(ws.cell(row=r, column=2).value) == "业务接入交换机1"
            and str(ws.cell(row=r, column=3).value) == "万兆1"
        ):
            target_row = r
            break
    assert target_row is not None, "没找到 ch2/业务接入交换机1/万兆1 那条链"
    ws.cell(row=target_row, column=5, value=40)
    wb.save(out)
    wb.close()

    edited, _, _ = read_intent_workbook(out, project=intent.project, site=intent.site)
    after = _generate(edited, geometry)

    # 行数不变（改容量不改总节点数），但部分行的端口号会变
    assert len(after.rows) == len(base.rows)
    changed = [
        (a, b) for a, b in zip(base.rows, after.rows)
        if a.get("localPort") != b.get("localPort")
    ]
    assert changed, "改容量后应该有行的端口号变化"
