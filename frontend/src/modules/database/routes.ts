import { lazyRetry } from '@/lib/lazyRetry';

const DbConnections = lazyRetry(() => import('./pages/DbConnections'));

export const databaseRoutes = [
  { path: 'db-connections', element: DbConnections },
];
