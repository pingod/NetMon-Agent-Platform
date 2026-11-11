"""golden 用例的公共装置。

## 三条设计原则

1. **数据只读一次**：V13 的 5127 行解析要 1~2 秒，session 级 fixture 缓存。
2. **源文件缺失不跳过**：宁可整个 golden 套件红，也不要"静默跳过"导致 CI 绿着漏检。
   离线环境靠 `tests/golden/data/` 里的 fixture 副本。
3. **基线 JSON 是判据**：所有断言都和 `baseline/*.json` 比对，而不是硬编码数字 ——
   数字改了必须重新生成基线并 review diff，杜绝"漂移被当成新基线"。
"""

from __future__ import annotations

import json
import warnings
from functools import lru_cache
from pathlib import Path

import pytest

warnings.filterwarnings("ignore")

BACKEND = Path(__file__).resolve().parent.parent.parent
ROOT = BACKEND.parent
BASELINE_DIR = Path(__file__).resolve().parent / "baseline"

# 路径解析与基线读取的**纯函数**放在独立模块里（见 paths.py / baseline_io.py）：
# 两个 conftest.py 同时存在时 `from conftest import ...` 会解析错文件。
from baseline_io import load_baseline  # noqa: E402
from paths import source_path  # noqa: E402


# ---------------------------------------------------------------- 几何


@lru_cache(maxsize=1)
def geometry():
    """解析机房平面图，返回 (FloorParseResult, GeometrySnapshot)。"""
    from idcbox.domain.geometry.floor import (
        FloorCell,
        derive_cmax_by_room,
        parse_floor,
    )
    from idcbox.domain.geometry.layout import build_snapshot
    from idcbox.infrastructure.xlsx.openpyxl_reader import (
        FLOOR_SCAN_COLS,
        FLOOR_SCAN_ROWS,
        read_floor_cells,
    )

    cells = read_floor_cells(
        source_path(), row_range=FLOOR_SCAN_ROWS, col_range=FLOOR_SCAN_COLS
    )
    floor = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
    cmax = derive_cmax_by_room(floor.racks)
    snap = build_snapshot(floor.racks, version="v13", cmax_override=cmax)
    return floor, snap


@lru_cache(maxsize=1)
def cabling_rows():
    """布线表原始值 + 行号。"""
    from idcbox.infrastructure.xlsx.openpyxl_reader import read_v13_cabling

    sheet = read_v13_cabling(source_path())
    return [list(r.values) for r in sheet.rows], [r.row_no for r in sheet.rows]


@lru_cache(maxsize=1)
def column_map():
    return {
        "model_local": 0, "dev_local": 1, "ch_local": 2, "rack_local": 3,
        "u_local": 4, "port_local": 5,
        "model_peer": 6, "dev_peer": 7, "ch_peer": 8, "rack_peer": 9,
        "u_peer": 10, "port_peer": 11,
        "cable_type": 12, "n_cross": 13, "o_gap": 14, "p_count": 15,
        "q_in_rack": 16, "r_inter": 17, "s_total": 18, "t_ladder": 19,
        "u_approved": 20, "mod_local": 21, "mod_peer": 22, "remark": 23,
    }


@lru_cache(maxsize=1)
def empirical_table():
    from idcbox.infrastructure.config.empirical_loader import load_empirical_table

    return load_empirical_table(ROOT / "rules" / "len_empirical_v1.json")


@pytest.fixture(scope="session")
def geo():
    return geometry()[1]


@pytest.fixture(scope="session")
def floor_parse():
    return geometry()[0]


@pytest.fixture(scope="session")
def rows():
    return cabling_rows()[0]


@pytest.fixture(scope="session")
def row_nos():
    return cabling_rows()[1]


@pytest.fixture(scope="session")
def col():
    return column_map()


@pytest.fixture(scope="session")
def emp_table():
    return empirical_table()
