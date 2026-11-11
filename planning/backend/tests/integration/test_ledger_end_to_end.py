"""设备台账 / 端口台账的 CLI 与 API 端到端（真实 V13 意图）。

## 这组用例守护的是什么

`ledger.py` 的聚合逻辑已被 `tests/unit/test_ledger.py` 覆盖（34 例），
但**从意图一路走到台账**这条路没被证明过。三个环节都可能悄悄丢数据：

1. `generate_links` 生成的连线数量（5127）
2. `ledger_from_cabling` 把 U 位原文（`3-12U`）拆成（起始, 跨度）
3. `portledger.py` 聚合时的分母口径（只算有模板的设备）

任何一环出错，台账都会给出一个**看起来合理的数字**——用户拿着它去
判断"这台交换机还能接几台服务器"，接不下就是现场返工。

## 与单测的分工

单测用合成数据，能断言精确值；这里用真实意图，断言的是
**这些数字与原始交付物一致**（830 台设备、10252 个已用端口）。
数字对不上时说明上游变了，而不是"这里算错了"。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

INTENT = Path(__file__).resolve().parents[3] / "projects" / "v13" / "intent_full.json"

pytestmark = pytest.mark.skipif(
    not INTENT.exists(), reason="需要真实 V13 意图文件（只在完整仓库里跑）"
)

BACKEND = Path(__file__).resolve().parents[3] / "backend"


@pytest.fixture(scope="module")
def ledger():
    """真实意图 → 生成连线 → 端口台账。"""
    import sys

    if str(BACKEND) not in sys.path:
        sys.path.insert(0, str(BACKEND))
    from idcbox.domain.ports.generate import generate_links
    from idcbox.domain.ports.intent import intent_from_dict
    from idcbox.domain.ports.ledger import ledger_from_cabling

    intent = intent_from_dict(json.loads(INTENT.read_text(encoding="utf-8")))
    cabling = generate_links(intent)
    assert len(cabling) == 5127, f"生成行数变了：{len(cabling)}"
    return ledger_from_cabling(cabling)


@pytest.fixture(scope="module")
def payload(ledger):
    """台账 payload（API 返回的形状）。"""
    from idcbox.application.services.portledger import ledger_payload

    return ledger_payload(ledger, include_devices=True, limit=len(ledger.devices))


class TestRealNumbers:
    """真实意图上的台账数字必须与交付物对得上。"""

    def test_device_count_is_830(self, ledger) -> None:
        """830 = 真实唯一设备数。

        ## 为什么不是 954

        954 是「本端出现 + 对端出现」各算一次的口径 —— 一台交换机
        既作为本端（自己那 44 行）也作为对端（别人引用它）出现，
        按出现次数算会重复计一遍。台账必须按设备身份去重。
        """
        assert ledger.device_count == 830

    def test_used_ports(self, ledger) -> None:
        """10252 个已用端口（含两端各记一次）。

        端口条目 12820 里，两台设备之间一条线会在两端各留一条记录，
        所以已用端口数 ≈ 连线数 × 2（5127 × 2 = 10254）。
        差 2 是 `分线接入交换机` 的 MPO 一分三（物理上是一个口接三根线）。
        """
        assert ledger.used_total == 10252
        assert len(ledger.entries) == 12820

    def test_capacity_excludes_no_spec(self, ledger) -> None:
        """分母只算有模板的设备。

        830 台里 1 台（`带外交换机-2`）型号为空，标称端口数未知。
        把它算进分母（哪怕算 0）会让整体利用率凭空变化。
        """
        assert len(ledger.no_spec_devices) == 1
        assert "带外交换机-2" in ledger.no_spec_devices[0].device_label

    def test_no_over_used_ports(self, ledger) -> None:
        """没有任何设备用超端口数。

        这条在 0 未知端口之后才有意义 —— 越界端口会被收进
        `unknown_ports` 而不是抬高 used，所以结构上恒为空。
        保留它的价值：`bay_key` 归并逻辑被改坏时这里会立刻变红。
        """
        assert ledger.over_used == ()
        assert ledger.with_unknown_ports == ()


class TestCategoryAggregates:
    """按类别的汇总必须覆盖全部设备（一台不落）。"""

    def test_all_devices_are_categorised(self, ledger) -> None:
        total = sum(len(group) for group in ledger.by_rack().values())
        assert total == ledger.device_count

    def test_core_switches_are_partially_used(self, ledger) -> None:
        """业务核心/存储核心各 2 台，各有 156 口但只用了一半左右。

        核心交换机是上联口密集的设备，实际接的接入交换机数量决定用量。
        若这里变成 100%，说明端口模板把上联面板数量算少了。
        """
        cores = [d for d in ledger.devices if d.category == "core"]
        assert len(cores) == 6
        used = sum(d.used_count for d in cores)
        total = sum(d.total_count for d in cores)
        assert 0.4 < used / total < 0.7


class TestApiPayload:
    """`ledger_payload` 的形状，前端靠它渲染。"""

    def test_summary_shape(self, payload) -> None:
        summary = payload["summary"]
        assert summary["devices"] == 830
        assert summary["used"] == 10252
        assert summary["capacity"] > summary["used"]
        assert 0.7 < summary["ratio"] < 0.9

    def test_by_model_sorted_by_capacity(self, payload) -> None:
        """按**容量**降序，不是按已用降序。

        ## 为什么排序键是容量

        这张表的用途是「这个型号这批设备还能再插多少台」。
        按容量降序，前几行就是余量最大的型号（计算节点的 8 口 × 550 台）；
        按已用降序会把「已经插满的服务器型号」排在最前面 ——
        那几行的空闲永远是 0，看一眼没有任何信息。
        """
        capacity = [row["capacity"] for row in payload["byModel"]]
        assert capacity == sorted(capacity, reverse=True)

    def test_attention_issues_are_actionable(self, payload) -> None:
        """每一条「需要注意」都要能定位到设备。"""
        for issue in payload["attention"]:
            assert issue["kind"] in ("NO_SPEC", "OVER_USED", "UNKNOWN_PORT")
            assert issue["device"] or issue["detail"]

    def test_device_rows_omit_port_lists(self, payload) -> None:
        """批量设备行**不带**端口明细，只有用量汇总。

        ## 为什么这是对的而不是偷懒

        830 台 × 平均 15 个口 = 1.2 万条端口记录。全塞进列表接口，
        响应会到几 MB，而用户看列表时只关心「哪台快满了」。
        要看某个口插了什么，走 `?device=<名字>`（单台，带全量端口与空闲口）。

        这条用例把「没带端口」这件事固定成**契约**，
        以后有人图省事往列表里塞 ports，这里会提醒他先想清楚体积。
        """
        target = next(d for d in payload["devices"] if d["label"].startswith("业务核心交换机-1"))
        assert target["used"] == 89
        assert target["total"] == 156
        assert target["free"] == 67
        assert "portList" not in target

    def test_single_device_has_ports_and_free_bays(self, payload, ledger) -> None:
        """单台详情必须同时给「用了哪些口」和「还剩哪些口」。

        只给用量数字，现场还是要翻表才知道下一个该插哪个口。
        """
        from idcbox.application.services.portledger import device_payload

        core = next(d for d in ledger.devices if d.device_label.startswith("业务核心交换机-1"))
        detail = device_payload(core, ports=True)
        assert detail["used"] == 89
        assert detail["total"] == 156
        assert len(detail["portList"]) == 89
        assert len(detail["freeBays"]) == 67
        assert detail["portList"][0]["port"]
        assert detail["portList"][0]["role"]


class TestPortUtilizationWithSpecs:
    """`/api/ports/utilization` 的分母来自端口模板（v0.5 新增）。"""

    @pytest.fixture(scope="module")
    def util(self) -> dict:
        from fastapi.testclient import TestClient

        from idcbox.api.main import create_app

        return TestClient(create_app()).get("/api/ports/utilization").json()

    def test_summary_has_real_denominator(self, util) -> None:
        assert util["summary"]["modelsWithSpec"] == util["summary"]["models"]
        assert util["summary"]["totalPorts"] > util["summary"]["usedPorts"]
        assert 0.7 < util["summary"]["ratio"] < 0.85

    def test_every_model_has_spec(self, util) -> None:
        """27 个型号全部匹配到模板。

        ## 为什么这条值得单独断言

        修别名之前 `广域网负载均衡` 是 `hasSpec: false` —— 它和
        `广域网链路负载均衡` 是同一台设备的两种写法。漏这个别名，
        台账上直接少 2 台设备的容量，而整体统计看不出异常（只差 34 口）。
        """
        assert util["noSpecModels"] == []
        for row in util["byModel"]:
            assert row["hasSpec"] is True, row["model"]
            assert row["totalPorts"] is not None

    def test_unknown_ports_are_explained(self, util) -> None:
        """唯一一条「未知端口」的根因是型号记错，同时也在 modelConflicts 里。

        这条用例把两个字段的因果关系固定下来：看到未知端口时，
        先查 `modelConflicts`，若命中说明是源表型号问题而不是模板问题。
        """
        assert len(util["unknownPortDevices"]) == 1
        culprits = {line.split(":")[0] for line in util["unknownPortDevices"]}
        assert culprits <= set(util["modelConflicts"])


class TestCli:
    """CLI 三个入口都要能跑通并打印关键数字。"""

    def test_ledger_list(self, capsys) -> None:
        import sys

        if str(BACKEND) not in sys.path:
            sys.path.insert(0, str(BACKEND))
        from idcbox.cli import main

        code = main(["ledger", "--intent", str(INTENT), "--list", "--limit", "5"])
        out = capsys.readouterr().out
        assert code == 0
        assert "830 台" in out
        assert "已用 10252" in out

    def test_ledger_single_device(self, capsys) -> None:
        import sys

        if str(BACKEND) not in sys.path:
            sys.path.insert(0, str(BACKEND))
        from idcbox.cli import main

        code = main(["ledger", "--intent", str(INTENT), "--device", "存储核心交换机-1"])
        out = capsys.readouterr().out
        assert code == 0
        assert "存储核心交换机-1" in out
        assert "空闲口" in out

    def test_ledger_unknown_device(self, capsys) -> None:
        """找不到设备要报错退出，不能静默成功。"""
        import sys

        if str(BACKEND) not in sys.path:
            sys.path.insert(0, str(BACKEND))
        from idcbox.cli import EXIT_USAGE, main

        code = main(["ledger", "--intent", str(INTENT), "--device", "不存在的设备"])
        out = capsys.readouterr().out
        assert code == EXIT_USAGE
        assert "找不到设备" in out
