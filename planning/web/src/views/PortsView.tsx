/**
 * 端口与策略视图（FR-7.x）。
 *
 * ## 这一页回答三个"我在 Excel 里查不到"的问题
 *
 * 1. **端口用了多少**：按型号统计已用端口/设备数，找出"快满了"的交换机
 * 2. **长度从哪来**：四种策略（公式/经验表/人工/导入原值）各占多少行
 *    —— 这是"87.6% 的存量行根本不走公式"这一发现的直接可视化
 * 3. **几何可信度**：A/B/C 三类空间关系各多少行，C 类（60% 吻合）就该人工复核
 */

import { useMemo } from 'react';
import type { JSX } from 'react';

import { api } from '../api/client';
import { useEffect, useState } from 'react';
import type { PortUtilization } from '../api/client';
import { useAppStore } from '../store/appStore';

const POLICY_META: Record<string, { label: string; desc: string; css: string }> = {
  GEOMETRIC: {
    label: '公式链',
    desc: '按 N/O/P/Q/R/S/T 七公式算出，新建规划的主路径',
    css: 'policy-geometric',
  },
  EMPIRICAL: {
    label: '经验表',
    desc: '按（空间类·线缆类型·路径距离）查表，覆盖率 84.3%、覆盖内 94.0%',
    css: 'policy-empirical',
  },
  MANUAL: {
    label: '人工填写',
    desc: '公式与经验表只作参考，以人工核定值为准',
    css: 'policy-manual',
  },
  IMPORTED: {
    label: '导入原值',
    desc: '存量迁移：保留源表原值，不参与长度偏差告警',
    css: 'policy-imported',
  },
};

const O_CLASS_META: Record<string, { label: string; rate: string; css: string }> = {
  A: { label: '同排面对面', rate: '93.1% 吻合', css: 'oclass-a' },
  B: { label: '同通道跨侧', rate: '98.0% 吻合', css: 'oclass-b' },
  C: { label: '跨通道', rate: '60.0% 吻合 · 建议人工复核', css: 'oclass-c' },
};

const fmtInt = (n: number): string => n.toLocaleString('zh-CN');

export function PortsView(): JSX.Element {
  const policies = useAppStore((s) => s.policies);
  const [util, setUtil] = useState<PortUtilization | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    api
      .portUtilization()
      .then(setUtil)
      .catch((e: unknown) => setErr(e instanceof Error ? e.message : String(e)));
  }, []);

  const totalPolicyRows = useMemo(
    () => (policies ? policies.policies.reduce((s, p) => s + p.count, 0) : 0),
    [policies],
  );

  const totalOClass = useMemo(
    () => (policies ? policies.oClasses.reduce((s, c) => s + c.count, 0) : 0),
    [policies],
  );

  return (
    <div className="ports-view">
      {policies && (
        <section className="panel fresh-panel">
          <h3>如果这是一个新建项目，系统能替你算多少行？</h3>
          <p className="panel-note">
            下面这组数字才是「自动化覆盖率」的诚实口径：它忽略了原表已填的长度，
            假装从零开始算一遍。存量导入项目里 99.8% 的行是 <code>IMPORTED</code>
            （保留原值），所以那个分布式不能反映系统的计算能力。
          </p>
          <div className="fresh-bar">
            <div
              className="fresh-seg fresh-geometric"
              style={{
                width: `${(policies.freshProject.geometric / policies.freshProject.total) * 100}%`,
              }}
              title={`几何公式可算：${policies.freshProject.geometric} 行`}
            >
              {policies.freshProject.geometricPct.toFixed(1)}% 可自动算
            </div>
            {policies.freshProject.empirical > 0 && (
              <div
                className="fresh-seg fresh-empirical"
                style={{
                  width: `${(policies.freshProject.empirical / policies.freshProject.total) * 100}%`,
                }}
              >
                经验表 {policies.freshProject.empirical}
              </div>
            )}
            {policies.freshProject.manual > 0 && (
              <div
                className="fresh-seg fresh-manual"
                style={{
                  width: `${Math.max(
                    (policies.freshProject.manual / policies.freshProject.total) * 100,
                    2,
                  )}%`,
                }}
                title={`必须人工定档：${policies.freshProject.manual} 行`}
              >
                {policies.freshProject.manual} 行需人工
              </div>
            )}
          </div>
          <div className="fresh-legend">
            <span>
              共 <b>{policies.freshProject.total}</b> 行
            </span>
            <span>
              几何公式 <b>{policies.freshProject.geometric}</b>
            </span>
            <span>
              经验表 <b>{policies.freshProject.empirical}</b>
            </span>
            <span>
              强制人工 <b>{policies.freshProject.manual}</b>
            </span>
          </div>
        </section>
      )}

      <section className="panel">
        <h3>长度决策策略分布（本项目的实际状态）</h3>
        <p className="panel-note">
          一份真实交付物里往往同时存在多种策略。存量导入时，原表已填的长度
          全部落 <code>IMPORTED</code>，系统不会覆盖，也不会报"与计算值不符"。
        </p>
        {policies ? (
          <div className="policy-grid">
            {policies.policies.map((p) => {
              const meta = POLICY_META[p.policy] ?? {
                label: p.policy,
                desc: '',
                css: 'policy-manual',
              };
              const pct = totalPolicyRows > 0 ? (p.count / totalPolicyRows) * 100 : 0;
              return (
                <div key={p.policy} className={`policy-card ${meta.css}`}>
                  <div className="policy-card-head">
                    <b>{meta.label}</b>
                    <span className="policy-count">{p.count}</span>
                  </div>
                  <div className="policy-bar">
                    <i style={{ width: `${pct}%` }} />
                  </div>
                  <div className="policy-pct">{pct.toFixed(1)}%</div>
                  <p className="policy-desc">{meta.desc}</p>
                  <span className={`policy-deviation${p.participatesInDeviation ? ' on' : ' off'}`}>
                    {p.participatesInDeviation ? '参与长度偏差告警' : '不参与偏差告警'}
                  </span>
                </div>
              );
            })}
          </div>
        ) : (
          <div className="view-placeholder">加载中…</div>
        )}
      </section>

      <section className="panel">
        <h3>几何空间关系分类</h3>
        <p className="panel-note">
          O（间隔机柜）不是单一公式。A 类直接横穿通道，B/C 类必须走到排端走线架再折返。
          C 类只有 60% 吻合，产品上给建议值 + 强制人工复核标记。
        </p>
        {policies && (
          <div className="oclass-grid">
            {policies.oClasses.map((c) => {
              const meta = O_CLASS_META[c.class] ?? { label: c.class, rate: '', css: 'oclass-c' };
              const pct = totalOClass > 0 ? (c.count / totalOClass) * 100 : 0;
              return (
                <div key={c.class} className={`oclass-card ${meta.css}`}>
                  <div className="oclass-head">
                    <b>{c.class} 类</b>
                    <span>{c.count} 行</span>
                  </div>
                  <div className="oclass-label">{meta.label}</div>
                  <div className="oclass-rate">{meta.rate}</div>
                  <div className="policy-bar">
                    <i style={{ width: `${pct}%` }} />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </section>

      <section className="panel">
        <h3>置信度分布</h3>
        <p className="panel-note">
          低置信度的行只给建议、不进自动耗材汇总 —— 宁可少算，也不让采购清单
          带着猜出来的数字。
        </p>
        {policies && (
          <div className="conf-bars">
            {policies.confidence.map((c) => {
              const total = policies.confidence.reduce((s, x) => s + x.count, 0);
              const pct = total > 0 ? (c.count / total) * 100 : 0;
              return (
                <div key={c.level} className="conf-row">
                  <span className="conf-name">{c.level}</span>
                  <div className="conf-bar">
                    <i style={{ width: `${pct}%` }} />
                  </div>
                  <span className="conf-val">
                    {c.count} <span className="subtle">({pct.toFixed(1)}%)</span>
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </section>

      <section className="panel">
        <h3>端口使用率（按设备型号）</h3>
        {err && <div className="error-box">{err}</div>}
        {util ? (
          <>
            <div className="util-summary">
              <span>
                全部 <b>{util.summary.models}</b> 个型号 ·{' '}
                <b>{util.summary.modelsWithSpec}</b> 个有端口模板
              </span>
              <span className="util-ratio">
                已用 <b>{fmtInt(util.summary.usedPorts)}</b> / 标称{' '}
                <b>{fmtInt(util.summary.totalPorts)}</b>
                <em>（{(util.summary.ratio * 100).toFixed(1)}%）</em>
              </span>
              {util.noSpecModels.length > 0 && (
                <span className="util-warn">
                  {util.noSpecModels.length} 个型号无模板，不计入分母
                </span>
              )}
            </div>
            <table className="port-table">
              <thead>
                <tr>
                  <th>设备型号</th>
                  <th className="num">设备数</th>
                  <th className="num">已用</th>
                  <th className="num">标称</th>
                  <th className="num">空闲</th>
                  <th className="num">利用率</th>
                  <th>最常用端口</th>
                </tr>
              </thead>
              <tbody>
                {util.byModel.map((m) => (
                  <tr key={m.model}>
                    <td>{m.model}</td>
                    <td className="num">{m.deviceCount}</td>
                    <td className="num">{fmtInt(m.usedPorts)}</td>
                    <td className="num">{m.totalPorts === null ? '—' : fmtInt(m.totalPorts)}</td>
                    <td className="num">{m.freePorts === null ? '—' : fmtInt(m.freePorts)}</td>
                    <td className="num">
                      {m.ratio === null ? (
                        <span className="ratio-na" title="型号没有端口模板，分母未知">
                          —
                        </span>
                      ) : (
                        <span className={'ratio-bar' + (m.ratio >= 0.95 ? ' ratio-full' : '')}>
                          {(m.ratio * 100).toFixed(0)}%
                        </span>
                      )}
                    </td>
                    <td className="ports-cell">
                      {m.topPorts.map((p) => (
                        <span key={p.port} className="port-chip">
                          {p.port}
                          <i>{p.count}</i>
                        </span>
                      ))}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {util.unknownPortDevices.length > 0 && (
              <div className="util-attention">
                <strong>⚠️ {util.unknownPortDevices.length} 台设备出现模板里没有的端口</strong>
                <ul>
                  {util.unknownPortDevices.map((line) => (
                    <li key={line}>{line}</li>
                  ))}
                </ul>
                {util.modelConflicts.length > 0 && (
                  <p className="util-attention-note">
                    先看下面这组：同一个实例名记了两个型号的设备，会被当成两台、端口分散到两边，
                    看起来就是「未知端口」。<b>这是源表型号记错，不是模板问题</b>。
                    <br />
                    涉及的设备：{util.modelConflicts.join('、')}
                  </p>
                )}
              </div>
            )}
            <p className="panel-note">{util.note}</p>
          </>
        ) : (
          !err && <div className="view-placeholder">加载中…</div>
        )}
      </section>
    </div>
  );
}
