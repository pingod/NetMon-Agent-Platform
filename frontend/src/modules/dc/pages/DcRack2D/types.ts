// frontend/src/modules/dc/pages/DcRack2D/types.ts

export type DeviceCategory = 'managed' | 'custom' | 'odf';

export type DeviceHardwareType = 'switch' | 'router' | 'firewall' | 'server' | 'patch_panel' | 'odf' | 'pdu' | 'other';

export interface CustomDeviceTemplate {
  id: string;
  name: string;
  category: DeviceCategory;
  hardwareType: DeviceHardwareType;
  vendor: string;
  model: string;
  uHeight: number; // 占用 U 数 (e.g. 1, 2, 4, 8)
  description?: string;
  panelColor?: string; // 前面板主色调 (e.g. '#1e293b', '#7f1d1d')
  portsCount?: number; // 端口数量 (e.g. 24, 48)
  frontImage?: string; // 自定义前面板图片 Base64 或 URL
  isSystemPreset?: boolean;
}

export interface MountedSlotDevice {
  id: string; // Slot ID
  rackId: string;
  deviceId: string;
  deviceName: string;
  deviceType: 'server' | 'network_device' | 'custom';
  hardwareType?: DeviceHardwareType;
  startU: number; // 从第几 U 开始 (从下往上 1-42)
  endU: number;   // 到第几 U 结束
  uHeight: number;
  positionFace?: 'front' | 'rear';
  status: 'online' | 'warning' | 'critical' | 'offline' | 'unknown';
  ipAddress?: string;
  vendor?: string;
  model?: string;
  powerW?: number;
  temperature?: number;
  cpuUsage?: number;
  memoryUsage?: number;
  frontImage?: string;
  notes?: string;
}

export interface DcRackInfo {
  id: string;
  roomId: string;
  roomName?: string;
  name: string;
  heightU: number; // 标准 42U
  status: 'online' | 'warning' | 'offline' | 'normal';
  maxPowerW?: number;
  currentPowerW?: number;
  currentTemp?: number;
  slots: MountedSlotDevice[];
}

export interface DcRoomInfo {
  id: string;
  name: string;
  floor?: string;
  areaSqm?: number;
  racksCount?: number;
  currentTemp?: number;
  currentHumidity?: number;
  pue?: number;
  totalPowerKw?: number;
}
