"""意图分解落库的端到端验收：CLI + API + 真实 V13 数据。

## 这组用例守护什么

单测证明「每张表都写了、写两次不累积」；这里证明的是**真实数据全链路**：

- 949 行意图 → 20132 行规范记录 → 回读计数一行不差
- 落库之后才查得出来的三件事（按状态/角色/线缆类型筛选）数字对得上
- 删掉重写不残留、两个项目互不干扰（多项目共用一张表却不串数据）

## 为什么必须跑真实数据

合成数据里设备都有名字、型号都全、U 位都不重叠 —— 恰好绕开了
真实数据里全部四个坑（631 个位置无名、1 台型号空、13 对 U 重叠、
6 组命名变体）。用合成数据测，这些坑一个都测不出来。
"""

from __future__ import annotations

import os
import sqlite3
import subprocess
import sys
from pathlib import Path

import pytest

pytestmark = pytest.mark.skipif(
    not any((Path(__file__).resolve().parents[3] / "信息港部署上架").glob("*V13*设备上架*.xlsx")),
    reason="需要真实 V13 源文件（只在完整仓库里跑）",
)

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
INTENT = ROOT / "projects" / "v13" / "intent_full.json"

#: 期望的落库行数（真实 V13 意图）。
EXPECTED = {
    "room": 2,
    "aisle": 14,
    "rack": 362,
    "device_model": 27,        # 26 个真实型号 + 1 个「（型号未填）」占位
    "device_model_port": 855,
    "device_instance": 824,
    "port": 12776,
    "cable": 5127,
    "switch_service_range": 142,
    "device_conflict": 3,
}


@pytest.fixture(scope="module")
def db_path(tmp_path_factory: pytest.TempPathFactory) -> Path:
    """跑一次 `decompose` 落库到临时库，全模块共用。"""
    path = tmp_path_factory.mktemp("decompose") / "t.db"
    proc = subprocess.run(
        [
            sys.executable, "-m", "idcbox.cli", "decompose",
            "--intent", str(INTENT),
            "--db", str(path),
            "--stats",
        ],
        cwd=BACKEND,
        capture_output=True,
        text=True,
        env={**os.environ, "PYTHONHOME": "", "PYTHONPATH": ""},
    )
    assert proc.returncode == 0, proc.stdout + proc.stderr
    return path


def table_counts(db: Path) -> dict[str, int]:
    """直接连 SQLite 数行数 —— 不经过程序，避免「程序自己说写对了」。"""
    con = sqlite3.connect(db)
    try:
        return {
            table: con.execute(f"SELECT COUNT(*) FROM {table}").fetchone()[0]
            for table in EXPECTED
        }
    finally:
        con.close()


class TestRealDataLanding:
    def test_each_table_row_count(self, db_path: Path) -> None:
        """10 张表的行数逐张对上。

        ## 为什么逐张断言而不是只断言总数

        总数对上不等于每张都对：端口少 44 行、连线多 44 行的组合
        总数一样，但数据是错的。44 正好是合并产生的重复数（★1），
        是最容易在重构时被漏掉的一处。
        """
        assert table_counts(db_path) == EXPECTED

    def test_no_silent_skip(self, db_path: Path) -> None:
        """`device_instance` 与 `port` 的外键必须全部指向存在的行。

        ## 这条抓的是「静默跳过」

        `_insert_devices` 在型号或机柜对不上时会 `continue` ——
        不报错、不计数。表现是「设备少了一台」，而 CLI 打印的
        `stats` 说 824 台。这里用 SQL 反查悬空外键，把它们揪出来。
        """
        con = sqlite3.connect(db_path)
        try:
            orphan_port = con.execute(
                "SELECT COUNT(*) FROM port p LEFT JOIN device_instance d"
                " ON d.id = p.device_id WHERE d.id IS NULL"
            ).fetchone()[0]
            orphan_cable_a = con.execute(
                "SELECT COUNT(*) FROM cable c LEFT JOIN port p"
                " ON p.id = c.a_port_id WHERE p.id IS NULL"
            ).fetchone()[0]
            orphan_dev = con.execute(
                "SELECT COUNT(*) FROM device_instance d LEFT JOIN rack r"
                " ON r.id = d.rack_id WHERE r.id IS NULL"
            ).fetchone()[0]
        finally:
            con.close()
        assert orphan_port == 0
        assert orphan_cable_a == 0
        assert orphan_dev == 0

    def test_a_side_unique_constraint_holds(self, db_path: Path) -> None:
        """`uq_cable_a_active` 在真实数据上成立（a 侧每口最多一根线）。

        ## 为什么这条重要

        schema 里建了这个唯一索引，但它能建起来的前提是数据满足。
        不满足的话 `write()` 会 `UNIQUE constraint failed: cable.a_port_id`
        —— 一句话看不出是「有一根线插了两个口」还是「同一个口被插了两根线」。
        """
        con = sqlite3.connect(db_path)
        try:
            dup = con.execute(
                "SELECT COUNT(*) FROM (SELECT a_port_id FROM cable"
                " GROUP BY a_port_id HAVING COUNT(*) > 1)"
            ).fetchone()[0]
            total = con.execute("SELECT COUNT(*) FROM cable").fetchone()[0]
        finally:
            con.close()
        assert dup == 0, "a 侧每口只能一根线"
        assert total == 5127

    def test_z_side_allows_one_to_many(self, db_path: Path) -> None:
        """z 侧允许一对多 —— 真实数据里恰好 1 处（MPO 一分三）。

        `分线接入交换机-2` 的 `25GE1/0/48` 接 3 根线（源表就是这么记的）。
        schema 有意不给 z 侧建唯一索引，这条钉住「别顺手加上」。
        """
        con = sqlite3.connect(db_path)
        try:
            rows = con.execute(
                "SELECT z_port_id, COUNT(*) AS n FROM cable"
                " WHERE z_port_id IS NOT NULL GROUP BY z_port_id HAVING n > 1"
            ).fetchall()
        finally:
            con.close()
        assert len(rows) == 1, "真实数据里只有 1 个 z 侧端口接多根"
        assert rows[0][1] == 3


class TestQueryableAfterLanding:
    """落库之后才查得出来的三类问题。每一条都不能靠现算回答。"""

    def test_port_status_counts(self, db_path: Path) -> None:
        con = sqlite3.connect(db_path)
        try:
            rows = dict(
                con.execute("SELECT status, COUNT(*) FROM port GROUP BY status").fetchall()
            )
        finally:
            con.close()
        assert rows == {"occupied": 10252, "free": 2524}

    def test_port_role_counts(self, db_path: Path) -> None:
        """端口角色必须是一等枚举且都有值 —— 这决定候选池与 2D 配色。"""
        con = sqlite3.connect(db_path)
        try:
            rows = dict(
                con.execute("SELECT port_role, COUNT(*) FROM port GROUP BY port_role").fetchall()
            )
        finally:
            con.close()
        assert rows == {
            "downlink": 10748,
            "uplink": 1302,
            "mgmt": 690,
            "stack": 32,
            "unknown": 4,
        }
        assert sum(rows.values()) == 12776

    def test_cable_type_counts(self, db_path: Path) -> None:
        con = sqlite3.connect(db_path)
        try:
            rows = dict(
                con.execute("SELECT cable_type, COUNT(*) FROM cable GROUP BY cable_type").fetchall()
            )
        finally:
            con.close()
        assert rows == {"LC-LC": 2685, "RJ45": 2089, "MPO": 353}

    def test_unknown_port_devices_are_visible(self, db_path: Path) -> None:
        """4 个 `unknown` 角色端口来自**两**种成因，两种都得在库里看得见。

        ## 两种成因（实证）

        ```
        ('广域网链路负载均衡-1', '广域网链路负载均衡', 1)   ← GE8 孤立写法
        ('带外交换机-2',   '（型号未填）',  2)             ← 型号空白，落占位型号
        ('广域网链路负载均衡-2', '广域网链路负载均衡', 1)   ← GE8 孤立写法
        ```

        1. `带外交换机-2` 型号空白 → 落占位型号，端口规格未知。跳过不写会让
           「型号未填」这个待办永远没人做。
        2. `广域网链路负载均衡` 的 `GE8` 是源表里的孤立写法（解析器认不出序号），
           模板里单独声明成一个 unknown 角色的口 —— 而不是硬塞进 GE1~GE7 的
           某个位置（那样会让「GE8 用了什么口」这个问题得到错答案）。

        ## 为什么不能只断言「全是占位型号」

        第一版就是这么写的，结果红了。红得对：`unknown` 角色回答的是
        「这个口干什么用的说不清」，而不是「这台设备的型号填没填」——
        两件事撞在同一列上，只能分别断言。
        """
        con = sqlite3.connect(db_path)
        try:
            rows = con.execute(
                "SELECT d.name, m.name, p.physical_name FROM port p"
                " JOIN device_instance d ON d.id = p.device_id"
                " JOIN device_model m ON m.id = d.model_id"
                " WHERE p.port_role = 'unknown'"
            ).fetchall()
        finally:
            con.close()

        assert len(rows) == 4, f"unknown 角色端口应为 4 个，实得 {len(rows)}: {rows}"

        placeholder = [r for r in rows if r[1] == "（型号未填）"]
        assert len(placeholder) == 2, "带外交换机-2 的 2 个口"
        assert placeholder[0][0] == "带外交换机-2"

        isolated = [r for r in rows if r[1] == "广域网链路负载均衡"]
        assert len(isolated) == 2, "两台负载均衡各有 1 个 GE8"
        assert all(r[2] == "GE8" for r in isolated)


class TestRebuildIsIdempotent:
    def test_second_run_keeps_same_counts(self, db_path: Path) -> None:
        """再跑一次 `decompose`，行数不变（不是翻倍）。

        ## 这条抓的是「全量重建漏了删除」

        漏删的表现不是报错，是行数翻倍 —— 界面「设备 824 台」
        变成「1648 台」时，用户会以为重复导入了，不会想到程序没删旧的。
        """
        first = table_counts(db_path)
        proc = subprocess.run(
            [
                sys.executable, "-m", "idcbox.cli", "decompose",
                "--intent", str(INTENT), "--db", str(db_path),
            ],
            cwd=BACKEND, capture_output=True, text=True,
            env={**os.environ, "PYTHONHOME": "", "PYTHONPATH": ""},
        )
        assert proc.returncode == 0, proc.stdout + proc.stderr
        assert table_counts(db_path) == first

    def test_dry_run_does_not_touch_db(self, tmp_path: Path) -> None:
        """`--dry-run` 不建库文件。"""
        db = tmp_path / "never.db"
        proc = subprocess.run(
            [
                sys.executable, "-m", "idcbox.cli", "decompose",
                "--intent", str(INTENT), "--db", str(db), "--dry-run",
            ],
            cwd=BACKEND, capture_output=True, text=True,
            env={**os.environ, "PYTHONHOME": "", "PYTHONPATH": ""},
        )
        assert proc.returncode == 0, proc.stdout + proc.stderr
        assert not db.exists()
        assert "未落库" in proc.stdout


class TestMultiProject:
    def test_two_projects_do_not_mix(self, db_path: Path) -> None:
        """两个项目各存一份，行数各自独立（清空一个不影响另一个）。

        ## 为什么这条必须测

        `clear(project_id)` 里 10 张表的删除条件都是「按 project_id 或
        通过子查询关联到 project_id」。少写一处 where 就会把别的项目
        一起删掉 —— 而症状是「切到另一个项目发现数据没了」，
        在单项目开发时永远发现不了。
        """
        db = db_path.parent / "two.db"
        env = {**os.environ, "PYTHONHOME": "", "PYTHONPATH": ""}
        for name in ("A项目", "B项目"):
            proc = subprocess.run(
                [
                    sys.executable, "-m", "idcbox.cli", "decompose",
                    "--intent", str(INTENT), "--db", str(db), "--name", name,
                ],
                cwd=BACKEND, capture_output=True, text=True, env=env,
            )
            assert proc.returncode == 0, proc.stdout + proc.stderr

        con = sqlite3.connect(db)
        try:
            projects = con.execute("SELECT COUNT(*) FROM project").fetchone()[0]
            devices = con.execute("SELECT COUNT(*) FROM device_instance").fetchone()[0]
        finally:
            con.close()
        assert projects == 2
        assert devices == 824 * 2, "两个项目各 824 台，不能互相覆盖"
