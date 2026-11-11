/**
 * 全局状态。**单一 store + 纯 selector**（架构 §8.2）。
 *
 * ## 为什么不用 React Query / SWR
 *
 * 数据是「一个项目的一次读取结果」，不是高频轮询的资源。
 * 用 React Query 会引入缓存失效策略、window focus 重取、重试退避等
 * 一整套为"互联网应用"设计的行为 —— 而我们是离线单机，
 * 数据只在用户点"重新加载"时变。多一层抽象只增加心智负担。
 *
 * ## 为什么选择状态（选中设备/机柜）也在 store 里
 *
 * 2D 平面图、U 位图、布线表三处都要响应"我点了这个设备"：
 * 平面图高亮它所在机柜、U 位图滚动到它的位置、表格筛出它的连线。
 * 把选中状态放在 store 里，三者共享一个真相；
 * 放在某个组件的 useState 里就得靠回调层层传递。
 */

import { create } from 'zustand';

import type {
  CablesPayload,
  FloorPayload,
  HealthReportPayload,
  MetaPayload,
  PolicyPayload,
  RackPayload,
  RackListItem,
} from '../api/client';
import { api } from '../api/client';

export type TabKey =
  | 'floor'
  | 'design'
  | 'cables'
  | 'health'
  | 'ports'
  | 'consumables'
  | 'projects'
  | 'compare'
  | 'ledger'
  | 'models'
  | 'labels'
  | 'scene3d';

/** 选中目标。设备级优先于机柜级（点设备时两者都设，以设备为准）。 */
export interface Selection {
  channelNo: number | null;
  seqNo: number | null;
  deviceName: string | null;
}

export interface CableFilter {
  channel: number | null;
  rack: number | null;
  cableType: string | null;
  text: string;
}

interface AppState {
  // ---- 数据
  meta: MetaPayload | null;
  floor: FloorPayload | null;
  racks: RackListItem[];
  rack: RackPayload | null;
  cables: CablesPayload | null;
  health: HealthReportPayload | null;
  policies: PolicyPayload | null;

  // ---- 界面状态
  tab: TabKey;
  selection: Selection;
  filter: CableFilter;
  cablePage: number;
  loading: boolean;
  error: string | null;

  // ---- 动作
  bootstrap: () => Promise<void>;
  reload: () => Promise<void>;
  setTab: (tab: TabKey) => void;
  selectRack: (channelNo: number, seqNo: number) => Promise<void>;
  selectDevice: (channelNo: number, seqNo: number, deviceName: string) => Promise<void>;
  clearSelection: () => void;
  setFilter: (patch: Partial<CableFilter>) => Promise<void>;
  setCablePage: (page: number) => Promise<void>;
}

/** 每页行数。200 行 ≈ 首屏 4%，是"5127 行也能秒开"的关键。 */
export const PAGE_SIZE = 200;

/**
 * 全部合法 tab。**这是唯一来源** —— `main.tsx` 的 hashchange 监听、
 * `readInitialTab`、以及深链校验都从这里取，避免「七个 tab 里三个
 * 按浏览器后退键切不过去」这种只在特定路径下才会发现的缺陷。
 */
export const TAB_KEYS: TabKey[] = [
  'floor',
  'design',
  'cables',
  'health',
  'ports',
  'consumables',
  'projects',
  'compare',
  'ledger',
  'models',
  'labels',
  'scene3d',
];

/**
 * 把 `#design?edit=1` 拆成 `{tab:'design', params:{edit:'1'}}`。
 *
 * ## 为什么参数要放在 hash 里
 *
 * 这是单页应用，路由信息全在 hash（`#`）里。若把 `edit=1` 写成
 * `location.search`（即 `?`），浏览器刷新时 `?` 部分会保留但
 * `setTab` 里的 `history.replaceState` 会把 `?` 抹掉 —— 用户刷新一次
 * 编辑态就丢了。放在 hash 内则由我们自己完全控制。
 */
export function parseHash(hash: string): {
  tab: TabKey;
  params: Record<string, string>;
} {
  const raw = hash.replace(/^#/, '');
  const qmark = raw.indexOf('?');
  const name = qmark < 0 ? raw : raw.slice(0, qmark);
  const query = qmark < 0 ? '' : raw.slice(qmark + 1);
  const params: Record<string, string> = {};
  for (const [k, v] of new URLSearchParams(query)) params[k] = v;
  const tab = (TAB_KEYS as string[]).includes(name) ? (name as TabKey) : 'floor';
  return { tab, params };
}

/** 从 URL hash 读初始 tab。刷新后停在原页面，而不是永远跳回平面图。 */
function readInitialTab(): TabKey {
  if (typeof location === 'undefined') return 'floor';
  return parseHash(location.hash).tab;
}


export const useAppStore = create<AppState>((set, get) => ({
  meta: null,
  floor: null,
  racks: [],
  rack: null,
  cables: null,
  health: null,
  policies: null,

  tab: readInitialTab(),
  selection: { channelNo: null, seqNo: null, deviceName: null },
  filter: { channel: null, rack: null, cableType: null, text: '' },
  cablePage: 0,
  loading: false,
  error: null,

  async bootstrap() {
    set({ loading: true, error: null });
    try {
      // 并行拉取：这 5 个请求互不依赖，串行会让首屏慢 4 倍
      const [meta, floor, rackList, policies] = await Promise.all([
        api.meta(),
        api.floor(),
        api.racks(),
        api.policies(),
      ]);
      set({ meta, floor, racks: rackList.racks, policies, loading: false });

      // 体检报告较重（208 条 findings），不阻塞首屏
      void api
        .healthReport()
        .then((health) => set({ health }))
        .catch(() => {
          /* 体检失败不该让平面图打不开 */
        });

      // 默认选中第一个有设备的机柜，让用户一进来就看到内容而不是空白
      const first = rackList.racks[0];
      if (first) await get().selectRack(first.channelNo, first.seqNo);

      // 深链进来（#cables）时首屏就要有表格数据，不能等用户先点别的 tab
      if (get().tab === 'cables' && !get().cables) {
        await get().setCablePage(0);
      }
    } catch (e) {
      set({ loading: false, error: e instanceof Error ? e.message : String(e) });
    }
  },

  async reload() {
    set({ meta: null, floor: null, rack: null, cables: null, health: null });
    await get().bootstrap();
  },

  setTab(tab) {
    set({ tab });
    // 同步到地址栏：用户可以复制链接发给同事（"看这个机柜"），
    // 也让无头浏览器（截图验证）能直接打开指定视图。
    if (typeof location !== 'undefined' && location.hash !== `#${tab}`) {
      history.replaceState(null, '', `#${tab}`);
    }
    // 切到布线表时若还没数据才拉，避免每次切 tab 都打一次请求
    if (tab === 'cables' && !get().cables) {
      void get().setCablePage(0);
    }
  },

  async selectRack(channelNo, seqNo) {
    set({ selection: { channelNo, seqNo, deviceName: null } });
    try {
      const rack = await api.rack(channelNo, seqNo);
      set({ rack });
    } catch (e) {
      set({ rack: null, error: e instanceof Error ? e.message : String(e) });
    }
  },

  async selectDevice(channelNo, seqNo, deviceName) {
    set({ selection: { channelNo, seqNo, deviceName } });
    // 设备级选中同时保证机柜已加载（平面图点机柜空格时只走 selectRack）
    if (get().rack?.channelNo !== channelNo || get().rack?.seqNo !== seqNo) {
      try {
        const rack = await api.rack(channelNo, seqNo);
        set({ rack });
      } catch {
        /* 机柜加载失败不阻断设备选中 */
      }
    }
  },

  clearSelection() {
    set({ selection: { channelNo: null, seqNo: null, deviceName: null } });
  },

  async setFilter(patch) {
    const filter = { ...get().filter, ...patch };
    set({ filter, cablePage: 0 });
    await get().setCablePage(0);
  },

  async setCablePage(page) {
    const { filter } = get();
    set({ cablePage: page, loading: true });
    try {
      const cables = await api.cables({
        offset: page * PAGE_SIZE,
        limit: PAGE_SIZE,
        channel: filter.channel,
        rack: filter.rack,
        cableType: filter.cableType,
        text: filter.text || null,
      });
      set({ cables, loading: false });
    } catch (e) {
      set({ loading: false, error: e instanceof Error ? e.message : String(e) });
    }
  },
}));

// ---------------------------------------------------------------- selectors
// 纯函数，不依赖 store 实例 —— 可以在测试里直接调

export const selectSelectedRack = (s: AppState): RackPayload | null => s.rack;

export const selectIsSelectedDevice = (
  s: AppState,
  deviceName: string | null | undefined,
): boolean => Boolean(deviceName) && s.selection.deviceName === deviceName;

/** 表格里"当前选中设备的连线"高亮用的稳定键集合。 */
export const selectHighlightKeys = (s: AppState): Set<string> => {
  const out = new Set<string>();
  const { selection, cables } = s;
  if (!cables || !selection.deviceName) return out;
  for (const row of cables.rows) {
    if (row.localDeviceName === selection.deviceName || row.peerDeviceName === selection.deviceName) {
      out.add(row.stableKey);
    }
  }
  return out;
};
