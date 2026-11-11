// frontend/src/modules/dc/pages/DcRack2D/index.tsx
import React, { useState } from 'react';
import { Modal } from 'antd';
import { useRack2DData } from './useRack2DData';
import { RoomRackHeader } from './components/RoomRackHeader';
import { DeviceLibraryPanel } from './components/DeviceLibraryPanel';
import { RackCabinet2D } from './components/RackCabinet2D';
import { CustomDeviceModal } from './components/CustomDeviceModal';
import { MountDeviceModal } from './components/MountDeviceModal';
import { AddRoomRackModal } from './components/AddRoomRackModal';
import type { CustomDeviceTemplate, MountedSlotDevice } from './types';

export default function DcRack2DMonitor() {
  const {
    rooms,
    racks,
    selectedRoomId,
    setSelectedRoomId,
    selectedRackId,
    setSelectedRackId,
    currentRoom,
    currentRack,
    rackStats,
    mountedSlots,
    selectedSlotDevice,
    setSelectedSlotDevice,
    activeCategory,
    setActiveCategory,
    searchQuery,
    setSearchQuery,
    zoomScale,
    setZoomScale,
    filteredTemplates,
    unallocatedDevices,
    mountMutation,
    unmountMutation,
    handleCreateTemplate,
    handleUpdateTemplate,
    handleDeleteTemplate,
  } = useRack2DData();

  // 侧边栏折叠
  const [isLibraryCollapsed, setIsLibraryCollapsed] = useState(false);

  // 弹窗状态
  const [isCreateTemplateModalOpen, setIsCreateTemplateModalOpen] = useState(false);
  const [editingTemplate, setEditingTemplate] = useState<CustomDeviceTemplate | null>(null);

  const [isMountModalOpen, setIsMountModalOpen] = useState(false);
  const [targetSlotU, setTargetSlotU] = useState(1);
  const [mountModalSourceType, setMountModalSourceType] = useState<'template' | 'managed' | 'custom'>('template');
  const [mountModalDeviceId, setMountModalDeviceId] = useState<string>('');
  const [mountModalUHeight, setMountModalUHeight] = useState<number>(1);

  const [roomRackModalMode, setRoomRackModalMode] = useState<'room' | 'rack' | null>(null);

  // 点击空闲插槽上架
  const handleSlotClick = (uNumber: number) => {
    setTargetSlotU(uNumber);
    setMountModalSourceType('template');
    setMountModalDeviceId('');
    setMountModalUHeight(1);
    setIsMountModalOpen(true);
  };

  // 从左侧设备库指定设备上架
  const handleOpenMountModalForDevice = (device: {
    id: string;
    name: string;
    device_type: string;
    uHeight: number;
  }) => {
    const occupied = new Set<number>();
    mountedSlots.forEach(s => {
      for (let u = s.startU; u <= s.endU; u++) occupied.add(u);
    });
    const totalU = currentRack?.heightU || 42;
    let freeStartU = 1;
    for (let u = 1; u <= totalU - device.uHeight + 1; u++) {
      let isAllFree = true;
      for (let offset = 0; offset < device.uHeight; offset++) {
        if (occupied.has(u + offset)) {
          isAllFree = false;
          break;
        }
      }
      if (isAllFree) {
        freeStartU = u;
        break;
      }
    }

    setTargetSlotU(freeStartU);
    setMountModalSourceType('managed');
    setMountModalDeviceId(device.id);
    setMountModalUHeight(device.uHeight);
    setIsMountModalOpen(true);
  };

  // 从左侧设备库拖拽到插槽直接上架
  const handleMountDrop = (uNumber: number, template: CustomDeviceTemplate) => {
    if (!currentRack) return;
    const endU = Math.min(currentRack.heightU || 42, uNumber + template.uHeight - 1);
    mountMutation.mutate({
      rack_id: currentRack.id,
      device_id: template.id,
      device_name: template.name,
      device_type: template.hardwareType === 'server' ? 'server' : 'network_device',
      start_u: uNumber,
      end_u: endU,
      position_face: 'front',
      notes: template.description,
    });
  };

  // 快速上架到当前机柜下一个空闲 U 位
  const handleMountToNextFree = (device: {
    name: string;
    uHeight: number;
    hardwareType?: any;
    deviceId?: string;
    deviceType?: string;
  }) => {
    if (!currentRack) return;

    // 寻找可容纳该 U 高度的空闲位置（从下往上）
    const occupied = new Set<number>();
    mountedSlots.forEach(s => {
      for (let u = s.startU; u <= s.endU; u++) occupied.add(u);
    });

    let foundStartU = -1;
    const totalU = currentRack.heightU || 42;
    for (let u = 1; u <= totalU - device.uHeight + 1; u++) {
      let isAllFree = true;
      for (let offset = 0; offset < device.uHeight; offset++) {
        if (occupied.has(u + offset)) {
          isAllFree = false;
          break;
        }
      }
      if (isAllFree) {
        foundStartU = u;
        break;
      }
    }

    if (foundStartU === -1) {
      // 打开手动上架对话框
      setTargetSlotU(1);
      setIsMountModalOpen(true);
      return;
    }

    mountMutation.mutate({
      rack_id: currentRack.id,
      device_id: device.deviceId,
      device_name: device.name,
      device_type: device.deviceType || (device.hardwareType === 'server' ? 'server' : 'network_device'),
      start_u: foundStartU,
      end_u: foundStartU + device.uHeight - 1,
      position_face: 'front',
    });
  };

  // 拖拽模板起始事件
  const handleDragStartTemplate = (e: React.DragEvent, tpl: CustomDeviceTemplate) => {
    e.dataTransfer.setData('application/json', JSON.stringify(tpl));
  };

  // 下架设备
  const handleUnmountDevice = (slotId: string) => {
    Modal.confirm({
      title: '确认下架设备',
      content: '确定要从机架上下架该设备吗？设备将回到设备库。',
      okText: '下架',
      okType: 'danger',
      cancelText: '取消',
      onOk: () => unmountMutation.mutate(slotId),
    });
  };

  return (
    <div className="rack-canvas min-h-full p-4 md:p-6 flex flex-col gap-5 select-none font-sans">
      {/* 1. 顶部机房与新增操作栏 */}
      <RoomRackHeader
        rooms={rooms}
        racks={racks}
        selectedRoomId={selectedRoomId}
        selectedRackId={selectedRackId}
        onSelectRoom={setSelectedRoomId}
        onSelectRack={setSelectedRackId}
        onAddRoom={() => setRoomRackModalMode('room')}
        onAddRack={() => setRoomRackModalMode('rack')}
        currentRoom={currentRoom}
      />

      {/* 2. 主工作区：左侧设备库 + 右侧高拟真 2D 42U 立面机架 */}
      <div className="flex-1 flex flex-col md:flex-row items-start gap-6 relative">
        {/* 左侧设备库侧边栏（对照截图） */}
        <DeviceLibraryPanel
          isCollapsed={isLibraryCollapsed}
          onToggleCollapse={() => setIsLibraryCollapsed(!isLibraryCollapsed)}
          activeCategory={activeCategory}
          onSelectCategory={setActiveCategory}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          selectedSlotDevice={selectedSlotDevice}
          onUnmountDevice={handleUnmountDevice}
          templates={filteredTemplates}
          unallocatedDevices={unallocatedDevices}
          onMountDeviceToNextFree={handleMountToNextFree}
          onOpenMountModalForDevice={handleOpenMountModalForDevice}
          onOpenCreateTemplateModal={() => {
            setEditingTemplate(null);
            setIsCreateTemplateModalOpen(true);
          }}
          onEditTemplate={tpl => {
            setEditingTemplate(tpl);
            setIsCreateTemplateModalOpen(true);
          }}
          onDeleteTemplate={handleDeleteTemplate}
          onDragStartTemplate={handleDragStartTemplate}
        />

        {/* 右侧 2D 真实机柜立面图渲染区（对照截图） */}
        <div className="flex-1 w-full flex justify-center overflow-x-auto py-2">
          <RackCabinet2D
            rack={currentRack}
            mountedSlots={mountedSlots}
            selectedDevice={selectedSlotDevice}
            onSelectDevice={setSelectedSlotDevice}
            onSlotClick={handleSlotClick}
            onMountDrop={handleMountDrop}
            zoomScale={zoomScale}
            onZoomChange={setZoomScale}
            totalU={currentRack?.heightU || 42}
            rackList={racks}
            selectedRackId={selectedRackId}
            onSelectRack={setSelectedRackId}
          />
        </div>
      </div>

      {/* 3. 模态弹窗组件 */}
      <CustomDeviceModal
        isOpen={isCreateTemplateModalOpen}
        onClose={() => setIsCreateTemplateModalOpen(false)}
        onSave={tpl => {
          if (editingTemplate) {
            handleUpdateTemplate(editingTemplate.id, tpl);
          } else {
            handleCreateTemplate(tpl);
          }
        }}
        editingTemplate={editingTemplate}
      />

      <MountDeviceModal
        isOpen={isMountModalOpen}
        onClose={() => setIsMountModalOpen(false)}
        targetStartU={targetSlotU}
        maxU={currentRack?.heightU || 42}
        templates={filteredTemplates}
        unallocatedDevices={unallocatedDevices}
        onMount={params => mountMutation.mutate(params)}
        rackId={currentRack?.id || ''}
        rackName={currentRack?.name || 'R1'}
        initialSourceType={mountModalSourceType}
        initialDeviceId={mountModalDeviceId}
        initialUHeight={mountModalUHeight}
        mountedSlots={mountedSlots}
      />

      <AddRoomRackModal
        isOpen={roomRackModalMode !== null}
        onClose={() => setRoomRackModalMode(null)}
        mode={roomRackModalMode || 'room'}
        currentRoomId={selectedRoomId}
        rooms={rooms}
      />
    </div>
  );
}
