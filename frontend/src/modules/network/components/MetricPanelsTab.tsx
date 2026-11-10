/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Plus, Trash2, Search, Database, Activity,
  BarChart3, Pencil, Eye, X, Loader2, Upload, AlertTriangle,
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useAuth } from '../../../contexts/AuthContext';

// ============================================================
// 类型
// ============================================================

interface MetricDefinition {
  id: string;
  metric_name: string;
  module_name: string;
  oid: string;
  description: string;
  metric_type: string;
  is_table: number;
  index_tag: string;
  label_names: string;
  unit: string;
  source: string;
}

interface DashboardPanel {
  id: string;
  title: string;
  metric_name: string;
  aggregation: string;
  function_filter: string;
  tag_filters: string; // JSON map
  group_by: string; // JSON array
  chart_type: string;
  time_range: string;
  refresh_interval: number;
  grid_pos: number;
  enabled: number;
}

interface MetricPoint {
  timestamp: string;
  value: number;
}

interface MetricSeries {
  device_id: string;
  tags?: Record<string, string>;
  points: MetricPoint[];
}

const CHART_OPTIONS = [
  { value: 'line', label: '折线图' },
  { value: 'bar', label: '柱状图' },
];

const AGG_OPTIONS = [
  { value: 'avg', label: '平均' },
  { value: 'max', label: '最大' },
  { value: 'min', label: '最小' },
  { value: 'sum', label: '求和' },
  { value: 'last', label: '最新' },
];

const TIME_OPTIONS = [
  { value: '1h', label: '1 小时' },
  { value: '6h', label: '6 小时' },
  { value: '24h', label: '24 小时' },
  { value: '7d', label: '7 天' },
];

const SERIES_COLORS = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#06b6d4', '#ec4899', '#84cc16'];

// ============================================================
// 主组件
// ============================================================

export default function MetricPanelsTab() {
  const [activeSub, setActiveSub] = useState<'definitions' | 'panels' | 'generator'>('definitions');

  return (
    <div className="space-y-4">
      <div className="bg-surface rounded-xl border border-border p-1 w-fit">
        {[
          { key: 'definitions' as const, label: '指标定义', icon: Database },
          { key: 'panels' as const, label: '面板管理', icon: BarChart3 },
          { key: 'generator' as const, label: 'generator 导入', icon: Upload },
        ].map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveSub(tab.key)}
            className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-all ${
              activeSub === tab.key
                ? 'bg-gradient-to-r from-blue-600 to-blue-700 text-white shadow-sm'
                : 'text-text-secondary hover:text-text hover:bg-surface-secondary'
            }`}
          >
            <tab.icon className="w-3.5 h-3.5" />
            {tab.label}
          </button>
        ))}
      </div>

      {activeSub === 'definitions' && <DefinitionsSubTab />}
      {activeSub === 'panels' && <PanelsSubTab />}
      {activeSub === 'generator' && <GeneratorSubTab />}
    </div>
  );
}

import { useDebounce } from '../../../hooks/useDebounce';

// ============================================================
// 指标定义子页
// ============================================================

function DefinitionsSubTab() {
  const [q, setQ] = useState('');
  const [moduleFilter, setModuleFilter] = useState('');
  const [page, setPage] = useState(1);
  const debouncedQ = useDebounce(q, 300);

  const { data: modules = [] } = useQuery({
    queryKey: ['snmp-metric-modules'],
    queryFn: () => api.get('/snmp-metrics/definitions', { params: { limit: 500 } }).then(res => res.data.data || []),
    select: (defs: MetricDefinition[]) => Array.from(new Set(defs.map(d => d.module_name).filter(Boolean))).sort(),
    staleTime: 60000,
  });

  const { data: defs = [], isLoading } = useQuery<MetricDefinition[]>({
    queryKey: ['snmp-metric-definitions', moduleFilter, debouncedQ],
    queryFn: () => api.get('/snmp-metrics/definitions', {
      params: { module: moduleFilter || undefined, q: debouncedQ || undefined, limit: 200 },
    }).then(res => res.data.data || []),
  });

  const pageSize = 50;
  const totalPages = Math.ceil(defs.length / pageSize) || 1;
  const displayDefs = useMemo(() => {
    const start = (page - 1) * pageSize;
    return defs.slice(start, start + pageSize);
  }, [defs, page]);

  return (
    <div className="bg-surface rounded-xl border border-border p-4">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-medium text-text flex items-center gap-2">
          <Database className="w-4 h-4 text-blue-400" />
          指标定义（snmp_exporter 风格命名）
        </h3>
        <span className="text-xs text-text-secondary">共 {defs.length} 条</span>
      </div>

      <div className="flex gap-2 mb-3">
        <select
          value={moduleFilter}
          onChange={e => { setModuleFilter(e.target.value); setPage(1); }}
          className="flex-shrink-0 px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500 cursor-pointer"
        >
          <option value="">全部模块</option>
          {modules.map(m => <option key={m} value={m}>{m}</option>)}
        </select>
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-secondary" />
          <input
            value={q}
            onChange={e => { setQ(e.target.value); setPage(1); }}
            placeholder="搜索指标名 / 描述 / OID..."
            className="w-full pl-9 pr-4 py-2 bg-surface border border-border rounded-lg text-sm text-text placeholder:text-text-secondary focus:outline-none focus:border-blue-500"
          />
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-12"><Loader2 className="w-6 h-6 animate-spin text-blue-500" /></div>
      ) : defs.length === 0 ? (
        <p className="text-center py-12 text-sm text-text-secondary">暂无指标定义，请先到「generator 导入」页导入，或在后端执行种子</p>
      ) : (
        <>
          <div className="max-h-[55vh] overflow-y-auto border border-border rounded-lg">
            <table className="w-full">
              <thead className="sticky top-0 bg-surface z-10">
                <tr className="border-b border-border">
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">指标名</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">模块</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">OID</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">类型</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">来源</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">描述</th>
                </tr>
              </thead>
              <tbody>
                {displayDefs.map((d, i) => (
                  <tr key={d.id || i} className="border-b border-border/30 hover:bg-surface-secondary/20 transition-colors">
                    <td className="px-3 py-2 text-xs font-mono text-blue-400">{d.metric_name}</td>
                    <td className="px-3 py-2 text-xs text-text-secondary">{d.module_name || '-'}</td>
                    <td className="px-3 py-2 text-xs font-mono text-text-secondary">{d.oid || '-'}</td>
                    <td className="px-3 py-2">
                      <span className={`inline-block px-1.5 py-0.5 text-[10px] rounded border ${
                        d.metric_type === 'counter'
                          ? 'bg-amber-500/15 text-amber-400 border-amber-500/30'
                          : 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30'
                      }`}>{d.metric_type || '-'}</span>
                      {d.is_table === 1 && <span className="ml-1 inline-block px-1.5 py-0.5 text-[10px] rounded border bg-violet-500/15 text-violet-400 border-violet-500/30">表</span>}
                    </td>
                    <td className="px-3 py-2 text-xs text-text-secondary">{d.source || '-'}</td>
                    <td className="px-3 py-2 text-xs text-text-secondary max-w-xs truncate">{d.description || '-'}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {totalPages > 1 && (
            <div className="flex items-center justify-between mt-3 text-xs text-text-secondary">
              <span>显示第 {(page - 1) * pageSize + 1} - {Math.min(page * pageSize, defs.length)} 条，共 {defs.length} 条</span>
              <div className="flex items-center gap-1">
                <button
                  disabled={page <= 1}
                  onClick={() => setPage(p => p - 1)}
                  className="px-2 py-1 rounded bg-surface-secondary hover:bg-border border border-border disabled:opacity-40"
                >
                  上一页
                </button>
                <span className="px-2">{page} / {totalPages}</span>
                <button
                  disabled={page >= totalPages}
                  onClick={() => setPage(p => p + 1)}
                  className="px-2 py-1 rounded bg-surface-secondary hover:bg-border border border-border disabled:opacity-40"
                >
                  下一页
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ============================================================
// 面板管理子页
// ============================================================

function PanelsSubTab() {
  const { user } = useAuth();
  const isAdmin = user?.role === 'admin';
  const queryClient = useQueryClient();
  const toast = useToast();
  const [editPanel, setEditPanel] = useState<DashboardPanel | null>(null);
  const [showEditor, setShowEditor] = useState(false);
  const [previewId, setPreviewId] = useState<string | null>(null);

  const { data: panels = [], isLoading } = useQuery({
    queryKey: ['snmp-metric-panels'],
    queryFn: () => api.get('/snmp-metrics/panels').then(res => res.data.data || []),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/snmp-metrics/panels/${id}`),
    onSuccess: () => {
      toast.success('面板已删除');
      queryClient.invalidateQueries({ queryKey: ['snmp-metric-panels'] });
    },
    onError: (e: any) => toast.error(e.message || '删除面板失败'),
  });

  const openCreate = () => {
    setEditPanel(null);
    setShowEditor(true);
  };

  const openEdit = (p: DashboardPanel) => {
    setEditPanel(p);
    setShowEditor(true);
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-text flex items-center gap-2">
          <BarChart3 className="w-4 h-4 text-blue-400" />
          监控面板（Grafana 风格）
        </h3>
        {isAdmin && (
          <button
            onClick={openCreate}
            className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-gradient-to-r from-blue-600 to-blue-700 rounded-md hover:from-blue-700 hover:to-blue-800 transition-all"
          >
            <Plus className="w-3.5 h-3.5" />
            新建面板
          </button>
        )}
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-12"><Loader2 className="w-6 h-6 animate-spin text-blue-500" /></div>
      ) : panels.length === 0 ? (
        <div className="bg-surface rounded-xl border border-border p-12 text-center">
          <BarChart3 className="w-10 h-10 text-text-secondary mx-auto mb-3" />
          <p className="text-sm text-text-secondary">暂无面板</p>
          {isAdmin && <p className="text-xs text-text-secondary mt-1">点击右上角「新建面板」开始配置</p>}
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
          {panels.map((p: DashboardPanel) => (
            <div key={p.id} className="bg-surface rounded-xl border border-border p-3 flex flex-col">
              <div className="flex items-center justify-between mb-2">
                <div className="min-w-0">
                  <p className="text-sm font-medium text-text truncate">{p.title}</p>
                  <p className="text-[10px] text-text-secondary font-mono truncate">{p.metric_name}</p>
                </div>
                <div className="flex items-center gap-1 flex-shrink-0 ml-2">
                  <button onClick={() => setPreviewId(previewId === p.id ? null : p.id)} className="p-1.5 text-blue-400 hover:bg-blue-500/10 rounded-md transition-colors" title="预览">
                    <Eye className="w-3.5 h-3.5" />
                  </button>
                  {isAdmin && (
                    <>
                      <button onClick={() => openEdit(p)} className="p-1.5 text-text-secondary hover:text-blue-400 hover:bg-blue-500/10 rounded-md transition-colors" title="编辑">
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button onClick={() => deleteMutation.mutate(p.id)} disabled={deleteMutation.isPending} className="p-1.5 text-text-secondary hover:text-red-400 hover:bg-red-500/10 rounded-md transition-colors" title="删除">
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </>
                  )}
                </div>
              </div>
              <div className="flex items-center gap-2 text-[10px] text-text-secondary mb-2">
                <span className="px-1.5 py-0.5 rounded bg-surface-secondary">{p.chart_type || 'line'}</span>
                <span className="px-1.5 py-0.5 rounded bg-surface-secondary">{p.aggregation || 'avg'}</span>
                <span className="px-1.5 py-0.5 rounded bg-surface-secondary">{p.time_range || '24h'}</span>
              </div>
              {previewId === p.id && <PanelPreview key={p.id} panel={p} />}
            </div>
          ))}
        </div>
      )}

      {showEditor && (
        <PanelEditorModal
          panel={editPanel}
          onClose={() => setShowEditor(false)}
          onSaved={() => {
            setShowEditor(false);
            queryClient.invalidateQueries({ queryKey: ['snmp-metric-panels'] });
          }}
        />
      )}
    </div>
  );
}

// ============================================================
// 面板预览（调用 /panels/query 渲染时序）
// ============================================================

function PanelPreview({ panel }: { panel: DashboardPanel }) {
  const { data, isFetching, error } = useQuery({
    queryKey: ['snmp-metric-panel-preview', panel.id],
    queryFn: async () => {
      const tagFilters = safeParseJSON<Record<string, string>>(panel.tag_filters, {});
      const groupBy = safeParseJSON<string[]>(panel.group_by, []);
      const resp = await api.post('/snmp-metrics/panels/query', {
        metric_name: panel.metric_name,
        aggregation: panel.aggregation || 'avg',
        tag_filters: tagFilters,
        group_by: groupBy,
        time_range: panel.time_range || '24h',
      });
      return (resp.data.data || []) as MetricSeries[];
    },
  });

  if (isFetching) {
    return <div className="flex items-center justify-center py-8"><Loader2 className="w-5 h-5 animate-spin text-blue-500" /></div>;
  }
  if (error) {
    return (
      <div className="flex items-center gap-2 py-6 text-sm text-red-400">
        <AlertTriangle className="w-4 h-4" />
        查询失败，请确认该指标已有采集样本数据
      </div>
    );
  }
  const series = (data || []) as MetricSeries[];
  if (series.length === 0) {
    return <p className="text-xs text-text-secondary text-center py-6">暂无数据（需设备被采集后才有样本）</p>;
  }
  return <PanelChart series={series} chartType={panel.chart_type || 'line'} />;
}

// ============================================================
// 面板编辑器弹窗
// ============================================================

function PanelEditorModal({ panel, onClose, onSaved }: {
  panel: DashboardPanel | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();

  const [form, setForm] = useState({
    title: panel?.title || '',
    metric_name: panel?.metric_name || '',
    chart_type: panel?.chart_type || 'line',
    aggregation: panel?.aggregation || 'avg',
    time_range: panel?.time_range || '24h',
    tag_filters: prettyJSON(safeParseJSON<Record<string, string>>(panel?.tag_filters || '', {})),
    group_by: (safeParseJSON<string[]>(panel?.group_by || '', [])).join(', '),
  });
  const [saving, setSaving] = useState(false);

  const { data: defs = [] } = useQuery({
    queryKey: ['snmp-metric-definitions-all'],
    queryFn: () => api.get('/snmp-metrics/definitions').then(res => res.data.data || []),
    staleTime: 60000,
  });

  const saveMutation = useMutation({
    mutationFn: async () => {
      const tagFilters = safeParseJSON<Record<string, string>>(form.tag_filters, {});
      const groupBy = form.group_by.split(',').map(s => s.trim()).filter(Boolean);
      const payload = {
        title: form.title,
        metric_name: form.metric_name,
        chart_type: form.chart_type,
        aggregation: form.aggregation,
        time_range: form.time_range,
        tag_filters: JSON.stringify(tagFilters),
        group_by: JSON.stringify(groupBy),
      };
      if (panel) {
        await api.put(`/snmp-metrics/panels/${panel.id}`, payload);
      } else {
        await api.post('/snmp-metrics/panels', payload);
      }
    },
    onSuccess: () => {
      toast.success(panel ? '面板已更新' : '面板已创建');
      onSaved();
    },
    onError: (e: any) => toast.error(e.message || '保存失败'),
  });

  const handleSave = () => {
    if (!form.title.trim() || !form.metric_name.trim()) {
      toast.error('标题与指标名不能为空');
      return;
    }
    setSaving(true);
    saveMutation.mutate(undefined, { onSettled: () => setSaving(false) });
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-lg max-h-[85vh] overflow-y-auto p-6 shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-text flex items-center gap-2">
            <BarChart3 className="w-5 h-5 text-blue-400" />
            {panel ? '编辑面板' : '新建面板'}
          </h2>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-secondary rounded-md"><X className="w-4 h-4 text-text-secondary" /></button>
        </div>

        <div className="space-y-3">
          <div>
            <label className="block text-xs font-medium text-text mb-1">面板标题</label>
            <input
              value={form.title}
              onChange={e => setForm({ ...form, title: e.target.value })}
              placeholder="如：接口入向流量"
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-text mb-1">指标</label>
            <select
              value={form.metric_name}
              onChange={e => setForm({ ...form, metric_name: e.target.value })}
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500"
            >
              <option value="">请选择指标</option>
              {defs.map((d: MetricDefinition) => (
                <option key={d.id} value={d.metric_name}>{d.metric_name} {d.module_name ? `(${d.module_name})` : ''}</option>
              ))}
            </select>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs font-medium text-text mb-1">图表</label>
              <select value={form.chart_type} onChange={e => setForm({ ...form, chart_type: e.target.value })}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500">
                {CHART_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium text-text mb-1">聚合</label>
              <select value={form.aggregation} onChange={e => setForm({ ...form, aggregation: e.target.value })}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500">
                {AGG_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium text-text mb-1">时间范围</label>
              <select value={form.time_range} onChange={e => setForm({ ...form, time_range: e.target.value })}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500">
                {TIME_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
              </select>
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-text mb-1">标签过滤（JSON）</label>
            <textarea
              value={form.tag_filters}
              onChange={e => setForm({ ...form, tag_filters: e.target.value })}
              rows={2}
              placeholder='{"ifDescr":"Gi0/1"}'
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm font-mono text-text focus:outline-none focus:border-blue-500 resize-none"
            />
          </div>

          <div>
            <label className="block text-xs font-medium text-text mb-1">分组字段（逗号分隔）</label>
            <input
              value={form.group_by}
              onChange={e => setForm({ ...form, group_by: e.target.value })}
              placeholder="ifDescr, ifName"
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>

        <div className="flex justify-end gap-3 mt-5">
          <button onClick={onClose} className="px-4 py-2 text-sm text-text-secondary hover:text-text transition-colors">取消</button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all disabled:opacity-50"
          >
            {saving && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            保存
          </button>
        </div>
      </div>
    </div>
  );
}

// ============================================================
// generator.yml 导入子页
// ============================================================

function GeneratorSubTab() {
  const { user } = useAuth();
  const isAdmin = user?.role === 'admin';
  const toast = useToast();

  const [module, setModule] = useState('if_mib');
  const [walk, setWalk] = useState('1.3.6.1.2.1.2.2\n1.3.6.1.2.1.2.2.1.10');
  const [lookups, setLookups] = useState('1.3.6.1.2.1.2.2.1.2|ifDescr');
  const [submitting, setSubmitting] = useState(false);
  const [result, setResult] = useState<any>(null);

  const handleSubmit = async () => {
    if (!isAdmin) { toast.error('仅管理员可执行导入'); return; }
    if (!module.trim()) { toast.error('module 不能为空'); return; }
    const walkList = walk.split('\n').map(s => s.trim()).filter(Boolean);
    if (walkList.length === 0) { toast.error('walk 不能为空'); return; }
    const lookupList = lookups.split('\n').map(s => s.trim()).filter(Boolean).map(line => {
      const [oid, labelName] = line.split('|');
      return { oid: (oid || '').trim(), label_name: (labelName || '').trim() };
    }).filter(l => l.oid && l.label_name);

    setSubmitting(true);
    setResult(null);
    try {
      const resp = await api.post('/snmp-metrics/generator', { module: module.trim(), walk: walkList, lookups: lookupList });
      setResult(resp.data.data);
      toast.success(`导入成功，新增 ${resp.data.data?.created ?? 0} 个指标定义`);
    } catch (e: any) {
      toast.error(e.message || '导入失败');
    }
    setSubmitting(false);
  };

  if (!isAdmin) {
    return (
      <div className="bg-surface rounded-xl border border-border p-12 text-center">
        <Upload className="w-10 h-10 text-text-secondary mx-auto mb-3" />
        <p className="text-sm text-text-secondary">仅管理员可执行 generator.yml 风格指标导入</p>
      </div>
    );
  }

  return (
    <div className="bg-surface rounded-xl border border-border p-4 space-y-4">
      <h3 className="text-sm font-medium text-text flex items-center gap-2">
        <Upload className="w-4 h-4 text-blue-400" />
        generator.yml 风格导入（module + walk + lookups → 命名指标）
      </h3>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label className="block text-xs font-medium text-text mb-1">Module</label>
          <input
            value={module}
            onChange={e => setModule(e.target.value)}
            placeholder="如 if_mib"
            className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500"
          />
        </div>
        <div className="md:col-span-2">
          <label className="block text-xs font-medium text-text mb-1">Walk（每行一个 OID，表根或末级列）</label>
          <textarea
            value={walk}
            onChange={e => setWalk(e.target.value)}
            rows={3}
            placeholder={'1.3.6.1.2.1.2.2\n1.3.6.1.2.1.2.2.1.10'}
            className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm font-mono text-text focus:outline-none focus:border-blue-500 resize-none"
          />
        </div>
      </div>

      <div>
        <label className="block text-xs font-medium text-text mb-1">Lookups（每行：OID|标签名，作为派生标签源）</label>
        <textarea
          value={lookups}
          onChange={e => setLookups(e.target.value)}
          rows={2}
          placeholder={'1.3.6.1.2.1.2.2.1.2|ifDescr'}
          className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm font-mono text-text focus:outline-none focus:border-blue-500 resize-none"
        />
      </div>

      <div className="flex items-center gap-3">
        <button
          onClick={handleSubmit}
          disabled={submitting}
          className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all disabled:opacity-50"
        >
          {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
          解析并导入
        </button>
        {result && (
          <span className="text-xs text-emerald-400 flex items-center gap-1">
            <Activity className="w-3.5 h-3.5" />
            module={result.module}，created={result.created}
          </span>
        )}
      </div>
    </div>
  );
}

// ============================================================
// SVG 图表（无第三方图表库：折线 / 柱状，多序列）
// ============================================================

function PanelChart({ series, chartType }: { series: MetricSeries[]; chartType: string }) {
  const w = 520, h = 180, pad = 36;
  const allPoints = series.flatMap(s => s.points);
  if (allPoints.length < 2) {
    return <p className="text-xs text-text-secondary text-center py-6">数值点不足，无法绘制</p>;
  }

  const values = allPoints.map(p => p.value);
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = (max - min) || 1;

  // 统一 X 轴时间范围
  const times = allPoints.map(p => new Date(p.timestamp).getTime());
  const tMin = Math.min(...times);
  const tMax = Math.max(...times);
  const tRange = (tMax - tMin) || 1;

  const xOf = (t: number) => pad + ((t - tMin) / tRange) * (w - pad * 2);
  const yOf = (v: number) => h - pad - ((v - min) / range) * (h - pad * 2);

  const yTicks = [0, 0.5, 1].map(t => ({ y: h - pad - t * (h - pad * 2), val: min + t * range }));

  if (chartType === 'bar') {
    // 柱状：取每序列最新点
    const latest = series.map(s => s.points[s.points.length - 1]).filter(Boolean);
    const bw = (w - pad * 2) / Math.max(latest.length, 1) - 8;
    return (
      <div className="overflow-x-auto">
        <svg width={w} height={h} className="block">
          {yTicks.map((t, i) => (
            <g key={i}>
              <line x1={pad} y1={t.y} x2={w - pad} y2={t.y} stroke="#1e293b" strokeWidth="0.5" />
              <text x={pad - 5} y={t.y + 3} fill="#64748b" fontSize="9" textAnchor="end">{fmtNum(t.val)}</text>
            </g>
          ))}
          {latest.map((p, i) => {
            const x = pad + i * ((w - pad * 2) / latest.length) + 4;
            const y = yOf(p.value);
            const bh = (h - pad) - y;
            const color = SERIES_COLORS[i % SERIES_COLORS.length];
            return <rect key={i} x={x} y={y} width={Math.max(bw, 4)} height={Math.max(bh, 1)} fill={color} opacity="0.8" />;
          })}
        </svg>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto">
      <svg width={w} height={h} className="block">
        {yTicks.map((t, i) => (
          <g key={i}>
            <line x1={pad} y1={t.y} x2={w - pad} y2={t.y} stroke="#1e293b" strokeWidth="0.5" />
            <text x={pad - 5} y={t.y + 3} fill="#64748b" fontSize="9" textAnchor="end">{fmtNum(t.val)}</text>
          </g>
        ))}
        {series.map((s, si) => {
          const color = SERIES_COLORS[si % SERIES_COLORS.length];
          const pts = [...s.points].sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime());
          const path = `M ${pts.map(p => `${xOf(new Date(p.timestamp).getTime())},${yOf(p.value)}`).join(' L ')}`;
          const label = s.tags?.ifDescr || s.tags?.ifName || s.device_id || `序列${si + 1}`;
          return (
            <g key={si}>
              <path d={path} fill="none" stroke={color} strokeWidth="1.5" />
              {pts.map((p, i) => <circle key={i} cx={xOf(new Date(p.timestamp).getTime())} cy={yOf(p.value)} r="1.8" fill={color} />)}
              <text x={xOf(new Date(pts[0].timestamp).getTime())} y={yOf(pts[0].value) - 5} fill={color} fontSize="8">{label}</text>
            </g>
          );
        })}
      </svg>
    </div>
  );
}

// ============================================================
// 辅助
// ============================================================

function safeParseJSON<T>(s: string, fallback: T): T {
  if (!s || !s.trim()) return fallback;
  try { return JSON.parse(s) as T; } catch { return fallback; }
}

function prettyJSON(v: any): string {
  try { return JSON.stringify(v, null, 2); } catch { return '{}'; }
}

function fmtNum(n: number): string {
  if (Math.abs(n) >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (Math.abs(n) >= 1e6) return (n / 1e6).toFixed(2) + 'M';
  if (Math.abs(n) >= 1e3) return (n / 1e3).toFixed(2) + 'K';
  if (Number.isInteger(n)) return n.toString();
  return n.toFixed(2);
}
