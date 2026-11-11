/**
 * 数据体检报告视图（FR-D1~D13）。
 *
 * ## 这是 v0.1 最有差异化的模块
 *
 * 竞品（NetBox、Ralph、openDCIM）都不做"对用户自己的 Excel 做体检"。
 * 而实测这份 5127 行的真实交付物里有：52 处 N/O 算错、13 对 U 位重叠、
 * 49 行被 IFS 公式的 (6,7) 缝隙多算一档、126 行模块未标批次。
 * 这些错误在设计院人工阶段完全不可见，到了现场才发现。
 *
 * ## 为什么按维度分组而不是按严重度排
 *
 * 用户拿到报告的第一反应是"我该先改哪一类"，而不是"第一百条 warning 是什么"。
 * 按维度（几何/长度/设备/端口/模块/流程）分组，每组给出该组的分数与
 * 修一条的收益，用户就能按性价比排序动手。
 */

import { useMemo, useState } from 'react';
import type { JSX } from 'react';

import type { HealthFinding } from '../api/client';
import { useAppStore } from '../store/appStore';

const SEVERITY_LABEL: Record<string, string> = {
  blocker: '阻断',
  warn: '警告',
  info: '提示',
};

/** 维度前缀 → 中文名。规则码首字母决定所属维度（D=数据/长度/几何，R=规则，F=流程）。 */
function dimensionOf(code: string): string {
  if (code.startsWith('D')) return '数据一致性';
  if (code.startsWith('R')) return '规则校验';
  if (code.startsWith('F')) return '流程状态';
  return '其他';
}

export function HealthView(): JSX.Element {
  const health = useAppStore((s) => s.health);
  const [filterRule, setFilterRule] = useState<string | null>(null);
  const [filterSeverity, setFilterSeverity] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  const findings = health?.findings ?? [];

  const visible = useMemo(() => {
    return findings.filter((f) => {
      if (filterRule && f.ruleCode !== filterRule) return false;
      if (filterSeverity && f.severity !== filterSeverity) return false;
      return true;
    });
  }, [findings, filterRule, filterSeverity]);

  const grouped = useMemo(() => {
    const map = new Map<string, HealthFinding[]>();
    for (const f of visible) {
      const dim = dimensionOf(f.ruleCode);
      const list = map.get(dim);
      if (list) list.push(f);
      else map.set(dim, [f]);
    }
    return [...map.entries()];
  }, [visible]);

  if (!health) {
    return <div className="view-placeholder">体检报告加载中…</div>;
  }

  const scoreColor =
    health.score >= 90 ? 'score-good' : health.score >= 70 ? 'score-mid' : 'score-bad';

  return (
    <div className="health-view">
      <div className="health-summary">
        <div className={`health-score ${scoreColor}`}>
          <b>{health.score.toFixed(1)}</b>
          <span>/100</span>
        </div>
        <div className="health-counts">
          <span className="cnt-blocker">
            {health.counts.blocker} 阻断
          </span>
          <span className="cnt-warn">{health.counts.warn} 警告</span>
          <span className="cnt-info">{health.counts.info} 提示</span>
        </div>
        <div className="health-dims">
          {health.scoreBreakdown.map((d) => (
            <div key={d.dimension} className="dim-row">
              <span className="dim-name">{d.dimension}</span>
              <div className="dim-bar">
                <i style={{ width: `${d.score}%` }} />
              </div>
              <span className="dim-val">{d.score.toFixed(1)}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="health-rules">
        <h4>规则命中</h4>
        <table>
          <thead>
            <tr>
              <th>规则</th>
              <th>级别</th>
              <th>说明</th>
              <th className="num">条数</th>
              <th className="num">涉及行</th>
            </tr>
          </thead>
          <tbody>
            {health.rules
              .filter((r) => r.count > 0)
              .sort((a, b) => b.count - a.count)
              .map((r) => (
                <tr
                  key={r.code}
                  className={filterRule === r.code ? 'rule-active' : undefined}
                  onClick={() => setFilterRule(filterRule === r.code ? null : r.code)}
                >
                  <td>
                    <code>{r.code}</code>
                  </td>
                  <td>
                    <span className={`sev sev-${r.severity}`}>{SEVERITY_LABEL[r.severity] ?? r.severity}</span>
                  </td>
                  <td className="rule-title">{r.title}</td>
                  <td className="num">{r.count}</td>
                  <td className="num">{r.affectedRows}</td>
                </tr>
              ))}
          </tbody>
        </table>
      </div>

      <div className="health-findings">
        <div className="findings-toolbar">
          <h4>
            明细 <span className="subtle">（{visible.length} 条）</span>
          </h4>
          <div className="filter-chips">
            {(['blocker', 'warn', 'info'] as const).map((sev) => {
              const n = health.counts[sev];
              if (n === 0) return null;
              return (
                <button
                  key={sev}
                  type="button"
                  className={`chip chip-${sev}${filterSeverity === sev ? ' active' : ''}`}
                  onClick={() => setFilterSeverity(filterSeverity === sev ? null : sev)}
                >
                  {SEVERITY_LABEL[sev]} {n}
                </button>
              );
            })}
            {(filterRule || filterSeverity) && (
              <button
                type="button"
                className="chip"
                onClick={() => {
                  setFilterRule(null);
                  setFilterSeverity(null);
                }}
              >
                清除筛选
              </button>
            )}
          </div>
        </div>

        {grouped.map(([dim, list]) => (
          <div key={dim} className="finding-group">
            <h5>
              {dim} <span className="subtle">（{list.length}）</span>
            </h5>
            <ul className="finding-list">
              {list.slice(0, 100).map((f, i) => {
                const key = `${f.ruleCode}-${f.rowNo ?? i}-${i}`;
                const isOpen = expanded === key;
                return (
                  <li
                    key={key}
                    className={`finding-item sev-border-${f.severity}`}
                    onClick={() => setExpanded(isOpen ? null : key)}
                  >
                    <div className="finding-head">
                      <code className="rule-code">{f.ruleCode}</code>
                      <span className="finding-title">{f.title}</span>
                      {f.rowNo !== null && (
                        <span className="finding-loc">
                          {f.sheet} 第 {f.rowNo} 行
                          {f.column ? ` · ${f.column}` : ''}
                        </span>
                      )}
                      {f.group && <span className="finding-loc">{f.group}</span>}
                    </div>
                    {isOpen && (
                      <div className="finding-body">
                        <p>{f.detail}</p>
                        {f.action && (
                          <p className="finding-action">
                            <b>建议处理：</b>
                            {f.action}
                          </p>
                        )}
                      </div>
                    )}
                  </li>
                );
              })}
              {list.length > 100 && (
                <li className="finding-more">
                  仅显示前 100 条，请用上方规则表筛选后查看
                </li>
              )}
            </ul>
          </div>
        ))}
        {visible.length === 0 && <div className="view-placeholder">没有符合条件的发现</div>}
      </div>
    </div>
  );
}
