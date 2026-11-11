"""施工图绘制服务：把项目数据摆成可打印的机柜图。

## 为什么单独一个模块，不塞进 views.py

`views.py` 的职责是「把域数据切成前端要的 JSON 形状」。而这里的产物是
**一份要打印、要贴到机柜门上的图纸**——它有版式、有页边距、有图例。
两者的失败方式完全不同：JSON 形状错了前端报错、能定位；图纸错了是
**现场按错的图施工**，几周后才发现。

所以绘制单独成模块，并且明确禁止它做业务判断：
数据来自 `views.rack_devices`（唯一来源），这里只负责摆位置。

## 三种产出

- `render_rack_sheet()` —— 单柜或多柜的 U 位图（A3 横向）
- `render_floor_sheet()` —— 整层平面图（柜位 + 用量热力）
- 都要能单独下载成 `.svg` / `.html`（HTML 版带 `@page` 规则，Ctrl+P 即出图）

## 一个刻意的取舍：图上不画跨柜连线

连线表已经是一份完整交付物，把 5127 条线画到平面图上会变成一团墨。
现场真正需要的是**这个柜里有什么、几 U、几个口、几根线**，
跨柜怎么走线看布线表。所以图上只标「本柜进出线数」，不画线的走向。
"""

from __future__ import annotations

from typing import Iterable, Sequence

from ...domain.exporting.svg import RackSheet, SvgDevice, SvgOptions, SvgRack, render_racks
from .views import DEFAULT_RACK_HEIGHT_U, ProjectViews

#: 一页放几个柜。8 个柜 × 220px 宽在 A3 横向（1587px @96dpi）刚好占满，
#: 再多就要缩放到看不清 U 号。
RACKS_PER_SHEET = 8


def _cable_counts(views: ProjectViews) -> dict[tuple[int, int], int]:
    """每个机柜的进出线数。O(连线数) 扫一遍，不在内层再扫。

    5127 条连线 × 362 个柜如果写成双层循环，就是 185 万次比较；
    后端接口会从 20ms 变成 300ms，而这个接口是导出按钮直接调的。
    """
    counts: dict[tuple[int, int], int] = {}
    for link in views.project.links:
        for ep in (link.a, link.b):
            key = (ep.rack.channel_no, ep.rack.seq_no)
            counts[key] = counts.get(key, 0) + 1
    return counts


def build_rack_sheet(
    views: ProjectViews,
    racks: Sequence[tuple[int, int]] | None = None,
    *,
    title: str = "机柜 U 位图",
    subtitle: str = "",
    footer: str = "",
) -> RackSheet:
    """构造一份图纸的数据。

    `racks=None` 表示「所有有设备的机柜」。注意不是全部 362 个柜位 ——
    空柜印在图上只是浪费纸，现场也不会去看。
    """
    counts = _cable_counts(views)
    keys: Iterable[tuple[int, int]]
    if racks is None:
        keys = sorted(k for k, devices in views.rack_devices.items() if devices)
    else:
        keys = [(int(ch), int(seq)) for ch, seq in racks]

    built: list[SvgRack] = []
    for ch, seq in keys:
        devices = views.rack_devices.get((ch, seq), ())
        svg_devices = tuple(
            SvgDevice(
                model=d.model_raw or d.display_name,
                display_name=d.display_name,
                start_u=d.span[0],
                end_u=d.span[1],
                port_count=len(d.ports),
            )
            for d in devices
        )
        built.append(
            SvgRack(
                channel_no=ch,
                seq_no=seq,
                label=f"通道{ch} / 机柜{seq}",
                devices=svg_devices,
                cable_count=counts.get((ch, seq), 0),
            )
        )

    used = sum(r.used_u for r in built)
    options = SvgOptions(
        title=title,
        subtitle=subtitle
        or f"共 {len(built)} 个机柜 · {sum(len(r.devices) for r in built)} 台设备 · "
        f"占用 {used}U",
        footer=footer,
        max_racks_per_sheet=RACKS_PER_SHEET,
        rack_u=DEFAULT_RACK_HEIGHT_U,
    )
    return RackSheet(racks=tuple(built), options=options)


def render_rack_sheet(views: ProjectViews, **kwargs: object) -> str:
    """直接出 SVG 文本。参数透传给 `build_rack_sheet`。"""
    racks = kwargs.pop("racks", None)
    title = str(kwargs.pop("title", "机柜 U 位图"))
    subtitle = str(kwargs.pop("subtitle", ""))
    footer = str(kwargs.pop("footer", ""))
    return render_racks(
        build_rack_sheet(
            views,
            racks,  # type: ignore[arg-type]
            title=title,
            subtitle=subtitle,
            footer=footer,
        )
    )


__all__ = ["RACKS_PER_SHEET", "build_rack_sheet", "render_rack_sheet"]
