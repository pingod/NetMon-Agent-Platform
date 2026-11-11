/* eslint-disable @typescript-eslint/no-explicit-any */
import React, { useState } from 'react';
import {
  X, AlertTriangle, AlertCircle, Info, CheckCircle2,
  Clock, Server, Tag, Code2, Copy, Check, ExternalLink,
  Zap, Wrench, Wifi, Sliders, ChevronDown, ChevronRight, Activity, Terminal
} from 'lucide-react';
import { safeFormatDistance } from '../../../../lib/date';
import { sanitizeText } from '../../../../lib/xss';
import { useToast } from '../../../../contexts/ToastContext';
import type { Alert } from './types';

interface AlertDetailModalProps {
  alert: Alert | null;
  onClose: () => void;
  onAcknowledge: (alertId: string) => void;
  onResolve: (alertId: string) => void;
  onProcess: (alertId: string) => void;
  navigate: (path: string) => void;
}

function parseMetadata(meta: unknown): Record<string, any> {
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

// 辅助提取告警内容中的触发值
function extractValueFromContent(content: string): string | null {
  if (!content) return null;
  // 匹配 "采样值为 10.00" 或 "采样值 10.00"
  const valMatch = content.match(/采样值(?:为)?\s*([0-9.]+)/);
  if (valMatch) return valMatch[1];
  // 匹配百分比 "91.0%"
  const pctMatch = content.match(/([0-9.]+%)/);
  if (pctMatch) return pctMatch[1];
  // 匹配 "当前值: 810" 或 "值: 810"
  const numMatch = content.match(/(?:当前值|值)\s*[:：]?\s*([0-9.]+)/);
  if (numMatch) return numMatch[1];
  return null;
}

export default function AlertDetailModal({
  alert,
  onClose,
  onAcknowledge,
  onResolve,
  onProcess,
  navigate,
}: AlertDetailModalProps) {
  const toast = useToast();
  const [copiedPromql, setCopiedPromql] = useState(false);
  const [showRawJson, setShowRawJson] = useState(false);

  if (!alert) return null;

  const metadata = parseMetadata(alert.metadata);

  // 提取设备信息
  const deviceName = metadata.device_name || metadata.deviceName || '';
  const deviceIp = metadata.device_ip || metadata.deviceIp || metadata.ip || '';
  const vendor = metadata.vendor || metadata.tags?.vendor || '';
  const deviceId = metadata.device_id || metadata.deviceId || '';

  // 提取指标与触发值
  const metricName = metadata.metric_name || metadata.metricName || '';
  const metricLabel = metadata.metric_label || metadata.metricLabel || '';
  const promql = metadata.promql || '';
  const tags: Record<string, string> = metadata.tags || {};

  // 提取具体数值
  let triggerValue: string | number | null = metadata.value ?? metadata.trigger_value ?? null;
  if (triggerValue === null) {
    triggerValue = extractValueFromContent(alert.content);
  }

  const handleCopyPromql = () => {
    if (promql) {
      navigator.clipboard.writeText(promql);
      setCopiedPromql(true);
      toast.success('PromQL 表达式已复制');
      setTimeout(() => setCopiedPromql(false), 2000);
    }
  };

  const getSeverityBadge = (severity: string) => {
    switch (severity) {
      case 'critical':
        return {
          label: 'P1 紧急 (Critical)',
          bg: 'bg-rose-500/15 border-rose-500/30 text-rose-600',
          icon: AlertCircle,
        };
      case 'high':
      case 'warning':
        return {
          label: 'P2 警告 (Warning)',
          bg: 'bg-amber-500/15 border-amber-500/30 text-amber-600',
          icon: AlertTriangle,
        };
      case 'medium':
      case 'low':
      case 'info':
      default:
        return {
          label: 'P3 提醒 (Info)',
          bg: 'bg-blue-500/15 border-blue-500/30 text-blue-600',
          icon: Info,
        };
    }
  };

  const sevBadge = getSeverityBadge(alert.severity);
  const SevIcon = sevBadge.icon;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 overflow-y-auto">
      <div className="bg-surface border border-border rounded-2xl max-w-2xl w-full p-6 shadow-2xl space-y-5 my-8 max-h-[92vh] overflow-y-auto">
        {/* 顶部标题与关闭按钮 */}
        <div className="flex items-start justify-between border-b border-border pb-4">
          <div className="space-y-1.5 min-w-0 flex-1 pr-4">
            <div className="flex items-center gap-2 flex-wrap">
              <span className={`inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-semibold border ${sevBadge.bg}`}>
                <SevIcon className="w-3.5 h-3.5" />
                {sevBadge.label}
              </span>

              <span className={`px-2.5 py-0.5 rounded-full text-xs font-semibold border ${
                alert.status === 'resolved'
                  ? 'bg-emerald-500/15 text-emerald-600 border-emerald-500/30'
                  : alert.status === 'acknowledged'
                  ? 'bg-amber-500/15 text-amber-600 border-amber-500/30'
                  : 'bg-rose-500/15 text-rose-600 border-rose-500/30'
              }`}>
                {alert.status === 'resolved' ? '已解决' : alert.status === 'acknowledged' ? '已确认' : '触发中 (Firing)'}
              </span>

              <span className="px-2 py-0.5 rounded bg-surface-2 text-text-secondary text-xs font-mono border border-border">
                来源: {alert.source || 'promql_alert'}
              </span>
            </div>

            <h2 className="text-lg font-bold text-text-primary leading-snug">
              {sanitizeText(alert.title)}
            </h2>

            <div className="flex items-center gap-3 text-xs text-text-secondary">
              <span className="flex items-center gap-1">
                <Clock className="w-3.5 h-3.5" />
                触发时间: {new Date(alert.created_at).toLocaleString()} ({safeFormatDistance(alert.created_at)})
              </span>
              <span>ID: <code className="font-mono text-text-tertiary">{alert.id}</code></span>
            </div>
          </div>

          <button
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-surface-2 text-text-secondary hover:text-text-primary transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 1. 🎯 触发指标与具体采样数值卡片 */}
        <div className="bg-slate-950/80 border border-slate-800 rounded-xl p-4 space-y-3 shadow-inner">
          <div className="flex items-center justify-between border-b border-slate-800/80 pb-2">
            <h3 className="text-xs font-bold uppercase tracking-wider text-emerald-400 flex items-center gap-1.5">
              <Activity className="w-4 h-4" /> 触发采样与指标详情
            </h3>
            {metricName && (
              <span className="px-2 py-0.5 rounded bg-purple-500/10 text-purple-300 font-mono text-xs border border-purple-500/20">
                {metricName}
              </span>
            )}
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
            {/* 具体触发值 */}
            <div className="bg-slate-900/80 border border-slate-800 rounded-lg p-3">
              <span className="text-[11px] text-slate-400 block mb-1">告警触发具体采样值</span>
              <div className="text-xl font-extrabold text-rose-400 font-mono flex items-baseline gap-1">
                {triggerValue !== null ? String(triggerValue) : '已触发'}
                {typeof triggerValue === 'number' && (metricName.includes('percent') || metricName.includes('usage') ? '%' : '')}
              </div>
            </div>

            {/* 触发指标 */}
            <div className="bg-slate-900/80 border border-slate-800 rounded-lg p-3">
              <span className="text-[11px] text-slate-400 block mb-1">监控指标 (Metric)</span>
              <div className="text-sm font-bold text-white font-mono truncate">
                {metricLabel || metricName || alert.title || '系统指标'}
              </div>
            </div>

            {/* 规则ID / 来源 */}
            <div className="bg-slate-900/80 border border-slate-800 rounded-lg p-3">
              <span className="text-[11px] text-slate-400 block mb-1">告警来源引擎</span>
              <div className="text-sm font-semibold text-purple-300">
                {alert.source === 'promql_alert' ? '夜莺 / PromQL 评估引擎' : alert.source}
              </div>
            </div>
          </div>

          {/* PromQL 表达式展示 */}
          {promql && (
            <div className="mt-2 p-2.5 rounded-lg bg-slate-900 border border-slate-800 font-mono text-xs flex items-center justify-between gap-3">
              <div className="flex items-center gap-2 min-w-0">
                <Code2 className="w-4 h-4 text-purple-400 shrink-0" />
                <span className="text-slate-400 text-[11px] shrink-0 font-sans font-medium">PromQL 表达式:</span>
                <span className="text-emerald-400 truncate select-all">{promql}</span>
              </div>
              <button
                onClick={handleCopyPromql}
                className="p-1 hover:bg-slate-800 rounded text-slate-400 hover:text-white shrink-0 transition-colors"
                title="复制 PromQL 表达式"
              >
                {copiedPromql ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
              </button>
            </div>
          )}
        </div>

        {/* 2. 🖥️ 关联设备与位置信息 */}
        {(deviceName || deviceIp || vendor || deviceId) && (
          <div className="bg-surface-2/40 border border-border rounded-xl p-4 space-y-2.5">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-bold uppercase tracking-wider text-blue-600 flex items-center gap-1.5">
                <Server className="w-4 h-4" /> 关联设备与资产信息
              </h3>
              <button
                onClick={() => navigate('/network-devices')}
                className="text-xs text-blue-600 hover:text-blue-700 flex items-center gap-1 transition-colors"
              >
                前往网络设备 <ExternalLink className="w-3 h-3" />
              </button>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
              <div className="p-2.5 bg-surface rounded-lg border border-border">
                <span className="text-text-secondary block mb-0.5">设备名称</span>
                <span className="font-semibold text-text-primary text-sm">{deviceName || '核心设备'}</span>
              </div>

              <div className="p-2.5 bg-surface rounded-lg border border-border">
                <span className="text-text-secondary block mb-0.5">IP 地址</span>
                <span className="font-mono font-semibold text-text-primary text-sm">{deviceIp || '未知'}</span>
              </div>

              {vendor && (
                <div className="p-2.5 bg-surface rounded-lg border border-border">
                  <span className="text-text-secondary block mb-0.5">厂商 / 品牌</span>
                  <span className="font-semibold text-text-primary uppercase">{vendor}</span>
                </div>
              )}

              {deviceId && (
                <div className="p-2.5 bg-surface rounded-lg border border-border">
                  <span className="text-text-secondary block mb-0.5">资产 ID</span>
                  <span className="font-mono text-text-secondary truncate block">{deviceId}</span>
                </div>
              )}
            </div>
          </div>
        )}

        {/* 3. 🏷️ 采集 Tag 标签全量维度 */}
        {tags && Object.keys(tags).length > 0 && (
          <div className="bg-surface-2/40 border border-border rounded-xl p-4 space-y-2.5">
            <h3 className="text-xs font-bold uppercase tracking-wider text-emerald-600 flex items-center gap-1.5">
              <Tag className="w-4 h-4" /> 采集端 Tag 标签全量维度
            </h3>

            <div className="flex flex-wrap gap-2">
              {Object.entries(tags).map(([key, val]) => (
                <div
                  key={key}
                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-surface border border-border text-xs font-mono"
                >
                  <span className="text-purple-600 font-semibold">{key}:</span>
                  <span className="text-emerald-600 font-medium">"{String(val)}"</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* 4. 📝 告警消息内容与原始元数据 */}
        <div className="space-y-2">
          <h3 className="text-xs font-bold uppercase tracking-wider text-text-secondary flex items-center gap-1.5">
            <Sliders className="w-3.5 h-3.5" /> 告警消息描述
          </h3>
          <div className="p-3 bg-surface-2/60 rounded-xl border border-border text-sm text-text-primary leading-relaxed font-sans">
            {sanitizeText(alert.content) || '无详细消息描述'}
          </div>

          {/* 原始元数据折叠展开 */}
          <div className="pt-1">
            <button
              type="button"
              onClick={() => setShowRawJson(!showRawJson)}
              className="text-xs text-text-secondary hover:text-text-primary flex items-center gap-1 transition-colors"
            >
              {showRawJson ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
              {showRawJson ? '收起原始元数据 (Raw JSON)' : '查看原始元数据 (Raw JSON)'}
            </button>

            {showRawJson && (
              <pre className="mt-2 p-3 bg-slate-950 rounded-xl border border-slate-800 text-[11px] font-mono text-slate-300 overflow-x-auto max-h-48 whitespace-pre-wrap leading-relaxed shadow-inner">
                {JSON.stringify(metadata, null, 2)}
              </pre>
            )}
          </div>
        </div>

        {/* 5. 底部快速操作栏 */}
        <div className="flex flex-wrap items-center justify-between gap-3 pt-4 border-t border-border">
          {/* 左侧快捷跳转 */}
          <div className="flex items-center gap-2 flex-wrap">
            <button
              onClick={() => {
                onClose();
                navigate(`/alert-auto-analysis?alertId=${alert.id}`);
              }}
              className="inline-flex items-center gap-1 px-3 py-1.5 bg-emerald-500/10 text-emerald-600 hover:bg-emerald-500/20 border border-emerald-500/30 rounded-lg text-xs font-medium transition-colors"
            >
              <Zap className="w-3.5 h-3.5" /> AI 根因分析
            </button>

            <button
              onClick={() => {
                onClose();
                navigate(`/remediation-executions?alertId=${alert.id}`);
              }}
              className="inline-flex items-center gap-1 px-3 py-1.5 bg-orange-500/10 text-orange-600 hover:bg-orange-500/20 border border-orange-500/30 rounded-lg text-xs font-medium transition-colors"
            >
              <Wrench className="w-3.5 h-3.5" /> 修复记录
            </button>

            <button
              onClick={() => {
                onClose();
                navigate(`/inspection-center?alertId=${alert.id}`);
              }}
              className="inline-flex items-center gap-1 px-3 py-1.5 bg-blue-500/10 text-blue-600 hover:bg-blue-500/20 border border-blue-500/30 rounded-lg text-xs font-medium transition-colors"
            >
              <Wifi className="w-3.5 h-3.5" /> 巡检结果
            </button>
          </div>

          {/* 右侧处置按钮 */}
          <div className="flex items-center gap-2">
            {alert.status === 'new' && (
              <button
                onClick={() => {
                  onAcknowledge(alert.id);
                  onClose();
                }}
                className="px-3.5 py-1.5 bg-amber-500/15 hover:bg-amber-500/25 border border-amber-500/30 text-amber-600 rounded-lg text-xs font-semibold transition-colors"
              >
                确认告警
              </button>
            )}

            {alert.status !== 'resolved' && (
              <button
                onClick={() => {
                  onResolve(alert.id);
                  onClose();
                }}
                className="px-3.5 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg text-xs font-semibold transition-colors shadow-sm"
              >
                标记已解决
              </button>
            )}

            <button
              onClick={onClose}
              className="px-4 py-1.5 bg-surface-2 border border-border text-text-secondary hover:text-text-primary rounded-lg text-xs font-medium transition-colors"
            >
              关闭
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
