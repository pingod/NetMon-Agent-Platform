"""应用层依赖倒置契约（只有 Protocol，没有实现）。

## 为什么域层不能直接调这些

域层是纯函数：给定输入必然同输出，不读时钟、不访 DB、不碰文件。
一旦域函数里出现 `open(...)` 或 `datetime.now()`，`compute_batch` 就变成
不可测、不可重放的东西 —— 而它正是产品的核心资产（5127 行公式链）。

所以：**域层只认这里的 Protocol，实现在 `infrastructure/`**。
好处不只是可测 —— 换存储（SQLite→PG）、换 Excel 库（openpyxl→SheetJS 双语实现）、
换部署形态（CLI→HTTP）时，域层一行都不用改。

## 为什么源数据要拆成两个 Protocol

平面图和连线表是两种完全不同的数据，失败模式也不同：
- 平面图缺 sheet → 没有几何，什么都算不了（致命）
- 连线表缺 sheet → 也算不了，但**平面图里有空调位/柱子**是正常的
拆开后可以「只重算几何不重读连线表」，也便于将来接数据库而非 Excel。

## 为什么 `CablingSource.read_links()` 返回四元组

因为同一批行要两份视图：
- `links`（域层计算用）：已归一，`带外交换机 1` → `带外交换机1`
- `audit_rows`/`audit_col`（体检用）：**保留原始值**，否则永远看不到
  「名称带空格」「T 列填了待定」这类问题 —— 那些正是体检要报的东西。
两者不是两份数据，是对同一批行的两种投影。
"""

from __future__ import annotations

from datetime import datetime
from typing import Mapping, Protocol, Sequence

from ..domain.cable_length.link import LinkInput
from ..domain.cable_length.rules import LengthRule
from ..domain.geometry.layout import GeometrySnapshot
from ..domain.shared.types import Issue


class Clock(Protocol):
    """时钟。域层需要时间时必须从这里取，保证测试可注入固定时间。"""

    def now(self) -> datetime: ...


class IdGenerator(Protocol):
    """ID 生成。前缀区分实体类型（`cbl_`/`rck_`/`prj_`）。"""

    def new_id(self, prefix: str) -> str: ...


class SkippedRow(Protocol):
    """被跳过的一行及原因。**必须交给用户看**，不能静默丢弃。"""

    row_no: int
    reason: str


class LinkPreview(Protocol):
    """导入的中间产物。域层只认这几个字段，不认具体适配器类型。"""

    links: Sequence[LinkInput]
    raw_values: Mapping[str, Mapping[str, object]]
    skipped: Sequence[SkippedRow]
    total_rows: int
    empty_rows: int

    @property
    def effective_rows(self) -> int: ...


class FloorSource(Protocol):
    """平面图来源。返回原始单元格文本，不解释含义。"""

    def read_floor_cells(
        self,
    ) -> tuple[Sequence[tuple[int, int, object]], tuple[int, int], tuple[int, int]]:
        """返回 (单元格 (行,列,值) 列表, 行范围, 列范围)。"""
        ...


class CablingSource(Protocol):
    """连线表来源。返回「归一视图 + 原始视图」两份投影。"""

    def read_links(
        self,
    ) -> tuple[
        LinkPreview,
        Sequence[Sequence[object]],
        Sequence[int],
        Mapping[str, int],
    ]:
        """返回 (导入预览, 原始行值, 原始行号, 列名→索引)。"""
        ...


class CablingWriter(Protocol):
    """写出连线表。`mode='value'` 归档、`mode='formula'` 交设计院二次编辑。"""

    def write(self, rows: Sequence[object], path: str, *, mode: str) -> str: ...


class RuleRepository(Protocol):
    """规则仓库。规则带版本，每次计算都要记下用的是哪一版。"""

    def active_rule(self, project_id: str) -> LengthRule: ...


class GeometryRepository(Protocol):
    """几何快照仓库。快照不可变 —— 改了就是新版本。"""

    def snapshot(self, project_id: str) -> GeometrySnapshot: ...


class ProblemReporter(Protocol):
    """问题收集器。域层产出 `Issue`，由这里决定落到报告/日志/库。"""

    def emit(self, issues: Sequence[Issue]) -> None: ...


class LinkSource(Protocol):
    """连线清单来源。导入适配层负责把各种版式收敛成 `LinkInput`。"""

    def links(self) -> Sequence[LinkInput]: ...


class ProjectStore(Protocol):
    """项目与意图文档的持久化。**应用层只认这几个方法，不认 SQL**。

    ## 为什么把「保存」和「读版本」放在同一个 Protocol

    它们共享一个不变量：**同一项目内的版本号连续且不复用**。
    拆成两个 Protocol 会让调用方以为可以只拿读的那半，而读的语义
    （`delete_version` 只能删最新版）依赖写的实现。放在一起，
    实现者必须一次想清整个版本模型。
    """

    def create(
        self, *, name: str, site_key: str = "", source_path: str | None = None
    ) -> Any: ...

    def by_name(self, name: str) -> Any | None: ...

    def list(self, *, include_archived: bool = False) -> Sequence[Any]: ...

    def save_intent(
        self, project_id: str, doc: Mapping[str, Any], *, note: str | None = None
    ) -> Any: ...

    def latest_version(self, project_id: str) -> Any | None: ...

    def versions(self, project_id: str) -> Sequence[Any]: ...

    def load_doc(
        self, project_id: str, version: int | None = None
    ) -> Mapping[str, Any] | None: ...


class StockSheet(Protocol):
    """一个可参与对比的 sheet 的基本信息。

    只暴露应用层需要的四个字段 —— 不暴露 openpyxl 的 `SheetInfo`，
    否则 `compare.py` 会为了拿 `rows`/`cols` 而 import 基础设施模块。
    """

    name: str
    rows: int
    cols: int
    priority: int


class StockReader(Protocol):
    """存量 Excel 的读取能力（依赖倒置）。

    ## 为什么必须是个 Protocol 而不是直接 import `stock_reader`

    `compare.py` 属于应用层，读 Excel 属于基础设施层。契约
    「应用层不得依赖基础设施（依赖倒置：靠 Protocol 注入）」要求两者之间
    只能通过端口通信 —— 否则换一个读表实现（比如支持 xls / csv）就得改应用层。

    实测这条契约真的拦下过一次：`compare.py` 直接
    `from ...infrastructure.xlsx.stock_reader import read_stock_table`，
    `lint-imports` 立刻报 BROKEN。
    """

    def scan(self, path: str) -> Sequence[StockSheet]:
        """扫一个 Excel 里所有能当连线表的 sheet，按优先级排序。"""
        ...

    def read(self, path: str, *, sheet: str, label: str = "") -> Any:
        """读成 `StockTable`。返回 `Any` 是为了让 ports 不依赖 domain.exporting。"""
        ...


__all__ = [
    "CablingSource",
    "CablingWriter",
    "Clock",
    "FloorSource",
    "GeometryRepository",
    "IdGenerator",
    "LinkPreview",
    "LinkSource",
    "ProblemReporter",
    "ProjectStore",
    "RuleRepository",
    "SkippedRow",
    "StockReader",
    "StockSheet",
]
