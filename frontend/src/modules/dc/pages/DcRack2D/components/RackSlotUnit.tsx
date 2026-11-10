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
  const zebra = uNumber % 2 === 0 ? 'bg-[#f2f6fb]' : 'bg-[#e9eff7]';
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
      className={`w-full relative border-b border-border transition-all flex items-center justify-between px-3 cursor-pointer group ${
        isDragOver
          ? 'bg-[rgba(109,94,252,0.25)] border-primary shadow-[inset_0_0_14px_rgba(109,94,252,0.45)]'
          : `${zebra} hover:bg-[rgba(109,94,252,0.10)]`
      }`}
    >
      {/* 槽位顶部高光线，营造金属反光 */}
      <div className="absolute inset-x-0 top-0 h-px bg-gradient-to-r from-transparent via-slate-300/70 to-transparent pointer-events-none" />

      {/* 左右两侧螺丝孔示意 */}
      <div className="w-1.5 h-1.5 rounded-full bg-slate-400 border border-slate-300 flex-shrink-0" />

      {/* 鼠标悬浮才显示的上架提示 */}
      <span
        className={`text-[10px] font-mono text-text-tertiary transition-opacity flex items-center gap-1 select-none ${
          isDragOver ? 'opacity-100 text-primary' : 'opacity-0 group-hover:opacity-100 group-hover:text-primary'
        }`}
      >
        <svg className="w-3 h-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.4"
          strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M12 5v14M5 12h14" />
        </svg>
        上架 U{uNumber}
      </span>

      <div className="w-1.5 h-1.5 rounded-full bg-slate-400 border border-slate-300 flex-shrink-0" />
    </div>
  );
};
