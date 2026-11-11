"""项目持久化的端到端：CLI + API + 数据库文件（真实 SQLite 文件，非内存）。

## 与 `tests/unit/test_db.py` 的分工

unit 那组用内存库跑纯逻辑（快，但不碰文件系统与进程边界）。
这里用**真实文件 + 真 CLI + 真 HTTP**，验证：

1. 数据库文件真的落盘、重开还在（内存库测不出这一点）
2. CLI 五个子命令的打印格式与退出码
3. HTTP 端点的方法/状态码/错误映射
4. CLI 存的项目能被 API 读到，反之亦然 —— **两个入口必须指向同一份数据**，
   否则用户在界面里存的东西命令行看不见，会当 bug 报

## 为什么每个用例都用独立 db 文件

`IDCBOX_DB` 是进程级环境变量，用例之间共享会互相污染版本号。
用 `tmp_path` 给每个用例一个独立库，测出的版本号才是确定的。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.skipif(
    not (
        Path(__file__).resolve().parents[3] / "projects" / "v13" / "intent_full.json"
    ).exists(),
    reason="需要真实项目意图文件（只在完整仓库里跑）",
)

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
INTENT = ROOT / "projects" / "v13" / "intent_full.json"


@pytest.fixture()
def db_file(tmp_path: Path) -> Path:
    return tmp_path / "idcbox.db"


@pytest.fixture()
def cli(db_file: Path, monkeypatch: pytest.MonkeyPatch):
    """返回一个能跑 CLI 的函数（自动带上本用例的 db）。"""
    import sys

    if str(BACKEND) not in sys.path:
        sys.path.insert(0, str(BACKEND))
    from idcbox.cli import main

    def run(*args: str) -> int:
        return main([*args, "--db", str(db_file)])

    return run


@pytest.fixture()
def client(db_file: Path, monkeypatch: pytest.MonkeyPatch):
    """带本用例 db 的 TestClient。"""
    import sys

    if str(BACKEND) not in sys.path:
        sys.path.insert(0, str(BACKEND))
    monkeypatch.setenv("IDCBOX_DB", str(db_file))
    from fastapi.testclient import TestClient

    from idcbox.api.main import create_app

    return TestClient(create_app())


class TestCliPersistence:
    def test_empty_list_message(self, cli, capsys) -> None:
        assert cli("project-list") == 0
        out = capsys.readouterr().out
        assert "还没有任何项目" in out
        assert "project-save" in out  # 告诉用户下一步敲什么

    def test_save_first_version(self, cli, capsys) -> None:
        assert cli("project-save", "--name", "V13", "--intent", str(INTENT)) == 0
        out = capsys.readouterr().out
        assert "已保存" in out
        assert "v1" in out
        assert "949 行" in out
        assert "666 台服务器" in out

    def test_resave_same_content_stays(self, cli, capsys) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        capsys.readouterr()
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        out = capsys.readouterr().out
        assert "内容未变化" in out
        assert "仍是 v1" in out
        assert "--force" in out  # 提示怎么强制留快照

    def test_force_creates_snapshot(self, cli, capsys) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        capsys.readouterr()
        cli("project-save", "--name", "V13", "--intent", str(INTENT), "--force")
        assert "v2" in capsys.readouterr().out

    def test_versions_listing(self, cli, capsys) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT), "--note", "初版")
        capsys.readouterr()
        assert cli("project-versions", "--name", "V13") == 0
        out = capsys.readouterr().out
        assert "共 1 个版本" in out
        assert "初版" in out

    def test_show_counts(self, cli, capsys) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        capsys.readouterr()
        assert cli("project-show", "--name", "V13") == 0
        out = capsys.readouterr().out
        assert "布局 28" in out
        assert "交换机 130" in out
        assert "显式连线 697" in out

    def test_show_export_json(self, cli, tmp_path: Path) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        out = tmp_path / "back.json"
        assert cli("project-show", "--name", "V13", "--out", str(out)) == 0
        assert out.exists()
        doc = json.loads(out.read_text(encoding="utf-8"))
        assert len(doc["switches"]) == 130

    def test_show_export_xlsx(self, cli, tmp_path: Path) -> None:
        """读回的意图能直接导成六段表格继续改 —— 库不是只写不读的墓地。"""
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        out = tmp_path / "back.xlsx"
        assert cli("project-show", "--name", "V13", "--out", str(out)) == 0
        assert out.exists()

    def test_diff_after_change(self, cli, tmp_path: Path, capsys) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT), "--note", "v1")
        mod = json.loads(INTENT.read_text(encoding="utf-8"))
        mod["switches"].append(
            {"instance": "X-1", "model": "业务接入交换机1", "channel": 2, "rack": 26, "u": 40}
        )
        mod_path = tmp_path / "mod.json"
        mod_path.write_text(json.dumps(mod, ensure_ascii=False), encoding="utf-8")
        cli("project-save", "--name", "V13", "--intent", str(mod_path), "--note", "v2")
        capsys.readouterr()

        assert cli("project-diff", "--name", "V13", "--from", "1", "--to", "2") == 0
        out = capsys.readouterr().out
        assert "1 处变化" in out
        assert "【交换机】" in out
        assert "X-1" in out

    def test_diff_identical_versions(self, cli, capsys) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        cli("project-save", "--name", "V13", "--intent", str(INTENT), "--force")
        capsys.readouterr()
        assert cli("project-diff", "--name", "V13", "--from", "1", "--to", "2") == 0
        assert "内容完全相同" in capsys.readouterr().out

    def test_unknown_project_exit(self, cli, capsys) -> None:
        """未知项目要报错而不是静默成功（否则脚本会以为存上了）。"""
        code = cli("project-versions", "--name", "不存在")
        assert code != 0
        assert "项目不存在" in capsys.readouterr().err

    def test_db_file_actually_created(self, cli, db_file: Path) -> None:
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        assert db_file.exists()
        assert db_file.stat().st_size > 0

    def test_data_survives_reopen(self, cli, db_file: Path, capsys) -> None:
        """关掉再开，项目还在 —— 内存库测不出这一点。"""
        cli("project-save", "--name", "V13", "--intent", str(INTENT))
        capsys.readouterr()
        # 新的 main() 调用会新建引擎（相当于重启进程）
        cli("project-list")
        out = capsys.readouterr().out
        assert "V13" in out
        assert "共 1 个项目" in out


class TestCanonicalization:
    """同一意图从不同入口进来，必须得到同一个内容哈希。

    ## 这是真实踩到的 bug（不是假想）

    CLI 传 `Intent` 对象、API 传原始 dict。`intent_to_dict` 会省略等于默认值
    的字段（设备 `uSpan == 2` 时不写），而磁盘 JSON 里显式写了 `"uSpan": 2`。
    结果：v1 由 CLI 存、v2 由 API 存同样内容，版本对比报出 2 条
    「特殊设备被修改」，而两行打印出来一字不差 —— 差异藏在没显示的字段上。
    用户看到空 diff 只会失去信任，所以规范化必须有用例守着。
    """

    def test_intent_and_raw_dict_hash_identically(self) -> None:
        import json

        from idcbox.application.services.projects import canonical_doc
        from idcbox.domain.ports.intent import intent_to_dict, load_intent
        from idcbox.infrastructure.db.repositories import content_hash_of

        raw = json.loads(INTENT.read_text(encoding="utf-8"))
        it = load_intent(INTENT)
        assert content_hash_of(canonical_doc(it)) == content_hash_of(canonical_doc(raw))

    def test_unknown_top_level_keys_preserved(self) -> None:
        """人工加的备注不能被吃掉 —— 向前兼容的底线。"""
        import json

        from idcbox.application.services.projects import canonical_doc

        raw = json.loads(INTENT.read_text(encoding="utf-8"))
        doc = canonical_doc({**raw, "myNote": "人工备注"})
        assert doc["myNote"] == "人工备注"

    def test_canonical_is_idempotent(self) -> None:
        import json

        from idcbox.application.services.projects import canonical_doc

        raw = json.loads(INTENT.read_text(encoding="utf-8"))
        once = canonical_doc(raw)
        twice = canonical_doc(once)
        assert once == twice

    def test_cross_entry_versions_dont_create_false_diff(self, cli, client) -> None:
        """CLI 存 v1 → API 存相同内容 → 不该产生 v2。"""
        import json

        cli("project-save", "--name", "跨入口", "--intent", str(INTENT))
        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        r = client.put("/api/projects/跨入口/intent", json={"doc": doc})
        assert r.json()["isNew"] is False
        assert r.json()["version"] == 1

    def test_api_then_cli_no_false_diff(self, cli, client) -> None:
        import json

        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        client.put("/api/projects/反向/intent", json={"doc": doc})
        cli("project-save", "--name", "反向", "--intent", str(INTENT))
        # 第一次 CLI 会建项目（v1 已由 API 建），内容相同 → 不新建版本
        r = client.get("/api/projects/反向/versions")
        assert r.json()["total"] == 1


class TestApiPersistence:
    def test_empty_projects(self, client) -> None:
        r = client.get("/api/projects")
        assert r.status_code == 200
        assert r.json()["total"] == 0

    def test_create_project(self, client) -> None:
        r = client.post("/api/projects", json={"name": "P1", "siteKey": "xxg"})
        assert r.status_code == 200
        assert r.json()["name"] == "P1"

    def test_empty_name_rejected(self, client) -> None:
        r = client.post("/api/projects", json={"name": "   "})
        assert r.status_code == 422

    def test_save_intent_creates_version(self, client) -> None:
        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        r = client.put("/api/projects/V13/intent", json={"doc": doc, "note": "接口"})
        assert r.status_code == 200
        body = r.json()
        assert body["version"] == 1
        assert body["isNew"] is True
        assert body["summary"]["total"] == 949
        assert body["summary"]["serverCount"] == 666

    def test_save_same_content_not_new(self, client) -> None:
        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        client.put("/api/projects/V13/intent", json={"doc": doc})
        r = client.put("/api/projects/V13/intent", json={"doc": doc})
        assert r.json()["isNew"] is False
        assert r.json()["version"] == 1

    def test_doc_required(self, client) -> None:
        r = client.put("/api/projects/V13/intent", json={"note": "忘了 doc"})
        assert r.status_code == 422
        assert "doc" in r.json()["detail"]

    def test_read_back_intent(self, client) -> None:
        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        client.put("/api/projects/V13/intent", json={"doc": doc})
        r = client.get("/api/projects/V13/intent")
        assert r.status_code == 200
        assert len(r.json()["doc"]["switches"]) == 130

    def test_versions_endpoint(self, client) -> None:
        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        client.put("/api/projects/V13/intent", json={"doc": doc, "note": "a"})
        r = client.get("/api/projects/V13/versions")
        assert r.status_code == 200
        assert r.json()["total"] == 1
        assert r.json()["versions"][0]["note"] == "a"

    def test_diff_endpoint(self, client) -> None:
        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        client.put("/api/projects/V13/intent", json={"doc": doc})
        mod = json.loads(json.dumps(doc))
        mod["routes"].pop()
        client.put("/api/projects/V13/intent", json={"doc": mod})
        r = client.get("/api/projects/V13/diff", params={"from": 1, "to": 2})
        assert r.status_code == 200
        assert r.json()["totalChanges"] == 1

    def test_missing_project_404(self, client) -> None:
        assert client.get("/api/projects/无此项目/versions").status_code == 404
        assert client.get("/api/projects/无此项目/intent").status_code == 404

    def test_missing_version_404(self, client) -> None:
        assert client.get("/api/projects/空项目/intent").status_code == 404


class TestCrossEntryConsistency:
    """CLI 与 API 必须看到同一份数据。"""

    def test_cli_saved_then_api_reads(self, cli, client) -> None:
        cli("project-save", "--name", "共享项目", "--intent", str(INTENT))
        r = client.get("/api/projects")
        names = [p["name"] for p in r.json()["projects"]]
        assert "共享项目" in names

    def test_api_saved_then_cli_reads(self, cli, client, capsys) -> None:
        doc = json.loads(INTENT.read_text(encoding="utf-8"))
        client.put("/api/projects/双向项目/intent", json={"doc": doc})
        cli("project-versions", "--name", "双向项目")
        assert "共 1 个版本" in capsys.readouterr().out
