"""耗材清单的 CLI 与 API 端到端（真实 V13 数据）。

## 这组用例守护的是什么

`consumables.py` 的纯函数已被 `tests/unit/test_consumables.py` 覆盖，
但**服务层与入口的接线**没被证明过。这里用真实项目跑通
`views → consumable_payload → CLI 打印 / API JSON` 三条路，
并断言数字与原表逐项相等。

## 与 golden 用例的分工

`test_golden_consumables.py` 只调域层纯函数（快，但绕过了适配层）。
这里走 `load_project` 全流程（慢，但用的是用户真实会走的那条路）——
如果 `v13_adapter` 或 `pipeline` 把模块列弄丢了，只有这里能发现。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

pytestmark = pytest.mark.skipif(
    not (
        Path(__file__).resolve().parents[3]
        / "信息港部署上架"
        / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"
    ).exists(),
    reason="需要真实 V13 源文件（只在完整仓库里跑）",
)

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
SOURCE = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"


@pytest.fixture(scope="module")
def payload_first() -> dict:
    """首期口径的清单 payload。"""
    import sys

    if str(BACKEND) not in sys.path:
        sys.path.insert(0, str(BACKEND))
    from idcbox.application.services.pipeline import compute_project, load_project
    from idcbox.application.services.views import build_views, consumable_payload
    from idcbox.infrastructure.xlsx.sources import XlsxCablingSource, XlsxFloorSource

    project = load_project(
        floor_source=XlsxFloorSource(SOURCE),
        cabling_source=XlsxCablingSource(SOURCE),
        label="V13",
        source_path=SOURCE,
    )
    views = build_views(project, compute_project(project))
    return consumable_payload(views, phase="首期")


@pytest.fixture(scope="module")
def payload_all() -> dict:
    import sys

    if str(BACKEND) not in sys.path:
        sys.path.insert(0, str(BACKEND))
    from idcbox.application.services.pipeline import compute_project, load_project
    from idcbox.application.services.views import build_views, consumable_payload
    from idcbox.infrastructure.xlsx.sources import XlsxCablingSource, XlsxFloorSource

    project = load_project(
        floor_source=XlsxFloorSource(SOURCE),
        cabling_source=XlsxCablingSource(SOURCE),
        label="V13",
        source_path=SOURCE,
    )
    views = build_views(project, compute_project(project))
    return consumable_payload(views, phase=None)


class TestFirstPeriodPayload:
    """首期口径必须逐项复现原表 `首期耗材需求清单`。"""

    def test_row_count(self, payload_first: dict) -> None:
        assert payload_first["rowsTotal"] == 450
        assert payload_first["phase"] == "首期"

    def test_cable_total_and_types(self, payload_first: dict) -> None:
        cables = payload_first["cables"]
        assert cables["total"] == 450
        assert cables["byType"] == {"LC-LC": 204, "MPO": 88, "RJ45": 158}

    def test_cable_rows_match_source(self, payload_first: dict) -> None:
        """逐组对照原表 24 组（需求为 0 的档位原表有、这里不报）。"""
        want = {
            ("LC-LC", 5): 24, ("LC-LC", 7): 1, ("LC-LC", 10): 115, ("LC-LC", 15): 19,
            ("LC-LC", 20): 8, ("LC-LC", 25): 34, ("LC-LC", 30): 3,
            ("MPO", 10): 16, ("MPO", 15): 38, ("MPO", 20): 10, ("MPO", 25): 22,
            ("MPO", 30): 2,
            ("RJ45", 5): 18, ("RJ45", 7): 10, ("RJ45", 10): 87, ("RJ45", 15): 16,
            ("RJ45", 25): 27,
        }
        got = {(r["type"], r["length"]): r["demand"] for r in payload_first["cables"]["rows"]}
        assert got == want

    def test_modules_match_source(self, payload_first: dict) -> None:
        """4 类模块本端/对端必须逐项相等 —— 这是采购的依据。"""
        want = {"100G-SR4": (90, 88), "10G-SR": (112, 28), "25G-SR": (85, 15), "光转电": (12, 0)}
        got = {r["module"]: (r["local"], r["peer"]) for r in payload_first["modules"]["rows"]}
        assert got == want

    def test_disclosure_present(self, payload_first: dict) -> None:
        """覆盖率披露字段必须存在且说明未外推。"""
        mods = payload_first["modules"]
        assert mods["coverageRows"] > 0
        assert mods["totalRows"] == 450
        assert "未做外推" in mods["disclosure"]

    def test_suggested_purchase_is_not_final(self, payload_first: dict) -> None:
        """建议采购量存在但不是原表人工值。

        原表 `首期耗材需求清单` 的采购列是人工核定的（LC-LC 10m 需求 115 采 130、
        MPO 15m 需求 38 采 30），与 1.3 倍系数都对不上。所以这里断言
        「建议值存在且不等于需求」，不断言它等于任何具体值 ——
        一旦有人把它调成"能复现原表"，这条用例会失败提醒他那是拟合。
        """
        cables = payload_first["cables"]
        assert cables["purchaseRatio"] == 1.3
        for row in cables["rows"]:
            assert row["suggestedPurchase"] >= row["demand"]
        # 至少有一行与原表人工值不同（LC-LC 10m：建议 150，原表 130）
        lc10 = next(
            r for r in cables["rows"] if r["type"] == "LC-LC" and r["length"] == 10
        )
        assert lc10["suggestedPurchase"] == 150
        assert lc10["suggestedPurchase"] != 130


class TestFullProjectPayload:
    """全项目口径（不过滤批次）。"""

    def test_row_count_is_all_links(self, payload_all: dict) -> None:
        assert payload_all["rowsTotal"] == 5127
        assert payload_all["phase"] is None

    def test_cable_total_is_eleven_times_first_period(self, payload_all: dict) -> None:
        """全量 5127 根 ≈ 首期 450 根的 11.4 倍 —— 这就是必须让用户选口径的原因。"""
        assert payload_all["cables"]["total"] == 5127

    def test_module_coverage_is_low(self, payload_all: dict) -> None:
        """全量口径下模块覆盖率只有约 11% —— 披露文案必须体现。"""
        mods = payload_all["modules"]
        assert mods["totalRows"] == 5127
        assert 0.09 < mods["coverageRatio"] < 0.15
        assert "未做外推" in mods["disclosure"]


class TestApiEndpoint:
    """HTTP 端点。"""

    @pytest.fixture(scope="class")
    def client(self):
        import os
        import sys

        if str(BACKEND) not in sys.path:
            sys.path.insert(0, str(BACKEND))
        os.environ.setdefault("IDCBOX_SOURCE", str(SOURCE))
        from fastapi.testclient import TestClient

        from idcbox.api.main import create_app

        return TestClient(create_app())

    def test_phase_filter(self, client) -> None:
        r = client.get("/api/consumables", params={"phase": "首期"})
        assert r.status_code == 200
        body = r.json()
        assert body["rowsTotal"] == 450
        assert body["cables"]["total"] == 450

    def test_no_phase_returns_all(self, client) -> None:
        r = client.get("/api/consumables")
        assert r.status_code == 200
        assert r.json()["rowsTotal"] == 5127

    def test_payload_is_json_serializable(self, client) -> None:
        """整个 payload 必须能进 JSON（不能混入 tuple / Enum）。"""
        body = client.get("/api/consumables", params={"phase": "首期"}).json()
        assert json.loads(json.dumps(body)) == body


class TestCliCommand:
    """CLI `consumables` 子命令。"""

    def test_prints_expected_numbers(self, capsys) -> None:
        from idcbox.cli import main

        code = main(
            [
                "consumables",
                "--phase",
                "首期",
                "--source",
                str(SOURCE),
            ]
        )
        out = capsys.readouterr().out
        assert code == 0
        assert "450 根" in out
        assert "LC-LC 204" in out
        assert "100G-SR4" in out
        assert "未做外推" in out

    def test_writes_json_when_asked(self, tmp_path: Path) -> None:
        from idcbox.cli import main

        out = tmp_path / "cons.json"
        code = main(
            [
                "consumables",
                "--phase",
                "首期",
                "--source",
                str(SOURCE),
                "--out",
                str(out),
            ]
        )
        assert code == 0
        assert out.exists()
        body = json.loads(out.read_text(encoding="utf-8"))
        assert body["cables"]["total"] == 450
