import React, { useState, useEffect } from 'react';
import {
  X,
  Cpu,
  RotateCw,
  CheckCircle2,
  AlertCircle,
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import type { Server, ServerGroup } from '../pages/types';

interface VirtualMachineFormModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSuccess: () => void;
  vm: Server | null;
  groups: ServerGroup[];
}

export const VirtualMachineFormModal: React.FC<VirtualMachineFormModalProps> = ({
  isOpen,
  onClose,
  onSuccess,
  vm,
  groups,
}) => {
  const { toast } = useToast();
  const isEdit = !!vm?.id;

  const [formData, setFormData] = useState({
    name: '',
    hostname: '',
    port: 22,
    username: 'root',
    password: '',
    use_ssh_key: false,
    ssh_key_id: '',
    os_type: 'linux' as 'linux' | 'windows',
    description: '',
    tags: '',
    group_id: '',
  });

  const [isTesting, setIsTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (vm) {
      setFormData({
        name: vm.name || '',
        hostname: vm.hostname || vm.ip_address || '',
        port: vm.port || 22,
        username: vm.username || 'root',
        password: '',
        use_ssh_key: vm.use_ssh_key === 1,
        ssh_key_id: vm.ssh_key_id || '',
        os_type: (vm.os_type as any) || 'linux',
        description: vm.description || '',
        tags: vm.tags?.join(', ') || '',
        group_id: vm.groups?.[0]?.id || '',
      });
    } else {
      setFormData({
        name: '',
        hostname: '',
        port: 22,
        username: 'root',
        password: '',
        use_ssh_key: false,
        ssh_key_id: '',
        os_type: 'linux',
        description: '',
        tags: '',
        group_id: '',
      });
    }
    setTestResult(null);
  }, [vm, isOpen]);

  if (!isOpen) return null;

  const handleTestConnection = async () => {
    if (!formData.hostname) {
      toast('请输入主机名或 IP 地址', 'warning');
      return;
    }
    setIsTesting(true);
    setTestResult(null);
    try {
      const res = await api.post('/servers/test-connection', {
        server_id: vm?.id,
        hostname: formData.hostname,
        host: formData.hostname,
        ip_address: formData.hostname,
        port: formData.port || 22,
        username: formData.username || (formData.os_type === 'windows' ? 'Administrator' : 'root'),
        password: formData.password,
        os_type: formData.os_type,
        ssh_key_id: formData.ssh_key_id,
      });
      const data = res.data?.data || res.data;
      if (data?.success) {
        setTestResult({
          success: true,
          message: `连接成功 (延迟: ${data.latency || 15}ms)`,
        });
        toast('虚拟机连接测试成功！', 'success');
      } else {
        setTestResult({
          success: false,
          message: data?.message || '连接失败，请检查账号密码或端口',
        });
      }
    } catch (err: any) {
      setTestResult({
        success: false,
        message: err.response?.data?.message || err.message || '连接测试超时或失败',
      });
    } finally {
      setIsTesting(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formData.name || !formData.hostname) {
      toast('虚拟机名称和主机名/IP 为必填项', 'warning');
      return;
    }

    setIsSubmitting(true);
    try {
      const tagsArray = formData.tags
        .split(',')
        .map((t) => t.trim())
        .filter(Boolean);

      const payload: any = {
        name: formData.name,
        hostname: formData.hostname,
        ip_address: formData.hostname,
        port: formData.port,
        username: formData.username,
        os_type: formData.os_type,
        description: formData.description,
        tags: tagsArray,
        group_id: formData.group_id,
        is_virtual_machine: 1, // 标识为虚拟机
      };

      if (formData.password) {
        payload.password = formData.password;
      }
      if (formData.ssh_key_id) {
        payload.ssh_key_id = formData.ssh_key_id;
      }

      if (isEdit && vm?.id) {
        await api.put(`/servers/${vm.id}`, payload);
        toast('虚拟机更新成功', 'success');
      } else {
        await api.post('/servers', payload);
        toast('虚拟机创建成功', 'success');
      }
      onSuccess();
      onClose();
    } catch (err: any) {
      console.error('Save VM error:', err);
      toast('保存虚拟机失败: ' + (err.response?.data?.message || err.message), 'error');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-5 bg-black/80 backdrop-blur-md animate-in fade-in duration-200">
      <div className="relative w-full max-w-xl bg-surface border border-border rounded-2xl shadow-2xl overflow-hidden text-text-primary font-sans">
        
        {/* 顶部栏 */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-background">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-indigo-500/10 border border-indigo-500/30 flex items-center justify-center text-indigo-600">
              <Cpu className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-bold text-text-primary">
                {isEdit ? '编辑虚拟机' : '新建虚拟机'}
              </h2>
              <p className="text-xs text-text-secondary">
                硬件规格由 netmon-agent 安装后自动采集上报，无需手动填写
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-xl bg-surface-2 border border-border text-text-secondary hover:text-text-primary transition"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* 表单内容 */}
        <form onSubmit={handleSubmit} className="p-6 space-y-4">
          
          {/* 基本信息 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="text-xs font-semibold text-text-secondary block mb-1">
                虚拟机名称 <span className="text-red-600">*</span>
              </label>
              <input
                type="text"
                required
                value={formData.name}
                onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                placeholder="如: web-server-vm-01"
                className="w-full px-3 py-2 bg-background border border-border rounded-xl text-xs text-text-primary focus:border-indigo-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="text-xs font-semibold text-text-secondary block mb-1">
                主机名 / IP 地址 <span className="text-red-600">*</span>
              </label>
              <input
                type="text"
                required
                value={formData.hostname}
                onChange={(e) => setFormData({ ...formData, hostname: e.target.value })}
                placeholder="192.168.1.100 或 vm.local"
                className="w-full px-3 py-2 bg-background border border-border rounded-xl text-xs text-text-primary focus:border-indigo-500 focus:outline-none"
              />
            </div>
          </div>

          {/* 系统类型与所属分组 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="text-xs font-semibold text-text-secondary block mb-1">系统类型</label>
              <select
                value={formData.os_type}
                onChange={(e) => setFormData({ ...formData, os_type: e.target.value as any })}
                className="w-full px-3 py-2 bg-background border border-border rounded-xl text-xs text-text-primary focus:border-indigo-500 focus:outline-none"
              >
                <option value="linux">Linux</option>
                <option value="windows">Windows</option>
              </select>
            </div>

            <div>
              <label className="text-xs font-semibold text-text-secondary block mb-1">所属分组</label>
              <select
                value={formData.group_id}
                onChange={(e) => setFormData({ ...formData, group_id: e.target.value })}
                className="w-full px-3 py-2 bg-background border border-border rounded-xl text-xs text-text-primary focus:border-indigo-500 focus:outline-none"
              >
                <option value="">未分组</option>
                {groups.map((g) => (
                  <option key={g.id} value={g.id}>
                    {g.name}
                  </option>
                ))}
              </select>
            </div>
          </div>

          {/* SSH 认证信息 */}
          <div className="p-4 rounded-xl bg-background border border-border space-y-3">
            <label className="text-xs font-semibold text-text-secondary block">SSH 认证凭据 (用于下发 Agent 采集器)</label>
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label className="text-[11px] text-text-secondary block mb-1">SSH 端口</label>
                <input
                  type="number"
                  value={formData.port}
                  onChange={(e) => setFormData({ ...formData, port: Number(e.target.value) })}
                  className="w-full px-3 py-1.5 bg-background border border-border rounded-lg text-xs text-text-primary focus:border-indigo-500"
                />
              </div>

              <div>
                <label className="text-[11px] text-text-secondary block mb-1">用户名</label>
                <input
                  type="text"
                  value={formData.username}
                  onChange={(e) => setFormData({ ...formData, username: e.target.value })}
                  placeholder="root / administrator"
                  className="w-full px-3 py-1.5 bg-background border border-border rounded-lg text-xs text-text-primary focus:border-indigo-500"
                />
              </div>

              <div>
                <label className="text-[11px] text-text-secondary block mb-1">密码</label>
                <input
                  type="password"
                  value={formData.password}
                  placeholder={isEdit ? '留空保持原密码' : '输入虚拟机密码'}
                  onChange={(e) => setFormData({ ...formData, password: e.target.value })}
                  className="w-full px-3 py-1.5 bg-background border border-border rounded-lg text-xs text-text-primary focus:border-indigo-500"
                />
              </div>
            </div>

            {/* 测试连接按钮 */}
            <div className="flex items-center justify-between pt-2">
              <button
                type="button"
                onClick={handleTestConnection}
                disabled={isTesting}
                className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-surface-2 hover:bg-surface-hover border border-border text-text-primary text-xs font-medium transition disabled:opacity-50"
              >
                <RotateCw className={`w-3.5 h-3.5 ${isTesting ? 'animate-spin text-indigo-600' : ''}`} />
                <span>{isTesting ? '正在测试连接...' : '测试 SSH 连接'}</span>
              </button>

              {testResult && (
                <div
                  className={`flex items-center gap-1.5 text-xs font-medium ${
                    testResult.success ? 'text-emerald-600' : 'text-red-600'
                  }`}
                >
                  {testResult.success ? (
                    <CheckCircle2 className="w-4 h-4" />
                  ) : (
                    <AlertCircle className="w-4 h-4" />
                  )}
                  <span>{testResult.message}</span>
                </div>
              )}
            </div>
          </div>

          {/* 标签与描述 */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="text-xs font-semibold text-text-secondary block mb-1">标签 (逗号分隔)</label>
              <input
                type="text"
                value={formData.tags}
                onChange={(e) => setFormData({ ...formData, tags: e.target.value })}
                placeholder="生产环境, k8s-worker, 核心业务"
                className="w-full px-3 py-2 bg-background border border-border rounded-xl text-xs text-text-primary focus:border-indigo-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="text-xs font-semibold text-text-secondary block mb-1">描述备注</label>
              <input
                type="text"
                value={formData.description}
                onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                placeholder="虚拟机用途、归属团队等备注"
                className="w-full px-3 py-2 bg-background border border-border rounded-xl text-xs text-text-primary focus:border-indigo-500 focus:outline-none"
              />
            </div>
          </div>

          {/* 底部按钮栏 */}
          <div className="flex items-center justify-end gap-3 pt-4 border-t border-border">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-xl bg-surface-2 hover:bg-surface-hover text-text-secondary text-xs font-medium transition"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="px-5 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-lg shadow-indigo-600/30 transition disabled:opacity-50"
            >
              {isSubmitting ? '正在保存...' : isEdit ? '保存修改' : '确认创建'}
            </button>
          </div>
        </form>

      </div>
    </div>
  );
};
