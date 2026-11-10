import { useState, useEffect as _useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Network, Plus, Trash2, Play, Loader2, CheckCircle2, AlertCircle,
  Monitor, Wifi, Search, RefreshCw as _RefreshCw, Eye, EyeOff, Key,
  Server as _Server, List, Radio, Activity, Terminal, Settings, Database
} from 'lucide-react';
import clsx from 'clsx';
import api from '../../../lib/api';

interface SnmpCredential {
  id: string;
  device_id?: string;
  name: string;
  snmp_version: string;
  snmp_port: number;
  snmp_user?: string;
  snmp_auth_protocol?: string;
  snmp_priv_protocol?: string;
  community?: string;
  host?: string;
  created_at: string;
  updated_at: string;
}

type ApiError = { response?: { data?: { message?: string } }; message?: string };

type SnmpQueryResult = {
  type: 'system-info';
  data: Record<string, unknown>;
} | {
  type: 'interfaces';
  data: SnmpInterface[];
} | {
  type: 'error';
  data: string;
};

interface SnmpInterface {
  index: string;
  name: string;
  descr: string;
  operStatus: string;
  speed?: number;
  physAddr?: string;
}

interface SnmpTrap {
  received_at?: string;
  timestamp?: string;
  sourceIp?: string;
  source?: string;
  severity?: string;
  message?: string;
  description?: string;
  data?: unknown;
  oid?: string;
}

const _VERDOR_OPTIONS = [
  'Cisco', 'Huawei', 'H3C', 'Juniper',
  'Ruijie', 'Dell', 'HP', 'MikroTik',
  'TP-Link', 'Ubiquiti', 'Other'
];

const VERSIONS = ['v1', 'v2c', 'v3'];
const AUTH_PROTOCOLS = ['MD5', 'SHA'];
const PRIV_PROTOCOLS = ['DES', 'AES'];

export default function SNMP() {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState('credentials');
  const [searchQuery, setSearchQuery] = useState('');

  // ── 凭证列表 ──
  const { data: credentials = [], isLoading: credsLoading } = useQuery({
    queryKey: ['snmp-credentials'],
    queryFn: () => api.get('/snmp/credentials').then(r => r.data.data || []),
  });

  // ── 新建/编辑 表单 ──
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState({
    name: '',
    host: '',
    port: 161,
    version: 'v2c' as string,
    community: 'public',
    user: '',
    authProtocol: '' as string,
    authKey: '',
    privProtocol: '' as string,
    privKey: '',
  });
  const [showAuthKey, setShowAuthKey] = useState(false);
  const [showPrivKey, setShowPrivKey] = useState(false);

  // ── 连接测试 ──
  const [testResult, setTestResult] = useState<{ host: string; status: 'testing' | 'success' | 'fail'; msg?: string } | null>(null);

  const testConn = useMutation({
    mutationFn: () => api.post('/snmp/test', {
      host: form.host,
      port: form.port,
      version: form.version,
      community: form.community,
    }),
    onMutate: () => setTestResult({ host: form.host, status: 'testing' }),
    onSuccess: (res) => {
      const data = res.data?.data;
      const ok = 
        data?.reachable === true || 
        data?.data?.reachable === true || 
        data?.success === true || 
        (data?.code === 0 && res.data?.success !== false);
      if (ok) {
        setTestResult({ host: form.host, status: 'success' });
      } else {
        setTestResult({ host: form.host, status: 'fail', msg: data?.message || res.data?.message || '连接失败' });
      }
    },
    onError: (err: ApiError) => {
      setTestResult({ host: form.host, status: 'fail', msg: err.response?.data?.message || err.message });
    },
  });

  const saveCred = useMutation({
    mutationFn: () => {
      const body = {
        name: form.name,
        community: form.community,
        snmp_version: form.version,
        snmp_port: form.port,
        snmp_user: form.user || undefined,
        snmp_auth_protocol: form.authProtocol || undefined,
        snmp_auth_key: form.authKey || undefined,
        snmp_priv_protocol: form.privProtocol || undefined,
        snmp_priv_key: form.privKey || undefined,
        host: form.host || undefined,
      };
      if (editingId) {
        return api.put(`/snmp/credentials/${editingId}`, body);
      }
      return api.post('/snmp/credentials', body);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['snmp-credentials'] });
      resetForm();
    },
    onError: (err: ApiError) => {
      alert('保存失败: ' + (err.response?.data?.message || err.message));
    },
  });

  const deleteCred = useMutation({
    mutationFn: (id: string) => api.delete(`/snmp/credentials/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['snmp-credentials'] }),
  });

  // 凭证列表中的测试按钮（使用存储的凭证信息）
  const [credTestResults, setCredTestResults] = useState<Record<string, { status: 'testing' | 'success' | 'fail'; msg?: string }>>({});

  const testCred = useMutation({
    mutationFn: (cred: SnmpCredential) => api.post(`/snmp/credentials/${cred.id}/test`, { host: cred.host || undefined, port: cred.snmp_port || 161 }),
    onMutate: (cred) => {
      setCredTestResults(prev => ({ ...prev, [cred.id]: { status: 'testing' } }));
    },
    onSuccess: (res, cred) => {
      const data = res.data?.data;
      const ok = 
        data?.reachable === true || 
        data?.data?.reachable === true || 
        data?.success === true || 
        (data?.code === 0 && res.data?.success !== false);
      setCredTestResults(prev => ({
        ...prev,
        [cred.id]: ok
          ? { status: 'success' }
          : { status: 'fail', msg: data?.message || res.data?.message || '连接失败' },
      }));
      // 3秒后自动清除
      setTimeout(() => setCredTestResults(prev => { const n = { ...prev }; delete n[cred.id]; return n; }), 3000);
    },
    onError: (err: ApiError, cred: SnmpCredential) => {
      setCredTestResults(prev => ({
        ...prev,
        [cred.id]: { status: 'fail', msg: err.response?.data?.message || err.message },
      }));
      setTimeout(() => setCredTestResults(prev => { const n = { ...prev }; delete n[cred.id]; return n; }), 3000);
    },
  });

  const resetForm = () => {
    setForm({ name: '', host: '', port: 161, version: 'v2c', community: 'public', user: '', authProtocol: '', authKey: '', privProtocol: '', privKey: '' });
    setEditingId(null);
    setTestResult(null);
  };

  // ── SNMP 查询区 ──
  const [queryHost, setQueryHost] = useState('');
  const [queryCommunity, setQueryCommunity] = useState('public');
  const [queryVersion, setQueryVersion] = useState('v2c');
  const [queryResult, setQueryResult] = useState<SnmpQueryResult | null>(null);
  const [queryLoading, setQueryLoading] = useState(false);

  const fetchSystemInfo = async () => {
    setQueryLoading(true);
    setQueryResult(null);
    try {
      const res = await api.post('/snmp/system-info', {
        host: queryHost, community: queryCommunity, version: queryVersion,
      });
      setQueryResult({ type: 'system-info', data: res.data.data });
    } catch (err: unknown) {
      const e = err as ApiError;
      setQueryResult({ type: 'error', data: e.response?.data?.message || e.message || '请求失败' });
    }
    setQueryLoading(false);
  };

  const fetchInterfaces = async () => {
    setQueryLoading(true);
    setQueryResult(null);
    try {
      const res = await api.post('/snmp/interfaces', {
        host: queryHost, community: queryCommunity, version: queryVersion,
      });
      setQueryResult({ type: 'interfaces', data: res.data.data });
    } catch (err: unknown) {
      const e = err as ApiError;
      setQueryResult({ type: 'error', data: e.response?.data?.message || e.message || '请求失败' });
    }
    setQueryLoading(false);
  };

  // ── Trap 历史 ──
  const { data: traps = [], isLoading: trapsLoading } = useQuery({
    queryKey: ['snmp-traps'],
    queryFn: () => api.get('/snmp/traps?limit=50').then(r => r.data.data || []),
    refetchInterval: 30000,
  });

  const testTrapMutation = useMutation({
    mutationFn: () => api.post('/snmp/traps/test'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['snmp-traps'] });
    },
  });

  // ── 自定义 OID 采集 ──
  interface CustomOidProfile {
    id: string;
    name: string;
    vendor: string | null;
    description: string | null;
    oids_json: string;
    poll_interval_seconds: number;
    enabled: number | boolean;
    created_at: string;
    updated_at: string;
  }
  interface CustomOidResult {
    id: string;
    device_id: string;
    profile_id: string;
    metric_name: string;
    metric_value: string;
    oid: string;
    sampled_at: string;
  }
  interface OidEntry { oid: string; metric_name: string; description?: string }
  interface NetworkDevice {
    id: string;
    name: string;
    ip_address: string;
    vendor?: string;
    device_type?: string;
    status?: string;
  }

  const { data: customOidProfiles = [], isLoading: profilesLoading } = useQuery({
    queryKey: ['snmp-custom-oids'],
    queryFn: () => api.get('/snmp/custom-oids').then(r => r.data.data || []),
  });

  const { data: networkDevices = [] } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(r => r.data.data || []),
  });

  // Profile form state
  const [showProfileForm, setShowProfileForm] = useState(false);
  const [editingProfileId, setEditingProfileId] = useState<string | null>(null);
  const [profileForm, setProfileForm] = useState({
    name: '',
    vendor: '',
    description: '',
    poll_interval_seconds: 300,
    enabled: true,
    oids: [{ oid: '', metric_name: '', description: '' }] as OidEntry[],
  });

  const resetProfileForm = () => {
    setProfileForm({ name: '', vendor: '', description: '', poll_interval_seconds: 300, enabled: true, oids: [{ oid: '', metric_name: '', description: '' }] });
    setEditingProfileId(null);
    setShowProfileForm(false);
  };

  const saveProfile = useMutation({
    mutationFn: () => {
      const body = {
        name: profileForm.name,
        vendor: profileForm.vendor || null,
        description: profileForm.description || null,
        oids: profileForm.oids.filter(o => o.oid && o.metric_name),
        poll_interval_seconds: profileForm.poll_interval_seconds,
        enabled: profileForm.enabled,
      };
      if (editingProfileId) {
        return api.put(`/snmp/custom-oids/${editingProfileId}`, body);
      }
      return api.post('/snmp/custom-oids', body);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['snmp-custom-oids'] });
      resetProfileForm();
    },
  });

  const deleteProfile = useMutation({
    mutationFn: (id: string) => api.delete(`/snmp/custom-oids/${id}`),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['snmp-custom-oids'] }),
  });

  // Results viewer state
  const [resultDeviceId, setResultDeviceId] = useState('');
  const [collectingProfileId, setCollectingProfileId] = useState<string | null>(null);
  const [collectStatus, setCollectStatus] = useState<Record<string, { status: 'loading' | 'success' | 'fail'; msg?: string }>>({});

  const { data: customOidResults = [], isLoading: resultsLoading } = useQuery({
    queryKey: ['snmp-custom-oid-results', resultDeviceId],
    queryFn: () => api.get(`/snmp/custom-oids/results/${resultDeviceId}?limit=100`).then(r => r.data.data || []),
    enabled: !!resultDeviceId,
  });

  const triggerCollect = async (profileId: string, deviceId: string) => {
    const key = `${profileId}-${deviceId}`;
    setCollectStatus(prev => ({ ...prev, [key]: { status: 'loading' } }));
    setCollectingProfileId(profileId);
    try {
      const res = await api.post(`/snmp/custom-oids/${profileId}/collect/${deviceId}`);
      if (res.data.success !== false) {
        setCollectStatus(prev => ({ ...prev, [key]: { status: 'success' } }));
        queryClient.invalidateQueries({ queryKey: ['snmp-custom-oid-results', deviceId] });
      } else {
        setCollectStatus(prev => ({ ...prev, [key]: { status: 'fail', msg: res.data.message || '采集失败' } }));
      }
    } catch (err: unknown) {
      const e = err as ApiError;
      setCollectStatus(prev => ({ ...prev, [key]: { status: 'fail', msg: e.response?.data?.message || e.message || '采集失败' } }));
    }
    setCollectingProfileId(null);
    setTimeout(() => setCollectStatus(prev => { const n = { ...prev }; delete n[key]; return n; }), 5000);
  };

  // Parse oids_json for display
  const parseOids = (json: string): OidEntry[] => {
    try { return JSON.parse(json) || []; } catch { return []; }
  };

  return (
    <div className="h-full overflow-auto p-6">
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-text-primary mb-2 flex items-center gap-3">
              <Radio className="w-7 h-7 text-emerald-400" />
              SNMP 管理
            </h1>
            <p className="text-text-secondary">SNMP 凭证管理、设备发现与网络监控</p>
          </div>
        </div>

        {/* Tabs */}
        <div className="flex gap-1 bg-background rounded-lg p-1 border border-border">
          {[
            { id: 'credentials', label: '凭证管理', icon: Key },
            { id: 'query', label: 'SNMP 查询', icon: Search },
            { id: 'traps', label: 'Trap 接收', icon: Activity },
            { id: 'custom-oid', label: '自定义 OID', icon: Settings },
          ].map(tab => (
            <button key={tab.id}
              onClick={() => setActiveTab(tab.id)}
              className={clsx(
                'flex items-center gap-2 px-4 py-2 rounded-md text-sm font-medium transition-all',
                activeTab === tab.id ? 'bg-primary text-white shadow-sm' : 'text-text-secondary hover:text-text-primary'
              )}
            >
              <tab.icon className="w-4 h-4" />
              {tab.label}
            </button>
          ))}
        </div>

        {/* ======================== Tab 1: 凭证管理 ======================== */}
        {activeTab === 'credentials' && (
          <div className="space-y-4">
            {/* 工具栏 */}
            <div className="flex items-center justify-between">
              <div className="relative w-64">
                <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-text-tertiary" />
                <input type="text" placeholder="搜索凭证..."
                  value={searchQuery}
                  onChange={e => setSearchQuery(e.target.value)}
                  className="w-full pl-9 pr-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                />
              </div>
              <button onClick={() => { setShowForm(!showForm); if (!showForm) { setEditingId(null); resetForm(); } }}
                className="flex items-center gap-2 px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all"
              >
                <Plus className="w-4 h-4" />
                新增凭证
              </button>
            </div>

            {/* 新增表单 */}
            {showForm && (
              <div className="bg-surface rounded-xl border border-border p-5 space-y-4">
                <h3 className="font-medium text-text-primary">{editingId ? '编辑 SNMP 凭证' : '新增 SNMP 凭证'}</h3>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-xs text-text-secondary mb-1.5">凭证名称 *</label>
                    <input type="text" placeholder="例如: 核心交换机"
                      value={form.name}
                      onChange={e => setForm({ ...form, name: e.target.value })}
                      className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-text-secondary mb-1.5">设备 IP/Host *</label>
                    <input type="text" placeholder="192.168.1.1"
                      value={form.host}
                      onChange={e => setForm({ ...form, host: e.target.value })}
                      className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-text-secondary mb-1.5">端口</label>
                    <input type="number" min="1" max="65535"
                      value={form.port}
                      onChange={e => setForm({ ...form, port: parseInt(e.target.value) || 161 })}
                      className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-text-secondary mb-1.5">SNMP 版本</label>
                    <select value={form.version}
                      onChange={e => setForm({ ...form, version: e.target.value })}
                      className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                    >
                      {VERSIONS.map(v => <option key={v} value={v}>{v.toUpperCase()}</option>)}
                    </select>
                  </div>
                  {form.version !== 'v3' ? (
                    <div>
                      <label className="block text-xs text-text-secondary mb-1.5">Community</label>
                      <input type="text" placeholder="public"
                        value={form.community}
                        onChange={e => setForm({ ...form, community: e.target.value })}
                        className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                      />
                    </div>
                  ) : (
                    <>
                      <div>
                        <label className="block text-xs text-text-secondary mb-1.5">用户名</label>
                        <input type="text"
                          value={form.user}
                          onChange={e => setForm({ ...form, user: e.target.value })}
                          className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                        />
                      </div>
                      <div>
                        <label className="block text-xs text-text-secondary mb-1.5">认证协议</label>
                        <select value={form.authProtocol}
                          onChange={e => setForm({ ...form, authProtocol: e.target.value })}
                          className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                        >
                          <option value="">无</option>
                          {AUTH_PROTOCOLS.map(p => <option key={p} value={p}>{p}</option>)}
                        </select>
                      </div>
                      <div>
                        <label className="block text-xs text-text-secondary mb-1.5">认证密钥</label>
                        <div className="relative">
                          <input type={showAuthKey ? 'text' : 'password'}
                            value={form.authKey}
                            onChange={e => setForm({ ...form, authKey: e.target.value })}
                            className="w-full px-3 py-2 pr-8 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                          />
                          <button type="button" onClick={() => setShowAuthKey(!showAuthKey)}
                            className="absolute right-2 top-1/2 -translate-y-1/2 text-text-tertiary hover:text-text-secondary"
                          >{showAuthKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}</button>
                        </div>
                      </div>
                      <div>
                        <label className="block text-xs text-text-secondary mb-1.5">加密协议</label>
                        <select value={form.privProtocol}
                          onChange={e => setForm({ ...form, privProtocol: e.target.value })}
                          className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                        >
                          <option value="">无</option>
                          {PRIV_PROTOCOLS.map(p => <option key={p} value={p}>{p}</option>)}
                        </select>
                      </div>
                      <div>
                        <label className="block text-xs text-text-secondary mb-1.5">加密密钥</label>
                        <div className="relative">
                          <input type={showPrivKey ? 'text' : 'password'}
                            value={form.privKey}
                            onChange={e => setForm({ ...form, privKey: e.target.value })}
                            className="w-full px-3 py-2 pr-8 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                          />
                          <button type="button" onClick={() => setShowPrivKey(!showPrivKey)}
                            className="absolute right-2 top-1/2 -translate-y-1/2 text-text-tertiary hover:text-text-secondary"
                          >{showPrivKey ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}</button>
                        </div>
                      </div>
                    </>
                  )}
                </div>

                {/* 操作按钮 */}
                <div className="flex items-center gap-3 pt-2">
                  {testResult?.host === form.host && testResult.status === 'testing' && (
                    <span className="flex items-center gap-1.5 text-xs text-blue-400">
                      <Loader2 className="w-3.5 h-3.5 animate-spin" /> 测试连接中...
                    </span>
                  )}
                  {testResult?.status === 'success' && (
                    <span className="flex items-center gap-1.5 text-xs text-status-success">
                      <CheckCircle2 className="w-3.5 h-3.5" /> 连接成功
                    </span>
                  )}
                  {testResult?.status === 'fail' && (
                    <span className="flex items-center gap-1.5 text-xs text-status-failed">
                      <AlertCircle className="w-3.5 h-3.5" /> {testResult.msg}
                    </span>
                  )}
                  <div className="flex-1" />
                  {form.host && (
                    <button onClick={() => testConn.mutate()} disabled={testConn.isPending}
                      className="px-3 py-1.5 text-xs bg-surface border border-border text-text-secondary rounded-lg hover:bg-surface/80 transition-all disabled:opacity-50 flex items-center gap-1.5"
                    >
                      {testConn.isPending ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Play className="w-3.5 h-3.5" />}
                      测试连接
                    </button>
                  )}
                  <button onClick={() => { setShowForm(false); resetForm(); }}
                    className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary transition-colors"
                  >取消</button>
                  <button onClick={() => saveCred.mutate()} disabled={!form.name || !form.host || saveCred.isPending}
                    className="px-4 py-1.5 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all disabled:opacity-50 text-sm flex items-center gap-1.5"
                  >
                    {saveCred.isPending && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
                    {editingId ? '更新凭证' : '保存凭证'}
                  </button>
                </div>
              </div>
            )}

            {/* 凭证列表 */}
            <div className="space-y-2">
              {credsLoading ? (
                <div className="flex items-center justify-center py-12">
                  <Loader2 className="w-6 h-6 animate-spin text-text-tertiary" />
                </div>
              ) : credentials.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-16 text-text-tertiary">
                  <Radio className="w-12 h-12 mb-3 opacity-30" />
                  <p className="text-sm">暂无 SNMP 凭证</p>
                  <p className="text-xs mt-1">点击"新增凭证"添加网络设备的 SNMP 配置</p>
                </div>
              ) : (
                credentials.filter((c: SnmpCredential) =>
                  !searchQuery || c.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
                  (c.snmp_user || '').toLowerCase().includes(searchQuery.toLowerCase())
                ).map((cred: SnmpCredential) => (
                  <div key={cred.id}
                    className="bg-surface rounded-xl border border-border p-4 flex items-center justify-between hover:border-emerald-500/30 transition-all"
                  >
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                        <Network className="w-5 h-5 text-emerald-400" />
                      </div>
                      <div>
                        <p className="font-medium text-text-primary">{cred.name}</p>
                        <div className="flex items-center gap-3 mt-1 text-xs text-text-secondary">
                          <span>SNMP {cred.snmp_version.toUpperCase()}</span>
                          <span>端口 {cred.snmp_port}</span>
                          {cred.snmp_user && <span>用户 {cred.snmp_user}</span>}
                          {cred.snmp_auth_protocol && <span>认证 {cred.snmp_auth_protocol}</span>}
                          {cred.host && <span>IP {cred.host}</span>}
                          {cred.snmp_priv_protocol && <span>加密 {cred.snmp_priv_protocol}</span>}
                          <span>创建于 {new Date(cred.created_at).toLocaleDateString()}</span>
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {/* 测试结果反馈 */}
                      {credTestResults[cred.id]?.status === 'testing' && (
                        <Loader2 className="w-4 h-4 animate-spin text-emerald-400" />
                      )}
                      {credTestResults[cred.id]?.status === 'success' && (
                        <CheckCircle2 className="w-4 h-4 text-status-success" />
                      )}
                      {credTestResults[cred.id]?.status === 'fail' && (
                        <span className="text-xs text-status-failed">{credTestResults[cred.id].msg}</span>
                      )}

                      {/* 测试连接 */}
                      <button onClick={() => testCred.mutate(cred)}
                        disabled={credTestResults[cred.id]?.status === 'testing'}
                        className="p-2 text-text-tertiary hover:text-primary transition-colors disabled:opacity-50"
                        title="测试连接"
                      >
                        {credTestResults[cred.id]?.status === 'testing' ? (
                          <Loader2 className="w-4 h-4 animate-spin text-emerald-400" />
                        ) : (
                          <Play className="w-4 h-4" />
                        )}
                      </button>

                      {/* 编辑 */}
                      <button onClick={() => {
                        setEditingId(cred.id);
                        setForm({
                          name: cred.name,
                          host: cred.host || '',
                          port: cred.snmp_port,
                          version: cred.snmp_version,
                          community: cred.community || 'public',
                          user: cred.snmp_user || '',
                          authProtocol: cred.snmp_auth_protocol || '',
                          authKey: '',
                          privProtocol: cred.snmp_priv_protocol || '',
                          privKey: '',
                        });
                        setShowForm(true);
                      }}
                        className="p-2 text-text-tertiary hover:text-emerald-400 transition-colors"
                        title="编辑"
                      >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                        </svg>
                      </button>

                      {/* 删除 */}
                      <button onClick={() => deleteCred.mutate(cred.id)}
                        className="p-2 text-text-tertiary hover:text-status-failed transition-colors"
                        title="删除"
                      >
                        <Trash2 className="w-4 h-4" />
                      </button>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        )}

        {/* ======================== Tab 2: SNMP 查询 ======================== */}
        {activeTab === 'query' && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
            {/* 查询控制面板 */}
            <div className="bg-surface rounded-xl border border-border p-5 space-y-4">
              <h3 className="font-medium text-text-primary flex items-center gap-2">
                <Search className="w-4 h-4" />
                SNMP 查询
              </h3>
              <div>
                <label className="block text-xs text-text-secondary mb-1.5">目标设备 IP</label>
                <input type="text" placeholder="192.168.1.1"
                  value={queryHost}
                  onChange={e => setQueryHost(e.target.value)}
                  className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs text-text-secondary mb-1.5">Community</label>
                  <input type="text" placeholder="public"
                    value={queryCommunity}
                    onChange={e => setQueryCommunity(e.target.value)}
                    className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                  />
                </div>
                <div>
                  <label className="block text-xs text-text-secondary mb-1.5">SNMP 版本</label>
                  <select value={queryVersion}
                    onChange={e => setQueryVersion(e.target.value)}
                    className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                  >
                    {VERSIONS.map(v => <option key={v} value={v}>{v.toUpperCase()}</option>)}
                  </select>
                </div>
              </div>
              <div className="flex gap-2">
                <button onClick={fetchSystemInfo} disabled={!queryHost || queryLoading}
                  className="flex-1 px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary hover:border-emerald-400/50 transition-all disabled:opacity-50 flex items-center justify-center gap-2"
                >
                  {queryLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Monitor className="w-4 h-4 text-emerald-400" />}
                  系统信息
                </button>
                <button onClick={fetchInterfaces} disabled={!queryHost || queryLoading}
                  className="flex-1 px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary hover:border-emerald-400/50 transition-all disabled:opacity-50 flex items-center justify-center gap-2"
                >
                  {queryLoading ? <Loader2 className="w-4 h-4 animate-spin" /> : <Wifi className="w-4 h-4 text-emerald-400" />}
                  接口列表
                </button>
              </div>
            </div>

            {/* 查询结果 */}
            <div className="lg:col-span-2 bg-surface rounded-xl border border-border p-5 min-h-[400px]">
              {queryLoading ? (
                <div className="flex items-center justify-center h-64">
                  <Loader2 className="w-8 h-8 animate-spin text-text-tertiary" />
                </div>
              ) : !queryResult ? (
                <div className="flex flex-col items-center justify-center h-64 text-text-tertiary">
                  <Search className="w-12 h-12 mb-3 opacity-30" />
                  <p className="text-sm">输入目标 IP 后点击查询</p>
                </div>
              ) : queryResult.type === 'error' ? (
                <div className="flex flex-col items-center justify-center h-64 text-status-failed">
                  <AlertCircle className="w-12 h-12 mb-3 opacity-50" />
                  <p className="text-sm">{queryResult.data}</p>
                </div>
              ) : queryResult.type === 'system-info' ? (
                <div className="space-y-3">
                  <div className="flex items-center gap-2 mb-4">
                    <Monitor className="w-4 h-4 text-emerald-400" />
                    <h4 className="font-medium text-text-primary">系统信息 - {queryHost}</h4>
                  </div>
                  {queryResult.data && Object.entries(queryResult.data).map(([key, val]) => (
                    <div key={key} className="flex items-center gap-3 py-2 border-b border-border last:border-0">
                      <span className="text-sm text-text-secondary min-w-[140px] font-mono">{key}</span>
                      <span className="text-sm text-text-primary">{String(val)}</span>
                    </div>
                  ))}
                </div>
              ) : queryResult.type === 'interfaces' ? (
                <div className="space-y-3">
                  <div className="flex items-center gap-2 mb-4">
                    <List className="w-4 h-4 text-emerald-400" />
                    <h4 className="font-medium text-text-primary">接口列表 - {queryHost}</h4>
                    <span className="text-xs text-text-secondary ml-2">共 {queryResult.data?.length || 0} 个接口</span>
                  </div>
                  {queryResult.data?.length > 0 ? (
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="text-left text-text-secondary border-b border-border">
                            <th className="pb-2 pr-4">索引</th>
                            <th className="pb-2 pr-4">名称</th>
                            <th className="pb-2 pr-4">描述</th>
                            <th className="pb-2 pr-4">状态</th>
                            <th className="pb-2 pr-4">速率</th>
                            <th className="pb-2 pr-4">MAC</th>
                          </tr>
                        </thead>
                        <tbody>
                          {queryResult.data.map((iface: SnmpInterface, idx: number) => (
                            <tr key={idx} className="border-b border-border/50 hover:bg-background/50">
                              <td className="py-2 pr-4 font-mono text-text-tertiary">{iface.index}</td>
                              <td className="py-2 pr-4 text-text-primary">{iface.name}</td>
                              <td className="py-2 pr-4 text-text-secondary max-w-[200px] truncate">{iface.descr}</td>
                              <td className="py-2 pr-4">
                                <span className={clsx(
                                  'text-xs px-2 py-0.5 rounded-full',
                                  iface.operStatus === 'up' ? 'bg-status-success/10 text-status-success' : 'bg-status-failed/10 text-status-failed'
                                )}>
                                  {iface.operStatus === 'up' ? 'UP' : 'DOWN'}
                                </span>
                              </td>
                              <td className="py-2 pr-4 text-text-secondary">{iface.speed ? `${(iface.speed / 1e6).toFixed(0)} Mbps` : '-'}</td>
                              <td className="py-2 font-mono text-xs text-text-tertiary">{iface.physAddr || '-'}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <p className="text-text-tertiary text-sm py-4 text-center">未获取到接口数据</p>
                  )}
                </div>
              ) : null}
            </div>
          </div>
        )}

        {/* ======================== Tab 3: Trap 接收 ======================== */}
        {activeTab === 'traps' && (
          <div className="space-y-4">
            <div className="bg-surface rounded-xl border border-border p-5">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-2">
                  <Activity className="w-5 h-5 text-emerald-400" />
                  <h3 className="font-medium text-text-primary">SNMP Trap 接收记录</h3>
                </div>
                <div className="flex items-center gap-2 text-xs text-text-secondary">
                  <span className="flex items-center gap-1">
                    <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
                    监听中 (端口 162)
                  </span>
                  <span>自动刷新 30s</span>
                  <button
                    onClick={() => testTrapMutation.mutate()}
                    disabled={testTrapMutation.isPending}
                    className="flex items-center gap-1 px-2 py-1 text-xs bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 rounded-md hover:bg-emerald-500/20 transition-colors disabled:opacity-50"
                  >
                    {testTrapMutation.isPending ? <Loader2 className="w-3 h-3 animate-spin" /> : <Plus className="w-3 h-3" />}
                    生成测试 Trap
                  </button>
                </div>
              </div>

              {trapsLoading ? (
                <div className="flex items-center justify-center py-16">
                  <Loader2 className="w-8 h-8 animate-spin text-text-tertiary" />
                </div>
              ) : traps.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-16 text-text-tertiary">
                  <Activity className="w-12 h-12 mb-3 opacity-30" />
                  <p className="text-sm">暂无 Trap 记录</p>
                  <p className="text-xs mt-1">当网络设备发送 SNMP Trap 时，将在此显示</p>
                </div>
              ) : (
                <div className="space-y-2">
                  {traps.map((trap: SnmpTrap, idx: number) => (
                    <div key={idx} className="bg-background rounded-lg p-3 border border-border/50">
                      <div className="flex items-center justify-between mb-1">
                        <div className="flex items-center gap-2">
                          <span className="text-xs font-mono text-text-tertiary">
                            {new Date((trap.received_at || trap.timestamp) ?? 0).toLocaleString()}
                          </span>
                          <span className="text-xs text-primary font-mono">{trap.sourceIp || trap.source}</span>
                        </div>
                        {trap.severity && (
                          <span className={clsx(
                            'text-xs px-2 py-0.5 rounded-full',
                            trap.severity === 'critical' ? 'bg-status-failed/10 text-status-failed' :
                            trap.severity === 'warning' ? 'bg-yellow-500/10 text-yellow-400' :
                            'bg-blue-500/10 text-blue-400'
                          )}>
                            {trap.severity}
                          </span>
                        )}
                      </div>
                      <p className="text-sm text-text-primary">{trap.message || trap.description || JSON.stringify(trap.data || trap)}</p>
                      {trap.oid && <p className="text-xs font-mono text-text-tertiary mt-1">{trap.oid}</p>}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </div>
        )}

        {/* ======================== Tab 4: 自定义 OID 采集 ======================== */}
        {activeTab === 'custom-oid' && (
          <div className="space-y-4">
            {/* 工具栏 */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Database className="w-5 h-5 text-emerald-400" />
                <h3 className="font-medium text-text-primary">自定义 OID 采集配置</h3>
                <span className="text-xs text-text-secondary">({customOidProfiles.length} 个配置)</span>
              </div>
              <button onClick={() => { setShowProfileForm(!showProfileForm); if (!showProfileForm) { setEditingProfileId(null); setProfileForm({ name: '', vendor: '', description: '', poll_interval_seconds: 300, enabled: true, oids: [{ oid: '', metric_name: '', description: '' }] }); } }}
                className="flex items-center gap-2 px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all"
              >
                <Plus className="w-4 h-4" />
                新建采集配置
              </button>
            </div>

            {/* 新建/编辑表单 */}
            {showProfileForm && (
              <div className="bg-surface rounded-xl border border-border p-5 space-y-4">
                <h3 className="font-medium text-text-primary">{editingProfileId ? '编辑采集配置' : '新建采集配置'}</h3>
                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                  <div>
                    <label className="block text-xs text-text-secondary mb-1.5">配置名称 *</label>
                    <input type="text" placeholder="例如: 华为CPU监控"
                      value={profileForm.name}
                      onChange={e => setProfileForm({ ...profileForm, name: e.target.value })}
                      className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                    />
                  </div>
                  <div>
                    <label className="block text-xs text-text-secondary mb-1.5">适用厂商</label>
                    <select value={profileForm.vendor}
                      onChange={e => setProfileForm({ ...profileForm, vendor: e.target.value })}
                      className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                    >
                      <option value="">通用 (所有厂商)</option>
                      <option value="huawei">华为 Huawei</option>
                      <option value="cisco">思科 Cisco</option>
                      <option value="h3c">华三 H3C</option>
                      <option value="ruijie">锐捷 Ruijie</option>
                      <option value="zte">中兴 ZTE</option>
                      <option value="juniper">Juniper</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-xs text-text-secondary mb-1.5">采集间隔 (秒)</label>
                    <input type="number" min="30"
                      value={profileForm.poll_interval_seconds}
                      onChange={e => setProfileForm({ ...profileForm, poll_interval_seconds: parseInt(e.target.value) || 300 })}
                      className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                    />
                  </div>
                </div>
                <div>
                  <label className="block text-xs text-text-secondary mb-1.5">描述</label>
                  <input type="text" placeholder="可选"
                    value={profileForm.description}
                    onChange={e => setProfileForm({ ...profileForm, description: e.target.value })}
                    className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                  />
                </div>

                {/* OID 条目动态列表 */}
                <div>
                  <div className="flex items-center justify-between mb-2">
                    <label className="text-xs text-text-secondary">OID 条目</label>
                    <button onClick={() => setProfileForm({ ...profileForm, oids: [...profileForm.oids, { oid: '', metric_name: '', description: '' }] })}
                      className="text-xs text-primary hover:text-primary/80 flex items-center gap-1"
                    >
                      <Plus className="w-3 h-3" /> 添加 OID
                    </button>
                  </div>
                  <div className="space-y-2">
                    {profileForm.oids.map((entry, idx) => (
                      <div key={idx} className="grid grid-cols-12 gap-2 items-center">
                        <input type="text" placeholder="OID (如 1.3.6.1.4.1.2011.5.25.31.1.1.1.1.45)"
                          value={entry.oid}
                          onChange={e => {
                            const oids = [...profileForm.oids];
                            oids[idx] = { ...oids[idx], oid: e.target.value };
                            setProfileForm({ ...profileForm, oids });
                          }}
                          className="col-span-5 px-3 py-1.5 bg-background border border-border rounded-lg text-xs text-text-primary font-mono focus:outline-none focus:border-primary"
                        />
                        <input type="text" placeholder="指标名称 (如 cpu_usage)"
                          value={entry.metric_name}
                          onChange={e => {
                            const oids = [...profileForm.oids];
                            oids[idx] = { ...oids[idx], metric_name: e.target.value };
                            setProfileForm({ ...profileForm, oids });
                          }}
                          className="col-span-3 px-3 py-1.5 bg-background border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-primary"
                        />
                        <input type="text" placeholder="描述 (可选)"
                          value={entry.description || ''}
                          onChange={e => {
                            const oids = [...profileForm.oids];
                            oids[idx] = { ...oids[idx], description: e.target.value };
                            setProfileForm({ ...profileForm, oids });
                          }}
                          className="col-span-3 px-3 py-1.5 bg-background border border-border rounded-lg text-xs text-text-primary focus:outline-none focus:border-primary"
                        />
                        <button onClick={() => setProfileForm({ ...profileForm, oids: profileForm.oids.filter((_, i) => i !== idx) })}
                          className="col-span-1 p-1.5 text-text-tertiary hover:text-status-failed transition-colors flex items-center justify-center"
                          disabled={profileForm.oids.length === 1}
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </div>
                    ))}
                  </div>
                </div>

                {/* 启用开关 */}
                <label className="flex items-center gap-2 cursor-pointer">
                  <input type="checkbox" checked={profileForm.enabled}
                    onChange={e => setProfileForm({ ...profileForm, enabled: e.target.checked })}
                    className="w-4 h-4 accent-emerald-500"
                  />
                  <span className="text-sm text-text-secondary">启用此采集配置</span>
                </label>

                {/* 操作按钮 */}
                <div className="flex items-center justify-end gap-3 pt-2">
                  <button onClick={() => resetProfileForm()}
                    className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary transition-colors"
                  >取消</button>
                  <button onClick={() => saveProfile.mutate()} disabled={!profileForm.name || saveProfile.isPending}
                    className="px-4 py-1.5 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all disabled:opacity-50 text-sm flex items-center gap-1.5"
                  >
                    {saveProfile.isPending && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
                    {editingProfileId ? '更新配置' : '保存配置'}
                  </button>
                </div>
              </div>
            )}

            {/* 配置列表 */}
            {profilesLoading ? (
              <div className="flex items-center justify-center py-12">
                <Loader2 className="w-6 h-6 animate-spin text-text-tertiary" />
              </div>
            ) : customOidProfiles.length === 0 ? (
              <div className="flex flex-col items-center justify-center py-16 text-text-tertiary">
                <Settings className="w-12 h-12 mb-3 opacity-30" />
                <p className="text-sm">暂无自定义 OID 采集配置</p>
                <p className="text-xs mt-1">点击"新建采集配置"添加自定义 SNMP OID 监控</p>
              </div>
            ) : (
              <div className="space-y-2">
                {customOidProfiles.map((profile: CustomOidProfile) => {
                  const oids = parseOids(profile.oids_json);
                  return (
                    <div key={profile.id} className="bg-surface rounded-xl border border-border p-4">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                            <Database className="w-5 h-5 text-emerald-400" />
                          </div>
                          <div>
                            <div className="flex items-center gap-2">
                              <p className="font-medium text-text-primary">{profile.name}</p>
                              <span className={clsx(
                                'text-xs px-1.5 py-0.5 rounded-full',
                                profile.enabled ? 'bg-status-success/10 text-status-success' : 'bg-gray-500/10 text-text-tertiary'
                              )}>
                                {profile.enabled ? '启用' : '禁用'}
                              </span>
                              {profile.vendor && (
                                <span className="text-xs px-1.5 py-0.5 rounded-full bg-blue-500/10 text-blue-400">
                                  {profile.vendor}
                                </span>
                              )}
                            </div>
                            <div className="flex items-center gap-3 mt-1 text-xs text-text-secondary">
                              <span>{oids.length} 个 OID</span>
                              <span>间隔 {profile.poll_interval_seconds}s</span>
                              {profile.description && <span className="truncate max-w-[200px]">{profile.description}</span>}
                            </div>
                          </div>
                        </div>
                        <div className="flex items-center gap-2">
                          <button onClick={() => {
                            setEditingProfileId(profile.id);
                            setProfileForm({
                              name: profile.name,
                              vendor: profile.vendor || '',
                              description: profile.description || '',
                              poll_interval_seconds: profile.poll_interval_seconds,
                              enabled: !!profile.enabled,
                              oids: oids.length > 0 ? oids : [{ oid: '', metric_name: '', description: '' }],
                            });
                            setShowProfileForm(true);
                          }}
                            className="p-2 text-text-tertiary hover:text-emerald-400 transition-colors" title="编辑"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                            </svg>
                          </button>
                          <button onClick={() => deleteProfile.mutate(profile.id)}
                            className="p-2 text-text-tertiary hover:text-status-failed transition-colors" title="删除"
                          >
                            <Trash2 className="w-4 h-4" />
                          </button>
                        </div>
                      </div>

                      {/* OID 详情展开 */}
                      <div className="mt-3 pt-3 border-t border-border">
                        <div className="space-y-1">
                          {oids.map((o, i) => (
                            <div key={i} className="flex items-center gap-3 text-xs">
                              <code className="font-mono text-text-tertiary">{o.oid}</code>
                              <span className="text-emerald-400 font-medium">→ {o.metric_name}</span>
                              {o.description && <span className="text-text-secondary">{o.description}</span>}
                            </div>
                          ))}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}

            {/* 采集结果查看器 */}
            <div className="bg-surface rounded-xl border border-border p-5 space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="font-medium text-text-primary flex items-center gap-2">
                  <Activity className="w-4 h-4 text-emerald-400" />
                  采集结果查看
                </h3>
                <select value={resultDeviceId}
                  onChange={e => setResultDeviceId(e.target.value)}
                  className="px-3 py-1.5 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
                >
                  <option value="">选择设备...</option>
                  {networkDevices.map((dev: NetworkDevice) => (
                    <option key={dev.id} value={dev.id}>{dev.name} ({dev.ip_address})</option>
                  ))}
                </select>
              </div>

              {resultDeviceId && customOidProfiles.length > 0 && (
                <div className="flex flex-wrap gap-2">
                  {customOidProfiles.map((profile: CustomOidProfile) => {
                    const key = `${profile.id}-${resultDeviceId}`;
                    const status = collectStatus[key];
                    return (
                      <button key={profile.id}
                        onClick={() => triggerCollect(profile.id, resultDeviceId)}
                        disabled={collectingProfileId === profile.id}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-xs bg-emerald-500/10 border border-emerald-500/30 text-emerald-400 rounded-lg hover:bg-emerald-500/20 transition-colors disabled:opacity-50"
                      >
                        {status?.status === 'loading' && <Loader2 className="w-3 h-3 animate-spin" />}
                        {status?.status === 'success' && <CheckCircle2 className="w-3 h-3" />}
                        {status?.status === 'fail' && <AlertCircle className="w-3 h-3" />}
                        {!status && <Play className="w-3 h-3" />}
                        采集: {profile.name}
                      </button>
                    );
                  })}
                </div>
              )}

              {resultDeviceId && (
                resultsLoading ? (
                  <div className="flex items-center justify-center py-8">
                    <Loader2 className="w-6 h-6 animate-spin text-text-tertiary" />
                  </div>
                ) : customOidResults.length === 0 ? (
                  <div className="flex flex-col items-center justify-center py-8 text-text-tertiary">
                    <Database className="w-10 h-10 mb-2 opacity-30" />
                    <p className="text-sm">暂无采集结果</p>
                    <p className="text-xs mt-1">点击上方按钮手动触发采集</p>
                  </div>
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="text-left text-text-secondary border-b border-border">
                          <th className="pb-2 pr-4">指标名称</th>
                          <th className="pb-2 pr-4">OID</th>
                          <th className="pb-2 pr-4">采集值</th>
                          <th className="pb-2 pr-4">采集时间</th>
                        </tr>
                      </thead>
                      <tbody>
                        {(customOidResults as CustomOidResult[]).map((r, idx) => (
                          <tr key={idx} className="border-b border-border/50 hover:bg-background/50">
                            <td className="py-2 pr-4 text-emerald-400 font-medium">{r.metric_name}</td>
                            <td className="py-2 pr-4 font-mono text-xs text-text-tertiary">{r.oid}</td>
                            <td className="py-2 pr-4 text-text-primary">{r.metric_value}</td>
                            <td className="py-2 pr-4 text-text-secondary text-xs">{new Date(r.sampled_at).toLocaleString()}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )
              )}
            </div>
          </div>
        )}

        {/* API 速查 */}
        <details className="bg-background rounded-xl border border-border p-4">
          <summary className="flex items-center gap-2 cursor-pointer text-sm text-text-secondary hover:text-text-primary">
            <Terminal className="w-4 h-4" />
            SNMP API 端点速查
          </summary>
          <div className="mt-3 grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2 text-xs">
            {[
              'GET /api/snmp/credentials',
              'POST /api/snmp/credentials',
              'DELETE /api/snmp/credentials/:id',
              'POST /api/snmp/test',
              'POST /api/snmp/system-info',
              'POST /api/snmp/interfaces',
              'POST /api/snmp/walk',
              'POST /api/snmp/get',
              'POST /api/snmp/discover',
              'GET /api/snmp/health/:deviceId',
              'POST /api/snmp/health-batch',
              'GET /api/snmp/traps',
              'POST /api/snmp/trap/start',
              'POST /api/snmp/trap/stop',
              'POST /api/snmp/poll-interfaces',
              'GET /api/snmp/device/:deviceId/system-info',
              'GET /api/snmp/device/:deviceId/interfaces',
              'GET /api/snmp/custom-oids',
              'POST /api/snmp/custom-oids',
              'PUT /api/snmp/custom-oids/:id',
              'DELETE /api/snmp/custom-oids/:id',
              'GET /api/snmp/custom-oids/results/:deviceId',
              'POST /api/snmp/custom-oids/:id/collect/:deviceId',
            ].map(endpoint => (
              <code key={endpoint} className="block px-2 py-1.5 bg-surface rounded font-mono text-text-secondary">
                {endpoint}
              </code>
            ))}
          </div>
        </details>
      </div>
    </div>
  );
}
