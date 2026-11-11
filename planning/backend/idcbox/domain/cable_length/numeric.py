"""数值口径：Excel 15 位有效数字。

## 为什么不能用原始双精度

Excel 存值只保留 15 位有效数字。原表公式 `R = P*0.6*120%` 在 Excel 里
P=5 得到 `3.6`，而 Python 直接算 `5 * 0.6 * 1.2` 得到 `3.5999999999999996`。
实测 636 行里有 **170 行**因此与 Excel 存值不一致 —— 逐行 diff 的验收铁线
会当场挂掉。

`excel15()` 把所有中间结果收敛到 15 位有效数字口径，之后 R/S/P 全部 636/636 一致。
"""

from __future__ import annotations

import math
from typing import Final

from ..units import EXCEL_SIGNIFICANT_DIGITS, FLOAT_REL_TOL


def excel15(x: float) -> float:
    """把双精度结果收敛到 Excel 的 15 位有效数字口径。

    >>> excel15(5 * 0.6 * 1.2)
    3.6
    >>> excel15(0.1 + 0.2)
    0.3
    >>> excel15(0.0)
    0.0
    """
    if x == 0.0 or not math.isfinite(x):
        return x
    return float(f"{x:.{EXCEL_SIGNIFICANT_DIGITS}g}")


def feq(a: float | None, b: float | None) -> bool:
    """浮点相等：先过 excel15，再容差比较。禁止字符串比较。"""
    if a is None or b is None:
        return a is None and b is None
    a15, b15 = excel15(a), excel15(b)
    if a15 == b15:
        return True
    return math.isclose(a15, b15, rel_tol=FLOAT_REL_TOL, abs_tol=0.0)


def to_number(value: object) -> float | int | None:
    """把 Excel 里的「看起来是数字」的值转成数字，否则 None。

    只做**类型收敛**，不做业务归一（sentinel 文本如 `一` / `设备自带` / `待定`
    由导入层处理，域层不认识它们）。整数返回 int，避免 15.0 这种噪声。
    """
    if value is None:
        return None
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return int(value) if value.is_integer() else value
    if isinstance(value, str):
        s = value.strip()
        if not s:
            return None
        try:
            f = float(s)
        except ValueError:
            return None
        return int(f) if f.is_integer() else f
    return None


#: 长度档位比较时允许的浮点误差（例如 8.5 与 8.5000000001）
TIER_EPS: Final[float] = 1e-9


def tier_le(value: float, limit: float) -> bool:
    """档位比较的 `<=`：容忍浮点噪声，但不掩盖真实越界。"""
    return value <= limit + TIER_EPS


def tier_eq(value: float, limit: float) -> bool:
    return math.isclose(value, limit, rel_tol=0.0, abs_tol=TIER_EPS)


def tier_lt(value: float, limit: float) -> bool:
    return value < limit - TIER_EPS
