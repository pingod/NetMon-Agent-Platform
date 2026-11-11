import { useState, useEffect, useMemo } from 'react';
import {
  ShieldCheck, X, Sparkles, CheckSquare, Square, ChevronDown, ChevronUp,
  Cpu, HardDrive, Shield, Server as ServerIcon, Wrench, Layers, RefreshCw
} from 'lucide-react';
import clsx from 'clsx';
import api from '@/lib/api';
import type { Server as ServerType, ServerComplianceOptions, InspectionTemplateItem } from '../types';
import { SERVER_VENDORS } from '../types';

interface ServerComplianceOptionsModalProps {
  isOpen: boolean;
  selectedServer: ServerType | null;
  complianceOptions: ServerComplianceOptions;
  onComplianceOptionsChange: (updater: (prev: ServerComplianceOptions) => ServerComplianceOptions) => void;
  isRunningCompliance: boolean;
  onClose: () => void;
  onStartCheck: (customOpts?: ServerComplianceOptions) => void;
}

const LAYER_BADGES: Record<string, { badge: string; text: string }> = {
  硬件层: { badge: 'bg-rose-500/15 text-rose-600 border-rose-500/30', text: 'text-rose-600' },
  系统层: { badge: 'bg-sky-500/15 text-sky-600 border-sky-500/30', text: 'text-sky-600' },
  资源层: { badge: 'bg-amber-500/15 text-amber-600 border-amber-500/30', text: 'text-amber-600' },
  安全层: { badge: 'bg-violet-500/15 text-violet-600 border-violet-500/30', text: 'text-violet-600' },
  应用层: { badge: 'bg-emerald-500/15 text-emerald-600 border-emerald-500/30', text: 'text-emerald-600' },
};

export function ServerComplianceOptionsModal({
  isOpen,
  selectedServer,
  complianceOptions,
  onComplianceOptionsChange,
  isRunningCompliance,
  onClose,
  onStartCheck,
}: ServerComplianceOptionsModalProps) {
  const [mode, setMode] = useState<'standard' | 'custom'>(complianceOptions.inspectionType === 'custom' ? 'custom' : 'standard');
  const [templates, setTemplates] = useState<InspectionTemplateItem[]>([]);
  const [isLoadingTemplates, setIsLoadingTemplates] = useState(false);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('');
  const [selectedCommandNames, setSelectedCommandNames] = useState<Set<string>>(new Set());
  const [showCommandList, setShowCommandList] = useState(false);

  // 获取服务器品牌元数据
  const serverVendor = (selectedServer?.vendor || 'other').toLowerCase();
  const vendorMeta = useMemo(() => {
    return SERVER_VENDORS.find((v) => v.value === serverVendor) || {
      value: 'other',
      label: '其他/通用',
      color: 'text-text-secondary',
      bg: 'bg-slate-500/10',
      border: 'border-slate-500/30',
    };
  }, [serverVendor]);

  // 加载模版中心模版
  useEffect(() => {
    if (!isOpen) return;
    const fetchTemplates = async () => {
      setIsLoadingTemplates(true);
      try {
        const res = await api.get('/inspection-templates', {
          params: { category: 'server' },
        });
        const list: InspectionTemplateItem[] = res.data?.data || res.data || [];
        setTemplates(list);

        // 智能匹配同品牌模版
        const matched = list.find((t) => (t.vendor || '').toLowerCase() === serverVendor);
        if (matched) {
          setSelectedTemplateId(matched.id);
          setSelectedCommandNames(new Set(matched.commands?.map((c) => c.name) || []));
        } else {
          // 回退匹配通用 Linux/Windows 模版
          const osType = selectedServer?.os_type || 'linux';
          const fallback = list.find((t) => t.osType === osType) || list[0];
          if (fallback) {
            setSelectedTemplateId(fallback.id);
            setSelectedCommandNames(new Set(fallback.commands?.map((c) => c.name) || []));
          }
        }
      } catch (e) {
        console.error('加载巡检模版失败', e);
      } finally {
        setIsLoadingTemplates(false);
      }
    };
    fetchTemplates();
  }, [isOpen, selectedServer?.id, serverVendor]);

  // 当前选中的模版对象
  const activeTemplate = useMemo(() => {
    return templates.find((t) => t.id === selectedTemplateId) || null;
  }, [templates, selectedTemplateId]);

  // 当切换模版时更新默认勾选所有命令
  const handleSelectTemplate = (tplId: string) => {
    setSelectedTemplateId(tplId);
    const tpl = templates.find((t) => t.id === tplId);
    if (tpl) {
      setSelectedCommandNames(new Set(tpl.commands?.map((c) => c.name) || []));
    } else {
      setSelectedCommandNames(new Set());
    }
  };

  // 单项勾选/取消勾选命令
  const toggleCommand = (cmdName: string) => {
    const next = new Set(selectedCommandNames);
    if (next.has(cmdName)) {
      next.delete(cmdName);
    } else {
      next.add(cmdName);
    }
    setSelectedCommandNames(next);
  };

  // 全选/清空命令
  const toggleSelectAllCommands = () => {
    if (!activeTemplate) return;
    if (selectedCommandNames.size === activeTemplate.commands.length) {
      setSelectedCommandNames(new Set());
    } else {
      setSelectedCommandNames(new Set(activeTemplate.commands.map((c) => c.name)));
    }
  };

  // 开始执行巡检
  const handleStart = () => {
    const opts: ServerComplianceOptions = {
      ...complianceOptions,
      inspectionType: mode,
      templateId: mode === 'custom' ? selectedTemplateId : undefined,
      selectedCommands: mode === 'custom' ? Array.from(selectedCommandNames) : undefined,
    };
    onComplianceOptionsChange(() => opts);
    onStartCheck(opts);
  };

  if (!isOpen || !selectedServer) return null;

  const isBrandMatched = activeTemplate && (activeTemplate.vendor || '').toLowerCase() === serverVendor && serverVendor !== 'other';

  return (
    <div
      className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-[60] p-4"
      onClick={onClose}
    >
      <div
        className="bg-surface border border-border rounded-2xl p-6 w-full max-w-xl mx-auto shadow-2xl overflow-hidden flex flex-col max-h-[90vh] animate-in fade-in zoom-in-95 duration-200"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 头部标题与服务器品牌信息 */}
        <div className="flex items-center justify-between pb-4 border-b border-border">
          <div className="flex items-center gap-3">
            <div className="w-11 h-11 rounded-xl bg-gradient-to-br from-primary/20 to-purple-500/20 border border-primary/30 flex items-center justify-center text-primary shadow-lg shadow-primary/10">
              <ShieldCheck className="w-6 h-6" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="text-lg font-bold text-text-primary">服务器合规巡检</h3>
                <span className={clsx('text-[11px] px-2 py-0.5 rounded-full border font-semibold', vendorMeta.bg, vendorMeta.color, vendorMeta.border)}>
                  {vendorMeta.label.split(' ')[0]}
                </span>
                {selectedServer.model && (
                  <span className="text-[11px] px-1.5 py-0.5 rounded bg-surface border border-border text-text-tertiary font-mono">
                    {selectedServer.model}
                  </span>
                )}
              </div>
              <p className="text-xs text-text-secondary mt-0.5">
                目标服务器: <span className="font-semibold text-text-primary">{selectedServer.name}</span> ({selectedServer.hostname}:{selectedServer.port})
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-text-secondary hover:text-text-primary hover:bg-background rounded-lg transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 内容区域（可滚动） */}
        <div className="flex-1 overflow-y-auto py-4 space-y-4 pr-1">
          {/* 巡检模式选择卡片 */}
          <div>
            <label className="block text-xs font-semibold text-text-tertiary uppercase tracking-wider mb-2">
              巡检模式选择
            </label>
            <div className="grid grid-cols-2 gap-3">
              {/* 模式 1：标准合规巡检 */}
              <button
                type="button"
                onClick={() => setMode('standard')}
                className={clsx(
                  'p-3.5 rounded-xl border text-left transition-all relative overflow-hidden flex flex-col justify-between',
                  mode === 'standard'
                    ? 'border-primary bg-primary/10 text-primary shadow-md shadow-primary/10 ring-1 ring-primary/40'
                    : 'border-border bg-background hover:border-border/80 text-text-secondary hover:text-text-primary'
                )}
              >
                <div className="flex items-center gap-2 mb-1.5">
                  <ShieldCheck className="w-4 h-4 text-primary" />
                  <span className="text-sm font-bold text-text-primary">标准合规巡检</span>
                </div>
                <p className="text-xs text-text-tertiary leading-relaxed">
                  系统健康基线、CPU/内存/磁盘利用率、SSH安全配置与故障服务审计
                </p>
              </button>

              {/* 模式 2：自定义模版巡检 */}
              <button
                type="button"
                onClick={() => setMode('custom')}
                className={clsx(
                  'p-3.5 rounded-xl border text-left transition-all relative overflow-hidden flex flex-col justify-between',
                  mode === 'custom'
                    ? 'border-purple-500 bg-purple-500/10 text-purple-600 shadow-md shadow-purple-500/10 ring-1 ring-purple-500/40'
                    : 'border-border bg-background hover:border-border/80 text-text-secondary hover:text-text-primary'
                )}
              >
                <div className="flex items-center justify-between mb-1.5">
                  <div className="flex items-center gap-2">
                    <Sparkles className="w-4 h-4 text-purple-600" />
                    <span className="text-sm font-bold text-text-primary">自定义模版巡检</span>
                  </div>
                  <span className="text-[10px] px-1.5 py-0.2 rounded-full bg-purple-500/20 text-purple-600 font-medium border border-purple-500/30">
                    模版中心
                  </span>
                </div>
                <p className="text-xs text-text-tertiary leading-relaxed">
                  选择模版中心专属模版，按对应品牌硬件层至应用层进行五层深度检查
                </p>
              </button>
            </div>
          </div>

          {/* 当选择自定义模版巡检时的模版配置 */}
          {mode === 'custom' && (
            <div className="p-4 bg-background rounded-xl border border-border space-y-3 animate-in fade-in slide-in-from-top-2 duration-150">
              <div className="flex items-center justify-between">
                <label className="text-sm font-medium text-text-primary flex items-center gap-2">
                  <Layers className="w-4 h-4 text-purple-600" />
                  选择巡检模版
                </label>
                {isLoadingTemplates && (
                  <span className="text-xs text-text-tertiary flex items-center gap-1">
                    <RefreshCw className="w-3 h-3 animate-spin" /> 加载中...
                  </span>
                )}
              </div>

              {/* 品牌自动匹配提示 */}
              {isBrandMatched && (
                <div className="p-2.5 rounded-lg bg-gradient-to-r from-purple-500/15 to-blue-500/15 border border-purple-500/30 flex items-center gap-2 text-xs text-purple-600">
                  <Sparkles className="w-4 h-4 text-purple-600 flex-shrink-0" />
                  <span>
                    已根据该服务器品牌【<strong>{vendorMeta.label.split(' ')[0]}</strong>】自动匹配专属深度巡检模版
                  </span>
                </div>
              )}

              {/* 模版下拉选择 */}
              <div className="relative">
                <select
                  value={selectedTemplateId}
                  onChange={(e) => handleSelectTemplate(e.target.value)}
                  className="w-full bg-surface border border-border rounded-lg px-3.5 py-2.5 text-sm text-text-primary focus:outline-none focus:border-primary font-medium"
                >
                  {templates.map((tpl) => (
                    <option key={tpl.id} value={tpl.id}>
                      {tpl.name} ({tpl.commands?.length || 0}条指令) {tpl.isPreset ? ' [官方预置]' : ' [自定义]'}
                    </option>
                  ))}
                </select>
              </div>

              {/* 当前模版信息摘要 */}
              {activeTemplate && (
                <div className="space-y-2 pt-1">
                  <p className="text-xs text-text-secondary leading-relaxed bg-surface/60 p-2.5 rounded-lg border border-border/60">
                    {activeTemplate.description || '暂无说明'}
                  </p>

                  <div className="flex items-center justify-between text-xs pt-1">
                    <div className="flex items-center gap-2 text-text-tertiary">
                      <span>包含指令: <strong className="text-emerald-600">{activeTemplate.commands?.length || 0}</strong> 项</span>
                      <span>已勾选: <strong className="text-primary">{selectedCommandNames.size}</strong> 项</span>
                    </div>

                    <button
                      type="button"
                      onClick={() => setShowCommandList(!showCommandList)}
                      className="flex items-center gap-1 text-xs text-primary hover:underline font-medium"
                    >
                      {showCommandList ? (
                        <>收起指令明细 <ChevronUp className="w-3.5 h-3.5" /></>
                      ) : (
                        <>展开指令勾选与预览 <ChevronDown className="w-3.5 h-3.5" /></>
                      )}
                    </button>
                  </div>

                  {/* 展开的指令列表与勾选面板 */}
                  {showCommandList && (
                    <div className="mt-2 border border-border rounded-lg p-3 bg-surface max-h-56 overflow-y-auto space-y-2 text-xs">
                      <div className="flex items-center justify-between pb-2 border-b border-border/80">
                        <span className="text-text-secondary font-medium">巡检指令清单</span>
                        <button
                          type="button"
                          onClick={toggleSelectAllCommands}
                          className="text-primary hover:underline flex items-center gap-1 text-xs"
                        >
                          {selectedCommandNames.size === activeTemplate.commands?.length ? '取消全选' : '全部勾选'}
                        </button>
                      </div>

                      <div className="space-y-1.5 pt-1">
                        {activeTemplate.commands?.map((cmd) => {
                          const isSelected = selectedCommandNames.has(cmd.name);
                          const layerInfo = LAYER_BADGES[cmd.group || '系统层'] || { badge: 'bg-surface-2 text-text-secondary', text: '' };
                          return (
                            <div
                              key={cmd.name}
                              onClick={() => toggleCommand(cmd.name)}
                              className={clsx(
                                'flex items-start gap-2.5 p-2 rounded-md cursor-pointer transition-colors border',
                                isSelected
                                  ? 'bg-background/80 border-border'
                                  : 'bg-background/30 border-transparent opacity-60 hover:opacity-90'
                              )}
                            >
                              <div className="mt-0.5 text-primary flex-shrink-0">
                                {isSelected ? <CheckSquare className="w-3.5 h-3.5" /> : <Square className="w-3.5 h-3.5 text-text-tertiary" />}
                              </div>
                              <div className="flex-1 min-w-0">
                                <div className="flex items-center gap-1.5 flex-wrap">
                                  <span className="font-semibold text-text-primary truncate">{cmd.name}</span>
                                  <span className={clsx('px-1.5 py-0.2 rounded text-[10px] border', layerInfo.badge)}>
                                    {cmd.group || '系统层'}
                                  </span>
                                </div>
                                <code className="block font-mono text-[11px] text-emerald-600/90 truncate mt-0.5">
                                  {cmd.command}
                                </code>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>
              )}
            </div>
          )}

          {/* AI 智能分析开关 */}
          <div className="p-3.5 bg-background rounded-xl border border-border">
            <label className="flex items-center justify-between cursor-pointer">
              <div className="flex flex-col pr-4">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold text-text-primary">AI 智能深度分析</span>
                  {complianceOptions.useAI && (
                    <span className="text-[10px] px-1.5 py-0.5 bg-gradient-to-r from-primary/20 to-purple-500/20 text-primary border border-primary/30 rounded-full font-medium">
                      推荐开启
                    </span>
                  )}
                </div>
                <span className="text-xs text-text-tertiary mt-1">
                  {complianceOptions.useAI
                    ? '🤖 自动结合命令执行结果提供专业诊断建议与风险隐患排查'
                    : '⚡ 仅执行巡检命令，无需 AI 大模型介入，执行速度更快'}
                </span>
              </div>
              <div className="relative flex-shrink-0">
                <input
                  type="checkbox"
                  checked={complianceOptions.useAI}
                  onChange={(e) =>
                    onComplianceOptionsChange((prev) => ({ ...prev, useAI: e.target.checked }))
                  }
                  className="sr-only peer"
                />
                <div className="w-11 h-6 bg-surface border-2 border-border rounded-full peer peer-checked:bg-primary peer-checked:border-primary transition-all">
                  <div className="w-4 h-4 bg-white rounded-full shadow-md absolute top-0.5 left-0.5 peer-checked:translate-x-5 transition-transform"></div>
                </div>
              </div>
            </label>
          </div>

          {/* 并发执行数 */}
          <div className="p-3.5 bg-background rounded-xl border border-border">
            <div className="flex items-center justify-between mb-2">
              <div>
                <span className="text-sm font-medium text-text-primary">并发执行线程数</span>
                <p className="text-xs text-text-tertiary mt-0.5">同时发起的 SSH 检查指令进程数量</p>
              </div>
              <span className="text-base font-bold text-primary font-mono px-2 py-0.5 rounded bg-primary/10 border border-primary/20">
                {complianceOptions.concurrency} 并发
              </span>
            </div>
            <div className="flex gap-2">
              {[3, 5, 8, 10].map((num) => (
                <button
                  key={num}
                  type="button"
                  onClick={() =>
                    onComplianceOptionsChange((prev) => ({ ...prev, concurrency: num }))
                  }
                  className={clsx(
                    'flex-1 py-1.5 rounded-lg text-xs font-medium transition-all border',
                    complianceOptions.concurrency === num
                      ? 'bg-primary text-white border-primary shadow-sm'
                      : 'bg-surface text-text-secondary hover:text-text-primary border-border hover:bg-background'
                  )}
                >
                  {num} {num === 5 ? '(推荐)' : ''}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* 底部操作按钮 */}
        <div className="flex gap-3 pt-4 border-t border-border bg-surface">
          <button
            type="button"
            onClick={onClose}
            className="flex-1 px-4 py-2.5 bg-background border border-border text-text-secondary hover:text-text-primary rounded-xl text-sm font-medium transition-colors"
          >
            取消
          </button>
          <button
            type="button"
            onClick={handleStart}
            disabled={isRunningCompliance || (mode === 'custom' && selectedCommandNames.size === 0)}
            className="flex-1 px-4 py-2.5 bg-gradient-to-r from-primary to-purple-600 hover:from-primary/90 hover:to-purple-500 text-white rounded-xl text-sm font-medium transition-all shadow-lg shadow-primary/20 disabled:opacity-50 flex items-center justify-center gap-2"
          >
            {isRunningCompliance ? (
              <>
                <RefreshCw className="w-4 h-4 animate-spin" />
                正在执行巡检...
              </>
            ) : (
              <>
                <ShieldCheck className="w-4 h-4" />
                开始巡检 {mode === 'custom' ? `(${selectedCommandNames.size}项)` : ''}
              </>
            )}
          </button>
        </div>
      </div>
    </div>
  );
}