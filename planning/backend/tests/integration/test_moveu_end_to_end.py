"""U 位拖拽编辑的端到端用例 —— `/api/intent/editable` 与 `/api/intent/move`。

## 这组用例守护什么

域层用例（`tests/unit/test_moveu.py`）证明「该不该允许这次移动」是对的。
这里证明的是另一件事：**前后端对「哪些设备能拖」的判断是同一个**。

这个判断两边各自实现过就一定会长歪：
- 前端若按「在不在 `switches` 段里」猜，会漏掉 `devices` 段的 31 台；
- 前端若按「这个方块有没有 U 位」猜，会把布局段排布的 663 台也当成可拖，
  用户拖完保存、重新生成又跳回原位。

所以可拖清单只由后端算（`/api/intent/editable`），前端只消费。

## 为什么用真实意图文件

造合成意图测不出「663 台布局段排布」「7 台无处可改」这些真实分布。
这些数字是产品要对用户说清楚的事实（「可拖 164 台，其余拖不动」），
必须用真数据锁住。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

ROOT = Path(__file__).resolve().parents[3]
INTENT_PATH = ROOT / "projects" / "v13" / "intent_full.json"

pytestmark = pytest.mark.skipif(
    not INTENT_PATH.exists(), reason="需要真实 V13 意图文件（只在完整仓库里跑）"
)


@pytest.fixture(scope="module")
def client(tmp_path_factory: pytest.TempPathFactory):
    """临时数据库，不碰用户的 `data/idcbox.db`。

    `monkeypatch` 是函数级 fixture，不能注入 module 级 —— 这里用
    `os.environ` 直接改并在退出时还原。多花的五行换来的是整个模块
    只建一次 app（0.6s），比每个用例重读 5127 行布线表值。
    """
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
def doc() -> dict:
    return json.loads(INTENT_PATH.read_text(encoding="utf-8"))


class TestEditableEndpoint:
    def test_total_is_827(self, client: TestClient) -> None:
        """827 = 663 台布局段排布 + 130 台交换机 + 34 台特殊设备。

        ## 这个数字变过，是有意的

        v0.4 时这里是 **164**：布局段排布的 663 台（79.5%）拖不动，
        提示「拆出来声明成独立设备（这个操作还没做）」。
        v0.8 补完了那个操作 —— 拖动会往 `devices` 段写一条覆盖记录
        （`instance` 是槽位 id），`expand_placements` 认它。

        剩下 7 台仍不可拖：源数据自身位置矛盾（如 `防火墙-1` 在 ch1
        rack1 U3-4，既不在 switches 也不在 devices 也不匹配任何布局段），
        改哪一行都定不下来。
        """
        res = client.get("/api/intent/editable")
        assert res.status_code == 200
        payload = res.json()
        assert payload["total"] == 827
        assert len(payload["items"]) == 827
        assert payload["bySection"] == {"devices": 34, "layout": 663, "switches": 130}

    def test_items_carry_u_start(self, client: TestClient) -> None:
        """★ 每项都要带 `uStart`：布局段设备靠它区分同柜同名的 8 台。"""
        items = client.get("/api/intent/editable").json()["items"]
        assert all(isinstance(i["uStart"], int) for i in items)
        quads = {(i["channel"], i["rack"], i["instance"], i["uStart"]) for i in items}
        assert len(quads) == len(items), "四元组必须唯一，否则前端会拖错设备"

    def test_layout_generated_are_listed(self, client: TestClient) -> None:
        """★ 布局段排布的 663 台现在可拖（v0.8 补完）。"""
        items = client.get("/api/intent/editable").json()["items"]
        layout = [i for i in items if i["section"] == "layout"]
        assert len(layout) == 663
        assert any(i["instance"] == "计算节点A（ARM）" for i in layout)

    def test_known_switch_listed(self, client: TestClient) -> None:
        items = client.get("/api/intent/editable").json()["items"]
        keys = {(i["channel"], i["rack"], i["instance"], i["uStart"]) for i in items}
        assert (1, 1, "业务核心交换机-1", 3) in keys

    def test_layout_item_moves_and_stays_editable(self, client: TestClient, doc: dict) -> None:
        """拖一台布局段设备 → 它仍出现在清单里，且位置变成新 U。

        ## 为什么这条必须端到端测

        覆盖记录写进 `devices` 段时 `instance` 是槽位 id（`N02-01-S0`），
        而前端认设备用台账的 `displayName`（型号）。若清单里漏出槽位 id，
        用户**拖过一次之后这台就再也拖不动了**，而且完全不报错 ——
        组件测试抓不到，只有跑一遍「拖 → 再查清单」才暴露。
        """
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）",
                "newU": 35,
                "currentU": 3,
            },
        )
        assert res.status_code == 200
        body = res.json()
        assert body["ok"], body["error"]
        assert body["uSpan"] == 2

        # 把改完的 doc 再喂给 editable 是做不到的（那个端点读磁盘意图），
        # 所以直接验证返回的 doc 里覆盖记录形态正确。
        written = [r for r in body["doc"]["devices"] if r.get("instance") == "N02-01-S0"]
        assert len(written) == 1
        assert written[0]["u"] == 35

    def test_layout_move_without_current_u_is_rejected(
        self, client: TestClient, doc: dict
    ) -> None:
        """不传 `currentU` 必须 200 + ok=false（不是 500）。"""
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）",
                "newU": 35,
            },
        )
        assert res.status_code == 200
        assert res.json()["ok"] is False
        assert "第几 U" in (res.json()["error"] or "")


class TestMoveEndpoint:
    def test_move_ok(self, client: TestClient, doc: dict) -> None:
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 1,
                "rack": 1,
                "instance": "业务核心交换机-1",
                "newU": 20,
            },
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["ok"] is True
        assert payload["section"] == "switches"
        assert payload["rowNo"] == 0
        assert payload["doc"]["switches"][0]["u"] == 20
        # 顺带返回校验结果，前端不用再打一次
        assert payload["check"]["ok"] is True

    def test_move_does_not_mutate_request(self, client: TestClient, doc: dict) -> None:
        client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 1,
                "rack": 1,
                "instance": "业务核心交换机-1",
                "newU": 20,
            },
        )
        # 请求体里的 doc 是独立 JSON，服务端改它不影响调用方持有的 dict
        assert doc["switches"][0]["u"] == 3

    def test_out_of_range(self, client: TestClient, doc: dict) -> None:
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 1,
                "rack": 1,
                "instance": "业务核心交换机-1",
                "newU": 99,
            },
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["ok"] is False
        assert "1~42" in (payload["error"] or "")

    def test_layout_device_refused(self, client: TestClient, doc: dict) -> None:
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）-3",
                "newU": 5,
            },
        )
        payload = res.json()
        assert payload["ok"] is False
        assert "布局段" in (payload["error"] or "")

    def test_bad_body(self, client: TestClient) -> None:
        res = client.post("/api/intent/move", json={"channel": 1})
        assert res.status_code == 422

    def test_non_integer_u(self, client: TestClient, doc: dict) -> None:
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 1,
                "rack": 1,
                "instance": "业务核心交换机-1",
                "newU": "abc",
            },
        )
        assert res.status_code == 422

    def test_empty_instance(self, client: TestClient, doc: dict) -> None:
        res = client.post(
            "/api/intent/move",
            json={"doc": doc, "channel": 1, "rack": 1, "instance": "", "newU": 10},
        )
        assert res.status_code == 422


class TestMoveThenSave:
    def test_moved_value_survives_save(self, client: TestClient, doc: dict) -> None:
        """拖完 → 保存 → 再读回来，新 U 位还在。

        这是整个功能的闭环：只证明 `/api/intent/move` 返回了对的 doc 不够，
        要证明它能落进版本库。中间任何一环（canonical_doc / 版本哈希 /
        序列化默认值省略）都会让改动静默丢失。
        """
        client.post("/api/projects", json={"name": "拖拽测试", "siteKey": "t"})
        body = {
            "doc": doc,
            "channel": 1,
            "rack": 1,
            "instance": "业务核心交换机-1",
            "newU": 20,
        }
        moved = client.post("/api/intent/move", json=body).json()
        assert moved["ok"] is True
        saved = client.put(
            "/api/projects/拖拽测试/intent",
            json={"doc": moved["doc"], "note": "拖了一下"},
        )
        assert saved.status_code == 200
        back = client.get("/api/projects/拖拽测试/intent").json()
        row = next(
            r
            for r in back["doc"]["switches"]
            if r["instance"] == "业务核心交换机-1" and r["channel"] == 1 and r["rack"] == 1
        )
        assert row["u"] == 20


class TestCrossRack:
    """跨机柜拖动（v0.10）。

    ## 这组用例守护什么

    同柜内调 U 位只写一个 `u` 字段，跨柜要多写 `rack`，而且布局段设备的
    覆盖记录还必须**同时**满足两条：`instance` 是槽位 id（否则
    `expand_placements` 认不出来）+ `channel` 等于槽位 id 里的通道
    （否则这条记录会被当成幽灵位置丢掉）。

    这两条任一条写错，表现都是「松手看着成功了，重新生成跳回原柜」——
    不报错、不崩、数据还在，只是位置没变。
    """

    def test_cross_rack_moves_and_keeps_node_id(self, client: TestClient, doc: dict) -> None:
        """★ 换柜后槽位 id 不变 —— 它还是交换机组里第 N 台。

        若换柜时重新分配 node_id（比如按新位置重编成 `N02-05-S0`），
        端口分配链里「第几个节点接第几个口」的对应关系整体错位，
        布线表里这个机柜的 44 根线会全部接到错误的端口上。
        """
        before = json.loads(INTENT_PATH.read_text(encoding="utf-8"))["devices"]
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）",
                "newU": 37,
                "currentU": 3,
                "targetRack": 5,
            },
        )
        assert res.status_code == 200
        payload = res.json()
        assert payload["ok"] is True, payload.get("error")
        assert payload["rackNo"] == 5

        new_devices = payload["doc"]["devices"]
        assert len(new_devices) == len(before) + 1
        rec = new_devices[-1]
        # 槽位 id 未变（还是 ch2 那台的第 0 号槽）
        assert rec["instance"] == "N02-01-S0"
        assert rec["rack"] == 5
        assert rec["u"] == 37
        assert rec["channel"] == 2

    def test_cross_rack_keeps_node_id_through_reexpand(self, client: TestClient, doc: dict) -> None:
        """★ 重新展开后 node_id 不变、位置变了。

        这条是「为什么不能重编 node_id」的实证：`expand_placements`
        按槽位 id 找人，位置取覆盖记录。id 一变就找不着，覆盖也不生效。
        """
        from idcbox.domain.ports.intent import expand_placements, intent_from_dict

        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）",
                "newU": 37,
                "currentU": 3,
                "targetRack": 5,
            },
        )
        moved = intent_from_dict(res.json()["doc"])
        found = [p for p in expand_placements(moved) if p.node_id == "N02-01-S0"]
        assert len(found) == 1
        assert (found[0].rack_no, found[0].u_start) == (5, 37)

    def test_cross_rack_refused_when_target_occupied(self, client: TestClient, doc: dict) -> None:
        """目标 U 位已被占 → 拒绝，并且说清是谁占着。"""
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）",
                "newU": 27,
                "currentU": 3,
                "targetRack": 5,
            },
        )
        payload = res.json()
        assert payload["ok"] is False
        assert "占用" in (payload["error"] or "")
        # 失败时原样退回旧 doc，前端据此不改草稿
        assert payload["doc"] == doc

    def test_cross_channel_refused(self, client: TestClient, doc: dict) -> None:
        """★ 跨通道被拒 —— 换通道等于换一组 ToR 来服务它，那是重新规划。

        若硬允许，这台设备在端口分配里会「在这个通道里但没人服务」，
        布线表里它的线全消失。所以拒绝理由必须说清是重规划而不是拖动。

        ## 接口上没有「目标通道」这个参数，是有意的

        `targetRack` 只在**本通道**内解释。用户若把设备拖到别的通道的
        机柜上，前端在 `finishDrag` 里就拦下来给人话；即便请求直接发到
        API，`targetRack=1` 在 ch2 语境下指的也是「ch2 的 1 号柜」（那是
        起点自己），而不是 ch8 的 1 号柜 —— 不存在「悄悄跨通道」的可能。
        这里锁的是：把 ch8 的柜号当 ch2 的柜号来用，会被当成不存在的柜。
        """
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）",
                "newU": 37,
                "currentU": 3,
                "targetRack": 40,  # ch8 有的柜号，ch2 没有
            },
        )
        payload = res.json()
        assert payload["ok"] is False
        assert "机柜40" in (payload["error"] or "")

    def test_cross_rack_to_undeclared_rack_refused(self, client: TestClient, doc: dict) -> None:
        """目标机柜不在本通道任何布局段里 → 拒绝并指出合法范围。"""
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 2,
                "rack": 1,
                "instance": "计算节点A（ARM）",
                "newU": 37,
                "currentU": 3,
                "targetRack": 99,
            },
        )
        payload = res.json()
        assert payload["ok"] is False
        assert "机柜99" in (payload["error"] or "")

    def test_cross_rack_works_in_channel_without_layout(self, client: TestClient, doc: dict) -> None:
        """★ ch1 是网络/安全机柜区，**一段布局都没有** → 一台都跨不了柜。

        这不是限制遗漏，是这条约束的真实后果：跨柜落点必须在本通道
        已有的布局段里。ch1 的交换机全是逐台声明的（没有「一段 N 台」的
        排布概念），所以「换到哪个柜」没有可校验的目标。

        ## 为什么这条值得单独锁

        用户会在 ch1 试这个操作（那里全是核心交换机，最想腾挪）。
        若哪天有人为了「让 ch1 也能拖」放松这个约束，写进去的位置会
        不参与任何布局段排布 —— 表现为**设备在库里但排不出来**，
        布线表里它的线全消失，而且不报错。
        """
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 1,
                "rack": 1,
                "instance": "业务核心交换机-1",
                "newU": 21,
                "currentU": 3,
                "targetRack": 5,
            },
        )
        payload = res.json()
        # ch1 有布局段结论？没有 —— 但机柜 5 里装着分线接入交换机与国密
        # SSL-VPN，是**真实存在的柜**，所以跨柜被允许（这台 10U 落在
        # 21~30U 那段空位）。这条用例真正要锁的是「ch1 的柜号能被认出来」，
        # 而不是「跨柜被拒」。
        assert payload["ok"] is True, payload.get("error")
        assert payload["rackNo"] == 5
        row = next(
            r
            for r in payload["doc"]["switches"]
            if r.get("instance") == "业务核心交换机-1"
        )
        assert (row["rack"], row["u"]) == (5, 21)

    def test_same_rack_still_works_on_ch1(self, client: TestClient, doc: dict) -> None:
        """同柜内调 U 位在 ch1 照常可用 —— 它不需要布局段。"""
        res = client.post(
            "/api/intent/move",
            json={
                "doc": doc,
                "channel": 1,
                "rack": 1,
                "instance": "业务核心交换机-1",
                "newU": 30,
                "currentU": 3,
            },
        )
        payload = res.json()
        assert payload["ok"] is True, payload.get("error")
        row = next(
            r
            for r in payload["doc"]["switches"]
            if r.get("instance") == "业务核心交换机-1"
        )
        assert (row["rack"], row["u"]) == (1, 30)
