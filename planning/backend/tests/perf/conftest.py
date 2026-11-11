"""性能用例的夹具：复用 golden 的路径解析。

`paths.py` 与 `baseline_io.py` 放在 `tests/golden/` 下作为**纯模块**，
任何测试目录要用就把它加进 sys.path —— 不能 `from conftest import`，
因为多个 conftest.py 同时存在时 import 会解析到别的文件。
"""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

GOLDEN = Path(__file__).resolve().parent.parent / "golden"
if str(GOLDEN) not in sys.path:
    sys.path.insert(0, str(GOLDEN))

from paths import source_path  # noqa: E402


@pytest.fixture(scope="session")
def src() -> Path:
    return source_path()
