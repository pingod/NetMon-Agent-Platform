import React, { useEffect, useState } from 'react';
import {
  X,
  Copy,
  Check,
  Terminal,
  Layers,
  Cpu,
  Monitor,
  Loader2,
  AlertTriangle,
} from 'lucide-react';
import { useToast } from '../../../contexts/ToastContext';
import api from '../../../lib/api';
import type { Server as ServerType } from '../pages/types';

interface AgentInstallManualModalProps {
  isOpen: boolean;
  onClose: () => void;
  server: ServerType | null;
}

export const AgentInstallManualModal: React.FC<AgentInstallManualModalProps> = ({
  isOpen,
  onClose,
  server,
}) => {
  const { success, error } = useToast();
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [tab, setTab] = useState<'linux' | 'windows' | 'docker'>('linux');
  const [ticket, setTicket] = useState<string | null>(null);
  const [ticketError, setTicketError] = useState<string | null>(null);

  const serverId = server?.id || '';

  useEffect(() => {
    if (!isOpen || !serverId) return;
    let cancelled = false;
    setTicket(null);
    setTicketError(null);
    api
      .post('/agent/install-ticket', { server_id: serverId })
      .then((res) => {
        if (cancelled) return;
        const t = res.data?.data?.ticket;
        if (t) setTicket(t);
        else setTicketError('票据签发失败，请重试');
      })
      .catch((err) => {
        if (cancelled) return;
        setTicketError(err?.response?.data?.message || '票据签发失败，请重试');
      });
    return () => {
      cancelled = true;
    };
  }, [isOpen, serverId]);

  if (!isOpen) return null;

  const origin = window.location.origin;
  const ticketQuery = ticket ? `&ticket=${encodeURIComponent(ticket)}` : '';

  const linuxCmd = `curl -sSL -k "${origin}/api/v1/agent/install.sh?server_id=${serverId}${ticketQuery}" | sudo bash`;
  const windowsCmd = `powershell -ExecutionPolicy Bypass -Command "irm ${origin}/api/v1/agent/install.ps1?server_id=${serverId}${ticketQuery} | iex"`;
  const dockerCmd = `docker run -d \\
  --name netmon-agent \\
  --restart always \\
  --net=host \\
  --pid=host \\
  -v /proc:/proc:ro \\
  -v /sys:/sys:ro \\
  -v /etc:/etc:ro \\
  -e SERVER_ID="${serverId}" \\
  -e REPORT_URL="${origin}/api/v1/agent/report" \\
  netmon/agent:v1.2.0`;

  const cmdReady = tab === 'docker' || !!ticket;
  const currentCmd = tab === 'linux' ? linuxCmd : tab === 'windows' ? windowsCmd : dockerCmd;

  const copyToClipboard = (text: string, key: string) => {
    if (!cmdReady) {
      error('安装票据尚未就绪，请稍候或重新打开本窗口');
      return;
    }
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    success('已复制到剪贴板');
    setTimeout(() => setCopiedKey(null), 2000);
  };

  const tabBtn = (key: typeof tab, icon: React.ReactNode, label: string) => (
    <button
      onClick={() => setTab(key)}
      className="flex-1 py-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition"
      style={
        tab === key
          ? { background: 'var(--primary)', color: '#fff', boxShadow: '0 2px 8px rgba(90,74,232,.25)' }
          : { color: 'var(--text-2)' }
      }
    >
      {icon}
      <span>{label}</span>
    </button>
  );

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-5 bg-black/40 backdrop-blur-sm animate-in fade-in duration-200">
      <div
        className="relative w-full max-w-2xl rounded-2xl shadow-2xl overflow-hidden font-sans"
        style={{ background: 'var(--surface-1)', border: '1px solid var(--border-subtle)', color: 'var(--text-1)' }}
      >
        {/* 顶部标题 */}
        <div
          className="flex items-center justify-between px-6 py-4"
          style={{ borderBottom: '1px solid var(--border-subtle)', background: 'var(--surface-2)' }}
        >
          <div className="flex items-center gap-3">
            <div
              className="w-10 h-10 rounded-xl flex items-center justify-center"
              style={{ background: 'rgba(90,74,232,.08)', border: '1px solid rgba(90,74,232,.25)', color: 'var(--primary)' }}
            >
              <Terminal className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-bold" style={{ color: 'var(--text-1)' }}>
                手动安装 netmon-agent
              </h2>
              <p className="text-xs" style={{ color: 'var(--text-3)' }}>
                目标机器: <span className="font-semibold" style={{ color: 'var(--primary)' }}>{server?.name || '所有虚拟机'}</span>
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-xl transition hover:opacity-70"
            style={{ background: 'var(--surface-3)', border: '1px solid var(--border-subtle)', color: 'var(--text-2)' }}
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* 内容区 */}
        <div className="p-6 space-y-5">
          {/* 系统 Tab 切换 */}
          <div
            className="flex items-center gap-2 p-1 rounded-xl"
            style={{ background: 'var(--surface-2)', border: '1px solid var(--border-subtle)' }}
          >
            {tabBtn('linux', <Cpu className="w-4 h-4" />, 'Linux (一键脚本)')}
            {tabBtn('windows', <Monitor className="w-4 h-4" />, 'Windows (PowerShell)')}
            {tabBtn('docker', <Layers className="w-4 h-4" />, 'Docker 容器')}
          </div>

          {/* 指令展示框 */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs font-medium" style={{ color: 'var(--text-2)' }}>
                {tab === 'linux'
                  ? '在目标 Linux 终端以 root 或 sudo 执行：'
                  : tab === 'windows'
                  ? '以管理员身份打开 PowerShell 执行：'
                  : '在容器宿主机执行 Docker 命令：'}
              </span>
              <button
                onClick={() => copyToClipboard(currentCmd, 'main-cmd')}
                disabled={!cmdReady}
                className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg text-xs transition disabled:opacity-50"
                style={{
                  background: 'rgba(90,74,232,.08)',
                  border: '1px solid rgba(90,74,232,.25)',
                  color: 'var(--primary)',
                }}
              >
                {copiedKey === 'main-cmd' ? (
                  <>
                    <Check className="w-3.5 h-3.5" style={{ color: '#10b981' }} />
                    <span>已复制</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3.5 h-3.5" />
                    <span>一键复制指令</span>
                  </>
                )}
              </button>
            </div>

            <div
              className="p-4 rounded-xl font-mono text-xs break-all leading-relaxed min-h-[72px] flex items-center"
              style={{ background: 'var(--surface-2)', border: '1px solid var(--border-subtle)' }}
            >
              {tab === 'docker' ? (
                <pre className="whitespace-pre-wrap w-full" style={{ color: 'var(--text-1)' }}>{dockerCmd}</pre>
              ) : ticketError ? (
                <div className="flex items-center gap-2 text-xs" style={{ color: '#dc2626' }}>
                  <AlertTriangle className="w-4 h-4 shrink-0" />
                  <span>{ticketError}（重新打开本窗口可重试）</span>
                </div>
              ) : !ticket ? (
                <div className="flex items-center gap-2 text-xs" style={{ color: 'var(--text-3)' }}>
                  <Loader2 className="w-4 h-4 animate-spin shrink-0" />
                  <span>正在签发安装票据…</span>
                </div>
              ) : (
                <span style={{ color: 'var(--text-1)' }}>{currentCmd}</span>
              )}
            </div>
            {tab !== 'docker' && ticket && (
              <p className="mt-2 text-[11px]" style={{ color: 'var(--text-3)' }}>
                命令内含 15 分钟有效的安装票据，过期后重新打开本窗口即可刷新。
              </p>
            )}
          </div>

          {/* 运维指令说明 */}
          <div
            className="p-4 rounded-xl space-y-2 text-xs"
            style={{ background: 'var(--surface-2)', border: '1px solid var(--border-subtle)', color: 'var(--text-2)' }}
          >
            <p className="font-semibold" style={{ color: 'var(--text-1)' }}>常用服务管理指令：</p>
            {tab === 'linux' ? (
              <ul className="list-disc list-inside space-y-1 font-mono text-[11px]">
                <li>查看运行状态: <span style={{ color: 'var(--primary)' }}>systemctl status netmon-agent</span></li>
                <li>重启采集器: <span style={{ color: 'var(--primary)' }}>systemctl restart netmon-agent</span></li>
                <li>卸载并停止服务: <span style={{ color: 'var(--primary)' }}>systemctl stop netmon-agent && rm -f /usr/local/bin/netmon-agent</span></li>
              </ul>
            ) : tab === 'windows' ? (
              <ul className="list-disc list-inside space-y-1 font-mono text-[11px]">
                <li>查看运行进程: <span style={{ color: 'var(--primary)' }}>Get-Process powershell</span></li>
                <li>停止采集服务: <span style={{ color: 'var(--primary)' }}>Stop-Process -Name powershell</span></li>
              </ul>
            ) : (
              <ul className="list-disc list-inside space-y-1 font-mono text-[11px]">
                <li>查看容器日志: <span style={{ color: 'var(--primary)' }}>docker logs -f netmon-agent</span></li>
              </ul>
            )}
          </div>
        </div>

        {/* 底部按钮 */}
        <div
          className="flex items-center justify-end px-6 py-4"
          style={{ borderTop: '1px solid var(--border-subtle)', background: 'var(--surface-2)' }}
        >
          <button
            onClick={onClose}
            className="px-5 py-2 rounded-xl text-xs font-medium transition hover:opacity-80"
            style={{ background: 'var(--surface-3)', border: '1px solid var(--border-subtle)', color: 'var(--text-1)' }}
          >
            完成
          </button>
        </div>
      </div>
    </div>
  );
};
