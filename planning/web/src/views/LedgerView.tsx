/**
 * 设备台账 / 端口台账视图（v0.5）。
 *
 * ## 这一页回答的问题
 *
 * 「这台交换机还能再挂几台服务器」「这柜子还有没有余量」「哪些设备快插满了」——
 * 这三个问题在 Excel 里都要靠筛选 + 手数，而且是现场扩容决策的直接依据。
 *
 * ## 为什么和「端口与策略」页分开放
 *
 * 那一页按**型号**汇总（「业务接入交换机1 这一批还剩多少口」），
 * 这一页按**设备**展开（「3 号柜那台还剩哪几个口」）。
 * 两个粒度的问题合成一页，两组数字看起来会打架：
 * 一个型号利用率 77%，但其中某台已经插满 —— 用户会以为看错了。
 *
 * ## 三个数字的口径（每次都要交代清楚，否则会被误读）
 *
 * - **分母只算有模板的设备**：型号在 `portspec.py` 里没模板时标称端口数未知，
 *   算成 0 或猜一个会让整体利用率凭空变好看，比不给数字更糟。
 * - **已用端口 ≠ 连线数**：一条线在两端各记一个端口，所以已用 ≈ 连线 × 2。
 * - **无模板显示「—」不显示 0%**：0% 会被当成"全是空的、随便插"。
 */

import { useEffect, useMemo, useState } from 'react';
import type { JSX } from 'react';

import { api } from '../api/client';
import type { LedgerDevice, LedgerPayload, LedgerPort } from '../api/client';

const CATEGORY_LABEL: Record<string, string> = {
  core: '核心交换机',
  switch: '接入交换机',
  server: '服务器',
  storage: '存储',
  security: '安全设备',
  other: '其他',
};

const ROLE_LABEL: Record<string, string> = {
  downlink: '下联',
  uplink: '上联',
  peer_link: '互联',
  stack: '堆叠',
  mgmt: '管理',
  spare: '备用',
  unknown: '未知',
};

const fmtInt = (n: number): string => n.toLocaleString('zh-CN');

function ratioClass(ratio: number | null): string {
  if (ratio === null) return 'ratio-na';
  if (ratio >= 0.95) return 'ratio-full';
  if (ratio >= 0.8) return 'ratio-high';
  return 'ratio-ok';
}

function DeviceDetail({
  device,
  onClose,
}: {
  device: LedgerDevice;
  onClose: () => void;
}): JSX.Element {
  const ports = device.portList ?? [];
  const byRole = useMemo(() => {
    const out: Record<string, number> = {};
    for (const p of ports) {
      out[p.role] = (out[p.role] ?? 0) + 1;
    }
    return out;
  }, [ports]);

  return (
    <div className="device-detail">
      <div className="device-detail-head">
        <div>
          <h3>{device.label}</h3>
          <p className="panel-note">
            {device.model} · 通道 {device.channel} 机柜 {device.rack} · {device.uStart}U 起{' '}
            {device.uSpan}U
          </p>
        </div>
        <button type="button" className="btn-mini" onClick={onClose}>
          关闭
        </button>
      </div>

      <div className="device-detail-stats">
        <span>
          已用 <b>{device.used}</b>
        </span>
        <span>
          标称 <b>{device.total}</b>
        </span>
        <span>
          空闲 <b>{device.free}</b>
        </span>
        <span>
          利用率{' '}
          <b>
            {device.ratio === null ? '—' : `${(device.ratio * 100).toFixed(0)}%`}
          </b>
        </span>
        {Object.entries(byRole).map(([role, n]) => (
          <span key={role} className="role-chip">
            {ROLE_LABEL[role] ?? role} {n}
          </span>
        ))}
      </div>

      <table className="ledger-port-table">
        <thead>
          <tr>
            <th>端口</th>
            <th>角色</th>
            <th>速率</th>
            <th>线缆</th>
            <th>对端设备</th>
            <th>对端端口</th>
          </tr>
        </thead>
        <tbody>
          {ports.map((p: LedgerPort) => (
            <tr key={p.port}>
              <td className="mono">{p.port}</td>
              <td>
                <span className={'role-tag role-' + p.role}>{ROLE_LABEL[p.role] ?? p.role}</span>
              </td>
              <td className="num">{p.speed === null ? '—' : `${p.speed / 1000}G`}</td>
              <td>{p.cable || '—'}</td>
              <td>{p.peer || '—'}</td>
              <td className="mono">{p.peerPort || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {device.freeBays && device.freeBays.length > 0 && (
        <p className="device-free">
          空闲口（{device.freeBays.length} 个）：
          {device.freeBays
            .slice(0, 24)
            .map(([group, panel, idx]) => `${group}${panel}/${idx}`)
            .join('、')}
          {device.freeBays.length > 24 && ` …… 另有 ${device.freeBays.length - 24} 个`}
        </p>
      )}
    </div>
  );
}

export function LedgerView(): JSX.Element {
  const [data, setData] = useState<LedgerPayload | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [detail, setDetail] = useState<LedgerDevice | null>(null);
  const [detailErr, setDetailErr] = useState<string | null>(null);
  const [filter, setFilter] = useState('');
  const [deviceQuery, setDeviceQuery] = useState('');

  useEffect(() => {
    api
      .ledger()
      .then(setData)
      .catch((e: unknown) => setErr(e instanceof Error ? e.message : String(e)));
  }, []);

  const openDevice = (name: string): void => {
    setDetailErr(null);
    api
      .ledgerDevice(name)
      .then((r) => setDetail(r.device))
      .catch((e: unknown) => setDetailErr(e instanceof Error ? e.message : String(e)));
  };

  const racks = useMemo(() => {
    if (!data) return [];
    const needle = filter.trim();
    const rows = data.byRack;
    if (!needle) return rows.slice(0, 60);
    return rows
      .filter(
        (r) =>
          `${r.channel}-${r.rack}`.includes(needle) ||
          String(r.rack) === needle ||
          String(r.channel) === needle,
      )
      .slice(0, 60);
  }, [data, filter]);

  if (err) {
    return (
      <div className="view-placeholder">
        台账加载失败：{err}
        <br />
        若库里还没有项目，先跑 `idcbox project-save --name V13 --intent projects/v13/intent_full.json`。
      </div>
    );
  }
  if (!data) return <div className="view-placeholder">加载中…</div>;

  const s = data.summary;

  return (
    <div className="ledger-view">
      <section className="panel ledger-summary">
        <div className="ledger-hero">
          <div className="ledger-big">
            <b>{fmtInt(s.used)}</b>
            <span>已用端口</span>
          </div>
          <div className="ledger-big">
            <b>{fmtInt(s.capacity)}</b>
            <span>标称容量</span>
          </div>
          <div className="ledger-big">
            <b>{fmtInt(s.free)}</b>
            <span>空闲端口</span>
          </div>
          <div className="ledger-big ledger-ratio-big">
            <b>{(s.ratio * 100).toFixed(1)}%</b>
            <span>整体利用率</span>
          </div>
        </div>
        <p className="panel-note">
          共 <b>{s.devices}</b> 台设备。分母只算有端口模板的设备 —— 型号没模板时标称端口数未知，
          算成 0 或猜一个会让利用率凭空变好看。
          {s.noSpecDevices > 0 && (
            <>
              {' '}
              当前有 <b>{s.noSpecDevices}</b> 台无模板，未计入分母。
            </>
          )}
        </p>
        <div className="ledger-search">
          <input
            type="text"
            placeholder="查一台设备还剩哪些口（如 业务核心交换机-1）"
            value={deviceQuery}
            onChange={(e) => setDeviceQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && deviceQuery.trim()) openDevice(deviceQuery.trim());
            }}
          />
          <button
            type="button"
            className="btn-mini"
            disabled={!deviceQuery.trim()}
            onClick={() => openDevice(deviceQuery.trim())}
          >
            查端口
          </button>
          <span className="panel-note">
            按实例名精确查。要看一整批型号的余量看下面的表，要看某台设备逐个口插了什么用这里。
          </span>
        </div>
      </section>

      {data.attention && data.attention.length > 0 && (
        <section className="panel ledger-attention">
          <h3>需要人看一眼（{data.attention.length} 条）</h3>
          <ul>
            {data.attention.map((a, i) => (
              <li key={i} className={'attention-' + a.severity}>
                <span className="attention-kind">{a.kind}</span>
                {a.device && <b>{a.device}</b>}
                <span>{a.detail}</span>
              </li>
            ))}
          </ul>
          <p className="panel-note">
            这三类问题都落在「正常范围」里，不会被任何断言拦住：无模板的利用率显示「—」看起来只是没数据、
            越界很容易被显示逻辑截成 100%、未知端口只影响一两台设备。但每一条都会导致现场按台账插线时撞车。
          </p>
        </section>
      )}

      {detailErr && <div className="panel ledger-error">打开设备失败：{detailErr}</div>}
      {detail && <DeviceDetail device={detail} onClose={() => setDetail(null)} />}

      <section className="panel">
        <h3>按型号 —— 这批设备还能再插多少台</h3>
        <table className="port-table">
          <thead>
            <tr>
              <th>型号</th>
              <th className="num">设备数</th>
              <th className="num">已用</th>
              <th className="num">标称</th>
              <th className="num">空闲</th>
              <th className="num">利用率</th>
              <th className="num">单台用量区间</th>
            </tr>
          </thead>
          <tbody>
            {data.byModel.map((m) => (
              <tr key={m.model}>
                <td>{m.model}</td>
                <td className="num">{m.devices}</td>
                <td className="num">{fmtInt(m.used)}</td>
                <td className="num">{fmtInt(m.capacity)}</td>
                <td className="num">{fmtInt(m.free)}</td>
                <td className="num">
                  {m.ratio === null ? (
                    <span className="ratio-na">—</span>
                  ) : (
                    <span className={'ratio-bar ' + ratioClass(m.ratio)}>
                      {(m.ratio * 100).toFixed(0)}%
                    </span>
                  )}
                </td>
                <td className="num">
                  {m.minUsed === m.maxUsed ? m.minUsed : `${m.minUsed} ~ ${m.maxUsed}`}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="panel">
        <h3>按机柜 —— 这个柜子还有没有余量</h3>
        <div className="ledger-filter">
          <input
            type="text"
            placeholder="筛机柜（如 1-5 或 12）"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <span className="panel-note">
            显示 {racks.length} / {data.byRack.length} 个机柜
          </span>
        </div>
        <table className="port-table">
          <thead>
            <tr>
              <th>机柜</th>
              <th className="num">设备数</th>
              <th className="num">已用</th>
              <th className="num">标称</th>
              <th className="num">空闲</th>
              <th className="num">利用率</th>
            </tr>
          </thead>
          <tbody>
            {racks.map((r) => {
              const label = `${r.channel}-${r.rack}`;
              return (
                <tr key={label}>
                  <td className="mono">通道 {r.channel} · 机柜 {r.rack}</td>
                  <td className="num">{r.devices}</td>
                  <td className="num">{fmtInt(r.used)}</td>
                  <td className="num">{fmtInt(r.capacity)}</td>
                  <td className="num">{fmtInt(r.free)}</td>
                  <td className="num">
                    {r.ratio === null ? (
                      <span className="ratio-na">—</span>
                    ) : (
                      <span className={'ratio-bar ' + ratioClass(r.ratio)}>
                        {(r.ratio * 100).toFixed(0)}%
                      </span>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </section>

      <section className="panel">
        <h3>按类别</h3>
        <div className="category-grid">
          {Object.entries(
            data.byModel.reduce<Record<string, { devices: number; used: number; capacity: number }>>(
              (acc, m) => {
                const cur = acc[m.category] ?? { devices: 0, used: 0, capacity: 0 };
                acc[m.category] = {
                  devices: cur.devices + m.devices,
                  used: cur.used + m.used,
                  capacity: cur.capacity + m.capacity,
                };
                return acc;
              },
              {},
            ),
          ).map(([cat, v]) => {
            const ratio = v.capacity ? v.used / v.capacity : null;
            return (
              <div key={cat} className="category-card">
                <span className="category-name">{CATEGORY_LABEL[cat] ?? cat}</span>
                <b className="category-ratio">{ratio === null ? '—' : `${(ratio * 100).toFixed(0)}%`}</b>
                <span className="category-detail">
                  {v.devices} 台 · {fmtInt(v.used)}/{fmtInt(v.capacity)} 口
                </span>
              </div>
            );
          })}
        </div>
      </section>

      <p className="panel-note ledger-foot">
        数据来自项目「{data.project}」的最新意图，按 `意图 → 生成连线 → 聚合端口` 现算，
        读取的不是上次保存的缓存。想看某台设备的每个口插了什么，用 CLI：
        <code>idcbox ledger --intent projects/v13/intent_full.json --device 业务核心交换机-1</code>
      </p>
    </div>
  );
}
