"""端口命名的解析与生成。

## 两套命名体系必须双轨建模

真实数据里端口名有两套完全不同的体系（这是数据模型的关键约束）：

| 侧 | 形态 | 例子 | 数量 |
|---|---|---|---|
| 交换机侧（CLI 板卡式） | `^(GE\\|10GE\\|25GE\\|40GE\\|100GE)\\d+/\\d+/\\d+$` | `100GE1/0/1`、`GE1/0/2` | GE 2077、10GE 2277、25GE 414、100GE 354 |
| 服务器侧（标签式） | 中文速率标签 + 序号 | `千兆1`、`万兆2`、`25G2`、`IPMI` | 3604 + 290 + 684 |

服务器侧还有一个**别名**问题：同一个物理口在不同表里可能叫 `万兆1` 或 `10GE1`。
故端口需要「物理名 + 显示别名」双轨，别名映射表 `name_alias` 带上来源系统。
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Final, Literal

PortStyle = Literal["cli_board", "label", "unknown"]

#: CLI 板卡式：`100GE1/0/1`
CLI_PATTERN: Final[re.Pattern[str]] = re.compile(
    r"^(?P<rate>GE|10GE|25GE|40GE|100GE|400GE)(?P<chassis>\d+)/(?P<slot>\d+)/(?P<port>\d+)$"
)

#: 服务器侧速率标签 → Mbps
LABEL_RATE_MAP: Final[dict[str, int]] = {
    "千兆": 1_000,
    "万兆": 10_000,
    "10G": 10_000,
    "25G": 25_000,
    "40G": 40_000,
    "100G": 100_000,
    "IPMI": 1_000,  # IPMI 是 1G 管理口
    "GE": 1_000,
}

#: 服务器侧序列号：`千兆1`、`万兆12`、`25G2`、`IPMI`（无序号）
LABEL_PATTERN: Final[re.Pattern[str]] = re.compile(
    r"^(?P<label>千兆|万兆|10G|25G|40G|100G)(?P<idx>\d+)$"
)
IPMI_PATTERN: Final[re.Pattern[str]] = re.compile(r"^IPMI(\d*)$", re.IGNORECASE)

#: `GE8` / `10GE8` 这种「速率前缀 + 纯序号、没有板卡路径」的写法。
#:
#: 实测只出现在 `广域网链路负载均衡` 的 2 台设备上（各 1 个口）。
#: 它不是笔误 —— 该型号是安全设备，厂家 CLI 就叫 `GE8`。
#: 归到 label 式（label=`GE`，index=8），否则这个口会被算成"未知"，
#: 那台设备就永远差一个口对不上账。
BARE_PATTERN: Final[re.Pattern[str]] = re.compile(
    r"^(?P<label>GE|10GE|25GE|40GE|100GE)(?P<idx>\d+)$"
)


@dataclass(frozen=True, slots=True)
class PortName:
    """端口名的结构化解析结果。`raw` 永远保留原样，用于导出与日志。"""

    raw: str
    style: PortStyle
    #: 速率（Mbps），未知为 None
    rate_mbps: int | None = None
    #: CLI 式的板卡路径（chassis/slot/port），标签式为 None
    chassis: int | None = None
    slot: int | None = None
    port: int | None = None
    #: 标签式的标签名与序号
    label: str | None = None
    index: int | None = None
    #: 是否管理口（IPMI / 带外）
    is_mgmt: bool = False

    @property
    def is_known(self) -> bool:
        return self.style != "unknown"


def parse_port_name(raw: object) -> PortName:
    """解析端口名。**永不抛异常**，无法识别就归 `unknown`。

    >>> p = parse_port_name("100GE1/0/5")
    >>> p.style, p.rate_mbps, (p.chassis, p.slot, p.port)
    ('cli_board', 100000, (1, 0, 5))
    >>> q = parse_port_name("千兆2")
    >>> q.style, q.rate_mbps, (q.label, q.index)
    ('label', 1000, ('千兆', 2))
    >>> parse_port_name("IPMI").is_mgmt
    True
    >>> parse_port_name("Eth1/2/1").style
    'unknown'
    """
    s = "" if raw is None else str(raw).strip()
    if not s:
        return PortName(raw=s, style="unknown")

    m = CLI_PATTERN.match(s.upper().replace(" ", ""))
    if m:
        rate = _cli_rate(m.group("rate"))
        return PortName(
            raw=s,
            style="cli_board",
            rate_mbps=rate,
            chassis=int(m.group("chassis")),
            slot=int(m.group("slot")),
            port=int(m.group("port")),
        )

    if IPMI_PATTERN.match(s):
        idx = IPMI_PATTERN.match(s).group(1)  # type: ignore[union-attr]
        return PortName(
            raw=s,
            style="label",
            rate_mbps=1_000,
            label="IPMI",
            index=int(idx) if idx else 1,
            is_mgmt=True,
        )

    m = LABEL_PATTERN.match(s.upper().replace(" ", ""))
    if m:
        label = m.group("label")
        return PortName(
            raw=s,
            style="label",
            rate_mbps=LABEL_RATE_MAP.get(label),
            label=label,
            index=int(m.group("idx")),
        )

    # `GE8` 这类「速率前缀 + 纯序号」的写法（实测只出现在负载均衡上）
    m = BARE_PATTERN.match(s.upper().replace(" ", ""))
    if m:
        label = m.group("label")
        return PortName(
            raw=s,
            style="label",
            rate_mbps=_cli_rate(label),
            label=label,
            index=int(m.group("idx")),
        )

    return PortName(raw=s, style="unknown")


def _cli_rate(tag: str) -> int | None:
    return {
        "GE": 1_000,
        "10GE": 10_000,
        "25GE": 25_000,
        "40GE": 40_000,
        "100GE": 100_000,
        "400GE": 400_000,
    }.get(tag)


def make_cli_port(rate_tag: str, chassis: int, slot: int, port: int) -> str:
    """生成交换机侧端口名。

    >>> make_cli_port("100GE", 1, 0, 1)
    '100GE1/0/1'
    """
    return f"{rate_tag}{chassis}/{slot}/{port}"


def make_label_port(label: str, index: int) -> str:
    """生成服务器侧端口名。

    >>> make_label_port("千兆", 1)
    '千兆1'
    """
    return f"{label}{index}"


#: 速率 → 默认板卡前缀（推荐端口名时用）
RATE_TAG: Final[dict[int, str]] = {
    1_000: "GE",
    10_000: "10GE",
    25_000: "25GE",
    40_000: "40GE",
    100_000: "100GE",
}


def sort_key_port(raw: str) -> tuple[int, int, int, int, str]:
    """端口排序键：先按样式、再按速率、再按序号。

    用于生成「下一个可用端口」时保证确定性顺序（实测端口连号率 81%，
    332 组中 269 组完全连号，所以顺序分配就是正确策略）。
    """
    p = parse_port_name(raw)
    if p.style == "cli_board":
        return (0, p.chassis or 0, p.slot or 0, p.port or 0, raw)
    if p.style == "label":
        return (1, p.rate_mbps or 0, p.index or 0, 0, raw)
    return (2, 0, 0, 0, raw)
