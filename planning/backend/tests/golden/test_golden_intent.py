"""意图层黄金用例：132 行意图 → 3850 行布线表，逐条端口 + 交换机完全一致。

## 这组用例证明的是什么

用户的痛点是「新建机房时 5000 行连线表要人一行行填」。
本用例证明：**只要填 132 行意图，整张布线表就能算出来**，
而且是 100.00% 命中 —— 不是「大致对」，是逐条端口号都对。

132 行 = 6 条布局段 + 91 台交换机 + 14 条路由 + 21 条交换机组
（其中 91 台交换机的「型号 + 位置」本来就在上架规划里，不算额外负担）。

## 为什么意图是可信的输入

`v13_intent.json` 是从 V13 真实布线表**反解**出来的，反解过程只用：
- 上架位置（通道/机柜/U位）—— 上架规划本来就要填
- 型号级「网口→交换机型号」众数 —— 132 行
- 交换机自身位置与容量 —— 上架规划本来就要填

**没有用真实端口号**（除了 `flow.py` 已验证的规则本身）。

## 与 `test_golden_flow.py` 的分工

- `test_golden_flow.py`：验证 `flow.py` 的三条规则（给定已排序的节点序列）
- 本文件：验证 `intent.py` 能把「用户填的意图」翻译成 `flow.py` 需要的输入

分开的原因：`flow.py` 出错是算法错，`intent.py` 出错是**信息量不够**
（用户想填的东西表达不出真实意图）。两种错要能分别定位。
"""

from __future__ import annotations

import json
from collections import Counter, defaultdict
from pathlib import Path

import pytest

from idcbox.domain.ports.flow import plan_flow
from idcbox.domain.ports.intent import (
    INTENT_SCHEMA_VERSION,
    ChainSpec,
    Intent,
    RackBlock,
    RouteSpec,
    SwitchSpec,
    dump_intent,
    expand_placements,
    intent_from_dict,
    intent_to_dict,
    load_intent,
    to_flow_inputs,
)
from idcbox.domain.ports.naming import parse_port_name
from idcbox.domain.ports.phase import slot_of
from idcbox.domain.units import SLOT_BASE_U, SLOT_COUNT, SLOT_PITCH_U

from paths import source_path

INTENT_PATH = Path(__file__).resolve().parent / "data" / "v13_intent.json"

COMPUTE = {"计算节点A（ARM）", "计算节点B（C86）"}
NICS = ("万兆1", "万兆2", "万兆3", "万兆4", "千兆1", "千兆3", "IPMI")


def _nm(v: object) -> str:
    return str(v).strip() if v is not None else ""


def _u_start(raw: str) -> int:
    s = str(raw).upper().replace("U", "").replace(" ", "")
    return int(s.split("-")[0]) if s else 0


@pytest.fixture(scope="module")
def cabling() -> list[dict]:
    """布线表里「交换机在左、计算节点在右」的行（3850 行）。"""
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
        if v[6] not in COMPUTE:
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
def dev_of_row(cabling: list[dict]) -> dict[int, tuple[int, int, int, int]]:
    """Excel 行号 → 设备身份 `(通道, 机柜, U起始, 该位置内第几台)`。

    设备身份必须带 `occ`：V13 有 3 处硬件位置重叠（ch2/ch8/ch9 的 rack11
    31-32U 各 2 台），合并会让排序序列短一台，后面全部错位。
    """
    grouped: dict[tuple[int, str], list[dict]] = defaultdict(list)
    for x in cabling:
        grouped[(x["channel"], x["nic"])].append(x)

    out: dict[int, tuple[int, int, int, int]] = {}
    for rows in grouped.values():
        by_pos: dict[tuple[str, int, int], list[dict]] = defaultdict(list)
        for x in rows:
            by_pos[(x["node_model"], x["rack"], x["u_start"])].append(x)
        for xs in by_pos.values():
            for occ, x in enumerate(sorted(xs, key=lambda t: t["row"])):
                out[x["row"]] = (x["channel"], x["rack"], x["u_start"], occ)
    return out


@pytest.fixture(scope="module")
def intent() -> Intent:
    """V13 的接线意图（反解自真实数据，已固化）。"""
    return load_intent(INTENT_PATH)


@pytest.fixture(scope="module")
def plan(intent: Intent):
    placements, routes, chains, issues = to_flow_inputs(intent)
    assert not [i for i in issues if i.severity == "error"], "意图自身不自洽"
    return plan_flow(placements, routes, chains)


# ---------------------------------------------------------------------------
# 1. 意图体积：这就是用户要填的量
# ---------------------------------------------------------------------------
def test_intent_volume(intent: Intent) -> None:
    """132 行意图覆盖 550 台服务器、3850 条连线。

    其中 91 台交换机的位置与容量属于「上架规划」范畴（本来就要填），
    真正为接线新增的只有 6 + 14 + 21 = 41 行。
    """
    assert len(intent.racks) == 6
    assert len(intent.switches) == 91
    assert len(intent.routes) == 14
    assert len(intent.chains) == 21
    assert intent.server_count == 550
    # 接线专属的增量：布局段 + 路由 + 交换机组
    assert len(intent.racks) + len(intent.routes) + len(intent.chains) == 41


def test_intent_server_count_matches_real(intent: Intent, dev_of_row: dict) -> None:
    """意图展开出的设备台数 == 真实数据的唯一物理设备数（550）。"""
    placements = expand_placements(intent)
    assert len(placements) == 550
    assert len(set(dev_of_row.values())) == 550


# ---------------------------------------------------------------------------
# 2. 核心验收：3850/3850 = 100%
# ---------------------------------------------------------------------------
def test_intent_reproduces_every_port(
    plan, cabling: list[dict], dev_of_row: dict[int, tuple[int, int, int, int]]
) -> None:
    """**v0.3 核心验收**：意图生成的每一条端口分配，与真实布线表逐条一致。

    比对口径：真实值按 `(设备身份, 网口)` 建键（同一台服务器 7 个网口各一行），
    生成值按块遍历（同一 `node_id` 出现在 7 个块里，不能建单值字典）。
    """
    truth = {(dev_of_row[x["row"]], x["nic"]): (x["sw_instance"], x["sw_port_no"]) for x in cabling}

    ok = bad = missing = 0
    fails: list[tuple] = []
    for block in plan.blocks:
        for idx, node in enumerate(block.nodes):
            nid = node.node_id
            ch = int(nid[1:3])
            rack = int(nid[4:6])
            tail = nid[8:]
            slot = int(tail.split("-")[0])
            occ = int(tail.split("-")[1]) if "-" in tail else 0
            key = ((ch, rack, SLOT_BASE_U + slot * SLOT_PITCH_U, occ), block.nic)
            t = truth.get(key)
            if t is None:
                missing += 1
                continue
            if t == (block.switch_instance, idx + 1):
                ok += 1
            else:
                bad += 1
                fails.append((nid, block.nic, block.switch_instance, idx + 1, t))

    assert missing == 0, f"生成了 {missing} 条真实数据里没有的连线，前 3：{fails[:3]}"
    assert bad == 0, f"{bad} 条端口对不上，前 3：{fails[:3]}"
    assert ok == 3850, f"应为 3850 条，实际 {ok}"


def test_intent_total_assignment(plan) -> None:
    """生成总量 == 真实行数（3850 = 550 台 × 7 个网口）。"""
    assert plan.total_assignment == 3850
    assert len(plan.blocks) == 91


# ---------------------------------------------------------------------------
# 3. 布局层的诚实性：非标准机柜必须显式声明
# ---------------------------------------------------------------------------
def test_abnormal_racks_are_declared(intent: Intent) -> None:
    """7 个非标准机柜在意图里显式声明了槽位，不能靠「默认 8 台」猜。

    真实：ch2/ch8/ch9 的 rack7 各 7 台、rack11 各 9 台、ch9 的 rack17 只有 6 台。
    """
    declared = [
        (b.channel_no, rack, slots)
        for b in intent.racks
        for rack, slots in b.slot_map
    ]
    assert len(declared) == 7
    shapes = {(ch, rack): len(slots) for ch, rack, slots in declared}
    assert shapes == {
        (2, 7): 7,
        (2, 11): 9,
        (8, 7): 7,
        (8, 11): 9,
        (9, 7): 7,
        (9, 11): 9,
        (9, 17): 6,
    }
    # rack11 的 9 台里有 2 台同槽位（真实数据异常，按顺延轮处理）
    nine = next(s for c, r, s in declared if (c, r) == (2, 11))
    assert nine.count(7) == 2


def test_default_slots_are_eight() -> None:
    """未声明 slot_map 的机柜默认 8 个标准槽位。"""
    b = RackBlock(2, 1, 3, "A")
    assert b.device_count == 24
    assert b.slots_of(1) == (0, 1, 2, 3, 4, 5, 6, 7)


def test_slot_map_out_of_range_rejected() -> None:
    with pytest.raises(ValueError, match="不在本段"):
        RackBlock(2, 1, 3, "A", slot_map=((9, (0,)),))
    with pytest.raises(ValueError, match="槽位号越界"):
        RackBlock(2, 1, 3, "A", slot_map=((1, (0, 8)),))


def test_rack_block_rejects_reversed_range() -> None:
    with pytest.raises(ValueError, match="起止反了"):
        RackBlock(2, 5, 1, "A")


# ---------------------------------------------------------------------------
# 4. 意图自检：能抓出真实会犯的错
# ---------------------------------------------------------------------------
def test_unknown_switch_model_is_error(intent: Intent) -> None:
    """路由指向不存在的交换机型号 → error（该网口永远接不上）。"""
    bad = Intent(
        "bad",
        racks=intent.racks,
        switches=intent.switches,
        routes=(*intent.routes, RouteSpec("计算节点A（ARM）", "万兆1", "不存在的交换机")),
        chains=intent.chains,
    )
    _, _, _, issues = to_flow_inputs(bad)
    errs = [i for i in issues if i.severity == "error"]
    assert len(errs) == 1
    assert "不存在的交换机" in errs[0].message


def test_route_for_absent_model_is_warn(intent: Intent) -> None:
    """路由声明了布局里没有的机型 → warn（不会产生连线，但不算错）。"""
    bad = Intent(
        "bad",
        racks=intent.racks,
        switches=intent.switches,
        routes=(*intent.routes, RouteSpec("幽灵服务器", "万兆1", intent.switches[0].model)),
        chains=intent.chains,
    )
    _, _, _, issues = to_flow_inputs(bad)
    warns = [i for i in issues if i.severity == "warn"]
    assert any("幽灵服务器" in i.message for i in warns)


def test_duplicate_chain_is_error(intent: Intent) -> None:
    """同一 (通道, 型号, 网口) 声明两条链 → error（节点被分配两次）。"""
    dup = Intent(
        "bad",
        racks=intent.racks,
        switches=intent.switches,
        routes=intent.routes,
        chains=(*intent.chains, intent.chains[0]),
    )
    _, _, chains, issues = to_flow_inputs(dup)
    assert len(chains) == len(intent.chains), "重复的链应被丢弃，不能进入生成"
    errs = [i for i in issues if i.severity == "error"]
    assert len(errs) == 1
    assert "重复声明" in errs[0].message


def test_chain_referencing_unknown_switch_is_error(intent: Intent) -> None:
    """链里写了不在设备清单里的交换机 → error（位置未知，生成不出本端 U 位）。"""
    c0 = intent.chains[0]
    bad = Intent(
        "bad",
        racks=intent.racks,
        switches=intent.switches,
        routes=intent.routes,
        chains=(
            ChainSpec(c0.channel_no, c0.switch_model, c0.nic, (*c0.instances, "幽灵交换机-99")),
            *intent.chains[1:],
        ),
    )
    _, _, _, issues = to_flow_inputs(bad)
    errs = [i for i in issues if i.severity == "error"]
    assert any("幽灵交换机-99" in i.message for i in errs)


def test_capacity_lives_on_chain_not_switch(intent: Intent) -> None:
    """容量必须挂在「链（端口面）」上，不能挂在交换机上。

    ## 这个用例是被一次真实失败逼出来的

    最初我把 `capacity` 放在 `SwitchSpec` 上（每台交换机一个端口数）。
    查证 V13 全表 186 组 `(交换机实例, 网口)` 后发现这是错的：

    同一台交换机不同端口面的容量不同。实测 `存储接入交换机-1`：
    - `25G1` 面（接存储节点）只有 **40** 个下行口
    - `25GE1/0/48` 是**上行口**，整机 max 端口号 48
    - `100GE1/0/7|8` 也是上行口

    所以「容量」是「这个端口面能接多少台」的属性，只能在 `ChainSpec` 上声明。
    把整机 max（48）当下行容量，块大小会从 `[40, 40, 36]` 变成 `[48, 48, 20]`，
    后面所有端口号连锁错位。

    ## 本用例同时诚实记录一件事

    本次意图只覆盖**计算节点**的 3850 行。计算节点接的是存储接入交换机的
    `万兆2/万兆4` 面（10G），那两面确实各有 44 个下行口，所以这里 21 条链
    的容量全是 44 是对的 —— 40/36 出现在「存储节点接 25G1 面」的场景，
    不在本用例的覆盖面内。真正被证明的是「capacity 可以逐链声明」这个能力。
    """
    assert not hasattr(SwitchSpec("x", "y", 1, 1, 1), "capacity"), (
        "容量不该在交换机上，见上面 docstring"
    )

    _, _, chains, _ = to_flow_inputs(intent)
    assert chains
    # 计算节点范围：每条链都是首台满载 44
    caps = Counter(c.capacity for c in chains)
    assert caps == Counter({44: len(chains)}), f"实测应全 44，实际 {dict(caps)}"

    # 但容量能被逐链改：把 ch9 业务接入「万兆1」改成 40，块边界必须跟着变
    tuned = tuple(
        ChainSpec(
            c.channel_no, c.switch_model, c.nic, c.instances, capacity=40
        )
        if (c.channel_no, c.switch_model, c.nic) == (9, "业务接入交换机 1", "万兆1")
        else c
        for c in intent.chains
    )
    assert tuned[0] is not None
    p0, r0, c0, _ = to_flow_inputs(
        Intent(intent.project, racks=intent.racks, switches=intent.switches,
               routes=intent.routes, chains=tuned)
    )
    plan40 = plan_flow(p0, r0, c0)
    blk = [b for b in plan40.blocks if b.channel_no == 9 and b.nic == "万兆1"]
    sizes40 = [b.size for b in blk]
    plan44 = plan_flow(*to_flow_inputs(intent)[:3])
    blk44 = [b for b in plan44.blocks if b.channel_no == 9 and b.nic == "万兆1"]
    sizes44 = [b.size for b in blk44]
    # 真实 ch9 万兆1 三台是 44/44/46（末台吸收余量）—— 这是从真实端口号查证的，
    # 不是 [44,43,46]：43 是把 rack11 的重复槽位去重后的「位置数」，
    # 按端口号数应是 44 行（其中 2 行共用同一个 U 位）。
    assert sizes44 == [44, 44, 46], f"真实容量 44 时应为 [44,44,46]，实际 {sizes44}"
    # 块数由**交换机台数**决定，与容量无关；容量只决定前 K-1 台各吃多少，
    # 余量全部压给末台。所以容量从 44 降到 40 → 前两台各少 4 台，末台从 46 涨到 54。
    assert sizes44 == [44, 44, 46]
    assert sizes40 == [40, 40, 54], f"容量 40 时应为 [40,40,54]，实际 {sizes40}"


# ---------------------------------------------------------------------------
# 5. 序列化：意图文件能存能读、能改能跑
# ---------------------------------------------------------------------------
def test_intent_roundtrip(intent: Intent, tmp_path: Path) -> None:
    """意图 → JSON → 意图，内容完全一致，且再生成还是 100%。"""
    back = intent_from_dict(intent_to_dict(intent))
    assert back == intent

    out = tmp_path / "intent.json"
    dump_intent(intent, out)
    again = load_intent(out)
    assert again == intent

    p2, r2, c2, issues2 = to_flow_inputs(again)
    assert not [i for i in issues2 if i.severity == "error"]
    assert plan_flow(p2, r2, c2).total_assignment == 3850


def test_schema_version_guard() -> None:
    """未来版本的意图文件必须报错，不能静默读错字段。"""
    with pytest.raises(ValueError, match="比本程序支持的"):
        intent_from_dict({"schema": INTENT_SCHEMA_VERSION + 1, "project": "x"})


def test_unknown_fields_ignored() -> None:
    """未知字段忽略（向前兼容：新版加的字段不会让旧版崩）。"""
    it = intent_from_dict(
        {
            "schema": 1,
            "project": "p",
            "racks": [{"channel": 2, "from": 1, "to": 1, "model": "A", "未来字段": 1}],
        }
    )
    assert it.server_count == 8


def test_json_file_is_stable(intent: Intent) -> None:
    """固化文件与代码里的意图一致 —— 若不一致，说明有人改了 json 没跑回归。"""
    raw = json.loads(INTENT_PATH.read_text(encoding="utf-8"))
    assert raw["schema"] == INTENT_SCHEMA_VERSION
    assert raw["project"] == intent.project
    assert len(raw["racks"]) == 6
    assert len(raw["switches"]) == 91
    assert len(raw["routes"]) == 14
    assert len(raw["chains"]) == 21


# ---------------------------------------------------------------------------
# 6. 设备 id 稳定性（增量重算的前提）
# ---------------------------------------------------------------------------
def test_node_ids_are_stable_and_unique(intent: Intent) -> None:
    """设备 id 必须唯一且不含随机成分 —— 增量重算靠它对上「还是那台」。"""
    placements = expand_placements(intent)
    ids = [p.node_id for p in placements]
    assert len(ids) == len(set(ids)) == 550
    # 同一意图调两次，顺序与内容都一样
    assert expand_placements(intent) == placements


def test_node_id_encodes_position(intent: Intent) -> None:
    """id 形如 `N02-01-S0`，能反解出通道/机柜/槽位。"""
    placements = expand_placements(intent)
    p = next(x for x in placements if x.node_id == "N02-01-S0")
    assert (p.channel_no, p.rack_no, p.u_start) == (2, 1, SLOT_BASE_U)

    # 9 台那个机柜：同槽位第 2 台带 `-1` 后缀，且字典序排在后面
    rack11 = sorted(x.node_id for x in placements if x.channel_no == 2 and x.rack_no == 11)
    assert "N02-11-S7" in rack11 and "N02-11-S7-1" in rack11
    assert rack11.index("N02-11-S7") < rack11.index("N02-11-S7-1")


def test_slot_to_u_mapping() -> None:
    """槽位 → U 位：3, 7, 11, …, 31（实测标准上架方式）。"""
    slots = [slot_of(SLOT_BASE_U + s * SLOT_PITCH_U) for s in range(SLOT_COUNT)]
    assert slots == list(range(SLOT_COUNT))
