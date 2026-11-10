import { lazyRetry } from '@/lib/lazyRetry';

const Scripts = lazyRetry(() => import('./pages/Scripts'));
const AuditLogs = lazyRetry(() => import('./pages/AuditLogs'));
const Settings = lazyRetry(() => import('./pages/Settings'));
const ToolLinks = lazyRetry(() => import('./pages/ToolLinks'));
const Tools = lazyRetry(() => import('./pages/Tools'));

export const infraRoutes = [
  { path: 'scripts', element: Scripts },
  { path: 'audit', element: AuditLogs },
  { path: 'settings', element: Settings },
  { path: 'tool-links', element: ToolLinks },
  { path: 'tools', element: Tools },
];
