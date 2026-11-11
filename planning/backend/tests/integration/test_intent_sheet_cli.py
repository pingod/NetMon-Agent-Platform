"""接线意图表格的 CLI 与 API 入口（端到端）。

## 这组用例守护什么

`test_golden_sheet.py` 证明「表格与 Intent 一一对应」。本文件证明**用户能走通**：

    intent-export  →  人在 Excel 里改  →  intent-check  →  generate

以及 API 侧认同一份 Excel（`IDCBOX_INTENT` 指向 .xlsx）。

## 为什么必须端到端

CLI 的参数分派与 API 的环境变量分派都是「胶水」，胶水错了的表现是
**静默用了默认路径**（用户以为读了他改的表，实际读的是旧 JSON）。
这类 bug 只有真正从文件读起才能抓到。
"""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
INTENT_JSON = ROOT / "projects" / "v13" / "intent_full.json"
SOURCE = ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"

pytestmark = pytest.mark.skipif(
    not SOURCE.exists(), reason="需要真实 V13 源文件（只在完整仓库里跑）"
)


def _run_cli(*args: str) -> subprocess.CompletedProcess[str]:
    """用 venv 的 python 跑 CLI。必须 `env -u PYTHONHOME` —— 本机 PYTHONHOME 被污染。"""
    return subprocess.run(
        [sys.executable, "-m", "idcbox.cli", *args],
        cwd=BACKEND,
        capture_output=True,
        text=True,
        timeout=600,
    )


@pytest.fixture(scope="module")
def intent_xlsx(tmp_path_factory) -> Path:
    """导出一次六段表格，整个模块共用（导出 949 行约 1 秒）。"""
    out = tmp_path_factory.mktemp("intent") / "intent.xlsx"
    result = _run_cli(
        "intent-export", "--intent", str(INTENT_JSON), "--out", str(out)
    )
    assert result.returncode == 0, result.stderr
    return out


# --------------------------------------------------------------- intent-export


def test_intent_export_writes_six_sheets(intent_xlsx: Path) -> None:
    import openpyxl

    wb = openpyxl.load_workbook(intent_xlsx)
    try:
        assert wb.sheetnames == [
            "布局", "交换机", "特殊设备", "路由", "交换机组", "显式连线",
        ]
    finally:
        wb.close()


def test_intent_export_reports_913_rows(intent_xlsx: Path) -> None:
    """导出时的口径行数（949 = 28+130+34+34+26+697）。"""
    result = _run_cli(
        "intent-export", "--intent", str(INTENT_JSON), "--out", str(intent_xlsx)
    )
    assert "949 行" in result.stdout


def test_intent_export_missing_file_is_usage_error(tmp_path: Path) -> None:
    result = _run_cli(
        "intent-export", "--intent", str(tmp_path / "nope.json"),
        "--out", str(tmp_path / "x.xlsx"),
    )
    assert result.returncode != 0
    assert "不存在" in result.stderr


# ---------------------------------------------------------------- intent-check


def test_intent_check_reads_all_six(intent_xlsx: Path) -> None:
    result = _run_cli(
        "intent-check", "--sheet", str(intent_xlsx),
        "--project", "V13(内部)甘肃信息港-设备上架规划",
    )
    assert result.returncode == 0, result.stderr
    assert "找到 6 张表" in result.stdout
    assert "949 行" in result.stdout
    assert "666 台服务器" in result.stdout


def test_intent_check_reports_missing_required_sheet(intent_xlsx: Path, tmp_path: Path) -> None:
    """删掉「交换机组」后必须报错退出（非 0），不能静默生成 0 行。"""
    import openpyxl

    broken = tmp_path / "broken.xlsx"
    broken.write_bytes(intent_xlsx.read_bytes())
    wb = openpyxl.load_workbook(broken)
    del wb["交换机组"]
    wb.save(broken)
    wb.close()

    result = _run_cli("intent-check", "--sheet", str(broken))
    assert result.returncode != 0
    assert "缺少必需的表" in result.stdout
    assert "交换机组" in result.stdout


def test_intent_check_can_dump_json(intent_xlsx: Path, tmp_path: Path) -> None:
    out = tmp_path / "from_sheet.json"
    result = _run_cli(
        "intent-check", "--sheet", str(intent_xlsx),
        "--project", "V13(内部)甘肃信息港-设备上架规划",
        "--site", "甘肃信息港",
        "--out", str(out),
    )
    assert result.returncode == 0, result.stderr
    assert out.exists()

    from idcbox.domain.ports.intent import load_intent

    assert load_intent(out) == load_intent(INTENT_JSON)


# ------------------------------------------------------------------ generate


def test_generate_from_sheet_matches_generate_from_json(
    intent_xlsx: Path, tmp_path: Path
) -> None:
    """**核心**：两条命令（`--intent` 与 `--sheet`）产出同一张表。

    逐字节比 xlsx 不可行（时间戳/zip 顺序），所以比行数与关键列。
    完整的逐格比对在 `test_golden_sheet.py` 的域层用例里做（更快）。
    """
    import openpyxl

    out_json = tmp_path / "from_json.xlsx"
    out_sheet = tmp_path / "from_sheet.xlsx"

    for extra, out in (
        (["--intent", str(INTENT_JSON)], out_json),
        (["--sheet", str(intent_xlsx),
          "--project", "V13(内部)甘肃信息港-设备上架规划"], out_sheet),
    ):
        result = _run_cli(
            "generate", *extra, "--out", str(out), "--source", str(SOURCE)
        )
        assert result.returncode == 0, result.stderr
        assert "5127 行 × 24 列" in result.stdout

    def rows(path: Path) -> list[tuple[str, ...]]:
        wb = openpyxl.load_workbook(path, data_only=True)
        try:
            ws = wb["布线表"]
            return [
                tuple("" if v is None else str(v).strip() for v in r)
                for r in ws.iter_rows(values_only=True)
                if any(v not in (None, "") for v in r)
            ]
        finally:
            wb.close()

    assert rows(out_json) == rows(out_sheet)


def test_generate_rejects_both_sources(intent_xlsx: Path, tmp_path: Path) -> None:
    """`--intent` 与 `--sheet` 同时给要报错，不能默默选一个。"""
    result = _run_cli(
        "generate",
        "--intent", str(INTENT_JSON),
        "--sheet", str(intent_xlsx),
        "--out", str(tmp_path / "x.xlsx"),
    )
    assert result.returncode != 0
    assert "不能同时给" in result.stderr


def test_generate_without_source_is_usage_error(tmp_path: Path) -> None:
    result = _run_cli("generate", "--out", str(tmp_path / "x.xlsx"))
    assert result.returncode != 0
    assert "必须指定意图来源" in result.stderr


# ---------------------------------------------------------------------- API


def test_api_reads_excel_intent(intent_xlsx: Path, monkeypatch) -> None:
    """`IDCBOX_INTENT` 指向 .xlsx 时 API 直接读表格。"""
    from fastapi.testclient import TestClient

    from idcbox.api.main import create_app

    monkeypatch.setenv("IDCBOX_INTENT", str(intent_xlsx))
    client = TestClient(create_app())
    response = client.get("/api/design", params={"limit": 3, "compare": "false"})
    assert response.status_code == 200, response.text
    data = response.json()
    assert data["stats"]["links"] == 5127
    assert data["intent"]["total"] == 949
    assert data["intent"]["sourceFormat"] == "xlsx"
    assert data["rows"][0]["localDevice"] == "计算接入交换机(业务)-1"


def test_api_reports_missing_required_sheet(intent_xlsx: Path, tmp_path: Path, monkeypatch) -> None:
    """缺必需表要返回 422 且给出可操作的提示，不是 500。"""
    import openpyxl
    from fastapi.testclient import TestClient

    from idcbox.api.main import create_app

    broken = tmp_path / "broken.xlsx"
    broken.write_bytes(intent_xlsx.read_bytes())
    wb = openpyxl.load_workbook(broken)
    del wb["路由"]
    wb.save(broken)
    wb.close()

    monkeypatch.setenv("IDCBOX_INTENT", str(broken))
    client = TestClient(create_app())
    response = client.get("/api/design", params={"compare": "false"})
    assert response.status_code == 422
    assert "路由" in response.json()["detail"]
