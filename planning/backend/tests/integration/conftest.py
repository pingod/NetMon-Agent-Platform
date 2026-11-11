"""集成测试的公共夹具：复用 golden 的路径解析与列映射。"""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

GOLDEN = Path(__file__).resolve().parent.parent / "golden"
if str(GOLDEN) not in sys.path:
    sys.path.insert(0, str(GOLDEN))

from baseline_io import load_baseline  # noqa: E402
from paths import source_path  # noqa: E402


@pytest.fixture(scope="session")
def src() -> Path:
    return source_path()


@pytest.fixture(scope="session")
def geo(src):
    from idcbox.domain.geometry.floor import FloorCell, derive_cmax_by_room, parse_floor
    from idcbox.domain.geometry.layout import build_snapshot
    from idcbox.infrastructure.xlsx.openpyxl_reader import read_floor_cells

    cells = read_floor_cells(src)
    floor = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
    cmax = derive_cmax_by_room(floor.racks)
    return build_snapshot(floor.racks, version="V13", cmax_override=cmax)


@pytest.fixture(scope="session")
def rows(src):
    from idcbox.infrastructure.xlsx.openpyxl_reader import read_v13_cabling

    sheet = read_v13_cabling(src, "布线表")
    return [r.values for r in sheet.rows]
