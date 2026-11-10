// frontend/src/modules/dc/pages/DcRack2D/components/RackSlotUnit.tsx
import React from 'react';
import type { MountedSlotDevice } from '../types';
import { DevicePanelPresets } from './DevicePanelPresets';

interface RackSlotUnitProps {
  uNumber: number;
  slotHeightPx: number;
  mountedDevice?: MountedSlotDevice;
  isSelected?: boolean;
  onSelectDevice?: (dev: MountedSlotDevice) => void;
  onSlotClick?: (uNumber: number) => void;
  isDragOver?: boolean;
  onDragOver?: (e: React.DragEvent, uNumber: number) => void;
  onDragLeave?: (e: React.DragEvent) => void;
  onDrop?: (e: React.DragEvent, uNumber: number) => void;
}

export const RackSlotUnit: React.FC<RackSlotUnitProps> = ({
  uNumber,
  slotHeightPx,
  mountedDevice,
  isSelected,
  onSelectDevice,
  onSlotClick,
  isDragOver,
  onDragOver,
  onDragLeave,
  onDrop,
}) => {
  // 如果当前 U 是某个跨 U 设备的非起始位，跳过渲染本体
  const isCoveredByLowerDevice = mountedDevice && mountedDevice.startU !== uNumber;

  if (isCoveredByLowerDevice) {
    return null;
  }

  // 1. 该位置有已上架设备
  if (mountedDevice) {
    const totalHeight = mountedDevice.uHeight * slotHeightPx;

    return (
      <div
        style={{ height: `${totalHeight}px` }}
        onClick={e => {
          e.stopPropagation();
          onSelectDevice && onSelectDevice(mountedDevice);
        }}
        className={`w-full relative transition-all cursor-pointer rounded-[2px] z-10 ${
          isSelected
            ? 'ring-2 ring-purple-500 shadow-[0_0_15px_rgba(139,92,246,0.6)] scale-[1.005]'
            : 'hover:ring-1 hover:ring-purple-500/60'
        }`}
      >
        <DevicePanelPresets
          name={mountedDevice.deviceName}
          hardwareType={mountedDevice.hardwareType}
          uHeight={mountedDevice.uHeight}
          status={mountedDevice.status}
          vendor={mountedDevice.vendor}
          model={mountedDevice.model}
          ipAddress={mountedDevice.ipAddress}
          frontImage={mountedDevice.frontImage}
          heightPx={totalHeight}
        />
      </div>
    );
  }

  // 2. 空闲槽位 (Empty 1U Slot)
  return (
    <div
      style={{ height: `${slotHeightPx}px` }}
      onClick={() => onSlotClick && onSlotClick(uNumber)}
      onDragOver={e => {
        e.preventDefault();
        onDragOver && onDragOver(e, uNumber);
      }}
      onDragLeave={onDragLeave}
      onDrop={e => onDrop && onDrop(e, uNumber)}
      className={`w-full relative border-b border-slate-900 transition-all flex items-center justify-between px-3 cursor-pointer group ${
        isDragOver
          ? 'bg-purple-500/20 border-purple-400 shadow-[inset_0_0_12px_rgba(139,92,246,0.4)]'
          : 'bg-[#090d16] hover:bg-slate-800/60'
      }`}
    >
      {/* 槽位深色百叶窗横纹 */}
      <div className="absolute inset-x-0 inset-y-[3px] bg-gradient-to-b from-transparent via-slate-900/40 to-transparent pointer-events-none opacity-50" />

      {/* 左右两侧螺丝孔示意 */}
      <div className="w-1.5 h-1.5 rounded-full bg-slate-950 border border-slate-800 flex-shrink-0" />

      {/* 鼠标悬浮提示 */}
      <span className="text-[9px] font-mono text-slate-500 group-hover:text-purple-300 transition-colors">
        + 上架 (U{uNumber})
      </span>

      <div className="w-1.5 h-1.5 rounded-full bg-slate-950 border border-slate-800 flex-shrink-0" />
    </div>
  );
};
