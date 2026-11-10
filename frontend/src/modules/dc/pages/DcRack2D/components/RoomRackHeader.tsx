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
  selectedRoomId,
  onSelectRoom,
  onAddRoom,
  onAddRack,
  currentRoom,
}) => {
  return (
    <div className="flex w-full flex-col justify-between gap-4 border-b border-border pb-4 select-none md:flex-row md:items-center">
      {/* 左侧：标题与机房切换 */}
      <div className="flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-xl border border-primary/25 bg-primary-soft text-primary">
          <Server className="h-5 w-5" />
        </div>
        <div>
          <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight text-text-primary">
            <span>平面机架监控</span>
            {currentRoom && (
              <span className="rounded-md border border-primary/25 bg-primary-soft px-2 py-0.5 font-mono text-[11px] font-normal text-primary">
                {currentRoom.name}
              </span>
            )}
          </h2>
          <p className="font-mono text-[11px] text-text-tertiary">
            2D Flat Rack Monitor &amp; Elevation Management
          </p>
        </div>

        {rooms.length > 0 && (
          <div className="ml-3 hidden items-center gap-2 border-l border-border pl-3 sm:flex">
            <span className="flex items-center gap-1 text-xs text-text-secondary">
              <Building2 className="h-3.5 w-3.5 text-primary" />
              <span>切换机房:</span>
            </span>
            <select
              value={selectedRoomId}
              onChange={(e) => onSelectRoom(e.target.value)}
              className="cursor-pointer rounded-lg border border-border-strong bg-surface-2 px-2.5 py-1.5 text-xs text-text-primary transition hover:border-primary/40 focus:border-primary focus:outline-none"
            >
              {rooms.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.name} ({r.floor || '1F'})
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      {/* 右侧：环境指标与操作 */}
      <div className="flex items-center gap-3 self-end md:self-auto">
        {currentRoom && (
          <div className="hidden items-center gap-3 rounded-lg border border-border bg-surface-2 px-3 py-1.5 font-mono text-xs text-text-secondary shadow-sm lg:flex">
            <div className="flex items-center gap-1.5" title="机房当前平均温度">
              <Thermometer className="h-3.5 w-3.5 text-rose-400" />
              <span>{currentRoom.currentTemp || 22.5}°C</span>
            </div>
            <div className="h-3 w-px bg-border" />
            <div className="flex items-center gap-1.5" title="机房当前湿度">
              <Droplets className="h-3.5 w-3.5 text-sky-400" />
              <span>{currentRoom.currentHumidity || 48}%</span>
            </div>
            <div className="h-3 w-px bg-border" />
            <div className="flex items-center gap-1.5" title="能源效率 PUE">
              <Zap className="h-3.5 w-3.5 text-amber-400" />
              <span>PUE {currentRoom.pue || 1.32}</span>
            </div>
          </div>
        )}

        <button type="button" onClick={onAddRack} className="btn btn-ghost h-8 text-xs">
          <Server className="h-3.5 w-3.5 text-primary" />
          <span>新增机柜</span>
        </button>

        <button type="button" onClick={onAddRoom} className="btn btn-primary h-8 text-xs">
          <Plus className="h-3.5 w-3.5" />
          <span>新增机房</span>
        </button>
      </div>
    </div>
  );
};
