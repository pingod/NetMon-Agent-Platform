import { lazyRetry } from '@/lib/lazyRetry';

const Dashboard = lazyRetry(() => import('./pages/Dashboard'));
const Reports = lazyRetry(() => import('./pages/Reports'));
const BigScreenDashboard = lazyRetry(() => import('./pages/BigScreenDashboard'));
const CostAnalysis = lazyRetry(() => import('./pages/CostAnalysis'));

export const monitorRoutes = [
  { path: 'dashboard', element: Dashboard },
  { path: 'reports', element: Reports },
  { path: 'big-screen', element: BigScreenDashboard },
  { path: 'cost-analysis', element: CostAnalysis },
];
