/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo, useRef, useCallback, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  Plus, Trash2, RefreshCw, Play, Clock, FileText,
  ChevronRight, ChevronDown, X, Loader2, Search, Upload,
  Database, Activity, ToggleLeft, ToggleRight, Settings,
  Server, CheckCircle2, XCircle, AlertTriangle, Monitor,
  FileSpreadsheet, FolderOpen, Zap, FileArchive,
  CheckSquare, Square, BarChart3, Tag, Cpu, HardDrive, Wind, Sun, Copy, Check, Key,
  Eye, EyeOff, Shield
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';
import { useDebounce } from '../../../hooks/useDebounce';
import MibTransferModal from '../components/MibTransferModal';
import MetricPanelsTab from '../components/MetricPanelsTab';
import SendToDashboardModal from '../components/SendToDashboardModal';
import CustomOidTestAndAddModal from '../components/CustomOidTestAndAddModal';

// ============================================================
// 常量
// ============================================================

const INTERVAL_OPTIONS = [
  { value: 60, label: '1 分钟' },
  { value: 300, label: '5 分钟' },
  { value: 600, label: '10 分钟' },
  { value: 900, label: '15 分钟' },
  { value: 1800, label: '30 分钟' },
  { value: 3600, label: '1 小时' },
  { value: 21600, label: '6 小时' },
  { value: 43200, label: '12 小时' },
  { value: 86400, label: '24 小时' },
];

const SNMP_VERSION_OPTIONS = [
  { value: 'v1', label: 'SNMPv1' },
  { value: 'v2c', label: 'SNMPv2c' },
  { value: 'v3', label: 'SNMPv3' },
];

const DEVICE_TYPE_OPTIONS = [
  { value: 'network', label: '网络设备' },
  { value: 'server', label: '服务器' },
  { value: 'all', label: '全部' },
];

const FREQUENCY_MODE_OPTIONS = [
  { value: 'interval', label: '间隔模式' },
  { value: 'cron', label: 'Cron 模式' },
  { value: 'manual', label: '手动模式' },
];

export function getHardwareTags(name: string = '', desc: string = '', syntax: string = '') {
  const text = `${name} ${desc} ${syntax}`.toLowerCase();
  const lowerName = name.toLowerCase();

  const isLldpOrMeta = lowerName.startsWith('lldp') || lowerName.includes('member') || lowerName.includes('element') || lowerName.includes('frame');

  const tags: { key: string; label: string; bg: string; text: string; border: string; icon: any }[] = [];

  // CPU: cpu, processor, cpuratio, cpuusage, cpuload, cpuutil, 处理器
  if (!lowerName.startsWith('lldp') && /(cpu|processor|cpuload|cpuusage|cpuutil|cpuratio|cpupcent|zxancpu|zxr10cpu|处理器)/i.test(text)) {
    tags.push({ key: 'cpu', label: 'CPU', bg: 'bg-purple-500/15', text: 'text-purple-400', border: 'border-purple-500/30', icon: Cpu });
  }

  // 内存 (Memory): memory, memusage, memutil, memsize, memfree, memused, memtotal, memratio, zxanmem, zxr10mem, ram, flash, buffer, 内存
  if (!isLldpOrMeta && /(memory|memusage|memutil|memsize|memfree|memused|memtotal|memcapacity|memratio|ramusage|flashsize|bufferpool|zxanmem|zxr10mem|内存|\bmem\b)/i.test(text)) {
    if (!/(remote|lldprem|member|frame|element|comment|dimension|summary|timestamp)/i.test(lowerName)) {
      tags.push({ key: 'memory', label: '内存', bg: 'bg-indigo-500/15', text: 'text-indigo-400', border: 'border-indigo-500/30', icon: HardDrive });
    }
  }

  // 温度 (Temperature): temperature, tempval, boardtemp, racktemp, envtemp, temp, 温度, 摄氏度
  if (!lowerName.startsWith('lldp') && /(temperature|tempval|boardtemp|racktemp|envtemp|shelftemp|thermal|温度|摄氏度|\btemp\b)/i.test(text) && !/attempt|template/i.test(text)) {
    tags.push({ key: 'temp', label: '温度', bg: 'bg-rose-500/15', text: 'text-rose-400', border: 'border-rose-500/30', icon: Activity });
  }

  // 风扇 (Fan): fan, fanspeed, fanstatus, fanstate, rackfan, shelffan, envfan, wind, cooler, ventilator, 风扇
  if (!lowerName.startsWith('lldp') && /(fan|fanspeed|fanstatus|fanstate|rackfan|shelffan|envfan|wind|cooler|ventilator|风扇)/i.test(text)) {
    tags.push({ key: 'fan', label: '风扇', bg: 'bg-cyan-500/15', text: 'text-cyan-400', border: 'border-cyan-500/30', icon: Wind });
  }

  // 电源 (Power): power, psu, voltage, current, powervoltage, powerstatus, supply, 电源, 电压, 电流, 供电
  if (!lowerName.startsWith('lldp') && /(power|psu|voltage|current|supply|powervoltage|powerstatus|电源|电压|电流|电源状态|供电)/i.test(text) && !/powerof2/i.test(text)) {
    tags.push({ key: 'power', label: '电源', bg: 'bg-amber-500/15', text: 'text-amber-400', border: 'border-amber-500/30', icon: Zap });
  }

  // 光模块 (Optical): optical, optic, transceiver, sfp, qsfp, txpower, rxpower, bias, laser, light, opticalpower, 光模块, 收光, 发光, 光功率
  if (!lowerName.startsWith('lldp') && /(optical|optic|transceiver|sfp|sfp\+|qsfp|txpower|rxpower|bias|laser|light|opticalpower|光模块|收光|发光|光功率|偏置电流)/i.test(text)) {
    tags.push({ key: 'optical', label: '光模块', bg: 'bg-emerald-500/15', text: 'text-emerald-400', border: 'border-emerald-500/30', icon: Sun });
  }

  return tags;
}

// ============================================================
// 高频硬件指标 Tab 组件 (CPU, Memory, Fan, Power, Optical)
// ============================================================

function HardwareMetricsTab({ onOpenTest }: { onOpenTest: (target: { moduleId?: string; oid?: string; name?: string; is_table?: number }) => void }) {
  const toast = useToast();
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [searchTerm, setSearchTerm] = useState('');
  const [copiedOid, setCopiedOid] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const debouncedSearch = useDebounce(searchTerm, 300);

  const { data: hardwareObjects = [], isLoading } = useQuery({
    queryKey: ['snmp-hardware-objects', selectedCategory],
    queryFn: () => api.get('/snmp-collect/mib-objects/hardware', { params: { category: selectedCategory, limit: 500 } }).then(res => res.data.data || []),
  });

  const categoryCounts = useMemo(() => {
    const counts = { cpu: 0, memory: 0, fan: 0, power: 0, optical: 0, all: 0 };
    hardwareObjects.forEach((obj: any) => {
      const tags = getHardwareTags(obj.name, obj.description, obj.syntax);
      tags.forEach(t => {
        if (counts[t.key as keyof typeof counts] !== undefined) {
          counts[t.key as keyof typeof counts]++;
        }
      });
      counts.all++;
    });
    return counts;
  }, [hardwareObjects]);

  const filteredObjects = useMemo(() => {
    if (!debouncedSearch.trim()) return hardwareObjects;
    const term = debouncedSearch.toLowerCase();
    return hardwareObjects.filter((obj: any) =>
      obj.oid.toLowerCase().includes(term) ||
      obj.name.toLowerCase().includes(term) ||
      (obj.description && obj.description.toLowerCase().includes(term)) ||
      (obj.module_name && obj.module_name.toLowerCase().includes(term))
    );
  }, [hardwareObjects, debouncedSearch]);

  const pageSize = 50;
  const totalPages = Math.ceil(filteredObjects.length / pageSize) || 1;
  const displayObjects = useMemo(() => {
    const start = (page - 1) * pageSize;
    return filteredObjects.slice(start, start + pageSize);
  }, [filteredObjects, page]);

  const copyOid = (oid: string) => {
    navigator.clipboard.writeText(oid);
    setCopiedOid(oid);
    toast.success(`OID ${oid} 已复制到剪贴板`);
    setTimeout(() => setCopiedOid(null), 2000);
  };

  return (
    <div className="space-y-6">
      {/* 顶部高频指标概览卡片 */}
      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-5 gap-4">
        {[
          { key: 'cpu', label: 'CPU 利用率与负荷', icon: Cpu, color: 'from-purple-500/20 to-purple-950/30 border-purple-500/30 text-purple-400', countKey: 'cpu' },
          { key: 'memory', label: '内存与存储空间', icon: HardDrive, color: 'from-indigo-500/20 to-indigo-950/30 border-indigo-500/30 text-indigo-400', countKey: 'memory' },
          { key: 'fan', label: '风扇转速与风道状态', icon: Wind, color: 'from-cyan-500/20 to-cyan-950/30 border-cyan-500/30 text-cyan-400', countKey: 'fan' },
          { key: 'power', label: '电源模块与供电电压', icon: Zap, color: 'from-amber-500/20 to-amber-950/30 border-amber-500/30 text-amber-400', countKey: 'power' },
          { key: 'optical', label: '光模块收发光功率', icon: Sun, color: 'from-emerald-500/20 to-emerald-950/30 border-emerald-500/30 text-emerald-400', countKey: 'optical' },
        ].map(card => {
          const count = categoryCounts[card.countKey as keyof typeof categoryCounts] || 0;
          return (
            <div
              key={card.key}
              onClick={() => setSelectedCategory(selectedCategory === card.key ? 'all' : card.key)}
              className={`p-4 rounded-xl border bg-gradient-to-br transition-all cursor-pointer ${card.color} ${
                selectedCategory === card.key ? 'ring-2 ring-blue-500 shadow-lg scale-[1.02]' : 'hover:scale-[1.01]'
              }`}
            >
              <div className="flex items-center justify-between">
                <card.icon className="w-5 h-5" />
                <span className="text-xs font-mono font-semibold px-2 py-0.5 rounded-full bg-surface/50 border border-border">
                  {count} OIDs
                </span>
              </div>
              <p className="mt-3 text-sm font-semibold text-text">{card.label}</p>
              <p className="text-[11px] text-text-secondary mt-1">支持 SNMP GET/WALK 轮询</p>
            </div>
          );
        })}
      </div>

      {/* 分类筛选 Chip + 搜索框 */}
      <div className="flex flex-wrap items-center justify-between gap-4 bg-surface p-4 rounded-xl border border-border shadow-sm">
        <div className="flex flex-wrap items-center gap-2">
          {[
            { key: 'all', label: '全部高频指标', icon: Zap },
            { key: 'cpu', label: 'CPU', icon: Cpu },
            { key: 'memory', label: '内存', icon: HardDrive },
            { key: 'fan', label: '风扇', icon: Wind },
            { key: 'power', label: '电源', icon: Zap },
            { key: 'optical', label: '光模块', icon: Sun },
          ].map(item => (
            <button
              key={item.key}
              onClick={() => setSelectedCategory(item.key)}
              className={`flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-all ${
                selectedCategory === item.key
                  ? 'bg-blue-600 text-white shadow-sm'
                  : 'bg-surface-secondary/40 text-text-secondary hover:text-text hover:bg-surface-secondary'
              }`}
            >
              <item.icon className="w-3.5 h-3.5" />
              {item.label}
              <span className="ml-1 text-[10px] opacity-75 font-mono">
                ({categoryCounts[item.key as keyof typeof categoryCounts] || 0})
              </span>
            </button>
          ))}
        </div>

        <div className="relative w-full sm:w-72">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" />
          <input
            type="text"
            value={searchTerm}
            onChange={e => setSearchTerm(e.target.value)}
            placeholder="搜索 OID, 名称, 中文描述..."
            className="w-full pl-9 pr-3 py-1.5 bg-surface-secondary border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
          />
        </div>
      </div>

      {/* 核心指标列表 */}
      <div className="bg-surface rounded-xl border border-border overflow-hidden shadow-sm">
        {isLoading ? (
          <div className="flex items-center justify-center py-12 text-text-secondary">
            <Loader2 className="w-6 h-6 animate-spin mr-2" /> 正在智能识别并高亮硬件指标对象...
          </div>
        ) : filteredObjects.length === 0 ? (
          <div className="text-center py-12 text-text-secondary">
            <Activity className="w-10 h-10 mx-auto mb-2 opacity-30" />
            <p className="text-sm font-medium">暂未识别到该分类下的高频硬件 OID</p>
            <p className="text-xs mt-1">请上传包含硬件描述的 MIB 或 Excel 文件</p>
          </div>
        ) : (
          <table className="w-full text-left">
            <thead className="bg-surface-secondary border-b border-border">
              <tr>
                <th className="px-4 py-3 text-xs font-semibold text-text-secondary">硬件高亮标签</th>
                <th className="px-4 py-3 text-xs font-semibold text-text-secondary">OID 编号</th>
                <th className="px-4 py-3 text-xs font-semibold text-text-secondary">指标/对象名称</th>
                <th className="px-4 py-3 text-xs font-semibold text-text-secondary">中文描述 / 说明</th>
                <th className="px-4 py-3 text-xs font-semibold text-text-secondary">来源 MIB 模块</th>
                <th className="px-4 py-3 text-xs font-semibold text-text-secondary text-right">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border/40">
              {displayObjects.map((obj: any) => {
                const tags = getHardwareTags(obj.name, obj.description, obj.syntax);
                return (
                  <tr key={obj.id || obj.oid} className="hover:bg-blue-500/5 transition-colors">
                    <td className="px-4 py-2.5">
                      <div className="flex flex-wrap gap-1">
                        {tags.length > 0 ? (
                          tags.map(t => (
                            <span
                              key={t.key}
                              className={`inline-flex items-center gap-1 px-2 py-0.5 text-[11px] font-semibold rounded-md border ${t.bg} ${t.text} ${t.border}`}
                            >
                              <t.icon className="w-3 h-3" />
                              {t.label}
                            </span>
                          ))
                        ) : (
                          <span className="text-xs text-text-secondary">-</span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-2.5 font-mono text-xs text-blue-400 font-medium">
                      <div className="flex items-center gap-1.5">
                        <span>{obj.oid}</span>
                        <button
                          onClick={() => copyOid(obj.oid)}
                          className="p-1 hover:bg-blue-500/10 rounded text-text-secondary hover:text-blue-400 transition-colors"
                          title="复制 OID"
                        >
                          {copiedOid === obj.oid ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
                        </button>
                      </div>
                    </td>
                    <td className="px-4 py-2.5 text-xs font-semibold text-text">
                      {obj.name}
                      {obj.syntax && <span className="ml-2 text-[10px] font-mono text-text-secondary">({obj.syntax})</span>}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-text-secondary max-w-xs truncate" title={obj.description}>
                      {obj.description || '-'}
                    </td>
                    <td className="px-4 py-2.5 text-xs text-text-secondary">
                      <span className="px-2 py-0.5 rounded bg-surface-secondary text-text font-mono text-[11px]">
                        {obj.module_name || 'MIB'}
                      </span>
                    </td>
                    <td className="px-4 py-2.5 text-right">
                      <button
                        onClick={() => onOpenTest({ oid: obj.oid, name: obj.name, is_table: obj.is_table, moduleId: obj.module_id })}
                        className="inline-flex items-center gap-1 px-2.5 py-1 text-xs font-medium text-blue-400 hover:bg-blue-500/10 rounded-md transition-colors"
                      >
                        <Zap className="w-3.5 h-3.5" />
                        采集测试
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}

        {totalPages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 bg-surface-secondary border-t border-border text-xs text-text-secondary">
            <span>显示第 {(page - 1) * pageSize + 1} - {Math.min(page * pageSize, filteredObjects.length)} 条，共 {filteredObjects.length} 条对象</span>
            <div className="flex items-center gap-1">
              <button
                disabled={page <= 1}
                onClick={() => setPage(p => p - 1)}
                className="px-2 py-1 rounded bg-surface hover:bg-border border border-border disabled:opacity-40"
              >
                上一页
              </button>
              <span className="px-2">{page} / {totalPages}</span>
              <button
                disabled={page >= totalPages}
                onClick={() => setPage(p => p + 1)}
                className="px-2 py-1 rounded bg-surface hover:bg-border border border-border disabled:opacity-40"
              >
                下一页
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

// ============================================================
// 主组件
// ============================================================

export default function SnmpCollect() {
  const navigate = useNavigate();
  const [activeTab, setActiveTab] = useState<'mib' | 'hardware' | 'tasks' | 'data' | 'panels'>('mib');
  const [testModalOpen, setTestModalOpen] = useState(false);
  const [testTargetItem, setTestTargetItem] = useState<{ moduleId?: string; oid?: string; name?: string; is_table?: number } | null>(null);

  return (
    <div className="p-6 h-full flex flex-col">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-text">SNMP 采集</h1>
          <p className="text-sm text-text-secondary mt-1">SNMP OID 采集管理，支持自定义 MIB、高频硬件筛选、定时采集和数据查看</p>
        </div>
        <button
          onClick={() => navigate('/metric-tags')}
          className="flex items-center gap-2 px-4 py-2 bg-blue-500/10 border border-blue-500/30 text-blue-400 hover:bg-blue-500/20 rounded-xl text-sm font-medium transition-all"
        >
          <Tag className="w-4 h-4" />
          指标 Tag 览 (Prometheus 规范)
        </button>
      </div>

      <div className="flex gap-1 mb-6 bg-surface rounded-lg border border-border p-1 w-fit">
        {[
          { key: 'mib' as const, label: 'MIB 模块', icon: FileText },
          { key: 'hardware' as const, label: '🔥 高频硬件指标', icon: Cpu },
          { key: 'tasks' as const, label: '采集任务', icon: Database },
          { key: 'data' as const, label: '采集数据', icon: Activity },
          { key: 'panels' as const, label: '指标面板', icon: BarChart3 },
        ].map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-all ${
              activeTab === tab.key
                ? 'bg-gradient-to-r from-blue-600 to-blue-700 text-white shadow-sm'
                : 'text-text-secondary hover:text-text hover:bg-surface-secondary'
            }`}
          >
            <tab.icon className="w-4 h-4" />
            {tab.label}
          </button>
        ))}
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto">
        {activeTab === 'mib' && <MibTab />}
        {activeTab === 'hardware' && <HardwareMetricsTab onOpenTest={(target) => { setTestTargetItem(target); setTestModalOpen(true); }} />}
        {activeTab === 'tasks' && <TasksTab />}
        {activeTab === 'data' && <DataTab />}
        {activeTab === 'panels' && <MetricPanelsTab />}
      </div>

      {testModalOpen && testTargetItem && (
        <CollectTestModal
          targetItem={testTargetItem}
          onClose={() => { setTestModalOpen(false); setTestTargetItem(null); }}
        />
      )}
    </div>
  );
}

// ============================================================
// MIB 模块 Tab
// ============================================================

function MibTab() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);
  const [expandedModuleId, setExpandedModuleId] = useState<string | null>(null);
  const [expandedGroupId, setExpandedGroupId] = useState<string | null>(null);
  const [oidSearch, setOidSearch] = useState('');
  const [searchType, setSearchType] = useState('all');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [uploading, setUploading] = useState(false);
  const [uploadResults, setUploadResults] = useState<any>(null);
  const [testModalOpen, setTestModalOpen] = useState(false);
  const [testTargetItem, setTestTargetItem] = useState<{ moduleId?: string; oid?: string; name?: string; is_table?: number } | null>(null);
  const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
  const [selectedModuleIds, setSelectedModuleIds] = useState<Set<string>>(new Set());
  const [confirmBatchDelete, setConfirmBatchDelete] = useState(false);
  const [confirmGroupDelete, setConfirmGroupDelete] = useState<string | null>(null);
  const [batchTestModalOpen, setBatchTestModalOpen] = useState(false);
  const [transferModalOpen, setTransferModalOpen] = useState(false);

  const { data: modules = [], isLoading } = useQuery({
    queryKey: ['snmp-mib-modules'],
    queryFn: () => api.get('/snmp-collect/mib-modules').then(res => res.data.data || []),
  });

  const { data: moduleObjects = [] } = useQuery({
    queryKey: ['snmp-mib-objects', expandedModuleId],
    queryFn: () => api.get(`/snmp-collect/mib-modules/${expandedModuleId}/objects`).then(res => res.data.data || []),
    enabled: !!expandedModuleId,
  });

  const { data: searchResults = [] } = useQuery({
    queryKey: ['snmp-mib-search', debouncedSearch, searchType],
    queryFn: () => api.get('/snmp-collect/mib-objects/search', { params: { q: debouncedSearch, type: searchType !== 'all' ? searchType : undefined } }).then(res => res.data.data || []),
    enabled: !!debouncedSearch,
  });

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedSearch(oidSearch), 300);
    return () => clearTimeout(timer);
  }, [oidSearch]);

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/snmp-collect/mib-modules/${id}`),
    onSuccess: () => {
      toast.success('MIB 模块已删除');
      if (expandedModuleId) setExpandedModuleId(null);
      queryClient.invalidateQueries({ queryKey: ['snmp-mib-modules'] });
    },
    onError: () => toast.error('删除 MIB 模块失败'),
  });

  const batchDeleteMutation = useMutation({
    mutationFn: (ids: string[]) => api.post('/snmp-collect/mib-modules/batch-delete', { ids }),
    onSuccess: (_data, ids) => {
      toast.success(`已删除 ${ids.length} 个 MIB 模块`);
      setSelectedModuleIds(new Set());
      setConfirmBatchDelete(false);
      if (expandedModuleId && ids.includes(expandedModuleId)) setExpandedModuleId(null);
      queryClient.invalidateQueries({ queryKey: ['snmp-mib-modules'] });
    },
    onError: () => toast.error('批量删除 MIB 模块失败'),
  });

  const toggleModuleSelect = (id: string) => {
    const s = new Set(selectedModuleIds);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedModuleIds(s);
  };

  const selectAllModules = () => {
    if (selectedModuleIds.size === modules.length) {
      setSelectedModuleIds(new Set());
    } else {
      setSelectedModuleIds(new Set(modules.map((m: any) => m.id)));
    }
  };

  // 按文件名分组
  const moduleGroups = useMemo(() => {
    const groupMap = new Map<string, any[]>();
    for (const m of modules) {
      const key = m.file_name || '__unknown__';
      if (!groupMap.has(key)) groupMap.set(key, []);
      groupMap.get(key)!.push(m);
    }
    return Array.from(groupMap.entries()).map(([fileName, mods]) => ({
      key: fileName,
      fileName,
      modules: mods,
      totalObjects: mods.reduce((sum: number, m: any) => sum + (m.object_count || 0), 0),
      uploadedAt: mods[0]?.uploaded_at || '',
    }));
  }, [modules]);

  // 分组内全选/取消
  const toggleGroupSelect = (groupKey: string) => {
    const group = moduleGroups.find(g => g.key === groupKey);
    if (!group) return;
    const groupIds = new Set(group.modules.map((m: any) => m.id));
    const allSelected = Array.from(groupIds).every(id => selectedModuleIds.has(id));
    const s = new Set(selectedModuleIds);
    if (allSelected) {
      for (const id of groupIds) s.delete(id);
    } else {
      for (const id of groupIds) s.add(id);
    }
    setSelectedModuleIds(s);
  };

  // 删除整个分组
  const deleteGroup = (groupKey: string) => {
    const group = moduleGroups.find(g => g.key === groupKey);
    if (!group) return;
    const ids = group.modules.map((m: any) => m.id);
    batchDeleteMutation.mutate(ids);
  };

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>, isFolder = false) => {
    const files = e.target.files;
    if (!files || files.length === 0) return;

    setUploading(true);
    setUploadResults(null);
    const formData = new FormData();

    for (let i = 0; i < files.length; i++) {
      formData.append('files', files[i]);
    }

    try {
      const resp = await api.post('/snmp-collect/mib-modules/upload', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      const data = resp.data.data;
      setUploadResults(data);
      queryClient.invalidateQueries({ queryKey: ['snmp-mib-modules'] });

      if (data.failCount === 0) {
        toast.success(`${data.successCount} 个文件解析成功`);
      } else if (data.successCount > 0) {
        toast.success(`${data.successCount} 个成功，${data.failCount} 个失败`);
      } else {
        toast.error('所有文件解析失败');
      }
    } catch (err: any) {
      toast.error(err.response?.data?.error || '上传失败');
    }
    setUploading(false);
    e.target.value = '';
  };

  const openCollectTest = (target: { moduleId?: string; oid?: string; name?: string; is_table?: number }) => {
    setTestTargetItem(target);
    setTestModalOpen(true);
  };

  return (
    <div className="space-y-4">
      {/* 上传区域 */}
      <div className="bg-surface rounded-xl border border-border p-4">
        <div className="flex items-center justify-between mb-3">
          <p className="text-sm text-text-secondary">上传 MIB 文件、Excel 或压缩包，自动解析 OID 对象</p>
          {modules.length > 0 && (
            <button
              onClick={() => setTransferModalOpen(true)}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-emerald-400 bg-emerald-500/10 hover:bg-emerald-500/20 border border-emerald-500/30 rounded-md transition-colors"
            >
              <ChevronRight className="w-3.5 h-3.5" />
              穿梭框选指标到大屏
            </button>
          )}
        </div>

        <div className="flex gap-2 flex-wrap">
          <button
            onClick={() => fileInputRef.current?.click()}
            disabled={uploading}
            className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all disabled:opacity-50"
          >
            {uploading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
            上传文件
          </button>
          <button
            onClick={() => folderInputRef.current?.click()}
            disabled={uploading}
            className="flex items-center gap-2 px-4 py-2 bg-surface border border-border text-text text-sm font-medium rounded-md hover:bg-surface-secondary transition-all disabled:opacity-50"
          >
            <FolderOpen className="w-4 h-4" />
            上传文件夹
          </button>
        </div>

        <div className="flex items-center gap-4 mt-2 text-xs text-text-secondary">
          <span className="flex items-center gap-1"><FileText className="w-3 h-3" />MIB (.mib .my .txt)</span>
          <span className="flex items-center gap-1"><FileSpreadsheet className="w-3 h-3" />Excel (.xlsx .xls .csv)</span>
          <span className="flex items-center gap-1"><FileArchive className="w-3 h-3" />压缩包 (.zip)</span>
        </div>

        <input
          ref={fileInputRef}
          type="file"
          accept=".mib,.mib2,.my,.txt,.xlsx,.xls,.csv,.zip"
          multiple
          className="hidden"
          onChange={e => handleUpload(e, false)}
        />
        <input
          ref={folderInputRef}
          type="file"
          accept=".mib,.mib2,.my,.txt,.xlsx,.xls,.csv,.zip"
          multiple
          className="hidden"
          {...({ webkitdirectory: '', directory: '' } as any)}
          onChange={e => handleUpload(e, true)}
        />
      </div>

      {/* 上传结果提示 */}
      {uploadResults && (
        <div className="bg-surface rounded-xl border border-border p-4">
          <div className="flex items-center justify-between mb-2">
            <h3 className="text-sm font-medium text-text">上传结果</h3>
            <button onClick={() => setUploadResults(null)} className="p-1 hover:bg-surface-secondary rounded-md">
              <X className="w-3.5 h-3.5 text-text-secondary" />
            </button>
          </div>
          <div className="space-y-2">
            {uploadResults.results?.map((r: any, i: number) => (
              <div key={i} className="flex items-center gap-2 text-sm">
                {r.error ? (
                  <><XCircle className="w-4 h-4 text-red-400 flex-shrink-0" /><span className="text-text">{r.fileName}</span><span className="text-red-400 text-xs">{r.error}</span></>
                ) : r.type === 'zip' ? (
                  <><FileArchive className="w-4 h-4 text-blue-400 flex-shrink-0" /><span className="text-text">{r.fileName}</span><span className="text-text-secondary text-xs">{r.successCount} 成功{r.failCount > 0 ? `, ${r.failCount} 失败` : ''}</span></>
                ) : r.type === 'excel' ? (
                  <><FileSpreadsheet className="w-4 h-4 text-emerald-400 flex-shrink-0" /><span className="text-text">{r.fileName}</span><span className="text-emerald-400 text-xs">{r.objectCount} 个 OID</span></>
                ) : (
                  <><FileText className="w-4 h-4 text-emerald-400 flex-shrink-0" /><span className="text-text">{r.fileName}</span><span className="text-emerald-400 text-xs">{r.objectCount} 个对象</span></>
                )}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* OID 搜索 */}
      <div className="bg-surface rounded-xl border border-border p-4">
        <div className="flex gap-2">
          <select
            value={searchType}
            onChange={e => setSearchType(e.target.value)}
            className="flex-shrink-0 px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500 cursor-pointer"
          >
            <option value="all">全部</option>
            <option value="oid_name">OID 名称</option>
            <option value="oid_number">OID 编号</option>
            <option value="module_name">模块名称</option>
            <option value="brand">品牌/厂商</option>
          </select>
          <div className="relative flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-secondary" />
            <input
              value={oidSearch}
              onChange={e => setOidSearch(e.target.value)}
              placeholder={
                searchType === 'oid_name' ? '搜索 OID 名称（如 sysUpTime、ifDescr）...'
                : searchType === 'oid_number' ? '搜索 OID 编号（如 1.3.6.1.2.1.1.3）...'
                : searchType === 'module_name' ? '搜索模块/MIB 名称（如 IF-MIB、Huawei）...'
                : searchType === 'brand' ? '搜索品牌/厂商（如 Huawei、Cisco）...'
                : '搜索 OID、模块名或品牌（如 sysUpTime、Huawei、IF-MIB）...'
              }
              className="w-full pl-9 pr-4 py-2 bg-surface border border-border rounded-lg text-sm text-text placeholder:text-text-secondary focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>
        {debouncedSearch && searchResults.length > 0 && (
          <div className="mt-3 max-h-64 overflow-y-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-border">
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">分类</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">OID</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">名称</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">模块</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">品牌</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">匹配</th>
                  <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">语法</th>
                </tr>
              </thead>
              <tbody>
                {searchResults.map((obj: any, i: number) => {
                  const matchType = obj.match_type || 'oid_name';
                  const matchLabel: Record<string, { text: string; cls: string }> = {
                    oid_name: { text: 'OID名', cls: 'bg-blue-500/15 text-blue-400 border-blue-500/30' },
                    oid_number: { text: 'OID号', cls: 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30' },
                    module_name: { text: '模块', cls: 'bg-violet-500/15 text-violet-400 border-violet-500/30' },
                    brand: { text: '品牌', cls: 'bg-amber-500/15 text-amber-400 border-amber-500/30' },
                  };
                  const badge = matchLabel[matchType] || matchLabel.oid_name;
                  const cls = obj.classification || {};
                  const isHighlighted = cls.category && cls.category !== 'other';
                  const catColors: Record<string, string> = {
                    system: '#3b82f6', optical: '#10b981', interface: '#0ea5e9',
                    cpu: '#3b82f6', memory: '#8b5cf6', power: '#f59e0b', fan: '#06b6d4', temperature: '#ef4444',
                    system_info: '#6366f1',
                    module_type: '#a855f7', module_status: '#84cc16', rx_power: '#10b981', tx_power: '#22c55e',
                    module_temp: '#f97316', wavelength: '#d946ef',
                    if_status: '#0ea5e9', if_traffic: '#14b8a6', if_error: '#dc2626', if_speed: '#7c3aed',
                  };
                  const catColor = catColors[cls.subcategory] || catColors[cls.category] || '#6b7280';
                  return (
                    <tr key={i} className="border-b border-border/30 hover:bg-surface-secondary/20 transition-colors"
                      style={isHighlighted ? { borderLeft: `3px solid ${catColor}` } : {}}>
                      <td className="px-3 py-2">
                        {isHighlighted ? (
                          <span
                            className="inline-block px-1.5 py-0.5 text-[10px] font-medium rounded"
                            style={{ color: catColor, backgroundColor: catColor + '15' }}
                          >
                            {cls.label}
                          </span>
                        ) : (
                          <span className="text-[10px] text-text-secondary">-</span>
                        )}
                      </td>
                      <td className="px-3 py-2 text-xs font-mono text-blue-400">{obj.oid}</td>
                      <td className={`px-3 py-2 text-sm ${isHighlighted ? 'font-medium text-text' : 'text-text'}`}>{obj.name}</td>
                      <td className="px-3 py-2 text-xs text-text-secondary">{obj.module_name || '-'}</td>
                      <td className="px-3 py-2 text-xs text-text-secondary">{obj.organization || '-'}</td>
                      <td className="px-3 py-2">
                        <span className={`inline-block px-1.5 py-0.5 text-[10px] font-medium rounded border ${badge.cls}`}>{badge.text}</span>
                      </td>
                      <td className="px-3 py-2 text-xs text-text-secondary">{obj.syntax || '-'}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        {debouncedSearch && searchResults.length === 0 && (
          <p className="mt-3 text-sm text-text-secondary text-center py-4">未找到匹配的 OID</p>
        )}
      </div>

      {/* MIB 模块分组列表 */}
      {isLoading ? (
        <div className="flex items-center justify-center py-20"><Loader2 className="w-6 h-6 animate-spin text-blue-500" /></div>
      ) : modules.length === 0 ? (
        <div className="text-center py-20 text-text-secondary">暂无 MIB 模块，点击"上传文件"开始</div>
      ) : (
        <>
          {/* 批量操作栏 */}
          <div className="bg-surface rounded-xl border border-border p-3 flex items-center justify-between">
            <div className="flex items-center gap-3">
              <button onClick={selectAllModules} className="flex items-center gap-1.5 text-sm text-text-secondary hover:text-text transition-colors">
                {selectedModuleIds.size === modules.length ? (
                  <CheckSquare className="w-4 h-4 text-blue-400" />
                ) : (
                  <Square className="w-4 h-4" />
                )}
                {selectedModuleIds.size === modules.length ? '取消全选' : '全选'}
              </button>
              {selectedModuleIds.size > 0 && (
                <span className="text-sm text-text-secondary">
                  已选择 <span className="text-blue-400 font-medium">{selectedModuleIds.size}</span> 个模块
                </span>
              )}
            </div>
            {selectedModuleIds.size > 0 && (
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setTransferModalOpen(true)}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-emerald-400 hover:bg-emerald-500/10 rounded-md transition-colors"
                >
                  <ChevronRight className="w-3.5 h-3.5" />
                  穿梭框选指标 ({selectedModuleIds.size})
                </button>
                <button
                  onClick={() => setBatchTestModalOpen(true)}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-blue-400 hover:bg-blue-500/10 rounded-md transition-colors"
                >
                  <Zap className="w-3.5 h-3.5" />
                  批量采集测试 ({selectedModuleIds.size})
                </button>
                <button
                  onClick={() => setConfirmBatchDelete(true)}
                  className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-red-400 hover:bg-red-500/10 rounded-md transition-colors"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                  批量删除 ({selectedModuleIds.size})
                </button>
              </div>
            )}
          </div>

          {/* 分组卡片 */}
          <div className="space-y-3">
            {moduleGroups.map(group => {
              const isExpanded = expandedGroupId === group.key;
              const groupIds = new Set(group.modules.map((m: any) => m.id));
              const allGroupSelected = group.modules.length > 0 && group.modules.every((m: any) => selectedModuleIds.has(m.id));
              const someGroupSelected = group.modules.some((m: any) => selectedModuleIds.has(m.id));
              const isExcel = group.fileName.endsWith('.xlsx') || group.fileName.endsWith('.xls') || group.fileName.endsWith('.csv');
              const isZip = group.fileName.endsWith('.zip');
              const FileIcon = isExcel ? FileSpreadsheet : isZip ? FileArchive : FileText;

              return (
                <div key={group.key} className={`bg-surface rounded-xl border transition-all ${
                  someGroupSelected ? 'border-blue-500/50 ring-1 ring-blue-500/20' : 'border-border hover:border-blue-500/30'
                }`}>
                  {/* 分组头部 */}
                  <div className="p-4">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3 min-w-0 flex-1">
                        <button onClick={() => toggleGroupSelect(group.key)} className="flex-shrink-0">
                          {allGroupSelected ? (
                            <CheckSquare className="w-4 h-4 text-blue-400" />
                          ) : someGroupSelected ? (
                            <CheckSquare className="w-4 h-4 text-blue-300 opacity-60" />
                          ) : (
                            <Square className="w-4 h-4 text-text-secondary hover:text-blue-400 transition-colors" />
                          )}
                        </button>
                        <button
                          onClick={() => setExpandedGroupId(isExpanded ? null : group.key)}
                          className="flex items-center gap-2 min-w-0 flex-1 text-left"
                        >
                          {isExpanded ? (
                            <ChevronDown className="w-4 h-4 text-blue-400 flex-shrink-0" />
                          ) : (
                            <ChevronRight className="w-4 h-4 text-text-secondary flex-shrink-0" />
                          )}
                          <FileIcon className={`w-4 h-4 flex-shrink-0 ${isExcel ? 'text-emerald-400' : isZip ? 'text-blue-400' : 'text-text-secondary'}`} />
                          <span className="text-sm font-medium text-text truncate">{group.fileName}</span>
                        </button>
                      </div>

                      <div className="flex items-center gap-4 ml-4 flex-shrink-0">
                        <div className="flex items-center gap-4 text-xs text-text-secondary">
                          <span className="flex items-center gap-1"><Database className="w-3 h-3" />{group.modules.length} 模块</span>
                          <span className="flex items-center gap-1"><Zap className="w-3 h-3" />{group.totalObjects} OID</span>
                          <span className="flex items-center gap-1"><Clock className="w-3 h-3" />{group.uploadedAt ? new Date(group.uploadedAt).toLocaleString() : '-'}</span>
                        </div>
                        <div className="flex items-center gap-1">
                          <button
                            onClick={() => setConfirmGroupDelete(group.key)}
                            className="p-1.5 text-text-secondary hover:text-red-400 hover:bg-red-500/10 rounded-md transition-colors"
                            title="删除整个文件的所有模块"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>

                  {/* 展开的子模块列表 */}
                  {isExpanded && (
                    <div className="border-t border-border">
                      <div className="max-h-96 overflow-y-auto">
                        <table className="w-full">
                          <thead className="sticky top-0 bg-surface z-10">
                            <tr className="border-b border-border">
                              <th className="text-left text-xs font-medium text-text-secondary px-3 py-2 w-8"></th>
                              <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">模块名称</th>
                              <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">描述</th>
                              <th className="text-left text-xs font-medium text-text-secondary px-3 py-2 w-20">OID 数</th>
                              <th className="text-left text-xs font-medium text-text-secondary px-3 py-2 w-24">操作</th>
                            </tr>
                          </thead>
                          <tbody>
                            {group.modules.map((m: any) => (
                              <tr key={m.id} className={`border-b border-border/30 transition-colors ${
                                selectedModuleIds.has(m.id) ? 'bg-blue-500/5' : 'hover:bg-surface-secondary/20'
                              }`}>
                                <td className="px-3 py-2">
                                  <button onClick={() => toggleModuleSelect(m.id)}>
                                    {selectedModuleIds.has(m.id) ? (
                                      <CheckSquare className="w-3.5 h-3.5 text-blue-400" />
                                    ) : (
                                      <Square className="w-3.5 h-3.5 text-text-secondary hover:text-blue-400 transition-colors" />
                                    )}
                                  </button>
                                </td>
                                <td className="px-3 py-2">
                                  <span
                                    className="text-sm text-text cursor-pointer hover:text-blue-400 transition-colors"
                                    onClick={() => setExpandedModuleId(expandedModuleId === m.id ? null : m.id)}
                                  >
                                    {m.name}
                                  </span>
                                </td>
                                <td className="px-3 py-2">
                                  <span className="text-xs text-text-secondary line-clamp-1">{m.description || '-'}</span>
                                </td>
                                <td className="px-3 py-2">
                                  <span className="text-xs text-text-secondary">{m.object_count || 0}</span>
                                </td>
                                <td className="px-3 py-2">
                                  <div className="flex items-center gap-1">
                                    <button
                                      onClick={() => openCollectTest(m.id)}
                                      className="p-1 text-blue-400 hover:bg-blue-500/10 rounded transition-colors"
                                      title="采集测试"
                                    >
                                      <Zap className="w-3 h-3" />
                                    </button>
                                    <button
                                      onClick={() => setConfirmDeleteId(m.id)}
                                      className="p-1 text-text-secondary hover:text-red-400 hover:bg-red-500/10 rounded transition-colors"
                                      title="删除"
                                    >
                                      <Trash2 className="w-3 h-3" />
                                    </button>
                                  </div>
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    </div>
                  )}

                  {/* 单个模块的 OID 详情展开 - 显示在分组外部 */}
                </div>
              );
            })}
          </div>

          {/* OID 对象详情面板 */}
          {expandedModuleId && (
            <div className="bg-surface rounded-xl border border-blue-500/30">
              <div className="flex items-center justify-between px-4 py-3 border-b border-border">
                <h3 className="text-sm font-medium text-text flex items-center gap-2">
                  <Database className="w-4 h-4 text-blue-400" />
                  {(modules as any[]).find((m: any) => m.id === expandedModuleId)?.name || ''} - OID 对象
                </h3>
                <button 
                  onClick={() => setExpandedModuleId(null)}
                  className="p-1 hover:bg-surface-secondary rounded-md"
                >
                  <X className="w-4 h-4 text-text-secondary" />
                </button>
              </div>
              <div className="max-h-80 overflow-y-auto">
                {moduleObjects.length === 0 ? (
                  <p className="text-xs text-text-secondary text-center py-8">暂无 OID 对象</p>
                ) : (
                  <table className="w-full">
                    <thead>
                      <tr className="border-b border-border">
                        <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">高亮标签</th>
                        <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">OID</th>
                        <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">名称</th>
                        <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">中文描述 / 说明</th>
                        <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">语法</th>
                      </tr>
                    </thead>
                    <tbody>
                      {moduleObjects.map((obj: any, i: number) => {
                        const tags = getHardwareTags(obj.name, obj.description, obj.syntax);
                        const isHighlighted = tags.length > 0;
                        return (
                          <tr key={i} className={`border-b border-border/30 transition-colors ${
                            isHighlighted ? 'bg-blue-500/5 hover:bg-blue-500/10' : 'hover:bg-surface-secondary/20'
                          }`}>
                            <td className="px-3 py-1.5">
                              <div className="flex flex-wrap gap-1">
                                {tags.map(t => (
                                  <span key={t.key} className={`inline-flex items-center gap-0.5 px-1.5 py-0.5 text-[10px] font-semibold rounded border ${t.bg} ${t.text} ${t.border}`}>
                                    <t.icon className="w-2.5 h-2.5" />
                                    {t.label}
                                  </span>
                                ))}
                                {tags.length === 0 && <span className="text-[10px] text-text-secondary">-</span>}
                              </div>
                            </td>
                            <td className="px-3 py-1.5 text-xs font-mono text-blue-400">{obj.oid}</td>
                            <td className={`px-3 py-1.5 text-xs ${isHighlighted ? 'font-semibold text-text' : 'text-text'}`}>{obj.name}</td>
                            <td className="px-3 py-1.5 text-xs text-text-secondary max-w-xs truncate" title={obj.description}>{obj.description || '-'}</td>
                            <td className="px-3 py-1.5 text-xs text-text-secondary">{obj.syntax || '-'}</td>
                          </tr>
                        );
                      })}
                    </tbody>
                  </table>
                )}
              </div>
            </div>
          )}
        </>
      )}

      {/* 删除确认弹窗 */}
      {confirmDeleteId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={() => setConfirmDeleteId(null)}>
          <div className="bg-surface border border-border rounded-xl w-full max-w-sm p-6 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-text mb-2">确认删除</h3>
            <p className="text-sm text-text-secondary mb-4">确定删除此 MIB 模块？关联的 OID 对象也将被删除。</p>
            <div className="flex justify-end gap-3">
              <button onClick={() => setConfirmDeleteId(null)} className="px-4 py-2 text-sm text-text-secondary hover:text-text transition-colors">取消</button>
              <button onClick={() => { deleteMutation.mutate(confirmDeleteId); setConfirmDeleteId(null); }}
                className="px-4 py-2 bg-red-500 text-white text-sm font-medium rounded-md hover:bg-red-600 transition-colors">删除</button>
            </div>
          </div>
        </div>
      )}

      {/* 批量删除确认弹窗 */}
      {confirmBatchDelete && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={() => setConfirmBatchDelete(false)}>
          <div className="bg-surface border border-border rounded-xl w-full max-w-sm p-6 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-text mb-2">确认批量删除</h3>
            <p className="text-sm text-text-secondary mb-4">
              确定删除选中的 <span className="text-red-400 font-medium">{selectedModuleIds.size}</span> 个 MIB 模块？关联的 OID 对象也将被删除，此操作不可撤销。
            </p>
            <div className="flex justify-end gap-3">
              <button onClick={() => setConfirmBatchDelete(false)} className="px-4 py-2 text-sm text-text-secondary hover:text-text transition-colors">取消</button>
              <button
                onClick={() => batchDeleteMutation.mutate(Array.from(selectedModuleIds))}
                disabled={batchDeleteMutation.isPending}
                className="flex items-center gap-2 px-4 py-2 bg-red-500 text-white text-sm font-medium rounded-md hover:bg-red-600 transition-colors disabled:opacity-50"
              >
                {batchDeleteMutation.isPending && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
                确认删除
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 分组删除确认弹窗 */}
      {confirmGroupDelete && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={() => setConfirmGroupDelete(null)}>
          <div className="bg-surface border border-border rounded-xl w-full max-w-sm p-6 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-text mb-2">确认删除文件</h3>
            <p className="text-sm text-text-secondary mb-4">
              确定删除文件 <span className="text-text font-medium">{confirmGroupDelete}</span> 下的所有 MIB 模块？
              {(() => {
                const g = moduleGroups.find(g => g.key === confirmGroupDelete);
                return g ? `共 ${g.modules.length} 个模块、${g.totalObjects} 个 OID，` : '';
              })()}
              此操作不可撤销。
            </p>
            <div className="flex justify-end gap-3">
              <button onClick={() => setConfirmGroupDelete(null)} className="px-4 py-2 text-sm text-text-secondary hover:text-text transition-colors">取消</button>
              <button
                onClick={() => { deleteGroup(confirmGroupDelete); setConfirmGroupDelete(null); }}
                disabled={batchDeleteMutation.isPending}
                className="flex items-center gap-2 px-4 py-2 bg-red-500 text-white text-sm font-medium rounded-md hover:bg-red-600 transition-colors disabled:opacity-50"
              >
                {batchDeleteMutation.isPending && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
                确认删除
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 采集测试弹窗 */}
      {testModalOpen && testTargetItem && (
        <CollectTestModal
          targetItem={testTargetItem}
          onClose={() => { setTestModalOpen(false); setTestTargetItem(null); }}
        />
      )}

      {/* 批量采集测试弹窗 */}
      {batchTestModalOpen && (
        <BatchCollectTestModal
          moduleIds={Array.from(selectedModuleIds)}
          modules={modules}
          onClose={() => setBatchTestModalOpen(false)}
        />
      )}

      {/* OID 穿梭框弹窗 */}
      {transferModalOpen && (
        <MibTransferModal
          preselectedModuleIds={selectedModuleIds.size > 0 ? Array.from(selectedModuleIds) : undefined}
          onClose={() => setTransferModalOpen(false)}
        />
      )}
    </div>
  );
}

// ============================================================
// 采集测试弹窗
// ============================================================

function CollectTestModal({
  targetItem,
  onClose,
}: {
  targetItem: { moduleId?: string; oid?: string; name?: string; is_table?: number };
  onClose: () => void;
}) {
  const toast = useToast();
  const isSingleOid = Boolean(targetItem.oid);

  // 如果传了 moduleId 且非单节点 OID，获取该模块关联的对象
  const { data: objects = [] } = useQuery({
    queryKey: ['snmp-mib-objects', targetItem.moduleId],
    queryFn: () => targetItem.moduleId ? api.get(`/snmp-collect/mib-modules/${targetItem.moduleId}/objects`).then(res => res.data.data || []) : Promise.resolve([]),
    enabled: Boolean(targetItem.moduleId && !isSingleOid),
  });

  const { data: networkDevices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
  });

  const { data: savedCredentials = [] } = useQuery({
    queryKey: ['snmp-credentials'],
    queryFn: () => api.get('/snmp/credentials').then(res => res.data.data || []),
  });

  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(new Set());
  const [selectedOids, setSelectedOids] = useState<Set<string>>(new Set());
  const [snmpVersion, setSnmpVersion] = useState('v2c');
  const [community, setCommunity] = useState('public');
  const [snmpUser, setSnmpUser] = useState('');
  const [snmpAuthProtocol, setSnmpAuthProtocol] = useState('SHA');
  const [snmpAuthKey, setSnmpAuthKey] = useState('');
  const [snmpPrivProtocol, setSnmpPrivProtocol] = useState('AES');
  const [snmpPrivKey, setSnmpPrivKey] = useState('');
  const [snmpCredentialId, setSnmpCredentialId] = useState('');
  const [showAuthKey, setShowAuthKey] = useState(false);
  const [showPrivKey, setShowPrivKey] = useState(false);
  const [snmpTimeout, setSnmpTimeout] = useState(10000);
  const [testing, setTesting] = useState(false);
  const [testResults, setTestResults] = useState<any[] | null>(null);
  const [sendToDashboardData, setSendToDashboardData] = useState<any | null>(null);

  useEscapeKey({ onEscape: onClose });

  const validObjects = useMemo(() => objects.filter((o: any) => o.oid && !o.oid.startsWith('unresolved')), [objects]);

  // 当为模块级多 OID 测试且数据加载完成时，默认全选
  useEffect(() => {
    if (!isSingleOid && validObjects.length > 0) {
      setSelectedOids(new Set(validObjects.map((o: any) => o.id)));
    }
  }, [isSingleOid, validObjects]);

  const toggleDevice = (id: string) => {
    const s = new Set(selectedDeviceIds);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedDeviceIds(s);
  };

  const toggleOid = (id: string) => {
    const s = new Set(selectedOids);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedOids(s);
  };

  const selectAllDevices = () => {
    if (selectedDeviceIds.size === networkDevices.length) {
      setSelectedDeviceIds(new Set());
    } else {
      setSelectedDeviceIds(new Set(networkDevices.map((d: any) => d.id)));
    }
  };

  const selectAllOids = () => {
    if (selectedOids.size === validObjects.length) {
      setSelectedOids(new Set());
    } else {
      setSelectedOids(new Set(validObjects.map((o: any) => o.id)));
    }
  };

  const handleTest = async () => {
    if (selectedDeviceIds.size === 0) { toast.error('请选择至少一台设备'); return; }

    let oidsToTest: any[] = [];
    if (isSingleOid) {
      oidsToTest = [{
        oid: targetItem.oid!,
        metric_name: targetItem.name || targetItem.oid!,
        is_table: targetItem.is_table || 0,
      }];
    } else {
      if (selectedOids.size === 0) { toast.error('请选择至少一个 OID'); return; }
      const seen = new Set<string>();
      oidsToTest = validObjects
        .filter((o: any) => selectedOids.has(o.id))
        .filter((o: any) => {
          if (seen.has(o.oid)) return false;
          seen.add(o.oid);
          return true;
        })
        .map((o: any) => ({ oid: o.oid, metric_name: o.name, is_table: o.is_table || 0 }));
    }

    setTesting(true);
    setTestResults(null);

    try {
      const resp = await api.post('/snmp-collect/quick-test', {
        device_ids: Array.from(selectedDeviceIds),
        oids: oidsToTest,
        snmp_version: snmpVersion,
        community: snmpVersion !== 'v3' ? community : undefined,
        snmp_user: snmpVersion === 'v3' ? snmpUser : undefined,
        snmp_auth_protocol: snmpVersion === 'v3' ? snmpAuthProtocol : undefined,
        snmp_auth_key: snmpVersion === 'v3' ? snmpAuthKey : undefined,
        snmp_priv_protocol: snmpVersion === 'v3' ? snmpPrivProtocol : undefined,
        snmp_priv_key: snmpVersion === 'v3' ? snmpPrivKey : undefined,
        snmp_credential_id: snmpCredentialId || undefined,
        timeout_ms: snmpTimeout,
      });
      setTestResults(resp.data.data || []);
    } catch (err: any) {
      toast.error(err.response?.data?.error || '采集测试失败');
    }
    setTesting(false);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-3xl max-h-[85vh] overflow-y-auto p-6 shadow-2xl shadow-black/40" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4 pb-3 border-b border-border">
          <h2 className="text-lg font-bold text-text flex items-center gap-2">
            <Zap className="w-5 h-5 text-blue-400" />
            {isSingleOid ? `单点 OID 采集测试` : `模块采集测试`}
          </h2>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-secondary rounded-md text-text-secondary hover:text-text transition-colors"><X className="w-4 h-4" /></button>
        </div>

        {/* 预选目标单 OID 信息 Banner */}
        {isSingleOid ? (
          <div className="mb-4 p-3.5 rounded-xl bg-blue-500/10 border border-blue-500/30 flex items-center justify-between">
            <div>
              <div className="flex items-center gap-2">
                <span className="text-xs font-semibold text-blue-400">已自动选中 OID:</span>
                <span className="text-sm font-bold text-text">{targetItem.name || '未命名节点'}</span>
              </div>
              <p className="text-xs font-mono text-cyan-300 mt-0.5">{targetItem.oid}</p>
            </div>
            <span className="text-xs px-3 py-1 rounded-full bg-blue-500/20 text-blue-300 font-semibold flex items-center gap-1 border border-blue-500/30">
              <Check className="w-3.5 h-3.5 text-emerald-400" /> 免选 OID 模式
            </span>
          </div>
        ) : null}

        <div className="space-y-5">
          {/* 设备选择 */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-semibold text-text">1. 选择测试设备</label>
              <button onClick={selectAllDevices} className="text-xs text-blue-400 hover:text-blue-300 font-medium">
                {selectedDeviceIds.size === networkDevices.length ? '取消全选' : '全选设备'}
              </button>
            </div>
            <div className="max-h-48 overflow-y-auto border border-border rounded-lg">
              {networkDevices.length === 0 ? (
                <p className="text-center py-4 text-sm text-text-secondary">暂无网络设备</p>
              ) : networkDevices.map((d: any) => (
                <div key={d.id}
                  onClick={() => toggleDevice(d.id)}
                  className={`flex items-center gap-2.5 px-3.5 py-2.5 cursor-pointer transition-colors border-b border-border/30 last:border-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500/10' : 'hover:bg-surface-secondary/50'
                  }`}
                >
                  <div className={`w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500 border-blue-500' : 'border-border'
                  }`}>
                    {selectedDeviceIds.has(d.id) && <CheckCircle2 className="w-3 h-3 text-white" />}
                  </div>
                  <Monitor className="w-4 h-4 text-text-secondary flex-shrink-0" />
                  <span className="text-sm font-medium text-text truncate">{d.name}</span>
                  <span className="text-xs text-cyan-300 font-mono font-semibold ml-auto bg-cyan-500/10 px-2 py-0.5 rounded border border-cyan-500/20">{d.ip_address}</span>
                </div>
              ))}
            </div>
          </div>

          {/* 模块多 OID 列表（仅在非单点 OID 测试时展示） */}
          {!isSingleOid && (
            <div>
              <div className="flex items-center justify-between mb-2">
                <label className="text-sm font-semibold text-text">2. 模块下的 OID 列表</label>
                <button onClick={selectAllOids} className="text-xs text-blue-400 hover:text-blue-300 font-medium">
                  {selectedOids.size === validObjects.length ? '取消全选' : '全选 OID'}
                </button>
              </div>
              <div className="max-h-40 overflow-y-auto border border-border rounded-lg">
                {validObjects.length === 0 ? (
                  <p className="text-center py-4 text-sm text-text-secondary">无可用 OID</p>
                ) : validObjects.map((o: any) => (
                  <div key={o.id}
                    onClick={() => toggleOid(o.id)}
                    className={`flex items-center gap-2 px-3 py-1.5 cursor-pointer transition-colors border-b border-border/30 last:border-0 ${
                      selectedOids.has(o.id) ? 'bg-blue-500/10' : 'hover:bg-surface-secondary/50'
                    }`}
                  >
                    <div className={`w-3.5 h-3.5 rounded border flex items-center justify-center flex-shrink-0 ${
                      selectedOids.has(o.id) ? 'bg-blue-500 border-blue-500' : 'border-border'
                    }`}>
                      {selectedOids.has(o.id) && <CheckCircle2 className="w-2.5 h-2.5 text-white" />}
                    </div>
                    <span className="text-xs font-mono text-blue-400 flex-shrink-0">{o.oid}</span>
                    <span className="text-sm text-text truncate">{o.name || '-'}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* SNMP 参数设置 */}
          <div className="pt-2 border-t border-border/40">
            <label className="text-sm font-semibold text-text block mb-2">{isSingleOid ? '2. SNMP 连接参数' : '3. SNMP 连接参数'}</label>

            <div className="space-y-3.5 bg-surface-secondary/40 p-4 rounded-xl border border-border/60">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">SNMP 版本</label>
                  <select
                    value={snmpVersion}
                    onChange={e => setSnmpVersion(e.target.value)}
                    className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500 cursor-pointer"
                  >
                    {SNMP_VERSION_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-text-secondary mb-1">响应超时时间 (毫秒)</label>
                  <input
                    type="number"
                    value={snmpTimeout}
                    onChange={e => setSnmpTimeout(parseInt(e.target.value) || 10000)}
                    className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text font-mono focus:outline-none focus:border-blue-500"
                  />
                </div>
              </div>

              {savedCredentials.length > 0 && (
                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label className="block text-xs font-semibold text-text-secondary">快速套用凭据库</label>
                    <span className="text-[11px] text-blue-400 font-medium flex items-center gap-1">
                      <Key className="w-3 h-3 text-amber-400" /> 已连通 SNMP 密码库
                    </span>
                  </div>
                  <select
                    value={snmpCredentialId}
                    onChange={e => {
                      const cid = e.target.value;
                      setSnmpCredentialId(cid);
                      const c = savedCredentials.find((x: any) => x.id === cid);
                      if (c) {
                        if (c.snmp_version === 'v3') {
                          setSnmpVersion('v3');
                          setSnmpUser(c.snmp_user || '');
                          setSnmpAuthProtocol(c.snmp_auth_protocol || 'SHA');
                          setSnmpPrivProtocol(c.snmp_priv_protocol || 'AES');
                        } else {
                          setSnmpVersion(c.snmp_version || 'v2c');
                          setCommunity(c.community || 'public');
                        }
                      }
                    }}
                    className="w-full px-3 py-2 bg-background border border-blue-500/30 hover:border-blue-400 rounded-lg text-xs text-blue-400 font-semibold focus:outline-none cursor-pointer truncate shadow-sm transition-colors"
                  >
                    <option value="">🔑 选择已保存 SNMP 凭证 / 只读串...</option>
                    {savedCredentials.map((c: any) => (
                      <option key={c.id} value={c.id}>
                        {c.name} ({c.snmp_version === 'v3' ? `v3: ${c.snmp_user || '用户'}` : (c.community || 'public')})
                      </option>
                    ))}
                  </select>
                </div>
              )}

              {snmpVersion !== 'v3' ? (
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1.5">
                    SNMP 只读串密码 (Community) <span className="text-red-400">*</span>
                  </label>
                  <input
                    value={community}
                    onChange={e => setCommunity(e.target.value)}
                    placeholder="public"
                    className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-cyan-300 font-mono font-bold focus:outline-none focus:border-blue-500 shadow-inner"
                  />
                </div>
              ) : (
                <div className="bg-[#090d1a] border border-blue-500/20 rounded-xl p-3.5 space-y-3">
                  <div className="flex items-center gap-2 pb-1.5 border-b border-blue-500/15">
                    <Shield className="w-4 h-4 text-blue-400" />
                    <span className="text-xs font-bold text-blue-300">SNMP v3 安全参数 (USM)</span>
                  </div>

                  <div>
                    <label className="block text-xs font-semibold text-text-secondary mb-1">
                      安全用户名 (Username) <span className="text-red-400">*</span>
                    </label>
                    <input
                      type="text"
                      value={snmpUser}
                      onChange={e => setSnmpUser(e.target.value)}
                      placeholder="例如: snmpuser, admin_v3"
                      className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-cyan-300 focus:outline-none focus:border-blue-500"
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-xs font-semibold text-text-secondary mb-1">认证协议 (Auth)</label>
                      <select
                        value={snmpAuthProtocol}
                        onChange={e => setSnmpAuthProtocol(e.target.value)}
                        className="w-full px-2.5 py-1.5 bg-surface border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
                      >
                        <option value="">无 (NoAuth)</option>
                        <option value="SHA">SHA / SHA1</option>
                        <option value="SHA256">SHA-256 (推荐)</option>
                        <option value="SHA512">SHA-512</option>
                        <option value="MD5">MD5</option>
                      </select>
                    </div>
                    <div>
                      <label className="block text-xs font-semibold text-text-secondary mb-1">认证密码 (Auth Key)</label>
                      <div className="relative">
                        <input
                          type={showAuthKey ? 'text' : 'password'}
                          value={snmpAuthKey}
                          onChange={e => setSnmpAuthKey(e.target.value)}
                          placeholder="认证密钥密码"
                          className="w-full pl-2.5 pr-7 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-text focus:outline-none focus:border-blue-500"
                        />
                        <button
                          type="button"
                          onClick={() => setShowAuthKey(!showAuthKey)}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text"
                        >
                          {showAuthKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                        </button>
                      </div>
                    </div>
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-xs font-semibold text-text-secondary mb-1">加密协议 (Privacy)</label>
                      <select
                        value={snmpPrivProtocol}
                        onChange={e => setSnmpPrivProtocol(e.target.value)}
                        className="w-full px-2.5 py-1.5 bg-surface border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
                      >
                        <option value="">无 (NoPriv)</option>
                        <option value="AES">AES / AES128 (推荐)</option>
                        <option value="AES192">AES-192</option>
                        <option value="AES256">AES-256</option>
                        <option value="DES">DES</option>
                      </select>
                    </div>
                    <div>
                      <label className="block text-xs font-semibold text-text-secondary mb-1">加密密码 (Priv Key)</label>
                      <div className="relative">
                        <input
                          type={showPrivKey ? 'text' : 'password'}
                          value={snmpPrivKey}
                          onChange={e => setSnmpPrivKey(e.target.value)}
                          placeholder="数据隐私加密密码"
                          className="w-full pl-2.5 pr-7 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-text focus:outline-none focus:border-blue-500"
                        />
                        <button
                          type="button"
                          onClick={() => setShowPrivKey(!showPrivKey)}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text"
                        >
                          {showPrivKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* 触发测试按钮 */}
        <div className="mt-6 flex justify-end gap-3 border-t border-border pt-4">
          <button onClick={onClose} className="px-4 py-2 text-sm text-text-secondary hover:text-text transition-colors">
            取消
          </button>
          <button
            onClick={handleTest}
            disabled={testing}
            className="flex items-center gap-1.5 px-5 py-2 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-700 hover:to-blue-800 text-white text-sm font-semibold rounded-lg transition-all shadow-md disabled:opacity-50"
          >
            {testing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Zap className="w-4 h-4" />}
            {testing ? '正在测试...' : '立即测试'}
          </button>
        </div>

        {/* 测试结果 */}
        {testResults && (
          <div className="mt-6 space-y-3 border-t border-border pt-4">
            <h3 className="text-sm font-semibold text-text flex items-center justify-between">
              <span>测试结果 ({testResults.length} 条)</span>
              <span className="text-xs font-normal text-text-secondary">SNMP GET / WALK 响应结果</span>
            </h3>
            <div className="max-h-60 overflow-y-auto space-y-2">
              {testResults.map((r: any, idx: number) => (
                <div key={idx} className={`p-3 rounded-lg border text-xs font-mono ${
                  r.status === 'success' ? 'bg-emerald-500/5 border-emerald-500/30' : 'bg-red-500/5 border-red-500/30'
                }`}>
                  <div className="flex items-center justify-between font-sans mb-1.5">
                    <span className="font-semibold text-text">{r.deviceName} ({r.deviceIp})</span>
                    <div className="flex items-center gap-2">
                      <span className="text-[11px] text-text-secondary">耗时 {r.durationMs}ms</span>
                      <span className={`px-2 py-0.5 rounded text-[10px] font-semibold ${
                        r.status === 'success' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-red-500/20 text-red-400'
                      }`}>
                        {r.status === 'success' ? '成功' : '失败'}
                      </span>
                    </div>
                  </div>

                  {r.metrics?.length > 0 && (
                    <div className="space-y-1 mt-1">
                      {r.metrics.map((m: any, j: number) => (
                        <div key={j} className="flex items-center justify-between bg-surface-secondary/40 p-2 rounded border border-border/40">
                          <span className="text-blue-400 font-semibold">{m.metric_name || m.oid}:</span>
                          {m.error ? (
                            <span className="text-red-400">{m.error}</span>
                          ) : m.metric_value && m.metric_value.includes('; ') ? (
                            <div className="flex flex-col gap-1 w-full mt-1.5 font-mono text-xs">
                              {m.metric_value.split('; ').map((part: string, pIdx: number) => {
                                const eqIdx = part.indexOf('=');
                                const subIdx = eqIdx > -1 ? part.substring(0, eqIdx) : '';
                                const subVal = eqIdx > -1 ? part.substring(eqIdx + 1) : part;
                                return (
                                  <div key={pIdx} className="flex items-center justify-between px-2.5 py-1.5 rounded bg-surface/60 border border-border/40 hover:border-blue-500/30 transition-colors">
                                    <span className="text-text-secondary">实例 ID <code className="text-blue-400 font-semibold">{subIdx}</code></span>
                                    <div className="flex items-center gap-2.5">
                                      <span className="text-emerald-400 font-semibold font-mono">{subVal}</span>
                                      <button
                                        onClick={() => setSendToDashboardData({
                                          oid: subIdx,
                                          customName: targetItem?.name ? `${targetItem.name}` : `实例 ${subIdx}`,
                                          deviceId: r.deviceId,
                                          deviceName: r.deviceName,
                                          deviceIp: r.deviceIp,
                                          lastValue: subVal,
                                          snmpVersion,
                                          community,
                                          is_table: 1,
                                        })}
                                        className="flex items-center gap-1 px-2 py-0.5 text-[11px] font-semibold text-blue-400 bg-blue-500/10 hover:bg-blue-500/20 border border-blue-500/30 rounded transition-all shadow-sm"
                                      >
                                        <Monitor className="w-3 h-3" />
                                        发送到监控大屏
                                      </button>
                                    </div>
                                  </div>
                                );
                              })}
                            </div>
                          ) : (
                            <div className="flex items-center gap-2.5">
                              <span className="text-emerald-400 font-semibold font-mono">{m.metric_value ?? '-'}</span>
                              {m.metric_value !== null && m.metric_value !== undefined && m.metric_value !== 'N/A' && !m.error && (
                                <button
                                  onClick={() => setSendToDashboardData({
                                    oid: m.oid || targetItem?.oid,
                                    customName: m.metric_name || targetItem?.name || m.oid,
                                    deviceId: r.deviceId,
                                    deviceName: r.deviceName,
                                    deviceIp: r.deviceIp,
                                    lastValue: String(m.metric_value),
                                    snmpVersion,
                                    community,
                                    is_table: m.is_table || 0,
                                  })}
                                  className="flex items-center gap-1 px-2 py-0.5 text-[11px] font-semibold text-blue-400 bg-blue-500/10 hover:bg-blue-500/20 border border-blue-500/30 rounded transition-all shadow-sm"
                                >
                                  <Monitor className="w-3 h-3" />
                                  发送到监控大屏
                                </button>
                              )}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* 发送到监控大屏弹窗 */}
        {sendToDashboardData && (
          <SendToDashboardModal
            initialData={sendToDashboardData}
            onClose={() => setSendToDashboardData(null)}
          />
        )}
      </div>
    </div>
  );
}

// ============================================================
// 批量采集测试弹窗（支持多模块 OID 一次性测试）
// ============================================================

function BatchCollectTestModal({ moduleIds, modules, onClose }: { moduleIds: string[]; modules: any[]; onClose: () => void }) {
  const queryClient = useQueryClient();
  const toast = useToast();

  // 批量获取选中模块的所有 OID
  const { data: allObjects = [], isLoading: objectsLoading } = useQuery({
    queryKey: ['snmp-mib-batch-objects', moduleIds.sort().join(',')],
    queryFn: () => api.post('/snmp-collect/mib-modules/batch-objects', { module_ids: moduleIds }).then(res => res.data.data || []),
  });

  const { data: networkDevices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
  });

  const { data: savedCredentials = [] } = useQuery({
    queryKey: ['snmp-credentials'],
    queryFn: () => api.get('/snmp/credentials').then(res => res.data.data || []),
  });

  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(new Set());
  const [snmpVersion, setSnmpVersion] = useState('v2c');
  const [community, setCommunity] = useState('public');
  const [snmpUser, setSnmpUser] = useState('');
  const [snmpAuthProtocol, setSnmpAuthProtocol] = useState('SHA');
  const [snmpAuthKey, setSnmpAuthKey] = useState('');
  const [snmpPrivProtocol, setSnmpPrivProtocol] = useState('AES');
  const [snmpPrivKey, setSnmpPrivKey] = useState('');
  const [snmpCredentialId, setSnmpCredentialId] = useState('');
  const [showAuthKey, setShowAuthKey] = useState(false);
  const [showPrivKey, setShowPrivKey] = useState(false);
  const [snmpTimeout, setSnmpTimeout] = useState(15000);
  const [testing, setTesting] = useState(false);
  const [testResults, setTestResults] = useState<any[] | null>(null);
  const [testProgress, setTestProgress] = useState<{ current: number; total: number } | null>(null);
  const [savingMetrics, setSavingMetrics] = useState<Set<string>>(new Set());
  const [editingMetricName, setEditingMetricName] = useState<{ oid: string; name: string; deviceId: string; deviceName: string; is_table: number } | null>(null);
  const [sendToDashboardData, setSendToDashboardData] = useState<any | null>(null);

  useEscapeKey({ onEscape: onClose });

  // 有效的 OID（过滤未解析的），并去重
  const validObjects = useMemo(() => {
    const seen = new Set<string>();
    return allObjects
      .filter((o: any) => o.oid && !o.oid.startsWith('unresolved'))
      .filter((o: any) => {
        if (seen.has(o.oid)) return false;
        seen.add(o.oid);
        return true;
      });
  }, [allObjects]);

  // 按模块名分组显示
  const objectsByModule = useMemo(() => {
    const groupMap = new Map<string, any[]>();
    for (const o of validObjects) {
      const key = o.module_name || 'Unknown';
      if (!groupMap.has(key)) groupMap.set(key, []);
      groupMap.get(key)!.push(o);
    }
    return Array.from(groupMap.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [validObjects]);

  const toggleDevice = (id: string) => {
    const s = new Set(selectedDeviceIds);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedDeviceIds(s);
  };

  const selectAllDevices = () => {
    if (selectedDeviceIds.size === networkDevices.length) {
      setSelectedDeviceIds(new Set());
    } else {
      setSelectedDeviceIds(new Set(networkDevices.map((d: any) => d.id)));
    }
  };

  const handleTest = async () => {
    if (selectedDeviceIds.size === 0) { toast.error('请选择至少一台设备'); return; }
    if (validObjects.length === 0) { toast.error('无可用 OID'); return; }

    const allOids = validObjects.map((o: any) => ({
      oid: o.oid, name: o.name, is_table: o.is_table || 0,
    }));

    // 分批采集，每批 200 个 OID
    const BATCH_SIZE = 200;
    const batches: typeof allOids[] = [];
    for (let i = 0; i < allOids.length; i += BATCH_SIZE) {
      batches.push(allOids.slice(i, i + BATCH_SIZE));
    }

    setTesting(true);
    setTestResults(null);
    setTestProgress({ current: 0, total: batches.length });

    const allResults: any[] = [];
    const deviceIds = Array.from(selectedDeviceIds);

    for (let i = 0; i < batches.length; i++) {
      setTestProgress({ current: i + 1, total: batches.length });
      try {
        const resp = await api.post('/snmp-collect/quick-test', {
          device_ids: deviceIds,
          oids: batches[i],
          snmp_version: snmpVersion,
          community: snmpVersion !== 'v3' ? community : undefined,
          snmp_user: snmpVersion === 'v3' ? snmpUser : undefined,
          snmp_auth_protocol: snmpVersion === 'v3' ? snmpAuthProtocol : undefined,
          snmp_auth_key: snmpVersion === 'v3' ? snmpAuthKey : undefined,
          snmp_priv_protocol: snmpVersion === 'v3' ? snmpPrivProtocol : undefined,
          snmp_priv_key: snmpVersion === 'v3' ? snmpPrivKey : undefined,
          snmp_credential_id: snmpCredentialId || undefined,
          timeout_ms: snmpTimeout,
        });
        const batchResults = resp.data.data || [];
        // 合并结果：同一设备的多批结果合并
        for (const br of batchResults) {
          const existing = allResults.find((r: any) => r.deviceId === br.deviceId);
          if (existing) {
            existing.metrics = [...existing.metrics, ...br.metrics];
            // 重新计算状态
            const hasError = existing.metrics.some((m: any) => m.error !== null);
            const hasValue = existing.metrics.some((m: any) => m.metric_value !== null && m.metric_value !== undefined);
            existing.status = hasError ? (hasValue ? 'partial' : 'failed') : 'success';
            existing.durationMs += br.durationMs;
          } else {
            allResults.push(br);
          }
        }
        setTestResults([...allResults]);
      } catch (err: any) {
        toast.error(`第 ${i + 1} 批采集失败: ${err.response?.data?.error || err.message}`);
      }
    }

    setTesting(false);
    setTestProgress(null);
  };

  const selectedModuleNames = moduleIds.map(id => (modules as any[]).find((m: any) => m.id === id)?.name).filter(Boolean);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-5xl max-h-[85vh] overflow-y-auto p-6 shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold text-text flex items-center gap-2">
            <Zap className="w-5 h-5 text-blue-400" />
            批量采集测试
          </h2>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-secondary rounded-md"><X className="w-4 h-4 text-text-secondary" /></button>
        </div>

        {/* 模块概览 */}
        <div className="mb-4 p-3 bg-blue-500/5 border border-blue-500/20 rounded-lg">
          <p className="text-sm text-text-secondary mb-1">
            已选择 <span className="text-blue-400 font-medium">{moduleIds.length}</span> 个模块，
            共 <span className="text-blue-400 font-medium">{validObjects.length}</span> 个去重后 OID
          </p>
          <p className="text-xs text-text-tertiary truncate">{selectedModuleNames.join(', ')}</p>
        </div>

        <div className="grid grid-cols-2 gap-4">
          {/* 设备选择 */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-medium text-text">选择设备</label>
              <button onClick={selectAllDevices} className="text-xs text-blue-400 hover:text-blue-300">
                {selectedDeviceIds.size === networkDevices.length ? '取消全选' : '全选'}
              </button>
            </div>
            <div className="max-h-48 overflow-y-auto border border-border rounded-lg p-2 space-y-1">
              {networkDevices.map((d: any) => (
                <div key={d.id} onClick={() => toggleDevice(d.id)}
                  className={`flex items-center gap-2 px-2.5 py-1.5 rounded cursor-pointer transition-colors ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500/10 text-blue-400' : 'hover:bg-surface-secondary'
                  }`}>
                  <input type="checkbox" checked={selectedDeviceIds.has(d.id)} onChange={() => {}} className="rounded" />
                  <span className="text-sm text-text truncate">{d.name}</span>
                  <span className="text-xs text-text-secondary font-mono ml-auto">{d.ip_address}</span>
                </div>
              ))}
            </div>
          </div>

          {/* OID 按模块分组预览 */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <label className="text-sm font-medium text-text">OID 列表（按模块分组）</label>
              <span className="text-xs text-text-secondary">{validObjects.length} 个</span>
            </div>
            <div className="max-h-48 overflow-y-auto border border-border rounded-lg">
              {objectsLoading ? (
                <div className="flex items-center justify-center py-4"><Loader2 className="w-4 h-4 animate-spin text-blue-400" /></div>
              ) : objectsByModule.map(([modName, objs]) => (
                <div key={modName}>
                  <div className="px-3 py-1.5 bg-surface-secondary/30 text-xs font-medium text-text-secondary sticky top-0">{modName} ({objs.length})</div>
                  {objs.slice(0, 20).map((o: any) => (
                    <div key={o.id} className="flex items-center gap-2 px-3 py-1 border-b border-border/20">
                      <span className="text-xs font-mono text-blue-400 flex-shrink-0">{o.oid}</span>
                      <span className="text-xs text-text truncate">{o.name || '-'}</span>
                      {o.is_table === 1 && <span className="text-[10px] text-yellow-400 flex-shrink-0">TABLE</span>}
                    </div>
                  ))}
                  {objs.length > 20 && (
                    <div className="px-3 py-1 text-xs text-text-tertiary text-center">... 还有 {objs.length - 20} 个</div>
                  )}
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* SNMP 参数 */}
        <div className="mt-4 p-3.5 bg-surface-secondary/30 border border-border/60 rounded-xl space-y-3">
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs font-medium text-text mb-1">SNMP 版本</label>
              <select
                value={snmpVersion}
                onChange={e => setSnmpVersion(e.target.value)}
                className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500 cursor-pointer"
              >
                {SNMP_VERSION_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
              </select>
            </div>
            {savedCredentials.length > 0 && (
              <div>
                <label className="block text-xs font-medium text-text mb-1">选择已存凭据</label>
                <select
                  value={snmpCredentialId}
                  onChange={e => {
                    const cid = e.target.value;
                    setSnmpCredentialId(cid);
                    const c = savedCredentials.find((x: any) => x.id === cid);
                    if (c) {
                      if (c.snmp_version === 'v3') {
                        setSnmpVersion('v3');
                        setSnmpUser(c.snmp_user || '');
                        setSnmpAuthProtocol(c.snmp_auth_protocol || 'SHA');
                        setSnmpPrivProtocol(c.snmp_priv_protocol || 'AES');
                      } else {
                        setSnmpVersion(c.snmp_version || 'v2c');
                        setCommunity(c.community || 'public');
                      }
                    }
                  }}
                  className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-blue-400 focus:outline-none focus:border-blue-500 cursor-pointer truncate"
                >
                  <option value="">🔑 选 SNMP 凭据...</option>
                  {savedCredentials.map((c: any) => (
                    <option key={c.id} value={c.id}>
                      {c.name} ({c.snmp_version === 'v3' ? `v3: ${c.snmp_user || '用户'}` : (c.community || 'public')})
                    </option>
                  ))}
                </select>
              </div>
            )}
            <div>
              <label className="block text-xs font-medium text-text mb-1">超时(ms)</label>
              <input
                type="number"
                value={snmpTimeout}
                onChange={e => setSnmpTimeout(parseInt(e.target.value) || 15000)}
                className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500"
              />
            </div>
          </div>

          {snmpVersion !== 'v3' ? (
            <div>
              <label className="block text-xs font-medium text-text mb-1">Community (只读串)</label>
              <input
                value={community}
                onChange={e => setCommunity(e.target.value)}
                className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-sm text-text font-mono focus:outline-none focus:border-blue-500"
              />
            </div>
          ) : (
            <div className="bg-[#090d1a] border border-blue-500/20 rounded-lg p-3 space-y-2.5">
              <div>
                <label className="block text-xs font-semibold text-text-secondary mb-1">
                  安全用户名 (Username) <span className="text-red-400">*</span>
                </label>
                <input
                  type="text"
                  value={snmpUser}
                  onChange={e => setSnmpUser(e.target.value)}
                  placeholder="例如: snmpuser, admin_v3"
                  className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-cyan-300 focus:outline-none focus:border-blue-500"
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">认证协议 (Auth)</label>
                  <select
                    value={snmpAuthProtocol}
                    onChange={e => setSnmpAuthProtocol(e.target.value)}
                    className="w-full px-2.5 py-1.5 bg-surface border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
                  >
                    <option value="">无 (NoAuth)</option>
                    <option value="SHA">SHA / SHA1</option>
                    <option value="SHA256">SHA-256 (推荐)</option>
                    <option value="SHA512">SHA-512</option>
                    <option value="MD5">MD5</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">认证密码 (Auth Key)</label>
                  <div className="relative">
                    <input
                      type={showAuthKey ? 'text' : 'password'}
                      value={snmpAuthKey}
                      onChange={e => setSnmpAuthKey(e.target.value)}
                      placeholder="认证密码"
                      className="w-full pl-2.5 pr-7 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-text focus:outline-none focus:border-blue-500"
                    />
                    <button
                      type="button"
                      onClick={() => setShowAuthKey(!showAuthKey)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text"
                    >
                      {showAuthKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                    </button>
                  </div>
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">加密协议 (Privacy)</label>
                  <select
                    value={snmpPrivProtocol}
                    onChange={e => setSnmpPrivProtocol(e.target.value)}
                    className="w-full px-2.5 py-1.5 bg-surface border border-border rounded-lg text-xs text-text focus:outline-none focus:border-blue-500"
                  >
                    <option value="">无 (NoPriv)</option>
                    <option value="AES">AES / AES128 (推荐)</option>
                    <option value="AES192">AES-192</option>
                    <option value="AES256">AES-256</option>
                    <option value="DES">DES</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">加密密码 (Priv Key)</label>
                  <div className="relative">
                    <input
                      type={showPrivKey ? 'text' : 'password'}
                      value={snmpPrivKey}
                      onChange={e => setSnmpPrivKey(e.target.value)}
                      placeholder="加密密码"
                      className="w-full pl-2.5 pr-7 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-text focus:outline-none focus:border-blue-500"
                    />
                    <button
                      type="button"
                      onClick={() => setShowPrivKey(!showPrivKey)}
                      className="absolute right-2 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text"
                    >
                      {showPrivKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                    </button>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* 测试按钮 */}
        <div className="flex flex-col items-center gap-2 mt-4">
          <button onClick={handleTest} disabled={testing || objectsLoading}
            className="flex items-center gap-2 px-6 py-2.5 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all disabled:opacity-50">
            {testing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Zap className="w-4 h-4" />}
            {testing
              ? `采集中 ${testProgress ? `${testProgress.current}/${testProgress.total}` : ''}...`
              : `开始测试 (${validObjects.length} OID)`}
          </button>
          {testing && testProgress && (
            <div className="w-64">
              <div className="w-full bg-surface-secondary rounded-full h-1.5">
                <div className="bg-blue-500 h-1.5 rounded-full transition-all" style={{ width: `${(testProgress.current / testProgress.total) * 100}%` }} />
              </div>
              <p className="text-xs text-text-secondary text-center mt-1">
                第 {testProgress.current} / {testProgress.total} 批
              </p>
            </div>
          )}
        </div>

        {/* 测试结果 */}
        {testResults && (
          <div className="mt-4 border border-border rounded-xl overflow-hidden">
            <div className="px-4 py-2 bg-surface-secondary/30 border-b border-border flex items-center justify-between">
              <h3 className="text-sm font-medium text-text">测试结果</h3>
              <button
                onClick={async () => {
                  // 批量保存所有有值的指标到面板
                  const toSave: any[] = [];
                  for (const r of testResults) {
                    for (const m of r.metrics) {
                      if (m.metric_value !== null && m.metric_value !== undefined && m.metric_value !== 'N/A' && !m.error && !savingMetrics.has(m.oid)) {
                        toSave.push({
                          oid: m.oid,
                          custom_name: m.metric_name || m.oid,
                          original_name: m.metric_name,
                          device_id: r.deviceId,
                          snmp_version: snmpVersion,
                          community,
                          is_table: m.is_table || 0,
                        });
                      }
                    }
                  }
                  if (toSave.length === 0) { toast.info('没有可保存的有效指标'); return; }
                  try {
                    await api.post('/snmp-collect/dashboard-metrics/batch', { metrics: toSave });
                    toast.success(`已保存 ${toSave.length} 个指标到监控面板`);
                    queryClient.invalidateQueries({ queryKey: ['custom-dashboard-metrics'] });
                  } catch (err: any) {
                    toast.error(err.response?.data?.error || '保存失败');
                  }
                }}
                className="flex items-center gap-1 px-3 py-1 text-xs font-medium text-blue-400 hover:bg-blue-500/10 rounded-md transition-colors"
              >
                <CheckSquare className="w-3 h-3" />
                全部保存到面板
              </button>
            </div>
            <div className="max-h-72 overflow-y-auto">
              {testResults.map((r: any, i: number) => (
                <div key={i} className="border-b border-border/50 last:border-0">
                  <div className="flex items-center gap-2 px-4 py-2">
                    {r.status === 'success' ? <CheckCircle2 className="w-4 h-4 text-emerald-400" /> :
                     r.status === 'partial' ? <AlertTriangle className="w-4 h-4 text-yellow-400" /> :
                     <XCircle className="w-4 h-4 text-red-400" />}
                    <span className="text-sm font-medium text-text">{r.deviceName}</span>
                    <span className="text-xs text-text-secondary font-mono">{r.deviceIp}</span>
                    <span className="text-xs text-text-secondary ml-auto">{r.durationMs}ms</span>
                  </div>
                  {r.metrics?.length > 0 && (
                    <div className="px-4 pb-2">
                      <table className="w-full">
                        <thead>
                          <tr className="border-b border-border/20">
                            <th className="text-left text-[10px] font-medium text-text-secondary py-1">OID</th>
                            <th className="text-left text-[10px] font-medium text-text-secondary py-1">名称</th>
                            <th className="text-left text-[10px] font-medium text-text-secondary py-1">值</th>
                            <th className="text-left text-[10px] font-medium text-text-secondary py-1">操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {r.metrics.map((m: any, j: number) => (
                            <tr key={j} className="border-t border-border/10">
                              <td className="py-0.5 text-[10px] font-mono text-blue-400">{m.oid}</td>
                              <td className="py-0.5 text-[10px] text-text pl-2">{m.metric_name}</td>
                              <td className="py-0.5 text-[10px] text-text pl-2">
                                {m.error ? <span className="text-red-400">{m.error}</span> : <span className="font-mono">{m.metric_value ?? '-'}</span>}
                              </td>
                              <td className="py-0.5 pl-2">
                                {m.metric_value !== null && m.metric_value !== undefined && m.metric_value !== 'N/A' && !m.error && (
                                  <button
                                    onClick={() => setSendToDashboardData({
                                      oid: m.oid,
                                      customName: m.metric_name || m.name || m.oid,
                                      deviceId: r.deviceId,
                                      deviceName: r.deviceName,
                                      deviceIp: r.deviceIp,
                                      lastValue: String(m.metric_value),
                                      snmpVersion,
                                      community,
                                      is_table: m.is_table || 0,
                                    })}
                                    className="flex items-center gap-1 px-1.5 py-0.5 text-[10px] text-blue-400 hover:bg-blue-500/10 rounded transition-colors font-medium"
                                  >
                                    <Monitor className="w-2.5 h-2.5" />
                                    发送到监控大屏
                                  </button>
                                )}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}

        {/* 自定义名称弹窗 */}
        {editingMetricName && (
          <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={() => setEditingMetricName(null)}>
            <div className="bg-surface border border-border rounded-xl w-full max-w-sm p-5 shadow-xl" onClick={e => e.stopPropagation()}>
              <h3 className="text-sm font-semibold text-text mb-3">保存到监控面板</h3>
              <div className="space-y-3">
                <div>
                  <label className="block text-xs font-medium text-text mb-1">OID</label>
                  <p className="text-xs font-mono text-blue-400">{editingMetricName.oid}</p>
                </div>
                <div>
                  <label className="block text-xs font-medium text-text mb-1">设备</label>
                  <p className="text-xs text-text">{editingMetricName.deviceName}</p>
                </div>
                <div>
                  <label className="block text-xs font-medium text-text mb-1">自定义名称</label>
                  <input
                    value={editingMetricName.name}
                    onChange={e => setEditingMetricName({ ...editingMetricName, name: e.target.value })}
                    className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500"
                    autoFocus
                  />
                </div>
              </div>
              <div className="flex justify-end gap-2 mt-4">
                <button onClick={() => setEditingMetricName(null)} className="px-3 py-1.5 text-xs text-text-secondary hover:text-text">取消</button>
                <button
                  onClick={async () => {
                    try {
                      await api.post('/snmp-collect/dashboard-metrics', {
                        oid: editingMetricName.oid,
                        custom_name: editingMetricName.name,
                        original_name: editingMetricName.name,
                        device_id: editingMetricName.deviceId,
                        snmp_version: snmpVersion,
                        community,
                        is_table: editingMetricName.is_table,
                      });
                      toast.success(`指标 "${editingMetricName.name}" 已保存到监控面板`);
                      queryClient.invalidateQueries({ queryKey: ['custom-dashboard-metrics'] });
                      setEditingMetricName(null);
                    } catch (err: any) {
                      toast.error(err.response?.data?.error || '保存失败');
                    }
                  }}
                  className="px-3 py-1.5 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-xs font-medium rounded-md hover:from-blue-700 hover:to-blue-800"
                >
                  保存
                </button>
              </div>
            </div>
          </div>
        )}

        {/* 发送到监控大屏弹窗 */}
        {sendToDashboardData && (
          <SendToDashboardModal
            initialData={sendToDashboardData}
            onClose={() => setSendToDashboardData(null)}
          />
        )}
      </div>
    </div>
  );
}

// ============================================================
// 采集任务 Tab
// ============================================================

function TasksTab() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [showModal, setShowModal] = useState(false);
  const [customOidModalOpen, setCustomOidModalOpen] = useState(false);
  const [editingTask, setEditingTask] = useState<any | null>(null);
  const [executingId, setExecutingId] = useState<string | null>(null);
  const [confirmDeleteTaskId, setConfirmDeleteTaskId] = useState<string | null>(null);

  const { data: tasks = [], isLoading } = useQuery({
    queryKey: ['snmp-collect-tasks'],
    queryFn: () => api.get('/snmp-collect/tasks').then(res => res.data.data || []),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/snmp-collect/tasks/${id}`),
    onSuccess: () => {
      toast.success('任务已删除');
      queryClient.invalidateQueries({ queryKey: ['snmp-collect-tasks'] });
    },
    onError: () => toast.error('删除任务失败'),
  });

  const toggleMutation = useMutation({
    mutationFn: (id: string) => api.post(`/snmp-collect/tasks/${id}/toggle`),
    onSuccess: () => {
      toast.success('任务状态已切换');
      queryClient.invalidateQueries({ queryKey: ['snmp-collect-tasks'] });
    },
    onError: () => toast.error('切换任务状态失败'),
  });

  const executeMutation = useMutation({
    mutationFn: async (id: string) => {
      setExecutingId(id);
      return api.post(`/snmp-collect/tasks/${id}/execute`);
    },
    onSuccess: () => {
      toast.success('采集任务执行完成');
      setExecutingId(null);
      queryClient.invalidateQueries({ queryKey: ['snmp-collect-tasks'] });
      queryClient.invalidateQueries({ queryKey: ['snmp-collect-results'] });
    },
    onError: () => {
      toast.error('执行采集任务失败');
      setExecutingId(null);
    },
  });

  const frequencyLabel = (task: any) => {
    if (task.schedule_type === 'manual') return '手动';
    if (task.schedule_type === 'cron') return `Cron: ${task.cron_expression}`;
    if (task.schedule_type === 'interval') {
      return INTERVAL_OPTIONS.find(o => o.value === task.interval_seconds)?.label || `${task.interval_seconds}s`;
    }
    return '-';
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <p className="text-sm text-text-secondary">管理 SNMP 采集任务，定义采集目标、频率和参数</p>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => setCustomOidModalOpen(true)}
            className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-700 hover:to-indigo-700 text-white text-sm font-medium rounded-md shadow-sm transition-all"
          >
            <Zap className="w-4 h-4 text-yellow-300" />
            自定义 OID 探测与下发
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
        <div className="text-center py-16 bg-surface rounded-xl border border-dashed border-border p-8">
          <div className="w-12 h-12 rounded-2xl bg-purple-500/10 border border-purple-500/20 text-purple-400 flex items-center justify-center mx-auto mb-3">
            <Zap className="w-6 h-6" />
          </div>
          <h3 className="text-base font-semibold text-text mb-1">暂无采集任务</h3>
          <p className="text-sm text-text-secondary max-w-md mx-auto mb-5">
            您可以直接使用「自定义 OID 探测与下发」测试任意标量 (GET) 或表量 (WALK) OID 并配置 Tag 添加到设备，或点击「新建任务」批量创建定时轮询任务。
          </p>
          <div className="flex items-center justify-center gap-3">
            <button
              onClick={() => setCustomOidModalOpen(true)}
              className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-purple-600 to-indigo-600 text-white text-sm font-medium rounded-lg shadow-sm hover:from-purple-700 hover:to-indigo-700 transition-all"
            >
              <Zap className="w-4 h-4 text-yellow-300" />
              🎯 自定义 OID 探测与下发
            </button>
            <button
              onClick={() => { setEditingTask(null); setShowModal(true); }}
              className="flex items-center gap-2 px-4 py-2 bg-surface border border-border text-text hover:bg-surface-secondary text-sm font-medium rounded-lg transition-all"
            >
              <Plus className="w-4 h-4" />
              ➕ 新建定时采集任务
            </button>
          </div>
        </div>
      ) : (
        <div className="bg-surface rounded-xl border border-border overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border bg-surface-secondary/30">
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">任务名称</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">设备数</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">OID 数</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">频率</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">SNMP 版本</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">状态</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">上次执行</th>
                <th className="text-right text-xs font-medium text-text-secondary px-4 py-3">操作</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((task: any) => (
                <tr key={task.id} className="border-b border-border/50 hover:bg-surface-secondary/20 transition-colors">
                  <td className="px-4 py-3">
                    <div>
                      <span className="font-medium text-text">{task.name}</span>
                      {task.description && <p className="text-xs text-text-secondary mt-0.5">{task.description}</p>}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{task.device_count || 0} 台</td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{task.oids?.length || 0} 个</td>
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-1.5">
                      <Clock className="w-3.5 h-3.5 text-text-secondary" />
                      <span className="text-sm text-text-secondary">{frequencyLabel(task)}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-sm text-text-secondary">{task.snmp_version?.toUpperCase() || 'v2c'}</td>
                  <td className="px-4 py-3">
                    <button onClick={() => toggleMutation.mutate(task.id)} className="flex items-center gap-1.5">
                      {task.enabled ? (
                        <><ToggleRight className="w-5 h-5 text-emerald-400" /><span className="text-xs text-emerald-400">启用</span></>
                      ) : (
                        <><ToggleLeft className="w-5 h-5 text-text-secondary" /><span className="text-xs text-text-secondary">禁用</span></>
                      )}
                    </button>
                  </td>
                  <td className="px-4 py-3">
                    {task.last_run_at ? (
                      <div className="flex items-center gap-1.5">
                        {task.last_run_status === 'success' ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" /> :
                         task.last_run_status === 'failed' ? <XCircle className="w-3.5 h-3.5 text-red-400" /> :
                         <AlertTriangle className="w-3.5 h-3.5 text-yellow-400" />}
                        <span className="text-xs text-text-secondary">{new Date(task.last_run_at).toLocaleString()}</span>
                      </div>
                    ) : <span className="text-xs text-text-secondary">未执行</span>}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1">
                      <button
                        onClick={() => executeMutation.mutate(task.id)}
                        disabled={executingId === task.id}
                        className="flex items-center gap-1 px-2.5 py-1.5 text-xs font-medium text-blue-400 hover:bg-blue-500/10 rounded-md transition-colors disabled:opacity-50"
                      >
                        {executingId === task.id ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
                        执行
                      </button>
                      <button
                        onClick={() => { setEditingTask(task); setShowModal(true); }}
                        className="p-1.5 text-text-secondary hover:text-text hover:bg-surface-secondary rounded-md transition-colors"
                      >
                        <Settings className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => setConfirmDeleteTaskId(task.id)}
                        className="p-1.5 text-text-secondary hover:text-red-400 hover:bg-red-500/10 rounded-md transition-colors"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 删除确认弹窗 */}
      {confirmDeleteTaskId && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={() => setConfirmDeleteTaskId(null)}>
          <div className="bg-surface border border-border rounded-xl w-full max-w-sm p-6 shadow-xl" onClick={e => e.stopPropagation()}>
            <h3 className="text-base font-semibold text-text mb-2">确认删除</h3>
            <p className="text-sm text-text-secondary mb-4">确定删除此任务？关联的采集数据也将被删除。</p>
            <div className="flex justify-end gap-3">
              <button onClick={() => setConfirmDeleteTaskId(null)} className="px-4 py-2 text-sm text-text-secondary hover:text-text transition-colors">取消</button>
              <button onClick={() => { deleteMutation.mutate(confirmDeleteTaskId); setConfirmDeleteTaskId(null); }}
                className="px-4 py-2 bg-red-500 text-white text-sm font-medium rounded-md hover:bg-red-600 transition-colors">删除</button>
            </div>
          </div>
        </div>
      )}

      {showModal && (
        <TaskModal
          task={editingTask}
          onClose={() => { setShowModal(false); setEditingTask(null); }}
          onSave={() => { setShowModal(false); setEditingTask(null); queryClient.invalidateQueries({ queryKey: ['snmp-collect-tasks'] }); }}
        />
      )}

      {customOidModalOpen && (
        <CustomOidTestAndAddModal
          onClose={() => setCustomOidModalOpen(false)}
        />
      )}
    </div>
  );
}

// ============================================================
// 采集数据 Tab
// ============================================================

function DataTab() {
  const [selectedTask, setSelectedTask] = useState('');
  const [selectedDevice, setSelectedDevice] = useState('');

  const { data: tasks = [] } = useQuery({
    queryKey: ['snmp-collect-tasks'],
    queryFn: () => api.get('/snmp-collect/tasks').then(res => res.data.data || []),
  });

  const { data: resultsData, isLoading } = useQuery({
    queryKey: ['snmp-collect-results', selectedTask, selectedDevice],
    queryFn: () => api.get('/snmp-collect/results', {
      params: {
        task_id: selectedTask || undefined,
        device_id: selectedDevice || undefined,
        limit: 500,
      },
    }).then(res => res.data.data || { results: [], total: 0 }),
  });

  const results = (resultsData?.results || resultsData) as any[];

  const { data: networkDevices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
    enabled: !!selectedTask,
  });

  const filteredResults = useMemo(() => {
    if (!Array.isArray(results)) return [];
    return results;
  }, [results]);

  const formatDuration = (ms?: number | null) => {
    if (!ms) return '-';
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3 flex-wrap">
        <select
          value={selectedTask}
          onChange={e => { setSelectedTask(e.target.value); setSelectedDevice(''); }}
          className="px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500 min-w-[160px]"
        >
          <option value="">全部任务</option>
          {tasks.map((t: any) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>

        {selectedTask && (
          <select
            value={selectedDevice}
            onChange={e => setSelectedDevice(e.target.value)}
            className="px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500 min-w-[160px]"
          >
            <option value="">全部设备</option>
            {networkDevices.map((d: any) => <option key={d.id} value={d.id}>{d.name} ({d.ip_address})</option>)}
          </select>
        )}
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-20"><Loader2 className="w-6 h-6 animate-spin text-blue-500" /></div>
      ) : filteredResults.length === 0 ? (
        <div className="text-center py-20 text-text-secondary">暂无采集数据</div>
      ) : (
        <div className="bg-surface rounded-xl border border-border overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-border bg-surface-secondary/30">
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">时间</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">设备</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">OID 名称</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">值</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">类型</th>
                <th className="text-left text-xs font-medium text-text-secondary px-4 py-3">耗时</th>
              </tr>
            </thead>
            <tbody>
              {filteredResults.map((r: any, i: number) => (
                <tr key={r.id || i} className="border-b border-border/50 hover:bg-surface-secondary/20 transition-colors">
                  <td className="px-4 py-3 text-xs text-text-secondary">{r.sampled_at ? new Date(r.sampled_at).toLocaleString() : '-'}</td>
                  <td className="px-4 py-3">
                    <div>
                      <span className="text-sm text-text">{r.device_name || '-'}</span>
                      <span className="text-xs text-text-secondary font-mono ml-2">{r.device_ip || ''}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <div>
                      <span className="text-sm text-text">{r.metric_name || '-'}</span>
                      <span className="text-xs text-text-secondary font-mono ml-2">{r.oid || ''}</span>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-sm text-text font-mono">
                    {r.error_message ? <span className="text-red-400">{r.error_message}</span> : (r.metric_value ?? '-')}
                  </td>
                  <td className="px-4 py-3 text-xs text-text-secondary">{r.metric_type || '-'}</td>
                  <td className="px-4 py-3 text-xs text-text-secondary">{formatDuration(r.duration_ms)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ============================================================
// 任务创建/编辑弹窗
// ============================================================

function TaskModal({ task, onClose, onSave }: {
  task: any | null;
  onClose: () => void;
  onSave: () => void;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();

  const [name, setName] = useState(task?.name || '');
  const [description, setDescription] = useState(task?.description || '');
  const [deviceType, setDeviceType] = useState(task?.device_type || 'network');
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(
    new Set(task ? (Array.isArray(task.device_ids) ? task.device_ids : JSON.parse(task.device_ids || '[]')) : [])
  );
  const [oidsText, setOidsText] = useState<string>(
    task ? (Array.isArray(task.oids) ? task.oids : JSON.parse(task.oids || '[]')).map((o: any) =>
      typeof o === 'string' ? o : `${o.oid}|${o.name}`
    ).join('\n') : ''
  );
  const [frequencyMode, setFrequencyMode] = useState(task?.schedule_type || 'interval');
  const [intervalSeconds, setIntervalSeconds] = useState(task?.interval_seconds || 300);
  const [scheduleCron, setScheduleCron] = useState(task?.cron_expression || '');
  const [snmpTimeout, setSnmpTimeout] = useState(task?.timeout_ms || 10000);
  const [retryCount, setRetryCount] = useState(task?.retries ?? 2);
  const [snmpVersion, setSnmpVersion] = useState(task?.snmp_version || 'v2c');
  const [community, setCommunity] = useState(task?.community || 'public');
  const [retentionDays, setRetentionDays] = useState(task?.retention_days || 90);
  const [deviceSearch, setDeviceSearch] = useState('');
  const [oidSearchModal, setOidSearchModal] = useState(false);
  const [oidSearchQuery, setOidSearchQuery] = useState('');
  const [debouncedOidSearch, setDebouncedOidSearch] = useState('');

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedOidSearch(oidSearchQuery), 300);
    return () => clearTimeout(timer);
  }, [oidSearchQuery]);

  const { data: oidSearchResults = [] } = useQuery({
    queryKey: ['snmp-mib-search-modal', debouncedOidSearch],
    queryFn: () => api.get('/snmp-collect/mib-objects/search', { params: { q: debouncedOidSearch } }).then(res => res.data.data || []),
    enabled: !!debouncedOidSearch,
  });

  const { data: networkDevices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
    enabled: deviceType === 'network' || deviceType === 'all',
  });

  const { data: servers = [] } = useQuery({
    queryKey: ['servers'],
    queryFn: () => api.get('/servers').then(res => res.data.data || []),
    enabled: deviceType === 'server' || deviceType === 'all',
  });

  useEscapeKey({ onEscape: onClose, enabled: !oidSearchModal });
  useEscapeKey({ onEscape: () => setOidSearchModal(false), enabled: oidSearchModal });

  const devices = useMemo(() => {
    if (deviceType === 'network') return networkDevices;
    if (deviceType === 'server') return servers;
    return [...networkDevices, ...servers];
  }, [deviceType, networkDevices, servers]);

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

  const addOidFromSearch = (oid: string, name: string) => {
    const newLine = `${oid}|${name}`;
    const current = oidsText ? oidsText.trim() + '\n' + newLine : newLine;
    setOidsText(current);
    toast.success(`已添加 ${name}`);
  };

  const handleSave = async () => {
    if (!name.trim()) { toast.error('请输入任务名称'); return; }
    if (selectedDeviceIds.size === 0) { toast.error('请选择至少一台设备'); return; }

    const parsedOids = oidsText.split('\n').map(l => l.trim()).filter(l => l).map(line => {
      const parts = line.split('|');
      return parts.length >= 2 ? { oid: parts[0], name: parts.slice(1).join('|') } : { oid: line, name: '' };
    });
    if (parsedOids.length === 0) { toast.error('请至少添加一个 OID'); return; }
    if (frequencyMode === 'cron' && !scheduleCron.trim()) { toast.error('请输入 Cron 表达式'); return; }

    const payload = {
      name,
      description,
      device_type: deviceType,
      device_ids: Array.from(selectedDeviceIds),
      oids: parsedOids,
      schedule_type: frequencyMode,
      interval_seconds: frequencyMode === 'interval' ? intervalSeconds : undefined,
      cron_expression: frequencyMode === 'cron' ? scheduleCron : undefined,
      timeout_ms: snmpTimeout,
      retries: retryCount,
      snmp_version: snmpVersion,
      community,
      retention_days: retentionDays,
    };

    try {
      if (task) {
        await api.put(`/snmp-collect/tasks/${task.id}`, payload);
        toast.success('任务已更新');
      } else {
        await api.post('/snmp-collect/tasks', payload);
        toast.success('任务已创建');
      }
      onSave();
    } catch (err: any) {
      toast.error(err.response?.data?.error || '保存失败');
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-surface border border-border rounded-xl w-full max-w-3xl max-h-[85vh] overflow-y-auto p-6 shadow-xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-6">
          <h2 className="text-lg font-semibold text-text">{task ? '编辑任务' : '新建任务'}</h2>
          <button onClick={onClose} className="p-1.5 hover:bg-surface-secondary rounded-md"><X className="w-4 h-4 text-text-secondary" /></button>
        </div>

        <div className="space-y-4">
          {/* 基本信息 */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-text mb-1">任务名称</label>
              <input value={name} onChange={e => setName(e.target.value)} placeholder="如：核心交换机流量采集"
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500" />
            </div>
            <div>
              <label className="block text-sm font-medium text-text mb-1">描述</label>
              <input value={description} onChange={e => setDescription(e.target.value)} placeholder="可选"
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500" />
            </div>
          </div>

          {/* 设备选择 */}
          <div>
            <div className="grid grid-cols-2 gap-4 mb-3">
              <div>
                <label className="block text-sm font-medium text-text mb-1">设备类型</label>
                <select value={deviceType} onChange={e => { setDeviceType(e.target.value); setSelectedDeviceIds(new Set()); }}
                  className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500">
                  {DEVICE_TYPE_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                </select>
              </div>
              <div className="flex items-end">
                <span className="text-sm text-text-secondary">已选择 {selectedDeviceIds.size} 台设备</span>
              </div>
            </div>
            <div className="relative mb-2">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-text-secondary" />
              <input value={deviceSearch} onChange={e => setDeviceSearch(e.target.value)} placeholder="搜索设备..."
                className="w-full pl-9 pr-4 py-1.5 bg-surface border border-border rounded-lg text-sm text-text placeholder:text-text-secondary focus:outline-none focus:border-blue-500" />
            </div>
            <div className="flex items-center justify-between mb-1">
              <span className="text-xs text-text-secondary">设备列表</span>
              <button onClick={selectAllDevices} className="text-xs text-blue-400 hover:text-blue-300">
                {selectedDeviceIds.size === filteredDevices.length ? '取消全选' : '全选'}
              </button>
            </div>
            <div className="max-h-40 overflow-y-auto border border-border rounded-lg">
              {filteredDevices.length === 0 ? (
                <div className="text-center py-4 text-sm text-text-secondary">暂无设备</div>
              ) : filteredDevices.map((d: any) => (
                <div key={d.id}
                  onClick={() => toggleDevice(d.id)}
                  className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors border-b border-border/30 last:border-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500/10' : 'hover:bg-surface-secondary/50'
                  }`}
                >
                  <div className={`w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500 border-blue-500' : 'border-border'
                  }`}>
                    {selectedDeviceIds.has(d.id) && <CheckCircle2 className="w-3 h-3 text-white" />}
                  </div>
                  {deviceType === 'server' ? <Server className="w-4 h-4 text-text-secondary flex-shrink-0" /> : <Monitor className="w-4 h-4 text-text-secondary flex-shrink-0" />}
                  <span className="text-sm text-text">{d.name}</span>
                  <span className="text-xs text-text-secondary font-mono">{d.ip_address}</span>
                  {d.vendor && <span className="text-xs text-text-secondary ml-auto">{d.vendor}</span>}
                </div>
              ))}
            </div>
          </div>

          {/* OID 列表 */}
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="block text-sm font-medium text-text">OID 列表</label>
              <button onClick={() => setOidSearchModal(true)} className="flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300">
                <Search className="w-3 h-3" /> 从 MIB 搜索添加
              </button>
            </div>
            <textarea value={oidsText} onChange={e => setOidsText(e.target.value)}
              placeholder={"每行一个 OID，格式: OID|名称\n1.3.6.1.2.1.1.3.0|sysUpTime\n1.3.6.1.2.1.2.2.1.10|ifInOctets"}
              rows={5}
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm font-mono text-text focus:outline-none focus:border-blue-500 resize-y" />
            <p className="mt-1 text-xs text-text-secondary">每行一个，格式: OID|名称（名称可选）</p>
          </div>

          {/* 采集频率 */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-text mb-1">采集频率</label>
              <select value={frequencyMode} onChange={e => setFrequencyMode(e.target.value)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500">
                {FREQUENCY_MODE_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
              </select>
            </div>
            {frequencyMode === 'interval' && (
              <div>
                <label className="block text-sm font-medium text-text mb-1">采集间隔</label>
                <select value={intervalSeconds} onChange={e => setIntervalSeconds(parseInt(e.target.value))}
                  className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500">
                  {INTERVAL_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                </select>
              </div>
            )}
            {frequencyMode === 'cron' && (
              <div>
                <label className="block text-sm font-medium text-text mb-1">Cron 表达式</label>
                <input value={scheduleCron} onChange={e => setScheduleCron(e.target.value)} placeholder="*/5 * * * *"
                  className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm font-mono text-text focus:outline-none focus:border-blue-500" />
              </div>
            )}
          </div>

          {/* SNMP 参数 */}
          <div className="grid grid-cols-3 gap-4">
            <div>
              <label className="block text-sm font-medium text-text mb-1">SNMP 版本</label>
              <select value={snmpVersion} onChange={e => setSnmpVersion(e.target.value)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500">
                {SNMP_VERSION_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
              </select>
            </div>
            <div>
              <label className="block text-sm font-medium text-text mb-1">Community</label>
              <input value={community} onChange={e => setCommunity(e.target.value)} placeholder="public"
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500" />
            </div>
            <div>
              <label className="block text-sm font-medium text-text mb-1">超时(ms)</label>
              <input type="number" value={snmpTimeout} onChange={e => setSnmpTimeout(parseInt(e.target.value) || 10000)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500" />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-text mb-1">重试次数</label>
              <input type="number" value={retryCount} onChange={e => setRetryCount(parseInt(e.target.value) || 0)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500" />
            </div>
            <div>
              <label className="block text-sm font-medium text-text mb-1">数据保留天数</label>
              <input type="number" value={retentionDays} onChange={e => setRetentionDays(parseInt(e.target.value) || 90)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text focus:outline-none focus:border-blue-500" />
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-3 mt-6">
          <button onClick={onClose} className="px-4 py-2 text-sm text-text-secondary hover:text-text transition-colors">取消</button>
          <button onClick={handleSave}
            className="px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 text-white text-sm font-medium rounded-md hover:from-blue-700 hover:to-blue-800 transition-all">
            保存
          </button>
        </div>

        {/* OID 搜索弹窗 */}
        {oidSearchModal && (
          <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 backdrop-blur-sm" onClick={() => setOidSearchModal(false)}>
            <div className="bg-surface border border-border rounded-xl w-full max-w-lg max-h-[70vh] overflow-hidden shadow-xl" onClick={e => e.stopPropagation()}>
              <div className="flex items-center justify-between p-4 border-b border-border">
                <h3 className="text-sm font-semibold text-text">从 MIB 搜索添加 OID</h3>
                <button onClick={() => setOidSearchModal(false)} className="p-1 hover:bg-surface-secondary rounded-md"><X className="w-4 h-4 text-text-secondary" /></button>
              </div>
              <div className="p-4">
                <div className="relative mb-3">
                  <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-secondary" />
                  <input value={oidSearchQuery} onChange={e => setOidSearchQuery(e.target.value)}
                    placeholder="搜索 OID、模块名或品牌..."
                    className="w-full pl-9 pr-4 py-2 bg-surface border border-border rounded-lg text-sm text-text placeholder:text-text-secondary focus:outline-none focus:border-blue-500" />
                </div>
                <div className="max-h-56 overflow-y-auto">
                  {debouncedOidSearch && oidSearchResults.length === 0 ? (
                    <p className="text-center py-8 text-sm text-text-secondary">未找到匹配的 OID</p>
                  ) : oidSearchResults.map((obj: any, i: number) => {
                    const matchType = obj.match_type || 'oid_name';
                    const matchLabel: Record<string, { text: string; cls: string }> = {
                      oid_name: { text: 'OID名', cls: 'bg-blue-500/15 text-blue-400 border-blue-500/30' },
                      oid_number: { text: 'OID号', cls: 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30' },
                      module_name: { text: '模块', cls: 'bg-violet-500/15 text-violet-400 border-violet-500/30' },
                      brand: { text: '品牌', cls: 'bg-amber-500/15 text-amber-400 border-amber-500/30' },
                    };
                    const badge = matchLabel[matchType] || matchLabel.oid_name;
                    return (
                      <div key={i} className="flex items-center justify-between px-3 py-2 border-b border-border/30 hover:bg-surface-secondary/20 transition-colors">
                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2">
                            <p className="text-sm text-text truncate">{obj.name}</p>
                            <span className={`inline-block px-1.5 py-0.5 text-[10px] font-medium rounded border flex-shrink-0 ${badge.cls}`}>{badge.text}</span>
                          </div>
                          <p className="text-xs text-text-secondary font-mono">{obj.oid}</p>
                          {obj.organization && <p className="text-xs text-text-secondary">{obj.organization}</p>}
                        </div>
                        <button onClick={() => addOidFromSearch(obj.oid, obj.name)}
                          className="ml-3 flex-shrink-0 px-2 py-1 text-xs text-blue-400 hover:bg-blue-500/10 rounded transition-colors">
                          添加
                        </button>
                      </div>
                    );
                  })}
                  {!debouncedOidSearch && <p className="text-center py-8 text-sm text-text-secondary">输入关键词搜索 OID、模块名或品牌</p>}
                </div>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
