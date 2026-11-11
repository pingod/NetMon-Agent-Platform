"""持久化层的边界（内存 SQLite，不需要真实源文件）。

## 这组用例守护的五个「差一点就静默出错」的地方

1. **schema 幂等**：`init_schema` 跑两次不能报错、不能产生两个版本行。
   用户每次启动都会调它，报错就是"打不开软件"。
2. **版本号连续且不复用**：回退不删版本，而是把旧内容存成新版本号。
   这样"谁把改坏的东西改回去了"是可查的。若实现成删版本，历史就断了。
3. **同内容不堆版本**：用户会连点保存。每次点击都建版本会让版本历史被灌水。
4. **意图名可空但项目名不可空**：空项目名会让所有项目撞唯一约束，
   而报出的错误看不出原因 —— 服务层要先拦。
5. **删版本只能删最新**：删中间版本会在版本号里留洞，日后无法解释。
"""

from __future__ import annotations

import json

import pytest

from idcbox.application.services.projects import (
    compare_versions,
    create_project,
    list_projects,
    list_versions,
    load_intent_version,
    save_intent,
)
from idcbox.domain.ports.docdiff import diff_docs
from idcbox.infrastructure.db import (
    SCHEMA_VERSION,
    create_db_engine,
    current_version,
    init_schema,
    table_names,
)
from idcbox.infrastructure.db.repositories import (
    ProjectRepository,
    content_hash_of,
    intent_summary_of,
)

#: 一份最小但形状完整的意图文档。
MIN_DOC = {
    "schema": 1,
    "project": "T",
    "site": "",
    "racks": [{"channel": 2, "from": 1, "to": 2, "model": "节点A"}],
    "switches": [{"instance": "交换机-1", "model": "M", "channel": 2, "rack": 1, "u": 40}],
    "devices": [],
    "routes": [{"nodeModel": "节点A", "nic": "万兆1", "switchModel": "M"}],
    "chains": [
        {
            "channel": 2,
            "switchModel": "M",
            "nic": "万兆1",
            "instances": ["交换机-1"],
            "capacity": 44,
        }
    ],
    "explicitLinks": [],
}


@pytest.fixture()
def engine():
    eng = create_db_engine(":memory:")
    init_schema(eng)
    return eng


@pytest.fixture()
def repo(engine):
    return ProjectRepository(engine)


class TestSchema:
    def test_all_tables_created(self, engine) -> None:
        """36 张表：架构 §3.3 的 34 张 + schema_version + intent_document。"""
        names = table_names(engine)
        assert len(names) == 36
        assert "project" in names
        assert "cable" in names
        assert "intent_document" in names
        assert "schema_version" in names

    def test_init_is_idempotent(self, engine) -> None:
        """反复初始化不报错，版本行只有一条。"""
        init_schema(engine)
        init_schema(engine)
        assert current_version(engine) == SCHEMA_VERSION
        with engine.connect() as conn:
            from sqlalchemy import text

            n = conn.execute(text("SELECT COUNT(*) FROM schema_version")).scalar()
        assert n == 1

    def test_version_read_before_init(self) -> None:
        """全新库读版本返回 0 而不是抛异常。"""
        eng = create_db_engine(":memory:")
        assert current_version(eng) == 0

    def test_u_position_overlap_allowed(self, engine) -> None:
        """**U 位重叠必须能插进去** —— 源数据有 13 对重叠。

        若 schema 上有 `UNIQUE(rack_id, u_start)`，真实数据根本导不进来。
        冲突走 `device_conflict` 软约束 + 人工裁决。
        """
        from sqlalchemy import text

        now = "2026-01-01T00:00:00Z"
        with engine.begin() as conn:
            conn.execute(
                text(
                    "INSERT INTO project (id,name,site_key,status,created_at,updated_at)"
                    " VALUES ('p','P','s','draft',:t,:t)"
                ),
                {"t": now},
            )
            conn.execute(
                text(
                    "INSERT INTO room (id,project_id,name,code,col_min,col_max)"
                    " VALUES ('r','p','403','403',23,38)"
                )
            )
            conn.execute(
                text(
                    "INSERT INTO aisle (id,room_id,channel_no,plan_row_n,plan_row_s)"
                    " VALUES ('a','r',1,3,4)"
                )
            )
            conn.execute(
                text(
                    "INSERT INTO rack (id,room_id,aisle_id,seq_no,side,col_index,row_index)"
                    " VALUES ('k','r','a',1,'N',38,3)"
                )
            )
            conn.execute(
                text(
                    "INSERT INTO device_model (id,project_id,name,name_normalized,category)"
                    " VALUES ('m','p','X','X','server')"
                )
            )
            # 同一机柜同一 U 位插两台 —— 必须成功
            for did in ("d1", "d2"):
                conn.execute(
                    text(
                        "INSERT INTO device_instance (id,project_id,model_id,rack_id,"
                        "u_start,u_end,first_seen_at,updated_at)"
                        " VALUES (:d,'p','m','k',37,37,:t,:t)"
                    ),
                    {"d": did, "t": now},
                )
        assert True

    def test_foreign_keys_enforced(self, engine) -> None:
        """外键必须真的生效（pragma 没开的话 REFERENCES 是装饰品）。"""
        from sqlalchemy import text
        from sqlalchemy.exc import IntegrityError

        with pytest.raises(IntegrityError):
            with engine.begin() as conn:
                conn.execute(
                    text(
                        "INSERT INTO rack (id,room_id,aisle_id,seq_no,side,col_index,row_index)"
                        " VALUES ('k','no-such-room','no-such-aisle',1,'N',1,1)"
                    )
                )


class TestContentHash:
    def test_key_order_does_not_matter(self) -> None:
        """同样的内容、不同的键顺序 → 同一哈希（否则版本历史被灌水）。"""
        a = {"b": 1, "a": 2}
        b = {"a": 2, "b": 1}
        assert content_hash_of(a) == content_hash_of(b)

    def test_content_change_changes_hash(self) -> None:
        assert content_hash_of({"a": 1}) != content_hash_of({"a": 2})

    def test_nested_order_does_not_matter(self) -> None:
        a = {"racks": [{"from": 1, "to": 2}]}
        b = {"racks": [{"to": 2, "from": 1}]}
        assert content_hash_of(a) == content_hash_of(b)


class TestSummary:
    def test_counts_sections(self) -> None:
        s = intent_summary_of(MIN_DOC)
        assert s["racks"] == 1
        assert s["switches"] == 1
        assert s["routes"] == 1
        assert s["chains"] == 1
        assert s["total"] == 4

    def test_server_count_from_layout(self) -> None:
        """服务器台数 = 机柜数 × 每柜槽位数。"""
        s = intent_summary_of(MIN_DOC)
        # rack 1..2 共 2 柜 × 默认 8 槽位
        assert s["serverCount"] == 16

    def test_slot_map_override(self) -> None:
        """`slotMap` 覆盖的机柜按它自己的槽位数算（rack11 有 9 台）。"""
        doc = {
            "racks": [
                {
                    "channel": 2,
                    "from": 1,
                    "to": 2,
                    "model": "A",
                    "slotMap": {"2": [0, 1, 2, 3, 4, 5, 6, 7, 7]},
                }
            ]
        }
        s = intent_summary_of(doc)
        assert s["serverCount"] == 8 + 9

    def test_empty_doc(self) -> None:
        s = intent_summary_of({})
        assert s["total"] == 0
        assert s["serverCount"] == 0


class TestProjectRepository:
    def test_create_is_idempotent(self, repo) -> None:
        """同名重复创建返回同一个项目，不报错。"""
        a = repo.create(name="V13")
        b = repo.create(name="V13")
        assert a.id == b.id

    def test_create_records_fields(self, repo) -> None:
        p = repo.create(name="V13", site_key="xxg", source_path="/tmp/a.xlsx")
        assert p.name == "V13"
        assert p.site_key == "xxg"
        assert p.source_path == "/tmp/a.xlsx"
        assert p.status == "draft"
        assert p.latest_version == 0

    def test_list_ordering(self, repo) -> None:
        repo.create(name="A")
        repo.create(name="B")
        names = {p.name for p in repo.list()}
        assert names == {"A", "B"}

    def test_archived_excluded_by_default(self, repo) -> None:
        p = repo.create(name="A")
        repo.set_status(p.id, "archived")
        assert repo.list() == []
        assert len(repo.list(include_archived=True)) == 1

    def test_unknown_status_rejected(self, repo) -> None:
        p = repo.create(name="A")
        with pytest.raises(ValueError, match="未知项目状态"):
            repo.set_status(p.id, "bogus")


class TestVersioning:
    def test_first_save_creates_v1(self, repo) -> None:
        p = repo.create(name="V13")
        v = repo.save_intent(p.id, MIN_DOC)
        assert v.version == 1

    def test_same_content_does_not_bump(self, repo) -> None:
        """用户连点保存不该制造 10 个一样的版本。"""
        p = repo.create(name="V13")
        v1 = repo.save_intent(p.id, MIN_DOC)
        v2 = repo.save_intent(p.id, MIN_DOC)
        assert v1.version == v2.version == 1
        assert len(repo.versions(p.id)) == 1

    def test_force_new_version(self, repo) -> None:
        """里程碑快照：内容没变也建新版本。"""
        p = repo.create(name="V13")
        repo.save_intent(p.id, MIN_DOC)
        v2 = repo.save_intent(p.id, MIN_DOC, force_new_version=True)
        assert v2.version == 2

    def test_content_change_bumps(self, repo) -> None:
        p = repo.create(name="V13")
        repo.save_intent(p.id, MIN_DOC)
        changed = json.loads(json.dumps(MIN_DOC))
        changed["switches"].append(
            {"instance": "交换机-2", "model": "M", "channel": 2, "rack": 2, "u": 40}
        )
        v2 = repo.save_intent(p.id, changed)
        assert v2.version == 2

    def test_rollback_creates_new_version(self, repo) -> None:
        """回退不删版本，而是把旧内容存成新版本号。

        实现成删版本的话，「谁把改坏的东西改回去了」就查不到了。
        """
        p = repo.create(name="V13")
        repo.save_intent(p.id, MIN_DOC, note="v1")
        changed = json.loads(json.dumps(MIN_DOC))
        changed["switches"].append({"instance": "x", "model": "M", "channel": 2, "rack": 2, "u": 40})
        repo.save_intent(p.id, changed, note="v2")
        back = repo.save_intent(p.id, MIN_DOC, note="回退到 v1")
        assert back.version == 3
        assert len(repo.versions(p.id)) == 3
        assert repo.load_doc(p.id)["switches"] == MIN_DOC["switches"]

    def test_versions_newest_first(self, repo) -> None:
        p = repo.create(name="V13")
        repo.save_intent(p.id, MIN_DOC)
        doc2 = json.loads(json.dumps(MIN_DOC))
        doc2["project"] = "T2"
        repo.save_intent(p.id, doc2)
        assert [v.version for v in repo.versions(p.id)] == [2, 1]

    def test_load_specific_version(self, repo) -> None:
        p = repo.create(name="V13")
        repo.save_intent(p.id, MIN_DOC)
        doc2 = json.loads(json.dumps(MIN_DOC))
        doc2["project"] = "T2"
        repo.save_intent(p.id, doc2)
        assert repo.load_doc(p.id, 1)["project"] == "T"
        assert repo.load_doc(p.id, 2)["project"] == "T2"

    def test_delete_only_latest(self, repo) -> None:
        """只能删最新版 —— 删中间会在版本号里留洞。"""
        p = repo.create(name="V13")
        repo.save_intent(p.id, MIN_DOC)
        doc2 = json.loads(json.dumps(MIN_DOC))
        doc2["project"] = "T2"
        repo.save_intent(p.id, doc2)
        assert repo.delete_version(p.id, 1) is False   # 不是最新，拒绝
        assert repo.delete_version(p.id, 2) is True
        assert [v.version for v in repo.versions(p.id)] == [1]

    def test_project_tracks_latest(self, repo) -> None:
        p = repo.create(name="V13")
        repo.save_intent(p.id, MIN_DOC)
        doc2 = json.loads(json.dumps(MIN_DOC))
        doc2["project"] = "T2"
        repo.save_intent(p.id, doc2)
        again = repo.by_name("V13")
        assert again is not None
        assert again.latest_version == 2
        assert again.version_count == 2


class TestDiffDocs:
    def test_added(self) -> None:
        after = json.loads(json.dumps(MIN_DOC))
        after["switches"].append({"instance": "s2", "model": "M", "channel": 2, "rack": 2, "u": 40})
        diffs = {d.section: d for d in diff_docs(MIN_DOC, after)}
        assert len(diffs["switches"].added) == 1
        assert diffs["switches"].removed == ()

    def test_removed(self) -> None:
        before = json.loads(json.dumps(MIN_DOC))
        before["switches"].append({"instance": "s2", "model": "M", "channel": 2, "rack": 2, "u": 40})
        diffs = {d.section: d for d in diff_docs(before, MIN_DOC)}
        assert len(diffs["switches"].removed) == 1

    def test_changed(self) -> None:
        after = json.loads(json.dumps(MIN_DOC))
        after["switches"][0]["u"] = 41
        diffs = {d.section: d for d in diff_docs(MIN_DOC, after)}
        assert len(diffs["switches"].changed) == 1
        before_item, after_item = diffs["switches"].changed[0]
        assert before_item["u"] == 40
        assert after_item["u"] == 41

    def test_reorder_is_not_a_change(self) -> None:
        """**行换位置不算改** —— 用户在 Excel 里排序不该报 100 条差异。"""
        after = json.loads(json.dumps(MIN_DOC))
        after["switches"] = [
            {"instance": "s2", "model": "M", "channel": 2, "rack": 2, "u": 40},
            *MIN_DOC["switches"],
        ]
        before = json.loads(json.dumps(MIN_DOC))
        before["switches"] = [
            *MIN_DOC["switches"],
            {"instance": "s2", "model": "M", "channel": 2, "rack": 2, "u": 40},
        ]
        diffs = {d.section: d for d in diff_docs(before, after)}
        assert diffs["switches"].is_empty

    def test_identical_docs_all_empty(self) -> None:
        assert all(d.is_empty for d in diff_docs(MIN_DOC, MIN_DOC))


class TestServiceLayer:
    def test_empty_project_name_rejected(self, repo) -> None:
        """空项目名会让所有项目撞唯一约束，且报错看不出原因。"""
        with pytest.raises(ValueError, match="项目名不能为空"):
            create_project(repo, name="   ")

    def test_create_and_save(self, repo) -> None:
        create_project(repo, name="V13")
        saved = save_intent(repo, project_name="V13", intent=MIN_DOC, note="首次")
        assert saved.version == 1
        assert saved.is_new is True
        assert saved.summary["total"] == 4

    def test_save_auto_creates_project(self, repo) -> None:
        """保存时项目不存在就自动建 —— 用户不必先 create。"""
        saved = save_intent(repo, project_name="新项目", intent=MIN_DOC)
        assert saved.version == 1
        assert repo.by_name("新项目") is not None

    def test_resave_reports_not_new(self, repo) -> None:
        save_intent(repo, project_name="V13", intent=MIN_DOC)
        again = save_intent(repo, project_name="V13", intent=MIN_DOC)
        assert again.is_new is False

    def test_list_projects(self, repo) -> None:
        create_project(repo, name="A")
        create_project(repo, name="B")
        assert {p.name for p in list_projects(repo)} == {"A", "B"}

    def test_list_versions(self, repo) -> None:
        save_intent(repo, project_name="V13", intent=MIN_DOC, note="v1")
        doc2 = json.loads(json.dumps(MIN_DOC))
        doc2["project"] = "T2"
        save_intent(repo, project_name="V13", intent=doc2, note="v2")
        vs = list_versions(repo, project_name="V13")
        assert [v.version for v in vs] == [2, 1]
        assert vs[0].note == "v2"

    def test_load_intent_version(self, repo) -> None:
        save_intent(repo, project_name="V13", intent=MIN_DOC)
        intent = load_intent_version(repo, project_name="V13")
        assert intent.project == "T"
        assert len(intent.switches) == 1

    def test_load_missing_version_raises(self, repo) -> None:
        create_project(repo, name="V13")
        with pytest.raises(ValueError, match="还没有"):
            load_intent_version(repo, project_name="V13")

    def test_unknown_project_error_lists_existing(self, repo) -> None:
        create_project(repo, name="A")
        with pytest.raises(ValueError, match="现有项目"):
            load_intent_version(repo, project_name="不存在")

    def test_compare_versions(self, repo) -> None:
        save_intent(repo, project_name="V13", intent=MIN_DOC)
        doc2 = json.loads(json.dumps(MIN_DOC))
        doc2["switches"][0]["u"] = 41
        save_intent(repo, project_name="V13", intent=doc2)
        result = compare_versions(repo, project_name="V13", from_version=1, to_version=2)
        assert result["totalChanges"] == 1
        sw = next(s for s in result["sections"] if s["section"] == "switches")
        assert sw["count"] == 1
        assert sw["label"] == "交换机"

    def test_compare_identical_versions(self, repo) -> None:
        save_intent(repo, project_name="V13", intent=MIN_DOC)
        save_intent(repo, project_name="V13", intent=MIN_DOC, force_new_version=True)
        result = compare_versions(repo, project_name="V13", from_version=1, to_version=2)
        assert result["totalChanges"] == 0
