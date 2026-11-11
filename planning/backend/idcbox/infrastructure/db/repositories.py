"""意图文档仓储：版本化保存 + 哈希去重 + 差异对比。

## 为什么意图按"文档"落库，而不是分解进规范模型

见 `db/schema.sql` 文件头的说明。当初的理由是「分解依赖尚未实现的端口分配
流程」—— 那个流程 v0.6 已经做完（`domain/ports/canon.py` +
`infrastructure/db/canon_store.py`，`idcbox decompose` 命令）。

**现在两份并存，各有分工**：
- `intent_document`（本模块）：存「人写的原始意图」，逐版本留历史、可对比、可回退。
  它是**编辑器的工作对象** —— 用户改一格要能立刻看到校验结果，不需要重算 2 万行。
- 规范表（`device_instance` / `port` / `cable` / ...）：存「摊平后的台账」，
  供查询与统计（"全项目还剩多少个 25GE 口"）。由 `idcbox decompose` 从文档重建。

不做成「每次保存就自动分解」：分解要跑完整生成流程（5127 行连线 + 端口分配），
保存一次就多几秒延迟，而用户保存时想看的是「存住了没」。

## 版本语义

- 每次 `save` 都产生一个新版本（除非内容哈希与最新版相同 → 不产生新版本）
- 版本号从 1 连续递增，**不复用**。回退不是删版本，而是把旧版本内容再存一次
  （产生新版本号）—— 这样"谁在什么时候把改坏的东西改回去了"是可查的
- 同一份内容重复保存不会堆版本：实测用户会连点保存按钮
"""

from __future__ import annotations

import hashlib
import json
import uuid
from dataclasses import dataclass
from typing import Any, Sequence

from sqlalchemy import Engine, text

from ...domain.ports.docdiff import DIFF_SECTIONS, SectionDiff, diff_docs
from .session import utcnow_iso

#: 项目状态。`baselined` = 已定版（不再接受随手修改，改要立新版本）。
PROJECT_STATUSES = ("draft", "baselined", "archived")


@dataclass(frozen=True, slots=True)
class Project:
    id: str
    name: str
    site_key: str
    status: str
    source_path: str | None
    note: str | None
    created_at: str
    updated_at: str
    #: 最新意图版本号（没有意图时为 0）
    latest_version: int = 0
    #: 意图版本数
    version_count: int = 0


@dataclass(frozen=True, slots=True)
class IntentVersion:
    id: str
    project_id: str
    version: int
    content_hash: str
    summary: dict[str, Any]
    created_at: str
    created_by: str
    note: str | None


def content_hash_of(doc: dict[str, Any]) -> str:
    """文档内容哈希：规范化 JSON（排序键、无多余空格）后 sha256。

    ## 为什么必须规范化

    `json.dumps` 的键顺序取决于 dict 插入顺序。用户从 Excel 读入与从 JSON
    读入的同内容意图，键顺序不同 —— 不规范化就会存成两个版本，版本历史被灌水。
    """
    canonical = json.dumps(doc, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(canonical.encode("utf-8")).hexdigest()


def intent_summary_of(doc: dict[str, Any]) -> dict[str, Any]:
    """从意图文档里抽一行摘要，供列表页展示。

    ## 为什么要单独存一份摘要

    列表页要显示"949 行意图 / 666 台服务器"，但从 `doc_json` 现算要为每个版本
    解析一遍完整 JSON（几十 KB × 版本数）。存一份小摘要，列表页能直接读。
    """
    def _len(key: str) -> int:
        value = doc.get(key) or []
        return len(value) if isinstance(value, list) else 0

    racks = _len("racks")
    switches = _len("switches")
    devices = _len("devices")
    routes = _len("routes")
    chains = _len("chains")
    links = _len("explicitLinks")
    # 服务器台数 = 各布局段的「机柜数 × 每柜槽位数」，与 `Intent.server_count` 同口径
    servers = 0
    for block in doc.get("racks") or []:
        if not isinstance(block, dict):
            continue
        try:
            lo = int(block.get("from") or 0)
            hi = int(block.get("to") or 0)
        except (TypeError, ValueError):
            continue
        if hi < lo:
            continue
        slots = block.get("slots") or [0, 1, 2, 3, 4, 5, 6, 7]
        slot_map = block.get("slotMap") or {}
        for rack in range(lo, hi + 1):
            # slotMap 覆盖的机柜按它自己的槽位数算（rack7 只有 7 台、rack11 有 9 台）
            override = slot_map.get(str(rack))
            servers += len(override) if override else len(slots)
    return {
        "racks": racks,
        "switches": switches,
        "devices": devices,
        "routes": routes,
        "chains": chains,
        "explicitLinks": links,
        "total": racks + switches + devices + routes + chains + links,
        "serverCount": servers,
    }


class ProjectRepository:
    """项目 + 意图文档的读写。所有写操作走 `Engine.begin()`（自动事务）。"""

    def __init__(self, engine: Engine) -> None:
        self._engine = engine

    # ------------------------------------------------------------ 项目

    def create(
        self,
        *,
        name: str,
        site_key: str = "",
        source_path: str | None = None,
        note: str | None = None,
    ) -> Project:
        """建项目。同名项目已存在时**直接返回已存在的那个**。

        ## 为什么不报错

        CLI `project-add` 与 API POST 都可能被重复调用（用户点两下、脚本重跑）。
        报错会让调用方写一堆"先查再建"的代码；而幂等地返回已有项目，
        语义更接近用户意图（"我要有个叫 V13 的项目"，而不是"我要插入一行"）。
        """
        existing = self.by_name(name)
        if existing is not None:
            return existing

        pid = f"prj_{uuid.uuid4().hex[:12]}"
        now = utcnow_iso()
        with self._engine.begin() as conn:
            conn.execute(
                text(
                    "INSERT INTO project (id, name, site_key, status, source_path, note,"
                    " created_at, updated_at) VALUES (:id, :name, :site, 'draft',"
                    " :src, :note, :at, :at)"
                ),
                {
                    "id": pid,
                    "name": name,
                    "site": site_key,
                    "src": source_path,
                    "note": note,
                    "at": now,
                },
            )
        got = self.by_name(name)
        assert got is not None
        return got

    def by_name(self, name: str) -> Project | None:
        with self._engine.connect() as conn:
            row = conn.execute(
                text(
                    "SELECT p.*, "
                    " (SELECT MAX(version) FROM intent_document d"
                    "   WHERE d.project_id = p.id) AS latest_version, "
                    " (SELECT COUNT(*) FROM intent_document d"
                    "   WHERE d.project_id = p.id) AS version_count "
                    "FROM project p WHERE p.name = :name"
                ),
                {"name": name},
            ).first()
        return _row_to_project(row) if row is not None else None

    def by_id(self, project_id: str) -> Project | None:
        with self._engine.connect() as conn:
            row = conn.execute(
                text(
                    "SELECT p.*, "
                    " (SELECT MAX(version) FROM intent_document d"
                    "   WHERE d.project_id = p.id) AS latest_version, "
                    " (SELECT COUNT(*) FROM intent_document d"
                    "   WHERE d.project_id = p.id) AS version_count "
                    "FROM project p WHERE p.id = :id"
                ),
                {"id": project_id},
            ).first()
        return _row_to_project(row) if row is not None else None

    def list(self, *, include_archived: bool = False) -> list[Project]:
        sql = (
            "SELECT p.*, "
            " (SELECT MAX(version) FROM intent_document d WHERE d.project_id = p.id)"
            "   AS latest_version, "
            " (SELECT COUNT(*) FROM intent_document d WHERE d.project_id = p.id)"
            "   AS version_count "
            "FROM project p "
        )
        if not include_archived:
            sql += "WHERE p.status <> 'archived' "
        sql += "ORDER BY p.updated_at DESC"
        with self._engine.connect() as conn:
            rows = conn.execute(text(sql)).all()
        return [_row_to_project(r) for r in rows]

    def set_status(self, project_id: str, status: str) -> None:
        if status not in PROJECT_STATUSES:
            raise ValueError(f"未知项目状态：{status}，可选 {PROJECT_STATUSES}")
        with self._engine.begin() as conn:
            conn.execute(
                text(
                    "UPDATE project SET status = :s, updated_at = :at WHERE id = :id"
                ),
                {"s": status, "at": utcnow_iso(), "id": project_id},
            )

    # ------------------------------------------------------ 意图版本

    def save_intent(
        self,
        project_id: str,
        doc: dict[str, Any],
        *,
        note: str | None = None,
        created_by: str = "local",
        force_new_version: bool = False,
    ) -> IntentVersion:
        """保存意图文档，返回**实际生效的版本**。

        ## 三种情况

        1. 项目还没有任何版本 → 建 v1
        2. 内容哈希与最新版相同 → **不建新版本**，返回已有的最新版
           （用户连点保存不该制造 10 个一样的版本）
        3. 内容有变化 → 建 version = latest + 1

        `force_new_version=True` 用于"我要留一个里程碑"（如定版、交付前快照），
        即使内容没变也建新版本。
        """
        digest = content_hash_of(doc)
        latest = self.latest_version(project_id)
        if (
            latest is not None
            and latest.content_hash == digest
            and not force_new_version
        ):
            return latest

        next_no = (latest.version if latest is not None else 0) + 1
        vid = f"int_{uuid.uuid4().hex[:12]}"
        now = utcnow_iso()
        summary = intent_summary_of(doc)
        with self._engine.begin() as conn:
            conn.execute(
                text(
                    "INSERT INTO intent_document (id, project_id, version, content_hash,"
                    " doc_json, summary_json, created_at, created_by, note) VALUES"
                    " (:id, :pid, :ver, :hash, :doc, :sum, :at, :by, :note)"
                ),
                {
                    "id": vid,
                    "pid": project_id,
                    "ver": next_no,
                    "hash": digest,
                    "doc": json.dumps(doc, ensure_ascii=False, sort_keys=True),
                    "sum": json.dumps(summary, ensure_ascii=False),
                    "at": now,
                    "by": created_by,
                    "note": note,
                },
            )
            conn.execute(
                text("UPDATE project SET updated_at = :at WHERE id = :pid"),
                {"at": now, "pid": project_id},
            )
        return IntentVersion(
            id=vid,
            project_id=project_id,
            version=next_no,
            content_hash=digest,
            summary=summary,
            created_at=now,
            created_by=created_by,
            note=note,
        )

    def latest_version(self, project_id: str) -> IntentVersion | None:
        with self._engine.connect() as conn:
            row = conn.execute(
                text(
                    "SELECT * FROM intent_document WHERE project_id = :pid"
                    " ORDER BY version DESC LIMIT 1"
                ),
                {"pid": project_id},
            ).first()
        return _row_to_version(row) if row is not None else None

    def version(self, project_id: str, version: int) -> IntentVersion | None:
        with self._engine.connect() as conn:
            row = conn.execute(
                text(
                    "SELECT * FROM intent_document WHERE project_id = :pid"
                    " AND version = :ver"
                ),
                {"pid": project_id, "ver": version},
            ).first()
        return _row_to_version(row) if row is not None else None

    def versions(self, project_id: str) -> list[IntentVersion]:
        """版本列表，**从新到旧**（列表页想先看最近改了什么）。"""
        with self._engine.connect() as conn:
            rows = conn.execute(
                text(
                    "SELECT * FROM intent_document WHERE project_id = :pid"
                    " ORDER BY version DESC"
                ),
                {"pid": project_id},
            ).all()
        return [_row_to_version(r) for r in rows]

    def load_doc(self, project_id: str, version: int | None = None) -> dict[str, Any] | None:
        """读意图文档内容。`version=None` 取最新。"""
        target = self.latest_version(project_id) if version is None else self.version(
            project_id, version
        )
        if target is None:
            return None
        with self._engine.connect() as conn:
            row = conn.execute(
                text("SELECT doc_json FROM intent_document WHERE id = :id"),
                {"id": target.id},
            ).first()
        if row is None:
            return None
        loaded: dict[str, Any] = json.loads(row.doc_json)
        return loaded

    def delete_version(self, project_id: str, version: int) -> bool:
        """删一个版本。**只允许删最新版**。

        删中间版本会让后面的版本号出现空洞，日后回看"v3 去哪了"要翻日志。
        只删尾部就避免了这个问题（相当于 undo 最后一次保存）。
        """
        latest = self.latest_version(project_id)
        if latest is None or latest.version != version:
            return False
        with self._engine.begin() as conn:
            conn.execute(
                text("DELETE FROM intent_document WHERE id = :id"), {"id": latest.id}
            )
        return True


def _row_to_project(row: Any) -> Project:
    return Project(
        id=row.id,
        name=row.name,
        site_key=row.site_key,
        status=row.status,
        source_path=row.source_path,
        note=row.note,
        created_at=row.created_at,
        updated_at=row.updated_at,
        latest_version=int(row.latest_version or 0),
        version_count=int(row.version_count or 0),
    )


def _row_to_version(row: Any) -> IntentVersion:
    raw_summary = row.summary_json
    summary: dict[str, Any] = json.loads(raw_summary) if raw_summary else {}
    return IntentVersion(
        id=row.id,
        project_id=row.project_id,
        version=int(row.version),
        content_hash=row.content_hash,
        summary=summary,
        created_at=row.created_at,
        created_by=row.created_by,
        note=row.note,
    )


__all__ = [
    "DIFF_SECTIONS",
    "PROJECT_STATUSES",
    "IntentVersion",
    "Project",
    "ProjectRepository",
    "SectionDiff",
    "content_hash_of",
    "diff_docs",
    "intent_summary_of",
]
