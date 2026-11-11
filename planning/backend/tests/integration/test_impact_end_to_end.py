"""拖动影响面的端到端用例 —— `/api/intent/impact` 与 move 响应的 `impact`。

## 这组用例守护什么

域层用例证明「diff 算得对」，服务层用例证明「失败时说什么」。
这里证明的是**真数据上的数字**，以及一件域层测不到的事：

**拖一台设备会改掉别人 91 行。**

这条数字是 v0.12 的存在理由。它不是估算 —— 是把 `N02-01-S0` 从
通道2 机柜1 挪到机柜5 之后，逐行比出来的真实结果。锁住它有两个作用：
①将来有人改了端口分配规则，会立刻发现「影响面暴涨/归零」；
②任何人读这些用例就知道这件事有多严重，不会顺手把提示删掉。

## 为什么用真实意图文件

合成意图只有两三行，算不出「91 行」。而 91 这个数是产品要对用户
说清楚的事实 —— 它决定了「拖动要不要先问一句」。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[3]
INTENT_PATH = ROOT / "projects" / "v13" / "intent_full.json"

pytestmark = pytest.mark.skipif(
    not INTENT_PATH.exists(), reason="需要真实 V13 意图文件（只在完整仓库里跑）"
)


@pytest.fixture(scope="module")
def client(tmp_path_factory: pytest.TempPathFactory):
    import os

    db = tmp_path_factory.mktemp("db") / "t.db"
    prev = os.environ.get("IDCBOX_DB")
    os.environ["IDCBOX_DB"] = str(db)
    from idcbox.api.main import create_app

    try:
        with TestClient(create_app()) as c:
            yield c
    finally:
        if prev is None:
            os.environ.pop("IDCBOX_DB", None)
        else:
            os.environ["IDCBOX_DB"] = prev


@pytest.fixture(scope="module")
def doc() -> dict:
    return json.loads(INTENT_PATH.read_text(encoding="utf-8"))


#: 本组用例都基于这一次移动：通道2 机柜1 的槽位 0 → 机柜5。
#: 选它是因为它跨柜（位次必变），且是 v0.11 真机验证用过的同一台。
CROSS_RACK = {
    "channel": 2,
    "rack": 1,
    "instance": "计算节点A（ARM）",
    "newU": 3,
    "currentU": 3,
    "targetRack": 5,
}


class TestDryRun:
    """干跑端点：不改 doc，只报影响面。"""

    def test_reports_91_port_changes(self, client: TestClient, doc: dict) -> None:
        """★ 跨柜拖一台 = 91 行端口号变了，涉及 7 台交换机。

        这个数字是可复现的事实，不是上限也不是估计。它由端口分配规则
        决定：组 = (交换机型号, 实例名, 对端型号)，组内按
        `(机柜升序, U 位升序)` 排，设备换柜后位次变，同组排在它后面的
        设备端口号整体前移。

        若哪天这个数变了，先读 `docs/architecture/03-端口分配算法反解报告.md`
        再判断是新 bug 还是规则改了 —— **不要直接改这个数字让它变绿**。
        """
        res = client.post("/api/intent/impact", json={"doc": doc, "move": CROSS_RACK})
        assert res.status_code == 200
        payload = res.json()
        assert payload["ok"] is True, payload["error"]
        assert payload["hasHiddenImpact"] is True
        assert payload["counts"]["portChanges"] == 91
        assert payload["counts"]["affectedSwitches"] == 7
        assert payload["counts"]["added"] == 0
        assert payload["counts"]["removed"] == 0

    def test_reports_position_and_length_changes(self, client: TestClient, doc: dict) -> None:
        """7 行位置变化（该设备在 7 个网口各一根线），6 行线长变了。

        位置变化 7 > 线长变化 6：有一根的档位恰好没变。这正是为什么
        线长必须**真重算**而不能从「位置变了」推 —— 推的话会多报 1 行，
        用户去核对会发现那行其实没变，然后怀疑整个提示。
        """
        payload = client.post(
            "/api/intent/impact", json={"doc": doc, "move": CROSS_RACK}
        ).json()
        assert payload["counts"]["positionChanges"] == 7
        assert payload["counts"]["lengthChanges"] == 6
        assert payload["lengthChecked"] is True

    def test_affected_racks_are_both_ends(self, client: TestClient, doc: dict) -> None:
        payload = client.post(
            "/api/intent/impact", json={"doc": doc, "move": CROSS_RACK}
        ).json()
        assert {"channel": 2, "rack": 1} in payload["affectedRacks"]
        assert {"channel": 2, "rack": 5} in payload["affectedRacks"]

    def test_does_not_return_doc(self, client: TestClient, doc: dict) -> None:
        """干跑不该回整份 doc —— 159KB，前端还有一份，两份会让人以为不一样。"""
        payload = client.post(
            "/api/intent/impact", json={"doc": doc, "move": CROSS_RACK}
        ).json()
        assert "doc" not in payload

    def test_does_not_mutate_input_doc(self, client: TestClient, doc: dict) -> None:
        """干跑是只读的：返回后 doc 必须原样。"""
        before = json.dumps(doc, sort_keys=True)
        client.post("/api/intent/impact", json={"doc": doc, "move": CROSS_RACK})
        assert json.dumps(doc, sort_keys=True) == before


class TestDeepFlag:
    """`deep` 的两种口径。"""

    def test_deep_false_skips_length_and_says_so(self, client: TestClient, doc: dict) -> None:
        """★ `deep=false` 时 `lengthChecked=false` —— 界面靠它区分
        「线长没变」与「线长没算」。

        这两个说法对用户的意义完全不同：前者让他放心，后者提醒他
        去补源文件。混成一句就是说假话。
        """
        payload = client.post(
            "/api/intent/impact",
            json={"doc": doc, "move": CROSS_RACK, "deep": False},
        ).json()
        assert payload["lengthChecked"] is False
        assert payload["counts"]["lengthChanges"] == 0
        # 端口与位置照样报 —— 只有线长被跳过
        assert payload["counts"]["portChanges"] == 91
        assert payload["counts"]["positionChanges"] == 7

    def test_deep_true_is_default(self, client: TestClient, doc: dict) -> None:
        payload = client.post(
            "/api/intent/impact", json={"doc": doc, "move": CROSS_RACK}
        ).json()
        assert payload["lengthChecked"] is True


class TestFailure:
    """失败场景的语义。"""

    def test_overlap_failure_has_zero_counts(self, client: TestClient, doc: dict) -> None:
        """★ 失败时 counts 必须全 0，不能报「删除 5127 行」。

        这是修过的一个 bug：最早用 `diff_generation(before, ())` 表示
        「没有改动后的结果」，于是接口返回 `removed: 5127` ——
        一个失败的动作配一段最坏的消息。
        """
        payload = client.post(
            "/api/intent/impact",
            json={
                "doc": doc,
                "move": {**CROSS_RACK, "newU": 7, "targetRack": None},
            },
        ).json()
        assert payload["ok"] is False
        assert payload["error"]
        assert all(v == 0 for v in payload["counts"].values()), payload["counts"]
        assert payload["hasHiddenImpact"] is False

    def test_unknown_device_failure(self, client: TestClient, doc: dict) -> None:
        payload = client.post(
            "/api/intent/impact",
            json={"doc": doc, "move": {**CROSS_RACK, "instance": "不存在的设备"}},
        ).json()
        assert payload["ok"] is False
        assert "不存在的设备" in payload["error"]

    def test_missing_move_field_is_422(self, client: TestClient, doc: dict) -> None:
        res = client.post("/api/intent/impact", json={"doc": doc})
        assert res.status_code == 422

    def test_missing_doc_is_422(self, client: TestClient) -> None:
        res = client.post("/api/intent/impact", json={"move": CROSS_RACK})
        assert res.status_code == 422


class TestMoveCarriesImpact:
    """`/api/intent/move` 也要带影响面 —— 用户可能直接调它。"""

    def test_move_response_has_impact(self, client: TestClient, doc: dict) -> None:
        res = client.post("/api/intent/move", json={"doc": doc, **CROSS_RACK})
        assert res.status_code == 200
        payload = res.json()
        assert payload["ok"] is True
        assert payload["impact"]["counts"]["portChanges"] == 91
        # 与干跑端点算出来的必须一致，否则界面上「预览」和「实际」会打架
        dry = client.post(
            "/api/intent/impact", json={"doc": doc, "move": CROSS_RACK}
        ).json()
        assert payload["impact"]["counts"] == dry["counts"]

    def test_move_impact_has_no_doc(self, client: TestClient, doc: dict) -> None:
        """响应体里只能有一份 doc —— 两份 159KB 的意图会让前端困惑。"""
        payload = client.post("/api/intent/move", json={"doc": doc, **CROSS_RACK}).json()
        assert "doc" in payload
        assert "doc" not in payload["impact"]

    def test_move_batch_response_has_impact(self, client: TestClient, doc: dict) -> None:
        payload = client.post(
            "/api/intent/move-batch",
            json={
                "doc": doc,
                "targets": [
                    {"channel": 2, "rack": 1, "instance": "计算节点A（ARM）", "currentU": 3}
                ],
                "deltaU": 36,
            },
        ).json()
        assert payload["ok"] is True, payload["error"]
        assert "impact" in payload
        assert payload["impact"]["counts"]["portChanges"] > 0


class TestBatchDryRun:
    def test_batch_shape(self, client: TestClient, doc: dict) -> None:
        payload = client.post(
            "/api/intent/impact",
            json={
                "doc": doc,
                "targets": [
                    {"channel": 2, "rack": 1, "instance": "计算节点A（ARM）", "currentU": 3}
                ],
                "deltaU": 36,
            },
        ).json()
        assert payload["ok"] is True, payload["error"]
        assert payload["counts"]["portChanges"] > 0

    def test_batch_requires_delta(self, client: TestClient, doc: dict) -> None:
        res = client.post(
            "/api/intent/impact",
            json={
                "doc": doc,
                "targets": [
                    {"channel": 2, "rack": 1, "instance": "计算节点A（ARM）", "currentU": 3}
                ],
            },
        )
        assert res.status_code == 422


class TestSmallMoves:
    """不是所有拖动都有大影响 —— 界面不能一律报警。"""

    def test_same_slot_move_is_quiet(self, client: TestClient, doc: dict) -> None:
        """同一槽位内挪 2U（3→5）：端口不变、线长不变。

        这条守的是「提示不能滥用」：如果每次拖动都报一堆影响，
        用户很快学会无视它，那 91 行那次也就没人看了。
        """
        payload = client.post(
            "/api/intent/impact",
            json={
                "doc": doc,
                "move": {
                    "channel": 2,
                    "rack": 1,
                    "instance": "计算节点A（ARM）",
                    "newU": 5,
                    "currentU": 3,
                },
            },
        ).json()
        assert payload["ok"] is True
        assert payload["hasHiddenImpact"] is False
        assert payload["counts"]["portChanges"] == 0
        assert payload["counts"]["lengthChanges"] == 0

    def test_moving_out_of_slot_range_changes_ports(self, client: TestClient, doc: dict) -> None:
        """★ 柜内也不能假定安全：3U → 39U 落到第 10 个槽位，位次就变了。

        v0.12 之前的口径是「同柜内挪不影响线长所以安全」——
        那句话只看到线长，漏了端口号。实测这一挪改 56 行端口号。
        """
        payload = client.post(
            "/api/intent/impact",
            json={
                "doc": doc,
                "move": {
                    "channel": 2,
                    "rack": 1,
                    "instance": "计算节点A（ARM）",
                    "newU": 39,
                    "currentU": 3,
                },
            },
        ).json()
        assert payload["ok"] is True, payload["error"]
        assert payload["counts"]["portChanges"] == 56
        assert payload["hasHiddenImpact"] is True
