"""体检规则集：把「源数据缺陷」变成可复现的规则。

## 设计原则

1. **规则只判定，不修复**。修复属于用户决策（FR-D12 的"带病导入"选项）。
2. **每条规则给命中数**。用户第一眼要看的是「我这份表有几个问题」，不是规则说明。
3. **严重度分三级**：`blocker`（导入必失败）/ `warn`（能导但要改）/ `info`（知道就好）。
4. **同一条数据可能命中多条规则**，不去重 —— 但报告按 `rule_code` 分组，用户按组处理。

## 规则编号与需求对应

| 码 | 需求 | 说明 | 实测命中 |
|---|---|---|---|
| D1 | FR-D1 | 总览（非判定规则） | — |
| D2 | FR-D2 | 几何公式一致性（N/O 与源值比对） | N 14 行 / O 52 行 |
| D3 | FR-D3 | U 位区间重叠 | 原样 13 对 / 归一化 11 对 |
| D4 | FR-D4 | 同一实例名占多个 U 位 | 1 处 |
| D5 | FR-D5 | 名称空格变体 / 实例名缺失 | 1 组 / 631 台 |
| D6 | FR-D6 | 端口被多行占用 | 22 个 |
| D7 | FR-D7 | 孤儿机柜（不在平面图） | 0 |
| D8 | FR-D8 | 跨 sheet 一致性（需两份数据，本期仅接口） | — |
| D9 | FR-D9 | 健康度评分 | 计算得出 |
| D10 | FR-D10 | 定位信息（sheet/行/列全带） | 全部规则都带 |
| D11 | FR-D11 | 导出（infra 层） | — |
| D12 | FR-D12 | 非数值单元格（sentinel 塞进数值列） | O 4 / P 4 / T 2 |
| D13 | FR-D13 | 版本差异（需两版数据，本期仅接口） | — |
| R30 | §4.14 | 模块有值但未标批次 | 126 行 / 2 行 |
| F3 | §4.10-9 | 待核定（T 有值 U 空，**正常状态不是错误**） | 9 行 |
| F4 | §4.10-17 | IFS 缝隙行（同一 S 在连续模式下会掉一档） | 49 行 |

## `pending` 与 `sentinel` 的区别（最容易搞错）

- `待定`（T 列）= 有人明确写了"还没定" → 这是**信息**，导出必须原样保留
- U 列空 + T 列有值 = **待核定**（新项目的正常中间态）→ info，不是错误
- `25G-SR` 写在 O 列 = 填错列了 → warn，因为会污染下游计算
"""

from __future__ import annotations

from collections import Counter, defaultdict
from dataclasses import dataclass, field
from typing import Iterable, Literal, Mapping, Sequence

from ..cable_length.ladder import CONTINUOUS, IFS_COMPAT, ladder_round
from ..cable_length.numeric import to_number
from ..geometry.layout import GeometrySnapshot, RackRef
from ..importing.normalize import norm_int, norm_name
from ..shared.types import Issue, IssueCode, Severity
from .instances import (
    DeviceInstance,
    InstanceInventory,
    extract_instances,
    find_same_name_multi_u,
    find_u_conflicts,
)

#: 规则级标题（同一规则下的 finding 可能细分，但汇总用这一条，避免"标题取首条"的歧义）
RULE_TITLES: dict[str, str] = {
    "D2": "跨机柜 N 与平面图不符",
    "D2b": "间隔机柜 O 与平面图不符",
    "D3": "U 位区间重叠",
    "D4": "同一设备被记到多个 U 位",
    "D5": "命名规范",
    "D6": "端口被多行占用",
    "D7": "机柜不在平面图中",
    "D12": "数值列出现文本",
    "R30": "模块有值但未标记批次",
    "R30b": "标了批次却没填模块",
    "F3": "长度已计算未核定",
    "F4": "公式缝隙导致多买一档",
}

#: 规则严重度（体检报告用，比域层 Issue 的三级更贴近用户视角）
RuleSeverity = Literal["blocker", "warn", "info"]


@dataclass(frozen=True, slots=True)
class Finding:
    """一条体检发现。必须能定位到源文件具体位置（FR-D10）。"""

    rule_code: str
    severity: RuleSeverity
    title: str
    detail: str
    #: 源文件定位
    sheet: str = "布线表"
    row_no: int | None = None
    column: str | None = None
    #: 同一规则下的分组键（如 "通道1机柜5"），便于聚合展示
    group: str | None = None
    #: 建议动作
    action: str | None = None

    def as_dict(self) -> dict[str, object]:
        return {
            "rule_code": self.rule_code,
            "severity": self.severity,
            "title": self.title,
            "detail": self.detail,
            "sheet": self.sheet,
            "row_no": self.row_no,
            "column": self.column,
            "group": self.group,
            "action": self.action,
        }


@dataclass(frozen=True, slots=True)
class RuleSummary:
    rule_code: str
    severity: RuleSeverity
    title: str
    count: int
    #: 受影响的行的集合大小（同一行可能命中多条）
    affected_rows: int = 0


@dataclass(frozen=True, slots=True)
class HealthReport:
    """体检报告。`score` 是 0~100 的健康度（FR-D9）。"""

    project_label: str
    total_rows: int
    findings: tuple[Finding, ...]
    summaries: tuple[RuleSummary, ...]
    #: 各维度得分
    score_breakdown: tuple[tuple[str, float, float], ...]  # (维度, 得分, 权重)
    facts: Mapping[str, object] = field(default_factory=dict)

    @property
    def score(self) -> float:
        if not self.score_breakdown:
            return 100.0
        total_w = sum(w for _, _, w in self.score_breakdown)
        if total_w <= 0:
            return 100.0
        return round(sum(s * w for _, s, w in self.score_breakdown) / total_w, 1)

    def by_rule(self, code: str) -> tuple[Finding, ...]:
        return tuple(f for f in self.findings if f.rule_code == code)

    @property
    def blocker_count(self) -> int:
        return sum(1 for f in self.findings if f.severity == "blocker")

    @property
    def warn_count(self) -> int:
        return sum(1 for f in self.findings if f.severity == "warn")

    @property
    def info_count(self) -> int:
        return sum(1 for f in self.findings if f.severity == "info")


@dataclass(frozen=True, slots=True)
class AuditContext:
    """体检所需的全部输入。刻意做成显式结构，避免规则函数签名膨胀。"""

    rows: Sequence[Sequence[object]]
    row_nos: Sequence[int]
    col: Mapping[str, int]
    sheet_name: str = "布线表"
    geometry: GeometrySnapshot | None = None
    project_label: str = "未命名项目"

    def value(self, i: int, key: str) -> object:
        idx = self.col[key]
        v = self.rows[i]
        return v[idx] if idx < len(v) else None

    @property
    def row_count(self) -> int:
        return len(self.rows)


# ============================ D2 几何公式一致性 ============================


@dataclass(frozen=True, slots=True)
class GeometryDeviation:
    row_no: int
    kind: Literal["N", "O"]
    source_value: float
    computed_value: int | None
    channel_a: int
    rack_a: int
    channel_b: int
    rack_b: int
    space_class: str


def check_geometry(
    ctx: AuditContext,
    *,
    n_row_coef: int = 4,
    n_row_extra: int = 4,
) -> tuple[list[GeometryDeviation], Counter[str]]:
    """把每行的 N/O 与几何公式重算值比对。

    只统计「源值有数值」的行 —— 空行不参与（那是"没填"不是"填错"）。
    """
    from ..cable_length.formulas import classify_o, n_cross_rack, o_gap_rack
    from ..cable_length.rules import DEFAULT_RULE

    geo = ctx.geometry
    devs: list[GeometryDeviation] = []
    stats: Counter[str] = Counter()

    if geo is None:
        return devs, stats

    for i in range(ctx.row_count):
        cha, rka = norm_int(ctx.value(i, "ch_local")), norm_int(ctx.value(i, "rack_local"))
        chb, rkb = norm_int(ctx.value(i, "ch_peer")), norm_int(ctx.value(i, "rack_peer"))
        if cha is None or rka is None or chb is None or rkb is None:
            continue
        pos_a = geo.pos(RackRef(channel_no=cha, seq_no=rka))
        pos_b = geo.pos(RackRef(channel_no=chb, seq_no=rkb))
        if pos_a is None or pos_b is None:
            stats["geom_missing"] += 1
            continue

        n_src = to_number(ctx.value(i, "n_cross"))
        if n_src is not None:
            stats["n_comparable"] += 1
            n_calc = n_cross_rack(pos_a, pos_b, DEFAULT_RULE)
            if int(n_src) != n_calc:
                stats["n_bad"] += 1
                devs.append(
                    GeometryDeviation(
                        row_no=ctx.row_nos[i], kind="N", source_value=n_src,
                        computed_value=n_calc, channel_a=cha, rack_a=rka,
                        channel_b=chb, rack_b=rkb, space_class="",
                    )
                )

        o_src = to_number(ctx.value(i, "o_gap"))
        if o_src is not None:
            stats["o_comparable"] += 1
            cls = classify_o(pos_a, pos_b)
            o_calc, _ = o_gap_rack(pos_a, pos_b, DEFAULT_RULE, cls)
            stats[f"o_class_{cls}"] += 1
            if o_calc is not None and int(o_src) != o_calc:
                stats["o_bad"] += 1
                stats[f"o_bad_{cls}"] += 1
                devs.append(
                    GeometryDeviation(
                        row_no=ctx.row_nos[i], kind="O", source_value=o_src,
                        computed_value=o_calc, channel_a=cha, rack_a=rka,
                        channel_b=chb, rack_b=rkb, space_class=cls,
                    )
                )
    return devs, stats


def findings_geometry(devs: Sequence[GeometryDeviation], sheet: str) -> list[Finding]:
    out: list[Finding] = []
    for d in devs:
        kind_cn = "跨机柜(N)" if d.kind == "N" else "间隔机柜(O)"
        col_cn = "跨机柜" if d.kind == "N" else "间隔机柜"
        rule_code = "D2" if d.kind == "N" else "D2b"
        tail = f"（空间关系 {d.space_class} 类）" if d.space_class else ""
        out.append(
            Finding(
                rule_code=rule_code,
                severity="warn",
                title=f"{kind_cn} 与平面图几何不符",
                detail=(
                    f"通道{d.channel_a}机柜{d.rack_a} → 通道{d.channel_b}机柜{d.rack_b}："
                    f"表里填 {d.source_value:g}，按平面图算应为 {d.computed_value}{tail}"
                ),
                sheet=sheet,
                row_no=d.row_no,
                column=col_cn,
                group=f"{d.kind}-{'OK' if d.computed_value == d.source_value else 'DIFF'}",
                action="确认是平面图坐标填错还是本行长度按经验微调；系统以平面图为准",
            )
        )
    return out


# ============================ D12 非数值单元格 ============================

#: 数值列 → 列名
NUMERIC_COLUMNS: tuple[tuple[str, str], ...] = (
    ("n_cross", "跨机柜"),
    ("o_gap", "间隔机柜"),
    ("p_count", "机柜数量"),
    ("q_in_rack", "机内线缆"),
    ("r_inter", "柜间线缆"),
    ("s_total", "线缆2"),
    ("t_ladder", "线缆长度1"),
    ("u_approved", "线缆长度"),
)


def check_non_numeric(ctx: AuditContext) -> list[Finding]:
    """数值列里塞了文本。`待定`/`设备自带` 是 sentinel（info），其余（warn）。"""
    sentinel_ok = {"待定", "设备自带", "一", "无", "-", "—", "/", "N/A", "NA", "n/a"}
    out: list[Finding] = []
    for i in range(ctx.row_count):
        for key, col_cn in NUMERIC_COLUMNS:
            raw = ctx.value(i, key)
            if not isinstance(raw, str) or not raw.strip():
                continue
            s = raw.strip()
            sev: RuleSeverity = "info" if s in sentinel_ok else "warn"
            out.append(
                Finding(
                    rule_code="D12",
                    severity=sev,
                    title="数值列出现文本",
                    detail=f"{col_cn} 列填了「{s}」",
                    sheet=ctx.sheet_name,
                    row_no=ctx.row_nos[i],
                    column=col_cn,
                    group=col_cn,
                    action="sentinel 保留原样即可；若是填错列（如模块型号写进间隔机柜）需修正",
                )
            )
    return out


# ============================ D3/D4/D5 U 位与名称 ============================


def check_instances(ctx: AuditContext) -> tuple[InstanceInventory, list[Finding]]:
    inv = extract_instances(ctx.rows, ctx.row_nos, dict(ctx.col))
    out: list[Finding] = []

    conflicts_norm = find_u_conflicts(inv.normalized)
    conflicts_raw = find_u_conflicts(inv.raw)

    for c in conflicts_norm:
        if c.is_name_variant:
            out.append(
                Finding(
                    rule_code="D5",
                    severity="info",
                    title="同设备名称的空格变体（非真冲突）",
                    detail=(
                        f"{c.label}：「{c.a.model_raw}」与「{c.b.model_raw}」"
                        f"是同一设备的两种写法，U 位同为 {c.a.u_raw}"
                    ),
                    sheet=ctx.sheet_name,
                    row_no=c.a.rows[0] if c.a.rows else None,
                    group=c.label,
                    action="统一命名后此条自动消失",
                )
            )
            continue
        out.append(
            Finding(
                rule_code="D3",
                severity="warn",
                title="U 位区间重叠",
                detail=(
                    f"{c.label}：「{c.a.display_name}」{c.a.u_raw}"
                    f"（{c.a.model_raw}）与「{c.b.display_name}」{c.b.u_raw}"
                    f"（{c.b.model_raw}）占位相交"
                ),
                sheet=ctx.sheet_name,
                row_no=c.a.rows[0] if c.a.rows else None,
                column="U位",
                group=c.label,
                action="到现场确认实际安装位置；U 位冲突会导致实际装不进去",
            )
        )

    # 实例名 → 首行号，供 D4/D5 定位
    name_first_row: dict[tuple, int] = {}
    for inst in inv.normalized:
        for nm in inst.names:
            if nm and inst.rows:
                name_first_row.setdefault((inst.channel_no, inst.rack_no, nm), min(inst.rows))

    for m in find_same_name_multi_u(inv.normalized):
        spans = "、".join(f"{a}-{b}U" if a != b else f"{a}U" for a, b in m.spans)
        out.append(
            Finding(
                rule_code="D4",
                severity="warn",
                title="同一台设备被记到多个 U 位",
                detail=f"通道{m.channel_no}机柜{m.rack_no}：「{m.name}」出现在 {spans}",
                sheet=ctx.sheet_name,
                row_no=name_first_row.get((m.channel_no, m.rack_no, m.name)),
                column="U位",
                group=f"ch{m.channel_no}-rack{m.rack_no}",
                action="同一实例名只能占一个 U 位；确认是否为两台设备误用了同一名称",
            )
        )

    # D5 名称空格变体（型号名层面）
    holders: dict[str, set[str]] = defaultdict(set)
    holder_first_row: dict[str, int] = {}
    for i in range(ctx.row_count):
        for key in ("model_local", "model_peer"):
            m = ctx.value(i, key)
            if m is not None and str(m).strip():
                nk = norm_name(m)
                holders[nk].add(str(m).strip())
                holder_first_row.setdefault(nk, ctx.row_nos[i])
    for norm, variants in holder_items(holders):
        if len(variants) > 1:
            out.append(
                Finding(
                    rule_code="D5",
                    severity="info",
                    title="型号名空格变体",
                    detail=f"「{norm}」在表中出现 {len(variants)} 种写法：{'、'.join(repr(v) for v in sorted(variants))}",
                    sheet=ctx.sheet_name,
                    row_no=holder_first_row.get(norm),
                    column="本端设备/对端设备",
                    group=norm,
                    action="统一写法（系统已自动归一，仅作提示）",
                )
            )

    # D5 实例名缺失
    nameless = [i for i in inv.normalized if not any(n for n in i.names)]
    if nameless:
        out.append(
            Finding(
                rule_code="D5",
                severity="info",
                title="设备实例名为空（只有型号名）",
                detail=f"{len(nameless)} 台设备在「设备名称」列为空，只能靠 通道+机柜+U位+型号 定位",
                sheet=ctx.sheet_name,
                column="本端设备名称/对端设备名称",
                group="instance-name-missing",
                action="无名称不影响导入；生成标签/巡检清单时建议补齐",
            )
        )

    return inv, out


def holder_items(holders: Mapping[str, set[str]]):
    for k, v in sorted(holders.items()):
        yield k, v


# ============================ D6 端口重复占用 ============================


@dataclass(frozen=True, slots=True)
class PortConflict:
    channel_no: int
    rack_no: int
    u_raw: str
    model: str
    port: str
    rows: tuple[int, ...]


def check_ports(ctx: AuditContext) -> list[Finding]:
    """同一台设备上同一个端口被多行使用。**这是真正的物理冲突**：
    一个端口只能插一根线。"""
    use: dict[tuple, list[int]] = defaultdict(list)
    for i in range(ctx.row_count):
        for side in ("local", "peer"):
            ch = norm_int(ctx.value(i, f"ch_{side}"))
            rk = norm_int(ctx.value(i, f"rack_{side}"))
            u = ctx.value(i, f"u_{side}")
            model = norm_name(ctx.value(i, f"model_{side}"))
            port = ctx.value(i, f"port_{side}")
            if ch is None or rk is None or not model or not port:
                continue
            if not str(port).strip():
                continue
            key = (ch, rk, str(u).strip().upper(), model, str(port).strip())
            use[key].append(ctx.row_nos[i])

    out: list[Finding] = []
    for key, rows in sorted(use.items(), key=lambda kv: (kv[0][0], kv[0][1], kv[0][4])):
        if len(rows) < 2:
            continue
        ch, rk, u, model, port = key
        out.append(
            Finding(
                rule_code="D6",
                severity="warn",
                title="端口被多行占用",
                detail=(
                    f"通道{ch}机柜{rk} {u} 「{model}」端口 {port} "
                    f"被 {len(rows)} 行引用（第 {', '.join(str(r) for r in rows)} 行）"
                ),
                sheet=ctx.sheet_name,
                row_no=rows[0],
                column="本端接口/对端接口",
                group=f"ch{ch}-rack{rk}",
                action="一个物理端口只能接一根线；确认是否为分线（MPO 一分多）或填错",
            )
        )
    return out


# ============================ D7 孤儿机柜 ============================


def check_orphan_racks(ctx: AuditContext) -> list[Finding]:
    geo = ctx.geometry
    out: list[Finding] = []
    if geo is None:
        return out
    seen: Counter[str] = Counter()
    first_row: dict[str, int] = {}
    for i in range(ctx.row_count):
        for side in ("local", "peer"):
            ch = norm_int(ctx.value(i, f"ch_{side}"))
            rk = norm_int(ctx.value(i, f"rack_{side}"))
            if ch is None or rk is None:
                continue
            if geo.pos(RackRef(channel_no=ch, seq_no=rk)) is not None:
                continue
            k = f"通道{ch}机柜{rk}"
            seen[k] += 1
            first_row.setdefault(k, ctx.row_nos[i])
    for k, n in sorted(seen.items()):
        out.append(
            Finding(
                rule_code="D7",
                severity="warn",
                title="机柜不在机房平面图中",
                detail=f"{k} 被 {n} 行引用，但平面图上该位置不是机柜",
                sheet=ctx.sheet_name,
                row_no=first_row[k],
                column="本端机柜/对端机柜",
                group=k,
                action="补齐平面图，或修正通道/机柜号",
            )
        )
    return out


# ============================ R30 模块与批次 ============================


MODULE_SENTINEL = "一"


@dataclass(frozen=True, slots=True)
class ModulePhaseStats:
    """R30 的两个口径（必须同时披露）。"""

    #: 有真实模块型号（需采购）但无批次 → 真问题
    real_module_no_batch: int
    #: 模块列非空但全是 sentinel（一/设备自带）且无批次 → 无需采购，不是问题
    sentinel_only_no_batch: int
    #: 有批次但模块列全空 → 采购范围写了却没标模块
    batch_no_module: int


def check_module_phase(
    ctx: AuditContext,
) -> tuple[list[Finding], ModulePhaseStats]:
    """模块列有真实型号但批次（备注1）为空 → 采购范围无法按期切分。

    ## 两个口径（这是最容易报错的地方）

    | 口径 | 命中 | 含义 |
    |---|---|---|
    | 模块列**任一非空** | 126 | 架构文档 §4.14 采用的宽松口径 |
    | 模块列含**真实型号** | **36** | 真正需要采购、真正需要批次的 |

    差 90 行全部是模块列=`一`（铜缆直连、无模块），本来就不需要采购，
    拿它报"模块未标批次"会让用户以为有 126 个问题，实际只有 36 个。

    两个数都算出来，报告里以 36 为准并把 126 作为附注披露。
    """
    real_mods = {"10G-SR", "25G-SR", "100G-SR4", "光转电", "40G-SR4", "100G-LR4"}
    no_procure = {"一", "设备自带"}
    out: list[Finding] = []
    n_sentinel = 0
    n_batch_no_mod = 0

    for i in range(ctx.row_count):
        mods = [
            str(ctx.value(i, k)).strip()
            for k in ("mod_local", "mod_peer")
            if ctx.value(i, k) is not None and str(ctx.value(i, k)).strip()
        ]
        batch = ctx.value(i, "remark")
        has_batch = batch is not None and str(batch).strip() != ""

        if not mods:
            if has_batch:
                n_batch_no_mod += 1
                out.append(
                    Finding(
                        rule_code="R30b",
                        severity="info",
                        title="标了批次却没填模块",
                        detail=f"备注1=「{str(batch).strip()}」，但本端/对端模块列均为空",
                        sheet=ctx.sheet_name,
                        row_no=ctx.row_nos[i],
                        column="本端模块/对端模块",
                        group="batch-no-module",
                        action="确认该连线是否需要光模块",
                    )
                )
            continue

        if not has_batch:
            if all(m in no_procure for m in mods):
                n_sentinel += 1
                continue
            if not any(m in real_mods for m in mods):
                continue
            if not (ctx.value(i, "model_local") and ctx.value(i, "model_peer")):
                continue
            out.append(
                Finding(
                    rule_code="R30",
                    severity="warn",
                    title="模块有值但未标记批次",
                    detail=f"本端/对端模块为「{'、'.join(mods)}」，但备注列（批次）为空",
                    sheet=ctx.sheet_name,
                    row_no=ctx.row_nos[i],
                    column="本端模块/对端模块",
                    group=f"{ctx.value(i, 'model_local')}→{ctx.value(i, 'model_peer')}",
                    action="补批次备注；否则按期汇总耗材会失真、无法按到货批次核销",
                )
            )

    stats = ModulePhaseStats(
        real_module_no_batch=sum(1 for f in out if f.rule_code == "R30"),
        sentinel_only_no_batch=n_sentinel,
        batch_no_module=n_batch_no_mod,
    )
    return out, stats


# ============================ F3 待核定 ============================


def check_pending_approval(ctx: AuditContext) -> list[Finding]:
    """T 有值（或 T 是"待定"）但 U 空 —— **这是新建项目的正常状态**，不是错误。

    实测 9 行（r707..r725）。导入时必须 info，绝不能报成 9 条数据错误。
    """
    out: list[Finding] = []
    for i in range(ctx.row_count):
        t_raw = ctx.value(i, "t_ladder")
        u_raw = ctx.value(i, "u_approved")
        t_num = to_number(t_raw)
        if t_num is None:
            continue
        if u_raw is not None and str(u_raw).strip():
            continue
        out.append(
            Finding(
                rule_code="F3",
                severity="info",
                title="长度已计算未核定",
                detail=f"线缆长度1 算出 {t_num:g} 米，但「线缆长度」列未核定",
                sheet=ctx.sheet_name,
                row_no=ctx.row_nos[i],
                column="线缆长度",
                group="pending-approval",
                action="批量核定或逐行确认（这是正常中间状态，不阻断导入）",
            )
        )
    return out


# ============================ F4 IFS 缝隙 ============================


def check_ladder_gap(ctx: AuditContext) -> list[Finding]:
    """同一 S 值，在「公式逐字」与「连续化」两种口径下差一档的行。

    实测 49 行，全部是 S=6.44 —— 恰好落在 `(6, 7)` 这个开区间缝隙里，
    公式给 10 米，连续口径只给 7 米。**这不是错误，是"多买一档"的采购线索。**
    """
    out: list[Finding] = []
    seen: Counter[str] = Counter()
    for i in range(ctx.row_count):
        s = to_number(ctx.value(i, "s_total"))
        if s is None:
            continue
        t_legacy = ladder_round(float(s), IFS_COMPAT)
        t_cont = ladder_round(float(s), CONTINUOUS)
        if t_legacy is None or t_cont is None or t_legacy == t_cont:
            continue
        seen[f"{s:g}"] += 1
        out.append(
            Finding(
                rule_code="F4",
                severity="info",
                title="公式缝隙导致多买一档",
                detail=(
                    f"线缆2={s:g} 时，原公式给 {t_legacy} 米，"
                    f"连续口径只需 {t_cont} 米（相差 {t_legacy - t_cont} 米）"
                ),
                sheet=ctx.sheet_name,
                row_no=ctx.row_nos[i],
                column="线缆长度1",
                group=f"S={s:g}",
                action="原公式有 (6,7) 开区间缝隙；保持 ifs_compat 兼容，或切 continuous 省成本",
            )
        )
    return out


# ============================ D1 总览 + D9 健康度 ============================


def run_audit(ctx: AuditContext) -> HealthReport:
    """跑全部规则，产出报告。"""
    findings: list[Finding] = []

    devs, geo_stats = check_geometry(ctx)
    findings.extend(findings_geometry(devs, ctx.sheet_name))

    inv, inst_findings = check_instances(ctx)
    findings.extend(inst_findings)
    u_raw_n = len(find_u_conflicts(inv.raw))
    u_norm_n = len(find_u_conflicts(inv.normalized))
    findings.extend(check_ports(ctx))
    findings.extend(check_orphan_racks(ctx))
    findings.extend(check_non_numeric(ctx))
    module_findings, module_stats = check_module_phase(ctx)
    findings.extend(module_findings)
    findings.extend(check_pending_approval(ctx))
    findings.extend(check_ladder_gap(ctx))

    # ---- 汇总 ----
    by_rule: dict[str, list[Finding]] = defaultdict(list)
    for f in findings:
        by_rule[f.rule_code].append(f)
    titles = {f.rule_code: RULE_TITLES.get(f.rule_code, f.title) for f in findings}
    sev_of: dict[str, RuleSeverity] = {}
    for f in findings:
        cur = sev_of.get(f.rule_code)
        if cur is None or _sev_rank(f.severity) > _sev_rank(cur):
            sev_of[f.rule_code] = f.severity
    summaries = tuple(
        RuleSummary(
            rule_code=code,
            severity=sev_of[code],
            title=titles[code],
            count=len(items),
            affected_rows=len({f.row_no for f in items if f.row_no}),
        )
        for code, items in sorted(by_rule.items(), key=lambda kv: (-len(kv[1]), kv[0]))
    )

    # ---- 健康度分维度 ----
    # 健康度按「问题数 / 可比对面」比例扣分，而不是绝对条数 ——
    # 否则 5127 行的表里 11 个 U 冲突会让分数直接归零，失去区分度。
    n_rows = max(ctx.row_count, 1)
    n_inst = max(len(inv.normalized), 1)
    n_geo_cmp = max(geo_stats.get("n_comparable", 0) + geo_stats.get("o_comparable", 0), 1)
    geo_penalty = len(devs) / n_geo_cmp              # 实测 66/1277 ≈ 5.2%
    u_penalty = len(by_rule.get("D3", [])) / n_inst  # 实测 11/834 ≈ 1.3%
    port_penalty = len(by_rule.get("D6", [])) / max(
        sum(1 for _ in by_rule.get("D6", [])), 1
    )
    port_penalty = min(len(by_rule.get("D6", [])) / 5000.0, 1.0)  # 22/5000 ≈ 0.4%
    data_penalty = sum(1 for f in by_rule.get("D12", []) if f.severity == "warn") / n_rows
    breakdown = (
        ("几何一致性", round(100 * (1 - geo_penalty), 1), 0.35),
        ("U 位正确性", round(100 * (1 - min(u_penalty * 8, 1)), 1), 0.30),
        ("端口唯一性", round(100 * (1 - min(port_penalty * 8, 1)), 1), 0.20),
        ("单元格规范性", round(100 * (1 - min(data_penalty * 5, 1)), 1), 0.15),
    )

    facts: dict[str, object] = {
        "rows_total": ctx.row_count,
        "n_comparable": geo_stats.get("n_comparable", 0),
        "n_bad": geo_stats.get("n_bad", 0),
        "o_comparable": geo_stats.get("o_comparable", 0),
        "o_bad": geo_stats.get("o_bad", 0),
        "o_class_A": geo_stats.get("o_class_A", 0),
        "o_class_B": geo_stats.get("o_class_B", 0),
        "o_class_C": geo_stats.get("o_class_C", 0),
        "o_bad_A": geo_stats.get("o_bad_A", 0),
        "o_bad_B": geo_stats.get("o_bad_B", 0),
        "o_bad_C": geo_stats.get("o_bad_C", 0),
        "u_conflicts_raw": u_raw_n,
        "u_conflicts_normalized": u_norm_n,
        "u_name_variant_pairs": u_raw_n - u_norm_n,
        "instances": len(inv.normalized),
        "r30_real_module_no_batch": module_stats.real_module_no_batch,
        "r30_sentinel_only_no_batch": module_stats.sentinel_only_no_batch,
        "r30_batch_no_module": module_stats.batch_no_module,
        "r30_loose_count": module_stats.real_module_no_batch
        + module_stats.sentinel_only_no_batch,
        "geom_missing_rows": geo_stats.get("geom_missing", 0),
    }

    return HealthReport(
        project_label=ctx.project_label,
        total_rows=ctx.row_count,
        findings=tuple(findings),
        summaries=summaries,
        score_breakdown=breakdown,
        facts=facts,
    )


def _sev_rank(s: RuleSeverity) -> int:
    return {"info": 0, "warn": 1, "blocker": 2}[s]


def _rate(bad: int, total: int) -> float:
    if total <= 0:
        return 0.0
    return min(bad / total, 1.0)


__all__ = [
    "AuditContext",
    "RULE_TITLES",
    "Finding",
    "GeometryDeviation",
    "HealthReport",
    "MODULE_SENTINEL",
    "NUMERIC_COLUMNS",
    "PortConflict",
    "RuleSeverity",
    "RuleSummary",
    "check_geometry",
    "check_instances",
    "check_ladder_gap",
    "check_module_phase",
    "check_non_numeric",
    "check_orphan_racks",
    "check_pending_approval",
    "check_ports",
    "findings_geometry",
    "run_audit",
]
