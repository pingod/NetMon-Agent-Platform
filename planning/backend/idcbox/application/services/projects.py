"""项目管理服务：建项目、存意图版本、比版本、导出回文件。

## 分层位置

`application/` 层，依赖方向是 `api → application → domain`，
存储实现（`infrastructure/db/repositories.py`）通过 `ProjectStore` Protocol 注入。
所以这个模块**不 import sqlalchemy**，可以拿内存假实现测。

## 与 `pipeline` / `design` 的分工

- `pipeline`：Excel 文件 → 域对象 → 体检（**读**）
- `design`：意图 → 布线表（**算**）
- `projects`（本模块）：意图文档 ↔ 数据库（**存**）

三者互不调用，由 API/CLI 编排。这样"存"不需要先"算"，
用户可以只保存一个改了一半的意图。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Mapping, Sequence

from ..ports import ProjectStore
from ...domain.ports.docdiff import DIFF_SECTIONS, diff_docs
from ...domain.ports.intent import Intent, intent_from_dict, intent_to_dict


@dataclass(frozen=True, slots=True)
class SavedVersion:
    """一次保存的结果，供 CLI/API 原样展示。"""

    project: str
    version: int
    #: 是否真的产生了新版本（内容未变时为 False）
    is_new: bool
    summary: Mapping[str, Any]
    note: str | None


@dataclass(frozen=True, slots=True)
class VersionInfo:
    version: int
    created_at: str
    created_by: str
    note: str | None
    summary: Mapping[str, Any]


@dataclass(frozen=True, slots=True)
class ProjectSummary:
    id: str
    name: str
    site_key: str
    status: str
    latest_version: int
    version_count: int
    updated_at: str


def _clean_name(name: str) -> str:
    """项目名去掉首尾空白并拒绝空名。

    项目名是用户的唯一标识（`uq_project_name`），空名会让所有项目撞在
    同一个唯一约束上，报出的错误还看不出原因。在服务层先拦掉。
    """
    cleaned = (name or "").strip()
    if not cleaned:
        raise ValueError("项目名不能为空")
    return cleaned


def create_project(
    store: ProjectStore,
    *,
    name: str,
    site_key: str = "",
    source_path: str | None = None,
) -> ProjectSummary:
    project = store.create(
        name=_clean_name(name), site_key=site_key, source_path=source_path
    )
    return _to_summary(project)


def canonical_doc(intent: Intent | Mapping[str, Any]) -> dict:
    """把任意入口的意图规范成**唯一**的 JSON 形态。

    ## 为什么必须规范化（真 bug 的记录）

    同一个意图，CLI 传 `Intent`、API 传原始 dict。`intent_to_dict` 会省略
    等于默认值的字段（设备 `uSpan == 2` 时不写），而磁盘上的 JSON 文件里
    往往显式写了 `"uSpan": 2`。结果两条入口存进库的字节不同、内容哈希不同：

    - v1 由 CLI 存 → 无 `uSpan`
    - v2 由 API 存同样内容 → 有 `uSpan`

    版本对比于是报出 2 条「特殊设备被修改」，而前后两行**打印出来一字不差**
    （差异在没显示的字段上）。用户看到这种空 diff 只会失去信任。

    ## 规范化规则

    已知段一律用 `intent_to_dict(intent_from_dict(...))` 的输出
    （字段顺序、省略规则、默认值全部由域层那一份定义）。
    原始 dict 里的**未知顶层键**原样保留 —— 让人工加的备注、
    未来 schema 的新段不至于被静默吃掉。段内未知字段不保留（已记录在案）。
    """
    it = intent if isinstance(intent, Intent) else intent_from_dict(intent)
    doc = intent_to_dict(it)
    if isinstance(intent, Mapping):
        for key, value in intent.items():
            if key not in doc:
                doc[key] = value
    return doc


def save_intent(
    store: ProjectStore,
    *,
    project_name: str,
    intent: Intent | Mapping[str, Any],
    note: str | None = None,
    site_key: str = "",
    source_path: str | None = None,
    force_new_version: bool = False,
) -> SavedVersion:
    """把意图存成新版本。

    ## 接受 `Intent` 或原始 dict

    CLI 走 `load_intent` 拿到 `Intent`；API 的 PUT body 本来就是 dict。
    都接受可以避免调用方为了"凑类型"做无意义的往返转换
    （dict → Intent → dict 会丢掉未知字段，而未知字段要保留以便向前兼容）。
    """
    doc = canonical_doc(intent)
    project = store.create(
        name=_clean_name(project_name), site_key=site_key, source_path=source_path
    )
    before = store.latest_version(project.id)
    saved = store.save_intent(
        project.id, doc, note=note, force_new_version=force_new_version
    )
    is_new = before is None or saved.version != before.version
    return SavedVersion(
        project=project.name,
        version=saved.version,
        is_new=is_new,
        summary=saved.summary,
        note=note,
    )


def list_projects(store: ProjectStore, *, include_archived: bool = False) -> list[ProjectSummary]:
    return [_to_summary(p) for p in store.list(include_archived=include_archived)]


def default_project_name(store: ProjectStore) -> str | None:
    """挑一个「用户大概想看」的项目名；一个项目都没有时返回 None。

    ## 为什么按 updated_at 而不是名字或 id

    `store.list()` 已按 `updated_at DESC` 排好。用户刚在界面里改完保存的那个
    项目，就是他现在最可能继续看的 —— 按名字排序会把他带到 `A项目`，
    按 id（随机 uuid）排序更是碰运气。

    ## 为什么只要「有版本」的项目

    刚建但还没存过意图的项目点了也没内容，选它只会让用户在生成页看到
    「还没有最新版意图」这种错误。宁可选一个更早但真能看的。

    ## 这是谁的默认值

    `GET /api/design` 不传 `project` 时用它。默认从库读而不是读磁盘文件，
    是为了修掉那条「改完保存、重开页面又是旧内容」的陷阱：
    磁盘上的 `intent.json` 是首次导入的种子，用户后续的改动都在库里。
    """
    for project in store.list():
        if project.latest_version > 0:
            return project.name
    return None


def list_versions(store: ProjectStore, *, project_name: str) -> list[VersionInfo]:
    project = _require(store, project_name)
    return [
        VersionInfo(
            version=v.version,
            created_at=v.created_at,
            created_by=v.created_by,
            note=v.note,
            summary=v.summary,
        )
        for v in store.versions(project.id)
    ]


def load_intent_version(
    store: ProjectStore, *, project_name: str, version: int | None = None
) -> Intent:
    """从库里读回一个版本的意图，转成域对象（可直接喂给 `generate_links`）。"""
    project = _require(store, project_name)
    doc = store.load_doc(project.id, version)
    if doc is None:
        target = "最新版" if version is None else f"v{version}"
        raise ValueError(f"项目「{project_name}」还没有{target}意图")
    return intent_from_dict(doc)


def compare_versions(
    store: ProjectStore, *, project_name: str, from_version: int, to_version: int
) -> dict[str, Any]:
    """比两个版本，返回可 JSON 化的差异。

    ## 为什么不让调用方自己 diff

    两个版本的内容都在库里，diff 逻辑在 `repositories.diff_docs`。
    若让 CLI 与 API 各自调一遍，两边的"哪段参与 diff、按什么键判断同一条"
    会漂移，最后界面上看到的和命令行看到的不是一回事。
    """
    project = _require(store, project_name)
    before = store.load_doc(project.id, from_version)
    after = store.load_doc(project.id, to_version)
    if before is None or after is None:
        raise ValueError(
            f"项目「{project_name}」缺少版本："
            f"v{from_version}={'有' if before else '无'}，"
            f"v{to_version}={'有' if after else '无'}"
        )

    labels = dict(DIFF_SECTIONS)
    sections: list[dict[str, Any]] = []
    for diff in diff_docs(before, after):
        sections.append(
            {
                "section": diff.section,
                "label": labels.get(diff.section, diff.section),
                "added": list(diff.added),
                "removed": list(diff.removed),
                "changed": [{"before": b, "after": a} for b, a in diff.changed],
                "count": diff.count,
                "isEmpty": diff.is_empty,
            }
        )
    return {
        "project": project.name,
        "fromVersion": from_version,
        "toVersion": to_version,
        "sections": sections,
        "totalChanges": sum(s["count"] for s in sections),
    }


def _require(store: ProjectStore, project_name: str) -> Any:
    """取项目，不存在时报出「现有项目有哪些」。

    ## 为什么错误信息要列出现有项目

    用户打错项目名时，只说"不存在"他得自己回去翻列表；
    直接告诉他"现有项目：V13、V14"就一步到位了。
    经验上这种细节能把"用不起来"变成"用起来"。

    用 `ValueError` 而不是自定义异常：CLI 与 API 都只需当成 4xx 处理，
    多一种异常类型只是多一处需要 catch。
    """
    project = store.by_name(_clean_name(project_name))
    if project is not None:
        return project
    names = [p.name for p in store.list(include_archived=True)]
    hint = f"；现有项目：{'、'.join(names)}" if names else "；当前还没有任何项目"
    raise ValueError(f"项目不存在：{project_name}{hint}")


def _to_summary(project: Any) -> ProjectSummary:
    return ProjectSummary(
        id=project.id,
        name=project.name,
        site_key=project.site_key,
        status=project.status,
        latest_version=project.latest_version,
        version_count=project.version_count,
        updated_at=project.updated_at,
    )


__all__ = [
    "ProjectSummary",
    "SavedVersion",
    "VersionInfo",
    "compare_versions",
    "create_project",
    "list_projects",
    "list_versions",
    "load_intent_version",
    "save_intent",
]
