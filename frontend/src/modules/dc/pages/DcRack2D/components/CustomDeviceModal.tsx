// frontend/src/modules/dc/pages/DcRack2D/components/CustomDeviceModal.tsx
import React, { useState, useEffect } from 'react';
import { X, Upload, Check, Image as ImageIcon } from 'lucide-react';
import type { CustomDeviceTemplate, DeviceCategory, DeviceHardwareType } from '../types';

interface CustomDeviceModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSave: (tpl: Omit<CustomDeviceTemplate, 'id'>) => void;
  editingTemplate: CustomDeviceTemplate | null;
}

export const CustomDeviceModal: React.FC<CustomDeviceModalProps> = ({
  isOpen,
  onClose,
  onSave,
  editingTemplate,
}) => {
  const [name, setName] = useState('');
  const [category, setCategory] = useState<DeviceCategory>('custom');
  const [hardwareType, setHardwareType] = useState<DeviceHardwareType>('switch');
  const [vendor, setVendor] = useState('自定义厂商');
  const [model, setModel] = useState('');
  const [uHeight, setUHeight] = useState<number>(1);
  const [panelColor, setPanelColor] = useState('#1e293b');
  const [description, setDescription] = useState('');
  const [frontImage, setFrontImage] = useState<string>('');

  useEffect(() => {
    if (editingTemplate) {
      setName(editingTemplate.name);
      setCategory(editingTemplate.category);
      setHardwareType(editingTemplate.hardwareType);
      setVendor(editingTemplate.vendor);
      setModel(editingTemplate.model);
      setUHeight(editingTemplate.uHeight);
      setPanelColor(editingTemplate.panelColor || '#1e293b');
      setDescription(editingTemplate.description || '');
      setFrontImage(editingTemplate.frontImage || '');
    } else {
      setName('');
      setCategory('custom');
      setHardwareType('switch');
      setVendor('自定义厂商');
      setModel('');
      setUHeight(1);
      setPanelColor('#1e293b');
      setDescription('');
      setFrontImage('');
    }
  }, [editingTemplate, isOpen]);

  if (!isOpen) return null;

  const handleImageUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      setFrontImage(reader.result as string);
    };
    reader.readAsDataURL(file);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;
    onSave({
      name: name.trim(),
      category,
      hardwareType,
      vendor: vendor.trim() || '自定义厂商',
      model: model.trim() || 'Custom-Model',
      uHeight: Math.max(1, uHeight),
      panelColor,
      description: description.trim(),
      frontImage,
    });
    onClose();
  };

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4">
      <div className="bg-surface-1 border border-border rounded-2xl p-6 w-full max-w-lg shadow-2xl relative overflow-hidden select-none">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 text-text-tertiary hover:text-text-primary p-1 rounded-lg hover:bg-surface-3 transition"
        >
          <X className="w-5 h-5" />
        </button>

        <h3 className="text-lg font-bold text-text-primary mb-5 flex items-center gap-2">
          <Upload className="w-5 h-5 text-primary" />
          <span>{editingTemplate ? '编辑设备模板' : '创建自定义设备模板'}</span>
        </h3>

        <form onSubmit={handleSubmit} className="space-y-4 text-xs">
          {/* 设备名称与所属分类 */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-text-secondary font-medium mb-1">设备名称 *</label>
              <input
                type="text"
                required
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder="如: 48口核心交换机"
                className="w-full bg-surface-2 border border-border rounded-lg px-3 py-2 text-text-primary focus:border-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-text-secondary font-medium mb-1">分类</label>
              <select
                value={category}
                onChange={e => setCategory(e.target.value as DeviceCategory)}
                className="w-full bg-surface-2 border border-border rounded-lg px-3 py-2 text-text-primary focus:border-primary focus:outline-none"
              >
                <option value="custom">自定义设备</option>
                <option value="odf">ODF组件</option>
                <option value="managed">纳管扩展设备</option>
              </select>
            </div>
          </div>

          {/* 硬件类型与占用 U 数 */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-text-secondary font-medium mb-1">硬件类型</label>
              <select
                value={hardwareType}
                onChange={e => setHardwareType(e.target.value as DeviceHardwareType)}
                className="w-full bg-surface-2 border border-border rounded-lg px-3 py-2 text-text-primary focus:border-primary focus:outline-none"
              >
                <option value="switch">网络交换机 (Switch)</option>
                <option value="router">路由器 (Router)</option>
                <option value="firewall">防火墙 (Firewall)</option>
                <option value="server">服务器 (Server)</option>
                <option value="patch_panel">配线架 (Patch Panel)</option>
                <option value="odf">ODF光纤配线箱</option>
                <option value="pdu">PDU电源条</option>
                <option value="other">其他硬件</option>
              </select>
            </div>
            <div>
              <div className="flex items-center justify-between mb-1">
                <label className="text-text-secondary font-medium">占用 U 数 (高度)</label>
                <div className="flex items-center gap-1">
                  {[1, 2, 3, 4, 8].map(u => (
                    <button
                      key={`tpl-u-${u}`}
                      type="button"
                      onClick={() => setUHeight(u)}
                      className={`px-1.5 py-0.5 text-[10px] font-mono rounded transition ${
                        uHeight === u
                          ? 'bg-primary text-white font-bold'
                          : 'bg-surface-2 text-text-tertiary hover:text-text-primary'
                      }`}
                    >
                      {u}U
                    </button>
                  ))}
                </div>
              </div>
              <input
                type="number"
                min={1}
                max={20}
                value={uHeight}
                onChange={e => setUHeight(Math.max(1, parseInt(e.target.value, 10) || 1))}
                className="w-full bg-surface-2 border border-border rounded-lg px-3 py-2 text-text-primary focus:border-primary focus:outline-none font-mono"
              />
            </div>
          </div>

          {/* 厂商与型号 */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-text-secondary font-medium mb-1">厂商品牌</label>
              <input
                type="text"
                value={vendor}
                onChange={e => setVendor(e.target.value)}
                placeholder="如: 华为 / 戴尔 / 康普"
                className="w-full bg-surface-2 border border-border rounded-lg px-3 py-2 text-text-primary focus:border-primary focus:outline-none"
              />
            </div>
            <div>
              <label className="block text-text-secondary font-medium mb-1">设备型号</label>
              <input
                type="text"
                value={model}
                onChange={e => setModel(e.target.value)}
                placeholder="如: CE6800-48T"
                className="w-full bg-surface-2 border border-border rounded-lg px-3 py-2 text-text-primary focus:border-primary focus:outline-none"
              />
            </div>
          </div>

          {/* 上传前面板图片 */}
          <div>
            <label className="block text-text-secondary font-medium mb-1">前面板图片 (可选)</label>
            <div className="flex items-center gap-3">
              <label className="px-3 py-2 bg-surface-2 border border-dashed border-primary cursor-pointer text-primary rounded-lg flex items-center gap-2 transition">
                <ImageIcon className="w-4 h-4" />
                <span>选择图片...</span>
                <input type="file" accept="image/*" onChange={handleImageUpload} className="hidden" />
              </label>
              {frontImage && (
                <div className="flex items-center gap-2">
                  <img src={frontImage} alt="Preview" className="w-16 h-7 object-cover rounded border border-border" />
                  <button
                    type="button"
                    onClick={() => setFrontImage('')}
                    className="text-status-failed hover:text-status-failed underline text-[11px]"
                  >
                    清除
                  </button>
                </div>
              )}
            </div>
          </div>

          {/* 描述 */}
          <div>
            <label className="block text-text-secondary font-medium mb-1">规格备注</label>
            <textarea
              rows={2}
              value={description}
              onChange={e => setDescription(e.target.value)}
              placeholder="如: 48口千兆电口，双冗余电源"
              className="w-full bg-surface-2 border border-border rounded-lg px-3 py-2 text-text-primary focus:border-primary focus:outline-none"
            />
          </div>

          {/* 底部按钮 */}
          <div className="flex items-center justify-end gap-3 pt-3 border-t border-border">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 bg-surface-2 hover:bg-surface-3 text-text-secondary rounded-lg transition"
            >
              取消
            </button>
            <button
              type="submit"
              className="px-5 py-2 bg-gradient-to-r from-primary-strong to-primary hover:opacity-90 text-white font-semibold rounded-lg transition shadow-[0_2px_8px_rgba(109,94,252,0.35)] flex items-center gap-1.5"
            >
              <Check className="w-4 h-4" />
              <span>保存模板</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
