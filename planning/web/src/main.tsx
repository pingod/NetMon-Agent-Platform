/**
 * 应用入口。挂载 store 的首次数据加载与 React 根。
 *
 * 注意：**不做 React Router**。四个视图是并列 tab，不是层级路由，
 * 引入 router 只会给"离线单机，刷新回首页"添麻烦。
 * 若将来需要深链（"把这个机柜发给同事"），再做 URL hash 同步。
 */

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';

import { App } from './App';
import { parseHash, useAppStore } from './store/appStore';

import 'ag-grid-community/styles/ag-grid.css';
import 'ag-grid-community/styles/ag-theme-quartz.css';
import './styles.css';

// AG Grid 36 起社区功能也需显式注册；否则 getRowClass / cellClass 会被忽略，
// 布线表的行高亮与单元格样式全部失效（控制台 error #200）。
import { CellStyleModule, ModuleRegistry, RowStyleModule } from 'ag-grid-community';
ModuleRegistry.registerModules([RowStyleModule, CellStyleModule]);

const root = document.getElementById('root');
if (!root) throw new Error('#root not found');

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);

// 浏览器前进/后退要跟着切 tab，否则用户按返回键会"什么都没发生"
window.addEventListener('hashchange', () => {
  // 用 `parseHash` 而不是自己 split：`#design?edit=1` 里 tab 名是 `design`，
  // 直接整串比对会认为「不是合法 tab」然后什么都不做 —— 表现为
  // 「点了深链没反应」或者「后退键失灵」，而且只在带参数的链接上出现。
  const { tab } = parseHash(window.location.hash);
  if (useAppStore.getState().tab !== tab) useAppStore.getState().setTab(tab);
});

void useAppStore.getState().bootstrap();
