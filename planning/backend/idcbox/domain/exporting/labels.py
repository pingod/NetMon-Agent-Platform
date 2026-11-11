"""线缆标签清单（纯函数，零 IO）。

## 这个模块解决什么

布线表说「这根线两端接哪儿」，标签回答**这张贴纸印什么字**。
现场流程是：按表放线 → 在两端贴标签 → 打线的人按标签找口。
标签打错等于线接错，而且错在两头都在几米外的机柜里 —— 事后核对成本极高。

## 真实标签格式（从 `11+12冷通道（示例）.xlsx` 反解，2844/2846 行匹配）

```
11A14-16U  HLW-YWHX01(SW)  40GE2/0/1
└─位置───┘  └─设备编码─┘  └─端口──┘
```

三段之间用**空格**分隔（不是 tab、不是横线）。逐段说明：

| 段 | 取值 | 来源 |
|---|---|---|
| 位置 | `{机柜标识}-{U}U`，如 `11A14-16U` | 机柜的施工标识 + 设备起始 U 位 |
| 设备编码 | `{站点}-{角色缩写}{序号}`，可带 `(SW)` 后缀 | 台账里的设备编码 |
| 端口 | `40GE2/0/1` / `10GE1` / `GE1/0/37` / `MGMT` | 端口名（交换机 CLI 式或服务器标签式） |

## 三条实测事实（决定了实现方式）

1. **位置→设备编码不是一对一**：5 个位置出现多个 code。那是机柜里
   上下相邻的两台设备被记成同一 U 位（源数据 U 位重叠），不是标签写错。
   → 生成时以设备实例为准，不以位置为准。
2. **设备编码→位置是一对一**：0 个 code 出现在多个位置。所以编码可以
   当设备唯一标识，这也是它能上标签的前提。
3. **`(SW)` 只出现在交换机上**（1747 条），服务器/安全设备没有这个后缀
   （1097 条）。它不是类型标签，是「这台是交换机，端口名是 CLI 口径」的提示。

## 为什么不从布线表直接抄

布线表有 25 列但**没有标签列**。标签是独立交付物：一份是「线清单」，
一份是「贴纸清单」，份数不同（标签每端一张，共 2 倍行数）、排序不同
（按施工顺序贴，不是按表行序）。
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Iterable, Mapping, Sequence

#: 自然排序用：把文本切成「数字段 / 非数字段」交替的序列。
_NATURAL_RE = re.compile(r"(\d+|\D+)")

#: 施工方习惯的位置格式：`{柜标识}-{U}U`。U 位数字不带前导零（`3U` 不是 `03U`）。
POSITION_TEMPLATE = "{rack}-{u}U"

#: 标签三段之间的分隔符。**一个空格**（实测：源表 2846 条全是单空格）。
SEP = " "

#: 设备编码的工作量上限。超过就是编码方案本身有问题（打印机也排不下）。
MAX_LABEL_CHARS = 48

#: 交换机编码后缀。只在 `category == "switch"`（含 core）时附加。
SWITCH_SUFFIX = "(SW)"


@dataclass(frozen=True, slots=True)
class LabelTarget:
    """一个标签指向的一端。

    ## 字段命名用 `rack_code` 而不是 `channel_no`

    标签上印的是**施工标识**（`11A14`、`4APU-13A-06`），不是「通道 2 机柜 5」。
    后者是我们内部的逻辑坐标，现场看不到。两者要能对上，靠 `models.Rack` 的
    `alias_code` / `display_name` 承载 —— 标签层只负责拼，不做换算。
    """

    #: 施工标识，如 `11A14` 或 `4APU-13A-06`
    rack_code: str
    #: 设备起始 U 位
    u_start: int
    #: 设备编码，如 `HLW-YWHX01`
    device_code: str
    #: 端口名，如 `40GE2/0/1` 或 `千兆1`
    port_name: str
    #: 设备类别（`switch`/`core`/`server`/`storage`/`security`/`other`）
    category: str = "other"

    @property
    def is_switch(self) -> bool:
        """交换机。`core` 也是交换机 —— 只是角色不同，端口名同样是 CLI 口径。"""
        return self.category in ("switch", "core")

    @property
    def position(self) -> str:
        """`11A14-16U`。"""
        return POSITION_TEMPLATE.format(rack=self.rack_code, u=self.u_start)


@dataclass(frozen=True, slots=True)
class LabelText:
    """一条标签的三段拆解。保留分段是为了让检查器能说清「哪一段错了」。"""

    position: str
    device: str
    port: str

    def joined(self) -> str:
        """三段用单空格连接，**空段直接跳过**。

        ## 为什么不能 `SEP.join(三段)`

        631 台服务器没有实例名，设备段是空的。直接 join 会得到
        `12A01-23U  10GE1`（双空格）—— 打印机与人工核对时都会以为
        中间漏印了一截，而实际是上游没填。跳过空段得到 `12A01-23U 10GE1`，
        看起来是「这台没有编码」，语义正确。
        """
        return SEP.join(x for x in (self.position, self.device, self.port) if x)

    @property
    def length(self) -> int:
        return len(self.joined())


def device_on_label(target: LabelTarget) -> str:
    """设备段：编码 + 交换机的 `(SW)` 后缀。

    ## 为什么后缀由代码加而不是让用户填进编码

    用户填的编码是「这台设备叫什么」（`HLW-YWHX01`）。`(SW)` 是**贴纸打印时的
    口径提示** —— 打线的人看到它就知道后面那段是 `40GE2/0/1` 这类交换机端口名，
    而不是 `千兆1`。把两件事混在一个字段里，改机型时容易漏改后缀。
    """
    code = target.device_code.strip()
    if not code:
        return SWITCH_SUFFIX if target.is_switch else ""
    return f"{code}{SWITCH_SUFFIX}" if target.is_switch else code


def render_label(target: LabelTarget) -> LabelText:
    """一端 → 一条标签。

    ## 为什么空编码不报错

    源数据里 631 台服务器没有实例名（`device_instance.name` 可空）。
    它们的标签只能是「位置 + 端口」，缺设备段。这不是我们算错，
    是上游没填 —— 报错会让整个导出失败，而少一个字段的标签现场仍能用
    （位置+端口已经能定位）。由 `check_labels()` 单独统计缺口。
    """
    return LabelText(
        position=target.position,
        device=device_on_label(target),
        port=target.port_name.strip(),
    )


def render_labels(targets: Iterable[LabelTarget]) -> tuple[LabelText, ...]:
    return tuple(render_label(t) for t in targets)


def render_plain(targets: Iterable[LabelTarget]) -> tuple[str, ...]:
    """拼好的整条标签文本（打印机直接吃这个）。"""
    return tuple(t.joined() for t in render_labels(targets))


# ------------------------------------------------------------------ 排序


#: 施工顺序：先按机柜，再按 U 位**从上往下**（柜顶先贴），最后按端口**数字序**。
#:
#: ## 为什么 U 位降序
#:
#: 打线的人站在机柜前是从上往下看的（柜顶 42U 在视线高处）。
#: 按 U 位升序排会让他在柜子里来回起伏 —— 一份 300 张的标签要弯腰几十次。
#:
#: ## 为什么机柜码与端口名都要自然排序（不是字符串序）
#:
#: 字符串序下 `12A02` < `1A01`（第二位 `2`(0x32) < `A`(0x41)），
#: 于是通道 12 的标签会排在通道 1 前面 —— 实测截图里就是这样，
#: 一份按通道分工的标签被打乱了。
#:
#: 端口名的字符串序更糟：`100GE1/0/10` < `100GE1/0/2`，
#: 于是现场按顺序贴会从 1 跳到 10 再跳回 2。实测 30 张连续标签里
#: 有 19 张是乱序的。
def _natural(text: str) -> tuple[object, ...]:
    """把 `12A02` 拆成 `(12, 'A', 2)`，`100GE1/0/10` 拆成 `('', 100, 'GE', 1, '/', 0, '/', 10)`。

    数字段转 int 参与比较，非数字段保持原样。这样 `2` 就排在 `10` 前面。
    """
    out: list[object] = []
    for chunk in _NATURAL_RE.findall(text):
        out.append(int(chunk) if chunk.isdigit() else chunk)
    return tuple(out)


def _port_key(port_name: str) -> tuple[object, ...]:
    """端口在标签上的顺序：**速率优先**，再按板卡/槽位/序号。

    ## 为什么速率要排在板卡前面

    同一台核心交换机上 `100GE1/0/1` 与 `25GE1/0/1` 的 (chassis, slot, port)
    完全相同（都是 1/0/1），只看它们会交替排出
    `100GE1/0/1, 25GE1/0/1, 100GE1/0/2, 25GE1/0/2…` —— 现场按这个顺序
    理线会把两种线缆缠在一起。

    按速率降序分组后，先贴完所有 100G 再贴 25G，与「线缆按类型成捆」
    的实际做法一致。实测 `业务核心交换机` 的 85 个口全部属于 2 种速率，
    分组后排成干净的 52 + 33 两段。
    """
    from ..ports.naming import parse_port_name, sort_key_port

    parsed = parse_port_name(port_name)
    rate = parsed.rate_mbps or 0
    return (-rate, *sort_key_port(port_name))


def sort_key(target: LabelTarget) -> tuple[object, ...]:
    return (
        _natural(target.rack_code),
        -target.u_start,
        _port_key(target.port_name),
    )


def sort_labels(targets: Iterable[LabelTarget]) -> tuple[LabelTarget, ...]:
    return tuple(sorted(targets, key=sort_key))


# ------------------------------------------------------------------ 检查


@dataclass(frozen=True, slots=True)
class LabelIssue:
    """一条标签的问题。`code` 用于汇总计数，`message` 用于给人看。"""

    code: str
    message: str
    label: str
    severity: str = "warn"


def check_label(label: LabelText) -> tuple[LabelIssue, ...]:
    """单条标签的体检。

    ## 为什么长度要卡 48 字符

    实测源表标签最长 46 字符（`12A02-41U HLW-YW01(SW) 10GE1/0/37` 这类）。
    常见的线缆标签纸（哑银 30×15mm）一行约容 24 个半角字符，
    48 字符要折两行 —— 已经是上限。再长打出来会挤成一团，现场认不出。
    """
    out: list[LabelIssue] = []
    text = label.joined()

    if not label.device:
        out.append(
            LabelIssue(
                code="NO_DEVICE_CODE",
                message="缺设备编码（上游没填实例名）—— 现场只能靠位置+端口定位",
                label=text,
                severity="info",
            )
        )
    if not label.port:
        out.append(LabelIssue(code="NO_PORT", message="缺端口名", label=text, severity="error"))
    if label.length > MAX_LABEL_CHARS:
        out.append(
            LabelIssue(
                code="TOO_LONG",
                message=f"{label.length} 字符，超过 {MAX_LABEL_CHARS} 会折行压挤",
                label=text,
            )
        )
    if SEP in label.port:
        # 端口名里带空格会让「三段」变成四段，打印机与人工都会按错位截断
        out.append(
            LabelIssue(
                code="PORT_HAS_SPACE",
                message="端口名含空格，会破坏三段结构",
                label=text,
                severity="error",
            )
        )
    return tuple(out)


@dataclass(frozen=True, slots=True)
class LabelSheet:
    """一批标签 + 体检结论。"""

    labels: tuple[LabelText, ...]
    issues: tuple[LabelIssue, ...] = ()

    @property
    def count(self) -> int:
        return len(self.labels)

    def counts_by_code(self) -> Mapping[str, int]:
        out: dict[str, int] = {}
        for issue in self.issues:
            out[issue.code] = out.get(issue.code, 0) + 1
        return dict(sorted(out.items()))

    def describe(self) -> str:
        if not self.issues:
            return f"{self.count} 条标签，未发现问题"
        parts = "、".join(f"{k} {v}" for k, v in self.counts_by_code().items())
        return f"{self.count} 条标签，{len(self.issues)} 个问题（{parts}）"


def build_sheet(targets: Iterable[LabelTarget]) -> LabelSheet:
    """按施工顺序排序 + 逐条体检。"""
    ordered = sort_labels(targets)
    labels = render_labels(ordered)
    issues: list[LabelIssue] = []
    for label in labels:
        issues.extend(check_label(label))
    return LabelSheet(labels=labels, issues=tuple(issues))
