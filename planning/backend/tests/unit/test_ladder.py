"""阶梯 T 的边界（这一段是本项目最容易实现错的地方）。

## 四个实现，只有一个能进生产

| 实现 | 复现率 | 能否进生产 |
|---|---|---|
| `ifs_compat` 逐字复刻原 IFS | 636/636 = 100% | ✅ 默认 |
| `continuous` 只补 (6,7) 缝隙 | 587/636 = 92.3% | ⚠️ 可选开关 |
| `ladder_ceil_tiers` 档位表上取整 | 424/636 = 66.7% | ❌ 仅测试负样本 |
| `ladder_round_nearest` 档位表四舍五入 | 111/636 = 17.5% | ❌ 仅测试负样本 |

## 原公式的 bug 必须被保留（默认模式）

```
=_xlfn.IFS(S2<=5,5, S2<=6,7, S2=7,7, S2<=8.5,10, ...)
```

`S<=6→7` 之后紧跟 `S=7→7`，再 `S<=8.5→10`。于是 `S∈(6,7)` 这个开区间
**落不到任何 `eq` 分支**，直接掉进 `S<=8.5→10`。

实测 S 的取值里有 49 行恰好是 6.44（`P=2` → `R=1.44`、`Q=5` → `S=6.44`），
它们本该 7 米，原公式给 10 米 —— **每行多买一档线缆**。

为什么系统仍默认复刻这个 bug：用户手里的 Excel 还是原公式，系统输出不一致会让
导出文件"自己改值"，信任立刻崩塌。正确做法是**保留原式 + 体检项 F4 主动告知**
「这 49 行按你的公式给 10 米、按连续口径只需 7 米」，由用户决定。
"""

from __future__ import annotations

import pytest

from idcbox.domain.cable_length.ladder import (
    CONTINUOUS,
    IFS_COMPAT,
    LADDER_TIERS,
    NON_PRODUCTION_IMPLS,
    PURCHASE_TIERS,
    ladder_ceil_tiers,
    ladder_gap_rows,
    ladder_round,
    ladder_round_nearest,
    to_purchase_tier,
)
from idcbox.domain.cable_length.rules import CONTINUOUS_RULE, DEFAULT_RULE


class TestIfsCompat:
    """逐条验证 IFS 的每个分支：`le` 取等号、`eq` 只取等号。"""

    @pytest.mark.parametrize(
        ("s", "expected"),
        [
            (0.0, 5),
            (4.99, 5),
            (5.0, 5),      # le 5 取等
            (5.01, 7),
            (6.0, 7),      # le 6 取等
            (6.44, 10),    # ★ 缝隙：本应 7，原公式给 10
            (6.99, 10),    # ★ 缝隙
            (7.0, 7),      # eq 7 只取等 —— 这是最反直觉的一条
            (7.01, 10),
            (8.5, 10),
            (8.51, 15),
            (13.0, 15),
            (13.01, 20),
            (17.5, 20),
            (17.51, 25),
            (22.0, 25),
            (22.01, 30),
            (27.0, 30),
            (27.01, 35),
            (32.0, 35),
            (32.01, 40),
            (37.0, 40),
        ],
    )
    def test_boundaries(self, s, expected):
        assert ladder_round(s, IFS_COMPAT) == expected

    def test_overflow_returns_none(self):
        """S>37 原公式无 else 分支 → 必须返回 None（而非 0 或 40）。"""
        assert ladder_round(37.0001, IFS_COMPAT) is None
        assert ladder_round(50, IFS_COMPAT) is None

    def test_not_monotonic(self):
        """关键事实：这不是单调阶梯 —— 6.99→10 而 7.0→7。"""
        assert ladder_round(6.99, IFS_COMPAT) > ladder_round(7.0, IFS_COMPAT)

    def test_default_rule_uses_ifs_compat(self):
        assert DEFAULT_RULE.ladder_mode == "ifs_compat"
        assert DEFAULT_RULE.rungs == IFS_COMPAT

    def test_t_of_delegates(self):
        assert DEFAULT_RULE.t_of(6.44) == 10
        assert DEFAULT_RULE.t_of(7.0) == 7


class TestContinuous:
    def test_gap_filled(self):
        """连续化只改一条：`eq 7` → `le 7`。"""
        assert ladder_round(6.44, CONTINUOUS) == 7
        assert ladder_round(6.99, CONTINUOUS) == 7
        assert ladder_round(7.0, CONTINUOUS) == 7

    def test_now_monotonic(self):
        """补上缝隙后变成单调阶梯。"""
        assert ladder_round(6.99, CONTINUOUS) <= ladder_round(7.0, CONTINUOUS)

    def test_only_differs_in_gap(self):
        """两实现**只在 (6,7) 开区间内不同**，其余完全相同。

        实测确认（见下表）：`7.01` 之后两者又一致了，因为原公式的
        `<=8.5→10` 与连续化的 `<=8.5→10` 是同一条。

            S        ifs   cont
            6.10     10    7     ← 不同
            6.44     10    7     ← 不同
            6.99     10    7     ← 不同
            7.00      7    7     ← 相同（eq 分支）
            7.01     10   10     ← 相同
        """
        for s in (0, 4.99, 5, 5.01, 5.5, 6, 7, 7.01, 8, 8.5, 13, 20, 37):
            assert ladder_round(s, IFS_COMPAT) == ladder_round(s, CONTINUOUS), (
                f"S={s} 在缝隙之外却给出不同结果"
            )
        for s in (6.01, 6.1, 6.44, 6.99):
            assert ladder_round(s, IFS_COMPAT) != ladder_round(s, CONTINUOUS), (
                f"S={s} 在缝隙区间却给出相同结果"
            )

    def test_continuous_rule(self):
        assert CONTINUOUS_RULE.ladder_mode == "continuous"
        assert CONTINUOUS_RULE.t_of(6.44) == 7


class TestNonProductionImpls:
    """两个档位法函数只作负样本，绝不能进生产路径。

    注意 `ladder_ceil_tiers(6.44)` 得 **7** 而不是 10：档位表是 [5,7,10,...]，
    第一个 ≥6.44 的档位就是 7。它之所以只有 424/636 正确，是因为它在
    **别的区间**（如 S=20 → 20 而非 25）与 IFS 不同，不是因为缝隙。
    """

    def test_ceil_tiers(self):
        assert ladder_ceil_tiers(6.44) == 7    # 第一个 >=6.44 的档位是 7
        assert ladder_ceil_tiers(5.0) == 5
        assert ladder_ceil_tiers(4.0) == 5
        assert ladder_ceil_tiers(7.01) == 10
        assert ladder_ceil_tiers(20.0) == 20   # ★ 这里 IFS 给 25，档位法给 20
        assert ladder_ceil_tiers(37.5) == 40   # IFS 无定义，档位法仍给 40

    def test_ceil_tiers_differs_from_ifs_outside_gap(self):
        """档位法的错误不在缝隙里，而在「恰好等于档位值」时不再进档。

        实测差异点（S 恰为档位值时，IFS 因为用 `<=` 会进下一档）：
        S=10→ifs 15/ceil 10、15→20/15、20→25/20、25→30/25、30→35/30、35→40/35、40→None/40。
        """
        for s in (10, 15, 20, 25, 30, 35):
            assert ladder_ceil_tiers(s) != ladder_round(s, IFS_COMPAT), (
                f"S={s} 处档位法应与 IFS 不同"
            )
        # 非档位值上两者一致（除缝隙）
        for s in (1, 2, 8, 12, 17.5, 22, 27, 32):
            assert ladder_ceil_tiers(s) == ladder_round(s, IFS_COMPAT)

    def test_round_nearest(self):
        assert ladder_round_nearest(5.0) == 5
        assert ladder_round_nearest(6.0) == 5   # equidistant 取小
        assert ladder_round_nearest(6.44) == 7  # 更接近 7
        assert ladder_round_nearest(8.0) == 7
        assert ladder_round_nearest(37.5) == 35

    def test_registered_as_non_production(self):
        """注册表存在，防止有人误把它们当默认实现。"""
        assert set(NON_PRODUCTION_IMPLS) == {
            "ladder_ceil_tiers",
            "ladder_round_nearest",
        }
        assert ladder_ceil_tiers in NON_PRODUCTION_IMPLS.values()
        assert ladder_round_nearest in NON_PRODUCTION_IMPLS.values()


class TestTiers:
    def test_ladder_has_9_tiers(self):
        """展示口径 9 档（含 35）。"""
        assert LADDER_TIERS == (5, 7, 10, 15, 20, 25, 30, 35, 40)

    def test_purchase_has_8_tiers(self):
        """采购口径 8 档 —— 35 不单独采购，并入 40。"""
        assert PURCHASE_TIERS == (5, 7, 10, 15, 20, 25, 30, 40)
        assert 35 not in PURCHASE_TIERS

    def test_to_purchase_tier(self):
        assert to_purchase_tier(5) == 5
        assert to_purchase_tier(35) == 40, "35 米不单独采购，入库为 40 米档"
        assert to_purchase_tier(40) == 40
        assert to_purchase_tier(30) == 30


class TestGapDetection:
    def test_gap_rows(self):
        """缝隙区间必须能被程序检出（供 F4 体检项使用）。"""
        gaps = ladder_gap_rows(IFS_COMPAT)
        assert gaps == ((6.0, 7.0),), f"应只检出 (6,7) 一个缝隙，实际 {gaps}"

    def test_continuous_has_no_gap(self):
        assert ladder_gap_rows(CONTINUOUS) == ()
