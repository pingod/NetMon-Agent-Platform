"""生成器黄金用例：意图 → 布线表 24 列，与真实 V13 逐格比对。

## 这组用例证明的是「用户不用再手填 5000 行」

前两组用例分别证明：
- `test_golden_flow.py`：三条排布规则能复现端口号
- `test_golden_intent.py` / `test_golden_intent_full.py`：意图能表达出那些规则所需的信息

本文件证明**最终交付物**：`generate_links()` + `bridge.to_link_inputs()` +
长度引擎串起来之后，导出的 13 列与真实布线表**逐格一致**。

## 一个诚实的偏差：`对端设备名称` 列

实测 4430 行里 200 行的「对端设备名称」与真实表不同 —— 生成器留空，真实表
填了 `计算节点A（ARM）-1` 这类名字。

**生成器不填是对的**，理由有三：
1. 该列在源表里是**半填**状态：4562 行里只有 332 行有值，其余为空；
2. 源表这一列**自相矛盾**：`计算接入交换机(业务)-1` 的 port9 型号写
   `计算节点A（ARM）`，名称却是 `计算节点B（C86）-1` —— 型号和名字对不上；
3. 设备名是**上架命名规范**的产物（`{型号}-{序号}`），不属于接线意图。
   机器猜一个名字写进交付物，比留空更糟 —— 施工队会照着错名字贴标签。

所以这条偏差被明确断言为「仅限这一列、仅限这 200 行」，见
`test_only_peer_device_name_differs`。若将来有人改了生成逻辑导致别的列或
别的行出现差异，这条用例会立刻红。
"""

from __future__ import annotations

import json
from collections import Counter
from pathlib import Path

import pytest

from idcbox.domain.cable_length.engine import compute_batch
from idcbox.domain.cable_length.rules import DEFAULT_RULE
from idcbox.domain.geometry.floor import FloorCell, derive_cmax_by_room, parse_floor
from idcbox.domain.geometry.layout import build_snapshot
from idcbox.domain.ports.bridge import to_link_inputs
from idcbox.domain.ports.generate import generate_links
from idcbox.domain.ports.intent import intent_from_dict
from idcbox.domain.ports.naming import parse_port_name
from paths import source_path

#: 意图文件放在 `projects/<项目>/intent.json`，与产线共用同一份 ——
#: 测试读的就是用户会改的那个文件，避免"fixture 与真货漂移"。
INTENT_FULL_PATH = Path(__file__).resolve().parents[3] / "projects" / "v13" / "intent.json"

#: 导出的 13 列（与 `CONTRACT_25` 前 13 项同序）。
CABLING_COLS = (
    "local_model",
    "local_device",
    "local_channel",
    "local_rack",
    "local_u",
    "local_port",
    "peer_model",
    "peer_device",
    "peer_channel",
    "peer_rack",
    "peer_u",
    "peer_port",
    "cable_type",
)

#: 型号列在真实表里有空格变体（`业务接入交换机1` / `业务接入交换机 1`），
#: 比较时必须归一，否则会误判成 1201 处差异。
_MODEL_COLS = frozenset({"local_model", "peer_model"})

#: `peer_device`（对端设备名称）**不入逐格比对**，理由见本文件顶部 docstring：
#: 源表这一列是半填状态（4562 行里仅 332 行有值）且存在型号与名字矛盾的记录，
#: 设备名属上架命名规范而非接线意图。该列的偏差由下一条用例单独钉量（恰好 200 行）。
_COMPARED_COLS = tuple(c for c in CABLING_COLS if c != "peer_device")


def _nm(v: object) -> str:
    return str(v).strip() if v is not None else ""


def _norm_model(s: str) -> str:
    return s.replace(" ", "").replace("　", "")


@pytest.fixture(scope="module")
def intent_full():
    return intent_from_dict(json.loads(INTENT_FULL_PATH.read_text(encoding="utf-8")))


@pytest.fixture(scope="module")
def generated(intent_full):
    return generate_links(intent_full)


@pytest.fixture(scope="module")
def geometry():
    """真实平面图几何（线长的 N/O 依赖它）。"""
    import openpyxl

    ws = openpyxl.load_workbook(source_path(), data_only=True)["机房平面图"]
    cells: list[FloorCell] = []
    for r in range(1, ws.max_row + 1):
        for c in range(1, ws.max_column + 1):
            val = ws.cell(row=r, column=c).value
            if val is None or not str(val).strip():
                continue
            cells.append(FloorCell(r, c, val))
    floor = parse_floor(cells)
    cmax = derive_cmax_by_room(floor.racks)
    return build_snapshot(floor.racks, version="golden", cmax_override=cmax)


@pytest.fixture(scope="module")
def real_rows() -> list[dict]:
    """真实布线表里「交换机在左、服务器在右」的 4562 行。"""
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
                "local_model": v[0],
                "local_device": v[1],
                "local_channel": v[2],
                "local_rack": v[3],
                "local_u": v[4],
                "local_port": v[5],
                "peer_model": v[6],
                "peer_device": v[7],
                "peer_channel": v[8],
                "peer_rack": v[9],
                "peer_u": v[10],
                "peer_port": v[11],
                "cable_type": v[12],
                #: 比对键：交换机实例 + 端口 + 网口（服务器行的名称常为空，
                #: 放进键会让 200 行假性未命中）
                "_key": (v[1], v[5], v[11]),
            }
        )
    return out


# ---------------------------------------------------------------------------
# 1. 生成量与结构
# ---------------------------------------------------------------------------
def test_generate_row_count(generated) -> None:
    """207 行意图 → 4430 条连线。"""
    assert len(generated.links) == 4430


def test_generate_by_nic(generated) -> None:
    """各网口条数与真实分布一致。

    计算节点 550 台 × 7 口 = 3850；存储节点 116 台（25G×2 + 千兆×2 + IPMI）= 580。
    """
    by_nic = dict(generated.by_nic)
    assert by_nic["万兆1"] == 550
    assert by_nic["万兆2"] == 550
    assert by_nic["万兆3"] == 550
    assert by_nic["万兆4"] == 550
    assert by_nic["千兆1"] == 666
    assert by_nic["千兆3"] == 550
    assert by_nic["IPMI"] == 666
    assert by_nic["25G1"] == 116
    assert by_nic["25G2"] == 116
    assert sum(by_nic.values()) == 4430


def test_generate_has_no_errors(generated) -> None:
    """意图自洽，不应有 error（warn 允许：3 处重复槽位会报）。"""
    assert generated.error_count == 0
    assert generated.warn_count == 21


def test_generate_reports_scope_boundary(generated) -> None:
    """必须在输出里说明「交换机之间的互联不在自动生成范围」。

    这是产品诚实性要求：用户看到 4430 行不能以为 5114 行全齐了。
    实测真实表 5114 行里有 552 行是交换机↔交换机（约 11%）。
    """
    joined = " ".join(generated.notes)
    assert "交换机之间" in joined
    assert "显式连线" in joined


# ---------------------------------------------------------------------------
# 2. 核心：与真实布线表逐格一致
# ---------------------------------------------------------------------------
def test_every_generated_row_exists_in_real(generated, real_rows) -> None:
    """4430 条生成行必须全部能在真实表里找到（按交换机+端口+网口）。"""
    truth = {r["_key"]: r for r in real_rows}
    gen_keys = [(gen.a_device, gen.a_port, gen.nic) for gen in generated.links]
    not_found = [k for k in gen_keys if k not in truth]
    assert not not_found, f"{len(not_found)} 条生成行在真实表中不存在，例如 {not_found[:3]}"


def test_thirteen_columns_match_exactly(generated, real_rows) -> None:
    """13 列逐格一致（`peer_device` 除外，见下一个用例）。

    这是 v0.3 的核心验收：不是「大致对」，是**逐格相同**。
    """
    truth = {r["_key"]: r for r in real_rows}
    diffs: Counter[str] = Counter()
    samples: list[str] = []
    for gen in generated.links:
        key = (gen.a_device, gen.a_port, gen.nic)
        real = truth.get(key)
        assert real is not None, f"生成行 {key} 在真实表中不存在"
        got = {
            "local_model": gen.a_model,
            "local_device": gen.a_device,
            "local_channel": str(gen.a_channel),
            "local_rack": str(gen.a_rack),
            "local_u": gen.a_u,
            "local_port": gen.a_port,
            "peer_model": gen.b_model,
            "peer_device": gen.b_device,
            "peer_channel": str(gen.b_channel),
            "peer_rack": str(gen.b_rack),
            "peer_u": gen.b_u,
            "peer_port": gen.b_port,
            "cable_type": gen.cable_type,
        }
        for col in _COMPARED_COLS:
            g, r = got[col], real[col]
            if col in _MODEL_COLS:
                g, r = _norm_model(g), _norm_model(r)
            if g != r:
                diffs[col] += 1
                if len(samples) < 5:
                    samples.append(f"{key} {col}: 生成 {g!r} 真实 {r!r}")

    assert not diffs, "生成结果与真实表有差异：\n" + "\n".join(samples)
    assert not samples


def test_only_peer_device_name_differs(generated, real_rows) -> None:
    """唯一允许的偏差：`peer_device`（对端设备名称）恰好 200 行。

    为什么允许：源表这一列只填了 332 行（半填），且存在型号与名字矛盾的记录
    （`计算接入交换机(业务)-1` port9 型号 = 计算节点A（ARM），名称却 =
    计算节点B（C86）-1）。设备名属上架命名规范，不是接线意图，机器不该猜。

    这条断言把偏差**钉死在数量和范围上**：任何新增偏差都会红。
    """
    truth = {r["_key"]: r for r in real_rows}
    named = 0
    other: list[str] = []
    for gen in generated.links:
        real = truth[(gen.a_device, gen.a_port, gen.nic)]
        if gen.b_device != real["peer_device"]:
            if gen.b_device == "":
                named += 1
            else:
                other.append(f"{gen.a_device}:{gen.a_port} 生成名 {gen.b_device!r}")
    assert not other, f"生成器不该自己编名字：{other[:3]}"
    assert named == 200


def test_peer_device_left_empty_is_intentional(generated) -> None:
    """生成器永远不填 `peer_device` —— 这是设计决定，不是遗漏。"""
    assert all(gen.b_device == "" for gen in generated.links)


# ---------------------------------------------------------------------------
# 3. 桥接与长度计算
# ---------------------------------------------------------------------------
def test_bridge_produces_unique_keys(generated) -> None:
    """4430 条链接 → 4430 个唯一 stable_key（无重复声明）。"""
    links, notes = to_link_inputs(generated)
    assert len(links) == 4430
    assert len({li.stable_key for li in links}) == 4430
    assert not notes, "不该有重复端点"


def test_bridge_keys_never_use_empty_placeholder(generated) -> None:
    """`stable_key` 里不许出现 `∅` —— 那意味着有端点身份是空的。

    `make_stable_key` 把空值写成 `∅`，本意是区分 `a||b` 与 `a|∅|b`。
    但生成行的对端如果留空（`b_device` 确实为空，那是显示名），
    键就会变成 `交换机|端口|∅|网口|类型`，「按设备查线缆」和跨版本
    比对都会失效。桥接层必须回落到 `b_id`（槽位 id）。
    """
    links, _ = to_link_inputs(generated)
    bad = [li.stable_key for li in links if "∅" in li.stable_key]
    assert not bad, f"{len(bad)} 条键里有空占位，例如 {bad[:2]}"


def test_bridge_keeps_endpoint_roles(generated) -> None:
    """交换机固定在 `a` 侧、服务器固定在 `b` 侧（导出列序依赖这个约定）。"""
    links, _ = to_link_inputs(generated)
    assert all(li.a.device_name.startswith(("计算接入", "存储接入", "计算带内", "计算带外",
                                            "存储带内", "存储带外")) for li in links)
    # b 侧用槽位 id（`N02-11-S7`）而不是设备名：设备名在真实数据里普遍为空，
    # 用它做身份会让 stable_key 里的对端退化成 `∅`。
    assert all(li.b.device_name.startswith("N") and "-S" in li.b.device_name for li in links)


def test_lengths_are_computed_for_all_rows(generated, geometry) -> None:
    """4430 行全部算出几何长度（不依赖任何人工填值）。"""
    links, _ = to_link_inputs(generated)
    batch = compute_batch(links, DEFAULT_RULE, geometry)
    assert batch.stats.total_rows == 4430
    assert batch.stats.computed_rows == 4430
    assert batch.stats.error_rows == 0


def test_length_distribution_is_plausible(generated, geometry) -> None:
    """长度分布必须落在真实阶梯的档位上（5/7/10/15/20/25/30/35/40）。"""
    links, _ = to_link_inputs(generated)
    batch = compute_batch(links, DEFAULT_RULE, geometry)
    ladder = Counter(r.t_ladder_m for r in batch.results)
    assert set(ladder) <= {5, 7, 10, 15, 20, 25, 30, 35, 40}
    # 机内（RJ45/LC）绝大多数 ≤ 10m
    assert ladder[5] + ladder[7] + ladder[10] > 3000


def test_u_approved_stays_empty_in_generation(generated, geometry) -> None:
    """U 列（线缆长度）生成时**必须留空** —— 那是人核定的值。

    与 `export` 同一条铁律：机器可以提供建议（T 列），但不能替人核定（U 列）。
    若这里变成有值，就说明有人把 T 直接抄进了 U，交付物里会出现
    「机器核定的长度」，而那本该是施工负责人的签字。
    """
    links, _ = to_link_inputs(generated)
    batch = compute_batch(links, DEFAULT_RULE, geometry)
    assert all(r.u_approved_m is None for r in batch.results)
    # T 列算出值了，说明是「有建议、待核定」
    assert sum(1 for r in batch.results if r.t_ladder_m is not None) == 4430
