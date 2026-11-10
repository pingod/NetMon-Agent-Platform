import React from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  Bot,
  GitBranch,
  Play,
  Bell,
  Clock,
  Server,
  BookOpen,
  Zap,
  Activity,
  Shield,
  Network,
  ChevronRight
} from 'lucide-react';
import { useNavigate, Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import api from '../../../lib/api';
import { safeFormatDistance } from '../../../lib/date';

interface Agent {
  id: string;
  name: string;
  avatar: string;
  role: string;
  enabled: number;
}

interface Task {
  id: string;
  name: string;
  status: string;
  created_at: string;
}

interface Alert {
  id: string;
  title: string;
  severity: string;
  status: string;
  created_at: string;
}

interface Server {
  id: string;
  name: string;
  hostname: string;
  enabled: number;
  last_connected?: string;
}

interface Workflow {
  id: string;
  name: string;
  is_template: number;
}

interface Knowledge {
  id: string;
  title: string;
  category: string;
  usage_count: number;
}

export default function Dashboard() {
  const { t } = useTranslation();
  const navigate = useNavigate();

  const quickActions = [
    {
      id: 'networkTopology',
      title: t('dashboard.networkTopology'),
      desc: t('dashboard.networkTopologyDesc'),
      icon: Network,
      color: 'text-sky-500',
      bg: 'bg-sky-500/10',
      action: () => navigate('/topology'),
    },
    {
      id: 'systemInspection',
      title: t('dashboard.systemInspection'),
      desc: t('dashboard.systemInspectionDesc'),
      icon: Activity,
      color: 'text-blue-600',
      bg: 'bg-blue-600/10',
      action: () => navigate('/workflows'),
    },
    {
      id: 'executeScript',
      title: t('dashboard.executeScript'),
      desc: t('dashboard.executeScriptDesc'),
      icon: Zap,
      color: 'text-purple-600',
      bg: 'bg-purple-600/10',
      action: () => navigate('/scripts'),
    },
    {
      id: 'viewAlerts',
      title: t('dashboard.viewAlerts'),
      desc: t('dashboard.viewAlertsDesc'),
      icon: Bell,
      color: 'text-red-600',
      bg: 'bg-red-600/10',
      action: () => navigate('/alerts'),
    },
  ];

  const { data: agents, isLoading: agentsLoading } = useQuery({
    queryKey: ['agents'],
    queryFn: async () => {
      const res = await api.get('/agents');
      return res.data.data as Agent[];
    },
    staleTime: 60000,
  });

  const { data: servers, isLoading: serversLoading } = useQuery({
    queryKey: ['servers'],
    queryFn: async () => {
      const res = await api.get('/servers');
      return res.data.data as Server[];
    },
    staleTime: 60000,
  });

  const { data: workflows, isLoading: workflowsLoading } = useQuery({
    queryKey: ['workflows'],
    queryFn: async () => {
      const res = await api.get('/workflows');
      return res.data.data as Workflow[];
    },
    staleTime: 120000,
  });

  const { data: knowledge, isLoading: knowledgeLoading } = useQuery({
    queryKey: ['knowledge'],
    queryFn: async () => {
      const res = await api.get('/knowledge');
      return res.data.data as Knowledge[];
    },
    staleTime: 120000,
  });

  const { data: tasks, isLoading: tasksLoading } = useQuery({
    queryKey: ['tasks', { limit: 5 }],
    queryFn: async () => {
      const res = await api.get('/tasks', { params: { limit: 5 } });
      return res.data.data as Task[];
    },
    staleTime: 30000,
  });

  const { data: alerts, isLoading: alertsLoading } = useQuery({
    queryKey: ['alerts', { limit: 5 }],
    queryFn: async () => {
      const res = await api.get('/alerts', { params: { limit: 5 } });
      return res.data.data as Alert[];
    },
    staleTime: 30000,
  });

  // 统计卡片用后端聚合总数，避免只看最近 5 条造成的低估
  const { data: dashboardStats, isLoading: dashboardStatsLoading } = useQuery({
    queryKey: ['dashboard-stats'],
    queryFn: async () => {
      const res = await api.get('/dashboard/full');
      return res.data.data?.stats as {
        tasks?: { running?: number };
        alerts?: { active?: number };
      } | undefined;
    },
    staleTime: 30000,
  });

  const isLoading = agentsLoading || serversLoading || workflowsLoading || knowledgeLoading || tasksLoading || alertsLoading || dashboardStatsLoading;

  const stats = [
    {
      id: 'servers',
      value: servers?.length || 0,
      icon: Server,
      color: 'text-purple-500',
      bg: 'bg-purple-500/10',
    },
    {
      id: 'agents',
      value: agents?.length || 0,
      icon: Bot,
      color: 'text-blue-500',
      bg: 'bg-blue-500/10',
    },
    {
      id: 'workflowTemplates',
      value: workflows?.filter((w) => w.is_template === 1).length || 0,
      icon: GitBranch,
      color: 'text-green-500',
      bg: 'bg-green-500/10',
    },
    {
      id: 'runningTasks',
      value: dashboardStats?.tasks?.running ?? 0,
      icon: Play,
      color: 'text-yellow-500',
      bg: 'bg-yellow-500/10',
    },
    {
      id: 'activeAlerts',
      value: dashboardStats?.alerts?.active ?? 0,
      icon: Bell,
      color: 'text-red-500',
      bg: 'bg-red-500/10',
    },
    {
      id: 'knowledge',
      value: knowledge?.length || 0,
      icon: BookOpen,
      color: 'text-cyan-500',
      bg: 'bg-cyan-500/10',
    },
  ];

  return (
    <div className="h-full overflow-auto page-shell space-y-6">
      {/* 页面标题 */}
      <div className="page-header">
        <div>
          <div className="page-eyebrow">OVERVIEW</div>
          <h1 className="page-title">{t('dashboard.title')}</h1>
          <p className="page-subtitle">{t('dashboard.subtitle')}</p>
        </div>

        <div className="flex items-center gap-2">
          <Link
            to="/topology"
            className="btn btn-soft h-9 px-3.5 text-[12px]"
          >
            <Network className="w-3.5 h-3.5" />
            进入网络拓扑
            <ChevronRight className="w-3.5 h-3.5" />
          </Link>
        </div>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="bg-surface rounded-xl p-6 border border-border animate-pulse">
              <div className="flex items-center justify-between mb-4">
                <div className="w-12 h-12 rounded-lg bg-border/50" />
                <div className="w-8 h-8 rounded bg-border/50" />
              </div>
              <div className="h-8 w-16 bg-border/50 rounded mb-2" />
              <div className="h-4 w-24 bg-border/50 rounded" />
            </div>
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-7 gap-4">
          {/* 左侧：6个统计卡片 */}
          <div className="grid grid-cols-2 gap-3 md:grid-cols-3 lg:col-span-4">
            {stats.map((stat) => (
              <div
                key={stat.id}
                className="metric-card cursor-pointer"
                onClick={() => {
                  if (stat.id === 'servers') navigate('/servers');
                  else if (stat.id === 'agents') navigate('/agents');
                  else if (stat.id === 'workflowTemplates') navigate('/workflows');
                  else if (stat.id === 'runningTasks') navigate('/tasks');
                  else if (stat.id === 'activeAlerts') navigate('/alerts');
                  else if (stat.id === 'knowledge') navigate('/knowledge');
                }}
              >
                <div className="mb-3 flex items-center justify-between">
                  <span className={`flex h-9 w-9 items-center justify-center rounded-xl ${stat.bg}`}>
                    <stat.icon className={`h-4 w-4 ${stat.color}`} />
                  </span>
                </div>
                <p className="metric-value">{stat.value}</p>
                <p className="metric-label mt-1">{t(`dashboard.${stat.id}`)}</p>
              </div>
            ))}
          </div>

          {/* 右侧：最新告警 */}
          <div className="panel p-4 lg:col-span-3">
            <div className="flex items-center justify-between mb-3">
              <h2 className="text-sm font-semibold text-text-primary flex items-center gap-1.5">
                <Bell className="w-4 h-4 text-red-500" />
                {t('dashboard.latestAlerts')}
              </h2>
              <Link to="/alerts" className="text-xs text-primary hover:underline">
                {t('dashboard.viewAll')} →
              </Link>
            </div>
            <div className="space-y-2">
              {alerts?.slice(0, 6).map((alert) => (
                <div
                  key={alert.id}
                  className="rounded-lg border border-transparent bg-surface-2 px-3 py-2.5 transition-colors hover:border-border"
                >
                  <div className="flex items-start justify-between gap-2">
                    <h3 className="font-medium text-text-primary text-xs leading-tight truncate">{alert.title}</h3>
                    <span
                      className={`px-1.5 py-0.5 rounded text-[10px] font-medium flex-shrink-0 ${
                        alert.severity === 'critical'
                          ? 'bg-status-failed/10 text-status-failed'
                          : alert.severity === 'high'
                          ? 'bg-status-warning/10 text-status-warning'
                          : 'bg-status-pending/10 text-status-pending'
                      }`}
                    >
                      {alert.severity}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[10px] text-text-secondary mt-1">
                    <Clock className="w-3 h-3" />
                    {safeFormatDistance(alert.created_at)}
                  </div>
                </div>
              ))}
              {!alerts || alerts.length === 0 ? (
                <div className="text-center py-6 text-text-secondary text-xs">{t('dashboard.noAlerts')}</div>
              ) : null}
            </div>
          </div>
        </div>
      )}

      {/* 快捷操作 */}
      <div className="panel p-5">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-base font-semibold text-text-primary flex items-center gap-2">
            <Zap className="w-4 h-4 text-primary" />
            {t('dashboard.quickActions')}
          </h2>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-3">
          {quickActions.map((action) => (
            <button
              key={action.id}
              onClick={action.action}
              className="group rounded-xl border border-border bg-surface-2 p-4 text-left transition-all hover:border-primary/40 hover:bg-surface-3"
            >
              <div className={`w-10 h-10 rounded-lg ${action.bg} flex items-center justify-center mb-2 group-hover:scale-110 transition-transform`}>
                <action.icon className={`w-5 h-5 ${action.color}`} />
              </div>
              <h3 className="font-semibold text-text-primary text-sm mb-0.5">{action.title}</h3>
              <p className="text-xs text-text-secondary">{action.desc}</p>
            </button>
          ))}
        </div>
      </div>

      {/* 底部各板块概览 */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <div className="panel p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-text-primary flex items-center gap-1.5">
              <Server className="w-4 h-4 text-purple-500" />
              {t('dashboard.servers')}
            </h2>
            <Link to="/servers" className="text-xs text-primary hover:underline">{t('dashboard.viewAll')}</Link>
          </div>
          <div className="space-y-2">
            {(Array.isArray(servers) ? servers : []).slice(0, 5).map((server) => (
              <div
                key={server.id}
                className="flex items-center gap-2 rounded-lg bg-surface-2 px-2.5 py-2 transition-colors hover:bg-surface-3"
              >
                <div className={`p-1.5 rounded-md ${server.enabled ? 'bg-purple-500/10' : 'bg-status-failed/10'}`}>
                  <Server className={`w-4 h-4 ${server.enabled ? 'text-purple-500' : 'text-text-secondary'}`} />
                </div>
                <div className="flex-1 min-w-0">
                  <h3 className="font-medium text-text-primary text-xs truncate">{server.name}</h3>
                  <p className="text-[10px] text-text-secondary truncate">{server.hostname}</p>
                </div>
                <span
                  className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
                    server.enabled
                      ? 'bg-status-success/10 text-status-success'
                      : 'bg-status-failed/10 text-status-failed'
                  }`}
                >
                  {server.enabled ? t('dashboard.enabled') : t('dashboard.disabled')}
                </span>
              </div>
            ))}
            {!servers || servers.length === 0 ? (
              <div className="text-center py-6 text-text-secondary text-xs">{t('dashboard.noServers')}</div>
            ) : null}
          </div>
        </div>

        <div className="panel p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-text-primary flex items-center gap-1.5">
              <Bot className="w-4 h-4 text-primary" />
              {t('dashboard.onlineAgents')}
            </h2>
            <Link to="/agents" className="text-xs text-primary hover:underline">{t('dashboard.viewAll')}</Link>
          </div>
          <div className="space-y-2">
            {agents?.slice(0, 5).map((agent) => (
              <div
                key={agent.id}
                className="flex items-center gap-2 rounded-lg bg-surface-2 px-2.5 py-2 transition-colors hover:bg-surface-3"
              >
                <span className="text-lg">{agent.avatar}</span>
                <div className="flex-1 min-w-0">
                  <h3 className="font-medium text-text-primary text-xs">{agent.name}</h3>
                  <p className="text-[10px] text-text-secondary">{agent.role}</p>
                </div>
                <span
                  className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
                    agent.enabled
                      ? 'bg-status-success/10 text-status-success'
                      : 'bg-status-failed/10 text-status-failed'
                  }`}
                >
                  {agent.enabled ? t('dashboard.online') : t('dashboard.offline')}
                </span>
              </div>
            ))}
          </div>
        </div>

        <div className="panel p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-sm font-semibold text-text-primary flex items-center gap-1.5">
              <BookOpen className="w-4 h-4 text-cyan-500" />
              {t('dashboard.knowledge')}
            </h2>
            <Link to="/knowledge" className="text-xs text-primary hover:underline">{t('dashboard.viewAll')}</Link>
          </div>
          <div className="space-y-2">
            {knowledge?.slice(0, 5).map((item) => (
              <div
                key={item.id}
                className="flex items-start gap-2 rounded-lg bg-surface-2 px-2.5 py-2 transition-colors hover:bg-surface-3"
              >
                <div className="p-1.5 rounded-md bg-cyan-500/10">
                  <BookOpen className="w-3.5 h-3.5 text-cyan-500" />
                </div>
                <div className="flex-1 min-w-0">
                  <h3 className="font-medium text-text-primary text-xs truncate">{item.title}</h3>
                  <div className="flex items-center justify-between mt-0.5">
                    <span className="text-[10px] text-text-secondary">{item.category}</span>
                    <span className="text-[10px] text-status-success">{item.usage_count || 0}{t('dashboard.timesUsed')}</span>
                  </div>
                </div>
              </div>
            ))}
            {!knowledge || knowledge.length === 0 ? (
              <div className="text-center py-6 text-text-secondary text-xs">{t('dashboard.noKnowledge')}</div>
            ) : null}
          </div>
        </div>

        <div className="panel p-4 lg:col-span-3">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-sm font-semibold text-text-primary flex items-center gap-1.5">
              <Play className="w-4 h-4 text-green-500" />
              {t('dashboard.recentTasks')}
            </h2>
            <Link to="/tasks" className="text-xs text-primary hover:underline">{t('dashboard.viewAll')}</Link>
          </div>
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="text-left text-xs text-text-secondary border-b border-border">
                  <th className="pb-2 font-medium">{t('dashboard.taskName')}</th>
                  <th className="pb-2 font-medium">{t('dashboard.status')}</th>
                  <th className="pb-2 font-medium">{t('dashboard.executionTime')}</th>
                </tr>
              </thead>
              <tbody>
                {tasks?.map((task) => (
                  <tr key={task.id} className="border-b border-border/50 hover:bg-background/50">
                    <td className="py-2 text-text-primary text-xs">{task.name}</td>
                    <td className="py-2">
                      <span
                        className={`px-1.5 py-0.5 rounded text-[10px] font-medium ${
                          task.status === 'completed'
                            ? 'bg-status-success/10 text-status-success'
                            : task.status === 'running'
                            ? 'bg-status-running/10 text-status-running'
                            : task.status === 'failed'
                            ? 'bg-status-failed/10 text-status-failed'
                            : 'bg-status-pending/10 text-status-pending'
                        }`}
                      >
                        {task.status}
                      </span>
                    </td>
                    <td className="py-2 text-xs text-text-secondary">
                      {safeFormatDistance(task.created_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  );
}
