import React, { useState } from 'react';
import {
  X,
  Play,
  Terminal,
  CheckCircle2,
  AlertCircle,
  Clock,
  Shield,
  Key,
  Server,
  Zap,
  RotateCw,
  Copy,
  Check,
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import type { Server as ServerType } from '../pages/types';

interface BatchInstallAgentModalProps {
  isOpen: boolean;
  onClose: () => void;
  selectedServers: ServerType[];
  onSuccess: () => void;
}

interface InstallLogItem {
  serverId: string;
  serverName: string;
  host: string;
  status: 'pending' | 'installing' | 'success' | 'failed';
  message?: string;
  logs?: string;
  durationMs?: number;
}

export const BatchInstallAgentModal: React.FC<BatchInstallAgentModalProps> = ({
  isOpen,
  onClose,
  selectedServers,
  onSuccess,
}) => {
  const { toast } = useToast();
  const [authMode, setAuthMode] = useState<'saved' | 'custom'>('saved');
  const [customPort, setCustomPort] = useState(22);
  const [customUser, setCustomUser] = useState('root');
  const [customPassword, setCustomPassword] = useState('');
  const [isInstalling, setIsInstalling] = useState(false);
  const [progressList, setProgressList] = useState<InstallLogItem[]>([]);
  const [activeLogTab, setActiveLogTab] = useState<string>('');

  if (!isOpen) return null;

  const handleStartBatchInstall = async () => {
    if (selectedServers.length === 0) {
      toast('请选择要安装 Agent 的虚拟机', 'warning');
      return;
    }

    setIsInstalling(true);
    const initialList: InstallLogItem[] = selectedServers.map((s) => ({
      serverId: s.id,
      serverName: s.name,
      host: s.hostname || s.ip_address || '',
      status: 'installing',
      message: '正在建立 SSH 连接并下发 netmon-agent 脚本...',
    }));
    setProgressList(initialList);
    if (initialList.length > 0) {
      setActiveLogTab(initialList[0].serverId);
    }

    try {
      const payload: any = {
        server_ids: selectedServers.map((s) => s.id),
      };
      if (authMode === 'custom') {
        payload.port = customPort;
        payload.username = customUser;
        if (customPassword) payload.password = customPassword;
      }

      const res = await api.post('/servers/agent/batch-install', payload);
      const results: any[] = res.data?.data?.results || res.data?.results || [];

      // 更新执行结果
      setProgressList((prev) =>
        prev.map((item) => {
          const match = results.find((r) => r.server_id === item.serverId);
          if (match) {
            return {
              ...item,
              status: match.success ? 'success' : 'failed',
              message: match.message,
              logs: match.logs,
              durationMs: match.duration_ms,
            };
          }
          return {
            ...item,
            status: 'failed',
            message: '未获取到安装结果',
          };
        })
      );

      const successCount = results.filter((r) => r.success).length;
      if (successCount === selectedServers.length) {
        toast(`🎉 批量安装成功！全部 ${successCount} 台虚拟机 Agent 已上线`, 'success');
      } else {
        toast(`批量下发完成：${successCount} 成功，${selectedServers.length - successCount} 失败`, 'info');
      }
      onSuccess();
    } catch (err: any) {
      console.error('Batch install error:', err);
      toast('下发批量安装指令失败: ' + (err.response?.data?.message || err.message), 'error');
      setProgressList((prev) =>
        prev.map((item) => ({
          ...item,
          status: 'failed',
          message: '请求异常: ' + (err.response?.data?.message || err.message),
        }))
      );
    } finally {
      setIsInstalling(false);
    }
  };

  const activeItem = progressList.find((p) => p.serverId === activeLogTab) || progressList[0];
  const successCount = progressList.filter((p) => p.status === 'success').length;
  const failedCount = progressList.filter((p) => p.status === 'failed').length;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-5 bg-black/80 backdrop-blur-md animate-in fade-in duration-200">
      <div className="relative w-full max-w-4xl max-h-[92vh] flex flex-col bg-[#0b0f19] border border-slate-800 rounded-2xl shadow-2xl overflow-hidden text-slate-100 font-sans">
        
        {/* 顶部标题 */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800 bg-slate-900/60">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-indigo-500/20 to-purple-500/20 border border-indigo-500/30 flex items-center justify-center text-indigo-400">
              <Zap className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-slate-100">批量下发安装 netmon-agent</h2>
              <p className="text-xs text-slate-400">
                通过 SSH 自动化并发安装超轻量采集器，自动注册 systemd 守护进程与心跳上报
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-xl bg-slate-800/80 border border-slate-700 text-slate-400 hover:text-white hover:bg-slate-700 transition"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* 主体区 */}
        <div className="flex-1 overflow-y-auto p-6 space-y-6">
          
          {/* 目标机器列表预览 */}
          <div>
            <label className="text-xs font-semibold text-slate-300 block mb-2">
              目标虚拟机清单 ({selectedServers.length} 台)
            </label>
            <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2.5 max-h-36 overflow-y-auto p-1">
              {selectedServers.map((s) => (
                <div
                  key={s.id}
                  className="flex items-center gap-2 p-2.5 rounded-lg bg-slate-900/80 border border-slate-800 text-xs"
                >
                  <Server className="w-4 h-4 text-indigo-400 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <p className="font-semibold text-slate-200 truncate">{s.name}</p>
                    <p className="text-[11px] text-slate-400 truncate">{s.hostname || s.ip_address}</p>
                  </div>
                  {s.agent_status === 'online' ? (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
                      已在线
                    </span>
                  ) : (
                    <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-800 text-slate-400">
                      未安装
                    </span>
                  )}
                </div>
              ))}
            </div>
          </div>

          {/* SSH 认证方式选择 */}
          <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800 space-y-3">
            <label className="text-xs font-semibold text-slate-300 block">SSH 远程认证配置</label>
            <div className="flex flex-wrap gap-4">
              <label className="flex items-center gap-2 text-xs text-slate-300 cursor-pointer">
                <input
                  type="radio"
                  name="authMode"
                  checked={authMode === 'saved'}
                  onChange={() => setAuthMode('saved')}
                  className="text-indigo-600 focus:ring-indigo-500"
                />
                <span>使用各虚拟机预先保存的 SSH 凭据与密钥 (推荐)</span>
              </label>

              <label className="flex items-center gap-2 text-xs text-slate-300 cursor-pointer">
                <input
                  type="radio"
                  name="authMode"
                  checked={authMode === 'custom'}
                  onChange={() => setAuthMode('custom')}
                  className="text-indigo-600 focus:ring-indigo-500"
                />
                <span>批量使用统一 SSH 凭据覆盖</span>
              </label>
            </div>

            {authMode === 'custom' && (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 pt-2">
                <div>
                  <label className="text-[11px] text-slate-400 block mb-1">SSH 端口</label>
                  <input
                    type="number"
                    value={customPort}
                    onChange={(e) => setCustomPort(Number(e.target.value))}
                    className="w-full px-3 py-1.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:border-indigo-500"
                  />
                </div>
                <div>
                  <label className="text-[11px] text-slate-400 block mb-1">SSH 用户名</label>
                  <input
                    type="text"
                    value={customUser}
                    onChange={(e) => setCustomUser(e.target.value)}
                    className="w-full px-3 py-1.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:border-indigo-500"
                  />
                </div>
                <div>
                  <label className="text-[11px] text-slate-400 block mb-1">SSH 密码</label>
                  <input
                    type="password"
                    value={customPassword}
                    placeholder="输入统一 root 密码"
                    onChange={(e) => setCustomPassword(e.target.value)}
                    className="w-full px-3 py-1.5 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-200 focus:border-indigo-500"
                  />
                </div>
              </div>
            )}
          </div>

          {/* 实时安装日志与终端执行状态 */}
          {progressList.length > 0 && (
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <label className="text-xs font-semibold text-slate-300 flex items-center gap-2">
                  <Terminal className="w-4 h-4 text-indigo-400" /> 安装进度与执行终端日志
                </label>
                <div className="flex items-center gap-3 text-xs">
                  <span className="text-emerald-400">成功: {successCount}</span>
                  <span className="text-red-400">失败: {failedCount}</span>
                  <span className="text-slate-400">总计: {progressList.length}</span>
                </div>
              </div>

              {/* 机器切换 Tab */}
              <div className="flex items-center gap-1.5 overflow-x-auto pb-1">
                {progressList.map((item) => (
                  <button
                    key={item.serverId}
                    onClick={() => setActiveLogTab(item.serverId)}
                    className={`px-3 py-1.5 rounded-lg text-xs font-medium flex items-center gap-1.5 whitespace-nowrap transition ${
                      activeLogTab === item.serverId
                        ? 'bg-slate-800 text-slate-100 border border-slate-700'
                        : 'bg-slate-900/50 text-slate-400 hover:bg-slate-850'
                    }`}
                  >
                    {item.status === 'installing' ? (
                      <RotateCw className="w-3 h-3 text-indigo-400 animate-spin" />
                    ) : item.status === 'success' ? (
                      <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
                    ) : item.status === 'failed' ? (
                      <AlertCircle className="w-3.5 h-3.5 text-red-400" />
                    ) : (
                      <Clock className="w-3.5 h-3.5 text-slate-500" />
                    )}
                    <span>{item.serverName}</span>
                  </button>
                ))}
              </div>

              {/* 终端黑框日志 */}
              {activeItem && (
                <div className="p-4 rounded-xl bg-[#070a13] border border-slate-800 font-mono text-xs text-slate-300 max-h-56 overflow-y-auto space-y-1.5">
                  <div className="flex items-center justify-between pb-2 mb-2 border-b border-slate-800 text-slate-400 text-[11px]">
                    <span>机器: {activeItem.serverName} ({activeItem.host})</span>
                    <span>状态: {activeItem.status === 'installing' ? '🟡 正在下发安装...' : activeItem.status === 'success' ? '🟢 安装成功并启动' : '🔴 失败'}</span>
                  </div>
                  {activeItem.message && <p className="text-indigo-400">{'>'} {activeItem.message}</p>}
                  {activeItem.logs ? (
                    <pre className="text-emerald-400 whitespace-pre-wrap leading-relaxed text-[11px]">
                      {activeItem.logs}
                    </pre>
                  ) : (
                    <p className="text-slate-500 italic">正在等待远程 SSH 会话输出日志...</p>
                  )}
                </div>
              )}
            </div>
          )}
        </div>

        {/* 底部按钮栏 */}
        <div className="flex items-center justify-between px-6 py-4 border-t border-slate-800 bg-slate-900/60">
          <div className="text-xs text-slate-400">
            netmon-agent 占用系统资源极低（CPU {'<'} 0.05%, 内存 3.8MB）
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={onClose}
              disabled={isInstalling}
              className="px-4 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
            >
              关闭
            </button>
            <button
              onClick={handleStartBatchInstall}
              disabled={isInstalling || selectedServers.length === 0}
              className="flex items-center gap-2 px-5 py-2 rounded-xl bg-gradient-to-r from-indigo-600 to-purple-600 hover:from-indigo-500 hover:to-purple-500 text-white text-xs font-semibold shadow-lg shadow-indigo-600/30 transition disabled:opacity-50"
            >
              {isInstalling ? (
                <>
                  <RotateCw className="w-4 h-4 animate-spin" />
                  <span>正在并发安装中...</span>
                </>
              ) : (
                <>
                  <Zap className="w-4 h-4" />
                  <span>开始批量安装 ({selectedServers.length})</span>
                </>
              )}
            </button>
          </div>
        </div>

      </div>
    </div>
  );
};
