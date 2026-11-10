export interface Server {
  id: string;
  name: string;
  hostname: string;
  port: number;
  username: string;
  use_ssh_key: number;
  description?: string;
  tags?: string[];
  enabled: number;
  last_connected?: string;
  created_at: string;
  os?: string;
  os_type?: 'linux' | 'windows' | 'unknown';
  cpu_cores?: number;
  memory_gb?: number;
  disk_gb?: number;
  ip_address?: string;
  private_ip?: string;
  ssh_key_id?: string;
  vnc_port?: number;
  vendor?: string;
  model?: string;
  is_virtual_machine?: number;
  agent_installed?: number;
  agent_version?: string;
  agent_status?: 'online' | 'offline' | 'uninstalled';
  agent_last_heartbeat?: string;
  agent_cpu_usage?: number;
  agent_mem_mb?: number;
  agent_uptime_seconds?: number;
  groups?: Array<{ id: string; name: string }>;
}

export interface MetricChartPoint {
  time: string;
  timestamp: number;
  disk_used_gb: number;
  disk_total_gb: number;
  disk_usage_pct: number;
  disk_read_kbps: number;
  disk_write_kbps: number;
  net_rx_kbps: number;
  net_tx_kbps: number;
  load_1: number;
  load_5: number;
  load_15: number;
  cpu_usage: number;
  cpu_user: number;
  cpu_system: number;
  memory_usage: number;
  memory_total_gb: number;
  memory_used_gb: number;
}

export interface VmMetricsDashboardData {
  server_id: string;
  server_name: string;
  hostname: string;
  time_range: string;
  agent_installed: boolean;
  agent_status: string;
  agent_version: string;
  agent_cpu: number;
  agent_mem_mb: number;
  uptime_seconds: number;
  summary: {
    disk_total_gb: number;
    disk_used_gb: number;
    disk_usage_pct: number;
    memory_total_gb: number;
    memory_used_gb: number;
    memory_usage: number;
    cpu_cores: number;
    cpu_usage: number;
    cpu_user: number;
    cpu_system: number;
    load_1: number;
    load_5: number;
    load_15: number;
    disk_read_kbps: number;
    disk_write_kbps: number;
    net_rx_kbps: number;
    net_tx_kbps: number;
  };
  series: MetricChartPoint[];
}

export interface ServerVendorMeta {
  value: string;
  label: string;
  color: string;
  bg: string;
  border: string;
}

export const SERVER_VENDORS: ServerVendorMeta[] = [
  { value: 'huawei', label: '华为 (Huawei)', color: 'text-red-400', bg: 'bg-red-500/10', border: 'border-red-500/30' },
  { value: 'inspur', label: '浪潮 (Inspur)', color: 'text-blue-400', bg: 'bg-blue-500/10', border: 'border-blue-500/30' },
  { value: 'tongfang', label: '清华同方 (Tongfang)', color: 'text-purple-400', bg: 'bg-purple-500/10', border: 'border-purple-500/30' },
  { value: 'lenovo', label: '联想 (Lenovo)', color: 'text-rose-400', bg: 'bg-rose-500/10', border: 'border-rose-500/30' },
  { value: 'dell', label: '戴尔 (Dell)', color: 'text-sky-400', bg: 'bg-sky-500/10', border: 'border-sky-500/30' },
  { value: 'hpe', label: '惠普 (HPE)', color: 'text-emerald-400', bg: 'bg-emerald-500/10', border: 'border-emerald-500/30' },
  { value: 'h3c', label: '新华三 (H3C)', color: 'text-cyan-400', bg: 'bg-cyan-500/10', border: 'border-cyan-500/30' },
  { value: 'sugon', label: '中科曙光 (Sugon)', color: 'text-amber-400', bg: 'bg-amber-500/10', border: 'border-amber-500/30' },
  { value: 'supermicro', label: '超微 (Supermicro)', color: 'text-teal-400', bg: 'bg-teal-500/10', border: 'border-teal-500/30' },
  { value: 'other', label: '其他 (Other)', color: 'text-slate-400', bg: 'bg-slate-500/10', border: 'border-slate-500/30' },
];

export interface ServerGroup {
  id: string;
  name: string;
  description?: string;
  parent_id?: string;
  sort_order: number;
  server_count?: number;
  children_count?: number;
  children?: ServerGroup[];
}

export interface CommandResult {
  success: boolean;
  stdout: string;
  stderr: string;
  command: string;
  duration: number;
  aiAnalysis?: string;
}

export interface CommandHistoryItem {
  id: string;
  server_id: string;
  command: string;
  stdout: string;
  stderr: string;
  success: number;
  execution_time_ms: number;
  executed_by: string;
  executed_at: string;
}

export interface ComplianceCheck {
  id: string;
  server_id: string;
  check_name: string;
  check_results: string;
  status: string;
  started_at: string;
  completed_at: string;
  created_at: string;
}

export interface InspectionTemplateItem {
  id: string;
  name: string;
  vendor: string;
  category: string;
  deviceType: string;
  osType: string;
  description: string;
  commands: Array<{
    type: string;
    name: string;
    command: string;
    fallbacks?: string[];
    description: string;
    group: string;
  }>;
  tags: string[];
  isPreset: boolean;
  enabled: boolean;
  usageCount: number;
}

export interface ServerComplianceOptions {
  useAI: boolean;
  concurrency: number;
  inspectionType?: 'standard' | 'custom' | 'brand_auto';
  templateId?: string;
  selectedCommands?: string[];
}

