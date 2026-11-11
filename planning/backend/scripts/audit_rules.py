"""体检规则的实测校验：先把每个规则的命中数算出来，再定门控。

跑法：env -u PYTHONHOME -u PYTHONPATH .venv/bin/python scripts/audit_rules.py
"""
from __future__ import annotations

import sys
import warnings
from collections import Counter, defaultdict
from pathlib import Path

warnings.filterwarnings("ignore")
BACKEND = Path(__file__).resolve().parent.parent
ROOT = BACKEND.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.geometry.floor import FloorCell, parse_floor
from idcbox.domain.geometry.layout import build_snapshot
from idcbox.domain.importing.normalize import norm_int, norm_name, parse_length
from idcbox.domain.ports.naming import parse_port_name
from idcbox.infrastructure.xlsx.openpyxl_reader import (
    FLOOR_SCAN_COLS,
    FLOOR_SCAN_ROWS,
    read_floor_cells,
    read_v13_cabling,
)

SRC = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"

cells = read_floor_cells(SRC, row_range=FLOOR_SCAN_ROWS, col_range=FLOOR_SCAN_COLS)
floor = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
snap = build_snapshot(floor.racks, version="v13")
sheet = read_v13_cabling(SRC)
rows = sheet.rows
print(f"数据行 {len(rows)}")

def parse_u_span(v):
    if v is None:
        return None
    s = str(v).strip().upper().replace("U", "")
    if not s:
        return None
    if "-" in s:
        parts = s.split("-")
        try:
            lo, hi = int(parts[0]), int(parts[1])
        except ValueError:
            return None
        return (lo, hi)
    try:
        n = int(float(s))
    except ValueError:
        return None
    return (n, n)

# ---- D3 U 位冲突：同机柜内 U 区间相交 ----
by_rack: dict[tuple[int, int], list[tuple[int, str, int, int]]] = defaultdict(list)
for r in rows:
    v = r.values
    ch, rk = norm_int(v[2]), norm_int(v[3])
    if ch is None or rk is None:
        continue
    span = parse_u_span(v[4])
    if span is None:
        continue
    by_rack[(ch, rk)].append((r.row_no, norm_name(v[0]), span[0], span[1]))

def find_overlaps(by_rack, *, dedupe: bool):
    pairs = []
    for key, items in by_rack.items():
        # 同一行的本端/对端可能重复出现，按 (型号,U) 去重
        uniq = []
        seen = set()
        for row_no, model, s, e in items:
            sig = (model, s, e) if dedupe else (row_no, model, s, e)
            if sig in seen:
                continue
            seen.add(sig)
            uniq.append((row_no, model, s, e))
        uniq.sort(key=lambda t: (t[2], t[3]))
        for i in range(len(uniq)):
            for j in range(i + 1, len(uniq)):
                a, b = uniq[i], uniq[j]
                if a[2] == b[2] and a[1] == b[1]:
                    # 同型号同 U 起点 = 同一设备被两行引用，不是冲突
                    continue
                if a[2] <= b[3] and b[2] <= a[3]:
                    pairs.append((key, a, b))
    return pairs

ov_raw = find_overlaps(by_rack, dedupe=False)
ov_norm = find_overlaps(by_rack, dedupe=True)
print(f"\nD3 U位区间重叠（原样口径）: {len(ov_raw)} 对")
print(f"D3 U位区间重叠（归一化去重）: {len(ov_norm)} 对")
for key, a, b in ov_norm[:20]:
    print(f"  ch{key[0]} rack{key[1]}: {a[1]}@{a[2]}-{a[3]}(r{a[0]}) vs {b[1]}@{b[2]}-{b[3]}(r{b[0]})")

# ---- D4 同型号在同机柜多 U 位 ----
multi = []
for key, items in by_rack.items():
    by_model: dict[str, set] = defaultdict(set)
    for row_no, model, s, e in items:
        by_model[model].add((s, e))
    for model, spans in by_model.items():
        if len(spans) > 1:
            multi.append((key, model, sorted(spans)))
print(f"\nD4 同型号同机柜多 U 位: {len(multi)} 处")
for key, model, spans in multi[:20]:
    print(f"  ch{key[0]} rack{key[1]}: {model} → {spans}")

# ---- D5 名称空格变体 ----
all_models = Counter()
for r in rows:
    for idx in (0, 6):
        m = r.values[idx]
        if m and str(m).strip():
            all_models[str(m)] += 1
norm_map: dict[str, set] = defaultdict(set)
for m in all_models:
    norm_map[norm_name(m)].add(m)
variants = {k: v for k, v in norm_map.items() if len(v) > 1}
print(f"\nD5 名称空格变体组: {len(variants)} 组")
for k, v in list(variants.items())[:20]:
    print(f"  '{k}' ← {sorted(v)}")

# ---- D6 端口重复占用 ----
port_use: dict[tuple, list[int]] = defaultdict(list)
for r in rows:
    v = r.values
    ch, rk = norm_int(v[2]), norm_int(v[3])
    dev, port = norm_name(v[1]) or norm_name(v[0]), str(v[5]).strip() if v[5] else ""
    if ch and rk and port:
        port_use[(ch, rk, dev, port)].append(r.row_no)
    ch2, rk2 = norm_int(v[8]), norm_int(v[9])
    dev2, port2 = norm_name(v[7]) or norm_name(v[6]), str(v[11]).strip() if v[11] else ""
    if ch2 and rk2 and port2:
        port_use[(ch2, rk2, dev2, port2)].append(r.row_no)
dups = {k: v for k, v in port_use.items() if len(v) > 1}
print(f"\nD6 端口被多行占用: {len(dups)} 个端口")
for k, v in list(dups.items())[:15]:
    print(f"  ch{k[0]} rack{k[1]} {k[2]} {k[3]} → rows {v}")

# ---- D6 双向重复（A→B 与 B→A 同时存在）----
sig = {}
rev = []
for r in rows:
    v = r.values
    a = (norm_int(v[2]), norm_int(v[3]), str(v[5]).strip())
    b = (norm_int(v[8]), norm_int(v[9]), str(v[11]).strip())
    k = (a, b)
    rk = (b, a)
    if rk in sig and a != b:
        rev.append((sig[rk], r.row_no, a, b))
    sig.setdefault(k, r.row_no)
print(f"\nD6 反向重复连线: {len(rev)} 对")
for x in rev[:10]:
    print(f"  r{x[0]} 与 r{x[1]}: {x[2]} ↔ {x[3]}")

# ---- D7 孤儿：机柜不在几何快照 ----
orphan = Counter()
for r in rows:
    v = r.values
    for ci, ri in ((2, 3), (8, 9)):
        ch, rk = norm_int(v[ci]), norm_int(v[ri])
        if ch is None or rk is None:
            orphan["通道或机柜为空"] += 1
            continue
        from idcbox.domain.geometry.layout import RackRef
        if snap.pos(RackRef(channel_no=ch, seq_no=rk)) is None:
            orphan[f"ch{ch}/rack{rk} 不在平面图"] += 1
print(f"\nD7 孤儿机柜: {dict(orphan)}")

# ---- D12 sentinel / 非数值 ----
bad = {"O非数值": [], "P非数值": [], "T非数值": [], "U非数值": []}
for r in rows:
    v = r.values
    for name, idx in (("O非数值", 14), ("P非数值", 15), ("T非数值", 19), ("U非数值", 20)):
        x = v[idx]
        if isinstance(x, str) and x.strip():
            try:
                float(x)
            except ValueError:
                bad[name].append((r.row_no, x))
print("\nD12 非数值单元格：")
for k, v in bad.items():
    print(f"  {k}: {len(v)} → {v[:6]}")

# ---- R30 模块有值但未标批次 ----
r30_a, r30_b = [], []
for r in rows:
    v = r.values
    has_mod = any(isinstance(v[i], str) and v[i].strip() for i in (21, 22))
    has_phase = isinstance(v[23], str) and v[23].strip() != ""
    if has_mod and not has_phase:
        r30_a.append(r.row_no)
    elif has_phase and not has_mod:
        r30_b.append(r.row_no)
print(f"\nR30 模块有值但无批次: {len(r30_a)} 行  {r30_a[:8]}")
print(f"R30 批次有值但无模块: {len(r30_b)} 行  {r30_b[:8]}")

# ---- T 有值 U 空（待核定，应 info）----
pending = []
for r in rows:
    v = r.values
    t, _ = parse_length(v[19])
    u, uraw = parse_length(v[20])
    if t is not None and u is None and uraw is None:
        pending.append(r.row_no)
print(f"\n待核定（T 有值 U 空，应为 info 不是 error）: {len(pending)} 行 {pending}")
