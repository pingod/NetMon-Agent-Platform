import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Modal } from 'antd';
import { BellOff, Plus, Trash2, Edit2, Power, Clock } from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';

interface Silence {
  id: string;
  name: string;
  match_source: string;
  match_severity: string;
  match_keywords: string;
  match_device_id: string;
  starts_at: string;
  ends_at: string;
  enabled: boolean;
  created_by: string;
  created_at: string;
}

const emptyForm = {
  name: '',
  match_source: '',
  match_severity: '',
  match_keywords: '',
  match_device_id: '',
  starts_at: '',
  ends_at: '',
  enabled: true,
};

function toLocalInput(s: string): string {
  return s ? s.replace(' ', 'T').slice(0, 16) : '';
}

function silenceState(s: Silence): 'active' | 'scheduled' | 'expired' | 'disabled' {
  if (!s.enabled) return 'disabled';
  const now = new Date();
  const start = new Date(s.starts_at.replace(' ', 'T'));
  const end = new Date(s.ends_at.replace(' ', 'T'));
  if (now < start) return 'scheduled';
  if (now > end) return 'expired';
  return 'active';
}

const stateMeta: Record<string, { label: string; cls: string }> = {
  active: { label: '生效中', cls: 'bg-green-500/15 text-green-500' },
  scheduled: { label: '未开始', cls: 'bg-blue-500/15 text-blue-400' },
  expired: { label: '已过期', cls: 'bg-background text-text-secondary' },
  disabled: { label: '已禁用', cls: 'bg-background text-text-secondary' },
};

export default function AlertSilences() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [modalOpen, setModalOpen] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState({ ...emptyForm });

  const { data: silences = [], isLoading } = useQuery({
    queryKey: ['alert-silences'],
    queryFn: async () => {
      const res = await api.get('/alert-silences');
      const d = res.data?.data ?? res.data;
      return (Array.isArray(d) ? d : []) as Silence[];
    },
    refetchInterval: 30000,
  });

  const openModal = (s?: Silence) => {
    if (s) {
      setEditingId(s.id);
      setForm({
        name: s.name,
        match_source: s.match_source,
        match_severity: s.match_severity,
        match_keywords: s.match_keywords,
        match_device_id: s.match_device_id,
        starts_at: toLocalInput(s.starts_at),
        ends_at: toLocalInput(s.ends_at),
        enabled: s.enabled,
      });
    } else {
      setEditingId(null);
      const now = new Date();
      const later = new Date(now.getTime() + 2 * 3600 * 1000);
      const fmt = (d: Date) => {
        const p = (n: number) => String(n).padStart(2, '0');
        return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
      };
      setForm({ ...emptyForm, starts_at: fmt(now), ends_at: fmt(later) });
    }
    setModalOpen(true);
  };

  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload = { ...form, name: form.name.trim() };
      if (editingId) {
        await api.put(`/alert-silences/${editingId}`, payload);
      } else {
        await api.post('/alert-silences', payload);
      }
    },
    onSuccess: () => {
      toast.success(editingId ? '静默规则已更新' : '静默规则已创建');
      setModalOpen(false);
      queryClient.invalidateQueries({ queryKey: ['alert-silences'] });
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '保存失败'),
  });

  const toggleMutation = useMutation({
    mutationFn: (id: string) => api.post(`/alert-silences/${id}/toggle`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['alert-silences'] }),
    onError: () => toast.error('切换状态失败'),
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/alert-silences/${id}`),
    onSuccess: () => {
      toast.success('静默规则已删除');
      queryClient.invalidateQueries({ queryKey: ['alert-silences'] });
    },
    onError: () => toast.error('删除失败'),
  });

  const confirmDelete = (s: Silence) => {
    Modal.confirm({
      title: '删除静默规则',
      content: `确定删除「${s.name}」？删除后匹配告警将恢复产生。`,
      okText: '删除',
      okButtonProps: { danger: true },
      cancelText: '取消',
      onOk: () => deleteMutation.mutateAsync(s.id),
    });
  };

  const activeCount = silences.filter(s => silenceState(s) === 'active').length;

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 bg-surface border border-border rounded-xl p-5">
        <div className="flex items-start gap-3.5">
          <div className="p-3 bg-primary/10 border border-primary/30 rounded-xl text-primary">
            <BellOff className="w-6 h-6" />
          </div>
          <div>
            <h1 className="text-lg font-bold text-text-primary flex items-center gap-2">
              告警静默（维护窗口）
              {activeCount > 0 && (
                <span className="text-xs font-normal px-2 py-0.5 rounded-full bg-green-500/15 text-green-500 border border-green-500/30">
                  {activeCount} 条生效中
                </span>
              )}
            </h1>
            <p className="text-xs text-text-secondary mt-1 max-w-2xl">
              在指定时间段内抑制匹配告警的产生（不入库、不通知、不触发自愈）。适用于计划内维护、割接、升级等窗口。条件之间为「与」，关键词之间为「或」。
            </p>
          </div>
        </div>
        <button
          onClick={() => openModal()}
          className="flex items-center gap-2 px-4 py-2 bg-primary hover:bg-primary/90 text-white text-sm font-medium rounded-lg flex-shrink-0"
        >
          <Plus className="w-4 h-4" />
          新建静默规则
        </button>
      </div>

      {isLoading ? (
        <div className="p-12 text-center text-text-secondary">加载中...</div>
      ) : silences.length === 0 ? (
        <div className="bg-surface border border-border rounded-lg p-12 text-center text-text-secondary">
          <BellOff className="w-10 h-10 mx-auto mb-3 opacity-50" />
          暂无静默规则。维护前创建一条，避免维护期间告警轰炸。
        </div>
      ) : (
        <div className="grid gap-4">
          {silences.map((s) => {
            const st = silenceState(s);
            const meta = stateMeta[st];
            const conditions = [
              s.match_source && `来源=${s.match_source}`,
              s.match_severity && `级别=${s.match_severity}`,
              s.match_keywords && `关键词=${s.match_keywords}`,
              s.match_device_id && `设备=${s.match_device_id.slice(0, 8)}`,
            ].filter(Boolean).join('，');
            return (
              <div key={s.id} className="bg-surface border border-border rounded-lg p-4">
                <div className="flex items-start justify-between gap-4">
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 mb-2 flex-wrap">
                      <span className={`px-2 py-0.5 text-xs rounded ${meta.cls}`}>{meta.label}</span>
                      <h3 className="text-base font-semibold text-text-primary">{s.name}</h3>
                    </div>
                    <div className="flex items-center gap-4 text-sm text-text-secondary flex-wrap">
                      <span className="flex items-center gap-1">
                        <Clock className="w-4 h-4" />
                        {s.starts_at} ~ {s.ends_at}
                      </span>
                      <span className="truncate">匹配：{conditions || '全部告警'}</span>
                      {s.created_by && <span>创建人：{s.created_by}</span>}
                    </div>
                  </div>
                  <div className="flex items-center gap-1 flex-shrink-0">
                    <button
                      onClick={() => toggleMutation.mutate(s.id)}
                      className={`p-2 ${s.enabled ? 'text-yellow-400 hover:text-yellow-300' : 'text-green-400 hover:text-green-300'}`}
                      title={s.enabled ? '禁用' : '启用'}
                    >
                      <Power className="w-5 h-5" />
                    </button>
                    <button onClick={() => openModal(s)} className="text-primary hover:text-primary/80 p-2" title="编辑">
                      <Edit2 className="w-5 h-5" />
                    </button>
                    <button onClick={() => confirmDelete(s)} className="text-rose-400 hover:text-rose-300 p-2" title="删除">
                      <Trash2 className="w-5 h-5" />
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {modalOpen && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
          <div className="bg-surface border border-border rounded-lg w-full max-w-xl max-h-[90vh] overflow-y-auto">
            <div className="p-6">
              <h2 className="text-xl font-bold text-text-primary mb-4">{editingId ? '编辑静默规则' : '新建静默规则'}</h2>
              <div className="space-y-4">
                <div>
                  <label className="block text-sm text-text-primary mb-1">名称</label>
                  <input
                    type="text"
                    value={form.name}
                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                    className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                    placeholder="如：核心交换机升级维护"
                  />
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm text-text-primary mb-1">开始时间</label>
                    <input
                      type="datetime-local"
                      value={form.starts_at}
                      onChange={(e) => setForm({ ...form, starts_at: e.target.value })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-primary mb-1">结束时间</label>
                    <input
                      type="datetime-local"
                      value={form.ends_at}
                      onChange={(e) => setForm({ ...form, ends_at: e.target.value })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                    />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm text-text-primary mb-1">告警来源（可留空）</label>
                    <select
                      value={form.match_source}
                      onChange={(e) => setForm({ ...form, match_source: e.target.value })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                    >
                      <option value="">不限</option>
                      <option value="promql_alert">指标规则 (promql_alert)</option>
                      <option value="influxdb">InfluxDB</option>
                      <option value="snmp">SNMP</option>
                      <option value="custom">自定义</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm text-text-primary mb-1">告警级别（可留空）</label>
                    <select
                      value={form.match_severity}
                      onChange={(e) => setForm({ ...form, match_severity: e.target.value })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                    >
                      <option value="">不限</option>
                      <option value="critical">critical</option>
                      <option value="high">high</option>
                      <option value="warning">warning</option>
                      <option value="info">info</option>
                    </select>
                  </div>
                </div>
                <div>
                  <label className="block text-sm text-text-primary mb-1">关键词（逗号分隔，任一命中即匹配，可留空）</label>
                  <input
                    type="text"
                    value={form.match_keywords}
                    onChange={(e) => setForm({ ...form, match_keywords: e.target.value })}
                    className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                    placeholder="如：核心交换机, 192.168.1.1"
                  />
                </div>
                <div>
                  <label className="block text-sm text-text-primary mb-1">设备 ID（可留空）</label>
                  <input
                    type="text"
                    value={form.match_device_id}
                    onChange={(e) => setForm({ ...form, match_device_id: e.target.value })}
                    className="w-full bg-background border border-border rounded-lg p-2 text-text-primary font-mono"
                    placeholder="精确匹配告警的 device_id"
                  />
                </div>
                <label className="flex items-center gap-2 text-sm text-text-primary cursor-pointer">
                  <input
                    type="checkbox"
                    checked={form.enabled}
                    onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
                    className="w-4 h-4 accent-primary"
                  />
                  启用
                </label>
                <p className="text-xs text-text-secondary">至少填写一个匹配条件；全部留空会被拒绝，防止误静默所有告警。</p>
              </div>
              <div className="flex gap-3 mt-6">
                <button
                  onClick={() => setModalOpen(false)}
                  className="flex-1 bg-background hover:bg-surface text-text-primary py-2 rounded-lg border border-border"
                >
                  取消
                </button>
                <button
                  onClick={() => saveMutation.mutate()}
                  disabled={saveMutation.isPending || !form.name.trim() || !form.starts_at || !form.ends_at}
                  className="flex-1 bg-primary hover:bg-primary/90 disabled:opacity-50 text-white py-2 rounded-lg"
                >
                  {saveMutation.isPending ? '保存中...' : '保存'}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}