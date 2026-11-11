import { Bell, Play, Clock, Zap, Wrench, Search, Terminal, Wifi } from 'lucide-react';
import clsx from 'clsx';
import { safeFormatDistance } from '../../../../lib/date';
import { sanitizeText } from '../../../../lib/xss';
import type { Alert, AnalysisItem } from './types';

function getSeverityLabel(severity: string) {
  const labels: Record<string, string> = {
    critical: '严重',
    high: '高',
    medium: '中',
    low: '低',
  };
  return labels[severity] || severity;
}

interface AlertListProps {
  alerts: Alert[];
  analysisMap: Record<string, AnalysisItem>;
  onProcess: (alertId: string) => void;
  processPending: boolean;
  onAcknowledge: (alertId: string) => void;
  onResolve: (alertId: string) => void;
  navigate: (path: string) => void;
  onViewAutomationLog: (alert: Alert) => void;
  onSelectAlert: (alert: Alert) => void;
}

function parseAlertMeta(meta: unknown): Record<string, any> {
  if (!meta) return {};
  if (typeof meta === 'object') return meta as Record<string, any>;
  if (typeof meta === 'string') {
    try {
      return JSON.parse(meta);
    } catch {
      return {};
    }
  }
  return {};
}

export default function AlertList({
  alerts,
  analysisMap,
  onProcess,
  processPending,
  onAcknowledge,
  onResolve,
  navigate,
  onViewAutomationLog,
  onSelectAlert,
}: AlertListProps) {
  if (alerts.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <div className="p-4 rounded-xl bg-surface border border-border mb-3">
          <Bell className="w-8 h-8 text-text-secondary opacity-50" />
        </div>
        <p className="text-sm text-text-secondary mb-1">暂无告警</p>
        <p className="text-xs text-text-tertiary">系统运行正常，没有告警信息</p>
      </div>
    );
  }

  return (
    <div className="divide-y divide-border">
      {alerts.map((alert) => {
        const meta = parseAlertMeta(alert.metadata);
        const promql = meta.promql || '';
        const triggerValue = meta.value ?? meta.trigger_value ?? null;
        const deviceName = meta.device_name || '';
        const deviceIp = meta.device_ip || '';
        const tags: Record<string, string> = meta.tags || {};

        return (
          <div
            key={alert.id}
            onClick={() => onSelectAlert(alert)}
            className="p-6 hover:bg-background/60 transition-all cursor-pointer group"
          >
            <div className="flex items-start justify-between">
              <div className="flex-1 min-w-0 pr-4">
                <div className="flex items-center gap-3 mb-2 flex-wrap">
                  <span
                    className={clsx(
                      'px-2 py-0.5 rounded text-xs font-medium',
                      (alert.severity === 'critical') && 'bg-status-failed/10 text-status-failed border border-status-failed/20',
                      (alert.severity === 'high' || alert.severity === 'warning') && 'bg-status-warning/10 text-status-warning border border-status-warning/20',
                      (alert.severity === 'medium') && 'bg-primary/10 text-primary border border-primary/20',
                      (alert.severity === 'low' || alert.severity === 'info') && 'bg-status-pending/10 text-status-pending border border-status-pending/20'
                    )}
                  >
                    {getSeverityLabel(alert.severity)}
                  </span>
                  <span
                    className={clsx(
                      'px-2 py-0.5 rounded text-xs font-medium',
                      (alert.status === 'new' || alert.status === 'firing') && 'bg-status-failed/10 text-status-failed border border-status-failed/20',
                      alert.status === 'acknowledged' && 'bg-status-warning/10 text-status-warning border border-status-warning/20',
                      alert.status === 'resolved' && 'bg-status-success/10 text-status-success border border-status-success/20'
                    )}
                  >
                    {alert.status === 'new' && '新告警'}
                    {alert.status === 'firing' && '触发中 (Firing)'}
                    {alert.status === 'acknowledged' && '已确认'}
                    {alert.status === 'resolved' && '已解决'}
                  </span>

                  {/* 来源 */}
                  <span className="text-[11px] text-text-tertiary font-mono">
                    来源: {alert.source || 'promql_alert'}
                  </span>
                </div>

                <div className="flex items-center gap-2">
                  <h3 className="font-semibold text-text-primary mb-1 group-hover:text-blue-600 transition-colors">
                    {sanitizeText(alert.title)}
                  </h3>
                  <span className="text-xs text-blue-600 opacity-0 group-hover:opacity-100 transition-opacity flex items-center">
                    查看详情 &rarr;
                  </span>
                </div>

                <p className="text-sm text-text-secondary mb-2.5 line-clamp-2">{sanitizeText(alert.content)}</p>

                {/* 🎯 PromQL 表达式与具体触发值徽章条 */}
                {(promql || triggerValue !== null || deviceName || Object.keys(tags).length > 0) && (
                  <div className="mb-3 p-2 rounded-lg bg-slate-950/70 border border-slate-800/80 flex flex-wrap items-center gap-2.5 text-xs font-mono">
                    {triggerValue !== null && (
                      <span className="px-2 py-0.5 rounded bg-rose-500/20 text-rose-600 font-bold border border-rose-500/30">
                        触发采样值: {String(triggerValue)}
                      </span>
                    )}

                    {promql && (
                      <span className="text-emerald-600 truncate max-w-md">
                        <span className="text-text-tertiary font-sans">PromQL:</span> {promql}
                      </span>
                    )}

                    {(deviceName || deviceIp) && (
                      <span className="text-blue-600 font-sans">
                        设备: {deviceName} {deviceIp ? `(${deviceIp})` : ''}
                      </span>
                    )}

                    {/* Tag 预览 */}
                    {Object.entries(tags).slice(0, 3).map(([tk, tv]) => (
                      <span key={tk} className="px-1.5 py-0.2 rounded bg-surface-2 text-purple-600 text-[11px]">
                        {tk}={tv}
                      </span>
                    ))}
                  </div>
                )}

                <div className="flex items-center gap-4 text-xs text-text-secondary">
                  <span>
                    触发时间: {safeFormatDistance(alert.created_at)}
                  </span>
                  <span>ID: <code className="font-mono text-text-tertiary">{alert.id}</code></span>
                </div>
              </div>

              {/* 右侧操作按钮 */}
              <div className="flex items-center gap-2 ml-4 shrink-0" onClick={e => e.stopPropagation()}>
                <button
                  onClick={() => onSelectAlert(alert)}
                  className="px-3 py-1 text-xs bg-blue-600/10 text-blue-600 hover:bg-blue-600/20 border border-blue-500/30 rounded-lg transition-colors flex items-center gap-1 font-medium"
                >
                  <Search className="w-3.5 h-3.5" />
                  详情
                </button>

                {alert.status !== 'resolved' && (
                  <button
                    onClick={() => onProcess(alert.id)}
                    disabled={processPending}
                    className="px-3 py-1 text-xs bg-purple-600/10 text-purple-600 rounded-lg hover:bg-purple-600/20 transition-colors flex items-center gap-1"
                    title="手动触发匹配映射+修复策略+根因分析"
                  >
                    <Play className="w-3.5 h-3.5" />
                    处理
                  </button>
                )}

                {(alert.status === 'new' || alert.status === 'firing') && (
                  <button
                    onClick={() => onAcknowledge(alert.id)}
                    className="px-3 py-1 text-xs bg-status-warning/10 text-status-warning rounded-lg hover:bg-status-warning/20 border border-status-warning/30"
                  >
                    确认
                  </button>
                )}

                {alert.status !== 'resolved' && (
                  <button
                    onClick={() => onResolve(alert.id)}
                    className="px-3 py-1 text-xs bg-status-success/10 text-status-success rounded-lg hover:bg-status-success/20 border border-status-success/30"
                  >
                    解决
                  </button>
                )}
              </div>
            </div>

            {/* ── 关联操作 ── */}
            <div className="flex items-center gap-2 mt-3 pt-3 border-t border-border/50" onClick={e => e.stopPropagation()}>
              <button
                onClick={() => onSelectAlert(alert)}
                className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700 transition-colors px-2 py-1 rounded bg-blue-500/10 hover:bg-blue-500/20 font-medium"
              >
                <Search className="w-3 h-3" />
                告警详情
              </button>

              {analysisMap[alert.id] && (
                <button
                  onClick={() => navigate(`/alert-auto-analysis?alertId=${alert.id}`)}
                  className="flex items-center gap-1 text-xs text-emerald-600 hover:text-emerald-700 transition-colors px-2 py-1 rounded bg-emerald-500/5 hover:bg-emerald-500/10"
                >
                  <Zap className="w-3 h-3" />
                  AI 分析
                </button>
              )}
              <button
                onClick={() => navigate(`/remediation-executions?alertId=${alert.id}`)}
                className="flex items-center gap-1 text-xs text-orange-600 hover:text-orange-700 transition-colors px-2 py-1 rounded bg-orange-500/5 hover:bg-orange-500/10"
              >
                <Wrench className="w-3 h-3" />
                修复记录
              </button>
              <button
                onClick={() => onViewAutomationLog(alert)}
                className="flex items-center gap-1 text-xs text-text-secondary hover:text-text-primary transition-colors px-2 py-1 rounded bg-slate-500/10 hover:bg-slate-500/20"
              >
                <Clock className="w-3 h-3" />
                自动处理记录
              </button>
              {alert.related_task_id && (
                <button
                  onClick={() => navigate(`/tasks?taskId=${encodeURIComponent(alert.related_task_id!)}`)}
                  className="flex items-center gap-1 text-xs text-cyan-600 hover:text-cyan-700 transition-colors px-2 py-1 rounded bg-cyan-500/5 hover:bg-cyan-500/10"
                >
                  <Terminal className="w-3 h-3" />
                  工作流任务
                </button>
              )}
              <button
                onClick={() => navigate(`/inspection-center?alertId=${alert.id}`)}
                className="flex items-center gap-1 text-xs text-blue-600 hover:text-blue-700 transition-colors px-2 py-1 rounded bg-blue-500/5 hover:bg-blue-500/10"
              >
                <Wifi className="w-3 h-3" />
                巡检结果
              </button>
              <button
                onClick={() => navigate(`/root-cause-analysis?alertId=${alert.id}`)}
                className="flex items-center gap-1 text-xs text-purple-600 hover:text-purple-700 transition-colors px-2 py-1 rounded bg-purple-500/5 hover:bg-purple-500/10"
              >
                <Search className="w-3 h-3" />
                根因分析
              </button>
              {analysisMap[alert.id]?.status === 'completed' && (
                <span className="ml-auto text-xs text-emerald-500/60">
                  ✅ 已自动诊断
                </span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}