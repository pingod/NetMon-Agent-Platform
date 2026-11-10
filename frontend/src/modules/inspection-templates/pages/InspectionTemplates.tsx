import React, { useState, useEffect, useMemo } from 'react';
import {
  Table,
  Button,
  Modal,
  Form,
  Input,
  Select,
  Tag,
  Space,
  message,
  Popconfirm,
  Tooltip,
  Badge,
  Collapse,
  Divider,
  Tabs,
  Card,
  Drawer,
  Upload,
} from 'antd';
import {
  Plus,
  Edit,
  Trash2,
  Search,
  RefreshCw,
  Copy,
  Lock,
  Download,
  Upload as UploadIcon,
  Layers,
  Server,
  Network,
  Cpu,
  Shield,
  Activity,
  HardDrive,
  CheckCircle2,
  Sliders,
  Eye,
  Zap,
  Tag as TagIcon,
  LayoutGrid,
  List as ListIcon,
  Sparkles,
} from 'lucide-react';
import api from '../../../lib/api';

interface CommandItem {
  type: string;
  name: string;
  command: string;
  fallbacks?: string[];
  description: string;
  group: string;
}

interface InspectionTemplate {
  id: string;
  name: string;
  vendor: string;
  category: string;
  deviceType: string;
  osType: string;
  description: string;
  commands: CommandItem[];
  variables: string[];
  tags: string[];
  isPreset: boolean;
  enabled: boolean;
  usageCount: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

const vendorMeta: Record<string, { label: string; color: string; bg: string; border: string }> = {
  huawei: { label: '华为', color: 'text-red-400', bg: 'bg-red-500/10', border: 'border-red-500/30' },
  inspur: { label: '浪潮', color: 'text-blue-400', bg: 'bg-blue-500/10', border: 'border-blue-500/30' },
  tongfang: { label: '清华同方', color: 'text-purple-400', bg: 'bg-purple-500/10', border: 'border-purple-500/30' },
  lenovo: { label: '联想', color: 'text-rose-400', bg: 'bg-rose-500/10', border: 'border-rose-500/30' },
  dell: { label: '戴尔', color: 'text-sky-400', bg: 'bg-sky-500/10', border: 'border-sky-500/30' },
  hpe: { label: '惠普/HPE', color: 'text-emerald-400', bg: 'bg-emerald-500/10', border: 'border-emerald-500/30' },
  h3c: { label: '华三/H3C', color: 'text-cyan-400', bg: 'bg-cyan-500/10', border: 'border-cyan-500/30' },
  sugon: { label: '中科曙光', color: 'text-amber-400', bg: 'bg-amber-500/10', border: 'border-amber-500/30' },
  supermicro: { label: '超微', color: 'text-teal-400', bg: 'bg-teal-500/10', border: 'border-teal-500/30' },
  zte: { label: '中兴', color: 'text-purple-400', bg: 'bg-purple-500/10', border: 'border-purple-500/30' },
  cisco: { label: '思科', color: 'text-sky-400', bg: 'bg-sky-500/10', border: 'border-sky-500/30' },
  ruijie: { label: '锐捷', color: 'text-amber-400', bg: 'bg-amber-500/10', border: 'border-amber-500/30' },
  sangfor: { label: '深信服', color: 'text-emerald-400', bg: 'bg-emerald-500/10', border: 'border-emerald-500/30' },
  maipu: { label: '迈普', color: 'text-blue-400', bg: 'bg-blue-500/10', border: 'border-blue-500/30' },
  fortinet: { label: '飞塔', color: 'text-indigo-400', bg: 'bg-indigo-500/10', border: 'border-indigo-500/30' },
  juniper: { label: '瞻博', color: 'text-pink-400', bg: 'bg-pink-500/10', border: 'border-pink-500/30' },
  paloalto: { label: 'PaloAlto', color: 'text-orange-400', bg: 'bg-orange-500/10', border: 'border-orange-500/30' },
  other: { label: '通用/其他', color: 'text-slate-400', bg: 'bg-slate-500/10', border: 'border-slate-500/30' },
};

const groupColors: Record<string, { badge: string; text: string; dot: string }> = {
  '硬件层': { badge: 'bg-rose-500/15 text-rose-400 border-rose-500/30', text: 'text-rose-400', dot: 'bg-rose-500' },
  '系统层': { badge: 'bg-sky-500/15 text-sky-400 border-sky-500/30', text: 'text-sky-400', dot: 'bg-sky-500' },
  '资源层': { badge: 'bg-amber-500/15 text-amber-400 border-amber-500/30', text: 'text-amber-400', dot: 'bg-amber-500' },
  '安全层': { badge: 'bg-violet-500/15 text-violet-400 border-violet-500/30', text: 'text-violet-400', dot: 'bg-violet-500' },
  '应用层': { badge: 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30', text: 'text-emerald-400', dot: 'bg-emerald-500' },
};

export default function InspectionTemplates() {
  const [data, setData] = useState<InspectionTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  const [search, setSearch] = useState('');
  const [categoryFilter, setCategoryFilter] = useState('all'); // all, network, server
  const [vendorFilter, setVendorFilter] = useState('');
  const [viewMode, setViewMode] = useState<'card' | 'table'>('card');
  const [modalOpen, setModalOpen] = useState(false);
  const [previewTemplate, setPreviewTemplate] = useState<InspectionTemplate | null>(null);
  const [editing, setEditing] = useState<InspectionTemplate | null>(null);
  const [form] = Form.useForm();
  const [commands, setCommands] = useState<CommandItem[]>([]);

  const fetchData = async () => {
    setLoading(true);
    try {
      const res = await api.get('/inspection-templates', {
        params: {
          search,
          category: categoryFilter === 'all' ? '' : categoryFilter,
          vendor: vendorFilter,
        },
      });
      setData(res.data.data || []);
    } catch {
      message.error('加载模版列表失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [search, categoryFilter, vendorFilter]);

  // 统计指标
  const stats = useMemo(() => {
    const total = data.length;
    const networkCount = data.filter(d => d.category === 'network').length;
    const serverCount = data.filter(d => d.category === 'server').length;
    const customCount = data.filter(d => !d.isPreset).length;
    const totalExecutions = data.reduce((acc, cur) => acc + (cur.usageCount || 0), 0);
    return { total, networkCount, serverCount, customCount, totalExecutions };
  }, [data]);

  const handleSave = async () => {
    const values = await form.validateFields();
    try {
      const payload = {
        ...values,
        commands,
        tags: values.tags || [],
        variables: values.variables || [],
      };
      if (editing) {
        await api.put(`/inspection-templates/${editing.id}`, payload);
        message.success('模版更新成功');
      } else {
        await api.post('/inspection-templates', payload);
        message.success('自定义模版创建成功');
      }
      setModalOpen(false);
      setEditing(null);
      form.resetFields();
      setCommands([]);
      fetchData();
    } catch {
      message.error('保存失败');
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.delete(`/inspection-templates/${id}`);
      message.success('删除模版成功');
      fetchData();
    } catch {
      message.error('删除失败');
    }
  };

  const handleClone = async (id: string) => {
    try {
      await api.post(`/inspection-templates/${id}/clone`);
      message.success('克隆模版成功，已生成副本');
      fetchData();
    } catch {
      message.error('克隆失败');
    }
  };

  const openEdit = (record: InspectionTemplate) => {
    setEditing(record);
    setCommands(record.commands || []);
    form.setFieldsValue({
      ...record,
      tags: record.tags || [],
    });
    setModalOpen(true);
  };

  const openCreate = () => {
    setEditing(null);
    form.resetFields();
    setCommands([
      { type: 'cpu', name: 'CPU 使用率', command: 'display cpu-usage', description: '检查 CPU 利用率', group: '资源层' },
      { type: 'memory', name: '内存使用率', command: 'display memory-usage', description: '检查内存利用率', group: '资源层' },
      { type: 'interface', name: '接口状态', command: 'display interface brief', description: '检查所有接口状态', group: '系统层' },
    ]);
    setModalOpen(true);
  };

  const addCommand = (group = '系统层') => {
    setCommands([...commands, { type: '', name: '', command: '', description: '', group }]);
  };

  const removeCommand = (idx: number) => {
    setCommands(commands.filter((_, i) => i !== idx));
  };

  const updateCommand = (idx: number, field: keyof CommandItem, val: any) => {
    const updated = [...commands];
    updated[idx] = {
      ...updated[idx],
      [field]: val,
    };
    setCommands(updated);
  };

  // 快捷载入 SmartKit 五层服务器检查项
  const preloadSmartKitServerItems = () => {
    const smartKitPresets: CommandItem[] = [
      { type: 'hardware', name: '服务器型号与SN', command: 'dmidecode -t system | grep -E "Manufacturer|Product Name|Serial Number"', description: '检查整机与主板型号', group: '硬件层' },
      { type: 'cpu_info', name: 'CPU 规格与健康', command: 'lscpu | grep -E "Model name|Socket|Thread|NUMA"', description: 'CPU 核心与架构', group: '硬件层' },
      { type: 'memory_info', name: '内存插槽与ECC', command: 'dmidecode -t memory | grep -E "Size|Type|Speed|Error" | grep -v "No Module"', description: '内存条及 ECC 状态', group: '硬件层' },
      { type: 'disk_info', name: '磁盘拓扑', command: 'lsblk -o NAME,SIZE,TYPE,FSTYPE,MOUNTPOINT', description: '物理磁盘与文件系统', group: '硬件层' },
      { type: 'raid_status', name: 'RAID 阵列状态', command: 'megacli -LDInfo -Lall -aALL 2>/dev/null || storcli /call show 2>/dev/null || echo Passthrough', description: 'RAID 控制卡与磁盘状态', group: '硬件层' },
      { type: 'os_version', name: 'OS 发行版与内核', command: 'cat /etc/os-release ; uname -r', description: '操作系统版本与内核', group: '系统层' },
      { type: 'uptime', name: '系统运行时间', command: 'uptime', description: '运行天数与平均负载', group: '系统层' },
      { type: 'cpu_usage', name: 'CPU 负载 TOP10', command: 'top -bn1 | head -15', description: 'CPU 利用率及消耗最高进程', group: '资源层' },
      { type: 'memory_usage', name: '内存与 Swap 使用', command: 'free -h', description: '内存剩余量与 Swap 占用', group: '资源层' },
      { type: 'disk_usage', name: '磁盘分区使用率', command: 'df -hT -x tmpfs -x devtmpfs', description: '挂载点磁盘使用率', group: '资源层' },
      { type: 'ssh_config', name: 'SSH 安全基线', command: 'grep -E "^(PermitRootLogin|PasswordAuthentication|Port)" /etc/ssh/sshd_config', description: 'SSH 访问配置', group: '安全层' },
      { type: 'firewall', name: '防火墙状态', command: 'systemctl is-active firewalld ufw 2>/dev/null || iptables -L -n | head -15', description: '防火墙安全状态', group: '安全层' },
      { type: 'failed_services', name: '异常故障服务', command: 'systemctl --failed --plain --no-pager', description: '崩溃退出的服务', group: '应用层' },
      { type: 'docker_status', name: '容器引擎与实例', command: 'docker ps --format "table {{.Names}}\\t{{.Status}}" 2>/dev/null || echo Inactive', description: '容器运行时健康', group: '应用层' },
    ];
    setCommands(smartKitPresets);
    message.success('已载入 SmartKit 五层标准巡检项模版');
  };

  // 导出 JSON
  const exportTemplateJSON = (tpl: InspectionTemplate) => {
    const exportData = JSON.stringify(tpl, null, 2);
    const blob = new Blob([exportData], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `inspection_template_${tpl.vendor}_${tpl.name.replace(/\s+/g, '_')}.json`;
    a.click();
    URL.revokeObjectURL(url);
    message.success('模版已导出');
  };

  // 导入 JSON
  const handleImportJSON = (file: File) => {
    const reader = new FileReader();
    reader.onload = async (e) => {
      try {
        const json = JSON.parse(e.target?.result as string);
        if (!json.name || !json.commands) {
          message.error('无效的模版 JSON 格式');
          return;
        }
        await api.post('/inspection-templates', {
          name: `${json.name} (导入)`,
          vendor: json.vendor || 'other',
          category: json.category || 'network',
          deviceType: json.deviceType || 'unknown',
          osType: json.osType || 'linux',
          description: json.description || '从 JSON 导入的巡检模版',
          commands: json.commands || [],
          tags: json.tags || ['导入'],
          variables: json.variables || [],
        });
        message.success('模版导入成功');
        fetchData();
      } catch {
        message.error('解析或导入模版失败');
      }
    };
    reader.readAsText(file);
    return false;
  };

  // 按分组展示指令（编辑器中）
  const groupedCommands = () => {
    const groups: Record<string, CommandItem[]> = {};
    commands.forEach(c => {
      const g = c.group || '系统层';
      if (!groups[g]) groups[g] = [];
      groups[g].push(c);
    });
    const order = ['硬件层', '系统层', '资源层', '安全层', '应用层'];
    return order.map(g => {
      const items = groups[g] || [];
      const col = groupColors[g] || { text: 'text-slate-400', badge: 'bg-slate-800' };
      return {
        key: g,
        label: (
          <div className="flex items-center justify-between w-full pr-4">
            <span className="flex items-center gap-2 font-medium text-sm text-slate-200">
              <span className={`px-2 py-0.5 rounded text-xs border ${col.badge}`}>{g}</span>
              <span>{items.length} 项指令</span>
            </span>
            <Button
              size="small"
              type="dashed"
              icon={<Plus size={12} />}
              onClick={(e) => {
                e.stopPropagation();
                addCommand(g);
              }}
            >
              在该层新增
            </Button>
          </div>
        ),
        children: items.length > 0 ? (
          <div className="space-y-3">
            {items.map((cmd, i) => {
              const globalIdx = commands.indexOf(cmd);
              return (
                <div
                  key={i}
                  className="p-3 bg-slate-900/60 rounded-lg border border-slate-800 hover:border-slate-700 transition-all space-y-2"
                >
                  <div className="flex items-center justify-between gap-3">
                    <div className="flex items-center gap-2 flex-1">
                      <Input
                        size="small"
                        value={cmd.name}
                        onChange={e => updateCommand(globalIdx, 'name', e.target.value)}
                        placeholder="巡检项名称 (如: CPU使用率)"
                        className="w-48 bg-slate-950/80 border-slate-700 text-slate-200"
                      />
                      <Input
                        size="small"
                        value={cmd.type}
                        onChange={e => updateCommand(globalIdx, 'type', e.target.value)}
                        placeholder="类型标识 (如: cpu)"
                        className="w-32 bg-slate-950/80 border-slate-700 text-slate-300 font-mono text-xs"
                      />
                      <Select
                        size="small"
                        value={cmd.group}
                        onChange={v => updateCommand(globalIdx, 'group', v)}
                        className="w-28"
                        options={[
                          { value: '硬件层', label: '硬件层' },
                          { value: '系统层', label: '系统层' },
                          { value: '资源层', label: '资源层' },
                          { value: '安全层', label: '安全层' },
                          { value: '应用层', label: '应用层' },
                        ]}
                      />
                    </div>
                    <Button
                      size="small"
                      danger
                      type="text"
                      icon={<Trash2 size={13} />}
                      onClick={() => removeCommand(globalIdx)}
                    />
                  </div>
                  <Input.TextArea
                    size="small"
                    value={cmd.command}
                    onChange={e => updateCommand(globalIdx, 'command', e.target.value)}
                    rows={2}
                    className="font-mono text-xs bg-slate-950 border-slate-800 text-emerald-400"
                    placeholder="SSH 巡检命令 (如: display cpu-usage)"
                  />
                  <Input
                    size="small"
                    value={cmd.description}
                    onChange={e => updateCommand(globalIdx, 'description', e.target.value)}
                    placeholder="巡检项目说明与判定标准 (如: 检查 CPU 综合利用率，正常应低于 80%)"
                    className="bg-slate-950/50 border-slate-800 text-slate-400 text-xs"
                  />
                </div>
              );
            })}
          </div>
        ) : (
          <div className="text-center py-4 text-xs text-slate-500">该层暂无指令，点击右上角「在该层新增」添加</div>
        ),
      };
    });
  };

  const columns = [
    {
      title: '模版名称',
      dataIndex: 'name',
      key: 'name',
      width: 280,
      render: (name: string, record: InspectionTemplate) => (
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <span className="font-semibold text-slate-100">{name}</span>
            {record.isPreset ? (
              <span className="px-1.5 py-0.5 rounded text-[10px] bg-blue-500/20 text-blue-400 border border-blue-500/30 flex items-center gap-0.5">
                <Lock size={9} /> 预置
              </span>
            ) : (
              <span className="px-1.5 py-0.5 rounded text-[10px] bg-purple-500/20 text-purple-400 border border-purple-500/30 flex items-center gap-0.5">
                <Sparkles size={9} /> 自定义
              </span>
            )}
          </div>
          <p className="text-xs text-slate-400 line-clamp-1">{record.description || '暂无说明'}</p>
        </div>
      ),
    },
    {
      title: '类别',
      dataIndex: 'category',
      key: 'category',
      width: 100,
      render: (c: string) => (
        <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${c === 'server' ? 'bg-indigo-500/20 text-indigo-400 border border-indigo-500/30' : 'bg-sky-500/20 text-sky-400 border border-sky-500/30'}`}>
          {c === 'server' ? '🖥️ 服务器' : '🌐 网络设备'}
        </span>
      ),
    },
    {
      title: '所属厂商',
      dataIndex: 'vendor',
      key: 'vendor',
      width: 100,
      render: (v: string) => {
        const meta = vendorMeta[v] || vendorMeta.other;
        return (
          <span className={`px-2.5 py-0.5 rounded-md text-xs font-medium border ${meta.bg} ${meta.color} ${meta.border}`}>
            {meta.label}
          </span>
        );
      },
    },
    {
      title: '指令数',
      key: 'cmdCount',
      width: 90,
      render: (_: unknown, r: InspectionTemplate) => (
        <span className="font-semibold text-emerald-400 text-xs px-2 py-0.5 rounded bg-emerald-500/10 border border-emerald-500/20">
          {r.commands?.length || 0} 条
        </span>
      ),
    },
    {
      title: '执行次数',
      dataIndex: 'usageCount',
      key: 'usageCount',
      width: 90,
      render: (c: number) => <span className="text-xs text-slate-400">{c || 0} 次</span>,
    },
    {
      title: '标签',
      dataIndex: 'tags',
      key: 'tags',
      width: 180,
      render: (tags: string[]) => (
        <div className="flex flex-wrap gap-1">
          {(tags || []).map(t => (
            <span key={t} className="px-1.5 py-0.5 rounded text-[11px] bg-slate-800 text-slate-300 border border-slate-700">
              {t}
            </span>
          ))}
        </div>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 180,
      render: (_: unknown, record: InspectionTemplate) => (
        <Space size="small">
          <Tooltip title="查看指令">
            <Button
              type="text"
              size="small"
              icon={<Eye size={14} className="text-sky-400" />}
              onClick={() => setPreviewTemplate(record)}
            />
          </Tooltip>
          <Tooltip title="克隆模版">
            <Button
              type="text"
              size="small"
              icon={<Copy size={14} className="text-purple-400" />}
              onClick={() => handleClone(record.id)}
            />
          </Tooltip>
          <Tooltip title="编辑">
            <Button
              type="text"
              size="small"
              icon={<Edit size={14} className="text-amber-400" />}
              onClick={() => openEdit(record)}
            />
          </Tooltip>
          <Tooltip title="导出 JSON">
            <Button
              type="text"
              size="small"
              icon={<Download size={14} className="text-emerald-400" />}
              onClick={() => exportTemplateJSON(record)}
            />
          </Tooltip>
          {!record.isPreset && (
            <Popconfirm title="确定删除该自定义模版?" onConfirm={() => handleDelete(record.id)}>
              <Tooltip title="删除">
                <Button type="text" size="small" danger icon={<Trash2 size={14} />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ];

  return (
    <div className="p-6 space-y-6 max-w-[1600px] mx-auto min-h-screen">
      {/* 顶部页头与统计 */}
      <div className="relative overflow-hidden rounded-2xl bg-gradient-to-r from-slate-900 via-indigo-950/40 to-slate-900 border border-slate-800/80 p-6 shadow-2xl">
        <div className="absolute top-0 right-0 w-96 h-96 bg-indigo-500/10 rounded-full blur-3xl pointer-events-none" />
        <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 relative z-10">
          <div>
            <div className="flex items-center gap-3">
              <div className="p-2.5 rounded-xl bg-indigo-500/20 border border-indigo-500/30 text-indigo-400 shadow-lg shadow-indigo-500/10">
                <Sliders className="w-6 h-6" />
              </div>
              <div>
                <h1 className="text-xl md:text-2xl font-bold text-white tracking-tight flex items-center gap-2">
                  模版管理中心
                  <span className="text-xs px-2.5 py-0.5 rounded-full bg-gradient-to-r from-indigo-500/20 to-purple-500/20 border border-indigo-500/30 text-indigo-300 font-normal">
                    同品牌设备联动巡检
                  </span>
                </h1>
                <p className="text-xs md:text-sm text-slate-400 mt-1">
                  内置华为、中兴、思科、华三、锐捷、深信服、迈普等网络设备巡检模板及 SmartKit 服务器五层深度巡检规范，支持自定义配置与一键克隆
                </p>
              </div>
            </div>
          </div>

          {/* 统计指标卡 */}
          <div className="grid grid-cols-3 sm:grid-cols-5 gap-2 w-full md:w-auto">
            <div className="px-3 py-2 rounded-xl bg-slate-800/60 border border-slate-700/60 text-center">
              <div className="text-[11px] text-slate-400">总模版数</div>
              <div className="text-lg font-bold text-white">{stats.total}</div>
            </div>
            <div className="px-3 py-2 rounded-xl bg-sky-500/10 border border-sky-500/20 text-center">
              <div className="text-[11px] text-sky-400">网络模版</div>
              <div className="text-lg font-bold text-sky-300">{stats.networkCount}</div>
            </div>
            <div className="px-3 py-2 rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-center">
              <div className="text-[11px] text-indigo-400">SmartKit/服务器</div>
              <div className="text-lg font-bold text-indigo-300">{stats.serverCount}</div>
            </div>
            <div className="px-3 py-2 rounded-xl bg-purple-500/10 border border-purple-500/20 text-center">
              <div className="text-[11px] text-purple-400">自定义模版</div>
              <div className="text-lg font-bold text-purple-300">{stats.customCount}</div>
            </div>
            <div className="px-3 py-2 rounded-xl bg-emerald-500/10 border border-emerald-500/20 text-center">
              <div className="text-[11px] text-emerald-400">已执行次数</div>
              <div className="text-lg font-bold text-emerald-300">{stats.totalExecutions}</div>
            </div>
          </div>
        </div>
      </div>

      {/* 筛选与操作工具栏 */}
      <div className="flex flex-col lg:flex-row items-stretch lg:items-center justify-between gap-4 bg-slate-900/60 backdrop-blur-md p-4 rounded-xl border border-slate-800/80">
        {/* 左侧：分类 Tabs 与搜索 */}
        <div className="flex flex-wrap items-center gap-3">
          <div className="inline-flex p-1 bg-slate-950/80 rounded-lg border border-slate-800">
            {[
              { key: 'all', label: '全部模版', icon: Layers },
              { key: 'network', label: '网络设备', icon: Network },
              { key: 'server', label: '服务器巡检', icon: Server },
            ].map(tab => (
              <button
                key={tab.key}
                onClick={() => setCategoryFilter(tab.key)}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${
                  categoryFilter === tab.key
                    ? 'bg-gradient-to-r from-indigo-600 to-indigo-700 text-white shadow-md shadow-indigo-500/20'
                    : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50'
                }`}
              >
                <tab.icon size={13} />
                {tab.label}
              </button>
            ))}
          </div>

          <Select
            placeholder="按厂商筛选"
            className="w-36"
            value={vendorFilter || undefined}
            onChange={(v: string) => setVendorFilter(v || '')}
            allowClear
            options={[
              { value: 'huawei', label: '华为 (Huawei)' },
              { value: 'zte', label: '中兴 (ZTE)' },
              { value: 'cisco', label: '思科 (Cisco)' },
              { value: 'h3c', label: '华三 (H3C)' },
              { value: 'ruijie', label: '锐捷 (Ruijie)' },
              { value: 'sangfor', label: '深信服 (Sangfor)' },
              { value: 'maipu', label: '迈普 (Maipu)' },
              { value: 'fortinet', label: '飞塔 (Fortinet)' },
              { value: 'juniper', label: '瞻博 (Juniper)' },
              { value: 'paloalto', label: 'PaloAlto' },
              { value: 'hpe', label: 'HPE / Aruba' },
              { value: 'other', label: '通用 / 其他' },
            ]}
          />

          <Input
            placeholder="搜索模版名称、指令或描述..."
            prefix={<Search size={14} className="text-slate-500 mr-1" />}
            className="w-64 bg-slate-950/80 border-slate-700 text-slate-200"
            value={search}
            onChange={e => setSearch(e.target.value)}
            allowClear
          />
        </div>

        {/* 右侧：视图切换与创建/导入 */}
        <div className="flex items-center gap-2.5">
          <div className="inline-flex p-1 bg-slate-950/80 rounded-lg border border-slate-800">
            <button
              onClick={() => setViewMode('card')}
              className={`p-1.5 rounded text-xs transition-colors ${viewMode === 'card' ? 'bg-slate-800 text-white' : 'text-slate-400 hover:text-white'}`}
              title="卡片网格视图"
            >
              <LayoutGrid size={14} />
            </button>
            <button
              onClick={() => setViewMode('table')}
              className={`p-1.5 rounded text-xs transition-colors ${viewMode === 'table' ? 'bg-slate-800 text-white' : 'text-slate-400 hover:text-white'}`}
              title="表格列表视图"
            >
              <ListIcon size={14} />
            </button>
          </div>

          <Button icon={<RefreshCw size={13} />} onClick={fetchData} className="bg-slate-800 border-slate-700 text-slate-200">
            刷新
          </Button>

          <Upload beforeUpload={handleImportJSON} showUploadList={false} accept=".json">
            <Button icon={<UploadIcon size={13} />} className="bg-slate-800 border-slate-700 text-slate-200">
              导入
            </Button>
          </Upload>

          <Button
            type="primary"
            icon={<Plus size={14} />}
            onClick={openCreate}
            className="bg-gradient-to-r from-indigo-600 to-purple-600 border-none shadow-lg shadow-indigo-500/25"
          >
            新建自定义模版
          </Button>
        </div>
      </div>

      {/* 模版列表展示 */}
      {viewMode === 'card' ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {data.map(tpl => {
            const vMeta = vendorMeta[tpl.vendor] || vendorMeta.other;
            // 统计各层级指令数
            const layerCounts: Record<string, number> = {};
            (tpl.commands || []).forEach(c => {
              const g = c.group || '系统层';
              layerCounts[g] = (layerCounts[g] || 0) + 1;
            });

            return (
              <div
                key={tpl.id}
                className="group bg-slate-900/70 backdrop-blur-md rounded-xl border border-slate-800 hover:border-indigo-500/40 transition-all duration-200 p-5 flex flex-col justify-between hover:shadow-xl hover:shadow-indigo-500/5 relative overflow-hidden"
              >
                <div className="space-y-3">
                  {/* 厂商 & 类别头部 */}
                  <div className="flex items-center justify-between">
                    <span className={`px-2.5 py-0.5 rounded-md text-xs font-semibold border ${vMeta.bg} ${vMeta.color} ${vMeta.border}`}>
                      {vMeta.label}
                    </span>

                    <div className="flex items-center gap-1.5">
                      <span className={`px-2 py-0.5 rounded text-[11px] ${tpl.category === 'server' ? 'bg-indigo-500/10 text-indigo-400 border border-indigo-500/20' : 'bg-sky-500/10 text-sky-400 border border-sky-500/20'}`}>
                        {tpl.category === 'server' ? '服务器' : '网络设备'}
                      </span>
                      {tpl.isPreset ? (
                        <span className="px-2 py-0.5 rounded text-[11px] bg-blue-500/10 text-blue-400 border border-blue-500/20 flex items-center gap-1">
                          <Lock size={9} /> 官方预置
                        </span>
                      ) : (
                        <span className="px-2 py-0.5 rounded text-[11px] bg-purple-500/10 text-purple-400 border border-purple-500/20 flex items-center gap-1">
                          <Sparkles size={9} /> 自定义
                        </span>
                      )}
                    </div>
                  </div>

                  {/* 模版名称与描述 */}
                  <div>
                    <h3 className="text-base font-semibold text-white group-hover:text-indigo-300 transition-colors line-clamp-1">
                      {tpl.name}
                    </h3>
                    <p className="text-xs text-slate-400 mt-1 line-clamp-2 min-h-[32px]">
                      {tpl.description || '暂无说明'}
                    </p>
                  </div>

                  {/* SmartKit 五层分层指标预览 */}
                  <div className="space-y-1.5 pt-2 border-t border-slate-800/80">
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-slate-400">巡检指令分布</span>
                      <span className="font-semibold text-emerald-400">{tpl.commands?.length || 0} 条</span>
                    </div>
                    <div className="flex flex-wrap gap-1">
                      {['硬件层', '系统层', '资源层', '安全层', '应用层'].map(g => {
                        const count = layerCounts[g] || 0;
                        if (count === 0) return null;
                        const col = groupColors[g] || { badge: 'bg-slate-800 text-slate-400 border-slate-700' };
                        return (
                          <span key={g} className={`px-2 py-0.5 rounded text-[10px] font-medium border ${col.badge}`}>
                            {g} ({count})
                          </span>
                        );
                      })}
                    </div>
                  </div>

                  {/* 标签 */}
                  {(tpl.tags || []).length > 0 && (
                    <div className="flex flex-wrap gap-1 pt-1">
                      {tpl.tags.map(t => (
                        <span key={t} className="px-1.5 py-0.5 rounded text-[10px] bg-slate-800/80 text-slate-400 border border-slate-700/50">
                          #{t}
                        </span>
                      ))}
                    </div>
                  )}
                </div>

                {/* 底部操作栏 */}
                <div className="flex items-center justify-between pt-4 mt-4 border-t border-slate-800/80">
                  <span className="text-[11px] text-slate-500">已执行 {tpl.usageCount || 0} 次</span>
                  <div className="flex items-center gap-1">
                    <Tooltip title="查看全部指令">
                      <Button
                        size="small"
                        type="text"
                        className="text-slate-300 hover:text-white"
                        icon={<Eye size={14} />}
                        onClick={() => setPreviewTemplate(tpl)}
                      />
                    </Tooltip>
                    <Tooltip title="克隆为自定义模版">
                      <Button
                        size="small"
                        type="text"
                        className="text-purple-400 hover:text-purple-300"
                        icon={<Copy size={14} />}
                        onClick={() => handleClone(tpl.id)}
                      />
                    </Tooltip>
                    <Tooltip title="编辑模版">
                      <Button
                        size="small"
                        type="text"
                        className="text-amber-400 hover:text-amber-300"
                        icon={<Edit size={14} />}
                        onClick={() => openEdit(tpl)}
                      />
                    </Tooltip>
                    <Tooltip title="导出 JSON">
                      <Button
                        size="small"
                        type="text"
                        className="text-emerald-400 hover:text-emerald-300"
                        icon={<Download size={14} />}
                        onClick={() => exportTemplateJSON(tpl)}
                      />
                    </Tooltip>
                    {!tpl.isPreset && (
                      <Popconfirm title="确定删除该自定义模版?" onConfirm={() => handleDelete(tpl.id)}>
                        <Tooltip title="删除">
                          <Button size="small" type="text" danger icon={<Trash2 size={14} />} />
                        </Tooltip>
                      </Popconfirm>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      ) : (
        <Table
          columns={columns}
          dataSource={data}
          rowKey="id"
          loading={loading}
          size="middle"
          pagination={{ pageSize: 15, showSizeChanger: true, showTotal: (t: number) => `共 ${t} 个模版` }}
          className="bg-slate-900/60 rounded-xl border border-slate-800 overflow-hidden"
        />
      )}

      {/* 模版编辑 / 创建弹窗 */}
      <Modal
        title={editing ? `编辑模版：${editing.name}` : '新建自定义巡检模版'}
        open={modalOpen}
        onOk={handleSave}
        onCancel={() => {
          setModalOpen(false);
          setEditing(null);
          setCommands([]);
        }}
        width={960}
        okText="保存模版"
        cancelText="取消"
        styles={{ body: { maxHeight: '72vh', overflow: 'auto' } }}
      >
        <Form form={form} layout="vertical" className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <Form.Item name="name" label="模版名称" rules={[{ required: true, message: '请输入模版名称' }]}>
              <Input placeholder="如：华为网络核心交换机日常巡检模版" />
            </Form.Item>
            <Form.Item name="vendor" label="适用厂商（设备巡检时严格匹配同品牌）" rules={[{ required: true }]}>
              <Select>
                <Select.Option value="huawei">华为 (Huawei)</Select.Option>
                <Select.Option value="zte">中兴 (ZTE)</Select.Option>
                <Select.Option value="cisco">思科 (Cisco)</Select.Option>
                <Select.Option value="h3c">华三 (H3C)</Select.Option>
                <Select.Option value="ruijie">锐捷 (Ruijie)</Select.Option>
                <Select.Option value="sangfor">深信服 (Sangfor)</Select.Option>
                <Select.Option value="maipu">迈普 (Maipu)</Select.Option>
                <Select.Option value="fortinet">飞塔 (Fortinet)</Select.Option>
                <Select.Option value="juniper">瞻博 (Juniper)</Select.Option>
                <Select.Option value="paloalto">PaloAlto</Select.Option>
                <Select.Option value="hpe">HPE / Aruba</Select.Option>
                <Select.Option value="other">通用 / 其他</Select.Option>
              </Select>
            </Form.Item>
          </div>

          <div className="grid grid-cols-3 gap-4">
            <Form.Item name="category" label="模版类别" rules={[{ required: true }]}>
              <Select>
                <Select.Option value="network">网络设备</Select.Option>
                <Select.Option value="server">服务器</Select.Option>
              </Select>
            </Form.Item>
            <Form.Item name="deviceType" label="设备类型">
              <Select allowClear>
                <Select.Option value="switch">交换机</Select.Option>
                <Select.Option value="router">路由器</Select.Option>
                <Select.Option value="firewall">防火墙</Select.Option>
                <Select.Option value="loadbalancer">负载均衡</Select.Option>
                <Select.Option value="server">服务器</Select.Option>
                <Select.Option value="unknown">通用/未知</Select.Option>
              </Select>
            </Form.Item>
            <Form.Item name="osType" label="操作系统 (服务器模版)">
              <Select allowClear>
                <Select.Option value="linux">Linux</Select.Option>
                <Select.Option value="windows">Windows</Select.Option>
              </Select>
            </Form.Item>
          </div>

          <Form.Item name="description" label="模版说明">
            <Input.TextArea rows={2} placeholder="详细说明模版的巡检目标和适用设备型号" />
          </Form.Item>

          <Form.Item name="tags" label="标签">
            <Select mode="tags" placeholder="输入标签后按回车" />
          </Form.Item>

          <Divider>
            <span className="text-sm font-semibold text-slate-300">巡检指令项配置（SmartKit 五层模型）</span>
          </Divider>

          <div className="flex items-center justify-between gap-3 mb-2">
            <div className="flex items-center gap-2">
              <Button size="small" type="primary" icon={<Plus size={12} />} onClick={() => addCommand('系统层')}>
                添加指令项
              </Button>
              <Button size="small" icon={<Sparkles size={12} />} onClick={preloadSmartKitServerItems}>
                一键导入 SmartKit 五层标准项
              </Button>
            </div>
            <span className="text-xs text-slate-400">已配置 {commands.length} 项指令</span>
          </div>

          <Collapse
            items={groupedCommands()}
            size="small"
            defaultActiveKey={['硬件层', '系统层', '资源层', '安全层', '应用层']}
            className="bg-slate-950/40 border-slate-800"
          />
        </Form>
      </Modal>

      {/* 模版指令详细预览 Drawer */}
      <Drawer
        title={
          previewTemplate && (
            <div className="flex items-center justify-between pr-4">
              <span className="text-base font-bold text-white">{previewTemplate.name}</span>
              <span className="text-xs px-2.5 py-0.5 rounded bg-indigo-500/20 text-indigo-300 border border-indigo-500/30">
                共 {previewTemplate.commands?.length || 0} 条指令
              </span>
            </div>
          )
        }
        open={!!previewTemplate}
        onClose={() => setPreviewTemplate(null)}
        width={680}
        extra={
          previewTemplate && (
            <Button
              size="small"
              icon={<Download size={13} />}
              onClick={() => exportTemplateJSON(previewTemplate)}
            >
              导出 JSON
            </Button>
          )
        }
      >
        {previewTemplate && (
          <div className="space-y-6">
            <div className="p-4 bg-slate-900/80 rounded-xl border border-slate-800 space-y-2">
              <div className="flex items-center gap-2 text-xs">
                <span className="text-slate-400">厂商:</span>
                <span className="font-semibold text-white">{vendorMeta[previewTemplate.vendor]?.label || previewTemplate.vendor}</span>
                <span className="text-slate-400 ml-3">类别:</span>
                <span className="font-semibold text-white">{previewTemplate.category === 'server' ? '服务器' : '网络设备'}</span>
              </div>
              <p className="text-xs text-slate-300 leading-relaxed">{previewTemplate.description || '暂无说明'}</p>
            </div>

            {/* 按五层展示 */}
            {['硬件层', '系统层', '资源层', '安全层', '应用层'].map(groupName => {
              const layerCmds = (previewTemplate.commands || []).filter(c => (c.group || '系统层') === groupName);
              if (layerCmds.length === 0) return null;
              const col = groupColors[groupName] || { badge: 'bg-slate-800 text-slate-300 border-slate-700' };

              return (
                <div key={groupName} className="space-y-2">
                  <div className="flex items-center gap-2">
                    <span className={`px-2.5 py-0.5 rounded text-xs font-semibold border ${col.badge}`}>
                      {groupName}
                    </span>
                    <span className="text-xs text-slate-400">({layerCmds.length} 条)</span>
                  </div>

                  <div className="space-y-2">
                    {layerCmds.map((cmd, i) => (
                      <div key={i} className="p-3 bg-slate-900/60 rounded-lg border border-slate-800/80 space-y-1.5">
                        <div className="flex items-center justify-between">
                          <span className="font-semibold text-xs text-slate-200">{cmd.name}</span>
                          <span className="text-[10px] font-mono text-slate-400 px-1.5 py-0.5 rounded bg-slate-950 border border-slate-800">
                            {cmd.type}
                          </span>
                        </div>
                        <div className="font-mono text-xs bg-slate-950 p-2 rounded text-emerald-400 select-all border border-slate-800/50">
                          {cmd.command}
                        </div>
                        {cmd.description && (
                          <p className="text-[11px] text-slate-400 leading-normal">{cmd.description}</p>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </Drawer>
    </div>
  );
}
