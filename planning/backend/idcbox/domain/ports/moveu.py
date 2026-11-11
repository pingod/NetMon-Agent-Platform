"""改设备 U 位 —— U 位图拖拽编辑的纯数据部分。

## 为什么这件事值得单独一个模块

拖拽看起来是纯前端交互，但「拖完之后什么变了」是有业务含义的：

- 设备的 U 位是**意图**里的字段（`SwitchSpec.u` / `DeviceSpec.u`），不是布线表的
- 改 U 位**不影响线长**：同机柜两端 `path_units = 0`，落到经验表/公式链都是 0，
  所以跳线长度档位不变。这一点实测过（`path_units(a, a) == 0`）。

  > ⚠️ **这句话曾被误读成「所以拖动是安全的」（v0.12 纠正）**。
  > 它只说明**线长**不变，而端口号会变：`端口号 = 组内位次 + base`，
  > 位次按 `(机柜, U)` 升序 —— 换柜或换槽位就换位次，**同组排在后面的
  > 设备端口号整体前移**。实测跨柜挪一台 = **91 行端口号变了、6 行线长变了**，
  > 而且连「同柜内挪」都可能改（3U→39U 改了 56 行）。
  > 所以每个移动入口都必须返回影响面（见 `gen_diff.py` 与
  > `application/services/impact.py`），不能按「同柜 = 安全」跳过。

- 改 U 位**会影响四件事**：占位冲突、U 位越界、该设备是否是「布局段生成的」、
  以及**同组其它设备的端口号**（v0.12 补上的第四个）

## 三类设备有三条写入路径（实测 827 台可拖 / 7 台拒绝）

| 来源 | 台数 | 改什么 | 目标位置约束 |
|---|---|---|---|
| `switches` 段（逐台声明） | 130 | 该行 `u`（换柜时还改 `rack`） | 本通道内真实存在的柜 |
| `devices` 段（逐台声明） | 34 | 同上 | 同上 |
| 布局段 `RackBlock` 自动排布 | 663 | 往 `devices` 段写**覆盖记录** | 必须是本通道某个布局段覆盖的柜 |
| 无对应声明 | 7 | ❌ 拒绝 | 源数据自身矛盾（同一实例在两个 U 位） |

### 布局段那 663 台为什么不能「直接把 U 写回布局段」

布局段是「连续几个机柜全放同型号」的压缩写法，28 条声明覆盖 663 台。
它的位置由槽位序列**算**出来（`SLOT_BASE_U + slot * SLOT_PITCH_U`），
没有「第几台放第几 U」这样的字段存在 —— 所以改一台就得往 `devices`
段追加一条以**槽位 id**（`N02-01-S0`）为 `instance` 的覆盖记录。

`node_id` 必须保持不变：换掉它就不再是「交换机组里第 N 台」，
端口分配链里「第几个节点接第几个口」整体错位，那个机柜的 44 根线
会全部接到错误的端口上。所以跨柜/换 U 都只改位置，不改身份。

### 跨机柜的两条规则，别混用

布局段设备写覆盖记录，而覆盖只在布局段覆盖的柜上被 `expand_placements`
认出来 —— 写进一个没有排布的柜，这台设备就**在库里但排不出来**，
布线表里它的线全消失，而且不报错。逐台声明的设备自己带 `rack` 字段，
只要目标柜真实存在即可。拿布局段的规则去卡后者，就会得出
「ch1 那段布局都没有，所以核心交换机一台都换不了柜」—— 而 ch1 的机柜
明明真实存在。详见 `_cross_rack_problem` 的 `needs_layout` 参数。

## 为什么 `move_device` 返回新的 doc 而不是原地改

与 `intentcheck.apply_cell` 一致：**失败不留半改**。前端拿到 `(ok, error, doc)`，
`ok=False` 时继续用旧 doc，界面不会出现「拖到一半」的中间态。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Mapping, Sequence

from .intent import expand_placements, intent_from_dict
from .intentcheck import coerce_cell
from ..importing.normalize import norm_name

#: 机柜 U 高度。U 位合法范围 1..42。
RACK_U_HEIGHT = 42


@dataclass(frozen=True, slots=True)
class MoveResult:
    """一次移动的结果。"""

    ok: bool
    section: str | None
    row_no: int | None
    error: str | None
    #: 改完的完整文档（`ok=False` 时原样返回）
    doc: dict[str, Any]
    #: 这次移动造成的新重叠（用于前端高亮）
    overlaps: tuple[str, ...] = ()
    #: 这台设备占几 U。前端要靠它把方块画成正确高度 ——
    #: 布局段设备在台账里只有型号，取不到 U 高度。
    u_span: int = 1
    #: 移到了哪个机柜（跨机柜拖动时与调用方传入的不同）。
    #: 前端靠它判断「要不要重新拉一遍两个机柜」—— 只改 U 位时同一张图刷新即可。
    rack_no: int | None = None
    #: 搬之前在哪（`target_rack_no is None` 时与 `rack_no` 相同）。
    #: 批量拖动要靠它把「这一批自己」从占用清单里排除 —— 否则 8 台整体下移
    #: 会互相撞，一台都动不了。
    from_rack_no: int | None = None
    from_u: int | None = None


def find_device_row(
    doc: Mapping[str, Any], channel_no: int, rack_no: int, instance: str
) -> tuple[str, int] | None:
    """在意图里定位一台设备：返回 `(段名, 行号)`，找不到返回 `None`。

    ## 为什么用 (通道, 机柜, 实例名) 三键

    实例名在同一通道内唯一，但**同名设备可能出现在别的通道**（`-1` 后缀的
    编号规则跨通道重复）。只按名字查会把 ch2 的 `计算接入交换机(业务)-1`
    拖到 ch8 去。

    ## 为什么要同时查 switches 和 devices

    用户从界面上看两者没有区别（都是机柜里一个方块）。让他在拖之前
    先想「这台算交换机还是特殊设备」是荒谬的。
    """
    target = norm_name(instance)
    for section in ("switches", "devices"):
        rows = doc.get(section)
        if not isinstance(rows, list):
            continue
        for idx, row in enumerate(rows):
            if not isinstance(row, dict):
                continue
            if row.get("channel") != channel_no or row.get("rack") != rack_no:
                continue
            if norm_name(str(row.get("instance") or "")) == target:
                return section, idx
    return None


def is_layout_generated(
    doc: Mapping[str, Any], channel_no: int, rack_no: int, model: str
) -> bool:
    """这台设备是否由布局段自动排布（因而没有单独的 U 位字段可改）。"""
    blocks = doc.get("racks")
    if not isinstance(blocks, list):
        return False
    target = norm_name(model)
    for block in blocks:
        if not isinstance(block, dict):
            continue
        if block.get("channel") != channel_no:
            continue
        lo, hi = block.get("from"), block.get("to")
        if not isinstance(lo, int) or not isinstance(hi, int):
            continue
        if not (lo <= rack_no <= hi):
            continue
        if norm_name(str(block.get("model") or "")) == target:
            return True
    return False


def occupied_spans(
    doc: Mapping[str, Any], *, channel_no: int, rack_no: int, skip: tuple[str, int] | None = None
) -> list[tuple[int, int, str]]:
    """列出该机柜已被占用的 U 区间 `(起, 止, 名字)`。

    `skip` 用于排除「正在移动的这台自己」—— 否则它的旧位置会和自己冲突。
    """
    out: list[tuple[int, int, str]] = []
    for section in ("switches", "devices"):
        rows = doc.get(section)
        if not isinstance(rows, list):
            continue
        for idx, row in enumerate(rows):
            if not isinstance(row, dict):
                continue
            if skip is not None and skip == (section, idx):
                continue
            if row.get("channel") != channel_no or row.get("rack") != rack_no:
                continue
            u = row.get("u")
            if not isinstance(u, int):
                continue
            span = row.get("uSpan")
            size = span if isinstance(span, int) and span >= 1 else 1
            out.append((u, u + size - 1, str(row.get("instance") or row.get("model") or "")))
    out.extend(_layout_spans(doc, channel_no=channel_no, rack_no=rack_no))
    return out


def _layout_spans(
    doc: Mapping[str, Any], *, channel_no: int, rack_no: int
) -> list[tuple[int, int, str]]:
    """布局段在**这个机柜**里排出的所有 U 区间。

    ## 不查它会让重叠悄悄发生

    `switches`/`devices` 两个段只记「逐台声明」的设备。663 台由布局段
    自动排布的设备不在这两个段里 —— 它们的位置要按槽位序列现算
    （v0.8 起还叠加 `devices` 段里的覆盖记录，所以这里直接用
    `expand_placements` 而不是套原始槽位公式，否则刚拖走的那台
    会被当成还占着旧位置）。
    如果 `occupied_spans` 只看那两段，用户把一台交换机拖到「计算节点A 的
    U3-4」上时不会被拦，生成后两台设备真的叠在一起，
    而这正是 `device_conflict` 里 U_OVERLAP 要防的事。

    ## 为什么把 overrides 也算进来

    「拆出来」的设备（`devices` 段里 instance 是槽位 id 的）会覆盖原位置，
    此时布局段那一槽已经不在它的旧 U 上了 —— 用 `expand_placements` 的
    结果而不是原始槽位公式，才不会把「刚拆走的那台」当成还占着旧位置。
    """
    try:
        intent = intent_from_dict(dict(doc))
        placements = expand_placements(intent)
    except (AttributeError, KeyError, TypeError, ValueError):
        # doc 处于半成品状态（编辑器正在改）时不阻断拖拽，只少一层保护。
        # `AttributeError` 也要吞：`racks` 里某块写成字符串时，
        # `_rack_block_from_dict` 会在 `d.get("slotMap")` 上炸
        # （`'str' object has no attribute 'get'`）—— 那是数据没填完，
        # 不是用户操作错，不该让整个拖拽面板挂掉。
        return []
    out: list[tuple[int, int, str]] = []
    for pl in placements:
        if pl.channel_no != channel_no or pl.rack_no != rack_no:
            continue
        out.append((pl.u_start, pl.u_start + 1, pl.model))
    return out


def _overlaps(
    start: int, end: int, spans: list[tuple[int, int, str]]
) -> tuple[str, ...]:
    hits = [f"{name}（{lo}-{hi}U）" for lo, hi, name in spans if not (end < lo or start > hi)]
    return tuple(hits)


def move_device(
    doc: Mapping[str, Any],
    *,
    channel_no: int,
    rack_no: int,
    instance: str,
    new_u: int,
    allow_overlap: bool = False,
    current_u: int | None = None,
    target_rack_no: int | None = None,
) -> MoveResult:
    """把一台设备挪到 `new_u`；给 `target_rack_no` 就是换机柜。

    ## 三种失败要比「成功」说得更清楚

    1. **设备不在意图里** → 明确说「这台是由布局段 `通道N/机柜X-Y/型号` 自动
       排布的，U 位由槽位序列决定；要改得改布局段」。只回「不支持」用户
       会以为程序坏了。
    2. **越界** → 说清合法范围 `1..42`，并给出它现在的 U 位。
    3. **重叠** → 默认**拒绝**（`allow_overlap=False`）。

    ## 为什么默认拒绝重叠

    ## 跨机柜拖动（`target_rack_no`）

    只能在同一通道内换机柜。理由不是偷懒：端口分配链按**通道**分组
    （`ChainSpec.channel_no`），一台设备换了通道就等于换了一组 ToR 来服务它，
    那是重新规划而不是拖动 —— 硬允许会得到一条「在这个通道里但没人服务」的
    孤立设备，布线表里它的线全消失。目标机柜还必须在某个布局段里，
    否则它在这份规划里根本不存在。

    ## `current_u` 为什么存在

    布局段排布的设备（663 台，占 79.5%）**没有实例名**：台账里它们的
    `display_name` 就是型号（如「计算节点A（ARM）」），同一机柜 8 台全一样。
    前端只能靠「型号 + 现在在第几 U」指认是哪一台，所以要把当前 U 位传进来。
    逐台声明过的设备（交换机/特殊机型）有唯一实例名，不传也行。

    源数据里本来就有 11 对重叠（规划占位），所以**不能把重叠当错误**。
    但用户主动拖一台进来造成新重叠，和「历史遗留」是两件事：
    前者说明他算错了，该当场拦住。要给用户留后门（`allow_overlap=True`），
    因为「先占位、后调」也是真实动作。
    """
    if not 1 <= new_u <= RACK_U_HEIGHT:
        return MoveResult(
            ok=False,
            section=None,
            row_no=None,
            error=f"U 位必须落在 1~{RACK_U_HEIGHT}，收到 {new_u}",
            doc=dict(doc),
        )

    located = find_device_row(doc, channel_no, rack_no, instance)

    # 布局段设备：先在 devices 段找一条「钉住它的」覆盖记录；
    # 一条都没有就现场拆一条出来（这就是「拆成独立设备」的动作）。
    if located is None:
        return _move_layout_device(
            doc,
            channel_no=channel_no,
            rack_no=rack_no,
            instance=instance,
            new_u=new_u,
            allow_overlap=allow_overlap,
            current_u=current_u,
            target_rack_no=target_rack_no,
        )

    dest_rack = rack_no if target_rack_no is None else target_rack_no
    if dest_rack != rack_no:
        # 走这条路径的是**逐台声明**的设备（`switches` / `devices` 里有一行），
        # 它自带 `rack` 字段，换柜不依赖任何布局段 —— 只要目标柜真实存在。
        bad = _cross_rack_problem(
            doc, channel_no, rack_no, dest_rack, needs_layout=False
        )
        if bad is not None:
            return MoveResult(
                ok=False, section=None, row_no=None, error=bad, doc=dict(doc)
            )

    section, row_no = located
    rows = doc.get(section)
    assert isinstance(rows, list)
    row = rows[row_no]
    assert isinstance(row, dict)

    size = row.get("uSpan")
    span = size if isinstance(size, int) and size >= 1 else 1
    end_u = new_u + span - 1
    if end_u > RACK_U_HEIGHT:
        return MoveResult(
            ok=False,
            section=section,
            row_no=row_no,
            error=(
                f"这台设备高 {span}U，放到 {new_u}U 会顶到 {end_u}U，超出机柜 {RACK_U_HEIGHT}U"
            ),
            doc=dict(doc),
        )

    spans = occupied_spans(
        doc, channel_no=channel_no, rack_no=dest_rack, skip=(section, row_no)
    )
    hits = _overlaps(new_u, end_u, spans)
    if hits and not allow_overlap:
        return MoveResult(
            ok=False,
            section=section,
            row_no=row_no,
            error=f"U{new_u}-{end_u} 已被占用：{('、'.join(hits))}",
            doc=dict(doc),
            overlaps=hits,
        )

    # 深拷贝到该段那一行再改 —— 其余引用保持不变，调用方可以按需 diff
    new_doc: dict[str, Any] = dict(doc)
    new_rows = list(rows)
    new_row = dict(row)
    new_row["u"] = new_u
    new_row["rack"] = dest_rack
    new_rows[row_no] = new_row
    new_doc[section] = new_rows
    return MoveResult(
        ok=True,
        section=section,
        row_no=row_no,
        error=None,
        doc=new_doc,
        overlaps=hits,
        u_span=span,
        rack_no=dest_rack,
    )


def _move_layout_device(
    doc: Mapping[str, Any],
    *,
    channel_no: int,
    rack_no: int,
    instance: str,
    new_u: int,
    allow_overlap: bool,
    current_u: int | None = None,
    target_rack_no: int | None = None,
) -> MoveResult:
    """拖一台「由布局段自动排布」的设备：写一条覆盖记录到 `devices` 段。

    ## 为什么是「写一条 devices」而不是「改布局段」

    布局段是「连续几个机柜全放同型号」的压缩写法，663 台设备共用 28 条声明。
    为了挪一台去改布局段，只能把整段拆成逐台（用户要重填几十行）。
    而 `devices` 段本来就是逐台声明，`instance` 允许写槽位 id ——
    加一条 `{"instance": "N02-01-S0", "model": ..., "channel":2, "rack":1, "u":7}`
    就表示「这一台的位置不用布局段算」。`expand_placements` 会认它。

    ## 槽位 id 从哪来

    用户在 U 位图上看到的是「型号 + U 位」，不是槽位 id。所以按
    **(通道, 机柜, 型号, 当前 U 位)** 反查 `expand_placements` 的结果 ——
    四者合起来在真实意图里唯一（同一机柜同型号 8 台，但 U 位各不相同）。

    ## 拖到别的槽位等于「换了物理位置」

    不尝试保持端口分配不变：设备换了 U 位，按相位轮转规则它本来就该接到
    另一个端口。这是真实世界的语义（机器挪了位置，线要重接），
    硬保留旧端口号会让布线表和现场对不上。
    """
    model = _guess_model(doc, channel_no, rack_no, instance)
    dest_rack = rack_no if target_rack_no is None else target_rack_no
    if dest_rack != rack_no:
        bad = _cross_rack_problem(doc, channel_no, rack_no, dest_rack)
        if bad is not None:
            return MoveResult(
                ok=False, section=None, row_no=None, error=bad, doc=dict(doc)
            )
    block = _layout_block_of(doc, channel_no, rack_no, model) if model else None
    if block is None:
        return MoveResult(
            ok=False,
            section=None,
            row_no=None,
            error=(
                f"意图里找不到「{instance}」（通道{channel_no} 机柜{rack_no}）。"
                "它可能是源数据里位置自相矛盾的设备，需要先在意图里补一条声明。"
            ),
            doc=dict(doc),
        )

    # ★ 名字白名单：布局段设备的 `instance` 只可能是两种东西
    #   —— 型号（台账 displayName）或槽位 id（`N02-01-S0`）。
    #   都不是，就说明这台设备压根不存在，**必须当场拒绝**。
    #
    # ## 少了这一步会静默挪错一台真设备（v0.12 修）
    #
    # `_slot_id_at` 在 instance 精确匹配不上时会退回「按型号 + U 位找第一台」。
    # 而 `_guess_model` 只看机柜属于哪个布局块 —— 通道2 机柜1 在
    # 「计算节点A（ARM）」那块里，于是随便传个名字都会拿到 `model=计算节点A`，
    # 再按 U 位 3 命中 `N02-01-S0`，最后**成功改掉了那台真设备的位置**，
    # 返回 `ok=True`。
    #
    # v0.11 已经在批量路径（`_identity`）加过同一道守卫，但单台路径漏了 ——
    # 于是「拖不存在的设备」在单台接口上成功、在批量接口上被拒。
    # 两个入口对同一件事给出不同答案，比两个都错更难发现。
    if current_u is not None:
        by_slot_id = instance.startswith("N")
        if not by_slot_id and norm_name(instance) != norm_name(model):
            return MoveResult(
                ok=False,
                section=None,
                row_no=None,
                error=(
                    f"意图里找不到「{instance}」（通道{channel_no} 机柜{rack_no} "
                    f"{current_u}U）。这个位置上是「{model}」"
                    " —— 若它由布局段自动排布，请按型号或槽位 id 指认。"
                ),
                doc=dict(doc),
            )

    if current_u is None:
        # 布局段设备在台账里没有实例名 —— 同机柜 8 台的显示名完全相同。
        # 不给当前位置就只能随机猜一台，而用户看到的「已移到 N U」提示
        # 完全正确，只是动的是别的设备。宁可不做。
        return MoveResult(
            ok=False,
            section=None,
            row_no=None,
            error=(
                f"『{instance}』由布局段自动排布，同机柜可能有多台同名设备，"
                "需要提供它现在在第几 U 才能确定改哪一台。"
            ),
            doc=dict(doc),
        )

    # 反查槽位 id：型号 + 当前 U 位
    slot_id, u_span = _slot_id_at(
        doc, channel_no, rack_no, model, instance, current_u=current_u
    )
    if slot_id is None:
        return MoveResult(
            ok=False,
            section=None,
            row_no=None,
            error=(
                f"在布局段『通道{block['channel']} 机柜{block['from']}-{block['to']} "
                f"{block['model']}』里，{current_u}U 上没有「{instance}」。"
                "可能是 U 位与槽位序列对不上，请检查布局段的 slots / slotMap。"
            ),
            doc=dict(doc),
        )

    end_u = new_u + u_span - 1
    if end_u > RACK_U_HEIGHT:
        return MoveResult(
            ok=False,
            section=None,
            row_no=None,
            error=(
                f"这台设备高 {u_span}U，放到 {new_u}U 会顶到 {end_u}U，"
                f"超出机柜 {RACK_U_HEIGHT}U"
            ),
            doc=dict(doc),
        )

    spans = occupied_spans(doc, channel_no=channel_no, rack_no=dest_rack)
    # 自己原来的位置要排除：`occupied_spans` 会把布局段里它自己算进去
    home_u = _current_u(doc, slot_id, channel_no, rack_no)
    if dest_rack == rack_no:
        # 同柜内挪：旧位置那一条是它自己，必须排掉，否则永远撞自己
        spans = [sp for sp in spans if sp[0] != home_u]
    else:
        # 跨柜：目的柜里若已有一条指向同一槽位 id 的旧覆盖（拖过去又拖回来），
        # 那也是它自己，同样排掉。
        spans = [
            sp
            for sp in spans
            if not (sp[0] == home_u and norm_name(sp[2]) == norm_name(model))
        ]
    hits = _overlaps(new_u, end_u, spans)
    if hits and not allow_overlap:
        return MoveResult(
            ok=False, section=None, row_no=None,
            error=f"U{new_u}-{end_u} 已被占用：{('、'.join(hits))}",
            doc=dict(doc), overlaps=hits,
        )

    new_doc: dict[str, Any] = dict(doc)
    rows = new_doc.get("devices")
    new_rows = list(rows) if isinstance(rows, list) else []
    # 已有覆盖记录就改它（用户第二次拖同一台）
    for idx, row in enumerate(new_rows):
        if isinstance(row, dict) and row.get("instance") == slot_id:
            upd = dict(row)
            upd["u"] = new_u
            upd["rack"] = dest_rack
            new_rows[idx] = upd
            new_doc["devices"] = new_rows
            return MoveResult(
                ok=True, section="devices", row_no=idx, error=None,
                doc=new_doc, overlaps=hits, u_span=u_span, rack_no=dest_rack,
            )
    new_rows.append(
        {
            "instance": slot_id,
            "model": model,
            "channel": channel_no,
            "rack": dest_rack,
            "u": new_u,
            **({"uSpan": u_span} if u_span != 2 else {}),
        }
    )
    new_doc["devices"] = new_rows
    return MoveResult(
        ok=True, section="devices", row_no=len(new_rows) - 1, error=None,
        doc=new_doc, overlaps=hits, u_span=u_span, rack_no=dest_rack,
    )


@dataclass(frozen=True, slots=True)
class DeviceTarget:
    """批量拖动里的一个成员。"""

    channel_no: int
    rack_no: int
    instance: str
    #: 布局段设备必须给（同柜 8 台同名）；逐台声明的可不给。
    current_u: int | None = None


@dataclass(frozen=True, slots=True)
class BatchMoveResult:
    """一次批量移动的结果。语义与 `MoveResult` 一致：失败不留半改。"""

    ok: bool
    error: str | None
    doc: dict[str, Any]
    #: `(实例名, 原机柜, 原U, 新机柜, 新U)` —— 前端据此画「拖进来」的方块
    moved: tuple[tuple[str, int, int, int, int], ...] = ()
    #: 整批共同的新重叠（`allow_overlap=True` 时才有内容）
    overlaps: tuple[str, ...] = ()


def _identity(
    doc: Mapping[str, Any], t: DeviceTarget
) -> tuple[str, str, int, int] | None:
    """解析一个成员的「身份 + 当前 U 位 + 高度」。

    返回 `(种类, 键, 当前U, 高度)`：
    - 逐台声明 → `("declared", "switches|devices#行号", u, span)`
    - 布局段排布 → `("layout", "N02-01-S0", u, span)`
    - 找不到 → `None`

    ## 为什么要单独解析一次

    批量拖动必须先知道「这一批原本占哪些位置」，才能把它们从占用清单里
    整体摘掉再校验。逐个调单台函数做不到这件事：第 8 台校验时，前 7 台
    还站在旧位置（或已被单独挪走但剩下 6 台还在），两边都算不对。
    """
    located = find_device_row(doc, t.channel_no, t.rack_no, t.instance)
    if located is not None:
        section, row_no = located
        rows = doc.get(section)
        if not isinstance(rows, list):
            return None
        row = rows[row_no]
        if not isinstance(row, dict):
            return None
        u = row.get("u")
        if not isinstance(u, int):
            return None
        span = row.get("uSpan")
        size = span if isinstance(span, int) and span >= 1 else 1
        return ("declared", f"{section}#{row_no}", u, size)

    if t.current_u is None:
        return None
    model = _guess_model(doc, t.channel_no, t.rack_no, t.instance)
    if not model:
        return None
    # ★ 布局段设备的 `instance` 只可能是两种东西：型号（台账 displayName）
    # 或槽位 id（`N02-01-S0`）。都不是就说明这个名字压根不存在。
    #
    # 少了这一步会出一个很坏的错：`_slot_id_at` 在 instance 匹配不上时会
    # 退回「按型号 + U 位找第一台」，于是「不存在的设备」被当成 U3 那台
    # 真设备，批量拖动会**真的挪动一台用户没选中的机器**。
    by_id = t.instance.startswith("N")
    if not by_id and norm_name(t.instance) != norm_name(model):
        return None
    slot_id, span = _slot_id_at(
        doc, t.channel_no, t.rack_no, model, t.instance, current_u=t.current_u
    )
    if slot_id is None:
        return None
    return ("layout", slot_id, t.current_u, span)


def _spans_excluding(
    doc: Mapping[str, Any],
    *,
    channel_no: int,
    rack_no: int,
    moving_declared: set[str],
    moving_layout: set[str],
) -> list[tuple[int, int, str]]:
    """该机柜的占用区间，**排除正在搬的这一批自己**。

    ## 为什么不能复用 `occupied_spans(..., skip=...)`

    那个 `skip` 只能排一台（`(段名, 行号)` 单值）。批量搬 8 台时，
    剩下 7 台仍站在旧位置 —— 整体下移会看到「新位置被同伴占着」，
    于是每台都失败，而用户明明只是把一摞机器往下挪。

    这里按身份（段名+行号 / 槽位 id）逐台排除，比按 U 区间排除更准：
    真实数据里本来就有 11 对重叠，按 U 排会把不相干的设备一起摘掉。
    """
    out: list[tuple[int, int, str]] = []
    for section in ("switches", "devices"):
        rows = doc.get(section)
        if not isinstance(rows, list):
            continue
        for idx, row in enumerate(rows):
            if not isinstance(row, dict):
                continue
            if f"{section}#{idx}" in moving_declared:
                continue
            if row.get("channel") != channel_no or row.get("rack") != rack_no:
                continue
            u = row.get("u")
            if not isinstance(u, int):
                continue
            span = row.get("uSpan")
            size = span if isinstance(span, int) and span >= 1 else 1
            out.append((u, u + size - 1, str(row.get("instance") or row.get("model") or "")))
    try:
        intent = intent_from_dict(dict(doc))
        placements = expand_placements(intent)
    except (AttributeError, KeyError, TypeError, ValueError):
        placements = []
    for pl in placements:
        if pl.channel_no != channel_no or pl.rack_no != rack_no:
            continue
        if pl.node_id in moving_layout:
            continue
        out.append((pl.u_start, pl.u_start + 1, pl.model))
    return out


def move_devices(
    doc: Mapping[str, Any],
    *,
    targets: Sequence[DeviceTarget],
    delta_u: int,
    target_rack_no: int | None = None,
    allow_overlap: bool = False,
) -> BatchMoveResult:
    """把一批设备**整体**平移 `delta_u` 个 U（可选同时换机柜）。

    ## 为什么是「同一个 delta」而不是「各自给新 U」

    现场动作是「这一摞往下挪 8 格」，不是「第 1 台到 3U、第 2 台到 11U」。
    给每台单独指定新 U，用户要手算保持相对间距，算错就是设备叠一起。
    delta 天然保住间距，也让「整批能否放下」变成一个可判定的问题。

    ## 校验顺序（每一步都要在写之前）

    1. 批量非空、`delta_u != 0`
    2. 所有成员都能定位（逐台声明或布局段槽位）
    3. 越界：任意一台 `new_u < 1` 或 `new_u + span - 1 > 42`
    4. 重叠：把这一批从占用清单里整体摘掉后，再逐台查（**包括这一批内部
       互相撞** —— 真实数据里同一位置可能已有两台叠放，整体平移后仍会叠）
    5. 跨柜：整批去同一个目标柜，用单台那套规则校验一次

    ## 与单台的关系

    校验通过后**逐台调用 `move_device(..., allow_overlap=True)`**，因为
    布局段设备的覆盖记录写法（找已有记录改还是追加）只有那一份实现。
    此时 `allow_overlap=True` 是安全的：重叠已经在第 4 步判过。
    """
    if not targets:
        return BatchMoveResult(ok=False, error="没有选中任何设备", doc=dict(doc))
    if delta_u == 0:
        return BatchMoveResult(ok=False, error="移动量为 0，位置没有变化", doc=dict(doc))

    channels = {t.channel_no for t in targets}
    if len(channels) > 1:
        return BatchMoveResult(
            ok=False,
            error=(
                f"这批设备分布在 {len(channels)} 个通道（{sorted(channels)}）里，"
                "不能一起移动 —— 通道之间要换到不同的 ToR，是重新规划。"
            ),
            doc=dict(doc),
        )
    channel_no = channels.pop()
    source_racks = {t.rack_no for t in targets}
    if target_rack_no is None:
        if len(source_racks) > 1:
            return BatchMoveResult(
                ok=False,
                error=(
                    f"这批设备来自 {len(source_racks)} 个机柜（{sorted(source_racks)}），"
                    "要一起移动就得给一个共同的目标机柜。"
                ),
                doc=dict(doc),
            )
        dest_rack = next(iter(source_racks))
    else:
        dest_rack = target_rack_no

    resolved: list[tuple[DeviceTarget, tuple[str, str, int, int]]] = []
    for t in targets:
        ident = _identity(doc, t)
        if ident is None:
            return BatchMoveResult(
                ok=False,
                error=(
                    f"意图里找不到「{t.instance}」（通道{t.channel_no} 机柜{t.rack_no}）。"
                    "它可能是源数据里位置自相矛盾的设备，需要先在意图里补一条声明。"
                ),
                doc=dict(doc),
            )
        resolved.append((t, ident))

    # 越界
    for t, (kind, key, u, span) in resolved:
        new_u = u + delta_u
        if new_u < 1 or new_u + span - 1 > RACK_U_HEIGHT:
            return BatchMoveResult(
                ok=False,
                error=(
                    f"『{t.instance}』现在 {u}U 高 {span}U，平移 {delta_u:+d} 后到 "
                    f"{new_u}U 会{'低于 1U' if new_u < 1 else f'顶到 {new_u + span - 1}U'}，"
                    f"超出机柜 1~{RACK_U_HEIGHT}U 的范围。"
                ),
                doc=dict(doc),
            )

    # 跨柜：整批去同一个目标柜
    if dest_rack not in source_racks:
        # 只要有一台是布局段排布的，目标柜就必须在布局段内（覆盖记录靠槽位 id
        # 被 expand_placements 认出来）。逐台声明的没这个限制 —— 见
        # `_cross_rack_problem` 的 `needs_layout`。
        needs_layout = any(kind == "layout" for _t, (kind, _k, _u, _s) in resolved)
        bad = _cross_rack_problem(
            doc, channel_no, next(iter(source_racks)), dest_rack,
            needs_layout=needs_layout,
        )
        if bad is not None:
            return BatchMoveResult(ok=False, error=bad, doc=dict(doc))

    # 占用：把这一批整体摘掉
    moving_declared = {key for _t, (kind, key, _u, _s) in resolved if kind == "declared"}
    moving_layout = {key for _t, (kind, key, _u, _s) in resolved if kind == "layout"}
    static_spans = _spans_excluding(
        doc,
        channel_no=channel_no,
        rack_no=dest_rack,
        moving_declared=moving_declared,
        moving_layout=moving_layout,
    )

    all_hits: list[str] = []
    for t, (_kind, _key, u, span) in resolved:
        new_u = u + delta_u
        end_u = new_u + span - 1
        hits = _overlaps(new_u, end_u, static_spans)
        if hits:
            all_hits.append(f"『{t.instance}』→ {new_u}-{end_u}U 被占用：" + "、".join(hits))
    if all_hits and not allow_overlap:
        return BatchMoveResult(
            ok=False,
            error="这一批放不下：" + "；".join(all_hits[:3])
            + (f"（另有 {len(all_hits) - 3} 处）" if len(all_hits) > 3 else ""),
            doc=dict(doc),
        )

    # 逐台落到新 doc 上。`allow_overlap=True` —— 重叠已在上面判过。
    cur = dict(doc)
    moved: list[tuple[str, int, int, int, int]] = []
    for t, (_kind, _key, u, span) in resolved:
        res = move_device(
            cur,
            channel_no=t.channel_no,
            rack_no=t.rack_no,
            instance=t.instance,
            new_u=u + delta_u,
            allow_overlap=True,
            current_u=t.current_u if t.current_u is not None else u,
            target_rack_no=dest_rack if dest_rack != t.rack_no else None,
        )
        if not res.ok:
            return BatchMoveResult(
                ok=False,
                error=f"『{t.instance}』移动失败：{res.error}",
                doc=dict(doc),
            )
        cur = res.doc
        moved.append((t.instance, t.rack_no, u, res.rack_no or t.rack_no, u + delta_u))

    return BatchMoveResult(
        ok=True, error=None, doc=cur, moved=tuple(moved), overlaps=tuple(all_hits)
    )


def _known_racks(doc: Mapping[str, Any], channel_no: int) -> set[int]:
    """这个通道里「有存在感」的机柜号：布局段覆盖的 + 已被声明过的。

    ## 为什么不能只看布局段

    ch1 是网络/安全机柜区，**一段布局都没有**（全是逐台声明的核心交换机）。
    只按布局段判断的话会得到「通道1 里没有机柜5」—— 可机柜 5 明明装着
    分线接入交换机和国密 SSL-VPN。把「没排布声明」当成「柜不存在」，
    用户想挪一台核心交换机就完全没法操作。
    """
    out: set[int] = set()
    blocks = doc.get("racks")
    if isinstance(blocks, list):
        for block in blocks:
            if not isinstance(block, dict) or block.get("channel") != channel_no:
                continue
            lo, hi = block.get("from"), block.get("to")
            if isinstance(lo, int) and isinstance(hi, int):
                out.update(range(lo, hi + 1))
    for section in ("switches", "devices"):
        rows = doc.get(section)
        if not isinstance(rows, list):
            continue
        for row in rows:
            if not isinstance(row, dict):
                continue
            rk, inst = row.get("rack"), row.get("instance")
            if not isinstance(rk, int):
                continue
            # 逐台声明：直接按 channel 匹配。
            if row.get("channel") == channel_no:
                out.add(rk)
            # 布局段的覆盖记录：`channel` 可能已被改成新柜所在通道，
            # `instance` 仍是槽位 id（`N02-01-S0`，前两位是原通道）——
            # 两种都要认，否则刚拖过去的柜会被判成不存在、连拖回来都做不到。
            elif isinstance(inst, str) and inst.startswith(f"N{channel_no:02d}-"):
                out.add(rk)
    return out


def _cross_rack_problem(
    doc: Mapping[str, Any],
    channel_no: int,
    from_rack: int,
    to_rack: int,
    *,
    needs_layout: bool = True,
) -> str | None:
    """跨机柜拖动的合法性检查。合法返回 `None`，否则返回一句能照做的话。

    ## 两类设备两条规则（本函数最容易搞错的地方）

    - **布局段排布的设备**（`needs_layout=True`）：目标柜必须落在本通道
      某个布局段里。因为「挪到哪」是靠往 `devices` 段写覆盖记录、再由
      `expand_placements` 按槽位 id 认出来的，而覆盖只在布局段覆盖的柜上
      生效 —— 写进一个没有排布的柜，这台设备就**在库里但排不出来**，
      布线表里它的线全消失，而且不报错。
    - **逐台声明的设备**（`needs_layout=False`）：目标柜只要真实存在即可。
      它自己带 `rack` 字段，改完就是这行的新位置，不依赖任何布局段。
      拿布局段去卡它，等于「核心交换机一台都换不了柜」（ch1 就是这种情况）。

    两条规则都要求同通道：换通道等于换一组 ToR 来服务它，那是重新规划。
    """
    if to_rack < 1:
        return f"机柜号必须从 1 开始，收到 {to_rack}"
    blocks = doc.get("racks")
    if needs_layout and not isinstance(blocks, list):
        return "意图里没有布局段，无法判断目标机柜是否存在"

    if not needs_layout:
        if to_rack in _known_racks(doc, channel_no):
            return None
        return (
            f"通道{channel_no} 里没有机柜{to_rack}。"
            "跨机柜只能拖到本通道已有设备的柜里 —— 换通道等于换一组 ToR "
            "来服务它，那是重新规划。"
        )

    assert isinstance(blocks, list)
    for block in blocks:
        if not isinstance(block, dict) or block.get("channel") != channel_no:
            continue
        lo, hi = block.get("from"), block.get("to")
        if isinstance(lo, int) and isinstance(hi, int) and lo <= to_rack <= hi:
            # 目标柜在某个布局段里就放行。刻意不校验「那段放的型号是否相同」：
            # 机柜里混装不同型号是常态（一台 ToR + 8 台异构服务器），
            # 卡型号会把正常操作拦掉，而真正的后果（槽位序列不同）
            # 由 U 位重叠检查兜住。
            return None
    covered = sorted(
        r
        for block in blocks
        if isinstance(block, dict) and block.get("channel") == channel_no
        for r in range(
            int(block.get("from", 0)), int(block.get("to", 0)) + 1
        )
    )
    span = f"{covered[0]}~{covered[-1]}" if covered else "无"
    return (
        f"通道{channel_no} 里没有可排布的机柜{to_rack}（布局段覆盖 {span}）。"
        "布局段排布的设备只能拖到本通道已有的布局段内 —— 写进没有排布的柜，"
        "它会在库里但排不出来。请在意图里补一段布局声明。"
    )


def _slot_id_at(
    doc: Mapping[str, Any],
    channel_no: int,
    rack_no: int,
    model: str,
    instance: str,
    *,
    current_u: int | None = None,
) -> tuple[str | None, int]:
    """按「型号 + 当前 U 位」反查布局段的槽位 id 与该设备的 U 高度。

    `instance` 参数用来消歧：U 位图上同一格可能叠了两台（source 数据如此），
    此时 `instance` 里的 `-k` 后缀能区分（`N02-11-S7` vs `N02-11-S7-1`）。
    取不到精确匹配时退回「该 U 位上第一台同型号」。
    """
    try:
        placements = expand_placements(intent_from_dict(dict(doc)))
    except (KeyError, TypeError, ValueError):
        return None, 1
    target_u = None
    for pl in placements:
        if pl.channel_no == channel_no and pl.rack_no == rack_no and pl.node_id == instance:
            target_u = pl.u_start
            break
    if target_u is None:
        # 布局段设备在台账里没有实例名（`instance` 传进来是型号或显示名），
        # 只能靠「型号 + 当前 U 位」定位 —— 同一机柜同型号 8 台，
        # 但每台的 U 位不同，所以这一个坐标就够唯一。
        if current_u is None:
            return None, 1
        for pl in placements:
            if (
                pl.channel_no == channel_no
                and pl.rack_no == rack_no
                and norm_name(pl.model) == norm_name(model)
                and pl.u_start == current_u
            ):
                target_u = pl.u_start
                break
    if target_u is None:
        return None, 1
    best: str | None = None
    for pl in placements:
        if (
            pl.channel_no == channel_no
            and pl.rack_no == rack_no
            and norm_name(pl.model) == norm_name(model)
            and pl.u_start == target_u
        ):
            best = pl.node_id
            break
    return best, 2


def _current_u(doc: Mapping[str, Any], slot_id: str, channel_no: int, rack_no: int) -> int:
    """这台（可能是覆盖记录）当前实际所在的 U 位。"""
    rows = doc.get("devices")
    if isinstance(rows, list):
        for row in rows:
            if isinstance(row, dict) and row.get("instance") == slot_id:
                u = row.get("u")
                if isinstance(u, int):
                    return u
    try:
        for pl in expand_placements(intent_from_dict(dict(doc))):
            if pl.node_id == slot_id:
                return pl.u_start
    except (KeyError, TypeError, ValueError):
        pass
    return -1


def _guess_model(
    doc: Mapping[str, Any], channel_no: int, rack_no: int, instance: str
) -> str:
    """从布局段里猜这台设备的型号 —— 拿不到就返回空串。

    没有别的地方能给出「这台到底是什么型号」：意图的布局段只写型号不写实例名，
    实例名是生成时按机柜顺序编出来的（`N02-01-S05`）。所以只能按机柜反查。
    """
    blocks = doc.get("racks")
    if not isinstance(blocks, list):
        return ""
    for block in blocks:
        if not isinstance(block, dict):
            continue
        lo, hi = block.get("from"), block.get("to")
        if (
            block.get("channel") == channel_no
            and isinstance(lo, int)
            and isinstance(hi, int)
            and lo <= rack_no <= hi
        ):
            return str(block.get("model") or "")
    return ""


def _layout_block_of(
    doc: Mapping[str, Any], channel_no: int, rack_no: int, model: str
) -> dict[str, Any] | None:
    blocks = doc.get("racks")
    if not isinstance(blocks, list):
        return None
    target = norm_name(model)
    for block in blocks:
        if not isinstance(block, dict):
            continue
        lo, hi = block.get("from"), block.get("to")
        if (
            block.get("channel") == channel_no
            and isinstance(lo, int)
            and isinstance(hi, int)
            and lo <= rack_no <= hi
            and norm_name(str(block.get("model") or "")) == target
        ):
            return block
    return None


def editable_devices(doc: Mapping[str, Any]) -> dict[tuple[int, int, str], str]:
    """只列出「逐台声明」的设备 → `{(通道, 机柜, 实例名): 段名}`。

    ## 这不是「哪些设备能拖」的答案，别拿它当前端清单

    v0.8 之后布局段排布的 663 台也能拖（写覆盖记录），它们**不在**这个
    字典里。要「能拖的全集」用 `editable_items()` —— 它返回带 `uStart`
    与 `section` 的完整清单，前端按 `editableKey(通道,机柜,实例,U位)`
    四元组比对（同柜 8 台同名计算节点必须靠 U 位区分）。

    ## 还留着做什么

    回答「有多少台是逐台声明的」—— 跨柜约束按来源分两条规则，统计口径
    需要它。真实项目实测 130（switches）+ 34（devices）= 164。
    """
    out: dict[tuple[int, int, str], str] = {}
    for section in ("switches", "devices"):
        rows = doc.get(section)
        if not isinstance(rows, list):
            continue
        for row in rows:
            if not isinstance(row, dict):
                continue
            ch, rk, inst = row.get("channel"), row.get("rack"), row.get("instance")
            if isinstance(ch, int) and isinstance(rk, int) and isinstance(inst, str) and inst:
                out[(ch, rk, inst)] = section
    return out


def editable_items(doc: Mapping[str, Any]) -> list[dict[str, Any]]:
    """列出所有「可以在 U 位图上拖」的设备，带上它们现在占的 U 位。

    ## 为什么不能只返回名字

    布局段排布的 663 台设备**没有实例名**：同一机柜里 8 台服务器的
    显示名都是型号（「计算节点A（ARM）」）。前端按名字找会 8 台一起动。

    所以每项都带 `uStart`，前端用 `(通道, 机柜, 显示名, 起始U)` 四元组认设备。
    逐台声明过的设备实例名本来就唯一，四元组里的 U 位只是冗余但无害 ——
    统一成一种键，前端不用分两套逻辑。

    ## `section` 的第三个取值 `layout`

    前两个取值 `switches` / `devices` 是意图里真实存在的段（改 U 位就是改那行）。
    `layout` 表示「这台的 U 位由布局段算出来，拖动会往 `devices` 段写一条覆盖记录」。
    """
    out: dict[tuple[int, int, str, int], dict[str, Any]] = {}

    # 先把布局段的槽位 id 集合算出来：`devices` 段里 `instance` 是槽位 id 的
    # 那些行是「布局段设备的覆盖记录」，不是独立的特殊机型。
    try:
        placements = expand_placements(intent_from_dict(dict(doc)))
    except (AttributeError, KeyError, TypeError, ValueError):
        placements = ()
    slot_ids = {pl.node_id for pl in placements}

    for section in ("switches", "devices"):
        rows = doc.get(section)
        if not isinstance(rows, list):
            continue
        for row in rows:
            if not isinstance(row, dict):
                continue
            ch, rk, inst, u = (
                row.get("channel"),
                row.get("rack"),
                row.get("instance"),
                row.get("u"),
            )
            if not (isinstance(ch, int) and isinstance(rk, int) and isinstance(inst, str) and inst):
                continue
            if not isinstance(u, int):
                continue
            # ★ 覆盖记录要按「型号 + U 位」暴露，不能按槽位 id。
            # 前端认设备用的是台账的 `displayName`（布局段设备就是型号），
            # 拿 `N02-01-S0` 去比永远匹配不上 —— 表现为「拖过一次之后这台
            # 就拖不动了」，而且不报错。
            if section == "devices" and inst in slot_ids:
                model = str(row.get("model") or "")
                if model:
                    out[(ch, rk, model, u)] = {
                        "channel": ch,
                        "rack": rk,
                        "instance": model,
                        "uStart": u,
                        "section": "layout",
                    }
                    continue
            out[(ch, rk, inst, u)] = {
                "channel": ch,
                "rack": rk,
                "instance": inst,
                "uStart": u,
                "section": section,
            }

    # 布局段的设备：统一记成 section="layout"，instance 用型号
    # （与台账 `display_name` 一致 —— 那 663 台没有实例名）
    declared = {(ch, rk, inst) for (ch, rk, inst, _u) in out}
    for pl in placements:
        if (pl.channel_no, pl.rack_no, pl.node_id) in declared:
            continue
        key = (pl.channel_no, pl.rack_no, pl.model, pl.u_start)
        if key in out:
            continue
        out[key] = {
            "channel": pl.channel_no,
            "rack": pl.rack_no,
            "instance": pl.model,
            "uStart": pl.u_start,
            "section": "layout",
        }

    return [out[k] for k in sorted(out)]


__all__ = [
    "RACK_U_HEIGHT",
    "BatchMoveResult",
    "DeviceTarget",
    "MoveResult",
    "editable_devices",
    "editable_items",
    "find_device_row",
    "is_layout_generated",
    "move_device",
    "move_devices",
    "occupied_spans",
]
