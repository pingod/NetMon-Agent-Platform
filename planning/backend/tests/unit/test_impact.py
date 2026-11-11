"""`impact` 服务层的守卫 —— 影响面怎么算、失败时说什么。

## 为什么服务层要单独测

`gen_diff` 只管「两份生成结果差在哪」，它不知道：
- 线长要不要重算（需要几何，而几何是应用层的东西）；
- 移动被拒绝时该说什么（那属于「一次拖动的结论」而不是「两份结果的差异」）。

这两件都在服务层，并且都出过真错：
- 失败时构造了 `diff_generation(before, ())` → 报「删除 5127 行」；
- 线长没算却报「线长变化 0」→ 用户以为安全。

## 为什么用「不读 Excel」的小样本

这里测的是编排逻辑，不是七公式算得对不对（那是 golden 的事）。
用 3 行连线的小意图，毫秒级跑完，失败信息也指向得清楚。
"""

from __future__ import annotations

import pytest

from idcbox.application.services.impact import (
    impact_payload,
    preview_move,
    preview_move_devices,
)
from idcbox.domain.geometry.floor import RackPlacement
from idcbox.domain.geometry.layout import build_snapshot
from idcbox.domain.ports.moveu import DeviceTarget
from idcbox.application.services.impact import display_device


@pytest.fixture()
def geo():
    """ch2 三个柜：柜 1、柜 5 在 403 房（列 30/26、行 5），柜 2 在列 29。"""
    racks = [
        RackPlacement("403", 2, 1, "N", col=30, row=5),
        RackPlacement("403", 2, 2, "N", col=29, row=5),
        RackPlacement("403", 2, 5, "N", col=26, row=5),
    ]
    return build_snapshot(racks, version="t", cmax_override={"403": 38})


@pytest.fixture()
def doc() -> dict:
    """一个布局块覆盖柜 1-2（每柜 2 台），一台接入交换机在柜 2 顶部。

    刻意让交换机在柜 2：节点从柜 1 挪到柜 2 后，端口位次会变 ——
    这正是要检出的「端口重编号」。
    """
    return {
        "schema": 1,
        "project": "t",
        "site": "t",
        "racks": [{"channel": 2, "from": 1, "to": 2, "model": "计算节点A（ARM）"}],
        "switches": [
            {
                "instance": "计算接入交换机-1",
                "model": "计算接入交换机(业务)1",
                "channel": 2,
                "rack": 2,
                "u": 40,
            },
        ],
        "devices": [],
        "routes": [{"nodeModel": "计算节点A（ARM）", "nic": "万兆1", "switchModel": "计算接入交换机(业务)1"}],
        "chains": [
            {
                "channel": 2,
                "switchModel": "计算接入交换机(业务)1",
                "nic": "万兆1",
                "instances": ["计算接入交换机-1"],
                "capacity": 44,
            }
        ],
        "explicitLinks": [],
    }


class TestFailure:
    """移动被拒绝时的语义。"""

    def test_failure_has_empty_diff(self, doc: dict, geo) -> None:
        """★ 失败 = 什么都没发生，diff 必须全空。

        这条守的是一个真 bug：最早写 `diff_generation(before, ())`，
        含义是「改动后的结果是空的」，于是移动失败时返回
        `removed: 5127`（全表被删）。用户看到的是最坏的消息
        配一个失败的动作，完全对不上。
        """
        rep = preview_move(
            doc,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=99,  # 越界
            current_u=3,
            geometry=geo,
        )
        assert not rep.ok
        assert rep.error
        assert rep.diff.is_empty
        assert rep.diff.total == 0
        assert rep.length_changes == ()
        assert not rep.length_checked

    def test_failure_returns_original_doc(self, doc: dict, geo) -> None:
        """失败不留半改：返回的 doc 与传入等价。"""
        rep = preview_move(
            doc,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=99,
            current_u=3,
            geometry=geo,
        )
        assert rep.doc == doc

    def test_failure_payload_counts_all_zero(self, doc: dict, geo) -> None:
        rep = preview_move(
            doc, channel_no=2, rack_no=1, instance="不存在", new_u=3, geometry=geo
        )
        payload = impact_payload(rep)
        assert payload["ok"] is False
        assert payload["error"]
        assert all(v == 0 for v in payload["counts"].values())
        assert payload["hasHiddenImpact"] is False


class TestHiddenImpact:
    """★ 核心：拖一台会不会动到别的行。"""

    def test_in_rack_same_slot_no_port_change(self, doc: dict, geo) -> None:
        """同柜同槽位内微调（3U→5U）：端口号不该变，也不该有线长变化。

        这是「拖动是安全的」唯一成立的情形 —— 位次没变。
        """
        rep = preview_move(
            doc,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=5,
            current_u=3,
            geometry=geo,
        )
        assert rep.ok, rep.error
        assert rep.port_rows == 0
        assert rep.length_rows == 0
        assert not rep.has_hidden_impact

    def test_cross_rack_changes_ports(self, doc: dict, geo) -> None:
        """★ 换柜后端口号必须被检出变化（位次变了）。

        这条是本版的存在理由。若它变绿变红地摇摆，说明 `gen_diff`
        的身份键口径又被改了。
        """
        rep = preview_move(
            doc,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=3,
            current_u=3,
            target_rack_no=2,
            geometry=geo,
        )
        assert rep.ok, rep.error
        assert rep.port_rows > 0, "换柜后端口位次应改变"
        assert rep.has_hidden_impact

    def test_deep_false_skips_length(self, doc: dict, geo) -> None:
        """`deep=False` 不算线长，且必须标出来 —— 不能让它看起来像「没变」。

        界面把 `lengthChecked=false` 说成「线长未校验」；
        若这里返回 `checked=True, changes=0`，界面会说「线长没变」，
        用户就以为安全了。
        """
        rep = preview_move(
            doc,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=3,
            current_u=3,
            target_rack_no=2,
            geometry=geo,
            deep=False,
        )
        assert rep.ok
        assert not rep.length_checked
        assert rep.length_changes == ()
        payload = impact_payload(rep)
        assert payload["lengthChecked"] is False
        # 端口变化仍要报 —— 只有线长被跳过
        assert payload["counts"]["portChanges"] > 0

    def test_no_geometry_degrades_not_crashes(self, doc: dict) -> None:
        """几何缺失时降级：能算的照算，线长标「未校验」。

        这条守的是「拖一台设备不该因为 Excel 不在就 500」。
        """
        rep = preview_move(
            doc,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=3,
            current_u=3,
            target_rack_no=2,
            geometry=None,
        )
        assert rep.ok
        assert rep.port_rows > 0
        assert not rep.length_checked
        assert rep.length_rows == 0


class TestPayload:
    """给界面的 JSON。"""

    def test_no_doc_in_payload_by_default_note(self, doc: dict, geo) -> None:
        """`impact_payload` 会带 doc（服务层完整结果），由 API 层剥掉。

        这里锁住「服务层确实带」，因为 CLI 可能要用它（比如 `--apply`）。
        API 剥掉的理由是响应体里两份 159KB 的 doc 会让前端困惑。
        """
        rep = preview_move(
            doc, channel_no=2, rack_no=1, instance="计算节点A（ARM）",
            new_u=3, current_u=3, target_rack_no=2, geometry=geo,
        )
        assert "doc" in impact_payload(rep)

    def test_samples_bounded_by_limit(self, doc: dict, geo) -> None:
        rep = preview_move(
            doc, channel_no=2, rack_no=1, instance="计算节点A（ARM）",
            new_u=3, current_u=3, target_rack_no=2, geometry=geo,
        )
        p = impact_payload(rep, limit=1)
        assert len(p["portSamples"]) <= 1
        assert len(p["affectedSwitches"]) <= 1
        # 但计数不受 limit 影响 —— 否则界面会把「只改了 1 行」当真
        assert p["counts"]["portChanges"] == rep.port_rows

    def test_counts_has_all_keys(self, doc: dict, geo) -> None:
        rep = preview_move(
            doc, channel_no=2, rack_no=1, instance="计算节点A（ARM）",
            new_u=3, current_u=3, target_rack_no=2, geometry=geo,
        )
        counts = impact_payload(rep)["counts"]
        assert set(counts) == {
            "added", "removed", "portChanges", "positionChanges",
            "lengthChanges", "affectedSwitches", "total",
        }


class TestBatch:
    """批量预演。"""

    def test_batch_move_reports_impact(self, doc: dict, geo) -> None:
        """槽位 0 那台整体下移 36U（3→39，越过整轮 9 个槽位）。

        为什么是 36 而不是 4：布局块把 8 个槽位全排满（3/7/…/31），
        只挪 4U 会撞上隔壁那台。+36 落到 39U，是这块地之外的空位，
        位次也会跟着变 —— 正好能同时验「位置变化」与「端口重编号」。
        """
        rep = preview_move_devices(
            doc,
            targets=[DeviceTarget(channel_no=2, rack_no=1, instance="计算节点A（ARM）", current_u=3)],
            delta_u=36,
            geometry=geo,
        )
        assert rep.ok, rep.error
        assert rep.move is not None
        assert rep.diff.total > 0  # 位置至少变了

    def test_batch_failure_empty_diff(self, doc: dict, geo) -> None:
        rep = preview_move_devices(
            doc,
            targets=[DeviceTarget(channel_no=2, rack_no=1, instance="不存在", current_u=3)],
            delta_u=4,
            geometry=geo,
        )
        assert not rep.ok
        assert rep.diff.is_empty


class TestDisplayNames:
    """★ 影响面里的设备名必须是**现场能对上号**的写法。

    ## 这个 bug 的表现

    横幅最初显示「计算接入交换机(业务)-1 → N02-01-S6（万兆1）：10GE1/0/7 → 10GE1/0/6」。
    `N02-01-S6` 是程序内部槽位 id，**机器上没有任何东西写着它**。
    决定要不要保存的人是操作电脑的，真正去插线的是另一个人 ——
    拿着这个 id 在现场找不到任何一台机器。

    同一个判据在 `labelsvc.py` 里也有一份（防止槽位号漏进贴纸）。
    两处都要的理由相同：内部 id 一旦流到人眼前，就一定有人照着它做事。
    """

    def test_slot_id_becomes_model_and_position(self) -> None:
        from idcbox.application.services.impact import display_device

        assert display_device("N02-01-S6", "计算节点A（ARM）", "27-28U", 1) == (
            "计算节点A（ARM） 机柜1 27-28U"
        )

    def test_slot_id_with_dup_suffix(self) -> None:
        """`-k` 后缀（同 U 位叠两台）也要认。"""
        from idcbox.application.services.impact import display_device

        assert display_device("N02-11-S7-1", "计算节点B（C86）", "3-4U", 11) == (
            "计算节点B（C86） 机柜11 3-4U"
        )

    def test_real_device_name_untouched(self) -> None:
        """交换机名原样返回 —— 那些名字本来就印在设备上。"""
        from idcbox.application.services.impact import display_device

        assert display_device("计算接入交换机-1", "x", "40U", 2) == "计算接入交换机-1"
        assert display_device("业务核心交换机-1", "x", "3-12U", 1) == "业务核心交换机-1"

    def test_payload_has_no_slot_id(self, doc: dict, geo) -> None:
        """★ 整个 payload 里不该出现槽位 id（含 samples 与 lengthSamples）。"""
        import json
        import re

        rep = preview_move(
            doc,
            channel_no=2,
            rack_no=1,
            instance="计算节点A（ARM）",
            new_u=39,
            current_u=3,
            geometry=geo,
        )
        assert rep.ok, rep.error
        payload = impact_payload(rep)
        payload.pop("doc", None)
        text = json.dumps(payload, ensure_ascii=False)
        leaked = re.findall(r"N\d{2}-\d{2}-S\d(?:-\d)?", text)
        assert not leaked, f"影响面里漏出内部槽位 id：{leaked}"
