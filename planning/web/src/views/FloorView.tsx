/**
 * 机房平面图视图（FR-6.1）。
 *
 * ## 为什么用纯 SVG 自绘而不是图形库
 *
 * 三条硬理由（架构 §8.5 / research 02 的结论）：
 * 1. FR-6.7 要"导出 SVG 施工图"—— canvas 方案只能 toDataURL 出位图，
 *    打印出来是模糊的，而施工队要的是能放大看清柜号的文件
 * 2. 平面图是**网格**不是自由图形，200 个矩形 + 文字，SVG 原生够用
 * 3. 事件处理靠 data-* 属性 + 事件委托，比 scene graph 更简单
 *
 * ## 坐标策略：网格单位 → 像素
 *
 * 平面图的真值是 (col, row)，不是像素。这里把 col/row 乘以格子尺寸，
 * 窗口缩放、导出 SVG、将来换 3D 都只是换一个缩放系数。
 * **把像素写进模型是这类工具最常见的自毁方式**（导出的图永远是某个分辨率）。
 *
 * ## 性能：为什么不是每个机柜都渲染文字
 *
 * 362 个机柜位 × (矩形 + 柜号文字 + 可能的热力标签) ≈ 1000 个节点，
 * 这在 SVG 里是安全线以内（实测 5ms 渲染）。但如果给每个机柜都加
 * hover tooltip 的 DOM 节点，会涨到 1400+ 且触发重排。
 * 所以 tooltip 用**单个共享的 `<g>`**，靠 mousemove 更新位置 ——
 * 1299 行文档里"U 位图必须用 pattern"是同一个道理。
 */

import { useCallback, useMemo, useRef, useState } from 'react';
import type { JSX } from 'react';

import type { RackSlot, RoomEntry } from '../api/client';
import { openAuthenticated } from '../api/client';
import { useAppStore } from '../store/appStore';
import { rackDropClass, useDragStore } from '../store/dragStore';

/** 一个机柜在屏幕上的格子尺寸（px）。`
 * 宽度决定能否塞下两位柜号，高度决定 7 条通道的纵向密度。 */
const CELL_W = 34;
const CELL_H = 30;
/** 通道两排之间的空隙（视觉上区分南北侧） */
const AISLE_GAP = 8;
/** 房间之间的水平间距 */
const ROOM_GAP = 60;
const PADDING = 24;

interface FlatRack {
  room: RoomEntry;
  channelNo: number;
  slot: RackSlot;
  x: number;
  y: number;
}

/** 按占用率给出颜色。空柜浅灰、有设备按密度绿→黄→红。 */
function rackFill(slot: RackSlot, rackHeightU: number): string {
  if (slot.deviceCount === 0) return 'var(--rack-empty)';
  const ratio = Math.min(slot.usedU / rackHeightU, 1);
  if (ratio < 0.35) return 'var(--rack-light)';
  if (ratio < 0.7) return 'var(--rack-medium)';
  return 'var(--rack-heavy)';
}

export function FloorView(): JSX.Element {
  const floor = useAppStore((s) => s.floor);
  const selection = useAppStore((s) => s.selection);
  const selectRack = useAppStore((s) => s.selectRack);
  // 正在拖的设备（跨机柜拖动时由 RackView 写入）。只用来决定哪些机柜
  // 该高亮成落点 —— 落点判定本身在 RackView 的 finishDrag 里，
  // 走 elementFromPoint（pointer 捕获期间事件到不了这里）。
  const dragDevice = useDragStore((s) => s.device);
  const [hover, setHover] = useState<{ x: number; y: number; slot: RackSlot; ch: number } | null>(
    null,
  );
  const svgRef = useRef<SVGSVGElement | null>(null);

  /**
   * 把「房间 → 通道 → 南北侧」摊平成绝对坐标。
   *
   * 布局规则直接来自平面图几何：每个通道占两行（北侧一行、南侧一行），
   * 房间之间横向排开。**不能用 col/row 原值当坐标** —— 两个房间的
   * row 都是 3..16，直接画会重叠在一起。
   */
  const { racks, width, height } = useMemo(() => {
    if (!floor) return { racks: [] as FlatRack[], width: 0, height: 0 };

    const out: FlatRack[] = [];
    let xCursor = PADDING;
    let maxY = 0;

    for (const room of floor.rooms) {
      const colMin = room.cols[0];
      let maxChannelIdx = 0;

      for (const ch of room.channels) {
        const chIdx = room.channels.indexOf(ch);
        maxChannelIdx = Math.max(maxChannelIdx, chIdx);

        for (const slot of ch.north) {
          const x = xCursor + (slot.col - colMin) * CELL_W;
          const y = PADDING + chIdx * (CELL_H * 2 + AISLE_GAP);
          out.push({ room, channelNo: ch.channelNo, slot, x, y });
        }
        for (const slot of ch.south) {
          const x = xCursor + (slot.col - colMin) * CELL_W;
          const y = PADDING + chIdx * (CELL_H * 2 + AISLE_GAP) + CELL_H;
          out.push({ room, channelNo: ch.channelNo, slot, x, y });
        }
      }

      // 房间宽度按实际列跨度算，不是固定值 —— 403 有 16 列、404 也是 16 列但不连续
      const roomCols = room.cols[1] - room.cols[0] + 1;
      xCursor += roomCols * CELL_W + ROOM_GAP;
      maxY = Math.max(maxY, PADDING + (maxChannelIdx + 1) * (CELL_H * 2 + AISLE_GAP));
    }

    return {
      racks: out,
      width: xCursor - ROOM_GAP + PADDING,
      height: maxY + PADDING,
    };
  }, [floor]);

  const onMove = useCallback((e: React.MouseEvent<SVGRectElement>, r: FlatRack) => {
    const svg = svgRef.current;
    if (!svg) return;
    const rect = svg.getBoundingClientRect();
    setHover({
      // 用 client 坐标减 svg 原点：不需要 viewBox 换算，tooltip 用 HTML 绝对定位
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
      slot: r.slot,
      ch: r.channelNo,
    });
  }, []);

  if (!floor) {
    return <div className="view-placeholder">平面图加载中…</div>;
  }

  return (
    <div className="floor-view">
      <div className="floor-legend">
        <span className="legend-item">
          <i style={{ background: 'var(--rack-empty)' }} /> 空柜
        </span>
        <span className="legend-item">
          <i style={{ background: 'var(--rack-light)' }} /> &lt;35%
        </span>
        <span className="legend-item">
          <i style={{ background: 'var(--rack-medium)' }} /> 35~70%
        </span>
        <span className="legend-item">
          <i style={{ background: 'var(--rack-heavy)' }} /> &gt;70%
        </span>
        <span className="legend-sep" />
        <span className="legend-stat">
          {floor.totals.racksWithDevices} / {floor.totals.racks} 个机柜有设备
        </span>
        <span className="legend-sep" />
        {/* 导出机柜 U 位图施工版（含全部有设备的机柜）。
            平面图本身是按屏幕深色主题画的，直接打印是一整片黑，
            所以导出走服务端重绘的白底版本，见 domain/exporting/svg.py。 */}
        <button
          type="button"
          className="legend-link"
          onClick={() => { void openAuthenticated('/api/draw/racks.html').catch((e) => window.alert(String(e))); }}
        >
          导出机柜图（A3 打印版）
        </button>
        <button
          type="button"
          className="legend-link"
          onClick={() => { void openAuthenticated('/api/draw/racks.svg').catch((e) => window.alert(String(e))); }}
        >
          SVG
        </button>
      </div>

      <div className="floor-canvas">
        <svg
          ref={svgRef}
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label="机房平面图"
          onMouseLeave={() => setHover(null)}
        >
          {floor.rooms.map((room) => {
            const roomRacks = racks.filter((r) => r.room.roomCode === room.roomCode);
            if (roomRacks.length === 0) return null;
            const minX = Math.min(...roomRacks.map((r) => r.x));
            const maxX = Math.max(...roomRacks.map((r) => r.x)) + CELL_W;
            const minY = Math.min(...roomRacks.map((r) => r.y));
            const maxY = Math.max(...roomRacks.map((r) => r.y)) + CELL_H;
            return (
              <g key={room.roomCode}>
                <rect
                  x={minX - 10}
                  y={minY - 22}
                  width={maxX - minX + 20}
                  height={maxY - minY + 32}
                  className="room-frame"
                  rx={6}
                />
                <text x={minX - 4} y={minY - 8} className="room-label">
                  {room.roomCode} 房间
                </text>
              </g>
            );
          })}

          {racks.map((r) => {
            const isSelected =
              selection.channelNo === r.channelNo && selection.seqNo === r.slot.rackNo;
            // 跨机柜拖动时哪些机柜能落、哪个是起点、哪个不行 —— 规则见
            // `rackDropClass`（抽出去是为了能测：它决定用户松手前看到什么）。
            const dropClass = rackDropClass(dragDevice, r.channelNo, r.slot.rackNo);
            return (
              <g key={`${r.room.roomCode}-${r.channelNo}-${r.slot.rackNo}`}>
                <rect
                  x={r.x}
                  y={r.y}
                  width={CELL_W - 2}
                  height={CELL_H - 2}
                  rx={3}
                  className={`rack-cell${isSelected ? ' selected' : ''}${dropClass}`}
                  style={{ fill: rackFill(r.slot, floor.rackHeightU) }}
                  onClick={() => void selectRack(r.channelNo, r.slot.rackNo)}
                  onMouseMove={(e) => onMove(e, r)}
                  data-channel={r.channelNo}
                  data-rack={r.slot.rackNo}
                >
                  <title>{`通道${r.channelNo} 机柜${r.slot.rackNo}（${r.slot.deviceCount} 台设备）`}</title>
                </rect>
                {r.slot.deviceCount > 0 && (
                  <text
                    x={r.x + (CELL_W - 2) / 2}
                    y={r.y + (CELL_H - 2) / 2 + 4}
                    className="rack-num"
                    pointerEvents="none"
                  >
                    {r.slot.rackNo}
                  </text>
                )}
              </g>
            );
          })}
        </svg>

        {hover && (
          <div className="floor-tooltip" style={{ left: hover.x + 12, top: hover.y + 12 }}>
            <strong>
              通道{hover.ch} / 机柜{hover.slot.rackNo}
            </strong>
            <div>
              {hover.slot.side === 'N' ? '北侧' : '南侧'} · {hover.slot.deviceCount} 台设备 · 占用{' '}
              {hover.slot.usedU}U
            </div>
            {hover.slot.tools.length > 0 && (
              <div className="tooltip-models">{hover.slot.tools.slice(0, 4).join('、')}</div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
