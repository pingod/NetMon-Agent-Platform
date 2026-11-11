"""端到端体检：读 V13 → 解析几何 → 跑规则 → 出报告。

跑法：
    env -u PYTHONHOME -u PYTHONPATH .venv/bin/python scripts/run_audit.py
    ... --out reports/audit.md
"""
from __future__ import annotations

import argparse
import sys
import warnings
from pathlib import Path

warnings.filterwarnings("ignore")
BACKEND = Path(__file__).resolve().parent.parent
ROOT = BACKEND.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.geometry.floor import FloorCell, derive_cmax_by_room, parse_floor
from idcbox.domain.geometry.layout import build_snapshot
from idcbox.domain.validation.report import render_markdown, render_summary_text
from idcbox.domain.validation.rules import AuditContext, run_audit
from idcbox.infrastructure.xlsx.openpyxl_reader import (
    FLOOR_SCAN_COLS,
    FLOOR_SCAN_ROWS,
    read_floor_cells,
    read_v13_cabling,
)

SRC = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"

COL = {
    "model_local": 0, "dev_local": 1, "ch_local": 2, "rack_local": 3,
    "u_local": 4, "port_local": 5,
    "model_peer": 6, "dev_peer": 7, "ch_peer": 8, "rack_peer": 9,
    "u_peer": 10, "port_peer": 11,
    "cable_type": 12,
    "n_cross": 13, "o_gap": 14, "p_count": 15, "q_in_rack": 16,
    "r_inter": 17, "s_total": 18, "t_ladder": 19, "u_approved": 20,
    "mod_local": 21, "mod_peer": 22, "remark": 23,
}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", default=str(SRC))
    ap.add_argument("--out", default="")
    ap.add_argument("--project", default="甘肃信息港 V13")
    args = ap.parse_args()

    src = Path(args.src)
    cells = read_floor_cells(src, row_range=FLOOR_SCAN_ROWS, col_range=FLOOR_SCAN_COLS)
    floor = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
    cmax = derive_cmax_by_room(floor.racks)
    snap = build_snapshot(floor.racks, version="v13", cmax_override=cmax)
    print(f"几何：{len(floor.racks)} 个机柜位，CMAX={cmax}")

    sheet = read_v13_cabling(src)
    rows = [r.values for r in sheet.rows]
    row_nos = [r.row_no for r in sheet.rows]
    print(f"数据：{len(rows)} 行")

    ctx = AuditContext(
        rows=rows,
        row_nos=row_nos,
        col=COL,
        sheet_name="布线表",
        geometry=snap,
        project_label=args.project,
    )
    report = run_audit(ctx)
    print()
    print(render_summary_text(report))

    md = render_markdown(report)
    if args.out:
        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(md, encoding="utf-8")
        print(f"\n报告已写出：{out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
