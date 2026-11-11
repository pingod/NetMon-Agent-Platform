/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Modal } from 'antd';
import {
  Key, Plus, Trash2, Edit3, Eye, EyeOff, Search, RefreshCw, Radio,
  Lock, CheckCircle2, AlertCircle, Shield, Copy, Check
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useEscapeKey } from '../../../hooks/useEscapeKey';

interface SnmpCredential {
  id: string;
  device_id?: string;
  name: string;
  community?: string;
  snmp_version: string;
  snmp_port: number;
  snmp_user?: string;
  snmp_auth_protocol?: string;
  snmp_auth_key?: string;
  snmp_priv_protocol?: string;
  snmp_priv_key?: string;
  host?: string;
  created_at: string;
}

export default function SnmpCommunities() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [searchQuery, setSearchQuery] = useState('');
  const [showModal, setShowModal] = useState(false);
  const [editingCred, setEditingCred] = useState<SnmpCredential | null>(null);
  const [showPasswords, setShowPasswords] = useState<Record<string, boolean>>({});
  const [showAuthKey, setShowAuthKey] = useState(false);
  const [showPrivKey, setShowPrivKey] = useState(false);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  const [testingId, setTestingId] = useState<string | null>(null);

  // 表单状态
  const [formData, setFormData] = useState({
    name: '',
    community: 'public',
    snmp_version: 'v2c',
    snmp_port: 161,
    snmp_user: '',
    snmp_auth_protocol: 'SHA',
    snmp_auth_key: '',
    snmp_priv_protocol: 'AES',
    snmp_priv_key: '',
  });

  useEscapeKey({ onEscape: () => setShowModal(false), enabled: showModal });

  // 获取凭证列表
  const { data: credentials = [], isLoading, refetch } = useQuery({
    queryKey: ['snmp-credentials'],
    queryFn: () => api.get('/snmp/credentials').then(res => (res.data.data || res.data) as SnmpCredential[]),
  });

  // 创建/更新 Mutation
  const saveMutation = useMutation({
    mutationFn: async (payload: any) => {
      if (editingCred) {
        return api.put(`/snmp/credentials/${editingCred.id}`, payload);
      }
      return api.post('/snmp/credentials', payload);
    },
    onSuccess: () => {
      toast.success(editingCred ? 'SNMP 凭据已更新' : 'SNMP 凭据添加成功');
      queryClient.invalidateQueries({ queryKey: ['snmp-credentials'] });
      setShowModal(false);
      resetForm();
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || err.message || '保存失败');
    },
  });

  // 删除 Mutation
  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/snmp/credentials/${id}`),
    onSuccess: () => {
      toast.success('SNMP 凭据已删除');
      queryClient.invalidateQueries({ queryKey: ['snmp-credentials'] });
    },
    onError: (err: any) => {
      toast.error(err.response?.data?.error || '删除失败');
    },
  });

  const resetForm = () => {
    setEditingCred(null);
    setShowAuthKey(false);
    setShowPrivKey(false);
    setFormData({
      name: '',
      community: 'public',
      snmp_version: 'v2c',
      snmp_port: 161,
      snmp_user: '',
      snmp_auth_protocol: 'SHA',
      snmp_auth_key: '',
      snmp_priv_protocol: 'AES',
      snmp_priv_key: '',
    });
  };

  const handleOpenAdd = () => {
    resetForm();
    setShowModal(true);
  };

  const handleOpenEdit = (cred: SnmpCredential) => {
    setEditingCred(cred);
    const isCiphertext = cred.community && cred.community.startsWith('2:');
    setShowAuthKey(false);
    setShowPrivKey(false);
    setFormData({
      name: cred.name || '',
      community: isCiphertext ? 'public' : (cred.community || 'public'),
      snmp_version: cred.snmp_version || 'v2c',
      snmp_port: cred.snmp_port || 161,
      snmp_user: cred.snmp_user || '',
      snmp_auth_protocol: cred.snmp_auth_protocol || 'SHA',
      snmp_auth_key: '',
      snmp_priv_protocol: cred.snmp_priv_protocol || 'AES',
      snmp_priv_key: '',
    });
    setShowModal(true);
  };

  const toggleShowPassword = (id: string) => {
    setShowPasswords(prev => ({ ...prev, [id]: !prev[id] }));
  };

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    toast.success('已复制到剪贴板');
    setTimeout(() => setCopiedId(null), 2000);
  };

  const handleTestConnection = async (cred: SnmpCredential) => {
    setTestingId(cred.id);
    try {
      const res = await api.post(`/snmp/credentials/${cred.id}/test`, {
        host: cred.host || '127.0.0.1',
        port: cred.snmp_port || 161,
      });
      const data = res.data?.data;
      const ok = 
        data?.reachable === true || 
        data?.data?.reachable === true || 
        data?.success === true || 
        (data?.code === 0 && res.data?.success !== false);
      if (ok) {
        toast.success(`SNMP 凭据 "${cred.name}" 探测正常 ✅`);
      } else {
        toast.warning(`SNMP 凭据 "${cred.name}" 探测超时或无响应`);
      }
    } catch (err: any) {
      toast.error(err.response?.data?.error || err.response?.data?.message || '测试失败');
    } finally {
      setTestingId(null);
    }
  };

  const filteredCredentials = credentials.filter((c: SnmpCredential) => {
    const q = searchQuery.toLowerCase().trim();
    if (!q) return true;
    return (
      c.name.toLowerCase().includes(q) ||
      (c.community || '').toLowerCase().includes(q) ||
      (c.host || '').toLowerCase().includes(q)
    );
  });

  return (
    <div className="p-6 space-y-6">
      {/* 顶部标题与说明栏 */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 bg-[#111836] border border-blue-500/20 rounded-xl p-5 shadow-lg">
        <div className="flex items-start gap-3.5">
          <div className="p-3 bg-blue-500/10 border border-blue-500/30 rounded-xl text-blue-600">
            <Key className="w-6 h-6" />
          </div>
          <div>
            <h1 className="text-lg font-bold text-text-primary flex items-center gap-2">
              SNMP 只读串 & 密码库
              <span className="text-xs font-normal px-2 py-0.5 rounded-full bg-blue-500/20 text-blue-300 border border-blue-500/30">
                {credentials.length} 个密钥只读串
              </span>
            </h1>
            <p className="text-xs text-text-secondary mt-1 max-w-2xl">
              集中统一管理网络设备的 SNMP Community 只读串密码与令牌。添加网络设备或执行 SNMP 巡检采集时可一键快捷调用。
            </p>
          </div>
        </div>
        <button
          onClick={handleOpenAdd}
          className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600 text-white text-sm font-medium rounded-lg shadow-lg shadow-blue-500/20 transition-all cursor-pointer flex-shrink-0"
        >
          <Plus className="w-4 h-4" />
          新建 SNMP 只读串
        </button>
      </div>

      {/* 快捷推荐预设 Pill 标签 */}
      <div className="flex items-center gap-2 text-xs text-text-secondary overflow-x-auto pb-1">
        <span className="font-semibold text-text-secondary flex items-center gap-1 flex-shrink-0">
          <Shield className="w-3.5 h-3.5 text-amber-600" />
          常用只读串预设:
        </span>
        {['public', 'private', 'huawei@123', 'h3c@readonly', 'cisco_snmp', 'zte_read'].map((preset) => (
          <button
            key={preset}
            onClick={() => {
              setFormData(prev => ({ ...prev, name: `${preset} 凭据`, community: preset, snmp_version: 'v2c', snmp_port: 161 }));
              setShowModal(true);
            }}
            className="px-2.5 py-1 bg-[#111836] border border-blue-500/20 hover:border-blue-400 text-text-secondary hover:text-text-primary rounded-md transition-all font-mono flex items-center gap-1 flex-shrink-0"
          >
            + {preset}
          </button>
        ))}
      </div>

      {/* 搜索与工具栏 */}
      <div className="flex items-center justify-between gap-4">
        <div className="relative flex-1 max-w-md">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="搜索凭据名称、只读串密码、关联主机..."
            className="w-full pl-9 pr-4 py-2 bg-[#111836] border border-blue-500/20 rounded-lg text-sm text-text-primary placeholder-text-tertiary focus:outline-none focus:border-blue-500 transition-colors"
          />
        </div>
        <button
          onClick={() => refetch()}
          className="flex items-center gap-1.5 px-3 py-2 bg-[#111836] border border-blue-500/20 hover:border-blue-500/40 text-text-secondary text-sm rounded-lg transition-colors"
        >
          <RefreshCw className={`w-4 h-4 ${isLoading ? 'animate-spin' : ''}`} />
          刷新列表
        </button>
      </div>

      {/* 凭据卡片列表 / 网格 */}
      {isLoading ? (
        <div className="flex items-center justify-center py-16">
          <RefreshCw className="w-6 h-6 text-blue-600 animate-spin" />
        </div>
      ) : filteredCredentials.length === 0 ? (
        <div className="bg-[#111836] border border-blue-500/20 rounded-xl p-12 text-center">
          <Lock className="w-12 h-12 text-slate-600 mx-auto mb-3" />
          <h3 className="text-base font-semibold text-text-secondary">暂无 SNMP 只读串数据</h3>
          <p className="text-xs text-text-tertiary mt-1 max-w-md mx-auto">
            点击右上角“新建 SNMP 只读串”，添加您的第一条只读串密码令牌，方便后续全网设备一键复用与指标采集。
          </p>
          <button
            onClick={handleOpenAdd}
            className="mt-4 inline-flex items-center gap-2 px-4 py-2 bg-blue-600 text-white text-xs font-medium rounded-lg hover:bg-blue-500 transition-colors"
          >
            <Plus className="w-4 h-4" />
            添加只读串
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {filteredCredentials.map((cred: SnmpCredential) => {
            const isShown = showPasswords[cred.id];
            const displayComm = cred.community || 'public';

            return (
              <div
                key={cred.id}
                className="bg-[#111836] border border-blue-500/20 hover:border-blue-500/50 rounded-xl p-4 transition-all shadow-md group flex flex-col justify-between"
              >
                <div>
                  {/* 卡片头部 */}
                  <div className="flex items-center justify-between mb-3">
                    <div className="flex items-center gap-2.5 min-w-0">
                      <div className="p-2 rounded-lg bg-blue-500/10 text-blue-600 border border-blue-500/30 flex-shrink-0">
                        <Key className="w-4 h-4" />
                      </div>
                      <div className="min-w-0">
                        <h4 className="text-sm font-bold text-text-primary truncate" title={cred.name}>
                          {cred.name}
                        </h4>
                        <span className="text-[10px] font-mono text-text-secondary">
                          ID: {cred.id.slice(0, 8)}...
                        </span>
                      </div>
                    </div>
                    <span className="px-2 py-0.5 rounded text-xs font-bold bg-blue-500/15 text-blue-300 border border-blue-500/30 uppercase">
                      {cred.snmp_version || 'v2c'}
                    </span>
                  </div>

                  {/* 只读串密码 / SNMPv3 安全参数 Display 框 */}
                  {cred.snmp_version === 'v3' ? (
                    <div className="bg-[#0d1225] border border-blue-500/15 rounded-lg p-3 my-2 space-y-2">
                      <div className="flex items-center justify-between text-xs text-text-secondary">
                        <span className="flex items-center gap-1 text-blue-600 font-semibold">
                          <Shield className="w-3.5 h-3.5 text-blue-600" />
                          SNMP v3 安全配置
                        </span>
                        <span className="text-[10px] text-text-tertiary">USM 用户安全模型</span>
                      </div>
                      <div className="grid grid-cols-2 gap-2 text-xs">
                        <div className="bg-surface px-2.5 py-1.5 rounded-lg border border-border">
                          <span className="text-[10px] text-text-secondary block">安全用户名:</span>
                          <span className="font-mono text-cyan-300 font-semibold">{cred.snmp_user || '—'}</span>
                        </div>
                        <div className="bg-surface px-2.5 py-1.5 rounded-lg border border-border">
                          <span className="text-[10px] text-text-secondary block">认证 / 加密:</span>
                          <span className="font-mono text-emerald-600 font-semibold">
                            {cred.snmp_auth_protocol || 'None'} / {cred.snmp_priv_protocol || 'None'}
                          </span>
                        </div>
                      </div>
                    </div>
                  ) : (
                    <div className="bg-[#0d1225] border border-blue-500/15 rounded-lg p-3 my-2 space-y-1.5">
                      <div className="flex items-center justify-between text-xs text-text-secondary">
                        <span>只读串密码 (Community):</span>
                        <div className="flex items-center gap-1.5">
                          <button
                            type="button"
                            onClick={() => toggleShowPassword(cred.id)}
                            className="text-text-secondary hover:text-blue-600 p-1 rounded transition-colors"
                            title={isShown ? '隐藏只读串' : '切换只读串可见性'}
                          >
                            {isShown ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                          </button>
                          <button
                            type="button"
                            onClick={() => handleCopy(displayComm, cred.id)}
                            className="text-text-secondary hover:text-blue-600 p-1 rounded transition-colors"
                            title="复制只读串"
                          >
                            {copiedId === cred.id ? (
                              <span className="flex items-center gap-1 text-emerald-600 text-[10px]">
                                <Check className="w-3.5 h-3.5" />
                                <span>已复制</span>
                              </span>
                            ) : (
                              <span className="flex items-center gap-1 hover:text-blue-300 text-[10px]">
                                <Copy className="w-3.5 h-3.5" />
                                <span>复制</span>
                              </span>
                            )}
                          </button>
                        </div>
                      </div>
                      <div className="font-mono text-sm font-bold text-cyan-300 tracking-wide break-all bg-surface px-3 py-2 rounded-lg border border-cyan-500/20 flex items-center justify-between select-all">
                        <span>{isShown ? '••••••••••••' : displayComm}</span>
                      </div>
                    </div>
                  )}

                  {/* 详细属性属性行 */}
                  <div className="grid grid-cols-2 gap-2 text-xs text-text-secondary pt-1">
                    <div>
                      <span className="text-text-tertiary">SNMP 端口:</span>{' '}
                      <span className="font-mono text-text-secondary font-semibold">{cred.snmp_port || 161}</span>
                    </div>
                    <div className="truncate">
                      <span className="text-text-tertiary">关联主机:</span>{' '}
                      <span className="font-mono text-text-secondary">{cred.host || '全局通用'}</span>
                    </div>
                  </div>
                </div>

                {/* 卡片底栏操作按钮 */}
                <div className="flex items-center justify-between pt-3 mt-3 border-t border-border text-xs">
                  <button
                    onClick={() => handleTestConnection(cred)}
                    disabled={testingId === cred.id}
                    className="flex items-center gap-1 text-emerald-600 hover:text-emerald-300 font-medium disabled:opacity-50"
                  >
                    {testingId === cred.id ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Radio className="w-3.5 h-3.5" />}
                    测试连接
                  </button>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => handleOpenEdit(cred)}
                      className="flex items-center gap-1 text-blue-600 hover:text-blue-700 font-medium"
                    >
                      <Edit3 className="w-3.5 h-3.5" />
                      编辑
                    </button>
                    <button
                      onClick={() => {
                        Modal.confirm({
                          title: '确认删除凭据',
                          content: `确定要删除只读串凭据「${cred.name}」吗？使用此凭据的设备采集将失败。`,
                          okText: '删除',
                          okType: 'danger',
                          cancelText: '取消',
                          onOk: () => deleteMutation.mutate(cred.id),
                        });
                      }}
                      className="flex items-center gap-1 text-red-600 hover:text-red-700 font-medium"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                      删除
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* 新建/编辑 Modal */}
      {showModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-[#111836] border border-blue-500/30 rounded-xl shadow-2xl w-full max-w-lg overflow-hidden">
            <div className="flex items-center justify-between px-5 py-4 border-b border-blue-500/20 bg-[#0d1225]">
              <h3 className="text-base font-bold text-text-primary flex items-center gap-2">
                <Key className="w-4 h-4 text-blue-600" />
                {editingCred
                  ? (formData.snmp_version === 'v3' ? '编辑 SNMP v3 凭证' : '编辑 SNMP 只读串')
                  : (formData.snmp_version === 'v3' ? '新建 SNMP v3 凭据' : '新建 SNMP 只读串 / 密码')}
              </h3>
              <button
                onClick={() => setShowModal(false)}
                className="text-text-secondary hover:text-text-primary transition-colors"
              >
                ✕
              </button>
            </div>

            <form
              onSubmit={(e) => {
                e.preventDefault();
                saveMutation.mutate(formData);
              }}
              className="p-5 space-y-4 max-h-[80vh] overflow-y-auto"
            >
              <div>
                <label className="block text-xs font-semibold text-text-secondary mb-1">
                  凭据名称 <span className="text-red-600">*</span>
                </label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  placeholder="例：华为核心交换机只读串 / 华为v3安全凭据"
                  className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">SNMP 版本</label>
                  <select
                    value={formData.snmp_version}
                    onChange={(e) => setFormData({ ...formData, snmp_version: e.target.value })}
                    className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500 cursor-pointer"
                  >
                    <option value="v2c">v2c (最常用)</option>
                    <option value="v3">v3 (安全用户认证/加密)</option>
                    <option value="v1">v1</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">SNMP 端口</label>
                  <input
                    type="number"
                    value={formData.snmp_port}
                    onChange={(e) => setFormData({ ...formData, snmp_port: parseInt(e.target.value) || 161 })}
                    className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-sm font-mono text-text-primary focus:outline-none focus:border-blue-500"
                  />
                </div>
              </div>

              {formData.snmp_version !== 'v3' ? (
                <div>
                  <label className="block text-xs font-semibold text-text-secondary mb-1">
                    SNMP 只读串密码 (Community String) <span className="text-red-600">*</span>
                  </label>
                  <input
                    type="text"
                    required
                    value={formData.community}
                    onChange={(e) => setFormData({ ...formData, community: e.target.value })}
                    placeholder="例：public, huawei@123, h3c@readonly"
                    className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-sm font-mono text-cyan-300 focus:outline-none focus:border-blue-500"
                  />
                  <p className="text-[11px] text-text-tertiary mt-1">
                    即设备的 SNMP 访问口令/密码串（如 public、private 或厂商专用口令）。
                  </p>
                </div>
              ) : (
                <div className="bg-[#090d1a] border border-blue-500/20 rounded-xl p-4 space-y-3.5">
                  <div className="flex items-center gap-2 pb-2 border-b border-blue-500/15">
                    <Shield className="w-4 h-4 text-blue-600" />
                    <span className="text-xs font-bold text-blue-300">SNMP v3 安全参数 (USM)</span>
                  </div>

                  <div>
                    <label className="block text-xs font-semibold text-text-secondary mb-1">
                      安全用户名 (Username) <span className="text-red-600">*</span>
                    </label>
                    <input
                      type="text"
                      required
                      value={formData.snmp_user}
                      onChange={(e) => setFormData({ ...formData, snmp_user: e.target.value })}
                      placeholder="例如: snmpuser, admin_v3"
                      className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-sm font-mono text-cyan-300 focus:outline-none focus:border-blue-500"
                    />
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <div>
                      <label className="block text-xs font-semibold text-text-secondary mb-1">认证协议 (Auth)</label>
                      <select
                        value={formData.snmp_auth_protocol}
                        onChange={(e) => setFormData({ ...formData, snmp_auth_protocol: e.target.value })}
                        className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500"
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
                          value={formData.snmp_auth_key}
                          onChange={(e) => setFormData({ ...formData, snmp_auth_key: e.target.value })}
                          placeholder="认证密钥密码"
                          className="w-full pl-3 pr-8 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-xs font-mono text-text-primary focus:outline-none focus:border-blue-500"
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
                        value={formData.snmp_priv_protocol}
                        onChange={(e) => setFormData({ ...formData, snmp_priv_protocol: e.target.value })}
                        className="w-full px-3 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-xs text-text-primary focus:outline-none focus:border-blue-500"
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
                          value={formData.snmp_priv_key}
                          onChange={(e) => setFormData({ ...formData, snmp_priv_key: e.target.value })}
                          placeholder="数据隐私加密密码"
                          className="w-full pl-3 pr-8 py-2 bg-[#0d1225] border border-blue-500/20 rounded-lg text-xs font-mono text-text-primary focus:outline-none focus:border-blue-500"
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

              <div className="flex items-center justify-end gap-3 pt-4 border-t border-border">
                <button
                  type="button"
                  onClick={() => setShowModal(false)}
                  className="px-4 py-2 bg-surface-2 hover:bg-surface-hover text-text-secondary text-xs font-medium rounded-lg transition-colors"
                >
                  取消
                </button>
                <button
                  type="submit"
                  disabled={saveMutation.isPending}
                  className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-xs font-medium rounded-lg transition-colors disabled:opacity-50"
                >
                  {saveMutation.isPending ? '保存中...' : '保存凭据'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
