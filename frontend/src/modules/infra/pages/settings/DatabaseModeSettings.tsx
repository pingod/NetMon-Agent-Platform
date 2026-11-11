import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Server,
  Database,
  CheckCircle2,
  AlertTriangle,
  RefreshCw,
  Copy,
  Download,
  ArrowRightLeft,
  Zap,
  HardDrive,
  ShieldCheck,
  Check,
  Loader2,
  FileCode,
  Layers,
  Table
} from 'lucide-react';
import api from '../../../../lib/api';
import { useToast } from '../../../../contexts/ToastContext';
import { getAxiosErrorMessage } from '@/lib/errorHandler';

interface DbConnection {
  id: string;
  name: string;
  db_type: string;
  host: string;
  port: number;
  username: string;
  database: string;
  enabled: number;
}

interface DatabaseModeStatus {
  current_engine: 'mysql' | 'sqlite';
  mysql_enabled: boolean;
  mysql_connected: boolean;
  mysql_dsn: string;
  sqlite_path: string;
  sqlite_table_count: number;
  sqlite_total_rows: number;
  mysql_table_count: number;
  mysql_total_rows: number;
}

interface MigrationReport {
  total_tables: number;
  total_rows: number;
  duration_ms: number;
  details: Array<{
    table_name: string;
    rows: number;
    status: 'success' | 'skipped' | 'error';
    error?: string;
  }>;
  migrated_at: string;
}

export default function DatabaseModeSettings() {
  const queryClient = useQueryClient();
  const toast = useToast();

  const [selectedConnId, setSelectedConnId] = useState<string>('');
  const [customDsn, setCustomDsn] = useState<string>('');
  const [useCustomDsn, setUseCustomDsn] = useState<boolean>(false);
  const [copied, setCopied] = useState<boolean>(false);
  const [migrationReport, setMigrationReport] = useState<MigrationReport | null>(null);

  // 1. 获取当前数据库模式与状态
  const { data: status, isLoading: isStatusLoading, refetch: refetchStatus } = useQuery({
    queryKey: ['database-mode-status'],
    queryFn: async () => {
      const res = await api.get('/database-mode');
      return res.data.data as DatabaseModeStatus;
    },
    refetchInterval: 10000,
  });

  // 2. 获取已配置的 MySQL 数据库连接列表
  const { data: dbConnections } = useQuery({
    queryKey: ['db-connections'],
    queryFn: async () => {
      const res = await api.get('/db-connections');
      const all = res.data.data as DbConnection[];
      return all.filter((c) => c.db_type === 'mysql' || c.db_type === '' || !c.db_type);
    },
  });

  // 自动选择第一个连接
  React.useEffect(() => {
    if (dbConnections && dbConnections.length > 0 && !selectedConnId && !useCustomDsn) {
      setSelectedConnId(dbConnections[0].id);
    }
  }, [dbConnections, selectedConnId, useCustomDsn]);

  // 3. 生成 .env 配置文件
  const configMutation = useMutation({
    mutationFn: async () => {
      const payload = useCustomDsn
        ? { dsn: customDsn }
        : { db_connection_id: selectedConnId };
      const res = await api.post('/database-mode/generate-config', payload);
      return res.data.data as { env_content: string; dsn: string };
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '生成配置文件失败'));
    },
  });

  // 4. 一键数据迁移 (SQLite -> MySQL)
  const migrateMutation = useMutation({
    mutationFn: async () => {
      const payload = useCustomDsn
        ? { dsn: customDsn }
        : { db_connection_id: selectedConnId };
      const res = await api.post('/database-mode/migrate-to-mysql', payload);
      return res.data.data as MigrationReport;
    },
    onSuccess: (data) => {
      setMigrationReport(data);
      toast.success(`迁移成功！共写入 ${data.total_tables} 张表，${data.total_rows} 条数据`);
      refetchStatus();
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '数据迁移失败'));
    },
  });

  // 5. 切换主数据库引擎模式
  const switchMutation = useMutation({
    mutationFn: async (mode: 'mysql' | 'sqlite') => {
      const payload = mode === 'mysql'
        ? (useCustomDsn ? { mode: 'mysql', dsn: customDsn, persist_env: true } : { mode: 'mysql', db_connection_id: selectedConnId, persist_env: true })
        : { mode: 'sqlite', persist_env: true };
      const res = await api.post('/database-mode/switch-mode', payload);
      return res.data;
    },
    onSuccess: (data) => {
      toast.success(data.message || '主数据源切换成功！');
      queryClient.invalidateQueries();
      refetchStatus();
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '数据源切换失败'));
    },
  });

  const isMySQL = status?.current_engine === 'mysql';

  const handleCopyEnv = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    toast.success('已复制 .env 生产配置到剪贴板');
    setTimeout(() => setCopied(false), 2000);
  };

  const handleDownloadEnv = (text: string) => {
    const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = 'production.env';
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
    toast.success('已下载 production.env 配置文件');
  };

  if (isStatusLoading) {
    return (
      <div className="flex items-center justify-center h-48">
        <Loader2 className="w-8 h-8 text-blue-600 animate-spin" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* 头部说明 */}
      <div>
        <h3 className="text-lg font-semibold text-text-primary mb-2 flex items-center gap-2">
          <Server className="w-5 h-5 text-blue-600" />
          MySQL 生产持久化与数据迁移
        </h3>
        <p className="text-sm text-text-secondary">
          生产环境下支持将所有常用配置（用户、网络设备、凭据、告警、拓扑、机房等）持久化至 MySQL 数据库，
          告别单机 SQLite 写入限制，支持集群部署与一键无损迁移。
        </p>
      </div>

      {/* 1. 当前数据库引擎状态仪表板 */}
      <div className={`p-6 rounded-2xl border transition-all ${
        isMySQL
          ? 'bg-emerald-950/20 border-emerald-500/30 shadow-lg shadow-emerald-900/10'
          : 'bg-surface border-border shadow-lg shadow-slate-950/30'
      }`}>
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div className="flex items-start gap-4">
            <div className={`w-12 h-12 rounded-xl flex items-center justify-center flex-shrink-0 ${
              isMySQL ? 'bg-emerald-500/20 text-emerald-600' : 'bg-blue-500/20 text-blue-600'
            }`}>
              {isMySQL ? <ShieldCheck className="w-6 h-6" /> : <HardDrive className="w-6 h-6" />}
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h4 className="text-lg font-bold text-text-primary">
                  当前数据存储引擎：{isMySQL ? 'MySQL 5.7 / 8.0 (企业生产模式)' : 'SQLite (本地单机模式)'}
                </h4>
                <span className={`px-2.5 py-0.5 text-xs font-semibold rounded-full border ${
                  isMySQL
                    ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/30'
                    : 'bg-blue-500/10 text-blue-600 border-blue-500/30'
                }`}>
                  {isMySQL ? '已持久化到 MySQL' : '本地单文件'}
                </span>
              </div>
              <p className="text-xs text-text-secondary mt-1">
                {isMySQL
                  ? `当前已接入生产 MySQL 数据库，所有配置与元数据直接读取与写入 MySQL (${status?.mysql_dsn || '已配置'})`
                  : `当前运行在 SQLite 轻量模式 (${status?.sqlite_path || './data/app.db'})，推荐在生产部署时一键迁移到 MySQL`}
              </p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <button
              onClick={() => refetchStatus()}
              className="px-3 py-2 bg-surface-2 hover:bg-surface-hover text-text-secondary rounded-xl text-sm font-medium border border-border transition-all flex items-center gap-1.5"
            >
              <RefreshCw className="w-4 h-4" />
              刷新状态
            </button>
          </div>
        </div>

        {/* 数据统计网格 */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mt-6 pt-6 border-t border-border">
          <div className="bg-surface p-4 rounded-xl border border-border">
            <p className="text-xs text-text-secondary">SQLite 数据表数</p>
            <p className="text-xl font-bold text-text-primary mt-1">{status?.sqlite_table_count || 0} 张</p>
            <p className="text-xs text-text-tertiary mt-0.5">记录数: {status?.sqlite_total_rows || 0} 条</p>
          </div>
          <div className="bg-surface p-4 rounded-xl border border-border">
            <p className="text-xs text-text-secondary">MySQL 生产数据表数</p>
            <p className="text-xl font-bold text-emerald-600 mt-1">{status?.mysql_table_count || 0} 张</p>
            <p className="text-xs text-text-tertiary mt-0.5">记录数: {status?.mysql_total_rows || 0} 条</p>
          </div>
          <div className="bg-surface p-4 rounded-xl border border-border">
            <p className="text-xs text-text-secondary">MySQL 连接状态</p>
            <div className="flex items-center gap-1.5 mt-1">
              <span className={`w-2.5 h-2.5 rounded-full ${status?.mysql_connected ? 'bg-emerald-400' : 'bg-slate-600'}`} />
              <p className="text-base font-semibold text-text-primary">{status?.mysql_connected ? '正常在线' : '未连接'}</p>
            </div>
            <p className="text-xs text-text-tertiary mt-0.5">支持 50+ 高并发连接</p>
          </div>
          <div className="bg-surface p-4 rounded-xl border border-border">
            <p className="text-xs text-text-secondary">数据读取来源</p>
            <p className="text-base font-bold text-indigo-300 mt-1">
              {isMySQL ? 'MySQL (已脱离 SQLite)' : 'SQLite 本地库'}
            </p>
            <p className="text-xs text-text-tertiary mt-0.5">{isMySQL ? '高可用集群级' : '单机进程级'}</p>
          </div>
        </div>
      </div>

      {/* 2. 目标 MySQL 目标连接配置 */}
      <div className="bg-surface border border-border rounded-2xl p-6 shadow-sm">
        <h4 className="font-semibold text-text-primary mb-3 flex items-center gap-2">
          <Database className="w-5 h-5 text-blue-600" />
          选择目标 MySQL 数据库
        </h4>
        <p className="text-xs text-text-secondary mb-4">
          请选择要进行数据同步与生产托管的 MySQL 数据库。可在「系统设置 → 数据库管理」中预先添加。
        </p>

        <div className="space-y-4 max-w-2xl">
          <div className="flex items-center gap-4">
            <label className="flex items-center gap-2 cursor-pointer text-sm text-text-secondary">
              <input
                type="radio"
                checked={!useCustomDsn}
                onChange={() => setUseCustomDsn(false)}
                className="text-blue-500 focus:ring-blue-500/50"
              />
              <span>从已保存的数据库连接选择</span>
            </label>
            <label className="flex items-center gap-2 cursor-pointer text-sm text-text-secondary">
              <input
                type="radio"
                checked={useCustomDsn}
                onChange={() => setUseCustomDsn(true)}
                className="text-blue-500 focus:ring-blue-500/50"
              />
              <span>手动输入 MySQL DSN</span>
            </label>
          </div>

          {!useCustomDsn ? (
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">已配置的 MySQL 连接</label>
              {dbConnections && dbConnections.length > 0 ? (
                <select
                  value={selectedConnId}
                  onChange={(e) => setSelectedConnId(e.target.value)}
                  className="w-full px-4 py-2.5 bg-background border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 text-sm"
                >
                  {dbConnections.map((conn) => (
                    <option key={conn.id} value={conn.id}>
                      {conn.name} ({conn.username}@{conn.host}:{conn.port}/{conn.database})
                    </option>
                  ))}
                </select>
              ) : (
                <div className="p-3 bg-amber-500/10 border border-amber-500/30 rounded-xl text-amber-300 text-xs flex items-center gap-2">
                  <AlertTriangle className="w-4 h-4 flex-shrink-0" />
                  <span>暂无已保存的 MySQL 连接，请先在「数据库管理」页面添加，或勾选上方手动输入 DSN。</span>
                </div>
              )}
            </div>
          ) : (
            <div>
              <label className="block text-xs font-medium text-text-secondary mb-1.5">MySQL DSN 连接字符串</label>
              <input
                type="text"
                placeholder="root:password@tcp(127.0.0.1:3306)/itops?charset=utf8mb4&parseTime=True&loc=Local"
                value={customDsn}
                onChange={(e) => setCustomDsn(e.target.value)}
                className="w-full px-4 py-2.5 bg-background border border-border rounded-xl text-text-primary font-mono text-xs focus:outline-none focus:border-blue-500/50"
              />
            </div>
          )}
        </div>
      </div>

      {/* 3. 核心操作区：生成配置、一键迁移、启用切换 */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        {/* 操作卡片 1：一键同步数据到 MySQL */}
        <div className="bg-surface border border-border rounded-2xl p-6 flex flex-col justify-between hover:border-blue-500/40 transition-all">
          <div>
            <div className="w-10 h-10 rounded-xl bg-blue-500/10 text-blue-600 flex items-center justify-center mb-4">
              <ArrowRightLeft className="w-5 h-5" />
            </div>
            <h4 className="text-base font-semibold text-text-primary mb-2">一键同步数据到 MySQL</h4>
            <p className="text-xs text-text-secondary leading-relaxed mb-4">
              自动在目标 MySQL 中创建全部 60+ 张业务表结构，并将当前 SQLite 中的所有数据（用户、设备、告警规则、自愈工作流、大屏面板）完整同步导入。
            </p>
          </div>
          <button
            onClick={() => migrateMutation.mutate()}
            disabled={migrateMutation.isPending || (!selectedConnId && !customDsn)}
            className="w-full py-2.5 px-4 bg-blue-600 hover:bg-blue-500 text-white font-medium rounded-xl transition-all shadow-lg shadow-blue-500/20 disabled:opacity-50 flex items-center justify-center gap-2 text-sm"
          >
            {migrateMutation.isPending ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                正在全量迁移数据...
              </>
            ) : (
              <>
                <Zap className="w-4 h-4" />
                一键导入全量数据到 MySQL
              </>
            )}
          </button>
        </div>

        {/* 操作卡片 2：启用 MySQL 模式 */}
        <div className="bg-surface border border-border rounded-2xl p-6 flex flex-col justify-between hover:border-emerald-500/40 transition-all">
          <div>
            <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center mb-4">
              <ShieldCheck className="w-5 h-5" />
            </div>
            <h4 className="text-base font-semibold text-text-primary mb-2">启用 MySQL 存储模式</h4>
            <p className="text-xs text-text-secondary leading-relaxed mb-4">
              切换主数据源为 MySQL，并将配置持久化写入服务端的 <code className="text-emerald-600">.env</code> 文件。切换后系统将完全读取与操作 MySQL，彻底不再使用 SQLite。
            </p>
          </div>
          {isMySQL ? (
            <button
              onClick={() => switchMutation.mutate('sqlite')}
              disabled={switchMutation.isPending}
              className="w-full py-2.5 px-4 bg-amber-600/80 hover:bg-amber-500 text-white font-medium rounded-xl transition-all disabled:opacity-50 flex items-center justify-center gap-2 text-sm"
            >
              {switchMutation.isPending ? <Loader2 className="w-4 h-4 animate-spin" /> : <RefreshCw className="w-4 h-4" />}
              切回 SQLite 单机模式
            </button>
          ) : (
            <button
              onClick={() => switchMutation.mutate('mysql')}
              disabled={switchMutation.isPending || (!selectedConnId && !customDsn)}
              className="w-full py-2.5 px-4 bg-emerald-600 hover:bg-emerald-500 text-white font-medium rounded-xl transition-all shadow-lg shadow-emerald-500/20 disabled:opacity-50 flex items-center justify-center gap-2 text-sm"
            >
              {switchMutation.isPending ? (
                <>
                  <Loader2 className="w-4 h-4 animate-spin" />
                  正在连接并切换...
                </>
              ) : (
                <>
                  <CheckCircle2 className="w-4 h-4" />
                  启用 MySQL 数据配置 (读取 MySQL)
                </>
              )}
            </button>
          )}
        </div>

        {/* 操作卡片 3：生成生产 .env 配置文件 */}
        <div className="bg-surface border border-border rounded-2xl p-6 flex flex-col justify-between hover:border-purple-500/40 transition-all">
          <div>
            <div className="w-10 h-10 rounded-xl bg-purple-500/10 text-purple-600 flex items-center justify-center mb-4">
              <FileCode className="w-5 h-5" />
            </div>
            <h4 className="text-base font-semibold text-text-primary mb-2">生成生产 .env 配置文件</h4>
            <p className="text-xs text-text-secondary leading-relaxed mb-4">
              根据当前选中的 MySQL 连接与 InfluxDB 设置，自动生成标准格式的生产环境变量配置文件，支持一键复制与下载。
            </p>
          </div>
          <button
            onClick={() => configMutation.mutate()}
            disabled={configMutation.isPending || (!selectedConnId && !customDsn)}
            className="w-full py-2.5 px-4 bg-purple-600 hover:bg-purple-500 text-white font-medium rounded-xl transition-all shadow-lg shadow-purple-500/20 disabled:opacity-50 flex items-center justify-center gap-2 text-sm"
          >
            {configMutation.isPending ? (
              <>
                <Loader2 className="w-4 h-4 animate-spin" />
                正在生成配置...
              </>
            ) : (
              <>
                <FileCode className="w-4 h-4" />
                生成 .env 配置文件
              </>
            )}
          </button>
        </div>
      </div>

      {/* 4. 迁移报告详情（如果刚刚执行了迁移） */}
      {migrationReport && (
        <div className="bg-surface border border-emerald-500/30 rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between mb-4">
            <h4 className="font-semibold text-text-primary flex items-center gap-2">
              <Layers className="w-5 h-5 text-emerald-600" />
              数据迁移报告（已写入 {migrationReport.total_tables} 张表，共 {migrationReport.total_rows} 条记录）
            </h4>
            <span className="text-xs text-text-secondary">
              耗时: {migrationReport.duration_ms}ms · 完成于 {migrationReport.migrated_at}
            </span>
          </div>

          <div className="max-h-60 overflow-y-auto rounded-xl border border-border bg-surface">
            <table className="w-full text-left text-xs">
              <thead className="bg-surface-2 text-text-secondary border-b border-border sticky top-0">
                <tr>
                  <th className="p-3">表名</th>
                  <th className="p-3">同步记录数</th>
                  <th className="p-3">状态</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border text-text-secondary">
                {migrationReport.details.map((d, i) => (
                  <tr key={i} className="hover:bg-surface-hover">
                    <td className="p-3 font-mono text-text-primary flex items-center gap-2">
                      <Table className="w-3.5 h-3.5 text-blue-600" />
                      {d.table_name}
                    </td>
                    <td className="p-3 font-semibold text-emerald-600">{d.rows} 行</td>
                    <td className="p-3">
                      <span className={`px-2 py-0.5 rounded-full text-[10px] font-semibold ${
                        d.status === 'success' ? 'bg-emerald-500/20 text-emerald-300' : 'bg-surface-2 text-text-secondary'
                      }`}>
                        {d.status === 'success' ? '同步完成' : '已跳过'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* 5. 生成的配置文件预览与复制/下载 */}
      {configMutation.data && (
        <div className="bg-surface border border-border rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between mb-4">
            <h4 className="font-semibold text-text-primary flex items-center gap-2">
              <FileCode className="w-5 h-5 text-purple-600" />
              生产环境 .env 配置文件预览
            </h4>
            <div className="flex items-center gap-2">
              <button
                onClick={() => handleCopyEnv(configMutation.data.env_content)}
                className="px-3 py-1.5 bg-surface-2 hover:bg-surface-hover text-text-primary rounded-lg text-xs font-medium transition-all flex items-center gap-1.5"
              >
                {copied ? <Check className="w-3.5 h-3.5 text-emerald-600" /> : <Copy className="w-3.5 h-3.5" />}
                {copied ? '已复制' : '复制配置'}
              </button>
              <button
                onClick={() => handleDownloadEnv(configMutation.data.env_content)}
                className="px-3 py-1.5 bg-blue-600 hover:bg-blue-500 text-white rounded-lg text-xs font-medium transition-all flex items-center gap-1.5 shadow-md shadow-blue-500/20"
              >
                <Download className="w-3.5 h-3.5" />
                下载 .env 文件
              </button>
            </div>
          </div>
          <pre className="p-4 bg-slate-950/80 rounded-xl text-xs font-mono text-slate-300 overflow-x-auto border border-slate-800 leading-relaxed max-h-72">
            {configMutation.data.env_content}
          </pre>
        </div>
      )}
    </div>
  );
}
