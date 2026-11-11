"""端口分配五条规则的 golden 回归（C1 映射 + C2 位次 + C3 相位 + C4 φ + C5 单调）。

## 为什么端口分配比线长更值得信任

线长有 8.2% 的人工填错率（N/O 52 行），但端口分配是 **3850/3850 = 100%**：
同一台交换机、同一批节点，端口号严格等于组内位次加 base，零例外。
这意味着「用户声明哪些节点归哪台交换机」→「系统直接出 3850 行端口」是可行的，
不是拟合出来的。

## 两个口径必须同时断言（否则会过度承诺）

| 口径 | 块数 | C3 相位 | C5 单调 |
|---|---|---|---|
| 计算节点（8 槽位标准机型） | 112 | **112/112 = 100%** | 112/112 = 100% |
| 全部节点（含网络功能服务器等） | 174 | 112/164 = 68.3% | 140/174 = 80.5% |

**全部节点口径做不到 100%** 的原因：`业务核心交换机→网络功能服务器` 存在
「一个 U 位塞两台设备」（slots `[1,1,1,1,2,2,2,2]`、us `[7,7,7,7,11,11,11,11]`），
`带外交换机→备份一体机` 的 slots 全 0 —— 这些机型根本不遵守 8 槽位规律。

产品含义：**相位校验必须按节点型号开关**，非计算节点只告警不阻断。
"""

from __future__ import annotations

import re
from collections import Counter, defaultdict

import pytest

from baseline_io import load_baseline
from idcbox.domain.ports.naming import parse_port_name
from idcbox.domain.ports.phase import NodeRef, phase_of, slot_of

pytestmark = pytest.mark.golden

#: 计算节点机型标识（8 槽位标准机型的判据）
COMPUTE_NODE_MARK = "计算节点"


def _extract_records(rows, col) -> list[dict]:
    """抽取「交换机 → 服务器」方向的行。

    判据：本端端口是 CLI 板卡式（`25GE1/0/1`），对端端口是标签式（`万兆1`/`IPMI`）。
    反向的行（服务器→交换机）端口序列在对端，不用于本回归。
    """
    recs: list[dict] = []
    for v in rows:
        if not v[col["model_local"]] and not v[col["model_peer"]]:
            continue
        local_p = parse_port_name(str(v[col["port_local"]] or ""))
        peer_p = parse_port_name(str(v[col["port_peer"]] or ""))
        if local_p.style != "cli_board" or peer_p.style != "label":
            continue
        if local_p.port is None:
            continue
        try:
            peer_ch = int(float(v[col["ch_peer"]]))
            peer_rack = int(float(v[col["rack_peer"]]))
        except (TypeError, ValueError):
            continue
        m = re.match(r"^(\d+)", str(v[col["u_peer"]] or ""))
        if not m:
            continue
        recs.append(
            {
                "switch_model": str(v[col["model_local"]]).strip(),
                "switch_instance": str(v[col["dev_local"]]).strip(),
                "switch_port": str(v[col["port_local"]]).strip(),
                "switch_port_no": local_p.port,
                "node_model": str(v[col["model_peer"]]).strip(),
                "node_id": str(v[col["dev_peer"]]).strip(),
                "node_ch": peer_ch,
                "node_rack": peer_rack,
                "node_u": int(m.group(1)),
            }
        )
    return recs


def _split_blocks(recs: list[dict]) -> tuple[list[list[dict]], dict[tuple[str, str, str], list[dict]]]:
    """按 `(交换机型号, 实例名, 节点型号)` 分组，再按端口号连续性切块。

    切块的物理含义：一台 48 口交换机可能服务两类节点，第 1 类占 1..8、
    第 2 类从 9 开始 —— 这就是 base 分布 `{1: 91, 9: 7, 17: 7, 20: 7}` 的来由。
    """
    groups: dict[tuple[str, str, str], list[dict]] = defaultdict(list)
    for d in recs:
        groups[(d["switch_model"], d["switch_instance"], d["node_model"])].append(d)

    blocks: list[list[dict]] = []
    for rows_g in groups.values():
        ordered = sorted(rows_g, key=lambda d: d["switch_port_no"])
        cur = [ordered[0]]
        for prev, nxt in zip(ordered, ordered[1:]):
            if nxt["switch_port_no"] - prev["switch_port_no"] == 1:
                cur.append(nxt)
            else:
                blocks.append(cur)
                cur = [nxt]
        blocks.append(cur)
    return blocks, groups


def _nodes_of(block: list[dict]) -> list[NodeRef]:
    return [
        NodeRef(
            node_id=d["node_id"],
            rack_channel=d["node_ch"],
            rack_seq=d["node_rack"],
            u_start=d["node_u"],
        )
        for d in sorted(block, key=lambda d: d["switch_port_no"])
    ]


class TestC2RankPlusBase:
    """C2：交换机端口号 = 组内位次 + base。这是 100% 的那一条。"""

    def test_c2_all_blocks(self, rows, col):
        base_b = load_baseline("port_baseline")
        recs = _extract_records(rows, col)
        assert len(recs) == base_b["link_rows"], f"交换机→服务器行数变了：{len(recs)}"

        blocks, groups = _split_blocks(recs)
        assert len(groups) == base_b["groups"]
        assert len(blocks) == base_b["blocks"]

        ok = total = 0
        bad: list[dict] = []
        for b in blocks:
            ordered = sorted(b, key=lambda d: d["switch_port_no"])
            base_port = ordered[0]["switch_port_no"]
            for i, d in enumerate(ordered):
                total += 1
                if d["switch_port_no"] == base_port + i:
                    ok += 1
                else:
                    bad.append({"port": d["switch_port"], "expected": base_port + i})

        assert total == base_b["c2_rank_plus_base"]["total"]
        assert ok == base_b["c2_rank_plus_base"]["ok"], (
            f"C2 命中数变了：{ok}/{total}，偏差 {bad[:10]}"
        )
        assert ok == total, "C2 必须 100%，任何例外都说明分组或排序口径错了"

    def test_base_distribution(self, rows, col):
        """base 分布是「一台交换机拆多块」的实证：91 块从 1 起，7 块从 9/17/20 起。"""
        base_b = load_baseline("port_baseline")
        recs = _extract_records(rows, col)
        blocks, _ = _split_blocks(recs)
        dist = Counter(sorted(b, key=lambda d: d["switch_port_no"])[0]["switch_port_no"] for b in blocks)
        # JSON 对象的键一定是字符串，转回 int 再比
        assert dict(sorted(dist.items())) == {int(k): v for k, v in base_b["base_port_dist"].items()}

        # 计算节点口径的 base 分布必须恰好是 {1:91, 9:7, 17:7, 20:7}
        compute_blocks = [b for b in blocks if COMPUTE_NODE_MARK in b[0]["node_model"]]
        compute_dist = Counter(
            sorted(b, key=lambda d: d["switch_port_no"])[0]["switch_port_no"]
            for b in compute_blocks
        )
        assert compute_dist == {1: 91, 9: 7, 17: 7, 20: 7}, (
            f"计算节点 base 分布变了：{dict(compute_dist)}"
        )


class TestC3Phase:
    """C3/C4：8 槽位相位 `slot_i ≡ (i + φ) mod 8`，且 φ = 首节点 slot。"""

    def test_slot_of(self):
        """槽位映射：U 位 3/7/11/.../31 → slot 0..7（每 4U 一个 2U 槽位）。"""
        assert [slot_of(u) for u in (3, 7, 11, 15, 19, 23, 27, 31)] == list(range(8))
        # 2U 设备占 u,u+1，起点都落在 4 的倍数+3
        assert slot_of(3) == 0
        assert slot_of(5) == 0  # 同一槽位的第二 U

    def test_c3_compute_nodes_100pct(self, rows, col):
        """计算节点口径：112/112 组 φ 唯一 = 100%。"""
        base_b = load_baseline("port_baseline")["scope_compute_nodes"]
        recs = _extract_records(rows, col)
        blocks, _ = _split_blocks(recs)
        compute_blocks = [b for b in blocks if COMPUTE_NODE_MARK in b[0]["node_model"]]
        assert len(compute_blocks) == base_b["blocks"]

        ok = total = 0
        phi_dist: Counter[int] = Counter()
        bad: list[dict] = []
        for b in compute_blocks:
            nodes = _nodes_of(b)
            if len(nodes) < 3:
                continue
            total += 1
            phi = phase_of(nodes)
            if phi is None:
                bad.append({"switch": b[0]["switch_instance"], "slots": [n.slot for n in nodes[:12]]})
            else:
                ok += 1
                phi_dist[phi] += 1

        assert total == base_b["c3_phase_unique"]["total"]
        assert ok == base_b["c3_phase_unique"]["ok"], (
            f"计算节点 C3 不再是 100%：{ok}/{total}，坏块 {bad[:5]}"
        )
        assert ok == total, "计算节点相位必须 100% 唯一"
        assert {str(k): v for k, v in sorted(phi_dist.items())} == base_b["c3_phase_unique"]["phi_dist"]

    def test_c4_phi_equals_first_slot(self, rows, col):
        """C4：φ 不是独立参数，等于首节点 slot —— 所以 φ 不该落库，实时算。"""
        base_b = load_baseline("port_baseline")["scope_compute_nodes"]
        recs = _extract_records(rows, col)
        blocks, _ = _split_blocks(recs)
        compute_blocks = [b for b in blocks if COMPUTE_NODE_MARK in b[0]["node_model"]]

        ok = total = 0
        for b in compute_blocks:
            nodes = _nodes_of(b)
            if len(nodes) < 3:
                continue
            total += 1
            phi = phase_of(nodes)
            if phi is not None and phi == slot_of(nodes[0].u_start):
                ok += 1
        assert total == base_b["c4_phi_equals_first_slot"]["total"]
        assert ok == base_b["c4_phi_equals_first_slot"]["ok"]
        assert ok == total, "φ 必须恒等于首节点 slot"

    def test_c3_all_nodes_confesses(self, rows, col):
        """全部节点口径只有 68.3% —— 这条断言是「不许对外说 100%」的守门员。"""
        base_b = load_baseline("port_baseline")["scope_all_nodes"]
        recs = _extract_records(rows, col)
        blocks, _ = _split_blocks(recs)

        ok = total = 0
        for b in blocks:
            nodes = _nodes_of(b)
            if len(nodes) < 3:
                continue
            total += 1
            if phase_of(nodes) is not None:
                ok += 1
        assert total == base_b["c3_phase_unique"]["total"]
        assert ok == base_b["c3_phase_unique"]["ok"], f"全部节点 C3 变了：{ok}/{total}"
        assert ok < total, (
            "全部节点相位若变成 100%，说明数据或口径变了，需重新评估「按型号开关」的设计"
        )


class TestC5RackMonotonic:
    """C5：组内机柜号严格单调不减（生成时靠 sort_rule 保证，校验时断言）。"""

    def test_c5_compute_nodes(self, rows, col):
        base_b = load_baseline("port_baseline")["scope_compute_nodes"]["c5_rack_monotonic"]
        recs = _extract_records(rows, col)
        blocks, _ = _split_blocks(recs)
        compute_blocks = [b for b in blocks if COMPUTE_NODE_MARK in b[0]["node_model"]]

        ok = 0
        for b in compute_blocks:
            racks = [d["node_rack"] for d in sorted(b, key=lambda x: x["switch_port_no"])]
            if all(racks[i] <= racks[i + 1] for i in range(len(racks) - 1)):
                ok += 1
        assert len(compute_blocks) == base_b["total"]
        assert ok == base_b["ok"]
        assert ok == base_b["total"], "计算节点组内机柜必须单调不减"

    def test_c5_all_nodes(self, rows, col):
        base_b = load_baseline("port_baseline")["scope_all_nodes"]["c5_rack_monotonic"]
        recs = _extract_records(rows, col)
        blocks, _ = _split_blocks(recs)
        ok = 0
        for b in blocks:
            racks = [d["node_rack"] for d in sorted(b, key=lambda x: x["switch_port_no"])]
            if all(racks[i] <= racks[i + 1] for i in range(len(racks) - 1)):
                ok += 1
        assert len(blocks) == base_b["total"]
        assert ok == base_b["ok"]


class TestC1PortNameMapping:
    """C1 的前置条件：端口命名两族可解析，且 CLI 式能拿到速率与端口号。"""

    def test_cli_board_ports(self, rows, col):
        seen_rates: Counter[int | None] = Counter()
        for v in rows:
            p = parse_port_name(str(v[col["port_local"]] or ""))
            if p.style == "cli_board":
                seen_rates[p.rate_mbps] += 1
        # 交换机侧只出现这四种速率
        assert set(seen_rates) <= {1000, 10000, 25000, 100000}, (
            f"出现意外速率：{set(seen_rates)}"
        )
        assert seen_rates[100000] > 0 and seen_rates[25000] > 0

    def test_label_ports(self, rows, col):
        """服务器侧是标签式：千兆1/万兆2/25G2/IPMI，必须都能解出端口序号。"""
        kinds: Counter[str] = Counter()
        for v in rows:
            p = parse_port_name(str(v[col["port_peer"]] or ""))
            if p.style == "label":
                kinds[p.label or "IPMI"] += 1
        assert {"千兆", "万兆", "25G"} <= set(kinds), f"标签族缺失：{set(kinds)}"
        # IPMI 走 is_mgmt 分支，不计入 label
        assert kinds["千兆"] > 0 and kinds["万兆"] > 0

    def test_unknown_style_tolerated(self):
        """`Eth1/2/1`（政务外网连线表的写法）必须归 unknown 而不是抛异常。"""
        p = parse_port_name("Eth1/2/1")
        assert p.style == "unknown"
        assert p.is_known is False
