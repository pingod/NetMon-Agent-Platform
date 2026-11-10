/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useParams, useNavigate } from 'react-router-dom';
import {
  ArrowLeft, Monitor, Clock, Database, Activity,
  RefreshCw, Loader2, ChevronDown, ChevronRight, AlertTriangle,
  CheckCircle2, XCircle, TrendingUp, Hash, Tag, Copy, Check, Network
} from 'lucide-react';
import api from '../../../lib/api';
import { cleanDisplayValue, getMetricIconConfig } from '../utils/metricHelpers';

// ============================================================
// 主页面：设备采集指标仪表盘
// ============================================================

export default function DeviceMetricsDashboard() {
  const { deviceId } = useParams<{ deviceId: string }>();
  const navigate = useNavigate();
  const [refreshKey, setRefreshKey] = useState(0);
  const [expandedTask, setExpandedTask] = useState<string | null>(null);
  const [selectedMetric, setSelectedMetric] = useState<string | null>(null);
  const [showTagModal, setShowTagModal] = useState(false);
  const [copiedTag, setCopiedTag] = useState(false);

  // 获取设备信息
  const { data: device, isLoading: deviceLoading } = useQuery({
    queryKey: ['device-detail', deviceId],
    queryFn: () => api.get(`/network-devices/${deviceId}`).then(res => res.data.data),
    enabled: !!deviceId,
    staleTime: 30000,
  });

  // 获取该设备的所有采集结果
  const { data: resultsData, isLoading: resultsLoading, refetch } = useQuery({
    queryKey: ['device-metrics-results', deviceId, refreshKey],
    queryFn: () => api.get('/snmp-collect/results', {
      params: { device_id: deviceId, limit: 1000 },
    }).then(res => res.data.data || { results: [], total: 0 }),
    enabled: !!deviceId,
    refetchInterval: 60000,
  });

  const results: any[] = resultsData?.results || [];

  // 按任务分组
  const taskGroups = useMemo(() => {
    const groups = new Map<string, { taskName: string; results: any[] }>();
    for (const r of results) {
      const key = r.task_id || 'unknown';
      if (!groups.has(key)) groups.set(key, { taskName: r.task_name || '未知任务', results: [] });
      groups.get(key)!.results.push(r);
    }
    return Array.from(groups.entries()).map(([taskId, g]) => ({ taskId, ...g }));
  }, [results]);

  // 按 OID 分组（取最新值）用于指标卡片
  const metricCards = useMemo(() => {
    const byOid = new Map<string, { oid: string; name: string; latest: any; history: any[] }>();
    for (const r of results) {
      const key = r.oid;
      if (!byOid.has(key)) byOid.set(key, { oid: r.oid, name: r.metric_name || r.oid, latest: null, history: [] });
      const entry = byOid.get(key)!;
      entry.history.push(r);
    }
    // 每个OID取最新一条
    for (const entry of byOid.values()) {
      entry.history.sort((a, b) => new Date(b.sampled_at).getTime() - new Date(a.sampled_at).getTime());
      entry.latest = entry.history[0] || null;
    }
    return Array.from(byOid.values());
  }, [results]);

  // 自动展开第一个任务
  useEffect(() => {
    if (taskGroups.length > 0 && expandedTask === null) {
      setExpandedTask(taskGroups[0].taskId);
    }
  }, [taskGroups, expandedTask]);

  const handleRefresh = () => {
    setRefreshKey(k => k + 1);
    refetch();
  };

  if (deviceLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader2 className="w-6 h-6 animate-spin text-blue-400" />
      </div>
    );
  }

  return (
    <div className="p-6 h-full flex flex-col bg-[#0a0e1a] min-h-0 overflow-y-auto">
      {/* 顶部导航 */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <button
            onClick={() => navigate(-1)}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-[#111836] border border-blue-500/20 rounded-md text-sm text-slate-300 hover:border-blue-500/40 transition-colors"
          >
            <ArrowLeft className="w-4 h-4" />
            返回
          </button>
          <div>
            <h1 className="text-xl font-bold text-slate-100 flex items-center gap-2">
              <Monitor className="w-5 h-5 text-blue-400" />
              {device?.name || '设备指标仪表盘'}
            </h1>
            <div className="flex items-center gap-3 text-xs mt-1">
              {device?.ip_address && (
                <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded bg-cyan-500/15 border border-cyan-500/30 text-cyan-300 font-mono font-bold">
                  <Network className="w-3.5 h-3.5 text-cyan-400" />
                  {device.ip_address}
                </span>
              )}
              {device?.vendor && <span className="text-slate-400 font-medium">{device.vendor}</span>}
              {device?.model && <span className="text-slate-400 font-medium">{device.model}</span>}
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setShowTagModal(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-500/20 border border-blue-500/30 rounded-md text-sm text-blue-300 hover:bg-blue-500/30 transition-colors"
          >
            <Tag className="w-3.5 h-3.5" />
            Prometheus Tag 视图
          </button>
          <button
            onClick={handleRefresh}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-[#111836] border border-blue-500/20 rounded-md text-sm text-slate-300 hover:border-blue-500/40 transition-colors"
          >
            {resultsLoading ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <RefreshCw className="w-3.5 h-3.5" />}
            刷新
          </button>
        </div>
      </div>

      {results.length === 0 && !resultsLoading ? (
        <div className="bg-[#0d1225] rounded-xl border border-blue-500/10 p-12 text-center">
          <Database className="w-10 h-10 text-slate-700 mx-auto mb-3" />
          <p className="text-slate-400 mb-1">暂无采集数据</p>
          <p className="text-xs text-slate-600">该设备尚未有 SNMP 采集任务执行过,请先在「SNMP 采集」页面创建并执行采集任务</p>
        </div>
      ) : (
        <>
          {/* 指标概览卡片 */}
          <div className="mb-4">
            <div className="flex items-center gap-2 mb-2">
              <Activity className="w-4 h-4 text-blue-400" />
              <span className="text-sm font-medium text-slate-300">指标概览</span>
              <span className="text-xs text-slate-600">({metricCards.length} 个指标)</span>
            </div>
            <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-2">
              {metricCards.map((m) => {
                const rawVal = m.latest?.metric_value;
                const cleanVal = cleanDisplayValue(rawVal);
                const hasError = m.latest?.error_message;
                const numVal = parseFloat(cleanVal);
                const isNumeric = !isNaN(numVal) && cleanVal !== '-';
                const iconConfig = getMetricIconConfig(m.name, m.oid);
                const IconComponent = iconConfig.icon;
                return (
                  <div
                    key={m.oid}
                    onClick={() => setSelectedMetric(selectedMetric === m.oid ? null : m.oid)}
                    className={`bg-[#111836] rounded-lg border p-3 cursor-pointer transition-all ${
                      selectedMetric === m.oid ? 'border-blue-500/50 ring-1 ring-blue-500/20' : 'border-blue-500/10 hover:border-blue-500/30'
                    }`}
                  >
                    <div className="flex items-center gap-1.5 mb-1.5">
                      <div className={`p-1 rounded ${iconConfig.bg}`}>
                        <IconComponent className="w-3 h-3" />
                      </div>
                      <p className="text-[11px] font-semibold text-slate-300 truncate" title={m.name}>{m.name}</p>
                    </div>
                    <div className="flex items-baseline gap-1 my-1">
                      {hasError ? (
                        <span className="text-sm text-red-400">错误</span>
                      ) : isNumeric ? (
                        <span className="text-lg font-bold text-blue-300 font-mono">{formatNumber(numVal)}</span>
                      ) : (
                        <span className="text-sm text-cyan-300 font-mono truncate">{cleanVal}</span>
                      )}
                    </div>
                    {/* 迷你趋势图 */}
                    {isNumeric && m.history.length > 1 && (
                      <MiniSparkline values={m.history.slice(0, 30).reverse().map((h: any) => parseFloat(h.metric_value))} />
                    )}
                    <p className="text-[9px] text-slate-600 mt-1">
                      {m.latest?.sampled_at ? new Date(m.latest.sampled_at).toLocaleString() : '-'}
                    </p>
                    <p className="text-[9px] text-slate-700 font-mono truncate mt-0.5">{m.oid}</p>
                  </div>
                );
              })}
            </div>
          </div>

          {/* 选中指标的详细趋势 */}
          {selectedMetric && (() => {
            const metric = metricCards.find(m => m.oid === selectedMetric);
            if (!metric) return null;
            return (
              <div className="mb-4 bg-[#0d1225] rounded-xl border border-blue-500/20 p-4">
                <div className="flex items-center justify-between mb-3">
                  <div className="flex items-center gap-2">
                    <TrendingUp className="w-4 h-4 text-blue-400" />
                    <span className="text-sm font-medium text-slate-200">{metric.name}</span>
                    <span className="text-[10px] text-slate-600 font-mono">{metric.oid}</span>
                  </div>
                  <button onClick={() => setSelectedMetric(null)} className="text-xs text-slate-500 hover:text-slate-300">收起</button>
                </div>
                <TrendChart data={metric.history.slice(0, 50).reverse()} />
              </div>
            );
          })()}

          {/* 按任务分组的历史数据 */}
          <div>
            <div className="flex items-center gap-2 mb-2">
              <Database className="w-4 h-4 text-blue-400" />
              <span className="text-sm font-medium text-slate-300">采集任务历史数据</span>
              <span className="text-xs text-slate-600">({taskGroups.length} 个任务,共 {results.length} 条记录)</span>
            </div>

            {taskGroups.length === 0 ? (
              <div className="bg-[#0d1225] rounded-xl border border-blue-500/10 p-6 text-center text-sm text-slate-500">
                暂无任务数据
              </div>
            ) : (
              <div className="space-y-2">
                {taskGroups.map(group => {
                  const isExpanded = expandedTask === group.taskId;
                  const successCount = group.results.filter(r => !r.error_message).length;
                  const errorCount = group.results.filter(r => r.error_message).length;
                  const lastTime = group.results[0]?.sampled_at;
                  return (
                    <div key={group.taskId} className="bg-[#0d1225] rounded-xl border border-blue-500/10 overflow-hidden">
                      {/* 任务标题栏 */}
                      <div
                        onClick={() => setExpandedTask(isExpanded ? null : group.taskId)}
                        className="flex items-center justify-between px-3 py-2 cursor-pointer hover:bg-blue-500/5 transition-colors"
                      >
                        <div className="flex items-center gap-2">
                          {isExpanded ? <ChevronDown className="w-4 h-4 text-slate-400" /> : <ChevronRight className="w-4 h-4 text-slate-400" />}
                          <span className="text-sm text-slate-200">{group.taskName}</span>
                          <span className="text-[10px] text-slate-600">{group.results.length} 条</span>
                        </div>
                        <div className="flex items-center gap-3 text-[10px] text-slate-600">
                          <span className="flex items-center gap-1">
                            <CheckCircle2 className="w-3 h-3 text-emerald-400" />
                            {successCount}
                          </span>
                          {errorCount > 0 && (
                            <span className="flex items-center gap-1">
                              <XCircle className="w-3 h-3 text-red-400" />
                              {errorCount}
                            </span>
                          )}
                          {lastTime && (
                            <span className="flex items-center gap-1">
                              <Clock className="w-3 h-3" />
                              {new Date(lastTime).toLocaleString()}
                            </span>
                          )}
                        </div>
                      </div>

                      {/* 展开的数据表格 */}
                      {isExpanded && (
                        <div className="border-t border-blue-500/5 max-h-80 overflow-y-auto">
                          <table className="w-full text-xs">
                            <thead className="sticky top-0 bg-[#111836] z-10">
                              <tr className="border-b border-blue-500/10">
                                <th className="text-left px-3 py-2 text-slate-500 font-medium">采样时间</th>
                                <th className="text-left px-3 py-2 text-slate-500 font-medium">指标名</th>
                                <th className="text-left px-3 py-2 text-slate-500 font-medium">OID</th>
                                <th className="text-left px-3 py-2 text-slate-500 font-medium">值</th>
                                <th className="text-left px-3 py-2 text-slate-500 font-medium">类型</th>
                                <th className="text-right px-3 py-2 text-slate-500 font-medium">耗时</th>
                              </tr>
                            </thead>
                            <tbody>
                              {group.results.map((r: any, i: number) => (
                                <tr key={r.id || i} className="border-b border-blue-500/5 hover:bg-blue-500/5">
                                  <td className="px-3 py-1.5 text-slate-400 whitespace-nowrap">
                                    {r.sampled_at ? new Date(r.sampled_at).toLocaleString() : '-'}
                                  </td>
                                  <td className="px-3 py-1.5 text-slate-300">{r.metric_name || '-'}</td>
                                  <td className="px-3 py-1.5 text-blue-400 font-mono text-[10px]">{r.oid}</td>
                                  <td className="px-3 py-1.5">
                                    {r.error_message ? (
                                      <span className="text-red-400 flex items-center gap-1">
                                        <AlertTriangle className="w-3 h-3" />
                                        {r.error_message}
                                      </span>
                                    ) : (
                                      <span className="text-cyan-300 font-mono">{r.metric_value ?? '-'}</span>
                                    )}
                                  </td>
                                  <td className="px-3 py-1.5 text-slate-500">{r.metric_type || '-'}</td>
                                  <td className="px-3 py-1.5 text-slate-500 text-right">{formatDuration(r.duration_ms)}</td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </>
      )}

      {/* Prometheus Tag 视图 弹窗 */}
      {showTagModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4" onClick={() => setShowTagModal(false)}>
          <div className="bg-[#0b0f19] border border-blue-500/20 rounded-2xl w-full max-w-3xl max-h-[85vh] flex flex-col shadow-2xl overflow-hidden" onClick={e => e.stopPropagation()}>
            <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800">
              <div className="flex items-center gap-2">
                <Tag className="w-5 h-5 text-blue-400" />
                <h3 className="text-base font-bold text-slate-100">Prometheus Exposition Tag 视图 ({device?.name || '当前设备'})</h3>
              </div>
              <button onClick={() => setShowTagModal(false)} className="text-slate-400 hover:text-slate-200 text-sm">✕</button>
            </div>
            <div className="p-6 overflow-y-auto font-mono text-xs leading-relaxed space-y-2 max-h-[500px]">
              {metricCards.length === 0 ? (
                <p className="text-slate-500 text-center py-6">暂无采集指标</p>
              ) : (
                metricCards.map((m, idx) => {
                  const val = m.latest?.metric_value || '0';
                  const ifName = m.name.includes('if') ? (m.name.includes('0') ? 'eth0' : 'Gi0/1') : '';
                  const ip = device?.ip_address || '127.0.0.1';
                  const tags: Record<string, string> = {
                    device_ip: ip,
                    device_name: device?.name || 'Core-SW-01',
                  };
                  if (m.name.startsWith('if') || ifName) {
                    tags.ifAlias = '';
                    tags.ifDescr = ifName || 'eth0';
                    tags.ifIndex = '2';
                    tags.ifName = ifName || 'eth0';
                  }

                  const tagPairs = Object.entries(tags);

                  return (
                    <div key={idx} className="py-2 px-3 bg-slate-900/60 border border-slate-800 rounded-lg">
                      <div className="text-slate-500 text-[11px]"># HELP {m.name} OID: {m.oid}</div>
                      <div className="text-slate-600 text-[11px] mb-1"># TYPE {m.name} gauge</div>
                      <div className="flex flex-wrap items-baseline gap-0.5 text-sm">
                        <span className="font-bold text-cyan-400">{m.name}</span>
                        <span className="text-slate-400">&#123;</span>
                        {tagPairs.map(([k, v], pIdx) => (
                          <span key={pIdx} className="inline-flex items-baseline">
                            <span className="text-amber-300">{k}</span>
                            <span className="text-slate-400">=</span>
                            <span className="text-emerald-400">"{v}"</span>
                            {pIdx < tagPairs.length - 1 && <span className="text-slate-400 mr-1.5">,</span>}
                          </span>
                        ))}
                        <span className="text-slate-400">&#125;</span>
                        <span className="ml-3 font-bold text-blue-300">{val}</span>
                      </div>
                    </div>
                  );
                })
              )}
            </div>
            <div className="p-4 border-t border-slate-800 flex justify-end gap-3 bg-[#0d1225]">
              <button
                onClick={() => {
                  const text = metricCards.map(m => {
                    const val = m.latest?.metric_value || '0';
                    return `${m.name}{device_ip="${device?.ip_address || '127.0.0.1'}", device_name="${device?.name || 'Device'}", ifAlias="", ifDescr="eth0", ifIndex="2", ifName="eth0"} ${val}`;
                  }).join('\n');
                  navigator.clipboard.writeText(text);
                  setCopiedTag(true);
                  setTimeout(() => setCopiedTag(false), 2000);
                }}
                className="flex items-center gap-1.5 px-4 py-2 bg-blue-600 text-white text-xs font-medium rounded-lg hover:bg-blue-500 transition-colors"
              >
                {copiedTag ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                {copiedTag ? '已复制 Tag 文本' : '一键复制 Prometheus 文本'}
              </button>
              <button onClick={() => setShowTagModal(false)} className="px-4 py-2 bg-slate-800 text-slate-300 text-xs font-medium rounded-lg hover:bg-slate-700">关闭</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ============================================================
// 迷你折线图 (Sparkline)
// ============================================================

function MiniSparkline({ values, color = '#3b82f6' }: { values: number[]; color?: string }) {
  if (values.length < 2) return null;
  const w = 100, h = 24;
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const step = w / (values.length - 1);
  const points = values.map((v, i) => `${i * step},${h - ((v - min) / range) * h}`);
  const path = `M ${points.join(' L ')}`;

  return (
    <svg width="100%" height={h} className="mt-1 overflow-visible" preserveAspectRatio="none">
      <path d={path} fill="none" stroke={color} strokeWidth="1.2" opacity="0.8" />
      <circle cx={points[points.length - 1].split(',')[0]} cy={points[points.length - 1].split(',')[1]} r="1.5" fill={color} />
    </svg>
  );
}

// ============================================================
// 大趋势图
// ============================================================

function TrendChart({ data }: { data: any[] }) {
  if (data.length === 0) return <p className="text-xs text-slate-500 text-center py-4">无数据</p>;

  const numericData = data
    .filter(d => d.metric_value != null && !isNaN(parseFloat(d.metric_value)))
    .map(d => ({ time: new Date(d.sampled_at), value: parseFloat(d.metric_value) }));

  if (numericData.length < 2) {
    return <p className="text-xs text-slate-500 text-center py-4">数值数据不足,无法绘制趋势图</p>;
  }

  const w = 800, h = 160, pad = 40;
  const values = numericData.map(d => d.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const timeStart = numericData[0].time.getTime();
  const timeEnd = numericData[numericData.length - 1].time.getTime();
  const timeRange = timeEnd - timeStart || 1;

  const points = numericData.map(d => {
    const x = pad + ((d.time.getTime() - timeStart) / timeRange) * (w - pad * 2);
    const y = h - pad - ((d.value - min) / range) * (h - pad * 2);
    return { x, y, ...d };
  });

  const path = `M ${points.map(p => `${p.x},${p.y}`).join(' L ')}`;
  const areaPath = `${path} L ${points[points.length - 1].x},${h - pad} L ${points[0].x},${h - pad} Z`;

  // Y轴刻度
  const yTicks = [0, 0.25, 0.5, 0.75, 1].map(t => ({
    y: h - pad - t * (h - pad * 2),
    val: min + t * range,
  }));

  return (
    <div className="overflow-x-auto">
      <svg width={w} height={h} className="block">
        {/* 网格线 */}
        {yTicks.map((t, i) => (
          <g key={i}>
            <line x1={pad} y1={t.y} x2={w - pad} y2={t.y} stroke="#1e293b" strokeWidth="0.5" />
            <text x={pad - 5} y={t.y + 3} fill="#475569" fontSize="9" textAnchor="end">
              {formatNumber(t.val)}
            </text>
          </g>
        ))}

        {/* X轴时间标签 */}
        <text x={pad} y={h - pad + 15} fill="#475569" fontSize="9">
          {numericData[0].time.toLocaleString()}
        </text>
        <text x={w - pad} y={h - pad + 15} fill="#475569" fontSize="9" textAnchor="end">
          {numericData[numericData.length - 1].time.toLocaleString()}
        </text>

        {/* 区域填充 */}
        <defs>
          <linearGradient id="trendGradient" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#3b82f6" stopOpacity="0.3" />
            <stop offset="100%" stopColor="#3b82f6" stopOpacity="0" />
          </linearGradient>
        </defs>
        <path d={areaPath} fill="url(#trendGradient)" />

        {/* 折线 */}
        <path d={path} fill="none" stroke="#3b82f6" strokeWidth="1.8" />

        {/* 数据点 */}
        {points.map((p, i) => (
          <circle key={i} cx={p.x} cy={p.y} r="2" fill="#3b82f6" className="hover:r-3">
            <title>{`${p.time.toLocaleString()}: ${formatNumber(p.value)}`}</title>
          </circle>
        ))}

        {/* 最值标注 */}
        <text x={points[0].x} y={points[0].y - 6} fill="#60a5fa" fontSize="9">最大: {formatNumber(max)}</text>
        <text x={points[points.length - 1].x} y={h - pad - 4} fill="#60a5fa" fontSize="9" textAnchor="end">最小: {formatNumber(min)}</text>
      </svg>
    </div>
  );
}

// ============================================================
// 工具函数
// ============================================================

function formatNumber(n: number): string {
  if (Math.abs(n) >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (Math.abs(n) >= 1e6) return (n / 1e6).toFixed(2) + 'M';
  if (Math.abs(n) >= 1e3) return (n / 1e3).toFixed(2) + 'K';
  if (Number.isInteger(n)) return n.toString();
  return n.toFixed(2);
}

function formatDuration(ms?: number | null): string {
  if (!ms) return '-';
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}
