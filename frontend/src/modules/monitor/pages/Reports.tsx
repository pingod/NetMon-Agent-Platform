import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { FileText, Plus, Download, Clock, Trash2, Edit2, Eye, X, FolderArchive, RefreshCw, Sparkles, CheckCircle2, Search } from 'lucide-react';
import { useToast } from '@/contexts/ToastContext';
import { logger } from '@/lib/logger';
import api from '../../../lib/api';
import MarkdownOutput from '../../../shared/components/MarkdownOutput';

interface ReportTemplate {
  id: string;
  name: string;
  description: string;
  type: string;
  content: string;
  variables: string[];
  is_preset?: boolean;
}

interface GeneratedReport {
  id: string;
  name: string;
  type: string;
  format: string;
  created_at: string;
  content: string;
  metadata?: string;
}

interface ScheduledReport {
  id: string;
  name: string;
  format: string;
  enabled: boolean;
  cron_expression: string;
  last_generated?: string;
  recipients?: string[];
}

interface AlertTrendItem {
  date: string;
  severity: string;
  count: number;
}

interface DiagnosisItem {
  summary: string;
  count: number;
}

interface _ReportAnalytics {
  analysisStats?: { total?: number; completed?: number; failed?: number };
  remediationStats?: { total?: number; success_count?: number; failed_count?: number; rolled_back?: number };
  alertTrends?: AlertTrendItem[];
  topDiagnoses?: DiagnosisItem[];
}

export default function Reports() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const [activeTab, setActiveTab] = useState<'templates' | 'reports' | 'scheduled' | 'analytics'>('reports');
  const [showCreateTemplateModal, setShowCreateTemplateModal] = useState(false);
  const [showGenerateReportModal, setShowGenerateReportModal] = useState(false);
  const [showViewReportModal, setShowViewReportModal] = useState(false);
  const [selectedReport, setSelectedReport] = useState<GeneratedReport | null>(null);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('');
  const [formData, setFormData] = useState<Record<string, string>>({});
  const [isSyncing, setIsSyncing] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [templateForm, setTemplateForm] = useState({
    name: '',
    description: '',
    type: 'incident' as 'incident' | 'inspection' | 'change',
    content: '',
    variables: [] as string[]
  });

  const { data: templates } = useQuery({
    queryKey: ['reportTemplates'],
    queryFn: async () => {
      const res = await api.get('/reports/templates');
      const d = res.data?.data ?? res.data;
      return Array.isArray(d) ? d : [];
    }
  });

  const { data: rawReports, isLoading: isReportsLoading } = useQuery({
    queryKey: ['reports'],
    queryFn: async () => {
      const res = await api.get('/reports');
      const d = res.data?.data ?? res.data;
      return Array.isArray(d) ? d : [];
    }
  });
  const reports = (Array.isArray(rawReports) ? rawReports : []) as GeneratedReport[];

  const { data: analytics } = useQuery({
    queryKey: ['reportAnalytics'],
    queryFn: async () => {
      const res = await api.get('/reports/analytics');
      return res.data?.data ?? res.data;
    },
    staleTime: 120000,
  });

  const { data: scheduledReports } = useQuery({
    queryKey: ['scheduledReports'],
    queryFn: async () => {
      const res = await api.get('/reports/scheduled/all');
      const d = res.data?.data ?? res.data;
      return Array.isArray(d) ? d : [];
    }
  });

  const handleSyncReports = async () => {
    setIsSyncing(true);
    try {
      const res = await api.post('/reports/sync-inspection-reports');
      const data = res.data?.data || res.data;
      toast.success(data?.message || '已成功同步全网设备巡检报告');
      await queryClient.invalidateQueries({ queryKey: ['reports'] });
    } catch (err: any) {
      toast.error('同步失败: ' + (err.response?.data?.error || err.message || '网络错误'));
    } finally {
      setIsSyncing(false);
    }
  };

  const deleteReportMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/reports/${id}`);
    },
    onSuccess: () => {
      toast.success('报告已删除');
      queryClient.invalidateQueries({ queryKey: ['reports'] });
    },
    onError: (err: any) => {
      toast.error('删除报告失败: ' + (err.response?.data?.error || err.message));
    }
  });

  const createTemplateMutation = useMutation({
    mutationFn: async (template: Omit<ReportTemplate, 'id'>) => {
      const res = await api.post('/reports/templates', template);
      return res.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reportTemplates'] });
      setShowCreateTemplateModal(false);
      setTemplateForm({
        name: '',
        description: '',
        type: 'incident',
        content: '',
        variables: []
      });
    }
  });

  const generateReportMutation = useMutation({
    mutationFn: async ({ templateId, variables }: { templateId: string; variables: Record<string, string> }) => {
      const res = await api.post('/reports/generate', { templateId, variables, format: 'markdown' });
      return res.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reports'] });
      setShowGenerateReportModal(false);
      setFormData({});
    }
  });

  const deleteTemplateMutation = useMutation({
    mutationFn: async (id: string) => {
      await api.delete(`/reports/templates/${id}`);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['reportTemplates'] });
    }
  });

  const handleGenerateReport = (templateId: string) => {
    const template = (templates as ReportTemplate[])?.find((t) => t.id === templateId);
    if (template) {
      const initialData: Record<string, string> = {};
      template.variables?.forEach((v: string) => {
        initialData[v] = '';
      });
      setFormData(initialData);
      setSelectedTemplateId(templateId);
      setShowGenerateReportModal(true);
    }
  };

  const handleSubmitGenerate = () => {
    generateReportMutation.mutate({
      templateId: selectedTemplateId,
      variables: formData
    });
  };

  const handleViewReport = (report: GeneratedReport) => {
    setSelectedReport(report);
    setShowViewReportModal(true);
  };

  const handleDownloadReport = async (reportId: string, format: 'markdown' | 'pdf' | 'word' | 'zip' = 'markdown') => {
    try {
      const response = await api.get(`/reports/${reportId}/export?format=${format}`, { responseType: 'blob' });
      const blob = response.data;
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      const fileExtension = format === 'pdf' ? 'pdf' : format === 'word' ? 'doc' : format === 'zip' ? 'zip' : 'md';
      a.download = `report-${reportId}.${fileExtension}`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
      toast.success(format === 'zip' ? '已开始下载 ZIP 压缩包（内含每台设备独立巡检文件）' : '报告下载完成');
    } catch (error) {
      logger.error('Download failed:', error);
      toast.error('下载失败: ' + ((error as any)?.message || '网络错误'));
    }
  };

  const typeLabels: Record<string, string> = {
    incident: '故障报告',
    inspection: '巡检报告',
    change: '变更记录'
  };

  const typeColors: Record<string, string> = {
    incident: 'text-red-400 bg-red-900/30',
    inspection: 'text-blue-400 bg-blue-900/30',
    change: 'text-green-400 bg-green-900/30'
  };

  return (
    <div className="h-full overflow-auto p-6">
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-bold text-text-primary flex items-center gap-3">
              <FileText className="w-8 h-8 text-primary" />
              报告管理
            </h1>
            <p className="text-text-secondary mt-1">管理报告模板、生成报告和定时报告</p>
          </div>
          {activeTab === 'templates' && (
            <button
              onClick={() => setShowCreateTemplateModal(true)}
              className="bg-primary hover:bg-primary/90 text-white px-4 py-2 rounded-lg flex items-center gap-2"
            >
              <Plus className="w-4 h-4" />
              创建模板
            </button>
          )}
        </div>

        <div className="flex gap-2 border-b border-border">
          {(['templates', 'reports', 'scheduled', 'analytics'] as const).map(tab => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`px-4 py-2 border-b-2 transition-colors ${
                activeTab === tab
                  ? 'border-primary text-primary'
                  : 'border-transparent text-text-secondary hover:text-text-primary'
              }`}
            >
              {tab === 'templates' ? '报告模板' : tab === 'reports' ? '已生成报告' : tab === 'scheduled' ? '定时报告' : '数据分析'}
            </button>
          ))}
        </div>

        {activeTab === 'templates' && (
          <div className="grid gap-4">
            {(templates as ReportTemplate[])?.map((template) => (
              <div key={template.id} className="bg-surface border border-border rounded-lg p-4">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <span className={`px-2 py-1 text-xs rounded ${typeColors[template.type] || 'text-text-secondary bg-background'}`}>
                        {typeLabels[template.type] || template.type}
                      </span>
                      {template.is_preset && (
                        <span className="px-2 py-1 text-xs bg-purple-900/30 text-purple-400 rounded">
                          预设
                        </span>
                      )}
                    </div>
                    <h3 className="text-lg font-semibold text-text-primary mb-1">{template.name}</h3>
                    <p className="text-text-secondary text-sm mb-2">{template.description}</p>
                    <div className="flex flex-wrap gap-2">
                      {template.variables?.map((v: string, i: number) => (
                        <span key={i} className="px-2 py-1 text-xs bg-background text-text-secondary rounded">
                          {v}
                        </span>
                      ))}
                    </div>
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={() => handleGenerateReport(template.id)}
                      className="text-primary hover:text-primary/80 p-2"
                      title="生成报告"
                    >
                      <Plus className="w-5 h-5" />
                    </button>
                    {!template.is_preset && (
                      <>
                        <button className="text-text-secondary hover:text-text-primary p-2" title="编辑">
                          <Edit2 className="w-5 h-5" />
                        </button>
                        <button
                          onClick={() => deleteTemplateMutation.mutate(template.id)}
                          className="text-red-400 hover:text-red-300 p-2"
                          title="删除"
                        >
                          <Trash2 className="w-5 h-5" />
                        </button>
                      </>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}

        {activeTab === 'reports' && (
          <div className="space-y-4">
            {/* 工具与筛选栏 */}
            <div className="flex flex-wrap items-center justify-between gap-3 bg-surface/50 border border-border/40 p-3 rounded-xl">
              <div className="flex items-center gap-2 flex-1 min-w-[240px] max-w-md">
                <div className="relative w-full">
                  <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-text-secondary" />
                  <input
                    type="text"
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                    placeholder="搜索巡检报告名称、关键词..."
                    className="w-full pl-9 pr-3 py-1.5 bg-background border border-border rounded-lg text-xs text-text-primary placeholder:text-text-secondary focus:outline-none focus:border-primary"
                  />
                </div>
                {searchTerm && (
                  <button
                    onClick={() => setSearchTerm('')}
                    className="text-xs text-text-secondary hover:text-text-primary px-2 py-1 bg-surface rounded-lg"
                  >
                    清空
                  </button>
                )}
              </div>

              <div className="flex items-center gap-2">
                <button
                  onClick={handleSyncReports}
                  disabled={isSyncing}
                  className="flex items-center gap-2 px-3.5 py-1.5 bg-gradient-to-r from-blue-600 to-indigo-600 hover:from-blue-500 hover:to-indigo-500 text-white rounded-lg text-xs font-semibold shadow-md shadow-blue-500/20 transition-all disabled:opacity-50"
                  title="扫描全网网络设备与服务器，聚合生成最新巡检报告"
                >
                  <RefreshCw className={`w-3.5 h-3.5 ${isSyncing ? 'animate-spin' : ''}`} />
                  <span>{isSyncing ? '正在生成全网巡检报告...' : '一键同步生成全网设备巡检报告'}</span>
                </button>
              </div>
            </div>

            {/* 报告列表展示 */}
            {isReportsLoading ? (
              <div className="py-12 text-center text-text-secondary text-sm flex flex-col items-center justify-center gap-2">
                <RefreshCw className="w-6 h-6 animate-spin text-primary" />
                <span>正在加载巡检报告列表...</span>
              </div>
            ) : reports.filter(r => !searchTerm || r.name.toLowerCase().includes(searchTerm.toLowerCase())).length === 0 ? (
              <div className="bg-surface/40 border border-dashed border-border rounded-2xl p-12 text-center flex flex-col items-center justify-center space-y-4">
                <div className="p-4 bg-primary/10 rounded-2xl border border-primary/20 text-primary">
                  <FolderArchive className="w-10 h-10" />
                </div>
                <div className="max-w-md space-y-1">
                  <h3 className="text-base font-bold text-text-primary">暂无生成的巡检报告</h3>
                  <p className="text-xs text-text-secondary">
                    巡检完网络设备或服务器后会自动在此生成报告。您也可以点击下方按钮，立即对全网设备进行汇总并生成包含每台设备独立巡检文件的 ZIP 压缩包。
                  </p>
                </div>
                <button
                  onClick={handleSyncReports}
                  disabled={isSyncing}
                  className="flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white rounded-xl text-xs font-semibold shadow-lg shadow-blue-500/20 transition-all disabled:opacity-50"
                >
                  <Sparkles className="w-4 h-4" />
                  <span>{isSyncing ? '正在生成中...' : '立即同步生成全网设备巡检报告'}</span>
                </button>
              </div>
            ) : (
              <div className="grid gap-3">
                {reports
                  .filter(r => !searchTerm || r.name.toLowerCase().includes(searchTerm.toLowerCase()))
                  .map((report) => {
                    let meta: any = null;
                    try {
                      if (report.metadata) {
                        meta = typeof report.metadata === 'string' ? JSON.parse(report.metadata) : report.metadata;
                      }
                    } catch (e) {}

                    return (
                      <div key={report.id} className="bg-surface border border-border/80 hover:border-primary/40 rounded-xl p-4 transition-all shadow-sm">
                        <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                          <div className="flex-1 min-w-0">
                            <div className="flex flex-wrap items-center gap-2 mb-1.5">
                              <span className={`px-2 py-0.5 text-xs font-medium rounded-md ${typeColors[report.type] || 'text-text-secondary bg-background'}`}>
                                {typeLabels[report.type] || report.type}
                              </span>
                              <span className="px-2 py-0.5 text-xs bg-purple-500/10 text-purple-400 font-medium rounded-md border border-purple-500/20">
                                ZIP 压缩包支持
                              </span>
                              {meta?.total && (
                                <span className="px-2 py-0.5 text-xs bg-emerald-500/10 text-emerald-400 font-medium rounded-md border border-emerald-500/20">
                                  包含 {meta.total} 台设备独立报告
                                </span>
                              )}
                              <span className="text-xs text-text-secondary">
                                {report.format.toUpperCase()}
                              </span>
                            </div>
                            <h3 className="text-base font-bold text-text-primary truncate mb-1">{report.name}</h3>
                            <p className="text-text-secondary text-xs flex items-center gap-1.5">
                              <Clock className="w-3.5 h-3.5" />
                              生成时间: {new Date(report.created_at).toLocaleString()}
                            </p>
                          </div>

                          <div className="flex flex-wrap items-center gap-2 shrink-0">
                            <button
                              onClick={() => handleDownloadReport(report.id, 'zip')}
                              className="flex items-center gap-1.5 px-3.5 py-1.5 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white rounded-lg text-xs font-semibold shadow-md shadow-blue-500/20 transition-all hover:scale-[1.02]"
                              title="下载包含每台设备独立巡检文件的 ZIP 压缩包（有10台设备就有10个文件）"
                            >
                              <FolderArchive className="w-4 h-4" />
                              <span>下载 ZIP 压缩包 (独立报告)</span>
                            </button>
                            <button
                              onClick={() => handleViewReport(report)}
                              className="flex items-center gap-1 px-3 py-1.5 text-xs font-medium text-sky-400 hover:text-sky-300 bg-sky-500/10 hover:bg-sky-500/20 border border-sky-500/20 rounded-lg transition-colors"
                              title="在线预览巡检报告"
                            >
                              <Eye className="w-3.5 h-3.5" />
                              <span>预览</span>
                            </button>
                            <button
                              onClick={() => handleDownloadReport(report.id, 'markdown')}
                              className="p-1.5 text-text-secondary hover:text-text-primary bg-background hover:bg-surface border border-border rounded-lg transition-colors"
                              title="下载 Markdown 文件"
                            >
                              <FileText className="w-4 h-4" />
                            </button>
                            <button
                              onClick={() => {
                                if (window.confirm(`确定要删除报告「${report.name}」吗？`)) {
                                  deleteReportMutation.mutate(report.id);
                                }
                              }}
                              className="p-1.5 text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 rounded-lg border border-transparent hover:border-rose-500/20 transition-colors"
                              title="删除此报告"
                            >
                              <Trash2 className="w-4 h-4" />
                            </button>
                          </div>
                        </div>
                      </div>
                    );
                  })}
              </div>
            )}
          </div>
        )}

        {activeTab === 'scheduled' && (
          <div className="grid gap-4">
            {(scheduledReports as ScheduledReport[])?.map((report) => (
              <div key={report.id} className="bg-surface border border-border rounded-lg p-4">
                <div className="flex items-start justify-between">
                  <div className="flex-1">
                    <div className="flex items-center gap-3 mb-2">
                      <span className={`px-2 py-1 text-xs rounded ${
                        report.enabled ? 'text-green-400 bg-green-900/30' : 'text-text-secondary bg-background'
                      }`}>
                        {report.enabled ? '已启用' : '已禁用'}
                      </span>
                      <span className="px-2 py-1 text-xs bg-background text-text-secondary rounded">
                        {report.format}
                      </span>
                    </div>
                    <h3 className="text-lg font-semibold text-text-primary mb-1">{report.name}</h3>
                    <div className="flex items-center gap-4 text-sm text-text-secondary">
                      <span className="flex items-center gap-1">
                        <Clock className="w-4 h-4" />
                        {report.cron_expression}
                      </span>
                      {report.last_generated && (
                        <span>
                          最后生成: {new Date(report.last_generated).toLocaleString()}
                        </span>
                      )}
                      <span>
                        接收人: {report.recipients?.join(', ') || '无'}
                      </span>
                    </div>
                  </div>
                  <button className="text-primary hover:text-primary/80 p-2" title="编辑">
                    <Edit2 className="w-5 h-5" />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}

        {activeTab === 'analytics' && (
          <div className="space-y-6">
            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="bg-surface border border-border rounded-lg p-4">
                <h3 className="text-sm font-medium text-text-secondary mb-1">AI 分析统计</h3>
                <p className="text-2xl font-bold text-text-primary">{analytics?.analysisStats?.total ?? '-'}</p>
                <div className="flex gap-3 mt-2 text-xs">
                  <span className="text-green-400">成功 {analytics?.analysisStats?.completed ?? 0}</span>
                  <span className="text-red-400">失败 {analytics?.analysisStats?.failed ?? 0}</span>
                </div>
              </div>
              <div className="bg-surface border border-border rounded-lg p-4">
                <h3 className="text-sm font-medium text-text-secondary mb-1">最近30天修复执行</h3>
                <p className="text-2xl font-bold text-text-primary">{analytics?.remediationStats?.total ?? '-'}</p>
                <div className="flex gap-3 mt-2 text-xs">
                  <span className="text-green-400">成功 {analytics?.remediationStats?.success_count ?? 0}</span>
                  <span className="text-red-400">失败 {analytics?.remediationStats?.failed_count ?? 0}</span>
                  <span className="text-yellow-400">回滚 {analytics?.remediationStats?.rolled_back ?? 0}</span>
                </div>
              </div>
              <div className="bg-surface border border-border rounded-lg p-4">
                <h3 className="text-sm font-medium text-text-secondary mb-1">告警趋势（7天）</h3>
                <p className="text-2xl font-bold text-text-primary">
                  {analytics?.alertTrends?.reduce?.((s: number, r: AlertTrendItem) => s + r.count, 0) ?? '-'}
                </p>
                <div className="text-xs text-text-secondary mt-2">总告警数</div>
              </div>
            </div>

            <div className="bg-surface border border-border rounded-lg p-4">
              <h3 className="text-lg font-semibold text-text-primary mb-4">告警趋势</h3>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border">
                      <th className="text-left py-2 text-text-secondary">日期</th>
                      <th className="text-left py-2 text-text-secondary">严重级别</th>
                      <th className="text-right py-2 text-text-secondary">数量</th>
                    </tr>
                  </thead>
                  <tbody>
                    {analytics?.alertTrends?.map((row: AlertTrendItem, i: number) => (
                      <tr key={i} className="border-b border-border/50">
                        <td className="py-2 text-text-primary">{row.date}</td>
                        <td className="py-2">
                          <span className={`px-2 py-0.5 text-xs rounded ${
                            row.severity === 'critical' ? 'bg-red-900/30 text-red-400' :
                            row.severity === 'high' ? 'bg-orange-900/30 text-orange-400' :
                            'bg-blue-900/30 text-blue-400'
                          }`}>{row.severity}</span>
                        </td>
                        <td className="py-2 text-right text-text-primary">{row.count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="bg-surface border border-border rounded-lg p-4">
              <h3 className="text-lg font-semibold text-text-primary mb-4">热点分析摘要</h3>
              <div className="flex flex-wrap gap-2">
                {analytics?.topDiagnoses?.length ? (
                  analytics.topDiagnoses.map((d: DiagnosisItem, i: number) => (
                    <span key={i} className="px-3 py-1 bg-background border border-border rounded-full text-sm text-text-primary">
                      {d.summary} ({d.count}次)
                    </span>
                  ))
                ) : (
                  <p className="text-text-secondary">暂无分析记录</p>
                )}
              </div>
            </div>
          </div>
        )}

        {showCreateTemplateModal && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
            <div className="bg-surface border border-border rounded-lg w-full max-w-2xl max-h-[90vh] overflow-y-auto">
              <div className="p-6">
                <h2 className="text-xl font-bold text-text-primary mb-4">创建报告模板</h2>
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm text-text-primary mb-1">模板名称</label>
                    <input
                      type="text"
                      value={templateForm.name}
                      onChange={(e) => setTemplateForm({ ...templateForm, name: e.target.value })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                      placeholder="输入模板名称"
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-primary mb-1">描述</label>
                    <input
                      type="text"
                      value={templateForm.description}
                      onChange={(e) => setTemplateForm({ ...templateForm, description: e.target.value })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                      placeholder="输入模板描述"
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-primary mb-1">报告类型</label>
                    <select
                      value={templateForm.type}
                      onChange={(e) => setTemplateForm({ ...templateForm, type: e.target.value as 'incident' | 'inspection' | 'change' })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                    >
                      <option value="incident">故障报告</option>
                      <option value="inspection">巡检报告</option>
                      <option value="change">变更记录</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-sm text-text-primary mb-1">
                      模板内容 (使用 {'{{variable}}'} 定义变量)
                    </label>
                    <textarea
                      value={templateForm.content}
                      onChange={(e) => setTemplateForm({ ...templateForm, content: e.target.value })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary font-mono text-sm"
                      rows={10}
                      placeholder="输入模板内容..."
                    />
                  </div>
                  <div>
                    <label className="block text-sm text-text-primary mb-1">
                      变量列表 (每行一个)
                    </label>
                    <textarea
                      value={templateForm.variables?.join('\n') || ''}
                      onChange={(e) => setTemplateForm({
                        ...templateForm,
                        variables: e.target.value.split('\n').filter(v => v.trim())
                      })}
                      className="w-full bg-background border border-border rounded-lg p-2 text-text-primary font-mono text-sm"
                      rows={4}
                      placeholder="variable1&#10;variable2&#10;..."
                    />
                  </div>
                </div>
                <div className="flex gap-3 mt-6">
                  <button
                    onClick={() => setShowCreateTemplateModal(false)}
                    className="flex-1 bg-background hover:bg-surface text-text-primary py-2 rounded-lg"
                  >
                    取消
                  </button>
                  <button
                    onClick={() => createTemplateMutation.mutate(templateForm)}
                    className="flex-1 bg-primary hover:bg-primary/90 text-white py-2 rounded-lg"
                  >
                    创建
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {showGenerateReportModal && selectedTemplateId && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
            <div className="bg-surface border border-border rounded-lg w-full max-w-2xl max-h-[90vh] overflow-y-auto">
              <div className="p-6">
                <h2 className="text-xl font-bold text-text-primary mb-4">生成报告</h2>
                <div className="space-y-4">
                  {Object.keys(formData).map(key => (
                    <div key={key}>
                      <label className="block text-sm text-text-primary mb-1">{key}</label>
                      <input
                        type="text"
                        value={formData[key]}
                        onChange={(e) => setFormData({ ...formData, [key]: e.target.value })}
                        className="w-full bg-background border border-border rounded-lg p-2 text-text-primary"
                        placeholder={`请输入 ${key}`}
                      />
                    </div>
                  ))}
                </div>
                <div className="flex gap-3 mt-6">
                  <button
                    onClick={() => setShowGenerateReportModal(false)}
                    className="flex-1 bg-background hover:bg-surface text-text-primary py-2 rounded-lg"
                  >
                    取消
                  </button>
                  <button
                    onClick={handleSubmitGenerate}
                    disabled={generateReportMutation.isPending}
                    className="flex-1 bg-primary hover:bg-primary/90 disabled:opacity-50 text-white py-2 rounded-lg"
                  >
                    {generateReportMutation.isPending ? '生成中...' : '生成'}
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {showViewReportModal && selectedReport && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
            <div className="bg-surface border border-border rounded-lg w-full max-w-4xl max-h-[90vh] overflow-hidden flex flex-col">
              <div className="flex items-center justify-between p-4 border-b border-border">
                <h2 className="text-xl font-bold text-text-primary">{selectedReport.name}</h2>
                <button
                  onClick={() => setShowViewReportModal(false)}
                  className="p-2 hover:bg-background rounded-lg text-text-secondary hover:text-text-primary"
                >
                  <X className="w-5 h-5" />
                </button>
              </div>
              <div className="flex-1 overflow-y-auto p-6">
                <div className="mb-4 flex items-center gap-2">
                  <span className={`px-2 py-1 text-xs rounded ${typeColors[selectedReport.type] || 'text-text-secondary bg-background'}`}>
                    {typeLabels[selectedReport.type] || selectedReport.type}
                  </span>
                  <span className="text-text-secondary text-sm">
                    创建时间: {new Date(selectedReport.created_at).toLocaleString()}
                  </span>
                </div>
                <div className="prose prose-invert max-w-none">
                  <MarkdownOutput content={selectedReport.content || '无内容'} />
                </div>
              </div>
              <div className="p-4 border-t border-border flex justify-end gap-3">
                <button
                  onClick={() => setShowViewReportModal(false)}
                  className="px-4 py-2 bg-background hover:bg-surface text-text-primary rounded-lg border border-border"
                >
                  关闭
                </button>
                <button
                  onClick={() => handleDownloadReport(selectedReport.id, 'zip')}
                  className="px-4 py-2 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white rounded-lg font-medium shadow-md shadow-blue-500/20 flex items-center gap-1.5"
                >
                  <FolderArchive className="w-4 h-4" />
                  下载 ZIP 压缩包 (各设备独立文件)
                </button>
                <button
                  onClick={() => handleDownloadReport(selectedReport.id, 'markdown')}
                  className="px-4 py-2 bg-primary hover:bg-primary/90 text-white rounded-lg flex items-center gap-1.5"
                >
                  <FileText className="w-4 h-4" />
                  下载 Markdown
                </button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
