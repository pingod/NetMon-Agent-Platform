"""用 V13 真实数据校验域层：N / O / T 复现率、端口分配、经验表 LOO。

这是**验收的唯一判据来源**。脚本会：
1. 读 `机房平面图` → 几何快照
2. 读 `布线表` → 5127 行 → 域层 link
3. 只用「跨机柜/间隔机柜/机柜数量」列有数值的行（原表 636 行）比对 N/O
4. 用全部 636 行有 T 的行比对 T（ifs_compat 逐字）
5. 输出 JSON 基线到 tests/golden/baseline/

运行：cd backend && env -u PYTHONHOME -u PYTHONPATH .venv/bin/python -m scripts.build_baseline
"""

from __future__ import annotations

import json
import os
import sys
from collections import Counter
from pathlib import Path
from typing import Any

BACKEND = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.cable_length.formulas import (  # noqa: E402
    classify_o,
    n_cross_rack,
    o_gap_rack,
    p_rack_count,
    q_in_rack,
    r_inter_rack,
    s_total,
)
from idcbox.domain.cable_length.ladder import (  # noqa: E402
    CONTINUOUS,
    IFS_COMPAT,
    ladder_ceil_tiers,
    ladder_round,
    ladder_round_nearest,
)
from idcbox.domain.cable_length.numeric import excel15  # noqa: E402
from idcbox.domain.cable_length.rules import DEFAULT_RULE  # noqa: E402
from idcbox.domain.geometry.floor import (  # noqa: E402
    FloorCell,
    derive_cmax_by_room,
    parse_floor,
)
from idcbox.domain.geometry.layout import build_snapshot  # noqa: E402
from idcbox.domain.importing.normalize import norm_cable_type, norm_int  # noqa: E402
from idcbox.infrastructure.xlsx.openpyxl_reader import (  # noqa: E402
    read_floor_cells,
    read_v13_cabling,
)

V13 = Path(os.environ.get("V13_PATH", BACKEND.parent / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"))


def _num(v: object) -> float | int | None:
    if v is None or isinstance(v, bool):
        return None
    if isinstance(v, (int, float)):
        return int(v) if isinstance(v, float) and v.is_integer() else v
    return None


def main() -> int:
    print(f"读取：{V13}")
    cells = read_floor_cells(V13)
    fr = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
    cmax = derive_cmax_by_room(fr.racks)
    snap = build_snapshot(fr.racks, version="v13", cmax_override=cmax)
    print(f"几何：{len(fr.racks)} 个机柜位，CMAX={cmax}")
    assert len(fr.racks) == 362, f"机柜位置数应为 362，实际 {len(fr.racks)}"
    assert cmax == {"403": 38, "404": 18}, f"CMAX 应 38/18，实际 {cmax}"

    sheet = read_v13_cabling(V13)
    print(f"布线表：{len(sheet.rows)} 行数据")

    # ---- 统计各列有值情况 ------------------------------------------------
    has_p = has_t = has_u = 0
    n_cmp = o_cmp = t_cmp = 0
    n_ok = o_ok = t_ok = 0
    p_ok = r_ok = s_ok = 0
    o_by_class: dict[str, dict[str, int]] = {"A": Counter(), "B": Counter(), "C": Counter()}
    n_mismatch: list[dict[str, Any]] = []
    o_mismatch: list[dict[str, Any]] = []
    t_by_impl = {
        "ifs_compat": [0, 0],
        "continuous": [0, 0],
        "ceil_tiers": [0, 0],
        "round_nearest": [0, 0],
    }
    t_mismatch: list[dict[str, Any]] = []
    geometry_missing: list[int] = []
    u_dist: Counter = Counter()
    t_dist: Counter = Counter()
    tu_relation = Counter()

    for row in sheet.rows:
        v = row.values
        ch_a, rack_a = norm_int(v[2]), norm_int(v[3])
        ch_b, rack_b = norm_int(v[8]), norm_int(v[9])
        if ch_a is None or rack_a is None or ch_b is None or rack_b is None:
            geometry_missing.append(row.row_no)
            continue
        from idcbox.domain.geometry.layout import RackRef

        pos_a, iss_a = snap.resolve(RackRef(ch_a, rack_a))
        pos_b, iss_b = snap.resolve(RackRef(ch_b, rack_b))
        if pos_a is None or pos_b is None:
            geometry_missing.append(row.row_no)
            continue

        p_col = _num(v[15])
        t_col = _num(v[19])
        u_col = _num(v[20])
        if p_col is not None:
            has_p += 1
        if t_col is not None:
            has_t += 1
            t_dist[int(t_col)] += 1
        if u_col is not None:
            has_u += 1
            u_dist[int(u_col)] += 1

        cls = classify_o(pos_a, pos_b)
        n_pred = n_cross_rack(pos_a, pos_b, DEFAULT_RULE)
        o_pred, _ = o_gap_rack(pos_a, pos_b, DEFAULT_RULE, cls)
        cable_type = norm_cable_type(v[12])
        q_pred, _ = q_in_rack(cable_type, DEFAULT_RULE)

        # --- N 比对（原表 N 是手填字面值） ---
        n_src = _num(v[13])
        if n_src is not None:
            n_cmp += 1
            if n_src == n_pred:
                n_ok += 1
            elif len(n_mismatch) < 100:
                n_mismatch.append(
                    {
                        "row": row.row_no,
                        "ch_a": ch_a, "rack_a": rack_a, "side_a": pos_a.side, "row_a": pos_a.row,
                        "ch_b": ch_b, "rack_b": rack_b, "side_b": pos_b.side, "row_b": pos_b.row,
                        "src": n_src, "pred": n_pred, "cls": cls,
                    }
                )

        # --- O 比对 ---
        o_src = _num(v[14])
        if o_src is not None:
            o_cmp += 1
            o_by_class[cls]["total"] += 1
            if o_src == o_pred:
                o_ok += 1
                o_by_class[cls]["ok"] += 1
            elif len(o_mismatch) < 120:
                o_mismatch.append(
                    {
                        "row": row.row_no, "cls": cls,
                        "ch_a": ch_a, "rack_a": rack_a, "col_a": pos_a.col, "row_a": pos_a.row,
                        "ch_b": ch_b, "rack_b": rack_b, "col_b": pos_b.col, "row_b": pos_b.row,
                        "src": o_src, "pred": o_pred,
                    }
                )

        # --- P / R / S 复算（原表 P/R/S 是活公式） ---
        if n_src is not None and o_src is not None:
            if p_col is not None and p_col == p_rack_count(n_src, o_src):
                p_ok += 1
            r_src = _num(v[17])
            if r_src is not None and excel15(r_src) == r_inter_rack(p_rack_count(n_src, o_src), DEFAULT_RULE):
                r_ok += 1
            s_src = _num(v[18])
            q_src = _num(v[16])
            if s_src is not None and q_src is not None:
                if excel15(s_src) == s_total(r_inter_rack(p_rack_count(n_src, o_src), DEFAULT_RULE), float(q_src)):
                    s_ok += 1

        # --- T 四档实现比对 ---
        s_src = _num(v[18])
        if t_col is not None and s_src is not None:
            t_cmp += 1
            s_val = float(s_src)
            impls = {
                "ifs_compat": ladder_round(s_val, IFS_COMPAT),
                "continuous": ladder_round(s_val, CONTINUOUS),
                "ceil_tiers": ladder_ceil_tiers(s_val),
                "round_nearest": ladder_round_nearest(s_val),
            }
            all_match = True
            for name, pred in impls.items():
                t_by_impl[name][1] += 1
                if pred == int(t_col):
                    t_by_impl[name][0] += 1
                else:
                    all_match = False
            if impls["ifs_compat"] == int(t_col):
                t_ok += 1
            elif len(t_mismatch) < 60:
                t_mismatch.append(
                    {"row": row.row_no, "s": s_val, "src": int(t_col), **{k: v2 for k, v2 in impls.items()}}
                )

        # --- T → U 人工调整关系 ---
        if t_col is not None and u_col is not None:
            if int(t_col) == int(u_col):
                tu_relation["same"] += 1
            elif int(u_col) > int(t_col):
                tu_relation["up"] += 1
            else:
                tu_relation["down"] += 1

    print()
    print("=" * 72)
    print(f"N  复现：{n_ok}/{n_cmp} = {n_ok / n_cmp:.4f}   （原表 N 为手填字面值）")
    print(f"O  复现：{o_ok}/{o_cmp} = {o_ok / o_cmp:.4f}")
    for cls in ("A", "B", "C"):
        c = o_by_class[cls]
        tot = c.get("total", 0)
        ok = c.get("ok", 0)
        if tot:
            print(f"     {cls} 类：{ok}/{tot} = {ok / tot:.4f}")
    print(f"P  复算：{p_ok} 行一致（原表 =N+O）")
    print(f"R  复算：{r_ok} 行一致（excel15）")
    print(f"S  复算：{s_ok} 行一致（excel15）")
    print(f"T  复现：{t_ok}/{t_cmp} = {t_ok / t_cmp:.4f}")
    for name, (ok, tot) in t_by_impl.items():
        print(f"     {name:14s}: {ok}/{tot} = {ok / tot:.4f}  差 {tot - ok}")
    print(f"几何缺失：{len(geometry_missing)} 行 {geometry_missing[:10]}")
    print(f"T→U：{dict(tu_relation)}")
    print(f"T 分布：{dict(sorted(t_dist.items()))}")
    print(f"U 分布：{dict(sorted(u_dist.items()))}")
    print("=" * 72)

    baseline = {
        "source_file": str(V13.name),
        "geometry": {"rack_count": len(fr.racks), "cmax_by_room": cmax},
        "rows": {"total": len(sheet.rows), "with_p": has_p, "with_t": has_t, "with_u": has_u},
        "n": {"cmp": n_cmp, "ok": n_ok, "rate": round(n_ok / n_cmp, 6), "mismatches": n_mismatch},
        "o": {
            "cmp": o_cmp, "ok": o_ok, "rate": round(o_ok / o_cmp, 6),
            "by_class": {k: dict(v) for k, v in o_by_class.items()},
            "mismatches": o_mismatch,
        },
        "p": {"ok": p_ok},
        "r": {"ok": r_ok},
        "s": {"ok": s_ok},
        "t": {
            "cmp": t_cmp, "ok": t_ok, "rate": round(t_ok / t_cmp, 6),
            "by_impl": {k: {"ok": a, "cmp": b} for k, (a, b) in t_by_impl.items()},
            "mismatches": t_mismatch,
        },
        "tu_relation": dict(tu_relation),
        "dist": {"t": dict(sorted(t_dist.items())), "u": dict(sorted(u_dist.items()))},
        "geometry_missing_rows": geometry_missing,
    }
    out = BACKEND / "tests" / "golden" / "baseline" / "v13_baseline.json"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(baseline, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"基线已写入 {out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
