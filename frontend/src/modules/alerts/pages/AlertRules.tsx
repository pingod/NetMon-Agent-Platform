/* eslint-disable @typescript-eslint/no-explicit-any */
import React, { useState, useEffect, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Shield, Plus, Trash2, Edit, ToggleLeft, ToggleRight,
  AlertTriangle, AlertCircle, Info, X, Loader2, Save,
  Sparkles, Tag, Filter, Clock, Sliders, CheckCircle2, Zap,
  Code2, Check, Copy, HelpCircle, Activity, CheckCircle, Flame,
  Bell, Send
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';

export interface TagFilterItem {
  key: string;
  op: string; // '=', '!=', '=~', '!~'
  value: string;
}

export interface AlertRule {
  id: string;
  name: string;
  description: string;
  severity: 'critical' | 'warning' | 'info';
  metricName: string;
  promql?: string;
  tagsFilter: TagFilterItem[];
  operator: '>' | '>=' | '<' | '<=' | '==' | '!=';
  threshold: number;
  forDurationSec: number;
  groupByTags?: string;
  enabled: boolean;
  notifyRemote?: boolean;
  cooldownMs: number;
  lastTriggered?: number;
  template?: string;
  createdAt?: string;
  updatedAt?: string;
}

export interface MetricItemInfo {
  metricName: string;
  label: string;
  category: string;
  unit: string;
  description: string;
}

export interface SampleEvaluationItem {
  value: number;
  deviceId: string;
  deviceIp: string;
  deviceName: string;
  vendor: string;
  ifName?: string;
  metricName: string;
  metricLabel?: string;
  unit?: string;
  tags: Record<string, string>;
  isTriggered: boolean;
}

export interface PreviewMatchResponse {
  matchedCount: number;
  totalSamples: number;
  metricName: string;
  operator: string;
  threshold: number;
  items: SampleEvaluationItem[];
}

const SEVERITY_CONFIG = {
  critical: { label: 'P1 紧急', color: 'text-rose-400', bg: 'bg-rose-500/15 border-rose-500/30', icon: AlertCircle },
  warning: { label: 'P2 警告', color: 'text-amber-400', bg: 'bg-amber-500/15 border-amber-500/30', icon: AlertTriangle },
  info: { label: 'P3 提醒', color: 'text-blue-400', bg: 'bg-blue-500/15 border-blue-500/30', icon: Info },
};

const DURATION_OPTIONS = [
  { value: 0, label: '0 秒 (即刻触发)' },
  { value: 60, label: '1 分钟 (持续异常)' },
  { value: 180, label: '3 分钟 (持续异常)' },
  { value: 300, label: '5 分钟 (持续异常)' },
];

const COOLDOWN_OPTIONS = [
  { value: 600000, label: '10 分钟 (推荐)' },
  { value: 1800000, label: '30 分钟' },
  { value: 3600000, label: '1 小时' },
  { value: 86400000, label: '24 小时' },
];

// 构造 PromQL 表达式字符串
export function buildPromQL(metric: string, tags: TagFilterItem[], op: string, threshold: number): string {
  const validTags = (tags || []).filter(t => t.key && t.key.trim() !== '');
  const tagParts = validTags.map(t => `${t.key}${t.op || '='}"${t.value || ''}"`);
  const tagStr = tagParts.length > 0 ? `{${tagParts.join(', ')}}` : '';
  const operator = op || '>';
  const th = threshold ?? 85;
  return `${metric || 'cpu_usage'}${tagStr} ${operator} ${th}`;
}

// 解析 PromQL 表达式为结构化字段
export function parsePromQLText(promql: string): {
  metricName: string;
  tagsFilter: TagFilterItem[];
  operator: '>' | '>=' | '<' | '<=' | '==' | '!=';
  threshold: number;
} | null {
  const trimmed = (promql || '').trim();
  if (!trimmed) return null;

  // 正则匹配: metric{tags} op threshold
  const mainRegex = /^\s*([a-zA-Z_][a-zA-Z0-9_]*)(?:\{([^}]*)\})?(?:\s*([><=!]=?|==|!=)\s*([0-9.]+))?\s*$/;
  const match = trimmed.match(mainRegex);
  if (!match) return null;

  const metricName = match[1] || 'cpu_usage';
  const tagStr = match[2];
  const operator = (match[3] as any) || '>';
  const threshold = match[4] ? parseFloat(match[4]) : 85;

  const tagsFilter: TagFilterItem[] = [];
  if (tagStr && tagStr.trim()) {
    const tagRegex = /([a-zA-Z_][a-zA-Z0-9_]*)\s*(=~|!~|!=|=)\s*["']?([^"',}]*)["']?/g;
    let tm;
    while ((tm = tagRegex.exec(tagStr)) !== null) {
      if (tm[1]) {
        tagsFilter.push({
          key: tm[1].trim(),
          op: tm[2] || '=',
          value: tm[3] ? tm[3].trim() : '',
        });
      }
    }
  }

  return {
    metricName,
    tagsFilter,
    operator,
    threshold,
  };
}

export default function AlertRules() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [editingRule, setEditingRule] = useState<AlertRule | null>(null);
  const [isCreating, setIsCreating] = useState(false);
  const [deleteConfirm, setDeleteConfirm] = useState<AlertRule | null>(null);

  useEscapeKey({ onEscape: () => { setEditingRule(null); setIsCreating(false); }, enabled: !!editingRule || isCreating });
  useEscapeKey({ onEscape: () => setDeleteConfirm(null), enabled: !!deleteConfirm });

  // 获取规则列表
  const { data: rules = [], isLoading } = useQuery({
    queryKey: ['alert-rules'],
    queryFn: () => api.get('/alert-rules').then(res => res.data.data as AlertRule[]),
  });

  // 获取可用于告警策略的 Prometheus 指标
  const { data: availableMetrics = [] } = useQuery({
    queryKey: ['alert-available-metrics'],
    queryFn: () => api.get('/alert-rules/available-metrics').then(res => res.data.data as MetricItemInfo[]),
  });

  // 获取已存在的标签数据（自动联想候选）
  const { data: availableTagsMap = {} } = useQuery({
    queryKey: ['alert-available-tags'],
    queryFn: () => api.get('/alert-rules/available-tags').then(res => res.data.data as Record<string, string[]>),
  });

  const createMutation = useMutation({
    mutationFn: (data: any) => api.post('/alert-rules', data),
    onSuccess: () => {
      toast.success('告警策略新建成功');
      queryClient.invalidateQueries({ queryKey: ['alert-rules'] });
      setIsCreating(false);
    },
    onError: () => toast.error('创建告警策略失败'),
  });

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: any }) => api.put(`/alert-rules/${id}`, data),
    onSuccess: () => {
      toast.success('告警策略更新成功');
      queryClient.invalidateQueries({ queryKey: ['alert-rules'] });
      setEditingRule(null);
    },
    onError: () => toast.error('更新策略失败'),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/alert-rules/${id}`),
    onSuccess: () => {
      toast.success('策略已成功删除');
      queryClient.invalidateQueries({ queryKey: ['alert-rules'] });
      setDeleteConfirm(null);
    },
    onError: () => toast.error('删除策略失败'),
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => api.put(`/alert-rules/${id}`, { enabled }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['alert-rules'] }),
    onError: () => toast.error('切换状态失败'),
  });

  const seedTemplatesMutation = useMutation({
    mutationFn: () => api.post('/alert-rules/seed-templates'),
    onSuccess: (res: any) => {
      toast.success(`成功导入 ${res.data.count || 8} 套网络监控大屏内置告警模板`);
      queryClient.invalidateQueries({ queryKey: ['alert-rules'] });
    },
    onError: () => toast.error('导入模版失败'),
  });

  const handleSave = (formData: Partial<AlertRule>) => {
    if (editingRule) {
      updateMutation.mutate({ id: editingRule.id, data: formData });
    } else {
      createMutation.mutate(formData);
    }
  };

  const formatLastTriggered = (ts?: number) => {
    if (!ts || ts === 0) return '从未触发';
    const diff = Date.now() - ts;
    if (diff < 60000) return `${Math.round(diff / 1000)} 秒前`;
    if (diff < 3600000) return `${Math.round(diff / 60000)} 分钟前`;
    return `${Math.round(diff / 3600000)} 小时前`;
  };

  return (
    <div className="p-6 max-w-7xl mx-auto space-y-6">
      {/* 头部标题与控制按钮 */}
      <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 bg-surface p-5 rounded-2xl border border-border shadow-sm">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold text-text">告警规则策略 (PromQL 驱动)</h1>
            <span className="px-2.5 py-0.5 rounded-full text-xs font-semibold bg-blue-500/10 text-blue-400 border border-blue-500/20 flex items-center gap-1">
              <Sparkles className="w-3 h-3" /> PromQL 自由输入 + Tag 采集实时匹配
            </span>
          </div>
          <p className="text-xs text-text-secondary mt-1">
            支持用户自定义输入 PromQL 语句，自动与采集端的设备标签 (Tag) 及网络大屏指标精准关联判定
          </p>
        </div>

        <div className="flex items-center gap-3">
          <button
            onClick={() => seedTemplatesMutation.mutate()}
            disabled={seedTemplatesMutation.isPending}
            className="flex items-center gap-1.5 px-3.5 py-2 bg-purple-600/10 hover:bg-purple-600/20 border border-purple-500/30 text-purple-300 text-xs font-medium rounded-lg transition-colors"
          >
            {seedTemplatesMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Sparkles className="w-4 h-4 text-purple-400" />}
            导入大屏内置告警模板
          </button>

          <button
            onClick={() => { setEditingRule(null); setIsCreating(true); }}
            className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white text-xs font-semibold rounded-lg shadow-md hover:shadow-blue-500/20 transition-all"
          >
            <Plus className="w-4 h-4" />
            新建告警规则
          </button>
        </div>
      </div>

      {/* 统计概览 */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4">
        <div className="bg-surface rounded-xl p-4 border border-border shadow-sm">
          <p className="text-2xl font-extrabold text-text">{rules.length}</p>
          <p className="text-xs text-text-secondary mt-1">规则总数</p>
        </div>
        <div className="bg-surface rounded-xl p-4 border border-border shadow-sm">
          <p className="text-2xl font-extrabold text-emerald-400">{rules.filter(r => r.enabled).length}</p>
          <p className="text-xs text-text-secondary mt-1">已启用规则</p>
        </div>
        <div className="bg-surface rounded-xl p-4 border border-border shadow-sm">
          <p className="text-2xl font-extrabold text-rose-400">{rules.filter(r => r.severity === 'critical').length}</p>
          <p className="text-xs text-text-secondary mt-1">P1 紧急级别</p>
        </div>
        <div className="bg-surface rounded-xl p-4 border border-border shadow-sm">
          <p className="text-2xl font-extrabold text-amber-400">{rules.filter(r => r.severity === 'warning').length}</p>
          <p className="text-xs text-text-secondary mt-1">P2 警告级别</p>
        </div>
      </div>

      {/* 规则列表 */}
      {isLoading ? (
        <div className="flex items-center justify-center py-20 bg-surface rounded-2xl border border-border text-text-secondary">
          <Loader2 className="w-6 h-6 animate-spin mr-2" /> 正在加载 Prometheus 告警规则策略...
        </div>
      ) : rules.length === 0 ? (
        <div className="bg-surface rounded-2xl border border-border p-12 text-center">
          <Shield className="w-12 h-12 mx-auto mb-3 text-text-secondary opacity-30" />
          <p className="text-base font-semibold text-text">暂无告警规则</p>
          <p className="text-xs text-text-secondary mt-1 mb-6">点击“导入大屏内置告警模板”快速一键导入，或点击“新建告警规则”自定义 PromQL 策略</p>
          <button
            onClick={() => seedTemplatesMutation.mutate()}
            className="inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white rounded-lg text-xs font-medium hover:bg-blue-500 transition-colors shadow-sm"
          >
            <Sparkles className="w-4 h-4" /> 导入大屏内置告警模板 (推荐)
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {rules.map((rule) => {
            const sev = SEVERITY_CONFIG[rule.severity] || SEVERITY_CONFIG.warning;
            const SevIcon = sev.icon;
            const metricItem = availableMetrics.find(m => m.metricName === rule.metricName);
            const displayPromQL = rule.promql || buildPromQL(rule.metricName, rule.tagsFilter, rule.operator, rule.threshold);

            return (
              <div
                key={rule.id}
                className={`bg-surface rounded-xl border p-5 transition-all shadow-sm hover:border-blue-500/40 ${
                  rule.enabled ? 'border-border' : 'border-border/40 opacity-70'
                }`}
              >
                <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                  <div className="space-y-1.5 flex-1 min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <span className={`inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-xs font-semibold border ${sev.bg} ${sev.color}`}>
                        <SevIcon className="w-3.5 h-3.5" />
                        {sev.label}
                      </span>

                      <h3 className="text-base font-bold text-text">{rule.name}</h3>

                      <span className="px-2 py-0.5 rounded bg-blue-500/10 text-blue-400 font-mono text-xs border border-blue-500/20">
                        {rule.metricName}
                      </span>

                      {rule.notifyRemote !== false ? (
                        <span className="px-2 py-0.5 rounded bg-purple-500/15 text-purple-300 text-xs border border-purple-500/30 flex items-center gap-1 font-medium">
                          <Send className="w-3 h-3 text-purple-400" /> 远程通知
                        </span>
                      ) : (
                        <span className="px-2 py-0.5 rounded bg-slate-800/60 text-slate-400 text-xs border border-slate-700/60">
                          仅站内
                        </span>
                      )}
                    </div>

                    <p className="text-xs text-text-secondary">{rule.description || metricItem?.description || '暂无规则描述'}</p>
                  </div>

                  {/* 右侧控制 */}
                  <div className="flex items-center gap-3 shrink-0">
                    <button
                      onClick={() => toggleMutation.mutate({ id: rule.id, enabled: !rule.enabled })}
                      className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-semibold transition-colors ${
                        rule.enabled ? 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30' : 'bg-surface-secondary text-text-secondary border border-border'
                      }`}
                    >
                      {rule.enabled ? <ToggleRight className="w-4 h-4 text-emerald-400" /> : <ToggleLeft className="w-4 h-4" />}
                      {rule.enabled ? '已启用' : '已禁用'}
                    </button>

                    <button
                      onClick={() => { setEditingRule(rule); setIsCreating(false); }}
                      className="p-1.5 rounded-lg bg-surface-secondary hover:bg-border text-text-secondary hover:text-text transition-colors"
                      title="编辑策略"
                    >
                      <Edit className="w-4 h-4" />
                    </button>

                    <button
                      onClick={() => setDeleteConfirm(rule)}
                      className="p-1.5 rounded-lg bg-surface-secondary hover:bg-rose-500/15 text-text-secondary hover:text-rose-400 transition-colors"
                      title="删除策略"
                    >
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>

                {/* 核心 PromQL 表达式代码条 */}
                <div className="mt-3.5 p-2.5 rounded-lg bg-slate-950/80 border border-slate-800 font-mono text-xs text-emerald-400 flex items-center justify-between gap-3 overflow-x-auto shadow-inner">
                  <div className="flex items-center gap-2 min-w-0">
                    <Code2 className="w-4 h-4 text-purple-400 shrink-0" />
                    <span className="text-slate-400 text-[11px] shrink-0 font-sans font-medium">PromQL:</span>
                    <span className="truncate select-all">{displayPromQL}</span>
                  </div>
                  <button
                    onClick={() => {
                      navigator.clipboard.writeText(displayPromQL);
                      toast.success('PromQL 表达式已复制');
                    }}
                    className="p-1 hover:bg-slate-800 rounded text-slate-400 hover:text-white shrink-0 transition-colors"
                    title="复制 PromQL 语句"
                  >
                    <Copy className="w-3.5 h-3.5" />
                  </button>
                </div>

                {/* 规则触发逻辑详情条 */}
                <div className="mt-3 pt-3 border-t border-border/50 grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs">
                  <div className="flex items-center gap-1.5 text-text-secondary">
                    <Sliders className="w-3.5 h-3.5 text-blue-400 shrink-0" />
                    <span>判定阈值: </span>
                    <span className="font-mono font-semibold text-text">
                      {rule.operator} {rule.threshold} {metricItem?.unit || ''}
                    </span>
                  </div>

                  <div className="flex items-center gap-1.5 text-text-secondary">
                    <Clock className="w-3.5 h-3.5 text-purple-400 shrink-0" />
                    <span>持续时长: </span>
                    <span className="text-text font-medium">{rule.forDurationSec > 0 ? `持续 ${rule.forDurationSec} 秒` : '0 秒 (即刻触发)'}</span>
                  </div>

                  <div className="flex items-center gap-1.5 text-text-secondary">
                    <Zap className="w-3.5 h-3.5 text-amber-400 shrink-0" />
                    <span>上次触发: </span>
                    <span className="text-text font-medium">{formatLastTriggered(rule.lastTriggered)}</span>
                  </div>
                </div>

                {/* 全量 Tag 标签展示栏 */}
                {rule.tagsFilter && rule.tagsFilter.length > 0 && (
                  <div className="mt-3 flex flex-wrap items-center gap-1.5">
                    <span className="text-[11px] text-text-secondary flex items-center gap-1">
                      <Tag className="w-3 h-3 text-emerald-400" /> Tag 标签过滤:
                    </span>
                    {rule.tagsFilter.map((t, idx) => (
                      <span key={idx} className="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-surface-secondary border border-border text-[11px] font-mono text-text">
                        <span className="text-purple-400">{t.key}</span>
                        <span className="text-amber-400 font-bold">{t.op}</span>
                        <span className="text-emerald-400">"{t.value}"</span>
                      </span>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* 新建/编辑规则弹窗 */}
      {(isCreating || editingRule) && (
        <RuleEditorModal
          initialRule={editingRule}
          availableMetrics={availableMetrics}
          availableTagsMap={availableTagsMap}
          onClose={() => { setEditingRule(null); setIsCreating(false); }}
          onSave={handleSave}
          isSubmitting={createMutation.isPending || updateMutation.isPending}
        />
      )}

      {/* 删除确认弹窗 */}
      {deleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-surface border border-border rounded-xl max-w-sm w-full p-6 space-y-4 shadow-xl">
            <h3 className="text-base font-bold text-text">确认删除该告警策略？</h3>
            <p className="text-xs text-text-secondary">规则“{deleteConfirm.name}”删除后将停止后台轮询评估，无法撤销。</p>
            <div className="flex items-center justify-end gap-3 pt-2">
              <button
                onClick={() => setDeleteConfirm(null)}
                className="px-3.5 py-1.5 rounded-lg border border-border text-xs text-text-secondary hover:text-text"
              >
                取消
              </button>
              <button
                onClick={() => deleteMutation.mutate(deleteConfirm.id)}
                disabled={deleteMutation.isPending}
                className="px-3.5 py-1.5 rounded-lg bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold shadow-sm"
              >
                {deleteMutation.isPending ? '正在删除...' : '确认删除'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ============================================================
// 夜莺 (Nightingale) 风格告警策略编辑/新建 Modal 模态框
// 支持 PromQL 自由输入 + Tag 采集实时匹配
// ============================================================

function RuleEditorModal({
  initialRule,
  availableMetrics,
  availableTagsMap,
  onClose,
  onSave,
  isSubmitting
}: {
  initialRule: AlertRule | null;
  availableMetrics: MetricItemInfo[];
  availableTagsMap: Record<string, string[]>;
  onClose: () => void;
  onSave: (rule: Partial<AlertRule>) => void;
  isSubmitting: boolean;
}) {
  const [name, setName] = useState(initialRule?.name || '');
  const [description, setDescription] = useState(initialRule?.description || '');
  const [severity, setSeverity] = useState<'critical' | 'warning' | 'info'>(initialRule?.severity || 'warning');
  const [metricName, setMetricName] = useState(initialRule?.metricName || 'cpu_usage');
  const [operator, setOperator] = useState<'>' | '>=' | '<' | '<=' | '==' | '!='>(initialRule?.operator || '>');
  const [threshold, setThreshold] = useState<number>(initialRule?.threshold ?? 85);
  const [forDurationSec, setForDurationSec] = useState<number>(initialRule?.forDurationSec ?? 60);
  const [cooldownMs, setCooldownMs] = useState<number>(initialRule?.cooldownMs ?? 600000);
  const [tagsFilter, setTagsFilter] = useState<TagFilterItem[]>(initialRule?.tagsFilter || []);
  const [template, setTemplate] = useState(initialRule?.template || '设备 ${device_name}(${device_ip}) 指标 ${metricName} 当前采样值为 ${value}');
  const [notifyRemote, setNotifyRemote] = useState<boolean>(initialRule?.notifyRemote !== false);

  // PromQL 自由输入状态
  const [promqlInput, setPromqlInput] = useState<string>(() => {
    if (initialRule?.promql) return initialRule.promql;
    return buildPromQL(
      initialRule?.metricName || 'cpu_usage',
      initialRule?.tagsFilter || [],
      initialRule?.operator || '>',
      initialRule?.threshold ?? 85
    );
  });
  const [promqlParseError, setPromqlParseError] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<'promql' | 'form'>('promql');

  // 实时匹配结果预览
  const [previewResult, setPreviewResult] = useState<PreviewMatchResponse | null>(null);
  const [isPreviewLoading, setIsPreviewLoading] = useState<boolean>(false);

  const currentMetric = availableMetrics.find(m => m.metricName === metricName);

  // 当用户在 PromQL 输入框打字时，尝试解析并同步到各表单字段
  const handlePromqlInputChange = (newPromql: string) => {
    setPromqlInput(newPromql);
    const parsed = parsePromQLText(newPromql);
    if (parsed) {
      setPromqlParseError(null);
      setMetricName(parsed.metricName);
      setTagsFilter(parsed.tagsFilter);
      setOperator(parsed.operator);
      setThreshold(parsed.threshold);
    } else {
      setPromqlParseError('PromQL 语法格式提示: 请按照 <指标名>{<tag_key>=<"值">} <比较符> <阈值> 格式输入');
    }
  };

  // 当用户在表单上做修改时，自动更新 PromQL 文本
  const handleFormChangeSyncPromql = (
    newMetric: string,
    newTags: TagFilterItem[],
    newOp: string,
    newTh: number
  ) => {
    const updatedPromql = buildPromQL(newMetric, newTags, newOp, newTh);
    setPromqlInput(updatedPromql);
    setPromqlParseError(null);
  };

  const handleMetricChange = (val: string) => {
    setMetricName(val);
    handleFormChangeSyncPromql(val, tagsFilter, operator, threshold);
  };

  const handleOperatorChange = (val: any) => {
    setOperator(val);
    handleFormChangeSyncPromql(metricName, tagsFilter, val, threshold);
  };

  const handleThresholdChange = (val: number) => {
    setThreshold(val);
    handleFormChangeSyncPromql(metricName, tagsFilter, operator, val);
  };

  const handleAddTagFilter = () => {
    const updated = [...tagsFilter, { key: 'env', op: '=', value: 'production' }];
    setTagsFilter(updated);
    handleFormChangeSyncPromql(metricName, updated, operator, threshold);
  };

  const handleRemoveTagFilter = (idx: number) => {
    const updated = tagsFilter.filter((_, i) => i !== idx);
    setTagsFilter(updated);
    handleFormChangeSyncPromql(metricName, updated, operator, threshold);
  };

  const handleUpdateTagFilter = (idx: number, field: keyof TagFilterItem, val: string) => {
    const updated = [...tagsFilter];
    updated[idx] = { ...updated[idx], [field]: val };
    setTagsFilter(updated);
    handleFormChangeSyncPromql(metricName, updated, operator, threshold);
  };

  // 快捷 PromQL 预置模板注入
  const applyPresetPromql = (preset: string) => {
    handlePromqlInputChange(preset);
  };

  // 实时测试 PromQL / Tag 匹配
  useEffect(() => {
    const timer = setTimeout(() => {
      setIsPreviewLoading(true);
      api.post('/alert-rules/preview-match', {
        promql: promqlInput,
        metricName,
        tagsFilter,
        operator,
        threshold,
      })
        .then(res => {
          setPreviewResult(res.data.data);
        })
        .catch(() => {
          // 降级使用空
          setPreviewResult(null);
        })
        .finally(() => {
          setIsPreviewLoading(false);
        });
    }, 300);

    return () => clearTimeout(timer);
  }, [promqlInput, metricName, tagsFilter, operator, threshold]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    onSave({
      name,
      description,
      severity,
      metricName,
      promql: promqlInput,
      operator,
      threshold,
      forDurationSec,
      cooldownMs,
      tagsFilter,
      template,
      notifyRemote,
      enabled: initialRule ? initialRule.enabled : true,
    });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4 overflow-y-auto">
      <div className="bg-surface border border-border rounded-2xl max-w-3xl w-full p-6 shadow-2xl space-y-6 my-8 max-h-[92vh] overflow-y-auto">
        {/* 顶部标题 */}
        <div className="flex items-center justify-between border-b border-border pb-4">
          <div>
            <h2 className="text-lg font-bold text-text flex items-center gap-2">
              <Code2 className="w-5 h-5 text-blue-400" />
              {initialRule ? '编辑告警策略' : '新建告警规则策略 (PromQL 自由输入 + Tag 匹配)'}
            </h2>
            <p className="text-xs text-text-secondary mt-0.5">
              支持用户直接键入 PromQL 表达式，实时匹配采集端 Tag 标签并进行指标阈值计算
            </p>
          </div>
          <button onClick={onClose} className="p-1 rounded-lg hover:bg-surface-secondary text-text-secondary hover:text-text">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-5">
          {/* 1. 基础属性 */}
          <div className="space-y-3">
            <h3 className="text-xs font-bold uppercase tracking-wider text-blue-400 flex items-center gap-1.5">
              <Sliders className="w-3.5 h-3.5" /> 1. 规则基本属性
            </h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-semibold text-text-secondary mb-1">策略名称 *</label>
                <input
                  type="text"
                  required
                  value={name}
                  onChange={e => setName(e.target.value)}
                  placeholder="例如: [网络大屏] 核心交换机 CPU 负载超限"
                  className="w-full px-3 py-2 bg-surface-secondary border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-text-secondary mb-1">告警级别 (Severity)</label>
                <select
                  value={severity}
                  onChange={e => setSeverity(e.target.value as any)}
                  className="w-full px-3 py-2 bg-surface-secondary border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
                >
                  <option value="critical">P1 紧急 (Critical)</option>
                  <option value="warning">P2 警告 (Warning)</option>
                  <option value="info">P3 提醒 (Info)</option>
                </select>
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-text-secondary mb-1">规则描述说明</label>
              <input
                type="text"
                value={description}
                onChange={e => setDescription(e.target.value)}
                placeholder="详细说明此告警规则的触发场景与影响"
                className="w-full px-3 py-2 bg-surface-secondary border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
              />
            </div>
          </div>

          {/* 2. PromQL 表达式自由输入核心区 */}
          <div className="space-y-3 pt-3 border-t border-border/70">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-bold uppercase tracking-wider text-purple-400 flex items-center gap-1.5">
                <Code2 className="w-3.5 h-3.5" /> 2. 告警 PromQL 表达式输入 (用户自由编写)
              </h3>

              {/* 模式切换 */}
              <div className="flex items-center bg-surface-secondary rounded-lg p-0.5 border border-border text-[11px]">
                <button
                  type="button"
                  onClick={() => setActiveTab('promql')}
                  className={`px-2.5 py-1 rounded-md font-medium transition-all ${
                    activeTab === 'promql'
                      ? 'bg-purple-600 text-white shadow-sm'
                      : 'text-text-secondary hover:text-text'
                  }`}
                >
                  ✍️ PromQL 表达式模式
                </button>
                <button
                  type="button"
                  onClick={() => setActiveTab('form')}
                  className={`px-2.5 py-1 rounded-md font-medium transition-all ${
                    activeTab === 'form'
                      ? 'bg-purple-600 text-white shadow-sm'
                      : 'text-text-secondary hover:text-text'
                  }`}
                >
                  🛠️ 表单向导模式
                </button>
              </div>
            </div>

            {/* PromQL 快捷常用模板 Chips */}
            <div className="flex items-center gap-1.5 flex-wrap">
              <span className="text-[10px] text-text-secondary font-medium">快速套用:</span>
              {[
                { label: 'CPU使用率 > 85%', q: 'cpu_usage{env="production"} > 85' },
                { label: '内存使用率 >= 90%', q: 'memory_usage{cluster="core-net"} >= 90' },
                { label: '华为设备离线', q: 'device_status{vendor="huawei"} == 0' },
                { label: '机房温度 > 45℃', q: 'temperature_celsius{location="IDC-1"} > 45' },
                { label: '核心交换机', q: 'cpu_usage{role="core-switch"} > 80' },
              ].map((chip, idx) => (
                <button
                  key={idx}
                  type="button"
                  onClick={() => applyPresetPromql(chip.q)}
                  className="px-2 py-0.5 text-[10px] rounded bg-purple-500/10 hover:bg-purple-500/20 text-purple-300 border border-purple-500/25 font-mono transition-colors"
                >
                  + {chip.label}
                </button>
              ))}
            </div>

            {/* PromQL 文本编辑器 */}
            <div className="space-y-1.5">
              <div className="relative rounded-xl overflow-hidden border border-purple-500/30 bg-slate-950 focus-within:ring-2 focus-within:ring-purple-500/50 shadow-inner">
                <div className="flex items-center justify-between px-3 py-1.5 bg-slate-900 border-b border-slate-800 text-[11px] text-slate-400">
                  <div className="flex items-center gap-1.5">
                    <span className="w-2 h-2 rounded-full bg-emerald-400 inline-block animate-pulse"></span>
                    <span className="font-mono text-purple-300 font-semibold">PromQL Alert Expression</span>
                  </div>
                  <span className="text-[10px] text-slate-500">支持 {`{tag_key="value"}`} 标签过滤与比较运算</span>
                </div>
                <textarea
                  rows={2}
                  value={promqlInput}
                  onChange={e => handlePromqlInputChange(e.target.value)}
                  placeholder='例如: cpu_usage{env="production", cluster="core-net", role="core-switch"} > 85'
                  className="w-full px-3 py-2.5 bg-transparent font-mono text-xs text-emerald-400 placeholder:text-slate-600 focus:outline-none resize-none leading-relaxed"
                />
              </div>

              {promqlParseError && (
                <p className="text-[11px] text-amber-400 flex items-center gap-1">
                  <AlertTriangle className="w-3.5 h-3.5 shrink-0" />
                  {promqlParseError}
                </p>
              )}
            </div>
          </div>

          {/* 3. 可视化表单向导部分 (与 PromQL 双向同步) */}
          {activeTab === 'form' && (
            <div className="space-y-4 p-4 rounded-xl bg-surface-secondary/40 border border-border">
              {/* 指标选择 */}
              <div>
                <label className="block text-xs font-semibold text-text-secondary mb-1">
                  目标 Prometheus 指标 (Metric Name) *
                </label>
                <select
                  value={metricName}
                  onChange={e => handleMetricChange(e.target.value)}
                  className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-xs text-text focus:outline-none focus:border-purple-500 font-mono"
                >
                  {availableMetrics.map(m => (
                    <option key={m.metricName} value={m.metricName}>
                      [{m.category}] {m.label} — ({m.metricName})
                    </option>
                  ))}
                </select>
                {currentMetric && (
                  <p className="text-[11px] text-text-secondary mt-1">
                    指标单位: <span className="text-purple-400 font-mono font-semibold">{currentMetric.unit || '无'}</span> | 说明: {currentMetric.description}
                  </p>
                )}
              </div>

              {/* Tag 标签组筛选器 */}
              <div className="space-y-2 pt-2 border-t border-border/50">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-text-secondary flex items-center gap-1">
                    <Tag className="w-3.5 h-3.5 text-emerald-400" /> Tag 标签筛选匹配器
                  </span>
                  <button
                    type="button"
                    onClick={handleAddTagFilter}
                    className="inline-flex items-center gap-1 px-2.5 py-1 bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20 border border-emerald-500/30 rounded text-xs font-semibold transition-colors"
                  >
                    <Plus className="w-3.5 h-3.5" /> 添加 Tag 过滤条件
                  </button>
                </div>

                {tagsFilter.length === 0 ? (
                  <div className="bg-surface/50 border border-dashed border-border rounded-lg p-3 text-center text-xs text-text-secondary">
                    未设置 Tag 筛选条件，默认匹配该指标下的全部设备对象
                  </div>
                ) : (
                  <div className="space-y-2">
                    {tagsFilter.map((item, idx) => (
                      <div key={idx} className="flex items-center gap-2">
                        {/* Tag Key input with datalist */}
                        <div className="relative w-40">
                          <input
                            type="text"
                            value={item.key}
                            onChange={e => handleUpdateTagFilter(idx, 'key', e.target.value)}
                            placeholder="标签键 (如 env, cluster)"
                            list={`tag-keys-list`}
                            className="w-full px-2.5 py-1.5 bg-surface border border-border rounded text-xs text-purple-400 font-mono font-semibold focus:outline-none focus:border-emerald-500"
                          />
                          <datalist id="tag-keys-list">
                            {Object.keys(availableTagsMap).map(k => (
                              <option key={k} value={k} />
                            ))}
                          </datalist>
                        </div>

                        {/* Operator */}
                        <select
                          value={item.op}
                          onChange={e => handleUpdateTagFilter(idx, 'op', e.target.value)}
                          className="w-24 px-2.5 py-1.5 bg-surface border border-border rounded text-xs text-amber-400 font-mono font-bold focus:outline-none focus:border-amber-500"
                        >
                          <option value="=">= (等于)</option>
                          <option value="!=">!= (不等于)</option>
                          <option value="=~">=~ (正则匹配)</option>
                          <option value="!~">!~ (正则排除)</option>
                        </select>

                        {/* Tag Value */}
                        <div className="relative flex-1">
                          <input
                            type="text"
                            value={item.value}
                            onChange={e => handleUpdateTagFilter(idx, 'value', e.target.value)}
                            placeholder="目标值 (如 production)"
                            list={`suggest-val-${item.key}`}
                            className="w-full px-2.5 py-1.5 bg-surface border border-border rounded text-xs text-emerald-400 font-mono focus:outline-none focus:border-emerald-500"
                          />
                          <datalist id={`suggest-val-${item.key}`}>
                            {(availableTagsMap[item.key] || []).map((val, vIdx) => (
                              <option key={vIdx} value={val} />
                            ))}
                          </datalist>
                        </div>

                        <button
                          type="button"
                          onClick={() => handleRemoveTagFilter(idx)}
                          className="p-1.5 text-text-secondary hover:text-rose-400 transition-colors"
                          title="移除条件"
                        >
                          <X className="w-4 h-4" />
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* 运算符与阈值 */}
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/50">
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">比较运算符 (Operator)</label>
                  <select
                    value={operator}
                    onChange={e => handleOperatorChange(e.target.value as any)}
                    className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-xs text-amber-400 font-mono font-bold focus:outline-none focus:border-amber-500"
                  >
                    <option value=">">&gt; (大于)</option>
                    <option value="=">&gt;= (大于等于)</option>
                    <option value="<">&lt; (小于)</option>
                    <option value="<=">&lt;= (小于等于)</option>
                    <option value="==">== (等于)</option>
                    <option value="!=">!= (不等于)</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">触发阈值 (Threshold)</label>
                  <input
                    type="number"
                    step="any"
                    required
                    value={threshold}
                    onChange={e => handleThresholdChange(parseFloat(e.target.value) || 0)}
                    className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-xs text-text font-mono focus:outline-none focus:border-amber-500"
                  />
                </div>
              </div>
            </div>
          )}

          {/* 4. 🎯 实时 Tag 匹配度预检与采样测试面板 */}
          <div className="space-y-2 pt-2 border-t border-border/60">
            <div className="flex items-center justify-between">
              <h3 className="text-xs font-bold uppercase tracking-wider text-emerald-400 flex items-center gap-1.5">
                <Activity className="w-3.5 h-3.5" /> 3. 采集 Tag 实时匹配预检 (Live Tag Matching Preview)
              </h3>
              {isPreviewLoading && (
                <span className="text-[11px] text-text-secondary flex items-center gap-1">
                  <Loader2 className="w-3 h-3 animate-spin text-blue-400" /> 正在匹配系统中...
                </span>
              )}
            </div>

            <div className="bg-slate-950/70 border border-slate-800 rounded-xl p-3.5 space-y-3">
              {/* 匹配统计汇总 */}
              <div className="flex items-center justify-between flex-wrap gap-2 pb-2.5 border-b border-slate-800/80 text-xs">
                <div className="flex items-center gap-2">
                  {previewResult && previewResult.matchedCount > 0 ? (
                    <span className="px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 text-[11px] font-semibold flex items-center gap-1">
                      <CheckCircle className="w-3 h-3" /> 已精准匹配到 {previewResult.matchedCount} 个指标采样对象
                    </span>
                  ) : (
                    <span className="px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-400 border border-amber-500/30 text-[11px] font-semibold flex items-center gap-1">
                      <AlertTriangle className="w-3 h-3" /> 当前未匹配到设备 Tag 标签
                    </span>
                  )}

                  <span className="text-slate-400 text-[11px]">
                    (当前系统在线采样库: {previewResult?.totalSamples ?? 0} 项)
                  </span>
                </div>

                <span className="text-slate-400 text-[11px] font-mono">
                  判定条件: <span className="text-amber-400">{operator} {threshold}</span>
                </span>
              </div>

              {/* 匹配到的对象清单 */}
              {previewResult && previewResult.items && previewResult.items.length > 0 ? (
                <div className="space-y-2 max-h-44 overflow-y-auto pr-1">
                  {previewResult.items.map((item, idx) => (
                    <div
                      key={idx}
                      className={`p-2.5 rounded-lg border text-xs flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2 transition-colors ${
                        item.isTriggered
                          ? 'bg-rose-950/30 border-rose-500/40 text-rose-300'
                          : 'bg-slate-900/60 border-slate-800 text-slate-300'
                      }`}
                    >
                      <div className="space-y-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-white">
                            {item.deviceName || item.deviceId || '设备'}
                          </span>
                          <span className="font-mono text-[11px] text-slate-400">
                            ({item.deviceIp || '无IP'})
                          </span>
                          {item.vendor && (
                            <span className="px-1.5 py-0.2 rounded bg-slate-800 text-slate-300 text-[10px] uppercase">
                              {item.vendor}
                            </span>
                          )}
                          {item.metricLabel && (
                            <span className="text-[11px] text-purple-300">
                              {item.metricLabel}
                            </span>
                          )}
                        </div>

                        {/* Tag 标签展示 */}
                        <div className="flex items-center gap-1 flex-wrap">
                          {Object.entries(item.tags || {}).map(([tk, tv]) => (
                            <span key={tk} className="px-1.5 py-0.2 rounded bg-slate-800/80 border border-slate-700/60 text-[10px] font-mono text-emerald-400">
                              <span className="text-slate-400">{tk}=</span>"{tv}"
                            </span>
                          ))}
                        </div>
                      </div>

                      {/* 当前采样值与告警状态 */}
                      <div className="flex items-center gap-2 shrink-0 self-end sm:self-center font-mono">
                        <div className="text-right">
                          <div className="text-[10px] text-slate-400">当前采样值</div>
                          <div className="font-bold text-sm text-white">
                            {item.value.toFixed(2)} {item.unit || ''}
                          </div>
                        </div>

                        {item.isTriggered ? (
                          <span className="px-2 py-1 rounded bg-rose-600 text-white text-[11px] font-bold flex items-center gap-1 shadow-sm animate-pulse">
                            <Flame className="w-3 h-3" /> 触发告警
                          </span>
                        ) : (
                          <span className="px-2 py-1 rounded bg-emerald-600/20 text-emerald-400 border border-emerald-500/30 text-[11px] font-medium flex items-center gap-1">
                            <Check className="w-3 h-3" /> 正常
                          </span>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="p-3 text-center text-xs text-slate-400 space-y-1">
                  <p>当前 PromQL 或 Tag 条件未匹配到任何设备。</p>
                  <p className="text-[11px] text-slate-500">
                    提示: 可在“网络大屏”为监控指标设置 <code className="text-purple-300 font-mono">env</code>、<code className="text-purple-300 font-mono">cluster</code>、<code className="text-purple-300 font-mono">role</code> 等自定义 Tag 标签。
                  </p>
                </div>
              )}
            </div>
          </div>

          {/* 5. 持续时长与再触发静默 */}
          <div className="space-y-3 pt-2 border-t border-border/60">
            <h3 className="text-xs font-bold uppercase tracking-wider text-amber-400 flex items-center gap-1.5">
              <Clock className="w-3.5 h-3.5" /> 4. 持续判定与通知冷却
            </h3>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-semibold text-text-secondary mb-1">持续异常时长 (For Duration)</label>
                <select
                  value={forDurationSec}
                  onChange={e => setForDurationSec(parseInt(e.target.value))}
                  className="w-full px-3 py-2 bg-surface-secondary border border-border rounded-lg text-xs text-text focus:outline-none focus:border-amber-500"
                >
                  {DURATION_OPTIONS.map(d => (
                    <option key={d.value} value={d.value}>{d.label}</option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-text-secondary mb-1">告警再触发静默间隔 (Cooldown)</label>
                <select
                  value={cooldownMs}
                  onChange={e => setCooldownMs(parseInt(e.target.value))}
                  className="w-full px-3 py-2 bg-surface-secondary border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
                >
                  {COOLDOWN_OPTIONS.map(c => (
                    <option key={c.value} value={c.value}>{c.label}</option>
                  ))}
                </select>
              </div>
            </div>

            <div>
              <label className="block text-xs font-semibold text-text-secondary mb-1">告警通知消息模版</label>
              <input
                type="text"
                value={template}
                onChange={e => setTemplate(e.target.value)}
                placeholder="模版变量: ${device_name}, ${device_ip}, ${if_name}, ${value}"
                className="w-full px-3 py-2 bg-surface-secondary border border-border rounded-lg text-xs text-text font-mono focus:outline-none focus:border-blue-500"
              />
            </div>
          </div>

          {/* 5. 触发远程通知分发 */}
          <div className="space-y-3 pt-3 border-t border-border/60">
            <div className="flex items-center justify-between p-3.5 rounded-xl bg-purple-500/5 border border-purple-500/20">
              <div className="space-y-0.5 pr-4">
                <h3 className="text-xs font-bold text-purple-300 flex items-center gap-1.5">
                  <Send className="w-3.5 h-3.5 text-purple-400" /> 触发远程通知 (Webhook / 企业微信 / 钉钉 / 邮件)
                </h3>
                <p className="text-[11px] text-text-secondary leading-relaxed">
                  勾选后，当此规则判定触发告警时，会自动将告警信息分发至「系统设置 &gt; 监控告警」中已启用的远程 Webhook 或群机器人通道。
                </p>
              </div>
              <label className="relative inline-flex items-center cursor-pointer shrink-0">
                <input
                  type="checkbox"
                  className="sr-only peer"
                  checked={notifyRemote}
                  onChange={e => setNotifyRemote(e.target.checked)}
                />
                <div className="w-11 h-6 bg-slate-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-purple-600 shadow-inner" />
              </label>
            </div>
          </div>

          {/* 提交按钮 */}
          <div className="flex items-center justify-end gap-3 pt-4 border-t border-border">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-lg border border-border text-xs text-text-secondary hover:text-text transition-colors"
            >
              取消
            </button>

            <button
              type="submit"
              disabled={isSubmitting}
              className="inline-flex items-center gap-2 px-5 py-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white text-xs font-semibold rounded-lg shadow-md transition-all"
            >
              {isSubmitting ? <Loader2 className="w-4 h-4 animate-spin" /> : <Save className="w-4 h-4" />}
              {initialRule ? '保存更新' : '立即创建策略'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
