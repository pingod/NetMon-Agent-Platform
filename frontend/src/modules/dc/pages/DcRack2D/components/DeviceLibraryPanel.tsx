// frontend/src/modules/dc/pages/DcRack2D/components/DeviceLibraryPanel.tsx
import React from 'react';
import {
  Search,
  ChevronLeft,
  ChevronRight,
  Upload,
  Trash2,
  Edit2,
  GripVertical,
  Layers,
  HardDrive,
} from 'lucide-react';
import type {
  DeviceCategory,
  CustomDeviceTemplate,
  MountedSlotDevice,
} from '../types';

interface DeviceLibraryPanelProps {
  isCollapsed: boolean;
  onToggleCollapse: () => void;
  activeCategory: DeviceCategory;
  onSelectCategory: (cat: DeviceCategory) => void;
  searchQuery: string;
  onSearchChange: (q: string) => void;
  selectedSlotDevice: MountedSlotDevice | null;
  onUnmountDevice: (slotId: string) => void;
  templates: CustomDeviceTemplate[];
  unallocatedDevices: Array<{ id: string; name: string; device_type: string }>;
  onMountDeviceToNextFree: (device: { name: string; uHeight: number; hardwareType?: any; deviceId?: string; deviceType?: string }) => void;
  onOpenMountModalForDevice?: (device: { id: string; name: string; device_type: string; uHeight: number }) => void;
  onOpenCreateTemplateModal: () => void;
  onEditTemplate: (tpl: CustomDeviceTemplate) => void;
  onDeleteTemplate: (id: string) => void;
  onDragStartTemplate?: (e: React.DragEvent, tpl: CustomDeviceTemplate) => void;
}

export const DeviceLibraryPanel: React.FC<DeviceLibraryPanelProps> = ({
  isCollapsed,
  onToggleCollapse,
  activeCategory,
  onSelectCategory,
  searchQuery,
  onSearchChange,
  selectedSlotDevice,
  onUnmountDevice,
  templates,
  unallocatedDevices,
  onMountDeviceToNextFree,
  onOpenMountModalForDevice,
  onOpenCreateTemplateModal,
  onEditTemplate,
  onDeleteTemplate,
  onDragStartTemplate,
}) => {
  const [deviceUHeights, setDeviceUHeights] = React.useState<Record<string, number>>({});
  const getDevU = (dev: { id: string; device_type: string }) =>
    deviceUHeights[dev.id] || (dev.device_type === 'server' ? 2 : 1);

  if (isCollapsed) {
    return (
      <div className="w-10 bg-surface-2 border-r border-border flex flex-col items-center py-4 transition-all select-none">
        <button
          onClick={onToggleCollapse}
          className="p-1.5 rounded-lg bg-surface-3 text-text-secondary hover:text-primary hover:bg-surface-3 transition"
          title="展开设备库"
        >
          <ChevronRight className="w-4 h-4" />
        </button>
        <span className="writing-vertical text-xs font-medium text-text-tertiary mt-6 tracking-widest uppercase">
          设备库
        </span>
      </div>
    );
  }

  return (
    <div className="w-80 md:w-[350px] bg-surface-1 backdrop-blur-md border border-border rounded-xl p-4 flex flex-col gap-4 shadow-xl select-none flex-shrink-0">
      {/* 1. 面板头部：标题与折叠按钮 */}
      <div className="flex items-center justify-between pb-1">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-bold text-text-primary tracking-wide">设备库</h3>
        </div>
        <button
          onClick={onToggleCollapse}
          className="p-1 rounded text-text-tertiary hover:text-primary hover:bg-surface-3 transition"
          title="折叠设备库"
        >
          <ChevronLeft className="w-4 h-4" />
        </button>
      </div>

      {/* 2. 三标签页切换：已纳管设备 / 自定义设备 / ODF组件 */}
      <div className="flex items-center gap-1.5 p-1 bg-surface-2 rounded-lg border border-border">
        <button
          onClick={() => onSelectCategory('managed')}
          className={`flex-1 py-1.5 text-xs font-medium rounded-md transition-all text-center ${
            activeCategory === 'managed'
              ? 'bg-gradient-to-r from-primary-strong to-primary text-white font-bold shadow-md shadow-[0_2px_8px_rgba(109,94,252,0.35)] text-white'
              : 'text-text-tertiary hover:text-text-primary'
          }`}
        >
          已纳管设备
        </button>
        <button
          onClick={() => onSelectCategory('custom')}
          className={`flex-1 py-1.5 text-xs font-medium rounded-md transition-all text-center ${
            activeCategory === 'custom'
              ? 'bg-gradient-to-r from-primary-strong to-primary text-white font-bold shadow-md shadow-[0_2px_8px_rgba(109,94,252,0.35)] text-white'
              : 'text-text-tertiary hover:text-text-primary'
          }`}
        >
          自定义设备
        </button>
        <button
          onClick={() => onSelectCategory('odf')}
          className={`flex-1 py-1.5 text-xs font-medium rounded-md transition-all text-center ${
            activeCategory === 'odf'
              ? 'bg-gradient-to-r from-primary-strong to-primary text-white font-bold shadow-md shadow-[0_2px_8px_rgba(109,94,252,0.35)] text-white'
              : 'text-text-tertiary hover:text-text-primary'
          }`}
        >
          ODF组件
        </button>
      </div>

      {/* 3. 「当前设备」激活卡片 */}
      {selectedSlotDevice ? (
        <div className="p-3.5 bg-gradient-to-b from-primary-soft via-surface-2 to-surface-1 border border-[rgba(90,74,232,0.4)] rounded-xl flex flex-col gap-2.5 shadow-lg relative overflow-hidden group">
          <div className="absolute top-0 left-0 w-1 h-full bg-primary" />
          <div className="flex justify-between items-start">
            <span className="text-[11px] text-text-tertiary font-medium tracking-wide">当前设备</span>
            <span className="text-[10px] font-mono px-1.5 py-0.5 bg-emerald-100 text-emerald-800 border border-emerald-300 rounded">
              {selectedSlotDevice.status === 'online' ? '运行正常' : '已就位'}
            </span>
          </div>
          <div>
            <h4 className="text-sm font-bold text-text-primary truncate drop-shadow-sm">
              {selectedSlotDevice.deviceName}
            </h4>
            <p className="text-xs text-primary font-mono mt-0.5">
              U{selectedSlotDevice.startU}
              {selectedSlotDevice.endU !== selectedSlotDevice.startU && ` ~ U${selectedSlotDevice.endU}`} · 占用 {selectedSlotDevice.uHeight}U
            </p>
          </div>

          {/* 下架设备按钮 */}
          <button
            onClick={() => onUnmountDevice(selectedSlotDevice.id)}
            className="w-full py-2 px-3 bg-surface-2 border border-border hover:border-status-failed text-text-secondary hover:text-status-failed hover:bg-[rgba(225,29,72,0.08)] rounded-lg text-xs font-semibold flex items-center justify-center gap-2 transition-all shadow-sm group-hover:border-[rgba(90,74,232,0.6)]"
          >
            <Trash2 className="w-3.5 h-3.5" />
            <span>下架设备</span>
          </button>
        </div>
      ) : (
        <div className="p-3 bg-surface-2 border border-border rounded-xl text-center text-xs text-text-tertiary py-4">
          点击机架上的设备可在此查看详情与下架
        </div>
      )}

      {/* 4. 搜索框 */}
      <div className="relative">
        <input
          type="text"
          value={searchQuery}
          onChange={e => onSearchChange(e.target.value)}
          placeholder="搜索名称 / 厂商 / 型号"
          className="w-full bg-surface-2 border border-border focus:border-primary text-text-primary text-xs rounded-lg pl-3 pr-8 py-2 placeholder:text-text-tertiary focus:outline-none transition-all"
        />
        <Search className="w-3.5 h-3.5 text-text-tertiary absolute right-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
      </div>

      {/* 5. 设备卡片列表 */}
      <div className="flex-1 overflow-y-auto space-y-2.5 pr-1 max-h-[360px] custom-rack-scrollbar">
        {/* A. 已纳管设备类别 */}
        {activeCategory === 'managed' && (
          <>
            {unallocatedDevices.length === 0 ? (
              <div className="text-center py-8 text-xs text-text-tertiary">
                暂无待上架的已纳管设备
              </div>
            ) : (
              unallocatedDevices.map(dev => {
                const curU = getDevU(dev);
                return (
                  <div
                    key={dev.id}
                    draggable
                    onDragStart={e => {
                      const tpl: any = {
                        id: dev.id,
                        name: dev.name,
                        category: 'managed',
                        hardwareType: dev.device_type === 'server' ? 'server' : 'switch',
                        vendor: '已纳管资产',
                        model: dev.name,
                        uHeight: curU,
                        description: dev.name,
                        panelColor: '#1e293b',
                      };
                      if (onDragStartTemplate) {
                        onDragStartTemplate(e, tpl);
                      } else {
                        e.dataTransfer.setData('application/json', JSON.stringify(tpl));
                      }
                    }}
                    className="p-2.5 bg-surface-2 border border-border hover:border-primary hover:shadow-[0_0_0_1px_var(--primary-ring)] rounded-xl flex items-center justify-between gap-2 transition-all group hover:bg-surface-3 cursor-grab active:cursor-grabbing"
                  >
                    <div className="flex items-center gap-2 overflow-hidden flex-1">
                      <div className="w-8 h-8 rounded-lg bg-surface-3 border border-border flex items-center justify-center text-primary flex-shrink-0">
                        {dev.device_type === 'server' ? <HardDrive className="w-4 h-4" /> : <Layers className="w-4 h-4" />}
                      </div>
                      <div className="overflow-hidden flex-1">
                        <h5 className="text-xs font-semibold text-text-primary truncate group-hover:text-primary">
                          {dev.name}
                        </h5>
                        <div className="flex items-center gap-1.5 mt-0.5">
                          <span className="text-[10px] text-text-tertiary font-mono">
                            {dev.device_type === 'server' ? '服务器' : '网络设备'}
                          </span>
                          {/* 占用 U 数选择器 */}
                          <select
                            value={curU}
                            onClick={e => e.stopPropagation()}
                            onChange={e => {
                              e.stopPropagation();
                              const val = parseInt(e.target.value, 10) || 1;
                              setDeviceUHeights(prev => ({ ...prev, [dev.id]: val }));
                            }}
                            className="bg-primary-soft border border-primary text-primary font-mono font-bold text-[10px] px-1 py-0.5 rounded cursor-pointer focus:outline-none"
                            title="修改设备占用的 U 数"
                          >
                            <option value={1}>1U</option>
                            <option value={2}>2U</option>
                            <option value={3}>3U</option>
                            <option value={4}>4U</option>
                            <option value={6}>6U</option>
                            <option value={8}>8U</option>
                          </select>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-1">
                      <button
                        onClick={() => {
                          if (onOpenMountModalForDevice) {
                            onOpenMountModalForDevice({
                              id: dev.id,
                              name: dev.name,
                              device_type: dev.device_type,
                              uHeight: curU,
                            });
                          } else {
                            onMountDeviceToNextFree({
                              name: dev.name,
                              uHeight: curU,
                              deviceId: dev.id,
                              deviceType: dev.device_type,
                            });
                          }
                        }}
                        className="px-2.5 py-1 text-[11px] font-medium rounded-md border border-primary bg-primary-soft text-primary hover:bg-primary hover:text-white transition whitespace-nowrap shadow-[0_0_0_1px_var(--primary-ring)] flex items-center gap-1"
                        title="上架到机柜（可自选U位或快捷上架）"
                      >
                        <span>上架</span>
                        <span className="font-mono text-[10px] opacity-70">({curU}U)</span>
                      </button>
                    </div>
                  </div>
                );
              })
            )}
          </>
        )}

        {/* B. 自定义设备 / ODF 组件模板列表 */}
        {activeCategory !== 'managed' && (
          <>
            {templates.length === 0 ? (
              <div className="text-center py-8 text-xs text-text-tertiary">
                暂无匹配的设备模板
              </div>
            ) : (
              templates.map(tpl => {
                const isFirewall = tpl.hardwareType === 'firewall' || tpl.name.includes('防火墙');
                const isPatch = tpl.hardwareType === 'patch_panel' || tpl.name.includes('配线架');

                return (
                  <div
                    key={tpl.id}
                    draggable
                    onDragStart={e => onDragStartTemplate && onDragStartTemplate(e, tpl)}
                    className="p-2.5 bg-surface-2 border border-border hover:border-primary hover:shadow-[0_0_0_1px_var(--primary-ring)] rounded-xl flex items-center justify-between gap-2.5 transition-all group hover:bg-surface-3 cursor-grab active:cursor-grabbing shadow-sm"
                  >
                    {/* 左侧：拖拽把手 + 设备缩略图 */}
                    <div className="flex items-center gap-2 overflow-hidden flex-1">
                      <GripVertical className="w-3.5 h-3.5 text-text-tertiary group-hover:text-primary flex-shrink-0" />

                      {/* 设备缩略图：统一深色面板边框，图片内缩显示 */}
                      <div className="w-12 h-6 rounded-md bg-surface border border-border flex items-center justify-center relative overflow-hidden flex-shrink-0 shadow-inner">
                        {tpl.frontImage ? (
                          <img
                            src={tpl.frontImage}
                            alt={tpl.name}
                            className="w-full h-full object-contain p-0.5"
                          />
                        ) : (
                          <div className={`flex items-center gap-0.5 ${isFirewall ? 'text-red-600' : isPatch ? 'text-text-secondary' : 'text-purple-600'}`}>
                            <span className="w-1 h-1 rounded-full bg-current" />
                            <span className="w-3 h-0.5 bg-current opacity-60" />
                          </div>
                        )}
                      </div>

                      {/* 设备名称与标签 */}
                      <div className="overflow-hidden">
                        <h5 className="text-xs font-bold text-text-primary truncate group-hover:text-primary">
                          {tpl.name}
                        </h5>
                        <span className="text-[10px] text-text-tertiary font-mono">
                          {tpl.category === 'odf' ? 'ODF组件' : '自定义'}
                        </span>
                      </div>
                    </div>

                    {/* 右侧：占用U控制区、编辑与删除 */}
                    <div className="flex items-center gap-1.5 flex-shrink-0">
                      <div className="flex flex-col items-end">
                        <span className="text-[9px] text-text-tertiary font-mono scale-90 -mr-1">占用U</span>
                        <div className="px-2 py-0.5 bg-surface-3 border border-border rounded text-xs font-mono font-bold text-text-secondary">
                          {tpl.uHeight}
                        </div>
                      </div>

                      <div className="flex flex-col gap-1 pl-1">
                        <button
                          onClick={() => onEditTemplate(tpl)}
                          className="p-1 rounded text-text-tertiary hover:text-primary hover:bg-surface-3 transition"
                          title="编辑模板"
                        >
                          <Edit2 className="w-3 h-3" />
                        </button>
                        {!tpl.isSystemPreset && (
                          <button
                            onClick={() => onDeleteTemplate(tpl.id)}
                            className="p-1 rounded text-text-tertiary hover:text-status-failed hover:bg-surface-3 transition"
                            title="删除模板"
                          >
                            <Trash2 className="w-3 h-3" />
                          </button>
                        )}
                      </div>

                      {/* 快速上架按钮 */}
                      <button
                        onClick={() =>
                          onMountDeviceToNextFree({
                            name: tpl.name,
                            uHeight: tpl.uHeight,
                            hardwareType: tpl.hardwareType,
                          })
                        }
                        className="px-2 py-1 text-[11px] font-medium rounded-md border border-primary bg-primary-soft text-primary hover:bg-primary hover:text-white transition shadow-[0_0_0_1px_var(--primary-ring)]"
                        title="快速上架到当前机柜"
                      >
                        上架
                      </button>
                    </div>
                  </div>
                );
              })
            )}
          </>
        )}
      </div>

      {/* 6. 底部虚线卡片：「上传设备图片」/「创建自定义厂商设备模板」 */}
      <div
        onClick={onOpenCreateTemplateModal}
        className="mt-auto p-4 border-2 border-dashed border-border hover:border-primary hover:shadow-[0_0_0_1px_var(--primary-ring)] rounded-xl bg-[color:var(--primary-soft)] hover:bg-[color:var(--primary-soft)] flex flex-col items-center justify-center gap-1.5 cursor-pointer transition-all group"
      >
        <div className="w-7 h-7 rounded-full bg-primary-soft border border-primary flex items-center justify-center text-primary group-hover:scale-110 group-hover:shadow-[0_0_10px_rgba(109,94,252,0.4)] transition-all">
          <Upload className="w-3.5 h-3.5" />
        </div>
        <span className="text-xs font-bold text-primary group-hover:text-primary">
          上传设备图片
        </span>
        <span className="text-[10px] text-text-tertiary text-center">
          创建自定义厂商设备模板
        </span>
      </div>
    </div>
  );
};
