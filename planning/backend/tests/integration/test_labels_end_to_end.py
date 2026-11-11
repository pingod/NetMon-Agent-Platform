"""标签清单端到端：真实 V13 数据 → API/CLI。

## 为什么要跑真实数据

标签的三段拼接本身有单测守着，但「10254 张标签、4230 张缺编码、
机柜码从通道+柜号生成」这些**规模与分布**只有跑全量才看得见。
做法是把它们钉成断言，任何一处接线断了都会在这里变红。

## 守护的四件事

1. 数量 = 连线 × 2（每端一张）。少一端的清单看起来也像模像样，
   现场贴完才发现有一端全是手写。
2. 缺编码的数量与源数据一致。它是产品要如实告知用户的数字，
   不该随实现变化漂移。
3. 机柜码形如 `11A14`，且**能按数字序排**（`1A01` 在 `12A02` 前面）。
4. 自动编号后编码全局唯一 —— 曾按通道独立计数，4230 张标签只有 235 个
   唯一编码，`SRV80` 同时出现在 4 个机柜。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from idcbox.domain.ports.generate import generate_links
from idcbox.domain.ports.intent import intent_from_dict

ROOT = Path(__file__).resolve().parents[3]
SOURCE = next(iter(ROOT.glob("信息港部署上架/*V13*设备上架*.xlsx")), None)
INTENT = ROOT / "projects" / "v13" / "intent_full.json"

pytestmark = pytest.mark.skipif(
    not (SOURCE and SOURCE.exists() and INTENT.exists()),
    reason="需要真实 V13 源文件与意图（只在完整仓库里跑）",
)


@pytest.fixture(scope="module")
def payload() -> dict:
    from fastapi.testclient import TestClient

    from idcbox.api.main import create_app

    client = TestClient(create_app())
    resp = client.get("/api/labels", params={"limit": 5000})
    assert resp.status_code == 200, resp.text
    return resp.json()


class TestRealNumbers:
    def test_one_label_per_end(self, payload: dict) -> None:
        """5127 根线 → 10254 张标签。"""
        assert payload["total"] == 10254

    def test_missing_device_code_count(self, payload: dict) -> None:
        """4230 张缺编码 —— 源表里服务器与存储节点没有实例名。

        这个数字是产品要如实告知用户的（页面顶部就挂着），不该漂移。
        """
        assert payload["countsByCode"].get("NO_DEVICE_CODE") == 4230

    def test_no_other_issue_kinds(self, payload: dict) -> None:
        """除了缺编码，不该有别的体检问题（端口名无空格、无超长）。"""
        assert set(payload["countsByCode"]) == {"NO_DEVICE_CODE"}


class TestLabelShape:
    def test_rack_code_numeric_prefix(self, payload: dict) -> None:
        codes = {r["position"].split("-")[0] for r in payload["rows"]}
        assert all("A" in c for c in codes)
        assert "1A01" in codes

    def test_switch_has_suffix(self, payload: dict) -> None:
        assert any("(SW)" in r["device"] for r in payload["rows"])

    def test_missing_code_row_has_no_double_space(self, payload: dict) -> None:
        """缺编码的行必须是单空格（`2A01-31U 万兆1`），不是双空格。"""
        rows = [r for r in payload["rows"] if not r["device"]][:20]
        assert rows, "应当有缺编码的行"
        assert all("  " not in r["text"] for r in rows)

    def test_slot_id_never_leaks(self, payload: dict) -> None:
        """槽位号 `N02-01-S0` 是内部 id，不能出现在任何标签上。"""
        assert not any("N02-01-S0" in r["text"] for r in payload["rows"])


class TestOrdering:
    def test_rack_codes_in_numeric_order(self) -> None:
        """`1A01` 必须排在 `12A02` 前面（字符串序会反过来）。

        ## 为什么这条不能用上面那个 limit=5000 的 payload

        5000 条只覆盖前几个机柜，`12A02` 根本不在里面 —— 断言会
        `ValueError: '12A02' is not in list`。必须取全量。
        """
        from fastapi.testclient import TestClient

        from idcbox.api.main import create_app

        client = TestClient(create_app())
        rows = client.get("/api/labels", params={"limit": 20000}).json()["rows"]
        order: list[str] = []
        for r in rows:
            code = r["position"].split("-")[0]
            if not order or order[-1] != code:
                order.append(code)
        #: V13 只有通道 1/2/3/8/9（没有 10 以上的通道），所以这里**测不出**
        #: `12A02` vs `1A01` 这类跨位数的坑 —— 那条由单测
        #: `TestNaturalSort::test_rack_code_numeric_order` 用合成数据守着。
        #: 这里能守的是「机柜码序列按自然序单调」，防止排序键被改成乱序。
        from idcbox.domain.exporting.labels import _natural

        assert order, "应该有标签"
        assert order == sorted(order, key=_natural), order[:10]

    def test_ports_ascending_within_device(self, payload: dict) -> None:
        """同一设备内**同速率**端口必须递增（`1,2,3,10` 不是 `1,10,2,3`）。

        ## 为什么分组键要带速率

        排序是「速率降序优先，再按板卡/槽位/序号」，所以 `100GE1/0/1` 之后
        跟的是 `100GE1/0/2`（不是 `25GE1/0/1`）。把两种速率混在一个序列里
        断言单调会得到 `[1,2,3,… 然后 1,2,3]`，看起来像乱序，其实是对的。
        """
        from idcbox.domain.ports.naming import parse_port_name

        seen: dict[tuple[str, int], list[int]] = {}
        for r in payload["rows"]:
            p = parse_port_name(r["port"])
            if p.style == "cli_board" and p.slot == 0 and p.chassis == 1:
                key = (r["position"], p.rate_mbps or 0)
                seen.setdefault(key, []).append(p.port or 0)
        checked = 0
        for ports in seen.values():
            if len(ports) > 2:
                assert ports == sorted(ports), ports
                checked += 1
        assert checked > 0, "应当有可检验的端口序列"

    def test_u_descending_or_grouped(self, payload: dict) -> None:
        """机柜内 U 位必须非递增（柜顶先贴）。"""
        prev_rack = ""
        prev_u = 999
        for r in payload["rows"]:
            rack, u = r["position"].rsplit("-", 1)
            u_num = int(u.rstrip("U"))
            if rack != prev_rack:
                prev_rack, prev_u = rack, u_num
                continue
            assert u_num <= prev_u or u_num == prev_u
            prev_u = min(prev_u, u_num)


class TestAutoNumber:
    def test_codes_unique_after_numbering(self) -> None:
        """自动编号后必须**全局唯一**。

        曾按通道独立计数 → 4 个通道各从 01 开始 → 4230 张标签只有 235 个
        唯一编码，`SRV80` 同时出现在 2A13/3A16/8A11/9A11 四个机柜。
        """
        from fastapi.testclient import TestClient

        from idcbox.api.main import create_app

        client = TestClient(create_app())
        resp = client.get("/api/labels", params={"limit": 20000, "numberMissing": "true"})
        rows = resp.json()["rows"]
        auto = [r for r in rows if r["device"].startswith(("SRV", "STG"))]
        assert auto, "应该有自动编号的标签"
        codes = {r["device"] for r in auto}
        assert len(codes) == 631, f"631 个设备位置应有 631 个编码，实得 {len(codes)}"

    def test_numbering_leaves_no_gap(self) -> None:
        from fastapi.testclient import TestClient

        from idcbox.api.main import create_app

        client = TestClient(create_app())
        resp = client.get("/api/labels", params={"limit": 20000, "numberMissing": "true"})
        assert resp.json()["countsByCode"] == {}


class TestTextFormat:
    def test_plain_text_one_per_line(self) -> None:
        """现场用法是「导出 txt 丢给打印机」，后端直接给成品。"""
        from fastapi.testclient import TestClient

        from idcbox.api.main import create_app

        client = TestClient(create_app())
        resp = client.get("/api/labels", params={"limit": 5, "format": "text"})
        assert resp.status_code == 200
        lines = [x for x in resp.text.split("\n") if x]
        assert len(lines) == 5
        assert lines[0].startswith("1A01-3U ")

    def test_chinese_filename_header(self) -> None:
        """项目名是中文，HTTP 头只能是 latin-1 —— 必须走 RFC 5987。

        直接塞进 `filename="..."` 会抛 `UnicodeEncodeError`，看起来像端点 500。
        """
        from fastapi.testclient import TestClient

        from idcbox.api.main import create_app

        client = TestClient(create_app())
        resp = client.get("/api/labels", params={"limit": 2, "format": "text"})
        disp = resp.headers["content-disposition"]
        assert "filename*=UTF-8''" in disp


class TestCli:
    def test_cli_runs(self) -> None:
        from idcbox.cli import main

        code = main(["labels", "--intent", str(INTENT), "--limit", "3"])
        assert code == 0

    def test_cli_writes_json(self, tmp_path: Path) -> None:
        from idcbox.cli import main

        out = tmp_path / "labels.json"
        code = main(["labels", "--intent", str(INTENT), "--limit", "5", "--out", str(out)])
        assert code == 0
        doc = json.loads(out.read_text(encoding="utf-8"))
        assert doc["total"] == 10254
        assert len(doc["rows"]) == 5
