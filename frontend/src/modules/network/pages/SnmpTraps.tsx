import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Radio, Play, Square, RefreshCw, FlaskConical, ChevronDown, ChevronRight, AlertCircle } from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';

interface TrapEvent {
  id: string;
  source_ip: string;
  trap_type: string;
  enterprise_oid: string;
  agent_address: string;
  generic_type: number;
  specific_type: number;
  varbinds_json: string;
  created_at: string;
}

interface ListenerStatus {
  running: boolean;
  port: number;
  received: number;
  started_at?: string;
  last_error?: string;
}

interface Varbind {
  oid: string;
  value: unknown;
  type: number;
}

const trapTypeLabels: Record<string, string> = {
  coldStart: '冷启动',
  warmStart: '热启动',
  linkDown: '链路断开',
  linkUp: '链路恢复',
  authenticationFailure: '认证失败',
  egpNeighborLoss: 'EGP 邻居丢失',
  enterpriseSpecific: '厂商自定义',
};

export default function SnmpTraps() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [port, setPort] = useState<number>(162);
  const [portTouched, setPortTouched] = useState(false);
  const [sourceFilter, setSourceFilter] = useState('');
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const { data: status } = useQuery({
    queryKey: ['snmp-trap-status'],
    queryFn: async () => {
      const res = await api.get('/snmp/trap/status');
      return (res.data?.data ?? res.data) as ListenerStatus;
    },
    refetchInterval: 5000,
  });

  const { data: traps = [], isLoading, refetch } = useQuery({
    queryKey: ['snmp-traps', sourceFilter],
    queryFn: async () => {
      const res = await api.get('/snmp/traps', { params: { limit: 100, sourceIp: sourceFilter || undefined } });
      const d = res.data?.data ?? res.data;
      return (Array.isArray(d) ? d : []) as TrapEvent[];
    },
    refetchInterval: 10000,
  });

  const effectivePort = portTouched ? port : (status?.port || 162);

  const startMutation = useMutation({
    mutationFn: () => api.post('/snmp/trap/start', { port: effectivePort }),
    onSuccess: () => {
      toast.success(`Trap 监听已启动（UDP ${effectivePort}）`);
      queryClient.invalidateQueries({ queryKey: ['snmp-trap-status'] });
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '启动失败'),
  });

  const stopMutation = useMutation({
    mutationFn: () => api.post('/snmp/trap/stop'),
    onSuccess: () => {
      toast.success('Trap 监听已停止');
      queryClient.invalidateQueries({ queryKey: ['snmp-trap-status'] });
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '停止失败'),
  });

  const testMutation = useMutation({
    mutationFn: () => api.post('/snmp/traps/test'),
    onSuccess: () => {
      toast.success('测试 Trap 已写入');
      queryClient.invalidateQueries({ queryKey: ['snmp-traps'] });
    },
    onError: () => toast.error('生成测试 Trap 失败'),
  });

  const running = status?.running ?? false;

  const parseVarbinds = (s: string): Varbind[] => {
    try {
      const v = JSON.parse(s);
      return Array.isArray(v) ? v : [];
    } catch {
      return [];
    }
  };

  return (
    <div className="p-6 space-y-6">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 bg-surface border border-border rounded-xl p-5">
        <div className="flex items-start gap-3.5">
          <div className="p-3 bg-primary/10 border border-primary/30 rounded-xl text-primary">
            <Radio className="w-6 h-6" />
          </div>
          <div>
            <h1 className="text-lg font-bold text-text-primary flex items-center gap-2">
              SNMP Trap 接收
              <span className={`text-xs font-normal px-2 py-0.5 rounded-full border ${
                running ? 'bg-green-500/15 text-green-500 border-green-500/30' : 'bg-background text-text-secondary border-border'
              }`}>
                {running ? `监听中 · UDP ${status?.port}` : '未运行'}
              </span>
            </h1>
            <p className="text-xs text-text-secondary mt-1 max-w-2xl">
              接收网络设备主动上报的 SNMP Trap / Inform（v1/v2c），实时落库。设备侧需将 Trap 目标指向本机 UDP 端口（默认 162，特权端口需相应权限）。
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <input
            type="number"
            min={1}
            max={65535}
            value={effectivePort}
            disabled={running}
            onChange={(e) => { setPortTouched(true); setPort(Number(e.target.value)); }}
            className="w-24 bg-background border border-border rounded-lg p-2 text-text-primary text-sm disabled:opacity-60"
            title="监听端口"
          />
          {running ? (
            <button
              onClick={() => stopMutation.mutate()}
              disabled={stopMutation.isPending}
              className="flex items-center gap-2 px-4 py-2 bg-rose-600 hover:bg-rose-500 disabled:opacity-50 text-white text-sm font-medium rounded-lg"
            >
              <Square className="w-4 h-4" />
              停止监听
            </button>
          ) : (
            <button
              onClick={() => startMutation.mutate()}
              disabled={startMutation.isPending}
              className="flex items-center gap-2 px-4 py-2 bg-primary hover:bg-primary/90 disabled:opacity-50 text-white text-sm font-medium rounded-lg"
            >
              <Play className="w-4 h-4" />
              {startMutation.isPending ? '启动中...' : '启动监听'}
            </button>
          )}
        </div>
      </div>

      {status?.last_error && !running && (
        <div className="flex items-center gap-2 bg-rose-500/10 border border-rose-500/30 rounded-lg p-3 text-sm text-rose-400">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          上次监听异常：{status.last_error}
        </div>
      )}

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="bg-surface border border-border rounded-lg p-4">
          <p className="text-xs text-text-secondary mb-1">本次运行已接收</p>
          <p className="text-2xl font-bold text-text-primary">{status?.received ?? 0}</p>
        </div>
        <div className="bg-surface border border-border rounded-lg p-4">
          <p className="text-xs text-text-secondary mb-1">启动时间</p>
          <p className="text-sm font-medium text-text-primary mt-2">{status?.started_at || '—'}</p>
        </div>
        <div className="bg-surface border border-border rounded-lg p-4">
          <p className="text-xs text-text-secondary mb-1">监听端口</p>
          <p className="text-2xl font-bold text-text-primary">{status?.port || 162}</p>
        </div>
        <div className="bg-surface border border-border rounded-lg p-4">
          <p className="text-xs text-text-secondary mb-1">列表记录数</p>
          <p className="text-2xl font-bold text-text-primary">{traps.length}</p>
        </div>
      </div>

      <div className="bg-surface border border-border rounded-lg">
        <div className="flex items-center justify-between p-4 border-b border-border">
          <h2 className="text-sm font-semibold text-text-primary">Trap 事件</h2>
          <div className="flex items-center gap-2">
            <input
              type="text"
              value={sourceFilter}
              onChange={(e) => setSourceFilter(e.target.value)}
              placeholder="按来源 IP 过滤"
              className="w-40 bg-background border border-border rounded-lg p-1.5 text-sm text-text-primary"
            />
            <button
              onClick={() => testMutation.mutate()}
              disabled={testMutation.isPending}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-background hover:bg-surface border border-border text-text-primary rounded-lg"
              title="写入一条模拟 Trap，用于验证列表链路"
            >
              <FlaskConical className="w-4 h-4" />
              测试 Trap
            </button>
            <button
              onClick={() => refetch()}
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-background hover:bg-surface border border-border text-text-primary rounded-lg"
            >
              <RefreshCw className="w-4 h-4" />
              刷新
            </button>
          </div>
        </div>
        {isLoading ? (
          <div className="p-12 text-center text-text-secondary">加载中...</div>
        ) : traps.length === 0 ? (
          <div className="p-12 text-center text-text-secondary">
            <Radio className="w-10 h-10 mx-auto mb-3 opacity-50" />
            暂无 Trap 记录{running ? '，等待设备上报...' : '，启动监听后设备上报的 Trap 会出现在这里'}
          </div>
        ) : (
          <div className="divide-y divide-border">
            {traps.map((t) => {
              const expanded = expandedId === t.id;
              const varbinds = expanded ? parseVarbinds(t.varbinds_json) : [];
              return (
                <div key={t.id}>
                  <button
                    onClick={() => setExpandedId(expanded ? null : t.id)}
                    className="w-full flex items-center gap-3 p-3 hover:bg-background/60 text-left"
                  >
                    {expanded ? <ChevronDown className="w-4 h-4 text-text-secondary flex-shrink-0" /> : <ChevronRight className="w-4 h-4 text-text-secondary flex-shrink-0" />}
                    <span className="text-sm text-text-secondary w-36 flex-shrink-0">{t.created_at}</span>
                    <span className="text-sm font-mono text-text-primary w-32 flex-shrink-0">{t.source_ip}</span>
                    <span className={`text-xs px-2 py-0.5 rounded flex-shrink-0 ${
                      t.trap_type === 'linkDown' || t.trap_type === 'authenticationFailure'
                        ? 'bg-rose-500/15 text-rose-400'
                        : t.trap_type === 'linkUp' || t.trap_type === 'coldStart' || t.trap_type === 'warmStart'
                          ? 'bg-green-500/15 text-green-500'
                          : 'bg-background text-text-secondary'
                    }`}>
                      {trapTypeLabels[t.trap_type] || t.trap_type}
                    </span>
                    <span className="text-xs font-mono text-text-secondary truncate">{t.enterprise_oid}</span>
                  </button>
                  {expanded && (
                    <div className="px-10 pb-3">
                      <table className="w-full text-xs">
                        <thead>
                          <tr className="text-text-secondary text-left">
                            <th className="py-1 pr-4 font-medium">OID</th>
                            <th className="py-1 pr-4 font-medium">值</th>
                            <th className="py-1 font-medium">类型</th>
                          </tr>
                        </thead>
                        <tbody>
                          {varbinds.map((v, i) => (
                            <tr key={i} className="border-t border-border/50">
                              <td className="py-1 pr-4 font-mono text-text-primary">{v.oid}</td>
                              <td className="py-1 pr-4 font-mono text-text-primary break-all">{String(v.value)}</td>
                              <td className="py-1 text-text-secondary">{v.type}</td>
                            </tr>
                          ))}
                          {varbinds.length === 0 && (
                            <tr><td colSpan={3} className="py-2 text-text-secondary">无 Varbind 数据</td></tr>
                          )}
                        </tbody>
                      </table>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}