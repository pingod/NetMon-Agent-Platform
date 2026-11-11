import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import { Modal, Input, message } from 'antd';
import { Search, Zap, CheckCircle, XCircle, Eye, Play, Flag, Clock, TrendingUp, AlertTriangle, BarChart3 } from 'lucide-react';
import clsx from 'clsx';
import api from '../../../lib/api';

interface RCAItem {
  id: string;
  alert_id?: string;
  title: string;
  description?: string;
  status: 'pending' | 'analyzing' | 'completed' | 'failed';
  root_cause?: string;
  symptoms?: string[];
  timeline?: Array<{ time: string; event: string }>;
  evidence?: string[];
  recommendations?: string[];
  false_positive?: boolean;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

interface RCAStats {
  todayCount: number;
  avgConfidence: number;
  autoRemediations: number;
  falsePositives: number;
  totalCompleted: number;
}

export default function AIRootCause() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [analyzeModalOpen, setAnalyzeModalOpen] = useState(false);
  const [analyzeAlertId, setAnalyzeAlertId] = useState('');

  const analyzeMutation = useMutation({
    mutationFn: async (alertId: string) => {
      await api.post(`/root-cause-analysis/auto-analyze/${alertId}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['root-cause-analysis'] });
      message.success('已触发根因分析');
      setAnalyzeModalOpen(false);
      setAnalyzeAlertId('');
    },
    onError: (e: any) => {
      const detail = e?.response?.data;
      message.error('触发失败：' + (detail?.message || detail?.error || e?.message || '未知错误'));
    },
  });

  const handleNewAnalysis = () => {
    setAnalyzeAlertId('');
    setAnalyzeModalOpen(true);
  };

  const remediateMutation = useMutation({
    mutationFn: async (id: string) => {
      const res = await api.post(`/root-cause-analysis/${id}/remediate`);
      return res.data.data as { outcomes: Array<{ policy_name: string; result: string; reason?: string }> };
    },
    onSuccess: (data) => {
      const parts = (data?.outcomes || []).map(o => {
        if (o.result === 'created') return `「${o.policy_name}」已创建执行`;
        if (o.result === 'already_exists') return `「${o.policy_name}」已有执行记录`;
        return `「${o.policy_name}」被限流：${o.reason || ''}`;
      });
      message.success(parts.join('；') || '已触发自愈匹配');
      queryClient.invalidateQueries({ queryKey: ['root-cause-analysis'] });
    },
    onError: (e: any) => {
      const detail = e?.response?.data;
      message.error(detail?.error || detail?.message || '触发修复失败');
    },
  });

  const falsePositiveMutation = useMutation({
    mutationFn: async ({ id, target }: { id: string; target: boolean }) => {
      await api.post(`/root-cause-analysis/${id}/false-positive`, { false_positive: target });
    },
    onSuccess: (_d, vars) => {
      message.success(vars.target ? '已标记为误报' : '已取消误报标记');
      queryClient.invalidateQueries({ queryKey: ['root-cause-analysis'] });
    },
    onError: (e: any) => {
      const detail = e?.response?.data;
      message.error(detail?.error || detail?.message || '操作失败');
    },
  });

  const confirmRemediate = (item: RCAItem) => {
    Modal.confirm({
      title: '执行修复',
      content: `将对关联告警触发自愈策略匹配（按策略配置可能需要审批）。确定继续？`,
      okText: '触发修复',
      cancelText: '取消',
      onOk: () => remediateMutation.mutateAsync(item.id).catch(() => {}),
    });
  };

  const { data: rcaItems, isLoading } = useQuery({
    queryKey: ['root-cause-analysis'],
    queryFn: async () => {
      const res = await api.get('/root-cause-analysis');
      return res.data.data as RCAItem[];
    },
  });

  const { data: stats } = useQuery({
    queryKey: ['root-cause-analysis', 'stats'],
    queryFn: async () => {
      const res = await api.get('/root-cause-analysis/stats');
      return res.data.data as RCAStats;
    },
  });

  const statusLabels: Record<string, string> = {
    pending: '待分析',
    analyzing: '分析中',
    completed: '已完成',
    failed: '分析失败'
  };

  const statusColors: Record<string, string> = {
    pending: 'bg-gray-100 text-gray-700',
    analyzing: 'bg-blue-100 text-blue-700',
    completed: 'bg-green-100 text-green-700',
    failed: 'bg-red-100 text-red-700'
  };

  return (
    <div className="h-full overflow-auto p-6">
      <div className="mb-6">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <Search className="w-7 h-7 text-primary" />
            <div>
              <h1 className="text-2xl font-bold text-text-primary">AI 智能根因分析</h1>
              <p className="text-text-secondary text-sm mt-0.5">基于 AI 的自动化根因定位与分析</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button className="px-3 py-2 text-text-secondary hover:text-text-primary hover:bg-background rounded-lg flex items-center gap-2 text-sm transition-colors">
              <Clock className="w-4 h-4" />
              历史
            </button>
            <button onClick={handleNewAnalysis} className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 flex items-center gap-2 text-sm font-medium transition-colors">
              <Zap className="w-4 h-4" />
              新建分析
            </button>
          </div>
        </div>

        {stats && (
          <div className="grid grid-cols-4 gap-4 mb-6">
            <div className="bg-surface rounded-xl border border-border p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm text-text-secondary">今日分析</span>
                <BarChart3 className="w-5 h-5 text-primary" />
              </div>
              <div className="text-2xl font-bold text-text-primary">{stats.todayCount}</div>
              <div className="text-xs text-text-secondary mt-1">总计完成 {stats.totalCompleted}</div>
            </div>
            <div className="bg-surface rounded-xl border border-border p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm text-text-secondary">平均置信度</span>
                <TrendingUp className="w-5 h-5 text-green-600" />
              </div>
              <div className="text-2xl font-bold text-green-600">{Math.round(stats.avgConfidence * 100)}%</div>
              <div className="text-xs text-text-secondary mt-1">LLM分析准确率</div>
            </div>
            <div className="bg-surface rounded-xl border border-border p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm text-text-secondary">自动修复</span>
                <CheckCircle className="w-5 h-5 text-blue-600" />
              </div>
              <div className="text-2xl font-bold text-text-primary">{stats.autoRemediations}</div>
              <div className="text-xs text-text-secondary mt-1">可自动执行方案</div>
            </div>
            <div className="bg-surface rounded-xl border border-border p-4">
              <div className="flex items-center justify-between mb-2">
                <span className="text-sm text-text-secondary">误报数</span>
                <XCircle className="w-5 h-5 text-red-600" />
              </div>
              <div className="text-2xl font-bold text-text-primary">{stats.falsePositives}</div>
              <div className="text-xs text-text-secondary mt-1">标记为误报</div>
            </div>
          </div>
        )}
      </div>

      {isLoading ? (
        <div className="flex items-center justify-center py-20">
          <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
      ) : (
        <div className="grid gap-4">
          {rcaItems?.map((item) => (
            <div key={item.id} className="bg-surface rounded-xl border border-border p-5">
              <div className="flex items-start justify-between">
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-3 mb-2">
                    <h3 className="text-base font-semibold text-text-primary truncate">{item.title}</h3>
                    <span className={clsx(
                      'px-2.5 py-0.5 rounded-full text-xs font-medium',
                      statusColors[item.status] || 'bg-gray-100 text-gray-700'
                    )}>
                      {statusLabels[item.status]}
                    </span>
                    {item.false_positive && (
                      <span className="px-2.5 py-0.5 rounded-full text-xs font-medium bg-orange-100 text-orange-700">
                        误报
                      </span>
                    )}
                  </div>

                  <p className="text-text-secondary text-sm mb-3 line-clamp-2">{item.root_cause || item.description || '暂无分析结果'}</p>

                  <div className="flex items-center gap-4 text-sm">
                    <div className="flex items-center gap-2 text-text-secondary">
                      <AlertTriangle className="w-4 h-4" />
                      <span>症状: {(item.symptoms || []).length} 个</span>
                    </div>
                    <div className="flex items-center gap-2 text-text-secondary">
                      <BarChart3 className="w-4 h-4" />
                      <span>建议: {(item.recommendations || []).length} 个</span>
                    </div>
                    <div className="text-text-secondary">
                      {new Date(item.created_at).toLocaleString('zh-CN')}
                    </div>
                  </div>
                </div>

                <div className="flex items-center gap-2 ml-4">
                  <button
                    onClick={() => navigate(`/ai-root-cause/${item.id}`)}
                    className="p-2 text-text-secondary hover:text-text-primary hover:bg-background rounded-lg transition-colors"
                    title="查看详情"
                  >
                    <Eye className="w-4 h-4" />
                  </button>
                  {item.status === 'completed' && !item.false_positive && item.alert_id && (
                    <button
                      onClick={() => confirmRemediate(item)}
                      disabled={remediateMutation.isPending}
                      className="p-2 text-green-600 hover:bg-green-50 rounded-lg transition-colors disabled:opacity-50"
                      title="执行修复（触发自愈策略匹配）"
                    >
                      <Play className="w-4 h-4" />
                    </button>
                  )}
                  <button
                    onClick={() => falsePositiveMutation.mutate({ id: item.id, target: !item.false_positive })}
                    disabled={falsePositiveMutation.isPending}
                    className={clsx(
                      'p-2 rounded-lg transition-colors disabled:opacity-50',
                      item.false_positive ? 'text-orange-600 hover:bg-orange-50' : 'text-red-600 hover:bg-red-50'
                    )}
                    title={item.false_positive ? '取消误报标记' : '标记误报'}
                  >
                    <Flag className="w-4 h-4" />
                  </button>
                </div>
              </div>
            </div>
          ))}

          {rcaItems?.length === 0 && (
            <div className="text-center py-20 bg-surface rounded-xl border border-border">
              <Search className="w-16 h-16 text-text-secondary mx-auto mb-4" />
              <h3 className="text-lg font-medium text-text-primary mb-2">暂无根因分析记录</h3>
              <p className="text-text-secondary mb-6">创建一个新的 AI 根因分析来开始</p>
              <button onClick={handleNewAnalysis} className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90">
                创建第一个分析
              </button>
            </div>
          )}
        </div>
      )}

      <Modal
        title="新建根因分析"
        open={analyzeModalOpen}
        okText="开始分析"
        cancelText="取消"
        confirmLoading={analyzeMutation.isPending}
        okButtonProps={{ disabled: !analyzeAlertId.trim() }}
        onOk={() => analyzeMutation.mutate(analyzeAlertId.trim())}
        onCancel={() => setAnalyzeModalOpen(false)}
      >
        <p className="text-text-secondary text-sm mb-3">输入告警 ID，系统将自动采集相关指标与日志并进行根因分析。</p>
        <Input
          placeholder="告警 ID"
          value={analyzeAlertId}
          onChange={(e) => setAnalyzeAlertId(e.target.value)}
          onPressEnter={() => { if (analyzeAlertId.trim()) analyzeMutation.mutate(analyzeAlertId.trim()); }}
          autoFocus
        />
      </Modal>
    </div>
  );
}
