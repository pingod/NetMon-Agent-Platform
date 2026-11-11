"""V13 布线表（24 列）→ 域层 LinkInput 的适配。

## 版式事实（实测）

- 表头 24 个（第 25 列表头为 None、仅 12 行有值 → 视作脏列，导出时不写）
- 数据行 5128-1 = **5127 行**，其中 **14 行完全空行** → 有效 **5113 行**
- `U位` 列名出现两次（本端 idx4、对端 idx10）
- `本端通道`/`本端机柜`/`对端通道`/`对端机柜` 是整数
- 长度列 7 个：跨机柜(13) 间隔机柜(14) 机柜数量(15) 机内线缆(16) 柜间线缆(17) 线缆2(18) 线缆长度1(19) 线缆长度(20)
- `备注1`(23) 实测 450 行为 `首期`

## 两个刻意的设计

1. **空行直接丢弃，但记下行号**：导出时按 `source_row_no` 排序，保证与原表顺序一致。
2. **原值一律原样保留**：`线缆长度`(U) 的数值进 `imported_len_u`，
   文本 sentinel（`待定`）进 `imported_len_raw`。二者互斥，绝不互相覆盖。
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Iterable, Sequence

from ...domain.cable_length.link import Endpoint, LinkInput
from ...domain.geometry.layout import RackRef
from ...domain.importing.normalize import (
    is_length_sentinel,
    make_stable_key,
    norm_cable_type,
    norm_int,
    norm_name,
    norm_text,
    norm_u_position,
    parse_length,
)
from ..xlsx.openpyxl_reader import RawRow, RawSheet

#: 列索引（0-based），与 V13_HEADERS 对齐
COL_LOCAL_MODEL = 0
COL_LOCAL_DEVICE = 1
COL_LOCAL_CHANNEL = 2
COL_LOCAL_RACK = 3
COL_LOCAL_U = 4
COL_LOCAL_PORT = 5
COL_PEER_MODEL = 6
COL_PEER_DEVICE = 7
COL_PEER_CHANNEL = 8
COL_PEER_RACK = 9
COL_PEER_U = 10
COL_PEER_PORT = 11
COL_CABLE_TYPE = 12
COL_N_CROSS = 13
COL_O_GAP = 14
COL_P_COUNT = 15
COL_Q_IN_RACK = 16
COL_R_INTER = 17
COL_S_TOTAL = 18
COL_T_LADDER = 19
COL_U_APPROVED = 20
COL_LOCAL_MODULE = 21
COL_PEER_MODULE = 22
COL_REMARK = 23


@dataclass(frozen=True, slots=True)
class ImportSkip:
    """被跳过的行及原因。**必须返回给用户看**，不能静默丢弃。"""

    row_no: int
    reason: str


@dataclass(frozen=True, slots=True)
class ImportPreview:
    """导入的中间产物：连线 + 原值快照 + 跳过清单。"""

    links: tuple[LinkInput, ...]
    #: 原值快照：stable_key → 8 个长度列 + 2 个模块列的原始值（导出保真用）
    raw_values: dict[str, dict[str, object]]
    skipped: tuple[ImportSkip, ...]
    total_rows: int
    empty_rows: int

    @property
    def effective_rows(self) -> int:
        return len(self.links)


def _is_empty_row(values: Sequence[object]) -> bool:
    return all(v is None or (isinstance(v, str) and not v.strip()) for v in values)


def _build_endpoint(
    model: object,
    device: object,
    channel: object,
    rack: object,
    u: object,
    port: object,
) -> Endpoint:
    ch = norm_int(channel)
    rc = norm_int(rack)
    return Endpoint(
        rack=RackRef(channel_no=ch if ch is not None else -1, seq_no=rc if rc is not None else -1),
        device_name=norm_name(device) or norm_name(model),
        port_name=norm_text(port),
        u_position=norm_u_position(u),
        device_model=norm_name(model) or None,
    )


#: 原值快照里存原始单元格元组的键。**导出保真靠它，不靠归一值。**
RAW_ALL_KEY = "__cols__"


def _raw_snapshot(values: Sequence[object]) -> dict[str, object]:
    """保留导出时必须逐字还原的原始值。

    ## 为什么除了 11 个长度列还要存 `__cols__`

    因为归一化是**有损**的：`业务接入交换机 1` 归一化成 `业务接入交换机1`、
    `rack1` 归一化成 `1`。计算与去重需要用归一值（否则同一台设备会被当成两台），
    但导出必须逐字还原原值 —— 用户手里的 Excel 里是带空格的，
    我们导出的文件却不带，那这份文件就不是他的那份文件的等价物。

    所以两份都存：`values` 给计算，`__cols__` 给导出。
    这不是冗余，是两种用途的不同投影。
    """
    return {
        RAW_ALL_KEY: tuple(values),
        "n_cross": values[COL_N_CROSS],
        "o_gap": values[COL_O_GAP],
        "p_count": values[COL_P_COUNT],
        "q_in_rack": values[COL_Q_IN_RACK],
        "r_inter": values[COL_R_INTER],
        "s_total": values[COL_S_TOTAL],
        "t_ladder": values[COL_T_LADDER],
        "u_approved": values[COL_U_APPROVED],
        "local_module": values[COL_LOCAL_MODULE],
        "peer_module": values[COL_PEER_MODULE],
        "remark": values[COL_REMARK],
    }


def adapt_rows(rows: Iterable[RawRow]) -> ImportPreview:
    """把原始行适配成域层输入。空行静默丢弃（计入 `empty_rows`），
    其他问题行保留并进 `skipped` 说明 —— 但**只要能定位两端就保留**。
    """
    links: list[LinkInput] = []
    raw_values: dict[str, dict[str, object]] = {}
    skipped: list[ImportSkip] = []
    total = 0
    empty = 0
    seen_keys: dict[str, int] = {}

    for row in rows:
        total += 1
        v = row.values
        if _is_empty_row(v):
            empty += 1
            continue

        a = _build_endpoint(
            v[COL_LOCAL_MODEL], v[COL_LOCAL_DEVICE], v[COL_LOCAL_CHANNEL],
            v[COL_LOCAL_RACK], v[COL_LOCAL_U], v[COL_LOCAL_PORT],
        )
        b = _build_endpoint(
            v[COL_PEER_MODEL], v[COL_PEER_DEVICE], v[COL_PEER_CHANNEL],
            v[COL_PEER_RACK], v[COL_PEER_U], v[COL_PEER_PORT],
        )

        if a.rack.channel_no < 0 or b.rack.channel_no < 0:
            skipped.append(ImportSkip(row.row_no, "本端或对端的通道号缺失，无法定位机柜"))
            continue

        cable_type = norm_cable_type(v[COL_CABLE_TYPE])
        u_val, u_raw = parse_length(v[COL_U_APPROVED])
        if u_raw is not None and is_length_sentinel(u_raw):
            # sentinel 文本：原样保留到 imported_len_raw
            pass

        base_key = make_stable_key(
            a.device_name, a.port_name, b.device_name, b.port_name, cable_type
        )
        n = seen_keys.get(base_key, 0)
        seen_keys[base_key] = n + 1
        key = base_key if n == 0 else f"{base_key}#{n + 1}"

        links.append(
            LinkInput(
                stable_key=key,
                a=a,
                b=b,
                cable_type=cable_type,
                source_row_no=row.row_no,
                imported_len_u=u_val,
                imported_len_raw=None if u_val is not None else u_raw,
                remark=norm_text(v[COL_REMARK]) or None,
                phase=norm_text(v[COL_REMARK]) or None,
                extra={
                    "local_u": v[COL_LOCAL_U],
                    "peer_u": v[COL_PEER_U],
                },
            )
        )
        raw_values[key] = _raw_snapshot(v)

    return ImportPreview(
        links=tuple(links),
        raw_values=raw_values,
        skipped=tuple(skipped),
        total_rows=total,
        empty_rows=empty,
    )


def adapt_sheet(sheet: RawSheet) -> ImportPreview:
    return adapt_rows(sheet.rows)
