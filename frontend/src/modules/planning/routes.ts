import { lazyRetry } from '@/lib/lazyRetry';

const PlanningApp = lazyRetry(() => import('./pages/PlanningApp'));

export const planningRoutes = [{ path: 'planning', element: PlanningApp }];
