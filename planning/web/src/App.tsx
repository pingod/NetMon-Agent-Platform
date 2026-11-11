/**
 * 应用外壳：三视图一真相（架构 §8.1）。
 *
 * ## 布局决策：平面图 + U 位图并排，表格/体检/端口占满下半屏
 *
 * 因为用户的真实工作流是「看平面图挑机柜 → 看 U 位图确认位置 →
 * 查这个机柜的连线」。前两步是空间性的、要并排对照；
 * 后一步是列表性的、需要宽度。所以上半固定双栏、下半独占。
 *
 * ## 为什么选中状态用颜色而不是箭头连线
 *
 * v0.2 不做 2D 连线绘制（那是 P1 的 FR-6.6）。用"选中设备在三处同时高亮"
 * 已经能覆盖 80% 的核对场景，而画连线要引入路径布局、
 * 交叉避免、性能降级一整套东西 —— 留到有真实需求再做。
 */

import { lazy, Suspense } from 'react';
import type { JSX } from 'react';

import { useAppStore } from './store/appStore';
import type { TabKey } from './store/appStore';
import { CablesView } from './views/CablesView';
import { CompareView } from './views/CompareView';
import { LedgerView } from './views/LedgerView';
import { ModelsView } from './views/ModelsView';
import { LabelsView } from './views/LabelsView';
import { ConsumablesView } from './views/ConsumablesView';
import { ProjectsView } from './views/ProjectsView';
import { DesignView } from './views/DesignView';
import { FloorView } from './views/FloorView';
import { HealthView } from './views/HealthView';
import { PortsView } from './views/PortsView';
import { RackView } from './views/RackView';


/**
 * 3D 视图懒加载。
 *
 * ## 为什么必须懒加载（不是为了"体积好看"）
 *
 * three.js 一家就 530KB（压缩后）。静态 import 的话，**每个打开首页的人**
 * 都要下载它 —— 而绝大多数用户从不点 3D。这个项目要离线交付，
 * 首屏多 530KB 在跳板机上就是多等一秒多。
 *
 * 懒加载后 three 单独打成一个 chunk，只有真的点进「3D 机房」才下载。
 * `Suspense` 的 fallback 用一句话而不是转圈：加载通常 <1s，
 * 转圈反而像卡住了。
 */
const Scene3DView = lazy(() =>
  import('./views/Scene3DView').then((m) => ({ default: m.Scene3DView })),
);

const TABS: { key: TabKey; label: string; hint: string }[] = [
  { key: 'floor', label: '平面与机柜', hint: '机房平面图 + U 位图' },
  { key: 'design', label: '接线设计', hint: '949 行意图 → 5127 行布线表' },
  { key: 'cables', label: '布线表', hint: '5127 行连线明细' },
  { key: 'health', label: '数据体检', hint: '导入即体检' },
  { key: 'ports', label: '端口与策略', hint: '端口使用率 + 长度来源' },
  { key: 'consumables', label: '耗材清单', hint: '要买多少根线 / 多少个模块' },
  { key: 'projects', label: '项目与版本', hint: '改了能存、能比、能回退' },
  { key: 'compare', label: '版本对比', hint: 'V8→V13 六版 Excel 差了什么' },
  { key: 'ledger', label: '设备台账', hint: '每台设备用了几个口、还剩几个' },
  { key: 'models', label: '设备型号库', hint: '端口模板：每个型号几个口、谁说的' },
  { key: 'labels', label: '线缆标签', hint: '每端一张贴纸印什么字' },
  { key: 'scene3d', label: '3D 机房', hint: '三维看排布：拖动旋转、点柜进 2D' },
];

export function App(): JSX.Element {
  const meta = useAppStore((s) => s.meta);
  const tab = useAppStore((s) => s.tab);
  const loading = useAppStore((s) => s.loading);
  const error = useAppStore((s) => s.error);
  const setTab = useAppStore((s) => s.setTab);
  const reload = useAppStore((s) => s.reload);
  const selection = useAppStore((s) => s.selection);
  const clearSelection = useAppStore((s) => s.clearSelection);

  // 嵌入 NetMon 时继承主平台的浅色视觉，并让侧边栏成为唯一导航外壳
  const embedded = typeof window !== 'undefined' && window.self !== window.top;

  return (
    <div className={`app${embedded ? ' is-embedded' : ''}`}>
      <header className="app-header">
        <div className="brand">
          <span className="brand-mark">IDC</span>
          <div>
            <h1>IDCBoxLine</h1>
            <p className="subtle">机柜上架规划与布线自动化</p>
          </div>
        </div>

        {meta && (
          <div className="header-meta">
            <span>
              <b>{meta.rows.effective}</b> 行连线
            </span>
            <span>
              <b>{meta.geometry.rackPositions}</b> 机柜位
            </span>
            <span>
              <b>{meta.devices.normalized}</b> 设备
            </span>
            {meta.cables.overLimitRows > 0 && (
              <span className="stat-warn">
                <b>{meta.cables.overLimitRows}</b> 行超档位
              </span>
            )}
          </div>
        )}

        <div className="header-actions">
          {selection.deviceName && (
            <button type="button" className="ghost" onClick={clearSelection}>
              取消选中设备
            </button>
          )}
          <button type="button" onClick={() => void reload()} disabled={loading}>
            {loading ? '加载中…' : '重新加载'}
          </button>
        </div>
      </header>

      <nav className="app-tabs">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            className={`tab${tab === t.key ? ' active' : ''}`}
            onClick={() => setTab(t.key)}
            title={t.hint}
          >
            {t.label}
          </button>
        ))}
        {selection.deviceName && (
          <span className="selection-hint">
            已选中 <code>{selection.deviceName}</code>
          </span>
        )}
      </nav>

      {error && (
        <div className="error-box">
          <b>加载失败：</b>
          {error}
          <button type="button" onClick={() => void reload()}>
            重试
          </button>
        </div>
      )}

      <main className={`app-main tab-${tab}`}>
        {tab === 'floor' && (
          <div className="split">
            <section className="pane pane-floor">
              <FloorView />
            </section>
            <section className="pane pane-rack">
              <RackView />
            </section>
          </div>
        )}
        {tab === 'design' && <DesignView />}
        {tab === 'cables' && <CablesView />}
        {tab === 'health' && <HealthView />}
        {tab === 'ports' && <PortsView />}
        {tab === 'consumables' && <ConsumablesView />}
        {tab === 'projects' && <ProjectsView />}
        {tab === 'compare' && <CompareView />}
        {tab === 'ledger' && <LedgerView />}
        {tab === 'models' && <ModelsView />}
        {tab === 'labels' && <LabelsView />}
        {tab === 'scene3d' && (
          <Suspense fallback={<div className="view-placeholder">正在加载 3D 模块…</div>}>
            <Scene3DView />
          </Suspense>
        )}
      </main>

      <footer className="app-footer">
        <span>
          源文件：<code>{meta?.sourceFile ?? '—'}</code>
        </span>
        <span className="subtle">
          导出 24 列逐格保真 · 体检 {meta ? `${meta.cables.computed} 行已计算` : '—'} ·
          {embedded ? 'NetMon 内置引擎' : '离线单机运行'}
        </span>
      </footer>
    </div>
  );
}
