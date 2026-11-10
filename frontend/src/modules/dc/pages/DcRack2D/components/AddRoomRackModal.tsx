// frontend/src/modules/dc/pages/DcRack2D/components/AddRoomRackModal.tsx
import React, { useState, useEffect } from 'react';
import { X, Building2, Server, Check } from 'lucide-react';
import api from '@/lib/api';
import { message } from 'antd';
import { useQueryClient } from '@tanstack/react-query';

interface AddRoomModalProps {
  isOpen: boolean;
  onClose: () => void;
  mode: 'room' | 'rack';
  currentRoomId?: string;
  rooms?: Array<{ id: string; name: string; floor?: string }>;
}

export const AddRoomRackModal: React.FC<AddRoomModalProps> = ({
  isOpen,
  onClose,
  mode,
  currentRoomId,
  rooms = [],
}) => {
  const queryClient = useQueryClient();
  const [name, setName] = useState('');
  const [floor, setFloor] = useState('1F');
  const [heightU, setHeightU] = useState(42);
  const [targetRoomId, setTargetRoomId] = useState(currentRoomId || (rooms.length > 0 ? rooms[0].id : ''));
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (currentRoomId) {
      setTargetRoomId(currentRoomId);
    } else if (rooms.length > 0) {
      setTargetRoomId(rooms[0].id);
    }
  }, [currentRoomId, rooms]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    setIsSubmitting(true);
    try {
      if (mode === 'room') {
        await api.post('/dc/rooms', {
          name: name.trim(),
          floor: floor.trim() || '1F',
          area_sqm: 100,
        });
        message.success(`机房「${name}」创建成功！`);
      } else {
        const finalRoomId = targetRoomId || currentRoomId || (rooms.length > 0 ? rooms[0].id : '');
        await api.post('/dc/racks', {
          name: name.trim(),
          room_id: finalRoomId,
          total_u: heightU,
          height_u: heightU,
          status: 'online',
        });
        message.success(`机柜「${name}」创建成功！`);
      }
      queryClient.invalidateQueries({ queryKey: ['dc'] });
      setName('');
      onClose();
    } catch (err: any) {
      message.error(`创建失败: ${err.response?.data?.error || err.message}`);
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/80 flex items-center justify-center z-50 p-4">
      <div className="bg-slate-900 border border-cyan-800/80 rounded-2xl p-6 w-full max-w-sm shadow-2xl relative select-none">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
        >
          <X className="w-5 h-5" />
        </button>

        <h3 className="text-lg font-bold text-white mb-4 flex items-center gap-2">
          {mode === 'room' ? (
            <>
              <Building2 className="w-5 h-5 text-cyan-400" />
              <span>新增机房</span>
            </>
          ) : (
            <>
              <Server className="w-5 h-5 text-cyan-400" />
              <span>新增机柜</span>
            </>
          )}
        </h3>

        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          {mode === 'rack' && rooms.length > 0 && (
            <div>
              <label className="block text-slate-300 font-medium mb-1">所属机房 *</label>
              <select
                value={targetRoomId}
                onChange={e => setTargetRoomId(e.target.value)}
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-cyan-400 focus:outline-none"
              >
                {rooms.map(r => (
                  <option key={r.id} value={r.id}>
                    {r.name} ({r.floor || '1F'})
                  </option>
                ))}
              </select>
            </div>
          )}

          <div>
            <label className="block text-slate-300 font-medium mb-1">
              {mode === 'room' ? '机房名称 *' : '机柜名称/编号 *'}
            </label>
            <input
              type="text"
              required
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder={mode === 'room' ? '如: test数据中心' : '如: R4 / A03'}
              className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-cyan-400 focus:outline-none"
            />
          </div>

          {mode === 'room' && (
            <div>
              <label className="block text-slate-300 font-medium mb-1">所属楼层</label>
              <input
                type="text"
                value={floor}
                onChange={e => setFloor(e.target.value)}
                placeholder="如: 3F"
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-cyan-400 focus:outline-none"
              />
            </div>
          )}

          {mode === 'rack' && (
            <div>
              <label className="block text-slate-300 font-medium mb-1">机柜高度规格 (U数)</label>
              <input
                type="number"
                min={12}
                max={54}
                value={heightU}
                onChange={e => setHeightU(parseInt(e.target.value, 10) || 42)}
                className="w-full bg-slate-950 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 focus:border-cyan-400 focus:outline-none font-mono"
              />
            </div>
          )}

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
              disabled={isSubmitting}
              className="px-5 py-2 bg-gradient-to-r from-purple-600 to-purple-700 hover:from-purple-500 hover:to-purple-600 disabled:opacity-50 text-white font-semibold rounded-lg transition shadow-md shadow-purple-500/25 flex items-center gap-1.5"
            >
              <Check className="w-4 h-4" />
              <span>{isSubmitting ? '保存中...' : '确认保存'}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
