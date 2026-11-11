/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useEffect } from 'react';
import { useSearchParams, useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { Bell, Database, Shield, Loader2, CheckCircle2, AlertCircle, BookOpen, Upload, FileText, Globe, Wifi, Brain, Activity, Server, Palette } from 'lucide-react';
import clsx from 'clsx';
import { useTranslation } from 'react-i18next';
import api from '../../../lib/api';
import { getAxiosErrorMessage } from '../../../lib/errorHandler';
import { useToast } from '../../../contexts/ToastContext';
import ModelSettings from './settings/ModelSettings';
import NotificationSettings from '../../notification/pages/NotificationSettings';
import BackupSettings from '../../backup/pages/BackupSettings';
import SecuritySettings from './settings/SecuritySettings';
import GeneralSettings from './settings/GeneralSettings';
import AppearanceSettings from './settings/AppearanceSettings';
import DatabaseModeSettings from './settings/DatabaseModeSettings';

export default function Settings() {
  const { t } = useTranslation();
  const [searchParams] = useSearchParams();
  const [activeTab, setActiveTab] = useState('models');
  const queryClient = useQueryClient();

  // 如果是强制修改密码，自动切换到安全设置标签
  useEffect(() => {
    if (searchParams.get('changePassword') === 'true') {
      setActiveTab('security');
    }
  }, [searchParams]);

  // ========== QAnything 配置（保留在主入口） ==========

  const [qanythingConfig, setQanythingConfig] = useState({
    enabled: false,
    apiBase: '',
    apiKey: '',
    kbId: '',
    mode: 'cloud' as 'cloud' | 'local',
    topK: 5,
  });
  const [qanythingSaveStatus, setQanythingSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle');
  const [qanythingTestStatus, setQanythingTestStatus] = useState<'idle' | 'testing' | 'success' | 'error'>('idle');
  const [qanythingTestMessage, setQanythingTestMessage] = useState('');

  // 文档上传状态
  const [uploadFiles, setUploadFiles] = useState<File[]>([]);
  const [uploadStatus, setUploadStatus] = useState<'idle' | 'uploading' | 'success' | 'error'>('idle');
  const [uploadMessage, setUploadMessage] = useState('');
  const [isDragOver, setIsDragOver] = useState(false);

  useQuery({
    queryKey: ['qanythingConfig'],
    queryFn: async () => {
      const res = await api.get('/knowledge/qanything/config');
      if (res.data.data) {
        const backendData = res.data.data;
        if (backendData.apiKey?.includes('****')) {
          backendData.apiKey = qanythingConfig.apiKey;
        }
        setQanythingConfig(backendData);
      }
      return res.data.data;
    },
  });

  const qanythingConfigMutation = useMutation({
    mutationFn: async (config: Record<string, unknown>) => {
      const res = await api.post('/knowledge/qanything/config', config);
      return res.data;
    },
    onMutate: () => {
      setQanythingSaveStatus('saving');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['qanythingConfig'] });
      setQanythingSaveStatus('saved');
      setQanythingTestMessage('配置已保存');
      setQanythingTestStatus('success');
      setTimeout(() => setQanythingSaveStatus('idle'), 2000);
      setTimeout(() => setQanythingTestStatus('idle'), 3000);
    },
    onError: (err: unknown) => {
      setQanythingSaveStatus('error');
      setQanythingTestMessage(getAxiosErrorMessage(err, '保存失败'));
      setQanythingTestStatus('error');
      setTimeout(() => setQanythingSaveStatus('idle'), 3000);
      setTimeout(() => setQanythingTestStatus('idle'), 5000);
    },
  });

  const qanythingTestMutation = useMutation({
    mutationFn: async () => {
      const res = await api.post('/knowledge/qanything/test');
      return res.data;
    },
    onMutate: () => {
      setQanythingTestStatus('testing');
      setQanythingTestMessage('');
    },
    onSuccess: (data) => {
      setQanythingTestStatus(data.success ? 'success' : 'error');
      setQanythingTestMessage(data.message);
    },
    onError: (err: unknown) => {
      setQanythingTestStatus('error');
      setQanythingTestMessage(getAxiosErrorMessage(err, '连接失败'));
    },
  });

  const uploadMutation = useMutation({
    mutationFn: async (files: File[]) => {
      const formData = new FormData();
      files.forEach((file) => {
        formData.append('files', file);
      });
      const res = await api.post('/knowledge/qanything/upload-batch', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      return res.data;
    },
    onMutate: () => {
      setUploadStatus('uploading');
      setUploadMessage('');
    },
    onSuccess: (data) => {
      setUploadStatus('success');
      setUploadMessage(`成功上传 ${data.summary?.success || 0} 个文件，失败 ${data.summary?.failed || 0} 个`);
      setUploadFiles([]);
      setTimeout(() => setUploadStatus('idle'), 5000);
    },
    onError: (err: unknown) => {
      setUploadStatus('error');
      setUploadMessage(getAxiosErrorMessage(err, '上传失败'));
      setTimeout(() => setUploadStatus('idle'), 5000);
    },
  });

  const handleTestQAnythingConnection = () => {
    if (!qanythingConfig.apiBase.trim()) {
      setQanythingTestStatus('error');
      setQanythingTestMessage('请先填写 API 地址');
      setTimeout(() => setQanythingTestStatus('idle'), 3000);
      return;
    }
    qanythingTestMutation.mutate();
  };

  const handleUploadDocuments = () => {
    if (!qanythingConfig.enabled) {
      setUploadStatus('error');
      setUploadMessage('请先启用 QAnything 知识库并保存配置');
      setTimeout(() => setUploadStatus('idle'), 5000);
      return;
    }
    if (uploadFiles.length === 0) {
      setUploadStatus('error');
      setUploadMessage('请先选择要上传的文件');
      setTimeout(() => setUploadStatus('idle'), 3000);
      return;
    }
    uploadMutation.mutate(uploadFiles);
  };

  const handleSaveQAnythingConfig = () => {
    if (qanythingConfig.enabled) {
      if (!qanythingConfig.apiBase.trim()) {
        setQanythingSaveStatus('error');
        setQanythingTestMessage('API 地址不能为空');
        setQanythingTestStatus('error');
        setTimeout(() => setQanythingSaveStatus('idle'), 3000);
        setTimeout(() => setQanythingTestStatus('idle'), 3000);
        return;
      }
      if (!qanythingConfig.kbId.trim()) {
        setQanythingSaveStatus('error');
        setQanythingTestMessage('知识库 ID 不能为空');
        setQanythingTestStatus('error');
        setTimeout(() => setQanythingSaveStatus('idle'), 3000);
        setTimeout(() => setQanythingTestStatus('idle'), 3000);
        return;
      }
      if (qanythingConfig.mode === 'cloud' && !qanythingConfig.apiKey.trim()) {
        setQanythingSaveStatus('error');
        setQanythingTestMessage('API Key 不能为空');
        setQanythingTestStatus('error');
        setTimeout(() => setQanythingSaveStatus('idle'), 3000);
        setTimeout(() => setQanythingTestStatus('idle'), 3000);
        return;
      }
    }
    qanythingConfigMutation.mutate(qanythingConfig);
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setIsDragOver(false);
    const files = e.dataTransfer.files;
    if (files && files.length > 0) {
      setUploadFiles((prev) => [...prev, ...Array.from(files)]);
    }
  };

  const removeFile = (index: number) => {
    setUploadFiles((prev) => prev.filter((_, i) => i !== index));
  };

  // ========== Tab 定义 ==========

  const tabs = [
    { id: 'models', name: t('settings.models'), icon: Brain },
    { id: 'appearance', name: t('settings.appearance'), icon: Palette },
    { id: 'qanything', name: t('dashboard.knowledge'), icon: BookOpen },
    { id: 'notifications', name: t('settings.monitoring'), icon: Bell },
    { id: 'database', name: t('settings.backup'), icon: Database },
    { id: 'mysql_storage', name: t('settings.mysqlStorage'), icon: Server },
    { id: 'influxdb', name: t('settings.influxdb'), icon: Activity },
    { id: 'security', name: t('settings.security'), icon: Shield },
  ];

  return (
    <div className="h-full overflow-auto">
      <div className="page-shell space-y-6">
        <div className="page-header">
          <div>
            <div className="page-eyebrow">{t('settings.eyebrow')}</div>
            <h1 className="page-title">{t('settings.title')}</h1>
            <p className="page-subtitle">{t('settings.description')}</p>
          </div>
        </div>

        <div className="grid gap-5 lg:grid-cols-[236px_1fr]">
          <aside className="surface-card h-fit p-2 lg:sticky lg:top-4">
            <nav className="space-y-1">
              {tabs.map((tab) => {
                const Icon = tab.icon;
                return (
                  <button
                    key={tab.id}
                    onClick={() => setActiveTab(tab.id)}
                    className={clsx(
                      'flex w-full items-center gap-2.5 rounded-lg px-3 py-2.5 text-left text-[13px] font-medium transition-all',
                      activeTab === tab.id
                        ? 'bg-primary-soft text-primary shadow-[inset_0_0_0_1px_var(--primary-ring)]'
                        : 'text-text-secondary hover:bg-surface-2 hover:text-text-primary'
                    )}
                  >
                    <Icon className="h-4 w-4 shrink-0" />
                    <span className="truncate">{tab.name}</span>
                  </button>
                );
              })}
            </nav>
          </aside>

          <div className="surface-card min-w-0 p-5 lg:p-6">
              {/* ========== AI 模型配置 ========== */}
              {activeTab === 'models' && <ModelSettings />}

              {/* ========== QAnything 知识库 ========== */}
              {activeTab === 'qanything' && (
                <div className="space-y-6">
                  <div>
                    <h3 className="text-lg font-semibold text-text-primary mb-4 flex items-center gap-2">
                      <BookOpen className="w-5 h-5" />
                      知识库配置 (QAnything)
                    </h3>
                    <p className="text-sm text-text-secondary mb-6">
                      对接 QAnything 知识库，支持 PDF/Word/Excel 等多种格式文档上传，自动解析并用于 Agent 检索增强。
                    </p>
                  </div>

                  {/* 知识库连接配置 */}
                  <div className="bg-background rounded-lg p-6">
                    <h4 className="font-medium text-text-primary mb-4 flex items-center gap-2">
                      <Globe className="w-4 h-4" />
                      连接配置
                    </h4>
                    
                    <div className="space-y-4">
                      {/* 启用开关 */}
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="text-sm font-medium text-text-primary">启用 QAnything 知识库</p>
                          <p className="text-xs text-text-secondary">启用后，Agent 执行时将优先检索 QAnything 知识库</p>
                        </div>
                        <label className="relative inline-flex items-center cursor-pointer">
                          <input
                            type="checkbox"
                            className="sr-only peer"
                            checked={qanythingConfig.enabled}
                            onChange={(e) => setQanythingConfig({...qanythingConfig, enabled: e.target.checked})}
                          />
                          <div className="w-11 h-6 bg-border rounded-full peer peer-checked:bg-primary after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:after:translate-x-full"></div>
                        </label>
                      </div>

                      {/* 部署模式 */}
                      <div>
                        <label className="block text-sm font-medium text-text-secondary mb-2">部署模式</label>
                        <div className="flex gap-3">
                          <button
                            onClick={() => setQanythingConfig({...qanythingConfig, mode: 'cloud'})}
                            className={clsx(
                              'flex-1 px-4 py-2 rounded-lg text-sm font-medium transition-all border',
                              qanythingConfig.mode === 'cloud'
                                ? 'bg-primary text-white border-primary'
                                : 'bg-surface text-text-secondary border-border hover:border-primary/50'
                            )}
                          >
                            ️ 云端 API
                          </button>
                          <button
                            onClick={() => setQanythingConfig({...qanythingConfig, mode: 'local'})}
                            className={clsx(
                              'flex-1 px-4 py-2 rounded-lg text-sm font-medium transition-all border',
                              qanythingConfig.mode === 'local'
                                ? 'bg-primary text-white border-primary'
                                : 'bg-surface text-text-secondary border-border hover:border-primary/50'
                            )}
                          >
                             本地部署
                          </button>
                        </div>
                        <p className="text-xs text-text-secondary mt-2">
                          {qanythingConfig.mode === 'cloud' 
                            ? '使用网易有道云端 QAnything API，需外网访问' 
                            : '本地 Docker 部署，数据不出服务器，更安全'}
                        </p>
                      </div>

                      {/* API 地址 */}
                      <div>
                        <label className="block text-sm font-medium text-text-secondary mb-2">API 地址</label>
                        <input
                          type="text"
                          placeholder={qanythingConfig.mode === 'cloud' ? 'https://openapi.youdao.com/q_anything/api' : 'http://localhost:8777'}
                          value={qanythingConfig.apiBase}
                          onChange={(e) => setQanythingConfig({...qanythingConfig, apiBase: e.target.value})}
                          className="w-full px-4 py-2 bg-surface border border-border rounded-lg text-text-primary focus:outline-none focus:border-primary"
                        />
                        <p className="text-xs text-text-secondary mt-1">
                          {qanythingConfig.mode === 'cloud' 
                            ? '云端 API 地址: https://openapi.youdao.com/q_anything/api' 
                            : '本地部署默认地址: http://localhost:8777'}
                        </p>
                      </div>

                      {/* API 密钥 */}
                      <div>
                        <label className="block text-sm font-medium text-text-secondary mb-2">
                          {qanythingConfig.mode === 'cloud' ? '管理秘钥' : 'API Key'}
                        </label>
                        <input
                          type="password"
                          placeholder={qanythingConfig.mode === 'cloud' ? '从 QAnything 管理后台获取' : '本地部署通常不需要'}
                          value={qanythingConfig.apiKey}
                          onChange={(e) => setQanythingConfig({...qanythingConfig, apiKey: e.target.value})}
                          className="w-full px-4 py-2 bg-surface border border-border rounded-lg text-text-primary focus:outline-none focus:border-primary"
                        />
                      </div>

                      {/* 知识库 ID */}
                      <div>
                        <label className="block text-sm font-medium text-text-secondary mb-2">知识库 ID</label>
                        <input
                          type="text"
                          placeholder="KBxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx_xxxxxx"
                          value={qanythingConfig.kbId}
                          onChange={(e) => setQanythingConfig({...qanythingConfig, kbId: e.target.value})}
                          className="w-full px-4 py-2 bg-surface border border-border rounded-lg text-text-primary focus:outline-none focus:border-primary"
                        />
                        <p className="text-xs text-text-secondary mt-1">
                          在 QAnything 管理后台创建知识库后获取 ID
                        </p>
                      </div>

                      {/* 检索数量 */}
                      <div>
                        <label className="block text-sm font-medium text-text-secondary mb-2">
                          每次检索返回的片段数
                        </label>
                        <input
                          type="number"
                          min="1"
                          max="20"
                          value={qanythingConfig.topK}
                          onChange={(e) => {
                            const val = parseInt(e.target.value) || 5;
                            setQanythingConfig({...qanythingConfig, topK: Math.max(1, Math.min(20, val))});
                          }}
                          className="w-32 px-4 py-2 bg-surface border border-border rounded-lg text-text-primary focus:outline-none focus:border-primary"
                        />
                        <span className="text-xs text-text-secondary ml-2">默认 5，范围 1-20</span>
                      </div>

                      {/* 测试连接 */}
                      <div className="flex items-center gap-3 pt-2">
                        <button
                          onClick={handleTestQAnythingConnection}
                          disabled={qanythingTestStatus === 'testing'}
                          className="px-4 py-2 bg-surface border border-border text-text-primary rounded-lg hover:bg-surface/80 transition-all disabled:opacity-50 flex items-center gap-2"
                        >
                          {qanythingTestStatus === 'testing' ? (
                            <Loader2 className="w-4 h-4 animate-spin" />
                          ) : (
                            <Wifi className="w-4 h-4" />
                          )}
                          测试连接
                        </button>
                        
                        {qanythingTestStatus === 'success' && (
                          <span className="text-sm text-status-success flex items-center gap-1">
                            <CheckCircle2 className="w-4 h-4" />
                            {qanythingTestMessage}
                          </span>
                        )}
                        {qanythingTestStatus === 'error' && (
                          <span className="text-sm text-status-failed flex items-center gap-1">
                            <AlertCircle className="w-4 h-4" />
                            {qanythingTestMessage}
                          </span>
                        )}
                      </div>

                      {/* 保存配置 */}
                      <div className="flex items-center justify-between pt-4 border-t border-border">
                        <div className="flex items-center gap-2">
                          {qanythingSaveStatus === 'saving' && (
                            <Loader2 className="w-4 h-4 animate-spin text-text-secondary" />
                          )}
                          {qanythingSaveStatus === 'saved' && (
                            <p className="text-xs text-status-success flex items-center gap-1">
                              <CheckCircle2 className="w-3 h-3" />
                              已保存
                            </p>
                          )}
                          {qanythingSaveStatus === 'error' && (
                            <p className="text-xs text-status-failed flex items-center gap-1">
                              <AlertCircle className="w-3 h-3" />
                              保存失败
                            </p>
                          )}
                        </div>
                        <button
                          onClick={handleSaveQAnythingConfig}
                          disabled={qanythingSaveStatus === 'saving'}
                          className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
                        >
                          {qanythingSaveStatus === 'saving' && (
                            <Loader2 className="w-4 h-4 animate-spin" />
                          )}
                          保存配置
                        </button>
                      </div>
                    </div>
                  </div>

                  {/* 文档上传 */}
                  <div className="bg-background rounded-lg p-6">
                    <h4 className="font-medium text-text-primary mb-4 flex items-center gap-2">
                      <Upload className="w-4 h-4" />
                      上传文档到知识库
                    </h4>
                    
                    <div className="space-y-4">
                      <div 
                        className={clsx(
                          'border-2 border-dashed rounded-lg p-8 text-center transition-all',
                          isDragOver ? 'border-primary bg-primary/5' : 'border-border'
                        )}
                        onDragOver={handleDragOver}
                        onDragLeave={handleDragLeave}
                        onDrop={handleDrop}
                      >
                        <FileText className="w-12 h-12 mx-auto text-text-secondary mb-3" />
                        <p className="text-sm text-text-primary mb-1">拖拽文件到此处，或点击选择文件</p>
                        <p className="text-xs text-text-secondary">支持 PDF/Word/Excel/PPT/Markdown/TXT/CSV/图片</p>
                        <input
                          type="file"
                          multiple
                          accept=".pdf,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.md,.txt,.csv,.jpg,.jpeg,.png"
                          onChange={(e) => {
                            const files = e.target.files;
                            if (files) {
                              setUploadFiles((prev) => [...prev, ...Array.from(files)]);
                              e.target.value = '';
                            }
                          }}
                          className="hidden"
                          id="file-upload"
                        />
                        <label
                          htmlFor="file-upload"
                          className="inline-block mt-4 px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all cursor-pointer text-sm"
                        >
                          选择文件
                        </label>
                      </div>

                      {/* 已选文件列表 */}
                      {uploadFiles.length > 0 && (
                        <div className="space-y-2">
                          <p className="text-sm font-medium text-text-primary">已选择 {uploadFiles.length} 个文件:</p>
                          <div className="max-h-40 overflow-y-auto space-y-1">
                            {uploadFiles.map((file, index) => (
                              <div key={index} className="flex items-center justify-between p-2 bg-surface rounded-lg">
                                <div className="flex items-center gap-2 min-w-0 flex-1">
                                  <FileText className="w-4 h-4 text-text-secondary flex-shrink-0" />
                                  <span className="text-sm text-text-secondary truncate">{file.name}</span>
                                </div>
                                <div className="flex items-center gap-2 flex-shrink-0">
                                  <span className="text-xs text-text-secondary">
                                    {formatFileSize(file.size)}
                                  </span>
                                  <button
                                    onClick={() => removeFile(index)}
                                    className="text-xs text-status-failed hover:text-status-failed/80 transition-colors"
                                  >
                                    移除
                                  </button>
                                </div>
                              </div>
                            ))}
                          </div>
                        </div>
                      )}

                      {/* 上传状态 */}
                      {uploadStatus !== 'idle' && (
                        <div className={clsx(
                          'p-3 rounded-lg flex items-center gap-2',
                          uploadStatus === 'uploading' && 'bg-blue-500/10 text-blue-600',
                          uploadStatus === 'success' && 'bg-green-500/10 text-green-600',
                          uploadStatus === 'error' && 'bg-red-500/10 text-red-600'
                        )}>
                          {uploadStatus === 'uploading' && <Loader2 className="w-4 h-4 animate-spin" />}
                          {uploadStatus === 'success' && <CheckCircle2 className="w-4 h-4" />}
                          {uploadStatus === 'error' && <AlertCircle className="w-4 h-4" />}
                          <span className="text-sm">{uploadMessage}</span>
                        </div>
                      )}

                      {/* 上传按钮 */}
                      <button
                        onClick={handleUploadDocuments}
                        disabled={!qanythingConfig.enabled || uploadFiles.length === 0 || uploadStatus === 'uploading'}
                        className="w-full px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-all disabled:opacity-50 disabled:cursor-not-allowed flex items-center justify-center gap-2"
                      >
                        {uploadStatus === 'uploading' ? (
                          <>
                            <Loader2 className="w-4 h-4 animate-spin" />
                            上传中...
                          </>
                        ) : (
                          <>
                            <Upload className="w-4 h-4" />
                            上传到知识库
                          </>
                        )}
                      </button>
                    </div>
                  </div>
                </div>
              )}

              {/* ========== 通知配置 ========== */}
              {activeTab === 'notifications' && <NotificationSettings />}

              {/* ========== 数据库/备份 ========== */}
              {activeTab === 'database' && <BackupSettings />}

              {/* ========== MySQL 生产持久化与数据迁移 ========== */}
              {activeTab === 'mysql_storage' && <DatabaseModeSettings />}

              {/* ========== 时序库写入配置 ========== */}
              {activeTab === 'influxdb' && <InfluxdbSettings />}

              {/* ========== 界面与菜单 ========== */}
              {activeTab === 'appearance' && <AppearanceSettings />}
              {/* ========== 安全设置 + 通用设置 ========== */}
              {activeTab === 'security' && (
                <div className="space-y-6">
                  <div>
                    <h3 className="text-lg font-semibold text-text-primary mb-4 flex items-center gap-2">
                      <Shield className="w-5 h-5" />
                      安全设置
                    </h3>
                    <p className="text-sm text-text-secondary mb-6">
                      配置安全策略和访问控制
                    </p>
                  </div>

                  <SecuritySettings />
                  <GeneralSettings />
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
  );
}

// ============================================================
// InfluxDB 时序库写入配置
// ============================================================

function InfluxdbSettings() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const navigate = useNavigate();

  const { data, isLoading } = useQuery({
    queryKey: ['influxdb-write-settings'],
    queryFn: async () => {
      const res = await api.get('/influxdb-write');
      const payload = (res.data?.data !== undefined ? res.data.data : res.data) as {
        settings: {
          id: string; enabled: number; db_connection_id: string | null;
          measurement_prefix: string; write_mode: string;
          batch_size: number; flush_interval_ms: number;
        };
        influxdbConnections: Array<{
          id: string; name: string; db_type: string; host: string; port: number; enabled: number;
        }>;
      };
      return payload;
    },
  });

  const settings = data?.settings;
  const connections = data?.influxdbConnections || [];

  const updateMutation = useMutation({
    mutationFn: async (payload: Record<string, unknown>) => {
      const res = await api.put('/influxdb-write', payload);
      return res.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['influxdb-write-settings'] });
      toast.success('InfluxDB 写入配置已更新');
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '更新失败'));
    },
  });

  const testWriteMutation = useMutation({
    mutationFn: async () => {
      const res = await api.post('/influxdb-write/test-write');
      return res.data;
    },
    onSuccess: (d) => {
      toast.success(d.message || '测试写入成功');
    },
    onError: (err: unknown) => {
      toast.error(getAxiosErrorMessage(err, '测试写入失败'));
    },
  });

  const handleToggleEnabled = (checked: boolean) => {
    const newEnabled = checked ? 1 : 0;
    if (newEnabled === 1 && connections.length === 0) {
      toast.warning('未检测到 InfluxDB 连接，请先在「数据库管理」中添加 InfluxDB 类型的数据库连接后再启用');
      return;
    }
    const payload: Record<string, unknown> = { enabled: newEnabled };
    if (newEnabled === 1 && !settings?.db_connection_id && connections.length > 0) {
      payload.db_connection_id = connections[0].id;
    }
    updateMutation.mutate(payload);
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-40">
        <Loader2 className="w-6 h-6 text-blue-600 animate-spin" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h3 className="text-lg font-semibold text-text-primary mb-4 flex items-center gap-2">
          <Activity className="w-5 h-5 text-purple-600" />
          时序库写入配置
        </h3>
        <p className="text-sm text-text-secondary mb-6">
          将 SNMP 采集结果实时写入 InfluxDB 时序数据库，用于长期历史趋势分析和可视化展示。
          请先在「数据库管理」中添加 InfluxDB 连接。
        </p>
      </div>

      {/* 状态概览 */}
      <div className={`rounded-xl p-5 border transition-all ${
        settings?.enabled && settings?.db_connection_id
          ? 'bg-emerald-500/10 border-emerald-500/30 shadow-sm'
          : 'bg-surface border-border'
      }`}>
        <div className="flex items-center gap-2.5">
          {settings?.enabled && settings?.db_connection_id ? (
            <CheckCircle2 className="w-5 h-5 text-emerald-600 flex-shrink-0" />
          ) : (
            <AlertCircle className="w-5 h-5 text-text-secondary flex-shrink-0" />
          )}
          <span className="text-sm font-semibold text-text-primary">
            {settings?.enabled && settings?.db_connection_id
              ? 'InfluxDB 写入已启用'
              : settings?.enabled ? '未选择 InfluxDB 连接' : 'InfluxDB 写入未启用'}
          </span>
        </div>
        {connections.length === 0 && (
          <div className="mt-3 flex flex-col sm:flex-row sm:items-center justify-between gap-2.5 p-3 bg-amber-500/10 border border-amber-500/30 rounded-xl text-amber-600 text-xs">
            <span className="flex items-center gap-1.5">
              <AlertCircle className="w-4 h-4 flex-shrink-0" />
              未检测到 InfluxDB 连接，请先在「数据库管理」中添加 InfluxDB 类型的数据库连接。
            </span>
            <button
              onClick={() => navigate('/db-connections')}
              className="px-3 py-1.5 bg-amber-600 hover:bg-amber-500 text-white rounded-lg text-xs font-medium transition-all shadow-sm flex-shrink-0 self-start sm:self-auto"
            >
              前往添加 InfluxDB 连接
            </button>
          </div>
        )}
      </div>

      {/* 配置表单 */}
      <div className="bg-background rounded-xl p-6 space-y-5 border border-border">
        {/* 启用开关 */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-sm font-medium text-text-primary">启用时序库写入</p>
            <p className="text-xs text-text-secondary mt-0.5">启用后，SNMP 采集结果将自动写入配置的 InfluxDB</p>
          </div>
          <label className="relative inline-flex items-center cursor-pointer">
            <input
              type="checkbox"
              className="sr-only peer"
              checked={settings?.enabled === 1}
              onChange={(e) => handleToggleEnabled(e.target.checked)}
            />
            <div className="w-11 h-6 bg-border peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-blue-600" />
          </label>
        </div>

        {/* 选择 InfluxDB 连接 */}
        <div>
          <div className="flex items-center justify-between mb-1.5">
            <label className="block text-sm font-medium text-text-primary">InfluxDB 连接</label>
            {connections.length > 0 && (
              <span className="text-xs text-emerald-600">已检测到 {connections.length} 个可用 InfluxDB 连接</span>
            )}
          </div>
          <select
            value={settings?.db_connection_id || ''}
            onChange={(e) => updateMutation.mutate({ db_connection_id: e.target.value || null })}
            className="w-full px-4 py-2.5 bg-surface border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all text-sm"
          >
            <option value="">-- 选择 InfluxDB 连接 --</option>
            {connections.map(c => (
              <option key={c.id} value={c.id}>
                {c.name} ({c.db_type} - {c.host}:{c.port})
              </option>
            ))}
          </select>
        </div>

        {/* Measurement 前缀 */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-text-primary mb-1">Measurement 前缀</label>
            <input
              value={settings?.measurement_prefix || 'snmp'}
              onChange={(e) => updateMutation.mutate({ measurement_prefix: e.target.value })}
              placeholder="snmp"
              className="w-full px-4 py-2.5 bg-surface border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all text-sm"
            />
            <p className="text-xs text-text-secondary mt-1">写入的 measurement 名称格式：前缀_指标名</p>
          </div>
          <div>
            <label className="block text-sm font-medium text-text-primary mb-1">写入模式</label>
            <select
              value={settings?.write_mode || 'async'}
              onChange={(e) => updateMutation.mutate({ write_mode: e.target.value })}
              className="w-full px-4 py-2.5 bg-surface border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all text-sm"
            >
              <option value="async">异步（推荐）</option>
              <option value="sync">同步</option>
            </select>
          </div>
        </div>

        {/* 批量大小 + 刷新间隔 */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-text-primary mb-1">批量大小</label>
            <input
              type="number"
              value={settings?.batch_size || 100}
              onChange={(e) => updateMutation.mutate({ batch_size: Number(e.target.value) })}
              min={1}
              max={10000}
              className="w-full px-4 py-2.5 bg-surface border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all text-sm"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-text-primary mb-1">刷新间隔 (ms)</label>
            <input
              type="number"
              value={settings?.flush_interval_ms || 5000}
              onChange={(e) => updateMutation.mutate({ flush_interval_ms: Number(e.target.value) })}
              min={1000}
              max={60000}
              className="w-full px-4 py-2.5 bg-surface border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all text-sm"
            />
          </div>
        </div>

        {/* 测试写入 */}
        <div className="pt-3 border-t border-border/40 flex flex-col sm:flex-row sm:items-center justify-between gap-2">
          <div>
            <button
              onClick={() => testWriteMutation.mutate()}
              disabled={testWriteMutation.isPending || !settings?.db_connection_id}
              className="px-4 py-2 bg-purple-600 hover:bg-purple-500 text-white rounded-xl text-sm font-medium transition-all shadow-md shadow-purple-500/20 disabled:opacity-50 flex items-center gap-2"
            >
              {testWriteMutation.isPending ? (
                <Loader2 className="w-4 h-4 animate-spin" />
              ) : (
                <Wifi className="w-4 h-4" />
              )}
              测试写入
            </button>
            <p className="text-xs text-text-secondary mt-1">向当前选中的 InfluxDB 发送一条测试指标验证写入通信是否正常</p>
          </div>
          {testWriteMutation.isSuccess && (
            <span className="text-xs text-emerald-600 flex items-center gap-1">
              <CheckCircle2 className="w-3.5 h-3.5" />
              通信正常
            </span>
          )}
        </div>
      </div>
    </div>
  );
}
