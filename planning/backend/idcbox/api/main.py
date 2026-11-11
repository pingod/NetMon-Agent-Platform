"""FastAPI 应用装配。

## 为什么只读接口先上，写接口后上

v0.2 的目标是「看得见」——用户要先能**审查**这份数据是不是对的，
才谈得上编辑。写接口（改 U 位、切策略、批量核定）依赖乐观锁与审计落库，
是 v0.3「管得住」的事。先做只读能把「数据对不对」这个最贵的问题解决掉。

## 为什么项目状态用进程内缓存

单机单用户（Q4 裁决：1 主编辑 + N 只读），一个项目 5127 行全量加载 + 体检
约 1.5 秒。每次请求都重算会让页面卡死。缓存以源文件 `(路径, mtime, size)`
为键 —— 文件改了自动失效，不需要用户手动"刷新缓存"。

这也是为什么没有先引入 Redis：单进程缓存已经够，引入外部依赖
会让"双击运行的绿色包"这个部署形态破产。
"""

from __future__ import annotations

import os
import threading
from collections import Counter
from pathlib import Path
from typing import TYPE_CHECKING, Any

from fastapi import FastAPI, HTTPException, Query
from fastapi.middleware.cors import CORSMiddleware

from ..application.services.pipeline import (
    check_project,
    compute_project,
    load_project,
)
from ..application.services.compare import (
    compare_files,
    compare_project_latest,
    list_stock_files,
    set_stock_reader,
    stock_dir,
)
from ..application.services.design import design_payload
from ..application.services.portledger import device_payload, ledger_payload
from ..application.services.labelsvc import label_payload as labels_payload
from ..application.services.modeladmin import (
    export_json as model_export_json,
)
from ..application.services.modeladmin import (
    import_json_library,
    library_payload,
    load_project_library,
)
from ..application.services.projects import (
    compare_versions,
    create_project,
    default_project_name,
    list_projects,
    list_versions,
    load_intent_version,
    save_intent,
)
from ..application.services.drawing import build_rack_sheet, render_rack_sheet
from ..domain.exporting.svg import render_racks, render_to_html
from ..application.services.scene import scene_payload
from ..application.services.views import (
    ProjectViews,
    build_views,
    cables_payload,
    consumable_payload,
    device_cables_payload,
    floor_payload,
    health_payload,
    meta_payload,
    policy_payload,
    port_utilization_payload,
    rack_list_payload,
    rack_payload,
)

from ..domain.cable_length.rules import DEFAULT_RULE

from ..domain.ports.clipboard import (
    CellRange,
    apply_fill,
    apply_paste,
    format_for_clipboard,
    normalize_range,
    parse_clipboard,
    plan_fill,
    plan_paste,
)
from ..domain.ports.intentcheck import (
    append_row as intent_append_row,
    apply_cell as intent_apply_cell,
    check_intent,
    delete_row as intent_delete_row,
    coerce_cell as intent_coerce_cell,
)
from ..domain.ports.intent import intent_to_dict
from ..domain.ports.moveu import (
    DeviceTarget,
    editable_items,
    move_device,
    move_devices,
)
from ..application.services.impact import (
    impact_payload,
    preview_move,
    preview_move_devices,
)
from ..infrastructure.config.empirical_loader import load_empirical_table
from ..infrastructure.xlsx.sources import XlsxProjectSource
from ..infrastructure.xlsx.stock_reader import XlsxStockReader

if TYPE_CHECKING:  # pragma: no cover - 仅供类型检查器
    # `Intent` 只在函数体里用作类型注解（`source_format` 分支决定来源），
    # 运行时不需要它 —— 放进来会多一次域层导入。
    from ..domain.ports.intent import Intent

#: 默认源文件。CLI 与 API 共用同一套解析，路径可由环境变量覆盖。
DEFAULT_SOURCE = (
    Path(__file__).resolve().parents[3]
    / "信息港部署上架"
    / "V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"
)
DEFAULT_EMPIRICAL = Path(__file__).resolve().parents[3] / "rules" / "len_empirical_v1.json"

#: 默认接线意图。放在 `projects/<项目>/intent.json`，与 golden 测试共用一份 ——
#: 测试读的就是用户会改的那个文件，避免"fixture 与真货漂移"。
#: 默认意图文件 = **完整意图**（含 697 条显式连线），能生成全表 5127 行。
#: 同目录还有 `intent.json`（只覆盖计算节点的 4430 行），是「只做上架规划、
#: 暂不接交换机互联」场景的精简版；要切过去用 `IDCBOX_INTENT=...` 覆盖。
DEFAULT_INTENT = Path(__file__).resolve().parents[3] / "projects" / "v13" / "intent_full.json"


class ProjectCache:
    """按源文件指纹缓存的已加载项目。

    ## 为什么锁粒度是整个缓存而不是每个项目

    只有 1 个用户（Q4 裁决），并发只会出现在"前端同时发 6 个请求"这种场景，
    它们打开的是同一个项目。粗锁让实现简单且不会死锁，
    代价是首屏 6 个请求串行 —— 实测总耗时 1.6 秒，可接受。

    若将来真多人用，这里换成 per-key 锁即可，接口不变。
    """

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._key: tuple[str, float, int] | None = None
        self._views: ProjectViews | None = None
        self._report: Any = None

    def get(self, source: Path) -> tuple[ProjectViews, Any]:
        st = source.stat()
        key = (str(source), st.st_mtime, st.st_size)
        with self._lock:
            if self._key == key and self._views is not None:
                return self._views, self._report
            views, report = _load(source)
            self._key, self._views, self._report = key, views, report
            return views, report


def _load(source: Path) -> tuple[ProjectViews, Any]:
    if not source.exists():
        raise HTTPException(
            status_code=404,
            detail=f"源文件不存在：{source}。请把 V13 放到 信息港部署上架/ 下，或设置 IDCBOX_SOURCE。",
        )
    # 同一实例同时充当 FloorSource 与 CablingSource：工作簿只打开一次
    source_reader = XlsxProjectSource(source)
    project = load_project(
        floor_source=source_reader,
        cabling_source=source_reader,
        label=source.stem,
        source_path=source,
        rule=DEFAULT_RULE,
    )
    table = (
        load_empirical_table(DEFAULT_EMPIRICAL) if DEFAULT_EMPIRICAL.exists() else None
    )
    computed = compute_project(project, table=table)
    checked = check_project(computed)
    return build_views(project, computed), checked.report


_cache = ProjectCache()


#: 意图六段表格的列顺序。粘贴是**按位置对齐**的，所以这份顺序必须与
#: 前端 `IntentEditor.SECTIONS` 的 columns 顺序逐字一致 —— 一旦错位，
#: 用户粘 5 列数据会静默写进 5 个错误的字段。
INTENT_SECTION_COLUMNS: dict[str, tuple[str, ...]] = {
    "racks": ("channel", "from", "to", "model"),
    "switches": ("instance", "model", "channel", "rack", "u", "uSpan"),
    "devices": ("instance", "model", "channel", "rack", "u", "uSpan"),
    "routes": ("nodeModel", "nic", "switchModel"),
    "chains": ("channel", "switchModel", "nic", "instances", "capacity"),
    "explicitLinks": ("a", "b", "cableType"),
}


def _resolve_racks(
    views: Any, channel: int | None, rack: str
) -> list[tuple[int, int]] | None:
    """把 `channel` + `rack` 两个查询参数解析成机柜键列表。

    返回 None 表示「不筛选」（全部有设备的机柜）。
    返回空列表表示「筛了但一个都没匹配上」—— 调用方据此报 404，
    不能把空列表当 None 处理，否则用户打错柜号会拿到一张全是柜子的图。
    """
    if channel is None and not rack.strip():
        return None
    wanted: set[int] | None = None
    if rack.strip():
        wanted = set()
        for part in rack.replace("，", ",").split(","):
            part = part.strip()
            if not part:
                continue
            try:
                wanted.add(int(part))
            except ValueError as exc:
                raise HTTPException(
                    status_code=422, detail=f"机柜号必须是整数，收到「{part}」"
                ) from exc
    hits: list[tuple[int, int]] = []
    for ch, seq in sorted(views.rack_devices):
        if channel is not None and ch != channel:
            continue
        if wanted is not None and seq not in wanted:
            continue
        hits.append((ch, seq))
    return hits


def _source_from_env() -> Path:
    import os

    override = os.environ.get("IDCBOX_SOURCE")
    return Path(override) if override else DEFAULT_SOURCE


def _intent_from_env() -> Path:
    """意图文件路径。`IDCBOX_INTENT` 可指向 `.json` 或六段 `.xlsx`。

    ## 为什么 API 也认 Excel

    用户填的是 Excel（`intent-export` 导出的模板）。若 API 只认 JSON，
    他就得每次改完表再跑一条 CLI 转换命令 —— 多一步就会有人忘记，
    然后界面显示的还是上一次的结果，而**界面不会报错**。

    按扩展名分派而不是加一个环境变量：用户已经用文件名表达了意图，
    再加一个开关只会多一处能配错的地方。
    """
    import os

    override = os.environ.get("IDCBOX_INTENT")
    return Path(override) if override else DEFAULT_INTENT


def _models_dir(project_name: str) -> Path:
    """项目型号库所在目录。

    ## 为什么按项目名建目录而不是复用 source_path 的目录

    `source_path` 指向的是客户交付的 Excel 文件（`信息港部署上架/...`），
    那是只读的交付物目录。用户改出来的型号库是自己的数据，混进去下次
    交付时会被一起带走 —— 而且文件名相近，很容易覆盖掉客户的原始表。

    所以统一放 `<仓库>/projects/<项目名>/models.json`，与意图 JSON 同层。
    """
    root = Path(__file__).resolve().parents[3] / "projects"
    return root / project_name


def _library_for(project_name: str):
    """按项目名加载生效的型号库。项目名空时退回内置表。"""
    if not project_name:
        return load_project_library(None)
    return load_project_library(_models_dir(project_name))


def _load_intent_file(path: Path):
    """按扩展名读意图：`.xlsx` 走六段表格，其余当 JSON。"""
    if path.suffix.lower() in (".xlsx", ".xlsm"):
        from ..infrastructure.xlsx.intent_sheet import (
            REQUIRED_SHEETS,
            read_intent_workbook,
        )

        intent, _found, missing = read_intent_workbook(path)
        lost = [n for n in missing if n in REQUIRED_SHEETS]
        if lost:
            raise ValueError(
                f"意图表格缺少必需的表：{'、'.join(lost)}。"
                f"请用 `intent-export` 导出模板后填写。"
            )
        return intent
    from ..domain.ports.intent import load_intent

    return load_intent(path)


def create_app() -> FastAPI:
    app = FastAPI(
        title="IDCBoxLine API",
        version="0.2.0",
        description="IDC 机柜与布线管理 — 只读视图接口（v0.2「看得见」）",
    )

    # 组合根：把基础设施实现注册进应用层。
    # 应用层 `compare.py` 只认 `StockReader` Protocol，代码里不出现 openpyxl ——
    # 这是 `lint-imports` 第 4 条契约「应用层不得依赖基础设施」的要求。
    set_stock_reader(XlsxStockReader())

    # 开发期 Vite 在 5173，后端在 8000。生产由 FastAPI 同源托管 `web/dist`，不需要 CORS。
    #
    # `allow_methods` 从 v0.4 起必须含 POST/PUT —— v0.4 加了 8 个写端点
    # （`POST /api/intent/check|validate|row|move|paste`、`POST /api/projects`、
    # `PUT /api/projects/{name}/intent`）。只写 GET 时浏览器预检直接失败，
    # 表现为「界面点了没反应、控制台一条 CORS 红字」，排查起来很绕。
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["http://127.0.0.1:5173", "http://localhost:5173"],
        allow_methods=["GET", "POST", "PUT", "OPTIONS"],
        allow_headers=["*"],
    )

    def views_and_report() -> tuple[ProjectViews, Any]:
        return _cache.get(_source_from_env())

    @app.get("/api/health")
    def health() -> dict[str, str]:
        """存活探针。不读项目数据 —— 探针不该因为数据文件大而超时。"""
        return {"status": "ok"}

    @app.get("/api/meta")
    def meta() -> dict[str, Any]:
        views, _ = views_and_report()
        return meta_payload(views)

    @app.get("/api/floor")
    def floor() -> dict[str, Any]:
        views, _ = views_and_report()
        return floor_payload(views)

    @app.get("/api/scene")
    def scene() -> dict[str, Any]:
        """3D 场景数据（FR-7.1/7.2）：房间 + 机柜 + 全部设备，一次给全。

        ## 为什么单独一个端点，不让 3D 复用 `/api/floor` + `/api/racks`

        两者的取数形状本质不同：

        | | 2D 需要 | 3D 需要 |
        |---|---|---|
        | 机柜 | 一个格子 `col/row` | 一个**实心盒子**的位置 + 朝向 |
        | 设备 | 点进某个柜才要 | **一次要全部 834 台**（要算 instance 矩阵） |

        复用的话，3D 启动要为 102 个有设备的机柜各发一次
        `/api/racks/{ch}/{seq}` —— 102 个请求才能画出第一帧。

        ## 坐标是毫米，且只有一处换算

        42U 柜高 = 42 × 44.45 = 1866.9mm（EIA-310 标准 1U）、柜宽 600mm、
        柜深 1200mm、通道净宽 1200mm。柜宽 600 与布线表 R 列的
        `0.6m/柜位`**同源**，不是另拍的数。

        前端 `mmToWorld(mm) = mm / 1000` 一次转完（three 的惯例是 1 单位 = 1 米）。
        数据里绝不出现像素 —— 一旦像素进模型，导出 SVG、打印 300dpi、
        3D 世界单位就各自乱掉。

        ## 为什么 `rooms[].originX` 必须给

        403 房用列 23..38、404 房用列 3..18，**两间房的列号各自独立**。
        若都按「减本房最左列」算，两间房的柜会拿到完全相同的世界坐标 ——
        实测不修的话 362 个柜里 146 个坐标重复（表现为「一个房间、
        柜数是两倍」）。
        """
        views, _ = views_and_report()
        return scene_payload(views)

    @app.get("/api/racks")
    def racks() -> dict[str, Any]:
        views, _ = views_and_report()
        return rack_list_payload(views)

    @app.get("/api/racks/{channel_no}/{seq_no}")
    def rack(channel_no: int, seq_no: int) -> dict[str, Any]:
        views, _ = views_and_report()
        payload = rack_payload(views, channel_no, seq_no)
        if payload is None:
            raise HTTPException(404, f"机柜 通道{channel_no}/机柜{seq_no} 不在平面图中")
        return payload

    @app.get("/api/cables")
    def cables(
        offset: int = Query(0, ge=0),
        limit: int = Query(200, ge=1, le=2000),
        channel: int | None = Query(None, ge=1),
        rack: int | None = Query(None, ge=1),
        cableType: str | None = None,
        text: str | None = None,
    ) -> dict[str, Any]:
        views, _ = views_and_report()
        return cables_payload(
            views,
            offset=offset,
            limit=limit,
            channel=channel,
            rack_no=rack,
            cable_type=cableType,
            text=text,
        )

    @app.get("/api/devices/{device_name}/cables")
    def device_cables(device_name: str) -> dict[str, Any]:
        views, _ = views_and_report()
        return device_cables_payload(views, device_name)

    @app.get("/api/health-report")
    def health_report() -> dict[str, Any]:
        _, report = views_and_report()
        return health_payload(report)

    @app.get("/api/ports/utilization")
    def ports_utilization() -> dict[str, Any]:
        views, _ = views_and_report()
        return port_utilization_payload(views)

    @app.get("/api/models")
    def models(
        project: str = Query("", description="项目名；空=库里最新的项目"),
        ports: bool = Query(False, description="是否附每组的端口明细"),
    ) -> dict[str, Any]:
        """设备型号库：这个项目现在用哪套端口模板、每个型号几个口、谁提供的。

        ## 为什么要显示「出处」

        型号可能是内置的、项目 `models.json` 里的、或 Excel 导入的。
        用户改了半天 Excel 发现数字没动，最容易怀疑是程序坏了 ——
        把出处写在旁边，他一眼就能看到「这个型号还是内置的，我的 Excel 没生效」。
        只显示端口数做不到这一点：内置和覆盖后的数字看起来都合理。
        """
        store = _project_store()
        chosen = project or default_project_name(store) or ""
        lib = _library_for(chosen)
        payload = library_payload(lib, with_ports=ports)
        payload["project"] = chosen
        return payload

    @app.post("/api/models/import")
    def models_import(body: dict[str, Any]) -> dict[str, Any]:
        """导入型号库（`models.json` 的内容），合并进项目库并返回差异。

        ## 为什么是合并不是覆盖

        覆盖会把之前手工加过的其它型号一起抹掉，而那些型号没有别的副本。
        要删型号得在条目里显式写 `remove: true`。
        """
        project_name = str(body.get("project") or "").strip()
        if not project_name:
            raise HTTPException(status_code=422, detail="请求体需要 project（项目名）")
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（型号库对象）")
        try:
            lib, diff = import_json_library(
                _models_dir(project_name), doc, detail="界面导入"
            )
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        payload = library_payload(lib)
        payload["project"] = project_name
        payload["diff"] = diff
        return payload

    @app.get("/api/models/export")
    def models_export(
        project: str = Query("", description="项目名；空=库里最新的项目"),
        all: bool = Query(False, description="含内置型号（默认只导项目自己的）"),
    ) -> dict[str, Any]:
        """导出型号库成 `models.json`，用于「导出来改、改完再导回」。

        默认只导项目自己声明的型号 —— 这样再导入一次不会把 26 条内置型号
        复制成用户数据，下次内置表更新时那 26 条就再也收不到更新了。
        """
        store = _project_store()
        chosen = project or default_project_name(store) or ""
        lib = _library_for(chosen)
        return {"project": chosen, "doc": model_export_json(lib, only_user=not all)}

    @app.get("/api/labels")
    def labels(
        limit: int = Query(500, ge=1, le=20000, description="返回条数上限"),
        numberMissing: bool = Query(False, description="给没有实例名的设备自动编号"),
        format: str = Query("json", description="json=明细 / text=纯文本（打印机直接吃）"),
    ) -> Any:
        """线缆标签清单：每端一张贴纸印什么字。

        ## 为什么与 `/api/cables` 分开

        布线表是 25 列 × 5127 行的宽表，标签是「位置 设备编码(SW) 端口」三段窄文本，
        份数是两倍（每端一张），排序也不同（按施工贴纸顺序，不是表行序）。
        硬塞进 `/api/cables` 会让那个端点同时服务两种完全不同的消费方。

        ## 为什么要有 `format=text`

        现场的用法是「导出一份 txt、丢给标签打印机」。让前端再解析一遍 JSON 拼字符串，
        等于把同一份格式约定写两遍 —— 后端这里直接给成品。
        """
        views, _ = views_and_report()
        payload = labels_payload(views, limit=limit, number_missing_codes=numberMissing)
        if format == "text":
            from fastapi.responses import PlainTextResponse

            #: 文件名必须走 RFC 5987 的 `filename*=UTF-8''...` 形式。
            #: HTTP 头只能是 latin-1，而项目名是中文（「信息港二期」）——
            #: 直接塞进 `filename="..."` 会让 starlette 在编码响应头时抛
            #: `UnicodeEncodeError: 'latin-1' codec can't encode characters`，
            #: 且报错发生在响应构造阶段，看起来像端点 500 而不是编码问题。
            from urllib.parse import quote

            ascii_name = "labels.txt"
            utf8_name = quote(f"labels-{views.project.label}.txt")
            return PlainTextResponse(
                "\n".join(row["text"] for row in payload["rows"]),
                headers={
                    "Content-Disposition": (
                        f"attachment; filename={ascii_name}; filename*=UTF-8''{utf8_name}"
                    )
                },
            )
        return payload


    @app.get("/api/ledger")
    def ledger(
        devices: bool = Query(False, description="是否附设备明细（默认只给汇总）"),
        limit: int = Query(200, ge=1, le=2000),
        device: str = Query("", description="只看某台设备（按实例名或位置号）"),
        project: str = Query("", description="项目名；空=库里最新的项目"),
    ) -> dict[str, Any]:
        """端口台账：哪台设备用了几个口、还剩几个、每个口插了什么。

        ## 为什么和 `/api/ports/utilization` 并存

        那个按**型号**汇总（「业务接入交换机1 这一批还剩多少口」），
        这个按**设备**展开（「3 号柜那台还剩哪几个口」）。
        现场问答是两个不同粒度的问题，合成一页会让两组数字看起来在打架。

        ## 为什么从项目意图现算而不查数据库

        v0.5 的意图仍以版本化 JSON 存（`intent_document`），规范表
        （`device_instance`/`port`/`cable`）尚未落库 —— 那是 v0.6 的事。
        现算一次约 60ms（5127 条线建台账），比维护两份可能不一致的数据便宜。
        """
        store = _project_store()
        chosen = project or default_project_name(store)
        if not chosen:
            raise HTTPException(
                status_code=404,
                detail="库里还没有任何带意图版本的项目，无法生成端口台账。",
            )
        try:
            intent = load_intent_version(store, project_name=chosen)
        except ValueError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc
        from ..application.services.design import generate_design

        # 型号库按项目加载：「这个项目的自研设备也能算出台账」。
        # 不传的话台账会把这些设备算成「无模板」，而它们的端口其实是已知的。
        design = generate_design(intent, library=_library_for(chosen))
        if design.ledger is None:
            raise HTTPException(status_code=404, detail="意图里还没有任何连线，台账为空。")
        led = design.ledger
        if device:
            for d in led.devices:
                if d.device_key == device or d.device_label == device:
                    return {
                        "project": chosen,
                        "device": device_payload(d, ports=True),
                    }
            raise HTTPException(status_code=404, detail=f"台账里没有设备：{device}")
        payload = ledger_payload(led, include_devices=devices, limit=limit)
        payload["project"] = chosen
        return payload

    @app.get("/api/consumables")
    def consumables(phase: str = Query("", description="批次过滤（如 首期）；空=全项目")) -> dict[str, Any]:
        """耗材与模块需求清单。

        ## 为什么 `phase` 默认空而不是 `首期`

        原表只有 450/5127 行标了 `首期`（8.8%）。默认 `首期` 会让
        「全项目采购量」看起来只有首期的量，用户以为规划完了；
        默认全量则是「如果一次买齐要多少」。后者是中性基线，
        前者需要用户明确表达。所以默认给全量，并在 payload 里回显 `phase`。

        ## 为什么返回的是「需求」+「建议采购」而不是「采购量」

        实测原表人工采购倍数是 1.124~2.314（见 `consumables.py` docstring），
        固定系数复现不了。程序给需求与建议，最终数量由人核定。
        """
        views, _ = views_and_report()
        return consumable_payload(views, phase=phase or None)

    # ---------------- 施工图导出（FR-6.7）----------------
    #
    # ## 为什么用 StreamingResponse 而不是返回字符串
    #
    # FastAPI 返回 str 会带上 `Content-Type: text/plain`，浏览器打开就是
    # 一屏源码而不是图。必须显式声明 `image/svg+xml`。
    #
    # ## 为什么同时给 .html 版本
    #
    # 单独打开 .svg 打印时，浏览器会加自己的页眉页脚、@page 规则不生效、
    # 边缘被裁。HTML 版把 `@page { size: A3 landscape }` 写进去，
    # Ctrl+P 直接出图。给现场用的就是 HTML 版。

    @app.get("/api/draw/racks.svg")
    def draw_racks_svg(
        channel: int | None = Query(None, description="只画某个通道；空=全部有设备的机柜"),
        rack: str = Query("", description="逗号分隔的机柜号，如 1,2,3；空=该通道全部"),
        title: str = Query("机柜 U 位图"),
    ) -> Any:
        from fastapi.responses import Response

        views, _ = views_and_report()
        racks = _resolve_racks(views, channel, rack)
        if racks is not None and not racks:
            raise HTTPException(status_code=404, detail="没有匹配的机柜")
        svg = render_rack_sheet(views, racks=racks, title=title)
        return Response(
            content=svg,
            media_type="image/svg+xml",
            headers={"Content-Disposition": 'inline; filename="racks.svg"'},
        )

    @app.get("/api/draw/racks.html")
    def draw_racks_html(
        channel: int | None = Query(None),
        rack: str = Query(""),
        title: str = Query("机柜 U 位图"),
    ) -> Any:
        from fastapi.responses import Response

        views, _ = views_and_report()
        racks = _resolve_racks(views, channel, rack)
        if racks is not None and not racks:
            raise HTTPException(status_code=404, detail="没有匹配的机柜")
        sheet = build_rack_sheet(views, racks, title=title)
        html = render_to_html(render_racks(sheet), title=title)
        return Response(
            content=html,
            media_type="text/html; charset=utf-8",
            headers={"Content-Disposition": 'inline; filename="racks.html"'},
        )

    # ---------------- 项目持久化（v0.4）----------------
    #
    # ## 为什么这些是写接口而其它端点都是只读
    #
    # 前 12 个端点读的是「一份 Excel 的解析结果」—— 数据源在文件里，
    # 服务只是投影。项目端点读写的却是**用户自己的数据**（意图 + 版本历史），
    # 它没有文件可回落，必须落库。这也是 v0.4 补持久化的原因。

    @app.get("/api/projects")
    def projects(includeArchived: bool = Query(False)) -> dict[str, Any]:
        store = _project_store()
        items = list_projects(store, include_archived=includeArchived)
        return {
            "projects": [
                {
                    "id": p.id,
                    "name": p.name,
                    "siteKey": p.site_key,
                    "status": p.status,
                    "latestVersion": p.latest_version,
                    "versionCount": p.version_count,
                    "updatedAt": p.updated_at,
                }
                for p in items
            ],
            "total": len(items),
            "database": str(_db_path()),
        }

    @app.post("/api/projects")
    def create_project_endpoint(body: dict[str, Any]) -> dict[str, Any]:
        name = str(body.get("name") or "").strip()
        if not name:
            raise HTTPException(status_code=422, detail="项目名不能为空")
        store = _project_store()
        p = create_project(
            store,
            name=name,
            site_key=str(body.get("siteKey") or ""),
            source_path=str(body.get("sourcePath") or "") or None,
        )
        return {"id": p.id, "name": p.name, "latestVersion": p.latest_version}

    @app.get("/api/projects/{name}/versions")
    def project_versions(name: str) -> dict[str, Any]:
        store = _project_store()
        try:
            items = list_versions(store, project_name=name)
        except ValueError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc
        return {
            "project": name,
            "versions": [
                {
                    "version": v.version,
                    "createdAt": v.created_at,
                    "createdBy": v.created_by,
                    "note": v.note,
                    "summary": v.summary,
                }
                for v in items
            ],
            "total": len(items),
        }

    @app.get("/api/projects/{name}/intent")
    def project_intent(
        name: str,
        version: int | None = Query(None, description="版本号；空=最新"),
    ) -> dict[str, Any]:
        """读回意图文档（原始 JSON，非生成结果）。

        ## 为什么直接返回原始 doc 而不转成域对象

        前端意图编辑器要的是**可原样回写**的结构。转成 `Intent` 再序列化
        会丢掉未知字段（向前兼容要求保留），也会把空段补成 `[]` 改变形状。

        ## 关于返回的 `version`

        一定是**实际读到的那个版本号**，
        而不是请求里传的那个（传空时它会是 `None`）。前端要拿它显示
        「正在编辑 v2」，回报 `null` 就会显示成「v?」—— 这种小洞
        会让用户怀疑「我改的到底是最新版吗」。
        """
        store = _project_store()
        try:
            project = store.by_name(name)
            if project is None:
                raise ValueError(f"项目不存在：{name}")
            resolved = version
            if resolved is None:
                latest = store.latest_version(project.id)
                if latest is None:
                    raise ValueError(f"项目「{name}」还没有保存过意图")
                resolved = latest.version
            doc = store.load_doc(project.id, resolved)
        except ValueError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc
        if doc is None:
            raise HTTPException(
                status_code=404,
                detail=f"项目「{name}」还没有保存过意图（version={version}）",
            )
        return {"project": name, "version": resolved, "doc": doc}

    @app.put("/api/projects/{name}/intent")
    def save_project_intent(name: str, body: dict[str, Any]) -> dict[str, Any]:
        """保存意图为新版本。

        ## 请求体形状

        `{"doc": <意图 JSON>, "note": "改了什么"}`。
        `doc` 允许是 `{}` 以外任何合法意图；校验交给域层 `intent_from_dict`
        （它会检查 schema 版本与段类型），本端点只负责落库。

        ## 为什么返回 `isNew`

        用户点保存时界面要提示「已保存为 v3」还是「内容没变，仍是 v3」。
        由后端判定而不是前端比对，因为内容哈希的规范化规则只在后端有一份。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        note = body.get("note")
        store = _project_store()
        try:
            saved = save_intent(
                store,
                project_name=name,
                intent=doc,
                note=str(note) if note else None,
                force_new_version=bool(body.get("force")),
            )
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc
        return {
            "project": saved.project,
            "version": saved.version,
            "isNew": saved.is_new,
            "summary": saved.summary,
        }

    @app.get("/api/projects/{name}/diff")
    def project_diff(
        name: str,
        fromVersion: int = Query(..., alias="from"),
        toVersion: int = Query(..., alias="to"),
    ) -> dict[str, Any]:
        store = _project_store()
        try:
            return compare_versions(
                store, project_name=name, from_version=fromVersion, to_version=toVersion
            )
        except ValueError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc

    @app.post("/api/intent/check")
    def intent_check(body: dict[str, Any]) -> dict[str, Any]:
        """校验一份意图，返回全部问题（不落库、不生成）。

        ## 为什么校验独立成一个端点

        编辑器每次改一格都想知道「这一格填错没有」，但它不该触发
        「存版本」也不该触发「生成 5127 行」。三件事分开，
        前端才能做到「改一格 → 只校验 → 不产生版本历史」。

        ## 为什么不是「先转 Intent 再校验」

        `intent_from_dict` 遇到缺字段会抛 `KeyError`（如 `x["instance"]`），
        一条错误就中断，用户看不到还有哪些地方也错了。
        这里直接在原始 dict 上走，一次报全 —— 而且能指出**段内行号**，
        `Intent` 对象里没有那个信息。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        return check_intent(doc).as_dict()

    @app.post("/api/intent/validate")
    def intent_validate(body: dict[str, Any]) -> dict[str, Any]:
        """校验 + 试跑一遍「能不能真的转成 Intent」。

        `check_intent` 是业务规则校验（名字、引用、范围）；
        `intent_from_dict` 是结构校验（字段类型、必填键）。
        两者都可能失败，且失败原因不同：
        - 业务校验失败 → 人能看懂的一句话
        - 结构校验失败 → 通常是编辑器 bug 或缺字段，要把原始异常也返回

        这条路是「保存前最后一次确认」，所以两个都跑。
        """
        from ..domain.ports.intent import intent_from_dict

        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        result = check_intent(doc).as_dict()
        structural: str | None = None
        try:
            intent_from_dict(doc)
        except Exception as exc:  # noqa: BLE001 - 任何结构问题都要报给人看
            structural = f"{type(exc).__name__}: {exc}"
        result["structuralError"] = structural
        result["generatable"] = result["ok"] and structural is None
        return result

    @app.post("/api/intent/row")
    def intent_row(body: dict[str, Any]) -> dict[str, Any]:
        """对文档做一次行级操作：改一格 / 加一行 / 删一行。

        ## 为什么这个端点不落库

        编辑器是「本地改、看效果、满意了再存」。这里只返回改完的文档，
        由前端持有草稿状态；想留版本时再调 `PUT /api/projects/{name}/intent`。
        把编辑与版本解耦，用户才能「改十次存一次」而不是「改十次留十个版本」。

        ## 为什么改一格要回服务端

        单元格类型转换（`"3"` → `3`）与校验规则只在后端有一份。
        前端也实现一套的话，两套规则迟早不一致 ——
        而现在不一致的后果是「界面说没问题，保存时 500」。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        op = str(body.get("op") or "")
        section = str(body.get("section") or "")

        if op == "cell":
            row_no = body.get("rowNo")
            if not isinstance(row_no, int):
                raise HTTPException(status_code=422, detail="cell 操作需要整数 rowNo")
            column = str(body.get("column") or "")
            if not column:
                raise HTTPException(status_code=422, detail="cell 操作需要 column")
            wrote, err = intent_apply_cell(doc, section, row_no, column, body.get("value"))
            out_row = row_no
        elif op == "append":
            template = body.get("template")
            out_row = intent_append_row(
                doc, section, template if isinstance(template, dict) else None
            )
            wrote, err = True, None
        elif op == "delete":
            row_no = body.get("rowNo")
            if not isinstance(row_no, int):
                raise HTTPException(status_code=422, detail="delete 操作需要整数 rowNo")
            wrote, err = intent_delete_row(doc, section, row_no), None
            out_row = row_no
        else:
            raise HTTPException(status_code=422, detail=f"不认识的操作：{op!r}")

        result = check_intent(doc).as_dict()
        return {
            "ok": wrote,
            "error": err,
            "rowNo": out_row,
            "doc": doc,
            "check": result,
        }

    @app.post("/api/intent/paste")
    def intent_paste(body: dict[str, Any]) -> dict[str, Any]:
        """把 Excel 粘贴块写进某一段（v0.4 #3 Excel 化交互层）。

        ## 为什么这是「Excel 化」里最重要的一件事

        949 行意图听起来不多，但没人愿意在网页上一格一格敲 949 个格子。
        真实动作永远是：Excel 里排好一片 → 框选 → Ctrl+C → 切到浏览器 → Ctrl+V。
        AG Grid Community 的剪贴板是 Enterprise 功能（$999/dev），
        本项目离线部署，不给客户再加一笔授权成本 —— 所以自己实现。

        ## 请求

        ```json
        {"doc": {...}, "section": "switches", "text": "接入-1\t业务接入交换机1\t2\t26\t40",
         "startRow": 13, "mode": "overwrite"}
        ```

        `startRow` 省略 = 追加到末尾；`mode=insert` = 在 startRow 处插入。

        ## 为什么解析与落盘都在服务端

        类型转换与校验规则只在后端有一份（见 `/api/intent/row` 的同款理由）。
        前端只负责把 `paste` 事件的 `text/plain` 原样送上来，
        不自己切分 —— 否则「界面粘对了、保存时错」这类问题又要出现一次。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        section = str(body.get("section") or "")
        if section not in INTENT_SECTION_COLUMNS:
            raise HTTPException(status_code=422, detail=f"没有「{section}」这一段")
        text = body.get("text")
        if not isinstance(text, str):
            raise HTTPException(status_code=422, detail="请求体需要 text 字段（剪贴板文本）")

        block = parse_clipboard(text)
        if block.is_empty:
            return {
                "ok": False,
                "error": "剪贴板里没有内容",
                "written": 0,
                "notes": [],
                "doc": doc,
                "check": check_intent(doc).as_dict(),
            }

        start_row = body.get("startRow")
        if start_row is not None and not isinstance(start_row, int):
            raise HTTPException(status_code=422, detail="startRow 必须是整数或省略")
        mode = str(body.get("mode") or "overwrite")
        if mode not in ("overwrite", "insert"):
            raise HTTPException(status_code=422, detail=f"不认识粘贴模式：{mode!r}")

        plan = plan_paste(
            block,
            INTENT_SECTION_COLUMNS[section],
            start_row=start_row,
            mode=mode,
        )
        written, notes = apply_paste(doc, section, plan, coerce=intent_coerce_cell)
        if block.ragged:
            notes.insert(0, "粘贴的各行列数不一致，短的行尾部留空")
        return {
            "ok": written > 0,
            "error": None,
            "written": written,
            "notes": notes,
            "doc": doc,
            "check": check_intent(doc).as_dict(),
        }

    @app.post("/api/intent/copy")
    def intent_copy(body: dict[str, Any]) -> dict[str, Any]:
        """把一块区域序列化成 TSV，供前端写进剪贴板（v0.4 #3 复制出）。

        ## 为什么收 `doc` 而不是自己从磁盘/库读

        编辑器里是**本地草稿** —— 用户改了 10 格还没保存。若这个端点
        从磁盘或库读，复制出去的是「上次保存的版本」，而用户看到的是
        自己刚改的内容。这种「复制了但内容不是眼前这份」的错极难发现，
        因为文本看起来也很像。

        与 `/api/intent/paste`、`/api/intent/fill` 一致：都收 `doc`。

        ## 为什么行号从 1 开始

        与界面显示的「第 N 行」一致。界面上没人看到 0-based 行号，
        传 0 会让人以为程序算错了一行 —— 这类差一错误用户很难自己发现。

        ## 为什么返回文本而不是让前端自己拼

        制表符/引号/换行的转义规则（Excel 那套）只该有一份实现。
        前端拼一个、后端拼一个，迟早出现「复制出去粘回来丢了一列」。
        这里与 `parse_clipboard` 是对称的一对，往返恒等由单测守着。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        section = str(body.get("section") or "")
        if section not in INTENT_SECTION_COLUMNS:
            raise HTTPException(status_code=422, detail=f"没有「{section}」这一段")
        columns = INTENT_SECTION_COLUMNS[section]

        try:
            cell_range = normalize_range(
                int(body["rowStart"]) - 1,
                int(body["rowEnd"]) - 1,
                int(body["colStart"]) - 1,
                int(body["colEnd"]) - 1,
            )
        except KeyError as exc:
            raise HTTPException(
                status_code=422, detail="需要 rowStart/rowEnd/colStart/colEnd 四个坐标"
            ) from exc
        except (TypeError, ValueError) as exc:
            raise HTTPException(status_code=422, detail=f"坐标不合法：{exc}") from exc

        rows = doc.get(section)
        if not isinstance(rows, list):
            rows = []
        text = format_for_clipboard(rows, columns, cell_range)
        return {
            "ok": True,
            "text": text,
            "cellCount": cell_range.cell_count,
            "range": cell_range.describe(),
        }

    @app.post("/api/intent/fill")
    def intent_fill(body: dict[str, Any]) -> dict[str, Any]:
        """填充柄：把源区域按规律延伸到目标行（v0.4 #3）。

        ## 为什么单独一个端点而不是复用 paste

        粘贴是「用户明确给了 N 行数据」，填充是「用户给了 1~2 行样例让程序推断」。
        前者照抄，后者要判断规律。混在一起会让「粘贴时不该推断」这条
        说不清楚（有人就会在粘贴里加推断，然后粘空值也变成复制上一行）。

        ## 为什么不做智能推断

        Excel 的智能填充猜错时用户往往不会发现（值看着都合理）。
        意图表里猜错一个机柜号，生成出来就是接错一柜的设备。
        只做数字等差、内置文本序列、照抄三种确定性规律。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        section = str(body.get("section") or "")
        if section not in INTENT_SECTION_COLUMNS:
            raise HTTPException(status_code=422, detail=f"没有「{section}」这一段")

        try:
            cell_range = normalize_range(
                int(body["rowStart"]) - 1,
                int(body["rowEnd"]) - 1,
                int(body["colStart"]) - 1,
                int(body["colEnd"]) - 1,
            )
        except KeyError as exc:
            raise HTTPException(
                status_code=422, detail="需要 rowStart/rowEnd/colStart/colEnd 四个坐标"
            ) from exc
        except (TypeError, ValueError) as exc:
            raise HTTPException(status_code=422, detail=f"坐标不合法：{exc}") from exc

        target = body.get("targetRow")
        if not isinstance(target, int) or target < 1:
            raise HTTPException(status_code=422, detail="targetRow 必须是正整数（1-based 终点行）")

        rows = doc.get(section)
        if not isinstance(rows, list):
            rows = []

        try:
            plan = plan_fill(rows, INTENT_SECTION_COLUMNS[section], cell_range, target - 1)
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

        written, notes = apply_fill(doc, section, plan, coerce=intent_coerce_cell)
        return {
            "ok": written > 0,
            "error": None,
            "written": written,
            "pattern": plan.pattern,
            "notes": notes,
            "doc": doc,
            "check": check_intent(doc).as_dict(),
        }

    @app.post("/api/intent/move")
    def intent_move(body: dict[str, Any]) -> dict[str, Any]:
        """改一台设备的 U 位（U 位图拖拽编辑，v0.4 #5）。

        ## 为什么这个端点不落库

        与 `/api/intent/row`、`/api/intent/paste` 一致：编辑器是「本地草稿」模型。
        用户拖动 5 次看效果、不满意就刷新，不该在库里留 5 个版本。
        满意了再点「保存为新版本」。

        ## 请求

        ```json
        {"doc": {...}, "channel": 2, "rack": 5, "instance": "N02-05-S07",
         "newU": 20, "allowOverlap": false, "currentU": 3, "targetRack": 7}

        `currentU` 只在拖「布局段自动排布的设备」时必需：那 663 台在台账里
        没有实例名，`instance` 传的是型号，必须靠「型号 + 当前 U 位」定位。
        逐台声明过的设备传了也无害（会先按 instance 精确匹配）。

        `targetRack` 是**跨机柜拖动**的目的机柜；不传就是原地调 U 位。
        只允许同通道内换柜（换通道等于换一组 ToR 服务它，那是重新规划）。
        ```

        ## 为什么响应里带 `impact`（v0.12）

        拖动会改掉**用户没选中的行**：换柜后这台在端口组里的位次变了，
        同组排在它后面的设备端口号整体前移。实测把通道 2 机柜 1 的
        `N02-01-S0` 挪到 5 号柜，**91 行端口号变了、6 行线长变了**。
        旧版响应里只有「已移到 5 号柜 3U」——用户以为只动了一台。

        所以每次移动都把影响面一起算出来（实测约 +75ms，仍在 NFR-1
        的 200ms 内）。`deep=false` 可跳过线长重算，只比端口与位置。

        ## 几何取不到时不能 500

        `move` 原本完全不依赖平面图。加影响面后若源文件缺失就报错，
        等于把一个纯编辑操作变成了「必须先有 Excel」。所以几何是**可选**的：
        取不到时 `lengthChecked=false`，界面据此说「线长未校验」
        而不是「线长没变」—— 这两句话对用户的意义完全不同。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        try:
            channel_no = int(body["channel"])
            rack_no = int(body["rack"])
            new_u = int(body["newU"])
        except (KeyError, TypeError, ValueError) as exc:
            raise HTTPException(
                status_code=422, detail="channel / rack / newU 都必须是整数"
            ) from exc
        instance = body.get("instance")
        if not isinstance(instance, str) or not instance:
            raise HTTPException(status_code=422, detail="instance 必须是非空字符串")

        current_u_raw = body.get("currentU")
        current_u = int(current_u_raw) if isinstance(current_u_raw, int) else None
        target_raw = body.get("targetRack")
        target_rack = int(target_raw) if isinstance(target_raw, int) else None
        geo = _geometry_or_none()
        report = preview_move(
            doc,
            channel_no=channel_no,
            rack_no=rack_no,
            instance=instance,
            new_u=new_u,
            allow_overlap=bool(body.get("allowOverlap")),
            current_u=current_u,
            target_rack_no=target_rack,
            geometry=geo,
            deep=geo is not None and body.get("deep", True) is not False,
        )
        result = report.move
        impact = impact_payload(report)
        # `impact.doc` 与 `result.doc` 是同一份，只保留一处，避免响应体里
        # 两份 159KB 的意图文档（前端要重新解析，还会以为有两份不同文档）。
        impact.pop("doc", None)
        return {
            "ok": result.ok,
            "error": result.error,
            "section": result.section,
            "rowNo": result.row_no,
            "overlaps": list(result.overlaps),
            "uSpan": result.u_span,
            "rackNo": result.rack_no,
            "doc": result.doc,
            "check": check_intent(result.doc).as_dict(),
            "impact": impact,
        }

    @app.post("/api/intent/move-batch")
    def intent_move_batch(body: dict[str, Any]) -> dict[str, Any]:
        """一批设备整体平移 `deltaU` 个 U（可同时换机柜）。

        ## 为什么不是一个一个调 `/api/intent/move`

        现场动作是「这一摞往下挪 8 格」。逐个调会失败：第 2 台校验时第 1 台
        已经移走、但第 3~8 台还站在旧位置，两边都算不对 —— 8 台整体下移
        时每台的新位置恰好是下一台的旧位置，逐台来一台都动不了。
        批量端点先把这一批从占用清单里整体摘掉，再统一校验。

        ## 请求

        ```json
        {"doc": {...},
         "targets": [{"channel":2,"rack":1,"instance":"计算节点A（ARM）","currentU":3},
                     {"channel":2,"rack":1,"instance":"计算节点A（ARM）","currentU":7}],
         "deltaU": 8, "targetRack": 5, "allowOverlap": false}
        ```

        `deltaU` 是**相对量**而非各自的新 U 位：给每台单独指定新位置，
        用户要手算保持间距，算错就是设备叠一起；delta 天然保住间距。

        `currentU` 对布局段排布的设备必需（同柜 8 台同名，只能靠 U 位区分）。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        raw_targets = body.get("targets")
        if not isinstance(raw_targets, list) or not raw_targets:
            raise HTTPException(status_code=422, detail="targets 必须是非空数组")
        try:
            delta_u = int(body["deltaU"])
        except (KeyError, TypeError, ValueError) as exc:
            raise HTTPException(status_code=422, detail="deltaU 必须是整数") from exc

        targets: list[DeviceTarget] = []
        for idx, raw in enumerate(raw_targets):
            if not isinstance(raw, dict):
                raise HTTPException(
                    status_code=422, detail=f"targets[{idx}] 必须是对象"
                )
            inst = raw.get("instance")
            if not isinstance(inst, str) or not inst:
                raise HTTPException(
                    status_code=422, detail=f"targets[{idx}].instance 必须是非空字符串"
                )
            try:
                ch = int(raw["channel"])
                rk = int(raw["rack"])
            except (KeyError, TypeError, ValueError) as exc:
                raise HTTPException(
                    status_code=422,
                    detail=f"targets[{idx}] 的 channel / rack 必须是整数",
                ) from exc
            cur_raw = raw.get("currentU")
            targets.append(
                DeviceTarget(
                    channel_no=ch,
                    rack_no=rk,
                    instance=inst,
                    current_u=int(cur_raw) if isinstance(cur_raw, int) else None,
                )
            )

        target_raw = body.get("targetRack")
        geo = _geometry_or_none()
        report = preview_move_devices(
            doc,
            targets=targets,
            delta_u=delta_u,
            target_rack_no=int(target_raw) if isinstance(target_raw, int) else None,
            allow_overlap=bool(body.get("allowOverlap")),
            geometry=geo,
            deep=geo is not None and body.get("deep", True) is not False,
        )
        result = report.move
        impact = impact_payload(report)
        impact.pop("doc", None)
        return {
            "ok": result.ok,
            "error": result.error,
            "moved": [
                {"instance": i, "fromRack": fr, "fromU": fu, "toRack": tr, "toU": tu}
                for i, fr, fu, tr, tu in result.moved
            ],
            "overlaps": list(result.overlaps),
            "doc": result.doc,
            "check": check_intent(result.doc).as_dict(),
            "impact": impact,
        }

    @app.get("/api/intent/editable")
    def intent_editable() -> dict[str, Any]:
        """哪些设备可以在 U 位图上拖 —— 前端据此决定方块是否可拖。

        ## 为什么必须由后端算

        判据是「这台设备在意图里有没有逐台声明」。663 台（79.5%）是布局段
        按槽位序列自动排布的，**拖了也没有字段可写**，重新生成会跳回原位。
        如果前端自己按「有没有在 switches 段里」判断，两套规则又会长歪。

        返回 `{(通道,机柜,实例名): 段名}` 的扁平数组形式，
        因为 JSON 的键必须是字符串，用 `"2|5|N02-05-S07"` 拼键再拆回很脆。
        """
        intent = _load_intent_file(_intent_from_env())
        items = editable_items(intent_to_dict(intent))
        return {
            "items": items,
            "total": len(items),
            "bySection": dict(
                sorted(Counter(x["section"] for x in items).items())
            ),
        }

    @app.post("/api/intent/impact")
    def intent_impact(body: dict[str, Any]) -> dict[str, Any]:
        """**干跑**：这次拖动会动到哪些行 —— 不落库、不改 doc。

        ## 为什么要有独立端点

        界面在拖的过程中就能提示「松手会改掉 91 行端口号」，用户可以选择
        取消。`/api/intent/move` 也能算出同一份影响面，但它是**已经改了**
        才告诉你 —— 那时候用户只能撤销，而撤销要重新算一遍。

        这不是性能优化，是**知情同意**：现场按表插线的人不是操作电脑的人，
        改 91 行端口号这件事必须在动手之前说出来。

        ## 请求

        ```json
        {"doc": {...},
         "move": {"channel":2,"rack":1,"instance":"计算节点A（ARM）",
                  "newU":3,"currentU":3,"targetRack":5},
         "deep": true}
        ```

        或批量形式：

        ```json
        {"doc": {...},
         "targets": [{"channel":2,"rack":1,"instance":"计算节点A（ARM）","currentU":3}],
         "deltaU": 8, "targetRack": 5}
        ```

        ## `deep` 的取舍

        `deep=true`（默认）连**线长**一起重算，实测约 95ms；`deep=false`
        只比端口与位置，约 55ms。拖拽过程里每动一下都问一次的话，
        前者会明显卡手（尤其触控板拖动会连发几十次移动事件）。
        所以界面策略是：**拖的过程中 `deep=false`，松手前最后一次 `deep=true`**。
        """
        doc = body.get("doc")
        if not isinstance(doc, dict):
            raise HTTPException(status_code=422, detail="请求体需要 doc 字段（意图对象）")
        geo = _geometry_or_none()
        deep = geo is not None and body.get("deep", True) is not False

        raw_targets = body.get("targets")
        if isinstance(raw_targets, list) and raw_targets:
            try:
                delta_u = int(body["deltaU"])
            except (KeyError, TypeError, ValueError) as exc:
                raise HTTPException(status_code=422, detail="deltaU 必须是整数") from exc
            targets: list[DeviceTarget] = []
            for idx, raw in enumerate(raw_targets):
                if not isinstance(raw, dict):
                    raise HTTPException(
                        status_code=422, detail=f"targets[{idx}] 必须是对象"
                    )
                inst = raw.get("instance")
                if not isinstance(inst, str) or not inst:
                    raise HTTPException(
                        status_code=422,
                        detail=f"targets[{idx}].instance 必须是非空字符串",
                    )
                try:
                    ch = int(raw["channel"])
                    rk = int(raw["rack"])
                except (KeyError, TypeError, ValueError) as exc:
                    raise HTTPException(
                        status_code=422,
                        detail=f"targets[{idx}] 的 channel / rack 必须是整数",
                    ) from exc
                cur_raw = raw.get("currentU")
                targets.append(
                    DeviceTarget(
                        channel_no=ch,
                        rack_no=rk,
                        instance=inst,
                        current_u=int(cur_raw) if isinstance(cur_raw, int) else None,
                    )
                )
            target_raw = body.get("targetRack")
            report = preview_move_devices(
                doc,
                targets=targets,
                delta_u=delta_u,
                target_rack_no=int(target_raw) if isinstance(target_raw, int) else None,
                allow_overlap=bool(body.get("allowOverlap")),
                geometry=geo,
                deep=deep,
            )
            payload = impact_payload(report)
            payload.pop("doc", None)
            return payload

        move = body.get("move")
        if not isinstance(move, dict):
            raise HTTPException(
                status_code=422,
                detail="请求体需要 move 对象（单台）或 targets 数组（批量）",
            )
        try:
            channel_no = int(move["channel"])
            rack_no = int(move["rack"])
            new_u = int(move["newU"])
        except (KeyError, TypeError, ValueError) as exc:
            raise HTTPException(
                status_code=422, detail="move.channel / rack / newU 都必须是整数"
            ) from exc
        instance = move.get("instance")
        if not isinstance(instance, str) or not instance:
            raise HTTPException(status_code=422, detail="move.instance 必须是非空字符串")
        cur_raw = move.get("currentU")
        tgt_raw = move.get("targetRack")
        report = preview_move(
            doc,
            channel_no=channel_no,
            rack_no=rack_no,
            instance=instance,
            new_u=new_u,
            allow_overlap=bool(move.get("allowOverlap")),
            current_u=int(cur_raw) if isinstance(cur_raw, int) else None,
            target_rack_no=int(tgt_raw) if isinstance(tgt_raw, int) else None,
            geometry=geo,
            deep=deep,
        )
        payload = impact_payload(report)
        payload.pop("doc", None)
        return payload

    # ---------------------------------------------------- 跨版本 / 跨项目对比
    @app.get("/api/compare/stock-files")
    def compare_stock_files() -> dict[str, Any]:
        """可用于对比的存量表清单（仓库 `信息港部署上架/` 里扫出来的）。

        ## 为什么清单要后端给而不是前端写死

        用户会往目录里丢新的 Excel。写死一份列表，第二天新版本就选不到。
        扫描成本约 0.3s（只看表头不读数据），足够每次进页面刷一次。
        """
        files = list_stock_files()
        return {
            "files": [f.as_dict() for f in files],
            "total": len(files),
            "directory": str(stock_dir()),
        }

    @app.get("/api/compare/stocks")
    def compare_stocks_endpoint(
        leftPath: str = Query(..., alias="leftPath"),
        leftSheet: str = Query(..., alias="leftSheet"),
        rightPath: str = Query(..., alias="rightPath"),
        rightSheet: str = Query(..., alias="rightSheet"),
        limit: int = Query(20, ge=1, le=200),
    ) -> dict[str, Any]:
        """比两个存量 sheet（任意两版 Excel，含同一文件内的「（旧）」存档）。

        ## 为什么允许传任意路径

        目录里还躺着 `政务外网区交换机连线表-0606.xlsx` 与
        `11+12冷通道（示例）.xlsx` —— 它们是**别的项目**（不同机房/规模），
        不该出现在「版本对比」下拉里，但用户确实会想拿它们跟主体比
        接线风格或复用配置。所以清单只列主体，路径参数不做白名单限制
        （本地单机工具，能读到文件就是用户的文件）。

        路径不存在时返回 404 并带上路径原文：用户手打时多半是笔误。
        """
        try:
            return compare_files(
                (leftPath, leftSheet),
                (rightPath, rightSheet),
                limit=limit,
            )
        except FileNotFoundError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc
        except KeyError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc
        except ValueError as exc:
            raise HTTPException(status_code=422, detail=str(exc)) from exc

    @app.get("/api/compare/projects")
    def compare_projects_endpoint(
        left: str = Query(..., description="项目名"),
        right: str = Query(..., description="项目名"),
        limit: int = Query(20, ge=1, le=200),
    ) -> dict[str, Any]:
        """比两个项目的**最新版本**（如「一期 vs 二期」「403 机房 vs 404 机房」）。

        只比最新版：跨项目问的是"现在差在哪"，不是考古。
        要比同一项目的历史版本用 `/api/projects/{name}/diff`。
        """
        store = _project_store()
        try:
            return compare_project_latest(store, left, right, limit=limit)
        except ValueError as exc:
            raise HTTPException(status_code=404, detail=str(exc)) from exc

    @app.get("/api/policies")
    def policies() -> dict[str, Any]:
        views, _ = views_and_report()
        return policy_payload(views)

    @app.get("/api/design")
    def design(
        limit: int = Query(200, ge=1, le=5000),
        compare: bool = Query(True, description="与已导入的存量布线表比对"),
        project: str = Query("", description="从项目库里读意图（按名字）；空=用库里最新的项目"),
        version: int | None = Query(None, description="版本号；空=最新"),
        source: str = Query(
            "",
            description="强制意图来源：`db` 项目库 / `file` 磁盘文件。空=自动（优先库）",
        ),
    ) -> dict[str, Any]:
        """接线意图 → 布线表（v0.3 主功能）。

        ## 为什么同时返回「意图体积」和「生成行」

        这个功能的价值全在**对比**里：949 行意图 → 5127 行连线。
        只给生成结果，用户不知道省了多少事；只给统计，用户不敢信。
        两个一起给，界面才能说"你写 949 行，机器补 5127 行"。

        ## 为什么 geometry 从当前已加载项目里取

        线长的 N/O 依赖平面图。项目加载时已经解析过几何快照，
        复用它可以避免为了生成再读一遍 Excel（5127 行解析约 1.5 秒）。

        ## 意图来源：默认走库，磁盘文件是显式回退

        v0.4 之前意图只存在于磁盘上的 `intent.json`，生成直接读它。
        加了版本库之后，用户在界面里改意图、保存成的**是新版本**，
        磁盘上那个文件并不会跟着变 —— 于是「改完保存了，布线表没变」。

        v0.4 第一版把「从库读」做成了 `?project=` 才生效，结果同一个坑
        换了个形态：用户刷新页面（URL 里没有 `project`）就退回读磁盘，
        看到的是首次导入的旧意图，而库里明明是刚存的新版。**默认值错了，
        功能就等于没修。**

        所以现在：①库里能挑出一个项目 → 默认读它；②库里什么都没有
        （全新部署、还没导入过）→ 读磁盘文件；③`source=file` 强制读磁盘
        （「我刚改完 intent.xlsx 想先看看效果」这种场景要能表达）。
        """
        store = _project_store()
        chosen = project
        source_format = "json"
        intent: Intent
        #: 用户明确要磁盘文件时不动库；否则库里有项目就用库。
        #
        # 例外：**显式设置了 `IDCBOX_INTENT` 环境变量时优先读那个文件**。
        # 设这个变量本身就是一句明确指令（「我就想看这份表」），此时若还
        # 悄悄改读库里的项目，用户会以为环境变量没生效 —— 而且是静默的。
        env_pinned = bool(os.environ.get("IDCBOX_INTENT"))
        if source == "file" or (env_pinned and source != "db" and not chosen):
            chosen = ""
        elif not chosen and source != "db":
            chosen = default_project_name(store) or ""

        if chosen:
            try:
                intent = load_intent_version(store, project_name=chosen, version=version)
            except ValueError as exc:
                raise HTTPException(status_code=404, detail=str(exc)) from exc
            source_format = "db"
        else:
            if source == "db":
                raise HTTPException(
                    status_code=404,
                    detail=(
                        "库里还没有任何带意图版本的项目。"
                        "先用 `idcbox project-save --name <名> --intent <文件>` 存一份，"
                        "或不传 source=db 让它读磁盘上的意图文件。"
                    ),
                )
            intent_path = _intent_from_env()
            if not intent_path.exists():
                raise HTTPException(
                    status_code=404,
                    detail=(
                        f"意图文件不存在：{intent_path}。"
                        "请在 projects/<项目>/intent.json（或 intent.xlsx）放一份，"
                        "或设置 IDCBOX_INTENT 指向它。"
                    ),
                )
            try:
                intent = _load_intent_file(intent_path)
            except ValueError as exc:
                # 版本不匹配 / JSON 坏了 / 表格缺必需的表 —— 都要原样告诉用户怎么修
                raise HTTPException(status_code=422, detail=str(exc)) from exc
            source_format = "xlsx" if intent_path.suffix.lower() in (".xlsx", ".xlsm") else "json"

        geometry = None
        stock_rows: list[dict[str, Any]] | None = None
        peer_model_of: dict[tuple[str, str, str], str] | None = None
        if compare:
            views, _ = views_and_report()
            geometry = views.project.geometry
            stock_rows, peer_model_of = _stock_rows_for_compare(views)

        payload = design_payload(
            intent,
            geometry,
            stock_rows=stock_rows,
            peer_model_of=peer_model_of,
            limit=limit,
            source_format=source_format,
            library=_library_for(chosen),
        )
        #: 告诉界面「这次到底读了哪儿」。只说 `sourceFormat=db` 不够 ——
        #: 用户库里可能有三个项目，他需要知道生成结果出自哪一个，
        #: 否则改完 A 项目却看到 B 项目的结果，会以为保存没生效。
        payload["source"] = {
            "kind": source_format,
            "project": chosen or None,
            "version": version,
        }
        return payload

    _mount_web(app)
    return app


def _db_path() -> Path:
    """项目库位置。`IDCBOX_DB` 可覆盖（默认 `<仓库>/data/idcbox.db`）。

    与 CLI 用同一个环境变量名，好处是两种入口指向同一份数据 ——
    否则用户在界面里存的项目，命令行看不到，会以为是 bug。
    """
    raw = os.environ.get("IDCBOX_DB", "")
    if raw:
        return Path(raw)
    # main.py 在 idcbox/api/ 下，仓库根是 parents[3]
    return Path(__file__).resolve().parents[3] / "data" / "idcbox.db"


def _geometry_or_none():
    """平面图几何快照；源文件缺失时返回 None。

    ## 为什么「拖一台设备」不该因为 Excel 不在就失败

    `moveu` 是纯意图操作：它只改 `u` / `rack` 字段，完全不需要平面图。
    但影响面里的**线长变化**要几何（N/O 看机柜坐标）。如果几何取不到就
    让整个请求 500，等于把一个纯编辑操作绑死在「必须先有源 Excel」上 ——
    而用户在只有意图 JSON 的环境里改布局是完全合法的用法。

    取不到时返回 None，服务层就会给 `lengthChecked=false`，
    界面据此说「线长未校验」而不是「线长没变」。**这两句话必须能分开**：
    前者让用户知道去补源文件，后者会让他以为安全。
    """
    try:
        views, _ = _cache.get(_source_from_env())
        return views.project.geometry
    except Exception:  # noqa: BLE001 —— 任何加载失败都降级，不影响拖动本身
        return None


def _project_store():
    """装配仓储（每次请求新建引擎）。

    ## 为什么不用全局缓存

    引擎缓存要按 `(路径, mtime)` 失效，而 SQLite 的 WAL 会持续改 mtime
    （`-wal`/`-shm` 文件），按 mtime 判失效会频繁重建。
    建引擎本身很便宜（不连库），连接由 SQLAlchemy 池化，所以每次新建反而更稳。
    `init_schema` 幂等，顺带保证「用户第一次打开界面就把库建好」。
    """
    from ..infrastructure.db import create_db_engine, init_schema
    from ..infrastructure.db.repositories import ProjectRepository

    engine = create_db_engine(_db_path())
    init_schema(engine, note="api")
    return ProjectRepository(engine)


def _stock_rows_for_compare(
    views: ProjectViews,
) -> tuple[list[dict[str, Any]], dict[tuple[str, str, str], str]]:
    """存量布线表 → 比对用的精简行 + `键 → 对端型号` 归因表。

    ## 全量口径（v0.3 起）

    v0.2 时生成器只覆盖「交换机 → 服务器下行」4574 行，所以这里也只取那一部分，
    否则会报出 548 条「生成缺失」—— 而那些本就不在自动生成范围。

    v0.3 的 `ExplicitLink` 能把交换机互联与特殊机型也生成出来，生成器产出
    **5127 行 = 整张真实表的行数**。此时再过滤反而会让 553 行被误算成
    「生成侧多出来的」，比对结果看起来像有问题。所以改为全量。

    `nic` 字段两边都用「对端端口名」：交换机↔服务器行是 `万兆1`，
    交换机↔交换机行是 `100GE1/0/1`，与生成侧 `GeneratedLink.nic` 同口径。

    ## 为什么同时返回 `peer_model_of`

    缺失行需要告诉用户「这些是什么」而不是只给个数。
    对端型号就在存量行的 `link.b.device_model` 里，顺手建一张表比再扫
    一遍 5127 行便宜得多。
    """
    out: list[dict[str, Any]] = []
    peer_model_of: dict[tuple[str, str, str], str] = {}
    for idx, link in enumerate(views.project.links):
        if not link.a.device_name or not link.a.port_name:
            continue
        key = (link.a.device_name, link.a.port_name, link.b.port_name)
        # 12 行的对端型号列是空的（原表未填）。归因到「原表未填型号」，
        # 比归到「未知」诚实 —— 用户一看就知道要回去补那张表。
        peer_model_of[key] = link.b.device_model or "（原表未填型号）"
        out.append(
            {
                "localDevice": link.a.device_name,
                "localPort": link.a.port_name,
                "nic": link.b.port_name,
                #: ## 必须用原表 T 列的人工值，不能用 `res.calc_len_t`
                #:
                #: `calc_len_t` 是本系统按七公式**重算**的结果，5124/5127 行都有值。
                #: 原表 T 列（线缆长度1）是人工填的，**只有 638 行有值** ——
                #: 其余 4489 行那格本来就空白（策划只对跨机柜行填长度）。
                #:
                #: 早先版本错用 `calc_len_t` 做比对基准，导致「线长不一致」只报
                #: 10 条：绝大多数真实值是 None 被跳过，看起来几乎完美，其实
                #: 只比了零星几行。该比的是「人工填的 638 行，系统能复现多少」，
                #: 这个口径下吻合 581/638 = 91.07%，才是诚实的数字。
                "tLadderM": _raw_t_ladder(views, link),
                #: 原表的 N/O/Q 三列**人工输入值**。只给比对做归因用，不参与计算。
                #:
                #: 为什么必须带上：原表只有 P/R/S/T 是公式，N/O/Q 是人手填的。
                #: 判断「公式链准不准」必须先把这三个人工输入对齐 ——
                #: 输入不一致时公式当然给出不同结果，那是数据问题不是算法问题。
                #: 不带这三个字段，界面只能拿 5127 当分母、把 55 处人工差异
                #: 说成"算法算错"，等于误导用户去怀疑一个没问题的东西。
                "nCrossRaw": _raw_num(views, link, "n_cross"),
                "oGapRaw": _raw_num(views, link, "o_gap"),
                "qInRackRaw": _raw_num(views, link, "q_in_rack"),
                # 保留原始行号，方便前端「跳到我表里那行」
                "stockRowNo": idx + 1,
            }
        )
    return out, peer_model_of


def _raw_num(views: ProjectViews, link: Any, field: str) -> int | None:
    """原表某列的人工值；空白或非数字（如 `待定`、`25G-SR`）返回 None。

    ## 为什么不用 float(v) 一把梭

    原表这几列混着备注文本：`25G-SR` 被写进了 N 列、`设备自带` 写进了 Q 列、
    还有 `待定`。硬转会抛异常，静默转 0 会让它们看起来像"人填了 0 而系统算错"。
    返回 None 表示「这一格没有可比数值」，三类语义严格区分。
    """
    raw = views.raw_by_key.get(link.stable_key) or {}
    value = raw.get(field)
    if value is None or value == "":
        return None
    try:
        return int(float(str(value).strip()))
    except (TypeError, ValueError):
        return None


def _raw_t_ladder(views: ProjectViews, link: Any) -> int | None:
    """原表 T 列（线缆长度1）的人工值；空白或非数字返回 None。

    只取数字：原表里存在 `待定` 这类文本，硬转 int 会抛异常，
    静默转 0 会让它看起来「算错了」。
    """
    raw = views.raw_by_key.get(link.stable_key) or {}
    value = raw.get("t_ladder")
    if value is None or value == "":
        return None
    try:
        return int(float(str(value).strip()))
    except (TypeError, ValueError):
        return None


def _mount_web(app: FastAPI) -> None:
    """托管前端构建产物（单进程部署形态）。

    ## 为什么由 FastAPI 托管而不是 nginx
    v0.2 的部署目标是"双击一个 exe / 一条命令启动，浏览器打开就能用"。
    引入 nginx 会让离线包多一个需要单独下载的二进制。FastAPI 的
    StaticFiles 够用（实测 953KB JS + 303KB CSS，单机静态响应 <5ms）。

    生产环境仍推荐 nginx（gzip + 缓存头 + 反向代理），
    但那是 v1.0 的部署选项，不是 v0.2 的必需品。

    ## 为什么 SPA 用 catch-all 而不是只挂 /
    刷新 `/cables` 这类前端路由时，浏览器请求的是 `/cables`，
    后端没有这个 API → 必须回退到 index.html 由前端路由接管。
    否则用户按 F5 就是 404。
    """
    from pathlib import Path

    from fastapi.responses import FileResponse
    from fastapi.staticfiles import StaticFiles

    dist = Path(__file__).resolve().parents[3] / "web" / "dist"
    if not dist.is_dir():
        return  # 未构建前端时不挂载，API 照常可用

    assets = dist / "assets"
    if assets.is_dir():
        app.mount("/assets", StaticFiles(directory=assets), name="assets")

    index = dist / "index.html"

    @app.get("/{full_path:path}", include_in_schema=False)
    def spa(full_path: str) -> FileResponse:
        # 静态文件直出（favicon 等），其余一律交给 SPA
        candidate = dist / full_path
        if full_path and candidate.is_file() and dist in candidate.resolve().parents:
            return FileResponse(candidate)
        return FileResponse(index)


app = create_app()
