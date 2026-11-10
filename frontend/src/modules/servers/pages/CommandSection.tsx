import { Terminal, Trash2 } from 'lucide-react';
import clsx from 'clsx';
import type { CommandResult, Server } from './types';

interface CommandSectionProps {
  selectedServer: Server | null;
  command: string;
  onCommandChange: (v: string) => void;
  commandResult: CommandResult | null;
  onClearResult: () => void;
  isExecuting: boolean;
  onExecute: () => void;
}

export function CommandSection({
  selectedServer,
  command,
  onCommandChange,
  commandResult,
  onClearResult,
  isExecuting,
  onExecute,
}: CommandSectionProps) {
  if (!selectedServer) return null;

  return (
    <>
      {/* 命令执行结果 */}
      {commandResult !== null && (
        <div className="bg-surface border border-border rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-lg font-semibold text-text-primary">命令执行结果</h3>
            <button onClick={onClearResult} className="p-1 hover:bg-background rounded transition-colors">
              <Trash2 className="w-4 h-4 text-text-secondary" />
            </button>
          </div>
          <div className="space-y-4">
            <div>
              <p className="text-xs text-text-secondary mb-1">执行的命令:</p>
              <code className="font-mono text-sm bg-background px-2 py-1 rounded text-text-primary">
                {commandResult.command}
              </code>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-text-secondary">状态:</span>
              <span
                className={clsx(
                  'px-2 py-1 rounded text-xs font-medium',
                  commandResult.success
                    ? 'bg-status-success/10 text-status-success'
                    : 'bg-status-failed/10 text-status-failed',
                )}
              >
                {commandResult.success ? '成功' : '失败'}
              </span>
              <span className="text-xs text-text-secondary ml-4">耗时: {commandResult.duration}ms</span>
            </div>
            {commandResult.stdout && (
              <div>
                <p className="text-xs text-text-secondary mb-1">输出:</p>
                <pre className="bg-background p-3 rounded text-xs overflow-x-auto text-text-primary font-mono max-h-60 overflow-y-auto">
                  {commandResult.stdout}
                </pre>
              </div>
            )}
            {commandResult.stderr && (
              <div>
                <p className="text-xs text-status-warning mb-1">错误:</p>
                <pre className="bg-status-failed/5 p-3 rounded text-xs overflow-x-auto text-status-failed font-mono max-h-60 overflow-y-auto">
                  {commandResult.stderr}
                </pre>
              </div>
            )}
          </div>
        </div>
      )}

      {/* 快速命令执行区域 */}
      <div className="bg-surface border border-border rounded-lg p-6">
        <h3 className="text-lg font-semibold text-text-primary mb-4">
          在 {selectedServer.name} 上执行命令
        </h3>
        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">命令</label>
            <div className="flex gap-2">
              <input
                type="text"
                value={command}
                onChange={(e) => onCommandChange(e.target.value)}
                placeholder="输入要执行的命令..."
                className="flex-1 px-4 py-2 bg-background border border-border rounded-lg focus:outline-none focus:border-primary text-text-primary"
                disabled={isExecuting}
              />
              <button
                onClick={onExecute}
                disabled={!command || isExecuting}
                className="px-4 py-2 bg-primary text-white rounded-lg hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
              >
                {isExecuting ? (
                  <>
                    <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    执行中...
                  </>
                ) : (
                  <>
                    <Terminal className="w-4 h-4" />
                    执行
                  </>
                )}
              </button>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <span className="text-xs text-text-secondary">常用命令:</span>
            {['uname -a', 'df -h', 'free -h', 'uptime', 'whoami', 'ps aux'].map((cmd) => (
              <button
                key={cmd}
                onClick={() => onCommandChange(cmd)}
                className="px-2 py-1 bg-background border border-border rounded text-xs text-text-secondary hover:text-text-primary transition-colors"
              >
                {cmd}
              </button>
            ))}
          </div>
        </div>
      </div>
    </>
  );
}
