/* eslint-disable @typescript-eslint/no-explicit-any */
import React, { useState, useMemo, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  X, Zap, Play, Loader2, CheckCircle2, AlertCircle, Server,
  Tag, BarChart3, Plus, Trash2, Check, ArrowRight, Sliders, Info, HelpCircle,
  Eye, EyeOff, Shield
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';

interface CustomOidTestAndAddModalProps {
  onClose: () => void;
  initialDeviceId?: string;
}

// 常用 OID 预设
const PRESET_OIDS = [
  { label: '系统描述 sysDescr', oid: '.1.3.6.1.2.1.1.1.0', name: '系统描述', type: 'get', unit: '', isTable: 0 },
  { label: '系统运行时间 sysUpTime', oid: '.1.3.6.1.2.1.1.3.0', name: '系统正常运行时间', type: 'get', unit: '', isTable: 0 },
  { label: '接口描述 ifDescr', oid: '.1.3.6.1.2.1.2.2.1.2', name: '接口名称列表', type: 'walk', unit: '', isTable: 1 },
  { label: '接口运营状态 ifOperStatus', oid: '.1.3.6.1.2.1.2.2.1.8', name: '接口链路状态', type: 'walk', unit: '', isTable: 1 },
  { label: '华为 CPU 使用率', oid: '.1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5', name: '华为CPU使用率', type: 'walk', unit: '%', isTable: 1 },
  { label: '华为 内存使用率', oid: '.1.3.6.1.4.1.2011.5.25.31.1.1.1.1.7', name: '华为内存使用率', type: 'walk', unit: '%', isTable: 1 },
  { label: '思科 CPU 利用率', oid: '.1.3.6.1.4.1.9.9.109.1.1.1.1.5', name: '思科CPU利用率', type: 'walk', unit: '%', isTable: 1 },
  { label: '华三 CPU 使用率', oid: '.1.3.6.1.4.1.25506.2.6.1.1.1.1.6', name: '华三CPU利用率', type: 'walk', unit: '%', isTable: 1 },
];

const UNIT_OPTIONS = ['%', '℃', 'MB', 'GB', 'Mbps', 'Gbps', 'rpm', 'V', 'A', 'W', '个', 'ms', ''];

const PRESET_TAG_OPTIONS = [
  { key: 'env', val: 'production' },
  { key: 'env', val: 'staging' },
  { key: 'cluster', val: 'core-net' },
  { key: 'cluster', val: 'access-net' },
  { key: 'role', val: 'core-switch' },
  { key: 'role', val: 'border-router' },
  { key: 'location', val: 'IDC-1' },
  { key: 'location', val: 'IDC-2' },
  { key: 'metric_label', val: 'CPU使用率' },
  { key: 'metric_label', val: '内存使用率' },
];

export default function CustomOidTestAndAddModal({
  onClose,
  initialDeviceId,
}: CustomOidTestAndAddModalProps) {
  const toast = useToast();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  useEscapeKey({ onEscape: onClose });

  // 1. OID 与采集配置
  const [oid, setOid] = useState('');
  const [customName, setCustomName] = useState('');
  const [collectType, setCollectType] = useState<'get' | 'walk'>('walk');
  const [unit, setUnit] = useState('%');

  // 2. 目标设备与连接参数
  const [selectedDeviceId, setSelectedDeviceId] = useState<string>(initialDeviceId || '');
  const [overrideCredentials, setOverrideCredentials] = useState(false);
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

  // 3. 测试结果与表量实例选中
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<any | null>(null);
  const [selectedWalkRow, setSelectedWalkRow] = useState<number | null>(null);
  const [sendMode, setSendMode] = useState<'selected' | 'all'>('selected');

  // 4. 下发到设备指标配置（Tag 必须填写）
  const [metricName, setMetricName] = useState('');
  const [tags, setTags] = useState<Array<{ key: string; value: string }>>([
    { key: 'env', value: 'production' },
  ]);
  const [chartType, setChartType] = useState('auto');
  const [submitting, setSubmitting] = useState(false);

  // 获取所有网络设备
  const { data: networkDevices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
  });

  // 获取所有已存 SNMP 凭据
  const { data: savedCredentials = [] } = useQuery({
    queryKey: ['snmp-credentials'],
    queryFn: () => api.get('/snmp/credentials').then(res => res.data.data || []),
  });

  // 当选择设备变化时，自动设置默认所选设备
  useEffect(() => {
    if (!selectedDeviceId && networkDevices.length > 0) {
      setSelectedDeviceId(networkDevices[0].id);
    }
  }, [networkDevices, selectedDeviceId]);

  // 当前选中的设备对象
  const currentDevice = useMemo(() => {
    return networkDevices.find((d: any) => d.id === selectedDeviceId) || null;
  }, [networkDevices, selectedDeviceId]);

  // 切换常用 OID 预设
  const handleApplyPreset = (preset: typeof PRESET_OIDS[0]) => {
    setOid(preset.oid);
    setCustomName(preset.name);
    setCollectType(preset.type as 'get' | 'walk');
    if (preset.unit) setUnit(preset.unit);
    // 自动生成 metricName 建议
    const cleanMetric = preset.name.toLowerCase().replace(/[^a-z0-9_]/g, '_');
    if (!metricName) {
      setMetricName(cleanMetric || 'custom_metric');
    }
    toast.success(`已载入预设: ${preset.label}`);
  };

  // 添加 Tag 行
  const handleAddTag = () => {
    setTags([...tags, { key: '', value: '' }]);
  };

  // 删除 Tag 行
  const handleRemoveTag = (index: number) => {
    setTags(tags.filter((_, i) => i !== index));
  };

  // 更新 Tag
  const handleUpdateTag = (index: number, field: 'key' | 'value', val: string) => {
    const next = [...tags];
    next[index][field] = val;
    setTags(next);
  };

  // 应用快捷 Tag 预设
  const handleApplyTagPreset = (presetKey: string, presetVal: string) => {
    const existing = tags.find(t => t.key === presetKey);
    if (existing) {
      setTags(tags.map(t => t.key === presetKey ? { ...t, value: presetVal } : t));
    } else {
      setTags([...tags, { key: presetKey, value: presetVal }]);
    }
  };

  // 1. 发起测试采集
  const handleTestCollect = async () => {
    if (!oid.trim()) {
      toast.error('请输入需要采集测试的 OID');
      return;
    }
    if (!selectedDeviceId) {
      toast.error('请选择测试目标设备');
      return;
    }

    setTesting(true);
    setTestResult(null);

    try {
      const isTable = collectType === 'walk' ? 1 : 0;
      const resp = await api.post('/snmp-collect/quick-test', {
        device_ids: [selectedDeviceId],
        oids: [{
          oid: oid.trim(),
          metric_name: customName.trim() || oid.trim(),
          is_table: isTable,
        }],
        snmp_version: overrideCredentials ? snmpVersion : undefined,
        community: overrideCredentials && snmpVersion !== 'v3' ? community : undefined,
        snmp_credential_id: overrideCredentials ? (snmpCredentialId || undefined) : undefined,
        snmp_user: overrideCredentials && snmpVersion === 'v3' ? snmpUser : undefined,
        snmp_auth_protocol: overrideCredentials && snmpVersion === 'v3' ? snmpAuthProtocol : undefined,
        snmp_auth_key: overrideCredentials && snmpVersion === 'v3' ? snmpAuthKey : undefined,
        snmp_priv_protocol: overrideCredentials && snmpVersion === 'v3' ? snmpPrivProtocol : undefined,
        snmp_priv_key: overrideCredentials && snmpVersion === 'v3' ? snmpPrivKey : undefined,
        timeout_ms: snmpTimeout,
      });

      const resList = resp.data.data || [];
      if (resList.length > 0) {
        setTestResult(resList[0]);
        if (resList[0].status === 'success') {
          toast.success('SNMP 采集测试成功！');
          // 若 metricName 尚未填写，自动赋予默认名
          if (!metricName.trim()) {
            const raw = (customName || 'custom_metric').trim();
            setMetricName(raw.toLowerCase().replace(/[^a-z0-9_]/g, '_'));
          }
        } else {
          toast.warning(`测试失败: ${resList[0].error || '未能采集到数据'}`);
        }
      } else {
        toast.error('未收到设备探测响应');
      }
    } catch (err: any) {
      toast.error(err.response?.data?.error || '采集测试请求出错');
    } finally {
      setTesting(false);
    }
  };

  // 解析 Walk 表格数据项
  interface WalkItem {
    rawIndex: string;
    displayIndex: string;
    fullOid: string;
    value: string;
    rowNum: number;
    hasValue: boolean;
  }

  const parsedWalkItems = useMemo<WalkItem[]>(() => {
    if (!testResult?.metrics || testResult.metrics.length === 0) return [];
    const rawVal = testResult.metrics[0].metric_value || '';
    if (!rawVal || rawVal === 'N/A') return [];
    if (collectType !== 'walk') return [];

    const baseOidClean = oid.trim().replace(/^\.+/, '').replace(/\.+$/, '');
    // 若包含分号分隔的 key=val
    const parts = rawVal.split(';').map((s: string) => s.trim()).filter(Boolean);
    return parts.map((part: string, idx: number) => {
      let rawIndex = String(idx + 1);
      let value = part;
      const eqIdx = part.indexOf('=');
      if (eqIdx !== -1) {
        rawIndex = part.substring(0, eqIdx).trim();
        value = part.substring(eqIdx + 1).trim();
      }

      let displayIndex = rawIndex;
      let fullOid = '';

      const isAbsoluteOid = rawIndex.startsWith('.') || rawIndex.startsWith('1.');
      if (isAbsoluteOid) {
        const cleanRaw = rawIndex.replace(/^\.+/, '');
        fullOid = '.' + cleanRaw;
        if (baseOidClean && cleanRaw.startsWith(baseOidClean + '.')) {
          displayIndex = cleanRaw.substring(baseOidClean.length + 1);
        } else {
          const segments = cleanRaw.split('.');
          displayIndex = segments[segments.length - 1] || cleanRaw;
        }
      } else {
        displayIndex = rawIndex;
        fullOid = '.' + (baseOidClean ? `${baseOidClean}.${rawIndex}` : rawIndex);
      }

      const num = parseFloat(value);
      const hasValue = (!isNaN(num) && num !== 0) || (value !== '' && value !== '0' && value !== '0.0' && value !== 'N/A' && value !== 'null');

      return {
        rawIndex,
        displayIndex,
        fullOid,
        value,
        rowNum: idx + 1,
        hasValue,
      };
    });
  }, [testResult, collectType, oid]);

  // 当测试成功返回表量时，智能默认选中第一个具有有效值 (非0) 的实例行 (例如 华为AR 2048 MB)
  useEffect(() => {
    if (parsedWalkItems.length > 0) {
      const firstWithVal = parsedWalkItems.find(item => item.hasValue);
      if (firstWithVal) {
        setSelectedWalkRow(firstWithVal.rowNum);
      } else {
        setSelectedWalkRow(parsedWalkItems[0].rowNum);
      }
      setSendMode('selected');
    } else {
      setSelectedWalkRow(null);
    }
  }, [parsedWalkItems]);

  const selectedWalkItem = useMemo(() => {
    if (!selectedWalkRow || parsedWalkItems.length === 0) return null;
    return parsedWalkItems.find(item => item.rowNum === selectedWalkRow) || null;
  }, [selectedWalkRow, parsedWalkItems]);

  // 2. 校验并发送到具体设备添加自定义指标采集
  const handleDeployToDevice = async () => {
    if (!selectedDeviceId) {
      toast.error('请选择目标设备');
      return;
    }
    if (!oid.trim()) {
      toast.error('OID 不能为空');
      return;
    }
    if (!customName.trim()) {
      toast.error('请输入自定义指标显示名称');
      return;
    }

    // 🔴 必填 Prometheus 规范指标名校验
    const cleanMetricName = metricName.trim();
    if (!cleanMetricName) {
      toast.error('【必填】请输入 Prometheus 规范指标名 (如 huawei_cpu_usage)');
      return;
    }
    if (!/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(cleanMetricName)) {
      toast.error('指标名必须以字母或下划线开头，仅包含字母、数字和下划线');
      return;
    }

    // 🔴 必填 Tag 校验
    const validTags: Record<string, string> = {};
    const filledTags = tags.filter(t => t.key.trim() && t.value.trim());
    if (filledTags.length === 0) {
      toast.error('【必填要求】添加前必须至少输入 1 个有效的自定义 Tag 名与标签值！');
      return;
    }

    // 检查是否有半填的非法项
    for (const t of tags) {
      if ((t.key.trim() && !t.value.trim()) || (!t.key.trim() && t.value.trim())) {
        toast.error(`Tag 键值对不完整: "${t.key}"="${t.value}"，请填写完整或删除`);
        return;
      }
      if (t.key.trim() && t.value.trim()) {
        validTags[t.key.trim()] = t.value.trim();
      }
    }

    setSubmitting(true);
    try {
      const isTable = collectType === 'walk' ? 1 : 0;
      let lastVal = '';
      if (testResult?.metrics && testResult.metrics.length > 0) {
        lastVal = String(testResult.metrics[0].metric_value || '');
      }

      // 如果为表量并且选择了特定实例（默认行为，推荐）
      const isInstanceSelected = collectType === 'walk' && sendMode === 'selected' && selectedWalkItem;
      const targetOid = isInstanceSelected ? selectedWalkItem.fullOid : oid.trim();
      const targetIsTable = isInstanceSelected ? 0 : isTable;
      const targetSyntax = targetIsTable === 1 ? 'COLUMNAR' : 'SCALAR';
      const targetLastVal = isInstanceSelected ? selectedWalkItem.value : lastVal;

      const finalTags = { ...validTags };
      if (isInstanceSelected && !finalTags['table_index']) {
        finalTags['table_index'] = selectedWalkItem.displayIndex;
      }

      await api.post('/snmp-collect/dashboard-metrics', {
        oid: targetOid,
        custom_name: customName.trim(),
        original_name: customName.trim(),
        device_id: selectedDeviceId,
        module_name: 'CUSTOM-MIB',
        syntax: targetSyntax,
        unit: unit.trim(),
        is_table: targetIsTable,
        chart_type: chartType,
        metric_name: cleanMetricName,
        tags: JSON.stringify(finalTags),
        last_value: targetLastVal,
      });

      const succMsg = isInstanceSelected
        ? `自定义指标 "${customName}" (实例 #${selectedWalkItem.rowNum}: ${selectedWalkItem.value} ${unit}) 已成功下发并开启采集！`
        : `自定义指标 "${customName}" 已成功添加到设备并开启采集！`;
      toast.success(succMsg);
      queryClient.invalidateQueries({ queryKey: ['snmp-dashboard-metrics'] });
      queryClient.invalidateQueries({ queryKey: ['custom-dashboard-metrics'] });
      queryClient.invalidateQueries({ queryKey: ['metric-tags-all'] });
      onClose();
    } catch (err: any) {
      toast.error(err.response?.data?.error || '添加自定义指标采集失败');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 backdrop-blur-sm p-4 overflow-y-auto" onClick={onClose}>
      <div
        className="bg-surface border border-border rounded-2xl max-w-3xl w-full p-6 shadow-2xl space-y-6 my-6 max-h-[92vh] overflow-y-auto"
        onClick={e => e.stopPropagation()}
      >
        {/* 标题 */}
        <div className="flex items-start justify-between border-b border-border pb-4">
          <div>
            <h2 className="text-xl font-bold text-text-primary flex items-center gap-2.5">
              <span className="p-2 rounded-xl bg-gradient-to-br from-purple-500/20 to-indigo-500/20 border border-purple-500/30 text-purple-600">
                <Zap className="w-5 h-5" />
              </span>
              自定义 OID 探测、测试与设备指标下发
            </h2>
            <p className="text-xs text-text-secondary mt-1">
              输入任意标量 (GET) 或表量 (WALK) OID，实时测试采集，并绑定自定义 Tag 标签后下发到目标设备监控
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 rounded-lg hover:bg-surface-2 text-text-secondary hover:text-text-primary transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* 1. OID 输入与采集模式选择 */}
        <div className="space-y-4 bg-surface-2/40 border border-border rounded-xl p-4">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-bold uppercase tracking-wider text-purple-600 flex items-center gap-1.5">
              <Sliders className="w-4 h-4" /> 1. 自定义 OID 与采集模式配置
            </h3>
            <span className="text-[11px] text-text-tertiary">支持标准 OID 及厂商私有 MIB 节点</span>
          </div>

          {/* 预设 OID 芯片 */}
          <div>
            <span className="text-[11px] text-text-secondary block mb-1.5 font-medium">常用 OID 快捷载入:</span>
            <div className="flex flex-wrap gap-1.5">
              {PRESET_OIDS.map((p) => (
                <button
                  key={p.oid}
                  type="button"
                  onClick={() => handleApplyPreset(p)}
                  className="px-2.5 py-1 rounded-lg bg-surface hover:bg-purple-500/10 border border-border hover:border-purple-500/30 text-xs text-text-secondary hover:text-purple-300 transition-all flex items-center gap-1"
                >
                  <span className="font-medium">{p.label}</span>
                  <span className="text-[10px] px-1 rounded bg-purple-500/10 text-purple-600 font-mono">
                    {p.type.toUpperCase()}
                  </span>
                </button>
              ))}
            </div>
          </div>

          {/* OID 与 名称 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-semibold text-text-primary mb-1">
                OID 节点号 <span className="text-rose-400">*</span>
              </label>
              <input
                type="text"
                value={oid}
                onChange={e => setOid(e.target.value)}
                placeholder="例如: .1.3.6.1.4.1.2011.5.25.31.1.1.1.1.5"
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary font-mono focus:outline-none focus:border-purple-500 transition-colors"
              />
            </div>

            <div>
              <label className="block text-xs font-semibold text-text-primary mb-1">
                指标显示名称 <span className="text-rose-400">*</span>
              </label>
              <input
                type="text"
                value={customName}
                onChange={e => setCustomName(e.target.value)}
                placeholder="例如: 华为核心板卡CPU使用率"
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-purple-500 transition-colors"
              />
            </div>
          </div>

          {/* 采集模式 (GET vs WALK) 与 单位 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
            {/* GET vs WALK 单选切换 */}
            <div>
              <label className="block text-xs font-semibold text-text-primary mb-1.5">
                采集方式 (标量 vs 表量) <span className="text-rose-400">*</span>
              </label>
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  onClick={() => setCollectType('get')}
                  className={`px-3 py-2 rounded-lg border text-xs font-medium flex items-center justify-center gap-2 transition-all ${
                    collectType === 'get'
                      ? 'bg-purple-600/20 border-purple-500 text-purple-300 font-bold shadow-sm'
                      : 'bg-surface border-border text-text-secondary hover:text-text-primary'
                  }`}
                >
                  <span>📌 标量 (SNMP GET)</span>
                </button>

                <button
                  type="button"
                  onClick={() => setCollectType('walk')}
                  className={`px-3 py-2 rounded-lg border text-xs font-medium flex items-center justify-center gap-2 transition-all ${
                    collectType === 'walk'
                      ? 'bg-purple-600/20 border-purple-500 text-purple-300 font-bold shadow-sm'
                      : 'bg-surface border-border text-text-secondary hover:text-text-primary'
                  }`}
                >
                  <span>📊 表量 (SNMP WALK)</span>
                </button>
              </div>
              <p className="text-[11px] text-text-tertiary mt-1">
                {collectType === 'get'
                  ? '适用于单值标量（如系统总时间、单板状态），系统自动兼容 .0 节点'
                  : '适用于列族表量（如多接口列表、各核 CPU、多风扇转速），递归遍历整列'}
              </p>
            </div>

            {/* 单位 */}
            <div>
              <label className="block text-xs font-semibold text-text-primary mb-1.5">
                数据单位 (Unit)
              </label>
              <div className="flex gap-2">
                <select
                  value={unit}
                  onChange={e => setUnit(e.target.value)}
                  className="px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-purple-500 w-28"
                >
                  {UNIT_OPTIONS.map(u => (
                    <option key={u} value={u}>{u || '(无单位)'}</option>
                  ))}
                </select>
                <input
                  type="text"
                  value={unit}
                  onChange={e => setUnit(e.target.value)}
                  placeholder="或自定义单位"
                  className="flex-1 px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-purple-500"
                />
              </div>
            </div>
          </div>
        </div>

        {/* 2. 目标测试设备 */}
        <div className="space-y-3 bg-surface-2/40 border border-border rounded-xl p-4">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-bold uppercase tracking-wider text-blue-600 flex items-center gap-1.5">
              <Server className="w-4 h-4" /> 2. 测试目标设备与 SNMP 凭据
            </h3>
            <label className="flex items-center gap-1.5 text-xs text-text-secondary cursor-pointer">
              <input
                type="checkbox"
                checked={overrideCredentials}
                onChange={e => setOverrideCredentials(e.target.checked)}
                className="rounded border-border"
              />
              自定义覆盖 SNMP 参数
            </label>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-semibold text-text-primary mb-1">选择已纳管设备</label>
              <select
                value={selectedDeviceId}
                onChange={e => setSelectedDeviceId(e.target.value)}
                className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500"
              >
                {networkDevices.map((d: any) => (
                  <option key={d.id} value={d.id}>
                    {d.name} ({d.ip_address}) - {d.vendor?.toUpperCase() || 'NET'}
                  </option>
                ))}
              </select>
            </div>

            {currentDevice && (
              <div className="p-2.5 bg-surface rounded-lg border border-border flex items-center justify-between text-xs">
                <div>
                  <span className="text-text-secondary block">当前设备信息</span>
                  <span className="font-semibold text-text-primary">{currentDevice.name}</span>
                  <span className="font-mono text-text-secondary ml-2">({currentDevice.ip_address})</span>
                </div>
                <span className="px-2 py-0.5 rounded bg-blue-500/10 text-blue-600 font-semibold uppercase">
                  {currentDevice.vendor || 'SNMP'}
                </span>
              </div>
            )}
          </div>

          {overrideCredentials && (
            <div className="space-y-3 pt-2 border-t border-border/50">
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs text-text-secondary mb-1">SNMP 版本</label>
                  <select
                    value={snmpVersion}
                    onChange={e => setSnmpVersion(e.target.value)}
                    className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary cursor-pointer"
                  >
                    <option value="v2c">SNMPv2c</option>
                    <option value="v3">SNMPv3</option>
                    <option value="v1">SNMPv1</option>
                  </select>
                </div>
                {savedCredentials.length > 0 && (
                  <div>
                    <label className="block text-xs text-text-secondary mb-1">选择已存凭据</label>
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
                      className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-blue-600 focus:outline-none focus:border-blue-500 cursor-pointer truncate"
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
                  <label className="block text-xs text-text-secondary mb-1">超时时间 (ms)</label>
                  <input
                    type="number"
                    value={snmpTimeout}
                    onChange={e => setSnmpTimeout(Number(e.target.value))}
                    className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary"
                  />
                </div>
              </div>

              {snmpVersion !== 'v3' ? (
                <div>
                  <label className="block text-xs text-text-secondary mb-1">团体字 (Community)</label>
                  <input
                    type="text"
                    value={community}
                    onChange={e => setCommunity(e.target.value)}
                    className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-cyan-300"
                  />
                </div>
              ) : (
                <div className="bg-[#090d1a] border border-blue-500/20 rounded-lg p-3 space-y-2.5">
                  <div>
                    <label className="block text-xs font-semibold text-text-secondary mb-1">
                      安全用户名 (Username) <span className="text-red-600">*</span>
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
                        className="w-full px-2.5 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500"
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
                          className="w-full pl-2.5 pr-7 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-text-primary focus:outline-none focus:border-blue-500"
                        />
                        <button
                          type="button"
                          onClick={() => setShowAuthKey(!showAuthKey)}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text-primary"
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
                        className="w-full px-2.5 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500"
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
                          className="w-full pl-2.5 pr-7 py-1.5 bg-surface border border-border rounded-lg text-xs font-mono text-text-primary focus:outline-none focus:border-blue-500"
                        />
                        <button
                          type="button"
                          onClick={() => setShowPrivKey(!showPrivKey)}
                          className="absolute right-2 top-1/2 -translate-y-1/2 text-text-secondary hover:text-text-primary"
                        >
                          {showPrivKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* 实时探测按钮 */}
          <div className="pt-2 flex justify-end">
            <button
              type="button"
              onClick={handleTestCollect}
              disabled={testing || !oid.trim() || !selectedDeviceId}
              className="px-5 py-2.5 bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 text-white rounded-xl text-xs font-bold transition-all shadow-md flex items-center gap-2 disabled:opacity-50"
            >
              {testing ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4 fill-current" />}
              {testing ? '正在向设备发起 SNMP 探测...' : '🚀 立即发起测试采集 (Test)'}
            </button>
          </div>
        </div>

        {/* 3. 测试结果面板 */}
        {testResult && (
          <div className="space-y-3 bg-background border border-border rounded-xl p-4 shadow-inner">
            <div className="flex items-center justify-between border-b border-border pb-2">
              <h3 className="text-xs font-bold uppercase tracking-wider text-emerald-600 flex items-center gap-1.5">
                <CheckCircle2 className="w-4 h-4" /> 3. 实时采集测试返回结果
              </h3>
              <div className="flex items-center gap-2">
                <span className="text-xs font-mono text-text-secondary">
                  耗时: <strong className="text-emerald-600">{testResult.durationMs}ms</strong>
                </span>
                <span className={`px-2 py-0.5 rounded text-xs font-bold ${
                  testResult.status === 'success' ? 'bg-emerald-500/20 text-emerald-600 border border-emerald-500/30' : 'bg-rose-500/20 text-rose-400 border border-rose-500/30'
                }`}>
                  {testResult.status === 'success' ? '✅ 探测成功' : '❌ 探测失败'}
                </span>
              </div>
            </div>

            {testResult.status === 'failed' && (
              <div className="p-3 bg-rose-500/10 border border-rose-500/30 rounded-lg text-xs text-rose-400 space-y-1">
                <p className="font-bold">错误原因: {testResult.error || '未获取到任何返回数据'}</p>
                <p className="text-[11px] text-rose-300/80">
                  排查建议：请确认设备 IP 是否在线、SNMP 端口 161 是否放行、团体字是否正确；若为表量 OID 请切换为「表量 (WALK)」重试。
                </p>
              </div>
            )}

            {testResult.status === 'success' && (
              <div className="space-y-3">
                {/* 标量 GET 结果 */}
                {collectType === 'get' && (
                  <div className="p-3 bg-surface rounded-lg border border-border">
                    <span className="text-[11px] text-text-secondary block mb-1">标量采样值 (Scalar Value):</span>
                    <div className="text-lg font-bold font-mono text-emerald-600 break-all">
                      {testResult.metrics?.[0]?.metric_value || '无返回值'} {unit}
                    </div>
                    <span className="text-[10px] text-text-tertiary font-mono mt-1 block">
                      返回节点: {testResult.metrics?.[0]?.oid || oid}
                    </span>
                  </div>
                )}

                {/* 表量 WALK 结果 */}
                {collectType === 'walk' && (
                  <div className="space-y-3">
                    <div className="flex items-center justify-between text-xs text-text-secondary">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-text-secondary">表量遍历实例 (Table Instances):</span>
                        <span className="font-mono text-purple-300 font-semibold px-2 py-0.5 rounded bg-purple-500/15 border border-purple-500/25">
                          共 {parsedWalkItems.length} 行数据
                        </span>
                      </div>
                      {parsedWalkItems.some(item => item.hasValue) && (
                        <span className="text-[11px] text-emerald-600 font-medium flex items-center gap-1">
                          <Check className="w-3.5 h-3.5" /> 已智能推荐含有效值的实例
                        </span>
                      )}
                    </div>

                    <div className="p-2.5 rounded-lg bg-surface border border-border text-[11px] text-text-secondary flex items-center justify-between">
                      <span className="flex items-center gap-1.5">
                        <Info className="w-3.5 h-3.5 text-purple-600 flex-shrink-0" />
                        <span>💡 点击下方任意一行即可<strong>选择该实例发送至大屏</strong>；有非零数值的行已用绿色高亮标识</span>
                      </span>
                      {selectedWalkItem && (
                        <div className="flex items-center gap-1.5">
                          <button
                            type="button"
                            onClick={() => setSendMode('selected')}
                            className={`px-2 py-0.5 rounded text-[10px] font-bold border transition-colors ${
                              sendMode === 'selected'
                                ? 'bg-purple-600 text-white border-purple-400'
                                : 'bg-surface-2 text-text-secondary border-border hover:text-text-primary'
                            }`}
                          >
                            🎯 仅发送选中行 (#{selectedWalkItem.rowNum})
                          </button>
                          <button
                            type="button"
                            onClick={() => setSendMode('all')}
                            className={`px-2 py-0.5 rounded text-[10px] font-bold border transition-colors ${
                              sendMode === 'all'
                                ? 'bg-purple-600 text-white border-purple-400'
                                : 'bg-surface-2 text-text-secondary border-border hover:text-text-primary'
                            }`}
                          >
                            📦 整表遍历 (WALK)
                          </button>
                        </div>
                      )}
                    </div>

                    {parsedWalkItems.length > 0 ? (
                      <div className="max-h-52 overflow-y-auto rounded-xl border border-border bg-surface divide-y divide-border">
                        {parsedWalkItems.map((item) => {
                          const isSelected = selectedWalkRow === item.rowNum && sendMode === 'selected';
                          return (
                            <div
                              key={item.rowNum}
                              onClick={() => {
                                setSelectedWalkRow(item.rowNum);
                                setSendMode('selected');
                              }}
                              className={`flex items-center justify-between px-3 py-2 text-xs font-mono cursor-pointer transition-all ${
                                isSelected
                                  ? 'bg-purple-950/60 border-l-4 border-l-purple-500 text-white shadow-sm ring-1 ring-purple-500/30'
                                  : 'hover:bg-surface-hover text-text-secondary'
                              }`}
                            >
                              <div className="flex items-center gap-2.5 min-w-0">
                                <div className={`w-4 h-4 rounded-full flex items-center justify-center border transition-colors ${
                                  isSelected ? 'border-purple-400 bg-purple-500 text-white' : 'border-border bg-surface-2'
                                }`}>
                                  {isSelected && <Check className="w-2.5 h-2.5 stroke-[3]" />}
                                </div>
                                <span className="font-semibold text-text-secondary">
                                  #{item.rowNum}
                                </span>
                                <span className="text-text-secondary truncate" title={item.fullOid}>
                                  (索引: <span className="text-purple-300 font-bold">{item.displayIndex}</span>)
                                </span>
                                {item.hasValue && (
                                  <span className="px-1.5 py-0.2 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-600 border border-emerald-500/30 flex items-center gap-0.5">
                                    ★ 有效值
                                  </span>
                                )}
                              </div>
                              <div className="flex items-center gap-2.5">
                                <span className={`font-bold font-mono ${item.hasValue ? 'text-emerald-600 text-sm' : 'text-text-tertiary'}`}>
                                  {item.value} {unit}
                                </span>
                                {isSelected ? (
                                  <span className="px-1.5 py-0.5 rounded text-[10px] font-bold bg-purple-500/30 text-purple-200 border border-purple-400/40">
                                    已选
                                  </span>
                                ) : (
                                  <span className="text-[10px] text-text-tertiary hover:text-text-secondary">
                                    选择
                                  </span>
                                )}
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    ) : (
                      <div className="p-3 bg-surface rounded-lg text-xs font-mono text-text-secondary break-all">
                        {testResult.metrics?.[0]?.metric_value || '无数据'}
                      </div>
                    )}

                    {/* 当前选中的实例信息横幅 */}
                    {selectedWalkItem && sendMode === 'selected' && (
                      <div className="p-3 bg-purple-950/40 border border-purple-500/30 rounded-xl flex items-center justify-between text-xs">
                        <div className="flex items-center gap-2">
                          <span className="p-1.5 rounded-lg bg-purple-500/20 text-purple-300 font-bold">🎯</span>
                          <div>
                            <span className="text-text-primary font-semibold">
                              已选择发送实例: <strong className="text-purple-300">#{selectedWalkItem.rowNum}</strong> (索引: <code className="text-cyan-300 font-mono">{selectedWalkItem.displayIndex}</code>)
                            </span>
                            <div className="text-[11px] text-text-secondary font-mono mt-0.5">
                              下发 OID: <span className="text-emerald-600">{selectedWalkItem.fullOid}</span>
                            </div>
                          </div>
                        </div>
                        <div className="text-right">
                          <span className="text-[11px] text-text-secondary block">实时采样值</span>
                          <span className="text-sm font-bold font-mono text-emerald-600">
                            {selectedWalkItem.value} {unit}
                          </span>
                        </div>
                      </div>
                    )}
                  </div>
                )}
              </div>
            )}
          </div>
        )}

        {/* 4. 💾 发送到具体设备添加自定义指标采集 (必须输入自定义 Tag) */}
        <div className="space-y-4 bg-surface-2/50 border-2 border-dashed border-purple-500/40 rounded-xl p-4">
          <div className="flex items-center justify-between">
            <h3 className="text-xs font-bold uppercase tracking-wider text-purple-600 flex items-center gap-1.5">
              <Tag className="w-4 h-4 text-purple-600" /> 4. 发送到具体设备添加自定义指标采集 (添加前必须输入自定义 Tag)
            </h3>
            <span className="text-[11px] px-2 py-0.5 rounded-full bg-rose-500/10 text-rose-400 border border-rose-500/30 font-semibold">
              Tag 标签为必填项
            </span>
          </div>

          {/* 若选择了指定实例，展示高亮提示 */}
          {collectType === 'walk' && selectedWalkItem && sendMode === 'selected' && (
            <div className="p-3 bg-purple-950/40 border border-purple-500/30 rounded-xl flex items-center justify-between text-xs">
              <div className="flex items-center gap-2">
                <span className="p-1 rounded bg-purple-500/20 text-purple-300 font-bold">🎯</span>
                <span className="text-text-secondary">
                  已切换为<strong>指定实例标量采集</strong>：实例 <strong className="text-purple-300">#{selectedWalkItem.rowNum} (索引: {selectedWalkItem.displayIndex})</strong>，
                  当前值: <strong className="text-emerald-600 font-mono">{selectedWalkItem.value} {unit}</strong>
                  （下发 OID: <code className="text-cyan-300 font-mono">{selectedWalkItem.fullOid}</code>）
                </span>
              </div>
            </div>
          )}

          {/* Prometheus 规范指标名 */}
          <div>
            <label className="block text-xs font-bold text-text-primary mb-1">
              Prometheus 英文指标名 (Metric Identifier) <span className="text-rose-400">*</span>
            </label>
            <input
              type="text"
              value={metricName}
              onChange={e => setMetricName(e.target.value)}
              placeholder="例如: huawei_custom_cpu_usage 或 device_fan_speed"
              className="w-full px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary font-mono focus:outline-none focus:border-purple-500 transition-colors"
            />
            <p className="text-[11px] text-text-tertiary mt-1">
              用于 Prometheus 与 PromQL 告警规则匹配（如 <code className="text-purple-300 font-mono">{metricName || 'metric'}{`{env="production"} > 80`}</code>）
            </p>
          </div>

          {/* 自定义 Tag 标签配置（必须输入 Tag） */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="block text-xs font-bold text-text-primary">
                自定义 Tag 标签 (Tags Dimensions) <span className="text-rose-400">* (必填，至少 1 项)</span>
              </label>
              <button
                type="button"
                onClick={handleAddTag}
                className="text-xs text-purple-600 hover:text-purple-300 flex items-center gap-1 font-semibold transition-colors"
              >
                <Plus className="w-3.5 h-3.5" /> 添加新 Tag 标签
              </button>
            </div>

            {/* 常用 Tag 预设快速点击 */}
            <div className="flex flex-wrap items-center gap-1 pb-1">
              <span className="text-[10px] text-text-tertiary mr-1 font-medium">常用标签快速填充:</span>
              {PRESET_TAG_OPTIONS.map((pt, idx) => (
                <button
                  key={idx}
                  type="button"
                  onClick={() => handleApplyTagPreset(pt.key, pt.val)}
                  className="px-2 py-0.5 rounded bg-surface hover:bg-purple-500/10 border border-border hover:border-purple-500/30 text-[11px] text-text-secondary hover:text-purple-300 font-mono transition-all"
                >
                  +{pt.key}="{pt.val}"
                </button>
              ))}
            </div>

            {/* Tag 动态列表 */}
            <div className="space-y-2">
              {tags.map((t, idx) => (
                <div key={idx} className="flex items-center gap-2">
                  <input
                    type="text"
                    value={t.key}
                    onChange={e => handleUpdateTag(idx, 'key', e.target.value)}
                    placeholder="Tag 名 (如 env, cluster, role, location)"
                    className="w-1/3 px-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary font-mono focus:outline-none focus:border-purple-500"
                  />
                  <span className="text-text-tertiary font-mono">=</span>
                  <input
                    type="text"
                    value={t.value}
                    onChange={e => handleUpdateTag(idx, 'value', e.target.value)}
                    placeholder="Tag 值 (如 production, core-net, IDC-1)"
                    className="flex-1 px-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary font-mono focus:outline-none focus:border-purple-500"
                  />
                  {tags.length > 1 && (
                    <button
                      type="button"
                      onClick={() => handleRemoveTag(idx)}
                      className="p-1.5 text-text-tertiary hover:text-rose-400 hover:bg-rose-500/10 rounded-md transition-colors"
                      title="删除此 Tag"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  )}
                </div>
              ))}
            </div>
          </div>

          {/* 图表展示类型 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-1">
            <div>
              <label className="block text-xs font-semibold text-text-primary mb-1">大屏展示图表类型</label>
              <select
                value={chartType}
                onChange={e => setChartType(e.target.value)}
                className="w-full px-3 py-1.5 bg-surface border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-purple-500"
              >
                <option value="auto">自动推荐 (Auto)</option>
                <option value="gauge">仪表盘 (Gauge)</option>
                <option value="line">折线趋势图 (Line)</option>
                <option value="status">状态卡片 (Status)</option>
              </select>
            </div>
            <div className="flex items-end">
              <p className="text-[11px] text-text-tertiary leading-relaxed">
                下发成功后，系统将自动把该 OID 纳入该设备的后台轮询调度，并在「网络大屏」与「指标面板」中实时呈现。
              </p>
            </div>
          </div>
        </div>

        {/* 底部确认下发操作栏 */}
        <div className="flex items-center justify-between pt-3 border-t border-border">
          <div className="text-xs text-text-secondary">
            目标设备: <strong className="text-text-primary">{currentDevice?.name || '未选设备'}</strong>
          </div>

          <div className="flex items-center gap-2.5">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 bg-surface border border-border rounded-xl text-xs font-medium text-text-secondary hover:text-text-primary transition-colors"
            >
              取消
            </button>

            <button
              type="button"
              onClick={handleDeployToDevice}
              disabled={submitting || !oid.trim() || !customName.trim() || !metricName.trim() || !selectedDeviceId}
              className="px-6 py-2.5 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 text-white rounded-xl text-xs font-bold transition-all shadow-lg flex items-center gap-2 disabled:opacity-50"
            >
              {submitting ? <Loader2 className="w-4 h-4 animate-spin" /> : <Check className="w-4 h-4" />}
              {submitting ? '正在写入设备自定义指标...' : '💾 确认发送到设备添加指标采集'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
