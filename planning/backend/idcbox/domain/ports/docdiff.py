"""意图文档的版本差异（纯函数，零 IO）。

## 为什么在域层

`diff` 只做两件事：按业务键把两份 dict 索引，再做集合运算。
它不需要数据库、不需要文件、不知道 SQL —— 这是**纯域逻辑**。
放在 `infrastructure/db/` 里会逼应用层反向依赖基础设施
（违反 `api → application → domain` 单向契约），虽然 import-linter
的 layers 合同默认只查被点名的包，但契约精神如此。

## 为什么用「业务键集合」而不是逐行文本 diff

用户在表格里改一行时，行的位置会变（Excel 排序、插入新行）。
文本 diff 会报"这行删了、那行加了"，而实际是同一条记录换了位置。
按业务键做集合运算，报的才是"哪台交换机被删了"。

## 顺序保持

`added` / `removed` 按在 `after` / `before` 里的原顺序输出 ——
用户看差异时希望能顺着原表看，而不是按字母序。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any


@dataclass(frozen=True, slots=True)
class SectionDiff:
    """某一段（布局/交换机/路由…）的版本差异。"""

    section: str
    added: tuple[dict[str, Any], ...]
    removed: tuple[dict[str, Any], ...]
    changed: tuple[tuple[dict[str, Any], dict[str, Any]], ...]

    @property
    def is_empty(self) -> bool:
        return not self.added and not self.removed and not self.changed

    @property
    def count(self) -> int:
        return len(self.added) + len(self.removed) + len(self.changed)


#: 参与 diff 的六段。键名与意图 JSON 一致。
DIFF_SECTIONS: tuple[tuple[str, str], ...] = (
    ("racks", "布局"),
    ("switches", "交换机"),
    ("devices", "特殊设备"),
    ("routes", "路由"),
    ("chains", "交换机组"),
    ("explicitLinks", "显式连线"),
)

#: 每段用来判断"是不是同一条记录"的键。顺序即优先级。
#:
#: ## 这些键是怎么定的
#:
#: 取「用户改了它就算另一条记录」的字段组合：
#: - 交换机按 `instance`（名字是身份，位置变了还是那台机器）
#: - 路由按 `(nodeModel, nic, switchModel)`（三元组即一条路由规则）
#: - 显式连线按两端 `(aDevice, aPort, bDevice, bPort)`（一个插口只能接一根）
SECTION_KEYS: dict[str, tuple[str, ...]] = {
    "racks": ("channel", "from", "to", "model"),
    "switches": ("instance",),
    "devices": ("instance",),
    "routes": ("nodeModel", "nic", "switchModel"),
    "chains": ("channel", "switchModel", "nic"),
    "explicitLinks": ("aDevice", "aPort", "bDevice", "bPort"),
}


def diff_docs(
    before: dict[str, Any], after: dict[str, Any]
) -> tuple[SectionDiff, ...]:
    """两份意图文档的逐段差异。"""
    out: list[SectionDiff] = []
    for section, _label in DIFF_SECTIONS:
        b_items = _indexed(before.get(section), section)
        a_items = _indexed(after.get(section), section)
        added = tuple(a_items[k] for k in a_items if k not in b_items)
        removed = tuple(b_items[k] for k in b_items if k not in a_items)
        changed = tuple(
            (b_items[k], a_items[k])
            for k in a_items
            if k in b_items and b_items[k] != a_items[k]
        )
        out.append(SectionDiff(section, added, removed, changed))
    return tuple(out)


def _indexed(items: Any, section: str) -> dict[str, dict[str, Any]]:
    """把一段的行按业务键索引。键缺失时用序号兜底（不丢行）。"""
    if not isinstance(items, list):
        return {}
    keys = SECTION_KEYS.get(section, ())
    out: dict[str, dict[str, Any]] = {}
    for idx, item in enumerate(items):
        if not isinstance(item, dict):
            continue
        parts = [str(item.get(key) or "") for key in keys]
        label = "|".join(parts) if parts else f"#{idx}"
        # 完全相同的业务键在同一段里出现多次 → 加序号区分（否则后者覆盖前者，
        # 差异会「少报一条」。这在显式连线里可能发生：同一对设备同一对端口
        # 理论上不该出现两次，但源数据不该由 diff 层做假设。）
        if label in out:
            n = 2
            while f"{label}#{n}" in out:
                n += 1
            label = f"{label}#{n}"
        out[label] = item
    return out


__all__ = [
    "DIFF_SECTIONS",
    "SECTION_KEYS",
    "SectionDiff",
    "diff_docs",
]
