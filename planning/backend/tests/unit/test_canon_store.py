"""规范台账落库的单测：确定性 ID、事务完整性、全量重建幂等。

## 这个模块守护什么

`CanonicalStore.write()` 往 10 张表里写 2 万行，任何一处出错的表现都很像：

- 数字少了一点（823 而不是 824 台设备）—— 看不出少的是哪台
- 报 `FOREIGN KEY constraint failed` —— 不说是哪张表的哪一行
- 报 `UNIQUE constraint failed: port.device_id, port.physical_name` ——
  不说是哪台设备的哪个口

所以这里逐个把「会造成上面三种症状的具体原因」钉住。

## 四条口径（每条都有对应用例）

1. **确定性 ID**：同一份 bundle 写两次得到完全同样的主键。用 UUID 就做不到，
   而主键不稳定会让「重跑一次导入」变成「换了一套设备」。
2. **全量重建**：第二次写之前先删干净，行数不累积。
3. **删除顺序**：`cable`/`device_instance`/`rack` 三处外键没有级联，顺序错了直接报错。
4. **静默跳过必须为零**：`write()` 返回的 `WriteStats` 必须等于回读计数，
   否则说明有行被静默丢了。
"""

from __future__ import annotations

import pytest
from dataclasses import replace

from idcbox.domain.ports.canon import (
    CanonAisle,
    CanonCable,
    CanonConflict,
    CanonDevice,
    CanonModel,
    CanonModelPort,
    CanonPort,
    CanonRack,
    CanonRange,
    CanonRoom,
    CanonicalBundle,
)
from idcbox.infrastructure.db.canon_store import (
    UNKNOWN_MODEL_NAME,
    CanonicalStore,
    aisle_id,
    db_category,
    db_role_hint,
    device_id,
    model_id,
    model_port_id,
    port_id,
    rack_id,
    room_id,
)
from idcbox.infrastructure.db.repositories import ProjectRepository
from idcbox.infrastructure.db.session import create_db_engine, init_schema

# ---------------------------------------------------------------------------
# 装置
# ---------------------------------------------------------------------------

DEV_A = (1, 5, 3, "业务核心交换机")
DEV_B = (1, 6, 3, "计算节点A（ARM）")


def _bundle(*, with_cable: bool = True, device_count: int = 2) -> CanonicalBundle:
    """一个能覆盖全部 10 张表的最小 bundle。"""
    devices = [
        CanonDevice(
            key=DEV_A, name="业务核心交换机-1", name_normalized="业务核心交换机-1",
            model="业务核心交换机", model_normalized="业务核心交换机",
            category="core", channel_no=1, rack_no=5, side="N", u_start=3, u_end=12,
            ledger_key="业务核心交换机-1",
        ),
        CanonDevice(
            key=DEV_B, name="", name_normalized="",
            model="计算节点A（ARM）", model_normalized="计算节点A（ARM）",
            category="server", channel_no=1, rack_no=6, side="N", u_start=3, u_end=4,
            ledger_key="N01-06-S0",
        ),
    ][:device_count]
    ports = [
        CanonPort(
            device_key=DEV_A, physical_name="100GE1/0/1", display_name="100GE1/0/1",
            speed_mbps=100_000, connector="QSFP28", port_type="optical",
            port_role="uplink", status="occupied", panel_slot=1, panel_row=0, panel_col=1,
        ),
        CanonPort(
            device_key=DEV_A, physical_name="100GE1/0/2", display_name="100GE1/0/2",
            speed_mbps=100_000, connector="QSFP28", port_type="optical",
            port_role="uplink", status="free", panel_slot=1, panel_row=0, panel_col=2,
        ),
        CanonPort(
            device_key=DEV_B, physical_name="万兆1", display_name="万兆1",
            speed_mbps=25_000, connector="SFP28", port_type="optical",
            port_role="downlink", status="occupied", panel_slot=1, panel_row=0, panel_col=1,
        ),
    ]
    cables: list[CanonCable] = []
    if with_cable:
        cables.append(
            CanonCable(
                stable_key="业务核心交换机-1|100GE1/0/1|N01-06-S0|万兆1|LC-LC",
                occurrence=0,
                a_device_key=DEV_A, a_port_name="100GE1/0/1",
                z_device_key=DEV_B, z_port_name="万兆1",
                cable_type="LC-LC",
            )
        )
    return CanonicalBundle(
        rooms=(CanonRoom(code="403", name="403机房", col_min=23, col_max=38),),
        aisles=(CanonAisle(room_code="403", channel_no=1, plan_row_n=3, plan_row_s=4),),
        racks=(
            CanonRack(room_code="403", channel_no=1, seq_no=5, side="N", col_index=34,
                      row_index=3, display_name="A模组-1-5"),
            CanonRack(room_code="403", channel_no=1, seq_no=6, side="N", col_index=33,
                      row_index=3, display_name="A模组-1-6"),
        ),
        models=(
            CanonModel(name="业务核心交换机", name_normalized="业务核心交换机",
                       category="core", spec_text="108×100GE + 48×25GE", u_height=10.0),
            CanonModel(name="计算节点A（ARM）", name_normalized="计算节点A（ARM）",
                       category="server", spec_text="四口25G网卡", u_height=2.0),
        ),
        model_ports=(
            CanonModelPort(model_normalized="业务核心交换机", seq=1,
                           physical_name="100GE1/0/1", speed_mbps=100_000,
                           connector="QSFP28", port_type="optical",
                           panel_slot=1, panel_row=0, panel_col=1, port_role="uplink"),
        ),
        devices=tuple(devices),
        ports=tuple(ports),
        cables=tuple(cables),
        ranges=(
            CanonRange(switch_device_key=DEV_A, node_model_normalized="计算节点A（ARM）",
                       base_port_no=1, node_count_expected=44),
        ),
        conflicts=(
            CanonConflict(kind="U_OVERLAP", channel_no=1, rack_no=6, u_start=31, u_end=31,
                          detail="两台设备抢占 31U"),
        ),
    )


@pytest.fixture
def engine():
    eng = create_db_engine(":memory:")
    init_schema(eng)
    return eng


@pytest.fixture
def store(engine):
    return CanonicalStore(engine)


@pytest.fixture
def project(engine):
    return ProjectRepository(engine).create(name="T")


# ---------------------------------------------------------------------------
# 1. 确定性 ID
# ---------------------------------------------------------------------------

PROJ = "prj_test"


class TestDeterministicIds:
    def test_same_key_same_id(self) -> None:
        """同一自然键永远得到同一个 ID。

        ## 如果这条不成立

        用 `uuid4()` 的话，重跑一次导入就是「换了一套设备」：外键全部重接、
        审计日志里的 `device_id` 全部失效、跟外部系统对接的 ID 也全变。
        """
        assert device_id(PROJ, DEV_A) == device_id(PROJ, DEV_A)
        assert device_id(PROJ, DEV_A) != device_id(PROJ, DEV_B)

    def test_id_is_stable_across_runs(self) -> None:
        """进程重启后 ID 不变（不能依赖哈希种子）。"""
        import subprocess
        import sys
        from pathlib import Path

        code = (
            "import sys; sys.path.insert(0, 'backend');"
            "from idcbox.infrastructure.db.canon_store import device_id;"
            "print(device_id('prj_test', (1, 5, 3, '业务核心交换机')))"
        )
        root = Path(__file__).resolve().parents[3]
        out = subprocess.run(
            [sys.executable, "-c", code], cwd=root, capture_output=True, text=True
        )
        assert out.returncode == 0, out.stderr
        assert out.stdout.strip() == device_id(PROJ, DEV_A)

    def test_different_prefixes_for_different_tables(self) -> None:
        """不同表用不同前缀 —— 同 ID 出现在两张表里会让 join 写出幻觉。"""
        assert device_id(PROJ, DEV_A).startswith("dev_")
        assert rack_id(PROJ, "403", 1, "N", 5).startswith("rk_")
        assert room_id(PROJ, "403").startswith("rm_")
        assert model_id(PROJ, "业务核心交换机").startswith("mdl_")
        assert port_id(PROJ, DEV_A, "100GE1/0/1").startswith("prt_")

    def test_port_id_includes_physical_name(self) -> None:
        """端口 ID 必须含端口名，否则一台设备的所有口会撞成同一个 ID。"""
        assert port_id(PROJ, DEV_A, "100GE1/0/1") != port_id(PROJ, DEV_A, "100GE1/0/2")

    def test_ids_are_scoped_by_project(self) -> None:
        """换项目就是换一套 ID，哪怕自然键一模一样。

        ## 这条抓的是一个真实事故

        第一版 `room_id("403")` 只哈希房间号。第二个项目也有「403」房间，
        于是两个项目算出同一个主键 → `UNIQUE constraint failed: room.id`，
        报错参数是 `('rm_8980dc86c1e9', 'prj_f3c157d13e63', '403机房', ...)` ——
        **看得见 ID 冲突，看不出「因为没按项目分范围」**。

        更糟的是：如果 ID 不撞（比如房间号不同），两个项目就会「共用」
        同一个型号行 —— 改一个项目的型号名，另一个项目的设备跟着变，
        而这种跨项目污染比报错难查得多。
        """
        for fn, args in (
            (room_id, ("403",)),
            (aisle_id, ("403", 1)),
            (rack_id, ("403", 1, "N", 5)),
            (model_id, ("业务核心交换机",)),
            (model_port_id, ("业务核心交换机", "100GE1/0/1")),
            (device_id, (DEV_A,)),
            (port_id, (DEV_A, "100GE1/0/1")),
        ):
            assert fn("p1", *args) != fn("p2", *args), f"{fn.__name__} 没有按项目分范围"


# ---------------------------------------------------------------------------
# 2. 分类映射（schema CHECK 的约束）
# ---------------------------------------------------------------------------


class TestCategoryMapping:
    def test_core_becomes_switch_with_role_hint(self) -> None:
        """`core` 必须落成 `switch` + `role_hint='core'`。

        ## 为什么不能直接写 `core`

        `schema.sql` 的 `device_model.category` CHECK 只允许
        `switch/server/security/balancer/storage/other` —— **没有 `core`**。
        直接写会 `CHECK constraint failed`，而且报错不提「6 台核心交换机」。

        ## 为什么不改 CHECK 加一个 core

        分类是采购维度（交换机就是交换机），「核心」是角色维度
        （同一台 S7706 可以做核心也可以做接入）。混在一列里，
        以后按「核心交换机几台」统计就得靠字符串匹配。
        """
        assert db_category("core") == "switch"
        assert db_role_hint("core") == "core"
        assert db_role_hint("switch") is None

    def test_other_categories_pass_through(self) -> None:
        for cat in ("switch", "server", "security", "storage"):
            assert db_category(cat) == cat


# ---------------------------------------------------------------------------
# 3. 写库：行数、静默跳过、幂等
# ---------------------------------------------------------------------------


class TestWrite:
    def test_all_tables_written(self, store, project) -> None:
        bundle = _bundle()
        stats = store.write(project.id, bundle)
        counts = store.counts(project.id)
        assert counts["rooms"] == 1
        assert counts["aisles"] == 1
        assert counts["racks"] == 2
        assert counts["models"] == 2
        assert counts["modelPorts"] == 1
        assert counts["devices"] == 2
        assert counts["ports"] == 3
        assert counts["cables"] == 1
        assert counts["ranges"] == 1
        assert counts["conflicts"] == 1
        assert stats.total == sum(counts.values()), "stats 必须等于回读，不能有静默跳过"

    def test_write_twice_is_idempotent(self, store, project) -> None:
        """写两次行数不累积。

        ## 这条抓的是「全量重建漏了删除」

        漏删的表现不是报错，是行数翻倍 —— 而界面上「设备 824 台」
        变成「1648 台」时，用户第一反应是「数据重复导入了」，
        不会想到是程序没删旧的。
        """
        bundle = _bundle()
        store.write(project.id, bundle)
        first = store.counts(project.id)
        stats2 = store.write(project.id, bundle)
        assert store.counts(project.id) == first
        assert stats2.deleted["port"] == 3
        assert stats2.deleted["device_instance"] == 2

    def test_delete_order_respects_foreign_keys(self, store, project) -> None:
        """第二次写不报外键错 —— 删除顺序必须倒序。

        `cable.a_port_id` 与 `device_instance.rack_id` 都没有 ON DELETE
        CASCADE，顺序错了会 `FOREIGN KEY constraint failed`，
        而且报错只说表名不说是哪个引用。
        """
        bundle = _bundle()
        store.write(project.id, bundle)
        store.write(project.id, bundle)  # 不抛异常即通过
        assert store.counts(project.id)["devices"] == 2

    def test_clear_reports_per_table_counts(self, store, project) -> None:
        """`clear()` 要报每张表删了多少 —— CLI 输出「清理旧台账：端口 12776 行」
        比「清理完成」可信得多。"""
        store.write(project.id, _bundle())
        deleted = store.clear(project.id)
        assert deleted["cable"] == 1
        assert deleted["port"] == 3
        assert deleted["room"] == 1
        # 全清之后必须一行不剩。写成 `assert all(...) is False or ...`
        # 之类的花招会掩盖「某张表漏删」—— 直接断言总和。
        assert sum(store.counts(project.id).values()) == 0

    def test_empty_bundle_is_a_noop(self, store, project) -> None:
        """空 bundle 不报错（新建项目还没排设备时会走到这里）。"""
        empty = CanonicalBundle(
            rooms=(), aisles=(), racks=(), models=(), model_ports=(),
            devices=(), ports=(), cables=(), ranges=(), conflicts=(),
        )
        stats = store.write(project.id, empty)
        assert stats.total == 0
        assert sum(store.counts(project.id).values()) == 0


# ---------------------------------------------------------------------------
# 4. 两台设备的特殊情形
# ---------------------------------------------------------------------------


class TestDeviceEdgeCases:
    def test_server_without_name_gets_null(self, store, project) -> None:
        """服务器没名字时 `name` 落 NULL，绝不用位置号填充。

        ## 为什么这件事必须钉住

        实测 824 个位置里 631 个没有名字。用位置号（`N01-06-S0`）填进去，
        界面看起来一切正常 —— 但所有按名字分组的功能（「这台设备改过什么」、
        「同名设备合并」）会把每一台都当成独立命名的新设备。
        """
        store.write(project.id, _bundle())
        with store._engine.connect() as conn:  # noqa: SLF001 - 测试要直查
            rows = conn.execute(
                text_for_test("SELECT name, name_normalized FROM device_instance ORDER BY u_start")
            ).all()
        assert rows[0].name == "业务核心交换机-1"
        assert rows[1].name is None
        assert rows[1].name_normalized is None

    def test_missing_model_gets_placeholder(self, store, project) -> None:
        """型号空白的设备仍要落库，用占位型号。

        ## 真实数据里恰好一台

        `带外交换机-2`（通道1 机柜23 34U）：源表里作为本端 0 行、
        作为对端 2 行且对端型号列也空。`device_instance.model_id` 是
        `NOT NULL REFERENCES device_model`，不给它建型号行就写不进去，
        而且 `_insert_devices` 会**静默跳过** —— 表现为「设备少了一台」，
        而那台恰好是「型号没填」这个待办本身。
        """
        bundle = _bundle()
        devices = list(bundle.devices)
        devices[1] = CanonDevice(
            key=DEV_B, name="", name_normalized="", model="", model_normalized="",
            category="other", channel_no=1, rack_no=6, side="N", u_start=3, u_end=4,
            ledger_key="N01-06-S0",
        )
        # `CanonicalBundle` 是 `slots=True`，没有 `__dict__` —— 必须用
        # `dataclasses.replace`，不能 `**bundle.__dict__`。
        store.write(project.id, replace(bundle, devices=tuple(devices)))
        counts = store.counts(project.id)
        assert counts["devices"] == 2, "型号空白的设备不能被静默跳过"
        assert counts["models"] == 3, "26 个真实型号 + 1 个占位"
        with store._engine.connect() as conn:  # noqa: SLF001
            name = conn.execute(
                text_for_test(
                    "SELECT m.name FROM device_instance d JOIN device_model m"
                    " ON m.id = d.model_id WHERE d.name IS NULL"
                )
            ).scalar_one()
        assert name == UNKNOWN_MODEL_NAME

    def test_device_in_south_rack_is_written(self, store, project) -> None:
        """南侧（S）机柜里的设备也要落库。

        ## 这条抓的是一个真实存在的 368 台缺口

        `CanonDevice.side` 目前恒为 `"N"`（canon 层没有几何快照，判断不了
        设备在通道哪一侧）。若只按 `(通道, 侧别, 柜号)` 精确查 rack_id，
        368 台在 S 侧机柜里的设备全部找不到 rack 被静默跳过，
        表现为「设备只有 456 台」。

        实测**同一通道内柜号唯一确定侧别**（14 个通道全满足），
        所以补一个 `(通道, 柜号)` 的备用索引就够了。
        """
        bundle = _bundle()
        racks = list(bundle.racks)
        racks[1] = CanonRack(room_code="403", channel_no=1, seq_no=6, side="S",
                             col_index=33, row_index=4, display_name="A模组-1-6")
        store.write(project.id, replace(bundle, racks=tuple(racks)))
        assert store.counts(project.id)["devices"] == 2, "S 侧机柜的设备不能被跳过"


# ---------------------------------------------------------------------------
# 5. 读：聚合查询
# ---------------------------------------------------------------------------


class TestReads:
    def test_port_status_counts(self, store, project) -> None:
        store.write(project.id, _bundle())
        assert store.port_status_counts(project.id) == {"occupied": 2, "free": 1}

    def test_port_role_counts(self, store, project) -> None:
        store.write(project.id, _bundle())
        assert store.port_role_counts(project.id) == {"uplink": 2, "downlink": 1}

    def test_cable_type_counts_uses_positional_index(self, store, project) -> None:
        """按线缆类型分组要得到 `{'LC-LC': 1}` 而不是 `{"('LC-LC', 1)": 1}`。

        ## 这条抓的是一个真实的 Row 取值 bug

        用 `SELECT cable_type AS t, COUNT(*) AS n` 再读 `r.t`/`r.n` 时，
        SQLAlchemy 2.0 的 Row 对带别名的表达式列偶尔解析到整行，
        实测拿到 `{"('LC-LC', 2683)": 2683}` —— 键是整行元组的字符串形式。
        改用位置索引 `r[0]`/`r[1]`。
        """
        store.write(project.id, _bundle())
        assert store.cable_type_counts(project.id) == {"LC-LC": 1}

    def test_devices_by_state(self, store, project) -> None:
        store.write(project.id, _bundle())
        assert store.devices_by_state(project.id) == {"planned": 2}

    def test_open_conflicts_payload(self, store, project) -> None:
        store.write(project.id, _bundle())
        out = store.open_conflicts(project.id)
        assert len(out) == 1
        assert out[0]["kind"] == "U_OVERLAP"
        assert out[0]["uStart"] == 31
        assert out[0]["detail"]["detail"] == "两台设备抢占 31U"

    def test_counts_of_unknown_project_is_all_zero(self, store) -> None:
        assert sum(store.counts("prj_nonexistent").values()) == 0


def text_for_test(sql: str):  # noqa: ANN201 - 测试内部助手
    from sqlalchemy import text

    return text(sql)
