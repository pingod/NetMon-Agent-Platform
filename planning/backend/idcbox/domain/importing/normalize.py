"""导入期的字段归一化（域层，零 IO）。

真实数据的脏点分两类，处理方式**必须不同**：

| 类型 | 例子 | 处理 |
|---|---|---|
| 格式噪声 | `业务接入交换机 1` vs `业务接入交换机1`、`rack1` vs `1`、尾随空格 | **归一**，视为同一实体 |
| 业务 sentinel | `一`、`设备自带`、`待定`（放在长度/模块列） | **保留原值**，不归一，转人工语义 |

把 sentinel 当噪声归一掉，会让「设备自带」变成空值，导出时原样列就丢了；
把格式噪声当业务值保留，会让同一台设备变成两个实体。这个区分是导入层的第一原则。
"""

from __future__ import annotations

import re
from typing import Final

#: 长度列的 sentinel 文本：出现即表示「没有可用数值」
LENGTH_SENTINELS: Final[frozenset[str]] = frozenset(
    {"一", "设备自带", "待定", "-", "—", "/", "无", "N/A", "NA", "n/a"}
)

#: 模块列的 sentinel：`一` 表示铜缆直连无需模块
MODULE_SENTINELS: Final[frozenset[str]] = frozenset({"一", "-", "—", "无"})

_SPACES = re.compile(r"[\s\u3000]+")
_RACK_PREFIX = re.compile(r"^(rack|RACK|机柜)\s*[-_]?\s*", re.IGNORECASE)


def norm_name(value: object) -> str:
    """设备/型号名归一：去所有空白（含全角空格）。

    实测 4 个型号名带空格（`业务接入交换机 1`、`业务接入交换机 2`、
    `存储接入交换机 1`、`带外交换机 1`），与不带空格的是同一型号；
    不归一会让 834 个实例虚增到 836 个。

    >>> norm_name("业务接入交换机 1 ")
    '业务接入交换机1'
    >>> norm_name("  带外交换机　1")
    '带外交换机1'
    """
    if value is None:
        return ""
    return _SPACES.sub("", str(value))


def norm_text(value: object) -> str:
    """普通文本归一：只去首尾空白，不动中间（备注等自由文本不能被破坏）。"""
    if value is None:
        return ""
    return str(value).strip()


def norm_int(value: object) -> int | None:
    """机柜号/通道号归一：支持 `1`、`1.0`、`rack1`、`2号柜`、` 3 `。"""
    if value is None:
        return None
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return int(value) if value.is_integer() else None
    s = norm_text(value)
    if not s:
        return None
    s = _RACK_PREFIX.sub("", s)
    m = re.search(r"\d+", s)
    return int(m.group()) if m else None


def norm_cable_type(value: object) -> str:
    """线缆类型归一。真实值有 `LC-LC` `RJ45` `MPO`，也有中文 `40GMPO线缆`。

    >>> norm_cable_type("lc-lc"), norm_cable_type("RJ45"), norm_cable_type("MPO")
    ('LC-LC', 'RJ45', 'MPO')
    >>> norm_cable_type("40GMPO线缆")
    'MPO'
    >>> norm_cable_type("万兆光纤")
    'LC-LC'
    """
    s = norm_text(value).upper()
    if not s:
        return "UNKNOWN"
    if "MPO" in s:
        return "MPO"
    if "RJ45" in s or "网线" in s or "网口" in s:
        return "RJ45"
    if "LC" in s or "光纤" in s or "光缆" in s:
        return "LC-LC"
    return s


def is_length_sentinel(value: object) -> bool:
    """判断一个单元格是否是「没有数值」的 sentinel 文本。"""
    s = norm_text(value)
    return bool(s) and s in LENGTH_SENTINELS


def parse_length(value: object) -> tuple[int | None, str | None]:
    """把长度单元格解析成 (数值, 原始文本)。

    返回三种组合：
    - `(5, None)` 正常数值
    - `(None, "待定")` 文本 sentinel —— **原值必须保留**，导出时原样输出
    - `(None, None)` 空

    >>> parse_length(10)
    (10, None)
    >>> parse_length("10")
    (10, None)
    >>> parse_length("待定")
    (None, '待定')
    >>> parse_length(None)
    (None, None)
    """
    if value is None:
        return None, None
    if isinstance(value, bool):
        return None, str(value)
    if isinstance(value, int):
        return value, None
    if isinstance(value, float):
        return (int(value), None) if value.is_integer() else (None, str(value))
    s = norm_text(value)
    if not s:
        return None, None
    try:
        f = float(s)
    except ValueError:
        return None, s
    return (int(f), None) if f.is_integer() else (None, s)


def norm_u_position(value: object) -> str | None:
    """U 位归一成 `3U` / `3-12U` / `40U` 这种标准写法。

    实测 13 种形态：`40U`(2685)、`37U`(1947)、`34U`(184)、`38U`、
    区间 `3-4U` `7-8U` ... `3-12U`（占 10U）、`19-20U`。

    区间符号统一成 ASCII 连字符：中文破折号 `—`、波浪号 `~`、全角减号
    `－` 都要收敛。这不是洁癖 —— 归一结果会被用作设备实例的**身份键**
    （`(通道, 机柜, U位, 型号)`），`3-12U` 与 `3—12U` 若不收敛就是两个实例，
    会让 U 位冲突检测把同一台设备报成两台叠放。

    >>> norm_u_position("40U"), norm_u_position(" 3-12U "), norm_u_position("3-4")
    ('40U', '3-12U', '3-4U')
    >>> norm_u_position("3—12U")
    '3-12U'
    """
    s = norm_text(value).upper().replace(" ", "")
    if not s:
        return None
    s = s.replace("Ｕ", "U")
    # 区间连接符统一（顺序有讲究：先换全角减号，再换破折号/波浪号）
    for dash in ("－", "–", "—", "~", "～"):
        s = s.replace(dash, "-")
    if not s.endswith("U"):
        s += "U"
    return s


def u_position_span(value: object) -> tuple[int | None, int | None]:
    """U 位 → (起始 U, 结束 U)。

    >>> u_position_span("40U"), u_position_span("3-12U"), u_position_span("19-20U")
    ((40, 40), (3, 12), (19, 20))
    """
    s = norm_text(value).upper().replace(" ", "").replace("Ｕ", "U").rstrip("U")
    if not s:
        return None, None
    if "-" in s or "~" in s or "—" in s:
        m = re.match(r"^(\d+)\s*[-~—]\s*(\d+)$", s)
        if m:
            a, b = int(m.group(1)), int(m.group(2))
            return (a, b) if a <= b else (b, a)
        return None, None
    m = re.match(r"^(\d+)$", s)
    return (int(m.group(1)), int(m.group(1))) if m else (None, None)


def make_stable_key(*parts: object) -> str:
    """生成稳定键。用 `|` 连接，空值写 `∅`，避免 `a||b` 与 `a|∅|b` 混淆。"""
    return "|".join("∅" if p is None or p == "" else str(p) for p in parts)
