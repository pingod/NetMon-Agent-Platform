/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * MIB OID 穿梭框组件
 *
 * 功能：
 * 1. 左侧展示所有 MIB 对象（带分类高亮和筛选）
 * 2. 每行 OID 可单独测试（需先选择设备）
 * 3. 选中 OID 移到右侧
 * 4. 右侧可编辑自定义名称
 * 5. "发送到大屏" 将选中 OID 批量创建为大屏指标
 */
import { useState, useMemo, useEffect } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import {
  X, Search, ChevronRight, ChevronsRight, ChevronsLeft,
  Zap, Loader2, Check, ArrowRight, Monitor,
  Network, Activity, Server,
  CheckCircle2, XCircle, Send, Filter
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';

// ── 分类元数据（与后端 mibClassifier.ts 对应） ──
const CATEGORY_META: Record<string, { label: string; color: string; bg: string; icon: any }> = {
  all:        { label: '全部',     color: '#6b7280', bg: 'bg-gray-500/10',    icon: Filter },
  system:     { label: '系统模块', color: '#3b82f6', bg: 'bg-blue-500/10',    icon: Server },
  optical:    { label: '光模块',   color: '#10b981', bg: 'bg-emerald-500/10', icon: Activity },
  interface:  { label: '接口',     color: '#0ea5e9', bg: 'bg-sky-500/10',     icon: Network },
  other:      { label: '其他',     color: '#6b7280', bg: 'bg-gray-500/10',    icon: Filter },
};

const SUBCATEGORY_META: Record<string, { label: string; color: string }> = {
  cpu:          { label: 'CPU',       color: '#3b82f6' },
  memory:       { label: '内存',      color: '#8b5cf6' },
  power:        { label: '电源',      color: '#f59e0b' },
  fan:          { label: '风扇',      color: '#06b6d4' },
  temperature:  { label: '板卡温度',  color: '#ef4444' },
  system_info:  { label: '系统信息',  color: '#6366f1' },
  module_type:  { label: '模块类型',  color: '#a855f7' },
  module_status:{ label: '模块在线',  color: '#84cc16' },
  rx_power:     { label: '接收功率',  color: '#10b981' },
  tx_power:     { label: '发送功率',  color: '#22c55e' },
  module_temp:  { label: '模块温度',  color: '#f97316' },
  wavelength:   { label: '波长',      color: '#d946ef' },
  if_status:    { label: '接口状态',  color: '#0ea5e9' },
  if_traffic:   { label: '接口流量',  color: '#14b8a6' },
  if_error:     { label: '接口错误',  color: '#dc2626' },
  if_speed:     { label: '接口速率',  color: '#7c3aed' },
  other:        { label: '',          color: '#6b7280' },
};

interface SelectedOid {
  oid: string;
  name: string;
  module_name: string;
  syntax: string;
  is_table: number;
  is_scalar: number;
  category: string;
  subcategory: string;
  label: string;
  customName: string;
  testValue?: string | null;
  testStatus?: 'success' | 'failed' | 'pending';
}

interface MibTransferModalProps {
  onClose: () => void;
  preselectedModuleIds?: string[];
}

export default function MibTransferModal({ onClose, preselectedModuleIds }: MibTransferModalProps) {
  const toast = useToast();
  useEscapeKey({ onEscape: onClose, enabled: true });

  // ── 状态 ──
  const [categoryFilter, setCategoryFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [debouncedSearch, setDebouncedSearch] = useState('');
  const [leftChecked, setLeftChecked] = useState<Set<string>>(new Set()); // oid as key
  const [selectedOids, setSelectedOids] = useState<Map<string, SelectedOid>>(new Map()); // oid -> SelectedOid
  const [testDeviceId, setTestDeviceId] = useState<string>('');
  const [deviceType, setDeviceType] = useState<'network' | 'server' | 'all'>('network');
  const [snmpVersion, setSnmpVersion] = useState('v2c');
  const [community, setCommunity] = useState('public');
  const [snmpCredentialId, setSnmpCredentialId] = useState('');
  const [snmpUser, setSnmpUser] = useState('');
  const [snmpAuthProtocol, setSnmpAuthProtocol] = useState('SHA');
  const [snmpPrivProtocol, setSnmpPrivProtocol] = useState('AES');
  const [testResults, setTestResults] = useState<Record<string, { value: string | null; status: 'success' | 'failed'; error?: string }>>({});
  const [batchTesting, setBatchTesting] = useState(false);
  const [testingOid, setTestingOid] = useState<string | null>(null);
  const [sendingToDashboard, setSendingToDashboard] = useState(false);

  // 搜索防抖
  useEffect(() => {
    const t = setTimeout(() => setDebouncedSearch(searchQuery), 300);
    return () => clearTimeout(t);
  }, [searchQuery]);

  // ── 数据查询 ──
  // 获取所有 MIB 对象（带分类）
  const { data: allObjects = [], isLoading: loadingObjects } = useQuery({
    queryKey: ['mib-transfer-objects', preselectedModuleIds?.join(',') || 'all'],
    queryFn: async () => {
      if (preselectedModuleIds && preselectedModuleIds.length > 0) {
        const res = await api.post('/snmp-collect/mib-modules/batch-objects', { module_ids: preselectedModuleIds });
        return res.data.data || [];
      }
      // 获取全部
      const res = await api.get('/snmp-collect/mib-objects/classified', { params: { limit: 3000 } });
      return res.data.data || [];
    },
  });

  // 获取网络设备列表
  const { data: networkDevices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
  });

  // 获取 SNMP 只读串列表
  const { data: savedCredentials = [] } = useQuery({
    queryKey: ['snmp-credentials'],
    queryFn: () => api.get('/snmp/credentials').then(res => res.data.data || []),
  });

  // 获取服务器列表
  const { data: servers = [] } = useQuery({
    queryKey: ['servers'],
    queryFn: () => api.get('/servers').then(res => res.data.data || []),
    enabled: deviceType === 'server' || deviceType === 'all',
  });

  const allDevices = useMemo(() => {
    const net = deviceType === 'server' ? [] : networkDevices.map((d: any) => ({ ...d, _type: 'network' }));
    const srv = deviceType === 'network' ? [] : servers.map((d: any) => ({ ...d, _type: 'server' }));
    return [...net, ...srv];
  }, [networkDevices, servers, deviceType]);

  // ── 左侧列表（过滤后） ──
  const filteredObjects = useMemo(() => {
    let list = allObjects;

    // 分类筛选
    if (categoryFilter !== 'all') {
      list = list.filter((obj: any) => obj.classification?.category === categoryFilter);
    }

    // 搜索筛选
    if (debouncedSearch) {
      const q = debouncedSearch.toLowerCase();
      list = list.filter((obj: any) =>
        obj.name?.toLowerCase().includes(q) ||
        obj.oid?.toLowerCase().includes(q) ||
        obj.module_name?.toLowerCase().includes(q) ||
        obj.description?.toLowerCase().includes(q)
      );
    }

    // 排除已在右侧的
    list = list.filter((obj: any) => !selectedOids.has(obj.oid));

    return list;
  }, [allObjects, categoryFilter, debouncedSearch, selectedOids]);

  // ── 分类统计 ──
  const categoryStats = useMemo(() => {
    const stats: Record<string, number> = { all: 0, system: 0, optical: 0, interface: 0, other: 0 };
    allObjects.forEach((obj: any) => {
      const cat = obj.classification?.category || 'other';
      stats.all++;
      stats[cat] = (stats[cat] || 0) + 1;
    });
    return stats;
  }, [allObjects]);

  // ── 操作 ──
  const toggleLeftCheck = (oid: string) => {
    setLeftChecked(prev => {
      const next = new Set(prev);
      if (next.has(oid)) next.delete(oid);
      else next.add(oid);
      return next;
    });
  };

  const selectAllLeft = () => {
    setLeftChecked(new Set(filteredObjects.map((o: any) => o.oid)));
  };

  const clearLeftChecks = () => {
    setLeftChecked(new Set());
  };

  const moveToRight = () => {
    const newSelected = new Map(selectedOids);
    leftChecked.forEach(oid => {
      const obj = allObjects.find((o: any) => o.oid === oid);
      if (obj) {
        const cls = obj.classification || { category: 'other', subcategory: 'other', label: '', color: '' };
        newSelected.set(oid, {
          oid: obj.oid,
          name: obj.name,
          module_name: obj.module_name || '',
          syntax: obj.syntax || '',
          is_table: obj.is_table || 0,
          is_scalar: obj.is_scalar ?? 1,
          category: cls.category,
          subcategory: cls.subcategory,
          label: cls.label,
          customName: cls.label || obj.name,
        });
      }
    });
    setSelectedOids(newSelected);
    setLeftChecked(new Set());
  };

  const removeFromRight = (oid: string) => {
    const next = new Map(selectedOids);
    next.delete(oid);
    setSelectedOids(next);
  };

  const updateCustomName = (oid: string, name: string) => {
    const next = new Map(selectedOids);
    const item = next.get(oid);
    if (item) {
      next.set(oid, { ...item, customName: name });
      setSelectedOids(next);
    }
  };

  // ── OID 测试 ──
  const testSingleOid = async (oid: string) => {
    if (!testDeviceId) {
      toast.warning('请先选择测试设备');
      return;
    }
    const obj = allObjects.find((o: any) => o.oid === oid);
    if (!obj) return;

    setTestingOid(oid);
    try {
      const res = await api.post('/snmp-collect/quick-test', {
        device_ids: [testDeviceId],
        oids: [{ oid: obj.oid, metric_name: obj.name, is_table: obj.is_table || 0 }],
        snmp_version: snmpVersion,
        community: snmpVersion !== 'v3' ? community : undefined,
        snmp_credential_id: snmpCredentialId || undefined,
        snmp_user: snmpVersion === 'v3' ? snmpUser : undefined,
        snmp_auth_protocol: snmpVersion === 'v3' ? snmpAuthProtocol : undefined,
        snmp_priv_protocol: snmpVersion === 'v3' ? snmpPrivProtocol : undefined,
        timeout_ms: 10000,
      });
      const results = res.data.data || [];
      if (results.length > 0 && results[0].metrics) {
        const metric = results[0].metrics.find((m: any) => m.oid === oid || m.metric_name === obj.name);
        setTestResults(prev => ({
          ...prev,
          [oid]: { value: metric?.metric_value ?? null, status: metric?.metric_value ? 'success' : 'failed', error: metric?.error || results[0].error },
        }));
      } else {
        setTestResults(prev => ({ ...prev, [oid]: { value: null, status: 'failed' } }));
      }
    } catch {
      setTestResults(prev => ({ ...prev, [oid]: { value: null, status: 'failed' } }));
    } finally {
      setTestingOid(null);
    }
  };

  // ── 获取待测试 OID 列表（合并左侧勾选 + 右侧已选） ──
  const getAllSelectableOids = (): SelectedOid[] => {
    const map = new Map<string, SelectedOid>();
    // 右侧已选优先
    selectedOids.forEach((v, _k) => map.set(v.oid, v));
    // 左侧勾选补充
    leftChecked.forEach(oid => {
      if (map.has(oid)) return;
      const obj = allObjects.find((o: any) => o.oid === oid);
      if (obj) {
        const cls = obj.classification || { category: 'other', subcategory: 'other', label: '' };
        map.set(oid, {
          oid: obj.oid,
          name: obj.name,
          module_name: obj.module_name || '',
          syntax: obj.syntax || '',
          is_table: obj.is_table || 0,
          is_scalar: obj.is_scalar ?? 1,
          category: cls.category,
          subcategory: cls.subcategory,
          label: cls.label,
          customName: cls.label || obj.name,
        });
      }
    });
    return Array.from(map.values());
  };

  const selectableCount = getAllSelectableOids().length;

  const testSelectedOids = async () => {
    if (!testDeviceId) {
      toast.warning('请先选择测试设备');
      return;
    }
    const allOids = getAllSelectableOids();
    if (allOids.length === 0) {
      toast.warning('请先选择 OID');
      return;
    }

    setBatchTesting(true);
    try {
      const oidList = allOids.map(o => ({
        oid: o.oid,
        metric_name: o.name,
        is_table: o.is_table,
      }));

      // 分批测试（每批 50 个）
      const batchSize = 50;
      const results: Record<string, { value: string | null; status: 'success' | 'failed'; error?: string }> = {};

      for (let i = 0; i < oidList.length; i += batchSize) {
        const batch = oidList.slice(i, i + batchSize);
        try {
          const res = await api.post('/snmp-collect/quick-test', {
            device_ids: [testDeviceId],
            oids: batch,
            snmp_version: snmpVersion,
            community: snmpVersion !== 'v3' ? community : undefined,
            snmp_credential_id: snmpCredentialId || undefined,
            snmp_user: snmpVersion === 'v3' ? snmpUser : undefined,
            snmp_auth_protocol: snmpVersion === 'v3' ? snmpAuthProtocol : undefined,
            snmp_priv_protocol: snmpVersion === 'v3' ? snmpPrivProtocol : undefined,
            timeout_ms: Math.max(10000, batch.length * 3000),
          });
          const deviceResults = res.data.data || [];
          if (deviceResults.length > 0 && deviceResults[0].metrics) {
            deviceResults[0].metrics.forEach((m: any) => {
              results[m.oid] = {
                value: m.metric_value,
                status: m.metric_value && m.metric_value !== 'N/A' ? 'success' : 'failed',
                error: m.error || deviceResults[0].error,
              };
            });
          }
        } catch {
          // 批次失败，标记所有为 failed
          batch.forEach(o => {
            results[o.oid] = { value: null, status: 'failed' };
          });
        }
      }

      setTestResults(prev => ({ ...prev, ...results }));
      const successCount = Object.values(results).filter(r => r.status === 'success').length;
      toast.success(`测试完成：${successCount}/${oidList.length} 个 OID 采集成功`);
    } catch (err: any) {
      toast.error('批量测试失败: ' + (err.message || '未知错误'));
    } finally {
      setBatchTesting(false);
    }
  };

  // ── 发送到大屏 ──
  const sendToDashboard = async () => {
    if (selectedOids.size === 0) {
      toast.warning('请先选择 OID');
      return;
    }
    if (!testDeviceId) {
      toast.warning('请先选择目标设备');
      return;
    }

    setSendingToDashboard(true);
    try {
      const items = Array.from(selectedOids.values()).map((o, idx) => ({
        oid: o.oid,
        custom_name: o.customName || o.name,
        original_name: o.name,
        device_id: testDeviceId,
        module_name: o.module_name,
        syntax: o.syntax,
        is_table: o.is_table,
        show_on_dashboard: 1,
        display_order: idx,
        snmp_version: snmpVersion,
        community,
      }));

      await api.post('/snmp-collect/dashboard-metrics/batch', { metrics: items });
      toast.success(`已将 ${items.length} 个指标发送到网络大屏`);
      onClose();
    } catch (err: any) {
      toast.error('发送失败: ' + (err.message || '未知错误'));
    } finally {
      setSendingToDashboard(false);
    }
  };

  // ── 渲染 ──
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="bg-surface border border-border rounded-2xl shadow-2xl flex flex-col" style={{ width: '92vw', height: '88vh', maxWidth: '1400px' }}>
        {/* 头部 */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border flex-shrink-0">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-blue-500 to-violet-500 flex items-center justify-center">
              <Network className="w-5 h-5 text-white" />
            </div>
            <div>
              <h2 className="text-lg font-semibold text-text-primary">OID 穿梭框 — 选择监控指标</h2>
              <p className="text-xs text-text-secondary">从 MIB 库中筛选、测试并选择 OID 发送到网络大屏</p>
            </div>
          </div>
          <button onClick={onClose} className="p-2 hover:bg-surface-2 rounded-lg transition-colors">
            <X className="w-5 h-5 text-text-secondary" />
          </button>
        </div>

        {/* 工具栏 */}
        <div className="px-6 py-3 border-b border-border flex items-center gap-3 flex-wrap flex-shrink-0">
          {/* 设备选择 */}
          <div className="flex items-center gap-2">
            <Monitor className="w-4 h-4 text-text-secondary flex-shrink-0" />
            <select
              value={deviceType}
              onChange={e => { setDeviceType(e.target.value as any); setTestDeviceId(''); }}
              className="px-2 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500"
            >
              <option value="network">网络设备</option>
              <option value="server">服务器</option>
              <option value="all">全部</option>
            </select>
            <select
              value={testDeviceId}
              onChange={e => setTestDeviceId(e.target.value)}
              className="px-2 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500 min-w-[180px]"
            >
              <option value="">-- 选择测试设备 --</option>
              {allDevices.map((d: any) => (
                <option key={d.id} value={d.id}>
                  {d.name} ({d.ip_address || d.ip || '-'})
                </option>
              ))}
            </select>
          </div>

          {/* SNMP 参数 */}
          <div className="flex items-center gap-1.5">
            <select
              value={snmpVersion}
              onChange={e => setSnmpVersion(e.target.value)}
              className="px-2 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500 cursor-pointer"
            >
              <option value="v2c">v2c</option>
              <option value="v3">v3</option>
              <option value="v1">v1</option>
            </select>
            {savedCredentials.length > 0 && (
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
                className="px-2 py-1.5 bg-surface border border-border rounded-lg text-xs text-blue-400 focus:outline-none focus:border-blue-500 cursor-pointer max-w-[180px] truncate"
                title="选择已保存的 SNMP 凭据 / 只读串"
              >
                <option value="">🔑 选 SNMP 凭据...</option>
                {savedCredentials.map((c: any) => (
                  <option key={c.id} value={c.id}>
                    {c.name} ({c.snmp_version === 'v3' ? `v3: ${c.snmp_user || '用户'}` : (c.community || 'public')})
                  </option>
                ))}
              </select>
            )}
            {snmpVersion !== 'v3' && (
              <input
                value={community}
                onChange={e => setCommunity(e.target.value)}
                className="w-24 px-2 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary font-mono focus:outline-none focus:border-blue-500"
                placeholder="community"
              />
            )}
          </div>

          {/* 测试选中按钮 */}
          <button
            onClick={testSelectedOids}
            disabled={batchTesting || selectableCount === 0 || !testDeviceId}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-blue-500/10 text-blue-400 border border-blue-500/30 rounded-lg text-xs font-medium hover:bg-blue-500/20 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {batchTesting ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Zap className="w-3.5 h-3.5" />}
            测试选中 ({selectableCount})
          </button>

          <div className="flex-1" />

          {/* 发送大屏按钮 */}
          <button
            onClick={() => {
              // 自动将左侧勾选项移到右侧再发送
              if (leftChecked.size > 0) moveToRight();
              sendToDashboard();
            }}
              disabled={sendingToDashboard || selectableCount === 0 || !testDeviceId}
            className="flex items-center gap-1.5 px-4 py-1.5 bg-gradient-to-r from-emerald-500 to-green-600 text-white rounded-lg text-xs font-medium hover:opacity-90 transition-opacity disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {sendingToDashboard ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Send className="w-3.5 h-3.5" />}
            发送到大屏 ({selectableCount})
          </button>
        </div>

        {/* 分类筛选栏 */}
        <div className="px-6 py-2 border-b border-border flex items-center gap-1.5 flex-shrink-0">
          {['all', 'system', 'optical', 'interface', 'other'].map(cat => {
            const meta = CATEGORY_META[cat];
            const Icon = meta.icon;
            const isActive = categoryFilter === cat;
            const count = categoryStats[cat] || 0;
            if (cat !== 'all' && count === 0) return null;
            return (
              <button
                key={cat}
                onClick={() => setCategoryFilter(cat)}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
                  isActive
                    ? 'bg-surface-2 text-text-primary shadow-sm ring-1 ring-border'
                    : 'text-text-secondary hover:text-text-primary hover:bg-surface-2/50'
                }`}
                style={isActive ? { color: meta.color } : {}}
              >
                <Icon className="w-3.5 h-3.5" style={{ color: isActive ? meta.color : undefined }} />
                {cat === 'all' ? '全部' : meta.label}
                <span className="ml-0.5 px-1 py-0.5 bg-surface-2 rounded text-[10px] text-text-secondary">{count}</span>
              </button>
            );
          })}
        </div>

        {/* 主体：穿梭框 */}
        <div className="flex-1 min-h-0 flex">
          {/* 左侧：可选 OID */}
          <div className="flex-1 min-w-0 flex flex-col border-r border-border">
            <div className="px-4 py-2 border-b border-border flex items-center gap-2">
              <Search className="w-4 h-4 text-text-secondary flex-shrink-0" />
              <input
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                placeholder="搜索 OID 名称、编号、模块..."
                className="flex-1 bg-transparent text-xs text-text-primary placeholder:text-text-secondary focus:outline-none"
              />
              {leftChecked.size > 0 && (
                <>
                  <span className="text-xs text-blue-400">{leftChecked.size} 选中</span>
                  <button onClick={clearLeftChecks} className="text-xs text-text-secondary hover:text-text-primary">取消</button>
                </>
              )}
              <button onClick={selectAllLeft} className="text-xs text-blue-400 hover:underline">全选</button>
            </div>

            <div className="flex-1 overflow-y-auto">
              {loadingObjects ? (
                <div className="flex items-center justify-center h-full"><Loader2 className="w-5 h-5 animate-spin text-blue-500" /></div>
              ) : filteredObjects.length === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-text-secondary">
                  <Filter className="w-8 h-8 mb-2 opacity-40" />
                  <p className="text-sm">没有可用的 OID</p>
                </div>
              ) : (
                <div className="divide-y divide-border/30">
                  {filteredObjects.map((obj: any) => {
                    const cls = obj.classification || {};
                    const catMeta = CATEGORY_META[cls.category] || CATEGORY_META.other;
                    const subMeta = SUBCATEGORY_META[cls.subcategory] || SUBCATEGORY_META.other;
                    const isChecked = leftChecked.has(obj.oid);
                    const testResult = testResults[obj.oid];

                    return (
                      <div
                        key={obj.id || (obj.oid + '|' + obj.module_name + '|' + Math.random())}
                        className={`flex items-center gap-2 px-4 py-2 transition-colors cursor-pointer ${
                          isChecked ? 'bg-blue-500/5' : 'hover:bg-surface-2/30'
                        }`}
                        onClick={() => toggleLeftCheck(obj.oid)}
                      >
                        {/* 复选框 */}
                        <div className="flex-shrink-0 w-4 h-4 rounded border flex items-center justify-center transition-all"
                          style={{
                            borderColor: isChecked ? catMeta.color : 'var(--color-border)',
                            backgroundColor: isChecked ? catMeta.color : 'transparent',
                          }}
                        >
                          {isChecked && <Check className="w-3 h-3 text-white" />}
                        </div>

                        {/* 分类标签 */}
                        {cls.category !== 'other' && (
                          <span
                            className="flex-shrink-0 px-1.5 py-0.5 text-[10px] font-medium rounded"
                            style={{ color: subMeta.color, backgroundColor: subMeta.color + '15' }}
                          >
                            {subMeta.label}
                          </span>
                        )}

                        {/* OID 信息 */}
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <span className="text-xs font-medium text-text-primary truncate">{obj.name}</span>
                            <span className="text-[10px] font-mono text-blue-400">{obj.oid}</span>
                          </div>
                          <div className="flex items-center gap-2 mt-0.5">
                            <span className="text-[10px] text-text-secondary">{obj.module_name}</span>
                            {obj.syntax && <span className="text-[10px] text-text-secondary">· {obj.syntax}</span>}
                            {obj.is_table ? <span className="text-[10px] text-amber-400">· TABLE</span> : null}
                          </div>
                        </div>

                        {/* 测试结果 */}
                        {testResult && (
                          <div className="flex-shrink-0">
                            {testResult.status === 'success' ? (
                              <span className="flex items-center gap-1 text-[10px] text-emerald-400">
                                <CheckCircle2 className="w-3 h-3" />
                                {testResult.value ? (testResult.value.length > 20 ? testResult.value.slice(0, 20) + '...' : testResult.value) : '(空)'}
                              </span>
                            ) : (
                              <span className="flex items-center gap-1 text-[10px] text-red-400" title={testResult.error || '采集失败'}>
                                <XCircle className="w-3 h-3" />{testResult.error ? '失败(' + (testResult.error.length > 10 ? testResult.error.slice(0, 10) + '…' : testResult.error) + ')' : '失败'}
                              </span>
                            )}
                          </div>
                        )}

                        {/* 单行测试按钮 */}
                        <button
                          onClick={(e) => { e.stopPropagation(); testSingleOid(obj.oid); }}
                          disabled={!testDeviceId || testingOid === obj.oid}
                          className="flex-shrink-0 p-1 text-blue-400 hover:bg-blue-500/10 rounded transition-colors disabled:opacity-30 disabled:cursor-not-allowed"
                          title="测试此 OID"
                        >
                          {testingOid === obj.oid ? (
                            <Loader2 className="w-3 h-3 text-blue-400 animate-spin" />
                          ) : (
                            <Zap className="w-3 h-3" />
                          )}
                        </button>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>

          {/* 中间：穿梭按钮 */}
          <div className="flex flex-col items-center justify-center gap-3 px-3 border-r border-border bg-surface-2/20">
            <button
              onClick={moveToRight}
              disabled={leftChecked.size === 0}
              className="p-2 bg-surface border border-border rounded-lg text-blue-400 hover:bg-blue-500/10 hover:border-blue-500/30 transition-all disabled:opacity-30 disabled:cursor-not-allowed"
              title="移到右侧"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
            <button
              onClick={() => { selectAllLeft(); setTimeout(() => moveToRight(), 0); }}
              disabled={filteredObjects.length === 0}
              className="p-2 bg-surface border border-border rounded-lg text-blue-400 hover:bg-blue-500/10 hover:border-blue-500/30 transition-all disabled:opacity-30 disabled:cursor-not-allowed"
              title="全部移到右侧"
            >
              <ChevronsRight className="w-4 h-4" />
            </button>
            <div className="w-6 border-t border-border my-1" />
            <button
              onClick={() => { /* remove all from right */ setSelectedOids(new Map()); }}
              disabled={selectedOids.size === 0}
              className="p-2 bg-surface border border-border rounded-lg text-text-secondary hover:bg-surface-2 transition-all disabled:opacity-30 disabled:cursor-not-allowed"
              title="全部移回左侧"
            >
              <ChevronsLeft className="w-4 h-4" />
            </button>
          </div>

          {/* 右侧：已选 OID */}
          <div className="w-[380px] flex-shrink-0 flex flex-col">
            <div className="px-4 py-2 border-b border-border flex items-center justify-between">
              <span className="text-xs font-medium text-text-primary">已选指标 ({selectedOids.size})</span>
              {selectedOids.size > 0 && (
                <button
                  onClick={() => setSelectedOids(new Map())}
                  className="text-xs text-text-secondary hover:text-red-400 transition-colors"
                >
                  清空
                </button>
              )}
            </div>

            <div className="flex-1 overflow-y-auto">
              {selectedOids.size === 0 ? (
                <div className="flex flex-col items-center justify-center h-full text-text-secondary">
                  <ArrowRight className="w-8 h-8 mb-2 opacity-30" />
                  <p className="text-xs">从左侧选择 OID</p>
                  <p className="text-[10px] mt-1">点击行或勾选后按 → 移到此处</p>
                </div>
              ) : (
                <div className="divide-y divide-border/30">
                  {Array.from(selectedOids.values()).map((item) => {
                    const catMeta = CATEGORY_META[item.category] || CATEGORY_META.other;
                    const subMeta = SUBCATEGORY_META[item.subcategory] || SUBCATEGORY_META.other;
                    const testResult = testResults[item.oid];

                    return (
                      <div key={item.oid} className="px-4 py-2 group">
                        <div className="flex items-center gap-2">
                          {item.category !== 'other' && (
                            <span
                              className="flex-shrink-0 px-1.5 py-0.5 text-[10px] font-medium rounded"
                              style={{ color: subMeta.color, backgroundColor: subMeta.color + '15' }}
                            >
                              {subMeta.label}
                            </span>
                          )}
                          <input
                            value={item.customName}
                            onChange={e => updateCustomName(item.oid, e.target.value)}
                            className="flex-1 min-w-0 text-xs font-medium text-text-primary bg-transparent border-b border-transparent hover:border-border focus:border-blue-500 focus:outline-none py-0.5 transition-colors"
                            placeholder="自定义名称"
                          />
                          <button
                            onClick={() => removeFromRight(item.oid)}
                            className="flex-shrink-0 p-0.5 text-text-secondary opacity-0 group-hover:opacity-100 hover:text-red-400 transition-all"
                          >
                            <X className="w-3 h-3" />
                          </button>
                        </div>
                        <div className="flex items-center gap-2 mt-1 pl-1">
                          <span className="text-[10px] font-mono text-blue-400">{item.oid}</span>
                          <span className="text-[10px] text-text-secondary">{item.name}</span>
                        </div>
                        {testResult && (
                          <div className="mt-1 pl-1">
                            {testResult.status === 'success' ? (
                              <span className="text-[10px] text-emerald-400">
                                采集值: {testResult.value || '(空)'}
                              </span>
                            ) : (
                              <span className="text-[10px] text-red-400" title={testResult.error}>采集失败{testResult.error ? `: ${testResult.error.length > 30 ? testResult.error.slice(0, 30) + '…' : testResult.error}` : ''}</span>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        </div>

        {/* 底部状态栏 */}
        <div className="px-6 py-2 border-t border-border flex items-center justify-between text-xs text-text-secondary flex-shrink-0">
          <div className="flex items-center gap-4">
            <span>共 {allObjects.length} 个 OID</span>
            {categoryFilter !== 'all' && <span>筛选: {CATEGORY_META[categoryFilter]?.label} ({filteredObjects.length})</span>}
            {Object.keys(testResults).length > 0 && (
              <span className="flex items-center gap-1">
                <Activity className="w-3 h-3" />
                已测试 {Object.keys(testResults).length} 个
                <span className="text-emerald-400">{Object.values(testResults).filter(r => r.status === 'success').length} 成功</span>
                <span className="text-red-400">{Object.values(testResults).filter(r => r.status === 'failed').length} 失败</span>
              </span>
            )}
          </div>
          <div className="flex items-center gap-2">
            <span>已选: <span className="text-blue-400 font-medium">{selectedOids.size}</span></span>
          </div>
        </div>
      </div>
    </div>
  );
}
