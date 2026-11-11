"""golden 基线读取（独立小模块，供各 golden 用例 import）。

为什么单独一个模块而不是放在 `conftest.py`：
`conftest.py` 不能被测试模块用相对导入引用（pytest 以 rootdir 目录方式插入 sys.path，
测试模块不是包）。所以把纯函数放这里，`conftest.py` 只留 fixture。
"""

from __future__ import annotations

import json
from pathlib import Path

BASELINE_DIR = Path(__file__).resolve().parent / "baseline"


def load_baseline(name: str) -> dict:
    """读取 `baseline/{name}.json`。缺文件直接报错，不返回空 dict。"""
    path = BASELINE_DIR / f"{name}.json"
    if not path.exists():
        raise FileNotFoundError(
            f"基线文件不存在：{path}。请先运行 backend/scripts/build_{name.split('_')[0]}_baseline.py"
        )
    with path.open(encoding="utf-8") as f:
        return json.load(f)
