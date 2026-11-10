import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { logger } from '@/lib/logger';
import api from '../../../../lib/api';
import type {
  Alert,
  AlertTrendPoint,
  AgentStat,
  DashboardStats,
  DataPoint,
  NetworkDeviceStats,
  RemediationStats,
  ServerMetricsData,
  ServerType,
  SlaStats,
  Task,
  TaskTrendPoint,
  TaskWithProgress,
} from './types';
import { SERVER_COLORS } from './types';

const RETRY_CONFIG = { retry: 3, retryDelay: (attemptIndex: number) => Math.min(1000 * 2 ** attemptIndex, 5000) };

export function useBigScreenData() {
  const navigate = useNavigate();
  const [currentTime, setCurrentTime] = useState(new Date());
  const [refreshKey, setRefreshKey] = useState(0);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [dashboardTitle, setDashboardTitle] = useState(() => {
    const saved = localStorage.getItem('dashboardTitle');
    return saved || 'NetMon 网络监控大屏';
  });
  const [isEditingTitle, setIsEditingTitle] = useState(false);
  const [titleInputValue, setTitleInputValue] = useState(dashboardTitle);

  const prevCriticalCountRef = useRef(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const audioContextRef = useRef<AudioContext | null>(null);
  const criticalAlertSoundPlayedRef = useRef(false);
  const criticalSoundResetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const toggleFullscreen = useCallback(() => {
    if (!document.fullscreenElement) {
      containerRef.current?.requestFullscreen();
      setIsFullscreen(true);
    } else {
      document.exitFullscreen();
      setIsFullscreen(false);
    }
  }, []);

  useEffect(() => {
    const timer = setInterval(() => setCurrentTime(new Date()), 1000);
    return () => clearInterval(timer);
  }, []);

  useEffect(() => {
    if (dashboardTitle !== 'NetMon 网络监控大屏') {
      localStorage.setItem('dashboardTitle', dashboardTitle);
    }
  }, [dashboardTitle]);

  const handleSaveTitle = () => {
    if (titleInputValue.trim()) {
      setDashboardTitle(titleInputValue.trim());
      setIsEditingTitle(false);
    }
  };

  const handleCancelEditTitle = () => {
    setTitleInputValue(dashboardTitle);
    setIsEditingTitle(false);
  };

  useEffect(() => {
    const handleKeydown = (e: KeyboardEvent) => {
      if (e.key === 'F11') {
        e.preventDefault();
        toggleFullscreen();
      }
      if (e.key === 'Escape' && isFullscreen) {
        setIsFullscreen(false);
      }
    };
    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  }, [isFullscreen, toggleFullscreen]);

  const refreshData = useCallback(() => {
    setRefreshKey((prev) => prev + 1);
  }, []);

  const { data: fullDashboard, isError: isStatsError } = useQuery({
    queryKey: ['dashboard-full', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/full');
      return res.data.data as {
        stats: DashboardStats;
        recentTasks: Task[];
        recentAlerts: Alert[];
        servers: ServerType[];
      };
    },
    refetchInterval: 30000,
    ...RETRY_CONFIG,
  });

  const stats = fullDashboard?.stats;
  const servers = fullDashboard?.servers;
  const alerts = fullDashboard?.recentAlerts;

  const { data: rawTasks } = useQuery({
    queryKey: ['tasks', { limit: 10 }, refreshKey],
    queryFn: async () => {
      const res = await api.get('/tasks', { params: { limit: 10 } });
      return res.data.data as Task[];
    },
    refetchInterval: 15000,
    ...RETRY_CONFIG,
  });

  const tasks: TaskWithProgress[] = useMemo(() => {
    if (!rawTasks) return [];
    return rawTasks.map((task) => {
      let progress = 0;
      let completedNodes = 0;
      let totalNodes = 0;
      let executingNode = '';

      if (task.status === 'completed') {
        progress = 100;
      } else if (task.status === 'failed') {
        try {
          const results = task.node_results ? (JSON.parse(task.node_results) as Record<string, { status: string }>) : {};
          const completedCount = Object.values(results).filter((r) => r.status === 'completed').length;
          totalNodes = Object.keys(results).length;
          completedNodes = completedCount;
          progress = totalNodes > 0 ? Math.round((completedCount / totalNodes) * 100) : 0;
        } catch {
          progress = 0;
        }
      } else if (task.status === 'running') {
        try {
          const execOrder = task.execution_order ? (JSON.parse(task.execution_order) as string[]) : [];
          const results = task.node_results ? (JSON.parse(task.node_results) as Record<string, { status: string }>) : {};
          totalNodes = execOrder.length;
          completedNodes = Object.values(results).filter((r) => r.status === 'completed').length;
          executingNode = task.current_node_id || '';
          progress = totalNodes > 0 ? Math.round((completedNodes / totalNodes) * 100) : 0;
        } catch {
          progress = 0;
        }
      }

      return { ...task, progress, completedNodes, totalNodes, executingNode };
    });
  }, [rawTasks]);

  const { data: alertTrends } = useQuery({
    queryKey: ['alert-trends', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/alert-trends');
      return res.data.data as AlertTrendPoint[];
    },
    refetchInterval: 60000,
    ...RETRY_CONFIG,
  });

  const { data: taskTrends } = useQuery({
    queryKey: ['task-trends', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/task-trends');
      return res.data.data as TaskTrendPoint[];
    },
    refetchInterval: 60000,
    ...RETRY_CONFIG,
  });

  const { data: agentStats } = useQuery({
    queryKey: ['agent-stats', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/agent-stats');
      return res.data.data as {
        agents: AgentStat[];
        overall: {
          totalExecutions: number;
          totalSuccess: number;
          overallSuccessRate: number;
          todayExecutions: number;
        };
      };
    },
    refetchInterval: 60000,
    ...RETRY_CONFIG,
  });

  const { data: taskDistribution } = useQuery({
    queryKey: ['task-distribution', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/task-distribution');
      return res.data.data as {
        byStatus: Array<{ status: string; count: number }>;
        byWorkflow: Array<{ name: string; count: number }>;
      };
    },
    refetchInterval: 60000,
    ...RETRY_CONFIG,
  });

  const { data: remediationStats } = useQuery<RemediationStats>({
    queryKey: ['remediation-stats', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/remediation-stats');
      return res.data.data;
    },
    refetchInterval: 30000,
    ...RETRY_CONFIG,
  });

  const { data: serverMetricsData } = useQuery<ServerMetricsData>({
    queryKey: ['server-metrics', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/server-metrics');
      return res.data.data;
    },
    refetchInterval: 30000,
    ...RETRY_CONFIG,
  });

  const { data: slaStats } = useQuery<SlaStats>({
    queryKey: ['sla-stats', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/sla-stats');
      return res.data.data;
    },
    refetchInterval: 60000,
    ...RETRY_CONFIG,
  });

  const { data: networkDeviceStats } = useQuery<NetworkDeviceStats>({
    queryKey: ['network-device-stats', refreshKey],
    queryFn: async () => {
      const res = await api.get('/dashboard/network-device-stats');
      return res.data.data;
    },
    refetchInterval: 60000,
    ...RETRY_CONFIG,
  });

  const playCriticalAlertSound = useCallback(() => {
    try {
      const audioContext = audioContextRef.current || new AudioContext();
      audioContextRef.current = audioContext;

      const oscillator = audioContext.createOscillator();
      const gainNode = audioContext.createGain();

      oscillator.connect(gainNode);
      gainNode.connect(audioContext.destination);

      oscillator.type = 'square';
      oscillator.frequency.setValueAtTime(880, audioContext.currentTime);
      oscillator.frequency.setValueAtTime(660, audioContext.currentTime + 0.15);
      oscillator.frequency.setValueAtTime(880, audioContext.currentTime + 0.3);

      gainNode.gain.setValueAtTime(0.15, audioContext.currentTime);
      gainNode.gain.exponentialRampToValueAtTime(0.001, audioContext.currentTime + 0.5);

      oscillator.start(audioContext.currentTime);
      oscillator.stop(audioContext.currentTime + 0.5);
    } catch (error) {
      logger.error('Failed to play critical alert sound:', error);
    }
  }, []);

  // 卸载时释放 AudioContext 与声音节流定时器，避免音频资源泄漏
  useEffect(() => {
    const contextRef = audioContextRef;
    return () => {
      if (criticalSoundResetTimerRef.current) {
        clearTimeout(criticalSoundResetTimerRef.current);
        criticalSoundResetTimerRef.current = null;
      }
      const context = contextRef.current;
      if (context && context.state !== 'closed') {
        void context.close();
      }
      contextRef.current = null;
    };
  }, []);

  useEffect(() => {
    const newCriticalCount = stats?.alerts.critical || 0;

    if (newCriticalCount > prevCriticalCountRef.current && newCriticalCount > 0 && !criticalAlertSoundPlayedRef.current) {
      playCriticalAlertSound();
      criticalAlertSoundPlayedRef.current = true;
      criticalSoundResetTimerRef.current = setTimeout(() => {
        criticalAlertSoundPlayedRef.current = false;
        criticalSoundResetTimerRef.current = null;
      }, 30000);
    }

    prevCriticalCountRef.current = newCriticalCount;
  }, [stats?.alerts.critical, playCriticalAlertSound]);

  const hasCriticalAlerts = (stats?.alerts.critical || 0) > 0;
  const hasHighAlerts = (stats?.alerts.high || 0) > 0;
  const systemHealthStatus = hasCriticalAlerts ? 'critical' : hasHighAlerts ? 'warning' : 'healthy';

  // Chart data — 初始为空，仅在有真实数据时填充
  const [cpuData, setCpuData] = useState<DataPoint[]>([]);
  const [memoryData, setMemoryData] = useState<DataPoint[]>([]);
  const [networkData, setNetworkData] = useState<DataPoint[]>([]);
  const [diskIOData, setDiskIOData] = useState<DataPoint[]>([]);

  useEffect(() => {
    if (serverMetricsData?.has_real_data && serverMetricsData.cpu_history.length > 0) {
      const aggregateMetric = (history: Array<{ server_id: string; value: number; timestamp: string }>) => {
        const timeMap = new Map<string, number[]>();
        history.forEach((h) => {
          if (!timeMap.has(h.timestamp)) timeMap.set(h.timestamp, []);
          timeMap.get(h.timestamp)!.push(h.value);
        });
        const points: DataPoint[] = [];
        timeMap.forEach((values, ts) => {
          points.push({
            timestamp: new Date(ts).getTime(),
            value: values.reduce((a, b) => a + b, 0) / values.length,
          });
        });
        return points.sort((a, b) => a.timestamp - b.timestamp).slice(-30);
      };

      setCpuData(aggregateMetric(serverMetricsData.cpu_history));
      setMemoryData(aggregateMetric(serverMetricsData.memory_history));
      setNetworkData(aggregateMetric(serverMetricsData.network_history));
      setDiskIOData(aggregateMetric(serverMetricsData.disk_history));
    } else {
      // 无真实数据时清空，不做随机填充
      setCpuData([]);
      setMemoryData([]);
      setNetworkData([]);
      setDiskIOData([]);
    }
  }, [serverMetricsData]);

  const alertTrendData = (alertTrends || []).map((t) => ({
    timestamp: new Date(t.time_bucket).getTime(),
    value: t.total,
  }));

  const taskTrendData = (taskTrends || []).map((t) => ({
    timestamp: new Date(t.time_bucket).getTime(),
    value: t.total,
  }));

  const serverMetrics = useMemo(() => {
    if (serverMetricsData?.has_real_data && serverMetricsData.servers.length > 0) {
      return serverMetricsData.servers.slice(0, 6).map((s, i) => ({
        label: s.server_name.substring(0, 8),
        value: s.cpu_usage ?? 0,
        color: SERVER_COLORS[i],
      }));
    }
    return [];
  }, [serverMetricsData]);

  const aggregatedMetrics = useMemo(() => {
    if (serverMetricsData?.has_real_data && serverMetricsData.servers.length > 0) {
      const validCpu = serverMetricsData.servers.filter((s) => s.cpu_usage !== null);
      const validMem = serverMetricsData.servers.filter((s) => s.memory_usage !== null);
      const validNetIn = serverMetricsData.servers.filter((s) => s.network_in_mbps !== null);
      const validNetOut = serverMetricsData.servers.filter((s) => s.network_out_mbps !== null);
      const validDisk = serverMetricsData.servers.filter((s) => s.disk_usage !== null);

      return {
        cpu: validCpu.length > 0 ? validCpu.reduce((sum, s) => sum + (s.cpu_usage ?? 0), 0) / validCpu.length : null,
        memory: validMem.length > 0 ? validMem.reduce((sum, s) => sum + (s.memory_usage ?? 0), 0) / validMem.length : null,
        networkIn: validNetIn.length > 0 ? validNetIn.reduce((sum, s) => sum + (s.network_in_mbps ?? 0), 0) / validNetIn.length : null,
        networkOut: validNetOut.length > 0 ? validNetOut.reduce((sum, s) => sum + (s.network_out_mbps ?? 0), 0) / validNetOut.length : null,
        disk: validDisk.length > 0 ? validDisk.reduce((sum, s) => sum + (s.disk_usage ?? 0), 0) / validDisk.length : null,
      };
    }
    return {
      cpu: null,
      memory: null,
      networkIn: null,
      networkOut: null,
      disk: null,
    };
  }, [serverMetricsData]);

  const taskDistData = (taskDistribution?.byStatus || []).map((s) => {
    const colors: Record<string, string> = {
      completed: '#22c55e',
      running: '#3b82f6',
      failed: '#ef4444',
      pending: '#64748b',
    };
    return {
      label: s.status,
      value: s.count,
      color: colors[s.status] || '#64748b',
    };
  });

  return {
    navigate,
    currentTime,
    isFullscreen,
    dashboardTitle,
    isEditingTitle,
    titleInputValue,
    stats,
    servers,
    alerts,
    tasks,
    isStatsError,
    criticalAlertCount: stats?.alerts.critical || 0,
    hasCriticalAlerts,
    hasHighAlerts,
    systemHealthStatus,
    refreshData,
    refreshKey,
    toggleFullscreen,
    containerRef,
    setIsEditingTitle,
    setTitleInputValue,
    handleSaveTitle,
    handleCancelEditTitle,
    cpuData,
    memoryData,
    networkData,
    diskIOData,
    alertTrendData,
    taskTrendData,
    serverMetrics,
    aggregatedMetrics,
    taskDistData,
    agentStats,
    remediationStats,
    slaStats,
    serverMetricsData,
    networkDeviceStats,
  };
}