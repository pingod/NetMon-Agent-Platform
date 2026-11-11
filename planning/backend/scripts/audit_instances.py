"""从连线表抽取「设备实例」并做体检（D3/D4/D5/D6/D7/D12/R30）。

## 为什么必须先抽实例

连线表是**以连线为行**的，同一台设备会出现在多行（一台 44 口交换机有 44 行）。
直接按行做「同机柜 U 位重叠」会把 **833 对**噪音当成冲突；按**唯一物理实例**才是真冲突。

实例身份 = `(通道, 机柜, U位原文, 型号归一化)`。四元组任一不同即为两台设备。

跑法：env -u PYTHONHOME -u PYTHONPATH .venv/bin/python scripts/audit_instances.py
"""
from __future__ import annotations

import sys
import warnings
from collections import Counter, defaultdict
from dataclasses import dataclass, field
from pathlib import Path

warnings.filterwarnings("ignore")
BACKEND = Path(__file__).resolve().parent.parent
ROOT = BACKEND.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.geometry.floor import FloorCell, parse_floor
from idcbox.domain.geometry.layout import RackRef, build_snapshot
from idcbox.domain.importing.normalize import norm_int, norm_name
from idcbox.infrastructure.xlsx.openpyxl_reader import read_v13_cabling

SRC = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"

COL = dict(local_model=0, local_dev=1, local_ch=2, local_rack=3, local_u=4, local_port=5,
           peer_model=6, peer_dev=7, peer_ch=8, peer_rack=9, peer_u=10, peer_port=11,
           cable=12, remark=23, local_mod=21, peer_mod=22)


def parse_u_span(v: object) -> tuple[int, int] | None:
    if v is None:
        return None
    s = str(v).strip().upper().replace("U", "").replace(" ", "")
    if not s:
        return None
    if "-" in s:
        parts = s.split("-")
        if len(parts) != 2:
            return None
        try:
            return (int(parts[0]), int(parts[1]))
        except ValueError:
            return None
    try:
        n = int(float(s))
    except ValueError:
        return None
    return (n, n)


@dataclass
class Instance:
    channel: int
    rack: int
    u_raw: str
    span: tuple[int, int]
    model_raw: str
    model: str
    names: set[str] = field(default_factory=set)
    ports: set[str] = field(default_factory=set)
    rows: list[int] = field(default_factory=list)

    @property
    def key(self) -> tuple:
        return (self.channel, self.rack, self.u_raw, self.model)

    @property
    def size(self) -> int:
        return self.span[1] - self.span[0] + 1


def extract_instances(rows) -> dict[tuple, Instance]:
    out: dict[tuple, Instance] = {}
    for r in rows:
        v = r.values
        for side in ("local", "peer"):
            ch = norm_int(v[COL[f"{side}_ch"]])
            rk = norm_int(v[COL[f"{side}_rack"]])
            u_raw = v[COL[f"{side}_u"]]
            model_raw = v[COL[f"{side}_model"]]
            span = parse_u_span(u_raw)
            if ch is None or rk is None or span is None:
                continue
            model = norm_name(model_raw)
            if not model:
                continue
            u_norm = str(u_raw).strip().upper()
            key = (ch, rk, u_norm, model)
            inst = out.get(key)
            if inst is None:
                inst = Instance(ch, rk, u_norm, span, str(model_raw).strip(), model)
                out[key] = inst
            inst.names.add(norm_name(v[COL[f"{side}_dev"]]))
            port = v[COL[f"{side}_port"]]
            if port:
                inst.ports.add(str(port).strip())
            inst.rows.append(r.row_no)
    return out


def main() -> None:
    sheet = read_v13_cabling(SRC)
    rows = sheet.rows
    insts = extract_instances(rows)
    print(f"数据行 {len(rows)}，唯一设备实例 {len(insts)} 台")

    # ---------- D3 U 位区间重叠 ----------
    by_rack: dict[tuple[int, int], list[Instance]] = defaultdict(list)
    for i in insts.values():
        by_rack[(i.channel, i.rack)].append(i)
    # 原样口径：型号名**不做空格归一**，同型号变体会被当成两台设备（这才叫"原样"）
    raw_insts: dict[tuple, Instance] = {}
    for r in rows:
        v = r.values
        for side in ("local", "peer"):
            ch = norm_int(v[COL[f"{side}_ch"]])
            rk = norm_int(v[COL[f"{side}_rack"]])
            u_raw = v[COL[f"{side}_u"]]
            model_raw = v[COL[f"{side}_model"]]
            span = parse_u_span(u_raw)
            if ch is None or rk is None or span is None or not model_raw:
                continue
            u_norm = str(u_raw).strip().upper()
            mraw = str(model_raw).strip()
            key = (ch, rk, u_norm, mraw)
            inst = raw_insts.get(key)
            if inst is None:
                inst = Instance(ch, rk, u_norm, span, mraw, norm_name(mraw))
                raw_insts[key] = inst
            inst.names.add(norm_name(v[COL[f"{side}_dev"]]))
            port = v[COL[f"{side}_port"]]
            if port:
                inst.ports.add(str(port).strip())
            inst.rows.append(r.row_no)
    by_rack_raw: dict[tuple[int, int], list[Instance]] = defaultdict(list)
    for i in raw_insts.values():
        by_rack_raw[(i.channel, i.rack)].append(i)
    print(f"原样口径设备实例 {len(raw_insts)} 台 / 归一化口径 {len(insts)} 台")

    def overlaps(groups) -> list[tuple]:
        out = []
        for key, items in groups.items():
            items = sorted(items, key=lambda x: (x.span[0], x.span[1]))
            for a in range(len(items)):
                for b in range(a + 1, len(items)):
                    x, y = items[a], items[b]
                    if x.span[0] <= y.span[1] and y.span[0] <= x.span[1]:
                        out.append((key, x, y))
        return out

    ov_norm = overlaps(by_rack)
    ov_raw = overlaps(by_rack_raw)
    print(f"\nD3 U位区间重叠（原样口径，型号名不归一）: {len(ov_raw)} 对")
    print(f"D3 U位区间重叠（归一化口径）: {len(ov_norm)} 对")
    for key, x, y in ov_norm:
        kind = "名称变体（假冲突）" if x.model == y.model else "真撞车"
        print(f"  ch{key[0]} rack{key[1]}: {x.model_raw}@{x.span} vs {y.model_raw}@{y.span} ← {kind}")

    # ---------- D4 同一台设备（实例名相同）被记到多个 U 位 ----------
    # 注意：同型号占多个 U 位是**正常**的（一个柜里 8 台同型号计算节点），
    #       真正的缺陷信号是**同一个实例名出现在同一机柜的多个 U 位**。
    name_spans: dict[tuple, set] = defaultdict(set)
    for i in insts.values():
        for nm in i.names:
            if nm:
                name_spans[(i.channel, i.rack, nm)].add(i.span)
    multi = {k: v for k, v in name_spans.items() if len(v) > 1}
    print(f"\nD4 同一实例名在同机柜占多个 U 位（真缺陷）: {len(multi)} 处")
    for k, spans in list(multi.items())[:30]:
        print(f"  ch{k[0]} rack{k[1]} {k[2]} → {sorted(spans)}")
    # 同型号多少 U 位（仅统计，作参考不是缺陷）
    model_span_cnt: dict[tuple, int] = defaultdict(int)
    for i in insts.values():
        model_span_cnt[(i.channel, i.rack, i.model)] += 1
    print(f"D4 参考：同型号在同机柜有多台设备的组合 {sum(1 for v in model_span_cnt.values() if v > 1)} 组（正常现象）")

    # ---------- D5 名称变体 ----------
    name_holder: dict[str, set] = defaultdict(set)
    for r in rows:
        v = r.values
        for idx in (0, 6):
            m = v[idx]
            if m and str(m).strip():
                name_holder[norm_name(m)].add(str(m))
    variants = {k: v for k, v in name_holder.items() if len(v) > 1}
    print(f"\nD5 名称空格变体: {len(variants)} 组")
    for k, v in variants.items():
        print(f"  '{k}' ← {sorted(v)}")

    # 实例名缺失
    no_name = [i for i in insts.values() if i.names == {""}]
    print(f"D5 实例名为空（只有型号名）: {len(no_name)} 台")
    for i in no_name[:10]:
        print(f"  ch{i.channel} rack{i.rack} {i.u_raw} {i.model}")

    # ---------- D6 端口重复占用（实例粒度）----------
    port_use: dict[tuple, list[int]] = defaultdict(list)
    for r in rows:
        v = r.values
        for side in ("local", "peer"):
            ch = norm_int(v[COL[f"{side}_ch"]])
            rk = norm_int(v[COL[f"{side}_rack"]])
            u_norm = str(v[COL[f"{side}_u"]]).strip().upper()
            model = norm_name(v[COL[f"{side}_model"]])
            port = v[COL[f"{side}_port"]]
            if ch is None or rk is None or not model or not port:
                continue
            port_use[(ch, rk, u_norm, model, str(port).strip())].append(r.row_no)
    dups = {k: v for k, v in port_use.items() if len(v) > 1}
    print(f"\nD6 端口被多行占用: {len(dups)} 个")
    for k, v in list(dups.items())[:15]:
        print(f"  ch{k[0]} rack{k[1]} u{k[2]} {k[3]} {k[4]} → rows {v}")

    # ---------- D7 孤儿 ----------
    fr = parse_floor([FloorCell(c.row, c.col, c.value) for c in []] or [])
    from idcbox.infrastructure.xlsx.openpyxl_reader import (
        FLOOR_SCAN_COLS,
        FLOOR_SCAN_ROWS,
        read_floor_cells,
    )
    cells = read_floor_cells(SRC, row_range=FLOOR_SCAN_ROWS, col_range=FLOOR_SCAN_COLS)
    floor = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
    snap = build_snapshot(floor.racks, version="v13")
    orphan = Counter()
    for i in insts.values():
        if snap.pos(RackRef(channel_no=i.channel, seq_no=i.rack)) is None:
            orphan[f"ch{i.channel}/rack{i.rack}"] += 1
    print(f"\nD7 设备位于平面图未标注的机柜: {dict(orphan) or '无'}")

    # ---------- R30 模块 / 批次 ----------
    remark_vals = Counter()
    for r in rows:
        rm = r.values[COL["remark"]]
        remark_vals[str(rm).strip() if rm is not None else "<None>"] += 1
    print(f"\n备注1 取值分布: {dict(remark_vals)}")

    rows_mod_no_batch, rows_batch_no_mod = [], []
    for r in rows:
        v = r.values
        has_mod = any(v[i] is not None and str(v[i]).strip() for i in (21, 22))
        has_batch = v[23] is not None and str(v[23]).strip() != ""
        if has_mod and not has_batch:
            rows_mod_no_batch.append(r.row_no)
        elif has_batch and not has_mod:
            rows_batch_no_mod.append(r.row_no)
    print(f"R30 模块有值但无批次: {len(rows_mod_no_batch)} 行")
    print(f"R30 批次有值但无模块: {len(rows_batch_no_mod)} 行 {rows_batch_no_mod}")

    # 模块值分布
    mod_vals = Counter()
    for r in rows:
        for i in (21, 22):
            x = r.values[i]
            if x is not None and str(x).strip():
                mod_vals[str(x).strip()] += 1
    print(f"模块值分布: {dict(mod_vals)}")


if __name__ == "__main__":
    main()
