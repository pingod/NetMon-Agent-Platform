"""型号库的应用层服务 —— 「这个项目现在用的是哪套端口模板」。

## 为什么需要一层服务，而不是 API 直接调 domain

型号库有三个来源（内置 / 项目 JSON / Excel），叠加顺序与来源标注都在
`domain/ports/modellib.py`，但**找到那几个文件**是环境相关的事：
项目目录在哪、项目的 `source_path` 指向哪个 Excel、用户是否显式指定了库。

这一层只做「找到文件 → 交给加载器 → 附上出处」，
不碰 openpyxl（那是基础设施的事，由调用方注入 `entries`）。

## 为什么型号库按项目而不是全局一份

同一个客户的两个机房可能用不同批次的设备。项目级库让「一期用 A 型
交换机、二期换 B 型」不需要在两份全局配置之间来回切。
"""

from __future__ import annotations

import json
from dataclasses import replace
from pathlib import Path
from typing import Any

from ...domain.ports.modellib import (
    ModelLibrary,
    diff_libraries,
    library_to_json,
    load_library_json,
    load_model_library,
    merge_entries,
    parse_library_json,
)


def library_path_of(project_dir: Path) -> Path:
    """项目内型号库的约定路径：`<项目目录>/models.json`。"""
    return project_dir / "models.json"


def load_project_library(
    project_dir: Path | None,
    *,
    excel_entries: tuple[Any, ...] | None = None,
    excel_detail: str = "",
) -> ModelLibrary:
    """按「内置 < 项目 models.json < Excel」三层叠出这个项目生效的库。

    `project_dir` 为 None 或目录里没有 `models.json` 时只用内置表 ——
    这不是错误状态：绝大多数项目的设备都在内置 26 个型号里。
    """
    json_path: Path | None = None
    if project_dir is not None:
        candidate = library_path_of(project_dir)
        if candidate.is_file():
            json_path = candidate
    return load_model_library(
        json_path=json_path,
        excel_entries=excel_entries,
        excel_detail=excel_detail,
    )


def library_payload(lib: ModelLibrary, *, with_ports: bool = False) -> dict[str, Any]:
    """型号库的可 JSON 化视图。

    `withPorts=False` 时只给型号级汇总（26 行，界面表格够用）；
    `True` 时附每组的端口明细（上千行，只在"看这个型号到底有哪几个口"时要）。
    """
    models: list[dict[str, Any]] = []
    for spec in lib.all():
        src = lib.source_of(spec.model)
        item: dict[str, Any] = {
            "model": spec.model,
            "category": spec.category,
            "totalPorts": spec.total_ports,
            "source": src.kind if src else "unknown",
            "sourceDetail": src.describe() if src else "",
            "sourceText": spec.source_text,
        }
        if with_ports:
            item["bays"] = [
                {
                    "group": bay.group,
                    "count": bay.count,
                    "nominalMbps": bay.nominal_mbps,
                    "speeds": list(bay.speeds),
                    "role": bay.role,
                }
                for bay in spec.bays
            ]
        models.append(item)
    return {
        "models": models,
        "total": len(models),
        "totalPorts": lib.total_ports(),
        "warnings": list(lib.warnings),
        "bySource": _count_by_source(lib),
    }


def _count_by_source(lib: ModelLibrary) -> dict[str, int]:
    out: dict[str, int] = {}
    for spec in lib.all():
        src = lib.source_of(spec.model)
        kind = src.kind if src else "unknown"
        out[kind] = out.get(kind, 0) + 1
    return out


def save_project_library(
    project_dir: Path, entries: tuple[Any, ...], *, kind: str = "json", detail: str = ""
) -> tuple[ModelLibrary, dict[str, Any]]:
    """把导入的型号**合并**进项目 `models.json`，返回新库与差异。

    ## 为什么是合并而不是覆盖整个文件

    用户从一份 `型号库.xlsx` 导入时，通常只想加那几个新型号。
    覆盖会把之前用 JSON 手工加过的其它型号一起抹掉 ——
    而那些型号没有任何别的副本，抹了就没了。

    所以流程是：读现有文件 → 叠加新条目 → 整份写回。
    """
    path = library_path_of(project_dir)
    before = load_project_library(project_dir)
    doc: dict[str, Any] = {"schema": 1, "models": []}
    if path.is_file():
        try:
            doc = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError:
            doc = {"schema": 1, "models": []}
    existing, _ = parse_library_json(doc)
    merged = merge_entries(before, tuple(existing) + tuple(entries), kind=kind, detail=detail)

    #: 墓碑：`before` 里有、`merged` 里没有的型号 = 被 `remove: true` 删掉的。
    #
    # ## 为什么必须写进文件
    #
    # 删掉的若是**内置**型号，它本来就不在 `models.json` 里 ——
    # 只写「当前有哪些」的话，重新加载时内置表又把它带回来，
    # 用户看到「删成功」但下次打开又出现了。
    # 所以删除要落成一条 `{"name": X, "remove": true}` 记录。
    tombstones = [
        {"name": before.specs[key].model, "remove": True}
        for key in before.specs
        if key not in merged.specs
    ]
    # 墓碑必须排在前面：重读文件时按顺序叠加，若墓碑在后，
    # 会把同一份文件里刚声明过的型号删掉 —— 表现为「导出再导入，
    # 那个型号消失了」，而两个文件都看不出问题。
    doc["models"] = tombstones + (library_to_json(merged, only_kind="json") or {}).get(
        "models", []
    )
    project_dir.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(doc, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    reloaded = load_project_library(project_dir)
    return reloaded, diff_payload(diff_libraries(before, reloaded))


def import_json_library(
    project_dir: Path, payload: Any, *, detail: str = "上传"
) -> tuple[ModelLibrary, dict[str, Any]]:
    """导入 + 落盘。返回的 diff 已经是可 JSON 化的 dict。

    `save_project_library` 就返回 dict 了 —— 这里不能再包一层
    `diff_payload()`，那会在调用方 `diff.added` 上抛
    `AttributeError: 'dict' object has no attribute 'added'`，
    而端点报的是 500 而不是 422，看起来像服务崩了。
    """
    entries, warns = parse_library_json(payload)
    lib, diff = save_project_library(project_dir, entries, kind="json", detail=detail)
    if warns:
        lib = replace(lib, warnings=lib.warnings + warns)
    return lib, diff


def diff_payload(diff: Any) -> dict[str, Any]:
    return {
        "added": list(diff.added),
        "removed": list(diff.removed),
        "changed": list(diff.changed),
        "isEmpty": diff.is_empty,
        "describe": diff.describe(),
    }


def export_json(lib: ModelLibrary, *, only_user: bool = True) -> dict[str, Any]:
    """导出成 `models.json`。

    `only_user=True`（默认）只导项目库自己声明的型号 —— 这样「导出 →
    改 → 导回」不会把 26 条内置型号复制成用户数据。
    """
    return library_to_json(lib, only_kind="json" if only_user else None)


__all__ = [
    "diff_payload",
    "export_json",
    "import_json_library",
    "library_path_of",
    "library_payload",
    "load_project_library",
    "save_project_library",
]
