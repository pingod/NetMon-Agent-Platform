/* eslint-disable @typescript-eslint/no-explicit-any */
import React, { useRef, useState, useEffect, useCallback, useMemo } from 'react';
import clsx from 'clsx';
import { renderDeviceIcon } from './DeviceIcons';
import { TopologyNodeData } from './DeviceInterfaceDrawer';
import {
  ZoomIn,
  ZoomOut,
  Maximize2,
  Minimize2,
  RotateCcw,
  Focus,
  AlertTriangle,
  Link as LinkIcon,
  Trash2,
  X,
  Radio,
  Plus,
  Monitor
} from 'lucide-react';

export interface TopologyLinkData {
  id: string;
  source: string;
  target: string;
  source_name?: string;
  source_interface?: string;
  target_name?: string;
  target_interface?: string;
  status?: string;
  bandwidth?: string;
  is_down?: boolean;
}

interface NetworkTopologyCanvasProps {
  nodes: TopologyNodeData[];
  links: TopologyLinkData[];
  backgroundImage?: string;
  bgOpacity?: number;
  isEditMode: boolean;
  selectedNodeId: string | null;
  onSelectNode: (node: TopologyNodeData | null) => void;
  onUpdateNodes: (nodes: TopologyNodeData[]) => void;
  onAddLink?: (sourceId: string, targetId: string) => void;
  onDeleteNode?: (nodeId: string) => void;
  onDeleteLink?: (linkId: string) => void;
}

export const NetworkTopologyCanvas: React.FC<NetworkTopologyCanvasProps> = ({
  nodes,
  links,
  backgroundImage,
  bgOpacity = 0.5,
  isEditMode,
  selectedNodeId,
  onSelectNode,
  onUpdateNodes,
  onAddLink,
  onDeleteNode,
  onDeleteLink
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [transform, setTransform] = useState({ x: 0, y: 0, scale: 1 });
  const [isPanning, setIsPanning] = useState(false);
  const [panStart, setPanStart] = useState({ x: 0, y: 0 });

  // 拖拽节点状态
  const [draggingNodeId, setDraggingNodeId] = useState<string | null>(null);
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 });

  // 连线发起状态
  const [connectingSourceId, setConnectingSourceId] = useState<string | null>(null);
  const [mousePos, setMousePos] = useState<{ x: number; y: number } | null>(null);

  // 节点 Map 缓存
  const nodeMap = useMemo(() => {
    const map = new Map<string, TopologyNodeData>();
    nodes.forEach(n => map.set(n.id, n));
    return map;
  }, [nodes]);

  // 全屏状态与控制
  const [isFullscreen, setIsFullscreen] = useState(false);

  const toggleFullscreen = useCallback(() => {
    if (!isFullscreen && !document.fullscreenElement) {
      if (containerRef.current?.requestFullscreen) {
        containerRef.current.requestFullscreen().then(() => {
          setIsFullscreen(true);
        }).catch(() => {
          // Fallback to CSS Fullscreen (in iframe or security block)
          setIsFullscreen(true);
        });
      } else {
        setIsFullscreen(true);
      }
    } else {
      if (document.fullscreenElement) {
        document.exitFullscreen().catch(() => {});
      }
      setIsFullscreen(false);
    }
  }, [isFullscreen]);

  // 同步原生浏览器全屏状态
  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(!!document.fullscreenElement);
    };
    document.addEventListener('fullscreenchange', handleFullscreenChange);
    document.addEventListener('webkitfullscreenchange', handleFullscreenChange);
    return () => {
      document.removeEventListener('fullscreenchange', handleFullscreenChange);
      document.removeEventListener('webkitfullscreenchange', handleFullscreenChange);
    };
  }, []);

  // ESC 键取消连线模式或退出全屏
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (connectingSourceId) {
          setConnectingSourceId(null);
        } else if (isFullscreen) {
          if (document.fullscreenElement) {
            document.exitFullscreen().catch(() => {});
          }
          setIsFullscreen(false);
        }
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [connectingSourceId, isFullscreen]);

  // 初始化节点坐标
  useEffect(() => {
    if (nodes.length === 0) return;
    let needsUpdate = false;
    const updated = nodes.map((node, index) => {
      if (node.x === undefined || node.y === undefined) {
        needsUpdate = true;
        const total = nodes.length;
        const radius = Math.min(320, 120 + total * 30);
        const angle = (index / total) * 2 * Math.PI - Math.PI / 2;
        return {
          ...node,
          x: Math.round(520 + radius * Math.cos(angle)),
          y: Math.round(300 + radius * Math.sin(angle)),
        };
      }
      return node;
    });
    if (needsUpdate) {
      onUpdateNodes(updated);
    }
  }, [nodes.length]);

  // 缩放控制
  const handleZoom = (delta: number) => {
    setTransform(prev => {
      const nextScale = Math.min(2.5, Math.max(0.4, prev.scale + delta));
      return { ...prev, scale: nextScale };
    });
  };

  const handleResetZoom = () => {
    setTransform({ x: 0, y: 0, scale: 1 });
  };

  const handleWheel = (e: React.WheelEvent) => {
    e.preventDefault();
    const zoomFactor = e.deltaY < 0 ? 0.08 : -0.08;
    handleZoom(zoomFactor);
  };

  // 画布平移处理
  const handleMouseDown = (e: React.MouseEvent) => {
    // 只有在点击画布背景时触发平移
    if (e.button === 0 && (e.target === containerRef.current || (e.target as HTMLElement).tagName === 'svg')) {
      setIsPanning(true);
      setPanStart({ x: e.clientX - transform.x, y: e.clientY - transform.y });
    }
  };

  const handleMouseMove = (e: React.MouseEvent) => {
    const rect = containerRef.current?.getBoundingClientRect();
    if (rect) {
      const canvasX = (e.clientX - rect.left - transform.x) / transform.scale;
      const canvasY = (e.clientY - rect.top - transform.y) / transform.scale;
      setMousePos({ x: canvasX, y: canvasY });
    }

    if (isPanning) {
      setTransform(prev => ({
        ...prev,
        x: e.clientX - panStart.x,
        y: e.clientY - panStart.y,
      }));
    } else if (draggingNodeId && isEditMode) {
      if (!rect) return;
      const canvasX = (e.clientX - rect.left - transform.x) / transform.scale;
      const canvasY = (e.clientY - rect.top - transform.y) / transform.scale;

      const updated = nodes.map(n =>
        n.id === draggingNodeId
          ? { ...n, x: Math.round(canvasX - dragOffset.x), y: Math.round(canvasY - dragOffset.y) }
          : n
      );
      onUpdateNodes(updated);
    }
  };

  const handleMouseUp = () => {
    setIsPanning(false);
    setDraggingNodeId(null);
  };

  // 点击节点卡片或拖拽节点
  const handleNodeClickOrMouseDown = (e: React.MouseEvent, node: TopologyNodeData) => {
    e.stopPropagation();

    // 如果当前正处于连线模式
    if (connectingSourceId) {
      if (connectingSourceId !== node.id) {
        // 完成连线
        if (onAddLink) {
          onAddLink(connectingSourceId, node.id);
        }
        setConnectingSourceId(null);
      } else {
        // 点击自身取消
        setConnectingSourceId(null);
      }
      return;
    }

    // 选中该节点查看抽屉
    onSelectNode(node);

    // 编辑模式下开始拖拽
    if (isEditMode && e.button === 0) {
      setDraggingNodeId(node.id);
      const rect = containerRef.current?.getBoundingClientRect();
      if (rect) {
        const canvasX = (e.clientX - rect.left - transform.x) / transform.scale;
        const canvasY = (e.clientY - rect.top - transform.y) / transform.scale;
        setDragOffset({
          x: canvasX - (node.x || 0),
          y: canvasY - (node.y || 0),
        });
      }
    }
  };

  // 点击“连线”按钮发起连线
  const handleStartConnect = (e: React.MouseEvent, nodeId: string) => {
    e.stopPropagation();
    if (!isEditMode) return;

    if (connectingSourceId === nodeId) {
      setConnectingSourceId(null);
    } else {
      setConnectingSourceId(nodeId);
    }
  };

  const connectingSourceNode = connectingSourceId ? nodeMap.get(connectingSourceId) : null;

  return (
    <div
      ref={containerRef}
      onMouseDown={handleMouseDown}
      onMouseMove={handleMouseMove}
      onMouseUp={handleMouseUp}
      onWheel={handleWheel}
      className={clsx(
        'relative bg-slate-950 border overflow-hidden select-none shadow-inner transition-all',
        isFullscreen
          ? 'fixed inset-0 z-[9999] w-screen h-screen rounded-none border-none'
          : 'w-full h-[680px] rounded-2xl border-slate-800/80',
        connectingSourceId
          ? 'cursor-crosshair border-sky-500/80 ring-2 ring-sky-500/30'
          : 'cursor-grab active:cursor-grabbing',
      )}
    >
      {/* 背景网格点阵 */}
      <div
        className="absolute inset-0 pointer-events-none opacity-25"
        style={{
          backgroundImage:
            'radial-gradient(circle, #38bdf8 1px, transparent 1px), radial-gradient(circle, #38bdf8 1px, transparent 1px)',
          backgroundSize: '24px 24px',
          backgroundPosition: `${transform.x}px ${transform.y}px`,
        }}
      />

      {/* 顶部连线引导横幅提示 */}
      {connectingSourceId && connectingSourceNode && (
        <div className="absolute top-4 left-1/2 -translate-x-1/2 z-50 flex items-center gap-3 px-5 py-2.5 rounded-2xl bg-sky-950/95 border border-sky-400 text-white shadow-2xl backdrop-blur-xl animate-in slide-in-from-top-3">
          <Radio className="w-4 h-4 text-sky-400 animate-pulse" />
          <span className="text-sm font-medium">
            正在从 <strong className="text-sky-300 font-bold underline">{connectingSourceNode.name}</strong> 连线：请点击目标设备以建立连接
          </span>
          <button
            onClick={() => setConnectingSourceId(null)}
            className="px-2.5 py-1 text-xs bg-red-600/80 hover:bg-red-500 text-white rounded-lg transition-colors flex items-center gap-1"
          >
            <X className="w-3.5 h-3.5" />
            取消 (ESC)
          </button>
        </div>
      )}

      {/* 缩放/平移容器 */}
      <div
        className="absolute inset-0 origin-top-left transition-transform duration-75"
        style={{
          transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})`,
        }}
      >
        {/* 上传的拓扑底图 */}
        {backgroundImage && (
          <div
            className="absolute inset-0 pointer-events-none transition-opacity duration-300"
            style={{ opacity: bgOpacity }}
          >
            <img
              src={backgroundImage}
              alt="Topology Blueprint"
              className="max-w-none rounded-xl"
              style={{ width: '1300px', height: '850px', objectFit: 'contain' }}
            />
          </div>
        )}

        {/* SVG 连线层 */}
        <svg
          className="absolute inset-0 w-[3000px] h-[2000px] pointer-events-none"
          style={{ overflow: 'visible' }}
        >
          <defs>
            <linearGradient id="linkNormal" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#0284c7" />
              <stop offset="100%" stopColor="#38bdf8" />
            </linearGradient>
            <linearGradient id="linkAlert" x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#ef4444" />
              <stop offset="100%" stopColor="#f87171" />
            </linearGradient>
            <filter id="linkGlow">
              <feDropShadow dx="0" dy="0" stdDeviation="3" floodColor="#38bdf8" floodOpacity="0.8" />
            </filter>
            <filter id="alertGlow">
              <feDropShadow dx="0" dy="0" stdDeviation="4" floodColor="#ef4444" floodOpacity="0.9" />
            </filter>
          </defs>

          {/* 渲染已有连线 */}
          {links.map(link => {
            const sourceNode = nodeMap.get(link.source);
            const targetNode = nodeMap.get(link.target);
            if (!sourceNode || !targetNode) return null;

            const x1 = sourceNode.x ?? 500;
            const y1 = sourceNode.y ?? 300;
            const x2 = targetNode.x ?? 600;
            const y2 = targetNode.y ?? 300;

            const isLinkDown =
              link.is_down ||
              link.status === 'down' ||
              (sourceNode.down_interfaces_count ?? 0) > 0 ||
              (targetNode.down_interfaces_count ?? 0) > 0;

            const midX = (x1 + x2) / 2;
            const midY = (y1 + y2) / 2;

            return (
              <g key={link.id} className="pointer-events-auto group">
                <line
                  x1={x1}
                  y1={y1}
                  x2={x2}
                  y2={y2}
                  stroke={isLinkDown ? 'url(#linkAlert)' : 'url(#linkNormal)'}
                  strokeWidth={isLinkDown ? 3.5 : 2.5}
                  strokeDasharray={isLinkDown ? '6,6' : undefined}
                  filter={isLinkDown ? 'url(#alertGlow)' : 'url(#linkGlow)'}
                  className={isLinkDown ? 'animate-pulse' : ''}
                />

                {/* 动态流量流动粒子 */}
                {!isLinkDown && (
                  <circle r="3.5" fill="#a7f3d0">
                    <animateMotion
                      path={`M ${x1} ${y1} L ${x2} ${y2}`}
                      dur="2.5s"
                      repeatCount="indefinite"
                    />
                  </circle>
                )}

                {/* 中间状态/接口标签 */}
                <g transform={`translate(${midX}, ${midY})`}>
                  {isLinkDown ? (
                    <g className="cursor-pointer">
                      <rect x="-38" y="-12" width="76" height="24" rx="6" fill="#450a0a" stroke="#ef4444" strokeWidth="1.5" />
                      <text x="0" y="4" fill="#fca5a5" fontSize="11" fontWeight="bold" textAnchor="middle">
                        ⚠️ 接口 DOWN
                      </text>
                    </g>
                  ) : (
                    <g className="opacity-80 hover:opacity-100 transition-opacity">
                      <rect x="-32" y="-10" width="64" height="20" rx="4" fill="#0f172a" stroke="#334155" strokeWidth="1" />
                      <text x="0" y="4" fill="#94a3b8" fontSize="10" textAnchor="middle">
                        {link.bandwidth || '10 Gbps'}
                      </text>
                    </g>
                  )}
                </g>

                {/* 端口标签 */}
                {link.source_interface && (
                  <text
                    x={x1 + (x2 - x1) * 0.22}
                    y={y1 + (y2 - y1) * 0.22 - 6}
                    fill="#38bdf8"
                    fontSize="9"
                    fontFamily="monospace"
                    textAnchor="middle"
                    className="opacity-75"
                  >
                    {link.source_interface}
                  </text>
                )}
                {link.target_interface && (
                  <text
                    x={x1 + (x2 - x1) * 0.78}
                    y={y1 + (y2 - y1) * 0.78 - 6}
                    fill="#38bdf8"
                    fontSize="9"
                    fontFamily="monospace"
                    textAnchor="middle"
                    className="opacity-75"
                  >
                    {link.target_interface}
                  </text>
                )}

                {/* 编辑模式下的删除连线按钮 */}
                {isEditMode && onDeleteLink && (
                  <g
                    transform={`translate(${midX + 24}, ${midY - 14})`}
                    className="cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity"
                    onClick={(e) => {
                      e.stopPropagation();
                      onDeleteLink(link.id);
                    }}
                  >
                    <circle cx="0" cy="0" r="9" fill="#ef4444" />
                    <text x="0" y="3.5" fill="#fff" fontSize="12" textAnchor="middle" fontWeight="bold">
                      ×
                    </text>
                  </g>
                )}
              </g>
            );
          })}

          {/* 实时橡皮筋引导连线 */}
          {connectingSourceNode && mousePos && (
            <line
              x1={connectingSourceNode.x ?? 0}
              y1={connectingSourceNode.y ?? 0}
              x2={mousePos.x}
              y2={mousePos.y}
              stroke="#38bdf8"
              strokeWidth="2.5"
              strokeDasharray="6,4"
              className="animate-pulse"
            />
          )}
        </svg>

        {/* 节点渲染层 */}
        {nodes.map(node => {
          const isSelected = selectedNodeId === node.id;
          const hasDown = (node.down_interfaces_count ?? 0) > 0;
          const hasAlert = hasDown || !!node.active_alert_count || node.status === 'offline';
          const isConnectingSource = connectingSourceId === node.id;
          const isConnectTargetCandidate = connectingSourceId && connectingSourceId !== node.id;

          const nx = node.x ?? 500;
          const ny = node.y ?? 300;

          return (
            <div
              key={node.id}
              style={{
                left: `${nx}px`,
                top: `${ny}px`,
                transform: 'translate(-50%, -50%)',
              }}
              onMouseDown={(e) => handleNodeClickOrMouseDown(e, node)}
              className={`absolute cursor-pointer select-none transition-all group ${
                isSelected ? 'z-30' : 'z-20'
              }`}
            >
              {/* 告警红色呼吸光环 */}
              {hasAlert && (
                <div
                  className="absolute -inset-4 rounded-3xl bg-red-500/25 animate-ping pointer-events-none"
                  style={{ animationDuration: '2s' }}
                />
              )}

              {/* 连线发起者的高亮光晕 */}
              {isConnectingSource && (
                <div
                  className="absolute -inset-4 rounded-3xl bg-sky-500/30 animate-pulse pointer-events-none ring-2 ring-sky-400"
                />
              )}

              {/* 节点外层卡片 */}
              <div
                className={`relative flex flex-col items-center p-3 rounded-2xl border transition-all duration-200 ${
                  isConnectingSource
                    ? 'bg-slate-900/95 border-sky-400 shadow-2xl shadow-sky-500/40 ring-2 ring-sky-400 scale-110'
                    : isConnectTargetCandidate
                    ? 'bg-slate-900/90 border-sky-500/80 shadow-lg hover:scale-110 hover:border-sky-300 hover:ring-2 hover:ring-sky-400'
                    : isSelected
                    ? 'bg-slate-900/90 border-sky-400 shadow-xl shadow-sky-500/20 scale-105'
                    : hasAlert
                    ? 'bg-slate-900/85 border-red-500/80 shadow-lg shadow-red-950/60 hover:scale-105'
                    : 'bg-slate-900/70 border-slate-700/60 hover:border-slate-500 hover:scale-105'
                }`}
              >
                {/* 连线目标候选提示标签 */}
                {isConnectTargetCandidate && (
                  <div className="absolute -top-3 px-2 py-0.5 bg-sky-500 text-white font-bold text-[10px] rounded-full shadow-lg animate-bounce">
                    点击连接
                  </div>
                )}

                {/* 真实设备图标 */}
                <div className="relative">
                  {renderDeviceIcon(node.type, {
                    size: 56,
                    status: node.status,
                    hasAlert,
                    downCount: node.down_interfaces_count,
                  })}

                  {/* 接口 DOWN 告警角标 */}
                  {hasDown && (
                    <div className="absolute -top-2 -right-3 px-1.5 py-0.5 bg-red-600 text-white font-bold text-[10px] rounded-full border border-red-300 shadow-md animate-bounce flex items-center gap-0.5">
                      <AlertTriangle className="w-2.5 h-2.5" />
                      {node.down_interfaces_count} DOWN
                    </div>
                  )}
                </div>

                {/* 设备名称与 IP */}
                <div className="mt-2 text-center">
                  <div className="text-xs font-bold text-slate-100 tracking-tight max-w-[120px] truncate">
                    {node.name}
                  </div>
                  {node.ip && (
                    <div className="text-[10px] text-sky-400 font-mono mt-0.5">
                      {node.ip}
                    </div>
                  )}
                </div>

                {/* 编辑模式下的连线按钮 */}
                {isEditMode && (
                  <button
                    type="button"
                    onClick={(e) => handleStartConnect(e, node.id)}
                    className={`mt-2 px-2.5 py-1 text-[11px] font-semibold rounded-lg flex items-center gap-1 transition-all ${
                      isConnectingSource
                        ? 'bg-red-600 hover:bg-red-500 text-white shadow-md'
                        : 'bg-sky-600/30 hover:bg-sky-500 text-sky-300 hover:text-white border border-sky-500/40 shadow-sm'
                    }`}
                    title="点击以连接到其他设备"
                  >
                    <LinkIcon className="w-3 h-3" />
                    {isConnectingSource ? '取消连线' : '连线'}
                  </button>
                )}

                {/* 编辑模式下的删除按钮 */}
                {isEditMode && onDeleteNode && isSelected && (
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation();
                      onDeleteNode(node.id);
                    }}
                    className="absolute -top-2 -left-2 p-1.5 bg-red-600 hover:bg-red-500 text-white rounded-full shadow-lg transition-colors"
                    title="删除节点"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                )}
              </div>
            </div>
          );
        })}
      </div>

      {/* 全屏退出提醒横幅 */}
      {isFullscreen && (
        <div className="absolute top-4 left-1/2 -translate-x-1/2 z-50 flex items-center gap-3 px-4 py-2 rounded-2xl bg-slate-900/95 border border-sky-500/40 text-slate-200 shadow-2xl backdrop-blur-xl animate-in slide-in-from-top-3">
          <Monitor className="w-4 h-4 text-sky-400" />
          <span className="text-xs font-medium">全屏监控模式中 (可按 ESC 或点击右上角退出)</span>
          <button
            onClick={toggleFullscreen}
            className="px-2.5 py-1 text-xs bg-sky-600/30 hover:bg-sky-600/50 text-sky-300 rounded-lg border border-sky-500/40 transition-colors"
          >
            退出全屏
          </button>
        </div>
      )}

      {/* 悬浮操作工具条 */}
      <div className="absolute top-4 right-4 z-40 flex items-center gap-1.5 p-1.5 rounded-xl bg-slate-900/90 backdrop-blur-md border border-slate-700/80 shadow-2xl">
        <button
          onClick={() => handleZoom(0.15)}
          className="p-2 text-slate-300 hover:text-white hover:bg-slate-800 rounded-lg transition-colors"
          title="放大"
        >
          <ZoomIn className="w-4 h-4" />
        </button>
        <span
          onClick={handleResetZoom}
          className="text-[11px] font-mono font-semibold px-2 py-1 text-slate-300 hover:text-white bg-slate-800/80 rounded-md cursor-pointer select-none transition-colors"
          title="点击复位为 100%"
        >
          {Math.round(transform.scale * 100)}%
        </span>
        <button
          onClick={() => handleZoom(-0.15)}
          className="p-2 text-slate-300 hover:text-white hover:bg-slate-800 rounded-lg transition-colors"
          title="缩小"
        >
          <ZoomOut className="w-4 h-4" />
        </button>
        <button
          onClick={handleResetZoom}
          className="p-2 text-slate-300 hover:text-white hover:bg-slate-800 rounded-lg transition-colors"
          title="适应居中 / 复位视角"
        >
          <RotateCcw className="w-4 h-4" />
        </button>
        <div className="w-[1px] h-4 bg-slate-700 mx-0.5" />
        <button
          onClick={toggleFullscreen}
          className={clsx(
            'p-2 rounded-lg transition-all',
            isFullscreen
              ? 'text-sky-400 bg-sky-500/20 border border-sky-500/40 shadow-sm'
              : 'text-slate-300 hover:text-white hover:bg-slate-800',
          )}
          title={isFullscreen ? '退出全屏 (ESC)' : '全屏放大'}
        >
          {isFullscreen ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
        </button>
      </div>

      {/* 底部状态图例与提示 */}
      <div className="absolute bottom-4 left-4 z-40 flex items-center gap-3 px-3.5 py-2 rounded-xl bg-slate-900/90 backdrop-blur-md border border-slate-700/80 shadow-xl text-xs text-slate-300">
        <div className="flex items-center gap-1.5">
          <span className="w-2.5 h-2.5 rounded-full bg-emerald-400" />
          <span>正常运行</span>
        </div>
        <div className="flex items-center gap-1.5">
          <span className="w-2.5 h-2.5 rounded-full bg-red-500 animate-ping" />
          <span className="text-red-300 font-semibold">接口 DOWN / 告警</span>
        </div>
        <div className="flex items-center gap-1.5">
          <span className="w-2.5 h-2.5 rounded-full bg-slate-500" />
          <span>离线</span>
        </div>
        <span className="text-slate-500">|</span>
        <span className="text-slate-400">
          {isEditMode
            ? '💡 连线方法：点击任意设备的【连线】按钮，然后点击另一个目标设备即可建立连线！'
            : '💡 提示：点击任意设备节点可滑出接口实时流量与 UP/DOWN 状态抽屉'}
        </span>
      </div>
    </div>
  );
};
