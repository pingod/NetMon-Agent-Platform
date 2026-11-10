import { lazyRetry } from '@/lib/lazyRetry';

const Approvals = lazyRetry(() => import('./pages/Approvals'));

export const changeManagementRoutes = [
  { path: 'approvals', element: Approvals },
];
