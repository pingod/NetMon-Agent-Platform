"""3D 场景端点的端到端用例（`GET /api/scene`）。

## 为什么要在真数据上测

单元测试用的是手工造的两个柜，测不出「362 个柜会不会互相穿模」这类
整体性问题 —— 而那个问题正好是真 bug（两排柜只隔 600mm，柜深 1200mm）。

所以这里在 V13 真数据上断言：
- 362 个柜的 (x,z) 两两不同（无穿模、无房间重叠）
- 两个房间在 x 轴上确实分开
- 834 台设备全部带 U 范围与类别

## 为什么不用 TestClient 起 app 就够

`/api/scene` 只读已缓存的 `views`，不碰数据库、不写文件。
起一次 app 约 0.6s（要读 5127 行 Excel），所以 module 级 fixture。
"""

from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

pytestmark = pytest.mark.skipif(
    not __import__("pathlib").Path(
        __file__
    ).resolve().parents[3].joinpath(
        "信息港部署上架/V13(内部)甘肃信息港-设备上架规划-20260714.xlsx"
    ).exists(),
    reason="需要真实 V13 源文件（只在完整仓库里跑）",
)


@pytest.fixture(scope="module")
def client(tmp_path_factory: pytest.TempPathFactory):
    import os

    db = tmp_path_factory.mktemp("db") / "t.db"
    prev = os.environ.get("IDCBOX_DB")
    os.environ["IDCBOX_DB"] = str(db)
    from idcbox.api.main import create_app

    try:
        with TestClient(create_app()) as c:
            yield c
    finally:
        if prev is None:
            os.environ.pop("IDCBOX_DB", None)
        else:
            os.environ["IDCBOX_DB"] = prev


@pytest.fixture(scope="module")
def scene(client: TestClient) -> dict:
    res = client.get("/api/scene")
    assert res.status_code == 200, res.text
    return res.json()


class TestTotals:
    def test_racks_and_devices(self, scene: dict) -> None:
        """362 个柜位、102 个有设备、834 台设备（型号归一化口径）。

        ## 834 而不是 824 或 836

        - **824** = 物理位置数（通道+机柜+U 位唯一）
        - **834** = 型号归一化后的实例数 ← 3D 用这个（每台设备画一个盒子）
        - **836** = 型号原样

        3D 画的是「实例」，所以取 834。用 824 会少画 10 台（同位置叠放），
        用 836 会多画 2 个空格变体假设备。**口径错了画面就会多/少设备，
        而用户没法一眼看出来**。
        """
        t = scene["totals"]
        assert t["racks"] == 362
        assert t["racksWithDevices"] == 102
        assert t["devices"] == 834
        assert t["rooms"] == 2

    def test_devices_list_length_matches_total(self, scene: dict) -> None:
        """★ 计数与列表长度必须一致（计数被样本覆盖是老坑，见 AGENTS §6.2）。"""
        assert len(scene["devices"]) == scene["totals"]["devices"]
        assert len(scene["racks"]) == scene["totals"]["racks"]


class TestNoOverlap:
    def test_no_two_cabinets_share_xz(self, scene: dict) -> None:
        """★ 362 个柜的世界坐标 (x,z) 必须两两不同。

        这条守两个真 bug：
        ① 两排柜只隔 600mm（柜位格距）而柜深 1200mm → 穿模
        ② 两个房间都按「减本房最左列」算 x → 完全重叠（实测 146 个重复）
        """
        seen = set()
        dup = []
        for r in scene["racks"]:
            key = (round(r["x"]), round(r["z"]))
            if key in seen:
                dup.append((r["channel"], r["rack"], key))
            seen.add(key)
        assert not dup, f"{len(dup)} 个柜坐标重复，例如 {dup[:5]}"

    def test_rooms_separated_in_x(self, scene: dict) -> None:
        """两个房间在 x 轴上必须分开，不能叠在一起。"""
        rooms = {r["roomCode"]: r for r in scene["rooms"]}
        assert rooms["403"]["originX"] < rooms["404"]["originX"]
        # 403 的右边界不能越过 404 的左起点（含一条通道宽的空隙）
        assert (
            rooms["403"]["originX"] + rooms["403"]["widthMm"]
            <= rooms["404"]["originX"]
        )

    def test_z_range_covers_all_channels(self, scene: dict) -> None:
        """z 的最大值应接近「7 个通道 × 3600mm」= 25200mm。

        403 与 404 各有 7 条通道，所以柜的 z 最大值约 24600
        （最后一个通道的南侧柜中心）。
        """
        zs = [r["z"] for r in scene["racks"]]
        assert max(zs) > 24000
        assert min(zs) > 0


class TestRacks:
    def test_empty_racks_included(self, scene: dict) -> None:
        """★ 空柜也要在列表里 —— 施工图要能看出哪几个柜空着。"""
        empty = [r for r in scene["racks"] if r["deviceCount"] == 0]
        assert len(empty) == 362 - 102

    def test_rotation_only_0_or_180(self, scene: dict) -> None:
        """朝向只可能是 0 或 180（北侧朝南、南侧朝北）。

        出现别的值说明镜像逻辑被改坏了 —— 那会让整排柜门朝同一边，
        通道消失。
        """
        assert {r["rotationY"] for r in scene["racks"]} == {0.0, 180.0}

    def test_used_u_within_rack_height(self, scene: dict) -> None:
        """单柜已用 U 数不能超过 42。

        超过说明 `sum(size_u)` 把重叠设备重复计了 —— 真实数据有 11 对
        U 位重叠（规划占位），所以「单柜可能超过 42」是已知事实；
        这里只断言不出现离谱值（比如把整柜 8 台的 U 全加起来超过 2 倍）。
        """
        for r in scene["racks"]:
            assert r["usedU"] < 42 * 2, f"通道{r['channel']}机柜{r['rack']} usedU={r['usedU']}"


class TestDevices:
    def test_every_device_has_u_range(self, scene: dict) -> None:
        for d in scene["devices"]:
            assert d["startU"] >= 1
            assert d["endU"] >= d["startU"]
            assert d["sizeU"] == d["endU"] - d["startU"] + 1

    def test_every_device_has_known_category(self, scene: dict) -> None:
        """★ 类别必须是后端 `categorize()` 的六种之一。

        出现未知值会让前端配色兜底成 `other` —— 那台设备就变成灰色，
        与「真的没分类」区分不开。
        """
        known = {"core", "switch", "server", "storage", "security", "other"}
        assert {d["category"] for d in scene["devices"]} <= known

    def test_category_distribution(self, scene: dict) -> None:
        """六类的实际分布。这个数字变了说明 `categorize()` 的判定顺序被动了。

        顺序是硬约束：「核心」必须在「交换机」之前（`业务核心交换机`
        两个词都有），「服务器密码机」必须归安全而不是服务器。
        """
        from collections import Counter

        c = Counter(d["category"] for d in scene["devices"])
        assert c["server"] == 657
        assert c["switch"] == 133
        assert c["storage"] == 20
        assert c["security"] == 14
        assert c["core"] == 6
        assert c["other"] == 4

    def test_device_keys_unique_per_rack_u(self, scene: dict) -> None:
        """`key` 在场景内必须唯一 —— 前端拿它做 React key 与选中标识。

        重复的 key 会让 React 复用错组件，表现为「点 A 设备 B 高亮」。
        """
        keys = [d["key"] for d in scene["devices"]]
        assert len(set(keys)) == len(keys)


class TestPayloadShape:
    def test_units_is_mm(self, scene: dict) -> None:
        assert scene["units"] == "mm"

    def test_constants_match_expectation(self, scene: dict) -> None:
        c = scene["constants"]
        assert c["mmPerU"] == 44.45
        assert c["cabinetWidth"] == 600
        assert c["cabinetDepth"] == 1200
        assert c["aisleWidth"] == 1200

    def test_payload_size_reasonable(self, client: TestClient) -> None:
        """载荷应在 100~400KB。超了说明把不该给的东西塞进来了。

        834 台设备 + 362 个柜，每条约 100 字节 → 约 120KB + 开销。
        超过 400KB 通常意味着有人把端口明细（12776 条）也塞进来了。
        """
        raw = client.get("/api/scene").content
        kb = len(raw) / 1024
        assert 100 < kb < 400, f"载荷 {kb:.0f}KB 超出预期区间"
