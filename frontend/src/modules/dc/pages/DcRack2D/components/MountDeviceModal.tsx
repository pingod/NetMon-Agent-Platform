// frontend/src/modules/dc/pages/DcRack2D/components/MountDeviceModal.tsx
import React, { useState, useEffect } from 'react';
import { X, Server, Check, AlertTriangle, Layers, HardDrive } from 'lucide-react';
import type { CustomDeviceTemplate } from '../types';

interface MountDeviceModalProps {
  isOpen: boolean;
  onClose: () => void;
  targetStartU: number;
  maxU?: number;
  templates: CustomDeviceTemplate[];
  unallocatedDevices: Array<{ id: string; name: string; device_type: string }>;
  onMount: (params: {
    rack_id: string;
    device_id?: string;
    device_name: string;
    device_type: string;
    start_u: number;
    end_u: number;
    position_face: string;
    notes?: string;
  }) => void;
  rackId: string;
  rackName: string;
  initialSourceType?: 'template' | 'managed' | 'custom';
  initialDeviceId?: string;
  initialUHeight?: number;
  mountedSlots?: Array<{ startU: number; endU: number; deviceName?: string }>;
}

export const MountDeviceModal: React.FC<MountDeviceModalProps> = ({
  isOpen,
  onClose,
  targetStartU,
  maxU = 42,
  templates,
  unallocatedDevices,
  onMount,
  rackId,
  rackName,
  initialSourceType = 'template',
  initialDeviceId = '',
  initialUHeight = 1,
  mountedSlots = [],
}) => {
  const [sourceType, setSourceType] = useState<'template' | 'managed' | 'custom'>(initialSourceType);
  const [selectedTemplateId, setSelectedTemplateId] = useState<string>('');
  const [selectedManagedId, setSelectedManagedId] = useState<string>('');
  const [customDeviceName, setCustomDeviceName] = useState<string>('');
  const [startU, setStartU] = useState<number>(targetStartU);
  const [uHeight, setUHeight] = useState<number>(initialUHeight);
  const [positionFace, setPositionFace] = useState<string>('front');
  const [notes, setNotes] = useState<string>('');

  useEffect(() => {
    if (!isOpen) return;

    setStartU(targetStartU);
    setSourceType(initialSourceType);

    if (initialUHeight && initialUHeight > 0) {
      setUHeight(initialUHeight);
    } else {
      setUHeight(1);
    }

    if (initialDeviceId) {
      if (initialSourceType === 'managed') {
        setSelectedManagedId(initialDeviceId);
      } else {
        setSelectedTemplateId(initialDeviceId);
      }
    } else {
      if (templates.length > 0 && !selectedTemplateId) {
        setSelectedTemplateId(templates[0].id);
        if (!initialUHeight) {
          setUHeight(templates[0].uHeight || 1);
        }
      }
      if (unallocatedDevices.length > 0 && !selectedManagedId) {
        setSelectedManagedId(unallocatedDevices[0].id);
      }
    }
  }, [targetStartU, isOpen, initialSourceType, initialDeviceId, initialUHeight]);

  const handleTemplateChange = (id: string) => {
    setSelectedTemplateId(id);
    const tpl = templates.find(t => t.id === id);
    if (tpl) {
      setUHeight(tpl.uHeight);
    }
  };

  const handleManagedChange = (id: string) => {
    setSelectedManagedId(id);
    const dev = unallocatedDevices.find(d => d.id === id);
    if (dev && dev.device_type === 'server' && uHeight === 1) {
      setUHeight(2);
    }
  };

  if (!isOpen) return null;

  // 计算当前结束 U 位
  const endU = Math.min(maxU, startU + uHeight - 1);

  // 检查是否与已上架设备重叠
  const conflictDevice = mountedSlots.find(
    s => !(endU < s.startU || startU > s.endU)
  );

  const handleStartUChange = (val: number) => {
    const s = Math.max(1, Math.min(maxU, val));
    setStartU(s);
  };

  const handleUHeightChange = (height: number) => {
    const h = Math.max(1, Math.min(maxU - startU + 1, height));
    setUHeight(h);
  };

  const handleEndUChange = (val: number) => {
    const e = Math.max(startU, Math.min(maxU, val));
    setUHeight(e - startU + 1);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    if (sourceType === 'template') {
      const tpl = templates.find(t => t.id === selectedTemplateId);
      if (!tpl) return;
      onMount({
        rack_id: rackId,
        device_id: tpl.id,
        device_name: tpl.name,
        device_type: tpl.hardwareType === 'server' ? 'server' : 'network_device',
        start_u: startU,
        end_u: endU,
        position_face: positionFace,
        notes: notes.trim() || tpl.description,
      });
    } else if (sourceType === 'managed') {
      const dev = unallocatedDevices.find(d => d.id === selectedManagedId);
      if (!dev) return;
      onMount({
        rack_id: rackId,
        device_id: dev.id,
        device_name: dev.name,
        device_type: dev.device_type,
        start_u: startU,
        end_u: endU,
        position_face: positionFace,
        notes: notes.trim(),
      });
    } else {
      if (!customDeviceName.trim()) return;
      onMount({
        rack_id: rackId,
        device_name: customDeviceName.trim(),
        device_type: 'network_device',
        start_u: startU,
        end_u: endU,
        position_face: positionFace,
        notes: notes.trim(),
      });
    }
    onClose();
  };

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-50 p-4">
      <div className="bg-slate-900 border border-slate-700 rounded-2xl p-6 w-full max-w-lg shadow-2xl relative select-none">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
        >
          <X className="w-5 h-5" />
        </button>

        <h3 className="text-lg font-bold text-white mb-4 flex items-center gap-2">
          <Server className="w-5 h-5 text-purple-400" />
          <span>上架设备至机柜 {rackName} (U{startU})</span>
        </h3>

        {/* 来源切换 */}
        <div className="flex p-1 bg-slate-950 rounded-lg border border-slate-800 mb-4 text-xs font-medium">
          <button
            type="button"
            onClick={() => setSourceType('template')}
            className={`flex-1 py-1.5 rounded-md transition ${
              sourceType === 'template'
                ? 'bg-gradient-to-r from-purple-600 to-purple-700 text-white font-bold shadow-md shadow-purple-500/25'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            从设备库模板
          </button>
          <button
            type="button"
            onClick={() => setSourceType('managed')}
            className={`flex-1 py-1.5 rounded-md transition ${
              sourceType === 'managed'
                ? 'bg-gradient-to-r from-purple-600 to-purple-700 text-white font-bold shadow-md shadow-purple-500/25'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            已纳管资产
          </button>
          <button
            type="button"
            onClick={() => setSourceType('custom')}
            className={`flex-1 py-1.5 rounded-md transition ${
              sourceType === 'custom'
                ? 'bg-gradient-to-r from-purple-600 to-purple-700 text-white font-bold shadow-md shadow-purple-500/25'
                : 'text-slate-400 hover:text-white'
            }`}
          >
            直接输入
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          {sourceType === 'template' && (
            <div>
              <label className="block text-slate-300 font-medium mb-1">选择设备模板</label>
              <select
                value={selectedTemplateId}
                onChange={e => handleTemplateChange(e.target.value)}
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-purple-500 focus:outline-none"
              >
                {templates.map(t => (
                  <option key={t.id} value={t.id}>
                    {t.name} (预设 {t.uHeight}U · {t.vendor})
                  </option>
                ))}
              </select>
            </div>
          )}

          {sourceType === 'managed' && (
            <div>
              <label className="block text-slate-300 font-medium mb-1">选择未上架纳管资产</label>
              {unallocatedDevices.length === 0 ? (
                <div className="p-3 bg-slate-950 rounded border border-slate-800 text-slate-500 text-center">
                  暂无未上架的服务器或网络设备
                </div>
              ) : (
                <select
                  value={selectedManagedId}
                  onChange={e => handleManagedChange(e.target.value)}
                  className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-purple-500 focus:outline-none"
                >
                  <option value="">-- 请选择设备 --</option>
                  {unallocatedDevices.map(d => (
                    <option key={d.id} value={d.id}>
                      {d.name} ({d.device_type === 'server' ? '服务器' : '网络设备'})
                    </option>
                  ))}
                </select>
              )}
            </div>
          )}

          {sourceType === 'custom' && (
            <div>
              <label className="block text-slate-300 font-medium mb-1">设备名称 *</label>
              <input
                type="text"
                required
                value={customDeviceName}
                onChange={e => setCustomDeviceName(e.target.value)}
                placeholder="如: 临时测试交换机 / 防火墙"
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-purple-500 focus:outline-none"
              />
            </div>
          )}

          {/* 重点：U 高度快捷选择器与区间设定 */}
          <div className="p-3.5 bg-slate-950/80 border border-slate-800 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-slate-200 font-semibold flex items-center gap-1.5">
                <Layers className="w-3.5 h-3.5 text-purple-400" />
                <span>设备占用高度 (U数选择)</span>
              </span>
              <span className="text-[11px] font-mono text-purple-400 font-bold bg-purple-950/60 border border-purple-800/60 px-2 py-0.5 rounded-full">
                当前: {uHeight} U
              </span>
            </div>

            {/* 快捷 U 数药丸按钮 */}
            <div className="flex items-center gap-1.5 flex-wrap">
              {[1, 2, 3, 4, 5, 6, 8].map(u => {
                const isActive = uHeight === u;
                return (
                  <button
                    key={`btn-u-${u}`}
                    type="button"
                    onClick={() => handleUHeightChange(u)}
                    className={`px-3 py-1.5 rounded-lg font-mono text-xs font-bold transition-all flex items-center gap-1 ${
                      isActive
                        ? 'bg-purple-600 text-white shadow-lg shadow-purple-500/30 scale-105 border border-purple-400'
                        : 'bg-slate-900 text-slate-400 border border-slate-800 hover:text-slate-200 hover:bg-slate-800'
                    }`}
                  >
                    <span>{u}U</span>
                    {isActive && <Check className="w-3 h-3" />}
                  </button>
                );
              })}
            </div>

            {/* 起始 U 位、占用高度微调与终止 U 位 */}
            <div className="grid grid-cols-3 gap-2.5 pt-1">
              <div>
                <label className="block text-[11px] text-slate-400 mb-1">起始 U 位</label>
                <input
                  type="number"
                  min={1}
                  max={maxU}
                  value={startU}
                  onChange={e => handleStartUChange(parseInt(e.target.value, 10) || 1)}
                  className="w-full bg-slate-900 border border-slate-700 rounded-lg px-2.5 py-1.5 text-slate-100 font-mono text-xs focus:border-purple-500 focus:outline-none"
                />
              </div>

              <div>
                <label className="block text-[11px] text-slate-400 mb-1">占用高度 (U)</label>
                <div className="flex items-center">
                  <button
                    type="button"
                    onClick={() => handleUHeightChange(uHeight - 1)}
                    className="px-2 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-l-lg border border-r-0 border-slate-700"
                  >
                    -
                  </button>
                  <input
                    type="number"
                    min={1}
                    max={maxU - startU + 1}
                    value={uHeight}
                    onChange={e => handleUHeightChange(parseInt(e.target.value, 10) || 1)}
                    className="w-full bg-slate-900 border border-slate-700 text-center py-1.5 text-slate-100 font-mono text-xs focus:border-purple-500 focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={() => handleUHeightChange(uHeight + 1)}
                    className="px-2 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-r-lg border border-l-0 border-slate-700"
                  >
                    +
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-[11px] text-slate-400 mb-1">结束 U 位</label>
                <input
                  type="number"
                  min={startU}
                  max={maxU}
                  value={endU}
                  onChange={e => handleEndUChange(parseInt(e.target.value, 10) || startU)}
                  className="w-full bg-slate-900 border border-slate-700 rounded-lg px-2.5 py-1.5 text-purple-300 font-mono font-bold text-xs focus:border-purple-500 focus:outline-none"
                />
              </div>
            </div>

            {/* 机架占用区间横幅 */}
            <div className="px-3 py-2 bg-purple-950/40 border border-purple-800/40 rounded-lg flex items-center justify-between">
              <span className="text-slate-400 text-[11px]">将在机柜占据机位：</span>
              <span className="font-mono text-xs font-bold text-purple-300">
                U{startU} ~ U{endU} <span className="text-[10px] text-purple-400 font-normal">（共 {uHeight} 个U位）</span>
              </span>
            </div>

            {/* 冲突提示 */}
            {conflictDevice && (
              <div className="p-2 bg-amber-950/60 border border-amber-800/80 rounded-lg text-amber-300 text-[11px] flex items-center gap-1.5">
                <AlertTriangle className="w-3.5 h-3.5 flex-shrink-0 text-amber-400" />
                <span>
                  提示: U{conflictDevice.startU}~U{conflictDevice.endU} 已存在「{conflictDevice.deviceName || '已上架设备'}」，上架将占用该区间。
                </span>
              </div>
            )}
          </div>

          {/* 朝向与备注 */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-slate-300 font-medium mb-1">安装朝向</label>
              <select
                value={positionFace}
                onChange={e => setPositionFace(e.target.value)}
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-purple-500 focus:outline-none"
              >
                <option value="front">正面安装 (Front)</option>
                <option value="rear">背面安装 (Rear)</option>
              </select>
            </div>
            <div>
              <label className="block text-slate-300 font-medium mb-1">备注信息</label>
              <input
                type="text"
                value={notes}
                onChange={e => setNotes(e.target.value)}
                placeholder="可选备注说明"
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-purple-500 focus:outline-none"
              />
            </div>
          </div>

          <div className="flex items-center justify-end gap-3 pt-3 border-t border-slate-800">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg transition"
            >
              取消
            </button>
            <button
              type="submit"
              className="px-5 py-2 bg-gradient-to-r from-purple-600 to-purple-700 hover:from-purple-500 hover:to-purple-600 text-white font-semibold rounded-lg transition shadow-md shadow-purple-500/25 flex items-center gap-1.5"
            >
              <Check className="w-4 h-4" />
              <span>确认上架 ({uHeight}U)</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
