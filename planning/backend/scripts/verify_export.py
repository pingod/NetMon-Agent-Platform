"""导出保真验证：25 列逐格 diff（AC-1 / AC-3 的唯一判据）。

## 判据

**原值回写模式下，导出文件与源文件的 24 列 × 5127 行必须逐格相同。**
这不是「看起来差不多」，是 `cell.value == source.value` 的严格相等；
数值按 `excel15` 口径比较（Excel 15 位有效数字），文本严格相等。

## 为什么这个脚本是最重要的验收装置

产品的第一条价值主张是「替代 Excel 但不改变交付物形态」。
如果导出的文件与用户手里的 Excel 有任何一格不同，用户就得多做一次人工核对 ——
那产品就白做了。所以这个 diff 必须是**零容忍**的。

用法：
    python scripts/verify_export.py            # 逐格比较并打印报告
    python scripts/verify_export.py --limit 200  # 只比前 200 行（调试用）
"""

from __future__ import annotations

import argparse
import sys
import warnings
from pathlib import Path
from typing import Any

import openpyxl

warnings.filterwarnings("ignore")

BACKEND = Path(__file__).resolve().parent.parent
ROOT = BACKEND.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.cable_length.numeric import excel15  # noqa: E402
from idcbox.domain.exporting.contract25 import CONTRACT_25, HEADER_ROW  # noqa: E402
from idcbox.infrastructure.xlsx.exporter import ExportRow, write_xlsx  # noqa: E402
from idcbox.infrastructure.xlsx.openpyxl_reader import read_v13_cabling  # noqa: E402

SRC = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"


def _norm_cell(v: Any) -> Any:
    """单元格归一：数值走 excel15，文本严格，空统一成 None。"""
    if v is None:
        return None
    if isinstance(v, bool):
        return v
    if isinstance(v, (int, float)):
        x = excel15(float(v))
        return int(x) if float(x).is_integer() else x
    s = str(v)
    return s if s.strip() else None


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--limit", type=int, default=0, help="只比前 N 行（0 = 全部）")
    ap.add_argument("--out", default=str(BACKEND / ".out" / "export_roundtrip.xlsx"))
    args = ap.parse_args()

    sheet = read_v13_cabling(SRC, "布线表")
    rows: list[ExportRow] = []
    for raw_row in sheet.rows:
        values: dict[str, Any] = {}
        raw: dict[int, Any] = {}
        for spec in CONTRACT_25:
            v = raw_row.values[spec.index] if spec.index < len(raw_row.values) else None
            raw[spec.index] = v
            if spec.field:
                values[spec.field] = v
        rows.append(ExportRow(source_row_no=raw_row.row_no, values=values, raw=raw))

    if args.limit:
        rows = rows[: args.limit]
    print(f"待导出：{len(rows)} 行 × {len(CONTRACT_25)} 列")

    out_path = write_xlsx(rows, args.out, mode="value", sheet_name="布线表")
    print(f"已写出：{out_path.relative_to(ROOT)}")

    # ---- 逐格比较 ----
    wb_src = openpyxl.load_workbook(SRC, data_only=True)
    wb_out = openpyxl.load_workbook(out_path, data_only=True)
    ws_src = wb_src["布线表"]
    ws_out = wb_out["布线表"]

    diffs: list[dict[str, Any]] = []
    checked = 0

    # 表头逐格
    for spec in CONTRACT_25:
        a = _norm_cell(ws_src.cell(row=HEADER_ROW, column=spec.index + 1).value)
        b = _norm_cell(ws_out.cell(row=HEADER_ROW, column=spec.index + 1).value)
        checked += 1
        if a != b:
            diffs.append(
                {"kind": "header", "col": spec.header, "src": a, "out": b}
            )

    # 数据逐格
    for i, row in enumerate(rows):
        r_src = row.source_row_no
        r_out = HEADER_ROW + 1 + i
        if r_src is None:
            continue
        for spec in CONTRACT_25:
            a = _norm_cell(ws_src.cell(row=r_src, column=spec.index + 1).value)
            b = _norm_cell(ws_out.cell(row=r_out, column=spec.index + 1).value)
            checked += 1
            if a != b:
                diffs.append(
                    {
                        "kind": "cell",
                        "src_row": r_src,
                        "col_index": spec.index,
                        "col": spec.header,
                        "src": a,
                        "out": b,
                    }
                )

    print(f"\n逐格比较：{checked} 格，差异 {len(diffs)} 格")
    if diffs:
        print("\n差异样例（前 20）：")
        for d in diffs[:20]:
            print(" ", d)
        # 按列汇总
        by_col: dict[str, int] = {}
        for d in diffs:
            by_col[str(d["col"])] = by_col.get(str(d["col"]), 0) + 1
        print("\n按列汇总：", dict(sorted(by_col.items(), key=lambda kv: -kv[1])))
        print("\n❌ 导出保真验证失败")
        return 1

    print("\n✅ 导出保真验证通过：24 列逐格完全一致（含 2 个同名 U位 列、sentinel 文本）")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
