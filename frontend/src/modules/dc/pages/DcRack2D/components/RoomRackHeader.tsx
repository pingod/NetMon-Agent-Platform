// frontend/src/modules/dc/pages/DcRack2D/components/RoomRackHeader.tsx
import React from 'react';
import { Plus, Thermometer, Droplets, Zap, Building2, Server } from 'lucide-react';
import type { DcRoomInfo, DcRackInfo } from '../types';

interface RoomRackHeaderProps {
  rooms: DcRoomInfo[];
  racks: DcRackInfo[];
  selectedRoomId: string;
  selectedRackId: string;
  onSelectRoom: (roomId: string) => void;
  onSelectRack: (rackId: string) => void;
  onAddRoom: () => void;
  onAddRack: () => void;
  currentRoom: DcRoomInfo | null;
}

export const RoomRackHeader: React.FC<RoomRackHeaderProps> = ({
  rooms,
  racks,
  selectedRoomId,
  selectedRackId,
  onSelectRoom,
  onSelectRack,
  onAddRoom,
  onAddRack,
  currentRoom,
}) => {
  return (
    <div className="w-full flex flex-col md:flex-row md:items-center justify-between gap-4 pb-4 border-b border-slate-800/80 select-none">
      {/* 左侧：系统风格标题与机房下拉切换 */}
      <div className="flex items-center gap-3">
        <div className="w-9 h-9 rounded-xl bg-purple-500/10 border border-purple-500/20 flex items-center justify-center text-purple-400 shadow-inner">
          <Server className="w-5 h-5" />
        </div>
        <div>
          <h2 className="text-base font-bold text-white tracking-tight flex items-center gap-2">
            <span>平面机架监控</span>
            {currentRoom && (
              <span className="text-xs font-normal text-purple-400/90 font-mono px-2 py-0.5 rounded bg-purple-950/60 border border-purple-500/30">
                {currentRoom.name}
              </span>
            )}
          </h2>
          <p className="text-[11px] text-slate-400 font-mono">
            2D Flat Rack Monitor & Elevation Management
          </p>
        </div>

        {/* 机房下拉切换器 */}
        {rooms.length > 0 && (
          <div className="ml-3 hidden sm:flex items-center gap-2 pl-3 border-l border-slate-800">
            <span className="text-xs text-slate-400 flex items-center gap-1">
              <Building2 className="w-3.5 h-3.5 text-purple-400" />
              <span>切换机房:</span>
            </span>
            <select
              value={selectedRoomId}
              onChange={e => onSelectRoom(e.target.value)}
              className="bg-slate-900 border border-slate-700 text-slate-200 text-xs rounded-lg px-2.5 py-1.5 focus:border-purple-500 focus:outline-none cursor-pointer hover:border-slate-600 transition"
            >
              {rooms.map(r => (
                <option key={r.id} value={r.id}>
                  {r.name} ({r.floor || '1F'})
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      {/* 右侧：机房实时环境指标与操作按钮 */}
      <div className="flex items-center gap-3 self-end md:self-auto">
        {/* 机房环境状态指标 */}
        {currentRoom && (
          <div className="hidden lg:flex items-center gap-3 px-3 py-1.5 bg-slate-900/90 border border-slate-800 rounded-lg text-xs font-mono text-slate-300 shadow-inner">
            <div className="flex items-center gap-1.5" title="机房当前平均温度">
              <Thermometer className="w-3.5 h-3.5 text-purple-400" />
              <span>{currentRoom.currentTemp || 22.5}°C</span>
            </div>
            <div className="w-px h-3 bg-slate-700" />
            <div className="flex items-center gap-1.5" title="机房当前湿度">
              <Droplets className="w-3.5 h-3.5 text-blue-400" />
              <span>{currentRoom.currentHumidity || 48}%</span>
            </div>
            <div className="w-px h-3 bg-slate-700" />
            <div className="flex items-center gap-1.5" title="能源效率 PUE">
              <Zap className="w-3.5 h-3.5 text-amber-400" />
              <span>PUE {currentRoom.pue || 1.32}</span>
            </div>
          </div>
        )}

        {/* 新增机柜按钮 */}
        <button
          onClick={onAddRack}
          className="px-3.5 py-1.5 rounded-lg text-xs font-medium text-slate-300 bg-slate-900 border border-slate-700 hover:border-purple-500/60 hover:text-purple-300 hover:bg-slate-800 transition-all flex items-center gap-1.5 shadow-sm"
        >
          <Server className="w-3.5 h-3.5 text-purple-400" />
          <span>新增机柜</span>
        </button>

        {/* 新增机房按钮 */}
        <button
          onClick={onAddRoom}
          className="px-4 py-1.5 rounded-lg text-xs font-semibold text-white bg-gradient-to-r from-purple-600 to-purple-700 hover:from-purple-500 hover:to-purple-600 shadow-md shadow-purple-500/25 transition-all flex items-center gap-1.5"
        >
          <Plus className="w-3.5 h-3.5" />
          <span>新增机房</span>
        </button>
      </div>
    </div>
  );
};
