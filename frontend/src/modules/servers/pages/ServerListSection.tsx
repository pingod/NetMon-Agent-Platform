import { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import {
  Server, Terminal, CheckCircle2, AlertCircle, ShieldCheck, Wifi, History, Clock, FolderTree,
  Upload, RefreshCw, Plus, Edit, Trash2, Cpu, HardDrive, MemoryStick, Monitor,
  MonitorPlay, Sparkles, FolderPlus, CheckSquare, Square, FileText, X, AlertTriangle, Loader2,
  Building2, Layers, CheckCheck
} from 'lucide-react';
import clsx from 'clsx';
import { useToast } from '@/contexts/ToastContext';
import api from '@/lib/api';
import type { Server as ServerType, ServerGroup, InspectionTemplateItem } from './types';
import { SERVER_VENDORS } from './types';
import { GroupTree } from './ServerGroupSection';

interface ServerListSectionProps {
  // Data
  servers: ServerType[];
  isLoading: boolean;
  groupsData: ServerGroup[] | undefined;
  allTags: string[];
  filteredServers: ServerType[];
  // Selection
  selectedTag: string | null;
  onSelectTag: (tag: string | null) => void;
  selectedGroupId: string | null;
  onSelectGroupId: (id: string | null) => void;
  selectedVendor?: string | null;
  onSelectVendor?: (vendor: string | null) => void;
  // Toolbar state
  showGroups: boolean;
  onToggleGroups: () => void;
  isCollecting: boolean;
  isCollectingMetrics: boolean;
  // Handlers
  onCollectAll: () => void;
  onCollectAllMetrics: () => void;
  onOpenImport: () => void;
  onOpenGroupModal: () => void;
  onTestConnection: (server: ServerType) => void;
  onCollectInfo: (server: ServerType) => void;
  onCollectMetrics: (server: ServerType) => void;
  onEdit: (server: ServerType) => void;
  onDelete: (id: string, name: string) => void;
  onOpenAiCommand: (server: ServerType) => void;
  onSelectForCommand: (server: ServerType) => void;
  onRunCompliance: (server: ServerType) => void;
  onViewCommandHistory: (server: ServerType) => void;
  onViewComplianceHistory: (server: ServerType) => void;
  testingServerId?: string | null;
}

export function ServerListSection({
  servers,
  isLoading,
  groupsData,
  allTags,
  filteredServers,
  selectedTag,
  onSelectTag,
  selectedGroupId,
  onSelectGroupId,
  selectedVendor,
  onSelectVendor,
  showGroups,
  onToggleGroups,
  isCollecting,
  isCollectingMetrics,
  onCollectAll,
  onCollectAllMetrics,
  onOpenImport,
  onOpenGroupModal,
  onTestConnection,
  onCollectInfo,
  onCollectMetrics,
  onEdit,
  onDelete,
  onOpenAiCommand,
  onSelectForCommand,
  onRunCompliance,
  onViewCommandHistory,
  onViewComplianceHistory,
  testingServerId,
}: ServerListSectionProps) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const toast = useToast();
  const [selectedServerIds, setSelectedServerIds] = useState<Set<string>>(new Set());
  const [showBatchModal, setShowBatchModal] = useState(false);
  const [showBatchDeleteModal, setShowBatchDeleteModal] = useState(false);
  const [isBatchDeleting, setIsBatchDeleting] = useState(false);
  const [isBatchInspecting, setIsBatchInspecting] = useState(false);
  const [inspectionType, setInspectionType] = useState('standard');
  const [batchMode, setBatchMode] = useState<'brand_auto' | 'custom_template' | 'standard'>('brand_auto');
  const [serverTemplates, setServerTemplates] = useState<InspectionTemplateItem[]>([]);
  const [isLoadingTemplates, setIsLoadingTemplates] = useState(false);
  const [selectedBatchTemplateId, setSelectedBatchTemplateId] = useState('');
  const [batchUseAI, setBatchUseAI] = useState(true);
  const [concurrency, setConcurrency] = useState(5);
  const [batchResultReportId, setBatchResultReportId] = useState<string | null>(null);

  const getVendorMeta = (vendorKey?: string | null) => {
    const v = (vendorKey || 'other').toLowerCase();
    return (
      SERVER_VENDORS.find((item) => item.value === v) || {
        value: 'other',
        label: '其他 (Other)',
        color: 'text-slate-400',
        bg: 'bg-slate-500/10',
        border: 'border-slate-500/30',
      }
    );
  };

  // 选中的服务器实体列表
  const selectedServersList = useMemo(() => {
    return filteredServers.filter((s) => selectedServerIds.has(s.id));
  }, [filteredServers, selectedServerIds]);

  // 选中的服务器品牌分布统计
  const selectedVendorsDistribution = useMemo(() => {
    const counts: Record<string, number> = {};
    selectedServersList.forEach((s) => {
      const v = (s.vendor || 'other').toLowerCase();
      counts[v] = (counts[v] || 0) + 1;
    });
    return counts;
  }, [selectedServersList]);

  // 加载模版中心服务器模版
  useEffect(() => {
    if (!showBatchModal) return;
    const fetchTemplates = async () => {
      setIsLoadingTemplates(true);
      try {
        const res = await api.get('/inspection-templates', {
          params: { category: 'server' },
        });
        const list: InspectionTemplateItem[] = res.data?.data || res.data || [];
        setServerTemplates(list);

        // 如果全部属于同一个品牌，默认选中该品牌模版
        const vendors = Object.keys(selectedVendorsDistribution);
        if (vendors.length === 1 && vendors[0] !== 'other') {
          const matched = list.find((t) => (t.vendor || '').toLowerCase() === vendors[0]);
          if (matched) {
            setSelectedBatchTemplateId(matched.id);
          } else if (list.length > 0) {
            setSelectedBatchTemplateId(list[0].id);
          }
        } else if (list.length > 0) {
          const fallback = list.find((t) => t.osType === 'linux' && (t.name.includes('SmartKit') || t.name.includes('标准'))) || list[0];
          setSelectedBatchTemplateId(fallback.id);
        }
      } catch (e) {
        console.error('加载巡检模版失败', e);
      } finally {
        setIsLoadingTemplates(false);
      }
    };
    fetchTemplates();
  }, [showBatchModal, selectedVendorsDistribution]);

  const toggleSelectAll = () => {
    if (selectedServerIds.size === filteredServers.length && filteredServers.length > 0) {
      setSelectedServerIds(new Set());
    } else {
      setSelectedServerIds(new Set(filteredServers.map((s) => s.id)));
    }
  };

  const toggleSelectServer = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const next = new Set(selectedServerIds);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    setSelectedServerIds(next);
  };

  const handleExecuteBatchInspect = async () => {
    if (selectedServerIds.size === 0) return;
    setIsBatchInspecting(true);
    try {
      let finalInspectionType = 'standard';
      let finalTemplateId: string | undefined = undefined;

      if (batchMode === 'brand_auto') {
        finalInspectionType = 'brand_auto';
      } else if (batchMode === 'custom_template') {
        finalInspectionType = 'custom';
        finalTemplateId = selectedBatchTemplateId;
      } else {
        finalInspectionType = inspectionType;
      }

      const res = await api.post('/servers/batch-inspect', {
        server_ids: Array.from(selectedServerIds),
        inspectionType: finalInspectionType,
        templateId: finalTemplateId,
        concurrency,
        useAI: batchUseAI,
      });
      const data = res.data.data;
      setBatchResultReportId(data.report_id);
      toast.success(`批量巡检完成！成功 ${data.success_count} 台，失败 ${data.failed_count} 台。已生成巡检报告！`);
      setSelectedServerIds(new Set());
      setShowBatchModal(false);
    } catch (err: any) {
      toast.error('批量巡检失败: ' + (err.response?.data?.error || err.message || '网络错误'));
    } finally {
      setIsBatchInspecting(false);
    }
  };

  const confirmBatchDeleteServers = async () => {
    if (selectedServerIds.size === 0) return;
    setIsBatchDeleting(true);
    try {
      const res = await api.post('/servers/batch-delete', {
        server_ids: Array.from(selectedServerIds),
      });
      const data = res.data?.data || res.data;
      toast.success(data?.message || `已成功批量删除 ${selectedServerIds.size} 台服务器`);
      setSelectedServerIds(new Set());
      setShowBatchDeleteModal(false);
      await queryClient.invalidateQueries({ queryKey: ['servers'] });
    } catch (err: any) {
      toast.error('批量删除失败: ' + (err.response?.data?.error || err.message || '网络错误'));
    } finally {
      setIsBatchDeleting(false);
    }
  };

  return (
    <>
      {/* 工具栏 */}
      <div className="flex flex-wrap items-center gap-2 mb-4">
        <button
          onClick={onToggleGroups}
          className={clsx(
            'flex items-center gap-2 px-3 py-2 rounded-lg text-sm transition-colors border',
            showGroups
              ? 'bg-primary/10 border-primary/30 text-primary'
              : 'border-border bg-surface text-text-secondary hover:text-text-primary',
          )}
        >
          <FolderTree className="w-4 h-4" />
          分组
        </button>

        {/* 批量全选 */}
        {filteredServers.length > 0 && (
          <button
            onClick={toggleSelectAll}
            className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm bg-surface border border-border text-text-secondary hover:text-text-primary transition-colors"
          >
            {selectedServerIds.size === filteredServers.length ? (
              <CheckSquare className="w-4 h-4 text-primary" />
            ) : (
              <Square className="w-4 h-4" />
            )}
            {selectedServerIds.size > 0 ? `已选 ${selectedServerIds.size}/${filteredServers.length} 台` : '全选'}
          </button>
        )}

        {/* 批量巡检按钮 */}
        <button
          onClick={() => {
            if (selectedServerIds.size === 0) {
              toast.error('请先勾选需要批量巡检的服务器');
              return;
            }
            setShowBatchModal(true);
          }}
          disabled={selectedServerIds.size === 0 || isBatchInspecting}
          className={clsx(
            'flex items-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-colors border',
            selectedServerIds.size > 0
              ? 'bg-gradient-to-r from-blue-600 to-purple-600 text-white border-blue-500 shadow-md shadow-blue-500/20 hover:from-blue-500 hover:to-purple-500'
              : 'bg-surface border-border text-text-secondary opacity-60 cursor-not-allowed',
          )}
        >
          <ShieldCheck className={clsx('w-4 h-4', isBatchInspecting && 'animate-spin')} />
          {isBatchInspecting ? '正在批量巡检...' : `批量巡检 (${selectedServerIds.size})`}
        </button>

        {/* 批量删除按钮 */}
        {selectedServerIds.size > 0 && (
          <button
            onClick={() => setShowBatchDeleteModal(true)}
            disabled={isBatchDeleting}
            className="flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors border bg-rose-600 hover:bg-rose-500 text-white border-rose-500 shadow-md shadow-rose-500/20 disabled:opacity-50"
          >
            <Trash2 className="w-4 h-4" />
            批量删除 ({selectedServerIds.size})
          </button>
        )}

        <button
          onClick={onCollectAll}
          disabled={isCollecting}
          className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm bg-surface border border-border text-text-secondary hover:text-text-primary transition-colors disabled:opacity-50"
        >
          <RefreshCw className={clsx('w-4 h-4', isCollecting && 'animate-spin')} />
          采集所有主机信息
        </button>
        <button
          onClick={onCollectAllMetrics}
          disabled={isCollectingMetrics}
          className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm bg-surface border border-border text-text-secondary hover:text-text-primary transition-colors disabled:opacity-50"
        >
          <RefreshCw className={clsx('w-4 h-4', isCollectingMetrics && 'animate-spin')} />
          采集所有性能指标
        </button>
        <button
          onClick={onOpenImport}
          className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm bg-surface border border-border text-text-secondary hover:text-text-primary transition-colors"
        >
          <Upload className="w-4 h-4" />
          批量导入
        </button>
        <button
          onClick={onOpenGroupModal}
          className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm bg-surface border border-border text-text-secondary hover:text-text-primary transition-colors"
        >
          <FolderPlus className="w-4 h-4" />
          新建分组
        </button>

        <button
          onClick={() => navigate('/reports')}
          className="flex items-center gap-2 px-3 py-2 rounded-lg text-sm bg-surface border border-primary/40 text-primary hover:bg-primary/10 transition-colors ml-auto"
        >
          <FileText className="w-4 h-4" />
          报告中心 (ZIP 下载)
        </button>
      </div>

      <div className="flex gap-4">
        {/* 分组侧边栏 */}
        {showGroups && (
          <div className="w-56 flex-shrink-0 bg-surface border border-border rounded-lg p-3">
            <div className="flex items-center justify-between mb-2">
              <h3 className="text-sm font-medium text-text-primary">服务器分组</h3>
              <button
                onClick={() => onSelectGroupId(null)}
                className="text-xs text-text-secondary hover:text-text-primary"
              >
                清除筛选
              </button>
            </div>
            {groupsData && groupsData.length > 0 ? (
              <GroupTree groups={groupsData} selectedGroupId={selectedGroupId} onSelectGroup={onSelectGroupId} />
            ) : (
              <p className="text-xs text-text-secondary py-4 text-center">暂无分组</p>
            )}
          </div>
        )}

        {/* 服务器列表 */}
        <div className="flex-1">
          {/* 品牌厂商快捷筛选 */}
          <div className="mb-3 flex flex-wrap items-center gap-1.5 p-2 bg-surface border border-border rounded-lg">
            <span className="text-xs text-text-secondary font-medium mr-1 flex items-center gap-1">
              <Building2 className="w-3.5 h-3.5 text-primary" />
              品牌厂商:
            </span>
            <button
              onClick={() => onSelectVendor?.(null)}
              className={clsx(
                'px-2.5 py-1 rounded-md text-xs font-medium transition-all',
                !selectedVendor
                  ? 'bg-primary text-white shadow-sm'
                  : 'bg-background border border-border text-text-secondary hover:text-text-primary hover:bg-surface',
              )}
            >
              全部 ({servers.length})
            </button>
            {SERVER_VENDORS.map((v) => {
              const count = servers.filter((s) => (s.vendor || 'other').toLowerCase() === v.value).length;
              return (
                <button
                  key={v.value}
                  onClick={() => onSelectVendor?.(selectedVendor === v.value ? null : v.value)}
                  className={clsx(
                    'px-2.5 py-1 rounded-md text-xs font-medium transition-all border flex items-center gap-1.5',
                    selectedVendor === v.value
                      ? `${v.bg} ${v.color} ${v.border} ring-1 ring-primary/40 font-semibold`
                      : 'bg-background border-border text-text-secondary hover:text-text-primary hover:bg-surface',
                  )}
                >
                  <span>{v.label.split(' ')[0]}</span>
                  {count > 0 && (
                    <span
                      className={clsx(
                        'text-[10px] px-1 py-0.2 rounded-full',
                        selectedVendor === v.value ? 'bg-primary/20 text-white' : 'bg-surface text-text-tertiary',
                      )}
                    >
                      {count}
                    </span>
                  )}
                </button>
              );
            })}
          </div>

          {/* 标签筛选器 */}
          {allTags.length > 0 && (
            <div className="mb-4 flex flex-wrap gap-2">
              <button
                onClick={() => {
                  onSelectTag(null);
                  onSelectGroupId(null);
                }}
                className={clsx(
                  'px-3 py-1 rounded-full text-sm transition-colors',
                  !selectedTag && !selectedGroupId
                    ? 'bg-primary text-white'
                    : 'bg-background border border-border text-text-secondary hover:bg-surface',
                )}
              >
                全部
              </button>
              {allTags.map((tag) => (
                <button
                  key={tag}
                  onClick={() => {
                    onSelectTag(selectedTag === tag ? null : tag);
                    onSelectGroupId(null);
                  }}
                  className={clsx(
                    'px-3 py-1 rounded-full text-sm transition-colors',
                    selectedTag === tag
                      ? 'bg-primary text-white'
                      : 'bg-background border border-border text-text-secondary hover:bg-surface',
                  )}
                >
                  {tag}
                </button>
              ))}
            </div>
          )}

          <div className="grid gap-4 grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {isLoading ? (
              Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="bg-surface border border-border rounded-lg p-4 animate-pulse">
                  <div className="h-4 bg-border rounded w-1/2 mb-2" />
                  <div className="h-3 bg-border rounded w-3/4" />
                </div>
              ))
            ) : filteredServers.length === 0 ? (
              <div className="col-span-full flex flex-col items-center justify-center py-12 text-text-secondary">
                <Server className="w-12 h-12 mb-4 opacity-50" />
                <p>
                  {selectedVendor
                    ? `没有品牌为 "${getVendorMeta(selectedVendor).label}" 的服务器`
                    : selectedTag
                      ? `没有带标签 "${selectedTag}" 的服务器`
                      : selectedGroupId
                        ? '该分组下暂无服务器'
                        : '暂无服务器，请添加第一个服务器'}
                </p>
              </div>
            ) : (
              filteredServers.map((server) => (
                <div
                  key={server.id}
                  className={clsx(
                    'relative bg-surface border rounded-lg p-4 min-w-0 overflow-hidden',
                    server.os_type === 'linux'
                      ? 'border-yellow-500/30'
                      : server.os_type === 'windows'
                        ? 'border-blue-500/30'
                        : 'border-border',
                  )}
                >
                  {/* 操作系统左侧标识条 */}
                  <div
                    className={clsx(
                      'absolute left-0 top-0 bottom-0 w-1',
                      server.os_type === 'linux'
                        ? 'bg-gradient-to-b from-yellow-500 to-orange-500'
                        : server.os_type === 'windows'
                          ? 'bg-gradient-to-b from-blue-500 to-cyan-500'
                          : 'bg-gradient-to-b from-text-tertiary/50 to-text-tertiary/30',
                    )}
                  />

                  <div className="flex items-start justify-between mb-3 min-w-0 pl-2">
                    <div className="flex items-center gap-2 min-w-0 flex-1">
                      {/* 勾选框 */}
                      <button
                        onClick={(e) => toggleSelectServer(server.id, e)}
                        className="p-0.5 text-text-secondary hover:text-primary transition-colors flex-shrink-0"
                        title={selectedServerIds.has(server.id) ? '取消勾选' : '勾选此服务器'}
                      >
                        {selectedServerIds.has(server.id) ? (
                          <CheckSquare className="w-4 h-4 text-primary" />
                        ) : (
                          <Square className="w-4 h-4 opacity-60 hover:opacity-100" />
                        )}
                      </button>

                      <div
                        className={clsx(
                          'w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0',
                          server.os_type === 'linux'
                            ? 'bg-yellow-500/10'
                            : server.os_type === 'windows'
                              ? 'bg-blue-500/10'
                              : 'bg-primary/10',
                        )}
                      >
                        <Server
                          className={clsx(
                            'w-4 h-4',
                            server.os_type === 'linux'
                              ? 'text-yellow-500'
                              : server.os_type === 'windows'
                                ? 'text-blue-500'
                                : 'text-primary',
                          )}
                        />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex items-center gap-1.5 flex-wrap">
                          <h3 className="font-medium text-text-primary truncate">{server.name}</h3>
                          {(() => {
                            const vm = getVendorMeta(server.vendor);
                            return (
                              <span
                                onClick={(e) => {
                                  e.stopPropagation();
                                  onSelectVendor?.(server.vendor || 'other');
                                }}
                                title={`品牌: ${vm.label} (点击筛选)`}
                                className={clsx(
                                  'text-[10px] px-1.5 py-0.5 rounded font-medium border cursor-pointer hover:opacity-80 transition-opacity',
                                  vm.bg,
                                  vm.color,
                                  vm.border,
                                )}
                              >
                                {vm.label.split(' ')[0]}
                              </span>
                            );
                          })()}
                        </div>
                        <p className="text-xs text-text-secondary truncate">
                          {server.hostname}:{server.port}
                          {server.model && (
                            <span className="ml-1.5 text-[11px] text-text-tertiary font-mono">
                              [{server.model}]
                            </span>
                          )}
                        </p>
                      </div>
                    </div>
                    <div className="flex gap-1 flex-shrink-0">
                      {server.os_type === 'windows' && (
                        <button
                          onClick={() => navigate(`/remote-desktop/${server.id}`)}
                          className="p-1 hover:bg-background rounded transition-colors"
                          title="远程桌面"
                        >
                          <MonitorPlay className="w-4 h-4 text-text-secondary" />
                        </button>
                      )}
                      <button
                        onClick={() => onTestConnection(server)}
                        disabled={testingServerId === server.id}
                        className="p-1 hover:bg-background rounded transition-colors disabled:opacity-50"
                        title="测试连接"
                      >
                        {testingServerId === server.id ? (
                          <Loader2 className="w-4 h-4 text-primary animate-spin" />
                        ) : (
                          <Wifi className="w-4 h-4 text-text-secondary" />
                        )}
                      </button>
                      <button
                        onClick={() => onCollectInfo(server)}
                        disabled={isCollecting}
                        className="p-1 hover:bg-background rounded transition-colors disabled:opacity-50"
                        title="采集主机信息"
                      >
                        <RefreshCw className={clsx('w-4 h-4 text-text-secondary', isCollecting && 'animate-spin')} />
                      </button>
                      <button
                        onClick={() => onCollectMetrics(server)}
                        disabled={isCollectingMetrics}
                        className="p-1 hover:bg-background rounded transition-colors disabled:opacity-50"
                        title="采集性能指标"
                      >
                        <Monitor className={clsx('w-4 h-4 text-text-secondary', isCollectingMetrics && 'animate-spin')} />
                      </button>
                      <button
                        onClick={() => onDelete(server.id, server.name)}
                        className="p-1 hover:bg-background rounded transition-colors"
                        title="删除"
                      >
                        <Trash2 className="w-4 h-4 text-status-failed" />
                      </button>
                      <button
                        onClick={() => onEdit(server)}
                        className="p-1 hover:bg-background rounded transition-colors"
                        title="编辑"
                      >
                        <Edit className="w-4 h-4 text-text-secondary" />
                      </button>
                    </div>
                  </div>
                  {server.description && (
                    <p className="text-xs text-text-secondary mb-3">{server.description}</p>
                  )}

                  {/* 主机扩展信息 */}
                  {(server.os || server.cpu_cores || server.memory_gb || server.disk_gb) && (
                    <div className="mb-3 p-2 bg-background rounded-lg">
                      {server.os && (
                        <div className="flex items-center gap-1.5 text-xs text-text-secondary mb-2">
                          <Monitor className="w-3 h-3 flex-shrink-0" />
                          <span className="truncate">{server.os}</span>
                        </div>
                      )}
                      {(server.cpu_cores !== undefined ||
                        server.memory_gb !== undefined ||
                        server.disk_gb !== undefined) && (
                        <div className="grid grid-cols-3 gap-2 text-xs">
                          {server.cpu_cores !== undefined && (
                            <div className="flex items-center gap-1.5 text-text-secondary">
                              <Cpu className="w-3 h-3 flex-shrink-0" />
                              <span>{server.cpu_cores} 核</span>
                            </div>
                          )}
                          {server.memory_gb !== undefined && (
                            <div className="flex items-center gap-1.5 text-text-secondary">
                              <MemoryStick className="w-3 h-3 flex-shrink-0" />
                              <span>{server.memory_gb} GB</span>
                            </div>
                          )}
                          {server.disk_gb !== undefined && (
                            <div className="flex items-center gap-1.5 text-text-secondary">
                              <HardDrive className="w-3 h-3 flex-shrink-0" />
                              <span>{server.disk_gb} GB</span>
                            </div>
                          )}
                        </div>
                      )}
                    </div>
                  )}

                  {/* 分组展示 */}
                  {server.groups && server.groups.length > 0 && (
                    <div className="flex flex-wrap gap-1 mb-2">
                      {server.groups.map((g) => (
                        <span
                          key={g.id}
                          className="px-2 py-0.5 bg-purple-500/10 text-purple-500 text-xs rounded-full flex items-center gap-1"
                        >
                          <FolderTree className="w-2.5 h-2.5" />
                          {g.name}
                        </span>
                      ))}
                    </div>
                  )}

                  {/* 标签展示 */}
                  {server.tags && server.tags.length > 0 && (
                    <div className="flex flex-wrap gap-1 mb-3">
                      {server.tags.map((tag: string) => (
                        <span key={tag} className="px-2 py-0.5 bg-primary/10 text-primary text-xs rounded-full">
                          {tag}
                        </span>
                      ))}
                    </div>
                  )}

                  <div className="flex items-center gap-2 mb-3">
                    {server.last_connected ? (
                      <span className="flex items-center gap-1 text-xs text-text-secondary">
                        <CheckCircle2 className="w-3 h-3 text-status-success" />
                        最后连接: {new Date(server.last_connected).toLocaleDateString()}
                      </span>
                    ) : (
                      <span className="flex items-center gap-1 text-xs text-text-secondary">
                        <AlertCircle className="w-3 h-3 text-status-warning" />
                        未连接过
                      </span>
                    )}
                  </div>
                  <div className="grid grid-cols-3 gap-2 mt-3">
                    <button
                      onClick={() => onOpenAiCommand(server)}
                      className="flex flex-col items-center justify-center gap-1 px-2 py-2.5 bg-gradient-to-r from-purple-600/20 to-blue-600/20 border border-purple-500/30 rounded-lg text-xs font-medium text-purple-300 whitespace-nowrap hover:from-purple-600/30 hover:to-blue-600/30 transition-colors"
                    >
                      <Sparkles className="w-4 h-4" />
                      <span>AI 执行</span>
                    </button>
                    <button
                      onClick={() => onSelectForCommand(server)}
                      className="flex flex-col items-center justify-center gap-1 px-2 py-2.5 bg-surface border border-border rounded-lg text-xs text-text-primary whitespace-nowrap hover:bg-background transition-colors"
                    >
                      <Terminal className="w-4 h-4" />
                      <span>执行命令</span>
                    </button>
                    <button
                      onClick={() => onRunCompliance(server)}
                      className="flex flex-col items-center justify-center gap-1 px-2 py-2.5 bg-surface border border-border rounded-lg text-xs text-text-primary whitespace-nowrap hover:bg-background transition-colors"
                    >
                      <ShieldCheck className="w-4 h-4" />
                      <span>合规检查</span>
                    </button>
                  </div>
                  <div className="grid grid-cols-2 gap-2 mt-2">
                    <button
                      onClick={() => onViewCommandHistory(server)}
                      className="flex items-center justify-center gap-1.5 px-3 py-2 bg-surface border border-border rounded-lg text-xs text-text-secondary hover:text-text-primary transition-colors whitespace-nowrap"
                    >
                      <History className="w-3.5 h-3.5" />
                      <span>命令历史</span>
                    </button>
                    <button
                      onClick={() => onViewComplianceHistory(server)}
                      className="flex items-center justify-center gap-1.5 px-3 py-2 bg-surface border border-border rounded-lg text-xs text-text-secondary hover:text-text-primary transition-colors whitespace-nowrap"
                    >
                      <Clock className="w-3.5 h-3.5" />
                      <span>检查历史</span>
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* 批量巡检弹窗 */}
      {showBatchModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm p-4">
          <div className="bg-surface border border-border rounded-2xl shadow-2xl w-full max-w-2xl overflow-hidden flex flex-col max-h-[90vh] animate-in fade-in zoom-in-95 duration-200">
            {/* 弹窗头部 */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-gradient-to-r from-blue-600/10 via-purple-600/10 to-indigo-600/10">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-xl bg-primary/20 border border-primary/30 flex items-center justify-center text-primary shadow-md shadow-primary/10">
                  <ShieldCheck className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="font-bold text-text-primary text-base">服务器集群批量巡检</h3>
                  <p className="text-xs text-text-secondary mt-0.5">
                    已勾选 <span className="font-bold text-primary">{selectedServerIds.size}</span> 台服务器进行并发巡检
                  </p>
                </div>
              </div>
              <button
                onClick={() => setShowBatchModal(false)}
                className="p-1.5 text-text-secondary hover:text-text-primary hover:bg-background rounded-lg transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* 弹窗内容 */}
            <div className="p-6 space-y-5 overflow-y-auto flex-1">
              {/* 所选服务器与品牌分布 */}
              <div>
                <div className="flex items-center justify-between mb-2">
                  <span className="text-xs font-semibold text-text-tertiary uppercase tracking-wider">
                    受检设备与品牌分布
                  </span>
                  <div className="flex flex-wrap gap-1.5">
                    {Object.entries(selectedVendorsDistribution).map(([vendor, count]) => {
                      const meta = getVendorMeta(vendor);
                      return (
                        <span
                          key={vendor}
                          className={clsx('text-[11px] px-2 py-0.5 rounded-full border font-medium flex items-center gap-1', meta.bg, meta.color, meta.border)}
                        >
                          <Building2 className="w-3 h-3" />
                          {meta.label.split(' ')[0]}: {count}台
                        </span>
                      );
                    })}
                  </div>
                </div>

                <div className="max-h-28 overflow-y-auto p-2 bg-background border border-border rounded-xl text-xs space-y-1">
                  {selectedServersList.map((s) => {
                    const meta = getVendorMeta(s.vendor);
                    return (
                      <div key={s.id} className="flex justify-between items-center py-1 px-1.5 rounded hover:bg-surface/50 text-text-secondary">
                        <div className="flex items-center gap-2">
                          <span className="font-medium text-text-primary">{s.name}</span>
                          <span className={clsx('text-[10px] px-1.5 py-0.2 rounded border', meta.bg, meta.color, meta.border)}>
                            {meta.label.split(' ')[0]}
                          </span>
                        </div>
                        <span className="font-mono text-text-tertiary">{s.hostname}:{s.port}</span>
                      </div>
                    );
                  })}
                </div>
              </div>

              {/* 巡检模式选择卡片 */}
              <div>
                <label className="block text-xs font-semibold text-text-tertiary uppercase tracking-wider mb-2">
                  选择巡检模式
                </label>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                  {/* 模式 1：品牌专属自动联动 */}
                  <button
                    type="button"
                    onClick={() => setBatchMode('brand_auto')}
                    className={clsx(
                      'p-3.5 rounded-xl border text-left transition-all relative overflow-hidden flex flex-col justify-between',
                      batchMode === 'brand_auto'
                        ? 'border-purple-500 bg-purple-500/10 text-purple-300 shadow-md shadow-purple-500/10 ring-1 ring-purple-500/40'
                        : 'border-border bg-background hover:border-border/80 text-text-secondary hover:text-text-primary'
                    )}
                  >
                    <div>
                      <div className="flex items-center justify-between mb-1.5">
                        <div className="flex items-center gap-1.5">
                          <Sparkles className="w-4 h-4 text-purple-400" />
                          <span className="text-sm font-bold text-text-primary">品牌专属联动</span>
                        </div>
                        <span className="text-[10px] px-1.5 py-0.2 rounded-full bg-purple-500/20 text-purple-300 font-medium border border-purple-500/30">
                          推荐
                        </span>
                      </div>
                      <p className="text-xs text-text-tertiary leading-relaxed mt-1">
                        智能识别每台服务器所属品牌，自动分发对应专属五层巡检模版
                      </p>
                    </div>
                  </button>

                  {/* 模式 2：统一指定模版 */}
                  <button
                    type="button"
                    onClick={() => setBatchMode('custom_template')}
                    className={clsx(
                      'p-3.5 rounded-xl border text-left transition-all relative overflow-hidden flex flex-col justify-between',
                      batchMode === 'custom_template'
                        ? 'border-indigo-500 bg-indigo-500/10 text-indigo-300 shadow-md shadow-indigo-500/10 ring-1 ring-indigo-500/40'
                        : 'border-border bg-background hover:border-border/80 text-text-secondary hover:text-text-primary'
                    )}
                  >
                    <div>
                      <div className="flex items-center gap-1.5 mb-1.5">
                        <Layers className="w-4 h-4 text-indigo-400" />
                        <span className="text-sm font-bold text-text-primary">统一指定模版</span>
                      </div>
                      <p className="text-xs text-text-tertiary leading-relaxed mt-1">
                        从模版中心选择一个指定模版，批量应用到全部服务器
                      </p>
                    </div>
                  </button>

                  {/* 模式 3：标准基线巡检 */}
                  <button
                    type="button"
                    onClick={() => setBatchMode('standard')}
                    className={clsx(
                      'p-3.5 rounded-xl border text-left transition-all relative overflow-hidden flex flex-col justify-between',
                      batchMode === 'standard'
                        ? 'border-primary bg-primary/10 text-primary shadow-md shadow-primary/10 ring-1 ring-primary/40'
                        : 'border-border bg-background hover:border-border/80 text-text-secondary hover:text-text-primary'
                    )}
                  >
                    <div>
                      <div className="flex items-center gap-1.5 mb-1.5">
                        <ShieldCheck className="w-4 h-4 text-primary" />
                        <span className="text-sm font-bold text-text-primary">标准基线巡检</span>
                      </div>
                      <p className="text-xs text-text-tertiary leading-relaxed mt-1">
                        执行通用系统健康、CPU/内存/磁盘与安全配置检查
                      </p>
                    </div>
                  </button>
                </div>
              </div>

              {/* 模式联动详细信息 */}
              {batchMode === 'brand_auto' && (
                <div className="p-4 bg-background rounded-xl border border-purple-500/20 space-y-2.5 animate-in fade-in duration-150">
                  <div className="flex items-center gap-2 text-xs font-semibold text-purple-300">
                    <Sparkles className="w-4 h-4 text-purple-400" />
                    <span>各品牌设备模版智能匹配清单：</span>
                  </div>
                  <div className="space-y-1.5 text-xs">
                    {Object.entries(selectedVendorsDistribution).map(([v, count]) => {
                      const meta = getVendorMeta(v);
                      const matchedTpl = serverTemplates.find((t) => (t.vendor || '').toLowerCase() === v);
                      return (
                        <div key={v} className="flex items-center justify-between p-2 rounded-lg bg-surface/70 border border-border">
                          <div className="flex items-center gap-2">
                            <span className={clsx('px-2 py-0.5 rounded text-[11px] font-medium border', meta.bg, meta.color, meta.border)}>
                              {meta.label.split(' ')[0]} ({count}台)
                            </span>
                            <span className="text-text-tertiary">➔</span>
                            <span className="font-medium text-text-primary">
                              {matchedTpl ? matchedTpl.name : 'Linux 通用 SmartKit 五层巡检模版'}
                            </span>
                          </div>
                          <span className="text-[11px] text-emerald-400 font-mono">
                            {matchedTpl ? `${matchedTpl.commands?.length || 0}条指令` : '标准项'}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                  <p className="text-[11px] text-text-tertiary pt-1">
                    🎯 系统将在后端执行时自动按每台服务器品牌分发对应硬件与系统诊断指令。
                  </p>
                </div>
              )}

              {batchMode === 'custom_template' && (
                <div className="p-4 bg-background rounded-xl border border-indigo-500/20 space-y-3 animate-in fade-in duration-150">
                  <div className="flex items-center justify-between">
                    <label className="text-sm font-medium text-text-primary flex items-center gap-2">
                      <Layers className="w-4 h-4 text-indigo-400" />
                      从模版中心选择统一执行的模版
                    </label>
                    {isLoadingTemplates && (
                      <span className="text-xs text-text-tertiary flex items-center gap-1">
                        <RefreshCw className="w-3 h-3 animate-spin" /> 加载中...
                      </span>
                    )}
                  </div>
                  <select
                    value={selectedBatchTemplateId}
                    onChange={(e) => setSelectedBatchTemplateId(e.target.value)}
                    className="w-full bg-surface border border-border rounded-lg px-3.5 py-2.5 text-sm text-text-primary focus:outline-none focus:border-primary font-medium"
                  >
                    {serverTemplates.map((tpl) => (
                      <option key={tpl.id} value={tpl.id}>
                        {tpl.name} ({tpl.commands?.length || 0}条指令) {tpl.isPreset ? ' [官方预置]' : ' [自定义]'}
                      </option>
                    ))}
                  </select>
                  {(() => {
                    const tpl = serverTemplates.find((t) => t.id === selectedBatchTemplateId);
                    if (!tpl) return null;
                    return (
                      <div className="p-2.5 bg-surface/60 rounded-lg border border-border text-xs text-text-secondary leading-relaxed">
                        <p>{tpl.description || '暂无说明'}</p>
                        <div className="flex gap-3 text-text-tertiary mt-1.5 pt-1.5 border-t border-border/50 text-[11px]">
                          <span>适用厂商: <strong className="text-text-primary">{tpl.vendor}</strong></span>
                          <span>指令数量: <strong className="text-emerald-400">{tpl.commands?.length || 0} 项</strong></span>
                          <span>类别: <strong className="text-text-primary">{tpl.category}</strong></span>
                        </div>
                      </div>
                    );
                  })()}
                </div>
              )}

              {batchMode === 'standard' && (
                <div className="p-4 bg-background rounded-xl border border-border space-y-2 animate-in fade-in duration-150">
                  <label className="block text-sm font-medium text-text-primary mb-1">标准巡检类型</label>
                  <select
                    value={inspectionType}
                    onChange={(e) => setInspectionType(e.target.value)}
                    className="w-full bg-surface border border-border rounded-lg px-3.5 py-2.5 text-sm text-text-primary focus:outline-none focus:border-primary font-medium"
                  >
                    <option value="standard">标准合规与系统健康巡检 (推荐)</option>
                    <option value="resource">系统资源与负载深度巡检</option>
                    <option value="security">安全配置与基线合规审计</option>
                    <option value="full">全量综合深度巡检</option>
                  </select>
                </div>
              )}

              {/* 并发与 AI 设置 */}
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
                {/* AI 智能分析 */}
                <div className="p-3 bg-background rounded-xl border border-border">
                  <label className="flex items-center justify-between cursor-pointer">
                    <div className="flex flex-col pr-2">
                      <span className="text-xs font-semibold text-text-primary">AI 智能分析</span>
                      <span className="text-[11px] text-text-tertiary mt-0.5">汇总诊断并生成报告建议</span>
                    </div>
                    <div className="relative flex-shrink-0">
                      <input
                        type="checkbox"
                        checked={batchUseAI}
                        onChange={(e) => setBatchUseAI(e.target.checked)}
                        className="sr-only peer"
                      />
                      <div className="w-10 h-5 bg-surface border-2 border-border rounded-full peer peer-checked:bg-primary peer-checked:border-primary transition-all">
                        <div className="w-3.5 h-3.5 bg-white rounded-full shadow-md absolute top-0.5 left-0.5 peer-checked:translate-x-5 transition-transform"></div>
                      </div>
                    </div>
                  </label>
                </div>

                {/* 并发工作线程数 */}
                <div className="p-3 bg-background rounded-xl border border-border flex items-center justify-between">
                  <div className="flex flex-col">
                    <span className="text-xs font-semibold text-text-primary">并发线程数</span>
                    <span className="text-[11px] text-text-tertiary mt-0.5">支持 1-20 并发 (默认 5)</span>
                  </div>
                  <input
                    type="number"
                    min={1}
                    max={20}
                    value={concurrency}
                    onChange={(e) => setConcurrency(parseInt(e.target.value) || 5)}
                    className="w-20 bg-surface border border-border rounded-lg px-2.5 py-1 text-xs text-text-primary font-mono text-center focus:outline-none focus:border-primary"
                  />
                </div>
              </div>

              <div className="p-3 bg-blue-500/10 border border-blue-500/20 rounded-xl text-xs text-blue-400 space-y-1">
                <p>💡 巡检完成后，系统将自动汇总生成巡检报告记录，并支持在报告中心下载包含每台服务器独立文件的 ZIP 压缩包。</p>
              </div>
            </div>

            {/* 弹窗底部操作 */}
            <div className="flex justify-end gap-3 px-6 py-4 border-t border-border bg-background/50">
              <button
                type="button"
                onClick={() => setShowBatchModal(false)}
                className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary rounded-xl transition-colors border border-border bg-surface"
              >
                取消
              </button>
              <button
                type="button"
                onClick={handleExecuteBatchInspect}
                disabled={isBatchInspecting}
                className="px-5 py-2.5 text-sm text-white bg-gradient-to-r from-blue-600 via-purple-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 rounded-xl font-medium shadow-lg shadow-purple-500/25 disabled:opacity-50 flex items-center gap-2"
              >
                {isBatchInspecting && <RefreshCw className="w-4 h-4 animate-spin" />}
                {isBatchInspecting ? '正在并发巡检中...' : `开始批量巡检 (${selectedServerIds.size}台)`}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 批量删除服务器确认模态框 */}
      {showBatchDeleteModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-surface border border-border rounded-xl shadow-2xl w-full max-w-md overflow-hidden animate-in fade-in zoom-in duration-200">
            <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-rose-500/10">
              <div className="flex items-center gap-2">
                <AlertTriangle className="w-5 h-5 text-rose-500" />
                <h3 className="font-semibold text-text-primary">确认批量删除服务器</h3>
              </div>
              <button
                onClick={() => setShowBatchDeleteModal(false)}
                className="p-1 text-text-secondary hover:text-text-primary rounded-lg transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="p-6 space-y-4">
              <p className="text-sm text-text-secondary">
                您已选中 <span className="font-bold text-rose-400">{selectedServerIds.size}</span> 台服务器。确定要将它们全部删除吗？关联的分组映射、巡检历史及合规检查记录也将一并清除。
              </p>

              <div className="max-h-36 overflow-y-auto p-2.5 bg-background border border-border rounded-lg text-xs space-y-1">
                {filteredServers
                  .filter((s) => selectedServerIds.has(s.id))
                  .map((s) => (
                    <div key={s.id} className="flex justify-between items-center text-text-secondary">
                      <span className="font-medium text-text-primary">{s.name}</span>
                      <span>{s.hostname}:{s.port}</span>
                    </div>
                  ))}
              </div>
            </div>

            <div className="flex justify-end gap-3 px-6 py-4 border-t border-border bg-background/50">
              <button
                type="button"
                onClick={() => setShowBatchDeleteModal(false)}
                disabled={isBatchDeleting}
                className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary rounded-lg transition-colors border border-border bg-surface"
              >
                取消
              </button>
              <button
                type="button"
                onClick={confirmBatchDeleteServers}
                disabled={isBatchDeleting}
                className="px-4 py-2 text-sm text-white bg-rose-600 hover:bg-rose-500 rounded-lg font-medium shadow-md shadow-rose-600/20 disabled:opacity-50 flex items-center gap-2"
              >
                {isBatchDeleting && <Loader2 className="w-4 h-4 animate-spin" />}
                {isBatchDeleting ? '正在删除...' : `确认批量删除 (${selectedServerIds.size}台)`}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
