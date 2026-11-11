/**
 * 耗材与模块清单（FR-8.x，采购依据）。
 *
 * ## 这一页回答的唯一问题：「要买多少根线、多少个模块」
 *
 * 前几个页签都是「接得对不对」，这一页是「买多少」。
 * 它跑的还是同一份布线数据，只是汇总口径完全不同。
 *
 * ## 三个必须在界面上说清的事（否则用户会买错）
 *
 * 1. **口径是首期还是全项目**：原表只有 450/5127 行标了 `首期`（8.8%）。
 *    不给筛选就是全项目需求，看着像是首期量的 11 倍。
 * 2. **采购量是建议不是结论**：实测人工核定倍数是 1.124~2.314
 *    （10G-SR 买 2.3 倍备件，100G-SR4 只买 1.12 倍），固定系数复现不了，
 *    最终数量必须由人定。所以采购列带「建议」字样且标黄。
 * 3. **模块覆盖率必须显示**：模块列只有 570/5113 行有值（11.1%），
 *    按这个比例外推会少买模块，是现场停工级事故。
 */

import { useEffect, useMemo, useState } from 'react';
import type { JSX } from 'react';

import { api } from '../api/client';
import type { ConsumablesPayload } from '../api/client';

/** 线缆类型配色，与布线表页保持一致。 */
const TYPE_CSS: Record<string, string> = {
  'LC-LC': 'cable-lc',
  MPO: 'cable-mpo',
  RJ45: 'cable-rj45',
};

function pct(n: number): string {
  return `${(n * 100).toFixed(1)}%`;
}

export function ConsumablesView(): JSX.Element {
  const [phase, setPhase] = useState<string>('首期');
  const [data, setData] = useState<ConsumablesPayload | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    setErr(null);
    api
      .consumables(phase ? { phase } : undefined)
      .then((d) => {
        if (alive) setData(d);
      })
      .catch((e: unknown) => {
        if (alive) setErr(e instanceof Error ? e.message : String(e));
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [phase]);

  /** 线缆表按「有需求的档位」铺开，需求 0 的档位不显示（避免 24 行全是 0）。 */
  const cableRows = useMemo(() => data?.cables.rows ?? [], [data]);

  if (err) {
    return (
      <div className="panel">
        <div className="error-box">
          <b>清单加载失败：</b>
          {err}
        </div>
      </div>
    );
  }

  return (
    <div className="consumables-view">
      <section className="panel">
        <div className="panel-head">
          <h2>耗材与模块需求</h2>
          <div className="phase-switch">
            <label htmlFor="phase-select">口径</label>
            <select
              id="phase-select"
              value={phase}
              onChange={(e) => setPhase(e.target.value)}
            >
              <option value="首期">首期采购（原表标记）</option>
              <option value="">全项目（不过滤批次）</option>
            </select>
          </div>
        </div>

        <div className="consume-headline">
          <div className="consume-big">
            <span className="consume-num">{data?.cables.total ?? '—'}</span>
            <span className="consume-label">线缆（根）</span>
          </div>
          <div className="consume-big">
            <span className="consume-num">
              {data ? data.modules.rows.reduce((s, m) => s + m.total, 0) : '—'}
            </span>
            <span className="consume-label">模块（个）</span>
          </div>
          <div className="consume-big">
            <span className="consume-num">{data?.rowsTotal ?? '—'}</span>
            <span className="consume-label">参与汇总的连线行</span>
          </div>
        </div>

        <p className="subtle consume-scope">
          {phase
            ? `当前口径：阶段「${phase}」—— 只统计布线表备注列为「${phase}」的行。`
            : '当前口径：全项目 —— 不过滤批次，等于「一次买齐」的量。'}
          {loading && ' 加载中…'}
        </p>
      </section>

      <section className="panel">
        <h2>线缆需求</h2>
        <table className="consume-table">
          <thead>
            <tr>
              <th>类型</th>
              <th className="num">长度</th>
              <th className="num">需求数量</th>
              <th className="num suggested">建议采购</th>
            </tr>
          </thead>
          <tbody>
            {cableRows.map((r) => (
              <tr key={`${r.type}-${r.length}`}>
                <td>
                  <span className={`cable-chip ${TYPE_CSS[r.type] ?? ''}`}>{r.type}</span>
                </td>
                <td className="num">{r.length} m</td>
                <td className="num">{r.demand}</td>
                <td className="num suggested">{r.suggestedPurchase}</td>
              </tr>
            ))}
            {cableRows.length === 0 && (
              <tr>
                <td colSpan={4} className="empty-row">
                  该口径下没有线缆需求
                </td>
              </tr>
            )}
          </tbody>
        </table>

        <p className="callout warn">
          <b>建议采购</b> = 需求 × {data?.cables.purchaseRatio ?? 1.3}（向上取整），
          <b>不是最终采购量</b>。原表人工核定的倍数是 1.124~2.314（10G-SR 买 2.3 倍备件、
          100G-SR4 只买 1.12 倍），固定系数复现不了 —— 请按预算与交期逐档核定。
        </p>
      </section>

      <section className="panel">
        <h2>模块需求</h2>
        <table className="consume-table">
          <thead>
            <tr>
              <th>模块</th>
              <th className="num">本端</th>
              <th className="num">对端</th>
              <th className="num">实际需求</th>
            </tr>
          </thead>
          <tbody>
            {(data?.modules.rows ?? []).map((m) => (
              <tr key={m.module}>
                <td>
                  <span className="module-chip">{m.module}</span>
                </td>
                <td className="num">{m.local}</td>
                <td className="num">{m.peer}</td>
                <td className="num strong">{m.total}</td>
              </tr>
            ))}
            {(data?.modules.rows ?? []).length === 0 && (
              <tr>
                <td colSpan={4} className="empty-row">
                  该口径下没有模块需求
                </td>
              </tr>
            )}
          </tbody>
        </table>

        <div className="coverage-bar">
          <div className="coverage-track">
            <div
              className="coverage-fill"
              style={{ width: pct(data?.modules.coverageRatio ?? 0) }}
            />
          </div>
          <span className="coverage-text">
            {data
              ? `覆盖 ${data.modules.coverageRows}/${data.modules.totalRows} 行（${pct(
                  data.modules.coverageRatio,
                )}）`
              : '—'}
          </span>
        </div>

        <p className="callout warn">
          {data?.modules.disclosure ??
            '模块列只有部分行有值；其余行未填模块，未做外推。'}
        </p>
      </section>
    </div>
  );
}
