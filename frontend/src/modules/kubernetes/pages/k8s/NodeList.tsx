/* eslint-disable @typescript-eslint/no-explicit-any */
import { Box, Cpu, AlertCircle, RefreshCw } from 'lucide-react';
import clsx from 'clsx';
import type { NodeInfo } from '../Kubernetes';
import { nodeStatusColors } from '../Kubernetes';

interface NodeListProps {
  nodes: NodeInfo[];
  loading: boolean;
  error: boolean;
  onRetry: () => void;
  searchText: string;
}

const Spinner = () => (
  <div className="flex items-center justify-center py-20">
    <div className="animate-spin rounded-full h-8 w-8 border-2 border-primary border-t-transparent" />
  </div>
);

const ErrorState = ({ message, onRetry }: { message: string; onRetry: () => void }) => (
  <div className="flex flex-col items-center justify-center py-16 gap-3">
    <AlertCircle size={36} className="text-red-400" />
    <p className="text-text-secondary text-sm">{message}</p>
    <button
      onClick={onRetry}
      className="flex items-center gap-1.5 text-sm text-primary hover:text-primary/80 transition-colors"
    >
      <RefreshCw size={14} /> 重试
    </button>
  </div>
);

const EmptyState = ({ message }: { message: string }) => (
  <div className="flex flex-col items-center justify-center py-16 gap-2">
    <Box size={36} className="text-text-tertiary" />
    <p className="text-text-tertiary text-sm">{message}</p>
  </div>
);

export default function NodeList({ nodes, loading, error, onRetry, searchText }: NodeListProps) {
  const filtered = nodes.filter(n =>
    !searchText || n.name.toLowerCase().includes(searchText.toLowerCase())
  );

  return (
    <div className="p-4">
      {loading ? (
        <Spinner />
      ) : error ? (
        <ErrorState message="获取节点信息失败" onRetry={onRetry} />
      ) : filtered.length === 0 ? (
        <EmptyState message={searchText ? '无匹配的节点' : '暂无节点数据'} />
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filtered.map(node => {
            const cpuPercent = node.cpuTotal > 0 ? Math.round((node.cpuAllocated / node.cpuTotal) * 100) : 0;
            const memPercent = node.memoryTotal > 0 ? Math.round((node.memoryAllocated / node.memoryTotal) * 100) : 0;
            const podPercent = node.podsMax > 0 ? Math.round((node.podsCount / node.podsMax) * 100) : 0;
            return (
              <div key={node.name} className="bg-surface border border-border rounded-xl p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Cpu size={16} className="text-text-tertiary" />
                    <span className="text-text-primary text-sm font-medium truncate max-w-[180px]">{node.name}</span>
                  </div>
                  <span className={clsx('px-2 py-0.5 rounded text-xs font-medium', nodeStatusColors[node.status] || 'text-text-tertiary bg-surface')}>
                    {node.status}
                  </span>
                </div>

                {/* CPU */}
                <div>
                  <div className="flex justify-between text-xs text-text-tertiary mb-1">
                    <span>CPU</span>
                    <span>{node.cpuAllocated} / {node.cpuTotal} 核</span>
                  </div>
                  <div className="w-full h-2 bg-background rounded-full overflow-hidden">
                    <div
                      className="h-full rounded-full transition-all duration-500"
                      style={{
                        width: `${Math.min(cpuPercent, 100)}%`,
                        backgroundColor: cpuPercent > 80 ? '#ef4444' : cpuPercent > 60 ? '#f59e0b' : '#3b82f6',
                      }}
                    />
                  </div>
                </div>

                {/* 内存 */}
                <div>
                  <div className="flex justify-between text-xs text-text-tertiary mb-1">
                    <span>内存</span>
                    <span>{node.memoryAllocated} / {node.memoryTotal} GB</span>
                  </div>
                  <div className="w-full h-2 bg-background rounded-full overflow-hidden">
                    <div
                      className="h-full rounded-full transition-all duration-500"
                      style={{
                        width: `${Math.min(memPercent, 100)}%`,
                        backgroundColor: memPercent > 80 ? '#ef4444' : memPercent > 60 ? '#f59e0b' : '#22c55e',
                      }}
                    />
                  </div>
                </div>

                {/* Pods */}
                <div>
                  <div className="flex justify-between text-xs text-text-tertiary mb-1">
                    <span>Pods</span>
                    <span>{node.podsCount} / {node.podsMax}</span>
                  </div>
                  <div className="w-full h-2 bg-background rounded-full overflow-hidden">
                    <div
                      className="h-full rounded-full transition-all duration-500"
                      style={{
                        width: `${Math.min(podPercent, 100)}%`,
                        backgroundColor: podPercent > 80 ? '#ef4444' : podPercent > 60 ? '#f59e0b' : '#8b5cf6',
                      }}
                    />
                  </div>
                </div>

                <div className="text-xs text-text-tertiary pt-1">
                  K8s 版本：{node.kubeletVersion || '-'}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
