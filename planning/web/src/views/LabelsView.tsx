/**
 * 线缆标签清单（第十一页签）。
 *
 * ## 与「布线表」页的分工
 *
 * 布线表答「哪根线接哪儿」（25 列 × 5127 行的宽表，给设计院看）；
 * 这里答「这张贴纸印什么字」（三段窄文本 × 10254 张，给打线的人贴）。
 * 同一份数据的两种消费方式，份数和排序都不一样。
 *
 * ## 三条界面决定
 *
 * 1. **默认关掉自动编号**。源表里 564 台服务器没有实例名。自动编号会发明
 *    一个客户没见过的名字；若客户已有自己的编号规则，印上去与他们的台账
 *    对不上，等于制造新的一份错误数据。所以默认留空 + 显示缺口数量，
 *    由用户勾选后才编号。
 * 2. **必须显示「哪些标签有问题」**。4230 张缺编码的标签不会让导出的文件
 *    报错，也不会让页面变红 —— 现场贴到一半才发现有一半没有设备名。
 * 3. **提供 txt 下载而不是只在页面看**。现场的用法是「导出一份、丢给标签
 *    打印机」，在浏览器里看不会产生任何交付物。
 */

import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ReactElement } from 'react';

import { api } from '../api/client';
import type { LabelsPayload } from '../api/client';

const PAGE_ROWS = 300;

export function LabelsView(): ReactElement {
  const [data, setData] = useState<LabelsPayload | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [numberMissing, setNumberMissing] = useState(false);
  const [filter, setFilter] = useState('');

  const load = useCallback(
    (numMissing: boolean) => {
      setBusy(true);
      setErr(null);
      api
        .labels({ limit: PAGE_ROWS, numberMissing: numMissing })
        .then(setData)
        .catch((e: unknown) => setErr(e instanceof Error ? e.message : String(e)))
        .finally(() => setBusy(false));
    },
    [],
  );

  useEffect(() => {
    load(false);
  }, [load]);

  const rows = useMemo(() => {
    if (!data) return [];
    const q = filter.trim();
    if (!q) return data.rows;
    return data.rows.filter((r) => r.text.includes(q));
  }, [data, filter]);

  const missingCodes = data?.countsByCode.NO_DEVICE_CODE ?? 0;
  const otherIssues = useMemo(
    () => (data ? data.issues.filter((i) => i.code !== 'NO_DEVICE_CODE') : []),
    [data],
  );

  return (
    <div className="labels-view">
      <section className="panel labels-hero">
        <div className="labels-big">
          <span className="labels-n">{data ? data.total.toLocaleString('zh-CN') : '—'}</span>
          <span className="labels-unit">张标签</span>
        </div>
        <div className="labels-meta">
          <div>
            每根线出 <b>两张</b>（两端各一）· 按施工顺序排（机柜 → U 位从上往下 → 端口）
          </div>
          <div className="labels-dim">
            {data ? `项目：${data.project}` : '加载中…'}
          </div>
        </div>
        <div className="labels-actions">
          <label className="labels-toggle">
            <input
              type="checkbox"
              checked={numberMissing}
              disabled={busy}
              onChange={(e) => {
                setNumberMissing(e.target.checked);
                load(e.target.checked);
              }}
            />
            给没有实例名的设备自动编号
            {missingCodes > 0 && !numberMissing && (
              <span className="labels-gap">（{missingCodes.toLocaleString('zh-CN')} 张缺编码）</span>
            )}
          </label>
          <a
            className="btn-mini"
            href={api.labelsTextUrl({ limit: 20000, numberMissing })}
            download
          >
            下载 txt（给打印机）
          </a>
        </div>
      </section>

      {err && <div className="panel editor-error">{err}</div>}

      {missingCodes > 0 && (
        <section className="panel labels-warn">
          <h3>⚠️ {missingCodes.toLocaleString('zh-CN')} 张标签没有设备编码</h3>
          <p className="panel-note">
            源表里这些设备（服务器与存储节点）本来就没有实例名。它们现在只剩
            「位置 + 端口」，两台相邻服务器都插 <code>万兆1</code> 时标签无法区分。
            勾选上面的「自动编号」会按 <code>SRV01/STG01…</code> 全项目连续编号
            —— 但这是<b>我们发明的名字</b>，若贵方已有自己的编号规则，
            请先改 <code>models.json</code> 或直接在意图里填 <code>instance</code>。
          </p>
        </section>
      )}

      {otherIssues.length > 0 && (
        <section className="panel labels-warn">
          <h3>⚠️ {otherIssues.length} 条其他问题</h3>
          <ul className="issue-list">
            {otherIssues.slice(0, 20).map((i, idx) => (
              <li key={idx} className={`issue-${i.severity}`}>
                <code>{i.code}</code> {i.label} — {i.message}
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="panel">
        <div className="labels-head">
          <h3>标签明细</h3>
          <div className="labels-controls">
            <input
              className="labels-filter"
              placeholder="按任意片段筛选（机柜 / 编码 / 端口）"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
            {data && data.total > data.rows.length && (
              <span className="labels-dim">
                显示前 {data.rows.length} 条 / 共 {data.total.toLocaleString('zh-CN')} 条
                （完整清单请下载 txt）
              </span>
            )}
          </div>
        </div>
        <table className="labels-table">
          <thead>
            <tr>
              <th style={{ width: 48 }}>#</th>
              <th style={{ width: 130 }}>位置</th>
              <th style={{ width: 220 }}>设备编码</th>
              <th>端口</th>
              <th>整条（打印机吃这个）</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, idx) => (
              <tr key={idx}>
                <td className="labels-no">{idx + 1}</td>
                <td className="mono">{r.position}</td>
                <td className="mono">
                  {r.device || <span className="cell-empty">—</span>}
                </td>
                <td className="mono">{r.port}</td>
                <td className="mono labels-full">{r.text}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td colSpan={5} className="empty-box">
                  {data ? '没有匹配的标签' : '加载中…'}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </section>

      <section className="panel labels-note">
        <h3>这条清单是怎么算出来的</h3>
        <ul>
          <li>
            <b>位置</b> = <code>{'{通道}A{机柜号}-{U}U'}</code>，如 <code>11A14-16U</code>。
            编号规则取自「11+12冷通道」示例表的实测格式（2844/2846 行匹配）。
          </li>
          <li>
            <b>设备编码</b> = 设备实例名。<b>交换机</b>额外加 <code>(SW)</code>
            —— 提示后面那段是 CLI 端口名（<code>40GE2/0/1</code>），不是服务器的
            <code>千兆1</code>。
          </li>
          <li>
            <b>排序</b>：机柜升序 → U 位<b>降序</b>（柜顶先贴）→ 端口升序。
            按 U 位升序会让打线的人在柜子里来回弯腰几十次。
          </li>
        </ul>
      </section>
    </div>
  );
}
