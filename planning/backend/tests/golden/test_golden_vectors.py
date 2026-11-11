"""前后端公式镜像的双向校验（Python 侧）。

`shared/golden/length_vectors.json` 是前后端共用的唯一契约，两侧测试都必须读它：

- `backend/tests/golden/test_golden_vectors.py`（本文件）—— 校验 Python 实现
- `web/src/domain/__tests__/golden.test.ts` —— 校验 TypeScript 实现

任何一侧算错，两个 CI 任务里必有一个红。这是"允许公式存在两份代码"的唯一保险，
也是 §2.1 第 2 条铁律（公式单点）在跨语言场景下的落地方式。

## 为什么要单独写这个测试，而不是复用 test_golden_length.py

`test_golden_length.py` 用**真实 V13 数据**回归（627/636 等），验的是"反解对不对"。
本文件用**人工构造的覆盖向量**，验的是"前后端两套实现在边界上是否一致"。
两者互补：
- 真实数据回归发现不了"某类边界前端写错但真实数据恰好没覆盖"
- 合成向量发现不了"公式形式对但常量抄错"
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from idcbox.domain.cable_length import formulas
from idcbox.domain.cable_length.ladder import CONTINUOUS, IFS_COMPAT, ladder_round
from idcbox.domain.cable_length.numeric import excel15
from idcbox.domain.cable_length.rules import DEFAULT_RULE, LengthRule
from idcbox.domain.geometry.floor import RackPlacement

GOLDEN_PATH = Path(__file__).resolve().parents[3] / "shared" / "golden" / "length_vectors.json"


@pytest.fixture(scope="module")
def golden() -> dict:
    assert GOLDEN_PATH.exists(), (
        f"golden 向量缺失：{GOLDEN_PATH}\n"
        "请先跑 backend/scripts/build_golden_vectors.py 生成"
    )
    return json.loads(GOLDEN_PATH.read_text(encoding="utf-8"))


def _rack(v: dict) -> RackPlacement:
    return RackPlacement(v["room"], v["channel"], v["seq"], v["side"], v["col"], v["row"])


class TestGoldenFileIntegrity:
    def test_vectors_cover_all_o_classes(self, golden: dict) -> None:
        """三类空间关系都要有向量，否则前端 B/C 实现错了也没人发现。"""
        classes = {v["oClass"] for v in golden["linkVectors"]}
        assert classes == {"A", "B", "C"}, f"向量没覆盖全部空间类：{classes}"

    def test_rule_in_golden_matches_python_default(self, golden: dict) -> None:
        """向量里固化的规则参数必须等于 Python 默认规则。

        若有人改了 `DEFAULT_RULE` 却没重新生成向量，这里会红 ——
        这正是我们想要的：改公式必须显式重生成契约。
        """
        r = golden["rule"]
        assert r["rackSpacingM"] == DEFAULT_RULE.rack_spacing_m
        assert r["slackRatio"] == DEFAULT_RULE.slack_ratio
        assert r["nRowCoef"] == DEFAULT_RULE.n_row_coef
        assert r["nRowExtra"] == DEFAULT_RULE.n_row_extra
        assert r["inRackLenByType"] == dict(DEFAULT_RULE.in_rack_len_by_type)
        assert r["cmaxByRoom"] == dict(DEFAULT_RULE.cmax_by_room)
        assert r["crossAisleOffset"] == formulas.CROSS_AISLE_OFFSET

    def test_rungs_in_golden_match_ifs_compat(self, golden: dict) -> None:
        """阶梯表必须与 IFS_COMPAT 逐条一致（含顺序）。"""
        got = [(r["seq"], r["op"], r["limit"], r["out"]) for r in golden["rungs"]]
        want = [(r.seq, r.op, r.limit, r.out) for r in IFS_COMPAT]
        assert got == want

    def test_has_over_limit_vector(self, golden: dict) -> None:
        """必须有超末档的向量 —— 这是最容易漏的分支。"""
        over = [
            v for v in golden["linkVectors"]
            if v["expect"]["t"] is None and v["expect"]["s"] is not None
        ]
        assert over, "缺少超末档（S>37）向量"


class TestGoldenLinkVectors:
    @pytest.mark.parametrize("idx", range(52))
    def test_link_vector(self, golden: dict, idx: int) -> None:
        """逐条向量比对完整公式链。"""
        vectors = golden["linkVectors"]
        if idx >= len(vectors):
            pytest.skip("向量数量少于参数化上限")
        v = vectors[idx]
        a, b = _rack(v["a"]), _rack(v["b"])

        assert formulas.classify_o(a, b) == v["oClass"], f"{v['name']}: 空间类判定不符"

        n = formulas.n_cross_rack(a, b, DEFAULT_RULE)
        assert n == v["expect"]["n"], f"{v['name']}: N 不符"

        o, _ = formulas.o_gap_rack(a, b, DEFAULT_RULE, v["oClass"])
        assert o == v["expect"]["o"], f"{v['name']}: O 不符"

        if o is None:
            return

        p = formulas.p_rack_count(n, o)
        q, _ = formulas.q_in_rack(v["cableType"], DEFAULT_RULE)
        r = formulas.r_inter_rack(p, DEFAULT_RULE)
        s = formulas.s_total(r, q)
        t = formulas.t_ladder(s, DEFAULT_RULE)

        assert p == v["expect"]["p"], f"{v['name']}: P 不符"
        assert q == v["expect"]["q"], f"{v['name']}: Q 不符"
        assert r == v["expect"]["r"], f"{v['name']}: R 不符"
        assert s == v["expect"]["s"], f"{v['name']}: S 不符"
        assert t == v["expect"]["t"], f"{v['name']}: T 不符"


class TestGoldenLadderVectors:
    @pytest.mark.parametrize("idx", range(36))
    def test_ladder_vector(self, golden: dict, idx: int) -> None:
        vectors = golden["ladderVectors"]
        if idx >= len(vectors):
            pytest.skip("向量数量少于参数化上限")
        v = vectors[idx]
        assert ladder_round(v["s"], IFS_COMPAT) == v["expect"], f"s={v['s']}"


class TestGoldenNumericVectors:
    @pytest.mark.parametrize("idx", range(55))
    def test_numeric_vector(self, golden: dict, idx: int) -> None:
        vectors = golden["numericVectors"]
        if idx >= len(vectors):
            pytest.skip("向量数量少于参数化上限")
        v = vectors[idx]
        assert excel15(v["raw"]) == v["expect"], f"p={v['p']}"
        assert excel15(v["p"] * 0.6 * 1.2) == v["expect"], f"p={v['p']} 口径不一致"


class TestContinuousDiffersOnlyAtGap:
    """CONTINUOUS 与 IFS_COMPAT 的差异必须恰好在 (6,7) 缝隙内。"""

    def test_gap_is_exactly_6_to_7(self) -> None:
        differ = [s / 100 for s in range(0, 4000) if ladder_round(s / 100, IFS_COMPAT) != ladder_round(s / 100, CONTINUOUS)]
        assert differ, "两种阶梯居然完全一致？"
        assert min(differ) > 6.0, f"差异出现在 {min(differ)}，不该小于 6"
        assert max(differ) < 7.0, f"差异出现在 {max(differ)}，不该达到 7"
