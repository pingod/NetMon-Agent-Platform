"""导出保真 golden：24 列 × 5127 行逐格相等（AC-1 / AC-3 的唯一判据）。

## 为什么这条用例值整个项目的信任

用户的第一诉求是「替代 Excel 但不改变交付物形态」。如果导出文件与手里的 Excel
有任何一格不同，用户就得再做一次人工核对 —— 那产品就白做了。所以判据必须是
**零容忍的逐格相等**，不是"看起来差不多"。

## 判据的三个细节

1. **数值按 `excel15` 比**：Excel 内部是双精度，`P*0.6*1.2` 存 3.6 而 Python 算出
   3.5999999999999996。必须收敛到 15 位有效数字再比，否则会有 170 行假差异。
2. **文本严格比**：包括 sentinel（`待定`/`一`/`设备自带`）和首尾空格。不改不删。
3. **两列同名 `U位`** 必须各自独立——第 5 列是设备 U 位，第 11 列是对端设备 U 位，
   列名完全一样，不能靠名字区分位置。

## 两种模式都要验

`value` 模式（原值回写）是基线；`formula` 模式（输出活公式）要验公式文本与源文件一致。
"""

from __future__ import annotations

import warnings
from pathlib import Path
from typing import Any

import pytest

pytestmark = pytest.mark.golden

warnings.filterwarnings("ignore")


def _norm_cell(v: Any) -> Any:
    """单元格归一：数值走 excel15，文本严格，空统一成 None。"""
    from idcbox.domain.cable_length.numeric import excel15

    if v is None:
        return None
    if isinstance(v, bool):
        return v
    if isinstance(v, (int, float)):
        x = excel15(float(v))
        return int(x) if float(x).is_integer() else x
    s = str(v)
    return s if s.strip() else None


def _build_export_rows(sheet) -> list:
    from idcbox.domain.exporting.contract25 import CONTRACT_25
    from idcbox.infrastructure.xlsx.exporter import ExportRow

    rows = []
    for raw_row in sheet.rows:
        values: dict[str, Any] = {}
        raw: dict[int, Any] = {}
        for spec in CONTRACT_25:
            v = raw_row.values[spec.index] if spec.index < len(raw_row.values) else None
            raw[spec.index] = v
            if spec.field:
                values[spec.field] = v
        rows.append(ExportRow(source_row_no=raw_row.row_no, values=values, raw=raw))
    return rows


class TestContractDeclaration:
    """契约表本身必须自洽，否则后面全是错的。"""

    def test_24_columns(self):
        from idcbox.domain.exporting.contract25 import CONTRACT_25

        assert len(CONTRACT_25) == 24, "有效列是 24（第 25 列是 phantom，表头为 None）"
        assert [s.index for s in CONTRACT_25] == list(range(24))

    def test_headers_match_source(self):
        """表头必须与源文件逐字相同 —— 用户靠它认列。"""
        from idcbox.domain.exporting.contract25 import CONTRACT_25
        from idcbox.infrastructure.xlsx.openpyxl_reader import V13_HEADERS

        assert tuple(s.header for s in CONTRACT_25) == V13_HEADERS

    def test_two_same_name_u_columns(self):
        """两列都叫「U位」，必须靠 index 而非 header 区分。"""
        from idcbox.domain.exporting.contract25 import CONTRACT_25

        names = [s.header for s in CONTRACT_25]
        assert names.count("U位") == 2
        assert names.index("U位") == 4
        assert [s.index for s in CONTRACT_25 if s.header == "U位"] == [4, 10]

    def test_formula_columns(self):
        """哪几列是公式列、哪几列必须原样回写，是契约的硬约束。

        源表实测：只有 P(机柜数量)/R(柜间线缆)/S(线缆2)/T(线缆长度1) 四列是活公式；
        N/O/Q/U 全是人手填的字面值，契约里归 `value`/`raw`，绝不能写成公式。
        """
        from idcbox.domain.exporting.contract25 import CONTRACT_25

        by_field = {s.field: s for s in CONTRACT_25}
        for f in ("p_rack_count", "r_inter_rack_m", "s_total_m", "t_ladder_m"):
            assert by_field[f].kind == "formula", f"{f} 应是公式列"
        # 人工列绝不能是公式（否则会把人的判断覆盖掉）
        for f in ("n_cross_rack", "o_gap_rack", "q_in_rack_m"):
            assert by_field[f].kind == "value", f"{f} 是人工列，不能标成 formula"
        # raw 列必须原样回写
        for f in ("local_u", "peer_u", "u_approved_m", "local_module", "peer_module", "remark"):
            assert by_field[f].kind == "raw", f"{f} 必须原样回写"

    def test_t_formula_text_matches_source(self):
        """T 列公式文本必须与源文件逐字一致（含 (6,7) 缝隙）。"""
        from idcbox.domain.exporting.contract25 import t_formula_text

        expected = (
            "=IFS(S2<=5,5,S2<=6,7,S2=7,7,S2<=8.5,10,S2<=13,15,S2<=17.5,20,"
            "S2<=22,25,S2<=27,30,S2<=32,35,S2<=37,40)"
        )
        assert t_formula_text(2) == expected
        # 行号必须跟着走
        assert t_formula_text(100).startswith("=IFS(S100<=5,5")
        assert "S100<=37,40)" in t_formula_text(100)

    def test_column_widths(self):
        from idcbox.domain.exporting.contract25 import V13_COL_WIDTHS

        assert len(V13_COL_WIDTHS) == 24
        assert V13_COL_WIDTHS[1] == 47.0, "第二列（设备名称）最宽"


class TestRoundTripFidelity:
    """核心用例：导出 → 重新读回 → 与源文件逐格比。"""

    def test_24_columns_zero_diff(self, rows, tmp_path):
        from idcbox.domain.exporting.contract25 import CONTRACT_25
        from idcbox.infrastructure.xlsx.exporter import write_xlsx
        from idcbox.infrastructure.xlsx.openpyxl_reader import read_v13_cabling

        from paths import source_path

        sheet = read_v13_cabling(source_path(), "布线表")
        export_rows = _build_export_rows(sheet)
        out = write_xlsx(export_rows, tmp_path / "roundtrip.xlsx", mode="value", sheet_name="布线表")

        # 读回导出文件（data_only=False 才看得到公式文本；value 模式下全是值）
        import openpyxl

        wb = openpyxl.load_workbook(out, data_only=False)
        ws = wb["布线表"]

        # 表头
        for spec in CONTRACT_25:
            got = ws.cell(row=1, column=spec.index + 1).value
            assert got == spec.header, f"表头第 {spec.index} 列不符：{got!r} != {spec.header!r}"

        # 正文逐格
        diffs: list[tuple[int, int, Any, Any]] = []
        for r_i, raw_row in enumerate(sheet.rows):
            excel_row = r_i + 2
            for spec in CONTRACT_25:
                src_v = _norm_cell(
                    raw_row.values[spec.index] if spec.index < len(raw_row.values) else None
                )
                got_v = _norm_cell(ws.cell(row=excel_row, column=spec.index + 1).value)
                if src_v != got_v:
                    diffs.append((excel_row, spec.index, src_v, got_v))
                    if len(diffs) > 30:
                        break
            if len(diffs) > 30:
                break

        assert not diffs, (
            f"逐格比较出现 {len(diffs)}+ 处差异，前几处："
            + "; ".join(f"行{r}列{c}: {a!r} != {b!r}" for r, c, a, b in diffs[:10])
        )

    def test_sentinel_preserved(self, tmp_path):
        """sentinel 文本必须原样回写：`待定` 不能变成空、`一` 不能变成 0。"""
        import openpyxl

        from idcbox.domain.exporting.contract25 import CONTRACT_25
        from idcbox.infrastructure.xlsx.exporter import ExportRow, write_xlsx
        from idcbox.infrastructure.xlsx.openpyxl_reader import read_v13_cabling

        from paths import source_path

        sheet = read_v13_cabling(source_path(), "布线表")
        # 找出含 sentinel 的行
        sents = {"待定", "一", "设备自带"}
        target: list = []
        for raw_row in sheet.rows:
            vals = {str(v).strip() for v in raw_row.values if v is not None}
            if vals & sents:
                target.append(raw_row)
        assert len(target) >= 2, "源数据里应有 sentinel 行"

        export_rows = []
        for raw_row in target:
            values = {s.field: raw_row.values[s.index] for s in CONTRACT_25 if s.field}
            export_rows.append(
                ExportRow(
                    source_row_no=raw_row.row_no,
                    values=values,
                    raw={s.index: raw_row.values[s.index] for s in CONTRACT_25},
                )
            )
        out = write_xlsx(export_rows, tmp_path / "sentinel.xlsx", mode="value")
        ws = openpyxl.load_workbook(out, data_only=False)["布线表"]

        for i, raw_row in enumerate(target, start=2):
            for spec in CONTRACT_25:
                src_v = raw_row.values[spec.index]
                if isinstance(src_v, str) and src_v.strip() in sents:
                    got = ws.cell(row=i, column=spec.index + 1).value
                    assert got == src_v, (
                        f"sentinel 被改写：行{raw_row.row_no}列{spec.index} "
                        f"{src_v!r} → {got!r}"
                    )

    def test_formula_mode_writes_live_formula(self, tmp_path):
        """formula 模式：P/R/S/T 四列必须是活公式，且能被 Excel 求值。

        注意：源表 P/R/S/T 是公式，我们用 `data_only=True` 读的是**缓存值**。
        导出 formula 模式时不能把缓存值当公式写进去（那会变成静态数）。
        """
        import openpyxl

        from idcbox.domain.exporting.contract25 import CONTRACT_25, t_formula_text
        from idcbox.infrastructure.xlsx.exporter import ExportRow, write_xlsx

        # 构造一行可算数据的记录
        values = {s.field: None for s in CONTRACT_25 if s.field}
        values.update(
            {
                "model_local": "测试交换机",
                "dev_local": "测试交换机-1",
                "ch_local": 1, "rack_local": 1, "port_local": "25GE1/0/1",
                "model_peer": "测试服务器",
                "dev_peer": "测试服务器-1",
                "ch_peer": 2, "rack_peer": 2, "port_peer": "25G1",
                "cable_type": "LC-LC",
                "n_cross": 12, "o_gap": 5, "p_count": 17,
                "q_in_rack": 5, "r_inter": 12.24, "s_total": 17.24,
                "t_ladder": 20, "u_approved": 20,
            }
        )
        row = ExportRow(source_row_no=2, values=values, raw=None)
        out = write_xlsx([row], tmp_path / "formula.xlsx", mode="formula")

        ws = openpyxl.load_workbook(out, data_only=False)["布线表"]
        # P/R/S/T 必须是公式字符串
        assert str(ws.cell(row=2, column=16).value) == "=N2+O2"
        assert str(ws.cell(row=2, column=18).value) == "=P2*0.6*120%"
        assert str(ws.cell(row=2, column=19).value) == "=R2+Q2"
        assert str(ws.cell(row=2, column=20).value) == t_formula_text(2)


class TestExportIdempotent:
    """导出两次必须完全相同（03 号 §9A.6 的幂等要求）。"""

    def test_write_twice_identical(self, tmp_path):
        import hashlib

        from idcbox.infrastructure.xlsx.exporter import write_xlsx
        from idcbox.infrastructure.xlsx.openpyxl_reader import read_v13_cabling

        from paths import source_path

        sheet = read_v13_cabling(source_path(), "布线表")
        rows = _build_export_rows(sheet[:50]) if hasattr(sheet, "__getitem__") else _build_export_rows(
            type(sheet)(headers=sheet.headers, rows=sheet.rows[:50])
        )

        a = write_xlsx(rows, tmp_path / "a.xlsx", mode="value")
        b = write_xlsx(rows, tmp_path / "b.xlsx", mode="value")
        # 注：xlsx 内含时间戳，字节级不一定相同，但单元格内容必须相同
        import openpyxl

        wa = openpyxl.load_workbook(a, data_only=False)["布线表"]
        wb_ = openpyxl.load_workbook(b, data_only=False)["布线表"]
        assert wa.max_row == wb_.max_row
        assert wa.max_column == wb_.max_column
        for r in range(1, wa.max_row + 1):
            for c in range(1, wa.max_column + 1):
                assert wa.cell(row=r, column=c).value == wb_.cell(row=r, column=c).value, (
                    f"两次导出在 r{r}c{c} 不同"
                )
