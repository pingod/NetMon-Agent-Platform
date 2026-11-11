/**
 * 3D 机房视图（FR-7.1~7.4）。
 *
 * ## 为什么场景数据不进全局 store
 *
 * `appStore` 的 `bootstrap()` 一次拉 meta/floor/racks/policies 四个小载荷。
 * 场景载荷 180KB（834 台设备 + 362 个柜），塞进去会让**每次进首页**
 * 都多下载 180KB —— 而大多数用户根本不点 3D。
 *
 * 所以这里按需加载：只有切到本页签才请求，且同一个会话内缓存
 * （`loadedRef` 保证不重复拉）。刷新按钮才重新拉。
 *
 * ## 为什么点机柜要跳去 2D
 *
 * 3D 的机柜只是一个盒子，看不到柜里 U3 那台具体是什么设备。用户点柜是
 * 想「看这个柜里有什么」—— 那个信息在 2D U 位图里。所以点柜 = 切到
 * 「平面与机柜」页 + 选中那个柜（FR-7.3 的联动）。
 *
 * 反过来的联动（2D 选中 → 3D 高亮）由 `selectedRack` 传下去实现。
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import type { JSX } from 'react';

import type { ScenePayload } from '../api/client';
import { api } from '../api/client';
import { useAppStore } from '../store/appStore';
import { Scene3D } from './Scene3D';

export function Scene3DView(): JSX.Element {
  const [scene, setScene] = useState<ScenePayload | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const loadedRef = useRef(false);

  const selectRack = useAppStore((s) => s.selectRack);
  const setTab = useAppStore((s) => s.setTab);
  const selChannel = useAppStore((s) => s.selection.channelNo);
  const selRack = useAppStore((s) => s.selection.seqNo);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const payload = await api.scene();
      setScene(payload);
    } catch (exc) {
      setError(exc instanceof Error ? exc.message : String(exc));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;
    void load();
  }, [load]);

  /** 点机柜 → 切到 2D 并选中它。 */
  const onPickRack = useCallback(
    (channel: number, rack: number) => {
      setTab('floor');
      void selectRack(channel, rack);
    },
    [selectRack, setTab],
  );

  const selected =
    selChannel !== null && selRack !== null
      ? { channel: selChannel, rack: selRack }
      : null;

  if (error) {
    return (
      <div className="view-placeholder">
        <b>场景加载失败：</b>
        {error}
        <button type="button" className="btn-mini" onClick={() => void load()}>
          重试
        </button>
      </div>
    );
  }

  if (loading || !scene) {
    return <div className="view-placeholder">正在加载 3D 场景…</div>;
  }

  return (
    <div className="scene3d-view">
      <header className="scene3d-head">
        <div>
          <h3>3D 机房</h3>
          <p className="subtle">
            {scene.totals.rooms} 个房间 · {scene.totals.racks} 个柜位（
            {scene.totals.racksWithDevices} 个有设备）· {scene.totals.devices} 台设备
          </p>
        </div>
        <div className="scene3d-legend">
          {(
            [
              ['core', '核心'],
              ['switch', '接入/交换机'],
              ['server', '服务器'],
              ['storage', '存储'],
              ['security', '安全'],
              ['other', '其它'],
            ] as const
          ).map(([key, label]) => (
            <span key={key} className={`legend-item cat-${key}`}>
              <i />
              {label}
            </span>
          ))}
        </div>
      </header>

      <Scene3D scene={scene} onPickRack={onPickRack} selectedRack={selected} />

      {/*
        诚实边界。3D 只到「设备盒子」粒度，不画走线、不画子刀片、
        不显示端口 —— 这些是刻意的取舍，不是没做完。
      */}
      <div className="scene3d-note subtle">
        3D 只画到「设备盒子」这一层：柜内子刀片、端口面板、实际走线都不画。
        线长与走线路径请看「布线表」，柜内 U 位详情点机柜会跳到 2D。
      </div>
    </div>
  );
}

export default Scene3DView;
