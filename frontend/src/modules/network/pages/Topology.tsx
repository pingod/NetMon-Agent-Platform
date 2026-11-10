/* eslint-disable @typescript-eslint/no-explicit-any */
import React, { useState, useEffect, useRef } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Network,
  Plus,
  Save,
  Upload,
  RefreshCw,
  Edit3,
  Eye,
  Sparkles,
  Image as ImageIcon,
  Link as LinkIcon,
  Cable,
  Zap
} from 'lucide-react';
import api from '@/lib/api';
import { useToast } from '@/contexts/ToastContext';
import { getAxiosErrorMessage } from '@/lib/errorHandler';
import { NetworkTopologyCanvas, TopologyLinkData } from '../components/topology/NetworkTopologyCanvas';
import { DeviceInterfaceDrawer, TopologyNodeData } from '../components/topology/DeviceInterfaceDrawer';

export default function Topology() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const fileInputRef = useRef<HTMLInputElement>(null);

  // 模式：监控查看模式 (view) / 拓扑编辑绘制模式 (edit)
  const [mode, setMode] = useState<'view' | 'edit'>('view');

  // 当前激活的自定义拓扑 ID
  const [currentTopologyId, setCurrentTopologyId] = useState<string>('default');
  const [topologyName, setTopologyName] = useState('全局骨干网络拓扑');

  // 画布数据状态
  const [nodes, setNodes] = useState<TopologyNodeData[]>([]);
  const [links, setLinks] = useState<TopologyLinkData[]>([]);
  const [backgroundImage, setBackgroundImage] = useState<string>('');
  const [bgOpacity, setBgOpacity] = useState<number>(0.35);

  // 选中查看的节点（用于打开右侧接口抽屉）
  const [selectedNode, setSelectedNode] = useState<TopologyNodeData | null>(null);

  // 模态框状态：添加新设备
  const [isAddDeviceModalOpen, setIsAddDeviceModalOpen] = useState(false);
  const [newDeviceForm, setNewDeviceForm] = useState({
    name: '',
    ip: '',
    type: 'core_switch',
    vendor: 'Huawei',
    model: 'CloudEngine 12800',
    selectedInventoryDeviceId: '',
  });

  // 模态框状态：添加连线
  const [isAddLinkModalOpen, setIsAddLinkModalOpen] = useState(false);
  const [newLinkForm, setNewLinkForm] = useState({
    sourceId: '',
    targetId: '',
    sourceInterface: 'GigabitEthernet0/0/1',
    targetInterface: 'GigabitEthernet0/0/1',
    bandwidth: '10 Gbps',
  });

  // 1. 获取系统保存的自定义拓扑列表
  const { data: savedTopologies, refetch: refetchSaved } = useQuery({
    queryKey: ['custom-topologies'],
    queryFn: async () => {
      const res = await api.get('/topology/custom');
      return res.data?.data || [];
    },
  });

  // 2. 获取实际网络设备资产列表（用于快速拖入绑定）
  const { data: inventoryDevices } = useQuery({
    queryKey: ['network-devices-inventory'],
    queryFn: async () => {
      const res = await api.get('/network-devices');
      return res.data?.data?.devices || res.data?.data || [];
    },
  });

  // 3. 轮询获取全网设备实时健康与接口 DOWN 状态 (每 8 秒自动刷新)
  const { data: liveStatusData, refetch: refetchLiveStatus } = useQuery({
    queryKey: ['topology-live-status'],
    queryFn: async () => {
      const res = await api.get('/topology/live-status');
      return res.data?.data?.devices || {};
    },
    refetchInterval: 8000,
  });

  // 4. 加载全局默认拓扑或从后端读取数据
  useEffect(() => {
    const loadInitialTopology = async () => {
      try {
        const res = await api.get('/topology/global');
        const globalData = res.data?.data;
        if (globalData && globalData.nodes && globalData.nodes.length > 0) {
          const formattedNodes: TopologyNodeData[] = globalData.nodes.map((n: any, idx: number) => ({
            id: n.id || `dev-${idx}`,
            name: n.name || n.label || `设备-${idx + 1}`,
            ip: n.ip || '192.168.1.1',
            type: n.type || 'switch',
            vendor: n.vendor || 'Huawei',
            status: n.status || 'online',
            x: n.x ?? 220 + (idx % 4) * 250,
            y: n.y ?? 200 + Math.floor(idx / 4) * 220,
          }));

          const formattedLinks: TopologyLinkData[] = (globalData.edges || []).map((e: any, idx: number) => ({
            id: e.id || `link-${idx}`,
            source: e.source,
            target: e.target,
            source_interface: e.source_interface || 'GE0/0/1',
            target_interface: e.target_interface || 'GE0/0/1',
            bandwidth: '10 Gbps',
            status: e.status || 'active',
          }));

          // 如果没有预设连线，自动建立连线以展示拓扑效果
          if (formattedLinks.length === 0 && formattedNodes.length >= 2) {
            for (let i = 0; i < formattedNodes.length - 1; i++) {
              formattedLinks.push({
                id: `auto-link-${i}`,
                source: formattedNodes[i].id,
                target: formattedNodes[i + 1].id,
                source_name: formattedNodes[i].name,
                target_name: formattedNodes[i + 1].name,
                source_interface: `GE0/0/${i + 1}`,
                target_interface: `GE0/0/${i + 1}`,
                bandwidth: '10 Gbps',
                status: 'active',
              });
            }
          }

          setNodes(formattedNodes);
          setLinks(formattedLinks);
        } else {
          seedDefaultTopology();
        }
      } catch {
        seedDefaultTopology();
      }
    };

    loadInitialTopology();
  }, []);

  // 预设高精细示例拓扑数据
  const seedDefaultTopology = () => {
    const demoNodes: TopologyNodeData[] = [
      { id: 'gw-cisco-01', name: 'WAN-Gateway-01', ip: '192.168.1.254', type: 'router', vendor: 'Cisco', status: 'online', x: 500, y: 100 },
      { id: 'fw-cisco-01', name: 'Core-Firewall-01', ip: '192.168.1.253', type: 'firewall', vendor: 'Cisco', status: 'online', x: 500, y: 240 },
      { id: 'sw-core-01', name: 'Core-Switch-01', ip: '192.168.1.1', type: 'core_switch', vendor: 'Huawei', status: 'online', down_interfaces_count: 2, down_interfaces: ['GigabitEthernet0/0/2', 'GigabitEthernet0/0/5'], x: 320, y: 380 },
      { id: 'sw-core-02', name: 'Core-Switch-02', ip: '192.168.1.2', type: 'core_switch', vendor: 'H3C', status: 'online', x: 680, y: 380 },
      { id: 'sw-acc-01', name: 'Floor-Access-01', ip: '192.168.2.1', type: 'switch', vendor: 'Huawei', status: 'online', x: 180, y: 530 },
      { id: 'sw-acc-02', name: 'Floor-Access-02', ip: '192.168.2.2', type: 'switch', vendor: 'H3C', status: 'online', x: 440, y: 530 },
      { id: 'srv-host-01', name: 'Kube-Master-01', ip: '192.168.10.11', type: 'server', vendor: 'Dell', status: 'online', x: 720, y: 530 },
      { id: 'san-storage-01', name: 'Enterprise-SAN-01', ip: '192.168.10.20', type: 'storage', vendor: 'Huawei', status: 'online', x: 920, y: 530 },
    ];

    const demoLinks: TopologyLinkData[] = [
      { id: 'l1', source: 'gw-cisco-01', target: 'fw-cisco-01', source_interface: 'GE0/0/0', target_interface: 'WAN1', bandwidth: '40 Gbps' },
      { id: 'l2', source: 'fw-cisco-01', target: 'sw-core-01', source_interface: 'LAN1', target_interface: 'XGE0/0/1', bandwidth: '10 Gbps' },
      { id: 'l3', source: 'fw-cisco-01', target: 'sw-core-02', source_interface: 'LAN2', target_interface: 'XGE0/0/1', bandwidth: '10 Gbps' },
      { id: 'l4', source: 'sw-core-01', target: 'sw-core-02', source_interface: 'Eth-Trunk1', target_interface: 'BAGG1', bandwidth: '100 Gbps' },
      { id: 'l5', source: 'sw-core-01', target: 'sw-acc-01', source_interface: 'GE0/0/2', target_interface: 'Uplink1', bandwidth: '1 Gbps', is_down: true },
      { id: 'l6', source: 'sw-core-01', target: 'sw-acc-02', source_interface: 'GE0/0/3', target_interface: 'Uplink1', bandwidth: '1 Gbps' },
      { id: 'l7', source: 'sw-core-02', target: 'srv-host-01', source_interface: 'GE1/0/1', target_interface: 'eth0', bandwidth: '10 Gbps' },
      { id: 'l8', source: 'sw-core-02', target: 'san-storage-01', source_interface: 'FC0/1', target_interface: 'HBA1', bandwidth: '32 Gbps' },
    ];

    setNodes(demoNodes);
    setLinks(demoLinks);
  };

  // 5. 合并实时状态到画布节点
  useEffect(() => {
    if (!liveStatusData || Object.keys(liveStatusData).length === 0) return;
    setNodes(prevNodes =>
      prevNodes.map(node => {
        const live = liveStatusData[node.id];
        if (!live) return node;
        return {
          ...node,
          status: live.status || node.status,
          down_interfaces_count: live.down_interfaces_count,
          down_interfaces: live.down_interfaces,
          active_alert_count: live.active_alert_count,
          max_severity: live.max_severity,
          alerts: live.alerts,
        };
      })
    );
  }, [liveStatusData]);

  // 保存当前拓扑到后端
  const saveMutation = useMutation({
    mutationFn: async () => {
      const payload = {
        name: topologyName,
        background_image: backgroundImage,
        nodes: nodes,
        links: links,
        is_default: 1,
      };
      if (currentTopologyId && currentTopologyId !== 'default' && currentTopologyId !== 'new') {
        const res = await api.put(`/topology/custom/${currentTopologyId}`, payload);
        return res.data;
      } else {
        const res = await api.post('/topology/custom', payload);
        return res.data;
      }
    },
    onSuccess: (data) => {
      toast.success('网络拓扑设计保存成功！');
      queryClient.invalidateQueries({ queryKey: ['custom-topologies'] });
      if (data?.data?.id) {
        setCurrentTopologyId(data.data.id);
      }
    },
    onError: (err) => {
      toast.error(getAxiosErrorMessage(err, '保存拓扑失败'));
    },
  });

  // 处理底图文件上传
  const handleBackgroundUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === 'string') {
        setBackgroundImage(reader.result);
        toast.success(`底图已加载: ${file.name}`);
      }
    };
    reader.readAsDataURL(file);
  };

  // 添加新设备节点
  const handleAddDeviceSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newDeviceForm.name) {
      toast.warning('请输入设备名称');
      return;
    }

    const newNode: TopologyNodeData = {
      id: newDeviceForm.selectedInventoryDeviceId || `custom-${Date.now()}`,
      name: newDeviceForm.name,
      ip: newDeviceForm.ip || '192.168.1.100',
      type: newDeviceForm.type,
      vendor: newDeviceForm.vendor,
      model: newDeviceForm.model,
      status: 'online',
      x: 450 + Math.floor(Math.random() * 120),
      y: 280 + Math.floor(Math.random() * 120),
    };

    setNodes(prev => [...prev, newNode]);
    setIsAddDeviceModalOpen(false);
    toast.success(`已添加设备节点: ${newDeviceForm.name}`);
    setNewDeviceForm({
      name: '',
      ip: '',
      type: 'core_switch',
      vendor: 'Huawei',
      model: 'CloudEngine 12800',
      selectedInventoryDeviceId: '',
    });
  };

  // 连线创建处理（从画布连线手柄或点击连线）
  const handleAddLink = (sourceId: string, targetId: string) => {
    const sourceNode = nodes.find(n => n.id === sourceId);
    const targetNode = nodes.find(n => n.id === targetId);

    // 检查是否已存在反向或正向连线
    const exists = links.some(
      l => (l.source === sourceId && l.target === targetId) || (l.source === targetId && l.target === sourceId)
    );
    if (exists) {
      toast.warning('这两台设备之间已存在连线');
      return;
    }

    const newLink: TopologyLinkData = {
      id: `link-${Date.now()}`,
      source: sourceId,
      target: targetId,
      source_name: sourceNode?.name,
      target_name: targetNode?.name,
      source_interface: 'GE0/0/1',
      target_interface: 'GE0/0/1',
      bandwidth: '10 Gbps',
      status: 'active',
    };
    setLinks(prev => [...prev, newLink]);
    toast.success(`已建立连线: ${sourceNode?.name || sourceId} ↔ ${targetNode?.name || targetId}`);
  };

  // 表单弹窗添加连线
  const handleAddLinkSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newLinkForm.sourceId || !newLinkForm.targetId) {
      toast.warning('请选择源设备和目标设备');
      return;
    }
    if (newLinkForm.sourceId === newLinkForm.targetId) {
      toast.warning('源设备与目标设备不能相同');
      return;
    }

    handleAddLink(newLinkForm.sourceId, newLinkForm.targetId);
    setIsAddLinkModalOpen(false);
  };

  // 一键自动建立骨干网推荐连线
  const handleAutoConnectAll = () => {
    if (nodes.length < 2) {
      toast.warning('至少需要 2 台设备才能建立连线');
      return;
    }

    const newLinksList: TopologyLinkData[] = [];
    for (let i = 0; i < nodes.length - 1; i++) {
      newLinksList.push({
        id: `auto-link-${i}-${Date.now()}`,
        source: nodes[i].id,
        target: nodes[i + 1].id,
        source_name: nodes[i].name,
        target_name: nodes[i + 1].name,
        source_interface: `GE0/0/${i + 1}`,
        target_interface: `GE0/0/${i + 1}`,
        bandwidth: '10 Gbps',
        status: 'active',
      });
    }

    setLinks(newLinksList);
    toast.success(`已为 ${nodes.length} 台设备自动建立骨干连线拓扑！`);
  };

  // 删除节点与关联连线
  const handleDeleteNode = (nodeId: string) => {
    setNodes(prev => prev.filter(n => n.id !== nodeId));
    setLinks(prev => prev.filter(l => l.source !== nodeId && l.target !== nodeId));
    if (selectedNode?.id === nodeId) {
      setSelectedNode(null);
    }
    toast.info('已移除节点');
  };

  // 删除连线
  const handleDeleteLink = (linkId: string) => {
    setLinks(prev => prev.filter(l => l.id !== linkId));
    toast.info('已移除连线');
  };

  // 自动排版算法 (分层层级布局)
  const handleAutoLayout = () => {
    const layerOrder: Record<string, number> = {
      cloud: 0,
      router: 1,
      gateway: 1,
      firewall: 2,
      core_switch: 3,
      core: 3,
      switch: 4,
      aggregation: 4,
      access: 4,
      server: 5,
      storage: 5,
      wireless_ap: 5,
    };

    const layers: Record<number, TopologyNodeData[]> = {};
    nodes.forEach(n => {
      const normType = (n.type || 'switch').toLowerCase();
      let layerIndex = 4;
      for (const [key, val] of Object.entries(layerOrder)) {
        if (normType.includes(key)) {
          layerIndex = val;
          break;
        }
      }
      if (!layers[layerIndex]) layers[layerIndex] = [];
      layers[layerIndex].push(n);
    });

    const arrangedNodes: TopologyNodeData[] = [];
    const sortedLayerKeys = Object.keys(layers).map(Number).sort((a, b) => a - b);

    sortedLayerKeys.forEach((layerKey, lIdx) => {
      const layerNodes = layers[layerKey];
      const count = layerNodes.length;
      const spacing = 900 / (count + 1);
      const layerY = 90 + lIdx * 120;

      layerNodes.forEach((node, nIdx) => {
        arrangedNodes.push({
          ...node,
          x: Math.round(150 + spacing * (nIdx + 1)),
          y: layerY,
        });
      });
    });

    setNodes(arrangedNodes);
    toast.success('已应用智能分层拓扑布局');
  };

  // 模拟接口 DOWN / 恢复开关
  const handleSimulateToggle = (nodeId: string, interfaceName: string, targetStatus: 'up' | 'down') => {
    setNodes(prevNodes =>
      prevNodes.map(n => {
        if (n.id !== nodeId) return n;
        const currentDown = n.down_interfaces || [];
        let nextDown: string[];
        if (targetStatus === 'down') {
          nextDown = Array.from(new Set([...currentDown, interfaceName]));
        } else {
          nextDown = currentDown.filter(name => name !== interfaceName);
        }
        return {
          ...n,
          down_interfaces_count: nextDown.length,
          down_interfaces: nextDown,
        };
      })
    );

    setLinks(prevLinks =>
      prevLinks.map(l => {
        if (l.source === nodeId || l.target === nodeId) {
          return {
            ...l,
            is_down: targetStatus === 'down',
          };
        }
        return l;
      })
    );

    toast.info(`模拟已切换: 接口 ${interfaceName} 状态为 ${targetStatus.toUpperCase()}`);
  };

  return (
    <div className="viz-dark p-6 space-y-6 max-w-[1600px] mx-auto text-slate-100 min-h-screen">
      {/* 顶部标题与控制栏 */}
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 p-5 rounded-2xl bg-slate-900/80 backdrop-blur-xl border border-slate-800 shadow-xl">
        <div className="flex items-center gap-3.5">
          <div className="p-2.5 rounded-xl bg-gradient-to-tr from-sky-600 to-indigo-600 text-white shadow-lg shadow-sky-500/20">
            <Network className="w-6 h-6" />
          </div>
          <div>
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-bold text-white tracking-tight">网络拓扑设计与实时监控中心</h1>
              <span className="px-2.5 py-0.5 text-xs font-semibold rounded-full bg-sky-500/20 text-sky-400 border border-sky-500/30 flex items-center gap-1.5">
                <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
                SNMP 采集与告警联动就绪
              </span>
            </div>
            <p className="text-xs text-slate-400 mt-1">
              支持可视化绘制拓扑、上传底图架构图、实时感知端口 UP/DOWN 状态与告警呼吸动效
            </p>
          </div>
        </div>

        {/* 模式切换与操作按钮组 */}
        <div className="flex flex-wrap items-center gap-2.5">
          {/* 模式切换切换器 */}
          <div className="flex p-1 bg-slate-950 rounded-xl border border-slate-800">
            <button
              onClick={() => setMode('view')}
              className={`px-3.5 py-1.5 text-xs font-semibold rounded-lg transition-all flex items-center gap-1.5 ${
                mode === 'view'
                  ? 'bg-sky-600 text-white shadow-md'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Eye className="w-3.5 h-3.5" />
              实时监控大盘
            </button>
            <button
              onClick={() => setMode('edit')}
              className={`px-3.5 py-1.5 text-xs font-semibold rounded-lg transition-all flex items-center gap-1.5 ${
                mode === 'edit'
                  ? 'bg-indigo-600 text-white shadow-md'
                  : 'text-slate-400 hover:text-slate-200'
              }`}
            >
              <Edit3 className="w-3.5 h-3.5" />
              画拓扑 / 编辑设计
            </button>
          </div>

          {/* 编辑模式下的专属工具 */}
          {mode === 'edit' && (
            <>
              <button
                onClick={() => setIsAddDeviceModalOpen(true)}
                className="px-3 py-1.5 text-xs bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg border border-slate-700 transition-colors flex items-center gap-1.5"
              >
                <Plus className="w-3.5 h-3.5 text-sky-400" />
                添加设备
              </button>

              <button
                onClick={() => {
                  if (nodes.length >= 2) {
                    setNewLinkForm(prev => ({
                      ...prev,
                      sourceId: nodes[0].id,
                      targetId: nodes[1].id,
                    }));
                  }
                  setIsAddLinkModalOpen(true);
                }}
                className="px-3 py-1.5 text-xs bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg border border-slate-700 transition-colors flex items-center gap-1.5"
                title="选择设备添加新连线"
              >
                <Cable className="w-3.5 h-3.5 text-emerald-400" />
                添加连线
              </button>

              <button
                onClick={handleAutoConnectAll}
                className="px-3 py-1.5 text-xs bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg border border-slate-700 transition-colors flex items-center gap-1.5"
                title="自动为现有设备生成骨干连接"
              >
                <Zap className="w-3.5 h-3.5 text-amber-400" />
                一键全互联
              </button>

              <button
                onClick={() => fileInputRef.current?.click()}
                className="px-3 py-1.5 text-xs bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg border border-slate-700 transition-colors flex items-center gap-1.5"
                title="上传底图/网络架构图 (PNG/JPG/SVG)"
              >
                <Upload className="w-3.5 h-3.5 text-indigo-400" />
                上传拓扑底图
              </button>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                className="hidden"
                onChange={handleBackgroundUpload}
              />

              <button
                onClick={handleAutoLayout}
                className="px-3 py-1.5 text-xs bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg border border-slate-700 transition-colors flex items-center gap-1.5"
                title="自动按核心-汇聚-接入-终端分层排版"
              >
                <Sparkles className="w-3.5 h-3.5 text-amber-400" />
                智能分层排版
              </button>

              <button
                onClick={() => saveMutation.mutate()}
                disabled={saveMutation.isPending}
                className="px-4 py-1.5 text-xs font-semibold bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg transition-colors flex items-center gap-1.5 shadow-lg shadow-emerald-600/30 disabled:opacity-50"
              >
                <Save className="w-3.5 h-3.5" />
                {saveMutation.isPending ? '保存中...' : '保存拓扑'}
              </button>
            </>
          )}

          {/* 刷新指标 */}
          <button
            onClick={() => refetchLiveStatus()}
            className="p-2 text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg border border-slate-800 transition-colors"
            title="手动刷新实时状态"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
        </div>
      </div>

      {/* 编辑模式下的底图参数控制条 */}
      {mode === 'edit' && backgroundImage && (
        <div className="p-3 bg-slate-900/60 rounded-xl border border-slate-800 flex items-center justify-between text-xs text-slate-300">
          <div className="flex items-center gap-3">
            <ImageIcon className="w-4 h-4 text-sky-400" />
            <span>已加载自定义拓扑底图</span>
            <span className="text-slate-500">|</span>
            <label className="flex items-center gap-2">
              <span>底图透明度:</span>
              <input
                type="range"
                min="0.05"
                max="0.9"
                step="0.05"
                value={bgOpacity}
                onChange={(e) => setBgOpacity(parseFloat(e.target.value))}
                className="w-28 accent-sky-500 cursor-pointer"
              />
              <span className="font-mono">{Math.round(bgOpacity * 100)}%</span>
            </label>
          </div>

          <button
            onClick={() => setBackgroundImage('')}
            className="text-red-400 hover:text-red-300 text-xs"
          >
            移除底图
          </button>
        </div>
      )}

      {/* 核心画布区域 */}
      <NetworkTopologyCanvas
        nodes={nodes}
        links={links}
        backgroundImage={backgroundImage}
        bgOpacity={bgOpacity}
        isEditMode={mode === 'edit'}
        selectedNodeId={selectedNode?.id || null}
        onSelectNode={(n) => setSelectedNode(n)}
        onUpdateNodes={(updated) => setNodes(updated)}
        onAddLink={handleAddLink}
        onDeleteNode={handleDeleteNode}
        onDeleteLink={handleDeleteLink}
      />

      {/* 设备接口与告警详情抽屉 */}
      <DeviceInterfaceDrawer
        node={selectedNode}
        onClose={() => setSelectedNode(null)}
        onSimulateToggle={handleSimulateToggle}
      />

      {/* 模态弹窗：添加连线 */}
      {isAddLinkModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
          <div className="w-full max-w-md bg-slate-900 border border-slate-700 rounded-2xl p-6 shadow-2xl space-y-4">
            <h3 className="text-lg font-bold text-white flex items-center gap-2">
              <Cable className="w-5 h-5 text-emerald-400" />
              新建设备物理/逻辑连线
            </h3>

            <form onSubmit={handleAddLinkSubmit} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">源端设备 (Source Device) *</label>
                <select
                  required
                  value={newLinkForm.sourceId}
                  onChange={(e) => setNewLinkForm({ ...newLinkForm, sourceId: e.target.value })}
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                >
                  <option value="">-- 请选择源设备 --</option>
                  {nodes.map(n => (
                    <option key={n.id} value={n.id}>
                      {n.name} ({n.ip || '无IP'})
                    </option>
                  ))}
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">目标设备 (Target Device) *</label>
                <select
                  required
                  value={newLinkForm.targetId}
                  onChange={(e) => setNewLinkForm({ ...newLinkForm, targetId: e.target.value })}
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                >
                  <option value="">-- 请选择目标设备 --</option>
                  {nodes.map(n => (
                    <option key={n.id} value={n.id}>
                      {n.name} ({n.ip || '无IP'})
                    </option>
                  ))}
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">源端口名称</label>
                  <input
                    type="text"
                    value={newLinkForm.sourceInterface}
                    onChange={(e) => setNewLinkForm({ ...newLinkForm, sourceInterface: e.target.value })}
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500 font-mono"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">目标端口名称</label>
                  <input
                    type="text"
                    value={newLinkForm.targetInterface}
                    onChange={(e) => setNewLinkForm({ ...newLinkForm, targetInterface: e.target.value })}
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500 font-mono"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">链路带宽 (Bandwidth)</label>
                <select
                  value={newLinkForm.bandwidth}
                  onChange={(e) => setNewLinkForm({ ...newLinkForm, bandwidth: e.target.value })}
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                >
                  <option value="1 Gbps">1 Gbps (千兆以太网)</option>
                  <option value="10 Gbps">10 Gbps (万兆光纤)</option>
                  <option value="40 Gbps">40 Gbps (QSFP+)</option>
                  <option value="100 Gbps">100 Gbps (骨干聚合)</option>
                </select>
              </div>

              <div className="flex justify-end gap-2.5 pt-2">
                <button
                  type="button"
                  onClick={() => setIsAddLinkModalOpen(false)}
                  className="px-4 py-2 text-sm bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg transition-colors"
                >
                  取消
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 text-sm font-semibold bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg transition-colors shadow-lg shadow-emerald-600/30"
                >
                  确认建立连线
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* 模态弹窗：添加设备节点 */}
      {isAddDeviceModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
          <div className="w-full max-w-md bg-slate-900 border border-slate-700 rounded-2xl p-6 shadow-2xl space-y-4">
            <h3 className="text-lg font-bold text-white flex items-center gap-2">
              <Plus className="w-5 h-5 text-sky-400" />
              添加拓扑设备节点
            </h3>

            <form onSubmit={handleAddDeviceSubmit} className="space-y-3.5">
              {/* 从现有资产选择 */}
              {inventoryDevices && inventoryDevices.length > 0 && (
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    从现有网络资产快速导入（可选）
                  </label>
                  <select
                    value={newDeviceForm.selectedInventoryDeviceId}
                    onChange={(e) => {
                      const devId = e.target.value;
                      const matched = inventoryDevices.find((d: any) => d.id === devId);
                      if (matched) {
                        setNewDeviceForm({
                          selectedInventoryDeviceId: devId,
                          name: matched.name,
                          ip: matched.ip_address || matched.ip,
                          type: matched.role || matched.device_type || 'switch',
                          vendor: matched.vendor || 'Huawei',
                          model: matched.model || 'CE12800',
                        });
                      } else {
                        setNewDeviceForm(prev => ({ ...prev, selectedInventoryDeviceId: '' }));
                      }
                    }}
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                  >
                    <option value="">-- 手动创建新设备 --</option>
                    {inventoryDevices.map((d: any) => (
                      <option key={d.id} value={d.id}>
                        {d.name} ({d.ip_address || d.ip}) - {d.role || d.device_type}
                      </option>
                    ))}
                  </select>
                </div>
              )}

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">设备名称 *</label>
                <input
                  type="text"
                  required
                  placeholder="例如: Core-SW-01"
                  value={newDeviceForm.name}
                  onChange={(e) => setNewDeviceForm({ ...newDeviceForm, name: e.target.value })}
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">设备类型</label>
                  <select
                    value={newDeviceForm.type}
                    onChange={(e) => setNewDeviceForm({ ...newDeviceForm, type: e.target.value })}
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                  >
                    <option value="core_switch">核心交换机 (Chassis)</option>
                    <option value="switch">接入/汇聚交换机 (1U)</option>
                    <option value="router">企业路由器 (Router)</option>
                    <option value="firewall">安全防火墙 (Firewall)</option>
                    <option value="server">服务器主机 (Server)</option>
                    <option value="wireless_ap">无线 AP (Wi-Fi)</option>
                    <option value="storage">存储阵列 (SAN)</option>
                    <option value="cloud">云网关 / 出口 (Cloud)</option>
                  </select>
                </div>

                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">管理 IP</label>
                  <input
                    type="text"
                    placeholder="192.168.1.1"
                    value={newDeviceForm.ip}
                    onChange={(e) => setNewDeviceForm({ ...newDeviceForm, ip: e.target.value })}
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500 font-mono"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">厂商</label>
                  <input
                    type="text"
                    placeholder="Huawei / Cisco / H3C"
                    value={newDeviceForm.vendor}
                    onChange={(e) => setNewDeviceForm({ ...newDeviceForm, vendor: e.target.value })}
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">设备型号</label>
                  <input
                    type="text"
                    placeholder="CloudEngine 12800"
                    value={newDeviceForm.model}
                    onChange={(e) => setNewDeviceForm({ ...newDeviceForm, model: e.target.value })}
                    className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-sm text-slate-200 focus:outline-none focus:border-sky-500"
                  />
                </div>
              </div>

              <div className="flex justify-end gap-2.5 pt-3">
                <button
                  type="button"
                  onClick={() => setIsAddDeviceModalOpen(false)}
                  className="px-4 py-2 text-sm bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg transition-colors"
                >
                  取消
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 text-sm font-semibold bg-sky-600 hover:bg-sky-500 text-white rounded-lg transition-colors"
                >
                  确认添加
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
