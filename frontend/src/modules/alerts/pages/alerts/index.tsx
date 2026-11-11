/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo, useEffect, useRef } from 'react';
import { useQuery, useMutation } from '@tanstack/react-query';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Bell, CheckCircle, Clock, AlertCircle, Loader2 } from 'lucide-react';
import api from '../../../../lib/api';
import { useToast } from '../../../../contexts/ToastContext';
import { getAxiosErrorMessage } from '../../../../lib/errorHandler';
import AlertFilterBar from './AlertFilterBar';
import AlertList from './AlertList';
import AlertDetailPanel from './AlertDetailPanel';
import AlertDetailModal from './AlertDetailModal';
import AutomationLogPanel from './AutomationLogPanel';
import type { Alert, ProcessResult, AutomationLog, AnalysisItem } from './types';

export default function Alerts() {
  const toast = useToast();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [searchQuery, setSearchQuery] = useState('');
  const [severityFilter, setSeverityFilter] = useState<string>('all');
  const [statusFilter, setStatusFilter] = useState<string>('all');
  const [automationLogAlert, setAutomationLogAlert] = useState<Alert | null>(null);
  const [processResult, setProcessResult] = useState<ProcessResult | null>(null);
  const [selectedAlert, setSelectedAlert] = useState<Alert | null>(null);
  // 记录已自动打开过的 URL 告警参数，避免轮询刷新后重复弹出
  const handledAlertParamRef = useRef<string | null>(null);

  const { data: alerts, refetch } = useQuery({
    queryKey: ['alerts', statusFilter, severityFilter],
    queryFn: async () => {
      const params: Record<string, string> = {};
      if (statusFilter !== 'all') params.status = statusFilter;
      if (severityFilter !== 'all') params.severity = severityFilter;
      const res = await api.get('/alerts', { params });
      return res.data.data as Alert[];
    },
    staleTime: 30000,
    // 后端无 socket.io 推送，轮询保证新告警可见
    refetchInterval: 30000,
  });

  // 关联数据：AI 分析结果 + 修复执行记录
  const { data: analysisMap = {} } = useQuery({
    queryKey: ['alert-auto-analysis-map'],
    queryFn: async () => {
      const res = await api.get('/alert-auto-analysis?limit=200');
      const items = (res.data.data || []) as AnalysisItem[];
      const map: Record<string, AnalysisItem> = {};
      items.forEach((item) => { if (item.alert_id && !map[item.alert_id]) map[item.alert_id] = item; });
      return map;
    },
    refetchInterval: 30000,
  });

  const { data: automationLogs = [], isLoading: automationLogsLoading } = useQuery({
    queryKey: ['alert-automation-logs', automationLogAlert?.id],
    enabled: !!automationLogAlert,
    queryFn: async () => {
      const res = await api.get(`/alerts/${automationLogAlert!.id}/automation-logs`);
      return (res.data.data || []) as AutomationLog[];
    },
  });

  const serverSideFilteredAlerts = useMemo(() => {
    if (!alerts) return [];
    if (!searchQuery) return alerts;
    return alerts.filter((alert) => {
      const q = searchQuery.toLowerCase();
      return (alert.title || '').toLowerCase().includes(q) ||
        (alert.content || '').toLowerCase().includes(q) ||
        (alert.source || '').toLowerCase().includes(q);
    });
  }, [alerts, searchQuery]);

  const acknowledgeMutation = useMutation({
    mutationFn: async (alertId: string) => {
      await api.put(`/alerts/${alertId}/acknowledge`);
    },
    onSuccess: () => refetch(),
    onError: (err) => toast.error(getAxiosErrorMessage(err, '确认告警失败')),
  });

  const resolveMutation = useMutation({
    mutationFn: async (alertId: string) => {
      await api.put(`/alerts/${alertId}/resolve`);
    },
    onSuccess: () => refetch(),
    onError: (err) => toast.error(getAxiosErrorMessage(err, '解决告警失败')),
  });

  const processMutation = useMutation({
    mutationFn: async (alertId: string) => {
      const res = await api.post(`/alerts/${alertId}/process`);
      return res.data;
    },
    onSuccess: (data) => {
      if (data?.data) {
        setProcessResult(data.data);
      }
      refetch();
    },
    onError: (err) => toast.error(getAxiosErrorMessage(err, '处理告警失败')),
  });

  const exportMutation = useMutation({
    mutationFn: async () => {
      const params: Record<string, string> = {};
      if (statusFilter !== 'all') params.status = statusFilter;
      if (severityFilter !== 'all') params.severity = severityFilter;
      const res = await api.get('/alerts/export', { params, responseType: 'blob' });
      const url = window.URL.createObjectURL(res.data);
      const a = document.createElement('a');
      a.href = url;
      a.download = `alerts-${new Date().toISOString().slice(0, 10)}.csv`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    },
    onSuccess: () => toast.success('CSV 导出成功'),
    onError: (err) => toast.error(getAxiosErrorMessage(err, '导出失败')),
  });

  // 支持 URL 参数 ?alertId=... 或 ?highlight=... 自动弹出告警详情弹窗
  useEffect(() => {
    const alertIdParam = searchParams.get('alertId') || searchParams.get('highlight');
    if (!alertIdParam || !alerts || alerts.length === 0) return;
    if (handledAlertParamRef.current === alertIdParam) return;
    const target = alerts.find(a => a.id === alertIdParam);
    if (target) {
      handledAlertParamRef.current = alertIdParam;
      setSelectedAlert(target);
    }
  }, [searchParams, alerts]);

  return (
    <div className="h-full overflow-auto p-6">
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-text-primary mb-2">告警中心</h1>
            <p className="text-text-secondary">查看和管理系统告警</p>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div className="bg-surface rounded-xl p-5 border border-border hover:border-status-failed/30 transition-all">
            <div className="p-2 bg-status-failed/10 rounded-lg w-fit mb-3">
              <AlertCircle className="w-5 h-5 text-status-failed" />
            </div>
            <p className="text-3xl font-bold text-text-primary mb-1">
              {alerts?.filter((a) => a.status === 'new' || a.status === 'firing').length || 0}
            </p>
            <p className="text-sm text-text-secondary">新告警</p>
          </div>
          <div className="bg-surface rounded-xl p-5 border border-border hover:border-status-warning/30 transition-all">
            <div className="p-2 bg-status-warning/10 rounded-lg w-fit mb-3">
              <Clock className="w-5 h-5 text-status-warning" />
            </div>
            <p className="text-3xl font-bold text-text-primary mb-1">
              {alerts?.filter((a) => a.status === 'acknowledged').length || 0}
            </p>
            <p className="text-sm text-text-secondary">已确认</p>
          </div>
          <div className="bg-surface rounded-xl p-5 border border-border hover:border-status-success/30 transition-all">
            <div className="p-2 bg-status-success/10 rounded-lg w-fit mb-3">
              <CheckCircle className="w-5 h-5 text-status-success" />
            </div>
            <p className="text-3xl font-bold text-text-primary mb-1">
              {alerts?.filter((a) => a.status === 'resolved').length || 0}
            </p>
            <p className="text-sm text-text-secondary">已解决</p>
          </div>
          <div className="bg-surface rounded-xl p-5 border border-border hover:border-primary/30 transition-all">
            <div className="p-2 bg-primary/10 rounded-lg w-fit mb-3">
              <Bell className="w-5 h-5 text-primary" />
            </div>
            <p className="text-3xl font-bold text-text-primary mb-1">{alerts?.length || 0}</p>
            <p className="text-sm text-text-secondary">总计</p>
          </div>
        </div>

        <div className="bg-surface rounded-xl border border-border overflow-hidden">
          <AlertFilterBar
            searchQuery={searchQuery}
            onSearchChange={setSearchQuery}
            severityFilter={severityFilter}
            onSeverityChange={setSeverityFilter}
            statusFilter={statusFilter}
            onStatusChange={setStatusFilter}
            onExport={() => exportMutation.mutate()}
            exporting={exportMutation.isPending}
          />
          <AlertList
            alerts={serverSideFilteredAlerts}
            analysisMap={analysisMap}
            onProcess={(alertId) => processMutation.mutate(alertId)}
            processPending={processMutation.isPending}
            onAcknowledge={(alertId) => acknowledgeMutation.mutate(alertId)}
            onResolve={(alertId) => resolveMutation.mutate(alertId)}
            navigate={(path) => navigate(path)}
            onViewAutomationLog={setAutomationLogAlert}
            onSelectAlert={(alert) => setSelectedAlert(alert)}
          />
        </div>
      </div>

      {/* 告警详细信息弹窗 (包括具体触发值、PromQL、关联设备与Tags等) */}
      {selectedAlert && (
        <AlertDetailModal
          alert={selectedAlert}
          onClose={() => setSelectedAlert(null)}
          onAcknowledge={(alertId) => acknowledgeMutation.mutate(alertId)}
          onResolve={(alertId) => resolveMutation.mutate(alertId)}
          onProcess={(alertId) => processMutation.mutate(alertId)}
          navigate={(path) => navigate(path)}
        />
      )}

      {processResult && (
        <AlertDetailPanel
          processResult={processResult}
          onClose={() => setProcessResult(null)}
          navigate={(path) => navigate(path)}
        />
      )}

      {/* 处理中遮罩 */}
      {processMutation.isPending && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <div className="bg-surface rounded-xl border border-border p-6 shadow-2xl flex items-center gap-3">
            <Loader2 className="w-5 h-5 animate-spin text-primary" />
            <span className="text-text-primary font-medium">正在处理告警...</span>
          </div>
        </div>
      )}

      {automationLogAlert && (
        <AutomationLogPanel
          alert={automationLogAlert}
          onClose={() => setAutomationLogAlert(null)}
          logs={automationLogs}
          loading={automationLogsLoading}
        />
      )}
    </div>
  );
}