"""型号库的端到端：界面能不能自己加型号、加了之后台账数字会不会跟着变。

## 为什么必须有这一层

单测（`test_modellib.py`）保证「叠加逻辑对」；这里保证的是
**三层接线都对**：API 找得到项目库文件、台账真的用了它、导出的库
能原样导回。

三处接线里任何一处断了，症状都是「数字不变」——
不报错、不变红，用户只会觉得程序坏了。所以每个用例都断言**具体数字**，
而不是 `assert result is not None`。
"""

from __future__ import annotations

import os
import shutil
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[3]
SAMPLE = ROOT / "projects" / "v13" / "intent_full.json"
PROJ_DIR = ROOT / "projects"

pytestmark = pytest.mark.skipif(
    not SAMPLE.is_file(), reason="需要真实 V13 意图（只在完整仓库里跑）"
)


#: 两个 fixture 必须指向同一个库文件，否则 project 建在一处、API 读另一处
_DB_NAME = "test.db"


@pytest.fixture()
def client(tmp_path: Path, monkeypatch: pytest.MonkeyPatch):
    from idcbox.api.main import create_app

    monkeypatch.setenv("IDCBOX_DB", str(tmp_path / _DB_NAME))
    monkeypatch.delenv("IDCBOX_INTENT", raising=False)
    return TestClient(create_app())


@pytest.fixture()
def project_name(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> str:
    """造一个临时项目（库里一条 + 目录一个），用完都删掉。

    ## 为什么要真的建项目而不是只造一个名字

    `/api/ledger` 需要一个「库里有意图版本」的项目才能算台账。
    只造名字的话它返回 404，而测试会以为「容量没变」是型号库没接上 ——
    实际上根本没走到那一步。造项目才能测到真正的接线。

    不能直接用 `v13` —— 那会写进用户真实的项目目录，
    跑一次测试就在仓库里留下一个 `projects/v13/models.json`。
    """
    name = f"_test_models_{os.getpid()}"
    shutil.rmtree(PROJ_DIR / name, ignore_errors=True)

    import json

    from idcbox.application.services.projects import save_intent
    from idcbox.infrastructure.db.repositories import ProjectRepository
    from idcbox.infrastructure.db.session import create_db_engine, init_schema

    engine = create_db_engine(os.environ["IDCBOX_DB"])
    init_schema(engine)
    repo = ProjectRepository(engine)
    repo.create(name=name)
    save_intent(repo, project_name=name, intent=json.loads(SAMPLE.read_text(encoding="utf-8")))

    yield name
    shutil.rmtree(PROJ_DIR / name, ignore_errors=True)


def _custom(name: str = "自研汇聚X", ports: int = 4) -> dict:
    return {
        "schema": 1,
        "models": [
            {
                "name": name,
                "category": "switch",
                "groups": [
                    {
                        "group": "25GE",
                        "panel": 1,
                        "indices": list(range(1, ports + 1)),
                        "nominalMbps": 25000,
                        "speeds": [25000],
                        "role": "downlink",
                    }
                ],
            }
        ],
    }


class TestReadOnlySurface:
    def test_default_library_is_builtin(self, client: TestClient) -> None:
        """没有任何项目库时读到内置 26 个型号、855 个口。"""
        doc = client.get("/api/models").json()
        assert doc["total"] == 26
        assert doc["totalPorts"] == 855
        assert doc["bySource"] == {"builtin": 26}

    def test_every_model_reports_its_source(self, client: TestClient) -> None:
        """「这个 48 口谁说的」必须能答 —— 是排查"Excel 没生效"的唯一线索。"""
        doc = client.get("/api/models").json()
        for item in doc["models"]:
            assert item["source"] in ("builtin", "json", "excel")
            assert item["sourceDetail"]

    def test_ports_flag_attaches_bay_detail(self, client: TestClient) -> None:
        lean = client.get("/api/models").json()["models"][0]
        assert "bays" not in lean
        rich = client.get("/api/models", params={"ports": True}).json()["models"][0]
        assert rich["bays"], "带 ports=true 时每组端口明细必须给出来"
        assert {"group", "count", "nominalMbps", "speeds", "role"} <= set(rich["bays"][0])


class TestImportAndEffect:
    def test_import_adds_model_and_keeps_builtin(
        self, client: TestClient, project_name: str
    ) -> None:
        """导入后 27 个型号，内置 26 个一个不少。

        若实现成「替换」而不是「叠加」，另外 25 个型号会全部变成无模板 ——
        台账上大批设备显示「端口未知」，而用户根本不知道自己哪里做错了。
        """
        r = client.post(
            "/api/models/import", json={"project": project_name, "doc": _custom()}
        )
        assert r.status_code == 200
        doc = r.json()
        assert doc["total"] == 27
        assert doc["totalPorts"] == 859
        assert doc["bySource"] == {"builtin": 26, "json": 1}
        assert doc["diff"]["added"] == ["自研汇聚X"]

    def test_reimport_same_content_is_noop(
        self, client: TestClient, project_name: str
    ) -> None:
        """同样内容导两次不该报「又改了」。"""
        client.post("/api/models/import", json={"project": project_name, "doc": _custom()})
        r = client.post(
            "/api/models/import", json={"project": project_name, "doc": _custom()}
        )
        assert r.json()["diff"]["isEmpty"] is True
        assert r.json()["total"] == 27

    def test_import_is_merge_not_replace(
        self, client: TestClient, project_name: str
    ) -> None:
        """第二次导入只带一个新型号，第一个必须还在。

        覆盖式实现会把它抹掉，而它没有任何别的副本 —— 抹了就没了。
        """
        client.post("/api/models/import", json={"project": project_name, "doc": _custom("甲")})
        r = client.post(
            "/api/models/import", json={"project": project_name, "doc": _custom("乙")}
        )
        names = {m["model"] for m in r.json()["models"]}
        assert {"甲", "乙"} <= names
        assert r.json()["total"] == 28

    def test_override_changes_port_count(
        self, client: TestClient, project_name: str
    ) -> None:
        """同名覆盖生效 —— 这是「现场发现内置表写错了能改」的唯一途径。"""
        doc = {
            "schema": 1,
            "models": [
                {
                    "name": "业务核心交换机",
                    "category": "core",
                    "groups": [
                        {
                            "group": "100GE",
                            "panel": 1,
                            "indices": [1, 2],
                            "nominalMbps": 100000,
                            "speeds": [100000],
                            "role": "uplink",
                        }
                    ],
                }
            ],
        }
        r = client.post("/api/models/import", json={"project": project_name, "doc": doc})
        core = [m for m in r.json()["models"] if m["model"] == "业务核心交换机"][0]
        assert core["totalPorts"] == 2
        assert core["source"] == "json"
        assert r.json()["total"] == 26, "覆盖不新增型号数"

    def test_remove_is_explicit(self, client: TestClient, project_name: str) -> None:
        """删内置型号必须显式写 remove，不能因为「库里没有」就消失。"""
        doc = {"schema": 1, "models": [{"name": "分线接入交换机", "remove": True}]}
        r = client.post("/api/models/import", json={"project": project_name, "doc": doc})
        assert r.json()["total"] == 25
        assert r.json()["diff"]["removed"] == ["分线接入交换机"]


class TestExportRoundTrip:
    def test_export_only_user_models_by_default(
        self, client: TestClient, project_name: str
    ) -> None:
        """默认只导项目自己的型号。

        把 26 条内置型号也导出去，再导回来就变成用户数据了 ——
        下次内置表更新时那 26 条再也收不到更新。
        """
        client.post("/api/models/import", json={"project": project_name, "doc": _custom()})
        doc = client.get("/api/models/export", params={"project": project_name}).json()
        assert [m["name"] for m in doc["doc"]["models"]] == ["自研汇聚X"]

    def test_export_all_includes_builtin(
        self, client: TestClient, project_name: str
    ) -> None:
        doc = client.get(
            "/api/models/export", params={"project": project_name, "all": True}
        ).json()
        assert len(doc["doc"]["models"]) == 26

    def test_export_then_import_is_lossless(
        self, client: TestClient, project_name: str
    ) -> None:
        """导出 → 导回，一个口都不能变。"""
        client.post("/api/models/import", json={"project": project_name, "doc": _custom()})
        exported = client.get(
            "/api/models/export", params={"project": project_name, "all": True}
        ).json()["doc"]
        r = client.post("/api/models/import", json={"project": project_name, "doc": exported})
        assert r.json()["diff"]["isEmpty"] is True
        assert r.json()["total"] == 27
        assert r.json()["totalPorts"] == 859


class TestLedgerUsesProjectLibrary:
    def test_ledger_totals_follow_override(
        self, client: TestClient, project_name: str
    ) -> None:
        """把核心交换机改成 2 个口，台账容量必须跟着降。

        ## 这个用例守护的是「接线」

        型号库逻辑本身有单测；这里查的是 `/api/ledger` 到底有没有把它
        传进去。不传的症状是数字完全不变 —— 而「数字不变」这件事
        在界面上看起来和「功能正常」一样。
        """
        base = client.get("/api/ledger").json()["summary"]
        doc = {
            "schema": 1,
            "models": [
                {
                    "name": "业务核心交换机",
                    "category": "core",
                    "groups": [
                        {
                            "group": "100GE",
                            "panel": 1,
                            "indices": [1, 2],
                            "nominalMbps": 100000,
                            "speeds": [100000],
                            "role": "uplink",
                        }
                    ],
                }
            ],
        }
        client.post("/api/models/import", json={"project": project_name, "doc": doc})
        # 覆盖只对「当前项目」生效；把项目切到刚建的那个再查
        after = client.get("/api/ledger", params={"project": project_name}).json()["summary"]
        assert after["capacity"] < base["capacity"]

    def test_over_used_detected_after_shrink(
        self, client: TestClient, project_name: str
    ) -> None:
        """模板被改小到装不下实际连线时，必须报「用量超出模板」而不是悄悄 clamp。"""
        doc = {
            "schema": 1,
            "models": [
                {
                    "name": "业务核心交换机",
                    "category": "core",
                    "groups": [
                        {
                            "group": "100GE",
                            "panel": 1,
                            "indices": [1, 2],
                            "nominalMbps": 100000,
                            "speeds": [100000],
                            "role": "uplink",
                        }
                    ],
                }
            ],
        }
        client.post("/api/models/import", json={"project": project_name, "doc": doc})
        payload = client.get(
            "/api/ledger", params={"project": project_name}
        ).json()
        kinds = {a["kind"] for a in payload.get("attention", [])}
        assert "OVER_USED" in kinds


class TestBadInput:
    def test_missing_project_is_422(self, client: TestClient) -> None:
        r = client.post("/api/models/import", json={"doc": _custom()})
        assert r.status_code == 422
        assert "project" in r.json()["detail"]

    def test_bad_doc_is_422(self, client: TestClient, project_name: str) -> None:
        r = client.post(
            "/api/models/import", json={"project": project_name, "doc": "不是对象"}
        )
        assert r.status_code == 422

    def test_broken_entries_do_not_break_the_library(
        self, client: TestClient, project_name: str
    ) -> None:
        """坏条目只提示、不阻断 —— 型号库错一条不该让整个项目打不开。"""
        doc = {
            "schema": 1,
            "models": [
                {"name": "缺端口的型号"},
                {"name": "坏组", "groups": [{"group": "GE", "indices": [1]}]},
                _custom("好的型号")["models"][0],
            ],
        }
        r = client.post("/api/models/import", json={"project": project_name, "doc": doc})
        assert r.status_code == 200
        assert r.json()["warnings"], "坏条目必须给出提示"
        assert any(m["model"] == "好的型号" for m in r.json()["models"])
