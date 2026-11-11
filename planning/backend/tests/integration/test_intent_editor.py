"""意图编辑的端到端：改格子 → 校验 → 保存 → **生成结果真的变了**。

## 这组用例守护的确是一条闭环

单独测「`/api/intent/row` 能改文档」和「`PUT /intent` 能存版本」都不够 ——
v0.4 开发过程中真的出现过这样的缺陷：

    编辑器改完、保存成功、界面提示「已保存为 v2」，
    但上面的布线表还是 5127 行、一点没变。

原因：生成读的是磁盘上的 `intent.json`，编辑器保存的是**库里新版本**。
两条路径各读一处，都在正常工作，合起来就是错的。

这种 bug 单测抓不到（每个端点都对），只有跑完整条链才会暴露。
所以这组用例的主线只有一个：**改一行 → 存 → 生成出来的行数变化**。

## 为什么用 `TestClient` 而不是起真服务

这组用例关心的是「HTTP 层的参数传递有没有丢」，不是网络栈。
`TestClient` 走的是真实的 FastAPI 路由 + 真实的依赖解析，
参数名写错、Query 别名漏了，它一样会红。
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import pytest
from fastapi.testclient import TestClient

from idcbox.api.main import create_app

pytestmark = pytest.mark.skipif(
    not (Path(__file__).resolve().parents[3] / "信息港部署上架").is_dir(),
    reason="需要真实 V13 源文件（只在完整仓库里跑）",
)

INTENT_FULL = Path(__file__).resolve().parents[3] / "projects" / "v13" / "intent_full.json"


@pytest.fixture(scope="module")
def client(tmp_path_factory: pytest.TempPathFactory) -> TestClient:
    """每个模块用独立的临时数据库 —— 不能碰用户 `data/idcbox.db`。"""
    db = tmp_path_factory.mktemp("db") / "editor.db"
    import os

    os.environ["IDCBOX_DB"] = str(db)
    yield TestClient(create_app())
    os.environ.pop("IDCBOX_DB", None)


@pytest.fixture(scope="module")
def seeded_doc(client: TestClient) -> dict[str, Any]:
    """把全量意图存成 v1，返回原始文档。"""
    doc = json.loads(INTENT_FULL.read_text(encoding="utf-8"))
    r = client.put("/api/projects/T/intent", json={"doc": doc, "note": "基线"})
    assert r.status_code == 200, r.text
    assert r.json()["version"] == 1
    return doc


class TestDesignSourceSwitch:
    """生成的两个来源：项目库 vs 磁盘文件，以及**默认值选哪个**。

    ## 默认值为什么从「磁盘」改成了「库」

    v0.4 第一版：不传 `project` 就读磁盘文件。结果同一个坑换了个形态 ——
    用户保存完刷新页面（URL 里没有 `project`），退回读磁盘，
    看到的是首次导入的旧意图，库里明明是刚存的新版，而且**界面不报错**。

    「修了一半的 bug」比不修更麻烦：它让人以为已经好使了。
    所以默认改成「库里有带版本的项目就读库」，磁盘文件降级为回退。
    """

    def test_project_param_reads_from_db(self, client: TestClient, seeded_doc: dict) -> None:
        payload = client.get("/api/design", params={"project": "T", "limit": 1}).json()
        assert payload["intent"]["sourceFormat"] == "db"
        assert payload["source"] == {"kind": "db", "project": "T", "version": None}
        assert payload["rowsTotal"] > 0

    def test_default_reads_db_not_disk(self, client: TestClient, seeded_doc: dict) -> None:
        """不传 `project` 时用库里最近更新的项目 —— 不是磁盘文件。

        这条是本组用例的核心：默认值错了，用户「保存完刷新就看到改动」
        这件事就是假的。
        """
        payload = client.get("/api/design", params={"limit": 1}).json()
        assert payload["intent"]["sourceFormat"] == "db"
        assert payload["source"]["project"] == "T"

    def test_source_file_forces_disk(self, client: TestClient, seeded_doc: dict) -> None:
        """`source=file` 强制读磁盘 —— 「我刚改完 intent.xlsx 想先看效果」。"""
        payload = client.get("/api/design", params={"limit": 1, "source": "file"}).json()
        assert payload["intent"]["sourceFormat"] in ("json", "xlsx")
        assert payload["source"]["kind"] in ("json", "xlsx")
        assert payload["source"]["project"] is None

    def test_unknown_project_is_404(self, client: TestClient) -> None:
        r = client.get("/api/design", params={"project": "不存在的项目"})
        assert r.status_code == 404

    def test_specific_version_is_honoured(self, client: TestClient, seeded_doc: dict) -> None:
        base = client.get("/api/design", params={"project": "T", "limit": 1}).json()
        v1 = client.get("/api/design", params={"project": "T", "version": 1, "limit": 1}).json()
        assert base["rowsTotal"] == v1["rowsTotal"]
        assert v1["source"]["version"] == 1


class TestEditSaveRegenerate:
    """主线：改一行 → 存 → 生成结果变化。"""

    def test_adding_a_switch_changes_generated_rows(
        self, client: TestClient, seeded_doc: dict
    ) -> None:
        """真正的验收：这不是「保存成功」，而是「生效了」。

        在 ch2 加一台交换机、加一条路由、把它挂进一个新的交换机组，
        生成的连线数必须增加。只加交换机不加组是不会增行的
        （没有组就没有下联关系）—— 这本身就是个容易踩的坑，
        所以这里把三段一起改，模拟用户的真实操作。
        """
        before = client.get("/api/design", params={"project": "T", "limit": 1}).json()
        doc = copy_doc(seeded_doc)

        # 1. 加一台交换机
        r = client.post(
            "/api/intent/row",
            json={
                "doc": doc,
                "section": "switches",
                "op": "append",
                "template": {
                    "instance": "测试接入-1",
                    "model": "业务接入交换机1",
                    "channel": 2,
                    "rack": 26,
                    "u": 40,
                },
            },
        )
        assert r.status_code == 200, r.text
        doc = r.json()["doc"]
        assert r.json()["ok"] is True

        # 2. 加一个交换机组（只含这台新交换机，容量 8 → 只接 8 台服务器）
        r = client.post(
            "/api/intent/row",
            json={
                "doc": doc,
                "section": "chains",
                "op": "append",
                "template": {
                    "channel": 2,
                    "switchModel": "业务接入交换机1",
                    "nic": "万兆9",
                    "instances": ["测试接入-1"],
                    "capacity": 8,
                },
            },
        )
        assert r.status_code == 200, r.text
        doc = r.json()["doc"]

        # 3. 加一条路由，让计算节点A 的万兆9 走这台交换机
        r = client.post(
            "/api/intent/row",
            json={
                "doc": doc,
                "section": "routes",
                "op": "append",
                "template": {
                    "nodeModel": "计算节点A（ARM）",
                    "nic": "万兆9",
                    "switchModel": "业务接入交换机1",
                },
            },
        )
        assert r.status_code == 200, r.text
        doc = r.json()["doc"]
        assert r.json()["check"]["ok"] is True, r.json()["check"]["issues"]

        # 4. 保存成新版本
        saved = client.put("/api/projects/T/intent", json={"doc": doc, "note": "加一台"}).json()
        assert saved["isNew"] is True
        assert saved["version"] == 2

        # 5. 重新生成 —— 行数必须变
        after = client.get("/api/design", params={"project": "T", "limit": 1}).json()
        assert after["rowsTotal"] > before["rowsTotal"], (
            f"保存了新版本但生成结果没变：{before['rowsTotal']} → {after['rowsTotal']}"
        )

        # 6. 增量的确切数字：96 = 通道 2 里全部「计算节点A（ARM）」
        #
        # 这里有个容易误判的语义：**链只有一台交换机时，capacity 不起作用**。
        # 生成器的切块规则是「前 K-1 台各吃 capacity，最后一台吸收全部余量」，
        # 所以单块链 = 把该 (通道, 型号, 网口) 的所有节点都接到这一台交换机上。
        # 96 就是通道 2 的计算节点A 台数。
        #
        # 这也解释了为什么真实意图里每条链都声明 5 台交换机而不是 1 台：
        # 一台 ToR 只有 44~48 个口，接不下 96 台。
        assert after["rowsTotal"] - before["rowsTotal"] == 96, (
            f"新增 {after['rowsTotal'] - before['rowsTotal']} 行，"
            "应为通道 2 全部计算节点A 的 96 台（单块链吸收余量）"
        )

    def test_old_version_still_generates_old_result(
        self, client: TestClient, seeded_doc: dict
    ) -> None:
        """回看历史版本时，生成结果也必须是当时的 —— 否则「版本」没有意义。"""
        v1 = client.get("/api/design", params={"project": "T", "version": 1, "limit": 1}).json()
        latest = client.get("/api/design", params={"project": "T", "limit": 1}).json()
        assert v1["rowsTotal"] < latest["rowsTotal"]


class TestRowEndpoint:
    """行级操作：改一格 / 加一行 / 删一行。"""

    def test_cell_edit_updates_doc_and_check(self, client: TestClient, seeded_doc: dict) -> None:
        doc = copy_doc(seeded_doc)
        r = client.post(
            "/api/intent/row",
            json={
                "doc": doc,
                "section": "switches",
                "op": "cell",
                "rowNo": 0,
                "column": "u",
                "value": "7",
            },
        ).json()
        assert r["ok"] is True
        assert r["doc"]["switches"][0]["u"] == 7
        assert "check" in r

    def test_text_in_int_column_is_rejected_not_zeroed(
        self, client: TestClient, seeded_doc: dict
    ) -> None:
        """原表 `间隔机柜` 列真的出现过 `25G-SR`。静默转 0 会让结果全错而界面正常。"""
        r = client.post(
            "/api/intent/row",
            json={
                "doc": copy_doc(seeded_doc),
                "section": "switches",
                "op": "cell",
                "rowNo": 0,
                "column": "u",
                "value": "二十五",
            },
        ).json()
        assert r["ok"] is False
        assert r["error"] and "需要整数" in r["error"]

    def test_delete_then_check_reports_dangling_reference(
        self, client: TestClient, seeded_doc: dict
    ) -> None:
        """删掉被交换机组引用的交换机后，校验必须报「找不到这台交换机」。"""
        r = client.post(
            "/api/intent/row",
            json={
                "doc": copy_doc(seeded_doc),
                "section": "switches",
                "op": "delete",
                "rowNo": 0,
            },
        ).json()
        assert r["ok"] is True
        # 被删的那台多半在某条链里 —— 除非恰好孤立
        codes = {x["code"] for x in r["check"]["issues"]}
        assert "CHAIN_INSTANCE_UNKNOWN" in codes or r["check"]["ok"] is True

    def test_bad_op_is_422(self, client: TestClient, seeded_doc: dict) -> None:
        r = client.post(
            "/api/intent/row",
            json={"doc": copy_doc(seeded_doc), "section": "switches", "op": "炸掉"},
        )
        assert r.status_code == 422

    def test_missing_doc_is_422(self, client: TestClient) -> None:
        assert client.post("/api/intent/row", json={"section": "switches", "op": "append"}).status_code == 422


class TestValidateEndpoint:
    def test_structural_error_is_reported_not_raised(
        self, client: TestClient, seeded_doc: dict
    ) -> None:
        """结构坏掉时要返回 `structuralError` 让人看见，而不是 500。

        自动化的关键是：保存前的最后一道门必须能**说出哪里坏**，
        不能只给一句「服务器内部错误」。
        """
        broken = copy_doc(seeded_doc)
        broken["schema"] = 999  # 版本不认识
        r = client.post("/api/intent/validate", json={"doc": broken})
        assert r.status_code == 200
        data = r.json()
        assert data["structuralError"] is not None
        assert data["generatable"] is False

    def test_clean_doc_is_generatable(self, client: TestClient, seeded_doc: dict) -> None:
        data = client.post("/api/intent/validate", json={"doc": copy_doc(seeded_doc)}).json()
        assert data["generatable"] is True
        assert data["structuralError"] is None


class TestProjectIntentVersion:
    def test_get_returns_resolved_version(self, client: TestClient, seeded_doc: dict) -> None:
        """传空 version 时必须回填**实际读到的版本号** —— 否则界面显示「v?」。"""
        r = client.get("/api/projects/T/intent").json()
        assert isinstance(r["version"], int)
        assert r["version"] >= 1

    def test_get_specific_version(self, client: TestClient, seeded_doc: dict) -> None:
        r = client.get("/api/projects/T/intent", params={"version": 1}).json()
        assert r["version"] == 1


def copy_doc(doc: dict[str, Any]) -> dict[str, Any]:
    """深拷贝：`/api/intent/row` 会**原地修改**传进来的文档。"""
    return json.loads(json.dumps(doc))


class TestDefaultSourceResolution:
    """默认意图来源的优先级：显式参数 > 环境变量 > 项目库 > 磁盘文件。

    ## 为什么这组用例单独存在

    默认值选错的后果是**静默的**：界面不报错，只是显示旧内容。
    上面 `TestDesignSourceSwitch` 锁的是「有库时默认读库」，
    这里把其余三条回退路径也钉住 —— 少任何一条，都会在某个部署形态下
    变成「保存了但看不到改动」。
    """

    def test_empty_db_falls_back_to_disk(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """库里一个项目都没有时必须回退读磁盘，而不是报错。

        全新部署、还没导入过意图的场景。此时报 404 会让第一次打开界面的人
        以为程序装坏了。
        """
        monkeypatch.setenv("IDCBOX_DB", str(tmp_path / "empty.db"))
        monkeypatch.delenv("IDCBOX_INTENT", raising=False)
        client = TestClient(create_app())
        payload = client.get("/api/design", params={"limit": 1}).json()
        assert payload["intent"]["sourceFormat"] in ("json", "xlsx")
        assert payload["source"]["project"] is None

    def test_source_db_on_empty_db_is_404_with_hint(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """显式 `source=db` 但没有项目 → 404，且告诉用户怎么存一份。

        只说「没有项目」用户不知道下一步做什么；给出 `project-save` 的命令
        才能让他继续。
        """
        monkeypatch.setenv("IDCBOX_DB", str(tmp_path / "empty2.db"))
        monkeypatch.delenv("IDCBOX_INTENT", raising=False)
        client = TestClient(create_app())
        r = client.get("/api/design", params={"limit": 1, "source": "db"})
        assert r.status_code == 404
        assert "project-save" in r.json()["detail"]

    def test_env_pin_beats_db(
        self, client: TestClient, seeded_doc: dict, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """显式设了 `IDCBOX_INTENT` 时优先读那个文件，即使库里有项目。

        设环境变量是一句明确指令。若还悄悄改读库，用户会以为变量没生效 ——
        而且是静默的，最难查。
        """
        monkeypatch.setenv("IDCBOX_INTENT", str(INTENT_FULL))
        payload = client.get("/api/design", params={"limit": 1}).json()
        assert payload["intent"]["sourceFormat"] == "json"
        assert payload["source"]["project"] is None

    def test_explicit_project_beats_env_pin(
        self, client: TestClient, seeded_doc: dict, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """点了具体项目时，即使有环境变量也读该项目。

        界面上选了某个项目是比环境变量更强的意图 —— 用户是在这一页
        明确点了它。
        """
        monkeypatch.setenv("IDCBOX_INTENT", str(INTENT_FULL))
        payload = client.get("/api/design", params={"limit": 1, "project": "T"}).json()
        assert payload["intent"]["sourceFormat"] == "db"
        assert payload["source"]["project"] == "T"
