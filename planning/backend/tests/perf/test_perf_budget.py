"""性能预算用例（NFR：全量重算 < 3s、增量 < 200ms）。

## 为什么阈值定得比实测宽 10 倍

实测全量 310ms、单柜增量 1.3ms。阈值定 3s / 200ms 而不是 400ms / 10ms，
理由是**这类断言的目的不是压性能，是拦住数量级退化**：

- 有人在 `compute_link` 里加一次 `datetime.now()` 或一次字典深拷贝 →
  从 310ms 变成 10s，会被拦下
- 从 310ms 变成 500ms（机器繁忙、CI 环境慢）不该让 CI 变红

真按窄阈值卡，CI 会在慢机器上随机变红，最后所有人都学会"重跑一次"——
那这个测试就废了。
"""

from __future__ import annotations

import time

import pytest

pytestmark = pytest.mark.perf

FULL_RECOMPUTE_BUDGET_MS = 3000
INCREMENTAL_BUDGET_MS = 200


@pytest.fixture(scope="module")
def project():
    from idcbox.application.services.pipeline import load_project
    from idcbox.infrastructure.xlsx.sources import XlsxCablingSource, XlsxFloorSource
    from paths import source_path

    src = source_path()
    return load_project(
        floor_source=XlsxFloorSource(src),
        cabling_source=XlsxCablingSource(src),
        label="V13",
        source_path=src,
    )


def test_full_recompute_under_budget(project):
    """5127 行全量重算必须远快于交互阈值。

    这个数字是「用户在页面上改一个 U 位就能看到长度全表刷新」的前提。
    超过 3s 就必须改用 Worker + 增量，否则交互会卡。
    """
    from idcbox.domain.cable_length.engine import compute_batch

    ts: list[float] = []
    for _ in range(3):
        t0 = time.perf_counter()
        compute_batch(project.links, project.rule, project.geometry)
        ts.append((time.perf_counter() - t0) * 1000)
    median = sorted(ts)[1]
    assert median < FULL_RECOMPUTE_BUDGET_MS, f"全量重算中位数 {median:.0f}ms 超预算"


def test_incremental_is_much_faster(project):
    """改一个机柜只该重算受影响的连线，耗时必须是全量的零头。"""
    from idcbox.domain.cable_length.engine import compute_batch, compute_incremental

    t0 = time.perf_counter()
    full = compute_batch(project.links, project.rule, project.geometry)
    full_ms = (time.perf_counter() - t0) * 1000
    prev = full.by_key()

    ts: list[float] = []
    for _ in range(3):
        t0 = time.perf_counter()
        compute_incremental(
            project.links, {(1, 7)}, project.rule, project.geometry, prev=prev
        )
        ts.append((time.perf_counter() - t0) * 1000)
    inc_ms = min(ts)

    assert inc_ms < INCREMENTAL_BUDGET_MS, f"单柜增量 {inc_ms:.0f}ms 超预算"
    # 增量至少要比全量快 10 倍，否则说明筛选没生效（等于全量重算）
    assert inc_ms * 10 < full_ms, f"增量 {inc_ms:.0f}ms 与全量 {full_ms:.0f}ms 同数量级，筛选可能失效"


def test_audit_under_budget(project):
    """体检要能在导入后立刻出结果，不能让用户等。"""
    from idcbox.domain.validation.rules import run_audit

    ctx = project.audit_context()
    t0 = time.perf_counter()
    run_audit(ctx)
    ms = (time.perf_counter() - t0) * 1000
    assert ms < FULL_RECOMPUTE_BUDGET_MS, f"体检 {ms:.0f}ms 超预算"
