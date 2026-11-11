"""施工图导出的端到端：HTTP 层能拿到合法 SVG / HTML。

## 这组用例为什么必须存在

`domain/exporting/svg.py` 的单元测试只能证明「给它数据它能画出合法的图」。
但导出走的是 HTTP，中间有三处容易错、且单测抓不到的地方：

1. **`Content-Type` 必须是 `image/svg+xml`**。FastAPI 返回 `str` 会默认
   `text/plain`，浏览器打开是一屏源码而不是图 —— 用户会以为「导出坏了」。
2. **筛选参数的语义要分清 None 和 []**。`_resolve_racks` 返回 `None` 表示
   「不筛选」、返回 `[]` 表示「筛了但没匹配上」。把两者弄混的结果是：
   用户打错柜号，拿到一张全部机柜的图，还以为筛成功了。
3. **SVG 必须能被 schema 解析**。少一个闭合标签就是白屏，而白屏不会
   在任何日志里留下痕迹。

## 为什么用真实 V13 数据

柜位重叠、U 位跨度、设备型号里的全角括号 —— 这些都是**真实数据里的形状**。
用构造的小样本测出来的转义逻辑，遇到 `计算节点A（ARM）` 这种名字才知道对不对。
"""

from __future__ import annotations

import xml.etree.ElementTree as ET
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from idcbox.api.main import create_app

pytestmark = pytest.mark.skipif(
    not (Path(__file__).resolve().parents[3] / "信息港部署上架").is_dir(),
    reason="需要真实 V13 源文件（只在完整仓库里跑）",
)

SVG_NS = "{http://www.w3.org/2000/svg}"


@pytest.fixture(scope="module")
def client(tmp_path_factory: pytest.TempPathFactory) -> TestClient:
    db = tmp_path_factory.mktemp("db") / "draw.db"
    import os

    os.environ["IDCBOX_DB"] = str(db)
    yield TestClient(create_app())
    os.environ.pop("IDCBOX_DB", None)


class TestContentType:
    """Content-Type 错了，图就变成了源码。"""

    def test_svg_endpoint_declares_svg_type(self, client: TestClient) -> None:
        r = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": 1})
        assert r.status_code == 200
        assert r.headers["content-type"].startswith("image/svg+xml")

    def test_html_endpoint_declares_html_type(self, client: TestClient) -> None:
        r = client.get("/api/draw/racks.html", params={"channel": 2, "rack": 1})
        assert r.status_code == 200
        assert r.headers["content-type"].startswith("text/html")

    def test_svg_is_inline_not_attachment(self, client: TestClient) -> None:
        """`inline` 才能在新标签页里直接看图；`attachment` 会弹下载框。

        现场人员是「点开看一眼、觉得对再打印」，弹下载框还得去找文件在哪。
        """
        r = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": 1})
        assert "inline" in r.headers.get("content-disposition", "")


class TestSvgValidity:
    """SVG 必须能过 XML schema 解析 —— 少一个闭合标签就是白屏。"""

    def test_svg_parses_as_xml(self, client: TestClient) -> None:
        body = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": "1,2"}).text
        root = ET.fromstring(body)
        assert root.tag == f"{SVG_NS}svg"
        assert root.get("viewBox") is not None

    def test_html_body_contains_svg_and_page_rule(self, client: TestClient) -> None:
        body = client.get("/api/draw/racks.html", params={"channel": 2, "rack": 1}).text
        assert "<svg" in body
        assert "@page" in body
        # 从 HTML 里抠出 SVG 也要能解析
        inner = body[body.index("<svg") : body.index("</svg>") + 6]
        ET.fromstring(inner)

    def test_svg_has_white_background(self, client: TestClient) -> None:
        body = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": 1}).text
        assert "#ffffff" in body

    def test_full_width_cjk_not_broken(self, client: TestClient) -> None:
        """真实设备名带全角括号 `（ARM）`，XML 解析器必须能接受。"""
        body = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": "1,2,3"}).text
        ET.fromstring(body)
        assert "计算节点" in body


class TestFilterSemantics:
    """None（不筛选）vs []（筛了没命中）必须分清。"""

    def test_no_filter_returns_all_racks(self, client: TestClient) -> None:
        """不传参数 = 全部有设备的机柜（102 个），不是 362 个柜位。

        空柜印在图上只是浪费纸，现场不会去看。
        """
        body = client.get("/api/draw/racks.svg").text
        assert body.count("机柜") > 100

    def test_single_channel_filter(self, client: TestClient) -> None:
        body = client.get("/api/draw/racks.svg", params={"channel": 1}).text
        # 只画通道 1，不应出现通道 2 的标签
        assert "通道1 / 机柜" in body
        assert "通道2 / 机柜" not in body

    def test_rack_list_filter(self, client: TestClient) -> None:
        body = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": "1,2"}).text
        assert "通道2 / 机柜1" in body
        assert "通道2 / 机柜2" in body
        assert "通道2 / 机柜3" not in body

    def test_chinese_comma_tolerated(self, client: TestClient) -> None:
        """用户从 Excel 复制柜号时带的是中文逗号。"""
        body = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": "1，2"}).text
        assert "通道2 / 机柜1" in body and "通道2 / 机柜2" in body

    def test_unknown_rack_is_404_not_empty_sheet(self, client: TestClient) -> None:
        """打错柜号必须 404。

        如果返回一张空图纸，用户会以为「这个柜真的没有设备」——
        而真相是他打错了。这两个结论对施工的影响完全不同。
        """
        assert client.get("/api/draw/racks.svg", params={"rack": "999"}).status_code == 404
        assert client.get("/api/draw/racks.html", params={"rack": "999"}).status_code == 404

    def test_non_numeric_rack_is_422(self, client: TestClient) -> None:
        r = client.get("/api/draw/racks.svg", params={"rack": "abc"})
        assert r.status_code == 422
        assert "整数" in r.json()["detail"]


class TestContentCorrectness:
    """图上画的必须和数据库里的一致 —— 这是施工图的底线。"""

    def test_rack_summary_matches_api(self, client: TestClient) -> None:
        """图上「N 台」必须等于 `/api/racks` 里该柜的 deviceCount。

        两处不一致意味着一处数据源被改坏了，而现场只会看到纸上的那个数。
        """
        racks = client.get("/api/racks").json()["racks"]
        target = next(r for r in racks if r["deviceCount"] > 0)
        ch, seq = target["channelNo"], target["seqNo"]

        body = client.get("/api/draw/racks.svg", params={"channel": ch, "rack": seq}).text
        assert f"{target['deviceCount']} 台" in body

    def test_overlap_is_marked_red(self, client: TestClient) -> None:
        """真实数据有 11 对 U 位重叠。至少其中一对必须在图上标红。"""
        racks = client.get("/api/racks").json()["racks"]
        # 找第一个有重叠的柜
        for r in racks:
            if r["deviceCount"] == 0:
                continue
            detail = client.get(
                "/api/racks/" + str(r["channelNo"]) + "/" + str(r["seqNo"])
            ).json()
            if detail.get("conflicts"):
                body = client.get(
                    "/api/draw/racks.svg",
                    params={"channel": r["channelNo"], "rack": r["seqNo"]},
                ).text
                assert "#cc0000" in body, f"通道{r['channelNo']}机柜{r['seqNo']} 有重叠但图上没标红"
                return
        pytest.skip("当前数据里没有 U 位重叠（数据被修过？）")

    def test_multi_row_stacks_vertically(self, client: TestClient) -> None:
        """整层 102 个柜、每页 8 个 = 13 行。高度必须随之增长。

        第一版把 13 行全画在同一个 y 上，导出后看起来只有一个柜 ——
        而 SVG 完全合法，靠代码审查几乎发现不了。
        """
        import re

        one = client.get("/api/draw/racks.svg", params={"channel": 2, "rack": "1"}).text
        many = client.get(
            "/api/draw/racks.svg", params={"channel": 2, "rack": "1,2,3,4,5,6,7,8,9"}
        ).text

        def height(svg: str) -> int:
            return int(re.search(r'height="(\d+)"', svg).group(1))  # type: ignore[union-attr]

        assert height(many) > height(one) * 1.5
