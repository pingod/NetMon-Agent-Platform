import React, { useState } from 'react';
import {
  X,
  Copy,
  Check,
  Terminal,
  ShieldCheck,
  Server,
  Layers,
  Cpu,
  Monitor,
  CheckCircle2,
} from 'lucide-react';
import { useToast } from '../../../contexts/ToastContext';
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
  const { success } = useToast();
  const [copiedKey, setCopiedKey] = useState<string | null>(null);
  const [tab, setTab] = useState<'linux' | 'windows' | 'docker'>('linux');

  if (!isOpen) return null;

  const serverId = server?.id || 'YOUR_VM_ID';
  const origin = window.location.origin;

  const linuxCmd = `curl -sSL -k "${origin}/api/v1/agent/install.sh?server_id=${serverId}" | sudo bash`;
  const windowsCmd = `powershell -ExecutionPolicy Bypass -Command "irm ${origin}/api/v1/agent/install.ps1?server_id=${serverId} | iex"`;
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

  const copyToClipboard = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopiedKey(key);
    success('已复制到剪贴板');
    setTimeout(() => setCopiedKey(null), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-5 bg-black/80 backdrop-blur-md animate-in fade-in duration-200">
      <div className="relative w-full max-w-2xl bg-[#0b0f19] border border-slate-800 rounded-2xl shadow-2xl overflow-hidden text-slate-100 font-sans">
        
        {/* 顶部标题 */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-slate-800 bg-slate-900/60">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-indigo-500/10 border border-indigo-500/30 flex items-center justify-center text-indigo-400">
              <Terminal className="w-5 h-5" />
            </div>
            <div>
              <h2 className="text-base font-bold text-slate-100">
                手动安装 netmon-agent
              </h2>
              <p className="text-xs text-slate-400">
                目标机器: <span className="text-indigo-400 font-semibold">{server?.name || '所有虚拟机'}</span>
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 rounded-xl bg-slate-800/80 border border-slate-700 text-slate-400 hover:text-white transition"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* 内容区 */}
        <div className="p-6 space-y-5">
          
          {/* 系统 Tab 切换 */}
          <div className="flex items-center gap-2 p-1 bg-slate-950 rounded-xl border border-slate-800">
            <button
              onClick={() => setTab('linux')}
              className={`flex-1 py-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition ${
                tab === 'linux'
                  ? 'bg-indigo-600 text-white shadow-md'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Cpu className="w-4 h-4" />
              <span>Linux (一键脚本)</span>
            </button>

            <button
              onClick={() => setTab('windows')}
              className={`flex-1 py-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition ${
                tab === 'windows'
                  ? 'bg-indigo-600 text-white shadow-md'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Monitor className="w-4 h-4" />
              <span>Windows (PowerShell)</span>
            </button>

            <button
              onClick={() => setTab('docker')}
              className={`flex-1 py-2 rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition ${
                tab === 'docker'
                  ? 'bg-indigo-600 text-white shadow-md'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Layers className="w-4 h-4" />
              <span>Docker 容器</span>
            </button>
          </div>

          {/* 指令展示框 */}
          <div>
            <div className="flex items-center justify-between mb-2">
              <span className="text-xs text-slate-300 font-medium">
                {tab === 'linux'
                  ? '在目标 Linux 终端以 root 或 sudo 执行：'
                  : tab === 'windows'
                  ? '以管理员身份打开 PowerShell 执行：'
                  : '在容器宿主机执行 Docker 命令：'}
              </span>
              <button
                onClick={() =>
                  copyToClipboard(
                    tab === 'linux' ? linuxCmd : tab === 'windows' ? windowsCmd : dockerCmd,
                    'main-cmd'
                  )
                }
                className="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-indigo-500/10 hover:bg-indigo-500/20 border border-indigo-500/30 text-indigo-300 text-xs transition"
              >
                {copiedKey === 'main-cmd' ? (
                  <>
                    <Check className="w-3.5 h-3.5 text-emerald-400" />
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

            <div className="p-4 rounded-xl bg-[#070a13] border border-slate-800 font-mono text-xs text-emerald-400 break-all leading-relaxed relative group">
              {tab === 'linux' ? linuxCmd : tab === 'windows' ? windowsCmd : dockerCmd}
            </div>
          </div>

          {/* 运维指令说明 */}
          <div className="p-4 rounded-xl bg-slate-900/40 border border-slate-800 space-y-2 text-xs text-slate-400">
            <p className="font-semibold text-slate-300">💡 常用服务管理指令：</p>
            {tab === 'linux' ? (
              <ul className="list-disc list-inside space-y-1 font-mono text-[11px] text-slate-400">
                <li>查看运行状态: <span className="text-indigo-300">systemctl status netmon-agent</span></li>
                <li>重启采集器: <span className="text-indigo-300">systemctl restart netmon-agent</span></li>
                <li>卸载并停止服务: <span className="text-indigo-300">systemctl stop netmon-agent && rm -f /usr/local/bin/netmon-agent</span></li>
              </ul>
            ) : tab === 'windows' ? (
              <ul className="list-disc list-inside space-y-1 font-mono text-[11px] text-slate-400">
                <li>查看运行进程: <span className="text-indigo-300">Get-Process powershell</span></li>
                <li>停止采集服务: <span className="text-indigo-300">Stop-Process -Name powershell</span></li>
              </ul>
            ) : (
              <ul className="list-disc list-inside space-y-1 font-mono text-[11px] text-slate-400">
                <li>查看容器日志: <span className="text-indigo-300">docker logs -f netmon-agent</span></li>
              </ul>
            )}
          </div>
        </div>

        {/* 底部按钮 */}
        <div className="flex items-center justify-end px-6 py-4 border-t border-slate-800 bg-slate-900/60">
          <button
            onClick={onClose}
            className="px-5 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
          >
            完成
          </button>
        </div>

      </div>
    </div>
  );
};
