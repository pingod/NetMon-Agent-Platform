"""端到端集成测试：CLI 四个子命令与流水线三段。

## 为什么这批测试不能少

golden test 验证的是「域层函数对不对」，unit test 验证的是「边界处理对不对」，
但两者都不验证**装配是否成立**：数据源适配器接得对不对、导出行的 raw 视图
是否真的覆盖了 24 列、`verify` 的比较口径是否与源文件一致。

历史上这里出过两个真 bug（都是装配层、都不是域层）：
1. `raw_values` 只存了 11 个长度列 → 设备名归一化后导出丢空格
2. 导出器把「源值为空」当成「没有源值」→ 回填计算值，保真破功

这两个 bug 用域层单测永远测不出来，只有跑一遍真实文件的端到端才能发现。

## 为什么用真实文件而不是 fixture

因为要比的就是「与用户手里那份 Excel 是否逐格相同」。
用裁剪过的 fixture 测出来的"通过"没有意义 —— 脏数据、sentinel、重复列名、
空调位跳号全都藏在真实文件里。宁可让测试依赖源文件存在（找不到就 fail，
不是 skip），也不要一个永远绿的假测试。
"""

from __future__ import annotations

import warnings

import pytest

from idcbox.cli import EXIT_MISMATCH, EXIT_OK, main

pytestmark = pytest.mark.integration

warnings.filterwarnings("ignore")


class TestPipelineStages:
    """流水线三段能各自独立调用（增量场景依赖这个性质）。"""

    def test_load_only_does_not_compute(self, geo, rows):
        from pathlib import Path

        from idcbox.application.services.pipeline import load_project
        from idcbox.infrastructure.xlsx.sources import XlsxCablingSource, XlsxFloorSource
        from paths import source_path

        src = source_path()
        project = load_project(
            floor_source=XlsxFloorSource(src),
            cabling_source=XlsxCablingSource(src),
            label="V13",
            source_path=src,
        )
        assert project.rack_count == 362
        assert project.cmax_by_room == {"403": 38, "404": 18}
        assert project.total_rows == 5127
        assert project.effective_rows == 5127
        # 只 load 不该有计算结果
        assert not hasattr(project, "batch")

    def test_compute_needs_no_io(self, geo, rows):
        """compute 只吃内存里的对象 —— 这保证它能进 Worker / 批量重算。"""
        from idcbox.application.services.pipeline import compute_project, load_project
        from idcbox.infrastructure.xlsx.sources import XlsxCablingSource, XlsxFloorSource
        from pathlib import Path  # noqa: F401
        from paths import source_path

        src = source_path()
        project = load_project(
            floor_source=XlsxFloorSource(src),
            cabling_source=XlsxCablingSource(src),
            label="V13",
            source_path=src,
        )
        computed = compute_project(project)
        assert len(computed.batch.results) == 5127
        assert len(computed.resolved) == 5127
        # 两套结果键集合必须完全一致（同一批行的两种投影）
        assert set(computed.batch.by_key()) == set(computed.by_key())

    def test_audit_context_reuses_loaded_rows(self, geo, rows):
        """体检上下文必须复用已加载的行，不能重读文件。"""
        from idcbox.application.services.pipeline import check_project, compute_project, load_project
        from idcbox.infrastructure.xlsx.sources import XlsxCablingSource, XlsxFloorSource
        from paths import source_path

        src = source_path()
        project = load_project(
            floor_source=XlsxFloorSource(src),
            cabling_source=XlsxCablingSource(src),
            label="V13",
            source_path=src,
        )
        ctx = project.audit_context()
        assert ctx.row_count == 5127
        assert list(ctx.row_nos)[:3] == [2, 3, 4]
        checked = check_project(compute_project(project))
        assert checked.report.total_rows == 5127
        assert abs(checked.report.score - 94.2) < 0.2


class TestCliVerify:
    """`verify` 是验收铁线的一键装置，必须零差异。"""

    def test_verify_reports_zero_diff(self, tmp_path):
        out = tmp_path / "verify.xlsx"
        code = main(["verify", "--out", str(out)])
        assert code == EXIT_OK
        assert out.exists()

    def test_verify_is_idempotent(self, tmp_path):
        """连跑两次必须都通过，且内容一致 —— 防止「第一次靠缓存歪打正着」。

        比的是**单元格内容**，不是文件字节：xlsx 是 zip，
        内部带写入时间戳，字节数天然不同（实测差 2 字节），
        拿 size 比较会得到假失败。
        """
        import openpyxl

        a = tmp_path / "a.xlsx"
        b = tmp_path / "b.xlsx"
        assert main(["verify", "--out", str(a)]) == EXIT_OK
        assert main(["verify", "--out", str(b)]) == EXIT_OK

        wa = openpyxl.load_workbook(a, data_only=True)["布线表"]
        wb_ = openpyxl.load_workbook(b, data_only=True)["布线表"]
        assert wa.max_row == wb_.max_row
        for r in (2, 100, 1000, 3000, 5128):
            for c in (1, 14, 15, 20, 21):
                assert wa.cell(row=r, column=c).value == wb_.cell(row=r, column=c).value


class TestCliExportModes:
    def test_value_mode_is_archive(self, tmp_path):
        out = tmp_path / "value.xlsx"
        assert main(["export", "--out", str(out), "--mode", "value"]) == EXIT_OK
        import openpyxl

        wb = openpyxl.load_workbook(out, data_only=True)
        ws = wb["布线表"]
        # value 模式：T 列（第 20 列）写出的是数值而不是公式
        assert ws.cell(row=2, column=20).value is not None
        assert not str(ws.cell(row=2, column=20).value).startswith("=")

    def test_formula_mode_is_editable(self, tmp_path):
        """formula 模式必须留活公式，否则设计院无法二次编辑。"""
        out = tmp_path / "formula.xlsx"
        assert main(["export", "--out", str(out), "--mode", "formula"]) == EXIT_OK
        import openpyxl

        wb = openpyxl.load_workbook(out, data_only=False)
        ws = wb["布线表"]
        # 找一行源表有 N/O 值的（走公式链的行），其 P 列应是活公式
        assert ws.cell(row=2, column=16).value == "=N2+O2"
        assert ws.cell(row=2, column=18).value == "=P2*0.6*120%"
        assert ws.cell(row=2, column=19).value == "=R2+Q2"
        assert str(ws.cell(row=2, column=20).value).startswith("=IFS(")

    def test_formula_mode_keeps_sentinel_t(self, tmp_path):
        """T 列原值是 sentinel（待定）时绝不写公式，否则覆盖人的判断。"""
        out = tmp_path / "formula_sentinel.xlsx"
        assert main(["export", "--out", str(out), "--mode", "formula"]) == EXIT_OK
        import openpyxl

        wb = openpyxl.load_workbook(out, data_only=False)
        ws = wb["布线表"]
        # r332 / r333 的 T 列原值是「待定」
        assert ws.cell(row=332, column=20).value == "待定"
        assert ws.cell(row=333, column=20).value == "待定"


class TestCliCheck:
    def test_check_writes_markdown(self, tmp_path):
        out = tmp_path / "audit.md"
        assert main(["check", "--out", str(out)]) == EXIT_OK
        text = out.read_text(encoding="utf-8")
        assert "体检" in text
        assert "94.2" in text
        # 每条规则都要有标题（否则用户看不懂 code）
        assert "间隔机柜" in text

    def test_blockers_exit_code(self, tmp_path):
        """无 blocker 时返回 0 —— CI 才能直接 `idcbox check || exit 1`。"""
        out = tmp_path / "audit.md"
        assert main(["check", "--out", str(out)]) == EXIT_OK


class TestCliImport:
    def test_import_prints_summary(self, capsys):
        assert main(["import"]) == EXIT_OK
        out = capsys.readouterr().out
        assert "5127 行" in out
        assert "362 个机柜位" in out
        assert "94.2" in out

    def test_import_limit_controls_detail(self, capsys):
        assert main(["import", "--limit", "5"]) == EXIT_OK
        out = capsys.readouterr().out
        assert "前若干条明细" in out
        assert out.count("[D") + out.count("[F") + out.count("[R") <= 5


class TestCliErrors:
    def test_missing_source_exits_usage(self, capsys):
        from idcbox.cli import EXIT_USAGE

        code = main(["import", "--source", "/nonexistent/nope.xlsx"])
        assert code == EXIT_USAGE
        assert "不存在" in capsys.readouterr().err

    def test_bad_sheet_exits_usage(self, capsys):
        from idcbox.cli import EXIT_USAGE

        code = main(["import", "--cabling-sheet", "不存在的sheet"])
        assert code == EXIT_USAGE
