"""版本对比的端到端测试（v0.4 #6）。

## 这个模块守护什么

`CompareView` 截图暴露过一个真事故：界面报「新增 30 行 / 删除 30 行」，
但两个列表内容看起来一模一样，用户无法理解。

根因有两层：

1. **计数被 `limit` 截断**：`compare_stocks(limit=30)` 时样本截到 30 条，
   而 payload 用 `len(diff.added)` 当条数 → 4886 条新增被报成「30 行」。
   单元测试里原来的断言是 `total_changes >= 5`，`5 >= 5` 通过 —— 漏网了。
2. **抽象层级错了**：V12→V13 的真相是同一批交换机整体挪了 1U。
   按位置键（含 U 位）比对，U 一变每行都成了"新增/删除"；
   设备级聚合才是「56 台挪了 U 位」。

所以本模块断言三件事：
① 计数是真实值，不受 `limit` 影响
② 设备级聚合给出可读结论
③ 行级与设备级数字的差距有解释

## 用哪些文件比

默认比 V12 布线表 → V13 布线表。这一对是「U 位整体重排」的典型样本：
行级差异将近 5000，设备级只有 56 台挪位置。

另外断言 V12 布线表 ≡ V13「布线表（旧）」逐行完全一致 —— 这证明
V13 的真实改动是把 V12 原样归档，而不是"改了接线"。
"""

from __future__ import annotations

import os
from pathlib import Path
from typing import Any

import pytest
from fastapi.testclient import TestClient

from idcbox.api.main import create_app

ROOT = Path(__file__).resolve().parents[3]
STOCK_DIR = ROOT / "信息港部署上架"

pytestmark = pytest.mark.skipif(
    not STOCK_DIR.is_dir(),
    reason="需要真实的历史版本 Excel（只在完整仓库里跑）",
)


def _find(prefix: str) -> Path:
    """按 V 号前缀找文件。

    各版本的文件名后缀不统一（`V8甘肃信息港-设备上架点位图及连线表规划`、
    `V12(内部)甘肃信息港-设备上架规划`），硬编码会 `FileNotFoundError`。
    """
    hits = sorted(STOCK_DIR.glob(f"{prefix}*设备上架*.xlsx"))
    if not hits:
        pytest.skip(f"找不到 {prefix} 文件")
    return hits[0]


@pytest.fixture(scope="module")
def client(tmp_path_factory: pytest.TempPathFactory) -> Any:
    db = tmp_path_factory.mktemp("db") / "compare.db"
    os.environ["IDCBOX_DB"] = str(db)
    yield TestClient(create_app())
    os.environ.pop("IDCBOX_DB", None)


def _compare(
    client: TestClient, left: tuple[Path, str], right: tuple[Path, str], limit: int = 30
) -> dict[str, Any]:
    r = client.get(
        "/api/compare/stocks",
        params={
            "leftPath": str(left[0]),
            "leftSheet": left[1],
            "rightPath": str(right[0]),
            "rightSheet": right[1],
            "limit": limit,
        },
    )
    assert r.status_code == 200, r.text
    return r.json()


class TestStockFileList:
    def test_lists_cabling_sheets_only(self, client: TestClient) -> None:
        """只列设备上架系的连线/布线表，且当前版本排在前面。

        ## 为什么必须过滤

        `信息港部署上架/` 目录里还有 `政务外网区交换机连线表.xlsx` 与
        `11+12冷通道（示例）.xlsx` —— 它们**有**连线表但属于**别的项目**
        （不同机房、不同规模）。混进"版本对比"下拉会让用户误以为
        它们是同一份规划的历史版本，从而得出"完全一致/完全不同"的错误结论。
        """
        r = client.get("/api/compare/stock-files")
        assert r.status_code == 200
        data = r.json()
        names = [f["name"] for f in data["files"]]
        assert all("设备上架" in n for n in names), f"混进了别的项目：{names}"
        assert "政务外网" not in " ".join(names)
        assert "冷通道" not in " ".join(names)

    def test_current_plans_come_first(self, client: TestClient) -> None:
        """priority=0 的是「当前布线表」，排最前；`（旧）` 排最后。

        ## 排序错了会怎样

        `布线表（旧）` 同时命中「布线表」与「（旧）」两个特征。若按"取最小权重"
        它会得 0，排到当前版前面 —— 用户在下拉里第一眼看到存档版本，
        点进去得到的是几周前的数据。
        """
        data = client.get("/api/compare/stock-files").json()
        files = data["files"]
        assert files[0]["priority"] == 0
        assert "（旧）" not in files[0]["sheet"]
        # 存档标记在 **sheet 名** 里（`布线表（旧）`），文件名仍是同一个 V13 文件 ——
        # 第一版断言写成查 name，永远查不到。
        archived = [f for f in files if "（旧）" in f["sheet"]]
        assert archived, "应该有存档版本（布线表（旧））"
        assert archived[0]["priority"] > 0
        assert files[-1]["priority"] > 0, "存档版本必须排在最后"


class TestCountsNotTruncated:
    """计数绝不能被 `limit` 截断 —— 这是本模块最重要的一组用例。"""

    def test_count_is_real_not_sample_size(self, client: TestClient) -> None:
        """`limit=5` 时 `addedCount` 仍是 4886，不是 5。

        ## 事故回顾

        第一版 `diff_payload` 用 `len(diff.added)` 当条数，而 `added` 被
        `limit` 截断过。界面传 `limit=30` → 报「新增 30 行」。
        用户看到 30 会以为改动很小，实际是整表重排。

        这种"数字小了但看起来正常"的错最难发现 —— 没有报错、没有异常、
        界面渲染完全正常，只是数字悄悄小了两个数量级。
        """
        d = _compare(
            client, (_find("V12"), "布线表"), (_find("V13"), "布线表"), limit=5
        )
        assert d["addedCount"] > 1000, "计数必须是真实值（V12→V13 约 4886）"
        assert d["removedCount"] > 1000
        assert len(d["added"]) <= 5, "样本仍受 limit 限制"

    def test_sample_counts_expose_truncation(self, client: TestClient) -> None:
        """payload 必须显式告诉前端"我只给了 N 条样例"。

        前端要显示「另有 4878 行」这类提示，就需要知道样本被截过。
        """
        d = _compare(
            client, (_find("V12"), "布线表"), (_find("V13"), "布线表"), limit=3
        )
        sc = d["sampleCounts"]
        assert sc["added"] <= 3
        assert d["addedCount"] > sc["added"]


class TestDeviceLevel:
    """设备级聚合：把「4886 条新增」翻译成「56 台挪了 U 位」。"""

    def test_u_shift_is_reported_as_moved(self, client: TestClient) -> None:
        """V12→V13 的核心变化是同一批交换机整体挪 1U。

        逐行比：4886 新增 / 4890 删除 → 看起来像整个规划重做。
        设备级：56 台 moved + 2 台新增 + 54 台 changed → 才说得清。
        """
        d = _compare(client, (_find("V12"), "布线表"), (_find("V13"), "布线表"))
        dd = d["byDevice"]
        assert dd["counts"]["moved"] >= 50, "应识别出大批设备只是挪了 U 位"
        assert dd["counts"]["moved"] < 200, "不应把整表都算成挪位置"
        assert dd["counts"]["leftDevices"] > 100
        assert dd["counts"]["rightDevices"] > 100

    def test_moved_sample_mentions_old_and_new_u(self, client: TestClient) -> None:
        """moved 的描述必须同时给出旧 U 位、新 U 位、端口重合度。

        只有"某设备变了"没有信息量；施工队要知道"从哪挪到哪、几根线受影响"。
        """
        d = _compare(client, (_find("V12"), "布线表"), (_find("V13"), "布线表"))
        moved = d["byDevice"]["moved"]
        assert moved, "应该有挪位置的设备"
        first = moved[0]
        assert "→" in first, f"要给出旧→新：{first}"
        assert "端口不变" in first or "个端口" in first

    def test_counts_not_overwritten_by_samples(self, client: TestClient) -> None:
        """计数与样本用不同的键。

        第一版把计数展开进 payload 顶层，又用同名键写样本列表，
        后者覆盖前者 —— 前端拿到 `counts.moved` 是字符串数组。
        """
        d = _compare(client, (_find("V12"), "布线表"), (_find("V13"), "布线表"))
        dd = d["byDevice"]
        assert isinstance(dd["counts"]["moved"], int)
        assert isinstance(dd["moved"], list)

    def test_note_explains_the_gap(self, client: TestClient) -> None:
        """必须解释行级与设备级的数字差距。

        两个数字同屏出现而不解释，用户会问"到底差了多少条线"。
        真相是：U 位一变，按行比就是几十条新增+删除，实际一根线都没换。
        """
        d = _compare(client, (_find("V12"), "布线表"), (_find("V13"), "布线表"))
        note = d["byDevice"]["note"]
        assert note, "有行级差异时必须给解释"
        assert "U 位" in note or "U位" in note


class TestArchivedVersionIsIdentical:
    """V12 布线表 ≡ V13「布线表（旧）」—— 证明 V13 是原样归档。"""

    def test_archived_copy_is_byte_identical(self, client: TestClient) -> None:
        """两份完全一致 → 0 新增 0 删除 0 修改。

        ## 这条断言的价值

        它证明了「V13 的改动」不是"改了接线"，而是"把 V12 归档 · 补实例名列 ·
        删 4 条线"。如果这里报出几千条差异，说明比对键或表头识别出了问题 ——
        而用户会误以为 V13 大改过规划。
        """
        d = _compare(
            client,
            (_find("V12"), "布线表"),
            (_find("V13"), "布线表（旧）"),
            limit=10,
        )
        assert d["addedCount"] == 0, f"归档副本不该有新增，实得 {d['addedCount']}"
        assert d["removedCount"] == 0
        assert d["changedCount"] == 0
        assert d["isEmpty"] is True
        assert d["byDevice"]["isEmpty"] is True


class TestCrossProject:
    def test_unknown_project_returns_404(self, client: TestClient) -> None:
        r = client.get(
            "/api/compare/projects", params={"left": "不存在的项目", "right": "也不存在"}
        )
        assert r.status_code == 404

    def test_missing_file_returns_404(self, client: TestClient) -> None:
        r = client.get(
            "/api/compare/stocks",
            params={
                "leftPath": "/nonexistent/a.xlsx",
                "leftSheet": "布线表",
                "rightPath": str(_find("V13")),
                "rightSheet": "布线表",
            },
        )
        assert r.status_code == 404

    def test_bad_sheet_returns_404(self, client: TestClient) -> None:
        """sheet 名打错要 404，不是 500，也不是返回空结果。

        返回空结果会让用户以为"两份表完全一致"。
        """
        r = client.get(
            "/api/compare/stocks",
            params={
                "leftPath": str(_find("V13")),
                "leftSheet": "不存在的sheet",
                "rightPath": str(_find("V13")),
                "rightSheet": "布线表",
            },
        )
        assert r.status_code == 404
