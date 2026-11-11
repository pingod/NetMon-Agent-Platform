"""型号库的可替换性 —— 「现场来一个新型号不用改代码」。

## 这个模块守护的东西

v0.5/v0.6 的端口模板是硬编码的 26 个型号。这一层把它变成数据，
三个来源按优先级叠加（内置 < 项目 JSON < Excel）。守护四条不变量：

1. **内置表永远是底座**：项目库只覆盖它声明的型号，不能把另外 25 个型号
   弄丢。丢了的表现是台账上大批设备显示「端口未知」而使用者不知道自己做错了什么。
2. **来回一趟必须无损**：导出 → 改 → 导回，端口身份一个不能变。
3. **面板号必须显式带上**：`业务核心交换机` 有 3 块同规格 100GE 板，
   导出时不写面板号，再导入会合并成一块（156 口 → 108 口）。
4. **坏数据只提示不阻断**：型号库错一条不该让整个项目打不开。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from idcbox.domain.ports.modellib import (
    ModelLibrary,
    diff_libraries,
    free_bays_of,
    library_to_json,
    load_model_library,
    merge_entries,
    parse_library_json,
)
from idcbox.domain.ports.portspec import PortBay, PortSpec

CORE = "业务核心交换机"


def _base() -> ModelLibrary:
    return load_model_library()


def _custom(name: str = "自研汇聚交换机X") -> dict:
    return {
        "name": name,
        "category": "switch",
        "groups": [
            {
                "group": "10GE",
                "panel": 1,
                "indices": list(range(1, 25)),
                "nominalMbps": 10000,
                "speeds": [10000],
                "role": "downlink",
            },
            {
                "group": "100GE",
                "panel": 1,
                "indices": [1, 2, 3, 4],
                "nominalMbps": 100000,
                "speeds": [100000],
                "role": "uplink",
            },
        ],
    }


class TestBuiltinBase:
    def test_builtin_has_26_models(self) -> None:
        lib = _base()
        assert len(lib.specs) == 26

    def test_builtin_total_ports_is_855(self) -> None:
        """26 个型号共 855 个口 —— 这是 v0.5 逐条核过 `首期设备清单` 的数字。"""
        assert _base().total_ports() == 855

    def test_core_switch_has_three_100ge_panels(self) -> None:
        """核心交换机 156 口 = 3 × 36 × 100GE + 48 × 25GE。

        面板号只存在于 `slots()` 里（`bay.panel` 内置表恒为 0，
        表示「按出现顺序自动编号」）。三块 100GE 板靠的就是这个顺序。
        """
        spec = _base().spec_of(CORE)
        assert spec is not None
        assert spec.total_ports == 156
        assert len({panel for group, panel, _ in spec.slots() if group == "100GE"}) == 3
        assert spec.bay_key("100GE3/0/36") == ("100GE", 3, 36)

    def test_alias_resolves(self) -> None:
        """`广域网负载均衡` 与 `广域网链路负载均衡` 是同一台设备的两种写法。"""
        lib = _base()
        assert lib.spec_of("广域网负载均衡") is not None
        assert lib.spec_of("广域网负载均衡").model == lib.spec_of("广域网链路负载均衡").model

    def test_unknown_model_returns_none(self) -> None:
        assert _base().spec_of("根本不存在的型号") is None
        assert _base().spec_of("") is None
        assert _base().spec_of(None) is None


class TestOverlay:
    def test_json_adds_model_and_keeps_builtin(self) -> None:
        """叠加不是替换：新增 1 个型号后内置 26 个还在。"""
        lib = load_model_library(
            excel_entries=parse_library_json({"models": [_custom()]})[0],
            excel_detail="test",
        )
        assert len(lib.specs) == 27
        assert lib.spec_of("自研汇聚交换机X").total_ports == 28
        assert lib.spec_of(CORE).total_ports == 156

    def test_override_changes_port_count(self) -> None:
        """同名型号覆盖，不再是内置的 156 口。"""
        entry = {
            "name": CORE,
            "category": "core",
            "groups": [
                {
                    "group": "100GE",
                    "indices": [1, 2],
                    "nominalMbps": 100000,
                    "speeds": [100000],
                    "role": "uplink",
                }
            ],
        }
        lib = load_model_library(excel_entries=parse_library_json({"models": [entry]})[0])
        assert lib.spec_of(CORE).total_ports == 2
        assert len(lib.specs) == 26

    def test_remove_is_explicit_not_side_effect(self) -> None:
        """删型号必须显式写 remove，别人不能因为「库里没有」就消失。"""
        lib = load_model_library(
            excel_entries=parse_library_json({"models": [{"name": CORE, "remove": True}]})[0]
        )
        assert lib.spec_of(CORE) is None
        assert len(lib.specs) == 25

    def test_source_is_tracked(self) -> None:
        """「这 48 口谁说的」要能答。"""
        lib = load_model_library(
            excel_entries=parse_library_json({"models": [_custom()]})[0],
            excel_detail="现场.xlsx",
        )
        assert lib.source_of(CORE).kind == "builtin"
        src = lib.source_of("自研汇聚交换机X")
        assert src.kind == "excel"
        assert "现场.xlsx" in src.describe()


class TestRoundTrip:
    def test_json_roundtrip_is_lossless(self) -> None:
        lib = _base()
        entries, warns = parse_library_json(library_to_json(lib))
        assert warns == ()
        back = merge_entries(lib, entries, kind="json")
        assert diff_libraries(lib, back).is_empty
        assert back.total_ports() == lib.total_ports()

    def test_roundtrip_preserves_panel_split(self) -> None:
        """核心交换机的 3 块 100GE 板往返后仍是 3 块、仍是 156 口。

        导出时不写面板号，三块同规格板会被合并成一块 —— 156 → 108，
        数字看着像个正常数字，没人会发现少了 48 个口。
        """
        lib = _base()
        entries, _ = parse_library_json(library_to_json(lib))
        back = merge_entries(lib, entries, kind="json")
        assert back.spec_of(CORE).total_ports == 156
        assert back.spec_of(CORE).slots() == lib.spec_of(CORE).slots()

    def test_shape_ignores_panel_zero_vs_one(self) -> None:
        """内置表写 `panel=0`（自动编号），库里写算出来的 `panel=1`。

        两者端口身份等价，不能报成「改动」—— 否则用户导出一趟再导回来
        看到「改动 26」，只会以为导出坏了。

        反证在同一处：若比较口径取 `bays`（含 `panel` 那个 0/1 差异），
        这一趟往返会报出 26 个「改动」，而它们其实一个口都没变。
        """
        from idcbox.domain.ports.modellib import _shape

        lib = _base()
        back = merge_entries(lib, parse_library_json(library_to_json(lib))[0], kind="json")
        assert lib.specs[CORE].bays[0].panel == 0, "内置表的面板号是 0（自动编号）"
        assert back.specs[CORE].bays[0].panel == 1, "导出的库写的是算出来的面板号"
        for spec in lib.all():
            other = back.spec_of(spec.model)
            assert _shape(spec) == _shape(other), spec.model
        assert diff_libraries(lib, back).changed == ()

    def test_diff_reports_real_change(self) -> None:
        lib = _base()
        entry = {
            "name": CORE,
            "category": "core",
            "groups": [
                {
                    "group": "100GE",
                    "indices": [1],
                    "nominalMbps": 100000,
                    "speeds": [100000],
                }
            ],
        }
        changed = merge_entries(lib, parse_library_json({"models": [entry]})[0], kind="json")
        assert diff_libraries(lib, changed).changed == (CORE,)


class TestBadInput:
    def test_missing_nominal_is_reported_not_fatal(self) -> None:
        entries, warns = parse_library_json(
            {"models": [{"name": "坏型号", "groups": [{"group": "GE", "indices": [1, 2]}]}]}
        )
        assert len(entries) == 1
        assert any("nominalMbps" in w for w in warns)

    def test_unknown_category_falls_back(self) -> None:
        lib = load_model_library(
            excel_entries=parse_library_json(
                {"models": [{**_custom(), "category": "外星设备"}]}
            )[0]
        )
        assert lib.spec_of("自研汇聚交换机X").category == "other"

    def test_unknown_role_falls_back(self) -> None:
        spec = {
            "name": "某设备",
            "groups": [
                {
                    "group": "GE",
                    "indices": [1],
                    "nominalMbps": 1000,
                    "speeds": [1000],
                    "role": "外星球口",
                }
            ],
        }
        lib = load_model_library(excel_entries=parse_library_json({"models": [spec]})[0])
        assert lib.spec_of("某设备").bays[0].role == "unknown"

    def test_broken_json_file_does_not_raise(self, tmp_path: Path) -> None:
        p = tmp_path / "models.json"
        p.write_text("{ 这不是 json", encoding="utf-8")
        lib = load_model_library(json_path=p)
        assert len(lib.specs) == 26
        assert lib.warnings

    def test_missing_file_does_not_raise(self, tmp_path: Path) -> None:
        lib = load_model_library(json_path=tmp_path / "nope.json")
        assert len(lib.specs) == 26


class TestFreeBaysOf:
    def test_matches_builtin_free_bays(self) -> None:
        """新函数必须与老的 `portspec.free_bays` 同结果。"""
        from idcbox.domain.ports.portspec import free_bays

        lib = _base()
        used = ["25GE1/0/1", "100GE1/0/1", "100GE2/0/1"]
        assert free_bays_of(lib.spec_of(CORE), used) == free_bays(CORE, used)

    def test_works_for_model_not_in_builtin(self) -> None:
        """型号来自项目库时也必须算得出空闲口。

        若内部回退到 `portspec.spec_of()`（只认内置表），
        项目库里的新型号会得到「空闲 0 个口」—— 不报错、不变红，只是数字永远 0。
        """
        lib = load_model_library(
            excel_entries=parse_library_json({"models": [_custom()]})[0]
        )
        spec = lib.spec_of("自研汇聚交换机X")
        # 端口写法必须与源表一致：板卡式 `10GE1/0/1`（机箱/槽/口）。
        # 写 `10GE1/1` 会被解析成 unknown → 不匹配任何模板口 → 空闲数不变。
        free = free_bays_of(spec, ["10GE1/0/1", "100GE1/0/4"])
        assert len(free) == 26
        assert ("10GE", 1, 1) not in free
        assert ("100GE", 1, 4) not in free
