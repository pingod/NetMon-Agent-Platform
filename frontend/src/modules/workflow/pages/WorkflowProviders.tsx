
import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { 
  Plus, 
  RefreshCw, 
  Wrench, 
  Bell, 
  MessageSquare, 
  Activity, 
  Globe,
  Search,
  Play,
} from 'lucide-react';
import clsx from 'clsx';
import api from '../../../lib/api';

interface WorkflowProvider {
  id: string;
  name: string;
  type: string;
  configSchema: unknown;
}

export default function WorkflowProviders() {
  const queryClient = useQueryClient();
  const [selectedType, setSelectedType] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState('');

  const { data: providers, isLoading } = useQuery({
    queryKey: ['workflow-providers', selectedType],
    queryFn: async () => {
      const params = selectedType ? { type: selectedType } : undefined;
      const res = await api.get('/workflows/providers/list', { params });
      return res.data.data as WorkflowProvider[];
    },
  });

  const types = Array.from(new Set((providers || []).map(p => p.type)));
  const filteredProviders = (providers || []).filter(p => 
    !searchQuery || 
    p.name.toLowerCase().includes(searchQuery.toLowerCase()) || 
    p.id.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'notification': return Bell;
      case 'action': return Activity;
      case 'script': return Play;
      case 'alert': return MessageSquare;
      default: return Wrench;
    }
  };

  if (isLoading) {
    return (
      <div className="h-screen flex items-center justify-center bg-background">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
      </div>
    );
  }

  return (
    <div className="h-screen flex flex-col bg-background overflow-hidden">
      <div className="p-6 border-b border-border flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text-primary mb-2">工作流 Provider 管理</h1>
          <p className="text-text-secondary">管理和查看工作流 Provider</p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={() => queryClient.invalidateQueries({ queryKey: ['workflow-providers'] })}
            className="flex items-center gap-2 px-4 py-2 bg-surface text-text-primary rounded-lg hover:bg-surface-hover transition-all"
          >
            <RefreshCw className="w-4 h-4" />
            刷新
          </button>
        </div>
      </div>

      <div className="p-4 border-b border-border flex flex-wrap gap-3 items-center">
        <div className="flex items-center gap-2">
          <Search className="w-4 h-4 text-text-secondary" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="搜索 Provider..."
            className="px-3 py-2 bg-surface border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:border-blue-500 w-64"
          />
        </div>

        <div className="flex gap-2 items-center">
          <span className="text-sm text-text-secondary">类型:</span>
          <button
            onClick={() => setSelectedType(null)}
            className={clsx(
              "px-3 py-1.5 rounded-full text-sm font-medium transition-all",
              !selectedType
                ? "bg-blue-600 text-white"
                : "bg-surface text-text-secondary hover:bg-surface-hover"
            )}
          >
            全部
          </button>
          {types.map((type) => {
            const Icon = getTypeIcon(type);
            return (
              <button
                key={type}
                onClick={() => setSelectedType(selectedType === type ? null : type)}
                className={clsx(
                  "px-3 py-1.5 rounded-full text-sm font-medium flex items-center gap-1.5 transition-all",
                  selectedType === type
                    ? "bg-blue-600 text-white"
                    : "bg-surface text-text-secondary hover:bg-surface-hover"
                )}
              >
                <Icon className="w-3.5 h-3.5" />
                {type}
              </button>
            );
          })}
        </div>
      </div>

      <div className="flex-1 overflow-auto p-6">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {filteredProviders.map((provider) => {
            const Icon = getTypeIcon(provider.type);
            return (
              <div
                key={provider.id}
                className="bg-surface border border-border rounded-xl p-5 hover:border-border-strong transition-all"
              >
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 flex items-center justify-center rounded-lg bg-gradient-to-br from-blue-600/20 to-purple-600/20 border border-blue-500/20">
                      <Icon className="w-5 h-5 text-blue-600" />
                    </div>
                    <div>
                      <h3 className="text-lg font-semibold text-text-primary">{provider.name}</h3>
                      <span className="text-xs text-text-secondary">{provider.type}</span>
                    </div>
                  </div>
                </div>
                
                {provider.configSchema ? (
                  <div className="mb-4 p-3 bg-background rounded-lg border border-border">
                    <p className="text-xs text-text-secondary mb-1.5">配置 Schema:</p>
                    <pre className="text-xs text-text-secondary overflow-x-auto max-h-32">
                      {JSON.stringify((provider.configSchema as { properties?: unknown }).properties, null, 2)}
                    </pre>
                  </div>
                ) : null}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
