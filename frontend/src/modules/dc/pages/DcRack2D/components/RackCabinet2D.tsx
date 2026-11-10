// frontend/src/modules/dc/pages/DcRack2D/components/RackCabinet2D.tsx
import React, { useState, useMemo } from 'react';
import {
  ZoomIn,
  ZoomOut,
  RotateCcw,
  Server,
} from 'lucide-react';
import type {
  DcRackInfo,
  MountedSlotDevice,
  CustomDeviceTemplate,
} from '../types';
import { RackSlotUnit } from './RackSlotUnit';

interface RackCabinet2DProps {
  rack: DcRackInfo | null;
  mountedSlots: MountedSlotDevice[];
  selectedDevice: MountedSlotDevice | null;
  onSelectDevice: (dev: MountedSlotDevice) => void;
  onSlotClick: (uNumber: number) => void;
  onMountDrop: (uNumber: number, template: CustomDeviceTemplate) => void;
  zoomScale: number;
  onZoomChange: (scale: number) => void;
  totalU?: number;
  rackList: DcRackInfo[];
  selectedRackId: string;
  onSelectRack: (id: string) => void;
}

export const RackCabinet2D: React.FC<RackCabinet2DProps> = ({
  rack,
  mountedSlots,
  selectedDevice,
  onSelectDevice,
  onSlotClick,
  onMountDrop,
  zoomScale,
  onZoomChange,
  totalU = 42,
  rackList,
  selectedRackId,
  onSelectRack,
}) => {
  const [dragOverU, setDragOverU] = useState<number | null>(null);

  // 计算每个 U 的占用映射 (Map from U -> MountedSlotDevice)
  const slotMap = useMemo(() => {
    const map = new Map<number, MountedSlotDevice>();
    mountedSlots.forEach(slot => {
      for (let u = slot.startU; u <= slot.endU; u++) {
        map.set(u, slot);
      }
    });
    return map;
  }, [mountedSlots]);

  // 已用 U 数
  const usedU = useMemo(() => {
    return mountedSlots.reduce((acc, curr) => acc + (curr.endU - curr.startU + 1), 0);
  }, [mountedSlots]);

  // 1U 像素基准高度
  const baseSlotHeightPx = 18;
  const scaledSlotHeightPx = Math.round(baseSlotHeightPx * (zoomScale / 100));

  // 42 到 1 的 U 序列（机架从上往下看，42U在顶端，1U在底端）
  const uNumbersDesc = useMemo(() => {
    const list: number[] = [];
    for (let u = totalU; u >= 1; u--) {
      list.push(u);
    }
    return list;
  }, [totalU]);

  // 拖拽处理
  const handleDragOver = (e: React.DragEvent, u: number) => {
    e.preventDefault();
    setDragOverU(u);
  };

  const handleDragLeave = () => {
    setDragOverU(null);
  };

  const handleDrop = (e: React.DragEvent, u: number) => {
    e.preventDefault();
    setDragOverU(null);
    try {
      const dataStr = e.dataTransfer.getData('application/json');
      if (dataStr) {
        const template: CustomDeviceTemplate = JSON.parse(dataStr);
        onMountDrop(u, template);
      }
    } catch (err) {
      console.error('Failed to parse drag drop data', err);
    }
  };

  if (!rack) {
    return (
      <div className="flex-1 flex items-center justify-center p-12 bg-surface-2 border border-border rounded-2xl text-text-secondary">
        请在顶部选择或创建一个机房与机柜
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col items-center select-none overflow-x-auto min-w-[380px] p-2">
      {/* 顶部机柜切换与缩放工具条 */}
      <div className="w-full max-w-[560px] flex items-center justify-between gap-3 mb-4">
        {/* 机柜快速选择药丸 */}
        <div className="flex items-center gap-1.5 overflow-x-auto py-1">
          {rackList.map(r => {
            const isCur = r.id === selectedRackId;
            return (
              <button
                key={r.id}
                onClick={() => onSelectRack(r.id)}
                className={`px-3 py-1 text-xs font-semibold rounded-lg transition-all flex items-center gap-1.5 ${
                  isCur
                    ? 'bg-primary-soft text-primary border border-[rgba(90,74,232,0.4)] shadow-[0_0_10px_rgba(109,94,252,0.18)]'
                    : 'bg-surface-2 text-text-secondary border border-border hover:text-text-primary hover:bg-surface-3'
                }`}
              >
                <Server className="w-3 h-3" />
                <span>{r.name}</span>
              </button>
            );
          })}
        </div>

        {/* 缩放控制器 */}
        <div className="flex items-center gap-1 rounded-lg border border-border bg-surface-2 px-2 py-1 text-xs text-text-secondary shadow-sm">
          <button
            onClick={() => onZoomChange(Math.max(70, zoomScale - 10))}
            className="rounded p-1 text-text-tertiary transition-colors hover:bg-surface-3 hover:text-primary"
            title="缩小"
          >
            <ZoomOut className="w-3.5 h-3.5" />
          </button>
          <span className="px-1 font-mono text-[11px] text-primary">{zoomScale}%</span>
          <button
            onClick={() => onZoomChange(Math.min(140, zoomScale + 10))}
            className="rounded p-1 text-text-tertiary transition-colors hover:bg-surface-3 hover:text-primary"
            title="放大"
          >
            <ZoomIn className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => onZoomChange(100)}
            className="ml-0.5 rounded p-1 text-text-tertiary transition-colors hover:bg-surface-3 hover:text-primary"
            title="重置 100%"
          >
            <RotateCcw className="w-3 h-3" />
          </button>
        </div>
      </div>

      {/* 机柜立面主容器（适配系统浅色金属风格） */}
      <div
        className="w-full max-w-[540px] bg-gradient-to-b from-[#f5f8fd] via-[#eef2f9] to-[#e5ebf5] border-2 border-slate-300/90 rounded-3xl p-5 shadow-[0_4px_24px_rgba(15,23,42,0.14),0_0_0_1px_var(--border)] flex flex-col relative"
        style={{
          transform: `scale(${zoomScale / 100})`,
          transformOrigin: 'top center',
          transition: 'transform 0.15s ease-out',
        }}
      >
        {/* 机柜头部状态栏（R1 · [在线] · [37U / 42U]） */}
        <div className="flex items-center justify-between pb-3.5 mb-3 border-b border-border px-1">
          <div className="flex items-center gap-2">
            <h2 className="text-2xl font-black tracking-wider text-text-primary font-mono">
              {rack.name}
            </h2>
          </div>

          <div className="flex items-center gap-2.5">
            {/* 在线状态徽章（绿色胶囊） */}
            <span className="px-3 py-1 bg-emerald-50 text-emerald-700 border border-emerald-300 rounded-full text-xs font-bold flex items-center gap-1.5">
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
              在线
            </span>

            {/* 容量占用徽章（系统紫色胶囊） */}
            <span className="px-3 py-1 bg-[color:var(--primary-soft)] text-primary border border-[rgba(90,74,232,0.4)] rounded-full text-xs font-mono font-bold">
              {usedU}U / {totalU}U
            </span>
          </div>
        </div>

        {/* 占用进度条 + 空机柜引导 */}
        <div className="px-1 mb-3">
          <div className="h-1.5 w-full rounded-full bg-slate-200 overflow-hidden">
            <div
              className="h-full bg-gradient-to-r from-[#6d5efc] via-[#8b5cf6] to-cyan-400 rounded-full transition-[width] duration-500"
              style={{ width: `${Math.max(0, Math.min(100, (usedU / totalU) * 100))}%` }}
            />
          </div>
        </div>

        {usedU === 0 && (
          <div className="mb-3 px-1 flex items-center justify-center gap-2 rounded-xl border border-dashed border-border bg-surface-2 py-2.5 text-xs text-text-secondary">
            <Server className="w-3.5 h-3.5 text-primary" />
            <span>
              机柜为空 · 点击任意 U 位或从左侧「设备库」拖拽设备上架
            </span>
          </div>
        )}

        {/* 机柜立柱外框架与 U 位槽位 */}
        <div className="relative w-full bg-surface-1 border-2 border-slate-300 rounded-xl overflow-hidden shadow-[inset_0_2px_6px_rgba(15,23,42,0.06)] flex">
          {/* 左侧 U 标尺立柱 */}
          <div className="w-9 bg-gradient-to-r from-[#eef2f9] via-[#e8eef7] to-[#e2e9f3] border-r border-border flex flex-col justify-between select-none py-1 flex-shrink-0 z-20 shadow-md">
            {uNumbersDesc.map(u => (
              <div
                key={`left-u-${u}`}
                style={{ height: `${scaledSlotHeightPx}px` }}
                className="flex items-center justify-center border-b border-border"
              >
                <span className="text-[10.5px] font-mono font-bold text-text-secondary tracking-tight">
                  {u}
                </span>
              </div>
            ))}
          </div>

          {/* 中间 U 槽位主网格区 */}
          <div className="flex-1 flex flex-col relative bg-surface-2 overflow-hidden">
            {uNumbersDesc.map(u => {
              const mounted = slotMap.get(u);
              const isSelected = Boolean(mounted && selectedDevice && mounted.id === selectedDevice.id);

              return (
                <RackSlotUnit
                  key={`slot-${u}`}
                  uNumber={u}
                  slotHeightPx={scaledSlotHeightPx}
                  mountedDevice={mounted}
                  isSelected={isSelected}
                  onSelectDevice={onSelectDevice}
                  onSlotClick={onSlotClick}
                  isDragOver={dragOverU === u}
                  onDragOver={handleDragOver}
                  onDragLeave={handleDragLeave}
                  onDrop={handleDrop}
                />
              );
            })}
          </div>

          {/* 右侧 U 标尺立柱 */}
          <div className="w-9 bg-gradient-to-l from-[#eef2f9] via-[#e8eef7] to-[#e2e9f3] border-l border-border flex flex-col justify-between select-none py-1 flex-shrink-0 z-20 shadow-md">
            {uNumbersDesc.map(u => (
              <div
                key={`right-u-${u}`}
                style={{ height: `${scaledSlotHeightPx}px` }}
                className="flex items-center justify-center border-b border-border"
              >
                <span className="text-[10.5px] font-mono font-bold text-text-secondary tracking-tight">
                  {u}
                </span>
              </div>
            ))}
          </div>
        </div>

        {/* 底部标准机柜说明 */}
        <div className="mt-3 flex items-center justify-between text-[10px] text-text-tertiary font-mono px-2">
          <span>42U EIA-310-D STANDARD RACK</span>
          <span>AIRFLOW: FRONT TO REAR</span>
        </div>
      </div>
    </div>
  );
};
