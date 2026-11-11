"""接线意图 → 布线表：整链路端到端（意图文件 → 生成 → 长度 → 比对）。

## 这组用例守护的东西

`test_golden_generate.py` 证明「生成器算得对」，`test_golden_intent_full.py` 证明
「意图装得下」，本文件证明**用户从头到尾走一遍能拿到什么**：

    projects/v13/intent.json  →  generate_links  →  compute_batch  →  xlsx

三个阶段都有独立的单元/golden 覆盖，但它们拼起来会出两类 bug，
只有端到端才能抓到：

1. **序列化往返丢字段**（真实发生过）：`RackBlock.slots` 只在存盘再读回时
   暴露 —— 内存构造的 Intent 是对的，JSON 往返后 ch3 每柜按默认 8 槽展开，
   设备从 116 台涨到 192 台。这类 bug 单测永远不会发现。
2. **CLI 的列映射错位**：生成行的 `peerId`（槽位 id `N02-11-S7`）如果被写进
   「对端设备名称」列，导出文件里就会出现一堆不存在的设备名。这个 bug 也
   真实发生过一次。

## 与真实 V13 的对账口径

比对键是 `(交换机实例, 交换机端口, 服务器网口)` 三元组。
不用四元组：真实表服务器行**没有设备名**（4562 行里只 332 行有值，且存在
型号与名字矛盾的记录），带上它会让 200 行假性未命中。
"""

from __future__ import annotations

import json
import re
from pathlib import Path

import pytest

from idcbox.application.services.design import (
    compare_with_stock,
    generate_design,
    intent_volume,
)
from idcbox.domain.ports.intent import (
    Intent,
    dump_intent,
    intent_from_dict,
    intent_to_dict,
    load_intent,
)

ROOT = Path(__file__).resolve().parents[3]

#: 用**完整**意图（含 697 条显式连线），不是只覆盖计算节点的 `intent.json`。
#: 存量侧比对基准 `main._stock_rows_for_compare` 自 v0.3 起返回全量 5127 行，
#: 拿 4430 行的子集去比会报出 697 条「生成缺失」—— 那是口径不对，不是算法错。
INTENT_PATH = ROOT / "projects" / "v13" / "intent_full.json"

#: 存量表里型号列为空的行，`main._stock_rows_for_compare` 用的归因标签。
BLANK_MODEL = "（原表未填型号）"

@pytest.fixture(scope="module")
def intent() -> Intent:
    return load_intent(INTENT_PATH)

@pytest.fixture(scope="module")
def design(intent: Intent):
    """带几何的生成结果：长度是本组用例的验收项之一，不能跳过。"""
    from idcbox.api.main import DEFAULT_SOURCE, _load

    views, _ = _load(DEFAULT_SOURCE)
    return generate_design(intent, views.project.geometry)


# ------------------------------------------------------- 意图文件的完整性


def test_intent_file_exists_and_parses(intent: Intent) -> None:
    assert intent.project
    assert intent.racks and intent.switches and intent.routes and intent.chains


def test_intent_volume_is_the_headline_number(intent: Intent) -> None:
    """完整意图的体积 —— 这是产品要讲的核心数字，回归时盯住它。

    `total` 的演进：207（只算布局/交换机/路由/链）→ 915（加上 697 条显式连线）
    → 949（再加上 34 台特殊设备）。每次涨都是因为**之前漏报了用户要手写的量**。
    最后一个数字才是诚实口径：要生成整张 5127 行的表，用户实际要填 949 条。
    """
    vol = intent_volume(intent)
    assert vol["racks"] == 28
    assert vol["switches"] == 130
    assert vol["devices"] == 34
    assert vol["routes"] == 34
    assert vol["chains"] == 26
    assert vol["explicitLinks"] == 697
    assert vol["serverCount"] == 666
    # 949 = 28 布局 + 130 交换机 + 34 特殊设备 + 34 路由 + 26 交换机组 + 697 显式连线
    assert vol["total"] == 949
    assert vol["total"] == sum(
        vol[k] for k in ("racks", "switches", "devices", "routes", "chains", "explicitLinks")
    )


def test_intent_roundtrip_preserves_everything(intent: Intent, tmp_path: Path) -> None:
    """存盘 → 读回 → 再生成，行数与内容必须完全一致。

    这是用来抓 `RackBlock.slots` 那类「只在往返路径暴露」的 bug 的。
    """
    out = tmp_path / "intent.json"
    dump_intent(intent, out)
    again = load_intent(out)

    assert intent_to_dict(intent) == intent_to_dict(again)

    first = generate_design(intent, geometry=None)
    second = generate_design(again, geometry=None)
    assert len(first.rows) == len(second.rows)
    assert first.rows == second.rows


def test_roundtrip_via_dict_is_stable(intent: Intent) -> None:
    """纯 dict 往返（不落盘）也要稳定 —— 防止序列化里出现 set/dict 顺序依赖。"""
    once = intent_to_dict(intent)
    twice = intent_to_dict(intent_from_dict(once))
    assert json.dumps(once, sort_keys=True, ensure_ascii=False) == json.dumps(
        twice, sort_keys=True, ensure_ascii=False
    )


# ------------------------------------------------------- 生成结果的行形状


def test_design_row_count(design) -> None:
    """5127 行 = 真实 V13 布线表的行数，一行不多一行不少。"""
    assert design.stats["links"] == 5127
    assert len(design.rows) == 5127


def test_design_has_no_errors_and_expected_warnings(design) -> None:
    """0 error / 21 warn —— 警告全部是上架位置的**数据**问题，不是生成问题。

    21 个 `RACK_SLOT_CONFLICT`：ch2/ch8/ch9 的 rack11 槽位 7 各叠了 2 台设备
    （源表 3 处 × 7 个网口 = 21 条）。生成器按「顺延整轮」规则排位，
    但**不替用户改上架位置** —— 这是要人去核对现场的事。

    曾经还有第 22 个 `UNKNOWN_CABLE_TYPE`（`广域网链路负载均衡-1 GE8 ↔ -2 GE8`，
    `GE8` 缺 `机框/槽位` 两段、速率解析不出）。后来在意图里给它补了
    `cableType: LC-LC`（真实表就是这么填的），警告随之消失 ——
    **这正是「无法派生的少数几行用显式声明补」该有的样子**。
    """
    assert design.stats["errors"] == 0
    assert design.stats["warns"] == 21
    assert design.stats["issuesByCode"].get("RACK_SLOT_CONFLICT") == 21


def test_every_row_has_identity(design) -> None:
    """每行都有 `peerId`，供界面做「点设备看连线」。

    槽位设备（布局段展开的服务器）id 形如 `N02-11-S7`；
    显式连线的对端是交换机或特殊机型，id 是设备实例名。
    两者都必须非空 —— 空 id 会让界面的「点设备看连线」静默失效。
    """
    for row in design.rows:
        assert row["peerId"], row


#: 槽位 id 的形状：`N02-11-S7`（通道 2 位、机柜 2 位、槽位 1 位，可带 `-1` 后缀）。
#: 不能简单用「以 N 开头」判断 —— `NAT接入交换机-1` 也以 N 开头，
#: 那个是真设备名，误判会让这条用例假报警（已踩过一次）。
SLOT_ID_RE = re.compile(r"^N\d{2}-\d{2}-S\d(-\d)?$")


def test_peer_device_name_is_never_a_slot_id(design) -> None:
    """曾经的真 bug：槽位 id 被当成设备名写进「对端设备名称」列。

    槽位 id 形如 `N02-11-S7`，真实设备名形如 `xkgw-1`/`计算节点A（ARM）-3`。
    """
    for row in design.rows:
        name = row["peerDevice"]
        if name:
            assert not SLOT_ID_RE.match(name), f"槽位 id 泄漏进设备名列：{row}"


def test_u_approved_is_blank_everywhere(design) -> None:
    """核定长度列必须全空 —— 机器不替人签字。"""
    assert all(row.get("uApprovedM") is None for row in design.rows)


# ------------------------------------------------------- 与存量表比对

@pytest.fixture(scope="module")
def stock_compare():
    """存量侧比对素材，直接复用 API 自己的口径函数。

    不自己从 `/api/cables` 拼：那一步的 `localDevice` 是**型号**、
    `localDeviceName` 才是实例名，且字段名与生成行不同（`tLadderM` 缺失）。
    比起在测试里复刻一套字段映射，用 `main._stock_rows_for_compare`
    才能保证「测试比的就是用户看到的那份数据」。
    """
    from idcbox.api.main import _load, _stock_rows_for_compare, DEFAULT_SOURCE

    views, _ = _load(DEFAULT_SOURCE)
    rows, peer_model_of = _stock_rows_for_compare(views)
    return rows, peer_model_of


def test_compare_matches_every_row(design, stock_compare) -> None:
    """5127 生成行与存量表**逐行对齐**：共有 5127、多出 0、缺失 0。

    这是整条链路最强的验收：意图 → 生成 → 长度 → 比对，
    结果与用户手上那张表完全一致（除「对端设备名称」列，见下）。
    """
    rows, _ = stock_compare
    result = compare_with_stock(design, rows, limit=10)
    assert result["generatedRows"] == 5127
    assert result["stockRows"] == 5127
    assert result["common"] == 5127
    assert result["added"] == 0
    assert result["missing"] == 0


def test_no_special_model_rows_are_missing(design, stock_compare) -> None:
    """特殊机型**不再**需要人工补，显式连线能把它们覆盖住。

    历史：v0.3 初期 `ExplicitLink` 只声明不生成，所以 144 行特殊机型
    报「缺失」。补上 `DeviceSpec` + 生成实现后，缺失降到 0。

    这条用例保留断言「缺失必须是 0」而不删掉，因为它同时守着两件事：
    1. 显式连线真的产出行了（否则缺失会跳回 144）
    2. 比对口径没有退化成只取子集（否则 added 会跳到 697）
    """
    rows, peer_model_of = stock_compare
    result = compare_with_stock(design, rows, limit=10, peer_model_of=peer_model_of)
    assert result["missing"] == 0
    assert result["missingByModel"] == []


def test_compare_length_baseline_is_the_636_formula_rows(stock_compare) -> None:
    """比对的**分母**只有 636 行，不是 5127 行。

    源表 T 列（线缆长度1）虽然覆盖 5127 行，但真正带数组公式
    （`_xlfn.IFS` 阶梯）的只有 **636 行**；其余行那格是空白 ——
    策划只给「需要精确长度」的行填了公式。

    生成器对 5127 行**全部**算出长度，是超集。如果把「生成有值、原表空白」
    也算成差异，会得到 4491 条假差异。所以：
    - 分母 = 原表 T 列有数值的行（636）；
    - 分子 = 其中生成器也算出且相等的行（578）。

    这条用例把分母钉死。若哪天意图或生成范围变了导致 636 变成别的数，
    它会先失败，提醒重新核对基线，而不是让吻合率悄悄漂移。
    """
    rows, _ = stock_compare
    with_len = [r for r in rows if r.get("tLadderM") is not None]
    assert len(with_len) == 636, f"原表 T 列有值的行变多了/少了：{len(with_len)}"


def test_length_reproduction_on_clean_inputs_is_100_percent(stock_compare) -> None:
    """**核心验收**：N/O/Q 三个「人填输入列」与几何一致时，公式链 100% 复现。

    ## 这条用例为什么这样设计

    真实表里只有 P/R/S/T 是公式，N/O/Q 是**人手填的**（实测：P/R/S/T 各 636
    个公式格，N/O/Q 公式 0 格、硬编码 640 格）。而公式列完全由 N/O/Q 决定：

        P = N + O
        R = P * 0.6 * 120%
        S = R + Q
        T = IFS(S <= 5, 5, S <= 6, 7, ...)

    所以「公式算得对不对」这个问题必须**先把输入对齐**才能问。
    拿含人工差异的 636 行直接比，得到 578/636 = 90.9%，看起来像公式有问题，
    实际是 58 行的人工 N/O/Q 与几何推导不一致（其中 5 行甚至凑巧抵消，
    T 反而对上了）。

    本用例的做法：只取 N/O/Q 与几何一致的行，再比 P/R/S/T。
    实测 578 行全对，四个公式列都是 578/578 = 100.00%。

    ## 这个数字对产品意味着什么

    - 公式链本身**没有误差**，用户换一套机柜布局，长度是可信的；
    - 剩下的 58 行差异是「原表人工值 vs 几何推导值」的冲突，
      应该报成 issue 让人去核对，**不能悄悄对齐** ——
      悄悄对齐等于把源数据的错误固化进算法。
    """
    from idcbox.api.main import _load, DEFAULT_SOURCE

    views, _ = _load(DEFAULT_SOURCE)
    batch = views.computed.batch.by_key()

    def num(v: object) -> int | None:
        try:
            return int(float(str(v).strip())) if v not in (None, "") else None
        except (TypeError, ValueError):
            return None

    inputs = {"N": ("n_cross", "n_cross_rack"), "O": ("o_gap", "o_gap_rack"), "Q": ("q_in_rack", "q_in_rack_m")}
    formulas = {"P": ("p_count", "p_rack_count"), "R": ("r_inter", "r_inter_rack_m"), "S": ("s_total", "s_total_m"), "T": ("t_ladder", "t_ladder_m")}

    clean = 0
    match = {c: 0 for c in formulas}
    total = {c: 0 for c in formulas}
    for link in views.project.links:
        res = batch.get(link.stable_key)
        raw = views.raw_by_key.get(link.stable_key, {})
        if res is None or num(raw.get("t_ladder")) is None or res.t_ladder_m is None:
            continue
        if any(
            (rv := num(raw.get(rf))) is not None
            and (gv := getattr(res, af, None)) is not None
            and int(gv) != rv
            for rf, af in inputs.values()
        ):
            continue
        clean += 1
        for c, (rf, af) in formulas.items():
            rv, gv = num(raw.get(rf)), getattr(res, af, None)
            if rv is None or gv is None:
                continue
            total[c] += 1
            if int(gv) == rv:
                match[c] += 1

    assert clean == 578, f"N/O/Q 与几何一致的行数变了：{clean}"
    for c in formulas:
        assert match[c] == total[c], f"公式列 {c} 复现 {match[c]}/{total[c]}，应为全中"


def test_length_diffs_are_all_hand_filled_input_conflicts(design, stock_compare) -> None:
    """55 处长度差异**全部**由人填的 N/O/Q 与几何不一致引起，且可分类归因。

    ## 636 行的精确四分表（每一格都实测过，加起来正好 636）

        (人工 N/O/Q 一致?, 长度 T 吻合?)  →  行数
        (True,  True )                   578   ← 公式链 100% 复现
        (False, True )                     3   ← 人工值填错但恰好抵消
        (False, False)                    55   ← 本用例守的就是这些
        (True,  False)                     0   ← **绝不允出现**，见下

    `(True, False) == 0` 是整条公式链最强的断言：只要三个人工输入与几何
    一致，公式给出的长度就必须与 Excel 一模一样。这一格一旦非 0，
    说明是算法退化（公式/阶梯/几何某一环错了），必须立刻查。

    那 55 行的首次分歧列分布：**O 38 / N 9 / Q 6 / T 终值 2**。

    ## 三类各自的成因（每一类都有源表证据）

    - **O 38 行**：集中在 5 台设备（`计算接入交换机(存储)-2` 16 行、
      `存储带外交换机-1` 9 行、`业务核心交换机-2` 6 行、`管理核心交换机-1/-2`
      7 行）。典型形态是**同排两柜的 O 值互换**：
      `业务核心-1 → ch1-r6` 人填 O=7（几何 6）、`→ ch1-r22` 人填 O=6（几何 7）。
      另有 `计算接入交换机(存储)-2 → r4/r5` 出现 `表 0/1 而几何 1/0` ——
      同柜与邻柜的判定被填反。
    - **N 9 行**：`业务核心交换机-1/-2` 的 25GE 口，人填把 `ch1-r22` 记成
      跨 0 排（几何 8）、`ch1-r6` 记成跨 8 排（几何 0），两组恰好互换。
    - **Q 6 行**：`业务核心交换机-1/-2` 与 `广域网链路负载均衡-2` 的
      100G/10G 口，人填 Q=7 而几何 5（同柜内路径系数不同）。

    结论：**不能改算法去追这 55 行**。追等于把源数据的录入错误固化成规则，
    下一套机房照抄这个错。产品该做的是把它列成 issue 让人回去核。
    """
    rows, _ = stock_compare
    result = compare_with_stock(design, rows, limit=200)
    assert result["lengthDiffs"] == 55, result["lengthDiffSamples"][:5]

    # 涉及的本端交换机必须全部落在这 5 台里 —— 若出现新设备，说明是新的
    # 算法问题而不是已知的数据问题，这条断言会先失败。
    known = {
        "计算接入交换机(存储)-2",
        "存储带外交换机-1",
        "业务核心交换机-2",
        "管理核心交换机-1",
        "管理核心交换机-2",
        "业务核心交换机-1",
        "广域网链路负载均衡-1",
        "广域网链路负载均衡-2",
    }
    unexpected = {d["key"][0] for d in result["lengthDiffSamples"]} - known
    assert not unexpected, f"出现了预期外的差异设备：{unexpected}"


def test_length_contingency_table_is_exact(stock_compare) -> None:
    """636 行的四分表必须精确等于 (578, 3, 55, 0)。

    这条用例把三个数字同时钉死：

    - `clean_ok = 578`：公式链能完全复现的行数；
    - `dirty_ok = 3`：人工 N/O/Q 填错、但错误恰好与几何抵消，T 反而对上的行
      （不是好事，只是巧；这 3 行的输入仍是错的，核对时不能被它们迷惑）；
    - `dirty_bad = 55`：人工输入与几何冲突且结果也不同的行；
    - `clean_bad = 0`：**输入一致却算错 —— 一格都不许有。**

    为什么不用「吻合率 578/636 = 90.9%」这种单一数字：它把「算法有问题」
    和「数据有问题」混在一根数轴上了。四分表能把两者分开，用户看到
    `clean_bad = 0` 才知道公式可以信；看到 55 落在 `dirty_bad` 才知道
    要去核那几行的 N/O/Q。

    万一将来意图或几何变了导致某个数字变化，这条用例会失败并打印四格
    实际值，而不是让一个笼统的吻合率悄悄漂移。
    """
    from idcbox.api.main import _load, _stock_rows_for_compare, DEFAULT_SOURCE
    from idcbox.application.services.design import generate_design
    from idcbox.domain.ports.intent import load_intent

    views, _ = _load(DEFAULT_SOURCE)
    rows, _ = _stock_rows_for_compare(views)
    design = generate_design(load_intent(INTENT_PATH), views.project.geometry)

    gen = {(r["localDevice"], r["localPort"], r["nic"]): r for r in design.rows}
    stock = {
        (r.get("localDeviceName") or r["localDevice"], r["localPort"], r["nic"]): r
        for r in rows
    }

    tab = {"clean_ok": 0, "dirty_ok": 0, "dirty_bad": 0, "clean_bad": 0}
    for k, st in stock.items():
        if st.get("tLadderM") is None:
            continue
        g = gen.get(k)
        if g is None or g.get("tLadderM") is None:
            continue

        def same(sf: str, gf: str) -> bool:
            sv, gv = st.get(sf), g.get(gf)
            return sv is None or gv is None or int(gv) == int(sv)

        inputs_ok = (
            same("nCrossRaw", "nCrossRack")
            and same("oGapRaw", "oGapRack")
            and same("qInRackRaw", "qInRackM")
        )
        t_ok = int(g["tLadderM"]) == int(st["tLadderM"])
        cell = ("clean_" if inputs_ok else "dirty_") + ("ok" if t_ok else "bad")
        tab[cell] += 1

    assert tab == {"clean_ok": 578, "dirty_ok": 3, "dirty_bad": 55, "clean_bad": 0}, tab


# ------------------------------------------------------- 边界说明


def test_notes_state_the_generation_boundary(design) -> None:
    """「生成边界」必须随结果下发，否则用户以为 4430 行就是全部。"""
    joined = " ".join(design.notes)
    assert "交换机之间" in joined or "显式连线" in joined


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(pytest.main([__file__, "-q"]))
