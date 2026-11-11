"""CLI 入口：`idcbox import / check / export / verify`。

## 为什么 CLI 先于 Web

v0.1 的验收标准是「导出 25 列逐格 diff 为空」。这是可以**在终端里一次跑完**的判定，
不需要点击 30 次页面。先用 CLI 把整条流水线跑通并固化，Web 层只是它的可视化外壳 ——
这样即使 Web 层永远做不完，产品也已经能交付价值。

更重要的是：CLI 让「回归」变成一行命令（`idcbox verify`），
用户可以自己在交付前跑一遍，而不是等我们承诺"我们测过了"。

## 四个子命令的分工

- `import`：只读入 + 体检，输出报告（不动文件，最安全的入口）
- `check`：同 import，但可指定规则集与输出路径
- `export`：读入 + 计算 + 导出（可选同时体检）
- `verify`：导出到临时路径并与源文件逐格比较 —— **验收铁线的一键装置**

## 退出码约定

`0` 成功、`1` 有 blocker、`2` 参数/文件错误、`3` 校验不一致。
让 CI 能直接 `idcbox verify || exit 1`。
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import warnings
from pathlib import Path
from typing import Any, Sequence

EXIT_OK = 0
EXIT_BLOCKER = 1
EXIT_USAGE = 2
EXIT_MISMATCH = 3


class CliError(Exception):
    """可预期的用户错误（文件不存在、sheet 名打错）。

    **不能直接用 SystemExit**：`SystemExit` 会绕过 `main()` 的 except 分支直接冒泡，
    进程退出码变成 1 而不是我们约定的 2，且调用方（测试、上层脚本）无法区分
    "用户参数错" 和 "程序崩了"。
    """

#: 源文件默认位置（相对仓库根）。用户可用 --source 覆盖。
DEFAULT_SOURCE = "信息港部署上架/V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"


def _repo_root() -> Path:
    """仓库根 = 本文件往上 3 层（idcbox/cli.py → idcbox → backend → 根）。"""
    return Path(__file__).resolve().parent.parent.parent


def _resolve_source(value: str) -> Path:
    p = Path(value)
    if p.is_absolute():
        return p
    root = _repo_root()
    for candidate in (root / p, Path.cwd() / p):
        if candidate.exists():
            return candidate
    return root / p


def _build_project(args: argparse.Namespace):
    """按参数装配数据源并跑完 load + compute + check。"""
    from .application.services.pipeline import (
        check_project,
        compute_project,
        load_project,
    )
    from .domain.cable_length.rules import CONTINUOUS_RULE, DEFAULT_RULE
    from .infrastructure.config.empirical_loader import load_empirical_table
    from .infrastructure.xlsx.sources import XlsxCablingSource, XlsxFloorSource

    source = _resolve_source(args.source)
    if not source.exists():
        raise CliError(f"源文件不存在：{source}")

    rule = CONTINUOUS_RULE if getattr(args, "continuous", False) else DEFAULT_RULE
    table = None
    if getattr(args, "empirical", None):
        table = load_empirical_table(_resolve_source(args.empirical))

    project = load_project(
        floor_source=XlsxFloorSource(source, args.floor_sheet),
        cabling_source=XlsxCablingSource(source, args.cabling_sheet),
        label=source.stem,
        source_path=source,
        rule=rule,
    )
    computed = compute_project(project, table=table)
    return check_project(computed), table


def _print_banner(checked) -> None:
    project = checked.project
    print(project.summary())
    print(checked.computed.summary())


def _print_findings(checked, *, limit: int) -> None:
    report = checked.report
    print()
    print(f"体检得分：{report.score} / 100    发现 {len(report.findings)} 条"
          f"（blocker {report.blocker_count} / warn {report.warn_count} / info {report.info_count}）")
    print()
    ordered = sorted(report.summaries, key=lambda s: (-s.count, s.rule_code))
    for s in ordered:
        print(f"  {s.rule_code:5s} {s.severity:8s} {s.title}：{s.count}")
    if limit:
        print()
        print("前若干条明细：")
        for f in report.findings[:limit]:
            where = f"r{f.row_no}" if f.row_no else "—"
            print(f"  [{f.rule_code}] {where:>7s} {f.detail}")


# --------------------------------------------------------------------------
# 子命令
# --------------------------------------------------------------------------


def cmd_import(args: argparse.Namespace) -> int:
    checked, _ = _build_project(args)
    _print_banner(checked)
    _print_findings(checked, limit=args.limit)
    return EXIT_BLOCKER if checked.report.blocker_count else EXIT_OK


def cmd_check(args: argparse.Namespace) -> int:
    from .domain.validation.report import render_markdown

    checked, _ = _build_project(args)
    _print_banner(checked)
    _print_findings(checked, limit=args.limit)

    if args.out:
        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(render_markdown(checked.report), encoding="utf-8")
        print()
        print(f"报告已写出：{out}")
    return EXIT_BLOCKER if checked.report.blocker_count else EXIT_OK


def _build_export_rows(checked) -> list:
    """把「原值 + 计算值」拼成导出行的两套投影。

    **保真第一原则：源值有值就原样输出，我们不是来改用户数据的。**
    只有源值为空时，才补我们自己算的值；且 U 列（人工核定）永远不自动补 ——
    那是人的判断，机器填了就是伪造。

    两套投影的分工：
    - `raw`（列索引 → 原值）：导出器写单元格时优先用它，保证逐格回写
    - `values`（字段名 → 值）：`raw` 缺位时的兜底
    """
    from .domain.exporting.contract25 import CONTRACT_25
    from .infrastructure.xlsx.exporter import ExportRow
    from .infrastructure.xlsx.v13_adapter import RAW_ALL_KEY

    project = checked.project
    results = checked.computed.batch.by_key()
    rows: list[ExportRow] = []

    for link in project.links:
        raw = project.raw_values.get(link.stable_key, {})
        result = results.get(link.stable_key)

        values: dict[str, Any] = {
            "local_model": link.a.device_model,
            "local_device": link.a.device_name,
            "local_channel": link.a.rack.channel_no,
            "local_rack": link.a.rack.seq_no,
            "local_u": link.a.u_position,
            "local_port": link.a.port_name,
            "peer_model": link.b.device_model,
            "peer_device": link.b.device_name,
            "peer_channel": link.b.rack.channel_no,
            "peer_rack": link.b.rack.seq_no,
            "peer_u": link.b.u_position,
            "peer_port": link.b.port_name,
            "cable_type": link.cable_type,
            "local_module": raw.get("local_module"),
            "peer_module": raw.get("peer_module"),
            "remark": raw.get("remark"),
        }

        # 长度块：源值优先，源值空才补计算值
        for field in _LENGTH_FIELDS:
            src_val = raw.get(_RAW_KEY_OF[field])
            if src_val is not None and not (isinstance(src_val, str) and not src_val.strip()):
                values[field] = src_val
            elif result is not None:
                values[field] = _computed_or_none(field, result)

        # raw 视图：优先用整行原始元组（归一化前的原值），保证逐字还原
        cols = raw.get(RAW_ALL_KEY)
        if cols is not None:
            raw_idx = {i: v for i, v in enumerate(cols) if i < len(CONTRACT_25)}
        else:
            raw_idx = {}
            for spec in CONTRACT_25:
                key = _RAW_KEY_OF.get(spec.field or "")
                if key is not None:
                    raw_idx[spec.index] = raw.get(key)
        rows.append(ExportRow(source_row_no=link.source_row_no, values=values, raw=raw_idx))
    return rows


#: 长度块 8 个字段（N/O/P/Q/R/S/T/U）
_LENGTH_FIELDS = (
    "n_cross_rack",
    "o_gap_rack",
    "p_rack_count",
    "q_in_rack_m",
    "r_inter_rack_m",
    "s_total_m",
    "t_ladder_m",
    "u_approved_m",
)

#: 导出字段 → 原值快照键
_RAW_KEY_OF = {
    "n_cross_rack": "n_cross",
    "o_gap_rack": "o_gap",
    "p_rack_count": "p_count",
    "q_in_rack_m": "q_in_rack",
    "r_inter_rack_m": "r_inter",
    "s_total_m": "s_total",
    "t_ladder_m": "t_ladder",
    "u_approved_m": "u_approved",
    "local_module": "local_module",
    "peer_module": "peer_module",
    "remark": "remark",
}


def _computed_or_none(field: str, result) -> Any:
    """从 `LengthResult` 取该字段。

    **U 列（line 20 / 线缆长度）刻意返回 None**：它是人工核定值，
    源表没填就是"待核定"，自动补一个数字等于伪造人的判断。
    """
    if field == "u_approved_m":
        return None
    return getattr(result, field, None)


def _load_intent_source(args: argparse.Namespace) -> "Intent":
    """从 `--intent`（JSON）或 `--sheet`（六段 Excel）读意图。

    ## 为什么两个入口是同一个函数

    生成器不该知道意图是 JSON 还是 Excel 来的 —— 那是输入格式问题，
    不是算法问题。若在 `cmd_generate` 里写两个分支，两个分支的
    「提示语 / 错误信息 / 缺表检查」很快会漂移。

    优先级：给了 `--sheet` 就用表格，否则用 `--intent`。两个都给属误用，
    直接报错而不是默默选一个 —— 用户会以为程序读了他想看的那份。
    """
    from .domain.ports.intent import Intent, load_intent

    sheet = getattr(args, "sheet", "") or ""
    json_path = getattr(args, "intent", "") or ""
    if sheet and json_path:
        raise CliError("--intent 与 --sheet 不能同时给：请指定意图的来源")
    if sheet:
        from .infrastructure.xlsx.intent_sheet import (
            REQUIRED_SHEETS,
            read_intent_workbook,
        )

        path = Path(sheet)
        if not path.exists():
            raise CliError(f"意图表格不存在：{path}")
        intent, _found, missing = read_intent_workbook(
            path, project=getattr(args, "project", "") or ""
        )
        lost = [n for n in missing if n in REQUIRED_SHEETS]
        if lost:
            print(f"⚠️  意图表格缺少必需的表：{'、'.join(lost)}（按空处理）")
        return intent
    if not json_path:
        raise CliError("必须指定意图来源：--intent <json> 或 --sheet <xlsx>")
    path = Path(json_path)
    if not path.exists():
        raise CliError(f"意图文件不存在：{path}")
    return load_intent(path)


def _flow_plan(intent):
    """跑一遍端口分配流程，得到接线区间（`switch_service_range` 的来源）。

    ## 为什么失败时返回 None 而不是中断

    接线区间回答「这台交换机的哪几个口是自动分给哪个型号的节点」，
    是**解释性**数据：连线本身已经算出来了，没有区间不影响施工。
    而 `plan_flow` 在意图缺布局段或交换机组时会报结构性错误 ——
    那时用户更该看到连线结果 + 那条错误，而不是整个分解失败。
    """
    from .domain.ports.flow import plan_flow
    from .domain.ports.intent import to_flow_inputs

    try:
        placements, routes, chains, _ = to_flow_inputs(intent)
        return plan_flow(placements, routes, chains)
    except (ValueError, KeyError) as exc:
        print(f"⚠️  端口分配流程跳过（接线区间为空）：{exc}")
        return None


def _geometry_snapshot(args: argparse.Namespace):
    """从源 Excel 的机房平面图读出几何快照（房间/通道/机柜/列行坐标）。

    ## 为什么分解需要它

    规范表里 `room`/`aisle`/`rack` 三张表的内容全部来自平面图几何，
    意图里只有 `(通道, 机柜号)`，没有「机柜在平面图第几列第几行」。
    没有几何就没法写这三张表。

    ## 读不到时返回 None 而不是报错

    几何只用于填位置坐标；`room`/`aisle`/`rack` 是分解的展示维度，
    不是连线与端口的前置条件。源文件不在（比如只带了一份意图 JSON 出去）
    时，仍然应该能把设备/端口/连线落库 —— 少三张表的位置信息，
    比整批分解失败好。
    """
    from .domain.geometry.floor import FloorCell, derive_cmax_by_room, parse_floor
    from .domain.geometry.layout import build_snapshot
    from .infrastructure.xlsx.openpyxl_reader import read_floor_cells

    # 走 `_resolve_source`：`--source` 的默认值是相对仓库根的路径，
    # 直接 `Path(args.source)` 在 `cd backend` 之后会找不到文件。
    path = _resolve_source(args.source)
    if not path.exists():
        print(f"⚠️  源文件不存在（{path}），房间/通道/机柜的位置信息将缺失")
        # 返回空快照而不是 None：`build_canonical` 要求 geometry 必填，
        # 传 None 会在 `canon_rooms` 里 `AttributeError`。
        # 空快照让 rooms/aisles/racks 三张表为空，其余八张表照常落库。
        from .domain.geometry.layout import GeometrySnapshot

        return GeometrySnapshot(version="empty", racks={}, cmax_by_room={})
    cells = read_floor_cells(path, sheet_name=args.floor_sheet)
    floor = parse_floor([FloorCell(c.row, c.col, c.value) for c in cells])
    return build_snapshot(
        floor.racks, version=f"floor@{path.name}", cmax_override=derive_cmax_by_room(floor.racks)
    )


def cmd_intent_export(args: argparse.Namespace) -> int:
    """意图 JSON → 六段 Excel 模板（给人填）。

    ## 为什么要有这一步

    `intent.json` 是 915 行机器格式，做机房规划的工程师填不了。
    但项目往往**已有存量布线表**（如 V13），此时不需要从零填 ——
    正确做法是把存量项目导成模板，用户在上面改。

    这是「导入表格」的另一半：没有模板，用户不知道六张表该长什么样。
    """
    from .domain.ports.intent import load_intent
    from .infrastructure.xlsx.intent_sheet import write_intent_xlsx

    intent_path = Path(args.intent)
    if not intent_path.exists():
        raise CliError(f"意图文件不存在：{intent_path}")
    intent = load_intent(intent_path)
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    write_intent_xlsx(intent, out)

    print(f"意图：{intent.project}")
    print(
        f"  布局块 {len(intent.racks)}，交换机 {len(intent.switches)}，"
        f"特殊设备 {len(intent.devices)}，路由 {len(intent.routes)}，"
        f"交换机组 {len(intent.chains)}，显式连线 {len(intent.explicit_links)}"
    )
    total = (
        len(intent.racks) + len(intent.switches) + len(intent.devices)
        + len(intent.routes) + len(intent.chains) + len(intent.explicit_links)
    )
    print(f"已导出 6 张表 / {total} 行 → {out}")
    print("  填表提示写在每张表的第 2 行（灰色斜体，可删）")
    return EXIT_OK


def cmd_intent_check(args: argparse.Namespace) -> int:
    """六段 Excel → 意图 → 体检。**只检查不生成**，改表时反复跑这个。"""
    from .infrastructure.xlsx.intent_sheet import (
        OPTIONAL_SHEETS,
        REQUIRED_SHEETS,
        read_intent_workbook,
    )

    path = Path(args.sheet)
    if not path.exists():
        raise CliError(f"意图表格不存在：{path}")
    intent, found, missing = read_intent_workbook(
        path, project=args.project, site=args.site
    )

    print(f"读入：{path.name}")
    print(f"  找到 {len(found)} 张表：{'、'.join(found) or '（无）'}")
    lost_required = [n for n in missing if n in REQUIRED_SHEETS]
    lost_optional = [n for n in missing if n in OPTIONAL_SHEETS]
    if lost_required:
        print(f"  ⚠️  缺少必需的表：{'、'.join(lost_required)} —— 这几段按空处理")
    if lost_optional:
        print(f"  提示：缺可选表 {'、'.join(lost_optional)}（新项目正常）")

    total = (
        len(intent.racks) + len(intent.switches) + len(intent.devices)
        + len(intent.routes) + len(intent.chains) + len(intent.explicit_links)
    )
    print(
        f"  布局块 {len(intent.racks)}，交换机 {len(intent.switches)}，"
        f"特殊设备 {len(intent.devices)}，路由 {len(intent.routes)}，"
        f"交换机组 {len(intent.chains)}，显式连线 {len(intent.explicit_links)}"
        f"（合计 {total} 行）"
    )
    print(f"  按布局展开 {intent.server_count} 台服务器")

    if args.out:
        from .domain.ports.intent import dump_intent

        dump_intent(intent, args.out)
        print(f"  已转成 JSON → {args.out}")

    if lost_required:
        return EXIT_BLOCKER
    return EXIT_OK


def cmd_generate(args: argparse.Namespace) -> int:
    """意图 JSON → 布线表（这是 v0.3 的主功能：不再需要手填 5000 行）。

    与 `export` 的根本区别：
    - `export` 是**存量保真**：读用户的 Excel，算长度，原样回写
    - `generate` 是**从零生成**：只读意图（几十到几百行），产出全部连线

    两者最终都写同一套 24 列契约，所以生成结果可以拿去和存量表逐格比对。
    """
    from .domain.cable_length.engine import compute_batch
    from .domain.ports.bridge import to_link_inputs
    from .domain.ports.generate import generate_links
    from .infrastructure.xlsx.exporter import ExportRow, write_xlsx

    intent = _load_intent_source(args)

    print(f"意图：{intent.project}")
    print(
        f"  布局块 {len(intent.racks)}，交换机 {len(intent.switches)}，"
        f"路由 {len(intent.routes)}，交换机组 {len(intent.chains)}，"
        f"显式连线 {len(intent.explicit_links)}"
    )

    cabling = generate_links(intent)
    print(f"生成 {len(cabling)} 条连线")
    for nic, n in cabling.by_nic:
        print(f"  {nic}: {n} 条")
    if cabling.notes:
        for note in cabling.notes:
            print(f"  提示：{note}")

    errors = [i for i in cabling.issues if i.severity == "error"]
    warns = [i for i in cabling.issues if i.severity == "warn"]
    if errors:
        print(f"\n❌ 生成失败：{len(errors)} 个错误")
        for i in errors[:20]:
            print(f"  {i.code}: {i.message}")
        return EXIT_BLOCKER
    if warns:
        print(f"\n⚠️  {len(warns)} 个警告（不影响生成）")
        for i in warns[: args.limit or 5]:
            print(f"  {i.code}: {i.message}")

    links, bridge_notes = to_link_inputs(cabling)
    for note in bridge_notes:
        print(f"  提示：{note}")

    # 算长度：几何来自平面图，规则来自 length_rule
    from .domain.cable_length.rules import CONTINUOUS_RULE, DEFAULT_RULE

    rule = CONTINUOUS_RULE if getattr(args, "continuous", False) else DEFAULT_RULE
    geo = _geometry_for_generate(args)
    batch = compute_batch(links, rule, geo)

    by_key = {r.stable_key: r for r in batch.results}
    # stable_key → 对端显示名。生成行的 `b_device` 一律为空（源表这一列本就
    # 只填了 332/4562 行），不能拿桥接层回落后的槽位 id 当名字导出。
    peer_name_of = {
        li.stable_key: gen.b_device for li, gen in zip(links, cabling.links)
    }
    rows: list[ExportRow] = []
    for li in links:
        r = by_key.get(li.stable_key)
        values = {
            "local_model": li.a.device_model,
            "local_device": li.a.device_name,
            "local_channel": li.a.rack.channel_no,
            "local_rack": li.a.rack.seq_no,
            "local_u": li.a.u_position,
            "local_port": li.a.port_name,
            "peer_model": li.b.device_model,
            # `peer_device` 写**显示名**，而桥接层把它的身份位换成了槽位 id
            # （`N02-11-S7`，为了让 stable_key 唯一）。这里必须回到
            # `GeneratedLink.b_device`：真实表这一列是空的就是空的，
            # 把 `N02-11-S7` 写进去等于给施工队造了一个不存在设备名。
            "peer_device": peer_name_of.get(li.stable_key, ""),
            "peer_channel": li.b.rack.channel_no,
            "peer_rack": li.b.rack.seq_no,
            "peer_u": li.b.u_position,
            "peer_port": li.b.port_name,
            "cable_type": li.cable_type,
        }
        if r is not None:
            for field in _LENGTH_FIELDS:
                # U 列是人工核定值，机器不填（与 export 同一铁律）
                if field == "u_approved_m":
                    continue
                values[field] = getattr(r, field, None)
        rows.append(ExportRow(source_row_no=None, values=values))

    out = Path(args.out)
    path = write_xlsx(rows, out, mode="value", sheet_name=args.cabling_sheet)

    ok = batch.stats.computed_rows
    print()
    print(f"已导出 {len(rows)} 行 × 24 列 → {path}")
    print(f"长度计算：{ok}/{len(rows)} 行算出几何长度（U 列留空待人工核定）")
    if batch.stats.error_rows:
        print(f"  {batch.stats.error_rows} 行算出错")
    return EXIT_OK


def _geometry_for_generate(args: argparse.Namespace):
    """生成模式也要几何快照：线长的 N/O 依赖机柜平面位置。

    优先用 `--source` 指向的 Excel 的平面图 sheet；用户也可以给
    `--floor` 单独指定一份。两者都没有时明确报错 —— 绝不能拿默认几何糊过去，
    那会让所有跨机柜长度算错而用户毫无察觉。
    """
    from .domain.geometry.floor import FloorCell, derive_cmax_by_room, parse_floor
    from .domain.geometry.layout import build_snapshot
    from .infrastructure.xlsx.sources import XlsxFloorSource

    src = _resolve_source(args.floor or args.source)
    if not src.exists():
        raise CliError(
            f"平面图来源不存在：{src}\n"
            "生成线长需要机柜平面位置（跨机柜/间隔机柜个数）。"
            "请用 --floor 指定含平面图的 Excel，或用 --source 指定同款文件。"
        )
    cells, _row_range, _col_range = XlsxFloorSource(src, args.floor_sheet).read_floor_cells()
    floor = parse_floor([FloorCell(c[0], c[1], c[2]) for c in cells])
    cmax = derive_cmax_by_room(floor.racks)
    return build_snapshot(floor.racks, version="generate", cmax_override=cmax)


def cmd_consumables(args: argparse.Namespace) -> int:
    """耗材与模块需求清单：回答「要买多少根」。

    ## 与 `export` / `generate` 的分工

    三者跑同一份布线数据，但输出不同交付物：
    - `export`   → 布线表（哪根线接哪里，施工队用）
    - `generate` → 布线表（从意图生成）
    - `consumables` → 采购清单（买多少，采购用）

    ## 为什么 `--phase` 必须显式给

    原表 `首期` 标记只覆盖 450/5127 行（8.8%）。不指定阶段就是全项目需求，
    指定 `--phase 首期` 才是首期采购量。默认不给 = 全量，并**在输出里写明**
    「当前是全项目口径」—— 否则用户会把全量当首期买，多买 11 倍。

    ## 为什么采购量是「建议」不是结论

    实测原表人工采购倍数是 1.124~2.314（10G-SR 买 2.3 倍备件，100G-SR4 只买 1.12 倍），
    固定系数复现不了。程序只给「需求 + 按系数算的建议」，最终数量由人定。
    """
    from .application.services.pipeline import compute_project
    from .application.services.views import build_views, consumable_payload

    checked, _ = _build_project(args)
    views = build_views(checked.project, compute_project(checked.project))

    phase = args.phase or None
    payload = consumable_payload(views, phase=phase)

    scope = f"阶段「{phase}」" if phase else "全项目（未按阶段过滤）"
    print(f"口径：{scope} —— {payload['rowsTotal']} 行连线")
    print()

    cables = payload["cables"]
    print(f"线缆需求：{cables['total']} 根")
    by_type = cables["byType"]
    if by_type:
        print("  " + "    ".join(f"{k} {v}" for k, v in sorted(by_type.items())))
    print()
    print(f"  {'类型':8s} {'长度':>6s} {'需求':>6s} {'建议采购':>8s}")
    for row in cables["rows"]:
        print(
            f"  {row['type']:8s} {row['length']:>4d}m "
            f"{row['demand']:>6d} {row['suggestedPurchase']:>8d}"
        )
    ratio = cables["purchaseRatio"]
    print(
        f"\n  建议采购量 = 需求 × {ratio}（向上取整）。"
        "这不是最终采购量 —— 实测人工核定倍数为 1.12~2.31，请逐行核定。"
    )

    modules = payload["modules"]
    print()
    print("模块需求（本端 / 对端 / 合计）：")
    for row in modules["rows"]:
        print(
            f"  {row['module']:10s} {row['local']:>5d} {row['peer']:>5d} {row['total']:>6d}"
        )
    # 覆盖率必须打印：按覆盖率外推会少买模块，是现场停工级事故
    print()
    print(f"  ⚠️  {modules['disclosure']}")

    if args.out:
        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        out.write_text(
            json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8"
        )
        print(f"\n清单已写出：{out}")
    return EXIT_OK


def cmd_labels(args: argparse.Namespace) -> int:
    """线缆标签清单：回答「这张贴纸印什么字」。

    ## 与 `generate` / `consumables` 的分工

    - `generate`    → 哪根线接哪儿（施工）
    - `consumables` → 要买多少根线（采购）
    - `labels`      → 每端贴什么字（贴标）

    三份交付物都从同一份意图算，但**份数不同**（标签每端一张 = 2 倍行数）、
    **排序不同**（标签按施工贴纸顺序，不是按表行序）。

    ## 为什么默认不自动编号

    源表里 564 台服务器没有实例名。自动编号会发明一个客户没见过的名字；
    若客户已有自己的编号规则，印上去的编码与他们的台账对不上，
    等于制造新的一份错误数据。默认留空 + 报 `NO_DEVICE_CODE`（提示性质），
    由 `--number-missing` 显式打开。
    """
    from .application.services.labelsvc import build_label_payload
    from .domain.ports.generate import generate_links
    from .domain.validation.instances import extract_instances

    intent = _load_intent_source(args)
    cabling = generate_links(intent)

    errors = [i for i in cabling.issues if i.severity == "error"]
    if errors:
        print(f"❌ 意图有 {len(errors)} 个错误，先修好再出标签")
        for i in errors[:20]:
            print(f"  {i.code}: {i.message}")
        return EXIT_BLOCKER

    # 标签要设备名，而连线里的服务器名普遍为空 —— 用实例台账补。
    # 走 `adapt_intent_rows` 而不是自己拼：CLI 侧没有 Excel 行，
    # 所以直接构造一个最小的 inventory。
    inventory = _inventory_from_intent(intent)

    payload = build_label_payload(
        links=cabling.links,
        inventory=inventory,
        project_label=intent.project or "未命名项目",
        limit=args.limit or None,
        number_missing_codes=args.number_missing,
    )

    print(f"标签清单：{payload['describe']}")
    if payload["rows"]:
        print("  按施工顺序：机柜 → U 位从上往下 → 端口")
        for row in payload["rows"]:
            print(f"  {row['text']}")
    if payload["issues"]:
        print("\n问题：")
        for issue in payload["issues"][:10]:
            print(f"  [{issue['severity']}] {issue['code']} {issue['label']}")
            print(f"      {issue['message']}")
        if len(payload["issues"]) > 10:
            print(f"  …另有 {len(payload['issues']) - 10} 条")

    if args.out:
        args.out.write_text(
            json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8"
        )
        print(f"\n已写出：{args.out}")
    return EXIT_OK


def _inventory_from_intent(intent):
    """从意图构造一个最小 inventory（只有 `.normalized` 里的实例名有用）。

    ## 为什么不用 `extract_instances`

    那个函数吃的是 Excel 行（`Sequence[Sequence[object]]` + 列索引映射），
    CLI 手上没有。但标签只需要一件事：**这个位置的设备叫什么名字**。
    从意图的 `switches` / `devices` 两段就能拿到，布局段排布的服务器
    本来就没有名字（源表也没有），所以不需要展开 663 台。

    `span` 用 `(u_start, u_start + u_span - 1)`，与其他地方的 U 位区间口径一致。
    """
    from .domain.ports.intent import intent_to_dict  # noqa: F401  (保持可读性)
    from .domain.validation.instances import DeviceInstance, InstanceInventory

    items: list[DeviceInstance] = []
    for spec in (*intent.switches, *intent.devices):
        base = {
            "channel_no": spec.channel_no,
            "rack_no": spec.rack_no,
            "u_raw": f"{spec.u_start}U" if getattr(spec, "u_span", 1) == 1 else f"{spec.u_start}-{spec.u_start + spec.u_span - 1}U",
            "span": (spec.u_start, spec.u_start + getattr(spec, "u_span", 1) - 1),
            "model_raw": spec.model,
            "model": spec.model,
            "names": {spec.instance} if spec.instance else set(),
            "ports": set(),
            "rows": [],
        }
        items.append(DeviceInstance(**base))
    return InstanceInventory(normalized=tuple(items), raw=tuple(items), rows_total=len(items))


def cmd_decompose(args: argparse.Namespace) -> int:
    """把意图分解成规范台账并落库（v0.6）。

    ## 这一层解决什么问题

    v0.5 的台账是现算的：每次请求都从 949 行意图重新展开 12820 个端口。
    三个地方不够用：

    1. 没法按状态/角色/批次筛选 —— 「哪些线还是 planned」查不出来
    2. 没法记录人的改动 —— 人工核定的线长、标成 reserve 的口
    3. 没法回答「这根线什么时候改过」—— 没有行就没有 created_at

    落库之后这三件事都成立。

    ## 为什么是全量重建而不是增量

    949 行意图摊平出 2 万行只要 ~300ms，而增量同步的 bug（漏删、错配、
    级联顺序）比省下的 300ms 贵得多。每次 `write()` 都是「先清空再插入」，
    单个事务，失败即回滚 —— 不会留下半套数据。

    ## 为什么长度字段留空

    `cable` 有 15 个长度相关列，本命令只写结构（两端、线缆类型、模块、备注）。
    长度是 `domain/cable_length/` 的事，它需要 `LengthRule` + `GeometrySnapshot`
    + `EmpiricalTable` 三样输入。把长度也算进来会让「只重算长度、不动结构」
    这种常见操作没法做。
    """
    from .domain.ports.canon import build_canonical
    from .domain.ports.generate import generate_links
    from .domain.ports.ledger import ledger_from_cabling
    from .infrastructure.db.canon_store import CanonicalStore
    from .infrastructure.db.repositories import ProjectRepository
    from .infrastructure.db.session import create_db_engine, init_schema

    intent = _load_intent_source(args)
    cabling = generate_links(intent)

    errors = [i for i in cabling.issues if i.severity == "error"]
    if errors:
        print(f"❌ 意图有 {len(errors)} 个错误，先修好再分解")
        for i in errors[:20]:
            print(f"  {i.code}: {i.message}")
        return EXIT_BLOCKER

    library = _model_library(args)
    ledger = ledger_from_cabling(cabling, library=library)
    geometry = _geometry_snapshot(args)
    bundle = build_canonical(
        ledger,
        geometry=geometry,
        links=cabling.links,
        plan=_flow_plan(intent),
        library=library,
    )

    print(f"意图 → 规范记录：{bundle.describe()}")
    if args.dry_run:
        print("（--dry-run：未落库）")
        for d in bundle.devices[:5]:
            print(f"  {d.display_label}")
        return EXIT_OK

    engine = create_db_engine(_db_path(args))
    init_schema(engine)
    projects = ProjectRepository(engine)
    name = args.name or intent.project or "未命名项目"
    project = projects.create(name=name, site_key=intent.site or "")

    store = CanonicalStore(engine)
    stats = store.write(project.id, bundle, note=f"decompose {len(cabling)} 条连线")
    print(f"落库到项目「{project.name}」：{stats.describe()}")

    back = store.counts(project.id)
    expect = {
        "rooms": len(bundle.rooms), "aisles": len(bundle.aisles),
        "racks": len(bundle.racks), "modelPorts": len(bundle.model_ports),
        "devices": len(bundle.devices), "ports": len(bundle.ports),
        "cables": len(bundle.cables), "ranges": len(bundle.ranges),
        "conflicts": len(bundle.conflicts),
    }
    bad = {k: (v, back[k]) for k, v in expect.items() if back[k] != v}
    if bad:
        print(f"❌ 回读与写入不一致（静默跳过）：{bad}")
        return EXIT_BLOCKER
    print("✅ 回读校验通过：10 张表的行数与写入完全一致")

    if args.stats:
        print(f"  端口状态：{store.port_status_counts(project.id)}")
        print(f"  端口角色：{store.port_role_counts(project.id)}")
        print(f"  线缆类型：{store.cable_type_counts(project.id)}")
        print(f"  设备状态：{store.devices_by_state(project.id)}")
        oc = store.open_conflicts(project.id)
        if oc:
            print(f"  未解决冲突 {len(oc)} 条：")
            for c in oc[:10]:
                print(f"    {c['kind']} U{c['uStart']}-{c['uEnd']} {c['detail'].get('detail', '')}")
    return EXIT_OK


def _model_library(args: argparse.Namespace):
    """按命令行给的文件叠出型号库。"""
    from .domain.ports.modellib import load_model_library
    from .infrastructure.xlsx.model_sheet import read_model_sheet

    entries = None
    detail = ""
    if getattr(args, "models_xlsx", ""):
        entries, warns = read_model_sheet(args.models_xlsx)
        detail = Path(args.models_xlsx).name
        for w in warns:
            print(f"⚠️  {w}")
    return load_model_library(
        json_path=getattr(args, "models_json", "") or None,
        excel_entries=entries,
        excel_detail=detail,
    )


def cmd_models(args: argparse.Namespace) -> int:
    """设备型号库的查看 / 导出 / 校验（v0.7）。

    ## 三条子动作的分工

    - 不给参数：列出当前生效的库（型号、端口数、出处）
    - `--export-json/--export-xlsx`：把库整份导出，交给现场去改
    - `--check`：只报问题（缺端口组、口径可疑），不打印全表

    `--check` 单独存在的理由：型号库通常有 20+ 型号、上千行端口，
     全表刷屏会把真正需要注意的那两三条提示淹没。
    """
    from .domain.ports.modellib import library_to_json

    lib = _model_library(args)
    if args.check:
        if not lib.warnings:
            print("✅ 型号库没有发现问题")
            return 0
        for w in lib.warnings:
            print(f"⚠️  {w}")
        return 0

    # 两个导出开关都要生效：`models --export-json a.json --export-xlsx a.xlsx`
    # 是很自然的一次调用（同一份库给两个人用），若写成 `elif`
    # 第二个会被静默忽略 —— 用户以为导出了，实际文件不存在。
    if args.export_json:
        import json as _json

        path = Path(args.export_json)
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            _json.dumps(library_to_json(lib), ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
        )
        print(f"已导出 JSON：{path}（{len(lib.specs)} 个型号 / {lib.total_ports()} 个口）")

    if args.export_xlsx:
        from .infrastructure.xlsx.model_sheet import write_model_sheet

        out = write_model_sheet(lib, args.export_xlsx)
        print(f"已导出 Excel：{out}（{len(lib.specs)} 个型号 / {lib.total_ports()} 个口）")

    if args.export_json or args.export_xlsx:
        return 0

    print(lib.describe())
    if lib.warnings:
        for w in lib.warnings:
            print(f"  ⚠️  {w}")
    print(f"  {'型号':<22}{'类别':<10}{'端口':>5}  {'出处'}")
    for spec in lib.all():
        src = lib.source_of(spec.model)
        print(
            f"  {spec.model:<22}{spec.category:<10}{spec.total_ports:>5}  "
            f"{src.describe() if src else '—'}"
        )
    return 0


def cmd_ledger(args: argparse.Namespace) -> int:
    """设备台账 + 端口台账：回答「这个口用了没 / 还剩几个口」。

    ## 与 `consumables` / `generate` 的分工

    三者读同一份意图，但回答的问题不同：
    - `generate`    → 哪根线接哪里（施工）
    - `consumables` → 要买多少根线、多少个模块（采购）
    - `ledger`      → 每台设备用了几个口、还剩几个口（规划复核 / 扩容评估）

    ## 为什么台账要从意图现算而不是查库

    v0.5 的意图仍以版本化 JSON 存（规范表落库是 v0.6 的事），
    所以台账是「意图 → 生成连线 → 聚合端口」的一次性计算，
    与页面看到的是同一份真相。查库反而会读到上次保存的旧版本。

    ## 为什么分母只算有模板的设备

    型号在 `portspec.py` 里没有模板时，标称端口数未知。把它算成 0 或
    猜一个数，会让整体利用率凭空变好看 —— 比不给数字更糟。
    所以那几台单独列出来，且不参与分母。
    """
    from .domain.ports.generate import generate_links
    from .domain.ports.ledger import ledger_from_cabling

    intent = _load_intent_source(args)
    cabling = generate_links(intent)

    errors = [i for i in cabling.issues if i.severity == "error"]
    if errors:
        print(f"❌ 意图有 {len(errors)} 个错误，先修好再查台账")
        for i in errors[:20]:
            print(f"  {i.code}: {i.message}")
        return EXIT_BLOCKER

    ledger = ledger_from_cabling(cabling, library=_model_library(args))

    print(f"项目：{intent.project}")
    print(
        f"连线 {len(cabling)} 条 → 设备 {ledger.device_count} 台 / "
        f"端口条目 {len(ledger.entries)}"
    )
    print(
        f"  已用 {ledger.used_total} / 容量 {ledger.capacity_total} / "
        f"空闲 {ledger.free_total}"
    )
    if ledger.no_spec_devices:
        print(f"  ⚠️  {len(ledger.no_spec_devices)} 台设备型号没有端口模板（不计入分母）")
        for dev in ledger.no_spec_devices[:5]:
            print(f"     {dev.describe()}")
    if ledger.over_used:
        print(f"  ❌ {len(ledger.over_used)} 台设备端口用量超出模板")
    if ledger.with_unknown_ports:
        print(f"  ⚠️  {len(ledger.with_unknown_ports)} 台设备出现模板里没有的端口")

    if args.device:
        target = args.device
        hits = [
            d
            for d in ledger.devices
            if d.device_label == target or d.device_key == target or target in d.device_label
        ]
        if not hits:
            print(f"\n找不到设备「{target}」。可用 --list 看全部设备名。")
            return EXIT_USAGE
        print()
        for dev in hits:
            print(dev.describe())
            for entry in dev.used:
                peer = f" → {entry.peer_device} {entry.peer_port}" if entry.peer_device else ""
                print(f"  {entry.physical_name:14s} {entry.role:9s} {entry.cable_type:6s}{peer}")
            if dev.free:
                shown = dev.free[: args.limit or 12]
                print(f"  空闲口（共 {dev.free_count} 个，示 {len(shown)}）: {shown}")
    elif args.list:
        print()
        print(f"  {'设备':32s} {'位置':14s} {'已用/容量':>12s}  利用率")
        for dev in ledger.devices[: args.limit or 40]:
            print(
                f"  {dev.device_label[:32]:32s} "
                f"{dev.channel_no}-{dev.rack_no}-{dev.u_start}U".ljust(14)
                + f" {dev.used_count:>5d}/{dev.total_count:<6d}"
                + ("     —" if dev.ratio is None else f"  {dev.ratio * 100:5.1f}%")
            )
        if len(ledger.devices) > (args.limit or 40):
            print(f"  …… 共 {len(ledger.devices)} 台，用 --limit 调整")

    if args.out:
        from .application.services.portledger import ledger_payload

        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        payload = ledger_payload(ledger, include_devices=True, limit=len(ledger.devices))
        out.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")
        print(f"\n台账已写出：{out}")
    return EXIT_OK


# --------------------------------------------------------------------------
# 项目持久化（v0.4）
# --------------------------------------------------------------------------

def _db_path(args: argparse.Namespace) -> Path:
    """数据库位置。

    ## 为什么默认放在仓库的 `data/` 而不是 `~/.idcbox`

    这是个**项目级工具**，不是 SaaS 客户端。使用者希望"把整个仓库拷走，
    项目数据也跟着走"（交给同事、交档、搬到另一台机器）。
    放 `~/.idcbox/` 会让数据与代码分离，备份时容易漏掉，
    而这套数据（意图 + 版本历史）是几周人工的成果。

    `IDCBOX_DB` 环境变量可覆盖（服务器部署时指向持久卷）。
    """
    raw = getattr(args, "db", "") or os.environ.get("IDCBOX_DB", "")
    if raw:
        return Path(raw)
    return _repo_root() / "data" / "idcbox.db"


def _project_store(args: argparse.Namespace):
    """装配仓储。建库 + 迁移都在这一次调用里完成。"""
    from .infrastructure.db import create_db_engine, init_schema
    from .infrastructure.db.repositories import ProjectRepository

    engine = create_db_engine(_db_path(args))
    init_schema(engine, note=f"cli {args.command}")
    return ProjectRepository(engine)


def cmd_project_list(args: argparse.Namespace) -> int:
    from .application.services.projects import list_projects

    store = _project_store(args)
    projects = list_projects(store, include_archived=args.all)
    if not projects:
        print(
            "还没有任何项目。用 `idcbox project-save --name <项目名>"
            " --intent <json>` 建一个。"
        )
        return EXIT_OK
    print(f"共 {len(projects)} 个项目（数据库 {_db_path(args)}）：")
    print()
    print(f"  {'项目':20s} {'状态':10s} {'版本':>5s} {'更新于':20s}")
    for p in projects:
        print(f"  {p.name:20s} {p.status:10s} {p.latest_version:>5d} {p.updated_at:20s}")
    return EXIT_OK


def cmd_project_save(args: argparse.Namespace) -> int:
    """意图 → 数据库（版本化保存）。

    ## 为什么 `--intent` 与 `--sheet` 都支持

    与 `generate` 同一套 `_load_intent_source`：用户可能刚在 Excel 里改完
    就直接存库，不该逼他先 `intent-check --out x.json` 再存。

    ## 为什么打印「是否产生新版本」

    用户连点保存时如果每次都建版本，版本历史很快失去意义。
    明确告诉他"内容没变，仍是 v3"，他就不用反复确认自己有没有存上。
    """
    from .application.services.projects import save_intent

    intent = _load_intent_source(args)
    store = _project_store(args)
    saved = save_intent(
        store,
        project_name=args.name or intent.project,
        intent=intent,
        note=args.note or None,
        site_key=intent.site,
        source_path=getattr(args, "source", "") or None,
        force_new_version=args.force,
    )
    if saved.is_new:
        print(f"已保存：项目「{saved.project}」→ v{saved.version}")
    else:
        print(
            f"内容未变化：项目「{saved.project}」仍是 v{saved.version}"
            "（用 --force 可强制留一个版本快照）"
        )
    s = saved.summary
    print(
        f"  意图 {s.get('total', 0)} 行"
        f"（布局 {s.get('racks', 0)} / 交换机 {s.get('switches', 0)} /"
        f" 特殊设备 {s.get('devices', 0)} / 路由 {s.get('routes', 0)} /"
        f" 交换机组 {s.get('chains', 0)} / 显式连线 {s.get('explicitLinks', 0)}）"
        f" · {s.get('serverCount', 0)} 台服务器"
    )
    if saved.note:
        print(f"  备注：{saved.note}")
    return EXIT_OK


def cmd_project_versions(args: argparse.Namespace) -> int:
    from .application.services.projects import list_versions

    store = _project_store(args)
    versions = list_versions(store, project_name=args.name)
    if not versions:
        print(f"项目「{args.name}」还没有保存过任何意图版本。")
        return EXIT_OK
    print(f"项目「{args.name}」共 {len(versions)} 个版本：")
    print()
    print(f"  {'版本':>5s} {'保存于':20s} {'意图行数':>8s} {'服务器':>7s}  备注")
    for v in versions:
        total = v.summary.get("total", 0)
        servers = v.summary.get("serverCount", 0)
        print(
            f"  v{v.version:<4d} {v.created_at:20s} {total:>8d} {servers:>7d}"
            f"  {v.note or ''}"
        )
    return EXIT_OK


def cmd_project_show(args: argparse.Namespace) -> int:
    """读回某版本意图（可转成 JSON 或六段 Excel，供人看/继续改）。"""
    from .application.services.projects import load_intent_version
    from .domain.ports.intent import dump_intent

    store = _project_store(args)
    intent = load_intent_version(
        store, project_name=args.name, version=args.version or None
    )

    total = (
        len(intent.racks) + len(intent.switches) + len(intent.devices)
        + len(intent.routes) + len(intent.chains) + len(intent.explicit_links)
    )
    label = "最新版" if not args.version else f"v{args.version}"
    print(f"项目「{args.name}」{label}：")
    print(
        f"  布局 {len(intent.racks)}，交换机 {len(intent.switches)}，"
        f"特殊设备 {len(intent.devices)}，路由 {len(intent.routes)}，"
        f"交换机组 {len(intent.chains)}，显式连线 {len(intent.explicit_links)}"
        f"（合计 {total} 行）"
    )
    print(f"  按布局展开 {intent.server_count} 台服务器")

    if args.out:
        out = Path(args.out)
        out.parent.mkdir(parents=True, exist_ok=True)
        if out.suffix.lower() in (".xlsx", ".xlsm"):
            from .infrastructure.xlsx.intent_sheet import write_intent_xlsx

            write_intent_xlsx(intent, out)
            print(f"  已导出六段表格 → {out}")
        else:
            dump_intent(intent, out)
            print(f"  已导出 JSON → {out}")
    return EXIT_OK


def cmd_project_diff(args: argparse.Namespace) -> int:
    """比两个版本 —— 「上次改了什么」。"""
    from .application.services.projects import compare_versions

    store = _project_store(args)
    result = compare_versions(
        store,
        project_name=args.name,
        from_version=args.from_version,
        to_version=args.to_version,
    )
    if result["totalChanges"] == 0:
        print(
            f"v{result['fromVersion']} 与 v{result['toVersion']} 内容完全相同"
            "（可能是 --force 留下的快照）。"
        )
        return EXIT_OK

    limit = args.limit or 10
    print(
        f"项目「{result['project']}」"
        f"v{result['fromVersion']} → v{result['toVersion']}："
        f"{result['totalChanges']} 处变化"
    )
    for sec in result["sections"]:
        if sec["isEmpty"]:
            continue
        print()
        print(f"  【{sec['label']}】{sec['count']} 处")
        for item in sec["added"][:limit]:
            print(f"    + {_brief(item)}")
        if len(sec["added"]) > limit:
            print(f"    … 另有 {len(sec['added']) - limit} 条新增")
        for item in sec["removed"][:limit]:
            print(f"    - {_brief(item)}")
        if len(sec["removed"]) > limit:
            print(f"    … 另有 {len(sec['removed']) - limit} 条删除")
        # `compare_versions` 已把 changed 转成 {"before":…, "after":…}；
        # 这里按字典取，不要再当元组解包（曾因此 AttributeError: 'str'）
        for pair in sec["changed"][:limit]:
            print(f"    ~ {_brief(pair['before'], pair['after'])}")
        if len(sec["changed"]) > limit:
            print(f"    … 另有 {len(sec['changed']) - limit} 条修改")
    return EXIT_OK


def _brief(item: dict, after: dict | None = None) -> str:
    """一条记录的一行摘要。

    优先用"对人最有辨识度"的字段：设备用 `instance`，路由用三元组，
    显式连线用两端插口。比打印整个 dict 可读得多。
    """
    if after is not None:
        diffs = [
            f"{k}: {item.get(k)!r}→{after.get(k)!r}"
            for k in after
            if item.get(k) != after.get(k)
        ]
        label = item.get("instance") or item.get("nodeModel") or item.get("aDevice") or ""
        return f"{label}  {', '.join(diffs)}"
    if "instance" in item:
        bits = [str(item.get("instance"))]
        for k in ("model", "channel", "rack", "u"):
            if item.get(k) not in (None, ""):
                bits.append(f"{k}={item[k]}")
        return " ".join(bits)
    if "nodeModel" in item:
        return f"{item.get('nodeModel')} · {item.get('nic')} → {item.get('switchModel')}"
    if "aDevice" in item:
        return (
            f"{item.get('aDevice')}:{item.get('aPort')}"
            f" ↔ {item.get('bDevice')}:{item.get('bPort')}"
        )
    if "channel" in item:
        return (
            f"通道{item.get('channel')} 柜{item.get('from')}..{item.get('to')}"
            f" = {item.get('model')}"
        )
    return str(item)[:80]


def cmd_export(args: argparse.Namespace) -> int:
    from .infrastructure.xlsx.exporter import write_xlsx

    checked, _ = _build_project(args)
    _print_banner(checked)

    rows = _build_export_rows(checked)
    out = Path(args.out)
    path = write_xlsx(rows, out, mode=args.mode, sheet_name=args.cabling_sheet)
    print(f"已导出 {len(rows)} 行 × 24 列 → {path}（mode={args.mode}）")
    return EXIT_BLOCKER if checked.report.blocker_count else EXIT_OK


def cmd_verify(args: argparse.Namespace) -> int:
    """验收铁线：导出后逐格 diff，必须 0 差异。"""
    import openpyxl

    from .domain.cable_length.numeric import excel15
    from .domain.exporting.contract25 import CONTRACT_25, HEADER_ROW
    from .infrastructure.xlsx.exporter import write_xlsx
    from .infrastructure.xlsx.openpyxl_reader import read_v13_cabling

    checked, _ = _build_project(args)
    _print_banner(checked)

    rows = _build_export_rows(checked)
    out = Path(args.out)
    write_xlsx(rows, out, mode="value", sheet_name=args.cabling_sheet)

    src_sheet = read_v13_cabling(_resolve_source(args.source), args.cabling_sheet)
    wb = openpyxl.load_workbook(out, data_only=True)
    ws = wb[args.cabling_sheet]

    diffs: list[tuple[int, int, Any, Any]] = []
    for offset, raw_row in enumerate(src_sheet.rows):
        r = HEADER_ROW + 1 + offset
        for spec in CONTRACT_25:
            expected = raw_row.values[spec.index] if spec.index < len(raw_row.values) else None
            got = ws.cell(row=r, column=spec.index + 1).value
            if _norm(expected) != _norm(got):
                diffs.append((raw_row.row_no, spec.index, expected, got))

    cells = len(src_sheet.rows) * len(CONTRACT_25)
    if diffs:
        print()
        print(f"❌ 导出保真验证失败：{cells} 格中发现 {len(diffs)} 格差异")
        for row_no, col, exp, got in diffs[:20]:
            print(f"  r{row_no} 列{col}: 期望 {exp!r} 实得 {got!r}")
        return EXIT_MISMATCH

    print()
    print(f"逐格比较：{cells} 格，差异 0 格")
    print("✅ 导出保真验证通过：24 列逐格完全一致（含 2 个同名 U位 列、sentinel 文本）")
    return EXIT_BLOCKER if checked.report.blocker_count else EXIT_OK


def _norm(v: Any) -> Any:
    from .domain.cable_length.numeric import excel15

    if v is None:
        return None
    if isinstance(v, bool):
        return v
    if isinstance(v, (int, float)):
        x = excel15(float(v))
        return int(x) if float(x).is_integer() else x
    s = str(v)
    return s if s.strip() else None


# --------------------------------------------------------------------------
# 参数
# --------------------------------------------------------------------------


def build_parser() -> argparse.ArgumentParser:
    shared = argparse.ArgumentParser(add_help=False)
    shared.add_argument(
        "--source", default=DEFAULT_SOURCE, help=f"源 Excel 路径（默认 {DEFAULT_SOURCE}）"
    )
    shared.add_argument("--floor-sheet", default="机房平面图", help="平面图 sheet 名")
    shared.add_argument("--cabling-sheet", default="布线表", help="布线表 sheet 名")
    shared.add_argument(
        "--empirical",
        default="rules/len_empirical_v1.json",
        help="经验表 JSON 路径（默认 rules/len_empirical_v1.json）",
    )
    shared.add_argument(
        "--continuous", action="store_true", help="用修正后的连续阶梯（默认逐字复刻 Excel IFS）"
    )
    shared.add_argument("--limit", type=int, default=0, help="明细打印条数（0 = 不打印）")

    ap = argparse.ArgumentParser(
        prog="idcbox",
        description="IDCBoxLine —— 机房上架与布线规划工具（v0.1 止血+生成版）",
    )
    sub = ap.add_subparsers(dest="command", required=True)

    p_import = sub.add_parser("import", parents=[shared], help="读入并体检（不改文件）")
    p_import.set_defaults(func=cmd_import)

    p_check = sub.add_parser("check", parents=[shared], help="体检并输出 Markdown 报告")
    p_check.add_argument("--out", default="", help="报告输出路径（.md）")
    p_check.set_defaults(func=cmd_check)

    p_export = sub.add_parser("export", parents=[shared], help="计算并导出 25 列布线表")
    p_export.add_argument("--out", required=True, help="输出 .xlsx 路径")
    p_export.add_argument(
        "--mode", choices=("formula", "value"), default="formula",
        help="formula=输出可编辑公式（默认）｜value=纯值归档",
    )
    p_export.set_defaults(func=cmd_export)

    p_gen = sub.add_parser(
        "generate", parents=[shared], help="意图 → 布线表（v0.3 主功能）"
    )
    p_gen.add_argument("--intent", default="", help="意图 JSON 路径")
    p_gen.add_argument("--sheet", default="", help="意图六段 Excel 路径（替代 --intent）")
    p_gen.add_argument("--project", default="", help="项目名（用 --sheet 时，默认取文件名）")
    p_gen.add_argument("--out", required=True, help="输出 .xlsx 路径")
    p_gen.add_argument(
        "--floor", default="", help="平面图来源 Excel（默认与 --source 同一个文件）"
    )
    p_gen.set_defaults(func=cmd_generate)

    p_iex = sub.add_parser(
        "intent-export", parents=[shared], help="意图 JSON → 六段 Excel 模板（给人填）"
    )
    p_iex.add_argument("--intent", required=True, help="意图 JSON 路径")
    p_iex.add_argument("--out", required=True, help="输出 .xlsx 路径")
    p_iex.set_defaults(func=cmd_intent_export)

    p_ich = sub.add_parser(
        "intent-check", parents=[shared], help="检查意图表格能否读通（不生成）"
    )
    p_ich.add_argument("--sheet", required=True, help="意图六段 Excel 路径")
    p_ich.add_argument("--project", default="", help="项目名（默认取文件名）")
    p_ich.add_argument("--site", default="", help="机房/区域标签")
    p_ich.add_argument("--out", default="", help="同时转成 JSON 输出到此路径")
    p_ich.set_defaults(func=cmd_intent_check)

    p_cons = sub.add_parser(
        "consumables", parents=[shared], help="耗材与模块需求清单（采购用）"
    )
    p_cons.add_argument(
        "--phase", default="", help="只看某个批次（如 首期）；不给则是全项目口径"
    )
    p_cons.add_argument("--out", default="", help="把清单另存为 JSON（可选）")
    p_cons.set_defaults(func=cmd_consumables)

    p_dec = sub.add_parser(
        "decompose", parents=[shared], help="把意图分解进规范表（v0.6，落库）"
    )
    p_dec.add_argument("--intent", default="", help="意图 JSON 路径")
    p_dec.add_argument("--sheet", default="", help="意图六段 Excel 路径")
    p_dec.add_argument("--project", default="", help="用 --sheet 时的项目名")
    p_dec.add_argument("--site", default="", help="机房/区域标签")
    p_dec.add_argument(
        "--name", default="",
        help="落库到哪个项目（默认取意图的 project 字段；不存在则自动建）",
    )
    p_dec.add_argument(
        "--db", default="", help="数据库路径（默认 <仓库>/data/idcbox.db）"
    )
    p_dec.add_argument(
        "--dry-run", action="store_true", help="只分解不落库，打印各表行数"
    )
    p_dec.add_argument(
        "--stats", action="store_true", help="落库后打印按状态/角色/线缆类型的聚合"
    )
    p_dec.add_argument("--models-json", default="", help="项目型号库 JSON（叠加到内置表）")
    p_dec.add_argument("--models-xlsx", default="", help="Excel 型号库（最高优先级）")
    p_dec.set_defaults(func=cmd_decompose)

    p_mod = sub.add_parser(
        "models", parents=[shared], help="设备型号库：查看 / 导出 / 校验（v0.7）"
    )
    p_mod.add_argument("--models-json", default="", help="项目内 models.json（叠加到内置表）")
    p_mod.add_argument("--models-xlsx", default="", help="Excel 型号库（优先级最高）")
    p_mod.add_argument("--export-json", default="", help="把生效的库导出成 models.json")
    p_mod.add_argument("--export-xlsx", default="", help="把生效的库导出成 Excel 型号库")
    p_mod.add_argument("--check", action="store_true", help="只报问题，不打印全表")
    p_mod.set_defaults(func=cmd_models)

    p_lab = sub.add_parser(
        "labels", parents=[shared], help="线缆标签清单（每端一张贴纸印什么字）"
    )
    p_lab.add_argument("--out", type=Path, help="把结果写成 JSON")
    p_lab.add_argument("--intent", default="", help="意图 JSON 路径")
    p_lab.add_argument("--sheet", default="", help="意图六段 Excel 路径（替代 --intent）")
    p_lab.add_argument("--models-json", type=Path, help="型号库 JSON（覆盖内置模板）")
    p_lab.add_argument(
        "--number-missing",
        action="store_true",
        help="给没有实例名的设备自动编号（SRV01/STG01…）；默认留空并报缺口",
    )
    p_lab.set_defaults(func=cmd_labels)

    p_led = sub.add_parser(
        "ledger", parents=[shared], help="设备台账 + 端口台账（规划复核用）"
    )
    p_led.add_argument("--intent", default="", help="意图 JSON 路径")
    p_led.add_argument("--sheet", default="", help="意图六段 Excel 路径")
    p_led.add_argument("--project", default="", help="用 --sheet 时的项目名")
    p_led.add_argument("--site", default="", help="机房/区域标签")
    p_led.add_argument("--device", default="", help="只看某台设备（名字或位置号）")
    p_led.add_argument("--list", action="store_true", help="列出全部设备用量")
    p_led.add_argument("--out", default="", help="把台账另存为 JSON（可选）")
    p_led.add_argument("--models-json", default="", help="项目型号库 JSON（叠加到内置表）")
    p_led.add_argument("--models-xlsx", default="", help="Excel 型号库（最高优先级）")
    p_led.set_defaults(func=cmd_ledger)

    # ---- 项目持久化（v0.4）----
    dbopt = argparse.ArgumentParser(add_help=False)
    dbopt.add_argument(
        "--db", default="", help="数据库路径（默认 <仓库>/data/idcbox.db）"
    )

    p_pl = sub.add_parser(
        "project-list", parents=[dbopt], help="列出已保存的项目"
    )
    p_pl.add_argument("--all", action="store_true", help="含已归档项目")
    p_pl.set_defaults(func=cmd_project_list)

    p_ps = sub.add_parser(
        "project-save", parents=[shared, dbopt], help="意图 → 数据库（版本化保存）"
    )
    p_ps.add_argument("--name", default="", help="项目名（默认取意图里的 project）")
    p_ps.add_argument("--intent", default="", help="意图 JSON 路径")
    p_ps.add_argument("--sheet", default="", help="意图六段 Excel 路径")
    p_ps.add_argument("--project", default="", help="用 --sheet 时的项目名")
    p_ps.add_argument("--note", default="", help="本次保存的备注（写进版本历史）")
    p_ps.add_argument(
        "--force", action="store_true", help="内容没变也留一个版本快照（里程碑）"
    )
    p_ps.set_defaults(func=cmd_project_save)

    p_pv = sub.add_parser(
        "project-versions", parents=[dbopt], help="列出版本历史"
    )
    p_pv.add_argument("--name", required=True, help="项目名")
    p_pv.set_defaults(func=cmd_project_versions)

    p_pw = sub.add_parser(
        "project-show", parents=[dbopt], help="读回某版本意图（可导出 JSON/Excel）"
    )
    p_pw.add_argument("--name", required=True, help="项目名")
    p_pw.add_argument("--version", type=int, default=0, help="版本号（默认最新）")
    p_pw.add_argument("--out", default="", help="导出到 .json 或 .xlsx")
    p_pw.set_defaults(func=cmd_project_show)

    p_pd = sub.add_parser(
        "project-diff", parents=[dbopt], help="比两个版本（上次改了什么）"
    )
    p_pd.add_argument("--name", required=True, help="项目名")
    p_pd.add_argument("--from", dest="from_version", type=int, required=True, help="起始版本")
    p_pd.add_argument("--to", dest="to_version", type=int, required=True, help="目标版本")
    p_pd.add_argument("--limit", type=int, default=10, help="每类明细打印条数")
    p_pd.set_defaults(func=cmd_project_diff)

    p_verify = sub.add_parser("verify", parents=[shared], help="验收铁线：导出并逐格 diff")
    p_verify.add_argument(
        "--out",
        default=str(Path(__file__).resolve().parent.parent / ".out" / "verify.xlsx"),
        help="临时输出路径",
    )
    p_verify.set_defaults(func=cmd_verify)

    return ap


def main(argv: Sequence[str] | None = None) -> int:
    warnings.filterwarnings("ignore")
    args = build_parser().parse_args(argv)
    try:
        return args.func(args)
    except CliError as exc:
        print(f"{exc}", file=sys.stderr)
        return EXIT_USAGE
    except FileNotFoundError as exc:
        print(f"文件缺失：{exc}", file=sys.stderr)
        return EXIT_USAGE
    except KeyError as exc:
        print(f"sheet 或字段缺失：{exc}", file=sys.stderr)
        return EXIT_USAGE
    except ValueError as exc:
        # 项目名不存在、版本号不存在、意图 JSON 结构坏等用户输入问题。
        # 不打 traceback：用户看到栈只会以为程序崩了，而实际是他打错了名字。
        print(f"{exc}", file=sys.stderr)
        return EXIT_USAGE


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
