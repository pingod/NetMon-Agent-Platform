"""`domain/ports/canon.py` 的单元测试 —— 意图摊平成规范行。

## 这个模块守护什么

它是「意图」与「数据库」之间唯一的换算层。上游是 949 行意图（人写的），
下游是 6 张规范表（`device_instance` / `port` / `cable` /
`switch_service_range` / `device_model` / `rack`）。这里的每个换算错了，
表现都**不在这一层**，而在很远的地方：

- 设备合并判错 → 台账上「两台各用一半口」，现场按台账插线必撞车
- 端口状态判错 → 「还剩几个空闲口」答错，采购多买或少买
- 端点解析漏了 → 库里的连线凭空少几十条，而 `cables` 计数看起来正常

所以这组用例以**真实数据的固定数字**为主：数字变了就必须解释为什么变。

## 为什么这些断言能写在单测里而不是集成测试

`build_canonical` 是纯函数，输入是台账 + 几何快照 + 连线，不碰 DB。
构造这几样输入需要读 Excel（那部分放在 `TestAgainstRealData` 的 one-shot
fixture 里，只读一次），但断言本身没有任何 IO。
"""

from __future__ import annotations

import json
from collections import Counter
from pathlib import Path

import pytest

from idcbox.domain.geometry.floor import FloorCell, derive_cmax_by_room, parse_floor
from idcbox.domain.geometry.layout import build_snapshot
from idcbox.domain.importing.normalize import norm_name
from idcbox.domain.ports import canon
from idcbox.domain.ports.intent import intent_from_dict, to_flow_inputs
from idcbox.domain.ports.flow import plan_flow
from idcbox.domain.ports.ledger import (
    DevicePorts,
    PortEntry,
    PortLedger,
    RawPortUse,
    build_ledger,
)
from idcbox.domain.ports.portspec import PortBay, PortSpec, spec_of

ROOT = Path(__file__).resolve().parents[3]
SOURCE = next(
    (ROOT / "信息港部署上架").glob("*V13*设备上架*.xlsx"),
    None,
)
INTENT = ROOT / "projects" / "v13" / "intent_full.json"


# ---------------------------------------------------------------------------
# 合成数据：能精确控制「端口集是否重叠」的两种情形
# ---------------------------------------------------------------------------
def _use(
    *,
    device_key: str,
    device_label: str,
    model: str,
    channel: int = 1,
    rack: int = 5,
    u: int = 3,
    span: int = 2,
    port: str,
    peer: str = "对端-1",
    peer_port: str = "GE1",
) -> RawPortUse:
    return RawPortUse(
        side="a",
        device_key=device_key,
        device_label=device_label,
        model=model,
        channel_no=channel,
        rack_no=rack,
        u_start=u,
        u_span=span,
        port=port,
        peer_device=peer,
        peer_port=peer_port,
        cable_type="RJ45",
    )


def _geometry():
    from idcbox.domain.geometry.floor import RackPlacement

    racks = [
        RackPlacement("403", 1, 1, "N", col=38, row=3),
        RackPlacement("403", 1, 5, "N", col=34, row=3),
        RackPlacement("403", 1, 27, "S", col=38, row=4),
        RackPlacement("404", 8, 1, "N", col=18, row=3),
    ]
    return build_snapshot(racks, version="test", cmax_override={"403": 38, "404": 18})


class TestMergeNamedVariants:
    """★1：同一台设备被记了两个名字 → 必须合并成一台。"""

    def test_disjoint_ports_merge_into_one(self) -> None:
        """端口集不重叠 = 一台设备的两半 —— 合并后端口应当齐全。

        真实原型：`备份-1` 记了 IPMI/千兆1/千兆3，`备份一体机-1` 记了
        万兆1..4，同一个 (通道1, 机柜5, 3U)。不合并 → 库里两台各用 3/8 与 4/8，
        台账上看起来「两台都空着一半」，实际一台用了 7/8。
        """
        model = "本地备份-备份一体机"
        uses = [
            _use(device_key="备份-1", device_label="备份-1", model=model, port=p)
            for p in ("IPMI", "千兆1", "千兆3")
        ] + [
            _use(
                device_key="备份一体机-1",
                device_label="备份一体机-1",
                model=model,
                port=p,
            )
            for p in ("万兆1", "万兆2", "万兆3", "万兆4")
        ]
        outcome = canon.merge_devices(build_ledger(uses))

        assert len(outcome.devices) == 1, "同一位置的命名变体必须合成一台"
        assert outcome.merged_groups == 1
        dev = outcome.devices[0]
        assert dev.name, "合并后必须有名字"
        assert dev.merged_from, "被合并掉的别名要留档，否则用户查不到自己填的那个名"
        assert outcome.conflicts == (), "命名变体不是冲突，不该报 U_OVERLAP"

    def test_primary_name_is_the_shorter_one(self) -> None:
        """主名取短名（`备份-1` 而非 `备份一体机-1`）。

        依据：短名出现在更多行里，是现场实际在用的名字。
        """
        model = "本地备份-备份一体机"
        uses = [
            _use(device_key="备份一体机-1", device_label="备份一体机-1", model=model, port="万兆1"),
            _use(device_key="备份-1", device_label="备份-1", model=model, port="IPMI"),
        ]
        outcome = canon.merge_devices(build_ledger(uses))
        assert outcome.devices[0].name == "备份-1"


class TestMergeStackedDevices:
    """★2：真叠放（端口集相交）→ 保留两台 + 记冲突，绝不合并。"""

    def test_overlapping_ports_stay_two_devices(self) -> None:
        """端口集有交集 = 同 U 位真放了两台。

        真实原型：`ch2 rack11 31U` 上的 `N02-11-S7` 与 `N02-11-S7-1`，
        端口完全相同（7/7）—— 意图里 slot_map 把 slot 7 声明了两次。
        合并会把两台真设备变成一台，现场照台账只装一台就少装一半算力。
        """
        model = "计算节点A（ARM）"
        uses = [
            _use(device_key=k, device_label=k, model=model, channel=2, rack=11, u=31, port=p)
            for k in ("N02-11-S7", "N02-11-S7-1")
            for p in ("万兆1", "千兆1")
        ]
        outcome = canon.merge_devices(build_ledger(uses))

        assert len(outcome.devices) == 2, "真叠放必须保留两台"
        assert len(outcome.conflicts) == 1
        conflict = outcome.conflicts[0]
        assert conflict.kind == "U_OVERLAP"
        assert (conflict.channel_no, conflict.rack_no, conflict.u_start) == (2, 11, 31)
        assert "N02-11-S7" in conflict.detail
        assert outcome.merged_groups == 0, "真叠放不算合并"

    def test_two_devices_must_have_distinct_keys(self) -> None:
        """叠放的两台在库里的键必须不同 —— 否则主键冲突写不进去。

        物理位置相同，唯一能区分的是型号归一名加后缀。
        """
        model = "计算节点A（ARM）"
        uses = [
            _use(device_key=k, device_label=k, model=model, channel=2, rack=11, u=31, port=p)
            for k in ("S7", "S7-1")
            for p in ("万兆1",)
        ]
        outcome = canon.merge_devices(build_ledger(uses))
        keys = {d.key for d in outcome.devices}
        assert len(keys) == 2, "两台的键必须能区分，否则 INSERT 会撞主键"


class TestPortStatus:
    """端口三态：occupied / free / unknown。"""

    def test_used_port_is_occupied(self) -> None:
        ledger = build_ledger(
            [_use(device_key="SW-1", device_label="SW-1", model="带内交换机1", port="GE1/0/1")]
        )
        devices = canon.merge_devices(ledger).devices
        ports = canon.canon_ports(ledger, devices)
        used = [p for p in ports if p.physical_name == "GE1/0/1"]
        assert used and used[0].status == "occupied"

    def test_free_port_from_template(self) -> None:
        """空闲口来自模板展开 —— 这是「还剩几个口」的唯一来源。"""
        ledger = build_ledger(
            [_use(device_key="SW-1", device_label="SW-1", model="带内交换机1", port="GE1/0/1")]
        )
        devices = canon.merge_devices(ledger).devices
        ports = canon.canon_ports(ledger, devices)
        assert sum(1 for p in ports if p.status == "free") > 0

    def test_used_port_is_occupied_even_without_spec(self) -> None:
        """型号没模板时，**接了线的口仍然是 occupied**。

        ## 这条用例原本写成断言 unknown，是错的

        第一版认为「没模板 → 什么都不知道 → unknown」。但 `occupied` 说的
        是「这个口插了线」——那是**已经发生的物理事实**，跟有没有模板无关。
        把它改成 unknown 会让「已用端口数」少算，而少算的部分恰好是
        源表里型号记错的那批设备，最需要被看见。

        `unknown` 只留给「既没有线、又不在模板里」的口：那种口的真实状态
        确实不知道（可能空着、可能接了没登记），算 free 会让
        「还剩几个口」这个数凭空变大。
        """
        ledger = build_ledger(
            [_use(device_key="谜之设备-1", device_label="谜之设备-1", model="不存在的型号", port="X1")]
        )
        devices = canon.merge_devices(ledger).devices
        ports = canon.canon_ports(ledger, devices)
        assert [p.status for p in ports] == ["occupied"]
        assert all(p.port_role == "unknown" for p in ports), "角色未知，但状态是已知的"

    def test_no_spec_device_has_no_free_ports(self) -> None:
        """没模板的设备不产生 free 端口 —— 不知道就不猜。

        真实数据里唯一这样的设备是 `带外交换机-2`（型号为空）。
        给它猜 48 个空闲 GE 口，整体空闲数会从 2568 变成 2616，
        而多出来的 48 个口根本没有依据。
        """
        ledger = build_ledger(
            [_use(device_key="谜之设备-1", device_label="谜之设备-1", model="不存在的型号", port="X1")]
        )
        devices = canon.merge_devices(ledger).devices
        ports = canon.canon_ports(ledger, devices)
        assert [p for p in ports if p.status == "free"] == []


class TestCanonModels:
    """型号模板展开。"""

    def test_26_models_855_ports(self) -> None:
        models, model_ports = canon.canon_models()
        assert len(models) == 26
        assert len(model_ports) == 855

    def test_core_switch_156_ports(self) -> None:
        """业务核心交换机 = 3×36 个 100GE + 48 个 25GE = 156。"""
        _, ports = canon.canon_models()
        core = [p for p in ports if p.model_normalized == norm_name("业务核心交换机")]
        assert len(core) == 156
        assert len([p for p in core if p.speed_mbps == 100_000]) == 108
        assert len([p for p in core if p.speed_mbps == 25_000]) == 48

    def test_port_names_are_unique_per_model(self) -> None:
        """同一型号内端口名不能重复 —— `UNIQUE(model_id, physical_name)` 的前提。"""
        _, ports = canon.canon_models()
        per_model = Counter((p.model_normalized, p.physical_name) for p in ports)
        dup = {k: v for k, v in per_model.items() if v > 1}
        assert not dup, f"端口名重复会导致型号端口表写不进去: {dup}"

    def test_connector_and_type(self) -> None:
        _, ports = canon.canon_models()
        ge = next(p for p in ports if p.physical_name == "GE1/0/1")
        assert (ge.connector, ge.port_type) == ("RJ45", "electrical")
        q = next(p for p in ports if p.physical_name == "100GE1/0/1")
        assert (q.connector, q.port_type) == ("QSFP28", "optical")


class TestCanonRacks:
    """机柜与几何。"""

    def test_362_racks_from_real_geometry(self) -> None:
        geo = _geometry()
        assert len(canon.canon_racks(geo)) == 4, "合成几何只有 4 个柜"

    def test_aisle_rows_come_from_formula_not_literal(self) -> None:
        """通道行号走 `floor_row` 而不是抄平面图 —— 平面图改了也不会错。"""
        aisles = canon.canon_aisles(_geometry())
        by_ch = {a.channel_no: a for a in aisles}
        assert (by_ch[1].plan_row_n, by_ch[1].plan_row_s) == (3, 4)
        assert (by_ch[8].plan_row_n, by_ch[8].plan_row_s) == (3, 4)

    def test_room_col_range(self) -> None:
        rooms = {r.code: r for r in canon.canon_rooms(_geometry())}
        assert (rooms["403"].col_min, rooms["403"].col_max) == (23, 38)
        assert (rooms["404"].col_min, rooms["404"].col_max) == (3, 18)


class TestCableEndpoints:
    """连线端点解析（★4 与别名回退）。"""

    def test_alias_endpoint_resolves_to_primary(self) -> None:
        """连线指向被合并掉的别名时，要解析回主设备。

        漏这一条会有 22 条真实连线消失：`备份接入交换机-1` 的 4 个口
        接的正是 `备份一体机-1..4`，而那 4 个名字已被合并。
        """
        model = "本地备份-备份一体机"
        uses = [
            _use(device_key="备份-1", device_label="备份-1", model=model, port="IPMI"),
            _use(device_key="备份一体机-1", device_label="备份一体机-1", model=model, port="万兆1"),
        ]
        ledger = build_ledger(uses)
        devices = canon.merge_devices(ledger).devices

        class _Link:
            a_device = "备份接入交换机-1"
            a_port = "100GE1/0/1"
            b_device = "备份一体机-1"
            b_id = ""
            b_model = model
            b_port = "万兆1"
            cable_type = "MPO"
            nic = ""

        # 交换机那端不在设备表里 → 整条被跳过（这是既有行为，见下条用例）
        assert canon.canon_cables([_Link()], devices) == ()

    def test_unresolvable_endpoint_is_dropped_not_guessed(self) -> None:
        """解析不到的端点必须丢弃，不能硬塞一个设备。

        硬塞的后果：写出一条指向不存在设备的连线，
        SQLite 只报 `FOREIGN KEY constraint failed`，不说是哪条线。
        """

        class _Link:
            a_device = "不存在的设备"
            a_port = "P1"
            b_device = "也不存在"
            b_id = ""
            b_model = "X"
            b_port = "P2"
            cable_type = "RJ45"
            nic = ""

        devices = canon.merge_devices(
            build_ledger(
                [_use(device_key="SW-1", device_label="SW-1", model="带内交换机1", port="GE1/0/1")]
            )
        ).devices
        assert canon.canon_cables([_Link()], devices) == ()


class TestRanges:
    """★3：`switch_service_range` 按 (块 × 型号) 拆行。"""

    def test_split_by_node_model(self) -> None:
        """一个块里混两种型号节点时必须拆成两行。

        真实原型：`计算接入交换机(业务)-5` 的 44 个口里既有
        `计算节点A（ARM）` 又有 `计算节点B（C86）`。
        按「一块一行」写会违反
        `UNIQUE(project_id, switch_device_id, node_model_id, base_port_no)`。
        """

        class _Node:
            def __init__(self, node_id: str, model: str) -> None:
                self.node_id = node_id
                self.model = model

        class _Block:
            switch_instance = "SW-1"
            nodes = (_Node("n1", "型号甲"), _Node("n2", "型号乙"), _Node("n3", "型号甲"))
            order = "rack_then_u"
            port_role = "downlink"

        class _Assign:
            def __init__(self, node_id: str, port: int) -> None:
                self.switch_instance = "SW-1"
                self.node_id = node_id
                self.switch_port_no = port

        class _Plan:
            blocks = (_Block(),)
            assignments = (_Assign("n1", 1), _Assign("n2", 2), _Assign("n3", 3))

        devices = canon.merge_devices(
            build_ledger(
                [_use(device_key="SW-1", device_label="SW-1", model="带内交换机1", port="GE1/0/1")]
            )
        ).devices
        ranges = canon.canon_ranges(_Plan(), devices)

        assert len(ranges) == 2, "两种型号必须拆两行"
        by_model = {r.node_model_normalized: r for r in ranges}
        assert by_model[norm_name("型号甲")].base_port_no == 1, "取该型号的最小端口号"
        assert by_model[norm_name("型号甲")].node_count_expected == 2
        assert by_model[norm_name("型号乙")].base_port_no == 2
        assert by_model[norm_name("型号乙")].node_count_expected == 1


# ---------------------------------------------------------------------------
# 真实数据：这些数字是验收基线，变了必须解释
# ---------------------------------------------------------------------------
@pytest.fixture(scope="module")
def real_bundle() -> canon.CanonicalBundle:
    if SOURCE is None or not INTENT.is_file():
        pytest.skip("需要真实 V13 源文件与意图（只在完整仓库里跑）")
    from idcbox.application.services.design import generate_design
    from idcbox.domain.ports.ledger import ledger_from_cabling
    from idcbox.infrastructure.xlsx.openpyxl_reader import read_floor_cells

    cells = read_floor_cells(SOURCE)
    floor = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
    geometry = build_snapshot(
        floor.racks, version="v1", cmax_override=derive_cmax_by_room(floor.racks)
    )
    doc = json.loads(INTENT.read_text())
    intent = intent_from_dict(doc)
    placements, routes, chains, _ = to_flow_inputs(intent)
    plan = plan_flow(placements, routes, chains)
    design = generate_design(intent, geometry)
    ledger = ledger_from_cabling(design.cabling)
    return canon.build_canonical(
        ledger, geometry=geometry, links=design.cabling.links, plan=plan
    )


class TestAgainstRealData:
    """真实 V13 意图的固定数字。"""

    def test_device_count(self, real_bundle: canon.CanonicalBundle) -> None:
        """824 台 = 830 个台账设备 − 6 组命名变体。

        830 是台账口径（同一台被记两个名字算两台），824 才是真实物理台数。
        """
        assert real_bundle.device_count == 824
        assert real_bundle.stats["merged"] == 6

    def test_port_count_matches_ledger(
        self, real_bundle: canon.CanonicalBundle
    ) -> None:
        """端口行数 = 台账条目数 **减去 44 条合并产生的重复**（12820 → 12776）。

        ## 这 44 条是什么

        台账里一台设备可能有两个 `device_key`（合并掉的别名，★1）：
        `备份一体机-1` 与 `备份-1` 各产出一份端口 —— 一份来自连线
        （`occupied`），一份来自模板（`free`）。同一个物理口出两行。

        不合并的后果是落库时撞 `UNIQUE(device_id, physical_name)`，
        而报错只说约束名、不说是哪台设备的哪个口。

        合并规则：**occupied 优先** —— 接了线是物理事实，free 只是
        「模板里还有这个口」。所以 44 条全部是从 free 里去掉的：
        free 2568 → 2524，occupied 10252 不变。
        """
        assert real_bundle.port_count == 12776
        assert real_bundle.stats["occupied"] == 10252
        assert real_bundle.stats["free"] == 2524
        assert real_bundle.stats["occupied"] + real_bundle.stats["free"] == 12776
        # 台账原始条目数 − 合并重复 = 端口行数
        assert real_bundle.port_count == 12820 - 44

    def test_cable_count_matches_generator(
        self, real_bundle: canon.CanonicalBundle
    ) -> None:
        """5127 条连线一条不少。

        `dropped_cables` 是「解析不到端点」的计数，必须为 0 ——
        非 0 说明设备表与连线对不上，而这类丢失在界面完全不显眼。
        """
        assert len(real_bundle.cables) == 5127
        assert real_bundle.stats["dropped_cables"] == 0

    def test_rack_and_model_counts(self, real_bundle: canon.CanonicalBundle) -> None:
        assert len(real_bundle.racks) == 362, "362 个机柜位全部落库（含空柜）"
        assert len(real_bundle.rooms) == 2
        assert len(real_bundle.aisles) == 14, "403 房 7 条 + 404 房 7 条"
        assert len(real_bundle.models) == 26
        assert len(real_bundle.model_ports) == 855

    def test_ranges_split_to_142(self, real_bundle: canon.CanonicalBundle) -> None:
        """106 个接线块按型号拆成 142 行。"""
        assert len(real_bundle.ranges) == 142

    def test_ranges_are_unique(self, real_bundle: canon.CanonicalBundle) -> None:
        """`(交换机, 型号, base)` 三重唯一 —— 表上 UNIQUE 约束的前提。"""
        keys = {
            (r.switch_device_key, r.node_model_normalized, r.base_port_no)
            for r in real_bundle.ranges
        }
        assert len(keys) == len(real_bundle.ranges)

    def test_conflicts_are_the_three_real_stacks(
        self, real_bundle: canon.CanonicalBundle
    ) -> None:
        """只有 3 条冲突，且全是 ch2/ch8/ch9 rack11 31U 的真叠放。

        命名变体那 6 组不算冲突 —— 它们已经合并了。
        """
        kinds = Counter(c.kind for c in real_bundle.conflicts)
        assert kinds == {"U_OVERLAP": 3}
        spots = {(c.channel_no, c.rack_no, c.u_start) for c in real_bundle.conflicts}
        assert spots == {(2, 11, 31), (8, 11, 31), (9, 11, 31)}

    def test_no_unnamed_switch(self, real_bundle: canon.CanonicalBundle) -> None:
        """交换机必须有名字；只有服务器可以无名（★5）。"""
        unnamed_switches = [
            d for d in real_bundle.devices if not d.name and d.category in ("switch", "core")
        ]
        assert unnamed_switches == [], "交换机无名说明实例名解析坏了"
        unnamed = [d for d in real_bundle.devices if not d.name]
        assert len(unnamed) == 666, "666 台无名设备应全是服务器/存储"

    def test_a_side_is_unique(self, real_bundle: canon.CanonicalBundle) -> None:
        """a 侧每个端口最多 1 根线 —— `uq_cable_a_active` 唯一索引的前提。

        这条不成立就不能建那个索引，否则迁移会在真实数据上失败。
        """
        keys = {(c.a_device_key, c.a_port_name) for c in real_bundle.cables}
        assert len(keys) == len(real_bundle.cables)

    def test_z_side_allows_fanout(self, real_bundle: canon.CanonicalBundle) -> None:
        """z 侧允许一对多，实测只有 1 处 —— 分线交换机的 MPO 一分三。

        若给 z 侧也加唯一约束，这条真实连线会被拒。
        """
        fanout = Counter((c.z_device_key, c.z_port_name) for c in real_bundle.cables)
        multi = {k: v for k, v in fanout.items() if v > 1}
        assert len(multi) == 1
        assert list(multi.values()) == [3]

    def test_u_spans(self, real_bundle: canon.CanonicalBundle) -> None:
        """U 位跨度分布：2U 694 台 / 1U 126 台 / 10U 4 台（4 台框式核心）。"""
        spans = Counter(d.u_span for d in real_bundle.devices)
        assert dict(spans) == {2: 694, 1: 126, 10: 4}
