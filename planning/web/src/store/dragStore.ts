/**
 * 「正在拖拽一台设备」的跨视图共享状态。
 *
 * ## 为什么必须提到组件外
 *
 * 跨机柜拖动的起点在右侧 U 位图、终点在左侧平面图 —— 两个是**兄弟组件**。
 * 局部 `useState` 传不过去，而它们共同的父组件 `App` 只负责布局，
 * 为了一格高亮把它改成 props 中转站会让 `App` 知道「U 位图里正在拖什么」，
 * 这是它不该知道的事。
 *
 * ## 为什么只有这么点内容
 *
 * 拖拽的**几何**（落点第几 U、鬼影线）留在 RackView 本地 —— 那些每帧都变，
 * 放进全局 store 会让平面图跟着每帧重渲染。这里只存「正在拖的是哪台设备」，
 * 一次拖拽里只变两次（begin / end）。平面图订阅它，只为决定
 * 「哪些机柜能当落点、要高亮」。
 *
 * ## 为什么不靠 HTML5 drag-and-drop 的 dragover 判定落点
 *
 * 落点判定走 `document.elementFromPoint`（见 `RackView.finishDrag`），
 * 因为 pointer 捕获期间事件全被起点元素吃掉，平面图根本收不到 pointerup。
 * `elementFromPoint` 不受捕获影响，返回光标下真实的那个元素。
 * 这个 store 因此只承担「告诉平面图我在拖」这一件事，不参与判定。
 */

import { create } from 'zustand';

/** 正在拖的那台设备的身份。与 `editableKey` 的四元组一一对应。 */
export interface DragDevice {
  channel: number;
  rack: number;
  /** 显示名。布局段排布的 663 台没有实例名，这里就是型号。 */
  name: string;
  /** 拖动前的起始 U 位（布局段设备靠它定位到具体那一台）。 */
  startU: number;
  sizeU: number;
}

interface DragState {
  device: DragDevice | null;
  begin: (device: DragDevice) => void;
  end: () => void;
}

export const useDragStore = create<DragState>((set) => ({
  device: null,
  begin: (device) => set({ device }),
  // 结束时不保留「上一次拖了什么」：平面图靠它判断要不要画落点高亮，
  // 留着会让松手后高亮不消失。
  end: () => set({ device: null }),
}));

/**
 * 平面图上某个机柜在当前拖动下的落点样式。
 *
 * 抽成纯函数是因为它决定「用户松手前看到什么」，而这个判断有三个容易
 * 搞错的边界：正在拖的是不是本柜、是不是同通道、有没有在拖。写成 JSX
 * 里的三元表达式既没法测，也没法一眼看出三个分支。
 *
 * - `drop-target`：同通道的**其他**机柜 —— 合法落点（后端只允许同通道换柜）
 * - `drop-origin`：拖动起点所在的那个柜 —— 标出来免得用户以为松手会移动它
 * - `drop-invalid`：别的通道 —— 后端会拒绝；**降透明度但不隐藏**，
 *   隐藏会让平面图在拖动时突然少一半格子，用户以为丢东西了
 */
export function rackDropClass(
  device: DragDevice | null,
  channelNo: number,
  rackNo: number,
): string {
  if (device === null) return '';
  if (device.channel !== channelNo) return ' drop-invalid';
  if (device.rack === rackNo) return ' drop-origin';
  return ' drop-target';
}
