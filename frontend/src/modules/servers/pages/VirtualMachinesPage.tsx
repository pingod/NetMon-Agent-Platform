import React, { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Cpu,
  Server,
  Plus,
  RefreshCw,
  Search,
  Zap,
  Terminal,
  Activity,
  HardDrive,
  Layers,
  MoreVertical,
  CheckCircle2,
  AlertCircle,
  Clock,
  Trash2,
  Edit,
  ExternalLink,
  ShieldAlert,
  Code,
  Folder,
  Tag,
  Monitor,
  LayoutGrid,
  List as ListIcon,
  CheckSquare,
  Square,
  Sparkles,
  Wifi,
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import type { Server as ServerType, ServerGroup } from './types';
import { VmMetricsDashboardModal } from '../components/VmMetricsDashboardModal';
import { BatchInstallAgentModal } from '../components/BatchInstallAgentModal';
import { AgentInstallManualModal } from '../components/AgentInstallManualModal';
import { VirtualMachineFormModal } from '../components/VirtualMachineFormModal';

export const VirtualMachinesPage: React.FC = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const [searchQuery, setSearchQuery] = useState('');
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [agentStatusFilter, setAgentStatusFilter] = useState<'all' | 'online' | 'uninstalled'>('all');
  const [viewMode, setViewMode] = useState<'grid' | 'table'>('grid');

  // 多选状态
  const [selectedIds, setSelectedIds] = useState<string[]>([]);

  // 弹窗状态
  const [isFormModalOpen, setIsFormModalOpen] = useState(false);
  const [editingVm, setEditingVm] = useState<ServerType | null>(null);

  const [isMetricsModalOpen, setIsMetricsModalOpen] = useState(false);
  const [activeMetricsVm, setActiveMetricsVm] = useState<ServerType | null>(null);

  const [isBatchInstallModalOpen, setIsBatchInstallModalOpen] = useState(false);
  const [isManualModalOpen, setIsManualModalOpen] = useState(false);
  const [manualTargetVm, setManualTargetVm] = useState<ServerType | null>(null);
  const [testingVmId, setTestingVmId] = useState<string | null>(null);

  // 测试单个虚机 SSH 连接
  const handleTestSsh = async (vm: ServerType) => {
    setTestingVmId(vm.id);
    try {
      const res = await api.post(`/servers/${vm.id}/test-connection`);
      const data = res.data?.data || res.data;
      if (data?.success) {
        toast(`✅ [${vm.name}] SSH 连接测试成功！延迟: ${data.latency || 15}ms`, 'success');
      } else {
        toast(`❌ [${vm.name}] SSH 连接失败: ${data?.message || '连接超时或认证失败'}`, 'error');
      }
    } catch (err: any) {
      const msg = err.response?.data?.message || err.response?.data?.error || err.message || '连接失败';
      toast(`❌ [${vm.name}] SSH 连接异常: ${msg}`, 'error');
    } finally {
      setTestingVmId(null);
    }
  };

  // 获取虚拟机/服务器列表
  const { data: allServers = [], isLoading, refetch } = useQuery<ServerType[]>({
    queryKey: ['virtual-machines'],
    queryFn: async () => {
      const res = await api.get('/servers');
      const list: ServerType[] = res.data?.data || res.data || [];
      // 过滤出虚拟机（或通用纳管机器）
      return list;
    },
    refetchInterval: 10000,
  });

  // 获取分组列表
  const { data: groups = [] } = useQuery<ServerGroup[]>({
    queryKey: ['server-groups'],
    queryFn: async () => {
      const res = await api.get('/server-groups');
      return res.data?.data || res.data || [];
    },
  });

  // 过滤列表（无品牌过滤！）
  const filteredVms = useMemo(() => {
    return allServers.filter((vm) => {
      // 搜索词
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        const matchName = vm.name.toLowerCase().includes(query);
        const matchHost = (vm.hostname || '').toLowerCase().includes(query);
        const matchIp = (vm.ip_address || '').toLowerCase().includes(query);
        const matchTags = (vm.tags || []).some((t) => t.toLowerCase().includes(query));
        if (!matchName && !matchHost && !matchIp && !matchTags) return false;
      }

      // 分组
      if (selectedGroupId && !vm.groups?.some((g) => g.id === selectedGroupId)) {
        return false;
      }

      // Agent 状态
      if (agentStatusFilter === 'online') {
        return vm.agent_status === 'online' || vm.agent_installed === 1;
      }
      if (agentStatusFilter === 'uninstalled') {
        return vm.agent_status !== 'online' && vm.agent_installed !== 1;
      }

      return true;
    });
  }, [allServers, searchQuery, selectedGroupId, agentStatusFilter]);

  // 统计指标
  const stats = useMemo(() => {
    const total = allServers.length;
    const online = allServers.filter((s) => s.agent_status === 'online' || s.agent_installed === 1).length;
    const uninstalled = total - online;
    return { total, online, uninstalled };
  }, [allServers]);

  // 全选/反选
  const handleSelectAll = () => {
    if (selectedIds.length === filteredVms.length) {
      setSelectedIds([]);
    } else {
      setSelectedIds(filteredVms.map((v) => v.id));
    }
  };

  const handleToggleSelect = (id: string) => {
    setSelectedIds((prev) =>
      prev.includes(id) ? prev.filter((i) => i !== id) : [...prev, id]
    );
  };

  // 单机删除
  const handleDelete = async (vm: ServerType) => {
    if (!window.confirm(`确定要删除虚拟机 "${vm.name}" 吗？此操作无法撤销。`)) return;
    try {
      await api.delete(`/servers/${vm.id}`);
      toast('虚拟机已删除', 'success');
      queryClient.invalidateQueries({ queryKey: ['virtual-machines'] });
    } catch (err: any) {
      toast('删除失败: ' + (err.response?.data?.message || err.message), 'error');
    }
  };

  // 批量删除
  const handleBatchDelete = async () => {
    if (selectedIds.length === 0) return;
    if (!window.confirm(`确定要批量删除选中的 ${selectedIds.length} 台虚拟机吗？`)) return;
    try {
      await api.post('/servers/batch-delete', { ids: selectedIds });
      toast(`已成功批量删除 ${selectedIds.length} 台虚拟机`, 'success');
      setSelectedIds([]);
      queryClient.invalidateQueries({ queryKey: ['virtual-machines'] });
    } catch (err: any) {
      toast('批量删除失败: ' + (err.response?.data?.message || err.message), 'error');
    }
  };

  return (
    <div className="space-y-6 pb-12 font-sans text-slate-100">
      
      {/* ── 顶部 Header ── */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 bg-slate-900/40 p-6 rounded-2xl border border-slate-800/80 backdrop-blur-sm">
        <div className="flex items-center gap-3.5">
          <div className="w-12 h-12 rounded-2xl bg-gradient-to-br from-indigo-500/20 to-purple-500/20 border border-indigo-500/30 flex items-center justify-center text-indigo-400 shadow-inner">
            <Cpu className="w-6 h-6" />
          </div>
          <div>
            <div className="flex items-center gap-2.5">
              <h1 className="text-2xl font-bold text-slate-100 tracking-tight">
                虚拟机管理
              </h1>
              <span className="text-xs px-2 py-0.5 rounded-md bg-indigo-500/10 border border-indigo-500/30 text-indigo-400 font-semibold">
                Agent 监控模式
              </span>
            </div>
            <p className="text-xs text-slate-400 mt-1">
              超轻量、低开销采集架构 (CPU {'<'} 0.05%, 内存 3.8MB)，支持秒级时序监控与批量并发下发
            </p>
          </div>
        </div>

        {/* 顶部操作按钮组 */}
        <div className="flex flex-wrap items-center gap-2.5">
          <button
            onClick={() => {
              if (selectedIds.length > 0) {
                setIsBatchInstallModalOpen(true);
              } else {
                setIsBatchInstallModalOpen(true);
              }
            }}
            className="flex items-center gap-2 px-4 py-2 rounded-xl bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-500 hover:to-purple-500 text-white text-xs font-semibold shadow-lg shadow-indigo-600/25 transition"
          >
            <Zap className="w-4 h-4" />
            <span>🚀 批量安装 Agent {selectedIds.length > 0 ? `(${selectedIds.length})` : ''}</span>
          </button>

          <button
            onClick={() => {
              setManualTargetVm(null);
              setIsManualModalOpen(true);
            }}
            className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-xs font-medium transition"
          >
            <Terminal className="w-4 h-4 text-indigo-400" />
            <span>📋 手动安装命令</span>
          </button>

          <button
            onClick={() => {
              setEditingVm(null);
              setIsFormModalOpen(true);
            }}
            className="flex items-center gap-2 px-3.5 py-2 rounded-xl bg-indigo-600/20 hover:bg-indigo-600/30 border border-indigo-500/30 text-indigo-300 text-xs font-medium transition"
          >
            <Plus className="w-4 h-4" />
            <span>新建虚拟机</span>
          </button>

          <button
            onClick={() => refetch()}
            disabled={isLoading}
            className="p-2 rounded-xl bg-slate-800/80 hover:bg-slate-700 border border-slate-700 text-slate-300 transition"
            title="刷新列表"
          >
            <RefreshCw className={`w-4 h-4 ${isLoading ? 'animate-spin text-indigo-400' : ''}`} />
          </button>
        </div>
      </div>

      {/* ── 顶部统计卡片 ── */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <div className="p-4 rounded-2xl bg-[#0f1422] border border-slate-800/80 flex items-center justify-between">
          <div>
            <p className="text-xs text-slate-400 font-medium">虚拟机总数</p>
            <p className="text-2xl font-bold text-slate-100 mt-1">{stats.total} <span className="text-xs text-slate-500 font-normal">台</span></p>
          </div>
          <div className="w-11 h-11 rounded-xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
            <Cpu className="w-5 h-5" />
          </div>
        </div>

        <div className="p-4 rounded-2xl bg-[#0f1422] border border-slate-800/80 flex items-center justify-between">
          <div>
            <p className="text-xs text-slate-400 font-medium">netmon-agent 在线</p>
            <div className="flex items-baseline gap-2 mt-1">
              <span className="text-2xl font-bold text-emerald-400">{stats.online}</span>
              <span className="text-xs text-slate-500">
                ({stats.total > 0 ? Math.round((stats.online / stats.total) * 100) : 0}% 覆盖)
              </span>
            </div>
          </div>
          <div className="w-11 h-11 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400">
            <CheckCircle2 className="w-5 h-5" />
          </div>
        </div>

        <div className="p-4 rounded-2xl bg-[#0f1422] border border-slate-800/80 flex items-center justify-between">
          <div>
            <p className="text-xs text-slate-400 font-medium">未安装 / 待部署</p>
            <p className="text-2xl font-bold text-amber-400 mt-1">{stats.uninstalled} <span className="text-xs text-slate-500 font-normal">台</span></p>
          </div>
          <div className="w-11 h-11 rounded-xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center text-amber-400">
            <Zap className="w-5 h-5" />
          </div>
        </div>
      </div>

      {/* ── 筛选工具栏（无品牌过滤，精简高效） ── */}
      <div className="flex flex-wrap items-center justify-between gap-3 p-3.5 rounded-2xl bg-slate-900/40 border border-slate-800">
        
        {/* 左侧：搜索 + 分组 + Agent 过滤 */}
        <div className="flex flex-wrap items-center gap-2.5 flex-1 min-w-[280px]">
          <div className="relative flex-1 min-w-[200px] max-w-md">
            <Search className="w-4 h-4 text-slate-400 absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="搜索虚拟机名称、IP 地址、标签..."
              className="w-full pl-9 pr-4 py-1.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-200 focus:border-indigo-500 focus:outline-none"
            />
          </div>

          <select
            aria-label="虚拟机分组筛选"
            value={selectedGroupId || ''}
            onChange={(e) => setSelectedGroupId(e.target.value || null)}
            className="px-3 py-1.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-300 focus:border-indigo-500"
          >
            <option value="">全部虚拟机分组</option>
            {groups.map((g) => (
              <option key={g.id} value={g.id}>
                {g.name}
              </option>
            ))}
          </select>

          <div className="flex items-center bg-slate-950 p-0.5 rounded-xl border border-slate-800 text-xs">
            <button
              onClick={() => setAgentStatusFilter('all')}
              className={`px-2.5 py-1 rounded-lg font-medium transition ${
                agentStatusFilter === 'all' ? 'bg-indigo-600 text-white' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              全部 ({allServers.length})
            </button>
            <button
              onClick={() => setAgentStatusFilter('online')}
              className={`px-2.5 py-1 rounded-lg font-medium transition ${
                agentStatusFilter === 'online' ? 'bg-emerald-600 text-white' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              🟢 在线 ({stats.online})
            </button>
            <button
              onClick={() => setAgentStatusFilter('uninstalled')}
              className={`px-2.5 py-1 rounded-lg font-medium transition ${
                agentStatusFilter === 'uninstalled' ? 'bg-amber-600 text-white' : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              ⚪ 未安装 ({stats.uninstalled})
            </button>
          </div>
        </div>

        {/* 右侧：视图切换与批量操作 */}
        <div className="flex items-center gap-2">
          {selectedIds.length > 0 && (
            <button
              onClick={handleBatchDelete}
              className="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-red-500/10 hover:bg-red-500/20 border border-red-500/30 text-red-400 text-xs font-medium transition"
            >
              <Trash2 className="w-3.5 h-3.5" />
              <span>批量删除 ({selectedIds.length})</span>
            </button>
          )}

          <div className="flex items-center bg-slate-950 p-1 rounded-xl border border-slate-800">
            <button
              onClick={() => setViewMode('grid')}
              className={`p-1.5 rounded-lg transition ${
                viewMode === 'grid' ? 'bg-slate-800 text-indigo-400' : 'text-slate-400 hover:text-slate-200'
              }`}
              title="网格视图"
            >
              <LayoutGrid className="w-4 h-4" />
            </button>
            <button
              onClick={() => setViewMode('table')}
              className={`p-1.5 rounded-lg transition ${
                viewMode === 'table' ? 'bg-slate-800 text-indigo-400' : 'text-slate-400 hover:text-slate-200'
              }`}
              title="表格视图"
            >
              <ListIcon className="w-4 h-4" />
            </button>
          </div>
        </div>
      </div>

      {/* ── 虚拟机列表展示 ── */}
      {filteredVms.length === 0 ? (
        <div className="p-12 text-center rounded-2xl bg-slate-900/30 border border-slate-800 text-slate-400 space-y-3">
          <Cpu className="w-10 h-10 mx-auto text-slate-600" />
          <p className="text-sm font-medium">未找到匹配的虚拟机</p>
          <button
            onClick={() => {
              setEditingVm(null);
              setIsFormModalOpen(true);
            }}
            className="px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-medium inline-flex items-center gap-2"
          >
            <Plus className="w-4 h-4" /> 新建虚拟机
          </button>
        </div>
      ) : viewMode === 'grid' ? (
        /* 网格视图 */
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredVms.map((vm) => {
            const isSelected = selectedIds.includes(vm.id);
            const isOnline = vm.agent_status === 'online' || vm.agent_installed === 1;

            return (
              <div
                key={vm.id}
                className={`group relative p-5 rounded-2xl bg-[#0e1322] border transition-all hover:shadow-xl hover:border-indigo-500/40 ${
                  isSelected ? 'border-indigo-500 ring-1 ring-indigo-500 bg-indigo-950/10' : 'border-slate-800/80'
                }`}
              >
                {/* 勾选框与标题 */}
                <div className="flex items-start justify-between gap-2 mb-3">
                  <div className="flex items-center gap-2.5 min-w-0">
                    <button
                      onClick={() => handleToggleSelect(vm.id)}
                      className="text-slate-400 hover:text-indigo-400 transition"
                    >
                      {isSelected ? (
                        <CheckSquare className="w-4 h-4 text-indigo-400" />
                      ) : (
                        <Square className="w-4 h-4 text-slate-600" />
                      )}
                    </button>
                    <div className="min-w-0">
                      <h3 className="text-sm font-bold text-slate-100 truncate group-hover:text-indigo-300 transition">
                        {vm.name}
                      </h3>
                      <p className="text-[11px] text-slate-400 font-mono mt-0.5 truncate">
                        {vm.hostname || vm.ip_address || '未配置 IP'}
                      </p>
                    </div>
                  </div>

                  {/* 操作系统标签 */}
                  <span className="text-[11px] px-2 py-0.5 rounded-md bg-slate-800/80 border border-slate-700 text-slate-300 shrink-0">
                    {vm.os_type === 'windows' ? 'Windows' : 'Linux'}
                  </span>
                </div>

                {/* 规格参数概览（由 Agent 自动采集上报） */}
                <div className="grid grid-cols-3 gap-2 p-2.5 rounded-xl bg-slate-950/60 border border-slate-800/60 text-xs mb-3.5">
                  <div>
                    <span className="text-[10px] text-slate-500 block">CPU</span>
                    <span className="font-semibold text-slate-200">{vm.cpu_cores ? `${vm.cpu_cores} 核` : isOnline ? '已接入' : '待采集'}</span>
                  </div>
                  <div>
                    <span className="text-[10px] text-slate-500 block">内存</span>
                    <span className="font-semibold text-slate-200">{vm.memory_gb ? `${vm.memory_gb} GB` : isOnline ? '已接入' : '待采集'}</span>
                  </div>
                  <div>
                    <span className="text-[10px] text-slate-500 block">磁盘</span>
                    <span className="font-semibold text-slate-200">{vm.disk_gb ? `${vm.disk_gb} GB` : isOnline ? '已接入' : '待采集'}</span>
                  </div>
                </div>

                {/* Agent 状态徽章条 */}
                <div className="flex items-center justify-between text-xs py-1.5 border-t border-slate-800/60 mb-3.5">
                  <span className="text-slate-400 text-[11px]">采集器状态:</span>
                  {isOnline ? (
                    <div className="flex items-center gap-1.5 text-emerald-400 font-medium text-[11px]">
                      <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
                      <span>netmon-agent 在线</span>
                    </div>
                  ) : (
                    <div className="flex items-center gap-1.5 text-slate-400 text-[11px]">
                      <span className="w-2 h-2 rounded-full bg-slate-600"></span>
                      <span>未安装 Agent</span>
                    </div>
                  )}
                </div>

                {/* 底部功能快捷操作组 */}
                <div className="grid grid-cols-3 gap-2 pt-1 border-t border-slate-800/60">
                  {/* 性能大屏按钮 (重点突出) */}
                  <button
                    onClick={() => {
                      setActiveMetricsVm(vm);
                      setIsMetricsModalOpen(true);
                    }}
                    className="flex items-center justify-center gap-1 py-1.5 px-2 rounded-xl bg-indigo-600/20 hover:bg-indigo-600/30 border border-indigo-500/30 text-indigo-300 text-xs font-semibold transition shadow-sm"
                    title="性能指标大屏"
                  >
                    <Activity className="w-3.5 h-3.5" />
                    <span>性能大屏</span>
                  </button>

                  {/* 测试 SSH 按钮 */}
                  <button
                    onClick={() => handleTestSsh(vm)}
                    disabled={testingVmId === vm.id}
                    className="flex items-center justify-center gap-1 py-1.5 px-2 rounded-xl bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-xs font-medium transition shadow-sm disabled:opacity-50"
                    title="测试该虚拟机的 SSH 连通性与密码凭据"
                  >
                    <Wifi className={`w-3.5 h-3.5 ${testingVmId === vm.id ? 'animate-spin text-indigo-400' : 'text-emerald-400'}`} />
                    <span>{testingVmId === vm.id ? '测试中' : '测试 SSH'}</span>
                  </button>

                  {/* 一键安装 / 命令 */}
                  {isOnline ? (
                    <button
                      onClick={() => navigate(`/terminal?host=${vm.hostname || vm.ip_address}&user=${vm.username || 'root'}`)}
                      className="flex items-center justify-center gap-1 py-1.5 px-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
                      title="打开 Web 终端"
                    >
                      <Terminal className="w-3.5 h-3.5 text-slate-400" />
                      <span>Web 终端</span>
                    </button>
                  ) : (
                    <button
                      onClick={() => {
                        setManualTargetVm(vm);
                        setIsManualModalOpen(true);
                      }}
                      className="flex items-center justify-center gap-1 py-1.5 px-2 rounded-xl bg-purple-500/10 hover:bg-purple-500/20 border border-purple-500/30 text-purple-300 text-xs font-medium transition"
                      title="安装 netmon-agent"
                    >
                      <Zap className="w-3.5 h-3.5" />
                      <span>安装 Agent</span>
                    </button>
                  )}
                </div>

                {/* 右上角悬浮修改与删除菜单 */}
                <div className="absolute top-4 right-4 opacity-0 group-hover:opacity-100 transition flex items-center gap-1 bg-slate-900/90 p-1 rounded-lg border border-slate-700">
                  <button
                    onClick={() => handleTestSsh(vm)}
                    disabled={testingVmId === vm.id}
                    title="测试 SSH 连接"
                    className="p-1 hover:text-emerald-400 text-slate-400 transition disabled:opacity-50"
                  >
                    <Wifi className={`w-3.5 h-3.5 ${testingVmId === vm.id ? 'animate-spin text-indigo-400' : ''}`} />
                  </button>
                  <button
                    onClick={() => {
                      setEditingVm(vm);
                      setIsFormModalOpen(true);
                    }}
                    title="编辑虚拟机"
                    className="p-1 hover:text-indigo-300 transition"
                  >
                    <Edit className="w-3.5 h-3.5" />
                  </button>
                  <button
                    onClick={() => handleDelete(vm)}
                    title="删除虚拟机"
                    className="p-1 hover:text-red-400 transition"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        /* 表格视图 */
        <div className="overflow-x-auto rounded-2xl bg-[#0e1322] border border-slate-800 shadow-xl">
          <table className="w-full text-left text-xs text-slate-300">
            <thead className="bg-slate-900/80 border-b border-slate-800 text-slate-400 font-semibold">
              <tr>
                <th className="p-4 w-10">
                  <input
                    type="checkbox"
                    checked={selectedIds.length === filteredVms.length && filteredVms.length > 0}
                    onChange={handleSelectAll}
                    className="rounded bg-slate-950 border-slate-700 text-indigo-600 focus:ring-indigo-500"
                  />
                </th>
                <th className="p-4">虚拟机名称</th>
                <th className="p-4">主机名 / IP</th>
                <th className="p-4">系统</th>
                <th className="p-4">配置规格</th>
                <th className="p-4">Agent 状态</th>
                <th className="p-4 text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {filteredVms.map((vm) => {
                const isSelected = selectedIds.includes(vm.id);
                const isOnline = vm.agent_status === 'online' || vm.agent_installed === 1;

                return (
                  <tr key={vm.id} className="hover:bg-slate-800/30 transition">
                    <td className="p-4">
                      <input
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => handleToggleSelect(vm.id)}
                        className="rounded bg-slate-950 border-slate-700 text-indigo-600 focus:ring-indigo-500"
                      />
                    </td>
                    <td className="p-4 font-bold text-slate-100">{vm.name}</td>
                    <td className="p-4 font-mono text-slate-400">{vm.hostname || vm.ip_address}</td>
                    <td className="p-4">
                      <span className="px-2 py-0.5 rounded bg-slate-800 border border-slate-700 text-slate-300">
                        {vm.os_type === 'windows' ? 'Windows' : 'Linux'}
                      </span>
                    </td>
                    <td className="p-4 text-slate-300">
                      {vm.cpu_cores || vm.memory_gb || vm.disk_gb
                        ? `${vm.cpu_cores || '-'}核 / ${vm.memory_gb || '-'}GB / ${vm.disk_gb || '-'}GB`
                        : isOnline
                        ? '🟢 Agent 采集中'
                        : '待安装 Agent'}
                    </td>
                    <td className="p-4">
                      {isOnline ? (
                        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 font-medium">
                          <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
                          netmon-agent 在线
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md bg-slate-800 text-slate-400">
                          未安装
                        </span>
                      )}
                    </td>
                    <td className="p-4 text-right space-x-2">
                      <button
                        onClick={() => handleTestSsh(vm)}
                        disabled={testingVmId === vm.id}
                        className="px-2.5 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 font-medium transition disabled:opacity-50 inline-flex items-center gap-1"
                        title="测试该虚拟机的 SSH 连通性与密码凭据"
                      >
                        <Wifi className={`w-3 h-3 ${testingVmId === vm.id ? 'animate-spin text-indigo-400' : 'text-emerald-400'}`} />
                        <span>{testingVmId === vm.id ? '测试中' : '测试 SSH'}</span>
                      </button>
                      <button
                        onClick={() => {
                          setActiveMetricsVm(vm);
                          setIsMetricsModalOpen(true);
                        }}
                        className="px-2.5 py-1 rounded-lg bg-indigo-600/20 hover:bg-indigo-600/30 border border-indigo-500/30 text-indigo-300 font-medium transition"
                      >
                        性能大屏
                      </button>
                      <button
                        onClick={() => {
                          setEditingVm(vm);
                          setIsFormModalOpen(true);
                        }}
                        className="px-2 py-1 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-300 transition"
                      >
                        编辑
                      </button>
                      <button
                        onClick={() => handleDelete(vm)}
                        className="px-2 py-1 rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-400 transition"
                      >
                        删除
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* ── 虚拟机性能大屏弹窗（对照截图 2x2 极简深色大屏） ── */}
      <VmMetricsDashboardModal
        isOpen={isMetricsModalOpen}
        onClose={() => {
          setIsMetricsModalOpen(false);
          setActiveMetricsVm(null);
        }}
        server={activeMetricsVm}
      />

      {/* ── 批量下发安装 Agent 弹窗 ── */}
      <BatchInstallAgentModal
        isOpen={isBatchInstallModalOpen}
        onClose={() => {
          setIsBatchInstallModalOpen(false);
          queryClient.invalidateQueries({ queryKey: ['virtual-machines'] });
        }}
        selectedServers={
          selectedIds.length > 0
            ? allServers.filter((s) => selectedIds.includes(s.id))
            : allServers
        }
        onSuccess={() => {
          queryClient.invalidateQueries({ queryKey: ['virtual-machines'] });
        }}
      />

      {/* ── 手动安装 Agent 命令弹窗 ── */}
      <AgentInstallManualModal
        isOpen={isManualModalOpen}
        onClose={() => {
          setIsManualModalOpen(false);
          setManualTargetVm(null);
        }}
        server={manualTargetVm}
      />

      {/* ── 新建/编辑虚拟机弹窗（无品牌属性） ── */}
      <VirtualMachineFormModal
        isOpen={isFormModalOpen}
        onClose={() => {
          setIsFormModalOpen(false);
          setEditingVm(null);
        }}
        onSuccess={() => {
          queryClient.invalidateQueries({ queryKey: ['virtual-machines'] });
        }}
        vm={editingVm}
        groups={groups}
      />

    </div>
  );
};
