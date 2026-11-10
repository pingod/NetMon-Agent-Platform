import { useState } from 'react';
import { X, Clock, Settings, Save, Loader2, ShieldCheck, Activity } from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';

interface NetworkDevice {
  id: string;
  name: string;
  ip_address: string;
  vendor: string;
  system_poll_interval?: number;
  interface_poll_interval?: number;
}

interface PollSettingsModalProps {
  device: NetworkDevice;
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
}

const presetIntervals = [
  { label: '10秒', value: 10 },
  { label: '30秒', value: 30 },
  { label: '1分钟', value: 60 },
  { label: '2分钟', value: 120 },
  { label: '5分钟', value: 300 },
  { label: '10分钟', value: 600 },
];

export default function PollSettingsModal({ device, isOpen, onClose, onSuccess }: PollSettingsModalProps) {
  const toast = useToast();
  const [systemInterval, setSystemInterval] = useState<number>(device.system_poll_interval || 300);
  const [interfaceInterval, setInterfaceInterval] = useState<number>(device.interface_poll_interval || 60);
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
      await api.put(`/network-devices/${device.id}/poll-interval`, {
        system_poll_interval: Number(systemInterval),
        interface_poll_interval: Number(interfaceInterval),
      });
      toast.success(`[${device.name}] 采集周期设置已更新`);
      onSuccess();
      onClose();
    } catch (err: any) {
      toast.error(err.response?.data?.message || '更新采集周期失败');
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
            <div className="w-9 h-9 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
              <Settings className="w-5 h-5" />
            </div>
            <div>
              <h3 className="font-semibold text-text-primary text-base">采集周期设置</h3>
              <p className="text-xs text-text-secondary">
                {device.name} <span className="font-mono text-primary/80">({device.ip_address})</span>
              </p>
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
          {/* 系统模块设置 */}
          <div className="p-4 bg-background/40 border border-border/80 rounded-lg space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <ShieldCheck className="w-4 h-4 text-emerald-400" />
                <span className="font-medium text-sm text-text-primary">系统模块采集周期</span>
              </div>
              <span className="text-[11px] text-text-secondary bg-surface px-2 py-0.5 rounded border border-border">
                CPU / 内存 / 风扇 / 电源 / 温度 / 私有 OID
              </span>
            </div>
            <p className="text-xs text-text-secondary">
              定义系统硬件与环境私有 OID 指标的自动采集轮询频率。
            </p>
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
                    key={`sys-${p.value}`}
                    type="button"
                    onClick={() => setSystemInterval(p.value)}
                    className={`px-2.5 py-1 text-xs rounded border transition-colors ${
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
                标准 IF-MIB / 端口状态 / 进出流量
              </span>
            </div>
            <p className="text-xs text-text-secondary">
              定义标准 MIB 端口链路状态、接口流量及错误包统计的采集频率。
            </p>
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
                    key={`if-${p.value}`}
                    type="button"
                    onClick={() => setInterfaceInterval(p.value)}
                    className={`px-2.5 py-1 text-xs rounded border transition-colors ${
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
              className="flex items-center gap-2 px-5 py-2 text-sm font-medium text-white bg-primary hover:bg-primary/90 rounded-lg shadow-lg shadow-primary/20 transition-all disabled:opacity-50"
            >
              {isSubmitting ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  保存中...
                </>
              ) : (
                <>
                  <Save className="w-4 h-4" />
                  保存配置
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
