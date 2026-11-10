import { X, AlertTriangle, Radio, Wifi, ArrowDown, ArrowUp, AlertCircle } from 'lucide-react';
import { useEscapeKey } from '../../../hooks/useEscapeKey';

interface SnmpInterfaceMetric {
  index: number;
  name: string;
  operStatus: 'up' | 'down';
  adminStatus: 'up' | 'down';
  speed: number;
  mtu: number;
  mac: string;
  inBps: number;
  outBps: number;
  inUtilization: number;
  outUtilization: number;
  inErrors: number;
  outErrors: number;
}

interface SnmpInspectionResultData {
  reachable: boolean;
  sysName: string;
  sysDescr: string;
  sysUptime: number;
  interfaces: SnmpInterfaceMetric[];
  interfaceCount: number;
  upCount: number;
  downCount: number;
  alerts: string[];
  pollDurationMs: number;
}

interface Props {
  result: SnmpInspectionResultData;
  deviceName: string;
  onClose: () => void;
}

function formatSpeed(speed: number): string {
  if (speed >= 1e9) return `${(speed / 1e9).toFixed(0)} Gbps`;
  if (speed >= 1e6) return `${(speed / 1e6).toFixed(0)} Mbps`;
  if (speed >= 1e3) return `${(speed / 1e3).toFixed(0)} Kbps`;
  return `${speed} bps`;
}

function formatRate(bps: number): string {
  if (bps >= 1e9) return `${(bps / 1e9).toFixed(2)} GB/s`;
  if (bps >= 1e6) return `${(bps / 1e6).toFixed(2)} MB/s`;
  if (bps >= 1e3) return `${(bps / 1e3).toFixed(1)} KB/s`;
  return `${bps} B/s`;
}

function formatUptime(ticks: number): string {
  const seconds = Math.floor(ticks / 100);
  if (seconds < 60) return `${seconds} 秒`;
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return `${d} 天 ${h} 小时 ${m} 分钟`;
}

export default function SnmpInspectionResult({ result, deviceName, onClose }: Props) {
  useEscapeKey({ onEscape: onClose });

  if (!result.reachable) {
    return (
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
        <div className="bg-surface border border-border rounded-xl shadow-2xl w-full max-w-md mx-4">
          <div className="flex items-center justify-between px-6 py-4 border-b border-border">
            <h3 className="text-base font-medium text-text-primary">SNMP 巡检 - {deviceName}</h3>
            <button onClick={onClose} className="text-text-secondary hover:text-text-primary"><X className="w-5 h-5" /></button>
          </div>
          <div className="p-6 text-center">
            <AlertTriangle className="w-12 h-12 text-red-500 mx-auto mb-3" />
            <p className="text-sm text-text-primary font-medium mb-1">SNMP 连接失败</p>
            {result.alerts.map((a, i) => (
              <p key={i} className="text-xs text-text-secondary mt-1">{a}</p>
            ))}
            <p className="text-xs text-text-secondary mt-3">耗时: {result.pollDurationMs}ms</p>
          </div>
          <div className="flex justify-end px-6 py-4 border-t border-border">
            <button onClick={onClose} className="px-4 py-2 text-sm bg-blue-600 text-white rounded-md">关闭</button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
      <div className="bg-surface border border-border rounded-xl shadow-2xl w-full max-w-3xl mx-4 max-h-[90vh] overflow-y-auto">
        {/* 头部 */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border sticky top-0 bg-surface rounded-t-xl z-10">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-emerald-500/10 flex items-center justify-center">
              <Radio className="w-4 h-4 text-emerald-400" />
            </div>
            <div>
              <h3 className="text-base font-medium text-text-primary">SNMP 巡检 - {deviceName}</h3>
              <p className="text-xs text-text-secondary">耗时 {result.pollDurationMs}ms</p>
            </div>
          </div>
          <button onClick={onClose} className="text-text-secondary hover:text-text-primary"><X className="w-5 h-5" /></button>
        </div>

        {/* 系统信息 */}
        <div className="p-6 border-b border-border">
          <div className="flex items-center gap-2 mb-3">
            <Radio className="w-4 h-4 text-emerald-400" />
            <span className="text-sm font-medium text-text-primary">系统信息</span>
            <span className="px-2 py-0.5 text-xs rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
              SNMP 可达
            </span>
          </div>
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div className="p-3 bg-background rounded-lg border border-border">
              <p className="text-xs text-text-secondary mb-1">设备名称</p>
              <p className="text-sm font-medium text-text-primary font-mono">{result.sysName || '-'}</p>
            </div>
            <div className="p-3 bg-background rounded-lg border border-border">
              <p className="text-xs text-text-secondary mb-1">运行时间</p>
              <p className="text-sm font-medium text-text-primary">{formatUptime(result.sysUptime)}</p>
            </div>
            <div className="p-3 bg-background rounded-lg border border-border col-span-2">
              <p className="text-xs text-text-secondary mb-1">系统描述</p>
              <p className="text-sm font-medium text-text-primary truncate" title={result.sysDescr}>
                {result.sysDescr || '-'}
              </p>
            </div>
          </div>
        </div>

        {/* 告警区域 */}
        {result.alerts.length > 0 && (
          <div className="px-6 py-4 border-b border-border bg-red-500/5">
            <div className="flex items-center gap-2 mb-2">
              <AlertTriangle className="w-4 h-4 text-red-400" />
              <span className="text-sm font-medium text-red-400">告警（{result.alerts.length}）</span>
            </div>
            <div className="space-y-1">
              {result.alerts.map((a, i) => (
                <div key={i} className="flex items-center gap-2 text-xs text-red-300">
                  <span>⚠️</span>
                  <span>{a}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* 接口列表 */}
        <div className="p-6">
          <div className="flex items-center justify-between mb-3">
            <div className="flex items-center gap-2">
              <Wifi className="w-4 h-4 text-blue-400" />
              <span className="text-sm font-medium text-text-primary">接口状态</span>
            </div>
            <div className="flex items-center gap-3 text-xs">
              <span className="text-text-secondary">共 {result.interfaceCount} 个</span>
              <span className="text-green-400">{result.upCount} UP</span>
              {result.downCount > 0 && <span className="text-red-400">{result.downCount} DOWN</span>}
            </div>
          </div>

          <div className="space-y-2 max-h-[400px] overflow-y-auto scrollbar-thin">
            {result.interfaces.map(iface => {
              const hasErrors = (iface.inErrors || 0) > 0 || (iface.outErrors || 0) > 0;
              const inUtilColor = iface.inUtilization > 80 ? 'from-red-500 to-red-400' : iface.inUtilization > 50 ? 'from-amber-500 to-amber-400' : 'from-blue-500 to-blue-400';
              const outUtilColor = iface.outUtilization > 80 ? 'from-red-500 to-red-400' : iface.outUtilization > 50 ? 'from-amber-500 to-amber-400' : 'from-cyan-500 to-cyan-400';
              return (
                <div
                  key={iface.index}
                  className={`p-3 bg-background rounded-lg border transition-colors ${
                    hasErrors ? 'border-amber-500/40 bg-amber-500/5' :
                    iface.operStatus === 'down' ? 'border-red-500/20 bg-red-500/5 opacity-60' :
                    'border-border hover:border-blue-500/30'
                  }`}
                >
                  {/* Interface header row */}
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <span className={`w-2 h-2 rounded-full flex-shrink-0 ${iface.operStatus === 'up' ? 'bg-green-500' : 'bg-red-500'}`} />
                      <span className="text-xs font-medium text-text-primary font-mono">{iface.name}</span>
                      {hasErrors && <AlertCircle className="w-3 h-3 text-amber-400" />}
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-[10px] text-text-secondary">{formatSpeed(iface.speed)}</span>
                      <span className={`text-[10px] px-1.5 py-0.5 rounded font-medium ${
                        iface.operStatus === 'up' ? 'text-green-400 bg-green-500/10' : 'text-red-400 bg-red-500/10'
                      }`}>
                        {iface.operStatus === 'up' ? 'UP' : 'DOWN'}
                      </span>
                    </div>
                  </div>

                  {/* Utilization bars */}
                  {iface.operStatus === 'up' && (
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <div className="flex items-center justify-between mb-1">
                          <div className="flex items-center gap-1">
                            <ArrowDown className="w-3 h-3 text-blue-400" />
                            <span className="text-[10px] text-text-secondary">入</span>
                          </div>
                          <span className="text-[10px] text-text-primary font-mono">{formatRate(iface.inBps)}</span>
                        </div>
                        <div className="h-1.5 bg-slate-700/50 rounded-full overflow-hidden">
                          <div className={`h-full bg-gradient-to-r ${inUtilColor} rounded-full transition-all duration-500`} style={{ width: `${Math.min(iface.inUtilization, 100)}%` }} />
                        </div>
                        <span className={`text-[10px] font-mono ${
                          iface.inUtilization > 80 ? 'text-red-400' : iface.inUtilization > 50 ? 'text-amber-400' : 'text-text-secondary'
                        }`}>{iface.inUtilization.toFixed(1)}%</span>
                      </div>
                      <div>
                        <div className="flex items-center justify-between mb-1">
                          <div className="flex items-center gap-1">
                            <ArrowUp className="w-3 h-3 text-cyan-400" />
                            <span className="text-[10px] text-text-secondary">出</span>
                          </div>
                          <span className="text-[10px] text-text-primary font-mono">{formatRate(iface.outBps)}</span>
                        </div>
                        <div className="h-1.5 bg-slate-700/50 rounded-full overflow-hidden">
                          <div className={`h-full bg-gradient-to-r ${outUtilColor} rounded-full transition-all duration-500`} style={{ width: `${Math.min(iface.outUtilization, 100)}%` }} />
                        </div>
                        <span className={`text-[10px] font-mono ${
                          iface.outUtilization > 80 ? 'text-red-400' : iface.outUtilization > 50 ? 'text-amber-400' : 'text-text-secondary'
                        }`}>{iface.outUtilization.toFixed(1)}%</span>
                      </div>
                    </div>
                  )}

                  {/* Error info */}
                  {hasErrors && (
                    <div className="mt-1.5 flex items-center gap-2 text-[10px] text-amber-400">
                      <AlertCircle className="w-3 h-3" />
                      <span>错误: 入 {iface.inErrors} / 出 {iface.outErrors || 0}</span>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        </div>

        {/* 底部 */}
        <div className="flex justify-end px-6 py-4 border-t border-border">
          <button onClick={onClose} className="px-4 py-2 text-sm bg-blue-600 text-white rounded-md">关闭</button>
        </div>
      </div>
    </div>
  );
}
