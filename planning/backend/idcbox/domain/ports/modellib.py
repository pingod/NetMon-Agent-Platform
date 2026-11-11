"""设备型号库 —— 「这台设备一共几个口」的可替换来源。

## 为什么需要这个模块（v0.7）

v0.5/v0.6 的端口模板是 `portspec.py` 里**硬编码的 26 个型号**。
那 26 条本身是对的（逐条核过 `首期设备清单` 的配置原文），但它有两个问题：

1. **现场来一个新型号就要改代码**。客户的机房不会只有 26 种设备，
   而写代码的人也不该是每次加型号都要在的人。
2. **新型号的端口序号规则没人知道**。实测内置表里有几处「不能按 1..n 生成」：
   `带内交换机1` 的万兆口编号是 3~6（1、2 不存在）、
   `计算节点B` 的千兆口是 1 和 3、`网络节点交换机` 48 口只用到前 9 个。
   这些只能靠人看设备面板如实填。

所以这一层把「型号 → 端口组」变成**数据**，三个入口按优先级叠加：

| 优先级 | 来源 | 谁维护 | 典型场景 |
|---|---|---|---|
| 1（最低） | `portspec.SPECS` 内置 26 条 | 本仓库 | 底座，永远在 |
| 2 | 项目内 `models.json` | 项目使用者 | 该项目特有的自研设备 |
| 3（最高） | Excel `型号库` sheet | 现场工程师 | 用 Excel 维护，不用碰代码 |

## 为什么内置表必须当底座而不是「被替换」

用户的型号库里通常只有几个自研设备。若**替换**内置表，另外二十几个
型号会全部变成「无模板」—— 表现是台账上大批设备显示「端口未知」，
而使用者根本不知道自己做错了什么。**叠加**则不会：新库只覆盖它自己声明的型号。

要删掉一个内置型号，得显式写 `{"name": "X", "remove": true}` 或者
在 Excel 里给该型号打 `停用` 标记。删除是**显式动作**，不是副作用。

## Excel 版式（两段，同一 sheet 上下排）

```
型号库
  型号名            类别      U高   规格原文
  业务接入交换机1     switch   1    48*25GE SFP28,8*100GE QSFP28
  ...

  型号名             组名    序号    速率(Mbps)   角色       备注
  业务接入交换机1     25GE    1       25000        downlink
  业务接入交换机1     25GE    2       25000        downlink
  ...
```

第二段的「序号 + 速率」是**同一口径的重复行**：一个口配一种速率就写一行，
同一个序号允许多行（表示同一个物理口协商成多种速率）。
这跟 `PortBay.pairs()` 的展开形式一一对应，可以无损往返。
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field, replace
from pathlib import Path
from typing import Any, Final, Iterable, Literal, Mapping

from ..importing.normalize import norm_name
from .portspec import MODELS, PortBay, PortRole, PortSpec

# ---------------------------------------------------------------------------
# 类型
# ---------------------------------------------------------------------------

ROLE_VALUES: Final[tuple[str, ...]] = (
    "downlink",
    "uplink",
    "peer_link",
    "stack",
    "mgmt",
    "spare",
    "unknown",
)

CATEGORY_VALUES: Final[tuple[str, ...]] = (
    "core",
    "switch",
    "security",
    "storage",
    "server",
    "balancer",
    "other",
)

LIBRARY_SCHEMA_VERSION: Final[int] = 1

SourceKind = Literal["builtin", "json", "excel"]


@dataclass(frozen=True, slots=True)
class ModelSource:
    """一个型号最终是从哪儿来的 —— 「这 48 口谁说的」要能答。"""

    name: str
    kind: SourceKind
    detail: str = ""

    def describe(self) -> str:
        return {
            "builtin": "内置（信息港实测定稿）",
            "json": f"项目库 {self.detail}" if self.detail else "项目库",
            "excel": f"Excel {self.detail}" if self.detail else "Excel 型号库",
        }[self.kind]


@dataclass(frozen=True, slots=True)
class ModelDiff:
    """两次加载之间的差异。用于「我导入的库到底改了什么」。"""

    added: tuple[str, ...] = ()
    removed: tuple[str, ...] = ()
    changed: tuple[str, ...] = ()

    @property
    def is_empty(self) -> bool:
        return not (self.added or self.removed or self.changed)

    def describe(self) -> str:
        if self.is_empty:
            return "型号库无变化"
        bits = []
        if self.added:
            bits.append(f"新增 {len(self.added)}（{'、'.join(self.added[:3])}）")
        if self.removed:
            bits.append(f"移除 {len(self.removed)}（{'、'.join(self.removed[:3])}）")
        if self.changed:
            bits.append(f"改动 {len(self.changed)}（{'、'.join(self.changed[:3])}）")
        return "型号库：" + "；".join(bits)


@dataclass(frozen=True, slots=True)
class ModelLibrary:
    """一个项目当前生效的型号库（不可变快照）。"""

    specs: Mapping[str, PortSpec]
    """型号归一名 → 模板。"""

    sources: Mapping[str, ModelSource]
    """型号归一名 → 出处。"""

    aliases: Mapping[str, str] = field(default_factory=dict)
    """别名归一名 → 主型号归一名。内置表的 `广域网负载均衡` 等在这里。"""

    warnings: tuple[str, ...] = ()
    """加载过程中的问题。**不阻断** —— 型号库错一条不该让整个项目打不开。"""

    def spec_of(self, model: object) -> PortSpec | None:
        """按型号查模板。与 `portspec.spec_of()` 同语义，但走本库。"""
        if not model:
            return None
        key = norm_name(model)
        key = self.aliases.get(key, key)
        return self.specs.get(key)

    def source_of(self, model: object) -> ModelSource | None:
        if not model:
            return None
        key = norm_name(model)
        key = self.aliases.get(key, key)
        return self.sources.get(key)

    def all(self) -> tuple[PortSpec, ...]:
        order = {"core": 0, "switch": 1, "security": 2, "storage": 3, "server": 4, "other": 5}
        return tuple(sorted(self.specs.values(), key=lambda s: (order.get(s.category, 9), s.model)))

    def total_ports(self) -> int:
        return sum(s.total_ports for s in self.specs.values())

    def describe(self) -> str:
        by_kind: dict[str, int] = {}
        for src in self.sources.values():
            by_kind[src.kind] = by_kind.get(src.kind, 0) + 1
        parts = [f"{k} {v}" for k, v in sorted(by_kind.items())]
        text = f"型号库：{len(self.specs)} 个型号（{'、'.join(parts)}），共 {self.total_ports()} 个端口"
        if self.warnings:
            text += f"，{len(self.warnings)} 条提示"
        return text


# ---------------------------------------------------------------------------
# 加载：JSON
# ---------------------------------------------------------------------------
@dataclass(frozen=True, slots=True)
class _Entry:
    """中间态：一个型号的原始声明（JSON/Excel 都先转成它）。"""

    name: str
    category: str = "other"
    u_height: float = 1.0
    source_text: str = ""
    remove: bool = False
    groups: tuple[tuple[str, int, int, int, str], ...] = ()
    """(组名, 面板, 序号, 速率, 角色) 的重复行。"""


def _role_or_unknown(value: object) -> PortRole:
    text = str(value or "").strip().lower()
    return text if text in ROLE_VALUES else "unknown"  # type: ignore[return-value]


def _category_or_other(value: object) -> str:
    text = str(value or "").strip().lower()
    return text if text in CATEGORY_VALUES else "other"


def _bays_of(entry: _Entry) -> tuple[PortBay, ...]:
    """把「(组, 序号, 速率, 角色) 重复行」还原成端口组。

    ## 分组规则

    - 同一个组名、同一个**面板**、同一个角色 → 一组（`indices` 去重排序）
    - `nominal_mbps` 取该组出现次数最多的速率（并列取第一行）
    - `speeds` 取该组全部速率（去重、保序）

    这样 `(25GE, 1, 25000)` 与 `(25GE, 1, 10000)` 两行会合成
    「组 25GE、序号 [1]、标称 25000、允许 (25000, 10000)」——
    正是内置表里 `PortBay.of("25GE", 48, 25_000, (25_000, 10_000))` 的形态。
    """
    buckets: dict[tuple[str, int, str], list[tuple[int, int]]] = {}
    order: list[tuple[str, int, str]] = []
    for group, panel, idx, speed, role in entry.groups:
        k = (group, panel, role)
        if k not in buckets:
            buckets[k] = []
            order.append(k)
        buckets[k].append((idx, speed))

    bays: list[PortBay] = []
    for group, panel, role in order:
        pairs = buckets[(group, panel, role)]
        indices = tuple(sorted({i for i, _ in pairs}))
        speeds: list[int] = []
        for _, sp in pairs:
            if sp not in speeds:
                speeds.append(sp)
        counts: dict[int, int] = {}
        for _, sp in pairs:
            counts[sp] = counts.get(sp, 0) + 1
        nominal = max(speeds, key=lambda sp: (counts[sp], -speeds.index(sp)))
        bays.append(
            PortBay(
                group=group,
                indices=indices,
                nominal_mbps=nominal,
                speeds=tuple(speeds),
                role=role,  # type: ignore[arg-type]
                panel=panel,
            )
        )
    return tuple(bays)


def _entry_to_spec(entry: _Entry) -> PortSpec:
    return PortSpec(
        model=entry.name,
        bays=_bays_of(entry),
        category=entry.category,
        source_text=entry.source_text,
    )


def parse_library_json(payload: object) -> tuple[tuple[_Entry, ...], tuple[str, ...]]:
    """解析 `models.json`，返回 (条目, 提示)。

    ## 格式

    ```json
    {
      "schema": 1,
      "models": [
        {"name": "自研交换机X", "category": "switch", "uHeight": 1,
         "sourceText": "48*25GE",
         "groups": [{"group": "25GE", "indices": [1,2,3], "nominalMbps": 25000,
                     "speeds": [25000, 10000], "role": "downlink"}]},
        {"name": "广域网负载均衡", "remove": true}
      ]
    }
    ```

    `groups` 也接受 `PortBay.of` 那种简写：`{"group": "GE", "count": 4,
    "nominalMbps": 1000}`（等价于 `indices: [1,2,3,4]`）。
    """
    warnings: list[str] = []
    if not isinstance(payload, dict):
        return (), ("型号库 JSON 顶层必须是对象",)
    schema = payload.get("schema", LIBRARY_SCHEMA_VERSION)
    if schema != LIBRARY_SCHEMA_VERSION:
        warnings.append(f"型号库格式版本 {schema} 与本程序支持的 {LIBRARY_SCHEMA_VERSION} 不同，已尽量解析")

    raw_models = payload.get("models")
    if not isinstance(raw_models, list):
        return (), warnings + ("型号库缺少 models 数组",)

    out: list[_Entry] = []
    for i, item in enumerate(raw_models):
        if not isinstance(item, dict):
            warnings.append(f"第 {i + 1} 个型号不是对象，已跳过")
            continue
        name = str(item.get("name") or "").strip()
        if not name:
            warnings.append(f"第 {i + 1} 个型号缺少 name，已跳过")
            continue
        remove = bool(item.get("remove"))
        groups: list[tuple[str, int, int, str]] = []
        raw_groups = item.get("groups") or []
        if not isinstance(raw_groups, list):
            warnings.append(f"型号「{name}」的 groups 不是数组，已忽略")
            raw_groups = []
        for g in raw_groups:
            if not isinstance(g, dict):
                warnings.append(f"型号「{name}」有非对象的端口组，已跳过")
                continue
            grp = str(g.get("group") or "").strip()
            if not grp:
                warnings.append(f"型号「{name}」有端口组缺少 group，已跳过")
                continue
            nominal = g.get("nominalMbps")
            if not isinstance(nominal, int) or nominal <= 0:
                warnings.append(f"型号「{name}」端口组 {grp} 的 nominalMbps 非法，已跳过")
                continue
            speeds_raw = g.get("speeds")
            speeds = (
                tuple(int(s) for s in speeds_raw if isinstance(s, int))
                if isinstance(speeds_raw, list)
                else (nominal,)
            )
            indices_raw = g.get("indices")
            count_raw = g.get("count")
            if isinstance(indices_raw, list) and indices_raw:
                indices = tuple(int(x) for x in indices_raw if isinstance(x, int))
            elif isinstance(count_raw, int) and count_raw > 0:
                indices = tuple(range(1, count_raw + 1))
            else:
                warnings.append(f"型号「{name}」端口组 {grp} 既没有 indices 也没有 count，已跳过")
                continue
            role = _role_or_unknown(g.get("role"))
            panel_raw = g.get("panel")
            panel = panel_raw if isinstance(panel_raw, int) and panel_raw > 0 else 0
            for idx in indices:
                for sp in speeds or (nominal,):
                    groups.append((grp, panel, idx, int(sp), role))
        u_height = item.get("uHeight")
        out.append(
            _Entry(
                name=name,
                category=_category_or_other(item.get("category")),
                u_height=float(u_height) if isinstance(u_height, (int, float)) else 1.0,
                source_text=str(item.get("sourceText") or ""),
                remove=remove,
                groups=tuple(groups),
            )
        )
    return tuple(out), tuple(warnings)


def load_library_json(path: str | Path) -> tuple[tuple[_Entry, ...], tuple[str, ...]]:
    p = Path(path)
    try:
        payload = json.loads(p.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return (), (f"型号库文件不存在：{p}",)
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        return (), (f"型号库文件解析失败（{p.name}）：{exc}",)
    return parse_library_json(payload)


# ---------------------------------------------------------------------------
# 加载：叠加
# ---------------------------------------------------------------------------
def free_bays_of(spec: PortSpec, port_names: Iterable[str]) -> tuple[tuple[str, int, int], ...]:
    """按模板算空闲口 —— 与 `portspec.free_bays()` 同语义，但直接吃 spec。

    ## 为什么不直接调 `portspec.free_bays(model, names)`

    那个函数内部会重新 `spec_of(model)` 走一遍**内置表**。型号来自
    项目库或 Excel 时，它查不到 → 返回空 → 界面上这台设备显示
    「空闲 0 个口」，而它其实有几十个空口。这类错不会报错、不会变红，
    只是数字永远是 0。
    """
    used = {spec.bay_key(name) for name in port_names}
    return tuple(slot for slot in spec.slots() if slot not in used)


def _base_library() -> ModelLibrary:
    """内置 26 条当底座。"""
    from .portspec import _ALIASES  # 内部别名表，只在这里用
    specs = {norm_name(s.model): s for s in MODELS}
    sources = {k: ModelSource(name=k, kind="builtin") for k in specs}
    return ModelLibrary(specs=specs, sources=sources, aliases=dict(_ALIASES))


def merge_entries(
    base: ModelLibrary, entries: Iterable[_Entry], *, kind: SourceKind, detail: str = ""
) -> ModelLibrary:
    """把一批条目叠到库上。同名覆盖，`remove: true` 删掉。"""
    specs = dict(base.specs)
    sources = dict(base.sources)
    warnings = list(base.warnings)
    for entry in entries:
        key = norm_name(entry.name)
        if entry.remove:
            if key in specs:
                specs.pop(key)
                sources.pop(key, None)
            continue
        if not entry.groups:
            warnings.append(f"型号「{entry.name}」没有端口组，已跳过（至少要有 1 个口）")
            continue
        spec = _entry_to_spec(entry)
        specs[key] = spec
        sources[key] = ModelSource(name=entry.name, kind=kind, detail=detail)
    return ModelLibrary(specs=specs, sources=sources, aliases=dict(base.aliases), warnings=tuple(warnings))


def load_model_library(
    *,
    json_path: str | Path | None = None,
    excel_entries: Iterable[_Entry] | None = None,
    excel_detail: str = "",
) -> ModelLibrary:
    """按优先级叠加出当前生效的型号库。

    ## 为什么顺序不可反

    Excel（现场工程师维护）必须压过 JSON（项目使用者维护），
    JSON 必须压过内置。反过来的话，现场改了半天 Excel 发现没生效 ——
    而界面上不会报错，只是数字不动。
    """
    lib = _base_library()
    if json_path:
        entries, warns = load_library_json(json_path)
        if warns:
            lib = replace(lib, warnings=lib.warnings + warns)
        if entries:
            lib = merge_entries(lib, entries, kind="json", detail=str(Path(json_path).name))
    if excel_entries:
        lib = merge_entries(lib, excel_entries, kind="excel", detail=excel_detail)
    return lib


# ---------------------------------------------------------------------------
# 导出（往返用）
# ---------------------------------------------------------------------------
def _bay_panels(spec: PortSpec) -> tuple[tuple[PortBay, int], ...]:
    """给每个 bay 标上它的面板号 —— 这是往返无损的关键。

    内置表的 bay 不带 `panel`（0），面板号是 `slots()` 里数出来的。
    导出时必须把这个数**算出来写死**，否则再导入时两块同规格板会合并
    （实测 `业务核心交换机` 156 口 → 108 口）。
    """
    out: list[tuple[PortBay, int]] = []
    seen: dict[str, int] = {}
    for bay in spec.bays:
        if bay.panel:
            panel = bay.panel
            seen[bay.group] = max(seen.get(bay.group, 0), panel)
        else:
            panel = 1
            if bay.group.endswith("GE"):
                panel = seen.get(bay.group, 0) + 1
            seen[bay.group] = panel
        out.append((bay, panel))
    return tuple(out)


def library_to_json(lib: ModelLibrary, *, only_kind: SourceKind | None = None) -> dict[str, Any]:
    """把库导成 `models.json` 的结构。

    `only_kind="json"` 时只导项目库自己声明的型号（用于「导出当前项目库」），
    这样再导入一次不会把 26 条内置型号复制成用户数据。
    """
    models: list[dict[str, Any]] = []
    for spec in lib.all():
        key = norm_name(spec.model)
        src = lib.sources.get(key)
        if only_kind and (src is None or src.kind != only_kind):
            continue
        groups = []
        for bay, panel in _bay_panels(spec):
            pairs = bay.pairs()
            indices = sorted({i for i, _ in pairs})
            speeds = []
            for _, sp in pairs:
                if sp not in speeds:
                    speeds.append(sp)
            groups.append(
                {
                    "group": bay.group,
                    "panel": panel,
                    "indices": indices,
                    "nominalMbps": bay.nominal_mbps,
                    "speeds": speeds,
                    "role": bay.role,
                }
            )
        models.append(
            {
                "name": spec.model,
                "category": spec.category,
                "sourceText": spec.source_text,
                "groups": groups,
            }
        )
    return {"schema": LIBRARY_SCHEMA_VERSION, "models": models}


def diff_libraries(before: ModelLibrary, after: ModelLibrary) -> ModelDiff:
    """比对两个库：新增 / 移除 / 端口规格变了。"""
    b, a = before.specs, after.specs
    added = tuple(sorted(k for k in a if k not in b))
    removed = tuple(sorted(k for k in b if k not in a))
    changed: list[str] = []
    for k in sorted(set(a) & set(b)):
        if _shape(a[k]) != _shape(b[k]) or a[k].category != b[k].category:
            changed.append(k)
    return ModelDiff(added=added, removed=removed, changed=tuple(changed))


def _shape(spec: PortSpec) -> tuple[tuple[object, ...], ...]:
    """型号的可比较形状：端口数 + 每组的身份集合。

    ## 为什么不直接比 `bays`

    内置表的 bay 写 `panel=0`（「按出现顺序自动编号」），
    型号库里写的是算出来的 `panel=1`。两者对**端口身份**是等价的
    （`slots()` 完全一样），直接比 `bays` 会把 26 个型号全报成「改动」——
    用户导出一个库再导回来看到「改动 26」，只会以为导出坏了。

    所以比较口径定在「展开后的端口身份 + 角色」这一层，
    也就是台账实际用到的那一层。
    """
    return tuple(
        (group, panel, idx, role) for (group, panel, idx), role in _slot_roles(spec)
    )


def _slot_roles(spec: PortSpec) -> tuple[tuple[tuple[str, int, int], str], ...]:
    """把 `slots()` 与每个槽的角色配起来。"""
    role_of: dict[tuple[str, int], str] = {}
    for bay, panel in _bay_panels(spec):
        role_of[(bay.group, panel)] = bay.role
    out: list[tuple[tuple[str, int, int], str]] = []
    for slot in spec.slots():
        out.append((slot, role_of.get((slot[0], slot[1]), "unknown")))
    return tuple(out)


def entry_from_rows(
    name: str,
    rows: Iterable[tuple[str, int, int, int, str]],
    *,
    category: str = "other",
    source_text: str = "",
    remove: bool = False,
) -> _Entry:
    """从「(组名, 面板, 序号, 速率, 角色) 行」造一个条目 —— 供 Excel/CSV 导入用。"""
    return _Entry(
        name=name,
        category=_category_or_other(category),
        source_text=source_text,
        remove=remove,
        groups=tuple(rows),
    )


__all__ = [
    "CATEGORY_VALUES",
    "entry_from_rows",
    "LIBRARY_SCHEMA_VERSION",
    "ModelDiff",
    "ModelLibrary",
    "ModelSource",
    "ROLE_VALUES",
    "diff_libraries",
    "free_bays_of",
    "library_to_json",
    "load_library_json",
    "load_model_library",
    "merge_entries",
    "parse_library_json",
]
