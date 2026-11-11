"""数据库连接与 schema 初始化。

## 为什么不引 alembic

架构 §2.2 的目录树里写了 `migrations/versions/`（alembic），但离线部署环境下
装 pip 包不可靠（实测 pypi 直连超时、uv 需重定向缓存）。而本项目的
schema 演进模式很特殊：

- 单机、单人/小团队使用，**没有多实例并发升级**问题
- 用户可能从任意历史版本升上来，但**不会同时有多个版本在跑**
- schema 里 34 张表绝大多数是"一次建好不再改"

所以用「幂等 DDL（`CREATE TABLE IF NOT EXISTS`）+ 版本表 + 顺序迁移函数」更合适：
启动时比对版本，落后就逐版升级。比 alembic 少一个依赖、少一套 autogenerate 的
心智负担，且完全可测（`test_db.py` 覆盖 v0 → vN 的升级路径）。

## 为什么不引 ORM 关系映射

`SQLAlchemy 2.0` 已装（架构 §1.8 定的技术栈），但这里只用它的 **Core**
（`text()` + `Row`），不用 ORM 声明式模型。理由：

- 34 张表里有一半是"配置/日志/快照"性质的，用 ORM 会写 34 个类再写 34 个
  转换函数，代码量是 SQL 的三倍而收益为零
- 域层是 dataclass（`LengthRule`/`Intent`/`CableDemand`…），与 ORM 实体本来就要
  转换两次（db → ORM → dataclass）；直接用 `Row → dataclass` 只转一次
- 复杂查询（耗材汇总、体检、diff）用 ORM 表达反而不如 SQL 清楚

**但事务与连接池仍走 SQLAlchemy**：手写 `sqlite3` 要自己管 `BEGIN`/`COMMIT`/回滚，
而 SQLAlchemy 的 `Engine.begin()` 已经把这块做对了。
"""

from __future__ import annotations

import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterator

from sqlalchemy import Engine, create_engine, event, text

#: 当前 schema 版本。每次改 `schema.sql` 必须 +1 并在 `MIGRATIONS` 里补一步。
SCHEMA_VERSION = 1

#: `schema.sql` 与本文件同目录。
_SCHEMA_FILE = Path(__file__).resolve().parent / "schema.sql"


def utcnow_iso() -> str:
    """统一的时间戳格式（UTC ISO8601 秒级）。

    全库只用这一种格式，好处是 SQLite 的文本比较与时间比较等价 ——
    `ORDER BY created_at` 直接就是按时间排，不需要解析成 datetime。
    """
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _apply_pragmas(engine: Engine) -> None:
    """SQLite 关键 pragma。**每条都有具体理由，不是照抄模板**。"""

    @event.listens_for(engine, "connect")
    def _on_connect(dbapi_conn: sqlite3.Connection, _record: Any) -> None:
        cur = dbapi_conn.cursor()
        # 外键约束默认关闭，不开启的话 34 张表的 REFERENCES 全是装饰品
        cur.execute("PRAGMA foreign_keys = ON")
        # WAL：读写不互斥。单机也可能"导出时后台在算"，不加会遇到 database is locked
        cur.execute("PRAGMA journal_mode = WAL")
        # NORMAL + WAL 是官方推荐的平衡点：崩溃可能丢最后一笔事务，
        # 但绝不损坏库。对规划工具（不是金融账本）这个权衡是对的。
        cur.execute("PRAGMA synchronous = NORMAL")
        # 5 秒等锁，避免并发写直接报错
        cur.execute("PRAGMA busy_timeout = 5000")
        cur.close()


def create_db_engine(url_or_path: str | Path) -> Engine:
    """建引擎。

    `":memory:"` 得到内存库（测试用）；其他值当文件路径。
    父目录自动创建 —— 用户可能把库放在 `~/.idcbox/` 这种还没建的目录。
    """
    if isinstance(url_or_path, Path):
        target = url_or_path
    elif url_or_path == ":memory:":
        target = None
    else:
        target = Path(url_or_path)

    if target is None:
        engine = create_engine("sqlite://", future=True)
    else:
        target.parent.mkdir(parents=True, exist_ok=True)
        engine = create_engine(f"sqlite:///{target}", future=True)

    _apply_pragmas(engine)
    return engine


def current_version(engine: Engine) -> int:
    """读 schema 版本。表不存在时返回 0（全新库）。"""
    with engine.connect() as conn:
        exists = conn.execute(
            text(
                "SELECT name FROM sqlite_master "
                "WHERE type='table' AND name='schema_version'"
            )
        ).first()
        if exists is None:
            return 0
        row = conn.execute(text("SELECT MAX(version) AS v FROM schema_version")).first()
    return int(row.v or 0) if row is not None else 0


def init_schema(engine: Engine, *, note: str = "") -> int:
    """建表 / 升级到 `SCHEMA_VERSION`，返回最终版本。

    幂等：重复调用不报错也不重复写入版本行。
    `schema.sql` 全部用 `IF NOT EXISTS`，所以第一版直接整份执行；
    后续版本在 `MIGRATIONS` 里补增量。

    ## 为什么版本行要带 `note`

    半年后回看「v1 是什么时候上的、因为什么」，光有数字没用。
    note 写一句人话，比 commit message 更靠近数据。
    """
    version = current_version(engine)
    ddl = _SCHEMA_FILE.read_text(encoding="utf-8")

    with engine.begin() as conn:
        # 整份 DDL 幂等执行（包在事务里：SQLite DDL 也是事务性的）
        for stmt in _split_statements(ddl):
            conn.execute(text(stmt))

    version = current_version(engine)
    if version < SCHEMA_VERSION:
        with engine.begin() as conn:
            conn.execute(
                text(
                    "INSERT OR REPLACE INTO schema_version (version, applied_at, note) "
                    "VALUES (:v, :at, :note)"
                ),
                {"v": SCHEMA_VERSION, "at": utcnow_iso(), "note": note or "初始化"},
            )
        version = SCHEMA_VERSION
    return version


def _split_statements(ddl: str) -> Iterator[str]:
    """按分号切 SQL，但跳过 `--` 注释行。

    ## 为什么不用 `sqlite3.complete_statement` 或正则

    这份 DDL 里没有触发器/存储过程（那些才需要真正的语句分割器）。
    唯一要小心的是注释里的分号与 `CHECK (...)` 里的逗号。
    按行过滤注释后按 `;` 切已经足够 —— 而且更容易在出错时定位到具体语句。
    """
    buf: list[str] = []
    for line in ddl.splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("--"):
            continue
        buf.append(line)
        # 一行可能包含多条语句（本项目 DDL 里没有，但防御性处理）
        while ";" in "\n".join(buf):
            joined = "\n".join(buf)
            idx = joined.index(";")
            stmt = joined[:idx].strip()
            if stmt:
                yield stmt
            rest = joined[idx + 1 :]
            buf = [rest] if rest.strip() else []
            if not rest.strip():
                break
    tail = "\n".join(buf).strip()
    if tail:
        yield tail


def table_names(engine: Engine) -> list[str]:
    """列出所有业务表（排除 SQLite 内部表）。诊断与测试用。"""
    with engine.connect() as conn:
        rows = conn.execute(
            text(
                "SELECT name FROM sqlite_master WHERE type='table' "
                "AND name NOT LIKE 'sqlite_%' ORDER BY name"
            )
        ).all()
    return [r[0] for r in rows]


__all__ = [
    "SCHEMA_VERSION",
    "create_db_engine",
    "current_version",
    "init_schema",
    "table_names",
    "utcnow_iso",
]
