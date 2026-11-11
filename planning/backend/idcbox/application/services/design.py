"""设计态服务：意图 → 布线表，以及意图与存量表的比对。

## 这一层为什么存在（不只是"转发 domain"）

`domain/ports/generate.py` 是纯函数：给意图，出 4430 条 `GeneratedLink`。
但界面与 API 需要的是**能直接渲染的 JSON**，而且要回答三个用户问题：

1. **生成的行长什么样** —— 字段要摊平成前端表格能吃的形状；
2. **和手上的 Excel 差在哪** —— 用户已有 V13 布线表，他要的是"我改了什么"；
3. **哪些不在自动生成范围** —— 交换机之间的互联约 11%，不说清会让人
   以为 4430 行就是全部。

把这三件事放在 application 层，是为了让 domain 保持零依赖纯逻辑，
也让 CLI / API / 未来的写接口共用同一份口径 —— 否则 CLI 打印一套、
API 返回另一套，用户对不上账。
"""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass
from typing import Any

from ...domain.cable_length.engine import compute_batch
from ...domain.cable_length.rules import DEFAULT_RULE
from ...domain.geometry.layout import GeometrySnapshot
from ...domain.ports.bridge import to_link_inputs
from ...domain.ports.generate import GeneratedCabling, generate_links
from ...domain.ports.intent import Intent
from ...domain.ports.ledger import PortLedger, ledger_from_cabling
from .portledger import ledger_payload


@dataclass(frozen=True, slots=True)
class DesignResult:
    """一次生成的全部产物（不含落库，v0.3 仍是内存态）。"""

    cabling: GeneratedCabling
    rows: tuple[dict[str, Any], ...]
    stats: dict[str, Any]
    notes: tuple[str, ...]
    #: 端口台账（设备→端口→连线的三层视图）。生成时顺手建一次，
    #: 不额外读 Excel —— 台账的输入就是刚生成的这 5127 条线。
    ledger: PortLedger | None = None


#: 生成行摊平成表格时的列顺序（与 24 列契约前 13 列同序）。
GENERATE_COLUMNS: tuple[tuple[str, str], ...] = (
    ("localModel", "本端设备"),
    ("localDevice", "本端设备名称"),
    ("localChannel", "本端通道"),
    ("localRack", "本端机柜"),
    ("localU", "U位"),
    ("localPort", "本端接口"),
    ("peerModel", "对端设备"),
    ("peerDevice", "对端设备名称"),
    ("peerChannel", "对端通道"),
    ("peerRack", "对端机柜"),
    ("peerU", "U位"),
    ("peerPort", "对端接口"),
    ("cableType", "线缆类型"),
    ("nic", "网口"),
)


def generate_design(
    intent: Intent,
    geometry: GeometrySnapshot | None = None,
    *,
    library: Any = None,
) -> DesignResult:
    """意图 → 布线表行 + 长度。`geometry` 为 None 时只出连线不算长度。

    为什么允许不算长度：界面上的「生成预览」要先让用户看到**接得对不对**，
    长度是第二位。而算长度需要平面图几何（依赖一份 Excel），
    API 层拿不到源文件时不该整页报错。
    """
    cabling = generate_links(intent)

    lengths: dict[str, Any] = {}
    if geometry is not None and cabling.links:
        links, _bridge_notes = to_link_inputs(cabling)
        batch = compute_batch(links, DEFAULT_RULE, geometry)
        lengths = {r.stable_key: r for r in batch.results}
        computed = batch.stats.computed_rows
        error_rows = batch.stats.error_rows
    else:
        links = []
        computed = 0
        error_rows = 0

    rows: list[dict[str, Any]] = []
    for idx, gen in enumerate(cabling.links):
        row: dict[str, Any] = {
            "rowNo": idx + 1,
            "localModel": gen.a_model,
            "localDevice": gen.a_device,
            "localChannel": gen.a_channel,
            "localRack": gen.a_rack,
            "localU": gen.a_u,
            "localPort": gen.a_port,
            "peerModel": gen.b_model,
            # 对端设备名称**永远为空**：源表这一列只填了 332/4562 行，
            # 且存在型号与名字矛盾的记录。设备名属上架命名规范，不是接线意图。
            "peerDevice": gen.b_device,
            "peerChannel": gen.b_channel,
            "peerRack": gen.b_rack,
            "peerU": gen.b_u,
            "peerPort": gen.b_port,
            "cableType": gen.cable_type,
            "nic": gen.nic,
            # 设备标识：界面按它做「点一台设备看它的所有连线」
            "peerId": gen.b_id,
        }
        if links:
            res = lengths.get(links[idx].stable_key)
            if res is not None:
                row["nCrossRack"] = res.n_cross_rack
                row["oGapRack"] = res.o_gap_rack
                row["pRackCount"] = res.p_rack_count
                row["qInRackM"] = res.q_in_rack_m
                row["rInterRackM"] = res.r_inter_rack_m
                row["sTotalM"] = res.s_total_m
                row["tLadderM"] = res.t_ladder_m
                row["confidence"] = res.confidence
                row["oClass"] = res.o_class
                # U 列（核定长度）**不填** —— 那是人工签字的位置
                row["uApprovedM"] = None
        rows.append(row)

    stats = {
        "links": len(cabling.links),
        "errors": cabling.error_count,
        "warns": cabling.warn_count,
        "byNic": [{"nic": nic, "count": n} for nic, n in cabling.by_nic],
        #: 显式连线按对端端口名分布（`100GE1/0/1` 这类）。与 `byNic` 分开的理由见
        #: `GeneratedCabling.explicit_by_port`。
        "explicitByPort": [
            {"port": port, "count": n} for port, n in cabling.explicit_by_port
        ],
        "lengthComputed": computed,
        "lengthError": error_rows,
        "geometryUsed": geometry is not None,
        #: 按问题代码分组的计数。界面靠它把「21 条槽位冲突」和
        #: 「3 条线缆类型不明」分开显示 —— 两类问题的处置方式完全不同，
        #: 混成一个 `warns: 24` 等于没告诉用户该干什么。
        "issuesByCode": dict(
            sorted(Counter(i.code.value for i in cabling.issues).items())
        ),
    }

    return DesignResult(
        cabling=cabling,
        rows=tuple(rows),
        stats=stats,
        notes=cabling.notes,
        # 台账直接从刚生成的连线建 —— 不再读一遍 Excel。
        # 5000 条线建一次约 40ms，比让界面每次查端口利用率都重算便宜得多。
        ledger=ledger_from_cabling(cabling, library=library) if cabling.links else None,
    )


# ---------------------------------------------------------------------------
# 与存量布线表比对
# ---------------------------------------------------------------------------
def compare_with_stock(
    design: DesignResult,
    stock_rows: list[dict[str, Any]],
    *,
    limit: int = 200,
    peer_model_of: dict[tuple[str, str, str], str] | None = None,
) -> dict[str, Any]:
    """生成结果 vs 存量布线表：新增 / 缺失 / 长度差异。

    ## 比对键是 `(交换机实例, 交换机端口, 服务器网口)`

    三个都在两侧恒定非空：
    - 交换机实例 → 生成侧 `a_device`，存量侧 `link.a.device_name`；
    - 交换机端口 → `10GE1/0/1` 这类 CLI 名，两侧同源；
    - 服务器网口 → `万兆1` 这类标签名，生成侧是 `b_port`，存量侧是 `link.b.port_name`。

    为什么不用「对端设备名称」：那一列存量表**半填**（4562 行里只有 332 行
    有值，且存在型号与名字矛盾），拿它进键会让 200 行假性未命中 —— 而
    200 这个数字恰好等于生成侧不填名的行数，看起来像"算法漏了 200 行"，
    实际是两套口径。

    ## 返回的长度差异只统计「两边都有值」的行

    存量表 U 列（核定长度）有大量空白，生成结果 U 列恒为 None。
    拿 None 和 5 比会产出 4430 条假差异 —— 用户看到就再也不信这个功能了。

    ## 为什么缺失行必须按「对端型号」归因后再返回

    实测 144 行缺失（3.1%）全部来自特殊机型：网络功能服务器 48、本地备份 28、
    GSO 24、管理节点 15、密评设备 12、负载均衡 4、防火墙 4 等。
    这些机型每台一种接法（同型号不同实例接不同交换机），规则无法推导。

    如果只返回 144 这个数字，用户会理解成"算法漏了 144 行"，
    于是不信任整份结果。归因后他知道**这 144 行本来就得手写**，
    剩下 4430 行才是机器替他的部分。
    """
    gen_by_key: dict[tuple[str, str, str], dict[str, Any]] = {}
    for row in design.rows:
        gen_by_key[(row["localDevice"], row["localPort"], row["nic"])] = row

    stock_by_key: dict[tuple[str, str, str], dict[str, Any]] = {}
    for row in stock_rows:
        # 存量行（`main._stock_rows_for_compare` 的产物）三个字段名与生成行一致，
        # 唯一要小心的是 `localDevice`：生成侧它就是交换机实例名，
        # 而 `/api/cables` 的 `localDevice` 是**型号**。所以存量行走
        # `localDeviceName` 优先，避免 4430 行全被算成"新增 + 缺失"。
        switch = str(row.get("localDeviceName") or row.get("localDevice") or "")
        key = (switch, str(row.get("localPort") or ""), str(row.get("nic") or ""))
        stock_by_key[key] = row

    added = [k for k in gen_by_key if k not in stock_by_key]
    missing = [k for k in stock_by_key if k not in gen_by_key]
    common = [k for k in gen_by_key if k in stock_by_key]

    length_diffs: list[dict[str, Any]] = []
    #: 长度比对的**分母**：存量表 T 列（线缆长度1）有数值的行数。
    #:
    #: ## 为什么必须单独返回这个数
    #:
    #: 源表 5127 行里，T 列真正带公式（`_xlfn.IFS` 阶梯）的只有 **636 行**，
    #: 其余行那格本来就空白 —— 策划只给「需要精确长度」的行填了公式。
    #: 生成器对 5127 行全部算出长度，是超集。
    #:
    #: 如果界面只说「5127 行里 55 行不一致」，用户会以为准确率 98.9%；
    #: 说「636 行里 55 行不一致」，用户才知道要看的是那 636 行的人工输入。
    #: 两个说法描述同一批数据，但对产品的意义完全不同 —— 所以分母必须传出去。
    length_baseline = 0
    #: N/O/Q 三个人工输入列与几何完全一致、且公式链也复现的行数。
    #:
    #: 这个数字是对外解释「公式有没有误差」的唯一依据：
    #: 实测 **578 / 578**（即 clean 行里 T 列 100% 对上）。
    #: 如果只报「636 行里 55 行不一致」，用户会怀疑公式；给出 clean 口径，
    #: 用户才能理解「公式没问题，是那 55 行的人工输入与几何冲突」。
    length_clean = 0
    length_lucky = 0
    for k in common:
        g_row, s_row = gen_by_key[k], stock_by_key[k]
        g = g_row.get("tLadderM")
        st = s_row.get("tLadderM")
        if st is None:
            # 存量侧没有值 = 原表那格空白，不进分母（也谈不上"不一致"）
            continue
        length_baseline += 1
        if g is None:
            continue
        diffs = int(g) != int(st)
        if diffs:
            length_diffs.append(
                {
                    "key": list(k),
                    "generated": int(g),
                    "stock": int(st),
                }
            )
        # 人工输入全一致时，四个公式列必须全中 —— 这条不成立就是算法退化了
        inputs_match = all(
            s_row.get(sf) is None
            or g_row.get(gf) is None
            or int(g_row[gf]) == int(s_row[sf])
            for sf, gf in (
                ("nCrossRaw", "nCrossRack"),
                ("oGapRaw", "oGapRack"),
                ("qInRackRaw", "qInRackM"),
            )
        )
        if inputs_match and not diffs:
            length_clean += 1
        elif not inputs_match and not diffs:
            #: 人工输入填错、但错误恰好与几何抵消，T 反而对上的行（实测 3）。
            #: 必须单独计数：它们看着"没问题"，实际那三格输入仍是错的。
            length_lucky += 1

    missing_by_model: Counter[str] = Counter()
    if peer_model_of:
        for k in missing:
            missing_by_model[peer_model_of.get(k, "（未知型号）")] += 1

    return {
        "generatedRows": len(gen_by_key),
        "stockRows": len(stock_by_key),
        "added": len(added),
        "missing": len(missing),
        "common": len(common),
        #: 长度比对的分母（存量表 T 列有数值的行数）。解释见上方 `length_baseline`。
        "lengthBaseline": length_baseline,
        #: 人工输入一致且公式复现的行数（实测 578）。界面用它讲清「公式无误差」。
        "lengthClean": length_clean,
        #: 人工输入填错但结果凑巧对上的行数（实测 3）。它与 `lengthClean` 加起来
        #: 不等于 `lengthBaseline`，差额才是真正的 `lengthDiffs` —— 界面上
        #: 三个数摆出来，用户能自己验算：578 + 3 + 55 = 636。
        "lengthLucky": length_lucky,
        "lengthDiffs": len(length_diffs),
        "addedSamples": [list(k) for k in added[:limit]],
        "missingSamples": [list(k) for k in missing[:limit]],
        "lengthDiffSamples": length_diffs[:limit],
        "missingByModel": [
            {"model": m, "count": n} for m, n in missing_by_model.most_common()
        ],
    }


def intent_volume(intent: Intent, *, source_format: str = "") -> dict[str, int | str]:
    """意图体积统计 —— 这是产品最需要讲的数字：几十行 vs 几千行。

    ## 为什么 `devices` 必须计入 total

    特殊设备（网络功能服务器/防火墙/GSO 等 34 台）也是用户一行一行填的，
    漏掉它们会让 total 少 34 行（915 而非 949）。数字少报了看着"更好看"，
    但那是在骗用户 —— 他按 total 预估填表工作量，实际要多填 34 行。

    ## `sourceFormat` 是给用户看的「我改的是哪份文件」

    意图可以是 JSON 也可以是六段 Excel。用户改完 Excel 忘了重新导出 JSON
    时，界面显示的还是旧结果，而**界面不会报错**。把来源格式与文件名
    摆出来，至少能让他发现"咦，这里写的还是 json"。
    """
    counts = {
        "racks": len(intent.racks),
        "switches": len(intent.switches),
        "devices": len(intent.devices),
        "routes": len(intent.routes),
        "chains": len(intent.chains),
        "explicitLinks": len(intent.explicit_links),
    }
    return {
        **counts,
        "total": sum(counts.values()),
        "serverCount": intent.server_count,
        "sourceFormat": source_format or "json",
    }


def design_payload(
    intent: Intent,
    geometry: GeometrySnapshot | None = None,
    *,
    stock_rows: list[dict[str, Any]] | None = None,
    peer_model_of: dict[tuple[str, str, str], str] | None = None,
    limit: int = 200,
    source_format: str = "",
    library: Any = None,
) -> dict[str, Any]:
    """给前端的完整载荷：体积 + 生成行（分页）+ 统计 + 与存量比对。

    `library` 是项目型号库；不给则用内置 26 个型号。传它才能让项目里
    的自研设备在台账中算出端口容量。
    """
    design = generate_design(intent, geometry, library=library)
    by_model = Counter(row["localModel"] for row in design.rows)
    #: 覆盖了多少**台交换机**。必须数交换机实例（`localDevice`），
    #: 早先误用 `localRack`（机柜号）做键，得到 25 这个数 —— 机柜跨通道重号，
    #: 而真实是 106 台交换机。这种"看起来合理但语义错"的数字最伤人。
    by_switch = {row["localDevice"] for row in design.rows}

    payload: dict[str, Any] = {
        "intent": intent_volume(intent, source_format=source_format),
        "project": intent.project,
        "site": intent.site,
        "stats": design.stats,
        "notes": list(design.notes),
        "columns": [{"id": cid, "label": label} for cid, label in GENERATE_COLUMNS],
        "rows": list(design.rows[:limit]),
        "rowsTotal": len(design.rows),
        "byModel": [{"model": m, "count": n} for m, n in by_model.most_common()],
        "bySwitchCount": len(by_switch),
        "switchModels": sorted({row["localModel"] for row in design.rows}),
        "issues": [
            {
                "code": i.code,
                "severity": i.severity,
                "message": i.message,
                "field": i.field,
            }
            for i in design.cabling.issues[:limit]
        ],
    }
    if stock_rows is not None:
        payload["compare"] = compare_with_stock(
            design, stock_rows, limit=limit, peer_model_of=peer_model_of
        )
    #: 端口台账摘要。放在生成载荷里而不是单独一个接口，
    #: 是因为它和 `rows` 出自同一次生成 —— 分开请求会出现
    #: 「表格说 5127 行、台账按另一次生成算」的两份真相。
    if design.ledger is not None:
        payload["ledger"] = ledger_payload(
            design.ledger, include_devices=False, limit=0
        )
    return payload
