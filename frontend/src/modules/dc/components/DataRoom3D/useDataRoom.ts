import { useState, useEffect, useRef, useCallback } from 'react';
import { logger } from '@/lib/logger';
import api from '../../../../lib/api';
import type { Rack3D, SlotInfo, OverviewData, AlertItem } from './types';

interface RawSlotItem {
  rack_id: string;
  slot_id: string;
  start_u: number;
  end_u: number;
  device_name?: string;
  device_id?: string;
  device_type?: string;
  device_status?: string;
  server_status?: string;
  cpu_usage?: number | null;
  memory_usage?: number | null;
  disk_usage?: number | null;
}

interface RawRackItem {
  id: string;
  name: string;
  row_number?: number;
  total_u?: number;
  used_u?: number;
  device_count?: number;
  position_x?: number;
  position_z?: number;
}

interface RawAlertItem {
  id: string;
  title: string;
  severity: string;
  status?: string;
  source?: string;
  created_at?: string;
}

interface UseDataRoomReturn {
  loading: boolean;
  racks: Rack3D[];
  overview: OverviewData | null;
  isReal: boolean;
  alerts: AlertItem[];
  alertsList: AlertItem[];
  selectedRack: Rack3D | null;
  rackSlots: SlotInfo[];
  rackSlotsMap: Record<string, SlotInfo[]>;
  slotDetailOpen: boolean;
  startTime: number;
  fetchRackSlots: (rackId: string) => Promise<void>;
  setSelectedRack: (rack: Rack3D | null) => void;
  setSlotDetailOpen: (open: boolean) => void;
}

export default function useDataRoom(): UseDataRoomReturn {
  const [racks, setRacks] = useState<Rack3D[]>([]);
  const [overview, setOverview] = useState<OverviewData | null>(null);
  const [loading, setLoading] = useState(true);
  const [isReal, setIsReal] = useState(false);
  const [alerts, setAlerts] = useState<AlertItem[]>([]);
  const [alertsList, setAlertsList] = useState<AlertItem[]>([]);
  const [selectedRack, setSelectedRack] = useState<Rack3D | null>(null);
  const [rackSlots, setRackSlots] = useState<SlotInfo[]>([]);
  const [rackSlotsMap, setRackSlotsMap] = useState<Record<string, SlotInfo[]>>({});
  const [slotDetailOpen, setSlotDetailOpen] = useState(false);
  const startTime = useRef(Date.now());

  /** 加载概览 + 机柜 + 槽位数据 */
  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      try {
        const [ovRes, rackRes, slotsRes] = await Promise.all([
          api.get('/dc/overview'),
          api.get('/dc/racks'),
          api.get('/dc/batch-slots').catch(() => ({ data: { data: [] } })),
        ]);

        if (cancelled) return;

        const ov = ovRes.data.data as OverviewData;
        setOverview(ov);
        setIsReal(true);

        // 按机柜分组 U 位
        const slotsByRack: Record<string, SlotInfo[]> = {};
        for (const s of (slotsRes.data.data || []) as RawSlotItem[]) {
          if (!slotsByRack[s.rack_id]) slotsByRack[s.rack_id] = [];
          slotsByRack[s.rack_id].push({
            id: s.slot_id,
            startU: s.start_u,
            endU: s.end_u,
            deviceName: s.device_name || s.device_id || '',
            deviceType: s.device_type || '',
            deviceStatus: s.device_status || 'unknown',
            cpuUsage: null,
            memUsage: null,
            diskUsage: null,
          });
        }
        setRackSlotsMap(slotsByRack);

        // 处理机柜列表
        const rawRacks = (rackRes.data.data || []) as RawRackItem[];
        if (rawRacks.length > 0) {
          setRacks(rawRacks.map((r: RawRackItem) => ({
            id: r.id,
            name: r.name,
            roomName: '',
            roomLabel: '',
            row: r.row_number || 1,
            totalU: r.total_u || 42,
            usedU: r.used_u || 0,
            deviceCount: r.device_count || 0,
            alertCount: 0,
            deviceStatus: 'normal',
            sceneX: r.position_x || 0,
            sceneZ: r.position_z || 0,
          })));
        } else if (ov.rackData) {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          setRacks(ov.rackData.map((r: any, i: number) => ({
            id: r.id || `rack-${i}`,
            name: r.name,
            roomName: r.room_name || '',
            roomLabel: r.room_label || '',
            row: r.row_number || 1,
            totalU: r.total_u || 42,
            usedU: r.used_u || 0,
            deviceCount: r.device_count || 0,
            alertCount: r.alert_count || 0,
            deviceStatus: (r.alert_count || 0) > 0 ? 'warning' : 'normal',
            sceneX: r.position_x || 0,
            sceneZ: r.position_z || 0,
          })));
        } else {
          setRacks([]);
        }
      } catch (err) {
        logger.error('加载数据中心数据失败:', err);
        // API 失败时显示空状态，不使用假数据
        if (!cancelled) {
          setRacks([]);
          setOverview(null);
          setIsReal(false);
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    load();
    // 后端无实时推送，轮询刷新概览与机柜利用率
    const timer = setInterval(load, 60000);
    return () => { cancelled = true; clearInterval(timer); };
  }, []);

  /** 加载告警数据（活动告警 = 未 resolved/closed；'open' 不是合法状态值） */
  useEffect(() => {
    let cancelled = false;

    const loadAlerts = () => {
      api.get('/alerts', { params: { limit: 50 } })
        .then(res => {
          if (cancelled) return;
          const items = ((res.data.data || []) as RawAlertItem[])
            .filter(a => a.status !== 'resolved' && a.status !== 'closed')
            .map(a => ({
              id: a.id,
              title: a.title,
              severity: a.severity,
              source: a.source,
              time: a.created_at,
            }));
          setAlerts(items);
          setAlertsList(items);
        })
        .catch(() => {
          if (!cancelled) {
            setAlerts([]);
            setAlertsList([]);
          }
        });
    };

    loadAlerts();
    const timer = setInterval(loadAlerts, 30000);
    return () => { cancelled = true; clearInterval(timer); };
  }, []);

  /** 加载选中机柜的 U 位详情 */
  const fetchRackSlots = useCallback(async (rackId: string) => {
    try {
      const res = await api.get(`/dc/slots/${rackId}`);
      const data = (res.data.data || []) as RawSlotItem[];
      setRackSlots(data.map((s: RawSlotItem) => ({
        id: s.slot_id,
        startU: s.start_u,
        endU: s.end_u,
        deviceName: s.device_name || s.device_id || '',
        deviceType: s.device_type || '',
        deviceStatus: s.device_status || s.server_status || 'unknown',
        cpuUsage: s.cpu_usage ?? null,
        memUsage: s.memory_usage ?? null,
        diskUsage: s.disk_usage ?? null,
      })));
    } catch (err) {
      logger.error('加载机柜 U 位失败:', err);
      setRackSlots([]);
    }
  }, []);

  return {
    loading,
    racks,
    overview,
    isReal,
    alerts,
    alertsList,
    selectedRack,
    rackSlots,
    rackSlotsMap,
    slotDetailOpen,
    startTime: startTime.current,
    fetchRackSlots,
    setSelectedRack,
    setSlotDetailOpen,
  };
}
