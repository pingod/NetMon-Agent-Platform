"""长度计算规则（可配置、可版本化、可外置）。

架构 §4.8：所有会变的量都必须外置成规则文件，代码里只留**结构性**常量。
这里定义规则的**内存形态**，具体加载由 `infrastructure.config.loader` 负责。

关键设计：`LengthRule` 是不可变值对象，且带 `version_id`。每条算出来的连线都记下
它当时用的是哪个 `version_id` —— 否则「三个月前这个值是怎么来的」永远说不清。
"""

from __future__ import annotations

from dataclasses import dataclass, field, replace
from typing import Literal, Mapping, Sequence

from ..geometry.floor import RackPlacement
from ..units import (
    CABINET_PITCH_M,
    CABLE_SLACK_RATIO,
    DEFAULT_CMAX_BY_ROOM,
    DEFAULT_IN_RACK_LEN_BY_TYPE,
    N_ROW_COEF,
    N_ROW_EXTRA,
)
from .ladder import IFS_COMPAT, LADDER_MODES, Rung, ladder_round

#: 几何位置。架构文档里叫 `RackPos`，实现上复用平面图解析出的同一个结构，
#: 避免出现两个「一个机柜」的类型各自漂移。
RackPos = RackPlacement

OClass = Literal["A", "B", "C"]
LadderMode = Literal["ifs_compat", "continuous"]
CrossAisleMode = Literal["rack_end", "manual_only"]


@dataclass(frozen=True, slots=True)
class LengthRule:
    """线长规则的完整参数集。

    默认值即信息港 V13 的实测口径。换机房时只需换这份配置 + 重新标定几何，
    **不需要改任何公式代码**。
    """

    version_id: str = "FV-1.0.0-baseline"
    #: 机柜中心间距（米）
    rack_spacing_m: float = CABINET_PITCH_M
    #: 线缆余量系数（原公式 120%）
    slack_ratio: float = CABLE_SLACK_RATIO
    #: N = 0 if Δrow==0 else n_row_coef*Δrow + n_row_extra
    n_row_coef: int = N_ROW_COEF
    n_row_extra: int = N_ROW_EXTRA
    #: 机内线缆长度（按线缆类型）
    in_rack_len_by_type: Mapping[str, int] = field(
        default_factory=lambda: dict(DEFAULT_IN_RACK_LEN_BY_TYPE)
    )
    #: 房间 → CMAX（排端走线架入口列）
    cmax_by_room: Mapping[str, int] = field(default_factory=lambda: dict(DEFAULT_CMAX_BY_ROOM))
    #: 阶梯规则表
    rungs: Sequence[Rung] = IFS_COMPAT
    ladder_mode: LadderMode = "ifs_compat"
    #: C 类（跨通道）行为：rack_end = 照公式给值 + 标黄；manual_only = 只给空值强制人工
    o_cross_aisle_mode: CrossAisleMode = "rack_end"
    #: 按 (channel_a, channel_b) 的人工校准补偿，键必须归一成 a<b 的顺序
    cross_aisle_overrides: Mapping[tuple[int, int], int] = field(default_factory=dict)
    #: 未知线缆类型时的兜底 Q 值（不猜类型，但要有值才能算）
    default_unknown_q: float = 5.0

    # ------------------------------------------------------------------
    def with_ladder_mode(self, mode: LadderMode) -> "LengthRule":
        return replace(
            self,
            ladder_mode=mode,
            rungs=LADDER_MODES[mode],
            version_id=f"{self.version_id}@{mode}",
        )

    def cmax(self, room_code: str) -> int | None:
        return self.cmax_by_room.get(room_code)

    def q_of(self, cable_type: str) -> int | float | None:
        return self.in_rack_len_by_type.get(cable_type)

    def override_for(self, channel_a: int, channel_b: int) -> int | None:
        key = (channel_a, channel_b) if channel_a <= channel_b else (channel_b, channel_a)
        return self.cross_aisle_overrides.get(key)

    def t_of(self, s: float) -> int | None:
        return ladder_round(s, self.rungs)


#: 生产默认规则（信息港口径）
DEFAULT_RULE = LengthRule()

#: 体检用的对照规则（补上 (6,7) 缝隙）
CONTINUOUS_RULE = DEFAULT_RULE.with_ladder_mode("continuous")
