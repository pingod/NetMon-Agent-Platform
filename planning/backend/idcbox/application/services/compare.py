"""跨版本/跨项目对比服务 —— 把 `stockdiff` 与 `projects` 两套 diff 收在一处。

## 为什么要一个服务层（而不是 API 直接调 domain）

对比有三种对象，用户**不需要先分清**：
1. 两个**项目版本**（意图 949 行，同一项目的演进）、
2. 两个**存量 Excel sheet**（交付物 5127 行，同一项目的不同文件）、
3. 两个**项目的最新版**（不同项目之间，例如"两个机房"或"一期 vs 二期"）。

界面上它就是一个"对比"按钮。所以服务层要做的核心工作不是算 diff，
而是**认出用户在比什么，并给出该看的那几个数字**。

## 决定性实测：`布线表（旧）` 就是上一版原件

| 比较对象 | 结果 |
|---|---|
| V12 布线表 ↔ V13 布线表（旧） | **逐行完全一致（5131 = 5131，零差异）** |
| V13 布线表（旧） ↔ V13 布线表 | 新增 3 / 缺失 3 / 修改 3，实例名填充率 14%→100% |
| V11 设计院填写长度 ↔ V12 布线表 | 新增 3 / 缺失 3 / 修改 3，长度填充率 99.9%→12.5% |
| V8 连线表 ↔ V13 连线表(不含长度) | 逐行完全一致（5110 = 5110，零差异） |

**所以 V13 那次改动的真实内容不是"改接线"，是三件事**：
①把 V12 的布线表原样归档成「布线表（旧）」②补齐「本端设备名称」列（14%→100%）
③删掉 3 条连线（5131→5127）。

这三件事用"改了 3 行"表达会严重失真 —— 补名字那一列按行比是 4400 条"修改"，
但它不是接线变化。所以本服务把**填充率变化**作为与"行增删改"并列的一级结论。
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any, Sequence

from ..ports import StockReader, StockSheet
from ...domain.exporting.stockdiff import (
    StockDiff,
    StockTable,
    compare_devices,
    compare_stocks,
    device_payload,
    diff_payload,
    parse_length,
)

#: 仓库里历史版本的默认目录名。
DEFAULT_STOCK_DIR = "信息港部署上架"

#: 在 `信息港部署上架/` 里认"这是设备上架规划表"的文件名特征。
PLAN_FILE_MARKERS: tuple[str, ...] = ("设备上架", "设备上架点位图")

#: 读表缓存键：(路径, mtime, size, sheet)。文件被替换后 mtime 变，自然失效。
_CACHE: dict[tuple[str, float, int, str], StockTable] = {}


@dataclass(frozen=True, slots=True)
class StockFile:
    """一个可选的对比对象（某个 Excel 的某个 sheet）。"""

    path: str
    name: str
    sheet: str
    rows: int
    cols: int
    priority: int

    @property
    def label(self) -> str:
        return f"{self.name} · {self.sheet}"

    def as_dict(self) -> dict[str, Any]:
        return {
            "path": self.path,
            "name": self.name,
            "sheet": self.sheet,
            "label": self.label,
            "rows": self.rows,
            "cols": self.cols,
            "priority": self.priority,
        }


#: 默认的存量表读取实现，由组合根（API / CLI）在启动时注入。
#:
#: ## 为什么用模块级注册表而不是函数体内 import
#:
#: 应用层不得依赖基础设施，这是 `lint-imports` 的第 4 条契约。
#: 最初 `compare.py` 直接 `from ...infrastructure.xlsx.stock_reader import
#: read_stock_table`，契约报 BROKEN；改成函数体内 import 想绕过，
#: **契约仍然报 BROKEN** —— import-linter 会扫描函数体内的 import 语句，
#: 它拦的正是"延迟导入"这种假解耦。
#:
#: 所以真正的解法是把装配点挪到组合根：`api/main.py` 启动时调用
#: `set_stock_reader(XlsxStockReader())` 注册一次。应用层只在
#: 调用时从注册表取，代码里不出现任何 infrastructure 的名字。
_DEFAULT_READER: StockReader | None = None


def set_stock_reader(reader: StockReader) -> None:
    """注册默认的存量表读取实现（由组合根调用一次）。"""
    global _DEFAULT_READER
    _DEFAULT_READER = reader


def _resolve_reader(reader: StockReader | None) -> StockReader:
    """拿到一个读取实现：显式传入优先，其次用注册表里的默认值。"""
    if reader is not None:
        return reader
    if _DEFAULT_READER is None:
        raise RuntimeError(
            "还没有注册存量表读取实现。"
            "请在启动时调用 compare.set_stock_reader(XlsxStockReader())，"
            "或给本次调用显式传 reader 参数。"
        )
    return _DEFAULT_READER


def stock_dir(root: Path | None = None) -> Path:
    """历史版本目录。找不到就返回默认路径（调用方自己判断存在性）。"""
    base = Path(root) if root else Path.cwd()
    for candidate in (base / DEFAULT_STOCK_DIR, base.parent / DEFAULT_STOCK_DIR):
        if candidate.is_dir():
            return candidate
    return base / DEFAULT_STOCK_DIR


def list_stock_files(
    root: Path | None = None, *, reader: StockReader | None = None
) -> tuple[StockFile, ...]:
    """列出目录里所有"可比的" Excel sheet。

    ## 为什么只保留设备上架规划表

    目录里还有 `安全产品型号参数.xlsx`、`甘肃信创云-到货829根耗材盘点.xlsx`、
    `信息港项目计划表.xlsx`、`政务外网区交换机连线表.xlsx`、`11+12冷通道（示例）.xlsx`。
    前三个没有连线表（扫出来 `is_cabling` 全是 False，自动排除）。
    政务外网区与冷通道示例**有**连线表，但它们是**别的项目**（不同机房/不同规模），
    混进"版本对比"列表会让用户误以为它们是同一份规划的历史版本。

    所以按文件名特征过滤，只留 `设备上架` 系；别的项目另有一个入口（见 API 的 `path` 参数）。
    """
    d = stock_dir(root)
    if not d.is_dir():
        return ()
    out: list[StockFile] = []
    for path in sorted(d.glob("*.xlsx")):
        if not any(marker in path.name for marker in PLAN_FILE_MARKERS):
            continue
        try:
            sheets: Sequence[StockSheet] = _resolve_reader(reader).scan(str(path))
        except Exception:
            continue
        for s in sheets:
            out.append(
                StockFile(
                    path=str(path),
                    name=path.stem,
                    sheet=s.name,
                    rows=s.rows,
                    cols=s.cols,
                    priority=s.priority,
                )
            )
    # 默认顺序：当前版本在前（priority 小），同 priority 按文件名倒序（V13 在 V12 前）
    out.sort(key=lambda f: (f.priority, _version_key(f.name)), reverse=False)
    return tuple(out)


def _version_key(name: str) -> tuple[int, str]:
    """从文件名里抽 `V13` 这种版本号，用于排序时"新版在前"。

    没有版本号的（如 `11+12冷通道`）排在最后。返回负数让 `sorted` 升序时
    大版本号在前 —— 这样 `list_stock_files` 不用再单独反转。
    """
    import re

    m = re.search(r"[Vv](\d+)", name)
    if m:
        return (-int(m.group(1)), name)
    return (0, name)


def load_stock(
    path: str | Path,
    *,
    sheet: str,
    label: str = "",
    reader: StockReader | None = None,
) -> StockTable:
    """读一份存量表（带 mtime 缓存）。

    为什么缓存：一次对比要读两个文件、每个 5000 行，实测约 1.2s。
    界面上"换一个对比对象"是高频操作，每次重读会让点击有明显延迟。
    缓存键含 mtime 与 size，文件被 Excel 重新保存后自动失效。
    """
    p = Path(path)
    if not p.is_file():
        raise FileNotFoundError(f"找不到文件：{p}")
    stat = p.stat()
    key = (str(p.resolve()), stat.st_mtime, stat.st_size, sheet)
    cached = _CACHE.get(key)
    if cached is not None:
        return cached
    table = _resolve_reader(reader).read(
        str(p), sheet=sheet, label=label or f"{p.stem} · {sheet}"
    )
    # 只留最近 8 份，避免长时间运行累积（每份 5000 行 × 14 字段约 3MB）
    if len(_CACHE) > 8:
        _CACHE.clear()
    _CACHE[key] = table
    return table


def compare_files(
    left: tuple[str, str],
    right: tuple[str, str],
    *,
    limit: int = 50,
    sample: int = 8,
    reader: StockReader | None = None,
) -> dict[str, Any]:
    """比两个 (路径, sheet)。返回可直接 JSON 化的结果。"""
    left_table = load_stock(left[0], sheet=left[1], reader=reader)
    right_table = load_stock(right[0], sheet=right[1], reader=reader)
    diff = compare_stocks(left_table, right_table, limit=limit)
    payload = diff_payload(diff, sample=sample)
    payload["leftFile"] = str(left[0])
    payload["rightFile"] = str(right[0])
    payload["leftSheet"] = left[1]
    payload["rightSheet"] = right[1]
    # 长度差异单独给「分母」：只有两侧都有值的行才该比。没有分母，
    # 用户看到"0 条长度变化"会以为是好东西，实际是两边都没填。
    both_len = _both_length_rows(left_table, right_table, diff)
    payload["lengthBaseline"] = both_len
    # 设备级聚合。逐行 diff 只能回答"哪个格子变了"；用户真正问的是
    # "哪几台设备干了什么"。V12→V13 逐行是 4886 新增 / 4890 删除，
    # 设备级是 56 台挪了 U 位 + 2 台新增 + 54 台改了端口 —— 后者才是
    # 施工队能读懂的说法。
    payload["byDevice"] = device_payload(
        compare_devices(left_table, right_table),
        row_added=payload["addedCount"],
        row_removed=payload["removedCount"],
    )
    return payload


def _both_length_rows(left: StockTable, right: StockTable, diff: StockDiff) -> int:
    """两侧长度列**都有数值**的行数（长度比对的真实分母）。"""
    # 简化口径：按各自的填充率 × 行数估不出交集，得按比对键重新走一遍。
    # 走 diff 的 added/removed/changed 之外的重合部分是 O(n)，可接受。
    from ...domain.exporting.stockdiff import row_key

    mode = diff.key_mode
    li = {row_key(r, mode): r for r in left.rows}
    ri = {row_key(r, mode): r for r in right.rows}
    n = 0
    for k, r in ri.items():
        other = li.get(k)
        if other is None:
            continue
        if parse_length(r.get("length")) is not None and parse_length(other.get("length")) is not None:
            n += 1
    return n


# --------------------------------------------------------------- 项目间对比
def compare_project_latest(
    store: Any, left_name: str, right_name: str, *, limit: int = 50
) -> dict[str, Any]:
    """比两个项目的**最新版本**（不同项目之间，如"一期 vs 二期"）。

    ## 为什么只比最新版

    跨项目比的语义是"这两个项目的规划现在差在哪"，不是考古。
    要比历史版本用同项目的 `compare_versions`。

    ## 返回结构复用版本 diff

    这样前端一个组件能吃两种 diff，用户也只需学一套术语（新增/缺失/修改）。
    """
    from ...domain.ports.docdiff import DIFF_SECTIONS, diff_docs

    left_project = store.by_name(left_name)
    right_project = store.by_name(right_name)
    if left_project is None:
        raise ValueError(f"项目不存在：{left_name}")
    if right_project is None:
        raise ValueError(f"项目不存在：{right_name}")

    left_latest = store.latest_version(left_project.id)
    right_latest = store.latest_version(right_project.id)
    if left_latest is None:
        raise ValueError(f"项目「{left_name}」还没有任何版本")
    if right_latest is None:
        raise ValueError(f"项目「{right_name}」还没有任何版本")

    # `compare_versions` 只能比同一项目内的两个版本号。跨项目要直接对两份
    # 最新文档算 diff —— 逻辑一致（同一个 `diff_docs`），只是绕过了版本号。
    payload: dict[str, Any] = {}

    before = store.load_doc(left_project.id, left_latest.version) or {}
    after = store.load_doc(right_project.id, right_latest.version) or {}
    labels = dict(DIFF_SECTIONS)
    sections: list[dict[str, Any]] = []
    for diff in diff_docs(dict(before), dict(after)):
        sections.append(
            {
                "section": diff.section,
                "label": labels.get(diff.section, diff.section),
                "added": list(diff.added)[:limit],
                "removed": list(diff.removed)[:limit],
                "changed": [
                    {"before": b, "after": a} for b, a in list(diff.changed)[:limit]
                ],
                "count": diff.count,
                "isEmpty": diff.is_empty,
            }
        )
    payload["leftProject"] = left_name
    payload["rightProject"] = right_name
    payload["leftVersion"] = left_latest.version
    payload["rightVersion"] = right_latest.version
    payload["crossProject"] = True
    payload["sections"] = sections
    payload["totalChanges"] = sum(s["count"] for s in sections)
    return payload


__all__ = [
    "DEFAULT_STOCK_DIR",
    "PLAN_FILE_MARKERS",
    "StockFile",
    "compare_files",
    "compare_project_latest",
    "list_stock_files",
    "load_stock",
    "set_stock_reader",
    "stock_dir",
]
