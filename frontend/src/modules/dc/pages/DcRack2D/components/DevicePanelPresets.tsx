// frontend/src/modules/dc/pages/DcRack2D/components/DevicePanelPresets.tsx
import React from 'react';
import type { DeviceHardwareType } from '../types';

interface DevicePanelProps {
  name: string;
  hardwareType?: DeviceHardwareType;
  uHeight: number;
  status?: 'online' | 'warning' | 'critical' | 'offline' | 'unknown';
  frontImage?: string;
  panelColor?: string;
  vendor?: string;
  model?: string;
  ipAddress?: string;
  heightPx: number; // 实际插槽渲染高度
}

export const DevicePanelPresets: React.FC<DevicePanelProps> = ({
  name,
  hardwareType = 'other',
  uHeight,
  status = 'online',
  frontImage,
  panelColor,
  vendor,
  model,
  heightPx,
}) => {
  // 状态指示灯颜色
  const isOnline = status === 'online';
  const isWarn = status === 'warning';
  const isCrit = status === 'critical';
  const ledColor = isCrit ? '#ef4444' : isWarn ? '#eab308' : isOnline ? '#22c55e' : '#64748b';

  // 如果上传了自定义图片
  if (frontImage) {
    return (
      <div className="relative w-full h-full flex items-center justify-between px-3 overflow-hidden rounded-[2px] bg-slate-900 border border-border select-none shadow-inner">
        <img
          src={frontImage}
          alt={name}
          className="absolute inset-0 w-full h-full object-cover opacity-90"
        />
        <div className="absolute inset-0 bg-gradient-to-r from-black/80 via-transparent to-black/80 pointer-events-none" />
        <div className="relative z-10 flex items-center gap-2">
          <span
            className="w-2 h-2 rounded-full flex-shrink-0 animate-pulse shadow-[0_0_8px_currentColor]"
            style={{ backgroundColor: ledColor, color: ledColor }}
          />
          <span className="text-xs font-semibold text-white truncate max-w-[180px] drop-shadow-md">
            {name}
          </span>
        </div>
        <div className="relative z-10 text-[10px] text-purple-600 font-mono bg-slate-100/90 px-1.5 py-0.5 rounded border border-purple-500/30">
          {uHeight}U {model || vendor || ''}
        </div>
      </div>
    );
  }

  // 1. 交换机 (Switch) 前面板
  if (hardwareType === 'switch' || name.includes('交换机')) {
    return (
      <div className="relative w-full h-full flex items-center justify-between px-3 bg-gradient-to-b from-[#eef2f9] via-[#e6edf7] to-[#dde6f2] border border-border rounded-[2px] shadow-[inset_0_1px_2px_rgba(255,255,255,0.08),inset_0_-1px_2px_rgba(0,0,0,0.8)] overflow-hidden select-none">
        {/* 金属边框 */}
        <div className="absolute inset-y-0 left-0 w-1 bg-gradient-to-b from-slate-300 to-slate-400 border-r border-border" />
        <div className="absolute inset-y-0 right-0 w-1 bg-gradient-to-b from-slate-300 to-slate-400 border-l border-border" />

        {/* 左侧：LED指示灯、LOGO与名称 */}
        <div className="flex items-center gap-2 z-10">
          <div className="flex flex-col items-center gap-0.5">
            <span
              className="w-2 h-2 rounded-full animate-pulse shadow-[0_0_6px_currentColor]"
              style={{ backgroundColor: ledColor, color: ledColor }}
              title={`状态: ${status}`}
            />
            <span className="text-[7px] text-slate-500 uppercase font-mono tracking-tighter">PWR</span>
          </div>
          <div className="flex flex-col">
            <span className="text-xs font-bold text-slate-800 tracking-wide truncate max-w-[140px]">
              {name}
            </span>
            <span className="text-[9px] text-purple-600 font-mono tracking-wider">
              {vendor ? `${vendor.toUpperCase()} ` : ''}{model || 'GIGA-SWITCH 48P'}
            </span>
          </div>
        </div>

        {/* 中间：48口网口网格阵列 */}
        <div className="hidden sm:flex items-center gap-1.5 px-3 py-1 bg-slate-100/90 rounded border border-border shadow-inner">
          <div className="grid grid-cols-12 gap-0.5">
            {Array.from({ length: 24 }).map((_, idx) => (
              <div
                key={`p-top-${idx}`}
                className="w-2.5 h-2 bg-slate-950 border border-border rounded-[1px] flex items-center justify-center relative group"
              >
                <div
                  className={`w-1 h-0.5 rounded-[0.5px] ${idx % 3 === 0 ? 'bg-emerald-400 shadow-[0_0_3px_#10b981]' : idx % 5 === 0 ? 'bg-amber-400' : 'bg-slate-700'}`}
                />
              </div>
            ))}
            {Array.from({ length: 24 }).map((_, idx) => (
              <div
                key={`p-bot-${idx}`}
                className="w-2.5 h-2 bg-slate-950 border border-border rounded-[1px] flex items-center justify-center relative group"
              >
                <div
                  className={`w-1 h-0.5 rounded-[0.5px] ${idx % 4 === 0 ? 'bg-emerald-400 shadow-[0_0_3px_#10b981]' : 'bg-slate-700'}`}
                />
              </div>
            ))}
          </div>

          {/* 4 个 10G 光口 SFP+ 模块槽 */}
          <div className="flex items-center gap-0.5 pl-2 border-l border-border">
            {Array.from({ length: 4 }).map((_, idx) => (
              <div
                key={`sfp-${idx}`}
                className="w-3 h-4 bg-slate-900 border border-purple-300 rounded-[1px] flex flex-col items-center justify-between p-0.5"
              >
                <span className="w-1 h-1 rounded-full bg-purple-400 shadow-[0_0_4px_#a855f7]" />
                <span className="text-[6px] text-slate-500 font-mono">{idx + 1}</span>
              </div>
            ))}
          </div>
        </div>

        {/* 右侧：散热格栅与占用 U 标签 */}
        <div className="flex items-center gap-2 z-10">
          <div className="hidden md:flex flex-col gap-0.5 opacity-40">
            <div className="w-6 h-0.5 bg-slate-400 rounded-full" />
            <div className="w-6 h-0.5 bg-slate-400 rounded-full" />
            <div className="w-6 h-0.5 bg-slate-400 rounded-full" />
          </div>
          <span className="text-[10px] font-mono px-1.5 py-0.5 bg-purple-100 border border-purple-300 text-purple-600 rounded shadow-sm">
            {uHeight}U
          </span>
        </div>
      </div>
    );
  }

  // 2. 防火墙 (Firewall) 前面板 — 暗红 / 专属安全造型
  if (hardwareType === 'firewall' || name.includes('防火墙')) {
    return (
      <div className="relative w-full h-full flex items-center justify-between px-3 bg-gradient-to-b from-[#fdecea] via-[#fbe0dd] to-[#f6c9c5] border border-red-300 rounded-[2px] shadow-[inset_0_1px_2px_rgba(255,100,100,0.15),inset_0_-1px_2px_rgba(0,0,0,0.9)] overflow-hidden select-none">
        {/* 左侧红色警示条 */}
        <div className="absolute inset-y-0 left-0 w-1.5 bg-gradient-to-b from-red-500 to-red-800 shadow-[0_0_8px_rgba(239,68,68,0.5)]" />

        <div className="flex items-center gap-2.5 z-10 pl-1">
          <div className="flex flex-col items-center gap-0.5">
            <span
              className="w-2 h-2 rounded-full animate-pulse shadow-[0_0_8px_#ef4444]"
              style={{ backgroundColor: ledColor }}
              title={`状态: ${status}`}
            />
            <span className="text-[7px] text-red-600 font-mono tracking-tighter">SEC</span>
          </div>
          <div className="flex flex-col">
            <span className="text-xs font-bold text-red-900 tracking-wide truncate max-w-[140px]">
              {name}
            </span>
            <span className="text-[9px] text-red-600 font-mono tracking-wider">
              {vendor ? `${vendor.toUpperCase()} ` : ''}{model || 'NEXT-GEN FIREWALL'}
            </span>
          </div>
        </div>

        {/* 中间网络接口 */}
        <div className="hidden sm:flex items-center gap-2 px-3 py-1 bg-slate-100/90 rounded border border-red-200 shadow-inner">
          <div className="flex items-center gap-1">
            {Array.from({ length: 8 }).map((_, idx) => (
              <div
                key={`fw-eth-${idx}`}
                className="w-3.5 h-3 bg-neutral-950 border border-red-200 rounded-[1px] flex flex-col items-center justify-between p-0.5"
              >
                <span className="w-1 h-0.5 bg-emerald-400 shadow-[0_0_2px_#10b981]" />
                <span className="text-[6px] text-red-600 font-mono">G{idx}</span>
              </div>
            ))}
          </div>
          <div className="w-px h-4 bg-red-900/50 mx-1" />
          <div className="flex items-center gap-1">
            <div className="w-3.5 h-3 bg-neutral-950 border border-red-700/60 rounded-[1px] flex flex-col items-center justify-between p-0.5">
              <span className="w-1 h-0.5 bg-amber-400 shadow-[0_0_2px_#f59e0b]" />
              <span className="text-[6px] text-amber-300/80 font-mono">MGT</span>
            </div>
            <div className="w-3.5 h-3 bg-neutral-950 border border-border rounded-[1px] flex flex-col items-center justify-between p-0.5">
              <span className="w-1 h-0.5 bg-purple-400" />
              <span className="text-[6px] text-slate-500 font-mono">CON</span>
            </div>
          </div>
        </div>

        {/* 右侧标签 */}
        <div className="flex items-center gap-2 z-10">
          <span className="text-[10px] font-mono px-1.5 py-0.5 bg-red-100 border border-red-300 text-red-600 rounded shadow-sm">
            {uHeight}U
          </span>
        </div>
      </div>
    );
  }

  // 3. 配线架 (Patch Panel) 前面板
  if (hardwareType === 'patch_panel' || name.includes('配线架')) {
    return (
      <div className="relative w-full h-full flex items-center justify-between px-3 bg-gradient-to-b from-[#eef2f9] via-[#e6edf7] to-[#dde6f2] border border-border rounded-[2px] shadow-inner select-none">
        {/* 左侧信息 */}
        <div className="flex items-center gap-2 z-10">
          <span
            className="w-1.5 h-1.5 rounded-full"
            style={{ backgroundColor: ledColor }}
          />
          <div className="flex flex-col">
            <span className="text-xs font-bold text-slate-700 truncate max-w-[130px]">
              {name}
            </span>
            <span className="text-[8px] text-slate-500 font-mono">
              CAT6A SHIELDED PATCH PANEL
            </span>
          </div>
        </div>

        {/* 中间高密端口排布条 */}
        <div className="flex-1 max-w-[340px] mx-4 py-0.5 px-2 bg-slate-100/90 rounded border border-border flex flex-col gap-0.5">
          <div className="flex justify-between items-center px-1 text-[7px] text-slate-500 font-mono">
            <span>1-12</span>
            <span>13-24</span>
          </div>
          <div className="flex items-center justify-between gap-1">
            {Array.from({ length: 24 }).map((_, idx) => (
              <div
                key={`jack-${idx}`}
                className="w-2.5 h-2.5 bg-slate-900 border border-border rounded-[1px] flex items-center justify-center"
              >
                <div className="w-1.5 h-1 bg-slate-950 border-t border-amber-400" />
              </div>
            ))}
          </div>
        </div>

        <span className="text-[10px] font-mono px-1.5 py-0.5 bg-purple-100 text-slate-600 rounded border border-slate-300">
          {uHeight}U
        </span>
      </div>
    );
  }

  // 4. 服务器 (Server) 前面板
  if (hardwareType === 'server' || name.includes('服务器') || uHeight >= 2) {
    return (
      <div
        className="relative w-full h-full flex items-center justify-between px-3 rounded-[2px] shadow-inner overflow-hidden select-none border border-border"
        style={{
          background: panelColor
            ? `linear-gradient(to bottom, ${panelColor}40, #dde6f2)`
            : 'linear-gradient(to bottom, #eef2f9, #e6edf7 60%, #dde6f2)',
        }}
      >
        {/* 左侧面板信息 */}
        <div className="flex items-center gap-2.5 z-10">
          <div className="flex flex-col items-center gap-1">
            <span
              className="w-2 h-2 rounded-full animate-pulse shadow-[0_0_6px_currentColor]"
              style={{ backgroundColor: ledColor, color: ledColor }}
              title={`状态: ${status}`}
            />
            <div className="w-2.5 h-2.5 rounded-full border border-slate-500 bg-slate-900 flex items-center justify-center shadow-inner">
              <span className="text-[6px] text-purple-600 font-bold">⏻</span>
            </div>
          </div>

          <div className="flex flex-col">
            <span className="text-xs font-bold text-slate-800 tracking-wide truncate max-w-[140px]">
              {name}
            </span>
            <span className="text-[9px] text-purple-600 font-mono">
              {vendor ? `${vendor.toUpperCase()} ` : ''}{model || `${uHeight}U RACK SERVER`}
            </span>
          </div>
        </div>

        {/* 中间硬盘盒抽取架阵列 (SAS/NVMe Drive Trays) */}
        <div className="hidden sm:flex items-center gap-1 px-2.5 py-1 bg-slate-100/90 rounded border border-border shadow-inner">
          {Array.from({ length: uHeight >= 4 ? 12 : uHeight >= 2 ? 8 : 4 }).map((_, idx) => (
            <div
              key={`drive-${idx}`}
              className="w-6 h-5 bg-gradient-to-b from-slate-200 to-slate-300 border border-border rounded-[1px] flex flex-col justify-between p-0.5 relative group shadow-sm"
            >
              <div className="flex items-center justify-between">
                <span className="w-1 h-0.5 bg-emerald-400 shadow-[0_0_2px_#10b981]" />
                <span className="text-[5px] text-slate-500 font-mono">{idx + 1}</span>
              </div>
              <div className="w-full h-1 bg-slate-300 rounded-[0.5px] border-t border-border" />
            </div>
          ))}
        </div>

        {/* 右侧风扇格栅与规格 */}
        <div className="flex items-center gap-2 z-10">
          <div className="hidden md:flex flex-col gap-0.5 opacity-50">
            <div className="w-4 h-0.5 bg-slate-400 rounded-full" />
            <div className="w-4 h-0.5 bg-slate-400 rounded-full" />
            <div className="w-4 h-0.5 bg-slate-400 rounded-full" />
          </div>
          <span className="text-[10px] font-mono px-1.5 py-0.5 bg-purple-100 text-purple-600 rounded border border-purple-300">
            {uHeight}U
          </span>
        </div>
      </div>
    );
  }

  // 5. 路由器 / ODF / 其他通用硬件前面板
  return (
    <div className="relative w-full h-full flex items-center justify-between px-3 bg-gradient-to-b from-[#eef2f9] via-[#e6edf7] to-[#dde6f2] border border-border rounded-[2px] shadow-inner select-none">
      <div className="flex items-center gap-2 z-10">
        <span
          className="w-2 h-2 rounded-full shadow-[0_0_6px_currentColor]"
          style={{ backgroundColor: ledColor }}
          title={`状态: ${status}`}
        />
        <div className="flex flex-col">
          <span className="text-xs font-bold text-slate-800 tracking-wide truncate max-w-[140px]">
            {name}
          </span>
          <span className="text-[9px] text-slate-500 font-mono">
            {vendor || 'GENERIC'} {model || 'NETWORK DEVICE'}
          </span>
        </div>
      </div>

      <div className="hidden sm:flex items-center gap-2 px-2 py-0.5 bg-slate-100/90 rounded border border-border">
        <div className="flex gap-1">
          <div className="w-3 h-3 bg-slate-900 border border-border rounded-[1px] flex items-center justify-center">
            <span className="w-1 h-0.5 bg-emerald-400" />
          </div>
          <div className="w-3 h-3 bg-slate-900 border border-border rounded-[1px] flex items-center justify-center">
            <span className="w-1 h-0.5 bg-emerald-400" />
          </div>
        </div>
      </div>

      <span className="text-[10px] font-mono px-1.5 py-0.5 bg-purple-100 text-slate-600 rounded border border-slate-300">
        {uHeight}U
      </span>
    </div>
  );
};
