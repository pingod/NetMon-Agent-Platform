import {
  Network,
  MoreHorizontal,
  Cpu,
  Wifi,
  History,
  Play,
  Settings,
  Edit,
  Trash2,
  ArrowDown,
  ArrowUp,
  AlertCircle,
  Activity,
  Terminal,
  Database,
  Shield,
  Radio,
  Router as RouterIcon,
  CheckSquare,
  Square,
  Loader2
} from 'lucide-react';
import { useState, useEffect, useRef } from 'react';
import { useQuery } from '@tanstack/react-query';
import api from '../../../lib/api';

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
  snmp_credential_name?: string;
}

interface NetworkDeviceCardProps {
  device: NetworkDevice;
  isSelected?: boolean;
  onToggleSelect?: () => void;
  onEdit: (device: NetworkDevice) => void;
  onDelete: (device: NetworkDevice) => void;
  onInspect: (device: NetworkDevice, type: 'standard' | 'custom' | 'full') => void;
  onSnmpInspect?: (device: NetworkDevice) => void;
  onSnmpTestConnection?: (device: NetworkDevice) => void;
  onTestConnection: (device: NetworkDevice) => void;
  onHistory: (device: NetworkDevice) => void;
  onWebShell?: (device: NetworkDevice) => void;
  onMibCollect?: (device: NetworkDevice) => void;
  onPollSettings?: (device: NetworkDevice) => void;
  isTestingConnection?: boolean;
  isTestingSnmp?: boolean;
}

const vendorConfig: Record<string, { label: string; color: string; bgClass: string; dotColor: string }> = {
  huawei: { label: '华为', color: 'text-red-400', bgClass: 'bg-red-500/10 border border-red-500/20', dotColor: 'bg-red-500' },
  cisco: { label: '思科', color: 'text-blue-400', bgClass: 'bg-blue-500/10 border border-blue-500/20', dotColor: 'bg-blue-500' },
  h3c: { label: '华三', color: 'text-emerald-400', bgClass: 'bg-emerald-500/10 border border-emerald-500/20', dotColor: 'bg-emerald-500' },
  ruijie: { label: '锐捷', color: 'text-purple-400', bgClass: 'bg-purple-500/10 border border-purple-500/20', dotColor: 'bg-purple-500' },
  zte: { label: '中兴', color: 'text-amber-400', bgClass: 'bg-amber-500/10 border border-amber-500/20', dotColor: 'bg-amber-500' },
  maipu: { label: '迈普', color: 'text-pink-400', bgClass: 'bg-pink-500/10 border border-pink-500/20', dotColor: 'bg-pink-500' },
  fortinet: { label: '飞塔', color: 'text-orange-400', bgClass: 'bg-orange-500/10 border border-orange-500/20', dotColor: 'bg-orange-500' },
  juniper: { label: '瞻博', color: 'text-teal-400', bgClass: 'bg-teal-500/10 border border-teal-500/20', dotColor: 'bg-teal-500' },
  other: { label: '其他', color: 'text-slate-400', bgClass: 'bg-slate-500/10 border border-slate-500/20', dotColor: 'bg-slate-500' },
};

const getRoleIcon = (role?: string) => {
  const norm = (role || '').toLowerCase();
  if (norm.includes('firewall')) return { icon: Shield, label: '防火墙', gradient: 'from-amber-600 to-rose-600' };
  if (norm.includes('router') || norm.includes('gateway')) return { icon: RouterIcon, label: '路由器', gradient: 'from-blue-600 to-cyan-600' };
  if (norm.includes('ap') || norm.includes('wireless')) return { icon: Radio, label: '无线AP', gradient: 'from-purple-600 to-pink-600' };
  return { icon: Network, label: '交换机', gradient: 'from-sky-600 to-indigo-600' };
};

export default function NetworkDeviceCard({
  device,
  isSelected,
  onToggleSelect,
  onEdit,
  onDelete,
  onInspect,
  onSnmpInspect,
  onSnmpTestConnection,
  onTestConnection,
  onHistory,
  onWebShell,
  onMibCollect,
  onPollSettings,
  isTestingConnection,
  isTestingSnmp,
}: NetworkDeviceCardProps) {
  const [showMenu, setShowMenu] = useState(false);
  const menuContainerRef = useRef<HTMLDivElement>(null);

  // Fetch interface summary if SNMP-enabled
  const { data: ifaceSummary } = useQuery({
    queryKey: ['device-iface-summary', device.id],
    queryFn: () => api.get(`/network-devices/${device.id}/interface-summary`).then(r => r.data.data),
    enabled: device.snmp_enabled === 1,
    refetchInterval: 60000,
    staleTime: 30000,
  });

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuContainerRef.current && !menuContainerRef.current.contains(event.target as Node)) {
        setShowMenu(false);
      }
    };

    if (showMenu) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [showMenu]);

  const vendorKey = (device.vendor || '').toLowerCase();
  const vendor = vendorConfig[vendorKey] || {
    label: device.vendor || '其他',
    color: 'text-slate-400',
    bgClass: 'bg-slate-500/10 border border-slate-500/20',
    dotColor: 'bg-slate-500'
  };

  const roleMeta = getRoleIcon(device.role);
  const RoleIconComp = roleMeta.icon;

  const isOnline = device.status === 'online';

  const formatLastInspection = (date?: string) => {
    if (!date) return '未巡检';
    const d = new Date(date);
    const now = new Date();
    const diff = now.getTime() - d.getTime();
    const hours = Math.floor(diff / (1000 * 60 * 60));
    if (hours < 1) return `${Math.floor(diff / (1000 * 60))} 分钟前`;
    if (hours < 24) return `${hours} 小时前`;
    const days = Math.floor(hours / 24);
    if (days < 7) return `${days} 天前`;
    return d.toLocaleDateString('zh-CN');
  };

  return (
    <div className={`group relative bg-slate-900/80 hover:bg-slate-900/95 backdrop-blur-xl rounded-2xl border ${
      isSelected ? 'border-sky-500/70 ring-1 ring-sky-500/30' : 'border-slate-800/80 hover:border-sky-500/40'
    } shadow-lg hover:shadow-xl hover:shadow-sky-500/5 transition-all duration-300 flex flex-col justify-between overflow-hidden`}>
      {/* 顶部微光装饰线条 */}
      <div className="absolute top-0 inset-x-0 h-1 bg-gradient-to-r from-transparent via-sky-500/40 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300" />

      {/* 卡片主体 */}
      <div className="p-5 space-y-4">
        {/* 头部：勾选框、设备图标、名称、IP 与操作菜单 */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0 flex-1">
            {onToggleSelect && (
              <button
                type="button"
                onClick={(e) => {
                  e.preventDefault();
                  e.stopPropagation();
                  onToggleSelect();
                }}
                className={`p-1.5 rounded-lg border transition-all flex-shrink-0 ${
                  isSelected
                    ? 'bg-sky-500/20 border-sky-500/50 text-sky-400'
                    : 'bg-slate-950/60 border-slate-800 text-slate-500 hover:text-slate-300 hover:border-slate-700'
                }`}
                title={isSelected ? '取消选择' : '选中设备'}
              >
                {isSelected ? <CheckSquare className="w-4 h-4 text-sky-400" /> : <Square className="w-4 h-4" />}
              </button>
            )}

            <div className={`w-11 h-11 rounded-xl bg-gradient-to-br ${roleMeta.gradient} flex items-center justify-center text-white shadow-md shadow-sky-500/10 flex-shrink-0 group-hover:scale-105 transition-transform`}>
              <RoleIconComp className="w-5 h-5" />
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <h3 className="text-sm font-bold text-slate-100 truncate tracking-tight" title={device.name}>
                  {device.name}
                </h3>
              </div>
              <p className="text-xs text-slate-400 font-mono mt-0.5 truncate select-all">{device.ip_address}</p>
            </div>
          </div>

          {/* 绝对定位的优雅下拉菜单容器 */}
          <div ref={menuContainerRef} className="relative flex-shrink-0">
            <button
              onClick={() => setShowMenu(prev => !prev)}
              className={`p-1.5 rounded-lg border transition-all ${
                showMenu
                  ? 'bg-slate-800 border-slate-600 text-white'
                  : 'bg-slate-950/60 border-slate-800 text-slate-400 hover:text-slate-200 hover:border-slate-700'
              }`}
              title="更多操作"
            >
              <MoreHorizontal className="w-4 h-4" />
            </button>

            {showMenu && (
              <div className="absolute right-0 top-full mt-2 w-44 bg-slate-900/95 backdrop-blur-2xl border border-slate-700/80 rounded-xl shadow-2xl z-40 py-1.5 divide-y divide-slate-800 animate-in fade-in zoom-in-95 duration-150">
                <div className="py-1">
                  <button
                    onClick={() => { setShowMenu(false); onInspect(device, 'standard'); }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-slate-300 hover:text-white hover:bg-slate-800/80 transition-colors text-left"
                  >
                    <Play className="w-3.5 h-3.5 text-sky-400" />
                    标准巡检
                  </button>
                  <button
                    onClick={() => { setShowMenu(false); onInspect(device, 'full'); }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-slate-300 hover:text-white hover:bg-slate-800/80 transition-colors text-left"
                  >
                    <Cpu className="w-3.5 h-3.5 text-indigo-400" />
                    全面巡检
                  </button>
                  <button
                    onClick={() => { setShowMenu(false); onInspect(device, 'custom'); }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-slate-300 hover:text-white hover:bg-slate-800/80 transition-colors text-left"
                  >
                    <Settings className="w-3.5 h-3.5 text-slate-400" />
                    自定义巡检
                  </button>
                </div>

                {(device.snmp_enabled === 1 && (onSnmpInspect || onMibCollect)) && (
                  <div className="py-1">
                    {onSnmpInspect && (
                      <button
                        onClick={() => { setShowMenu(false); onSnmpInspect(device); }}
                        className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-emerald-400 hover:bg-emerald-500/10 transition-colors text-left"
                      >
                        <Cpu className="w-3.5 h-3.5" />
                        SNMP 巡检
                      </button>
                    )}
                    {onMibCollect && (
                      <button
                        onClick={() => { setShowMenu(false); onMibCollect(device); }}
                        className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-purple-400 hover:bg-purple-500/10 transition-colors text-left"
                      >
                        <Database className="w-3.5 h-3.5" />
                        MIB 采集
                      </button>
                    )}
                  </div>
                )}

                <div className="py-1">
                  <button
                    onClick={() => { setShowMenu(false); onTestConnection(device); }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-slate-300 hover:text-white hover:bg-slate-800/80 transition-colors text-left"
                  >
                    <Wifi className="w-3.5 h-3.5 text-emerald-400" />
                    测试网络连接
                  </button>
                  {onWebShell && (
                    <button
                      onClick={() => { setShowMenu(false); onWebShell(device); }}
                      className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-slate-300 hover:text-white hover:bg-slate-800/80 transition-colors text-left"
                    >
                      <Terminal className="w-3.5 h-3.5 text-sky-400" />
                      WebShell 终端
                    </button>
                  )}
                  <button
                    onClick={() => { setShowMenu(false); onHistory(device); }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-slate-300 hover:text-white hover:bg-slate-800/80 transition-colors text-left"
                  >
                    <History className="w-3.5 h-3.5 text-amber-400" />
                    巡检历史记录
                  </button>
                  {onPollSettings && (
                    <button
                      onClick={() => { setShowMenu(false); onPollSettings(device); }}
                      className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-purple-400 hover:bg-purple-500/10 transition-colors text-left"
                    >
                      <Settings className="w-3.5 h-3.5" />
                      采集周期设置
                    </button>
                  )}
                </div>

                <div className="py-1">
                  <button
                    onClick={() => { setShowMenu(false); onEdit(device); }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-slate-300 hover:text-white hover:bg-slate-800/80 transition-colors text-left"
                  >
                    <Edit className="w-3.5 h-3.5 text-sky-400" />
                    编辑设备
                  </button>
                  <button
                    onClick={() => { setShowMenu(false); onDelete(device); }}
                    className="w-full flex items-center gap-2.5 px-3 py-1.5 text-xs text-red-400 hover:bg-red-500/10 transition-colors text-left"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                    删除设备
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>

        {/* 标签属性栏 */}
        <div className="flex items-center gap-2 flex-wrap">
          <span className={`px-2 py-0.5 text-xs font-semibold rounded-md ${vendor.bgClass} ${vendor.color} flex items-center gap-1`}>
            <span className={`w-1.5 h-1.5 rounded-full ${vendor.dotColor}`} />
            {vendor.label}
          </span>
          {device.model && (
            <span className="px-2 py-0.5 text-xs text-slate-300 bg-slate-800/80 border border-slate-700/60 rounded-md">
              {device.model}
            </span>
          )}
          <span className="px-2 py-0.5 text-xs text-slate-300 bg-slate-800/80 border border-slate-700/60 rounded-md">
            {roleMeta.label}
          </span>
          {device.snmp_enabled === 1 && (
            <span
              className="px-2 py-0.5 text-xs font-semibold text-emerald-400 bg-emerald-500/10 border border-emerald-500/20 rounded-md"
              title={device.snmp_credential_name ? `SNMP 凭据: ${device.snmp_credential_name}` : 'SNMP 采集已启用'}
            >
              SNMP
            </span>
          )}
        </div>

        {/* 基础参数网格 */}
        <div className="bg-slate-950/40 rounded-xl p-3 border border-slate-800/60 space-y-2 text-xs">
          {device.location && (
            <div className="flex items-center justify-between">
              <span className="text-slate-400">安装位置</span>
              <span className="font-medium text-slate-200">{device.location}</span>
            </div>
          )}
          <div className="flex items-center justify-between">
            <span className="text-slate-400">运行状态</span>
            <div className="flex items-center gap-1.5">
              <span className={`w-2 h-2 rounded-full ${isOnline ? 'bg-emerald-400 animate-pulse' : 'bg-slate-500'}`} />
              <span className={`font-semibold ${isOnline ? 'text-emerald-400' : 'text-slate-400'}`}>
                {isOnline ? '在线运行' : '离线'}
              </span>
            </div>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-slate-400">最后巡检</span>
            <span className="font-medium text-slate-300">{formatLastInspection(device.last_inspection_at)}</span>
          </div>
          {device.last_inspection_result && (
            <div className="flex items-center justify-between">
              <span className="text-slate-400">巡检结果</span>
              <span className="font-medium text-slate-200 truncate max-w-[120px]">{device.last_inspection_result}</span>
            </div>
          )}
        </div>

        {/* SNMP 吞吐量与接口健康 */}
        {device.snmp_enabled === 1 && ifaceSummary && (
          <div className="pt-2 border-t border-slate-800/80 space-y-2.5">
            {ifaceSummary.throughput && (
              <div className="bg-slate-950/40 p-2.5 rounded-xl border border-slate-800/50">
                <div className="flex items-center justify-between text-xs mb-1.5">
                  <span className="text-slate-400 flex items-center gap-1 font-medium">
                    <Activity className="w-3.5 h-3.5 text-purple-400" />
                    总吞吐量
                  </span>
                  <span className="font-bold text-purple-400 font-mono">
                    {ifaceSummary.throughput.totalMbps.toFixed(1)} Mbps
                  </span>
                </div>
                <div className="flex items-center justify-between text-[11px] text-slate-400">
                  <div className="flex items-center gap-1">
                    <ArrowDown className="w-3 h-3 text-sky-400" />
                    <span className="text-sky-400 font-mono font-medium">{ifaceSummary.throughput.inMbps.toFixed(1)}</span>
                    <span>下行</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <ArrowUp className="w-3 h-3 text-emerald-400" />
                    <span className="text-emerald-400 font-mono font-medium">{ifaceSummary.throughput.outMbps.toFixed(1)}</span>
                    <span>上行</span>
                  </div>
                </div>
              </div>
            )}

            {ifaceSummary.interfaces && ifaceSummary.interfaces.length > 0 && (
              <div>
                <div className="flex items-center justify-between text-xs mb-1.5">
                  <span className="text-slate-400 font-medium">物理端口概览</span>
                  <div className="flex items-center gap-2 text-xs font-semibold font-mono">
                    <span className="text-emerald-400">{ifaceSummary.totalUp} UP</span>
                    {ifaceSummary.totalDown > 0 && (
                      <span className="text-rose-400 px-1.5 py-0.2 bg-rose-500/10 rounded">{ifaceSummary.totalDown} DOWN</span>
                    )}
                  </div>
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* 底部快捷操作栏 */}
      <div className="border-t border-slate-800/80 px-4 py-2.5 flex items-center justify-between bg-slate-950/60">
        {device.snmp_enabled === 1 && onSnmpTestConnection ? (
          <button
            onClick={() => onSnmpTestConnection(device)}
            disabled={isTestingSnmp}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold text-emerald-400 hover:bg-emerald-500/10 rounded-lg border border-emerald-500/20 transition-colors disabled:opacity-50"
          >
            {isTestingSnmp ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <Wifi className="w-3.5 h-3.5" />}
            {isTestingSnmp ? '测试中...' : 'SNMP 测试连接'}
          </button>
        ) : (
          <button
            onClick={() => onInspect(device, 'standard')}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-semibold text-sky-400 hover:bg-sky-500/10 rounded-lg border border-sky-500/20 transition-colors"
          >
            <Play className="w-3.5 h-3.5" />
            一键巡检
          </button>
        )}

        <div className="flex items-center gap-1">
          {onWebShell && (
            <button
              onClick={() => onWebShell(device)}
              className="p-1.5 text-slate-400 hover:text-sky-400 hover:bg-slate-800 rounded-lg transition-colors"
              title="WebShell 终端"
            >
              <Terminal className="w-3.5 h-3.5" />
            </button>
          )}
          <button
            onClick={() => onTestConnection(device)}
            disabled={isTestingConnection}
            className="p-1.5 text-slate-400 hover:text-emerald-400 hover:bg-slate-800 rounded-lg transition-colors disabled:opacity-50"
            title="测试网络连接"
          >
            {isTestingConnection ? <Loader2 className="w-3.5 h-3.5 text-emerald-400 animate-spin" /> : <Wifi className="w-3.5 h-3.5" />}
          </button>
          <button
            onClick={() => onHistory(device)}
            className="p-1.5 text-slate-400 hover:text-amber-400 hover:bg-slate-800 rounded-lg transition-colors"
            title="巡检历史"
          >
            <History className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => onEdit(device)}
            className="p-1.5 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded-lg transition-colors"
            title="编辑"
          >
            <Edit className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}
