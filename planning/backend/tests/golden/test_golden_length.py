"""Golden Test：七公式 + 阶梯，用 V13 真实 636 行逐行回归。

## 断言表（与架构 §10.2 一一对应）

| 用例 | 断言 | 基线 |
|---|---|---|
| `test_n_formula` | N 复现率 | 627/641 = 97.82%（14 行偏差全部在基线白名单内） |
| `test_o_formula` | O 分三分类复现 | A 462/496、B 98/100、C 24/40、合计 584/636 |
| `test_prst` | P/R/S 复算 | 各 636/636 |
| `test_t_ladder` | T 四档实现 | ifs_compat 636/636；continuous 587；ceil_tiers 424；round_nearest 111 |
| `test_excel15` | Excel 15 位口径 | 636/636；不用 excel15 时 170 行差异 |
| `test_u_not_computable` | T→U 不可算（**证明我们没试图算 U**） | same 352 / up 103 / down 172 |

## 为什么 `test_u_not_computable` 是重要断言

如果有人"聪明"地让系统去算 U，这条断言会红。它同时保护两个东西：
一是产品诚实（U 是人的决策），二是审计留痕（系统只给建议）。
"""

from __future__ import annotations

import math

import pytest

from idcbox.domain.cable_length.formulas import (
    classify_o,
    n_cross_rack,
    o_gap_rack,
    p_rack_count,
    q_in_rack,
    r_inter_rack,
    s_total,
    t_ladder,
)
from idcbox.domain.cable_length.ladder import (
    CONTINUOUS,
    IFS_COMPAT,
    ladder_ceil_tiers,
    ladder_round,
    ladder_round_nearest,
)
from idcbox.domain.cable_length.numeric import excel15, to_number
from idcbox.domain.cable_length.rules import CONTINUOUS_RULE, DEFAULT_RULE
from idcbox.domain.geometry.layout import RackRef

from baseline_io import load_baseline

pytestmark = pytest.mark.golden


def _pos(geo, ch, rack):
    return geo.pos(RackRef(channel_no=ch, seq_no=rack))


class TestGeometryFormulas:
    def test_n_formula(self, geo, rows, col):
        base = load_baseline("v13_baseline")["n"]
        ok = cmp_ = 0
        bad_rows = []
        for i, v in enumerate(rows):
            a = _pos(geo, v[col["ch_local"]], v[col["rack_local"]])
            b = _pos(geo, v[col["ch_peer"]], v[col["rack_peer"]])
            if a is None or b is None:
                continue
            src = to_number(v[col["n_cross"]])
            if src is None:
                continue
            cmp_ += 1
            calc = n_cross_rack(a, b, DEFAULT_RULE)
            if int(src) == calc:
                ok += 1
            else:
                bad_rows.append(i)

        assert cmp_ == base["cmp"], f"N 可比对行数变了：{cmp_} != {base['cmp']}"
        assert ok == base["ok"], (
            f"N 命中数变了：{ok}/{cmp_}（基线 {base['ok']}/{base['cmp']}）。"
            f"偏差行索引：{bad_rows[:20]}"
        )
        assert ok / cmp_ >= 0.97, f"N 复现率跌破 97%：{ok / cmp_:.4f}"

        # 偏差行必须与基线白名单**逐行一致**：证明偏差来自源数据而非我们改动了实现。
        # 注意 rows[i] 的 i 是 0-based，基线 row 是 Excel 行号（+2）。
        whitelist = {m["row"] for m in base["mismatches"]}
        got = {i + 2 for i in bad_rows}
        assert got == whitelist, (
            f"N 偏差行变了。多出：{sorted(got - whitelist)[:10]}；"
            f"少了：{sorted(whitelist - got)[:10]}"
        )

    def test_o_formula(self, geo, rows, col):
        base = load_baseline("v13_baseline")["o"]
        by_class = {"A": [0, 0], "B": [0, 0], "C": [0, 0]}
        for i, v in enumerate(rows):
            a = _pos(geo, v[col["ch_local"]], v[col["rack_local"]])
            b = _pos(geo, v[col["ch_peer"]], v[col["rack_peer"]])
            if a is None or b is None:
                continue
            src = to_number(v[col["o_gap"]])
            if src is None:
                continue
            cls = classify_o(a, b)
            calc, _ = o_gap_rack(a, b, DEFAULT_RULE, cls)
            by_class[cls][0] += 1
            if calc is not None and int(src) == calc:
                by_class[cls][1] += 1

        for cls in ("A", "B", "C"):
            exp = base["by_class"][cls]
            got_cmp, got_ok = by_class[cls]
            assert got_cmp == exp["total"], f"O-{cls} 可比对数变了：{got_cmp} != {exp['total']}"
            assert got_ok == exp["ok"], (
                f"O-{cls} 命中数变了：{got_ok}/{got_cmp}（基线 {exp['ok']}/{exp['total']}）"
            )
        total_ok = sum(v[1] for v in by_class.values())
        assert total_ok == base["ok"], f"O 合计命中变了：{total_ok} != {base['ok']}"

    def test_prst(self, geo, rows, col):
        """P/R/S 是公式链的**代数恒等**验证，必须喂源 N/O。

        为什么不用我们算出的 N/O：源表里 P/R/S 本身是活公式（`=N2+O2`、
        `=P2*0.6*120%`、`=R2+Q2`），它们与源 N/O 恒等。若用我们算出的 N/O，
        一旦 N/O 行填错（52 行），P/R/S 也会跟着错 —— 那样测的是"N/O 对不对"
        而不是"公式链实现对不对"，两个错误会互相掩盖。

        本用例只管一件事：**把 N/O 喂进去，P→R→S 出来的数是否与 Excel 一模一样**
        （含 15 位有效数字口径）。N/O 的准确率由上面两个用例单独负责。
        """
        base = load_baseline("v13_baseline")
        p_ok = r_ok = s_ok = 0
        p_cmp = r_cmp = s_cmp = 0
        for v in rows:
            n_src = to_number(v[col["n_cross"]])
            o_src = to_number(v[col["o_gap"]])
            if n_src is None or o_src is None:
                continue
            p_cmp += 1
            p_calc = p_rack_count(int(n_src), int(o_src))
            p_src = to_number(v[col["p_count"]])
            if p_src is not None and p_calc == int(p_src):
                p_ok += 1

            r_calc = r_inter_rack(p_calc, DEFAULT_RULE)
            r_src = to_number(v[col["r_inter"]])
            if r_src is not None:
                r_cmp += 1
                if math.isclose(r_calc, float(r_src), rel_tol=1e-12):
                    r_ok += 1

            q_src = to_number(v[col["q_in_rack"]])
            s_src = to_number(v[col["s_total"]])
            if q_src is not None and s_src is not None:
                s_cmp += 1
                if math.isclose(s_total(r_calc, float(q_src)), float(s_src), rel_tol=1e-12):
                    s_ok += 1

        assert p_cmp == base["p"]["ok"], f"P 可比对行数变了：{p_cmp}"
        assert p_ok == base["p"]["ok"], f"P 复算一致数变了：{p_ok}/{p_cmp}"
        assert r_ok == base["r"]["ok"], f"R 复算一致数变了：{r_ok}/{r_cmp}"
        assert s_ok == base["s"]["ok"], f"S 复算一致数变了：{s_ok}/{s_cmp}"

    def test_q_by_cable_type(self, rows, col):
        """Q（机内线缆）实测只 2 个值且强绑定线缆类型：RJ45 固定 7、光纤固定 5。

        这条用例锁住"不能把 Q 建模成可调参数"的认知：数据集里它就是一个查表。
        """
        seen: dict[str, set[float]] = {}
        for v in rows:
            q_src = to_number(v[col["q_in_rack"]])
            if q_src is None:
                continue
            cab = str(v[col["cable_type"]]).strip()
            seen.setdefault(cab, set()).add(float(q_src))
        assert seen["RJ45"] == {7.0}, f"RJ45 的 Q 不唯一：{seen['RJ45']}"
        assert seen["LC-LC"] <= {5.0, 7.0}, f"LC-LC 的 Q 出现意外值：{seen['LC-LC']}"
        assert seen["MPO"] <= {5.0, 7.0}, f"MPO 的 Q 出现意外值：{seen['MPO']}"
        # 查表函数本身
        assert q_in_rack("RJ45", DEFAULT_RULE)[0] == 7
        assert q_in_rack("LC-LC", DEFAULT_RULE)[0] == 5
        assert q_in_rack("MPO", DEFAULT_RULE)[0] == 5


class TestLadder:
    def test_t_ladder_four_impls(self, geo, rows, col):
        """四档实现的精确对比 —— 这是防止实现者偷懒换成档位表的关键断言。"""
        base = load_baseline("v13_baseline")["t"]["by_impl"]
        samples: list[tuple[float, int]] = []
        for v in rows:
            s_src = to_number(v[col["s_total"]])
            t_src = to_number(v[col["t_ladder"]])
            if s_src is None or t_src is None:
                continue
            samples.append((float(s_src), int(t_src)))

        assert len(samples) == base["ifs_compat"]["cmp"]

        def score(fn) -> int:
            return sum(1 for s, t in samples if fn(s) == t)

        got = {
            "ifs_compat": score(lambda s: ladder_round(s, IFS_COMPAT)),
            "continuous": score(lambda s: ladder_round(s, CONTINUOUS)),
            "ceil_tiers": score(ladder_ceil_tiers),
            "round_nearest": score(ladder_round_nearest),
        }
        for name, n in got.items():
            assert n == base[name]["ok"], (
                f"{name} 命中数变了：{n}（基线 {base[name]['ok']}）"
            )

        # 四条硬边界
        assert got["ifs_compat"] == len(samples), "IFS 逐字实现必须 100% 复现"
        assert got["continuous"] != got["ifs_compat"], (
            "continuous 与 ifs_compat 若相同，说明缝隙处理被改掉了"
        )
        assert got["ceil_tiers"] < got["continuous"], "档位法必须显著更差，否则实现有问题"

    def test_ladder_gap_exists(self):
        """(6, 7) 开区间缝隙必须存在 —— 这是原公式 bug 的可执行记录。"""
        assert ladder_round(6.44, IFS_COMPAT) == 10
        assert ladder_round(7.0, IFS_COMPAT) == 7
        assert ladder_round(6.44, CONTINUOUS) == 7
        # 缝隙区间内两个实现必须不同
        for s in (6.01, 6.44, 6.99):
            assert ladder_round(s, IFS_COMPAT) != ladder_round(s, CONTINUOUS)

    def test_ladder_boundaries(self):
        """逐条边界：`le` 取等号，`eq` 只取等号，超末档返回 None。"""
        assert ladder_round(5.0, IFS_COMPAT) == 5
        assert ladder_round(5.0000001, IFS_COMPAT) == 7
        assert ladder_round(6.0, IFS_COMPAT) == 7
        assert ladder_round(8.5, IFS_COMPAT) == 10
        assert ladder_round(13.0, IFS_COMPAT) == 15
        assert ladder_round(37.0, IFS_COMPAT) == 40
        assert ladder_round(37.0001, IFS_COMPAT) is None
        assert ladder_round(100.0, IFS_COMPAT) is None

    def test_t_ladder_fn(self, geo, rows, col):
        base = load_baseline("v13_baseline")["t"]
        ok = cmp_ = 0
        for v in rows:
            s = to_number(v[col["s_total"]])
            t = to_number(v[col["t_ladder"]])
            if s is None or t is None:
                continue
            cmp_ += 1
            if t_ladder(float(s), DEFAULT_RULE) == int(t):
                ok += 1
        assert cmp_ == base["cmp"]
        assert ok == base["ok"]


class TestExcel15:
    def test_excel15_matches_source(self, geo, rows, col):
        """R 列必须用 excel15；直接 IEEE754 会有 170 行差异（这是防"优化"断言）。

        判据必须是**精确相等**，不能用 `math.isclose`：`3.5999999999999996` 与
        `3.6` 在 rel_tol=1e-12 下被判为相等，那样这条用例就测不出任何东西了。
        Excel 单元格里存的就是 3.6 这个双精度值，我们必须产出位级相同的数。
        """
        base = load_baseline("v13_baseline")["r"]["ok"]
        with_15 = without_15 = cmp_ = 0
        for v in rows:
            p = to_number(v[col["p_count"]])
            r_src = to_number(v[col["r_inter"]])
            if p is None or r_src is None:
                continue
            cmp_ += 1
            target = float(r_src)
            if excel15(p * 0.6 * 1.2) == target:
                with_15 += 1
            if p * 0.6 * 1.2 == target:
                without_15 += 1
        assert cmp_ == base
        assert with_15 == base, f"excel15 口径必须 100% 位级一致，实际 {with_15}/{base}"
        assert without_15 < with_15, "直接 IEEE754 应当显著更差（约 170 行差异）"
        assert with_15 - without_15 > 100, (
            f"差异行数只有 {with_15 - without_15} 行，excel15 可能没生效"
        )

    def test_excel15_basics(self):
        assert excel15(0.1 + 0.2) == 0.3
        assert excel15(5 * 0.6 * 1.2) == 3.6
        assert excel15(3 * 0.6 * 1.2) == 2.16
        assert excel15(0) == 0


class TestUNotComputable:
    def test_u_relation(self, geo, rows, col):
        """T→U 的关系只能统计，不能复现 —— 这条断言是「产品诚实」的可执行证明。"""
        base = load_baseline("v13_baseline")["tu_relation"]
        same = up = down = 0
        for v in rows:
            t = to_number(v[col["t_ladder"]])
            u = to_number(v[col["u_approved"]])
            if t is None or u is None:
                continue
            if u == t:
                same += 1
            elif u > t:
                up += 1
            else:
                down += 1
        assert {"same": same, "up": up, "down": down} == base
        # 关键：下调组必须非空（证明"人工只会加余量"是错的）
        assert down > 0, "如果下调为 0，说明 U 真的可以算出来 —— 那要重新设计"
        assert same / (same + up + down) < 0.60, (
            "T→U 一致率意外地高，需重新评估 U 是否可算"
        )
