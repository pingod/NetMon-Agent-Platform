/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Radio, Search, Loader2, CheckCircle2, AlertCircle, XCircle,
  Trash2, Play, Square, Wifi, Monitor,
  Globe, Server, Clock, HardDrive, Download
} from 'lucide-react';
import clsx from 'clsx';
import api from '../../../lib/api';
import { getAxiosErrorMessage } from '../../../lib/errorHandler';
import { useToast } from '../../../contexts/ToastContext';

interface DiscoveryJob {
  id: string;
  name: string;
  start_ip: string;
  end_ip: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  total_hosts: number;
  scanned_hosts: number;
  found_devices: number;
  credential_ids: string;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

interface DiscoveryResult {
  id: string;
  job_id: string;
  ip_address: string;
  status: 'offline' | 'online' | 'snmp_ok' | 'snmp_fail';
  sys_name?: string;
  sys_descr?: string;
  vendor?: string;
  model?: string;
  snmp_version?: string;
  interface_count?: number;
  response_time_ms?: number;
  created_at: string;
}

export default function NetworkDiscovery() {
  const queryClient = useQueryClient();
  const toast = useToast();

  // 扫描表单
  const [scanName, setScanName] = useState('');
  const [startIp, setStartIp] = useState('');
  const [endIp, setEndIp] = useState('');

  // 删除确认弹窗
  const [deleteTarget, setDeleteTarget] = useState<{ id: string; name: string } | null>(null);

  // 导入对话框
  const [showImportModal, setShowImportModal] = useState(false);
  const [selectedResults, setSelectedResults] = useState<Set<string>>(new Set());
  const [sshUsername, setSshUsername] = useState('admin');
  const [sshPassword, setSshPassword] = useState('');
  const [sshPort, setSshPort] = useState(22);
  const [importResult, setImportResult] = useState<{ imported: number; errors: string[] } | null>(null);

  // 查询
  const { data: jobs = [], isLoading: jobsLoading } = useQuery({
    queryKey: ['network-discovery-jobs'],
    queryFn: () => api.get('/network-discovery/jobs').then(r => r.data.data || []),
    refetchInterval: 3000,
  });

  // 选中的任务的结果
  const [selectedJobId, setSelectedJobId] = useState<string | null>(null);
  const jobsList: DiscoveryJob[] = Array.isArray(jobs) ? jobs : [];
  const selectedJob = jobsList.find((j: DiscoveryJob) => j.id === selectedJobId) as DiscoveryJob | undefined;
  const { data: resultsData, isLoading: resultsLoading } = useQuery({
    queryKey: ['network-discovery-results', selectedJobId],
    queryFn: async () => {
      const res = await api.get(`/network-discovery/results`, {
        params: { jobId: selectedJobId || undefined, limit: 500 }
      });
      const payload = res.data?.data;
      if (Array.isArray(payload)) {
        return { data: payload as DiscoveryResult[], total: res.data?.total ?? payload.length };
      }
      if (payload && Array.isArray(payload.results)) {
        return { data: payload.results as DiscoveryResult[], total: payload.total ?? payload.results.length };
      }
      return { data: [] as DiscoveryResult[], total: 0 };
    },
    enabled: !!selectedJobId,
    refetchInterval: selectedJob?.status === 'running' ? 3000 : undefined,
  });

  const results: DiscoveryResult[] = Array.isArray(resultsData?.data) ? resultsData.data : [];
  const resultTotal = resultsData?.total ?? results.length;
  const onlineResults = results.filter(r => r && r.status !== 'offline');
  const offlineResults = results.filter(r => r && r.status === 'offline');
  const snmpOkResults = results.filter(r => r && r.status === 'snmp_ok');

  // 创建扫描（纯 Ping 探测，不传 credential_ids）
  const createJob = useMutation({
    mutationFn: () => api.post('/network-discovery/jobs', {
      name: scanName || `${startIp}-${endIp}`,
      start_ip: startIp,
      end_ip: endIp,
      credential_ids: [],
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['network-discovery-jobs'] });
      toast.success('Ping 扫描任务已创建');
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '创建失败'));
    },
  });

  // 取消扫描
  const cancelJob = useMutation({
    mutationFn: (jobId: string) => api.post(`/network-discovery/jobs/${jobId}/cancel`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['network-discovery-jobs'] });
      toast.success('扫描任务已取消');
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '取消失败'));
    },
  });

  // 删除扫描
  const deleteJob = useMutation({
    mutationFn: (jobId: string) => api.delete(`/network-discovery/jobs/${jobId}`),
    onSuccess: (_data: unknown, variables: string) => {
      queryClient.invalidateQueries({ queryKey: ['network-discovery-jobs'] });
      if (selectedJobId === variables) setSelectedJobId(null);
      setDeleteTarget(null);
      toast.success('扫描任务已删除');
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '删除失败'));
    },
  });

  // 导入设备
  const importDevices = useMutation({
    mutationFn: () => api.post('/network-discovery/import', {
      result_ids: Array.from(selectedResults),
      ssh_username: sshUsername,
      ssh_password: sshPassword || undefined,
      ssh_port: sshPort,
    }),
    onSuccess: (res) => {
      setImportResult(res.data.data);
      queryClient.invalidateQueries({ queryKey: ['network-devices'] });
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '导入失败'));
    },
  });

  return (
    <div className="h-full overflow-auto p-6">
      <div className="space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-text-primary mb-2 flex items-center gap-3">
              <Radio className="w-7 h-7 text-emerald-600" />
              Ping 网络探测
            </h1>
            <p className="text-text-secondary">批量 Ping IP 范围，快速探测主机可达性</p>
          </div>
        </div>

        {/* 扫描表单 */}
        <div className="bg-surface rounded-xl border border-border p-5 space-y-4">
          <h3 className="font-medium text-text-primary">新建扫描任务</h3>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div>
              <label className="block text-xs text-text-secondary mb-1.5">任务名称</label>
              <input type="text" placeholder="例如: 办公室扫描"
                value={scanName}
                onChange={e => setScanName(e.target.value)}
                className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary"
              />
            </div>
            <div>
              <label className="block text-xs text-text-secondary mb-1.5">起始 IP *</label>
              <input type="text" placeholder="192.168.1.1"
                value={startIp}
                onChange={e => setStartIp(e.target.value)}
                className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary font-mono"
              />
            </div>
            <div>
              <label className="block text-xs text-text-secondary mb-1.5">结束 IP *</label>
              <input type="text" placeholder="192.168.1.254"
                value={endIp}
                onChange={e => setEndIp(e.target.value)}
                className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-primary font-mono"
              />
            </div>
            <div className="flex items-end">
              <button
                onClick={() => createJob.mutate()}
                disabled={!startIp || !endIp || createJob.isPending}
                className="w-full px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all disabled:opacity-50 flex items-center justify-center gap-2"
              >
                {createJob.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <Play className="w-4 h-4" />}
                开始扫描
              </button>
            </div>
          </div>
        </div>

        {/* 任务列表 */}
        <div className="space-y-2">
          <h3 className="font-medium text-text-primary">扫描历史</h3>
          {jobsLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="w-6 h-6 animate-spin text-text-tertiary" />
            </div>
          ) : jobsList.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-text-tertiary bg-surface rounded-xl border border-border">
              <Globe className="w-10 h-10 mb-2 opacity-30" />
              <p className="text-sm">暂无扫描记录</p>
            </div>
          ) : (
            jobsList.map((job: DiscoveryJob) => (
              <div key={job.id}
                onClick={() => { setSelectedJobId(job.id); }}
                className={clsx(
                  'bg-surface rounded-xl border p-4 cursor-pointer transition-all',
                  selectedJobId === job.id ? 'border-primary' : 'border-border hover:border-emerald-400/30'
                )}
              >
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className={clsx(
                      'w-10 h-10 rounded-lg flex items-center justify-center',
                      job.status === 'completed' ? 'bg-emerald-500/10' :
                      job.status === 'running' ? 'bg-blue-500/10' :
                      job.status === 'failed' ? 'bg-red-500/10' :
                      'bg-slate-500/10'
                    )}>
                      {job.status === 'running' ? <Loader2 className="w-5 h-5 animate-spin text-blue-600" /> :
                       job.status === 'completed' ? <CheckCircle2 className="w-5 h-5 text-emerald-600" /> :
                       job.status === 'cancelled' ? <XCircle className="w-5 h-5 text-yellow-600" /> :
                       job.status === 'failed' ? <AlertCircle className="w-5 h-5 text-red-600" /> :
                       <Clock className="w-5 h-5 text-text-secondary" />}
                    </div>
                    <div>
                      <p className="font-medium text-text-primary">{job.name}</p>
                      <div className="flex items-center gap-3 mt-1 text-xs text-text-secondary">
                        <span>{job.start_ip} → {job.end_ip}</span>
                        <span>{job.total_hosts} 个主机</span>
                        <span>在线 {job.found_devices} 个</span>
                        {job.status === 'running' && (
                          <span className="text-blue-600 font-medium">扫描中 {job.progress}%</span>
                        )}
                        <span>{job.created_at ? new Date(job.created_at).toLocaleString() : ''}</span>
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    {job.status === 'running' && (
                      <button onClick={(e) => { e.stopPropagation(); cancelJob.mutate(job.id); }}
                        className="p-2 text-yellow-600 hover:text-yellow-700 transition-colors" title="取消">
                        <Square className="w-4 h-4" />
                      </button>
                    )}
                    <button onClick={(e) => { e.stopPropagation(); setDeleteTarget({ id: job.id, name: job.name }); }}
                      className="p-2 text-text-tertiary hover:text-status-failed transition-colors" title="删除">
                      <Trash2 className="w-4 h-4" />
                    </button>
                  </div>
                </div>
                {job.status === 'running' && (
                  <div className="mt-3 h-1.5 bg-background rounded-full overflow-hidden">
                    <div className="h-full bg-gradient-to-r from-blue-500 to-emerald-500 rounded-full transition-all duration-500"
                      style={{ width: `${job.progress || 0}%` }} />
                  </div>
                )}
              </div>
            ))
          )}
        </div>

        {/* 扫描结果 */}
        <div className="bg-surface rounded-xl border border-border p-5">
          <div className="flex items-center justify-between mb-4">
            <div className="flex items-center gap-2">
              <Monitor className="w-5 h-5 text-emerald-600" />
              <h3 className="font-medium text-text-primary">
                探测结果 {selectedJob ? `(${selectedJob.name})` : ''}
              </h3>
              {selectedJob && (
                <span className="text-xs text-text-secondary ml-2">
                  共 {resultTotal} 条
                </span>
              )}
            </div>
            {selectedJob && (
              <div className="flex items-center gap-3">
                <span className="text-xs px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-600">
                  在线 {onlineResults.length}
                </span>
                <span className="text-xs px-2 py-0.5 rounded-full bg-slate-500/10 text-text-tertiary">
                  离线 {offlineResults.length}
                </span>
              </div>
            )}
          </div>

          {!selectedJobId ? (
            <div className="flex flex-col items-center justify-center py-12 text-text-tertiary">
              <Search className="w-10 h-10 mb-2 opacity-30" />
              <p className="text-sm">选择一个扫描任务查看结果</p>
            </div>
          ) : resultsLoading ? (
            <div className="flex items-center justify-center py-12">
              <Loader2 className="w-8 h-8 animate-spin text-text-tertiary" />
            </div>
          ) : results.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-text-tertiary">
              <Wifi className="w-10 h-10 mb-2 opacity-30" />
              <p className="text-sm">暂无扫描结果</p>
            </div>
          ) : (
            <div className="space-y-1.5 max-h-[500px] overflow-y-auto">
              {[...results]
                .sort((a, b) => {
                  const order: Record<string, number> = { online: 0, snmp_ok: 1, snmp_fail: 2, offline: 3 };
                  return (order[a?.status] ?? 99) - (order[b?.status] ?? 99);
                })
                .map((result: DiscoveryResult) => (
                <div key={result.id}
                  className={clsx(
                    'flex items-center justify-between p-3 rounded-lg border transition-all',
                    result.status === 'online' || result.status === 'snmp_ok'
                      ? 'bg-emerald-500/5 border-emerald-500/20'
                      : 'bg-background border-border/50'
                  )}
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <div className={clsx(
                      'w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0',
                      result.status === 'online' || result.status === 'snmp_ok'
                        ? 'bg-emerald-500/10' : 'bg-slate-500/10'
                    )}>
                      {result.status === 'online' || result.status === 'snmp_ok'
                        ? <Server className="w-4 h-4 text-emerald-600" />
                        : <HardDrive className="w-4 h-4 text-text-tertiary" />}
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="font-mono text-sm text-text-primary">{result.ip_address}</span>
                        {result.sys_name && (
                          <span className="text-sm text-text-secondary truncate">{result.sys_name}</span>
                        )}
                      </div>
                      <div className="flex items-center gap-2 mt-0.5 text-xs text-text-tertiary">
                        {result.vendor && <span>{result.vendor}</span>}
                        {result.model && <span className="truncate max-w-[200px]">{result.model}</span>}
                        {result.response_time_ms !== null && result.response_time_ms !== undefined && (
                          <span>{result.response_time_ms}ms</span>
                        )}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2 flex-shrink-0">
                    <span className={clsx(
                      'text-xs px-2 py-0.5 rounded-full',
                      result.status === 'online' || result.status === 'snmp_ok'
                        ? 'bg-emerald-500/10 text-emerald-600' : 'bg-slate-500/10 text-text-tertiary'
                    )}>
                      {result.status === 'online' || result.status === 'snmp_ok' ? '在线' : '离线'}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* 导入在线设备（仅对有 SNMP 信息的结果） */}
          {selectedJobId && snmpOkResults.length > 0 && (
            <div className="mt-4 pt-4 border-t border-border">
              <button onClick={() => { setShowImportModal(true); setSelectedResults(new Set(snmpOkResults.map(r => r.id))); }}
                className="px-4 py-2 bg-emerald-500/10 border border-emerald-500/30 text-emerald-600 rounded-lg hover:bg-emerald-500/20 transition-all text-sm flex items-center gap-2"
              >
                <Download className="w-4 h-4" />
                导入 SNMP 可达设备（{snmpOkResults.length} 个）
              </button>
            </div>
          )}
        </div>
      </div>

      {/* 删除确认弹窗 */}
      {deleteTarget && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-surface rounded-xl border border-border p-6 w-full max-w-sm shadow-2xl">
            <h3 className="font-medium text-text-primary mb-2">确认删除</h3>
            <p className="text-sm text-text-secondary mb-5">
              确定要删除扫描任务「{deleteTarget.name}」及其所有结果吗？此操作不可撤销。
            </p>
            <div className="flex items-center justify-end gap-2">
              <button onClick={() => setDeleteTarget(null)}
                className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary transition-colors"
              >取消</button>
              <button onClick={() => deleteJob.mutate(deleteTarget.id)}
                disabled={deleteJob.isPending}
                className="px-4 py-2 bg-red-500 text-white rounded-lg hover:bg-red-600 transition-all disabled:opacity-50 text-sm flex items-center gap-2"
              >
                {deleteJob.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                删除
              </button>
            </div>
          </div>
        </div>
      )}

      {/* 导入对话框 */}
      {showImportModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-surface rounded-xl border border-border p-6 w-full max-w-lg shadow-2xl">
            <h3 className="font-medium text-text-primary mb-4">导入发现的设备</h3>

            <div className="space-y-3 mb-4">
              <div className="text-sm text-text-secondary">
                将选中 <strong className="text-text-primary">{selectedResults.size}</strong> 个设备导入到网络设备库
              </div>
              <div className="grid grid-cols-3 gap-3">
                <div>
                  <label className="block text-xs text-text-secondary mb-1">SSH 用户名</label>
                  <input type="text" value={sshUsername}
                    onChange={e => setSshUsername(e.target.value)}
                    className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary"
                  />
                </div>
                <div>
                  <label className="block text-xs text-text-secondary mb-1">SSH 密码（可选）</label>
                  <input type="password" value={sshPassword}
                    onChange={e => setSshPassword(e.target.value)}
                    className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary"
                  />
                </div>
                <div>
                  <label className="block text-xs text-text-secondary mb-1">SSH 端口</label>
                  <input type="number" value={sshPort}
                    onChange={e => setSshPort(parseInt(e.target.value) || 22)}
                    className="w-full px-3 py-2 bg-background border border-border rounded-lg text-sm text-text-primary"
                  />
                </div>
              </div>

              {importResult && (
                <div className={clsx(
                  'p-3 rounded-lg text-sm',
                  importResult.imported > 0 ? 'bg-emerald-500/10 text-emerald-600' : 'bg-yellow-500/10 text-yellow-600'
                )}>
                  {importResult.imported > 0
                    ? `成功导入 ${importResult.imported} 个设备`
                    : '没有新设备被导入（可能已有相同 IP 的设备）'}
                  {importResult.errors.length > 0 && importResult.errors.length <= 5 && (
                    <div className="mt-1 text-xs text-text-secondary">
                      {importResult.errors.map((e, i) => <div key={i}>- {e}</div>)}
                    </div>
                  )}
                  {importResult.errors.length > 5 && (
                    <div className="mt-1 text-xs text-text-secondary">
                      ...以及其他 {importResult.errors.length - 5} 个错误
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="flex items-center justify-end gap-2">
              <button onClick={() => { setShowImportModal(false); setImportResult(null); }}
                className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary"
              >关闭</button>
              {!importResult && (
                <button onClick={() => importDevices.mutate()}
                  disabled={selectedResults.size === 0 || importDevices.isPending}
                  className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all disabled:opacity-50 text-sm flex items-center gap-2"
                >
                  {importDevices.isPending && <Loader2 className="w-4 h-4 animate-spin" />}
                  确认导入
                </button>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
