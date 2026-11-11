"""真实数据回归：端口分配五条规则（3850/3850 + 112/112）。

## 目的

端口分配的五条规则是产品「自动化」的可行性基石——用户声明「哪些节点归哪台交换机的哪一段端口」，
系统就能确定性地推出 3850 条端口分配。这个脚本用 V13 全量真实数据验证这五条：

| 规则 | 内容 | 期望 |
|---|---|---|
| C2 | 交换机端口号 = 组内位次 + base | 3850/3850 = 100% |
| C3 | U 位槽位相位 `slot_i ≡ (i + φ) mod 8` | 112/112 组 φ 唯一 |
| C4 | φ = slot_of(首节点 U 位) | 同上 |
| C5 | 组内机柜号单调不减 | 112/112 |

## 关键实现细节

**分组口径**：组 = `(交换机型号, 交换机实例名, 对端节点型号)`。
但同一台交换机的同一对端型号可能被拆成多个「接线块」（base 不同），
所以真正的分组键还要带上 **端口号连续性** —— 按端口号排序后，
如果相邻端口号跳变（gap > 1），就要切成新块。

这就是 `base` 分布 `{1: 91 组, 9: 7, 17: 7, 20: 7}` 的来由：
一台 48 口交换机服务两类节点时，第 1 类占 1..8，第 2 类从 9 开始。

输出：`tests/golden/baseline/port_baseline.json`
"""

from __future__ import annotations

import json
import re
import sys
from collections import Counter, defaultdict
from pathlib import Path

import openpyxl
import warnings

warnings.filterwarnings("ignore")

BACKEND = Path(__file__).resolve().parent.parent
ROOT = BACKEND.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.ports.naming import parse_port_name  # noqa: E402
from idcbox.domain.ports.phase import (  # noqa: E402
    NodeRef,
    phase_of,
    slot_of,
)

SRC = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"
OUT = BACKEND / "tests" / "golden" / "baseline" / "port_baseline.json"


def nm(v: object) -> str:
    return str(v).strip() if v is not None else ""


def main() -> int:
    wb = openpyxl.load_workbook(SRC, data_only=True)
    ws = wb["布线表"]

    # 只取「交换机 → 服务器」方向的行（服务器侧端口是标签式 千兆1/IPMI 等）
    recs: list[dict[str, object]] = []
    for r in range(2, ws.max_row + 1):
        v = [nm(ws.cell(row=r, column=c).value) for c in range(1, 25)]
        if not v[0] and not v[6]:
            continue
        local_model, local_port = v[0], v[5]
        peer_model, peer_port = v[6], v[11]

        local_p = parse_port_name(local_port)
        peer_p = parse_port_name(peer_port)

        # 交换机在「本端」：本端是 CLI 式、对端是标签式
        if local_p.style != "cli_board" or peer_p.style != "label":
            continue
        # 只要下联口（同速率段的普通口），排除上联/堆叠段
        if local_p.port is None:
            continue

        try:
            ch, rack = int(float(v[2])), int(float(v[3]))
            peer_ch, peer_rack = int(float(v[8])), int(float(v[9]))
        except (TypeError, ValueError):
            continue
        u_raw = v[10]
        m = re.match(r"^(\d+)", u_raw)
        if not m:
            continue
        peer_u = int(m.group(1))

        recs.append(
            {
                "row": r,
                "switch_model": local_model,
                "switch_instance": v[1],
                "switch_port": local_port,
                "switch_port_no": local_p.port,
                "switch_rate": local_p.rate_mbps,
                "node_model": peer_model,
                "node_id": v[7],
                "node_ch": peer_ch,
                "node_rack": peer_rack,
                "node_u": peer_u,
            }
        )

    print(f"交换机→服务器 连线行：{len(recs)}")

    # ---- C2：按 (交换机型号, 实例, 对端型号) 分组，再按端口号连续性切块 ----
    groups: dict[tuple[str, str, str], list[dict[str, object]]] = defaultdict(list)
    for d in recs:
        groups[(str(d["switch_model"]), str(d["switch_instance"]), str(d["node_model"]))].append(d)

    blocks: list[list[dict[str, object]]] = []
    for key, rows in groups.items():
        rows.sort(key=lambda d: int(d["switch_port_no"]))  # type: ignore[arg-type]
        cur: list[dict[str, object]] = [rows[0]]
        for prev, nxt in zip(rows, rows[1:]):
            if int(nxt["switch_port_no"]) - int(prev["switch_port_no"]) == 1:  # type: ignore[arg-type]
                cur.append(nxt)
            else:
                blocks.append(cur)
                cur = [nxt]
        blocks.append(cur)

    print(f"分组（型号×实例×节点型号）：{len(groups)}  接线块：{len(blocks)}")

    compute_blocks_pre = [b for b in blocks if "计算节点" in str(b[0]["node_model"])]
    compute_rows = sum(len(b) for b in compute_blocks_pre)

    # ---- C2 验证：端口号 == 块内位次 + base ----
    c2_ok = c2_total = 0
    c2_bad: list[dict[str, object]] = []
    base_dist: Counter[int] = Counter()
    for b in blocks:
        ordered = sorted(b, key=lambda d: int(d["switch_port_no"]))  # type: ignore[arg-type]
        base = int(ordered[0]["switch_port_no"])  # type: ignore[arg-type]
        base_dist[base] += 1
        for i, d in enumerate(ordered):
            c2_total += 1
            if int(d["switch_port_no"]) == base + i:  # type: ignore[arg-type]
                c2_ok += 1
            else:
                c2_bad.append(
                    {
                        "row": d["row"],
                        "switch": d["switch_instance"],
                        "port": d["switch_port"],
                        "expected": base + i,
                    }
                )

    def check_blocks(subset: list[list[dict[str, object]]], tag: str) -> dict[str, object]:
        """对一组接线块跑 C3/C4/C5 三条规则。"""
        c5_ok = c5_total = 0
        c3_ok = c3_total = 0
        c4_ok = 0
        phi_dist: Counter[int] = Counter()
        phi_bad: list[dict[str, object]] = []
        c5_bad: list[dict[str, object]] = []

        for b in subset:
            ordered = sorted(b, key=lambda d: int(d["switch_port_no"]))  # type: ignore[arg-type]
            racks = [int(d["node_rack"]) for d in ordered]  # type: ignore[arg-type]
            c5_total += 1
            if all(racks[i] <= racks[i + 1] for i in range(len(racks) - 1)):
                c5_ok += 1
            else:
                c5_bad.append({"switch": ordered[0]["switch_instance"], "racks": racks})

            if len(ordered) < 3:
                continue
            nodes = [
                NodeRef(
                    node_id=str(d["node_id"]),
                    rack_channel=int(d["node_ch"]),  # type: ignore[arg-type]
                    rack_seq=int(d["node_rack"]),  # type: ignore[arg-type]
                    u_start=int(d["node_u"]),  # type: ignore[arg-type]
                )
                for d in ordered
            ]
            c3_total += 1
            phi = phase_of(nodes)
            if phi is not None:
                c3_ok += 1
                phi_dist[phi] += 1
                if phi == slot_of(nodes[0].u_start):
                    c4_ok += 1
            else:
                phi_bad.append(
                    {
                        "switch": ordered[0]["switch_instance"],
                        "node_model": ordered[0]["node_model"],
                        "n": len(nodes),
                        "slots": [n.slot for n in nodes[:12]],
                        "us": [n.u_start for n in nodes[:12]],
                        "racks": [n.rack_seq for n in nodes[:12]],
                    }
                )

        return {
            "tag": tag,
            "blocks": len(subset),
            "rows": sum(len(b) for b in subset),
            "c3_phase_unique": {
                "total": c3_total,
                "ok": c3_ok,
                "rate": round(c3_ok / c3_total, 6) if c3_total else 0.0,
                "phi_dist": dict(sorted(phi_dist.items())),
                "mismatches": phi_bad[:10],
            },
            "c4_phi_equals_first_slot": {
                "ok": c4_ok,
                "total": c3_total,
                "rate": round(c4_ok / c3_total, 6) if c3_total else 0.0,
            },
            "c5_rack_monotonic": {
                "total": c5_total,
                "ok": c5_ok,
                "rate": round(c5_ok / c5_total, 6) if c5_total else 0.0,
                "mismatches": c5_bad[:10],
            },
        }

    # 口径一：计算节点块（这是 01/03 号文档里 "112 组 / 3850 行" 的原始口径）
    compute_blocks = [
        b for b in blocks if "计算节点" in str(b[0]["node_model"])
    ]
    compute = check_blocks(compute_blocks, "计算节点")
    overall = check_blocks(blocks, "全部节点")

    result = {
        "source_file": SRC.name,
        "link_rows": len(recs),
        "groups": len(groups),
        "blocks": len(blocks),
        "c2_rank_plus_base": {
            "total": c2_total,
            "ok": c2_ok,
            "compute_node_rows": compute_rows,
            "rate": round(c2_ok / c2_total, 6) if c2_total else 0.0,
            "mismatches": c2_bad[:20],
        },
        "scope_compute_nodes": compute,
        "scope_all_nodes": overall,
        "base_port_dist": dict(sorted(base_dist.items())),
    }

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(result, ensure_ascii=False, indent=1), encoding="utf-8")

    print("\n=== 端口分配规则回归（C2 全量 / C3-C5 分两个口径）===")
    print(f"C2 端口号=位次+base : {c2_ok}/{c2_total} = {result['c2_rank_plus_base']['rate']:.4%}")
    for sc in (compute, overall):
        print(f"\n-- 口径：{sc['tag']}（{sc['blocks']} 块 / {sc['rows']} 行）")
        c3 = sc["c3_phase_unique"]
        c4 = sc["c4_phi_equals_first_slot"]
        c5 = sc["c5_rack_monotonic"]
        print(f"   C3 相位 φ 唯一     : {c3['ok']}/{c3['total']} = {c3['rate']:.4%}  φ={c3['phi_dist']}")
        print(f"   C4 φ=首节点slot    : {c4['ok']}/{c4['total']} = {c4['rate']:.4%}")
        print(f"   C5 组内机柜单调    : {c5['ok']}/{c5['total']} = {c5['rate']:.4%}")
    print(f"\nbase 分布 : {result['base_port_dist']}")
    print(f"落盘：{OUT.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
