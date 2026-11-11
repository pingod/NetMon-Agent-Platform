"""域层单元测试公共装置。

## 与 golden 测试的分工

- **golden**：锁住「真实数据的数字对不对」（5113 行 / 627/641 / 3850/3850）
- **unit**：锁住「边界与契约对不对」（超档位、几何缺失、纯函数性、无穷举）

unit 测试**不读 Excel**，全部用手工构造的小对象。好处是毫秒级、可在任何环境跑、
且失败信息指向性极强（golden 失败时往往只能看出"少了一行"）。
"""

from __future__ import annotations

import pytest


@pytest.fixture()
def geo():
    """最小几何：403 房间 ch1 两个柜（正对）、ch2 一个柜，模拟三类空间关系。

    403 ch1 布局（真实数据）：N 侧行 3、S 侧行 4，柜 1 在 col 38、柜 27 在 col 38。
    ch2 的 N 侧行 5。
    """
    from idcbox.domain.geometry.floor import RackPlacement
    from idcbox.domain.geometry.layout import build_snapshot

    racks = [
        RackPlacement("403", 1, 1, "N", col=38, row=3),
        RackPlacement("403", 1, 2, "N", col=37, row=3),
        RackPlacement("403", 1, 27, "S", col=38, row=4),
        RackPlacement("403", 2, 1, "N", col=38, row=5),
        RackPlacement("404", 8, 1, "N", col=18, row=3),
    ]
    return build_snapshot(racks, version="test", cmax_override={"403": 38, "404": 18})


@pytest.fixture()
def rule():
    from idcbox.domain.cable_length.rules import DEFAULT_RULE

    return DEFAULT_RULE
