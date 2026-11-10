/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Monitor, Wifi, WifiOff, Activity, ArrowUpRight, ArrowDownRight,
  Server, AlertTriangle, CheckCircle2, XCircle, Clock, RefreshCw,
  ChevronLeft, Cpu, HardDrive, Thermometer, Zap, Settings, Trash2, Plus, X,
  BarChart3, Network, Loader2, Info, Tag, Sparkles
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';
import CustomListPanel from '../components/CustomListPanel';
import { useNavigate } from 'react-router-dom';
import CircularProgress from '../../monitor/components/CircularProgress';
import AnimatedBarChart from '../../monitor/components/AnimatedBarChart';
import { cleanDisplayValue, getMetricIconConfig } from '../utils/metricHelpers';
import SendToDashboardModal from '../components/SendToDashboardModal';

// ============================================================
// 常量与类型
// ============================================================

const VENDOR_COLORS: Record<string, string> = {
  huawei: '#e04040',
  h3c: '#3b82f6',
  cisco: '#22c55e',
  ruijie: '#f59e0b',
  zte: '#8b5cf6',
  maipu: '#ec4899',
  fortinet: '#eab308',
  juniper: '#14b8a6',
  other: '#6b7280',
};

const VENDOR_LABELS: Record<string, string> = {
  huawei: '华为', h3c: '华三', cisco: '思科', ruijie: '锐捷',
  zte: '中兴', maipu: '迈普', fortinet: '飞塔', juniper: '瞻博',
  other: '其他',
};

// ============================================================
// 主组件
// ============================================================

export default function NetworkDashboard() {
  const navigate = useNavigate();
  const [refreshKey, setRefreshKey] = useState(0);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [selectedVendorFilter, setSelectedVendorFilter] = useState<string>('all');
  const [selectedDevice, setSelectedDevice] = useState<string | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout>>();

  // 大屏数据
  const { data: stats, isLoading: statsLoading } = useQuery({
    queryKey: ['network-dashboard-stats', refreshKey],
    queryFn: () => api.get('/dashboard/network-device-stats').then(res => res.data.data),
    refetchInterval: autoRefresh ? 30000 : false,
  });

  // 设备列表
  const { data: devices = [] } = useQuery({
    queryKey: ['network-devices-list', refreshKey],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
    refetchInterval: autoRefresh ? 30000 : false,
  });

  // 告警数据
  const { data: alertsData } = useQuery({
    queryKey: ['network-alerts-stats', refreshKey],
    queryFn: () => api.get('/alerts', { params: { limit: 10, source: 'network' } }).then(res => res.data.data || []),
    refetchInterval: autoRefresh ? 30000 : false,
  });

  const alerts = Array.isArray(alertsData) ? alertsData : (alertsData?.alerts || []);

  // 选中设备接口摘要
  const { data: interfaceSummary } = useQuery({
    queryKey: ['device-interface-summary', selectedDevice],
    queryFn: () => api.get(`/network-devices/${selectedDevice}/interface-summary`).then(res => res.data.data),
    enabled: !!selectedDevice,
  });

  const handleRefresh = useCallback(() => setRefreshKey(k => k + 1), []);

  useEffect(() => {
    return () => { if (timerRef.current) clearInterval(timerRef.current); };
  }, []);

  const deviceStats = stats?.devices || { total: 0, online: 0, offline: 0, snmpEnabled: 0 };
  const interfaceStats = stats?.interfaces || { totalUp: 0, totalDown: 0, totalErrors: 0 };
  const throughput = stats?.throughput || { totalInMbps: 0, totalOutMbps: 0, totalMbps: 0 };
  const topDevices = stats?.topDevicesByTraffic || [];
  const vendorDist = stats?.vendorDistribution || [];
  const recentInspections = stats?.recentInspections || [];

  // 厂商分布图数据
  const vendorChartData = vendorDist.map((v: any) => ({
    label: VENDOR_LABELS[v.vendor] || v.vendor || '其他',
    value: v.count,
    color: VENDOR_COLORS[v.vendor] || VENDOR_COLORS.other,
  }));

  // Top设备流量图数据
  const topTrafficData = topDevices.slice(0, 8).map((d: any) => ({
    label: d.device_name?.length > 8 ? d.device_name.slice(0, 8) + '..' : d.device_name || d.device_id,
    value: Number(d.total_traffic_mbps?.toFixed(1)) || 0,
    color: '#3b82f6',
  }));

  // 计算各厂商设备数量
  const vendorCounts = useMemo(() => {
    const counts: Record<string, number> = { all: devices.length };
    for (const d of devices) {
      const v = (d.vendor || 'other').toLowerCase();
      counts[v] = (counts[v] || 0) + 1;
    }
    return counts;
  }, [devices]);

  // 根据厂商筛选的设备列表
  const filteredDevices = useMemo(() => {
    if (selectedVendorFilter === 'all') return devices;
    return devices.filter((d: any) => (d.vendor || 'other').toLowerCase() === selectedVendorFilter.toLowerCase());
  }, [devices, selectedVendorFilter]);

  return (
    <div className="viz-dark min-h-screen bg-[#0a0e1a] text-white overflow-y-auto">
      {/* 顶部标题栏 */}
      <div className="sticky top-0 z-30 bg-gradient-to-r from-[#0d1225] via-[#111836] to-[#0d1225] border-b border-blue-500/20 px-6 py-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <button
              onClick={() => window.history.back()}
              aria-label="返回"
              className="p-1.5 hover:bg-white/5 rounded-lg transition-colors"
            >
              <ChevronLeft className="w-5 h-5 text-blue-400" />
            </button>
            <div className="flex items-center gap-2">
              <Monitor className="w-5 h-5 text-blue-400" />
              <h1 className="text-lg font-bold bg-gradient-to-r from-blue-400 to-cyan-400 bg-clip-text text-transparent">
                网络设备监控大屏
              </h1>
            </div>
            <span className="text-xs text-slate-500">实时监控</span>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex items-center gap-1.5 text-xs text-slate-400">
              <Clock className="w-3.5 h-3.5" />
              {new Date().toLocaleString('zh-CN')}
            </div>
            <button
              onClick={() => setAutoRefresh(!autoRefresh)}
              className={`flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-md transition-colors ${
                autoRefresh ? 'bg-emerald-500/10 text-emerald-400' : 'bg-slate-700/50 text-slate-400'
              }`}
            >
              <RefreshCw className={`w-3 h-3 ${autoRefresh ? 'animate-spin' : ''}`} style={{ animationDuration: '3s' }} />
              {autoRefresh ? '自动刷新' : '手动'}
            </button>
            <button
              onClick={handleRefresh}
              aria-label="刷新数据"
              className="p-1.5 hover:bg-white/5 rounded-lg transition-colors text-slate-400 hover:text-white"
            >
              <RefreshCw className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      {/* 厂商快捷筛选器 Bar */}
      <div className="bg-[#111836]/90 border-b border-blue-500/10 px-6 py-2 flex items-center justify-between overflow-x-auto sticky top-[53px] z-20 backdrop-blur-md">
        <div className="flex items-center gap-1.5 text-xs">
          <span className="text-slate-400 font-medium mr-1 flex items-center gap-1">
            <Server className="w-3.5 h-3.5 text-blue-400" /> 厂商筛选:
          </span>
          {[
            { id: 'all', label: '全部设备' },
            { id: 'huawei', label: '华为' },
            { id: 'h3c', label: '华三' },
            { id: 'cisco', label: '思科' },
            { id: 'ruijie', label: '锐捷' },
            { id: 'zte', label: '中兴' },
            { id: 'other', label: '其他' },
          ].map(v => {
            const count = vendorCounts[v.id] || 0;
            const isSelected = selectedVendorFilter === v.id;
            const vColor = VENDOR_COLORS[v.id] || '#3b82f6';
            return (
              <button
                key={v.id}
                onClick={() => setSelectedVendorFilter(v.id)}
                className={`flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold transition-all ${
                  isSelected
                    ? 'bg-gradient-to-r from-blue-600 to-cyan-600 text-white shadow-md shadow-blue-500/20'
                    : 'bg-[#0d1225] text-slate-400 hover:text-slate-200 border border-blue-500/10 hover:border-blue-500/30'
                }`}
              >
                {v.id !== 'all' && <span className="w-2 h-2 rounded-full" style={{ backgroundColor: isSelected ? '#ffffff' : vColor }} />}
                {v.label}
                <span className={`text-[10px] px-1.5 py-0.2 rounded-full ${isSelected ? 'bg-white/20 text-white' : 'bg-slate-800 text-slate-400'}`}>
                  {count}
                </span>
              </button>
            );
          })}
        </div>
        {selectedVendorFilter !== 'all' && (
          <button
            onClick={() => setSelectedVendorFilter('all')}
            className="text-xs text-blue-400 hover:text-blue-300 transition-colors flex items-center gap-1"
          >
            <X className="w-3 h-3" /> 重置筛选
          </button>
        )}
      </div>

      <div className="p-4 space-y-4">
          {/* ========== 顶部：网络设备接口指标 (支持下拉选择设备) ========== */}
          <TopInterfaceMetricsSection devices={devices} />

          {/* ========== 第一行：KPI 概览 ========== */}
          <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
            <KpiCard
              icon={<Server className="w-5 h-5" />}
              label="设备总数"
              value={deviceStats.total}
              color="#3b82f6"
            />
            <KpiCard
              icon={<Wifi className="w-5 h-5" />}
              label="在线设备"
              value={deviceStats.online}
              suffix={`/${deviceStats.total}`}
              color="#22c55e"
            />
            <KpiCard
              icon={<WifiOff className="w-5 h-5" />}
              label="离线设备"
              value={deviceStats.offline}
              color="#ef4444"
            />
            <KpiCard
              icon={<Activity className="w-5 h-5" />}
              label="总吞吐量"
              value={throughput.totalMbps.toFixed(1)}
              unit="Mbps"
              color="#f59e0b"
            />
            <KpiCard
              icon={<ArrowUpRight className="w-5 h-5" />}
              label="接口 UP"
              value={interfaceStats.totalUp}
              color="#22c55e"
            />
            <KpiCard
              icon={<AlertTriangle className="w-5 h-5" />}
              label="接口错误"
              value={interfaceStats.totalErrors}
              color="#ef4444"
            />
          </div>

          {/* ========== 第二行：吞吐量 + 厂商分布 + Top 流量设备 ========== */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            {/* 上下行吞吐量 */}
            <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4">
              <h3 className="text-sm font-medium text-slate-300 mb-3 flex items-center gap-2">
                <Activity className="w-4 h-4 text-blue-400" />
                吞吐量概览
              </h3>
              <div className="grid grid-cols-2 gap-4">
                <div className="text-center">
                  <div className="flex items-center justify-center gap-1 mb-2">
                    <ArrowUpRight className="w-4 h-4 text-emerald-400" />
                    <span className="text-xs text-slate-400">上行</span>
                  </div>
                  <p className="text-2xl font-bold text-emerald-400">
                    {(throughput.totalOutMbps ?? 0).toFixed(2)}
                  </p>
                  <p className="text-xs text-slate-500 mt-1">Mbps</p>
                </div>
                <div className="text-center">
                  <div className="flex items-center justify-center gap-1 mb-2">
                    <ArrowDownRight className="w-4 h-4 text-amber-400" />
                    <span className="text-xs text-slate-400">下行</span>
                  </div>
                  <p className="text-2xl font-bold text-amber-400">
                    {(throughput.totalInMbps ?? 0).toFixed(2)}
                  </p>
                  <p className="text-xs text-slate-500 mt-1">Mbps</p>
                </div>
              </div>
              <div className="mt-4 flex items-center justify-between text-xs text-slate-500">
                <span>SNMP 采集设备数</span>
                <span className="text-blue-400 font-medium">{deviceStats.snmpEnabled}</span>
              </div>
              <div className="mt-1 w-full bg-slate-700/30 rounded-full h-1.5">
                <div
                  className="bg-gradient-to-r from-blue-500 to-cyan-400 h-1.5 rounded-full transition-all"
                  style={{ width: `${deviceStats.total > 0 ? (deviceStats.snmpEnabled / deviceStats.total) * 100 : 0}%` }}
                />
              </div>
            </div>

            {/* 厂商分布 */}
            <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4">
              <h3 className="text-sm font-medium text-slate-300 mb-3 flex items-center gap-2">
                <Server className="w-4 h-4 text-purple-400" />
                厂商分布
              </h3>
              {vendorChartData.length > 0 ? (
                <>
                  <AnimatedBarChart data={vendorChartData} height={160} />
                </>
              ) : (
                <div className="flex items-center justify-center h-40 text-slate-500 text-sm">暂无数据</div>
              )}
            </div>

            {/* Top 流量设备 (瀑布流星雨粒子流光) */}
            <WaterfallMeteorTopTrafficPanel topDevices={topDevices} />
          </div>

          {/* ========== 第三行：设备状态列表 + 接口详情 ========== */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {/* 设备状态表 */}
            <div className="bg-[#111836] rounded-xl border border-blue-500/10 overflow-hidden">
              <div className="px-4 py-3 border-b border-blue-500/10 flex items-center justify-between">
                <h3 className="text-sm font-medium text-slate-300 flex items-center gap-2">
                  <Server className="w-4 h-4 text-blue-400" />
                  设备资源状态
                  <span className="text-xs text-slate-500">({devices.length})</span>
                </h3>
              </div>
              <div className="max-h-80 overflow-y-auto">
                <table className="w-full">
                  <thead className="sticky top-0 bg-[#111836] z-10">
                    <tr className="border-b border-blue-500/10">
                      <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">设备名称</th>
                      <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">IP</th>
                      <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">厂商</th>
                      <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">状态</th>
                      <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">运行时长</th>
                      <th className="text-right text-xs font-medium text-slate-500 px-3 py-2 w-10">指标</th>
                    </tr>
                  </thead>
                  <tbody>
                    {devices.map((d: any) => (
                      <tr
                        key={d.id}
                        onClick={() => setSelectedDevice(selectedDevice === d.id ? null : d.id)}
                        className={`border-b border-slate-700/30 cursor-pointer transition-colors ${
                          selectedDevice === d.id ? 'bg-blue-500/10' : 'hover:bg-white/[0.02]'
                        }`}
                      >
                        <td className="px-3 py-2">
                          <span className="text-sm text-slate-200">{d.name}</span>
                        </td>
                        <td className="px-3 py-2">
                          <span className="text-xs font-mono text-amber-400">{d.ip_address}</span>
                        </td>
                        <td className="px-3 py-2">
                          <span
                            className="text-xs px-1.5 py-0.5 rounded"
                            style={{ color: VENDOR_COLORS[d.vendor] || '#6b7280', backgroundColor: (VENDOR_COLORS[d.vendor] || '#6b7280') + '15' }}
                          >
                            {VENDOR_LABELS[d.vendor] || d.vendor || '-'}
                          </span>
                        </td>
                        <td className="px-3 py-2">
                          <span className={`inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full ${
                            d.status === 'online' ? 'bg-emerald-500/10 text-emerald-400' : 'bg-red-500/10 text-red-400'
                          }`}>
                            {d.status === 'online' ? <Wifi className="w-3 h-3" /> : <WifiOff className="w-3 h-3" />}
                            {d.status === 'online' ? 'UP' : 'DOWN'}
                          </span>
                        </td>
                        <td className="px-3 py-2">
                          <span className="text-xs text-emerald-400">
                            {d.last_inspection_at ? formatUptime(d.last_inspection_at) : '-'}
                          </span>
                        </td>
                        <td className="px-3 py-2 text-right" onClick={(e) => e.stopPropagation()}>
                          <button
                            onClick={() => navigate(`/device-metrics/${d.id}`)}
                            className="p-1 text-slate-500 hover:text-cyan-400 hover:bg-cyan-500/10 rounded transition-colors"
                            title="查看设备采集仪表盘"
                          >
                            <BarChart3 className="w-3.5 h-3.5" />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            {/* 接口流量详情 / 设备详情面板 */}
            <div className="bg-[#111836] rounded-xl border border-blue-500/10 overflow-hidden">
              <div className="px-4 py-3 border-b border-blue-500/10 flex items-center justify-between">
                <h3 className="text-sm font-medium text-slate-300 flex items-center gap-2">
                  <Activity className="w-4 h-4 text-cyan-400" />
                  {selectedDevice
                    ? `接口详情 - ${(devices as any[]).find((d: any) => d.id === selectedDevice)?.name || ''}`
                    : '指定接口流量'}
                </h3>
              </div>
              <div className="max-h-80 overflow-y-auto">
                {selectedDevice && interfaceSummary ? (
                  <table className="w-full">
                    <thead className="sticky top-0 bg-[#111836] z-10">
                      <tr className="border-b border-blue-500/10">
                        <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">接口</th>
                        <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">描述</th>
                        <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">状态</th>
                        <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">入站速率</th>
                        <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">出站速率</th>
                        <th className="text-left text-xs font-medium text-slate-500 px-3 py-2">利用率</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(interfaceSummary?.interfaces || []).map((iface: any, i: number) => {
                        const isUp = iface.if_oper_status === 1 || iface.operStatus === 'up' || iface.operStatus === 1;
                        const ifaceName = iface.interface_name || iface.name || '-';
                        const ifaceDesc = iface.if_description || iface.description || '';
                        const inUtil = iface.if_in_utilization ?? iface.inUtilization ?? null;
                        const outUtil = iface.if_out_utilization ?? iface.outUtilization ?? null;
                        const inOctets = iface.if_in_octets ?? iface.inOctets ?? 0;
                        const outOctets = iface.if_out_octets ?? iface.outOctets ?? 0;
                        const speed = iface.if_speed ?? iface.speed ?? 0;
                        return (
                          <tr key={i} className="border-b border-slate-700/30 hover:bg-white/[0.02]">
                            <td className="px-3 py-2">
                              <span className="text-sm text-emerald-400">{ifaceName}</span>
                            </td>
                            <td className="px-3 py-2">
                              <span className="text-xs text-slate-400" title={ifaceDesc}>
                                {ifaceDesc ? (ifaceDesc.length > 16 ? ifaceDesc.slice(0, 16) + '..' : ifaceDesc) : '-'}
                              </span>
                            </td>
                            <td className="px-3 py-2">
                              <span className={`inline-flex items-center text-xs px-2 py-0.5 rounded-full ${
                                isUp ? 'bg-emerald-500/10 text-emerald-400' : 'bg-red-500/10 text-red-400'
                              }`}>
                                {isUp ? 'UP' : 'DOWN'}
                              </span>
                            </td>
                            <td className="px-3 py-2">
                              <span className="text-xs text-amber-400 font-mono">
                                {inUtil != null ? `${Number(inUtil).toFixed(2)}%` : formatOctets(inOctets, speed)}
                              </span>
                            </td>
                            <td className="px-3 py-2">
                              <span className="text-xs text-cyan-400 font-mono">
                                {outUtil != null ? `${Number(outUtil).toFixed(2)}%` : formatOctets(outOctets, speed)}
                              </span>
                            </td>
                            <td className="px-3 py-2">
                              <div className="flex items-center gap-2">
                                <div className="w-16 bg-slate-700/30 rounded-full h-1.5">
                                  <div
                                    className="h-1.5 rounded-full transition-all"
                                    style={{
                                      width: `${Math.min(Math.max(inUtil || 0, 0), 100)}%`,
                                      background: getUtilizationColor(inUtil),
                                    }}
                                  />
                                </div>
                                <span className="text-xs text-slate-400">
                                  {inUtil != null ? `${Number(inUtil).toFixed(1)}%` : '-'}
                                </span>
                              </div>
                            </td>
                          </tr>
                        );
                      })}
                      {(!interfaceSummary?.interfaces || interfaceSummary.interfaces.length === 0) && (
                        <tr><td colSpan={6} className="text-center py-8 text-sm text-slate-500">暂无接口数据</td></tr>
                      )}
                    </tbody>
                  </table>
                ) : (
                  <div className="flex items-center justify-center h-40 text-slate-500 text-sm">
                    <div className="text-center">
                      <Monitor className="w-8 h-8 mx-auto mb-2 text-slate-600" />
                      <p>点击左侧设备查看接口详情</p>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* ========== 第四行：设备健康仪表盘 + 最近巡检 + 告警 ========== */}
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            {/* 设备健康仪表盘 */}
            <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4">
              <h3 className="text-sm font-medium text-slate-300 mb-4 flex items-center gap-2">
                <Cpu className="w-4 h-4 text-blue-400" />
                设备健康概览
              </h3>
              <div className="flex items-center justify-around">
                <div className="text-center">
                  <CircularProgress
                    value={deviceStats.total > 0 ? (deviceStats.online / deviceStats.total) * 100 : 0}
                    size={100}
                    strokeWidth={8}
                    color="#22c55e"
                    label="在线率"
                  />
                </div>
                <div className="text-center">
                  <CircularProgress
                    value={deviceStats.total > 0 ? (deviceStats.snmpEnabled / deviceStats.total) * 100 : 0}
                    size={100}
                    strokeWidth={8}
                    color="#3b82f6"
                    label="SNMP覆盖"
                  />
                </div>
                <div className="text-center">
                  <CircularProgress
                    value={deviceStats.online > 0 && interfaceStats.totalUp + interfaceStats.totalDown > 0
                      ? (interfaceStats.totalUp / (interfaceStats.totalUp + interfaceStats.totalDown)) * 100
                      : 0}
                    size={100}
                    strokeWidth={8}
                    color="#f59e0b"
                    label="接口UP率"
                  />
                </div>
              </div>
            </div>

            {/* 最近巡检 */}
            <div className="bg-[#111836] rounded-xl border border-blue-500/10 overflow-hidden">
              <div className="px-4 py-3 border-b border-blue-500/10">
                <h3 className="text-sm font-medium text-slate-300 flex items-center gap-2">
                  <CheckCircle2 className="w-4 h-4 text-emerald-400" />
                  最近巡检
                </h3>
              </div>
              <div className="max-h-52 overflow-y-auto">
                {recentInspections.length > 0 ? recentInspections.map((insp: any, i: number) => (
                  <div key={i} className="px-4 py-2.5 border-b border-slate-700/30 hover:bg-white/[0.02] transition-colors">
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-slate-200">{insp.device_name}</span>
                      <span className={`text-xs px-1.5 py-0.5 rounded ${
                        insp.status === 'success' ? 'bg-emerald-500/10 text-emerald-400' : 'bg-red-500/10 text-red-400'
                      }`}>
                        {insp.status === 'success' ? '成功' : '失败'}
                      </span>
                    </div>
                    <div className="flex items-center justify-between mt-1">
                      <span className="text-xs text-slate-500">{insp.inspection_type}</span>
                      <span className="text-xs text-slate-500">{insp.duration_ms ? `${insp.duration_ms}ms` : '-'}</span>
                    </div>
                    <div className="text-xs text-slate-600 mt-0.5">
                      {insp.created_at ? new Date(insp.created_at).toLocaleString('zh-CN') : '-'}
                    </div>
                  </div>
                )) : (
                  <div className="text-center py-8 text-sm text-slate-500">暂无巡检记录</div>
                )}
              </div>
            </div>

            {/* 告警列表 */}
            <div className="bg-[#111836] rounded-xl border border-blue-500/10 overflow-hidden">
              <div className="px-4 py-3 border-b border-blue-500/10">
                <h3 className="text-sm font-medium text-slate-300 flex items-center gap-2">
                  <AlertTriangle className="w-4 h-4 text-red-400" />
                  最近告警
                </h3>
              </div>
              <div className="max-h-52 overflow-y-auto">
                {alerts.length > 0 ? alerts.map((alert: any, i: number) => (
                  <div key={alert.id || i} className="px-4 py-2.5 border-b border-slate-700/30 hover:bg-white/[0.02] transition-colors">
                    <div className="flex items-center justify-between">
                      <span className="text-sm text-slate-200 truncate max-w-[70%]">{alert.title || alert.name || '-'}</span>
                      <span className={`text-xs px-1.5 py-0.5 rounded ${
                        alert.severity === 'critical' ? 'bg-red-500/10 text-red-400' :
                        alert.severity === 'warning' ? 'bg-amber-500/10 text-amber-400' :
                        'bg-blue-500/10 text-blue-400'
                      }`}>
                        {alert.severity === 'critical' ? '严重' : alert.severity === 'warning' ? '警告' : '信息'}
                      </span>
                    </div>
                    <div className="text-xs text-slate-500 mt-1">
                      {alert.source_name || alert.source || '-'}
                    </div>
                    <div className="text-xs text-slate-600 mt-0.5">
                      {alert.created_at ? new Date(alert.created_at).toLocaleString('zh-CN') : '-'}
                    </div>
                  </div>
                )) : (
                  <div className="flex items-center justify-center h-32 text-sm text-slate-500">
                    <CheckCircle2 className="w-5 h-5 mr-2 text-emerald-500" />
                    暂无告警
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* ========== 第五行：自定义监控指标 ========== */}
          <CustomMetricsPanel vendorFilter={selectedVendorFilter} />

          {/* ========== SNMP 指标面板（Grafana 风格可配置） ========== */}
          <SnmpMetricPanelsSection />

          {/* ========== 第六行：自定义列表面板（Grafana 风格） ========== */}
          <CustomListPanel />
        </div>
    </div>
  );
}

// ============================================================
// 自定义监控指标面板
// ============================================================

function CustomMetricsPanel({ vendorFilter = 'all' }: { vendorFilter?: string }) {
  const toast = useToast();
  const [refreshKey, setRefreshKey] = useState(0);
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>('all');
  const [editingMetric, setEditingMetric] = useState<any | null>(null);
  const [reuseModalData, setReuseModalData] = useState<any | null>(null);

  // 获取所有指标
  const { data: customMetrics = [], isLoading, refetch } = useQuery({
    queryKey: ['custom-dashboard-metrics', refreshKey],
    queryFn: () => api.get('/snmp-collect/dashboard-metrics', { params: { dashboard_only: 'true' } }).then(res => res.data.data || []),
    refetchInterval: 60000,
  });

  // 获取设备列表（用于 IP 选择器）
  const { data: devices = [] } = useQuery({
    queryKey: ['network-devices-for-metrics'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
    staleTime: 60000,
  });

  const handleRefresh = async () => {
    try {
      await api.post('/snmp-collect/dashboard-metrics/refresh');
      refetch();
      toast.success('指标值已刷新');
    } catch (err: any) {
      toast.error(err.response?.data?.error || '刷新失败');
    }
  };

  // 按厂商和设备 ID 联动过滤指标
  const filteredMetrics = useMemo(() => {
    let list = customMetrics;
    if (vendorFilter && vendorFilter !== 'all') {
      const vendorDevIds = new Set(
        devices.filter((d: any) => (d.vendor || 'other').toLowerCase() === vendorFilter.toLowerCase()).map((d: any) => d.id)
      );
      list = list.filter((m: any) => vendorDevIds.has(m.device_id));
    }
    if (selectedDeviceId !== 'all') {
      list = list.filter((m: any) => m.device_id === selectedDeviceId);
    }
    return list;
  }, [customMetrics, vendorFilter, selectedDeviceId, devices]);

  // 构建设备选项
  const deviceOptions = [
    { id: 'all', name: '全部设备', ip: '' },
    ...devices.map((d: any) => ({ id: d.id, name: d.name, ip: d.ip_address })),
  ];

  // 值映射：解析 JSON 并应用
  const applyValueMapping = (rawValue: string | null, mappingsStr: string | null): { display: string; color: string } => {
    if (!rawValue || rawValue === 'N/A' || !mappingsStr) {
      return { display: rawValue ?? '-', color: '#6b7280' };
    }
    try {
      const mappings = JSON.parse(mappingsStr) as Record<string, string>;
      const mapped = mappings[rawValue];
      if (mapped) {
        // 根据映射值内容判断颜色
        if (/正常|online|up|active|running/i.test(mapped)) return { display: mapped, color: '#22c55e' };
        if (/异常|down|offline|error|fail/i.test(mapped)) return { display: mapped, color: '#ef4444' };
        if (/未知|unknown/i.test(mapped)) return { display: mapped, color: '#f59e0b' };
        return { display: mapped, color: '#06b6d4' };
      }
    } catch { /* ignore */ }
    return { display: rawValue, color: '#06b6d4' };
  };

  if (!isLoading && customMetrics.length === 0) return null;

  return (
    <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4">
      <div className="flex items-center justify-between mb-4">
        <h3 className="text-sm font-medium text-slate-300 flex items-center gap-2">
          <Zap className="w-4 h-4 text-amber-400" />
          自定义监控指标
          <span className="text-xs text-slate-500">({filteredMetrics.length})</span>
        </h3>
        <div className="flex items-center gap-3">
          {/* 设备 IP 选择器 */}
          <select
            aria-label="选择设备"
            value={selectedDeviceId}
            onChange={e => setSelectedDeviceId(e.target.value)}
            className="px-2.5 py-1 bg-[#0d1225] border border-blue-500/20 rounded-md text-xs text-slate-300 focus:outline-none focus:border-blue-500 cursor-pointer"
          >
            {deviceOptions.map(d => (
              <option key={d.id} value={d.id}>
                {d.ip ? `${d.ip} (${d.name})` : d.name}
              </option>
            ))}
          </select>
          <button
            onClick={handleRefresh}
            className="flex items-center gap-1 px-2.5 py-1 text-xs text-blue-400 hover:bg-blue-500/10 rounded-md transition-colors"
          >
            <RefreshCw className="w-3 h-3" />
            刷新值
          </button>
        </div>
      </div>
      {isLoading ? (
        <div className="flex items-center justify-center py-8">
          <RefreshCw className="w-5 h-5 text-blue-400 animate-spin" />
        </div>
      ) : filteredMetrics.length === 0 ? (
        <div className="text-center py-8 text-sm text-slate-500">
          {selectedDeviceId !== 'all' ? '该设备暂无自定义指标' : '暂无自定义指标'}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-3">
          {filteredMetrics.map((m: any) => {
            const rawCleanValue = cleanDisplayValue(m.last_value);
            const { display, color } = applyValueMapping(rawCleanValue, m.value_mappings);
            const iconConfig = getMetricIconConfig(m.custom_name, m.oid);
            const IconComponent = iconConfig.icon;

            const isMapped = Boolean(m.value_mappings && rawCleanValue && (() => {
              try { return Boolean(JSON.parse(m.value_mappings)[rawCleanValue]); } catch { return false; }
            })());

            const targetDevice = devices.find((d: any) => d.id === m.device_id);
            const vendorKey = (targetDevice?.vendor || 'other').toLowerCase();
            const vendorName = VENDOR_LABELS[vendorKey] || targetDevice?.vendor || '';
            const vendorColor = VENDOR_COLORS[vendorKey] || '#6b7280';

            const numVal = parseFloat(rawCleanValue);
            const hasValidNum = !isNaN(numVal) && isFinite(numVal);

            // 智能与显式配置判定：默认百分比/使用率类指标自动渲染为 🍩 环形圆盘 (CircularProgress)
            // 注意：若单位明确指定为 MB/GB/Mbps 等实体容量/流量量纲，绝不能当作 0-100% 环形圆盘，应展示数字卡片
            const isPercentage = m.unit === '%' || (!m.unit && /率|百分比/i.test(m.custom_name));
            const isCircle = m.chart_type === 'circle' || m.chart_type === 'ring' ||
              ((!m.chart_type || m.chart_type === 'auto') && isPercentage && (m.unit === '%' || /率|CPU|磁盘|占用|UP率|百分比/i.test(m.custom_name)));

            const isBar = m.chart_type === 'bar' || m.chart_type === 'progress';

            return (
              <div
                key={m.id}
                className="bg-[#0d1225] rounded-xl border border-blue-500/15 p-3.5 hover:border-blue-500/40 transition-all shadow-md group flex flex-col justify-between"
              >
                {/* 顶部：图标 + 名称 + 操作按键 */}
                <div className="flex items-center justify-between mb-2">
                  <div className="flex items-center gap-2 min-w-0">
                    <div className={`p-1.5 rounded-lg border flex items-center justify-center ${iconConfig.bg}`}>
                      <IconComponent className="w-4 h-4" />
                    </div>
                    <span className="text-xs font-bold text-slate-200 truncate" title={m.custom_name}>
                      {m.custom_name}
                    </span>
                  </div>
                  <button
                    onClick={() => setEditingMetric(m)}
                    className="opacity-0 group-hover:opacity-100 p-1 text-slate-500 hover:text-blue-400 hover:bg-blue-500/10 rounded transition-all"
                    title="配置指标"
                  >
                    <Settings className="w-3.5 h-3.5" />
                  </button>
                </div>

                {/* 中间：依据图表样式配置动态渲染 (圆盘 / 柱状进度 / 映射值 / 数字大字) */}
                <div className="my-2 min-h-[70px] flex items-center justify-center">
                  {isMapped ? (
                    <span
                      className="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-bold"
                      style={{ backgroundColor: color + '22', color, border: `1px solid ${color}44` }}
                    >
                      <span className="w-2 h-2 rounded-full" style={{ backgroundColor: color }} />
                      {display}
                    </span>
                  ) : isCircle && hasValidNum ? (
                    <div className="flex items-center justify-center relative scale-95">
                      <CircularProgress
                        value={numVal}
                        size={82}
                        strokeWidth={7.5}
                        color={numVal > 85 ? '#ef4444' : numVal > 70 ? '#f59e0b' : '#3b82f6'}
                        showValue={true}
                      />
                    </div>
                  ) : isBar && hasValidNum ? (
                    <div className="w-full space-y-1.5 px-1">
                      <div className="flex justify-between items-baseline text-xs font-mono">
                        <span className="text-xl font-extrabold text-white tracking-tight">{display}</span>
                        <span className="text-xs font-bold text-cyan-400">{m.unit || '%'}</span>
                      </div>
                      <div className="w-full h-2.5 bg-slate-800/90 rounded-full overflow-hidden p-0.5 border border-slate-700/50">
                        <div
                          className="h-full rounded-full transition-all duration-500 shadow-sm"
                          style={{
                            width: `${Math.min(Math.max(numVal, 0), 100)}%`,
                            backgroundColor: numVal > 85 ? '#ef4444' : numVal > 70 ? '#f59e0b' : '#3b82f6'
                          }}
                        />
                      </div>
                    </div>
                  ) : (
                    <div className="flex items-baseline gap-1.5">
                      <span className="text-2xl font-extrabold font-mono text-white tracking-tight">
                        {display}
                      </span>
                      {m.unit && (
                        <span className="text-xs font-bold text-cyan-400">
                          {m.unit}
                        </span>
                      )}
                    </div>
                  )}
                </div>

                {/* 设备名称与厂商标识 */}
                <div className="flex items-center justify-between gap-1 mt-1 mb-2 pt-1 border-t border-slate-700/30">
                  <span className="text-xs font-semibold text-slate-300 truncate" title={targetDevice?.name || m.device_id}>
                    {targetDevice?.name || '网络设备'}
                  </span>
                  {vendorName && (
                    <span
                      className="px-1.5 py-0.5 text-[10px] font-bold rounded flex-shrink-0"
                      style={{
                        color: vendorColor,
                        backgroundColor: vendorColor + '18',
                        border: `1px solid ${vendorColor}33`
                      }}
                    >
                      {vendorName}
                    </span>
                  )}
                </div>

                {/* 底部：高亮突出设备 IP 和 采样时间 */}
                <div className="flex items-center justify-between pt-2 border-t border-blue-500/15">
                  <div
                    className="flex items-center gap-1.5 px-2 py-0.5 rounded bg-cyan-500/15 border border-cyan-500/30 text-cyan-300 text-xs font-mono font-bold tracking-wide shadow-sm"
                    title={targetDevice?.name ? `${targetDevice.name} (${targetDevice.ip_address})` : ''}
                  >
                    <Network className="w-3.5 h-3.5 text-cyan-400 flex-shrink-0" />
                    <span>{targetDevice?.ip_address || m.device_id}</span>
                  </div>
                  <span className="text-xs font-mono text-slate-400 font-medium" title={`OID: ${m.oid}`}>
                    {m.last_sampled_at ? new Date(m.last_sampled_at).toLocaleTimeString('zh-CN') : '-'}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* 编辑弹窗 */}
      {editingMetric && (
        <MetricEditModal
          metric={editingMetric}
          onClose={() => setEditingMetric(null)}
          onSave={async (updates) => {
            try {
              await api.put(`/snmp-collect/dashboard-metrics/${editingMetric.id}`, updates);
              toast.success('指标已更新');
              setRefreshKey(k => k + 1);
              setEditingMetric(null);
            } catch (err: any) {
              toast.error(err.response?.data?.error || '更新失败');
            }
          }}
          onDelete={async () => {
            try {
              await api.delete(`/snmp-collect/dashboard-metrics/${editingMetric.id}`);
              toast.success('指标已删除');
              setRefreshKey(k => k + 1);
              setEditingMetric(null);
            } catch (err: any) {
              toast.error(err.response?.data?.error || '删除失败');
            }
          }}
        />
      )}
    </div>
  );
}

// ============================================================
// 指标编辑弹窗 (支持自定义标签 Tag 与 Prometheus 命名编辑)
// ============================================================

const METRIC_NAME_PRESETS = [
  { label: 'cpu_usage', value: 'cpu_usage', desc: 'CPU利用率' },
  { label: 'memory_usage', value: 'memory_usage', desc: '内存利用率' },
  { label: 'device_status', value: 'device_status', desc: '设备状态' },
  { label: 'temperature_celsius', value: 'temperature_celsius', desc: '环境温度' },
  { label: 'fan_status', value: 'fan_status', desc: '风扇状态' },
  { label: 'power_status', value: 'power_status', desc: '电源状态' },
  { label: 'custom_metric', value: 'custom_metric', desc: '通用自定义指标' },
];

const TAG_PRESETS = [
  { key: 'env', value: 'production', label: 'env: production' },
  { key: 'cluster', value: 'core-net', label: 'cluster: core-net' },
  { key: 'role', value: 'core-switch', label: 'role: core-switch' },
  { key: 'rack', value: 'A-01', label: 'rack: A-01' },
  { key: 'location', value: 'IDC-1', label: 'location: IDC-1' },
  { key: 'source', value: 'snmp_poll', label: 'source: snmp_poll' },
];

const UNIT_PRESETS = ['%', '℃', 'MB', 'GB', 'Mbps', 'Gbps', 'ms', '个', 'RPM', 'V', 'A', 'W'];

function MetricEditModal({ metric, onClose, onSave, onDelete }: {
  metric: any;
  onClose: () => void;
  onSave: (updates: any) => Promise<void> | void;
  onDelete: () => Promise<void> | void;
}) {
  useEscapeKey({ onEscape: onClose });
  const [customName, setCustomName] = useState(metric.custom_name || '');
  const [metricName, setMetricName] = useState(() => {
    if (metric.metric_name) return metric.metric_name;
    const cn = (metric.custom_name || metric.original_name || '').toLowerCase();
    if (cn.includes('cpu')) return 'cpu_usage';
    if (cn.includes('内存') || cn.includes('mem')) return 'memory_usage';
    if (cn.includes('状态') || cn.includes('status')) return 'device_status';
    if (cn.includes('温') || cn.includes('temp')) return 'temperature_celsius';
    if (cn.includes('风扇') || cn.includes('fan')) return 'fan_status';
    if (cn.includes('电源') || cn.includes('power')) return 'power_status';
    const clean = cn.replace(/[^a-zA-Z0-9_]/g, '_').replace(/^_+|_+$/g, '');
    return clean || 'custom_metric';
  });

  const [tagList, setTagList] = useState<Array<{ id: string; key: string; value: string }>>(() => {
    const list: Array<{ id: string; key: string; value: string }> = [];
    let parsed: Record<string, string> = {};
    try {
      if (metric.tags) {
        parsed = typeof metric.tags === 'string' ? JSON.parse(metric.tags) : metric.tags;
      }
    } catch {}

    if (Object.keys(parsed).length > 0) {
      for (const [k, v] of Object.entries(parsed)) {
        list.push({ id: `tag-${Math.random().toString(36).slice(2, 7)}`, key: k, value: String(v) });
      }
    } else {
      list.push({ id: `tag-${Math.random().toString(36).slice(2, 7)}`, key: 'metric_label', value: metric.custom_name || '自定义指标' });
    }
    return list;
  });

  const [unit, setUnit] = useState(metric.unit || '');
  const [chartType, setChartType] = useState(metric.chart_type || 'circle');
  const [mappings, setMappings] = useState<Array<{ key: string; value: string }>>(() => {
    try {
      const parsed = JSON.parse(metric.value_mappings || '{}');
      return Object.entries(parsed).map(([key, value]) => ({ key, value: value as string }));
    } catch {
      return [];
    }
  });

  interface TableInstanceItem {
    rowNum: number;
    rawIndex: string;
    displayIndex: string;
    fullOid: string;
    value: string;
    hasValue: boolean;
  }

  const rawTableParts = useMemo(() => {
    if (metric.last_value && typeof metric.last_value === 'string' && metric.last_value.includes(';')) {
      return metric.last_value.split(';').map((s: string) => s.trim()).filter(Boolean);
    }
    return [];
  }, [metric.last_value]);

  const parsedInstances = useMemo<TableInstanceItem[]>(() => {
    if (rawTableParts.length === 0) return [];
    const baseOidClean = (metric.oid || '').trim().replace(/^\.+/, '').replace(/\.+$/, '');
    return rawTableParts.map((part: string, idx: number) => {
      let rawIndex = String(idx + 1);
      let val = part;
      if (part.includes('=')) {
        const eqIdx = part.indexOf('=');
        rawIndex = part.substring(0, eqIdx).trim();
        val = part.substring(eqIdx + 1).trim();
      }
      let displayIndex = rawIndex;
      let fullOid = '';
      if (rawIndex.startsWith('.') || rawIndex.startsWith('1.')) {
        const cleanRaw = rawIndex.replace(/^\.+/, '');
        fullOid = '.' + cleanRaw;
        if (baseOidClean && cleanRaw.startsWith(baseOidClean + '.')) {
          displayIndex = cleanRaw.substring(baseOidClean.length + 1);
        } else {
          const segs = cleanRaw.split('.');
          displayIndex = segs[segs.length - 1] || cleanRaw;
        }
      } else {
        displayIndex = rawIndex;
        fullOid = '.' + (baseOidClean ? `${baseOidClean}.${rawIndex}` : rawIndex);
      }
      const num = parseFloat(val);
      const hasValue = (!isNaN(num) && num !== 0) || (val !== '' && val !== '0' && val !== '0.0' && val !== 'N/A');
      return { rowNum: idx + 1, rawIndex, displayIndex, fullOid, value: val, hasValue };
    });
  }, [rawTableParts, metric.oid]);

  const [currentOid, setCurrentOid] = useState(metric.oid || '');
  const [selectedInstanceRow, setSelectedInstanceRow] = useState<number | null>(() => {
    if (parsedInstances.length === 0) return null;
    const firstWithVal = parsedInstances.find((i: TableInstanceItem) => i.hasValue);
    return firstWithVal ? firstWithVal.rowNum : null;
  });

  const [confirmDelete, setConfirmDelete] = useState(false);
  const [showSendModal, setShowSendModal] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  // Tag 操作
  const addTag = () => {
    setTagList(prev => [...prev, { id: `tag-${Date.now()}-${Math.random().toString(36).slice(2, 5)}`, key: '', value: '' }]);
  };
  const removeTag = (id: string) => {
    setTagList(prev => prev.filter(t => t.id !== id));
  };
  const updateTag = (id: string, field: 'key' | 'value', val: string) => {
    setTagList(prev => prev.map(t => t.id === id ? { ...t, [field]: val } : t));
  };
  const applyPresetTag = (key: string, val: string) => {
    const existing = tagList.find(t => t.key === key);
    if (existing) {
      setTagList(prev => prev.map(t => t.key === key ? { ...t, value: val } : t));
    } else {
      setTagList(prev => [...prev, { id: `tag-${Date.now()}`, key, value: val }]);
    }
  };

  const autoGenerateMetricName = () => {
    const cn = customName.toLowerCase();
    if (cn.includes('cpu')) setMetricName('cpu_usage');
    else if (cn.includes('内存') || cn.includes('mem')) setMetricName('memory_usage');
    else if (cn.includes('状态') || cn.includes('status')) setMetricName('device_status');
    else if (cn.includes('温') || cn.includes('temp')) setMetricName('temperature_celsius');
    else if (cn.includes('风扇') || cn.includes('fan')) setMetricName('fan_status');
    else if (cn.includes('电源') || cn.includes('power')) setMetricName('power_status');
    else {
      const clean = cn.replace(/[^a-zA-Z0-9_]/g, '_').replace(/^_+|_+$/g, '');
      setMetricName(clean || 'custom_metric');
    }
  };

  // 值映射操作
  const addMapping = () => setMappings([...mappings, { key: '', value: '' }]);
  const removeMapping = (i: number) => setMappings(mappings.filter((_, idx) => idx !== i));
  const updateMapping = (i: number, field: 'key' | 'value', val: string) => {
    setMappings(mappings.map((m, idx) => idx === i ? { ...m, [field]: val } : m));
  };

  const handleSave = async (e?: React.MouseEvent) => {
    if (e) e.preventDefault();
    if (isSaving) return;

    const mappingObj: Record<string, string> = {};
    for (const m of mappings) {
      if (m.key.trim()) mappingObj[m.key.trim()] = m.value.trim();
    }

    const tagObj: Record<string, string> = {};
    for (const t of tagList) {
      if (t.key.trim()) tagObj[t.key.trim()] = t.value.trim();
    }

    const chosenInst = selectedInstanceRow ? parsedInstances.find((i: TableInstanceItem) => i.rowNum === selectedInstanceRow) : null;
    const updates: any = {
      custom_name: customName.trim(),
      metric_name: metricName.trim() || 'custom_metric',
      tags: Object.keys(tagObj).length > 0 ? JSON.stringify(tagObj) : '',
      unit: unit.trim(),
      chart_type: chartType,
      value_mappings: Object.keys(mappingObj).length > 0 ? JSON.stringify(mappingObj) : '',
    };
    if (currentOid && currentOid !== metric.oid) {
      updates.oid = currentOid.trim();
    }
    if (chosenInst) {
      updates.oid = chosenInst.fullOid;
      updates.is_table = 0;
      updates.last_value = chosenInst.value;
    }

    setIsSaving(true);
    try {
      await onSave(updates);
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async (e?: React.MouseEvent) => {
    if (e) e.preventDefault();
    if (isDeleting) return;
    setIsDeleting(true);
    try {
      await onDelete();
    } finally {
      setIsDeleting(false);
    }
  };

  // 整理有效 Tag 列表用于实时预览
  const validTags = tagList.filter(t => t.key.trim());
  const formattedMetricName = (metricName.trim() || 'custom_metric').replace(/[^a-zA-Z0-9_]/g, '_');

  return (
    <div
      className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={onClose}
    >
      <div
        className="bg-[#111836] border border-blue-500/30 rounded-xl w-full max-w-xl max-h-[90vh] flex flex-col shadow-2xl shadow-black/50 overflow-hidden"
        onClick={e => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-blue-500/20 bg-[#0d1225] flex-shrink-0">
          <div className="flex items-center gap-2">
            <Settings className="w-4 h-4 text-blue-400" />
            <h3 className="text-sm font-bold text-slate-100">
              编辑指标属性与 Tag 标签
            </h3>
            <span className="text-[10px] px-2 py-0.5 rounded bg-blue-500/15 text-blue-300 border border-blue-500/30">
              Prometheus & Tag 视图同步
            </span>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="p-1 hover:bg-slate-800 text-slate-400 hover:text-white rounded-md transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto p-5 space-y-4">
          {/* OID 信息 Banner */}
          <div className="p-3 bg-[#0d1225] rounded-xl border border-blue-500/20 space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-[10px] uppercase font-bold text-blue-400">OID 采样节点:</span>
              <span className="text-[10px] text-slate-500 font-mono">ID: {metric.id?.slice(0, 8)}...</span>
            </div>
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={currentOid}
                onChange={e => setCurrentOid(e.target.value)}
                placeholder="OID 节点"
                className="w-full px-2.5 py-1.5 bg-[#080c17] border border-blue-500/30 rounded-lg text-xs font-mono text-cyan-300 focus:outline-none focus:border-cyan-400"
              />
            </div>
            <p className="text-[11px] text-slate-400">原始名称: {metric.original_name || metric.custom_name || '-'}</p>

            {/* 如果检测到表量多实例数据，显示实例切换器 */}
            {parsedInstances.length > 0 && (
              <div className="pt-2 border-t border-slate-800 space-y-2">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold text-purple-300 flex items-center gap-1.5">
                    🎯 表量多实例切换 (共 {parsedInstances.length} 个实例)
                  </span>
                  <span className="text-[10px] text-slate-400">点击切换要在大屏绑定的实例</span>
                </div>
                <div className="max-h-36 overflow-y-auto rounded-lg border border-slate-800 bg-[#080c17] divide-y divide-slate-800/60">
                  {parsedInstances.map((inst: TableInstanceItem) => {
                    const isSelected = selectedInstanceRow === inst.rowNum || currentOid === inst.fullOid;
                    return (
                      <div
                        key={inst.rowNum}
                        onClick={() => {
                          setSelectedInstanceRow(inst.rowNum);
                          setCurrentOid(inst.fullOid);
                        }}
                        className={`flex items-center justify-between px-2.5 py-1.5 text-xs font-mono cursor-pointer transition-colors ${
                          isSelected ? 'bg-purple-900/40 border-l-2 border-purple-400 text-white' : 'hover:bg-slate-800/60 text-slate-300'
                        }`}
                      >
                        <div className="flex items-center gap-2">
                          <span className="text-slate-400">#{inst.rowNum} (索引: {inst.displayIndex})</span>
                          {inst.hasValue && (
                            <span className="px-1 py-0.2 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400">★ 有效值</span>
                          )}
                        </div>
                        <div className="flex items-center gap-2">
                          <span className={`font-bold ${inst.hasValue ? 'text-emerald-400' : 'text-slate-500'}`}>{inst.value} {unit}</span>
                          {isSelected && <span className="text-[10px] text-purple-300 font-bold">已选</span>}
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>

          {/* 1. 自定义指标大屏显示名称 */}
          <div>
            <label className="block text-xs font-semibold text-slate-200 mb-1 flex items-center justify-between">
              <span>自定义指标显示名称 <span className="text-red-400">*</span></span>
              <span className="text-[10px] text-slate-400 font-normal">在大屏卡片与拓扑图上直观呈现</span>
            </label>
            <input
              type="text"
              value={customName}
              onChange={e => {
                const newName = e.target.value;
                setCustomName(newName);
                // 自动更新 tagList 里的 metric_label
                setTagList(prev => {
                  const hasLabel = prev.some(t => t.key === 'metric_label');
                  if (hasLabel) {
                    return prev.map(t => t.key === 'metric_label' ? { ...t, value: newName } : t);
                  }
                  return prev;
                });
              }}
              placeholder="例：CPU使用率、内存占用、核心交换机-1 CPU利用率"
              className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-blue-500"
            />
          </div>

          {/* 2. Prometheus 英文指标标识 (Metric Name) */}
          <div className="p-3 bg-[#0d1225] rounded-xl border border-blue-500/20 space-y-2">
            <div className="flex items-center justify-between">
              <label className="text-xs font-semibold text-slate-200 flex items-center gap-1.5">
                <Tag className="w-3.5 h-3.5 text-cyan-400" />
                <span>Prometheus 指标英文标识 (Metric Name)</span>
              </label>
              <button
                type="button"
                onClick={autoGenerateMetricName}
                className="flex items-center gap-1 px-2 py-0.5 text-[11px] text-cyan-400 hover:text-cyan-300 hover:bg-cyan-500/10 rounded transition-colors"
                title="根据上方显示名称自动生成英文标识"
              >
                <Sparkles className="w-3 h-3" />
                自动生成/同步
              </button>
            </div>
            <input
              type="text"
              value={metricName}
              onChange={e => setMetricName(e.target.value.toLowerCase().replace(/[^a-zA-Z0-9_]/g, '_'))}
              placeholder="例：cpu_usage, memory_usage, switch_cpu_percent"
              className="w-full px-3 py-1.5 bg-[#080c17] border border-blue-500/30 rounded-lg text-xs font-mono text-cyan-300 focus:outline-none focus:border-cyan-400"
            />
            <div className="flex flex-wrap items-center gap-1.5 pt-1">
              <span className="text-[10px] text-slate-500">快捷预设:</span>
              {METRIC_NAME_PRESETS.map((p) => (
                <button
                  key={p.value}
                  type="button"
                  onClick={() => setMetricName(p.value)}
                  className={`px-2 py-0.5 text-[10px] font-mono rounded border transition-colors ${
                    metricName === p.value
                      ? 'bg-cyan-500/20 border-cyan-400 text-cyan-300'
                      : 'bg-[#080c17] border-slate-700 text-slate-400 hover:text-slate-200 hover:border-slate-600'
                  }`}
                  title={p.desc}
                >
                  {p.label}
                </button>
              ))}
            </div>
          </div>

          {/* 3. 自定义 Tag 标签列表 (Prometheus Labels Key-Value 键值对编辑) */}
          <div className="p-3 bg-[#0d1225] rounded-xl border border-blue-500/20 space-y-2.5">
            <div className="flex items-center justify-between">
              <label className="text-xs font-semibold text-slate-200 flex items-center gap-1.5">
                <Tag className="w-3.5 h-3.5 text-amber-400" />
                <span>自定义 Tag 标签键值对 (Prometheus Labels)</span>
              </label>
              <button
                type="button"
                onClick={addTag}
                className="flex items-center gap-1 px-2 py-0.5 text-xs text-amber-400 hover:bg-amber-500/10 rounded transition-colors font-medium"
              >
                <Plus className="w-3 h-3" />
                添加 Tag 标签
              </button>
            </div>
            <p className="text-[11px] text-slate-400">
              设置透出至 Prometheus 导出与 Tag 视图的自定义标签维度（如环境、机房、集群、角色等）
            </p>

            {/* Tag 快捷预设 */}
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="text-[10px] text-slate-500">常用标签:</span>
              {TAG_PRESETS.map((p) => (
                <button
                  key={p.key}
                  type="button"
                  onClick={() => applyPresetTag(p.key, p.value)}
                  className="px-2 py-0.5 text-[10px] font-mono rounded bg-[#080c17] border border-amber-500/20 text-amber-300/90 hover:bg-amber-500/15 hover:border-amber-500/40 transition-colors"
                >
                  + {p.label}
                </button>
              ))}
            </div>

            {/* Tag 输入行列表 */}
            {tagList.length === 0 ? (
              <div className="text-center py-2.5 text-xs text-slate-500 bg-[#080c17]/60 rounded-lg border border-dashed border-slate-700/50">
                暂无自定义 Tag 标签，点击上方“添加 Tag 标签”新增
              </div>
            ) : (
              <div className="space-y-2 pt-1">
                {tagList.map((tag) => (
                  <div key={tag.id} className="flex items-center gap-2">
                    <input
                      type="text"
                      value={tag.key}
                      onChange={e => updateTag(tag.id, 'key', e.target.value.toLowerCase().replace(/[^a-zA-Z0-9_]/g, '_'))}
                      placeholder="标签键 (如 env)"
                      className="w-1/3 px-2.5 py-1.5 bg-[#080c17] border border-amber-500/20 rounded text-xs text-amber-300 font-mono focus:outline-none focus:border-amber-400 placeholder:text-slate-600"
                    />
                    <span className="text-slate-500 text-xs font-mono">=</span>
                    <input
                      type="text"
                      value={tag.value}
                      onChange={e => updateTag(tag.id, 'value', e.target.value)}
                      placeholder="标签值 (如 prod, 核心交换机)"
                      className="flex-1 px-2.5 py-1.5 bg-[#080c17] border border-emerald-500/20 rounded text-xs text-emerald-300 font-mono focus:outline-none focus:border-emerald-400 placeholder:text-slate-600"
                    />
                    <button
                      type="button"
                      onClick={() => removeTag(tag.id)}
                      className="p-1 text-red-400/60 hover:text-red-400 hover:bg-red-500/10 rounded transition-colors"
                      title="删除该标签"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* 4. 实时 Prometheus Exposition & Tag 动态采集预览 */}
          <div className="p-3 bg-[#080c17] rounded-xl border border-cyan-500/30 space-y-1.5 font-mono text-xs">
            <div className="text-slate-400 text-[11px] flex items-center justify-between">
              <span className="flex items-center gap-1.5 text-slate-300">
                <Sparkles className="w-3.5 h-3.5 text-cyan-400" />
                <span>实时 Prometheus Exposition & Tag 采集预览</span>
              </span>
              <span className="px-1.5 py-0.5 text-[9px] font-bold rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">
                实时编译生效
              </span>
            </div>
            <div className="text-[11px] text-slate-400 leading-tight">
              # HELP {formattedMetricName} ({customName || '用户自定义标签'})<br />
              # TYPE {formattedMetricName} gauge
            </div>
            <div className="flex flex-wrap items-baseline gap-0.5 text-xs bg-[#050811] p-2.5 rounded-lg border border-slate-800">
              <span className="font-bold text-cyan-300">{formattedMetricName}</span>
              <span className="text-slate-400">&#123;</span>
              <span className="text-amber-400">device_ip</span>
              <span className="text-slate-400">=</span>
              <span className="text-emerald-300">"{metric.device_ip || '10.136.251.14'}"</span>
              {validTags.map((t) => (
                <span key={t.id} className="inline-flex items-baseline">
                  <span className="text-slate-400">, </span>
                  <span className="text-amber-300 font-bold ml-1">{t.key}</span>
                  <span className="text-slate-400">=</span>
                  <span className="text-emerald-300 font-bold">"{t.value || '-'}"</span>
                </span>
              ))}
              <span className="text-slate-400">&#125;</span>
              <span className="ml-2.5 font-bold text-blue-300">{cleanDisplayValue(metric.last_value) || '10.00'}</span>
            </div>
          </div>

          {/* 5. 自定义单位 / 符号 */}
          <div>
            <label className="block text-xs font-semibold text-slate-200 mb-1 flex items-center justify-between">
              <span>自定义单位 / 符号 <span className="text-slate-500 font-normal">(如 %、℃、Mbps、个、RPM)</span></span>
            </label>
            <input
              type="text"
              value={unit}
              onChange={e => setUnit(e.target.value)}
              placeholder="如 %、℃、MB、Gbps"
              className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-blue-500"
            />
            <div className="flex flex-wrap gap-1 mt-1.5">
              {UNIT_PRESETS.map((u) => (
                <button
                  key={u}
                  type="button"
                  onClick={() => setUnit(u)}
                  className={`px-2 py-0.5 text-xs rounded border transition-colors ${
                    unit === u
                      ? 'bg-blue-500/30 border-blue-400 text-blue-200'
                      : 'bg-[#0d1225] border-blue-500/15 text-slate-400 hover:text-slate-200'
                  }`}
                >
                  {u}
                </button>
              ))}
            </div>
          </div>

          {/* 6. 大屏展现样式 */}
          <div>
            <label className="block text-xs font-semibold text-slate-200 mb-1.5">
              大屏展现样式
            </label>
            <div className="grid grid-cols-3 gap-2">
              <button
                type="button"
                onClick={() => setChartType('circle')}
                className={`px-2.5 py-2 rounded-lg border text-xs font-bold flex items-center justify-center gap-1 transition-all cursor-pointer ${
                  chartType === 'circle' || chartType === 'auto'
                    ? 'bg-blue-500/25 border-blue-400 text-blue-300 shadow-md shadow-blue-500/10'
                    : 'bg-[#0d1225] border-blue-500/20 text-slate-400 hover:text-slate-200'
                }`}
              >
                <span>🍩 环形圆盘</span>
              </button>
              <button
                type="button"
                onClick={() => setChartType('bar')}
                className={`px-2.5 py-2 rounded-lg border text-xs font-bold flex items-center justify-center gap-1 transition-all cursor-pointer ${
                  chartType === 'bar'
                    ? 'bg-blue-500/25 border-blue-400 text-blue-300 shadow-md shadow-blue-500/10'
                    : 'bg-[#0d1225] border-blue-500/20 text-slate-400 hover:text-slate-200'
                }`}
              >
                <span>📊 柱状进度</span>
              </button>
              <button
                type="button"
                onClick={() => setChartType('number')}
                className={`px-2.5 py-2 rounded-lg border text-xs font-bold flex items-center justify-center gap-1 transition-all cursor-pointer ${
                  chartType === 'number'
                    ? 'bg-blue-500/25 border-blue-400 text-blue-300 shadow-md shadow-blue-500/10'
                    : 'bg-[#0d1225] border-blue-500/20 text-slate-400 hover:text-slate-200'
                }`}
              >
                <span>🔢 数字卡片</span>
              </button>
            </div>
          </div>

          {/* 7. Grafana 风格值映射 */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-xs font-semibold text-slate-200">Grafana 风格值映射</label>
              <button
                type="button"
                onClick={addMapping}
                className="flex items-center gap-1 px-2 py-0.5 text-xs text-blue-400 hover:bg-blue-500/10 rounded transition-colors"
              >
                <Plus className="w-3 h-3" />
                添加规则
              </button>
            </div>
            <p className="text-[11px] text-slate-400 mb-2">将采集出的数字映射为直观文本（例：1 → 正常，2 → 告警，0 → 离线）</p>
            {mappings.length === 0 ? (
              <div className="text-center py-3 text-xs text-slate-500 bg-[#0d1225]/60 rounded-lg border border-dashed border-slate-700/50">
                暂未设置值映射规则
              </div>
            ) : (
              <div className="space-y-2">
                {mappings.map((m, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <input
                      type="text"
                      value={m.key}
                      onChange={e => updateMapping(i, 'key', e.target.value)}
                      placeholder="采集原始值 (如 1)"
                      className="w-24 px-2.5 py-1.5 bg-[#0d1225] border border-blue-500/20 rounded text-xs text-slate-200 text-center font-mono focus:outline-none focus:border-blue-500"
                    />
                    <span className="text-slate-500 text-xs">→</span>
                    <input
                      type="text"
                      value={m.value}
                      onChange={e => updateMapping(i, 'value', e.target.value)}
                      placeholder="大屏映射文本 (如 正常)"
                      className="flex-1 px-2.5 py-1.5 bg-[#0d1225] border border-blue-500/20 rounded text-xs text-slate-200 focus:outline-none focus:border-blue-500"
                    />
                    <button
                      type="button"
                      onClick={() => removeMapping(i)}
                      className="p-1 text-red-400/60 hover:text-red-400 hover:bg-red-500/10 rounded transition-colors"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Footer 操作按钮 - 固定吸底 */}
        <div className="flex items-center justify-between px-5 py-3.5 border-t border-slate-700/50 bg-[#0d1225] flex-shrink-0">
          <div>
            {confirmDelete ? (
              <div className="flex items-center gap-2">
                <span className="text-xs text-red-400 font-medium">确认删除？</span>
                <button
                  type="button"
                  onClick={handleDelete}
                  disabled={isDeleting}
                  className="px-2.5 py-1 text-xs bg-red-500 text-white rounded hover:bg-red-600 font-medium disabled:opacity-50"
                >
                  {isDeleting ? '删除中...' : '确定删除'}
                </button>
                <button
                  type="button"
                  onClick={() => setConfirmDelete(false)}
                  className="px-2 py-1 text-xs text-slate-400 hover:text-slate-200"
                >
                  取消
                </button>
              </div>
            ) : (
              <button
                type="button"
                onClick={() => setConfirmDelete(true)}
                className="flex items-center gap-1 px-2.5 py-1.5 text-xs text-red-400 hover:bg-red-500/10 rounded-md transition-colors font-medium"
              >
                <Trash2 className="w-3.5 h-3.5" />
                删除指标
              </button>
            )}
          </div>

          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={() => setShowSendModal(true)}
              className="flex items-center gap-1 px-3 py-1.5 bg-blue-500/15 text-blue-300 border border-blue-500/30 text-xs font-semibold rounded-lg hover:bg-blue-500/25 transition-all cursor-pointer"
            >
              <Zap className="w-3.5 h-3.5 text-amber-400" />
              复用到多台设备
            </button>
            <button
              type="button"
              onClick={onClose}
              className="px-3.5 py-1.5 text-xs text-slate-300 hover:text-white bg-slate-800/80 hover:bg-slate-700 rounded-lg transition-colors cursor-pointer"
            >
              取消
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={isSaving}
              className="flex items-center gap-1.5 px-4 py-1.5 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600 text-white text-xs font-bold rounded-lg shadow-lg shadow-blue-500/20 transition-all cursor-pointer disabled:opacity-50"
            >
              {isSaving && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              {isSaving ? '保存中...' : '保存修改'}
            </button>
          </div>
        </div>
      </div>

      {showSendModal && (
        <SendToDashboardModal
          initialData={{
            oid: metric.oid,
            customName: customName || metric.custom_name,
            metricName: metricName,
            tags: Object.keys(tagList.reduce((acc, t) => {
              if (t.key.trim()) acc[t.key.trim()] = t.value.trim();
              return acc;
            }, {} as Record<string, string>)).length > 0
              ? JSON.stringify(tagList.reduce((acc, t) => {
                  if (t.key.trim()) acc[t.key.trim()] = t.value.trim();
                  return acc;
                }, {} as Record<string, string>))
              : '',
            deviceId: metric.device_id,
            lastValue: metric.last_value,
            unit,
            snmpVersion: metric.snmp_version,
            community: metric.community,
            is_table: metric.is_table,
          }}
          onClose={() => setShowSendModal(false)}
          onSuccess={() => {
            setShowSendModal(false);
            onClose();
          }}
        />
      )}
    </div>
  );
}

// ============================================================
// SNMP 可配置指标面板（dashboard_panels）— Grafana 风格
// ============================================================

interface PanelConfig {
  id: string;
  title: string;
  metric_name: string;
  aggregation: string;
  chart_type: string;
  time_range: string;
  tag_filters: string;
  group_by: string;
  enabled: number;
}

interface SeriesPoint {
  timestamp: string;
  value: number;
}

interface SeriesData {
  device_id: string;
  tags?: Record<string, string>;
  points: SeriesPoint[];
}

const PANEL_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#84cc16'];

function SnmpMetricPanelsSection() {
  const { data: panels = [], isLoading } = useQuery<PanelConfig[]>({
    queryKey: ['snmp-metric-panels-dashboard'],
    queryFn: () => api.get('/snmp-metrics/panels').then(res => res.data.data || []),
    staleTime: 60000,
  });

  const enabledPanels = panels.filter(p => p.enabled !== 0);

  if (isLoading) {
    return (
      <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4">
        <div className="flex items-center justify-center py-8"><RefreshCw className="w-5 h-5 text-blue-400 animate-spin" /></div>
      </div>
    );
  }

  if (enabledPanels.length === 0) return null;

  return (
    <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4">
      <h3 className="text-sm font-medium text-slate-300 mb-4 flex items-center gap-2">
        <BarChart3 className="w-4 h-4 text-cyan-400" />
        SNMP 指标面板
        <span className="text-xs text-slate-500">({enabledPanels.length})</span>
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
        {enabledPanels.map(panel => (
          <SnmpPanelCard key={panel.id} panel={panel} />
        ))}
      </div>
    </div>
  );
}

function SnmpPanelCard({ panel }: { panel: PanelConfig }) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const { data, isFetching, error } = useQuery<SeriesData[]>({
    queryKey: ['snmp-metric-panel-chart', panel.id],
    queryFn: async () => {
      const tagFilters = safeJson<Record<string, string>>(panel.tag_filters, {});
      const groupBy = safeJson<string[]>(panel.group_by, []);
      const resp = await api.post('/snmp-metrics/panels/query', {
        metric_name: panel.metric_name,
        aggregation: panel.aggregation || 'avg',
        tag_filters: tagFilters,
        group_by: groupBy,
        time_range: panel.time_range || '24h',
      });
      return resp.data.data || [];
    },
    refetchInterval: 60000,
  });

  const series = data || [];
  const aggLabel = panel.aggregation === 'avg' ? '平均' : panel.aggregation === 'max' ? '最大' : panel.aggregation === 'min' ? '最小' : panel.aggregation === 'sum' ? '求和' : panel.aggregation;

  const displayTitle = (!panel.title || panel.title.includes('?') || panel.title.trim() === '')
    ? (panel.metric_name || 'SNMP 流量趋势')
    : panel.title;

  const handleDelete = async () => {
    try {
      await api.delete(`/snmp-metrics/panels/${panel.id}`);
      toast.success('面板已删除');
      queryClient.invalidateQueries({ queryKey: ['snmp-metric-panels-dashboard'] });
    } catch {
      toast.error('删除面板失败');
    }
  };

  return (
    <div className="group relative bg-[#0d1225] rounded-lg border border-blue-500/10 p-3 hover:border-blue-500/30 transition-colors">
      <div className="flex items-center justify-between mb-2">
        <div className="min-w-0 flex-1 pr-2">
          <p className="text-xs font-semibold text-slate-200 truncate">{displayTitle}</p>
          <p className="text-[10px] text-slate-500 font-mono truncate">{panel.metric_name}</p>
        </div>
        <div className="flex items-center gap-1.5 flex-shrink-0">
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700/50 text-slate-400">{panel.time_range || '24h'}</span>
          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-700/50 text-slate-400">{aggLabel}</span>
          <button
            type="button"
            onClick={handleDelete}
            title="删除面板"
            className="p-1 opacity-0 group-hover:opacity-100 text-red-400/60 hover:text-red-400 hover:bg-red-500/10 rounded transition-all ml-1"
          >
            <Trash2 className="w-3 h-3" />
          </button>
        </div>
      </div>
      {isFetching && series.length === 0 ? (
        <div className="flex items-center justify-center py-8"><RefreshCw className="w-4 h-4 text-blue-400 animate-spin" /></div>
      ) : error ? (
        <div className="flex items-center justify-center py-6 text-xs text-slate-500">查询失败</div>
      ) : series.length === 0 ? (
        <div className="flex items-center justify-center py-6 text-xs text-slate-500">暂无数据</div>
      ) : (
        <PanelSvgChart series={series} chartType={panel.chart_type || 'line'} />
      )}
    </div>
  );
}

function PanelSvgChart({ series, chartType }: { series: SeriesData[]; chartType: string }) {
  const w = 320, h = 120, pad = 28;
  const allPoints = series.flatMap(s => s.points);
  if (allPoints.length < 2) return <div className="text-center py-4 text-xs text-slate-500">数据点不足</div>;

  const values = allPoints.map(p => p.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = (max - min) || 1;
  const times = allPoints.map(p => new Date(p.timestamp).getTime());
  const tMin = Math.min(...times);
  const tMax = Math.max(...times);
  const tRange = (tMax - tMin) || 1;
  const xOf = (t: number) => pad + ((t - tMin) / tRange) * (w - pad * 2);
  const yOf = (v: number) => h - pad - ((v - min) / range) * (h - pad * 2);
  const yTicks = [0, 0.5, 1].map(t => ({ y: h - pad - t * (h - pad * 2), val: min + t * range }));

  if (chartType === 'bar') {
    const latest = series.map(s => s.points[s.points.length - 1]).filter(Boolean);
    const bw = (w - pad * 2) / Math.max(latest.length, 1) - 6;
    return (
      <svg width={w} height={h} className="block w-full" viewBox={`0 0 ${w} ${h}`}>
        {yTicks.map((t, i) => <g key={i}><line x1={pad} y1={t.y} x2={w - pad} y2={t.y} stroke="#1e293b" strokeWidth="0.5" /><text x={pad - 4} y={t.y + 3} fill="#475569" fontSize="8" textAnchor="end">{fmtNum(t.val)}</text></g>)}
        {latest.map((p, i) => {
          const x = pad + i * ((w - pad * 2) / latest.length) + 3;
          const y = yOf(p.value);
          const bh = (h - pad) - y;
          return <rect key={i} x={x} y={y} width={Math.max(bw, 4)} height={Math.max(bh, 1)} fill={PANEL_COLORS[i % PANEL_COLORS.length]} opacity="0.8" rx="2" />;
        })}
      </svg>
    );
  }

  return (
    <svg width={w} height={h} className="block w-full" viewBox={`0 0 ${w} ${h}`}>
      {yTicks.map((t, i) => <g key={i}><line x1={pad} y1={t.y} x2={w - pad} y2={t.y} stroke="#1e293b" strokeWidth="0.5" /><text x={pad - 4} y={t.y + 3} fill="#475569" fontSize="8" textAnchor="end">{fmtNum(t.val)}</text></g>)}
      {series.map((s, si) => {
        const color = PANEL_COLORS[si % PANEL_COLORS.length];
        const pts = [...s.points].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
        const path = `M ${pts.map(p => `${xOf(new Date(p.timestamp).getTime())},${yOf(p.value)}`).join(' L ')}`;
        const label = s.tags?.ifDescr || s.tags?.ifName || s.device_id || '';
        return (
          <g key={si}>
            <path d={path} fill="none" stroke={color} strokeWidth="1.5" />
            {label && pts[0] && <text x={xOf(new Date(pts[0].timestamp).getTime())} y={yOf(pts[0].value) - 4} fill={color} fontSize="7">{label.length > 12 ? label.slice(0, 12) + '..' : label}</text>}
          </g>
        );
      })}
    </svg>
  );
}

function safeJson<T>(s: string, fb: T): T {
  if (!s || !s.trim()) return fb;
  try { return JSON.parse(s) as T; } catch { return fb; }
}

function fmtNum(n: number): string {
  if (Math.abs(n) >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (Math.abs(n) >= 1e6) return (n / 1e6).toFixed(2) + 'M';
  if (Math.abs(n) >= 1e3) return (n / 1e3).toFixed(2) + 'K';
  if (Number.isInteger(n)) return n.toString();
  return n.toFixed(2);
}

// ============================================================
// 子组件
// ============================================================

function KpiCard({ icon, label, value, suffix, unit, color }: {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  suffix?: string;
  unit?: string;
  color: string;
}) {
  return (
    <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4 hover:border-blue-500/30 transition-colors">
      <div className="flex items-center gap-2 mb-2">
        <span style={{ color }}>{icon}</span>
        <span className="text-xs text-slate-400">{label}</span>
      </div>
      <div className="flex items-baseline gap-1">
        <span className="text-2xl font-bold" style={{ color }}>{value}</span>
        {suffix && <span className="text-sm text-slate-500">{suffix}</span>}
        {unit && <span className="text-xs text-slate-500">{unit}</span>}
      </div>
    </div>
  );
}

// ============================================================
// 工具函数
// ============================================================

function formatUptime(dateStr: string): string {
  try {
    const start = new Date(dateStr);
    const now = new Date();
    const diffMs = now.getTime() - start.getTime();
    if (diffMs < 0) return '-';
    const days = Math.floor(diffMs / 86400000);
    if (days > 0) return `${days}天`;
    const hours = Math.floor(diffMs / 3600000);
    if (hours > 0) return `${hours}小时`;
    return `${Math.floor(diffMs / 60000)}分钟`;
  } catch {
    return '-';
  }
}

function formatOctets(octets: number, speed: number): string {
  if (!octets || !Number.isFinite(octets) || octets === 0) return '-';
  // 将字节数转为可读的流量值
  if (octets > 1e12) return `${(octets / 1e12).toFixed(1)} TB`;
  if (octets > 1e9) return `${(octets / 1e9).toFixed(1)} GB`;
  if (octets > 1e6) return `${(octets / 1e6).toFixed(1)} MB`;
  if (octets > 1e3) return `${(octets / 1e3).toFixed(1)} KB`;
  return `${octets} B`;
}

function getUtilizationColor(utilization: number | undefined | null): string {
  if (utilization == null) return '#6b7280';
  if (utilization > 80) return '#ef4444';
  if (utilization > 60) return '#f59e0b';
  return '#22c55e';
}

// ============================================================
// 顶部网络设备接口指标概览组件 (模仿参考图样式 + 包含设备下拉选择)
// ============================================================

function TopInterfaceMetricsSection({ devices = [] }: { devices?: any[] }) {
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>('all');

  const { data: stats } = useQuery({
    queryKey: ['interface-stats-summary', selectedDeviceId],
    queryFn: () =>
      api.get('/network-devices/interface-stats-summary', {
        params: { device_id: selectedDeviceId },
      }).then(res => res.data.data || {}),
    refetchInterval: 15000,
  });

  const inMbps = stats?.inMbps ?? 0.0;
  const inMbpsTrend = stats?.inMbpsTrend ?? 0.0;
  const outMbps = stats?.outMbps ?? 0.0;
  const outMbpsTrend = stats?.outMbpsTrend ?? 0.0;
  const totalInVolumeGb = stats?.totalInVolumeGb ?? 0.0;
  const totalOutVolumeGb = stats?.totalOutVolumeGb ?? 0.0;
  const packetLossRate = stats?.packetLossRate ?? 0.0;
  const packetErrorRate = stats?.packetErrorRate ?? 0.0;

  return (
    <div className="bg-[#0b0f21] rounded-xl border border-blue-500/20 p-4 shadow-xl">
      {/* 头部标题与设备下拉菜单 */}
      <div className="flex flex-wrap items-center justify-between gap-3 mb-3.5 pb-2.5 border-b border-blue-500/10">
        <div className="flex items-center gap-2.5">
          <div className="p-1.5 rounded-lg bg-blue-500/20 text-blue-400">
            <Activity className="w-4 h-4" />
          </div>
          <div>
            <h3 className="text-sm font-bold text-slate-100 flex items-center gap-2">
              网络设备接口流量概览
            </h3>
            <p className="text-[11px] text-slate-400">
              下拉切换目标网络设备，实时查看上行/下行吞吐率、累计数据流量及包损率
            </p>
          </div>
        </div>

        {/* 设备选择下拉框 */}
        <div className="flex items-center gap-2">
          <span className="text-xs font-semibold text-slate-300">目标设备:</span>
          <select
            aria-label="选择目标设备"
            value={selectedDeviceId}
            onChange={e => setSelectedDeviceId(e.target.value)}
            className="px-3 py-1.5 bg-[#111836] border border-blue-500/30 rounded-lg text-xs text-slate-100 font-medium focus:outline-none focus:border-blue-500 cursor-pointer shadow-inner"
          >
            <option value="all">🌐 全局汇总 (全部网络设备)</option>
            {devices.map((d: any) => (
              <option key={d.id} value={d.id}>
                {d.name} ({d.ip || '未定IP'})
              </option>
            ))}
          </select>
        </div>
      </div>

      {/* 6 个指标卡片网格 (100% 还原参考图经典视觉) */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
        {/* 卡片 1: 总体上行吞吐量 (当前) */}
        <div className="bg-[#0d1225] border border-slate-800 rounded-lg p-3.5 flex flex-col justify-between hover:border-blue-500/30 transition-all shadow-md group">
          <div className="flex items-center justify-between text-slate-400 text-xs font-medium mb-2">
            <span className="truncate">总体上行吞吐量 (当前)</span>
            <span title="当前实时上行传输速率">
              <Info className="w-3.5 h-3.5 text-slate-500 group-hover:text-blue-400 cursor-help" />
            </span>
          </div>
          <div className="my-1 flex items-baseline gap-1">
            <span className="text-2xl font-black font-mono tracking-tight text-amber-400">
              {inMbps.toFixed(2)}
            </span>
            <span className="text-xs font-bold text-amber-400/90">Mb/s</span>
          </div>
          {inMbpsTrend !== 0 ? (
            <div className={`text-[11px] font-semibold flex items-center gap-0.5 mt-0.5 ${inMbpsTrend > 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              <span>{inMbpsTrend > 0 ? '↑' : '↓'}</span>
              <span>{Math.abs(inMbpsTrend)}%</span>
            </div>
          ) : (
            <div className="text-[11px] text-slate-500 mt-0.5">
              <span>0.00%</span>
            </div>
          )}
        </div>

        {/* 卡片 2: 总体下行吞吐量 (当前) */}
        <div className="bg-[#0d1225] border border-slate-800 rounded-lg p-3.5 flex flex-col justify-between hover:border-blue-500/30 transition-all shadow-md group">
          <div className="flex items-center justify-between text-slate-400 text-xs font-medium mb-2">
            <span className="truncate">总体下行吞吐量 (当前)</span>
            <span title="当前实时下行传输速率">
              <Info className="w-3.5 h-3.5 text-slate-500 group-hover:text-blue-400 cursor-help" />
            </span>
          </div>
          <div className="my-1 flex items-baseline gap-1">
            <span className="text-2xl font-black font-mono tracking-tight text-amber-400">
              {outMbps.toFixed(2)}
            </span>
            <span className="text-xs font-bold text-amber-400/90">Mb/s</span>
          </div>
          {outMbpsTrend !== 0 ? (
            <div className={`text-[11px] font-semibold flex items-center gap-0.5 mt-0.5 ${outMbpsTrend > 0 ? 'text-emerald-400' : 'text-red-400'}`}>
              <span>{outMbpsTrend > 0 ? '↑' : '↓'}</span>
              <span>{Math.abs(outMbpsTrend)}%</span>
            </div>
          ) : (
            <div className="text-[11px] text-slate-500 mt-0.5">
              <span>0.00%</span>
            </div>
          )}
        </div>

        {/* 卡片 3: 上行总流量 */}
        <div className="bg-[#0d1225] border border-slate-800 rounded-lg p-3.5 flex flex-col justify-between hover:border-blue-500/30 transition-all shadow-md group">
          <div className="flex items-center justify-between text-slate-400 text-xs font-medium mb-2">
            <span className="truncate">上行总流量</span>
            <span title="选定设备累计发送数据总量">
              <Info className="w-3.5 h-3.5 text-slate-500 group-hover:text-blue-400 cursor-help" />
            </span>
          </div>
          <div className="my-1 flex items-baseline gap-1">
            <span className="text-2xl font-black font-mono tracking-tight text-orange-400">
              {totalInVolumeGb.toFixed(2)}
            </span>
            <span className="text-xs font-bold text-orange-400/90">Gb</span>
          </div>
          <div className="h-4" />
        </div>

        {/* 卡片 4: 下行总流量 */}
        <div className="bg-[#0d1225] border border-slate-800 rounded-lg p-3.5 flex flex-col justify-between hover:border-blue-500/30 transition-all shadow-md group">
          <div className="flex items-center justify-between text-slate-400 text-xs font-medium mb-2">
            <span className="truncate">下行总流量</span>
            <span title="选定设备累计接收数据总量">
              <Info className="w-3.5 h-3.5 text-slate-500 group-hover:text-blue-400 cursor-help" />
            </span>
          </div>
          <div className="my-1 flex items-baseline gap-1">
            <span className="text-2xl font-black font-mono tracking-tight text-orange-400">
              {totalOutVolumeGb.toFixed(2)}
            </span>
            <span className="text-xs font-bold text-orange-400/90">Gb</span>
          </div>
          <div className="h-4" />
        </div>

        {/* 卡片 5: 当前设备丢包率 */}
        <div className="bg-[#0d1225] border border-slate-800 rounded-lg p-3.5 flex flex-col justify-between hover:border-blue-500/30 transition-all shadow-md group">
          <div className="flex items-center justify-between text-slate-400 text-xs font-medium mb-2">
            <span className="truncate">当前设备丢包率</span>
            <span title="数据包丢失比例">
              <Info className="w-3.5 h-3.5 text-slate-500 group-hover:text-blue-400 cursor-help" />
            </span>
          </div>
          <div className="my-1 flex items-baseline gap-1">
            <span className="text-2xl font-black font-mono tracking-tight text-emerald-400">
              {packetLossRate.toFixed(2)}%
            </span>
          </div>
          <div className="h-4" />
        </div>

        {/* 卡片 6: 当前设备错包率 */}
        <div className="bg-[#0d1225] border border-slate-800 rounded-lg p-3.5 flex flex-col justify-between hover:border-blue-500/30 transition-all shadow-md group">
          <div className="flex items-center justify-between text-slate-400 text-xs font-medium mb-2">
            <span className="truncate">当前设备错包率</span>
            <span title="数据包校验错误比例">
              <Info className="w-3.5 h-3.5 text-slate-500 group-hover:text-blue-400 cursor-help" />
            </span>
          </div>
          <div className="my-1 flex items-baseline gap-1">
            <span className="text-2xl font-black font-mono tracking-tight text-emerald-400">
              {packetErrorRate.toFixed(2)}%
            </span>
          </div>
          <div className="h-4" />
        </div>
      </div>
    </div>
  );
}

// ============================================================
// Top 流量设备 - 瀑布型流星雨粒子流光面板
// ============================================================

function WaterfallMeteorTopTrafficPanel({ topDevices = [] }: { topDevices?: any[] }) {
  const maxTraffic = useMemo(() => {
    if (!topDevices || topDevices.length === 0) return 1;
    return Math.max(...topDevices.map((d: any) => d.total_traffic_mbps || 1), 1);
  }, [topDevices]);

  if (!topDevices || topDevices.length === 0) {
    return (
      <div className="bg-[#111836] rounded-xl border border-blue-500/10 p-4">
        <h3 className="text-sm font-medium text-slate-300 mb-3 flex items-center gap-2">
          <Zap className="w-4 h-4 text-amber-400" />
          Top 流量设备
        </h3>
        <div className="flex items-center justify-center h-40 text-slate-500 text-sm">暂无数据</div>
      </div>
    );
  }

  return (
    <div className="bg-[#111836] rounded-xl border border-blue-500/15 p-4 flex flex-col justify-between shadow-xl relative overflow-hidden">
      {/* 头部标题 */}
      <div className="flex items-center justify-between mb-3.5">
        <h3 className="text-sm font-bold text-slate-100 flex items-center gap-2">
          <Zap className="w-4 h-4 text-amber-400 animate-pulse" />
          Top 流量设备
          <span className="text-[10px] font-semibold text-cyan-400 px-2 py-0.5 rounded-full bg-cyan-500/10 border border-cyan-500/20">
            瀑布流星极速带
          </span>
        </h3>
        <span className="text-xs text-slate-400 font-mono">TOP {topDevices.length}</span>
      </div>

      {/* 瀑布型流星雨设备列表 */}
      <div className="space-y-3 max-h-[175px] overflow-y-auto pr-1">
        {topDevices.slice(0, 5).map((d: any, idx: number) => {
          const val = d.total_traffic_mbps || 0;
          const pct = Math.min(Math.max((val / maxTraffic) * 100, 18), 100);

          const displayVal = val > 1000 ? `${(val / 1000).toFixed(2)} Gb/s` : `${val.toFixed(1)} Mb/s`;

          const rankColors = [
            { border: 'border-cyan-400/50', badge: 'bg-cyan-500/20 text-cyan-300', gradient: 'from-blue-600 via-cyan-400 to-emerald-400', meteor: 'bg-cyan-200 shadow-[0_0_8px_#22d3ee]' },
            { border: 'border-amber-400/50', badge: 'bg-amber-500/20 text-amber-300', gradient: 'from-amber-600 via-yellow-400 to-orange-400', meteor: 'bg-amber-200 shadow-[0_0_8px_#f59e0b]' },
            { border: 'border-purple-400/50', badge: 'bg-purple-500/20 text-purple-300', gradient: 'from-purple-600 via-indigo-400 to-cyan-400', meteor: 'bg-purple-200 shadow-[0_0_8px_#c084fc]' },
          ];

          const rankStyle = rankColors[idx] || {
            border: 'border-blue-500/20',
            badge: 'bg-slate-800 text-slate-400',
            gradient: 'from-blue-600 to-blue-400',
            meteor: 'bg-blue-300 shadow-[0_0_6px_#60a5fa]',
          };

          return (
            <div key={d.device_id || idx} className="space-y-1 group">
              <div className="flex items-center justify-between text-xs">
                <div className="flex items-center gap-2 min-w-0">
                  <span className={`w-4 h-4 rounded flex items-center justify-center text-[10px] font-black font-mono border ${rankStyle.badge} ${rankStyle.border}`}>
                    {idx + 1}
                  </span>
                  <span className="font-bold text-slate-200 truncate max-w-[130px]" title={d.device_name || d.device_id}>
                    {d.device_name || d.device_id}
                  </span>
                  <span className="text-[10px] px-1.5 py-0.2 rounded text-slate-400 bg-slate-800/80 border border-slate-700/50">
                    {VENDOR_LABELS[(d.vendor || '').toLowerCase()] || d.vendor || '网络'}
                  </span>
                </div>
                <div className="flex items-center gap-1 font-mono font-bold text-xs text-amber-300">
                  <span>↑</span>
                  <span>{displayVal}</span>
                </div>
              </div>

              <div className="relative w-full h-3 bg-slate-900/90 rounded-full p-0.5 overflow-hidden border border-slate-700/40">
                <div
                  className={`h-full rounded-full bg-gradient-to-r ${rankStyle.gradient} transition-all duration-700 shadow-sm relative overflow-hidden`}
                  style={{ width: `${pct}%` }}
                >
                  <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/40 to-transparent animate-pulse" />
                  <div className={`absolute top-0 right-0 bottom-0 w-2 rounded-full ${rankStyle.meteor} animate-ping`} />
                </div>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
