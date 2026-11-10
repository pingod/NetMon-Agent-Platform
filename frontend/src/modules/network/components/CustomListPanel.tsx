/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo, useCallback } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  RefreshCw, Settings, Trash2, Plus, X, Table2, Loader2,
  ChevronUp, ChevronDown, ArrowUpDown, ExternalLink, Eye, BarChart3
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';
import { useNavigate } from 'react-router-dom';

// ============================================================
// 颜色预设
// ============================================================

const TEXT_COLOR_PRESETS = [
  { label: '默认', value: '' },
  { label: '淡蓝', value: '#73a6ff' },
  { label: '白色', value: '#ffffff' },
  { label: '黄色', value: '#ffd93d' },
  { label: '橙红', value: '#ff9f43' },
  { label: '红色', value: '#ff5757' },
  { label: '绿色', value: '#a3e667' },
  { label: '青色', value: '#06b6d4' },
  { label: '紫色', value: '#c084fc' },
  { label: '灰色', value: '#9ca3af' },
];

const BG_COLOR_PRESETS = [
  { label: '无', value: '' },
  { label: '绿色', value: '#67c267' },
  { label: '黄色', value: '#f5c518' },
  { label: '红色', value: '#ef4444' },
  { label: '蓝色', value: '#3b82f6' },
  { label: '紫色', value: '#8b5cf6' },
];

const LABEL_FIELD_OPTIONS = [
  { value: 'device_ip', label: '设备IP' },
  { value: 'device_name', label: '设备名称' },
  { value: 'device_vendor', label: '设备厂商' },
  { value: 'device_model', label: '设备型号' },
  { value: 'device_location', label: '设备位置' },
];

const ALIGN_OPTIONS = [
  { value: 'left', label: '左对齐' },
  { value: 'center', label: '居中' },
  { value: 'right', label: '右对齐' },
];

// ============================================================
// 状态徽章颜色映射
// ============================================================

function getStatusBadge(value: string, mappings?: Record<string, string>): { bg: string; text: string } | null {
  if (!mappings) return null;
  const display = mappings[value];
  if (!display) return null;
  const lower = display.toLowerCase();
  if (/正常|运行|online|up|ok|active/.test(lower)) return { bg: '#16a34a', text: '#ffffff' };
  if (/告警|warning|warn/.test(lower))              return { bg: '#d97706', text: '#ffffff' };
  if (/离线|down|error|fault|critical/.test(lower)) return { bg: '#dc2626', text: '#ffffff' };
  if (/未知|unknown/.test(lower))                   return { bg: '#6b7280', text: '#ffffff' };
  return null;
}

/** 根据百分比数值返回颜色 */
function getPercentColor(val: number): string {
  if (val >= 80) return '#ef4444';
  if (val >= 60) return '#f59e0b';
  return '#22c55e';
}

// ============================================================
// 主面板：自定义列表面板
// ============================================================

export default function CustomListPanel() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();
  const [refreshKey, setRefreshKey] = useState(0);
  const [editingList, setEditingList] = useState<any | null>(null);
  const [showEditModal, setShowEditModal] = useState(false);
  const [detailModal, setDetailModal] = useState<{ row: any; columns: any[]; listName: string } | null>(null);

  // 获取所有列表面板配置
  const { data: lists = [], isLoading } = useQuery({
    queryKey: ['custom-dashboard-lists', refreshKey],
    queryFn: () => api.get('/snmp-collect/dashboard-lists', { params: { dashboard_only: 'true' } }).then(res => res.data.data || []),
    refetchInterval: 120000,
  });

  const handleRefresh = useCallback(() => {
    setRefreshKey(k => k + 1);
    lists.forEach((l: any) => {
      queryClient.invalidateQueries({ queryKey: ['custom-list-data', l.id] });
    });
    toast.success('列表数据已刷新');
  }, [lists, queryClient, toast]);

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/snmp-collect/dashboard-lists/${id}`),
    onSuccess: () => {
      toast.success('列表面板已删除');
      queryClient.invalidateQueries({ queryKey: ['custom-dashboard-lists'] });
    },
    onError: () => toast.error('删除失败'),
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="w-5 h-5 animate-spin text-blue-400" />
      </div>
    );
  }

  return (
    <div className="space-y-4">
      {/* 标题栏 */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Table2 className="w-4 h-4 text-blue-400" />
          <span className="text-sm font-medium text-slate-300">自定义列表面板</span>
          {lists.length > 0 && <span className="text-xs text-slate-500">({lists.length})</span>}
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleRefresh}
            className="flex items-center gap-1 px-2.5 py-1 bg-[#0d1225] border border-blue-500/20 rounded-md text-xs text-slate-300 hover:border-blue-500/40 transition-colors"
          >
            <RefreshCw className="w-3 h-3" />
            刷新
          </button>
          <button
            onClick={() => { setEditingList(null); setShowEditModal(true); }}
            className="flex items-center gap-1 px-2.5 py-1 bg-blue-500/20 border border-blue-500/30 rounded-md text-xs text-blue-300 hover:bg-blue-500/30 transition-colors"
          >
            <Plus className="w-3 h-3" />
            新建列表
          </button>
        </div>
      </div>

      {/* 列表渲染 */}
      {lists.length === 0 ? (
        <div className="bg-[#0d1225] rounded-xl border border-blue-500/10 p-8 text-center">
          <Table2 className="w-8 h-8 text-slate-600 mx-auto mb-2" />
          <p className="text-sm text-slate-500 mb-1">暂无自定义列表</p>
          <p className="text-xs text-slate-600">点击"新建列表"创建一个 Grafana 风格的设备信息表格</p>
        </div>
      ) : (
        <div className="space-y-4">
          {lists.map((list: any) => (
            <ListTable
              key={list.id}
              list={list}
              onEdit={() => { setEditingList(list); setShowEditModal(true); }}
              onDelete={() => deleteMutation.mutate(list.id)}
              onViewRow={(row) => setDetailModal({ row, columns: list.columns, listName: list.name })}
              onViewMetrics={(deviceId) => navigate(`/device-metrics/${deviceId}`)}
            />
          ))}
        </div>
      )}

      {/* 编辑弹窗 */}
      {showEditModal && (
        <ListEditModal
          list={editingList}
          onClose={() => { setShowEditModal(false); setEditingList(null); }}
          onSave={() => {
            setShowEditModal(false);
            setEditingList(null);
            setRefreshKey(k => k + 1);
            queryClient.invalidateQueries({ queryKey: ['custom-dashboard-lists'] });
          }}
        />
      )}

      {/* 行详情弹窗 */}
      {detailModal && (
        <RowDetailModal
          row={detailModal.row}
          columns={detailModal.columns}
          listName={detailModal.listName}
          onClose={() => setDetailModal(null)}
        />
      )}
    </div>
  );
}

// ============================================================
// 单个列表表格
// ============================================================

function ListTable({ list, onEdit, onDelete, onViewRow, onViewMetrics }: {
  list: any;
  onEdit: () => void;
  onDelete: () => void;
  onViewRow: (row: any) => void;
  onViewMetrics: (deviceId: string) => void;
}) {
  const [sortCol, setSortCol] = useState<string | null>(null);
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc');

  const { data: collectData, isLoading, refetch } = useQuery({
    queryKey: ['custom-list-data', list.id, sortCol, sortDir],
    queryFn: () => api.post(`/snmp-collect/dashboard-lists/${list.id}/collect`).then(res => res.data.data),
    refetchInterval: (list.refresh_interval || 60) * 1000,
    refetchOnWindowFocus: false,
  });

  const columns: any[] = list.columns || [];
  const rows = useMemo(() => {
    return collectData?.rows || [];
  }, [collectData, columns]);

  // 排序
  const sortedRows = useMemo(() => {
    if (!sortCol) return rows;
    return [...rows].sort((a, b) => {
      const va = a.cells?.[sortCol]?.value ?? '';
      const vb = b.cells?.[sortCol]?.value ?? '';
      const na = parseFloat(va), nb = parseFloat(vb);
      let cmp: number;
      if (!isNaN(na) && !isNaN(nb)) cmp = na - nb;
      else cmp = String(va).localeCompare(String(vb));
      return sortDir === 'asc' ? cmp : -cmp;
    });
  }, [rows, sortCol, sortDir]);

  const handleSort = (colId: string) => {
    if (sortCol === colId) {
      setSortDir(d => d === 'asc' ? 'desc' : 'asc');
    } else {
      setSortCol(colId);
      setSortDir('asc');
    }
  };

  return (
    <div className="bg-[#0d1225] rounded-xl border border-blue-500/15 overflow-hidden">
      {/* 表格标题栏 */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-blue-500/10">
        <div className="flex items-center gap-2">
          <Table2 className="w-3.5 h-3.5 text-blue-400" />
          <span className="text-sm font-medium text-slate-200">{list.name}</span>
          {list.description && <span className="text-xs text-slate-500">— {list.description}</span>}
          {!isLoading && rows.length > 0 && (
            <span className="text-[10px] text-slate-600 ml-1">{rows.length} 行</span>
          )}
        </div>
        <div className="flex items-center gap-1">
          <button onClick={() => refetch()} className="p-1 text-slate-500 hover:text-blue-400 transition-colors" title="刷新数据">
            {isLoading ? <Loader2 className="w-3 h-3 animate-spin" /> : <RefreshCw className="w-3 h-3" />}
          </button>
          <button onClick={onEdit} className="p-1 text-slate-500 hover:text-blue-400 transition-colors" title="编辑">
            <Settings className="w-3 h-3" />
          </button>
          <button onClick={onDelete} className="p-1 text-slate-500 hover:text-red-400 transition-colors" title="删除">
            <Trash2 className="w-3 h-3" />
          </button>
        </div>
      </div>

      {/* 表格内容 */}
      <div className="overflow-x-auto max-h-80">
        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="w-4 h-4 animate-spin text-blue-400" />
            <span className="text-xs text-slate-500 ml-2">采集中...</span>
          </div>
        ) : rows.length === 0 ? (
          <div className="text-center py-8 text-xs text-slate-600">暂无数据</div>
        ) : (
          <table className="w-full text-xs">
            <thead className="sticky top-0 bg-[#111836] z-10">
              <tr className="border-b border-blue-500/10">
                {columns.map((col: any) => (
                  <th
                    key={col.id}
                    onClick={() => handleSort(col.id)}
                    className={`px-3 py-2 text-slate-400 font-medium cursor-pointer hover:text-slate-200 select-none whitespace-nowrap ${
                      col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left'
                    }`}
                  >
                    <span className="inline-flex items-center gap-1">
                      {col.label}
                      {sortCol === col.id ? (
                        sortDir === 'asc' ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />
                      ) : (
                        <ArrowUpDown className="w-2.5 h-2.5 opacity-40" />
                      )}
                    </span>
                  </th>
                ))}
                <th className="px-3 py-2 w-16"></th>
              </tr>
            </thead>
            <tbody>
              {sortedRows.map((row: any, i: number) => (
                <tr key={`${row.deviceId}-${row.rowIndex}-${i}`} className="border-b border-blue-500/5 hover:bg-blue-500/5 transition-colors">
                  {columns.map((col: any) => {
                    const cell = row.cells?.[col.id];
                    if (!cell) return <td key={col.id} className="px-3 py-1.5"></td>;
                    const isLink = col.link || col.type === 'label';
                    const textColor = col.color || (col.type === 'label' ? '#73a6ff' : '');
                    const bgColor = col.bgColor || '';
                    const display = cell.display ?? cell.value ?? '-';

                    // 状态徽章（值映射列）
                    const badge = col.type === 'metric' ? getStatusBadge(cell.value, col.valueMappings) : null;
                    // 百分比进度条
                    const numVal = parseFloat(cell.value);
                    const isPercent = col.type === 'metric' && col.unit === '%' && !isNaN(numVal);

                    return (
                      <td
                        key={col.id}
                        className={`px-3 py-1.5 whitespace-nowrap ${col.align === 'center' ? 'text-center' : col.align === 'right' ? 'text-right' : 'text-left'}`}
                        style={bgColor ? { backgroundColor: bgColor } : undefined}
                      >
                        {badge ? (
                          /* 状态徽章 */
                          <span
                            className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium"
                            style={{ backgroundColor: badge.bg + '22', color: badge.bg, border: `1px solid ${badge.bg}44` }}
                          >
                            <span className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: badge.bg }} />
                            {display}
                          </span>
                        ) : isPercent ? (
                          /* 百分比 + 迷你进度条 */
                          <div className="flex items-center gap-2 justify-center">
                            <span className="font-mono text-[11px]" style={{ color: getPercentColor(numVal) }}>{display}</span>
                            <div className="w-16 h-1.5 bg-slate-700/50 rounded-full overflow-hidden">
                              <div
                                className="h-full rounded-full transition-all duration-500"
                                style={{
                                  width: `${Math.min(numVal, 100)}%`,
                                  backgroundColor: getPercentColor(numVal),
                                }}
                              />
                            </div>
                          </div>
                        ) : (
                          /* 普通文本 */
                          <span
                            className={isLink ? 'cursor-pointer hover:underline' : ''}
                            style={{ color: textColor || undefined }}
                            onClick={isLink ? () => onViewRow(row) : undefined}
                          >
                            {display}
                          </span>
                        )}
                      </td>
                    );
                  })}
                  <td className="px-3 py-1.5 text-center whitespace-nowrap">
                    <div className="flex items-center justify-center gap-0.5">
                      <button
                        onClick={() => onViewRow(row)}
                        className="p-0.5 text-slate-600 hover:text-blue-400 transition-colors"
                        title="查看行详情"
                      >
                        <Eye className="w-3 h-3" />
                      </button>
                      <button
                        onClick={() => onViewMetrics(row.deviceId)}
                        className="p-0.5 text-slate-600 hover:text-cyan-400 transition-colors"
                        title="查看设备采集仪表盘"
                      >
                        <BarChart3 className="w-3 h-3" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* 采样时间 */}
      {collectData?.sampledAt && (
        <div className="px-3 py-1.5 border-t border-blue-500/5 text-[10px] text-slate-600 flex items-center gap-2">
          <span>采样时间: {new Date(collectData.sampledAt).toLocaleString()}</span>
        </div>
      )}
    </div>
  );
}

// ============================================================
// 行详情弹窗（查看单行所有数据）
// ============================================================

function RowDetailModal({ row, columns, listName, onClose }: {
  row: any;
  columns: any[];
  listName: string;
  onClose: () => void;
}) {
  useEscapeKey({ onEscape: onClose });
  const navigate = useNavigate();

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-[#0d1225] border border-blue-500/20 rounded-xl w-full max-w-md max-h-[70vh] overflow-y-auto shadow-2xl" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-4 py-3 border-b border-blue-500/10">
          <h3 className="text-sm font-semibold text-slate-200 flex items-center gap-2">
            <ExternalLink className="w-4 h-4 text-blue-400" />
            {listName} - 详情
          </h3>
          <button onClick={onClose} className="p-1 hover:bg-blue-500/10 rounded"><X className="w-4 h-4 text-slate-400" /></button>
        </div>
        <div className="p-4 space-y-2">
          <div className="text-xs text-slate-500 mb-2 flex items-center justify-between">
            <div>
              设备: <span className="text-slate-300">{row.deviceName}</span>
              <span className="text-slate-600 ml-2 font-mono">{row.deviceIp}</span>
              {row.rowIndex !== '_device' && <span className="ml-2 text-blue-400">索引: {row.rowIndex}</span>}
            </div>
            <button
              onClick={() => { onClose(); navigate(`/device-metrics/${row.deviceId}`); }}
              className="flex items-center gap-1 px-2 py-1 text-[10px] text-cyan-300 border border-cyan-500/30 rounded hover:bg-cyan-500/10 transition-colors"
            >
              <BarChart3 className="w-3 h-3" />
              采集仪表盘
            </button>
          </div>
          <div className="border border-blue-500/10 rounded-lg overflow-hidden">
            {columns.map((col: any) => {
              const cell = row.cells?.[col.id];
              const display = cell?.display ?? cell?.value ?? '-';
              const textColor = col.color || (col.type === 'label' ? '#73a6ff' : '');
              const badge = col.type === 'metric' ? getStatusBadge(cell?.value, col.valueMappings) : null;
              const numVal = parseFloat(cell?.value);
              const isPercent = col.type === 'metric' && col.unit === '%' && !isNaN(numVal);
              return (
                <div key={col.id} className="flex items-center justify-between px-3 py-2 border-b border-blue-500/5 last:border-0">
                  <span className="text-xs text-slate-500">{col.label}</span>
                  {badge ? (
                    <span className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-[11px] font-medium"
                      style={{ backgroundColor: badge.bg + '22', color: badge.bg, border: `1px solid ${badge.bg}44` }}>
                      <span className="w-1.5 h-1.5 rounded-full" style={{ backgroundColor: badge.bg }} />
                      {display}
                    </span>
                  ) : isPercent ? (
                    <span className="font-mono text-xs" style={{ color: getPercentColor(numVal) }}>{display}</span>
                  ) : (
                    <span className="text-xs font-mono" style={{ color: textColor || '#cbd5e1' }}>{display}</span>
                  )}
                </div>
              );
            })}
          </div>
        </div>
      </div>
    </div>
  );
}

// ============================================================
// 列表编辑弹窗
// ============================================================

function ListEditModal({ list, onClose, onSave }: {
  list: any | null;
  onClose: () => void;
  onSave: () => void;
}) {
  const toast = useToast();
  const [name, setName] = useState(list?.name || '');
  const [description, setDescription] = useState(list?.description || '');
  const [selectedDeviceIds, setSelectedDeviceIds] = useState<Set<string>>(
    new Set(list?.device_ids || [])
  );
  const [columns, setColumns] = useState<any[]>(
    list?.columns || [
      { id: `col-${Date.now()}`, type: 'label', label: '设备IP', field: 'device_ip', color: '#73a6ff', align: 'left' },
      { id: `col-${Date.now() + 1}`, type: 'label', label: '设备名称', field: 'device_name', color: '#73a6ff', align: 'left' },
    ]
  );
  const [saving, setSaving] = useState(false);

  useEscapeKey({ onEscape: onClose });

  const { data: devices = [] } = useQuery({
    queryKey: ['network-devices-for-list'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data || []),
    staleTime: 60000,
  });

  const { data: mibObjects = [] } = useQuery({
    queryKey: ['mib-objects-for-list'],
    queryFn: () => api.get('/snmp-collect/mib-objects/search', { params: { q: '' } }).then(res => res.data.data || []),
    enabled: false, // 手动触发
  });

  const toggleDevice = (id: string) => {
    const s = new Set(selectedDeviceIds);
    if (s.has(id)) s.delete(id); else s.add(id);
    setSelectedDeviceIds(s);
  };

  const selectAllDevices = () => {
    if (selectedDeviceIds.size === devices.length) {
      setSelectedDeviceIds(new Set());
    } else {
      setSelectedDeviceIds(new Set(devices.map((d: any) => d.id)));
    }
  };

  const addLabelColumn = () => {
    setColumns([...columns, {
      id: `col-${Date.now()}`,
      type: 'label',
      label: '新标签',
      field: 'device_ip',
      color: '#73a6ff',
      align: 'left',
    }]);
  };

  const addMetricColumn = () => {
    setColumns([...columns, {
      id: `col-${Date.now()}`,
      type: 'metric',
      label: '新指标',
      oid: '1.3.6.1.2.1.1.3.0',
      is_table: 0,
      unit: '',
      color: '',
      bgColor: '',
      align: 'center',
      link: false,
      valueMappings: {},
    }]);
  };

  const updateColumn = (colId: string, field: string, value: any) => {
    setColumns(columns.map(c => c.id === colId ? { ...c, [field]: value } : c));
  };

  const removeColumn = (colId: string) => {
    setColumns(columns.filter(c => c.id !== colId));
  };

  const moveColumn = (colId: string, dir: 'up' | 'down') => {
    const idx = columns.findIndex(c => c.id === colId);
    if (idx < 0) return;
    const newIdx = dir === 'up' ? idx - 1 : idx + 1;
    if (newIdx < 0 || newIdx >= columns.length) return;
    const newCols = [...columns];
    [newCols[idx], newCols[newIdx]] = [newCols[newIdx], newCols[idx]];
    setColumns(newCols);
  };

  const handleSave = async () => {
    if (!name.trim()) { toast.error('请输入列表名称'); return; }
    if (selectedDeviceIds.size === 0) { toast.error('请至少选择一台设备'); return; }
    if (columns.length === 0) { toast.error('请至少添加一列'); return; }

    const payload = {
      name,
      description,
      device_ids: Array.from(selectedDeviceIds),
      columns: columns.map(c => ({
        ...c,
        valueMappings: c.valueMappings && Object.keys(c.valueMappings).length > 0 ? c.valueMappings : undefined,
      })),
    };

    setSaving(true);
    try {
      if (list) {
        await api.put(`/snmp-collect/dashboard-lists/${list.id}`, payload);
        toast.success('列表面板已更新');
      } else {
        await api.post('/snmp-collect/dashboard-lists', payload);
        toast.success('列表面板已创建');
      }
      onSave();
    } catch (err: any) {
      toast.error(err.response?.data?.error || '保存失败');
    }
    setSaving(false);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={onClose}>
      <div className="bg-[#0d1225] border border-blue-500/20 rounded-xl w-full max-w-3xl max-h-[85vh] overflow-y-auto shadow-2xl" onClick={e => e.stopPropagation()}>
        {/* 头部 */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-blue-500/10 sticky top-0 bg-[#0d1225] z-10">
          <h2 className="text-sm font-semibold text-slate-200 flex items-center gap-2">
            <Table2 className="w-4 h-4 text-blue-400" />
            {list ? '编辑列表面板' : '新建列表面板'}
          </h2>
          <button onClick={onClose} className="p-1 hover:bg-blue-500/10 rounded"><X className="w-4 h-4 text-slate-400" /></button>
        </div>

        <div className="p-4 space-y-4">
          {/* 基本信息 */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-slate-400 mb-1">列表名称</label>
              <input value={name} onChange={e => setName(e.target.value)} placeholder="如：光模块状态"
                className="w-full px-3 py-1.5 bg-[#111836] border border-blue-500/20 rounded-md text-xs text-slate-200 focus:outline-none focus:border-blue-500" />
            </div>
            <div>
              <label className="block text-xs font-medium text-slate-400 mb-1">描述（可选）</label>
              <input value={description} onChange={e => setDescription(e.target.value)} placeholder="如：各设备光模块收发光功率监控"
                className="w-full px-3 py-1.5 bg-[#111836] border border-blue-500/20 rounded-md text-xs text-slate-200 focus:outline-none focus:border-blue-500" />
            </div>
          </div>

          {/* 设备选择 */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-xs font-medium text-slate-400">关联设备 <span className="text-slate-600">({selectedDeviceIds.size}/{devices.length})</span></label>
              <button onClick={selectAllDevices} className="text-[10px] text-blue-400 hover:text-blue-300">
                {selectedDeviceIds.size === devices.length ? '取消全选' : '全选'}
              </button>
            </div>
            <div className="max-h-32 overflow-y-auto border border-blue-500/15 rounded-lg bg-[#111836]">
              {devices.length === 0 ? (
                <p className="text-center py-3 text-xs text-slate-600">暂无设备</p>
              ) : devices.map((d: any) => (
                <div
                  key={d.id}
                  onClick={() => toggleDevice(d.id)}
                  className={`flex items-center gap-2 px-3 py-1.5 cursor-pointer transition-colors border-b border-blue-500/5 last:border-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500/10' : 'hover:bg-blue-500/5'
                  }`}
                >
                  <div className={`w-3 h-3 rounded border flex items-center justify-center flex-shrink-0 ${
                    selectedDeviceIds.has(d.id) ? 'bg-blue-500 border-blue-500' : 'border-slate-600'
                  }`}>
                    {selectedDeviceIds.has(d.id) && <span className="text-white text-[8px]">✓</span>}
                  </div>
                  <span className="text-xs text-slate-300">{d.name}</span>
                  <span className="text-[10px] text-slate-500 font-mono ml-auto">{d.ip_address}</span>
                </div>
              ))}
            </div>
          </div>

          {/* 列配置 */}
          <div>
            <div className="flex items-center justify-between mb-1.5">
              <label className="text-xs font-medium text-slate-400">表格列配置 <span className="text-slate-600">({columns.length} 列)</span></label>
              <div className="flex items-center gap-2">
                <button onClick={addLabelColumn} className="flex items-center gap-1 px-2 py-0.5 text-[10px] text-slate-400 border border-slate-600/30 rounded hover:text-slate-200 hover:border-slate-500 transition-colors">
                  <Plus className="w-2.5 h-2.5" />标签列
                </button>
                <button onClick={addMetricColumn} className="flex items-center gap-1 px-2 py-0.5 text-[10px] text-blue-300 border border-blue-500/30 rounded hover:bg-blue-500/10 transition-colors">
                  <Plus className="w-2.5 h-2.5" />指标列
                </button>
              </div>
            </div>

            {/* 列列表 */}
            <div className="space-y-1.5">
              {columns.map((col, idx) => (
                <div key={col.id} className="border border-blue-500/10 rounded-lg p-2 bg-[#111836]">
                  <div className="flex items-center gap-2 mb-1.5">
                    {/* 序号 + 排序按钮 */}
                    <span className="text-[10px] text-slate-600 w-4 text-center">{idx + 1}</span>
                    <button onClick={() => moveColumn(col.id, 'up')} disabled={idx === 0}
                      className="p-0.5 text-slate-600 hover:text-slate-300 disabled:opacity-30 disabled:cursor-not-allowed">
                      <ChevronUp className="w-3 h-3" />
                    </button>
                    <button onClick={() => moveColumn(col.id, 'down')} disabled={idx === columns.length - 1}
                      className="p-0.5 text-slate-600 hover:text-slate-300 disabled:opacity-30 disabled:cursor-not-allowed">
                      <ChevronDown className="w-3 h-3" />
                    </button>

                    {/* 类型标记 */}
                    <span className={`text-[10px] px-1.5 py-0.5 rounded ${col.type === 'label' ? 'bg-violet-500/15 text-violet-300' : 'bg-blue-500/15 text-blue-300'}`}>
                      {col.type === 'label' ? '标签' : '指标'}
                    </span>

                    {/* 删除按钮 */}
                    <button onClick={() => removeColumn(col.id)} className="ml-auto p-0.5 text-slate-600 hover:text-red-400 transition-colors">
                      <Trash2 className="w-3 h-3" />
                    </button>
                  </div>

                  <div className="grid grid-cols-12 gap-1.5 items-end ml-7">
                    {/* 列标题 */}
                    <div className="col-span-3">
                      <label className="block text-[9px] text-slate-500 mb-0.5">列标题</label>
                      <input value={col.label} onChange={e => updateColumn(col.id, 'label', e.target.value)}
                        className="w-full px-2 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500" />
                    </div>

                    {/* 标签字段 or OID */}
                    {col.type === 'label' ? (
                      <div className="col-span-4">
                        <label className="block text-[9px] text-slate-500 mb-0.5">标签字段</label>
                        <select value={col.field} onChange={e => updateColumn(col.id, 'field', e.target.value)}
                          className="w-full px-2 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500">
                          {LABEL_FIELD_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                        </select>
                      </div>
                    ) : (
                      <>
                        <div className="col-span-4">
                          <label className="block text-[9px] text-slate-500 mb-0.5">OID</label>
                          <input value={col.oid || ''} onChange={e => updateColumn(col.id, 'oid', e.target.value)}
                            placeholder="1.3.6.1.2.1..."
                            className="w-full px-2 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-blue-300 font-mono focus:outline-none focus:border-blue-500" />
                        </div>
                        <div className="col-span-2">
                          <label className="block text-[9px] text-slate-500 mb-0.5">采集方式</label>
                          <select value={col.is_table || 0} onChange={e => updateColumn(col.id, 'is_table', parseInt(e.target.value))}
                            className="w-full px-2 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500">
                            <option value={0}>GET (标量)</option>
                            <option value={1}>WALK (表列)</option>
                          </select>
                        </div>
                        <div className="col-span-2">
                          <label className="block text-[9px] text-slate-500 mb-0.5">单位</label>
                          <input value={col.unit || ''} onChange={e => updateColumn(col.id, 'unit', e.target.value)} placeholder="dBm, ℃"
                            className="w-full px-2 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500" />
                        </div>
                      </>
                    )}

                    {/* 文字颜色 */}
                    <div className="col-span-2">
                      <label className="block text-[9px] text-slate-500 mb-0.5">文字颜色</label>
                      <select value={col.color || ''} onChange={e => updateColumn(col.id, 'color', e.target.value)}
                        className="w-full px-1 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500"
                        style={{ color: col.color || undefined }}>
                        {TEXT_COLOR_PRESETS.map(c => <option key={c.value} value={c.value} style={{ color: c.value || '#9ca3af' }}>{c.label}</option>)}
                      </select>
                    </div>

                    {/* 背景颜色（仅指标列） */}
                    {col.type === 'metric' && (
                      <div className="col-span-2">
                        <label className="block text-[9px] text-slate-500 mb-0.5">背景颜色</label>
                        <select value={col.bgColor || ''} onChange={e => updateColumn(col.id, 'bgColor', e.target.value)}
                          className="w-full px-1 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500">
                          {BG_COLOR_PRESETS.map(c => <option key={c.value} value={c.value}>{c.label}</option>)}
                        </select>
                      </div>
                    )}

                    {/* 对齐方式 */}
                    <div className="col-span-2">
                      <label className="block text-[9px] text-slate-500 mb-0.5">对齐</label>
                      <select value={col.align || 'left'} onChange={e => updateColumn(col.id, 'align', e.target.value)}
                        className="w-full px-1 py-1 bg-[#0d1225] border border-blue-500/10 rounded text-[11px] text-slate-200 focus:outline-none focus:border-blue-500">
                        {ALIGN_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                      </select>
                    </div>

                    {/* 可点击链接（仅指标列） */}
                    {col.type === 'metric' && (
                      <div className="col-span-2 flex items-center gap-1.5 pb-1">
                        <label className="flex items-center gap-1 cursor-pointer">
                          <input type="checkbox" checked={col.link || false} onChange={e => updateColumn(col.id, 'link', e.target.checked)}
                            className="w-3 h-3 accent-blue-500" />
                          <span className="text-[10px] text-slate-400">可点击</span>
                        </label>
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* 底部按钮 */}
        <div className="flex justify-end gap-2 px-4 py-3 border-t border-blue-500/10 sticky bottom-0 bg-[#0d1225]">
          <button onClick={onClose} className="px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200">取消</button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex items-center gap-1.5 px-4 py-1.5 bg-blue-500/20 border border-blue-500/30 text-xs text-blue-200 rounded-md hover:bg-blue-500/30 transition-colors disabled:opacity-50"
          >
            {saving && <Loader2 className="w-3 h-3 animate-spin" />}
            保存
          </button>
        </div>
      </div>
    </div>
  );
}
