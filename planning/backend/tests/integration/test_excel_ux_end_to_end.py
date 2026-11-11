"""Excel 化交互层的端到端验收（v0.4 #3 的复制出 / 区域选择 / 填充柄）。

## 这个文件守护什么

`tests/unit/test_clipboard.py` 已经覆盖了「TSV 转义对不对」「等差算得对不对」。
但那些用例是拿手写的小数据喂域函数 —— 它们证明不了下面三件事：

1. **复制出的文本能原样粘回 Excel**。转义规则写错时域层单测仍可能过
   （比如两边都用同一个错规则），但真实 Excel 会把它拆成一个格子。
2. **复制的是「眼前这份草稿」，不是「上次保存的版本」**。这是最容易
   写错的地方：端点若自己从磁盘/库读 doc，用户改了 10 格还没保存时
   复制出去的是旧内容 —— 文本看起来也很像，用户不会发现。
3. **填充真的改了文档的对应字段**，且不覆盖源区。

## 为什么这两个端点都收 `doc`

与 `/api/intent/paste`、`/api/intent/row` 一致：编辑器里是本地草稿。
若端点自己读文件，就出现上面第 2 条那种「复制了但内容不是眼前这份」。
真实数据里 V13 有 128 台交换机，`switches` 段第 1~3 行的内容与
保存过的版本可能完全不同 —— 正好用来验。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from idcbox.api.main import create_app

ROOT = Path(__file__).resolve().parents[3]
INTENT_FULL = ROOT / "projects" / "v13" / "intent_full.json"
DATA_DB = ROOT / "data" / "idcbox.db"

pytestmark = pytest.mark.skipif(
    not INTENT_FULL.exists(), reason="需要真实意图文件（只在完整仓库里跑）"
)


@pytest.fixture()
def client(tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> TestClient:
    """指向临时库，绝不碰用户的 data/idcbox.db。"""
    monkeypatch.setenv("IDCBOX_DB", str(tmp_path / "test.db"))
    return TestClient(create_app())


@pytest.fixture(scope="module")
def doc() -> dict:
    return json.loads(INTENT_FULL.read_text(encoding="utf-8"))


class TestCopyOut:
    """复制出：文本可被 Excel 认，且复制的就是传上来的这份草稿。"""

    def test_copies_requested_block(self, client: TestClient, doc: dict) -> None:
        r = client.post(
            "/api/intent/copy",
            json={
                "doc": doc,
                "section": "switches",
                "rowStart": 1,
                "rowEnd": 3,
                "colStart": 1,
                "colEnd": 3,
            },
        )
        assert r.status_code == 200, r.text
        data = r.json()
        assert data["ok"] is True
        assert data["cellCount"] == 9, "3 行 × 3 列"
        assert "第 1~3 行" in data["range"]
        # Excel 的换行是 \r\n；只给 \n 时 Windows 版 Excel 会把整块
        # 粘进一个单元格（实测过的真实事故）。
        lines = data["text"].split("\r\n")
        assert len(lines) == 3
        for line in lines:
            assert len(line.split("\t")) == 3
        # 第 1 行第 1 列必须是 switches[0].instance
        assert lines[0].split("\t")[0] == doc["switches"][0]["instance"]

    def test_reflects_unsaved_draft_not_saved_version(
        self, client: TestClient, doc: dict
    ) -> None:
        """复制出必须反映传上来的 doc，而不是库里存的那版。

        把第一台交换机的实例名改成一个不可能存在的值，若端点偷偷读了
        磁盘/库，返回的文本里就不会有这个值 —— 这就是本用例的判据。
        """
        draft = json.loads(json.dumps(doc))  # 深拷贝
        draft["switches"][0]["instance"] = "未保存的草稿名-ZZZ"
        r = client.post(
            "/api/intent/copy",
            json={
                "doc": draft,
                "section": "switches",
                "rowStart": 1,
                "rowEnd": 1,
                "colStart": 1,
                "colEnd": 2,
            },
        )
        assert r.status_code == 200
        assert "未保存的草稿名-ZZZ" in r.json()["text"]

    def test_reversed_coordinates_are_normalized(
        self, client: TestClient, doc: dict
    ) -> None:
        """从右下往左上拖是常见操作，不该报错也不该少一格。"""
        r = client.post(
            "/api/intent/copy",
            json={
                "doc": doc,
                "section": "switches",
                "rowStart": 3,
                "rowEnd": 1,
                "colStart": 3,
                "colEnd": 1,
            },
        )
        assert r.status_code == 200
        data = r.json()
        assert data["cellCount"] == 9
        assert "第 1~3 行 × 第 1~3 列" == data["range"]

    def test_missing_doc_is_422(self, client: TestClient) -> None:
        r = client.post("/api/intent/copy", json={"section": "switches"})
        assert r.status_code == 422

    def test_unknown_section_is_422(self, client: TestClient, doc: dict) -> None:
        r = client.post(
            "/api/intent/copy",
            json={
                "doc": doc,
                "section": "不存在的一段",
                "rowStart": 1,
                "rowEnd": 1,
                "colStart": 1,
                "colEnd": 1,
            },
        )
        assert r.status_code == 422

    def test_beyond_data_returns_empty_cells_not_error(
        self, client: TestClient, doc: dict
    ) -> None:
        """拖出来的选区超出数据末尾时，空行给空串，不要 500。"""
        n = len(doc["switches"])
        r = client.post(
            "/api/intent/copy",
            json={
                "doc": doc,
                "section": "switches",
                "rowStart": n,
                "rowEnd": n + 5,
                "colStart": 1,
                "colEnd": 2,
            },
        )
        assert r.status_code == 200
        assert r.json()["ok"] is True


class TestFillHandle:
    """填充柄：改到正确字段、不覆盖源区、规律可解释。"""

    def _doc(self) -> dict:
        """两行源 + 三行空壳（只差 u 没填）。

        ## 为什么目标行要预先摆好

        填充柄只写**选中那一列**（与 Excel 一致：选一列拖，只填那一列）。
        若目标行整行空白，填完 `u` 后这行依然缺 instance，
        `check_intent` 会因为 `SW_INSTANCE_EMPTY` 提前 `continue`，
        连 `u` 越界都报不出来 —— 这不是填充的 bug，是意图本来就残。
        真实用法里用户是先加好行、再拖 U 位的。
        """
        return {
            "switches": [
                {"instance": "SW-1", "model": "M", "channel": 2, "rack": 1, "u": 38},
                {"instance": "SW-2", "model": "M", "channel": 2, "rack": 2, "u": 37},
                {"instance": "SW-3", "model": "M", "channel": 2, "rack": 3},
                {"instance": "SW-4", "model": "M", "channel": 2, "rack": 4},
                {"instance": "SW-5", "model": "M", "channel": 2, "rack": 5},
            ]
        }

    def test_numeric_fill_extends_and_keeps_source(
        self, client: TestClient
    ) -> None:
        r = client.post(
            "/api/intent/fill",
            json={
                "doc": self._doc(),
                "section": "switches",
                "rowStart": 1,
                "rowEnd": 2,
                "colStart": 5,
                "colEnd": 5,
                "targetRow": 5,
            },
        )
        assert r.status_code == 200, r.text
        data = r.json()
        assert data["ok"] is True
        assert "递增" in data["pattern"]
        assert data["written"] == 3, "第 3~5 行，共 3 行"
        rows = data["doc"]["switches"]
        us = [row["u"] for row in rows]
        # 源区 38,37 不能被覆盖 —— 这是实现里真踩过的 bug
        # （起点用 target.row_start + 1，源区 3 行时会覆盖第 2 行）
        assert us[:2] == [38, 37], "源区必须原样"
        assert us[2:] == [36, 35, 34], "按 -1 递减"
        # 只写选中列：instance 必须还是原来那三个
        assert [r["instance"] for r in rows] == ["SW-1", "SW-2", "SW-3", "SW-4", "SW-5"]

    def test_text_sequence_fill(self, client: TestClient) -> None:
        doc = {
            "routes": [
                {"nodeModel": "A", "nic": "周一", "switchModel": "M"},
                {"nodeModel": "B", "nic": "周二", "switchModel": "M"},
            ]
        }
        r = client.post(
            "/api/intent/fill",
            json={
                "doc": doc,
                "section": "routes",
                "rowStart": 1,
                "rowEnd": 2,
                "colStart": 2,
                "colEnd": 2,
                "targetRow": 4,
            },
        )
        assert r.status_code == 200
        data = r.json()
        assert "序列" in data["pattern"]
        nics = [row["nic"] for row in data["doc"]["routes"]]
        assert nics == ["周一", "周二", "周三", "周四"]

    def test_multi_row_multi_col_source_is_422(self, client: TestClient) -> None:
        """2 行 × 2 列的源区域看不出该按行还是按列延伸，必须拒绝。"""
        doc = {"switches": [{"u": 1, "rack": 1}, {"u": 3, "rack": 2}]}
        r = client.post(
            "/api/intent/fill",
            json={
                "doc": doc,
                "section": "switches",
                "rowStart": 1,
                "rowEnd": 2,
                "colStart": 1,
                "colEnd": 2,
                "targetRow": 6,
            },
        )
        assert r.status_code == 422
        assert "单行或单列" in r.json()["detail"]

    def test_target_before_source_is_rejected(self, client: TestClient) -> None:
        r = client.post(
            "/api/intent/fill",
            json={
                "doc": self._doc(),
                "section": "switches",
                "rowStart": 1,
                "rowEnd": 2,
                "colStart": 5,
                "colEnd": 5,
                "targetRow": 1,
            },
        )
        assert r.status_code == 422

    def test_fill_returns_check_so_ui_can_show_errors(
        self, client: TestClient
    ) -> None:
        """填出坏数据（如 u 超出 1..42）时，返回的 check 里必须能看见。"""
        doc = {
            "switches": [
                {"instance": "S1", "model": "M", "channel": 2, "rack": 1, "u": 40},
                {"instance": "S2", "model": "M", "channel": 2, "rack": 2, "u": 42},
                # 目标行必须预先有 instance，否则 `SW_INSTANCE_EMPTY` 会让
                # 校验提前 continue，U 越界报不出来（见 `_doc` 的说明）。
                {"instance": "S3", "model": "M", "channel": 2, "rack": 3},
                {"instance": "S4", "model": "M", "channel": 2, "rack": 4},
                {"instance": "S5", "model": "M", "channel": 2, "rack": 5},
                {"instance": "S6", "model": "M", "channel": 2, "rack": 6},
            ]
        }
        r = client.post(
            "/api/intent/fill",
            json={
                "doc": doc,
                "section": "switches",
                "rowStart": 1,
                "rowEnd": 2,
                "colStart": 5,
                "colEnd": 5,
                "targetRow": 6,
            },
        )
        assert r.status_code == 200
        check = r.json()["check"]
        assert check["errorCount"] > 0, "u 递增到 44/46 必须报越界"
        codes = {i["code"] for i in check["issues"]}
        assert "U_RANGE" in codes


class TestCopyThenPasteRoundTrip:
    """复制出 → 粘回来，必须还是同一批数据。

    这是本功能最重要的一条：单看「复制对不对」和「粘贴对不对」都容易
    自证，只有往返才证明两边的转义规则是同一套。
    """

    def test_round_trip_through_endpoints(self, client: TestClient, doc: dict) -> None:
        copy = client.post(
            "/api/intent/copy",
            json={
                "doc": doc,
                "section": "switches",
                "rowStart": 1,
                "rowEnd": 4,
                "colStart": 1,
                "colEnd": 6,
            },
        )
        assert copy.status_code == 200
        text = copy.json()["text"]

        # 粘到一个空文档里
        paste = client.post(
            "/api/intent/paste",
            json={"doc": {"switches": []}, "section": "switches", "text": text},
        )
        assert paste.status_code == 200, paste.text
        rows = paste.json()["doc"]["switches"]
        assert len(rows) == 4

        src = doc["switches"][:4]
        keys = ("instance", "model", "channel", "rack", "u", "uSpan")
        for got, want in zip(rows, src, strict=True):
            for k in keys:
                if k in want:
                    assert str(got.get(k)) == str(want[k]) or got.get(k) == want[k], (
                        f"{k} 往返后不一致：{got.get(k)!r} vs {want[k]!r}"
                    )
