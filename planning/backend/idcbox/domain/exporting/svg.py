"""把机柜 U 位图渲染成可打印的 SVG（FR-6.7「2D SVG 施工图导出」）。

## 为什么在域层，而且是纯函数

导出的 SVG 是要**贴到机柜门上**的东西。现场人员拿手机扫、监理拿 A3 纸核，
所以它必须和界面里看到的 U 位图**逐格一致**——一旦两份代码各画一遍，
早晚出现「屏幕上 40U 是交换机、纸上 40U 是服务器」这种事。

所以这里只做一件事：把已经算好的数据摆成图形，不做任何业务判断。
配色、字号、页边距都是参数，业务侧传进来，这里不查配置、不读文件。

## 为什么坚持 SVG 而不是 PNG

- 打印：A3 上 U 号是 3mm 高的小字，位图放大就糊；SVG 是矢量的，300dpi 打印清晰
- 可改：现场发现柜号印错，用浏览器/Illustrator 直接改文字，不用回系统重新导
- 体积：200 个机柜的整层图 PNG 要十几 MB，SVG 只有几百 KB
- 零依赖：浏览器原生渲染，不需要装任何东西（对比 PDF 要 jsPDF、图片要 canvas）

## 打印的第一原则：白底黑字

界面是深色主题，但**深色 SVG 打印出来是一整片黑**（打印机把背景色当实色喷墨，
一张 A3 全黑要几十分钟还伤纸）。所以这里硬编码白底，不跟随界面主题。
设备用淡彩色块区分类型，但文字一律深色——彩色文字在黑白打印时会变成灰糊。
"""

from __future__ import annotations

import html
from dataclasses import dataclass, field
from typing import Literal, Sequence

#: 一个机柜的 U 位高度。源数据全部是 42U 标准柜（V13 的 362 个柜位无一例外）。
DEFAULT_RACK_U = 42

#: 单 U 的默认像素高度。18px 时 42U 机柜高 756px，A3 横向（约 1587×1123）能放 8 个柜。
DEFAULT_UNIT_H = 18

Color = str

#: 设备分类 → 填充色。淡色，保证浅色打印后还能看清黑字。
#: 分类依据是型号关键字，规则在 `categorize()` 里，与前端 `deviceColor()` 同口径。
CATEGORY_FILLS: dict[str, Color] = {
    "core": "#cfe2f3",       # 核心交换机 —— 淡蓝
    "switch": "#d9ead3",     # 接入交换机 —— 淡绿
    "server": "#fff2cc",     # 计算/存储节点 —— 淡黄
    "storage": "#fce5cd",    # 存储 —— 淡橙
    "security": "#f4cccc",   # 安全设备 —— 淡红
    "other": "#e6e6e6",      # 其他 —— 淡灰
}

CATEGORY_LABELS: dict[str, str] = {
    "core": "核心交换机",
    "switch": "接入交换机",
    "server": "服务器 / 计算节点",
    "storage": "存储设备",
    "security": "安全设备",
    "other": "其他",
}


def categorize(model: str) -> str:
    """按型号名归类。**判定顺序是硬约束**，与前端 `RackView.deviceColor()` 逐条对齐。

    两条容易踩的：
    - 「核心」必须在「交换机」之前：`业务核心交换机` 同时含两个字，
      顺序反了就会被画成接入交换机的颜色，现场分不清哪台是核心。
    - 安全设备必须在「服务器」之前：`服务器密码机`、`签名服务器` 名字里都有
      「服务器」，但它们是要标红的安全设备，不是计算节点。
    """
    if "核心" in model:
        return "core"
    if "接入" in model or "交换机" in model:
        return "switch"
    for kw in ("防火墙", "负载", "密码", "签名", "加密", "密评"):
        if kw in model:
            return "security"
    if "存储" in model:
        return "storage"
    if "服务器" in model or "节点" in model:
        return "server"
    return "other"


@dataclass(frozen=True, slots=True)
class SvgDevice:
    """一个占 U 位的设备。只带渲染需要的字段。"""

    model: str
    display_name: str
    start_u: int
    end_u: int
    port_count: int = 0

    @property
    def size_u(self) -> int:
        return self.end_u - self.start_u + 1


@dataclass(frozen=True, slots=True)
class SvgRack:
    """一个机柜。`devices` 允许重叠 —— 真实数据里就有 11 对 U 位重叠
    （规划占位），重叠的块都要画出来并标红，不能只是叠着盖住。"""

    channel_no: int
    seq_no: int
    label: str
    devices: tuple[SvgDevice, ...] = ()
    #: 本柜进出线数（作为本端 + 作为对端）。
    #: 放在机柜上而不是设备上：一个柜多台设备时线数摊不到某一台上。
    cable_count: int = 0

    @property
    def used_u(self) -> int:
        return sum(d.size_u for d in self.devices)


@dataclass(frozen=True, slots=True)
class SvgOptions:
    unit_h: int = DEFAULT_UNIT_H
    rack_width: int = 220
    rack_gap: int = 28
    pad: int = 24
    title: str = "机柜 U 位图"
    subtitle: str = ""
    footer: str = ""
    show_ports: bool = True
    max_racks_per_sheet: int = 8
    #: 每页机柜数超过这个值时自动分页（多个 `<svg>` 拼接成一份可打印 HTML 更实用，
    #: 但这里保持单文件 SVG，分页由调用方决定）。
    rack_u: int = DEFAULT_RACK_U


@dataclass(frozen=True, slots=True)
class RackSheet:
    """一页：若干机柜 + 标题。"""

    racks: tuple[SvgRack, ...]
    options: SvgOptions = field(default_factory=SvgOptions)

    @property
    def sheets(self) -> list[tuple[SvgRack, ...]]:
        """按 `max_racks_per_sheet` 切页。"""
        n = max(1, self.options.max_racks_per_sheet)
        return [self.racks[i : i + n] for i in range(0, len(self.racks), n)]


def _esc(text: str) -> str:
    """SVG 里的文本必须转义。设备名里有 `（ARM）` 这种全角括号不算危险，
    但真实数据里也有 `A&B`、`<测试>` 这类 —— 一个未转义的 `<` 会让整个文件
    变成 XML 解析错误，浏览器直接白屏，且报错信息不会告诉你是哪台设备。
    """
    return html.escape(text, quote=True)


def _device_color(device: SvgDevice) -> Color:
    return CATEGORY_FILLS.get(categorize(device.model), CATEGORY_FILLS["other"])


def render_racks(sheet: RackSheet) -> str:
    """渲染一页或多页机柜 U 位图，返回完整 SVG 文本。

    多页时返回**多个并排的 `<svg>` 元素**？不 —— 返回一个 SVG，
    页与页之间横向拉开、中间画一条裁剪虚线。理由：一份施工图经常要整体缩放
    到 A3 一页，多个 `<svg>` 元素在打印时会各自分页导致列被切断。
    """
    o = sheet.options
    pages = sheet.sheets
    if not pages:
        pages = [()]

    per_page = max(1, o.max_racks_per_sheet)
    page_w = per_page * (o.rack_width + o.rack_gap)
    # 每行机柜（一"页"）的实际高度：机柜体 + 柜名两行 + 行间距
    row_h = o.rack_u * o.unit_h + 60
    # 多行必须**纵向堆叠**。第一版把所有页都画在同一个 top 上，
    # 结果 13 行机柜完全重叠成一张 —— 导出的 SVG 看起来只有一个柜。
    body_rows = len(pages)
    height = o.pad * 2 + 120 + row_h * body_rows

    parts: list[str] = []
    parts.append(
        f'<svg xmlns="http://www.w3.org/2000/svg" '
        f'width="{page_w + o.pad * 2}" height="{height}" '
        f'viewBox="0 0 {page_w + o.pad * 2} {height}" '
        f'font-family="Noto Sans CJK SC, Microsoft YaHei, sans-serif">'
    )
    # 白底：见模块 docstring「打印的第一原则」
    parts.append(f'<rect width="100%" height="100%" fill="#ffffff"/>')

    # ── 标题区 ──
    parts.append(
        f'<text x="{o.pad}" y="{o.pad + 26}" font-size="20" font-weight="700" '
        f'fill="#111111">{_esc(o.title)}</text>'
    )
    if o.subtitle:
        parts.append(
            f'<text x="{o.pad}" y="{o.pad + 48}" font-size="12" fill="#555555">'
            f"{_esc(o.subtitle)}</text>"
        )

    # 图例：右上角，说明颜色块的业务含义
    legend_y = o.pad + 8
    lx = o.pad + page_w - 300
    for i, (key, label) in enumerate(CATEGORY_LABELS.items()):
        col = i % 3
        row = i // 3
        x = lx + col * 100
        y = legend_y + row * 20
        parts.append(
            f'<rect x="{x}" y="{y}" width="14" height="12" '
            f'fill="{CATEGORY_FILLS[key]}" stroke="#999999" stroke-width="0.6"/>'
        )
        parts.append(
            f'<text x="{x + 19}" y="{y + 10}" font-size="10" fill="#333333">'
            f"{_esc(label)}</text>"
        )

    top = o.pad + 80
    body_h = o.rack_u * o.unit_h

    for page_index, rack_group in enumerate(pages):
        row_top = top + page_index * row_h
        if page_index > 0:
            # 行首标注：告诉现场这是同一张图里的下一行，不是另一个房间
            parts.append(
                f'<text x="{o.pad}" y="{row_top - 8}" font-size="10" fill="#999999">'
                f"（第 {page_index + 1} 行，接上）</text>"
            )
        for rack_index, rack in enumerate(rack_group):
            x = o.pad + rack_index * (o.rack_width + o.rack_gap)
            parts.extend(_render_one_rack(rack, x, row_top, o))

    # ── 页脚 ──
    if o.footer:
        parts.append(
            f'<text x="{o.pad}" y="{height - 16}" font-size="10" fill="#777777">'
            f"{_esc(o.footer)}</text>"
        )

    parts.append("</svg>")
    return "".join(parts)


def _render_one_rack(rack: SvgRack, x: int, top: int, o: SvgOptions) -> list[str]:
    """画一个机柜：U 刻度 + U 网格 + 设备块 + 柜名 + 用量统计。"""
    out: list[str] = []
    label_w = 26
    body_x = x + label_w
    body_y = top
    body_w = o.rack_width - label_w
    unit_h = o.unit_h
    body_h = o.rack_u * unit_h

    # U 网格：与界面一致，用 pattern 而不是 42 条 line。
    # 42 条 line × 200 柜 = 8400 个节点，浏览器打开会卡；pattern 后是 1 个。
    pid = f"ugrid-{rack.channel_no}-{rack.seq_no}"
    out.append(
        f'<defs><pattern id="{pid}" width="{body_w}" height="{unit_h}" '
        f'patternUnits="userSpaceOnUse">'
        f'<line x1="0" y1="{unit_h}" x2="{body_w}" y2="{unit_h}" '
        f'stroke="#dddddd" stroke-width="0.5"/></pattern></defs>'
    )
    out.append(
        f'<rect x="{body_x}" y="{body_y}" width="{body_w}" height="{body_h}" '
        f'fill="#fbfbfb" stroke="#666666" stroke-width="1"/>'
    )
    out.append(
        f'<rect x="{body_x}" y="{body_y}" width="{body_w}" height="{body_h}" '
        f'fill="url(#{pid})"/>'
    )

    # U 号：从 42 往下到 1，柜顶在上（与实物一致，现场是照着柜子装）
    for u in range(o.rack_u, 0, -1):
        y = body_y + (o.rack_u - u) * unit_h
        out.append(
            f'<text x="{body_x - 5}" y="{y + unit_h - 5}" font-size="9" '
            f'text-anchor="end" fill="#666666">{u}</text>'
        )

    # 设备块
    overlap_rows: dict[int, int] = {}
    for d in rack.devices:
        start = max(1, min(d.start_u, o.rack_u))
        end = max(1, min(d.end_u, o.rack_u))
        if end < start:
            continue
        # 统计重叠：同一 U 行被多台设备占，这一行要标红
        for u in range(start, end + 1):
            overlap_rows[u] = overlap_rows.get(u, 0) + 1

        y = body_y + (o.rack_u - end) * unit_h
        h = (end - start + 1) * unit_h
        fill = _device_color(d)

        out.append(
            f'<rect x="{body_x + 2}" y="{y + 1}" width="{body_w - 4}" '
            f'height="{h - 2}" fill="{fill}" stroke="#888888" stroke-width="0.7" rx="2"/>'
        )
        # 型号名（左）+ 实例名（右）分两行，U 位矮时只画型号
        name = d.display_name or d.model
        if h >= unit_h * 2:
            out.append(
                f'<text x="{body_x + 6}" y="{y + 13}" font-size="9" fill="#111111">'
                f"{_esc(_clip(d.model, 16))}</text>"
            )
            out.append(
                f'<text x="{body_x + 6}" y="{y + 25}" font-size="8" fill="#444444">'
                f"{_esc(_clip(name, 22))}</text>"
            )
            meta = f"{start}-{end}U"
            if o.show_ports and d.port_count:
                meta += f" · {d.port_count}口"
            out.append(
                f'<text x="{body_x + body_w - 6}" y="{y + h - 5}" font-size="7" '
                f'text-anchor="end" fill="#666666">{_esc(meta)}</text>'
            )
        else:
            out.append(
                f'<text x="{body_x + 6}" y="{y + h - 5}" font-size="8" fill="#111111">'
                f"{_esc(_clip(d.model, 20))}</text>"
            )

    # 重叠标红：真实数据里 11 对 U 位重叠，图纸必须显式标出来。
    # 只标红边不填充，避免盖住原本的设备色。
    for u, count in overlap_rows.items():
        if count < 2:
            continue
        y = body_y + (o.rack_u - u) * unit_h
        out.append(
            f'<rect x="{body_x + 1}" y="{y}" width="{body_w - 2}" height="{unit_h}" '
            f'fill="none" stroke="#cc0000" stroke-width="1.4" stroke-dasharray="3,2"/>'
        )

    # 柜名
    out.append(
        f'<text x="{body_x + body_w / 2}" y="{body_y + body_h + 20}" font-size="12" '
        f'text-anchor="middle" fill="#111111">{_esc(rack.label)}</text>'
    )
    # 用量：这个柜装了多少 U / 台数，现场核对用
    summary = f"{len(rack.devices)} 台 · {rack.used_u}/{o.rack_u}U"
    if rack.cable_count:
        summary += f" · {rack.cable_count} 线"
    out.append(
        f'<text x="{body_x + body_w / 2}" y="{body_y + body_h + 36}" font-size="9" '
        f'text-anchor="middle" fill="#666666">{_esc(summary)}</text>'
    )
    return out


def _clip(text: str, limit: int) -> str:
    """超长设备名截断。不截断的话文字会溢出到隔壁机柜，
    现场看到的名字属于别的柜，比看不到更糟。"""
    return text if len(text) <= limit else text[: limit - 1] + "…"


def render_to_html(svg: str, *, title: str) -> str:
    """包一层 HTML，方便打印时自动横向 A3。

    ## 为什么还要 HTML 包装

    单独一个 `.svg` 文件在浏览器里打开是居中的、没有页边距、
    @page 规则不生效 —— 打印出来会带浏览器的页眉页脚、边缘被裁。
    包一层 HTML 设好 `@page { size: A3 landscape; margin: 6mm }`，
    Ctrl+P 直接就是能交付的图纸。

    这是纯字符串拼接，不引入模板引擎：一个 HTML 骨架不值得多一个依赖。
    """
    return (
        "<!doctype html><html><head><meta charset=\"utf-8\">"
        f"<title>{_esc(title)}</title>"
        "<style>"
        "@page { size: A3 landscape; margin: 6mm; }"
        "html,body { margin:0; padding:0; background:#fff; }"
        "svg { display:block; width:100%; height:auto; }"
        "@media print { body { -webkit-print-color-adjust: exact; print-color-adjust: exact; } }"
        "</style></head><body>"
        f"{svg}"
        "</body></html>"
    )


__all__ = [
    "CATEGORY_FILLS",
    "CATEGORY_LABELS",
    "DEFAULT_RACK_U",
    "DEFAULT_UNIT_H",
    "RackSheet",
    "SvgDevice",
    "SvgOptions",
    "SvgRack",
    "categorize",
    "render_racks",
    "render_to_html",
]
