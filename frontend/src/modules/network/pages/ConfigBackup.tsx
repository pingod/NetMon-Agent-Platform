/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo, Fragment, useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Modal } from 'antd';
import {
  Plus, Trash2, RefreshCw, Play, Clock, FileText,
  ChevronRight, X, Loader2, Search, Copy, Download,
  ToggleLeft, ToggleRight, Settings, Shield, Server,
  Calendar, CheckCircle2, XCircle, AlertTriangle, Zap,
  Edit2, RotateCcw, Sparkles, Check, Terminal, Layers
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';
import type {
  ConfigBackupTemplate, ConfigBackupTemplateInput,
  ConfigBackupTask, ConfigBackupTaskInput,
  ConfigBackupRecord, BackupDeviceResult
} from '../../../modules/network/api';

// ============================================================
// 常量
// ============================================================

const VENDOR_OPTIONS = [
  { value: 'huawei', label: '华为 (Huawei)' },
  { value: 'h3c', label: '华三 (H3C)' },
  { value: 'cisco', label: '思科 (Cisco)' },
  { value: 'zte', label: '中兴 (ZTE)' },
  { value: 'ruijie', label: '锐捷 (Ruijie)' },
  { value: 'fortinet', label: '飞塔 (Fortinet)' },
  { value: 'paloalto', label: 'Palo Alto' },
  { value: 'juniper', label: 'Juniper (瞻博)' },
  { value: 'custom', label: '通用 / 自定义' },
];

const VENDOR_BADGE_STYLES: Record<string, { bg: string; text: string; border: string }> = {
  huawei: { bg: 'bg-red-500/10', text: 'text-red-600', border: 'border-red-500/30' },
  h3c: { bg: 'bg-blue-500/10', text: 'text-blue-600', border: 'border-blue-500/30' },
  cisco: { bg: 'bg-cyan-500/10', text: 'text-cyan-600', border: 'border-cyan-500/30' },
  zte: { bg: 'bg-indigo-500/10', text: 'text-indigo-600', border: 'border-indigo-500/30' },
  ruijie: { bg: 'bg-amber-500/10', text: 'text-amber-600', border: 'border-amber-500/30' },
  fortinet: { bg: 'bg-rose-500/10', text: 'text-rose-400', border: 'border-rose-500/30' },
  juniper: { bg: 'bg-emerald-500/10', text: 'text-emerald-600', border: 'border-emerald-500/30' },
  paloalto: { bg: 'bg-orange-500/10', text: 'text-orange-600', border: 'border-orange-500/30' },
  custom: { bg: 'bg-purple-500/10', text: 'text-purple-600', border: 'border-purple-500/30' },
};

const DEFAULT_VENDOR_COMMANDS: Record<string, string[]> = {
  huawei: ['display current-configuration'],
  h3c: ['display current-configuration'],
  cisco: ['show running-config'],
  zte: ['show running-config'],
  ruijie: ['show running-config'],
  fortinet: ['show full-configuration'],
  juniper: ['show configuration | display set'],
  paloalto: ['show config running'],
  custom: ['show running-config'],
};

const SCHEDULE_OPTIONS = [
  { value: 'manual', label: '手动执行' },
  { value: 'daily', label: '每天' },
  { value: 'weekly', label: '每周' },
  { value: 'monthly', label: '每月' },
  { value: 'cron', label: 'Cron 表达式' },
];

// ============================================================
// 主组件
// ============================================================

export default function ConfigBackup() {
  const [activeTab, setActiveTab] = useState<'templates' | 'tasks' | 'records'>('templates');
  const toast = useToast();

  return (
    <div className="p-6 h-full flex flex-col">
      {/* 页头 */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-text-primary">配置备份</h1>
          <p className="text-sm text-text-secondary mt-1">网络设备配置备份管理，支持主流厂商内置预置指令、自定义指令与定时备份</p>
        </div>
      </div>

      {/* 标签页 */}
      <div className="flex gap-1 mb-6 bg-surface rounded-lg border border-border p-1 w-fit">
        {[
          { key: 'templates' as const, label: '备份模板', icon: FileText },
          { key: 'tasks' as const, label: '备份任务', icon: Calendar },
          { key: 'records' as const, label: '备份记录', icon: Shield },
        ].map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-all ${
              activeTab === tab.key
                ? 'bg-gradient-to-r from-blue-600 to-blue-700 text-white shadow-sm'
                : 'text-text-secondary hover:text-text-primary hover:bg-surface-2'
            }`}
          >
            <tab.icon className="w-4 h-4" />
            {tab.label}
          </button>
        ))}
      </div>

      {/* 内容 */}
      <div className="flex-1 min-h-0">
        {activeTab === 'templates' && <TemplatesTab />}
        {activeTab === 'tasks' && <TasksTab />}
        {activeTab === 'records' && <RecordsTab />}
      </div>
    </div>
  );
}

// ============================================================
// 模板管理 Tab
// ============================================================

function TemplatesTab() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [showModal, setShowModal] = useState(false);
  const [editingTemplate, setEditingTemplate] = useState<ConfigBackupTemplate | null>(null);
  const [vendorFilter, setVendorFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const { data: templates = [], isLoading } = useQuery({
    queryKey: ['config-backup-templates'],
    queryFn: () => api.get('/config-backup/templates').then(res => res.data.data || []),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/config-backup/templates/${id}`),
    onSuccess: () => {
      toast.success('模板已成功删除');
      queryClient.invalidateQueries({ queryKey: ['config-backup-templates'] });
    },
    onError: (err: any) => {
      const msg = err?.response?.data?.error || err?.response?.data?.message || '删除模板失败';
      toast.error(msg);
    },
  });

  const restoreMutation = useMutation({
    mutationFn: () => api.post('/config-backup/templates/restore-presets'),
    onSuccess: (res) => {
      const count = res.data?.data?.length || 0;
      toast.success(`已成功载入/恢复预置模板 (共 ${count} 个)`);
      queryClient.invalidateQueries({ queryKey: ['config-backup-templates'] });
    },
    onError: () => toast.error('恢复预置模板失败'),
  });

  const handleCopyCommands = (id: string, commands: string[]) => {
    navigator.clipboard.writeText(commands.join('\n'));
    setCopiedId(id);
    toast.success('指令已复制到剪贴板');
    setTimeout(() => setCopiedId(null), 2000);
  };

  // 过滤后的模板列表
  const filteredTemplates = useMemo(() => {
    return templates.filter((t: any) => {
      const matchVendor = vendorFilter === 'all' || t.vendor === vendorFilter;
      const q = searchQuery.trim().toLowerCase();
      if (!q) return matchVendor;
      const matchName = t.name?.toLowerCase().includes(q);
      const matchDesc = t.description?.toLowerCase().includes(q);
      const matchCmds = (Array.isArray(t.commands) ? t.commands.join(' ') : String(t.commands)).toLowerCase().includes(q);
      return matchVendor && (matchName || matchDesc || matchCmds);
    });
  }, [templates, vendorFilter, searchQuery]);

  // 各厂商模板数量统计
  const vendorCounts = useMemo(() => {
    const counts: Record<string, number> = { all: templates.length };
    templates.forEach((t: any) => {
      counts[t.vendor] = (counts[t.vendor] || 0) + 1;
    });
    return counts;
  }, [templates]);

  return (
    <div className="space-y-4">
      {/* 顶部工具栏 */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 bg-surface p-4 rounded-xl border border-border">
        <div>
          <h2 className="text-base font-semibold text-text-primary flex items-center gap-2">
            <FileText className="w-4 h-4 text-blue-600" />
            备份指令模板库
          </h2>
          <p className="text-xs text-text-secondary mt-0.5">
            内置主流厂商（华为、华三、思科、中兴、锐捷等）常用备份命令，支持在线修改、删除与自定义扩展
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => {
              Modal.confirm({
                title: '恢复默认模板',
                content: '将恢复/补全所有主流厂商默认备份模板，自定义模板不受影响。是否继续？',
                okText: '恢复',
                cancelText: '取消',
                onOk: () => restoreMutation.mutate(),
              });
            }}
            disabled={restoreMutation.isPending}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-surface-2 border border-border text-text-primary text-xs font-medium rounded-lg hover:border-blue-500/50 hover:bg-blue-500/5 transition-all disabled:opacity-50 cursor-pointer"
            title="一键载入/恢复华为、华三、思科、中兴、锐捷等默认预置模板"
          >
            <RotateCcw className={`w-3.5 h-3.5 text-blue-600 ${restoreMutation.isPending ? 'animate-spin' : ''}`} />
            <span>恢复默认模板</span>
          </button>
          <button
            onClick={() => { setEditingTemplate(null); setShowModal(true); }}
            className="flex items-center gap-1.5 px-3.5 py-1.5 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-xs font-medium rounded-lg hover:from-blue-700 hover:to-blue-800 shadow-sm hover:shadow transition-all cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            新建模板
          </button>
        </div>
      </div>

      {/* 厂商分类筛选 & 搜索栏 */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-3">
        <div className="flex flex-wrap gap-1.5">
          <button
            onClick={() => setVendorFilter('all')}
            className={`px-3 py-1 rounded-lg text-xs font-medium transition-all ${
              vendorFilter === 'all'
                ? 'bg-blue-600 text-white shadow-sm'
                : 'bg-surface border border-border text-text-secondary hover:text-text-primary hover:bg-surface-2'
            }`}
          >
            全部 ({vendorCounts.all || 0})
          </button>
          {VENDOR_OPTIONS.map(v => {
            const count = vendorCounts[v.value] || 0;
            return (
              <button
                key={v.value}
                onClick={() => setVendorFilter(v.value)}
                className={`px-2.5 py-1 rounded-lg text-xs font-medium transition-all flex items-center gap-1.5 ${
                  vendorFilter === v.value
                    ? 'bg-blue-600 text-white shadow-sm'
                    : 'bg-surface border border-border text-text-secondary hover:text-text-primary hover:bg-surface-2'
                }`}
              >
                <span>{v.label.split(' ')[0]}</span>
                {count > 0 && (
                  <span className={`px-1.5 py-0.2 rounded-full text-[10px] ${
                    vendorFilter === v.value ? 'bg-white/20 text-white' : 'bg-surface-2 text-text-secondary'
                  }`}>
                    {count}
                  </span>
                )}
              </button>
            );
          })}
        </div>

        <div className="relative min-w-[220px]">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-secondary" />
          <input
            type="text"
            value={searchQuery}
            onChange={e => setSearchQuery(e.target.value)}
            placeholder="搜索模板名称、指令..."
            className="w-full pl-8 pr-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-blue-500"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery('')}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text-primary"
            >
              <X className="w-3 h-3" />
            </button>
          )}
        </div>
      </div>

      {/* 模板列表 */}
      {isLoading ? (
        <div className="flex flex-col items-center justify-center py-20 bg-surface rounded-xl border border-border">
          <Loader2 className="w-8 h-8 animate-spin text-blue-500 mb-2" />
          <span className="text-xs text-text-secondary">正在加载备份模板...</span>
        </div>
      ) : filteredTemplates.length === 0 ? (
        <div className="flex flex-col items-center justify-center py-16 bg-surface rounded-xl border border-border text-center">
          <FileText className="w-12 h-12 text-text-tertiary/40 mb-3" />
          <p className="text-sm font-medium text-text-primary">未找到符合条件的备份模板</p>
          <p className="text-xs text-text-secondary mt-1 max-w-sm">
            {templates.length === 0
              ? '当前系统尚未初始化模板，您可以一键恢复官方主流厂商默认模板，或手动新建模板'
              : '未搜索到匹配的模板，请尝试清空筛选条件或搜索关键词'}
          </p>
          <div className="flex items-center gap-2 mt-4">
            {templates.length === 0 && (
              <button
                onClick={() => restoreMutation.mutate()}
                disabled={restoreMutation.isPending}
                className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white text-xs font-medium rounded-lg transition-all cursor-pointer"
              >
                <Sparkles className="w-3.5 h-3.5" />
                一键载入主流厂商默认模板
              </button>
            )}
            <button
              onClick={() => { setEditingTemplate(null); setShowModal(true); }}
              className="flex items-center gap-1.5 px-3 py-1.5 bg-surface-2 border border-border text-text-primary text-xs font-medium rounded-lg hover:bg-surface transition-all cursor-pointer"
            >
              <Plus className="w-3.5 h-3.5" />
              新建自定义模板
            </button>
          </div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredTemplates.map((t: any) => {
            const commands = Array.isArray(t.commands)
              ? t.commands
              : (typeof t.commands === 'string' ? JSON.parse(t.commands || '[]') : []) as string[];
            const vendorOption = VENDOR_OPTIONS.find(v => v.value === t.vendor);
            const vendorLabel = vendorOption?.label || t.vendor;
            const badgeStyle = VENDOR_BADGE_STYLES[t.vendor] || VENDOR_BADGE_STYLES.custom;

            return (
              <div
                key={t.id}
                className="bg-surface rounded-xl border border-border hover:border-blue-500/40 p-4 transition-all flex flex-col justify-between group shadow-sm hover:shadow"
              >
                <div>
                  {/* 卡片头部 */}
                  <div className="flex items-start justify-between gap-2 mb-2.5">
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <span className={`px-2 py-0.5 text-xs font-medium rounded-md border ${badgeStyle.bg} ${badgeStyle.text} ${badgeStyle.border}`}>
                        {vendorLabel}
                      </span>
                      <span className={`px-1.5 py-0.5 text-[11px] rounded-md font-medium ${
                        t.is_builtin ? 'bg-blue-500/10 text-blue-600' : 'bg-emerald-500/10 text-emerald-600'
                      }`}>
                        {t.is_builtin ? '官方内置' : '自定义'}
                      </span>
                      <span className="text-[11px] text-text-secondary px-1.5 py-0.5 rounded bg-surface-2">
                        {commands.length} 条指令
                      </span>
                    </div>

                    {/* 操作按钮组 */}
                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => { setEditingTemplate(t); setShowModal(true); }}
                        className="p-1.5 rounded-lg text-text-secondary hover:text-blue-600 hover:bg-blue-500/10 transition-colors cursor-pointer"
                        title="编辑模板"
                      >
                        <Edit2 className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => {
                          Modal.confirm({
                            title: '确认删除模板',
                            content: `确定要删除模板「${t.name}」吗？使用此模板的备份任务将无法执行。`,
                            okText: '删除',
                            okType: 'danger',
                            cancelText: '取消',
                            onOk: () => deleteMutation.mutate(t.id),
                          });
                        }}
                        className="p-1.5 rounded-lg text-text-secondary hover:text-red-600 hover:bg-red-500/10 transition-colors cursor-pointer"
                        title="删除模板"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>

                  {/* 模板标题与描述 */}
                  <h3 className="font-semibold text-sm text-text-primary mb-1 flex items-center gap-1.5">
                    <Terminal className="w-3.5 h-3.5 text-blue-600 flex-shrink-0" />
                    <span className="truncate">{t.name}</span>
                  </h3>
                  {t.description && (
                    <p className="text-xs text-text-secondary mb-3 line-clamp-2 leading-relaxed">
                      {t.description}
                    </p>
                  )}

                  {/* 指令代码块 */}
                  <div className="space-y-1 my-2 bg-surface-2/70 rounded-lg p-2 border border-border/50">
                    <div className="flex items-center justify-between text-[11px] text-text-secondary mb-1">
                      <span>执行指令清单</span>
                      <button
                        onClick={() => handleCopyCommands(t.id, commands)}
                        className="flex items-center gap-1 text-[10px] text-blue-600 hover:text-blue-300 transition-colors"
                      >
                        {copiedId === t.id ? (
                          <>
                            <Check className="w-3 h-3 text-emerald-600" />
                            <span className="text-emerald-600">已复制</span>
                          </>
                        ) : (
                          <>
                            <Copy className="w-3 h-3" />
                            <span>复制</span>
                          </>
                        )}
                      </button>
                    </div>
                    {commands.map((cmd: string, i: number) => (
                      <div key={i} className="flex items-center gap-1.5 text-xs font-mono bg-surface rounded px-2 py-1 text-text-primary border border-border/30">
                        <ChevronRight className="w-3 h-3 text-blue-600 flex-shrink-0" />
                        <span className="truncate">{cmd}</span>
                      </div>
                    ))}
                  </div>
                </div>

                {/* 卡片底栏：快捷操作 */}
                <div className="pt-2 mt-2 border-t border-border/40 flex items-center justify-between text-[11px] text-text-secondary">
                  <span className="text-[10px] text-text-tertiary font-mono">
                    ID: {t.id.startsWith('tpl-builtin-') ? t.id.replace('tpl-builtin-', '') : t.id.slice(0, 8)}
                  </span>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => { setEditingTemplate(t); setShowModal(true); }}
                      className="text-xs text-blue-600 hover:text-blue-700 hover:underline flex items-center gap-1 cursor-pointer"
                    >
                      <Edit2 className="w-3 h-3" />
                      <span>编辑</span>
                    </button>
                    <button
                      onClick={() => {
                        Modal.confirm({
                          title: '确认删除模板',
                          content: `确定要删除模板「${t.name}」吗？使用此模板的备份任务将无法执行。`,
                          okText: '删除',
                          okType: 'danger',
                          cancelText: '取消',
                          onOk: () => deleteMutation.mutate(t.id),
                        });
                      }}
                      className="text-xs text-red-600 hover:text-red-700 hover:underline flex items-center gap-1 cursor-pointer"
                    >
                      <Trash2 className="w-3 h-3" />
                      <span>删除</span>
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {showModal && (
        <TemplateModal
          template={editingTemplate}
          onClose={() => { setShowModal(false); setEditingTemplate(null); }}
          onSave={() => {
            setShowModal(false);
            setEditingTemplate(null);
            queryClient.invalidateQueries({ queryKey: ['config-backup-templates'] });
          }}
        />
      )}
    </div>
  );
}

// ============================================================
// 任务管理 Tab
// ============================================================

function TasksTab() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [showModal, setShowModal] = useState(false);
  const [editingTask, setEditingTask] = useState<ConfigBackupTask | null>(null);
  const [executingId, setExecutingId] = useState<string | null>(null);
  const [backupProgress, setBackupProgress] = useState<{ total: number; completed: number; success: number; failed: number; is_running: boolean } | null>(null);
  const [showQuickBackup, setShowQuickBackup] = useState(false);


  // 进度轮询定时器与挂载标记，卸载后不再 setState
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (pollTimerRef.current) {
        clearTimeout(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, []);
  const { data: tasks = [], isLoading } = useQuery({
    queryKey: ['config-backup-tasks'],
    queryFn: () => api.get('/config-backup/tasks').then(res => res.data.data || []),
  });

  const { data: templates = [] } = useQuery({
    queryKey: ['config-backup-templates'],
    queryFn: () => api.get('/config-backup/templates').then(res => res.data.data || []),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/config-backup/tasks/${id}`),
    onSuccess: () => {
      toast.success('任务已删除');
      queryClient.invalidateQueries({ queryKey: ['config-backup-tasks'] });
    },
    onError: () => toast.error('删除任务失败'),
  });

  const toggleMutation = useMutation({
    mutationFn: (id: string) => api.post(`/config-backup/tasks/${id}/toggle`),
    onSuccess: () => {
      toast.success('任务状态已切换');
      queryClient.invalidateQueries({ queryKey: ['config-backup-tasks'] });
    },
    onError: () => toast.error('切换任务状态失败'),
  });

  const executeMutation = useMutation({
    mutationFn: async (id: string) => {
      setExecutingId(id);
      setBackupProgress(null);
      return api.post(`/config-backup/tasks/${id}/execute`);
    },
    onSuccess: (_data, id) => {
      toast.success('备份任务已开始执行');
      // 开始轮询进度
      pollProgress(id);
    },
    onError: () => {
      toast.error('执行备份任务失败');
      setExecutingId(null);
    },
  });

  const pollProgress = (taskId: string) => {
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
    }
    const poll = async () => {
      try {
        const res = await api.get(`/config-backup/progress/${taskId}`);
        if (!mountedRef.current) return;
        const progress = res.data?.data;
        // progress 为空说明后端已无该任务进度记录，按结束处理
        if (progress && progress.is_running) {
          setBackupProgress(progress);
          pollTimerRef.current = setTimeout(poll, 1500);
          return;
        }
        pollTimerRef.current = null;
        setExecutingId(null);
        setBackupProgress(null);
        queryClient.invalidateQueries({ queryKey: ['config-backup-tasks'] });
        queryClient.invalidateQueries({ queryKey: ['config-backup-records'] });
        if (progress) {
          if (progress.failed === 0) {
            toast.success(`备份完成: ${progress.success} 台设备成功`);
          } else if (progress.success === 0) {
            toast.error(`备份失败: ${progress.failed} 台设备失败`);
          } else {
            toast.success(`备份完成: ${progress.success} 成功, ${progress.failed} 失败`);
          }
        }
      } catch {
        // 轮询出错时停止
        if (!mountedRef.current) return;
        pollTimerRef.current = null;
        setExecutingId(null);
        setBackupProgress(null);
      }
    };
    pollTimerRef.current = setTimeout(poll, 1000);
  };

  const scheduleLabel = (type: string) => SCHEDULE_OPTIONS.find(s => s.value === type)?.label || type;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-text-secondary">管理定时备份任务，或快速选择设备执行一次性备份</p>
        <div className="flex gap-2">
          <button
            onClick={() => setShowQuickBackup(true)}
            className="flex items-center gap-2 px-4 py-2 bg-surface border border-border text-text-primary text-sm font-medium rounded-md hover:bg-surface-2 transition-all"
          >
            <Zap className="w-4 h-4" />
            快速备份
          </button>
          <button
            onClick={() => { setEditingTask(null); setShowModal(true); }}
            className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all"
          >
            <Plus className="w-4 h-4" />
            新建任务
          </button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-20"><Loader2 className="w-6 h-6 animate-spin text-blue-500" /></div>
      ) : tasks.length === 0 ? (
        <div className="text-center py-20 text-text-secondary">暂无备份任务，点击"新建任务"或"快速备份"开始</div>
      ) : (
        <div className="bg-surface rounded-xl border border-border overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border bg-surface-2/30">
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">任务名称</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">模板</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">调度</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">设备数</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">状态</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">上次执行</th>
                <th className="text-right text-xs font-medium text-text-secondary px-4 py-3">操作</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((task: any) => (
                <Fragment key={task.id}>
                <tr className="border-b border-border/50 hover:bg-surface-2/20 transition-colors">
                  <td className="px-4 py-3">
                    <span className="font-medium text-text-primary">{task.name}</span>
                  </td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{task.template_name || '-'}</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-1.5">
                      <Clock className="w-3.5 h-3.5 text-text-secondary" />
                      <span className="text-sm text-text-secondary">{scheduleLabel(task.schedule_type)}</span>
                      {task.schedule_type === 'cron' && (
                        <span className="text-xs font-mono text-blue-600">{task.schedule_cron}</span>
                      )}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{task.device_count || 0} 台</td>
                  <td className="px-4 py-3">
                    <button
                      onClick={() => toggleMutation.mutate(task.id)}
                      className="flex items-center gap-1.5"
                    >
                      {task.enabled ? (
                        <><ToggleRight className="w-5 h-5 text-emerald-600" /><span className="text-xs text-emerald-600">启用</span></>
                      ) : (
                        <><ToggleLeft className="w-5 h-5 text-text-secondary" /><span className="text-xs text-text-secondary">禁用</span></>
                      )}
                    </button>
                  </td>
                  <td className="px-4 py-3">
                    {task.last_run_at ? (
                      <div className="flex items-center gap-1.5">
                        {task.last_run_status === 'success' ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600" /> :
                         task.last_run_status === 'failed' ? <XCircle className="w-3.5 h-3.5 text-red-600" /> :
                         <AlertTriangle className="w-3.5 h-3.5 text-yellow-600" />}
                        <span className="text-xs text-text-secondary">{new Date(task.last_run_at).toLocaleString()}</span>
                      </div>
                    ) : <span className="text-xs text-text-secondary">未执行</span>}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <button
                        onClick={() => executeMutation.mutate(task.id)}
                        disabled={executingId === task.id}
                        className="flex items-center gap-1 px-2.5 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-500/10 rounded-md transition-colors disabled:opacity-50"
                      >
                        {executingId === task.id ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
                        执行
                      </button>
                      <button
                        onClick={() => { setEditingTask(task); setShowModal(true); }}
                        className="p-1.5 text-text-secondary hover:text-text-primary hover:bg-surface-2 rounded-md transition-colors"
                      >
                        <Settings className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => {
                          Modal.confirm({
                            title: '确认删除任务',
                            content: `确定删除任务「${task.name}」吗？关联的备份记录也将被删除。`,
                            okText: '删除',
                            okType: 'danger',
                            cancelText: '取消',
                            onOk: () => deleteMutation.mutate(task.id),
                          });
                        }}
                        className="p-1.5 text-text-secondary hover:text-red-600 hover:bg-red-500/10 rounded-md transition-colors"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
                {executingId === task.id && backupProgress && (
                  <tr key={`progress-${task.id}`} className="bg-blue-500/5">
                    <td colSpan={7} className="px-4 py-3">
                      <div className="flex items-center gap-4">
                        <div className="flex-1">
                          <div className="h-2 bg-surface-2 rounded-full overflow-hidden">
                            <div
                              className="h-full bg-gradient-to-r from-blue-500 to-blue-400 rounded-full transition-all duration-500"
                              style={{ width: `${backupProgress.total > 0 ? (backupProgress.completed / backupProgress.total) * 100 : 0}%` }}
                            />
                          </div>
                        </div>
                        <div className="flex items-center gap-3 text-xs whitespace-nowrap">
                          <span className="text-text-secondary">
                            {backupProgress.completed}/{backupProgress.total} 台设备
                          </span>
                          {backupProgress.success > 0 && (
                            <span className="flex items-center gap-1 text-emerald-600">
                              <CheckCircle2 className="w-3 h-3" />{backupProgress.success} 成功
                            </span>
                          )}
                          {backupProgress.failed > 0 && (
                            <span className="flex items-center gap-1 text-red-600">
                              <XCircle className="w-3 h-3" />{backupProgress.failed} 失败
                            </span>
                          )}
                          {backupProgress.completed < backupProgress.total && (
                            <span className="text-text-secondary">
                              剩余 {backupProgress.total - backupProgress.completed} 台
                            </span>
                          )}
                          {backupProgress.completed >= backupProgress.total && (
                            <span className="text-emerald-600 font-medium">完成</span>
                          )}
                        </div>
                      </div>
                    </td>
                  </tr>
                )}
                </Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showModal && (
        <TaskModal
          task={editingTask}
          templates={templates}
          onClose={() => { setShowModal(false); setEditingTask(null); }}
          onSave={() => { setShowModal(false); setEditingTask(null); queryClient.invalidateQueries({ queryKey: ['config-backup-tasks'] }); }}
        />
      )}

      {showQuickBackup && (
        <QuickBackupModal
          templates={templates}
          onClose={() => setShowQuickBackup(false)}
          onDone={() => { setShowQuickBackup(false); queryClient.invalidateQueries({ queryKey: ['config-backup-records'] }); }}
        />
      )}
    </div>
  );
}

// ============================================================
// 备份记录 Tab
// ============================================================

function RecordsTab() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [selectedRecord, setSelectedRecord] = useState<ConfigBackupRecord | null>(null);
  const [filterStatus, setFilterStatus] = useState<string>('');
  const [search, setSearch] = useState('');

  const { data: recordsData, isLoading } = useQuery({
    queryKey: ['config-backup-records', filterStatus],
    queryFn: () => api.get('/config-backup/records', { params: { status: filterStatus || undefined, limit: 200 } }).then(res => res.data.data || { records: [], total: 0 }),
    refetchInterval: (queryData: any) => (queryData?.records as any[])?.some((r: any) => r.status === 'running') ? 2000 : false,
  });

  const records = (recordsData?.records || []) as ConfigBackupRecord[];
  const filteredRecords = useMemo(() => {
    if (!search) return records;
    const q = search.toLowerCase();
    return records.filter(r =>
      r.device_name?.toLowerCase().includes(q) ||
      r.device_ip?.toLowerCase().includes(q) ||
      r.task_name?.toLowerCase().includes(q)
    );
  }, [records, search]);

  const deleteRecordMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/config-backup/records/${id}`),
    onSuccess: () => {
      toast.success('记录已删除');
      queryClient.invalidateQueries({ queryKey: ['config-backup-records'] });
    },
    onError: () => toast.error('删除记录失败'),
  });

  const formatDuration = (ms?: number | null) => {
    if (!ms) return '-';
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  const formatSize = (bytes?: number | null) => {
    if (!bytes) return '-';
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
  };

  const statusIcon = (status: string) => {
    switch (status) {
      case 'success': return <CheckCircle2 className="w-4 h-4 text-emerald-600" />;
      case 'failed': return <XCircle className="w-4 h-4 text-red-600" />;
      case 'running': return <Loader2 className="w-4 h-4 text-blue-600 animate-spin" />;
      case 'partial': return <AlertTriangle className="w-4 h-4 text-yellow-600" />;
      default: return null;
    }
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <div className="relative flex-1 max-w-xs">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-secondary" />
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="搜索设备/任务..."
            className="w-full pl-9 pr-4 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-blue-500"
          />
        </div>
        <select
          value={filterStatus}
          onChange={e => setFilterStatus(e.target.value)}
          className="px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500"
        >
          <option value="">全部状态</option>
          <option value="success">成功</option>
          <option value="failed">失败</option>
          <option value="running">执行中</option>
          <option value="partial">部分成功</option>
        </select>
        <button
          onClick={() => queryClient.invalidateQueries({ queryKey: ['config-backup-records'] })}
          className="p-2 text-text-secondary hover:text-text-primary hover:bg-surface-2 rounded-lg transition-colors"
        >
          <RefreshCw className="w-4 h-4" />
        </button>
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-20"><Loader2 className="w-6 h-6 animate-spin text-blue-500" /></div>
      ) : filteredRecords.length === 0 ? (
        <div className="text-center py-20 text-text-secondary">暂无备份记录</div>
      ) : (
        <div className="bg-surface rounded-xl border border-border overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border bg-surface-2/30">
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">状态</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">设备</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">IP</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">任务</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">模板</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">大小</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">耗时</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">执行时间</th>
                <th className="text-right text-xs font-medium text-text-secondary px-4 py-3">操作</th>
              </tr>
            </thead>
            <tbody>
              {filteredRecords.map(record => (
                <tr key={record.id} className="border-b border-border/50 hover:bg-surface-2/20 transition-colors cursor-pointer"
                    onClick={() => setSelectedRecord(record)}>
                  <td className="px-4 py-3">{statusIcon(record.status)}</td>
                  <td className="px-4 py-3 text-sm text-text-primary font-medium">{record.device_name}</td>
                  <td className="px-4 py-3 text-sm text-text-secondary font-mono">{record.device_ip}</td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{record.task_name || '-'}</td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{record.template_name || '-'}</td>
                  <td className="px-4 py-3 text-sm text-text-secondary font-mono">{formatSize(record.output_size)}</td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{formatDuration(record.duration_ms)}</td>
                  <td className="px-4 py-3 text-xs text-text-secondary">{new Date(record.started_at).toLocaleString()}</td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={e => { e.stopPropagation(); deleteRecordMutation.mutate(record.id); }}
                      className="p-1.5 text-text-secondary hover:text-red-600 hover:bg-red-500/10 rounded-md transition-colors"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {selectedRecord && (
        <RecordDetailModal
          record={selectedRecord}
          onClose={() => setSelectedRecord(null)}
        />
      )}
    </div>
  );
}

// ============================================================
// 模板创建/编辑弹窗
// ============================================================

function TemplateModal({ template, onClose, onSave }: {
  template: ConfigBackupTemplate | null;
  onClose: () => void;
  onSave: () => void;
}) {
  const toast = useToast();
  const [name, setName] = useState(template?.name || '');
  const [description, setDescription] = useState(template?.description || '');
  const [vendor, setVendor] = useState(template?.vendor || 'custom');
  const [commandsText, setCommandsText] = useState<string>(
    template ? (Array.isArray(template.commands) ? template.commands : JSON.parse(template.commands || '[]')).join('\n') : ''
  );
  const [saving, setSaving] = useState(false);

  useEscapeKey({ onEscape: onClose });

  const handleVendorChange = (newVendor: string) => {
    setVendor(newVendor);
    // 如果是新建模板且名称为空或包含默认格式，自动推荐名称和指令
    if (!template) {
      const vObj = VENDOR_OPTIONS.find(v => v.value === newVendor);
      const vName = vObj?.label.split(' ')[0] || newVendor;
      if (!name || name.endsWith('配置备份模板')) {
        setName(`${vName}设备配置备份模板`);
      }
      if (!description || description.includes('适用')) {
        setDescription(`适用于 ${vName} 网络设备的配置导出与备份`);
      }
      if (!commandsText) {
        const defCmds = DEFAULT_VENDOR_COMMANDS[newVendor] || ['show running-config'];
        setCommandsText(defCmds.join('\n'));
      }
    }
  };

  const handleLoadRecommendedCommands = () => {
    const defCmds = DEFAULT_VENDOR_COMMANDS[vendor] || ['show running-config'];
    setCommandsText(defCmds.join('\n'));
    toast.success(`已填入 ${vendor.toUpperCase()} 推荐备份指令`);
  };

  const handleSave = async () => {
    if (!name.trim()) { toast.error('请输入模板名称'); return; }
    const filteredCommands = commandsText.split('\n').map(c => c.trim()).filter(c => c);
    if (filteredCommands.length === 0) { toast.error('请至少添加一条指令'); return; }

    setSaving(true);
    try {
      if (template) {
        await api.put(`/config-backup/templates/${template.id}`, { name, description, vendor, commands: filteredCommands });
        toast.success('模板已成功更新');
      } else {
        await api.post('/config-backup/templates', { name, description, vendor, commands: filteredCommands });
        toast.success('模板已成功创建');
      }
      onSave();
    } catch (err: any) {
      toast.error(err.response?.data?.error || '保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-lg shadow-2xl overflow-hidden" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-surface-2/40">
          <div className="flex items-center gap-2">
            <div className="p-2 rounded-lg bg-blue-500/10 text-blue-600">
              <Terminal className="w-4 h-4" />
            </div>
            <div>
              <h2 className="text-base font-semibold text-text-primary">{template ? '编辑备份模板' : '新建备份模板'}</h2>
              <p className="text-xs text-text-secondary">自定义或调整设备配置导出的 CLI 指令集</p>
            </div>
          </div>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-2 rounded-lg text-text-secondary hover:text-text-primary transition-colors">
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="p-6 space-y-4 max-h-[75vh] overflow-y-auto">
          <div>
            <label className="block text-xs font-semibold text-text-primary mb-1.5">厂商类型 <span className="text-red-600">*</span></label>
            <select
              value={vendor}
              onChange={e => handleVendorChange(e.target.value)}
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500 cursor-pointer"
            >
              {VENDOR_OPTIONS.map(v => (
                <option key={v.value} value={v.value}>{v.label}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="block text-xs font-semibold text-text-primary mb-1.5">模板名称 <span className="text-red-600">*</span></label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="如：华为设备配置备份模板"
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-xs text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <label className="block text-xs font-semibold text-text-primary mb-1.5">模板描述</label>
            <input
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="如：适用于华为交换机与路由器的标准配置导出"
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-xs text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-blue-500"
            />
          </div>

          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-xs font-semibold text-text-primary">备份指令清单 <span className="text-red-600">*</span></label>
              <button
                type="button"
                onClick={handleLoadRecommendedCommands}
                className="text-[11px] text-blue-600 hover:text-blue-300 flex items-center gap-1 hover:underline cursor-pointer"
              >
                <Sparkles className="w-3 h-3" />
                <span>填入厂商推荐指令</span>
              </button>
            </div>
            <textarea
              value={commandsText}
              onChange={e => setCommandsText(e.target.value)}
              placeholder={"每行一条执行指令，例如：\ndisplay current-configuration\ndisplay version"}
              rows={5}
              className="w-full px-3 py-2 bg-surface-2/70 border border-border rounded-lg text-xs font-mono text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-blue-500 resize-y leading-relaxed"
            />
            <p className="mt-1 text-[11px] text-text-secondary">
              支持多条命令按换行依次执行，备份引擎将自动汇总命令输出保存为配置文件快照。
            </p>
          </div>
        </div>

        <div className="flex justify-end gap-2.5 px-6 py-4 border-t border-border bg-surface-2/20">
          <button
            onClick={onClose}
            className="px-4 py-2 text-xs font-medium text-text-secondary hover:text-text-primary hover:bg-surface-2 rounded-lg transition-colors cursor-pointer"
          >
            取消
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex items-center gap-1.5 px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-700 hover:to-blue-800 text-white text-xs font-medium rounded-lg shadow-sm hover:shadow transition-all disabled:opacity-50 cursor-pointer"
          >
            {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Check className="w-3.5 h-3.5" />}
            <span>保存模板</span>
          </button>
        </div>
      </div>
    </div>
  );
}

// ============================================================
// 任务创建/编辑弹窗
// ============================================================

function TaskModal({ task, templates, onClose, onSave }: {
  task: ConfigBackupTask | null;
  templates: ConfigBackupTemplate[];
  onClose: () => void;
  onSave: () => void;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();

  const [name, setName] = useState(task?.name || '');
  const [templateId, setTemplateId] = useState(task?.template_id || '');
  const [scheduleType, setScheduleType] = useState(task?.schedule_type || 'manual');
  const [scheduleCron, setScheduleCron] = useState(task?.schedule_cron || '');
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(
    new Set(task ? (Array.isArray(task.device_ids) ? task.device_ids : JSON.parse(task.device_ids || '[]')) : [])
  );
  const [retentionDays, setRetentionDays] = useState(task?.retention_days || 90);
  const [enabled, setEnabled] = useState(task?.enabled ?? 1);
  const [deviceSearch, setDeviceSearch] = useState('');

  const { data: devices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
  });

  useEscapeKey({ onEscape: onClose });

  const filteredDevices = useMemo(() => {
    if (!deviceSearch) return devices;
    const q = deviceSearch.toLowerCase();
    return devices.filter((d: any) => d.name?.toLowerCase().includes(q) || d.ip_address?.toLowerCase().includes(q));
  }, [devices, deviceSearch]);

  const toggleDevice = (id: string) => {
    const s = new Set(selectedDeviceIds);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedDeviceIds(s);
  };

  const selectAllDevices = () => {
    if (selectedDeviceIds.size === filteredDevices.length) {
      setSelectedDeviceIds(new Set());
    } else {
      setSelectedDeviceIds(new Set(filteredDevices.map((d: any) => d.id)));
    }
  };

  const handleSave = async () => {
    if (!name.trim()) { toast.error('请输入任务名称'); return; }
    if (!templateId) { toast.error('请选择备份模板'); return; }
    if (selectedDeviceIds.size === 0) { toast.error('请选择至少一台设备'); return; }
    if (scheduleType === 'cron' && !scheduleCron.trim()) { toast.error('请输入 Cron 表达式'); return; }

    const payload = {
      name,
      template_id: templateId,
      schedule_type: scheduleType,
      schedule_cron: scheduleType === 'cron' ? scheduleCron : undefined,
      device_ids: Array.from(selectedDeviceIds),
      retention_days: retentionDays,
      enabled: !!enabled,
    };

    try {
      if (task) {
        await api.put(`/config-backup/tasks/${task.id}`, payload);
        toast.success('任务已更新');
      } else {
        await api.post('/config-backup/tasks', payload);
        toast.success('任务已创建');
      }
      onSave();
    } catch (err: any) {
      toast.error(err.response?.data?.error || '保存失败');
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-2xl max-h-[85vh] overflow-y-auto p-6 shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold text-text-primary">{task ? '编辑任务' : '新建任务'}</h2>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-2 rounded-md"><X className="w-4 h-4 text-text-secondary" /></button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-text-primary mb-1">任务名称</label>
            <input value={name} onChange={e => setName(e.target.value)} placeholder="如：核心交换机每日备份"
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500" />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-text-primary mb-1">备份模板</label>
              <select value={templateId} onChange={e => setTemplateId(e.target.value)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500">
                <option value="">选择模板</option>
                {templates.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
            </div>

            <div>
              <label className="block text-sm font-medium text-text-primary mb-1">调度方式</label>
              <select value={scheduleType} onChange={e => setScheduleType(e.target.value as any)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500">
                {SCHEDULE_OPTIONS.map(s => <option key={s.value} value={s.value}>{s.label}</option>)}
              </select>
            </div>
          </div>

          {scheduleType === 'cron' && (
            <div>
              <label className="block text-sm font-medium text-text-primary mb-1">Cron 表达式</label>
              <input value={scheduleCron} onChange={e => setScheduleCron(e.target.value)} placeholder="0 3 * * * (每天凌晨3点)"
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm font-mono text-text-primary focus:outline-none focus:border-blue-500" />
            </div>
          )}

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-text-primary mb-1">保留天数</label>
              <input type="number" value={retentionDays} onChange={e => setRetentionDays(parseInt(e.target.value) || 90)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500" />
            </div>
            <div className="flex items-end">
              <button onClick={() => setEnabled(enabled ? 0 : 1)}
                className="flex items-center gap-2 px-3 py-2 border border-border rounded-lg hover:bg-surface-2 transition-colors">
                {enabled ? <ToggleRight className="w-5 h-5 text-emerald-600" /> : <ToggleLeft className="w-5 h-5 text-text-secondary" />}
                <span className="text-sm">{enabled ? '启用' : '禁用'}</span>
              </button>
            </div>
          </div>

          {/* 设备选择 */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-medium text-text-primary">选择设备 ({selectedDeviceIds.size} 台已选)</label>
              <button onClick={selectAllDevices} className="text-xs text-blue-600 hover:text-blue-300">
                {selectedDeviceIds.size === filteredDevices.length ? '取消全选' : '全选'}
              </button>
            </div>
            <div className="relative mb-2">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-secondary" />
              <input value={deviceSearch} onChange={e => setDeviceSearch(e.target.value)} placeholder="搜索设备..."
                className="w-full pl-9 pr-4 py-1.5 bg-surface border border-border rounded-lg text-sm text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-blue-500" />
            </div>
            <div className="max-h-48 overflow-y-auto border border-border rounded-lg">
              {filteredDevices.length === 0 ? (
                <div className="text-center py-6 text-sm text-text-secondary">暂无设备</div>
              ) : filteredDevices.map((d: any) => (
                <div key={d.id}
                  onClick={() => toggleDevice(d.id)}
                  className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors border-b border-border/30 last:border-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500/10' : 'hover:bg-surface-2/50'
                  }`}
                >
                  <div className={`w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500 border-blue-500' : 'border-border'
                  }`}>
                    {selectedDeviceIds.has(d.id) && <CheckCircle2 className="w-3 h-3 text-white" />}
                  </div>
                  <Server className="w-4 h-4 text-text-secondary flex-shrink-0" />
                  <span className="text-sm text-text-primary">{d.name}</span>
                  <span className="text-xs text-text-secondary font-mono">{d.ip_address}</span>
                  <span className="text-xs text-text-secondary ml-auto">{d.vendor}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary transition-colors">取消</button>
          <button onClick={handleSave}
            className="px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all">
            保存
          </button>
        </div>
      </div>
    </div>
  );
}

// ============================================================
// 快速备份弹窗
// ============================================================

function QuickBackupModal({ templates, onClose, onDone }: {
  templates: ConfigBackupTemplate[];
  onClose: () => void;
  onDone: () => void;
}) {
  const toast = useToast();
  const [templateId, setTemplateId] = useState('');
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(new Set());
  const [executing, setExecuting] = useState(false);
  const [batchProgress, setBatchProgress] = useState<{ total: number; completed: number; success: number; failed: number; is_running: boolean } | null>(null);
  const [deviceSearch, setDeviceSearch] = useState('');

  const { data: devices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
  });

  useEscapeKey({ onEscape: onClose });

  // 轮询定时器随弹窗卸载清理
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      if (pollTimerRef.current !== null) {
        clearTimeout(pollTimerRef.current);
        pollTimerRef.current = null;
      }
    };
  }, []);

  const filteredDevices = useMemo(() => {
    if (!deviceSearch) return devices;
    const q = deviceSearch.toLowerCase();
    return devices.filter((d: any) => d.name?.toLowerCase().includes(q) || d.ip_address?.toLowerCase().includes(q));
  }, [devices, deviceSearch]);

  const toggleDevice = (id: string) => {
    const s = new Set(selectedDeviceIds);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedDeviceIds(s);
  };

  const selectAllDevices = () => {
    if (selectedDeviceIds.size === filteredDevices.length) {
      setSelectedDeviceIds(new Set());
    } else {
      setSelectedDeviceIds(new Set(filteredDevices.map((d: any) => d.id)));
    }
  };

  const handleExecute = async () => {
    if (!templateId) { toast.error('请选择备份模板'); return; }
    if (selectedDeviceIds.size === 0) { toast.error('请选择至少一台设备'); return; }

    setExecuting(true);
    setBatchProgress(null);
    try {
      const result = await api.post('/config-backup/batch-execute', {
        device_ids: Array.from(selectedDeviceIds),
        template_id: templateId,
      });
      const progress = result.data?.data;
      if (progress && progress.is_running) {
        // 异步模式：轮询进度
        const batchId = progress.task_id;
        const poll = async () => {
          try {
            const res = await api.get(`/config-backup/progress/${batchId}`);
            if (!mountedRef.current) return;
            const p = res.data?.data;
            if (p && p.is_running) {
              setBatchProgress(p);
              pollTimerRef.current = setTimeout(poll, 1500);
              return;
            }
            pollTimerRef.current = null;
            setExecuting(false);
            setBatchProgress(null);
            if (!p) {
              // progress 为空视为任务已结束（后端记录被清理）
              onDone();
              return;
            }
            if (p.failed === 0) {
              toast.success(`备份完成：全部 ${p.success} 台设备成功`);
            } else {
              toast.warning(`备份完成：${p.success} 台成功，${p.failed} 台失败`);
            }
            onDone();
          } catch {
            if (!mountedRef.current) return;
            pollTimerRef.current = null;
            setExecuting(false);
            setBatchProgress(null);
          }
        };
        pollTimerRef.current = setTimeout(poll, 1000);
        toast.success('批量备份已开始执行');
      } else {
        // 兼容旧版同步返回
        setExecuting(false);
        onDone();
      }
    } catch (err: any) {
      toast.error(err.response?.data?.error || '批量备份失败');
      setExecuting(false);
      setBatchProgress(null);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-lg max-h-[85vh] overflow-y-auto p-6 shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold text-text-primary">快速备份</h2>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-2 rounded-md"><X className="w-4 h-4 text-text-secondary" /></button>
        </div>

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-text-primary mb-1">备份模板</label>
            <select value={templateId} onChange={e => setTemplateId(e.target.value)}
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500">
              <option value="">选择模板</option>
              {templates.map(t => <option key={t.id} value={t.id}>{t.name}</option>)}
            </select>
          </div>

          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-medium text-text-primary">选择设备 ({selectedDeviceIds.size} 台)</label>
              <button onClick={selectAllDevices} className="text-xs text-blue-600 hover:text-blue-300">
                {selectedDeviceIds.size === filteredDevices.length ? '取消全选' : '全选'}
              </button>
            </div>
            <div className="relative mb-2">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-secondary" />
              <input value={deviceSearch} onChange={e => setDeviceSearch(e.target.value)} placeholder="搜索设备..."
                className="w-full pl-9 pr-4 py-1.5 bg-surface border border-border rounded-lg text-sm text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-blue-500" />
            </div>
            <div className="max-h-48 overflow-y-auto border border-border rounded-lg">
              {filteredDevices.length === 0 ? (
                <div className="text-center py-6 text-sm text-text-secondary">暂无设备</div>
              ) : filteredDevices.map((d: any) => (
                <div key={d.id}
                  onClick={() => toggleDevice(d.id)}
                  className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors border-b border-border/30 last:border-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500/10' : 'hover:bg-surface-2/50'
                  }`}
                >
                  <div className={`w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500 border-blue-500' : 'border-border'
                  }`}>
                    {selectedDeviceIds.has(d.id) && <CheckCircle2 className="w-3 h-3 text-white" />}
                  </div>
                  <Server className="w-4 h-4 text-text-secondary flex-shrink-0" />
                  <span className="text-sm text-text-primary">{d.name}</span>
                  <span className="text-xs text-text-secondary font-mono">{d.ip_address}</span>
                </div>
              ))}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary transition-colors" disabled={executing}>取消</button>
          <button onClick={handleExecute} disabled={executing}
            className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all disabled:opacity-50">
            {executing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
            {executing ? '执行中...' : '开始备份'}
          </button>
        </div>

        {/* 批量备份进度条 */}
        {executing && batchProgress && (
          <div className="mt-4 space-y-2">
            <div className="flex items-center justify-between text-xs text-text-secondary">
              <span>备份进度</span>
              <span>{batchProgress.completed}/{batchProgress.total} 台设备</span>
            </div>
            <div className="h-2 bg-surface-2 rounded-full overflow-hidden">
              <div
                className="h-full bg-gradient-to-r from-blue-500 to-blue-400 rounded-full transition-all duration-500"
                style={{ width: `${batchProgress.total > 0 ? (batchProgress.completed / batchProgress.total) * 100 : 0}%` }}
              />
            </div>
            <div className="flex items-center gap-3 text-xs">
              {batchProgress.success > 0 && <span className="text-emerald-600">{batchProgress.success} 成功</span>}
              {batchProgress.failed > 0 && <span className="text-red-600">{batchProgress.failed} 失败</span>}
              {batchProgress.completed < batchProgress.total && <span className="text-text-secondary">剩余 {batchProgress.total - batchProgress.completed} 台</span>}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================
// 备份记录详情弹窗
// ============================================================

function RecordDetailModal({ record, onClose }: {
  record: ConfigBackupRecord;
  onClose: () => void;
}) {
  useEscapeKey({ onEscape: onClose });

  const handleCopy = () => {
    if (record.output) {
      navigator.clipboard.writeText(record.output);
    }
  };

  const handleDownload = () => {
    if (!record.output) return;
    const blob = new Blob([record.output], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${record.device_name}_${record.device_ip}_${new Date(record.started_at).toISOString().slice(0, 10)}.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-4xl max-h-[85vh] overflow-y-auto p-6 shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-text-primary">备份详情</h2>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-2 rounded-md"><X className="w-4 h-4 text-text-secondary" /></button>
        </div>

        {/* 元数据 */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-4">
          <div className="bg-surface-2/30 rounded-lg p-3">
            <p className="text-xs text-text-secondary mb-1">设备</p>
            <p className="text-sm font-medium text-text-primary">{record.device_name}</p>
            <p className="text-xs text-text-secondary font-mono">{record.device_ip}</p>
          </div>
          <div className="bg-surface-2/30 rounded-lg p-3">
            <p className="text-xs text-text-secondary mb-1">厂商</p>
            <p className="text-sm text-text-primary">{VENDOR_OPTIONS.find(v => v.value === record.device_vendor)?.label || record.device_vendor}</p>
          </div>
          <div className="bg-surface-2/30 rounded-lg p-3">
            <p className="text-xs text-text-secondary mb-1">执行时间</p>
            <p className="text-sm text-text-primary">{new Date(record.started_at).toLocaleString()}</p>
            {record.duration_ms && <p className="text-xs text-text-secondary">耗时 {(record.duration_ms / 1000).toFixed(1)}s</p>}
          </div>
          <div className="bg-surface-2/30 rounded-lg p-3">
            <p className="text-xs text-text-secondary mb-1">状态</p>
            <div className="flex items-center gap-1.5">
              {record.status === 'success' ? <CheckCircle2 className="w-4 h-4 text-emerald-600" /> :
               record.status === 'failed' ? <XCircle className="w-4 h-4 text-red-600" /> :
               <AlertTriangle className="w-4 h-4 text-yellow-600" />}
              <span className="text-sm text-text-primary">{record.status === 'success' ? '成功' : record.status === 'failed' ? '失败' : '部分成功'}</span>
            </div>
          </div>
        </div>

        {record.error_message && (
          <div className="mb-4 p-3 bg-red-500/10 border border-red-500/20 rounded-lg">
            <p className="text-sm text-red-600">{record.error_message}</p>
          </div>
        )}

        {/* 执行的命令 */}
        {record.commands && (
          <div className="mb-4">
            <p className="text-xs text-text-secondary mb-1">执行的命令</p>
            <div className="flex flex-wrap gap-2">
              {(JSON.parse(record.commands) as string[]).map((cmd, i) => (
                <span key={i} className="px-2 py-0.5 text-xs font-mono bg-blue-500/10 text-blue-600 rounded">{cmd}</span>
              ))}
            </div>
          </div>
        )}

        {/* 备份输出 */}
        {record.output && (
          <div>
            <div className="flex items-center justify-between mb-2">
              <p className="text-xs text-text-secondary">配置内容 ({record.output_size ? (record.output_size / 1024).toFixed(1) : 0} KB)</p>
              <div className="flex gap-2">
                <button onClick={handleCopy} className="flex items-center gap-1 px-2 py-1 text-xs text-text-secondary hover:text-text-primary hover:bg-surface-2 rounded transition-colors">
                  <Copy className="w-3.5 h-3.5" /> 复制
                </button>
                <button onClick={handleDownload} className="flex items-center gap-1 px-2 py-1 text-xs text-text-secondary hover:text-text-primary hover:bg-surface-2 rounded transition-colors">
                  <Download className="w-3.5 h-3.5" /> 下载
                </button>
              </div>
            </div>
            <pre className="max-h-96 overflow-auto bg-[#0d1117] text-[#c9d1d9] text-xs font-mono p-4 rounded-lg border border-border whitespace-pre-wrap break-all">
              {record.output}
            </pre>
          </div>
        )}
      </div>
    </div>
  );
}
