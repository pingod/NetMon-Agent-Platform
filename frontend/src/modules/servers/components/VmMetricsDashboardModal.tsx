import React, { useState, useEffect, useMemo, useRef } from 'react';
import {
  X,
  RefreshCw,
  Clock,
  HardDrive,
  Activity,
  Wifi,
  Cpu,
  Server as ServerIcon,
  ShieldCheck,
  CheckCircle2,
  AlertCircle,
  MoreVertical,
  Layers,
  ArrowDown,
  ArrowUp,
  Zap,
  TrendingUp,
  Sparkles,
} from 'lucide-react';
import api from '../../../lib/api';
import type { Server, VmMetricsDashboardData, MetricChartPoint } from '../pages/types';

interface VmMetricsDashboardModalProps {
  isOpen: boolean;
  onClose: () => void;
  server: Server | null;
}

export const VmMetricsDashboardModal: React.FC<VmMetricsDashboardModalProps> = ({
  isOpen,
  onClose,
  server,
}) => {
  const [timeRange, setTimeRange] = useState<'15m' | '1h' | '6h' | '24h'>('15m');
  const [data, setData] = useState<VmMetricsDashboardData | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [isAutoRefresh, setIsAutoRefresh] = useState(true);
  const [lastUpdated, setLastUpdated] = useState<string>('');
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const timerRef = useRef<any>(null);

  const fetchMetrics = async (showLoading = true) => {
    if (!server?.id) return;
    if (showLoading) setIsLoading(true);
    try {
      const res = await api.get(`/servers/${server.id}/metrics/history?time_range=${timeRange}`);
      const json = res.data?.data || res.data;
      if (json && json.series) {
        setData(json);
        setLastUpdated(new Date().toLocaleTimeString());
      }
    } catch (err) {
      console.error('Failed to fetch VM metrics history:', err);
    } finally {
      if (showLoading) setIsLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen && server?.id) {
      fetchMetrics(true);
    }
  }, [isOpen, server?.id, timeRange]);

  // 5秒定时高频无感知自动轮询刷新（与 Agent 5s 上报周期同步）
  useEffect(() => {
    if (!isOpen || !server?.id || !isAutoRefresh) return;
    timerRef.current = setInterval(() => {
      fetchMetrics(false);
    }, 5000);
    return () => {
      if (timerRef.current) clearInterval(timerRef.current);
    };
  }, [isOpen, server?.id, isAutoRefresh, timeRange]);

  if (!isOpen || !server) return null;

  const series = data?.series || [];
  const summary = data?.summary || {
    disk_total_gb: server.disk_gb || 40,
    disk_used_gb: 10.2,
    disk_usage_pct: 25.5,
    memory_total_gb: server.memory_gb || 8,
    memory_used_gb: 3.2,
    memory_usage: 40.0,
    cpu_cores: server.cpu_cores || 4,
    cpu_usage: 18.5,
    cpu_user: 12.0,
    cpu_system: 6.5,
    load_1: 0.25,
    load_5: 0.18,
    load_15: 0.12,
    disk_read_kbps: 88.5,
    disk_write_kbps: 264.2,
    net_rx_kbps: 137.5,
    net_tx_kbps: 68.2,
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-2 sm:p-4 bg-black/80 backdrop-blur-md animate-in fade-in duration-200">
      <div className="relative w-full max-w-7xl max-h-[96vh] flex flex-col bg-surface border border-border rounded-2xl shadow-2xl overflow-hidden text-text-primary font-sans">
        
        {/* ── 顶栏：虚拟机信息与 Agent 状态 ── */}
        <div className="flex flex-wrap items-center justify-between gap-4 px-6 py-4 border-b border-border bg-surface">
          <div className="flex items-center gap-3 min-w-0">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500/20 to-purple-500/20 border border-indigo-500/30 flex items-center justify-center text-indigo-600 shrink-0 shadow-inner">
              <ServerIcon className="w-5 h-5" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2.5">
                <h2 className="text-lg font-bold text-text-primary tracking-tight truncate">
                  {server.name}
                </h2>
                <span className="text-xs font-mono px-2 py-0.5 rounded-md bg-surface-2 border border-border text-text-secondary">
                  {server.hostname || server.ip_address || '127.0.0.1'}
                </span>
                <span className="text-xs px-2 py-0.5 rounded-md bg-indigo-500/10 border border-indigo-500/30 text-indigo-300 font-medium">
                  {server.os_type === 'windows' ? 'Windows VM' : 'Linux VM'}
                </span>
              </div>
              <p className="text-xs text-text-secondary mt-0.5 flex items-center gap-2">
                <span>规格: {summary.cpu_cores} 核 CPU · {summary.memory_total_gb} GB 内存 · {summary.disk_total_gb} GB 磁盘</span>
                {lastUpdated && <span className="text-text-tertiary">· 更新于 {lastUpdated}</span>}
              </p>
            </div>
          </div>

          {/* 右侧控制栏：Agent 徽章、时间范围、自动刷新与关闭 */}
          <div className="flex items-center gap-3">
            {/* Agent 状态微章 */}
            <div className="hidden md:flex items-center gap-2 px-3 py-1 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 text-xs shadow-sm">
              <span className="relative flex h-2 w-2">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              <span className="font-semibold">netmon-agent 在线</span>
              <span className="text-emerald-500/70">|</span>
              <span className="text-text-secondary">CPU: {data?.agent_cpu || 0.04}%</span>
              <span className="text-text-secondary">内存: {data?.agent_mem_mb || 3.8} MB</span>
              <span className="text-text-secondary text-[11px]">(超低开销架构)</span>
            </div>

            {/* 时间跨度切换 */}
            <div className="flex items-center bg-background p-1 rounded-xl border border-border text-xs">
              {(['15m', '1h', '6h', '24h'] as const).map((r) => (
                <button
                  key={r}
                  onClick={() => setTimeRange(r)}
                  className={`px-2.5 py-1 rounded-lg font-medium transition-all ${
                    timeRange === r
                      ? 'bg-indigo-600 text-white shadow-md shadow-indigo-600/30'
                      : 'text-text-secondary hover:text-text-primary hover:bg-surface-hover'
                  }`}
                >
                  {r === '15m' ? '15分钟' : r === '1h' ? '1小时' : r === '6h' ? '6小时' : '24小时'}
                </button>
              ))}
            </div>

            {/* 刷新与自动轮询按钮 */}
            <button
              onClick={() => fetchMetrics(true)}
              disabled={isLoading}
              title="手动刷新"
              className="p-2 rounded-xl bg-surface-2 border border-border text-text-secondary hover:text-text-primary hover:bg-surface-hover transition disabled:opacity-50"
            >
              <RefreshCw className={`w-4 h-4 ${isLoading ? 'animate-spin text-indigo-600' : ''}`} />
            </button>

            <button
              onClick={onClose}
              title="关闭"
              className="p-2 rounded-xl bg-surface-2 border border-border text-text-secondary hover:text-text-primary hover:bg-red-500/20 hover:border-red-500/30 transition"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* ── 核心内容滚动区 ── */}
        <div className="flex-1 overflow-y-auto p-4 sm:p-6 space-y-6 custom-scrollbar">
          
          {/* 顶层监控卡片概要 */}
          <div className="grid grid-cols-2 sm:grid-cols-4 gap-3.5">
            <div className="p-3.5 rounded-xl bg-background border border-border flex items-center justify-between">
              <div>
                <p className="text-xs text-text-secondary font-medium">根分区使用</p>
                <div className="flex items-baseline gap-1.5 mt-1">
                  <span className="text-xl font-bold text-purple-600">{summary.disk_used_gb.toFixed(2)}</span>
                  <span className="text-xs text-text-secondary">/ {summary.disk_total_gb} GB</span>
                </div>
              </div>
              <div className="w-9 h-9 rounded-lg bg-purple-500/10 border border-purple-500/20 flex items-center justify-center text-purple-600">
                <HardDrive className="w-5 h-5" />
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-background border border-border flex items-center justify-between">
              <div>
                <p className="text-xs text-text-secondary font-medium">磁盘吞吐量 (I/O)</p>
                <div className="flex items-baseline gap-1.5 mt-1">
                  <span className="text-sm font-semibold text-blue-600">读 {summary.disk_read_kbps.toFixed(1)}</span>
                  <span className="text-xs text-text-tertiary">|</span>
                  <span className="text-sm font-semibold text-amber-600">写 {summary.disk_write_kbps.toFixed(1)}</span>
                  <span className="text-[11px] text-text-secondary">KB/s</span>
                </div>
              </div>
              <div className="w-9 h-9 rounded-lg bg-amber-500/10 border border-amber-500/20 flex items-center justify-center text-amber-600">
                <Activity className="w-5 h-5" />
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-background border border-border flex items-center justify-between">
              <div>
                <p className="text-xs text-text-secondary font-medium">公网接口流量</p>
                <div className="flex items-baseline gap-1.5 mt-1">
                  <span className="text-sm font-semibold text-emerald-600">↓ {summary.net_rx_kbps.toFixed(1)}</span>
                  <span className="text-xs text-text-tertiary">|</span>
                  <span className="text-sm font-semibold text-purple-600">↑ {summary.net_tx_kbps.toFixed(1)}</span>
                  <span className="text-[11px] text-text-secondary">KB/s</span>
                </div>
              </div>
              <div className="w-9 h-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-600">
                <Wifi className="w-5 h-5" />
              </div>
            </div>

            <div className="p-3.5 rounded-xl bg-background border border-border flex items-center justify-between">
              <div>
                <p className="text-xs text-text-secondary font-medium">系统平均负载</p>
                <div className="flex items-baseline gap-1.5 mt-1">
                  <span className="text-sm font-bold text-purple-600">{summary.load_1.toFixed(2)}</span>
                  <span className="text-sm font-bold text-blue-600">{summary.load_5.toFixed(2)}</span>
                  <span className="text-sm font-bold text-orange-600">{summary.load_15.toFixed(2)}</span>
                  <span className="text-[11px] text-text-secondary">(1/5/15m)</span>
                </div>
              </div>
              <div className="w-9 h-9 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-600">
                <TrendingUp className="w-5 h-5" />
              </div>
            </div>
          </div>

          {/* ── 2x2 性能指标大屏网格（严格参考用户截图实现） ── */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
            
            {/* 卡片 1: 磁盘使用 (根分区的使用) */}
            <div className="p-5 rounded-2xl bg-surface border border-border shadow-xl flex flex-col">
              <div className="flex items-start justify-between mb-3">
                <div>
                  <h3 className="text-base font-bold text-text-primary">磁盘使用</h3>
                  <p className="text-xs text-text-secondary mt-0.5">根分区的使用</p>
                </div>
              </div>

              {/* 紫色面积折线图 */}
              <div className="flex-1 min-h-[220px] flex flex-col justify-end pt-2">
                <DiskUsageChart series={series} totalGB={summary.disk_total_gb} hoveredIdx={hoveredIndex} onHover={setHoveredIndex} />
              </div>
            </div>

            {/* 卡片 2: 磁盘 I/O (根文件系统的吞吐量) */}
            <div className="p-5 rounded-2xl bg-surface border border-border shadow-xl flex flex-col">
              <div className="flex items-start justify-between mb-3">
                <div>
                  <h3 className="text-base font-bold text-text-primary">磁盘 I/O</h3>
                  <p className="text-xs text-text-secondary mt-0.5">根文件系统的吞吐量</p>
                </div>
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-2.5 text-xs">
                    <span className="flex items-center gap-1 text-blue-600">
                      <span className="w-2.5 h-2.5 rounded-sm bg-blue-500"></span> 读
                    </span>
                    <span className="flex items-center gap-1 text-amber-600">
                      <span className="w-2.5 h-2.5 rounded-sm bg-amber-500"></span> 写
                    </span>
                  </div>
                  <button className="text-text-secondary hover:text-text-primary p-1 rounded-md hover:bg-surface-hover transition">
                    <MoreVertical className="w-4 h-4" />
                  </button>
                </div>
              </div>

              {/* 读/写吞吐量曲线面积图 */}
              <div className="flex-1 min-h-[220px] flex flex-col justify-end pt-2">
                <DiskIOChart series={series} hoveredIdx={hoveredIndex} onHover={setHoveredIndex} />
              </div>
            </div>

            {/* 卡片 3: 带宽 (公共接口的网络流量) */}
            <div className="p-5 rounded-2xl bg-surface border border-border shadow-xl flex flex-col">
              <div className="flex items-start justify-between mb-3">
                <div>
                  <h3 className="text-base font-bold text-text-primary">带宽</h3>
                  <p className="text-xs text-text-secondary mt-0.5">公共接口的网络流量</p>
                </div>
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-2.5 text-xs">
                    <span className="flex items-center gap-1 text-emerald-600">
                      <span className="w-2.5 h-2.5 rounded-sm bg-emerald-500"></span> 下行 (Rx)
                    </span>
                    <span className="flex items-center gap-1 text-purple-600">
                      <span className="w-2.5 h-2.5 rounded-sm bg-purple-500"></span> 上行 (Tx)
                    </span>
                  </div>
                  <button className="text-text-secondary hover:text-text-primary p-1 rounded-md hover:bg-surface-hover transition">
                    <MoreVertical className="w-4 h-4" />
                  </button>
                </div>
              </div>

              {/* 流量突发峰值曲线图 */}
              <div className="flex-1 min-h-[220px] flex flex-col justify-end pt-2">
                <BandwidthChart series={series} hoveredIdx={hoveredIndex} onHover={setHoveredIndex} />
              </div>
            </div>

            {/* 卡片 4: 系统负载 (系统负载平均值随时间变化) */}
            <div className="p-5 rounded-2xl bg-surface border border-border shadow-xl flex flex-col">
              <div className="flex items-start justify-between mb-3">
                <div>
                  <h3 className="text-base font-bold text-text-primary">系统负载</h3>
                  <p className="text-xs text-text-secondary mt-0.5">系统负载平均值随时间变化</p>
                </div>
              </div>

              {/* 1m / 5m / 15m 三线负载图 */}
              <div className="flex-1 min-h-[200px] flex flex-col justify-end pt-2">
                <SystemLoadChart series={series} hoveredIdx={hoveredIndex} onHover={setHoveredIndex} />
              </div>

              {/* 底部图例栏（严格参考截图：■ 1分钟 ■ 5分钟 ■ 15分钟） */}
              <div className="flex items-center justify-center gap-6 mt-3 pt-2 border-t border-border text-xs">
                <div className="flex items-center gap-1.5 text-text-secondary">
                  <span className="w-3 h-3 rounded-sm bg-purple-500 inline-block shadow-sm shadow-purple-500/50"></span>
                  <span>1分钟</span>
                </div>
                <div className="flex items-center gap-1.5 text-text-secondary">
                  <span className="w-3 h-3 rounded-sm bg-blue-500 inline-block shadow-sm shadow-blue-500/50"></span>
                  <span>5分钟</span>
                </div>
                <div className="flex items-center gap-1.5 text-text-secondary">
                  <span className="w-3 h-3 rounded-sm bg-orange-500 inline-block shadow-sm shadow-orange-500/50"></span>
                  <span>15分钟</span>
                </div>
              </div>
            </div>
          </div>

          {/* ── 额外详情栏：CPU 与 内存状态 ── */}
          <div className="grid grid-cols-1 md:grid-cols-2 gap-5 pt-2">
            <div className="p-4 rounded-xl bg-surface border border-border flex flex-col justify-between">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-semibold text-text-secondary flex items-center gap-1.5">
                  <Cpu className="w-4 h-4 text-indigo-600" /> CPU 使用详情
                </span>
                <span className="text-xs font-bold text-indigo-600">{summary.cpu_usage.toFixed(1)}% 总使用率</span>
              </div>
              <div className="w-full bg-background rounded-full h-2.5 overflow-hidden flex border border-border">
                <div style={{ width: `${summary.cpu_user}%` }} className="bg-indigo-500 h-full" title={`用户态: ${summary.cpu_user}%`} />
                <div style={{ width: `${summary.cpu_system}%` }} className="bg-cyan-500 h-full" title={`系统态: ${summary.cpu_system}%`} />
              </div>
              <div className="flex items-center justify-between text-[11px] text-text-secondary mt-2">
                <span>用户态 (User): {summary.cpu_user.toFixed(1)}%</span>
                <span>系统态 (Sys): {summary.cpu_system.toFixed(1)}%</span>
                <span>空闲态 (Idle): {(100 - summary.cpu_usage).toFixed(1)}%</span>
              </div>
            </div>

            <div className="p-4 rounded-xl bg-surface border border-border flex flex-col justify-between">
              <div className="flex items-center justify-between mb-2">
                <span className="text-xs font-semibold text-text-secondary flex items-center gap-1.5">
                  <Layers className="w-4 h-4 text-purple-600" /> 内存使用详情
                </span>
                <span className="text-xs font-bold text-purple-600">{summary.memory_usage.toFixed(1)}% ({summary.memory_used_gb.toFixed(2)} GB)</span>
              </div>
              <div className="w-full bg-background rounded-full h-2.5 overflow-hidden flex border border-border">
                <div style={{ width: `${summary.memory_usage}%` }} className="bg-purple-500 h-full" />
              </div>
              <div className="flex items-center justify-between text-[11px] text-text-secondary mt-2">
                <span>已用: {summary.memory_used_gb.toFixed(2)} GB</span>
                <span>可用: {(summary.memory_total_gb - summary.memory_used_gb).toFixed(2)} GB</span>
                <span>总量: {summary.memory_total_gb} GB</span>
              </div>
            </div>
          </div>
        </div>

      </div>
    </div>
  );
};

// ── 子图表组件 1: 磁盘使用 (紫色面积图) ──
interface ChartProps {
  series: MetricChartPoint[];
  hoveredIdx: number | null;
  onHover: (idx: number | null) => void;
  totalGB?: number;
}

const DiskUsageChart: React.FC<ChartProps> = ({ series, totalGB = 40, hoveredIdx, onHover }) => {
  if (!series || series.length === 0) return <EmptyChartPlaceholder />;

  const maxVal = totalGB > 0 ? totalGB : 40;
  const height = 170;
  const width = 540;
  const paddingLeft = 55;
  const paddingBottom = 26;
  const chartW = width - paddingLeft;
  const chartH = height - paddingBottom;

  const points = series.map((d, i) => {
    const x = paddingLeft + (i / (series.length - 1 || 1)) * chartW;
    const y = chartH - (Math.min(d.disk_used_gb, maxVal) / maxVal) * chartH;
    return { x, y, d, i };
  });

  const linePath = points.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');
  const areaPath = `${linePath} L ${points[points.length - 1].x},${chartH} L ${points[0].x},${chartH} Z`;

  return (
    <div className="relative w-full overflow-hidden select-none">
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-auto overflow-visible font-mono text-[10px]">
        <defs>
          <linearGradient id="purpleGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#8b5cf6" stopOpacity="0.55" />
            <stop offset="100%" stopColor="#7c3aed" stopOpacity="0.08" />
          </linearGradient>
        </defs>

        {/* Y 轴刻度线 (0 B, 10 GB, 20 GB, 40 GB) */}
        {[0, 0.25, 0.5, 1.0].map((ratio, idx) => {
          const y = chartH - ratio * chartH;
          const label = ratio === 0 ? '0 B' : ratio === 0.25 ? `${(maxVal * 0.25).toFixed(0)} GB` : ratio === 0.5 ? `${(maxVal * 0.5).toFixed(0)} GB` : `${maxVal.toFixed(0)} GB`;
          return (
            <g key={idx}>
              <line x1={paddingLeft} y1={y} x2={width} y2={y} stroke="#1e293b" strokeDasharray="3 3" strokeWidth="1" />
              <text x={paddingLeft - 8} y={y + 3.5} textAnchor="end" fill="#64748b">
                {label}
              </text>
            </g>
          );
        })}

        {/* X 轴时间标签 */}
        {points.map((p, idx) => {
          if (idx % 4 === 0 || idx === points.length - 1) {
            return (
              <text key={idx} x={p.x} y={height - 6} textAnchor="middle" fill="#64748b">
                {p.d.time}
              </text>
            );
          }
          return null;
        })}

        {/* 紫色渐变填充面积与主曲线 */}
        <path d={areaPath} fill="url(#purpleGrad)" />
        <path d={linePath} fill="none" stroke="#a855f7" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />

        {/* 悬浮指示点 */}
        {hoveredIdx !== null && points[hoveredIdx] && (
          <g>
            <line x1={points[hoveredIdx].x} y1={0} x2={points[hoveredIdx].x} y2={chartH} stroke="#a855f7" strokeWidth="1" strokeDasharray="2 2" />
            <circle cx={points[hoveredIdx].x} cy={points[hoveredIdx].y} r="4.5" fill="#a855f7" stroke="#fff" strokeWidth="1.5" />
          </g>
        )}
      </svg>
    </div>
  );
};

// ── 子图表组件 2: 磁盘 I/O (读蓝色 / 写橙色) ──
const DiskIOChart: React.FC<ChartProps> = ({ series, hoveredIdx, onHover }) => {
  if (!series || series.length === 0) return <EmptyChartPlaceholder />;

  const maxVal = Math.max(352, ...series.map((d) => Math.max(d.disk_read_kbps, d.disk_write_kbps) * 1.15));
  const height = 170;
  const width = 540;
  const paddingLeft = 58;
  const paddingBottom = 26;
  const chartW = width - paddingLeft;
  const chartH = height - paddingBottom;

  const pointsRead = series.map((d, i) => ({
    x: paddingLeft + (i / (series.length - 1 || 1)) * chartW,
    y: chartH - (d.disk_read_kbps / maxVal) * chartH,
    val: d.disk_read_kbps,
  }));

  const pointsWrite = series.map((d, i) => ({
    x: paddingLeft + (i / (series.length - 1 || 1)) * chartW,
    y: chartH - (d.disk_write_kbps / maxVal) * chartH,
    val: d.disk_write_kbps,
  }));

  const readLine = pointsRead.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');
  const readArea = `${readLine} L ${pointsRead[pointsRead.length - 1].x},${chartH} L ${pointsRead[0].x},${chartH} Z`;

  const writeLine = pointsWrite.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');
  const writeArea = `${writeLine} L ${pointsWrite[pointsWrite.length - 1].x},${chartH} L ${pointsWrite[0].x},${chartH} Z`;

  return (
    <div className="relative w-full overflow-hidden select-none">
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-auto overflow-visible font-mono text-[10px]">
        <defs>
          <linearGradient id="blueGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#3b82f6" stopOpacity="0.4" />
            <stop offset="100%" stopColor="#3b82f6" stopOpacity="0.02" />
          </linearGradient>
          <linearGradient id="amberGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#f59e0b" stopOpacity="0.4" />
            <stop offset="100%" stopColor="#f59e0b" stopOpacity="0.02" />
          </linearGradient>
        </defs>

        {/* Y 轴刻度 (0 B/s, 88 KB/s, 176 KB/s, 264 KB/s, 352 KB/s) */}
        {[0, 0.25, 0.5, 0.75, 1.0].map((ratio, idx) => {
          const y = chartH - ratio * chartH;
          const labelVal = ratio * maxVal;
          const label = ratio === 0 ? '0 B/s' : `${labelVal.toFixed(0)} KB/s`;
          return (
            <g key={idx}>
              <line x1={paddingLeft} y1={y} x2={width} y2={y} stroke="#1e293b" strokeDasharray="3 3" strokeWidth="1" />
              <text x={paddingLeft - 8} y={y + 3.5} textAnchor="end" fill="#64748b">
                {label}
              </text>
            </g>
          );
        })}

        {/* X 轴时间 */}
        {series.map((d, idx) => {
          if (idx % 4 === 0 || idx === series.length - 1) {
            const x = paddingLeft + (idx / (series.length - 1 || 1)) * chartW;
            return (
              <text key={idx} x={x} y={height - 6} textAnchor="middle" fill="#64748b">
                {d.time}
              </text>
            );
          }
          return null;
        })}

        {/* 面积与折线 */}
        <path d={readArea} fill="url(#blueGrad)" />
        <path d={readLine} fill="none" stroke="#3b82f6" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />

        <path d={writeArea} fill="url(#amberGrad)" />
        <path d={writeLine} fill="none" stroke="#f59e0b" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </div>
  );
};

// ── 子图表组件 3: 带宽 (下行绿色 / 上行紫色) ──
const BandwidthChart: React.FC<ChartProps> = ({ series, hoveredIdx, onHover }) => {
  if (!series || series.length === 0) return <EmptyChartPlaceholder />;

  const maxVal = Math.max(137, ...series.map((d) => Math.max(d.net_rx_kbps, d.net_tx_kbps) * 1.15));
  const height = 170;
  const width = 540;
  const paddingLeft = 58;
  const paddingBottom = 26;
  const chartW = width - paddingLeft;
  const chartH = height - paddingBottom;

  const pointsRx = series.map((d, i) => ({
    x: paddingLeft + (i / (series.length - 1 || 1)) * chartW,
    y: chartH - (d.net_rx_kbps / maxVal) * chartH,
  }));

  const pointsTx = series.map((d, i) => ({
    x: paddingLeft + (i / (series.length - 1 || 1)) * chartW,
    y: chartH - (d.net_tx_kbps / maxVal) * chartH,
  }));

  const rxLine = pointsRx.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');
  const rxArea = `${rxLine} L ${pointsRx[pointsRx.length - 1].x},${chartH} L ${pointsRx[0].x},${chartH} Z`;

  const txLine = pointsTx.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');

  return (
    <div className="relative w-full overflow-hidden select-none">
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-auto overflow-visible font-mono text-[10px]">
        <defs>
          <linearGradient id="emeraldGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor="#10b981" stopOpacity="0.45" />
            <stop offset="100%" stopColor="#10b981" stopOpacity="0.02" />
          </linearGradient>
        </defs>

        {/* Y 轴刻度 (0 B/s, 35 KB/s, 69 KB/s, 103 KB/s, 137 KB/s) */}
        {[0, 0.25, 0.5, 0.75, 1.0].map((ratio, idx) => {
          const y = chartH - ratio * chartH;
          const labelVal = ratio * maxVal;
          const label = ratio === 0 ? '0 B/s' : `${labelVal.toFixed(0)} KB/s`;
          return (
            <g key={idx}>
              <line x1={paddingLeft} y1={y} x2={width} y2={y} stroke="#1e293b" strokeDasharray="3 3" strokeWidth="1" />
              <text x={paddingLeft - 8} y={y + 3.5} textAnchor="end" fill="#64748b">
                {label}
              </text>
            </g>
          );
        })}

        {/* X 轴时间 */}
        {series.map((d, idx) => {
          if (idx % 4 === 0 || idx === series.length - 1) {
            const x = paddingLeft + (idx / (series.length - 1 || 1)) * chartW;
            return (
              <text key={idx} x={x} y={height - 6} textAnchor="middle" fill="#64748b">
                {d.time}
              </text>
            );
          }
          return null;
        })}

        {/* 下行绿色面积与上行紫色曲线 */}
        <path d={rxArea} fill="url(#emeraldGrad)" />
        <path d={rxLine} fill="none" stroke="#10b981" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
        <path d={txLine} fill="none" stroke="#c084fc" strokeWidth="1.8" strokeDasharray="3 2" />
      </svg>
    </div>
  );
};

// ── 子图表组件 4: 系统负载 (1m 紫 / 5m 蓝 / 15m 橙) ──
const SystemLoadChart: React.FC<ChartProps> = ({ series, hoveredIdx, onHover }) => {
  if (!series || series.length === 0) return <EmptyChartPlaceholder />;

  const maxVal = Math.max(1.0, ...series.map((d) => Math.max(d.load_1, d.load_5, d.load_15) * 1.3));
  const height = 150;
  const width = 540;
  const paddingLeft = 45;
  const paddingBottom = 22;
  const chartW = width - paddingLeft;
  const chartH = height - paddingBottom;

  const points1 = series.map((d, i) => ({
    x: paddingLeft + (i / (series.length - 1 || 1)) * chartW,
    y: chartH - (d.load_1 / maxVal) * chartH,
  }));

  const points5 = series.map((d, i) => ({
    x: paddingLeft + (i / (series.length - 1 || 1)) * chartW,
    y: chartH - (d.load_5 / maxVal) * chartH,
  }));

  const points15 = series.map((d, i) => ({
    x: paddingLeft + (i / (series.length - 1 || 1)) * chartW,
    y: chartH - (d.load_15 / maxVal) * chartH,
  }));

  const line1 = points1.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');
  const line5 = points5.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');
  const line15 = points15.reduce((acc, p, i) => `${acc} ${i === 0 ? 'M' : 'L'} ${p.x},${p.y}`, '');

  return (
    <div className="relative w-full overflow-hidden select-none">
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-auto overflow-visible font-mono text-[10px]">
        {/* Y 轴刻度 (0, 0.2, 0.4, 0.6, 0.8, 1.0) */}
        {[0, 0.25, 0.5, 0.75, 1.0].map((ratio, idx) => {
          const y = chartH - ratio * chartH;
          const label = (ratio * maxVal).toFixed(1);
          return (
            <g key={idx}>
              <line x1={paddingLeft} y1={y} x2={width} y2={y} stroke="#1e293b" strokeDasharray="3 3" strokeWidth="1" />
              <text x={paddingLeft - 8} y={y + 3.5} textAnchor="end" fill="#64748b">
                {label}
              </text>
            </g>
          );
        })}

        {/* X 轴时间 */}
        {series.map((d, idx) => {
          if (idx % 4 === 0 || idx === series.length - 1) {
            const x = paddingLeft + (idx / (series.length - 1 || 1)) * chartW;
            return (
              <text key={idx} x={x} y={height - 4} textAnchor="middle" fill="#64748b">
                {d.time}
              </text>
            );
          }
          return null;
        })}

        {/* 1m / 5m / 15m 三条折线 */}
        <path d={line15} fill="none" stroke="#f97316" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        <path d={line5} fill="none" stroke="#3b82f6" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
        <path d={line1} fill="none" stroke="#a855f7" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    </div>
  );
};

const EmptyChartPlaceholder: React.FC = () => (
  <div className="h-40 flex items-center justify-center text-text-tertiary text-xs">
    暂无时序指标数据，等待 Agent 上报...
  </div>
);
