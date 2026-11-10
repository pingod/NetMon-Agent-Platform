/* eslint-disable @typescript-eslint/no-explicit-any */
import { Box, AlertCircle, RefreshCw } from 'lucide-react';
import clsx from 'clsx';
import type { Service } from '../Kubernetes';
import { serviceTypeColors } from '../Kubernetes';

interface ServiceListProps {
  services: Service[];
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

export default function ServiceList({ services, loading, error, onRetry, searchText }: ServiceListProps) {
  const filtered = services.filter(s =>
    !searchText || s.name.toLowerCase().includes(searchText.toLowerCase())
  );

  return (
    <div className="p-4">
      {loading ? (
        <Spinner />
      ) : error ? (
        <ErrorState message="获取 Services 失败" onRetry={onRetry} />
      ) : filtered.length === 0 ? (
        <EmptyState message={searchText ? '无匹配的 Service' : '暂无 Service 数据'} />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border text-text-tertiary">
                <th className="text-left py-3 px-3 font-medium">名称</th>
                <th className="text-left py-3 px-3 font-medium">命名空间</th>
                <th className="text-left py-3 px-3 font-medium">类型</th>
                <th className="text-left py-3 px-3 font-medium">Cluster IP</th>
                <th className="text-left py-3 px-3 font-medium">External IP</th>
                <th className="text-left py-3 px-3 font-medium">端口映射</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map(svc => (
                <tr key={`${svc.namespace}/${svc.name}`} className="border-b border-border/50 hover:bg-surface/50 transition-colors">
                  <td className="py-2.5 px-3 text-text-primary font-medium max-w-[200px] truncate">{svc.name}</td>
                  <td className="py-2.5 px-3 text-text-secondary">{svc.namespace}</td>
                  <td className="py-2.5 px-3">
                    <span className={clsx('px-2 py-0.5 rounded text-xs font-medium', serviceTypeColors[svc.type] || 'text-text-tertiary bg-surface')}>
                      {svc.type}
                    </span>
                  </td>
                  <td className="py-2.5 px-3 text-text-secondary font-mono text-xs">{svc.clusterIP || '-'}</td>
                  <td className="py-2.5 px-3 text-text-secondary">{svc.externalIP || '-'}</td>
                  <td className="py-2.5 px-3 text-text-secondary font-mono text-xs">{svc.ports || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
