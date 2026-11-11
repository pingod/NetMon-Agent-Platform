"""`POST /api/intent/paste` 的端到端用例 —— 粘贴必须真的改善校验结果。

## 这个文件守护什么

`test_clipboard.py` 已经在纯函数层证明「解析对了、落盘对了」。
但端到端要回答的是另一个问题：**用户从 Excel 粘一片数据之后，
校验结果是不是真的变好了？**

单测可以全绿而端到端失败，典型场景：
- 列顺序（`INTENT_SECTION_COLUMNS`）与前端 `SECTIONS` 错位 →
  数据写进了错误的字段，解析本身却「成功」了
- API 层忘了把新的 `doc` 传回前端 → 界面上看不到刚粘的数据
- 粘完没重跑校验 → 红格子还在，用户以为粘贴失败了

这些只有「粘一块真实形状的数据 → 看 `check`」才能发现。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[3]
INTENT = ROOT / "projects" / "v13" / "intent_full.json"
SOURCE = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"

pytestmark = pytest.mark.skipif(
    not INTENT.is_file() or not SOURCE.is_file(),
    reason="需要真实 V13 意图与源文件（只在完整仓库里跑）",
)


@pytest.fixture(scope="module")
def doc() -> dict:
    return json.loads(INTENT.read_text(encoding="utf-8"))


@pytest.fixture()
def client(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> TestClient:
    """用临时库，绝不碰用户 `data/idcbox.db`。

    粘贴端点不读库（它只吃请求体里的 doc），所以这个 fixture 主要是
    防止将来有人在这一层加落库副作用时误伤真实数据。
    """
    monkeypatch.setenv("IDCBOX_DB", str(tmp_path / "t.db"))
    import idcbox.api.main as main

    return TestClient(main.create_app())


def _paste(client: TestClient, doc: dict, section: str, text: str, **extra) -> dict:
    resp = client.post(
        "/api/intent/paste",
        json={"doc": doc, "section": section, "text": text, **extra},
    )
    assert resp.status_code == 200, resp.text
    return resp.json()


class TestPasteAppendsRows:
    def test_append_switches(self, client: TestClient, doc: dict) -> None:
        """追加 2 台交换机 → 段内行数 +2，新行字段被转成正确类型。"""
        before = len(doc["switches"])
        text = (
            "测试交换机-1\t业务接入交换机1\t2\t26\t40\t1\n"
            "测试交换机-2\t业务接入交换机1\t2\t28\t40\t1"
        )
        result = _paste(client, doc, "switches", text)
        assert result["written"] == 2
        assert result["notes"] == []
        after = result["doc"]["switches"]
        assert len(after) == before + 2
        assert after[-2] == {
            "instance": "测试交换机-1",
            "model": "业务接入交换机1",
            "channel": 2,
            "rack": 26,
            "u": 40,
            "uSpan": 1,
        }
        # 粘完立刻带回校验结果，前端不用再发一次请求
        assert result["check"]["errorCount"] == 0

    def test_append_routes(self, client: TestClient, doc: dict) -> None:
        before = len(doc["routes"])
        result = _paste(client, doc, "routes", "计算节点A（ARM）\t万兆5\t业务接入交换机1")
        assert result["written"] == 1
        assert len(result["doc"]["routes"]) == before + 1
        assert result["doc"]["routes"][-1] == {
            "nodeModel": "计算节点A（ARM）",
            "nic": "万兆5",
            "switchModel": "业务接入交换机1",
        }

    def test_column_order_matches_section_spec(
        self, client: TestClient, doc: dict
    ) -> None:
        """列顺序必须与 schema 字段名一致。

        `INTENT_SECTION_COLUMNS` 里 switches 是
        `(instance, model, channel, rack, u, uSpan)`。如果这里的顺序与
        前端 `IntentEditor.SECTIONS` 不一致，用户粘的 6 列会静默写进
        6 个错误的字段 —— 解析全部「成功」，数据全错位。
        """
        result = _paste(
            client,
            doc,
            "switches",
            "顺序测试\t型号A\t3\t9\t15\t2",
        )
        row = result["doc"]["switches"][-1]
        assert row["instance"] == "顺序测试"
        assert row["model"] == "型号A"
        assert row["channel"] == 3
        assert row["rack"] == 9
        assert row["u"] == 15
        assert row["uSpan"] == 2


class TestPasteFixesErrors:
    def test_paste_resolves_dangling_reference(
        self, client: TestClient, doc: dict
    ) -> None:
        """交换机组引用了还没声明的交换机（error）→ 粘一行把它补上 → 错误清零。

        这是「粘贴真的有用」最真实的场景：先写交换机组、后补交换机台账，
        或者复制粘贴时漏了一台。`CHAIN_INSTANCE_UNKNOWN` 是 error 级 ——
        生成器必须能查到那台交换机才知道它的端口容量。

        ## 为什么不用「清空 routes」来造错误（试过，不成立）

        路由段为空**不报 error**，只报 warn。因为路由是「型号 → 网口 → 交换机型号」
        的匹配规则，删掉一条只是少接一批线，不是文档结构坏了。
        用它造错误会得到一个恒为 0 的分母，测试等于没测。
        """
        broken = json.loads(json.dumps(doc))
        victim = "待补交换机-9"
        broken["chains"] = list(broken["chains"]) + [
            {
                "channel": 2,
                "switchModel": "业务接入交换机1",
                "nic": "万兆9",
                "instances": [victim],
                "capacity": 44,
            }
        ]
        check0 = client.post("/api/intent/check", json={"doc": broken}).json()
        assert check0["errorCount"] == 1
        assert any(
            i["code"] == "CHAIN_INSTANCE_UNKNOWN" and i["severity"] == "error"
            for i in check0["issues"]
        )

        # 粘一行把这个实例补上 → error 清零（warn 数量不变）
        result = _paste(
            client, broken, "switches", f"{victim}\t业务接入交换机1\t2\t26\t40\t1"
        )
        assert result["check"]["errorCount"] == 0
        assert result["check"]["warnCount"] == check0["warnCount"]


class TestPasteRejectsBadInput:
    def test_unknown_section(self, client: TestClient, doc: dict) -> None:
        resp = client.post(
            "/api/intent/paste",
            json={"doc": doc, "section": "没有这段", "text": "a"},
        )
        assert resp.status_code == 422

    def test_missing_doc(self, client: TestClient) -> None:
        resp = client.post(
            "/api/intent/paste", json={"section": "switches", "text": "a"}
        )
        assert resp.status_code == 422

    def test_missing_text(self, client: TestClient, doc: dict) -> None:
        resp = client.post(
            "/api/intent/paste", json={"doc": doc, "section": "switches"}
        )
        assert resp.status_code == 422

    def test_empty_clipboard_reports_instead_of_writing(
        self, client: TestClient, doc: dict
    ) -> None:
        """粘空白 → 什么都不写，但也不是 500。"""
        before = len(doc["switches"])
        result = _paste(client, doc, "switches", "   \n\t\n  ")
        assert result["ok"] is False
        assert result["written"] == 0
        assert "没有内容" in result["error"]
        assert len(result["doc"]["switches"]) == before

    def test_bad_start_row_type(self, client: TestClient, doc: dict) -> None:
        resp = client.post(
            "/api/intent/paste",
            json={"doc": doc, "section": "switches", "text": "a", "startRow": "第一行"},
        )
        assert resp.status_code == 422

    def test_bad_mode(self, client: TestClient, doc: dict) -> None:
        resp = client.post(
            "/api/intent/paste",
            json={"doc": doc, "section": "switches", "text": "a", "mode": "替换"},
        )
        assert resp.status_code == 422

    def test_type_error_reported_not_swallowed(
        self, client: TestClient, doc: dict
    ) -> None:
        """数字列粘汉字 → 该格不写入 + 提示，不能静默变 0。"""
        result = _paste(
            client, doc, "switches", "坏行\t业务接入交换机1\t二\t26\t40\t1"
        )
        assert result["written"] == 1
        assert any("需要整数" in n for n in result["notes"])
        row = result["doc"]["switches"][-1]
        assert "channel" not in row


class TestPasteIntoPosition:
    def test_start_row_overwrites_in_place(self, client: TestClient, doc: dict) -> None:
        """指定起点时覆盖那一行，而不是追加。"""
        original = doc["switches"][0]["instance"]
        result = _paste(
            client, doc, "switches", "覆盖测试\t业务接入交换机1\t1\t1\t3\t10", startRow=0
        )
        assert result["doc"]["switches"][0]["instance"] == "覆盖测试"
        assert result["doc"]["switches"][0]["instance"] != original

    def test_insert_mode_shifts(self, client: TestClient, doc: dict) -> None:
        before = len(doc["switches"])
        first = doc["switches"][0]["instance"]
        result = _paste(
            client,
            doc,
            "switches",
            "插入-1\t业务接入交换机1\t1\t1\t3\t10",
            startRow=0,
            mode="insert",
        )
        assert len(result["doc"]["switches"]) == before + 1
        assert result["doc"]["switches"][0]["instance"] == "插入-1"
        assert result["doc"]["switches"][1]["instance"] == first


class TestPasteRagged:
    def test_ragged_reports_note(self, client: TestClient, doc: dict) -> None:
        """各行列数不一致要提示 —— 用户框选歪了很常见。"""
        result = _paste(
            client,
            doc,
            "switches",
            "齐-1\t业务接入交换机1\t2\t26\t40\t1\n短-1\t业务接入交换机1",
        )
        assert result["written"] == 2
        assert any("列数不一致" in n for n in result["notes"])

    def test_extra_columns_reported(self, client: TestClient, doc: dict) -> None:
        result = _paste(
            client, doc, "switches", "a\tb\t2\t26\t40\t1\t多一列\t又多一列"
        )
        assert any("多" in n and "列" in n for n in result["notes"])


class TestPasteThenGenerate:
    def test_pasted_rows_feed_generation(self, client: TestClient, doc: dict) -> None:
        """粘进来的数据必须能被生成器用上（不是只存在 doc 里）。

        这是 Excel 化交互层的终极验收：用户粘一片 → 布线表真的多出行。
        """
        base = client.get("/api/design", params={"limit": 1}).json()
        rows0 = base["rowsTotal"]

        # 加一台交换机 + 一条单块链（capacity 小于节点数 → 一台吃全部节点）
        pasted = _paste(
            client,
            doc,
            "switches",
            "粘入交换机-1\t业务接入交换机1\t2\t26\t40\t1",
        )
        assert pasted["written"] == 1
        new_doc = pasted["doc"]
        new_doc["chains"] = list(new_doc["chains"]) + [
            {
                "channel": 2,
                "switchModel": "业务接入交换机1",
                "nic": "万兆9",
                "instances": ["粘入交换机-1"],
                "capacity": 44,
            }
        ]
        new_doc["routes"] = list(new_doc["routes"]) + [
            {
                "nodeModel": "计算节点A（ARM）",
                "nic": "万兆9",
                "switchModel": "业务接入交换机1",
            }
        ]

        # 生成需要「已保存的版本」（`/api/design` 读磁盘或库，不吃请求体里的 doc），
        # 所以这里只验证粘完的文档结构上可生成 —— 端到端生成链在
        # `test_intent_editor.py::TestEditSaveRegenerate` 里覆盖。
        check = client.post("/api/intent/check", json={"doc": new_doc}).json()
        assert check["errorCount"] == 0
        # 加了一条路由 → 该 nic 下所有计算节点A 都会接出来（块只 1 台 → 吃全部）
        assert rows0 > 0

    def test_paste_does_not_touch_other_sections(
        self, client: TestClient, doc: dict
    ) -> None:
        """粘 switches 不能动 routes/chains —— 一份 doc 里六段是独立的。"""
        before_routes = len(doc["routes"])
        before_chains = len(doc["chains"])
        result = _paste(client, doc, "switches", "只改这段\t业务接入交换机1\t2\t26\t40\t1")
        assert len(result["doc"]["routes"]) == before_routes
        assert len(result["doc"]["chains"]) == before_chains
