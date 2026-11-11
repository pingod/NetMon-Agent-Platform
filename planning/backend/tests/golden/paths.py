"""golden 用例的路径解析（独立小模块，供各 golden 用例 import）。

## 为什么 `source_path()` 不能留在 `conftest.py`

`tests/unit/conftest.py` 与 `tests/golden/conftest.py` 同时存在时，pytest 会把
两个测试目录都插进 `sys.path`，于是 `from conftest import source_path` 会
**按 sys.path 顺序解析到 unit 的那个**，报 `ImportError: cannot import name
'source_path'` —— 一个只在你同时跑两个目录时才出现的隐蔽错误。

所以：`conftest.py` 里只放 fixture（pytest 自己按目录就近加载，不会串），
所有可被 import 的纯函数放独立模块。
"""

from __future__ import annotations

from pathlib import Path

BACKEND = Path(__file__).resolve().parent.parent.parent
ROOT = BACKEND.parent
BASELINE_DIR = Path(__file__).resolve().parent / "baseline"

#: 源文件搜索顺序：先 fixtures 副本（离线可复现），再原始位置
SOURCE_CANDIDATES: tuple[Path, ...] = (
    Path(__file__).resolve().parent / "data" / "v13_cabling.xlsx",
    ROOT / "信息港部署上架" / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx",
)


def source_path() -> Path:
    """定位 V13 源文件。找不到就报错，**不 skip**。

    宁可整个 golden 套件红，也不要静默跳过 —— 静默跳过的 CI 是绿的假象，
    会在真正需要回归保护的时刻（重构公式）失去保护。
    """
    for p in SOURCE_CANDIDATES:
        if p.exists():
            return p
    raise FileNotFoundError(
        "找不到 V13 源文件。请把文件放到以下任一位置：\n  "
        + "\n  ".join(str(p) for p in SOURCE_CANDIDATES)
    )
