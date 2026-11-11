"""全通道意图黄金用例：207 行意图 → 4430 行布线表，覆盖计算 + 存储两类通道。

## 与 `test_golden_intent.py` 的分工（互补，不重复）

- `test_golden_intent.py`：**计算通道**（ch2/ch8/ch9），3850 行，132 行意图，
  验证 `rack_major` 排布 + 相位轮转 + 重复槽位顺延。
- 本文件（**全通道**）：额外覆盖 **ch3 存储通道**（580 行）+ 其余通道的
  特殊机型布局，207 行意图，4430 行。

全量 4562 行里，4430 行（**97.11%**）由规则覆盖；剩余 132 行是
`网络功能服务器` / `防火墙` / `管理节点服务器` / `GSO` 等「一台一种接法」的
特殊机型，必须逐台显式声明（见 `Intent.explicit_links`），这是意图层的
**诚实边界**，不是缺陷 —— 详见 `test_special_models_need_explicit_links`。

## 本文件抓出来的两个关键规律（都已固化进 flow.py / intent.py）

### 1. `ch3` 存储通道是 `slot_major`（槽位外层、机柜内层）
ch2/ch8/ch9 的端口序列是「机柜 1 的 8 个槽位 → 机柜 2 的 8 个槽位」，
而 ch3 是「所有机柜的槽位 0 → 所有机柜的槽位 1 → …」。
用 `rack_major` 跑 ch3 只有 **17.24%** 命中，换成 `slot_major` 后 100%。
因此 `ChainSpec.order` 是**每通道可独立声明**的，不能全局写死。

### 2. `ch3` 的块顺序 = `(最小机柜号, 该型号首次出现机柜)`
`slot_major` 下块的**声明顺序就是接线顺序**，无法用一维坐标键推导。
实测 SSM/EHDD 的块在机柜区间上重叠（SSD 占 r1-4 全槽 + r5-8 的槽0/1，
EHDD1 占 r5-8 的槽2-7），只靠「最小机柜号」不足，必须再用
「该型号自身最早出现的机柜」做 tie-break。
"""

from __future__ import annotations

import json
from collections import Counter, defaultdict
from pathlib import Path

import pytest

from idcbox.domain.ports.flow import plan_flow
from idcbox.domain.ports.intent import (
    Intent,
    intent_from_dict,
    to_flow_inputs,
)
from idcbox.domain.ports.naming import parse_port_name
from idcbox.domain.ports.phase import slot_of
from paths import source_path

#: 意图文件放在 `projects/<项目>/intent.json`，与产线共用同一份 ——
#: 测试读的就是用户会改的那个文件，避免"fixture 与真货漂移"。
INTENT_FULL_PATH = Path(__file__).resolve().parents[3] / "projects" / "v13" / "intent.json"

#: 由规则自动分配的机型（其余走 `explicit_links`）。
#: 依据：这 6 个型号的「网口→交换机型号」指纹在型号内一致，
#: 且排布满足相位/槽位主轴规律。
RULE_COVERED = frozenset(
    {
        "计算节点A（ARM）",
        "计算节点B（C86）",
        "SSD云硬盘服务器",
        "EHDD云硬盘服务器1",
        "EHDD云硬盘服务器2",
        "对象存储数据节点服务器",
    }
)


def _nm(v: object) -> str:
    return str(v).strip() if v is not None else ""


def _u_start(raw: str) -> int:
    s = str(raw).upper().replace("U", "").replace(" ", "")
    return int(s.split("-")[0]) if s else 0


@pytest.fixture(scope="module")
def all_cabling() -> list[dict]:
    """布线表全量「交换机在左、服务器在右」的行（4562 行，不筛机型）。"""
    import openpyxl

    ws = openpyxl.load_workbook(source_path(), data_only=True)["布线表"]
    out: list[dict] = []
    for r in range(1, ws.max_row + 1):
        v = [_nm(ws.cell(row=r, column=c).value) for c in range(1, 25)]
        if not v[0] and not v[6]:
            continue
        lp = parse_port_name(v[5])
        pp = parse_port_name(v[11])
        if lp.style != "cli_board" or pp.style != "label" or lp.port is None:
            continue
        out.append(
            {
                "row": r,
                "sw_instance": v[1],
                "sw_model": v[0],
                "sw_port_no": lp.port,
                "node_model": v[6],
                "channel": int(v[8]) if v[8] else 0,
                "rack": int(v[9]) if v[9] else 0,
                "u_start": _u_start(v[10]),
                "nic": v[11],
            }
        )
    return out


@pytest.fixture(scope="module")
def intent_full() -> Intent:
    """全通道接线意图（反解自 V13，已固化）。"""
    data = json.loads(INTENT_FULL_PATH.read_text(encoding="utf-8"))
    return intent_from_dict(data)


@pytest.fixture(scope="module")
def plan_full(intent_full: Intent):
    placements, routes, chains, issues = to_flow_inputs(intent_full)
    assert not [i for i in issues if i.severity == "error"], "意图自身不自洽"
    return plan_flow(placements, routes, chains)


@pytest.fixture(scope="module")
def covered_rows(all_cabling: list[dict]) -> list[dict]:
    """规则可覆盖机型的行（4430 行）。"""
    return [x for x in all_cabling if x["node_model"] in RULE_COVERED]


@pytest.fixture(scope="module")
def generated(plan_full, covered_rows: list[dict]) -> dict[tuple, object]:
    """`(交换机实例, 端口号, 网口)` → 生成的节点（只保留覆盖机型的网口）。"""
    nics = frozenset(x["nic"] for x in covered_rows)
    out: dict[tuple, object] = {}
    for b in plan_full.blocks:
        if b.nic not in nics:
            continue
        for i, node in enumerate(b.nodes):
            out[(b.switch_instance, i + 1, b.nic)] = node
    return out


# ---------------------------------------------------------------------------
# 1. 意图体积与覆盖边界
# ---------------------------------------------------------------------------
def test_intent_full_volume(intent_full: Intent) -> None:
    """207 行意图覆盖全量 4562 行里 4430 行（97.11%）。

    207 = 28 布局块 + 119 交换机 + 34 路由 + 26 交换机组。
    其中 119 台交换机的型号+位置本来就属上架规划，接线新增只有 28+34+26=88 行。
    """
    total = (
        len(intent_full.racks)
        + len(intent_full.switches)
        + len(intent_full.routes)
        + len(intent_full.chains)
    )
    assert total == 207
    assert len(intent_full.racks) == 28
    assert len(intent_full.switches) == 119
    assert len(intent_full.routes) == 34
    assert len(intent_full.chains) == 26


def test_four_channels_are_declared(intent_full: Intent) -> None:
    """4 个通道全部有布局段：ch2/ch8/ch9 计算 + ch3 存储。"""
    chans = Counter(r.channel_no for r in intent_full.racks)
    assert set(chans) == {2, 3, 8, 9}
    assert chans[3] == 7, "存储通道 7 个布局块"


def test_coverage_boundary_is_honest(covered_rows: list[dict], all_cabling: list[dict]) -> None:
    """4430/4562 = 97.11% 由规则覆盖，剩余 132 行必须显式声明。

    这个数字是意图层的**能力边界**，写进用例防止以后有人悄悄把
    「特殊机型走规则」当成支持了 —— 实测那 132 行只有 13.64% 能蒙对。
    """
    n_ok = len(covered_rows)
    n_all = len(all_cabling)
    assert n_all == 4562
    assert n_ok == 4430
    assert round(n_ok / n_all * 100, 2) == 97.11


def test_special_models_need_explicit_links(
    covered_rows: list[dict], all_cabling: list[dict]
) -> None:
    """特殊机型共 132 行，规则只能蒙对约 13%，必须显式声明。"""
    special = [x for x in all_cabling if x["node_model"] not in RULE_COVERED]
    assert len(special) == 132
    models = Counter(x["node_model"] for x in special)
    # 网络功能服务器是最大的一块：一台一种接法（半配机型、NAT 分流）
    assert models["网络功能服务器"] == 48
    assert models["本地备份-备份一体机"] == 28
    assert models["GSO独立部署服务器"] == 21
    assert models["管理节点服务器"] == 15
    assert models["服务器密码机"] == 4
    assert models["签名服务器"] == 4
    assert models["数据库加密机"] == 4
    assert models["广域网负载均衡"] == 4
    assert models["防火墙1"] == 2
    assert models["防火墙2"] == 2
    assert sum(models.values()) == 132


# ---------------------------------------------------------------------------
# 2. 核心：逐条端口完全复现（4430/4430）
# ---------------------------------------------------------------------------
def test_intent_full_reproduces_every_port(
    covered_rows: list[dict], generated: dict[tuple, object]
) -> None:
    """核心用例：4430 行逐条比对「型号 + 机柜 + 槽位」，必须 0 差异。

    比对键是 `(交换机实例, 端口号, 网口)`，不是 `(交换机实例, 端口号)` ——
    同一台交换机不同网口的同号端口是**不同的物理口**（万兆1/3、千兆1/3、
    IPMI 各占一面），键里少一个维度会把它们混成一条。
    """
    ok = bad = miss = 0
    fails: list[str] = []
    for x in covered_rows:
        key = (x["sw_instance"], x["sw_port_no"], x["nic"])
        node = generated.get(key)
        want = (x["node_model"], x["rack"], slot_of(x["u_start"]))
        if node is None:
            miss += 1
            if len(fails) < 5:
                fails.append(f"缺失 {key} 应为 {want}")
            continue
        got = (node.model, node.rack_no, node.slot)
        if got == want:
            ok += 1
        else:
            bad += 1
            if len(fails) < 5:
                fails.append(f"错位 {key}：应为 {want}，实为 {got}")

    assert not fails, "端口分配未完全复现：\n" + "\n".join(fails)
    assert ok == 4430
    assert bad == 0
    assert miss == 0


def test_no_extra_assignments_on_covered_nics(
    generated: dict[tuple, object], covered_rows: list[dict]
) -> None:
    """覆盖机型所在的网口上不能多出分配（会白占端口）。"""
    nics = frozenset(x["nic"] for x in covered_rows)
    truth = {(x["sw_instance"], x["sw_port_no"], x["nic"]) for x in covered_rows}
    extra = [k for k in generated if k not in truth]
    assert not extra, f"多出 {len(extra)} 条分配，例如 {extra[:3]}"


def test_total_assignment(plan_full) -> None:
    """4430 条分配 + 106 块，块数 = 真实交换机组数。"""
    assert plan_full.total_assignment == 4430
    assert len(plan_full.blocks) == 106


# ---------------------------------------------------------------------------
# 3. ch3 存储通道的两条规律（这是本文件相对 test_golden_intent.py 的新增）
# ---------------------------------------------------------------------------
def test_storage_channel_uses_slot_major(intent_full: Intent) -> None:
    """ch3 的 5 条链必须声明 `order="slot_major"`，其余 21 条是 `rack_major`。

    ch3 的端口序列是「槽位外层、机柜内层」，与计算通道相反。
    用 rack_major 跑 ch3 只有 17.24% 命中。
    """
    by_order = defaultdict(list)
    for c in intent_full.chains:
        by_order[c.order].append(c)
    assert len(by_order["slot_major"]) == 5
    assert len(by_order["rack_major"]) == 21
    assert all(c.channel_no == 3 for c in by_order["slot_major"])
    assert not any(c.channel_no == 3 for c in by_order["rack_major"])


def test_storage_capacity_is_not_uniform(intent_full: Intent) -> None:
    """存储通道容量实测 `[40,40,40,40,36,36]`，不是计算通道的 44。

    写死 44 会让 [40,40,36] 错成 [44,44,28] 全盘错位 —— 容量必须由意图声明。
    """
    caps = {c.capacity for c in intent_full.chains if c.channel_no == 3}
    assert caps == {40}, "存储接入面 40 口（6 台里 4 台 40 口、2 台 36 口，末台吸收余量）"
    compute_caps = {c.capacity for c in intent_full.chains if c.channel_no != 3}
    assert compute_caps == {44}, "计算接入面 44 口（末台吸收余量）"


def test_storage_capacity_comes_from_first_switch(intent_full: Intent) -> None:
    """链容量 = 链中**第一台**交换机的端口面容量（末台才是吸收余量的那台）。

    实测 ch3 的 `25G1` 链三台是 40/40/36，若取中位数会得到 40 但
    ch9 的万兆1 三台是 44/43/46，取中位数得 43 → 全盘错位。
    """
    ch3 = [c for c in intent_full.chains if c.channel_no == 3 and c.nic == "25G1"]
    assert len(ch3) == 1
    assert ch3[0].capacity == 40
    assert ch3[0].instances[:1] == ("存储接入交换机-1",)


def test_block_sizes_absorb_remainder_on_storage() -> None:
    """存储通道块大小必须能吸收余量：`(128, 4, 40) → [40,40,40,8]`。"""
    from idcbox.domain.ports.flow import block_sizes

    assert block_sizes(128, 4, 40) == [40, 40, 40, 8]
    assert block_sizes(120, 3, 40) == [40, 40, 40]
    assert block_sizes(116, 3, 40) == [40, 40, 36]


# ---------------------------------------------------------------------------
# 4. 布局层：非标准机柜必须能声明
# ---------------------------------------------------------------------------
def test_abnormal_racks_declared_in_full_intent(intent_full: Intent) -> None:
    """4 个非标准机柜（3 个 rack11 叠放 + ch9 rack17 少 2 台）全部声明。

    不声明时命中率从 100% 掉到 97.81%：少 1 台会让后面所有机柜的
    相位轮转 `carry` 偏移，整段错位。
    """
    # 3 个 rack11 叠放用 slot_map（同柜槽位重复，块级 slots 不允许重复）
    with_map = [r for r in intent_full.racks if r.slot_map]
    assert len(with_map) == 3
    assert sorted(r.channel_no for r in with_map) == [2, 8, 9]
    for r in with_map:
        assert r.rack_from == 11
        assert r.device_count == 9, "rack11 的 31-32U 叠了两台"

    # 其余非标准布局用块级 slots（整块机柜共用同一槽位集，但非默认 8 槽）
    non_std = [r for r in intent_full.racks if r.slots and not r.slot_map]
    assert len(non_std) == 13

    # ch9 rack17 只有 6 台（不是 8），必须靠 slots 声明
    r17 = [r for r in non_std if r.channel_no == 9 and r.rack_from == 17]
    assert len(r17) == 1 and r17[0].device_count == 6

    # 3 个计算通道的 rack7 都只有 7 台（ch2/ch8/ch9 各一）
    r7 = [r for r in non_std if r.rack_from == 7 and r.channel_no in (2, 8, 9)]
    assert len(r7) == 3
    assert all(r.device_count == 7 for r in r7)

    # ch9 rack3 是 A/B 混装：B 占槽 0-2、A 占槽 3-7，两块拼接成 8 槽
    mix = [r for r in non_std if r.channel_no == 9 and r.rack_from == 3]
    assert len(mix) == 2
    assert {r.node_model for r in mix} == {"计算节点A（ARM）", "计算节点B（C86）"}
    by_model = {r.node_model: r.slots for r in mix}
    assert by_model["计算节点B（C86）"] == (0, 1, 2)
    assert by_model["计算节点A（ARM）"] == (3, 4, 5, 6, 7)

    # ch3 存储 6 个型号区间块：SSD 两段 + EHDD1 两段 + EHDD2 两段 + 对象存储
    ch3 = [r for r in non_std if r.channel_no == 3]
    assert len(ch3) == 7
    # 每块设备数实测：24 + 8 + 16 + 16 + 8 + 24 + 20 = 116 台
    assert [r.device_count for r in ch3] == [24, 8, 16, 16, 8, 24, 20]
    assert sum(r.device_count for r in ch3) == 116


def test_overlapping_rack_blocks_are_allowed(intent_full: Intent) -> None:
    """ch3 的块**允许机柜区间重叠**（SSD 占 r1-4 全槽，EHDD1 占 r5-8 的槽2-7）。

    这意味着布局层不能用「机柜区间不重叠」做校验 —— 重叠是真实规划形态。
    """
    ch3 = [r for r in intent_full.racks if r.channel_no == 3]
    ranges = [(r.rack_from, r.rack_to) for r in ch3]
    overlapping = any(
        ranges[i][0] <= ranges[j][1] and ranges[j][0] <= ranges[i][1]
        for i in range(len(ranges))
        for j in range(i + 1, len(ranges))
    )
    assert overlapping, "ch3 存储布局本应存在机柜区间重叠"


# ---------------------------------------------------------------------------
# 5. 序列化与稳定性
# ---------------------------------------------------------------------------
def test_full_intent_roundtrip(intent_full: Intent) -> None:
    """全通道意图 JSON 往返无损（含 slot_major / capacity / slot_map）。"""
    from idcbox.domain.ports.intent import intent_to_dict

    again = intent_from_dict(json.loads(json.dumps(intent_to_dict(intent_full))))
    assert again == intent_full


def test_full_json_is_in_sync_with_loader(intent_full: Intent) -> None:
    """磁盘上的 JSON 与 loader 解析出的对象一致（防手改不同步）。"""
    data = json.loads(INTENT_FULL_PATH.read_text(encoding="utf-8"))
    assert data["schema"] == 1
    assert data["project"] == intent_full.project
    assert len(data["racks"]) == len(intent_full.racks)
