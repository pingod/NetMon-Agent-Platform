"""经验表 LOO（leave-one-out）验证：87.11% 这个数字必须能被复算。

## 为什么要 LOO 而不是「全表主值命中率」

全表主值命中率是**自证**的：用包含自己的数据预测自己，必然偏高。
真实场景是「来了一个新项目，用历史经验去猜」，所以必须把当前样本从统计里剔除。

## 三级回退 + 门控（与 `rules/len_empirical_v1.json` 完全一致）

```
预测顺序：L1 (cls, cab, path) → L2 (cls, cab) → L3 (cab)
门控：sample_n >= 10 且 purity >= 0.75
强制人工：空间类 C 或 线缆类型 MPO（历史样本不足以支撑建议）
```

输出 `tests/golden/baseline/empirical_baseline.json`，含：
- `loo_cls_cab_path`：只在 L1 层做 LOO 的准确率（= 文档中的 87.11%）
- `loo_gated`：三级回退 + 门控后的覆盖率与覆盖内准确率（= 85.2% / 94.02%）
"""

from __future__ import annotations

import json
import sys
import warnings
from collections import Counter, defaultdict
from pathlib import Path

import openpyxl

warnings.filterwarnings("ignore")

BACKEND = Path(__file__).resolve().parent.parent
ROOT = BACKEND.parent
sys.path.insert(0, str(BACKEND))

from idcbox.domain.cable_length.empirical import (  # noqa: E402
    ADVISORY_PURITY,
    ADVISORY_SAMPLE_N,
)
from idcbox.domain.importing.normalize import norm_cable_type  # noqa: E402

SRC = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"
OUT = BACKEND / "tests" / "golden" / "baseline" / "empirical_baseline.json"

CMAX = {"403": 38, "404": 18}
GATE_N = 10
GATE_P = 0.75
FORCE_CLASSES = {"C"}
FORCE_CABLES = {"MPO"}


def nm(v: object) -> str:
    return str(v).strip() if v is not None else ""


def load_records() -> list[dict[str, object]]:
    wb = openpyxl.load_workbook(SRC, data_only=True)

    # ---- 几何 ----
    geo: dict[tuple[int, int], tuple[str, int, int]] = {}
    ws2 = wb["机房平面图"]
    for ch_lo, ch_hi, c_lo, c_hi in ((1, 7, 23, 38), (8, 14, 3, 18)):
        for k in range(ch_lo, ch_hi + 1):
            r_n = 3 + 2 * ((k - 1) if k <= 7 else (k - 8))
            for side, rr in (("N", r_n), ("S", r_n + 1)):
                for c in range(c_lo, c_hi + 1):
                    x = ws2.cell(row=rr, column=c).value
                    if x is None:
                        continue
                    s = str(x).strip()
                    if s.isdigit():
                        geo[(k, int(s))] = (side, rr, c)

    def room(ch: int) -> str:
        return "403" if ch <= 7 else "404"

    def pos(ch: str, rack: str) -> tuple[str, int, int] | None:
        try:
            return geo[(int(ch), int(float(rack)))]
        except (KeyError, ValueError):
            return None

    ws = wb["布线表"]
    out: list[dict[str, object]] = []
    for r in range(2, ws.max_row + 1):
        v = [nm(ws.cell(row=r, column=c).value) for c in range(1, 25)]
        if not v[0] and not v[6]:
            continue
        u_raw = v[20]
        if not u_raw:
            continue
        try:
            act = int(float(u_raw))
        except ValueError:
            continue  # sentinel 文本（待定 等）
        a, b = pos(v[2], v[3]), pos(v[8], v[9])
        if a is None or b is None:
            continue
        if a[1] == b[1]:
            cls, path = "A", abs(a[2] - b[2])
        else:
            cls = "B" if v[2] == v[8] else "C"
            path = (
                CMAX[room(int(v[2]))] - a[2]
            ) + (
                CMAX[room(int(v[8]))] - b[2]
            ) + 1
        out.append(
            {
                "row": r,
                "cls": cls,
                "cab": norm_cable_type(v[12]),
                "path": path,
                "actual": act,
            }
        )
    return out


def top_of(counter: Counter[int]) -> int:
    return counter.most_common(1)[0][0]


def main() -> int:
    recs = load_records()
    n = len(recs)
    print(f"样本 = {n} 行（有核定长度 U 且有几何）")

    # ---- L1 单层 LOO ----
    l1: dict[tuple[str, str, int], Counter[int]] = defaultdict(Counter)
    for d in recs:
        l1[(str(d["cls"]), str(d["cab"]), int(d["path"]))][int(d["actual"])] += 1

    ok = 0
    for d in recs:
        k = (str(d["cls"]), str(d["cab"]), int(d["path"]))
        c = Counter(l1[k])
        c[int(d["actual"])] -= 1
        cand = [(cnt, val) for val, cnt in c.items() if cnt > 0]
        pred = max(cand)[1] if cand else None
        if pred == d["actual"]:
            ok += 1
    loo_l1 = ok / n
    print(f"L1 单层 LOO（(cls,cab,path)）= {ok}/{n} = {loo_l1:.2%}")

    # ---- 三级回退 + 门控（LOO）----
    l2: dict[tuple[str, str], Counter[int]] = defaultdict(Counter)
    l3: dict[str, Counter[int]] = defaultdict(Counter)
    for d in recs:
        l2[(str(d["cls"]), str(d["cab"]))][int(d["actual"])] += 1
        l3[str(d["cab"])][int(d["actual"])] += 1

    covered = correct = forced = 0
    by_class: dict[tuple[str, str], list[int]] = defaultdict(lambda: [0, 0])
    advisory = 0
    for d in recs:
        cls, cab = str(d["cls"]), str(d["cab"])
        if cls in FORCE_CLASSES or cab in FORCE_CABLES:
            forced += 1
            continue
        pred: int | None = None
        # L1
        k1 = (cls, cab, int(d["path"]))
        c = Counter(l1.get(k1, Counter()))
        c[int(d["actual"])] -= 1
        tot = sum(v for v in c.values() if v > 0)
        if tot > 0:
            top_cnt, top_val = max((v, k) for k, v in c.items() if v > 0)
            if tot >= GATE_N and top_cnt / tot >= GATE_P:
                pred = top_val
        if pred is None:
            c2 = Counter(l2.get((cls, cab), Counter()))
            c2[int(d["actual"])] -= 1
            tot = sum(v for v in c2.values() if v > 0)
            if tot > 0:
                top_cnt, top_val = max((v, k) for k, v in c2.items() if v > 0)
                if tot >= GATE_N and top_cnt / tot >= GATE_P:
                    pred = top_val
        if pred is None:
            c3 = Counter(l3.get(cab, Counter()))
            c3[int(d["actual"])] -= 1
            tot = sum(v for v in c3.values() if v > 0)
            if tot > 0:
                top_cnt, top_val = max((v, k) for k, v in c3.items() if v > 0)
                if tot >= GATE_N and top_cnt / tot >= GATE_P:
                    pred = top_val
        if pred is None:
            forced += 1
            continue
        covered += 1
        by_class[(cls, cab)][0] += 1
        if pred == d["actual"]:
            correct += 1
            by_class[(cls, cab)][1] += 1

    cov_rate = covered / n
    acc_in_cov = correct / covered if covered else 0.0
    print(f"三级回退+门控 LOO：覆盖 {covered}/{n} = {cov_rate:.2%}，覆盖内正确 {acc_in_cov:.2%}")
    print(f"强制人工（C 类 + MPO + 门控未达）：{forced}/{n} = {forced / n:.2%}")
    for k in sorted(by_class):
        c_, o_ = by_class[k]
        print(f"  {k[0]} {k[1]:6s} 覆盖 {c_:4d} 正确 {o_:4d} = {o_ / c_:.1%}")

    # ---- 全表门控口径 ----
    # 文档里的 coverage_gated=4349 / 85.2% 是**不含 force_manual 排除**的回测口径；
    # 加上"C 类与 MPO 强制人工"后是 4303 / 84.31%。
    # 两个数都要留：前者用于和早期分析对齐，后者才是产品实际行为。
    def full_table_gate(apply_force_manual: bool) -> tuple[int, int]:
        cov = cor = 0
        for d in recs:
            cls, cab = str(d["cls"]), str(d["cab"])
            if apply_force_manual and (cls in FORCE_CLASSES or cab in FORCE_CABLES):
                continue
            pred_ft: int | None = None
            for counter, key in (
                (l1, (cls, cab, int(d["path"]))),
                (l2, (cls, cab)),
                (l3, cab),
            ):
                c = counter.get(key)  # type: ignore[call-overload]
                if not c:
                    continue
                tot = sum(c.values())
                top, top_n = c.most_common(1)[0]
                if tot >= GATE_N and top_n / tot >= GATE_P:
                    pred_ft = top
                    break
            if pred_ft is None:
                continue
            cov += 1
            if pred_ft == d["actual"]:
                cor += 1
        return cov, cor

    cov_no_force, cor_no_force = full_table_gate(False)
    cov_force, cor_force = full_table_gate(True)
    print(
        f"全表门控（回测，不排除 force_manual）：覆盖 {cov_no_force}/{n} = {cov_no_force / n:.2%}，"
        f"覆盖内正确 {cor_no_force / cov_no_force:.2%}"
    )
    print(
        f"全表门控（回测，C类+MPO强制人工）：覆盖 {cov_force}/{n} = {cov_force / n:.2%}，"
        f"覆盖内正确 {cor_force / cov_force:.2%}"
    )

    # ---- 各条经验规则「仅供参考」占比 ----
    l1_main = []
    for k, c in l1.items():
        tot = sum(c.values())
        top, top_n = c.most_common(1)[0]
        l1_main.append(
            {
                "cls": k[0],
                "cab": k[1],
                "path": k[2],
                "len": top,
                "n": tot,
                "purity": round(top_n / tot, 6),
            }
        )
    advisory = sum(
        1
        for e in l1_main
        if e["purity"] < ADVISORY_PURITY or e["n"] < ADVISORY_SAMPLE_N
    )

    result = {
        "source_file": SRC.name,
        "samples": n,
        "gate": {"min_sample_n": GATE_N, "min_purity": GATE_P},
        "force_manual": {"classes": sorted(FORCE_CLASSES), "cable_types": sorted(FORCE_CABLES)},
        "loo_cls_cab_path": round(loo_l1, 6),
        "loo_gated": {
            "covered": covered,
            "coverage_rate": round(cov_rate, 6),
            "correct_within_covered": correct,
            "accuracy_within_covered": round(acc_in_cov, 6),
            "forced_manual": forced,
            "forced_manual_rate": round(forced / n, 6),
            "by_class_cable": {
                f"{k[0]}|{k[1]}": {
                    "covered": v[0],
                    "correct": v[1],
                    "accuracy": round(v[1] / v[0], 6) if v[0] else 0.0,
                }
                for k, v in sorted(by_class.items())
            },
        },
        "full_table_gated_no_force": {
            "covered": cov_no_force,
            "coverage_rate": round(cov_no_force / n, 6),
            "accuracy_within_covered": round(cor_no_force / cov_no_force, 6),
            "note": "含样本自身的回测口径，与早期分析 4349/85.2% 对齐；仅对内披露",
        },
        "full_table_gated_with_force": {
            "covered": cov_force,
            "coverage_rate": round(cov_force / n, 6),
            "accuracy_within_covered": round(cor_force / cov_force, 6),
            "note": "C 类与 MPO 强制人工后的回测口径 = 产品实际行为",
        },
        "l1_rules": len(l1_main),
        "l1_advisory_rules": advisory,
    }
    OUT.parent.mkdir(parents=True, exist_ok=True)
    OUT.write_text(json.dumps(result, ensure_ascii=False, indent=1), encoding="utf-8")
    print(f"\nL1 规则 {len(l1_main)} 条，其中仅供参考 {advisory} 条")
    print(f"落盘：{OUT.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
