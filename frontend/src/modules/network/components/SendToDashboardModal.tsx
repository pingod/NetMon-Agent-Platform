/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  Monitor, Settings, Plus, Trash2, X, Zap, Loader2, ArrowRight, Server, Check
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';
import { cleanDisplayValue } from '../utils/metricHelpers';

const VENDOR_LABELS: Record<string, string> = {
  huawei: '华为',
  h3c: '华三',
  cisco: '思科',
  ruijie: '锐捷',
  zte: '中兴',
  maipu: '迈普',
  fortinet: '飞塔',
  juniper: '瞻博',
  other: '其他',
};

const UNIT_PRESETS = [
  { label: '%', value: '%' },
  { label: '℃', value: '℃' },
  { label: 'MB', value: 'MB' },
  { label: 'GB', value: 'GB' },
  { label: 'Mbps', value: 'Mbps' },
  { label: 'Gbps', value: 'Gbps' },
  { label: 'ms', value: 'ms' },
  { label: '个', value: '个' },
  { label: 'rpm', value: 'rpm' },
  { label: 'V', value: 'V' },
  { label: 'A', value: 'A' },
  { label: 'W', value: 'W' },
];

const VALUE_MAP_PRESETS = [
  {
    name: '1=正常 / 2=告警 / 0=离线',
    mappings: [
      { key: '1', value: '正常' },
      { key: '2', value: '告警' },
      { key: '0', value: '离线' },
    ],
  },
  {
    name: '1=UP / 2=DOWN',
    mappings: [
      { key: '1', value: 'UP' },
      { key: '2', value: 'DOWN' },
    ],
  },
  {
    name: '0=OK / 1=Critical',
    mappings: [
      { key: '0', value: 'OK' },
      { key: '1', value: 'Critical' },
    ],
  },
  {
    name: '1=启用 / 0=禁用',
    mappings: [
      { key: '1', value: '启用' },
      { key: '0', value: '禁用' },
    ],
  },
];

export interface SendToDashboardModalProps {
  initialData: {
    oid: string;
    customName?: string;
    metricName?: string;
    tags?: string;
    deviceId: string;
    deviceName?: string;
    deviceIp?: string;
    lastValue?: string;
    unit?: string;
    snmpVersion?: string;
    community?: string;
    is_table?: number;
    chartType?: string;
  };
  onClose: () => void;
  onSuccess?: () => void;
}

export default function SendToDashboardModal({
  initialData,
  onClose,
  onSuccess,
}: SendToDashboardModalProps) {
  const toast = useToast();
  const queryClient = useQueryClient();
  useEscapeKey({ onEscape: onClose });

  const [customName, setCustomName] = useState(
    initialData.customName || initialData.oid
  );
  const [metricName] = useState(initialData.metricName || '');
  const [tags] = useState(initialData.tags || '');
  const [unit, setUnit] = useState(initialData.unit || '');
  const [chartType, setChartType] = useState(initialData.chartType || 'circle');
  const [mappings, setMappings] = useState<Array<{ key: string; value: string }>>([]);
  const [saving, setSaving] = useState(false);

  // 表量多实例解析
  const rawTableParts = initialData.lastValue && typeof initialData.lastValue === 'string' && initialData.lastValue.includes(';')
    ? initialData.lastValue.split(';').map(s => s.trim()).filter(Boolean)
    : [];

  const parsedInstances = rawTableParts.map((part, idx) => {
    let rawIndex = String(idx + 1);
    let val = part;
    if (part.includes('=')) {
      const eqIdx = part.indexOf('=');
      rawIndex = part.substring(0, eqIdx).trim();
      val = part.substring(eqIdx + 1).trim();
    }
    const cleanBase = (initialData.oid || '').trim().replace(/^\.+/, '').replace(/\.+$/, '');
    let displayIndex = rawIndex;
    let fullOid = '';
    if (rawIndex.startsWith('.') || rawIndex.startsWith('1.')) {
      const cleanRaw = rawIndex.replace(/^\.+/, '');
      fullOid = '.' + cleanRaw;
      if (cleanBase && cleanRaw.startsWith(cleanBase + '.')) {
        displayIndex = cleanRaw.substring(cleanBase.length + 1);
      } else {
        const segs = cleanRaw.split('.');
        displayIndex = segs[segs.length - 1] || cleanRaw;
      }
    } else {
      displayIndex = rawIndex;
      fullOid = '.' + (cleanBase ? `${cleanBase}.${rawIndex}` : rawIndex);
    }
    const num = parseFloat(val);
    const hasValue = (!isNaN(num) && num !== 0) || (val !== '' && val !== '0' && val !== '0.0' && val !== 'N/A');
    return { rowNum: idx + 1, rawIndex, displayIndex, fullOid, value: val, hasValue };
  });

  const [selectedInstanceRow, setSelectedInstanceRow] = useState<number | null>(() => {
    if (parsedInstances.length === 0) return null;
    const firstWithVal = parsedInstances.find(i => i.hasValue);
    return firstWithVal ? firstWithVal.rowNum : parsedInstances[0].rowNum;
  });

  // 设备复用多选状态
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(
    new Set(initialData.deviceId ? [initialData.deviceId] : [])
  );
  const [modalVendorFilter, setModalVendorFilter] = useState<string>('all');

  // 获取全部网络设备
  const { data: devices = [] } = useQuery({
    queryKey: ['network-devices-for-modal'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
    staleTime: 60000,
  });

  const initialDevice = devices.find((d: any) => d.id === initialData.deviceId);
  const initialVendor = initialDevice?.vendor || '';

  const addMappingRow = () => setMappings([...mappings, { key: '', value: '' }]);
  const removeMappingRow = (index: number) =>
    setMappings(mappings.filter((_, i) => i !== index));
  const updateMappingRow = (index: number, field: 'key' | 'value', val: string) => {
    setMappings(mappings.map((m, i) => (i === index ? { ...m, [field]: val } : m)));
  };

  const applyPresetMapping = (presetMappings: Array<{ key: string; value: string }>) => {
    setMappings(presetMappings.map(m => ({ ...m })));
  };

  const toggleDevice = (id: string) => {
    const s = new Set(selectedDeviceIds);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedDeviceIds(s);
  };

  const selectSameVendorDevices = () => {
    const targetVendor = initialVendor || modalVendorFilter;
    if (!targetVendor || targetVendor === 'all') {
      setSelectedDeviceIds(new Set(devices.map((d: any) => d.id)));
      return;
    }
    const matching = devices.filter((d: any) => (d.vendor || '').toLowerCase() === targetVendor.toLowerCase());
    const s = new Set(selectedDeviceIds);
    matching.forEach((d: any) => s.add(d.id));
    setSelectedDeviceIds(s);
  };

  const selectAllModalDevices = () => {
    if (selectedDeviceIds.size === devices.length) {
      setSelectedDeviceIds(new Set());
    } else {
      setSelectedDeviceIds(new Set(devices.map((d: any) => d.id)));
    }
  };

  const handleSave = async () => {
    if (!customName.trim()) {
      toast.error('请输入指标名称');
      return;
    }
    if (selectedDeviceIds.size === 0) {
      toast.error('请至少选择一台关联网络设备');
      return;
    }

    // 格式化值映射为 JSON 字符串
    const mappingObj: Record<string, string> = {};
    for (const m of mappings) {
      if (m.key.trim()) {
        mappingObj[m.key.trim()] = m.value.trim();
      }
    }
    const valueMappingsStr =
      Object.keys(mappingObj).length > 0 ? JSON.stringify(mappingObj) : '';

    const deviceIdList = Array.from(selectedDeviceIds);
    setSaving(true);
    try {
      const chosenInst = selectedInstanceRow ? parsedInstances.find(i => i.rowNum === selectedInstanceRow) : null;
      const finalOid = chosenInst ? chosenInst.fullOid : initialData.oid;
      const finalIsTable = chosenInst ? 0 : (initialData.is_table || 0);
      const finalLastVal = chosenInst ? chosenInst.value : (initialData.lastValue || '');

      if (deviceIdList.length === 1) {
        await api.post('/snmp-collect/dashboard-metrics', {
          oid: finalOid,
          custom_name: customName.trim(),
          original_name: initialData.customName || initialData.oid,
          device_id: deviceIdList[0],
          unit: unit.trim(),
          snmp_version: initialData.snmpVersion || 'v2c',
          community: initialData.community || 'public',
          value_mappings: valueMappingsStr,
          is_table: finalIsTable,
          chart_type: chartType,
          metric_name: metricName.trim(),
          tags: tags.trim(),
          last_value: finalLastVal,
        });
      } else {
        const metricsToCreate = deviceIdList.map(devId => ({
          oid: finalOid,
          custom_name: customName.trim(),
          original_name: initialData.customName || initialData.oid,
          device_id: devId,
          unit: unit.trim(),
          snmp_version: initialData.snmpVersion || 'v2c',
          community: initialData.community || 'public',
          value_mappings: valueMappingsStr,
          is_table: finalIsTable,
          chart_type: chartType,
          metric_name: metricName.trim(),
          tags: tags.trim(),
          last_value: devId === initialData.deviceId ? finalLastVal : '',
        }));
        await api.post('/snmp-collect/dashboard-metrics/batch', { metrics: metricsToCreate });
      }

      // 后台异步触发后台设备网络采样刷新，非阻塞立即关闭弹窗
      api.post('/snmp-collect/dashboard-metrics/refresh').catch(() => {});

      toast.success(`已成功发布指标 "${customName}" 到 ${deviceIdList.length} 台网络设备！`);
      queryClient.invalidateQueries({ queryKey: ['custom-dashboard-metrics'] });
      if (onSuccess) onSuccess();
      onClose();
    } catch (err: any) {
      const errMsg = err.response?.data?.message || err.response?.data?.error || err.message || '发送到监控大屏失败';
      toast.error(errMsg);
    } finally {
      setSaving(false);
    }
  };

  // 格式化当前值 + 单位预览
  const renderValuePreview = () => {
    const chosenInst = selectedInstanceRow ? parsedInstances.find(i => i.rowNum === selectedInstanceRow) : null;
    const rawVal = chosenInst ? chosenInst.value : cleanDisplayValue(initialData.lastValue);
    if (rawVal === '-' || rawVal === undefined || rawVal === null) return <span className="text-text-tertiary">暂无测试值</span>;

    const mappingObj: Record<string, string> = {};
    for (const m of mappings) {
      if (m.key.trim()) mappingObj[m.key.trim()] = m.value.trim();
    }
    const mapped = mappingObj[rawVal];

    if (mapped) {
      let badgeClass = 'bg-blue-500/20 text-blue-300 border-blue-500/30';
      if (/正常|online|up|ok|active|running/i.test(mapped)) {
        badgeClass = 'bg-emerald-500/20 text-emerald-300 border-emerald-500/30';
      } else if (/告警|warning|warn/i.test(mapped)) {
        badgeClass = 'bg-amber-500/20 text-amber-300 border-amber-500/30';
      } else if (/离线|down|error|fault|critical/i.test(mapped)) {
        badgeClass = 'bg-red-500/20 text-red-300 border-red-500/30';
      }
      return (
        <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded border text-xs font-semibold ${badgeClass}`}>
          <span className="w-1.5 h-1.5 rounded-full bg-current" />
          {mapped} <span className="text-[10px] opacity-60">({rawVal})</span>
        </span>
      );
    }

    return (
      <span className="font-mono text-emerald-400 font-bold text-sm">
        {rawVal} <span className="text-cyan-400 text-xs font-normal">{unit}</span>
      </span>
    );
  };

  const filteredDevicesForModal = devices.filter((d: any) =>
    modalVendorFilter === 'all' || (d.vendor || '').toLowerCase() === modalVendorFilter.toLowerCase()
  );

  return (
    <div
      className="fixed inset-0 z-[70] flex items-center justify-center bg-black/60 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        className="bg-[#0d1225] border border-blue-500/30 rounded-xl w-full max-w-xl max-h-[90vh] overflow-y-auto shadow-2xl"
        onClick={e => e.stopPropagation()}
      >
        {/* Modal 头部 */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-blue-500/20 sticky top-0 bg-[#0d1225] z-10">
          <div className="flex items-center gap-2">
            <div className="p-1.5 rounded-lg bg-blue-500/20 text-blue-400">
              <Monitor className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-base font-bold text-text-primary flex items-center gap-2">
                发送到网络监控大屏 (支持同厂商多设备复用)
              </h3>
              <p className="text-xs text-text-secondary">配置自定义符号与 Grafana 值映射，批量发布到多台设备</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 hover:bg-slate-800 rounded-lg text-text-secondary hover:text-slate-200 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Modal 主体 */}
        <div className="p-5 space-y-4">
          {/* 测试对象与 OID 卡片 */}
          <div className="p-3 bg-[#111836] border border-blue-500/15 rounded-lg text-xs space-y-1.5">
            <div className="flex items-center justify-between text-text-secondary">
              <span className="text-text-secondary">采集 OID:</span>
              <span className="font-mono text-amber-400 select-all">{initialData.oid}</span>
            </div>
            <div className="flex items-center justify-between pt-1 border-t border-blue-500/10">
              <span className="text-text-secondary">测试采样值:</span>
              <div>{renderValuePreview()}</div>
            </div>
          </div>

          {/* 表量多实例选择 */}
          {parsedInstances.length > 0 && (
            <div className="p-3 bg-purple-950/40 border border-purple-500/30 rounded-xl space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-bold text-purple-300 flex items-center gap-1.5">
                  🎯 选择下发到大屏的实例 (共 {parsedInstances.length} 个实例)
                </span>
                <span className="text-[10px] text-text-secondary">点击切换要绑定的实例</span>
              </div>
              <div className="max-h-36 overflow-y-auto rounded-lg border border-slate-800 bg-[#0d1225] divide-y divide-slate-800/60">
                {parsedInstances.map(inst => {
                  const isSelected = selectedInstanceRow === inst.rowNum;
                  return (
                    <div
                      key={inst.rowNum}
                      onClick={() => setSelectedInstanceRow(inst.rowNum)}
                      className={`flex items-center justify-between px-2.5 py-1.5 text-xs font-mono cursor-pointer transition-colors ${
                        isSelected ? 'bg-purple-900/40 border-l-2 border-purple-400 text-white' : 'hover:bg-slate-800/60 text-text-secondary'
                      }`}
                    >
                      <div className="flex items-center gap-2">
                        <span className="text-text-secondary">#{inst.rowNum} (索引: {inst.displayIndex})</span>
                        {inst.hasValue && (
                          <span className="px-1 py-0.2 rounded text-[10px] font-bold bg-emerald-500/20 text-emerald-400">★ 有效值</span>
                        )}
                      </div>
                      <div className="flex items-center gap-2">
                        <span className={`font-bold ${inst.hasValue ? 'text-emerald-400' : 'text-text-tertiary'}`}>{inst.value} {unit}</span>
                        {isSelected && <span className="text-[10px] text-purple-300 font-bold">已选</span>}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {/* 1. 指标名称 */}
          <div>
            <label className="block text-xs font-semibold text-text-secondary mb-1">
              1. 指标名称 <span className="text-red-400">*</span>
            </label>
            <input
              value={customName}
              onChange={e => setCustomName(e.target.value)}
              placeholder="如：核心交换机 - 内存使用率"
              className="w-full px-3 py-2 bg-[#111836] border border-blue-500/20 rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500"
            />
          </div>

          {/* 2. 自定义符号 / 单位 */}
          <div>
            <label className="block text-xs font-semibold text-text-secondary mb-1">
              2. 自定义符号 / 单位 <span className="text-text-tertiary font-normal">(如内存使用率填 %、温度填 ℃)</span>
            </label>
            <input
              value={unit}
              onChange={e => setUnit(e.target.value)}
              placeholder="输入任意自定义单位符号，如 %、MB、GB、℃、bps、ms 等"
              className="w-full px-3 py-2 bg-[#111836] border border-blue-500/20 rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500 mb-2"
            />
            <div className="flex flex-wrap gap-1.5">
              {UNIT_PRESETS.map(u => (
                <button
                  key={u.value}
                  type="button"
                  onClick={() => setUnit(u.value)}
                  className={`px-2 py-0.5 text-xs rounded border transition-colors ${
                    unit === u.value
                      ? 'bg-blue-500 text-white border-blue-400'
                      : 'bg-[#111836] text-text-secondary border-slate-700 hover:text-slate-200 hover:border-slate-500'
                  }`}
                >
                  {u.label}
                </button>
              ))}
              {unit && (
                <button
                  type="button"
                  onClick={() => setUnit('')}
                  className="px-2 py-0.5 text-xs text-text-tertiary hover:text-slate-300"
                >
                  清空单位
                </button>
              )}
            </div>
          </div>

          {/* 3. 大屏卡片展现样式 */}
          <div>
            <label className="block text-xs font-semibold text-text-secondary mb-1.5">
              3. 大屏卡片展现样式
            </label>
            <div className="grid grid-cols-3 gap-2">
              <button
                type="button"
                onClick={() => setChartType('circle')}
                className={`px-2.5 py-2 rounded-lg border text-xs font-bold flex items-center justify-center gap-1 transition-all cursor-pointer ${
                  chartType === 'circle' || chartType === 'auto'
                    ? 'bg-blue-500/25 border-blue-400 text-blue-300 shadow-md shadow-blue-500/10'
                    : 'bg-[#111836] border-blue-500/15 text-text-secondary hover:text-slate-200'
                }`}
              >
                <span>🍩 环形圆盘</span>
              </button>
              <button
                type="button"
                onClick={() => setChartType('bar')}
                className={`px-2.5 py-2 rounded-lg border text-xs font-bold flex items-center justify-center gap-1 transition-all cursor-pointer ${
                  chartType === 'bar'
                    ? 'bg-blue-500/25 border-blue-400 text-blue-300 shadow-md shadow-blue-500/10'
                    : 'bg-[#111836] border-blue-500/15 text-text-secondary hover:text-slate-200'
                }`}
              >
                <span>📊 柱状进度条</span>
              </button>
              <button
                type="button"
                onClick={() => setChartType('number')}
                className={`px-2.5 py-2 rounded-lg border text-xs font-bold flex items-center justify-center gap-1 transition-all cursor-pointer ${
                  chartType === 'number'
                    ? 'bg-blue-500/25 border-blue-400 text-blue-300 shadow-md shadow-blue-500/10'
                    : 'bg-[#111836] border-blue-500/15 text-text-secondary hover:text-slate-200'
                }`}
              >
                <span>🔢 数字卡片</span>
              </button>
            </div>
          </div>

          {/* 3. Grafana 风格值映射 */}
          <div className="pt-2 border-t border-blue-500/10">
            <div className="flex items-center justify-between mb-1">
              <label className="text-xs font-semibold text-text-secondary flex items-center gap-1.5">
                <Settings className="w-3.5 h-3.5 text-blue-400" />
                3. Grafana 风格值映射 <span className="text-text-tertiary font-normal">(可选)</span>
              </label>
              <button
                type="button"
                onClick={addMappingRow}
                className="flex items-center gap-1 px-2 py-0.5 text-xs text-blue-400 hover:bg-blue-500/10 rounded transition-colors"
              >
                <Plus className="w-3 h-3" />
                添加规则
              </button>
            </div>
            <p className="text-[11px] text-text-secondary mb-2">
              将数字/原始值映射为直观文本（如 1 映射为"正常"、2 映射为"告警"、0 映射为"离线"）
            </p>

            <div className="mb-2">
              <span className="text-[10px] text-text-tertiary block mb-1">快捷模板:</span>
              <div className="flex flex-wrap gap-1.5">
                {VALUE_MAP_PRESETS.map((p, idx) => (
                  <button
                    key={idx}
                    type="button"
                    onClick={() => applyPresetMapping(p.mappings)}
                    className="px-2 py-0.5 text-[11px] bg-[#111836] border border-blue-500/20 text-blue-300 rounded hover:bg-blue-500/20 transition-colors"
                  >
                    {p.name}
                  </button>
                ))}
              </div>
            </div>

            {mappings.length === 0 ? (
              <div className="p-2.5 text-center text-xs text-text-tertiary bg-[#111836]/60 rounded-lg border border-dashed border-slate-700">
                暂未设置值映射
              </div>
            ) : (
              <div className="space-y-1.5">
                {mappings.map((m, idx) => (
                  <div key={idx} className="flex items-center gap-2">
                    <input
                      value={m.key}
                      onChange={e => updateMappingRow(idx, 'key', e.target.value)}
                      placeholder="原始采集值 (如 1)"
                      className="w-1/3 px-2.5 py-1 bg-[#111836] border border-blue-500/20 rounded text-xs text-text-primary font-mono focus:outline-none focus:border-blue-500"
                    />
                    <ArrowRight className="w-3.5 h-3.5 text-text-tertiary flex-shrink-0" />
                    <input
                      value={m.value}
                      onChange={e => updateMappingRow(idx, 'value', e.target.value)}
                      placeholder="大屏映射显示 (如 正常)"
                      className="flex-1 px-2.5 py-1 bg-[#111836] border border-blue-500/20 rounded text-xs text-text-primary focus:outline-none focus:border-blue-500"
                    />
                    <button
                      type="button"
                      onClick={() => removeMappingRow(idx)}
                      className="p-1 text-text-tertiary hover:text-red-400 transition-colors"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* 4. 复用应用到网络设备多选 (关键新特性) */}
          <div className="pt-2 border-t border-blue-500/10">
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-xs font-semibold text-text-secondary flex items-center gap-1.5">
                <Server className="w-3.5 h-3.5 text-cyan-400" />
                4. 复用发布到网络设备 <span className="text-blue-400">({selectedDeviceIds.size}/{devices.length} 台)</span>
              </label>
              <div className="flex items-center gap-1.5">
                {initialVendor && (
                  <button
                    type="button"
                    onClick={selectSameVendorDevices}
                    className="px-2 py-0.5 text-[11px] bg-blue-500/20 text-blue-300 border border-blue-500/30 rounded hover:bg-blue-500/30 transition-colors"
                  >
                    全选同厂商({VENDOR_LABELS[initialVendor] || initialVendor})设备
                  </button>
                )}
                <button
                  type="button"
                  onClick={selectAllModalDevices}
                  className="px-2 py-0.5 text-[11px] bg-slate-800 text-text-secondary border border-slate-700 rounded hover:bg-slate-700 transition-colors"
                >
                  {selectedDeviceIds.size === devices.length ? '清空' : '全选所有'}
                </button>
              </div>
            </div>

            {/* 厂商筛选分类 Pills */}
            <div className="flex items-center gap-1 mb-2 overflow-x-auto pb-1">
              <span className="text-[10px] text-text-tertiary flex-shrink-0">按厂商筛选:</span>
              {['all', 'huawei', 'h3c', 'cisco', 'ruijie', 'zte', 'other'].map(v => (
                <button
                  key={v}
                  type="button"
                  onClick={() => setModalVendorFilter(v)}
                  className={`px-2 py-0.5 text-[10px] rounded transition-colors whitespace-nowrap ${
                    modalVendorFilter === v
                      ? 'bg-blue-500 text-white font-semibold'
                      : 'bg-[#111836] text-text-secondary hover:text-slate-200'
                  }`}
                >
                  {v === 'all' ? '全部' : (VENDOR_LABELS[v] || v)}
                </button>
              ))}
            </div>

            {/* 设备勾选列表 */}
            <div className="max-h-36 overflow-y-auto border border-blue-500/15 rounded-lg bg-[#111836] divide-y divide-blue-500/5">
              {filteredDevicesForModal.length === 0 ? (
                <p className="text-center py-3 text-xs text-text-tertiary">暂无符合条件的网络设备</p>
              ) : filteredDevicesForModal.map((d: any) => {
                const isSelected = selectedDeviceIds.has(d.id);
                const vLabel = VENDOR_LABELS[d.vendor] || d.vendor || '其他';
                return (
                  <div
                    key={d.id}
                    onClick={() => toggleDevice(d.id)}
                    className={`flex items-center justify-between px-3 py-1.5 cursor-pointer transition-colors ${
                      isSelected ? 'bg-blue-500/10' : 'hover:bg-blue-500/5'
                    }`}
                  >
                    <div className="flex items-center gap-2 min-w-0">
                      <div className={`w-3.5 h-3.5 rounded border flex items-center justify-center flex-shrink-0 ${
                        isSelected ? 'bg-blue-500 border-blue-500' : 'border-slate-600'
                      }`}>
                        {isSelected && <Check className="w-2.5 h-2.5 text-white" />}
                      </div>
                      <span className="text-xs text-text-primary truncate">{d.name}</span>
                      <span className="text-[10px] px-1.5 py-0.2 rounded bg-slate-800 text-text-secondary font-medium">
                        {vLabel}
                      </span>
                    </div>
                    <span className="text-[10px] text-amber-400 font-mono flex-shrink-0 ml-2">{d.ip_address}</span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* Modal 底部 */}
        <div className="flex items-center justify-between px-5 py-3 border-t border-blue-500/20 sticky bottom-0 bg-[#0d1225]">
          <div className="text-xs text-text-tertiary flex items-center gap-1">
            预览效果: {renderValuePreview()}
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={onClose}
              className="px-4 py-1.5 text-xs text-text-secondary hover:text-slate-200 transition-colors"
            >
              取消
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex items-center gap-1.5 px-4 py-1.5 bg-gradient-to-r from-blue-600 to-cyan-600 hover:from-blue-700 hover:to-cyan-700 text-white text-xs font-bold rounded-lg transition-all shadow-md shadow-blue-500/20 disabled:opacity-50"
            >
              {saving ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Zap className="w-3.5 h-3.5" />}
              发送/复用发布 ({selectedDeviceIds.size}台设备)
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
