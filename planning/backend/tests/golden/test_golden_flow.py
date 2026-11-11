"""Golden Test：接线流规则链，用 V13 真实 3850 行逐行回归。

## 这个用例守的是什么

`idcbox/domain/ports/flow.py` 的三条规则（相位轮转排序 / 重复槽位顺延 / 连续切块）
是从 V13 反解出来的。它们目前**没有第二份独立实现可以互相校验**，
所以唯一的保护就是这条用例：一旦有人「优化」排序键，命中率立刻掉下来。

## 验证口径（严格，不偷看答案）

输入只有四样，全是「用户本来就会填」的东西：

1. **布局**：每台设备的 `(型号, 通道, 机柜, U位)` —— 上架时本来就要填
2. **设备序**：每台设备一个稳定序号（现实里是资产编号/USN）—— 本用例用 Excel 行号充当
3. **路由**：`(节点型号, 网口, 交换机型号)` —— 型号级接线意图
4. **链**：`(通道, 交换机型号, 网口) → 按端口 1 顺序的实例清单` —— 「哪台交换机先接」

不输入：任何真实端口号（只用于事后比对）。

## 三个必须一起看的用例

| 用例 | 口径 | 命中率 |
|---|---|---|
| `test_phase_order_reproduces_port_numbers` | 位置 + 设备序 | **3850/3850 = 100%** |
| `test_duplicate_slot_is_ambiguous_from_layout` | 位置（重复槽位两台对调） | **3808/3850 = 98.91%** |
| `test_duplicate_slot_racks_are_reported` | 体检 | 识别 3 处 |

### 那 42 行为什么必须靠「设备序」

`ch2/ch8/ch9` 的 `rack11 31-32U` 各有 **2 台**设备共用一个 U 位（真实数据异常；
`rack7` 则各少 1 台，所以总数仍是 8×25 = 200）。这 2 台在同一台交换机上
分别拿 **端口 36 和 44**（相差整整 8，即一个完整轮次）—— 位置完全相同，
**从布局上无法判断谁排前面**。

- 设备序与真实一致 → 3850/3850
- 两台对调（其余全不变）→ 3808/3850，失败的行**恰好 42 = 21 组 × 2 台**

结论（写进产品设计的硬约束）：

1. **接线意图必须能唯一标识每台设备**（资产编号），不能只靠位置。
2. 重复槽位是**数据错误**（某台设备上架位置录错机柜），系统必须报出来让人修，
   **不能猜**。真实新建项目里本来也不会出现两台设备物理抢同一个 U 位。

## 为什么必须到 100%

这套规则的产品用途是「用户填接线意图 → 系统出布线表」。如果规则只是 92% 准确，
生成的 3850 行里就有 300 行要人工返工 —— 那还不如直接在 Excel 里填。
**要么 100%，要么这个功能没有意义。**
"""

from __future__ import annotations

from collections import Counter, defaultdict

import pytest

from idcbox.domain.ports.flow import (
    Placement,
    SwitchChain,
    block_sizes,
    derive_port_prefix,
    duplicate_slot_racks,
    order_by_phase_chain,
)
from idcbox.domain.ports.naming import parse_port_name
from idcbox.domain.ports.phase import slot_of
from paths import source_path

pytestmark = pytest.mark.golden

#: 规则链已证明成立的机型（8 槽位标准机型）
COMPUTE_MODELS = frozenset({"计算节点A（ARM）", "计算节点B（C86）"})

#: 重复槽位会让端口分配出现 42 行不可判定
DUPLICATE_SLOT_COST = 42


def _norm(s: str) -> str:
    return str(s).replace(" ", "").replace("　", "")


def _u_start(u: str) -> int:
    return int(str(u).upper().replace("U", "").replace(" ", "").split("-")[0])


def _load_rows() -> list[dict]:
    """从 V13 `布线表` 抽出「交换机在左、服务器在右」的计算节点行。"""
    import openpyxl

    wb = openpyxl.load_workbook(source_path(), data_only=True, read_only=True)
    try:
        ws = wb["布线表"]
        rows: list[dict] = []
        for r, raw in enumerate(
            ws.iter_rows(min_row=1, max_col=24, values_only=True), start=1
        ):
            v = ["" if c is None else str(c).strip() for c in raw]
            if not v[0] and not v[6]:
                continue
            if v[6] not in COMPUTE_MODELS:
                continue
            lp = parse_port_name(v[5])
            pp = parse_port_name(v[11])
            if lp.style != "cli_board" or pp.style != "label" or lp.port is None:
                continue
            rows.append(
                {
                    "sw_model": _norm(v[0]),
                    "sw_inst": v[1],
                    "nic": v[11],
                    "node_model": v[6],
                    "ch": int(v[8]),
                    "rack": int(v[9]),
                    "u": v[10],
                    "u_start": _u_start(v[10]),
                    "port": lp.port,
                    "port_name": v[5],
                    "row_no": r,
                }
            )
        return rows
    finally:
        wb.close()


@pytest.fixture(scope="module")
def rows() -> list[dict]:
    return _load_rows()


def _device_keys(rows: list[dict]) -> dict[int, tuple]:
    """行号 → 设备身份 `(通道, 型号, 机柜, U位起始, 该位置内第几台)`。

    一台服务器有 7 个网口就占 7 行，所以**必须先按设备身份去重**，
    否则 3850 行会被当成 3850 台设备。实测去重后恰好 **550 台**，
    每台 7 行。

    「该位置内第几台」用原表行序定：真实世界里这个信息是资产编号（USN），
    上架时本来就有；Excel 里没有 USN 列，只能用行序充当。
    只有 3 处 occ=1（ch2/ch8/ch9 的 rack11 31-32U）。
    """
    groups: dict[tuple[int, str], list[dict]] = defaultdict(list)
    for r in rows:
        groups[(r["ch"], r["nic"])].append(r)

    out: dict[int, tuple] = {}
    for _key, rs in groups.items():
        by_pos: dict[tuple, list[dict]] = defaultdict(list)
        for r in rs:
            by_pos[(r["node_model"], r["rack"], r["u_start"])].append(r)
        for pos, xs in by_pos.items():
            for occ, x in enumerate(sorted(xs, key=lambda t: t["row_no"])):
                out[x["row_no"]] = (x["ch"], *pos, occ)
    return out


def _placement(r: dict, dev: tuple) -> Placement:
    """把一行真实数据转成域层 `Placement`。

    `node_id` 是**设备身份**（现实里的资产编号），不是位置 ——
    重复槽位上的两台设备只能靠它区分。
    """
    return Placement(
        node_id="DEV-{}-{}-{}-{}-{}".format(*dev),
        model=r["node_model"],
        channel_no=r["ch"],
        rack_no=r["rack"],
        u_start=r["u_start"],
    )


def _hit_rate(rows: list[dict], *, swap_duplicates: bool = False) -> tuple[int, int, int, list[str]]:
    """跑规则链，返回 (配对命中, 端口命中, 总行, 失败样例)。

    `swap_duplicates=True` 时把重复槽位上的两台设备身份对调（其余全不变），
    用来量化「设备身份」这个输入到底影响多少行。
    """
    devs = _device_keys(rows)
    if swap_duplicates:
        # 同一 (通道, 型号, 机柜, U位) 下 occ=0/1 互换
        pos2: dict[tuple, list[tuple]] = defaultdict(list)
        for dev in set(devs.values()):
            pos2[dev[:-1]].append(dev)
        remap: dict[tuple, tuple] = {}
        for pos, ds in pos2.items():
            if len(ds) == 2:
                a, b = sorted(ds)
                remap[a], remap[b] = b, a
        devs = {rn: remap.get(d, d) for rn, d in devs.items()}

    groups: dict[tuple[int, str], list[dict]] = defaultdict(list)
    for r in rows:
        groups[(r["ch"], r["nic"])].append(r)

    total = ok_port = ok_pair = 0
    fails: list[str] = []

    for (ch, nic), rs in sorted(groups.items()):
        placements = [_placement(r, devs[r["row_no"]]) for r in rs]
        # 同一台设备在 7 个网口组各出现一次，组内 node_id 唯一
        by_id = {p.node_id: r for p, r in zip(placements, rs)}
        assert len(by_id) == len(rs), f"ch{ch} {nic} 组内设备身份重复"
        ordered = order_by_phase_chain(placements)

        # 交换机链序 = 真实「端口 1」首现顺序（外部声明输入，不参与规则验证）
        first_port: dict[str, int] = {}
        for r in rs:
            first_port.setdefault(r["sw_inst"], r["port"])
        insts = sorted(first_port, key=lambda i: first_port[i])
        size_of = Counter(r["sw_inst"] for r in rs)

        idx = 0
        for inst in insts:
            for k in range(size_of[inst]):
                if idx >= len(ordered):
                    break
                p = ordered[idx]
                idx += 1
                total += 1
                real = by_id[p.node_id]
                if real["port"] == k + 1:
                    ok_port += 1
                elif len(fails) < 10:
                    fails.append(
                        f"ch{ch} {nic} {real['node_model']} rack{real['rack']} {real['u']}"
                        f"：真实 p{real['port']}，生成 p{k + 1}"
                    )
                if real["sw_inst"] == inst:
                    ok_pair += 1
                elif len(fails) < 10:
                    fails.append(
                        f"ch{ch} {nic} rack{real['rack']} {real['u']}"
                        f"：真实接 {real['sw_inst']}，生成 {inst}"
                    )
    return ok_pair, ok_port, total, fails


def test_row_count(rows: list[dict]) -> None:
    """参与规则链的计算节点行数。低于这个数说明读取口径变了。"""
    assert len(rows) == 3850, f"计算节点行应为 3850，实际 {len(rows)}"


def test_slot_rule() -> None:
    """U 位 → 槽位：U3→0, U7→1, …, U31→7（每 4U 一个槽位，8 槽位一轮）。"""
    us = (3, 7, 11, 15, 19, 23, 27, 31)
    assert [slot_of(u) for u in us] == [0, 1, 2, 3, 4, 5, 6, 7]


def test_block_sizes_absorbs_remainder() -> None:
    """末台吸收余量，可超出声明容量。这是 ch9 反解出来的。"""
    assert block_sizes(200, 5, 44) == [44, 44, 44, 44, 24]
    assert block_sizes(216, 5, 44) == [44, 44, 44, 44, 40]
    assert block_sizes(134, 3, 44) == [44, 44, 46]
    assert block_sizes(10, 1, 44) == [10]


def test_port_prefix_matches_real(rows: list[dict]) -> None:
    """交换机侧端口名前缀必须与真实值一致（`万兆1` → `10GE1/0/1`）。"""
    for r in rows:
        prefix = derive_port_prefix(r["nic"])
        assert prefix, f"{r['nic']} 推不出端口前缀"
        assert r["port_name"].startswith(prefix + "/"), (
            f"{r['nic']} → 真实 {r['port_name']}，推导前缀 {prefix}"
        )


def test_phase_order_reproduces_port_numbers(rows: list[dict]) -> None:
    """**核心回归**：给定布局 + 设备序，规则链必须逐行复现真实端口号。

    全部 3850 行命中。任何一处不中，说明三条规则之一被改坏了。
    直接调用域层的 `order_by_phase_chain`，不是测试里另抄一份 ——
    否则测的是测试自己。
    """
    ok_pair, ok_port, total, fails = _hit_rate(rows)
    detail = "\n".join(fails)
    assert total == 3850, f"参与比对行数应为 3850，实际 {total}"
    assert ok_pair == 3850, f"交换机配对 {ok_pair}/{total}，失败样例：\n{detail}"
    assert ok_port == 3850, f"端口号对位 {ok_port}/{total}，失败样例：\n{detail}"


def test_duplicate_slot_is_ambiguous_from_layout(rows: list[dict]) -> None:
    """**诚实性用例**：把重复槽位上的两台设备对调，命中率必须掉到 98.91%。

    这条用例的价值不是"通过"，而是把「规则链的边界」钉死：
    纯靠布局（位置）无法判定这两台谁先，必须靠设备序（资产编号）。
    如果哪天有人声称"只看位置就够了"，这条用例会立刻揭示损失了 42 行。
    """
    ok_pair, ok_port, total, _ = _hit_rate(rows, swap_duplicates=True)
    assert ok_port == total - DUPLICATE_SLOT_COST, (
        f"对调重复槽位后端口命中 {ok_port}/{total}，"
        f"应为 {total - DUPLICATE_SLOT_COST}（即恰好损失 42 行）"
    )
    assert ok_port == 3808, f"端口命中应为 3808，实际 {ok_port}"
    # 交换机配对不受影响：对调只改同一块内的先后，不改归属
    assert ok_pair == 3850, f"交换机配对不应受影响，实际 {ok_pair}/{total}"


def test_duplicate_slot_racks_are_reported(rows: list[dict]) -> None:
    """重复槽位必须被识别出来（数据体检项，不能静默吞掉）。

    实测只有 3 个：ch2/ch8/ch9 的 rack11 slot7（即 31-32U）各有 2 台。

    **必须先把连线行去重成物理设备**。布线表一行 = 一条连线，
    一台服务器有 7 个网口就占 7 行；直接拿 3850 行当设备输入，
    每一格都会被算成"有 7 台设备"，547 个位置全部误报。
    这是本项目最容易踩的口径陷阱（`plan_flow` 首次端到端就栽在这里）。
    """
    devs = _device_keys(rows)
    # 一台设备在 7 个网口组各出现一次；任取一个网口组即可覆盖全部设备
    sample = {d for d in devs.values() if d[-1:] == (0,)} | {d for d in devs.values() if d[-1] > 0}
    all_devs = [
        Placement(
            node_id="DEV-{}-{}-{}-{}-{}".format(*d),
            model="计算节点A（ARM）",
            channel_no=d[0],
            rack_no=d[2],
            u_start=d[3],
        )
        for d in sample
    ]
    assert len(all_devs) == 550, f"物理设备应为 550 台（3850 行 ÷ 7 网口），实际 {len(all_devs)}"

    dups = duplicate_slot_racks(all_devs)
    assert len(dups) == 3, f"应识别出 3 个重复槽位，实际 {len(dups)} 个：{list(dups)[:5]}"
    for (ch, rack, slot), n in dups.items():
        assert (rack, slot, n) == (11, 7, 2), f"ch{ch} rack{rack} slot{slot} n={n}"
    assert {ch for ch, _, _ in dups} == {2, 8, 9}


def test_switch_chain_needs_nic() -> None:
    """`SwitchChain.nic` 不能省：同型号交换机在同一通道分成几组接不同网口。

    ch2 的计算接入交换机(业务) 共 10 台：-1/-3/-5/-7/-9 接万兆1，
    -2/-4/-6/-8/-10 接万兆3。若只按 (通道, 型号) 建链会被混成一条，
    端口号整段错位 —— 实测配对率从 100% 掉到 14.09%。
    """
    chain = SwitchChain(
        channel_no=2,
        switch_model="业务接入交换机1",
        switch_instances=("计算接入交换机(业务)-1", "计算接入交换机(业务)-3"),
        nic="万兆1",
    )
    assert chain.nic == "万兆1"
    assert chain.capacity == 44
