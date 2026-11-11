"""经验表 JSON 加载器（`rules/len_empirical_v*.json` → `EmpiricalTableData`）。

## 为什么要加载器而不是直接 `json.load` 成 dict

盘上的 JSON 为了人能读，用的是**嵌套**结构：

```json
{"levels": {"L1_cls_cab_path": {"A": {"LC-LC": [{"path": 0, "len": 5, ...}]}},
            "L2_cls_cab":      {"A": {"LC-LC": {"len": 10, ...}}},
            "L3_cab":          {"LC-LC": {"len": 10, ...}}}}
```

而域层查询要 O(1) 命中，用的是**扁平 tuple 键**：

```python
{(cls, cab, path): EmpiricalCell, (cls, cab): EmpiricalCell, cab: EmpiricalCell}
```

加载器负责这层转换，并做 **schema 校验**：宁可加载失败，也不要带着一张坏表去给人报建议。
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Mapping

from ...domain.cable_length.empirical import (
    DEFAULT_MIN_PURITY,
    DEFAULT_MIN_SAMPLE_N,
    EmpiricalCell,
    EmpiricalTableData,
)


class EmpiricalTableError(ValueError):
    """经验表文件不合法。**必须显式失败**，不允许静默降级成空表。"""


def _cell(raw: Mapping[str, Any], *, where: str) -> EmpiricalCell:
    try:
        length_m = int(raw["len"])
        sample_n = int(raw["n"])
        purity = float(raw["purity"])
    except (KeyError, TypeError, ValueError) as exc:
        raise EmpiricalTableError(f"{where}: 缺少或非法的 len/n/purity —— {exc}") from exc
    if length_m <= 0:
        raise EmpiricalTableError(f"{where}: len 必须为正数，得到 {length_m}")
    if not 0.0 <= purity <= 1.0:
        raise EmpiricalTableError(f"{where}: purity 必须在 [0,1]，得到 {purity}")
    dist_raw = raw.get("dist") or {}
    dist: dict[int, int] = {}
    for k, v in dist_raw.items():
        try:
            dist[int(k)] = int(v)
        except (TypeError, ValueError):
            continue  # 分布只用于展示，坏一条不影响主值
    return EmpiricalCell(length_m=length_m, sample_n=sample_n, purity=purity, dist=dist)


def load_empirical_table(path: str | Path) -> EmpiricalTableData:
    """从 JSON 文件加载经验表。任何结构错误都抛 `EmpiricalTableError`。"""
    p = Path(path)
    if not p.exists():
        raise EmpiricalTableError(f"经验表文件不存在：{p}")
    with p.open("r", encoding="utf-8") as f:
        data = json.load(f)

    levels = data.get("levels")
    if not isinstance(levels, dict):
        raise EmpiricalTableError("缺少 levels 段")

    l1: dict[tuple[str, str, int], EmpiricalCell] = {}
    for cls, by_cable in (levels.get("L1_cls_cab_path") or {}).items():
        for cable, entries in (by_cable or {}).items():
            for e in entries:
                l1[(cls, cable, int(e["path"]))] = _cell(e, where=f"L1/{cls}/{cable}/{e['path']}")

    l2: dict[tuple[str, str], EmpiricalCell] = {}
    for cls, by_cable in (levels.get("L2_cls_cab") or {}).items():
        for cable, e in (by_cable or {}).items():
            l2[(cls, cable)] = _cell(e, where=f"L2/{cls}/{cable}")

    l3: dict[str, EmpiricalCell] = {}
    for cable, e in (levels.get("L3_cab") or {}).items():
        l3[cable] = _cell(e, where=f"L3/{cable}")

    gate_raw = data.get("gate") or {}
    gate = (
        int(gate_raw.get("min_sample_n", DEFAULT_MIN_SAMPLE_N)),
        float(gate_raw.get("min_purity", DEFAULT_MIN_PURITY)),
    )

    fm_raw = data.get("force_manual") or {}
    force_classes = tuple(fm_raw.get("classes") or ("C",))
    force_cables = tuple(fm_raw.get("cable_types") or ("MPO",))

    formula_version = str(data.get("formula_version") or "unknown")

    return EmpiricalTableData(
        formula_version=formula_version,
        l1=l1,
        l2=l2,
        l3=l3,
        gate=gate,
        force_manual_classes=force_classes,
        force_manual_cable_types=force_cables,
    )


def table_measured(path: str | Path) -> Mapping[str, Any]:
    """读取文件里自带的实测指标（覆盖率 / LOO 准确率），用于体检报告与 UI 披露。"""
    with Path(path).open("r", encoding="utf-8") as f:
        data = json.load(f)
    return dict(data.get("measured") or {})
