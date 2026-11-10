import { useState } from 'react';
import { X, Sliders, Save, Loader2, ShieldCheck, Activity, Layers } from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';

interface BatchPollSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

const vendors = [
  { id: 'all', label: '全部品牌 (All Vendors)' },
  { id: 'huawei', label: '华为 (Huawei)' },
  { id: 'h3c', label: '华三 (H3C)' },
  { id: 'cisco', label: '思科 (Cisco)' },
  { id: 'ruijie', label: '锐捷 (Ruijie)' },
  { id: 'zte', label: '中兴 (ZTE)' },
  { id: 'maipu', label: '迈普 (Maipu)' },
  { id: 'fortinet', label: '飞塔 (Fortinet)' },
  { id: 'juniper', label: '瞻博 (Juniper)' },
  { id: 'other', label: '其他 (Other)' },
];

const presetIntervals = [
  { label: '10秒', value: 10 },
  { label: '30秒', value: 30 },
  { label: '1分钟', value: 60 },
  { label: '2分钟', value: 120 },
  { label: '5分钟', value: 300 },
  { label: '10分钟', value: 600 },
];

export default function BatchPollSettingsModal({ isOpen, onClose, onSuccess }: BatchPollSettingsModalProps) {
  const toast = useToast();
  const [selectedVendor, setSelectedVendor] = useState<string>('all');
  const [systemInterval, setSystemInterval] = useState<number>(300);
  const [interfaceInterval, setInterfaceInterval] = useState<number>(60);
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (systemInterval < 5 || interfaceInterval < 5) {
      toast.error('采集周期最小不能小于 5 秒');
      return;
    }

    setIsSubmitting(true);
    try {
      const res = await api.post('/network-devices/batch-poll-interval', {
        vendor: selectedVendor,
        system_poll_interval: Number(systemInterval),
        interface_poll_interval: Number(interfaceInterval),
      });
      const count = res.data?.data?.count ?? res.data?.count ?? 0;
      toast.success(`已成功为 ${count} 台设备更新采集间隔`);
      onSuccess();
      onClose();
    } catch (err: any) {
      toast.error(err.response?.data?.message || '批量更新采集周期失败');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4 animate-in fade-in duration-200">
      <div className="bg-surface border border-border rounded-xl shadow-2xl w-full max-w-lg overflow-hidden">
        {/* Header */}
        <div className="flex items-center justify-between p-5 border-b border-border bg-background/50">
          <div className="flex items-center gap-3">
            <div className="w-9 h-9 rounded-lg bg-purple-500/10 border border-purple-500/20 flex items-center justify-center text-purple-400">
              <Sliders className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-semibold text-text-primary text-base">按品牌批量设置采集间隔</h3>
              <p className="text-xs text-text-secondary">批量指定特定品牌设备的分模块采集周期</p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-text-secondary hover:text-text-primary hover:bg-surface rounded-lg transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="p-5 space-y-6">
          {/* 品牌选择 */}
          <div className="space-y-2">
            <label className="text-xs font-medium text-text-primary flex items-center gap-2">
              <Layers className="w-4 h-4 text-primary" />
              选择目标品牌
            </label>
            <select
              value={selectedVendor}
              onChange={(e) => setSelectedVendor(e.target.value)}
              className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
            >
              {vendors.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.label}
                </option>
              ))}
            </select>
          </div>

          {/* 系统模块设置 */}
          <div className="p-4 bg-background/40 border border-border/80 rounded-lg space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <ShieldCheck className="w-4 h-4 text-emerald-400" />
                <span className="font-medium text-sm text-text-primary">系统模块采集周期</span>
              </div>
              <span className="text-[11px] text-text-secondary bg-surface px-2 py-0.5 rounded border border-border">
                CPU / 内存 / 风扇 / 电源 / 温度
              </span>
            </div>
            <div className="flex items-center gap-3">
              <div className="relative flex-1">
                <input
                  type="number"
                  min={5}
                  max={86400}
                  value={systemInterval}
                  onChange={(e) => setSystemInterval(parseInt(e.target.value) || 0)}
                  className="w-full pl-3 pr-10 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary font-mono"
                  placeholder="300"
                  required
                />
                <span className="absolute right-3 top-2.5 text-xs text-text-secondary">秒</span>
              </div>
              <div className="flex items-center gap-1 flex-wrap">
                {presetIntervals.map((p) => (
                  <button
                    key={`b-sys-${p.value}`}
                    type="button"
                    onClick={() => setSystemInterval(p.value)}
                    className={`px-2 py-1 text-xs rounded border transition-colors ${
                      systemInterval === p.value
                        ? 'bg-primary/20 border-primary text-primary font-medium'
                        : 'bg-surface border-border text-text-secondary hover:text-text-primary'
                    }`}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* 接口模块设置 */}
          <div className="p-4 bg-background/40 border border-border/80 rounded-lg space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Activity className="w-4 h-4 text-purple-400" />
                <span className="font-medium text-sm text-text-primary">接口模块采集周期</span>
              </div>
              <span className="text-[11px] text-text-secondary bg-surface px-2 py-0.5 rounded border border-border">
                标准 IF-MIB / 流量 / 端口状态
              </span>
            </div>
            <div className="flex items-center gap-3">
              <div className="relative flex-1">
                <input
                  type="number"
                  min={5}
                  max={86400}
                  value={interfaceInterval}
                  onChange={(e) => setInterfaceInterval(parseInt(e.target.value) || 0)}
                  className="w-full pl-3 pr-10 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary font-mono"
                  placeholder="60"
                  required
                />
                <span className="absolute right-3 top-2.5 text-xs text-text-secondary">秒</span>
              </div>
              <div className="flex items-center gap-1 flex-wrap">
                {presetIntervals.map((p) => (
                  <button
                    key={`b-if-${p.value}`}
                    type="button"
                    onClick={() => setInterfaceInterval(p.value)}
                    className={`px-2 py-1 text-xs rounded border transition-colors ${
                      interfaceInterval === p.value
                        ? 'bg-purple-500/20 border-purple-500 text-purple-400 font-medium'
                        : 'bg-surface border-border text-text-secondary hover:text-text-primary'
                    }`}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className="flex items-center justify-end gap-3 pt-2">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary bg-surface border border-border hover:bg-background rounded-lg transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="flex items-center gap-2 px-5 py-2 text-sm font-medium text-white bg-purple-600 hover:bg-purple-500 rounded-lg shadow-lg shadow-purple-500/20 transition-all disabled:opacity-50"
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  应用中...
                </>
              ) : (
                <>
                  <Save className="w-4 h-4" />
                  批量应用设置
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
