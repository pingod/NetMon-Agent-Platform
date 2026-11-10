/**
 * Network 模块 API 服务层
 * 封装网络设备、SNMP、子网管理、拓扑、网络发现相关端点
 */

import api from '@/lib/api';
import type { NetworkDevice as _NetworkDeviceEntity, SnmpCredential as _SnmpCredentialEntity } from '@/types/network';

// ============================================================
// 类型定义
// ============================================================

// ── 网络设备 ──

export interface NetworkDevice {
  id: string;
  name: string;
  ip_address: string;
  vendor: string;
  model?: string;
  os_version?: string;
  ssh_port: number;
  username: string;
  location?: string;
  role?: string;
  status: string;
  last_inspection_at?: string;
  last_inspection_result?: string;
  created_at: string;
  updated_at: string;
  snmp_enabled?: number;
  snmp_credential_id?: string;
  snmp_credential_name?: string;
}

export interface NetworkDeviceInput {
  name: string;
  ip_address: string;
  vendor: string;
  model?: string;
  ssh_port: number;
  username: string;
  password?: string;
  ssh_key_id?: string;
  location?: string;
  role?: string;
  snmp_enabled?: boolean;
  snmp_credential_id?: string;
}

export interface InspectionInput {
  inspectionType: 'standard' | 'custom' | 'full';
  customDescription?: string;
}

export interface BatchInspectionInput {
  deviceIds: string[];
  inspectionType: string;
}

export interface TestConnectionResult {
  success: boolean;
  latency?: number;
  message?: string;
}

// ── SNMP 凭证 ──

export interface SnmpCredential {
  id: string;
  device_id?: string;
  name: string;
  snmp_version: string;
  snmp_port: number;
  snmp_user?: string;
  snmp_auth_protocol?: string;
  snmp_priv_protocol?: string;
  community?: string;
  host?: string;
  created_at: string;
  updated_at: string;
}

export interface SnmpCredentialInput {
  name: string;
  community?: string;
  snmp_version: string;
  snmp_port: number;
  snmp_user?: string;
  snmp_auth_protocol?: string;
  snmp_auth_key?: string;
  snmp_priv_protocol?: string;
  snmp_priv_key?: string;
  host?: string;
}

export interface SnmpTestInput {
  host: string;
  port: number;
  version: string;
  community: string;
}

export interface SnmpQueryInput {
  host: string;
  community: string;
  version: string;
}

export interface SnmpTrap {
  id: string;
  [key: string]: unknown;
}

// ── 子网管理 ──

export interface SubnetInfo {
  id: string;
  name: string;
  cidr: string;
  gateway: string | null;
  vlan_id: number | null;
  network_type: string;
  location: string | null;
  description: string | null;
  status: string;
  total_ips: number;
  used_ips: number;
  created_at: string;
}

export interface SubnetInput {
  name: string;
  cidr: string;
  gateway?: string;
  vlan_id?: number;
  network_type: string;
  location?: string;
  description?: string;
}

export interface SubnetUpdateInput {
  name: string;
  gateway?: string | null;
  vlan_id?: number | null;
  network_type: string;
  location?: string | null;
  description?: string | null;
}

export interface IpInfo {
  id: string;
  subnet_id: string;
  ip_address: string;
  status: 'available' | 'used' | 'reserved';
  device_id: string | null;
  device_name: string | null;
  mac_address: string | null;
  description: string | null;
}

export interface IpListData {
  ips: IpInfo[];
  stats: Array<{ status: string; count: number }>;
  total: number;
}

export interface BatchIpInput {
  ip_ids: string[];
  status: string;
  device_name?: string;
}

// ── 拓扑 ──

export interface TopologyNode {
  id: string;
  label: string;
  type?: string;
  [key: string]: unknown;
}

export interface TopologyEdge {
  source: string;
  target: string;
  [key: string]: unknown;
}

export interface TopologyData {
  nodes: TopologyNode[];
  edges: TopologyEdge[];
}

export interface Dependency {
  id?: string;
  source: string;
  target: string;
  type: string;
  protocol: string;
  status: 'active' | 'inactive' | 'degraded';
  call_count?: number;
  avg_latency?: number;
}

export interface DependencyInput {
  source: string;
  target: string;
  type: string;
  protocol: string;
  status?: string;
}

// ── 网络发现 ──

export interface DiscoveryJob {
  id: string;
  name: string;
  start_ip: string;
  end_ip: string;
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  total_hosts: number;
  scanned_hosts: number;
  found_devices: number;
  credential_ids: string;
  error_message?: string;
  started_at?: string;
  completed_at?: string;
  created_at: string;
}

export interface DiscoveryJobInput {
  name: string;
  start_ip: string;
  end_ip: string;
  credential_ids: string[];
}

export interface DiscoveryResult {
  id: string;
  job_id: string;
  ip_address: string;
  status: 'offline' | 'online' | 'snmp_ok' | 'snmp_fail';
  sys_name?: string;
  sys_descr?: string;
  vendor?: string;
  model?: string;
  snmp_version?: string;
  interface_count?: number;
  response_time_ms?: number;
  created_at: string;
}

export interface DiscoveryImportInput {
  result_ids: string[];
  ssh_username: string;
  ssh_password?: string;
  ssh_port: number;
}

export interface DiscoveryImportResult {
  imported: number;
  errors: string[];
}

// ============================================================
// networkApi 对象
// ============================================================

export const networkApi = {
  // ── 网络设备 ──

  /** 获取网络设备列表 */
  async listDevices(): Promise<NetworkDevice[]> {
    const { data } = await api.get('/network-devices');
    return data.data;
  },

  /** 创建网络设备 */
  async createDevice(input: NetworkDeviceInput): Promise<NetworkDevice> {
    const { data } = await api.post('/network-devices', input);
    return data.data;
  },

  /** 更新网络设备 */
  async updateDevice(id: string, input: Partial<NetworkDeviceInput>): Promise<NetworkDevice> {
    const { data } = await api.put(`/network-devices/${id}`, input);
    return data.data;
  },

  /** 删除网络设备 */
  async deleteDevice(id: string): Promise<void> {
    await api.delete(`/network-devices/${id}`);
  },

  /** 测试设备连接 */
  async testDeviceConnection(id: string): Promise<TestConnectionResult> {
    const { data } = await api.post(`/network-devices/${id}/test-connection`);
    return data;
  },

  /** 批量测试连接 */
  async testConnectionBatch(input: NetworkDeviceInput): Promise<unknown> {
    const { data } = await api.post('/network-devices/test-connection', input);
    return data;
  },

  /** SNMP 巡检 */
  async inspectSnmp(id: string): Promise<unknown> {
    const { data } = await api.post(`/network-devices/${id}/inspect-snmp`);
    return data.data;
  },

  /** 设备巡检 */
  async inspectDevice(id: string, input: InspectionInput): Promise<unknown> {
    const { data } = await api.post(`/network-devices/${id}/inspect`, input);
    return data.data;
  },

  /** 批量巡检 */
  async batchInspect(input: BatchInspectionInput): Promise<unknown[]> {
    const { data } = await api.post('/network-devices/batch-inspect', input);
    return data.data;
  },

  // ── SNMP 凭证 ──

  /** 获取 SNMP 凭证列表 */
  async listSnmpCredentials(): Promise<SnmpCredential[]> {
    const { data } = await api.get('/snmp/credentials');
    return data.data || [];
  },

  /** 创建 SNMP 凭证 */
  async createSnmpCredential(input: SnmpCredentialInput): Promise<SnmpCredential> {
    const { data } = await api.post('/snmp/credentials', input);
    return data.data;
  },

  /** 更新 SNMP 凭证 */
  async updateSnmpCredential(id: string, input: SnmpCredentialInput): Promise<SnmpCredential> {
    const { data } = await api.put(`/snmp/credentials/${id}`, input);
    return data.data;
  },

  /** 删除 SNMP 凭证 */
  async deleteSnmpCredential(id: string): Promise<void> {
    await api.delete(`/snmp/credentials/${id}`);
  },

  /** 测试 SNMP 凭证 */
  async testSnmpCredential(id: string, params?: { host?: string }): Promise<unknown> {
    const { data } = await api.post(`/snmp/credentials/${id}/test`, params);
    return data;
  },

  /** SNMP 连接测试 */
  async testSnmpConnection(input: SnmpTestInput): Promise<unknown> {
    const { data } = await api.post('/snmp/test', input);
    return data;
  },

  /** 获取 SNMP 系统信息 */
  async getSnmpSystemInfo(input: SnmpQueryInput): Promise<unknown> {
    const { data } = await api.post('/snmp/system-info', input);
    return data.data;
  },

  /** 获取 SNMP 接口信息 */
  async getSnmpInterfaces(input: SnmpQueryInput): Promise<unknown> {
    const { data } = await api.post('/snmp/interfaces', input);
    return data.data;
  },

  // ── SNMP Trap ──

  /** 获取 Trap 列表 */
  async listTraps(params?: { limit?: number }): Promise<SnmpTrap[]> {
    const { data } = await api.get('/snmp/traps', { params });
    return data.data || [];
  },

  /** 发送测试 Trap */
  async testTrap(): Promise<void> {
    await api.post('/snmp/traps/test');
  },

  // ── 子网管理 ──

  /** 获取子网列表 */
  async listSubnets(): Promise<SubnetInfo[]> {
    const { data } = await api.get('/network-subnets');
    return data.data;
  },

  /** 创建子网 */
  async createSubnet(input: SubnetInput): Promise<SubnetInfo> {
    const { data } = await api.post('/network-subnets', input);
    return data.data;
  },

  /** 更新子网 */
  async updateSubnet(id: string, input: SubnetUpdateInput): Promise<SubnetInfo> {
    const { data } = await api.put(`/network-subnets/${id}`, input);
    return data.data;
  },

  /** 删除子网 */
  async deleteSubnet(id: string): Promise<void> {
    await api.delete(`/network-subnets/${id}`);
  },

  /** 获取子网 IP 列表 */
  async listSubnetIps(subnetId: string, params?: { status?: string; search?: string; pageSize?: number }): Promise<IpListData> {
    const { data } = await api.get(`/network-subnets/${subnetId}/ips`, { params });
    return data.data;
  },

  /** 批量操作 IP */
  async batchUpdateIps(subnetId: string, input: BatchIpInput): Promise<unknown> {
    const { data } = await api.post(`/network-subnets/${subnetId}/ips/batch`, input);
    return data;
  },

  // ── 拓扑 ──

  /** 获取全局拓扑 */
  async getGlobalTopology(): Promise<TopologyData> {
    const { data } = await api.get('/topology/global');
    return data.data;
  },

  /** 获取依赖关系列表 */
  async listDependencies(): Promise<Dependency[]> {
    const { data } = await api.get('/topology/dependency');
    return data.data;
  },

  /** 创建依赖关系 */
  async createDependency(input: DependencyInput): Promise<Dependency> {
    const { data } = await api.post('/topology/dependency', input);
    return data.data;
  },

  /** 删除依赖关系 */
  async deleteDependency(dependencyId: string): Promise<void> {
    await api.delete(`/topology/dependency/${dependencyId}`);
  },

  /** 发现服务器拓扑 */
  async discoverTopology(serverId: string): Promise<void> {
    await api.post(`/topology/discover/${serverId}`);
  },

  // ── 网络发现 ──

  /** 获取发现任务列表 */
  async listDiscoveryJobs(): Promise<DiscoveryJob[]> {
    const { data } = await api.get('/network-discovery/jobs');
    return data.data || [];
  },

  /** 创建发现任务 */
  async createDiscoveryJob(input: DiscoveryJobInput): Promise<DiscoveryJob> {
    const { data } = await api.post('/network-discovery/jobs', input);
    return data.data;
  },

  /** 取消发现任务 */
  async cancelDiscoveryJob(jobId: string): Promise<void> {
    await api.post(`/network-discovery/jobs/${jobId}/cancel`);
  },

  /** 删除发现任务 */
  async deleteDiscoveryJob(jobId: string): Promise<void> {
    await api.delete(`/network-discovery/jobs/${jobId}`);
  },

  /** 获取发现结果 */
  async listDiscoveryResults(params?: { jobId?: string; limit?: number }): Promise<{ data: DiscoveryResult[]; total: number }> {
    const { data } = await api.get('/network-discovery/results', { params });
    return { data: data.data, total: data.total };
  },

  /** 导入发现的设备 */
  async importDiscoveryResults(input: DiscoveryImportInput): Promise<DiscoveryImportResult> {
    const { data } = await api.post('/network-discovery/import', input);
    return data.data;
  },

  // ── 配置备份 ──

  /** 获取备份模板列表 */
  async listBackupTemplates(): Promise<ConfigBackupTemplate[]> {
    const { data } = await api.get('/config-backup/templates');
    return data.data || [];
  },

  /** 创建备份模板 */
  async createBackupTemplate(input: ConfigBackupTemplateInput): Promise<ConfigBackupTemplate> {
    const { data } = await api.post('/config-backup/templates', input);
    return data.data;
  },

  /** 更新备份模板 */
  async updateBackupTemplate(id: string, input: Partial<ConfigBackupTemplateInput>): Promise<ConfigBackupTemplate> {
    const { data } = await api.put(`/config-backup/templates/${id}`, input);
    return data.data;
  },

  /** 删除备份模板 */
  async deleteBackupTemplate(id: string): Promise<void> {
    await api.delete(`/config-backup/templates/${id}`);
  },

  /** 恢复默认备份模板 */
  async restoreBackupTemplates(): Promise<ConfigBackupTemplate[]> {
    const { data } = await api.post('/config-backup/templates/restore-presets');
    return data.data || [];
  },

  /** 获取备份任务列表 */
  async listBackupTasks(): Promise<ConfigBackupTask[]> {
    const { data } = await api.get('/config-backup/tasks');
    return data.data || [];
  },

  /** 创建备份任务 */
  async createBackupTask(input: ConfigBackupTaskInput): Promise<ConfigBackupTask> {
    const { data } = await api.post('/config-backup/tasks', input);
    return data.data;
  },

  /** 更新备份任务 */
  async updateBackupTask(id: string, input: Partial<ConfigBackupTaskInput>): Promise<ConfigBackupTask> {
    const { data } = await api.put(`/config-backup/tasks/${id}`, input);
    return data.data;
  },

  /** 删除备份任务 */
  async deleteBackupTask(id: string): Promise<void> {
    await api.delete(`/config-backup/tasks/${id}`);
  },

  /** 启用/禁用备份任务 */
  async toggleBackupTask(id: string): Promise<ConfigBackupTask> {
    const { data } = await api.post(`/config-backup/tasks/${id}/toggle`);
    return data.data;
  },

  /** 手动执行备份任务 */
  async executeBackupTask(id: string): Promise<BackupDeviceResult[]> {
    const { data } = await api.post(`/config-backup/tasks/${id}/execute`);
    return data.data;
  },

  /** 批量备份（选择设备+模板） */
  async batchBackup(deviceIds: string[], templateId: string): Promise<BackupDeviceResult[]> {
    const { data } = await api.post('/config-backup/batch-execute', { device_ids: deviceIds, template_id: templateId });
    return data.data;
  },

  /** 获取备份记录列表 */
  async listBackupRecords(params?: { task_id?: string; device_id?: string; status?: string; limit?: number; offset?: number }): Promise<{ records: ConfigBackupRecord[]; total: number }> {
    const { data } = await api.get('/config-backup/records', { params });
    return data.data;
  },

  /** 获取单条备份记录 */
  async getBackupRecord(id: string): Promise<ConfigBackupRecord> {
    const { data } = await api.get(`/config-backup/records/${id}`);
    return data.data;
  },

  /** 删除备份记录 */
  async deleteBackupRecord(id: string): Promise<void> {
    await api.delete(`/config-backup/records/${id}`);
  },
};

export default networkApi;

// ============================================================
// 配置备份类型定义
// ============================================================

export interface ConfigBackupTemplate {
  id: string;
  name: string;
  description: string | null;
  vendor: string;
  commands: string[];
  is_builtin: number;
  created_by: string | null;
  created_at: string;
  updated_at: string;
}

export interface ConfigBackupTemplateInput {
  name: string;
  description?: string;
  vendor: string;
  commands: string[];
}

export interface ConfigBackupTask {
  id: string;
  name: string;
  template_id: string;
  schedule_type: 'manual' | 'daily' | 'weekly' | 'monthly' | 'cron';
  schedule_cron: string | null;
  device_ids: string[];
  enabled: number;
  last_run_at: string | null;
  last_run_status: string | null;
  next_run_at: string | null;
  retention_days: number;
  created_by: string | null;
  created_at: string;
  updated_at: string;
  template_name?: string;
  device_count?: number;
}

export interface ConfigBackupTaskInput {
  name: string;
  template_id: string;
  schedule_type: 'manual' | 'daily' | 'weekly' | 'monthly' | 'cron';
  schedule_cron?: string;
  device_ids: string[];
  enabled?: boolean;
  retention_days?: number;
}

export interface ConfigBackupRecord {
  id: string;
  task_id: string | null;
  task_name: string | null;
  device_id: string;
  device_name: string;
  device_ip: string;
  device_vendor: string;
  template_id: string | null;
  template_name: string | null;
  status: 'running' | 'success' | 'failed' | 'partial';
  commands: string | null;
  output: string | null;
  output_size: number;
  duration_ms: number | null;
  error_message: string | null;
  started_at: string;
  completed_at: string | null;
}

export interface BackupDeviceResult {
  deviceId: string;
  deviceName: string;
  deviceIp: string;
  status: 'success' | 'failed';
  output?: string;
  outputSize?: number;
  durationMs?: number;
  error?: string;
}
