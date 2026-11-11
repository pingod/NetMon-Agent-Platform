/* eslint-disable @typescript-eslint/no-explicit-any */
import { useState, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Plus, Trash2, RefreshCw, CheckCircle2,
  Zap, Wifi, X, Loader2, Network, Search, ClipboardCheck,
  AlertTriangle, AlertCircle, CheckSquare, Square, Database, Activity,
  Upload, Download, Sliders, Lock, Layers, Sparkles,
  ChevronDown, ChevronUp, Cpu, Shield, Check, Settings2
} from 'lucide-react';
import api from '../../../lib/api';
import { useToast } from '../../../contexts/ToastContext';
import { useAuth } from '../../../contexts/AuthContext';
import AddDeviceModal from '../../../modules/infra/components/AddDeviceModal';
import NetworkDeviceCard from '../../../modules/network/components/NetworkDeviceCard';
import PollSettingsModal from '../../../modules/network/components/PollSettingsModal';
import BatchPollSettingsModal from '../../../modules/network/components/BatchPollSettingsModal';
import InspectionResult from '../../../modules/alerts/components/InspectionResult';
import SnmpInspectionResult from '../../../modules/network/components/SnmpInspectionResult';
import InspectionHistory from '../../../modules/alerts/components/InspectionHistory';
import WebTerminal from '../../../modules/servers/components/WebTerminal';
import { useEscapeKey } from '../../../hooks/useEscapeKey';
import { safeFormatDistance as _safeFormatDistance } from '../../../lib/date';
import { getAxiosErrorMessage } from '@/lib/errorHandler';

interface NetworkDevice {
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
  snmp_port?: number;
  system_poll_interval?: number;
  interface_poll_interval?: number;
}

interface InspectionResultData {
  _deviceName?: string;
  inspectionId: string;
  deviceId: string;
  inspectionType: 'standard' | 'custom' | 'full';
  status: 'success' | 'partial' | 'failed';
  results: Array<{
    type: string;
    success: boolean;
    value?: number | string;
    unit?: string;
    status: 'normal' | 'warning' | 'critical' | 'error';
    details: string;
    rawOutput: string;
    timestamp: string;
  }>;
  commandsExecuted: number;
  commandsFailed: number;
  durationMs: number;
  summary: string;
  [key: string]: unknown;
}

interface SnmpInterfaceMetric {
  index: number;
  name: string;
  operStatus: 'up' | 'down';
  adminStatus: 'up' | 'down';
  speed: number;
  mtu: number;
  mac: string;
  inBps: number;
  outBps: number;
  inUtilization: number;
  outUtilization: number;
  inErrors: number;
  outErrors: number;
}

interface SnmpInspectionData {
  _deviceName?: string;
  reachable: boolean;
  sysName: string;
  sysDescr: string;
  sysUptime: number;
  interfaces: SnmpInterfaceMetric[];
  interfaceCount: number;
  upCount: number;
  downCount: number;
  alerts: string[];
  pollDurationMs: number;
  [key: string]: unknown;
}

interface TimelineItem {
  id?: string;
  device_id?: string;
  source?: string;
  created_at?: string;
  [key: string]: unknown;
}

interface DeviceTimelineEntry {
  lastAnalysis?: TimelineItem;
  lastInspection?: TimelineItem;
  lastExecution?: TimelineItem;
}

interface InspectionDimension {
  key: string;
  name: string;
  category: 'hardware' | 'network' | 'system';
  description: string;
}

const ALL_INSPECTION_DIMENSIONS: InspectionDimension[] = [
  // 硬件与性能
  { key: 'cpu', name: 'CPU 使用率', category: 'hardware', description: '检查控制平面与数据平面 CPU 负载与阈值' },
  { key: 'memory', name: '内存使用率', category: 'hardware', description: '检查设备物理内存消耗与缓冲区状态' },
  { key: 'power', name: '电源状态', category: 'hardware', description: '监控主备双电源在位、供电与冗余状态' },
  { key: 'fan', name: '风扇状态', category: 'hardware', description: '监控风扇转速、风道与散热模块健康' },
  { key: 'environment', name: '环境与温度', category: 'hardware', description: '监控整机板卡温度传感器与告警阈值' },
  { key: 'optic', name: '光模块信息', category: 'hardware', description: '检查收发光功率、波长与衰减告警 (DDM)' },

  // 网络与链路
  { key: 'interface', name: '接口状态', category: 'network', description: '统计端口 Physical / Protocol 状态与错包' },
  { key: 'routes', name: '系统路由表', category: 'network', description: '检查全局路由表与各 VPN 实例路由前缀' },
  { key: 'arp', name: 'ARP 映射表', category: 'network', description: '检查 IP-MAC 解析表项数量与网关可达性' },
  { key: 'mac', name: 'MAC 地址表', category: 'network', description: '检查交换机二层 MAC 动态漂移与学习容量' },
  { key: 'vlan', name: 'VLAN 配置', category: 'network', description: '检查全局 VLAN 划分与 Trunk 透传列表' },
  { key: 'stp', name: 'STP 生成树', category: 'network', description: '检查根桥选举、端口角色阻塞与拓扑震荡' },
  { key: 'neighbor', name: 'LLDP 邻居', category: 'network', description: '检测链路层拓扑对端设备互联与接口对齐' },

  // 系统与安全
  { key: 'version', name: '系统与版本', category: 'system', description: '检查系统固件版本、运行时间与补丁状态' },
  { key: 'log', name: '系统日志', category: 'system', description: '检索最新 Logbuffer 告警、异常与致命错误' },
  { key: 'security_policy', name: '安全策略', category: 'system', description: '检查防火墙访问控制列表 (ACL) 与规则命中' },
  { key: 'nat', name: 'NAT 会话', category: 'system', description: '监控网络地址转换会话池与端口耗尽情况' },
  { key: 'vpn', name: 'VPN 隧道', category: 'system', description: '检查 IPSec / SSL VPN 隧道协商与连通状态' },
  { key: 'ospf', name: 'OSPF 状态', category: 'system', description: '检查 OSPF 邻居 Full 状态与链路状态数据库' },
  { key: 'bgp', name: 'BGP 邻居', category: 'system', description: '检查外部 BGP 对等体 Established 与路由收敛' },
  { key: 'ntp', name: 'NTP 时钟', category: 'system', description: '检查网络时间协议同步状态与时钟源层级' },
  { key: 'license', name: 'License 授权', category: 'system', description: '检查特权特性、接口速率与设备授权有效期' },
  { key: 'config_checksum', name: '配置快照', category: 'system', description: '获取当前配置变更摘要与最后保存时间' },
];

export default function NetworkDevices() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const { token } = useAuth();
  const [isAddModalOpen, setIsAddModalOpen] = useState(false);
  const [editingDevice, setEditingDevice] = useState<NetworkDevice | null>(null);
  const [selectedVendor, setSelectedVendor] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [inspectionResult, setInspectionResult] = useState<InspectionResultData | null>(null);
  const [snmpInspectionResult, setSnmpInspectionResult] = useState<SnmpInspectionData | null>(null);
  const [showInspectionModal, setShowInspectionModal] = useState(false);
  const [inspectingDevice, setInspectingDevice] = useState<NetworkDevice | null>(null);
  const [inspectionType, setInspectionType] = useState<'standard' | 'custom' | 'full'>('standard');
  const [customDescription, setCustomDescription] = useState('');
  const [isInspecting, setIsInspecting] = useState(false);
  const [showHistory, setShowHistory] = useState<NetworkDevice | null>(null);
  const [selectedDevices, setSelectedDevices] = useState<Set<string>>(new Set());
  const [isBatchInspecting, setIsBatchInspecting] = useState(false);
  const [showBatchModal, setShowBatchModal] = useState(false);
  const [showBatchDeleteModal, setShowBatchDeleteModal] = useState(false);
  const [isBatchDeleting, setIsBatchDeleting] = useState(false);
  const [deleteConfirmDevice, setDeleteConfirmDevice] = useState<NetworkDevice | null>(null);
  const [webShellDevice, setWebShellDevice] = useState<NetworkDevice | null>(null);

  // 批量巡检模式配置
  const [batchInspectionType, setBatchInspectionType] = useState<'standard' | 'custom' | 'full'>('standard');
  const [batchCustomMode, setBatchCustomMode] = useState<'dimension' | 'template'>('dimension');
  const [batchSelectedDimensions, setBatchSelectedDimensions] = useState<Set<string>>(
    () => new Set(['cpu', 'memory', 'interface', 'routes', 'environment', 'power', 'fan', 'log'])
  );
  const [batchTemplateId, setBatchTemplateId] = useState<string>('');
  const [batchSelectedCommands, setBatchSelectedCommands] = useState<Set<string>>(new Set());
  const [batchTemplates, setBatchTemplates] = useState<any[]>([]);
  const [isLoadingBatchTemplates, setIsLoadingBatchTemplates] = useState(false);
  const [batchConcurrency, setBatchConcurrency] = useState<number>(5);
  const [showBatchDeviceList, setShowBatchDeviceList] = useState(false);

  // 自定义模版与同品牌联动巡检状态
  const [deviceTemplates, setDeviceTemplates] = useState<any[]>([]);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('');
  const [selectedCommandNames, setSelectedCommandNames] = useState<Set<string>>(new Set());
  const [isLoadingTemplates, setIsLoadingTemplates] = useState(false);

  // MIB 采集相关状态
  const [mibCollectDevice, setMibCollectDevice] = useState<NetworkDevice | null>(null);
  const [mibCollectResults, setMibCollectResults] = useState<any[] | null>(null);
  const [isMibCollecting, setIsMibCollecting] = useState(false);
  const [selectedMibModuleId, setSelectedMibModuleId] = useState<string>('');

  // 批量导入相关状态
  const [showImportModal, setShowImportModal] = useState(false);
  const [importFile, setImportFile] = useState<File | null>(null);
  const [isImporting, setIsImporting] = useState(false);
  const [importResult, setImportResult] = useState<any | null>(null);

  // Ping 测全部网络设备状态
  const [isPingChecking, setIsPingChecking] = useState(false);

  // 采集周期设置状态
  const [pollSettingsDevice, setPollSettingsDevice] = useState<NetworkDevice | null>(null);
  const [showBatchPollModal, setShowBatchPollModal] = useState(false);
  const [testingConnectionDeviceId, setTestingConnectionDeviceId] = useState<string | null>(null);
  const [testingSnmpDeviceId, setTestingSnmpDeviceId] = useState<string | null>(null);

  const handlePingAllDevices = async () => {
    if (isPingChecking) return;
    setIsPingChecking(true);
    try {
      const res = await api.post('/network-devices/ping-all');
      const data = res.data?.data || res.data;
      toast.success(data?.message || `已对网络设备完成 Ping 测：${data?.online || 0} 台在线，${data?.offline || 0} 台离线`);
      await queryClient.invalidateQueries({ queryKey: ['network-devices'] });
      await queryClient.invalidateQueries({ queryKey: ['network-device-stats'] });
      await queryClient.invalidateQueries({ queryKey: ['network-dashboard-stats'] });
    } catch (err: any) {
      toast.error(err.response?.data?.error || err.message || 'Ping 测网络设备失败');
    } finally {
      setIsPingChecking(false);
    }
  };

  const handleBatchDelete = () => {
    if (selectedDevices.size === 0) {
      toast.error('请先选择要删除的设备');
      return;
    }
    setShowBatchDeleteModal(true);
  };

  const confirmBatchDelete = async () => {
    if (selectedDevices.size === 0) return;
    setIsBatchDeleting(true);
    try {
      const res = await api.post('/network-devices/batch-delete', {
        device_ids: Array.from(selectedDevices),
      });
      const data = res.data?.data || res.data;
      toast.success(data?.message || `已成功批量删除 ${selectedDevices.size} 台网络设备`);
      setSelectedDevices(new Set());
      setShowBatchDeleteModal(false);
      await queryClient.invalidateQueries({ queryKey: ['network-devices'] });
      await queryClient.invalidateQueries({ queryKey: ['network-device-stats'] });
      await queryClient.invalidateQueries({ queryKey: ['network-dashboard-stats'] });
    } catch (err: any) {
      toast.error(err.response?.data?.error || err.message || '批量删除网络设备失败');
    } finally {
      setIsBatchDeleting(false);
    }
  };

  // ESC key support for modals
  useEscapeKey({ onEscape: () => { setShowInspectionModal(false); setInspectingDevice(null); }, enabled: showInspectionModal });
  useEscapeKey({ onEscape: () => setShowBatchModal(false), enabled: showBatchModal });
  useEscapeKey({ onEscape: () => setShowBatchDeleteModal(false), enabled: showBatchDeleteModal });
  useEscapeKey({ onEscape: () => { setDeleteConfirmDevice(null); }, enabled: !!deleteConfirmDevice });
  useEscapeKey({ onEscape: () => { setInspectionResult(null); setInspectingDevice(null); }, enabled: !!inspectionResult });
  useEscapeKey({ onEscape: () => { setSnmpInspectionResult(null); }, enabled: !!snmpInspectionResult });
  useEscapeKey({ onEscape: () => setShowHistory(null), enabled: !!showHistory });
  useEscapeKey({ onEscape: () => setWebShellDevice(null), enabled: !!webShellDevice });
  useEscapeKey({ onEscape: () => { setMibCollectDevice(null); setMibCollectResults(null); }, enabled: !!mibCollectDevice });
  useEscapeKey({ onEscape: () => { setShowImportModal(false); setImportResult(null); setImportFile(null); }, enabled: showImportModal });

  // MIB 模块列表
  const { data: mibModules = [] } = useQuery({
    queryKey: ['mib-modules'],
    queryFn: () => api.get('/snmp-collect/mib-modules').then(r => (r.data.data || r.data) as any[]),
    enabled: !!mibCollectDevice,
  });

  const { data: devices = [], isLoading } = useQuery({
    queryKey: ['network-devices'],
    queryFn: () => api.get('/network-devices').then(res => res.data.data)
  });

  // 网络设备统计（吞吐率、接口状态等）
  const { data: deviceStats } = useQuery({
    queryKey: ['network-device-stats'],
    queryFn: () => api.get('/dashboard/network-device-stats').then(res => res.data.data),
    refetchInterval: 60000,
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.delete(`/network-devices/${id}`),
    onSuccess: () => {
      toast.success('设备删除成功');
      queryClient.invalidateQueries({ queryKey: ['network-devices'] });
    },
    onError: () => toast.error('删除设备失败')
  });

  const handleDelete = (device: NetworkDevice) => {
    setDeleteConfirmDevice(device);
  };

  const confirmDelete = () => {
    if (deleteConfirmDevice) {
      deleteMutation.mutate(deleteConfirmDevice.id);
      setDeleteConfirmDevice(null);
    }
  };

  const handleEdit = (device: NetworkDevice) => {
    setEditingDevice(device);
    setIsAddModalOpen(true);
  };

  const loadDeviceTemplates = async (vendor: string) => {
    setIsLoadingTemplates(true);
    try {
      const res = await api.get('/inspection-templates', {
        params: { category: 'network', vendor: (vendor || '').toLowerCase() },
      });
      let tpls = res.data.data || [];
      // 如果没有完全精确匹配的，尝试加载通用模版
      if (tpls.length === 0) {
        const fallbackRes = await api.get('/inspection-templates', {
          params: { category: 'network' },
        });
        tpls = (fallbackRes.data.data || []).filter((t: any) => t.vendor === (vendor || '').toLowerCase() || t.vendor === 'other');
      }
      setDeviceTemplates(tpls);
      if (tpls.length > 0) {
        setSelectedTemplateId(tpls[0].id);
        const names = (tpls[0].commands || []).map((c: any) => c.name || c.command);
        setSelectedCommandNames(new Set(names));
      } else {
        setSelectedTemplateId('');
        setSelectedCommandNames(new Set());
      }
    } catch {
      toast.error('加载厂商巡检模版失败');
    } finally {
      setIsLoadingTemplates(false);
    }
  };

  const handleSelectTemplate = (templateId: string) => {
    setSelectedTemplateId(templateId);
    const tpl = deviceTemplates.find((t: any) => t.id === templateId);
    if (tpl) {
      const names = (tpl.commands || []).map((c: any) => c.name || c.command);
      setSelectedCommandNames(new Set(names));
    }
  };

  const toggleCommandSelection = (cmdName: string) => {
    const next = new Set(selectedCommandNames);
    if (next.has(cmdName)) {
      next.delete(cmdName);
    } else {
      next.add(cmdName);
    }
    setSelectedCommandNames(next);
  };

  const handleInspect = (device: NetworkDevice, type: 'standard' | 'custom' | 'full' = 'standard') => {
    setInspectingDevice(device);
    setInspectionType(type);
    setCustomDescription('');
    loadDeviceTemplates(device.vendor);
    setShowInspectionModal(true);
  };

  const handleSnmpInspect = async (device: NetworkDevice) => {
    try {
      const response = await api.post(`/network-devices/${device.id}/inspect-snmp`);
      const data = response.data.data;
      data._deviceName = device.name;
      setSnmpInspectionResult(data);
      queryClient.invalidateQueries({ queryKey: ['network-devices'] });
    } catch (_error) {
      toast.error('SNMP 巡检失败: ' + ((_error as { response?: { data?: { error?: string } }; message?: string }).response?.data?.error || (_error as { message?: string }).message || '未知错误'));
    }
  };

  const handleSnmpTestConnection = async (device: NetworkDevice) => {
    if (!device.snmp_credential_id) {
      toast.error('该设备未关联 SNMP 凭证');
      return;
    }
    setTestingSnmpDeviceId(device.id);
    try {
      const response = await api.post(`/snmp/credentials/${device.snmp_credential_id}/test`, {
        host: device.ip_address,
        port: device.snmp_port || 161,
      });
      const data = response.data?.data;
      const reachable = 
        data?.reachable === true || 
        data?.data?.reachable === true || 
        data?.success === true || 
        (data?.code === 0 && response.data?.success !== false);
      if (reachable) {
        toast.success('SNMP 连接成功 ✅');
      } else {
        toast.error('SNMP 连接失败: ' + (data?.message || response.data?.message || '探测超时或无响应'));
      }
    } catch (error) {
      toast.error('SNMP 测试失败: ' + (((error as { response?: { data?: { message?: string } } }).response?.data?.message || (error as { message?: string }).message) || ''));
    } finally {
      setTestingSnmpDeviceId(null);
    }
  };

  const handleTestConnection = async (device: NetworkDevice) => {
    setTestingConnectionDeviceId(device.id);
    try {
      toast.info(`正在测试 ${device.name} 的连接...`);
      const response = await api.post(`/network-devices/${device.id}/test-connection`);
      const result = response.data;
      
      if (result.success) {
        toast.success(`连接成功 (${result.data.latency}ms)`);
      } else {
        toast.error(`连接失败: ${result.data.message}`);
      }
    } catch {
      toast.error('测试连接失败');
    } finally {
      setTestingConnectionDeviceId(null);
    }
  };

  const handleHistory = (device: NetworkDevice) => {
    setShowHistory(device);
  };

  const handleWebShell = (device: NetworkDevice) => {
    if (!device.ssh_port && !device.username) {
      toast.error('该设备未配置 SSH 凭据，无法打开终端');
      return;
    }
    setWebShellDevice(device);
  };

  // MIB 采集
  const handleMibCollect = (device: NetworkDevice) => {
    setMibCollectDevice(device);
    setMibCollectResults(null);
    setSelectedMibModuleId('');
  };

  const executeMibCollect = async () => {
    if (!mibCollectDevice || !selectedMibModuleId) return;

    setIsMibCollecting(true);
    try {
      // 获取选中模块的所有 OID
      const objectsRes = await api.get(`/snmp-collect/mib-modules/${selectedMibModuleId}/objects`);
      const objects = objectsRes.data.data || objectsRes.data || [];

      if (!objects || objects.length === 0) {
        toast.error('该模块没有可采集的 OID');
        setIsMibCollecting(false);
        return;
      }

      // 按 OID 去重（华为 Excel 可能导致同一 OID 多行记录），然后限制最多 50 个
      const uniqueOidMap = new Map<string, { oid: string; metric_name: string; is_table: number }>();
      for (const obj of objects) {
        const key = obj.oid;
        if (!uniqueOidMap.has(key)) {
          uniqueOidMap.set(key, { oid: obj.oid, metric_name: obj.name, is_table: obj.is_table || 0 });
        }
      }
      const oids = Array.from(uniqueOidMap.values()).slice(0, 50);

      // 调用快速采集 API
      const response = await api.post('/snmp-collect/quick-test', {
        device_ids: [mibCollectDevice.id],
        oids,
        timeout_ms: 15000,
      });

      const results = response.data.data || [];
      setMibCollectResults(results);
      if (results.length > 0 && results[0].error) {
        toast.warning(`MIB 采集完成，但有错误: ${results[0].error}`);
      } else if (results.length > 0 && results[0].metrics?.some((m: any) => m.metric_value && m.metric_value !== 'N/A')) {
        toast.success(`MIB 采集完成，获取 ${results[0].metrics?.filter((m: any) => m.metric_value && m.metric_value !== 'N/A').length || 0} 个指标`);
      } else {
        toast.warning('MIB 采集完成，但部分指标未获取到数据');
      }
    } catch (err: any) {
      const msg = err?.response?.data?.error || err?.message || '采集失败';
      toast.error(`MIB 采集失败: ${msg}`);
    } finally {
      setIsMibCollecting(false);
    }
  };

  // 批量导入
  const handleImport = async () => {
    if (!importFile) return;
    setIsImporting(true);
    setImportResult(null);
    try {
      const formData = new FormData();
      formData.append('file', importFile);
      const resp = await api.post('/network-devices/import', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      setImportResult(resp.data.data);
      queryClient.invalidateQueries({ queryKey: ['network-devices'] });
      if (resp.data.data.imported > 0) {
        toast.success(`成功导入 ${resp.data.data.imported} 台设备`);
      }
    } catch (err: any) {
      const msg = err?.response?.data?.error || err?.message || '导入失败';
      toast.error(`导入失败: ${msg}`);
    } finally {
      setIsImporting(false);
    }
  };

  const handleDownloadTemplate = async () => {
    try {
      const resp = await api.get('/network-devices/import/template', { responseType: 'blob' });
      const url = window.URL.createObjectURL(new Blob([resp.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', 'network_devices_import_template.xlsx');
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);
    } catch {
      toast.error('下载模板失败');
    }
  };

  const executeInspection = async () => {
    if (!inspectingDevice) return;

    setIsInspecting(true);
    try {
      const payload: Record<string, unknown> = {
        inspectionType,
      };
      if (inspectionType === 'custom') {
        payload.templateId = selectedTemplateId || undefined;
        payload.selectedCommands = Array.from(selectedCommandNames);
        payload.customDescription = customDescription || undefined;
      }

      const response = await api.post(`/network-devices/${inspectingDevice.id}/inspect`, payload);

      setInspectionResult(response.data.data);
      toast.success('巡检完成');
      setShowInspectionModal(false);
      queryClient.invalidateQueries({ queryKey: ['network-devices'] });
    } catch (error: unknown) {
      toast.error('巡检失败: ' + getAxiosErrorMessage(error));
    } finally {
      setIsInspecting(false);
    }
  };

  const toggleDeviceSelection = (id: string) => {
    const newSelected = new Set(selectedDevices);
    if (newSelected.has(id)) {
      newSelected.delete(id);
    } else {
      newSelected.add(id);
    }
    setSelectedDevices(newSelected);
  };

  const selectAllDevices = () => {
    if (selectedDevices.size === filteredDevices.length) {
      setSelectedDevices(new Set());
    } else {
      setSelectedDevices(new Set(filteredDevices.map((d: NetworkDevice) => d.id)));
    }
  };

  const loadBatchTemplates = async (vendor: string) => {
    setIsLoadingBatchTemplates(true);
    try {
      const res = await api.get('/inspection-templates', {
        params: { category: 'network', vendor: (vendor || '').toLowerCase() },
      });
      let tpls = res.data.data || [];
      if (tpls.length === 0) {
        const fallbackRes = await api.get('/inspection-templates', {
          params: { category: 'network' },
        });
        tpls = (fallbackRes.data.data || []).filter((t: any) => t.vendor === (vendor || '').toLowerCase() || t.vendor === 'other');
      }
      setBatchTemplates(tpls);
      if (tpls.length > 0) {
        setBatchTemplateId(tpls[0].id);
        const names = (tpls[0].commands || []).map((c: any) => c.name || c.command);
        setBatchSelectedCommands(new Set(names));
      } else {
        setBatchTemplateId('');
        setBatchSelectedCommands(new Set());
      }
    } catch {
      // fallback silently
    } finally {
      setIsLoadingBatchTemplates(false);
    }
  };

  const handleSelectBatchTemplate = (templateId: string) => {
    setBatchTemplateId(templateId);
    const tpl = batchTemplates.find((t: any) => t.id === templateId);
    if (tpl) {
      const names = (tpl.commands || []).map((c: any) => c.name || c.command);
      setBatchSelectedCommands(new Set(names));
    }
  };

  const toggleBatchCommandSelection = (cmdKey: string) => {
    const next = new Set(batchSelectedCommands);
    if (next.has(cmdKey)) {
      next.delete(cmdKey);
    } else {
      next.add(cmdKey);
    }
    setBatchSelectedCommands(next);
  };

  const toggleBatchDimension = (dimKey: string) => {
    const next = new Set(batchSelectedDimensions);
    if (next.has(dimKey)) {
      next.delete(dimKey);
    } else {
      next.add(dimKey);
    }
    setBatchSelectedDimensions(next);
  };

  const selectAllDimensions = () => {
    setBatchSelectedDimensions(new Set(ALL_INSPECTION_DIMENSIONS.map(d => d.key)));
  };

  const selectCoreDimensions = () => {
    setBatchSelectedDimensions(new Set(['cpu', 'memory', 'interface', 'routes', 'environment', 'power', 'fan', 'log']));
  };

  const clearDimensions = () => {
    setBatchSelectedDimensions(new Set());
  };

  const handleBatchInspect = () => {
    if (selectedDevices.size === 0) {
      toast.error('请至少选择一台设备');
      return;
    }
    const selList = devices.filter((d: NetworkDevice) => selectedDevices.has(d.id));
    const vSet = new Set<string>(selList.map((d: NetworkDevice) => (d.vendor || 'other').toLowerCase()));
    const vArr: string[] = Array.from(vSet);
    if (vArr.length === 1 && vArr[0] !== 'other') {
      loadBatchTemplates(vArr[0]);
    } else {
      loadBatchTemplates('other');
    }
    setShowBatchModal(true);
  };

  const executeBatchInspection = async () => {
    if (selectedDevices.size === 0) return;

    if (batchInspectionType === 'custom') {
      if (batchCustomMode === 'dimension' && batchSelectedDimensions.size === 0) {
        toast.error('请至少勾选一个巡检指标维度');
        return;
      }
      if (batchCustomMode === 'template' && (!batchTemplateId || batchSelectedCommands.size === 0)) {
        toast.error('请选择有效的巡检模版并至少勾选一条指令');
        return;
      }
    }

    setIsBatchInspecting(true);
    try {
      const payload: any = {
        deviceIds: Array.from(selectedDevices),
        inspectionType: batchInspectionType,
        concurrency: batchConcurrency,
      };

      if (batchInspectionType === 'custom') {
        if (batchCustomMode === 'dimension') {
          payload.customTypes = Array.from(batchSelectedDimensions);
        } else if (batchCustomMode === 'template' && batchTemplateId) {
          payload.templateId = batchTemplateId;
          payload.selectedCommands = Array.from(batchSelectedCommands);
        }
      }

      const response = await api.post('/network-devices/batch-inspect', payload);
      const resData = response.data?.data || response.data;
      const successCount = resData?.success_count ?? (Array.isArray(resData) ? resData.length : selectedDevices.size);
      const failedCount = resData?.failed_count ?? 0;

      toast.success(
        `批量巡检已完成！成功 ${successCount} 台${failedCount > 0 ? `，失败 ${failedCount} 台` : ''}。系统已自动汇总生成巡检报告！`
      );
      queryClient.invalidateQueries({ queryKey: ['network-devices'] });
      queryClient.invalidateQueries({ queryKey: ['network-device-stats'] });
      setSelectedDevices(new Set());
      setShowBatchModal(false);
    } catch (error) {
      toast.error('批量巡检失败: ' + ((error as { response?: { data?: { error?: string } }; message?: string }).response?.data?.error || (error as { message?: string }).message || '未知错误'));
    } finally {
      setIsBatchInspecting(false);
    }
  };

  const batchSelectedDeviceList = useMemo(() => {
    return devices.filter((d: NetworkDevice) => selectedDevices.has(d.id));
  }, [devices, selectedDevices]);

  const batchVendorStats = useMemo(() => {
    const counts: Record<string, number> = {};
    batchSelectedDeviceList.forEach((d: NetworkDevice) => {
      const v = (d.vendor || 'other').toLowerCase();
      counts[v] = (counts[v] || 0) + 1;
    });
    return counts;
  }, [batchSelectedDeviceList]);

  const batchVendorKeys = useMemo(() => Object.keys(batchVendorStats), [batchVendorStats]);
  const isBatchSingleVendor = batchVendorKeys.length === 1 && batchVendorKeys[0] !== 'other';
  const batchSingleVendorName = batchVendorKeys[0] || 'other';

  const filteredDevices = useMemo(() => {
    let result = selectedVendor === 'all'
      ? devices
      : devices.filter((d: NetworkDevice) => d.vendor === selectedVendor);

    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase();
      result = result.filter((d: NetworkDevice) =>
        d.name.toLowerCase().includes(query) ||
        d.ip_address.toLowerCase().includes(query) ||
        (d.location?.toLowerCase().includes(query)) ||
        (d.model?.toLowerCase().includes(query))
      );
    }

    return result;
  }, [devices, selectedVendor, searchQuery]);

  // 关联数据：各设备的最近巡检/分析/修复概览
  const { data: _linkageData = { alerts: [], analyses: [], inspections: [], executions: [] } } = useQuery({
    queryKey: ['device-linkage'],
    queryFn: () => api.get('/dashboard/linkage').then(r => r.data.data || {}),
    refetchInterval: 60000,
  });

  // 收集各设备最近活动时间轴
  const { data: deviceTimeline = {} as Record<string, DeviceTimelineEntry> } = useQuery({
    queryKey: ['device-timeline'],
    queryFn: async () => {
      const res = await api.get('/inspection-center?limit=300');
      const items = (res.data.data || []) as TimelineItem[];
      // 按 device_id 分组，取最新一条 per source
      const map: Record<string, DeviceTimelineEntry> = {};
      items.forEach((item: TimelineItem) => {
        if (!item.device_id) return;
        if (!map[item.device_id]) map[item.device_id] = {};
        if (item.source === 'analysis' && !map[item.device_id].lastAnalysis) map[item.device_id].lastAnalysis = item;
        if (item.source === 'inspection' && !map[item.device_id].lastInspection) map[item.device_id].lastInspection = item;
      });
      return map;
    },
    refetchInterval: 60000,
  });

  const navigate = useNavigate();

  const vendors = ['all', 'huawei', 'cisco', 'h3c', 'ruijie', 'zte', 'maipu', 'other'];
  const vendorLabels: Record<string, string> = {
    all: '全部厂商',
    huawei: '华为',
    cisco: '思科',
    h3c: '华三',
    ruijie: '锐捷',
    zte: '中兴',
    maipu: '迈普',
    other: '其他'
  };

  return (
    <div className="w-full px-4 sm:px-6 lg:px-8 py-6 space-y-6 text-text-primary min-h-screen">
      {/* 页面顶部汇总统计 */}
      {deviceStats && (
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          <div className="flex items-center gap-4 p-4 rounded-2xl bg-surface backdrop-blur-xl border border-border shadow-xl">
            <div className="p-3 rounded-xl bg-gradient-to-tr from-sky-600 to-indigo-600 text-white shadow-lg shadow-sky-500/20">
              <Network className="w-6 h-6" />
            </div>
            <div>
              <p className="text-xs font-semibold text-text-secondary uppercase tracking-wider">设备总数</p>
              <p className="text-2xl font-bold text-text-primary font-mono mt-0.5">{deviceStats.devices?.total ?? 0}</p>
            </div>
          </div>

          <div className="flex items-center gap-4 p-4 rounded-2xl bg-surface backdrop-blur-xl border border-border shadow-xl">
            <div className="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-emerald-600 shadow-lg shadow-emerald-500/10">
              <CheckCircle2 className="w-6 h-6" />
            </div>
            <div>
              <p className="text-xs font-semibold text-text-secondary uppercase tracking-wider">在线运行</p>
              <div className="flex items-center gap-2 mt-0.5">
                <span className="text-2xl font-bold text-emerald-600 font-mono">{deviceStats.devices?.online ?? 0}</span>
                <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
              </div>
            </div>
          </div>

          <div className="flex items-center gap-4 p-4 rounded-2xl bg-surface backdrop-blur-xl border border-border shadow-xl">
            <div className="p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-400 shadow-lg shadow-rose-500/10">
              <AlertTriangle className="w-6 h-6" />
            </div>
            <div>
              <p className="text-xs font-semibold text-text-secondary uppercase tracking-wider">离线设备</p>
              <p className="text-2xl font-bold text-text-secondary font-mono mt-0.5">{deviceStats.devices?.offline ?? 0}</p>
            </div>
          </div>
        </div>
      )}

      {/* 设备列表主工作区 */}
      <div className="bg-surface backdrop-blur-xl rounded-2xl border border-border shadow-xl overflow-hidden">
        {/* 工具栏与过滤栏 */}
        <div className="p-5 border-b border-border space-y-4">
          <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-bold text-text-secondary mr-1">厂商筛选:</span>
              {vendors.map(vendor => (
                <button
                  key={vendor}
                  onClick={() => setSelectedVendor(vendor)}
                  className={`px-3 py-1 text-xs font-semibold rounded-lg transition-all ${
                    selectedVendor === vendor
                      ? 'bg-sky-600 text-white shadow-md shadow-sky-600/30'
                      : 'bg-background border border-border text-text-secondary hover:text-text-primary hover:border-border'
                  }`}
                >
                  {vendorLabels[vendor]}
                </button>
              ))}
            </div>

            <div className="flex flex-wrap items-center gap-2.5">
              <button
                onClick={handlePingAllDevices}
                disabled={isPingChecking}
                className="p-2 text-text-secondary hover:text-text-primary hover:bg-surface-hover rounded-lg border border-border transition-colors disabled:opacity-50"
                title="Ping 探测全部网络设备在线状态"
              >
                <RefreshCw className={`w-4 h-4 ${isPingChecking ? 'animate-spin text-sky-600' : ''}`} />
              </button>

              {selectedDevices.size > 0 && (
                <>
                  <span className="text-xs text-sky-600 font-semibold px-2">{selectedDevices.size} 台已选</span>
                  <button
                    onClick={handleBatchInspect}
                    className="flex items-center gap-1.5 px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-semibold rounded-lg shadow-md transition-colors"
                  >
                    <ClipboardCheck className="w-3.5 h-3.5" />
                    批量巡检
                  </button>
                  <button
                    onClick={handleBatchDelete}
                    disabled={isBatchDeleting}
                    className="flex items-center gap-1.5 px-3 py-1.5 bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold rounded-lg shadow-md shadow-rose-600/20 transition-colors disabled:opacity-50"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                    批量删除 ({selectedDevices.size})
                  </button>
                  <button
                    onClick={() => setSelectedDevices(new Set())}
                    className="px-3 py-1.5 text-xs text-text-secondary hover:text-text-primary bg-surface-2 rounded-lg transition-colors"
                  >
                    取消选择
                  </button>
                </>
              )}

              <button
                onClick={() => setShowBatchPollModal(true)}
                className="flex items-center gap-1.5 px-3.5 py-1.5 bg-gradient-to-r from-purple-600 to-indigo-600 hover:from-purple-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-lg shadow-md shadow-purple-600/20 transition-all"
                title="按品牌批量设置系统与接口模块采集时间间隔"
              >
                <Sliders className="w-3.5 h-3.5" />
                品牌采集设置
              </button>
              <button
                onClick={() => setShowImportModal(true)}
                className="flex items-center gap-1.5 px-3.5 py-1.5 bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white text-xs font-semibold rounded-lg shadow-md shadow-emerald-600/20 transition-all"
              >
                <Upload className="w-3.5 h-3.5" />
                批量导入
              </button>
              <button
                onClick={() => { setEditingDevice(null); setIsAddModalOpen(true); }}
                className="flex items-center gap-1.5 px-3.5 py-1.5 bg-gradient-to-r from-sky-600 to-blue-600 hover:from-sky-500 hover:to-blue-500 text-white text-xs font-semibold rounded-lg shadow-md shadow-sky-600/20 transition-all"
              >
                <Plus className="w-3.5 h-3.5" />
                新建设备
              </button>
            </div>
          </div>

          <div className="relative">
            <Search className="absolute left-3.5 top-1/2 transform -translate-y-1/2 w-4 h-4 text-text-secondary" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="搜索设备名称、IP 地址、安装位置、设备型号..."
              className="w-full pl-10 pr-4 py-2.5 text-xs bg-background border border-border rounded-xl text-text-primary placeholder-text-tertiary focus:outline-none focus:border-sky-500 focus:ring-2 focus:ring-sky-500/20 transition-all"
            />
          </div>
        </div>

        {isLoading ? (
          <div className="flex flex-col items-center justify-center py-20 gap-3">
            <Loader2 className="w-8 h-8 text-sky-600 animate-spin" />
            <p className="text-xs text-text-secondary">正在加载网络设备列表...</p>
          </div>
        ) : filteredDevices.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 text-center">
            <div className="w-16 h-16 rounded-2xl bg-surface flex items-center justify-center text-text-secondary mb-3 border border-border">
              <Network className="w-8 h-8" />
            </div>
            <p className="text-sm font-semibold text-text-primary mb-1">
              {searchQuery ? '未找到匹配的网络设备' : '暂无网络设备资产'}
            </p>
            <p className="text-xs text-text-secondary mb-4 max-w-sm">
              {searchQuery ? '尝试更换搜索关键字或重置厂商过滤条件' : '您可以点击上方「新建设备」或「批量导入」添加第一台网络设备'}
            </p>
            {!searchQuery && (
              <button
                onClick={() => { setEditingDevice(null); setIsAddModalOpen(true); }}
                className="flex items-center gap-1.5 px-4 py-2 bg-sky-600 hover:bg-sky-500 text-white text-xs font-semibold rounded-xl shadow-lg shadow-sky-600/30 transition-all"
              >
                <Plus className="w-4 h-4" />
                立即新建设备
              </button>
            )}
          </div>
        ) : (
          <div className="p-5 space-y-4">
            <div className="flex items-center gap-2">
              <button
                onClick={selectAllDevices}
                className="flex items-center gap-2 text-xs font-semibold text-text-secondary hover:text-text-primary transition-colors"
              >
                {selectedDevices.size === filteredDevices.length && filteredDevices.length > 0 ? (
                  <CheckSquare className="w-4 h-4 text-sky-600" />
                ) : (
                  <Square className="w-4 h-4 text-text-tertiary" />
                )}
                全选本页 ({selectedDevices.size}/{filteredDevices.length})
              </button>
            </div>

            {/* 卡片栅格系统：左右宽度充分伸展，视觉更加开阔大气 */}
            <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-6">
              {filteredDevices.map((device: NetworkDevice) => (
                <div key={device.id} className="flex flex-col">
                    <NetworkDeviceCard
                      device={device}
                      isSelected={selectedDevices.has(device.id)}
                      onToggleSelect={() => toggleDeviceSelection(device.id)}
                      onEdit={handleEdit}
                      onDelete={handleDelete}
                      onInspect={handleInspect}
                      onSnmpInspect={handleSnmpInspect}
                      onSnmpTestConnection={handleSnmpTestConnection}
                      onTestConnection={handleTestConnection}
                      isTestingConnection={testingConnectionDeviceId === device.id}
                      isTestingSnmp={testingSnmpDeviceId === device.id}
                      onHistory={handleHistory}
                      onWebShell={handleWebShell}
                      onMibCollect={handleMibCollect}
                      onPollSettings={(dev) => setPollSettingsDevice(dev)}
                    />
                  {/* 设备时间轴分析提示 */}
                  {deviceTimeline[device.id]?.lastAnalysis && (
                    <div className="flex items-center justify-between px-4 py-2 mt-2 bg-background border border-border rounded-xl text-xs">
                      <button
                        onClick={() => navigate(`/alert-auto-analysis?deviceId=${device.id}`)}
                        className="flex items-center gap-1.5 text-emerald-600 hover:text-emerald-300 font-medium transition-colors"
                      >
                        <Zap className="w-3.5 h-3.5" />
                        AI 智能分析:{' '}
                        {(() => {
                          const a = deviceTimeline[device.id].lastAnalysis!;
                          try {
                            const d = new Date(a.created_at!);
                            const n = new Date();
                            const m = Math.floor((n.getTime() - d.getTime()) / 60000);
                            return m < 60 ? `${m}分钟前` : `${Math.floor(m / 60)}小时前`;
                          } catch {
                            return '刚刚';
                          }
                        })()}
                      </button>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {isAddModalOpen && (
        <AddDeviceModal
          device={editingDevice}
          onClose={() => { setIsAddModalOpen(false); setEditingDevice(null); }}
          onSuccess={() => {
            setIsAddModalOpen(false);
            setEditingDevice(null);
            queryClient.invalidateQueries({ queryKey: ['network-devices'] });
          }}
        />
      )}

      {showInspectionModal && inspectingDevice && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className={`bg-surface border border-border rounded-2xl shadow-2xl w-full ${inspectionType === 'custom' ? 'max-w-2xl' : 'max-w-md'} flex flex-col max-h-[85vh] overflow-hidden`}>
            {/* 弹窗头部 */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-background">
              <div className="flex items-center gap-2.5">
                <div className="p-2 rounded-lg bg-blue-500/10 text-blue-600 border border-blue-500/20">
                  <Wifi className="w-4 h-4" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-text-primary flex items-center gap-2">
                    {inspectionType === 'custom' ? '设备自定义模版巡检' : inspectionType === 'full' ? '设备全量深度巡检' : '设备标准巡检'}
                    <span className="text-xs px-2 py-0.5 rounded-full bg-surface-2 text-text-secondary border border-border font-normal">
                      {inspectingDevice.name}
                    </span>
                  </h3>
                  <p className="text-xs text-text-secondary mt-0.5">
                    目标 IP: <span className="font-mono text-text-secondary">{inspectingDevice.ip_address}</span> · 厂商: <span className="uppercase font-semibold text-indigo-600">{inspectingDevice.vendor}</span>
                  </p>
                </div>
              </div>
              <button
                onClick={() => setShowInspectionModal(false)}
                className="text-text-secondary hover:text-text-primary p-1 rounded-lg hover:bg-surface-hover transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* 弹窗主体内容 */}
            <div className="p-6 overflow-y-auto flex-1 space-y-4">
              {inspectionType === 'standard' ? (
                <div className="space-y-3">
                  <p className="text-sm text-text-secondary">标准巡检将按内置适配器执行以下核心指标检查：</p>
                  <div className="grid grid-cols-2 gap-2 text-xs">
                    {['CPU 使用率', '内存使用率', '接口状态', '版本信息', '路由表', '系统日志', '环境状态', '电源/风扇'].map(item => (
                      <div key={item} className="flex items-center gap-2 p-2 rounded-lg bg-background border border-border text-text-secondary">
                        <CheckCircle2 className="w-3.5 h-3.5 text-emerald-600 shrink-0" />
                        {item}
                      </div>
                    ))}
                  </div>
                </div>
              ) : inspectionType === 'custom' ? (
                <div className="space-y-4">
                  {/* 品牌锁定提示 */}
                  <div className="flex items-center gap-2 px-3 py-2 rounded-lg bg-indigo-500/10 border border-indigo-500/30 text-indigo-300 text-xs">
                    <Lock className="w-4 h-4 text-indigo-600 shrink-0" />
                    <span>
                      <strong>同品牌模版锁定：</strong>当前设备品牌为【<span className="uppercase font-bold">{inspectingDevice.vendor}</span>】，系统已自动过滤并仅允许选择该品牌的巡检模版。
                    </span>
                  </div>

                  {/* 模版选择器 */}
                  <div className="space-y-1.5">
                    <label className="block text-xs font-semibold text-text-secondary">选择适用的巡检模版</label>
                    {isLoadingTemplates ? (
                      <div className="flex items-center gap-2 text-xs text-text-secondary py-2">
                        <Loader2 className="w-4 h-4 animate-spin text-indigo-600" />
                        正在加载【{inspectingDevice.vendor}】巡检模版...
                      </div>
                    ) : deviceTemplates.length > 0 ? (
                      <select
                        value={selectedTemplateId}
                        onChange={(e) => handleSelectTemplate(e.target.value)}
                        className="w-full px-3 py-2 text-sm bg-background border border-border rounded-lg text-text-primary focus:outline-none focus:border-indigo-500 transition-colors"
                      >
                        {deviceTemplates.map(tpl => (
                          <option key={tpl.id} value={tpl.id}>
                            {tpl.name} ({tpl.isPreset ? '🔒 官方预置' : '✨ 自定义'} · {tpl.commands?.length || 0}项指令)
                          </option>
                        ))}
                      </select>
                    ) : (
                      <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-300 text-xs">
                        未找到专用于【{inspectingDevice.vendor}】的模版，可前往左侧「模版管理中心」新建或克隆该厂商模版。
                      </div>
                    )}
                  </div>

                  {/* 模版指令列表预览与勾选 */}
                  {selectedTemplateId && (
                    <div className="space-y-2">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <Layers className="w-4 h-4 text-text-secondary" />
                          <span className="text-xs font-semibold text-text-primary">
                            模版指令清单（已选 {selectedCommandNames.size} / {
                              (deviceTemplates.find(t => t.id === selectedTemplateId)?.commands || []).length
                            } 条）
                          </span>
                        </div>
                        <div className="flex items-center gap-2">
                          <button
                            type="button"
                            onClick={() => {
                              const tpl = deviceTemplates.find(t => t.id === selectedTemplateId);
                              if (tpl) {
                                setSelectedCommandNames(new Set((tpl.commands || []).map((c: any) => c.name || c.command)));
                              }
                            }}
                            className="text-[11px] text-indigo-600 hover:text-indigo-300 transition-colors"
                          >
                            全选
                          </button>
                          <span className="text-slate-600">|</span>
                          <button
                            type="button"
                            onClick={() => setSelectedCommandNames(new Set())}
                            className="text-[11px] text-text-secondary hover:text-text-primary transition-colors"
                          >
                            清空
                          </button>
                        </div>
                      </div>

                      <div className="max-h-56 overflow-y-auto space-y-2 p-2 bg-background rounded-xl border border-border">
                        {['硬件层', '系统层', '资源层', '安全层', '应用层'].map(layerName => {
                          const activeTpl = deviceTemplates.find(t => t.id === selectedTemplateId);
                          const layerCmds = (activeTpl?.commands || []).filter((c: any) => (c.group || '系统层') === layerName);
                          if (layerCmds.length === 0) return null;

                          return (
                            <div key={layerName} className="space-y-1.5">
                              <div className="text-[11px] font-semibold text-indigo-300/80 px-1 pt-1 flex items-center gap-1.5">
                                <span className="w-1.5 h-1.5 rounded-full bg-indigo-400" />
                                {layerName} ({layerCmds.length})
                              </div>
                              <div className="space-y-1">
                                {layerCmds.map((cmd: any, idx: number) => {
                                  const key = cmd.name || cmd.command;
                                  const isChecked = selectedCommandNames.has(key);
                                  return (
                                    <div
                                      key={idx}
                                      onClick={() => toggleCommandSelection(key)}
                                      className={`flex items-start gap-2.5 p-2 rounded-lg border text-xs cursor-pointer transition-all ${
                                        isChecked
                                          ? 'bg-surface border-indigo-500/40 text-text-primary shadow-sm'
                                          : 'bg-background border-border text-text-secondary opacity-60 hover:opacity-90'
                                      }`}
                                    >
                                      <input
                                        type="checkbox"
                                        checked={isChecked}
                                        onChange={() => {}}
                                        className="mt-0.5 rounded text-indigo-600 focus:ring-0 border-border bg-surface"
                                      />
                                      <div className="flex-1 min-w-0">
                                        <div className="flex items-center justify-between gap-2">
                                          <span className="font-semibold text-text-primary">{cmd.name}</span>
                                          <span className="text-[10px] font-mono text-emerald-600 truncate max-w-[200px]">
                                            {cmd.command}
                                          </span>
                                        </div>
                                        {cmd.description && (
                                          <p className="text-[11px] text-text-secondary truncate mt-0.5">{cmd.description}</p>
                                        )}
                                      </div>
                                    </div>
                                  );
                                })}
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  )}
                </div>
              ) : (
                <div className="space-y-3">
                  <p className="text-sm text-text-secondary">全面巡检将按内置适配器执行完整诊断项：</p>
                  <div className="grid grid-cols-2 gap-2 text-xs">
                    {['CPU', '内存', '接口', '版本', '路由', '日志', '环境', '电源', '风扇', 'STP', 'VLAN', 'ARP', 'MAC'].map(item => (
                      <div key={item} className="flex items-center gap-2 p-2 rounded-lg bg-background border border-border text-text-secondary">
                        <CheckCircle2 className="w-3.5 h-3.5 text-indigo-600 shrink-0" />
                        {item}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>

            {/* 弹窗底部操作栏 */}
            <div className="flex items-center justify-end gap-3 px-6 py-4 bg-background border-t border-border">
              <button
                onClick={() => setShowInspectionModal(false)}
                className="px-4 py-2 text-xs font-medium text-text-secondary hover:text-text-primary transition-colors rounded-lg"
              >
                取消
              </button>
              <button
                onClick={executeInspection}
                disabled={isInspecting || (inspectionType === 'custom' && selectedCommandNames.size === 0)}
                className="flex items-center gap-2 px-5 py-2 bg-gradient-to-r from-indigo-600 to-purple-600 text-white text-xs font-semibold rounded-lg hover:from-indigo-500 hover:to-purple-500 transition-all shadow-lg shadow-indigo-500/25 disabled:opacity-50 disabled:cursor-not-allowed disabled:shadow-none"
              >
                {isInspecting ? (
                  <>
                    <Loader2 className="w-4 h-4 animate-spin" />
                    巡检下发中...
                  </>
                ) : (
                  <>
                    <Wifi className="w-4 h-4" />
                    {inspectionType === 'custom' ? `开始自定义巡检 (${selectedCommandNames.size} 项)` : '立即开始巡检'}
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {showBatchModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-surface border border-border rounded-2xl shadow-2xl w-full max-w-2xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-200">
            {/* 弹窗头部 */}
            <div className="flex items-center justify-between px-6 py-4 border-b border-border bg-background">
              <div className="flex items-center gap-2.5">
                <div className="p-2 rounded-lg bg-emerald-500/10 text-emerald-600 border border-emerald-500/20">
                  <ClipboardCheck className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-semibold text-text-primary flex items-center gap-2">
                    网络设备批量巡检
                    <span className="text-xs px-2.5 py-0.5 rounded-full bg-emerald-500/15 text-emerald-300 border border-emerald-500/30 font-mono font-medium">
                      已选 {selectedDevices.size} 台设备
                    </span>
                  </h3>
                  <p className="text-xs text-text-secondary mt-0.5">
                    支持按标准模式、自定义维度或厂商模版批量下发巡检
                  </p>
                </div>
              </div>
              <button
                onClick={() => setShowBatchModal(false)}
                className="text-text-secondary hover:text-text-primary p-1 rounded-lg hover:bg-surface-hover transition-colors"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* 弹窗主体 */}
            <div className="p-6 overflow-y-auto flex-1 space-y-5">
              {/* 受检设备概况与清单 */}
              <div className="bg-background rounded-xl border border-border p-3 space-y-2">
                <div className="flex items-center justify-between text-xs">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-text-secondary font-medium">受检设备分布:</span>
                    {batchVendorKeys.map(v => (
                      <span
                        key={v}
                        className="px-2 py-0.5 rounded-md bg-surface border border-border text-text-primary font-medium text-[11px]"
                      >
                        {vendorLabels[v] || v.toUpperCase()} · <strong className="text-sky-600">{batchVendorStats[v]}</strong> 台
                      </span>
                    ))}
                    {isBatchSingleVendor ? (
                      <span className="text-[11px] text-emerald-600 flex items-center gap-1 font-medium">
                        <CheckCircle2 className="w-3.5 h-3.5" />
                        同品牌设备 ({vendorLabels[batchSingleVendorName] || batchSingleVendorName.toUpperCase()})
                      </span>
                    ) : (
                      <span className="text-[11px] text-amber-600 flex items-center gap-1 font-medium">
                        <AlertCircle className="w-3.5 h-3.5" />
                        跨品牌混合设备
                      </span>
                    )}
                  </div>
                  <button
                    type="button"
                    onClick={() => setShowBatchDeviceList(prev => !prev)}
                    className="flex items-center gap-1 text-[11px] text-sky-600 hover:text-sky-300 transition-colors ml-auto"
                  >
                    <span>{showBatchDeviceList ? '收起设备列表' : '展开设备列表'}</span>
                    {showBatchDeviceList ? <ChevronUp className="w-3.5 h-3.5" /> : <ChevronDown className="w-3.5 h-3.5" />}
                  </button>
                </div>

                {showBatchDeviceList && (
                  <div className="max-h-36 overflow-y-auto divide-y divide-border pt-2 border-t border-border">
                    {batchSelectedDeviceList.map((dev: NetworkDevice) => (
                      <div key={dev.id} className="py-1.5 flex items-center justify-between text-xs">
                        <div className="flex items-center gap-2">
                          <span className="font-semibold text-text-primary">{dev.name}</span>
                          <span className="text-text-tertiary font-mono text-[11px]">({dev.ip_address})</span>
                        </div>
                        <span className="px-1.5 py-0.2 text-[10px] uppercase rounded bg-surface-2 text-text-secondary border border-border">
                          {dev.vendor}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* 巡检模式选择卡片 */}
              <div className="space-y-2">
                <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wider">
                  选择巡检模式
                </label>
                <div className="grid grid-cols-3 gap-3">
                  {/* 标准巡检 */}
                  <div
                    onClick={() => setBatchInspectionType('standard')}
                    className={`relative p-3.5 rounded-xl border cursor-pointer transition-all ${
                      batchInspectionType === 'standard'
                        ? 'bg-emerald-500/10 border-emerald-500/60 ring-1 ring-emerald-500/40 shadow-lg shadow-emerald-500/10'
                        : 'bg-background border-border hover:border-border hover:bg-background text-text-secondary'
                    }`}
                  >
                    <div className="flex items-center justify-between mb-1.5">
                      <div className="flex items-center gap-1.5 font-semibold text-sm text-text-primary">
                        <Sparkles className="w-4 h-4 text-emerald-600" />
                        标准巡检
                      </div>
                      <span className="text-[10px] px-1.5 py-0.2 rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">
                        推荐
                      </span>
                    </div>
                    <p className="text-[11px] text-text-secondary leading-relaxed">
                      执行厂商核心基础健康诊断，自动适配所选的所有品牌设备。
                    </p>
                  </div>

                  {/* 自定义巡检 */}
                  <div
                    onClick={() => setBatchInspectionType('custom')}
                    className={`relative p-3.5 rounded-xl border cursor-pointer transition-all ${
                      batchInspectionType === 'custom'
                        ? 'bg-purple-500/10 border-purple-500/60 ring-1 ring-purple-500/40 shadow-lg shadow-purple-500/10'
                        : 'bg-background border-border hover:border-border hover:bg-background text-text-secondary'
                    }`}
                  >
                    <div className="flex items-center justify-between mb-1.5">
                      <div className="flex items-center gap-1.5 font-semibold text-sm text-text-primary">
                        <Sliders className="w-4 h-4 text-purple-600" />
                        自定义巡检
                      </div>
                      <span className="text-[10px] px-1.5 py-0.2 rounded bg-purple-500/20 text-purple-300 border border-purple-500/30">
                        高度灵活
                      </span>
                    </div>
                    <p className="text-[11px] text-text-secondary leading-relaxed">
                      自由按维度勾选指标，或选择厂商专属模版与指令。
                    </p>
                  </div>

                  {/* 全量深度巡检 */}
                  <div
                    onClick={() => setBatchInspectionType('full')}
                    className={`relative p-3.5 rounded-xl border cursor-pointer transition-all ${
                      batchInspectionType === 'full'
                        ? 'bg-indigo-500/10 border-indigo-500/60 ring-1 ring-indigo-500/40 shadow-lg shadow-indigo-500/10'
                        : 'bg-background border-border hover:border-border hover:bg-background text-text-secondary'
                    }`}
                  >
                    <div className="flex items-center justify-between mb-1.5">
                      <div className="flex items-center gap-1.5 font-semibold text-sm text-text-primary">
                        <Activity className="w-4 h-4 text-indigo-600" />
                        全量深度巡检
                      </div>
                      <span className="text-[10px] px-1.5 py-0.2 rounded bg-indigo-500/20 text-indigo-300 border border-indigo-500/30">
                        全项覆盖
                      </span>
                    </div>
                    <p className="text-[11px] text-text-secondary leading-relaxed">
                      执行全部内置适配器诊断项，覆盖设备软硬件全量维度。
                    </p>
                  </div>
                </div>
              </div>

              {/* 标准巡检面板 */}
              {batchInspectionType === 'standard' && (
                <div className="space-y-3 p-4 rounded-xl bg-background border border-border">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-semibold text-text-primary flex items-center gap-1.5">
                      <CheckCircle2 className="w-4 h-4 text-emerald-600" />
                      标准巡检核心检查项（8大核心维度）：
                    </span>
                    <span className="text-[10px] text-text-secondary">已内置各厂商官方适配器</span>
                  </div>
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
                    {[
                      { name: 'CPU 使用率', desc: '整机与各核心' },
                      { name: '内存使用率', desc: '物理内存与缓存' },
                      { name: '接口状态', desc: '物理与协议状态' },
                      { name: '系统版本', desc: '版本与运行时长' },
                      { name: '系统路由表', desc: 'IPv4 核心路由' },
                      { name: '系统日志', desc: '最新告警缓冲区' },
                      { name: '环境与温度', desc: '传感器与告警' },
                      { name: '电源与风扇', desc: '在位与运行状态' },
                    ].map(item => (
                      <div key={item.name} className="p-2 rounded-lg bg-surface border border-border text-text-secondary flex flex-col gap-0.5">
                        <span className="font-semibold text-text-primary">{item.name}</span>
                        <span className="text-[10px] text-text-tertiary">{item.desc}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* 自定义巡检面板 */}
              {batchInspectionType === 'custom' && (
                <div className="space-y-4 p-4 rounded-xl bg-background border border-border">
                  {/* 自定义子模式切换 */}
                  <div className="flex items-center justify-between border-b border-border pb-3 flex-wrap gap-2">
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() => setBatchCustomMode('dimension')}
                        className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
                          batchCustomMode === 'dimension'
                            ? 'bg-purple-600 text-white shadow-sm'
                            : 'bg-surface text-text-secondary hover:text-text-primary'
                        }`}
                      >
                        按指标维度自由勾选 (多品牌通用)
                      </button>
                      <button
                        type="button"
                        onClick={() => setBatchCustomMode('template')}
                        className={`px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
                          batchCustomMode === 'template'
                            ? 'bg-purple-600 text-white shadow-sm'
                            : 'bg-surface text-text-secondary hover:text-text-primary'
                        }`}
                      >
                        按厂商专属模版 (同品牌推荐)
                      </button>
                    </div>

                    {batchCustomMode === 'dimension' && (
                      <div className="flex items-center gap-1.5 text-xs">
                        <button
                          type="button"
                          onClick={selectAllDimensions}
                          className="text-[11px] text-purple-600 hover:text-purple-300 font-medium"
                        >
                          全选
                        </button>
                        <span className="text-slate-600">|</span>
                        <button
                          type="button"
                          onClick={selectCoreDimensions}
                          className="text-[11px] text-sky-600 hover:text-sky-300 font-medium"
                        >
                          核心项 (8)
                        </button>
                        <span className="text-slate-600">|</span>
                        <button
                          type="button"
                          onClick={clearDimensions}
                          className="text-[11px] text-text-secondary hover:text-text-primary font-medium"
                        >
                          清空
                        </button>
                      </div>
                    )}
                  </div>

                  {/* 方式 A：按维度自由勾选 */}
                  {batchCustomMode === 'dimension' && (
                    <div className="space-y-3">
                      <div className="flex items-center justify-between text-xs text-text-secondary">
                        <span>勾选需要巡检的指标（已选 <strong className="text-purple-300">{batchSelectedDimensions.size}</strong> 项）：</span>
                        <span className="text-[11px] text-text-tertiary">自动适配选中国产与主流各厂商命令</span>
                      </div>

                      <div className="max-h-56 overflow-y-auto pr-1 space-y-3">
                        {[
                          { category: 'hardware' as const, label: '硬件与性能', color: 'text-amber-600' },
                          { category: 'network' as const, label: '网络与链路', color: 'text-cyan-600' },
                          { category: 'system' as const, label: '系统与安全', color: 'text-indigo-600' },
                        ].map(group => {
                          const dims = ALL_INSPECTION_DIMENSIONS.filter(d => d.category === group.category);
                          return (
                            <div key={group.category} className="space-y-1.5">
                              <div className={`text-[11px] font-semibold ${group.color} flex items-center gap-1`}>
                                <span className="w-1.5 h-1.5 rounded-full bg-current" />
                                {group.label} ({dims.length})
                              </div>
                              <div className="grid grid-cols-1 sm:grid-cols-2 gap-1.5">
                                {dims.map(dim => {
                                  const isChecked = batchSelectedDimensions.has(dim.key);
                                  return (
                                    <div
                                      key={dim.key}
                                      onClick={() => toggleBatchDimension(dim.key)}
                                      className={`flex items-start gap-2 p-2 rounded-lg border text-xs cursor-pointer transition-all ${
                                        isChecked
                                          ? 'bg-purple-950/40 border-purple-500/50 text-text-primary shadow-sm'
                                          : 'bg-surface border-border text-text-secondary hover:border-border hover:text-text-secondary'
                                      }`}
                                    >
                                      <input
                                        type="checkbox"
                                        checked={isChecked}
                                        onChange={() => {}}
                                        className="mt-0.5 rounded text-purple-600 focus:ring-0 border-border bg-surface"
                                      />
                                      <div className="flex-1 min-w-0">
                                        <p className="font-semibold text-text-primary">{dim.name}</p>
                                        <p className="text-[10px] text-text-secondary truncate mt-0.5">{dim.description}</p>
                                      </div>
                                    </div>
                                  );
                                })}
                              </div>
                            </div>
                          );
                        })}
                      </div>
                    </div>
                  )}

                  {/* 方式 B：按厂商模版执行 */}
                  {batchCustomMode === 'template' && (
                    <div className="space-y-3">
                      {!isBatchSingleVendor && (
                        <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/30 text-amber-300 text-xs flex items-start gap-2">
                          <AlertTriangle className="w-4 h-4 shrink-0 mt-0.5" />
                          <div>
                            <p className="font-semibold">多品牌混合提示</p>
                            <p className="text-[11px] text-amber-300/80 mt-0.5">
                              当前受检设备包含多个品牌。特定品牌的 CLI 指令（如 Huawei 与 Cisco 命令格式不同）可能导致跨品牌设备报错。推荐切换为「按指标维度自由勾选」，系统将自动根据品牌适配官方指令。
                            </p>
                          </div>
                        </div>
                      )}

                      <div className="space-y-1.5">
                        <label className="block text-xs font-semibold text-text-secondary">
                          选择适用的巡检模版
                        </label>
                        {isLoadingBatchTemplates ? (
                          <div className="flex items-center gap-2 text-xs text-text-secondary py-2">
                            <Loader2 className="w-4 h-4 animate-spin text-purple-600" />
                            正在加载巡检模版...
                          </div>
                        ) : batchTemplates.length > 0 ? (
                          <select
                            value={batchTemplateId}
                            onChange={(e) => handleSelectBatchTemplate(e.target.value)}
                            className="w-full px-3 py-2 text-sm bg-background border border-border rounded-lg text-text-primary focus:outline-none focus:border-purple-500 transition-colors"
                          >
                            {batchTemplates.map(tpl => (
                              <option key={tpl.id} value={tpl.id}>
                                {tpl.name} ({tpl.isPreset ? '🔒 官方预置' : '✨ 自定义'} · {tpl.commands?.length || 0}项指令)
                              </option>
                            ))}
                          </select>
                        ) : (
                          <div className="p-3 rounded-lg bg-surface border border-border text-text-secondary text-xs">
                            未找到对应的巡检模版，建议切换至「按指标维度自由勾选」模式。
                          </div>
                        )}
                      </div>

                      {/* 模版指令列表 */}
                      {batchTemplateId && (
                        <div className="space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-xs font-semibold text-text-secondary">
                              模版包含指令 (已选 {batchSelectedCommands.size} / {(batchTemplates.find(t => t.id === batchTemplateId)?.commands || []).length} 条)：
                            </span>
                            <div className="flex items-center gap-1.5 text-xs">
                              <button
                                type="button"
                                onClick={() => {
                                  const tpl = batchTemplates.find(t => t.id === batchTemplateId);
                                  if (tpl) {
                                    setBatchSelectedCommands(new Set((tpl.commands || []).map((c: any) => c.name || c.command)));
                                  }
                                }}
                                className="text-[11px] text-purple-600 hover:text-purple-300"
                              >
                                全选
                              </button>
                              <span className="text-slate-600">|</span>
                              <button
                                type="button"
                                onClick={() => setBatchSelectedCommands(new Set())}
                                className="text-[11px] text-text-secondary hover:text-text-primary"
                              >
                                清空
                              </button>
                            </div>
                          </div>
                          <div className="max-h-44 overflow-y-auto space-y-1 p-2 bg-background rounded-lg border border-border">
                            {(batchTemplates.find(t => t.id === batchTemplateId)?.commands || []).map((cmd: any, idx: number) => {
                              const key = cmd.name || cmd.command;
                              const isChecked = batchSelectedCommands.has(key);
                              return (
                                <div
                                  key={idx}
                                  onClick={() => toggleBatchCommandSelection(key)}
                                  className={`flex items-center justify-between p-2 rounded border text-xs cursor-pointer transition-colors ${
                                    isChecked
                                      ? 'bg-purple-950/30 border-purple-500/40 text-text-primary'
                                      : 'bg-surface border-border text-text-secondary opacity-60'
                                  }`}
                                >
                                  <div className="flex items-center gap-2 min-w-0">
                                    <input
                                      type="checkbox"
                                      checked={isChecked}
                                      onChange={() => {}}
                                      className="rounded text-purple-600 focus:ring-0 border-border bg-surface"
                                    />
                                    <span className="font-semibold text-text-primary truncate">{cmd.name}</span>
                                  </div>
                                  <span className="text-[10px] font-mono text-emerald-600 truncate max-w-[200px]">
                                    {cmd.command}
                                  </span>
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {/* 全量深度巡检面板 */}
              {batchInspectionType === 'full' && (
                <div className="space-y-3 p-4 rounded-xl bg-background border border-border">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-semibold text-indigo-300 flex items-center gap-1.5">
                      <Activity className="w-4 h-4 text-indigo-600" />
                      全量深度诊断涵盖全部 13 项核心及扩展维度：
                    </span>
                    <span className="text-[10px] text-indigo-600/80">全面综合诊断</span>
                  </div>
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
                    {['CPU', '内存', '接口', '版本', '路由', '日志', '环境', '电源', '风扇', 'STP', 'VLAN', 'ARP', 'MAC'].map(item => (
                      <div key={item} className="flex items-center gap-1.5 p-2 rounded-lg bg-surface border border-border text-text-secondary">
                        <CheckCircle2 className="w-3.5 h-3.5 text-indigo-600 shrink-0" />
                        <span>{item}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {/* 并发与执行设置 */}
              <div className="flex items-center justify-between p-3 rounded-xl bg-background border border-border text-xs">
                <div className="flex items-center gap-2">
                  <Settings2 className="w-4 h-4 text-text-secondary" />
                  <span className="text-text-secondary font-medium">并发工作线程数:</span>
                  <span className="text-text-tertiary text-[11px]">(推荐 1 ~ 10 并发)</span>
                </div>
                <div className="flex items-center gap-2">
                  <input
                    type="number"
                    min={1}
                    max={20}
                    value={batchConcurrency}
                    onChange={(e) => setBatchConcurrency(Math.max(1, Math.min(20, parseInt(e.target.value) || 5)))}
                    className="w-16 px-2.5 py-1 bg-background border border-border rounded-lg text-center font-mono text-xs text-sky-600 focus:outline-none focus:border-sky-500"
                  />
                  <span className="text-text-secondary text-xs">线程</span>
                </div>
              </div>
            </div>

            {/* 弹窗底部操作栏 */}
            <div className="flex items-center justify-between px-6 py-4 bg-background border-t border-border">
              <span className="text-xs text-text-secondary">
                已选中 <strong className="text-emerald-600 font-mono">{selectedDevices.size}</strong> 台网络设备
              </span>
              <div className="flex items-center gap-3">
                <button
                  type="button"
                  onClick={() => setShowBatchModal(false)}
                  className="px-4 py-2 text-xs font-medium text-text-secondary hover:text-text-primary transition-colors rounded-lg bg-surface border border-border hover:bg-surface-hover"
                >
                  取消
                </button>
                <button
                  type="button"
                  onClick={executeBatchInspection}
                  disabled={
                    isBatchInspecting ||
                    (batchInspectionType === 'custom' &&
                      ((batchCustomMode === 'dimension' && batchSelectedDimensions.size === 0) ||
                        (batchCustomMode === 'template' && (!batchTemplateId || batchSelectedCommands.size === 0))))
                  }
                  className="flex items-center gap-2 px-5 py-2 bg-gradient-to-r from-emerald-600 to-teal-600 text-white text-xs font-semibold rounded-lg hover:from-emerald-500 hover:to-teal-500 transition-all shadow-lg shadow-emerald-500/25 disabled:opacity-50 disabled:cursor-not-allowed disabled:shadow-none"
                >
                  {isBatchInspecting ? (
                    <>
                      <Loader2 className="w-4 h-4 animate-spin" />
                      正在批量巡检中...
                    </>
                  ) : (
                    <>
                      <ClipboardCheck className="w-4 h-4" />
                      {batchInspectionType === 'standard'
                        ? `开始批量标准巡检 (${selectedDevices.size} 台)`
                        : batchInspectionType === 'custom'
                        ? batchCustomMode === 'dimension'
                          ? `开始自定义巡检 (${batchSelectedDimensions.size} 项 · ${selectedDevices.size} 台)`
                          : `开始模版巡检 (${batchSelectedCommands.size} 项 · ${selectedDevices.size} 台)`
                        : `开始批量全量巡检 (${selectedDevices.size} 台)`}
                    </>
                  )}
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {inspectionResult && (
        <InspectionResult
          result={inspectionResult}
          deviceName={inspectingDevice?.name || ''}
          onClose={() => setInspectionResult(null)}
        />
      )}

      {snmpInspectionResult && (
        <SnmpInspectionResult
          result={snmpInspectionResult}
          deviceName={snmpInspectionResult._deviceName || ''}
          onClose={() => setSnmpInspectionResult(null)}
        />
      )}

      {showHistory && (
        <InspectionHistory
          deviceId={showHistory.id}
          deviceName={showHistory.name}
          onClose={() => setShowHistory(null)}
        />
      )}

      {deleteConfirmDevice && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="bg-surface border border-border rounded-xl shadow-2xl w-full max-w-sm mx-4">
            <div className="p-6">
              <div className="flex items-center gap-3 mb-4">
                <div className="w-10 h-10 rounded-full bg-red-500/10 flex items-center justify-center">
                  <Trash2 className="w-5 h-5 text-red-500" />
                </div>
                <div>
                  <h3 className="text-base font-medium text-text-primary">确认删除</h3>
                  <p className="text-sm text-text-secondary">此操作不可撤销</p>
                </div>
              </div>
              <p className="text-sm text-text-secondary mb-4">
                确定要删除设备 <span className="font-medium text-text-primary">{deleteConfirmDevice.name}</span>（{deleteConfirmDevice.ip_address}）吗？
              </p>
              <div className="flex items-center justify-end gap-2">
                <button
                  onClick={() => setDeleteConfirmDevice(null)}
                  className="px-4 py-2 text-sm text-text-secondary hover:text-text-primary transition-colors rounded-md"
                >
                  取消
                </button>
                <button
                  onClick={confirmDelete}
                  className="px-4 py-2 text-sm bg-red-600 text-white font-medium rounded-md hover:bg-red-500 transition-colors shadow-lg shadow-red-600/20"
                >
                  确认删除
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {webShellDevice && token && (
        <div className="fixed inset-0 z-50 flex flex-col bg-black/90 backdrop-blur-sm">
          <div className="flex-1 overflow-hidden">
            <WebTerminal
              serverId={webShellDevice.id}
              serverName={`${webShellDevice.name} (${webShellDevice.ip_address})`}
              token={token}
              deviceType="network-device"
              onClose={() => setWebShellDevice(null)}
            />
          </div>
        </div>
      )}

      {/* MIB 采集弹窗 */}
      {mibCollectDevice && (
        <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 p-4">
          <div className="bg-surface backdrop-blur-xl rounded-2xl w-full max-w-5xl border border-border shadow-2xl flex flex-col max-h-[92vh]">
            <div className="p-6 border-b border-border/30 flex items-center justify-between flex-shrink-0">
              <h2 className="text-xl font-bold text-text-primary flex items-center gap-2">
                <Database className="w-5 h-5 text-purple-600" />
                MIB 采集 — {mibCollectDevice.name}
                <span className="text-xs text-text-secondary font-normal">({mibCollectDevice.ip_address})</span>
              </h2>
              <button
                onClick={() => { setMibCollectDevice(null); setMibCollectResults(null); }}
                className="p-2 hover:bg-surface-hover rounded-xl text-text-secondary hover:text-text-primary transition-all"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="p-6 overflow-y-auto flex-1 space-y-4">
              {/* 选择 MIB 模块 */}
              {!mibCollectResults && (
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-text-primary mb-1">选择 MIB 模块</label>
                    <select
                      value={selectedMibModuleId}
                      onChange={(e) => setSelectedMibModuleId(e.target.value)}
                      className="w-full px-4 py-2.5 bg-surface border border-border rounded-xl text-text-primary focus:outline-none focus:border-blue-500/50 focus:ring-2 focus:ring-blue-500/20 transition-all"
                      size={8}
                    >
                      <option value="">-- 选择 MIB 模块 --</option>
                      {mibModules.map((m: any) => (
                        <option key={m.id} value={m.id}>
                          {m.name} ({m.object_count || 0} 个 OID)
                        </option>
                      ))}
                    </select>
                    {mibModules.length === 0 && (
                      <p className="text-xs text-amber-600 mt-2">
                        未找到 MIB 模块，请先在「SNMP 采集」页面上传 MIB 文件或 Excel
                      </p>
                    )}
                  </div>
                </div>
              )}

              {/* 采集结果 */}
              {mibCollectResults && mibCollectResults.length > 0 && (
                <div className="space-y-3">
                  <div className="flex items-center gap-2">
                    {mibCollectResults[0].status === 'success' ? (
                      <CheckCircle2 className="w-5 h-5 text-emerald-600" />
                    ) : (
                      <AlertCircle className="w-5 h-5 text-amber-600" />
                    )}
                    <span className="text-sm font-medium text-text-primary">
                      采集{mibCollectResults[0].status === 'success' ? '成功' : '完成（部分失败）'}
                    </span>
                    <span className="text-xs text-text-secondary">
                      耗时 {mibCollectResults[0].durationMs}ms
                    </span>
                  </div>

                  <div className="bg-background rounded-lg overflow-auto">
                    <table className="w-full min-w-[700px]">
                      <thead className="sticky top-0 bg-background z-10">
                        <tr className="border-b border-border/50">
                          <th className="text-left text-xs font-medium text-text-secondary px-3 py-2 w-[180px]">指标名称</th>
                          <th className="text-left text-xs font-medium text-text-secondary px-3 py-2 w-[220px]">OID</th>
                          <th className="text-left text-xs font-medium text-text-secondary px-3 py-2">采集值</th>
                          <th className="text-left text-xs font-medium text-text-secondary px-3 py-2 w-[80px]">状态</th>
                        </tr>
                      </thead>
                      <tbody>
                        {(mibCollectResults[0].metrics || []).map((m: any, i: number) => (
                          <tr key={i} className="border-b border-border/20 hover:bg-background/50">
                            <td className="px-3 py-2">
                              <span className="text-xs text-emerald-600 font-medium break-all">{m.metric_name || '-'}</span>
                            </td>
                            <td className="px-3 py-2">
                              <span className="text-xs text-text-secondary font-mono break-all">{m.oid}</span>
                            </td>
                            <td className="px-3 py-2">
                              <span className={`text-xs font-mono break-all ${m.metric_value && m.metric_value !== 'N/A' ? 'text-blue-600' : 'text-text-secondary'}`}>
                                {m.metric_value && m.metric_value !== 'N/A' ? String(m.metric_value) : '-'}
                              </span>
                            </td>
                            <td className="px-3 py-2">
                              {m.error ? (
                                <span className="text-xs text-red-600">{m.error}</span>
                              ) : (
                                <span className="text-xs text-emerald-600">OK</span>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}
            </div>

            <div className="p-6 border-t border-border/30 flex gap-3 flex-shrink-0">
              {!mibCollectResults ? (
                <>
                  <button
                    onClick={() => { setMibCollectDevice(null); setMibCollectResults(null); }}
                    className="flex-1 px-4 py-2.5 bg-surface-2 text-text-primary rounded-xl hover:bg-surface-hover transition-all font-medium border border-border"
                  >
                    取消
                  </button>
                  <button
                    onClick={executeMibCollect}
                    disabled={isMibCollecting || !selectedMibModuleId}
                    className="flex-1 px-4 py-2.5 bg-purple-600 hover:bg-purple-500 text-white rounded-xl font-medium transition-all disabled:opacity-50 flex items-center justify-center gap-2"
                  >
                    {isMibCollecting ? (
                      <>
                        <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                        采集中...
                      </>
                    ) : (
                      <>
                        <Activity className="w-4 h-4" />
                        开始采集
                      </>
                    )}
                  </button>
                </>
              ) : (
                <button
                  onClick={() => { setMibCollectResults(null); setSelectedMibModuleId(''); }}
                  className="flex-1 px-4 py-2.5 bg-purple-600 hover:bg-purple-500 text-white rounded-xl font-medium transition-all"
                >
                  再次采集
                </button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* 批量导入弹窗 */}
      {showImportModal && (
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="bg-surface-2 rounded-2xl shadow-2xl w-full max-w-lg border border-border">
            <div className="p-6 border-b border-border/30">
              <div className="flex items-center justify-between">
                <h3 className="text-lg font-semibold text-text-primary">批量导入网络设备</h3>
                <button
                  onClick={() => { setShowImportModal(false); setImportResult(null); setImportFile(null); }}
                  className="p-1 hover:bg-surface-hover rounded-md transition-colors"
                >
                  <X className="w-5 h-5 text-text-secondary" />
                </button>
              </div>
              <p className="text-xs text-text-secondary mt-1">
                上传 Excel (.xlsx) 文件批量导入设备，支持中英文列名
              </p>
            </div>

            <div className="p-6 space-y-4">
              {/* 下载模板 */}
              <button
                onClick={handleDownloadTemplate}
                className="w-full flex items-center justify-center gap-2 px-4 py-2.5 border border-dashed border-border rounded-xl text-sm text-text-secondary hover:text-emerald-600 hover:border-emerald-500/50 transition-colors"
              >
                <Download className="w-4 h-4" />
                下载导入模板
              </button>

              {/* 文件上传区域 */}
              {!importResult && (
                <div
                  onDragOver={(e) => { e.preventDefault(); e.stopPropagation(); }}
                  onDrop={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    const file = e.dataTransfer.files[0];
                    if (file) { setImportFile(file); setImportResult(null); }
                  }}
                  className={`relative border-2 border-dashed rounded-xl p-6 text-center transition-colors cursor-pointer ${
                    importFile ? 'border-emerald-500/50 bg-emerald-500/5' : 'border-border hover:border-blue-500/50'
                  }`}
                  onClick={() => document.getElementById('import-file-input')?.click()}
                >
                  <input
                    id="import-file-input"
                    type="file"
                    accept=".xlsx,.xls,.csv"
                    className="hidden"
                    onChange={(e) => {
                      const file = e.target.files?.[0];
                      if (file) { setImportFile(file); setImportResult(null); }
                    }}
                  />
                  {importFile ? (
                    <div className="space-y-1">
                      <Upload className="w-8 h-8 text-emerald-600 mx-auto" />
                      <p className="text-sm text-text-primary font-medium">{importFile.name}</p>
                      <p className="text-xs text-text-secondary">{(importFile.size / 1024).toFixed(1)} KB</p>
                    </div>
                  ) : (
                    <div className="space-y-1">
                      <Upload className="w-8 h-8 text-text-secondary/50 mx-auto" />
                      <p className="text-sm text-text-secondary">点击或拖拽文件到此处</p>
                      <p className="text-xs text-text-secondary/50">支持 .xlsx / .xls / .csv</p>
                    </div>
                  )}
                </div>
              )}

              {/* 导入结果 */}
              {importResult && (
                <div className="bg-background rounded-xl p-4 space-y-3">
                  <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-full bg-emerald-500/20 flex items-center justify-center">
                      <CheckCircle2 className="w-5 h-5 text-emerald-600" />
                    </div>
                    <div>
                      <p className="text-sm font-medium text-text-primary">导入完成</p>
                      <p className="text-xs text-text-secondary">
                        成功 {importResult.imported} 台 / 跳过 {Array.isArray(importResult.skipped) ? importResult.skipped.length : importResult.skipped} 台 / 失败 {Array.isArray(importResult.failed) ? importResult.failed.length : importResult.failed} 台
                      </p>
                    </div>
                  </div>

                  {importResult.parseErrors && importResult.parseErrors.length > 0 && (
                    <div className="space-y-1">
                      <p className="text-xs font-medium text-red-600">解析错误：</p>
                      {importResult.parseErrors.map((err: any, i: number) => (
                        <p key={i} className="text-xs text-red-600/80">
                          第 {err.row ?? '?'} 行: {err.error ?? err.message ?? String(err)}
                        </p>
                      ))}
                    </div>
                  )}

                  {importResult.failed && Array.isArray(importResult.failed) && importResult.failed.length > 0 && (
                    <div className="space-y-1">
                      <p className="text-xs font-medium text-red-600">导入失败：</p>
                      {importResult.failed.map((err: any, i: number) => (
                        <p key={i} className="text-xs text-red-600/80">
                          {err.name ?? err.ip_address ?? '?'}: {err.reason ?? err.error ?? String(err)}
                        </p>
                      ))}
                    </div>
                  )}

                  {importResult.skipped && Array.isArray(importResult.skipped) && importResult.skipped.length > 0 && (
                    <div className="space-y-1">
                      <p className="text-xs font-medium text-amber-600">跳过的重复设备：</p>
                      {importResult.skipped.map((item: any, i: number) => (
                        <span key={i} className="inline-block text-xs bg-amber-500/10 text-amber-600 px-2 py-0.5 rounded mr-1 mb-1">
                          {item.name} ({item.ip_address}) — {item.reason ?? 'IP已存在'}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>

            <div className="p-6 border-t border-border/30 flex gap-3">
              <button
                onClick={() => { setShowImportModal(false); setImportResult(null); setImportFile(null); }}
                className="flex-1 px-4 py-2.5 bg-surface-2 text-text-primary rounded-xl hover:bg-surface-hover transition-all font-medium border border-border"
              >
                {importResult ? '关闭' : '取消'}
              </button>
              {!importResult && (
                <button
                  onClick={handleImport}
                  disabled={isImporting || !importFile}
                  className="flex-1 px-4 py-2.5 bg-emerald-600 hover:bg-emerald-500 text-white rounded-xl font-medium transition-all disabled:opacity-50 flex items-center justify-center gap-2"
                >
                  {isImporting ? (
                    <>
                      <div className="w-4 h-4 border-2 border-white/30 border-t-white rounded-full animate-spin" />
                      导入中...
                    </>
                  ) : (
                    <>
                      <Upload className="w-4 h-4" />
                      开始导入
                    </>
                  )}
                </button>
              )}
            </div>
          </div>
        </div>
      )}

      {/* 单设备采集周期设置模态框 */}
      {pollSettingsDevice && (
        <PollSettingsModal
          device={pollSettingsDevice}
          isOpen={!!pollSettingsDevice}
          onClose={() => setPollSettingsDevice(null)}
          onSuccess={() => {
            queryClient.invalidateQueries({ queryKey: ['network-devices'] });
          }}
        />
      )}

      {/* 按品牌批量采集周期设置模态框 */}
      <BatchPollSettingsModal
        isOpen={showBatchPollModal}
        onClose={() => setShowBatchPollModal(false)}
        onSuccess={() => {
          queryClient.invalidateQueries({ queryKey: ['network-devices'] });
        }}
      />

      {/* 批量删除确认模态框 */}
      {showBatchDeleteModal && (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-50 flex items-center justify-center p-4">
          <div className="bg-surface border border-border rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4 animate-in fade-in zoom-in duration-200">
            <div className="flex items-center gap-3 text-rose-400">
              <div className="p-3 bg-rose-500/10 rounded-xl border border-rose-500/20">
                <AlertTriangle className="w-6 h-6" />
              </div>
              <div>
                <h3 className="font-bold text-lg text-text-primary">确认批量删除设备</h3>
                <p className="text-xs text-text-secondary">此操作不可恢复</p>
              </div>
            </div>

            <p className="text-sm text-text-secondary">
              您已选中 <span className="font-bold text-rose-400">{selectedDevices.size}</span> 台网络设备。确定要将它们全部删除吗？关联的监控快照、巡检历史和告警分析也将一并清理。
            </p>

            <div className="max-h-40 overflow-y-auto p-3 bg-background border border-border rounded-xl space-y-1.5 text-xs">
              {devices
                .filter((d: NetworkDevice) => selectedDevices.has(d.id))
                .map((d: NetworkDevice) => (
                  <div key={d.id} className="flex justify-between items-center text-text-secondary">
                    <span className="font-medium text-text-primary truncate max-w-[200px]">{d.name}</span>
                    <span className="font-mono text-text-secondary">{d.ip_address}</span>
                  </div>
                ))}
            </div>

            <div className="flex gap-3 pt-2">
              <button
                type="button"
                onClick={() => setShowBatchDeleteModal(false)}
                disabled={isBatchDeleting}
                className="flex-1 px-4 py-2.5 bg-surface-2 text-text-secondary hover:bg-surface-hover hover:text-text-primary rounded-xl transition-colors text-sm font-medium"
              >
                取消
              </button>
              <button
                type="button"
                onClick={confirmBatchDelete}
                disabled={isBatchDeleting}
                className="flex-1 px-4 py-2.5 bg-rose-600 hover:bg-rose-500 text-white rounded-xl transition-all text-sm font-semibold shadow-lg shadow-rose-600/30 flex items-center justify-center gap-2 disabled:opacity-50"
              >
                {isBatchDeleting && <Loader2 className="w-4 h-4 animate-spin" />}
                {isBatchDeleting ? '正在删除...' : `确认删除 (${selectedDevices.size}台)`}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
