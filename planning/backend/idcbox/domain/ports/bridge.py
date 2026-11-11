"""生成结果 → 长度计算的桥。

## 为什么需要这一层

`generate.py` 只产出**物理事实**（谁的口连谁的口、线缆类型），
`cable_length` 只接受 `LinkInput`（能算长度的东西）。两者之间的转换
看似只有字段搬运，但有几处必须做对，否则生成的行算不出长度或算错：

1. **端点角色固定**：生成器把交换机放 `a` 侧、服务器放 `b` 侧。
   但 `n_cross_rack` / `o_gap_rack` 都只依赖两侧机柜坐标，
   与谁是 a/b 无关 —— 这一点必须确认过，否则换向会改变 N/O。
   （已核对：`classify_o` 与 `n_cross_rack` 都只看坐标对，不看顺序。）

2. **U 位原文必须带 `U` 后缀**：长度引擎的 Q 规则只按线缆类型查，
   不看 U 位；但导出时必须还原成 `3-4U` / `40U` 这种原文，
   否则 24 列逐格 diff 会失败。

3. **`source_row_no` 必须是 None**：生成的行是**新行**，没有对应的
   源 Excel 行。若给一个假行号，导出器的 `raw` 视图会以为「源值权威」
   而去查一份不存在的原值，结果写出空值。

4. **`stable_key` 必须唯一且稳定**：同一对端口只该有一条线。
   用 `(本端设备, 本端端口, 对端设备, 对端端口, 线缆类型)` 拼，
   与导入侧 `make_stable_key` 同一口径，这样生成行能和导入行直接比对。

## 分层位置

放在 `ports/` 而不是 `cable_length/`：import-linter 契约规定
`domain.cable_length` **不得**依赖 `domain.ports`（长度是先算出来的基础能力，
端口规划建立在它之上）。反向依赖是允许的，所以桥必须在 `ports/` 这一侧。
"""

from __future__ import annotations

from collections import Counter

from ..cable_length.link import Endpoint, LinkInput
from ..geometry.layout import RackRef
from ..importing.normalize import make_stable_key
from .generate import GeneratedCabling, GeneratedLink


def to_link_input(link: GeneratedLink, *, row_no: int | None = None) -> LinkInput:
    """`GeneratedLink` → `LinkInput`。

    `row_no` 只用于测试与调试排序；生产路径必须留 `None`
    （原因见模块 docstring 第 3 条）。

    >>> from .generate import GeneratedLink
    >>> gl = GeneratedLink(
    ...     a_device="sw-1", a_model="业务接入交换机1", a_channel=2, a_rack=2,
    ...     a_u="40U", a_port="10GE1/0/1",
    ...     b_device="", b_id="N02-01-S0", b_model="计算节点A（ARM）",
    ...     b_channel=2, b_rack=1,
    ...     b_u="3-4U", b_port="万兆1", cable_type="LC-LC", nic="万兆1",
    ... )
    >>> li = to_link_input(gl)
    >>> li.a.device_name, li.a.port_name, li.b.rack.channel_no, li.b.rack.seq_no
    ('sw-1', '10GE1/0/1', 2, 1)
    >>> li.stable_key
    'sw-1|10GE1/0/1|N02-01-S0|万兆1|LC-LC'
    >>> li.source_row_no is None
    True
    """
    a = Endpoint(
        rack=RackRef(channel_no=link.a_channel, seq_no=link.a_rack),
        device_name=link.a_device,
        port_name=link.a_port,
        u_position=link.a_u,
        device_model=link.a_model,
    )
    # 对端（服务器侧）用 `b_id` 而不是 `b_device`：`b_device` 是对外显示的
    # 设备名称，真实数据里服务器行普遍为空；`b_id` 是槽位标识（`N02-11-S7`），
    # 恒定非空，才能让 `stable_key` 唯一、并支持「按设备查线缆」。
    b = Endpoint(
        rack=RackRef(channel_no=link.b_channel, seq_no=link.b_rack),
        device_name=link.b_id or link.b_device or link.b_model,
        port_name=link.b_port,
        u_position=link.b_u,
        device_model=link.b_model,
    )
    return LinkInput(
        # 用 `b.device_name` 而不是 `link.b_device`：前者已回落到 `b_id`，
        # 恒定非空。若这里用空字符串，4430 条的键会塌缩成
        # `交换机|端口|∅|网口|线缆类型` —— 同一交换机同一端口碰巧不会重复，
        # 但「按设备查线缆」和跨项目比对会全部失效（`∅` 不是一个设备）。
        stable_key=make_stable_key(
            link.a_device, link.a_port, b.device_name, link.b_port, link.cable_type
        ),
        a=a,
        b=b,
        cable_type=link.cable_type,
        source_row_no=row_no,
    )


def to_link_inputs(cabling: GeneratedCabling) -> tuple[list[LinkInput], list[str]]:
    """批量转换，返回 `(links, notes)`。

    `notes` 记录**去重**等需要让用户知道的事：同一对端口出现两条线
    通常是意图里重复声明了 `ExplicitLink`，静默去重会让人以为已经接好了。
    """
    out: list[LinkInput] = []
    seen: Counter[str] = Counter()
    for gen in cabling.links:
        li = to_link_input(gen)
        n = seen[li.stable_key]
        seen[li.stable_key] = n + 1
        if n:
            # 加序号保持唯一，与导入侧同口径
            li = _with_key(li, f"{li.stable_key}#{n + 1}")
        out.append(li)

    notes: list[str] = []
    dups = sorted(((k, n) for k, n in seen.items() if n > 1), key=lambda t: -t[1])
    if dups:
        notes.append(
            f"有 {len(dups)} 对端点被声明了多次（例如 {dups[0][0]} 出现 {dups[0][1]} 次），"
            "已按序号区分；请检查意图里的显式连线是否重复"
        )
    return out, notes


def _with_key(link: LinkInput, key: str) -> LinkInput:
    """换 stable_key（`LinkInput` 是 frozen dataclass，只能重建）。"""
    from dataclasses import replace

    return replace(link, stable_key=key)


__all__ = [
    "to_link_input",
    "to_link_inputs",
]
