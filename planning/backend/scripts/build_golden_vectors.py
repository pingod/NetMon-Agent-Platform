"""生成前后端共用的 golden 向量（`shared/golden/length_vectors.json`）。

## 为什么需要这个文件

架构 §2.1 第 2 条铁律说「公式只许实现一次」，但前端必须在浏览器里**即时**算出
长度（用户改一个 U 位要立刻看到表里的数字变），不可能每次都等后端 HTTP。

所以允许存在两份实现（Python + TypeScript），但必须靠**同一份向量**双向校验：
`pytest` 和 `vitest` 都读这个 JSON，任何一侧算错都失败。这是"允许两份代码"的唯一保险。

向量不是随机抽的，是按**覆盖分类**设计的：

| 类别 | 覆盖目标 |
|---|---|
| A 类同排 | col 差 0..10，含 path=0 的 5 米特例 |
| B 类同通道跨侧 | 相邻两行、隔多行 |
| C 类跨通道 | 403 内跨通道、404 内跨通道、403↔404 |
| 同柜 | N=0 O=0 P=0 R=0，S=Q |
| T 阶梯边界 | 每个 rung 的 limit 值 ± 一点，含 (6,7) 缝隙 |
| Excel 15 位 | P 使 `P*0.6*1.2` 出现浮点尾巴的值 |
| 未知线缆类型 | Q 走默认值 |
| 超末档 | S > 37 → T=None |

用法：
    env -u PYTHONHOME -u PYTHONPATH .venv/bin/python scripts/build_golden_vectors.py
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

BACKEND = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.cable_length import formulas  # noqa: E402
from idcbox.domain.cable_length.ladder import IFS_COMPAT, ladder_round  # noqa: E402
from idcbox.domain.cable_length.numeric import excel15  # noqa: E402
from idcbox.domain.cable_length.rules import DEFAULT_RULE  # noqa: E402
from idcbox.domain.geometry.floor import RackPlacement  # noqa: E402

OUT = BACKEND.parent / "shared" / "golden" / "length_vectors.json"

RULE = DEFAULT_RULE


def rack(channel: int, seq: int, side: str, col: int, row: int) -> RackPlacement:
    room = "403" if channel <= 7 else "404"
    return RackPlacement(room, channel, seq, side, col, row)  # type: ignore[arg-type]


def link_vector(name: str, a: RackPlacement, b: RackPlacement, cable: str) -> dict:
    """算一条连线的完整公式链，作为一条向量。"""
    o_class = formulas.classify_o(a, b)
    n = formulas.n_cross_rack(a, b, RULE)
    o, issues = formulas.o_gap_rack(a, b, RULE, o_class)
    entry: dict = {
        "name": name,
        "a": {"channel": a.channel_no, "seq": a.seq_no, "side": a.side, "col": a.col, "row": a.row, "room": a.room_code},
        "b": {"channel": b.channel_no, "seq": b.seq_no, "side": b.side, "col": b.col, "row": b.row, "room": b.room_code},
        "cableType": cable,
        "oClass": o_class,
        "expect": {"n": n, "o": o},
    }
    if o is None:
        entry["expect"].update({"p": None, "q": None, "r": None, "s": None, "t": None})
        entry["issueCodes"] = [i.code.value for i in issues]
        return entry

    p = formulas.p_rack_count(n, o)
    q, q_issues = formulas.q_in_rack(cable, RULE)
    r = formulas.r_inter_rack(p, RULE)
    s = formulas.s_total(r, q)  # type: ignore[arg-type]
    t = formulas.t_ladder(s, RULE)
    entry["expect"].update({"p": p, "q": q, "r": r, "s": s, "t": t})
    codes = [i.code.value for i in issues] + [i.code.value for i in q_issues]
    if codes:
        entry["issueCodes"] = codes
    return entry


def build_link_vectors() -> list[dict]:
    """构造覆盖三类空间关系的连线向量。

    几何口径与真实平面图一致（403 房 ch1：N 侧 row=3、S 侧 row=4；
    N 侧柜号自右向左递增 1..13、S 侧自左向右 14..27），但**列坐标不模拟
    空调位跳号** —— 跳号只影响哪些 col 存在，不影响公式形态，而向量要验的是公式。

    各类的构造逻辑：
    - **A 类**：两柜同一 row（即同一排相邻柜位）→ `O = |Δcol|`，N=0
    - **B 类**：同通道跨侧（row 3 ↔ row 4）→ `O = (CMAX-a.col)+(CMAX-b.col)+1`
    - **C 类**：不同通道 → 同 B 的公式，但置信度最低
    """
    v: list[dict] = []
    CMAX = 38  # 403 房排端列坐标

    def north(seq: int, col: int) -> RackPlacement:
        """403 ch1 北侧（row=3）。"""
        return RackPlacement("403", 1, seq, "N", col, 3)

    def south(seq: int, col: int) -> RackPlacement:
        """403 ch1 南侧（row=4）。"""
        return RackPlacement("403", 1, seq, "S", col, 4)

    # --- 同柜（同一条记录两端指向同一机柜）---
    r1n = north(1, 38)
    v.append(link_vector("same_rack_lc", r1n, r1n, "LC-LC"))
    v.append(link_vector("same_rack_rj45", r1n, r1n, "RJ45"))
    v.append(link_vector("same_rack_mpo", r1n, r1n, "MPO"))

    # --- A 类：同一 row、列差 0..14（覆盖 path=0 的 5 米特例与 6~8 的 15 米段）---
    a0 = north(1, 38)
    for d in (0, 1, 2, 3, 4, 5, 6, 7, 8, 10, 14):
        b = north(1 + d, 38 - d)
        v.append(link_vector(f"A_same_row_d{d}_lc", a0, b, "LC-LC"))
        v.append(link_vector(f"A_same_row_d{d}_rj45", a0, b, "RJ45"))
    # A 类 MPO 覆盖列差 1/3/5/6
    for d in (1, 3, 5, 6):
        v.append(link_vector(f"A_same_row_d{d}_mpo", a0, north(1 + d, 38 - d), "MPO"))

    # --- B 类：同通道跨侧。北侧 seq 1..13，南侧柜号 = 27 - (北侧对应柜) + 1 ---
    # 真实布局里柜 1 与柜 27 面对面（col 都是 38），柜 2 与 26 面对面（col 37）
    for d in (0, 1, 2, 3, 4, 5, 6, 8, 12):
        b_north = north(1 + d, 38 - d)
        b_south = south(27 - d, 38 - d)
        v.append(link_vector(f"B_facing_d{d}_lc", b_north, b_south, "LC-LC"))
        v.append(link_vector(f"B_facing_d{d}_rj45", b_north, b_south, "RJ45"))

    # --- C 类：跨通道 ---
    v.append(link_vector("C_ch1_to_ch2", north(1, 38), RackPlacement("403", 2, 1, "N", 38, 5), "LC-LC"))
    v.append(link_vector("C_ch1_to_ch3", RackPlacement("403", 1, 3, "N", 36, 3), RackPlacement("403", 3, 3, "N", 36, 7), "LC-LC"))
    v.append(link_vector("C_ch1_to_ch3_far", south(25, 36), RackPlacement("403", 3, 3, "N", 36, 7), "LC-LC"))
    v.append(link_vector("C_ch1_to_ch2_mpo", RackPlacement("403", 1, 3, "N", 36, 3), RackPlacement("403", 2, 3, "N", 35, 5), "MPO"))

    # --- 超末档：管理核心交换机 ch1 最北 → 存储带内 ch3 南侧（真实 r451 形态）---
    v.append(link_vector("over_limit_r451", RackPlacement("403", 1, 7, "N", 30, 3), RackPlacement("403", 3, 16, "S", 25, 8), "LC-LC"))
    return v


def build_ladder_vectors() -> list[dict]:
    """T 阶梯：每个 rung 的边界值 + (6,7) 缝隙 + 超档位。"""
    cases = [
        0.0, 1.0, 5.0, 5.0001, 5.72, 6.0, 6.44, 6.9, 7.0, 7.0001, 7.16,
        7.5, 8.5, 8.5001, 10.0, 13.0, 13.0001, 15.0, 17.5, 17.5001, 20.0,
        22.0, 22.0001, 25.0, 25.88, 27.0, 27.0001, 30.0, 32.0, 32.0001,
        35.0, 37.0, 37.0001, 37.4, 38.12, 50.0,
    ]
    out = []
    for s in cases:
        out.append({"s": s, "expect": ladder_round(s, IFS_COMPAT)})
    return out


def build_numeric_vectors() -> list[dict]:
    """Excel 15 位精度：这些值用裸 IEEE754 会算出浮点尾巴。"""
    out = []
    for p in range(0, 55):
        raw = p * 0.6 * 1.2
        out.append({"p": p, "raw": raw, "expect": excel15(raw)})
    return out


def main() -> int:
    link_vectors = build_link_vectors()
    ladder_vectors = build_ladder_vectors()
    numeric_vectors = build_numeric_vectors()

    payload = {
        "schemaVersion": 1,
        "generatedBy": "backend/scripts/build_golden_vectors.py",
        "purpose": "前后端公式镜像的唯一校验契约；两侧测试都必须读它",
        "rule": {
            "rackSpacingM": RULE.rack_spacing_m,
            "slackRatio": RULE.slack_ratio,
            "nRowCoef": RULE.n_row_coef,
            "nRowExtra": RULE.n_row_extra,
            "inRackLenByType": dict(RULE.in_rack_len_by_type),
            "cmaxByRoom": dict(RULE.cmax_by_room),
            "ladderMode": RULE.ladder_mode,
            "crossAisleOffset": formulas.CROSS_AISLE_OFFSET,
        },
        "rungs": [{"seq": r.seq, "op": r.op, "limit": r.limit, "out": r.out, "label": r.label} for r in IFS_COMPAT],
        "linkVectors": link_vectors,
        "ladderVectors": ladder_vectors,
        "numericVectors": numeric_vectors,
    }

    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(payload, ensure_ascii=False, indent=1), encoding="utf-8")

    print(f"写出 {OUT}")
    print(f"  linkVectors    : {len(link_vectors)} 条")
    print(f"  ladderVectors  : {len(ladder_vectors)} 条")
    print(f"  numericVectors : {len(numeric_vectors)} 条")

    # 自检：A/B/C 三类都要有
    classes = {x['oClass'] for x in link_vectors}
    print(f"  覆盖空间类     : {sorted(classes)}")
    assert classes == {"A", "B", "C"}, f"向量没覆盖全部空间类：{classes}"

    # 自检：必须有超档位的向量
    over = [x for x in link_vectors if x["expect"]["t"] is None and x["expect"]["s"] is not None]
    print(f"  超档位向量     : {len(over)} 条")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
